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

package promql

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestHistogramStatsDecoding(t *testing.T) {
	cases := []struct {
		name          string
		histograms    []*histogram.Histogram
		expectedHints []histogram.CounterResetHint
	}{
		{
			name: "unknown counter reset for later sample triggers detection",
			histograms: []*histogram.Histogram{
				tsdbutil.GenerateTestHistogramWithHint(0, histogram.NotCounterReset),
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
				tsdbutil.GenerateTestHistogramWithHint(2, histogram.CounterReset),
				tsdbutil.GenerateTestHistogramWithHint(2, histogram.UnknownCounterReset),
			},
			expectedHints: []histogram.CounterResetHint{
				histogram.NotCounterReset,
				histogram.NotCounterReset,
				histogram.CounterReset,
				histogram.NotCounterReset,
			},
		},
		{
			name: "unknown counter reset for first sample does not trigger detection",
			histograms: []*histogram.Histogram{
				tsdbutil.GenerateTestHistogramWithHint(0, histogram.UnknownCounterReset),
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
				tsdbutil.GenerateTestHistogramWithHint(2, histogram.CounterReset),
				tsdbutil.GenerateTestHistogramWithHint(2, histogram.UnknownCounterReset),
			},
			expectedHints: []histogram.CounterResetHint{
				histogram.UnknownCounterReset,
				histogram.NotCounterReset,
				histogram.CounterReset,
				histogram.NotCounterReset,
			},
		},
		{
			name: "stale sample before unknown reset hint",
			histograms: []*histogram.Histogram{
				tsdbutil.GenerateTestHistogramWithHint(0, histogram.NotCounterReset),
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
				{Sum: math.Float64frombits(value.StaleNaN)},
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
			},
			expectedHints: []histogram.CounterResetHint{
				histogram.NotCounterReset,
				histogram.NotCounterReset,
				histogram.UnknownCounterReset,
				histogram.NotCounterReset,
			},
		},
		{
			name: "unknown counter reset at the beginning",
			histograms: []*histogram.Histogram{
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
			},
			expectedHints: []histogram.CounterResetHint{
				histogram.UnknownCounterReset,
			},
		},
		{
			name: "detect real counter reset",
			histograms: []*histogram.Histogram{
				tsdbutil.GenerateTestHistogramWithHint(2, histogram.UnknownCounterReset),
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
			},
			expectedHints: []histogram.CounterResetHint{
				histogram.UnknownCounterReset,
				histogram.CounterReset,
			},
		},
		{
			name: "detect real counter reset after stale NaN",
			histograms: []*histogram.Histogram{
				tsdbutil.GenerateTestHistogramWithHint(2, histogram.UnknownCounterReset),
				{Sum: math.Float64frombits(value.StaleNaN)},
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
			},
			expectedHints: []histogram.CounterResetHint{
				histogram.UnknownCounterReset,
				histogram.UnknownCounterReset,
				histogram.CounterReset,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check := func(statsIterator *HistogramStatsIterator) {
				decodedStats := make([]*histogram.FloatHistogram, 0)
				for typ := statsIterator.Next(); typ != chunkenc.ValNone; typ = statsIterator.Next() {
					require.Equal(t, chunkenc.ValFloatHistogram, typ)
					t1, h1 := statsIterator.AtFloatHistogram(nil)
					// Call AtFloatHistogram again to check for idempotency.
					t2, h2 := statsIterator.AtFloatHistogram(nil)
					require.Equal(t, t1, t2)
					require.True(t, h1.Equals(h2)) // require.Equal does not work with sum=NaN.
					decodedStats = append(decodedStats, h1)
				}
				require.NoError(t, statsIterator.Err())
				for i := 0; i < len(tc.histograms); i++ {
					require.Equal(t, tc.expectedHints[i], decodedStats[i].CounterResetHint)
					fh := tc.histograms[i].ToFloat(nil)
					if value.IsStaleNaN(fh.Sum) {
						require.True(t, value.IsStaleNaN(decodedStats[i].Sum))
						require.Equal(t, float64(0), decodedStats[i].Count)
					} else {
						require.Equal(t, fh.Count, decodedStats[i].Count)
						require.Equal(t, fh.Sum, decodedStats[i].Sum)
					}
				}
			}

			// Check that we get the expected results with a fresh iterator.
			statsIterator := NewHistogramStatsIterator(newHistogramSeries(tc.histograms).Iterator(nil))
			check(statsIterator)

			// Check that we get the same results if we reset and reuse that iterator.
			statsIterator.Reset(newHistogramSeries(tc.histograms).Iterator(nil))
			check(statsIterator)
		})
	}
}

