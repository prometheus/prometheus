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

package chunkenc

import (
	"encoding/binary"
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

// HistogramChunk holds encoded sample data for a sparse, high-resolution
// histogram.
//
// Each sample has multiple "fields", stored in the following way (raw = store
// number directly, delta = store delta to the previous number, dod = store
// delta of the delta to the previous number, xor = what we do for regular
// sample values):
//
//	field →    ts    count zeroCount sum []posbuckets []negbuckets
//	sample 1   raw   raw   raw       raw []raw        []raw
//	sample 2   delta delta delta     xor []delta      []delta
//	sample >2  dod   dod   dod       xor []dod        []dod
type HistogramChunk struct {
	b bstream
}

// NewHistogramChunk returns a new chunk with histogram encoding of the given
// size.
func NewHistogramChunk() *HistogramChunk {
	b := make([]byte, 3, 128)
	return &HistogramChunk{b: bstream{stream: b, count: 0}}
}

// Encoding returns the encoding type.
func (c *HistogramChunk) Encoding() Encoding {
	return EncHistogram
}

// Bytes returns the underlying byte slice of the chunk.
func (c *HistogramChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *HistogramChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Layout returns the histogram layout. Only call this on chunks that have at
// least one sample.
func (c *HistogramChunk) Layout() (
	schema int32, zeroThreshold float64,
	negativeSpans, positiveSpans []histogram.Span,
	err error,
) {
	if c.NumSamples() == 0 {
		panic("HistogramChunk.Layout() called on an empty chunk")
	}
	b := newBReader(c.Bytes()[2:])
	return readHistogramChunkLayout(&b)
}

// CounterResetHeader defines the first 2 bits of the chunk header.
type CounterResetHeader byte

const (
	// CounterReset means there was definitely a counter reset that resulted in this chunk.
	CounterReset CounterResetHeader = 0b10000000
	// NotCounterReset means there was definitely no counter reset when cutting this chunk.
	NotCounterReset CounterResetHeader = 0b01000000
	// GaugeType means this chunk contains a gauge histogram, where counter resets do not happen.
	GaugeType CounterResetHeader = 0b11000000
	// UnknownCounterReset means we cannot say if this chunk was created due to a counter reset or not.
	// An explicit counter reset detection needs to happen during query time.
	UnknownCounterReset CounterResetHeader = 0b00000000
)

// setCounterResetHeader sets the counter reset header of the chunk
// The third byte of the chunk is the counter reset header.
func setCounterResetHeader(h CounterResetHeader, bytes []byte) {
	switch h {
	case CounterReset, NotCounterReset, GaugeType, UnknownCounterReset:
		bytes[2] = (bytes[2] & 0b00111111) | byte(h)
	default:
		panic("invalid CounterResetHeader type")
	}
}

// SetCounterResetHeader sets the counter reset header.
func (c *HistogramChunk) SetCounterResetHeader(h CounterResetHeader) {
	setCounterResetHeader(h, c.Bytes())
}

// GetCounterResetHeader returns the info about the first 2 bits of the chunk
// header.
func (c *HistogramChunk) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(c.Bytes()[2] & 0b11000000)
}

// Compact implements the Chunk interface.
func (c *HistogramChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *HistogramChunk) Appender() (Appender, error) {
	it := c.iterator(nil)

	// To get an appender, we must know the state it would have if we had
	// appended all existing data from scratch. We iterate through the end
	// and populate via the iterator's state.
	for it.Next() == ValHistogram { // nolint:revive
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	a := &HistogramAppender{
		b: &c.b,

		schema:        it.schema,
		zThreshold:    it.zThreshold,
		pSpans:        it.pSpans,
		nSpans:        it.nSpans,
		t:             it.t,
		cnt:           it.cnt,
		zCnt:          it.zCnt,
		tDelta:        it.tDelta,
		cntDelta:      it.cntDelta,
		zCntDelta:     it.zCntDelta,
		pBuckets:      it.pBuckets,
		nBuckets:      it.nBuckets,
		pBucketsDelta: it.pBucketsDelta,
		nBucketsDelta: it.nBucketsDelta,

		sum:      it.sum,
		leading:  it.leading,
		trailing: it.trailing,
	}
	if it.numTotal == 0 {
		a.leading = 0xff
	}
	return a, nil
}

func countSpans(spans []histogram.Span) int {
	var cnt int
	for _, s := range spans {
		cnt += int(s.Length)
	}
	return cnt
}

func newHistogramIterator(b []byte) *histogramIterator {
	it := &histogramIterator{
		br:       newBReader(b),
		numTotal: binary.BigEndian.Uint16(b),
		t:        math.MinInt64,
	}
	// The first 3 bytes contain chunk headers.
	// We skip that for actual samples.
	_, _ = it.br.readBits(24)
	it.counterResetHeader = CounterResetHeader(b[2] & 0b11000000)
	return it
}

func (c *HistogramChunk) iterator(it Iterator) *histogramIterator {
	// This comment is copied from XORChunk.iterator:
	//   Should iterators guarantee to act on a copy of the data so it doesn't lock append?
	//   When using striped locks to guard access to chunks, probably yes.
	//   Could only copy data if the chunk is not completed yet.
	if histogramIter, ok := it.(*histogramIterator); ok {
		histogramIter.Reset(c.b.bytes())
		return histogramIter
	}
	return newHistogramIterator(c.b.bytes())
}

// Iterator implements the Chunk interface.
func (c *HistogramChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// HistogramAppender is an Appender implementation for sparse histograms.
type HistogramAppender struct {
	b *bstream

	// Layout:
	schema         int32
	zThreshold     float64
	pSpans, nSpans []histogram.Span

	// Although we intend to start new chunks on counter resets, we still
	// have to handle negative deltas for gauge histograms. Therefore, even
	// deltas are signed types here (even for tDelta to not treat that one
	// specially).
	t                            int64
	cnt, zCnt                    uint64
	tDelta, cntDelta, zCntDelta  int64
	pBuckets, nBuckets           []int64
	pBucketsDelta, nBucketsDelta []int64

	// The sum is Gorilla xor encoded.
	sum      float64
	leading  uint8
	trailing uint8
}

func (a *HistogramAppender) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(a.b.bytes()[2] & 0b11000000)
}

func (a *HistogramAppender) NumSamples() int {
	return int(binary.BigEndian.Uint16(a.b.bytes()))
}

// Append implements Appender. This implementation panics because normal float
// samples must never be appended to a histogram chunk.
func (a *HistogramAppender) Append(int64, float64) {
	panic("appended a float sample to a histogram chunk")
}

// AppendFloatHistogram implements Appender. This implementation panics because float
// histogram samples must never be appended to a histogram chunk.
func (a *HistogramAppender) AppendFloatHistogram(int64, *histogram.FloatHistogram) {
	panic("appended a float histogram to a histogram chunk")
}

// Appendable returns whether the chunk can be appended to, and if so whether
// any recoding needs to happen using the provided inserts (in case of any new
// buckets, positive or negative range, respectively).  If the sample is a gauge
// histogram, AppendableGauge must be used instead.
//
// The chunk is not appendable in the following cases:
//
//   - The schema has changed.
//   - The threshold for the zero bucket has changed.
//   - Any buckets have disappeared.
//   - There was a counter reset in the count of observations or in any bucket,
//     including the zero bucket.
//   - The last sample in the chunk was stale while the current sample is not stale.
//
// The method returns an additional boolean set to true if it is not appendable
// because of a counter reset. If the given sample is stale, it is always ok to
// append. If counterReset is true, okToAppend is always false.
func (a *HistogramAppender) Appendable(h *histogram.Histogram) (
	positiveInserts, negativeInserts []Insert,
	okToAppend, counterReset bool,
) {
	if a.NumSamples() > 0 && a.GetCounterResetHeader() == GaugeType {
		return
	}
	if value.IsStaleNaN(h.Sum) {
		// This is a stale sample whose buckets and spans don't matter.
		okToAppend = true
		return
	}
	if value.IsStaleNaN(a.sum) {
		// If the last sample was stale, then we can only accept stale
		// samples in this chunk.
		return
	}

	if h.Count < a.cnt {
		// There has been a counter reset.
		counterReset = true
		return
	}

	if h.Schema != a.schema || h.ZeroThreshold != a.zThreshold {
		return
	}

	if h.ZeroCount < a.zCnt {
		// There has been a counter reset since ZeroThreshold didn't change.
		counterReset = true
		return
	}

	var ok bool
	positiveInserts, ok = expandSpansForward(a.pSpans, h.PositiveSpans)
	if !ok {
		counterReset = true
		return
	}
	negativeInserts, ok = expandSpansForward(a.nSpans, h.NegativeSpans)
	if !ok {
		counterReset = true
		return
	}

	if counterResetInAnyBucket(a.pBuckets, h.PositiveBuckets, a.pSpans, h.PositiveSpans) ||
		counterResetInAnyBucket(a.nBuckets, h.NegativeBuckets, a.nSpans, h.NegativeSpans) {
		counterReset, positiveInserts, negativeInserts = true, nil, nil
		return
	}

	okToAppend = true
	return
}

// AppendableGauge returns whether the chunk can be appended to, and if so
// whether:
//  1. Any recoding needs to happen to the chunk using the provided inserts
//     (in case of any new buckets, positive or negative range, respectively).
//  2. Any recoding needs to happen for the histogram being appended, using the
//     backward inserts (in case of any missing buckets, positive or negative
//     range, respectively).
//
// This method must be only used for gauge histograms.
//
// The chunk is not appendable in the following cases:
//   - The schema has changed.
//   - The threshold for the zero bucket has changed.
//   - The last sample in the chunk was stale while the current sample is not stale.
func (a *HistogramAppender) AppendableGauge(h *histogram.Histogram) (
	positiveInserts, negativeInserts []Insert,
	backwardPositiveInserts, backwardNegativeInserts []Insert,
	positiveSpans, negativeSpans []histogram.Span,
	okToAppend bool,
) {
	if a.NumSamples() > 0 && a.GetCounterResetHeader() != GaugeType {
		return
	}
	if value.IsStaleNaN(h.Sum) {
		// This is a stale sample whose buckets and spans don't matter.
		okToAppend = true
		return
	}
	if value.IsStaleNaN(a.sum) {
		// If the last sample was stale, then we can only accept stale
		// samples in this chunk.
		return
	}

	if h.Schema != a.schema || h.ZeroThreshold != a.zThreshold {
		return
	}

	positiveInserts, backwardPositiveInserts, positiveSpans = expandSpansBothWays(a.pSpans, h.PositiveSpans)
	negativeInserts, backwardNegativeInserts, negativeSpans = expandSpansBothWays(a.nSpans, h.NegativeSpans)
	okToAppend = true
	return
}

// counterResetInAnyBucket returns true if there was a counter reset for any
// bucket. This should be called only when the bucket layout is the same or new
// buckets were added. It does not handle the case of buckets missing.
func counterResetInAnyBucket(oldBuckets, newBuckets []int64, oldSpans, newSpans []histogram.Span) bool {
	if len(oldSpans) == 0 || len(oldBuckets) == 0 {
		return false
	}

	oldSpanSliceIdx, newSpanSliceIdx := 0, 0                   // Index for the span slices.
	oldInsideSpanIdx, newInsideSpanIdx := uint32(0), uint32(0) // Index inside a span.
	oldIdx, newIdx := oldSpans[0].Offset, newSpans[0].Offset

	oldBucketSliceIdx, newBucketSliceIdx := 0, 0 // Index inside bucket slice.
	oldVal, newVal := oldBuckets[0], newBuckets[0]

	// Since we assume that new spans won't have missing buckets, there will never be a case
	// where the old index will not find a matching new index.
	for {
		if oldIdx == newIdx {
			if newVal < oldVal {
				return true
			}
		}

		if oldIdx <= newIdx {
			// Moving ahead old bucket and span by 1 index.
			if oldInsideSpanIdx == oldSpans[oldSpanSliceIdx].Length-1 {
				// Current span is over.
				oldSpanSliceIdx++
				oldInsideSpanIdx = 0
				if oldSpanSliceIdx >= len(oldSpans) {
					// All old spans are over.
					break
				}
				oldIdx += 1 + oldSpans[oldSpanSliceIdx].Offset
			} else {
				oldInsideSpanIdx++
				oldIdx++
			}
			oldBucketSliceIdx++
			oldVal += oldBuckets[oldBucketSliceIdx]
		}

		if oldIdx > newIdx {
			// Moving ahead new bucket and span by 1 index.
			if newInsideSpanIdx == newSpans[newSpanSliceIdx].Length-1 {
				// Current span is over.
				newSpanSliceIdx++
				newInsideSpanIdx = 0
				if newSpanSliceIdx >= len(newSpans) {
					// All new spans are over.
					// This should not happen, old spans above should catch this first.
					panic("new spans over before old spans in counterReset")
				}
				newIdx += 1 + newSpans[newSpanSliceIdx].Offset
			} else {
				newInsideSpanIdx++
				newIdx++
			}
			newBucketSliceIdx++
			newVal += newBuckets[newBucketSliceIdx]
		}
	}

	return false
}

// AppendHistogram appends a histogram to the chunk. The caller must ensure that
// the histogram is properly structured, e.g. the number of buckets used
// corresponds to the number conveyed by the span structures. First call
// Appendable() and act accordingly!
func (a *HistogramAppender) AppendHistogram(t int64, h *histogram.Histogram) {
	var tDelta, cntDelta, zCntDelta int64
	num := binary.BigEndian.Uint16(a.b.bytes())

	if value.IsStaleNaN(h.Sum) {
		// Emptying out other fields to write no buckets, and an empty
		// layout in case of first histogram in the chunk.
		h = &histogram.Histogram{Sum: h.Sum}
	}

	if num == 0 {
		// The first append gets the privilege to dictate the layout
		// but it's also responsible for encoding it into the chunk!
		writeHistogramChunkLayout(a.b, h.Schema, h.ZeroThreshold, h.PositiveSpans, h.NegativeSpans)
		a.schema = h.Schema
		a.zThreshold = h.ZeroThreshold

		if len(h.PositiveSpans) > 0 {
			a.pSpans = make([]histogram.Span, len(h.PositiveSpans))
			copy(a.pSpans, h.PositiveSpans)
		} else {
			a.pSpans = nil
		}
		if len(h.NegativeSpans) > 0 {
			a.nSpans = make([]histogram.Span, len(h.NegativeSpans))
			copy(a.nSpans, h.NegativeSpans)
		} else {
			a.nSpans = nil
		}

		numPBuckets, numNBuckets := countSpans(h.PositiveSpans), countSpans(h.NegativeSpans)
		if numPBuckets > 0 {
			a.pBuckets = make([]int64, numPBuckets)
			a.pBucketsDelta = make([]int64, numPBuckets)
		} else {
			a.pBuckets = nil
			a.pBucketsDelta = nil
		}
		if numNBuckets > 0 {
			a.nBuckets = make([]int64, numNBuckets)
			a.nBucketsDelta = make([]int64, numNBuckets)
		} else {
			a.nBuckets = nil
			a.nBucketsDelta = nil
		}

		// Now store the actual data.
		putVarbitInt(a.b, t)
		putVarbitUint(a.b, h.Count)
		putVarbitUint(a.b, h.ZeroCount)
		a.b.writeBits(math.Float64bits(h.Sum), 64)
		for _, b := range h.PositiveBuckets {
			putVarbitInt(a.b, b)
		}
		for _, b := range h.NegativeBuckets {
			putVarbitInt(a.b, b)
		}
	} else {
		// The case for the 2nd sample with single deltas is implicitly
		// handled correctly with the double delta code, so we don't
		// need a separate single delta logic for the 2nd sample.

		tDelta = t - a.t
		cntDelta = int64(h.Count) - int64(a.cnt)
		zCntDelta = int64(h.ZeroCount) - int64(a.zCnt)

		tDod := tDelta - a.tDelta
		cntDod := cntDelta - a.cntDelta
		zCntDod := zCntDelta - a.zCntDelta

		if value.IsStaleNaN(h.Sum) {
			cntDod, zCntDod = 0, 0
		}

		putVarbitInt(a.b, tDod)
		putVarbitInt(a.b, cntDod)
		putVarbitInt(a.b, zCntDod)

		a.writeSumDelta(h.Sum)

		for i, b := range h.PositiveBuckets {
			delta := b - a.pBuckets[i]
			dod := delta - a.pBucketsDelta[i]
			putVarbitInt(a.b, dod)
			a.pBucketsDelta[i] = delta
		}
		for i, b := range h.NegativeBuckets {
			delta := b - a.nBuckets[i]
			dod := delta - a.nBucketsDelta[i]
			putVarbitInt(a.b, dod)
			a.nBucketsDelta[i] = delta
		}
	}

	binary.BigEndian.PutUint16(a.b.bytes(), num+1)

	a.t = t
	a.cnt = h.Count
	a.zCnt = h.ZeroCount
	a.tDelta = tDelta
	a.cntDelta = cntDelta
	a.zCntDelta = zCntDelta

	copy(a.pBuckets, h.PositiveBuckets)
	copy(a.nBuckets, h.NegativeBuckets)
	// Note that the bucket deltas were already updated above.
	a.sum = h.Sum
}

// Recode converts the current chunk to accommodate an expansion of the set of
// (positive and/or negative) buckets used, according to the provided inserts,
// resulting in the honoring of the provided new positive and negative spans. To
// continue appending, use the returned Appender rather than the receiver of
// this method.
func (a *HistogramAppender) Recode(
	positiveInserts, negativeInserts []Insert,
	positiveSpans, negativeSpans []histogram.Span,
) (Chunk, Appender) {
	// TODO(beorn7): This currently just decodes everything and then encodes
	// it again with the new span layout. This can probably be done in-place
	// by editing the chunk. But let's first see how expensive it is in the
	// big picture. Also, in-place editing might create concurrency issues.
	byts := a.b.bytes()
	it := newHistogramIterator(byts)
	hc := NewHistogramChunk()
	app, err := hc.Appender()
	if err != nil {
		panic(err)
	}
	numPositiveBuckets, numNegativeBuckets := countSpans(positiveSpans), countSpans(negativeSpans)

	for it.Next() == ValHistogram {
		tOld, hOld := it.AtHistogram()

		// We have to newly allocate slices for the modified buckets
		// here because they are kept by the appender until the next
		// append.
		// TODO(beorn7): We might be able to optimize this.
		var positiveBuckets, negativeBuckets []int64
		if numPositiveBuckets > 0 {
			positiveBuckets = make([]int64, numPositiveBuckets)
		}
		if numNegativeBuckets > 0 {
			negativeBuckets = make([]int64, numNegativeBuckets)
		}

		// Save the modified histogram to the new chunk.
		hOld.PositiveSpans, hOld.NegativeSpans = positiveSpans, negativeSpans
		if len(positiveInserts) > 0 {
			hOld.PositiveBuckets = insert(hOld.PositiveBuckets, positiveBuckets, positiveInserts, true)
		}
		if len(negativeInserts) > 0 {
			hOld.NegativeBuckets = insert(hOld.NegativeBuckets, negativeBuckets, negativeInserts, true)
		}
		app.AppendHistogram(tOld, hOld)
	}

	hc.SetCounterResetHeader(CounterResetHeader(byts[2] & 0b11000000))
	return hc, app
}

// RecodeHistogram converts the current histogram (in-place) to accommodate an
// expansion of the set of (positive and/or negative) buckets used.
func (a *HistogramAppender) RecodeHistogram(
	h *histogram.Histogram,
	pBackwardInserts, nBackwardInserts []Insert,
) {
	if len(pBackwardInserts) > 0 {
		numPositiveBuckets := countSpans(h.PositiveSpans)
		h.PositiveBuckets = insert(h.PositiveBuckets, make([]int64, numPositiveBuckets), pBackwardInserts, true)
	}
	if len(nBackwardInserts) > 0 {
		numNegativeBuckets := countSpans(h.NegativeSpans)
		h.NegativeBuckets = insert(h.NegativeBuckets, make([]int64, numNegativeBuckets), nBackwardInserts, true)
	}
}

func (a *HistogramAppender) writeSumDelta(v float64) {
	xorWrite(a.b, v, a.sum, &a.leading, &a.trailing)
}

type histogramIterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	counterResetHeader CounterResetHeader

	// Layout:
	schema         int32
	zThreshold     float64
	pSpans, nSpans []histogram.Span

	// For the fields that are tracked as deltas and ultimately dod's.
	t                            int64
	cnt, zCnt                    uint64
	tDelta, cntDelta, zCntDelta  int64
	pBuckets, nBuckets           []int64   // Delta between buckets.
	pFloatBuckets, nFloatBuckets []float64 // Absolute counts.
	pBucketsDelta, nBucketsDelta []int64

	// The sum is Gorilla xor encoded.
	sum      float64
	leading  uint8
	trailing uint8

	// Track calls to retrieve methods. Once they have been called, we
	// cannot recycle the bucket slices anymore because we have returned
	// them in the histogram.
	atHistogramCalled, atFloatHistogramCalled bool

	err error
}

func (it *histogramIterator) Seek(t int64) ValueType {
	if it.err != nil {
		return ValNone
	}

	for t > it.t || it.numRead == 0 {
		if it.Next() == ValNone {
			return ValNone
		}
	}
	return ValHistogram
}

func (it *histogramIterator) At() (int64, float64) {
	panic("cannot call histogramIterator.At")
}

func (it *histogramIterator) AtHistogram() (int64, *histogram.Histogram) {
	if value.IsStaleNaN(it.sum) {
		return it.t, &histogram.Histogram{Sum: it.sum}
	}
	it.atHistogramCalled = true
	return it.t, &histogram.Histogram{
		CounterResetHint: counterResetHint(it.counterResetHeader, it.numRead),
		Count:            it.cnt,
		ZeroCount:        it.zCnt,
		Sum:              it.sum,
		ZeroThreshold:    it.zThreshold,
		Schema:           it.schema,
		PositiveSpans:    it.pSpans,
		NegativeSpans:    it.nSpans,
		PositiveBuckets:  it.pBuckets,
		NegativeBuckets:  it.nBuckets,
	}
}

func (it *histogramIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	if value.IsStaleNaN(it.sum) {
		return it.t, &histogram.FloatHistogram{Sum: it.sum}
	}
	it.atFloatHistogramCalled = true
	return it.t, &histogram.FloatHistogram{
		CounterResetHint: counterResetHint(it.counterResetHeader, it.numRead),
		Count:            float64(it.cnt),
		ZeroCount:        float64(it.zCnt),
		Sum:              it.sum,
		ZeroThreshold:    it.zThreshold,
		Schema:           it.schema,
		PositiveSpans:    it.pSpans,
		NegativeSpans:    it.nSpans,
		PositiveBuckets:  it.pFloatBuckets,
		NegativeBuckets:  it.nFloatBuckets,
	}
}

func (it *histogramIterator) AtT() int64 {
	return it.t
}

func (it *histogramIterator) Err() error {
	return it.err
}

func (it *histogramIterator) Reset(b []byte) {
	// The first 3 bytes contain chunk headers.
	// We skip that for actual samples.
	it.br = newBReader(b[3:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.numRead = 0

	it.counterResetHeader = CounterResetHeader(b[2] & 0b11000000)

	it.t, it.cnt, it.zCnt = 0, 0, 0
	it.tDelta, it.cntDelta, it.zCntDelta = 0, 0, 0

	// Recycle slices that have not been returned yet. Otherwise, start from
	// scratch.
	if it.atHistogramCalled {
		it.atHistogramCalled = false
		it.pBuckets, it.nBuckets = nil, nil
	} else {
		it.pBuckets = it.pBuckets[:0]
		it.nBuckets = it.nBuckets[:0]
	}
	if it.atFloatHistogramCalled {
		it.atFloatHistogramCalled = false
		it.pFloatBuckets, it.nFloatBuckets = nil, nil
	} else {
		it.pFloatBuckets = it.pFloatBuckets[:0]
		it.nFloatBuckets = it.nFloatBuckets[:0]
	}

	it.pBucketsDelta = it.pBucketsDelta[:0]
	it.nBucketsDelta = it.nBucketsDelta[:0]

	it.sum = 0
	it.leading = 0
	it.trailing = 0
	it.err = nil
}

func (it *histogramIterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	if it.numRead == 0 {
		// The first read is responsible for reading the chunk layout
		// and for initializing fields that depend on it. We give
		// counter reset info at chunk level, hence we discard it here.
		schema, zeroThreshold, posSpans, negSpans, err := readHistogramChunkLayout(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.schema = schema
		it.zThreshold = zeroThreshold
		it.pSpans, it.nSpans = posSpans, negSpans
		numPBuckets, numNBuckets := countSpans(posSpans), countSpans(negSpans)
		// The code below recycles existing slices in case this iterator
		// was reset and already has slices of a sufficient capacity.
		if numPBuckets > 0 {
			it.pBuckets = append(it.pBuckets, make([]int64, numPBuckets)...)
			it.pBucketsDelta = append(it.pBucketsDelta, make([]int64, numPBuckets)...)
			it.pFloatBuckets = append(it.pFloatBuckets, make([]float64, numPBuckets)...)
		}
		if numNBuckets > 0 {
			it.nBuckets = append(it.nBuckets, make([]int64, numNBuckets)...)
			it.nBucketsDelta = append(it.nBucketsDelta, make([]int64, numNBuckets)...)
			it.nFloatBuckets = append(it.nFloatBuckets, make([]float64, numNBuckets)...)
		}

		// Now read the actual data.
		t, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.t = t

		cnt, err := readVarbitUint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.cnt = cnt

		zcnt, err := readVarbitUint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.zCnt = zcnt

		sum, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.sum = math.Float64frombits(sum)

		var current int64
		for i := range it.pBuckets {
			v, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.pBuckets[i] = v
			current += it.pBuckets[i]
			it.pFloatBuckets[i] = float64(current)
		}
		current = 0
		for i := range it.nBuckets {
			v, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.nBuckets[i] = v
			current += it.nBuckets[i]
			it.nFloatBuckets[i] = float64(current)
		}

		it.numRead++
		return ValHistogram
	}

	// The case for the 2nd sample with single deltas is implicitly handled correctly with the double delta code,
	// so we don't need a separate single delta logic for the 2nd sample.

	// Recycle bucket slices that have not been returned yet. Otherwise,
	// copy them.
	if it.atHistogramCalled {
		it.atHistogramCalled = false
		if len(it.pBuckets) > 0 {
			newBuckets := make([]int64, len(it.pBuckets))
			copy(newBuckets, it.pBuckets)
			it.pBuckets = newBuckets
		} else {
			it.pBuckets = nil
		}
		if len(it.nBuckets) > 0 {
			newBuckets := make([]int64, len(it.nBuckets))
			copy(newBuckets, it.nBuckets)
			it.nBuckets = newBuckets
		} else {
			it.nBuckets = nil
		}
	}
	// FloatBuckets are set from scratch, so simply create empty ones.
	if it.atFloatHistogramCalled {
		it.atFloatHistogramCalled = false
		if len(it.pFloatBuckets) > 0 {
			it.pFloatBuckets = make([]float64, len(it.pFloatBuckets))
		} else {
			it.pFloatBuckets = nil
		}
		if len(it.nFloatBuckets) > 0 {
			it.nFloatBuckets = make([]float64, len(it.nFloatBuckets))
		} else {
			it.nFloatBuckets = nil
		}
	}

	tDod, err := readVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.tDelta += tDod
	it.t += it.tDelta

	cntDod, err := readVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.cntDelta += cntDod
	it.cnt = uint64(int64(it.cnt) + it.cntDelta)

	zcntDod, err := readVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.zCntDelta += zcntDod
	it.zCnt = uint64(int64(it.zCnt) + it.zCntDelta)

	ok := it.readSum()
	if !ok {
		return ValNone
	}

	if value.IsStaleNaN(it.sum) {
		it.numRead++
		return ValHistogram
	}

	var current int64
	for i := range it.pBuckets {
		dod, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.pBucketsDelta[i] += dod
		it.pBuckets[i] += it.pBucketsDelta[i]
		current += it.pBuckets[i]
		it.pFloatBuckets[i] = float64(current)
	}

	current = 0
	for i := range it.nBuckets {
		dod, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.nBucketsDelta[i] += dod
		it.nBuckets[i] += it.nBucketsDelta[i]
		current += it.nBuckets[i]
		it.nFloatBuckets[i] = float64(current)
	}

	it.numRead++
	return ValHistogram
}

func (it *histogramIterator) readSum() bool {
	err := xorRead(&it.br, &it.sum, &it.leading, &it.trailing)
	if err != nil {
		it.err = err
		return false
	}
	return true
}
