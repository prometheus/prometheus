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

// xorOptCSTChunk holds encoded sample data:
// 2B(numSamples+2bit ST header), varint(t), ?varint(st), xor(v), varuint(tDelta), ?stDiff(st), xor(v), classicvarbitint(tDod),?stDiff(st),  xor(v), ...
// stHeader: 00b - no STs, 10b - ST on first sample otherwise const, 11b - ST encoded for all samples
type xorRecodingChunk struct {
	b bstream
}

const (
	HasSTMASK       = uint16(0b1000000000000000)
	HasSTChangeMASK = uint16(0b0100000000000000)
	NumSampleMASK   = uint16(0b0011111111111111)
)

// NewxorRecodingChunk returns a new chunk with XOR encoding.
func NewXorRecodingChunk() *xorRecodingChunk {
	b := make([]byte, chunkHeaderSize, chunkAllocationSize)
	return &xorRecodingChunk{b: bstream{stream: b, count: 0}}
}

func (c *xorRecodingChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*xorRecodingChunk) Encoding() Encoding {
	return EncXORCST
}

// Bytes returns the underlying byte slice of the chunk.
func (c *xorRecodingChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *xorRecodingChunk) NumSamples() int {
	return int(c.header() & NumSampleMASK)
}

func (c *xorRecodingChunk) header() uint16 {
	return binary.BigEndian.Uint16(c.Bytes())
}

// Compact implements the Chunk interface.
func (c *xorRecodingChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *xorRecodingChunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *xorRecodingChunk) AppenderV2() (AppenderV2, error) {
	if len(c.b.stream) == chunkHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorRecodingAppender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
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

	a := &xorRecodingAppender{
		b:        &c.b,
		st:       it.st,
		t:        it.t,
		v:        it.val,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,

		header: it.header,
	}
	return a, nil
}

