// Copyright 2018 The Prometheus Authors
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

package tsdbutil

import (
	"fmt"
	"math"

	"github.com/pkg/errors"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// BufferedSeriesIterator wraps an iterator with a look-back buffer.
//
// TODO(beorn7): BufferedSeriesIterator does not support Histograms or
// FloatHistograms. Either add support or remove BufferedSeriesIterator
// altogether (it seems unused).
type BufferedSeriesIterator struct {
	it  chunkenc.Iterator
	buf *sampleRing

	lastTime int64
}

// NewBuffer returns a new iterator that buffers the values within the time range
// of the current element and the duration of delta before.
func NewBuffer(it chunkenc.Iterator, delta int64) *BufferedSeriesIterator {
	return &BufferedSeriesIterator{
		it:       it,
		buf:      newSampleRing(delta, 16),
		lastTime: math.MinInt64,
	}
}

// PeekBack returns the previous element of the iterator. If there is none buffered,
// ok is false.
func (b *BufferedSeriesIterator) PeekBack() (t int64, v float64, ok bool) {
	return b.buf.last()
}

// Buffer returns an iterator over the buffered data.
func (b *BufferedSeriesIterator) Buffer() chunkenc.Iterator {
	return b.buf.iterator()
}

// Seek advances the iterator to the element at time t or greater.
func (b *BufferedSeriesIterator) Seek(t int64) chunkenc.ValueType {
	t0 := t - b.buf.delta

	// If the delta would cause us to seek backwards, preserve the buffer
	// and just continue regular advancement while filling the buffer on the way.
	if t0 > b.lastTime {
		b.buf.reset()

		if b.it.Seek(t0) == chunkenc.ValNone {
			return chunkenc.ValNone
		}
		b.lastTime = b.AtT()
	}

	if b.lastTime >= t {
		return chunkenc.ValFloat
	}
	for {
		valueType := b.Next()
		switch valueType {
		case chunkenc.ValNone:
			return chunkenc.ValNone
		case chunkenc.ValFloat:
			if b.lastTime >= t {
				return valueType
			}
		default:
			panic(fmt.Errorf("BufferedSeriesIterator: unsupported value type %v", valueType))
		}
		if b.lastTime >= t {
			return valueType
		}
	}
}

// Next advances the iterator to the next element.
func (b *BufferedSeriesIterator) Next() chunkenc.ValueType {
	// Add current element to buffer before advancing.
	b.buf.add(b.it.At())

	valueType := b.it.Next()
	if valueType != chunkenc.ValNone {
		b.lastTime = b.AtT()
	}
	return valueType
}

// At returns the current element of the iterator.
func (b *BufferedSeriesIterator) At() (int64, float64) {
	return b.it.At()
}

// AtHistogram is unsupported.
func (b *BufferedSeriesIterator) AtHistogram() (int64, *histogram.Histogram) {
	panic(errors.New("BufferedSeriesIterator: AtHistogram not implemented"))
}

// AtFloatHistogram is unsupported.
func (b *BufferedSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	panic(errors.New("BufferedSeriesIterator: AtFloatHistogram not implemented"))
}

// At returns the timestamp of the current element of the iterator.
func (b *BufferedSeriesIterator) AtT() int64 {
	return b.it.AtT()
}

// Err returns the last encountered error.
func (b *BufferedSeriesIterator) Err() error {
	return b.it.Err()
}

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

func (r *sampleRing) iterator() chunkenc.Iterator {
	return &sampleRingIterator{r: r, i: -1}
}

type sampleRingIterator struct {
	r *sampleRing
	i int
}

func (it *sampleRingIterator) Next() chunkenc.ValueType {
	it.i++
	if it.i < it.r.l {
		return chunkenc.ValFloat
	}
	return chunkenc.ValNone
}

func (it *sampleRingIterator) Seek(int64) chunkenc.ValueType {
	return chunkenc.ValNone
}

func (it *sampleRingIterator) Err() error {
	return nil
}

func (it *sampleRingIterator) At() (int64, float64) {
	return it.r.at(it.i)
}

func (it *sampleRingIterator) AtHistogram() (int64, *histogram.Histogram) {
	panic(errors.New("sampleRingIterator: AtHistogram not implemented"))
}

func (it *sampleRingIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	panic(errors.New("sampleRingIterator: AtFloatHistogram not implemented"))
}

func (it *sampleRingIterator) AtT() int64 {
	t, _ := it.r.at(it.i)
	return t
}

func (r *sampleRing) at(i int) (int64, float64) {
	j := (r.f + i) % len(r.buf)
	s := r.buf[j]
	return s.t, s.v
}

// add adds a sample to the ring buffer and frees all samples that fall
// out of the delta range.
func (r *sampleRing) add(t int64, v float64) {
	l := len(r.buf)
	// Grow the ring buffer if it fits no more elements.
	if l == r.l {
		buf := make([]sample, 2*l)
		copy(buf[l+r.f:], r.buf[r.f:])
		copy(buf, r.buf[:r.f])

		r.buf = buf
		r.i = r.f
		r.f += l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	r.buf[r.i] = sample{t: t, v: v}
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	for r.buf[r.f].t < t-r.delta {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
}

// last returns the most recent element added to the ring.
func (r *sampleRing) last() (int64, float64, bool) {
	if r.l == 0 {
		return 0, 0, false
	}
	s := r.buf[r.i]
	return s.t, s.v, true
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
