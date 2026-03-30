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

// XOR2Chunk implements XOR encoding with joint timestamp+value control bits
// and byte-packed dod encoding for efficient appending. It also has an extra
// header byte after the sample count to allow for optionally encoding start
// timestamp (ST).
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
//
// Start timestamp (ST) encoding:
//
// 1-byte ST header (at b[chunkHeaderSize]) layout:
//
//	bit 7 (0x80): firstSTKnown   — ST for the first sample is present in the stream
//	bits 6-0:    firstSTChangeOn — sample index where the first ST change begins
//
// When no ST is provided (st == 0 always), the header stays 0x00 and the
// chunk has no additional bits in it.
//
// When ST is present, the ST delta (prevT - st) is appended after each
// sample's joint timestamp+value encoding using putVarbitIntFast.

package chunkenc

import (
	"encoding/binary"
	"math"
	"math/bits"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

const (
	chunkSTHeaderSize  = 1
	maxFirstSTChangeOn = 0x7F
)

func writeHeaderFirstSTKnown(b []byte) {
	b[0] = 0x80
}

func writeHeaderFirstSTChangeOn(b []byte, firstSTChangeOn uint16) {
	// First bit indicates the initial ST value.
	// Here we save the sample number from where the first change occurs in the
	// rest of the byte (7 bits)

	if firstSTChangeOn > maxFirstSTChangeOn {
		// This should never happen, would cause corruption (ST already skipped but shouldn't).
		return
	}
	b[0] |= uint8(firstSTChangeOn)
}

func readSTHeader(b []byte) (firstSTKnown bool, firstSTChangeOn uint8) {
	if b[0] == 0x00 {
		return false, 0
	}
	if b[0] == 0x80 {
		return true, 0
	}
	mask := byte(0x80)
	if b[0]&mask != 0 {
		firstSTKnown = true
	}
	mask = 0x7F
	return firstSTKnown, b[0] & mask
}

// XOR2Chunk holds XOR2 encoded samples with optional start
// timestamp per chunk or per sample.
type XOR2Chunk struct {
	b bstream
}

// NewXOR2Chunk returns a new chunk with XOR2 encoding.
func NewXOR2Chunk() *XOR2Chunk {
	b := make([]byte, chunkHeaderSize+chunkSTHeaderSize, chunkAllocationSize)
	return &XOR2Chunk{b: bstream{stream: b, count: 0}}
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
	return int(binary.BigEndian.Uint16(c.Bytes()))
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
	if len(c.b.stream) == chunkHeaderSize+chunkSTHeaderSize {
		return &xor2Appender{
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

	a := &xor2Appender{
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

func (c *XOR2Chunk) iterator(it Iterator) *xor2Iterator {
	if iter, ok := it.(*xor2Iterator); ok {
		iter.Reset(c.b.bytes())
		return iter
	}
	iter := &xor2Iterator{}
	iter.Reset(c.b.bytes())
	return iter
}

// Iterator implements the Chunk interface.
func (c *XOR2Chunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

// xor2Appender appends samples with optional start timestamps using
// the XOR2 joint control bit encoding for regular timestamp and value,
// and putVarbitIntFast for the start timestamp delta.
type xor2Appender struct {
	b *bstream

	st     int64
	t      int64
	v      float64
	tDelta uint64
	stDiff int64 // prevT - st for the previous sample.

	leading  uint8
	trailing uint8

	numTotal        uint16
	firstSTChangeOn uint16
	firstSTKnown    bool
}

func (a *xor2Appender) Append(st, t int64, v float64) {
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
		a.b.writeBitsFast(math.Float64bits(v), 64)

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
			putVarbitIntFast(a.b, stDiff)
		}

	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)

		// Fast path: no new ST data to write for this sample.
		// Covers: ST never seen (st=0 always), or ST recorded initially but unchanged.
		// Must use the slow path at maxFirstSTChangeOn so the header remains valid
		// even if ST changes on a later sample (index > maxFirstSTChangeOn).
		if a.firstSTChangeOn == 0 && st == a.st && a.numTotal != maxFirstSTChangeOn {
			vbits := math.Float64bits(v)
			switch {
			case dod == 0 && vbits == math.Float64bits(a.v):
				// Unchanged value and timestamp: write a single 0 bit.
				// This is the most common case for stable metrics.
				// a.v stays correct (v == a.v), so no update needed.
				a.b.writeBit(zero)
			case dod >= -(1<<12) && dod <= (1<<12)-1 && vbits == math.Float64bits(a.v):
				// 13-bit dod, value unchanged: the most common case for metrics with
				// small timestamp jitter. Inline both bytes and the zero value bit to
				// avoid calling encodeJoint and writeVDelta.
				a.b.writeByte(0b110_00000 | byte(uint64(dod)>>8)&0x1F)
				a.b.writeByte(byte(uint64(dod)))
				a.b.writeBit(zero)
			default:
				a.encodeJoint(dod, v)
				if !value.IsStaleNaN(v) {
					a.v = v
				}
			}
			a.t = t
			a.tDelta = tDelta
			a.numTotal++
			binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)
			return
		}

		// Active-ST fast path: firstSTChangeOn is set, so every sample needs a
		// per-sample ST delta. Inline T+V encoding and the zero-delta ST case to
		// avoid two non-inlined function calls (encodeJoint + putVarbitIntFast).
		if a.firstSTChangeOn > 0 {
			newStDiff := a.t - st
			deltaStDiff := newStDiff - a.stDiff
			vbits := math.Float64bits(v)
			switch {
			case dod == 0 && vbits == math.Float64bits(a.v):
				// T/V: single 0 bit (dod=0, value unchanged). For non-zero ST deltas
				// we fuse this bit with the ST delta write into a single writeBitsFast
				// call, saving a non-inlined writeBit call. For deltaStDiff=0 we use
				// two writeBit calls because writeBit has a smaller body than
				// writeBitsFast, making it faster for writing just 1 bit.
				switch {
				case deltaStDiff == 0:
					a.b.writeBit(zero)
					a.b.writeBit(zero)
				case deltaStDiff >= -3 && deltaStDiff <= 4:
					// 0 (T/V) + 5-bit ST = 6 bits.
					a.b.writeBitsFast((0b10<<3)|(uint64(deltaStDiff)&0x7), 6)
				case deltaStDiff >= -31 && deltaStDiff <= 32:
					// 0 (T/V) + 9-bit ST = 10 bits.
					a.b.writeBitsFast((0b110<<6)|(uint64(deltaStDiff)&0x3F), 10)
				case deltaStDiff >= -255 && deltaStDiff <= 256:
					// 0 (T/V) + 13-bit ST = 14 bits.
					a.b.writeBitsFast((0b1110<<9)|(uint64(deltaStDiff)&0x1FF), 14)
				default:
					a.b.writeBit(zero)
					putVarbitIntFast(a.b, deltaStDiff)
				}
			case dod >= -(1<<12) && dod <= (1<<12)-1 && vbits == math.Float64bits(a.v):
				a.b.writeByte(0b110_00000 | byte(uint64(dod)>>8)&0x1F)
				a.b.writeByte(byte(uint64(dod)))
				// T/V ends with a 0 bit (value unchanged indicator). Fuse it with
				// non-zero ST deltas to save a writeBit call; for deltaStDiff=0 keep
				// two cheap writeBit calls (faster than one writeBitsFast for 2 bits).
				switch {
				case deltaStDiff == 0:
					a.b.writeBit(zero)
					a.b.writeBit(zero)
				case deltaStDiff >= -3 && deltaStDiff <= 4:
					a.b.writeBitsFast((0b10<<3)|(uint64(deltaStDiff)&0x7), 6)
				case deltaStDiff >= -31 && deltaStDiff <= 32:
					a.b.writeBitsFast((0b110<<6)|(uint64(deltaStDiff)&0x3F), 10)
				case deltaStDiff >= -255 && deltaStDiff <= 256:
					a.b.writeBitsFast((0b1110<<9)|(uint64(deltaStDiff)&0x1FF), 14)
				default:
					a.b.writeBit(zero)
					putVarbitIntFast(a.b, deltaStDiff)
				}
			default:
				a.encodeJoint(dod, v)
				if !value.IsStaleNaN(v) {
					a.v = v
				}
				// Inline the three most common ST delta ranges to avoid the
				// non-inlineable putVarbitIntFast call for typical small-jitter STs.
				switch {
				case deltaStDiff == 0:
					a.b.writeBit(zero)
				case deltaStDiff >= -3 && deltaStDiff <= 4:
					a.b.writeBitsFast((0b10<<3)|(uint64(deltaStDiff)&0x7), 5)
				case deltaStDiff >= -31 && deltaStDiff <= 32:
					a.b.writeBitsFast((0b110<<6)|(uint64(deltaStDiff)&0x3F), 9)
				case deltaStDiff >= -255 && deltaStDiff <= 256:
					a.b.writeBitsFast((0b1110<<9)|(uint64(deltaStDiff)&0x1FF), 13)
				default:
					putVarbitIntFast(a.b, deltaStDiff)
				}
			}
			a.stDiff = newStDiff
			a.st = st
			a.t = t
			a.tDelta = tDelta
			a.numTotal++
			binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)
			return
		}

		// Full slow path: firstSTChangeOn == 0 and ST may be initialised here.
		a.encodeJoint(dod, v)

		if st != a.st || a.numTotal == maxFirstSTChangeOn {
			// First ST change: record prevT - st.
			stDiff = a.t - st
			a.firstSTChangeOn = a.numTotal
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.numTotal)
			putVarbitIntFast(a.b, stDiff)
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

// encodeJoint writes the XOR2 joint timestamp+value control sequence for
// samples >= 2.
func (a *xor2Appender) encodeJoint(dod int64, v float64) {
	if dod == 0 {
		if value.IsStaleNaN(v) {
			a.b.writeBitsFast(0b11111, 5)
			return
		}
		vbits := math.Float64bits(v) ^ math.Float64bits(a.v)
		if vbits == 0 {
			a.b.writeBit(zero)
			return
		}
		a.b.writeBitsFast(0b10, 2)
		a.writeVDeltaKnownNonZero(vbits)
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
		a.b.writeBitsFast(0b11110, 5)
		a.b.writeBitsFast(uint64(dod), 64)
	}
	// Inline the most common value-unchanged case to avoid a function call.
	if math.Float64bits(v) == math.Float64bits(a.v) {
		a.b.writeBit(zero)
	} else {
		a.writeVDelta(v)
	}
}

// writeVDelta encodes the value delta for the dod≠0 case.
func (a *xor2Appender) writeVDelta(v float64) {
	if value.IsStaleNaN(v) {
		a.b.writeBitsFast(0b111, 3)
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
		a.b.writeBitsFast(0b10, 2)
		a.b.writeBitsFast(delta>>a.trailing, 64-int(a.leading)-int(a.trailing))
		return
	}

	a.leading, a.trailing = newLeading, newTrailing

	a.b.writeBitsFast(0b110, 3)
	a.b.writeBitsFast(uint64(newLeading), 5)

	sigbits := 64 - newLeading - newTrailing
	a.b.writeBitsFast(uint64(sigbits), 6)
	a.b.writeBitsFast(delta>>newTrailing, int(sigbits))
}

// writeVDeltaKnownNonZero encodes a precomputed value XOR delta for the
// dod=0, value-changed case. delta must be non-zero or staleNaN. Stale NaN with dod=0 is
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
		a.b.writeBitsFast(delta>>a.trailing, 64-int(a.leading)-int(a.trailing))
		return
	}

	a.leading, a.trailing = newLeading, newTrailing

	a.b.writeBit(one)
	a.b.writeBitsFast(uint64(newLeading), 5)

	sigbits := 64 - newLeading - newTrailing
	a.b.writeBitsFast(uint64(sigbits), 6)
	a.b.writeBitsFast(delta>>newTrailing, int(sigbits))
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

	firstSTKnown    bool
	firstSTChangeOn uint8

	leading  uint8
	trailing uint8

	st  int64
	t   int64
	val float64

	tDelta uint64
	stDiff int64 // Accumulated prevT - st.
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

func (it *xor2Iterator) AtST() int64 {
	return it.st
}

func (it *xor2Iterator) Err() error {
	return it.err
}

func (it *xor2Iterator) Reset(b []byte) {
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

		// Optional ST for sample 0.
		if it.firstSTKnown {
			stDiff, err := it.br.readVarint()
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
		tDelta, err := it.br.readUvarint()
		if err != nil {
			it.err = err
			return ValNone
		}
		prevT := it.t
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		if err := it.decodeValue(); err != nil {
			it.err = err
			return ValNone
		}

		// Optional ST delta for sample 1.
		if it.firstSTChangeOn == 1 {
			sdod, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.stDiff = sdod
			it.st = prevT - sdod
		}

		it.numRead++
		return ValFloat
	}

	// Sample N >= 2: read joint XOR2 control, then optional ST data.
	prevT := it.t
	savedNumRead := it.numRead

	ctrl, ok := it.br.readXOR2ControlFast()
	if !ok {
		var err error
		ctrl, err = it.br.readXOR2Control()
		if err != nil {
			it.err = err
			return ValNone
		}
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
	// The ST delta was encoded as (prevT - st), using the PREVIOUS sample's t.
	if it.firstSTChangeOn > 0 && savedNumRead >= uint16(it.firstSTChangeOn) {
		sdod, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		if savedNumRead == uint16(it.firstSTChangeOn) {
			it.stDiff = sdod
		} else {
			it.stDiff += sdod
		}
		it.st = prevT - it.stDiff
	}

	it.numRead++
	return ValFloat
}

// readDod reads a signed dod of width w bits and updates it.tDelta and it.t.
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

	it.tDelta = uint64(int64(it.tDelta) + int64(b))
	it.t += int64(it.tDelta)
	return nil
}

// decodeValue reads the XOR2 value encoding for the dod≠0 case:
//
//	`0`   → value unchanged
//	`10`  → reuse previous leading/trailing window
//	`110` → new leading/trailing window
//	`111` → stale NaN
func (it *xor2Iterator) decodeValue() error {
	// Fast path: 3 bits available — read the full control prefix in one shot.
	// Encoding: `0`=unchanged, `10`=reuse window, `110`=new window, `111`=stale NaN.
	if it.br.valid >= 3 {
		ctrl := (it.br.buffer >> (it.br.valid - 3)) & 0x7
		if ctrl&0x4 == 0 {
			// `0xx`: value unchanged, consume 1 bit.
			it.br.valid--
			it.val = it.baselineV
			return nil
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
					return err
				}
			}
			vbits := math.Float64bits(it.baselineV)
			vbits ^= valueBits << it.trailing
			it.val = math.Float64frombits(vbits)
			it.baselineV = it.val
			return nil
		}
		// `11x`: consume 3 bits.
		it.br.valid -= 3
		if ctrl == 0x6 {
			// `110`: new leading/trailing window.
			return it.decodeNewLeadingTrailing()
		}
		// `111`: stale NaN.
		it.val = math.Float64frombits(value.StaleNaN)
		return nil
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

// decodeValueKnownNonZero reads the XOR2 value encoding for the dod=0,
// value-changed case:
//
//	`0` → reuse previous leading/trailing window
//	`1` → new leading/trailing window
func (it *xor2Iterator) decodeValueKnownNonZero() error {
	sz := uint8(64 - int(it.leading) - int(it.trailing))
	// Fast path: combine the 1-bit reuse/new-window control read with the
	// sz-bit value read into a single buffer operation.
	if it.br.valid >= 1+sz {
		ctrlBit := (it.br.buffer >> (it.br.valid - 1)) & 1
		if ctrlBit == 0 { // `0`: reuse previous leading/trailing window.
			it.br.valid -= 1 + sz
			valueBits := (it.br.buffer >> it.br.valid) & ((uint64(1) << sz) - 1)
			vbits := math.Float64bits(it.baselineV)
			vbits ^= valueBits << it.trailing
			it.val = math.Float64frombits(vbits)
			it.baselineV = it.val
			return nil
		}
		// `1`: new leading/trailing window.
		it.br.valid--
		return it.decodeNewLeadingTrailing()
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
			return err
		}
	}

	if bit == zero {
		// `0` → reuse previous leading/trailing window.
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
func (it *xor2Iterator) decodeNewLeadingTrailing() error {
	var newLeading, sigbits uint64
	// Fast path: read leading (5 bits) and sigbits (6 bits) together as 11 bits.
	if it.br.valid >= 11 {
		val := (it.br.buffer >> (it.br.valid - 11)) & 0x7ff
		it.br.valid -= 11
		newLeading = val >> 6
		sigbits = val & 0x3f
	} else {
		var err error
		newLeading, err = it.br.readBits(5)
		if err != nil {
			return err
		}
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
