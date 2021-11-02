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
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// BufferedSeriesIterator wraps an iterator with a look-back buffer.
type BufferedSeriesIterator struct {
	it    chunkenc.Iterator
	buf   *sampleRing
	delta int64

	lastTime int64
	ok       bool
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
		buf:   newSampleRing(delta, 16, it.ChunkEncoding()),
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
	b.ok = true
	b.buf.reset()
	b.buf.delta = b.delta
	it.Next()
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
func (b *BufferedSeriesIterator) Seek(t int64) bool {
	t0 := t - b.buf.delta

	// If the delta would cause us to seek backwards, preserve the buffer
	// and just continue regular advancement while filling the buffer on the way.
	if t0 > b.lastTime {
		b.buf.reset()

		b.ok = b.it.Seek(t0)
		if !b.ok {
			return false
		}
		if b.it.ChunkEncoding() == chunkenc.EncHistogram {
			b.lastTime, _ = b.HistogramValues()
		} else {
			b.lastTime, _ = b.Values()
		}
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
func (b *BufferedSeriesIterator) Next() bool {
	if !b.ok {
		return false
	}

	// Add current element to buffer before advancing.
	if b.it.ChunkEncoding() == chunkenc.EncHistogram {
		t, h := b.it.AtHistogram()
		b.buf.add(sample{t: t, h: &h})
	} else {
		t, v := b.it.At()
		b.buf.add(sample{t: t, v: v})
	}

	b.ok = b.it.Next()
	if b.ok {
		if b.it.ChunkEncoding() == chunkenc.EncHistogram {
			b.lastTime, _ = b.HistogramValues()
		} else {
			b.lastTime, _ = b.Values()
		}
	}

	return b.ok
}

// Values returns the current element of the iterator.
func (b *BufferedSeriesIterator) Values() (int64, float64) {
	return b.it.At()
}

// HistogramValues returns the current histogram element of the iterator.
func (b *BufferedSeriesIterator) HistogramValues() (int64, histogram.Histogram) {
	return b.it.AtHistogram()
}

// ChunkEncoding return the chunk encoding of the underlying iterator.
func (b *BufferedSeriesIterator) ChunkEncoding() chunkenc.Encoding {
	return b.it.ChunkEncoding()
}

// Err returns the last encountered error.
func (b *BufferedSeriesIterator) Err() error {
	return b.it.Err()
}

type sample struct {
	t int64
	v float64
	h *histogram.Histogram
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

type sampleRing struct {
	delta int64

	enc chunkenc.Encoding
	buf []sample // lookback buffer
	i   int      // position of most recent element in ring buffer
	f   int      // position of first element in ring buffer
	l   int      // number of elements in buffer

	it sampleRingIterator
}

func newSampleRing(delta int64, sz int, enc chunkenc.Encoding) *sampleRing {
	r := &sampleRing{delta: delta, buf: make([]sample, sz), enc: enc}
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

func (it *sampleRingIterator) Next() bool {
	it.i++
	return it.i < it.r.l
}

func (it *sampleRingIterator) Seek(int64) bool {
	return false
}

func (it *sampleRingIterator) Err() error {
	return nil
}

func (it *sampleRingIterator) At() (int64, float64) {
	return it.r.at(it.i)
}

// AtHistogram always returns (0, histogram.Histogram{}) because there is no
// support for histogram values yet.
func (it *sampleRingIterator) AtHistogram() (int64, histogram.Histogram) {
	return it.r.atHistogram(it.i)
}

func (it *sampleRingIterator) ChunkEncoding() chunkenc.Encoding {
	return it.r.enc
}

func (r *sampleRing) at(i int) (int64, float64) {
	j := (r.f + i) % len(r.buf)
	s := r.buf[j]
	return s.t, s.v
}

func (r *sampleRing) atHistogram(i int) (int64, histogram.Histogram) {
	j := (r.f + i) % len(r.buf)
	s := r.buf[j]
	return s.t, *s.h
}

func (r *sampleRing) atSample(i int) sample {
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
	return r.atSample(r.l - n), true
}

func (r *sampleRing) samples() []sample {
	res := make([]sample, r.l)

	var k = r.f + r.l
	var j int
	if k > len(r.buf) {
		k = len(r.buf)
		j = r.l - k + r.f
	}

	n := copy(res, r.buf[r.f:k])
	copy(res[n:], r.buf[:j])

	return res
}
