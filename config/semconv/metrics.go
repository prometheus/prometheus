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
	prometheus.Gauge
}

// NewPrometheusConfigLastReloadSuccessTimestampSeconds returns a new PrometheusConfigLastReloadSuccessTimestampSeconds instrument.
func NewPrometheusConfigLastReloadSuccessTimestampSeconds() PrometheusConfigLastReloadSuccessTimestampSeconds {
	return PrometheusConfigLastReloadSuccessTimestampSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_config_last_reload_success_timestamp_seconds",
			Help: "Timestamp of the last successful configuration reload.",
		}),
	}
}

// PrometheusConfigLastReloadSuccessful records the whether the last configuration reload attempt was successful.
type PrometheusConfigLastReloadSuccessful struct {
	prometheus.Gauge
}

// NewPrometheusConfigLastReloadSuccessful returns a new PrometheusConfigLastReloadSuccessful instrument.
func NewPrometheusConfigLastReloadSuccessful() PrometheusConfigLastReloadSuccessful {
	return PrometheusConfigLastReloadSuccessful{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_config_last_reload_successful",
			Help: "Whether the last configuration reload attempt was successful.",
		}),
	}
}
