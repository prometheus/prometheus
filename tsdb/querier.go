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

package tsdb

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/oklog/ulid"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/util/annotations"
)

// checkContextEveryNIterations is used in some tight loops to check if the context is done.
const checkContextEveryNIterations = 100

type blockBaseQuerier struct {
	blockID    ulid.ULID
	index      IndexReader
	chunks     ChunkReader
	tombstones tombstones.Reader

	closed bool

	mint, maxt int64
}

func newBlockBaseQuerier(b BlockReader, mint, maxt int64) (*blockBaseQuerier, error) {
	indexr, err := b.Index()
	if err != nil {
		return nil, fmt.Errorf("open index reader: %w", err)
	}
	chunkr, err := b.Chunks()
	if err != nil {
		indexr.Close()
		return nil, fmt.Errorf("open chunk reader: %w", err)
	}
	tombsr, err := b.Tombstones()
	if err != nil {
		indexr.Close()
		chunkr.Close()
		return nil, fmt.Errorf("open tombstone reader: %w", err)
	}

	if tombsr == nil {
		tombsr = tombstones.NewMemTombstones()
	}
	return &blockBaseQuerier{
		blockID:    b.Meta().ULID,
		mint:       mint,
		maxt:       maxt,
		index:      indexr,
		chunks:     chunkr,
		tombstones: tombsr,
	}, nil
}

func (q *blockBaseQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	res, err := q.index.SortedLabelValues(ctx, name, matchers...)
	return res, nil, err
}

func (q *blockBaseQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	res, err := q.index.LabelNames(ctx, matchers...)
	return res, nil, err
}

func (q *blockBaseQuerier) Close() error {
	if q.closed {
		return errors.New("block querier already closed")
	}

	errs := tsdb_errors.NewMulti(
		q.index.Close(),
		q.chunks.Close(),
		q.tombstones.Close(),
	)
	q.closed = true
	return errs.Err()
}

type blockQuerier struct {
	*blockBaseQuerier
}

// NewBlockQuerier returns a querier against the block reader and requested min and max time range.
func NewBlockQuerier(b BlockReader, mint, maxt int64) (storage.Querier, error) {
	q, err := newBlockBaseQuerier(b, mint, maxt)
	if err != nil {
		return nil, err
	}
	return &blockQuerier{blockBaseQuerier: q}, nil
}

func (q *blockQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	return selectSeriesSet(ctx, sortSeries, hints, ms, q.index, q.chunks, q.tombstones, q.mint, q.maxt)
}

func selectSeriesSet(ctx context.Context, sortSeries bool, hints *storage.SelectHints, ms []*labels.Matcher,
	index IndexReader, chunks ChunkReader, tombstones tombstones.Reader, mint, maxt int64,
) storage.SeriesSet {
	disableTrimming := false
	sharded := hints != nil && hints.ShardCount > 0

	p, err := PostingsForMatchers(ctx, index, ms...)
	if err != nil {
		return storage.ErrSeriesSet(err)
	}
	if sharded {
		p = index.ShardedPostings(p, hints.ShardIndex, hints.ShardCount)
	}
	if sortSeries {
		p = index.SortedPostings(p)
	}

	if hints != nil {
		mint = hints.Start
		maxt = hints.End
		disableTrimming = hints.DisableTrimming
		if hints.Func == "series" {
			// When you're only looking up metadata (for example series API), you don't need to load any chunks.
			return newBlockSeriesSet(index, newNopChunkReader(), tombstones, p, mint, maxt, disableTrimming)
		}
	}

	return newBlockSeriesSet(index, chunks, tombstones, p, mint, maxt, disableTrimming)
}

// blockChunkQuerier provides chunk querying access to a single block database.
type blockChunkQuerier struct {
	*blockBaseQuerier
}

// NewBlockChunkQuerier returns a chunk querier against the block reader and requested min and max time range.
func NewBlockChunkQuerier(b BlockReader, mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := newBlockBaseQuerier(b, mint, maxt)
	if err != nil {
		return nil, err
	}
	return &blockChunkQuerier{blockBaseQuerier: q}, nil
}

