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
		expectedBuckets []Bucket
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
			expectedBuckets: []Bucket{
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
			expectedBuckets: []Bucket{
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
			expectedBuckets: []Bucket{
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
			expectedBuckets: []Bucket{
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
			expectedBuckets: []Bucket{
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
			expectedBuckets: []Bucket{
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
			actualBuckets := make([]Bucket, 0, len(c.expectedBuckets))
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
		expectedPositiveBuckets []Bucket
		expectedNegativeBuckets []Bucket
	}{
		{
			histogram: Histogram{
				Schema: 0,
			},
			expectedPositiveBuckets: []Bucket{},
			expectedNegativeBuckets: []Bucket{},
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
			expectedPositiveBuckets: []Bucket{
				{Lower: 0.5, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 2, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 4, Upper: 8, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 8, Upper: 16, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
			},
			expectedNegativeBuckets: []Bucket{},
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
			expectedPositiveBuckets: []Bucket{},
			expectedNegativeBuckets: []Bucket{
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
			expectedPositiveBuckets: []Bucket{
				{Lower: 0.5, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 2, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: 1},
				{Lower: 2, Upper: 4, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 2},
				{Lower: 4, Upper: 8, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 8, Upper: 16, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 16, Upper: 32, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 5},
				{Lower: 32, Upper: 64, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 6},
			},
			expectedNegativeBuckets: []Bucket{
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
			expectedPositiveBuckets: []Bucket{
				{Lower: 0.5946035575013605, Upper: 0.6484197773255048, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -5},
				{Lower: 0.6484197773255048, Upper: 0.7071067811865475, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: -4},

				{Lower: 0.8408964152537144, Upper: 0.9170040432046711, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -1},
				{Lower: 0.9170040432046711, Upper: 1, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 1.0905077326652577, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 1.2968395546510096, Upper: 1.414213562373095, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 1.414213562373095, Upper: 1.5422108254079407, Count: 4, LowerInclusive: false, UpperInclusive: true, Index: 5},
			},
			expectedNegativeBuckets: []Bucket{},
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
			expectedPositiveBuckets: []Bucket{
				{Lower: 0.000244140625, Upper: 0.00390625, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -2},
				{Lower: 0.00390625, Upper: 0.0625, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: -1},
				{Lower: 0.0625, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 16, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 4096, Upper: 65536, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
				{Lower: 65536, Upper: 1048576, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 5},
			},
			expectedNegativeBuckets: []Bucket{},
		},
		{
			histogram: Histogram{
				Schema: -1,
				PositiveSpans: []Span{
					{Offset: -2, Length: 5}, // -2 -1 0 1 2
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1},
			},
			expectedPositiveBuckets: []Bucket{
				{Lower: 0.015625, Upper: 0.0625, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: -2},
				{Lower: 0.0625, Upper: 0.25, Count: 3, LowerInclusive: false, UpperInclusive: true, Index: -1},
				{Lower: 0.25, Upper: 1, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 0},
				{Lower: 1, Upper: 4, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},
				{Lower: 4, Upper: 16, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 2},
			},
			expectedNegativeBuckets: []Bucket{},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			it := c.histogram.PositiveBucketIterator()
			actualPositiveBuckets := make([]Bucket, 0, len(c.expectedPositiveBuckets))
			for it.Next() {
				actualPositiveBuckets = append(actualPositiveBuckets, it.At())
			}
			require.Equal(t, c.expectedPositiveBuckets, actualPositiveBuckets)
			it = c.histogram.NegativeBucketIterator()
			actualNegativeBuckets := make([]Bucket, 0, len(c.expectedNegativeBuckets))
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
