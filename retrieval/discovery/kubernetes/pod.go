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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/strutil"
	"golang.org/x/net/context"
)

type podDiscovery struct {
	mtx           sync.RWMutex
	pods          map[string]map[string]*Pod
	retryInterval time.Duration
	kd            *Discovery
}

func (d *podDiscovery) run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	pods, _, err := d.getPods()
	if err != nil {
		log.Errorf("Cannot initialize pods collection: %s", err)
		return
	}
	d.pods = pods

	initial := []*config.TargetGroup{}
	switch d.kd.Conf.Role {
	case config.KubernetesRolePod:
		initial = append(initial, d.updatePodsTargetGroup())
	case config.KubernetesRoleContainer:
		for _, ns := range d.pods {
			for _, pod := range ns {
				initial = append(initial, d.updateContainerTargetGroup(pod))
			}
		}
	}

	select {
	case ch <- initial:
	case <-ctx.Done():
		return
	}

	update := make(chan *podEvent, 10)
	go d.watchPods(update, ctx.Done(), d.retryInterval)

	for {
		tgs := []*config.TargetGroup{}
		select {
		case <-ctx.Done():
			return
		case e := <-update:
			log.Debugf("k8s discovery received pod event (EventType=%s, Pod Name=%s)", e.EventType, e.Pod.ObjectMeta.Name)
			d.updatePod(e.Pod, e.EventType)

			switch d.kd.Conf.Role {
			case config.KubernetesRoleContainer:
				// Update the per-pod target group
				tgs = append(tgs, d.updateContainerTargetGroup(e.Pod))
			case config.KubernetesRolePod:
				// Update the all pods target group
				tgs = append(tgs, d.updatePodsTargetGroup())
			}
		}
		if tgs == nil {
			continue
		}

		for _, tg := range tgs {
			select {
			case ch <- []*config.TargetGroup{tg}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (d *podDiscovery) getPods() (map[string]map[string]*Pod, string, error) {
	res, err := d.kd.queryAPIServerPath(podsURL)
	if err != nil {
		return nil, "", fmt.Errorf("unable to list Kubernetes pods: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unable to list Kubernetes pods; unexpected response: %d %s", res.StatusCode, res.Status)
	}

	var pods PodList
	if err := json.NewDecoder(res.Body).Decode(&pods); err != nil {
		body, _ := ioutil.ReadAll(res.Body)
		return nil, "", fmt.Errorf("unable to list Kubernetes pods; unexpected response body: %s", string(body))
	}

	podMap := map[string]map[string]*Pod{}
	for idx, pod := range pods.Items {
		if _, ok := podMap[pod.ObjectMeta.Namespace]; !ok {
			podMap[pod.ObjectMeta.Namespace] = map[string]*Pod{}
		}
		log.Debugf("Got pod %s in namespace %s", pod.ObjectMeta.Name, pod.ObjectMeta.Namespace)
		podMap[pod.ObjectMeta.Namespace][pod.ObjectMeta.Name] = &pods.Items[idx]
	}

	return podMap, pods.ResourceVersion, nil
}

func (d *podDiscovery) watchPods(events chan *podEvent, done <-chan struct{}, retryInterval time.Duration) {
	until(func() {
		pods, resourceVersion, err := d.getPods()
		if err != nil {
			log.Errorf("Cannot initialize pods collection: %s", err)
			return
		}
		d.mtx.Lock()
		d.pods = pods
		d.mtx.Unlock()

		req, err := http.NewRequest("GET", podsURL, nil)
		if err != nil {
			log.Errorf("Cannot create pods request: %s", err)
			return
		}

		values := req.URL.Query()
		values.Add("watch", "true")
		values.Add("resourceVersion", resourceVersion)
		req.URL.RawQuery = values.Encode()
		res, err := d.kd.queryAPIServerReq(req)
		if err != nil {
			log.Errorf("Failed to watch pods: %s", err)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			log.Errorf("Failed to watch pods: %d", res.StatusCode)
			return
		}

		d := json.NewDecoder(res.Body)

		for {
			var event podEvent
			if err := d.Decode(&event); err != nil {
				log.Errorf("Watch pods unexpectedly closed: %s", err)
				return
			}

			select {
			case events <- &event:
			case <-done:
			}
		}
	}, retryInterval, done)
}

func (d *podDiscovery) updatePod(pod *Pod, eventType EventType) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	switch eventType {
	case Deleted:
		if _, ok := d.pods[pod.ObjectMeta.Namespace]; ok {
			delete(d.pods[pod.ObjectMeta.Namespace], pod.ObjectMeta.Name)
			if len(d.pods[pod.ObjectMeta.Namespace]) == 0 {
				delete(d.pods, pod.ObjectMeta.Namespace)
			}
		}
	case Added, Modified:
		if _, ok := d.pods[pod.ObjectMeta.Namespace]; !ok {
			d.pods[pod.ObjectMeta.Namespace] = map[string]*Pod{}
		}
		d.pods[pod.ObjectMeta.Namespace][pod.ObjectMeta.Name] = pod
	}
}

func (d *podDiscovery) updateContainerTargetGroup(pod *Pod) *config.TargetGroup {
	d.mtx.RLock()
	defer d.mtx.RUnlock()

	tg := &config.TargetGroup{
		Source: podSource(pod),
	}

	// If this pod doesn't exist, return an empty target group
	if _, ok := d.pods[pod.ObjectMeta.Namespace]; !ok {
		return tg
	}
	if _, ok := d.pods[pod.ObjectMeta.Namespace][pod.ObjectMeta.Name]; !ok {
		return tg
	}

	tg.Labels = model.LabelSet{
		roleLabel: model.LabelValue("container"),
	}
	tg.Targets = updatePodTargets(pod, true)

	return tg
}

func (d *podDiscovery) updatePodsTargetGroup() *config.TargetGroup {
	tg := &config.TargetGroup{
		Source: podsTargetGroupName,
		Labels: model.LabelSet{
			roleLabel: model.LabelValue("pod"),
		},
	}

	for _, namespace := range d.pods {
		for _, pod := range namespace {
			tg.Targets = append(tg.Targets, updatePodTargets(pod, false)...)
		}
	}

	return tg
}

func podSource(pod *Pod) string {
	return sourcePodPrefix + ":" + pod.ObjectMeta.Namespace + ":" + pod.ObjectMeta.Name
}

func updatePodTargets(pod *Pod, allContainers bool) []model.LabelSet {
	var targets []model.LabelSet = make([]model.LabelSet, 0, len(pod.PodSpec.Containers))
	if pod.PodStatus.PodIP == "" {
		log.Debugf("skipping pod %s -- PodStatus.PodIP is empty", pod.ObjectMeta.Name)
		return targets
	}

	if pod.PodStatus.Phase != "Running" {
		log.Debugf("skipping pod %s -- status is not `Running`", pod.ObjectMeta.Name)
		return targets
	}

	// Should never hit this (running pods should always have this set), but better to be defensive.
	if pod.PodStatus.HostIP == "" {
		log.Debugf("skipping pod %s -- PodStatus.HostIP is empty", pod.ObjectMeta.Name)
		return targets
	}

	ready := "unknown"
	for _, cond := range pod.PodStatus.Conditions {
		if strings.ToLower(cond.Type) == "ready" {
			ready = strings.ToLower(cond.Status)
		}
	}

	sort.Sort(ByContainerName(pod.PodSpec.Containers))

	for _, container := range pod.PodSpec.Containers {
		// Collect a list of TCP ports
		// Sort by port number, ascending
		// Product a target pointed at the first port
		// Include a label containing all ports (portName=port,PortName=port,...,)
		var tcpPorts []ContainerPort
		var portLabel *bytes.Buffer = bytes.NewBufferString(",")

		for _, port := range container.Ports {
			if port.Protocol == "TCP" {
				tcpPorts = append(tcpPorts, port)
			}
		}

		if len(tcpPorts) == 0 {
			log.Debugf("skipping container %s with no TCP ports", container.Name)
			continue
		}

		sort.Sort(ByContainerPort(tcpPorts))

		t := model.LabelSet{
			model.AddressLabel:        model.LabelValue(net.JoinHostPort(pod.PodIP, strconv.FormatInt(int64(tcpPorts[0].ContainerPort), 10))),
			podNameLabel:              model.LabelValue(pod.ObjectMeta.Name),
			podAddressLabel:           model.LabelValue(pod.PodStatus.PodIP),
			podNamespaceLabel:         model.LabelValue(pod.ObjectMeta.Namespace),
			podContainerNameLabel:     model.LabelValue(container.Name),
			podContainerPortNameLabel: model.LabelValue(tcpPorts[0].Name),
			podReadyLabel:             model.LabelValue(ready),
			podNodeNameLabel:          model.LabelValue(pod.PodSpec.NodeName),
			podHostIPLabel:            model.LabelValue(pod.PodStatus.HostIP),
		}

		for _, port := range tcpPorts {
			portLabel.WriteString(port.Name)
			portLabel.WriteString("=")
			portLabel.WriteString(strconv.FormatInt(int64(port.ContainerPort), 10))
			portLabel.WriteString(",")
			t[model.LabelName(podContainerPortMapPrefix+port.Name)] = model.LabelValue(strconv.FormatInt(int64(port.ContainerPort), 10))
		}

		t[model.LabelName(podContainerPortListLabel)] = model.LabelValue(portLabel.String())

		for k, v := range pod.ObjectMeta.Labels {
			labelName := strutil.SanitizeLabelName(podLabelPrefix + k)
			t[model.LabelName(labelName)] = model.LabelValue(v)
		}

		for k, v := range pod.ObjectMeta.Annotations {
			labelName := strutil.SanitizeLabelName(podAnnotationPrefix + k)
			t[model.LabelName(labelName)] = model.LabelValue(v)
		}

		targets = append(targets, t)

		if !allContainers {
			break
		}
	}

	if len(targets) == 0 {
		log.Debugf("no targets for pod %s", pod.ObjectMeta.Name)
	}

	return targets
}

type ByContainerPort []ContainerPort

func (a ByContainerPort) Len() int           { return len(a) }
func (a ByContainerPort) Less(i, j int) bool { return a[i].ContainerPort < a[j].ContainerPort }
func (a ByContainerPort) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

type ByContainerName []Container

func (a ByContainerName) Len() int           { return len(a) }
func (a ByContainerName) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a ByContainerName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