func (q *blockChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.ChunkSeriesSet {
	return selectChunkSeriesSet(ctx, sortSeries, hints, ms, q.blockID, q.index, q.chunks, q.tombstones, q.mint, q.maxt)
}

func selectChunkSeriesSet(ctx context.Context, sortSeries bool, hints *storage.SelectHints, ms []*labels.Matcher,
	blockID ulid.ULID, index IndexReader, chunks ChunkReader, tombstones tombstones.Reader, mint, maxt int64,
) storage.ChunkSeriesSet {
	disableTrimming := false
	sharded := hints != nil && hints.ShardCount > 0

	if hints != nil {
		mint = hints.Start
		maxt = hints.End
		disableTrimming = hints.DisableTrimming
	}
	p, err := PostingsForMatchers(ctx, index, ms...)
	if err != nil {
		return storage.ErrChunkSeriesSet(err)
	}
	if sharded {
		p = index.ShardedPostings(p, hints.ShardIndex, hints.ShardCount)
	}
	if sortSeries {
		p = index.SortedPostings(p)
	}
	return NewBlockChunkSeriesSet(blockID, index, chunks, tombstones, p, mint, maxt, disableTrimming)
}

// PostingsForMatchers assembles a single postings iterator against the index reader
// based on the given matchers. The resulting postings are not ordered by series.
func PostingsForMatchers(ctx context.Context, ix IndexReader, ms ...*labels.Matcher) (index.Postings, error) {
	var its, notIts []index.Postings
	// See which label must be non-empty.
	// Optimization for case like {l=~".", l!="1"}.
	labelMustBeSet := make(map[string]bool, len(ms))
	for _, m := range ms {
		if !m.Matches("") {
			labelMustBeSet[m.Name] = true
		}
	}
	isSubtractingMatcher := func(m *labels.Matcher) bool {
		if !labelMustBeSet[m.Name] {
			return true
		}
		return (m.Type == labels.MatchNotEqual || m.Type == labels.MatchNotRegexp) && m.Matches("")
	}
	hasSubtractingMatchers, hasIntersectingMatchers := false, false
	for _, m := range ms {
		if isSubtractingMatcher(m) {
			hasSubtractingMatchers = true
		} else {
			hasIntersectingMatchers = true
		}
	}

	if hasSubtractingMatchers && !hasIntersectingMatchers {
		// If there's nothing to subtract from, add in everything and remove the notIts later.
		// We prefer to get AllPostings so that the base of subtraction (i.e. allPostings)
		// doesn't include series that may be added to the index reader during this function call.
		k, v := index.AllPostingsKey()
		allPostings, err := ix.Postings(ctx, k, v)
		if err != nil {
			return nil, err
		}
		its = append(its, allPostings)
	}

	// Sort matchers to have the intersecting matchers first.
	// This way the base for subtraction is smaller and
	// there is no chance that the set we subtract from
	// contains postings of series that didn't exist when
	// we constructed the set we subtract by.
	slices.SortStableFunc(ms, func(i, j *labels.Matcher) int {
		if !isSubtractingMatcher(i) && isSubtractingMatcher(j) {
			return -1
		}

		return +1
	})

	for _, m := range ms {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		switch {
		case m.Name == "" && m.Value == "": // Special-case for AllPostings, used in tests at least.
			k, v := index.AllPostingsKey()
			allPostings, err := ix.Postings(ctx, k, v)
			if err != nil {
				return nil, err
			}
			its = append(its, allPostings)
		case labelMustBeSet[m.Name]:
			// If this matcher must be non-empty, we can be smarter.
			matchesEmpty := m.Matches("")
			isNot := m.Type == labels.MatchNotEqual || m.Type == labels.MatchNotRegexp
			switch {
			case isNot && matchesEmpty: // l!="foo"
				// If the label can't be empty and is a Not and the inner matcher
				// doesn't match empty, then subtract it out at the end.
				inverse, err := m.Inverse()
				if err != nil {
					return nil, err
				}

				it, err := postingsForMatcher(ctx, ix, inverse)
				if err != nil {
					return nil, err
				}
				notIts = append(notIts, it)
			case isNot && !matchesEmpty: // l!=""
				// If the label can't be empty and is a Not, but the inner matcher can
				// be empty we need to use inversePostingsForMatcher.
				inverse, err := m.Inverse()
				if err != nil {
					return nil, err
				}

				it, err := inversePostingsForMatcher(ctx, ix, inverse)
				if err != nil {
					return nil, err
				}
				if index.IsEmptyPostingsType(it) {
					return index.EmptyPostings(), nil
				}
				its = append(its, it)
			default: // l="a"
				// Non-Not matcher, use normal postingsForMatcher.
				it, err := postingsForMatcher(ctx, ix, m)
				if err != nil {
					return nil, err
				}
				if index.IsEmptyPostingsType(it) {
					return index.EmptyPostings(), nil
				}
				its = append(its, it)
			}
		default: // l=""
			// If the matchers for a labelname selects an empty value, it selects all
			// the series which don't have the label name set too. See:
			// https://github.com/prometheus/prometheus/issues/3575 and
			// https://github.com/prometheus/prometheus/pull/3578#issuecomment-351653555
			it, err := inversePostingsForMatcher(ctx, ix, m)
			if err != nil {
				return nil, err
			}
			notIts = append(notIts, it)
		}
	}

	it := index.Intersect(its...)

	for _, n := range notIts {
		it = index.Without(it, n)
	}

	return it, nil
}

func postingsForMatcher(ctx context.Context, ix IndexReader, m *labels.Matcher) (index.Postings, error) {
	// This method will not return postings for missing labels.

	// Fast-path for equal matching.
	if m.Type == labels.MatchEqual {
		return ix.Postings(ctx, m.Name, m.Value)
	}

	// Fast-path for set matching.
	if m.Type == labels.MatchRegexp {
		setMatches := m.SetMatches()
		if len(setMatches) > 0 {
			return ix.Postings(ctx, m.Name, setMatches...)
		}
	}

	it := ix.PostingsForLabelMatching(ctx, m.Name, m.Matches)
	return it, it.Err()
}

// inversePostingsForMatcher returns the postings for the series with the label name set but not matching the matcher.
func inversePostingsForMatcher(ctx context.Context, ix IndexReader, m *labels.Matcher) (index.Postings, error) {
	// Fast-path for MatchNotRegexp matching.
	// Inverse of a MatchNotRegexp is MatchRegexp (double negation).
	// Fast-path for set matching.
	if m.Type == labels.MatchNotRegexp {
		setMatches := m.SetMatches()
		if len(setMatches) > 0 {
			return ix.Postings(ctx, m.Name, setMatches...)
		}
	}

	// Fast-path for MatchNotEqual matching.
	// Inverse of a MatchNotEqual is MatchEqual (double negation).
	if m.Type == labels.MatchNotEqual {
		return ix.Postings(ctx, m.Name, m.Value)
	}

	vals, err := ix.LabelValues(ctx, m.Name)
	if err != nil {
		return nil, err
	}

	res := vals[:0]
	// If the match before inversion was !="" or !~"", we just want all the values.
	if m.Value == "" && (m.Type == labels.MatchRegexp || m.Type == labels.MatchEqual) {
		res = vals
	} else {
		count := 1
		for _, val := range vals {
			if count%checkContextEveryNIterations == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			count++
			if !m.Matches(val) {
				res = append(res, val)
			}
		}
	}

	return ix.Postings(ctx, m.Name, res...)
}

func labelValuesWithMatchers(ctx context.Context, r IndexReader, name string, matchers ...*labels.Matcher) ([]string, error) {
	allValues, err := r.LabelValues(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("fetching values of label %s: %w", name, err)
	}

	// If we have a matcher for the label name, we can filter out values that don't match
	// before we fetch postings. This is especially useful for labels with many values.
	// e.g. __name__ with a selector like {__name__="xyz"}
	hasMatchersForOtherLabels := false
	for _, m := range matchers {
		if m.Name != name {
			hasMatchersForOtherLabels = true
			continue
		}

		// re-use the allValues slice to avoid allocations
		// this is safe because the iteration is always ahead of the append
		filteredValues := allValues[:0]
		count := 1
		for _, v := range allValues {
			if count%checkContextEveryNIterations == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			count++
			if m.Matches(v) {
				filteredValues = append(filteredValues, v)
			}
		}
		allValues = filteredValues
	}

	if len(allValues) == 0 {
		return nil, nil
	}

	// If we don't have any matchers for other labels, then we're done.
	if !hasMatchersForOtherLabels {
		return allValues, nil
	}

	p, err := PostingsForMatchers(ctx, r, matchers...)
	if err != nil {
		return nil, fmt.Errorf("fetching postings for matchers: %w", err)
	}

	valuesPostings := make([]index.Postings, len(allValues))
	for i, value := range allValues {
		valuesPostings[i], err = r.Postings(ctx, name, value)
		if err != nil {
			return nil, fmt.Errorf("fetching postings for %s=%q: %w", name, value, err)
		}
	}
	indexes, err := index.FindIntersectingPostings(p, valuesPostings)
	if err != nil {
		return nil, fmt.Errorf("intersecting postings: %w", err)
	}

	values := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		values = append(values, allValues[idx])
	}

	return values, nil
}

