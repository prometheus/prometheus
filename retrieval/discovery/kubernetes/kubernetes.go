// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

import (
	"io/ioutil"
	"time"

	"github.com/prometheus/prometheus/config"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	apiv1 "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

const (
	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "kubernetes_"
	namespaceLabel  = metaLabelPrefix + "namespace"
)

// Kubernetes implements the TargetProvider interface for discovering
// targets from Kubernetes.
type Kubernetes struct {
	client kubernetes.Interface
	role   config.KubernetesRole
	logger log.Logger
}

func init() {
	runtime.ErrorHandlers = append(runtime.ErrorHandlers, func(err error) {
		log.With("component", "kube_client_runtime").Errorln(err)
	})
}

// New creates a new Kubernetes discovery for the given role.
func New(l log.Logger, conf *config.KubernetesSDConfig) (*Kubernetes, error) {
	var (
		kcfg *rest.Config
		err  error
	)
	if conf.APIServer.URL == nil {
		kcfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		token := conf.BearerToken
		if conf.BearerTokenFile != "" {
			bf, err := ioutil.ReadFile(conf.BearerTokenFile)
			if err != nil {
				return nil, err
			}
			token = string(bf)
		}

		kcfg = &rest.Config{
			Host:        conf.APIServer.String(),
			BearerToken: token,
			TLSClientConfig: rest.TLSClientConfig{
				CAFile: conf.TLSConfig.CAFile,
			},
		}
	}
	kcfg.UserAgent = "prometheus/discovery"

	if conf.BasicAuth != nil {
		kcfg.Username = conf.BasicAuth.Username
		kcfg.Password = conf.BasicAuth.Password
	}
	kcfg.TLSClientConfig.CertFile = conf.TLSConfig.CertFile
	kcfg.TLSClientConfig.KeyFile = conf.TLSConfig.KeyFile
	kcfg.Insecure = conf.TLSConfig.InsecureSkipVerify

	c, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}
	return &Kubernetes{
		client: c,
		logger: l,
		role:   conf.Role,
	}, nil
}

const resyncPeriod = 10 * time.Minute

// Run implements the TargetProvider interface.
func (k *Kubernetes) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	rclient := k.client.Core().GetRESTClient()

	switch k.role {
	case "endpoints":
		elw := cache.NewListWatchFromClient(rclient, "endpoints", api.NamespaceAll, nil)
		slw := cache.NewListWatchFromClient(rclient, "services", api.NamespaceAll, nil)
		plw := cache.NewListWatchFromClient(rclient, "pods", api.NamespaceAll, nil)
		eps := NewEndpoints(
			k.logger.With("kubernetes_sd", "endpoint"),
			cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
			cache.NewSharedInformer(elw, &apiv1.Endpoints{}, resyncPeriod),
			cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
		)
		go eps.endpointsInf.Run(ctx.Done())
		go eps.serviceInf.Run(ctx.Done())
		go eps.podInf.Run(ctx.Done())

		for !eps.serviceInf.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		for !eps.endpointsInf.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		for !eps.podInf.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		eps.Run(ctx, ch)

	case "pod":
		plw := cache.NewListWatchFromClient(rclient, "pods", api.NamespaceAll, nil)
		pod := NewPod(
			k.logger.With("kubernetes_sd", "pod"),
			cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
		)
		go pod.informer.Run(ctx.Done())

		for !pod.informer.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		pod.Run(ctx, ch)

	case "service":
		slw := cache.NewListWatchFromClient(rclient, "services", api.NamespaceAll, nil)
		svc := NewService(
			k.logger.With("kubernetes_sd", "service"),
			cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
		)
		go svc.informer.Run(ctx.Done())

		for !svc.informer.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		svc.Run(ctx, ch)

	case "node":
		nlw := cache.NewListWatchFromClient(rclient, "nodes", api.NamespaceAll, nil)
		node := NewNode(
			k.logger.With("kubernetes_sd", "node"),
			cache.NewSharedInformer(nlw, &apiv1.Node{}, resyncPeriod),
		)
		go node.informer.Run(ctx.Done())

		for !node.informer.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		node.Run(ctx, ch)

	default:
		k.logger.Errorf("unknown Kubernetes discovery kind %q", k.role)
	}

	<-ctx.Done()
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}
