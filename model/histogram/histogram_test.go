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
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/value"
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
		{
			histogram: Histogram{
				Schema: CustomBucketsSchema,
				Count:  19,
				Sum:    2.7,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
				CustomValues:    []float64{1, 2, 5, 10, 15, 20, 25, 50},
			},
			expectedString: "{count:19, sum:2.7, [-Inf,1]:1, (1,2]:3, (2,5]:1, (5,10]:2, (10,15]:1, (15,20]:1, (20,25]:1}",
		},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
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
		{
			histogram: Histogram{
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{5, 10, 20, 50},
			},
			expectedBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 5, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: math.Inf(-1), Upper: 10, Count: 3, LowerInclusive: true, UpperInclusive: true, Index: 1},

				{Lower: math.Inf(-1), Upper: 20, Count: 3, LowerInclusive: true, UpperInclusive: true, Index: 2},

				{Lower: math.Inf(-1), Upper: 50, Count: 4, LowerInclusive: true, UpperInclusive: true, Index: 3},
				{Lower: math.Inf(-1), Upper: math.Inf(1), Count: 5, LowerInclusive: true, UpperInclusive: true, Index: 4},
			},
		},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
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
		{
			histogram: Histogram{
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{5, 10, 20, 50},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 5, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: 5, Upper: 10, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 20, Upper: 50, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 50, Upper: math.Inf(1), Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
			},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
		{
			histogram: Histogram{
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{0, 10, 20, 50},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: 0, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: 0, Upper: 10, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},

				{Lower: 20, Upper: 50, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 50, Upper: math.Inf(1), Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
			},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
		{
			histogram: Histogram{
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 5},
				},
				PositiveBuckets: []int64{1, 1, 0, -1, 0},
				CustomValues:    []float64{-5, 0, 20, 50},
			},
			expectedPositiveBuckets: []Bucket[uint64]{
				{Lower: math.Inf(-1), Upper: -5, Count: 1, LowerInclusive: true, UpperInclusive: true, Index: 0},
				{Lower: -5, Upper: 0, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 1},
				{Lower: 0, Upper: 20, Count: 2, LowerInclusive: false, UpperInclusive: true, Index: 2},
				{Lower: 20, Upper: 50, Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 3},
				{Lower: 50, Upper: math.Inf(1), Count: 1, LowerInclusive: false, UpperInclusive: true, Index: 4},
			},
			expectedNegativeBuckets: []Bucket[uint64]{},
		},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
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
	cases := []struct {
		name string
		fh   *FloatHistogram
	}{
		{name: "without prior float histogram"},
		{name: "prior float histogram with more buckets", fh: &FloatHistogram{
			Schema:        2,
			Count:         3,
			Sum:           5,
			ZeroThreshold: 4,
			ZeroCount:     1,
			PositiveSpans: []Span{
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
			},
			PositiveBuckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			NegativeSpans: []Span{
				{Offset: 20, Length: 6},
				{Offset: 12, Length: 7},
				{Offset: 33, Length: 10},
			},
			NegativeBuckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
		}},
		{name: "prior float histogram with fewer buckets", fh: &FloatHistogram{
			Schema:        2,
			Count:         3,
			Sum:           5,
			ZeroThreshold: 4,
			ZeroCount:     1,
			PositiveSpans: []Span{
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
			},
			PositiveBuckets: []float64{1, 2},
			NegativeSpans: []Span{
				{Offset: 20, Length: 6},
				{Offset: 12, Length: 7},
				{Offset: 33, Length: 10},
			},
			NegativeBuckets: []float64{1, 2},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fh := h.ToFloat(c.fh)
			require.Equal(t, h.String(), fh.String())
		})
	}
}

