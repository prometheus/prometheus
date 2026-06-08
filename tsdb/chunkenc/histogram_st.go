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

package chunkenc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

// HistogramSTChunk is a chunk for histogram samples with start timestamp (ST) support.
//
// Header layout (3 bytes):
//
//	byte 0 bits 7-6:           counter reset header.
//	byte 0 bits 5-0 + byte 1:  14-bit big-endian sample count.
//	byte 2:                    ST header (bit 7 = firstSTKnown, bits 6-0 = firstSTChangeOn).
type HistogramSTChunk struct {
	b bstream
}

// NewHistogramSTChunk returns a new empty HistogramSTChunk.
func NewHistogramSTChunk() *HistogramSTChunk {
	b := make([]byte, histogramHeaderSize, chunkAllocationSize)
	return &HistogramSTChunk{b: bstream{stream: b, count: 0}}
}

func (c *HistogramSTChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*HistogramSTChunk) Encoding() Encoding { return EncHistogramST }

// Bytes returns the underlying byte slice of the chunk.
func (c *HistogramSTChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *HistogramSTChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.b.bytes()) & 0x3FFF)
}

// GetCounterResetHeader returns the counter reset header from bits 7-6 of
// byte 0. This differs from base HistogramChunk (which reads byte 2 via
// histogramFlagPos) — in ST chunks byte 2 holds the ST header.
func (c *HistogramSTChunk) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(c.b.bytes()[0] & CounterResetHeaderMask)
}

// Compact implements the Chunk interface.
func (c *HistogramSTChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *HistogramSTChunk) Appender() (Appender, error) {
	if len(c.b.stream) == histogramHeaderSize {
		return &HistogramSTAppender{
			HistogramAppender: HistogramAppender{
				b:            &c.b,
				headerLayout: histogramHeaderST,
				t:            math.MinInt64,
				leading:      0xff,
			},
		}, nil
	}

	it := c.iterator(nil)

	for it.Next() == ValHistogram {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	// Set the bit position for continuing writes. The iterator's reader tracks
	// how many bits remain unread in the last byte.
	c.b.count = it.br.valid

	a := &HistogramSTAppender{
		HistogramAppender: HistogramAppender{
			b:            &c.b,
			headerLayout: histogramHeaderST,

			schema:        it.schema,
			zThreshold:    it.zThreshold,
			pSpans:        it.pSpans,
			nSpans:        it.nSpans,
			customValues:  it.customValues,
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
		},
		stEncoder: stEncoder{
			st:              it.st,
			stDiff:          it.stDiff,
			firstSTKnown:    it.firstSTKnown,
			firstSTChangeOn: it.firstSTChangeOn,
		},
	}
	return a, nil
}

func newHistogramSTIterator(b []byte) *histogramSTIterator {
	it := &histogramSTIterator{
		histogramIterator: histogramIterator{
			br:       newBReader(b[histogramHeaderSize:]),
			numTotal: binary.BigEndian.Uint16(b) & 0x3FFF,
			t:        math.MinInt64,
		},
	}
	it.counterResetHeader = CounterResetHeader(b[0] & CounterResetHeaderMask)
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramHeaderSize-1:])
	return it
}

func (c *HistogramSTChunk) iterator(it Iterator) *histogramSTIterator {
	if histIter, ok := it.(*histogramSTIterator); ok {
		histIter.Reset(c.b.bytes())
		return histIter
	}
	return newHistogramSTIterator(c.b.bytes())
}

// Iterator implements the Chunk interface.
func (c *HistogramSTChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// HistogramSTAppender is an Appender for histogram samples with start timestamp support.
// It embeds HistogramAppender and adds ST encoding after each sample.
type HistogramSTAppender struct {
	HistogramAppender
	stEncoder
}

// GetCounterResetHeader returns the counter-reset header from bits 7-6 of byte 0.
func (a *HistogramSTAppender) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(a.b.bytes()[0] & CounterResetHeaderMask)
}

// setCounterResetHeader writes the counter-reset header into bits 7-6 of byte 0.
func (a *HistogramSTAppender) setCounterResetHeader(cr CounterResetHeader) {
	b := a.b.bytes()
	b[0] = (b[0] &^ CounterResetHeaderMask) | (byte(cr) & CounterResetHeaderMask)
}

