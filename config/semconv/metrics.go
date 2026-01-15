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

// PrometheusConfigLastReloadSuccessTimestampSeconds records the timestamp of the last successful configuration reload.
type PrometheusConfigLastReloadSuccessTimestampSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusConfigLastReloadSuccessTimestampSeconds returns a new PrometheusConfigLastReloadSuccessTimestampSeconds instrument.
func NewPrometheusConfigLastReloadSuccessTimestampSeconds() PrometheusConfigLastReloadSuccessTimestampSeconds {
	labels := []string{}
	return PrometheusConfigLastReloadSuccessTimestampSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_config_last_reload_success_timestamp_seconds",
			Help: "Timestamp of the last successful configuration reload.",
		}, labels),
	}
}

type PrometheusConfigLastReloadSuccessTimestampSecondsAttr interface {
	Attribute
	implPrometheusConfigLastReloadSuccessTimestampSeconds()
}

func (m PrometheusConfigLastReloadSuccessTimestampSeconds) With(
	extra ...PrometheusConfigLastReloadSuccessTimestampSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusConfigLastReloadSuccessful records the whether the last configuration reload attempt was successful.
type PrometheusConfigLastReloadSuccessful struct {
	*prometheus.GaugeVec
}

// NewPrometheusConfigLastReloadSuccessful returns a new PrometheusConfigLastReloadSuccessful instrument.
func NewPrometheusConfigLastReloadSuccessful() PrometheusConfigLastReloadSuccessful {
	labels := []string{}
	return PrometheusConfigLastReloadSuccessful{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_config_last_reload_successful",
			Help: "Whether the last configuration reload attempt was successful.",
		}, labels),
	}
}

type PrometheusConfigLastReloadSuccessfulAttr interface {
	Attribute
	implPrometheusConfigLastReloadSuccessful()
}

func (m PrometheusConfigLastReloadSuccessful) With(
	extra ...PrometheusConfigLastReloadSuccessfulAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}
