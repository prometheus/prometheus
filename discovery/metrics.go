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

package discovery

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics to be used with a discovery manager.
type Metrics struct {
	FailedConfigs     prometheus.Gauge
	DiscoveredTargets *prometheus.GaugeVec
	ReceivedUpdates   prometheus.Counter
	DelayedUpdates    prometheus.Counter
	SentUpdates       prometheus.Counter
	LastUpdated       *prometheus.GaugeVec

	// Metrics for the resolve_addresses address expansion feature.
	ResolveLookups            *prometheus.CounterVec
	ResolveLookupFailures     *prometheus.CounterVec
	ResolveResolvedTargets    *prometheus.GaugeVec
	ResolveAddressTruncations *prometheus.CounterVec
}

func NewManagerMetrics(registerer prometheus.Registerer, sdManagerName string) (*Metrics, error) {
	m := &Metrics{}

	m.FailedConfigs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "prometheus_sd_failed_configs",
			Help:        "Current number of service discovery configurations that failed to load.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
	)

	m.DiscoveredTargets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "prometheus_sd_discovered_targets",
			Help:        "Current number of discovered targets.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
		[]string{"config"},
	)

	m.ReceivedUpdates = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "prometheus_sd_received_updates_total",
			Help:        "Total number of update events received from the SD providers.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
	)

	m.DelayedUpdates = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "prometheus_sd_updates_delayed_total",
			Help:        "Total number of update events that couldn't be sent immediately.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
	)

	m.SentUpdates = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "prometheus_sd_updates_total",
			Help:        "Total number of update events sent to the SD consumers.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
	)

	m.LastUpdated = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "prometheus_sd_last_update_timestamp_seconds",
			Help:        "Timestamp of the last update sent to the SD consumers.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
		[]string{"config"},
	)

	m.ResolveLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "prometheus_sd_resolve_address_lookups_total",
			Help:        "Total number of address resolution lookups performed by the resolve_addresses feature.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
		[]string{"config"},
	)

	m.ResolveLookupFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "prometheus_sd_resolve_address_lookup_failures_total",
			Help:        "Total number of failed address resolution lookups performed by the resolve_addresses feature.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
		[]string{"config"},
	)

	m.ResolveResolvedTargets = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "prometheus_sd_resolve_address_resolved_targets",
			Help:        "Current number of targets emitted by the resolve_addresses feature after address expansion.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
		[]string{"config"},
	)

	m.ResolveAddressTruncations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "prometheus_sd_resolve_address_truncations_total",
			Help:        "Total number of times the resolve_addresses feature truncated a host's resolved addresses to max_resolved_addresses.",
			ConstLabels: prometheus.Labels{"name": sdManagerName},
		},
		[]string{"config"},
	)

	metrics := []prometheus.Collector{
		m.FailedConfigs,
		m.DiscoveredTargets,
		m.ReceivedUpdates,
		m.DelayedUpdates,
		m.SentUpdates,
		m.LastUpdated,
		m.ResolveLookups,
		m.ResolveLookupFailures,
		m.ResolveResolvedTargets,
		m.ResolveAddressTruncations,
	}

	for _, collector := range metrics {
		err := registerer.Register(collector)
		if err != nil {
			return nil, fmt.Errorf("failed to register discovery manager metrics: %w", err)
		}
	}

	return m, nil
}

// Unregister unregisters all metrics.
func (m *Metrics) Unregister(registerer prometheus.Registerer) {
	registerer.Unregister(m.FailedConfigs)
	registerer.Unregister(m.DiscoveredTargets)
	registerer.Unregister(m.ReceivedUpdates)
	registerer.Unregister(m.DelayedUpdates)
	registerer.Unregister(m.SentUpdates)
	registerer.Unregister(m.LastUpdated)
	registerer.Unregister(m.ResolveLookups)
	registerer.Unregister(m.ResolveLookupFailures)
	registerer.Unregister(m.ResolveResolvedTargets)
	registerer.Unregister(m.ResolveAddressTruncations)
}
