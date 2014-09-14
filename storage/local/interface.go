package storage_ng

import (
	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/storage/metric"
)

// SeriesMap maps fingerprints to memory series.
type SeriesMap map[clientmodel.Fingerprint]*memorySeries

type Storage interface {
	// AppendSamples stores a group of new samples. Multiple samples for the same
	// fingerprint need to be submitted in chronological order, from oldest to
	// newest (both in the same call to AppendSamples and across multiple calls).
	AppendSamples(clientmodel.Samples)
	// NewPreloader returns a new Preloader which allows preloading and pinning
	// series data into memory for use within a query.
	NewPreloader() Preloader
	// Get all of the metric fingerprints that are associated with the
	// provided label matchers.
	GetFingerprintsForLabelMatchers(metric.LabelMatchers) clientmodel.Fingerprints
	// Get all of the label values that are associated with a given label name.
	GetLabelValuesForLabelName(clientmodel.LabelName) clientmodel.LabelValues
	// Get the metric associated with the provided fingerprint.
	GetMetricForFingerprint(clientmodel.Fingerprint) clientmodel.Metric
	// Get all label values that are associated with a given label name.
	GetAllValuesForLabel(clientmodel.LabelName) clientmodel.LabelValues
	// Construct an iterator for a given fingerprint.
	NewIterator(clientmodel.Fingerprint) SeriesIterator
	// Run the request-serving and maintenance loop.
	Serve(started chan<- bool)
	// Close the MetricsStorage and releases all resources.
	Close() error
}

type SeriesIterator interface {
	// Get the two values that are immediately adjacent to a given time.
	GetValueAtTime(clientmodel.Timestamp) metric.Values
	// Get the boundary values of an interval: the first value older than
	// the interval start, and the first value younger than the interval
	// end.
	GetBoundaryValues(metric.Interval) metric.Values
	// Get all values contained within a provided interval.
	GetRangeValues(metric.Interval) metric.Values
}

// A Persistence stores samples persistently across restarts.
type Persistence interface {
	// PersistChunk persists a single chunk of a series.
	PersistChunk(clientmodel.Fingerprint, chunk) error
	// PersistSeriesMapAndHeads persists the fingerprint to memory-series
	// mapping and all open (non-full) head chunks.
	PersistSeriesMapAndHeads(SeriesMap) error

	// DropChunks deletes all chunks from a timeseries whose last sample time is
	// before beforeTime.
	DropChunks(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) (allDropped bool, err error)

	// LoadChunks loads a group of chunks of a timeseries by their index. The
	// chunk with the earliest time will have index 0, the following ones will
	// have incrementally larger indexes.
	LoadChunks(fp clientmodel.Fingerprint, indexes []int) (chunks, error)
	// LoadChunkDescs loads chunkDescs for a series up until a given time.
	LoadChunkDescs(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) (chunkDescs, error)
	// LoadSeriesMapAndHeads loads the fingerprint to memory-series mapping
	// and all open (non-full) head chunks.
	LoadSeriesMapAndHeads() (SeriesMap, error)

	// GetFingerprintsForLabelPair returns the fingerprints for the given
	// label pair.
	GetFingerprintsForLabelPair(metric.LabelPair) (clientmodel.Fingerprints, error)
	// GetLabelValuesForLabelName returns the label values for the given
	// label name.
	GetLabelValuesForLabelName(clientmodel.LabelName) (clientmodel.LabelValues, error)

	// IndexMetric indexes the given metric for the needs of
	// GetFingerprintsForLabelPair and GetLabelValuesForLabelName.
	IndexMetric(clientmodel.Metric, clientmodel.Fingerprint) error
	// UnindexMetric removes references to the given metric from the indexes
	// used for GetFingerprintsForLabelPair and
	// GetLabelValuesForLabelName. The index of fingerprints to archived
	// metrics is not affected by this method. (In fact, never call this
	// method for an archived metric. To drop an archived metric, call
	// DropArchivedFingerprint.)
	UnindexMetric(clientmodel.Metric, clientmodel.Fingerprint) error

	// ArchiveMetric persists the mapping of the given fingerprint to the
	// given metric, together with the first and last timestamp of the
	// series belonging to the metric.
	ArchiveMetric(
		fingerprint clientmodel.Fingerprint, metric clientmodel.Metric,
		firstTime, lastTime clientmodel.Timestamp,
	) error
	// HasArchivedMetric returns whether the archived metric for the given
	// fingerprint exists and if yes, what the first and last timestamp in
	// the corresponding series is.
	HasArchivedMetric(clientmodel.Fingerprint) (
		hasMetric bool, firstTime, lastTime clientmodel.Timestamp, err error,
	)
	// GetArchivedMetric retrieves the archived metric with the given
	// fingerprint.
	GetArchivedMetric(clientmodel.Fingerprint) (clientmodel.Metric, error)
	// DropArchivedMetric deletes an archived fingerprint and its
	// corresponding metric entirely. It also un-indexes the metric (no need
	// to call UnindexMetric for the deleted metric.)
	DropArchivedMetric(clientmodel.Fingerprint) error
	// UnarchiveMetric deletes an archived fingerprint and its metric, but
	// (in contrast to DropArchivedMetric) does not un-index the metric.
	// The method returns true if a metric was actually deleted.
	UnarchiveMetric(clientmodel.Fingerprint) (bool, error)

	// Close flushes buffered data and releases any held resources.
	Close() error
}

// A Preloader preloads series data necessary for a query into memory and pins
// them until released via Close().
type Preloader interface {
	PreloadRange(fp clientmodel.Fingerprint, from clientmodel.Timestamp, through clientmodel.Timestamp) error
	/*
		// GetMetricAtTime loads and pins samples around a given time.
		GetMetricAtTime(clientmodel.Fingerprint, clientmodel.Timestamp) error
		// GetMetricAtInterval loads and pins samples at intervals.
		GetMetricAtInterval(fp clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval time.Duration) error
		// GetMetricRange loads and pins a given range of samples.
		GetMetricRange(fp clientmodel.Fingerprint, from, through clientmodel.Timestamp) error
		// GetMetricRangeAtInterval loads and pins sample ranges at intervals.
		GetMetricRangeAtInterval(fp clientmodel.Fingerprint, from, through clientmodel.Timestamp, interval, rangeDuration time.Duration) error
	*/
	// Close unpins any previously requested series data from memory.
	Close()
}
