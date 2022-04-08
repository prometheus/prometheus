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
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

var (
	ingressAddCount    = eventCount.WithLabelValues("ingress", "add")
	ingressUpdateCount = eventCount.WithLabelValues("ingress", "update")
	ingressDeleteCount = eventCount.WithLabelValues("ingress", "delete")
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
			ingressAddCount.Inc()
			s.enqueue(o)
		},
		DeleteFunc: func(o interface{}) {
			ingressDeleteCount.Inc()
			s.enqueue(o)
		},
		UpdateFunc: func(_, o interface{}) {
			ingressUpdateCount.Inc()
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
		send(ctx, ch, &targetgroup.Group{Source: ingressSourceFromNamespaceAndName(namespace, name)})
		return true
	}

	var ia ingressAdaptor
	switch ingress := o.(type) {
	case *v1.Ingress:
		ia = newIngressAdaptorFromV1(ingress)
	case *v1beta1.Ingress:
		ia = newIngressAdaptorFromV1beta1(ingress)
	default:
		level.Error(i.logger).Log("msg", "converting to Ingress object failed", "err",
			errors.Errorf("received unexpected object: %v", o))
		return true
	}
	send(ctx, ch, i.buildIngress(ia))
	return true
}

func ingressSource(s ingressAdaptor) string {
	return ingressSourceFromNamespaceAndName(s.namespace(), s.name())
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
	ingressClassNameLabel          = metaLabelPrefix + "ingress_class_name"
)

func ingressLabels(ingress ingressAdaptor) model.LabelSet {
	// Each label and annotation will create two key-value pairs in the map.
	ls := make(model.LabelSet, 2*(len(ingress.labels())+len(ingress.annotations()))+2)
	ls[ingressNameLabel] = lv(ingress.name())
	ls[namespaceLabel] = lv(ingress.namespace())
	if cls := ingress.ingressClassName(); cls != nil {
		ls[ingressClassNameLabel] = lv(*cls)
	}

	for k, v := range ingress.labels() {
		ln := strutil.SanitizeLabelName(k)
		ls[model.LabelName(ingressLabelPrefix+ln)] = lv(v)
		ls[model.LabelName(ingressLabelPresentPrefix+ln)] = presentValue
	}

	for k, v := range ingress.annotations() {
		ln := strutil.SanitizeLabelName(k)
		ls[model.LabelName(ingressAnnotationPrefix+ln)] = lv(v)
		ls[model.LabelName(ingressAnnotationPresentPrefix+ln)] = presentValue
	}
	return ls
}

func pathsFromIngressPaths(ingressPaths []string) []string {
	if ingressPaths == nil {
		return []string{"/"}
	}
	paths := make([]string, len(ingressPaths))
	for n, p := range ingressPaths {
		path := p
		if p == "" {
			path = "/"
		}
		paths[n] = path
	}
	return paths
}

func (i *Ingress) buildIngress(ingress ingressAdaptor) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: ingressSource(ingress),
	}
	tg.Labels = ingressLabels(ingress)

	for _, rule := range ingress.rules() {
		scheme := "http"
		paths := pathsFromIngressPaths(rule.paths())

	out:
		for _, pattern := range ingress.tlsHosts() {
			if matchesHostnamePattern(pattern, rule.host()) {
				scheme = "https"
				break out
			}
		}

		for _, path := range paths {
			tg.Targets = append(tg.Targets, model.LabelSet{
				model.AddressLabel: lv(rule.host()),
				ingressSchemeLabel: lv(scheme),
				ingressHostLabel:   lv(rule.host()),
				ingressPathLabel:   lv(path),
			})
		}
	}

	return tg
}

// matchesHostnamePattern returns true if the host matches a wildcard DNS
// pattern or pattern and host are equal.
func matchesHostnamePattern(pattern, host string) bool {
	if pattern == host {
		return true
	}

	patternParts := strings.Split(pattern, ".")
	hostParts := strings.Split(host, ".")

	// If the first element of the pattern is not a wildcard, give up.
	if len(patternParts) == 0 || patternParts[0] != "*" {
		return false
	}

	// A wildcard match require the pattern to have the same length as the host
	// path.
	if len(patternParts) != len(hostParts) {
		return false
	}

	for i := 1; i < len(patternParts); i++ {
		if patternParts[i] != hostParts[i] {
			return false
		}
	}

	return true
}
