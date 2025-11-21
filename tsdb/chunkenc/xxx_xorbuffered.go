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

	"github.com/fpetkovski/tscodec-go/delta"
	"github.com/fpetkovski/tscodec-go/dod"

	"github.com/prometheus/prometheus/model/histogram"
)

// xorBufferedChunk holds encoded sample data:
// 2B(tsOffset), 2B(valOffset), ?DOD(sts), DOD(ts), XOR(values)
// DOD: 8B(minVal), 2B(numSamples), 1B(bitWidth), dodValues...
type xorBufferedChunk struct {
	b bstream

	sts    []int64
	ts     []int64
	values []float64

	buf   []byte
	stSet bool
}

// NewXORBufferedChunk returns a new chunk with XOR encoding.
func NewXORBufferedChunk() *xorBufferedChunk {
	b := make([]byte, 2*chunkHeaderSize, chunkAllocationSize)
	return &xorBufferedChunk{
		b:      bstream{stream: b, count: 0},
		sts:    make([]int64, 0, 120),
		ts:     make([]int64, 0, 120),
		values: make([]float64, 0, 120),
	}
}

func (c *xorBufferedChunk) Reset(stream []byte) {
	c.b.Reset(stream)
	c.sts = c.sts[:0]
	c.ts = c.ts[:0]
	c.values = c.values[:0]
	c.buf = c.buf[:0]
	c.stSet = false
}

// Encoding returns the encoding type.
func (*xorBufferedChunk) Encoding() Encoding {
	return 134
}

func numSamplesFromBytes(b []byte) int {
	if len(b) == 2*chunkHeaderSize {
		return 0
	}
	return int(binary.LittleEndian.Uint16(b[2*chunkHeaderSize+delta.Int64SizeBytes:]))
}

// Bytes returns the underlying byte slice of the chunk.
func (c *xorBufferedChunk) Bytes() []byte {
	// TODO: Can we assume Bytes is only called once chunk is done?
	if len(c.values) > 0 && numSamplesFromBytes(c.b.bytes()) != len(c.values) {
		c.buf = c.buf[:0]
		if c.stSet {
			c.buf = dod.EncodeInt64(c.buf, c.sts[:])
			for _, b := range c.buf {
				c.b.writeByte(b)
			}
		}
		// tsOffset (from 2*chunkHeaderSize).
		binary.LittleEndian.PutUint16(c.b.bytes(), uint16(len(c.buf)))
		c.buf = dod.EncodeInt64(c.buf[:0], c.ts[:])
		for _, b := range c.buf {
			c.b.writeByte(b)
		}
		// valOffset (from 2*chunkHeaderSize+tsOffset).
		binary.LittleEndian.PutUint16(c.b.bytes()[chunkHeaderSize:], uint16(len(c.buf)))
		encodeXOR(&c.b, c.values)
	}
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *xorBufferedChunk) NumSamples() int {
	return len(c.values)
}

// Compact implements the Chunk interface.
func (c *xorBufferedChunk) Compact() {
	if l := len(c.b.stream); cap(c.b.stream) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b.stream)
		c.b.stream = buf
	}
}

func (c *xorBufferedChunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *xorBufferedChunk) AppenderV2() (AppenderV2, error) {
	return c, nil
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *xorBufferedChunk) Iterator(_ Iterator) Iterator {
	// TODO: This is yolo, ideally iterator is reused, slices are shared across iterations only etc.

	// Lazy decode.
	numSamples := numSamplesFromBytes(c.b.bytes())
	if len(c.values) == 0 && numSamples != len(c.values) {
		tsOffset := binary.LittleEndian.Uint16(c.b.bytes())
		valOffset := binary.LittleEndian.Uint16(c.b.bytes()[chunkHeaderSize:])
		if cap(c.values) < numSamples {
			if tsOffset > 0 {
				// tsOffset != 0 means st no zero.
				c.sts = make([]int64, 0, numSamples)
			}
			c.ts = make([]int64, 0, numSamples)
			c.values = make([]float64, 0, numSamples)
		}
		if tsOffset == 0 {
			c.sts = c.sts[:1]
			c.sts[0] = 0
		} else {
			c.sts = c.sts[:numSamples]
			// TODO: Optimize decode to give one ST for const.
			dod.DecodeInt64(c.sts, c.b.bytes()[2*chunkHeaderSize:])
		}
		c.ts = c.ts[:numSamples]
		dod.DecodeInt64(c.ts, c.b.bytes()[2*chunkHeaderSize+tsOffset:])
		br := newBReader(c.b.bytes()[2*chunkHeaderSize+tsOffset+valOffset:])
		if err := decodeXOR(&br, numSamples, &c.values); err != nil {
			return &xorBufferedtIterator{err: err}
		}
	}
	return &xorBufferedtIterator{c: c, curr: -1}
}

func (c *xorBufferedChunk) Append(st, t int64, v float64) {
	if !c.stSet {
		c.stSet = st != 0
	}
	c.sts = append(c.sts, st)
	c.ts = append(c.ts, t)
	c.values = append(c.values, v)
}

func (*xorBufferedChunk) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*xorBufferedChunk) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type xorBufferedtIterator struct {
	c    *xorBufferedChunk
	curr int

	err error
}

func (it *xorBufferedtIterator) Seek(t int64) ValueType {
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

func (it *xorBufferedtIterator) At() (int64, float64) {
	return it.c.ts[it.curr], it.c.values[it.curr]
}

func (*xorBufferedtIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*xorBufferedtIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *xorBufferedtIterator) AtT() int64 {
	return it.c.ts[it.curr]
}

func (it *xorBufferedtIterator) AtST() int64 {
	if it.curr >= len(it.c.sts) {
		// Const case.
		return it.c.sts[0]
	}
	return it.c.sts[it.curr]
}

func (it *xorBufferedtIterator) Err() error {
	return it.err
}

func (it *xorBufferedtIterator) Next() ValueType {
	if it.curr+1 >= len(it.c.values) {
		return ValNone
	}
	it.curr++
	return ValFloat
}
