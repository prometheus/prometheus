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

package promql_test

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestDeriv(t *testing.T) {
	// https://github.com/prometheus/prometheus/issues/2674#issuecomment-315439393
	// This requires more precision than the usual test system offers,
	// so we test it by hand.
	stor := teststorage.New(t)

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10000,
		Timeout:    10 * time.Second,
	}
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	a := stor.AppenderV2(context.Background())

	var start, interval, i int64
	metric := labels.FromStrings("__name__", "foo")
	start = 1493712816939
	interval = 30 * 1000
	// Introduce some timestamp jitter to test 0 slope case.
	// https://github.com/prometheus/prometheus/issues/7180
	for i = range int64(15) {
		jitter := 12 * i % 2
		_, err := a.Append(0, metric, 0, start+interval*i+jitter, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}

	require.NoError(t, a.Commit())

	ctx := context.Background()
	query, err := engine.NewInstantQuery(ctx, stor, nil, "deriv(foo[30m])", timestamp.Time(1493712846939))
	require.NoError(t, err)

	result := query.Exec(ctx)
	require.NoError(t, result.Err)

	vec, _ := result.Vector()
	require.Len(t, vec, 1, "Expected 1 result, got %d", len(vec))
	require.Equal(t, 0.0, vec[0].F, "Expected 0.0 as value, got %f", vec[0].F)
}

func TestTsOfLastOverTimeBeforeEpoch(t *testing.T) {
	ctx := context.Background()
	stor := teststorage.New(t)
	app := stor.AppenderV2(ctx)

	_, err := app.Append(0, labels.FromStrings("__name__", "float_metric"), 0, -1000, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("__name__", "histogram_metric"), 0, -1000, 0, nil, tsdbutil.GenerateTestFloatHistogram(0), storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	engine := promqltest.NewTestEngine(t, false, 0, 10000)
	for _, metric := range []string{"float_metric", "histogram_metric"} {
		query, err := engine.NewInstantQuery(ctx, stor, nil, "ts_of_last_over_time("+metric+"[1s])", timestamp.Time(-1000))
		require.NoError(t, err)
		result := query.Exec(ctx)
		require.NoError(t, result.Err)
		vector, _ := result.Vector()
		require.Len(t, vector, 1)
		require.Equal(t, -1.0, vector[0].F, metric)
	}
}

func TestFunctionList(t *testing.T) {
	// Test that Functions and parser.Functions list the same functions.
	for i := range promql.FunctionCalls {
		_, ok := parser.Functions[i]
		require.True(t, ok, "function %s exists in promql package, but not in parser package", i)
	}

	for i := range parser.Functions {
		_, ok := promql.FunctionCalls[i]
		require.True(t, ok, "function %s exists in parser package, but not in promql package", i)
	}
}

func TestStartTimestampOutputWhenUseStartTimestampIsDisabled(t *testing.T) {
	stor := teststorage.New(t, func(opts *tsdb.Options) {
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
	})

	a := stor.AppenderV2(t.Context())

	for i := range int64(5) {
		inputLabel := labels.FromStrings(model.MetricNameLabel, "some_series", "case", strconv.Itoa(int(i)))
		var (
			ts = i * 1000
			st = ts - i*100
		)
		_, err := a.Append(0, inputLabel, st, ts, 0, nil, nil, storage.AppendV2Options{})
		require.NoError(t, err)
	}
	require.NoError(t, a.Commit())

	tests := []struct {
		name               string
		useStartTimestamps bool
		expected           []float64
	}{
		{
			name:               "use-start-timestamps enabled",
			useStartTimestamps: true,
			expected:           []float64{0, 0.9, 1.8, 2.7, 3.6},
		},
		{
			name:               "use-start-timestamps disabled",
			useStartTimestamps: false,
			expected:           []float64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := promql.EngineOpts{
				MaxSamples:         10000,
				Timeout:            10 * time.Second,
				UseStartTimestamps: tt.useStartTimestamps,
				Parser:             parser.NewParser(promqltest.TestParserOpts),
			}
			engine := promqltest.NewTestEngineWithOpts(t, opts)

			query, err := engine.NewInstantQuery(t.Context(), stor, nil, "start_timestamp(some_series)", timestamp.Time(5000))
			require.NoError(t, err)

			result := query.Exec(t.Context())
			require.NoError(t, result.Err)

			vec, _ := result.Vector()
			require.Len(t, vec, len(tt.expected), "Unexpected number of results, got %d", len(vec))
			for i := range tt.expected {
				require.Equal(t, tt.expected[i], vec[i].F, "At index %d", i)
			}
		})
	}
}

