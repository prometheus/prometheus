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
	"math"

	"github.com/prometheus/prometheus/model/histogram"
)

const (
	chunkSTHeaderSize  = 1
	maxFirstSTChangeOn = 0xFF
)

func writeHeaderFirstSTChangeOn(b []byte, firstSTChangeOn uint16) {
	// First bit indicates the initial ST value.
	// Here we save the sample number from where the first change occurs in the
	// rest of the byte (7 bits)

	if firstSTChangeOn > maxFirstSTChangeOn {
		// This should never happen, would cause corruption (ST already skipped but shouldn't).
		return
	}
	b[0] = uint8(firstSTChangeOn)
}

func readSTHeader(b []byte) uint8 {
	return b[0]
}

// XorSTChunk holds XOR enncoded samples with optional start time (ST)
// per chunk or per sample. See tsdb/docs/format/chunks.md for details.
type XorSTChunk struct {
	b bstream
}

// NewXORSTChunk returns a new chunk with XORv2 encoding.
func NewXORSTChunk() *XorSTChunk {
	b := make([]byte, chunkHeaderSize+chunkSTHeaderSize, chunkAllocationSize)
	return &XorSTChunk{b: bstream{stream: b, count: 0}}
}

func (c *XorSTChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XorSTChunk) Encoding() Encoding {
	return EncXORST
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XorSTChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XorSTChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *XorSTChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
// It is not valid to call Appender() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *XorSTChunk) Appender() (Appender, error) {
	if len(c.b.stream) == chunkHeaderSize+chunkSTHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorSTAppender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
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

	a := &xorSTAppender{
		b:        &c.b,
		st:       it.st,
		t:        it.t,
		v:        it.val,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,

		numTotal:        it.numTotal,
		firstSTChangeOn: it.firstSTChangeOn,
	}
	return a, nil
}

