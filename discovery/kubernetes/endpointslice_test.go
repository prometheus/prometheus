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
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/discovery/v1"
	"k8s.io/api/discovery/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func strptr(str string) *string {
	return &str
}

func boolptr(b bool) *bool {
	return &b
}

func int32ptr(i int32) *int32 {
	return &i
}

func protocolptr(p corev1.Protocol) *corev1.Protocol {
	return &p
}

func makeEndpointSliceV1() *v1.EndpointSlice {
	return &v1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
			Labels: map[string]string{
				v1.LabelServiceName: "testendpoints",
			},
			Annotations: map[string]string{
				"test.annotation": "test",
			},
		},
		AddressType: v1.AddressTypeIPv4,
		Ports: []v1.EndpointPort{
			{
				Name:        strptr("testport"),
				Port:        int32ptr(9000),
				Protocol:    protocolptr(corev1.ProtocolTCP),
				AppProtocol: strptr("http"),
			},
		},
		Endpoints: []v1.Endpoint{
			{
				Addresses: []string{"1.2.3.4"},
				Conditions: v1.EndpointConditions{
					Ready:       boolptr(true),
					Serving:     boolptr(true),
					Terminating: boolptr(false),
				},
				Hostname:  strptr("testendpoint1"),
				TargetRef: &corev1.ObjectReference{},
				NodeName:  strptr("foobar"),
				DeprecatedTopology: map[string]string{
					"topology": "value",
				},
			}, {
				Addresses: []string{"2.3.4.5"},
				Conditions: v1.EndpointConditions{
					Ready:       boolptr(true),
					Serving:     boolptr(true),
					Terminating: boolptr(false),
				},
			}, {
				Addresses: []string{"3.4.5.6"},
				Conditions: v1.EndpointConditions{
					Ready:       boolptr(false),
					Serving:     boolptr(true),
					Terminating: boolptr(true),
				},
			}, {
				Addresses: []string{"4.5.6.7"},
				Conditions: v1.EndpointConditions{
					Ready:       boolptr(true),
					Serving:     boolptr(true),
					Terminating: boolptr(false),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Node",
					Name: "barbaz",
				},
			},
		},
	}
}

func makeEndpointSliceV1beta1() *v1beta1.EndpointSlice {
	return &v1beta1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
			Labels: map[string]string{
				v1beta1.LabelServiceName: "testendpoints",
			},
			Annotations: map[string]string{
				"test.annotation": "test",
			},
		},
		AddressType: v1beta1.AddressTypeIPv4,
		Ports: []v1beta1.EndpointPort{
			{
				Name:     strptr("testport"),
				Port:     int32ptr(9000),
				Protocol: protocolptr(corev1.ProtocolTCP),
			},
		},
		Endpoints: []v1beta1.Endpoint{
			{
				Addresses: []string{"1.2.3.4"},
				Hostname:  strptr("testendpoint1"),
			}, {
				Addresses: []string{"2.3.4.5"},
				Conditions: v1beta1.EndpointConditions{
					Ready:       boolptr(true),
					Serving:     boolptr(true),
					Terminating: boolptr(false),
				},
			}, {
				Addresses: []string{"3.4.5.6"},
				Conditions: v1beta1.EndpointConditions{
					Ready:       boolptr(false),
					Serving:     boolptr(true),
					Terminating: boolptr(true),
				},
			}, {
				Addresses: []string{"4.5.6.7"},
				Conditions: v1beta1.EndpointConditions{
					Ready:       boolptr(true),
					Serving:     boolptr(true),
					Terminating: boolptr(false),
				},
				TargetRef: &corev1.ObjectReference{
					Kind: "Node",
					Name: "barbaz",
				},
			},
		},
	}
}

