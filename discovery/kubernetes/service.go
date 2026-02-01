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
	"net"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// Service implements discovery of Kubernetes services.
type Service struct {
	logger                *slog.Logger
	informer              cache.SharedIndexInformer
	store                 cache.Store
	queue                 *workqueue.Typed[string]
	namespaceInf          cache.SharedInformer
	withNamespaceMetadata bool
}

// NewService returns a new service discovery.
func NewService(l *slog.Logger, inf cache.SharedIndexInformer, namespace cache.SharedInformer, eventCount *prometheus.CounterVec) *Service {
	if l == nil {
		l = promslog.NewNopLogger()
	}

	svcAddCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleAdd)
	svcUpdateCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleUpdate)
	svcDeleteCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleDelete)

	s := &Service{
		logger:   l,
		informer: inf,
		store:    inf.GetStore(),
		queue: workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{
			Name: RoleService.String(),
		}),
		namespaceInf:          namespace,
		withNamespaceMetadata: namespace != nil,
	}

	_, err := s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o any) {
			svcAddCount.Inc()
			s.enqueue(o)
		},
		DeleteFunc: func(o any) {
			svcDeleteCount.Inc()
			s.enqueue(o)
		},
		UpdateFunc: func(_, o any) {
			svcUpdateCount.Inc()
			s.enqueue(o)
		},
	})
	if err != nil {
		l.Error("Error adding services event handler.", "err", err)
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

func (s *Service) enqueue(obj any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	s.queue.Add(key)
}

func (s *Service) enqueueNamespace(namespace string) {
	services, err := s.informer.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		s.logger.Error("Error getting services in namespace", "namespace", namespace, "err", err)
		return
	}

	for _, service := range services {
		s.enqueue(service)
	}
}

// Run implements the Discoverer interface.
func (s *Service) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer s.queue.ShutDown()

	cacheSyncs := []cache.InformerSynced{s.informer.HasSynced}
	if s.withNamespaceMetadata {
		cacheSyncs = append(cacheSyncs, s.namespaceInf.HasSynced)
	}

	if !cache.WaitForCacheSync(ctx.Done(), cacheSyncs...) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			s.logger.Error("service informer unable to sync cache")
		}
		return
	}

	go func() {
		for s.process(ctx, ch) {
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (s *Service) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	key, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return true
	}

	o, exists, err := s.store.GetByKey(key)
	if err != nil {
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: serviceSourceFromNamespaceAndName(namespace, name)})
		return true
	}
	eps, err := convertToService(o)
	if err != nil {
		s.logger.Error("converting to Service object failed", "err", err)
		return true
	}
	send(ctx, ch, s.buildService(eps))
	return true
}

func convertToService(o any) (*apiv1.Service, error) {
	service, ok := o.(*apiv1.Service)
	if ok {
		return service, nil
	}
	return nil, fmt.Errorf("received unexpected object: %v", o)
}

func serviceSource(s *apiv1.Service) string {
	return serviceSourceFromNamespaceAndName(s.Namespace, s.Name)
}

func serviceSourceFromNamespaceAndName(namespace, name string) string {
	return "svc/" + namespace + "/" + name
}

const (
	servicePortNameLabel     = metaLabelPrefix + "service_port_name"
	servicePortNumberLabel   = metaLabelPrefix + "service_port_number"
	servicePortProtocolLabel = metaLabelPrefix + "service_port_protocol"
	serviceClusterIPLabel    = metaLabelPrefix + "service_cluster_ip"
	serviceLoadBalancerIP    = metaLabelPrefix + "service_loadbalancer_ip"
	serviceExternalNameLabel = metaLabelPrefix + "service_external_name"
	serviceType              = metaLabelPrefix + "service_type"
)

func serviceLabels(svc *apiv1.Service) model.LabelSet {
	ls := make(model.LabelSet)
	ls[namespaceLabel] = lv(svc.Namespace)
	addObjectMetaLabels(ls, svc.ObjectMeta, RoleService)

	return ls
}

func (s *Service) buildService(svc *apiv1.Service) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: serviceSource(svc),
	}
	tg.Labels = serviceLabels(svc)

	if s.withNamespaceMetadata {
		tg.Labels = addNamespaceLabels(tg.Labels, s.namespaceInf, s.logger, svc.Namespace)
	}

	for _, port := range svc.Spec.Ports {
		addr := net.JoinHostPort(svc.Name+"."+svc.Namespace+".svc", strconv.FormatInt(int64(port.Port), 10))

		labelSet := model.LabelSet{
			model.AddressLabel:       lv(addr),
			servicePortNameLabel:     lv(port.Name),
			servicePortNumberLabel:   lv(strconv.FormatInt(int64(port.Port), 10)),
			servicePortProtocolLabel: lv(string(port.Protocol)),
			serviceType:              lv(string(svc.Spec.Type)),
		}

		if svc.Spec.Type == apiv1.ServiceTypeExternalName {
			labelSet[serviceExternalNameLabel] = lv(svc.Spec.ExternalName)
		} else {
			labelSet[serviceClusterIPLabel] = lv(svc.Spec.ClusterIP)
		}

		if svc.Spec.Type == apiv1.ServiceTypeLoadBalancer {
			labelSet[serviceLoadBalancerIP] = lv(svc.Spec.LoadBalancerIP)
		}

		tg.Targets = append(tg.Targets, labelSet)
	}

	return tg
}
