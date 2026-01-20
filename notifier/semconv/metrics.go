// Code generated from semantic convention specification. DO NOT EDIT.

// Package metrics provides Prometheus instrumentation types for metrics
// defined in this semantic convention registry.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Attribute is an interface for metric label attributes.
type Attribute interface {
	ID() string
	Value() string
}
type AlertmanagerAttr string

func (a AlertmanagerAttr) ID() string {
	return "alertmanager"
}

func (a AlertmanagerAttr) Value() string {
	return string(a)
}

// PrometheusNotificationsAlertmanagersDiscovered records the number of alertmanagers discovered and active.
type PrometheusNotificationsAlertmanagersDiscovered struct {
	prometheus.Gauge
}

// NewPrometheusNotificationsAlertmanagersDiscovered returns a new PrometheusNotificationsAlertmanagersDiscovered instrument.
func NewPrometheusNotificationsAlertmanagersDiscovered() PrometheusNotificationsAlertmanagersDiscovered {
	return PrometheusNotificationsAlertmanagersDiscovered{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_notifications_alertmanagers_discovered",
			Help: "The number of alertmanagers discovered and active.",
		}),
	}
}

// PrometheusNotificationsDroppedTotal records the total number of alerts dropped due to errors when sending to Alertmanager.
type PrometheusNotificationsDroppedTotal struct {
	prometheus.Counter
}

// NewPrometheusNotificationsDroppedTotal returns a new PrometheusNotificationsDroppedTotal instrument.
func NewPrometheusNotificationsDroppedTotal() PrometheusNotificationsDroppedTotal {
	return PrometheusNotificationsDroppedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_notifications_dropped_total",
			Help: "Total number of alerts dropped due to errors when sending to Alertmanager.",
		}),
	}
}

// PrometheusNotificationsErrorsTotal records the total number of sent alerts affected by errors.
type PrometheusNotificationsErrorsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusNotificationsErrorsTotal returns a new PrometheusNotificationsErrorsTotal instrument.
func NewPrometheusNotificationsErrorsTotal() PrometheusNotificationsErrorsTotal {
	labels := []string{
		"alertmanager",
	}
	return PrometheusNotificationsErrorsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_notifications_errors_total",
			Help: "Total number of sent alerts affected by errors.",
		}, labels),
	}
}

type PrometheusNotificationsErrorsTotalAttr interface {
	Attribute
	implPrometheusNotificationsErrorsTotal()
}

func (a AlertmanagerAttr) implPrometheusNotificationsErrorsTotal() {}

func (m PrometheusNotificationsErrorsTotal) With(
	extra ...PrometheusNotificationsErrorsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"alertmanager": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusNotificationsLatencyHistogramSeconds records the latency histogram for sending alert notifications.
type PrometheusNotificationsLatencyHistogramSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusNotificationsLatencyHistogramSeconds returns a new PrometheusNotificationsLatencyHistogramSeconds instrument.
func NewPrometheusNotificationsLatencyHistogramSeconds() PrometheusNotificationsLatencyHistogramSeconds {
	labels := []string{
		"alertmanager",
	}
	return PrometheusNotificationsLatencyHistogramSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:                            "prometheus_notifications_latency_histogram_seconds",
			Help:                            "Latency histogram for sending alert notifications.",
			Buckets:                         []float64{0.01, 0.1, 1, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}, labels),
	}
}

type PrometheusNotificationsLatencyHistogramSecondsAttr interface {
	Attribute
	implPrometheusNotificationsLatencyHistogramSeconds()
}

func (a AlertmanagerAttr) implPrometheusNotificationsLatencyHistogramSeconds() {}

func (m PrometheusNotificationsLatencyHistogramSeconds) With(
	extra ...PrometheusNotificationsLatencyHistogramSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"alertmanager": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusNotificationsLatencySeconds records the latency quantiles for sending alert notifications.
type PrometheusNotificationsLatencySeconds struct {
	*prometheus.SummaryVec
}

// NewPrometheusNotificationsLatencySeconds returns a new PrometheusNotificationsLatencySeconds instrument.
func NewPrometheusNotificationsLatencySeconds() PrometheusNotificationsLatencySeconds {
	labels := []string{
		"alertmanager",
	}
	return PrometheusNotificationsLatencySeconds{
		SummaryVec: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "prometheus_notifications_latency_seconds",
			Help: "Latency quantiles for sending alert notifications.",
			Objectives: map[float64]float64{
				0.5: 0.05,
				0.9: 0.01,
				0.99: 0.001,
			},
		}, labels),
	}
}

type PrometheusNotificationsLatencySecondsAttr interface {
	Attribute
	implPrometheusNotificationsLatencySeconds()
}

func (a AlertmanagerAttr) implPrometheusNotificationsLatencySeconds() {}

func (m PrometheusNotificationsLatencySeconds) With(
	extra ...PrometheusNotificationsLatencySecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"alertmanager": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.SummaryVec.With(labels)
}

// PrometheusNotificationsQueueCapacity records the capacity of the alert notifications queue.
type PrometheusNotificationsQueueCapacity struct {
	prometheus.Gauge
}

// NewPrometheusNotificationsQueueCapacity returns a new PrometheusNotificationsQueueCapacity instrument.
func NewPrometheusNotificationsQueueCapacity() PrometheusNotificationsQueueCapacity {
	return PrometheusNotificationsQueueCapacity{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_notifications_queue_capacity",
			Help: "The capacity of the alert notifications queue.",
		}),
	}
}

// PrometheusNotificationsQueueLength records the number of alert notifications in the queue.
type PrometheusNotificationsQueueLength struct {
	prometheus.Gauge
}

// NewPrometheusNotificationsQueueLength returns a new PrometheusNotificationsQueueLength instrument.
func NewPrometheusNotificationsQueueLength() PrometheusNotificationsQueueLength {
	return PrometheusNotificationsQueueLength{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_notifications_queue_length",
			Help: "The number of alert notifications in the queue.",
		}),
	}
}

// PrometheusNotificationsSentTotal records the total number of alerts sent.
type PrometheusNotificationsSentTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusNotificationsSentTotal returns a new PrometheusNotificationsSentTotal instrument.
func NewPrometheusNotificationsSentTotal() PrometheusNotificationsSentTotal {
	labels := []string{
		"alertmanager",
	}
	return PrometheusNotificationsSentTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_notifications_sent_total",
			Help: "Total number of alerts sent.",
		}, labels),
	}
}

type PrometheusNotificationsSentTotalAttr interface {
	Attribute
	implPrometheusNotificationsSentTotal()
}

func (a AlertmanagerAttr) implPrometheusNotificationsSentTotal() {}

func (m PrometheusNotificationsSentTotal) With(
	extra ...PrometheusNotificationsSentTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"alertmanager": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