func (a *HistogramSTAppender) appendable(h *histogram.Histogram) (
	positiveInserts, negativeInserts []Insert,
	backwardPositiveInserts, backwardNegativeInserts []Insert,
	okToAppend bool, counterResetHint CounterResetHeader,
) {
	counterResetHint = NotCounterReset
	if a.NumSamples() > 0 && a.GetCounterResetHeader() == GaugeType {
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}
	if h.CounterResetHint == histogram.CounterReset {
		// Always honor the explicit counter reset hint.
		counterResetHint = CounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}
	if value.IsStaleNaN(h.Sum) {
		// This is a stale sample whose buckets and spans don't matter.
		okToAppend = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}
	if value.IsStaleNaN(a.sum) {
		// If the last sample was stale, then we can only accept stale
		// samples in this chunk.
		counterResetHint = UnknownCounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}

	if h.Count < a.cnt {
		// There has been a counter reset.
		counterResetHint = CounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}

	if h.Schema != a.schema || h.ZeroThreshold != a.zThreshold {
		counterResetHint = UnknownCounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}

	if histogram.IsCustomBucketsSchema(h.Schema) && !histogram.CustomBucketBoundsMatch(h.CustomValues, a.customValues) {
		counterResetHint = CounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}

	if h.ZeroCount < a.zCnt {
		// There has been a counter reset since ZeroThreshold didn't change.
		counterResetHint = CounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}

	var ok bool
	positiveInserts, backwardPositiveInserts, ok = expandIntSpansAndBuckets(a.pSpans, h.PositiveSpans, a.pBuckets, h.PositiveBuckets)
	if !ok {
		counterResetHint = CounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}
	negativeInserts, backwardNegativeInserts, ok = expandIntSpansAndBuckets(a.nSpans, h.NegativeSpans, a.nBuckets, h.NegativeBuckets)
	if !ok {
		counterResetHint = CounterReset
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
	}

	okToAppend = true
	return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterResetHint
}

func (a *HistogramSTAppender) appendableGauge(h *histogram.Histogram) (
	positiveInserts, negativeInserts []Insert,
	backwardPositiveInserts, backwardNegativeInserts []Insert,
	positiveSpans, negativeSpans []histogram.Span,
	okToAppend bool,
) {
	if a.NumSamples() > 0 && a.GetCounterResetHeader() != GaugeType {
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, positiveSpans, negativeSpans, okToAppend
	}
	if value.IsStaleNaN(h.Sum) {
		// This is a stale sample whose buckets and spans don't matter.
		okToAppend = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, positiveSpans, negativeSpans, okToAppend
	}
	if value.IsStaleNaN(a.sum) {
		// If the last sample was stale, then we can only accept stale
		// samples in this chunk.
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, positiveSpans, negativeSpans, okToAppend
	}

	if h.Schema != a.schema || h.ZeroThreshold != a.zThreshold {
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, positiveSpans, negativeSpans, okToAppend
	}

	if histogram.IsCustomBucketsSchema(h.Schema) && !histogram.CustomBucketBoundsMatch(h.CustomValues, a.customValues) {
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, positiveSpans, negativeSpans, okToAppend
	}

	positiveInserts, backwardPositiveInserts, positiveSpans = expandSpansBothWays(a.pSpans, h.PositiveSpans)
	negativeInserts, backwardNegativeInserts, negativeSpans = expandSpansBothWays(a.nSpans, h.NegativeSpans)
	okToAppend = true
	return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, positiveSpans, negativeSpans, okToAppend
}

