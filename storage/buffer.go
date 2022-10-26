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

package storage

import (
	"fmt"
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// BufferedSeriesIterator wraps an iterator with a look-back buffer.
type BufferedSeriesIterator struct {
	it    chunkenc.Iterator
	buf   *sampleRing
	delta int64

	lastTime  int64
	valueType chunkenc.ValueType
}

// NewBuffer returns a new iterator that buffers the values within the time range
// of the current element and the duration of delta before, initialized with an
// empty iterator. Use Reset() to set an actual iterator to be buffered.
func NewBuffer(delta int64) *BufferedSeriesIterator {
	return NewBufferIterator(chunkenc.NewNopIterator(), delta)
}

// NewBufferIterator returns a new iterator that buffers the values within the
// time range of the current element and the duration of delta before.
func NewBufferIterator(it chunkenc.Iterator, delta int64) *BufferedSeriesIterator {
	// TODO(codesome): based on encoding, allocate different buffer.
	bit := &BufferedSeriesIterator{
		buf:   newSampleRing(delta, 16),
		delta: delta,
	}
	bit.Reset(it)

	return bit
}

// Reset re-uses the buffer with a new iterator, resetting the buffered time
// delta to its original value.
func (b *BufferedSeriesIterator) Reset(it chunkenc.Iterator) {
	b.it = it
	b.lastTime = math.MinInt64
	b.buf.reset()
	b.buf.delta = b.delta
	b.valueType = it.Next()
}

// ReduceDelta lowers the buffered time delta, for the current SeriesIterator only.
func (b *BufferedSeriesIterator) ReduceDelta(delta int64) bool {
	return b.buf.reduceDelta(delta)
}

// PeekBack returns the nth previous element of the iterator. If there is none buffered,
// ok is false.
func (b *BufferedSeriesIterator) PeekBack(n int) (t int64, v float64, h *histogram.Histogram, ok bool) {
	s, ok := b.buf.nthLast(n)
	return s.t, s.v, s.h, ok
}

// Buffer returns an iterator over the buffered data. Invalidates previously
// returned iterators.
func (b *BufferedSeriesIterator) Buffer() chunkenc.Iterator {
	return b.buf.iterator()
}

// Seek advances the iterator to the element at time t or greater.
func (b *BufferedSeriesIterator) Seek(t int64) chunkenc.ValueType {
	t0 := t - b.buf.delta

	// If the delta would cause us to seek backwards, preserve the buffer
	// and just continue regular advancement while filling the buffer on the way.
	if b.valueType != chunkenc.ValNone && t0 > b.lastTime {
		b.buf.reset()

		b.valueType = b.it.Seek(t0)
		switch b.valueType {
		case chunkenc.ValNone:
			return chunkenc.ValNone
		case chunkenc.ValFloat:
			b.lastTime, _ = b.At()
		case chunkenc.ValHistogram:
			b.lastTime, _ = b.AtHistogram()
		case chunkenc.ValFloatHistogram:
			b.lastTime, _ = b.AtFloatHistogram()
		default:
			panic(fmt.Errorf("BufferedSeriesIterator: unknown value type %v", b.valueType))
		}
	}

	if b.lastTime >= t {
		return b.valueType
	}
	for {
		if b.valueType = b.Next(); b.valueType == chunkenc.ValNone || b.lastTime >= t {
			return b.valueType
		}
	}
}

// Next advances the iterator to the next element.
func (b *BufferedSeriesIterator) Next() chunkenc.ValueType {
	// Add current element to buffer before advancing.
	switch b.valueType {
	case chunkenc.ValNone:
		return chunkenc.ValNone
	case chunkenc.ValFloat:
		t, v := b.it.At()
		b.buf.add(sample{t: t, v: v})
	case chunkenc.ValHistogram:
		t, h := b.it.AtHistogram()
		b.buf.add(sample{t: t, h: h})
	case chunkenc.ValFloatHistogram:
		t, fh := b.it.AtFloatHistogram()
		b.buf.add(sample{t: t, fh: fh})
	default:
		panic(fmt.Errorf("BufferedSeriesIterator: unknown value type %v", b.valueType))
	}

	b.valueType = b.it.Next()
	if b.valueType != chunkenc.ValNone {
		b.lastTime = b.AtT()
	}
	return b.valueType
}

// At returns the current float element of the iterator.
func (b *BufferedSeriesIterator) At() (int64, float64) {
	return b.it.At()
}

// AtHistogram returns the current histogram element of the iterator.
func (b *BufferedSeriesIterator) AtHistogram() (int64, *histogram.Histogram) {
	return b.it.AtHistogram()
}

// AtFloatHistogram returns the current float-histogram element of the iterator.
func (b *BufferedSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	return b.it.AtFloatHistogram()
}

// AtT returns the current timestamp of the iterator.
func (b *BufferedSeriesIterator) AtT() int64 {
	return b.it.AtT()
}

// Err returns the last encountered error.
func (b *BufferedSeriesIterator) Err() error {
	return b.it.Err()
}

// TODO(beorn7): Consider having different sample types for different value types.
type sample struct {
	t  int64
	v  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func (s sample) T() int64 {
	return s.t
}

func (s sample) V() float64 {
	return s.v
}

func (s sample) H() *histogram.Histogram {
	return s.h
}

func (s sample) FH() *histogram.FloatHistogram {
	return s.fh
}

func (s sample) Type() chunkenc.ValueType {
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

type sampleRing struct {
	delta int64

	buf []sample // lookback buffer
	i   int      // position of most recent element in ring buffer
	f   int      // position of first element in ring buffer
	l   int      // number of elements in buffer

	it sampleRingIterator
}

func newSampleRing(delta int64, sz int) *sampleRing {
	r := &sampleRing{delta: delta, buf: make([]sample, sz)}
	r.reset()

	return r
}

func (r *sampleRing) reset() {
	r.l = 0
	r.i = -1
	r.f = 0
}

// Returns the current iterator. Invalidates previously returned iterators.
func (r *sampleRing) iterator() chunkenc.Iterator {
	r.it.r = r
	r.it.i = -1
	return &r.it
}

type sampleRingIterator struct {
	r *sampleRing
	i int
}

func (it *sampleRingIterator) Next() chunkenc.ValueType {
	it.i++
	if it.i >= it.r.l {
		return chunkenc.ValNone
	}
	s := it.r.at(it.i)
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

func (it *sampleRingIterator) Seek(int64) chunkenc.ValueType {
	return chunkenc.ValNone
}

func (it *sampleRingIterator) Err() error {
	return nil
}

func (it *sampleRingIterator) At() (int64, float64) {
	s := it.r.at(it.i)
	return s.t, s.v
}

func (it *sampleRingIterator) AtHistogram() (int64, *histogram.Histogram) {
	s := it.r.at(it.i)
	return s.t, s.h
}

func (it *sampleRingIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	s := it.r.at(it.i)
	if s.fh == nil {
		return s.t, s.h.ToFloat()
	}
	return s.t, s.fh
}

func (it *sampleRingIterator) AtT() int64 {
	s := it.r.at(it.i)
	return s.t
}

func (r *sampleRing) at(i int) sample {
	j := (r.f + i) % len(r.buf)
	return r.buf[j]
}

// add adds a sample to the ring buffer and frees all samples that fall
// out of the delta range.
func (r *sampleRing) add(s sample) {
	l := len(r.buf)
	// Grow the ring buffer if it fits no more elements.
	if l == r.l {
		buf := make([]sample, 2*l)
		copy(buf[l+r.f:], r.buf[r.f:])
		copy(buf, r.buf[:r.f])

		r.buf = buf
		r.i = r.f
		r.f += l
		l = 2 * l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	r.buf[r.i] = s
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	tmin := s.t - r.delta
	for r.buf[r.f].t < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
}

// reduceDelta lowers the buffered time delta, dropping any samples that are
// out of the new delta range.
func (r *sampleRing) reduceDelta(delta int64) bool {
	if delta > r.delta {
		return false
	}
	r.delta = delta

	if r.l == 0 {
		return true
	}

	// Free head of the buffer of samples that just fell out of the range.
	l := len(r.buf)
	tmin := r.buf[r.i].t - delta
	for r.buf[r.f].t < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
	return true
}

// nthLast returns the nth most recent element added to the ring.
func (r *sampleRing) nthLast(n int) (sample, bool) {
	if n > r.l {
		return sample{}, false
	}
	return r.at(r.l - n), true
}

func (r *sampleRing) samples() []sample {
	res := make([]sample, r.l)

	k := r.f + r.l
	var j int
	if k > len(r.buf) {
		k = len(r.buf)
		j = r.l - k + r.f
	}

	n := copy(res, r.buf[r.f:k])
	copy(res[n:], r.buf[:j])

	return res
}
