// Copyright 2020 The Prometheus Authors
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/discovery/v1"
	"k8s.io/api/discovery/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

// EndpointSlice discovers new endpoint targets.
type EndpointSlice struct {
	logger log.Logger

	endpointSliceInf cache.SharedIndexInformer
	serviceInf       cache.SharedInformer
	podInf           cache.SharedInformer
	nodeInf          cache.SharedInformer
	withNodeMetadata bool

	podStore           cache.Store
	endpointSliceStore cache.Store
	serviceStore       cache.Store

	queue *workqueue.Type
}

// NewEndpointSlice returns a new endpointslice discovery.
func NewEndpointSlice(l log.Logger, eps cache.SharedIndexInformer, svc, pod, node cache.SharedInformer, eventCount *prometheus.CounterVec) *EndpointSlice {
	if l == nil {
		l = log.NewNopLogger()
	}

	epslAddCount := eventCount.WithLabelValues(RoleEndpointSlice.String(), MetricLabelRoleAdd)
	epslUpdateCount := eventCount.WithLabelValues(RoleEndpointSlice.String(), MetricLabelRoleUpdate)
	epslDeleteCount := eventCount.WithLabelValues(RoleEndpointSlice.String(), MetricLabelRoleDelete)

	svcAddCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleAdd)
	svcUpdateCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleUpdate)
	svcDeleteCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleDelete)

	e := &EndpointSlice{
		logger:             l,
		endpointSliceInf:   eps,
		endpointSliceStore: eps.GetStore(),
		serviceInf:         svc,
		serviceStore:       svc.GetStore(),
		podInf:             pod,
		podStore:           pod.GetStore(),
		nodeInf:            node,
		withNodeMetadata:   node != nil,
		queue:              workqueue.NewNamed(RoleEndpointSlice.String()),
	}

	_, err := e.endpointSliceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			epslAddCount.Inc()
			e.enqueue(o)
		},
		UpdateFunc: func(_, o interface{}) {
			epslUpdateCount.Inc()
			e.enqueue(o)
		},
		DeleteFunc: func(o interface{}) {
			epslDeleteCount.Inc()
			e.enqueue(o)
		},
	})
	if err != nil {
		level.Error(l).Log("msg", "Error adding endpoint slices event handler.", "err", err)
	}

	serviceUpdate := func(o interface{}) {
		svc, err := convertToService(o)
		if err != nil {
			level.Error(e.logger).Log("msg", "converting to Service object failed", "err", err)
			return
		}

		// TODO(brancz): use cache.Indexer to index endpoints by
		// disv1beta1.LabelServiceName so this operation doesn't have to
		// iterate over all endpoint objects.
		for _, obj := range e.endpointSliceStore.List() {
			esa, err := e.getEndpointSliceAdaptor(obj)
			if err != nil {
				level.Error(e.logger).Log("msg", "converting to EndpointSlice object failed", "err", err)
				continue
			}
			if lv, exists := esa.labels()[esa.labelServiceName()]; exists && lv == svc.Name {
				e.enqueue(esa.get())
			}
		}
	}
	_, err = e.serviceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			svcAddCount.Inc()
			serviceUpdate(o)
		},
		UpdateFunc: func(_, o interface{}) {
			svcUpdateCount.Inc()
			serviceUpdate(o)
		},
		DeleteFunc: func(o interface{}) {
			svcDeleteCount.Inc()
			serviceUpdate(o)
		},
	})
	if err != nil {
		level.Error(l).Log("msg", "Error adding services event handler.", "err", err)
	}

	if e.withNodeMetadata {
		_, err = e.nodeInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(o interface{}) {
				node := o.(*apiv1.Node)
				e.enqueueNode(node.Name)
			},
			UpdateFunc: func(_, o interface{}) {
				node := o.(*apiv1.Node)
				e.enqueueNode(node.Name)
			},
			DeleteFunc: func(o interface{}) {
				node := o.(*apiv1.Node)
				e.enqueueNode(node.Name)
			},
		})
		if err != nil {
			level.Error(l).Log("msg", "Error adding nodes event handler.", "err", err)
		}
	}

	return e
}

func (e *EndpointSlice) enqueueNode(nodeName string) {
	endpoints, err := e.endpointSliceInf.GetIndexer().ByIndex(nodeIndex, nodeName)
	if err != nil {
		level.Error(e.logger).Log("msg", "Error getting endpoints for node", "node", nodeName, "err", err)
		return
	}

	for _, endpoint := range endpoints {
		e.enqueue(endpoint)
	}
}

func (e *EndpointSlice) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	e.queue.Add(key)
}

