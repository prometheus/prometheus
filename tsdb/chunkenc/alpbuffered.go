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
	"slices"

	"github.com/fpetkovski/tscodec-go/alp"
	"github.com/fpetkovski/tscodec-go/delta"
	"github.com/fpetkovski/tscodec-go/dod"
	"github.com/parquet-go/bitpack"

	"github.com/prometheus/prometheus/model/histogram"
)

// ALPBufferedChunk holds encoded sample data:
// 2B(numSamples), DOD(sts), DOD(ts), ALP(values)
type ALPBufferedChunk struct {
	b      []byte
	sts    []int64
	ts     []int64
	values []float64
}

// NewALPBufferedChunk returns a new chunk with ALPBuffered encoding.
func NewALPBufferedChunk() *ALPBufferedChunk {
	b := make([]byte, chunkHeaderSize, chunkAllocationSize)
	return &ALPBufferedChunk{b: b, sts: make([]int64, 0, 120), ts: make([]int64, 0, 120), values: make([]float64, 0, 120)}
}

func (c *ALPBufferedChunk) Reset(stream []byte) {
	c.b = stream
	c.sts = c.sts[:0]
	c.ts = c.ts[:0]
	c.values = c.values[:0]
}

// Encoding returns the encoding type.
func (*ALPBufferedChunk) Encoding() Encoding {
	return EncALPBuffered
}

// Bytes returns the underlying byte slice of the chunk.
func (c *ALPBufferedChunk) Bytes() []byte {
	// Do the magic here (lazy).
	// TODO: Can we assume Bytes is only called once chunk is done?
	if len(c.values) > 0 && int(binary.BigEndian.Uint16(c.b)) != len(c.values) {
		binary.BigEndian.PutUint16(c.b, uint16(len(c.values)))
		// TODO: optimization for unchanged sts?
		// API could be better?
		c.b = append(c.b, dod.EncodeInt64(c.b[chunkHeaderSize:], c.sts)...)
		c.b = append(c.b, dod.EncodeInt64(c.b[len(c.b):], c.ts)...)
		c.b = append(c.b, alp.Encode(c.b[len(c.b):], c.values)...)
	}
	return c.b
}

// NumSamples returns the number of samples in the chunk.
func (c *ALPBufferedChunk) NumSamples() int {
	return len(c.values)
}

// Compact implements the Chunk interface.
func (c *ALPBufferedChunk) Compact() {
	if l := len(c.b); cap(c.b) > l+chunkCompactCapacityThreshold {
		buf := make([]byte, l)
		copy(buf, c.b)
		c.b = buf
	}
}

func (c *ALPBufferedChunk) Appender() (Appender, error) {
	a, err := c.AppenderV2()
	return &compactAppender{AppenderV2: a}, err
}

// AppenderV2 implements the Chunk interface.
// It is not valid to call AppenderV2() multiple times concurrently or to use multiple
// Appenders on the same chunk.
func (c *ALPBufferedChunk) AppenderV2() (AppenderV2, error) {
	return c, nil
}

// Iterator implements the Chunk interface.
// Iterator() must not be called concurrently with any modifications to the chunk,
// but after it returns you can use an Iterator concurrently with an Appender or
// other Iterators.
func (c *ALPBufferedChunk) Iterator(_ Iterator) Iterator {
	// Lazy decode.
	if samples := int(binary.BigEndian.Uint16(c.b)); len(c.values) == 0 && samples != len(c.values) {
		if samples != len(c.values) {
			if cap(c.values) < samples {
				c.sts = make([]int64, samples)
				c.ts = make([]int64, samples)
				c.values = make([]float64, samples)
			} else {
				c.sts = c.sts[:samples]
				c.ts = c.ts[:samples]
				c.values = c.values[:samples]
			}
		}
		// API is not ready for this 3x glue, but let's hack it through.
		dod.DecodeInt64(c.sts, c.b[chunkHeaderSize:])
		header := delta.DecodeHeader(c.b[chunkHeaderSize:])
		offset := chunkHeaderSize + delta.HeaderSize + delta.Int64SizeBytes + bitpack.ByteCount(uint(header.BitWidth)*uint(len(c.sts[1:header.NumValues]))+8*bitpack.PaddingInt64)
		dod.DecodeInt64(c.ts, c.b[offset:])
		header = delta.DecodeHeader(c.b[offset:])
		offset += delta.HeaderSize + delta.Int64SizeBytes + bitpack.ByteCount(uint(header.BitWidth)*uint(len(c.ts[1:header.NumValues]))+8*bitpack.PaddingInt64)
		// In some cases it over-reads (and panics on bitpack.Unpack) super weird.
		c.b = slices.Grow(c.b, 422) // YOLO just for our case to pass...
		alp.Decode(c.values, c.b[offset:])
	}

	return &alpBufferedtIterator{c: c, curr: -1}
}

func (c *ALPBufferedChunk) Append(st, t int64, v float64) {
	c.sts = append(c.sts, st)
	c.ts = append(c.ts, t)
	c.values = append(c.values, v)
}

func (*ALPBufferedChunk) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a float chunk")
}

func (*ALPBufferedChunk) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a float chunk")
}

type alpBufferedtIterator struct {
	c    *ALPBufferedChunk
	curr int
}

func (it *alpBufferedtIterator) Seek(t int64) ValueType {
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

func (it *alpBufferedtIterator) At() (int64, float64) {
	return it.c.ts[it.curr], it.c.values[it.curr]
}

func (*alpBufferedtIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call xorIterator.AtHistogram")
}

func (*alpBufferedtIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call xorIterator.AtFloatHistogram")
}

func (it *alpBufferedtIterator) AtT() int64 {
	return it.c.ts[it.curr]
}

func (it *alpBufferedtIterator) AtST() int64 {
	return it.c.sts[it.curr]
}

func (it *alpBufferedtIterator) Err() error {
	return nil
}

func (it *alpBufferedtIterator) Next() ValueType {
	if it.curr+1 >= len(it.c.values) {
		return ValNone
	}
	it.curr++
	return ValFloat
}
