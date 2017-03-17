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
	"fmt"
	"testing"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/cache"
)

func serviceStoreKeyFunc(obj interface{}) (string, error) {
	return obj.(*v1.Service).ObjectMeta.Name, nil
}

func newFakeServiceInformer() *fakeInformer {
	return newFakeInformer(serviceStoreKeyFunc)
}

func makeTestServiceDiscovery() (*Service, *fakeInformer) {
	i := newFakeServiceInformer()
	return NewService(log.Base(), i), i
}

func makeMultiPortService() *v1.Service {
	return &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        "testservice",
			Namespace:   "default",
			Labels:      map[string]string{"testlabel": "testvalue"},
			Annotations: map[string]string{"testannotation": "testannotationvalue"},
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
		},
	}
}

func makeSuffixedService(suffix string) *v1.Service {
	return &v1.Service{
		ObjectMeta: v1.ObjectMeta{
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
		},
	}
}

func makeService() *v1.Service {
	return makeSuffixedService("")
}

func TestServiceDiscoveryInitial(t *testing.T) {
	n, i := makeTestServiceDiscovery()
	i.GetStore().Add(makeMultiPortService())

	k8sDiscoveryTest{
		discovery: n,
		expectedInitial: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                             "testservice.default.svc:30900",
						"__meta_kubernetes_service_port_name":     "testport0",
					},
					{
						"__meta_kubernetes_service_port_protocol": "UDP",
						"__address__":                             "testservice.default.svc:30901",
						"__meta_kubernetes_service_port_name":     "testport1",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name":                      "testservice",
					"__meta_kubernetes_namespace":                         "default",
					"__meta_kubernetes_service_label_testlabel":           "testvalue",
					"__meta_kubernetes_service_annotation_testannotation": "testannotationvalue",
				},
				Source: "svc/default/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryAdd(t *testing.T) {
	n, i := makeTestServiceDiscovery()

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Add(makeService()) }() },
		expectedRes: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                             "testservice.default.svc:30900",
						"__meta_kubernetes_service_port_name":     "testport",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryDelete(t *testing.T) {
	n, i := makeTestServiceDiscovery()
	i.GetStore().Add(makeService())

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Delete(makeService()) }() },
		expectedInitial: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                             "testservice.default.svc:30900",
						"__meta_kubernetes_service_port_name":     "testport",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/testservice",
			},
		},
		expectedRes: []*config.TargetGroup{
			{
				Source: "svc/default/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryDeleteUnknownCacheState(t *testing.T) {
	n, i := makeTestServiceDiscovery()
	i.GetStore().Add(makeService())

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Delete(cache.DeletedFinalStateUnknown{Obj: makeService()}) }() },
		expectedInitial: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                             "testservice.default.svc:30900",
						"__meta_kubernetes_service_port_name":     "testport",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/testservice",
			},
		},
		expectedRes: []*config.TargetGroup{
			{
				Source: "svc/default/testservice",
			},
		},
	}.Run(t)
}

func TestServiceDiscoveryUpdate(t *testing.T) {
	n, i := makeTestServiceDiscovery()
	i.GetStore().Add(makeService())

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Update(makeMultiPortService()) }() },
		expectedInitial: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                             "testservice.default.svc:30900",
						"__meta_kubernetes_service_port_name":     "testport",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name": "testservice",
					"__meta_kubernetes_namespace":    "default",
				},
				Source: "svc/default/testservice",
			},
		},
		expectedRes: []*config.TargetGroup{
			{
				Targets: []model.LabelSet{
					{
						"__meta_kubernetes_service_port_protocol": "TCP",
						"__address__":                             "testservice.default.svc:30900",
						"__meta_kubernetes_service_port_name":     "testport0",
					},
					{
						"__meta_kubernetes_service_port_protocol": "UDP",
						"__address__":                             "testservice.default.svc:30901",
						"__meta_kubernetes_service_port_name":     "testport1",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_service_name":                      "testservice",
					"__meta_kubernetes_namespace":                         "default",
					"__meta_kubernetes_service_label_testlabel":           "testvalue",
					"__meta_kubernetes_service_annotation_testannotation": "testannotationvalue",
				},
				Source: "svc/default/testservice",
			},
		},
	}.Run(t)
}
