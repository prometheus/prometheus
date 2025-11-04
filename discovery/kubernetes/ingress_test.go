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
	"maps"
	"testing"

	"github.com/prometheus/common/model"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type TLSMode int

const (
	TLSNo TLSMode = iota
	TLSYes
	TLSMixed
	TLSWildcard
)

func makeIngress(namespace string, tls TLSMode) *v1.Ingress {
	ret := &v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testingress",
			Namespace:   namespace,
			Labels:      map[string]string{"test/label": "testvalue"},
			Annotations: map[string]string{"test/annotation": "testannotationvalue"},
		},
		Spec: v1.IngressSpec{
			IngressClassName: classString("testclass"),
			TLS:              nil,
			Rules: []v1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{Path: "/"},
								{Path: "/foo"},
							},
						},
					},
				},
				{
					// No backend config, ignored
					Host: "nobackend.example.com",
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{},
					},
				},
				{
					Host: "test.example.com",
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{{}},
						},
					},
				},
			},
		},
	}

	switch tls {
	case TLSYes:
		ret.Spec.TLS = []v1.IngressTLS{{Hosts: []string{"example.com", "test.example.com"}}}
	case TLSMixed:
		ret.Spec.TLS = []v1.IngressTLS{{Hosts: []string{"example.com"}}}
	case TLSWildcard:
		ret.Spec.TLS = []v1.IngressTLS{{Hosts: []string{"*.example.com"}}}
	}

	return ret
}

func classString(v string) *string {
	return &v
}

func expectedTargetGroups(ns string, tls TLSMode) map[string]*targetgroup.Group {
	scheme1 := "http"
	scheme2 := "http"

	switch tls {
	case TLSYes:
		scheme1 = "https"
		scheme2 = "https"
	case TLSMixed:
		scheme1 = "https"
	case TLSWildcard:
		scheme2 = "https"
	}

	key := fmt.Sprintf("ingress/%s/testingress", ns)
	return map[string]*targetgroup.Group{
		key: {
			Targets: []model.LabelSet{
				{
					"__meta_kubernetes_ingress_scheme": lv(scheme1),
					"__meta_kubernetes_ingress_host":   "example.com",
					"__meta_kubernetes_ingress_path":   "/",
					"__address__":                      "example.com",
				},
				{
					"__meta_kubernetes_ingress_scheme": lv(scheme1),
					"__meta_kubernetes_ingress_host":   "example.com",
					"__meta_kubernetes_ingress_path":   "/foo",
					"__address__":                      "example.com",
				},
				{
					"__meta_kubernetes_ingress_scheme": lv(scheme2),
					"__meta_kubernetes_ingress_host":   "test.example.com",
					"__address__":                      "test.example.com",
					"__meta_kubernetes_ingress_path":   "/",
				},
			},
			Labels: model.LabelSet{
				"__meta_kubernetes_ingress_name":                              "testingress",
				"__meta_kubernetes_namespace":                                 lv(ns),
				"__meta_kubernetes_ingress_label_test_label":                  "testvalue",
				"__meta_kubernetes_ingress_labelpresent_test_label":           "true",
				"__meta_kubernetes_ingress_annotation_test_annotation":        "testannotationvalue",
				"__meta_kubernetes_ingress_annotationpresent_test_annotation": "true",
				"__meta_kubernetes_ingress_class_name":                        "testclass",
			},
			Source: key,
		},
	}
}

func TestIngressDiscoveryAdd(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"default"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngress("default", TLSNo)
			c.NetworkingV1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSNo),
	}.Run(t)
}

func TestIngressDiscoveryAddTLS(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"default"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngress("default", TLSYes)
			c.NetworkingV1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSYes),
	}.Run(t)
}

func TestIngressDiscoveryAddMixed(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"default"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngress("default", TLSMixed)
			c.NetworkingV1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSMixed),
	}.Run(t)
}

func TestIngressDiscoveryNamespaces(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"ns1", "ns2"}})

	expected := expectedTargetGroups("ns1", TLSNo)
	maps.Copy(expected, expectedTargetGroups("ns2", TLSNo))
	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				obj := makeIngress(ns, TLSNo)
				c.NetworkingV1().Ingresses(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes:      expected,
	}.Run(t)
}

func TestIngressDiscoveryOwnNamespace(t *testing.T) {
	t.Parallel()
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{IncludeOwnNamespace: true})

	expected := expectedTargetGroups("own-ns", TLSNo)
	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"own-ns", "non-own-ns"} {
				obj := makeIngress(ns, TLSNo)
				c.NetworkingV1().Ingresses(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 1,
		expectedRes:      expected,
	}.Run(t)
}

