// Code generated from semantic convention specification. DO NOT EDIT.

// Package metrics provides Prometheus instrumentation types for metrics
// defined in this semantic convention registry.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Attribute is an interface for metric label attributes.
type Attribute interface {
	ID() string
	Value() string
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
