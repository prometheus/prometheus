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
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	nodeIndex = "node"
	podIndex  = "pod"
)

// Pod discovers new pod targets.
type Pod struct {
	podInf                cache.SharedIndexInformer
	nodeInf               cache.SharedInformer
	withNodeMetadata      bool
	namespaceInf          cache.SharedInformer
	withNamespaceMetadata bool
	store                 cache.Store
	logger                *slog.Logger
	queue                 *workqueue.Typed[string]
}

// NewPod creates a new pod discovery.
func NewPod(l *slog.Logger, pods cache.SharedIndexInformer, nodes, namespace cache.SharedInformer, eventCount *prometheus.CounterVec) *Pod {
	if l == nil {
		l = promslog.NewNopLogger()
	}

	podAddCount := eventCount.WithLabelValues(RolePod.String(), MetricLabelRoleAdd)
	podDeleteCount := eventCount.WithLabelValues(RolePod.String(), MetricLabelRoleDelete)
	podUpdateCount := eventCount.WithLabelValues(RolePod.String(), MetricLabelRoleUpdate)

	p := &Pod{
		podInf:                pods,
		nodeInf:               nodes,
		withNodeMetadata:      nodes != nil,
		namespaceInf:          namespace,
		withNamespaceMetadata: namespace != nil,
		store:                 pods.GetStore(),
		logger:                l,
		queue: workqueue.NewTypedWithConfig(workqueue.TypedQueueConfig[string]{
			Name: RolePod.String(),
		}),
	}
	_, err := p.podInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o any) {
			podAddCount.Inc()
			p.enqueue(o)
		},
		DeleteFunc: func(o any) {
			podDeleteCount.Inc()
			p.enqueue(o)
		},
		UpdateFunc: func(_, o any) {
			podUpdateCount.Inc()
			p.enqueue(o)
		},
	})
	if err != nil {
		l.Error("Error adding pods event handler.", "err", err)
	}

	if p.withNodeMetadata {
		_, err = p.nodeInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(o any) {
				node := o.(*apiv1.Node)
				p.enqueuePodsForNode(node.Name)
			},
			UpdateFunc: func(_, o any) {
				node := o.(*apiv1.Node)
				p.enqueuePodsForNode(node.Name)
			},
			DeleteFunc: func(o any) {
				nodeName, err := nodeName(o)
				if err != nil {
					l.Error("Error getting Node name", "err", err)
				}
				p.enqueuePodsForNode(nodeName)
			},
		})
		if err != nil {
			l.Error("Error adding pods event handler.", "err", err)
		}
	}

	if p.withNamespaceMetadata {
		_, err = p.namespaceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(_, o any) {
				namespace := o.(*apiv1.Namespace)
				p.enqueuePodsForNamespace(namespace.Name)
			},
			// Creation and deletion will trigger events for the change handlers of the resources within the namespace.
			// No need to have additional handlers for them here.
		})
		if err != nil {
			l.Error("Error adding namespaces event handler.", "err", err)
		}
	}

	return p
}

func (p *Pod) enqueue(obj any) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	p.queue.Add(key)
}

// Run implements the Discoverer interface.
func (p *Pod) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer p.queue.ShutDown()

	cacheSyncs := []cache.InformerSynced{p.podInf.HasSynced}
	if p.withNodeMetadata {
		cacheSyncs = append(cacheSyncs, p.nodeInf.HasSynced)
	}
	if p.withNamespaceMetadata {
		cacheSyncs = append(cacheSyncs, p.namespaceInf.HasSynced)
	}

	if !cache.WaitForCacheSync(ctx.Done(), cacheSyncs...) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			p.logger.Error("pod informer unable to sync cache")
		}
		return
	}

	go func() {
		for p.process(ctx, ch) {
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (p *Pod) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	key, quit := p.queue.Get()
	if quit {
		return false
	}
	defer p.queue.Done(key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return true
	}

	o, exists, err := p.store.GetByKey(key)
	if err != nil {
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: podSourceFromNamespaceAndName(namespace, name)})
		return true
	}
	pod, err := convertToPod(o)
	if err != nil {
		p.logger.Error("converting to Pod object failed", "err", err)
		return true
	}
	send(ctx, ch, p.buildPod(pod))
	return true
}

func convertToPod(o any) (*apiv1.Pod, error) {
	pod, ok := o.(*apiv1.Pod)
	if ok {
		return pod, nil
	}

	return nil, fmt.Errorf("received unexpected object: %v", o)
}

