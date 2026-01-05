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
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

func (h *Head) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	return h.exemplars.ExemplarQuerier(ctx)
}

// Index returns an IndexReader against the block.
func (h *Head) Index() (IndexReader, error) {
	return h.indexRange(math.MinInt64, math.MaxInt64), nil
}

func (h *Head) indexRange(mint, maxt int64) *headIndexReader {
	if hmin := h.MinTime(); hmin > mint {
		mint = hmin
	}
	return &headIndexReader{head: h, mint: mint, maxt: maxt}
}

type headIndexReader struct {
	head       *Head
	mint, maxt int64
}

func (*headIndexReader) Close() error {
	return nil
}

func (h *headIndexReader) Symbols() index.StringIter {
	return h.head.postings.Symbols()
}

// SortedLabelValues returns label values present in the head for the
// specific label name that are within the time range mint to maxt.
// If matchers are specified the returned result set is reduced
// to label values of metrics matching the matchers.
func (h *headIndexReader) SortedLabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error) {
	values, err := h.LabelValues(ctx, name, hints, matchers...)
	if err == nil {
		slices.Sort(values)
	}
	return values, err
}

// LabelValues returns label values present in the head for the
// specific label name that are within the time range mint to maxt.
// If matchers are specified the returned result set is reduced
// to label values of metrics matching the matchers.
func (h *headIndexReader) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error) {
	if h.maxt < h.head.MinTime() || h.mint > h.head.MaxTime() {
		return []string{}, nil
	}

	if len(matchers) == 0 {
		return h.head.postings.LabelValues(ctx, name, hints), nil
	}

	return labelValuesWithMatchers(ctx, h, name, hints, matchers...)
}

// LabelNames returns all the unique label names present in the head
// that are within the time range mint to maxt.
func (h *headIndexReader) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, error) {
	if h.maxt < h.head.MinTime() || h.mint > h.head.MaxTime() {
		return []string{}, nil
	}

	if len(matchers) == 0 {
		labelNames := h.head.postings.LabelNames()
		slices.Sort(labelNames)
		return labelNames, nil
	}

	return labelNamesWithMatchers(ctx, h, matchers...)
}

// Postings returns the postings list iterator for the label pairs.
func (h *headIndexReader) Postings(ctx context.Context, name string, values ...string) (index.Postings, error) {
	return h.head.postings.Postings(ctx, name, values...), nil
}

func (h *headIndexReader) PostingsForLabelMatching(ctx context.Context, name string, match func(string) bool) index.Postings {
	return h.head.postings.PostingsForLabelMatching(ctx, name, match)
}

func (h *headIndexReader) PostingsForAllLabelValues(ctx context.Context, name string) index.Postings {
	return h.head.postings.PostingsForAllLabelValues(ctx, name)
}

func (h *headIndexReader) SortedPostings(p index.Postings) index.Postings {
	series := make([]*memSeries, 0, 128)

	notFoundSeriesCount := 0
	// Fetch all the series only once.
	for p.Next() {
		s := h.head.series.getByID(chunks.HeadSeriesRef(p.At()))
		if s == nil {
			notFoundSeriesCount++
		} else {
			series = append(series, s)
		}
	}
	if notFoundSeriesCount > 0 {
		h.head.logger.Debug("Looked up series not found", "count", notFoundSeriesCount)
	}
	if err := p.Err(); err != nil {
		return index.ErrPostings(fmt.Errorf("expand postings: %w", err))
	}

	slices.SortFunc(series, func(a, b *memSeries) int {
		return labels.Compare(a.labels(), b.labels())
	})

	// Convert back to list.
	ep := make([]storage.SeriesRef, 0, len(series))
	for _, p := range series {
		ep = append(ep, storage.SeriesRef(p.ref))
	}
	return index.NewListPostings(ep)
}

