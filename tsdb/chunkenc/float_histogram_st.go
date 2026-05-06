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
)

// FloatHistogramSTChunk is a chunk for float histogram samples with start timestamp (ST) support.
// It extends the FloatHistogramChunk format with a 1-byte ST header after the flags byte.
//
// Header layout (4 bytes):
//
//	bytes 0-1: sample count (big-endian uint16)
//	byte 2:    flags (bits 7-6 = counter reset header)
//	byte 3:    ST header (bit 7 = firstSTKnown, bits 6-0 = firstSTChangeOn)
type FloatHistogramSTChunk struct {
	b bstream
}

// NewFloatHistogramSTChunk returns a new empty FloatHistogramSTChunk.
func NewFloatHistogramSTChunk() *FloatHistogramSTChunk {
	b := make([]byte, histogramSTHeaderSize, chunkAllocationSize)
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
	return int(binary.BigEndian.Uint16(c.b.bytes()))
}

// GetCounterResetHeader returns the counter reset header from the flags byte.
func (c *FloatHistogramSTChunk) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(c.b.bytes()[histogramFlagPos] & CounterResetHeaderMask)
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
	if len(c.b.stream) == histogramSTHeaderSize {
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
			firstSTChangeOn: uint16(it.firstSTChangeOn),
		},
	}
	return a, nil
}

func newFloatHistogramSTIterator(b []byte) *floatHistogramSTIterator {
	it := &floatHistogramSTIterator{
		floatHistogramIterator: floatHistogramIterator{
			br:       newBReader(b[histogramSTHeaderSize:]),
			numTotal: binary.BigEndian.Uint16(b),
			t:        math.MinInt64,
		},
	}
	it.counterResetHeader = CounterResetHeader(b[histogramFlagPos] & CounterResetHeaderMask)
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramSTHeaderSize-1:])
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

// appendFloatHistogramST encodes a float histogram sample with start timestamp.
func (a *FloatHistogramSTAppender) appendFloatHistogramST(st, t int64, fh *histogram.FloatHistogram) {
	prevT := a.t
	num := a.appendFloatHistogram(a.NumSamples(), t, fh)
	a.setNumSamples(num)
	a.encode(a.b, uint16(num), a.t, prevT, st)
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
	if a.NumSamples() == 0 {
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

	happ.setCounterResetHeader(CounterResetHeader(byts[histogramFlagPos] & CounterResetHeaderMask))
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
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramSTHeaderSize-1:])

	// Reset the embedded floatHistogramIterator but with the correct header offset.
	it.br = newBReader(b[histogramSTHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.numRead = 0

	it.counterResetHeader = CounterResetHeader(b[histogramFlagPos] & CounterResetHeaderMask)

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
