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
	"net"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	svcAddCount    = eventCount.WithLabelValues("service", "add")
	svcUpdateCount = eventCount.WithLabelValues("service", "update")
	svcDeleteCount = eventCount.WithLabelValues("service", "delete")
)

// Service implements discovery of Kubernetes services.
type Service struct {
	logger   log.Logger
	informer cache.SharedInformer
	store    cache.Store
	queue    *workqueue.Type
}

// NewService returns a new service discovery.
func NewService(l log.Logger, inf cache.SharedInformer) *Service {
	if l == nil {
		l = log.NewNopLogger()
	}
	s := &Service{logger: l, informer: inf, store: inf.GetStore(), queue: workqueue.NewNamed("service")}
	_, err := s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			svcAddCount.Inc()
			s.enqueue(o)
		},
		DeleteFunc: func(o interface{}) {
			svcDeleteCount.Inc()
			s.enqueue(o)
		},
		UpdateFunc: func(_, o interface{}) {
			svcUpdateCount.Inc()
			s.enqueue(o)
		},
	})
	if err != nil {
		level.Error(l).Log("msg", "Error adding services event handler.", "err", err)
	}
	return s
}

func (s *Service) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	s.queue.Add(key)
}

// Run implements the Discoverer interface.
func (s *Service) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer s.queue.ShutDown()

	if !cache.WaitForCacheSync(ctx.Done(), s.informer.HasSynced) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			level.Error(s.logger).Log("msg", "service informer unable to sync cache")
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
	keyObj, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(keyObj)
	key := keyObj.(string)

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
		level.Error(s.logger).Log("msg", "converting to Service object failed", "err", err)
		return true
	}
	send(ctx, ch, s.buildService(eps))
	return true
}

func convertToService(o interface{}) (*apiv1.Service, error) {
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
