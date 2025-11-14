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

// XORV2Opt2Chunk holds encoded sample data:
// 2B(numSamples), 2B(stChangedOnSample), varbitint(st), varbitint(t), xor(v), varbituint(stDelta), varbituint(tDelta), xor(v), varbitint(stDod), varbitint(tDod), xor(v), ...
type XORV2Opt2Chunk struct {
	b bstream
}

// NewXORV2Opt2Chunk returns a new chunk with XORv2 encoding.
func NewXORV2Opt2Chunk() *XORV2Opt2Chunk {
	b := make([]byte, 2*chunkHeaderSize, chunkAllocationSize)
	return &XORV2Opt2Chunk{b: bstream{stream: b, count: 0}}
}

func (c *XORV2Opt2Chunk) Reset(stream []byte) {
	c.b.Reset(stream)
}

// Encoding returns the encoding type.
func (*XORV2Opt2Chunk) Encoding() Encoding {
	return EncXORV2Opt2
}

// Bytes returns the underlying byte slice of the chunk.
func (c *XORV2Opt2Chunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *XORV2Opt2Chunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

// Compact implements the Chunk interface.
func (c *XORV2Opt2Chunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *XORV2Opt2Chunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *XORV2Opt2Chunk) AppenderV2() (AppenderV2, error) {
	if len(c.b.stream) == 2*chunkHeaderSize { // Avoid allocating an Iterator when chunk is empty.
		return &xorV2Opt2Appender{b: &c.b, t: math.MinInt64, leading: 0xff}, nil
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

	a := &xorV2Opt2Appender{
		b:        &c.b,
		st:       it.st,
		t:        it.t,
		v:        it.val,
		stDelta:  it.stDelta,
		tDelta:   it.tDelta,
		leading:  it.leading,
		trailing: it.trailing,
	}
	return a, nil
}

func (c *XORV2Opt2Chunk) iterator(it Iterator) *xorV2Opt2tIterator {
	xorIter, ok := it.(*xorV2Opt2tIterator)
	if !ok {
		xorIter = &xorV2Opt2tIterator{}
	}

	xorIter.Reset(c.b.bytes())
	return xorIter
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *XORV2Opt2Chunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type xorV2Opt2Appender struct {
	b *bstream

	st, t   int64
	v       float64
	stDelta int64
	tDelta  uint64

	leading  uint8
	trailing uint8
}

func (a *xorV2Opt2Appender) Append(st, t int64, v float64) {
	var (
		stDelta int64
		tDelta  uint64
	)
	num := binary.BigEndian.Uint16(a.b.bytes())
	stChangedOnNum := binary.BigEndian.Uint16(a.b.bytes()[chunkHeaderSize:])

	switch num {
	case 0:
		putVarbitInt(a.b, st)
		putVarbitInt(a.b, t)
		a.b.writeBits(math.Float64bits(v), 64)
	case 1:
		stDelta = st - a.st
		if stDelta != 0 {
			stChangedOnNum = num
			putVarbitInt(a.b, stDelta)
		}

		tDelta = uint64(t - a.t)
		putVarbitUint(a.b, tDelta)
		a.writeVDelta(v)
	default:
		stDelta = st - a.st
		sdod := stDelta - a.stDelta
		if stChangedOnNum != 0 {
			putVarbitInt(a.b, sdod)
		} else if sdod != 0 {
			stChangedOnNum = num
			putVarbitInt(a.b, sdod)
		}

		tDelta = uint64(t - a.t)
		dod := int64(tDelta - a.tDelta)
		putVarbitInt(a.b, dod)
		a.writeVDelta(v)
	}

	a.st = st
	a.t = t
	a.v = v
	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
	binary.BigEndian.PutUint16(a.b.bytes()[chunkHeaderSize:], stChangedOnNum)
	a.tDelta = tDelta
	a.stDelta = stDelta
}

func (a *xorV2Opt2Appender) writeVDelta(v float64) {
	xorWrite(a.b, v, a.v, &a.leading, &a.trailing)
}

func (*xorV2Opt2Appender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorV2Opt2Appender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorV2Opt2tIterator struct {
	br                bstreamReader
	numTotal          uint16
	stChangedOnSample uint16
	numRead           uint16

	st, t int64
	val   float64

	leading  uint8
	trailing uint8

	stDelta int64
	tDelta  uint64
	err     error
}

func (it *xorV2Opt2tIterator) Seek(t int64) ValueType {
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

func (it *xorV2Opt2tIterator) At() (int64, float64) {
	return it.t, it.val
}

func (*xorV2Opt2tIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorV2Opt2tIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorV2Opt2tIterator) AtT() int64 {
	return it.t
}

func (it *xorV2Opt2tIterator) AtST() int64 {
	return it.st
}

func (it *xorV2Opt2tIterator) Err() error {
	return it.err
}

func (it *xorV2Opt2tIterator) Reset(b []byte) {
	// 2*chunkHeaderSize is for the first 2*2 bytes contain chunk headers (numSamples, stChangedOnSample).
	// We skip the above for actual samples.
	it.br = newBReader(b[2*chunkHeaderSize:])
	it.numTotal = binary.BigEndian.Uint16(b)
	it.stChangedOnSample = binary.BigEndian.Uint16(b[chunkHeaderSize:])

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

func (it *xorV2Opt2tIterator) Next() ValueType {
	if it.err != nil || it.numRead == it.numTotal {
		return ValNone
	}

	switch it.numRead {
	case 0:
		st, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.st = st

		t, err := readVarbitInt(&it.br)
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
		if it.stChangedOnSample > 0 && it.stChangedOnSample <= it.numRead {
			stDelta, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.stDelta = stDelta
			it.st += it.stDelta
		}
		tDelta, err := readVarbitUint(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}
		it.tDelta = tDelta
		it.t += int64(it.tDelta)

		return it.readValue()
	default:
		if it.stChangedOnSample > 0 && it.stChangedOnSample <= it.numRead {
			sdod, err := readVarbitInt(&it.br)
			if err != nil {
				it.err = err
				return ValNone
			}
			it.stDelta = it.stDelta + sdod
			it.st += it.stDelta
		}

		dod, err := readVarbitInt(&it.br)
		if err != nil {
			it.err = err
			return ValNone
		}

		it.tDelta = uint64(int64(it.tDelta) + dod)
		it.t += int64(it.tDelta)
		return it.readValue()
	}
}

func (it *xorV2Opt2tIterator) readValue() ValueType {
	err := xorRead(&it.br, &it.val, &it.leading, &it.trailing)
	if err != nil {
		it.err = err
		return ValNone
	}
	it.numRead++
	return ValFloat
}
