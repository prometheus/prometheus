// Copyright 2017 The Prometheus Authors
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
	"bytes"
	"container/heap"
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

type fanout struct {
	logger log.Logger

	primary     Storage
	secondaries []Storage
}

// NewFanout returns a new fanout Storage, which proxies reads and writes
// through to multiple underlying storages.
//
// The difference between primary and secondary Storage is only for read (Querier) path and it goes as follows:
// * If the primary querier returns an error, then any of the Querier operations will fail.
// * If any secondary querier returns an error the result from that queries is discarded. The overall operation will succeed,
// and the error from the secondary querier will be returned as a warning.
//
// NOTE: In the case of Prometheus, it treats all remote storages as secondary / best effort.
func NewFanout(logger log.Logger, primary Storage, secondaries ...Storage) Storage {
	return &fanout{
		logger:      logger,
		primary:     primary,
		secondaries: secondaries,
	}
}

// StartTime implements the Storage interface.
func (f *fanout) StartTime() (int64, error) {
	// StartTime of a fanout should be the earliest StartTime of all its storages,
	// both primary and secondaries.
	firstTime, err := f.primary.StartTime()
	if err != nil {
		return int64(model.Latest), err
	}

	for _, s := range f.secondaries {
		t, err := s.StartTime()
		if err != nil {
			return int64(model.Latest), err
		}
		if t < firstTime {
			firstTime = t
		}
	}
	return firstTime, nil
}

func (f *fanout) Querier(ctx context.Context, mint, maxt int64) (Querier, error) {
	primary, err := f.primary.Querier(ctx, mint, maxt)
	if err != nil {
		return nil, err
	}

	secondaries := make([]Querier, 0, len(f.secondaries))
	for _, storage := range f.secondaries {
		querier, err := storage.Querier(ctx, mint, maxt)
		if err != nil {
			// Close already open Queriers, append potential errors to returned error.
			errs := tsdb_errors.MultiError{err}
			errs.Add(primary.Close())
			for _, q := range secondaries {
				errs.Add(q.Close())
			}
			return nil, errs.Err()
		}
		secondaries = append(secondaries, querier)
	}
	return NewMergeQuerier(primary, secondaries, ChainedSeriesMerge), nil
}

func (f *fanout) ChunkQuerier(ctx context.Context, mint, maxt int64) (ChunkQuerier, error) {
	primary, err := f.primary.ChunkQuerier(ctx, mint, maxt)
	if err != nil {
		return nil, err
	}

	secondaries := make([]ChunkQuerier, 0, len(f.secondaries))
	for _, storage := range f.secondaries {
		querier, err := storage.ChunkQuerier(ctx, mint, maxt)
		if err != nil {
			// Close already open Queriers, append potential errors to returned error.
			errs := tsdb_errors.MultiError{err}
			errs.Add(primary.Close())
			for _, q := range secondaries {
				errs.Add(q.Close())
			}
			return nil, errs.Err()
		}
		secondaries = append(secondaries, querier)
	}
	return NewMergeChunkQuerier(primary, secondaries, NewCompactingChunkSeriesMerger(ChainedSeriesMerge)), nil
}

func (f *fanout) Appender() Appender {
	primary := f.primary.Appender()
	secondaries := make([]Appender, 0, len(f.secondaries))
	for _, storage := range f.secondaries {
		secondaries = append(secondaries, storage.Appender())
	}
	return &fanoutAppender{
		logger:      f.logger,
		primary:     primary,
		secondaries: secondaries,
	}
}

// Close closes the storage and all its underlying resources.
func (f *fanout) Close() error {
	errs := tsdb_errors.MultiError{}
	errs.Add(f.primary.Close())
	for _, s := range f.secondaries {
		errs.Add(s.Close())
	}
	return errs.Err()
}

// fanoutAppender implements Appender.
type fanoutAppender struct {
	logger log.Logger

	primary     Appender
	secondaries []Appender
}

func (f *fanoutAppender) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	ref, err := f.primary.Add(l, t, v)
	if err != nil {
		return ref, err
	}

	for _, appender := range f.secondaries {
		if _, err := appender.Add(l, t, v); err != nil {
			return 0, err
		}
	}
	return ref, nil
}

