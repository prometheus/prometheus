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
	maxConstSampleLimit = 0x7FFF
)

func writeMixedHeaderSTNotConst(b []byte) {
	b[0] = (b[0] & 0x7F) | 0x80
}

func writeMixedHeaderSampleNum(b []byte, numSamples uint16) {
	_ = b[1]

	b[0] = (b[0] & 0x80) | byte(numSamples&0x7f)
	b[1] = byte(numSamples >> 7)
}

func readMixedHeader(b []byte) (isSTConst bool, numSamples uint16) {
	_ = b[1]

	mask := byte(0x80)
	if b[0]&mask == 0 {
		isSTConst = true
	}
	return isSTConst, uint16(b[0]&0x7f) | uint16(b[1])<<7
}

// xorClassicBufferedChunk holds encoded sample data:
// 2B(mixedHeader), DOD(sts), DOD(ts), XOR(values)
// mixedHeader: 1b(isSTConstant), 15b(numSamples)
type xorClassicBufferedChunk struct {
	b bstream

	sts    []int64
	ts     []int64
	values []float64
}

// NewXORClassicBufferedChunk returns a new chunk with XOR encoding.
func NewXORClassicBufferedChunk() *xorClassicBufferedChunk {
	b := make([]byte, chunkHeaderSize, chunkAllocationSize)
	return &xorClassicBufferedChunk{
		b:      bstream{stream: b, count: 0},
		sts:    make([]int64, 0, 120),
		ts:     make([]int64, 0, 120),
		values: make([]float64, 0, 120),
	}
}

func (c *xorClassicBufferedChunk) Reset(stream []byte) {
	c.b.Reset(stream)
	c.sts = c.sts[:0]
	c.ts = c.ts[:0]
	c.values = c.values[:0]
}

// Encoding returns the encoding type.
func (*xorClassicBufferedChunk) Encoding() Encoding {
	return 133
}

// Bytes returns the underlying byte slice of the chunk.
func (c *xorClassicBufferedChunk) Bytes() []byte {
	_, numSamples := readMixedHeader(c.b.bytes())

	// TODO: Can we assume Bytes is only called once chunk is done?
	if len(c.values) > 0 && int(numSamples) != len(c.values) {
		// Encode.
		writeMixedHeaderSampleNum(c.b.bytes(), uint16(len(c.values)))
		// TOOD: We could prealloc much better here!
		var (
			stNotConst bool
			prev       = c.sts[0]
		)
		// TODO: This takes extra time, we can move it to append.
		for _, st := range c.sts[1:] {
			if prev != st {
				stNotConst = true
				break
			}
			prev = st
		}
		// TODO: Space explodes for random v, there is some bug?
		if stNotConst {
			writeMixedHeaderSTNotConst(c.b.bytes())
			encodeDoD(&c.b, c.sts)
		} else {
			// Write only one value.
			buf := make([]byte, binary.MaxVarintLen64)
			for _, b := range buf[:binary.PutVarint(buf, c.sts[0])] {
				c.b.writeByte(b)
			}
		}
		encodeDoD(&c.b, c.ts)
		encodeXOR(&c.b, c.values)
	}
	return c.b.bytes()
}

func encodeDoD(bs *bstream, ts []int64) {
	// 0
	prev := ts[0]
	buf := make([]byte, binary.MaxVarintLen64)
	for _, b := range buf[:binary.PutVarint(buf, prev)] {
		bs.writeByte(b)
	}
	// 1
	if len(ts) == 1 {
		return
	}
	// TODO: For timestamp (this for both) this could be uvarint, optimize later if needed..
	prevDelta := ts[1] - prev
	prev = ts[1]
	for _, b := range buf[:binary.PutVarint(buf, prevDelta)] {
		bs.writeByte(b)
	}
	if len(ts) == 2 {
		return
	}
	// 2
	// TODO: So much more could be optimized here for STs (e.g. new DoD, taking TS as diff..)
	for _, ts := range ts[2:] {
		delta := ts - prev
		dod := delta - prevDelta
		putClassicVarbitInt(bs, dod)

		prev = ts
		prevDelta = delta
	}
}

func encodeXOR(bs *bstream, values []float64) {
	// 0
	prev := values[0]
	bs.writeBits(math.Float64bits(prev), 64)
	// 1
	var leading, trailing uint8
	for _, v := range values[1:] {
		xorWrite(bs, v, prev, &leading, &trailing)
		prev = v
	}
}

