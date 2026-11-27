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
	"maps"
	"math"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/kahansum"
)

func TestHistogramRateCounterResetHint(t *testing.T) {
	points := []HPoint{
		{T: 0, H: &histogram.FloatHistogram{CounterResetHint: histogram.CounterReset, Count: 5, Sum: 5}},
		{T: 1, H: &histogram.FloatHistogram{CounterResetHint: histogram.UnknownCounterReset, Count: 10, Sum: 10}},
	}
	labels := labels.FromMap(map[string]string{model.MetricNameLabel: "foo"})
	fh, _ := histogramRate(points, nil, false, labels, posrange.PositionRange{})
	require.Equal(t, histogram.GaugeType, fh.CounterResetHint)

	fh, _ = histogramRate(points, nil, true, labels, posrange.PositionRange{})
	require.Equal(t, histogram.GaugeType, fh.CounterResetHint)
}

func TestKahanSumInc(t *testing.T) {
	testCases := map[string]struct {
		first    float64
		second   float64
		expected float64
	}{
		"+Inf + anything = +Inf": {
			first:    math.Inf(1),
			second:   2.0,
			expected: math.Inf(1),
		},
		"-Inf + anything = -Inf": {
			first:    math.Inf(-1),
			second:   2.0,
			expected: math.Inf(-1),
		},
		"+Inf + -Inf = NaN": {
			first:    math.Inf(1),
			second:   math.Inf(-1),
			expected: math.NaN(),
		},
		"NaN + anything = NaN": {
			first:    math.NaN(),
			second:   2,
			expected: math.NaN(),
		},
		"NaN + Inf = NaN": {
			first:    math.NaN(),
			second:   math.Inf(1),
			expected: math.NaN(),
		},
		"NaN + -Inf = NaN": {
			first:    math.NaN(),
			second:   math.Inf(-1),
			expected: math.NaN(),
		},
	}

	runTest := func(t *testing.T, a, b, expected float64) {
		t.Run(fmt.Sprintf("%v + %v = %v", a, b, expected), func(t *testing.T) {
			sum, c := kahansum.Inc(b, a, 0)
			result := sum + c

			if math.IsNaN(expected) {
				require.Truef(t, math.IsNaN(result), "expected result to be NaN, but got %v (from %v + %v)", result, sum, c)
			} else {
				require.Equalf(t, expected, result, "expected result to be %v, but got %v (from %v + %v)", expected, result, sum, c)
			}
		})
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			runTest(t, testCase.first, testCase.second, testCase.expected)
			runTest(t, testCase.second, testCase.first, testCase.expected)
		})
	}
}

func TestInterpolate(t *testing.T) {
	tests := []struct {
		p1, p2    FPoint
		t         int64
		isCounter bool
		expected  float64
	}{
		{FPoint{T: 1, F: 100}, FPoint{T: 2, F: 200}, 1, false, 100},
		{FPoint{T: 0, F: 100}, FPoint{T: 2, F: 200}, 1, false, 150},
		{FPoint{T: 0, F: 200}, FPoint{T: 2, F: 100}, 1, false, 150},
		{FPoint{T: 0, F: 200}, FPoint{T: 2, F: 0}, 1, true, 0},
		{FPoint{T: 0, F: 200}, FPoint{T: 2, F: 100}, 1, true, 50},
		{FPoint{T: 0, F: 500}, FPoint{T: 2, F: 100}, 1, true, 50},
		{FPoint{T: 0, F: 500}, FPoint{T: 10, F: 0}, 1, true, 0},
	}
	for _, test := range tests {
		result := interpolate(test.p1, test.p2, test.t, test.isCounter)
		require.Equal(t, test.expected, result)
	}
}

func TestInterpolateHistograms(t *testing.T) {
	h1 := &histogram.FloatHistogram{Count: 1, Sum: 1, CounterResetHint: histogram.UnknownCounterReset}
	h2 := &histogram.FloatHistogram{Count: 3, Sum: 3, CounterResetHint: histogram.UnknownCounterReset}
	h2Reset := &histogram.FloatHistogram{Count: 1, Sum: 1, CounterResetHint: histogram.CounterReset}
	pos := posrange.PositionRange{}

	tests := []struct {
		name      string
		h1, h2    *histogram.FloatHistogram
		t1, t2, t int64
		isCounter bool
		wantCount float64
	}{
		{
			name: "exact match t1",
			h1:   h1, h2: h2, t1: 0, t2: 20, t: 0,
			isCounter: false, wantCount: 1,
		},
		{
			name: "exact match t2",
			h1:   h1, h2: h2, t1: 0, t2: 20, t: 20,
			isCounter: false, wantCount: 3,
		},
		{
			name: "midpoint no reset",
			h1:   h1, h2: h2, t1: 0, t2: 20, t: 10,
			isCounter: false, wantCount: 2,
		},
		{
			name: "counter midpoint no reset",
			h1:   h1, h2: h2, t1: 0, t2: 20, t: 10,
			isCounter: true, wantCount: 2,
		},
		{
			name: "counter midpoint with reset: scale from zero",
			h1:   h1, h2: h2Reset, t1: 0, t2: 20, t: 10,
			// h2Reset * (10/20) = count:1 * 0.5 = 0.5.
			isCounter: true, wantCount: 0.5,
		},
		{
			name: "quarter point",
			h1:   h1, h2: h2, t1: 0, t2: 20, t: 5,
			// h1 + (h2-h1)*0.25 = 1 + 2*0.25 = 1.5.
			isCounter: false, wantCount: 1.5,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var annos annotations.Annotations
			result, err := interpolateHistograms(tc.h1, tc.t1, tc.h2, tc.t2, tc.t, tc.isCounter, &annos, pos)
			require.NoError(t, err)
			require.Equal(t, tc.wantCount, result.Count)
		})
	}
}

