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
	*prometheus.CounterVec
}

// NewPrometheusTemplateTextExpansionFailuresTotal returns a new PrometheusTemplateTextExpansionFailuresTotal instrument.
func NewPrometheusTemplateTextExpansionFailuresTotal() PrometheusTemplateTextExpansionFailuresTotal {
	labels := []string{}
	return PrometheusTemplateTextExpansionFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_template_text_expansion_failures_total",
			Help: "The total number of template text expansion failures.",
		}, labels),
	}
}

type PrometheusTemplateTextExpansionFailuresTotalAttr interface {
	Attribute
	implPrometheusTemplateTextExpansionFailuresTotal()
}

func (m PrometheusTemplateTextExpansionFailuresTotal) With(
	extra ...PrometheusTemplateTextExpansionFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTemplateTextExpansionsTotal records the total number of template text expansions.
type PrometheusTemplateTextExpansionsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTemplateTextExpansionsTotal returns a new PrometheusTemplateTextExpansionsTotal instrument.
func NewPrometheusTemplateTextExpansionsTotal() PrometheusTemplateTextExpansionsTotal {
	labels := []string{}
	return PrometheusTemplateTextExpansionsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_template_text_expansions_total",
			Help: "The total number of template text expansions.",
		}, labels),
	}
}

type PrometheusTemplateTextExpansionsTotalAttr interface {
	Attribute
	implPrometheusTemplateTextExpansionsTotal()
}

func (m PrometheusTemplateTextExpansionsTotal) With(
	extra ...PrometheusTemplateTextExpansionsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
