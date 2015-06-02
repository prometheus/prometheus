// Copyright 2014 The Prometheus Authors
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
	"time"

	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// Storage ingests and manages samples, along with various indexes. All methods
// are goroutine-safe. Storage implements storage.SampleAppender.
type Storage interface {
	prometheus.Collector
	// Append stores a sample in the Storage. Multiple samples for the same
	// fingerprint need to be submitted in chronological order, from oldest
	// to newest. When Append has returned, the appended sample might not be
	// queryable immediately. (Use WaitForIndexing to wait for complete
	// processing.)
	Append(*clientmodel.Sample)
	// NewPreloader returns a new Preloader which allows preloading and pinning
	// series data into memory for use within a query.
	NewPreloader() Preloader
	// Get all of the metric fingerprints that are associated with the
	// provided label matchers.
	FingerprintsForLabelMatchers(metric.LabelMatchers) clientmodel.Fingerprints
	// Get all of the label values that are associated with a given label name.
	LabelValuesForLabelName(clientmodel.LabelName) clientmodel.LabelValues
	// Get the metric associated with the provided fingerprint.
	MetricForFingerprint(clientmodel.Fingerprint) clientmodel.COWMetric
	// Construct an iterator for a given fingerprint.
	// The iterator will never return samples older than retention time,
	// relative to the time NewIterator was called.
	NewIterator(clientmodel.Fingerprint) SeriesIterator
	// Drop all time series associated with the given fingerprints. This operation
	// will not show up in the series operations metrics.
	DropMetricsForFingerprints(...clientmodel.Fingerprint)
	// Run the various maintenance loops in goroutines. Returns when the
	// storage is ready to use. Keeps everything running in the background
	// until Stop is called.
	Start() error
	// Stop shuts down the Storage gracefully, flushes all pending
	// operations, stops all maintenance loops,and frees all resources.
	Stop() error
	// WaitForIndexing returns once all samples in the storage are
	// indexed. Indexing is needed for FingerprintsForLabelMatchers and
	// LabelValuesForLabelName and may lag behind.
	WaitForIndexing()
}

// SeriesIterator enables efficient access of sample values in a series. Its
// methods are not goroutine-safe. A SeriesIterator iterates over a snapshot of
// a series, i.e. it is safe to continue using a SeriesIterator after or during
// modifying the corresponding series, but the iterator will represent the state
// of the series prior the modification.
type SeriesIterator interface {
	// Gets the two values that are immediately adjacent to a given time. In
	// case a value exist at precisely the given time, only that single
	// value is returned. Only the first or last value is returned (as a
	// single value), if the given time is before or after the first or last
	// value, respectively.
	ValueAtTime(clientmodel.Timestamp) metric.Values
	// Gets the boundary values of an interval: the first and last value
	// within a given interval.
	BoundaryValues(metric.Interval) metric.Values
	// Gets all values contained within a given interval.
	RangeValues(metric.Interval) metric.Values
}

// A Preloader preloads series data necessary for a query into memory and pins
// them until released via Close(). Its methods are generally not
// goroutine-safe.
type Preloader interface {
	PreloadRange(
		fp clientmodel.Fingerprint,
		from clientmodel.Timestamp, through clientmodel.Timestamp,
		stalenessDelta time.Duration,
	) error
	// Close unpins any previously requested series data from memory.
	Close()
}
