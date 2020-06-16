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

	"github.com/prometheus/prometheus/util/testutil"
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
		r := newSampleRing(c.delta, c.size)

		input := []sample{}
		for _, t := range c.input {
			input = append(input, sample{
				t: t,
				v: float64(rand.Intn(100)),
			})
		}

		for i, s := range input {
			r.add(s.t, s.v)
			buffered := r.samples()

			for _, sold := range input[:i] {
				found := false
				for _, bs := range buffered {
					if bs.t == sold.t && bs.v == sold.v {
						found = true
						break
					}
				}

				if found {
					testutil.Assert(t, sold.t >= s.t-c.delta, "%d: unexpected sample %d in buffer; buffer %v", i, sold.t, buffered)
				} else {
					testutil.Assert(t, sold.t < s.t-c.delta, "%d: expected sample %d to be in buffer but was not; buffer %v", i, sold.t, buffered)
				}
			}
		}
	}
}

func TestBufferedSeriesIterator(t *testing.T) {
	var it *BufferedSeriesIterator

	bufferEq := func(exp []sample) {
		var b []sample
		bit := it.Buffer()
		for bit.Next() {
			t, v := bit.At()
			b = append(b, sample{t: t, v: v})
		}
		testutil.Equals(t, exp, b, "buffer mismatch")
	}
	sampleEq := func(ets int64, ev float64) {
		ts, v := it.Values()
		testutil.Equals(t, ets, ts, "timestamp mismatch")
		testutil.Equals(t, ev, v, "value mismatch")
	}

	it = NewBufferIterator(NewListSeriesIterator(samples{
		sample{t: 1, v: 2},
		sample{t: 2, v: 3},
		sample{t: 3, v: 4},
		sample{t: 4, v: 5},
		sample{t: 5, v: 6},
		sample{t: 99, v: 8},
		sample{t: 100, v: 9},
		sample{t: 101, v: 10},
	}), 2)

	testutil.Assert(t, it.Seek(-123), "seek failed")
	sampleEq(1, 2)
	bufferEq(nil)

	testutil.Assert(t, it.Next(), "next failed")
	sampleEq(2, 3)
	bufferEq([]sample{{t: 1, v: 2}})

	testutil.Assert(t, it.Next(), "next failed")
	testutil.Assert(t, it.Next(), "next failed")
	testutil.Assert(t, it.Next(), "next failed")
	sampleEq(5, 6)
	bufferEq([]sample{{t: 2, v: 3}, {t: 3, v: 4}, {t: 4, v: 5}})

	testutil.Assert(t, it.Seek(5), "seek failed")
	sampleEq(5, 6)
	bufferEq([]sample{{t: 2, v: 3}, {t: 3, v: 4}, {t: 4, v: 5}})

	testutil.Assert(t, it.Seek(101), "seek failed")
	sampleEq(101, 10)
	bufferEq([]sample{{t: 99, v: 8}, {t: 100, v: 9}})

	testutil.Assert(t, !it.Next(), "next succeeded unexpectedly")
}

// At() should not be called once Next() returns false.
func TestBufferedSeriesIteratorNoBadAt(t *testing.T) {
	done := false

	m := &mockSeriesIterator{
		seek: func(int64) bool { return false },
		at: func() (int64, float64) {
			testutil.Assert(t, !done, "unexpectedly done")
			done = true
			return 0, 0
		},
		next: func() bool { return !done },
		err:  func() error { return nil },
	}

	it := NewBufferIterator(m, 60)
	it.Next()
	it.Next()
}

func BenchmarkBufferedSeriesIterator(b *testing.B) {
	// Simulate a 5 minute rate.
	it := NewBufferIterator(newFakeSeriesIterator(int64(b.N), 30), 5*60)

	b.SetBytes(int64(b.N * 16))
	b.ReportAllocs()
	b.ResetTimer()

	for it.Next() {
		// scan everything
	}
	testutil.Ok(b, it.Err())
}

type mockSeriesIterator struct {
	seek func(int64) bool
	at   func() (int64, float64)
	next func() bool
	err  func() error
}

func (m *mockSeriesIterator) Seek(t int64) bool    { return m.seek(t) }
func (m *mockSeriesIterator) At() (int64, float64) { return m.at() }
func (m *mockSeriesIterator) Next() bool           { return m.next() }
func (m *mockSeriesIterator) Err() error           { return m.err() }

type fakeSeriesIterator struct {
	nsamples int64
	step     int64
	idx      int64
}

func newFakeSeriesIterator(nsamples, step int64) *fakeSeriesIterator {
	return &fakeSeriesIterator{nsamples: nsamples, step: step, idx: -1}
}

func (it *fakeSeriesIterator) At() (int64, float64) {
	return it.idx * it.step, 123 // value doesn't matter
}

func (it *fakeSeriesIterator) Next() bool {
	it.idx++
	return it.idx < it.nsamples
}

func (it *fakeSeriesIterator) Seek(t int64) bool {
	it.idx = t / it.step
	return it.idx < it.nsamples
}

func (it *fakeSeriesIterator) Err() error { return nil }
