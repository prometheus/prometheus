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

package notifier

import (
	"github.com/prometheus/client_golang/prometheus"

	metrics "github.com/prometheus/prometheus/notifier/semconv"
)

type alertMetrics struct {
	latencySummary          metrics.PrometheusNotificationsLatencySeconds
	latencyHistogram        metrics.PrometheusNotificationsLatencyHistogramSeconds
	errors                  metrics.PrometheusNotificationsErrorsTotal
	sent                    metrics.PrometheusNotificationsSentTotal
	dropped                 metrics.PrometheusNotificationsDroppedTotal
	queueLength             prometheus.GaugeFunc
	queueCapacity           metrics.PrometheusNotificationsQueueCapacity
	alertmanagersDiscovered prometheus.GaugeFunc
}

func newAlertMetrics(
	r prometheus.Registerer,
	queueCap int,
	queueLen, alertmanagersDiscovered func() float64,
) *alertMetrics {
	m := &alertMetrics{
		latencySummary:   metrics.NewPrometheusNotificationsLatencySeconds(),
		latencyHistogram: metrics.NewPrometheusNotificationsLatencyHistogramSeconds(),
		errors:           metrics.NewPrometheusNotificationsErrorsTotal(),
		sent:             metrics.NewPrometheusNotificationsSentTotal(),
		dropped:          metrics.NewPrometheusNotificationsDroppedTotal(),
		// GaugeFunc metrics require callbacks, so they remain manual
		queueLength: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_notifications_queue_length",
			Help: "The number of alert notifications in the queue.",
		}, queueLen),
		queueCapacity: metrics.NewPrometheusNotificationsQueueCapacity(),
		alertmanagersDiscovered: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_notifications_alertmanagers_discovered",
			Help: "The number of alertmanagers discovered and active.",
		}, alertmanagersDiscovered),
	}

	m.queueCapacity.Set(float64(queueCap))

	if r != nil {
		r.MustRegister(
			m.latencySummary.SummaryVec,
			m.latencyHistogram.HistogramVec,
			m.errors.CounterVec,
			m.sent.CounterVec,
			m.dropped.Counter,
			m.queueLength,
			m.queueCapacity.Gauge,
			m.alertmanagersDiscovered,
		)
	}

	return m
}
