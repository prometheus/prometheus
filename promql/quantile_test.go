// Copyright 2023 The Prometheus Authors
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
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

func TestBucketQuantile_ForcedMonotonicity(t *testing.T) {
	eps := 1e-12

	for name, tc := range map[string]struct {
		getInput       func() Buckets // The buckets can be modified in-place so return a new one each time.
		expectedForced bool
		expectedFixed  bool
		expectedValues map[float64]float64
	}{
		"simple - monotonic": {
			getInput: func() Buckets {
				return Buckets{
					{
						UpperBound: 10,
						Count:      10,
					}, {
						UpperBound: 15,
						Count:      15,
					}, {
						UpperBound: 20,
						Count:      15,
					}, {
						UpperBound: 30,
						Count:      15,
					}, {
						UpperBound: math.Inf(1),
						Count:      15,
					},
				}
			},
			expectedForced: false,
			expectedFixed:  false,
			expectedValues: map[float64]float64{
				1:    15.,
				0.99: 14.85,
				0.9:  13.5,
				0.5:  7.5,
			},
		},
		"simple - non-monotonic middle": {
			getInput: func() Buckets {
				return Buckets{
					{
						UpperBound: 10,
						Count:      10,
					}, {
						UpperBound: 15,
						Count:      15,
					}, {
						UpperBound: 20,
						Count:      15.00000000001, // Simulate the case there's a small imprecision in float64.
					}, {
						UpperBound: 30,
						Count:      15,
					}, {
						UpperBound: math.Inf(1),
						Count:      15,
					},
				}
			},
			expectedForced: false,
			expectedFixed:  true,
			expectedValues: map[float64]float64{
				1:    15.,
				0.99: 14.85,
				0.9:  13.5,
				0.5:  7.5,
			},
		},
		"real example - monotonic": {
			getInput: func() Buckets {
				return Buckets{
					{
						UpperBound: 1,
						Count:      6454661.3014166197,
					}, {
						UpperBound: 5,
						Count:      8339611.2001912938,
					}, {
						UpperBound: 10,
						Count:      14118319.2444762159,
					}, {
						UpperBound: 25,
						Count:      14130031.5272856522,
					}, {
						UpperBound: 50,
						Count:      46001270.3030008152,
					}, {
						UpperBound: 64,
						Count:      46008473.8585563600,
					}, {
						UpperBound: 80,
						Count:      46008473.8585563600,
					}, {
						UpperBound: 100,
						Count:      46008473.8585563600,
					}, {
						UpperBound: 250,
						Count:      46008473.8585563600,
					}, {
						UpperBound: 1000,
						Count:      46008473.8585563600,
					}, {
						UpperBound: math.Inf(1),
						Count:      46008473.8585563600,
					},
				}
			},
			expectedForced: false,
			expectedFixed:  false,
			expectedValues: map[float64]float64{
				1:    64.,
				0.99: 49.64475715376406,
				0.9:  46.39671690938454,
				0.5:  31.96098248992002,
			},
		},
		"real example - non-monotonic": {
			getInput: func() Buckets {
				return Buckets{
					{
						UpperBound: 1,
						Count:      6454661.3014166225,
					}, {
						UpperBound: 5,
						Count:      8339611.2001912957,
					}, {
						UpperBound: 10,
						Count:      14118319.2444762159,
					}, {
						UpperBound: 25,
						Count:      14130031.5272856504,
					}, {
						UpperBound: 50,
						Count:      46001270.3030008227,
					}, {
						UpperBound: 64,
						Count:      46008473.8585563824,
					}, {
						UpperBound: 80,
						Count:      46008473.8585563898,
					}, {
						UpperBound: 100,
						Count:      46008473.8585563824,
					}, {
						UpperBound: 250,
						Count:      46008473.8585563824,
					}, {
						UpperBound: 1000,
						Count:      46008473.8585563898,
					}, {
						UpperBound: math.Inf(1),
						Count:      46008473.8585563824,
					},
				}
			},
			expectedForced: false,
			expectedFixed:  true,
			expectedValues: map[float64]float64{
				1:    64.,
				0.99: 49.64475715376406,
				0.9:  46.39671690938454,
				0.5:  31.96098248992002,
			},
		},
		"real example 2 - monotonic": {
			getInput: func() Buckets {
				return Buckets{
					{
						UpperBound: 0.005,
						Count:      9.6,
					}, {
						UpperBound: 0.01,
						Count:      9.688888889,
					}, {
						UpperBound: 0.025,
						Count:      9.755555556,
					}, {
						UpperBound: 0.05,
						Count:      9.844444444,
					}, {
						UpperBound: 0.1,
						Count:      9.888888889,
					}, {
						UpperBound: 0.25,
						Count:      9.888888889,
					}, {
						UpperBound: 0.5,
						Count:      9.888888889,
					}, {
						UpperBound: 1,
						Count:      9.888888889,
					}, {
						UpperBound: 2.5,
						Count:      9.888888889,
					}, {
						UpperBound: 5,
						Count:      9.888888889,
					}, {
						UpperBound: 10,
						Count:      9.888888889,
					}, {
						UpperBound: 25,
						Count:      9.888888889,
					}, {
						UpperBound: 50,
						Count:      9.888888889,
					}, {
						UpperBound: 100,
						Count:      9.888888889,
					}, {
						UpperBound: math.Inf(1),
						Count:      9.888888889,
					},
				}
			},
			expectedForced: false,
			expectedFixed:  false,
			expectedValues: map[float64]float64{
				1:    0.1,
				0.99: 0.03468750000281261,
				0.9:  0.00463541666671875,
				0.5:  0.0025752314815104174,
			},
		},
		"real example 2 - non-monotonic": {
			getInput: func() Buckets {
				return Buckets{
					{
						UpperBound: 0.005,
						Count:      9.6,
					}, {
						UpperBound: 0.01,
						Count:      9.688888889,
					}, {
						UpperBound: 0.025,
						Count:      9.755555556,
					}, {
						UpperBound: 0.05,
						Count:      9.844444444,
					}, {
						UpperBound: 0.1,
						Count:      9.888888889,
					}, {
						UpperBound: 0.25,
						Count:      9.888888889,
					}, {
						UpperBound: 0.5,
						Count:      9.888888889,
					}, {
						UpperBound: 1,
						Count:      9.888888889,
					}, {
						UpperBound: 2.5,
						Count:      9.888888889,
					}, {
						UpperBound: 5,
						Count:      9.888888889,
					}, {
						UpperBound: 10,
						Count:      9.888888889001, // Simulate the case there's a small imprecision in float64.
					}, {
						UpperBound: 25,
						Count:      9.888888889,
					}, {
						UpperBound: 50,
						Count:      9.888888888999, // Simulate the case there's a small imprecision in float64.
					}, {
						UpperBound: 100,
						Count:      9.888888889,
					}, {
						UpperBound: math.Inf(1),
						Count:      9.888888889,
					},
				}
			},
			expectedForced: false,
			expectedFixed:  true,
			expectedValues: map[float64]float64{
				1:    0.1,
				0.99: 0.03468750000281261,
				0.9:  0.00463541666671875,
				0.5:  0.0025752314815104174,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for q, v := range tc.expectedValues {
				res, forced, fixed := BucketQuantile(q, tc.getInput())
				require.Equal(t, tc.expectedForced, forced)
				require.Equal(t, tc.expectedFixed, fixed)
				require.InEpsilon(t, v, res, eps)
			}
		})
	}
}