func TestCustomBucketsHistogramToFloat(t *testing.T) {
	h := Histogram{
		Schema: CustomBucketsSchema,
		Count:  10,
		Sum:    2.7,
		PositiveSpans: []Span{
			{Offset: 0, Length: 4},
			{Offset: 0, Length: 0},
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
		CustomValues:    []float64{5, 10, 20, 50, 100, 500},
	}
	cases := []struct {
		name string
		fh   *FloatHistogram
	}{
		{name: "without prior float histogram"},
		{name: "prior float histogram with more buckets", fh: &FloatHistogram{
			Schema:        2,
			Count:         3,
			Sum:           5,
			ZeroThreshold: 4,
			ZeroCount:     1,
			PositiveSpans: []Span{
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
			},
			PositiveBuckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			NegativeSpans: []Span{
				{Offset: 20, Length: 6},
				{Offset: 12, Length: 7},
				{Offset: 33, Length: 10},
			},
			NegativeBuckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
		}},
		{name: "prior float histogram with fewer buckets", fh: &FloatHistogram{
			Schema:        2,
			Count:         3,
			Sum:           5,
			ZeroThreshold: 4,
			ZeroCount:     1,
			PositiveSpans: []Span{
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 2},
			},
			PositiveBuckets: []float64{1, 2},
			NegativeSpans: []Span{
				{Offset: 20, Length: 6},
				{Offset: 12, Length: 7},
				{Offset: 33, Length: 10},
			},
			NegativeBuckets: []float64{1, 2},
		}},
	}

	require.NoError(t, h.Validate())
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hStr := h.String()
			fh := h.ToFloat(c.fh)
			require.NoError(t, fh.Validate())
			require.Equal(t, hStr, h.String())
			require.Equal(t, hStr, fh.String())
		})
	}
}

// TestHistogramEquals tests both Histogram and FloatHistogram.
func TestHistogramEquals(t *testing.T) {
	h1 := Histogram{
		Schema:        3,
		Count:         62,
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
		h1f, h2f := h1.ToFloat(nil), h2.ToFloat(nil)
		require.True(t, h1f.Equals(h2f))
		require.True(t, h2f.Equals(h1f))
	}
	notEquals := func(h1, h2 Histogram) {
		require.False(t, h1.Equals(&h2))
		require.False(t, h2.Equals(&h1))
		h1f, h2f := h1.ToFloat(nil), h2.ToFloat(nil)
		require.False(t, h1f.Equals(h2f))
		require.False(t, h2f.Equals(h1f))
	}
	notEqualsUntilFloatConv := func(h1, h2 Histogram) {
		require.False(t, h1.Equals(&h2))
		require.False(t, h2.Equals(&h1))
		h1f, h2f := h1.ToFloat(nil), h2.ToFloat(nil)
		require.True(t, h1f.Equals(h2f))
		require.True(t, h2f.Equals(h1f))
	}

	require.NoError(t, h1.Validate())

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

	// Sum is StaleNaN.
	hStale := h1.Copy()
	hStale.Sum = math.Float64frombits(value.StaleNaN)
	notEquals(h1, *hStale)
	equals(*hStale, *hStale)

	// Sum is NaN (but not a StaleNaN).
	hNaN := h1.Copy()
	hNaN.Sum = math.NaN()
	notEquals(h1, *hNaN)
	equals(*hNaN, *hNaN)

	// Sum StaleNaN vs regular NaN.
	notEquals(*hStale, *hNaN)

	// Has non-empty custom bounds for exponential schema.
	hCustom := h1.Copy()
	hCustom.CustomValues = []float64{1, 2, 3}
	equals(h1, *hCustom)

	cbh1 := Histogram{
		Schema: CustomBucketsSchema,
		Count:  10,
		Sum:    2.7,
		PositiveSpans: []Span{
			{Offset: 0, Length: 4},
			{Offset: 10, Length: 3},
		},
		PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
		CustomValues:    []float64{0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 20, 25, 50, 75, 100, 200, 250, 500, 1000},
	}

	require.NoError(t, cbh1.Validate())

	cbh2 := cbh1.Copy()
	equals(cbh1, *cbh2)

	// Has different custom bounds for custom buckets schema.
	cbh2 = cbh1.Copy()
	cbh2.CustomValues = []float64{0.1, 0.2, 0.5}
	notEquals(cbh1, *cbh2)

	// Has non-empty negative spans and buckets for custom buckets schema.
	cbh2 = cbh1.Copy()
	cbh2.NegativeSpans = []Span{{Offset: 0, Length: 1}}
	cbh2.NegativeBuckets = []int64{1}
	notEqualsUntilFloatConv(cbh1, *cbh2)

	// Has non-zero zero count and threshold for custom buckets schema.
	cbh2 = cbh1.Copy()
	cbh2.ZeroThreshold = 0.1
	cbh2.ZeroCount = 10
	notEqualsUntilFloatConv(cbh1, *cbh2)
}

