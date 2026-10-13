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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/promql/parser/posrange"
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
				quantile, forced, fixed, _, _, _ := BucketQuantile(q, tc.getInput())
				require.Equal(t, tc.expectedForced, forced)
				require.Equal(t, tc.expectedFixed, fixed)
				require.InEpsilon(t, v, quantile, eps)
			}
		})
	}
}

// TestTrimBuckets_HistogramFractionCrossCheck checks that `h </ x` and `h >/ x` match histogram_fraction(-Inf, x, h) and histogram_fraction(x, +Inf, h) times histogram_count(h).
func TestTrimBuckets_HistogramFractionCrossCheck(t *testing.T) {
	testCases := []struct {
		name       string
		h          *histogram.FloatHistogram
		thresholds []float64
	}{
		{
			// Bucket bounds: (-64,-32] (-32,-16] (-16,-8] (-4,-2] (-2,-1] (-0.5,0.5] (0.5,1] (1,2] (2,4] (16,32] (32,64] (64,128].
			name: "exponential, positive and negative buckets, zero bucket, multiple spans, empty buckets",
			h: &histogram.FloatHistogram{
				Schema:          0,
				Count:           56,
				Sum:             113,
				ZeroThreshold:   0.5,
				ZeroCount:       7,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}, {Offset: 2, Length: 3}},
				PositiveBuckets: []float64{3, 11, 2, 9, 0, 6},
				NegativeSpans:   []histogram.Span{{Offset: 1, Length: 2}, {Offset: 1, Length: 3}},
				NegativeBuckets: []float64{4, 1, 8, 0, 5},
			},
			thresholds: []float64{
				math.Inf(-1), -100,
				-64, -32, -16, -8, -4, -2, -1, -0.5, // Bucket boundaries.
				-48, -12, -6, -3, -0.75, -0.3, // Inside buckets and inside gaps between spans.
				0,
				0.3, 0.73, 1.87, 3.3, 12, 100, // Inside buckets and inside gaps between spans.
				0.5, 1, 2, 4, 16, 32, 64, 128, // Bucket boundaries.
				200, math.Inf(1),
			},
		},
		{
			// Bucket bounds: (-Inf,1] (1,2] (2,4] (8,16] (16,+Inf].
			name: "custom buckets (NHCB), underflow bucket to -Inf and overflow bucket to +Inf",
			h: &histogram.FloatHistogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           36,
				Sum:             181,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}, {Offset: 1, Length: 2}},
				PositiveBuckets: []float64{2, 7, 5, 13, 9},
				CustomValues:    []float64{1, 2, 4, 8, 16},
			},
			// Finite negative thresholds fall inside the underflow bucket (-Inf, 1] and are covered by the known divergences below, not here.
			thresholds: []float64{
				math.Inf(-1), 0,
				1, 2, 4, 8, 16, // Bucket boundaries.
				0.3, 0.73, 1.87, 3.3, 6, 12, 20, // Inside buckets, inside the gap between spans and inside the overflow bucket.
				math.Inf(1),
			},
		},
		{
			// Bucket bounds: (0.5,0.5946] (0.5946,0.7071] (0.7071,0.8409] (0.8409,1] (1.6818,2] (2,2.3784] (2.3784,2.8284].
			name: "only positive buckets, no zero bucket, multiple spans, empty bucket",
			h: &histogram.FloatHistogram{
				Schema:          2,
				Count:           36,
				Sum:             49,
				PositiveSpans:   []histogram.Span{{Offset: -3, Length: 4}, {Offset: 3, Length: 3}},
				PositiveBuckets: []float64{1, 6, 0, 9, 4, 11, 5},
			},
			thresholds: []float64{
				math.Inf(-1), -12, -1.7, 0,
				0.5, 1, 2, // Bucket boundaries.
				0.6, 0.9, 1.5, 2.5, 3, // Inside buckets and inside the gap between spans.
				math.Inf(1),
			},
		},
		{
			// Bucket bounds: (-8,-5.6569] (-5.6569,-4] (-2,-1.4142] (-1.4142,-1] (-1,-0.7071].
			name: "only negative buckets, no zero bucket, multiple spans",
			h: &histogram.FloatHistogram{
				Schema:          1,
				Count:           29,
				Sum:             -87,
				NegativeSpans:   []histogram.Span{{Offset: 0, Length: 3}, {Offset: 2, Length: 2}},
				NegativeBuckets: []float64{4, 9, 3, 8, 5},
			},
			thresholds: []float64{
				math.Inf(-1), -20,
				-8, -4, -2, -1, // Bucket boundaries.
				-6, -3, -1.7, -0.9, -0.5, // Inside buckets and inside the gap between spans.
				0, 0.5, math.Inf(1),
			},
		},
		{
			name: "zero bucket only",
			h: &histogram.FloatHistogram{
				Schema:        0,
				Count:         9,
				Sum:           0.4,
				ZeroThreshold: 0.25,
				ZeroCount:     9,
			},
			thresholds: []float64{
				math.Inf(-1), -1,
				-0.25, 0.25, // Bucket boundaries.
				-0.1, 0, 0.1, 1, math.Inf(1),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			total := tc.h.Count
			// Guard against a fixture whose Count does not match its buckets, which would make every comparison below meaningless.
			var bucketCount float64
			for it := tc.h.AllBucketIterator(); it.Next(); {
				bucketCount += it.At().Count
			}
			require.Equal(t, total, bucketCount, "test case has an inconsistent Count")

			for _, x := range tc.thresholds {
				t.Run(fmt.Sprintf("x=%v", x), func(t *testing.T) {
					// TrimBuckets mutates its receiver, so it needs a copy; HistogramFraction does not.
					lower := tc.h.Copy().TrimBuckets(x, true) // h </ x
					fractionBelow, annos := HistogramFraction(math.Inf(-1), x, tc.h, "", posrange.PositionRange{})
					require.Empty(t, annos)
					if fractionBelow == 0 {
						require.Zero(t, lower.Count)
					} else {
						require.InEpsilon(t, fractionBelow*total, lower.Count, 1e-9)
					}

					upper := tc.h.Copy().TrimBuckets(x, false) // h >/ x
					fractionAbove, annos := HistogramFraction(x, math.Inf(1), tc.h, "", posrange.PositionRange{})
					require.Empty(t, annos)
					if fractionAbove == 0 {
						require.Zero(t, upper.Count)
					} else {
						require.InEpsilon(t, fractionAbove*total, upper.Count, 1e-9)
					}
				})
			}
		})
	}

	t.Run("zero-count histogram: NaN fraction vs empty trim", func(t *testing.T) {
		h := &histogram.FloatHistogram{}
		fraction, annos := HistogramFraction(math.Inf(-1), 0.5, h, "", posrange.PositionRange{})
		require.Empty(t, annos)
		require.True(t, math.IsNaN(fraction))

		trimmed := h.Copy().TrimBuckets(0.5, true)
		require.Zero(t, trimmed.Count)
	})

	// Known, pre-existing divergences between the two operations, asserted here so that a future change to either side fails the test instead of silently changing behaviour.
	divergences := []struct {
		name string
		h    *histogram.FloatHistogram
		x    float64
		// isUpperTrim selects `h </ x` (true) or `h >/ x` (false), and the matching histogram_fraction(-Inf, x, h) or histogram_fraction(x, +Inf, h).
		isUpperTrim      bool
		expectedCount    float64
		expectedFraction float64
		why              string
	}{
		{
			// TrimBuckets conservatively drops a bucket with an infinite bound, while HistogramFraction credits it to one side, since it treats any bucket straddling zero as having an effective bound of 0.
			name: "threshold inside an infinite-bound bucket",
			h: &histogram.FloatHistogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           20,
				Sum:             50,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 5}},
				PositiveBuckets: []float64{2, 3, 5, 4, 6},
				CustomValues:    []float64{1, 2, 4, 8, 16},
			},
			x:                -6,
			isUpperTrim:      false,
			expectedCount:    18, // TrimBuckets drops the whole underflow bucket.
			expectedFraction: 1,  // HistogramFraction credits the whole underflow bucket as being above x.
			why:              "underflow bucket",
		},
		{
			// TrimBuckets decides whether the zero bucket is one-sided from the bucket counts, so an all-empty positive span makes it treat the zero bucket as ending at 0, while HistogramFraction decides from the length of the bucket slices, so the empty span still counts as having positive buckets and the zero bucket keeps straddling 0.
			name: "empty positive bucket, non-empty negative bucket, non-empty zero bucket",
			h: &histogram.FloatHistogram{
				Schema:          0,
				ZeroThreshold:   0.001,
				ZeroCount:       3,
				Count:           8,
				Sum:             -10,
				NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				NegativeBuckets: []float64{5},
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []float64{0}, // Present but empty.
			},
			x:                0,
			isUpperTrim:      true,
			expectedCount:    8,      // TrimBuckets keeps the whole zero bucket.
			expectedFraction: 0.8125, // HistogramFraction splits the zero bucket in half: (5+1.5)/8.
			why:              "zero bucket",
		},
	}

	for _, tc := range divergences {
		t.Run("known divergence: "+tc.name, func(t *testing.T) {
			trimmed := tc.h.Copy().TrimBuckets(tc.x, tc.isUpperTrim)
			require.Equal(t, tc.expectedCount, trimmed.Count, "TrimBuckets handles the %s differently", tc.why)

			lower, upper := math.Inf(-1), tc.x
			if !tc.isUpperTrim {
				lower, upper = tc.x, math.Inf(1)
			}
			fraction, annos := HistogramFraction(lower, upper, tc.h, "", posrange.PositionRange{})
			require.Empty(t, annos)
			require.Equal(t, tc.expectedFraction, fraction, "HistogramFraction handles the %s differently", tc.why)
		})
	}
}
