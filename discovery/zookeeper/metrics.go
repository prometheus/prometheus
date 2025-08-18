// Copyright 2025 The Prometheus Authors
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
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
)

type zookeeperMetrics struct {
	// The total number of ZooKeeper failures.
	failureCounter prometheus.Counter
	// The current number of Zookeeper watcher goroutines.
	numWatchers prometheus.Gauge
}

// Create and register metrics.
func newDiscovererMetrics(reg prometheus.Registerer, _ discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	m := &zookeeperMetrics{
		failureCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "treecache",
			Name:      "zookeeper_failures_total",
			Help:      "The total number of ZooKeeper failures.",
		}),
		numWatchers: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "prometheus",
			Subsystem: "treecache",
			Name:      "watcher_goroutines",
			Help:      "The current number of watcher goroutines.",
		}),
	}
	// NOTE: These metrics are shared by both ServerSet and Nerve SD.
	// Therefore, we do NOT unregister them during config reloads to avoid unregistering while the other is still using them.
	// Long-term, this should be split to enable clean registration/unregistration per SD type.
	// For historical reasons, both ServerSet and Nerve SD share the same zookeeper metrics.
	// To not cause double registration problems, if both SD mechanisms are instantiated with
	// the same registry, we are handling the AlreadyRegisteredError accordingly below.
	// TODO: Consider separate zookeeper metrics for both SD mechanisms in the future.
	if err := reg.Register(m.failureCounter); err != nil {
		var are prometheus.AlreadyRegisteredError
		if !errors.As(err, &are) {
			panic(err)
		}
		m.failureCounter = are.ExistingCollector.(prometheus.Counter)
	}

	if err := reg.Register(m.numWatchers); err != nil {
		var are prometheus.AlreadyRegisteredError
		if !errors.As(err, &are) {
			panic(err)
		}
		m.numWatchers = are.ExistingCollector.(prometheus.Gauge)
	}

	return &zookeeperMetrics{
		failureCounter: m.failureCounter,
		numWatchers:    m.numWatchers,
	}
}

// Register implements discovery.DiscovererMetrics.
func (m *zookeeperMetrics) Register() error {
	// return m.metricRegisterer.RegisterMetrics()
	return nil
}

// Unregister implements discovery.DiscovererMetrics.
func (m *zookeeperMetrics) Unregister() {
	// m.metricRegisterer.UnregisterMetrics()
}
