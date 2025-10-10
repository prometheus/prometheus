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
	"errors"
	"fmt"
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
	b := make([]byte, histogramHeaderSize, chunkAllocationSize)
	return &HistogramChunk{b: bstream{stream: b, count: 0}}
}

func (c *HistogramChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*HistogramChunk) Encoding() Encoding {
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
	// CounterResetHeaderMask is the mask to get the counter reset header bits.
	CounterResetHeaderMask byte = 0b11000000
	// Position within the header bytes at the start of the stream.
	histogramFlagPos = 2
	// Total header size.
	histogramHeaderSize = 3
)

// GetCounterResetHeader returns the info about the first 2 bits of the chunk
// header.
func (c *HistogramChunk) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(c.Bytes()[histogramFlagPos] & CounterResetHeaderMask)
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
	if len(c.b.stream) == histogramHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &HistogramAppender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
	}
	it := c.iterator(nil)

	// To get an appender, we must know the state it would have if we had
	// appended all existing data from scratch. We iterate through the end
	// and populate via the iterator's state.
	for it.Next() == ValHistogram {
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
		br:       newBReader(b[histogramHeaderSize:]),
		numTotal: binary.BigEndian.Uint16(b),
		t:        math.MinInt64,
	}
	it.counterResetHeader = CounterResetHeader(b[histogramFlagPos] & CounterResetHeaderMask)
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
	// customValues is read only after the first sample is appended.
	customValues []float64

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
	return CounterResetHeader(a.b.bytes()[histogramFlagPos] & CounterResetHeaderMask)
}

func (a *HistogramAppender) setCounterResetHeader(cr CounterResetHeader) {
	a.b.bytes()[histogramFlagPos] = (a.b.bytes()[histogramFlagPos] & (^CounterResetHeaderMask)) | (byte(cr) & CounterResetHeaderMask)
}

func (a *HistogramAppender) NumSamples() int {
	return int(binary.BigEndian.Uint16(a.b.bytes()))
}

// Append implements Appender. This implementation panics because normal float
// samples must never be appended to a histogram chunk.
func (*HistogramAppender) Append(int64, float64) {
	panic("appended a float sample to a histogram chunk")
}

// appendable returns whether the chunk can be appended to, and if so whether
//  1. Any recoding needs to happen to the chunk using the provided forward
//     inserts (in case of any new buckets, positive or negative range,
//     respectively).
//  2. Any recoding needs to happen for the histogram being appended, using the
//     backward inserts (in case of any missing buckets, positive or negative
//     range, respectively).
//
// If the sample is a gauge histogram, AppendableGauge must be used instead.
//
// The chunk is not appendable in the following cases:
//
//   - The schema has changed.
//   - The custom bounds have changed if the current schema is custom buckets.
//   - The threshold for the zero bucket has changed.
//   - Any buckets have disappeared, unless the bucket count was 0, unused.
//     Empty bucket can happen if the chunk was recoded and we're merging a non
//     recoded histogram. In this case backward inserts will be provided.
//   - There was a counter reset in the count of observations or in any bucket,
//     including the zero bucket.
//   - The last sample in the chunk was stale while the current sample is not stale.
//
// The method returns an additional boolean set to true if it is not appendable
// because of a counter reset. If the given sample is stale, it is always ok to
// append. If counterReset is true, okToAppend is always false.
//
// The method returns an additional CounterResetHeader value that indicates the
// status of the counter reset detection. But it returns UnknownCounterReset
// when schema or zero threshold changed, because we don't do a full counter
// reset detection.
func (a *HistogramAppender) appendable(h *histogram.Histogram) (
	positiveInserts, negativeInserts []Insert,
	backwardPositiveInserts, backwardNegativeInserts []Insert,
	okToAppend bool, counterResetHint CounterResetHeader,
) {
	counterResetHint = NotCounterReset
	if a.NumSamples() > 0 && a.GetCounterResetHeader() == GaugeType {
		return
	}
	if h.CounterResetHint == histogram.CounterReset {
		// Always honor the explicit counter reset hint.
		counterResetHint = CounterReset
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
		counterResetHint = UnknownCounterReset
		return
	}

	if h.Count < a.cnt {
		// There has been a counter reset.
		counterResetHint = CounterReset
		return
	}

	if h.Schema != a.schema || h.ZeroThreshold != a.zThreshold {
		// This case might or might not go along with a counter reset and
		// we do not want to invest the work of a full counter reset detection
		// as long as https://github.com/prometheus/prometheus/issues/15346 is still open.
		// TODO: consider adding the counter reset detection here once #15346 is fixed.
		counterResetHint = UnknownCounterReset
		return
	}

	if histogram.IsCustomBucketsSchema(h.Schema) && !histogram.CustomBucketBoundsMatch(h.CustomValues, a.customValues) {
		counterResetHint = CounterReset
		return
	}

	if h.ZeroCount < a.zCnt {
		// There has been a counter reset since ZeroThreshold didn't change.
		counterResetHint = CounterReset
		return
	}

	var ok bool
	positiveInserts, backwardPositiveInserts, ok = expandIntSpansAndBuckets(a.pSpans, h.PositiveSpans, a.pBuckets, h.PositiveBuckets)
	if !ok {
		counterResetHint = CounterReset
		return
	}
	negativeInserts, backwardNegativeInserts, ok = expandIntSpansAndBuckets(a.nSpans, h.NegativeSpans, a.nBuckets, h.NegativeBuckets)
	if !ok {
		counterResetHint = CounterReset
		return
	}

	okToAppend = true
	return
}

