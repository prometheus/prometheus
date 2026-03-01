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

// This file implements the XOR18111ST2 chunk encoding: XOR18111 with an
// additional start timestamp that has its own independent append mode.

package chunkenc

import (
	"encoding/binary"
	"math"
	"math/bits"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

// XOR18111ST2Chunk holds XOR18111ST2 encoded sample data: XOR18111 encoding
// with start timestamp stored alongside each sample's regular timestamp.
// The start timestamp has its own independent mode that can switch from compact
// to full independently of the regular timestamp mode.
type XOR18111ST2Chunk struct {
	b bstream
}

// NewXOR18111ST2Chunk returns a new chunk with XOR18111ST2 encoding.
func NewXOR18111ST2Chunk() *XOR18111ST2Chunk {
	b := make([]byte, chunkHeaderSize, chunkAllocationSize)
	return &XOR18111ST2Chunk{b: bstream{stream: b, count: 0}}
}

func (c *XOR18111ST2Chunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XOR18111ST2Chunk) Encoding() Encoding {
	return EncXOR18111ST2
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XOR18111ST2Chunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XOR18111ST2Chunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *XOR18111ST2Chunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *XOR18111ST2Chunk) Appender() (Appender, error) {
	if len(c.b.stream) == chunkHeaderSize {
		return &xor18111st2Appender{
			b:       &c.b,
			t:       math.MinInt64,
			leading: 0xff,
			mode:    xor18111ModeCompact,
			stMode:  xor18111ModeCompact,
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

	a := &xor18111st2Appender{
		b:        &c.b,
		t:        it.t,
		st:       it.st,
		v:        it.baselineV,
		tDelta:   it.tDelta,
		stDelta:  it.stDelta,
		leading:  it.leading,
		trailing: it.trailing,
		mode:     it.mode,
		stMode:   it.stMode,
	}
	return a, nil
}

func (c *XOR18111ST2Chunk) iterator(it Iterator) *xor18111st2Iterator {
	if xor18111st2Iter, ok := it.(*xor18111st2Iterator); ok {
		xor18111st2Iter.Reset(c.b.bytes())
		return xor18111st2Iter
	}
	return &xor18111st2Iterator{
		br:       newBReader(c.b.bytes()[chunkHeaderSize:]),
		numTotal: binary.BigEndian.Uint16(c.b.bytes()),
		t:        math.MinInt64,
		mode:     xor18111ModeCompact,
		stMode:   xor18111ModeCompact,
	}
}

// Iterator implements the Chunk interface.
func (c *XOR18111ST2Chunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// xor18111st2Appender appends samples with start timestamps using the
// XOR18111ST2 encoding. The start timestamp has its own independent mode.
type xor18111st2Appender struct {
	b *bstream

	t       int64
	st      int64
	v       float64
	tDelta  uint64
	stDelta int64

	leading  uint8
	trailing uint8

	mode   uint8
	stMode uint8
}

func (a *xor18111st2Appender) Append(st, t int64, v float64) {
	var (
		tDelta  uint64
		stDelta int64
	)
	num := binary.BigEndian.Uint16(a.b.bytes())
	switch num {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		for _, b := range buf[:binary.PutVarint(buf, st)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		tDelta = uint64(t - a.t)
		stDelta = st - a.st

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}
		for _, b := range buf[:binary.PutVarint(buf, stDelta)] {
			a.b.writeByte(b)
		}

		a.writeVDelta(v)
	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		// Encode the regular timestamp dod. This may switch the timestamp mode
		// to full.
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

		// Encode the start timestamp dod using the ST's own independent mode.
		stDelta = st - a.st
		stDod := stDelta - a.stDelta
		a.writeSTDod(stDod)

		a.writeVDelta(v)
	}

	a.t = t
	a.st = st
	if !value.IsStaleNaN(v) {
		a.v = v
	}
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
	a.tDelta = tDelta
	a.stDelta = stDelta
}

// writeTimestampDeltaFull encodes a timestamp dod in full mode. This is
// identical to the method in xor18111Appender.
func (a *xor18111st2Appender) writeTimestampDeltaFull(dod int64) {
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

				if residual >= -128 && residual <= 127 {
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

// writeSTDod encodes the start timestamp delta-of-delta using the ST's own
// independent mode. In compact mode, 0b1111 triggers a mode switch to full
// (same semantics as the regular timestamp).
func (a *xor18111st2Appender) writeSTDod(stDod int64) {
	if a.stMode == xor18111ModeCompact {
		switch {
		case stDod == 0:
			a.b.writeBit(zero)
		case bitRange(stDod, 14):
			a.b.writeByte(0b10<<6 | (uint8(stDod>>8) & (1<<6 - 1)))
			a.b.writeByte(uint8(stDod))
		case bitRange(stDod, 17):
			a.b.writeBits(0b110, 3)
			a.b.writeBits(uint64(stDod), 17)
		case bitRange(stDod, 20):
			a.b.writeBits(0b1110, 4)
			a.b.writeBits(uint64(stDod), 20)
		default:
			// Mode switch: write marker and switch to full.
			a.b.writeBits(0b1111, 4)
			a.stMode = xor18111ModeFull
			a.writeSTDodFull(stDod)
		}
	} else {
		a.writeSTDodFull(stDod)
	}
}

// writeSTDodFull encodes a start timestamp dod in full mode. Uses stDelta as
// the multiplier reference since the ST advances independently of the
// timestamp.
func (a *xor18111st2Appender) writeSTDodFull(stDod int64) {
	switch {
	case stDod == 0:
		a.b.writeBit(zero)
	case bitRange(stDod, 7):
		a.b.writeBits(0b10, 2)
		a.b.writeBits(uint64(stDod), 7)
	case bitRange(stDod, 14):
		a.b.writeBits(0b110, 3)
		a.b.writeBits(uint64(stDod), 14)
	case bitRange(stDod, 20):
		a.b.writeBits(0b1110, 4)
		a.b.writeBits(uint64(stDod), 20)
	default:
		encoded := false
		if a.stDelta != 0 && stDod != 0 {
			multiplierF := float64(stDod) / float64(a.stDelta)
			multiplier := int64(multiplierF)
			if multiplierF > 0 && multiplierF-float64(multiplier) >= 0.5 {
				multiplier++
			} else if multiplierF < 0 && float64(multiplier)-multiplierF >= 0.5 {
				multiplier--
			}

			if multiplier >= -15 && multiplier <= 15 && multiplier != 0 {
				reconstructed := multiplier * a.stDelta
				residual := stDod - reconstructed

				if residual >= -128 && residual <= 127 {
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
			a.b.writeBits(uint64(stDod), 64)
		}
	}
}

// writeVDelta encodes the value delta with optimized staleness handling.
// This is identical to the method in xor18111Appender.
func (a *xor18111st2Appender) writeVDelta(v float64) {
	if value.IsStaleNaN(v) {
		a.b.writeBit(one)
		a.b.writeBit(one)
		a.b.writeBits(31, 5)
		a.b.writeBits(63, 6)
		return
	}

	delta := math.Float64bits(v) ^ math.Float64bits(a.v)

	if delta == 0 {
		a.b.writeBit(zero)
		return
	}
	a.b.writeBit(one)

	newLeading := uint8(bits.LeadingZeros64(delta))
	newTrailing := uint8(bits.TrailingZeros64(delta))

	if newLeading >= 32 {
		newLeading = 31
	}

	if a.leading != 0xff && newLeading >= a.leading && newTrailing >= a.trailing {
		a.b.writeBit(zero)
		a.b.writeBits(delta>>a.trailing, 64-int(a.leading)-int(a.trailing))
		return
	}

	a.leading, a.trailing = newLeading, newTrailing

	a.b.writeBit(one)
	a.b.writeBits(uint64(newLeading), 5)

	sigbits := 64 - newLeading - newTrailing
	a.b.writeBits(uint64(sigbits), 6)
	a.b.writeBits(delta>>newTrailing, int(sigbits))
}

func (*xor18111st2Appender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xor18111st2Appender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

// xor18111st2Iterator decodes XOR18111ST2 chunks.
type xor18111st2Iterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	t   int64
	st  int64
	val float64

	leading  uint8
	trailing uint8

	tDelta  uint64
	stDelta int64
	err     error

	baselineV float64
	mode      uint8
	stMode    uint8
}

func (it *xor18111st2Iterator) Seek(t int64) ValueType {
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

func (it *xor18111st2Iterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xor18111st2Iterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xor18111st2Iterator.AtHistogram")
}

func (*xor18111st2Iterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xor18111st2Iterator.AtFloatHistogram")
}

func (it *xor18111st2Iterator) AtT() int64 {
	return it.t
}

func (it *xor18111st2Iterator) AtST() int64 {
	return it.st
}

func (it *xor18111st2Iterator) Err() error {
	return it.err
}

func (it *xor18111st2Iterator) Reset(b []byte) {
	it.br = newBReader(b[chunkHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)

	it.numRead = 0
	it.t = 0
	it.st = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.tDelta = 0
	it.stDelta = 0
	it.err = nil
	it.baselineV = 0
	it.mode = xor18111ModeCompact
	it.stMode = xor18111ModeCompact
}

func (it *xor18111st2Iterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	if it.numRead == 0 {
		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.t = t

		st, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.st = st

		v, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return ValNone
		}
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

		stDelta, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.stDelta = stDelta
		it.st += it.stDelta

		return it.readValue()
	}

	// Read the regular timestamp dod. This may switch the timestamp mode to
	// full.
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

		if d == 0b1111 {
			// Mode switch marker: switch to full and read a full-mode dod.
			it.mode = xor18111ModeFull
			if err := it.readTimestampDeltaFull(); err != nil {
				it.err = err
				return ValNone
			}
		} else {
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
		}
	} else {
		if err := it.readTimestampDeltaFull(); err != nil {
			it.err = err
			return ValNone
		}
	}

	// Read the start timestamp dod using the ST's own independent mode.
	if err := it.readSTDod(); err != nil {
		it.err = err
		return ValNone
	}

	return it.readValue()
}

// readTimestampDeltaFull reads a timestamp dod in full mode and updates it.t.
func (it *xor18111st2Iterator) readTimestampDeltaFull() error {
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
		dod = multiplier*int64(it.tDelta) + int64(int8(residualBits))
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

// readSTDod reads the start timestamp dod using the ST's own independent mode
// and updates it.st.
func (it *xor18111st2Iterator) readSTDod() error {
	if it.stMode == xor18111ModeCompact {
		var d byte
		for range 4 {
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

		if d == 0b1111 {
			// Mode switch: switch to full and read a full-mode dod.
			it.stMode = xor18111ModeFull
			return it.readSTDodFull()
		}

		var sz uint8
		var stDod int64
		switch d {
		case 0b0:
			// stDod == 0.
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
				return err
			}
			if b > (1 << (sz - 1)) {
				b -= 1 << sz
			}
			stDod = int64(b)
		}

		it.stDelta += stDod
		it.st += it.stDelta
		return nil
	}

	return it.readSTDodFull()
}

// readSTDodFull reads an ST dod in full mode and updates it.st. Uses stDelta
// as the multiplier reference.
func (it *xor18111st2Iterator) readSTDodFull() error {
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

	var stDod int64
	switch d {
	case 0b0:
		// stDod == 0.
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
		stDod = int64(b)
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
		stDod = int64(b)
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
		stDod = int64(b)
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
		stDod = multiplier*it.stDelta + int64(int8(residualBits))
	case 0b11111:
		b, err := it.br.readBits(64)
		if err != nil {
			return err
		}
		stDod = int64(b)
	}

	it.stDelta += stDod
	it.st += it.stDelta
	return nil
}

// readValue reads a value with optimized staleness detection.
// This is identical to the method in xor18111Iterator.
func (it *xor18111st2Iterator) readValue() ValueType {
	bit, err := it.br.readBitFast()
	if err != nil {
		bit, err = it.br.readBit()
	}
	if err != nil {
		it.err = err
		return ValNone
	}

	if bit == zero {
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
