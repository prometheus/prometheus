// Copyright 2025 The Prometheus Authors
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
)

// XORV2NaiveChunk holds XORSTNaive encoded sample data, so:
// 2B(numSamples), varint(st), varint(t), xor(v), varuint(stDelta), varuint(tDelta), xor(v), varint(stDod), varint(tDod), xor(v), ...
type XORV2NaiveChunk struct {
	b bstream
}

// NewXORV2NaiveChunk returns a new chunk with XORv2 encoding.
func NewXORV2NaiveChunk() *XORV2NaiveChunk {
	b := make([]byte, chunkHeaderSize, chunkAllocationSize)
	return &XORV2NaiveChunk{b: bstream{stream: b, count: 0}}
}

func (c *XORV2NaiveChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XORV2NaiveChunk) Encoding() Encoding {
	return EncXORV2Naive
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XORV2NaiveChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XORV2NaiveChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *XORV2NaiveChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *XORV2NaiveChunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *XORV2NaiveChunk) AppenderV2() (AppenderV2, error) {
	if len(c.b.stream) == chunkHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorV2NaiveAppender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
	}
	it := c.iterator(nil)

	// To get an appender we must know the state it would have if we had
	// appended all existing data from scratch.
	// We iterate through the end and populate via the iterator's state.
	for it.Next() != ValNone {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	a := &xorV2NaiveAppender{
		b:        &c.b,
		st:       it.st,
		t:        it.t,
		v:        it.val,
		stDelta:  it.stDelta,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,
	}
	return a, nil
}

func (c *XORV2NaiveChunk) iterator(it Iterator) *xorV2NaiveIterator {
	if xorIter, ok := it.(*xorV2NaiveIterator); ok {
		xorIter.Reset(c.b.bytes())
		return xorIter
	}
	return &xorV2NaiveIterator{
		// The first 2 bytes contain chunk headers.
		// We skip that for actual samples.
		br:       newBReader(c.b.bytes()[chunkHeaderSize:]),
		numTotal: binary.BigEndian.Uint16(c.b.bytes()),
		t:        math.MinInt64,
	}
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *XORV2NaiveChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorV2NaiveAppender struct {
	b *bstream

	st, t           int64
	v               float64
	stDelta, tDelta uint64

	leading  uint8
	trailing uint8
}

func (a *xorV2NaiveAppender) Append(st, t int64, v float64) {
	var stDelta, tDelta uint64
	num := binary.BigEndian.Uint16(a.b.bytes())
	switch num {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, st)] {
			a.b.writeByte(b)
		}
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		stDelta = uint64(st - a.st)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, stDelta)] {
			a.b.writeByte(b)
		}

		tDelta = uint64(t - a.t)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}

		a.writeVDelta(v)
	default:
		stDelta = uint64(st - a.st)
		sdod := int64(stDelta - a.stDelta)
		a.writeDod(sdod)

		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		a.writeDod(dod)

		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
	a.stDelta = stDelta
	a.tDelta = tDelta
}

func (a *xorV2NaiveAppender) writeDod(dod int64) {
	// Gorilla has a max resolution of seconds, Prometheus milliseconds.
	// Thus we use higher value range steps with larger bit size.
	//
	// TODO(beorn7): This seems to needlessly jump to large bit
	// sizes even for very small deviations from zero. Timestamp
	// compression can probably benefit from some smaller bit
	// buckets. See also what was done for histogram encoding in
	// varbit.go.
	switch {
	case dod == 0:
		a.b.writeBit(zero)
	case bitRange(dod, 14):
		a.b.writeByte(0b10<<6 | (uint8(dod>>8) & (1<<6 - 1))) // 0b10 size code combined with 6 bits of dod.
		a.b.writeByte(uint8(dod))                             // Bottom 8 bits of dod.
	case bitRange(dod, 17):
		a.b.writeBits(0b110, 3)
		a.b.writeBits(uint64(dod), 17)
	case bitRange(dod, 20):
		a.b.writeBits(0b1110, 4)
		a.b.writeBits(uint64(dod), 20)
	default:
		a.b.writeBits(0b1111, 4)
		a.b.writeBits(uint64(dod), 64)
	}
}

func (a *xorV2NaiveAppender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorV2NaiveAppender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorV2NaiveAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorV2NaiveIterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	st, t int64
	val   float64

	leading  uint8
	trailing uint8

	stDelta, tDelta uint64
	err             error
}

func (it *xorV2NaiveIterator) Seek(t int64) ValueType {
	if it.err != nil {
		return ValNone
	}

	for t > it.t || it.numRead == 0 {
		if it.Next() == ValNone {
			return ValNone
		}
	}
	return ValFloat
}

func (it *xorV2NaiveIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorV2NaiveIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorV2NaiveIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorV2NaiveIterator) AtT() int64 {
	return it.t
}

func (it *xorV2NaiveIterator) AtST() int64 {
	return it.st
}

func (it *xorV2NaiveIterator) Err() error {
	return it.err
}

func (it *xorV2NaiveIterator) Reset(b []byte) {
	// The first 2 bytes contain chunk headers.
	// We skip that for actual samples.
	it.br = newBReader(b[chunkHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)

	it.numRead = 0
	it.st = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.stDelta = 0
	it.tDelta = 0
	it.err = nil
}

func (it *xorV2NaiveIterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	if it.numRead == 0 {
		st, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		v, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.st = st
		it.t = t
		it.val = math.Float64frombits(v)

		it.numRead++
		return ValFloat
	}
	if it.numRead == 1 {
		stDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.stDelta = stDelta
		it.st += int64(it.stDelta)

		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		return it.readValue()
	}

	sdod, err := it.readDod()
	if err != nil {
		it.err = err
		return ValNone
	}
	dod, err := it.readDod()
	if err != nil {
		it.err = err
		return ValNone
	}

	it.stDelta = uint64(int64(it.stDelta) + sdod)
	it.st += int64(it.stDelta)

	it.tDelta = uint64(int64(it.tDelta) + dod)
	it.t += int64(it.tDelta)
	return it.readValue()
}

func (it *xorV2NaiveIterator) readDod() (int64, error) {
	var d byte
	// read delta-of-delta
	for range 4 {
		d <<= 1
		bit, err := it.br.readBitFast()
		if err != nil {
			bit, err = it.br.readBit()
		}
		if err != nil {
			return 0, err
		}
		if bit == zero {
			break
		}
		d |= 1
	}
	var sz uint8
	var dod int64
	switch d {
	case 0b0:
		// dod == 0
	case 0b10:
		sz = 14
	case 0b110:
		sz = 17
	case 0b1110:
		sz = 20
	case 0b1111:
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := it.br.readBits(64)
		if err != nil {
			return 0, err
		}

		dod = int64(bits)
	}

	if sz != 0 {
		bits, err := it.br.readBitsFast(sz)
		if err != nil {
			bits, err = it.br.readBits(sz)
		}
		if err != nil {
			return 0, err
		}

		// Account for negative numbers, which come back as high unsigned numbers.
		// See docs/bstream.md.
		if bits > (1 << (sz - 1)) {
			bits -= 1 << sz
		}
		dod = int64(bits)
	}
	return dod, nil
}

func (it *xorV2NaiveIterator) readValue() ValueType {
	err := xorRead(&it.br, &it.val, &it.leading, &it.trailing)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.numRead++
	return ValFloat
}
