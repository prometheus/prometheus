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

package consul

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
)

var _ discovery.DiscovererMetrics = (*consulMetrics)(nil)

type consulMetrics struct {
	rpcFailuresCount     prometheus.Counter
	rpcDuration          *prometheus.SummaryVec
	rpcDurationHistogram *prometheus.HistogramVec

	servicesRPCDuration prometheus.Observer
	serviceRPCDuration  prometheus.Observer

	metricRegisterer discovery.MetricRegisterer
}

func newDiscovererMetrics(reg prometheus.Registerer, _ discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	m := &consulMetrics{
		rpcFailuresCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "sd_consul_rpc_failures_total",
				Help:      "The number of Consul RPC call failures.",
			},
		),
		rpcDuration: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  namespace,
				Name:       "sd_consul_rpc_duration_seconds",
				Help:       "The duration of a Consul RPC call in seconds.",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			[]string{"endpoint", "call"},
		),
		rpcDurationHistogram: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:                       namespace,
				Name:                            "sd_consul_rpc_duration_histogram_seconds",
				Help:                            "The duration of a Consul RPC call in seconds.",
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  100,
				NativeHistogramMinResetDuration: 1 * time.Hour,
			},
			[]string{"endpoint", "call"},
		),
	}

	m.metricRegisterer = discovery.NewMetricRegisterer(reg, []prometheus.Collector{
		m.rpcFailuresCount,
		m.rpcDuration,
		m.rpcDurationHistogram,
	})

	// Initialize metric vectors.
	servicesSummary := m.rpcDuration.WithLabelValues("catalog", "services")
	servicesHistogram := m.rpcDurationHistogram.WithLabelValues("catalog", "services")
	m.servicesRPCDuration = prometheus.ObserverFunc(func(value float64) {
		servicesSummary.Observe(value)
		servicesHistogram.Observe(value)
	})
	serviceSummary := m.rpcDuration.WithLabelValues("catalog", "service")
	serviceHistogram := m.rpcDurationHistogram.WithLabelValues("catalog", "service")
	m.serviceRPCDuration = prometheus.ObserverFunc(func(value float64) {
		serviceSummary.Observe(value)
		serviceHistogram.Observe(value)
	})

	return m
}

// Register implements discovery.DiscovererMetrics.
func (m *consulMetrics) Register() error {
	return m.metricRegisterer.RegisterMetrics()
}

// Unregister implements discovery.DiscovererMetrics.
func (m *consulMetrics) Unregister() {
	m.metricRegisterer.UnregisterMetrics()
}
