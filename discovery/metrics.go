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

	semconv "github.com/prometheus/prometheus/discovery/semconv"
)

// Metrics to be used with a discovery manager.
type Metrics struct {
	FailedConfigs     semconv.PrometheusSDFailedConfigs
	DiscoveredTargets semconv.PrometheusSDDiscoveredTargets
	ReceivedUpdates   semconv.PrometheusSDReceivedUpdatesTotal
	DelayedUpdates    semconv.PrometheusSDUpdatesDelayedTotal
	SentUpdates       semconv.PrometheusSDUpdatesTotal
}

func NewManagerMetrics(registerer prometheus.Registerer, sdManagerName string) (*Metrics, error) {
	name := semconv.NameAttr(sdManagerName)
	m := &Metrics{
		FailedConfigs:     semconv.NewPrometheusSDFailedConfigs(name),
		DiscoveredTargets: semconv.NewPrometheusSDDiscoveredTargets(name),
		ReceivedUpdates:   semconv.NewPrometheusSDReceivedUpdatesTotal(name),
		DelayedUpdates:    semconv.NewPrometheusSDUpdatesDelayedTotal(name),
		SentUpdates:       semconv.NewPrometheusSDUpdatesTotal(name),
	}

	metrics := []prometheus.Collector{
		m.FailedConfigs.Gauge,
		m.DiscoveredTargets.GaugeVec,
		m.ReceivedUpdates.Counter,
		m.DelayedUpdates.Counter,
		m.SentUpdates.Counter,
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
	registerer.Unregister(m.FailedConfigs.Gauge)
	registerer.Unregister(m.DiscoveredTargets.GaugeVec)
	registerer.Unregister(m.ReceivedUpdates.Counter)
	registerer.Unregister(m.DelayedUpdates.Counter)
	registerer.Unregister(m.SentUpdates.Counter)
}