func labelNamesWithMatchers(ctx context.Context, r IndexReader, matchers ...*labels.Matcher) ([]string, error) {
	p, err := PostingsForMatchers(ctx, r, matchers...)
	if err != nil {
		return nil, err
	}
	return r.LabelNamesFor(ctx, p)
}

// seriesData, used inside other iterators, are updated when we move from one series to another.
type seriesData struct {
	chks      []chunks.Meta
	intervals tombstones.Intervals
	labels    labels.Labels
}

// Labels implements part of storage.Series and storage.ChunkSeries.
func (s *seriesData) Labels() labels.Labels { return s.labels }

// blockBaseSeriesSet allows to iterate over all series in the single block.
// Iterated series are trimmed with given min and max time as well as tombstones.
// See newBlockSeriesSet and NewBlockChunkSeriesSet to use it for either sample or chunk iterating.
type blockBaseSeriesSet struct {
	blockID         ulid.ULID
	p               index.Postings
	index           IndexReader
	chunks          ChunkReader
	tombstones      tombstones.Reader
	mint, maxt      int64
	disableTrimming bool

	curr seriesData

	bufChks []chunks.Meta
	builder labels.ScratchBuilder
	err     error
}

func (b *blockBaseSeriesSet) Next() bool {
	for b.p.Next() {
		if err := b.index.Series(b.p.At(), &b.builder, &b.bufChks); err != nil {
			// Postings may be stale. Skip if no underlying series exists.
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			b.err = fmt.Errorf("get series %d: %w", b.p.At(), err)
			return false
		}

		if len(b.bufChks) == 0 {
			continue
		}

		intervals, err := b.tombstones.Get(b.p.At())
		if err != nil {
			b.err = fmt.Errorf("get tombstones: %w", err)
			return false
		}

		// NOTE:
		// * block time range is half-open: [meta.MinTime, meta.MaxTime).
		// * chunks are both closed: [chk.MinTime, chk.MaxTime].
		// * requested time ranges are closed: [req.Start, req.End].

		var trimFront, trimBack bool

		// Copy chunks as iterables are reusable.
		// Count those in range to size allocation (roughly - ignoring tombstones).
		nChks := 0
		for _, chk := range b.bufChks {
			if !(chk.MaxTime < b.mint || chk.MinTime > b.maxt) {
				nChks++
			}
		}
		chks := make([]chunks.Meta, 0, nChks)

		// Prefilter chunks and pick those which are not entirely deleted or totally outside of the requested range.
		for _, chk := range b.bufChks {
			if chk.MaxTime < b.mint {
				continue
			}
			if chk.MinTime > b.maxt {
				continue
			}
			if (tombstones.Interval{Mint: chk.MinTime, Maxt: chk.MaxTime}.IsSubrange(intervals)) {
				continue
			}
			chks = append(chks, chk)

			// If still not entirely deleted, check if trim is needed based on requested time range.
			if !b.disableTrimming {
				if chk.MinTime < b.mint {
					trimFront = true
				}
				if chk.MaxTime > b.maxt {
					trimBack = true
				}
			}
		}

		if len(chks) == 0 {
			continue
		}

		if trimFront {
			intervals = intervals.Add(tombstones.Interval{Mint: math.MinInt64, Maxt: b.mint - 1})
		}
		if trimBack {
			intervals = intervals.Add(tombstones.Interval{Mint: b.maxt + 1, Maxt: math.MaxInt64})
		}

		b.curr.labels = b.builder.Labels()
		b.curr.chks = chks
		b.curr.intervals = intervals
		return true
	}
	return false
}

