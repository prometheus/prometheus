// Copyright 2021 The Prometheus Authors
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
	"math"
	"sync"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"

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

func (h *headIndexReader) Close() error {
	return nil
}

func (h *headIndexReader) Symbols() index.StringIter {
	return h.head.postings.Symbols()
}

// SortedLabelValues returns label values present in the head for the
// specific label name that are within the time range mint to maxt.
// If matchers are specified the returned result set is reduced
// to label values of metrics matching the matchers.
func (h *headIndexReader) SortedLabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	values, err := h.LabelValues(ctx, name, matchers...)
	if err == nil {
		slices.Sort(values)
	}
	return values, err
}

// LabelValues returns label values present in the head for the
// specific label name that are within the time range mint to maxt.
// If matchers are specified the returned result set is reduced
// to label values of metrics matching the matchers.
func (h *headIndexReader) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	if h.maxt < h.head.MinTime() || h.mint > h.head.MaxTime() {
		return []string{}, nil
	}

	if len(matchers) == 0 {
		return h.head.postings.LabelValues(ctx, name), nil
	}

	return labelValuesWithMatchers(ctx, h, name, matchers...)
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
	switch len(values) {
	case 0:
		return index.EmptyPostings(), nil
	case 1:
		return h.head.postings.Get(name, values[0]), nil
	default:
		res := make([]index.Postings, 0, len(values))
		for _, value := range values {
			if p := h.head.postings.Get(name, value); !index.IsEmptyPostingsType(p) {
				res = append(res, p)
			}
		}
		return index.Merge(ctx, res...), nil
	}
}

func (h *headIndexReader) PostingsForMatchers(ctx context.Context, concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	return h.head.pfmc.PostingsForMatchers(ctx, h, concurrent, ms...)
}

func (h *headIndexReader) SortedPostings(p index.Postings) index.Postings {
	series := make([]*memSeries, 0, 128)

	// Fetch all the series only once.
	for p.Next() {
		s := h.head.series.getByID(chunks.HeadSeriesRef(p.At()))
		if s == nil {
			level.Debug(h.head.logger).Log("msg", "Looked up series not found")
		} else {
			series = append(series, s)
		}
	}
	if err := p.Err(); err != nil {
		return index.ErrPostings(errors.Wrap(err, "expand postings"))
	}

	slices.SortFunc(series, func(a, b *memSeries) int {
		return labels.Compare(a.lset, b.lset)
	})

	// Convert back to list.
	ep := make([]storage.SeriesRef, 0, len(series))
	for _, p := range series {
		ep = append(ep, storage.SeriesRef(p.ref))
	}
	return index.NewListPostings(ep)
}

func (h *headIndexReader) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	out := make([]storage.SeriesRef, 0, 128)

	for p.Next() {
		s := h.head.series.getByID(chunks.HeadSeriesRef(p.At()))
		if s == nil {
			level.Debug(h.head.logger).Log("msg", "Looked up series not found")
			continue
		}

		// Check if the series belong to the shard.
		if s.shardHash%shardCount != shardIndex {
			continue
		}

		out = append(out, storage.SeriesRef(s.ref))
	}

	return index.NewListPostings(out)
}

// Series returns the series for the given reference.
func (h *headIndexReader) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	s := h.head.series.getByID(chunks.HeadSeriesRef(ref))

	if s == nil {
		h.head.metrics.seriesNotFound.Inc()
		return storage.ErrNotFound
	}
	builder.Assign(s.lset)

	if chks == nil {
		return nil
	}

	s.Lock()
	defer s.Unlock()

	*chks = (*chks)[:0]

	for i, c := range s.mmappedChunks {
		// Do not expose chunks that are outside of the specified range.
		if !c.OverlapsClosedInterval(h.mint, h.maxt) {
			continue
		}
		*chks = append(*chks, chunks.Meta{
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
			if chk.OverlapsClosedInterval(h.mint, h.maxt) {
				*chks = append(*chks, chunks.Meta{
					MinTime: chk.minTime,
					MaxTime: maxTime,
					Ref:     chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.headChunkID(len(s.mmappedChunks)+j))),
				})
			}
			j++
		}
	}

	return nil
}