// expandIntSpansAndBuckets returns the inserts to expand the bucket spans 'a' so that
// they match the spans in 'b'. 'b' must cover the same or more buckets than
// 'a', otherwise the function will return false.
// The function also returns the inserts to expand 'b' to also cover all the
// buckets that are missing in 'b', but are present with 0 counter value in 'a'.
// The function also checks for counter resets between 'a' and 'b'.
//
// Example:
//
// Let's say the old buckets look like this:
//
//	span syntax: [offset, length]
//	spans      : [ 0 , 2 ]               [2,1]                   [ 3 , 2 ]                     [3,1]       [1,1]
//	bucket idx : [0]   [1]    2     3    [4]    5     6     7    [8]   [9]    10    11    12   [13]   14   [15]
//	raw values    6     3                 3                       2     4                       5           1
//	deltas        6    -3                 0                      -1     2                       1          -4
//
// But now we introduce a new bucket layout. (Carefully chosen example where we
// have a span appended, one unchanged[*], one prepended, and two merge - in
// that order.)
//
// [*] unchanged in terms of which bucket indices they represent. but to achieve
// that, their offset needs to change if "disrupted" by spans changing ahead of
// them
//
//	                                      \/ this one is "unchanged"
//	spans      : [  0  ,  3    ]         [1,1]       [    1    ,   4     ]                     [  3  ,   3    ]
//	bucket idx : [0]   [1]   [2]    3    [4]    5    [6]   [7]   [8]   [9]    10    11    12   [13]  [14]  [15]
//	raw values    6     3     0           3           0     0     2     4                       5     0     1
//	deltas        6    -3    -3           3          -3     0     2     2                       1    -5     1
//	delta mods:                          / \                     / \                                       / \
//
// Note for histograms with delta-encoded buckets: Whenever any new buckets are
// introduced, the subsequent "old" bucket needs to readjust its delta to the
// new base of 0. Thus, for the caller who wants to transform the set of
// original deltas to a new set of deltas to match a new span layout that adds
// buckets, we simply need to generate a list of inserts.
//
// Note: Within expandIntSpansAndBuckets we don't have to worry about the changes to the
// spans themselves, thanks to the iterators we get to work with the more useful
// bucket indices (which of course directly correspond to the buckets we have to
// adjust).
func expandIntSpansAndBuckets(a, b []histogram.Span, aBuckets, bBuckets []int64) (forward, backward []Insert, ok bool) {
	ai := newBucketIterator(a)
	bi := newBucketIterator(b)

	var aInserts []Insert // To insert into buckets of a, to make up for missing buckets in b.
	var bInserts []Insert // To insert into buckets of b, to make up for missing empty(!) buckets in a.

	// When aInter.num or bInter.num becomes > 0, this becomes a valid insert that should
	// be yielded when we finish a streak of new buckets.
	var aInter Insert
	var bInter Insert

	aIdx, aOK := ai.Next()
	bIdx, bOK := bi.Next()

	// Bucket count. Initialize the absolute count and index into the
	// positive/negative counts or deltas array. The bucket count is
	// used to detect counter reset as well as unused buckets in a.
	var (
		aCount    int64
		bCount    int64
		aCountIdx int
		bCountIdx int
	)
	if aOK {
		aCount = aBuckets[aCountIdx]
	}
	if bOK {
		bCount = bBuckets[bCountIdx]
	}

	// addInsert updates the current Insert with a new insert at the given
	// bucket index (otherIdx).
	addInsert := func(inserts []Insert, insert *Insert, otherIdx int) []Insert {
		if insert.num == 0 {
			// First insert.
			insert.bucketIdx = otherIdx
		} else if insert.bucketIdx+insert.num != otherIdx {
			// Insert is not continuous from previous insert.
			inserts = append(inserts, *insert)
			insert.num = 0
			insert.bucketIdx = otherIdx
		}
		insert.num++
		return inserts
	}

	advanceA := func() {
		if aInter.num > 0 {
			aInserts = append(aInserts, aInter)
			aInter.num = 0
		}
		aIdx, aOK = ai.Next()
		aInter.pos++
		aCountIdx++
		if aOK {
			aCount += aBuckets[aCountIdx]
		}
	}

	advanceB := func() {
		if bInter.num > 0 {
			bInserts = append(bInserts, bInter)
			bInter.num = 0
		}
		bIdx, bOK = bi.Next()
		bInter.pos++
		bCountIdx++
		if bOK {
			bCount += bBuckets[bCountIdx]
		}
	}

loop:
	for {
		switch {
		case aOK && bOK:
			switch {
			case aIdx == bIdx: // Both have an identical bucket index.
				// Bucket count. Check bucket for reset from a to b.
				if aCount > bCount {
					return nil, nil, false
				}

				advanceA()
				advanceB()

				continue
			case aIdx < bIdx: // b misses a bucket index that is in a.
				// This is ok if the count in a is 0, in which case we make a note to
				// fill in the bucket in b and advance a.
				if aCount == 0 {
					bInserts = addInsert(bInserts, &bInter, aIdx)
					advanceA()
					continue
				}
				// Otherwise we are missing a bucket that was in use in a, which is a reset.
				return nil, nil, false
			case aIdx > bIdx: // a misses a value that is in b. Forward b and recompare.
				aInserts = addInsert(aInserts, &aInter, bIdx)
				advanceB()
			}
		case aOK && !bOK: // b misses a value that is in a.
			// This is ok if the count in a is 0, in which case we make a note to
			// fill in the bucket in b and advance a.
			if aCount == 0 {
				bInserts = addInsert(bInserts, &bInter, aIdx)
				advanceA()
				continue
			}
			// Otherwise we are missing a bucket that was in use in a, which is a reset.
			return nil, nil, false
		case !aOK && bOK: // a misses a value that is in b. Forward b and recompare.
			aInserts = addInsert(aInserts, &aInter, bIdx)
			advanceB()
		default: // Both iterators ran out. We're done.
			if aInter.num > 0 {
				aInserts = append(aInserts, aInter)
			}
			if bInter.num > 0 {
				bInserts = append(bInserts, bInter)
			}
			break loop
		}
	}

	return aInserts, bInserts, true
}

