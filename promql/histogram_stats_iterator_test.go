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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestHistogramStatsDecoding(t *testing.T) {
	histograms := []*histogram.Histogram{
		tsdbutil.GenerateTestHistogram(0),
		tsdbutil.GenerateTestHistogram(1),
		tsdbutil.GenerateTestHistogram(2),
		tsdbutil.GenerateTestHistogram(2),
	}
	histograms[0].CounterResetHint = histogram.NotCounterReset
	histograms[1].CounterResetHint = histogram.UnknownCounterReset
	histograms[2].CounterResetHint = histogram.CounterReset
	histograms[3].CounterResetHint = histogram.UnknownCounterReset

	expectedHints := []histogram.CounterResetHint{
		histogram.NotCounterReset,
		histogram.NotCounterReset,
		histogram.CounterReset,
		histogram.NotCounterReset,
	}

	t.Run("histogram_stats", func(t *testing.T) {
		decodedStats := make([]*histogram.Histogram, 0)
		statsIterator := NewHistogramStatsIterator(newHistogramSeries(histograms).Iterator(nil))
		for statsIterator.Next() != chunkenc.ValNone {
			_, h := statsIterator.AtHistogram(nil)
			decodedStats = append(decodedStats, h)
		}
		for i := 0; i < len(histograms); i++ {
			require.Equal(t, expectedHints[i], decodedStats[i].CounterResetHint)
			require.Equal(t, histograms[i].Count, decodedStats[i].Count)
			require.Equal(t, histograms[i].Sum, decodedStats[i].Sum)
		}
	})
	t.Run("float_histogram_stats", func(t *testing.T) {
		decodedStats := make([]*histogram.FloatHistogram, 0)
		statsIterator := NewHistogramStatsIterator(newHistogramSeries(histograms).Iterator(nil))
		for statsIterator.Next() != chunkenc.ValNone {
			_, h := statsIterator.AtFloatHistogram(nil)
			decodedStats = append(decodedStats, h)
		}
		for i := 0; i < len(histograms); i++ {
			fh := histograms[i].ToFloat(nil)
			require.Equal(t, expectedHints[i], decodedStats[i].CounterResetHint)
			require.Equal(t, fh.Count, decodedStats[i].Count)
			require.Equal(t, fh.Sum, decodedStats[i].Sum)
		}
	})
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

func (h *histogramIterator) Seek(t int64) chunkenc.ValueType { panic("not implemented") }

func (h *histogramIterator) At() (int64, float64) { panic("not implemented") }

func (h *histogramIterator) AtHistogram(_ *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, h.histograms[h.i]
}

func (h *histogramIterator) AtFloatHistogram(_ *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, h.histograms[h.i].ToFloat(nil)
}

func (h *histogramIterator) AtT() int64 { return 0 }

func (h *histogramIterator) Err() error { return nil }
