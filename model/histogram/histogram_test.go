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

func TestHistogramString(t *testing.T) {
	cases := []struct {
		histogram      Histogram
		expectedString string
	}{
		{
			histogram: Histogram{
				Schema: 0,
			},
			expectedString: "{count:0, sum:0}",
		},
		{
			histogram: Histogram{
				Schema:        0,
				Count:         9,
				Sum:           -3.1415,
				ZeroCount:     12,
				ZeroThreshold: 0.001,
				NegativeSpans: []Span{
					{Offset: 0, Length: 5},
					{Offset: 1, Length: 1},
				},
				NegativeBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expectedString: "{count:9, sum:-3.1415, [-64,-32):1, [-16,-8):1, [-8,-4):2, [-4,-2):1, [-2,-1):3, [-1,-0.5):1, [-0.001,0.001]:12}",
		},
		{
			histogram: Histogram{
				Schema: 0,
				Count:  19,
				Sum:    2.7,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				NegativeSpans: []Span{
					{Offset: 0, Length: 5},
					{Offset: 1, Length: 0},
					{Offset: 0, Length: 1},
				},
				NegativeBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expectedString: "{count:19, sum:2.7, [-64,-32):1, [-16,-8):1, [-8,-4):2, [-4,-2):1, [-2,-1):3, [-1,-0.5):1, (0.5,1]:1, (1,2]:3, (2,4]:1, (4,8]:2, (8,16]:1, (16,32]:1, (32,64]:1}",
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actualString := c.histogram.String()
			require.Equal(t, c.expectedString, actualString)
		})
	}
}

func TestCumulativeBucketIterator(t *testing.T) {
	cases := []struct {
		histogram       Histogram
		expectedBuckets []Bucket[uint64]
	}{
		{
			histogram: Histogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
			},
			expectedBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 1, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: math.Inf(-1), Upper: 2, Count: 3, LowerInclusive: true, UpperInclusive: true, Index: 1},

				{Lower: math.Inf(-1), Upper: 4, Count: 3, LowerInclusive: true, UpperInclusive: true, Index: 2},

				{Lower: math.Inf(-1), Upper: 8, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: 3},
				{Lower: math.Inf(-1), Upper: 16, Count: 5, LowerInclusive: true, UpperInclusive: true, Index: 4},
			},
		},
		{
			histogram: Histogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 5},
					{Offset: 1, Length: 1},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expectedBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 1, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: math.Inf(-1), Upper: 2, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: 1},
				{Lower: math.Inf(-1), Upper: 4, Count: 5, LowerInclusive: true, UpperInclusive: true, Index: 2},
				{Lower: math.Inf(-1), Upper: 8, Count: 7, LowerInclusive: true, UpperInclusive: true, Index: 3},

				{Lower: math.Inf(-1), Upper: 16, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 4},

				{Lower: math.Inf(-1), Upper: 32, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 5},
				{Lower: math.Inf(-1), Upper: 64, Count: 9, LowerInclusive: true, UpperInclusive: true, Index: 6},
			},
		},
		{
			histogram: Histogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 7},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
			},
			expectedBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 1, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: math.Inf(-1), Upper: 2, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: 1},
				{Lower: math.Inf(-1), Upper: 4, Count: 5, LowerInclusive: true, UpperInclusive: true, Index: 2},
				{Lower: math.Inf(-1), Upper: 8, Count: 7, LowerInclusive: true, UpperInclusive: true, Index: 3},
				{Lower: math.Inf(-1), Upper: 16, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 4},
				{Lower: math.Inf(-1), Upper: 32, Count: 9, LowerInclusive: true, UpperInclusive: true, Index: 5},
				{Lower: math.Inf(-1), Upper: 64, Count: 10, LowerInclusive: true, UpperInclusive: true, Index: 6},
			},
		},
		{
			histogram: Histogram{
				Schema: 3,
				PositiveSpans: []Span{
					{Offset: -5, Length: 2}, // -5 -4
					{Offset: 2, Length: 3},  // -1 0 1
					{Offset: 2, Length: 2},  // 4 5
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 3},
			},
			expectedBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 0.6484197773255048, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: -5},
				{Lower: math.Inf(-1), Upper: 0.7071067811865475, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: -4},

				{Lower: math.Inf(-1), Upper: 0.7711054127039704, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: -3},
				{Lower: math.Inf(-1), Upper: 0.8408964152537144, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: -2},

				{Lower: math.Inf(-1), Upper: 0.9170040432046711, Count: 5, LowerInclusive: true, UpperInclusive: true, Index: -1},
				{Lower: math.Inf(-1), Upper: 1, Count: 7, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: math.Inf(-1), Upper: 1.0905077326652577, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 1},

				{Lower: math.Inf(-1), Upper: 1.189207115002721, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 2},
				{Lower: math.Inf(-1), Upper: 1.2968395546510096, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 3},

				{Lower: math.Inf(-1), Upper: 1.414213562373095, Count: 9, LowerInclusive: true, UpperInclusive: true, Index: 4},
				{Lower: math.Inf(-1), Upper: 1.5422108254079407, Count: 13, LowerInclusive: true, UpperInclusive: true, Index: 5},
			},
		},
		{
			histogram: Histogram{
				Schema: -2,
				PositiveSpans: []Span{
					{Offset: -2, Length: 4}, // -2 -1 0 1
					{Offset: 2, Length: 2},  // 4 5
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expectedBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 0.00390625, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: -2},
				{Lower: math.Inf(-1), Upper: 0.0625, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: -1},
				{Lower: math.Inf(-1), Upper: 1, Count: 5, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: math.Inf(-1), Upper: 16, Count: 7, LowerInclusive: true, UpperInclusive: true, Index: 1},

				{Lower: math.Inf(-1), Upper: 256, Count: 7, LowerInclusive: true, UpperInclusive: true, Index: 2},
				{Lower: math.Inf(-1), Upper: 4096, Count: 7, LowerInclusive: true, UpperInclusive: true, Index: 3},

				{Lower: math.Inf(-1), Upper: 65536, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 4},
				{Lower: math.Inf(-1), Upper: 1048576, Count: 9, LowerInclusive: true, UpperInclusive: true, Index: 5},
			},
		},
		{
			histogram: Histogram{
				Schema: -1,
				PositiveSpans: []Span{
					{Offset: -2, Length: 5}, // -2 -1 0 1 2
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1},
			},
			expectedBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 0.0625, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: -2},
				{Lower: math.Inf(-1), Upper: 0.25, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: -1},
				{Lower: math.Inf(-1), Upper: 1, Count: 5, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: math.Inf(-1), Upper: 4, Count: 7, LowerInclusive: true, UpperInclusive: true, Index: 1},
				{Lower: math.Inf(-1), Upper: 16, Count: 8, LowerInclusive: true, UpperInclusive: true, Index: 2},
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			it := c.histogram.CumulativeBucketIterator()
			actualBuckets := make([]Bucket[uint64], 0, len(c.expectedBuckets))
			for it.Next() {
				actualBuckets = append(actualBuckets, it.At())
			}
			require.Equal(t, c.expectedBuckets, actualBuckets)
		})
	}
}

