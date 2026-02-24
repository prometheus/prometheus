// Copyright The Prometheus Authors
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
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/discovery/v1"
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

func makeEndpointSliceV1(namespace string) *v1.EndpointSlice {
	return &v1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: namespace,
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
				Zone: strptr("us-east-1a"),
			}, {
				Addresses: []string{"2.3.4.5"},
				Conditions: v1.EndpointConditions{
					Ready:       boolptr(true),
					Serving:     boolptr(true),
					Terminating: boolptr(false),
				},
				Zone: strptr("us-east-1b"),
			}, {
				Addresses: []string{"3.4.5.6"},
				Conditions: v1.EndpointConditions{
					Ready:       boolptr(false),
					Serving:     boolptr(true),
					Terminating: boolptr(true),
				},
				Zone: strptr("us-east-1c"),
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
				Zone: strptr("us-east-1a"),
			},
		},
	}
}

func makeNamespace(name string, labels, annotations map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func TestEndpointSliceDiscoveryBeforeRun(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}})

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := makeEndpointSliceV1("default")
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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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

func TestEndpointSliceDiscoveryAdd(t *testing.T) {
	t.Parallel()
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
	n, c := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, obj)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := &v1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testendpoints",
					Namespace: "default",
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
							Namespace: "default",
						},
						Conditions: v1.EndpointConditions{
							Ready: boolptr(false),
						},
					},
				},
			}
			c.DiscoveryV1().EndpointSlices(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
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
						"__meta_kubernetes_pod_container_init":                      "false",
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
						"__meta_kubernetes_pod_container_init":          "false",
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
	t.Parallel()
	n, c := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, makeEndpointSliceV1("default"))

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeEndpointSliceV1("default")
			c.DiscoveryV1().EndpointSlices(obj.Namespace).Delete(context.Background(), obj.Name, metav1.DeleteOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
				Source: "endpointslice/default/testendpoints",
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryUpdate(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, makeEndpointSliceV1("default"))

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeEndpointSliceV1("default")
			obj.ObjectMeta.Labels = nil
			obj.ObjectMeta.Annotations = nil
			obj.Endpoints = obj.Endpoints[0:2]
			c.DiscoveryV1().EndpointSlices(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
						"__meta_kubernetes_endpointslice_port":                            "9000",
						"__meta_kubernetes_endpointslice_port_app_protocol":               "http",
						"__meta_kubernetes_endpointslice_port_name":                       "testport",
						"__meta_kubernetes_endpointslice_port_protocol":                   "TCP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type": "IPv4",
					"__meta_kubernetes_endpointslice_name":         "testendpoints",
					"__meta_kubernetes_namespace":                  "default",
				},
			},
		},
	}.Run(t)
}

