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

const histogramSTHeaderSize = 4

// HistogramSTChunk is a chunk for histogram samples with start timestamp (ST) support.
// It extends the HistogramChunk format with a 1-byte ST header after the flags byte.
//
// Header layout (4 bytes):
//
//	bytes 0-1: sample count (big-endian uint16)
//	byte 2:    flags (bits 7-6 = counter reset header)
//	byte 3:    ST header (bit 7 = firstSTKnown, bits 6-0 = firstSTChangeOn)
type HistogramSTChunk struct {
	b bstream
}

// NewHistogramSTChunk returns a new empty HistogramSTChunk.
func NewHistogramSTChunk() *HistogramSTChunk {
	b := make([]byte, histogramSTHeaderSize, chunkAllocationSize)
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
	return int(binary.BigEndian.Uint16(c.b.bytes()))
}

// GetCounterResetHeader returns the counter reset header from the flags byte.
func (c *HistogramSTChunk) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(c.b.bytes()[histogramFlagPos] & CounterResetHeaderMask)
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
	if len(c.b.stream) == histogramSTHeaderSize {
		return &HistogramSTAppender{
			HistogramAppender: HistogramAppender{
				b:       &c.b,
				t:       math.MinInt64,
				leading: 0xff},
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
			b: &c.b,

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
		st:              it.st,
		stDiff:          it.stDiff,
		firstSTKnown:    it.firstSTKnown,
		firstSTChangeOn: uint16(it.firstSTChangeOn),
	}
	return a, nil
}

func newHistogramSTIterator(b []byte) *histogramSTIterator {
	it := &histogramSTIterator{
		histogramIterator: histogramIterator{
			br:       newBReader(b[histogramSTHeaderSize:]),
			numTotal: binary.BigEndian.Uint16(b),
			t:        math.MinInt64,
		},
	}
	it.counterResetHeader = CounterResetHeader(b[histogramFlagPos] & CounterResetHeaderMask)
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramSTHeaderSize-1:])
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

	st              int64
	stDiff          int64
	firstSTChangeOn uint16
	firstSTKnown    bool
}

// encodeST encodes the start timestamp for the current sample.
// It must be called after appendHistogram() which increments the sample count.
// prevT is the timestamp of the previous sample (before appendHistogram updated a.t).
// For sample 0, prevT is unused.
func (a *HistogramSTAppender) encodeST(prevT, st int64) {
	num := binary.BigEndian.Uint16(a.b.bytes())

	switch {
	case num == 1: // First sample (count was just incremented from 0).
		if st != 0 {
			buf := make([]byte, binary.MaxVarintLen64)
			for _, b := range buf[:binary.PutVarint(buf, a.t-st)] {
				a.b.writeByte(b)
			}
			a.firstSTKnown = true
			writeHeaderFirstSTKnown(a.b.bytes()[histogramSTHeaderSize-1:])
		}
	case num == 2: // Second sample.
		if st != a.st {
			stDiff := prevT - st
			a.firstSTChangeOn = 1
			writeHeaderFirstSTChangeOn(a.b.bytes()[histogramSTHeaderSize-1:], 1)
			putVarbitInt(a.b, stDiff)
			a.stDiff = stDiff
		}
	default: // Sample N >= 2.
		// Fast path: no ST data to write.
		if st == 0 && num-1 != maxFirstSTChangeOn && a.firstSTChangeOn == 0 && !a.firstSTKnown {
			break
		}
		if a.firstSTChangeOn == 0 {
			if st != a.st || num-1 == maxFirstSTChangeOn {
				stDiff := prevT - st
				a.firstSTChangeOn = num - 1
				writeHeaderFirstSTChangeOn(a.b.bytes()[histogramSTHeaderSize-1:], num-1)
				putVarbitInt(a.b, stDiff)
				a.stDiff = stDiff
			}
		} else {
			stDiff := prevT - st
			putVarbitInt(a.b, stDiff-a.stDiff)
			a.stDiff = stDiff
		}
	}
	a.st = st
}

// appendHistogramST encodes a histogram sample with start timestamp.
func (a *HistogramSTAppender) appendHistogramST(st, t int64, h *histogram.Histogram) {
	prevT := a.t
	a.appendHistogram(t, h)
	a.encodeST(prevT, st)
}

func (*HistogramSTAppender) Append(int64, int64, float64) {
	panic("appended a float sample to a histogram chunk")
}

func (*HistogramSTAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a histogram chunk")
}

// AppendHistogram implements Appender for HistogramSTAppender.
func (a *HistogramSTAppender) AppendHistogram(prev *HistogramAppender, st, t int64, h *histogram.Histogram, appendOnly bool) (Chunk, bool, Appender, error) {
	if a.NumSamples() == 0 {
		a.appendHistogramST(st, t, h)
		if h.CounterResetHint == histogram.GaugeType {
			a.setCounterResetHeader(GaugeType)
			return nil, false, a, nil
		}

		switch {
		case h.CounterResetHint == histogram.CounterReset:
			a.setCounterResetHeader(CounterReset)
		case prev != nil:
			_, _, _, _, _, counterReset := prev.appendable(h)
			a.setCounterResetHeader(counterReset)
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

	happ.setCounterResetHeader(CounterResetHeader(byts[histogramFlagPos] & CounterResetHeaderMask))
	return hc, app
}

// histogramSTIterator is an iterator for HistogramSTChunk that decodes ST after each sample.
type histogramSTIterator struct {
	histogramIterator

	// ST fields.
	st              int64
	stDiff          int64
	firstSTKnown    bool
	firstSTChangeOn uint8
}

func (it *histogramSTIterator) AtST() int64 {
	return it.st
}

func (it *histogramSTIterator) Reset(b []byte) {
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[histogramSTHeaderSize-1:])
	it.st = 0
	it.stDiff = 0

	// Reset the embedded histogramIterator but with the correct header offset.
	it.br = newBReader(b[histogramSTHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.numRead = 0

	it.counterResetHeader = CounterResetHeader(b[histogramFlagPos] & CounterResetHeaderMask)

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
	if err := it.decodeST(it.numRead, prevT); err != nil {
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

// decodeST decodes the start timestamp for the current sample.
// numRead is the number of samples read so far (already incremented by histogramIterator.Next()).
// prevT is the timestamp of the previous sample (before histogramIterator.Next() updated it.t).
func (it *histogramSTIterator) decodeST(numRead uint16, prevT int64) error {
	switch {
	case numRead == 1: // After sample 0.
		if it.firstSTKnown {
			stDiff, err := it.br.readVarint()
			if err != nil {
				return err
			}
			it.stDiff = stDiff
			it.st = it.t - stDiff
		}
	case numRead == 2: // After sample 1.
		if it.firstSTChangeOn == 1 {
			sdod, err := readVarbitInt(&it.br)
			if err != nil {
				return err
			}
			it.stDiff = sdod
			it.st = prevT - sdod
		}
	default: // After sample N >= 2.
		if it.firstSTChangeOn > 0 && numRead-1 >= uint16(it.firstSTChangeOn) {
			sdod, err := readVarbitInt(&it.br)
			if err != nil {
				return err
			}
			if numRead-1 == uint16(it.firstSTChangeOn) {
				it.stDiff = sdod
			} else {
				it.stDiff += sdod
			}
			it.st = prevT - it.stDiff
		}
	}
	return nil
}
