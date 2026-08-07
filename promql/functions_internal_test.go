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

func makeFloatSeries(metric labels.Labels, vals []float64) Series {
	pts := make([]FPoint, len(vals))
	for i, v := range vals {
		pts[i] = FPoint{T: int64(i * 10000), F: v}
	}
	return Series{Metric: metric, Floats: pts}
}

func makeHistSeries(metric labels.Labels, counts, sums []float64) Series {
	pts := make([]HPoint, len(counts))
	for i := range counts {
		pts[i] = HPoint{T: int64(i * 10000), H: &histogram.FloatHistogram{Count: counts[i], Sum: sums[i]}}
	}
	return Series{Metric: metric, Histograms: pts}
}

func runAnomalyFunc(fn FunctionCall, series Series) (float64, bool) {
	enh := &EvalNodeHelper{Out: make(Vector, 0)}
	out, _ := fn(nil, Matrix{series}, nil, enh)
	if len(out) == 0 {
		return 0, false
	}
	return out[0].F, true
}

var anomalyMetric = labels.FromStrings("__name__", "test")

// --- clampScore ---

func TestClampScore(t *testing.T) {
	require.Equal(t, 0.0, clampScore(-1))
	require.Equal(t, 0.0, clampScore(0))
	require.Equal(t, 0.5, clampScore(0.5))
	require.Equal(t, 1.0, clampScore(1))
	require.Equal(t, 1.0, clampScore(2))
}

// --- quickSelectMedian ---

func TestQuickSelectMedian(t *testing.T) {
	require.Equal(t, 3.0, quickSelectMedian([]float64{1, 2, 3, 4, 5}))
	require.Equal(t, 2.5, quickSelectMedian([]float64{1, 2, 3, 4}))
	require.Equal(t, 5.0, quickSelectMedian([]float64{5}))
	require.Equal(t, 0.0, quickSelectMedian(nil))
}

// --- extractAnomalySamples ---

func TestExtractAnomalySamples(t *testing.T) {
	t.Run("floats", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 2, 3})
		samples, annos := extractAnomalySamples(s, "test", posrange.PositionRange{})
		require.Nil(t, annos)
		require.Len(t, samples, 3)
		require.Equal(t, 2.0, samples[1].V)
	})
	t.Run("histograms use avg", func(t *testing.T) {
		s := makeHistSeries(anomalyMetric, []float64{4, 8}, []float64{8, 24})
		samples, annos := extractAnomalySamples(s, "test", posrange.PositionRange{})
		require.Nil(t, annos)
		require.Len(t, samples, 2)
		require.Equal(t, 2.0, samples[0].V) // 8/4
		require.Equal(t, 3.0, samples[1].V) // 24/8
	})
	t.Run("nil histogram skipped", func(t *testing.T) {
		s := Series{Metric: anomalyMetric, Histograms: []HPoint{{T: 0, H: nil}, {T: 1000, H: &histogram.FloatHistogram{Count: 2, Sum: 6}}}}
		samples, annos := extractAnomalySamples(s, "test", posrange.PositionRange{})
		require.Nil(t, annos)
		require.Len(t, samples, 1)
		require.Equal(t, 3.0, samples[0].V)
	})
	t.Run("mixed returns warning", func(t *testing.T) {
		s := Series{
			Metric:     anomalyMetric,
			Floats:     []FPoint{{T: 0, F: 1}},
			Histograms: []HPoint{{T: 1000, H: &histogram.FloatHistogram{Count: 1, Sum: 1}}},
		}
		_, annos := extractAnomalySamples(s, "test", posrange.PositionRange{})
		require.NotNil(t, annos)
	})
}

// --- funcEWMA ---

func TestFuncEWMA(t *testing.T) {
	t.Run("constant series scores zero", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{5, 5, 5, 5, 5})
		v, ok := runAnomalyFunc(funcEWMA, s)
		require.True(t, ok)
		require.Equal(t, 0.0, v)
	})
	t.Run("spike scores high", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1000})
		v, ok := runAnomalyFunc(funcEWMA, s)
		require.True(t, ok)
		require.Greater(t, v, 0.4)
	})
	t.Run("too few samples returns empty", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1})
		_, ok := runAnomalyFunc(funcEWMA, s)
		require.False(t, ok)
	})
	t.Run("histogram series works", func(t *testing.T) {
		counts := make([]float64, 10)
		sums := make([]float64, 10)
		for i := range 10 {
			counts[i] = 2
			sums[i] = float64(i * 2)
		}
		s := makeHistSeries(anomalyMetric, counts, sums)
		_, ok := runAnomalyFunc(funcEWMA, s)
		require.True(t, ok)
	})
}

