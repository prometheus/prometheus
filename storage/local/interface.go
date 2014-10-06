// Copyright 2014 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package local

import (
	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/storage/metric"
)

// SeriesMap maps fingerprints to memory series.
type SeriesMap map[clientmodel.Fingerprint]*memorySeries

// Storage ingests and manages samples, along with various indexes. All methods
// are goroutine-safe.
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
	// Construct an iterator for a given fingerprint.
	NewIterator(clientmodel.Fingerprint) SeriesIterator
	// Run the request-serving and maintenance loop.
	Serve(started chan<- bool)
	// Close the MetricsStorage and releases all resources.
	Close() error
}

// SeriesIterator enables efficient access of sample values in a series. All
// methods are goroutine-safe. A SeriesIterator iterates over a snapshot of a
// series, i.e. it is safe to continue using a SeriesIterator after modifying
// the corresponding series, but the iterator will represent the state of the
// series prior the modification.
type SeriesIterator interface {
	// Gets the two values that are immediately adjacent to a given time. In
	// case a value exist at precisely the given time, only that single
	// value is returned. Only the first or last value is returned (as a
	// single value), if the given time is before or after the first or last
	// value, respectively.
	GetValueAtTime(clientmodel.Timestamp) metric.Values
	// Gets the boundary values of an interval: the first and last value
	// within a given interval.
	GetBoundaryValues(metric.Interval) metric.Values
	// Gets all values contained within a given interval.
	GetRangeValues(metric.Interval) metric.Values
}

// A Persistence is used by a Storage implementation to store samples
// persistently across restarts. The methods are generally not goroutine-safe
// unless marked otherwise. The chunk-related methods PersistChunk, DropChunks,
// LoadChunks, and LoadChunkDescs can be called concurrently with each other if
// each call refers to a different fingerprint.
//
// TODO: As a Persistence is really only used within this package, consider not
// exporting it.
type Persistence interface {
	// PersistChunk persists a single chunk of a series. It is the caller's
	// responsibility to not modify chunk concurrently.
	PersistChunk(clientmodel.Fingerprint, chunk) error
	// DropChunks deletes all chunks from a series whose last sample time is
	// before beforeTime. It returns true if all chunks of the series have
	// been deleted.
	DropChunks(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) (allDropped bool, err error)
	// LoadChunks loads a group of chunks of a timeseries by their index. The
	// chunk with the earliest time will have index 0, the following ones will
	// have incrementally larger indexes.
	LoadChunks(fp clientmodel.Fingerprint, indexes []int) (chunks, error)
	// LoadChunkDescs loads chunkDescs for a series up until a given time.
	LoadChunkDescs(fp clientmodel.Fingerprint, beforeTime clientmodel.Timestamp) (chunkDescs, error)

	// PersistSeriesMapAndHeads persists the fingerprint to memory-series
	// mapping and all open (non-full) head chunks. It is the caller's
	// responsibility to not modify SeriesMap concurrently. Do not call
	// concurrently with LoadSeriesMapAndHeads.
	PersistSeriesMapAndHeads(SeriesMap) error
	// LoadSeriesMapAndHeads loads the fingerprint to memory-series mapping
	// and all open (non-full) head chunks. Do not call
	// concurrently with PersistSeriesMapAndHeads.
	LoadSeriesMapAndHeads() (SeriesMap, error)

	// GetFingerprintsForLabelPair returns the fingerprints for the given
	// label pair. This method is goroutine-safe but take into account that
	// metrics queued for indexing with IndexMetric might not yet made it
	// into the index. (Same applies correspondingly to UnindexMetric.)
	GetFingerprintsForLabelPair(metric.LabelPair) (clientmodel.Fingerprints, error)
	// GetLabelValuesForLabelName returns the label values for the given
	// label name. This method is goroutine-safe but take into account that
	// metrics queued for indexing with IndexMetric might not yet made it
	// into the index. (Same applies correspondingly to UnindexMetric.)
	GetLabelValuesForLabelName(clientmodel.LabelName) (clientmodel.LabelValues, error)

	// IndexMetric queues the given metric for addition to the indexes
	// needed by GetFingerprintsForLabelPair and GetLabelValuesForLabelName.
	// If the queue is full, this method blocks until the metric can be queued.
	// This method is goroutine-safe.
	IndexMetric(clientmodel.Metric, clientmodel.Fingerprint)
	// UnindexMetric queues references to the given metric for removal from
	// the indexes used for GetFingerprintsForLabelPair and
	// GetLabelValuesForLabelName. The index of fingerprints to archived
	// metrics is not affected by this removal. (In fact, never call this
	// method for an archived metric. To drop an archived metric, call
	// DropArchivedFingerprint.)  If the queue is full, this method blocks
	// until the metric can be queued. This method is goroutine-safe.
	UnindexMetric(clientmodel.Metric, clientmodel.Fingerprint)
	// WaitForIndexing waits until all items in the indexing queue are
	// processed. If queue processing is currently on hold (to gather more
	// ops for batching), this method will trigger an immediate start of
	// processing. This method is goroutine-safe.
	WaitForIndexing()

	// ArchiveMetric persists the mapping of the given fingerprint to the
	// given metric, together with the first and last timestamp of the
	// series belonging to the metric. Do not call concurrently with
	// UnarchiveMetric or DropArchivedMetric.
	ArchiveMetric(
		fingerprint clientmodel.Fingerprint, metric clientmodel.Metric,
		firstTime, lastTime clientmodel.Timestamp,
	) error
	// HasArchivedMetric returns whether the archived metric for the given
	// fingerprint exists and if yes, what the first and last timestamp in
	// the corresponding series is. This method is goroutine-safe.
	HasArchivedMetric(clientmodel.Fingerprint) (
		hasMetric bool, firstTime, lastTime clientmodel.Timestamp, err error,
	)
	// GetFingerprintsModifiedBefore returns the fingerprints of archived
	// timeseries that have live samples before the provided timestamp. This
	// method is goroutine-safe (but behavior during concurrent modification
	// via ArchiveMetric, UnarchiveMetric, or DropArchivedMetric is
	// undefined).
	GetFingerprintsModifiedBefore(clientmodel.Timestamp) ([]clientmodel.Fingerprint, error)
	// GetArchivedMetric retrieves the archived metric with the given
	// fingerprint. This method is goroutine-safe.
	GetArchivedMetric(clientmodel.Fingerprint) (clientmodel.Metric, error)
	// DropArchivedMetric deletes an archived fingerprint and its
	// corresponding metric entirely. It also queues the metric for
	// un-indexing (no need to call UnindexMetric for the deleted metric.)
	// Do not call concurrently with UnarchiveMetric or ArchiveMetric.
	DropArchivedMetric(clientmodel.Fingerprint) error
	// UnarchiveMetric deletes an archived fingerprint and its metric, but
	// (in contrast to DropArchivedMetric) does not un-index the metric.
	// The method returns true if a metric was actually deleted. Do not call
	// concurrently with DropArchivedMetric or ArchiveMetric.
	UnarchiveMetric(clientmodel.Fingerprint) (bool, error)

	// Close flushes the indexing queue and other buffered data and releases
	// any held resources.
	Close() error
}

// A Preloader preloads series data necessary for a query into memory and pins
// them until released via Close(). Its methods are generally not
// goroutine-safe.
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