func (b *blockBaseSeriesSet) Err() error {
	if b.err != nil {
		return b.err
	}
	return b.p.Err()
}

func (b *blockBaseSeriesSet) Warnings() annotations.Annotations { return nil }

// populateWithDelGenericSeriesIterator allows to iterate over given chunk
// metas. In each iteration it ensures that chunks are trimmed based on given
// tombstones interval if any.
//
// populateWithDelGenericSeriesIterator assumes that chunks that would be fully
// removed by intervals are filtered out in previous phase.
//
// On each iteration currMeta is available. If currDelIter is not nil, it
// means that the chunk in currMeta is invalid and a chunk rewrite is needed,
// for which currDelIter should be used.
type populateWithDelGenericSeriesIterator struct {
	blockID ulid.ULID
	cr      ChunkReader
	// metas are expected to be sorted by minTime and should be related to
	// the same, single series.
	// It's possible for a single chunks.Meta to refer to multiple chunks.
	// cr.ChunkOrIterator() would return an iterable and a nil chunk in this
	// case.
	metas []chunks.Meta

	i         int // Index into metas; -1 if not started yet.
	err       error
	bufIter   DeletedIterator // Retained for memory re-use. currDelIter may point here.
	intervals tombstones.Intervals

	currDelIter chunkenc.Iterator
	// currMeta is the current chunks.Meta from metas. currMeta.Chunk is set to
	// the chunk returned from cr.ChunkOrIterable(). As that can return a nil
	// chunk, currMeta.Chunk is not always guaranteed to be set.
	currMeta chunks.Meta
}

func (p *populateWithDelGenericSeriesIterator) reset(blockID ulid.ULID, cr ChunkReader, chks []chunks.Meta, intervals tombstones.Intervals) {
	p.blockID = blockID
	p.cr = cr
	p.metas = chks
	p.i = -1
	p.err = nil
	// Note we don't touch p.bufIter.Iter; it is holding on to an iterator we might reuse in next().
	p.bufIter.Intervals = p.bufIter.Intervals[:0]
	p.intervals = intervals
	p.currDelIter = nil
	p.currMeta = chunks.Meta{}
}

