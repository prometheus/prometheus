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
	"log/slog"
	"net"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const serviceIndex = "service"

// EndpointSlice discovers new endpoint targets.
type EndpointSlice struct {
	logger *slog.Logger

	endpointSliceInf      cache.SharedIndexInformer
	serviceInf            cache.SharedInformer
	podInf                cache.SharedInformer
	nodeInf               cache.SharedInformer
	withNodeMetadata      bool
	namespaceInf          cache.SharedInformer
	withNamespaceMetadata bool

	podStore           cache.Store
	endpointSliceStore cache.Store
	serviceStore       cache.Store

	queue *workqueue.Typed[string]
}

// NewEndpointSlice returns a new endpointslice discovery.
func NewEndpointSlice(l *slog.Logger, eps cache.SharedIndexInformer, svc, pod, node, namespace cache.SharedInformer, eventCount *prometheus.CounterVec) *EndpointSlice {
	if l == nil {
		l = promslog.NewNopLogger()
	}

	epslAddCount := eventCount.WithLabelValues(RoleEndpointSlice.String(), MetricLabelRoleAdd)
	epslUpdateCount := eventCount.WithLabelValues(RoleEndpointSlice.String(), MetricLabelRoleUpdate)
	epslDeleteCount := eventCount.WithLabelValues(RoleEndpointSlice.String(), MetricLabelRoleDelete)

	svcAddCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleAdd)
	svcUpdateCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleUpdate)
	svcDeleteCount := eventCount.WithLabelValues(RoleService.String(), MetricLabelRoleDelete)

	e := &EndpointSlice{
		logger:                l,
		endpointSliceInf:      eps,
		endpointSliceStore:    eps.GetStore(),
		serviceInf:            svc,
		serviceStore:          svc.GetStore(),
		podInf:                pod,
		podStore:              pod.GetStore(),
		nodeInf:               node,
		withNodeMetadata:      node != nil,
		namespaceInf:          namespace,
		withNamespaceMetadata: namespace != nil,
		queue: workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{
			Name: RoleEndpointSlice.String(),
		}),
	}

	_, err := e.endpointSliceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o any) {
			epslAddCount.Inc()
			e.enqueue(o)
		},
		UpdateFunc: func(_, o any) {
			epslUpdateCount.Inc()
			e.enqueue(o)
		},
		DeleteFunc: func(o any) {
			epslDeleteCount.Inc()
			e.enqueue(o)
		},
	})
	if err != nil {
		l.Error("Error adding endpoint slices event handler.", "err", err)
	}

	serviceUpdate := func(o any) {
		svc, err := convertToService(o)
		if err != nil {
			e.logger.Error("converting to Service object failed", "err", err)
			return
		}

		endpointSlices, err := e.endpointSliceInf.GetIndexer().ByIndex(serviceIndex, namespacedName(svc.Namespace, svc.Name))
		if err != nil {
			e.logger.Error("getting endpoint slices by service name failed", "err", err)
			return
		}

		for _, endpointSlice := range endpointSlices {
			e.enqueue(endpointSlice)
		}
	}
	_, err = e.serviceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o any) {
			svcAddCount.Inc()
			serviceUpdate(o)
		},
		UpdateFunc: func(_, o any) {
			svcUpdateCount.Inc()
			serviceUpdate(o)
		},
		DeleteFunc: func(o any) {
			svcDeleteCount.Inc()
			serviceUpdate(o)
		},
	})
	if err != nil {
		l.Error("Error adding services event handler.", "err", err)
	}

	if e.withNodeMetadata {
		_, err = e.nodeInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(o any) {
				node := o.(*apiv1.Node)
				e.enqueueNode(node.Name)
			},
			UpdateFunc: func(_, o any) {
				node := o.(*apiv1.Node)
				e.enqueueNode(node.Name)
			},
			DeleteFunc: func(o any) {
				nodeName, err := nodeName(o)
				if err != nil {
					l.Error("Error getting Node name", "err", err)
				}
				e.enqueueNode(nodeName)
			},
		})
		if err != nil {
			l.Error("Error adding nodes event handler.", "err", err)
		}
	}

	if e.withNamespaceMetadata {
		_, err = e.namespaceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(_, o any) {
				namespace := o.(*apiv1.Namespace)
				e.enqueueNamespace(namespace.Name)
			},
			// Creation and deletion will trigger events for the change handlers of the resources within the namespace.
			// No need to have additional handlers for them here.
		})
		if err != nil {
			l.Error("Error adding namespaces event handler.", "err", err)
		}
	}

	return e
}

