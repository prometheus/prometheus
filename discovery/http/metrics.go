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

package http

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
)

var _ discovery.DiscovererMetrics = (*httpMetrics)(nil)

type httpMetrics struct {
	refreshMetrics discovery.RefreshMetricsInstantiator

	failuresCount prometheus.Counter

	metricRegisterer discovery.MetricRegisterer
}

func newDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	m := &httpMetrics{
		refreshMetrics: rmi,
		failuresCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_sd_http_failures_total",
				Help: "Number of HTTP service discovery refresh failures.",
			}),
	}

	m.metricRegisterer = discovery.NewMetricRegisterer(reg, []prometheus.Collector{
		m.failuresCount,
	})

	return m
}

// Register implements discovery.DiscovererMetrics.
func (m *httpMetrics) Register() error {
	return m.metricRegisterer.RegisterMetrics()
}

// Unregister implements discovery.DiscovererMetrics.
func (m *httpMetrics) Unregister() {
	m.metricRegisterer.UnregisterMetrics()
}
