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
type ErrorAttr string

func (a ErrorAttr) ID() string {
	return "error"
}

func (a ErrorAttr) Value() string {
	return string(a)
}

// PrometheusTSDBWALReaderCorruptionErrorsTotal records the errors encountered when reading the WAL.
type PrometheusTSDBWALReaderCorruptionErrorsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBWALReaderCorruptionErrorsTotal returns a new PrometheusTSDBWALReaderCorruptionErrorsTotal instrument.
func NewPrometheusTSDBWALReaderCorruptionErrorsTotal() PrometheusTSDBWALReaderCorruptionErrorsTotal {
	labels := []string{
		"error",
	}
	return PrometheusTSDBWALReaderCorruptionErrorsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_wal_reader_corruption_errors_total",
			Help: "Errors encountered when reading the WAL.",
		}, labels),
	}
}

type PrometheusTSDBWALReaderCorruptionErrorsTotalAttr interface {
	Attribute
	implPrometheusTSDBWALReaderCorruptionErrorsTotal()
}

func (a ErrorAttr) implPrometheusTSDBWALReaderCorruptionErrorsTotal() {}

func (m PrometheusTSDBWALReaderCorruptionErrorsTotal) With(
	extra ...PrometheusTSDBWALReaderCorruptionErrorsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"error": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