// ShardedPostings implements IndexReader. This function returns an failing postings list if sharding
// has not been enabled in the Head.
func (h *headIndexReader) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	if !h.head.opts.EnableSharding {
		return index.ErrPostings(errors.New("sharding is disabled"))
	}

	out := make([]storage.SeriesRef, 0, 128)
	notFoundSeriesCount := 0

	for p.Next() {
		s := h.head.series.getByID(chunks.HeadSeriesRef(p.At()))
		if s == nil {
			notFoundSeriesCount++
			continue
		}

		// Check if the series belong to the shard.
		if s.shardHash%shardCount != shardIndex {
			continue
		}

		out = append(out, storage.SeriesRef(s.ref))
	}
	if notFoundSeriesCount > 0 {
		h.head.logger.Debug("Looked up series not found", "count", notFoundSeriesCount)
	}

	return index.NewListPostings(out)
}

// Series returns the series for the given reference.
// Chunks are skipped if chks is nil.
func (h *headIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	s := h.head.series.getByID(chunks.HeadSeriesRef(ref))

	if s == nil {
		h.head.metrics.seriesNotFound.Inc()
		return storage.ErrNotFound
	}
	builder.Assign(s.labels())

	if chks == nil {
		return nil
	}

	s.Lock()
	defer s.Unlock()

	*chks = (*chks)[:0]
	*chks = appendSeriesChunks(s, h.mint, h.maxt, *chks)

	return nil
}

func appendSeriesChunks(s *memSeries, mint, maxt int64, chks []chunks.Meta) []chunks.Meta {
	for i, c := range s.mmappedChunks {
		// Do not expose chunks that are outside of the specified range.
		if !c.OverlapsClosedInterval(mint, maxt) {
			continue
		}
		chks = append(chks, chunks.Meta{
			MinTime: c.minTime,
			MaxTime: c.maxTime,
			Ref:     chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.headChunkID(i))),
		})
	}

	if s.headChunks != nil {
		var maxTime int64
		var i, j int
		for i = s.headChunks.len() - 1; i >= 0; i-- {
			chk := s.headChunks.atOffset(i)
			if i == 0 {
				// Set the head chunk as open (being appended to) for the first headChunk.
				maxTime = math.MaxInt64
			} else {
				maxTime = chk.maxTime
			}
			if chk.OverlapsClosedInterval(mint, maxt) {
				chks = append(chks, chunks.Meta{
					MinTime: chk.minTime,
					MaxTime: maxTime,
					Ref:     chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.headChunkID(len(s.mmappedChunks)+j))),
				})
			}
			j++
		}
	}
	return chks
}

// headChunkID returns the HeadChunkID referred to by the given position.
// * 0 <= pos < len(s.mmappedChunks) refer to s.mmappedChunks[pos]
// * pos >= len(s.mmappedChunks) refers to s.headChunks linked list.
func (s *memSeries) headChunkID(pos int) chunks.HeadChunkID {
	return chunks.HeadChunkID(pos) + s.firstChunkID
}

const oooChunkIDMask = 1 << 23

// oooHeadChunkID returns the HeadChunkID referred to by the given position.
// Only the bottom 24 bits are used. Bit 23 is always 1 for an OOO chunk; for the rest:
// * 0 <= pos < len(s.oooMmappedChunks) refer to s.oooMmappedChunks[pos]
// * pos == len(s.oooMmappedChunks) refers to s.oooHeadChunk
// The caller must ensure that s.ooo is not nil.
func (s *memSeries) oooHeadChunkID(pos int) chunks.HeadChunkID {
	return (chunks.HeadChunkID(pos) + s.ooo.firstOOOChunkID) | oooChunkIDMask
}

func unpackHeadChunkRef(ref chunks.ChunkRef) (seriesID chunks.HeadSeriesRef, chunkID chunks.HeadChunkID, isOOO bool) {
	sid, cid := chunks.HeadChunkRef(ref).Unpack()
	return sid, (cid & (oooChunkIDMask - 1)), (cid & oooChunkIDMask) != 0
}