func TestQuantileInterpolationPreservesExactValues(t *testing.T) {
	smallestSubnormal := math.SmallestNonzeroFloat64
	largestSubnormal := math.Nextafter(math.Ldexp(1, -1022), 0)
	smallestNormal := math.Ldexp(1, -1022)
	negativeZero := math.Copysign(0, -1)
	maxFloat := math.MaxFloat64
	previousMaxFloat := math.Nextafter(maxFloat, 0)

	testCases := []struct {
		name     string
		values   []float64
		want     float64
		constant bool
	}{
		{name: "smallest positive subnormal", values: []float64{smallestSubnormal, smallestSubnormal}, want: smallestSubnormal, constant: true},
		{name: "three positive subnormal units", values: []float64{math.Float64frombits(3), math.Float64frombits(3)}, want: math.Float64frombits(3), constant: true},
		{name: "smallest negative subnormal", values: []float64{-smallestSubnormal, -smallestSubnormal}, want: -smallestSubnormal, constant: true},
		{name: "three negative subnormal units", values: []float64{-math.Float64frombits(3), -math.Float64frombits(3)}, want: -math.Float64frombits(3), constant: true},
		{name: "largest positive subnormal", values: []float64{largestSubnormal, largestSubnormal}, want: largestSubnormal, constant: true},
		{name: "largest negative subnormal", values: []float64{-largestSubnormal, -largestSubnormal}, want: -largestSubnormal, constant: true},
		{name: "smallest positive normal", values: []float64{smallestNormal, smallestNormal}, want: smallestNormal, constant: true},
		{name: "smallest negative normal", values: []float64{-smallestNormal, -smallestNormal}, want: -smallestNormal, constant: true},
		{name: "ordinary normal", values: []float64{3, 3}, want: 3, constant: true},
		{name: "positive zero", values: []float64{0, 0}, want: 0, constant: true},
		{name: "negative zero", values: []float64{negativeZero, negativeZero}, want: negativeZero, constant: true},
		{name: "mixed signed zero", values: []float64{negativeZero, 0}, want: 0},
		{name: "equal positive infinity", values: []float64{math.Inf(1), math.Inf(1)}, want: math.Inf(1)},
		{name: "equal negative infinity", values: []float64{math.Inf(-1), math.Inf(-1)}, want: math.Inf(-1)},
		{name: "mixed infinities", values: []float64{math.Inf(-1), math.Inf(1)}, want: math.NaN()},
		{name: "equal NaNs", values: []float64{math.NaN(), math.NaN()}, want: math.NaN()},
		{name: "NaN and finite value", values: []float64{math.NaN(), 1}, want: math.NaN()},
		{name: "opposite finite extremes", values: []float64{-maxFloat, maxFloat}, want: 0},
		{name: "adjacent finite values near maximum", values: []float64{previousMaxFloat, maxFloat}, want: previousMaxFloat},
	}

	stor := teststorage.New(t)
	app := stor.AppenderV2(t.Context())
	for _, tc := range testCases {
		scope := "controls"
		if tc.constant {
			scope = "constants"
		}
		rangeLabels := labels.FromStrings("__name__", "quantile_range", "case", tc.name, "scope", scope)
		for i, value := range tc.values {
			_, err := app.Append(0, rangeLabels, 0, int64(i*1000), value, nil, nil, storage.AppendV2Options{})
			require.NoError(t, err)

			aggregationLabels := labels.FromStrings("__name__", "quantile_aggregation", "case", tc.name, "scope", scope, "replica", strconv.Itoa(i))
			_, err = app.Append(0, aggregationLabels, 0, 1000, value, nil, nil, storage.AppendV2Options{})
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())

	engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)
	checkQuery := func(expr string, expected map[string]float64, warning bool) {
		t.Helper()
		query, err := engine.NewInstantQuery(t.Context(), stor, nil, expr, timestamp.Time(1000))
		require.NoError(t, err)
		defer query.Close()

		result := query.Exec(t.Context())
		require.NoError(t, result.Err)
		if warning {
			require.NotEmpty(t, result.Warnings)
		} else {
			require.Empty(t, result.Warnings)
		}
		vector, err := result.Vector()
		require.NoError(t, err)
		require.Len(t, vector, len(expected))
		seen := make(map[string]struct{}, len(expected))
		for _, sample := range vector {
			name := sample.Metric.Get("case")
			want, ok := expected[name]
			require.True(t, ok, "unexpected result for case %q", name)
			_, duplicate := seen[name]
			require.False(t, duplicate, "duplicate result for case %q", name)
			seen[name] = struct{}{}
			if math.IsNaN(want) {
				require.True(t, math.IsNaN(sample.F), "case %q: expected NaN, got %v", name, sample.F)
				continue
			}
			require.Equal(t, math.Float64bits(want), math.Float64bits(sample.F), "case %q", name)
		}
		require.Len(t, seen, len(expected))
	}

	constantValues := make(map[string]float64)
	allValues := make(map[string]float64)
	for _, tc := range testCases {
		allValues[tc.name] = tc.want
		if tc.constant {
			constantValues[tc.name] = tc.want
		}
	}
	ordinaryValues := map[string]float64{"ordinary normal": 3}
	checkQuery("quantile_over_time(0.2, quantile_range{case=\"ordinary normal\"}[2s])", ordinaryValues, false)
	checkQuery("quantile by (case) (0.2, quantile_aggregation{case=\"ordinary normal\"})", ordinaryValues, false)

	for _, q := range []string{"0.25", "0.5", "0.75"} {
		checkQuery("quantile_over_time("+q+", quantile_range{scope=\"constants\"}[2s])", constantValues, false)
		checkQuery("quantile by (case) ("+q+", quantile_aggregation{scope=\"constants\"})", constantValues, false)
	}
	checkQuery("quantile_over_time(0.5, quantile_range[2s])", allValues, false)
	checkQuery("quantile by (case) (0.5, quantile_aggregation)", allValues, false)

	for _, tc := range []struct {
		q    string
		want float64
	}{
		{q: "NaN", want: math.NaN()},
		{q: "-1", want: math.Inf(-1)},
		{q: "2", want: math.Inf(1)},
	} {
		expected := map[string]float64{"smallest positive subnormal": tc.want}
		checkQuery("quantile_over_time("+tc.q+", quantile_range{case=\"smallest positive subnormal\"}[2s])", expected, true)
		checkQuery("quantile by (case) ("+tc.q+", quantile_aggregation{case=\"smallest positive subnormal\"})", expected, true)
	}
}

