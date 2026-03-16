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
	"math/bits"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

// XOR2Chunk implements XOR encoding with joint timestamp+value control bits
// and byte-packed dod encoding for efficient appending.
//
// Control prefix for samples >= 2:
//
//	0     → dod=0 AND value unchanged              (1 bit)
//	10    → dod=0, value changed                   (2 bits, then value encoding)
//	110   → dod≠0, 13-bit signed [-4096, 4095]     (prefix+dod packed into 2 bytes)
//	1110  → dod≠0, 20-bit signed [-524288, 524287] (prefix+dod packed into 3 bytes)
//	11110 → dod≠0, 64-bit escape                   (5+64 bits, then value encoding)
//	11111 → dod=0, stale NaN                       (5 bits, no value field)
//
// The dod bins are widened so that prefix+dod aligns to byte boundaries,
// replacing writeBit calls with writeByte for common cases.
//
// Value encoding for the dod≠0 cases (`<varbit_xor2>`):
//
//	0   → value unchanged
//	10  → reuse previous leading/trailing window
//	110 → new leading/trailing window
//	111 → stale NaN
//
// Value encoding for the dod=0, value-changed case (`<varbit_xor2_nn>`):
//
//	0 → reuse previous leading/trailing window
//	1 → new leading/trailing window
type XOR2Chunk struct {
	b   bstream
	app xor2Appender
}

// NewXOR2Chunk returns a new chunk with XOR2 encoding.
func NewXOR2Chunk() *XOR2Chunk {
	return &XOR2Chunk{b: bstream{stream: make([]byte, chunkHeaderSize, chunkAllocationSize)}}
}

func (c *XOR2Chunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XOR2Chunk) Encoding() Encoding {
	return EncXOR2
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XOR2Chunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XOR2Chunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.b.bytes()))
}

