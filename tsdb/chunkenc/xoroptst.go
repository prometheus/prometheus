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
	chunkSTHeaderSize = 1
)

// stHeader is a 16 bit header for start time (ST) in chunks.
// If can store a maximum of 0x7FF (2047) samples before the first ST change.
type stHeader uint16

func (h stHeader) stEqualsT() bool {
	return (h & 0x8000) != 0
}

func (h *stHeader) setSTEqualsT() {
	*h |= 0x8000
}

func (h stHeader) firstSTKnown() bool {
	return (h & 0x1000) != 0
}

func (h *stHeader) setFirstSTKnown() {
	*h |= 0x1000
}

func (h stHeader) firstSTDiffKnown() bool {
	return (h & 0x800) != 0
}

func (h *stHeader) setFirstSTDiffKnown() {
	*h |= 0x800
}

func (h stHeader) firstSTChangeOn() uint16 {
	return uint16(h) & 0x7FF
}

func (h *stHeader) setFirstSTChangeOn(pos uint16) {
	*h |= stHeader(pos & 0x7FF)
}

// nsHeader is a 16 bit header for number of sample (NS) in chunks.
// Maximum number of samples is 0x7FF (2047).
type nsHeader uint16

func readNSHeader(b []byte) nsHeader {
	return nsHeader(b[1]) | nsHeader(b[0]&0x07)<<8
}

func readHeaders(b []byte) (stHeader, nsHeader) {
	v := b[0]
	numSamples := nsHeader(v&0x07)<<8 | nsHeader(b[1])
	stHeader := (stHeader(v>>3))<<8 | stHeader(b[2])
	return stHeader, numSamples
}

func writeHeaders(b []byte, stHeader stHeader, nsHeader nsHeader) {
	b[0] = (uint8(stHeader>>8) << 3) | (uint8(nsHeader>>8) & 0x07)
	b[1] = uint8(nsHeader)
	b[2] = uint8(stHeader)
}

// XorOptSTChunk holds XOR enncoded samples with optional start time (ST)
// per chunk or per sample. See tsdb/docs/format/chunks.md for details.
type XorOptSTChunk struct {
	b bstream
}

// NewXOROptSTChunk returns a new chunk with XORv2 encoding.
func NewXOROptSTChunk() *XorOptSTChunk {
	b := make([]byte, chunkHeaderSize+chunkSTHeaderSize, chunkAllocationSize)
	return &XorOptSTChunk{b: bstream{stream: b, count: 0}}
}

func (c *XorOptSTChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XorOptSTChunk) Encoding() Encoding {
	return EncXOROptST
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XorOptSTChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XorOptSTChunk) NumSamples() int {
	return int(readNSHeader(c.b.bytes()))
}

// Compact implements the Chunk interface.
func (c *XorOptSTChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
// It is not valid to call Appender() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *XorOptSTChunk) Appender() (Appender, error) {
	if len(c.b.stream) == chunkHeaderSize+chunkSTHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorOptSTAppender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
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

	// Set the bit position for continuing writes.
	// The iterator's reader tracks how many bits remain unread in the last byte.
	c.b.count = it.br.valid

	a := &xorOptSTAppender{
		b:        &c.b,
		st:       it.st,
		t:        it.t,
		v:        it.val,
		stDiff:   it.stDiff,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,

		numTotal: it.numTotal,
		stHeader: it.stHeader,
	}
	return a, nil
}

