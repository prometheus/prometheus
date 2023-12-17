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
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestSampleRing(t *testing.T) {
	cases := []struct {
		input []int64
		delta int64
		size  int
	}{
		{
			input: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			delta: 2,
			size:  1,
		},
		{
			input: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			delta: 2,
			size:  2,
		},
		{
			input: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			delta: 7,
			size:  3,
		},
		{
			input: []int64{1, 2, 3, 4, 5, 16, 17, 18, 19, 20},
			delta: 7,
			size:  1,
		},
		{
			input: []int64{1, 2, 3, 4, 6},
			delta: 4,
			size:  4,
		},
	}
	for _, c := range cases {
		r := newSampleRing(c.delta, c.size, chunkenc.ValFloat)

		input := []fSample{}
		for _, t := range c.input {
			input = append(input, fSample{
				t: t,
				f: float64(rand.Intn(100)),
			})
		}

		for i, s := range input {
			r.add(s)
			buffered := r.samples()

			for _, sold := range input[:i] {
				found := false
				for _, bs := range buffered {
					if bs.T() == sold.t && bs.F() == sold.f {
						found = true
						break
					}
				}

				if found {
					require.GreaterOrEqual(t, sold.t, s.t-c.delta, "%d: unexpected sample %d in buffer; buffer %v", i, sold.t, buffered)
				} else {
					require.Less(t, sold.t, s.t-c.delta, "%d: expected sample %d to be in buffer but was not; buffer %v", i, sold.t, buffered)
				}
			}
		}
	}
}

func TestSampleRingMixed(t *testing.T) {
	h1 := tsdbutil.GenerateTestHistogram(1)
	h2 := tsdbutil.GenerateTestHistogram(2)

	// With ValNone as the preferred type, nothing should be initialized.
	r := newSampleRing(10, 2, chunkenc.ValNone)
	require.Zero(t, len(r.fBuf))
	require.Zero(t, len(r.hBuf))
	require.Zero(t, len(r.fhBuf))
	require.Zero(t, len(r.iBuf))

	// But then mixed adds should work as expected.
	r.addF(fSample{t: 1, f: 3.14})
	r.addH(hSample{t: 2, h: h1})

	it := r.iterator()

	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, f := it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, 3.14, f)
	require.Equal(t, chunkenc.ValHistogram, it.Next())
	var h *histogram.Histogram
	ts, h = it.AtHistogram()
	require.Equal(t, int64(2), ts)
	require.Equal(t, h1, h)
	require.Equal(t, chunkenc.ValNone, it.Next())

	r.reset()
	it = r.iterator()
	require.Equal(t, chunkenc.ValNone, it.Next())

	r.addF(fSample{t: 3, f: 4.2})
	r.addH(hSample{t: 4, h: h2})

	it = r.iterator()

	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, f = it.At()
	require.Equal(t, int64(3), ts)
	require.Equal(t, 4.2, f)
	require.Equal(t, chunkenc.ValHistogram, it.Next())
	ts, h = it.AtHistogram()
	require.Equal(t, int64(4), ts)
	require.Equal(t, h2, h)
	require.Equal(t, chunkenc.ValNone, it.Next())
}

func TestBufferedSeriesIterator(t *testing.T) {
	var it *BufferedSeriesIterator

	bufferEq := func(exp []fSample) {
		var b []fSample
		bit := it.Buffer()
		for bit.Next() == chunkenc.ValFloat {
			t, f := bit.At()
			b = append(b, fSample{t: t, f: f})
		}
		require.Equal(t, exp, b, "buffer mismatch")
	}
	sampleEq := func(ets int64, ev float64) {
		ts, v := it.At()
		require.Equal(t, ets, ts, "timestamp mismatch")
		require.Equal(t, ev, v, "value mismatch")
	}
	prevSampleEq := func(ets int64, ev float64, eok bool) {
		s, ok := it.PeekBack(1)
		require.Equal(t, eok, ok, "exist mismatch")
		require.Equal(t, ets, s.T(), "timestamp mismatch")
		require.Equal(t, ev, s.F(), "value mismatch")
	}

	it = NewBufferIterator(NewListSeriesIterator(samples{
		fSample{t: 1, f: 2},
		fSample{t: 2, f: 3},
		fSample{t: 3, f: 4},
		fSample{t: 4, f: 5},
		fSample{t: 5, f: 6},
		fSample{t: 99, f: 8},
		fSample{t: 100, f: 9},
		fSample{t: 101, f: 10},
	}), 2)

	require.Equal(t, chunkenc.ValFloat, it.Seek(-123), "seek failed")
	sampleEq(1, 2)
	prevSampleEq(0, 0, false)
	bufferEq(nil)

	require.Equal(t, chunkenc.ValFloat, it.Next(), "next failed")
	sampleEq(2, 3)
	prevSampleEq(1, 2, true)
	bufferEq([]fSample{{t: 1, f: 2}})

	require.Equal(t, chunkenc.ValFloat, it.Next(), "next failed")
	require.Equal(t, chunkenc.ValFloat, it.Next(), "next failed")
	require.Equal(t, chunkenc.ValFloat, it.Next(), "next failed")
	sampleEq(5, 6)
	prevSampleEq(4, 5, true)
	bufferEq([]fSample{{t: 2, f: 3}, {t: 3, f: 4}, {t: 4, f: 5}})

	require.Equal(t, chunkenc.ValFloat, it.Seek(5), "seek failed")
	sampleEq(5, 6)
	prevSampleEq(4, 5, true)
	bufferEq([]fSample{{t: 2, f: 3}, {t: 3, f: 4}, {t: 4, f: 5}})

	require.Equal(t, chunkenc.ValFloat, it.Seek(101), "seek failed")
	sampleEq(101, 10)
	prevSampleEq(100, 9, true)
	bufferEq([]fSample{{t: 99, f: 8}, {t: 100, f: 9}})

	require.Equal(t, chunkenc.ValNone, it.Next(), "next succeeded unexpectedly")
	require.Equal(t, chunkenc.ValNone, it.Seek(1024), "seek succeeded unexpectedly")
}

