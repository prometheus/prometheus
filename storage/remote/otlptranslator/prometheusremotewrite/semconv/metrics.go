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

// PrometheusAPIOtlpAppendedSamplesWithoutMetadataTotal records the total number of samples ingested from OTLP without corresponding metadata.
type PrometheusAPIOtlpAppendedSamplesWithoutMetadataTotal struct {
	prometheus.Counter
}

// NewPrometheusAPIOtlpAppendedSamplesWithoutMetadataTotal returns a new PrometheusAPIOtlpAppendedSamplesWithoutMetadataTotal instrument.
func NewPrometheusAPIOtlpAppendedSamplesWithoutMetadataTotal() PrometheusAPIOtlpAppendedSamplesWithoutMetadataTotal {
	return PrometheusAPIOtlpAppendedSamplesWithoutMetadataTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_api_otlp_appended_samples_without_metadata_total",
			Help: "The total number of samples ingested from OTLP without corresponding metadata.",
		}),
	}
}

// PrometheusAPIOtlpOutOfOrderExemplarsTotal records the total number of received OTLP exemplars which were rejected because they were out of order.
type PrometheusAPIOtlpOutOfOrderExemplarsTotal struct {
	prometheus.Counter
}

// NewPrometheusAPIOtlpOutOfOrderExemplarsTotal returns a new PrometheusAPIOtlpOutOfOrderExemplarsTotal instrument.
func NewPrometheusAPIOtlpOutOfOrderExemplarsTotal() PrometheusAPIOtlpOutOfOrderExemplarsTotal {
	return PrometheusAPIOtlpOutOfOrderExemplarsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_api_otlp_out_of_order_exemplars_total",
			Help: "The total number of received OTLP exemplars which were rejected because they were out of order.",
		}),
	}
}