const (
	podIPLabel                    = metaLabelPrefix + "pod_ip"
	podContainerNameLabel         = metaLabelPrefix + "pod_container_name"
	podContainerIDLabel           = metaLabelPrefix + "pod_container_id"
	podContainerImageLabel        = metaLabelPrefix + "pod_container_image"
	podContainerPortNameLabel     = metaLabelPrefix + "pod_container_port_name"
	podContainerPortNumberLabel   = metaLabelPrefix + "pod_container_port_number"
	podContainerPortProtocolLabel = metaLabelPrefix + "pod_container_port_protocol"
	podContainerIsInit            = metaLabelPrefix + "pod_container_init"
	podReadyLabel                 = metaLabelPrefix + "pod_ready"
	podPhaseLabel                 = metaLabelPrefix + "pod_phase"
	podNodeNameLabel              = metaLabelPrefix + "pod_node_name"
	podHostIPLabel                = metaLabelPrefix + "pod_host_ip"
	podUID                        = metaLabelPrefix + "pod_uid"
	podControllerKind             = metaLabelPrefix + "pod_controller_kind"
	podControllerName             = metaLabelPrefix + "pod_controller_name"
	podIPv4Label                  = metaLabelPrefix + "pod_ipv4"
	podIPv6Label                  = metaLabelPrefix + "pod_ipv6"
)

// GetControllerOf returns a pointer to a copy of the controllerRef if controllee has a controller
// https://github.com/kubernetes/apimachinery/blob/cd2cae2b39fa57e8063fa1f5f13cfe9862db3d41/pkg/apis/meta/v1/controller_ref.go
func GetControllerOf(controllee metav1.Object) *metav1.OwnerReference {
	for _, ref := range controllee.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			return &ref
		}
	}
	return nil
}

func podLabels(pod *apiv1.Pod) model.LabelSet {
	ls := model.LabelSet{
		podIPLabel:       lv(pod.Status.PodIP),
		podReadyLabel:    podReady(pod),
		podPhaseLabel:    lv(string(pod.Status.Phase)),
		podNodeNameLabel: lv(pod.Spec.NodeName),
		podHostIPLabel:   lv(pod.Status.HostIP),
		podUID:           lv(string(pod.UID)),
	}

	// PodIPs contains all the IP addresses of the eth0 network interface in the Pod container.
	// The specific number of IP addresses depends on the specific cni plugin response, which means there may be more than two IP addresses.
	// However, in most cases, there is one IP address (single-stack; IPv4 or IPv6) or two IP addresses (dual-stack; IPv4 and IPv6 co-existing simultaneously).
	// If there are multiple IP addresses, use the first IPv4 and IPv6 respectively, this way, it can be consistent with the primary IP address (pod.Status.PodIP behavior) and avoid exposing redundant IP information.
	// Reference1: https://github.com/kubernetes/kubernetes/blob/c8fb7c9174b03ebc9a072913ca12c08ef3ed83c6/pkg/kubelet/kuberuntime/kuberuntime_sandbox.go#L317
	// Reference2: https://github.com/containerd/containerd/blob/b99b82db7d05dfd257edcdb50bc72769e782d6c2/internal/cri/server/sandbox_run.go#L572
	var ipv4Exists, ipv6Exists bool
	for _, podIP := range pod.Status.PodIPs {
		if podIP.IP == "" {
			continue
		}
		_, ipv4Exists = ls[podIPv4Label]
		_, ipv6Exists = ls[podIPv6Label]
		if ipv4Exists && ipv6Exists {
			break
		}
		ip := net.ParseIP(podIP.IP)
		if ip != nil {
			if ip.To4() != nil {
				if !ipv4Exists {
					ls[podIPv4Label] = lv(podIP.IP)
				}
			} else if ip.To16() != nil {
				if !ipv6Exists {
					ls[podIPv6Label] = lv(podIP.IP)
				}
			}
		}
	}

	addObjectMetaLabels(ls, pod.ObjectMeta, RolePod)

	createdBy := GetControllerOf(pod)
	if createdBy != nil {
		if createdBy.Kind != "" {
			ls[podControllerKind] = lv(createdBy.Kind)
		}
		if createdBy.Name != "" {
			ls[podControllerName] = lv(createdBy.Name)
		}
	}

	return ls
}