// Compact implements the Chunk interface.
func (c *XOR2Chunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *XOR2Chunk) Appender() (Appender, error) {
	if len(c.b.stream) == chunkHeaderSize {
		c.app = xor2Appender{
			b:       &c.b,
			t:       math.MinInt64,
			leading: 0xff,
		}
		return &c.app, nil
	}
	it := c.iterator(nil)

	for it.Next() != ValNone {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	c.app = xor2Appender{
		b:        &c.b,
		num:      it.numTotal,
		t:        it.t,
		v:        it.baselineV,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,
	}
	return &c.app, nil
}

func (c *XOR2Chunk) iterator(it Iterator) *xor2Iterator {
	if xor2Iter, ok := it.(*xor2Iterator); ok {
		xor2Iter.Reset(c.b.bytes())
		return xor2Iter
	}
	return &xor2Iterator{
		br:       newBReader(c.b.bytes()[chunkHeaderSize:]),
		numTotal: binary.BigEndian.Uint16(c.b.bytes()),
		t:        math.MinInt64,
	}
}

// Iterator implements the Chunk interface.
func (c *XOR2Chunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// xor2Appender uses joint timestamp+value control bits and byte-packed dod bins.
type xor2Appender struct {
	b *bstream

	num uint16 // Number of samples appended so far; mirrors bytes[0:2] to avoid per-sample Uint16 reads.

	t      int64
	v      float64
	tDelta uint64

	leading  uint8
	trailing uint8
}

func (a *xor2Appender) Append(_, t int64, v float64) {
	var tDelta uint64
	num := a.num
	switch num {
	case 0:
		var buf [binary.MaxVarintLen64]byte
		for _, b := range buf[:binary.PutVarint(buf[:], t)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		tDelta = uint64(t - a.t)

		var buf [binary.MaxVarintLen64]byte
		for _, b := range buf[:binary.PutUvarint(buf[:], tDelta)] {
			a.b.writeByte(b)
		}

		a.writeVDelta(v)
	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		if dod == 0 {
			// Timestamp unchanged.
			vbits := math.Float64bits(v) ^ math.Float64bits(a.v)
			switch {
			case vbits == 0:
				// Both unchanged: single 0 bit.
				a.b.writeBit(zero)
			case value.IsStaleNaN(v):
				// Stale NaN with dod=0: joint control `11111`.
				a.b.writeBits(0b11111, 5)
			default:
				// Value changed: joint control `10` + value.
				a.b.writeBit(one)
				a.b.writeBit(zero)
				a.writeVDeltaKnownNonZero(vbits)
				a.v = v
				a.t = t
				binary.BigEndian.PutUint16(a.b.bytes(), num+1)
				a.num++
				a.tDelta = tDelta
				return
			}
		} else {
			// Timestamp changed: byte-packed dod encoding + value.
			switch {
			case dod >= -(1<<12) && dod <= (1<<12)-1:
				// 13-bit dod: prefix `110` packed with top 5 bits → 2 bytes total.
				a.b.writeByte(0b110_00000 | byte(uint64(dod)>>8)&0x1F)
				a.b.writeByte(byte(uint64(dod)))
			case dod >= -(1<<19) && dod <= (1<<19)-1:
				// 20-bit dod: prefix `1110` packed with top 4 bits → 3 bytes total.
				a.b.writeByte(0b1110_0000 | byte(uint64(dod)>>16)&0x0F)
				a.b.writeByte(byte(uint64(dod) >> 8))
				a.b.writeByte(byte(uint64(dod)))
			default:
				// 64-bit escape (rare): `11110`.
				a.b.writeBits(0b11110, 5)
				a.b.writeBits(uint64(dod), 64)
			}
			a.writeVDelta(v)
		}
	}

	a.t = t
	if !value.IsStaleNaN(v) {
		a.v = v
	}
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
	a.num++
	a.tDelta = tDelta
}

// writeVDelta encodes the value delta for the dod≠0 case.
// Encoding:
//
//	`0`   → value unchanged (XOR = 0)
//	`10`  → reuse previous leading/trailing window
//	`110` → new leading/trailing window
//	`111` → stale NaN marker
func (a *xor2Appender) writeVDelta(v float64) {
	if value.IsStaleNaN(v) {
		a.b.writeBits(0b111, 3)
		return
	}

	delta := math.Float64bits(v) ^ math.Float64bits(a.v)

	if delta == 0 {
		a.b.writeBit(zero)
		return
	}

	newLeading := uint8(bits.LeadingZeros64(delta))
	newTrailing := uint8(bits.TrailingZeros64(delta))

	if newLeading >= 32 {
		newLeading = 31
	}

	if a.leading != 0xff && newLeading >= a.leading && newTrailing >= a.trailing {
		a.b.writeBits(0b10, 2)
		a.b.writeBits(delta>>a.trailing, 64-int(a.leading)-int(a.trailing))
		return
	}

	a.leading, a.trailing = newLeading, newTrailing

	a.b.writeBits(0b110, 3)
	a.b.writeBits(uint64(newLeading), 5)

	sigbits := 64 - newLeading - newTrailing
	a.b.writeBits(uint64(sigbits), 6)
	a.b.writeBits(delta>>newTrailing, int(sigbits))
}

// writeVDeltaKnownNonZero encodes a precomputed value XOR delta for the
// dod=0, value-changed case. delta must be non-zero; stale NaN with dod=0 is
// handled at the joint control level (`11111`) and never reaches this function.
//
// Encoding:
//
//	`0` → reuse previous leading/trailing window
//	`1` → new leading/trailing window
func (a *xor2Appender) writeVDeltaKnownNonZero(delta uint64) {
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

func (*xor2Appender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xor2Appender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

// xor2Iterator decodes XOR2 chunks.
type xor2Iterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	t   int64
	val float64

	leading  uint8
	trailing uint8

	tDelta uint64
	err    error

	baselineV float64 // Last non-stale value for XOR baseline.
}

func (it *xor2Iterator) Seek(t int64) ValueType {
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

func (it *xor2Iterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xor2Iterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xor2Iterator.AtHistogram")
}

func (*xor2Iterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xor2Iterator.AtFloatHistogram")
}

func (it *xor2Iterator) AtT() int64 {
	return it.t
}

func (*xor2Iterator) AtST() int64 {
	return 0
}

func (it *xor2Iterator) Err() error {
	return it.err
}

func (it *xor2Iterator) Reset(b []byte) {
	it.br = newBReader(b[chunkHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)

	it.numRead = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.tDelta = 0
	it.baselineV = 0
	it.err = nil
}

func (it *xor2Iterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	if it.numRead == 0 {
		t, err := it.br.readVarint()
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
		tDelta, err := it.br.readUvarint()
		if err != nil {
			it.err = err
			return ValNone
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		return it.readValue()
	}

	ctrl, err := it.br.readXOR2ControlFast()
	if err != nil {
		ctrl, err = it.br.readXOR2Control()
		if err != nil {
			it.err = err
			return ValNone
		}
	}

	switch ctrl {
	case 0:
		// dod=0, value unchanged: `0`.
		it.t += int64(it.tDelta)
		it.val = it.baselineV
		it.numRead++
		return ValFloat
	case 1:
		// dod=0, value changed: `10`.
		it.t += int64(it.tDelta)
		return it.readValueKnownNonZero()
	case 2:
		// 13-bit dod: `110`.
		if err := it.readDod(13); err != nil {
			it.err = err
			return ValNone
		}
	case 3:
		// 20-bit dod: `1110`.
		if err := it.readDod(20); err != nil {
			it.err = err
			return ValNone
		}
	case 4:
		// 64-bit escape: `11110`.
		if err := it.readDod(64); err != nil {
			it.err = err
			return ValNone
		}
	default:
		// dod=0, stale NaN: `11111`.
		it.t += int64(it.tDelta)
		it.val = math.Float64frombits(value.StaleNaN)
		it.numRead++
		return ValFloat
	}

	return it.readValue()
}

func (it *xor2Iterator) readDod(w uint8) error {
	var b uint64
	if it.br.valid >= w {
		it.br.valid -= w
		b = (it.br.buffer >> it.br.valid) & ((uint64(1) << w) - 1)
	} else {
		var err error
		b, err = it.br.readBits(w)
		if err != nil {
			return err
		}
	}

	if w < 64 && b >= (1<<(w-1)) {
		b -= 1 << w
	}
	dod := int64(b)

	it.tDelta = uint64(int64(it.tDelta) + dod)
	it.t += int64(it.tDelta)
	return nil
}

func (it *xor2Iterator) readValue() ValueType {
	// Fast path: 3 bits available — read the full control prefix in one shot.
	// Encoding: `0`=unchanged, `10`=reuse window, `110`=new window, `111`=stale NaN.
	if it.br.valid >= 3 {
		ctrl := (it.br.buffer >> (it.br.valid - 3)) & 0x7
		if ctrl&0x4 == 0 {
			// `0xx`: value unchanged, consume 1 bit.
			it.br.valid--
			it.val = it.baselineV
			it.numRead++
			return ValFloat
		}
		if ctrl&0x6 == 0x4 {
			// `10x`: reuse previous leading/trailing window, consume 2 bits.
			it.br.valid -= 2
			sz := uint8(64 - int(it.leading) - int(it.trailing))
			var valueBits uint64
			if it.br.valid >= sz {
				it.br.valid -= sz
				valueBits = (it.br.buffer >> it.br.valid) & ((uint64(1) << sz) - 1)
			} else {
				var err error
				valueBits, err = it.br.readBits(sz)
				if err != nil {
					it.err = err
					return ValNone
				}
			}
			vbits := math.Float64bits(it.baselineV)
			vbits ^= valueBits << it.trailing
			it.val = math.Float64frombits(vbits)
			it.baselineV = it.val
			it.numRead++
			return ValFloat
		}
		// `11x`: consume 3 bits.
		it.br.valid -= 3
		if ctrl == 0x6 {
			// `110`: new leading/trailing window.
			return it.readNewLeadingTrailing()
		}
		// `111`: stale NaN.
		it.val = math.Float64frombits(value.StaleNaN)
		it.numRead++
		return ValFloat
	}

	// Slow path: fewer than 3 bits buffered (rare, only near buffer refills).
	var bit bit
	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			it.err = err
			return ValNone
		}
	}

	if bit == zero {
		// `0`: value unchanged.
		it.val = it.baselineV
		it.numRead++
		return ValFloat
	}

	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			it.err = err
			return ValNone
		}
	}

	if bit == zero {
		// `10`: reuse previous leading/trailing window.
		sz := uint8(64 - int(it.leading) - int(it.trailing))
		var valueBits uint64
		if it.br.valid >= sz {
			it.br.valid -= sz
			valueBits = (it.br.buffer >> it.br.valid) & ((uint64(1) << sz) - 1)
		} else {
			var err error
			valueBits, err = it.br.readBits(sz)
			if err != nil {
				it.err = err
				return ValNone
			}
		}
		vbits := math.Float64bits(it.baselineV)
		vbits ^= valueBits << it.trailing
		it.val = math.Float64frombits(vbits)
		it.baselineV = it.val
		it.numRead++
		return ValFloat
	}

	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			it.err = err
			return ValNone
		}
	}

	if bit == zero {
		// `110`: new leading/trailing window.
		return it.readNewLeadingTrailing()
	}

	// `111`: stale NaN.
	it.val = math.Float64frombits(value.StaleNaN)
	it.numRead++
	return ValFloat
}

func (it *xor2Iterator) readValueKnownNonZero() ValueType {
	sz := uint8(64 - int(it.leading) - int(it.trailing))
	// Fast path: combine the 1-bit reuse/new-window control read with the
	// sz-bit value read into a single buffer operation.
	if it.br.valid >= 1+sz {
		ctrlBit := (it.br.buffer >> (it.br.valid - 1)) & 1
		if ctrlBit == 0 { // '0': reuse previous leading/trailing window.
			it.br.valid -= 1 + sz
			valueBits := (it.br.buffer >> it.br.valid) & ((uint64(1) << sz) - 1)
			vbits := math.Float64bits(it.baselineV)
			vbits ^= valueBits << it.trailing
			it.val = math.Float64frombits(vbits)
			it.baselineV = it.val
			it.numRead++
			return ValFloat
		}
		// '1': new leading/trailing window.
		it.br.valid--
		return it.readNewLeadingTrailing()
	}

	// Slow path: read control bit then value bits separately.
	var bit bit
	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			it.err = err
			return ValNone
		}
	}

	if bit == zero {
		var valueBits uint64
		if it.br.valid >= sz {
			it.br.valid -= sz
			valueBits = (it.br.buffer >> it.br.valid) & ((uint64(1) << sz) - 1)
		} else {
			var err error
			valueBits, err = it.br.readBits(sz)
			if err != nil {
				it.err = err
				return ValNone
			}
		}

		vbits := math.Float64bits(it.baselineV)
		vbits ^= valueBits << it.trailing
		it.val = math.Float64frombits(vbits)
		it.baselineV = it.val
		it.numRead++
		return ValFloat
	}

	return it.readNewLeadingTrailing()
}