func TestHistogramStatsMixedUse(t *testing.T) {
	histograms := []*histogram.Histogram{
		tsdbutil.GenerateTestHistogramWithHint(2, histogram.UnknownCounterReset),
		tsdbutil.GenerateTestHistogramWithHint(4, histogram.UnknownCounterReset),
		tsdbutil.GenerateTestHistogramWithHint(0, histogram.UnknownCounterReset),
	}

	series := newHistogramSeries(histograms)
	it := series.Iterator(nil)

	statsIterator := NewHistogramStatsIterator(it)

	expectedHints := []histogram.CounterResetHint{
		histogram.UnknownCounterReset,
		histogram.NotCounterReset,
		histogram.CounterReset,
	}
	// Note that statsIterator always returns float histograms.
	actualHints := make([]histogram.CounterResetHint, 3)
	typ := statsIterator.Next()
	require.Equal(t, chunkenc.ValFloatHistogram, typ)
	_, h := statsIterator.AtFloatHistogram(nil)
	actualHints[0] = h.CounterResetHint
	typ = statsIterator.Next()
	require.Equal(t, chunkenc.ValFloatHistogram, typ)
	_, h = statsIterator.AtFloatHistogram(nil)
	// We call AtFloatHistogram here again "randomly" to check idempotency.
	_, h2 := statsIterator.AtFloatHistogram(nil)
	require.True(t, h.Equals(h2))
	actualHints[1] = h.CounterResetHint
	typ = statsIterator.Next()
	require.Equal(t, chunkenc.ValFloatHistogram, typ)
	_, fh := statsIterator.AtFloatHistogram(nil)
	actualHints[2] = fh.CounterResetHint

	require.Equal(t, chunkenc.ValNone, statsIterator.Next())
	require.Equal(t, expectedHints, actualHints)
}

type histogramSeries struct {
	histograms []*histogram.Histogram
}

func newHistogramSeries(histograms []*histogram.Histogram) *histogramSeries {
	return &histogramSeries{
		histograms: histograms,
	}
}

func (histogramSeries) Labels() labels.Labels { return labels.EmptyLabels() }

func (m histogramSeries) Iterator(chunkenc.Iterator) chunkenc.Iterator {
	return &histogramIterator{
		i:          -1,
		histograms: m.histograms,
	}
}

type histogramIterator struct {
	i          int
	histograms []*histogram.Histogram
}

func (h *histogramIterator) Next() chunkenc.ValueType {
	h.i++
	if h.i < len(h.histograms) {
		return chunkenc.ValHistogram
	}
	return chunkenc.ValNone
}

func (*histogramIterator) Seek(int64) chunkenc.ValueType { panic("not implemented") }

func (*histogramIterator) At() (int64, float64) { panic("not implemented") }

func (h *histogramIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, h.histograms[h.i]
}

func (h *histogramIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, h.histograms[h.i].ToFloat(nil)
}

func (*histogramIterator) AtT() int64 { return 0 }

func (*histogramIterator) AtST() int64 { return 0 }

func (*histogramIterator) Err() error { return nil }

