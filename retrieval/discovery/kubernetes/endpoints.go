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
	"fmt"
	"net"
	"strconv"

	"github.com/prometheus/prometheus/config"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	apiv1 "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/cache"
)

// Endpoints discovers new endpoint targets.
type Endpoints struct {
	logger log.Logger

	endpointsInf cache.SharedInformer
	serviceInf   cache.SharedInformer
	podInf       cache.SharedInformer

	podStore       cache.Store
	endpointsStore cache.Store
	serviceStore   cache.Store
}

// NewEndpoints returns a new endpoints discovery.
func NewEndpoints(l log.Logger, svc, eps, pod cache.SharedInformer) *Endpoints {
	ep := &Endpoints{
		logger:         l,
		endpointsInf:   eps,
		endpointsStore: eps.GetStore(),
		serviceInf:     svc,
		serviceStore:   svc.GetStore(),
		podInf:         pod,
		podStore:       pod.GetStore(),
	}

	return ep
}

// Run implements the retrieval.TargetProvider interface.
func (e *Endpoints) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Send full initial set of endpoint targets.
	var initial []*config.TargetGroup

	for _, o := range e.endpointsStore.List() {
		tg := e.buildEndpoints(o.(*apiv1.Endpoints))
		initial = append(initial, tg)
	}
	select {
	case <-ctx.Done():
		return
	case ch <- initial:
	}
	// Send target groups for pod updates.
	send := func(tg *config.TargetGroup) {
		if tg == nil {
			return
		}
		e.logger.With("tg", fmt.Sprintf("%#v", tg)).Debugln("endpoints update")
		select {
		case <-ctx.Done():
		case ch <- []*config.TargetGroup{tg}:
		}
	}

	e.endpointsInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("endpoints", "add").Inc()

			eps, err := convertToEndpoints(o)
			if err != nil {
				e.logger.With("err", err).Errorln("converting to Endpoints object failed")
				return
			}
			send(e.buildEndpoints(eps))
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("endpoints", "update").Inc()

			eps, err := convertToEndpoints(o)
			if err != nil {
				e.logger.With("err", err).Errorln("converting to Endpoints object failed")
				return
			}
			send(e.buildEndpoints(eps))
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("endpoints", "delete").Inc()

			eps, err := convertToEndpoints(o)
			if err != nil {
				e.logger.With("err", err).Errorln("converting to Endpoints object failed")
				return
			}
			send(&config.TargetGroup{Source: endpointsSource(eps)})
		},
	})

	serviceUpdate := func(o interface{}) {
		svc, err := convertToService(o)
		if err != nil {
			e.logger.With("err", err).Errorln("converting to Service object failed")
			return
		}

		ep := &apiv1.Endpoints{}
		ep.Namespace = svc.Namespace
		ep.Name = svc.Name
		obj, exists, err := e.endpointsStore.Get(ep)
		if exists && err != nil {
			send(e.buildEndpoints(obj.(*apiv1.Endpoints)))
		}
		if err != nil {
			e.logger.With("err", err).Errorln("retrieving endpoints failed")
		}
	}
	e.serviceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		// TODO(fabxc): potentially remove add and delete event handlers. Those should
		// be triggered via the endpoint handlers already.
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("service", "add").Inc()
			serviceUpdate(o)
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("service", "update").Inc()
			serviceUpdate(o)
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("service", "delete").Inc()
			serviceUpdate(o)
		},
	})

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func convertToEndpoints(o interface{}) (*apiv1.Endpoints, error) {
	endpoints, isEndpoints := o.(*apiv1.Endpoints)
	if !isEndpoints {
		deletedState, ok := o.(cache.DeletedFinalStateUnknown)
		if !ok {
			return nil, fmt.Errorf("Received unexpected object: %v", o)
		}
		endpoints, ok = deletedState.Obj.(*apiv1.Endpoints)
		if !ok {
			return nil, fmt.Errorf("DeletedFinalStateUnknown contained non-Endpoints object: %v", deletedState.Obj)
		}
	}

	return endpoints, nil
}

func endpointsSource(ep *apiv1.Endpoints) string {
	return "endpoints/" + ep.ObjectMeta.Namespace + "/" + ep.ObjectMeta.Name
}

const (
	endpointsNameLabel        = metaLabelPrefix + "endpoints_name"
	endpointReadyLabel        = metaLabelPrefix + "endpoint_ready"
	endpointPortNameLabel     = metaLabelPrefix + "endpoint_port_name"
	endpointPortProtocolLabel = metaLabelPrefix + "endpoint_port_protocol"
)