func TestEndpointSliceDiscoveryBeforeRun(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "v1.25.0")

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := makeEndpointSliceV1()
			c.DiscoveryV1().EndpointSlices(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_namespace":                                             "default",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryBeforeRunV1beta1(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "1.20.0")

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := makeEndpointSliceV1beta1()
			c.DiscoveryV1beta1().EndpointSlices(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_endpoint_hostname": "testendpoint1",
						"__meta_kubernetes_endpointslice_port":              "9000",
						"__meta_kubernetes_endpointslice_port_name":         "testport",
						"__meta_kubernetes_endpointslice_port_protocol":     "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_namespace":                                             "default",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryAdd(t *testing.T) {
	obj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "default",
			UID:       types.UID("deadbeef"),
		},
		Spec: corev1.PodSpec{
			NodeName: "testnode",
			Containers: []corev1.Container{
				{
					Name:  "c1",
					Image: "c1:latest",
					Ports: []corev1.ContainerPort{
						{
							Name:          "mainport",
							ContainerPort: 9000,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
				{
					Name:  "c2",
					Image: "c2:latest",
					Ports: []corev1.ContainerPort{
						{
							Name:          "sideport",
							ContainerPort: 9001,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			HostIP: "2.3.4.5",
			PodIP:  "1.2.3.4",
		},
	}
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "v1.20.0", obj)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := &v1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
				},
				AddressType: v1beta1.AddressTypeIPv4,
				Ports: []v1beta1.EndpointPort{
					{
						Name:     strptr("testport"),
						Port:     int32ptr(9000),
						Protocol: protocolptr(corev1.ProtocolTCP),
					},
				},
				Endpoints: []v1beta1.Endpoint{
					{
						Addresses: []string{"4.3.2.1"},
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Name:      "testpod",
							Namespace: "default",
						},
						Conditions: v1beta1.EndpointConditions{
							Ready: boolptr(false),
						},
					},
				},
			}
			c.DiscoveryV1beta1().EndpointSlices(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "4.3.2.1:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":       "Pod",
						"__meta_kubernetes_endpointslice_address_target_name":       "testpod",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready": "false",
						"__meta_kubernetes_endpointslice_port":                      "9000",
						"__meta_kubernetes_endpointslice_port_name":                 "testport",
						"__meta_kubernetes_endpointslice_port_protocol":             "TCP",
						"__meta_kubernetes_pod_container_name":                      "c1",
						"__meta_kubernetes_pod_container_image":                     "c1:latest",
						"__meta_kubernetes_pod_container_port_name":                 "mainport",
						"__meta_kubernetes_pod_container_port_number":               "9000",
						"__meta_kubernetes_pod_container_port_protocol":             "TCP",
						"__meta_kubernetes_pod_host_ip":                             "2.3.4.5",
						"__meta_kubernetes_pod_ip":                                  "1.2.3.4",
						"__meta_kubernetes_pod_name":                                "testpod",
						"__meta_kubernetes_pod_node_name":                           "testnode",
						"__meta_kubernetes_pod_phase":                               "",
						"__meta_kubernetes_pod_ready":                               "unknown",
						"__meta_kubernetes_pod_uid":                                 "deadbeef",
					},
					{
						"__address__":                                   "1.2.3.4:9001",
						"__meta_kubernetes_pod_container_name":          "c2",
						"__meta_kubernetes_pod_container_image":         "c2:latest",
						"__meta_kubernetes_pod_container_port_name":     "sideport",
						"__meta_kubernetes_pod_container_port_number":   "9001",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
						"__meta_kubernetes_pod_host_ip":                 "2.3.4.5",
						"__meta_kubernetes_pod_ip":                      "1.2.3.4",
						"__meta_kubernetes_pod_name":                    "testpod",
						"__meta_kubernetes_pod_node_name":               "testnode",
						"__meta_kubernetes_pod_phase":                   "",
						"__meta_kubernetes_pod_ready":                   "unknown",
						"__meta_kubernetes_pod_uid":                     "deadbeef",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type": "IPv4",
					"__meta_kubernetes_endpointslice_name":         "testendpoints",
					"__meta_kubernetes_namespace":                  "default",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryDelete(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "v1.21.0", makeEndpointSliceV1())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeEndpointSliceV1()
			c.DiscoveryV1beta1().EndpointSlices(obj.Namespace).Delete(context.Background(), obj.Name, metav1.DeleteOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Source: "endpointslice/default/testendpoints",
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: map[model.LabelName]model.LabelValue{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "default",
				},
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryUpdate(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "v1.21.0", makeEndpointSliceV1())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := &v1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
				},
				AddressType: v1beta1.AddressTypeIPv4,
				Ports: []v1beta1.EndpointPort{
					{
						Name:     strptr("testport"),
						Port:     int32ptr(9000),
						Protocol: protocolptr(corev1.ProtocolTCP),
					},
				},
				Endpoints: []v1beta1.Endpoint{
					{
						Addresses: []string{"1.2.3.4"},
						Hostname:  strptr("testendpoint1"),
					}, {
						Addresses: []string{"2.3.4.5"},
						Conditions: v1beta1.EndpointConditions{
							Ready: boolptr(true),
						},
					},
				},
			}
			c.DiscoveryV1beta1().EndpointSlices(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Source: "endpointslice/default/testendpoints",
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "default",
				},
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryEmptyEndpoints(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "v1.21.0", makeEndpointSliceV1())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := &v1beta1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
				},
				AddressType: v1beta1.AddressTypeIPv4,
				Ports: []v1beta1.EndpointPort{
					{
						Name:     strptr("testport"),
						Port:     int32ptr(9000),
						Protocol: protocolptr(corev1.ProtocolTCP),
					},
				},
				Endpoints: []v1beta1.Endpoint{},
			}
			c.DiscoveryV1beta1().EndpointSlices(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "default",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryWithService(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "v1.21.0", makeEndpointSliceV1())

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := &corev1.Service{
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
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "default",
					"__meta_kubernetes_service_label_app_name":                                "test",
					"__meta_kubernetes_service_labelpresent_app_name":                         "true",
					"__meta_kubernetes_service_name":                                          "testendpoints",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryWithServiceUpdate(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, "v1.21.0", makeEndpointSliceV1())

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := &corev1.Service{
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
			obj := &corev1.Service{
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
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "default",
					"__meta_kubernetes_service_label_app_name":                                "svc",
					"__meta_kubernetes_service_label_component":                               "testing",
					"__meta_kubernetes_service_labelpresent_app_name":                         "true",
					"__meta_kubernetes_service_labelpresent_component":                        "true",
					"__meta_kubernetes_service_name":                                          "testendpoints",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsSlicesDiscoveryWithNodeMetadata(t *testing.T) {
	metadataConfig := AttachMetadataConfig{Node: true}
	nodeLabels1 := map[string]string{"az": "us-east1"}
	nodeLabels2 := map[string]string{"az": "us-west2"}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
			Labels: map[string]string{
				"app/name": "test",
			},
		},
	}
	objs := []runtime.Object{makeEndpointSliceV1(), makeNode("foobar", "", "", nodeLabels1, nil), makeNode("barbaz", "", "", nodeLabels2, nil), svc}
	n, _ := makeDiscoveryWithMetadata(RoleEndpointSlice, NamespaceDiscovery{}, metadataConfig, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
						"__meta_kubernetes_node_label_az":                                    "us-east1",
						"__meta_kubernetes_node_labelpresent_az":                             "true",
						"__meta_kubernetes_node_name":                                        "foobar",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_node_label_az":                                 "us-west2",
						"__meta_kubernetes_node_labelpresent_az":                          "true",
						"__meta_kubernetes_node_name":                                     "barbaz",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "default",
					"__meta_kubernetes_service_label_app_name":                                "test",
					"__meta_kubernetes_service_labelpresent_app_name":                         "true",
					"__meta_kubernetes_service_name":                                          "testendpoints",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointsSlicesDiscoveryWithUpdatedNodeMetadata(t *testing.T) {
	metadataConfig := AttachMetadataConfig{Node: true}
	nodeLabels1 := map[string]string{"az": "us-east1"}
	nodeLabels2 := map[string]string{"az": "us-west2"}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: "default",
			Labels: map[string]string{
				"app/name": "test",
			},
		},
	}
	node1 := makeNode("foobar", "", "", nodeLabels1, nil)
	node2 := makeNode("barbaz", "", "", nodeLabels2, nil)
	objs := []runtime.Object{makeEndpointSliceV1(), node1, node2, svc}
	n, c := makeDiscoveryWithMetadata(RoleEndpointSlice, NamespaceDiscovery{}, metadataConfig, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 2,
		afterStart: func() {
			node1.Labels["az"] = "us-central1"
			c.CoreV1().Nodes().Update(context.Background(), node1, metav1.UpdateOptions{})
		},
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
						"__meta_kubernetes_node_label_az":                                    "us-east1",
						"__meta_kubernetes_node_labelpresent_az":                             "true",
						"__meta_kubernetes_node_name":                                        "foobar",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_node_label_az":                                 "us-west2",
						"__meta_kubernetes_node_labelpresent_az":                          "true",
						"__meta_kubernetes_node_name":                                     "barbaz",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "default",
					"__meta_kubernetes_service_label_app_name":                                "test",
					"__meta_kubernetes_service_labelpresent_app_name":                         "true",
					"__meta_kubernetes_service_name":                                          "testendpoints",
				},
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryNamespaces(t *testing.T) {
	epOne := makeEndpointSliceV1()
	epOne.Namespace = "ns1"
	objs := []runtime.Object{
		epOne,
		&v1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testendpoints",
				Namespace: "ns2",
			},
			AddressType: v1.AddressTypeIPv4,
			Ports: []v1.EndpointPort{
				{
					Name:     strptr("testport"),
					Port:     int32ptr(9000),
					Protocol: protocolptr(corev1.ProtocolTCP),
				},
			},
			Endpoints: []v1.Endpoint{
				{
					Addresses: []string{"4.3.2.1"},
					TargetRef: &corev1.ObjectReference{
						Kind:      "Pod",
						Name:      "testpod",
						Namespace: "ns2",
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testendpoints",
				Namespace: "ns1",
				Labels: map[string]string{
					"app": "app1",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpod",
				Namespace: "ns2",
				UID:       types.UID("deadbeef"),
			},
			Spec: corev1.PodSpec{
				NodeName: "testnode",
				Containers: []corev1.Container{
					{
						Name:  "c1",
						Image: "c1:latest",
						Ports: []corev1.ContainerPort{
							{
								Name:          "mainport",
								ContainerPort: 9000,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				HostIP: "2.3.4.5",
				PodIP:  "4.3.2.1",
			},
		},
	}
	n, _ := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"ns1", "ns2"}}, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/ns1/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
					"__meta_kubernetes_namespace":                                             "ns1",
					"__meta_kubernetes_service_label_app":                                     "app1",
					"__meta_kubernetes_service_labelpresent_app":                              "true",
					"__meta_kubernetes_service_name":                                          "testendpoints",
				},
				Source: "endpointslice/ns1/testendpoints",
			},
			"endpointslice/ns2/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "4.3.2.1:9000",
						"__meta_kubernetes_endpointslice_address_target_kind": "Pod",
						"__meta_kubernetes_endpointslice_address_target_name": "testpod",
						"__meta_kubernetes_endpointslice_port":                "9000",
						"__meta_kubernetes_endpointslice_port_name":           "testport",
						"__meta_kubernetes_endpointslice_port_protocol":       "TCP",
						"__meta_kubernetes_pod_container_name":                "c1",
						"__meta_kubernetes_pod_container_image":               "c1:latest",
						"__meta_kubernetes_pod_container_port_name":           "mainport",
						"__meta_kubernetes_pod_container_port_number":         "9000",
						"__meta_kubernetes_pod_container_port_protocol":       "TCP",
						"__meta_kubernetes_pod_host_ip":                       "2.3.4.5",
						"__meta_kubernetes_pod_ip":                            "4.3.2.1",
						"__meta_kubernetes_pod_name":                          "testpod",
						"__meta_kubernetes_pod_node_name":                     "testnode",
						"__meta_kubernetes_pod_phase":                         "",
						"__meta_kubernetes_pod_ready":                         "unknown",
						"__meta_kubernetes_pod_uid":                           "deadbeef",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type": "IPv4",
					"__meta_kubernetes_endpointslice_name":         "testendpoints",
					"__meta_kubernetes_namespace":                  "ns2",
				},
				Source: "endpointslice/ns2/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryOwnNamespace(t *testing.T) {
	epOne := makeEndpointSliceV1()
	epOne.Namespace = "own-ns"

	epTwo := makeEndpointSliceV1()
	epTwo.Namespace = "non-own-ns"

	podOne := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "own-ns",
			UID:       types.UID("deadbeef"),
		},
		Spec: corev1.PodSpec{
			NodeName: "testnode",
			Containers: []corev1.Container{
				{
					Name:  "p1",
					Image: "p1:latest",
					Ports: []corev1.ContainerPort{
						{
							Name:          "mainport",
							ContainerPort: 9000,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
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
	n, _ := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{IncludeOwnNamespace: true}, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/own-ns/testendpoints": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_port":                               "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":                  "http",
						"__meta_kubernetes_endpointslice_port_name":                          "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                      "TCP",
					},
					{
						"__address__": "2.3.4.5:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
					},
					{
						"__address__": "3.4.5.6:9000",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "false",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "true",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
					},
					{
						"__address__": "4.5.6.7:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":             "Node",
						"__meta_kubernetes_endpointslice_address_target_name":             "barbaz",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":       "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":     "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating": "false",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type":                            "IPv4",
					"__meta_kubernetes_endpointslice_name":                                    "testendpoints",
					"__meta_kubernetes_namespace":                                             "own-ns",
					"__meta_kubernetes_endpointslice_label_kubernetes_io_service_name":        "testendpoints",
					"__meta_kubernetes_endpointslice_labelpresent_kubernetes_io_service_name": "true",
					"__meta_kubernetes_endpointslice_annotation_test_annotation":              "test",
					"__meta_kubernetes_endpointslice_annotationpresent_test_annotation":       "true",
				},
				Source: "endpointslice/own-ns/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryEmptyPodStatus(t *testing.T) {
	ep := makeEndpointSliceV1()
	ep.Namespace = "ns"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpod",
			Namespace: "ns",
			UID:       types.UID("deadbeef"),
		},
		Spec: corev1.PodSpec{
			NodeName: "testnode",
			Containers: []corev1.Container{
				{
					Name:  "p1",
					Image: "p1:latest",
					Ports: []corev1.ContainerPort{
						{
							Name:          "mainport",
							ContainerPort: 9000,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{},
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

// TestEndpointSliceInfIndexersCount makes sure that RoleEndpointSlice discovery
// sets up indexing for the main Kube informer only when needed.
// See: https://github.com/prometheus/prometheus/pull/13554#discussion_r1490965817
func TestEndpointSliceInfIndexersCount(t *testing.T) {
	tests := []struct {
		name             string
		withNodeMetadata bool
	}{
		{"with node metadata", true},
		{"without node metadata", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				n                    *Discovery
				mainInfIndexersCount int
			)
			if tc.withNodeMetadata {
				mainInfIndexersCount = 1
				n, _ = makeDiscoveryWithMetadata(RoleEndpointSlice, NamespaceDiscovery{}, AttachMetadataConfig{Node: true})
			} else {
				n, _ = makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{})
			}

			k8sDiscoveryTest{
				discovery: n,
				afterStart: func() {
					n.RLock()
					defer n.RUnlock()
					require.Len(t, n.discoverers, 1)
					require.Len(t, n.discoverers[0].(*EndpointSlice).endpointSliceInf.GetIndexer().GetIndexers(), mainInfIndexersCount)
				},
			}.Run(t)
		})
	}
}
