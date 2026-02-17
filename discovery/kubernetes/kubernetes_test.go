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
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	// Disable the WatchListClient feature gate that is enabled by default in
	// client-go v0.35.0+. The WatchList flow requires the server to support
	// SendInitialEvents and to send a bookmark event with the
	// "k8s.io/initial-events-end" annotation. The fake clientset used in tests
	// does not support this protocol, causing informers to hang indefinitely
	// waiting for the bookmark. Disabling this feature restores the traditional
	// List+Watch flow which is compatible with the fake clientset.
	os.Setenv("KUBE_FEATURE_WatchListClient", "false")
	testutil.TolerantVerifyLeak(m)
}

// makeDiscovery creates a kubernetes.Discovery instance for testing.
func makeDiscovery(role Role, nsDiscovery NamespaceDiscovery, objects ...runtime.Object) (*Discovery, kubernetes.Interface) {
	return makeDiscoveryWithVersion(role, nsDiscovery, "v1.25.0", objects...)
}

// makeDiscoveryWithVersion creates a kubernetes.Discovery instance with the specified kubernetes version for testing.
func makeDiscoveryWithVersion(role Role, nsDiscovery NamespaceDiscovery, k8sVer string, objects ...runtime.Object) (*Discovery, kubernetes.Interface) {
	clientset := fake.NewClientset(objects...)
	fakeDiscovery, _ := clientset.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscovery.FakedServerVersion = &version.Info{GitVersion: k8sVer}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := newDiscovererMetrics(reg, refreshMetrics)
	err := metrics.Register()
	if err != nil {
		panic(err)
	}
	// TODO(ptodev): Unregister the metrics at the end of the test.

	kubeMetrics, ok := metrics.(*kubernetesMetrics)
	if !ok {
		panic("invalid discovery metrics type")
	}

	d := &Discovery{
		client:             clientset,
		logger:             promslog.NewNopLogger(),
		role:               role,
		namespaceDiscovery: &nsDiscovery,
		ownNamespace:       "own-ns",
		metrics:            kubeMetrics,
	}

	return d, clientset
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
	go readResultWithTimeout(t, ctx, ch, d.expectedMaxItems, time.Second, resChan)

	dd, ok := d.discovery.(hasSynced)
	require.True(t, ok, "discoverer does not implement hasSynced interface")
	require.True(t, cache.WaitForCacheSync(ctx.Done(), dd.hasSynced), "discoverer failed to sync: %v", dd)

	if d.afterStart != nil {
		d.afterStart()
	}

	if d.expectedRes != nil {
		res := <-resChan
		requireTargetGroups(t, d.expectedRes, res)
	} else {
		// Stop readResultWithTimeout and wait for it.
		cancel()
		<-resChan
	}
}

// readResultWithTimeout reads all targetgroups from channel with timeout.
// It merges targetgroups by source and sends the result to result channel.
func readResultWithTimeout(t *testing.T, ctx context.Context, ch <-chan []*targetgroup.Group, maxGroups int, stopAfter time.Duration, resChan chan<- map[string]*targetgroup.Group) {
	res := make(map[string]*targetgroup.Group)
	timeout := time.After(stopAfter)
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
			if len(res) == maxGroups {
				// Reached max target groups we may get, break fast.
				break Loop
			}
		case <-timeout:
			// Because we use queue, an object that is created then
			// deleted or updated may be processed only once.
			// So possibly we may skip events, timed out here.
			t.Logf("timed out, got %d (max: %d) items, some events are skipped", len(res), maxGroups)
			break Loop
		case <-ctx.Done():
			t.Logf("stopped, got %d (max: %d) items", len(res), maxGroups)
			break Loop
		}
	}

	resChan <- res
}

func requireTargetGroups(t *testing.T, expected, res map[string]*targetgroup.Group) {
	t.Helper()
	b1, err := marshalTargetGroups(expected)
	if err != nil {
		panic(err)
	}
	b2, err := marshalTargetGroups(res)
	if err != nil {
		panic(err)
	}

	require.JSONEq(t, string(b1), string(b2))
}

// marshalTargetGroups serializes a set of target groups to JSON, ignoring the
// custom MarshalJSON function defined on the targetgroup.Group struct.
// marshalTargetGroups can be used for making exact comparisons between target groups
// as it will serialize all target labels.
func marshalTargetGroups(tgs map[string]*targetgroup.Group) ([]byte, error) {
	type targetGroupAlias targetgroup.Group

	aliases := make(map[string]*targetGroupAlias, len(tgs))
	for k, v := range tgs {
		tg := targetGroupAlias(*v)
		aliases[k] = &tg
	}

	return json.Marshal(aliases)
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
	t.Parallel()
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

func TestFailuresCountMetric(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role             Role
		minFailedWatches int
	}{
		{RoleNode, 1},
		{RolePod, 1},
		{RoleService, 1},
		{RoleEndpoint, 3},
		{RoleEndpointSlice, 3},
		{RoleIngress, 1},
	}

	for _, tc := range tests {
		t.Run(string(tc.role), func(t *testing.T) {
			t.Parallel()

			n, c := makeDiscovery(tc.role, NamespaceDiscovery{})
			// The counter is initialized and no failures at the beginning.
			require.Equal(t, float64(0), prom_testutil.ToFloat64(n.metrics.failuresCount))

			// Simulate an error on watch requests.
			c.Discovery().(*fakediscovery.FakeDiscovery).PrependWatchReactor("*", func(kubetesting.Action) (bool, watch.Interface, error) {
				return true, nil, apierrors.NewUnauthorized("unauthorized")
			})

			// Start the discovery.
			k8sDiscoveryTest{discovery: n}.Run(t)

			// At least the errors of the initial watches should be caught (watches are retried on errors).
			require.GreaterOrEqual(t, prom_testutil.ToFloat64(n.metrics.failuresCount), float64(tc.minFailedWatches))
		})
	}
}

func TestNodeName(t *testing.T) {
	t.Parallel()
	node := &apiv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}
	name, err := nodeName(node)
	require.NoError(t, err)
	require.Equal(t, "foo", name)

	name, err = nodeName(cache.DeletedFinalStateUnknown{Key: "bar"})
	require.NoError(t, err)
	require.Equal(t, "bar", name)
}
