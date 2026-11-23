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

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
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

func TestRangeUnaryAndTimestampFastPaths(t *testing.T) {
	stor := promqltest.LoadedStorage(t, `
load 10s
	unary_float{__type__="gauge",__unit__="seconds",k="1"}@st _ -1s -2s -3s -4s -5s -6s
	unary_float{__type__="gauge",__unit__="seconds",k="1"}    _ -1  -2  -3  -4  -5  -6
	unary_hist{k="2"}@st                                      _ -1s -2s -3s -4s -5s -6s
	unary_hist{k="2"}                                         _ {{sum:1}} {{sum:2}} {{sum:3}} {{sum:4}} {{sum:5}} {{sum:6}}
	unary_mixed{k="3"}@st                                     _ -1s -2s -3s -4s -5s -6s
	unary_mixed{k="3"}                                        _ -1  -2  -3  {{sum:4}} {{sum:5}} {{sum:6}}
	dup_a{k="dup"}@st                                         _ -1s -2s
	dup_a{k="dup"}                                            _ -1  -2
	dup_b{k="dup"}@st                                         _ _   _   _   _   -5s -6s
	dup_b{k="dup"}                                            _ _   _   _   _   -5  -6
	dup_err_a{k="dup_err"}@st                                 _ -5s
	dup_err_a{k="dup_err"}                                    _ -1
	dup_err_b{k="dup_err"}@st                                 _ -5s
	dup_err_b{k="dup_err"}                                    _ -2
	mix_dup_a{k="mix_dup"}@st                                 _ -1s -2s
	mix_dup_a{k="mix_dup"}                                    _ -1  -2
	mix_dup_b{k="mix_dup"}@st                                 _ -1s -2s _   _   -5s -6s
	mix_dup_b{k="mix_dup"}                                    _ {{sum:1}} {{sum:2}} _ _ -5 -6
	stale_float{k="stale"}@st                                 _ -1s _   _   -4s
	stale_float{k="stale"}                                    _ -1  stale _ -4
`, func(opt *tsdb.Options) {
		opt.EnableSTStorage = true
		opt.FloatChunkEncoding = chunkenc.EncXOR2
		opt.EnableHistogramSTEncoding = true
	})

	// Append a native histogram series with an explicit stale histogram marker
	// (promqltest load syntax does not support stale inside {{...}} histograms).
	app := stor.AppenderV2(t.Context())
	staleHistLbls := labels.FromStrings(model.MetricNameLabel, "stale_hist", "k", "stale_h")
	for _, pt := range []struct {
		st, ts int64
		fh     *histogram.FloatHistogram
	}{
		{st: 9_000, ts: 10_000, fh: &histogram.FloatHistogram{Count: 1, Sum: 1}},
		{st: 0, ts: 20_000, fh: &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)}},
		{st: 36_000, ts: 40_000, fh: &histogram.FloatHistogram{Count: 1, Sum: 4}},
	} {
		_, err := app.Append(0, staleHistLbls, pt.st, pt.ts, 0, nil, pt.fh, storage.AppendV2Options{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	start := time.Unix(10, 0)
	end := time.Unix(60, 0)
	// Query every 5s across 10s sample intervals so odd steps exercise PeekPrev lookback.
	step := 5 * time.Second

	expectedSampleIdx := func(tsMS int64) int {
		return int(tsMS/10_000) - 1
	}

	testCases := []struct {
		expr string
		want func(tsMS int64) map[string]float64
	}{
		{
			expr: `abs({k=~"1|2|3"})`,
			want: func(tsMS int64) map[string]float64 {
				idx := expectedSampleIdx(tsMS)
				v := float64(idx + 1)
				if idx < 3 {
					return map[string]float64{`{k="1"}`: v, `{k="3"}`: v}
				}
				return map[string]float64{`{k="1"}`: v}
			},
		},
		{
			expr: `sgn({k=~"1|2|3"})`,
			want: func(tsMS int64) map[string]float64 {
				if expectedSampleIdx(tsMS) < 3 {
					return map[string]float64{`{k="1"}`: -1, `{k="3"}`: -1}
				}
				return map[string]float64{`{k="1"}`: -1}
			},
		},
		{
			expr: `timestamp({k=~"1|2|3"})`,
			want: func(tsMS int64) map[string]float64 {
				sampleTS := float64((expectedSampleIdx(tsMS) + 1) * 10)
				return map[string]float64{`{k="1"}`: sampleTS, `{k="2"}`: sampleTS, `{k="3"}`: sampleTS}
			},
		},
		{
			expr: `abs(unary_hist)`,
			want: func(int64) map[string]float64 {
				return nil
			},
		},
		{
			expr: `timestamp(unary_float @ 30)`,
			want: func(int64) map[string]float64 {
				return map[string]float64{`{k="1"}`: 30}
			},
		},
		{
			expr: `start_timestamp(unary_float @ 30)`,
			want: func(int64) map[string]float64 {
				return map[string]float64{`{k="1"}`: 27}
			},
		},
		{
			expr: `timestamp(unary_float offset 10s)`,
			want: func(tsMS int64) map[string]float64 {
				if tsMS < 20_000 {
					return nil
				}
				sampleTS := float64((expectedSampleIdx(tsMS-10_000) + 1) * 10)
				return map[string]float64{`{k="1"}`: sampleTS}
			},
		},
		{
			expr: `start_timestamp(unary_float offset 10s)`,
			want: func(tsMS int64) map[string]float64 {
				if tsMS < 20_000 {
					return nil
				}
				sampleST := float64((expectedSampleIdx(tsMS-10_000) + 1) * 9)
				return map[string]float64{`{k="1"}`: sampleST}
			},
		},
		{
			expr: `start_timestamp({k=~"1|2|3"})`,
			want: func(tsMS int64) map[string]float64 {
				idx := expectedSampleIdx(tsMS)
				sampleST := float64((idx + 1) * 9)
				return map[string]float64{`{k="1"}`: sampleST, `{k="2"}`: sampleST, `{k="3"}`: sampleST}
			},
		},
		{
			expr: `abs({k="dup"})`,
			want: func(tsMS int64) map[string]float64 {
				if tsMS >= 35_000 && tsMS < 50_000 {
					return nil
				}
				idx := expectedSampleIdx(tsMS)
				if tsMS == 30_000 {
					idx = 1
				}
				return map[string]float64{`{k="dup"}`: float64(idx + 1)}
			},
		},
		{
			expr: `timestamp({k="dup"})`,
			want: func(tsMS int64) map[string]float64 {
				if tsMS >= 35_000 && tsMS < 50_000 {
					return nil
				}
				idx := expectedSampleIdx(tsMS)
				if tsMS == 30_000 {
					idx = 1
				}
				sampleTS := float64((idx + 1) * 10)
				return map[string]float64{`{k="dup"}`: sampleTS}
			},
		},
		{
			expr: `start_timestamp({k="dup"})`,
			want: func(tsMS int64) map[string]float64 {
				if tsMS >= 35_000 && tsMS < 50_000 {
					return nil
				}
				idx := expectedSampleIdx(tsMS)
				if tsMS == 30_000 {
					idx = 1
				}
				sampleST := float64((idx + 1) * 9)
				return map[string]float64{`{k="dup"}`: sampleST}
			},
		},
		{
			// abs drops mix_dup_b's histograms at 10s..30s before duplicate-labelset merging,
			// so the remaining float points from mix_dup_a and mix_dup_b do not collide.
			expr: `abs({k="mix_dup"})`,
			want: func(tsMS int64) map[string]float64 {
				if tsMS >= 35_000 && tsMS < 50_000 {
					return nil
				}
				idx := expectedSampleIdx(tsMS)
				if tsMS == 30_000 {
					idx = 1
				}
				return map[string]float64{`{k="mix_dup"}`: float64(idx + 1)}
			},
		},
		{
			expr: `abs({k=~"stale|stale_h"})`,
			want: func(tsMS int64) map[string]float64 {
				switch {
				case tsMS <= 15_000:
					return map[string]float64{`{k="stale"}`: 1}
				case tsMS >= 40_000 && tsMS <= 50_000:
					return map[string]float64{`{k="stale"}`: 4}
				default:
					return nil
				}
			},
		},
		{
			expr: `timestamp({k=~"stale|stale_h"})`,
			want: func(tsMS int64) map[string]float64 {
				switch {
				case tsMS <= 15_000:
					return map[string]float64{`{k="stale"}`: 10, `{k="stale_h"}`: 10}
				case tsMS >= 40_000 && tsMS <= 50_000:
					return map[string]float64{`{k="stale"}`: 40, `{k="stale_h"}`: 40}
				default:
					return nil
				}
			},
		},
		{
			expr: `start_timestamp({k=~"stale|stale_h"})`,
			want: func(tsMS int64) map[string]float64 {
				switch {
				case tsMS <= 15_000:
					return map[string]float64{`{k="stale"}`: 9, `{k="stale_h"}`: 9}
				case tsMS >= 40_000 && tsMS <= 50_000:
					return map[string]float64{`{k="stale"}`: 36, `{k="stale_h"}`: 36}
				default:
					return nil
				}
			},
		},
	}

	for _, delayedNameRemoval := range []bool{false, true} {
		t.Run("delayedNameRemoval="+strconv.FormatBool(delayedNameRemoval), func(t *testing.T) {
			engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
				MaxSamples:               promqltest.DefaultMaxSamplesPerQuery,
				Timeout:                  100 * time.Second,
				EnableAtModifier:         true,
				LookbackDelta:            15 * time.Second,
				EnableDelayedNameRemoval: delayedNameRemoval,
				UseStartTimestamps:       true,
				Parser:                   parser.NewParser(promqltest.TestParserOpts),
			})

			for _, tc := range testCases {
				t.Run("expr="+tc.expr, func(t *testing.T) {
					rq, err := engine.NewRangeQuery(t.Context(), stor, nil, tc.expr, start, end, step)
					require.NoError(t, err)
					t.Cleanup(rq.Close)

					rRes := rq.Exec(t.Context())
					require.NoError(t, rRes.Err)
					require.Empty(t, rRes.Warnings)
					mat, err := rRes.Matrix()
					require.NoError(t, err)

					// Verify that duplicate labelsets were actually merged into one Series,
					// timestamps are strictly increasing per series, and no stray timestamps exist.
					seenSeries := make(map[string]struct{}, len(mat))
					gotAtTS := make(map[int64]map[string]float64)
					for _, s := range mat {
						require.Empty(t, s.Histograms)
						require.NotEmpty(t, s.Floats)
						lbl := s.Metric.String()
						require.NotContains(t, seenSeries, lbl, "duplicate series in matrix: %s", lbl)
						seenSeries[lbl] = struct{}{}
						for i, pt := range s.Floats {
							if i > 0 {
								require.Greater(t, pt.T, s.Floats[i-1].T, "non-increasing timestamp for %s", lbl)
							}
							if gotAtTS[pt.T] == nil {
								gotAtTS[pt.T] = make(map[string]float64)
							}
							require.NotContains(t, gotAtTS[pt.T], lbl, "duplicate sample for %s at ts=%d", lbl, pt.T)
							gotAtTS[pt.T][lbl] = pt.F
						}
					}

					expectedSteps := 0
					for ts := start; !ts.After(end); ts = ts.Add(step) {
						tsMS := timestamp.FromTime(ts)
						want := tc.want(tsMS)
						if len(want) > 0 {
							expectedSteps++
						}
						require.Equal(t, want, gotAtTS[tsMS], "ts=%d", tsMS)

						iq, err := engine.NewInstantQuery(t.Context(), stor, nil, tc.expr, ts)
						require.NoError(t, err)
						iRes := iq.Exec(t.Context())
						iq.Close()
						require.NoError(t, iRes.Err)
						require.Empty(t, iRes.Warnings)
						vec, err := iRes.Vector()
						require.NoError(t, err)

						var instGot map[string]float64
						if len(vec) > 0 {
							instGot = make(map[string]float64, len(vec))
							for _, smp := range vec {
								require.Nil(t, smp.H)
								instGot[smp.Metric.String()] = smp.F
							}
						}
						require.Equal(t, want, instGot, "instant ts=%d", tsMS)
					}
					if expectedSteps == 0 {
						require.Empty(t, mat)
					} else {
						require.NotEmpty(t, mat)
					}
					require.Len(t, gotAtTS, expectedSteps)
				})
			}

			t.Run("start_timestamp disabled in range query", func(t *testing.T) {
				disableST := false
				disabledOpts := promql.NewPrometheusQueryOpts(false, 0, &disableST)
				rq, err := engine.NewRangeQuery(t.Context(), stor, disabledOpts, `start_timestamp({k=~"1|2|3"})`, start, end, step)
				require.NoError(t, err)
				t.Cleanup(rq.Close)

				rRes := rq.Exec(t.Context())
				require.NoError(t, rRes.Err)
				mat, err := rRes.Matrix()
				require.NoError(t, err)
				require.Empty(t, mat)
			})

			for _, expr := range []string{
				`abs({k="dup_err"})`,
				`timestamp({k="dup_err"})`,
				`start_timestamp({k="dup_err"})`,
				`timestamp({k="mix_dup"})`,
				`start_timestamp({k="mix_dup"})`,
			} {
				t.Run("duplicate labelset error/"+expr, func(t *testing.T) {
					rq, err := engine.NewRangeQuery(t.Context(), stor, nil, expr, start, end, step)
					require.NoError(t, err)
					t.Cleanup(rq.Close)

					rRes := rq.Exec(t.Context())
					require.ErrorContains(t, rRes.Err, "vector cannot contain metrics with the same labelset")
				})
			}

			// Nested expressions observe metadata label preservation (__name__, __type__,
			// __unit__) and delayed duplicate merging, which top-level queries mask via
			// Eval's final cleanupMetricLabels.
			nestedLabelCases := []struct {
				expr string
				want func(tsMS int64) float64
			}{
				{
					expr: `label_join(abs(unary_float), "meta", ",", "__name__", "__type__", "__unit__")`,
					want: func(tsMS int64) float64 { return float64(expectedSampleIdx(tsMS) + 1) },
				},
				{
					expr: `label_join(timestamp(unary_float), "meta", ",", "__name__", "__type__", "__unit__")`,
					want: func(tsMS int64) float64 { return float64((expectedSampleIdx(tsMS) + 1) * 10) },
				},
				{
					expr: `label_join(start_timestamp(unary_float), "meta", ",", "__name__", "__type__", "__unit__")`,
					want: func(tsMS int64) float64 { return float64((expectedSampleIdx(tsMS) + 1) * 9) },
				},
			}
			for _, tc := range nestedLabelCases {
				t.Run("nested label_join/"+tc.expr, func(t *testing.T) {
					rq, err := engine.NewRangeQuery(t.Context(), stor, nil, tc.expr, start, end, step)
					require.NoError(t, err)
					t.Cleanup(rq.Close)

					rRes := rq.Exec(t.Context())
					require.NoError(t, rRes.Err)
					mat, err := rRes.Matrix()
					require.NoError(t, err)
					require.Len(t, mat, 1)

					wantLabels := `{k="1", meta=",,"}`
					if delayedNameRemoval {
						wantLabels = `{k="1", meta="unary_float,gauge,seconds"}`
					}
					require.Equal(t, wantLabels, mat[0].Metric.String())
					var wantFloats []promql.FPoint
					for ts := start; !ts.After(end); ts = ts.Add(step) {
						tsMS := timestamp.FromTime(ts)
						wantFloats = append(wantFloats, promql.FPoint{T: tsMS, F: tc.want(tsMS)})
					}
					require.Equal(t, wantFloats, mat[0].Floats)
				})
			}

			for _, expr := range []string{`count(abs({k="dup_err"}))`, `count(timestamp({k="dup_err"}))`, `count(start_timestamp({k="dup_err"}))`} {
				t.Run("nested count/"+expr, func(t *testing.T) {
					rq, err := engine.NewRangeQuery(t.Context(), stor, nil, expr, start, end, step)
					require.NoError(t, err)
					t.Cleanup(rq.Close)

					rRes := rq.Exec(t.Context())
					if !delayedNameRemoval {
						require.ErrorContains(t, rRes.Err, "vector cannot contain metrics with the same labelset")
						return
					}
					require.NoError(t, rRes.Err)
					mat, err := rRes.Matrix()
					require.NoError(t, err)
					require.Len(t, mat, 1)
					require.Equal(t, `{}`, mat[0].Metric.String())
					require.Equal(t, []promql.FPoint{{T: 10_000, F: 2}, {T: 15_000, F: 2}, {T: 20_000, F: 2}}, mat[0].Floats)
				})
			}
		})
	}
}
