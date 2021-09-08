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
	"sort"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/pkg/labels"
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
func (h *headIndexReader) SortedLabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	values, err := h.LabelValues(name, matchers...)
	if err == nil {
		sort.Strings(values)
	}
	return values, err
}

// LabelValues returns label values present in the head for the
// specific label name that are within the time range mint to maxt.
// If matchers are specified the returned result set is reduced
// to label values of metrics matching the matchers.
func (h *headIndexReader) LabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	if h.maxt < h.head.MinTime() || h.mint > h.head.MaxTime() {
		return []string{}, nil
	}

	if len(matchers) == 0 {
		return h.head.postings.LabelValues(name), nil
	}

	return labelValuesWithMatchers(h, name, matchers...)
}

// LabelNames returns all the unique label names present in the head
// that are within the time range mint to maxt.
func (h *headIndexReader) LabelNames(matchers ...*labels.Matcher) ([]string, error) {
	if h.maxt < h.head.MinTime() || h.mint > h.head.MaxTime() {
		return []string{}, nil
	}

	if len(matchers) == 0 {
		labelNames := h.head.postings.LabelNames()
		sort.Strings(labelNames)
		return labelNames, nil
	}

	return labelNamesWithMatchers(h, matchers...)
}

// Postings returns the postings list iterator for the label pairs.
func (h *headIndexReader) Postings(name string, values ...string) (index.Postings, error) {
	res := make([]index.Postings, 0, len(values))
	for _, value := range values {
		res = append(res, h.head.postings.Get(name, value))
	}
	return index.Merge(res...), nil
}

func (h *headIndexReader) SortedPostings(p index.Postings) index.Postings {
	series := make([]*memSeries, 0, 128)

	// Fetch all the series only once.
	for p.Next() {
		s := h.head.series.getByID(p.At())
		if s == nil {
			level.Debug(h.head.logger).Log("msg", "Looked up series not found")
		} else {
			series = append(series, s)
		}
	}
	if err := p.Err(); err != nil {
		return index.ErrPostings(errors.Wrap(err, "expand postings"))
	}

	sort.Slice(series, func(i, j int) bool {
		return labels.Compare(series[i].lset, series[j].lset) < 0
	})

	// Convert back to list.
	ep := make([]uint64, 0, len(series))
	for _, p := range series {
		ep = append(ep, p.ref)
	}
	return index.NewListPostings(ep)
}

// Series returns the series for the given reference.
func (h *headIndexReader) Series(ref uint64, lbls *labels.Labels, chks *[]chunks.Meta) error {
	s := h.head.series.getByID(ref)

	if s == nil {
		h.head.metrics.seriesNotFound.Inc()
		return storage.ErrNotFound
	}
	*lbls = append((*lbls)[:0], s.lset...)

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
			Ref:     packChunkID(s.ref, uint64(s.chunkID(i))),
		})
	}
	if s.headChunk != nil && s.headChunk.OverlapsClosedInterval(h.mint, h.maxt) {
		*chks = append(*chks, chunks.Meta{
			MinTime: s.headChunk.minTime,
			MaxTime: math.MaxInt64, // Set the head chunks as open (being appended to).
			Ref:     packChunkID(s.ref, uint64(s.chunkID(len(s.mmappedChunks)))),
		})
	}

	return nil
}

func (s *memSeries) chunkID(pos int) int {
	return pos + s.firstChunkID
}

