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

	"github.com/stretchr/testify/assert"
)

func TestBucketQuantile_ForcedMonotonicity(t *testing.T) {
	eps := 1e-12

	t.Run("simple - monotonic", func(t *testing.T) {
		// The buckets can be modified in-place so return each time a new one.
		getInput := func() buckets {
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
		}

		res, forced := bucketQuantile(1, getInput())
		assert.False(t, forced)
		assert.Equal(t, 15., res)

		res, forced = bucketQuantile(0.99, getInput())
		assert.False(t, forced)
		assert.Equal(t, 14.85, res)

		res, forced = bucketQuantile(0.9, getInput())
		assert.False(t, forced)
		assert.Equal(t, 13.5, res)

		res, forced = bucketQuantile(0.5, getInput())
		assert.False(t, forced)
		assert.Equal(t, 7.5, res)
	})

	t.Run("simple - non-monotonic middle", func(t *testing.T) {
		// The buckets can be modified in-place so return each time a new one.
		getInput := func() buckets {
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
		}

		res, forced := bucketQuantile(1, getInput())
		assert.True(t, forced)
		assert.Equal(t, 15., res)

		res, forced = bucketQuantile(0.99, getInput())
		assert.True(t, forced)
		assert.Equal(t, 14.85, res)

		res, forced = bucketQuantile(0.9, getInput())
		assert.True(t, forced)
		assert.Equal(t, 13.5, res)

		res, forced = bucketQuantile(0.5, getInput())
		assert.True(t, forced)
		assert.Equal(t, 7.5, res)
	})

	t.Run("real example - monotonic", func(t *testing.T) {
		// The buckets can be modified in-place so return each time a new one.
		getInput := func() buckets {
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
		}

		res, forced := bucketQuantile(1, getInput())
		assert.False(t, forced)
		assert.Equal(t, 64., res)

		res, forced = bucketQuantile(0.99, getInput())
		assert.False(t, forced)
		assert.InEpsilon(t, 49.64475715376406, res, eps)

		res, forced = bucketQuantile(0.9, getInput())
		assert.False(t, forced)
		assert.InEpsilon(t, 46.39671690938454, res, eps)

		res, forced = bucketQuantile(0.5, getInput())
		assert.False(t, forced)
		assert.InEpsilon(t, 31.96098248992002, res, eps)
	})

	t.Run("real example - non-monotonic", func(t *testing.T) {
		// The buckets can be modified in-place so return each time a new one.
		getInput := func() buckets {
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
		}

		res, forced := bucketQuantile(1, getInput())
		assert.True(t, forced)
		assert.Equal(t, 64., res)

		res, forced = bucketQuantile(0.99, getInput())
		assert.True(t, forced)
		assert.InEpsilon(t, 49.64475715376406, res, eps)

		res, forced = bucketQuantile(0.9, getInput())
		assert.True(t, forced)
		assert.InEpsilon(t, 46.39671690938454, res, eps)

		res, forced = bucketQuantile(0.5, getInput())
		assert.True(t, forced)
		assert.InEpsilon(t, 31.96098248992002, res, eps)
	})

	t.Run("real example 2 - monotonic", func(t *testing.T) {
		// The buckets can be modified in-place so return each time a new one.
		getInput := func() buckets {
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
		}

		res, forced := bucketQuantile(1, getInput())
		assert.False(t, forced)
		assert.Equal(t, 0.1, res)

		res, forced = bucketQuantile(0.99, getInput())
		assert.False(t, forced)
		assert.InEpsilon(t, 0.03468750000281261, res, eps)

		res, forced = bucketQuantile(0.9, getInput())
		assert.False(t, forced)
		assert.InEpsilon(t, 0.00463541666671875, res, eps)

		res, forced = bucketQuantile(0.5, getInput())
		assert.False(t, forced)
		assert.InEpsilon(t, 0.0025752314815104174, res, eps)
	})

	t.Run("real example 2 - non-monotonic", func(t *testing.T) {
		// The buckets can be modified in-place so return each time a new one.
		getInput := func() buckets {
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
		}

		res, forced := bucketQuantile(1, getInput())
		assert.True(t, forced)
		assert.Equal(t, 0.1, res)

		res, forced = bucketQuantile(0.99, getInput())
		assert.True(t, forced)
		assert.InEpsilon(t, 0.03468750000281261, res, eps)

		res, forced = bucketQuantile(0.9, getInput())
		assert.True(t, forced)
		assert.InEpsilon(t, 0.00463541666671875, res, eps)

		res, forced = bucketQuantile(0.5, getInput())
		assert.True(t, forced)
		assert.InEpsilon(t, 0.0025752314815104174, res, eps)
	})
}