// NumSamples returns the number of samples in the chunk.
func (c *xorClassicBufferedChunk) NumSamples() int {
	return len(c.values)
}

// Compact implements the Chunk interface.
func (c *xorClassicBufferedChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *xorClassicBufferedChunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *xorClassicBufferedChunk) AppenderV2() (AppenderV2, error) {
	return c, nil
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *xorClassicBufferedChunk) Iterator(_ Iterator) Iterator {
	// TODO: This is yolo, ideally iterator is reused, slices are shared across iterations only etc.

	// Lazy decode.
	isSTConst, numSamples := readMixedHeader(c.b.bytes())
	if len(c.values) == 0 && int(numSamples) != len(c.values) {
		if cap(c.values) < int(numSamples) {
			c.sts = make([]int64, 0, int(numSamples))
			c.ts = make([]int64, 0, int(numSamples))
			c.values = make([]float64, 0, int(numSamples))
		}

		br := newBReader(c.b.bytes()[chunkHeaderSize:])
		if isSTConst {
			st, err := binary.ReadVarint(&br)
			if err != nil {
				return &xorClassicBufferedtIterator{err: err}
			}
			c.sts = append(c.sts, st)
		} else {
			if err := decodeDoD(&br, int(numSamples), &c.sts); err != nil {
				return &xorClassicBufferedtIterator{err: err}
			}
		}
		if err := decodeDoD(&br, int(numSamples), &c.ts); err != nil {
			return &xorClassicBufferedtIterator{err: err}
		}
		if err := decodeXOR(&br, int(numSamples), &c.values); err != nil {
			return &xorClassicBufferedtIterator{err: err}
		}
	}
	return &xorClassicBufferedtIterator{c: c, curr: -1}
}

func decodeDoD(br *bstreamReader, numSamples int, ts *[]int64) error {
	// 0
	curr, err := binary.ReadVarint(br)
	if err != nil {
		return err
	}
	*ts = append(*ts, curr)
	if len(*ts) == numSamples {
		return nil
	}
	// 1
	currDelta, err := binary.ReadVarint(br)
	if err != nil {
		return err
	}
	curr += currDelta
	*ts = append(*ts, curr)
	// 2
	for len(*ts) < numSamples {
		dod, err := readClassicVarbitInt(br)
		if err != nil {
			return err
		}
		currDelta += dod
		curr += currDelta
		*ts = append(*ts, curr)
	}
	return nil
}

func decodeXOR(br *bstreamReader, numSamples int, values *[]float64) error {
	// 0
	v, err := br.readBits(64)
	if err != nil {
		return err
	}
	curr := math.Float64frombits(v)
	*values = append(*values, curr)
	if len(*values) == numSamples {
		return nil
	}
	// 1
	var leading, trailing uint8
	for len(*values) < numSamples {

		err := xorRead(br, &curr, &leading, &trailing)
		if err != nil {
			return err
		}
		*values = append(*values, curr)
	}
	return nil
}

func (c *xorClassicBufferedChunk) Append(st, t int64, v float64) {
	c.sts = append(c.sts, st)
	c.ts = append(c.ts, t)
	c.values = append(c.values, v)
}

func (*xorClassicBufferedChunk) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorClassicBufferedChunk) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorClassicBufferedtIterator struct {
	c    *xorClassicBufferedChunk
	curr int

	err error
}

func (it *xorClassicBufferedtIterator) Seek(t int64) ValueType {
	if it.curr >= len(it.c.values) {
		return ValNone
	}

	for it.curr == -1 || t > it.c.ts[it.curr] {
		if it.Next() == ValNone {
			return ValNone
		}
	}
	return ValFloat
}

func (it *xorClassicBufferedtIterator) At() (int64, float64) {
	return it.c.ts[it.curr], it.c.values[it.curr]
}

func (*xorClassicBufferedtIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorClassicBufferedtIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorClassicBufferedtIterator) AtT() int64 {
	return it.c.ts[it.curr]
}

func (it *xorClassicBufferedtIterator) AtST() int64 {
	if it.curr >= len(it.c.sts) {
		// Const case.
		return it.c.sts[0]
	}
	return it.c.sts[it.curr]
}

func (it *xorClassicBufferedtIterator) Err() error {
	return it.err
}

func (it *xorClassicBufferedtIterator) Next() ValueType {
	if it.curr+1 >= len(it.c.values) {
		return ValNone
	}
	it.curr++
	return ValFloat
}