func (f *fanoutAppender) AddFast(ref uint64, t int64, v float64) error {
	if err := f.primary.AddFast(ref, t, v); err != nil {
		return err
	}

	for _, appender := range f.secondaries {
		if err := appender.AddFast(ref, t, v); err != nil {
			return err
		}
	}
	return nil
}

func (f *fanoutAppender) Commit() (err error) {
	err = f.primary.Commit()

	for _, appender := range f.secondaries {
		if err == nil {
			err = appender.Commit()
		} else {
			if rollbackErr := appender.Rollback(); rollbackErr != nil {
				level.Error(f.logger).Log("msg", "Squashed rollback error on commit", "err", rollbackErr)
			}
		}
	}
	return
}

func (f *fanoutAppender) Rollback() (err error) {
	err = f.primary.Rollback()

	for _, appender := range f.secondaries {
		rollbackErr := appender.Rollback()
		if err == nil {
			err = rollbackErr
		} else if rollbackErr != nil {
			level.Error(f.logger).Log("msg", "Squashed rollback error on rollback", "err", rollbackErr)
		}
	}
	return nil
}

type mergeGenericQuerier struct {
	queriers []genericQuerier

	// mergeFn is used when we see series from different queriers Selects with the same labels.
	mergeFn genericSeriesMergeFunc
}

// NewMergeQuerier returns a new Querier that merges results of given primary and slice of secondary queriers.
// See NewFanout commentary to learn more about primary vs secondary differences.
//
// In case of overlaps between the data given by primary + secondaries Selects, merge function will be used.
func NewMergeQuerier(primary Querier, secondaries []Querier, mergeFn VerticalSeriesMergeFunc) Querier {
	queriers := make([]genericQuerier, 0, len(secondaries)+1)
	if primary != nil {
		queriers = append(queriers, newGenericQuerierFrom(primary))
	}
	for _, querier := range secondaries {
		if _, ok := querier.(noopQuerier); !ok && querier != nil {
			queriers = append(queriers, newSecondaryQuerierFrom(querier))
		}
	}

	return &querierAdapter{&mergeGenericQuerier{
		mergeFn:  (&seriesMergerAdapter{VerticalSeriesMergeFunc: mergeFn}).Merge,
		queriers: queriers,
	}}
}

// NewMergeChunkQuerier returns a new ChunkQuerier that merges results of given primary and slice of secondary chunk queriers.
// See NewFanout commentary to learn more about primary vs secondary differences.
//
// In case of overlaps between the data given by primary + secondaries Selects, merge function will be used.
// TODO(bwplotka): Currently merge will compact overlapping chunks with bigger chunk, without limit. Split it: https://github.com/prometheus/tsdb/issues/670
func NewMergeChunkQuerier(primary ChunkQuerier, secondaries []ChunkQuerier, mergeFn VerticalChunkSeriesMergeFunc) ChunkQuerier {
	queriers := make([]genericQuerier, 0, len(secondaries)+1)
	if primary != nil {
		queriers = append(queriers, newGenericQuerierFromChunk(primary))
	}
	for _, querier := range secondaries {
		if _, ok := querier.(noopChunkQuerier); !ok && querier != nil {
			queriers = append(queriers, newSecondaryQuerierFromChunk(querier))
		}
	}

	return &chunkQuerierAdapter{&mergeGenericQuerier{
		mergeFn:  (&chunkSeriesMergerAdapter{VerticalChunkSeriesMergeFunc: mergeFn}).Merge,
		queriers: queriers,
	}}
}

// Select returns a set of series that matches the given label matchers.
func (q *mergeGenericQuerier) Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) genericSeriesSet {
	if len(q.queriers) == 1 {
		return q.queriers[0].Select(sortSeries, hints, matchers...)
	}

	var (
		seriesSets    = make([]genericSeriesSet, 0, len(q.queriers))
		wg            sync.WaitGroup
		seriesSetChan = make(chan genericSeriesSet)
	)

	// Schedule all Selects for all queriers we know about.
	for _, querier := range q.queriers {
		wg.Add(1)
		go func(qr genericQuerier) {
			defer wg.Done()

			// We need to sort for NewMergeSeriesSet to work.
			seriesSetChan <- qr.Select(true, hints, matchers...)
		}(querier)
	}
	go func() {
		wg.Wait()
		close(seriesSetChan)
	}()

	for r := range seriesSetChan {
		seriesSets = append(seriesSets, r)
	}
	return &lazySeriesSet{create: create(seriesSets, q.mergeFn)}
}

