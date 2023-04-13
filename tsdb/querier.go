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
	"fmt"
	"math"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

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
		return nil, errors.Wrap(err, "open index reader")
	}
	chunkr, err := b.Chunks()
	if err != nil {
		indexr.Close()
		return nil, errors.Wrap(err, "open chunk reader")
	}
	tombsr, err := b.Tombstones()
	if err != nil {
		indexr.Close()
		chunkr.Close()
		return nil, errors.Wrap(err, "open tombstone reader")
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

func (q *blockBaseQuerier) LabelValues(name string, matchers ...*labels.Matcher) ([]string, storage.Warnings, error) {
	res, err := q.index.SortedLabelValues(name, matchers...)
	return res, nil, err
}

func (q *blockBaseQuerier) LabelNames(matchers ...*labels.Matcher) ([]string, storage.Warnings, error) {
	res, err := q.index.LabelNames(matchers...)
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

func (q *blockQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	mint := q.mint
	maxt := q.maxt
	disableTrimming := false
	sharded := hints != nil && hints.ShardCount > 0
	p, err := q.index.PostingsForMatchers(sharded, ms...)
	if err != nil {
		return storage.ErrSeriesSet(err)
	}
	if sharded {
		p = q.index.ShardedPostings(p, hints.ShardIndex, hints.ShardCount)
	}
	if sortSeries {
		p = q.index.SortedPostings(p)
	}

	if hints != nil {
		mint = hints.Start
		maxt = hints.End
		disableTrimming = hints.DisableTrimming
		if hints.Func == "series" {
			// When you're only looking up metadata (for example series API), you don't need to load any chunks.
			return newBlockSeriesSet(q.index, newNopChunkReader(), q.tombstones, p, mint, maxt, disableTrimming)
		}
	}

	return newBlockSeriesSet(q.index, q.chunks, q.tombstones, p, mint, maxt, disableTrimming)
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

func (q *blockChunkQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.ChunkSeriesSet {
	mint := q.mint
	maxt := q.maxt
	disableTrimming := false
	if hints != nil {
		mint = hints.Start
		maxt = hints.End
		disableTrimming = hints.DisableTrimming
	}
	sharded := hints != nil && hints.ShardCount > 0
	p, err := q.index.PostingsForMatchers(sharded, ms...)
	if err != nil {
		return storage.ErrChunkSeriesSet(err)
	}
	if sharded {
		p = q.index.ShardedPostings(p, hints.ShardIndex, hints.ShardCount)
	}
	if sortSeries {
		p = q.index.SortedPostings(p)
	}
	return newBlockChunkSeriesSet(q.blockID, q.index, q.chunks, q.tombstones, p, mint, maxt, disableTrimming)
}

// PostingsForMatchers assembles a single postings iterator against the index reader
// based on the given matchers. The resulting postings are not ordered by series.
func PostingsForMatchers(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
	var its, notIts []index.Postings
	// See which label must be non-empty.
	// Optimization for case like {l=~".", l!="1"}.
	labelMustBeSet := make(map[string]bool, len(ms))
	for _, m := range ms {
		if !m.Matches("") {
			labelMustBeSet[m.Name] = true
		}
	}

	for _, m := range ms {
		if m.Name == "" && m.Value == "" { // Special-case for AllPostings, used in tests at least.
			k, v := index.AllPostingsKey()
			allPostings, err := ix.Postings(k, v)
			if err != nil {
				return nil, err
			}
			its = append(its, allPostings)
		} else if labelMustBeSet[m.Name] {
			// If this matcher must be non-empty, we can be smarter.
			matchesEmpty := m.Matches("")
			isNot := m.Type == labels.MatchNotEqual || m.Type == labels.MatchNotRegexp
			if isNot && matchesEmpty { // l!="foo"
				// If the label can't be empty and is a Not and the inner matcher
				// doesn't match empty, then subtract it out at the end.
				inverse, err := m.Inverse()
				if err != nil {
					return nil, err
				}

				it, err := postingsForMatcher(ix, inverse)
				if err != nil {
					return nil, err
				}
				notIts = append(notIts, it)
			} else if isNot && !matchesEmpty { // l!=""
				// If the label can't be empty and is a Not, but the inner matcher can
				// be empty we need to use inversePostingsForMatcher.
				inverse, err := m.Inverse()
				if err != nil {
					return nil, err
				}

				it, err := inversePostingsForMatcher(ix, inverse)
				if err != nil {
					return nil, err
				}
				if index.IsEmptyPostingsType(it) {
					return index.EmptyPostings(), nil
				}
				its = append(its, it)
			} else { // l="a"
				// Non-Not matcher, use normal postingsForMatcher.
				it, err := postingsForMatcher(ix, m)
				if err != nil {
					return nil, err
				}
				if index.IsEmptyPostingsType(it) {
					return index.EmptyPostings(), nil
				}
				its = append(its, it)
			}
		} else { // l=""
			// If the matchers for a labelname selects an empty value, it selects all
			// the series which don't have the label name set too. See:
			// https://github.com/prometheus/prometheus/issues/3575 and
			// https://github.com/prometheus/prometheus/pull/3578#issuecomment-351653555
			it, err := inversePostingsForMatcher(ix, m)
			if err != nil {
				return nil, err
			}
			notIts = append(notIts, it)
		}
	}

	// If there's nothing to subtract from, add in everything and remove the notIts later.
	if len(its) == 0 && len(notIts) != 0 {
		k, v := index.AllPostingsKey()
		allPostings, err := ix.Postings(k, v)
		if err != nil {
			return nil, err
		}
		its = append(its, allPostings)
	}

	it := index.Intersect(its...)

	for _, n := range notIts {
		it = index.Without(it, n)
	}

	return it, nil
}

func postingsForMatcher(ix IndexPostingsReader, m *labels.Matcher) (index.Postings, error) {
	// This method will not return postings for missing labels.

	// Fast-path for equal matching.
	if m.Type == labels.MatchEqual {
		return ix.Postings(m.Name, m.Value)
	}

	// Fast-path for set matching.
	if m.Type == labels.MatchRegexp {
		setMatches := m.SetMatches()
		if len(setMatches) > 0 {
			return ix.Postings(m.Name, setMatches...)
		}
	}

	vals, err := ix.LabelValues(m.Name)
	if err != nil {
		return nil, err
	}

	var res []string
	for _, val := range vals {
		if m.Matches(val) {
			res = append(res, val)
		}
	}

	if len(res) == 0 {
		return index.EmptyPostings(), nil
	}

	return ix.Postings(m.Name, res...)
}

// inversePostingsForMatcher returns the postings for the series with the label name set but not matching the matcher.
func inversePostingsForMatcher(ix IndexPostingsReader, m *labels.Matcher) (index.Postings, error) {
	vals, err := ix.LabelValues(m.Name)
	if err != nil {
		return nil, err
	}

	var res []string
	// If the inverse match is ="", we just want all the values.
	if m.Type == labels.MatchEqual && m.Value == "" {
		res = vals
	} else {
		for _, val := range vals {
			if !m.Matches(val) {
				res = append(res, val)
			}
		}
	}

	return ix.Postings(m.Name, res...)
}

func labelValuesWithMatchers(r IndexReader, name string, matchers ...*labels.Matcher) ([]string, error) {
	p, err := PostingsForMatchers(r, matchers...)
	if err != nil {
		return nil, errors.Wrap(err, "fetching postings for matchers")
	}

	allValues, err := r.LabelValues(name)
	if err != nil {
		return nil, errors.Wrapf(err, "fetching values of label %s", name)
	}
	valuesPostings := make([]index.Postings, len(allValues))
	for i, value := range allValues {
		valuesPostings[i], err = r.Postings(name, value)
		if err != nil {
			return nil, errors.Wrapf(err, "fetching postings for %s=%q", name, value)
		}
	}
	indexes, err := index.FindIntersectingPostings(p, valuesPostings)
	if err != nil {
		return nil, errors.Wrap(err, "intersecting postings")
	}

	values := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		values = append(values, allValues[idx])
	}

	return values, nil
}

func labelNamesWithMatchers(r IndexReader, matchers ...*labels.Matcher) ([]string, error) {
	p, err := r.PostingsForMatchers(false, matchers...)
	if err != nil {
		return nil, err
	}

	var postings []storage.SeriesRef
	for p.Next() {
		postings = append(postings, p.At())
	}
	if p.Err() != nil {
		return nil, errors.Wrapf(p.Err(), "postings for label names with matchers")
	}

	return r.LabelNamesFor(postings...)
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
// See newBlockSeriesSet and newBlockChunkSeriesSet to use it for either sample or chunk iterating.
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
			if errors.Cause(err) == storage.ErrNotFound {
				continue
			}
			b.err = errors.Wrapf(err, "get series %d", b.p.At())
			return false
		}

		if len(b.bufChks) == 0 {
			continue
		}

		intervals, err := b.tombstones.Get(b.p.At())
		if err != nil {
			b.err = errors.Wrap(err, "get tombstones")
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

func (b *blockBaseSeriesSet) Warnings() storage.Warnings { return nil }

// populateWithDelGenericSeriesIterator allows to iterate over given chunk
// metas. In each iteration it ensures that chunks are trimmed based on given
// tombstones interval if any.
//
// populateWithDelGenericSeriesIterator assumes that chunks that would be fully
// removed by intervals are filtered out in previous phase.
//
// On each iteration currChkMeta is available. If currDelIter is not nil, it
// means that the chunk iterator in currChkMeta is invalid and a chunk rewrite
// is needed, for which currDelIter should be used.
type populateWithDelGenericSeriesIterator struct {
	blockID ulid.ULID
	chunks  ChunkReader
	// chks are expected to be sorted by minTime and should be related to
	// the same, single series.
	chks []chunks.Meta

	i         int // Index into chks; -1 if not started yet.
	err       error
	bufIter   DeletedIterator // Retained for memory re-use. currDelIter may point here.
	intervals tombstones.Intervals

	currDelIter chunkenc.Iterator
	currChkMeta chunks.Meta
}

func (p *populateWithDelGenericSeriesIterator) reset(blockID ulid.ULID, cr ChunkReader, chks []chunks.Meta, intervals tombstones.Intervals) {
	p.blockID = blockID
	p.chunks = cr
	p.chks = chks
	p.i = -1
	p.err = nil
	p.bufIter.Iter = nil
	p.bufIter.Intervals = p.bufIter.Intervals[:0]
	p.intervals = intervals
	p.currDelIter = nil
	p.currChkMeta = chunks.Meta{}
}

// If copyHeadChunk is true, then the head chunk (i.e. the in-memory chunk of the TSDB)
// is deep copied to avoid races between reads and copying chunk bytes.
// However, if the deletion intervals overlaps with the head chunk, then the head chunk is
// not copied irrespective of copyHeadChunk because it will be re-encoded later anyway.
func (p *populateWithDelGenericSeriesIterator) next(copyHeadChunk bool) bool {
	if p.err != nil || p.i >= len(p.chks)-1 {
		return false
	}

	p.i++
	p.currChkMeta = p.chks[p.i]

	p.bufIter.Intervals = p.bufIter.Intervals[:0]
	for _, interval := range p.intervals {
		if p.currChkMeta.OverlapsClosedInterval(interval.Mint, interval.Maxt) {
			p.bufIter.Intervals = p.bufIter.Intervals.Add(interval)
		}
	}

	hcr, ok := p.chunks.(*headChunkReader)
	if ok && copyHeadChunk && len(p.bufIter.Intervals) == 0 {
		// ChunkWithCopy will copy the head chunk.
		var maxt int64
		p.currChkMeta.Chunk, maxt, p.err = hcr.ChunkWithCopy(p.currChkMeta)
		// For the in-memory head chunk the index reader sets maxt as MaxInt64. We fix it here.
		p.currChkMeta.MaxTime = maxt
	} else {
		p.currChkMeta.Chunk, p.err = p.chunks.Chunk(p.currChkMeta)
	}
	if p.err != nil {
		p.err = errors.Wrapf(p.err, "cannot populate chunk %d from block %s", p.currChkMeta.Ref, p.blockID.String())
		return false
	}

	if len(p.bufIter.Intervals) == 0 {
		// If there is no overlap with deletion intervals, we can take chunk as it is.
		p.currDelIter = nil
		return true
	}

	// We don't want the full chunk, take just a part of it.
	p.bufIter.Iter = p.currChkMeta.Chunk.Iterator(p.bufIter.Iter)
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
			p.curr = p.currChkMeta.Chunk.Iterator(p.curr)
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

func (p *populateWithDelSeriesIterator) AtHistogram() (int64, *histogram.Histogram) {
	return p.curr.AtHistogram()
}

func (p *populateWithDelSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	return p.curr.AtFloatHistogram()
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

	curr chunks.Meta
}

func (p *populateWithDelChunkSeriesIterator) reset(blockID ulid.ULID, cr ChunkReader, chks []chunks.Meta, intervals tombstones.Intervals) {
	p.populateWithDelGenericSeriesIterator.reset(blockID, cr, chks, intervals)
	p.curr = chunks.Meta{}
}

func (p *populateWithDelChunkSeriesIterator) Next() bool {
	if !p.next(true) {
		return false
	}
	p.curr = p.currChkMeta
	if p.currDelIter == nil {
		return true
	}
	valueType := p.currDelIter.Next()
	if valueType == chunkenc.ValNone {
		if err := p.currDelIter.Err(); err != nil {
			p.err = errors.Wrap(err, "iterate chunk while re-encoding")
		}
		return false
	}

	// Re-encode the chunk if iterator is provider. This means that it has
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
		if hc, ok := p.currChkMeta.Chunk.(*chunkenc.HistogramChunk); ok {
			newChunk.(*chunkenc.HistogramChunk).SetCounterResetHeader(hc.GetCounterResetHeader())
		}
		var h *histogram.Histogram
		t, h = p.currDelIter.AtHistogram()
		p.curr.MinTime = t

		app.AppendHistogram(t, h)
		for vt := p.currDelIter.Next(); vt != chunkenc.ValNone; vt = p.currDelIter.Next() {
			if vt != chunkenc.ValHistogram {
				err = fmt.Errorf("found value type %v in histogram chunk", vt)
				break
			}
			t, h = p.currDelIter.AtHistogram()

			// Defend against corrupted chunks.
			pI, nI, okToAppend, counterReset := app.(*chunkenc.HistogramAppender).Appendable(h)
			if len(pI)+len(nI) > 0 {
				err = fmt.Errorf(
					"bucket layout has changed unexpectedly: %d positive and %d negative bucket interjections required",
					len(pI), len(nI),
				)
				break
			}
			if counterReset {
				err = errors.New("detected unexpected counter reset in histogram")
				break
			}
			if !okToAppend {
				err = errors.New("unable to append histogram due to unexpected schema change")
				break
			}

			app.AppendHistogram(t, h)
		}
	case chunkenc.ValFloat:
		newChunk = chunkenc.NewXORChunk()
		if app, err = newChunk.Appender(); err != nil {
			break
		}
		var v float64
		t, v = p.currDelIter.At()
		p.curr.MinTime = t
		app.Append(t, v)
		for vt := p.currDelIter.Next(); vt != chunkenc.ValNone; vt = p.currDelIter.Next() {
			if vt != chunkenc.ValFloat {
				err = fmt.Errorf("found value type %v in float chunk", vt)
				break
			}
			t, v = p.currDelIter.At()
			app.Append(t, v)
		}
	case chunkenc.ValFloatHistogram:
		newChunk = chunkenc.NewFloatHistogramChunk()
		if app, err = newChunk.Appender(); err != nil {
			break
		}
		if hc, ok := p.currChkMeta.Chunk.(*chunkenc.FloatHistogramChunk); ok {
			newChunk.(*chunkenc.FloatHistogramChunk).SetCounterResetHeader(hc.GetCounterResetHeader())
		}
		var h *histogram.FloatHistogram
		t, h = p.currDelIter.AtFloatHistogram()
		p.curr.MinTime = t

		app.AppendFloatHistogram(t, h)
		for vt := p.currDelIter.Next(); vt != chunkenc.ValNone; vt = p.currDelIter.Next() {
			if vt != chunkenc.ValFloatHistogram {
				err = fmt.Errorf("found value type %v in histogram chunk", vt)
				break
			}
			t, h = p.currDelIter.AtFloatHistogram()

			// Defend against corrupted chunks.
			pI, nI, okToAppend, counterReset := app.(*chunkenc.FloatHistogramAppender).Appendable(h)
			if len(pI)+len(nI) > 0 {
				err = fmt.Errorf(
					"bucket layout has changed unexpectedly: %d positive and %d negative bucket interjections required",
					len(pI), len(nI),
				)
				break
			}
			if counterReset {
				err = errors.New("detected unexpected counter reset in histogram")
				break
			}
			if !okToAppend {
				err = errors.New("unable to append histogram due to unexpected schema change")
				break
			}

			app.AppendFloatHistogram(t, h)
		}
	default:
		err = fmt.Errorf("populateWithDelChunkSeriesIterator: value type %v unsupported", valueType)
	}

	if err != nil {
		p.err = errors.Wrap(err, "iterate chunk while re-encoding")
		return false
	}
	if err := p.currDelIter.Err(); err != nil {
		p.err = errors.Wrap(err, "iterate chunk while re-encoding")
		return false
	}

	p.curr.Chunk = newChunk
	p.curr.MaxTime = t
	return true
}

func (p *populateWithDelChunkSeriesIterator) At() chunks.Meta { return p.curr }

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

func newBlockChunkSeriesSet(id ulid.ULID, i IndexReader, c ChunkReader, t tombstones.Reader, p index.Postings, mint, maxt int64, disableTrimming bool) storage.ChunkSeriesSet {
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

	if !m.aok {
		m.cur = m.b.At()
		m.bok = m.b.Next()
		m.err = m.b.Err()
	} else if !m.bok {
		m.cur = m.a.At()
		m.aok = m.a.Next()
		m.err = m.a.Err()
	} else if m.b.At() > m.a.At() {
		m.cur = m.a.At()
		m.aok = m.a.Next()
		m.err = m.a.Err()
	} else if m.a.At() > m.b.At() {
		m.cur = m.b.At()
		m.bok = m.b.Next()
		m.err = m.b.Err()
	} else { // Equal.
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

func (it *DeletedIterator) AtHistogram() (int64, *histogram.Histogram) {
	t, h := it.Iter.AtHistogram()
	return t, h
}

func (it *DeletedIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	t, h := it.Iter.AtFloatHistogram()
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

func (cr nopChunkReader) Chunk(meta chunks.Meta) (chunkenc.Chunk, error) {
	return cr.emptyChunk, nil
}

func (cr nopChunkReader) Close() error { return nil }
