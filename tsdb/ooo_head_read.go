// Copyright The Prometheus Authors
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

	"github.com/oklog/ulid/v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/util/annotations"
)

var _ IndexReader = &HeadAndOOOIndexReader{}

type HeadAndOOOIndexReader struct {
	*headIndexReader            // A reference to the headIndexReader so we can reuse as many interface implementation as possible.
	inoMint                     int64
	lastGarbageCollectedMmapRef chunks.ChunkDiskMapperRef
}

var _ chunkenc.Iterable = &mergedOOOChunks{}

// mergedOOOChunks holds the list of iterables for overlapping chunks.
type mergedOOOChunks struct {
	chunkIterables []chunkenc.Iterable
}

func (o mergedOOOChunks) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	return storage.ChainSampleIteratorFromIterables(iterator, o.chunkIterables)
}

func NewHeadAndOOOIndexReader(head *Head, inoMint, mint, maxt int64, lastGarbageCollectedMmapRef chunks.ChunkDiskMapperRef) *HeadAndOOOIndexReader {
	hr := &headIndexReader{
		head: head,
		mint: mint,
		maxt: maxt,
	}
	return &HeadAndOOOIndexReader{hr, inoMint, lastGarbageCollectedMmapRef}
}

func (oh *HeadAndOOOIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	s := oh.head.series.getByID(chunks.HeadSeriesRef(ref))

	if s == nil {
		oh.head.metrics.seriesNotFound.Inc()
		return storage.ErrNotFound
	}
	builder.Assign(s.labels())

	if chks == nil {
		return nil
	}

	s.Lock()
	defer s.Unlock()
	*chks = (*chks)[:0]

	if s.ooo != nil {
		return getOOOSeriesChunks(s, oh.mint, oh.maxt, oh.lastGarbageCollectedMmapRef, 0, true, oh.inoMint, chks)
	}
	*chks = appendSeriesChunks(s, oh.inoMint, oh.maxt, *chks)
	return nil
}

// lastGarbageCollectedMmapRef gives the last mmap chunk that may be being garbage collected and so
// any chunk at or before this ref will not be considered. 0 disables this check.
//
// maxMmapRef tells upto what max m-map chunk that we can consider. If it is non-0, then
// the oooHeadChunk will not be considered.
func getOOOSeriesChunks(s *memSeries, mint, maxt int64, lastGarbageCollectedMmapRef, maxMmapRef chunks.ChunkDiskMapperRef, includeInOrder bool, inoMint int64, chks *[]chunks.Meta) error {
	tmpChks := make([]chunks.Meta, 0, len(s.ooo.oooMmappedChunks))

	addChunk := func(minT, maxT int64, ref chunks.ChunkRef, chunk chunkenc.Chunk) {
		tmpChks = append(tmpChks, chunks.Meta{
			MinTime: minT,
			MaxTime: maxT,
			Ref:     ref,
			Chunk:   chunk,
		})
	}

	// Collect all chunks that overlap the query range.
	if s.ooo.oooHeadChunk != nil {
		c := s.ooo.oooHeadChunk
		if c.OverlapsClosedInterval(mint, maxt) && maxMmapRef == 0 {
			ref := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(len(s.ooo.oooMmappedChunks))))
			if len(c.chunk.samples) > 0 { // Empty samples happens in tests, at least.
				chks, err := s.ooo.oooHeadChunk.chunk.ToEncodedChunks(c.minTime, c.maxTime)
				if err != nil {
					handleChunkWriteError(err)
					return nil
				}
				for _, chk := range chks {
					addChunk(chk.minTime, chk.maxTime, ref, chk.chunk)
				}
			} else {
				var emptyChunk chunkenc.Chunk
				addChunk(c.minTime, c.maxTime, ref, emptyChunk)
			}
		}
	}
	for i := len(s.ooo.oooMmappedChunks) - 1; i >= 0; i-- {
		c := s.ooo.oooMmappedChunks[i]
		if c.OverlapsClosedInterval(mint, maxt) && (maxMmapRef == 0 || maxMmapRef.GreaterThanOrEqualTo(c.ref)) && (lastGarbageCollectedMmapRef == 0 || c.ref.GreaterThan(lastGarbageCollectedMmapRef)) {
			ref := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(i)))
			addChunk(c.minTime, c.maxTime, ref, nil)
		}
	}

	if includeInOrder {
		tmpChks = appendSeriesChunks(s, inoMint, maxt, tmpChks)
	}

	// There is nothing to do if we did not collect any chunk.
	if len(tmpChks) == 0 {
		return nil
	}

	// Next we want to sort all the collected chunks by min time so we can find
	// those that overlap.
	slices.SortFunc(tmpChks, lessByMinTimeAndMinRef)

	// Next we want to iterate the sorted collected chunks and return composites for chunks that overlap with others.
	// Example chunks of a series: 5:(100, 200) 6:(500, 600) 7:(150, 250) 8:(550, 650)
	// In the example 5 overlaps with 7 and 6 overlaps with 8 so we will return
	// [5,7], [6,8].
	toBeMerged := tmpChks[0]
	for _, c := range tmpChks[1:] {
		if c.MinTime > toBeMerged.MaxTime {
			// This chunk doesn't overlap. Send current toBeMerged to output and start a new one.
			*chks = append(*chks, toBeMerged)
			toBeMerged = c
		} else {
			// Merge this chunk with existing toBeMerged.
			if mm, ok := toBeMerged.Chunk.(*multiMeta); ok {
				mm.metas = append(mm.metas, c)
			} else {
				toBeMerged.Chunk = &multiMeta{metas: []chunks.Meta{toBeMerged, c}}
			}
			if toBeMerged.MaxTime < c.MaxTime {
				toBeMerged.MaxTime = c.MaxTime
			}
		}
	}
	*chks = append(*chks, toBeMerged)

	return nil
}

