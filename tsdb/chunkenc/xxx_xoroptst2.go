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

// xorOptST2Chunk holds encoded sample data:
// 2B(numSamples), 1B(stHeader), ?st(st), varint(t), xor(v), ?varuint(stDelta), varuint(tDelta), xor(v), ?stvarbitint(stDod), classicvarbitint(tDod), xor(v), ...
// stHeader: 1b(firstSTKnown), 7b(firstSTChangeOn)
type xorOptST2Chunk struct {
	b bstream
}

// NewXOROptST2Chunk returns a new chunk with XORv2 encoding.
func NewXOROptST2Chunk() *xorOptST2Chunk {
	b := make([]byte, chunkHeaderSize+chunkSTHeaderSize, chunkAllocationSize)
	return &xorOptST2Chunk{b: bstream{stream: b, count: 0}}
}

func (c *xorOptST2Chunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*xorOptST2Chunk) Encoding() Encoding {
	return 199
}

// Bytes returns the underlying byte slice of the chunk.
func (c *xorOptST2Chunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *xorOptST2Chunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *xorOptST2Chunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *xorOptST2Chunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *xorOptST2Chunk) AppenderV2() (AppenderV2, error) {
	if len(c.b.stream) == chunkHeaderSize+chunkSTHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorOptST2Appender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
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

	a := &xorOptST2Appender{
		b:        &c.b,
		st:       it.st,
		t:        it.t,
		v:        it.val,
		stDelta:  it.stDelta,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,

		numTotal:        it.numTotal,
		firstSTChangeOn: it.firstSTChangeOn,
	}
	return a, nil
}

