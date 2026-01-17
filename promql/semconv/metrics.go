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
type SliceAttr string

func (a SliceAttr) ID() string {
	return "slice"
}

func (a SliceAttr) Value() string {
	return string(a)
}

// PrometheusEngineQueries records the current number of queries being executed or waiting.
type PrometheusEngineQueries struct {
	prometheus.Gauge
}

// NewPrometheusEngineQueries returns a new PrometheusEngineQueries instrument.
func NewPrometheusEngineQueries() PrometheusEngineQueries {
	return PrometheusEngineQueries{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_engine_queries",
			Help: "The current number of queries being executed or waiting.",
		}),
	}
}

// PrometheusEngineQueriesConcurrentMax records the max number of concurrent queries.
type PrometheusEngineQueriesConcurrentMax struct {
	prometheus.Gauge
}

// NewPrometheusEngineQueriesConcurrentMax returns a new PrometheusEngineQueriesConcurrentMax instrument.
func NewPrometheusEngineQueriesConcurrentMax() PrometheusEngineQueriesConcurrentMax {
	return PrometheusEngineQueriesConcurrentMax{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_engine_queries_concurrent_max",
			Help: "The max number of concurrent queries.",
		}),
	}
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
			Name:                            "prometheus_engine_query_duration_histogram_seconds",
			Help:                            "Histogram of query timings.",
			Buckets:                         []float64{0.01, 0.1, 1, 10},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
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
	*prometheus.SummaryVec
}

// NewPrometheusEngineQueryDurationSeconds returns a new PrometheusEngineQueryDurationSeconds instrument.
func NewPrometheusEngineQueryDurationSeconds() PrometheusEngineQueryDurationSeconds {
	labels := []string{
		"slice",
	}
	return PrometheusEngineQueryDurationSeconds{
		SummaryVec: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name: "prometheus_engine_query_duration_seconds",
			Help: "Query timings.",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, labels),
	}
}

type PrometheusEngineQueryDurationSecondsAttr interface {
	Attribute
	implPrometheusEngineQueryDurationSeconds()
}

func (a SliceAttr) implPrometheusEngineQueryDurationSeconds() {}

func (m PrometheusEngineQueryDurationSeconds) With(
	extra ...PrometheusEngineQueryDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"slice": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.SummaryVec.With(labels)
}

// PrometheusEngineQueryLogEnabled records the state of the query log.
type PrometheusEngineQueryLogEnabled struct {
	prometheus.Gauge
}

// NewPrometheusEngineQueryLogEnabled returns a new PrometheusEngineQueryLogEnabled instrument.
func NewPrometheusEngineQueryLogEnabled() PrometheusEngineQueryLogEnabled {
	return PrometheusEngineQueryLogEnabled{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_engine_query_log_enabled",
			Help: "State of the query log.",
		}),
	}
}

// PrometheusEngineQueryLogFailuresTotal records the number of query log failures.
type PrometheusEngineQueryLogFailuresTotal struct {
	prometheus.Counter
}

// NewPrometheusEngineQueryLogFailuresTotal returns a new PrometheusEngineQueryLogFailuresTotal instrument.
func NewPrometheusEngineQueryLogFailuresTotal() PrometheusEngineQueryLogFailuresTotal {
	return PrometheusEngineQueryLogFailuresTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_engine_query_log_failures_total",
			Help: "The number of query log failures.",
		}),
	}
}

// PrometheusEngineQuerySamplesTotal records the total number of samples loaded by all queries.
type PrometheusEngineQuerySamplesTotal struct {
	prometheus.Counter
}

// NewPrometheusEngineQuerySamplesTotal returns a new PrometheusEngineQuerySamplesTotal instrument.
func NewPrometheusEngineQuerySamplesTotal() PrometheusEngineQuerySamplesTotal {
	return PrometheusEngineQuerySamplesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_engine_query_samples_total",
			Help: "The total number of samples loaded by all queries.",
		}),
	}
}
