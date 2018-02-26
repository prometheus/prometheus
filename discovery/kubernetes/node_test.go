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
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
)

type fakeInformer struct {
	store    cache.Store
	handlers []cache.ResourceEventHandler

	blockDeltas sync.Mutex
}

func newFakeInformer(f func(obj interface{}) (string, error)) *fakeInformer {
	i := &fakeInformer{
		store: cache.NewStore(f),
	}
	// We want to make sure that all delta events (Add/Update/Delete) are blocked
	// until our handlers to test have been added.
	i.blockDeltas.Lock()
	return i
}

func (i *fakeInformer) AddEventHandler(h cache.ResourceEventHandler) {
	i.handlers = append(i.handlers, h)
	// Only now that there is a registered handler, we are able to handle deltas.
	i.blockDeltas.Unlock()
}

func (i *fakeInformer) AddEventHandlerWithResyncPeriod(h cache.ResourceEventHandler, _ time.Duration) {
	i.AddEventHandler(h)
}

func (i *fakeInformer) GetStore() cache.Store {
	return i.store
}

func (i *fakeInformer) GetController() cache.Controller {
	return nil
}

func (i *fakeInformer) Run(stopCh <-chan struct{}) {
}

func (i *fakeInformer) HasSynced() bool {
	return true
}

func (i *fakeInformer) LastSyncResourceVersion() string {
	return "0"
}

func (i *fakeInformer) Add(obj interface{}) {
	i.blockDeltas.Lock()
	defer i.blockDeltas.Unlock()

	for _, h := range i.handlers {
		h.OnAdd(obj)
	}
}

func (i *fakeInformer) Delete(obj interface{}) {
	i.blockDeltas.Lock()
	defer i.blockDeltas.Unlock()

	for _, h := range i.handlers {
		h.OnDelete(obj)
	}
}

func (i *fakeInformer) Update(obj interface{}) {
	i.blockDeltas.Lock()
	defer i.blockDeltas.Unlock()

	for _, h := range i.handlers {
		h.OnUpdate(nil, obj)
	}
}

type discoverer interface {
	Run(ctx context.Context, up chan<- []*targetgroup.Group)
}

type k8sDiscoveryTest struct {
	discovery       discoverer
	afterStart      func()
	expectedInitial []*targetgroup.Group
	expectedRes     []*targetgroup.Group
}

func (d k8sDiscoveryTest) Run(t *testing.T) {
	ch := make(chan []*targetgroup.Group)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)
	defer cancel()
	go func() {
		d.discovery.Run(ctx, ch)
	}()

	initialRes := <-ch
	if d.expectedInitial != nil {
		requireTargetGroups(t, d.expectedInitial, initialRes)
	}

	if d.afterStart != nil && d.expectedRes != nil {
		d.afterStart()
		res := <-ch

		requireTargetGroups(t, d.expectedRes, res)
	}
}

func requireTargetGroups(t *testing.T, expected, res []*targetgroup.Group) {
	b1, err := json.Marshal(expected)
	if err != nil {
		panic(err)
	}
	b2, err := json.Marshal(res)
	if err != nil {
		panic(err)
	}

	require.JSONEq(t, string(b1), string(b2))
}

func nodeStoreKeyFunc(obj interface{}) (string, error) {
	return obj.(*v1.Node).ObjectMeta.Name, nil
}

func newFakeNodeInformer() *fakeInformer {
	return newFakeInformer(nodeStoreKeyFunc)
}

func makeTestNodeDiscovery() (*Node, *fakeInformer) {
	i := newFakeNodeInformer()
	return NewNode(nil, i), i
}

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

func TestNodeDiscoveryInitial(t *testing.T) {
	n, i := makeTestNodeDiscovery()
	i.GetStore().Add(makeNode(
		"test",
		"1.2.3.4",
		map[string]string{"testlabel": "testvalue"},
		map[string]string{"testannotation": "testannotationvalue"},
	))

	k8sDiscoveryTest{
		discovery: n,
		expectedInitial: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_name":                      "test",
					"__meta_kubernetes_node_label_testlabel":           "testvalue",
					"__meta_kubernetes_node_annotation_testannotation": "testannotationvalue",
				},
				Source: "node/test",
			},
		},
	}.Run(t)
}

func TestNodeDiscoveryAdd(t *testing.T) {
	n, i := makeTestNodeDiscovery()

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Add(makeEnumeratedNode(1)) }() },
		expectedRes: []*targetgroup.Group{
			{
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
	n, i := makeTestNodeDiscovery()
	i.GetStore().Add(makeEnumeratedNode(0))

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Delete(makeEnumeratedNode(0)) }() },
		expectedInitial: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test0",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_name": "test0",
				},
				Source: "node/test0",
			},
		},
		expectedRes: []*targetgroup.Group{
			{
				Source: "node/test0",
			},
		},
	}.Run(t)
}

func TestNodeDiscoveryDeleteUnknownCacheState(t *testing.T) {
	n, i := makeTestNodeDiscovery()
	i.GetStore().Add(makeEnumeratedNode(0))

	k8sDiscoveryTest{
		discovery:  n,
		afterStart: func() { go func() { i.Delete(cache.DeletedFinalStateUnknown{Obj: makeEnumeratedNode(0)}) }() },
		expectedInitial: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test0",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_name": "test0",
				},
				Source: "node/test0",
			},
		},
		expectedRes: []*targetgroup.Group{
			{
				Source: "node/test0",
			},
		},
	}.Run(t)
}

func TestNodeDiscoveryUpdate(t *testing.T) {
	n, i := makeTestNodeDiscovery()
	i.GetStore().Add(makeEnumeratedNode(0))

	k8sDiscoveryTest{
		discovery: n,
		afterStart: func() {
			go func() {
				i.Update(
					makeNode(
						"test0",
						"1.2.3.4",
						map[string]string{"Unschedulable": "true"},
						map[string]string{},
					),
				)
			}()
		},
		expectedInitial: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test0",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_name": "test0",
				},
				Source: "node/test0",
			},
		},
		expectedRes: []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{
						"__address__": "1.2.3.4:10250",
						"instance":    "test0",
						"__meta_kubernetes_node_address_InternalIP": "1.2.3.4",
					},
				},
				Labels: model.LabelSet{
					"__meta_kubernetes_node_label_Unschedulable": "true",
					"__meta_kubernetes_node_name":                "test0",
				},
				Source: "node/test0",
			},
		},
	}.Run(t)
}