func TestHistogramCopy(t *testing.T) {
	cases := []struct {
		name     string
		orig     *Histogram
		expected *Histogram
	}{
		{
			name:     "without buckets",
			orig:     &Histogram{},
			expected: &Histogram{},
		},
		{
			name: "with buckets",
			orig: &Histogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000},
			},
			expected: &Histogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000},
			},
		},
		{
			name: "with empty buckets and non empty capacity",
			orig: &Histogram{
				PositiveSpans:   make([]Span, 0, 1),
				PositiveBuckets: make([]int64, 0, 1),
				NegativeSpans:   make([]Span, 0, 1),
				NegativeBuckets: make([]int64, 0, 1),
			},
			expected: &Histogram{},
		},
		{
			name: "with custom buckets",
			orig: &Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				CustomValues:    []float64{5, 10, 15},
			},
			expected: &Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				CustomValues:    []float64{5, 10, 15},
			},
		},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			hCopy := tcase.orig.Copy()

			// Modify a primitive value in the original histogram.
			tcase.orig.Sum++
			require.Equal(t, tcase.expected, hCopy)
			assertDeepCopyHSpans(t, tcase.orig, hCopy, tcase.expected)
		})
	}
}

func TestHistogramCopyTo(t *testing.T) {
	cases := []struct {
		name     string
		orig     *Histogram
		expected *Histogram
	}{
		{
			name:     "without buckets",
			orig:     &Histogram{},
			expected: &Histogram{},
		},
		{
			name: "with buckets",
			orig: &Histogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000},
			},
			expected: &Histogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []int64{5, 3, 1.234e5, 1000},
			},
		},
		{
			name: "with empty buckets and non empty capacity",
			orig: &Histogram{
				PositiveSpans:   make([]Span, 0, 1),
				PositiveBuckets: make([]int64, 0, 1),
				NegativeSpans:   make([]Span, 0, 1),
				NegativeBuckets: make([]int64, 0, 1),
			},
			expected: &Histogram{},
		},
		{
			name: "with custom buckets",
			orig: &Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				CustomValues:    []float64{5, 10, 15},
			},
			expected: &Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				CustomValues:    []float64{5, 10, 15},
			},
		},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			hCopy := &Histogram{}
			tcase.orig.CopyTo(hCopy)

			// Modify a primitive value in the original histogram.
			tcase.orig.Sum++
			require.Equal(t, tcase.expected, hCopy)
			assertDeepCopyHSpans(t, tcase.orig, hCopy, tcase.expected)
		})
	}
}

func assertDeepCopyHSpans(t *testing.T, orig, hCopy, expected *Histogram) {
	// Do an in-place expansion of an original spans slice.
	orig.PositiveSpans = expandSpans(orig.PositiveSpans)
	orig.PositiveSpans[len(orig.PositiveSpans)-1] = Span{1, 2}

	hCopy.PositiveSpans = expandSpans(hCopy.PositiveSpans)
	expected.PositiveSpans = expandSpans(expected.PositiveSpans)
	// Expand the copy spans and assert that modifying the original has not affected the copy.
	require.Equal(t, expected, hCopy)
}

