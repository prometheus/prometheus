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
// TODO: encode schema and spans metadata in the chunk
// TODO: decode-recode chunk when new spans appear

type HistoChunk struct {
	b bstream

	// "metadata" describing all the data within this chunk
	schema             int32
	posSpans, negSpans []histogram.Span
}

// NewHistoChunk returns a new chunk with Histo encoding of the given size.
func NewHistoChunk() *HistoChunk {
	b := make([]byte, 2, 128)
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

	a := &histoAppender{
		c: c,
		b: &c.b,

		schema:   c.schema,
		posSpans: c.posSpans,
		negSpans: c.negSpans,

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

// TODO fix this
func (c *HistoChunk) iterator(it Iterator) *histoIterator {
	// Should iterators guarantee to act on a copy of the data so it doesn't lock append?
	// When using striped locks to guard access to chunks, probably yes.
	// Could only copy data if the chunk is not completed yet.
	//if histoIter, ok := it.(*histoIterator); ok {
	//	histoIter.Reset(c.b.bytes())
	//	return histoIter
	//}

	var numPosBuckets, numNegBuckets int
	for _, s := range c.posSpans {
		numPosBuckets += int(s.Length)
	}
	for _, s := range c.negSpans {
		numNegBuckets += int(s.Length)
	}

	return &histoIterator{
		// The first 2 bytes contain chunk headers.
		// We skip that for actual samples.
		br:       newBReader(c.b.bytes()[2:]),
		numTotal: binary.BigEndian.Uint16(c.b.bytes()),
		t:        math.MinInt64,

		schema:   c.schema,
		posSpans: c.posSpans,
		negSpans: c.negSpans,

		posbuckets:      make([]int64, numPosBuckets),
		negbuckets:      make([]int64, numNegBuckets),
		posbucketsDelta: make([]int64, numPosBuckets),
		negbucketsDelta: make([]int64, numNegBuckets),
	}
}

// Iterator implements the Chunk interface.
func (c *HistoChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type histoAppender struct {
	c *HistoChunk // this is such that during the first append we can set the metadata on the chunk. not sure if that's how it should work

	b *bstream

	// Meta
	schema             int32
	posSpans, negSpans []histogram.Span

	// for the fields that are tracked as dod's
	// note that we expect to handle negative deltas (e.g. resets) by creating new chunks, we still want to support it in general hence signed integer types
	t                           int64
	cnt, zcnt                   uint64
	tDelta, cntDelta, zcntDelta int64

	posbuckets, negbuckets           []int64
	posbucketsDelta, negbucketsDelta []int64

	// for the fields that are gorilla xor coded
	sum      float64
	leading  uint8
	trailing uint8

	buf64 []byte // for working on varint64's
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

// we use this for millisec timestamps and all counts
// for now this is copied from xor.go - we will probably want to be more conservative (use fewer bits for small values) - can be tweaked later
func putDod(b *bstream, dod int64) {
	switch {
	case dod == 0:
		b.writeBit(zero)
	case bitRange(dod, 14):
		b.writeBits(0x02, 2) // '10'
		b.writeBits(uint64(dod), 14)
	case bitRange(dod, 17):
		b.writeBits(0x06, 3) // '110'
		b.writeBits(uint64(dod), 17)
	case bitRange(dod, 20):
		b.writeBits(0x0e, 4) // '1110'
		b.writeBits(uint64(dod), 20)
	default:
		b.writeBits(0x0f, 4) // '1111'
		b.writeBits(uint64(dod), 64)
	}
}

func (a *histoAppender) Append(int64, float64) {
	panic("cannot call histoAppender.Append().")
}

// AppendHistogram appends a SparseHistogram to the chunk
// we assume the histogram is properly structured. E.g. that the number pos/neg buckets used corresponds to the number conveyed by the pos/neg span structures
func (a *histoAppender) AppendHistogram(t int64, h histogram.SparseHistogram) {
	var tDelta, cntDelta, zcntDelta int64
	num := binary.BigEndian.Uint16(a.b.bytes())

	if num == 0 {
		// the first append gets the privilege to dictate the metadata, on both the appender and the chunk
		// TODO we should probably not reach back into the chunk here. should metadata be set when we create the chunk?
		a.c.schema = h.Schema
		a.c.posSpans, a.c.negSpans = h.PositiveSpans, h.NegativeSpans

		a.schema = h.Schema
		a.posSpans, a.negSpans = h.PositiveSpans, h.NegativeSpans

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
	} else if num == 1 {
		tDelta = t - a.t
		cntDelta = int64(h.Count) - int64(a.cnt)
		zcntDelta = int64(h.ZeroCount) - int64(a.zcnt)

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
	} else {
		tDelta = t - a.t
		cntDelta = int64(h.Count) - int64(a.cnt)
		zcntDelta = int64(h.ZeroCount) - int64(a.zcnt)

		tDod := tDelta - a.tDelta
		cntDod := cntDelta - a.cntDelta
		zcntDod := zcntDelta - a.zcntDelta

		putDod(a.b, tDod)
		putDod(a.b, cntDod)
		putDod(a.b, zcntDod)

		a.writeSumDelta(h.Sum)

		for i, buck := range h.PositiveBuckets {
			delta := buck - a.posbuckets[i]
			dod := delta - a.posbucketsDelta[i]
			putDod(a.b, dod)
			a.posbucketsDelta[i] = delta
		}
		for i, buck := range h.NegativeBuckets {
			delta := buck - a.negbuckets[i]
			dod := delta - a.negbucketsDelta[i]
			putDod(a.b, dod)
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

func (a *histoAppender) writeSumDelta(v float64) {
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

	// Meta
	schema             int32
	posSpans, negSpans []histogram.Span

	// for the fields that are tracked as dod's
	t                           int64
	cnt, zcnt                   uint64
	tDelta, cntDelta, zcntDelta int64

	posbuckets, negbuckets           []int64
	posbucketsDelta, negbucketsDelta []int64

	// for the fields that are gorilla xor coded
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

func (it *histoIterator) AtHistogram() (int64, histogram.SparseHistogram) {
	return it.t, histogram.SparseHistogram{
		Count:           it.cnt,
		ZeroCount:       it.zcnt,
		Sum:             it.sum,
		ZeroThreshold:   0, // TODO
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

		return true
	}

	tDod, ok := it.readDod()
	if !ok {
		return ok
	}
	it.tDelta = it.tDelta + tDod
	it.t += it.tDelta

	cntDod, ok := it.readDod()
	if !ok {
		return ok
	}
	it.cntDelta = it.cntDelta + cntDod
	it.cnt = uint64(int64(it.cnt) + it.cntDelta)

	zcntDod, ok := it.readDod()
	if !ok {
		return ok
	}
	it.zcntDelta = it.zcntDelta + zcntDod
	it.zcnt = uint64(int64(it.zcnt) + it.zcntDelta)

	ok = it.readSum()
	if !ok {
		return false
	}

	for i := range it.posbuckets {
		dod, ok := it.readDod()
		if !ok {
			return ok
		}
		it.posbucketsDelta[i] = it.posbucketsDelta[i] + dod
		it.posbuckets[i] = it.posbuckets[i] + it.posbucketsDelta[i]
	}

	for i := range it.negbuckets {
		dod, ok := it.readDod()
		if !ok {
			return ok
		}
		it.negbucketsDelta[i] = it.negbucketsDelta[i] + dod
		it.negbuckets[i] = it.negbuckets[i] + it.negbucketsDelta[i]
	}

	return true
}

func (it *histoIterator) readDod() (int64, bool) {
	var d byte
	// read delta-of-delta
	for i := 0; i < 4; i++ {
		d <<= 1
		bit, err := it.br.readBitFast()
		if err != nil {
			bit, err = it.br.readBit()
		}
		if err != nil {
			it.err = err
			return 0, false
		}
		if bit == zero {
			break
		}
		d |= 1
	}

	var sz uint8
	var dod int64
	switch d {
	case 0x00:
		// dod == 0
	case 0x02:
		sz = 14
	case 0x06:
		sz = 17
	case 0x0e:
		sz = 20
	case 0x0f:
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return 0, false
		}

		dod = int64(bits)
	}

	if sz != 0 {
		bits, err := it.br.readBitsFast(sz)
		if err != nil {
			bits, err = it.br.readBits(sz)
		}
		if err != nil {
			it.err = err
			return 0, false
		}
		if bits > (1 << (sz - 1)) {
			// or something
			bits = bits - (1 << sz)
		}
		dod = int64(bits)
	}

	return dod, true
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

	it.numRead++
	return true
}
