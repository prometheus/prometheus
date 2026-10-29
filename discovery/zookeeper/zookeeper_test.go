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
	)
	require.Error(t, err)
}

func TestDiscovererMetricsSharedLifecycle(t *testing.T) {
	for _, configs := range [][2]discovery.Config{
		{&ServersetSDConfig{}, &NerveSDConfig{}},
		{&NerveSDConfig{}, &ServersetSDConfig{}},
	} {
		t.Run(configs[0].Name()+" first", func(t *testing.T) {
			for stopped := range 2 {
				t.Run("unregister "+configs[stopped].Name(), func(t *testing.T) {
					reg := prometheus.NewPedanticRegistry()
					var metrics [2]discovery.DiscovererMetrics
					for i, cfg := range configs {
						metrics[i] = cfg.NewDiscovererMetrics(reg, nil)
						require.NoError(t, metrics[i].Register())
						t.Cleanup(func() {
							if metrics[i] != nil {
								metrics[i].Unregister()
							}
						})
					}

					// Gather both the legacy global collectors and the injected registry
					// without changing global state.
					gatherer := prometheus.Gatherers{reg, prometheus.DefaultGatherer}
					checkMetrics := func(t *testing.T) {
						t.Helper()
						for _, name := range []string{
							"prometheus_treecache_zookeeper_failures_total",
							"prometheus_treecache_watcher_goroutines",
						} {
							count, err := testutil.GatherAndCount(gatherer, name)
							require.NoError(t, err)
							require.Equal(t, 1, count, name)
						}
					}

					checkMetrics(t)
					metrics[stopped].Unregister()
					metrics[stopped] = nil
					checkMetrics(t)
				})
			}
		})
	}
}