func TestIngressDiscoveryWithNamespaceMetadata(t *testing.T) {
	t.Parallel()

	ns := "test-ns"
	nsLabels := map[string]string{"service": "web", "layer": "frontend"}
	nsAnnotations := map[string]string{"contact": "platform", "release": "v5.6.7"}

	n, _ := makeDiscoveryWithMetadata(RoleIngress, NamespaceDiscovery{}, AttachMetadataConfig{Namespace: true}, makeNamespace(ns, nsLabels, nsAnnotations), makeIngress(ns, TLSNo))
	k8sDiscoveryTest{
		discovery:        n,
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			fmt.Sprintf("ingress/%s/testingress", ns): {
				Targets: []model.LabelSet{
					{
						"__address__":                      "example.com",
						"__meta_kubernetes_ingress_host":   "example.com",
						"__meta_kubernetes_ingress_path":   "/",
						"__meta_kubernetes_ingress_scheme": "http",
					},
					{
						"__address__":                      "example.com",
						"__meta_kubernetes_ingress_host":   "example.com",
						"__meta_kubernetes_ingress_path":   "/foo",
						"__meta_kubernetes_ingress_scheme": "http",
					},
					{
						"__address__":                      "test.example.com",
						"__meta_kubernetes_ingress_host":   "test.example.com",
						"__meta_kubernetes_ingress_path":   "/",
						"__meta_kubernetes_ingress_scheme": "http",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                 model.LabelValue(ns),
					"__meta_kubernetes_namespace_annotation_contact":              "platform",
					"__meta_kubernetes_namespace_annotationpresent_contact":       "true",
					"__meta_kubernetes_namespace_annotation_release":              "v5.6.7",
					"__meta_kubernetes_namespace_annotationpresent_release":       "true",
					"__meta_kubernetes_namespace_label_service":                   "web",
					"__meta_kubernetes_namespace_labelpresent_service":            "true",
					"__meta_kubernetes_namespace_label_layer":                     "frontend",
					"__meta_kubernetes_namespace_labelpresent_layer":              "true",
					"__meta_kubernetes_ingress_name":                              "testingress",
					"__meta_kubernetes_ingress_label_test_label":                  "testvalue",
					"__meta_kubernetes_ingress_labelpresent_test_label":           "true",
					"__meta_kubernetes_ingress_annotation_test_annotation":        "testannotationvalue",
					"__meta_kubernetes_ingress_annotationpresent_test_annotation": "true",
					"__meta_kubernetes_ingress_class_name":                        "testclass",
				},
				Source: fmt.Sprintf("ingress/%s/testingress", ns),
			},
		},
	}.Run(t)
}

func TestIngressDiscoveryWithUpdatedNamespaceMetadata(t *testing.T) {
	t.Parallel()

	ns := "test-ns"
	nsLabels := map[string]string{"component": "database", "layer": "backend"}
	nsAnnotations := map[string]string{"contact": "dba", "release": "v6.7.8"}

	namespace := makeNamespace(ns, nsLabels, nsAnnotations)
	n, c := makeDiscoveryWithMetadata(RoleIngress, NamespaceDiscovery{}, AttachMetadataConfig{Namespace: true}, namespace, makeIngress(ns, TLSNo))

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
			fmt.Sprintf("ingress/%s/testingress", ns): {
				Targets: []model.LabelSet{
					{
						"__address__":                      "example.com",
						"__meta_kubernetes_ingress_host":   "example.com",
						"__meta_kubernetes_ingress_path":   "/",
						"__meta_kubernetes_ingress_scheme": "http",
					},
					{
						"__address__":                      "example.com",
						"__meta_kubernetes_ingress_host":   "example.com",
						"__meta_kubernetes_ingress_path":   "/foo",
						"__meta_kubernetes_ingress_scheme": "http",
					},
					{
						"__address__":                      "test.example.com",
						"__meta_kubernetes_ingress_host":   "test.example.com",
						"__meta_kubernetes_ingress_path":   "/",
						"__meta_kubernetes_ingress_scheme": "http",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_namespace":                                 model.LabelValue(ns),
					"__meta_kubernetes_namespace_annotation_contact":              "sre",
					"__meta_kubernetes_namespace_annotationpresent_contact":       "true",
					"__meta_kubernetes_namespace_annotation_release":              "v6.7.8",
					"__meta_kubernetes_namespace_annotationpresent_release":       "true",
					"__meta_kubernetes_namespace_annotation_monitoring":           "enabled",
					"__meta_kubernetes_namespace_annotationpresent_monitoring":    "true",
					"__meta_kubernetes_namespace_label_component":                 "cache",
					"__meta_kubernetes_namespace_labelpresent_component":          "true",
					"__meta_kubernetes_namespace_label_layer":                     "backend",
					"__meta_kubernetes_namespace_labelpresent_layer":              "true",
					"__meta_kubernetes_namespace_label_region":                    "us-central",
					"__meta_kubernetes_namespace_labelpresent_region":             "true",
					"__meta_kubernetes_ingress_name":                              "testingress",
					"__meta_kubernetes_ingress_label_test_label":                  "testvalue",
					"__meta_kubernetes_ingress_labelpresent_test_label":           "true",
					"__meta_kubernetes_ingress_annotation_test_annotation":        "testannotationvalue",
					"__meta_kubernetes_ingress_annotationpresent_test_annotation": "true",
					"__meta_kubernetes_ingress_class_name":                        "testclass",
				},
				Source: fmt.Sprintf("ingress/%s/testingress", ns),
			},
		},
	}.Run(t)
}