// headChunkID returns the HeadChunkID referred to by the given position.
// * 0 <= pos < len(s.mmappedChunks) refer to s.mmappedChunks[pos]
// * pos >= len(s.mmappedChunks) refers to s.headChunks linked list
func (s *memSeries) headChunkID(pos int) chunks.HeadChunkID {
	return chunks.HeadChunkID(pos) + s.firstChunkID
}

// oooHeadChunkID returns the HeadChunkID referred to by the given position.
// * 0 <= pos < len(s.oooMmappedChunks) refer to s.oooMmappedChunks[pos]
// * pos == len(s.oooMmappedChunks) refers to s.oooHeadChunk
// The caller must ensure that s.ooo is not nil.
func (s *memSeries) oooHeadChunkID(pos int) chunks.HeadChunkID {
	return chunks.HeadChunkID(pos) + s.ooo.firstOOOChunkID
}

// LabelValueFor returns label value for the given label name in the series referred to by ID.
func (h *headIndexReader) LabelValueFor(_ context.Context, id storage.SeriesRef, label string) (string, error) {
	memSeries := h.head.series.getByID(chunks.HeadSeriesRef(id))
	if memSeries == nil {
		return "", storage.ErrNotFound
	}

	value := memSeries.lset.Get(label)
	if value == "" {
		return "", storage.ErrNotFound
	}

	return value, nil
}

