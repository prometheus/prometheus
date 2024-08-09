// Copyright 2024 The Prometheus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aliyun

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
)

var _ discovery.DiscovererMetrics = (*ecsMetrics)(nil)

type ecsMetrics struct {
	refreshMetrics discovery.RefreshMetricsInstantiator

	queryCount         prometheus.Counter
	queryFailuresCount prometheus.Counter

	metricRegisterer discovery.MetricRegisterer
}

func newDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	m := &ecsMetrics{
		refreshMetrics: rmi,
		queryCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_sd_ecs_query_total",
				Help: "Number of aliyun ecs service discovery refresh.",
			}),
		queryFailuresCount: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "prometheus_sd_ecs_query_failures_total",
				Help: "Number of aliyun ecs service discovery refresh failures.",
			}),
	}

	m.metricRegisterer = discovery.NewMetricRegisterer(reg, []prometheus.Collector{
		m.queryCount,
		m.queryFailuresCount,
	})

	return m
}

// Register implements discovery.DiscovererMetrics.
func (m *ecsMetrics) Register() error {
	return m.metricRegisterer.RegisterMetrics()
}

// Unregister implements discovery.DiscovererMetrics.
func (m *ecsMetrics) Unregister() {
	m.metricRegisterer.UnregisterMetrics()
}