// Run implements the Discoverer interface.
func (e *EndpointSlice) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer e.queue.ShutDown()

	cacheSyncs := []cache.InformerSynced{e.endpointSliceInf.HasSynced, e.serviceInf.HasSynced, e.podInf.HasSynced}
	if e.withNodeMetadata {
		cacheSyncs = append(cacheSyncs, e.nodeInf.HasSynced)
	}
	if !cache.WaitForCacheSync(ctx.Done(), cacheSyncs...) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			level.Error(e.logger).Log("msg", "endpointslice informer unable to sync cache")
		}
		return
	}

	go func() {
		for e.process(ctx, ch) {
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (e *EndpointSlice) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	keyObj, quit := e.queue.Get()
	if quit {
		return false
	}
	defer e.queue.Done(keyObj)
	key := keyObj.(string)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		level.Error(e.logger).Log("msg", "splitting key failed", "key", key)
		return true
	}

	o, exists, err := e.endpointSliceStore.GetByKey(key)
	if err != nil {
		level.Error(e.logger).Log("msg", "getting object from store failed", "key", key)
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: endpointSliceSourceFromNamespaceAndName(namespace, name)})
		return true
	}

	esa, err := e.getEndpointSliceAdaptor(o)
	if err != nil {
		level.Error(e.logger).Log("msg", "converting to EndpointSlice object failed", "err", err)
		return true
	}

	send(ctx, ch, e.buildEndpointSlice(esa))
	return true
}

func (e *EndpointSlice) getEndpointSliceAdaptor(o interface{}) (endpointSliceAdaptor, error) {
	switch endpointSlice := o.(type) {
	case *v1.EndpointSlice:
		return newEndpointSliceAdaptorFromV1(endpointSlice), nil
	case *v1beta1.EndpointSlice:
		return newEndpointSliceAdaptorFromV1beta1(endpointSlice), nil
	default:
		return nil, fmt.Errorf("received unexpected object: %v", o)
	}
}

func endpointSliceSource(ep endpointSliceAdaptor) string {
	return endpointSliceSourceFromNamespaceAndName(ep.namespace(), ep.name())
}

func endpointSliceSourceFromNamespaceAndName(namespace, name string) string {
	return "endpointslice/" + namespace + "/" + name
}

const (
	endpointSliceAddressTypeLabel                   = metaLabelPrefix + "endpointslice_address_type"
	endpointSlicePortNameLabel                      = metaLabelPrefix + "endpointslice_port_name"
	endpointSlicePortProtocolLabel                  = metaLabelPrefix + "endpointslice_port_protocol"
	endpointSlicePortLabel                          = metaLabelPrefix + "endpointslice_port"
	endpointSlicePortAppProtocol                    = metaLabelPrefix + "endpointslice_port_app_protocol"
	endpointSliceEndpointConditionsReadyLabel       = metaLabelPrefix + "endpointslice_endpoint_conditions_ready"
	endpointSliceEndpointConditionsServingLabel     = metaLabelPrefix + "endpointslice_endpoint_conditions_serving"
	endpointSliceEndpointConditionsTerminatingLabel = metaLabelPrefix + "endpointslice_endpoint_conditions_terminating"
	endpointSliceEndpointZoneLabel                  = metaLabelPrefix + "endpointslice_endpoint_zone"
	endpointSliceEndpointHostnameLabel              = metaLabelPrefix + "endpointslice_endpoint_hostname"
	endpointSliceEndpointNodenameLabel              = metaLabelPrefix + "endpointslice_endpoint_node_name"
	endpointSliceAddressTargetKindLabel             = metaLabelPrefix + "endpointslice_address_target_kind"
	endpointSliceAddressTargetNameLabel             = metaLabelPrefix + "endpointslice_address_target_name"
	endpointSliceEndpointTopologyLabelPrefix        = metaLabelPrefix + "endpointslice_endpoint_topology_"
	endpointSliceEndpointTopologyLabelPresentPrefix = metaLabelPrefix + "endpointslice_endpoint_topology_present_"
)

