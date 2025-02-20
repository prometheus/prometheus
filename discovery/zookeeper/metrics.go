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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/discovery"
)

type zookeeperMetrics struct {
	// The total number of ZooKeeper failures.
	failureCounter prometheus.Counter
	// The current number of Zookeeper watcher goroutines.
	numWatchers prometheus.Gauge

	metricRegisterer discovery.MetricRegisterer
}

// Create and register metrics.
func newDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
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

	m.metricRegisterer = discovery.NewMetricRegisterer(reg, []prometheus.Collector{
		m.failureCounter,
		m.numWatchers,
	})

	return m
}

// Register implements discovery.DiscovererMetrics.
func (m *zookeeperMetrics) Register() error {
	return m.metricRegisterer.RegisterMetrics()
}

// Unregister implements discovery.DiscovererMetrics.
func (m *zookeeperMetrics) Unregister() {
	m.metricRegisterer.UnregisterMetrics()
}

// Expose individual metrics (to avoid struct circular import issues).
func (m *zookeeperMetrics) FailuresCounter() prometheus.Counter {
	return m.failureCounter
}

func (m *zookeeperMetrics) WatcherGauge() prometheus.Gauge {
	return m.numWatchers
}
