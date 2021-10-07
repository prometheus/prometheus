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

// The code in this file was largely written by Damian Gryski as part of
// https://github.com/dgryski/go-tsz and published under the license below.
// It was modified to accommodate reading from byte slices without modifying
// the underlying bytes, which would panic when reading from mmap'd
// read-only byte slices.

// Copyright (c) 2015,2016 Damian Gryski <damian@gryski.com>
// All rights reserved.

// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:

// * Redistributions of source code must retain the above copyright notice,
// this list of conditions and the following disclaimer.
//
// * Redistributions in binary form must reproduce the above copyright notice,
// this list of conditions and the following disclaimer in the documentation
// and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package chunkenc

import (
	"encoding/binary"
	"math"
	"math/bits"

	"github.com/prometheus/prometheus/pkg/histogram"
	"github.com/prometheus/prometheus/pkg/value"
)

const ()

// HistoChunk holds sparse histogram encoded sample data.
// Appends a histogram sample
// * schema defines the resolution (number of buckets per power of 2)
//   Currently, valid numbers are -4 <= n <= 8.
//   They are all for base-2 bucket schemas, where 1 is a bucket boundary in each case, and
//   then each power of two is divided into 2^n logarithmic buckets.
//   Or in other words, each bucket boundary is the previous boundary times 2^(2^-n).
//   In the future, more bucket schemas may be added using numbers < -4 or > 8.
// The bucket with upper boundary of 1 is always bucket 0.
// Then negative numbers for smaller boundaries and positive for uppers.
//
// fields are stored like so:
// field           ts    count zeroCount sum []posbuckets negbuckets
// observation 1   raw   raw   raw       raw []raw        []raw
// observation 2   delta delta delta     xor []delta      []delta
// observation >2  dod   dod   dod       xor []dod        []dod
// TODO zerothreshold
type HistoChunk struct {
	b bstream
}

// NewHistoChunk returns a new chunk with Histo encoding of the given size.
func NewHistoChunk() *HistoChunk {
	b := make([]byte, 3, 128)
	return &HistoChunk{b: bstream{stream: b, count: 0}}
}

// Encoding returns the encoding type.
func (c *HistoChunk) Encoding() Encoding {
	return EncSHS
}

