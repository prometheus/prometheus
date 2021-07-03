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

func TestCumulativeExpandSparseHistogram(t *testing.T) {
	cases := []struct {
		hist       SparseHistogram
		expBuckets []Bucket
	}{
		{
			hist: SparseHistogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{1, 1, -1, 0},
			},
			expBuckets: []Bucket{
				{Le: 1, Count: 1},
				{Le: 2, Count: 3},

				{Le: 4, Count: 3},

				{Le: 8, Count: 4},
				{Le: 16, Count: 5},
			},
		},
		{
			hist: SparseHistogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 5},
					{Offset: 1, Length: 1},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expBuckets: []Bucket{
				{Le: 1, Count: 1},
				{Le: 2, Count: 4},
				{Le: 4, Count: 5},
				{Le: 8, Count: 7},

				{Le: 16, Count: 8},

				{Le: 32, Count: 8},
				{Le: 64, Count: 9},
			},
		},
		{
			hist: SparseHistogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 7},
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 0},
			},
			expBuckets: []Bucket{
				{Le: 1, Count: 1},
				{Le: 2, Count: 4},
				{Le: 4, Count: 5},
				{Le: 8, Count: 7},
				{Le: 16, Count: 8},
				{Le: 32, Count: 9},
				{Le: 64, Count: 10},
			},
		},
		{
			hist: SparseHistogram{
				Schema: 3,
				PositiveSpans: []Span{
					{Offset: -5, Length: 2}, // -5 -4
					{Offset: 2, Length: 3},  // -1 0 1
					{Offset: 2, Length: 2},  // 4 5
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0, 3},
			},
			expBuckets: []Bucket{
				{Le: 0.6484197773255048, Count: 1}, // -5
				{Le: 0.7071067811865475, Count: 4}, // -4

				{Le: 0.7711054127039704, Count: 4}, // -3
				{Le: 0.8408964152537144, Count: 4}, // -2

				{Le: 0.9170040432046711, Count: 5}, // -1
				{Le: 1, Count: 7},                  // 1
				{Le: 1.0905077326652577, Count: 8}, // 0

				{Le: 1.189207115002721, Count: 8},  // 1
				{Le: 1.2968395546510096, Count: 8}, // 2

				{Le: 1.414213562373095, Count: 9},   // 3
				{Le: 1.5422108254079407, Count: 13}, // 4
			},
		},
		{
			hist: SparseHistogram{
				Schema: -2,
				PositiveSpans: []Span{
					{Offset: -2, Length: 4}, // -2 -1 0 1
					{Offset: 2, Length: 2},  // 4 5
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1, 0},
			},
			expBuckets: []Bucket{
				{Le: 0.00390625, Count: 1}, // -2
				{Le: 0.0625, Count: 4},     // -1
				{Le: 1, Count: 5},          // 0
				{Le: 16, Count: 7},         // 1

				{Le: 256, Count: 7},  // 2
				{Le: 4096, Count: 7}, // 3

				{Le: 65536, Count: 8},   // 4
				{Le: 1048576, Count: 9}, // 5
			},
		},
		{
			hist: SparseHistogram{
				Schema: -1,
				PositiveSpans: []Span{
					{Offset: -2, Length: 5}, // -2 -1 0 1 2
				},
				PositiveBuckets: []int64{1, 2, -2, 1, -1},
			},
			expBuckets: []Bucket{
				{Le: 0.0625, Count: 1}, // -2
				{Le: 0.25, Count: 4},   // -1
				{Le: 1, Count: 5},      // 0
				{Le: 4, Count: 7},      // 1
				{Le: 16, Count: 8},     // 2
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			it := CumulativeExpandSparseHistogram(c.hist)
			actBuckets := make([]Bucket, 0, len(c.expBuckets))
			for it.Next() {
				actBuckets = append(actBuckets, it.At())
			}
			require.NoError(t, it.Err())
			require.Equal(t, c.expBuckets, actBuckets)
		})
	}
}
