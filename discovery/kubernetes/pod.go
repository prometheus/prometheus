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
	"net"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

// Pod discovers new pod targets.
type Pod struct {
	informer cache.SharedInformer
	store    cache.Store
	logger   log.Logger
	queue    *workqueue.Type
}

// NewPod creates a new pod discovery.
func NewPod(l log.Logger, pods cache.SharedInformer) *Pod {
	if l == nil {
		l = log.NewNopLogger()
	}
	p := &Pod{
		informer: pods,
		store:    pods.GetStore(),
		logger:   l,
		queue:    workqueue.NewNamed("pod"),
	}
	p.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("pod", "add").Inc()
			p.enqueue(o)
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("pod", "delete").Inc()
			p.enqueue(o)
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("pod", "update").Inc()
			p.enqueue(o)
		},
	})
	return p
}

func (p *Pod) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	p.queue.Add(key)
}

// Run implements the Discoverer interface.
func (p *Pod) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer p.queue.ShutDown()

	if !cache.WaitForCacheSync(ctx.Done(), p.informer.HasSynced) {
		if ctx.Err() != context.Canceled {
			level.Error(p.logger).Log("msg", "pod informer unable to sync cache")
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
	keyObj, quit := p.queue.Get()
	if quit {
		return false
	}
	defer p.queue.Done(keyObj)
	key := keyObj.(string)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return true
	}

	o, exists, err := p.store.GetByKey(key)
	if err != nil {
		return true
	}
	if !exists {
		send(ctx, p.logger, RolePod, ch, &targetgroup.Group{Source: podSourceFromNamespaceAndName(namespace, name)})
		return true
	}
	eps, err := convertToPod(o)
	if err != nil {
		level.Error(p.logger).Log("msg", "converting to Pod object failed", "err", err)
		return true
	}
	send(ctx, p.logger, RolePod, ch, p.buildPod(eps))
	return true
}

func convertToPod(o interface{}) (*apiv1.Pod, error) {
	pod, ok := o.(*apiv1.Pod)
	if ok {
		return pod, nil
	}

	return nil, errors.Errorf("received unexpected object: %v", o)
}

const (
	podNameLabel                  = metaLabelPrefix + "pod_name"
	podIPLabel                    = metaLabelPrefix + "pod_ip"
	podContainerNameLabel         = metaLabelPrefix + "pod_container_name"
	podContainerPortNameLabel     = metaLabelPrefix + "pod_container_port_name"
	podContainerPortNumberLabel   = metaLabelPrefix + "pod_container_port_number"
	podContainerPortProtocolLabel = metaLabelPrefix + "pod_container_port_protocol"
	podContainerIsInit            = metaLabelPrefix + "pod_container_init"
	podReadyLabel                 = metaLabelPrefix + "pod_ready"
	podPhaseLabel                 = metaLabelPrefix + "pod_phase"
	podLabelPrefix                = metaLabelPrefix + "pod_label_"
	podLabelPresentPrefix         = metaLabelPrefix + "pod_labelpresent_"
	podAnnotationPrefix           = metaLabelPrefix + "pod_annotation_"
	podAnnotationPresentPrefix    = metaLabelPrefix + "pod_annotationpresent_"
	podNodeNameLabel              = metaLabelPrefix + "pod_node_name"
	podHostIPLabel                = metaLabelPrefix + "pod_host_ip"
	podUID                        = metaLabelPrefix + "pod_uid"
	podControllerKind             = metaLabelPrefix + "pod_controller_kind"
	podControllerName             = metaLabelPrefix + "pod_controller_name"
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
		podNameLabel:     lv(pod.ObjectMeta.Name),
		podIPLabel:       lv(pod.Status.PodIP),
		podReadyLabel:    podReady(pod),
		podPhaseLabel:    lv(string(pod.Status.Phase)),
		podNodeNameLabel: lv(pod.Spec.NodeName),
		podHostIPLabel:   lv(pod.Status.HostIP),
		podUID:           lv(string(pod.ObjectMeta.UID)),
	}

	createdBy := GetControllerOf(pod)
	if createdBy != nil {
		if createdBy.Kind != "" {
			ls[podControllerKind] = lv(createdBy.Kind)
		}
		if createdBy.Name != "" {
			ls[podControllerName] = lv(createdBy.Name)
		}
	}

	for k, v := range pod.Labels {
		ln := strutil.SanitizeLabelName(k)
		ls[model.LabelName(podLabelPrefix+ln)] = lv(v)
		ls[model.LabelName(podLabelPresentPrefix+ln)] = presentValue
	}

	for k, v := range pod.Annotations {
		ln := strutil.SanitizeLabelName(k)
		ls[model.LabelName(podAnnotationPrefix+ln)] = lv(v)
		ls[model.LabelName(podAnnotationPresentPrefix+ln)] = presentValue
	}

	return ls
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

	containers := append(pod.Spec.Containers, pod.Spec.InitContainers...)
	for i, c := range containers {
		isInit := i >= len(pod.Spec.Containers)

		// If no ports are defined for the container, create an anonymous
		// target per container.
		if len(c.Ports) == 0 {
			// We don't have a port so we just set the address label to the pod IP.
			// The user has to add a port manually.
			tg.Targets = append(tg.Targets, model.LabelSet{
				model.AddressLabel:    lv(pod.Status.PodIP),
				podContainerNameLabel: lv(c.Name),
				podContainerIsInit:    lv(strconv.FormatBool(isInit)),
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
				podContainerPortNumberLabel:   lv(ports),
				podContainerPortNameLabel:     lv(port.Name),
				podContainerPortProtocolLabel: lv(string(port.Protocol)),
				podContainerIsInit:            lv(strconv.FormatBool(isInit)),
			})
		}
	}

	return tg
}

func podSource(pod *apiv1.Pod) string {
	return podSourceFromNamespaceAndName(pod.Namespace, pod.Name)
}

func podSourceFromNamespaceAndName(namespace, name string) string {
	return "pod/" + namespace + "/" + name
}

func podReady(pod *apiv1.Pod) model.LabelValue {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == apiv1.PodReady {
			return lv(strings.ToLower(string(cond.Status)))
		}
	}
	return lv(strings.ToLower(string(apiv1.ConditionUnknown)))
}
