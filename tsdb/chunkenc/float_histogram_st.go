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

// FloatHistogramSTChunk is a chunk for float histogram samples with start timestamp (ST) support.
//
// Header layout:
//
//	byte 0 bits 7-6:           counter reset header.
//	byte 0 bits 5-0 + byte 1:  14-bit big-endian sample count.
//	byte 2:                    ST header (bit 7 = firstSTKnown, bits 6-0 = firstSTChangeOn).
type FloatHistogramSTChunk struct {
	b bstream
}

// NewFloatHistogramSTChunk returns a new empty FloatHistogramSTChunk.
func NewFloatHistogramSTChunk() *FloatHistogramSTChunk {
	b := make([]byte, histogramHeaderSize, chunkAllocationSize)
	return &FloatHistogramSTChunk{b: bstream{stream: b, count: 0}}
}

// Reset resets the chunk given stream.
func (c *FloatHistogramSTChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*FloatHistogramSTChunk) Encoding() Encoding { return EncFloatHistogramST }

// Bytes returns the underlying byte slice of the chunk.
func (c *FloatHistogramSTChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *FloatHistogramSTChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.b.bytes()) & 0x3FFF)
}

// GetCounterResetHeader returns the counter reset header from bits 7-6 of
// byte 0. This differs from base FloatHistogramChunk (which reads byte 2 via
// histogramFlagPos) — in ST chunks byte 2 holds the ST header.
func (c *FloatHistogramSTChunk) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(c.b.bytes()[0] & CounterResetHeaderMask)
}

