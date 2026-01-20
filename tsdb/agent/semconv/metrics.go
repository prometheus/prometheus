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
type TypeAttr string

func (a TypeAttr) ID() string {
	return "type"
}

func (a TypeAttr) Value() string {
	return string(a)
}

// PrometheusAgentActiveSeries records the number of active series being tracked by the WAL storage.
type PrometheusAgentActiveSeries struct {
	prometheus.Gauge
}

// NewPrometheusAgentActiveSeries returns a new PrometheusAgentActiveSeries instrument.
func NewPrometheusAgentActiveSeries() PrometheusAgentActiveSeries {
	return PrometheusAgentActiveSeries{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_agent_active_series",
			Help: "Number of active series being tracked by the WAL storage.",
		}),
	}
}

// PrometheusAgentCheckpointCreationsFailedTotal records the total number of checkpoint creations that failed.
type PrometheusAgentCheckpointCreationsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusAgentCheckpointCreationsFailedTotal returns a new PrometheusAgentCheckpointCreationsFailedTotal instrument.
func NewPrometheusAgentCheckpointCreationsFailedTotal() PrometheusAgentCheckpointCreationsFailedTotal {
	return PrometheusAgentCheckpointCreationsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_agent_checkpoint_creations_failed_total",
			Help: "Total number of checkpoint creations that failed.",
		}),
	}
}

// PrometheusAgentCheckpointCreationsTotal records the total number of checkpoint creations attempted.
type PrometheusAgentCheckpointCreationsTotal struct {
	prometheus.Counter
}

// NewPrometheusAgentCheckpointCreationsTotal returns a new PrometheusAgentCheckpointCreationsTotal instrument.
func NewPrometheusAgentCheckpointCreationsTotal() PrometheusAgentCheckpointCreationsTotal {
	return PrometheusAgentCheckpointCreationsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_agent_checkpoint_creations_total",
			Help: "Total number of checkpoint creations attempted.",
		}),
	}
}

// PrometheusAgentCheckpointDeletionsFailedTotal records the total number of checkpoint deletions that failed.
type PrometheusAgentCheckpointDeletionsFailedTotal struct {
	prometheus.Counter
}

// NewPrometheusAgentCheckpointDeletionsFailedTotal returns a new PrometheusAgentCheckpointDeletionsFailedTotal instrument.
func NewPrometheusAgentCheckpointDeletionsFailedTotal() PrometheusAgentCheckpointDeletionsFailedTotal {
	return PrometheusAgentCheckpointDeletionsFailedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_agent_checkpoint_deletions_failed_total",
			Help: "Total number of checkpoint deletions that failed.",
		}),
	}
}

// PrometheusAgentCheckpointDeletionsTotal records the total number of checkpoint deletions attempted.
type PrometheusAgentCheckpointDeletionsTotal struct {
	prometheus.Counter
}

// NewPrometheusAgentCheckpointDeletionsTotal returns a new PrometheusAgentCheckpointDeletionsTotal instrument.
func NewPrometheusAgentCheckpointDeletionsTotal() PrometheusAgentCheckpointDeletionsTotal {
	return PrometheusAgentCheckpointDeletionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_agent_checkpoint_deletions_total",
			Help: "Total number of checkpoint deletions attempted.",
		}),
	}
}

// PrometheusAgentCorruptionsTotal records the total number of WAL corruptions.
type PrometheusAgentCorruptionsTotal struct {
	prometheus.Counter
}

// NewPrometheusAgentCorruptionsTotal returns a new PrometheusAgentCorruptionsTotal instrument.
func NewPrometheusAgentCorruptionsTotal() PrometheusAgentCorruptionsTotal {
	return PrometheusAgentCorruptionsTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_agent_corruptions_total",
			Help: "Total number of WAL corruptions.",
		}),
	}
}

// PrometheusAgentDataReplayDurationSeconds records the time taken to replay the data on disk.
type PrometheusAgentDataReplayDurationSeconds struct {
	prometheus.Gauge
}

