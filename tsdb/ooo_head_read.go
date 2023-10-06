// Copyright 2022 The Prometheus Authors
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

// nolint:revive // Many unused function arguments in this file by design.
package tsdb

import (
	"context"
	"errors"
	"math"

	"golang.org/x/exp/slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

var _ IndexReader = &OOOHeadIndexReader{}

// OOOHeadIndexReader implements IndexReader so ooo samples in the head can be
// accessed.
// It also has a reference to headIndexReader so we can leverage on its
// IndexReader implementation for all the methods that remain the same. We
// decided to do this to avoid code duplication.
// The only methods that change are the ones about getting Series and Postings.
type OOOHeadIndexReader struct {
	*headIndexReader // A reference to the headIndexReader so we can reuse as many interface implementation as possible.
}

func NewOOOHeadIndexReader(head *Head, mint, maxt int64) *OOOHeadIndexReader {
	hr := &headIndexReader{
		head: head,
		mint: mint,
		maxt: maxt,
	}
	return &OOOHeadIndexReader{hr}
}

func (oh *OOOHeadIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	return oh.series(ref, builder, chks, 0)
}

// The passed lastMmapRef tells upto what max m-map chunk that we can consider.
// If it is 0, it means all chunks need to be considered.
// If it is non-0, then the oooHeadChunk must not be considered.
func (oh *OOOHeadIndexReader) series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta, lastMmapRef chunks.ChunkDiskMapperRef) error {
	s := oh.head.series.getByID(chunks.HeadSeriesRef(ref))

	if s == nil {
		oh.head.metrics.seriesNotFound.Inc()
		return storage.ErrNotFound
	}
	builder.Assign(s.lset)

	if chks == nil {
		return nil
	}

	s.Lock()
	defer s.Unlock()
	*chks = (*chks)[:0]

	if s.ooo == nil {
		return nil
	}

	tmpChks := make([]chunks.Meta, 0, len(s.ooo.oooMmappedChunks))

	// We define these markers to track the last chunk reference while we
	// fill the chunk meta.
	// These markers are useful to give consistent responses to repeated queries
	// even if new chunks that might be overlapping or not are added afterwards.
	// Also, lastMinT and lastMaxT are initialized to the max int as a sentinel
	// value to know they are unset.
	var lastChunkRef chunks.ChunkRef
	lastMinT, lastMaxT := int64(math.MaxInt64), int64(math.MaxInt64)

	addChunk := func(minT, maxT int64, ref chunks.ChunkRef) {
		// the first time we get called is for the last included chunk.
		// set the markers accordingly
		if lastMinT == int64(math.MaxInt64) {
			lastChunkRef = ref
			lastMinT = minT
			lastMaxT = maxT
		}

		tmpChks = append(tmpChks, chunks.Meta{
			MinTime:        minT,
			MaxTime:        maxT,
			Ref:            ref,
			OOOLastRef:     lastChunkRef,
			OOOLastMinTime: lastMinT,
			OOOLastMaxTime: lastMaxT,
		})
	}

	// Collect all chunks that overlap the query range, in order from most recent to most old,
	// so we can set the correct markers.
	if s.ooo.oooHeadChunk != nil {
		c := s.ooo.oooHeadChunk
		if c.OverlapsClosedInterval(oh.mint, oh.maxt) && lastMmapRef == 0 {
			ref := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(len(s.ooo.oooMmappedChunks))))
			addChunk(c.minTime, c.maxTime, ref)
		}
	}
	for i := len(s.ooo.oooMmappedChunks) - 1; i >= 0; i-- {
		c := s.ooo.oooMmappedChunks[i]
		if c.OverlapsClosedInterval(oh.mint, oh.maxt) && (lastMmapRef == 0 || lastMmapRef.GreaterThanOrEqualTo(c.ref)) {
			ref := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(i)))
			addChunk(c.minTime, c.maxTime, ref)
		}
	}

	// There is nothing to do if we did not collect any chunk.
	if len(tmpChks) == 0 {
		return nil
	}

	// Next we want to sort all the collected chunks by min time so we can find
	// those that overlap.
	slices.SortFunc(tmpChks, lessByMinTimeAndMinRef)

	// Next we want to iterate the sorted collected chunks and only return the
	// chunks Meta the first chunk that overlaps with others.
	// Example chunks of a series: 5:(100, 200) 6:(500, 600) 7:(150, 250) 8:(550, 650)
	// In the example 5 overlaps with 7 and 6 overlaps with 8 so we only want to
	// return chunk Metas for chunk 5 and chunk 6e
	*chks = append(*chks, tmpChks[0])
	maxTime := tmpChks[0].MaxTime // Tracks the maxTime of the previous "to be merged chunk".
	for _, c := range tmpChks[1:] {
		switch {
		case c.MinTime > maxTime:
			*chks = append(*chks, c)
			maxTime = c.MaxTime
		case c.MaxTime > maxTime:
			maxTime = c.MaxTime
			(*chks)[len(*chks)-1].MaxTime = c.MaxTime
		}
	}

	return nil
}

