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

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/metric"
)

// Storage ingests and manages samples, along with various indexes. All methods
// are goroutine-safe. Storage implements storage.SampleAppender.
type Storage interface {
	// Querier returns a new Querier on the storage.
	Querier() (Querier, error)

	// This SampleAppender needs multiple samples for the same fingerprint to be
	// submitted in chronological order, from oldest to newest. When Append has
	// returned, the appended sample might not be queryable immediately. (Use
	// WaitForIndexing to wait for complete processing.) The implementation might
	// remove labels with empty value from the provided Sample as those labels
	// are considered equivalent to a label not present at all.
	//
	// Appending is throttled if the Storage has too many chunks in memory
	// already or has too many chunks waiting for persistence.
	storage.SampleAppender

	// Drop all time series associated with the given label matchers. Returns
	// the number series that were dropped.
	DropMetricsForLabelMatchers(context.Context, ...*metric.LabelMatcher) (int, error)
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

// Querier allows querying a time series storage.
type Querier interface {
	// Close closes the querier. Behavior for subsequent calls to Querier methods
	// is undefined.
	Close() error
	// QueryRange returns a list of series iterators for the selected
	// time range and label matchers. The iterators need to be closed
	// after usage.
	QueryRange(ctx context.Context, from, through model.Time, matchers ...*metric.LabelMatcher) ([]SeriesIterator, error)
	// QueryInstant returns a list of series iterators for the selected
	// instant and label matchers. The iterators need to be closed after usage.
	QueryInstant(ctx context.Context, ts model.Time, stalenessDelta time.Duration, matchers ...*metric.LabelMatcher) ([]SeriesIterator, error)
	// MetricsForLabelMatchers returns the metrics from storage that satisfy
	// the given sets of label matchers. Each set of matchers must contain at
	// least one label matcher that does not match the empty string. Otherwise,
	// an empty list is returned. Within one set of matchers, the intersection
	// of matching series is computed. The final return value will be the union
	// of the per-set results. The times from and through are hints for the
	// storage to optimize the search. The storage MAY exclude metrics that
	// have no samples in the specified interval from the returned map. In
	// doubt, specify model.Earliest for from and model.Latest for through.
	MetricsForLabelMatchers(ctx context.Context, from, through model.Time, matcherSets ...metric.LabelMatchers) ([]metric.Metric, error)
	// LastSampleForLabelMatchers returns the last samples that have been
	// ingested for the time series matching the given set of label matchers.
	// The label matching behavior is the same as in MetricsForLabelMatchers.
	// All returned samples are between the specified cutoff time and now.
	LastSampleForLabelMatchers(ctx context.Context, cutoff model.Time, matcherSets ...metric.LabelMatchers) (model.Vector, error)
	// Get all of the label values that are associated with a given label name.
	LabelValuesForLabelName(context.Context, model.LabelName) (model.LabelValues, error)
}

// SeriesIterator enables efficient access of sample values in a series. Its
// methods are not goroutine-safe. A SeriesIterator iterates over a snapshot of
// a series, i.e. it is safe to continue using a SeriesIterator after or during
// modifying the corresponding series, but the iterator will represent the state
// of the series prior to the modification.
type SeriesIterator interface {
	// Gets the value that is closest before the given time. In case a value
	// exists at precisely the given time, that value is returned. If no
	// applicable value exists, model.ZeroSamplePair is returned.
	ValueAtOrBeforeTime(model.Time) model.SamplePair
	// Gets all values contained within a given interval.
	RangeValues(metric.Interval) []model.SamplePair
	// Returns the metric of the series that the iterator corresponds to.
	Metric() metric.Metric
	// Closes the iterator and releases the underlying data.
	Close()
}