func (e *EndpointSlice) buildEndpointSlice(eps endpointSliceAdaptor) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: endpointSliceSource(eps),
	}
	tg.Labels = model.LabelSet{
		namespaceLabel:                lv(eps.namespace()),
		endpointSliceAddressTypeLabel: lv(eps.addressType()),
	}

	addObjectMetaLabels(tg.Labels, eps.getObjectMeta(), RoleEndpointSlice)

	e.addServiceLabels(eps, tg)

	type podEntry struct {
		pod          *apiv1.Pod
		servicePorts []endpointSlicePortAdaptor
	}
	seenPods := map[string]*podEntry{}

	add := func(addr string, ep endpointSliceEndpointAdaptor, port endpointSlicePortAdaptor) {
		a := addr
		if port.port() != nil {
			a = net.JoinHostPort(addr, strconv.FormatUint(uint64(*port.port()), 10))
		}

		target := model.LabelSet{
			model.AddressLabel: lv(a),
		}

		if port.name() != nil {
			target[endpointSlicePortNameLabel] = lv(*port.name())
		}

		if port.protocol() != nil {
			target[endpointSlicePortProtocolLabel] = lv(*port.protocol())
		}

		if port.port() != nil {
			target[endpointSlicePortLabel] = lv(strconv.FormatUint(uint64(*port.port()), 10))
		}

		if port.appProtocol() != nil {
			target[endpointSlicePortAppProtocol] = lv(*port.appProtocol())
		}

		if ep.conditions().ready() != nil {
			target[endpointSliceEndpointConditionsReadyLabel] = lv(strconv.FormatBool(*ep.conditions().ready()))
		}

		if ep.conditions().serving() != nil {
			target[endpointSliceEndpointConditionsServingLabel] = lv(strconv.FormatBool(*ep.conditions().serving()))
		}

		if ep.conditions().terminating() != nil {
			target[endpointSliceEndpointConditionsTerminatingLabel] = lv(strconv.FormatBool(*ep.conditions().terminating()))
		}

		if ep.hostname() != nil {
			target[endpointSliceEndpointHostnameLabel] = lv(*ep.hostname())
		}

		if ep.targetRef() != nil {
			target[model.LabelName(endpointSliceAddressTargetKindLabel)] = lv(ep.targetRef().Kind)
			target[model.LabelName(endpointSliceAddressTargetNameLabel)] = lv(ep.targetRef().Name)
		}

		if ep.nodename() != nil {
			target[endpointSliceEndpointNodenameLabel] = lv(*ep.nodename())
		}

		if ep.zone() != nil {
			target[model.LabelName(endpointSliceEndpointZoneLabel)] = lv(*ep.zone())
		}

		for k, v := range ep.topology() {
			ln := strutil.SanitizeLabelName(k)
			target[model.LabelName(endpointSliceEndpointTopologyLabelPrefix+ln)] = lv(v)
			target[model.LabelName(endpointSliceEndpointTopologyLabelPresentPrefix+ln)] = presentValue
		}

		if e.withNodeMetadata {
			if ep.targetRef() != nil && ep.targetRef().Kind == "Node" {
				target = addNodeLabels(target, e.nodeInf, e.logger, &ep.targetRef().Name)
			} else {
				target = addNodeLabels(target, e.nodeInf, e.logger, ep.nodename())
			}
		}

		pod := e.resolvePodRef(ep.targetRef())
		if pod == nil {
			// This target is not a Pod, so don't continue with Pod specific logic.
			tg.Targets = append(tg.Targets, target)
			return
		}
		s := namespacedName(pod.Namespace, pod.Name)

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
				if port.port() == nil {
					continue
				}
				if *port.port() == cport.ContainerPort {
					ports := strconv.FormatUint(uint64(*port.port()), 10)

					target[podContainerNameLabel] = lv(c.Name)
					target[podContainerImageLabel] = lv(c.Image)
					target[podContainerPortNameLabel] = lv(cport.Name)
					target[podContainerPortNumberLabel] = lv(ports)
					target[podContainerPortProtocolLabel] = lv(string(cport.Protocol))
					break
				}
			}
		}

		// Add service port so we know that we have already generated a target
		// for it.
		sp.servicePorts = append(sp.servicePorts, port)
		tg.Targets = append(tg.Targets, target)
	}

	for _, ep := range eps.endpoints() {
		for _, port := range eps.ports() {
			for _, addr := range ep.addresses() {
				add(addr, ep, port)
			}
		}
	}

	// For all seen pods, check all container ports. If they were not covered
	// by one of the service endpoints, generate targets for them.
	for _, pe := range seenPods {
		// PodIP can be empty when a pod is starting or has been evicted.
		if len(pe.pod.Status.PodIP) == 0 {
			continue
		}

		for _, c := range pe.pod.Spec.Containers {
			for _, cport := range c.Ports {
				hasSeenPort := func() bool {
					for _, eport := range pe.servicePorts {
						if eport.port() == nil {
							continue
						}
						if cport.ContainerPort == *eport.port() {
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
					podContainerImageLabel:        lv(c.Image),
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

func (e *EndpointSlice) resolvePodRef(ref *apiv1.ObjectReference) *apiv1.Pod {
	if ref == nil || ref.Kind != "Pod" {
		return nil
	}
	p := &apiv1.Pod{}
	p.Namespace = ref.Namespace
	p.Name = ref.Name

	obj, exists, err := e.podStore.Get(p)
	if err != nil {
		level.Error(e.logger).Log("msg", "resolving pod ref failed", "err", err)
		return nil
	}
	if !exists {
		return nil
	}
	return obj.(*apiv1.Pod)
}

func (e *EndpointSlice) addServiceLabels(esa endpointSliceAdaptor, tg *targetgroup.Group) {
	var (
		svc   = &apiv1.Service{}
		found bool
	)
	svc.Namespace = esa.namespace()

	// Every EndpointSlice object has the Service they belong to in the
	// kubernetes.io/service-name label.
	svc.Name, found = esa.labels()[esa.labelServiceName()]
	if !found {
		return
	}

	obj, exists, err := e.serviceStore.Get(svc)
	if err != nil {
		level.Error(e.logger).Log("msg", "retrieving service failed", "err", err)
		return
	}
	if !exists {
		return
	}
	svc = obj.(*apiv1.Service)

	tg.Labels = tg.Labels.Merge(serviceLabels(svc))
}
