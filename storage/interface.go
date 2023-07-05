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

package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// The errors exposed.
var (
	ErrNotFound = errors.New("not found")
	// ErrOutOfOrderSample is when out of order support is disabled and the sample is out of order.
	ErrOutOfOrderSample = errors.New("out of order sample")
	// ErrOutOfBounds is when out of order support is disabled and the sample is older than the min valid time for the append.
	ErrOutOfBounds = errors.New("out of bounds")
	// ErrTooOldSample is when out of order support is enabled but the sample is outside the time window allowed.
	ErrTooOldSample = errors.New("too old sample")
	// ErrDuplicateSampleForTimestamp is when the sample has same timestamp but different value.
	ErrDuplicateSampleForTimestamp   = errors.New("duplicate sample for timestamp")
	ErrOutOfOrderExemplar            = errors.New("out of order exemplar")
	ErrDuplicateExemplar             = errors.New("duplicate exemplar")
	ErrExemplarLabelLength           = fmt.Errorf("label length for exemplar exceeds maximum of %d UTF-8 characters", exemplar.ExemplarMaxLabelSetLength)
	ErrExemplarsDisabled             = fmt.Errorf("exemplar storage is disabled or max exemplars is less than or equal to 0")
	ErrNativeHistogramsDisabled      = fmt.Errorf("native histograms are disabled")
	ErrHistogramCountNotBigEnough    = errors.New("histogram's observation count should be at least the number of observations found in the buckets")
	ErrHistogramNegativeBucketCount  = errors.New("histogram has a bucket whose observation count is negative")
	ErrHistogramSpanNegativeOffset   = errors.New("histogram has a span whose offset is negative")
	ErrHistogramSpansBucketsMismatch = errors.New("histogram spans specify different number of buckets than provided")
)

// SeriesRef is a generic series reference. In prometheus it is either a
// HeadSeriesRef or BlockSeriesRef, though other implementations may have
// their own reference types.
type SeriesRef uint64

// Appendable allows creating appenders.
type Appendable interface {
	// Appender returns a new appender for the storage. The implementation
	// can choose whether or not to use the context, for deadlines or to check
	// for errors.
	Appender(ctx context.Context) Appender
}

// SampleAndChunkQueryable allows retrieving samples as well as encoded samples in form of chunks.
type SampleAndChunkQueryable interface {
	Queryable
	ChunkQueryable
}

// Storage ingests and manages samples, along with various indexes. All methods
// are goroutine-safe. Storage implements storage.Appender.
type Storage interface {
	SampleAndChunkQueryable
	Appendable

	// StartTime returns the oldest timestamp stored in the storage.
	StartTime() (int64, error)

	// Close closes the storage and all its underlying resources.
	Close() error
}

// ExemplarStorage ingests and manages exemplars, along with various indexes. All methods are
// goroutine-safe. ExemplarStorage implements storage.ExemplarAppender and storage.ExemplarQuerier.
type ExemplarStorage interface {
	ExemplarQueryable
	ExemplarAppender
}

// A Queryable handles queries against a storage.
// Use it when you need to have access to all samples without chunk encoding abstraction e.g promQL.
type Queryable interface {
	// Querier returns a new Querier on the storage.
	Querier(ctx context.Context, mint, maxt int64) (Querier, error)
}

// A MockQueryable is used for testing purposes so that a mock Querier can be used.
type MockQueryable struct {
	MockQuerier Querier
}

func (q *MockQueryable) Querier(context.Context, int64, int64) (Querier, error) {
	return q.MockQuerier, nil
}

// Querier provides querying access over time series data of a fixed time range.
type Querier interface {
	LabelQuerier

	// Select returns a set of series that matches the given label matchers.
	// Caller can specify if it requires returned series to be sorted. Prefer not requiring sorting for better performance.
	// It allows passing hints that can help in optimising select, but it's up to implementation how this is used if used at all.
	Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet
}

