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

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
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

func TestCompareNatural(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{a: "", b: "", want: 0},
		{a: "a", b: "a", want: 0},
		{a: "1", b: "1", want: 0},
		{a: "a1", b: "a2", want: -1},
		{a: "a2", b: "a1", want: 1},
		{a: "2", b: "10", want: -1},
		{a: "10", b: "2", want: 1},
		{a: "4m5", b: "4m600", want: -1},
		{a: "4m600", b: "4m1000", want: -1},
		{a: "1.2.3", b: "1.11.3", want: -1},
		// Equal numeric chunk values with different representations (e.g. leading zeros).
		{a: "01", b: "1", want: -1},
		{a: "1", b: "01", want: 1},
		{a: "00", b: "0", want: 1},
		{a: "0", b: "00", want: -1},
		{a: "a01", b: "a1", want: -1},
		{a: "a1", b: "a01", want: 1},
		{a: "001", b: "01", want: -1},
		{a: "01", b: "001", want: 1},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q_vs_%q", tc.a, tc.b), func(t *testing.T) {
			got := compareNatural(tc.a, tc.b)
			require.Equal(t, tc.want, got)

			// Strict weak ordering / antisymmetry: compareNatural(a, b) == -compareNatural(b, a).
			gotRev := compareNatural(tc.b, tc.a)
			require.Equal(t, -got, gotRev)
		})
	}
}

func TestCompareNaturalStrictWeakOrderingProperties(t *testing.T) {
	vocabulary := []string{
		"", "a", "b", "z",
		"0", "00", "000", "1", "01", "001", "2", "02", "10", "010", "11", "99", "099", "100", "1000",
		"a0", "a00", "a01", "a1", "a001", "a2", "a10", "a010", "a20", "b1", "b01", "aa1",
		"node-0", "node-00", "node-1", "node-01", "node-2", "node-10", "node-10-beta", "node-10-alpha",
		"1.0.0", "1.0.1", "1.00.1", "1.0.01", "1.1.0", "1.2.3", "1.11.3", "1.111.3", "2.0.0",
		"_", "__name__", "host:80", "host:080", "host:8080",
	}

	// 1. Irreflexivity and Antisymmetry on all pairs.
	for _, a := range vocabulary {
		require.Equal(t, 0, compareNatural(a, a), "irreflexivity failed for %q", a)

		for _, b := range vocabulary {
			ab := compareNatural(a, b)
			ba := compareNatural(b, a)
			require.Equal(t, -ab, ba, "antisymmetry failed for (%q, %q)", a, b)
			if a == b {
				require.Equal(t, 0, ab)
			} else {
				require.NotEqual(t, 0, ab, "trichotomy failed for distinct strings (%q, %q)", a, b)
			}
		}
	}

	// 2. Transitivity on all triples.
	for _, a := range vocabulary {
		for _, b := range vocabulary {
			ab := compareNatural(a, b)
			for _, c := range vocabulary {
				bc := compareNatural(b, c)
				ac := compareNatural(a, c)

				if ab < 0 && bc < 0 {
					require.Negative(t, ac, "transitivity failed: %q < %q and %q < %q, but %q not < %q (got %d)", a, b, b, c, a, c, ac)
				}
				if ab > 0 && bc > 0 {
					require.Positive(t, ac, "transitivity failed: %q > %q and %q > %q, but %q not > %q (got %d)", a, b, b, c, a, c, ac)
				}
				if ab == 0 && bc == 0 {
					require.Equal(t, 0, ac, "transitivity of equivalence failed for (%q, %q, %q)", a, b, c)
				}
			}
		}
	}
}

func TestSortByLabelStrictWeakOrdering(t *testing.T) {
	// Tests that sort_by_label produces deterministic order regardless of input permutation,
	// especially for equal-chunk label values like "01" and "1", or "0" and "00".
	exprs := parser.Expressions{
		&parser.VectorSelector{Name: "dummy"},
		&parser.StringLiteral{Val: "instance"},
	}

	samples1 := Vector{
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "01"), F: 1},
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "1"), F: 2},
	}
	samples2 := Vector{
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "1"), F: 2},
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "01"), F: 1},
	}

	res1, _ := funcSortByLabel([]Vector{samples1}, nil, exprs, nil)
	res2, _ := funcSortByLabel([]Vector{samples2}, nil, exprs, nil)

	require.Equal(t, "01", res1[0].Metric.Get("instance"))
	require.Equal(t, "1", res1[1].Metric.Get("instance"))
	require.Equal(t, "01", res2[0].Metric.Get("instance"))
	require.Equal(t, "1", res2[1].Metric.Get("instance"))

	// Descending sort.
	samplesDesc1 := Vector{
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "01"), F: 1},
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "1"), F: 2},
	}
	samplesDesc2 := Vector{
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "1"), F: 2},
		Sample{Metric: labels.FromStrings("__name__", "m", "instance", "01"), F: 1},
	}

	resDesc1, _ := funcSortByLabelDesc([]Vector{samplesDesc1}, nil, exprs, nil)
	resDesc2, _ := funcSortByLabelDesc([]Vector{samplesDesc2}, nil, exprs, nil)

	require.Equal(t, "1", resDesc1[0].Metric.Get("instance"))
	require.Equal(t, "01", resDesc1[1].Metric.Get("instance"))
	require.Equal(t, "1", resDesc2[0].Metric.Get("instance"))
	require.Equal(t, "01", resDesc2[1].Metric.Get("instance"))
}

func BenchmarkCompareNatural(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = compareNatural("01", "1")
		_ = compareNatural("4m5", "4m600")
		_ = compareNatural("node_exporter", "node_exporter")
	}
}

func BenchmarkSortByLabel(b *testing.B) {
	exprs := parser.Expressions{
		&parser.VectorSelector{Name: "dummy"},
		&parser.StringLiteral{Val: "instance"},
	}

	makeSamples := func() Vector {
		instances := []string{"0", "00", "01", "1", "02", "2", "10", "11", "20", "100", "4m5", "4m600", "4m1000", "a01", "a1"}
		vec := make(Vector, len(instances))
		for i, inst := range instances {
			vec[i] = Sample{
				Metric: labels.FromStrings("__name__", "m", "instance", inst),
				F:      float64(i),
			}
		}
		return vec
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		samples := makeSamples()
		_, _ = funcSortByLabel([]Vector{samples}, nil, exprs, nil)
	}
}