func (c *XorSTChunk) iterator(it Iterator) *xorSTIterator {
	xorIter, ok := it.(*xorSTIterator)
	if !ok {
		xorIter = &xorSTIterator{}
	}

	xorIter.Reset(c.b.bytes())
	return xorIter
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *XorSTChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorSTAppender struct {
	b               *bstream
	numTotal        uint16
	firstSTChangeOn uint16
	leading         uint8
	trailing        uint8
	st, t           int64
	v               float64
	tDelta          uint64
}

func (a *xorSTAppender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorSTAppender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorSTAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorSTIterator struct {
	br       bstreamReader
	numTotal uint16

	firstSTChangeOn uint16
	leading         uint8
	trailing        uint8

	numRead uint16

	st, t int64
	val   float64

	tDelta uint64
	err    error
}

func (it *xorSTIterator) Seek(t int64) ValueType {
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

func (it *xorSTIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorSTIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorSTIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorSTIterator) AtT() int64 {
	return it.t
}

func (it *xorSTIterator) AtST() int64 {
	return it.st
}

func (it *xorSTIterator) Err() error {
	return it.err
}

func (it *xorSTIterator) Reset(b []byte) {
	// We skip initial headers for actual samples.
	it.br = newBReader(b[chunkHeaderSize+chunkSTHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.firstSTChangeOn = uint16(readSTHeader(b[chunkHeaderSize:]))
	it.numRead = 0
	it.st = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.tDelta = 0
	it.err = nil
}

func (a *xorSTAppender) Append(st, t int64, v float64) (Chunk, Appender) {
	var tDelta uint64

	switch a.numTotal {
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
		buf := make([]byte, binary.MaxVarintLen64)
		if st != a.st {
			a.firstSTChangeOn = 1
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], 1)
			stDiff := a.t - st // This is 0 or 1 for inclusive or exclusive delta ST.
			for _, b := range buf[:binary.PutVarint(buf, stDiff)] {
				a.b.writeByte(b)
			}
		}

		tDelta = uint64(t - a.t)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}
		a.writeVDelta(v)
	default:
		if a.firstSTChangeOn == 0 && (st != a.st || a.numTotal == maxFirstSTChangeOn) {
			a.firstSTChangeOn = a.numTotal
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.numTotal)
		}
		if a.firstSTChangeOn != 0 {
			stDiff := a.t - st // This is 0 or 1 for inclusive or exclusive delta ST.
			// Gorilla has a max resolution of seconds, Prometheus milliseconds.
			// Thus we use higher value range steps with larger bit size.
			//
			// TODO(beorn7): This seems to needlessly jump to large bit
			// sizes even for very small deviations from zero. Timestamp
			// compression can probably benefit from some smaller bit
			// buckets. See also what was done for histogram encoding in
			// varbit.go.
			switch {
			case stDiff == 0:
				// Cover normal delta case where st_i == t_(i-1).
				a.b.writeBit(zero)
			case bitRange(stDiff, 6):
				// Cover delta case where st_i == t_(i-1)-31 .. t_(i-1)+32.
				// Including the +1 case.
				a.b.writeByte(0b10<<6 | uint8(stDiff)&0x3F) // 0b10 size code combined with 6 bits of stDelta.
			case bitRange(stDiff, 17):
				// Cover delta case where st_i == t_(i-1)-65535 .. t_(i-1)+65536.
				// ~1 min range at ms resolution.
				a.b.writeBits(0b110, 3)
				a.b.writeBits(uint64(stDiff), 17)
			case bitRange(stDiff, 20):
				a.b.writeBits(0b1110, 4)
				a.b.writeBits(uint64(stDiff), 20)
			default:
				a.b.writeBits(0b1111, 4)
				a.b.writeBits(uint64(stDiff), 64)
			}
		}

		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

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

		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta

	a.numTotal++
	binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)
	return nil, a
}

func (it *xorSTIterator) retErr(err error) ValueType {
	it.err = err
	return ValNone
}

func (it *xorSTIterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	if it.numRead == 0 {
		// Always read ST for the first sample.
		st, err := binary.ReadVarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}
		it.st = st
		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}
		v, err := it.br.readBits(64)
		if err != nil {
			return it.retErr(err)
		}
		it.t = t
		it.val = math.Float64frombits(v)

		it.numRead++
		return ValFloat
	}

	if it.numRead == 1 {
		// Optional ST delta read.
		if it.firstSTChangeOn == 1 {
			stDiff, err := binary.ReadVarint(&it.br)
			if err != nil {
				return it.retErr(err)
			}
			it.st = it.t - stDiff
		}
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		return it.readValue()
	}

	if it.firstSTChangeOn > 0 && it.numRead >= it.firstSTChangeOn {
		var d byte
		// read delta to it.t
		for range 4 {
			d <<= 1
			bit, err := it.br.readBitFast()
			if err != nil {
				bit, err = it.br.readBit()
				if err != nil {
					return it.retErr(err)
				}
			}
			if bit == zero {
				break
			}
			d |= 1
		}
		var sz uint8
		var stDiff int64
		switch d {
		case 0b0:
			// stDelta == 0
		case 0b10:
			sz = 6
		case 0b110:
			sz = 17
		case 0b1110:
			sz = 20
		case 0b1111:
			// Do not use fast because it's very unlikely it will succeed.
			bits, err := it.br.readBits(64)
			if err != nil {
				return it.retErr(err)
			}

			stDiff = int64(bits)
		}

		if sz != 0 {
			bits, err := it.br.readBitsFast(sz)
			if err != nil {
				bits, err = it.br.readBits(sz)
				if err != nil {
					return it.retErr(err)
				}
			}

			// Account for negative numbers, which come back as high unsigned numbers.
			// See docs/bstream.md.
			if bits > (1 << (sz - 1)) {
				bits -= 1 << sz
			}
			stDiff = int64(bits)
		}

		it.st = it.t - stDiff
	}

	var d byte
	// read delta-of-delta
	for range 4 {
		d <<= 1
		bit, err := it.br.readBitFast()
		if err != nil {
			bit, err = it.br.readBit()
		}
		if err != nil {
			return it.retErr(err)
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
			return it.retErr(err)
		}

		dod = int64(bits)
	}

	if sz != 0 {
		bits, err := it.br.readBitsFast(sz)
		if err != nil {
			bits, err = it.br.readBits(sz)
		}
		if err != nil {
			return it.retErr(err)
		}

		// Account for negative numbers, which come back as high unsigned numbers.
		// See docs/bstream.md.
		if bits > (1 << (sz - 1)) {
			bits -= 1 << sz
		}
		dod = int64(bits)
	}

	it.tDelta = uint64(int64(it.tDelta) + dod)
	it.t += int64(it.tDelta)

	return it.readValue()
}

func (it *xorSTIterator) readValue() ValueType {
	err := xorRead(&it.br, &it.val, &it.leading, &it.trailing)
	if err != nil {
		return it.retErr(err)
	}
	it.numRead++
	return ValFloat
}