// MockQuerier is used for test purposes to mock the selected series that is returned.
type MockQuerier struct {
	SelectMockFunction func(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet
}

func (q *MockQuerier) LabelValues(string, ...*labels.Matcher) ([]string, Warnings, error) {
	return nil, nil, nil
}

func (q *MockQuerier) LabelNames(...*labels.Matcher) ([]string, Warnings, error) {
	return nil, nil, nil
}

func (q *MockQuerier) Close() error {
	return nil
}

func (q *MockQuerier) Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	return q.SelectMockFunction(sortSeries, hints, matchers...)
}

// A ChunkQueryable handles queries against a storage.
// Use it when you need to have access to samples in encoded format.
type ChunkQueryable interface {
	// ChunkQuerier returns a new ChunkQuerier on the storage.
	ChunkQuerier(ctx context.Context, mint, maxt int64) (ChunkQuerier, error)
}

// ChunkQuerier provides querying access over time series data of a fixed time range.
type ChunkQuerier interface {
	LabelQuerier

	// Select returns a set of series that matches the given label matchers.
	// Caller can specify if it requires returned series to be sorted. Prefer not requiring sorting for better performance.
	// It allows passing hints that can help in optimising select, but it's up to implementation how this is used if used at all.
	Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) ChunkSeriesSet
}

// LabelQuerier provides querying access over labels.
type LabelQuerier interface {
	// LabelValues returns all potential values for a label name.
	// It is not safe to use the strings beyond the lifetime of the querier.
	// If matchers are specified the returned result set is reduced
	// to label values of metrics matching the matchers.
	LabelValues(name string, matchers ...*labels.Matcher) ([]string, Warnings, error)

	// LabelNames returns all the unique label names present in the block in sorted order.
	// If matchers are specified the returned result set is reduced
	// to label names of metrics matching the matchers.
	LabelNames(matchers ...*labels.Matcher) ([]string, Warnings, error)

	// Close releases the resources of the Querier.
	Close() error
}

type ExemplarQueryable interface {
	// ExemplarQuerier returns a new ExemplarQuerier on the storage.
	ExemplarQuerier(ctx context.Context) (ExemplarQuerier, error)
}

// ExemplarQuerier provides reading access to time series data.
type ExemplarQuerier interface {
	// Select all the exemplars that match the matchers.
	// Within a single slice of matchers, it is an intersection. Between the slices, it is a union.
	Select(start, end int64, matchers ...[]*labels.Matcher) ([]exemplar.QueryResult, error)
}

// SelectHints specifies hints passed for data selections.
// This is used only as an option for implementation to use.
type SelectHints struct {
	Start int64 // Start time in milliseconds for this select.
	End   int64 // End time in milliseconds for this select.

	Step int64  // Query step size in milliseconds.
	Func string // String representation of surrounding function or aggregation.

	Grouping []string // List of label names used in aggregation.
	By       bool     // Indicate whether it is without or by.
	Range    int64    // Range vector selector range in milliseconds.

	ShardIndex uint64 // Current shard index (starts from 0 and up to ShardCount-1).
	ShardCount uint64 // Total number of shards (0 means sharding is disabled).

	// DisableTrimming allows to disable trimming of matching series chunks based on query Start and End time.
	// When disabled, the result may contain samples outside the queried time range but Select() performances
	// may be improved.
	DisableTrimming bool
}

// TODO(bwplotka): Move to promql/engine_test.go?
// QueryableFunc is an adapter to allow the use of ordinary functions as
// Queryables. It follows the idea of http.HandlerFunc.
type QueryableFunc func(ctx context.Context, mint, maxt int64) (Querier, error)

// Querier calls f() with the given parameters.
func (f QueryableFunc) Querier(ctx context.Context, mint, maxt int64) (Querier, error) {
	return f(ctx, mint, maxt)
}

