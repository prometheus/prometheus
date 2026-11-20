// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"bytes"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/stats"
)

var testParser = parser.NewParser(parser.Options{})

func TestRecoverEvaluatorRuntime(t *testing.T) {
	var output bytes.Buffer
	logger := promslog.New(&promslog.Config{Writer: &output})
	ev := &evaluator{logger: logger}

	expr, _ := testParser.ParseExpr("sum(up)")

	var err error

	defer func() {
		require.EqualError(t, err, "unexpected error: runtime error: index out of range [123] with length 0")
		require.Contains(t, output.String(), "sum(up)")
	}()
	defer ev.recover(expr, nil, &err)
	// Cause a runtime panic.
	var a []int
	a[123] = 1 //nolint:govet // This is intended to cause a runtime panic.
}

func TestRecoverEvaluatorError(t *testing.T) {
	ev := &evaluator{logger: promslog.NewNopLogger()}
	var err error

	e := errors.New("custom error")

	defer func() {
		require.EqualError(t, err, e.Error())
	}()
	defer ev.recover(nil, nil, &err)

	panic(e)
}

func TestRecoverEvaluatorErrorWithWarnings(t *testing.T) {
	ev := &evaluator{logger: promslog.NewNopLogger()}
	var err error
	var ws annotations.Annotations

	warnings := annotations.New().Add(errors.New("custom warning"))
	e := errWithWarnings{
		err:      errors.New("custom error"),
		warnings: warnings,
	}

	defer func() {
		require.EqualError(t, err, e.Error())
		require.Equal(t, warnings, ws, "wrong warning message")
	}()
	defer ev.recover(nil, &ws, &err)

	panic(e)
}