func TestHistogramFraction(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lower    float64
		upper    float64
		buckets  Buckets
		expected float64
	}{
		{
			name:  "positive buckets, lower falls in the first bucket",
			lower: 0.0,
			upper: 1.5,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [0, 1]: contributes 1.0 observation (full bucket)
			// - Bucket [1, 2]: contributes (1.5-1)/(2-1) * (3-1) = 0.5 * 2 = 1.0 observations
			// Total: (1.0 + 1.0) / 100.0 = 0.02
			expected: 0.02,
		},
		{
			name:  "negative buckets, lower falls in the first bucket",
			lower: -4.0,
			upper: -2.0,
			buckets: Buckets{
				{UpperBound: -3.0, Count: 10.0},
				{UpperBound: -2.0, Count: 12.0},
				{UpperBound: -1.0, Count: 15.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [-Inf, -3]: contributes zero observations (no interpolation with infinite width bucket)
			// - Bucket [-3, -2]: contributes 12-10 = 2.0 observations (full bucket)
			// Total: 2.0 / 100.0 = 0.02
			expected: 0.02,
		},
		{
			name:  "lower is -Inf",
			lower: math.Inf(-1),
			upper: -1.5,
			buckets: Buckets{
				{UpperBound: -3.0, Count: 10.0},
				{UpperBound: -2.0, Count: 12.0},
				{UpperBound: -1.0, Count: 15.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [-Inf, -3]: contributes 10.0 observations (full bucket)
			// - Bucket [-3, -2]: contributes 12-10 = 2.0 observations (full bucket)
			// - Bucket [-2, -1]: contributes (-1.5-(-2))/(-1-(-2)) * (15-12) = 0.5 * 3 = 1.5 observations
			// Total: (10.0 + 2.0 + 1.5) / 100.0 = 0.135
			expected: 0.135,
		},
		{
			name:  "lower is -Inf and upper is +Inf (positive buckets)",
			lower: math.Inf(-1),
			upper: math.Inf(1),
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// Range [-Inf, +Inf] captures all observations
			expected: 1.0,
		},

		{
			name:  "lower is -Inf and upper is +Inf (negative buckets)",
			lower: math.Inf(-1),
			upper: math.Inf(+1),
			buckets: Buckets{
				{UpperBound: -3.0, Count: 10.0},
				{UpperBound: -2.0, Count: 12.0},
				{UpperBound: -1.0, Count: 15.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// Range [-Inf, +Inf] captures all observations
			expected: 1.0,
		},
		{
			name:  "lower and upper fall in last bucket (positive buckets)",
			lower: 4.0,
			upper: 5.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [3, +Inf]: contributes zero observations (no interpolation with infinite width bucket)
			// Total: 0.0 / 100.0 = 0.0
			expected: 0.0,
		},
		{
			name:  "lower and upper fall in last bucket (negative buckets)",
			lower: 0.0,
			upper: 1.0,
			buckets: Buckets{
				{UpperBound: -3.0, Count: 10.0},
				{UpperBound: -2.0, Count: 12.0},
				{UpperBound: -1.0, Count: 15.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [-1, +Inf]: contributes zero observations (no interpolation with infinite width bucket)
			// Total: 0.0 / 100.0 = 0.0
			expected: 0.0,
		},
		{
			name:  "upper falls in last bucket",
			lower: 2.0,
			upper: 5.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [2, 3]: 6-3 = 3.0 observations (full bucket)
			// - Bucket [3, +Inf]: contributes zero observations (no interpolation with infinite width bucket)
			// Total: 3.0 / 100.0 = 0.03
			expected: 0.03,
		},
		{
			name:  "upper is +Inf",
			lower: 400.0,
			upper: math.Inf(1),
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// All observations in +Inf bucket: 100-6 = 94.0 observations
			// Total: 94.0 / 100.0 = 0.94
			expected: 0.94,
		},
		{
			name:  "lower equals upper",
			lower: 2.0,
			upper: 2.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// No observations can be captured in a zero-width range
			expected: 0.0,
		},
		{
			name:  "lower greater than upper",
			lower: 3.0,
			upper: 2.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			expected: 0.0,
		},
		{
			name:  "single bucket",
			lower: 0.0,
			upper: 1.0,
			buckets: Buckets{
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [0, +Inf]: contributes zero observations (no interpolation with infinite width bucket)
			// Total: 0.0 / 100.0 = 0.0
			expected: 0.0,
		},
		{
			name:  "all zero counts",
			lower: 0.0,
			upper: 5.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 0.0},
				{UpperBound: 2.0, Count: 0.0},
				{UpperBound: 3.0, Count: 0.0},
				{UpperBound: math.Inf(1), Count: 0.0},
			},
			expected: math.NaN(),
		},
		{
			name:  "lower exactly on bucket boundary",
			lower: 2.0,
			upper: 3.5,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [2, 3]: 6-3 = 3.0 observations (full bucket)
			// - Bucket [3, +Inf]: contributes zero observations (no interpolation with infinite width bucket)
			// Total: 3.0 / 100.0 = 0.03
			expected: 0.03,
		},
		{
			name:  "upper exactly on bucket boundary",
			lower: 0.5,
			upper: 2.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [0, 1]: (1.0-0.5)/(1.0-0.0) * 1.0 = 0.5 * 1.0 = 0.5 observations
			// - Bucket [1, 2]: 3-1 = 2.0 observations (full bucket)
			// Total: (0.5 + 2.0) / 100.0 = 0.025
			expected: 0.025,
		},
		{
			name:  "both bounds exactly on bucket boundaries",
			lower: 1.0,
			upper: 3.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: 3.0, Count: 6.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [1, 2]: 3-1 = 2.0 observations (full bucket)
			// - Bucket [2, 3]: 6-3 = 3.0 observations (full bucket)
			// Total: (2.0 + 3.0) / 100.0 = 0.05
			expected: 0.05,
		},
		{
			name:  "fractional bucket bounds",
			lower: 0.1,
			upper: 0.75,
			buckets: Buckets{
				{UpperBound: 0.5, Count: 2.5},
				{UpperBound: 1.0, Count: 7.5},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [0, 0.5]: (0.5-0.1)/(0.5-0.0) * 2.5 = 0.8 * 2.5 = 2.0 observations
			// - Bucket [0.5, 1.0]: (0.75-0.5)/(1.0-0.5) * (7.5-2.5) = 0.5 * 5.0 = 2.5 observations
			// Total: (2.0 + 2.5) / 100.0 = 0.045
			expected: 0.045,
		},
		{
			name:  "range crosses zero",
			lower: -1.0,
			upper: 1.0,
			buckets: Buckets{
				{UpperBound: -2.0, Count: 5.0},
				{UpperBound: -1.0, Count: 10.0},
				{UpperBound: 0.0, Count: 15.0},
				{UpperBound: 1.0, Count: 20.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			// - Bucket [-1, 0]: 15-10 = 5.0 observations (full bucket)
			// - Bucket [0, 1]: 20-15 = 5.0 observations (full bucket)
			// Total: (5.0 + 5.0) / 100.0 = 0.1
			expected: 0.1,
		},
		{
			name:  "lower is NaN",
			lower: math.NaN(),
			upper: 1.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			expected: math.NaN(),
		},
		{
			name:  "upper is NaN",
			lower: 0.0,
			upper: math.NaN(),
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: math.Inf(1), Count: 100.0},
			},
			expected: math.NaN(),
		},
		{
			name:  "range entirely below all buckets",
			lower: -10.0,
			upper: -5.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: math.Inf(1), Count: 10.0},
			},
			expected: 0.0,
		},
		{
			name:  "range entirely above all buckets",
			lower: 5.0,
			upper: 10.0,
			buckets: Buckets{
				{UpperBound: 1.0, Count: 1.0},
				{UpperBound: 2.0, Count: 3.0},
				{UpperBound: math.Inf(1), Count: 10.0},
			},
			expected: 0.0,
		},
	} {
		t.Run(
			tc.name, func(t *testing.T) {
				actual := BucketFraction(tc.lower, tc.upper, tc.buckets)
				requireFloatEqual(t, tc.expected, actual)

				fh, err := tc.buckets.toFloatHistogram()
				if err != nil {
					t.Fatal(err)
					return
				}
				actual, _ = HistogramFraction(tc.lower, tc.upper, fh, "test", posrange.PositionRange{})
				requireFloatEqual(t, tc.expected, actual)
			},
		)
	}
}

func (buckets Buckets) toFloatHistogram() (*histogram.FloatHistogram, error) {
	tmp := convertnhcb.NewTempHistogram()
	for _, bucket := range buckets {
		if err := tmp.SetBucketCount(bucket.UpperBound, bucket.Count); err != nil {
			return nil, err
		}
	}

	h, fh, err := tmp.Convert()
	if err != nil {
		return nil, err
	}
	if h != nil {
		fh = h.ToFloat(nil)
	}
	return fh, nil
}

func requireFloatEqual(t *testing.T, expected, actual float64, msgAndArgs ...interface{}) {
	t.Helper()

	// math.NaN() != math.NaN()
	if math.IsNaN(expected) && math.IsNaN(actual) {
		return
	}
	require.Equal(t, expected, actual, msgAndArgs)
}
