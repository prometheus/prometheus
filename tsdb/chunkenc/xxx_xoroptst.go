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

const chunkSTHeaderSize = 1

// xorOptSTChunk holds encoded sample data:
// 2B(numSamples), 1B(stHeader), ?varint(st), varint(t), xor(v), ?varuint(stDelta), varuint(tDelta), xor(v), ?classicvarbitint(stDod), classicvarbitint(tDod), xor(v), ...
// stHeader: 1b(nonZeroFirstST), 7b(stSampleUntil)
type xorOptSTChunk struct {
	b bstream
}

// NewXOROptSTChunk returns a new chunk with XORv2 encoding.
func NewXOROptSTChunk() *xorOptSTChunk {
	b := make([]byte, chunkHeaderSize+chunkSTHeaderSize, chunkAllocationSize)
	return &xorOptSTChunk{b: bstream{stream: b, count: 0}}
}

func (c *xorOptSTChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*xorOptSTChunk) Encoding() Encoding {
	return EncXOROptST
}

// Bytes returns the underlying byte slice of the chunk.
func (c *xorOptSTChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *xorOptSTChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *xorOptSTChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *xorOptSTChunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *xorOptSTChunk) AppenderV2() (AppenderV2, error) {
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

		numTotal:                 it.numTotal,
		stZeroInitially:          it.stZeroInitially,
		stSameUntil:              it.stSameUntil,
		stChangeTrackingDisabled: it.stSameUntil != it.numTotal,
	}
	return a, nil
}

