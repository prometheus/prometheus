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

// This file implements the XOR18238OPTST2 chunk encoding: XOR18238 with
// a combined ST encoding that captures the strengths of both XOR18238OPTST
// and the prefix-table approach.
//
// The ST header byte (at b[chunkHeaderSize]) uses the same layout as XOROptST:
//
//	bit 7 (0x80): firstSTKnown   — ST for the first sample is present in the stream
//	bits 6-0:    firstSTChangeOn — sample index where the first ST change begins
//
// When no ST is provided (st == 0 always), the header stays 0x00 and the
// chunk is byte-for-byte identical to XOR18238.
//
// Starting from the second sample, ST changes are encoded with a 1-bit prefix
// followed by XOR18238OPTST's varbit dod encoding for the non-zero case:
//
//	0                  — d(ST) = 0: ST unchanged (efficient for constant-ST series)
//	1 <varbit-int>     — dod(prevT - st): the delta-of-delta of (prevT - st)
//
// This combines the two strengths:
//   - The "0" prefix handles constant or mostly-zero ST cheaply (1 bit/sample).
//   - The varbit dod(prevT-st) encoding handles delta-offset metrics efficiently,
//     since (prevT - st) is nearly constant when ST tracks T at a fixed lag.

package chunkenc

import (
	"encoding/binary"
	"math"
	"math/bits"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

// XOR18238OPTST2Chunk holds XOR18238 encoded samples with optional start
// timestamp per chunk or per sample. See XOROptST for the ST header format.
type XOR18238OPTST2Chunk struct {
	b bstream
}

// NewXOR18238OPTST2Chunk returns a new chunk with XOR18238OPTST2 encoding.
func NewXOR18238OPTST2Chunk() *XOR18238OPTST2Chunk {
	b := make([]byte, chunkHeaderSize+chunkSTHeaderSize, chunkAllocationSize)
	return &XOR18238OPTST2Chunk{b: bstream{stream: b, count: 0}}
}

func (c *XOR18238OPTST2Chunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XOR18238OPTST2Chunk) Encoding() Encoding {
	return EncXOR18238OPTST2
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XOR18238OPTST2Chunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XOR18238OPTST2Chunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *XOR18238OPTST2Chunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
func (c *XOR18238OPTST2Chunk) Appender() (Appender, error) {
	if len(c.b.stream) == chunkHeaderSize+chunkSTHeaderSize {
		return &xor18238OPTST2Appender{
			b:       &c.b,
			t:       math.MinInt64,
			leading: 0xff,
		}, nil
	}
	it := c.iterator(nil)

	for it.Next() != ValNone {
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	// Set the bit position for continuing writes. The iterator's reader tracks
	// how many bits remain unread in the last byte.
	c.b.count = it.br.valid

	a := &xor18238OPTST2Appender{
		b:               &c.b,
		st:              it.st,
		t:               it.t,
		v:               it.baselineV,
		tDelta:          it.tDelta,
		stDiff:          it.stDiff,
		leading:         it.leading,
		trailing:        it.trailing,
		numTotal:        binary.BigEndian.Uint16(c.b.bytes()),
		firstSTKnown:    it.firstSTKnown,
		firstSTChangeOn: uint16(it.firstSTChangeOn),
	}
	return a, nil
}

func (c *XOR18238OPTST2Chunk) iterator(it Iterator) *xor18238OPTST2Iterator {
	if iter, ok := it.(*xor18238OPTST2Iterator); ok {
		iter.Reset(c.b.bytes())
		return iter
	}
	iter := &xor18238OPTST2Iterator{}
	iter.Reset(c.b.bytes())
	return iter
}

// Iterator implements the Chunk interface.
func (c *XOR18238OPTST2Chunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// xor18238OPTST2Appender appends samples with optional start timestamps.
// ST encoding after the first change uses a 1-bit prefix: 0 = unchanged,
// 1 = varbit dod(prevT-st) (same quantity as XOR18238OPTST).
type xor18238OPTST2Appender struct {
	b *bstream

	st      int64
	t       int64
	v       float64
	tDelta  uint64
	stDiff  int64 // prevT - st for the previous sample (same as XOR18238OPTST).

	leading  uint8
	trailing uint8

	numTotal        uint16
	firstSTChangeOn uint16
	firstSTKnown    bool
}

func (a *xor18238OPTST2Appender) Append(st, t int64, v float64) {
	var (
		tDelta uint64
		stDiff int64
	)

	switch a.numTotal {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)

		if st != 0 {
			for _, b := range buf[:binary.PutVarint(buf, t-st)] {
				a.b.writeByte(b)
			}
			a.firstSTKnown = true
			writeHeaderFirstSTKnown(a.b.bytes()[chunkHeaderSize:])
		}

	case 1:
		tDelta = uint64(t - a.t)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}

		a.writeVDelta(v)

		if st != a.st {
			stDiff = a.t - st
			a.firstSTChangeOn = 1
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], 1)
			a.b.writeBit(one)
			putVarbitInt(a.b, stDiff)
		}

	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		// Fast path: no ST involvement at all.
		if st == 0 && a.numTotal != maxFirstSTChangeOn && a.firstSTChangeOn == 0 && !a.firstSTKnown {
			a.encodeJoint(dod, v)
			a.t = t
			if !value.IsStaleNaN(v) {
				a.v = v
			}
			a.tDelta = tDelta
			a.numTotal++
			binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)
			return
		}

		// Slow path: ST may be involved.
		a.encodeJoint(dod, v)

		if a.firstSTChangeOn == 0 {
			if st != a.st || a.numTotal == maxFirstSTChangeOn {
				stDiff = a.t - st
				a.firstSTChangeOn = a.numTotal
				writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.numTotal)
				if st == a.st {
					// Forced by maxFirstSTChangeOn; ST has not changed.
					a.b.writeBit(zero)
				} else {
					// First ST change: write absolute stDiff (no dod yet).
					a.b.writeBit(one)
					putVarbitInt(a.b, stDiff)
				}
			}
		} else {
			stDiff = a.t - st
			if st == a.st {
				// ST unchanged: 1-bit prefix only, advance tracking.
				a.b.writeBit(zero)
			} else {
				// ST changed: 1-bit prefix + varbit dod(prevT-st).
				a.b.writeBit(one)
				putVarbitInt(a.b, stDiff-a.stDiff)
			}
		}
	}

	a.st = st
	a.t = t
	if !value.IsStaleNaN(v) {
		a.v = v
	}
	a.tDelta = tDelta
	a.stDiff = stDiff
	a.numTotal++
	binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)
}

