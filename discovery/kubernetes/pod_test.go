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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
)

func newFakePodInformer() *fakeInformer {
	return newFakeInformer(cache.DeletionHandlingMetaNamespaceKeyFunc)
}

func makeTestPodDiscovery() (*Pod, *fakeInformer) {
	i := newFakePodInformer()
	return NewPod(nil, i), i
}

func makeMultiPortPod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testpod",
			Namespace:   "default",
			Labels:      map[string]string{"testlabel": "testvalue"},
			Annotations: map[string]string{"testannotation": "testannotationvalue"},
			UID:         types.UID("abc123"),
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
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
}

func makePod() *v1.Pod {
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
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
}

func TestPodDiscoveryAdd(t *testing.T) {
	n, i := makeTestPodDiscovery()

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Add(makePod()) }() },
		expectedRes: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__":                                   "1.2.3.4:9000",
						"__meta_kubernetes_pod_container_name":          "testcontainer",
						"__meta_kubernetes_pod_container_port_name":     "testport",
						"__meta_kubernetes_pod_container_port_number":   "9000",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_pod_name":      "testpod",
					"__meta_kubernetes_namespace":     "default",
					"__meta_kubernetes_pod_node_name": "testnode",
					"__meta_kubernetes_pod_ip":        "1.2.3.4",
					"__meta_kubernetes_pod_host_ip":   "2.3.4.5",
					"__meta_kubernetes_pod_ready":     "true",
					"__meta_kubernetes_pod_uid":       "abc123",
				},
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryDelete(t *testing.T) {
	n, i := makeTestPodDiscovery()
	i.GetStore().Add(makePod())

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Delete(makePod()) }() },
		expectedRes: []*targetgroup.Group{
			{
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryDeleteUnknownCacheState(t *testing.T) {
	n, i := makeTestPodDiscovery()
	i.GetStore().Add(makePod())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			go func() {
				obj := makePod()
				key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
				if err != nil {
					t.Errorf("failed to get key for %v: %v", obj, err)
				}
				i.Delete(cache.DeletedFinalStateUnknown{Key: key, Obj: obj})
			}()
		},
		expectedRes: []*targetgroup.Group{
			{
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryUpdate(t *testing.T) {
	n, i := makeTestPodDiscovery()
	i.GetStore().Add(&v1.Pod{
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
	})

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Update(makePod()) }() },
		expectedRes: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__":                                   "1.2.3.4:9000",
						"__meta_kubernetes_pod_container_name":          "testcontainer",
						"__meta_kubernetes_pod_container_port_name":     "testport",
						"__meta_kubernetes_pod_container_port_number":   "9000",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_pod_name":      "testpod",
					"__meta_kubernetes_namespace":     "default",
					"__meta_kubernetes_pod_node_name": "testnode",
					"__meta_kubernetes_pod_ip":        "1.2.3.4",
					"__meta_kubernetes_pod_host_ip":   "2.3.4.5",
					"__meta_kubernetes_pod_ready":     "true",
					"__meta_kubernetes_pod_uid":       "abc123",
				},
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryUpdateEmptyPodIP(t *testing.T) {
	n, i := makeTestPodDiscovery()
	initialPod := makePod()

	updatedPod := makePod()
	updatedPod.Status.PodIP = ""

	i.GetStore().Add(initialPod)

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Update(updatedPod) }() },
		expectedInitial: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__":                                   "1.2.3.4:9000",
						"__meta_kubernetes_pod_container_name":          "testcontainer",
						"__meta_kubernetes_pod_container_port_name":     "testport",
						"__meta_kubernetes_pod_container_port_number":   "9000",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_pod_name":      "testpod",
					"__meta_kubernetes_namespace":     "default",
					"__meta_kubernetes_pod_node_name": "testnode",
					"__meta_kubernetes_pod_ip":        "1.2.3.4",
					"__meta_kubernetes_pod_host_ip":   "2.3.4.5",
					"__meta_kubernetes_pod_ready":     "true",
					"__meta_kubernetes_pod_uid":       "abc123",
				},
				Source: "pod/default/testpod",
			},
		},
		expectedRes: []*targetgroup.Group{
			{
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}
