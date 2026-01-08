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

// xorVarbitTSChunk holds XOR encoded sample data.
// 2B(numSamples), varint(t), xor(v), varuint(tDelta), xor(v), varint(tDod), xor(v), ...
// This yields up to 20% chunk size improvements, but up to 2x write slow down..
type xorVarbitTSChunk struct {
	b bstream
}

// NewXORVarbitTSChunk returns a new chunk with XOR encoding.
func NewXORVarbitTSChunk() *xorVarbitTSChunk {
	b := make([]byte, chunkHeaderSize, chunkAllocationSize)
	return &xorVarbitTSChunk{b: bstream{stream: b, count: 0}}
}

func (c *xorVarbitTSChunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*xorVarbitTSChunk) Encoding() Encoding {
	return 110
}

// Bytes returns the underlying byte slice of the chunk.
func (c *xorVarbitTSChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *xorVarbitTSChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *xorVarbitTSChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

// Appender implements the Chunk interface.
// It is not valid to call Appender() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *xorVarbitTSChunk) Appender() (Appender, error) {
	if len(c.b.stream) == chunkHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorVarbitTSAppender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
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

	a := &xorVarbitTSAppender{
		b:        &c.b,
		num:      binary.BigEndian.Uint16(c.b.bytes()),
		t:        it.t,
		v:        it.val,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,
	}
	return a, nil
}

func (c *xorVarbitTSChunk) iterator(it Iterator) *xorVarintTSIterator {
	if xorIter, ok := it.(*xorVarintTSIterator); ok {
		xorIter.Reset(c.b.bytes())
		return xorIter
	}
	return &xorVarintTSIterator{
		// The first 2 bytes contain chunk headers.
		// We skip that for actual samples.
		br:       newBReader(c.b.bytes()[chunkHeaderSize:]),
		numTotal: binary.BigEndian.Uint16(c.b.bytes()),
		t:        math.MinInt64,
	}
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *xorVarbitTSChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorVarbitTSAppender struct {
	b *bstream

	num uint16

	t      int64
	v      float64
	tDelta uint64

	leading  uint8
	trailing uint8
}

func (a *xorVarbitTSAppender) Append(t int64, v float64) {
	var tDelta uint64
	switch a.num {
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
		putTimestampVarbitInt(a.b, dod)
		a.writeVDelta(v)
	}

	a.t = t
	a.v = v
	a.num++
	binary.BigEndian.PutUint16(a.b.bytes(), a.num)
	a.tDelta = tDelta
}

func (a *xorVarbitTSAppender) BitProfiledAppend(p *bitProfiler[any], _, t int64, v float64) {
	var tDelta uint64
	num := binary.BigEndian.Uint16(a.b.bytes())

	switch num {
	case 0:
		p.Write(a.b, t, "t", func() {
			buf := make([]byte, binary.MaxVarintLen64)
			for _, b := range buf[:binary.PutVarint(buf, t)] {
				a.b.writeByte(b)
			}
		})
		p.Write(a.b, v, "v", func() {
			a.b.writeBits(math.Float64bits(v), 64)
		})
	case 1:
		tDelta = uint64(t - a.t)
		p.Write(a.b, t, "tDelta", func() {
			buf := make([]byte, binary.MaxVarintLen64)
			for _, b := range buf[:binary.PutUvarint(buf, tDelta)] {
				a.b.writeByte(b)
			}
		})
		p.Write(a.b, v, "v", func() {
			a.writeVDelta(v)
		})

	default:
		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		p.Write(a.b, dodSample{t: t, tDelta: tDelta, dod: dod}, "tDod", func() {
			putTimestampVarbitInt(a.b, dod)
		})
		p.Write(a.b, v, "v", func() {
			a.writeVDelta(v)
		})
	}

	a.t = t
	a.v = v
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
	a.tDelta = tDelta
}

func (a *xorVarbitTSAppender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorVarbitTSAppender) AppendHistogram(*HistogramAppender, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorVarbitTSAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorVarintTSIterator struct {
	br       bstreamReader
	numTotal uint16
	numRead  uint16

	t   int64
	val float64

	leading  uint8
	trailing uint8

	tDelta uint64
	err    error
}

func (it *xorVarintTSIterator) Seek(t int64) ValueType {
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

func (it *xorVarintTSIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorVarintTSIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorVarintTSIterator.AtHistogram")
}

func (*xorVarintTSIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorVarintTSIterator.AtFloatHistogram")
}

func (it *xorVarintTSIterator) AtT() int64 {
	return it.t
}

func (it *xorVarintTSIterator) AtST() int64 {
	return 0 // This version of XOR format does not support start timestamp.
}

func (it *xorVarintTSIterator) Err() error {
	return it.err
}

func (it *xorVarintTSIterator) Reset(b []byte) {
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
}

func (it *xorVarintTSIterator) Next() ValueType {
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

	dod, err := readTimestampVarbitInt(&it.br)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.tDelta = uint64(int64(it.tDelta) + dod)
	it.t += int64(it.tDelta)

	return it.readValue()
}

func (it *xorVarintTSIterator) readValue() ValueType {
	err := xorRead(&it.br, &it.val, &it.leading, &it.trailing)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.numRead++
	return ValFloat
}