// appendableGauge returns whether the chunk can be appended to, and if so
// whether:
//  1. Any recoding needs to happen to the chunk using the provided forward
//     inserts (in case of any new buckets, positive or negative range,
//     respectively).
//  2. Any recoding needs to happen for the histogram being appended, using the
//     backward inserts (in case of any missing buckets, positive or negative
//     range, respectively).
//
// This method must be only used for gauge histograms.
//
// The chunk is not appendable in the following cases:
//   - The schema has changed.
//   - The custom bounds have changed if the current schema is custom buckets.
//   - The threshold for the zero bucket has changed.
//   - The last sample in the chunk was stale while the current sample is not stale.
func (a *HistogramAppender) appendableGauge(h *histogram.Histogram) (
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

	if histogram.IsCustomBucketsSchema(h.Schema) && !histogram.CustomBucketBoundsMatch(h.CustomValues, a.customValues) {
		return
	}

	positiveInserts, backwardPositiveInserts, positiveSpans = expandSpansBothWays(a.pSpans, h.PositiveSpans)
	negativeInserts, backwardNegativeInserts, negativeSpans = expandSpansBothWays(a.nSpans, h.NegativeSpans)
	okToAppend = true
	return
}

// appendHistogram appends a histogram to the chunk. The caller must ensure that
// the histogram is properly structured, e.g. the number of buckets used
// corresponds to the number conveyed by the span structures. First call
// Appendable() and act accordingly!
func (a *HistogramAppender) appendHistogram(t int64, h *histogram.Histogram) {
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

// recode converts the current chunk to accommodate an expansion of the set of
// (positive and/or negative) buckets used, according to the provided inserts,
// resulting in the honoring of the provided new positive and negative spans. To
// continue appending, use the returned Appender rather than the receiver of
// this method.
func (a *HistogramAppender) recode(
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
		panic(err) // This should never happen for an empty histogram chunk.
	}
	happ := app.(*HistogramAppender)
	numPositiveBuckets, numNegativeBuckets := countSpans(positiveSpans), countSpans(negativeSpans)

	for it.Next() == ValHistogram {
		tOld, hOld := it.AtHistogram(nil)

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
		happ.appendHistogram(tOld, hOld)
	}

	happ.setCounterResetHeader(CounterResetHeader(byts[histogramFlagPos] & CounterResetHeaderMask))
	return hc, app
}

