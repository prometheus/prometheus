// Copyright 2017 The Prometheus Authors
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
	"math"
	"sync"

	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/model/histogram"
)

// Encoding is the identifier for a chunk encoding.
type Encoding uint8

// The different available chunk encodings.
const (
	EncNone Encoding = iota
	EncXOR
	EncHistogram
	EncFloatHistogram
)

func (e Encoding) String() string {
	switch e {
	case EncNone:
		return "none"
	case EncXOR:
		return "XOR"
	case EncHistogram:
		return "histogram"
	case EncFloatHistogram:
		return "floathistogram"
	}
	return "<unknown>"
}

// Chunk encodings for out-of-order chunks.
// These encodings must be only used by the Head block for its internal bookkeeping.
const (
	OutOfOrderMask = 0b10000000
	EncOOOXOR      = EncXOR | OutOfOrderMask
)

func IsOutOfOrderChunk(e Encoding) bool {
	return (e & OutOfOrderMask) != 0
}

// IsValidEncoding returns true for supported encodings.
func IsValidEncoding(e Encoding) bool {
	return e == EncXOR || e == EncOOOXOR || e == EncHistogram || e == EncFloatHistogram
}

// Chunk holds a sequence of sample pairs that can be iterated over and appended to.
type Chunk interface {
	// Bytes returns the underlying byte slice of the chunk.
	Bytes() []byte

	// Encoding returns the encoding type of the chunk.
	Encoding() Encoding

	// Appender returns an appender to append samples to the chunk.
	Appender() (Appender, error)

	// The iterator passed as argument is for re-use.
	// Depending on implementation, the iterator can
	// be re-used or a new iterator can be allocated.
	Iterator(Iterator) Iterator

	// NumSamples returns the number of samples in the chunk.
	NumSamples() int

	// Compact is called whenever a chunk is expected to be complete (no more
	// samples appended) and the underlying implementation can eventually
	// optimize the chunk.
	// There's no strong guarantee that no samples will be appended once
	// Compact() is called. Implementing this function is optional.
	Compact()
}

// Appender adds sample pairs to a chunk.
type Appender interface {
	Append(int64, float64)
	AppendHistogram(t int64, h *histogram.Histogram)
	AppendFloatHistogram(t int64, h *histogram.FloatHistogram)
}

// Iterator is a simple iterator that can only get the next value.
// Iterator iterates over the samples of a time series, in timestamp-increasing order.
type Iterator interface {
	// Next advances the iterator by one and returns the type of the value
	// at the new position (or ValNone if the iterator is exhausted).
	Next() ValueType
	// Seek advances the iterator forward to the first sample with a
	// timestamp equal or greater than t. If the current sample found by a
	// previous `Next` or `Seek` operation already has this property, Seek
	// has no effect. If a sample has been found, Seek returns the type of
	// its value. Otherwise, it returns ValNone, after with the iterator is
	// exhausted.
	Seek(t int64) ValueType
	// At returns the current timestamp/value pair if the value is a float.
	// Before the iterator has advanced, the behaviour is unspecified.
	At() (int64, float64)
	// AtHistogram returns the current timestamp/value pair if the value is
	// a histogram with integer counts. Before the iterator has advanced,
	// the behaviour is unspecified.
	AtHistogram() (int64, *histogram.Histogram)
	// AtFloatHistogram returns the current timestamp/value pair if the
	// value is a histogram with floating-point counts. It also works if the
	// value is a histogram with integer counts, in which case a
	// FloatHistogram copy of the histogram is returned. Before the iterator
	// has advanced, the behaviour is unspecified.
	AtFloatHistogram() (int64, *histogram.FloatHistogram)
	// AtT returns the current timestamp.
	// Before the iterator has advanced, the behaviour is unspecified.
	AtT() int64
	// Err returns the current error. It should be used only after the
	// iterator is exhausted, i.e. `Next` or `Seek` have returned ValNone.
	Err() error
}

// ValueType defines the type of a value an Iterator points to.
type ValueType uint8

// Possible values for ValueType.
const (
	ValNone           ValueType = iota // No value at the current position.
	ValFloat                           // A simple float, retrieved with At.
	ValHistogram                       // A histogram, retrieve with AtHistogram, but AtFloatHistogram works, too.
	ValFloatHistogram                  // A floating-point histogram, retrieve with AtFloatHistogram.
)

func (v ValueType) String() string {
	switch v {
	case ValNone:
		return "none"
	case ValFloat:
		return "float"
	case ValHistogram:
		return "histogram"
	case ValFloatHistogram:
		return "floathistogram"
	default:
		return "unknown"
	}
}

func (v ValueType) ChunkEncoding() Encoding {
	switch v {
	case ValFloat:
		return EncXOR
	case ValHistogram:
		return EncHistogram
	case ValFloatHistogram:
		return EncFloatHistogram
	default:
		return EncNone
	}
}

// MockSeriesIterator returns an iterator for a mock series with custom timeStamps and values.
func MockSeriesIterator(timestamps []int64, values []float64) Iterator {
	return &mockSeriesIterator{
		timeStamps: timestamps,
		values:     values,
		currIndex:  0,
	}
}

type mockSeriesIterator struct {
	timeStamps []int64
	values     []float64
	currIndex  int
}