func create(seriesSets []genericSeriesSet, mergeFunc genericSeriesMergeFunc) func() (genericSeriesSet, bool) {
	// Returned function gets called with the first call to Next().
	return func() (genericSeriesSet, bool) {
		if len(seriesSets) == 1 {
			return seriesSets[0], seriesSets[0].Next()
		}
		var h genericSeriesSetHeap
		for _, set := range seriesSets {
			if set == nil {
				continue
			}
			if set.Next() {
				heap.Push(&h, set)
				continue
			}
			// When primary fails ignore results from secondaries.
			// Only the primary querier returns error.
			if err := set.Err(); err != nil {
				return errorOnlySeriesSet{err}, false
			}
		}
		set := &genericMergeSeriesSet{
			mergeFunc: mergeFunc,
			sets:      seriesSets,
			heap:      h,
		}
		return set, set.Next()
	}
}

// LabelValues returns all potential values for a label name.
func (q *mergeGenericQuerier) LabelValues(name string) ([]string, Warnings, error) {
	var (
		results  [][]string
		warnings Warnings
	)
	for _, querier := range q.queriers {
		values, wrn, err := querier.LabelValues(name)
		if wrn != nil {
			// TODO(bwplotka): We could potentially wrap warnings.
			warnings = append(warnings, wrn...)
		}
		if err != nil {
			return nil, nil, errors.Wrapf(err, "LabelValues() from Querier for label %s", name)
		}
		results = append(results, values)
	}
	return mergeStringSlices(results), warnings, nil
}

func mergeStringSlices(ss [][]string) []string {
	switch len(ss) {
	case 0:
		return nil
	case 1:
		return ss[0]
	case 2:
		return mergeTwoStringSlices(ss[0], ss[1])
	default:
		halfway := len(ss) / 2
		return mergeTwoStringSlices(
			mergeStringSlices(ss[:halfway]),
			mergeStringSlices(ss[halfway:]),
		)
	}
}

func mergeTwoStringSlices(a, b []string) []string {
	i, j := 0, 0
	result := make([]string, 0, len(a)+len(b))
	for i < len(a) && j < len(b) {
		switch strings.Compare(a[i], b[j]) {
		case 0:
			result = append(result, a[i])
			i++
			j++
		case -1:
			result = append(result, a[i])
			i++
		case 1:
			result = append(result, b[j])
			j++
		}
	}
	result = append(result, a[i:]...)
	result = append(result, b[j:]...)
	return result
}

// LabelNames returns all the unique label names present in the block in sorted order.
func (q *mergeGenericQuerier) LabelNames() ([]string, Warnings, error) {
	var (
		labelNamesMap = make(map[string]struct{})
		warnings      Warnings
	)
	for _, querier := range q.queriers {
		names, wrn, err := querier.LabelNames()
		if wrn != nil {
			// TODO(bwplotka): We could potentially wrap warnings.
			warnings = append(warnings, wrn...)
		}
		if err != nil {
			return nil, nil, errors.Wrap(err, "LabelNames() from Querier")
		}
		for _, name := range names {
			labelNamesMap[name] = struct{}{}
		}
	}
	if len(labelNamesMap) == 0 {
		return nil, warnings, nil
	}

	labelNames := make([]string, 0, len(labelNamesMap))
	for name := range labelNamesMap {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)
	return labelNames, warnings, nil
}

// Close releases the resources of the Querier.
func (q *mergeGenericQuerier) Close() error {
	errs := tsdb_errors.MultiError{}
	for _, querier := range q.queriers {
		if err := querier.Close(); err != nil {
			errs.Add(err)
		}
	}
	return errs.Err()
}

// VerticalSeriesMergeFunc returns merged series implementation that merges series with same labels together.
// It has to handle time-overlapped series as well.
type VerticalSeriesMergeFunc func(...Series) Series