func (c *xorOptSTChunk) iterator(it Iterator) *xorOptSTtIterator {
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
func (c *xorOptSTChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorOptSTAppender struct {
	b        *bstream
	numTotal uint16

	// stZeroInitially if true, indicates that the first ST sample was zero
	// and there is no first ST value encoded.
	stZeroInitially bool
	// stSameUntil is a sample number (counting from 0) when ST changed over the
	// first ST appended value.
	// This means that reader should read first sample and then start reading
	// STs only from stSameUntil sample onwards.
	stSameUntil              uint16
	stChangeTrackingDisabled bool

	st, t   int64
	v       float64
	stDelta int64
	tDelta  uint64

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
	numTotal uint16

	stZeroInitially bool
	stSameUntil     uint16

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
	it.numTotal = binary.BigEndian.Uint16(b)
	it.stZeroInitially, it.stSameUntil = readSTHeader(b[chunkHeaderSize:], it.numTotal)
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

func (a *xorOptSTAppender) Append(st, t int64, v float64) {
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
		} else {
			a.stZeroInitially = true
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
		stDelta = st - a.st
		sdod := stDelta - a.stDelta
		if sdod != 0 || a.stChangeTrackingDisabled {
			stChanged = true
			putClassicVarbitInt(a.b, sdod)
		}

		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		putClassicVarbitInt(a.b, dod)
		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	a.tDelta = tDelta
	a.stDelta = stDelta

	a.numTotal++
	binary.BigEndian.PutUint16(a.b.bytes(), a.numTotal)

	// Bump stSameUntil if we see continuously unchanged ST over all samples so far.
	if !a.stChangeTrackingDisabled && !stChanged {
		a.stSameUntil++
	}
	a.stChangeTrackingDisabled = !updateSTHeader(a.b.bytes()[chunkHeaderSize:], a.stZeroInitially, a.stSameUntil, a.numTotal)
}

func (a *xorOptSTAppender) BitProfiledAppend(p *bitProfiler[any], st, t int64, v float64) {
	var (
		stDelta int64
		tDelta  uint64
	)
	// TODO update!!!
	num := binary.BigEndian.Uint16(a.b.bytes())
	stChangedOnNum := binary.BigEndian.Uint16(a.b.bytes()[chunkHeaderSize:])

	switch num {
	case 0:
		buf := make([]byte, binary.MaxVarintLen64)
		p.Write(a.b, t, "st", func() {
			for _, b := range buf[:binary.PutVarint(buf, st)] {
				a.b.writeByte(b)
			}
		})
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
		p.Write(a.b, t, "stDelta", func() {
			if stDelta != 0 {
				stChangedOnNum = num
				for _, b := range buf[:binary.PutVarint(buf, stDelta)] {
					a.b.writeByte(b)
				}
			}
		})

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
		stDelta = st - a.st
		sdod := stDelta - a.stDelta
		if stChangedOnNum != 0 {
			p.Write(a.b, dodSample{t: t, tDelta: tDelta, dod: sdod}, "tDod", func() {
				putClassicVarbitInt(a.b, sdod)
			})
		} else if sdod != 0 {
			stChangedOnNum = num
			p.Write(a.b, dodSample{t: t, tDelta: tDelta, dod: sdod}, "tDod", func() {
				putClassicVarbitInt(a.b, sdod)
			})
		}

		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		p.Write(a.b, dodSample{t: t, tDelta: tDelta, dod: dod}, "tDod", func() {
			putClassicVarbitInt(a.b, dod)
		})
		p.Write(a.b, v, "v", func() {
			a.writeVDelta(v)
		})
	}
	a.st = st
	a.t = t
	a.v = v
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
	binary.BigEndian.PutUint16(a.b.bytes()[chunkHeaderSize:], stChangedOnNum)
	a.tDelta = tDelta
	a.stDelta = stDelta
}

func (it *xorOptSTtIterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	switch it.numRead {
	case 0:
		if !it.stZeroInitially {
			st, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.st = st
		}
	case 1:
		if it.stSameUntil <= it.numRead {
			stDelta, err := binary.ReadVarint(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.stDelta = stDelta
			it.st += it.stDelta
		}
	default:
		if it.stSameUntil <= it.numRead {
			sdod, err := readClassicVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.stDelta = it.stDelta + sdod
			it.st += it.stDelta
		}
	}
	return it.stAgnosticNext()
}

func (it *xorOptSTtIterator) stAgnosticNext() ValueType {
	switch it.numRead {
	case 0:
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

		it.numRead++
		return ValFloat
	case 1:
		tDelta, err := binary.ReadUvarint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)
		return it.readValue()
	default:
		dod, err := readClassicVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}

		it.tDelta = uint64(int64(it.tDelta) + dod)
		it.t += int64(it.tDelta)
		return it.readValue()
	}
}

// NOTE: A lot of info we can pack within 1 byte. We can:
// * we can project stSameUntil input into 63 number (e.g. stSameUntil*63/numTotal)
// * we can project stSameUntil input into 127 number (e.g. stSameUntil*127/numTotal) which would fit exact samples.
// * we could use 6 bits to indicate where ST changes, when it's worth to write (and read it). For 120 samples it means 20 sample chunks.
// updateSTHeader updates one byte ST header with stZeroInitially and stSameUntil data.
// numTotal is used to encode a special case of a chunk having a single ST value (so far).
//
// updateSTHeader returns true is returned if it's ok to skip ST tracking until next changed value. False means encoders have to start tracking
// all ST DoD values.
//
// NOTE: Given 1B constrain we can only fit 127 samples before we don't have space for tracking stSameUntil.
// For those cases encoders and decoders would have to start tracking ST even if they don't change for the rest
// of a chunk. This is fine as typically we see 120 chunk samples.
func updateSTHeader(b []byte, stZeroInitially bool, stSameUntil, numTotal uint16) (ok bool) {
	// First bit indicates initial ST value. 0 bit means 0 ST, otherwise non-zero (need to read first varint).
	b[0] = 0x00
	if !stZeroInitially {
		b[0] = 0x80
	}

	if stSameUntil > 0x7F {
		// This should never happen, would cause corruption (ST already skipped but shouldn't).
		return false
	}

	if stSameUntil >= numTotal {
		// Fast path for the fully unchanged chunk.
		// 0000 0000 for noST.
		// 1000 0000 for single ST value for the whole chunk.
		return stSameUntil != 0x7F
	}

	rest := uint8(stSameUntil)
	if rest == 0 {
		// stSameUntil == 0 makes no sense, but we treat it as stSameUntil == 1
		rest = 1
	}
	b[0] |= rest
	return false
}

func readSTHeader(b []byte, numTotal uint16) (stZeroInitially bool, stSameUntil uint16) {
	if b[0] == 0x00 {
		// Maybe easier without numTotal?
		return true, numTotal
	}
	if b[0] == 0x80 {
		return false, numTotal
	}
	if b[0] == 0x7F {
		return true, 127
	}
	if b[0] == 0xFF {
		return false, 127
	}

	mask := byte(0x80)
	if b[0]&mask == 0 {
		stZeroInitially = true
	}

	mask = 0x7F
	return stZeroInitially, uint16(b[0] & mask)
}

func (it *xorOptSTtIterator) readValue() ValueType {
	err := xorRead(&it.br, &it.val, &it.leading, &it.trailing)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.numRead++
	return ValFloat
}