// PostingsForMatchers needs to be overridden so that the right IndexReader
// implementation gets passed down to the PostingsForMatchers call.
func (oh *OOOHeadIndexReader) PostingsForMatchers(ctx context.Context, concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	return oh.head.pfmc.PostingsForMatchers(ctx, oh, concurrent, ms...)
}

// LabelValues needs to be overridden from the headIndexReader implementation due
// to the check that happens at the beginning where we make sure that the query
// interval overlaps with the head minooot and maxooot.
func (oh *OOOHeadIndexReader) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	if oh.maxt < oh.head.MinOOOTime() || oh.mint > oh.head.MaxOOOTime() {
		return []string{}, nil
	}

	if len(matchers) == 0 {
		return oh.head.postings.LabelValues(ctx, name), nil
	}

	return labelValuesWithMatchers(ctx, oh, name, matchers...)
}

type chunkMetaAndChunkDiskMapperRef struct {
	meta     chunks.Meta
	ref      chunks.ChunkDiskMapperRef
	origMinT int64
	origMaxT int64
}

func refLessByMinTimeAndMinRef(a, b chunkMetaAndChunkDiskMapperRef) int {
	if a.meta.MinTime == b.meta.MinTime {
		return int(a.meta.Ref - b.meta.Ref)
	}
	return int(a.meta.MinTime - b.meta.MinTime)
}

func lessByMinTimeAndMinRef(a, b chunks.Meta) int {
	if a.MinTime == b.MinTime {
		return int(a.Ref - b.Ref)
	}
	return int(a.MinTime - b.MinTime)
}

func (oh *OOOHeadIndexReader) Postings(ctx context.Context, name string, values ...string) (index.Postings, error) {
	switch len(values) {
	case 0:
		return index.EmptyPostings(), nil
	case 1:
		return oh.head.postings.Get(name, values[0]), nil // TODO(ganesh) Also call GetOOOPostings
	default:
		// TODO(ganesh) We want to only return postings for out of order series.
		res := make([]index.Postings, 0, len(values))
		for _, value := range values {
			res = append(res, oh.head.postings.Get(name, value)) // TODO(ganesh) Also call GetOOOPostings
		}
		return index.Merge(ctx, res...), nil
	}
}

type OOOHeadChunkReader struct {
	head       *Head
	mint, maxt int64
}

func NewOOOHeadChunkReader(head *Head, mint, maxt int64) *OOOHeadChunkReader {
	return &OOOHeadChunkReader{
		head: head,
		mint: mint,
		maxt: maxt,
	}
}