func TestEndpointSliceDiscoveryEmptyEndpoints(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, makeEndpointSliceV1("default"))

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeEndpointSliceV1("default")
			obj.Endpoints = []v1.Endpoint{}
			c.DiscoveryV1().EndpointSlices(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testendpoints": {
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
	t.Parallel()
	n, c := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, makeEndpointSliceV1("default"))

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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
	t.Parallel()
	n, c := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{Names: []string{"default"}}, makeEndpointSliceV1("default"))

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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
	t.Parallel()
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
	objs := []runtime.Object{makeEndpointSliceV1("default"), makeNode("foobar", "", "", nodeLabels1, nil), makeNode("barbaz", "", "", nodeLabels2, nil), svc}
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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
	t.Parallel()
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
	objs := []runtime.Object{makeEndpointSliceV1("default"), node1, node2, svc}
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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
	t.Parallel()
	epOne := makeEndpointSliceV1("default")
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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
						"__meta_kubernetes_pod_container_init":                "false",
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
	t.Parallel()
	epOne := makeEndpointSliceV1("default")
	epOne.Namespace = "own-ns"

	epTwo := makeEndpointSliceV1("default")
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
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
	t.Parallel()
	ep := makeEndpointSliceV1("default")
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
	t.Parallel()
	tests := []struct {
		name             string
		withNodeMetadata bool
	}{
		{"with node metadata", true},
		{"without node metadata", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				n *Discovery
				// service indexer is enabled by default
				mainInfIndexersCount = 1
			)
			if tc.withNodeMetadata {
				mainInfIndexersCount++
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

func TestEndpointSliceDiscoverySidecarContainer(t *testing.T) {
	t.Parallel()
	objs := []runtime.Object{
		&v1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testsidecar",
				Namespace: "default",
			},
			AddressType: v1.AddressTypeIPv4,
			Ports: []v1.EndpointPort{
				{
					Name:     strptr("testport"),
					Port:     int32ptr(9000),
					Protocol: protocolptr(corev1.ProtocolTCP),
				},
				{
					Name:     strptr("initport"),
					Port:     int32ptr(9111),
					Protocol: protocolptr(corev1.ProtocolTCP),
				},
			},
			Endpoints: []v1.Endpoint{
				{
					Addresses: []string{"4.3.2.1"},
					TargetRef: &corev1.ObjectReference{
						Kind:      "Pod",
						Name:      "testpod",
						Namespace: "default",
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpod",
				Namespace: "default",
				UID:       types.UID("deadbeef"),
			},
			Spec: corev1.PodSpec{
				NodeName: "testnode",
				InitContainers: []corev1.Container{
					{
						Name:  "ic1",
						Image: "ic1:latest",
						Ports: []corev1.ContainerPort{
							{
								Name:          "initport",
								ContainerPort: 1111,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
					{
						Name:  "ic2",
						Image: "ic2:latest",
						Ports: []corev1.ContainerPort{
							{
								Name:          "initport",
								ContainerPort: 9111,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
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

	n, _ := makeDiscovery(RoleEndpointSlice, NamespaceDiscovery{}, objs...)

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"endpointslice/default/testsidecar": {
				Targets: []model.LabelSet{
					{
						"__address__": "4.3.2.1:9000",
						"__meta_kubernetes_endpointslice_address_target_kind": "Pod",
						"__meta_kubernetes_endpointslice_address_target_name": "testpod",
						"__meta_kubernetes_endpointslice_port":                "9000",
						"__meta_kubernetes_endpointslice_port_name":           "testport",
						"__meta_kubernetes_endpointslice_port_protocol":       "TCP",
						"__meta_kubernetes_pod_container_image":               "c1:latest",
						"__meta_kubernetes_pod_container_name":                "c1",
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
						"__meta_kubernetes_pod_container_init":                "false",
					},
					{
						"__address__": "4.3.2.1:9111",
						"__meta_kubernetes_endpointslice_address_target_kind": "Pod",
						"__meta_kubernetes_endpointslice_address_target_name": "testpod",
						"__meta_kubernetes_endpointslice_port":                "9111",
						"__meta_kubernetes_endpointslice_port_name":           "initport",
						"__meta_kubernetes_endpointslice_port_protocol":       "TCP",
						"__meta_kubernetes_pod_container_image":               "ic2:latest",
						"__meta_kubernetes_pod_container_name":                "ic2",
						"__meta_kubernetes_pod_container_port_name":           "initport",
						"__meta_kubernetes_pod_container_port_number":         "9111",
						"__meta_kubernetes_pod_container_port_protocol":       "TCP",
						"__meta_kubernetes_pod_host_ip":                       "2.3.4.5",
						"__meta_kubernetes_pod_ip":                            "4.3.2.1",
						"__meta_kubernetes_pod_name":                          "testpod",
						"__meta_kubernetes_pod_node_name":                     "testnode",
						"__meta_kubernetes_pod_phase":                         "",
						"__meta_kubernetes_pod_ready":                         "unknown",
						"__meta_kubernetes_pod_uid":                           "deadbeef",
						"__meta_kubernetes_pod_container_init":                "true",
					},
					{
						"__address__":                                   "4.3.2.1:1111",
						"__meta_kubernetes_pod_container_image":         "ic1:latest",
						"__meta_kubernetes_pod_container_name":          "ic1",
						"__meta_kubernetes_pod_container_port_name":     "initport",
						"__meta_kubernetes_pod_container_port_number":   "1111",
						"__meta_kubernetes_pod_container_port_protocol": "TCP",
						"__meta_kubernetes_pod_host_ip":                 "2.3.4.5",
						"__meta_kubernetes_pod_ip":                      "4.3.2.1",
						"__meta_kubernetes_pod_name":                    "testpod",
						"__meta_kubernetes_pod_node_name":               "testnode",
						"__meta_kubernetes_pod_phase":                   "",
						"__meta_kubernetes_pod_ready":                   "unknown",
						"__meta_kubernetes_pod_uid":                     "deadbeef",
						"__meta_kubernetes_pod_container_init":          "true",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_endpointslice_address_type": "IPv4",
					"__meta_kubernetes_endpointslice_name":         "testsidecar",
					"__meta_kubernetes_namespace":                  "default",
				},
				Source: "endpointslice/default/testsidecar",
			},
		},
	}.Run(t)
}

func TestEndpointsSlicesDiscoveryWithNamespaceMetadata(t *testing.T) {
	t.Parallel()

	ns := "test-ns"
	nsLabels := map[string]string{"service": "web", "layer": "frontend"}
	nsAnnotations := map[string]string{"contact": "platform", "release": "v5.6.7"}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: ns,
			Labels: map[string]string{
				"app/name": "test",
			},
		},
	}
	objs := []runtime.Object{makeNamespace(ns, nsLabels, nsAnnotations), svc, makeEndpointSliceV1(ns)}
	n, _ := makeDiscoveryWithMetadata(RoleEndpointSlice, NamespaceDiscovery{}, AttachMetadataConfig{Namespace: true}, objs...)
	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			fmt.Sprintf("endpointslice/%s/testendpoints", ns): {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
					"__meta_kubernetes_namespace":                                             model.LabelValue(ns),
					"__meta_kubernetes_namespace_annotation_contact":                          "platform",
					"__meta_kubernetes_namespace_annotationpresent_contact":                   "true",
					"__meta_kubernetes_namespace_annotation_release":                          "v5.6.7",
					"__meta_kubernetes_namespace_annotationpresent_release":                   "true",
					"__meta_kubernetes_namespace_label_service":                               "web",
					"__meta_kubernetes_namespace_labelpresent_service":                        "true",
					"__meta_kubernetes_namespace_label_layer":                                 "frontend",
					"__meta_kubernetes_namespace_labelpresent_layer":                          "true",
					"__meta_kubernetes_service_label_app_name":                                "test",
					"__meta_kubernetes_service_labelpresent_app_name":                         "true",
					"__meta_kubernetes_service_name":                                          "testendpoints",
				},
				Source: fmt.Sprintf("endpointslice/%s/testendpoints", ns),
			},
		},
	}.Run(t)
}

func TestEndpointsSlicesDiscoveryWithUpdatedNamespaceMetadata(t *testing.T) {
	t.Parallel()

	ns := "test-ns"
	nsLabels := map[string]string{"component": "database", "layer": "backend"}
	nsAnnotations := map[string]string{"contact": "dba", "release": "v6.7.8"}
	metadataConfig := AttachMetadataConfig{Namespace: true}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testendpoints",
			Namespace: ns,
			Labels: map[string]string{
				"app/name": "test",
			},
		},
	}

	namespace := makeNamespace(ns, nsLabels, nsAnnotations)
	n, c := makeDiscoveryWithMetadata(RoleEndpointSlice, NamespaceDiscovery{}, metadataConfig, namespace, svc, makeEndpointSliceV1(ns))

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 2,
		afterStart: func() {
			namespace.Labels["component"] = "cache"
			namespace.Labels["region"] = "us-central"
			namespace.Annotations["contact"] = "sre"
			namespace.Annotations["monitoring"] = "enabled"
			c.CoreV1().Namespaces().Update(context.Background(), namespace, metav1.UpdateOptions{})
		},
		expectedRes: map[string]*targetgroup.Group{
			fmt.Sprintf("endpointslice/%s/testendpoints", ns): {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:9000",
						"__meta_kubernetes_endpointslice_address_target_kind":                "",
						"__meta_kubernetes_endpointslice_address_target_name":                "",
						"__meta_kubernetes_endpointslice_endpoint_conditions_ready":          "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_serving":        "true",
						"__meta_kubernetes_endpointslice_endpoint_conditions_terminating":    "false",
						"__meta_kubernetes_endpointslice_endpoint_hostname":                  "testendpoint1",
						"__meta_kubernetes_endpointslice_endpoint_node_name":                 "foobar",
						"__meta_kubernetes_endpointslice_endpoint_topology_present_topology": "true",
						"__meta_kubernetes_endpointslice_endpoint_topology_topology":         "value",
						"__meta_kubernetes_endpointslice_endpoint_zone":                      "us-east-1a",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1b",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1c",
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
						"__meta_kubernetes_endpointslice_endpoint_zone":                   "us-east-1a",
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
					"__meta_kubernetes_namespace":                                             model.LabelValue(ns),
					"__meta_kubernetes_namespace_annotation_contact":                          "sre",
					"__meta_kubernetes_namespace_annotationpresent_contact":                   "true",
					"__meta_kubernetes_namespace_annotation_release":                          "v6.7.8",
					"__meta_kubernetes_namespace_annotationpresent_release":                   "true",
					"__meta_kubernetes_namespace_annotation_monitoring":                       "enabled",
					"__meta_kubernetes_namespace_annotationpresent_monitoring":                "true",
					"__meta_kubernetes_namespace_label_component":                             "cache",
					"__meta_kubernetes_namespace_labelpresent_component":                      "true",
					"__meta_kubernetes_namespace_label_layer":                                 "backend",
					"__meta_kubernetes_namespace_labelpresent_layer":                          "true",
					"__meta_kubernetes_namespace_label_region":                                "us-central",
					"__meta_kubernetes_namespace_labelpresent_region":                         "true",
					"__meta_kubernetes_service_label_app_name":                                "test",
					"__meta_kubernetes_service_labelpresent_app_name":                         "true",
					"__meta_kubernetes_service_name":                                          "testendpoints",
				},
				Source: fmt.Sprintf("endpointslice/%s/testendpoints", ns),
			},
		},
	}.Run(t)
}