// LabelNamesFor returns all the label names for the series referred to by IDs.
// The names returned are sorted.
func (h *headIndexReader) LabelNamesFor(ctx context.Context, ids ...storage.SeriesRef) ([]string, error) {
	namesMap := make(map[string]struct{})
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		memSeries := h.head.series.getByID(chunks.HeadSeriesRef(id))
		if memSeries == nil {
			return nil, storage.ErrNotFound
		}
		memSeries.lset.Range(func(lbl labels.Label) {
			namesMap[lbl.Name] = struct{}{}
		})
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

// Chunk returns the chunk for the reference number.
func (h *headChunkReader) Chunk(meta chunks.Meta) (chunkenc.Chunk, error) {
	chk, _, err := h.chunk(meta, false)
	return chk, err
}

// ChunkWithCopy returns the chunk for the reference number.
// If the chunk is the in-memory chunk, then it makes a copy and returns the copied chunk.
func (h *headChunkReader) ChunkWithCopy(meta chunks.Meta) (chunkenc.Chunk, int64, error) {
	return h.chunk(meta, true)
}

// chunk returns the chunk for the reference number.
// If copyLastChunk is true, then it makes a copy of the head chunk if asked for it.
// Also returns max time of the chunk.
func (h *headChunkReader) chunk(meta chunks.Meta, copyLastChunk bool) (chunkenc.Chunk, int64, error) {
	sid, cid := chunks.HeadChunkRef(meta.Ref).Unpack()

	s := h.head.series.getByID(sid)
	// This means that the series has been garbage collected.
	if s == nil {
		return nil, 0, storage.ErrNotFound
	}

	s.Lock()
	c, headChunk, isOpen, err := s.chunk(cid, h.head.chunkDiskMapper, &h.head.memChunkPool)
	if err != nil {
		s.Unlock()
		return nil, 0, err
	}
	defer func() {
		if !headChunk {
			// Set this to nil so that Go GC can collect it after it has been used.
			c.chunk = nil
			c.prev = nil
			h.head.memChunkPool.Put(c)
		}
	}()

	// This means that the chunk is outside the specified range.
	if !c.OverlapsClosedInterval(h.mint, h.maxt) {
		s.Unlock()
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
		chk, err = h.head.opts.ChunkPool.Get(s.headChunks.chunk.Encoding(), newB)
		if err != nil {
			return nil, 0, err
		}
	}
	s.Unlock()

	return &safeHeadChunk{
		Chunk:    chk,
		s:        s,
		cid:      cid,
		isoState: h.isoState,
	}, maxTime, nil
}

// chunk returns the chunk for the HeadChunkID from memory or by m-mapping it from the disk.
// If headChunk is false, it means that the returned *memChunk
// (and not the chunkenc.Chunk inside it) can be garbage collected after its usage.
// if isOpen is true, it means that the returned *memChunk is used for appends.
func (s *memSeries) chunk(id chunks.HeadChunkID, cdm chunkDiskMapper, memChunkPool *sync.Pool) (chunk *memChunk, headChunk, isOpen bool, err error) {
	// ix represents the index of chunk in the s.mmappedChunks slice. The chunk id's are
	// incremented by 1 when new chunk is created, hence (id - firstChunkID) gives the slice index.
	// The max index for the s.mmappedChunks slice can be len(s.mmappedChunks)-1, hence if the ix
	// is >= len(s.mmappedChunks), it represents one of the chunks on s.headChunks linked list.
	// The order of elemens is different for slice and linked list.
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
		chk, err := cdm.Chunk(s.mmappedChunks[ix].ref)
		if err != nil {
			if _, ok := err.(*chunks.CorruptionErr); ok {
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

// oooMergedChunk returns the requested chunk based on the given chunks.Meta
// reference from memory or by m-mapping it from the disk. The returned chunk
// might be a merge of all the overlapping chunks, if any, amongst all the
// chunks in the OOOHead.
// This function is not thread safe unless the caller holds a lock.
// The caller must ensure that s.ooo is not nil.
func (s *memSeries) oooMergedChunk(meta chunks.Meta, cdm chunkDiskMapper, mint, maxt int64) (chunk *mergedOOOChunks, err error) {
	_, cid := chunks.HeadChunkRef(meta.Ref).Unpack()

	// ix represents the index of chunk in the s.mmappedChunks slice. The chunk meta's are
	// incremented by 1 when new chunk is created, hence (meta - firstChunkID) gives the slice index.
	// The max index for the s.mmappedChunks slice can be len(s.mmappedChunks)-1, hence if the ix
	// is len(s.mmappedChunks), it represents the next chunk, which is the head chunk.
	ix := int(cid) - int(s.ooo.firstOOOChunkID)
	if ix < 0 || ix > len(s.ooo.oooMmappedChunks) {
		return nil, storage.ErrNotFound
	}

	if ix == len(s.ooo.oooMmappedChunks) {
		if s.ooo.oooHeadChunk == nil {
			return nil, errors.New("invalid ooo head chunk")
		}
	}

	// We create a temporary slice of chunk metas to hold the information of all
	// possible chunks that may overlap with the requested chunk.
	tmpChks := make([]chunkMetaAndChunkDiskMapperRef, 0, len(s.ooo.oooMmappedChunks))

	oooHeadRef := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(len(s.ooo.oooMmappedChunks))))
	if s.ooo.oooHeadChunk != nil && s.ooo.oooHeadChunk.OverlapsClosedInterval(mint, maxt) {
		// We only want to append the head chunk if this chunk existed when
		// Series() was called. This brings consistency in case new data
		// is added in between Series() and Chunk() calls.
		if oooHeadRef == meta.OOOLastRef {
			tmpChks = append(tmpChks, chunkMetaAndChunkDiskMapperRef{
				meta: chunks.Meta{
					// Ignoring samples added before and after the last known min and max time for this chunk.
					MinTime: meta.OOOLastMinTime,
					MaxTime: meta.OOOLastMaxTime,
					Ref:     oooHeadRef,
				},
			})
		}
	}

	for i, c := range s.ooo.oooMmappedChunks {
		chunkRef := chunks.ChunkRef(chunks.NewHeadChunkRef(s.ref, s.oooHeadChunkID(i)))
		// We can skip chunks that came in later than the last known OOOLastRef.
		if chunkRef > meta.OOOLastRef {
			break
		}

		switch {
		case chunkRef == meta.OOOLastRef:
			tmpChks = append(tmpChks, chunkMetaAndChunkDiskMapperRef{
				meta: chunks.Meta{
					MinTime: meta.OOOLastMinTime,
					MaxTime: meta.OOOLastMaxTime,
					Ref:     chunkRef,
				},
				ref:      c.ref,
				origMinT: c.minTime,
				origMaxT: c.maxTime,
			})
		case c.OverlapsClosedInterval(mint, maxt):
			tmpChks = append(tmpChks, chunkMetaAndChunkDiskMapperRef{
				meta: chunks.Meta{
					MinTime: c.minTime,
					MaxTime: c.maxTime,
					Ref:     chunkRef,
				},
				ref: c.ref,
			})
		}
	}

	// Next we want to sort all the collected chunks by min time so we can find
	// those that overlap and stop when we know the rest don't.
	slices.SortFunc(tmpChks, refLessByMinTimeAndMinRef)

	mc := &mergedOOOChunks{}
	absoluteMax := int64(math.MinInt64)
	for _, c := range tmpChks {
		if c.meta.Ref != meta.Ref && (len(mc.chunks) == 0 || c.meta.MinTime > absoluteMax) {
			continue
		}
		if c.meta.Ref == oooHeadRef {
			var xor *chunkenc.XORChunk
			// If head chunk min and max time match the meta OOO markers
			// that means that the chunk has not expanded so we can append
			// it as it is.
			if s.ooo.oooHeadChunk.minTime == meta.OOOLastMinTime && s.ooo.oooHeadChunk.maxTime == meta.OOOLastMaxTime {
				xor, err = s.ooo.oooHeadChunk.chunk.ToXOR() // TODO(jesus.vazquez) (This is an optimization idea that has no priority and might not be that useful) See if we could use a copy of the underlying slice. That would leave the more expensive ToXOR() function only for the usecase where Bytes() is called.
			} else {
				// We need to remove samples that are outside of the markers
				xor, err = s.ooo.oooHeadChunk.chunk.ToXORBetweenTimestamps(meta.OOOLastMinTime, meta.OOOLastMaxTime)
			}
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert ooo head chunk to xor chunk")
			}
			c.meta.Chunk = xor
		} else {
			chk, err := cdm.Chunk(c.ref)
			if err != nil {
				if _, ok := err.(*chunks.CorruptionErr); ok {
					return nil, errors.Wrap(err, "invalid ooo mmapped chunk")
				}
				return nil, err
			}
			if c.meta.Ref == meta.OOOLastRef &&
				(c.origMinT != meta.OOOLastMinTime || c.origMaxT != meta.OOOLastMaxTime) {
				// The head expanded and was memory mapped so now we need to
				// wrap the chunk within a chunk that doesnt allows us to iterate
				// through samples out of the OOOLastMinT and OOOLastMaxT
				// markers.
				c.meta.Chunk = boundedChunk{chk, meta.OOOLastMinTime, meta.OOOLastMaxTime}
			} else {
				c.meta.Chunk = chk
			}
		}
		mc.chunks = append(mc.chunks, c.meta)
		if c.meta.MaxTime > absoluteMax {
			absoluteMax = c.meta.MaxTime
		}
	}

	return mc, nil
}

var _ chunkenc.Chunk = &mergedOOOChunks{}

// mergedOOOChunks holds the list of overlapping chunks. This struct satisfies
// chunkenc.Chunk.
type mergedOOOChunks struct {
	chunks []chunks.Meta
}

// Bytes is a very expensive method because its calling the iterator of all the
// chunks in the mergedOOOChunk and building a new chunk with the samples.
func (o mergedOOOChunks) Bytes() []byte {
	xc := chunkenc.NewXORChunk()
	app, err := xc.Appender()
	if err != nil {
		panic(err)
	}
	it := o.Iterator(nil)
	for it.Next() == chunkenc.ValFloat {
		t, v := it.At()
		app.Append(t, v)
	}

	return xc.Bytes()
}

func (o mergedOOOChunks) Encoding() chunkenc.Encoding {
	return chunkenc.EncXOR
}

func (o mergedOOOChunks) Appender() (chunkenc.Appender, error) {
	return nil, errors.New("can't append to mergedOOOChunks")
}

func (o mergedOOOChunks) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	return storage.ChainSampleIteratorFromMetas(iterator, o.chunks)
}