// LabelNamesFor returns all the label names for the series referred to by the postings.
// The names returned are sorted.
func (h *headIndexReader) LabelNamesFor(ctx context.Context, series index.Postings) ([]string, error) {
	namesMap := make(map[string]struct{})
	i := 0
	for series.Next() {
		i++
		if i%checkContextEveryNIterations == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		memSeries := h.head.series.getByID(chunks.HeadSeriesRef(series.At()))
		if memSeries == nil {
			// Series not found, this happens during compaction,
			// when series was garbage collected after the caller got the series IDs.
			continue
		}
		memSeries.labels().Range(func(lbl labels.Label) {
			namesMap[lbl.Name] = struct{}{}
		})
	}
	if err := series.Err(); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(namesMap))
	for name := range namesMap {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

// Chunks returns a ChunkReader against the block.
func (h *Head) Chunks() (ChunkReader, error) {
	return h.chunksRange(math.MinInt64, math.MaxInt64, h.iso.State(math.MinInt64, math.MaxInt64))
}

func (h *Head) chunksRange(mint, maxt int64, is *isolationState) (*headChunkReader, error) {
	h.closedMtx.Lock()
	defer h.closedMtx.Unlock()
	if h.closed {
		return nil, errors.New("can't read from a closed head")
	}
	if hmin := h.MinTime(); hmin > mint {
		mint = hmin
	}
	return &headChunkReader{
		head:     h,
		mint:     mint,
		maxt:     maxt,
		isoState: is,
	}, nil
}

type headChunkReader struct {
	head       *Head
	mint, maxt int64
	isoState   *isolationState
}

func (h *headChunkReader) Close() error {
	if h.isoState != nil {
		h.isoState.Close()
	}
	return nil
}

// ChunkOrIterable returns the chunk for the reference number.
func (h *headChunkReader) ChunkOrIterable(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	chk, _, err := h.chunk(meta, false)
	return chk, nil, err
}

type ChunkReaderWithCopy interface {
	ChunkOrIterableWithCopy(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, int64, error)
}

// ChunkOrIterableWithCopy returns the chunk for the reference number.
// If the chunk is the in-memory chunk, then it makes a copy and returns the copied chunk, plus the max time of the chunk.
func (h *headChunkReader) ChunkOrIterableWithCopy(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, int64, error) {
	chk, maxTime, err := h.chunk(meta, true)
	return chk, nil, maxTime, err
}

// chunk returns the chunk for the reference number.
// If copyLastChunk is true, then it makes a copy of the head chunk if asked for it.
// Also returns max time of the chunk.
func (h *headChunkReader) chunk(meta chunks.Meta, copyLastChunk bool) (chunkenc.Chunk, int64, error) {
	sid, cid, isOOO := unpackHeadChunkRef(meta.Ref)

	s := h.head.series.getByID(sid)
	// This means that the series has been garbage collected.
	if s == nil {
		return nil, 0, storage.ErrNotFound
	}

	s.Lock()
	defer s.Unlock()
	return h.head.chunkFromSeries(s, cid, isOOO, h.mint, h.maxt, h.isoState, copyLastChunk)
}

// Dumb thing to defeat chunk pool.
type wrapOOOHeadChunk struct {
	chunkenc.Chunk
}

// Call with s locked.
func (h *Head) chunkFromSeries(s *memSeries, cid chunks.HeadChunkID, isOOO bool, mint, maxt int64, isoState *isolationState, copyLastChunk bool) (chunkenc.Chunk, int64, error) {
	if isOOO {
		chk, maxTime, err := s.oooChunk(cid, h.chunkDiskMapper, &h.memChunkPool)
		return wrapOOOHeadChunk{chk}, maxTime, err
	}
	c, headChunk, isOpen, err := s.chunk(cid, h.chunkDiskMapper, &h.memChunkPool)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if !headChunk {
			// Set this to nil so that Go GC can collect it after it has been used.
			c.chunk = nil
			c.prev = nil
			h.memChunkPool.Put(c)
		}
	}()

	// This means that the chunk is outside the specified range.
	if !c.OverlapsClosedInterval(mint, maxt) {
		return nil, 0, storage.ErrNotFound
	}

	chk, maxTime := c.chunk, c.maxTime
	if headChunk && isOpen && copyLastChunk {
		// The caller may ask to copy the head chunk in order to take the
		// bytes of the chunk without causing the race between read and append.
		b := s.headChunks.chunk.Bytes()
		newB := make([]byte, len(b))
		copy(newB, b) // TODO(codesome): Use bytes.Clone() when we upgrade to Go 1.20.
		// TODO(codesome): Put back in the pool (non-trivial).
		chk, err = h.opts.ChunkPool.Get(s.headChunks.chunk.Encoding(), newB)
		if err != nil {
			return nil, 0, err
		}
	}

	return &safeHeadChunk{
		Chunk:    chk,
		s:        s,
		cid:      cid,
		isoState: isoState,
	}, maxTime, nil
}

