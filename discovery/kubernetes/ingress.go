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
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

// Ingress implements discovery of Kubernetes ingress.
type Ingress struct {
	logger   log.Logger
	informer cache.SharedInformer
	store    cache.Store
	queue    *workqueue.Type
}

// NewIngress returns a new ingress discovery.
func NewIngress(l log.Logger, inf cache.SharedInformer) *Ingress {
	s := &Ingress{logger: l, informer: inf, store: inf.GetStore(), queue: workqueue.NewNamed("ingress")}
	s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("ingress", "add").Inc()
			s.enqueue(o)
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("ingress", "delete").Inc()
			s.enqueue(o)
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("ingress", "update").Inc()
			s.enqueue(o)
		},
	})
	return s
}

func (i *Ingress) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	i.queue.Add(key)
}

// Run implements the Discoverer interface.
func (i *Ingress) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer i.queue.ShutDown()

	if !cache.WaitForCacheSync(ctx.Done(), i.informer.HasSynced) {
		if ctx.Err() != context.Canceled {
			level.Error(i.logger).Log("msg", "ingress informer unable to sync cache")
		}
		return
	}

	go func() {
		for i.process(ctx, ch) {
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (i *Ingress) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	keyObj, quit := i.queue.Get()
	if quit {
		return false
	}
	defer i.queue.Done(keyObj)
	key := keyObj.(string)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return true
	}

	o, exists, err := i.store.GetByKey(key)
	if err != nil {
		return true
	}
	if !exists {
		send(ctx, i.logger, RoleIngress, ch, &targetgroup.Group{Source: ingressSourceFromNamespaceAndName(namespace, name)})
		return true
	}
	eps, err := convertToIngress(o)
	if err != nil {
		level.Error(i.logger).Log("msg", "converting to Ingress object failed", "err", err)
		return true
	}
	send(ctx, i.logger, RoleIngress, ch, i.buildIngress(eps))
	return true
}

func convertToIngress(o interface{}) (*v1beta1.Ingress, error) {
	ingress, ok := o.(*v1beta1.Ingress)
	if ok {
		return ingress, nil
	}

	return nil, errors.Errorf("received unexpected object: %v", o)
}

func ingressSource(s *v1beta1.Ingress) string {
	return ingressSourceFromNamespaceAndName(s.Namespace, s.Name)
}

func ingressSourceFromNamespaceAndName(namespace, name string) string {
	return "ingress/" + namespace + "/" + name
}

const (
	ingressNameLabel               = metaLabelPrefix + "ingress_name"
	ingressLabelPrefix             = metaLabelPrefix + "ingress_label_"
	ingressLabelPresentPrefix      = metaLabelPrefix + "ingress_labelpresent_"
	ingressAnnotationPrefix        = metaLabelPrefix + "ingress_annotation_"
	ingressAnnotationPresentPrefix = metaLabelPrefix + "ingress_annotationpresent_"
	ingressSchemeLabel             = metaLabelPrefix + "ingress_scheme"
	ingressHostLabel               = metaLabelPrefix + "ingress_host"
	ingressPathLabel               = metaLabelPrefix + "ingress_path"
)

func ingressLabels(ingress *v1beta1.Ingress) model.LabelSet {
	ls := make(model.LabelSet, len(ingress.Labels)+len(ingress.Annotations)+2)
	ls[ingressNameLabel] = lv(ingress.Name)
	ls[namespaceLabel] = lv(ingress.Namespace)

	for k, v := range ingress.Labels {
		ln := strutil.SanitizeLabelName(k)
		ls[model.LabelName(ingressLabelPrefix+ln)] = lv(v)
		ls[model.LabelName(ingressLabelPresentPrefix+ln)] = presentValue
	}

	for k, v := range ingress.Annotations {
		ln := strutil.SanitizeLabelName(k)
		ls[model.LabelName(ingressAnnotationPrefix+ln)] = lv(v)
		ls[model.LabelName(ingressAnnotationPresentPrefix+ln)] = presentValue
	}
	return ls
}

func pathsFromIngressRule(rv *v1beta1.IngressRuleValue) []string {
	if rv.HTTP == nil {
		return []string{"/"}
	}
	paths := make([]string, len(rv.HTTP.Paths))
	for n, p := range rv.HTTP.Paths {
		path := p.Path
		if path == "" {
			path = "/"
		}
		paths[n] = path
	}
	return paths
}

func (i *Ingress) buildIngress(ingress *v1beta1.Ingress) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: ingressSource(ingress),
	}
	tg.Labels = ingressLabels(ingress)

	tlsHosts := make(map[string]struct{})
	for _, tls := range ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			tlsHosts[host] = struct{}{}
		}
	}

	for _, rule := range ingress.Spec.Rules {
		paths := pathsFromIngressRule(&rule.IngressRuleValue)

		scheme := "http"
		_, isTLS := tlsHosts[rule.Host]
		if isTLS {
			scheme = "https"
		}

		for _, path := range paths {
			tg.Targets = append(tg.Targets, model.LabelSet{
				model.AddressLabel: lv(rule.Host),
				ingressSchemeLabel: lv(scheme),
				ingressHostLabel:   lv(rule.Host),
				ingressPathLabel:   lv(path),
			})
		}
	}

	return tg
}
