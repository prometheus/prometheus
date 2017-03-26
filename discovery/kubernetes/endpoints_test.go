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

func endpointsStoreKeyFunc(obj interface{}) (string, error) {
	return obj.(*v1.Endpoints).ObjectMeta.Name, nil
}

func newFakeEndpointsInformer() *fakeInformer {
	return newFakeInformer(endpointsStoreKeyFunc)
}

func makeTestEndpointsDiscovery() (*Endpoints, *fakeInformer, *fakeInformer, *fakeInformer) {
	svc := newFakeServiceInformer()
	eps := newFakeEndpointsInformer()
	pod := newFakePodInformer()
	return NewEndpoints(log.Base(), svc, eps, pod), svc, eps, pod
}

func makeEndpoints() *v1.Endpoints {
	return &v1.Endpoints{
		ObjectMeta: v1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "1.2.3.4",
					},
				},
				Ports: []v1.EndpointPort{
					{
						Name:     "testport",
						Port:     9000,
						Protocol: v1.ProtocolTCP,
					},
				},
			},
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "2.3.4.5",
					},
				},
				NotReadyAddresses: []v1.EndpointAddress{
					{
						IP: "2.3.4.5",
					},
				},
				Ports: []v1.EndpointPort{
					{
						Name:     "testport",
						Port:     9001,
						Protocol: v1.ProtocolTCP,
					},
				},
			},
		},
	}
}

func TestEndpointsDiscoveryInitial(t *testing.T) {
	n, _, eps, _ := makeTestEndpointsDiscovery()
	eps.GetStore().Add(makeEndpoints())

	k8sDiscoveryTest{
		discovery: n,
		expectedInitial: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_port_name":     "testport",
						"__meta_kubernetes_endpoint_port_protocol": "TCP",
						"__meta_kubernetes_endpoint_ready":         "true",
					},
					{
						"__address__":                              "2.3.4.5:9001",
						"__meta_kubernetes_endpoint_port_name":     "testport",
						"__meta_kubernetes_endpoint_port_protocol": "TCP",
						"__meta_kubernetes_endpoint_ready":         "true",
					},
					{
						"__address__":                              "2.3.4.5:9001",
						"__meta_kubernetes_endpoint_port_name":     "testport",
						"__meta_kubernetes_endpoint_port_protocol": "TCP",
						"__meta_kubernetes_endpoint_ready":         "false",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":      "default",
					"__meta_kubernetes_endpoints_name": "testendpoints",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryAdd(t *testing.T) {
	n, _, eps, pods := makeTestEndpointsDiscovery()
	pods.GetStore().Add(&v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name: "c1",
					Ports: []v1.ContainerPort{
						{
							Name:          "mainport",
							ContainerPort: 9000,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
				{
					Name: "c2",
					Ports: []v1.ContainerPort{
						{
							Name:          "sideport",
							ContainerPort: 9001,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			HostIP: "2.3.4.5",
			PodIP:  "1.2.3.4",
		},
	})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			go func() {
				eps.Add(
					&v1.Endpoints{
						ObjectMeta: v1.ObjectMeta{
							Name:      "testendpoints",
							Namespace: "default",
						},
						Subsets: []v1.EndpointSubset{
							{
								Addresses: []v1.EndpointAddress{
									{
										IP: "4.3.2.1",
										TargetRef: &v1.ObjectReference{
											Kind:      "Pod",
											Name:      "testpod",
											Namespace: "default",
										},
									},
								},
								Ports: []v1.EndpointPort{
									{
										Name:     "testport",
										Port:     9000,
										Protocol: v1.ProtocolTCP,
									},
								},
							},
						},
					},
				)
			}()
		},
		expectedRes: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__address__":                                   "4.3.2.1:9000",
						"__meta_kubernetes_endpoint_port_name":          "testport",
						"__meta_kubernetes_endpoint_port_protocol":      "TCP",
						"__meta_kubernetes_endpoint_ready":              "true",
						"__meta_kubernetes_pod_name":                    "testpod",
						"__meta_kubernetes_pod_ip":                      "1.2.3.4",
						"__meta_kubernetes_pod_ready":                   "unknown",
						"__meta_kubernetes_pod_node_name":               "testnode",
						"__meta_kubernetes_pod_host_ip":                 "2.3.4.5",
						"__meta_kubernetes_pod_container_name":          "c1",
						"__meta_kubernetes_pod_container_port_name":     "mainport",
						"__meta_kubernetes_pod_container_port_number":   "9000",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
					},
					{
						"__address__":                                   "1.2.3.4:9001",
						"__meta_kubernetes_pod_name":                    "testpod",
						"__meta_kubernetes_pod_ip":                      "1.2.3.4",
						"__meta_kubernetes_pod_ready":                   "unknown",
						"__meta_kubernetes_pod_node_name":               "testnode",
						"__meta_kubernetes_pod_host_ip":                 "2.3.4.5",
						"__meta_kubernetes_pod_container_name":          "c2",
						"__meta_kubernetes_pod_container_port_name":     "sideport",
						"__meta_kubernetes_pod_container_port_number":   "9001",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpoints_name": "testendpoints",
					"__meta_kubernetes_namespace":      "default",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryDelete(t *testing.T) {
	n, _, eps, _ := makeTestEndpointsDiscovery()
	eps.GetStore().Add(makeEndpoints())

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { eps.Delete(makeEndpoints()) }() },
		expectedRes: []*config.TargetGroup{
			{
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryDeleteUnknownCacheState(t *testing.T) {
	n, _, eps, _ := makeTestEndpointsDiscovery()
	eps.GetStore().Add(makeEndpoints())

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { eps.Delete(cache.DeletedFinalStateUnknown{Obj: makeEndpoints()}) }() },
		expectedRes: []*config.TargetGroup{
			{
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryUpdate(t *testing.T) {
	n, _, eps, _ := makeTestEndpointsDiscovery()
	eps.GetStore().Add(makeEndpoints())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			go func() {
				eps.Update(&v1.Endpoints{
					ObjectMeta: v1.ObjectMeta{
						Name:      "testendpoints",
						Namespace: "default",
					},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								{
									IP: "1.2.3.4",
								},
							},
							Ports: []v1.EndpointPort{
								{
									Name:     "testport",
									Port:     9000,
									Protocol: v1.ProtocolTCP,
								},
							},
						},
						{
							Addresses: []v1.EndpointAddress{
								{
									IP: "2.3.4.5",
								},
							},
							Ports: []v1.EndpointPort{
								{
									Name:     "testport",
									Port:     9001,
									Protocol: v1.ProtocolTCP,
								},
							},
						},
					},
				})
			}()
		},
		expectedRes: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_port_name":     "testport",
						"__meta_kubernetes_endpoint_port_protocol": "TCP",
						"__meta_kubernetes_endpoint_ready":         "true",
					},
					{
						"__address__":                              "2.3.4.5:9001",
						"__meta_kubernetes_endpoint_port_name":     "testport",
						"__meta_kubernetes_endpoint_port_protocol": "TCP",
						"__meta_kubernetes_endpoint_ready":         "true",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":      "default",
					"__meta_kubernetes_endpoints_name": "testendpoints",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}
