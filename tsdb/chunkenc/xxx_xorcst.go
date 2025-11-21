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

// xorOptCSTChunk holds encoded sample data:
// 2B(numSamples), 1B(stHeader), varint(t), ?varint(st), xor(v), varuint(tDelta), ?stDiff(st), xor(v), classicvarbitint(tDod),?stDiff(st),  xor(v), ...
// stHeader: 1b(firstSTKnown), 7b(firstSTChangeOn)
type xorCSTChunk struct {
	b bstream
}

// NewXORCSTChunk returns a new chunk with XOR encoding.
func NewXORCSTChunk() *xorCSTChunk {
	b := make([]byte, chunkHeaderSize+chunkSTHeaderSize, chunkAllocationSize)
	return &xorCSTChunk{b: bstream{stream: b, count: 0}}
}

func (c *xorCSTChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*xorCSTChunk) Encoding() Encoding {
	return EncXORCST
}

// Bytes returns the underlying byte slice of the chunk.
func (c *xorCSTChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *xorCSTChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *xorCSTChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *xorCSTChunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *xorCSTChunk) AppenderV2() (AppenderV2, error) {
	if len(c.b.stream) == chunkHeaderSize+chunkSTHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorCSTAppender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
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

	a := &xorCSTAppender{
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

func (c *xorCSTChunk) iterator(it Iterator) *xorCSTtIterator {
	xorIter, ok := it.(*xorCSTtIterator)
	if !ok {
		xorIter = &xorCSTtIterator{}
	}

	xorIter.Reset(c.b.bytes())
	return xorIter
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *xorCSTChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorCSTAppender struct {
	b        *bstream
	numTotal uint16

	firstSTChangeOn uint16

	st, t  int64
	v      float64
	tDelta uint64

	leading  uint8
	trailing uint8
}

func (a *xorCSTAppender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorCSTAppender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorCSTAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorCSTtIterator struct {
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

func (it *xorCSTtIterator) Seek(t int64) ValueType {
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

func (it *xorCSTtIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorCSTtIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorCSTtIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorCSTtIterator) AtT() int64 {
	return it.t
}

func (it *xorCSTtIterator) AtST() int64 {
	return it.st
}

func (it *xorCSTtIterator) Err() error {
	return it.err
}

func (it *xorCSTtIterator) Reset(b []byte) {
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

func (a *xorCSTAppender) Append(st, t int64, v float64) {
	var (
		tDelta    uint64
		stChanged bool
	)

	switch a.numTotal {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		if st != 0 {
			for _, b := range buf[:binary.PutVarint(buf, st)] {
				a.b.writeByte(b)
			}
			writeHeaderFirstSTKnown(a.b.bytes()[chunkHeaderSize:])
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		tDelta = uint64(t - a.t)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}

		stChanged = st != a.st
		if stChanged {
			putSTDiff(a.b, false, t-st)
		}
		a.writeVDelta(v)
	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		putClassicVarbitInt(a.b, dod)

		// ST.
		if a.firstSTChangeOn == 0 && a.numTotal == maxFirstSTChangeOn {
			// We are at the 127th sample. firstSTChangeOn can only fit 7 bits due to a
			// single byte header constrain, which is fine, given typical 120 sample size.
			a.firstSTChangeOn = a.numTotal
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.firstSTChangeOn)
		}
		stChanged = st != a.st
		if stChanged || a.firstSTChangeOn != 0 {
			putSTDiff(a.b, !stChanged, int64(tDelta)-(t-st))
		}
		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta

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

func (a *xorCSTAppender) BitProfiledAppend(p *bitProfiler[any], st, t int64, v float64) {
	var (
		tDelta    uint64
		stChanged bool
	)

	switch a.numTotal {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		if st != 0 {
			for _, b := range buf[:binary.PutVarint(buf, st)] {
				a.b.writeByte(b)
			}
			writeHeaderFirstSTKnown(a.b.bytes()[chunkHeaderSize:])
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		tDelta = uint64(t - a.t)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}

		stChanged = st != a.st
		if stChanged {
			putSTDiff(a.b, false, t-st)
		}
		a.writeVDelta(v)
	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		putClassicVarbitInt(a.b, dod)

		// ST.
		if a.firstSTChangeOn == 0 && a.numTotal == maxFirstSTChangeOn {
			// We are at the 127th sample. firstSTChangeOn can only fit 7 bits due to a
			// single byte header constrain, which is fine, given typical 120 sample size.
			a.firstSTChangeOn = a.numTotal
			writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.firstSTChangeOn)
		}
		stChanged = st != a.st
		if stChanged || a.firstSTChangeOn != 0 {
			putSTDiff(a.b, stChanged, t-st)
		}
		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta

	a.numTotal++
	binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)

	// firstSTChangeOn == 0 indicates that we have one ST value (zero or not)
	// for all STs in the appends until now. If we see a first "update"
	// we are saving this number in the header and continue tracking all DoD (including zeros).
	if a.firstSTChangeOn == 0 && stChanged {
		a.firstSTChangeOn = a.numTotal - 1
		writeHeaderFirstSTChangeOn(a.b.bytes()[chunkHeaderSize:], a.firstSTChangeOn)
	}
	//p.Write(a.b, t, "st", func() {
	//	for _, b := range buf[:binary.PutVarint(buf, st)] {
	//		a.b.writeByte(b)
	//	}
	//	writeHeaderFirstSTKnown(a.b.bytes()[chunkHeaderSize:])
	//})
}

func (it *xorCSTtIterator) retErr(err error) ValueType {
	it.err = err
	it.state = eofState
	return ValNone
}

func (it *xorCSTtIterator) Next() ValueType {
	switch it.state {
	case eofState:
		return ValNone
	case read0State:
		it.state++
		// TS.
		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}

		// ST.
		if it.firstSTKnown {
			st, err := binary.ReadVarint(&it.br)
			if err != nil {
				return it.retErr(err)
			}
			it.st = st
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
		// TS.
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			return it.retErr(err)
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		// ST.
		if it.firstSTChangeOn == 0 {
			// This means we have same (zero or non-zero) ST value for the rest of
			// chunk. We can simply use ~classic XOR chunk iterations.
			it.state = readDoDNoSTState
		} else if it.firstSTChangeOn == 1 {
			// We got early ST change on the second sample, likely delta.
			// Continue ST rich flow from the next iteration.
			it.state = readDoDState

			_, stDiff, err := readSTDiff(&it.br)
			if err != nil {
				return it.retErr(err)
			}
			it.st = it.t - stDiff
		}

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
		panic("xorCSTtIterator: broken machine state")
	}
}

func (it *xorCSTtIterator) dodNext() ValueType {
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

	// ST.
	noChange, stDoDDiff, err := readSTDiff(&it.br)
	if err != nil {
		return it.retErr(err)
	}
	if !noChange {
		// Our value is the delta of tDelta (Dod).
		it.st = it.t + (stDoDDiff - int64(it.tDelta))
	}

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

func (it *xorCSTtIterator) dodNoSTNext() ValueType {
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