// Bytes returns the underlying byte slice of the chunk.
func (c *HistoChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *HistoChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Meta returns the histogram metadata.
// callers may only call this on chunks that have at least one sample
func (c *HistoChunk) Meta() (int32, float64, []histogram.Span, []histogram.Span, error) {
	if c.NumSamples() == 0 {
		panic("HistoChunk.Meta() called on an empty chunk")
	}
	b := newBReader(c.Bytes()[2:])
	return readHistoChunkMeta(&b)
}

// CounterResetHeader defines the first 2 bits of the chunk header.
type CounterResetHeader byte

const (
	CounterReset        CounterResetHeader = 0b10000000
	NotCounterReset     CounterResetHeader = 0b01000000
	GaugeType           CounterResetHeader = 0b11000000
	UnknownCounterReset CounterResetHeader = 0b00000000
)

// SetCounterResetHeader sets the counter reset header.
func (c *HistoChunk) SetCounterResetHeader(h CounterResetHeader) {
	switch h {
	case CounterReset, NotCounterReset, GaugeType, UnknownCounterReset:
		bytes := c.Bytes()
		bytes[2] = (bytes[2] & 0b00111111) | byte(h)
	default:
		panic("invalid CounterResetHeader type")
	}
}

// GetCounterResetHeader returns the info about the first 2 bits of the chunk header.
func (c *HistoChunk) GetCounterResetHeader() CounterResetHeader {
	return CounterResetHeader(c.Bytes()[2] & 0b11000000)
}

// Compact implements the Chunk interface.
func (c *HistoChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *HistoChunk) Appender() (Appender, error) {
	it := c.iterator(nil)

	// To get an appender we must know the state it would have if we had
	// appended all existing data from scratch.
	// We iterate through the end and populate via the iterator's state.
	for it.Next() {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	a := &HistoAppender{
		b: &c.b,

		schema:          it.schema,
		zeroThreshold:   it.zeroThreshold,
		posSpans:        it.posSpans,
		negSpans:        it.negSpans,
		t:               it.t,
		cnt:             it.cnt,
		zcnt:            it.zcnt,
		tDelta:          it.tDelta,
		cntDelta:        it.cntDelta,
		zcntDelta:       it.zcntDelta,
		posbuckets:      it.posbuckets,
		negbuckets:      it.negbuckets,
		posbucketsDelta: it.posbucketsDelta,
		negbucketsDelta: it.negbucketsDelta,

		sum:      it.sum,
		leading:  it.leading,
		trailing: it.trailing,

		buf64: make([]byte, binary.MaxVarintLen64),
	}
	if binary.BigEndian.Uint16(a.b.bytes()) == 0 {
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

func newHistoIterator(b []byte) *histoIterator {
	it := &histoIterator{
		br:       newBReader(b),
		numTotal: binary.BigEndian.Uint16(b),
		t:        math.MinInt64,
	}
	// The first 2 bytes contain chunk headers.
	// We skip that for actual samples.
	_, _ = it.br.readBits(16)
	return it
}

func (c *HistoChunk) iterator(it Iterator) *histoIterator {
	// TODO fix this. this is taken from xor.go // dieter not sure what the purpose of this is
	// Should iterators guarantee to act on a copy of the data so it doesn't lock append?
	// When using striped locks to guard access to chunks, probably yes.
	// Could only copy data if the chunk is not completed yet.
	//if histoIter, ok := it.(*histoIterator); ok {
	//	histoIter.Reset(c.b.bytes())
	//	return histoIter
	//}
	return newHistoIterator(c.b.bytes())
}

// Iterator implements the Chunk interface.
func (c *HistoChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// HistoAppender is an Appender implementation for sparse histograms.
type HistoAppender struct {
	b *bstream

	// Metadata:
	schema             int32
	zeroThreshold      float64
	posSpans, negSpans []histogram.Span

	// For the fields that are tracked as dod's. Note that we expect to
	// handle negative deltas (e.g. resets) by creating new chunks, we still
	// want to support it in general hence signed integer types.
	t                           int64
	cnt, zcnt                   uint64
	tDelta, cntDelta, zcntDelta int64

	posbuckets, negbuckets           []int64
	posbucketsDelta, negbucketsDelta []int64

	// The sum is Gorilla xor encoded.
	sum      float64
	leading  uint8
	trailing uint8

	buf64 []byte // For working on varint64's.
}

func putVarint(b *bstream, buf []byte, x int64) {
	for _, byt := range buf[:binary.PutVarint(buf, x)] {
		b.writeByte(byt)
	}
}

func putUvarint(b *bstream, buf []byte, x uint64) {
	for _, byt := range buf[:binary.PutUvarint(buf, x)] {
		b.writeByte(byt)
	}
}

// Append implements Appender. This implementation does nothing for now.
// TODO(beorn7): Implement in a meaningful way, i.e. we need to support
// appending of stale markers, but this should never be used for "real"
// samples.
func (a *HistoAppender) Append(int64, float64) {}

// Appendable returns whether the chunk can be appended to, and if so
// whether any recoding needs to happen using the provided interjections
// (in case of any new buckets, positive or negative range, respectively)
// The chunk is not appendable if:
// * the schema has changed
// * the zerobucket threshold has changed
// * any buckets disappeared
// * there was a counter reset in the count of observations or in any bucket, including the zero bucket
// * the last sample in the chunk was stale while the current sample is not stale
// It returns an additional boolean set to true if it is not appendable because of a counter reset.
// If the given sample is stale, it will always return true.
// If counterReset is true, okToAppend MUST be false.
func (a *HistoAppender) Appendable(h histogram.SparseHistogram) (posInterjections []Interjection, negInterjections []Interjection, okToAppend bool, counterReset bool) {
	if value.IsStaleNaN(h.Sum) {
		// This is a stale sample whose buckets and spans don't matter.
		okToAppend = true
		return
	}
	if value.IsStaleNaN(a.sum) {
		// If the last sample was stale, then we can only accept stale samples in this chunk.
		return
	}

	if h.Count < a.cnt {
		// There has been a counter reset.
		counterReset = true
		return
	}

	if h.Schema != a.schema || h.ZeroThreshold != a.zeroThreshold {
		return
	}

	if h.ZeroCount < a.zcnt {
		// There has been a counter reset since ZeroThreshold didn't change.
		counterReset = true
		return
	}

	var ok bool
	posInterjections, ok = compareSpans(a.posSpans, h.PositiveSpans)
	if !ok {
		counterReset = true
		return
	}
	negInterjections, ok = compareSpans(a.negSpans, h.NegativeSpans)
	if !ok {
		counterReset = true
		return
	}

	if counterResetInAnyBucket(a.posbuckets, h.PositiveBuckets, a.posSpans, h.PositiveSpans) ||
		counterResetInAnyBucket(a.negbuckets, h.NegativeBuckets, a.negSpans, h.NegativeSpans) {
		counterReset, posInterjections, negInterjections = true, nil, nil
		return
	}

	okToAppend = true
	return
}

// counterResetInAnyBucket returns true if there was a counter reset for any bucket.
// This should be called only when buckets are same or new buckets were added,
// and does not handle the case of buckets missing.
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

// AppendHistogram appends a SparseHistogram to the chunk. We assume the
// histogram is properly structured. E.g. that the number of pos/neg buckets
// used corresponds to the number conveyed by the pos/neg span structures.
// callers must call Appendable() first and act accordingly!
func (a *HistoAppender) AppendHistogram(t int64, h histogram.SparseHistogram) {
	var tDelta, cntDelta, zcntDelta int64
	num := binary.BigEndian.Uint16(a.b.bytes())

	if value.IsStaleNaN(h.Sum) {
		// Emptying out other fields to write no buckets, and an empty meta in case of
		// first histogram in the chunk.
		h = histogram.SparseHistogram{Sum: h.Sum}
	}

	switch num {
	case 0:
		// the first append gets the privilege to dictate the metadata
		// but it's also responsible for encoding it into the chunk!

		writeHistoChunkMeta(a.b, h.Schema, h.ZeroThreshold, h.PositiveSpans, h.NegativeSpans)
		a.schema = h.Schema
		a.zeroThreshold = h.ZeroThreshold
		a.posSpans, a.negSpans = h.PositiveSpans, h.NegativeSpans
		numPosBuckets, numNegBuckets := countSpans(h.PositiveSpans), countSpans(h.NegativeSpans)
		a.posbuckets = make([]int64, numPosBuckets)
		a.negbuckets = make([]int64, numNegBuckets)
		a.posbucketsDelta = make([]int64, numPosBuckets)
		a.negbucketsDelta = make([]int64, numNegBuckets)

		// now store actual data
		putVarint(a.b, a.buf64, t)
		putUvarint(a.b, a.buf64, h.Count)
		putUvarint(a.b, a.buf64, h.ZeroCount)
		a.b.writeBits(math.Float64bits(h.Sum), 64)
		for _, buck := range h.PositiveBuckets {
			putVarint(a.b, a.buf64, buck)
		}
		for _, buck := range h.NegativeBuckets {
			putVarint(a.b, a.buf64, buck)
		}
	case 1:
		tDelta = t - a.t
		cntDelta = int64(h.Count) - int64(a.cnt)
		zcntDelta = int64(h.ZeroCount) - int64(a.zcnt)

		if value.IsStaleNaN(h.Sum) {
			cntDelta, zcntDelta = 0, 0
		}

		putVarint(a.b, a.buf64, tDelta)
		putVarint(a.b, a.buf64, cntDelta)
		putVarint(a.b, a.buf64, zcntDelta)

		a.writeSumDelta(h.Sum)

		for i, buck := range h.PositiveBuckets {
			delta := buck - a.posbuckets[i]
			putVarint(a.b, a.buf64, delta)
			a.posbucketsDelta[i] = delta
		}
		for i, buck := range h.NegativeBuckets {
			delta := buck - a.negbuckets[i]
			putVarint(a.b, a.buf64, delta)
			a.negbucketsDelta[i] = delta
		}

	default:
		tDelta = t - a.t
		cntDelta = int64(h.Count) - int64(a.cnt)
		zcntDelta = int64(h.ZeroCount) - int64(a.zcnt)

		tDod := tDelta - a.tDelta
		cntDod := cntDelta - a.cntDelta
		zcntDod := zcntDelta - a.zcntDelta

		if value.IsStaleNaN(h.Sum) {
			cntDod, zcntDod = 0, 0
		}

		putInt64VBBucket(a.b, tDod)
		putInt64VBBucket(a.b, cntDod)
		putInt64VBBucket(a.b, zcntDod)

		a.writeSumDelta(h.Sum)

		for i, buck := range h.PositiveBuckets {
			delta := buck - a.posbuckets[i]
			dod := delta - a.posbucketsDelta[i]
			putInt64VBBucket(a.b, dod)
			a.posbucketsDelta[i] = delta
		}
		for i, buck := range h.NegativeBuckets {
			delta := buck - a.negbuckets[i]
			dod := delta - a.negbucketsDelta[i]
			putInt64VBBucket(a.b, dod)
			a.negbucketsDelta[i] = delta
		}
	}

	binary.BigEndian.PutUint16(a.b.bytes(), num+1)

	a.t = t
	a.cnt = h.Count
	a.zcnt = h.ZeroCount
	a.tDelta = tDelta
	a.cntDelta = cntDelta
	a.zcntDelta = zcntDelta

	a.posbuckets, a.negbuckets = h.PositiveBuckets, h.NegativeBuckets
	// note that the bucket deltas were already updated above
	a.sum = h.Sum
}

// Recode converts the current chunk to accommodate an expansion of the set of
// (positive and/or negative) buckets used, according to the provided
// interjections, resulting in the honoring of the provided new posSpans and
// negSpans.
func (a *HistoAppender) Recode(posInterjections, negInterjections []Interjection, posSpans, negSpans []histogram.Span) (Chunk, Appender) {
	// TODO(beorn7): This currently just decodes everything and then encodes
	// it again with the new span layout. This can probably be done in-place
	// by editing the chunk. But let's first see how expensive it is in the
	// big picture.
	byts := a.b.bytes()
	it := newHistoIterator(byts)
	hc := NewHistoChunk()
	app, err := hc.Appender()
	if err != nil {
		panic(err)
	}
	numPosBuckets, numNegBuckets := countSpans(posSpans), countSpans(negSpans)

	for it.Next() {
		tOld, hOld := it.AtHistogram()

		// We have to newly allocate slices for the modified buckets
		// here because they are kept by the appender until the next
		// append.
		// TODO(beorn7): We might be able to optimize this.
		posBuckets := make([]int64, numPosBuckets)
		negBuckets := make([]int64, numNegBuckets)

		// Save the modified histogram to the new chunk.
		hOld.PositiveSpans, hOld.NegativeSpans = posSpans, negSpans
		if len(posInterjections) > 0 {
			hOld.PositiveBuckets = interject(hOld.PositiveBuckets, posBuckets, posInterjections)
		}
		if len(negInterjections) > 0 {
			hOld.NegativeBuckets = interject(hOld.NegativeBuckets, negBuckets, negInterjections)
		}
		app.AppendHistogram(tOld, hOld)
	}

	// Set the flags.
	hc.SetCounterResetHeader(CounterResetHeader(byts[2] & 0b11000000))
	return hc, app
}

func (a *HistoAppender) writeSumDelta(v float64) {
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

		// Note that if leading == trailing == 0, then sigbits == 64.  But that value doesn't actually fit into the 6 bits we have.
		// Luckily, we never need to encode 0 significant bits, since that would put us in the other case (vdelta == 0).
		// So instead we write out a 0 and adjust it back to 64 on unpacking.
		sigbits := 64 - leading - trailing
		a.b.writeBits(uint64(sigbits), 6)
		a.b.writeBits(vDelta>>trailing, int(sigbits))
	}
}

type histoIterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	// Metadata:
	schema             int32
	zeroThreshold      float64
	posSpans, negSpans []histogram.Span

	// For the fields that are tracked as dod's.
	t                           int64
	cnt, zcnt                   uint64
	tDelta, cntDelta, zcntDelta int64

	posbuckets, negbuckets           []int64
	posbucketsDelta, negbucketsDelta []int64

	// The sum is Gorilla xor encoded.
	sum      float64
	leading  uint8
	trailing uint8

	err error
}

func (it *histoIterator) Seek(t int64) bool {
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

func (it *histoIterator) At() (int64, float64) {
	panic("cannot call histoIterator.At().")
}

func (it *histoIterator) ChunkEncoding() Encoding {
	return EncSHS
}

func (it *histoIterator) AtHistogram() (int64, histogram.SparseHistogram) {
	if value.IsStaleNaN(it.sum) {
		return it.t, histogram.SparseHistogram{Sum: it.sum}
	}
	return it.t, histogram.SparseHistogram{
		Count:           it.cnt,
		ZeroCount:       it.zcnt,
		Sum:             it.sum,
		ZeroThreshold:   it.zeroThreshold,
		Schema:          it.schema,
		PositiveSpans:   it.posSpans,
		NegativeSpans:   it.negSpans,
		PositiveBuckets: it.posbuckets,
		NegativeBuckets: it.negbuckets,
	}
}

func (it *histoIterator) Err() error {
	return it.err
}

func (it *histoIterator) Reset(b []byte) {
	// The first 2 bytes contain chunk headers.
	// We skip that for actual samples.
	it.br = newBReader(b[2:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.numRead = 0

	it.t, it.cnt, it.zcnt = 0, 0, 0
	it.tDelta, it.cntDelta, it.zcntDelta = 0, 0, 0

	for i := range it.posbuckets {
		it.posbuckets[i] = 0
		it.posbucketsDelta[i] = 0
	}
	for i := range it.negbuckets {
		it.negbuckets[i] = 0
		it.negbucketsDelta[i] = 0
	}

	it.sum = 0
	it.leading = 0
	it.trailing = 0
	it.err = nil
}

func (it *histoIterator) Next() bool {
	if it.err != nil || it.numRead == it.numTotal {
		return false
	}

	if it.numRead == 0 {

		// first read is responsible for reading chunk metadata and initializing fields that depend on it
		// We give counter reset info at chunk level, hence we discard it here.
		schema, zeroThreshold, posSpans, negSpans, err := readHistoChunkMeta(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.schema = schema
		it.zeroThreshold = zeroThreshold
		it.posSpans, it.negSpans = posSpans, negSpans
		numPosBuckets, numNegBuckets := countSpans(posSpans), countSpans(negSpans)
		if numPosBuckets > 0 {
			it.posbuckets = make([]int64, numPosBuckets)
			it.posbucketsDelta = make([]int64, numPosBuckets)
		}
		if numNegBuckets > 0 {
			it.negbuckets = make([]int64, numNegBuckets)
			it.negbucketsDelta = make([]int64, numNegBuckets)
		}

		// now read actual data

		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.t = t

		cnt, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.cnt = cnt

		zcnt, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.zcnt = zcnt

		sum, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return false
		}
		it.sum = math.Float64frombits(sum)

		for i := range it.posbuckets {
			v, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.posbuckets[i] = v
		}
		for i := range it.negbuckets {
			v, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.negbuckets[i] = v
		}

		it.numRead++
		return true
	}

	if it.numRead == 1 {
		tDelta, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		cntDelta, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.cntDelta = cntDelta
		it.cnt = uint64(int64(it.cnt) + it.cntDelta)

		zcntDelta, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.zcntDelta = zcntDelta
		it.zcnt = uint64(int64(it.zcnt) + it.zcntDelta)

		ok := it.readSum()
		if !ok {
			return false
		}

		if value.IsStaleNaN(it.sum) {
			it.numRead++
			return true
		}

		for i := range it.posbuckets {
			delta, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.posbucketsDelta[i] = delta
			it.posbuckets[i] = it.posbuckets[i] + delta
		}

		for i := range it.negbuckets {
			delta, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return false
			}
			it.negbucketsDelta[i] = delta
			it.negbuckets[i] = it.negbuckets[i] + delta
		}

		it.numRead++
		return true
	}

	tDod, err := readInt64VBBucket(&it.br)
	if err != nil {
		it.err = err
		return false
	}
	it.tDelta = it.tDelta + tDod
	it.t += it.tDelta

	cntDod, err := readInt64VBBucket(&it.br)
	if err != nil {
		it.err = err
		return false
	}
	it.cntDelta = it.cntDelta + cntDod
	it.cnt = uint64(int64(it.cnt) + it.cntDelta)

	zcntDod, err := readInt64VBBucket(&it.br)
	if err != nil {
		it.err = err
		return false
	}
	it.zcntDelta = it.zcntDelta + zcntDod
	it.zcnt = uint64(int64(it.zcnt) + it.zcntDelta)

	ok := it.readSum()
	if !ok {
		return false
	}

	if value.IsStaleNaN(it.sum) {
		it.numRead++
		return true
	}

	for i := range it.posbuckets {
		dod, err := readInt64VBBucket(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.posbucketsDelta[i] = it.posbucketsDelta[i] + dod
		it.posbuckets[i] = it.posbuckets[i] + it.posbucketsDelta[i]
	}

	for i := range it.negbuckets {
		dod, err := readInt64VBBucket(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.negbucketsDelta[i] = it.negbucketsDelta[i] + dod
		it.negbuckets[i] = it.negbuckets[i] + it.negbucketsDelta[i]
	}

	it.numRead++
	return true
}

func (it *histoIterator) readSum() bool {
	bit, err := it.br.readBitFast()
	if err != nil {
		bit, err = it.br.readBit()
	}
	if err != nil {
		it.err = err
		return false
	}

	if bit == zero {
		// it.sum = it.sum
	} else {
		bit, err := it.br.readBitFast()
		if err != nil {
			bit, err = it.br.readBit()
		}
		if err != nil {
			it.err = err
			return false
		}
		if bit == zero {
			// reuse leading/trailing zero bits
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
			// 0 significant bits here means we overflowed and we actually need 64; see comment in encoder
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
	}

	return true
}