// If copyHeadChunk is true, then the head chunk (i.e. the in-memory chunk of the TSDB)
// is deep copied to avoid races between reads and copying chunk bytes.
// However, if the deletion intervals overlaps with the head chunk, then the head chunk is
// not copied irrespective of copyHeadChunk because it will be re-encoded later anyway.
func (p *populateWithDelGenericSeriesIterator) next(copyHeadChunk bool) bool {
	if p.err != nil || p.i >= len(p.metas)-1 {
		return false
	}

	p.i++
	p.currMeta = p.metas[p.i]

	p.bufIter.Intervals = p.bufIter.Intervals[:0]
	for _, interval := range p.intervals {
		if p.currMeta.OverlapsClosedInterval(interval.Mint, interval.Maxt) {
			p.bufIter.Intervals = p.bufIter.Intervals.Add(interval)
		}
	}

	hcr, ok := p.cr.(ChunkReaderWithCopy)
	var iterable chunkenc.Iterable
	if ok && copyHeadChunk && len(p.bufIter.Intervals) == 0 {
		// ChunkOrIterableWithCopy will copy the head chunk, if it can.
		var maxt int64
		p.currMeta.Chunk, iterable, maxt, p.err = hcr.ChunkOrIterableWithCopy(p.currMeta)
		if p.currMeta.Chunk != nil {
			// For the in-memory head chunk the index reader sets maxt as MaxInt64. We fix it here.
			p.currMeta.MaxTime = maxt
		}
	} else {
		p.currMeta.Chunk, iterable, p.err = p.cr.ChunkOrIterable(p.currMeta)
	}

	if p.err != nil {
		p.err = fmt.Errorf("cannot populate chunk %d from block %s: %w", p.currMeta.Ref, p.blockID.String(), p.err)
		return false
	}

	// Use the single chunk if possible.
	if p.currMeta.Chunk != nil {
		if len(p.bufIter.Intervals) == 0 {
			// If there is no overlap with deletion intervals and a single chunk is
			// returned, we can take chunk as it is.
			p.currDelIter = nil
			return true
		}
		// Otherwise we need to iterate over the samples in the single chunk
		// and create new chunks.
		p.bufIter.Iter = p.currMeta.Chunk.Iterator(p.bufIter.Iter)
		p.currDelIter = &p.bufIter
		return true
	}

	// Otherwise, use the iterable to create an iterator.
	p.bufIter.Iter = iterable.Iterator(p.bufIter.Iter)
	p.currDelIter = &p.bufIter
	return true
}

func (p *populateWithDelGenericSeriesIterator) Err() error { return p.err }

type blockSeriesEntry struct {
	chunks  ChunkReader
	blockID ulid.ULID
	seriesData
}

func (s *blockSeriesEntry) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	pi, ok := it.(*populateWithDelSeriesIterator)
	if !ok {
		pi = &populateWithDelSeriesIterator{}
	}
	pi.reset(s.blockID, s.chunks, s.chks, s.intervals)
	return pi
}

type chunkSeriesEntry struct {
	chunks  ChunkReader
	blockID ulid.ULID
	seriesData
}

func (s *chunkSeriesEntry) Iterator(it chunks.Iterator) chunks.Iterator {
	pi, ok := it.(*populateWithDelChunkSeriesIterator)
	if !ok {
		pi = &populateWithDelChunkSeriesIterator{}
	}
	pi.reset(s.blockID, s.chunks, s.chks, s.intervals)
	return pi
}

// populateWithDelSeriesIterator allows to iterate over samples for the single series.
type populateWithDelSeriesIterator struct {
	populateWithDelGenericSeriesIterator

	curr chunkenc.Iterator
}

func (p *populateWithDelSeriesIterator) reset(blockID ulid.ULID, cr ChunkReader, chks []chunks.Meta, intervals tombstones.Intervals) {
	p.populateWithDelGenericSeriesIterator.reset(blockID, cr, chks, intervals)
	p.curr = nil
}

func (p *populateWithDelSeriesIterator) Next() chunkenc.ValueType {
	if p.curr != nil {
		if valueType := p.curr.Next(); valueType != chunkenc.ValNone {
			return valueType
		}
	}

	for p.next(false) {
		if p.currDelIter != nil {
			p.curr = p.currDelIter
		} else {
			p.curr = p.currMeta.Chunk.Iterator(p.curr)
		}
		if valueType := p.curr.Next(); valueType != chunkenc.ValNone {
			return valueType
		}
	}
	return chunkenc.ValNone
}

func (p *populateWithDelSeriesIterator) Seek(t int64) chunkenc.ValueType {
	if p.curr != nil {
		if valueType := p.curr.Seek(t); valueType != chunkenc.ValNone {
			return valueType
		}
	}
	for p.Next() != chunkenc.ValNone {
		if valueType := p.curr.Seek(t); valueType != chunkenc.ValNone {
			return valueType
		}
	}
	return chunkenc.ValNone
}

func (p *populateWithDelSeriesIterator) At() (int64, float64) {
	return p.curr.At()
}

func (p *populateWithDelSeriesIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return p.curr.AtHistogram(h)
}

func (p *populateWithDelSeriesIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return p.curr.AtFloatHistogram(fh)
}

func (p *populateWithDelSeriesIterator) AtT() int64 {
	return p.curr.AtT()
}

func (p *populateWithDelSeriesIterator) Err() error {
	if err := p.populateWithDelGenericSeriesIterator.Err(); err != nil {
		return err
	}
	if p.curr != nil {
		return p.curr.Err()
	}
	return nil
}

