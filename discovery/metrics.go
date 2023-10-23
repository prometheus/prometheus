// Copyright 2016 The Prometheus Authors
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

package discovery

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	clientGoRequestMetrics  = &clientGoRequestMetricAdapter{}
	clientGoWorkloadMetrics = &clientGoWorkqueueMetricsProvider{}
)

func init() {
	clientGoRequestMetrics.RegisterWithK8sGoClient()
	clientGoWorkloadMetrics.RegisterWithK8sGoClient()
}

// Metrics to be used with a discovery manager.
type Metrics struct {
	FailedConfigs     *prometheus.GaugeVec
	DiscoveredTargets *prometheus.GaugeVec
	ReceivedUpdates   *prometheus.CounterVec
	DelayedUpdates    *prometheus.CounterVec
	SentUpdates       *prometheus.CounterVec
}

func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{}

	m.FailedConfigs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_sd_failed_configs",
			Help: "Current number of service discovery configurations that failed to load.",
		},
		[]string{"name"},
	)

	m.DiscoveredTargets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "prometheus_sd_discovered_targets",
			Help: "Current number of discovered targets.",
		},
		[]string{"name", "config"},
	)

	m.ReceivedUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_received_updates_total",
			Help: "Total number of update events received from the SD providers.",
		},
		[]string{"name"},
	)

	m.DelayedUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_updates_delayed_total",
			Help: "Total number of update events that couldn't be sent immediately.",
		},
		[]string{"name"},
	)

	m.SentUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_sd_updates_total",
			Help: "Total number of update events sent to the SD consumers.",
		},
		[]string{"name"},
	)

	metrics := append(
		[]prometheus.Collector{
			m.FailedConfigs,
			m.DiscoveredTargets,
			m.ReceivedUpdates,
			m.DelayedUpdates,
			m.SentUpdates,
		},
		clientGoMetrics()...,
	)

	for _, collector := range metrics {
		err := registerer.Register(collector)
		if err != nil {
			return nil, fmt.Errorf("failed to register discovery manager metrics: %w", err)
		}
	}

	return m, nil
}
