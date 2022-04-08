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
	"testing"

	"github.com/prometheus/common/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func makeOptionalBool(v bool) *bool {
	return &v
}

func makeMultiPortPods() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testpod",
			Namespace:   "default",
			Labels:      map[string]string{"test/label": "testvalue"},
			Annotations: map[string]string{"test/annotation": "testannotationvalue"},
			UID:         types.UID("abc123"),
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "testcontrollerkind",
					Name:       "testcontrollername",
					Controller: makeOptionalBool(true),
				},
			},
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name: "testcontainer0",
					Ports: []v1.ContainerPort{
						{
							Name:          "testport0",
							Protocol:      v1.ProtocolTCP,
							ContainerPort: int32(9000),
						},
						{
							Name:          "testport1",
							Protocol:      v1.ProtocolUDP,
							ContainerPort: int32(9001),
						},
					},
				},
				{
					Name: "testcontainer1",
				},
			},
		},
		Status: v1.PodStatus{
			PodIP:  "1.2.3.4",
			HostIP: "2.3.4.5",
			Phase:  "Running",
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
}

func makePods() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
			UID:       types.UID("abc123"),
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name: "testcontainer",
					Ports: []v1.ContainerPort{
						{
							Name:          "testport",
							Protocol:      v1.ProtocolTCP,
							ContainerPort: int32(9000),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			PodIP:  "1.2.3.4",
			HostIP: "2.3.4.5",
			Phase:  "Running",
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
}

func makeInitContainerPods() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
			UID:       types.UID("abc123"),
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name: "testcontainer",
					Ports: []v1.ContainerPort{
						{
							Name:          "testport",
							Protocol:      v1.ProtocolTCP,
							ContainerPort: int32(9000),
						},
					},
				},
			},

			InitContainers: []v1.Container{
				{
					Name: "initcontainer",
				},
			},
		},
		Status: v1.PodStatus{
			PodIP:  "1.2.3.4",
			HostIP: "2.3.4.5",
			Phase:  "Pending",
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionFalse,
				},
			},
		},
	}
}

func expectedPodTargetGroups(ns string) map[string]*targetgroup.Group {
	key := fmt.Sprintf("pod/%s/testpod", ns)
	return map[string]*targetgroup.Group{
		key: {
			Targets: []model.LabelSet{
				{
					"__address__":                                   "1.2.3.4:9000",
					"__meta_kubernetes_pod_container_name":          "testcontainer",
					"__meta_kubernetes_pod_container_port_name":     "testport",
					"__meta_kubernetes_pod_container_port_number":   "9000",
					"__meta_kubernetes_pod_container_port_protocol": "TCP",
					"__meta_kubernetes_pod_container_init":          "false",
				},
			},
			Labels: model.LabelSet{
				"__meta_kubernetes_pod_name":      "testpod",
				"__meta_kubernetes_namespace":     lv(ns),
				"__meta_kubernetes_pod_node_name": "testnode",
				"__meta_kubernetes_pod_ip":        "1.2.3.4",
				"__meta_kubernetes_pod_host_ip":   "2.3.4.5",
				"__meta_kubernetes_pod_ready":     "true",
				"__meta_kubernetes_pod_phase":     "Running",
				"__meta_kubernetes_pod_uid":       "abc123",
			},
			Source: key,
		},
	}
}

func expectedPodTargetGroupsWithNodeMeta(ns, nodeName string, nodeLabels map[string]string) map[string]*targetgroup.Group {
	result := expectedPodTargetGroups(ns)
	for _, tg := range result {
		tg.Labels["__meta_kubernetes_node_name"] = lv(nodeName)
		for k, v := range nodeLabels {
			tg.Labels[model.LabelName("__meta_kubernetes_node_label_"+k)] = lv(v)
			tg.Labels[model.LabelName("__meta_kubernetes_node_labelpresent_"+k)] = lv("true")
		}
	}

	return result
}