// Compact implements the Chunk interface.
func (c *FloatHistogramSTChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *FloatHistogramSTChunk) Appender() (Appender, error) {
	if len(c.b.stream) == histogramHeaderSize {
		return &FloatHistogramSTAppender{
			FloatHistogramAppender: FloatHistogramAppender{
				b:    &c.b,
				t:    math.MinInt64,
				sum:  xorValue{leading: 0xff},
				cnt:  xorValue{leading: 0xff},
				zCnt: xorValue{leading: 0xff},
			},
		}, nil
	}

	it := c.iterator(nil)

	for it.Next() == ValFloatHistogram {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	// Set the bit position for continuing writes.
	c.b.count = it.br.valid

	pBuckets := make([]xorValue, len(it.pBuckets))
	for i := 0; i < len(it.pBuckets); i++ {
		pBuckets[i] = xorValue{
			value:    it.pBuckets[i],
			leading:  it.pBucketsLeading[i],
			trailing: it.pBucketsTrailing[i],
		}
	}
	nBuckets := make([]xorValue, len(it.nBuckets))
	for i := 0; i < len(it.nBuckets); i++ {
		nBuckets[i] = xorValue{
			value:    it.nBuckets[i],
			leading:  it.nBucketsLeading[i],
			trailing: it.nBucketsTrailing[i],
		}
	}

	a := &FloatHistogramSTAppender{
		FloatHistogramAppender: FloatHistogramAppender{
			b: &c.b,

			schema:       it.schema,
			zThreshold:   it.zThreshold,
			pSpans:       it.pSpans,
			nSpans:       it.nSpans,
			customValues: it.customValues,
			t:            it.t,
			tDelta:       it.tDelta,
			cnt:          it.cnt,
			zCnt:         it.zCnt,
			pBuckets:     pBuckets,
			nBuckets:     nBuckets,
			sum:          it.sum,
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

func newFloatHistogramSTIterator(b []byte) *floatHistogramSTIterator {
	it := &floatHistogramSTIterator{
		floatHistogramIterator: floatHistogramIterator{
			br:       newBReader(b[histogramHeaderSize:]),
			numTotal: binary.BigEndian.Uint16(b) & 0x3FFF,
			t:        math.MinInt64,
		},
	}
	it.counterResetHeader = CounterResetHeader(b[0] & CounterResetHeaderMask)
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramHeaderSize-1:])
	return it
}

func (c *FloatHistogramSTChunk) iterator(it Iterator) *floatHistogramSTIterator {
	if fhIter, ok := it.(*floatHistogramSTIterator); ok {
		fhIter.Reset(c.b.bytes())
		return fhIter
	}
	return newFloatHistogramSTIterator(c.b.bytes())
}

// Iterator implements the Chunk interface.
func (c *FloatHistogramSTChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// FloatHistogramSTAppender is an Appender for float histogram samples with start timestamp support.
// It embeds FloatHistogramAppender and adds ST encoding after each sample.
type FloatHistogramSTAppender struct {
	FloatHistogramAppender
	stEncoder
}

// GetCounterResetHeader returns the counter-reset header from bits 7-6 of byte 0.
func (a *FloatHistogramSTAppender) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(a.b.bytes()[0] & CounterResetHeaderMask)
}

// setCounterResetHeader writes the counter-reset header into bits 7-6 of byte 0.
func (a *FloatHistogramSTAppender) setCounterResetHeader(cr CounterResetHeader) {
	b := a.b.bytes()
	b[0] = (b[0] &^ CounterResetHeaderMask) | (byte(cr) & CounterResetHeaderMask)
}

// NumSamples returns the number of samples in the chunk. Since the counter-reset header
// is in the top 2 bits of the sample count word, so samples count occupies only the low 14 bits.
func (a *FloatHistogramSTAppender) NumSamples() int {
	return int(binary.BigEndian.Uint16(a.b.bytes()) & histogramSTSampleCountMask)
}

func (a *FloatHistogramSTAppender) appendable(h *histogram.FloatHistogram) (
	positiveInserts, negativeInserts []Insert,
	backwardPositiveInserts, backwardNegativeInserts []Insert,
	okToAppend, counterReset bool,
) {
	if a.NumSamples() > 0 && a.GetCounterResetHeader() == GaugeType {
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}
	if h.CounterResetHint == histogram.CounterReset {
		// Always honor the explicit counter reset hint.
		counterReset = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}
	if value.IsStaleNaN(h.Sum) {
		// This is a stale sample whose buckets and spans don't matter.
		okToAppend = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}
	if value.IsStaleNaN(a.sum.value) {
		// If the last sample was stale, then we can only accept stale
		// samples in this chunk.
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}

	if h.Count < a.cnt.value {
		// There has been a counter reset.
		counterReset = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}

	if h.Schema != a.schema || h.ZeroThreshold != a.zThreshold {
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}

	if histogram.IsCustomBucketsSchema(h.Schema) && !histogram.CustomBucketBoundsMatch(h.CustomValues, a.customValues) {
		counterReset = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}

	if h.ZeroCount < a.zCnt.value {
		// There has been a counter reset since ZeroThreshold didn't change.
		counterReset = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}

	var ok bool
	positiveInserts, backwardPositiveInserts, ok = expandFloatSpansAndBuckets(a.pSpans, h.PositiveSpans, a.pBuckets, h.PositiveBuckets)
	if !ok {
		counterReset = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}
	negativeInserts, backwardNegativeInserts, ok = expandFloatSpansAndBuckets(a.nSpans, h.NegativeSpans, a.nBuckets, h.NegativeBuckets)
	if !ok {
		counterReset = true
		return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
	}

	okToAppend = true
	return positiveInserts, negativeInserts, backwardPositiveInserts, backwardNegativeInserts, okToAppend, counterReset
}

func (a *FloatHistogramSTAppender) appendableGauge(h *histogram.FloatHistogram) (
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
	if value.IsStaleNaN(a.sum.value) {
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

func (a *FloatHistogramSTAppender) appendFloatHistogram(t int64, h *histogram.FloatHistogram) {
	var tDelta int64
	num := binary.BigEndian.Uint16(a.b.bytes()) & 0x3FFF

	if value.IsStaleNaN(h.Sum) {
		// Emptying out other fields to write no buckets, and an empty
		// layout in case of first histogram in the chunk.
		h = &histogram.FloatHistogram{Sum: h.Sum}
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
			a.pBuckets = make([]xorValue, numPBuckets)
			for i := range numPBuckets {
				a.pBuckets[i] = xorValue{
					value:   h.PositiveBuckets[i],
					leading: 0xff,
				}
			}
		} else {
			a.pBuckets = nil
		}
		if numNBuckets > 0 {
			a.nBuckets = make([]xorValue, numNBuckets)
			for i := range numNBuckets {
				a.nBuckets[i] = xorValue{
					value:   h.NegativeBuckets[i],
					leading: 0xff,
				}
			}
		} else {
			a.nBuckets = nil
		}

		// Now store the actual data.
		putVarbitInt(a.b, t)
		a.b.writeBits(math.Float64bits(h.Count), 64)
		a.b.writeBits(math.Float64bits(h.ZeroCount), 64)
		a.b.writeBits(math.Float64bits(h.Sum), 64)
		a.cnt.value = h.Count
		a.zCnt.value = h.ZeroCount
		a.sum.value = h.Sum
		for _, b := range h.PositiveBuckets {
			a.b.writeBits(math.Float64bits(b), 64)
		}
		for _, b := range h.NegativeBuckets {
			a.b.writeBits(math.Float64bits(b), 64)
		}
	} else {
		// The case for the 2nd sample with single deltas is implicitly handled correctly with the double delta code,
		// so we don't need a separate single delta logic for the 2nd sample.
		tDelta = t - a.t
		tDod := tDelta - a.tDelta
		putVarbitInt(a.b, tDod)

		a.writeXorValue(&a.cnt, h.Count)
		a.writeXorValue(&a.zCnt, h.ZeroCount)
		a.writeXorValue(&a.sum, h.Sum)

		for i, b := range h.PositiveBuckets {
			a.writeXorValue(&a.pBuckets[i], b)
		}
		for i, b := range h.NegativeBuckets {
			a.writeXorValue(&a.nBuckets[i], b)
		}
	}

	// Write the incremented count back, preserving the counter-reset bits in
	// the top 2 bits of byte 0.
	buf := a.b.bytes()
	crBits := buf[0] & CounterResetHeaderMask
	binary.BigEndian.PutUint16(buf, (uint16(crBits)<<8)|(num+1))

	a.t = t
	a.tDelta = tDelta
}

// appendFloatHistogramST encodes a float histogram sample with start timestamp.
func (a *FloatHistogramSTAppender) appendFloatHistogramST(st, t int64, fh *histogram.FloatHistogram) {
	prevT := a.t
	a.appendFloatHistogram(t, fh)
	num := binary.BigEndian.Uint16(a.b.bytes()) & 0x3FFF
	a.encode(a.b, num, a.t, prevT, st)
}

// Append implements Appender. This implementation panics because normal float
// samples must never be appended to a float histogram chunk.
func (*FloatHistogramSTAppender) Append(int64, int64, float64) {
	panic("appended a float sample to a float histogram chunk")
}

// AppendHistogram implements Appender. This implementation panics because integer
// histogram samples must never be appended to a float histogram chunk.
func (*FloatHistogramSTAppender) AppendHistogram(Appender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float histogram chunk")
}

// AppendFloatHistogram implements Appender for FloatHistogramSTAppender.
func (a *FloatHistogramSTAppender) AppendFloatHistogram(prev Appender, st, t int64, fh *histogram.FloatHistogram, appendOnly bool) (Chunk, bool, Appender, error) {
	numSamples := a.NumSamples()

	// ST chunks store the sample count in the low 14 bits (the high 2 are the
	// counter-reset header), so the capacity is histogramSTSampleCountMask rather
	// than math.MaxUint16.
	if numSamples == histogramSTSampleCountMask {
		panic("chunk capacity exceeded")
	}

	if numSamples == 0 {
		a.appendFloatHistogramST(st, t, fh)
		if fh.CounterResetHint == histogram.GaugeType {
			a.setCounterResetHeader(GaugeType)
			return nil, false, a, nil
		}

		switch {
		case fh.CounterResetHint == histogram.CounterReset:
			a.setCounterResetHeader(CounterReset)
		case prev != nil:
			if p, ok := prev.(floatHistogramAppendable); ok {
				_, _, _, _, _, counterReset := p.appendable(fh)
				if counterReset {
					a.setCounterResetHeader(CounterReset)
				} else {
					a.setCounterResetHeader(NotCounterReset)
				}
			}
		}
		return nil, false, a, nil
	}

	// Adding counter-like histogram.
	if fh.CounterResetHint != histogram.GaugeType {
		pForwardInserts, nForwardInserts, pBackwardInserts, nBackwardInserts, okToAppend, counterReset := a.appendable(fh)
		if !okToAppend || counterReset {
			if appendOnly {
				if counterReset {
					return nil, false, a, errors.New("float histogram counter reset")
				}
				return nil, false, a, errors.New("float histogram schema change")
			}
			newChunk := NewFloatHistogramSTChunk()
			app, err := newChunk.Appender()
			if err != nil {
				panic(err)
			}
			happ := app.(*FloatHistogramSTAppender)
			if counterReset {
				happ.setCounterResetHeader(CounterReset)
			}
			happ.appendFloatHistogramST(st, t, fh)
			return newChunk, false, app, nil
		}
		if len(pBackwardInserts) > 0 || len(nBackwardInserts) > 0 {
			if len(pForwardInserts) == 0 && len(nForwardInserts) == 0 {
				fh.PositiveSpans = make([]histogram.Span, len(a.pSpans))
				copy(fh.PositiveSpans, a.pSpans)
				fh.NegativeSpans = make([]histogram.Span, len(a.nSpans))
				copy(fh.NegativeSpans, a.nSpans)
			} else {
				fh.PositiveSpans = adjustForInserts(fh.PositiveSpans, pBackwardInserts)
				fh.NegativeSpans = adjustForInserts(fh.NegativeSpans, nBackwardInserts)
			}
			a.recodeHistogram(fh, pBackwardInserts, nBackwardInserts)
		}
		if len(pForwardInserts) > 0 || len(nForwardInserts) > 0 {
			if appendOnly {
				return nil, false, a, fmt.Errorf("float histogram layout change with %d positive and %d negative forwards inserts", len(pForwardInserts), len(nForwardInserts))
			}
			chk, app := a.recodeST(
				pForwardInserts, nForwardInserts,
				fh.PositiveSpans, fh.NegativeSpans,
			)
			app.(*FloatHistogramSTAppender).appendFloatHistogramST(st, t, fh)
			return chk, true, app, nil
		}
		a.appendFloatHistogramST(st, t, fh)
		return nil, false, a, nil
	}

	// Adding gauge histogram.
	pForwardInserts, nForwardInserts, pBackwardInserts, nBackwardInserts, pMergedSpans, nMergedSpans, okToAppend := a.appendableGauge(fh)
	if !okToAppend {
		if appendOnly {
			return nil, false, a, errors.New("float gauge histogram schema change")
		}
		newChunk := NewFloatHistogramSTChunk()
		app, err := newChunk.Appender()
		if err != nil {
			panic(err)
		}
		happ := app.(*FloatHistogramSTAppender)
		happ.setCounterResetHeader(GaugeType)
		happ.appendFloatHistogramST(st, t, fh)
		return newChunk, false, app, nil
	}

	if len(pBackwardInserts)+len(nBackwardInserts) > 0 {
		if appendOnly {
			return nil, false, a, fmt.Errorf("float gauge histogram layout change with %d positive and %d negative backwards inserts", len(pBackwardInserts), len(nBackwardInserts))
		}
		fh.PositiveSpans = pMergedSpans
		fh.NegativeSpans = nMergedSpans
		a.recodeHistogram(fh, pBackwardInserts, nBackwardInserts)
	}

	if len(pForwardInserts) > 0 || len(nForwardInserts) > 0 {
		if appendOnly {
			return nil, false, a, fmt.Errorf("float gauge histogram layout change with %d positive and %d negative forwards inserts", len(pForwardInserts), len(nForwardInserts))
		}
		chk, app := a.recodeST(
			pForwardInserts, nForwardInserts,
			fh.PositiveSpans, fh.NegativeSpans,
		)
		app.(*FloatHistogramSTAppender).appendFloatHistogramST(st, t, fh)
		return chk, true, app, nil
	}

	a.appendFloatHistogramST(st, t, fh)
	return nil, false, a, nil
}

// recodeST is like FloatHistogramAppender.recode but creates FloatHistogramSTChunk and preserves ST.
func (a *FloatHistogramSTAppender) recodeST(
	positiveInserts, negativeInserts []Insert,
	positiveSpans, negativeSpans []histogram.Span,
) (Chunk, Appender) {
	byts := a.b.bytes()
	it := newFloatHistogramSTIterator(byts)
	hc := NewFloatHistogramSTChunk()
	app, err := hc.Appender()
	if err != nil {
		panic(err)
	}
	happ := app.(*FloatHistogramSTAppender)
	numPositiveBuckets, numNegativeBuckets := countSpans(positiveSpans), countSpans(negativeSpans)

	for it.Next() == ValFloatHistogram {
		tOld, fhOld := it.AtFloatHistogram(nil)
		stOld := it.AtST()

		var positiveBuckets, negativeBuckets []float64
		if numPositiveBuckets > 0 {
			positiveBuckets = make([]float64, numPositiveBuckets)
		}
		if numNegativeBuckets > 0 {
			negativeBuckets = make([]float64, numNegativeBuckets)
		}

		fhOld.PositiveSpans, fhOld.NegativeSpans = positiveSpans, negativeSpans
		if len(positiveInserts) > 0 {
			fhOld.PositiveBuckets = insert(fhOld.PositiveBuckets, positiveBuckets, positiveInserts, false)
		}
		if len(negativeInserts) > 0 {
			fhOld.NegativeBuckets = insert(fhOld.NegativeBuckets, negativeBuckets, negativeInserts, false)
		}
		happ.appendFloatHistogramST(stOld, tOld, fhOld)
	}

	happ.setCounterResetHeader(CounterResetHeader(byts[0] & CounterResetHeaderMask))
	return hc, app
}

// floatHistogramSTIterator is an iterator for FloatHistogramSTChunk that decodes ST after each sample.
type floatHistogramSTIterator struct {
	floatHistogramIterator
	stDecoder
}

// AtST returns the start timestamp for the current sample.
func (it *floatHistogramSTIterator) AtST() int64 {
	return it.st
}

// Reset resets the iterator for reuse.
func (it *floatHistogramSTIterator) Reset(b []byte) {
	it.stDecoder = stDecoder{}
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramHeaderSize-1:])

	// Reset the embedded floatHistogramIterator but with the correct header offset.
	it.br = newBReader(b[histogramHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b) & 0x3FFF
	it.numRead = 0

	it.counterResetHeader = CounterResetHeader(b[0] & CounterResetHeaderMask)

	it.t, it.tDelta = 0, 0
	it.cnt, it.zCnt, it.sum = xorValue{}, xorValue{}, xorValue{}

	if it.atFloatHistogramCalled {
		it.atFloatHistogramCalled = false
		it.pBuckets, it.nBuckets = nil, nil
		it.pSpans, it.nSpans = nil, nil
		it.customValues = nil
	} else {
		it.pBuckets, it.nBuckets = it.pBuckets[:0], it.nBuckets[:0]
	}
	it.pBucketsLeading, it.pBucketsTrailing = it.pBucketsLeading[:0], it.pBucketsTrailing[:0]
	it.nBucketsLeading, it.nBucketsTrailing = it.nBucketsLeading[:0], it.nBucketsTrailing[:0]

	it.err = nil
}

// Next advances the iterator by one sample.
// It calls the embedded floatHistogramIterator.Next() to decode the float histogram sample,
// then decodes the ST data that follows in the bitstream.
func (it *floatHistogramSTIterator) Next() ValueType {
	prevT := it.t
	vt := it.floatHistogramIterator.Next()
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
func (it *floatHistogramSTIterator) Seek(t int64) ValueType {
	if it.err != nil {
		return ValNone
	}
	for t > it.t || it.numRead == 0 {
		if it.Next() == ValNone {
			return ValNone
		}
	}
	return ValFloatHistogram
}
