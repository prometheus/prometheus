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
	"strings"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/net/context"
	"k8s.io/client-go/1.5/pkg/api"
	apiv1 "k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/cache"
)

// Pod discovers new pod targets.
type Pod struct {
	informer cache.SharedInformer
	store    cache.Store
	logger   log.Logger
}

// NewPod creates a new pod discovery.
func NewPod(l log.Logger, pods cache.SharedInformer) *Pod {
	return &Pod{
		informer: pods,
		store:    pods.GetStore(),
		logger:   l,
	}
}

// Run implements the TargetProvider interface.
func (p *Pod) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	// Send full initial set of pod targets.
	var initial []*config.TargetGroup
	for _, o := range p.store.List() {
		tg := p.buildPod(o.(*apiv1.Pod))
		initial = append(initial, tg)

		p.logger.With("tg", fmt.Sprintf("%#v", tg)).Debugln("initial pod")
	}
	select {
	case <-ctx.Done():
		return
	case ch <- initial:
	}

	// Send target groups for pod updates.
	send := func(tg *config.TargetGroup) {
		p.logger.With("tg", fmt.Sprintf("%#v", tg)).Debugln("pod update")
		select {
		case <-ctx.Done():
		case ch <- []*config.TargetGroup{tg}:
		}
	}
	p.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			eventCount.WithLabelValues("pod", "add").Inc()

			pod, err := convertToPod(o)
			if err != nil {
				p.logger.With("err", err).Errorln("converting to Pod object failed")
				return
			}
			send(p.buildPod(pod))
		},
		DeleteFunc: func(o interface{}) {
			eventCount.WithLabelValues("pod", "delete").Inc()

			pod, err := convertToPod(o)
			if err != nil {
				p.logger.With("err", err).Errorln("converting to Pod object failed")
				return
			}
			send(&config.TargetGroup{Source: podSource(pod)})
		},
		UpdateFunc: func(_, o interface{}) {
			eventCount.WithLabelValues("pod", "update").Inc()

			pod, err := convertToPod(o)
			if err != nil {
				p.logger.With("err", err).Errorln("converting to Pod object failed")
				return
			}
			send(p.buildPod(pod))
		},
	})

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func convertToPod(o interface{}) (*apiv1.Pod, error) {
	pod, isPod := o.(*apiv1.Pod)
	if !isPod {
		deletedState, ok := o.(cache.DeletedFinalStateUnknown)
		if !ok {
			return nil, fmt.Errorf("Received unexpected object: %v", o)
		}
		pod, ok = deletedState.Obj.(*apiv1.Pod)
		if !ok {
			return nil, fmt.Errorf("DeletedFinalStateUnknown contained non-Pod object: %v", deletedState.Obj)
		}
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
)

func podLabels(pod *apiv1.Pod) model.LabelSet {
	ls := model.LabelSet{
		podNameLabel:     lv(pod.ObjectMeta.Name),
		podIPLabel:       lv(pod.Status.PodIP),
		podReadyLabel:    podReady(pod),
		podNodeNameLabel: lv(pod.Spec.NodeName),
		podHostIPLabel:   lv(pod.Status.HostIP),
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

func (p *Pod) buildPod(pod *apiv1.Pod) *config.TargetGroup {
	// During startup the pod may not have an IP yet. This does not even allow
	// for an up metric, so we skip the target.
	if len(pod.Status.PodIP) == 0 {
		return nil
	}
	tg := &config.TargetGroup{
		Source: podSource(pod),
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