// Fake Chunk object to pass a set of Metas inside Meta.Chunk.
type multiMeta struct {
	chunkenc.Chunk // We don't expect any of the methods to be called.
	metas          []chunks.Meta
}

// LabelValues needs to be overridden from the headIndexReader implementation
// so we can return labels within either in-order range or ooo range.
func (oh *HeadAndOOOIndexReader) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error) {
	if oh.maxt < oh.head.MinTime() && oh.maxt < oh.head.MinOOOTime() || oh.mint > oh.head.MaxTime() && oh.mint > oh.head.MaxOOOTime() {
		return []string{}, nil
	}

	if len(matchers) == 0 {
		return oh.head.postings.LabelValues(ctx, name, hints), nil
	}

	return labelValuesWithMatchers(ctx, oh, name, hints, matchers...)
}

func lessByMinTimeAndMinRef(a, b chunks.Meta) int {
	switch {
	case a.MinTime < b.MinTime:
		return -1
	case a.MinTime > b.MinTime:
		return 1
	}

	switch {
	case a.Ref < b.Ref:
		return -1
	case a.Ref > b.Ref:
		return 1
	default:
		return 0
	}
}

type HeadAndOOOChunkReader struct {
	head        *Head
	mint, maxt  int64
	cr          *headChunkReader // If nil, only read OOO chunks.
	maxMmapRef  chunks.ChunkDiskMapperRef
	oooIsoState *oooIsolationState
}

func NewHeadAndOOOChunkReader(head *Head, mint, maxt int64, cr *headChunkReader, oooIsoState *oooIsolationState, maxMmapRef chunks.ChunkDiskMapperRef) *HeadAndOOOChunkReader {
	return &HeadAndOOOChunkReader{
		head:        head,
		mint:        mint,
		maxt:        maxt,
		cr:          cr,
		maxMmapRef:  maxMmapRef,
		oooIsoState: oooIsoState,
	}
}

func (cr *HeadAndOOOChunkReader) ChunkOrIterable(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	c, it, _, err := cr.chunkOrIterable(meta, false)
	return c, it, err
}

// ChunkOrIterableWithCopy implements ChunkReaderWithCopy. The special Copy
// behaviour is only implemented for the in-order head chunk.
func (cr *HeadAndOOOChunkReader) ChunkOrIterableWithCopy(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, int64, error) {
	return cr.chunkOrIterable(meta, true)
}

