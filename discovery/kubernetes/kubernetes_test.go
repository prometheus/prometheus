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

	"github.com/go-kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeak(m)
}

// makeDiscovery creates a kubernetes.Discovery instance for testing.
func makeDiscovery(role Role, nsDiscovery NamespaceDiscovery, objects ...runtime.Object) (*Discovery, kubernetes.Interface) {
	return makeDiscoveryWithVersion(role, nsDiscovery, "v1.22.0", objects...)
}

// makeDiscoveryWithVersion creates a kubernetes.Discovery instance with the specified kubernetes version for testing.
func makeDiscoveryWithVersion(role Role, nsDiscovery NamespaceDiscovery, k8sVer string, objects ...runtime.Object) (*Discovery, kubernetes.Interface) {
	clientset := fake.NewSimpleClientset(objects...)
	fakeDiscovery, _ := clientset.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscovery.FakedServerVersion = &version.Info{GitVersion: k8sVer}

	return &Discovery{
		client:             clientset,
		logger:             log.NewNopLogger(),
		role:               role,
		namespaceDiscovery: &nsDiscovery,
		ownNamespace:       "own-ns",
	}, clientset
}

// makeDiscoveryWithMetadata creates a kubernetes.Discovery instance with the specified metadata config.
func makeDiscoveryWithMetadata(role Role, nsDiscovery NamespaceDiscovery, attachMetadata AttachMetadataConfig, objects ...runtime.Object) (*Discovery, kubernetes.Interface) {
	d, k8s := makeDiscovery(role, nsDiscovery, objects...)
	d.attachMetadata = attachMetadata
	return d, k8s
}

type k8sDiscoveryTest struct {
	// discovery is instance of discovery.Discoverer
	discovery discovery.Discoverer
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
	lastDiscoverersCount := 0
	dis := d.discovery.(*Discovery)
	for {
		dis.RLock()
		l := len(dis.discoverers)
		dis.RUnlock()
		if l > 0 && l == lastDiscoverersCount {
			break
		}
		time.Sleep(100 * time.Millisecond)

		lastDiscoverersCount = l
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

// readResultWithTimeout reads all targetgroups from channel with timeout.
// It merges targetgroups by source and sends the result to result channel.
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

	require.Equal(t, string(b1), string(b2))
}

type hasSynced interface {
	// hasSynced returns true if all informers synced.
	// This is only used in testing to determine when discoverer synced to
	// kubernetes apiserver.
	hasSynced() bool
}

var (
	_ hasSynced = &Discovery{}
	_ hasSynced = &Node{}
	_ hasSynced = &Endpoints{}
	_ hasSynced = &EndpointSlice{}
	_ hasSynced = &Ingress{}
	_ hasSynced = &Pod{}
	_ hasSynced = &Service{}
)

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

func (e *EndpointSlice) hasSynced() bool {
	return e.endpointSliceInf.HasSynced() && e.serviceInf.HasSynced() && e.podInf.HasSynced()
}

func (i *Ingress) hasSynced() bool {
	return i.informer.HasSynced()
}

func (p *Pod) hasSynced() bool {
	return p.podInf.HasSynced()
}

func (s *Service) hasSynced() bool {
	return s.informer.HasSynced()
}

func TestRetryOnError(t *testing.T) {
	for _, successAt := range []int{1, 2, 3} {
		var called int
		f := func() error {
			called++
			if called >= successAt {
				return nil
			}
			return errors.New("dummy")
		}
		retryOnError(context.TODO(), 0, f)
		require.Equal(t, successAt, called)
	}
}

func TestCheckNetworkingV1Supported(t *testing.T) {
	tests := []struct {
		version       string
		wantSupported bool
		wantErr       bool
	}{
		{version: "v1.18.0", wantSupported: false, wantErr: false},
		{version: "v1.18.1", wantSupported: false, wantErr: false},
		// networking v1 is supported since Kubernetes v1.19
		{version: "v1.19.0", wantSupported: true, wantErr: false},
		{version: "v1.20.0-beta.2", wantSupported: true, wantErr: false},
		// error patterns
		{version: "", wantSupported: false, wantErr: true},
		{version: "<>", wantSupported: false, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.version, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			fakeDiscovery, _ := clientset.Discovery().(*fakediscovery.FakeDiscovery)
			fakeDiscovery.FakedServerVersion = &version.Info{GitVersion: tc.version}
			supported, err := checkNetworkingV1Supported(clientset)

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantSupported, supported)
		})
	}
}