func (c *xorRecodingChunk) iterator(it Iterator) *xorRecodingIterator {
	xorIter, ok := it.(*xorRecodingIterator)
	if !ok {
		xorIter = &xorRecodingIterator{}
	}

	xorIter.Reset(c.b.bytes())
	return xorIter
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *xorRecodingChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorRecodingAppender struct {
	b      *bstream
	header uint16

	st, t  int64
	v      float64
	tDelta uint64

	leading  uint8
	trailing uint8
}

func (a *xorRecodingAppender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorRecodingAppender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorRecodingAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorRecodingIterator struct {
	br     bstreamReader
	header uint16

	numRead uint16

	st, t int64
	val   float64

	leading  uint8
	trailing uint8

	stDelta int64
	tDelta  uint64
	err     error
}

func (it *xorRecodingIterator) numTotal() uint16 {
	return it.header & NumSampleMASK
}

func (it *xorRecodingIterator) Seek(t int64) ValueType {
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

func (it *xorRecodingIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorRecodingIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorRecodingIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorRecodingIterator) AtT() int64 {
	return it.t
}

func (it *xorRecodingIterator) AtST() int64 {
	return it.st
}

func (it *xorRecodingIterator) Err() error {
	return it.err
}

func (it *xorRecodingIterator) Reset(b []byte) {
	// We skip initial headers for actual samples.
	it.br = newBReader(b[chunkHeaderSize:])
	it.header = binary.BigEndian.Uint16(b)
	it.numRead = 0
	it.st = 0
	it.t = 0
	it.val = 0
	it.leading = 0
	it.trailing = 0
	it.stDelta = 0
	it.tDelta = 0
	it.err = nil
}

func (a *xorRecodingAppender) Append(st, t int64, v float64) {
	var tDelta uint64

	header := a.header

	switch header & NumSampleMASK {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, t)] {
			a.b.writeByte(b)
		}
		if st != 0 {
			header |= HasSTMASK
			a.st = st
			for _, b := range buf[:binary.PutVarint(buf, st)] {
				a.b.writeByte(b)
			}
		}
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		tDelta = uint64(t - a.t)

		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
			a.b.writeByte(b)
		}

		if st != a.st {
			if header&HasSTMASK == 0 {
				a.recodeWithST(false)
				header |= HasSTMASK
			}
			header |= HasSTChangeMASK
			putSTDiff(a.b, false, t-st)
		}

		a.writeVDelta(v)
	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		putClassicVarbitInt(a.b, dod)

		if st == a.st {
			if header&HasSTChangeMASK != 0 {
				putSTDiff(a.b, true, 0)
			}
		} else {
			if header&HasSTChangeMASK == 0 {
				a.recodeWithST(header&HasSTMASK != 0)
				header |= HasSTMASK
			}
			header |= HasSTChangeMASK
			putSTDiff(a.b, false, int64(tDelta)-(t-st))
		}

		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta
	header++
	a.header = header
	binary.BigEndian.PutUint16(a.b.bytes(), header)
}

// recodeWithST rewrites the chunk to include ST values for all samples.
// It assumes that:
// - the first sample is already written without ST
// - (optional) some samples written without ST
// - there is at least one timestamp written for the sample that triggers recode
func (a *xorRecodingAppender) recodeWithST(hasST bool) {
	numSamples := int(a.header & NumSampleMASK)
	tmp := bstream{
		stream: make([]byte, chunkHeaderSize, len(a.b.stream)),
	}
	defer func() {
		// Replace old stream with the new one.
		a.b.stream = tmp.stream
		a.b.count = tmp.count
	}()
	// We skip copy of the header, that will be set by Append later, just copy
	// the samples.
	br := newBReader(a.b.stream[chunkHeaderSize:])
	buf := make([]byte, binary.MaxVarintLen64)

	var (
		t        int64
		tDelta   uint64
		st       int64
		err      error
		v, vPrev float64

		readLeading   uint8
		readTrailing  uint8
		writeLeading  uint8 = 0xff
		writeTrailing uint8
	)

	// Copy first sample t.
	t, err = binary.ReadVarint(&br)
	if err != nil {
		// This should never happen, data is coming from memory, not disk.
		panic(err)
	}

	for _, b := range buf[:binary.PutVarint(buf, t)] {
		tmp.writeByte(b)
	}

	if hasST {
		// Copy first sample st.
		st, err = binary.ReadVarint(&br)
		if err != nil {
			// This should never happen, data is coming from memory, not disk.
			panic(err)
		}
		for _, b := range buf[:binary.PutVarint(buf, st)] {
			tmp.writeByte(b)
		}
	} else {
		// Put ST as zero.
		for _, b := range buf[:binary.PutVarint(buf, 0)] {
			tmp.writeByte(b)
		}
	}

	// Copy first sample value.
	vBits, err := br.readBits(64)
	if err != nil {
		// This should never happen, data is coming from memory, not disk.
		panic(err)
	}
	v = math.Float64frombits(vBits)
	tmp.writeBits(vBits, 64)

	// Copy timestamp of second sample.
	tDelta, err = binary.ReadUvarint(&br)
	if err != nil {
		// This should never happen, data is coming from memory, not disk.
		panic(err)
	}
	for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
		tmp.writeByte(b)
	}

	if numSamples == 1 {
		return
	}

	t += int64(tDelta)
	// From the second sample onwards there was no ST before recode.
	// Either because there was no ST at all, or because we had constant ST.
	// In any case we need to write the marker that says there is no ST change.
	putSTDiff(&tmp, true, 0 /* dummy */)

	// Copy second sample value.
	vPrev = v
	err = xorRead(&br, &v, &readLeading, &readTrailing)
	if err != nil {
		// This should never happen, data is coming from memory, not disk.
		panic(err)
	}
	xorWrite(&tmp, v, vPrev, &writeLeading, &writeTrailing)

	// Copy remaining samples.
	i := 2
	for {
		// Copy timestamp.
		var dod int64
		dod, err = readClassicVarbitInt(&br)
		if err != nil {
			// This should never happen, data is coming from memory, not disk.
			panic(err)
		}
		putClassicVarbitInt(&tmp, dod)
		tDelta += uint64(dod)
		t += int64(tDelta)

		if i >= numSamples {
			break
		}

		// Put ST diff as zero (no change).
		putSTDiff(&tmp, true, 0 /* dummy */)

		// Copy value.
		vPrev = v
		err = xorRead(&br, &v, &readLeading, &readTrailing)
		if err != nil {
			// This should never happen, data is coming from memory, not disk.
			panic(err)
		}
		xorWrite(&tmp, v, vPrev, &writeLeading, &writeTrailing)
		vPrev = v

		i++
	}
}

func (a *xorRecodingAppender) BitProfiledAppend(p *bitProfiler[any], st, t int64, v float64) {
	a.Append(st, t, v)
}

func (it *xorRecodingIterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal() {
		return ValNone
	}

	if it.numRead == 0 {
		t, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}

		if it.header&HasSTMASK != 0 {
			st, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.st = st
		}

		v, err := it.br.readBits(64)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.t = t
		it.val = math.Float64frombits(v)

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

		if it.header&HasSTChangeMASK != 0 {
			noChange, stDiff, err := readSTDiff(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			if !noChange {
				it.st = it.t - stDiff
			}
		}

		if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
			it.err = err
			return ValNone
		}
		it.numRead++
		return ValFloat
	}

	// From the third sample onwards.
	dod, err := readClassicVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.tDelta = uint64(int64(it.tDelta) + dod)
	it.t += int64(it.tDelta)

	if it.header&HasSTChangeMASK != 0 {
		noChange, stDoDDiff, err := readSTDiff(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		if !noChange {
			// Our value is the delta of tDelta (Dod).
			it.st = it.t + (stDoDDiff - int64(it.tDelta))
		}
	}

	if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
		it.err = err
		return ValNone
	}

	it.numRead++
	return ValFloat
}

