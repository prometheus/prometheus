package index

import (
	"encoding"

	clientmodel "github.com/prometheus/client_golang/model"
)

// MetricIndexer indexes facets of a clientmodel.Metric. Implementers may or may
// not be concurrency-safe.
type MetricIndexer interface {
	// IndexMetrics adds metrics to the index. If the metrics was added
	// before and has been archived in the meantime, it is now un-archived.
	IndexMetrics(FingerprintMetricMapping) error
	// UnindexMetrics removes metrics from the index.
	UnindexMetrics(FingerprintMetricMapping) error
	// ArchiveMetrics marks the metric with the given fingerprint as
	// 'archived', which has to be called if upon eviction of the
	// corresponding time series from memory. By calling this method, the
	// MetricIndexer learns about the time range of the evicted time series,
	// which is used later for the decision if an evicted time series has to
	// be moved back into memory. The implementer of MetricIndexer can make
	// use of the archived state, e.g. by saving archived metrics in an
	// on-disk index and non-archived metrics in an in-memory index.
	ArchiveMetrics(fp clientmodel.Fingerprint, first, last clientmodel.Timestamp) error

	// GetMetricForFingerprint returns the metric associated with the provided fingerprint.
	GetMetricForFingerprint(clientmodel.Fingerprint) (clientmodel.Metric, error)
	// GetFingerprintsForLabelPair returns all fingerprints for the provided label pair.
	GetFingerprintsForLabelPair(l clientmodel.LabelName, v clientmodel.LabelValue) (clientmodel.Fingerprints, error)
	// GetLabelValuesForLabelName returns all label values associated with a given label name.
	GetLabelValuesForLabelName(clientmodel.LabelName) (clientmodel.LabelValues, error)
	// HasFingerprint returns true if a metric with the given fingerprint
	// has been indexed and has NOT been archived yet.
	HasFingerprint(clientmodel.Fingerprint) (bool, error)
	// HasArchivedFingerprint returns true if a metric with the given
	// fingerprint was indexed before and has been archived in the
	// meantime. In that case, the time range of the archived metric is also
	// returned.
	HasArchivedFingerprint(clientmodel.Fingerprint) (present bool, first, last clientmodel.Timestamp, err error)

	Close() error
}

// KeyValueStore persists key/value pairs.
type KeyValueStore interface {
	Put(key, value encoding.BinaryMarshaler) error
	Get(k encoding.BinaryMarshaler, v encoding.BinaryUnmarshaler) (bool, error)
	Has(k encoding.BinaryMarshaler) (has bool, err error)
	Delete(k encoding.BinaryMarshaler) error

	NewBatch() Batch
	Commit(b Batch) error

	Close() error
}

// Batch allows KeyValueStore mutations to be pooled and committed together.
type Batch interface {
	Put(key, value encoding.BinaryMarshaler) error
	Delete(key encoding.BinaryMarshaler) error
	Reset()
}