// Appender provides batched appends against a storage.
// It must be completed with a call to Commit or Rollback and must not be reused afterwards.
//
// Operations on the Appender interface are not goroutine-safe.
//
// The type of samples (float64, histogram, etc) appended for a given series must remain same within an Appender.
// The behaviour is undefined if samples of different types are appended to the same series in a single Commit().
type Appender interface {
	// Append adds a sample pair for the given series.
	// An optional series reference can be provided to accelerate calls.
	// A series reference number is returned which can be used to add further
	// samples to the given series in the same or later transactions.
	// Returned reference numbers are ephemeral and may be rejected in calls
	// to Append() at any point. Adding the sample via Append() returns a new
	// reference number.
	// If the reference is 0 it must not be used for caching.
	Append(ref SeriesRef, l labels.Labels, t int64, v float64) (SeriesRef, error)

	// Commit submits the collected samples and purges the batch. If Commit
	// returns a non-nil error, it also rolls back all modifications made in
	// the appender so far, as Rollback would do. In any case, an Appender
	// must not be used anymore after Commit has been called.
	Commit() error

	// Rollback rolls back all modifications made in the appender so far.
	// Appender has to be discarded after rollback.
	Rollback() error

	ExemplarAppender
	HistogramAppender
	MetadataUpdater
}

// GetRef is an extra interface on Appenders used by downstream projects
// (e.g. Cortex) to avoid maintaining a parallel set of references.
type GetRef interface {
	// Returns reference number that can be used to pass to Appender.Append(),
	// and a set of labels that will not cause another copy when passed to Appender.Append().
	// 0 means the appender does not have a reference to this series.
	// hash should be a hash of lset.
	GetRef(lset labels.Labels, hash uint64) (SeriesRef, labels.Labels)
}

// ExemplarAppender provides an interface for adding samples to exemplar storage, which
// within Prometheus is in-memory only.
type ExemplarAppender interface {
	// AppendExemplar adds an exemplar for the given series labels.
	// An optional reference number can be provided to accelerate calls.
	// A reference number is returned which can be used to add further
	// exemplars in the same or later transactions.
	// Returned reference numbers are ephemeral and may be rejected in calls
	// to Append() at any point. Adding the sample via Append() returns a new
	// reference number.
	// If the reference is 0 it must not be used for caching.
	// Note that in our current implementation of Prometheus' exemplar storage
	// calls to Append should generate the reference numbers, AppendExemplar
	// generating a new reference number should be considered possible erroneous behaviour and be logged.
	AppendExemplar(ref SeriesRef, l labels.Labels, e exemplar.Exemplar) (SeriesRef, error)
}

// HistogramAppender provides an interface for appending histograms to the storage.
type HistogramAppender interface {
	// AppendHistogram adds a histogram for the given series labels. An
	// optional reference number can be provided to accelerate calls. A
	// reference number is returned which can be used to add further
	// histograms in the same or later transactions. Returned reference
	// numbers are ephemeral and may be rejected in calls to Append() at any
	// point. Adding the sample via Append() returns a new reference number.
	// If the reference is 0, it must not be used for caching.
	//
	// For efficiency reasons, the histogram is passed as a
	// pointer. AppendHistogram won't mutate the histogram, but in turn
	// depends on the caller to not mutate it either.
	AppendHistogram(ref SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error)
}

// MetadataUpdater provides an interface for associating metadata to stored series.
type MetadataUpdater interface {
	// UpdateMetadata updates a metadata entry for the given series and labels.
	// A series reference number is returned which can be used to modify the
	// metadata of the given series in the same or later transactions.
	// Returned reference numbers are ephemeral and may be rejected in calls
	// to UpdateMetadata() at any point. If the series does not exist,
	// UpdateMetadata returns an error.
	// If the reference is 0 it must not be used for caching.
	UpdateMetadata(ref SeriesRef, l labels.Labels, m metadata.Metadata) (SeriesRef, error)
}

// SeriesSet contains a set of series.
type SeriesSet interface {
	Next() bool
	// At returns full series. Returned series should be iterable even after Next is called.
	At() Series
	// The error that iteration as failed with.
	// When an error occurs, set cannot continue to iterate.
	Err() error
	// A collection of warnings for the whole set.
	// Warnings could be return even iteration has not failed with error.
	Warnings() Warnings
}

var emptySeriesSet = errSeriesSet{}

