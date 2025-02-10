// Copyright 2015 The Prometheus Authors
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
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
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
			name: "unknown counter reset triggers detection",
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
				histogram.NotCounterReset,
			},
		},
		{
			name: "detect real counter reset",
			histograms: []*histogram.Histogram{
				tsdbutil.GenerateTestHistogramWithHint(2, histogram.UnknownCounterReset),
				tsdbutil.GenerateTestHistogramWithHint(1, histogram.UnknownCounterReset),
			},
			expectedHints: []histogram.CounterResetHint{
				histogram.NotCounterReset,
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
				histogram.NotCounterReset,
				histogram.UnknownCounterReset,
				histogram.CounterReset,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("histogram_stats", func(t *testing.T) {
				decodedStats := make([]*histogram.Histogram, 0)
				statsIterator := NewHistogramStatsIterator(newHistogramSeries(tc.histograms).Iterator(nil))
				for statsIterator.Next() != chunkenc.ValNone {
					_, h := statsIterator.AtHistogram(nil)
					decodedStats = append(decodedStats, h)
				}
				for i := 0; i < len(tc.histograms); i++ {
					require.Equalf(t, tc.expectedHints[i], decodedStats[i].CounterResetHint, "mismatch in counter reset hint for histogram %d", i)
					h := tc.histograms[i]
					if value.IsStaleNaN(h.Sum) {
						require.True(t, value.IsStaleNaN(decodedStats[i].Sum))
						require.Equal(t, uint64(0), decodedStats[i].Count)
					} else {
						require.Equal(t, tc.histograms[i].Count, decodedStats[i].Count)
						require.Equal(t, tc.histograms[i].Sum, decodedStats[i].Sum)
					}
				}
			})
			t.Run("float_histogram_stats", func(t *testing.T) {
				decodedStats := make([]*histogram.FloatHistogram, 0)
				statsIterator := NewHistogramStatsIterator(newHistogramSeries(tc.histograms).Iterator(nil))
				for statsIterator.Next() != chunkenc.ValNone {
					_, h := statsIterator.AtFloatHistogram(nil)
					decodedStats = append(decodedStats, h)
				}
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
			})
		})
	}
}

type histogramSeries struct {
	histograms []*histogram.Histogram
}

func newHistogramSeries(histograms []*histogram.Histogram) *histogramSeries {
	return &histogramSeries{
		histograms: histograms,
	}
}

func (m histogramSeries) Labels() labels.Labels { return labels.EmptyLabels() }

func (m histogramSeries) Iterator(_ chunkenc.Iterator) chunkenc.Iterator {
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

func (h *histogramIterator) Seek(_ int64) chunkenc.ValueType { panic("not implemented") }

func (h *histogramIterator) At() (int64, float64) { panic("not implemented") }

func (h *histogramIterator) AtHistogram(_ *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, h.histograms[h.i]
}

func (h *histogramIterator) AtFloatHistogram(_ *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, h.histograms[h.i].ToFloat(nil)
}

func (h *histogramIterator) AtT() int64 { return 0 }

func (h *histogramIterator) Err() error { return nil }
