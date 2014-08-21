package index

import (
	clientmodel "github.com/prometheus/client_golang/model"
)

// MetricIndexer indexes facets of a clientmodel.Metric. The interface makes no
// assumptions about the concurrency safety of the underlying implementer.
type MetricIndexer interface {
	// IndexMetrics adds metrics to the index.
	IndexMetrics(FingerprintMetricMapping) error
	// UnindexMetrics removes metrics from the index.
	UnindexMetrics(FingerprintMetricMapping) error

	// GetMetricForFingerprint returns the metric associated with the provided fingerprint.
	GetMetricForFingerprint(clientmodel.Fingerprint) (clientmodel.Metric, error)
	// GetFingerprintsForLabelPair returns all fingerprints for the provided label pair.
	GetFingerprintsForLabelPair(l clientmodel.LabelName, v clientmodel.LabelValue) (clientmodel.Fingerprints, error)
	// GetLabelValuesForLabelName returns all label values associated with a given label name.
	GetLabelValuesForLabelName(clientmodel.LabelName) (clientmodel.LabelValues, error)
	// HasFingerprint returns true if a metric with the given fingerprint has been indexed.
	HasFingerprint(clientmodel.Fingerprint) (bool, error)
}

// KeyValueStore persists key/value pairs.
type KeyValueStore interface {
	Put(key, value encodable) error
	Get(k encodable, v decodable) (bool, error)
	Has(k encodable) (has bool, err error)
	Delete(k encodable) error

	NewBatch() Batch
	Commit(b Batch) error

	Close() error
}

// Batch allows KeyValueStore mutations to be pooled and committed together.
type Batch interface {
	Put(key, value encodable)
	Delete(key encodable)
	Reset()
}
