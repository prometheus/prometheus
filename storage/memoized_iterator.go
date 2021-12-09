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

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// MemoizedSeriesIterator wraps an iterator with a buffer to look back the previous element.
type MemoizedSeriesIterator struct {
	it    chunkenc.Iterator
	delta int64

	lastTime int64
	ok       bool

	// Keep track of the previously returned value.
	prevTime  int64
	prevValue float64
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
	b.ok = true
	b.prevTime = math.MinInt64
	it.Next()
}

// PeekPrev returns the previous element of the iterator. If there is none buffered,
// ok is false.
func (b *MemoizedSeriesIterator) PeekPrev() (t int64, v float64, ok bool) {
	if b.prevTime == math.MinInt64 {
		return 0, 0, false
	}
	return b.prevTime, b.prevValue, true
}

// Seek advances the iterator to the element at time t or greater.
func (b *MemoizedSeriesIterator) Seek(t int64) bool {
	t0 := t - b.delta

	if b.ok && t0 > b.lastTime {
		// Reset the previously stored element because the seek advanced
		// more than the delta.
		b.prevTime = math.MinInt64

		b.ok = b.it.Seek(t0)
		if !b.ok {
			return false
		}
		b.lastTime, _ = b.it.At()
	}

	if b.lastTime >= t {
		return true
	}
	for b.Next() {
		if b.lastTime >= t {
			return true
		}
	}

	return false
}

// Next advances the iterator to the next element.
func (b *MemoizedSeriesIterator) Next() bool {
	if !b.ok {
		return false
	}

	// Keep track of the previous element.
	b.prevTime, b.prevValue = b.it.At()

	b.ok = b.it.Next()
	if b.ok {
		b.lastTime, _ = b.it.At()
	}

	return b.ok
}

// At returns the current element of the iterator.
func (b *MemoizedSeriesIterator) At() (int64, float64) {
	return b.it.At()
}

// Err returns the last encountered error.
func (b *MemoizedSeriesIterator) Err() error {
	return b.it.Err()
}