func (o mergedOOOChunks) NumSamples() int {
	samples := 0
	for _, c := range o.chunks {
		samples += c.Chunk.NumSamples()
	}
	return samples
}

func (o mergedOOOChunks) Compact() {}

var _ chunkenc.Chunk = &boundedChunk{}

// boundedChunk is an implementation of chunkenc.Chunk that uses a
// boundedIterator that only iterates through samples which timestamps are
// >= minT and <= maxT
type boundedChunk struct {
	chunkenc.Chunk
	minT int64
	maxT int64
}

func (b boundedChunk) Bytes() []byte {
	xor := chunkenc.NewXORChunk()
	a, _ := xor.Appender()
	it := b.Iterator(nil)
	for it.Next() == chunkenc.ValFloat {
		t, v := it.At()
		a.Append(t, v)
	}
	return xor.Bytes()
}

func (b boundedChunk) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	it := b.Chunk.Iterator(iterator)
	if it == nil {
		panic("iterator shouldn't be nil")
	}
	return boundedIterator{it, b.minT, b.maxT}
}

var _ chunkenc.Iterator = &boundedIterator{}

// boundedIterator is an implementation of Iterator that only iterates through
// samples which timestamps are >= minT and <= maxT
type boundedIterator struct {
	chunkenc.Iterator
	minT int64
	maxT int64
}