func TestSubqueryTimeRange(t *testing.T) {
	// noStepInterval returns the interval the engine uses for a subquery without
	// an explicit step ([range:]). This is the configured global evaluation
	// interval (default 1 minute) and does not depend on the subquery range, so
	// the argument is ignored, matching NoStepSubqueryIntervalFn in production.
	const noStepEvalInterval = time.Minute
	noStepInterval := func(int64) int64 { return durationMilliseconds(noStepEvalInterval) }

	const second = 1000

	for _, tc := range []struct {
		name string
		// Parent evaluator (the outer range/instant query) time range.
		parentStart, parentEnd, parentInterval int64
		query                                  string

		wantStart, wantEnd, wantInterval int64
	}{
		{
			// Instant query: a single evaluation step, no end alignment needed.
			// Subquery steps span (1000s-60s, 1000s] on the 5s grid: 945s..1000s.
			name:        "instant query",
			parentStart: 1000 * second, parentEnd: 1000 * second, parentInterval: 0,
			query:     "metric[60s:5s]",
			wantStart: 945 * second, wantEnd: 1000 * second, wantInterval: 5 * second,
		},
		{
			// Range query whose end is already on the 5s step grid
			// (steps at 201s, 206s, 211s, 216s): subquery ends at the parent end.
			name:        "range query, end already step-aligned",
			parentStart: 201 * second, parentEnd: 216 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s]",
			wantStart: 145 * second, wantEnd: 216 * second, wantInterval: 5 * second,
		},
		{
			// Same query but end=220s is 4s past the last aligned step (216s).
			// The subquery must still stop at 216s, matching the aligned case.
			name:        "range query, end past last step is aligned down",
			parentStart: 201 * second, parentEnd: 220 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s]",
			wantStart: 145 * second, wantEnd: 216 * second, wantInterval: 5 * second,
		},
		{
			// Offset shifts both the subquery start and end back by the offset.
			name:        "range query with offset, end aligned down",
			parentStart: 201 * second, parentEnd: 220 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s] offset 30s",
			wantStart: 115 * second, wantEnd: 186 * second, wantInterval: 5 * second,
		},
		{
			// Subquery without an explicit step uses noStepSubqueryIntervalFn,
			// here the 60s default. First aligned start after 201s-100s=101s is
			// 120s; end is aligned down to 216s as above.
			name:        "range query, subquery without explicit step",
			parentStart: 201 * second, parentEnd: 220 * second, parentInterval: 5 * second,
			query:     "metric[100s:]",
			wantStart: 120 * second, wantEnd: 216 * second, wantInterval: 60 * second,
		},
		{
			// For a non-negative boundary the truncated-down multiple always
			// lands on or below the boundary, so the +interval guard fires here
			// (140s -> 145s) as in every case above. The only way to skip the
			// guard is a negative boundary, exercised in the next case.
			name:        "range query with offset, start bumped onto the grid",
			parentStart: 201 * second, parentEnd: 216 * second, parentInterval: 5 * second,
			query:     "metric[60s:5s] offset 1s",
			wantStart: 145 * second, wantEnd: 215 * second, wantInterval: 5 * second,
		},
		{
			// offset+range exceeds the parent start, so the boundary
			// (30s-61s=-31s) is negative. Integer division truncates toward
			// zero, yielding -30s, which is already strictly past -31s, so the
			// +interval guard is NOT taken. Guards the negative-timestamp path.
			name:        "instant query, negative boundary skips the start bump",
			parentStart: 30 * second, parentEnd: 30 * second, parentInterval: 0,
			query:     "metric[61s:5s]",
			wantStart: -30 * second, wantEnd: 30 * second, wantInterval: 5 * second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := testParser.ParseExpr(tc.query)
			require.NoError(t, err)
			// Mirror the engine's query setup: PreprocessExpr resolves
			// duration expressions, then setOffsetForAtModifier resolves
			// OriginalOffset into the Offset the evaluator uses.
			step := time.Duration(tc.parentInterval) * time.Millisecond
			expr, err = PreprocessExpr(expr, timestamp.Time(tc.parentStart), timestamp.Time(tc.parentEnd), step)
			require.NoError(t, err)
			setOffsetForAtModifier(tc.parentStart, expr)
			sq, ok := expr.(*parser.SubqueryExpr)
			require.True(t, ok, "query must parse to a subquery expression")

			ev := &evaluator{
				startTimestamp:           tc.parentStart,
				endTimestamp:             tc.parentEnd,
				interval:                 tc.parentInterval,
				samplesStats:             stats.NewQuerySamples(false),
				noStepSubqueryIntervalFn: noStepInterval,
			}

			start, end, interval := ev.subqueryTimeRange(sq)

			require.Equal(t, tc.wantStart, start, "subquery start timestamp")
			require.Equal(t, tc.wantEnd, end, "subquery end timestamp")
			require.Equal(t, tc.wantInterval, interval, "subquery interval")
		})
	}
}