// NewMergeSeriesSet returns a new SeriesSet that merges many SeriesSets together.
func NewMergeSeriesSet(sets []SeriesSet, mergeFunc VerticalSeriesMergeFunc) SeriesSet {
	genericSets := make([]genericSeriesSet, 0, len(sets))
	for _, s := range sets {
		genericSets = append(genericSets, &genericSeriesSetAdapter{s})

	}
	return &seriesSetAdapter{newGenericMergeSeriesSet(genericSets, (&seriesMergerAdapter{VerticalSeriesMergeFunc: mergeFunc}).Merge)}
}

// VerticalChunkSeriesMergeFunc returns merged chunk series implementation that merges potentially time-overlapping
// chunk series with the same labels into single ChunkSeries.
//
// NOTE: It's up to implementation how series are vertically merged (if chunks are sorted, re-encoded etc).
type VerticalChunkSeriesMergeFunc func(...ChunkSeries) ChunkSeries

// NewMergeChunkSeriesSet returns a new ChunkSeriesSet that merges many SeriesSet together.
func NewMergeChunkSeriesSet(sets []ChunkSeriesSet, mergeFunc VerticalChunkSeriesMergeFunc) ChunkSeriesSet {
	genericSets := make([]genericSeriesSet, 0, len(sets))
	for _, s := range sets {
		genericSets = append(genericSets, &genericChunkSeriesSetAdapter{s})

	}
	return &chunkSeriesSetAdapter{newGenericMergeSeriesSet(genericSets, (&chunkSeriesMergerAdapter{VerticalChunkSeriesMergeFunc: mergeFunc}).Merge)}
}

// genericMergeSeriesSet implements genericSeriesSet.
type genericMergeSeriesSet struct {
	currentLabels labels.Labels
	mergeFunc     genericSeriesMergeFunc

	heap        genericSeriesSetHeap
	sets        []genericSeriesSet
	currentSets []genericSeriesSet
}

// newGenericMergeSeriesSet returns a new genericSeriesSet that merges (and deduplicates)
// series returned by the series sets when iterating.
// Each series set must return its series in labels order, otherwise
// merged series set will be incorrect.
// Overlapped situations are merged using provided mergeFunc.
func newGenericMergeSeriesSet(sets []genericSeriesSet, mergeFunc genericSeriesMergeFunc) genericSeriesSet {
	if len(sets) == 1 {
		return sets[0]
	}

	// We are pre-advancing sets, so we can introspect the label of the
	// series under the cursor.
	var h genericSeriesSetHeap
	for _, set := range sets {
		if set == nil {
			continue
		}
		if set.Next() {
			heap.Push(&h, set)
		}
	}
	return &genericMergeSeriesSet{
		mergeFunc: mergeFunc,
		sets:      sets,
		heap:      h,
	}
}

func (c *genericMergeSeriesSet) Next() bool {
	// Run in a loop because the "next" series sets may not be valid anymore.
	// If, for the current label set, all the next series sets come from
	// failed remote storage sources, we want to keep trying with the next label set.
	for {
		// Firstly advance all the current series sets. If any of them have run out
		// we can drop them, otherwise they should be inserted back into the heap.
		for _, set := range c.currentSets {
			if set.Next() {
				heap.Push(&c.heap, set)
			}
		}

		if len(c.heap) == 0 {
			return false
		}

		// Now, pop items of the heap that have equal label sets.
		c.currentSets = nil
		c.currentLabels = c.heap[0].At().Labels()
		for len(c.heap) > 0 && labels.Equal(c.currentLabels, c.heap[0].At().Labels()) {
			set := heap.Pop(&c.heap).(genericSeriesSet)
			c.currentSets = append(c.currentSets, set)
		}

		// As long as the current set contains at least 1 set,
		// then it should return true.
		if len(c.currentSets) != 0 {
			break
		}
	}
	return true
}

func (c *genericMergeSeriesSet) At() Labels {
	if len(c.currentSets) == 1 {
		return c.currentSets[0].At()
	}
	series := make([]Labels, 0, len(c.currentSets))
	for _, seriesSet := range c.currentSets {
		series = append(series, seriesSet.At())
	}
	return c.mergeFunc(series...)
}