type populateWithDelChunkSeriesIterator struct {
	populateWithDelGenericSeriesIterator

	// currMetaWithChunk is current meta with its chunk field set. This meta
	// is guaranteed to map to a single chunk. This differs from
	// populateWithDelGenericSeriesIterator.currMeta as that
	// could refer to multiple chunks.
	currMetaWithChunk chunks.Meta

	// chunksFromIterable stores the chunks created from iterating through
	// the iterable returned by cr.ChunkOrIterable() (with deleted samples
	// removed).
	chunksFromIterable    []chunks.Meta
	chunksFromIterableIdx int
}

func (p *populateWithDelChunkSeriesIterator) reset(blockID ulid.ULID, cr ChunkReader, chks []chunks.Meta, intervals tombstones.Intervals) {
	p.populateWithDelGenericSeriesIterator.reset(blockID, cr, chks, intervals)
	p.currMetaWithChunk = chunks.Meta{}
	p.chunksFromIterable = p.chunksFromIterable[:0]
	p.chunksFromIterableIdx = -1
}

func (p *populateWithDelChunkSeriesIterator) Next() bool {
	if p.currMeta.Chunk == nil {
		// If we've been creating chunks from the iterable, check if there are
		// any more chunks to iterate through.
		if p.chunksFromIterableIdx < len(p.chunksFromIterable)-1 {
			p.chunksFromIterableIdx++
			p.currMetaWithChunk = p.chunksFromIterable[p.chunksFromIterableIdx]
			return true
		}
	}

	// Move to the next chunk/deletion iterator.
	// This is a for loop as if the current p.currDelIter returns no samples
	// (which means a chunk won't be created), there still might be more
	// samples/chunks from the rest of p.metas.
	for p.next(true) {
		if p.currDelIter == nil {
			p.currMetaWithChunk = p.currMeta
			return true
		}

		if p.currMeta.Chunk != nil {
			// If ChunkOrIterable() returned a non-nil chunk, the samples in
			// p.currDelIter will only form one chunk, as the only change
			// p.currDelIter might make is deleting some samples.
			if p.populateCurrForSingleChunk() {
				return true
			}
		} else {
			// If ChunkOrIterable() returned an iterable, multiple chunks may be
			// created from the samples in p.currDelIter.
			if p.populateChunksFromIterable() {
				return true
			}
		}
	}
	return false
}

// populateCurrForSingleChunk sets the fields within p.currMetaWithChunk. This
// should be called if the samples in p.currDelIter only form one chunk.
func (p *populateWithDelChunkSeriesIterator) populateCurrForSingleChunk() bool {
	valueType := p.currDelIter.Next()
	if valueType == chunkenc.ValNone {
		if err := p.currDelIter.Err(); err != nil {
			p.err = fmt.Errorf("iterate chunk while re-encoding: %w", err)
		}
		return false
	}
	p.currMetaWithChunk.MinTime = p.currDelIter.AtT()

	// Re-encode the chunk if iterator is provided. This means that it has
	// some samples to be deleted or chunk is opened.
	var (
		newChunk chunkenc.Chunk
		app      chunkenc.Appender
		t        int64
		err      error
	)
	switch valueType {
	case chunkenc.ValHistogram:
		newChunk = chunkenc.NewHistogramChunk()
		if app, err = newChunk.Appender(); err != nil {
			break
		}
		for vt := valueType; vt != chunkenc.ValNone; vt = p.currDelIter.Next() {
			if vt != chunkenc.ValHistogram {
				err = fmt.Errorf("found value type %v in histogram chunk", vt)
				break
			}
			var h *histogram.Histogram
			t, h = p.currDelIter.AtHistogram(nil)
			_, _, app, err = app.AppendHistogram(nil, t, h, true)
			if err != nil {
				break
			}
		}
	case chunkenc.ValFloat:
		newChunk = chunkenc.NewXORChunk()
		if app, err = newChunk.Appender(); err != nil {
			break
		}
		for vt := valueType; vt != chunkenc.ValNone; vt = p.currDelIter.Next() {
			if vt != chunkenc.ValFloat {
				err = fmt.Errorf("found value type %v in float chunk", vt)
				break
			}
			var v float64
			t, v = p.currDelIter.At()
			app.Append(t, v)
		}
	case chunkenc.ValFloatHistogram:
		newChunk = chunkenc.NewFloatHistogramChunk()
		if app, err = newChunk.Appender(); err != nil {
			break
		}
		for vt := valueType; vt != chunkenc.ValNone; vt = p.currDelIter.Next() {
			if vt != chunkenc.ValFloatHistogram {
				err = fmt.Errorf("found value type %v in histogram chunk", vt)
				break
			}
			var h *histogram.FloatHistogram
			t, h = p.currDelIter.AtFloatHistogram(nil)
			_, _, app, err = app.AppendFloatHistogram(nil, t, h, true)
			if err != nil {
				break
			}
		}
	default:
		err = fmt.Errorf("populateCurrForSingleChunk: value type %v unsupported", valueType)
	}

	if err != nil {
		p.err = fmt.Errorf("iterate chunk while re-encoding: %w", err)
		return false
	}
	if err := p.currDelIter.Err(); err != nil {
		p.err = fmt.Errorf("iterate chunk while re-encoding: %w", err)
		return false
	}

	p.currMetaWithChunk.Chunk = newChunk
	p.currMetaWithChunk.MaxTime = t
	return true
}