func TestInPlaceFloatUnaryFuncsParity(t *testing.T) {
	floatInputs := []float64{-2, -1, -0.5, math.Copysign(0, -1), 0, 0.5, 1, 2, math.NaN(), math.Inf(1), math.Inf(-1)}
	metric := labels.FromStrings("__name__", "test_metric", "__type__", "gauge", "__unit__", "seconds", "k", "1")
	strippedMetric := metric.DropReserved(schema.IsMetadataLabel)

	inVec := make(Vector, 0, len(floatInputs)+1)
	for _, v := range floatInputs {
		inVec = append(inVec, Sample{Metric: metric, F: v})
	}
	inVec = append(inVec, Sample{Metric: metric, H: &histogram.FloatHistogram{Count: 1, Sum: 1}})

	for _, name := range slices.Sorted(maps.Keys(inPlaceFloatUnaryFuncs)) {
		fn := inPlaceFloatUnaryFuncs[name]
		t.Run(name, func(t *testing.T) {
			spec, ok := parser.Functions[name]
			require.True(t, ok, "missing parser.Functions entry for %s", name)
			require.Equal(t, []parser.ValueType{parser.ValueTypeVector}, spec.ArgTypes)
			require.Equal(t, parser.ValueTypeVector, spec.ReturnType)
			require.Zero(t, spec.Variadic)
			require.NotContains(t, AtModifierUnsafeFunctions, name)

			call := FunctionCalls[name]
			require.NotNil(t, call, "missing FunctionCalls entry for %s", name)

			for _, delayedNameRemoval := range []bool{false, true} {
				t.Run("delayedNameRemoval="+strconv.FormatBool(delayedNameRemoval), func(t *testing.T) {
					enh := &EvalNodeHelper{enableDelayedNameRemoval: delayedNameRemoval}
					got, annos := call([]Vector{inVec}, nil, nil, enh)
					require.Empty(t, annos)
					require.Len(t, got, len(floatInputs))

					wantMetric := strippedMetric
					if delayedNameRemoval {
						wantMetric = metric
					}
					for i, inF := range floatInputs {
						wantF := fn(inF)
						require.Nil(t, got[i].H)
						require.True(t, got[i].DropName)
						require.Equal(t, wantMetric, got[i].Metric)
						if math.IsNaN(wantF) {
							require.True(t, math.IsNaN(got[i].F), "input %v: expected NaN, got %v", inF, got[i].F)
						} else {
							require.Equal(t, math.Float64bits(wantF), math.Float64bits(got[i].F), "input %v", inF)
						}
					}
				})
			}
		})
	}
}

func TestDirectTimestampFunctionCalls(t *testing.T) {
	metric := labels.FromStrings("__name__", "test_metric", "__type__", "gauge", "__unit__", "seconds", "k", "1")
	strippedMetric := metric.DropReserved(schema.IsMetadataLabel)
	inVec := Vector{{Metric: metric, T: 12_500, F: 42}}

	for _, delayedNameRemoval := range []bool{false, true} {
		t.Run("delayedNameRemoval="+strconv.FormatBool(delayedNameRemoval), func(t *testing.T) {
			wantMetric := strippedMetric
			if delayedNameRemoval {
				wantMetric = metric
			}

			got, annos := FunctionCalls["timestamp"]([]Vector{inVec}, nil, nil, &EvalNodeHelper{
				enableDelayedNameRemoval: delayedNameRemoval,
			})
			require.Empty(t, annos)
			require.Equal(t, Vector{{Metric: wantMetric, F: 12.5, DropName: true}}, got)

			got, annos = FunctionCalls["start_timestamp"]([]Vector{inVec}, nil, nil, &EvalNodeHelper{
				enableDelayedNameRemoval: delayedNameRemoval,
				StartTimestamps:          &StartTimestamps{Floats: []int64{9_500}},
			})
			require.Empty(t, annos)
			require.Equal(t, Vector{{Metric: wantMetric, F: 9.5, DropName: true}}, got)

			got, annos = FunctionCalls["start_timestamp"]([]Vector{inVec}, nil, nil, &EvalNodeHelper{
				enableDelayedNameRemoval: delayedNameRemoval,
			})
			require.Empty(t, annos)
			require.Empty(t, got)
		})
	}
}