func (e *EndpointSlice) enqueueNode(nodeName string) {
	endpoints, err := e.endpointSliceInf.GetIndexer().ByIndex(nodeIndex, nodeName)
	if err != nil {
		e.logger.Error("Error getting endpoints for node", "node", nodeName, "err", err)
		return
	}

	for _, endpoint := range endpoints {
		e.enqueue(endpoint)
	}
}

func (e *EndpointSlice) enqueueNamespace(namespace string) {
	endpoints, err := e.endpointSliceInf.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		e.logger.Error("Error getting endpoints in namespace", "namespace", namespace, "err", err)
		return
	}

	for _, endpoint := range endpoints {
		e.enqueue(endpoint)
	}
}

func (e *EndpointSlice) enqueue(obj any) {
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
	if e.withNamespaceMetadata {
		cacheSyncs = append(cacheSyncs, e.namespaceInf.HasSynced)
	}
	if !cache.WaitForCacheSync(ctx.Done(), cacheSyncs...) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			e.logger.Error("endpointslice informer unable to sync cache")
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
	key, quit := e.queue.Get()
	if quit {
		return false
	}
	defer e.queue.Done(key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		e.logger.Error("splitting key failed", "key", key)
		return true
	}

	o, exists, err := e.endpointSliceStore.GetByKey(key)
	if err != nil {
		e.logger.Error("getting object from store failed", "key", key)
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: endpointSliceSourceFromNamespaceAndName(namespace, name)})
		return true
	}

	if es, ok := o.(*v1.EndpointSlice); ok {
		send(ctx, ch, e.buildEndpointSlice(*es))
	} else {
		e.logger.Error("received unexpected object", "object", o)
		return false
	}
	return true
}