func TestVectorElemBinop_Histograms(t *testing.T) {
	testCases := []struct {
		name       string
		op         parser.ItemType
		hlhs, hrhs *histogram.FloatHistogram
		want       *histogram.FloatHistogram
		wantErr    error
		wantInfo   error
	}{
		{
			name: "ADD with unknown counter reset hints",
			op:   parser.ADD,
			hlhs: &histogram.FloatHistogram{},
			hrhs: &histogram.FloatHistogram{},
			want: &histogram.FloatHistogram{},
		},
		{
			name: "ADD with mixed counter reset hints",
			op:   parser.ADD,
			hlhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.CounterReset,
			},
			hrhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.NotCounterReset,
			},
			want:    &histogram.FloatHistogram{},
			wantErr: annotations.HistogramCounterResetCollisionWarning,
		},
		{
			name: "SUB with unknown counter reset hints",
			op:   parser.SUB,
			hlhs: &histogram.FloatHistogram{},
			hrhs: &histogram.FloatHistogram{},
			want: &histogram.FloatHistogram{
				CounterResetHint: histogram.GaugeType,
			},
		},
		{
			name: "SUB with mixed counter reset hints",
			op:   parser.SUB,
			hlhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.CounterReset,
			},
			hrhs: &histogram.FloatHistogram{
				CounterResetHint: histogram.NotCounterReset,
			},
			want:    &histogram.FloatHistogram{},
			wantErr: annotations.HistogramCounterResetCollisionWarning,
		},
		{
			name:     "ADD with mismatched NHCB bounds",
			op:       parser.ADD,
			hlhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{1}},
			hrhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{2}},
			want:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}},
			wantInfo: annotations.MismatchedCustomBucketsHistogramsInfo,
		},
		{
			name:     "SUB with mismatched NHCB bounds",
			op:       parser.SUB,
			hlhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{1}},
			hrhs:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CustomValues: []float64{2}},
			want:     &histogram.FloatHistogram{Schema: histogram.CustomBucketsSchema, PositiveBuckets: []float64{}, CounterResetHint: histogram.GaugeType},
			wantInfo: annotations.MismatchedCustomBucketsHistogramsInfo,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, _, info, err := vectorElemBinop(tc.op, 0, 0, tc.hlhs, tc.hrhs, posrange.PositionRange{})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)

			if tc.wantInfo != nil {
				require.ErrorIs(t, info, tc.wantInfo)
			} else {
				require.NoError(t, info)
			}

			require.Equal(t, tc.want, got)
		})
	}
}

func TestCostFallbackSampleWindow(t *testing.T) {
	for _, tc := range []struct {
		name      string
		end       int64
		interval  time.Duration
		wantStart int64
	}{
		{name: "default", end: 0, wantStart: -300000},
		{name: "negative interval", end: 0, interval: -time.Second, wantStart: -300000},
		{name: "minimum duration", end: 0, interval: time.Duration(math.MinInt64), wantStart: -300000},
		{name: "eight intervals", end: 0, interval: time.Minute, wantStart: -480000},
		{name: "maximum duration", end: 0, interval: time.Duration(math.MaxInt64), wantStart: -1800000},
		{name: "timestamp underflow", end: math.MinInt64 + 1, interval: time.Minute, wantStart: math.MinInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end := fallbackSampleWindow(tc.end, tc.interval)
			require.Equal(t, tc.wantStart, start)
			require.Equal(t, tc.end, end)
		})
	}
}

type matrixFloatWindowTestSample struct {
	timestamp      int64
	startTimestamp int64
	value          float64
	stale          bool
}

func (s matrixFloatWindowTestSample) T() int64  { return s.timestamp }
func (s matrixFloatWindowTestSample) ST() int64 { return s.startTimestamp }
func (s matrixFloatWindowTestSample) F() float64 {
	if s.stale {
		return math.Float64frombits(value.StaleNaN)
	}
	return s.value
}
func (matrixFloatWindowTestSample) H() *histogram.Histogram       { return nil }
func (matrixFloatWindowTestSample) FH() *histogram.FloatHistogram { return nil }
func (matrixFloatWindowTestSample) Type() chunkenc.ValueType      { return chunkenc.ValFloat }
func (s matrixFloatWindowTestSample) Copy() chunks.Sample         { return s }

func matrixFloatWindowTestSamples(end int64, gap bool) []chunks.Sample {
	var samples []chunks.Sample
	for ts := int64(0); ts <= end; ts++ {
		if gap && ts >= 9 && ts < 14 {
			continue
		}
		samples = append(samples, matrixFloatWindowTestSample{
			timestamp:      ts,
			startTimestamp: 1000 + 10*ts,
			value:          float64(ts),
			stale:          ts == 5,
		})
	}
	return samples
}

