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
type OperationAttr string

func (a OperationAttr) ID() string {
	return "operation"
}

func (a OperationAttr) Value() string {
	return string(a)
}

// PrometheusTSDBChunkWriteQueueOperationsTotal records the number of operations on the chunk_write_queue.
type PrometheusTSDBChunkWriteQueueOperationsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTSDBChunkWriteQueueOperationsTotal returns a new PrometheusTSDBChunkWriteQueueOperationsTotal instrument.
func NewPrometheusTSDBChunkWriteQueueOperationsTotal() PrometheusTSDBChunkWriteQueueOperationsTotal {
	labels := []string{
		"operation",
	}
	return PrometheusTSDBChunkWriteQueueOperationsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tsdb_chunk_write_queue_operations_total",
			Help: "Number of operations on the chunk_write_queue.",
		}, labels),
	}
}

type PrometheusTSDBChunkWriteQueueOperationsTotalAttr interface {
	Attribute
	implPrometheusTSDBChunkWriteQueueOperationsTotal()
}

func (a OperationAttr) implPrometheusTSDBChunkWriteQueueOperationsTotal() {}

func (m PrometheusTSDBChunkWriteQueueOperationsTotal) With(
	extra ...PrometheusTSDBChunkWriteQueueOperationsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"operation": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}
