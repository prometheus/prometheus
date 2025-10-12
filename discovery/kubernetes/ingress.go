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
	"log/slog"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Ingress implements discovery of Kubernetes ingress.
type Ingress struct {
	logger                *slog.Logger
	informer              cache.SharedIndexInformer
	store                 cache.Store
	queue                 *workqueue.Typed[string]
	namespaceInf          cache.SharedInformer
	withNamespaceMetadata bool
}

// NewIngress returns a new ingress discovery.
func NewIngress(l *slog.Logger, inf cache.SharedIndexInformer, namespace cache.SharedInformer, eventCount *prometheus.CounterVec) *Ingress {
	ingressAddCount := eventCount.WithLabelValues(RoleIngress.String(), MetricLabelRoleAdd)
	ingressUpdateCount := eventCount.WithLabelValues(RoleIngress.String(), MetricLabelRoleUpdate)
	ingressDeleteCount := eventCount.WithLabelValues(RoleIngress.String(), MetricLabelRoleDelete)

	s := &Ingress{
		logger:   l,
		informer: inf,
		store:    inf.GetStore(),
		queue: workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{
			Name: RoleIngress.String(),
		}),
		namespaceInf:          namespace,
		withNamespaceMetadata: namespace != nil,
	}

	_, err := s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o any) {
			ingressAddCount.Inc()
			s.enqueue(o)
		},
		DeleteFunc: func(o any) {
			ingressDeleteCount.Inc()
			s.enqueue(o)
		},
		UpdateFunc: func(_, o any) {
			ingressUpdateCount.Inc()
			s.enqueue(o)
		},
	})
	if err != nil {
		l.Error("Error adding ingresses event handler.", "err", err)
	}

	if s.withNamespaceMetadata {
		_, err = s.namespaceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(_, o any) {
				namespace := o.(*apiv1.Namespace)
				s.enqueueNamespace(namespace.Name)
			},
			// Creation and deletion will trigger events for the change handlers of the resources within the namespace.
			// No need to have additional handlers for them here.
		})
		if err != nil {
			l.Error("Error adding namespaces event handler.", "err", err)
		}
	}

	return s
}

func (i *Ingress) enqueue(obj any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	i.queue.Add(key)
}

func (i *Ingress) enqueueNamespace(namespace string) {
	ingresses, err := i.informer.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		i.logger.Error("Error getting ingresses in namespace", "namespace", namespace, "err", err)
		return
	}

	for _, ingress := range ingresses {
		i.enqueue(ingress)
	}
}

// Run implements the Discoverer interface.
func (i *Ingress) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer i.queue.ShutDown()

	cacheSyncs := []cache.InformerSynced{i.informer.HasSynced}
	if i.withNamespaceMetadata {
		cacheSyncs = append(cacheSyncs, i.namespaceInf.HasSynced)
	}

	if !cache.WaitForCacheSync(ctx.Done(), cacheSyncs...) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			i.logger.Error("ingress informer unable to sync cache")
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
	key, quit := i.queue.Get()
	if quit {
		return false
	}
	defer i.queue.Done(key)

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

	if ingress, ok := o.(*v1.Ingress); ok {
		send(ctx, ch, i.buildIngress(*ingress))
	} else {
		i.logger.Error("converting to Ingress object failed", "err",
			fmt.Errorf("received unexpected object: %v", o))
		return true
	}
	return true
}

func ingressSource(s v1.Ingress) string {
	return ingressSourceFromNamespaceAndName(s.Namespace, s.Name)
}

func ingressSourceFromNamespaceAndName(namespace, name string) string {
	return "ingress/" + namespace + "/" + name
}

const (
	ingressSchemeLabel    = metaLabelPrefix + "ingress_scheme"
	ingressHostLabel      = metaLabelPrefix + "ingress_host"
	ingressPathLabel      = metaLabelPrefix + "ingress_path"
	ingressClassNameLabel = metaLabelPrefix + "ingress_class_name"
)

func ingressLabels(ingress v1.Ingress) model.LabelSet {
	// Each label and annotation will create two key-value pairs in the map.
	ls := make(model.LabelSet)
	ls[namespaceLabel] = lv(ingress.Namespace)
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

func rulePaths(rule v1.IngressRule) []string {
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

func tlsHosts(ingressTLS []v1.IngressTLS) []string {
	var hosts []string
	for _, tls := range ingressTLS {
		hosts = append(hosts, tls.Hosts...)
	}
	return hosts
}

func (i *Ingress) buildIngress(ingress v1.Ingress) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: ingressSource(ingress),
	}
	tg.Labels = ingressLabels(ingress)

	if i.withNamespaceMetadata {
		tg.Labels = addNamespaceLabels(tg.Labels, i.namespaceInf, i.logger, ingress.Namespace)
	}

	for _, rule := range ingress.Spec.Rules {
		scheme := "http"
		paths := pathsFromIngressPaths(rulePaths(rule))

	out:
		for _, pattern := range tlsHosts(ingress.Spec.TLS) {
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