// NewPrometheusAgentDataReplayDurationSeconds returns a new PrometheusAgentDataReplayDurationSeconds instrument.
func NewPrometheusAgentDataReplayDurationSeconds() PrometheusAgentDataReplayDurationSeconds {
	return PrometheusAgentDataReplayDurationSeconds{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_agent_data_replay_duration_seconds",
			Help: "Time taken to replay the data on disk.",
		}),
	}
}

// PrometheusAgentDeletedSeries records the number of series pending deletion from the WAL.
type PrometheusAgentDeletedSeries struct {
	prometheus.Gauge
}

// NewPrometheusAgentDeletedSeries returns a new PrometheusAgentDeletedSeries instrument.
func NewPrometheusAgentDeletedSeries() PrometheusAgentDeletedSeries {
	return PrometheusAgentDeletedSeries{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_agent_deleted_series",
			Help: "Number of series pending deletion from the WAL.",
		}),
	}
}

// PrometheusAgentExemplarsAppendedTotal records the total number of exemplars appended to the storage.
type PrometheusAgentExemplarsAppendedTotal struct {
	prometheus.Counter
}

// NewPrometheusAgentExemplarsAppendedTotal returns a new PrometheusAgentExemplarsAppendedTotal instrument.
func NewPrometheusAgentExemplarsAppendedTotal() PrometheusAgentExemplarsAppendedTotal {
	return PrometheusAgentExemplarsAppendedTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_agent_exemplars_appended_total",
			Help: "Total number of exemplars appended to the storage.",
		}),
	}
}

// PrometheusAgentOutOfOrderSamplesTotal records the total number of out of order samples ingestion failed attempts.
type PrometheusAgentOutOfOrderSamplesTotal struct {
	prometheus.Counter
}

// NewPrometheusAgentOutOfOrderSamplesTotal returns a new PrometheusAgentOutOfOrderSamplesTotal instrument.
func NewPrometheusAgentOutOfOrderSamplesTotal() PrometheusAgentOutOfOrderSamplesTotal {
	return PrometheusAgentOutOfOrderSamplesTotal{
		Counter: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_agent_out_of_order_samples_total",
			Help: "Total number of out of order samples ingestion failed attempts.",
		}),
	}
}

// PrometheusAgentSamplesAppendedTotal records the total number of samples appended to the storage.
type PrometheusAgentSamplesAppendedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusAgentSamplesAppendedTotal returns a new PrometheusAgentSamplesAppendedTotal instrument.
func NewPrometheusAgentSamplesAppendedTotal() PrometheusAgentSamplesAppendedTotal {
	labels := []string{
		"type",
	}
	return PrometheusAgentSamplesAppendedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_agent_samples_appended_total",
			Help: "Total number of samples appended to the storage.",
		}, labels),
	}
}

type PrometheusAgentSamplesAppendedTotalAttr interface {
	Attribute
	implPrometheusAgentSamplesAppendedTotal()
}

func (a TypeAttr) implPrometheusAgentSamplesAppendedTotal() {}

func (m PrometheusAgentSamplesAppendedTotal) With(
	extra ...PrometheusAgentSamplesAppendedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"type": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusAgentTruncateDurationSeconds records the duration of WAL truncation.
type PrometheusAgentTruncateDurationSeconds struct {
	prometheus.Summary
}

// NewPrometheusAgentTruncateDurationSeconds returns a new PrometheusAgentTruncateDurationSeconds instrument.
func NewPrometheusAgentTruncateDurationSeconds() PrometheusAgentTruncateDurationSeconds {
	return PrometheusAgentTruncateDurationSeconds{
		Summary: prometheus.NewSummary(prometheus.SummaryOpts{
			Name:       "prometheus_agent_truncate_duration_seconds",
			Help:       "Duration of WAL truncation.",
			Objectives: map[float64]float64{},
		}),
	}
}