// recodeHistogram converts the current histogram (in-place) to accommodate an
// expansion of the set of (positive and/or negative) buckets used.
func (*HistogramAppender) recodeHistogram(
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

func (*HistogramAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a histogram chunk")
}

func (a *HistogramAppender) AppendHistogram(prev *HistogramAppender, t int64, h *histogram.Histogram, appendOnly bool) (Chunk, bool, Appender, error) {
	if a.NumSamples() == 0 {
		a.appendHistogram(t, h)
		if h.CounterResetHint == histogram.GaugeType {
			a.setCounterResetHeader(GaugeType)
			return nil, false, a, nil
		}

		switch {
		case h.CounterResetHint == histogram.CounterReset:
			// Always honor the explicit counter reset hint.
			a.setCounterResetHeader(CounterReset)
		case prev != nil:
			// This is a new chunk, but continued from a previous one. We need to calculate the reset header unless already set.
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
			newChunk := NewHistogramChunk()
			app, err := newChunk.Appender()
			if err != nil {
				panic(err) // This should never happen for an empty histogram chunk.
			}
			happ := app.(*HistogramAppender)
			happ.setCounterResetHeader(counterResetHint)
			happ.appendHistogram(t, h)
			return newChunk, false, app, nil
		}
		if len(pBackwardInserts) > 0 || len(nBackwardInserts) > 0 {
			// The histogram needs to be expanded to have the extra empty buckets
			// of the chunk.
			if len(pForwardInserts) == 0 && len(nForwardInserts) == 0 {
				// No new buckets from the histogram, so the spans of the appender can accommodate the new buckets.
				// However we need to make a copy in case the input is sharing spans from an iterator.
				h.PositiveSpans = make([]histogram.Span, len(a.pSpans))
				copy(h.PositiveSpans, a.pSpans)
				h.NegativeSpans = make([]histogram.Span, len(a.nSpans))
				copy(h.NegativeSpans, a.nSpans)
			} else {
				// Spans need pre-adjusting to accommodate the new buckets.
				h.PositiveSpans = adjustForInserts(h.PositiveSpans, pBackwardInserts)
				h.NegativeSpans = adjustForInserts(h.NegativeSpans, nBackwardInserts)
			}
			a.recodeHistogram(h, pBackwardInserts, nBackwardInserts)
		}
		if len(pForwardInserts) > 0 || len(nForwardInserts) > 0 {
			if appendOnly {
				return nil, false, a, fmt.Errorf("histogram layout change with %d positive and %d negative forwards inserts", len(pForwardInserts), len(nForwardInserts))
			}
			chk, app := a.recode(
				pForwardInserts, nForwardInserts,
				h.PositiveSpans, h.NegativeSpans,
			)
			app.(*HistogramAppender).appendHistogram(t, h)
			return chk, true, app, nil
		}
		a.appendHistogram(t, h)
		return nil, false, a, nil
	}
	// Adding gauge histogram.
	pForwardInserts, nForwardInserts, pBackwardInserts, nBackwardInserts, pMergedSpans, nMergedSpans, okToAppend := a.appendableGauge(h)
	if !okToAppend {
		if appendOnly {
			return nil, false, a, errors.New("gauge histogram schema change")
		}
		newChunk := NewHistogramChunk()
		app, err := newChunk.Appender()
		if err != nil {
			panic(err) // This should never happen for an empty histogram chunk.
		}
		happ := app.(*HistogramAppender)
		happ.setCounterResetHeader(GaugeType)
		happ.appendHistogram(t, h)
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
		chk, app := a.recode(
			pForwardInserts, nForwardInserts,
			h.PositiveSpans, h.NegativeSpans,
		)
		app.(*HistogramAppender).appendHistogram(t, h)
		return chk, true, app, nil
	}

	a.appendHistogram(t, h)
	return nil, false, a, nil
}

func CounterResetHintToHeader(hint histogram.CounterResetHint) CounterResetHeader {
	switch hint {
	case histogram.CounterReset:
		return CounterReset
	case histogram.NotCounterReset:
		return NotCounterReset
	case histogram.GaugeType:
		return GaugeType
	default:
		return UnknownCounterReset
	}
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
	customValues   []float64

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

func (*histogramIterator) At() (int64, float64) {
	panic("cannot call histogramIterator.At")
}

func (it *histogramIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	if value.IsStaleNaN(it.sum) {
		return it.t, &histogram.Histogram{Sum: it.sum}
	}
	if h == nil {
		it.atHistogramCalled = true
		h = &histogram.Histogram{
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
			CustomValues:     it.customValues,
		}
		if h.Schema > histogram.ExponentialSchemaMax && h.Schema <= histogram.ExponentialSchemaMaxReserved {
			// This is a very slow path, but it should only happen if the
			// chunk is from a newer Prometheus version that supports higher
			// resolution.
			h = h.Copy()
			h.ReduceResolution(histogram.ExponentialSchemaMax)
		}
		return it.t, h
	}

	h.CounterResetHint = counterResetHint(it.counterResetHeader, it.numRead)
	h.Schema = it.schema
	h.ZeroThreshold = it.zThreshold
	h.ZeroCount = it.zCnt
	h.Count = it.cnt
	h.Sum = it.sum

	h.PositiveSpans = resize(h.PositiveSpans, len(it.pSpans))
	copy(h.PositiveSpans, it.pSpans)

	h.NegativeSpans = resize(h.NegativeSpans, len(it.nSpans))
	copy(h.NegativeSpans, it.nSpans)

	h.PositiveBuckets = resize(h.PositiveBuckets, len(it.pBuckets))
	copy(h.PositiveBuckets, it.pBuckets)

	h.NegativeBuckets = resize(h.NegativeBuckets, len(it.nBuckets))
	copy(h.NegativeBuckets, it.nBuckets)

	// Custom values are interned. The single copy is here in the iterator.
	h.CustomValues = it.customValues

	if h.Schema > histogram.ExponentialSchemaMax && h.Schema <= histogram.ExponentialSchemaMaxReserved {
		// This is a very slow path, but it should only happen if the
		// chunk is from a newer Prometheus version that supports higher
		// resolution.
		h.ReduceResolution(histogram.ExponentialSchemaMax)
	}

	return it.t, h
}

func (it *histogramIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	if value.IsStaleNaN(it.sum) {
		return it.t, &histogram.FloatHistogram{Sum: it.sum}
	}
	if fh == nil {
		it.atFloatHistogramCalled = true
		fh = &histogram.FloatHistogram{
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
			CustomValues:     it.customValues,
		}
		if fh.Schema > histogram.ExponentialSchemaMax && fh.Schema <= histogram.ExponentialSchemaMaxReserved {
			// This is a very slow path, but it should only happen if the
			// chunk is from a newer Prometheus version that supports higher
			// resolution.
			fh = fh.Copy()
			fh.ReduceResolution(histogram.ExponentialSchemaMax)
		}
		return it.t, fh
	}

	fh.CounterResetHint = counterResetHint(it.counterResetHeader, it.numRead)
	fh.Schema = it.schema
	fh.ZeroThreshold = it.zThreshold
	fh.ZeroCount = float64(it.zCnt)
	fh.Count = float64(it.cnt)
	fh.Sum = it.sum

	fh.PositiveSpans = resize(fh.PositiveSpans, len(it.pSpans))
	copy(fh.PositiveSpans, it.pSpans)

	fh.NegativeSpans = resize(fh.NegativeSpans, len(it.nSpans))
	copy(fh.NegativeSpans, it.nSpans)

	fh.PositiveBuckets = resize(fh.PositiveBuckets, len(it.pBuckets))
	var currentPositive float64
	for i, b := range it.pBuckets {
		currentPositive += float64(b)
		fh.PositiveBuckets[i] = currentPositive
	}

	fh.NegativeBuckets = resize(fh.NegativeBuckets, len(it.nBuckets))
	var currentNegative float64
	for i, b := range it.nBuckets {
		currentNegative += float64(b)
		fh.NegativeBuckets[i] = currentNegative
	}

	// Custom values are interned. The single copy is here in the iterator.
	fh.CustomValues = it.customValues

	if fh.Schema > histogram.ExponentialSchemaMax && fh.Schema <= histogram.ExponentialSchemaMaxReserved {
		// This is a very slow path, but it should only happen if the
		// chunk is from a newer Prometheus version that supports higher
		// resolution.
		fh.ReduceResolution(histogram.ExponentialSchemaMax)
	}

	return it.t, fh
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
	it.br = newBReader(b[histogramHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.numRead = 0

	it.counterResetHeader = CounterResetHeader(b[histogramFlagPos] & CounterResetHeaderMask)

	it.t, it.cnt, it.zCnt = 0, 0, 0
	it.tDelta, it.cntDelta, it.zCntDelta = 0, 0, 0

	// Recycle slices that have not been returned yet. Otherwise, start from
	// scratch.
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

func (it *histogramIterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	if it.numRead == 0 {
		// The first read is responsible for reading the chunk layout
		// and for initializing fields that depend on it. We give
		// counter reset info at chunk level, hence we discard it here.
		schema, zeroThreshold, posSpans, negSpans, customValues, err := readHistogramChunkLayout(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}

		if !histogram.IsKnownSchema(schema) {
			it.err = histogram.UnknownSchemaError(schema)
			return ValNone
		}

		it.schema = schema
		it.zThreshold = zeroThreshold
		it.pSpans, it.nSpans = posSpans, negSpans
		it.customValues = customValues
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

	// Recycle bucket, span and custom value slices that have not been returned yet. Otherwise, copy them.
	if it.atFloatHistogramCalled || it.atHistogramCalled {
		if len(it.pSpans) > 0 {
			newSpans := make([]histogram.Span, len(it.pSpans))
			copy(newSpans, it.pSpans)
			it.pSpans = newSpans
		} else {
			it.pSpans = nil
		}
		if len(it.nSpans) > 0 {
			newSpans := make([]histogram.Span, len(it.nSpans))
			copy(newSpans, it.nSpans)
			it.nSpans = newSpans
		} else {
			it.nSpans = nil
		}
		// it.CustomValues are interned, so we don't need to copy them.
	}

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

func resize[T any](items []T, n int) []T {
	if cap(items) < n {
		return make([]T, n)
	}
	return items[:n]
}
