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

func readSTHeader(b []byte) (firstSTKnown bool, firstSTChangeOn uint16) {
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
	return firstSTKnown, uint16(b[0] & mask)
}

// XorOptSTChunk holds encoded sample data:
// 2B(numSamples), 1B(stHeader), ?varint(st), varint(t), xor(v), ?varuint(stDelta), varuint(tDelta), xor(v), ?classicvarbitint(stDod), classicvarbitint(tDod), xor(v), ...
// stHeader: 1b(firstSTKnown), 7b(firstSTChangeOn).
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
	return int(binary.BigEndian.Uint16(c.Bytes()))
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

	a := &xorOptSTAppender{
		b:        &c.b,
		st:       it.st,
		t:        it.t,
		v:        it.val,
		stDelta:  it.stDelta,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,

		numTotal:        it.numTotal,
		firstSTKnown:    it.firstSTKnown,
		firstSTChangeOn: it.firstSTChangeOn,
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
	b               *bstream
	numTotal        uint16
	firstSTChangeOn uint16
	leading         uint8
	trailing        uint8
	firstSTKnown    bool
	st, t           int64
	v               float64
	stDelta         int64
	tDelta          uint64
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

const (
	read0State uint8 = iota
	read1State
	readDoDMaybeSTState
	readDoDNoSTState
	readDoDState

	eofState uint8 = 1<<8 - 1
)

type xorOptSTtIterator struct {
	br       bstreamReader
	numTotal uint16

	firstSTKnown    bool
	firstSTChangeOn uint16

	state   uint8
	numRead uint16

	st, t int64
	val   float64

	leading  uint8
	trailing uint8

	stDelta int64
	tDelta  uint64
	err     error
}

func (it *xorOptSTtIterator) Seek(t int64) ValueType {
	if it.state == eofState {
		return ValNone
	}

	for t > it.t || it.state == read0State {
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
	it.numTotal = binary.BigEndian.Uint16(b)
	it.firstSTKnown, it.firstSTChangeOn = readSTHeader(b[chunkHeaderSize:])
	it.numRead = 0
	it.st = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.stDelta = 0
	it.tDelta = 0
	it.err = nil
	it.state = read0State
	if it.numRead >= it.numTotal {
		it.state = eofState
	}
}

func (a *xorOptSTAppender) Append(st, t int64, v float64) {
	if st == 0 && a.numTotal != maxFirstSTChangeOn && a.firstSTChangeOn == 0 && !a.firstSTKnown {
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
		binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)

		return
	}

	var (
		stDelta int64
		tDelta  uint64
	)

	// Slow path for ST usage.
	switch a.numTotal {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)

		for _, b := range buf[:binary.PutVarint(buf, st)] {
			a.b.writeByte(b)
		}
		a.firstSTKnown = true
		writeHeaderFirstSTKnown(a.b.bytes()[chunkHeaderSize:])

		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		buf := make([]byte, binary.MaxVarintLen64)
		stDelta = st - a.st
		if stDelta != 0 {
			a.firstSTChangeOn = 1
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], 1)
			for _, b := range buf[:binary.PutVarint(buf, stDelta)] {
				a.b.writeByte(b)
			}
		}

		tDelta = uint64(t - a.t)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}
		a.writeVDelta(v)
	default:
		stDelta = st - a.st
		sdod := stDelta - a.stDelta
		if sdod != 0 {
			if a.firstSTChangeOn == 0 {
				a.firstSTChangeOn = a.numTotal
				writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.numTotal)
			}
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
			// 	putClassicVarbitInt(a.b, sdod)
		} else {
			if a.firstSTChangeOn == 0 {
				if a.numTotal == maxFirstSTChangeOn {
					// We are at the 127th sample. firstSTChangeOn can only fit
					// 7 bits due to a single byte header constrain, which is fine,
					// given typical 120 sample size.
					a.firstSTChangeOn = maxFirstSTChangeOn
					writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], maxFirstSTChangeOn)
					a.b.writeBit(zero)
				}
			} else {
				a.b.writeBit(zero)
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
	a.stDelta = stDelta

	a.numTotal++
	binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)
}

func (it *xorOptSTtIterator) retErr(err error) ValueType {
	it.err = err
	it.state = eofState
	return ValNone
}

func (it *xorOptSTtIterator) Next() ValueType {
	switch it.state {
	case eofState:
		return ValNone
	case read0State:
		it.state++

		// Optional ST read.
		if it.firstSTKnown {
			st, err := binary.ReadVarint(&it.br)
			if err != nil {
				return it.retErr(err)
			}
			it.st = st
		}

		// TS.
		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}
		// Value.
		v, err := it.br.readBits(64)
		if err != nil {
			return it.retErr(err)
		}

		it.t = t
		it.val = math.Float64frombits(v)

		// State EOF check.
		it.numRead++
		if it.numRead >= it.numTotal {
			it.state = eofState
		}
		return ValFloat
	case read1State:
		it.state++
		switch it.firstSTChangeOn {
		case 0:
			// This means we have same (zero or non-zero) ST value for the rest of
			// chunk. We can simply use ~classic XOR chunk iterations.
			it.state = readDoDNoSTState
		case 1:
			// We got early ST change on the second sample, likely delta.
			// Continue ST rich flow from the next iteration.
			it.state = readDoDState

			stDelta, err := binary.ReadVarint(&it.br)
			if err != nil {
				return it.retErr(err)
			}
			it.stDelta = stDelta
			it.st += it.stDelta
		}
		// TS.
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		// Value.
		if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
			return it.retErr(err)
		}

		// State EOF check.
		it.numRead++
		if it.numRead >= it.numTotal {
			it.state = eofState
		}
		return ValFloat
	case readDoDMaybeSTState:
		if it.firstSTChangeOn == it.numRead {
			// ST changes from this iteration, change state for future.
			it.state = readDoDState
			return it.dodNext()
		}
		return it.dodNoSTNext()
	case readDoDState:
		return it.dodNext()
	case readDoDNoSTState:
		return it.dodNoSTNext()
	default:
		panic("xorOptSTtIterator: broken machine state")
	}
}

func (it *xorOptSTtIterator) dodNext() ValueType {
	// Inlined readClassicVarbitInt(&it.br)
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

	it.stDelta += sdod
	it.st += it.stDelta
	return it.dodNoSTNext()
}

func (it *xorOptSTtIterator) dodNoSTNext() ValueType {
	// Inlined readClassicVarbitInt(&it.br)
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
			if err != nil {
				return it.retErr(err)
			}
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
	// Value.
	if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
		return it.retErr(err)
	}

	// State EOF check.
	it.numRead++
	if it.numRead >= it.numTotal {
		it.state = eofState
	}
	return ValFloat
}