// --- funcZScore ---

func TestFuncZScore(t *testing.T) {
	t.Run("constant series scores zero", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{5, 5, 5, 5, 5})
		v, ok := runAnomalyFunc(funcZScore, s)
		require.True(t, ok)
		require.Equal(t, 0.0, v)
	})
	t.Run("outlier scores high", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 1, 1, 1, 1, 1, 100})
		v, ok := runAnomalyFunc(funcZScore, s)
		require.True(t, ok)
		require.Greater(t, v, 0.5)
	})
	t.Run("histogram series works", func(t *testing.T) {
		counts := make([]float64, 5)
		sums := make([]float64, 5)
		for i := range 5 {
			counts[i] = 2
			sums[i] = float64(i * 2)
		}
		s := makeHistSeries(anomalyMetric, counts, sums)
		_, ok := runAnomalyFunc(funcZScore, s)
		require.True(t, ok)
	})
}

// --- funcMAD ---

func TestFuncMAD(t *testing.T) {
	t.Run("constant series scores zero", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10})
		v, ok := runAnomalyFunc(funcMAD, s)
		require.True(t, ok)
		require.Equal(t, 0.0, v)
	})
	t.Run("large outlier scores 1", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 2, 1, 2, 1, 2, 1, 2, 1, 1000})
		v, ok := runAnomalyFunc(funcMAD, s)
		require.True(t, ok)
		require.Equal(t, 1.0, v)
	})
	t.Run("histogram series works", func(t *testing.T) {
		counts := make([]float64, 5)
		sums := make([]float64, 5)
		for i := range 5 {
			counts[i] = 2
			sums[i] = float64(i * 2)
		}
		s := makeHistSeries(anomalyMetric, counts, sums)
		_, ok := runAnomalyFunc(funcMAD, s)
		require.True(t, ok)
	})
}

// --- funcHST ---

func TestFuncHSTHistogramSamples(t *testing.T) {
	counts := make([]float64, 40)
	sums := make([]float64, 40)
	for i := range 40 {
		counts[i] = float64(10 + i)
		sums[i] = float64(100 + i*5)
	}
	t.Run("32+ histogram samples produces score in [0,1]", func(t *testing.T) {
		s := makeHistSeries(anomalyMetric, counts, sums)
		v, ok := runAnomalyFunc(funcHST, s)
		require.True(t, ok)
		require.GreaterOrEqual(t, v, 0.0)
		require.LessOrEqual(t, v, 1.0)
	})
	t.Run("too few histogram samples returns empty", func(t *testing.T) {
		s := makeHistSeries(anomalyMetric, counts[:10], sums[:10])
		_, ok := runAnomalyFunc(funcHST, s)
		require.False(t, ok)
	})
}

// --- funcChangepoint ---

func TestFuncChangepoint(t *testing.T) {
	t.Run("constant series scores zero", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{5, 5, 5, 5, 5, 5, 5, 5})
		v, ok := runAnomalyFunc(funcChangepoint, s)
		require.True(t, ok)
		require.Equal(t, 0.0, v)
	})
	t.Run("sudden step change scores high", func(t *testing.T) {
		vals := make([]float64, 20)
		for i := range 10 {
			vals[i] = 1
		}
		for i := 10; i < 20; i++ {
			vals[i] = 100
		}
		s := makeFloatSeries(anomalyMetric, vals)
		v, ok := runAnomalyFunc(funcChangepoint, s)
		require.True(t, ok)
		require.Greater(t, v, 0.5)
	})
	t.Run("too few samples returns empty", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 2, 3})
		_, ok := runAnomalyFunc(funcChangepoint, s)
		require.False(t, ok)
	})
	t.Run("histogram series works", func(t *testing.T) {
		counts := make([]float64, 10)
		sums := make([]float64, 10)
		for i := range 10 {
			counts[i] = 2
			sums[i] = float64(i * 2)
		}
		s := makeHistSeries(anomalyMetric, counts, sums)
		_, ok := runAnomalyFunc(funcChangepoint, s)
		require.True(t, ok)
	})
}