func TestFloatWinScanOverlapWarningEvictionAndFallbackParity(t *testing.T) {
	metric := labels.FromStrings(model.MetricNameLabel, "overlap_counter_total")
	args := parser.Expressions{&parser.MatrixSelector{
		VectorSelector: &parser.VectorSelector{Name: "overlap_counter_total"},
		Range:          10 * time.Second,
	}}

	scan := &floatWinScan{}
	t.Cleanup(func() {
		putFPointSlice(scan.win.buf)
		putWinEventSeqs(scan.eventSeqs)
	})
	sts := &StartTimestamps{}
	scan.win.reset(sts)
	enh := &EvalNodeHelper{
		Out:             make(Vector, 0, 1),
		StartTimestamps: sts,
		floatWinScan:    scan,
	}

	evalStep := func(ts int64, wantWarn bool) {
		t.Helper()
		mat := Matrix{{Metric: metric, Floats: scan.win.activeSlice()}}
		enh.Ts = ts
		enh.Out = enh.Out[:0]
		gotVec, gotAnnos := extrapolatedRate(mat, args, enh, true, true)

		// Cross-check against the nil-floatWinScan fallback path on every step.
		fallbackEnh := &EvalNodeHelper{
			Out:             make(Vector, 0, 1),
			Ts:              ts,
			StartTimestamps: &StartTimestamps{Floats: slices.Clone(sts.Floats)},
		}
		wantVec, wantAnnos := extrapolatedRate(mat, args, fallbackEnh, true, true)
		require.Len(t, gotVec, len(wantVec))
		for i := range wantVec {
			require.InDelta(t, wantVec[i].F, gotVec[i].F, 1e-12)
		}
		gotWarns, _ := gotAnnos.AsStrings("rate(overlap_counter_total[10s])", 0, 0)
		wantWarns, _ := wantAnnos.AsStrings("rate(overlap_counter_total[10s])", 0, 0)
		require.Equal(t, wantWarns, gotWarns)
		if wantWarn {
			require.Len(t, gotWarns, 1)
			require.Contains(t, gotWarns[0], "overlap_counter_total")
		} else {
			require.Empty(t, gotWarns)
		}
	}

	// Step 0: pair (0,1) overlaps (currST=500 < prevT=1000 and != prevST=100).
	scan.win.append(FPoint{T: 1000, F: 10}, 100, sts)
	scan.win.append(FPoint{T: 2000, F: 20}, 500, sts)
	scan.win.append(FPoint{T: 3000, F: 30}, 500, sts)
	evalStep(3000, true)

	// Step 1: drop seq 0 so the overlap pair (0,1) slides out at the exact startSeq == lastOverlapCurrSeq boundary.
	scan.win.drop(1, sts)
	scan.win.append(FPoint{T: 4000, F: 40}, 500, sts)
	evalStep(4000, false)

	// Step 2: append two consecutive overlap pairs (3,4) and (4,5).
	scan.win.append(FPoint{T: 5000, F: 50}, 3500, sts)
	scan.win.append(FPoint{T: 6000, F: 60}, 4500, sts)
	evalStep(6000, true)

	// Step 3: drop through seq 3 so first overlap (3,4) slides out while second overlap (4,5) stays in window.
	scan.win.drop(3, sts)
	evalStep(6000, true)

	// Step 4: drop seq 4 and append a clean point so second overlap (4,5) slides out as well.
	scan.win.drop(1, sts)
	scan.win.append(FPoint{T: 7000, F: 70}, 4500, sts)
	evalStep(7000, false)

	// Verify direct FunctionCalls work both with nil floatWinScan and when a caller passes
	// a mismatched Floats slice alongside an existing floatWinScan (forcing safe fallback).
	synthFloats := []FPoint{
		{T: 1000, F: 10},
		{T: 2000, F: 5},
		{T: 3000, F: 15},
		{T: 4000, F: math.NaN()},
		{T: 5000, F: math.NaN()},
		{T: 6000, F: 20},
	}
	synthSTs := &StartTimestamps{Floats: []int64{100, 1500, 2500, 2500, 2500, 5500}}
	synthMat := Matrix{{Metric: metric, Floats: synthFloats}}
	for _, fnName := range []string{"rate", "increase", "resets", "changes"} {
		t.Run("direct_fallback/"+fnName, func(t *testing.T) {
			fn := FunctionCalls[fnName]
			nilEnh := &EvalNodeHelper{Out: make(Vector, 0, 1), Ts: 6000, StartTimestamps: synthSTs}
			wantVec, wantAnnos := fn(nil, synthMat, args, nilEnh)

			mismatchEnh := &EvalNodeHelper{Out: make(Vector, 0, 1), Ts: 6000, StartTimestamps: synthSTs, floatWinScan: scan}
			gotVec, gotAnnos := fn(nil, synthMat, args, mismatchEnh)
			require.Len(t, gotVec, len(wantVec))
			for i := range wantVec {
				if math.IsNaN(wantVec[i].F) {
					require.True(t, math.IsNaN(gotVec[i].F))
				} else {
					require.InDelta(t, wantVec[i].F, gotVec[i].F, 1e-12)
				}
			}
			require.Equal(t, wantAnnos, gotAnnos)
		})
	}
}
