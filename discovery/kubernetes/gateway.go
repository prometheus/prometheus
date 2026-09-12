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
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Gateway implements discovery of Gateway API Gateways.
type Gateway struct {
	logger                *slog.Logger
	informer              cache.SharedIndexInformer
	store                 cache.Store
	queue                 *workqueue.Typed[string]
	namespaceInf          cache.SharedInformer
	withNamespaceMetadata bool
}

// NewGateway returns a new Gateway discovery.
func NewGateway(l *slog.Logger, inf cache.SharedIndexInformer, namespace cache.SharedInformer, eventCount *prometheus.CounterVec) *Gateway {
	gatewayAddCount := eventCount.WithLabelValues(RoleGateway.String(), MetricLabelRoleAdd)
	gatewayUpdateCount := eventCount.WithLabelValues(RoleGateway.String(), MetricLabelRoleUpdate)
	gatewayDeleteCount := eventCount.WithLabelValues(RoleGateway.String(), MetricLabelRoleDelete)

	s := &Gateway{
		logger:   l,
		informer: inf,
		store:    inf.GetStore(),
		queue: workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{
			Name: RoleGateway.String(),
		}),
		namespaceInf:          namespace,
		withNamespaceMetadata: namespace != nil,
	}

	_, err := s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o any) {
			gatewayAddCount.Inc()
			s.enqueue(o)
		},
		DeleteFunc: func(o any) {
			gatewayDeleteCount.Inc()
			s.enqueue(o)
		},
		UpdateFunc: func(_, o any) {
			gatewayUpdateCount.Inc()
			s.enqueue(o)
		},
	})
	if err != nil {
		l.Error("Error adding gateways event handler.", "err", err)
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

func (g *Gateway) enqueue(obj any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	g.queue.Add(key)
}

func (g *Gateway) enqueueNamespace(namespace string) {
	gateways, err := g.informer.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		g.logger.Error("Error getting gateways in namespace", "namespace", namespace, "err", err)
		return
	}

	for _, gtw := range gateways {
		g.enqueue(gtw)
	}
}

// Run implements the Discoverer interface.
func (g *Gateway) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer g.queue.ShutDown()

	cacheSyncs := []cache.InformerSynced{g.informer.HasSynced}
	if g.withNamespaceMetadata {
		cacheSyncs = append(cacheSyncs, g.namespaceInf.HasSynced)
	}

	if !cache.WaitForCacheSync(ctx.Done(), cacheSyncs...) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			g.logger.Error("gateway informer unable to sync cache")
		}
		return
	}

	go func() {
		for g.process(ctx, ch) {
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (g *Gateway) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	key, quit := g.queue.Get()
	if quit {
		return false
	}
	defer g.queue.Done(key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return true
	}

	o, exists, err := g.store.GetByKey(key)
	if err != nil {
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: gatewaySourceFromNamespaceAndName(namespace, name)})
		return true
	}

	gtw, ok := o.(*gatewayv1.Gateway)
	if !ok {
		g.logger.Error("converting to Gateway object failed", "err",
			fmt.Errorf("received unexpected object: %v", o))
		return true
	}
	send(ctx, ch, g.buildGateway(*gtw))
	return true
}

func gatewaySource(s gatewayv1.Gateway) string {
	return gatewaySourceFromNamespaceAndName(s.Namespace, s.Name)
}

func gatewaySourceFromNamespaceAndName(namespace, name string) string {
	return "gateway/" + namespace + "/" + name
}

const (
	gatewayClassNameLabel        = metaLabelPrefix + "gateway_class_name"
	gatewayListenerNameLabel     = metaLabelPrefix + "gateway_listener_name"
	gatewayListenerHostnameLabel = metaLabelPrefix + "gateway_listener_hostname"
	gatewayListenerPortLabel     = metaLabelPrefix + "gateway_listener_port"
	gatewayListenerProtocolLabel = metaLabelPrefix + "gateway_listener_protocol"
)

func gatewayLabels(gtw gatewayv1.Gateway) model.LabelSet {
	ls := make(model.LabelSet)
	ls[namespaceLabel] = lv(gtw.Namespace)
	ls[gatewayClassNameLabel] = lv(string(gtw.Spec.GatewayClassName))

	addObjectMetaLabels(ls, gtw.ObjectMeta, RoleGateway)

	return ls
}

func (g *Gateway) buildGateway(gtw gatewayv1.Gateway) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: gatewaySource(gtw),
	}
	tg.Labels = gatewayLabels(gtw)

	if g.withNamespaceMetadata {
		tg.Labels = addNamespaceLabels(tg.Labels, g.namespaceInf, g.logger, gtw.Namespace)
	}

	for _, listener := range gtw.Spec.Listeners {
		hostname := ""
		if listener.Hostname != nil {
			hostname = string(*listener.Hostname)
		}

		port := strconv.Itoa(int(listener.Port))
		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel:           lv(hostname + ":" + port),
			gatewayListenerNameLabel:     lv(string(listener.Name)),
			gatewayListenerHostnameLabel: lv(hostname),
			gatewayListenerPortLabel:     lv(port),
			gatewayListenerProtocolLabel: lv(string(listener.Protocol)),
		})
	}

	return tg
}
