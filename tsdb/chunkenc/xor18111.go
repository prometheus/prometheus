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

// This file implements the XOR18111 chunk encoding, which corresponds to the
// encoding proposed in https://github.com/prometheus/prometheus/pull/18111.

package chunkenc

import (
	"encoding/binary"
	"math"
	"math/bits"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

// XOR18111Chunk implements XOR encoding with adaptive control bits and staleness
// optimization, as proposed in https://github.com/prometheus/prometheus/pull/18111.
// It starts with 4-bit control codes (like original XOR) for perfectly regular
// data, and switches to 5-bit control codes when irregular patterns are detected.
// This eliminates overhead on perfectly regular data while maintaining benefits
// for irregular data.
//
// This is a standalone implementation (not embedding XORChunk) for better
// inlining performance.
type XOR18111Chunk struct {
	b bstream
}

// NewXOR18111Chunk returns a new chunk with XOR18111 encoding.
func NewXOR18111Chunk() *XOR18111Chunk {
	b := make([]byte, chunkHeaderSize, chunkAllocationSize)
	return &XOR18111Chunk{b: bstream{stream: b, count: 0}}
}

func (c *XOR18111Chunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XOR18111Chunk) Encoding() Encoding {
	return EncXOR18111
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XOR18111Chunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XOR18111Chunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *XOR18111Chunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *XOR18111Chunk) Appender() (Appender, error) {
	if len(c.b.stream) == chunkHeaderSize {
		return &xor18111Appender{
			b:       &c.b,
			t:       math.MinInt64,
			leading: 0xff,
			mode:    xor18111ModeCompact,
		}, nil
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

	a := &xor18111Appender{
		b:        &c.b,
		t:        it.t,
		v:        it.baselineV,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,
		mode:     it.mode,
	}
	return a, nil
}

func (c *XOR18111Chunk) iterator(it Iterator) *xor18111Iterator {
	if xor18111Iter, ok := it.(*xor18111Iterator); ok {
		xor18111Iter.Reset(c.b.bytes())
		return xor18111Iter
	}
	return &xor18111Iterator{
		br:        newBReader(c.b.bytes()[chunkHeaderSize:]),
		numTotal:  binary.BigEndian.Uint16(c.b.bytes()),
		t:         math.MinInt64,
		baselineV: 0,
		mode:      xor18111ModeCompact,
	}
}

// Iterator implements the Chunk interface.
func (c *XOR18111Chunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

const (
	// xor18111ModeCompact uses 4-bit control codes (0, 10, 110, 1110, 1111).
	xor18111ModeCompact = 0
	// xor18111ModeFull uses 5-bit control codes (0, 10, 110, 1110, 11110, 11111).
	xor18111ModeFull = 1
)

// xor18111Appender uses adaptive control bit encoding with staleness optimization.
type xor18111Appender struct {
	b *bstream

	t      int64
	v      float64
	tDelta uint64

	leading  uint8
	trailing uint8

	mode uint8
}

func (a *xor18111Appender) Append(_, t int64, v float64) {
	var tDelta uint64
	num := binary.BigEndian.Uint16(a.b.bytes())
	switch num {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		tDelta = uint64(t - a.t)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}

		a.writeVDelta(v)
	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		if a.mode == xor18111ModeCompact {
			switch {
			case dod == 0:
				a.b.writeBit(zero)
			case bitRange(dod, 14):
				a.b.writeByte(0b10<<6 | (uint8(dod>>8) & (1<<6 - 1)))
				a.b.writeByte(uint8(dod))
			case bitRange(dod, 17):
				a.b.writeBits(0b110, 3)
				a.b.writeBits(uint64(dod), 17)
			case bitRange(dod, 20):
				a.b.writeBits(0b1110, 4)
				a.b.writeBits(uint64(dod), 20)
			default:
				a.b.writeBits(0b1111, 4)
				a.mode = xor18111ModeFull
				a.writeTimestampDeltaFull(dod)
			}
		} else {
			a.writeTimestampDeltaFull(dod)
		}
		a.writeVDelta(v)
	}

	a.t = t
	// Only update baseline for non-stale values.
	if !value.IsStaleNaN(v) {
		a.v = v
	}
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
	a.tDelta = tDelta
}

func (a *xor18111Appender) writeTimestampDeltaFull(dod int64) {
	switch {
	case dod == 0:
		a.b.writeBit(zero)
	case bitRange(dod, 7):
		a.b.writeBits(0b10, 2)
		a.b.writeBits(uint64(dod), 7)
	case bitRange(dod, 14):
		a.b.writeBits(0b110, 3)
		a.b.writeBits(uint64(dod), 14)
	case bitRange(dod, 20):
		a.b.writeBits(0b1110, 4)
		a.b.writeBits(uint64(dod), 20)
	default:
		// Try multiplier encoding.
		encoded := false
		if a.tDelta > 0 && dod != 0 {
			multiplierF := float64(dod) / float64(a.tDelta)
			multiplier := int64(multiplierF)
			if multiplierF > 0 && multiplierF-float64(multiplier) >= 0.5 {
				multiplier++
			} else if multiplierF < 0 && float64(multiplier)-multiplierF >= 0.5 {
				multiplier--
			}

			if multiplier >= -15 && multiplier <= 15 && multiplier != 0 {
				reconstructed := multiplier * int64(a.tDelta)
				residual := dod - reconstructed

				// Only use multiplier encoding if residual fits in 8 bits signed.
				if residual >= -128 && residual <= 127 {
					// Encode: 11110 [sign] [magnitude] [residual] (18 bits total).
					a.b.writeBits(0b11110, 5)
					if multiplier > 0 {
						a.b.writeBit(zero)
						a.b.writeBits(uint64(multiplier-1), 4)
					} else {
						a.b.writeBit(one)
						a.b.writeBits(uint64(-multiplier-1), 4)
					}
					a.b.writeBits(uint64(int8(residual)), 8)
					encoded = true
				}
			}
		}

		if !encoded {
			a.b.writeBits(0b11111, 5)
			a.b.writeBits(uint64(dod), 64)
		}
	}
}

// writeVDelta encodes the value delta with optimized staleness handling.
func (a *xor18111Appender) writeVDelta(v float64) {
	if value.IsStaleNaN(v) {
		// Write the impossible pattern: 11 + leading=31 + sigbits=63.
		// Normal NaN encoding would use ~110 bits; this uses only 13 bits.
		a.b.writeBit(one)
		a.b.writeBit(one)
		a.b.writeBits(31, 5)
		a.b.writeBits(63, 6)
		return
	}

	// Normal XOR encoding against the baseline (last non-stale) value.
	delta := math.Float64bits(v) ^ math.Float64bits(a.v)

	if delta == 0 {
		a.b.writeBit(zero)
		return
	}
	a.b.writeBit(one)

	newLeading := uint8(bits.LeadingZeros64(delta))
	newTrailing := uint8(bits.TrailingZeros64(delta))

	// Clamp number of leading zeros to avoid overflow when encoding.
	if newLeading >= 32 {
		newLeading = 31
	}

	if a.leading != 0xff && newLeading >= a.leading && newTrailing >= a.trailing {
		// Stick with the current leading/trailing.
		a.b.writeBit(zero)
		a.b.writeBits(delta>>a.trailing, 64-int(a.leading)-int(a.trailing))
		return
	}

	// Update leading/trailing.
	a.leading, a.trailing = newLeading, newTrailing

	a.b.writeBit(one)
	a.b.writeBits(uint64(newLeading), 5)

	// Note that if newLeading == newTrailing == 0, then sigbits == 64. But
	// that value doesn't actually fit into the 6 bits we have. Luckily, we
	// never need to encode 0 significant bits, since that would put us in
	// the other case (delta == 0). So instead we write out a 0 and adjust
	// it back to 64 on unpacking.
	sigbits := 64 - newLeading - newTrailing
	a.b.writeBits(uint64(sigbits), 6)
	a.b.writeBits(delta>>newTrailing, int(sigbits))
}

func (*xor18111Appender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xor18111Appender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

// xor18111Iterator decodes XOR18111 chunks with adaptive control bits and staleness.
type xor18111Iterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	t   int64
	val float64

	leading  uint8
	trailing uint8

	tDelta uint64
	err    error

	baselineV float64
	mode      uint8
}

func (it *xor18111Iterator) Seek(t int64) ValueType {
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

func (it *xor18111Iterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xor18111Iterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xor18111Iterator.AtHistogram")
}

func (*xor18111Iterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xor18111Iterator.AtFloatHistogram")
}

func (it *xor18111Iterator) AtT() int64 {
	return it.t
}

func (*xor18111Iterator) AtST() int64 {
	return 0
}

func (it *xor18111Iterator) Err() error {
	return it.err
}

func (it *xor18111Iterator) Reset(b []byte) {
	// The first 2 bytes contain chunk headers.
	// We skip that for actual samples.
	it.br = newBReader(b[chunkHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)

	it.numRead = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.tDelta = 0
	it.err = nil
	it.baselineV = 0
	it.mode = xor18111ModeCompact
}

func (it *xor18111Iterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	if it.numRead == 0 {
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
		it.t = t
		it.val = math.Float64frombits(v)
		if !value.IsStaleNaN(it.val) {
			it.baselineV = it.val
		}
		it.numRead++
		return ValFloat
	}

	if it.numRead == 1 {
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		return it.readValue()
	}

	// Read timestamp delta-of-delta.
	if it.mode == xor18111ModeCompact {
		var d byte
		for range 4 {
			d <<= 1
			bit, err := it.br.readBitFast()
			if err != nil {
				bit, err = it.br.readBit()
			}
			if err != nil {
				it.err = err
				return ValNone
			}
			if bit == zero {
				break
			}
			d |= 1
		}

		// Check for mode switch marker (1111).
		if d == 0b1111 {
			it.mode = xor18111ModeFull
			if err := it.readTimestampDeltaFull(); err != nil {
				it.err = err
				return ValNone
			}
			return it.readValue()
		}

		var sz uint8
		var dod int64
		switch d {
		case 0b0:
			// dod == 0.
		case 0b10:
			sz = 14
		case 0b110:
			sz = 17
		case 0b1110:
			sz = 20
		}

		if sz != 0 {
			b, err := it.br.readBitsFast(sz)
			if err != nil {
				b, err = it.br.readBits(sz)
			}
			if err != nil {
				it.err = err
				return ValNone
			}
			if b > (1 << (sz - 1)) {
				b -= 1 << sz
			}
			dod = int64(b)
		}

		it.tDelta = uint64(int64(it.tDelta) + dod)
		it.t += int64(it.tDelta)
	} else {
		if err := it.readTimestampDeltaFull(); err != nil {
			it.err = err
			return ValNone
		}
	}

	return it.readValue()
}

func (it *xor18111Iterator) readTimestampDeltaFull() error {
	var d byte
	for range 5 {
		d <<= 1
		bit, err := it.br.readBitFast()
		if err != nil {
			bit, err = it.br.readBit()
		}
		if err != nil {
			return err
		}
		if bit == zero {
			break
		}
		d |= 1
	}

	var dod int64
	switch d {
	case 0b0:
		// dod == 0.
	case 0b10:
		b, err := it.br.readBitsFast(7)
		if err != nil {
			b, err = it.br.readBits(7)
		}
		if err != nil {
			return err
		}
		if b > (1 << 6) {
			b -= 1 << 7
		}
		dod = int64(b)
	case 0b110:
		b, err := it.br.readBitsFast(14)
		if err != nil {
			b, err = it.br.readBits(14)
		}
		if err != nil {
			return err
		}
		if b > (1 << 13) {
			b -= 1 << 14
		}
		dod = int64(b)
	case 0b1110:
		b, err := it.br.readBitsFast(20)
		if err != nil {
			b, err = it.br.readBits(20)
		}
		if err != nil {
			return err
		}
		if b > (1 << 19) {
			b -= 1 << 20
		}
		dod = int64(b)
	case 0b11110:
		sign, err := it.br.readBit()
		if err != nil {
			return err
		}
		b, err := it.br.readBits(4)
		if err != nil {
			return err
		}
		multiplier := int64(b) + 1
		if sign == one {
			multiplier = -multiplier
		}

		residualBits, err := it.br.readBits(8)
		if err != nil {
			return err
		}
		residual := int64(int8(residualBits))

		dod = multiplier*int64(it.tDelta) + residual
	case 0b11111:
		b, err := it.br.readBits(64)
		if err != nil {
			return err
		}
		dod = int64(b)
	}

	it.tDelta = uint64(int64(it.tDelta) + dod)
	it.t += int64(it.tDelta)
	return nil
}

// readValue reads a value with optimized staleness detection.
func (it *xor18111Iterator) readValue() ValueType {
	bit, err := it.br.readBitFast()
	if err != nil {
		bit, err = it.br.readBit()
	}
	if err != nil {
		it.err = err
		return ValNone
	}

	if bit == zero {
		// Value unchanged: return the baseline (last non-stale) value.
		it.val = it.baselineV
		it.numRead++
		return ValFloat
	}

	bit, err = it.br.readBitFast()
	if err != nil {
		bit, err = it.br.readBit()
	}
	if err != nil {
		it.err = err
		return ValNone
	}

	if bit == zero {
		// Reuse leading/trailing zeros.
		sz := 64 - int(it.leading) - int(it.trailing)
		b, err := it.br.readBitsFast(uint8(sz))
		if err != nil {
			b, err = it.br.readBits(uint8(sz))
		}
		if err != nil {
			it.err = err
			return ValNone
		}

		vbits := math.Float64bits(it.baselineV)
		vbits ^= b << it.trailing
		it.val = math.Float64frombits(vbits)
		it.baselineV = it.val
		it.numRead++
		return ValFloat
	}

	// Read new leading and sigbits.
	newLeading, err := it.br.readBitsFast(5)
	if err != nil {
		newLeading, err = it.br.readBits(5)
	}
	if err != nil {
		it.err = err
		return ValNone
	}

	sigbits, err := it.br.readBitsFast(6)
	if err != nil {
		sigbits, err = it.br.readBits(6)
	}
	if err != nil {
		it.err = err
		return ValNone
	}

	// The pattern leading=31, sigbits=63 is impossible in normal XOR encoding
	// (it would require trailing = 64 - 31 - 63 = -30) and is used as the
	// staleness marker.
	if newLeading == 31 && sigbits == 63 {
		it.val = math.Float64frombits(value.StaleNaN)
		it.numRead++
		return ValFloat
	}

	it.leading = uint8(newLeading)

	if sigbits == 0 {
		sigbits = 64
	}
	it.trailing = 64 - it.leading - uint8(sigbits)

	b, err := it.br.readBitsFast(uint8(sigbits))
	if err != nil {
		b, err = it.br.readBits(uint8(sigbits))
	}
	if err != nil {
		it.err = err
		return ValNone
	}

	vbits := math.Float64bits(it.baselineV)
	vbits ^= b << it.trailing
	it.val = math.Float64frombits(vbits)
	it.baselineV = it.val
	it.numRead++
	return ValFloat
}