func (c *XorOptSTChunk) iterator(it Iterator) *xorOptSTtIterator {
	xorIter, ok := it.(*xorOptSTtIterator)
	if !ok {
		xorIter = &xorOptSTtIterator{}
	}

	xorIter.Reset(c.b.bytes())
	return xorIter
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *XorOptSTChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorOptSTAppender struct {
	b *bstream

	st, t int64
	v     float64

	stDiff   int64  // Difference between current ST and previous T. Undefined for first sample.
	tDelta   uint64 // Difference between current T and previous T. Undefined for first sample.
	numTotal nsHeader
	stHeader stHeader
	leading  uint8
	trailing uint8
}

func (a *xorOptSTAppender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorOptSTAppender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorOptSTAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorOptSTtIterator struct {
	br       bstreamReader
	numTotal nsHeader

	stHeader stHeader
	leading  uint8
	trailing uint8

	numRead uint16

	st, t int64
	val   float64

	stDiff int64
	tDelta uint64
	err    error
}

func (it *xorOptSTtIterator) Seek(t int64) ValueType {
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

func (it *xorOptSTtIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorOptSTtIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorOptSTtIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorOptSTtIterator) AtT() int64 {
	return it.t
}

func (it *xorOptSTtIterator) AtST() int64 {
	return it.st
}

func (it *xorOptSTtIterator) Err() error {
	return it.err
}

func (it *xorOptSTtIterator) Reset(b []byte) {
	// We skip initial headers for actual samples.
	it.br = newBReader(b[chunkHeaderSize+chunkSTHeaderSize:])
	it.stHeader, it.numTotal = readHeaders(b)
	it.numRead = 0
	it.st = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.stDiff = 0
	it.tDelta = 0
	it.err = nil
}

func (a *xorOptSTAppender) Append(st, t int64, v float64) {
	if st == 0 && a.stHeader == 0 {
		// Fast path for no ST usage at all.
		// Same as classic XOR chunk appender.

		var tDelta uint64

		switch a.numTotal {
		case 0:
			buf := make([]byte, binary.MaxVarintLen64)
			for _, b := range buf[:binary.PutVarint(buf, t)] {
				a.b.writeByte(b)
			}
			a.b.writeBits(math.Float64bits(v), 64)
		case 1:
			buf := make([]byte, binary.MaxVarintLen64)
			tDelta = uint64(t - a.t)
			for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
				a.b.writeByte(b)
			}
			a.writeVDelta(v)
		default:
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

		a.t = t
		a.v = v
		a.tDelta = tDelta
		a.numTotal++
		binary.BigEndian.PutUint16(a.b.bytes(), uint16(a.numTotal))
		return
	}

	var (
		stDiff int64  // Difference between current ST and previous T. Undefined for first sample.
		tDelta uint64 // Difference between current T and previous T. Undefined for first sample.
	)

	// Slow path for ST usage.
	switch a.numTotal {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)

		// Write T.
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}

		// Write V.
		a.b.writeBits(math.Float64bits(v), 64)

		// Write ST.
		for _, b := range buf[:binary.PutVarint(buf, t-st)] {
			a.b.writeByte(b)
		}
		a.stHeader.setFirstSTKnown()
		if st == t {
			a.stHeader.setSTEqualsT()
		}

	case 1:
		buf := make([]byte, binary.MaxVarintLen64)
		tDelta = uint64(t - a.t)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}
		a.writeVDelta(v)
		if st != 0 && st != a.st && a.stHeader.firstSTKnown() {
			a.stHeader.setFirstSTDiffKnown()
		}
		if (st == 0 && a.stHeader.firstSTKnown()) || (st != t && a.stHeader.stEqualsT()) || (st != 0 && !a.stHeader.firstSTKnown()) {
			a.stHeader.setFirstSTChangeOn(1)
		}
		if !a.stHeader.firstSTDiffKnown() && a.stHeader.firstSTChangeOn() == 0 {
			break
		}
		// Initialize double delta of st - prev_t.
		stDiff = a.t - st
		sdod := stDiff
		// Gorilla has a max resolution of seconds, Prometheus milliseconds.
		// Thus we use higher value range steps with larger bit size.
		//
		// TODO(beorn7): This seems to needlessly jump to large bit
		// sizes even for very small deviations from zero. Timestamp
		// compression can probably benefit from some smaller bit
		// buckets. See also what was done for histogram encoding in
		// varbit.go.
		switch {
		case sdod == 0:
			a.b.writeBit(zero)
		case bitRange(sdod, 14):
			a.b.writeByte(0b10<<6 | (uint8(sdod>>8) & (1<<6 - 1))) // 0b10 size code combined with 6 bits of dod.
			a.b.writeByte(uint8(sdod))                             // Bottom 8 bits of dod.
		case bitRange(sdod, 17):
			a.b.writeBits(0b110, 3)
			a.b.writeBits(uint64(sdod), 17)
		case bitRange(sdod, 20):
			a.b.writeBits(0b1110, 4)
			a.b.writeBits(uint64(sdod), 20)
		default:
			a.b.writeBits(0b1111, 4)
			a.b.writeBits(uint64(sdod), 64)
		}

	default:
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

		stDiff = a.t - st
		if a.stHeader.firstSTChangeOn() == 0 {
			if a.stHeader.firstSTKnown() {
				if stDiff == a.stDiff && a.stHeader.firstSTDiffKnown() || st == t && a.stHeader.stEqualsT() {
					// No stDiff change.
					break
				}
				if !a.stHeader.firstSTDiffKnown() {
					if st == a.st {
						// No st change.
						break
					}
					// This is the first ST diff change, reset the baseline.
					a.stDiff = 0
				}
				a.stHeader.setFirstSTChangeOn(uint16(a.numTotal))
			} else if st != 0 {
				// First ST change.
				a.stHeader.setFirstSTChangeOn(uint16(a.numTotal))
			}
		}

		// Gorilla has a max resolution of seconds, Prometheus milliseconds.
		// Thus we use higher value range steps with larger bit size.
		//
		// TODO(beorn7): This seems to needlessly jump to large bit
		// sizes even for very small deviations from zero. Timestamp
		// compression can probably benefit from some smaller bit
		// buckets. See also what was done for histogram encoding in
		// varbit.go.
		sdod := stDiff - a.stDiff
		switch {
		case sdod == 0:
			a.b.writeBit(zero)
		case bitRange(sdod, 14):
			a.b.writeByte(0b10<<6 | (uint8(sdod>>8) & (1<<6 - 1))) // 0b10 size code combined with 6 bits of dod.
			a.b.writeByte(uint8(sdod))                             // Bottom 8 bits of dod.
		case bitRange(sdod, 17):
			a.b.writeBits(0b110, 3)
			a.b.writeBits(uint64(sdod), 17)
		case bitRange(sdod, 20):
			a.b.writeBits(0b1110, 4)
			a.b.writeBits(uint64(sdod), 20)
		default:
			a.b.writeBits(0b1111, 4)
			a.b.writeBits(uint64(sdod), 64)
		}
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta
	a.stDiff = stDiff
	a.numTotal++
	writeHeaders(a.b.bytes(), a.stHeader, a.numTotal)
}

func (it *xorOptSTtIterator) retErr(err error) ValueType {
	it.err = err
	return ValNone
}

func (it *xorOptSTtIterator) Next() ValueType {
	if it.err != nil || it.numRead == uint16(it.numTotal) {
		return ValNone
	}

	if it.numRead == 0 {
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

		// Optional ST read.
		if it.stHeader.firstSTKnown() {
			st, err := binary.ReadVarint(&it.br)
			if err != nil {
				return it.retErr(err)
			}
			it.st = t - st
			if st == 0 {
				it.stHeader.setSTEqualsT()
			}
		}

		it.numRead++
		return ValFloat
	}

	if it.numRead == 1 {
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}
		it.tDelta = tDelta

		if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
			return it.retErr(err)
		}

		if it.stHeader.firstSTDiffKnown() || it.stHeader.firstSTChangeOn() == 1 {
			var d byte
			// read delta-of-delta
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
			var sdod int64
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

				sdod = int64(bits)
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
				sdod = int64(bits)
			}
			it.stDiff = sdod
			it.st = it.t - sdod
		}

		it.t += int64(it.tDelta)
		it.numRead++
		return ValFloat
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

	if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
		return it.retErr(err)
	}

	stChangeOn := it.stHeader.firstSTChangeOn()
	if stChangeOn == 0 || it.numRead < stChangeOn {
		// No ST change recorded.
		if it.stHeader.firstSTKnown() {
			if it.stHeader.stEqualsT() {
				// ST equals T.
				it.st = it.t + int64(it.tDelta)
			} else if it.stHeader.firstSTDiffKnown() {
				// First ST diff was known and hasn't changed.
				it.st = it.t - it.stDiff
			}
			// Otherwise first ST was known and hasn't changed.
			// Do nothing.
		}
	} else {
		// if it.numRead > stChangeOn {
		// Double delta of t - st continues.
		var d byte
		// read delta-of-delta
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
		var sdod int64
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

			sdod = int64(bits)
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
			sdod = int64(bits)
		}
		it.stDiff += sdod
		it.st = it.t - it.stDiff
	}
	it.t += int64(it.tDelta)

	it.numRead++
	return ValFloat
}