// LabelValueFor returns label value for the given label name in the series referred to by ID.
func (h *headIndexReader) LabelValueFor(id uint64, label string) (string, error) {
	memSeries := h.head.series.getByID(id)
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
func (h *headIndexReader) LabelNamesFor(ids ...uint64) ([]string, error) {
	namesMap := make(map[string]struct{})
	for _, id := range ids {
		memSeries := h.head.series.getByID(id)
		if memSeries == nil {
			return nil, storage.ErrNotFound
		}
		for _, lbl := range memSeries.lset {
			namesMap[lbl.Name] = struct{}{}
		}
	}
	names := make([]string, 0, len(namesMap))
	for name := range namesMap {
		names = append(names, name)
	}
	sort.Strings(names)
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
	h.isoState.Close()
	return nil
}

// packChunkID packs a seriesID and a chunkID within it into a global 8 byte ID.
// It panicks if the seriesID exceeds 5 bytes or the chunk ID 3 bytes.
func packChunkID(seriesID, chunkID uint64) uint64 {
	if seriesID > (1<<40)-1 {
		panic("series ID exceeds 5 bytes")
	}
	if chunkID > (1<<24)-1 {
		panic("chunk ID exceeds 3 bytes")
	}
	return (seriesID << 24) | chunkID
}

func unpackChunkID(id uint64) (seriesID, chunkID uint64) {
	return id >> 24, (id << 40) >> 40
}

// Chunk returns the chunk for the reference number.
func (h *headChunkReader) Chunk(ref uint64) (chunkenc.Chunk, error) {
	sid, cid := unpackChunkID(ref)

	s := h.head.series.getByID(sid)
	// This means that the series has been garbage collected.
	if s == nil {
		return nil, storage.ErrNotFound
	}

	s.Lock()
	c, garbageCollect, err := s.chunk(int(cid), h.head.chunkDiskMapper)
	if err != nil {
		s.Unlock()
		return nil, err
	}
	defer func() {
		if garbageCollect {
			// Set this to nil so that Go GC can collect it after it has been used.
			c.chunk = nil
			s.memChunkPool.Put(c)
		}
	}()

	// This means that the chunk is outside the specified range.
	if !c.OverlapsClosedInterval(h.mint, h.maxt) {
		s.Unlock()
		return nil, storage.ErrNotFound
	}
	s.Unlock()

	return &safeChunk{
		Chunk:           c.chunk,
		s:               s,
		cid:             int(cid),
		isoState:        h.isoState,
		chunkDiskMapper: h.head.chunkDiskMapper,
	}, nil
}

// chunk returns the chunk for the chunk id from memory or by m-mapping it from the disk.
// If garbageCollect is true, it means that the returned *memChunk
// (and not the chunkenc.Chunk inside it) can be garbage collected after it's usage.
func (s *memSeries) chunk(id int, chunkDiskMapper *chunks.ChunkDiskMapper) (chunk *memChunk, garbageCollect bool, err error) {
	// ix represents the index of chunk in the s.mmappedChunks slice. The chunk id's are
	// incremented by 1 when new chunk is created, hence (id - firstChunkID) gives the slice index.
	// The max index for the s.mmappedChunks slice can be len(s.mmappedChunks)-1, hence if the ix
	// is len(s.mmappedChunks), it represents the next chunk, which is the head chunk.
	ix := id - s.firstChunkID
	if ix < 0 || ix > len(s.mmappedChunks) {
		return nil, false, storage.ErrNotFound
	}
	if ix == len(s.mmappedChunks) {
		if s.headChunk == nil {
			return nil, false, errors.New("invalid head chunk")
		}
		return s.headChunk, false, nil
	}
	chk, err := chunkDiskMapper.Chunk(s.mmappedChunks[ix].ref)
	if err != nil {
		if _, ok := err.(*chunks.CorruptionErr); ok {
			panic(err)
		}
		return nil, false, err
	}
	mc := s.memChunkPool.Get().(*memChunk)
	mc.chunk = chk
	mc.minTime = s.mmappedChunks[ix].minTime
	mc.maxTime = s.mmappedChunks[ix].maxTime
	return mc, true, nil
}

type safeChunk struct {
	chunkenc.Chunk
	s               *memSeries
	cid             int
	isoState        *isolationState
	chunkDiskMapper *chunks.ChunkDiskMapper
}

func (c *safeChunk) Iterator(reuseIter chunkenc.Iterator) chunkenc.Iterator {
	c.s.Lock()
	it := c.s.iterator(c.cid, c.isoState, c.chunkDiskMapper, reuseIter)
	c.s.Unlock()
	return it
}

// iterator returns a chunk iterator.
// It is unsafe to call this concurrently with s.append(...) without holding the series lock.
func (s *memSeries) iterator(id int, isoState *isolationState, chunkDiskMapper *chunks.ChunkDiskMapper, it chunkenc.Iterator) chunkenc.Iterator {
	c, garbageCollect, err := s.chunk(id, chunkDiskMapper)
	// TODO(fabxc): Work around! An error will be returns when a querier have retrieved a pointer to a
	// series's chunk, which got then garbage collected before it got
	// accessed.  We must ensure to not garbage collect as long as any
	// readers still hold a reference.
	if err != nil {
		return chunkenc.NewNopIterator()
	}
	defer func() {
		if garbageCollect {
			// Set this to nil so that Go GC can collect it after it has been used.
			// This should be done always at the end.
			c.chunk = nil
			s.memChunkPool.Put(c)
		}
	}()

	ix := id - s.firstChunkID

	numSamples := c.chunk.NumSamples()
	stopAfter := numSamples

	if isoState != nil {
		totalSamples := 0    // Total samples in this series.
		previousSamples := 0 // Samples before this chunk.

		for j, d := range s.mmappedChunks {
			totalSamples += int(d.numSamples)
			if j < ix {
				previousSamples += int(d.numSamples)
			}
		}

		if s.headChunk != nil {
			totalSamples += s.headChunk.chunk.NumSamples()
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

	if id-s.firstChunkID < len(s.mmappedChunks) {
		if stopAfter == numSamples {
			return c.chunk.Iterator(it)
		}
		if msIter, ok := it.(*stopIterator); ok {
			msIter.Iterator = c.chunk.Iterator(msIter.Iterator)
			msIter.i = -1
			msIter.stopAfter = stopAfter
			return msIter
		}
		return &stopIterator{
			Iterator:  c.chunk.Iterator(it),
			i:         -1,
			stopAfter: stopAfter,
		}
	}
	// Serve the last 4 samples for the last chunk from the sample buffer
	// as their compressed bytes may be mutated by added samples.
	if msIter, ok := it.(*memSafeIterator); ok {
		msIter.Iterator = c.chunk.Iterator(msIter.Iterator)
		msIter.i = -1
		msIter.total = numSamples
		msIter.stopAfter = stopAfter
		msIter.buf = s.sampleBuf
		return msIter
	}
	return &memSafeIterator{
		stopIterator: stopIterator{
			Iterator:  c.chunk.Iterator(it),
			i:         -1,
			stopAfter: stopAfter,
		},
		total: numSamples,
		buf:   s.sampleBuf,
	}
}

type memSafeIterator struct {
	stopIterator

	total int
	buf   [4]sample
}

func (it *memSafeIterator) Seek(t int64) bool {
	if it.Err() != nil {
		return false
	}

	ts, _ := it.At()

	for t > ts || it.i == -1 {
		if !it.Next() {
			return false
		}
		ts, _ = it.At()
	}

	return true
}

func (it *memSafeIterator) Next() bool {
	if it.i+1 >= it.stopAfter {
		return false
	}
	it.i++
	if it.total-it.i > 4 {
		return it.Iterator.Next()
	}
	return true
}

func (it *memSafeIterator) At() (int64, float64) {
	if it.total-it.i > 4 {
		return it.Iterator.At()
	}
	s := it.buf[4-(it.total-it.i)]
	return s.t, s.v
}

type stopIterator struct {
	chunkenc.Iterator

	i, stopAfter int
}

func (it *stopIterator) Next() bool {
	if it.i+1 >= it.stopAfter {
		return false
	}
	it.i++
	return it.Iterator.Next()
}
