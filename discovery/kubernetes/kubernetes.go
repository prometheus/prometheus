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
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/config"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "kubernetes_"
	namespaceLabel  = metaLabelPrefix + "namespace"
)

var (
	eventCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_kubernetes_events_total",
			Help: "The number of Kubernetes events handled.",
		},
		[]string{"role", "event"},
	)
)

func init() {
	prometheus.MustRegister(eventCount)

	// Initialize metric vectors.
	for _, role := range []string{"endpoints", "node", "pod", "service"} {
		for _, evt := range []string{"add", "delete", "update"} {
			eventCount.WithLabelValues(role, evt)
		}
	}
}

// Discovery implements the TargetProvider interface for discovering
// targets from Kubernetes.
type Discovery struct {
	client             kubernetes.Interface
	role               config.KubernetesRole
	logger             log.Logger
	namespaceDiscovery *config.KubernetesNamespaceDiscovery
}

func init() {
	runtime.ErrorHandlers = []func(error){
		func(err error) {
			log.With("component", "kube_client_runtime").Errorln(err)
		},
	}
}

func (d *Discovery) getNamespaces() []string {
	namespaces := d.namespaceDiscovery.Names
	if len(namespaces) == 0 {
		namespaces = []string{api.NamespaceAll}
	}
	return namespaces
}

// New creates a new Kubernetes discovery for the given role.
func New(l log.Logger, conf *config.KubernetesSDConfig) (*Discovery, error) {
	var (
		kcfg *rest.Config
		err  error
	)
	if conf.APIServer.URL == nil {
		// Use the Kubernetes provided pod service account
		// as described in https://kubernetes.io/docs/admin/service-accounts-admin/
		kcfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		// Because the handling of configuration parameters changes
		// we should inform the user when their currently configured values
		// will be ignored due to precedence of InClusterConfig
		l.Info("Using pod service account via in-cluster config")
		if conf.TLSConfig.CAFile != "" {
			l.Warn("Configured TLS CA file is ignored when using pod service account")
		}
		if conf.TLSConfig.CertFile != "" || conf.TLSConfig.KeyFile != "" {
			l.Warn("Configured TLS client certificate is ignored when using pod service account")
		}
		if conf.BearerToken != "" {
			l.Warn("Configured auth token is ignored when using pod service account")
		}
		if conf.BasicAuth != nil {
			l.Warn("Configured basic authentication credentials are ignored when using pod service account")
		}
	} else {
		kcfg = &rest.Config{
			Host: conf.APIServer.String(),
			TLSClientConfig: rest.TLSClientConfig{
				CAFile:   conf.TLSConfig.CAFile,
				CertFile: conf.TLSConfig.CertFile,
				KeyFile:  conf.TLSConfig.KeyFile,
				Insecure: conf.TLSConfig.InsecureSkipVerify,
			},
		}
		token := string(conf.BearerToken)
		if conf.BearerTokenFile != "" {
			bf, err := ioutil.ReadFile(conf.BearerTokenFile)
			if err != nil {
				return nil, err
			}
			token = string(bf)
		}
		kcfg.BearerToken = token

		if conf.BasicAuth != nil {
			kcfg.Username = conf.BasicAuth.Username
			kcfg.Password = string(conf.BasicAuth.Password)
		}
	}

	kcfg.UserAgent = "prometheus/discovery"

	c, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}
	return &Discovery{
		client:             c,
		logger:             l,
		role:               conf.Role,
		namespaceDiscovery: &conf.NamespaceDiscovery,
	}, nil
}

const resyncPeriod = 10 * time.Minute

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	rclient := d.client.Core().RESTClient()

	namespaces := d.getNamespaces()

	switch d.role {
	case "endpoints":
		var wg sync.WaitGroup

		for _, namespace := range namespaces {
			elw := cache.NewListWatchFromClient(rclient, "endpoints", namespace, nil)
			slw := cache.NewListWatchFromClient(rclient, "services", namespace, nil)
			plw := cache.NewListWatchFromClient(rclient, "pods", namespace, nil)
			eps := NewEndpoints(
				d.logger.With("kubernetes_sd", "endpoint"),
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
			wg.Add(1)
			go func() {
				defer wg.Done()
				eps.Run(ctx, ch)
			}()
		}
		wg.Wait()
	case "pod":
		var wg sync.WaitGroup
		for _, namespace := range namespaces {
			plw := cache.NewListWatchFromClient(rclient, "pods", namespace, nil)
			pod := NewPod(
				d.logger.With("kubernetes_sd", "pod"),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
			)
			go pod.informer.Run(ctx.Done())

			for !pod.informer.HasSynced() {
				time.Sleep(100 * time.Millisecond)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				pod.Run(ctx, ch)
			}()
		}
		wg.Wait()
	case "service":
		var wg sync.WaitGroup
		for _, namespace := range namespaces {
			slw := cache.NewListWatchFromClient(rclient, "services", namespace, nil)
			svc := NewService(
				d.logger.With("kubernetes_sd", "service"),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
			)
			go svc.informer.Run(ctx.Done())

			for !svc.informer.HasSynced() {
				time.Sleep(100 * time.Millisecond)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				svc.Run(ctx, ch)
			}()
		}
		wg.Wait()
	case "node":
		nlw := cache.NewListWatchFromClient(rclient, "nodes", api.NamespaceAll, nil)
		node := NewNode(
			d.logger.With("kubernetes_sd", "node"),
			cache.NewSharedInformer(nlw, &apiv1.Node{}, resyncPeriod),
		)
		go node.informer.Run(ctx.Done())

		for !node.informer.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		node.Run(ctx, ch)

	default:
		d.logger.Errorf("unknown Kubernetes discovery kind %q", d.role)
	}

	<-ctx.Done()
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}