func endpointSliceSource(ep v1.EndpointSlice) string {
	return endpointSliceSourceFromNamespaceAndName(ep.Namespace, ep.Name)
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

func (e *EndpointSlice) buildEndpointSlice(eps v1.EndpointSlice) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: endpointSliceSource(eps),
	}
	tg.Labels = model.LabelSet{
		namespaceLabel:                lv(eps.Namespace),
		endpointSliceAddressTypeLabel: lv(string(eps.AddressType)),
	}

	addObjectMetaLabels(tg.Labels, eps.ObjectMeta, RoleEndpointSlice)

	e.addServiceLabels(eps, tg)

	if e.withNamespaceMetadata {
		tg.Labels = addNamespaceLabels(tg.Labels, e.namespaceInf, e.logger, eps.Namespace)
	}

	type podEntry struct {
		pod          *apiv1.Pod
		servicePorts []v1.EndpointPort
	}
	seenPods := map[string]*podEntry{}

	add := func(addr string, ep v1.Endpoint, port v1.EndpointPort) {
		a := addr
		if port.Port != nil {
			a = net.JoinHostPort(addr, strconv.FormatUint(uint64(*port.Port), 10))
		}

		target := model.LabelSet{
			model.AddressLabel: lv(a),
		}

		if port.Name != nil {
			target[endpointSlicePortNameLabel] = lv(*port.Name)
		}

		if port.Protocol != nil {
			target[endpointSlicePortProtocolLabel] = lv(string(*port.Protocol))
		}

		if port.Port != nil {
			target[endpointSlicePortLabel] = lv(strconv.FormatUint(uint64(*port.Port), 10))
		}

		if port.AppProtocol != nil {
			target[endpointSlicePortAppProtocol] = lv(*port.AppProtocol)
		}

		if ep.Conditions.Ready != nil {
			target[endpointSliceEndpointConditionsReadyLabel] = lv(strconv.FormatBool(*ep.Conditions.Ready))
		}

		if ep.Conditions.Serving != nil {
			target[endpointSliceEndpointConditionsServingLabel] = lv(strconv.FormatBool(*ep.Conditions.Serving))
		}

		if ep.Conditions.Terminating != nil {
			target[endpointSliceEndpointConditionsTerminatingLabel] = lv(strconv.FormatBool(*ep.Conditions.Terminating))
		}

		if ep.Hostname != nil {
			target[endpointSliceEndpointHostnameLabel] = lv(*ep.Hostname)
		}

		if ep.TargetRef != nil {
			target[model.LabelName(endpointSliceAddressTargetKindLabel)] = lv(ep.TargetRef.Kind)
			target[model.LabelName(endpointSliceAddressTargetNameLabel)] = lv(ep.TargetRef.Name)
		}

		if ep.NodeName != nil {
			target[endpointSliceEndpointNodenameLabel] = lv(*ep.NodeName)
		}

		if ep.Zone != nil {
			target[model.LabelName(endpointSliceEndpointZoneLabel)] = lv(*ep.Zone)
		}

		for k, v := range ep.DeprecatedTopology {
			ln := strutil.SanitizeLabelName(k)
			target[model.LabelName(endpointSliceEndpointTopologyLabelPrefix+ln)] = lv(v)
			target[model.LabelName(endpointSliceEndpointTopologyLabelPresentPrefix+ln)] = presentValue
		}

		if e.withNodeMetadata {
			if ep.TargetRef != nil && ep.TargetRef.Kind == "Node" {
				target = addNodeLabels(target, e.nodeInf, e.logger, &ep.TargetRef.Name)
			} else {
				target = addNodeLabels(target, e.nodeInf, e.logger, ep.NodeName)
			}
		}

		pod := e.resolvePodRef(ep.TargetRef)
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
		containers := append(pod.Spec.Containers, pod.Spec.InitContainers...)
		for i, c := range containers {
			for _, cport := range c.Ports {
				if port.Port == nil {
					continue
				}

				if *port.Port == cport.ContainerPort {
					ports := strconv.FormatUint(uint64(*port.Port), 10)
					isInit := i >= len(pod.Spec.Containers)

					target[podContainerNameLabel] = lv(c.Name)
					target[podContainerImageLabel] = lv(c.Image)
					target[podContainerPortNameLabel] = lv(cport.Name)
					target[podContainerPortNumberLabel] = lv(ports)
					target[podContainerPortProtocolLabel] = lv(string(cport.Protocol))
					target[podContainerIsInit] = lv(strconv.FormatBool(isInit))
					break
				}
			}
		}

		// Add service port so we know that we have already generated a target
		// for it.
		sp.servicePorts = append(sp.servicePorts, port)
		tg.Targets = append(tg.Targets, target)
	}

	for _, ep := range eps.Endpoints {
		for _, port := range eps.Ports {
			for _, addr := range ep.Addresses {
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

		containers := append(pe.pod.Spec.Containers, pe.pod.Spec.InitContainers...)
		for i, c := range containers {
			for _, cport := range c.Ports {
				hasSeenPort := func() bool {
					for _, eport := range pe.servicePorts {
						if eport.Port == nil {
							continue
						}
						if cport.ContainerPort == *eport.Port {
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

				isInit := i >= len(pe.pod.Spec.Containers)
				target := model.LabelSet{
					model.AddressLabel:            lv(a),
					podContainerNameLabel:         lv(c.Name),
					podContainerImageLabel:        lv(c.Image),
					podContainerPortNameLabel:     lv(cport.Name),
					podContainerPortNumberLabel:   lv(ports),
					podContainerPortProtocolLabel: lv(string(cport.Protocol)),
					podContainerIsInit:            lv(strconv.FormatBool(isInit)),
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

	obj, exists, err := e.podStore.GetByKey(namespacedName(ref.Namespace, ref.Name))
	if err != nil {
		e.logger.Error("resolving pod ref failed", "err", err)
		return nil
	}
	if !exists {
		return nil
	}
	return obj.(*apiv1.Pod)
}

func (e *EndpointSlice) addServiceLabels(esa v1.EndpointSlice, tg *targetgroup.Group) {
	var (
		found bool
		name  string
	)
	ns := esa.Namespace

	// Every EndpointSlice object has the Service they belong to in the
	// kubernetes.io/service-name label.
	name, found = esa.Labels[v1.LabelServiceName]
	if !found {
		return
	}

	obj, exists, err := e.serviceStore.GetByKey(namespacedName(ns, name))
	if err != nil {
		e.logger.Error("retrieving service failed", "err", err)
		return
	}
	if !exists {
		return
	}
	svc := obj.(*apiv1.Service)

	tg.Labels = tg.Labels.Merge(serviceLabels(svc))
}
