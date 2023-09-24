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
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
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
	_, err := s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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
	if err != nil {
		level.Error(l).Log("msg", "Error adding ingresses event handler.", "err", err)
	}
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
		if !errors.Is(ctx.Err(), context.Canceled) {
			level.Error(i.logger).Log("msg", "ingress informer unable to sync cache")
		}
		return
	}

	go func() {
		for i.process(ctx, ch) { // nolint:revive
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

	ingress, err := convertToIngress(o)
	if err != nil {
		level.Error(i.logger).Log("msg", "converting to Ingress object failed", "err", err)
		return true
	}

	send(ctx, ch, i.buildIngress(ingress))
	return true
}

func ingressSourceFromNamespaceAndName(namespace, name string) string {
	return path.Join("ingress", namespace, name)
}

func convertToIngress(o interface{}) (*v1.Ingress, error) {
	ingress, ok := o.(*v1.Ingress)
	if ok {
		return ingress, nil
	}

	return nil, fmt.Errorf("received unexpected object: %v", o)
}

const (
	ingressSchemeLabel    = metaLabelPrefix + "ingress_scheme"
	ingressHostLabel      = metaLabelPrefix + "ingress_host"
	ingressPathLabel      = metaLabelPrefix + "ingress_path"
	ingressClassNameLabel = metaLabelPrefix + "ingress_class_name"
)

func ingressLabels(ingress *v1.Ingress) model.LabelSet {
	// Each label and annotation will create two key-value pairs in the map.
	ls := make(model.LabelSet)
	ls[namespaceLabel] = lv(ingress.ObjectMeta.Namespace)
	if cls := ingress.Spec.IngressClassName; cls != nil {
		ls[ingressClassNameLabel] = lv(*cls)
	}

	addObjectMetaLabels(ls, ingress.ObjectMeta, RoleIngress)

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

func ingressRulePaths(rule v1.IngressRule) []string {
	rv := rule.IngressRuleValue
	if rv.HTTP == nil {
		return nil
	}
	paths := make([]string, len(rv.HTTP.Paths))
	for n, p := range rv.HTTP.Paths {
		paths[n] = p.Path
	}
	return paths
}

func (i *Ingress) buildIngress(ingress *v1.Ingress) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: ingressSourceFromNamespaceAndName(ingress.ObjectMeta.Namespace, ingress.ObjectMeta.Name),
	}
	tg.Labels = ingressLabels(ingress)

	for _, rule := range ingress.Spec.Rules {
		scheme := "http"
		paths := pathsFromIngressPaths(ingressRulePaths(rule))
		var hosts []string
		for _, tls := range ingress.Spec.TLS {
			hosts = append(hosts, tls.Hosts...)
		}

	out:
		for _, pattern := range hosts {
			if matchesHostnamePattern(pattern, rule.Host) {
				scheme = "https"
				break out
			}
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
