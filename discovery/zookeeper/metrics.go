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
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
)

type zookeeperMetrics struct {
	reg prometheus.Registerer

	// The total number of ZooKeeper failures.
	failureCounter prometheus.Counter
	// The current number of Zookeeper watcher goroutines.
	numWatchers prometheus.Gauge

	// registered holds the collectors that Register added to reg, so that
	// Unregister leaves the ones it reused from the other SD in place.
	registered []prometheus.Collector
}

func newDiscovererMetrics(reg prometheus.Registerer, _ discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return &zookeeperMetrics{
		reg: reg,
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
}

// Register implements discovery.DiscovererMetrics.
//
// For historical reasons, ServerSet and Nerve SD share the same metrics. When
// both register with the same registry, the second one reuses the collectors of
// the first. If a registration fails, the collectors that Register added are
// removed again, as discovery.NewMetricRegisterer does.
func (m *zookeeperMetrics) Register() error {
	counter, err := registerOrReuse(m, m.failureCounter)
	if err != nil {
		return err
	}
	gauge, err := registerOrReuse(m, m.numWatchers)
	if err != nil {
		m.Unregister()
		return err
	}
	m.failureCounter, m.numWatchers = counter, gauge
	return nil
}

// Unregister implements discovery.DiscovererMetrics.
// It removes only the collectors that Register added.
func (m *zookeeperMetrics) Unregister() {
	for _, c := range m.registered {
		m.reg.Unregister(c)
	}
	m.registered = nil
}

// registerOrReuse registers c with m.reg and returns it. If an equal collector
// is already registered, it returns that one instead.
func registerOrReuse[T prometheus.Collector](m *zookeeperMetrics, c T) (T, error) {
	err := m.reg.Register(c)
	if err == nil {
		m.registered = append(m.registered, c)
		return c, nil
	}
	if are, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
		if existing, ok := are.ExistingCollector.(T); ok {
			return existing, nil
		}
	}
	return c, fmt.Errorf("failed to register metric: %w", err)
}