func (*Pod) findPodContainerStatus(statuses *[]apiv1.ContainerStatus, containerName string) (*apiv1.ContainerStatus, error) {
	for _, s := range *statuses {
		if s.Name == containerName {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("cannot find container with name %v", containerName)
}

func (p *Pod) findPodContainerID(statuses *[]apiv1.ContainerStatus, containerName string) string {
	cStatus, err := p.findPodContainerStatus(statuses, containerName)
	if err != nil {
		p.logger.Debug("cannot find container ID", "err", err)
		return ""
	}
	return cStatus.ContainerID
}

func (p *Pod) buildPod(pod *apiv1.Pod) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: podSource(pod),
	}
	// PodIP can be empty when a pod is starting or has been evicted.
	if len(pod.Status.PodIP) == 0 {
		return tg
	}

	tg.Labels = podLabels(pod)
	tg.Labels[namespaceLabel] = lv(pod.Namespace)
	if p.withNodeMetadata {
		tg.Labels = addNodeLabels(tg.Labels, p.nodeInf, p.logger, &pod.Spec.NodeName)
	}
	if p.withNamespaceMetadata {
		tg.Labels = addNamespaceLabels(tg.Labels, p.namespaceInf, p.logger, pod.Namespace)
	}

	containers := append(pod.Spec.Containers, pod.Spec.InitContainers...)
	for i, c := range containers {
		isInit := i >= len(pod.Spec.Containers)

		cStatuses := &pod.Status.ContainerStatuses
		if isInit {
			cStatuses = &pod.Status.InitContainerStatuses
		}
		cID := p.findPodContainerID(cStatuses, c.Name)

		// If no ports are defined for the container, create an anonymous
		// target per container.
		if len(c.Ports) == 0 {
			// We don't have a port so we just set the address label to the pod IP.
			// The user has to add a port manually.
			addr := pod.Status.PodIP
			ip := net.ParseIP(addr)
			if ip != nil && ip.To4() == nil {
				// If pod.Status.PodIP is a IPv6 address, addr should be "[pod.Status.PodIP]".
				// Contrary to what net.JoinHostPort does when the port is empty, RFC3986 forbids a ":" delimiter as the last character in such cases, leaving us with the following implementation.
				// Refer https://datatracker.ietf.org/doc/html/rfc3986#section-3.2.3
				addr = "[" + ip.String() + "]"
			}
			tg.Targets = append(tg.Targets, model.LabelSet{
				model.AddressLabel:     lv(addr),
				podContainerNameLabel:  lv(c.Name),
				podContainerIDLabel:    lv(cID),
				podContainerImageLabel: lv(c.Image),
				podContainerIsInit:     lv(strconv.FormatBool(isInit)),
			})
			continue
		}
		// Otherwise create one target for each container/port combination.
		for _, port := range c.Ports {
			ports := strconv.FormatUint(uint64(port.ContainerPort), 10)
			addr := net.JoinHostPort(pod.Status.PodIP, ports)

			tg.Targets = append(tg.Targets, model.LabelSet{
				model.AddressLabel:            lv(addr),
				podContainerNameLabel:         lv(c.Name),
				podContainerIDLabel:           lv(cID),
				podContainerImageLabel:        lv(c.Image),
				podContainerPortNumberLabel:   lv(ports),
				podContainerPortNameLabel:     lv(port.Name),
				podContainerPortProtocolLabel: lv(string(port.Protocol)),
				podContainerIsInit:            lv(strconv.FormatBool(isInit)),
			})
		}
	}

	return tg
}

func (p *Pod) enqueuePodsForNode(nodeName string) {
	pods, err := p.podInf.GetIndexer().ByIndex(nodeIndex, nodeName)
	if err != nil {
		p.logger.Error("Error getting pods for node", "node", nodeName, "err", err)
		return
	}

	for _, pod := range pods {
		p.enqueue(pod.(*apiv1.Pod))
	}
}

func (p *Pod) enqueuePodsForNamespace(namespace string) {
	pods, err := p.podInf.GetIndexer().ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		p.logger.Error("Error getting pods in namespace", "namespace", namespace, "err", err)
		return
	}

	for _, pod := range pods {
		p.enqueue(pod.(*apiv1.Pod))
	}
}

func podSource(pod *apiv1.Pod) string {
	return podSourceFromNamespaceAndName(pod.Namespace, pod.Name)
}

func podSourceFromNamespaceAndName(namespace, name string) string {
	return "pod/" + namespacedName(namespace, name)
}

func podReady(pod *apiv1.Pod) model.LabelValue {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == apiv1.PodReady {
			return lv(strings.ToLower(string(cond.Status)))
		}
	}
	return lv(strings.ToLower(string(apiv1.ConditionUnknown)))
}