// Next the first time its called it will advance as many positions as necessary
// until its able to find a sample within the bounds minT and maxT.
// If there are samples within bounds it will advance one by one amongst them.
// If there are no samples within bounds it will return false.
func (b boundedIterator) Next() chunkenc.ValueType {
	for b.Iterator.Next() == chunkenc.ValFloat {
		t, _ := b.Iterator.At()
		switch {
		case t < b.minT:
			continue
		case t > b.maxT:
			return chunkenc.ValNone
		default:
			return chunkenc.ValFloat
		}
	}
	return chunkenc.ValNone
}

func (b boundedIterator) Seek(t int64) chunkenc.ValueType {
	if t < b.minT {
		// We must seek at least up to b.minT if it is asked for something before that.
		val := b.Iterator.Seek(b.minT)
		if !(val == chunkenc.ValFloat) {
			return chunkenc.ValNone
		}
		t, _ := b.Iterator.At()
		if t <= b.maxT {
			return chunkenc.ValFloat
		}
	}
	if t > b.maxT {
		// We seek anyway so that the subsequent Next() calls will also return false.
		b.Iterator.Seek(t)
		return chunkenc.ValNone
	}
	return b.Iterator.Seek(t)
}

// safeHeadChunk makes sure that the chunk can be accessed without a race condition
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
		appendIDsToConsider := s.txs.txIDCount - (totalSamples - (previousSamples + numSamples))

		// Iterate over the appendIDs, find the first one that the isolation state says not
		// to return.
		it := s.txs.iterator()
		for index := 0; index < appendIDsToConsider; index++ {
			appendID := it.At()
			if appendID <= isoState.maxAppendID { // Easy check first.
				if _, ok := isoState.incompleteAppends[appendID]; !ok {
					it.Next()
					continue
				}
			}
			stopAfter = numSamples - (appendIDsToConsider - index)
			if stopAfter < 0 {
				stopAfter = 0 // Stopped in a previous chunk.
			}
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
