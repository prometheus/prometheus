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
			h1: h1, h2: h2, t1: 0, t2: 20, t: 0,
			isCounter: false, wantCount: 1,
		},
		{
			name: "exact match t2",
			h1: h1, h2: h2, t1: 0, t2: 20, t: 20,
			isCounter: false, wantCount: 3,
		},
		{
			name: "midpoint no reset",
			h1: h1, h2: h2, t1: 0, t2: 20, t: 10,
			isCounter: false, wantCount: 2,
		},
		{
			name: "counter midpoint no reset",
			h1: h1, h2: h2, t1: 0, t2: 20, t: 10,
			isCounter: true, wantCount: 2,
		},
		{
			name: "counter midpoint with reset: scale from zero",
			h1: h1, h2: h2Reset, t1: 0, t2: 20, t: 10,
			// h2Reset * (10/20) = count:1 * 0.5 = 0.5.
			isCounter: true, wantCount: 0.5,
		},
		{
			name: "quarter point",
			h1: h1, h2: h2, t1: 0, t2: 20, t: 5,
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