// encodeJoint writes the XOR18238 joint timestamp+value control sequence for
// samples >= 2.
func (a *xor18238OPTST2Appender) encodeJoint(dod int64, v float64) {
	if dod == 0 {
		switch {
		case value.IsStaleNaN(v):
			a.b.writeBits(0b11111, 5)
		case math.Float64bits(v)^math.Float64bits(a.v) == 0:
			a.b.writeBit(zero)
		default:
			a.b.writeBits(0b10, 2)
			a.writeVDeltaKnownNonZero(v)
		}
		return
	}

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

// writeVDelta encodes the value delta for the dod≠0 case.
func (a *xor18238OPTST2Appender) writeVDelta(v float64) {
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

// writeVDeltaKnownNonZero encodes the value delta when it is known to be
// non-zero and non-stale (dod=0, value-changed case).
func (a *xor18238OPTST2Appender) writeVDeltaKnownNonZero(v float64) {
	delta := math.Float64bits(v) ^ math.Float64bits(a.v)

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

func (*xor18238OPTST2Appender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xor18238OPTST2Appender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

// xor18238OPTST2Iterator decodes XOR18238OPTST2 chunks.
type xor18238OPTST2Iterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	firstSTKnown    bool
	firstSTChangeOn uint8

	leading  uint8
	trailing uint8

	st  int64
	t   int64
	val float64

	tDelta  uint64
	stDiff  int64 // prevT - st for the previous sample (same as XOR18238OPTST).
	err     error

	baselineV float64 // Last non-stale value for XOR baseline.
}

func (it *xor18238OPTST2Iterator) Seek(t int64) ValueType {
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

func (it *xor18238OPTST2Iterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xor18238OPTST2Iterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xor18238OPTST2Iterator.AtHistogram")
}

func (*xor18238OPTST2Iterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xor18238OPTST2Iterator.AtFloatHistogram")
}

func (it *xor18238OPTST2Iterator) AtT() int64 {
	return it.t
}

func (it *xor18238OPTST2Iterator) AtST() int64 {
	return it.st
}

func (it *xor18238OPTST2Iterator) Err() error {
	return it.err
}

func (it *xor18238OPTST2Iterator) Reset(b []byte) {
	it.br = newBReader(b[chunkHeaderSize+chunkSTHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[chunkHeaderSize:])

	it.numRead = 0
	it.st = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.tDelta = 0
	it.stDiff = 0
	it.baselineV = 0
	it.err = nil
}

func (it *xor18238OPTST2Iterator) Next() ValueType {
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

		// Optional ST for sample 0.
		if it.firstSTKnown {
			stDiff, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.st = t - stDiff
		}

		it.numRead++
		return ValFloat
	}

	if it.numRead == 1 {
		prevT := it.t // t[0], needed for stDiff computation.
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		if err := it.decodeValue(); err != nil {
			it.err = err
			return ValNone
		}

		// Optional ST for sample 1.
		if it.firstSTChangeOn == 1 {
			if err := it.decodeST(prevT); err != nil {
				it.err = err
				return ValNone
			}
		}

		it.numRead++
		return ValFloat
	}

	// Sample N >= 2: read joint XOR18238 control, then optional ST data.
	prevT := it.t // save before the switch updates it.t.
	savedNumRead := it.numRead

	ctrl, err := it.br.readXOR18238Control()
	if err != nil {
		it.err = err
		return ValNone
	}

	switch ctrl {
	case 0:
		// dod=0, value unchanged.
		it.t += int64(it.tDelta)
		it.val = it.baselineV
	case 1:
		// dod=0, value changed.
		it.t += int64(it.tDelta)
		if err := it.decodeValueKnownNonZero(); err != nil {
			it.err = err
			return ValNone
		}
	case 2:
		// 13-bit dod.
		if err := it.readDod(13); err != nil {
			it.err = err
			return ValNone
		}
		if err := it.decodeValue(); err != nil {
			it.err = err
			return ValNone
		}
	case 3:
		// 20-bit dod.
		if err := it.readDod(20); err != nil {
			it.err = err
			return ValNone
		}
		if err := it.decodeValue(); err != nil {
			it.err = err
			return ValNone
		}
	case 4:
		// 64-bit escape.
		if err := it.readDod(64); err != nil {
			it.err = err
			return ValNone
		}
		if err := it.decodeValue(); err != nil {
			it.err = err
			return ValNone
		}
	default:
		// dod=0, stale NaN.
		it.t += int64(it.tDelta)
		it.val = math.Float64frombits(value.StaleNaN)
	}

	// Optional ST data, appended after the joint timestamp+value encoding.
	if it.firstSTChangeOn > 0 && savedNumRead >= uint16(it.firstSTChangeOn) {
		if err := it.decodeST(prevT); err != nil {
			it.err = err
			return ValNone
		}
	}

	it.numRead++
	return ValFloat
}

// decodeST decodes the combined ST encoding and updates it.st and it.stDiff.
// prevT is the previous sample's timestamp (t[N-1] when decoding sample N).
//
// Format: 0 = ST unchanged; 1 + varbit-int = dod(prevT-st).
func (it *xor18238OPTST2Iterator) decodeST(prevT int64) error {
	bit, err := it.br.readBit()
	if err != nil {
		return err
	}
	if bit == zero {
		// ST unchanged; advance stDiff tracking so the next dod is correct.
		it.stDiff = prevT - it.st
		return nil
	}

	sdod, err := readVarbitInt(&it.br)
	if err != nil {
		return err
	}
	if it.numRead == uint16(it.firstSTChangeOn) {
		// First write: sdod is the absolute stDiff value, not a delta.
		it.stDiff = sdod
	} else {
		it.stDiff += sdod
	}
	it.st = prevT - it.stDiff
	return nil
}

// readDod reads a signed dod of width w bits and updates it.tDelta and it.t.
func (it *xor18238OPTST2Iterator) readDod(w uint8) error {
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

	it.tDelta = uint64(int64(it.tDelta) + int64(b))
	it.t += int64(it.tDelta)
	return nil
}

// decodeValue reads the XOR18238 value encoding for the dod≠0 case:
//
//	`0`   → value unchanged
//	`10`  → reuse previous leading/trailing window
//	`110` → new leading/trailing window
//	`111` → stale NaN
func (it *xor18238OPTST2Iterator) decodeValue() error {
	var bit bit
	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			return err
		}
	}

	if bit == zero {
		// `0` → value unchanged.
		it.val = it.baselineV
		return nil
	}

	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			return err
		}
	}

	if bit == zero {
		// `10` → reuse previous leading/trailing window.
		sz := uint8(64 - int(it.leading) - int(it.trailing))
		var valueBits uint64
		if it.br.valid >= sz {
			it.br.valid -= sz
			valueBits = (it.br.buffer >> it.br.valid) & ((uint64(1) << sz) - 1)
		} else {
			var err error
			valueBits, err = it.br.readBits(sz)
			if err != nil {
				return err
			}
		}
		vbits := math.Float64bits(it.baselineV)
		vbits ^= valueBits << it.trailing
		it.val = math.Float64frombits(vbits)
		it.baselineV = it.val
		return nil
	}

	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			return err
		}
	}

	if bit == zero {
		// `110` → new leading/trailing window.
		return it.decodeNewLeadingTrailing()
	}

	// `111` → stale NaN.
	it.val = math.Float64frombits(value.StaleNaN)
	return nil
}