func (it *xor2Iterator) readNewLeadingTrailing() ValueType {
	var newLeading, sigbits uint64
	if it.br.valid >= 11 {
		val := (it.br.buffer >> (it.br.valid - 11)) & 0x7ff
		it.br.valid -= 11
		newLeading = val >> 6
		sigbits = val & 0x3f
	} else {
		var err error
		newLeading, err = it.br.readBits(5)
		if err != nil {
			it.err = err
			return ValNone
		}
		sigbits, err = it.br.readBits(6)
		if err != nil {
			it.err = err
			return ValNone
		}
	}

	it.leading = uint8(newLeading)

	if sigbits == 0 {
		sigbits = 64
	}
	it.trailing = 64 - it.leading - uint8(sigbits)

	n := uint8(sigbits)
	var valueBits uint64
	if it.br.valid >= n {
		it.br.valid -= n
		valueBits = (it.br.buffer >> it.br.valid) & ((uint64(1) << n) - 1)
	} else {
		var err error
		valueBits, err = it.br.readBits(n)
		if err != nil {
			it.err = err
			return ValNone
		}
	}

	vbits := math.Float64bits(it.baselineV)
	vbits ^= valueBits << it.trailing
	it.val = math.Float64frombits(vbits)
	it.baselineV = it.val
	it.numRead++
	return ValFloat
}