// populateChunksFromIterable reads the samples from currDelIter to create
// chunks for chunksFromIterable. It also sets p.currMetaWithChunk to the first
// chunk.
func (p *populateWithDelChunkSeriesIterator) populateChunksFromIterable() bool {
	p.chunksFromIterable = p.chunksFromIterable[:0]
	p.chunksFromIterableIdx = -1

	firstValueType := p.currDelIter.Next()
	if firstValueType == chunkenc.ValNone {
		if err := p.currDelIter.Err(); err != nil {
			p.err = fmt.Errorf("populateChunksFromIterable: no samples could be read: %w", err)
			return false
		}
		return false
	}

	var (
		// t is the timestamp for the current sample.
		t     int64
		cmint int64
		cmaxt int64

		currentChunk chunkenc.Chunk

		app chunkenc.Appender

		newChunk chunkenc.Chunk
		recoded  bool

		err error
	)

	prevValueType := chunkenc.ValNone

	for currentValueType := firstValueType; currentValueType != chunkenc.ValNone; currentValueType = p.currDelIter.Next() {
		// Check if the encoding has changed (i.e. we need to create a new
		// chunk as chunks can't have multiple encoding types).
		// For the first sample, the following condition will always be true as
		// ValNone != ValFloat | ValHistogram | ValFloatHistogram.
		if currentValueType != prevValueType {
			if prevValueType != chunkenc.ValNone {
				p.chunksFromIterable = append(p.chunksFromIterable, chunks.Meta{Chunk: currentChunk, MinTime: cmint, MaxTime: cmaxt})
			}
			cmint = p.currDelIter.AtT()
			if currentChunk, err = currentValueType.NewChunk(); err != nil {
				break
			}
			if app, err = currentChunk.Appender(); err != nil {
				break
			}
		}

		switch currentValueType {
		case chunkenc.ValFloat:
			{
				var v float64
				t, v = p.currDelIter.At()
				app.Append(t, v)
			}
		case chunkenc.ValHistogram:
			{
				var v *histogram.Histogram
				t, v = p.currDelIter.AtHistogram(nil)
				// No need to set prevApp as AppendHistogram will set the
				// counter reset header for the appender that's returned.
				newChunk, recoded, app, err = app.AppendHistogram(nil, t, v, false)
			}
		case chunkenc.ValFloatHistogram:
			{
				var v *histogram.FloatHistogram
				t, v = p.currDelIter.AtFloatHistogram(nil)
				// No need to set prevApp as AppendHistogram will set the
				// counter reset header for the appender that's returned.
				newChunk, recoded, app, err = app.AppendFloatHistogram(nil, t, v, false)
			}
		}

		if err != nil {
			break
		}

		if newChunk != nil {
			if !recoded {
				p.chunksFromIterable = append(p.chunksFromIterable, chunks.Meta{Chunk: currentChunk, MinTime: cmint, MaxTime: cmaxt})
			}
			currentChunk = newChunk
			cmint = t
		}

		cmaxt = t
		prevValueType = currentValueType
	}

	if err != nil {
		p.err = fmt.Errorf("populateChunksFromIterable: error when writing new chunks: %w", err)
		return false
	}
	if err = p.currDelIter.Err(); err != nil {
		p.err = fmt.Errorf("populateChunksFromIterable: currDelIter error when writing new chunks: %w", err)
		return false
	}

	if prevValueType != chunkenc.ValNone {
		p.chunksFromIterable = append(p.chunksFromIterable, chunks.Meta{Chunk: currentChunk, MinTime: cmint, MaxTime: cmaxt})
	}

	if len(p.chunksFromIterable) == 0 {
		return false
	}

	p.currMetaWithChunk = p.chunksFromIterable[0]
	p.chunksFromIterableIdx = 0
	return true
}

func (p *populateWithDelChunkSeriesIterator) At() chunks.Meta { return p.currMetaWithChunk }

// blockSeriesSet allows to iterate over sorted, populated series with applied tombstones.
// Series with all deleted chunks are still present as Series with no samples.
// Samples from chunks are also trimmed to requested min and max time.
type blockSeriesSet struct {
	blockBaseSeriesSet
}

func newBlockSeriesSet(i IndexReader, c ChunkReader, t tombstones.Reader, p index.Postings, mint, maxt int64, disableTrimming bool) storage.SeriesSet {
	return &blockSeriesSet{
		blockBaseSeriesSet{
			index:           i,
			chunks:          c,
			tombstones:      t,
			p:               p,
			mint:            mint,
			maxt:            maxt,
			disableTrimming: disableTrimming,
		},
	}
}