func (it *mockSeriesIterator) Seek(int64) ValueType { return ValNone }

func (it *mockSeriesIterator) At() (int64, float64) {
	return it.timeStamps[it.currIndex], it.values[it.currIndex]
}

func (it *mockSeriesIterator) AtHistogram() (int64, *histogram.Histogram) { return math.MinInt64, nil }

func (it *mockSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	return math.MinInt64, nil
}

func (it *mockSeriesIterator) AtT() int64 {
	return it.timeStamps[it.currIndex]
}

func (it *mockSeriesIterator) Next() ValueType {
	if it.currIndex < len(it.timeStamps)-1 {
		it.currIndex++
		return ValFloat
	}

	return ValNone
}
func (it *mockSeriesIterator) Err() error { return nil }

// NewNopIterator returns a new chunk iterator that does not hold any data.
func NewNopIterator() Iterator {
	return nopIterator{}
}

type nopIterator struct{}

func (nopIterator) Next() ValueType                                      { return ValNone }
func (nopIterator) Seek(int64) ValueType                                 { return ValNone }
func (nopIterator) At() (int64, float64)                                 { return math.MinInt64, 0 }
func (nopIterator) AtHistogram() (int64, *histogram.Histogram)           { return math.MinInt64, nil }
func (nopIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) { return math.MinInt64, nil }
func (nopIterator) AtT() int64                                           { return math.MinInt64 }
func (nopIterator) Err() error                                           { return nil }

// Pool is used to create and reuse chunk references to avoid allocations.
type Pool interface {
	Put(Chunk) error
	Get(e Encoding, b []byte) (Chunk, error)
}

// pool is a memory pool of chunk objects.
type pool struct {
	xor            sync.Pool
	histogram      sync.Pool
	floatHistogram sync.Pool
}

// NewPool returns a new pool.
func NewPool() Pool {
	return &pool{
		xor: sync.Pool{
			New: func() interface{} {
				return &XORChunk{b: bstream{}}
			},
		},
		histogram: sync.Pool{
			New: func() interface{} {
				return &HistogramChunk{b: bstream{}}
			},
		},
		floatHistogram: sync.Pool{
			New: func() interface{} {
				return &FloatHistogramChunk{b: bstream{}}
			},
		},
	}
}

func (p *pool) Get(e Encoding, b []byte) (Chunk, error) {
	switch e {
	case EncXOR, EncOOOXOR:
		c := p.xor.Get().(*XORChunk)
		c.b.stream = b
		c.b.count = 0
		return c, nil
	case EncHistogram:
		c := p.histogram.Get().(*HistogramChunk)
		c.b.stream = b
		c.b.count = 0
		return c, nil
	case EncFloatHistogram:
		c := p.floatHistogram.Get().(*FloatHistogramChunk)
		c.b.stream = b
		c.b.count = 0
		return c, nil
	}
	return nil, errors.Errorf("invalid chunk encoding %q", e)
}

func (p *pool) Put(c Chunk) error {
	switch c.Encoding() {
	case EncXOR, EncOOOXOR:
		xc, ok := c.(*XORChunk)
		// This may happen often with wrapped chunks. Nothing we can really do about
		// it but returning an error would cause a lot of allocations again. Thus,
		// we just skip it.
		if !ok {
			return nil
		}
		xc.b.stream = nil
		xc.b.count = 0
		p.xor.Put(c)
	case EncHistogram:
		sh, ok := c.(*HistogramChunk)
		// This may happen often with wrapped chunks. Nothing we can really do about
		// it but returning an error would cause a lot of allocations again. Thus,
		// we just skip it.
		if !ok {
			return nil
		}
		sh.b.stream = nil
		sh.b.count = 0
		p.histogram.Put(c)
	case EncFloatHistogram:
		sh, ok := c.(*FloatHistogramChunk)
		// This may happen often with wrapped chunks. Nothing we can really do about
		// it but returning an error would cause a lot of allocations again. Thus,
		// we just skip it.
		if !ok {
			return nil
		}
		sh.b.stream = nil
		sh.b.count = 0
		p.floatHistogram.Put(c)
	default:
		return errors.Errorf("invalid chunk encoding %q", c.Encoding())
	}
	return nil
}

// FromData returns a chunk from a byte slice of chunk data.
// This is there so that users of the library can easily create chunks from
// bytes.
func FromData(e Encoding, d []byte) (Chunk, error) {
	switch e {
	case EncXOR, EncOOOXOR:
		return &XORChunk{b: bstream{count: 0, stream: d}}, nil
	case EncHistogram:
		return &HistogramChunk{b: bstream{count: 0, stream: d}}, nil
	case EncFloatHistogram:
		return &FloatHistogramChunk{b: bstream{count: 0, stream: d}}, nil
	}
	return nil, errors.Errorf("invalid chunk encoding %q", e)
}

// NewEmptyChunk returns an empty chunk for the given encoding.
func NewEmptyChunk(e Encoding) (Chunk, error) {
	switch e {
	case EncXOR:
		return NewXORChunk(), nil
	case EncHistogram:
		return NewHistogramChunk(), nil
	case EncFloatHistogram:
		return NewFloatHistogramChunk(), nil
	}
	return nil, errors.Errorf("invalid chunk encoding %q", e)
}
