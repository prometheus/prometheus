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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCumulativeBucketIterator(t *testing.T) {
	cases := []struct {
		histogram                 Histogram
		expectedCumulativeBuckets []Bucket
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
			expectedCumulativeBuckets: []Bucket{
				{Upper: 1, Count: 1},
				{Upper: 2, Count: 3},

				{Upper: 4, Count: 3},

				{Upper: 8, Count: 4},
				{Upper: 16, Count: 5},
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
			expectedCumulativeBuckets: []Bucket{
				{Upper: 1, Count: 1},
				{Upper: 2, Count: 4},
				{Upper: 4, Count: 5},
				{Upper: 8, Count: 7},

				{Upper: 16, Count: 8},

				{Upper: 32, Count: 8},
				{Upper: 64, Count: 9},
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
			expectedCumulativeBuckets: []Bucket{
				{Upper: 1, Count: 1},
				{Upper: 2, Count: 4},
				{Upper: 4, Count: 5},
				{Upper: 8, Count: 7},
				{Upper: 16, Count: 8},
				{Upper: 32, Count: 9},
				{Upper: 64, Count: 10},
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
			expectedCumulativeBuckets: []Bucket{
				{Upper: 0.6484197773255048, Count: 1}, // -5
				{Upper: 0.7071067811865475, Count: 4}, // -4

				{Upper: 0.7711054127039704, Count: 4}, // -3
				{Upper: 0.8408964152537144, Count: 4}, // -2

				{Upper: 0.9170040432046711, Count: 5}, // -1
				{Upper: 1, Count: 7},                  // 1
				{Upper: 1.0905077326652577, Count: 8}, // 0

				{Upper: 1.189207115002721, Count: 8},  // 1
				{Upper: 1.2968395546510096, Count: 8}, // 2

				{Upper: 1.414213562373095, Count: 9},   // 3
				{Upper: 1.5422108254079407, Count: 13}, // 4
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
			expectedCumulativeBuckets: []Bucket{
				{Upper: 0.00390625, Count: 1}, // -2
				{Upper: 0.0625, Count: 4},     // -1
				{Upper: 1, Count: 5},          // 0
				{Upper: 16, Count: 7},         // 1

				{Upper: 256, Count: 7},  // 2
				{Upper: 4096, Count: 7}, // 3

				{Upper: 65536, Count: 8},   // 4
				{Upper: 1048576, Count: 9}, // 5
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
			expectedCumulativeBuckets: []Bucket{
				{Upper: 0.0625, Count: 1}, // -2
				{Upper: 0.25, Count: 4},   // -1
				{Upper: 1, Count: 5},      // 0
				{Upper: 4, Count: 7},      // 1
				{Upper: 16, Count: 8},     // 2
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			it := c.histogram.CumulativeBucketIterator()
			actualBuckets := make([]Bucket, 0, len(c.expectedCumulativeBuckets))
			for it.Next() {
				actualBuckets = append(actualBuckets, it.At())
			}
			require.NoError(t, it.Err())
			require.Equal(t, c.expectedCumulativeBuckets, actualBuckets)
		})
	}
}