// --- funcTrendScore ---

func TestFuncTrendScore(t *testing.T) {
	t.Run("perfect linear trend scores zero", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 2, 3, 4, 5, 6, 7, 8})
		v, ok := runAnomalyFunc(funcTrendScore, s)
		require.True(t, ok)
		require.Equal(t, 0.0, v)
	})
	t.Run("spike off trend scores high", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 2, 3, 4, 5, 6, 7, 100})
		v, ok := runAnomalyFunc(funcTrendScore, s)
		require.True(t, ok)
		require.Greater(t, v, 0.5)
	})
	t.Run("too few samples returns empty", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 2})
		_, ok := runAnomalyFunc(funcTrendScore, s)
		require.False(t, ok)
	})
	t.Run("histogram series works", func(t *testing.T) {
		counts := make([]float64, 5)
		sums := make([]float64, 5)
		for i := range 5 {
			counts[i] = 2
			sums[i] = float64(i * 2)
		}
		s := makeHistSeries(anomalyMetric, counts, sums)
		_, ok := runAnomalyFunc(funcTrendScore, s)
		require.True(t, ok)
	})
}

// --- funcBurstScore ---

func TestFuncBurstScore(t *testing.T) {
	t.Run("constant series scores zero", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{5, 5, 5, 5, 5, 5})
		v, ok := runAnomalyFunc(funcBurstScore, s)
		require.True(t, ok)
		require.Equal(t, 0.0, v)
	})
	t.Run("sudden spike scores high", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1000})
		v, ok := runAnomalyFunc(funcBurstScore, s)
		require.True(t, ok)
		require.Greater(t, v, 0.5)
	})
	t.Run("too few samples returns empty", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1})
		_, ok := runAnomalyFunc(funcBurstScore, s)
		require.False(t, ok)
	})
	t.Run("histogram series works", func(t *testing.T) {
		counts := make([]float64, 5)
		sums := make([]float64, 5)
		for i := range 5 {
			counts[i] = 2
			sums[i] = float64(i * 2)
		}
		s := makeHistSeries(anomalyMetric, counts, sums)
		_, ok := runAnomalyFunc(funcBurstScore, s)
		require.True(t, ok)
	})
}

// --- funcEntropy ---

func TestFuncEntropy(t *testing.T) {
	t.Run("constant series has zero entropy", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{5, 5, 5, 5, 5, 5})
		v, ok := runAnomalyFunc(funcEntropy, s)
		require.True(t, ok)
		require.Equal(t, 0.0, v)
	})
	t.Run("uniform distribution has high entropy", func(t *testing.T) {
		// 32 evenly spaced values → high entropy
		vals := make([]float64, 32)
		for i := range 32 {
			vals[i] = float64(i)
		}
		s := makeFloatSeries(anomalyMetric, vals)
		v, ok := runAnomalyFunc(funcEntropy, s)
		require.True(t, ok)
		require.Greater(t, v, 0.5)
	})
	t.Run("too few samples returns empty", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1})
		_, ok := runAnomalyFunc(funcEntropy, s)
		require.False(t, ok)
	})
	t.Run("result in [0,1]", func(t *testing.T) {
		s := makeFloatSeries(anomalyMetric, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
		v, ok := runAnomalyFunc(funcEntropy, s)
		require.True(t, ok)
		require.GreaterOrEqual(t, v, 0.0)
		require.LessOrEqual(t, v, 1.0)
	})
	t.Run("histogram series works", func(t *testing.T) {
		counts := make([]float64, 5)
		sums := make([]float64, 5)
		for i := range 5 {
			counts[i] = 2
			sums[i] = float64(i * 2)
		}
		s := makeHistSeries(anomalyMetric, counts, sums)
		_, ok := runAnomalyFunc(funcEntropy, s)
		require.True(t, ok)
	})
}
