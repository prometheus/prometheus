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

// PrometheusTemplateTextExpansionFailuresTotal records the total number of template text expansion failures.
type PrometheusTemplateTextExpansionFailuresTotal struct {
	prometheus.Counter
}

// NewPrometheusTemplateTextExpansionFailuresTotal returns a new PrometheusTemplateTextExpansionFailuresTotal instrument.
func NewPrometheusTemplateTextExpansionFailuresTotal() PrometheusTemplateTextExpansionFailuresTotal {
	return PrometheusTemplateTextExpansionFailuresTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_template_text_expansion_failures_total",
			Help: "The total number of template text expansion failures.",
		}),
	}
}

// PrometheusTemplateTextExpansionsTotal records the total number of template text expansions.
type PrometheusTemplateTextExpansionsTotal struct {
	prometheus.Counter
}

// NewPrometheusTemplateTextExpansionsTotal returns a new PrometheusTemplateTextExpansionsTotal instrument.
func NewPrometheusTemplateTextExpansionsTotal() PrometheusTemplateTextExpansionsTotal {
	return PrometheusTemplateTextExpansionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_template_text_expansions_total",
			Help: "The total number of template text expansions.",
		}),
	}
}