func TestHistogramStatsChainSeek(t *testing.T) {
	for _, enc := range []chunkenc.Encoding{chunkenc.EncHistogram, chunkenc.EncFloatHistogram} {
		for _, reuse := range []bool{false, true} {
			for _, initialSeek := range []bool{false, true} {
				for _, terminalSeek := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/reuse%t/initialSeek%t/terminalSeek%t", enc, reuse, initialSeek, terminalSeek), func(t *testing.T) {
						var pieces []storage.Series
						for i, ts := range []int64{10, 20} {
							c, err := chunkenc.NewEmptyChunk(enc)
							require.NoError(t, err)
							app, err := c.Appender()
							require.NoError(t, err)
							// Each chunk begins with an unknown reset hint. The second
							// histogram has lower bucket population/count, requiring detection.
							population := uint64(2 - i)
							if enc == chunkenc.EncHistogram {
								_, _, _, err = app.AppendHistogram(nil, 0, ts, &histogram.Histogram{Schema: 0, ZeroCount: 1, ZeroThreshold: 1, Count: population + 1, Sum: 1.5 * float64(population+1), PositiveSpans: []histogram.Span{{Offset: 1, Length: 1}}, PositiveBuckets: []int64{int64(population)}}, false)
							} else {
								_, _, _, err = app.AppendFloatHistogram(nil, 0, ts, &histogram.FloatHistogram{Schema: 0, ZeroCount: 1, ZeroThreshold: 1, Count: float64(population + 1), Sum: 1.5 * float64(population+1), PositiveSpans: []histogram.Span{{Offset: 1, Length: 1}}, PositiveBuckets: []float64{float64(population)}}, false)
							}
							require.NoError(t, err)
							decoded, err := chunkenc.FromData(enc, append([]byte(nil), c.Bytes()...))
							require.NoError(t, err)
							pieces = append(pieces, &storage.SeriesEntry{Lset: labels.FromStrings("series", "a"), SampleIteratorFn: func(chunkenc.Iterator) chunkenc.Iterator { return decoded.Iterator(nil) }})
						}
						merged := storage.ChainedSeriesMerge(pieces...)
						control := merged.Iterator(nil)
						require.NotEqual(t, chunkenc.ValNone, control.Seek(10))
						ts, h := control.AtFloatHistogram(nil)
						require.Equal(t, int64(10), ts)
						require.Equal(t, float64(3), h.Count)
						require.NotEqual(t, chunkenc.ValNone, control.Next())
						require.Equal(t, chunkenc.ValNone, control.Next())
						require.Equal(t, chunkenc.ValNone, control.Seek(30))
						require.NoError(t, control.Err())

						var previous chunkenc.Iterator
						if reuse {
							previous = NewHistogramStatsIterator(pieces[0].Iterator(nil))
							previous.Next()
							previous.AtFloatHistogram(nil)
						}
						wrapped := newHistogramStatsSeries(merged).Iterator(previous)
						operation := "initial positioning"
						defer func() {
							if r := recover(); r != nil {
								t.Fatalf("%s: %v", operation, r)
							}
						}()
						if initialSeek {
							require.Equal(t, chunkenc.ValFloatHistogram, wrapped.Seek(10))
						} else {
							require.Equal(t, chunkenc.ValFloatHistogram, wrapped.Next())
						}
						for i, ts := range []int64{10, 20} {
							if i > 0 {
								require.Equal(t, chunkenc.ValFloatHistogram, wrapped.Next())
							}
							hint := histogram.UnknownCounterReset
							if i > 0 {
								hint = histogram.CounterReset
							}
							want := &histogram.FloatHistogram{Schema: 0, Count: float64(3 - i), Sum: 1.5 * float64(3-i), CounterResetHint: hint}
							for range 2 {
								at, stats := wrapped.AtFloatHistogram(nil)
								require.Equal(t, ts, at)
								require.Equal(t, want, stats)
							}
							// Retaining Seek must preserve the cached reset decision.
							require.Equal(t, chunkenc.ValFloatHistogram, wrapped.Seek(ts))
							at, stats := wrapped.AtFloatHistogram(&histogram.FloatHistogram{})
							require.Equal(t, ts, at)
							require.Equal(t, want, stats)
						}
						operation = "terminal advancement"
						if terminalSeek {
							require.Equal(t, chunkenc.ValNone, wrapped.Seek(30))
						} else {
							require.Equal(t, chunkenc.ValNone, wrapped.Next())
						}
						operation = "Seek after exhaustion"
						for _, target := range []int64{30, 40} {
							require.Equal(t, chunkenc.ValNone, wrapped.Seek(target))
						}
						require.NoError(t, wrapped.Err())
						// Reset an exhausted wrapper, then advance with Seek. A jump
						// discards reset-detection history rather than treating the
						// two observations as consecutive.
						operation = "reset and advancing Seek"
						wrapped = newHistogramStatsSeries(merged).Iterator(wrapped)
						for i, target := range []int64{10, 20} {
							require.Equal(t, chunkenc.ValFloatHistogram, wrapped.Seek(target))
							at, stats := wrapped.AtFloatHistogram(nil)
							require.Equal(t, target, at)
							require.Equal(t, &histogram.FloatHistogram{Schema: 0, Count: float64(3 - i), Sum: 1.5 * float64(3-i), CounterResetHint: histogram.UnknownCounterReset}, stats)
						}
					})
				}
			}
		}
	}
}
