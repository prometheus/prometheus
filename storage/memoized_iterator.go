// Copyright 2021 The Prometheus Authors
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

package storage

import (
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// ValueType defines the type of a value in the storage.
type ValueType int

const (
	ValNone ValueType = iota
	ValFloat
	ValHistogram
)

// MemoizedSeriesIterator wraps an iterator with a buffer to look back the previous element.
type MemoizedSeriesIterator struct {
	it    chunkenc.Iterator
	delta int64

	lastTime  int64
	valueType ValueType

	// Keep track of the previously returned value.
	prevTime      int64
	prevValue     float64
	prevHistogram *histogram.Histogram
}

// NewMemoizedEmptyIterator is like NewMemoizedIterator but it's initialised with an empty iterator.
func NewMemoizedEmptyIterator(delta int64) *MemoizedSeriesIterator {
	return NewMemoizedIterator(chunkenc.NewNopIterator(), delta)
}

// NewMemoizedIterator returns a new iterator that buffers the values within the
// time range of the current element and the duration of delta before.
func NewMemoizedIterator(it chunkenc.Iterator, delta int64) *MemoizedSeriesIterator {
	bit := &MemoizedSeriesIterator{
		delta:    delta,
		prevTime: math.MinInt64,
	}
	bit.Reset(it)

	return bit
}

// Reset the internal state to reuse the wrapper with the provided iterator.
func (b *MemoizedSeriesIterator) Reset(it chunkenc.Iterator) {
	b.it = it
	b.lastTime = math.MinInt64
	b.prevTime = math.MinInt64
	it.Next()
	if it.ChunkEncoding() == chunkenc.EncHistogram {
		b.valueType = ValHistogram
	} else {
		b.valueType = ValFloat
	}
}

// PeekPrev returns the previous element of the iterator. If there is none buffered,
// ok is false.
func (b *MemoizedSeriesIterator) PeekPrev() (t int64, v float64, h *histogram.Histogram, ok bool) {
	if b.prevTime == math.MinInt64 {
		return 0, 0, nil, false
	}
	return b.prevTime, b.prevValue, b.prevHistogram, true
}

// Seek advances the iterator to the element at time t or greater.
func (b *MemoizedSeriesIterator) Seek(t int64) ValueType {
	t0 := t - b.delta

	if t0 > b.lastTime {
		// Reset the previously stored element because the seek advanced
		// more than the delta.
		b.prevTime = math.MinInt64

		ok := b.it.Seek(t0)
		if !ok {
			b.valueType = ValNone
			return ValNone
		}
		if b.it.ChunkEncoding() == chunkenc.EncHistogram {
			b.valueType = ValHistogram
			b.lastTime, _ = b.it.AtHistogram()
		} else {
			b.valueType = ValFloat
			b.lastTime, _ = b.it.At()
		}
	}

	if b.lastTime >= t {
		return b.valueType
	}
	for b.Next() != ValNone {
		if b.lastTime >= t {
			return b.valueType
		}
	}

	return ValNone
}

// Next advances the iterator to the next element.
func (b *MemoizedSeriesIterator) Next() ValueType {
	if b.valueType == ValNone {
		return ValNone
	}

	// Keep track of the previous element.
	if b.it.ChunkEncoding() == chunkenc.EncHistogram {
		b.prevTime, b.prevHistogram = b.it.AtHistogram()
		b.prevValue = 0
	} else {
		b.prevTime, b.prevValue = b.it.At()
		b.prevHistogram = nil
	}

	ok := b.it.Next()
	if ok {
		if b.it.ChunkEncoding() == chunkenc.EncHistogram {
			b.lastTime, _ = b.it.AtHistogram()
			b.valueType = ValHistogram

		} else {
			b.lastTime, _ = b.it.At()
			b.valueType = ValFloat
		}
	} else {
		b.valueType = ValNone
	}
	return b.valueType
}

// Values returns the current element of the iterator.
func (b *MemoizedSeriesIterator) Values() (int64, float64) {
	return b.it.At()
}

// Values returns the current element of the iterator.
func (b *MemoizedSeriesIterator) HistogramValues() (int64, *histogram.Histogram) {
	return b.it.AtHistogram()
}

// Err returns the last encountered error.
func (b *MemoizedSeriesIterator) Err() error {
	return b.it.Err()
}
