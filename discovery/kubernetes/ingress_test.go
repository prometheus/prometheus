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
	v1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
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

func makeIngress(tls TLSMode) *v1.Ingress {
	ret := &v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testingress",
			Namespace:   "default",
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

func makeIngressV1beta1(tls TLSMode) *v1beta1.Ingress {
	ret := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testingress",
			Namespace:   "default",
			Labels:      map[string]string{"test/label": "testvalue"},
			Annotations: map[string]string{"test/annotation": "testannotationvalue"},
		},
		Spec: v1beta1.IngressSpec{
			IngressClassName: classString("testclass"),
			TLS:              nil,
			Rules: []v1beta1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{Path: "/"},
								{Path: "/foo"},
							},
						},
					},
				},
				{
					// No backend config, ignored
					Host: "nobackend.example.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{},
					},
				},
				{
					Host: "test.example.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{{}},
						},
					},
				},
			},
		},
	}

	switch tls {
	case TLSYes:
		ret.Spec.TLS = []v1beta1.IngressTLS{{Hosts: []string{"example.com", "test.example.com"}}}
	case TLSMixed:
		ret.Spec.TLS = []v1beta1.IngressTLS{{Hosts: []string{"example.com"}}}
	case TLSWildcard:
		ret.Spec.TLS = []v1beta1.IngressTLS{{Hosts: []string{"*.example.com"}}}
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
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"default"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngress(TLSNo)
			c.NetworkingV1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSNo),
	}.Run(t)
}

func TestIngressDiscoveryAddV1beta1(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleIngress, NamespaceDiscovery{Names: []string{"default"}}, "v1.18.0")

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngressV1beta1(TLSNo)
			c.NetworkingV1beta1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSNo),
	}.Run(t)
}

func TestIngressDiscoveryAddTLS(t *testing.T) {
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"default"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngress(TLSYes)
			c.NetworkingV1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSYes),
	}.Run(t)
}

func TestIngressDiscoveryAddTLSV1beta1(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleIngress, NamespaceDiscovery{Names: []string{"default"}}, "v1.18.0")

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngressV1beta1(TLSYes)
			c.NetworkingV1beta1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSYes),
	}.Run(t)
}

func TestIngressDiscoveryAddMixed(t *testing.T) {
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"default"}})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngress(TLSMixed)
			c.NetworkingV1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSMixed),
	}.Run(t)
}

func TestIngressDiscoveryAddMixedV1beta1(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleIngress, NamespaceDiscovery{Names: []string{"default"}}, "v1.18.0")

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeIngressV1beta1(TLSMixed)
			c.NetworkingV1beta1().Ingresses("default").Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedTargetGroups("default", TLSMixed),
	}.Run(t)
}

func TestIngressDiscoveryNamespaces(t *testing.T) {
	n, c := makeDiscovery(RoleIngress, NamespaceDiscovery{Names: []string{"ns1", "ns2"}})

	expected := expectedTargetGroups("ns1", TLSNo)
	for k, v := range expectedTargetGroups("ns2", TLSNo) {
		expected[k] = v
	}
	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				obj := makeIngress(TLSNo)
				obj.Namespace = ns
				c.NetworkingV1().Ingresses(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes:      expected,
	}.Run(t)
}

func TestIngressDiscoveryNamespacesV1beta1(t *testing.T) {
	n, c := makeDiscoveryWithVersion(RoleIngress, NamespaceDiscovery{Names: []string{"ns1", "ns2"}}, "v1.18.0")

	expected := expectedTargetGroups("ns1", TLSNo)
	for k, v := range expectedTargetGroups("ns2", TLSNo) {
		expected[k] = v
	}
	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				obj := makeIngressV1beta1(TLSNo)
				obj.Namespace = ns
				c.NetworkingV1beta1().Ingresses(obj.Namespace).Create(context.Background(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes:      expected,
	}.Run(t)
}