func TestMADOverTimeUsesExactSubnormalMedian(t *testing.T) {
	stor := teststorage.New(t)
	app := stor.AppenderV2(t.Context())
	metric := labels.FromStrings("__name__", "metric")
	for i, value := range []float64{0, 0, math.Float64frombits(2), math.Float64frombits(2)} {
		_, err := app.Append(0, metric, 0, int64(i*1000), value, nil, nil, storage.AppendV2Options{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	engine := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)
	query, err := engine.NewInstantQuery(t.Context(), stor, nil, "mad_over_time(metric[4s])", timestamp.Time(3000))
	require.NoError(t, err)
	defer query.Close()

	result := query.Exec(t.Context())
	require.NoError(t, result.Err)
	require.Empty(t, result.Warnings)
	vector, err := result.Vector()
	require.NoError(t, err)
	require.Len(t, vector, 1)
	require.Equal(t, math.Float64bits(math.Float64frombits(1)), math.Float64bits(vector[0].F))
}

func TestRangeSlidingWindowMatchesInstantQueries(t *testing.T) {
	stor := teststorage.New(t, func(opts *tsdb.Options) {
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
	})
	app := stor.AppenderV2(t.Context())

	for _, seriesID := range []string{"1", "2"} {
		counter := labels.FromStrings("__name__", "sliding_counter_total", "series", seriesID)
		gauge := labels.FromStrings("__name__", "sliding_gauge", "series", seriesID)

		// Populate 120 samples with counter resets, start-timestamp resets/overlaps, NaNs, a dense sub-second burst,
		// and a mid-series gap so the window compacts, reallocates, resets, and slides across all boundary cases.
		var val float64
		st := int64(1000)
		for i := range 120 {
			if i >= 70 && i < 82 {
				// Leave a gap larger than the 10s range window to exercise non-overlapping window resets.
				continue
			}
			ts := int64((i + 1) * 1000)
			switch {
			case i%17 == 5:
				// Value reset.
				val = 2
				st = ts - 500
			case i == 31:
				// Start-timestamp reset without value drop.
				val += 5
				st = ts - 200
			case i == 44 && seriesID == "1":
				// Start-timestamp overlap warning case on series="1" only, testing per-series warning isolation.
				val += 3
				st = ts - 5000
			default:
				val += float64((i % 5) + 1)
			}
			_, err := app.Append(0, counter, st, ts, val, nil, nil, storage.AOptions{})
			require.NoError(t, err)

			gVal := float64(i % 4)
			if i == 22 || i == 23 {
				gVal = math.NaN()
			}
			_, err = app.Append(0, gauge, 0, ts, gVal, nil, nil, storage.AOptions{})
			require.NoError(t, err)

			if i == 10 {
				// Append a dense sub-second burst right after the 10s window begins sliding (off == 2)
				// so both floatWindow.append and enh.appendWinEvent hit their 0 < off < n/4 growth branches.
				for sub := int64(1); sub <= 9; sub++ {
					subTS := ts + sub*100
					if sub%2 == 1 {
						val = 1
						st = subTS - 50
					} else {
						val += 4
					}
					_, err := app.Append(0, counter, st, subTS, val, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					_, err = app.Append(0, gauge, 0, subTS, float64(sub), nil, nil, storage.AOptions{})
					require.NoError(t, err)
				}
			}
		}
	}
	require.NoError(t, app.Commit())

	engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
		MaxSamples:               promqltest.DefaultMaxSamplesPerQuery,
		Timeout:                  10 * time.Second,
		EnableDelayedNameRemoval: false,
	})

	queries := []string{
		"increase(sliding_counter_total[10s])",
		"rate(sliding_counter_total[25s])",
		"resets(sliding_counter_total[10s])",
		"resets(sliding_counter_total[25s])",
		"changes(sliding_gauge[10s])",
		"changes(sliding_gauge[25s])",
		"absent_over_time(sliding_gauge[10s])",
	}
	start := time.Unix(1, 0)
	end := time.Unix(125, 0)
	step := time.Second

	enableST := true
	qOpts := promql.NewPrometheusQueryOpts(false, 0, &enableST)
	for _, expr := range queries {
		t.Run("expr="+expr, func(t *testing.T) {
			rq, err := engine.NewRangeQuery(t.Context(), stor, qOpts, expr, start, end, step)
			require.NoError(t, err)
			t.Cleanup(rq.Close)

			rRes := rq.Exec(t.Context())
			require.NoError(t, rRes.Err)
			mat, err := rRes.Matrix()
			require.NoError(t, err)

			type sampleKey struct {
				series string
				ts     int64
			}
			gotByKey := make(map[sampleKey]float64)
			for _, ser := range mat {
				sID := ser.Metric.Get("series")
				for _, pt := range ser.Floats {
					gotByKey[sampleKey{series: sID, ts: pt.T}] = pt.F
				}
			}

			var wantAnnos annotations.Annotations
			for ts := start; !ts.After(end); ts = ts.Add(step) {
				iq, err := engine.NewInstantQuery(t.Context(), stor, qOpts, expr, ts)
				require.NoError(t, err)
				iRes := iq.Exec(t.Context())
				iq.Close()
				require.NoError(t, iRes.Err)
				wantAnnos.Merge(iRes.Warnings)
				vec, err := iRes.Vector()
				require.NoError(t, err)

				tsMS := timestamp.FromTime(ts)
				wantAtStep := make(map[string]float64, len(vec))
				for _, smpl := range vec {
					wantAtStep[smpl.Metric.Get("series")] = smpl.F
				}
				for _, sID := range []string{"", "1", "2"} {
					got, hasGot := gotByKey[sampleKey{series: sID, ts: tsMS}]
					want, hasWant := wantAtStep[sID]
					if !hasWant {
						require.False(t, hasGot, "unexpected sample for series=%q at ts=%d", sID, tsMS)
						continue
					}
					require.True(t, hasGot, "missing sample for series=%q at ts=%d", sID, tsMS)
					if math.IsNaN(want) {
						require.True(t, math.IsNaN(got), "series=%q ts=%d", sID, tsMS)
					} else {
						require.InDelta(t, want, got, 1e-12, "series=%q ts=%d", sID, tsMS)
					}
				}
			}
			wantWarns, wantInfos := wantAnnos.AsStrings(expr, 0, 0)
			gotWarns, gotInfos := rRes.Warnings.AsStrings(expr, 0, 0)
			require.ElementsMatch(t, wantWarns, gotWarns)
			require.ElementsMatch(t, wantInfos, gotInfos)
		})
	}
}