// chunk returns the chunk for the HeadChunkID from memory or by m-mapping it from the disk.
// If headChunk is false, it means that the returned *memChunk
// (and not the chunkenc.Chunk inside it) can be garbage collected after its usage.
// if isOpen is true, it means that the returned *memChunk is used for appends.
func (s *memSeries) chunk(id chunks.HeadChunkID, chunkDiskMapper *chunks.ChunkDiskMapper, memChunkPool *sync.Pool) (chunk *memChunk, headChunk, isOpen bool, err error) {
	// ix represents the index of chunk in the s.mmappedChunks slice. The chunk id's are
	// incremented by 1 when new chunk is created, hence (id - firstChunkID) gives the slice index.
	// The max index for the s.mmappedChunks slice can be len(s.mmappedChunks)-1, hence if the ix
	// is >= len(s.mmappedChunks), it represents one of the chunks on s.headChunks linked list.
	// The order of elements is different for slice and linked list.
	// For s.mmappedChunks slice newer chunks are appended to it.
	// For s.headChunks list newer chunks are prepended to it.
	//
	// memSeries {
	//   mmappedChunks: [t0, t1, t2]
	//   headChunk:     {t5}->{t4}->{t3}
	// }
	ix := int(id) - int(s.firstChunkID)

	var headChunksLen int
	if s.headChunks != nil {
		headChunksLen = s.headChunks.len()
	}

	if ix < 0 || ix > len(s.mmappedChunks)+headChunksLen-1 {
		return nil, false, false, storage.ErrNotFound
	}

	if ix < len(s.mmappedChunks) {
		chk, err := chunkDiskMapper.Chunk(s.mmappedChunks[ix].ref)
		if err != nil {
			var cerr *chunks.CorruptionErr
			if errors.As(err, &cerr) {
				panic(err)
			}
			return nil, false, false, err
		}
		mc := memChunkPool.Get().(*memChunk)
		mc.chunk = chk
		mc.minTime = s.mmappedChunks[ix].minTime
		mc.maxTime = s.mmappedChunks[ix].maxTime
		return mc, false, false, nil
	}

	ix -= len(s.mmappedChunks)

	offset := headChunksLen - ix - 1
	// headChunks is a linked list where first element is the most recent one and the last one is the oldest.
	// This order is reversed when compared with mmappedChunks, since mmappedChunks[0] is the oldest chunk,
	// while headChunk.atOffset(0) would give us the most recent chunk.
	// So when calling headChunk.atOffset() we need to reverse the value of ix.
	elem := s.headChunks.atOffset(offset)
	if elem == nil {
		// This should never really happen and would mean that headChunksLen value is NOT equal
		// to the length of the headChunks list.
		return nil, false, false, storage.ErrNotFound
	}
	return elem, true, offset == 0, nil
}

// oooChunk returns the chunk for the HeadChunkID by m-mapping it from the disk.
// It never returns the head OOO chunk.
func (s *memSeries) oooChunk(id chunks.HeadChunkID, chunkDiskMapper *chunks.ChunkDiskMapper, _ *sync.Pool) (chunk chunkenc.Chunk, maxTime int64, err error) {
	// ix represents the index of chunk in the s.ooo.oooMmappedChunks slice. The chunk id's are
	// incremented by 1 when new chunk is created, hence (id - firstOOOChunkID) gives the slice index.
	ix := int(id) - int(s.ooo.firstOOOChunkID)

	if ix < 0 || ix >= len(s.ooo.oooMmappedChunks) {
		return nil, 0, storage.ErrNotFound
	}

	chk, err := chunkDiskMapper.Chunk(s.ooo.oooMmappedChunks[ix].ref)
	return chk, s.ooo.oooMmappedChunks[ix].maxTime, err
}

