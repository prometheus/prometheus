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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestMemoizedSeriesIterator(t *testing.T) {
	var it *MemoizedSeriesIterator

	sampleEq := func(ets int64, ev float64, efh *histogram.FloatHistogram) {
		if efh == nil {
			ts, v := it.At()
			require.Equal(t, ets, ts, "timestamp mismatch")
			require.Equal(t, ev, v, "value mismatch")
		} else {
			ts, fh := it.AtFloatHistogram()
			require.Equal(t, ets, ts, "timestamp mismatch")
			require.Equal(t, efh, fh, "histogram mismatch")
		}
	}
	prevSampleEq := func(ets int64, ev float64, efh *histogram.FloatHistogram, eok bool) {
		ts, v, fh, ok := it.PeekPrev()
		require.Equal(t, eok, ok, "exist mismatch")
		require.Equal(t, ets, ts, "timestamp mismatch")
		if efh == nil {
			require.Equal(t, ev, v, "value mismatch")
		} else {
			require.Equal(t, efh, fh, "histogram mismatch")
		}
	}

	it = NewMemoizedIterator(NewListSeriesIterator(samples{
		fSample{t: 1, f: 2},
		fSample{t: 2, f: 3},
		fSample{t: 3, f: 4},
		fSample{t: 4, f: 5},
		fSample{t: 5, f: 6},
		fSample{t: 99, f: 8},
		fSample{t: 100, f: 9},
		fSample{t: 101, f: 10},
		hSample{t: 102, h: tsdbutil.GenerateTestHistogram(0)},
		hSample{t: 103, h: tsdbutil.GenerateTestHistogram(1)},
		fhSample{t: 104, fh: tsdbutil.GenerateTestFloatHistogram(2)},
		fhSample{t: 199, fh: tsdbutil.GenerateTestFloatHistogram(3)},
		hSample{t: 200, h: tsdbutil.GenerateTestHistogram(4)},
		fhSample{t: 299, fh: tsdbutil.GenerateTestFloatHistogram(5)},
		fSample{t: 300, f: 11},
		hSample{t: 399, h: tsdbutil.GenerateTestHistogram(6)},
		fSample{t: 400, f: 12},
	}), 2)

	require.Equal(t, it.Seek(-123), chunkenc.ValFloat, "seek failed")
	sampleEq(1, 2, nil)
	prevSampleEq(0, 0, nil, false)

	require.Equal(t, it.Seek(5), chunkenc.ValFloat, "seek failed")
	sampleEq(5, 6, nil)
	prevSampleEq(4, 5, nil, true)

	// Seek to a histogram sample with a previous float sample.
	require.Equal(t, it.Seek(102), chunkenc.ValFloatHistogram, "seek failed")
	sampleEq(102, 10, tsdbutil.GenerateTestFloatHistogram(0))
	prevSampleEq(101, 10, nil, true)

	// Attempt to seek backwards (no-op).
	require.Equal(t, it.Seek(50), chunkenc.ValFloatHistogram, "seek failed")
	sampleEq(102, 10, tsdbutil.GenerateTestFloatHistogram(0))
	prevSampleEq(101, 10, nil, true)

	// Seek to a float histogram sample with a previous histogram sample.
	require.Equal(t, it.Seek(104), chunkenc.ValFloatHistogram, "seek failed")
	sampleEq(104, 0, tsdbutil.GenerateTestFloatHistogram(2))
	prevSampleEq(103, 0, tsdbutil.GenerateTestFloatHistogram(1), true)

	// Seek to a float sample with a previous float histogram sample.
	require.Equal(t, chunkenc.ValFloat, it.Seek(300), "seek failed")
	sampleEq(300, 11, nil)
	prevSampleEq(299, 0, tsdbutil.GenerateTestFloatHistogram(5), true)

	// Seek to a float sample with a previous histogram sample.
	require.Equal(t, chunkenc.ValFloat, it.Seek(400), "seek failed")
	sampleEq(400, 12, nil)
	prevSampleEq(399, 0, tsdbutil.GenerateTestFloatHistogram(6), true)

	require.Equal(t, it.Seek(1024), chunkenc.ValNone, "seek succeeded unexpectedly")
}

func BenchmarkMemoizedSeriesIterator(b *testing.B) {
	// Simulate a 5 minute rate.
	it := NewMemoizedIterator(newFakeSeriesIterator(int64(b.N), 30), 5*60)

	b.SetBytes(16)
	b.ReportAllocs()
	b.ResetTimer()

	for it.Next() != chunkenc.ValNone { // nolint:revive
		// Scan everything.
	}
	require.NoError(b, it.Err())
}
