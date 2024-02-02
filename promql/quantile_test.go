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
)

func TestBucketQuantile_ForcedMonotonicity(t *testing.T) {
	eps := 1e-12

	for name, tc := range map[string]struct {
		getInput       func() buckets // The buckets can be modified in-place so return a new one each time.
		expectedForced bool
		expectedFixed  bool
		expectedValues map[float64]float64
	}{
		"simple - monotonic": {
			getInput: func() buckets {
				return buckets{
					{
						upperBound: 10,
						count:      10,
					}, {
						upperBound: 15,
						count:      15,
					}, {
						upperBound: 20,
						count:      15,
					}, {
						upperBound: 30,
						count:      15,
					}, {
						upperBound: math.Inf(1),
						count:      15,
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
			getInput: func() buckets {
				return buckets{
					{
						upperBound: 10,
						count:      10,
					}, {
						upperBound: 15,
						count:      15,
					}, {
						upperBound: 20,
						count:      15.00000000001, // Simulate the case there's a small imprecision in float64.
					}, {
						upperBound: 30,
						count:      15,
					}, {
						upperBound: math.Inf(1),
						count:      15,
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
			getInput: func() buckets {
				return buckets{
					{
						upperBound: 1,
						count:      6454661.3014166197,
					}, {
						upperBound: 5,
						count:      8339611.2001912938,
					}, {
						upperBound: 10,
						count:      14118319.2444762159,
					}, {
						upperBound: 25,
						count:      14130031.5272856522,
					}, {
						upperBound: 50,
						count:      46001270.3030008152,
					}, {
						upperBound: 64,
						count:      46008473.8585563600,
					}, {
						upperBound: 80,
						count:      46008473.8585563600,
					}, {
						upperBound: 100,
						count:      46008473.8585563600,
					}, {
						upperBound: 250,
						count:      46008473.8585563600,
					}, {
						upperBound: 1000,
						count:      46008473.8585563600,
					}, {
						upperBound: math.Inf(1),
						count:      46008473.8585563600,
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
			getInput: func() buckets {
				return buckets{
					{
						upperBound: 1,
						count:      6454661.3014166225,
					}, {
						upperBound: 5,
						count:      8339611.2001912957,
					}, {
						upperBound: 10,
						count:      14118319.2444762159,
					}, {
						upperBound: 25,
						count:      14130031.5272856504,
					}, {
						upperBound: 50,
						count:      46001270.3030008227,
					}, {
						upperBound: 64,
						count:      46008473.8585563824,
					}, {
						upperBound: 80,
						count:      46008473.8585563898,
					}, {
						upperBound: 100,
						count:      46008473.8585563824,
					}, {
						upperBound: 250,
						count:      46008473.8585563824,
					}, {
						upperBound: 1000,
						count:      46008473.8585563898,
					}, {
						upperBound: math.Inf(1),
						count:      46008473.8585563824,
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
			getInput: func() buckets {
				return buckets{
					{
						upperBound: 0.005,
						count:      9.6,
					}, {
						upperBound: 0.01,
						count:      9.688888889,
					}, {
						upperBound: 0.025,
						count:      9.755555556,
					}, {
						upperBound: 0.05,
						count:      9.844444444,
					}, {
						upperBound: 0.1,
						count:      9.888888889,
					}, {
						upperBound: 0.25,
						count:      9.888888889,
					}, {
						upperBound: 0.5,
						count:      9.888888889,
					}, {
						upperBound: 1,
						count:      9.888888889,
					}, {
						upperBound: 2.5,
						count:      9.888888889,
					}, {
						upperBound: 5,
						count:      9.888888889,
					}, {
						upperBound: 10,
						count:      9.888888889,
					}, {
						upperBound: 25,
						count:      9.888888889,
					}, {
						upperBound: 50,
						count:      9.888888889,
					}, {
						upperBound: 100,
						count:      9.888888889,
					}, {
						upperBound: math.Inf(1),
						count:      9.888888889,
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
			getInput: func() buckets {
				return buckets{
					{
						upperBound: 0.005,
						count:      9.6,
					}, {
						upperBound: 0.01,
						count:      9.688888889,
					}, {
						upperBound: 0.025,
						count:      9.755555556,
					}, {
						upperBound: 0.05,
						count:      9.844444444,
					}, {
						upperBound: 0.1,
						count:      9.888888889,
					}, {
						upperBound: 0.25,
						count:      9.888888889,
					}, {
						upperBound: 0.5,
						count:      9.888888889,
					}, {
						upperBound: 1,
						count:      9.888888889,
					}, {
						upperBound: 2.5,
						count:      9.888888889,
					}, {
						upperBound: 5,
						count:      9.888888889,
					}, {
						upperBound: 10,
						count:      9.888888889001, // Simulate the case there's a small imprecision in float64.
					}, {
						upperBound: 25,
						count:      9.888888889,
					}, {
						upperBound: 50,
						count:      9.888888888999, // Simulate the case there's a small imprecision in float64.
					}, {
						upperBound: 100,
						count:      9.888888889,
					}, {
						upperBound: math.Inf(1),
						count:      9.888888889,
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
				res, forced, fixed := bucketQuantile(q, tc.getInput())
				require.Equal(t, tc.expectedForced, forced)
				require.Equal(t, tc.expectedFixed, fixed)
				require.InEpsilon(t, v, res, eps)
			}
		})
	}
}
