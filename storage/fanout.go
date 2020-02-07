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
	"container/heap"
	"context"
	"math"
	"sort"
	"strings"

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

// NewFanout returns a new fan-out Storage, which proxies reads and writes
// through to multiple underlying storages.
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
	// both primaryQuerier and secondaries.
	firstTime, err := f.primary.StartTime()
	if err != nil {
		return int64(model.Latest), err
	}

	for _, storage := range f.secondaries {
		t, err := storage.StartTime()
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
	queriers := make([]Querier, 0, 1+len(f.secondaries))

	// Add primaryQuerier querier
	primaryQuerier, err := f.primary.Querier(ctx, mint, maxt)
	if err != nil {
		return nil, err
	}
	queriers = append(queriers, primaryQuerier)

	// Add secondary queriers
	for _, storage := range f.secondaries {
		querier, err := storage.Querier(ctx, mint, maxt)
		if err != nil {
			NewMergeQuerier(primaryQuerier, queriers, SeriesMergerFunc(ChainedSeriesMerge)).Close()
			return nil, err
		}
		queriers = append(queriers, querier)
	}

	return NewMergeQuerier(primaryQuerier, queriers, SeriesMergerFunc(ChainedSeriesMerge)), nil
}

func (f *fanout) ChunkedQuerier(ctx context.Context, mint, maxt int64) (ChunkedQuerier, error) {
	queriers := make([]ChunkedQuerier, 0, 1+len(f.secondaries))

	// Add primaryQuerier querier.
	primaryQuerier, err := f.primary.ChunkedQuerier(ctx, mint, maxt)
	if err != nil {
		return nil, err
	}
	queriers = append(queriers, primaryQuerier)

	// Add secondary queriers.
	for _, storage := range f.secondaries {
		querier, err := storage.ChunkedQuerier(ctx, mint, maxt)
		if err != nil {
			NewMergeChunkedQuerier(primaryQuerier, queriers, ChunkedSeriesMergerFunc(ChainedChunkedSeriesMerge)).Close()
			return nil, err
		}
		queriers = append(queriers, querier)
	}

	return NewMergeChunkedQuerier(primaryQuerier, queriers, ChunkedSeriesMergerFunc(ChainedChunkedSeriesMerge)), nil
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
	if err := f.primary.Close(); err != nil {
		return err
	}

	// TODO return multiple errors?
	var lastErr error
	for _, storage := range f.secondaries {
		if err := storage.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
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
	merger genericSeriesMerger

	primaryQuerier genericQuerier
	queriers       []genericQuerier
	failedQueriers map[genericQuerier]struct{}
	setQuerierMap  map[genericSeriesSet]genericQuerier
}

// NewMergeQuerier returns a new Querier that merges results of input queriers.
// NB NewMergeQuerier will return NoopQuerier if no queriers are passed to it,
// and will filter NoopQueriers from its arguments, in order to reduce overhead
// when only one querier is passed.
func NewMergeQuerier(primaryQuerier Querier, queriers []Querier, merger SeriesMerger) Querier {
	filtered := make([]genericQuerier, 0, len(queriers))
	for _, querier := range queriers {
		if querier != NoopQuerier() {
			filtered = append(filtered, newGenericQuerierFrom(querier))
		}
	}

	// There was bug previously.
	if len(filtered) == 0 {
		return primaryQuerier
	}

	return &querierAdapter{&mergeGenericQuerier{
		merger:         newGenericSeriesMergerFrom(merger),
		primaryQuerier: newGenericQuerierFrom(primaryQuerier),
		queriers:       filtered,
		failedQueriers: make(map[genericQuerier]struct{}),
		setQuerierMap:  make(map[genericSeriesSet]genericQuerier),
	}}
}

// NewMergeChunkedQuerier returns a new Querier that merges results of input queriers.
// NB NewMergeQuerier will return NoopQuerier if no queriers are passed to it,
// and will filter NoopQueriers from its arguments, in order to reduce overhead
// when only one querier is passed.
func NewMergeChunkedQuerier(primaryQuerier ChunkedQuerier, queriers []ChunkedQuerier, merger ChunkedSeriesMerger) ChunkedQuerier {
	filtered := make([]genericQuerier, 0, len(queriers))
	for _, querier := range queriers {
		if querier != NoopChunkedQuerier() {
			filtered = append(filtered, newGenericQuerierFromChunked(querier))
		}
	}

	// There was bug previously.
	if len(filtered) == 0 {
		return primaryQuerier
	}

	return &chunkedQuerierAdapter{&mergeGenericQuerier{
		merger:         newGenericSeriesMergerFromChunked(merger),
		primaryQuerier: newGenericQuerierFromChunked(primaryQuerier),
		queriers:       filtered,
		failedQueriers: make(map[genericQuerier]struct{}),
		setQuerierMap:  make(map[genericSeriesSet]genericQuerier),
	}}
}

// Select returns a set of series that matches the given label matchers.
func (q *mergeGenericQuerier) Select(params *SelectParams, matchers ...*labels.Matcher) (genericSeriesSet, Warnings, error) {
	if len(q.queriers) != 1 {
		// We need to sort for NewChainedMergeSeriesSet to work.
		return q.SelectSorted(params, matchers...)
	}
	return q.queriers[0].Select(params, matchers...)
}

// SelectSorted returns a set of sorted series that matches the given label matchers.
func (q *mergeGenericQuerier) SelectSorted(params *SelectParams, matchers ...*labels.Matcher) (genericSeriesSet, Warnings, error) {
	seriesSets := make([]genericSeriesSet, 0, len(q.queriers))
	var warnings Warnings

	var priErr error = nil
	type queryResult struct {
		qr          genericQuerier
		set         genericSeriesSet
		wrn         Warnings
		selectError error
	}
	queryResultChan := make(chan *queryResult)
	for _, querier := range q.queriers {
		go func(qr genericQuerier) {
			set, wrn, err := qr.SelectSorted(params, matchers...)
			queryResultChan <- &queryResult{qr: qr, set: set, wrn: wrn, selectError: err}
		}(querier)
	}
	for i := 0; i < len(q.queriers); i++ {
		qryResult := <-queryResultChan
		q.setQuerierMap[qryResult.set] = qryResult.qr
		if qryResult.wrn != nil {
			warnings = append(warnings, qryResult.wrn...)
		}
		if qryResult.selectError != nil {
			q.failedQueriers[qryResult.qr] = struct{}{}
			// If the error source isn't the primary querier, return the error as a warning and continue.
			if qryResult.qr != q.primaryQuerier {
				warnings = append(warnings, qryResult.selectError)
			} else {
				priErr = qryResult.selectError
			}
		}
		seriesSets = append(seriesSets, qryResult.set)
	}
	if priErr != nil {
		return nil, nil, priErr
	}
	return newGenericMergeSeriesSet(seriesSets, q, q.merger), warnings, nil
}

// LabelValues returns all potential values for a label name.
func (q *mergeGenericQuerier) LabelValues(name string) ([]string, Warnings, error) {
	var results [][]string
	var warnings Warnings
	for _, querier := range q.queriers {
		values, wrn, err := querier.LabelValues(name)
		if wrn != nil {
			warnings = append(warnings, wrn...)
		}
		if err != nil {
			q.failedQueriers[querier] = struct{}{}
			// If the error source isn't the primary querier, return the error as a warning and continue.
			if querier != q.primaryQuerier {
				warnings = append(warnings, err)
				continue
			} else {
				return nil, nil, err
			}
		}
		results = append(results, values)
	}
	return mergeStringSlices(results), warnings, nil
}

func (q *mergeGenericQuerier) IsFailedSet(set genericSeriesSet) bool {
	_, isFailedQuerier := q.failedQueriers[q.setQuerierMap[set]]
	return isFailedQuerier
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
	labelNamesMap := make(map[string]struct{})
	var warnings Warnings
	for _, querier := range q.queriers {
		names, wrn, err := querier.LabelNames()
		if wrn != nil {
			warnings = append(warnings, wrn...)
		}

		if err != nil {
			q.failedQueriers[querier] = struct{}{}
			// If the error source isn't the primaryQuerier querier, return the error as a warning and continue.
			if querier != q.primaryQuerier {
				warnings = append(warnings, err)
				continue
			} else {
				return nil, nil, errors.Wrap(err, "LabelNames() from Querier")
			}
		}

		for _, name := range names {
			labelNamesMap[name] = struct{}{}
		}
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
	var errs tsdb_errors.MultiError
	for _, querier := range q.queriers {
		if err := querier.Close(); err != nil {
			errs.Add(err)
		}
	}
	return errs.Err()
}

// genericMergeSeriesSet implements genericSeriesSet
type genericMergeSeriesSet struct {
	currentLabels labels.Labels
	merger        genericSeriesMerger

	heap seriesSetHeap
	sets []genericSeriesSet

	currentSets []genericSeriesSet
	querier     *mergeGenericQuerier
}

// NewGenericMergeSeriesSet returns a new series set that merges (deduplicates)
// series returned by the input series sets when iterating.
// Each input series set must return its series in labels order, otherwise
// merged series set will be incorrect.
// Overlapped samples/chunks will be dropped.
func newGenericMergeSeriesSet(sets []genericSeriesSet, querier *mergeGenericQuerier, merger genericSeriesMerger) genericSeriesSet {
	if len(sets) == 1 {
		return sets[0]
	}

	// Sets need to be pre-advanced, so we can introspect the label of the
	// series under the cursor.
	var h seriesSetHeap
	for _, set := range sets {
		if set == nil {
			continue
		}
		if set.Next() {
			heap.Push(&h, set)
		}
	}
	return &genericMergeSeriesSet{
		merger:  merger,
		heap:    h,
		sets:    sets,
		querier: querier,
	}
}

type SeriesMerger interface {
	// Merge returns merged series implementation that merges series with same labels together.
	// It has to handle time-overlapped series as well.
	Merge(...Series) Series
}

type SeriesMergerFunc func(...Series) Series

func (f SeriesMergerFunc) Merge(s ...Series) Series {
	return (f)(s...)
}

type ChunkedSeriesMerger interface {
	// Merge returns merged chunked series implementation that merges series with same labels together.
	// It has to handle time-overlapped series as well.
	Merge(...ChunkedSeries) ChunkedSeries
}

type ChunkedSeriesMergerFunc func(...ChunkedSeries) ChunkedSeries

func (f ChunkedSeriesMergerFunc) Merge(s ...ChunkedSeries) ChunkedSeries {
	return (f)(s...)
}

func (c *genericMergeSeriesSet) Next() bool {
	// Run in a loop because the "next" series sets may not be valid anymore.
	// If a remote querier fails, we discard all series sets from that querier.
	// If, for the current label set, all the next series sets come from
	// failed remote storage sources, we want to keep trying with the next label set.
	for {
		// Firstly advance all the current series sets.  If any of them have run out
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
			if c.querier != nil && c.querier.IsFailedSet(set) {
				continue
			}
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

func (c *genericMergeSeriesSet) At() Labeled {
	if len(c.currentSets) == 1 {
		return c.currentSets[0].At()
	}
	series := make([]Labeled, 0, len(c.currentSets))
	for _, seriesSet := range c.currentSets {
		series = append(series, seriesSet.At())
	}
	return c.merger.NewMergedSeries(series...)
}

func (c *genericMergeSeriesSet) Err() error {
	for _, set := range c.sets {
		if err := set.Err(); err != nil {
			return err
		}
	}
	return nil
}

// ChainedSeriesMerge returns single series from two by chaining series.
// In case of the overlap, first overlapped sample is kept, rest are dropped.
// We expect the same labels for each given series.
func ChainedSeriesMerge(s ...Series) Series {
	if len(s) == 0 {
		return nil
	}
	return &chainSeries{
		labels: s[0].Labels(),
		series: s,
	}
}

type seriesSetHeap []SeriesSet

func (h seriesSetHeap) Len() int      { return len(h) }
func (h seriesSetHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h seriesSetHeap) Less(i, j int) bool {
	a, b := h[i].At().Labels(), h[j].At().Labels()
	return labels.Compare(a, b) < 0
}

func (h *seriesSetHeap) Push(x interface{}) {
	*h = append(*h, x.(SeriesSet))
}

func (h *seriesSetHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
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

// sampleIterator is responsible to iterate over non-overlapping samples from different iterators of same time series.
// If the samples overlap, all but one will be dropped.
type sampleIterator struct {
	iterators []chunkenc.Iterator
	h         samplesIteratorHeap
}

func newChainSampleIterator(iterators []chunkenc.Iterator) chunkenc.Iterator {
	return &sampleIterator{
		iterators: iterators,
		h:         nil,
	}
}

func (c *sampleIterator) Seek(t int64) bool {
	c.h = samplesIteratorHeap{}
	for _, iter := range c.iterators {
		if iter.Seek(t) {
			heap.Push(&c.h, iter)
		}
	}
	return len(c.h) > 0
}

func (c *sampleIterator) At() (t int64, v float64) {
	if len(c.h) == 0 {
		return math.MaxInt64, 0
	}

	return c.h[0].At()
}

func (c *sampleIterator) Next() bool {
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

func (c *sampleIterator) Err() error {
	for _, iter := range c.iterators {
		if err := iter.Err(); err != nil {
			return err
		}
	}
	return nil
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

type chainChunkedSeries struct {
	labels labels.Labels
	series []ChunkedSeries
}

// ChainedChunkedSeriesMerge returns single series from two by chaining series together.
// In case of the chunk overlap, chunkMerger will decide.
// We expect the same labels for each given series.
func ChainedChunkedSeriesMerge(s ...ChunkedSeries) ChunkedSeries {
	if len(s) == 0 {
		return nil
	}
	return &chainChunkedSeries{
		labels: s[0].Labels(),
		series: s,
	}
}

func (s *chainChunkedSeries) Labels() labels.Labels {
	return s.labels
}

func (s *chainChunkedSeries) Iterator() chunks.Iterator {
	iterators := make([]chunks.Iterator, 0, len(s.series))
	for _, series := range s.series {
		iterators = append(iterators, series.Iterator())
	}
	return ChainedChunkMerge(iterators...)
}

// chainChunkIterator is responsible to iterate over chunks from different iterators of same time series.
type chainChunkIterator struct {
	iterators []chunks.Iterator
	h         chunkIteratorHeap
}

// ChainedChunkMerge will merge chunks by chaining them together. In case of overlap, they will be naively chained.
func ChainedChunkMerge(iterators ...chunks.Iterator) chunks.Iterator {
	return &chainChunkIterator{
		iterators: iterators,
		h:         nil,
	}
}

func (c *chainChunkIterator) At() chunks.Meta {
	if len(c.h) == 0 {
		return chunks.Meta{}
	}

	return c.h[0].At()
}

func (c *chainChunkIterator) Next() bool {
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

	// Overlapped chunks will be just chained.
	iter := heap.Pop(&c.h).(chunks.Iterator)
	if iter.Next() {
		heap.Push(&c.h, iter)
	}
	return len(c.h) > 0
}

func (c *chainChunkIterator) Err() error {
	for _, iter := range c.iterators {
		if err := iter.Err(); err != nil {
			return err
		}
	}
	return nil
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
