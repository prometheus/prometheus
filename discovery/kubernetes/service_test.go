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

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func makeMultiPortService() *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testservice",
			Namespace:   "default",
			Labels:      map[string]string{"test-label": "testvalue"},
			Annotations: map[string]string{"test-annotation": "testannotationvalue"},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "testport0",
					Protocol: v1.ProtocolTCP,
					Port:     int32(30900),
				},
				{
					Name:     "testport1",
					Protocol: v1.ProtocolUDP,
					Port:     int32(30901),
				},
			},
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
		},
	}
}

func makeSuffixedService(suffix string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("testservice%s", suffix),
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "testport",
					Protocol: v1.ProtocolTCP,
					Port:     int32(30900),
				},
			},
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
		},
	}
}

func makeService() *v1.Service {
	return makeSuffixedService("")
}

func makeExternalService() *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testservice-external",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "testport",
					Protocol: v1.ProtocolTCP,
					Port:     int32(31900),
				},
			},
			Type:         v1.ServiceTypeExternalName,
			ExternalName: "FooExternalName",
		},
	}
}

func makeLoadBalancerService() *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testservice-loadbalancer",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:     "testport",
					Protocol: v1.ProtocolTCP,
					Port:     int32(31900),
				},
			},
			Type:           v1.ServiceTypeLoadBalancer,
			LoadBalancerIP: "127.0.0.1",
			ClusterIP:      "10.0.0.1",
		},
	}
}

func TestServiceDiscoveryAdd(t *testing.T) {
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeService()
			c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			obj = makeExternalService()
			c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			obj = makeLoadBalancerService()
			c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 3,
		expectedRes: map[string]*targetgroup.Group{
			"svc/default/testservice": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                           "testservice.default.svc:30900",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport",
						"__meta_kubernetes_service_port_number": "30900",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/testservice",
			},
			"svc/default/testservice-external": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                             "testservice-external.default.svc:31900",
						"__meta_kubernetes_service_type":          "ExternalName",
						"__meta_kubernetes_service_port_name":     "testport",
						"__meta_kubernetes_service_port_number":   "31900",
						"__meta_kubernetes_service_external_name": "FooExternalName",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice-external",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/testservice-external",
			},
			"svc/default/testservice-loadbalancer": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                               "testservice-loadbalancer.default.svc:31900",
						"__meta_kubernetes_service_type":            "LoadBalancer",
						"__meta_kubernetes_service_port_name":       "testport",
						"__meta_kubernetes_service_port_number":     "31900",
						"__meta_kubernetes_service_cluster_ip":      "10.0.0.1",
						"__meta_kubernetes_service_loadbalancer_ip": "127.0.0.1",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice-loadbalancer",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/testservice-loadbalancer",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryDelete(t *testing.T) {
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{}, makeService())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeService()
			c.CoreV1().Services(obj.Namespace).Delete(context.Background(), obj.Name, metav1.DeleteOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"svc/default/testservice": {
				Source: "svc/default/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryUpdate(t *testing.T) {
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{}, makeService())

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeMultiPortService()
			c.CoreV1().Services(obj.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"svc/default/testservice": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                           "testservice.default.svc:30900",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport0",
						"__meta_kubernetes_service_port_number": "30900",
					},
					{
						"__meta_kubernetes_service_port_protocol": "UDP",
						"__address__":                           "testservice.default.svc:30901",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport1",
						"__meta_kubernetes_service_port_number": "30901",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name":                              "testservice",
					"__meta_kubernetes_namespace":                                 "default",
					"__meta_kubernetes_service_label_test_label":                  "testvalue",
					"__meta_kubernetes_service_labelpresent_test_label":           "true",
					"__meta_kubernetes_service_annotation_test_annotation":        "testannotationvalue",
					"__meta_kubernetes_service_annotationpresent_test_annotation": "true",
				},
				Source: "svc/default/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryNamespaces(t *testing.T) {
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{Names: []string{"ns1", "ns2"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				obj := makeService()
				obj.Namespace = ns
				c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"svc/ns1/testservice": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                           "testservice.ns1.svc:30900",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport",
						"__meta_kubernetes_service_port_number": "30900",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "ns1",
				},
				Source: "svc/ns1/testservice",
			},
			"svc/ns2/testservice": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                           "testservice.ns2.svc:30900",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport",
						"__meta_kubernetes_service_port_number": "30900",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "ns2",
				},
				Source: "svc/ns2/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryOwnNamespace(t *testing.T) {
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{IncludeOwnNamespace: true})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"own-ns", "non-own-ns"} {
				obj := makeService()
				obj.Namespace = ns
				c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"svc/own-ns/testservice": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                           "testservice.own-ns.svc:30900",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport",
						"__meta_kubernetes_service_port_number": "30900",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "own-ns",
				},
				Source: "svc/own-ns/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryAllNamespaces(t *testing.T) {
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"own-ns", "non-own-ns"} {
				obj := makeService()
				obj.Namespace = ns
				c.CoreV1().Services(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"svc/own-ns/testservice": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                           "testservice.own-ns.svc:30900",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport",
						"__meta_kubernetes_service_port_number": "30900",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "own-ns",
				},
				Source: "svc/own-ns/testservice",
			},
			"svc/non-own-ns/testservice": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                           "testservice.non-own-ns.svc:30900",
						"__meta_kubernetes_service_type":        "ClusterIP",
						"__meta_kubernetes_service_cluster_ip":  "10.0.0.1",
						"__meta_kubernetes_service_port_name":   "testport",
						"__meta_kubernetes_service_port_number": "30900",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "non-own-ns",
				},
				Source: "svc/non-own-ns/testservice",
			},
		},
	}.Run(t)
}