func TestRegularBucketIterator(t *testing.T) {
	cases := []struct {
		histogram               Histogram
		expectedPositiveBuckets []Bucket[uint64]
		expectedNegativeBuckets []Bucket[uint64]
	}{
		{
			histogram: Histogram{
				Schema: 0,
			},
			expectedPositiveBuckets: []Bucket[uint64]{},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
		{
			histogram: Histogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: 0.5, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 2, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 4, Upper: 8, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 8, Upper: 16, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
			},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
		{
			histogram: Histogram{
				Schema: 0,
				NegativeSpans: []Span{
					{Offset: 0, Length: 5},
					{Offset: 1, Length: 1},
				},
				NegativeBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expectedPositiveBuckets: []Bucket[uint64]{},
			expectedNegativeBuckets: []Bucket[uint64]{
				{Lower: -1, Upper: -0.5, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 0},
				{Lower: -2, Upper: -1, Count: 3, LowerInclusive: true, UpperInclusive: false, Index: 1},
				{Lower: -4, Upper: -2, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 2},
				{Lower: -8, Upper: -4, Count: 2, LowerInclusive: true, UpperInclusive: false, Index: 3},
				{Lower: -16, Upper: -8, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 4},

				{Lower: -64, Upper: -32, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 6},
			},
		},
		{
			histogram: Histogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				NegativeSpans: []Span{
					{Offset: 0, Length: 5},
					{Offset: 1, Length: 0},
					{Offset: 0, Length: 1},
				},
				NegativeBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: 0.5, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 2, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: 1},
				{Lower: 2, Upper: 4, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 2},
				{Lower: 4, Upper: 8, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 8, Upper: 16, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 16, Upper: 32, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 5},
				{Lower: 32, Upper: 64, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 6},
			},
			expectedNegativeBuckets: []Bucket[uint64]{
				{Lower: -1, Upper: -0.5, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 0},
				{Lower: -2, Upper: -1, Count: 3, LowerInclusive: true, UpperInclusive: false, Index: 1},
				{Lower: -4, Upper: -2, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 2},
				{Lower: -8, Upper: -4, Count: 2, LowerInclusive: true, UpperInclusive: false, Index: 3},
				{Lower: -16, Upper: -8, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 4},

				{Lower: -64, Upper: -32, Count: 1, LowerInclusive: true, UpperInclusive: false, Index: 6},
			},
		},
		{
			histogram: Histogram{
				Schema: 3,
				PositiveSpans: []Span{
					{Offset: -5, Length: 2}, // -5 -4
					{Offset: 2, Length: 3},  // -1 0 1
					{Offset: 2, Length: 2},  // 4 5
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 3},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: 0.5946035575013605, Upper: 0.6484197773255048, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -5},
				{Lower: 0.6484197773255048, Upper: 0.7071067811865475, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: -4},

				{Lower: 0.8408964152537144, Upper: 0.9170040432046711, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -1},
				{Lower: 0.9170040432046711, Upper: 1, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 1.0905077326652577, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 1.2968395546510096, Upper: 1.414213562373095, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 1.414213562373095, Upper: 1.5422108254079407, Count: 4, LowerInclusive: false, UpperInclusive: true, Index: 5},
			},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
		{
			histogram: Histogram{
				Schema: -2,
				PositiveSpans: []Span{
					{Offset: -2, Length: 4}, // -2 -1 0 1
					{Offset: 2, Length: 2},  // 4 5
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: 0.000244140625, Upper: 0.00390625, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -2},
				{Lower: 0.00390625, Upper: 0.0625, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: -1},
				{Lower: 0.0625, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 16, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 4096, Upper: 65536, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 65536, Upper: 1048576, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 5},
			},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
		{
			histogram: Histogram{
				Schema: -1,
				PositiveSpans: []Span{
					{Offset: -2, Length: 5}, // -2 -1 0 1 2
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: 0.015625, Upper: 0.0625, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -2},
				{Lower: 0.0625, Upper: 0.25, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: -1},
				{Lower: 0.25, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 4, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},
				{Lower: 4, Upper: 16, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 2},
			},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			it := c.histogram.PositiveBucketIterator()
			actualPositiveBuckets := make([]Bucket[uint64], 0, len(c.expectedPositiveBuckets))
			for it.Next() {
				actualPositiveBuckets = append(actualPositiveBuckets, it.At())
			}
			require.Equal(t, c.expectedPositiveBuckets, actualPositiveBuckets)
			it = c.histogram.NegativeBucketIterator()
			actualNegativeBuckets := make([]Bucket[uint64], 0, len(c.expectedNegativeBuckets))
			for it.Next() {
				actualNegativeBuckets = append(actualNegativeBuckets, it.At())
			}
			require.Equal(t, c.expectedNegativeBuckets, actualNegativeBuckets)
		})
	}
}