func (b *blockSeriesSet) At() storage.Series {
	// At can be looped over before iterating, so save the current values locally.
	return &blockSeriesEntry{
		chunks:     b.chunks,
		blockID:    b.blockID,
		seriesData: b.curr,
	}
}

// blockChunkSeriesSet allows to iterate over sorted, populated series with applied tombstones.
// Series with all deleted chunks are still present as Labelled iterator with no chunks.
// Chunks are also trimmed to requested [min and max] (keeping samples with min and max timestamps).
type blockChunkSeriesSet struct {
	blockBaseSeriesSet
}

func NewBlockChunkSeriesSet(id ulid.ULID, i IndexReader, c ChunkReader, t tombstones.Reader, p index.Postings, mint, maxt int64, disableTrimming bool) storage.ChunkSeriesSet {
	return &blockChunkSeriesSet{
		blockBaseSeriesSet{
			blockID:         id,
			index:           i,
			chunks:          c,
			tombstones:      t,
			p:               p,
			mint:            mint,
			maxt:            maxt,
			disableTrimming: disableTrimming,
		},
	}
}

func (b *blockChunkSeriesSet) At() storage.ChunkSeries {
	// At can be looped over before iterating, so save the current values locally.
	return &chunkSeriesEntry{
		chunks:     b.chunks,
		blockID:    b.blockID,
		seriesData: b.curr,
	}
}

// NewMergedStringIter returns string iterator that allows to merge symbols on demand and stream result.
func NewMergedStringIter(a, b index.StringIter) index.StringIter {
	return &mergedStringIter{a: a, b: b, aok: a.Next(), bok: b.Next()}
}

type mergedStringIter struct {
	a        index.StringIter
	b        index.StringIter
	aok, bok bool
	cur      string
	err      error
}

func (m *mergedStringIter) Next() bool {
	if (!m.aok && !m.bok) || (m.Err() != nil) {
		return false
	}
	switch {
	case !m.aok:
		m.cur = m.b.At()
		m.bok = m.b.Next()
		m.err = m.b.Err()
	case !m.bok:
		m.cur = m.a.At()
		m.aok = m.a.Next()
		m.err = m.a.Err()
	case m.b.At() > m.a.At():
		m.cur = m.a.At()
		m.aok = m.a.Next()
		m.err = m.a.Err()
	case m.a.At() > m.b.At():
		m.cur = m.b.At()
		m.bok = m.b.Next()
		m.err = m.b.Err()
	default: // Equal.
		m.cur = m.b.At()
		m.aok = m.a.Next()
		m.err = m.a.Err()
		m.bok = m.b.Next()
		if m.err == nil {
			m.err = m.b.Err()
		}
	}

	return true
}
func (m mergedStringIter) At() string { return m.cur }
func (m mergedStringIter) Err() error {
	return m.err
}

// DeletedIterator wraps chunk Iterator and makes sure any deleted metrics are not returned.
type DeletedIterator struct {
	// Iter is an Iterator to be wrapped.
	Iter chunkenc.Iterator
	// Intervals are the deletion intervals.
	Intervals tombstones.Intervals
}

func (it *DeletedIterator) At() (int64, float64) {
	return it.Iter.At()
}

func (it *DeletedIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	t, h := it.Iter.AtHistogram(h)
	return t, h
}

func (it *DeletedIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	t, h := it.Iter.AtFloatHistogram(fh)
	return t, h
}

func (it *DeletedIterator) AtT() int64 {
	return it.Iter.AtT()
}

func (it *DeletedIterator) Seek(t int64) chunkenc.ValueType {
	if it.Iter.Err() != nil {
		return chunkenc.ValNone
	}
	valueType := it.Iter.Seek(t)
	if valueType == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	// Now double check if the entry falls into a deleted interval.
	ts := it.AtT()
	for _, itv := range it.Intervals {
		if ts < itv.Mint {
			return valueType
		}

		if ts > itv.Maxt {
			it.Intervals = it.Intervals[1:]
			continue
		}

		// We're in the middle of an interval, we can now call Next().
		return it.Next()
	}

	// The timestamp is greater than all the deleted intervals.
	return valueType
}

func (it *DeletedIterator) Next() chunkenc.ValueType {
Outer:
	for valueType := it.Iter.Next(); valueType != chunkenc.ValNone; valueType = it.Iter.Next() {
		ts := it.AtT()
		for _, tr := range it.Intervals {
			if tr.InBounds(ts) {
				continue Outer
			}

			if ts <= tr.Maxt {
				return valueType
			}
			it.Intervals = it.Intervals[1:]
		}
		return valueType
	}
	return chunkenc.ValNone
}

func (it *DeletedIterator) Err() error { return it.Iter.Err() }

type nopChunkReader struct {
	emptyChunk chunkenc.Chunk
}

func newNopChunkReader() ChunkReader {
	return nopChunkReader{
		emptyChunk: chunkenc.NewXORChunk(),
	}
}

func (cr nopChunkReader) ChunkOrIterable(chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	return cr.emptyChunk, nil, nil
}

func (cr nopChunkReader) Close() error { return nil }