func TestMatrixIterSliceFloatWindow(t *testing.T) {
	for _, tc := range []struct {
		name string
		gap  bool
		end  int64
	}{
		{name: "overlap compaction", end: 24},
		{name: "gap resets the window", gap: true, end: 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			budget := &queryBudget{maxSamplesRead: 23}
			ev := &evaluator{maxSamples: 4, budget: budget, samplesStats: stats.NewQuerySamples(true)}
			ev.samplesStats.InitStepTracking(0, 20, 1)
			it := ev.newMatrixIterator(ctx, 4)
			samples := matrixFloatWindowTestSamples(tc.end, tc.gap)
			series := storage.NewListSeries(labels.EmptyLabels(), samples)
			it.Reset(series.Iterator(nil))

			window := matrixFloatWindow{
				floats:          make([]FPoint, 0, 8),
				startTimestamps: make([]int64, 0, 8),
			}
			startTimestamps := &StartTimestamps{Floats: window.startTimestamps}
			var floats []FPoint
			compactions := 0
			for step := int64(0); step <= 20; step++ {
				beforeHead, beforeLen := window.head, len(window.floats)
				floats, _, startTimestamps = ev.matrixIterSlice(ctx, it, step, step+4, floats, nil, startTimestamps, int(step), &window)
				points := window.points()
				wantPoints := make([]FPoint, 0, 4)
				wantStartTimestamps := make([]int64, 0, 4)
				for _, sample := range samples {
					if sample.T() > step && sample.T() <= step+4 && !value.IsStaleNaN(sample.F()) {
						wantPoints = append(wantPoints, FPoint{T: sample.T(), F: sample.F()})
						wantStartTimestamps = append(wantStartTimestamps, sample.ST())
					}
				}
				require.Equal(t, wantPoints, points)
				require.Equal(t, wantPoints, floats)
				require.Equal(t, wantStartTimestamps, startTimestamps.Floats)
				require.Equal(t, len(points), ev.currentSamples)
				if tc.gap && len(points) == 0 {
					require.Zero(t, window.head)
					require.Empty(t, window.floats)
					require.Empty(t, startTimestamps.Floats)
				}
				if !tc.gap && beforeHead > 0 && len(window.floats) < beforeLen {
					compactions++
				}
				if len(points) > 0 {
					it.ReduceDelta(1)
				}
			}
			if !tc.gap {
				require.Positive(t, compactions)
				require.Equal(t, 8, cap(window.floats))
				require.Equal(t, 8, cap(window.startTimestamps))
				require.Equal(t, int64(23), budget.samplesRead)
				require.Equal(t, int64(23), ev.samplesStats.SamplesRead)
				wantReadsPerStep := make([]int64, 21)
				wantReadsPerStep[0] = 4
				for step := 2; step < len(wantReadsPerStep); step++ {
					wantReadsPerStep[step] = 1
				}
				require.Equal(t, wantReadsPerStep, ev.samplesStats.SamplesReadPerStep)
			}
			require.Equal(t, 4, ev.samplesStats.PeakSamples)
		})
	}
}

