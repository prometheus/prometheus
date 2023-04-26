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
		buf:   newSampleRing(delta, 0, chunkenc.ValNone),
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
		b.buf.addF(fSample{t: t, f: f})
	case chunkenc.ValHistogram:
		t, h := b.it.AtHistogram()
		b.buf.addH(hSample{t: t, h: h})
	case chunkenc.ValFloatHistogram:
		t, fh := b.it.AtFloatHistogram()
		b.buf.addFH(fhSample{t: t, fh: fh})
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

func (s fSample) F() float64 {
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

func (s hSample) F() float64 {
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

func (s fhSample) F() float64 {
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

	// Lookback buffers. We use buf for mixed samples, but one of the three
	// concrete ones for homogenous samples. (Only one of the four bufs is
	// allowed to be populated!) This avoids the overhead of the interface
	// wrapper for the happy (and by far most common) case of homogenous
	// samples.
	buf   []tsdbutil.Sample
	fBuf  []fSample
	hBuf  []hSample
	fhBuf []fhSample

	i int // Position of most recent element in ring buffer.
	f int // Position of first element in ring buffer.
	l int // Number of elements in buffer.

	it sampleRingIterator
}

// newSampleRing creates a new sampleRing. If you do not know the prefereed
// value type yet, use a size of 0 (in which case the provided typ doesn't
// matter). On the first add, a buffer of size 16 will be allocated with the
// preferred type being the type of the first added sample.
func newSampleRing(delta int64, size int, typ chunkenc.ValueType) *sampleRing {
	r := &sampleRing{delta: delta}
	r.reset()
	if size <= 0 {
		// Will initialize on first add.
		return r
	}
	switch typ {
	case chunkenc.ValFloat:
		r.fBuf = make([]fSample, size)
	case chunkenc.ValHistogram:
		r.hBuf = make([]hSample, size)
	case chunkenc.ValFloatHistogram:
		r.fhBuf = make([]fhSample, size)
	default:
		r.buf = make([]tsdbutil.Sample, size)
	}
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
	f  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func (it *sampleRingIterator) Next() chunkenc.ValueType {
	it.i++
	if it.i >= it.r.l {
		return chunkenc.ValNone
	}
	switch {
	case len(it.r.fBuf) > 0:
		s := it.r.atF(it.i)
		it.t = s.t
		it.f = s.f
		return chunkenc.ValFloat
	case len(it.r.hBuf) > 0:
		s := it.r.atH(it.i)
		it.t = s.t
		it.h = s.h
		return chunkenc.ValHistogram
	case len(it.r.fhBuf) > 0:
		s := it.r.atFH(it.i)
		it.t = s.t
		it.fh = s.fh
		return chunkenc.ValFloatHistogram
	}
	s := it.r.at(it.i)
	it.t = s.T()
	switch s.Type() {
	case chunkenc.ValHistogram:
		it.h = s.H()
		it.fh = nil
		return chunkenc.ValHistogram
	case chunkenc.ValFloatHistogram:
		it.fh = s.FH()
		it.h = nil
		return chunkenc.ValFloatHistogram
	default:
		it.f = s.F()
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
	return it.t, it.f
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

func (r *sampleRing) atF(i int) fSample {
	j := (r.f + i) % len(r.fBuf)
	return r.fBuf[j]
}

func (r *sampleRing) atH(i int) hSample {
	j := (r.f + i) % len(r.hBuf)
	return r.hBuf[j]
}

func (r *sampleRing) atFH(i int) fhSample {
	j := (r.f + i) % len(r.fhBuf)
	return r.fhBuf[j]
}

// add adds a sample to the ring buffer and frees all samples that fall out of
// the delta range. Note that this method works for any sample
// implementation. If you know you are dealing with one of the implementations
// from this package (fSample, hSample, fhSample), call one of the specialized
// methods addF, addH, or addFH for better performance.
func (r *sampleRing) add(s tsdbutil.Sample) {
	if len(r.buf) == 0 {
		// Nothing added to the interface buf yet. Let's check if we can
		// stay specialized.
		switch s := s.(type) {
		case fSample:
			if len(r.hBuf)+len(r.fhBuf) == 0 {
				r.fBuf = addF(s, r.fBuf, r)
				return
			}
		case hSample:
			if len(r.fBuf)+len(r.fhBuf) == 0 {
				r.hBuf = addH(s, r.hBuf, r)
				return
			}
		case fhSample:
			if len(r.fBuf)+len(r.hBuf) == 0 {
				r.fhBuf = addFH(s, r.fhBuf, r)
				return
			}
		}
		// The new sample isn't a fit for the already existing
		// ones. Copy the latter into the interface buffer where needed.
		switch {
		case len(r.fBuf) > 0:
			for _, s := range r.fBuf {
				r.buf = append(r.buf, s)
			}
			r.fBuf = nil
		case len(r.hBuf) > 0:
			for _, s := range r.hBuf {
				r.buf = append(r.buf, s)
			}
			r.hBuf = nil
		case len(r.fhBuf) > 0:
			for _, s := range r.fhBuf {
				r.buf = append(r.buf, s)
			}
			r.fhBuf = nil
		}
	}
	r.buf = addSample(s, r.buf, r)
}

// addF is a version of the add method specialized for fSample.
func (r *sampleRing) addF(s fSample) {
	switch {
	case len(r.buf) > 0:
		// Already have interface samples. Add to the interface buf.
		r.buf = addSample(s, r.buf, r)
	case len(r.hBuf)+len(r.fhBuf) > 0:
		// Already have specialized samples that are not fSamples.
		// Need to call the checked add method for conversion.
		r.add(s)
	default:
		r.fBuf = addF(s, r.fBuf, r)
	}
}

// addH is a version of the add method specialized for hSample.
func (r *sampleRing) addH(s hSample) {
	switch {
	case len(r.buf) > 0:
		// Already have interface samples. Add to the interface buf.
		r.buf = addSample(s, r.buf, r)
	case len(r.fBuf)+len(r.fhBuf) > 0:
		// Already have samples that are not hSamples.
		// Need to call the checked add method for conversion.
		r.add(s)
	default:
		r.hBuf = addH(s, r.hBuf, r)
	}
}

// addFH is a version of the add method specialized for fhSample.
func (r *sampleRing) addFH(s fhSample) {
	switch {
	case len(r.buf) > 0:
		// Already have interface samples. Add to the interface buf.
		r.buf = addSample(s, r.buf, r)
	case len(r.fBuf)+len(r.hBuf) > 0:
		// Already have samples that are not fhSamples.
		// Need to call the checked add method for conversion.
		r.add(s)
	default:
		r.fhBuf = addFH(s, r.fhBuf, r)
	}
}

// genericAdd is a generic implementation of adding a tsdbutil.Sample
// implementation to a buffer of a sample ring. However, the Go compiler
// currently (go1.20) decides to not expand the code during compile time, but
// creates dynamic code to handle the different types. That has a significant
// overhead during runtime, noticeable in PromQL benchmarks. For example, the
// "RangeQuery/expr=rate(a_hundred[1d]),steps=.*" benchmarks show about 7%
// longer runtime, 9% higher allocation size, and 10% more allocations.
// Therefore, genericAdd has been manually implemented for all the types
// (addSample, addF, addH, addFH) below.
//
// func genericAdd[T tsdbutil.Sample](s T, buf []T, r *sampleRing) []T {
// 	l := len(buf)
// 	// Grow the ring buffer if it fits no more elements.
// 	if l == 0 {
// 		buf = make([]T, 16)
// 		l = 16
// 	}
// 	if l == r.l {
// 		newBuf := make([]T, 2*l)
// 		copy(newBuf[l+r.f:], buf[r.f:])
// 		copy(newBuf, buf[:r.f])
//
// 		buf = newBuf
// 		r.i = r.f
// 		r.f += l
// 		l = 2 * l
// 	} else {
// 		r.i++
// 		if r.i >= l {
// 			r.i -= l
// 		}
// 	}
//
// 	buf[r.i] = s
// 	r.l++
//
// 	// Free head of the buffer of samples that just fell out of the range.
// 	tmin := s.T() - r.delta
// 	for buf[r.f].T() < tmin {
// 		r.f++
// 		if r.f >= l {
// 			r.f -= l
// 		}
// 		r.l--
// 	}
// 	return buf
// }

// addSample is a handcoded specialization of genericAdd (see above).
func addSample(s tsdbutil.Sample, buf []tsdbutil.Sample, r *sampleRing) []tsdbutil.Sample {
	l := len(buf)
	// Grow the ring buffer if it fits no more elements.
	if l == 0 {
		buf = make([]tsdbutil.Sample, 16)
		l = 16
	}
	if l == r.l {
		newBuf := make([]tsdbutil.Sample, 2*l)
		copy(newBuf[l+r.f:], buf[r.f:])
		copy(newBuf, buf[:r.f])

		buf = newBuf
		r.i = r.f
		r.f += l
		l = 2 * l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	buf[r.i] = s
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	tmin := s.T() - r.delta
	for buf[r.f].T() < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
	return buf
}

// addF is a handcoded specialization of genericAdd (see above).
func addF(s fSample, buf []fSample, r *sampleRing) []fSample {
	l := len(buf)
	// Grow the ring buffer if it fits no more elements.
	if l == 0 {
		buf = make([]fSample, 16)
		l = 16
	}
	if l == r.l {
		newBuf := make([]fSample, 2*l)
		copy(newBuf[l+r.f:], buf[r.f:])
		copy(newBuf, buf[:r.f])

		buf = newBuf
		r.i = r.f
		r.f += l
		l = 2 * l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	buf[r.i] = s
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	tmin := s.T() - r.delta
	for buf[r.f].T() < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
	return buf
}

// addH is a handcoded specialization of genericAdd (see above).
func addH(s hSample, buf []hSample, r *sampleRing) []hSample {
	l := len(buf)
	// Grow the ring buffer if it fits no more elements.
	if l == 0 {
		buf = make([]hSample, 16)
		l = 16
	}
	if l == r.l {
		newBuf := make([]hSample, 2*l)
		copy(newBuf[l+r.f:], buf[r.f:])
		copy(newBuf, buf[:r.f])

		buf = newBuf
		r.i = r.f
		r.f += l
		l = 2 * l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	buf[r.i] = s
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	tmin := s.T() - r.delta
	for buf[r.f].T() < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
	return buf
}

// addFH is a handcoded specialization of genericAdd (see above).
func addFH(s fhSample, buf []fhSample, r *sampleRing) []fhSample {
	l := len(buf)
	// Grow the ring buffer if it fits no more elements.
	if l == 0 {
		buf = make([]fhSample, 16)
		l = 16
	}
	if l == r.l {
		newBuf := make([]fhSample, 2*l)
		copy(newBuf[l+r.f:], buf[r.f:])
		copy(newBuf, buf[:r.f])

		buf = newBuf
		r.i = r.f
		r.f += l
		l = 2 * l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	buf[r.i] = s
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	tmin := s.T() - r.delta
	for buf[r.f].T() < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
	return buf
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

	switch {
	case len(r.fBuf) > 0:
		genericReduceDelta(r.fBuf, r)
	case len(r.hBuf) > 0:
		genericReduceDelta(r.hBuf, r)
	case len(r.fhBuf) > 0:
		genericReduceDelta(r.fhBuf, r)
	default:
		genericReduceDelta(r.buf, r)
	}
	return true
}

func genericReduceDelta[T tsdbutil.Sample](buf []T, r *sampleRing) {
	// Free head of the buffer of samples that just fell out of the range.
	l := len(buf)
	tmin := buf[r.i].T() - r.delta
	for buf[r.f].T() < tmin {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
}

// nthLast returns the nth most recent element added to the ring.
func (r *sampleRing) nthLast(n int) (tsdbutil.Sample, bool) {
	if n > r.l {
		return fSample{}, false
	}
	i := r.l - n
	switch {
	case len(r.fBuf) > 0:
		return r.atF(i), true
	case len(r.hBuf) > 0:
		return r.atH(i), true
	case len(r.fhBuf) > 0:
		return r.atFH(i), true
	default:
		return r.at(i), true
	}
}

func (r *sampleRing) samples() []tsdbutil.Sample {
	res := make([]tsdbutil.Sample, r.l)

	k := r.f + r.l
	var j int

	switch {
	case len(r.buf) > 0:
		if k > len(r.buf) {
			k = len(r.buf)
			j = r.l - k + r.f
		}
		n := copy(res, r.buf[r.f:k])
		copy(res[n:], r.buf[:j])
	case len(r.fBuf) > 0:
		if k > len(r.fBuf) {
			k = len(r.fBuf)
			j = r.l - k + r.f
		}
		resF := make([]fSample, r.l)
		n := copy(resF, r.fBuf[r.f:k])
		copy(resF[n:], r.fBuf[:j])
		for i, s := range resF {
			res[i] = s
		}
	case len(r.hBuf) > 0:
		if k > len(r.hBuf) {
			k = len(r.hBuf)
			j = r.l - k + r.f
		}
		resH := make([]hSample, r.l)
		n := copy(resH, r.hBuf[r.f:k])
		copy(resH[n:], r.hBuf[:j])
		for i, s := range resH {
			res[i] = s
		}
	case len(r.fhBuf) > 0:
		if k > len(r.fhBuf) {
			k = len(r.fhBuf)
			j = r.l - k + r.f
		}
		resFH := make([]fhSample, r.l)
		n := copy(resFH, r.fhBuf[r.f:k])
		copy(resFH[n:], r.fhBuf[:j])
		for i, s := range resFH {
			res[i] = s
		}
	}

	return res
}