func (cr *HeadAndOOOChunkReader) chunkOrIterable(meta chunks.Meta, copyLastChunk bool) (chunkenc.Chunk, chunkenc.Iterable, int64, error) {
	sid, cid, isOOO := unpackHeadChunkRef(meta.Ref)
	s := cr.head.series.getByID(sid)
	// This means that the series has been garbage collected.
	if s == nil {
		return nil, nil, 0, storage.ErrNotFound
	}
	var isoState *isolationState
	if cr.cr != nil {
		isoState = cr.cr.isoState
	}

	s.Lock()
	defer s.Unlock()

	if meta.Chunk == nil {
		c, maxt, err := cr.head.chunkFromSeries(s, cid, isOOO, meta.MinTime, meta.MaxTime, isoState, copyLastChunk)
		return c, nil, maxt, err
	}
	mm, ok := meta.Chunk.(*multiMeta)
	if !ok { // Complete chunk was supplied.
		return meta.Chunk, nil, meta.MaxTime, nil
	}
	// We have a composite meta: construct a composite iterable.
	mc := &mergedOOOChunks{}
	for _, m := range mm.metas {
		switch {
		case m.Chunk != nil:
			mc.chunkIterables = append(mc.chunkIterables, m.Chunk)
		default:
			_, cid, isOOO := unpackHeadChunkRef(m.Ref)
			iterable, _, err := cr.head.chunkFromSeries(s, cid, isOOO, m.MinTime, m.MaxTime, isoState, copyLastChunk)
			if err != nil {
				return nil, nil, 0, fmt.Errorf("invalid head chunk: %w", err)
			}
			mc.chunkIterables = append(mc.chunkIterables, iterable)
		}
	}
	return nil, mc, meta.MaxTime, nil
}

func (cr *HeadAndOOOChunkReader) Close() error {
	if cr.cr != nil && cr.cr.isoState != nil {
		cr.cr.isoState.Close()
	}
	if cr.oooIsoState != nil {
		cr.oooIsoState.Close()
	}
	return nil
}

type OOOCompactionHead struct {
	head        *Head
	lastMmapRef chunks.ChunkDiskMapperRef
	lastWBLFile int
	postings    []storage.SeriesRef
	chunkRange  int64
	mint, maxt  int64 // Among all the compactable chunks.
}

// NewOOOCompactionHead does the following:
// 1. M-maps all the in-memory ooo chunks.
// 2. Compute the expected block ranges while iterating through all ooo series and store it.
// 3. Store the list of postings having ooo series.
// 4. Cuts a new WBL file for the OOO WBL.
// All the above together have a bit of CPU and memory overhead, and can have a bit of impact
// on the sample append latency. So call NewOOOCompactionHead only right before compaction.
func NewOOOCompactionHead(ctx context.Context, head *Head) (*OOOCompactionHead, error) {
	ch := &OOOCompactionHead{
		head:       head,
		chunkRange: head.chunkRange.Load(),
		mint:       math.MaxInt64,
		maxt:       math.MinInt64,
	}

	if head.wbl != nil {
		lastWBLFile, err := head.wbl.NextSegmentSync()
		if err != nil {
			return nil, err
		}
		ch.lastWBLFile = lastWBLFile
	}

	hr := headIndexReader{head: head, mint: ch.mint, maxt: ch.maxt}
	n, v := index.AllPostingsKey()
	// TODO: filter to series with OOO samples, before sorting.
	p, err := hr.Postings(ctx, n, v)
	if err != nil {
		return nil, err
	}
	p = hr.SortedPostings(p)

	var lastSeq, lastOff int
	for p.Next() {
		seriesRef := p.At()
		ms := head.series.getByID(chunks.HeadSeriesRef(seriesRef))
		if ms == nil {
			continue
		}

		// M-map the in-memory chunk and keep track of the last one.
		// Also build the block ranges -> series map.
		// TODO: consider having a lock specifically for ooo data.
		ms.Lock()

		if ms.ooo == nil {
			ms.Unlock()
			continue
		}

		var lastMmapRef chunks.ChunkDiskMapperRef
		mmapRefs := ms.mmapCurrentOOOHeadChunk(head.chunkDiskMapper, head.logger)
		if len(mmapRefs) == 0 && len(ms.ooo.oooMmappedChunks) > 0 {
			// Nothing was m-mapped. So take the mmapRef from the existing slice if it exists.
			mmapRefs = []chunks.ChunkDiskMapperRef{ms.ooo.oooMmappedChunks[len(ms.ooo.oooMmappedChunks)-1].ref}
		}
		if len(mmapRefs) == 0 {
			lastMmapRef = 0
		} else {
			lastMmapRef = mmapRefs[len(mmapRefs)-1]
		}
		seq, off := lastMmapRef.Unpack()
		if seq > lastSeq || (seq == lastSeq && off > lastOff) {
			ch.lastMmapRef, lastSeq, lastOff = lastMmapRef, seq, off
		}
		if len(ms.ooo.oooMmappedChunks) > 0 {
			ch.postings = append(ch.postings, seriesRef)
			for _, c := range ms.ooo.oooMmappedChunks {
				if c.minTime < ch.mint {
					ch.mint = c.minTime
				}
				if c.maxTime > ch.maxt {
					ch.maxt = c.maxTime
				}
			}
		}
		ms.Unlock()
	}

	return ch, nil
}