func expandSpans(spans []Span) []Span {
	n := len(spans)
	if cap(spans) > n {
		spans = spans[:n+1]
	} else {
		spans = append(spans, Span{})
	}
	return spans
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
		{
			"nothing should happen with custom buckets",
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				CustomValues:    []float64{5, 10, 15},
			},
			0,
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42},
				CustomValues:    []float64{5, 10, 15},
			},
		},
		{
			"eliminate zero offsets with custom buckets",
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 1}, {0, 3}, {0, 1}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				CustomValues:    []float64{5, 10, 15, 20},
			},
			0,
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 5}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				CustomValues:    []float64{5, 10, 15, 20},
			},
		},
		{
			"eliminate zero length with custom buckets",
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 2}, {2, 0}, {3, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				CustomValues:    []float64{5, 10, 15, 20},
			},
			0,
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				CustomValues:    []float64{5, 10, 15, 20},
			},
		},
		{
			"eliminate multiple zero length spans with custom buckets",
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 2}, {2, 0}, {2, 0}, {2, 0}, {3, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				CustomValues:    []float64{5, 10, 15, 20},
			},
			0,
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 2}, {9, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				CustomValues:    []float64{5, 10, 15, 20},
			},
		},
		{
			"cut empty buckets at start or end of spans, even in the middle, with custom buckets",
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-4, 6}, {3, 6}},
				PositiveBuckets: []int64{0, 0, 1, 3, -4, 0, 1, 42, 3, -46, 0, 0},
				CustomValues:    []float64{5, 10, 15, 20},
			},
			0,
			&Histogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []int64{1, 3, -3, 42, 3},
				CustomValues:    []float64{5, 10, 15, 20},
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

