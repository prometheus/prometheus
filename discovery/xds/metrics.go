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

package xds

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
)

var _ discovery.DiscovererMetrics = (*xdsMetrics)(nil)

type xdsMetrics struct {
	fetchDuration        prometheus.Histogram
	fetchSkipUpdateCount prometheus.Counter
	fetchFailuresCount   prometheus.Counter

	metricRegisterer discovery.MetricRegisterer
}

func newDiscovererMetrics(reg prometheus.Registerer, _ discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	m := &xdsMetrics{
		fetchFailuresCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "sd_kuma_fetch_failures_total",
				Help:      "The number of Kuma MADS fetch call failures.",
			},
		),
		fetchSkipUpdateCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "sd_kuma_fetch_skipped_updates_total",
				Help:      "The number of Kuma MADS fetch calls that result in no updates to the targets.",
			},
		),
		fetchDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace:                       namespace,
				Name:                            "sd_kuma_fetch_duration_seconds",
				Help:                            "The duration of a Kuma MADS fetch call.",
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  100,
				NativeHistogramMinResetDuration: 1 * time.Hour,
			},
		),
	}

	m.metricRegisterer = discovery.NewMetricRegisterer(reg, []prometheus.Collector{
		m.fetchFailuresCount,
		m.fetchSkipUpdateCount,
		m.fetchDuration,
	})

	return m
}

// Register implements discovery.DiscovererMetrics.
func (m *xdsMetrics) Register() error {
	return m.metricRegisterer.RegisterMetrics()
}

// Unregister implements discovery.DiscovererMetrics.
func (m *xdsMetrics) Unregister() {
	m.metricRegisterer.UnregisterMetrics()
}