func (c *genericMergeSeriesSet) Err() error {
	for _, set := range c.sets {
		if err := set.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (c *genericMergeSeriesSet) Warnings() Warnings {
	var ws Warnings
	for _, set := range c.sets {
		ws = append(ws, set.Warnings()...)
	}
	return ws
}

type genericSeriesSetHeap []genericSeriesSet

func (h genericSeriesSetHeap) Len() int      { return len(h) }
func (h genericSeriesSetHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h genericSeriesSetHeap) Less(i, j int) bool {
	a, b := h[i].At().Labels(), h[j].At().Labels()
	return labels.Compare(a, b) < 0
}

func (h *genericSeriesSetHeap) Push(x interface{}) {
	*h = append(*h, x.(genericSeriesSet))
}

func (h *genericSeriesSetHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// ChainedSeriesMerge returns single series from many same, potentially overlapping series by chaining samples together.
// If one or more samples overlap, one sample from random overlapped ones is kept and all others with the same
// timestamp are dropped.
//
// This works the best with replicated series, where data from two series are exactly the same. This does not work well
// with "almost" the same data, e.g. from 2 Prometheus HA replicas. This is fine, since from the Prometheus perspective
// this never happens.
//
// NOTE: Use this only when you see potentially overlapping series, as this introduces small overhead to handle overlaps
// between series.
func ChainedSeriesMerge(s ...Series) Series {
	if len(s) == 0 {
		return nil
	}
	return &chainSeries{
		labels: s[0].Labels(),
		series: s,
	}
}

type chainSeries struct {
	labels labels.Labels
	series []Series
}

func (m *chainSeries) Labels() labels.Labels {
	return m.labels
}

func (m *chainSeries) Iterator() chunkenc.Iterator {
	iterators := make([]chunkenc.Iterator, 0, len(m.series))
	for _, s := range m.series {
		iterators = append(iterators, s.Iterator())
	}
	return newChainSampleIterator(iterators)
}

// chainSampleIterator is responsible to iterate over samples from different iterators of the same time series in timestamps
// order. If one or more samples overlap, one sample from random overlapped ones is kept and all others with the same
// timestamp are dropped.
type chainSampleIterator struct {
	iterators []chunkenc.Iterator
	h         samplesIteratorHeap
}

func newChainSampleIterator(iterators []chunkenc.Iterator) chunkenc.Iterator {
	return &chainSampleIterator{
		iterators: iterators,
		h:         nil,
	}
}

func (c *chainSampleIterator) Seek(t int64) bool {
	c.h = samplesIteratorHeap{}
	for _, iter := range c.iterators {
		if iter.Seek(t) {
			heap.Push(&c.h, iter)
		}
	}
	return len(c.h) > 0
}

func (c *chainSampleIterator) At() (t int64, v float64) {
	if len(c.h) == 0 {
		panic("chainSampleIterator.At() called after .Next() returned false.")
	}

	return c.h[0].At()
}

func (c *chainSampleIterator) Next() bool {
	if c.h == nil {
		for _, iter := range c.iterators {
			if iter.Next() {
				heap.Push(&c.h, iter)
			}
		}

		return len(c.h) > 0
	}

	if len(c.h) == 0 {
		return false
	}

	currt, _ := c.At()
	for len(c.h) > 0 {
		nextt, _ := c.h[0].At()
		// All but one of the overlapping samples will be dropped.
		if nextt != currt {
			break
		}

		iter := heap.Pop(&c.h).(chunkenc.Iterator)
		if iter.Next() {
			heap.Push(&c.h, iter)
		}
	}

	return len(c.h) > 0
}

func (c *chainSampleIterator) Err() error {
	var errs tsdb_errors.MultiError
	for _, iter := range c.iterators {
		if err := iter.Err(); err != nil {
			errs.Add(err)
		}
	}
	return errs.Err()
}

type samplesIteratorHeap []chunkenc.Iterator

func (h samplesIteratorHeap) Len() int      { return len(h) }
func (h samplesIteratorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h samplesIteratorHeap) Less(i, j int) bool {
	at, _ := h[i].At()
	bt, _ := h[j].At()
	return at < bt
}

func (h *samplesIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(chunkenc.Iterator))
}

func (h *samplesIteratorHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type compactChunkSeriesMerger struct {
	mergeFunc VerticalSeriesMergeFunc

	labels labels.Labels
	series []ChunkSeries
}

// NewCompactingChunkSeriesMerger returns VerticalChunkSeriesMergeFunc that merges the same chunk series into single chunk series.
// In case of the chunk overlaps, it compacts those into one or more time-ordered non-overlapping chunks with merged data.
// Samples from overlapped chunks are merged using series vertical merge func.
// It expects the same labels for each given series.
//
// NOTE: Use this only when you see potentially overlapping series, as this introduces small overhead to handle overlaps
// between series.
func NewCompactingChunkSeriesMerger(mergeFunc VerticalSeriesMergeFunc) VerticalChunkSeriesMergeFunc {
	return func(s ...ChunkSeries) ChunkSeries {
		if len(s) == 0 {
			return nil
		}
		return &compactChunkSeriesMerger{
			mergeFunc: mergeFunc,
			labels:    s[0].Labels(),
			series:    s,
		}
	}
}

func (s *compactChunkSeriesMerger) Labels() labels.Labels {
	return s.labels
}

func (s *compactChunkSeriesMerger) Iterator() chunks.Iterator {
	iterators := make([]chunks.Iterator, 0, len(s.series))
	for _, series := range s.series {
		iterators = append(iterators, series.Iterator())
	}
	return &compactChunkIterator{
		mergeFunc: s.mergeFunc,
		labels:    s.labels,
		iterators: iterators,
	}
}

// compactChunkIterator is responsible to compact chunks from different iterators of the same time series into single chainSeries.
// If time-overlapping chunks are found, they are encoded and passed to series merge and encoded again into one bigger chunk.
// TODO(bwplotka): Currently merge will compact overlapping chunks with bigger chunk, without limit. Split it: https://github.com/prometheus/tsdb/issues/670
type compactChunkIterator struct {
	mergeFunc VerticalSeriesMergeFunc

	labels    labels.Labels
	iterators []chunks.Iterator

	h chunkIteratorHeap
}

func (c *compactChunkIterator) At() chunks.Meta {
	if len(c.h) == 0 {
		panic("compactChunkIterator.At() called after .Next() returned false.")
	}

	return c.h[0].At()
}

func (c *compactChunkIterator) Next() bool {
	if c.h == nil {
		for _, iter := range c.iterators {
			if iter.Next() {
				heap.Push(&c.h, iter)
			}
		}
		return len(c.h) > 0
	}

	if len(c.h) == 0 {
		return false
	}

	// Detect overlaps to compact.
	// Be smart about it and deduplicate on the fly if chunks are identical.
	last := c.At()
	var overlapped []Series
	for {
		iter := heap.Pop(&c.h).(chunks.Iterator)
		if iter.Next() {
			heap.Push(&c.h, iter)
		}
		if len(c.h) == 0 {
			break
		}

		// Get the current oldest chunk by min, then max time.
		next := c.At()
		if next.MinTime > last.MaxTime {
			// No overlap with last one.
			break
		}

		if next.MinTime == last.MinTime &&
			next.MaxTime == last.MaxTime &&
			bytes.Equal(next.Chunk.Bytes(), last.Chunk.Bytes()) {
			// 1:1 duplicates, skip last.
			continue
		}

		overlapped = append(overlapped, &chunkToSeriesDecoder{
			labels: c.labels,
			Meta:   last,
		})
		last = next
	}

	if len(overlapped) == 0 {
		return len(c.h) > 0
	}

	// Add last, not yet included overlap.
	overlapped = append(overlapped, &chunkToSeriesDecoder{
		labels: c.labels,
		Meta:   c.At(),
	})

	var chkSeries ChunkSeries = &seriesToChunkEncoder{Series: c.mergeFunc(overlapped...)}
	heap.Push(&c.h, chkSeries)
	return true
}

func (c *compactChunkIterator) Err() error {
	var errs tsdb_errors.MultiError
	for _, iter := range c.iterators {
		if err := iter.Err(); err != nil {
			errs.Add(err)
		}
	}
	return errs.Err()
}

type chunkIteratorHeap []chunks.Iterator

func (h chunkIteratorHeap) Len() int      { return len(h) }
func (h chunkIteratorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h chunkIteratorHeap) Less(i, j int) bool {
	at := h[i].At()
	bt := h[j].At()
	if at.MinTime == bt.MinTime {
		return at.MaxTime < bt.MaxTime
	}
	return at.MinTime < bt.MinTime
}

func (h *chunkIteratorHeap) Push(x interface{}) {
	*h = append(*h, x.(chunks.Iterator))
}

func (h *chunkIteratorHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
