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

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/cache"
)

func podStoreKeyFunc(obj interface{}) (string, error) {
	return obj.(*v1.Pod).ObjectMeta.Name, nil
}

func newFakePodInformer() *fakeInformer {
	return newFakeInformer(podStoreKeyFunc)
}

func makeTestPodDiscovery() (*Pod, *fakeInformer) {
	i := newFakePodInformer()
	return NewPod(log.Base(), i), i
}

func makeMultiPortPod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        "testpod",
			Namespace:   "default",
			Labels:      map[string]string{"testlabel": "testvalue"},
			Annotations: map[string]string{"testannotation": "testannotationvalue"},
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
		ObjectMeta: v1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
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

func TestPodDiscoveryInitial(t *testing.T) {
	n, i := makeTestPodDiscovery()
	i.GetStore().Add(makeMultiPortPod())

	k8sDiscoveryTest{
		discovery: n,
		expectedInitial: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__address__":                                   "1.2.3.4:9000",
						"__meta_kubernetes_pod_container_name":          "testcontainer0",
						"__meta_kubernetes_pod_container_port_name":     "testport0",
						"__meta_kubernetes_pod_container_port_number":   "9000",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
					},
					{
						"__address__":                                   "1.2.3.4:9001",
						"__meta_kubernetes_pod_container_name":          "testcontainer0",
						"__meta_kubernetes_pod_container_port_name":     "testport1",
						"__meta_kubernetes_pod_container_port_number":   "9001",
						"__meta_kubernetes_pod_container_port_protocol": "UDP",
					},
					{
						"__address__":                          "1.2.3.4",
						"__meta_kubernetes_pod_container_name": "testcontainer1",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_pod_name":                      "testpod",
					"__meta_kubernetes_namespace":                     "default",
					"__meta_kubernetes_pod_label_testlabel":           "testvalue",
					"__meta_kubernetes_pod_annotation_testannotation": "testannotationvalue",
					"__meta_kubernetes_pod_node_name":                 "testnode",
					"__meta_kubernetes_pod_ip":                        "1.2.3.4",
					"__meta_kubernetes_pod_host_ip":                   "2.3.4.5",
					"__meta_kubernetes_pod_ready":                     "true",
				},
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryAdd(t *testing.T) {
	n, i := makeTestPodDiscovery()

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Add(makePod()) }() },
		expectedRes: []*config.TargetGroup{
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
		expectedInitial: []*config.TargetGroup{
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
				},
				Source: "pod/default/testpod",
			},
		},
		expectedRes: []*config.TargetGroup{
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
		discovery:  n,
		afterStart: func() { go func() { i.Delete(cache.DeletedFinalStateUnknown{Obj: makePod()}) }() },
		expectedInitial: []*config.TargetGroup{
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
				},
				Source: "pod/default/testpod",
			},
		},
		expectedRes: []*config.TargetGroup{
			{
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}

func TestPodDiscoveryUpdate(t *testing.T) {
	n, i := makeTestPodDiscovery()
	i.GetStore().Add(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
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
		expectedInitial: []*config.TargetGroup{
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
					"__meta_kubernetes_pod_ready":     "unknown",
				},
				Source: "pod/default/testpod",
			},
		},
		expectedRes: []*config.TargetGroup{
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
				},
				Source: "pod/default/testpod",
			},
		},
	}.Run(t)
}
