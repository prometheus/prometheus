// Copyright 2018 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

// makeDiscovery creates a kubernetes.Discovery instance for testing.
func makeDiscovery(role Role, nsDiscovery NamespaceDiscovery, objects ...runtime.Object) (*Discovery, kubernetes.Interface) {
	clientset := fake.NewSimpleClientset(objects...)

	return &Discovery{
		client:             clientset,
		logger:             log.NewNopLogger(),
		role:               role,
		namespaceDiscovery: &nsDiscovery,
	}, clientset
}

type k8sDiscoveryTest struct {
	// discovery is instance of discovery.Discoverer
	discovery discoverer
	// beforeRun runs before discoverer run
	beforeRun func()
	// afterStart runs after discoverer has synced
	afterStart func()
	// expectedMaxItems is expected max items we may get from channel
	expectedMaxItems int
	// expectedRes is expected final result
	expectedRes map[string]*targetgroup.Group
}

func (d k8sDiscoveryTest) Run(t *testing.T) {
	t.Helper()
	ch := make(chan []*targetgroup.Group)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if d.beforeRun != nil {
		d.beforeRun()
	}

	// Run discoverer and start a goroutine to read results.
	go d.discovery.Run(ctx, ch)

	// Ensure that discovery has a discoverer set. This prevents a race
	// condition where the above go routine may or may not have set a
	// discoverer yet.
	for {
		dis := d.discovery.(*Discovery)
		dis.RLock()
		l := len(dis.discoverers)
		dis.RUnlock()
		if l > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	resChan := make(chan map[string]*targetgroup.Group)
	go readResultWithTimeout(t, ch, d.expectedMaxItems, time.Second, resChan)

	dd, ok := d.discovery.(hasSynced)
	if !ok {
		t.Errorf("discoverer does not implement hasSynced interface")
		return
	}
	if !cache.WaitForCacheSync(ctx.Done(), dd.hasSynced) {
		t.Errorf("discoverer failed to sync: %v", dd)
		return
	}

	if d.afterStart != nil {
		d.afterStart()
	}

	if d.expectedRes != nil {
		res := <-resChan
		requireTargetGroups(t, d.expectedRes, res)
	}
}

// readResultWithTimeout reads all targegroups from channel with timeout.
// It merges targegroups by source and sends the result to result channel.
func readResultWithTimeout(t *testing.T, ch <-chan []*targetgroup.Group, max int, timeout time.Duration, resChan chan<- map[string]*targetgroup.Group) {
	res := make(map[string]*targetgroup.Group)
Loop:
	for {
		select {
		case tgs := <-ch:
			for _, tg := range tgs {
				if tg == nil {
					continue
				}
				res[tg.Source] = tg
			}
			if len(res) == max {
				// Reached max target groups we may get, break fast.
				break Loop
			}
		case <-time.After(timeout):
			// Because we use queue, an object that is created then
			// deleted or updated may be processed only once.
			// So possibly we may skip events, timed out here.
			t.Logf("timed out, got %d (max: %d) items, some events are skipped", len(res), max)
			break Loop
		}
	}

	resChan <- res
}

func requireTargetGroups(t *testing.T, expected, res map[string]*targetgroup.Group) {
	t.Helper()
	b1, err := json.Marshal(expected)
	if err != nil {
		panic(err)
	}
	b2, err := json.Marshal(res)
	if err != nil {
		panic(err)
	}

	testutil.Equals(t, string(b1), string(b2))
}

type hasSynced interface {
	// hasSynced returns true if all informers synced.
	// This is only used in testing to determine when discoverer synced to
	// kubernetes apiserver.
	hasSynced() bool
}

var _ hasSynced = &Discovery{}
var _ hasSynced = &Node{}
var _ hasSynced = &Endpoints{}
var _ hasSynced = &Ingress{}
var _ hasSynced = &Pod{}
var _ hasSynced = &Service{}

func (d *Discovery) hasSynced() bool {
	d.RLock()
	defer d.RUnlock()
	for _, discoverer := range d.discoverers {
		if hasSynceddiscoverer, ok := discoverer.(hasSynced); ok {
			if !hasSynceddiscoverer.hasSynced() {
				return false
			}
		}
	}
	return true
}

func (n *Node) hasSynced() bool {
	return n.informer.HasSynced()
}

func (e *Endpoints) hasSynced() bool {
	return e.endpointsInf.HasSynced() && e.serviceInf.HasSynced() && e.podInf.HasSynced()
}

func (i *Ingress) hasSynced() bool {
	return i.informer.HasSynced()
}

func (p *Pod) hasSynced() bool {
	return p.informer.HasSynced()
}

func (s *Service) hasSynced() bool {
	return s.informer.HasSynced()
}