func (a *HistogramSTAppender) appendHistogram(t int64, h *histogram.Histogram) {
	var tDelta, cntDelta, zCntDelta int64
	num := binary.BigEndian.Uint16(a.b.bytes()) & 0x3FFF

	if value.IsStaleNaN(h.Sum) {
		// Emptying out other fields to write no buckets, and an empty
		// layout in case of first histogram in the chunk.
		h = &histogram.Histogram{Sum: h.Sum}
	}

	if num == 0 {
		// The first append gets the privilege to dictate the layout
		// but it's also responsible for encoding it into the chunk!
		writeHistogramChunkLayout(a.b, h.Schema, h.ZeroThreshold, h.PositiveSpans, h.NegativeSpans, h.CustomValues)
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
		if len(h.CustomValues) > 0 {
			a.customValues = make([]float64, len(h.CustomValues))
			copy(a.customValues, h.CustomValues)
		} else {
			a.customValues = nil
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

	// Write the incremented count back, preserving the counter-reset bits in
	// the top 2 bits of byte 0.
	buf := a.b.bytes()
	crBits := buf[0] & CounterResetHeaderMask
	binary.BigEndian.PutUint16(buf, (uint16(crBits)<<8)|(num+1))

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

// appendHistogramST encodes a histogram sample with start timestamp.
func (a *HistogramSTAppender) appendHistogramST(st, t int64, h *histogram.Histogram) {
	prevT := a.t
	a.appendHistogram(t, h)
	num := binary.BigEndian.Uint16(a.b.bytes()) & 0x3FFF
	a.encode(a.b, num, a.t, prevT, st)
}

func (*HistogramSTAppender) Append(int64, int64, float64) {
	panic("appended a float sample to a histogram chunk")
}

func (*HistogramSTAppender) AppendFloatHistogram(Appender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a histogram chunk")
}

// AppendHistogram implements Appender for HistogramSTAppender.
func (a *HistogramSTAppender) AppendHistogram(prev Appender, st, t int64, h *histogram.Histogram, appendOnly bool) (Chunk, bool, Appender, error) {
	numSamples := a.NumSamples()

	if numSamples == int(a.sampleCountMask()) {
		panic("chunk capacity exceeded")
	}

	if numSamples == 0 {
		a.appendHistogramST(st, t, h)
		if h.CounterResetHint == histogram.GaugeType {
			a.setCounterResetHeader(GaugeType)
			return nil, false, a, nil
		}

		switch {
		case h.CounterResetHint == histogram.CounterReset:
			a.setCounterResetHeader(CounterReset)
		case prev != nil:
			if p, ok := prev.(histogramAppendable); ok {
				_, _, _, _, _, counterReset := p.appendable(h)
				a.setCounterResetHeader(counterReset)
			}
		}
		return nil, false, a, nil
	}

	// Adding counter-like histogram.
	if h.CounterResetHint != histogram.GaugeType {
		pForwardInserts, nForwardInserts, pBackwardInserts, nBackwardInserts, okToAppend, counterResetHint := a.appendable(h)
		if !okToAppend || counterResetHint != NotCounterReset {
			if appendOnly {
				if counterResetHint == CounterReset {
					return nil, false, a, errors.New("histogram counter reset")
				}
				return nil, false, a, errors.New("histogram schema change")
			}
			newChunk := NewHistogramSTChunk()
			app, err := newChunk.Appender()
			if err != nil {
				panic(err)
			}
			happ := app.(*HistogramSTAppender)
			happ.setCounterResetHeader(counterResetHint)
			happ.appendHistogramST(st, t, h)
			return newChunk, false, app, nil
		}
		if len(pBackwardInserts) > 0 || len(nBackwardInserts) > 0 {
			if len(pForwardInserts) == 0 && len(nForwardInserts) == 0 {
				h.PositiveSpans = make([]histogram.Span, len(a.pSpans))
				copy(h.PositiveSpans, a.pSpans)
				h.NegativeSpans = make([]histogram.Span, len(a.nSpans))
				copy(h.NegativeSpans, a.nSpans)
			} else {
				h.PositiveSpans = adjustForInserts(h.PositiveSpans, pBackwardInserts)
				h.NegativeSpans = adjustForInserts(h.NegativeSpans, nBackwardInserts)
			}
			a.recodeHistogram(h, pBackwardInserts, nBackwardInserts)
		}
		if len(pForwardInserts) > 0 || len(nForwardInserts) > 0 {
			if appendOnly {
				return nil, false, a, fmt.Errorf("histogram layout change with %d positive and %d negative forwards inserts", len(pForwardInserts), len(nForwardInserts))
			}
			chk, app := a.recodeST(
				pForwardInserts, nForwardInserts,
				h.PositiveSpans, h.NegativeSpans,
			)
			app.(*HistogramSTAppender).appendHistogramST(st, t, h)
			return chk, true, app, nil
		}
		a.appendHistogramST(st, t, h)
		return nil, false, a, nil
	}

	// Adding gauge histogram.
	pForwardInserts, nForwardInserts, pBackwardInserts, nBackwardInserts, pMergedSpans, nMergedSpans, okToAppend := a.appendableGauge(h)
	if !okToAppend {
		if appendOnly {
			return nil, false, a, errors.New("gauge histogram schema change")
		}
		newChunk := NewHistogramSTChunk()
		app, err := newChunk.Appender()
		if err != nil {
			panic(err)
		}
		happ := app.(*HistogramSTAppender)
		happ.setCounterResetHeader(GaugeType)
		happ.appendHistogramST(st, t, h)
		return newChunk, false, app, nil
	}

	if len(pBackwardInserts)+len(nBackwardInserts) > 0 {
		if appendOnly {
			return nil, false, a, fmt.Errorf("gauge histogram layout change with %d positive and %d negative backwards inserts", len(pBackwardInserts), len(nBackwardInserts))
		}
		h.PositiveSpans = pMergedSpans
		h.NegativeSpans = nMergedSpans
		a.recodeHistogram(h, pBackwardInserts, nBackwardInserts)
	}

	if len(pForwardInserts) > 0 || len(nForwardInserts) > 0 {
		if appendOnly {
			return nil, false, a, fmt.Errorf("gauge histogram layout change with %d positive and %d negative forwards inserts", len(pForwardInserts), len(nForwardInserts))
		}
		chk, app := a.recodeST(
			pForwardInserts, nForwardInserts,
			h.PositiveSpans, h.NegativeSpans,
		)
		app.(*HistogramSTAppender).appendHistogramST(st, t, h)
		return chk, true, app, nil
	}

	a.appendHistogramST(st, t, h)
	return nil, false, a, nil
}

// recodeST is like HistogramAppender.recode but creates HistogramSTChunk and preserves ST.
func (a *HistogramSTAppender) recodeST(
	positiveInserts, negativeInserts []Insert,
	positiveSpans, negativeSpans []histogram.Span,
) (Chunk, Appender) {
	byts := a.b.bytes()
	it := newHistogramSTIterator(byts)
	hc := NewHistogramSTChunk()
	app, err := hc.Appender()
	if err != nil {
		panic(err)
	}
	happ := app.(*HistogramSTAppender)
	numPositiveBuckets, numNegativeBuckets := countSpans(positiveSpans), countSpans(negativeSpans)

	for it.Next() == ValHistogram {
		tOld, hOld := it.AtHistogram(nil)
		stOld := it.AtST()

		var positiveBuckets, negativeBuckets []int64
		if numPositiveBuckets > 0 {
			positiveBuckets = make([]int64, numPositiveBuckets)
		}
		if numNegativeBuckets > 0 {
			negativeBuckets = make([]int64, numNegativeBuckets)
		}

		hOld.PositiveSpans, hOld.NegativeSpans = positiveSpans, negativeSpans
		if len(positiveInserts) > 0 {
			hOld.PositiveBuckets = insert(hOld.PositiveBuckets, positiveBuckets, positiveInserts, true)
		}
		if len(negativeInserts) > 0 {
			hOld.NegativeBuckets = insert(hOld.NegativeBuckets, negativeBuckets, negativeInserts, true)
		}
		happ.appendHistogramST(stOld, tOld, hOld)
	}

	happ.setCounterResetHeader(CounterResetHeader(byts[0] & CounterResetHeaderMask))
	return hc, app
}

// histogramSTIterator is an iterator for HistogramSTChunk that decodes ST after each sample.
type histogramSTIterator struct {
	histogramIterator
	stDecoder
}

func (it *histogramSTIterator) AtST() int64 {
	return it.st
}

func (it *histogramSTIterator) Reset(b []byte) {
	it.stDecoder = stDecoder{}
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramHeaderSize-1:])

	// Reset the embedded histogramIterator but with the correct header offset.
	it.br = newBReader(b[histogramHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b) & 0x3FFF
	it.numRead = 0

	it.counterResetHeader = CounterResetHeader(b[0] & CounterResetHeaderMask)

	it.t, it.cnt, it.zCnt = 0, 0, 0
	it.tDelta, it.cntDelta, it.zCntDelta = 0, 0, 0

	if it.atHistogramCalled {
		it.atHistogramCalled = false
		it.pBuckets, it.nBuckets = nil, nil
		it.pSpans, it.nSpans = nil, nil
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
	it.customValues = nil
}

// Next advances the iterator by one sample.
// It calls the embedded histogramIterator.Next() to decode the histogram sample,
// then decodes the ST data that follows in the bitstream.
func (it *histogramSTIterator) Next() ValueType {
	prevT := it.t
	vt := it.histogramIterator.Next()
	if vt == ValNone {
		return ValNone
	}
	if err := it.decode(&it.br, it.numRead, it.t, prevT); err != nil {
		it.err = err
		return ValNone
	}
	return vt
}

// Seek advances the iterator forward to the first sample with timestamp >= t.
func (it *histogramSTIterator) Seek(t int64) ValueType {
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