func TestHistogramToFloat(t *testing.T) {
	h := Histogram{
		Schema:        3,
		Count:         61,
		Sum:           2.7,
		ZeroThreshold: 0.1,
		ZeroCount:     42,
		PositiveSpans: []Span{
			{Offset: 0, Length: 4},
			{Offset: 0, Length: 0},
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
		NegativeSpans: []Span{
			{Offset: 0, Length: 5},
			{Offset: 1, Length: 0},
			{Offset: 0, Length: 1},
		},
		NegativeBuckets: []int64{1, 2, -2, 1, -1, 0},
	}
	fh := h.ToFloat()

	require.Equal(t, h.String(), fh.String())
}

// TestHistogramMatches tests both Histogram and FloatHistogram.
func TestHistogramMatches(t *testing.T) {
	h1 := Histogram{
		Schema:        3,
		Count:         61,
		Sum:           2.7,
		ZeroThreshold: 0.1,
		ZeroCount:     42,
		PositiveSpans: []Span{
			{Offset: 0, Length: 4},
			{Offset: 10, Length: 3},
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
		NegativeSpans: []Span{
			{Offset: 0, Length: 4},
			{Offset: 10, Length: 3},
		},
		NegativeBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
	}

	equals := func(h1, h2 Histogram) {
		require.True(t, h1.Equals(&h2))
		require.True(t, h2.Equals(&h1))
		h1f, h2f := h1.ToFloat(), h2.ToFloat()
		require.True(t, h1f.Equals(h2f))
		require.True(t, h2f.Equals(h1f))
	}
	notEquals := func(h1, h2 Histogram) {
		require.False(t, h1.Equals(&h2))
		require.False(t, h2.Equals(&h1))
		h1f, h2f := h1.ToFloat(), h2.ToFloat()
		require.False(t, h1f.Equals(h2f))
		require.False(t, h2f.Equals(h1f))
	}

	h2 := h1.Copy()
	equals(h1, *h2)

	// Changed spans but same layout.
	h2.PositiveSpans = append(h2.PositiveSpans, Span{Offset: 5})
	h2.NegativeSpans = append(h2.NegativeSpans, Span{Offset: 2})
	equals(h1, *h2)
	// Adding empty spans in between.
	h2.PositiveSpans[1].Offset = 6
	h2.PositiveSpans = []Span{
		h2.PositiveSpans[0],
		{Offset: 1},
		{Offset: 3},
		h2.PositiveSpans[1],
		h2.PositiveSpans[2],
	}
	h2.NegativeSpans[1].Offset = 5
	h2.NegativeSpans = []Span{
		h2.NegativeSpans[0],
		{Offset: 2},
		{Offset: 3},
		h2.NegativeSpans[1],
		h2.NegativeSpans[2],
	}
	equals(h1, *h2)

	// All mismatches.
	notEquals(h1, Histogram{})

	h2.Schema = 1
	notEquals(h1, *h2)

	h2 = h1.Copy()
	h2.Count++
	notEquals(h1, *h2)

	h2 = h1.Copy()
	h2.Sum++
	notEquals(h1, *h2)

	h2 = h1.Copy()
	h2.ZeroThreshold++
	notEquals(h1, *h2)

	h2 = h1.Copy()
	h2.ZeroCount++
	notEquals(h1, *h2)

	// Changing value of buckets.
	h2 = h1.Copy()
	h2.PositiveBuckets[len(h2.PositiveBuckets)-1]++
	notEquals(h1, *h2)
	h2 = h1.Copy()
	h2.NegativeBuckets[len(h2.NegativeBuckets)-1]++
	notEquals(h1, *h2)

	// Changing bucket layout.
	h2 = h1.Copy()
	h2.PositiveSpans[1].Offset++
	notEquals(h1, *h2)
	h2 = h1.Copy()
	h2.NegativeSpans[1].Offset++
	notEquals(h1, *h2)

	// Adding an empty bucket.
	h2 = h1.Copy()
	h2.PositiveSpans[0].Offset--
	h2.PositiveSpans[0].Length++
	h2.PositiveBuckets = append([]int64{0}, h2.PositiveBuckets...)
	notEquals(h1, *h2)
	h2 = h1.Copy()
	h2.NegativeSpans[0].Offset--
	h2.NegativeSpans[0].Length++
	h2.NegativeBuckets = append([]int64{0}, h2.NegativeBuckets...)
	notEquals(h1, *h2)

	// Adding new bucket.
	h2 = h1.Copy()
	h2.PositiveSpans = append(h2.PositiveSpans, Span{
		Offset: 1,
		Length: 1,
	})
	h2.PositiveBuckets = append(h2.PositiveBuckets, 1)
	notEquals(h1, *h2)
	h2 = h1.Copy()
	h2.NegativeSpans = append(h2.NegativeSpans, Span{
		Offset: 1,
		Length: 1,
	})
	h2.NegativeBuckets = append(h2.NegativeBuckets, 1)
	notEquals(h1, *h2)
}

func TestHistogramCompact(t *testing.T) {
	cases := []struct {
		name            string
		in              *Histogram
		maxEmptyBuckets int
		expected        *Histogram
	}{
		{
			"empty histogram",
			&Histogram{},
			0,
			&Histogram{},
		},
		{
			"nothing should happen",
			&Histogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000},
			},
		},
		{
			"eliminate zero offsets",
			&Histogram{
				PositiveSpans:   []Span{{-2, 1}, {0, 3}, {0, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {0, 2}, {2, 1}, {0, 1}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000, 3, 4},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 5}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 4}, {2, 2}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"eliminate zero length",
			&Histogram{
				PositiveSpans:   []Span{{-2, 2}, {2, 0}, {3, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {0, 0}, {2, 0}, {1, 4}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000, 3, 4},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"eliminate multiple zero length spans",
			&Histogram{
				PositiveSpans:   []Span{{-2, 2}, {2, 0}, {2, 0}, {2, 0}, {3, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 2}, {9, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
			},
		},
		{
			"cut empty buckets at start or end",
			&Histogram{
				PositiveSpans:   []Span{{-4, 4}, {5, 3}},
				PositiveBuckets: []int64{0, 0, 1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, 3, 4, -9},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []int64{5, 3, -4, -2, 3, 4},
			},
		},
		{
			"cut empty buckets at start and end",
			&Histogram{
				PositiveSpans:   []Span{{-4, 4}, {5, 6}},
				PositiveBuckets: []int64{0, 0, 1, 3, -3, 42, 3, -46, 0, 0},
				NegativeSpans:   []Span{{-2, 4}, {3, 5}},
				NegativeBuckets: []int64{0, 0, 5, 3, -4, -2, 3, 4, -9},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []int64{5, 3, -4, -2, 3, 4},
			},
		},
		{
			"cut empty buckets at start or end of spans, even in the middle",
			&Histogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 6}},
				PositiveBuckets: []int64{0, 0, 1, 3, -4, 0, 1, 42, 3, -46, 0, 0},
				NegativeSpans:   []Span{{0, 2}, {2, 6}},
				NegativeBuckets: []int64{5, 3, -8, 4, -2, 3, 4, -9},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []int64{5, 3, -4, -2, 3, 4},
			},
		},
		{
			"cut empty buckets at start or end but merge spans due to maxEmptyBuckets",
			&Histogram{
				PositiveSpans:   []Span{{-4, 4}, {5, 3}},
				PositiveBuckets: []int64{0, 0, 1, 3, -3, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, 3, 4, -9},
			},
			10,
			&Histogram{
				PositiveSpans:   []Span{{-2, 10}},
				PositiveBuckets: []int64{1, 3, -4, 0, 0, 0, 0, 1, 42, 3},
				NegativeSpans:   []Span{{0, 9}},
				NegativeBuckets: []int64{5, 3, -8, 0, 0, 4, -2, 3, 4},
			},
		},
		{
			"cut empty buckets from the middle of a span",
			&Histogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []int64{0, 0, 1, -1, 0, 3, -2, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, -2, 3, 4},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 1}, {3, 3}},
				PositiveBuckets: []int64{1, 2, -2, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 2}, {1, 2}},
				NegativeBuckets: []int64{5, 3, -4, -2, 1, 4},
			},
		},
		{
			"cut out a span containing only empty buckets",
			&Histogram{
				PositiveSpans:   []Span{{-4, 3}, {2, 2}, {3, 4}},
				PositiveBuckets: []int64{0, 0, 1, -1, 0, 3, -2, 42, 3},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-2, 1}, {7, 4}},
				PositiveBuckets: []int64{1, 2, -2, 42, 3},
			},
		},
		{
			"cut empty buckets from the middle of a span, avoiding some due to maxEmptyBuckets",
			&Histogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []int64{0, 0, 1, -1, 0, 3, -2, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, -2, 3, 4},
			},
			1,
			&Histogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 1}, {3, 3}},
				PositiveBuckets: []int64{1, 2, -2, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, -2, 3, 4},
			},
		},
		{
			"avoiding all cutting of empty buckets from the middle of a chunk due to maxEmptyBuckets",
			&Histogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []int64{0, 0, 1, -1, 0, 3, -2, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, -2, 3, 4},
			},
			2,
			&Histogram{
				PositiveSpans:   []Span{{-2, 4}, {3, 3}},
				PositiveBuckets: []int64{1, -1, 0, 3, -2, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, -2, 3, 4},
			},
		},
		{
			"everything merged into one span due to maxEmptyBuckets",
			&Histogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []int64{0, 0, 1, -1, 0, 3, -2, 42, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []int64{5, 3, -4, -2, -2, 3, 4},
			},
			3,
			&Histogram{
				PositiveSpans:   []Span{{-2, 10}},
				PositiveBuckets: []int64{1, -1, 0, 3, -3, 0, 0, 1, 42, 3},
				NegativeSpans:   []Span{{0, 10}},
				NegativeBuckets: []int64{5, 3, -8, 0, 0, 4, -2, -2, 3, 4},
			},
		},
		{
			"only empty buckets and maxEmptyBuckets greater zero",
			&Histogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []int64{0, 0, 0, 0, 0, 0, 0, 0, 0},
				NegativeSpans:   []Span{{0, 7}},
				NegativeBuckets: []int64{0, 0, 0, 0, 0, 0, 0},
			},
			3,
			&Histogram{
				PositiveSpans:   []Span{},
				PositiveBuckets: []int64{},
				NegativeSpans:   []Span{},
				NegativeBuckets: []int64{},
			},
		},
		{
			"multiple spans of only empty buckets",
			&Histogram{
				PositiveSpans:   []Span{{-10, 2}, {2, 1}, {3, 3}},
				PositiveBuckets: []int64{0, 0, 0, 0, 2, 3},
				NegativeSpans:   []Span{{-10, 2}, {2, 1}, {3, 3}},
				NegativeBuckets: []int64{2, 3, -5, 0, 0, 0},
			},
			0,
			&Histogram{
				PositiveSpans:   []Span{{-1, 2}},
				PositiveBuckets: []int64{2, 3},
				NegativeSpans:   []Span{{-10, 2}},
				NegativeBuckets: []int64{2, 3},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.in.Compact(c.maxEmptyBuckets))
			// Compact has happened in-place, too.
			require.Equal(t, c.expected, c.in)
		})
	}
}