func TestPodDiscoveryBeforeRun(t *testing.T) {
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := makeMultiPortPods()
			c.CoreV1().Pods(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"pod/default/testpod": {
				Targets: []model.LabelSet{
					{
						"__address__":                                   "1.2.3.4:9000",
						"__meta_kubernetes_pod_container_name":          "testcontainer0",
						"__meta_kubernetes_pod_container_port_name":     "testport0",
						"__meta_kubernetes_pod_container_port_number":   "9000",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
						"__meta_kubernetes_pod_container_init":          "false",
					},
					{
						"__address__":                                   "1.2.3.4:9001",
						"__meta_kubernetes_pod_container_name":          "testcontainer0",
						"__meta_kubernetes_pod_container_port_name":     "testport1",
						"__meta_kubernetes_pod_container_port_number":   "9001",
						"__meta_kubernetes_pod_container_port_protocol": "UDP",
						"__meta_kubernetes_pod_container_init":          "false",
					},
					{
						"__address__":                          "1.2.3.4",
						"__meta_kubernetes_pod_container_name": "testcontainer1",
						"__meta_kubernetes_pod_container_init": "false",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_pod_name":                              "testpod",
					"__meta_kubernetes_namespace":                             "default",
					"__meta_kubernetes_pod_label_test_label":                  "testvalue",
					"__meta_kubernetes_pod_labelpresent_test_label":           "true",
					"__meta_kubernetes_pod_annotation_test_annotation":        "testannotationvalue",
					"__meta_kubernetes_pod_annotationpresent_test_annotation": "true",
					"__meta_kubernetes_pod_node_name":                         "testnode",
					"__meta_kubernetes_pod_ip":                                "1.2.3.4",
					"__meta_kubernetes_pod_host_ip":                           "2.3.4.5",
					"__meta_kubernetes_pod_ready":                             "true",
					"__meta_kubernetes_pod_phase":                             "Running",
					"__meta_kubernetes_pod_uid":                               "abc123",
					"__meta_kubernetes_pod_controller_kind":                   "testcontrollerkind",
					"__meta_kubernetes_pod_controller_name":                   "testcontrollername",
				},
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryInitContainer(t *testing.T) {
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{})

	ns := "default"
	key := fmt.Sprintf("pod/%s/testpod", ns)
	expected := expectedPodTargetGroups(ns)
	expected[key].Targets = append(expected[key].Targets, model.LabelSet{
		"__address__":                          "1.2.3.4",
		"__meta_kubernetes_pod_container_name": "initcontainer",
		"__meta_kubernetes_pod_container_init": "true",
	})
	expected[key].Labels["__meta_kubernetes_pod_phase"] = "Pending"
	expected[key].Labels["__meta_kubernetes_pod_ready"] = "false"

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := makeInitContainerPods()
			c.CoreV1().Pods(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expected,
	}.Run(t)
}

func TestPodDiscoveryAdd(t *testing.T) {
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makePods()
			c.CoreV1().Pods(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedPodTargetGroups("default"),
	}.Run(t)
}

func TestPodDiscoveryDelete(t *testing.T) {
	obj := makePods()
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{}, obj)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makePods()
			c.CoreV1().Pods(obj.Namespace).Delete(context.Background(), obj.Name, metav1.DeleteOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"pod/default/testpod": {
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryUpdate(t *testing.T) {
	obj := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
			UID:       "xyz321",
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name: "testcontainer",
					Ports: []v1.ContainerPort{
						{
							Name:          "testport",
							Protocol:      v1.ProtocolTCP,
							ContainerPort: int32(9000),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			PodIP:  "1.2.3.4",
			HostIP: "2.3.4.5",
		},
	}
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{}, obj)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makePods()
			c.CoreV1().Pods(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes:      expectedPodTargetGroups("default"),
	}.Run(t)
}

func TestPodDiscoveryUpdateEmptyPodIP(t *testing.T) {
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{})
	initialPod := makePods()

	updatedPod := makePods()
	updatedPod.Status.PodIP = ""

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			c.CoreV1().Pods(initialPod.Namespace).Create(context.Background(), initialPod, metav1.CreateOptions{})
		},
		afterStart: func() {
			c.CoreV1().Pods(updatedPod.Namespace).Update(context.Background(), updatedPod, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"pod/default/testpod": {
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryNamespaces(t *testing.T) {
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{Names: []string{"ns1", "ns2"}})

	expected := expectedPodTargetGroups("ns1")
	for k, v := range expectedPodTargetGroups("ns2") {
		expected[k] = v
	}
	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				pod := makePods()
				pod.Namespace = ns
				c.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes:      expected,
	}.Run(t)
}

func TestPodDiscoveryOwnNamespace(t *testing.T) {
	n, c := makeDiscovery(RolePod, NamespaceDiscovery{IncludeOwnNamespace: true})

	expected := expectedPodTargetGroups("own-ns")
	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			for _, ns := range []string{"own-ns", "non-own-ns"} {
				pod := makePods()
				pod.Namespace = ns
				c.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 1,
		expectedRes:      expected,
	}.Run(t)
}

func TestPodDiscoveryWithNodeMetadata(t *testing.T) {
	attachMetadata := AttachMetadataConfig{Node: true}
	n, c := makeDiscoveryWithMetadata(RolePod, NamespaceDiscovery{}, attachMetadata)
	nodeLbls := map[string]string{"l1": "v1"}

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			nodes := makeNode("testnode", "", "", nodeLbls, nil)
			c.CoreV1().Nodes().Create(context.Background(), nodes, metav1.CreateOptions{})

			pods := makePods()
			c.CoreV1().Pods(pods.Namespace).Create(context.Background(), pods, metav1.CreateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes:      expectedPodTargetGroupsWithNodeMeta("default", "testnode", nodeLbls),
	}.Run(t)
}

func TestPodDiscoveryWithNodeMetadataUpdateNode(t *testing.T) {
	nodeLbls := map[string]string{"l2": "v2"}
	attachMetadata := AttachMetadataConfig{Node: true}
	n, c := makeDiscoveryWithMetadata(RolePod, NamespaceDiscovery{}, attachMetadata)

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			oldNodeLbls := map[string]string{"l1": "v1"}
			nodes := makeNode("testnode", "", "", oldNodeLbls, nil)
			c.CoreV1().Nodes().Create(context.Background(), nodes, metav1.CreateOptions{})
		},
		afterStart: func() {
			pods := makePods()
			c.CoreV1().Pods(pods.Namespace).Create(context.Background(), pods, metav1.CreateOptions{})

			nodes := makeNode("testnode", "", "", nodeLbls, nil)
			c.CoreV1().Nodes().Update(context.Background(), nodes, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes:      expectedPodTargetGroupsWithNodeMeta("default", "testnode", nodeLbls),
	}.Run(t)
}
