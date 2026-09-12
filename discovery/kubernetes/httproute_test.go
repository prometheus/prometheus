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
	"fmt"
	"maps"
	"testing"

	"github.com/prometheus/common/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func pathMatch(v string) *gatewayv1.HTTPPathMatch {
	return &gatewayv1.HTTPPathMatch{Value: &v}
}

func makeHTTPRoute(namespace string) *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testhttproute",
			Namespace:   namespace,
			Labels:      map[string]string{"test/label": "testvalue"},
			Annotations: map[string]string{"test/annotation": "testannotationvalue"},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: "testgateway"},
				},
			},
			Hostnames: []gatewayv1.Hostname{"example.com"},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{Path: pathMatch("/")},
						{Path: pathMatch("/foo")},
					},
				},
				{
					// No matches, defaults to "/".
				},
			},
		},
	}
}

func expectedHTTPRouteTargetGroups(ns string) map[string]*targetgroup.Group {
	key := fmt.Sprintf("httproute/%s/testhttproute", ns)
	return map[string]*targetgroup.Group{
		key: {
			Targets: []model.LabelSet{
				{
					"__address__":                          "example.com",
					"__meta_kubernetes_httproute_hostname": "example.com",
					"__meta_kubernetes_httproute_path":     "/",
				},
				{
					"__address__":                          "example.com",
					"__meta_kubernetes_httproute_hostname": "example.com",
					"__meta_kubernetes_httproute_path":     "/foo",
				},
				{
					"__address__":                          "example.com",
					"__meta_kubernetes_httproute_hostname": "example.com",
					"__meta_kubernetes_httproute_path":     "/",
				},
			},
			Labels: model.LabelSet{
				"__meta_kubernetes_httproute_name":                              "testhttproute",
				"__meta_kubernetes_namespace":                                   lv(ns),
				"__meta_kubernetes_httproute_label_test_label":                  "testvalue",
				"__meta_kubernetes_httproute_labelpresent_test_label":           "true",
				"__meta_kubernetes_httproute_annotation_test_annotation":        "testannotationvalue",
				"__meta_kubernetes_httproute_annotationpresent_test_annotation": "true",
				"__meta_kubernetes_httproute_parent_ref_name":                   "testgateway",
			},
			Source: key,
		},
	}
}

func TestHTTPRouteDiscoveryAdd(t *testing.T) {
	t.Parallel()
	n, c := makeGatewayDiscovery(NamespaceDiscovery{Names: []string{"default"}}, AttachMetadataConfig{}, nil, nil)
	n.role = RoleHTTPRoute

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeHTTPRoute("default")
			c.GatewayV1().HTTPRoutes("default").Create(t.Context(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedHTTPRouteTargetGroups("default"),
	}.Run(t)
}

func TestHTTPRouteDiscoveryNamespaces(t *testing.T) {
	t.Parallel()
	n, c := makeGatewayDiscovery(NamespaceDiscovery{Names: []string{"ns1", "ns2"}}, AttachMetadataConfig{}, nil, nil)
	n.role = RoleHTTPRoute

	expected := expectedHTTPRouteTargetGroups("ns1")
	maps.Copy(expected, expectedHTTPRouteTargetGroups("ns2"))
	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				obj := makeHTTPRoute(ns)
				c.GatewayV1().HTTPRoutes(obj.Namespace).Create(t.Context(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes:      expected,
	}.Run(t)
}