// func (it *xorRecodingIterator) dodNext() ValueType {
// 	// Inlined readClassicVarbitInt(&it.br)
// 	var d byte
// 	// read delta-of-delta
// 	for range 4 {
// 		d <<= 1
// 		bit, err := it.br.readBitFast()
// 		if err != nil {
// 			bit, err = it.br.readBit()
// 			if err != nil {
// 				return it.retErr(err)
// 			}
// 		}
// 		if bit == zero {
// 			break
// 		}
// 		d |= 1
// 	}
// 	var sz uint8
// 	var dod int64
// 	switch d {
// 	case 0b0:
// 		// dod == 0
// 	case 0b10:
// 		sz = 14
// 	case 0b110:
// 		sz = 17
// 	case 0b1110:
// 		sz = 20
// 	case 0b1111:
// 		// Do not use fast because it's very unlikely it will succeed.
// 		bits, err := it.br.readBits(64)
// 		if err != nil {
// 			return it.retErr(err)
// 		}

// 		dod = int64(bits)
// 	}

// 	if sz != 0 {
// 		bits, err := it.br.readBitsFast(sz)
// 		if err != nil {
// 			bits, err = it.br.readBits(sz)
// 			if err != nil {
// 				return it.retErr(err)
// 			}
// 		}

// 		// Account for negative numbers, which come back as high unsigned numbers.
// 		// See docs/bstream.md.
// 		if bits > (1 << (sz - 1)) {
// 			bits -= 1 << sz
// 		}
// 		dod = int64(bits)
// 	}

// 	it.tDelta = uint64(int64(it.tDelta) + dod)
// 	it.t += int64(it.tDelta)

// 	// ST.
// 	noChange, stDoDDiff, err := readSTDiff(&it.br)
// 	if err != nil {
// 		return it.retErr(err)
// 	}
// 	if !noChange {
// 		// Our value is the delta of tDelta (Dod).
// 		it.st = it.t + (stDoDDiff - int64(it.tDelta))
// 	}

// 	// Value.
// 	if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
// 		return it.retErr(err)
// 	}

// 	// State EOF check.
// 	it.numRead++
// 	if it.numRead >= it.numTotal {
// 		it.state = eofState
// 	}
// 	return ValFloat
// }

// func (it *xorRecodingIterator) dodNoSTNext() ValueType {
// 	// Inlined readClassicVarbitInt(&it.br)
// 	var d byte
// 	// read delta-of-delta
// 	for range 4 {
// 		d <<= 1
// 		bit, err := it.br.readBitFast()
// 		if err != nil {
// 			bit, err = it.br.readBit()
// 			if err != nil {
// 				return it.retErr(err)
// 			}
// 		}
// 		if bit == zero {
// 			break
// 		}
// 		d |= 1
// 	}
// 	var sz uint8
// 	var dod int64
// 	switch d {
// 	case 0b0:
// 		// dod == 0
// 	case 0b10:
// 		sz = 14
// 	case 0b110:
// 		sz = 17
// 	case 0b1110:
// 		sz = 20
// 	case 0b1111:
// 		// Do not use fast because it's very unlikely it will succeed.
// 		bits, err := it.br.readBits(64)
// 		if err != nil {
// 			return it.retErr(err)
// 		}

// 		dod = int64(bits)
// 	}

// 	if sz != 0 {
// 		bits, err := it.br.readBitsFast(sz)
// 		if err != nil {
// 			bits, err = it.br.readBits(sz)
// 			if err != nil {
// 				return it.retErr(err)
// 			}
// 		}

// 		// Account for negative numbers, which come back as high unsigned numbers.
// 		// See docs/bstream.md.
// 		if bits > (1 << (sz - 1)) {
// 			bits -= 1 << sz
// 		}
// 		dod = int64(bits)
// 	}

// 	it.tDelta = uint64(int64(it.tDelta) + dod)
// 	it.t += int64(it.tDelta)
// 	// Value.
// 	if err := xorRead(&it.br, &it.val, &it.leading, &it.trailing); err != nil {
// 		return it.retErr(err)
// 	}

// 	// State EOF check.
// 	it.numRead++
// 	if it.numRead >= it.numTotal {
// 		it.state = eofState
// 	}
// 	return ValFloat
// }