// At() should not be called once Next() returns false.
func TestBufferedSeriesIteratorNoBadAt(t *testing.T) {
	done := false

	m := &mockSeriesIterator{
		seek: func(int64) chunkenc.ValueType { return chunkenc.ValNone },
		at: func() (int64, float64) {
			require.False(t, done, "unexpectedly done")
			done = true
			return 0, 0
		},
		next: func() chunkenc.ValueType {
			if done {
				return chunkenc.ValNone
			}
			return chunkenc.ValFloat
		},
		err: func() error { return nil },
	}

	it := NewBufferIterator(m, 60)
	it.Next()
	it.Next()
}

func TestBufferedSeriesIteratorMixedHistograms(t *testing.T) {
	histograms := tsdbutil.GenerateTestHistograms(2)

	it := NewBufferIterator(NewListSeriesIterator(samples{
		fhSample{t: 1, fh: histograms[0].ToFloat(nil)},
		hSample{t: 2, h: histograms[1]},
	}), 2)

	require.Equal(t, chunkenc.ValNone, it.Seek(3))
	require.NoError(t, it.Err())

	buf := it.Buffer()

	require.Equal(t, chunkenc.ValFloatHistogram, buf.Next())
	_, fh := buf.AtFloatHistogram(nil)
	require.Equal(t, histograms[0].ToFloat(nil), fh)

	require.Equal(t, chunkenc.ValHistogram, buf.Next())
	_, fh = buf.AtFloatHistogram(nil)
	require.Equal(t, histograms[1].ToFloat(nil), fh)
}

func BenchmarkBufferedSeriesIterator(b *testing.B) {
	// Simulate a 5 minute rate.
	it := NewBufferIterator(newFakeSeriesIterator(int64(b.N), 30), 5*60)

	b.SetBytes(16)
	b.ReportAllocs()
	b.ResetTimer()

	for it.Next() != chunkenc.ValNone {
		// Scan everything.
	}
	require.NoError(b, it.Err())
}

type mockSeriesIterator struct {
	seek func(int64) chunkenc.ValueType
	at   func() (int64, float64)
	next func() chunkenc.ValueType
	err  func() error
}

func (m *mockSeriesIterator) Seek(t int64) chunkenc.ValueType { return m.seek(t) }
func (m *mockSeriesIterator) At() (int64, float64)            { return m.at() }
func (m *mockSeriesIterator) Next() chunkenc.ValueType        { return m.next() }
func (m *mockSeriesIterator) Err() error                      { return m.err() }

func (m *mockSeriesIterator) AtHistogram() (int64, *histogram.Histogram) {
	return 0, nil // Not really mocked.
}

func (m *mockSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	return 0, nil // Not really mocked.
}

func (m *mockSeriesIterator) AtT() int64 {
	return 0 // Not really mocked.
}

type fakeSeriesIterator struct {
	nsamples int64
	step     int64
	idx      int64
}

func newFakeSeriesIterator(nsamples, step int64) *fakeSeriesIterator {
	return &fakeSeriesIterator{nsamples: nsamples, step: step, idx: -1}
}

func (it *fakeSeriesIterator) At() (int64, float64) {
	return it.idx * it.step, 123 // Value doesn't matter.
}

func (it *fakeSeriesIterator) AtHistogram() (int64, *histogram.Histogram) {
	return it.idx * it.step, &histogram.Histogram{} // Value doesn't matter.
}

func (it *fakeSeriesIterator) AtFloatHistogram() (int64, *histogram.FloatHistogram) {
	return it.idx * it.step, &histogram.FloatHistogram{} // Value doesn't matter.
}

func (it *fakeSeriesIterator) AtT() int64 {
	return it.idx * it.step
}

func (it *fakeSeriesIterator) Next() chunkenc.ValueType {
	it.idx++
	if it.idx >= it.nsamples {
		return chunkenc.ValNone
	}
	return chunkenc.ValFloat
}

func (it *fakeSeriesIterator) Seek(t int64) chunkenc.ValueType {
	it.idx = t / it.step
	if it.idx >= it.nsamples {
		return chunkenc.ValNone
	}
	return chunkenc.ValFloat
}

func (it *fakeSeriesIterator) Err() error { return nil }
