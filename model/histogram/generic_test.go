// Copyright 2022 The Prometheus Authors
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
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetBoundExponential(t *testing.T) {
	scenarios := []struct {
		idx    int32
		schema int32
		want   float64
	}{
		{
			idx:    -1,
			schema: -1,
			want:   0.25,
		},
		{
			idx:    0,
			schema: -1,
			want:   1,
		},
		{
			idx:    1,
			schema: -1,
			want:   4,
		},
		{
			idx:    512,
			schema: -1,
			want:   math.MaxFloat64,
		},
		{
			idx:    513,
			schema: -1,
			want:   math.Inf(+1),
		},
		{
			idx:    -1,
			schema: 0,
			want:   0.5,
		},
		{
			idx:    0,
			schema: 0,
			want:   1,
		},
		{
			idx:    1,
			schema: 0,
			want:   2,
		},
		{
			idx:    1024,
			schema: 0,
			want:   math.MaxFloat64,
		},
		{
			idx:    1025,
			schema: 0,
			want:   math.Inf(+1),
		},
		{
			idx:    -1,
			schema: 2,
			want:   0.8408964152537144,
		},
		{
			idx:    0,
			schema: 2,
			want:   1,
		},
		{
			idx:    1,
			schema: 2,
			want:   1.189207115002721,
		},
		{
			idx:    4096,
			schema: 2,
			want:   math.MaxFloat64,
		},
		{
			idx:    4097,
			schema: 2,
			want:   math.Inf(+1),
		},
	}

	for _, s := range scenarios {
		got := getBoundExponential(s.idx, s.schema)
		if s.want != got {
			require.Equal(t, s.want, got, "idx %d, schema %d", s.idx, s.schema)
		}
	}
}

func TestReduceResolutionHistogram(t *testing.T) {
	cases := []struct {
		spans           []Span
		buckets         []int64
		schema          int32
		targetSchema    int32
		expectedSpans   []Span
		expectedBuckets []int64
	}{
		{
			spans: []Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 3, Length: 2},
			},
			buckets:      []int64{1, 2, -2, 1, -1, 0},
			schema:       0,
			targetSchema: -1,
			expectedSpans: []Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 1},
			},
			expectedBuckets: []int64{1, 3, -2, 0},
			// schema 0, base 2 { (0.5, 1]:1  (1,2]:3, (2,4]:1, (4,8]:2, (8,16]:0, (16,32]:0, (32,64]:0, (64,128]:1, (128,256]:1}",
			// schema 1, base 4 { (0.25, 1):1 (1,4]:4,          (4,16]:2,          (16,64]:0,            (64,256]:2}
		},
	}

	for _, tc := range cases {
		spansCopy, bucketsCopy := slices.Clone(tc.spans), slices.Clone(tc.buckets)
		spans, buckets := reduceResolution(tc.spans, tc.buckets, tc.schema, tc.targetSchema, true, false)
		require.Equal(t, tc.expectedSpans, spans)
		require.Equal(t, tc.expectedBuckets, buckets)
		// Verify inputs were not mutated:
		require.Equal(t, spansCopy, tc.spans)
		require.Equal(t, bucketsCopy, tc.buckets)

		// Output slices reuse input slices:
		const inplace = true
		spans, buckets = reduceResolution(tc.spans, tc.buckets, tc.schema, tc.targetSchema, true, inplace)
		require.Equal(t, tc.expectedSpans, spans)
		require.Equal(t, tc.expectedBuckets, buckets)
		// Verify inputs were mutated which is now expected:
		require.Equal(t, spans, tc.spans[:len(spans)])
		require.Equal(t, buckets, tc.buckets[:len(buckets)])
	}
}

func TestReduceResolutionFloatHistogram(t *testing.T) {
	cases := []struct {
		spans           []Span
		buckets         []float64
		schema          int32
		targetSchema    int32
		expectedSpans   []Span
		expectedBuckets []float64
	}{
		{
			spans: []Span{
				{Offset: 0, Length: 4},
				{Offset: 0, Length: 0},
				{Offset: 3, Length: 2},
			},
			buckets:      []float64{1, 3, 1, 2, 1, 1},
			schema:       0,
			targetSchema: -1,
			expectedSpans: []Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 1},
			},
			expectedBuckets: []float64{1, 4, 2, 2},
			// schema 0, base 2 { (0.5, 1]:1  (1,2]:3, (2,4]:1, (4,8]:2, (8,16]:0, (16,32]:0, (32,64]:0, (64,128]:1, (128,256]:1}",
			// schema 1, base 4 { (0.25, 1):1 (1,4]:4,          (4,16]:2,          (16,64]:0,            (64,256]:2}
		},
	}

	for _, tc := range cases {
		spansCopy, bucketsCopy := slices.Clone(tc.spans), slices.Clone(tc.buckets)
		spans, buckets := reduceResolution(tc.spans, tc.buckets, tc.schema, tc.targetSchema, false, false)
		require.Equal(t, tc.expectedSpans, spans)
		require.Equal(t, tc.expectedBuckets, buckets)
		// Verify inputs were not mutated:
		require.Equal(t, spansCopy, tc.spans)
		require.Equal(t, bucketsCopy, tc.buckets)

		// Output slices reuse input slices:
		const inplace = true
		spans, buckets = reduceResolution(tc.spans, tc.buckets, tc.schema, tc.targetSchema, false, inplace)
		require.Equal(t, tc.expectedSpans, spans)
		require.Equal(t, tc.expectedBuckets, buckets)
		// Verify inputs were mutated which is now expected:
		require.Equal(t, spans, tc.spans[:len(spans)])
		require.Equal(t, buckets, tc.buckets[:len(buckets)])
	}
}

func TestCustomBucketBoundsMatch(t *testing.T) {
	tests := []struct {
		name   string
		c1, c2 []float64
		want   bool
	}{
		{
			name: "both nil",
			c1:   nil,
			c2:   nil,
			want: true,
		},
		{
			name: "both empty",
			c1:   []float64{},
			c2:   []float64{},
			want: true,
		},
		{
			name: "one empty one non-empty",
			c1:   []float64{},
			c2:   []float64{1.0},
			want: false,
		},
		{
			name: "different lengths",
			c1:   []float64{1.0, 2.0},
			c2:   []float64{1.0, 2.0, 3.0},
			want: false,
		},
		{
			name: "same single value",
			c1:   []float64{1.5},
			c2:   []float64{1.5},
			want: true,
		},
		{
			name: "different single value",
			c1:   []float64{1.5},
			c2:   []float64{2.5},
			want: false,
		},
		{
			name: "same multiple values",
			c1:   []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			c2:   []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			want: true,
		},
		{
			name: "different values",
			c1:   []float64{1.0, 2.1, 3.0},
			c2:   []float64{1.0, 2.0, 3.0},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CustomBucketBoundsMatch(tt.c1, tt.c2)
			require.Equal(t, tt.want, got)

			// Test commutativity (should be symmetric)
			gotReverse := CustomBucketBoundsMatch(tt.c2, tt.c1)
			require.Equal(t, got, gotReverse)
		})
	}
}
