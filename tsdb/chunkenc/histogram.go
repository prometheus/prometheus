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
	"math/bits"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/pkg/value"
)

const ()

// HistogramChunk holds encoded sample data for a sparse, high-resolution
// histogram.
//
// Each sample has multiple "fields", stored in the following way (raw = store
// number directly, delta = store delta to the previous number, dod = store
// delta of the delta to the previous number, xor = what we do for regular
// sample values):
//
//   field →    ts    count zeroCount sum []posbuckets []negbuckets
//   sample 1   raw   raw   raw       raw []raw        []raw
//   sample 2   delta delta delta     xor []delta      []delta
//   sample >2  dod   dod   dod       xor []dod        []dod
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
		panic("HistoChunk.Layout() called on an empty chunk")
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

// SetCounterResetHeader sets the counter reset header.
func (c *HistogramChunk) SetCounterResetHeader(h CounterResetHeader) {
	switch h {
	case CounterReset, NotCounterReset, GaugeType, UnknownCounterReset:
		bytes := c.Bytes()
		bytes[2] = (bytes[2] & 0b00111111) | byte(h)
	default:
		panic("invalid CounterResetHeader type")
	}
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
	for it.Next() {
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
	return it
}

func (c *HistogramChunk) iterator(it Iterator) *histogramIterator {
	// This commet is copied from XORChunk.iterator:
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

// Append implements Appender. This implementation panics because normal float
// samples must never be appended to a histogram chunk.
func (a *HistogramAppender) Append(int64, float64) {
	panic("appended a float sample to a histogram chunk")
}

// Appendable returns whether the chunk can be appended to, and if so
// whether any recoding needs to happen using the provided interjections
// (in case of any new buckets, positive or negative range, respectively).
//
// The chunk is not appendable in the following cases:
//
// • The schema has changed.
//
// • The threshold for the zero bucket has changed.
//
// • Any buckets have disappeared.
//
// • There was a counter reset in the count of observations or in any bucket,
// including the zero bucket.
//
// • The last sample in the chunk was stale while the current sample is not stale.
//
// The method returns an additional boolean set to true if it is not appendable
// because of a counter reset. If the given sample is stale, it is always ok to
// append. If counterReset is true, okToAppend is always false.
func (a *HistogramAppender) Appendable(h histogram.Histogram) (
	positiveInterjections, negativeInterjections []Interjection,
	okToAppend bool, counterReset bool,
) {
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
	positiveInterjections, ok = compareSpans(a.pSpans, h.PositiveSpans)
	if !ok {
		counterReset = true
		return
	}
	negativeInterjections, ok = compareSpans(a.nSpans, h.NegativeSpans)
	if !ok {
		counterReset = true
		return
	}

	if counterResetInAnyBucket(a.pBuckets, h.PositiveBuckets, a.pSpans, h.PositiveSpans) ||
		counterResetInAnyBucket(a.nBuckets, h.NegativeBuckets, a.nSpans, h.NegativeSpans) {
		counterReset, positiveInterjections, negativeInterjections = true, nil, nil
		return
	}

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
func (a *HistogramAppender) AppendHistogram(t int64, h histogram.Histogram) {
	var tDelta, cntDelta, zCntDelta int64
	num := binary.BigEndian.Uint16(a.b.bytes())

	if value.IsStaleNaN(h.Sum) {
		// Emptying out other fields to write no buckets, and an empty
		// layout in case of first histogram in the chunk.
		h = histogram.Histogram{Sum: h.Sum}
	}

	switch num {
	case 0:
		// The first append gets the privilege to dictate the layout
		// but it's also responsible for encoding it into the chunk!
		writeHistogramChunkLayout(a.b, h.Schema, h.ZeroThreshold, h.PositiveSpans, h.NegativeSpans)
		a.schema = h.Schema
		a.zThreshold = h.ZeroThreshold

		a.pSpans = make([]histogram.Span, len(h.PositiveSpans))
		copy(a.pSpans, h.PositiveSpans)
		a.nSpans = make([]histogram.Span, len(h.NegativeSpans))
		copy(a.nSpans, h.NegativeSpans)

		numPBuckets, numNBuckets := countSpans(h.PositiveSpans), countSpans(h.NegativeSpans)
		a.pBuckets = make([]int64, numPBuckets)
		a.nBuckets = make([]int64, numNBuckets)
		a.pBucketsDelta = make([]int64, numPBuckets)
		a.nBucketsDelta = make([]int64, numNBuckets)

		// Now store the actual data.
		putVarbitInt(a.b, t)
		putVarbitUint(a.b, h.Count)
		putVarbitUint(a.b, h.ZeroCount) //
		a.b.writeBits(math.Float64bits(h.Sum), 64)
		for _, b := range h.PositiveBuckets {
			putVarbitInt(a.b, b)
		}
		for _, b := range h.NegativeBuckets {
			putVarbitInt(a.b, b)
		}
	case 1:
		tDelta = t - a.t
		if tDelta < 0 {
			panic("out of order timestamp")
		}
		cntDelta = int64(h.Count) - int64(a.cnt)
		zCntDelta = int64(h.ZeroCount) - int64(a.zCnt)

		if value.IsStaleNaN(h.Sum) {
			cntDelta, zCntDelta = 0, 0
		}

		putVarbitUint(a.b, uint64(tDelta))
		putVarbitInt(a.b, cntDelta)
		putVarbitInt(a.b, zCntDelta)

		a.writeSumDelta(h.Sum)

		for i, b := range h.PositiveBuckets {
			delta := b - a.pBuckets[i]
			putVarbitInt(a.b, delta)
			a.pBucketsDelta[i] = delta
		}
		for i, b := range h.NegativeBuckets {
			delta := b - a.nBuckets[i]
			putVarbitInt(a.b, delta)
			a.nBucketsDelta[i] = delta
		}

	default:
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

		for i, buck := range h.PositiveBuckets {
			delta := buck - a.pBuckets[i]
			dod := delta - a.pBucketsDelta[i]
			putVarbitInt(a.b, dod)
			a.pBucketsDelta[i] = delta
		}
		for i, buck := range h.NegativeBuckets {
			delta := buck - a.nBuckets[i]
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
// (positive and/or negative) buckets used, according to the provided
// interjections, resulting in the honoring of the provided new positive and
// negative spans.
func (a *HistogramAppender) Recode(
	positiveInterjections, negativeInterjections []Interjection,
	positiveSpans, negativeSpans []histogram.Span,
) (Chunk, Appender) {
	// TODO(beorn7): This currently just decodes everything and then encodes
	// it again with the new span layout. This can probably be done in-place
	// by editing the chunk. But let's first see how expensive it is in the
	// big picture.
	byts := a.b.bytes()
	it := newHistogramIterator(byts)
	hc := NewHistogramChunk()
	app, err := hc.Appender()
	if err != nil {
		panic(err)
	}
	numPositiveBuckets, numNegativeBuckets := countSpans(positiveSpans), countSpans(negativeSpans)

	for it.Next() {
		tOld, hOld := it.AtHistogram()

		// We have to newly allocate slices for the modified buckets
		// here because they are kept by the appender until the next
		// append.
		// TODO(beorn7): We might be able to optimize this.
		positiveBuckets := make([]int64, numPositiveBuckets)
		negativeBuckets := make([]int64, numNegativeBuckets)

		// Save the modified histogram to the new chunk.
		hOld.PositiveSpans, hOld.NegativeSpans = positiveSpans, negativeSpans
		if len(positiveInterjections) > 0 {
			hOld.PositiveBuckets = interject(hOld.PositiveBuckets, positiveBuckets, positiveInterjections)
		}
		if len(negativeInterjections) > 0 {
			hOld.NegativeBuckets = interject(hOld.NegativeBuckets, negativeBuckets, negativeInterjections)
		}
		app.AppendHistogram(tOld, hOld)
	}

	hc.SetCounterResetHeader(CounterResetHeader(byts[2] & 0b11000000))
	return hc, app
}

func (a *HistogramAppender) writeSumDelta(v float64) {
	vDelta := math.Float64bits(v) ^ math.Float64bits(a.sum)

	if vDelta == 0 {
		a.b.writeBit(zero)
		return
	}
	a.b.writeBit(one)

	leading := uint8(bits.LeadingZeros64(vDelta))
	trailing := uint8(bits.TrailingZeros64(vDelta))

	// Clamp number of leading zeros to avoid overflow when encoding.
	if leading >= 32 {
		leading = 31
	}

	if a.leading != 0xff && leading >= a.leading && trailing >= a.trailing {
		a.b.writeBit(zero)
		a.b.writeBits(vDelta>>a.trailing, 64-int(a.leading)-int(a.trailing))
	} else {
		a.leading, a.trailing = leading, trailing

		a.b.writeBit(one)
		a.b.writeBits(uint64(leading), 5)

		// Note that if leading == trailing == 0, then sigbits == 64.
		// But that value doesn't actually fit into the 6 bits we have.
		// Luckily, we never need to encode 0 significant bits, since
		// that would put us in the other case (vdelta == 0).  So
		// instead we write out a 0 and adjust it back to 64 on
		// unpacking.
		sigbits := 64 - leading - trailing
		a.b.writeBits(uint64(sigbits), 6)
		a.b.writeBits(vDelta>>trailing, int(sigbits))
	}
}

type histogramIterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	// Layout:
	schema         int32
	zThreshold     float64
	pSpans, nSpans []histogram.Span

	// For the fields that are tracked as deltas and ultimately dod's.
	t                            int64
	cnt, zCnt                    uint64
	tDelta, cntDelta, zCntDelta  int64
	pBuckets, nBuckets           []int64
	pBucketsDelta, nBucketsDelta []int64

	// The sum is Gorilla xor encoded.
	sum      float64
	leading  uint8
	trailing uint8

	err error
}

func (it *histogramIterator) Seek(t int64) bool {
	if it.err != nil {
		return false
	}

	for t > it.t || it.numRead == 0 {
		if !it.Next() {
			return false
		}
	}
	return true
}

func (it *histogramIterator) At() (int64, float64) {
	panic("cannot call histogramIterator.At")
}

func (it *histogramIterator) ChunkEncoding() Encoding {
	return EncHistogram
}

func (it *histogramIterator) AtHistogram() (int64, histogram.Histogram) {
	if value.IsStaleNaN(it.sum) {
		return it.t, histogram.Histogram{Sum: it.sum}
	}
	return it.t, histogram.Histogram{
		Count:           it.cnt,
		ZeroCount:       it.zCnt,
		Sum:             it.sum,
		ZeroThreshold:   it.zThreshold,
		Schema:          it.schema,
		PositiveSpans:   it.pSpans,
		NegativeSpans:   it.nSpans,
		PositiveBuckets: it.pBuckets,
		NegativeBuckets: it.nBuckets,
	}
}

func (it *histogramIterator) Err() error {
	return it.err
}

func (it *histogramIterator) Reset(b []byte) {
	// The first 2 bytes contain chunk headers.
	// We skip that for actual samples.
	it.br = newBReader(b[2:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.numRead = 0

	it.t, it.cnt, it.zCnt = 0, 0, 0
	it.tDelta, it.cntDelta, it.zCntDelta = 0, 0, 0

	it.pBuckets = it.pBuckets[:0]
	it.pBucketsDelta = it.pBucketsDelta[:0]
	it.nBuckets = it.nBuckets[:0]
	it.pBucketsDelta = it.pBucketsDelta[:0]

	it.sum = 0
	it.leading = 0
	it.trailing = 0
	it.err = nil
}

func (it *histogramIterator) Next() bool {
	if it.err != nil || it.numRead == it.numTotal {
		return false
	}

	if it.numRead == 0 {
		// The first read is responsible for reading the chunk layout
		// and for initializing fields that depend on it. We give
		// counter reset info at chunk level, hence we discard it here.
		schema, zeroThreshold, posSpans, negSpans, err := readHistogramChunkLayout(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.schema = schema
		it.zThreshold = zeroThreshold
		it.pSpans, it.nSpans = posSpans, negSpans
		numPBuckets, numNBuckets := countSpans(posSpans), countSpans(negSpans)
		// Allocate bucket slices as needed, recycling existing slices
		// in case this iterator was reset and already has slices of a
		// sufficient capacity..
		if numPBuckets > 0 {
			if cap(it.pBuckets) < numPBuckets {
				it.pBuckets = make([]int64, numPBuckets)
				// If cap(it.pBuckets) isn't sufficient, neither is cap(it.pBucketsDelta).
				it.pBucketsDelta = make([]int64, numPBuckets)
			} else {
				for i := 0; i < numPBuckets; i++ {
					it.pBuckets = append(it.pBuckets, 0)
					it.pBucketsDelta = append(it.pBucketsDelta, 0)
				}
			}
		}
		if numNBuckets > 0 {
			if cap(it.nBuckets) < numNBuckets {
				it.nBuckets = make([]int64, numNBuckets)
				// If cap(it.nBuckets) isn't sufficient, neither is cap(it.nBucketsDelta).
				it.nBucketsDelta = make([]int64, numNBuckets)
			} else {
				for i := 0; i < numNBuckets; i++ {
					it.nBuckets = append(it.nBuckets, 0)
					it.nBucketsDelta = append(it.nBucketsDelta, 0)
				}
			}
		}

		// Now read the actual data.
		t, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.t = t

		cnt, err := readVarbitUint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.cnt = cnt

		zcnt, err := readVarbitUint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.zCnt = zcnt

		sum, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return false
		}
		it.sum = math.Float64frombits(sum)

		for i := range it.pBuckets {
			v, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.pBuckets[i] = v
		}
		for i := range it.nBuckets {
			v, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.nBuckets[i] = v
		}

		it.numRead++
		return true
	}

	if it.numRead == 1 {
		tDelta, err := readVarbitUint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.tDelta = int64(tDelta)
		it.t += it.tDelta

		cntDelta, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.cntDelta = cntDelta
		it.cnt = uint64(int64(it.cnt) + it.cntDelta)

		zcntDelta, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.zCntDelta = zcntDelta
		it.zCnt = uint64(int64(it.zCnt) + it.zCntDelta)

		ok := it.readSum()
		if !ok {
			return false
		}

		if value.IsStaleNaN(it.sum) {
			it.numRead++
			return true
		}

		for i := range it.pBuckets {
			delta, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.pBucketsDelta[i] = delta
			it.pBuckets[i] = it.pBuckets[i] + delta
		}

		for i := range it.nBuckets {
			delta, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.nBucketsDelta[i] = delta
			it.nBuckets[i] = it.nBuckets[i] + delta
		}

		it.numRead++
		return true
	}

	tDod, err := readVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return false
	}
	it.tDelta = it.tDelta + tDod
	it.t += it.tDelta

	cntDod, err := readVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return false
	}
	it.cntDelta = it.cntDelta + cntDod
	it.cnt = uint64(int64(it.cnt) + it.cntDelta)

	zcntDod, err := readVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return false
	}
	it.zCntDelta = it.zCntDelta + zcntDod
	it.zCnt = uint64(int64(it.zCnt) + it.zCntDelta)

	ok := it.readSum()
	if !ok {
		return false
	}

	if value.IsStaleNaN(it.sum) {
		it.numRead++
		return true
	}

	for i := range it.pBuckets {
		dod, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.pBucketsDelta[i] = it.pBucketsDelta[i] + dod
		it.pBuckets[i] = it.pBuckets[i] + it.pBucketsDelta[i]
	}

	for i := range it.nBuckets {
		dod, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.nBucketsDelta[i] = it.nBucketsDelta[i] + dod
		it.nBuckets[i] = it.nBuckets[i] + it.nBucketsDelta[i]
	}

	it.numRead++
	return true
}

func (it *histogramIterator) readSum() bool {
	bit, err := it.br.readBitFast()
	if err != nil {
		bit, err = it.br.readBit()
	}
	if err != nil {
		it.err = err
		return false
	}

	if bit == zero {
		return true // it.sum = it.sum
	}

	bit, err = it.br.readBitFast()
	if err != nil {
		bit, err = it.br.readBit()
	}
	if err != nil {
		it.err = err
		return false
	}
	if bit == zero {
		// Reuse leading/trailing zero bits.
		// it.leading, it.trailing = it.leading, it.trailing
	} else {
		bits, err := it.br.readBitsFast(5)
		if err != nil {
			bits, err = it.br.readBits(5)
		}
		if err != nil {
			it.err = err
			return false
		}
		it.leading = uint8(bits)

		bits, err = it.br.readBitsFast(6)
		if err != nil {
			bits, err = it.br.readBits(6)
		}
		if err != nil {
			it.err = err
			return false
		}
		mbits := uint8(bits)
		// 0 significant bits here means we overflowed and we actually
		// need 64; see comment in encoder.
		if mbits == 0 {
			mbits = 64
		}
		it.trailing = 64 - it.leading - mbits
	}

	mbits := 64 - it.leading - it.trailing
	bits, err := it.br.readBitsFast(mbits)
	if err != nil {
		bits, err = it.br.readBits(mbits)
	}
	if err != nil {
		it.err = err
		return false
	}
	vbits := math.Float64bits(it.sum)
	vbits ^= bits << it.trailing
	it.sum = math.Float64frombits(vbits)
	return true
}
