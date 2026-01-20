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

// PrometheusAPINotificationActiveSubscribers records the current number of active notification subscribers.
type PrometheusAPINotificationActiveSubscribers struct {
	prometheus.Gauge
}

// NewPrometheusAPINotificationActiveSubscribers returns a new PrometheusAPINotificationActiveSubscribers instrument.
func NewPrometheusAPINotificationActiveSubscribers() PrometheusAPINotificationActiveSubscribers {
	return PrometheusAPINotificationActiveSubscribers{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_api_notification_active_subscribers",
			Help: "The current number of active notification subscribers.",
		}),
	}
}

// PrometheusAPINotificationUpdatesDroppedTotal records the total number of notification updates dropped.
type PrometheusAPINotificationUpdatesDroppedTotal struct {
	prometheus.Counter
}

// NewPrometheusAPINotificationUpdatesDroppedTotal returns a new PrometheusAPINotificationUpdatesDroppedTotal instrument.
func NewPrometheusAPINotificationUpdatesDroppedTotal() PrometheusAPINotificationUpdatesDroppedTotal {
	return PrometheusAPINotificationUpdatesDroppedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_api_notification_updates_dropped_total",
			Help: "Total number of notification updates dropped.",
		}),
	}
}

// PrometheusAPINotificationUpdatesSentTotal records the total number of notification updates sent.
type PrometheusAPINotificationUpdatesSentTotal struct {
	prometheus.Counter
}

// NewPrometheusAPINotificationUpdatesSentTotal returns a new PrometheusAPINotificationUpdatesSentTotal instrument.
func NewPrometheusAPINotificationUpdatesSentTotal() PrometheusAPINotificationUpdatesSentTotal {
	return PrometheusAPINotificationUpdatesSentTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_api_notification_updates_sent_total",
			Help: "Total number of notification updates sent.",
		}),
	}
}
