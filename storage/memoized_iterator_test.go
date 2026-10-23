// Copyright The Prometheus Authors
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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestMemoizedSeriesIterator(t *testing.T) {
	t.Run("initial seek retains preloaded sample", func(t *testing.T) {
		for _, setup := range []string{"new", "reset"} {
			t.Run(setup+"/empty", func(t *testing.T) {
				it := NewMemoizedIterator(NewListSeriesIterator(samples{}), 0)
				if setup == "reset" {
					it.Reset(NewListSeriesIterator(samples{fSample{t: 1, f: 7}, fSample{t: 2, f: 8}}))
					it.Next()
					it.Reset(NewListSeriesIterator(samples{}))
				}
				require.Equal(t, chunkenc.ValNone, it.Seek(0))
				require.Equal(t, chunkenc.ValNone, it.Next())
				_, _, _, _, ok := it.PeekPrev()
				require.False(t, ok)
				require.NoError(t, it.Err())
			})
		}
		for _, setup := range []string{"new", "reset"} {
			for _, kind := range []chunkenc.ValueType{chunkenc.ValFloat, chunkenc.ValHistogram, chunkenc.ValFloatHistogram} {
				for _, tc := range []struct {
					name                 string
					first, target, delta int64
				}{
					{"minimum floor", math.MinInt64 + 1, math.MinInt64 + 1, 1},
					{"ordinary sample minimum floor", 10, math.MinInt64 + 1, 1},
					{"upper sample minimum floor", math.MaxInt64 - 1, math.MinInt64 + 1, 1},
					{"minimum zero delta", math.MinInt64 + 1, math.MinInt64 + 1, 0},
					{"ordinary retaining", 10, 10, 1},
					{"upper retaining", math.MaxInt64 - 1, math.MaxInt64 - 1, 1},
				} {
					t.Run(fmt.Sprintf("%s/%s/%s", setup, kind, tc.name), func(t *testing.T) {
						expected := &histogram.FloatHistogram{Schema: 0, ZeroThreshold: 1, Count: 2, Sum: 3, CounterResetHint: histogram.GaugeType, PositiveSpans: []histogram.Span{{Offset: 1, Length: 1}}, PositiveBuckets: []float64{2}}
						input := samples{fSample{st: tc.first - 1, t: tc.first, f: 7}, fSample{st: tc.first, t: tc.first + 1, f: 8}}
						wantType := chunkenc.ValFloat
						switch kind {
						case chunkenc.ValHistogram:
							wantType = chunkenc.ValFloatHistogram
							input[0] = hSample{st: tc.first - 1, t: tc.first, h: &histogram.Histogram{Schema: 0, ZeroThreshold: 1, Count: 2, Sum: 3, CounterResetHint: histogram.GaugeType, PositiveSpans: []histogram.Span{{Offset: 1, Length: 1}}, PositiveBuckets: []int64{2}}}
						case chunkenc.ValFloatHistogram:
							wantType = chunkenc.ValFloatHistogram
							input[0] = fhSample{st: tc.first - 1, t: tc.first, fh: expected.Copy()}
						}
						var it *MemoizedSeriesIterator
						if setup == "new" {
							it = NewMemoizedIterator(NewListSeriesIterator(input), tc.delta)
						} else {
							it = NewMemoizedIterator(NewListSeriesIterator(samples{fSample{t: 1, f: 1}, fSample{t: 2, f: 2}}), tc.delta)
							it.Next()
							it.Reset(NewListSeriesIterator(input))
						}
						for range 2 {
							got := it.Seek(tc.target)
							// Check position first to isolate the cursor defect from the
							// separate preloaded histogram normalization contract.
							require.Equal(t, tc.first, it.AtT())
							require.Equal(t, wantType, got)
							require.Equal(t, tc.first-1, it.AtST())
							if kind == chunkenc.ValFloat {
								ts, v := it.At()
								require.Equal(t, tc.first, ts)
								require.Equal(t, float64(7), v)
							} else {
								ts, h := it.AtFloatHistogram()
								require.Equal(t, tc.first, ts)
								require.Equal(t, expected, h)
							}
							_, _, _, _, ok := it.PeekPrev()
							require.False(t, ok)
						}
						require.Equal(t, chunkenc.ValFloat, it.Next())
						ts, v := it.At()
						require.Equal(t, tc.first+1, ts)
						require.Equal(t, float64(8), v)
						require.Equal(t, tc.first, it.AtST())
						st, ts, v, h, ok := it.PeekPrev()
						require.True(t, ok)
						require.Equal(t, tc.first-1, st)
						require.Equal(t, tc.first, ts)
						if kind == chunkenc.ValFloat {
							require.Equal(t, float64(7), v)
							require.Nil(t, h)
						} else {
							require.Zero(t, v)
							require.Equal(t, expected, h)
						}
						require.Equal(t, chunkenc.ValNone, it.Next())
						require.NoError(t, it.Err())
					})
				}
			}
		}
	})

	var it *MemoizedSeriesIterator

	sampleEq := func(est, ets int64, ev float64, efh *histogram.FloatHistogram) {
		if efh == nil {
			ts, v := it.At()
			require.Equal(t, ets, ts, "At() timestamp mismatch")
			require.Equal(t, ev, v, "At() value mismatch")
		} else {
			ts, fh := it.AtFloatHistogram()
			require.Equal(t, ets, ts, "AtFloatHistogram() timestamp mismatch")
			require.Equal(t, efh, fh, "AtFloatHistogram() histogram mismatch")
		}

		require.Equal(t, est, it.AtST(), "AtST() start timestamp mismatch")
		require.Equal(t, ets, it.AtT(), "AtT() timestamp mismatch")
	}
	prevSampleEq := func(est, ets int64, ev float64, efh *histogram.FloatHistogram, eok bool) {
		st, ts, v, fh, ok := it.PeekPrev()
		require.Equal(t, est, st, "start timestamp mismatch")
		require.Equal(t, eok, ok, "exist mismatch")
		require.Equal(t, ets, ts, "timestamp mismatch")
		if efh == nil {
			require.Equal(t, ev, v, "value mismatch")
		} else {
			require.Equal(t, efh, fh, "histogram mismatch")
		}
	}

	it = NewMemoizedIterator(NewListSeriesIterator(samples{
		fSample{st: 0, t: 1, f: 2},
		fSample{st: 1, t: 2, f: 3},
		fSample{st: 2, t: 3, f: 4},
		fSample{st: 3, t: 4, f: 5},
		fSample{st: 4, t: 5, f: 6},
		fSample{st: 98, t: 99, f: 8},
		fSample{st: 99, t: 100, f: 9},
		fSample{st: 100, t: 101, f: 10},
		hSample{st: 101, t: 102, h: tsdbutil.GenerateTestHistogram(0)},
		hSample{st: 102, t: 103, h: tsdbutil.GenerateTestHistogram(1)},
		fhSample{st: 103, t: 104, fh: tsdbutil.GenerateTestFloatHistogram(2)},
		fhSample{st: 104, t: 199, fh: tsdbutil.GenerateTestFloatHistogram(3)},
		hSample{st: 199, t: 200, h: tsdbutil.GenerateTestHistogram(4)},
		fhSample{st: 298, t: 299, fh: tsdbutil.GenerateTestFloatHistogram(5)},
		fSample{st: 299, t: 300, f: 11},
		hSample{st: 350, t: 399, h: tsdbutil.GenerateTestHistogram(6)},
		fSample{st: 399, t: 400, f: 12},
	}), 2)

	require.Equal(t, chunkenc.ValFloat, it.Seek(-123), "seek failed")
	sampleEq(0, 1, 2, nil)
	prevSampleEq(0, 0, 0, nil, false)

	require.Equal(t, chunkenc.ValFloat, it.Seek(5), "seek failed")
	sampleEq(4, 5, 6, nil)
	prevSampleEq(3, 4, 5, nil, true)

	// Seek to a histogram sample with a previous float sample.
	require.Equal(t, chunkenc.ValFloatHistogram, it.Seek(102), "seek failed")
	sampleEq(101, 102, 10, tsdbutil.GenerateTestFloatHistogram(0))
	prevSampleEq(100, 101, 10, nil, true)

	// Attempt to seek backwards (no-op).
	require.Equal(t, chunkenc.ValFloatHistogram, it.Seek(50), "seek failed")
	sampleEq(101, 102, 10, tsdbutil.GenerateTestFloatHistogram(0))
	prevSampleEq(100, 101, 10, nil, true)

	// Seek to a float histogram sample with a previous histogram sample.
	require.Equal(t, chunkenc.ValFloatHistogram, it.Seek(104), "seek failed")
	sampleEq(103, 104, 0, tsdbutil.GenerateTestFloatHistogram(2))
	prevSampleEq(102, 103, 0, tsdbutil.GenerateTestFloatHistogram(1), true)

	// Seek to a float sample with a previous float histogram sample.
	require.Equal(t, chunkenc.ValFloat, it.Seek(300), "seek failed")
	sampleEq(299, 300, 11, nil)
	prevSampleEq(298, 299, 0, tsdbutil.GenerateTestFloatHistogram(5), true)

	// Seek to a float sample with a previous histogram sample.
	require.Equal(t, chunkenc.ValFloat, it.Seek(400), "seek failed")
	sampleEq(399, 400, 12, nil)
	prevSampleEq(350, 399, 0, tsdbutil.GenerateTestFloatHistogram(6), true)

	require.Equal(t, chunkenc.ValNone, it.Seek(1024), "seek succeeded unexpectedly")
}

func BenchmarkMemoizedSeriesIterator(b *testing.B) {
	// Simulate a 5 minute rate.
	it := NewMemoizedIterator(newFakeSeriesIterator(int64(b.N), 30), 5*60)

	b.SetBytes(16)
	b.ReportAllocs()
	b.ResetTimer()

	for it.Next() != chunkenc.ValNone {
		// Scan everything.
	}
	require.NoError(b, it.Err())
}