func TestHistogramValidation(t *testing.T) {
	tests := map[string]struct {
		h         *Histogram
		errMsg    string
		skipFloat bool
	}{
		"valid histogram": {
			h: &Histogram{
				Count:         12,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           19.4,
				Schema:        1,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				NegativeSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{1, 1, -1, 0},
			},
		},
		"valid histogram with NaN observations that has its Count (4) higher than the actual total of buckets (2 + 1)": {
			// This case is possible if NaN values (which do not fall into any bucket) are observed.
			h: &Histogram{
				ZeroCount:       2,
				Count:           4,
				Sum:             math.NaN(),
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{1},
			},
		},
		"rejects histogram without NaN observations that has its Count (4) higher than the actual total of buckets (2 + 1)": {
			h: &Histogram{
				ZeroCount:       2,
				Count:           4,
				Sum:             333,
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{1},
			},
			errMsg:    `3 observations found in buckets, but the Count field is 4: histogram's observation count should equal the number of observations found in the buckets (in absence of NaN)`,
			skipFloat: true,
		},
		"rejects histogram that has too few negative buckets": {
			h: &Histogram{
				NegativeSpans:   []Span{{Offset: 0, Length: 1}},
				NegativeBuckets: []int64{},
			},
			errMsg: `negative side: spans need 1 buckets, have 0 buckets: histogram spans specify different number of buckets than provided`,
		},
		"rejects histogram that has too few positive buckets": {
			h: &Histogram{
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{},
			},
			errMsg: `positive side: spans need 1 buckets, have 0 buckets: histogram spans specify different number of buckets than provided`,
		},
		"rejects histogram that has too many negative buckets": {
			h: &Histogram{
				NegativeSpans:   []Span{{Offset: 0, Length: 1}},
				NegativeBuckets: []int64{1, 2},
			},
			errMsg: `negative side: spans need 1 buckets, have 2 buckets: histogram spans specify different number of buckets than provided`,
		},
		"rejects histogram that has too many positive buckets": {
			h: &Histogram{
				PositiveSpans:   []Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{1, 2},
			},
			errMsg: `positive side: spans need 1 buckets, have 2 buckets: histogram spans specify different number of buckets than provided`,
		},
		"rejects a histogram that has a negative span with a negative offset": {
			h: &Histogram{
				NegativeSpans:   []Span{{Offset: -1, Length: 1}, {Offset: -1, Length: 1}},
				NegativeBuckets: []int64{1, 2},
			},
			errMsg: `negative side: span number 2 with offset -1: histogram has a span whose offset is negative`,
		},
		"rejects a histogram which has a positive span with a negative offset": {
			h: &Histogram{
				PositiveSpans:   []Span{{Offset: -1, Length: 1}, {Offset: -1, Length: 1}},
				PositiveBuckets: []int64{1, 2},
			},
			errMsg: `positive side: span number 2 with offset -1: histogram has a span whose offset is negative`,
		},
		"rejects a histogram that has a negative bucket with a negative count": {
			h: &Histogram{
				NegativeSpans:   []Span{{Offset: -1, Length: 1}},
				NegativeBuckets: []int64{-1},
			},
			errMsg: `negative side: bucket number 1 has observation count of -1: histogram has a bucket whose observation count is negative`,
		},
		"rejects a histogram that has a positive bucket with a negative count": {
			h: &Histogram{
				PositiveSpans:   []Span{{Offset: -1, Length: 1}},
				PositiveBuckets: []int64{-1},
			},
			errMsg: `positive side: bucket number 1 has observation count of -1: histogram has a bucket whose observation count is negative`,
		},
		"rejects a histogram that has a lower count than count in buckets": {
			h: &Histogram{
				Count:           0,
				NegativeSpans:   []Span{{Offset: -1, Length: 1}},
				PositiveSpans:   []Span{{Offset: -1, Length: 1}},
				NegativeBuckets: []int64{1},
				PositiveBuckets: []int64{1},
			},
			errMsg:    `2 observations found in buckets, but the Count field is 0: histogram's observation count should equal the number of observations found in the buckets (in absence of NaN)`,
			skipFloat: true,
		},
		"rejects a histogram that doesn't count the zero bucket in its count": {
			h: &Histogram{
				Count:           2,
				ZeroCount:       1,
				NegativeSpans:   []Span{{Offset: -1, Length: 1}},
				PositiveSpans:   []Span{{Offset: -1, Length: 1}},
				NegativeBuckets: []int64{1},
				PositiveBuckets: []int64{1},
			},
			errMsg:    `3 observations found in buckets, but the Count field is 2: histogram's observation count should equal the number of observations found in the buckets (in absence of NaN)`,
			skipFloat: true,
		},
		"rejects an exponential histogram with custom buckets schema": {
			h: &Histogram{
				Count:         12,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           19.4,
				Schema:        CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				NegativeSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{1, 1, -1, 0},
			},
			errMsg: `custom buckets: only 0 custom bounds defined which is insufficient to cover total span length of 5: histogram custom bounds are too few`,
		},
		"rejects a custom buckets histogram with exponential schema": {
			h: &Histogram{
				Count:  5,
				Sum:    19.4,
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			errMsg:    `histogram with exponential schema must not have custom bounds`,
			skipFloat: true, // Converting to float will remove the wrong fields so only the float version will pass validation
		},
		"rejects a custom buckets histogram with zero/negative buckets": {
			h: &Histogram{
				Count:         12,
				ZeroCount:     2,
				ZeroThreshold: 0.001,
				Sum:           19.4,
				Schema:        CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				NegativeSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				NegativeBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			errMsg:    `custom buckets: must have zero count of 0`,
			skipFloat: true, // Converting to float will remove the wrong fields so only the float version will pass validation
		},
		"rejects a custom buckets histogram with negative offset in first span": {
			h: &Histogram{
				Count:  5,
				Sum:    19.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: -1, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			errMsg: `custom buckets: span number 1 with offset -1: histogram has a span whose offset is negative`,
		},
		"rejects a custom buckets histogram with negative offset in subsequent spans": {
			h: &Histogram{
				Count:  5,
				Sum:    19.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: -1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			errMsg: `custom buckets: span number 2 with offset -1: histogram has a span whose offset is negative`,
		},
		"rejects a custom buckets histogram with non-matching bucket counts": {
			h: &Histogram{
				Count:  5,
				Sum:    19.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			errMsg: `custom buckets: spans need 4 buckets, have 3 buckets: histogram spans specify different number of buckets than provided`,
		},
		"rejects a custom buckets histogram with too few bounds": {
			h: &Histogram{
				Count:  5,
				Sum:    19.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{1, 2, 3},
			},
			errMsg: `custom buckets: only 3 custom bounds defined which is insufficient to cover total span length of 5: histogram custom bounds are too few`,
		},
		"valid custom buckets histogram": {
			h: &Histogram{
				Count:  5,
				Sum:    19.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{1, 2, 3, 4},
			},
		},
		"valid custom buckets histogram with extra bounds": {
			h: &Histogram{
				Count:  5,
				Sum:    19.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8},
			},
		},
		"reject custom buckets histogram with non-increasing bound": {
			h: &Histogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{0, 0},
			},
			errMsg: "custom buckets: previous bound is 0.000000 and current is 0.000000: histogram custom bounds must be in strictly increasing order",
		},
		"reject custom buckets histogram with explicit +Inf bound": {
			h: &Histogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, math.Inf(1)},
			},
			errMsg: "custom buckets: last +Inf bound must not be explicitly defined: histogram custom bounds must be finite",
		},
		"valid custom buckets histogram with explicit -Inf bound": {
			h: &Histogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{math.Inf(-1), 1},
			},
		},
		"reject custom buckets histogram with NaN bound": {
			h: &Histogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, math.NaN(), 3},
			},
			errMsg: "custom buckets: histogram custom bounds must not be NaN",
		},
		"schema too high": {
			h: &Histogram{
				Schema: 10,
			},
			errMsg: `histogram has an invalid schema, which must be between -4 and 8 for exponential buckets, or -53 for custom buckets, got schema 10`,
		},
		"schema too low": {
			h: &Histogram{
				Schema: -10,
			},
			errMsg: `histogram has an invalid schema, which must be between -4 and 8 for exponential buckets, or -53 for custom buckets, got schema -10`,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			if err := tc.h.Validate(); tc.errMsg != "" {
				require.ErrorAs(t, err, new(Error))
				require.EqualError(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
			if tc.skipFloat {
				return
			}

			fh := tc.h.ToFloat(nil)
			if err := fh.Validate(); tc.errMsg != "" {
				require.ErrorAs(t, err, new(Error))
				require.EqualError(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func BenchmarkHistogramValidation(b *testing.B) {
	histograms := GenerateBigTestHistograms(b.N, 500)
	b.ResetTimer()
	for _, h := range histograms {
		require.NoError(b, h.Validate())
	}
}

func TestHistogramReduceResolution(t *testing.T) {
	tcs := map[string]struct {
		origin *Histogram
		target *Histogram
	}{
		"valid histogram": {
			origin: &Histogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 3, Length: 2},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0},
				NegativeSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 3, Length: 2},
				},
				NegativeBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			target: &Histogram{
				Schema: -1,
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
					{Offset: 1, Length: 1},
				},
				PositiveBuckets: []int64{1, 3, -2, 0},
				NegativeSpans: []Span{
					{Offset: 0, Length: 3},
					{Offset: 1, Length: 1},
				},
				NegativeBuckets: []int64{1, 3, -2, 0},
			},
		},
	}

	for _, tc := range tcs {
		target := tc.origin.ReduceResolution(tc.target.Schema)
		require.Equal(t, tc.target, target)
		// Check that receiver histogram was mutated:
		require.Equal(t, tc.target, tc.origin)
	}
}
