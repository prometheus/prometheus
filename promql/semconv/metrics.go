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
type SliceAttr string

func (a SliceAttr) ID() string {
	return "slice"
}

func (a SliceAttr) Value() string {
	return string(a)
}

// PrometheusEngineQueries records the current number of queries being executed or waiting.
type PrometheusEngineQueries struct {
	*prometheus.GaugeVec
}

// NewPrometheusEngineQueries returns a new PrometheusEngineQueries instrument.
func NewPrometheusEngineQueries() PrometheusEngineQueries {
	labels := []string{}
	return PrometheusEngineQueries{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_engine_queries",
			Help: "The current number of queries being executed or waiting.",
		}, labels),
	}
}

type PrometheusEngineQueriesAttr interface {
	Attribute
	implPrometheusEngineQueries()
}

func (m PrometheusEngineQueries) With(
	extra ...PrometheusEngineQueriesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusEngineQueriesConcurrentMax records the max number of concurrent queries.
type PrometheusEngineQueriesConcurrentMax struct {
	*prometheus.GaugeVec
}

// NewPrometheusEngineQueriesConcurrentMax returns a new PrometheusEngineQueriesConcurrentMax instrument.
func NewPrometheusEngineQueriesConcurrentMax() PrometheusEngineQueriesConcurrentMax {
	labels := []string{}
	return PrometheusEngineQueriesConcurrentMax{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_engine_queries_concurrent_max",
			Help: "The max number of concurrent queries.",
		}, labels),
	}
}

type PrometheusEngineQueriesConcurrentMaxAttr interface {
	Attribute
	implPrometheusEngineQueriesConcurrentMax()
}

func (m PrometheusEngineQueriesConcurrentMax) With(
	extra ...PrometheusEngineQueriesConcurrentMaxAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusEngineQueryDurationHistogramSeconds records the histogram of query timings.
type PrometheusEngineQueryDurationHistogramSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusEngineQueryDurationHistogramSeconds returns a new PrometheusEngineQueryDurationHistogramSeconds instrument.
func NewPrometheusEngineQueryDurationHistogramSeconds() PrometheusEngineQueryDurationHistogramSeconds {
	labels := []string{
		"slice",
	}
	return PrometheusEngineQueryDurationHistogramSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_engine_query_duration_histogram_seconds",
			Help: "Histogram of query timings.",
		}, labels),
	}
}

type PrometheusEngineQueryDurationHistogramSecondsAttr interface {
	Attribute
	implPrometheusEngineQueryDurationHistogramSeconds()
}

func (a SliceAttr) implPrometheusEngineQueryDurationHistogramSeconds() {}

func (m PrometheusEngineQueryDurationHistogramSeconds) With(
	extra ...PrometheusEngineQueryDurationHistogramSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"slice": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusEngineQueryDurationSeconds records the query timings.
type PrometheusEngineQueryDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusEngineQueryDurationSeconds returns a new PrometheusEngineQueryDurationSeconds instrument.
func NewPrometheusEngineQueryDurationSeconds() PrometheusEngineQueryDurationSeconds {
	labels := []string{}
	return PrometheusEngineQueryDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_engine_query_duration_seconds",
			Help: "Query timings.",
		}, labels),
	}
}

type PrometheusEngineQueryDurationSecondsAttr interface {
	Attribute
	implPrometheusEngineQueryDurationSeconds()
}

func (m PrometheusEngineQueryDurationSeconds) With(
	extra ...PrometheusEngineQueryDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusEngineQueryLogEnabled records the state of the query log.
type PrometheusEngineQueryLogEnabled struct {
	*prometheus.GaugeVec
}

// NewPrometheusEngineQueryLogEnabled returns a new PrometheusEngineQueryLogEnabled instrument.
func NewPrometheusEngineQueryLogEnabled() PrometheusEngineQueryLogEnabled {
	labels := []string{}
	return PrometheusEngineQueryLogEnabled{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_engine_query_log_enabled",
			Help: "State of the query log.",
		}, labels),
	}
}

type PrometheusEngineQueryLogEnabledAttr interface {
	Attribute
	implPrometheusEngineQueryLogEnabled()
}

func (m PrometheusEngineQueryLogEnabled) With(
	extra ...PrometheusEngineQueryLogEnabledAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusEngineQueryLogFailuresTotal records the number of query log failures.
type PrometheusEngineQueryLogFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusEngineQueryLogFailuresTotal returns a new PrometheusEngineQueryLogFailuresTotal instrument.
func NewPrometheusEngineQueryLogFailuresTotal() PrometheusEngineQueryLogFailuresTotal {
	labels := []string{}
	return PrometheusEngineQueryLogFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_engine_query_log_failures_total",
			Help: "The number of query log failures.",
		}, labels),
	}
}

type PrometheusEngineQueryLogFailuresTotalAttr interface {
	Attribute
	implPrometheusEngineQueryLogFailuresTotal()
}

func (m PrometheusEngineQueryLogFailuresTotal) With(
	extra ...PrometheusEngineQueryLogFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusEngineQuerySamplesTotal records the total number of samples loaded by all queries.
type PrometheusEngineQuerySamplesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusEngineQuerySamplesTotal returns a new PrometheusEngineQuerySamplesTotal instrument.
func NewPrometheusEngineQuerySamplesTotal() PrometheusEngineQuerySamplesTotal {
	labels := []string{}
	return PrometheusEngineQuerySamplesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_engine_query_samples_total",
			Help: "The total number of samples loaded by all queries.",
		}, labels),
	}
}

type PrometheusEngineQuerySamplesTotalAttr interface {
	Attribute
	implPrometheusEngineQuerySamplesTotal()
}

func (m PrometheusEngineQuerySamplesTotal) With(
	extra ...PrometheusEngineQuerySamplesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
