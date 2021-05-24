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

	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// The errors exposed.
var (
	ErrNotFound                    = errors.New("not found")
	ErrOutOfOrderSample            = errors.New("out of order sample")
	ErrDuplicateSampleForTimestamp = errors.New("duplicate sample for timestamp")
	ErrOutOfBounds                 = errors.New("out of bounds")
	ErrOutOfOrderExemplar          = errors.New("out of order exemplar")
	ErrDuplicateExemplar           = errors.New("duplicate exemplar")
	ErrExemplarLabelLength         = fmt.Errorf("label length for exemplar exceeds maximum of %d UTF-8 characters", exemplar.ExemplarMaxLabelSetLength)
)

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

// Querier provides querying access over time series data of a fixed time range.
type Querier interface {
	LabelQuerier

	// Select returns a set of series that matches the given label matchers.
	// Caller can specify if it requires returned series to be sorted. Prefer not requiring sorting for better performance.
	// It allows passing hints that can help in optimising select, but it's up to implementation how this is used if used at all.
	Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet
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
	// It is not safe to use the strings beyond the lifefime of the querier.
	// If matchers are specified the returned result set is reduced
	// to label values of metrics matching the matchers.
	LabelValues(name string, matchers ...*labels.Matcher) ([]string, Warnings, error)

	// LabelNames returns all the unique label names present in the block in sorted order.
	// TODO(yeya24): support matchers or hints.
	LabelNames() ([]string, Warnings, error)

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
type Appender interface {
	// Append adds a sample pair for the given series.
	// An optional reference number can be provided to accelerate calls.
	// A reference number is returned which can be used to add further
	// samples in the same or later transactions.
	// Returned reference numbers are ephemeral and may be rejected in calls
	// to Append() at any point. Adding the sample via Append() returns a new
	// reference number.
	// If the reference is 0 it must not be used for caching.
	Append(ref uint64, l labels.Labels, t int64, v float64) (uint64, error)

	// Commit submits the collected samples and purges the batch. If Commit
	// returns a non-nil error, it also rolls back all modifications made in
	// the appender so far, as Rollback would do. In any case, an Appender
	// must not be used anymore after Commit has been called.
	Commit() error

	// Rollback rolls back all modifications made in the appender so far.
	// Appender has to be discarded after rollback.
	Rollback() error

	ExemplarAppender
}

// GetRef is an extra interface on Appenders used by downstream projects
// (e.g. Cortex) to avoid maintaining a parallel set of references.
type GetRef interface {
	// Returns reference number that can be used to pass to Appender.Append(),
	// and a set of labels that will not cause another copy when passed to Appender.Append().
	// 0 means the appender does not have a reference to this series.
	GetRef(lset labels.Labels) (uint64, labels.Labels)
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
	AppendExemplar(ref uint64, l labels.Labels, e exemplar.Exemplar) (uint64, error)
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
}

// Labels represents an item that has labels e.g. time series.
type Labels interface {
	// Labels returns the complete set of labels. For series it means all labels identifying the series.
	Labels() labels.Labels
}

type SampleIterable interface {
	// Iterator returns a new, independent iterator of the data of the series.
	Iterator() chunkenc.Iterator
}

type ChunkIterable interface {
	// Iterator returns a new, independent iterator that iterates over potentially overlapping
	// chunks of the series, sorted by min time.
	Iterator() chunks.Iterator
}

type Warnings []error
