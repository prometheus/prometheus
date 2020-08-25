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

func makeNode(name, address string, labels map[string]string, annotations map[string]string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: address,
				},
			},
			DaemonEndpoints: v1.NodeDaemonEndpoints{
				KubeletEndpoint: v1.DaemonEndpoint{
					Port: 10250,
				},
			},
		},
	}
}

func makeEnumeratedNode(i int) *v1.Node {
	return makeNode(fmt.Sprintf("test%d", i), "1.2.3.4", map[string]string{}, map[string]string{})
}

func TestNodeDiscoveryBeforeStart(t *testing.T) {
	n, c := makeDiscovery(RoleNode, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		beforeRun: func() {
			obj := makeNode(
				"test",
				"1.2.3.4",
				map[string]string{"test-label": "testvalue"},
				map[string]string{"test-annotation": "testannotationvalue"},
			)
			c.CoreV1().Nodes().Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"node/test": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_name":                              "test",
					"__meta_kubernetes_node_label_test_label":                  "testvalue",
					"__meta_kubernetes_node_labelpresent_test_label":           "true",
					"__meta_kubernetes_node_annotation_test_annotation":        "testannotationvalue",
					"__meta_kubernetes_node_annotationpresent_test_annotation": "true",
				},
				Source: "node/test",
			},
		},
	}.Run(t)
}

func TestNodeDiscoveryAdd(t *testing.T) {
	n, c := makeDiscovery(RoleNode, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj := makeEnumeratedNode(1)
			c.CoreV1().Nodes().Create(context.Background(), obj, metav1.CreateOptions{})
		},
		expectedMaxItems: 1,
		expectedRes: map[string]*targetgroup.Group{
			"node/test1": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test1",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_name": "test1",
				},
				Source: "node/test1",
			},
		},
	}.Run(t)
}

func TestNodeDiscoveryDelete(t *testing.T) {
	obj := makeEnumeratedNode(0)
	n, c := makeDiscovery(RoleNode, NamespaceDiscovery{}, obj)

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			c.CoreV1().Nodes().Delete(context.Background(), obj.Name, metav1.DeleteOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"node/test0": {
				Source: "node/test0",
			},
		},
	}.Run(t)
}

func TestNodeDiscoveryUpdate(t *testing.T) {
	n, c := makeDiscovery(RoleNode, NamespaceDiscovery{})

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			obj1 := makeEnumeratedNode(0)
			c.CoreV1().Nodes().Create(context.Background(), obj1, metav1.CreateOptions{})
			obj2 := makeNode(
				"test0",
				"1.2.3.4",
				map[string]string{"Unschedulable": "true"},
				map[string]string{},
			)
			c.CoreV1().Nodes().Update(context.Background(), obj2, metav1.UpdateOptions{})
		},
		expectedMaxItems: 2,
		expectedRes: map[string]*targetgroup.Group{
			"node/test0": {
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test0",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_label_Unschedulable":        "true",
					"__meta_kubernetes_node_labelpresent_Unschedulable": "true",
					"__meta_kubernetes_node_name":                       "test0",
				},
				Source: "node/test0",
			},
		},
	}.Run(t)
}
