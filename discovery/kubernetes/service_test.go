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

func makeService(namespace string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testservice",
			Namespace: namespace,
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

func makeLoadBalancerServiceWithStatusIPs(name string, ips []string) *v1.Service {
	ing := make([]v1.LoadBalancerIngress, 0, len(ips))
	for _, ip := range ips {
		ing = append(ing, v1.LoadBalancerIngress{IP: ip})
	}
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "testport",
				Protocol: v1.ProtocolTCP,
				Port:     int32(32001),
			}},
			Type:      v1.ServiceTypeLoadBalancer,
			ClusterIP: "10.0.0.1",
		},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{Ingress: ing},
		},
	}
}

func makeLoadBalancerServiceWithStatusHostnames(name string, hosts []string) *v1.Service {
	ing := make([]v1.LoadBalancerIngress, 0, len(hosts))
	for _, h := range hosts {
		ing = append(ing, v1.LoadBalancerIngress{Hostname: h})
	}
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Name:     "testport",
				Protocol: v1.ProtocolTCP,
				Port:     int32(32002),
			}},
			Type:      v1.ServiceTypeLoadBalancer,
			ClusterIP: "10.0.0.1",
		},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{Ingress: ing},
		},
	}
}

func TestServiceDiscoveryAdd(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeService("default")
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
						"__meta_kubernetes_service_loadbalancer_ips": "127.0.0.1",
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

func TestServiceDiscoveryLoadBalancerRealWorld(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{})

	metallbV4 := makeLoadBalancerServiceWithStatusIPs("svc-metallb-v4", []string{"1.2.3.4"})
	metallbDual := makeLoadBalancerServiceWithStatusIPs("svc-metallb-dual", []string{"2001:1:2:3:4::", "1.2.3.4"})
	eks := makeLoadBalancerServiceWithStatusHostnames("svc-eks", []string{"example-lb-name-abc.elb.eu-central-1.test.com"})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			c.CoreV1().Services(metallbV4.Namespace).Create(context.Background(), metallbV4, metav1.CreateOptions{})
			c.CoreV1().Services(metallbDual.Namespace).Create(context.Background(), metallbDual, metav1.CreateOptions{})
			c.CoreV1().Services(eks.Namespace).Create(context.Background(), eks, metav1.CreateOptions{})
		},
		expectedMaxItems: 3,
		expectedRes: map[string]*targetgroup.Group{
			"svc/default/svc-metallb-v4": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                                "svc-metallb-v4.default.svc:32001",
						"__meta_kubernetes_service_type":             "LoadBalancer",
						"__meta_kubernetes_service_port_name":        "testport",
						"__meta_kubernetes_service_port_number":      "32001",
						"__meta_kubernetes_service_cluster_ip":       "10.0.0.1",
						"__meta_kubernetes_service_loadbalancer_ips": "1.2.3.4",
						"__meta_kubernetes_service_loadbalancer_ip": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "svc-metallb-v4",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/svc-metallb-v4",
			},
			"svc/default/svc-metallb-dual": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                                "svc-metallb-dual.default.svc:32001",
						"__meta_kubernetes_service_type":             "LoadBalancer",
						"__meta_kubernetes_service_port_name":        "testport",
						"__meta_kubernetes_service_port_number":      "32001",
						"__meta_kubernetes_service_cluster_ip":       "10.0.0.1",
						"__meta_kubernetes_service_loadbalancer_ips": "2001:1:2:3:4::,1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "svc-metallb-dual",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/svc-metallb-dual",
			},
			"svc/default/svc-eks": {
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                                      "svc-eks.default.svc:32002",
						"__meta_kubernetes_service_type":                   "LoadBalancer",
						"__meta_kubernetes_service_port_name":              "testport",
						"__meta_kubernetes_service_port_number":            "32002",
						"__meta_kubernetes_service_cluster_ip":             "10.0.0.1",
						"__meta_kubernetes_service_loadbalancer_hostnames": "example-lb-name-abc.elb.eu-central-1.test.com",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "svc-eks",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/svc-eks",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryDelete(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{}, makeService("default"))

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeService("default")
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
	t.Parallel()
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{}, makeService("default"))

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
	t.Parallel()
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{Names: []string{"ns1", "ns2"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				obj := makeService(ns)
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
	t.Parallel()
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{IncludeOwnNamespace: true})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"own-ns", "non-own-ns"} {
				obj := makeService(ns)
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
	t.Parallel()
	n, c := makeDiscovery(RoleService, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"own-ns", "non-own-ns"} {
				obj := makeService(ns)
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

func TestServiceDiscoveryWithNamespaceMetadata(t *testing.T) {
	t.Parallel()

	ns := "test-ns"
	nsLabels := map[string]string{"environment": "production", "team": "backend"}
	nsAnnotations := map[string]string{"owner": "platform", "version": "v1.2.3"}

	n, _ := makeDiscoveryWithMetadata(RoleService, NamespaceDiscovery{}, AttachMetadataConfig{Namespace: true}, makeNamespace(ns, nsLabels, nsAnnotations), makeService(ns))
	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			fmt.Sprintf("svc/%s/testservice", ns): {
				Targets: []model.LabelSet{
					{
						"__address__":                             "testservice.test-ns.svc:30900",
						"__meta_kubernetes_service_cluster_ip":    "10.0.0.1",
						"__meta_kubernetes_service_port_name":     "testport",
						"__meta_kubernetes_service_port_number":   "30900",
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__meta_kubernetes_service_type":          "ClusterIP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                           model.LabelValue(ns),
					"__meta_kubernetes_namespace_annotation_owner":          "platform",
					"__meta_kubernetes_namespace_annotationpresent_owner":   "true",
					"__meta_kubernetes_namespace_annotation_version":        "v1.2.3",
					"__meta_kubernetes_namespace_annotationpresent_version": "true",
					"__meta_kubernetes_namespace_label_environment":         "production",
					"__meta_kubernetes_namespace_labelpresent_environment":  "true",
					"__meta_kubernetes_namespace_label_team":                "backend",
					"__meta_kubernetes_namespace_labelpresent_team":         "true",
					"__meta_kubernetes_service_name":                        "testservice",
				},
				Source: fmt.Sprintf("svc/%s/testservice", ns),
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryWithUpdatedNamespaceMetadata(t *testing.T) {
	t.Parallel()

	ns := "test-ns"
	nsLabels := map[string]string{"environment": "development", "team": "frontend"}
	nsAnnotations := map[string]string{"owner": "devops", "version": "v2.1.0"}

	namespace := makeNamespace(ns, nsLabels, nsAnnotations)
	n, c := makeDiscoveryWithMetadata(RoleService, NamespaceDiscovery{}, AttachMetadataConfig{Namespace: true}, namespace, makeService(ns))

	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 2,
		afterStart: func() {
			namespace.Labels["environment"] = "staging"
			namespace.Labels["region"] = "us-west"
			namespace.Annotations["owner"] = "sre"
			namespace.Annotations["cost-center"] = "engineering"
			c.CoreV1().Namespaces().Update(context.Background(), namespace, metav1.UpdateOptions{})
		},
		expectedRes: map[string]*targetgroup.Group{
			fmt.Sprintf("svc/%s/testservice", ns): {
				Targets: []model.LabelSet{
					{
						"__address__":                             "testservice.test-ns.svc:30900",
						"__meta_kubernetes_service_cluster_ip":    "10.0.0.1",
						"__meta_kubernetes_service_port_name":     "testport",
						"__meta_kubernetes_service_port_number":   "30900",
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__meta_kubernetes_service_type":          "ClusterIP",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                               model.LabelValue(ns),
					"__meta_kubernetes_namespace_annotation_owner":              "sre",
					"__meta_kubernetes_namespace_annotationpresent_owner":       "true",
					"__meta_kubernetes_namespace_annotation_version":            "v2.1.0",
					"__meta_kubernetes_namespace_annotationpresent_version":     "true",
					"__meta_kubernetes_namespace_annotation_cost_center":        "engineering",
					"__meta_kubernetes_namespace_annotationpresent_cost_center": "true",
					"__meta_kubernetes_namespace_label_environment":             "staging",
					"__meta_kubernetes_namespace_labelpresent_environment":      "true",
					"__meta_kubernetes_namespace_label_team":                    "frontend",
					"__meta_kubernetes_namespace_labelpresent_team":             "true",
					"__meta_kubernetes_namespace_label_region":                  "us-west",
					"__meta_kubernetes_namespace_labelpresent_region":           "true",
					"__meta_kubernetes_service_name":                            "testservice",
				},
				Source: fmt.Sprintf("svc/%s/testservice", ns),
			},
		},
	}.Run(t)
}