// decodeValueKnownNonZero reads the XOR18238 value encoding for the dod=0,
// value-changed case:
//
//	`0` → reuse previous leading/trailing window
//	`1` → new leading/trailing window
func (it *xor18238OPTST2Iterator) decodeValueKnownNonZero() error {
	var bit bit
	if it.br.valid > 0 {
		it.br.valid--
		bit = (it.br.buffer & (uint64(1) << it.br.valid)) != 0
	} else {
		var err error
		bit, err = it.br.readBit()
		if err != nil {
			return err
		}
	}

	if bit == zero {
		// `0` → reuse previous leading/trailing window.
		sz := uint8(64 - int(it.leading) - int(it.trailing))
		var valueBits uint64
		if it.br.valid >= sz {
			it.br.valid -= sz
			valueBits = (it.br.buffer >> it.br.valid) & ((uint64(1) << sz) - 1)
		} else {
			var err error
			valueBits, err = it.br.readBits(sz)
			if err != nil {
				return err
			}
		}
		vbits := math.Float64bits(it.baselineV)
		vbits ^= valueBits << it.trailing
		it.val = math.Float64frombits(vbits)
		it.baselineV = it.val
		return nil
	}

	// `1` → new leading/trailing window.
	return it.decodeNewLeadingTrailing()
}

// decodeNewLeadingTrailing reads a new leading/sigbits/value triple and
// updates it.leading, it.trailing, it.val, and it.baselineV.
func (it *xor18238OPTST2Iterator) decodeNewLeadingTrailing() error {
	var newLeading uint64
	if it.br.valid >= 5 {
		it.br.valid -= 5
		newLeading = (it.br.buffer >> it.br.valid) & 0x1f
	} else {
		var err error
		newLeading, err = it.br.readBits(5)
		if err != nil {
			return err
		}
	}

	var sigbits uint64
	if it.br.valid >= 6 {
		it.br.valid -= 6
		sigbits = (it.br.buffer >> it.br.valid) & 0x3f
	} else {
		var err error
		sigbits, err = it.br.readBits(6)
		if err != nil {
			return err
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
			return err
		}
	}

	vbits := math.Float64bits(it.baselineV)
	vbits ^= valueBits << it.trailing
	it.val = math.Float64frombits(vbits)
	it.baselineV = it.val
	return nil
}