func (ch *OOOCompactionHead) Index() (IndexReader, error) {
	return NewOOOCompactionHeadIndexReader(ch), nil
}

func (ch *OOOCompactionHead) Chunks() (ChunkReader, error) {
	return NewHeadAndOOOChunkReader(ch.head, ch.mint, ch.maxt, nil, nil, ch.lastMmapRef), nil
}

func (*OOOCompactionHead) Tombstones() (tombstones.Reader, error) {
	return tombstones.NewMemTombstones(), nil
}

var oooCompactionHeadULID = ulid.MustParse("0000000000XX000COMPACTHEAD")

func (ch *OOOCompactionHead) Meta() BlockMeta {
	return BlockMeta{
		MinTime: ch.mint,
		MaxTime: ch.maxt,
		ULID:    oooCompactionHeadULID,
		Stats: BlockStats{
			NumSeries: uint64(len(ch.postings)),
		},
	}
}

// CloneForTimeRange clones the OOOCompactionHead such that the IndexReader and ChunkReader
// obtained from this only looks at the m-map chunks within the given time ranges while not looking
// beyond the ch.lastMmapRef.
// Only the method of BlockReader interface are valid for the cloned OOOCompactionHead.
func (ch *OOOCompactionHead) CloneForTimeRange(mint, maxt int64) *OOOCompactionHead {
	return &OOOCompactionHead{
		head:        ch.head,
		lastMmapRef: ch.lastMmapRef,
		postings:    ch.postings,
		chunkRange:  ch.chunkRange,
		mint:        mint,
		maxt:        maxt,
	}
}

func (*OOOCompactionHead) Size() int64                               { return 0 }
func (ch *OOOCompactionHead) MinTime() int64                         { return ch.mint }
func (ch *OOOCompactionHead) MaxTime() int64                         { return ch.maxt }
func (ch *OOOCompactionHead) ChunkRange() int64                      { return ch.chunkRange }
func (ch *OOOCompactionHead) LastMmapRef() chunks.ChunkDiskMapperRef { return ch.lastMmapRef }
func (ch *OOOCompactionHead) LastWBLFile() int                       { return ch.lastWBLFile }

type OOOCompactionHeadIndexReader struct {
	ch *OOOCompactionHead
}

func NewOOOCompactionHeadIndexReader(ch *OOOCompactionHead) IndexReader {
	return &OOOCompactionHeadIndexReader{ch: ch}
}

func (ir *OOOCompactionHeadIndexReader) Symbols() index.StringIter {
	hr := headIndexReader{head: ir.ch.head, mint: ir.ch.mint, maxt: ir.ch.maxt}
	return hr.Symbols()
}

func (ir *OOOCompactionHeadIndexReader) Postings(_ context.Context, name string, values ...string) (index.Postings, error) {
	n, v := index.AllPostingsKey()
	if name != n || len(values) != 1 || values[0] != v {
		return nil, errors.New("only AllPostingsKey is supported")
	}
	return index.NewListPostings(ir.ch.postings), nil
}

func (*OOOCompactionHeadIndexReader) PostingsForLabelMatching(context.Context, string, func(string) bool) index.Postings {
	return index.ErrPostings(errors.New("not supported"))
}

func (*OOOCompactionHeadIndexReader) PostingsForAllLabelValues(context.Context, string) index.Postings {
	return index.ErrPostings(errors.New("not supported"))
}

func (*OOOCompactionHeadIndexReader) SortedPostings(p index.Postings) index.Postings {
	// This will already be sorted from the Postings() call above.
	return p
}

func (ir *OOOCompactionHeadIndexReader) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	hr := headIndexReader{head: ir.ch.head, mint: ir.ch.mint, maxt: ir.ch.maxt}
	return hr.ShardedPostings(p, shardIndex, shardCount)
}

func (ir *OOOCompactionHeadIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	s := ir.ch.head.series.getByID(chunks.HeadSeriesRef(ref))

	if s == nil {
		ir.ch.head.metrics.seriesNotFound.Inc()
		return storage.ErrNotFound
	}
	builder.Assign(s.labels())

	s.Lock()
	defer s.Unlock()
	*chks = (*chks)[:0]

	if s.ooo == nil {
		return nil
	}

	return getOOOSeriesChunks(s, ir.ch.mint, ir.ch.maxt, 0, ir.ch.lastMmapRef, false, 0, chks)
}