// safeHeadChunk makes sure that the chunk can be accessed without a race condition.
type safeHeadChunk struct {
	chunkenc.Chunk
	s        *memSeries
	cid      chunks.HeadChunkID
	isoState *isolationState
}

func (c *safeHeadChunk) Iterator(reuseIter chunkenc.Iterator) chunkenc.Iterator {
	c.s.Lock()
	it := c.s.iterator(c.cid, c.Chunk, c.isoState, reuseIter)
	c.s.Unlock()
	return it
}

// iterator returns a chunk iterator for the requested chunkID, or a NopIterator if the requested ID is out of range.
// It is unsafe to call this concurrently with s.append(...) without holding the series lock.
func (s *memSeries) iterator(id chunks.HeadChunkID, c chunkenc.Chunk, isoState *isolationState, it chunkenc.Iterator) chunkenc.Iterator {
	ix := int(id) - int(s.firstChunkID)

	numSamples := c.NumSamples()
	stopAfter := numSamples

	if isoState != nil && !isoState.IsolationDisabled() {
		totalSamples := 0    // Total samples in this series.
		previousSamples := 0 // Samples before this chunk.

		for j, d := range s.mmappedChunks {
			totalSamples += int(d.numSamples)
			if j < ix {
				previousSamples += int(d.numSamples)
			}
		}

		ix -= len(s.mmappedChunks)
		if s.headChunks != nil {
			// Iterate all head chunks from the oldest to the newest.
			headChunksLen := s.headChunks.len()
			for j := headChunksLen - 1; j >= 0; j-- {
				chk := s.headChunks.atOffset(j)
				chkSamples := chk.chunk.NumSamples()
				totalSamples += chkSamples
				// Chunk ID is len(s.mmappedChunks) + $(headChunks list position).
				// Where $(headChunks list position) is zero for the oldest chunk and $(s.headChunks.len() - 1)
				// for the newest (open) chunk.
				if headChunksLen-1-j < ix {
					previousSamples += chkSamples
				}
			}
		}

		// Removing the extra transactionIDs that are relevant for samples that
		// come after this chunk, from the total transactionIDs.
		appendIDsToConsider := int(s.txs.txIDCount) - (totalSamples - (previousSamples + numSamples))

		// Iterate over the appendIDs, find the first one that the isolation state says not
		// to return.
		it := s.txs.iterator()
		for index := range appendIDsToConsider {
			appendID := it.At()
			if appendID <= isoState.maxAppendID { // Easy check first.
				if _, ok := isoState.incompleteAppends[appendID]; !ok {
					it.Next()
					continue
				}
			}
			// Stopped in a previous chunk.
			stopAfter = max(numSamples-(appendIDsToConsider-index), 0)
			break
		}
	}

	if stopAfter == 0 {
		return chunkenc.NewNopIterator()
	}
	if stopAfter == numSamples {
		return c.Iterator(it)
	}
	return makeStopIterator(c, it, stopAfter)
}

// stopIterator wraps an Iterator, but only returns the first
// stopAfter values, if initialized with i=-1.
type stopIterator struct {
	chunkenc.Iterator

	i, stopAfter int
}

func (it *stopIterator) Next() chunkenc.ValueType {
	if it.i+1 >= it.stopAfter {
		return chunkenc.ValNone
	}
	it.i++
	return it.Iterator.Next()
}

func makeStopIterator(c chunkenc.Chunk, it chunkenc.Iterator, stopAfter int) chunkenc.Iterator {
	// Re-use the Iterator object if it is a stopIterator.
	if stopIter, ok := it.(*stopIterator); ok {
		stopIter.Iterator = c.Iterator(stopIter.Iterator)
		stopIter.i = -1
		stopIter.stopAfter = stopAfter
		return stopIter
	}

	return &stopIterator{
		Iterator:  c.Iterator(it),
		i:         -1,
		stopAfter: stopAfter,
	}
}
