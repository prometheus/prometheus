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
	*prometheus.GaugeVec
}

// NewPrometheusNotificationsAlertmanagersDiscovered returns a new PrometheusNotificationsAlertmanagersDiscovered instrument.
func NewPrometheusNotificationsAlertmanagersDiscovered() PrometheusNotificationsAlertmanagersDiscovered {
	labels := []string{}
	return PrometheusNotificationsAlertmanagersDiscovered{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_notifications_alertmanagers_discovered",
			Help: "The number of alertmanagers discovered and active.",
		}, labels),
	}
}

type PrometheusNotificationsAlertmanagersDiscoveredAttr interface {
	Attribute
	implPrometheusNotificationsAlertmanagersDiscovered()
}

func (m PrometheusNotificationsAlertmanagersDiscovered) With(
	extra ...PrometheusNotificationsAlertmanagersDiscoveredAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusNotificationsDroppedTotal records the total number of alerts dropped due to errors when sending to Alertmanager.
type PrometheusNotificationsDroppedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusNotificationsDroppedTotal returns a new PrometheusNotificationsDroppedTotal instrument.
func NewPrometheusNotificationsDroppedTotal() PrometheusNotificationsDroppedTotal {
	labels := []string{}
	return PrometheusNotificationsDroppedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_notifications_dropped_total",
			Help: "Total number of alerts dropped due to errors when sending to Alertmanager.",
		}, labels),
	}
}

type PrometheusNotificationsDroppedTotalAttr interface {
	Attribute
	implPrometheusNotificationsDroppedTotal()
}

func (m PrometheusNotificationsDroppedTotal) With(
	extra ...PrometheusNotificationsDroppedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusNotificationsQueueCapacity records the capacity of the alert notifications queue.
type PrometheusNotificationsQueueCapacity struct {
	*prometheus.GaugeVec
}

// NewPrometheusNotificationsQueueCapacity returns a new PrometheusNotificationsQueueCapacity instrument.
func NewPrometheusNotificationsQueueCapacity() PrometheusNotificationsQueueCapacity {
	labels := []string{}
	return PrometheusNotificationsQueueCapacity{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_notifications_queue_capacity",
			Help: "The capacity of the alert notifications queue.",
		}, labels),
	}
}

type PrometheusNotificationsQueueCapacityAttr interface {
	Attribute
	implPrometheusNotificationsQueueCapacity()
}

func (m PrometheusNotificationsQueueCapacity) With(
	extra ...PrometheusNotificationsQueueCapacityAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusNotificationsQueueLength records the number of alert notifications in the queue.
type PrometheusNotificationsQueueLength struct {
	*prometheus.GaugeVec
}

// NewPrometheusNotificationsQueueLength returns a new PrometheusNotificationsQueueLength instrument.
func NewPrometheusNotificationsQueueLength() PrometheusNotificationsQueueLength {
	labels := []string{}
	return PrometheusNotificationsQueueLength{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_notifications_queue_length",
			Help: "The number of alert notifications in the queue.",
		}, labels),
	}
}

type PrometheusNotificationsQueueLengthAttr interface {
	Attribute
	implPrometheusNotificationsQueueLength()
}

func (m PrometheusNotificationsQueueLength) With(
	extra ...PrometheusNotificationsQueueLengthAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}