func (*OOOCompactionHeadIndexReader) SortedLabelValues(_ context.Context, _ string, _ *storage.LabelHints, _ ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (*OOOCompactionHeadIndexReader) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (*OOOCompactionHeadIndexReader) PostingsForMatchers(context.Context, bool, ...*labels.Matcher) (index.Postings, error) {
	return nil, errors.New("not implemented")
}

func (*OOOCompactionHeadIndexReader) LabelNames(context.Context, ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (*OOOCompactionHeadIndexReader) LabelNamesFor(context.Context, index.Postings) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (*OOOCompactionHeadIndexReader) Close() error {
	return nil
}

// HeadAndOOOQuerier queries both the head and the out-of-order head.
type HeadAndOOOQuerier struct {
	mint, maxt int64
	head       *Head
	index      IndexReader
	chunkr     ChunkReader
	querier    storage.Querier // Used for LabelNames, LabelValues, but may be nil if head was truncated in the mean time, in which case we ignore it and not close it in the end.
}

func NewHeadAndOOOQuerier(inoMint, mint, maxt int64, head *Head, oooIsoState *oooIsolationState, querier storage.Querier) storage.Querier {
	cr := &headChunkReader{
		head:     head,
		mint:     mint,
		maxt:     maxt,
		isoState: head.iso.State(mint, maxt),
	}
	return &HeadAndOOOQuerier{
		mint:    mint,
		maxt:    maxt,
		head:    head,
		index:   NewHeadAndOOOIndexReader(head, inoMint, mint, maxt, oooIsoState.minRef),
		chunkr:  NewHeadAndOOOChunkReader(head, mint, maxt, cr, oooIsoState, 0),
		querier: querier,
	}
}

func (q *HeadAndOOOQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if q.querier == nil {
		return nil, nil, nil
	}
	return q.querier.LabelValues(ctx, name, hints, matchers...)
}

func (q *HeadAndOOOQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if q.querier == nil {
		return nil, nil, nil
	}
	return q.querier.LabelNames(ctx, hints, matchers...)
}

func (q *HeadAndOOOQuerier) Close() error {
	q.chunkr.Close()
	if q.querier == nil {
		return nil
	}
	return q.querier.Close()
}

func (q *HeadAndOOOQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	return selectSeriesSet(ctx, sortSeries, hints, matchers, q.index, q.chunkr, q.head.tombstones, q.mint, q.maxt)
}

// HeadAndOOOChunkQuerier queries both the head and the out-of-order head.
type HeadAndOOOChunkQuerier struct {
	mint, maxt int64
	head       *Head
	index      IndexReader
	chunkr     ChunkReader
	querier    storage.ChunkQuerier
}

func NewHeadAndOOOChunkQuerier(inoMint, mint, maxt int64, head *Head, oooIsoState *oooIsolationState, querier storage.ChunkQuerier) storage.ChunkQuerier {
	cr := &headChunkReader{
		head:     head,
		mint:     mint,
		maxt:     maxt,
		isoState: head.iso.State(mint, maxt),
	}
	return &HeadAndOOOChunkQuerier{
		mint:    mint,
		maxt:    maxt,
		head:    head,
		index:   NewHeadAndOOOIndexReader(head, inoMint, mint, maxt, oooIsoState.minRef),
		chunkr:  NewHeadAndOOOChunkReader(head, mint, maxt, cr, oooIsoState, 0),
		querier: querier,
	}
}

func (q *HeadAndOOOChunkQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if q.querier == nil {
		return nil, nil, nil
	}
	return q.querier.LabelValues(ctx, name, hints, matchers...)
}

func (q *HeadAndOOOChunkQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if q.querier == nil {
		return nil, nil, nil
	}
	return q.querier.LabelNames(ctx, hints, matchers...)
}

func (q *HeadAndOOOChunkQuerier) Close() error {
	q.chunkr.Close()
	if q.querier == nil {
		return nil
	}
	return q.querier.Close()
}

func (q *HeadAndOOOChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	return selectChunkSeriesSet(ctx, sortSeries, hints, matchers, rangeHeadULID, q.index, q.chunkr, q.head.tombstones, q.mint, q.maxt)
}
