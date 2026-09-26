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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

// makeGatewayDiscovery creates a kubernetes.Discovery instance wired to a fake
// Gateway API clientset for testing. coreObjects seeds the regular Kubernetes
// fake clientset (used for namespace metadata), gatewayObjects seeds the fake
// Gateway API clientset.
func makeGatewayDiscovery(nsDiscovery NamespaceDiscovery, attachMetadata AttachMetadataConfig, coreObjects, gatewayObjects []runtime.Object) (*Discovery, gatewayapi.Interface) {
	clientset := fake.NewClientset(coreObjects...)
	gwClientset := gatewayfake.NewSimpleClientset(gatewayObjects...)

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := newDiscovererMetrics(reg, refreshMetrics)
	err := metrics.Register()
	if err != nil {
		panic(err)
	}

	kubeMetrics, ok := metrics.(*kubernetesMetrics)
	if !ok {
		panic("invalid discovery metrics type")
	}

	d := &Discovery{
		client:             clientset,
		gatewayClient:      gwClientset,
		logger:             promslog.NewNopLogger(),
		role:               RoleGateway,
		namespaceDiscovery: &nsDiscovery,
		ownNamespace:       "own-ns",
		attachMetadata:     attachMetadata,
		metrics:            kubeMetrics,
	}

	return d, gwClientset
}

func makeGateway(namespace string) *gatewayv1.Gateway {
	hostname := gatewayv1.Hostname("example.com")
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testgateway",
			Namespace:   namespace,
			Labels:      map[string]string{"test/label": "testvalue"},
			Annotations: map[string]string{"test/annotation": "testannotationvalue"},
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "testclass",
			Listeners: []gatewayv1.Listener{
				{
					Name:     "http",
					Hostname: &hostname,
					Port:     80,
					Protocol: gatewayv1.HTTPProtocolType,
				},
				{
					Name:     "https",
					Hostname: &hostname,
					Port:     443,
					Protocol: gatewayv1.HTTPSProtocolType,
				},
			},
		},
	}
}

func expectedGatewayTargetGroups(ns string) map[string]*targetgroup.Group {
	key := fmt.Sprintf("gateway/%s/testgateway", ns)
	return map[string]*targetgroup.Group{
		key: {
			Targets: []model.LabelSet{
				{
					"__address__": "example.com:80",
					"__meta_kubernetes_gateway_listener_name":     "http",
					"__meta_kubernetes_gateway_listener_hostname": "example.com",
					"__meta_kubernetes_gateway_listener_port":     "80",
					"__meta_kubernetes_gateway_listener_protocol": "HTTP",
				},
				{
					"__address__": "example.com:443",
					"__meta_kubernetes_gateway_listener_name":     "https",
					"__meta_kubernetes_gateway_listener_hostname": "example.com",
					"__meta_kubernetes_gateway_listener_port":     "443",
					"__meta_kubernetes_gateway_listener_protocol": "HTTPS",
				},
			},
			Labels: model.LabelSet{
				"__meta_kubernetes_gateway_name":                              "testgateway",
				"__meta_kubernetes_namespace":                                 lv(ns),
				"__meta_kubernetes_gateway_label_test_label":                  "testvalue",
				"__meta_kubernetes_gateway_labelpresent_test_label":           "true",
				"__meta_kubernetes_gateway_annotation_test_annotation":        "testannotationvalue",
				"__meta_kubernetes_gateway_annotationpresent_test_annotation": "true",
				"__meta_kubernetes_gateway_class_name":                        "testclass",
			},
			Source: key,
		},
	}
}

func TestGatewayDiscoveryAdd(t *testing.T) {
	t.Parallel()
	n, c := makeGatewayDiscovery(NamespaceDiscovery{Names: []string{"default"}}, AttachMetadataConfig{}, nil, nil)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeGateway("default")
			c.GatewayV1().Gateways("default").Create(t.Context(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expectedGatewayTargetGroups("default"),
	}.Run(t)
}

func TestGatewayDiscoveryNamespaces(t *testing.T) {
	t.Parallel()
	n, c := makeGatewayDiscovery(NamespaceDiscovery{Names: []string{"ns1", "ns2"}}, AttachMetadataConfig{}, nil, nil)

	expected := expectedGatewayTargetGroups("ns1")
	maps.Copy(expected, expectedGatewayTargetGroups("ns2"))
	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			for _, ns := range []string{"ns1", "ns2"} {
				obj := makeGateway(ns)
				c.GatewayV1().Gateways(obj.Namespace).Create(t.Context(), obj, metav1.CreateOptions{})
			}
		},
		expectedMaxItems: 2,
		expectedRes:      expected,
	}.Run(t)
}

func TestGatewayDiscoveryWithNamespaceMetadata(t *testing.T) {
	t.Parallel()

	ns := "test-ns"
	nsLabels := map[string]string{"service": "web", "layer": "frontend"}
	nsAnnotations := map[string]string{"contact": "platform", "release": "v5.6.7"}

	namespace := makeNamespace(ns, nsLabels, nsAnnotations)
	n, c := makeGatewayDiscovery(NamespaceDiscovery{}, AttachMetadataConfig{Namespace: true}, []runtime.Object{namespace}, nil)

	expected := expectedGatewayTargetGroups(ns)
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_label_service"] = "web"
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_labelpresent_service"] = "true"
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_label_layer"] = "frontend"
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_labelpresent_layer"] = "true"
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_annotation_contact"] = "platform"
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_annotationpresent_contact"] = "true"
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_annotation_release"] = "v5.6.7"
	expected[fmt.Sprintf("gateway/%s/testgateway", ns)].Labels["__meta_kubernetes_namespace_annotationpresent_release"] = "true"

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeGateway(ns)
			c.GatewayV1().Gateways(ns).Create(t.Context(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes:      expected,
	}.Run(t)
}