func (e *Endpoints) buildEndpoints(eps *apiv1.Endpoints) *config.TargetGroup {
	if len(eps.Subsets) == 0 {
		return nil
	}

	tg := &config.TargetGroup{
		Source: endpointsSource(eps),
	}
	tg.Labels = model.LabelSet{
		namespaceLabel:     lv(eps.Namespace),
		endpointsNameLabel: lv(eps.Name),
	}
	e.addServiceLabels(eps.Namespace, eps.Name, tg)

	type podEntry struct {
		pod          *apiv1.Pod
		servicePorts []apiv1.EndpointPort
	}
	seenPods := map[string]*podEntry{}

	add := func(addr apiv1.EndpointAddress, port apiv1.EndpointPort, ready string) {
		a := net.JoinHostPort(addr.IP, strconv.FormatUint(uint64(port.Port), 10))

		target := model.LabelSet{
			model.AddressLabel:        lv(a),
			endpointPortNameLabel:     lv(port.Name),
			endpointPortProtocolLabel: lv(string(port.Protocol)),
			endpointReadyLabel:        lv(ready),
		}

		pod := e.resolvePodRef(addr.TargetRef)
		if pod == nil {
			// This target is not a Pod, so don't continue with Pod specific logic.
			tg.Targets = append(tg.Targets, target)
			return
		}
		s := pod.Namespace + "/" + pod.Name

		sp, ok := seenPods[s]
		if !ok {
			sp = &podEntry{pod: pod}
			seenPods[s] = sp
		}

		// Attach standard pod labels.
		target = target.Merge(podLabels(pod))

		// Attach potential container port labels matching the endpoint port.
		for _, c := range pod.Spec.Containers {
			for _, cport := range c.Ports {
				if port.Port == cport.ContainerPort {
					ports := strconv.FormatUint(uint64(port.Port), 10)

					target[podContainerNameLabel] = lv(c.Name)
					target[podContainerPortNameLabel] = lv(cport.Name)
					target[podContainerPortNumberLabel] = lv(ports)
					target[podContainerPortProtocolLabel] = lv(string(port.Protocol))
					break
				}
			}
		}

		// Add service port so we know that we have already generated a target
		// for it.
		sp.servicePorts = append(sp.servicePorts, port)
		tg.Targets = append(tg.Targets, target)
	}

	for _, ss := range eps.Subsets {
		for _, port := range ss.Ports {
			for _, addr := range ss.Addresses {
				add(addr, port, "true")
			}
			// Although this generates the same target again, as it was generated in
			// the loop above, it causes the ready meta label to be overridden.
			for _, addr := range ss.NotReadyAddresses {
				add(addr, port, "false")
			}
		}
	}

	// For all seen pods, check all container ports. If they were not covered
	// by one of the service endpoints, generate targets for them.
	for _, pe := range seenPods {
		for _, c := range pe.pod.Spec.Containers {
			for _, cport := range c.Ports {
				hasSeenPort := func() bool {
					for _, eport := range pe.servicePorts {
						if cport.ContainerPort == eport.Port {
							return true
						}
					}
					return false
				}
				if hasSeenPort() {
					continue
				}

				a := net.JoinHostPort(pe.pod.Status.PodIP, strconv.FormatUint(uint64(cport.ContainerPort), 10))
				ports := strconv.FormatUint(uint64(cport.ContainerPort), 10)

				target := model.LabelSet{
					model.AddressLabel:            lv(a),
					podContainerNameLabel:         lv(c.Name),
					podContainerPortNameLabel:     lv(cport.Name),
					podContainerPortNumberLabel:   lv(ports),
					podContainerPortProtocolLabel: lv(string(cport.Protocol)),
				}
				tg.Targets = append(tg.Targets, target.Merge(podLabels(pe.pod)))
			}
		}
	}

	return tg
}

func (e *Endpoints) resolvePodRef(ref *apiv1.ObjectReference) *apiv1.Pod {
	if ref == nil || ref.Kind != "Pod" {
		return nil
	}
	p := &apiv1.Pod{}
	p.Namespace = ref.Namespace
	p.Name = ref.Name

	obj, exists, err := e.podStore.Get(p)
	if err != nil || !exists {
		return nil
	}
	if err != nil {
		e.logger.With("err", err).Errorln("resolving pod ref failed")
	}
	return obj.(*apiv1.Pod)
}

func (e *Endpoints) addServiceLabels(ns, name string, tg *config.TargetGroup) {
	svc := &apiv1.Service{}
	svc.Namespace = ns
	svc.Name = name

	obj, exists, err := e.serviceStore.Get(svc)
	if !exists || err != nil {
		return
	}
	if err != nil {
		e.logger.With("err", err).Errorln("retrieving service failed")
	}
	svc = obj.(*apiv1.Service)

	tg.Labels = tg.Labels.Merge(serviceLabels(svc))
}
