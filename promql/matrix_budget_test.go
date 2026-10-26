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
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/stats"
)

type matrixIteratorTestSample struct {
	timestamp  int64
	floatValue float64
	histogram  *histogram.FloatHistogram
}

type matrixIteratorTestWindow struct {
	name       string
	mint, maxt int64
}

func (s matrixIteratorTestSample) T() int64                      { return s.timestamp }
func (matrixIteratorTestSample) ST() int64                       { return 0 }
func (s matrixIteratorTestSample) F() float64                    { return s.floatValue }
func (matrixIteratorTestSample) H() *histogram.Histogram         { return nil }
func (s matrixIteratorTestSample) FH() *histogram.FloatHistogram { return s.histogram }
func (s matrixIteratorTestSample) Type() chunkenc.ValueType {
	if s.histogram != nil {
		return chunkenc.ValFloatHistogram
	}
	return chunkenc.ValFloat
}

func (s matrixIteratorTestSample) Copy() chunks.Sample {
	if s.histogram != nil {
		s.histogram = s.histogram.Copy()
	}
	return s
}

func matrixIteratorTestHistogramSample(timestamp int64, bucketCount int) chunks.Sample {
	buckets := make([]float64, bucketCount)
	for i := range buckets {
		buckets[i] = 1
	}
	return matrixIteratorTestSample{
		timestamp: timestamp,
		histogram: &histogram.FloatHistogram{
			CounterResetHint: histogram.GaugeType,
			Schema:           0,
			Count:            float64(bucketCount),
			Sum:              float64(bucketCount),
			PositiveSpans:    []histogram.Span{{Offset: 0, Length: uint32(bucketCount)}},
			PositiveBuckets:  buckets,
		},
	}
}

func TestMatrixIteratorWindowHistogramSamples(t *testing.T) {
	ctx := context.Background()
	series := storage.NewListSeries(labels.EmptyLabels(), []chunks.Sample{
		matrixIteratorTestHistogramSample(0, 1),
		matrixIteratorTestHistogramSample(10_000, 2),
		matrixIteratorTestHistogramSample(20_000, 3),
		matrixIteratorTestSample{timestamp: 25_000, floatValue: 25},
		matrixIteratorTestHistogramSample(30_000, 4),
		matrixIteratorTestSample{
			timestamp: 50_000,
			histogram: &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)},
		},
		matrixIteratorTestHistogramSample(60_000, 1),
	})
	windows := []matrixIteratorTestWindow{
		{name: "first", mint: -20_000, maxt: 0},
		{name: "overlap", mint: -10_000, maxt: 10_000},
		{name: "changed size and slot reuse", mint: 0, maxt: 20_000},
		{name: "mixed float and histograms", mint: 10_000, maxt: 30_000},
		{name: "retained float", mint: 20_000, maxt: 40_000},
		{name: "stale-only empty window", mint: 30_000, maxt: 50_000},
		{name: "new window after empty", mint: 40_000, maxt: 60_000},
	}

	evaluateWindows := func(t *testing.T, ev *evaluator, it *matrixIterator, s storage.Series, windows []matrixIteratorTestWindow) ([]FPoint, []HPoint) {
		t.Helper()
		it.Reset(s.Iterator(nil))
		var floats []FPoint
		var histograms []HPoint
		for _, window := range windows {
			floats, histograms, _ = ev.matrixIterSlice(ctx, it, window.mint, window.maxt, floats, histograms, nil, -1)
			histogramSamples := totalHPointSize(histograms)
			require.Equal(t, histogramSamples, it.windowHistogramSamples, window.name)
			require.Equal(t, len(floats)+histogramSamples, ev.currentSamples, window.name)
		}
		return floats, histograms
	}

	ev := &evaluator{maxSamples: 1_000, samplesStats: stats.NewQuerySamples(false)}
	it := ev.newMatrixIterator(ctx, 20_000)
	floats, histograms := evaluateWindows(t, ev, it, series, windows)

	// A new series reuses the same iterator and the dropped point's buffer slot.
	ev.currentSamples -= len(floats) + it.windowHistogramSamples
	floats, histograms = floats[:0], histograms[:0]
	secondSeries := storage.NewListSeries(labels.FromStrings("series", "second"), []chunks.Sample{
		matrixIteratorTestHistogramSample(0, 8),
	})
	it.Reset(secondSeries.Iterator(nil))
	require.Zero(t, it.windowHistogramSamples)
	floats, histograms, _ = ev.matrixIterSlice(ctx, it, -20_000, 0, floats, histograms, nil, -1)
	secondSeriesHistogramSamples := totalHPointSize(histograms)
	require.Equal(t, secondSeriesHistogramSamples, it.windowHistogramSamples)
	require.Equal(t, len(floats)+secondSeriesHistogramSamples, ev.currentSamples)

	t.Run("non-overlapping windows", func(t *testing.T) {
		isolatedEvaluator := &evaluator{maxSamples: 1_000, samplesStats: stats.NewQuerySamples(false)}
		isolatedIterator := isolatedEvaluator.newMatrixIterator(ctx, 20_000)
		nonOverlappingWindows := []matrixIteratorTestWindow{
			{name: "first", mint: -20_000, maxt: 0},
			{name: "gap", mint: 10_000, maxt: 30_000},
			{name: "after gap", mint: 40_000, maxt: 60_000},
		}
		evaluateWindows(t, isolatedEvaluator, isolatedIterator, series, nonOverlappingWindows)
	})
}

func TestMatrixIterator_checkContext_CancellationAccounting(t *testing.T) {
	for _, quota := range []int64{0, 30} {
		for _, billed := range []int{0, 7, 20} {
			for _, step := range []int{0, -1} {
				t.Run(fmt.Sprintf("quota=%d/billed=%d/step=%d", quota, billed, step), func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					ev := &evaluator{budget: &queryBudget{maxSamplesRead: quota}, samplesStats: stats.NewQuerySamples(true)}
					ev.samplesStats.InitStepTracking(0, 0, 1)
					ev.consumeMatrixSamples(10, 0) // A previous selector's cost.
					it := ev.newMatrixIterator(ctx, 30)
					series := storage.NewListSeries(labels.EmptyLabels(), chunks.GenerateSamples(0, 21))
					it.Reset(series.Iterator(nil))
					it.beginFrame(ctx, step)
					require.Equal(t, chunkenc.ValFloat, it.seek(-1, -1, 20))
					// Charge actual buffered consumption, then interrupt the drain.
					buf := it.Buffer()
					for range billed {
						require.Equal(t, chunkenc.ValFloat, buf.Next())
						_, _ = buf.At()
						ev.consumeMatrixSamples(1, step)
					}
					cancel()
					require.PanicsWithValue(t, ErrQueryCanceled(env), it.checkContext)
					want := int64(30)
					if step < 0 {
						want = 10
					}
					require.Equal(t, want, ev.budget.samplesRead, "commit only unbilled storage input")
					require.Equal(t, want, ev.samplesStats.SamplesRead)
					require.Equal(t, []int64{want}, ev.samplesStats.SamplesReadPerStep)
				})
			}
		}
	}
}