func (cr OOOHeadChunkReader) Chunk(meta chunks.Meta) (chunkenc.Chunk, error) {
	sid, _ := chunks.HeadChunkRef(meta.Ref).Unpack()

	s := cr.head.series.getByID(sid)
	// This means that the series has been garbage collected.
	if s == nil {
		return nil, storage.ErrNotFound
	}

	s.Lock()
	if s.ooo == nil {
		// There is no OOO data for this series.
		s.Unlock()
		return nil, storage.ErrNotFound
	}
	c, err := s.oooMergedChunk(meta, cr.head.chunkDiskMapper, cr.mint, cr.maxt)
	s.Unlock()
	if err != nil {
		return nil, err
	}

	// This means that the query range did not overlap with the requested chunk.
	if len(c.chunks) == 0 {
		return nil, storage.ErrNotFound
	}

	return c, nil
}

func (cr OOOHeadChunkReader) Close() error {
	return nil
}

type OOOCompactionHead struct {
	oooIR       *OOOHeadIndexReader
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

	ch.oooIR = NewOOOHeadIndexReader(head, math.MinInt64, math.MaxInt64)
	n, v := index.AllPostingsKey()

	// TODO: verify this gets only ooo samples.
	p, err := ch.oooIR.Postings(ctx, n, v)
	if err != nil {
		return nil, err
	}
	p = ch.oooIR.SortedPostings(p)

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

		mmapRef := ms.mmapCurrentOOOHeadChunk(head.chunkDiskMapper)
		if mmapRef == 0 && len(ms.ooo.oooMmappedChunks) > 0 {
			// Nothing was m-mapped. So take the mmapRef from the existing slice if it exists.
			mmapRef = ms.ooo.oooMmappedChunks[len(ms.ooo.oooMmappedChunks)-1].ref
		}
		seq, off := mmapRef.Unpack()
		if seq > lastSeq || (seq == lastSeq && off > lastOff) {
			ch.lastMmapRef, lastSeq, lastOff = mmapRef, seq, off
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
	return NewOOOHeadChunkReader(ch.oooIR.head, ch.oooIR.mint, ch.oooIR.maxt), nil
}

func (ch *OOOCompactionHead) Tombstones() (tombstones.Reader, error) {
	return tombstones.NewMemTombstones(), nil
}

func (ch *OOOCompactionHead) Meta() BlockMeta {
	var id [16]byte
	copy(id[:], "copy(id[:], \"ooo_compact_head\")")
	return BlockMeta{
		MinTime: ch.mint,
		MaxTime: ch.maxt,
		ULID:    id,
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
		oooIR:       NewOOOHeadIndexReader(ch.oooIR.head, mint, maxt),
		lastMmapRef: ch.lastMmapRef,
		postings:    ch.postings,
		chunkRange:  ch.chunkRange,
		mint:        ch.mint,
		maxt:        ch.maxt,
	}
}

func (ch *OOOCompactionHead) Size() int64                            { return 0 }
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
	return ir.ch.oooIR.Symbols()
}

func (ir *OOOCompactionHeadIndexReader) Postings(_ context.Context, name string, values ...string) (index.Postings, error) {
	n, v := index.AllPostingsKey()
	if name != n || len(values) != 1 || values[0] != v {
		return nil, errors.New("only AllPostingsKey is supported")
	}
	return index.NewListPostings(ir.ch.postings), nil
}

func (ir *OOOCompactionHeadIndexReader) SortedPostings(p index.Postings) index.Postings {
	// This will already be sorted from the Postings() call above.
	return p
}

func (ir *OOOCompactionHeadIndexReader) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	return ir.ch.oooIR.ShardedPostings(p, shardIndex, shardCount)
}

func (ir *OOOCompactionHeadIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	return ir.ch.oooIR.series(ref, builder, chks, ir.ch.lastMmapRef)
}

func (ir *OOOCompactionHeadIndexReader) SortedLabelValues(_ context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelValues(_ context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) PostingsForMatchers(_ context.Context, concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelNames(context.Context, ...*labels.Matcher) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelValueFor(context.Context, storage.SeriesRef, string) (string, error) {
	return "", errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) LabelNamesFor(ctx context.Context, ids ...storage.SeriesRef) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (ir *OOOCompactionHeadIndexReader) Close() error {
	return ir.ch.oooIR.Close()
}
