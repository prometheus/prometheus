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
	"testing"

	"github.com/prometheus/common/model"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func makeEndpoints() *v1.Endpoints {
	nodeName := "foobar"
	return &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
			Annotations: map[string]string{
				"test.annotation": "test",
			},
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP:       "1.2.3.4",
						Hostname: "testendpoint1",
						NodeName: &nodeName,
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
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "6.7.8.9",
						TargetRef: &v1.ObjectReference{
							Kind: "Node",
							Name: "barbaz",
						},
					},
				},
				Ports: []v1.EndpointPort{
					{
						Name:     "testport",
						Port:     9002,
						Protocol: v1.ProtocolTCP,
					},
				},
			},
		},
	}
}

func TestEndpointsDiscoveryBeforeRun(t *testing.T) {
	n, c := makeDiscovery(RoleEndpoint, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := makeEndpoints()
			c.CoreV1().Endpoints(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_hostname":      "testendpoint1",
						"__meta_kubernetes_endpoint_node_name":     "foobar",
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
					{
						"__address__": "6.7.8.9:9002",
						"__meta_kubernetes_endpoint_address_target_kind": "Node",
						"__meta_kubernetes_endpoint_address_target_name": "barbaz",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                   "default",
					"__meta_kubernetes_endpoints_name":                              "testendpoints",
					"__meta_kubernetes_endpoints_annotation_test_annotation":        "test",
					"__meta_kubernetes_endpoints_annotationpresent_test_annotation": "true",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryAdd(t *testing.T) {
	obj := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
			UID:       types.UID("deadbeef"),
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name:  "c1",
					Image: "c1:latest",
					Ports: []v1.ContainerPort{
						{
							Name:          "mainport",
							ContainerPort: 9000,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
				{
					Name:  "c2",
					Image: "c2:latest",
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
	}
	n, c := makeDiscovery(RoleEndpoint, NamespaceDiscovery{}, obj)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
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
			}
			c.CoreV1().Endpoints(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                                    "4.3.2.1:9000",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
						"__meta_kubernetes_endpoint_address_target_kind": "Pod",
						"__meta_kubernetes_endpoint_address_target_name": "testpod",
						"__meta_kubernetes_pod_name":                     "testpod",
						"__meta_kubernetes_pod_ip":                       "1.2.3.4",
						"__meta_kubernetes_pod_ready":                    "unknown",
						"__meta_kubernetes_pod_phase":                    "",
						"__meta_kubernetes_pod_node_name":                "testnode",
						"__meta_kubernetes_pod_host_ip":                  "2.3.4.5",
						"__meta_kubernetes_pod_container_name":           "c1",
						"__meta_kubernetes_pod_container_image":          "c1:latest",
						"__meta_kubernetes_pod_container_port_name":      "mainport",
						"__meta_kubernetes_pod_container_port_number":    "9000",
						"__meta_kubernetes_pod_container_port_protocol":  "TCP",
						"__meta_kubernetes_pod_uid":                      "deadbeef",
					},
					{
						"__address__":                                   "1.2.3.4:9001",
						"__meta_kubernetes_pod_name":                    "testpod",
						"__meta_kubernetes_pod_ip":                      "1.2.3.4",
						"__meta_kubernetes_pod_ready":                   "unknown",
						"__meta_kubernetes_pod_phase":                   "",
						"__meta_kubernetes_pod_node_name":               "testnode",
						"__meta_kubernetes_pod_host_ip":                 "2.3.4.5",
						"__meta_kubernetes_pod_container_name":          "c2",
						"__meta_kubernetes_pod_container_image":         "c2:latest",
						"__meta_kubernetes_pod_container_port_name":     "sideport",
						"__meta_kubernetes_pod_container_port_number":   "9001",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
						"__meta_kubernetes_pod_uid":                     "deadbeef",
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
	n, c := makeDiscovery(RoleEndpoint, NamespaceDiscovery{}, makeEndpoints())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeEndpoints()
			c.CoreV1().Endpoints(obj.Namespace).Delete(context.Background(), obj.Name, metav1.DeleteOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryUpdate(t *testing.T) {
	n, c := makeDiscovery(RoleEndpoint, NamespaceDiscovery{}, makeEndpoints())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
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
			}
			c.CoreV1().Endpoints(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
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

func TestEndpointsDiscoveryEmptySubsets(t *testing.T) {
	n, c := makeDiscovery(RoleEndpoint, NamespaceDiscovery{}, makeEndpoints())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
				},
				Subsets: []v1.EndpointSubset{},
			}
			c.CoreV1().Endpoints(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":      "default",
					"__meta_kubernetes_endpoints_name": "testendpoints",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryWithService(t *testing.T) {
	n, c := makeDiscovery(RoleEndpoint, NamespaceDiscovery{}, makeEndpoints())

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
					Labels: map[string]string{
						"app/name": "test",
					},
				},
			}
			c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_hostname":      "testendpoint1",
						"__meta_kubernetes_endpoint_node_name":     "foobar",
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
					{
						"__address__": "6.7.8.9:9002",
						"__meta_kubernetes_endpoint_address_target_kind": "Node",
						"__meta_kubernetes_endpoint_address_target_name": "barbaz",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                   "default",
					"__meta_kubernetes_endpoints_name":                              "testendpoints",
					"__meta_kubernetes_service_label_app_name":                      "test",
					"__meta_kubernetes_service_labelpresent_app_name":               "true",
					"__meta_kubernetes_service_name":                                "testendpoints",
					"__meta_kubernetes_endpoints_annotation_test_annotation":        "test",
					"__meta_kubernetes_endpoints_annotationpresent_test_annotation": "true",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryWithServiceUpdate(t *testing.T) {
	n, c := makeDiscovery(RoleEndpoint, NamespaceDiscovery{}, makeEndpoints())

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
					Labels: map[string]string{
						"app/name": "test",
					},
				},
			}
			c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		afterStart: func() {
			obj := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
					Labels: map[string]string{
						"app/name":  "svc",
						"component": "testing",
					},
				},
			}
			c.CoreV1().Services(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_hostname":      "testendpoint1",
						"__meta_kubernetes_endpoint_node_name":     "foobar",
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
					{
						"__address__": "6.7.8.9:9002",
						"__meta_kubernetes_endpoint_address_target_kind": "Node",
						"__meta_kubernetes_endpoint_address_target_name": "barbaz",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                   "default",
					"__meta_kubernetes_endpoints_name":                              "testendpoints",
					"__meta_kubernetes_service_label_app_name":                      "svc",
					"__meta_kubernetes_service_labelpresent_app_name":               "true",
					"__meta_kubernetes_service_name":                                "testendpoints",
					"__meta_kubernetes_service_label_component":                     "testing",
					"__meta_kubernetes_service_labelpresent_component":              "true",
					"__meta_kubernetes_endpoints_annotation_test_annotation":        "test",
					"__meta_kubernetes_endpoints_annotationpresent_test_annotation": "true",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryWithNodeMetadata(t *testing.T) {
	metadataConfig := AttachMetadataConfig{Node: true}
	nodeLabels1 := map[string]string{"az": "us-east1"}
	nodeLabels2 := map[string]string{"az": "us-west2"}
	node1 := makeNode("foobar", "", "", nodeLabels1, nil)
	node2 := makeNode("barbaz", "", "", nodeLabels2, nil)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
			Labels: map[string]string{
				"app/name": "test",
			},
		},
	}
	n, _ := makeDiscoveryWithMetadata(RoleEndpoint, NamespaceDiscovery{}, metadataConfig, makeEndpoints(), svc, node1, node2)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_hostname":      "testendpoint1",
						"__meta_kubernetes_endpoint_node_name":     "foobar",
						"__meta_kubernetes_endpoint_port_name":     "testport",
						"__meta_kubernetes_endpoint_port_protocol": "TCP",
						"__meta_kubernetes_endpoint_ready":         "true",
						"__meta_kubernetes_node_label_az":          "us-east1",
						"__meta_kubernetes_node_labelpresent_az":   "true",
						"__meta_kubernetes_node_name":              "foobar",
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
					{
						"__address__": "6.7.8.9:9002",
						"__meta_kubernetes_endpoint_address_target_kind": "Node",
						"__meta_kubernetes_endpoint_address_target_name": "barbaz",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
						"__meta_kubernetes_node_label_az":                "us-west2",
						"__meta_kubernetes_node_labelpresent_az":         "true",
						"__meta_kubernetes_node_name":                    "barbaz",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                   "default",
					"__meta_kubernetes_endpoints_name":                              "testendpoints",
					"__meta_kubernetes_service_label_app_name":                      "test",
					"__meta_kubernetes_service_labelpresent_app_name":               "true",
					"__meta_kubernetes_service_name":                                "testendpoints",
					"__meta_kubernetes_endpoints_annotation_test_annotation":        "test",
					"__meta_kubernetes_endpoints_annotationpresent_test_annotation": "true",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryWithUpdatedNodeMetadata(t *testing.T) {
	nodeLabels1 := map[string]string{"az": "us-east1"}
	nodeLabels2 := map[string]string{"az": "us-west2"}
	node1 := makeNode("foobar", "", "", nodeLabels1, nil)
	node2 := makeNode("barbaz", "", "", nodeLabels2, nil)
	metadataConfig := AttachMetadataConfig{Node: true}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
			Labels: map[string]string{
				"app/name": "test",
			},
		},
	}
	n, c := makeDiscoveryWithMetadata(RoleEndpoint, NamespaceDiscovery{}, metadataConfig, makeEndpoints(), node1, node2, svc)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			node1.Labels["az"] = "eu-central1"
			c.CoreV1().Nodes().Update(context.Background(), node1, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_hostname":      "testendpoint1",
						"__meta_kubernetes_endpoint_node_name":     "foobar",
						"__meta_kubernetes_endpoint_port_name":     "testport",
						"__meta_kubernetes_endpoint_port_protocol": "TCP",
						"__meta_kubernetes_endpoint_ready":         "true",
						"__meta_kubernetes_node_label_az":          "us-east1",
						"__meta_kubernetes_node_labelpresent_az":   "true",
						"__meta_kubernetes_node_name":              "foobar",
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
					{
						"__address__": "6.7.8.9:9002",
						"__meta_kubernetes_endpoint_address_target_kind": "Node",
						"__meta_kubernetes_endpoint_address_target_name": "barbaz",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
						"__meta_kubernetes_node_label_az":                "us-west2",
						"__meta_kubernetes_node_labelpresent_az":         "true",
						"__meta_kubernetes_node_name":                    "barbaz",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                   "default",
					"__meta_kubernetes_endpoints_name":                              "testendpoints",
					"__meta_kubernetes_service_label_app_name":                      "test",
					"__meta_kubernetes_service_labelpresent_app_name":               "true",
					"__meta_kubernetes_service_name":                                "testendpoints",
					"__meta_kubernetes_endpoints_annotation_test_annotation":        "test",
					"__meta_kubernetes_endpoints_annotationpresent_test_annotation": "true",
				},
				Source: "endpoints/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryNamespaces(t *testing.T) {
	epOne := makeEndpoints()
	epOne.Namespace = "ns1"
	objs := []runtime.Object{
		epOne,
		&v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testendpoints",
				Namespace: "ns2",
			},
			Subsets: []v1.EndpointSubset{
				{
					Addresses: []v1.EndpointAddress{
						{
							IP: "4.3.2.1",
							TargetRef: &v1.ObjectReference{
								Kind:      "Pod",
								Name:      "testpod",
								Namespace: "ns2",
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
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testendpoints",
				Namespace: "ns1",
				Labels: map[string]string{
					"app": "app1",
				},
			},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpod",
				Namespace: "ns2",
				UID:       types.UID("deadbeef"),
			},
			Spec: v1.PodSpec{
				NodeName: "testnode",
				Containers: []v1.Container{
					{
						Name:  "c1",
						Image: "c1:latest",
						Ports: []v1.ContainerPort{
							{
								Name:          "mainport",
								ContainerPort: 9000,
								Protocol:      v1.ProtocolTCP,
							},
						},
					},
				},
			},
			Status: v1.PodStatus{
				HostIP: "2.3.4.5",
				PodIP:  "4.3.2.1",
			},
		},
	}
	n, _ := makeDiscovery(RoleEndpoint, NamespaceDiscovery{Names: []string{"ns1", "ns2"}}, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/ns1/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_hostname":      "testendpoint1",
						"__meta_kubernetes_endpoint_node_name":     "foobar",
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
					{
						"__address__": "6.7.8.9:9002",
						"__meta_kubernetes_endpoint_address_target_kind": "Node",
						"__meta_kubernetes_endpoint_address_target_name": "barbaz",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                   "ns1",
					"__meta_kubernetes_endpoints_name":                              "testendpoints",
					"__meta_kubernetes_endpoints_annotation_test_annotation":        "test",
					"__meta_kubernetes_endpoints_annotationpresent_test_annotation": "true",
					"__meta_kubernetes_service_label_app":                           "app1",
					"__meta_kubernetes_service_labelpresent_app":                    "true",
					"__meta_kubernetes_service_name":                                "testendpoints",
				},
				Source: "endpoints/ns1/testendpoints",
			},
			"endpoints/ns2/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                                    "4.3.2.1:9000",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
						"__meta_kubernetes_endpoint_address_target_kind": "Pod",
						"__meta_kubernetes_endpoint_address_target_name": "testpod",
						"__meta_kubernetes_pod_name":                     "testpod",
						"__meta_kubernetes_pod_ip":                       "4.3.2.1",
						"__meta_kubernetes_pod_ready":                    "unknown",
						"__meta_kubernetes_pod_phase":                    "",
						"__meta_kubernetes_pod_node_name":                "testnode",
						"__meta_kubernetes_pod_host_ip":                  "2.3.4.5",
						"__meta_kubernetes_pod_container_name":           "c1",
						"__meta_kubernetes_pod_container_image":          "c1:latest",
						"__meta_kubernetes_pod_container_port_name":      "mainport",
						"__meta_kubernetes_pod_container_port_number":    "9000",
						"__meta_kubernetes_pod_container_port_protocol":  "TCP",
						"__meta_kubernetes_pod_uid":                      "deadbeef",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":      "ns2",
					"__meta_kubernetes_endpoints_name": "testendpoints",
				},
				Source: "endpoints/ns2/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryOwnNamespace(t *testing.T) {
	epOne := makeEndpoints()
	epOne.Namespace = "own-ns"

	epTwo := makeEndpoints()
	epTwo.Namespace = "non-own-ns"

	podOne := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "own-ns",
			UID:       types.UID("deadbeef"),
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name:  "p1",
					Image: "p1:latest",
					Ports: []v1.ContainerPort{
						{
							Name:          "mainport",
							ContainerPort: 9000,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			HostIP: "2.3.4.5",
			PodIP:  "4.3.2.1",
		},
	}

	podTwo := podOne.DeepCopy()
	podTwo.Namespace = "non-own-ns"

	objs := []runtime.Object{
		epOne,
		epTwo,
		podOne,
		podTwo,
	}

	n, _ := makeDiscovery(RoleEndpoint, NamespaceDiscovery{IncludeOwnNamespace: true}, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpoints/own-ns/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__":                              "1.2.3.4:9000",
						"__meta_kubernetes_endpoint_hostname":      "testendpoint1",
						"__meta_kubernetes_endpoint_node_name":     "foobar",
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
					{
						"__address__": "6.7.8.9:9002",
						"__meta_kubernetes_endpoint_address_target_kind": "Node",
						"__meta_kubernetes_endpoint_address_target_name": "barbaz",
						"__meta_kubernetes_endpoint_port_name":           "testport",
						"__meta_kubernetes_endpoint_port_protocol":       "TCP",
						"__meta_kubernetes_endpoint_ready":               "true",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                   "own-ns",
					"__meta_kubernetes_endpoints_name":                              "testendpoints",
					"__meta_kubernetes_endpoints_annotation_test_annotation":        "test",
					"__meta_kubernetes_endpoints_annotationpresent_test_annotation": "true",
				},
				Source: "endpoints/own-ns/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsDiscoveryEmptyPodStatus(t *testing.T) {
	ep := makeEndpoints()
	ep.Namespace = "ns"

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "ns",
			UID:       types.UID("deadbeef"),
		},
		Spec: v1.PodSpec{
			NodeName: "testnode",
			Containers: []v1.Container{
				{
					Name:  "p1",
					Image: "p1:latest",
					Ports: []v1.ContainerPort{
						{
							Name:          "mainport",
							ContainerPort: 9000,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: v1.PodStatus{},
	}

	objs := []runtime.Object{
		ep,
		pod,
	}

	n, _ := makeDiscovery(RoleEndpoint, NamespaceDiscovery{IncludeOwnNamespace: true}, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 0,
		expectedRes:      map[string]*targetgroup.Group{},
	}.Run(t)
}
