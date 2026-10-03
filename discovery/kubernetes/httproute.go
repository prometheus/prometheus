// Copyright The Prometheus Authors
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// HTTPRoute implements discovery of Gateway API HTTPRoutes.
type HTTPRoute struct {
	logger                *slog.Logger
	informer              cache.SharedIndexInformer
	store                 cache.Store
	queue                 *workqueue.Typed[string]
	namespaceInf          cache.SharedInformer
	withNamespaceMetadata bool
}

// NewHTTPRoute returns a new HTTPRoute discovery.
func NewHTTPRoute(l *slog.Logger, inf cache.SharedIndexInformer, namespace cache.SharedInformer, eventCount *prometheus.CounterVec) *HTTPRoute {
	httpRouteAddCount := eventCount.WithLabelValues(RoleHTTPRoute.String(), MetricLabelRoleAdd)
	httpRouteUpdateCount := eventCount.WithLabelValues(RoleHTTPRoute.String(), MetricLabelRoleUpdate)
	httpRouteDeleteCount := eventCount.WithLabelValues(RoleHTTPRoute.String(), MetricLabelRoleDelete)

	s := &HTTPRoute{
		logger:   l,
		informer: inf,
		store:    inf.GetStore(),
		queue: workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{
			Name: RoleHTTPRoute.String(),
		}),
		namespaceInf:          namespace,
		withNamespaceMetadata: namespace != nil,
	}

	_, err := s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o any) {
			httpRouteAddCount.Inc()
			s.enqueue(o)
		},
		DeleteFunc: func(o any) {
			httpRouteDeleteCount.Inc()
			s.enqueue(o)
		},
		UpdateFunc: func(_, o any) {
			httpRouteUpdateCount.Inc()
			s.enqueue(o)
		},
	})
	if err != nil {
		l.Error("Error adding httproutes event handler.", "err", err)
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

func (r *HTTPRoute) enqueue(obj any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	r.queue.Add(key)
}

func (r *HTTPRoute) enqueueNamespace(namespace string) {
	httpRoutes, err := r.informer.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		r.logger.Error("Error getting httproutes in namespace", "namespace", namespace, "err", err)
		return
	}

	for _, hr := range httpRoutes {
		r.enqueue(hr)
	}
}

// Run implements the Discoverer interface.
func (r *HTTPRoute) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer r.queue.ShutDown()

	cacheSyncs := []cache.InformerSynced{r.informer.HasSynced}
	if r.withNamespaceMetadata {
		cacheSyncs = append(cacheSyncs, r.namespaceInf.HasSynced)
	}

	if !cache.WaitForCacheSync(ctx.Done(), cacheSyncs...) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			r.logger.Error("httproute informer unable to sync cache")
		}
		return
	}

	go func() {
		for r.process(ctx, ch) {
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (r *HTTPRoute) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	key, quit := r.queue.Get()
	if quit {
		return false
	}
	defer r.queue.Done(key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return true
	}

	o, exists, err := r.store.GetByKey(key)
	if err != nil {
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: httpRouteSourceFromNamespaceAndName(namespace, name)})
		return true
	}

	hr, ok := o.(*gatewayv1.HTTPRoute)
	if !ok {
		r.logger.Error("converting to HTTPRoute object failed", "err",
			fmt.Errorf("received unexpected object: %v", o))
		return true
	}
	send(ctx, ch, r.buildHTTPRoute(*hr))
	return true
}

func httpRouteSource(s gatewayv1.HTTPRoute) string {
	return httpRouteSourceFromNamespaceAndName(s.Namespace, s.Name)
}

func httpRouteSourceFromNamespaceAndName(namespace, name string) string {
	return "httproute/" + namespace + "/" + name
}

const (
	httpRouteHostnameLabel  = metaLabelPrefix + "httproute_hostname"
	httpRoutePathLabel      = metaLabelPrefix + "httproute_path"
	httpRouteParentRefLabel = metaLabelPrefix + "httproute_parent_ref_name"
)

func httpRouteLabels(hr gatewayv1.HTTPRoute) model.LabelSet {
	ls := make(model.LabelSet)
	ls[namespaceLabel] = lv(hr.Namespace)

	if len(hr.Spec.ParentRefs) > 0 {
		ls[httpRouteParentRefLabel] = lv(string(hr.Spec.ParentRefs[0].Name))
	}

	addObjectMetaLabels(ls, hr.ObjectMeta, RoleHTTPRoute)

	return ls
}

// httpRoutePaths returns the path matches configured for a rule, defaulting
// to "/" the same way Ingress paths do, since an HTTPRoute rule with no
// matches at all still applies to all paths.
func httpRoutePaths(rule gatewayv1.HTTPRouteRule) []string {
	if len(rule.Matches) == 0 {
		return []string{"/"}
	}
	paths := make([]string, 0, len(rule.Matches))
	for _, m := range rule.Matches {
		if m.Path == nil || m.Path.Value == nil || *m.Path.Value == "" {
			paths = append(paths, "/")
			continue
		}
		paths = append(paths, *m.Path.Value)
	}
	return paths
}

func (r *HTTPRoute) buildHTTPRoute(hr gatewayv1.HTTPRoute) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: httpRouteSource(hr),
	}
	tg.Labels = httpRouteLabels(hr)

	if r.withNamespaceMetadata {
		tg.Labels = addNamespaceLabels(tg.Labels, r.namespaceInf, r.logger, hr.Namespace)
	}

	hostnames := hr.Spec.Hostnames
	if len(hostnames) == 0 {
		// No hostnames means the route matches any hostname allowed by the
		// Gateways it's attached to. We can't resolve that here without
		// cross-referencing the parent Gateway, so we still emit one target
		// per rule/path with an empty hostname, same as Ingress does for an
		// unset rule.Host.
		hostnames = []gatewayv1.Hostname{""}
	}

	for _, hostname := range hostnames {
		for _, rule := range hr.Spec.Rules {
			for _, path := range httpRoutePaths(rule) {
				tg.Targets = append(tg.Targets, model.LabelSet{
					model.AddressLabel:     lv(string(hostname)),
					httpRouteHostnameLabel: lv(string(hostname)),
					httpRoutePathLabel:     lv(path),
				})
			}
		}
	}

	return tg
}
