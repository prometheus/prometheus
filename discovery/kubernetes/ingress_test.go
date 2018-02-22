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
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func ingressStoreKeyFunc(obj interface{}) (string, error) {
	return obj.(*v1beta1.Ingress).ObjectMeta.Name, nil
}

func newFakeIngressInformer() *fakeInformer {
	return newFakeInformer(ingressStoreKeyFunc)
}

func makeTestIngressDiscovery() (*Ingress, *fakeInformer) {
	i := newFakeIngressInformer()
	return NewIngress(nil, i), i
}

func makeIngress(tls []v1beta1.IngressTLS) *v1beta1.Ingress {
	return &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testingress",
			Namespace:   "default",
			Labels:      map[string]string{"testlabel": "testvalue"},
			Annotations: map[string]string{"testannotation": "testannotationvalue"},
		},
		Spec: v1beta1.IngressSpec{
			TLS: tls,
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
}

func expectedTargetGroups(tls bool) []*targetgroup.Group {
	scheme := "http"
	if tls {
		scheme = "https"
	}
	return []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{
					"__meta_kubernetes_ingress_scheme": lv(scheme),
					"__meta_kubernetes_ingress_host":   "example.com",
					"__meta_kubernetes_ingress_path":   "/",
					"__address__":                      "example.com",
				},
				{
					"__meta_kubernetes_ingress_scheme": lv(scheme),
					"__meta_kubernetes_ingress_host":   "example.com",
					"__meta_kubernetes_ingress_path":   "/foo",
					"__address__":                      "example.com",
				},
				{
					"__meta_kubernetes_ingress_scheme": lv(scheme),
					"__meta_kubernetes_ingress_host":   "test.example.com",
					"__address__":                      "test.example.com",
					"__meta_kubernetes_ingress_path":   "/",
				},
			},
			Labels: model.LabelSet{
				"__meta_kubernetes_ingress_name":                      "testingress",
				"__meta_kubernetes_namespace":                         "default",
				"__meta_kubernetes_ingress_label_testlabel":           "testvalue",
				"__meta_kubernetes_ingress_annotation_testannotation": "testannotationvalue",
			},
			Source: "ingress/default/testingress",
		},
	}
}

func TestIngressDiscoveryInitial(t *testing.T) {
	n, i := makeTestIngressDiscovery()
	i.GetStore().Add(makeIngress(nil))

	k8sDiscoveryTest{
		discovery:       n,
		expectedInitial: expectedTargetGroups(false),
	}.Run(t)
}

func TestIngressDiscoveryInitialTLS(t *testing.T) {
	n, i := makeTestIngressDiscovery()
	i.GetStore().Add(makeIngress([]v1beta1.IngressTLS{{}}))

	k8sDiscoveryTest{
		discovery:       n,
		expectedInitial: expectedTargetGroups(true),
	}.Run(t)
}
