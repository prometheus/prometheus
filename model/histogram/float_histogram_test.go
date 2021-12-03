// Copyright 2021 The Prometheus Authors
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

package histogram

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReverseFloatBucketIterator(t *testing.T) {
	h := &FloatHistogram{
		Count:         405,
		ZeroCount:     102,
		ZeroThreshold: 0.001,
		Sum:           1008.4,
		Schema:        1,
		PositiveSpans: []Span{
			{Offset: 0, Length: 4},
			{Offset: 1, Length: 0},
			{Offset: 3, Length: 3},
			{Offset: 3, Length: 0},
			{Offset: 2, Length: 0},
			{Offset: 5, Length: 3},
		},
		PositiveBuckets: []float64{100, 344, 123, 55, 3, 63, 2, 54, 235, 33},
		NegativeSpans: []Span{
			{Offset: 0, Length: 3},
			{Offset: 1, Length: 0},
			{Offset: 3, Length: 0},
			{Offset: 3, Length: 4},
			{Offset: 2, Length: 0},
			{Offset: 5, Length: 3},
		},
		NegativeBuckets: []float64{10, 34, 1230, 54, 67, 63, 2, 554, 235, 33},
	}

	// Assuming that the regular iterator is correct.

	// Positive buckets.
	var expBuckets, actBuckets []FloatBucket
	it := h.PositiveBucketIterator()
	for it.Next() {
		// Append in reverse to check reversed list.
		expBuckets = append([]FloatBucket{it.At()}, expBuckets...)
	}
	it = h.PositiveReverseBucketIterator()
	for it.Next() {
		actBuckets = append(actBuckets, it.At())
	}
	require.Greater(t, len(expBuckets), 0)
	require.Greater(t, len(actBuckets), 0)
	require.Equal(t, expBuckets, actBuckets)

	// Negative buckets.
	expBuckets = expBuckets[:0]
	actBuckets = actBuckets[:0]
	it = h.NegativeBucketIterator()
	for it.Next() {
		// Append in reverse to check reversed list.
		expBuckets = append([]FloatBucket{it.At()}, expBuckets...)
	}
	it = h.NegativeReverseBucketIterator()
	for it.Next() {
		actBuckets = append(actBuckets, it.At())
	}
	require.Greater(t, len(expBuckets), 0)
	require.Greater(t, len(actBuckets), 0)
	require.Equal(t, expBuckets, actBuckets)
}

func TestAllFloatBucketIterator(t *testing.T) {
	cases := []struct {
		h FloatHistogram
		// To determine the expected buckets.
		includeNeg, includeZero, includePos bool
	}{
		{
			h: FloatHistogram{
				Count:         405,
				ZeroCount:     102,
				ZeroThreshold: 0.001,
				Sum:           1008.4,
				Schema:        1,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 1, Length: 0},
					{Offset: 3, Length: 3},
					{Offset: 3, Length: 0},
					{Offset: 2, Length: 0},
					{Offset: 5, Length: 3},
				},
				PositiveBuckets: []float64{100, 344, 123, 55, 3, 63, 2, 54, 235, 33},
				NegativeSpans: []Span{
					{Offset: 0, Length: 3},
					{Offset: 1, Length: 0},
					{Offset: 3, Length: 0},
					{Offset: 3, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 5, Length: 3},
				},
				NegativeBuckets: []float64{10, 34, 1230, 54, 67, 63, 2, 554, 235, 33},
			},
			includeNeg:  true,
			includeZero: true,
			includePos:  true,
		},
		{
			h: FloatHistogram{
				Count:         405,
				ZeroCount:     102,
				ZeroThreshold: 0.001,
				Sum:           1008.4,
				Schema:        1,
				NegativeSpans: []Span{
					{Offset: 0, Length: 3},
					{Offset: 1, Length: 0},
					{Offset: 3, Length: 0},
					{Offset: 3, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 5, Length: 3},
				},
				NegativeBuckets: []float64{10, 34, 1230, 54, 67, 63, 2, 554, 235, 33},
			},
			includeNeg:  true,
			includeZero: true,
			includePos:  false,
		},
		{
			h: FloatHistogram{
				Count:         405,
				ZeroCount:     102,
				ZeroThreshold: 0.001,
				Sum:           1008.4,
				Schema:        1,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 1, Length: 0},
					{Offset: 3, Length: 3},
					{Offset: 3, Length: 0},
					{Offset: 2, Length: 0},
					{Offset: 5, Length: 3},
				},
				PositiveBuckets: []float64{100, 344, 123, 55, 3, 63, 2, 54, 235, 33},
			},
			includeNeg:  false,
			includeZero: true,
			includePos:  true,
		},
		{
			h: FloatHistogram{
				Count:         405,
				ZeroCount:     102,
				ZeroThreshold: 0.001,
				Sum:           1008.4,
				Schema:        1,
			},
			includeNeg:  false,
			includeZero: true,
			includePos:  false,
		},
		{
			h: FloatHistogram{
				Count:         405,
				ZeroCount:     0,
				ZeroThreshold: 0.001,
				Sum:           1008.4,
				Schema:        1,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 1, Length: 0},
					{Offset: 3, Length: 3},
					{Offset: 3, Length: 0},
					{Offset: 2, Length: 0},
					{Offset: 5, Length: 3},
				},
				PositiveBuckets: []float64{100, 344, 123, 55, 3, 63, 2, 54, 235, 33},
				NegativeSpans: []Span{
					{Offset: 0, Length: 3},
					{Offset: 1, Length: 0},
					{Offset: 3, Length: 0},
					{Offset: 3, Length: 4},
					{Offset: 2, Length: 0},
					{Offset: 5, Length: 3},
				},
				NegativeBuckets: []float64{10, 34, 1230, 54, 67, 63, 2, 554, 235, 33},
			},
			includeNeg:  true,
			includeZero: false,
			includePos:  true,
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			var expBuckets, actBuckets []FloatBucket

			if c.includeNeg {
				it := c.h.NegativeReverseBucketIterator()
				for it.Next() {
					expBuckets = append(expBuckets, it.At())
				}
			}
			if c.includeZero {
				expBuckets = append(expBuckets, FloatBucket{
					Lower:          -c.h.ZeroThreshold,
					Upper:          c.h.ZeroThreshold,
					LowerInclusive: true,
					UpperInclusive: true,
					Count:          c.h.ZeroCount,
					Index:          math.MinInt32,
				})
			}
			if c.includePos {
				it := c.h.PositiveBucketIterator()
				for it.Next() {
					expBuckets = append(expBuckets, it.At())
				}
			}

			it := c.h.AllFloatBucketIterator()
			for it.Next() {
				actBuckets = append(actBuckets, it.At())
			}

			require.Equal(t, expBuckets, actBuckets)
		})
	}
}