func TestMatrixIterSliceFloatWindowCompactsBeforeAppend(t *testing.T) {
	for _, tc := range []struct {
		name     string
		floatCap int
		startCap int
		maxt     int64
	}{
		{name: "without start timestamps", floatCap: 8, maxt: 10},
		{name: "float tail fills first", floatCap: 8, startCap: 16, maxt: 10},
		{name: "start timestamp tail fills first", floatCap: 16, startCap: 8, maxt: 10},
		{name: "buffered float tail fills first", floatCap: 8, startCap: 16, maxt: 11},
		{name: "buffered start timestamp tail fills first", floatCap: 16, startCap: 8, maxt: 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			ev := &evaluator{maxSamples: 100, samplesStats: stats.NewQuerySamples(true)}
			ev.samplesStats.InitStepTracking(0, 0, 1)
			it := ev.newMatrixIterator(ctx, 10)
			samples := matrixFloatWindowTestSamples(10, false)
			series := storage.NewListSeries(labels.EmptyLabels(), samples)
			it.Reset(series.Iterator(nil))

			window := matrixFloatWindow{floats: make([]FPoint, 0, tc.floatCap)}
			var startTimestamps *StartTimestamps
			if tc.startCap > 0 {
				window.startTimestamps = make([]int64, 0, tc.startCap)
				startTimestamps = &StartTimestamps{Floats: window.startTimestamps}
			}
			var floats []FPoint
			floats, _, startTimestamps = ev.matrixIterSlice(ctx, it, 0, 6, floats, nil, startTimestamps, 0, &window)
			it.ReduceDelta(4)
			floats, _, startTimestamps = ev.matrixIterSlice(ctx, it, 1, tc.maxt, floats, nil, startTimestamps, 0, &window)

			wantFloats := []FPoint{
				{T: 2, F: 2},
				{T: 3, F: 3},
				{T: 4, F: 4},
				{T: 6, F: 6},
				{T: 7, F: 7},
				{T: 8, F: 8},
				{T: 9, F: 9},
				{T: 10, F: 10},
			}
			wantStarts := []int64{1020, 1030, 1040, 1060, 1070, 1080, 1090, 1100}
			require.Equal(t, wantFloats, floats)
			require.Equal(t, wantFloats, window.points())
			if startTimestamps != nil {
				require.Equal(t, wantStarts, startTimestamps.Floats)
				require.Equal(t, wantStarts, window.startTimestamps)
			}
			require.Zero(t, window.head)
			require.Equal(t, tc.floatCap, cap(window.floats), "compact before growing the tail view")
			if startTimestamps != nil {
				require.Equal(t, tc.startCap, cap(window.startTimestamps))
			}
		})
	}
}

func TestMatrixFloatWindowStartTimestamps(t *testing.T) {
	var series []storage.Series
	for i := range 3 {
		var samples []chunks.Sample
		for ts := int64(0); ts <= 700; ts++ {
			// Let one series empty its window and later activate it again.
			if i == 1 && ts >= 250 && ts < 450 {
				continue
			}
			samples = append(samples, matrixFloatWindowTestSample{
				timestamp:      ts * 1000,
				startTimestamp: (ts / 70 * 70) * 1000,
				// Increasing values make resets visible only through STs.
				value: float64(1000*(i+1)) + float64(ts),
			})
		}
		series = append(series, storage.NewListSeries(labels.FromStrings("__name__", "window_total", "series", strconv.Itoa(i)), samples))
	}
	eval := func(start, end int64, useST bool) Matrix {
		expr, err := testParser.ParseExpr("rate(window_total[160s])")
		require.NoError(t, err)
		expr.(*parser.Call).Args[0].(*parser.MatrixSelector).VectorSelector.(*parser.VectorSelector).Series = series
		ev := &evaluator{
			startTimestamp: start, endTimestamp: end, interval: 3000,
			maxSamples: 100000, samplesStats: stats.NewQuerySamples(false),
			useStartTimestamps: useST,
		}
		result, warnings := ev.eval(t.Context(), expr)
		require.Empty(t, warnings)
		matrix := result.(Matrix)
		t.Cleanup(func() {
			for _, s := range matrix {
				putFPointSlice(s.Floats)
			}
		})
		return matrix
	}
	got := eval(160000, 700000, true)
	require.Len(t, got, len(series))
	for ts := int64(160000); ts <= 700000; ts += 3000 {
		// A single-step evaluation cannot activate the sliding window. Compare
		// every value, including after gaps and across the series reset.
		want := eval(ts, ts, true)
		for _, s := range got {
			var expected []FPoint
			for _, w := range want {
				if labels.Equal(s.Metric, w.Metric) {
					expected = w.Floats
				}
			}
			var actual []FPoint
			for _, p := range s.Floats {
				if p.T == ts {
					actual = append(actual, p)
				}
			}
			require.Equal(t, expected, actual, "series %s at %d", s.Metric, ts)
		}
	}
	// Ensure that ignoring STs would not silently satisfy this comparison.
	require.NotEqual(t, eval(160000, 700000, false), got)
}
