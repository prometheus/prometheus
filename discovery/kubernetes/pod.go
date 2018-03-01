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
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"k8s.io/client-go/pkg/api"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

// Pod discovers new pod targets.
type Pod struct {
	informer cache.SharedInformer
	store    cache.Store
	logger   log.Logger
}

// NewPod creates a new pod discovery.
func NewPod(l log.Logger, pods cache.SharedInformer) *Pod {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &Pod{
		informer: pods,
		store:    pods.GetStore(),
		logger:   l,
	}
}

// Run implements the Discoverer interface.
func (p *Pod) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// Send full initial set of pod targets.
	var initial []*targetgroup.Group
	for _, o := range p.store.List() {
		tg := p.buildPod(o.(*apiv1.Pod))
		initial = append(initial, tg)

		level.Debug(p.logger).Log("msg", "initial pod", "tg", fmt.Sprintf("%#v", tg))
	}
	select {
	case <-ctx.Done():
		return
	case ch <- initial:
	}

	// Send target groups for pod updates.
	send := func(tg *targetgroup.Group) {
		if tg == nil {
			return
		}
		level.Debug(p.logger).Log("msg", "pod update", "tg", fmt.Sprintf("%#v", tg))
		select {
		case <-ctx.Done():
		case ch <- []*targetgroup.Group{tg}:
		}
	}
	p.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("pod", "add").Inc()

			pod, err := convertToPod(o)
			if err != nil {
				level.Error(p.logger).Log("msg", "converting to Pod object failed", "err", err)
				return
			}
			send(p.buildPod(pod))
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("pod", "delete").Inc()

			pod, err := convertToPod(o)
			if err != nil {
				level.Error(p.logger).Log("msg", "converting to Pod object failed", "err", err)
				return
			}
			send(&targetgroup.Group{Source: podSource(pod)})
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("pod", "update").Inc()

			pod, err := convertToPod(o)
			if err != nil {
				level.Error(p.logger).Log("msg", "converting to Pod object failed", "err", err)
				return
			}
			send(p.buildPod(pod))
		},
	})

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func convertToPod(o interface{}) (*apiv1.Pod, error) {
	pod, ok := o.(*apiv1.Pod)
	if ok {
		return pod, nil
	}

	deletedState, ok := o.(cache.DeletedFinalStateUnknown)
	if !ok {
		return nil, fmt.Errorf("Received unexpected object: %v", o)
	}
	pod, ok = deletedState.Obj.(*apiv1.Pod)
	if !ok {
		return nil, fmt.Errorf("DeletedFinalStateUnknown contained non-Pod object: %v", deletedState.Obj)
	}
	return pod, nil
}

const (
	podNameLabel                  = metaLabelPrefix + "pod_name"
	podIPLabel                    = metaLabelPrefix + "pod_ip"
	podContainerNameLabel         = metaLabelPrefix + "pod_container_name"
	podContainerPortNameLabel     = metaLabelPrefix + "pod_container_port_name"
	podContainerPortNumberLabel   = metaLabelPrefix + "pod_container_port_number"
	podContainerPortProtocolLabel = metaLabelPrefix + "pod_container_port_protocol"
	podReadyLabel                 = metaLabelPrefix + "pod_ready"
	podLabelPrefix                = metaLabelPrefix + "pod_label_"
	podAnnotationPrefix           = metaLabelPrefix + "pod_annotation_"
	podNodeNameLabel              = metaLabelPrefix + "pod_node_name"
	podHostIPLabel                = metaLabelPrefix + "pod_host_ip"
	podUID                        = metaLabelPrefix + "pod_uid"
)

func podLabels(pod *apiv1.Pod) model.LabelSet {
	ls := model.LabelSet{
		podNameLabel:     lv(pod.ObjectMeta.Name),
		podIPLabel:       lv(pod.Status.PodIP),
		podReadyLabel:    podReady(pod),
		podNodeNameLabel: lv(pod.Spec.NodeName),
		podHostIPLabel:   lv(pod.Status.HostIP),
		podUID:           lv(string(pod.ObjectMeta.UID)),
	}

	for k, v := range pod.Labels {
		ln := strutil.SanitizeLabelName(podLabelPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
	}

	for k, v := range pod.Annotations {
		ln := strutil.SanitizeLabelName(podAnnotationPrefix + k)
		ls[model.LabelName(ln)] = lv(v)
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

	for _, c := range pod.Spec.Containers {
		// If no ports are defined for the container, create an anonymous
		// target per container.
		if len(c.Ports) == 0 {
			// We don't have a port so we just set the address label to the pod IP.
			// The user has to add a port manually.
			tg.Targets = append(tg.Targets, model.LabelSet{
				model.AddressLabel:    lv(pod.Status.PodIP),
				podContainerNameLabel: lv(c.Name),
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
			})
		}
	}

	return tg
}

func podSource(pod *apiv1.Pod) string {
	return "pod/" + pod.Namespace + "/" + pod.Name
}

func podReady(pod *apiv1.Pod) model.LabelValue {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == apiv1.PodReady {
			return lv(strings.ToLower(string(cond.Status)))
		}
	}
	return lv(strings.ToLower(string(api.ConditionUnknown)))
}
