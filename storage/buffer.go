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
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
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
func (b *BufferedSeriesIterator) PeekBack(n int) (sample tsdbutil.Sample, ok bool) {
	return b.buf.nthLast(n)
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
		t, f := b.it.At()
		b.buf.add(fSample{t: t, f: f})
	case chunkenc.ValHistogram:
		t, h := b.it.AtHistogram()
		b.buf.add(hSample{t: t, h: h})
	case chunkenc.ValFloatHistogram:
		t, fh := b.it.AtFloatHistogram()
		b.buf.add(fhSample{t: t, fh: fh})
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

type fSample struct {
	t int64
	f float64
}

func (s fSample) T() int64 {
	return s.t
}

func (s fSample) V() float64 {
	return s.f
}

func (s fSample) H() *histogram.Histogram {
	panic("H() called for fSample")
}

func (s fSample) FH() *histogram.FloatHistogram {
	panic("FH() called for fSample")
}

func (s fSample) Type() chunkenc.ValueType {
	return chunkenc.ValFloat
}

type hSample struct {
	t int64
	h *histogram.Histogram
}

func (s hSample) T() int64 {
	return s.t
}

func (s hSample) V() float64 {
	panic("F() called for hSample")
}

func (s hSample) H() *histogram.Histogram {
	return s.h
}

func (s hSample) FH() *histogram.FloatHistogram {
	return s.h.ToFloat()
}

func (s hSample) Type() chunkenc.ValueType {
	return chunkenc.ValHistogram
}

type fhSample struct {
	t  int64
	fh *histogram.FloatHistogram
}

func (s fhSample) T() int64 {
	return s.t
}

func (s fhSample) V() float64 {
	panic("F() called for fhSample")
}

func (s fhSample) H() *histogram.Histogram {
	panic("H() called for fhSample")
}

func (s fhSample) FH() *histogram.FloatHistogram {
	return s.fh
}

func (s fhSample) Type() chunkenc.ValueType {
	return chunkenc.ValFloatHistogram
}

type sampleRing struct {
	delta int64

	buf []tsdbutil.Sample // lookback buffer
	i   int               // position of most recent element in ring buffer
	f   int               // position of first element in ring buffer
	l   int               // number of elements in buffer

	it sampleRingIterator
}

func newSampleRing(delta int64, sz int) *sampleRing {
	r := &sampleRing{delta: delta, buf: make([]tsdbutil.Sample, sz)}
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
	r  *sampleRing
	i  int
	t  int64
	v  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func (it *sampleRingIterator) Next() chunkenc.ValueType {
	it.i++
	if it.i >= it.r.l {
		return chunkenc.ValNone
	}
	s := it.r.at(it.i)
	it.t = s.T()
	switch s.Type() {
	case chunkenc.ValHistogram:
		it.h = s.H()
		return chunkenc.ValHistogram
	case chunkenc.ValFloatHistogram:
		it.fh = s.FH()
		return chunkenc.ValFloatHistogram
	default:
		it.v = s.V()
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
	return it.t, it.v
}

func (it *sampleRingIterator) AtHistogram() (int64, *histogram.Histogram) {
	return it.t, it.h
}

func (it *sampleRingIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	if it.fh == nil {
		return it.t, it.h.ToFloat()
	}
	return it.t, it.fh
}

func (it *sampleRingIterator) AtT() int64 {
	return it.t
}

func (r *sampleRing) at(i int) tsdbutil.Sample {
	j := (r.f + i) % len(r.buf)
	return r.buf[j]
}

// add adds a sample to the ring buffer and frees all samples that fall
// out of the delta range.
func (r *sampleRing) add(s tsdbutil.Sample) {
	l := len(r.buf)
	// Grow the ring buffer if it fits no more elements.
	if l == r.l {
		buf := make([]tsdbutil.Sample, 2*l)
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
	tmin := s.T() - r.delta
	for r.buf[r.f].T() < tmin {
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
	tmin := r.buf[r.i].T() - delta
	for r.buf[r.f].T() < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
	return true
}

// nthLast returns the nth most recent element added to the ring.
func (r *sampleRing) nthLast(n int) (tsdbutil.Sample, bool) {
	if n > r.l {
		return fSample{}, false
	}
	return r.at(r.l - n), true
}

func (r *sampleRing) samples() []tsdbutil.Sample {
	res := make([]tsdbutil.Sample, r.l)

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