func (c *xorOptST2Chunk) iterator(it Iterator) *xorOptST2tIterator {
	xorIter, ok := it.(*xorOptST2tIterator)
	if !ok {
		xorIter = &xorOptST2tIterator{}
	}

	xorIter.Reset(c.b.bytes())
	return xorIter
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *xorOptST2Chunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorOptST2Appender struct {
	b        *bstream
	numTotal uint16

	firstSTChangeOn uint16

	st, t   int64
	v       float64
	stDelta int64
	tDelta  uint64

	leading  uint8
	trailing uint8
}

func (a *xorOptST2Appender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorOptST2Appender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorOptST2Appender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorOptST2tIterator struct {
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

	nextFn func() ValueType
}

func (it *xorOptST2tIterator) Seek(t int64) ValueType {
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

func (it *xorOptST2tIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorOptST2tIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorOptST2tIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorOptST2tIterator) AtT() int64 {
	return it.t
}

func (it *xorOptST2tIterator) AtST() int64 {
	return it.st
}

func (it *xorOptST2tIterator) Err() error {
	return it.err
}

func (it *xorOptST2tIterator) Reset(b []byte) {
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

func (a *xorOptST2Appender) Append(st, t int64, v float64) {
	var (
		stDelta   int64
		tDelta    uint64
		stChanged bool
	)

	switch a.numTotal {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		if st != 0 {
			for _, b := range buf[:binary.PutVarint(buf, st)] {
				a.b.writeByte(b)
			}
			writeHeaderFirstSTKnown(a.b.bytes()[chunkHeaderSize:])
		}

		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		buf := make([]byte, binary.MaxVarintLen64)
		stDelta = st - a.st
		if stDelta != 0 {
			stChanged = true
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
		if a.firstSTChangeOn == 0 && a.numTotal == maxFirstSTChangeOn {
			// We are at the 127th sample. firstSTChangeOn can only fit 7 bits due to a
			// single byte header constrain, which is fine, given typical 120 sample size.
			a.firstSTChangeOn = a.numTotal
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.firstSTChangeOn)
		}

		stDelta = st - a.st
		sdod := stDelta - a.stDelta
		if sdod != 0 || a.firstSTChangeOn != 0 {
			stChanged = true
			putSTVarbitInt(a.b, sdod)
		}

		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		putSTVarbitInt(a.b, dod)
		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta
	a.stDelta = stDelta

	a.numTotal++
	binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)

	// firstSTChangeOn == 0 indicates that we have one ST value (zero or not)
	// for all STs in the appends until now. If we see a first "update"
	// we are saving this number in the header and continue tracking all DoD (including zeros).
	if a.firstSTChangeOn == 0 && stChanged {
		a.firstSTChangeOn = a.numTotal - 1
		writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.firstSTChangeOn)
	}
}

func (a *xorOptST2Appender) BitProfiledAppend(p *bitProfiler[any], st, t int64, v float64) {
	var (
		stDelta   int64
		tDelta    uint64
		stChanged bool
	)

	switch a.numTotal {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		if st != 0 {
			p.Write(a.b, t, "st", func() {
				for _, b := range buf[:binary.PutVarint(buf, st)] {
					a.b.writeByte(b)
				}
				writeHeaderFirstSTKnown(a.b.bytes()[chunkHeaderSize:])
			})
		}
		p.Write(a.b, t, "t", func() {
			for _, b := range buf[:binary.PutVarint(buf, t)] {
				a.b.writeByte(b)
			}
		})
		p.Write(a.b, v, "v", func() {
			a.b.writeBits(math.Float64bits(v), 64)
		})
	case 1:
		buf := make([]byte, binary.MaxVarintLen64)
		stDelta = st - a.st
		if stDelta != 0 {
			stChanged = true
			p.Write(a.b, t, "stDelta", func() {
				for _, b := range buf[:binary.PutVarint(buf, stDelta)] {
					a.b.writeByte(b)
				}
			})
		}

		tDelta = uint64(t - a.t)
		p.Write(a.b, t, "tDelta", func() {
			for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
				a.b.writeByte(b)
			}
		})
		p.Write(a.b, v, "v", func() {
			a.writeVDelta(v)
		})
	default:
		if a.firstSTChangeOn == 0 && a.numTotal == maxFirstSTChangeOn {
			// We are at the 127th sample. firstSTChangeOn can only fit 7 bits due to a
			// single byte header constrain, which is fine, given typical 120 sample size.
			a.firstSTChangeOn = a.numTotal
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.firstSTChangeOn)
		}

		stDelta = st - a.st
		sdod := stDelta - a.stDelta
		if sdod != 0 || a.firstSTChangeOn != 0 {
			stChanged = true
			p.Write(a.b, dodSample{t: t, tDelta: tDelta, dod: sdod}, "stDod", func() {
				stChanged = true
				putSTVarbitInt(a.b, sdod)
			})
		}

		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		p.Write(a.b, dodSample{t: t, tDelta: tDelta, dod: sdod}, "tDod", func() {
			putSTVarbitInt(a.b, dod)
		})
		p.Write(a.b, v, "v", func() {
			a.writeVDelta(v)
		})
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta
	a.stDelta = stDelta

	a.numTotal++
	binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)

	// firstSTChangeOn == 0 indicates that we have one ST value (zero or not)
	// for all STs in the appends until now. If we see a first "update" OR
	// we are at the 127th sample, we are saving this number in the header
	// and continue tracking all DoD (including zeros). 0x7F is due to a single byte
	// header constrain, which is fine, given typical 120 sample size.
	if a.firstSTChangeOn == 0 && (stChanged || a.numTotal > 0x7F) {
		a.firstSTChangeOn = a.numTotal - 1
		writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.firstSTChangeOn)
	}
}

func (it *xorOptST2tIterator) retErr(err error) ValueType {
	it.err = err
	it.state = eofState
	return ValNone
}

func (it *xorOptST2tIterator) Next() ValueType {
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
		if it.firstSTChangeOn == 0 {
			// This means we have same (zero or non-zero) ST value for the rest of
			// chunk. We can simply use ~classic XOR chunk iterations.
			it.state = readDoDNoSTState
		} else if it.firstSTChangeOn == 1 {
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
		panic("xorOptST2tIterator: broken machine state")
	}
}

func (it *xorOptST2tIterator) dodNext() ValueType {
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
		sz = 6
	case 0b110:
		sz = 13
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

	it.stDelta = it.stDelta + sdod
	it.st += it.stDelta
	return it.dodNoSTNext()
}

func (it *xorOptST2tIterator) dodNoSTNext() ValueType {
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
		sz = 6
	case 0b110:
		sz = 13
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