// EmptySeriesSet returns a series set that's always empty.
func EmptySeriesSet() SeriesSet {
	return emptySeriesSet
}

type testSeriesSet struct {
	series Series
}

func (s testSeriesSet) Next() bool         { return true }
func (s testSeriesSet) At() Series         { return s.series }
func (s testSeriesSet) Err() error         { return nil }
func (s testSeriesSet) Warnings() Warnings { return nil }

// TestSeriesSet returns a mock series set
func TestSeriesSet(series Series) SeriesSet {
	return testSeriesSet{series: series}
}

type errSeriesSet struct {
	err error
}

func (s errSeriesSet) Next() bool         { return false }
func (s errSeriesSet) At() Series         { return nil }
func (s errSeriesSet) Err() error         { return s.err }
func (s errSeriesSet) Warnings() Warnings { return nil }

// ErrSeriesSet returns a series set that wraps an error.
func ErrSeriesSet(err error) SeriesSet {
	return errSeriesSet{err: err}
}

var emptyChunkSeriesSet = errChunkSeriesSet{}

// EmptyChunkSeriesSet returns a chunk series set that's always empty.
func EmptyChunkSeriesSet() ChunkSeriesSet {
	return emptyChunkSeriesSet
}

type errChunkSeriesSet struct {
	err error
}

func (s errChunkSeriesSet) Next() bool         { return false }
func (s errChunkSeriesSet) At() ChunkSeries    { return nil }
func (s errChunkSeriesSet) Err() error         { return s.err }
func (s errChunkSeriesSet) Warnings() Warnings { return nil }

// ErrChunkSeriesSet returns a chunk series set that wraps an error.
func ErrChunkSeriesSet(err error) ChunkSeriesSet {
	return errChunkSeriesSet{err: err}
}

// Series exposes a single time series and allows iterating over samples.
type Series interface {
	Labels
	SampleIterable
}

type mockSeries struct {
	timestamps []int64
	values     []float64
	labelSet   []string
}

func (s mockSeries) Labels() labels.Labels {
	return labels.FromStrings(s.labelSet...)
}

func (s mockSeries) Iterator(chunkenc.Iterator) chunkenc.Iterator {
	return chunkenc.MockSeriesIterator(s.timestamps, s.values)
}

// MockSeries returns a series with custom timestamps, values and labelSet.
func MockSeries(timestamps []int64, values []float64, labelSet []string) Series {
	return mockSeries{
		timestamps: timestamps,
		values:     values,
		labelSet:   labelSet,
	}
}

// ChunkSeriesSet contains a set of chunked series.
type ChunkSeriesSet interface {
	Next() bool
	// At returns full chunk series. Returned series should be iterable even after Next is called.
	At() ChunkSeries
	// The error that iteration has failed with.
	// When an error occurs, set cannot continue to iterate.
	Err() error
	// A collection of warnings for the whole set.
	// Warnings could be return even iteration has not failed with error.
	Warnings() Warnings
}

// ChunkSeries exposes a single time series and allows iterating over chunks.
type ChunkSeries interface {
	Labels
	ChunkIterable

	// EstimatedChunkCount returns an estimate of the number of chunks available from this ChunkSeries.
	//
	// This estimate is used by Mimir's ingesters to report the number of chunks expected to be returned by a query,
	// which is used by queriers to enforce the 'max chunks per query' limit.
	EstimatedChunkCount() int
}

// Labels represents an item that has labels e.g. time series.
type Labels interface {
	// Labels returns the complete set of labels. For series it means all labels identifying the series.
	Labels() labels.Labels
}

type SampleIterable interface {
	// Iterator returns an iterator of the data of the series.
	// The iterator passed as argument is for re-use, if not nil.
	// Depending on implementation, the iterator can
	// be re-used or a new iterator can be allocated.
	Iterator(chunkenc.Iterator) chunkenc.Iterator
}

type ChunkIterable interface {
	// Iterator returns an iterator that iterates over potentially overlapping
	// chunks of the series, sorted by min time.
	Iterator(chunks.Iterator) chunks.Iterator
}

type Warnings []error
