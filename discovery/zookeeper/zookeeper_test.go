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

package zookeeper

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/discovery"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestNewDiscoveryError can fail if the DNS resolver mistakenly resolves the domain below.
// See https://github.com/prometheus/prometheus/issues/16191 for a precedent.
func TestNewDiscoveryError(t *testing.T) {
	t.Parallel()
	_, err := NewDiscovery(
		[]string{"unreachable.invalid"},
		time.Second, []string{"/"},
		nil,
		func([]byte, string) (model.LabelSet, error) { return nil, nil },
		newDiscovererMetrics(prometheus.NewRegistry(), nil),
	)
	require.Error(t, err)
}

// TestNewDiscoveryInvalidMetrics uses an address that zk.Connect accepts, so a
// connection opened before the metrics are checked leaks, and goleak reports it.
func TestNewDiscoveryInvalidMetrics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics discovery.DiscovererMetrics
	}{
		{name: "nil"},
		{name: "noop", metrics: &discovery.NoopDiscovererMetrics{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDiscovery(
				[]string{"127.0.0.1:1"},
				time.Second, []string{"/"},
				nil,
				func([]byte, string) (model.LabelSet, error) { return nil, nil },
				tc.metrics,
			)
			require.EqualError(t, err, "invalid discovery metrics type")
		})
	}
}

func TestDiscovererMetricsRegister(t *testing.T) {
	const names = "prometheus_treecache_zookeeper_failures_total,prometheus_treecache_watcher_goroutines"
	gathered := func(t *testing.T, reg prometheus.Gatherer) []string {
		t.Helper()
		mfs, err := reg.Gather()
		require.NoError(t, err)
		var got []string
		for _, mf := range mfs {
			if strings.Contains(names, mf.GetName()) {
				got = append(got, mf.GetName()+" "+mf.GetHelp())
			}
		}
		return got
	}

	t.Run("shared by ServerSet and Nerve", func(t *testing.T) {
		reg := prometheus.NewPedanticRegistry()
		serverset := (&ServersetSDConfig{}).NewDiscovererMetrics(reg, nil)
		nerve := (&NerveSDConfig{}).NewDiscovererMetrics(reg, nil)
		require.NoError(t, serverset.Register())
		require.NoError(t, nerve.Register())

		// Both SDs write to the same series.
		serverset.(*zookeeperMetrics).failureCounter.Inc()
		nerve.(*zookeeperMetrics).failureCounter.Inc()
		require.Equal(t, 2.0, testutil.ToFloat64(nerve.(*zookeeperMetrics).failureCounter))

		// Nerve reused the collectors of ServerSet, so it does not remove them.
		nerve.Unregister()
		require.Len(t, gathered(t, reg), 2)
		serverset.Unregister()
		require.Empty(t, gathered(t, reg))
	})

	t.Run("conflicting registration", func(t *testing.T) {
		reg := prometheus.NewPedanticRegistry()
		reg.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_treecache_watcher_goroutines",
			Help: "A conflicting metric.",
		}))
		m := newDiscovererMetrics(reg, nil)
		require.ErrorContains(t, m.Register(), "failed to register metric")
		// The failure counter that Register added before the conflict is gone.
		require.Equal(t, []string{"prometheus_treecache_watcher_goroutines A conflicting metric."}, gathered(t, reg))
	})
}
