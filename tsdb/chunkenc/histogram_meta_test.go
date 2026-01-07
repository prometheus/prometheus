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

// The code in this file was largely written by Damian Gryski as part of
// https://github.com/dgryski/go-tsz and published under the license below.
// It was modified to accommodate reading from byte slices without modifying
// the underlying bytes, which would panic when reading from mmapped
// read-only byte slices.
package chunkenc

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

// Example of a span layout and resulting bucket indices (_idx_ is used in this
// histogram, others are shown just for context):
//
//   spans      : [offset: 0, length: 2]                [offset 1, length 1]
//   bucket idx : _0_                _1_      2         [3]                  4 ...

func TestBucketIterator(t *testing.T) {
	type test struct {
		spans []histogram.Span
		idxs  []int
	}
	tests := []test{
		{
			spans: []histogram.Span{
				{
					Offset: 0,
					Length: 1,
				},
			},
			idxs: []int{0},
		},
		{
			spans: []histogram.Span{
				{
					Offset: 0,
					Length: 2,
				},
				{
					Offset: 1,
					Length: 1,
				},
			},
			idxs: []int{0, 1, 3},
		},
		{
			spans: []histogram.Span{
				{
					Offset: 100,
					Length: 4,
				},
				{
					Offset: 8,
					Length: 7,
				},
				{
					Offset: 0,
					Length: 1,
				},
			},
			idxs: []int{100, 101, 102, 103, 112, 113, 114, 115, 116, 117, 118, 119},
		},
		// The below 2 sets ore the ones described in expandFloatSpansAndBuckets's comments.
		{
			spans: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 2, Length: 1},
				{Offset: 3, Length: 2},
				{Offset: 3, Length: 1},
				{Offset: 1, Length: 1},
			},
			idxs: []int{0, 1, 4, 8, 9, 13, 15},
		},
		{
			spans: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 1},
				{Offset: 1, Length: 4},
				{Offset: 3, Length: 3},
			},
			idxs: []int{0, 1, 2, 4, 6, 7, 8, 9, 13, 14, 15},
		},
	}
	for _, test := range tests {
		b := newBucketIterator(test.spans)
		var got []int
		v, ok := b.Next()
		for ok {
			got = append(got, v)
			v, ok = b.Next()
		}
		require.Equal(t, test.idxs, got)
	}
}

func TestExpandSpansBothWaysAndInsert(t *testing.T) {
	scenarios := []struct {
		description           string
		spansA, spansB        []histogram.Span
		fInserts, bInserts    []Insert
		bucketsIn, bucketsOut []int64
		mergedSpans           []histogram.Span
	}{
		{
			description: "single prepend at the beginning",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -11, Length: 4},
			},
			fInserts: []Insert{
				{
					pos: 0,
					num: 1,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{0, 6, -3, 0},
			mergedSpans: []histogram.Span{
				{Offset: -11, Length: 4},
			},
		},
		{
			description: "single append at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			fInserts: []Insert{
				{
					pos: 3,
					num: 1,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{6, -3, 0, -3},
			mergedSpans: []histogram.Span{
				{Offset: -10, Length: 4},
			},
		},
		{
			description: "double prepend at the beginning",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -12, Length: 5},
			},
			fInserts: []Insert{
				{
					pos: 0,
					num: 2,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{0, 0, 6, -3, 0},
			mergedSpans: []histogram.Span{
				{Offset: -12, Length: 5},
			},
		},
		{
			description: "double append at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -10, Length: 5},
			},
			fInserts: []Insert{
				{
					pos: 3,
					num: 2,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{6, -3, 0, -3, 0},
			mergedSpans: []histogram.Span{
				{Offset: -10, Length: 5},
			},
		},
		{
			description: "double prepond at the beginning and double append at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -12, Length: 7},
			},
			fInserts: []Insert{
				{
					pos: 0,
					num: 2,
				},
				{
					pos: 3,
					num: 2,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{0, 0, 6, -3, 0, -3, 0},
			mergedSpans: []histogram.Span{
				{Offset: -12, Length: 7},
			},
		},
		{
			description: "single removal of bucket at the start",
			spansA: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			spansB: []histogram.Span{
				{Offset: -9, Length: 3},
			},
			bInserts: []Insert{
				{pos: 0, num: 1},
			},
			bucketsIn:  []int64{1, 2, -1, 2},
			bucketsOut: []int64{1, 2, -1, 2},
			mergedSpans: []histogram.Span{
				{Offset: -10, Length: 4},
			},
		},
		{
			description: "single removal of bucket in the middle",
			spansA: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			spansB: []histogram.Span{
				{Offset: -10, Length: 2},
				{Offset: 1, Length: 1},
			},
			bInserts: []Insert{
				{pos: 2, num: 1},
			},
			bucketsIn:  []int64{1, 2, -1, 2},
			bucketsOut: []int64{1, 2, -1, 2},
			mergedSpans: []histogram.Span{
				{Offset: -10, Length: 4},
			},
		},
		{
			description: "single removal of bucket at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			spansB: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			bInserts: []Insert{
				{pos: 3, num: 1},
			},
			mergedSpans: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			bucketsIn:  []int64{1, 2, -1, 2},
			bucketsOut: []int64{1, 2, -1, 2},
		},
		{
			description: "as described in doc comment",
			spansA: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 2, Length: 1},
				{Offset: 3, Length: 2},
				{Offset: 3, Length: 1},
				{Offset: 1, Length: 1},
			},
			spansB: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 1},
				{Offset: 1, Length: 4},
				{Offset: 3, Length: 3},
			},
			fInserts: []Insert{
				{
					pos: 2,
					num: 1,
				},
				{
					pos: 3,
					num: 2,
				},
				{
					pos: 6,
					num: 1,
				},
			},
			bucketsIn:  []int64{6, -3, 0, -1, 2, 1, -4},
			bucketsOut: []int64{6, -3, -3, 3, -3, 0, 2, 2, 1, -5, 1},
			mergedSpans: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 1},
				{Offset: 1, Length: 4},
				{Offset: 3, Length: 3},
			},
		},
		{
			description: "both forward and backward inserts, complex case",
			spansA: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 2, Length: 1},
				{Offset: 3, Length: 2},
				{Offset: 3, Length: 1},
				{Offset: 1, Length: 1},
			},
			spansB: []histogram.Span{
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 1},
				{Offset: 1, Length: 2},
				{Offset: 1, Length: 1},
				{Offset: 4, Length: 1},
			},
			fInserts: []Insert{
				{
					pos: 2,
					num: 1,
				},
				{
					pos: 3,
					num: 2,
				},
				{
					pos: 6,
					num: 1,
				},
			},
			bInserts: []Insert{
				{
					pos: 0,
					num: 1,
				},
				{
					pos: 5,
					num: 1,
				},
				{
					pos: 6,
					num: 1,
				},
				{
					pos: 7,
					num: 1,
				},
			},
			bucketsIn:  []int64{1, 2, -1, 2, 0, 3, 1},
			bucketsOut: []int64{1, 2, -3, 2, -2, 0, 4, 0, 3, -7, 8},
			mergedSpans: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 1},
				{Offset: 1, Length: 4},
				{Offset: 3, Length: 3},
			},
		},
		{
			description: "inserts with gaps",
			spansA: []histogram.Span{
				{Offset: -19, Length: 2},
				{Offset: 1, Length: 2},
			},
			spansB: []histogram.Span{
				{Offset: -19, Length: 1},
				{Offset: 4, Length: 1},
				{Offset: 3, Length: 1},
			},
			fInserts: []Insert{
				{pos: 4, num: 2},
			},
			bInserts: []Insert{
				{pos: 1, num: 3},
			},
			bucketsIn:  []int64{1, 2, -1, 1},
			bucketsOut: []int64{1, 2, -1, 1, -3, 0},
			mergedSpans: []histogram.Span{
				{Offset: -19, Length: 2},
				{Offset: 1, Length: 3},
				{Offset: 3, Length: 1},
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.description, func(t *testing.T) {
			fInserts, bInserts, m := expandSpansBothWays(s.spansA, s.spansB)
			require.Equal(t, s.fInserts, fInserts)
			require.Equal(t, s.bInserts, bInserts)
			require.Equal(t, s.mergedSpans, m)

			gotBuckets := make([]int64, len(s.bucketsOut))
			insert(s.bucketsIn, gotBuckets, fInserts, true)
			require.Equal(t, s.bucketsOut, gotBuckets)

			floatBucketsIn := make([]float64, len(s.bucketsIn))
			last := s.bucketsIn[0]
			floatBucketsIn[0] = float64(last)
			for i := 1; i < len(floatBucketsIn); i++ {
				last += s.bucketsIn[i]
				floatBucketsIn[i] = float64(last)
			}
			floatBucketsOut := make([]float64, len(s.bucketsOut))
			last = s.bucketsOut[0]
			floatBucketsOut[0] = float64(last)
			for i := 1; i < len(floatBucketsOut); i++ {
				last += s.bucketsOut[i]
				floatBucketsOut[i] = float64(last)
			}
			gotFloatBuckets := make([]float64, len(floatBucketsOut))
			insert(floatBucketsIn, gotFloatBuckets, fInserts, false)
			require.Equal(t, floatBucketsOut, gotFloatBuckets)
		})
	}
}

func TestWriteReadHistogramChunkLayout(t *testing.T) {
	layouts := []struct {
		schema                       int32
		zeroThreshold                float64
		positiveSpans, negativeSpans []histogram.Span
		customValues                 []float64
	}{
		{
			schema:        3,
			zeroThreshold: 0,
			positiveSpans: []histogram.Span{{Offset: -4, Length: 3}, {Offset: 2, Length: 42}},
			negativeSpans: nil,
		},
		{
			schema:        -2,
			zeroThreshold: 2.938735877055719e-39, // Default value in client_golang.
			positiveSpans: nil,
			negativeSpans: []histogram.Span{{Offset: 2, Length: 5}, {Offset: 1, Length: 34}},
		},
		{
			schema:        6,
			zeroThreshold: 1024, // The largest power of two we can encode in one byte.
			positiveSpans: nil,
			negativeSpans: nil,
		},
		{
			schema:        6,
			zeroThreshold: 1025,
			positiveSpans: []histogram.Span{{Offset: 2, Length: 5}, {Offset: 1, Length: 34}, {Offset: 0, Length: 0}}, // Weird span.
			negativeSpans: []histogram.Span{{Offset: -345, Length: 4545}, {Offset: 53645665, Length: 345}, {Offset: 945995, Length: 85848}},
		},
		{
			schema:        6,
			zeroThreshold: 2048,
			positiveSpans: nil,
			negativeSpans: nil,
		},
		{
			schema:        0,
			zeroThreshold: math.Ldexp(0.5, -242), // The smallest power of two we can encode in one byte.
			positiveSpans: []histogram.Span{{Offset: -4, Length: 3}},
			negativeSpans: []histogram.Span{{Offset: 2, Length: 5}, {Offset: 1, Length: 34}},
		},
		{
			schema:        0,
			zeroThreshold: math.Ldexp(0.5, -243),
			positiveSpans: []histogram.Span{{Offset: -4, Length: 3}},
			negativeSpans: []histogram.Span{{Offset: 2, Length: 5}, {Offset: 1, Length: 34}},
		},
		{
			schema:        4,
			zeroThreshold: 42, // Not a power of two.
			positiveSpans: nil,
			negativeSpans: nil,
		},
		{
			schema:        histogram.CustomBucketsSchema,
			positiveSpans: []histogram.Span{{Offset: -4, Length: 3}, {Offset: 2, Length: 42}},
			negativeSpans: nil,
			customValues:  []float64{-5, -2.5, 0, 0.1, 0.25, 0.5, 1, 2, 5, 10, 25, 50, 100, 255, 500, 1000, 50000, 1e7},
		},
		{
			schema:        histogram.CustomBucketsSchema,
			positiveSpans: []histogram.Span{{Offset: -4, Length: 3}, {Offset: 2, Length: 42}},
			negativeSpans: nil,
			customValues:  []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 25.0, 50.0, 100.0},
		},
		{
			schema:        histogram.CustomBucketsSchema,
			positiveSpans: []histogram.Span{{Offset: -4, Length: 3}, {Offset: 2, Length: 42}},
			negativeSpans: nil,
			customValues:  []float64{0.001, 0.002, 0.004, 0.008, 0.016, 0.032, 0.064, 0.128, 0.256, 0.512, 1.024, 2.048, 4.096, 8.192},
		},
		{
			schema:        histogram.CustomBucketsSchema,
			positiveSpans: []histogram.Span{{Offset: -4, Length: 3}, {Offset: 2, Length: 42}},
			negativeSpans: nil,
			customValues:  []float64{1.001, 1.023, 2.01, 4.007, 4.095, 8.001, 8.19, 16.24},
		},
	}

	bs := bstream{}

	for _, l := range layouts {
		writeHistogramChunkLayout(&bs, l.schema, l.zeroThreshold, l.positiveSpans, l.negativeSpans, l.customValues)
	}

	bsr := newBReader(bs.bytes())

	for _, want := range layouts {
		gotSchema, gotZeroThreshold, gotPositiveSpans, gotNegativeSpans, gotCustomBounds, err := readHistogramChunkLayout(&bsr)
		require.NoError(t, err)
		require.Equal(t, want.schema, gotSchema)
		require.Equal(t, want.zeroThreshold, gotZeroThreshold)
		require.Equal(t, want.positiveSpans, gotPositiveSpans)
		require.Equal(t, want.negativeSpans, gotNegativeSpans)
		require.Equal(t, want.customValues, gotCustomBounds)
	}
}

func TestSpansFromBidirectionalCompareSpans(t *testing.T) {
	cases := []struct {
		s1, s2, exp []histogram.Span
	}{
		{ // All empty.
			s1: []histogram.Span{},
			s2: []histogram.Span{},
		},
		{ // Same spans.
			s1: []histogram.Span{},
			s2: []histogram.Span{},
		},
		{
			// Has the cases of
			// 1.  |----|        (partial overlap)
			//        |----|
			//
			// 2.       |-----|  (no gap but no overlap as well)
			//     |---|
			//
			// 3.  |----|        (complete overlap)
			//     |----|
			s1: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 3, Length: 3},
				{Offset: 5, Length: 3},
			},
			s2: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 2, Length: 2},
				{Offset: 2, Length: 3},
				{Offset: 3, Length: 3},
			},
			exp: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 7},
				{Offset: 3, Length: 3},
			},
		},
		{
			// s1 is superset of s2.
			s1: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 3, Length: 5},
				{Offset: 3, Length: 3},
			},
			s2: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 5, Length: 3},
				{Offset: 4, Length: 3},
			},
			exp: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 3, Length: 5},
				{Offset: 3, Length: 3},
			},
		},
		{
			// No overlaps but one span is side by side.
			s1: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 3, Length: 3},
				{Offset: 5, Length: 3},
			},
			s2: []histogram.Span{
				{Offset: 3, Length: 3},
				{Offset: 4, Length: 2},
			},
			exp: []histogram.Span{
				{Offset: 0, Length: 9},
				{Offset: 1, Length: 2},
				{Offset: 2, Length: 3},
			},
		},
		{
			// No buckets in one of them.
			s1: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 3, Length: 3},
				{Offset: 5, Length: 3},
			},
			s2: []histogram.Span{},
			exp: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 3, Length: 3},
				{Offset: 5, Length: 3},
			},
		},
		{ // Zero length spans.
			s1: []histogram.Span{
				{Offset: -5, Length: 0},
				{Offset: 2, Length: 0},
				{Offset: 3, Length: 3},
				{Offset: 1, Length: 0},
				{Offset: 2, Length: 3},
				{Offset: 2, Length: 0},
				{Offset: 2, Length: 0},
				{Offset: 1, Length: 3},
				{Offset: 4, Length: 0},
				{Offset: 5, Length: 0},
			},
			s2: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 2, Length: 2},
				{Offset: 1, Length: 0},
				{Offset: 1, Length: 3},
				{Offset: 3, Length: 3},
			},
			exp: []histogram.Span{
				{Offset: 0, Length: 3},
				{Offset: 1, Length: 7},
				{Offset: 3, Length: 3},
			},
		},
	}

	for _, c := range cases {
		s1c := make([]histogram.Span, len(c.s1))
		s2c := make([]histogram.Span, len(c.s2))
		copy(s1c, c.s1)
		copy(s2c, c.s2)

		_, _, act := expandSpansBothWays(c.s1, c.s2)
		require.Equal(t, c.exp, act)
		// Check that s1 and s2 are not modified.
		require.Equal(t, s1c, c.s1)
		require.Equal(t, s2c, c.s2)
		_, _, act = expandSpansBothWays(c.s2, c.s1)
		require.Equal(t, c.exp, act)
	}
}

func TestExpandIntOrFloatSpansAndBuckets(t *testing.T) {
	testCases := map[string]struct {
		spansA   []histogram.Span
		bucketsA []int64
		spansB   []histogram.Span
		bucketsB []int64

		expectReset           bool
		expectForwardInserts  []Insert
		expectBackwardInserts []Insert
		expectMergedSpans     []histogram.Span
		expectBucketsA        []int64
		expectBucketsB        []int64
	}{
		"empty": {
			spansA:                []histogram.Span{},
			bucketsA:              []int64{},
			spansB:                []histogram.Span{},
			bucketsB:              []int64{},
			expectReset:           false,
			expectForwardInserts:  nil,
			expectBackwardInserts: nil,
			expectMergedSpans:     []histogram.Span{},
			expectBucketsA:        []int64{},
			expectBucketsB:        []int64{},
		},
		"single bucket reset to none": {
			spansA:      []histogram.Span{{Offset: 1, Length: 1}},
			bucketsA:    []int64{1},
			spansB:      []histogram.Span{},
			bucketsB:    []int64{},
			expectReset: true,
		},
		"single bucket reset to lower": {
			spansA:      []histogram.Span{{Offset: 1, Length: 1}},
			bucketsA:    []int64{2},
			spansB:      []histogram.Span{{Offset: 1, Length: 1}},
			bucketsB:    []int64{1},
			expectReset: true,
		},
		"single bucket increase": {
			spansA:                []histogram.Span{{Offset: 1, Length: 1}},
			bucketsA:              []int64{1},
			spansB:                []histogram.Span{{Offset: 1, Length: 1}},
			bucketsB:              []int64{2},
			expectReset:           false,
			expectForwardInserts:  nil,
			expectBackwardInserts: nil,
			expectMergedSpans:     []histogram.Span{{Offset: 1, Length: 1}},
			expectBucketsA:        []int64{1},
			expectBucketsB:        []int64{2},
		},
		"distinct new buckets and increase": {
			// A:  ___1_____
			// B:  22_22___2
			// B': 22_22___2
			spansA:                []histogram.Span{{Offset: 1, Length: 1}},
			bucketsA:              []int64{1},
			spansB:                []histogram.Span{{Offset: -2, Length: 2}, {Offset: 1, Length: 2}, {Offset: 3, Length: 1}},
			bucketsB:              []int64{2, 0, 0, 0, 0},
			expectReset:           false,
			expectForwardInserts:  []Insert{{pos: 0, num: 2, bucketIdx: -2}, {pos: 1, num: 1, bucketIdx: 2}, {pos: 1, num: 1, bucketIdx: 6}},
			expectBackwardInserts: nil,
			expectMergedSpans:     []histogram.Span{{Offset: -2, Length: 2}, {Offset: 1, Length: 2}, {Offset: 3, Length: 1}},
			expectBucketsA:        []int64{0, 0, 1, -1, 0},
			expectBucketsB:        []int64{2, 0, 0, 0, 0},
		},
		"distinct new buckets but reset": {
			// A: ___2_____
			// B: 11_11___1
			spansA:      []histogram.Span{{Offset: 1, Length: 1}},
			bucketsA:    []int64{2},
			spansB:      []histogram.Span{{Offset: -2, Length: 2}, {Offset: 1, Length: 2}, {Offset: 3, Length: 1}},
			bucketsB:    []int64{1, 0, 0, 0, 0},
			expectReset: true,
		},
		"distinct new buckets but missing": {
			// A: ___2_____
			// B: 11__1___1
			spansA:      []histogram.Span{{Offset: 1, Length: 1}},
			bucketsA:    []int64{2},
			spansB:      []histogram.Span{{Offset: -2, Length: 2}, {Offset: 2, Length: 1}, {Offset: 3, Length: 1}},
			bucketsB:    []int64{1, 0, 0, 0},
			expectReset: true,
		},
		"distinct new buckets and missing an empty bucket": {
			// A:  _0__
			// B:  ___1
			// B': _0_1
			spansA:                []histogram.Span{{Offset: 1, Length: 1}},
			bucketsA:              []int64{0},
			spansB:                []histogram.Span{{Offset: 3, Length: 1}},
			bucketsB:              []int64{1},
			expectReset:           false,
			expectForwardInserts:  []Insert{{pos: 1, num: 1, bucketIdx: 3}},
			expectBackwardInserts: []Insert{{pos: 0, num: 1, bucketIdx: 1}},
			expectMergedSpans:     []histogram.Span{{Offset: 1, Length: 1}, {Offset: 1, Length: 1}},
			expectBucketsA:        []int64{0, 0},
			expectBucketsB:        []int64{0, 1},
		},
		"distinct new buckets and missing multiple empty buckets": {
			// Idx: 01234567890123
			// A:   _000_00__0__00
			// B;   ________1_____
			// B':  _000_00_10__00
			spansA:                []histogram.Span{{Offset: 1, Length: 3}, {Offset: 1, Length: 2}, {Offset: 2, Length: 1}, {Offset: 2, Length: 2}},
			bucketsA:              []int64{0, 0, 0, 0, 0, 0, 0, 0},
			spansB:                []histogram.Span{{Offset: 8, Length: 1}},
			bucketsB:              []int64{1},
			expectReset:           false,
			expectForwardInserts:  []Insert{{pos: 5, num: 1, bucketIdx: 8}},
			expectBackwardInserts: []Insert{{pos: 0, num: 3, bucketIdx: 1}, {pos: 0, num: 2, bucketIdx: 5}, {pos: 1, num: 1, bucketIdx: 9}, {pos: 1, num: 2, bucketIdx: 12}},
			expectMergedSpans:     []histogram.Span{{Offset: 1, Length: 3}, {Offset: 1, Length: 2}, {Offset: 1, Length: 2}, {Offset: 2, Length: 2}},
			expectBucketsA:        []int64{0, 0, 0, 0, 0, 0, 0, 0, 0},
			expectBucketsB:        []int64{0, 0, 0, 0, 0, 1, -1, 0, 0},
		},
		"overlap new buckets and missing multiple empty buckets": {
			// Idx: 01234567890123
			// A:   _000_00_10__00
			// B;   ________2_____
			// B':  _000_00_20__00
			spansA:                []histogram.Span{{Offset: 1, Length: 3}, {Offset: 1, Length: 2}, {Offset: 1, Length: 2}, {Offset: 2, Length: 2}},
			bucketsA:              []int64{0, 0, 0, 0, 0, 1, -1, 0, 0},
			spansB:                []histogram.Span{{Offset: 8, Length: 1}},
			bucketsB:              []int64{2},
			expectReset:           false,
			expectForwardInserts:  nil,
			expectBackwardInserts: []Insert{{pos: 0, num: 3, bucketIdx: 1}, {pos: 0, num: 2, bucketIdx: 5}, {pos: 1, num: 1, bucketIdx: 9}, {pos: 1, num: 2, bucketIdx: 12}},
			expectMergedSpans:     []histogram.Span{{Offset: 1, Length: 3}, {Offset: 1, Length: 2}, {Offset: 1, Length: 2}, {Offset: 2, Length: 2}},
			expectBucketsA:        []int64{0, 0, 0, 0, 0, 1, -1, 0, 0},
			expectBucketsB:        []int64{0, 0, 0, 0, 0, 2, -2, 0, 0},
		},
		"overlap new buckets and missing multiple empty buckets with 0 length/offset spans": {
			// Idx: 01234567890123
			// A:   _000_00_10__00
			// B;   ________2_____
			// B':  _000_00_20__00
			spansA:                []histogram.Span{{Offset: 1, Length: 3}, {Offset: 1, Length: 2}, {Offset: 1, Length: 2}, {Offset: 1, Length: 0}, {Offset: 1, Length: 2}},
			bucketsA:              []int64{0, 0, 0, 0, 0, 1, -1, 0, 0},
			spansB:                []histogram.Span{{Offset: 1, Length: 0}, {Offset: 7, Length: 1}},
			bucketsB:              []int64{2},
			expectReset:           false,
			expectForwardInserts:  nil,
			expectBackwardInserts: []Insert{{pos: 0, num: 3, bucketIdx: 1}, {pos: 0, num: 2, bucketIdx: 5}, {pos: 1, num: 1, bucketIdx: 9}, {pos: 1, num: 2, bucketIdx: 12}},
			expectMergedSpans:     []histogram.Span{{Offset: 1, Length: 3}, {Offset: 1, Length: 2}, {Offset: 1, Length: 2}, {Offset: 2, Length: 2}},
			expectBucketsA:        []int64{0, 0, 0, 0, 0, 1, -1, 0, 0},
			expectBucketsB:        []int64{0, 0, 0, 0, 0, 2, -2, 0, 0},
		},
		"new empty buckets between filled buckets": {
			// A:  11212332____1__1
			// B:  122323321__11__1
			// A': 112123320__01__1
			// B': 122323321__11__1
			spansA:               []histogram.Span{{Offset: -51, Length: 8}, {Offset: 11, Length: 1}, {Offset: 14, Length: 1}},
			bucketsA:             []int64{1, 0, 1, -1, 1, 1, 0, -1, -1, 0},
			spansB:               []histogram.Span{{Offset: -51, Length: 9}, {Offset: 9, Length: 2}, {Offset: 14, Length: 1}},
			bucketsB:             []int64{1, 1, 0, 1, -1, 1, 0, -1, -1, 0, 0, 0},
			expectReset:          false,
			expectForwardInserts: []Insert{{pos: 8, num: 1, bucketIdx: -43}, {pos: 8, num: 1, bucketIdx: -33}},
			expectMergedSpans:    []histogram.Span{{Offset: -51, Length: 9}, {Offset: 9, Length: 2}, {Offset: 14, Length: 1}},
			expectBucketsA:       []int64{1, 0, 1, -1, 1, 1, 0, -1, -2, 0, 1, 0},
			// 1 0 1 -1 1 1 0 -1 -2 -2 1 0

			expectBucketsB: []int64{1, 1, 0, 1, -1, 1, 0, -1, -1, 0, 0, 0},
		},
		"real example 1": {
			// I-  6543210987654321
			// A:  0__2_______0__
			// B:  _0130_____00_0
			// A': 00020_____00_0
			// B': 00130_____00_0
			spansA:                []histogram.Span{{Offset: -16, Length: 1}, {Offset: 2, Length: 1}, {Offset: 7, Length: 1}},
			bucketsA:              []int64{0, 2, -2},
			spansB:                []histogram.Span{{Offset: -15, Length: 4}, {Offset: 5, Length: 2}, {Offset: 1, Length: 1}},
			bucketsB:              []int64{0, 1, 2, -3, 0, 0, 0},
			expectReset:           false,
			expectForwardInserts:  []Insert{{pos: 1, num: 2, bucketIdx: -15}, {pos: 2, num: 1, bucketIdx: -12}, {pos: 2, num: 1, bucketIdx: -6}, {pos: 3, num: 1, bucketIdx: -3}},
			expectBackwardInserts: []Insert{{pos: 0, num: 1, bucketIdx: -16}},
			expectMergedSpans:     []histogram.Span{{Offset: -16, Length: 5}, {Offset: 5, Length: 2}, {Offset: 1, Length: 1}},
			expectBucketsA:        []int64{0, 0, 0, 2, -2, 0, 0, 0},
			expectBucketsB:        []int64{0, 0, 1, 2, -3, 0, 0, 0},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Sanity check.
			require.Len(t, tc.bucketsA, countSpans(tc.spansA))
			require.Len(t, tc.bucketsB, countSpans(tc.spansB))
			require.Len(t, tc.expectBucketsA, countSpans(tc.expectMergedSpans))
			require.Len(t, tc.expectBucketsB, countSpans(tc.expectMergedSpans))

			t.Run("integers", func(t *testing.T) {
				fInserts, bInserts, ok := expandIntSpansAndBuckets(tc.spansA, tc.spansB, tc.bucketsA, tc.bucketsB)
				if tc.expectReset {
					require.False(t, ok)
					return
				}
				require.Equal(t, tc.expectForwardInserts, fInserts, "forward inserts")
				require.Equal(t, tc.expectBackwardInserts, bInserts, "backward inserts")

				gotBspans := adjustForInserts(tc.spansB, bInserts)
				require.Equal(t, tc.expectMergedSpans, gotBspans)

				gotAbuckets := make([]int64, len(tc.expectBucketsA))
				insert(tc.bucketsA, gotAbuckets, fInserts, true)
				require.Equal(t, tc.expectBucketsA, gotAbuckets)

				gotBbuckets := make([]int64, len(tc.expectBucketsB))
				insert(tc.bucketsB, gotBbuckets, bInserts, true)
				require.Equal(t, tc.expectBucketsB, gotBbuckets)
			})

			t.Run("floats", func(t *testing.T) {
				aXorValues := make([]xorValue, len(tc.bucketsA))
				absolute := float64(0)
				for i, v := range tc.bucketsA {
					absolute += float64(v)
					aXorValues[i].value = absolute
				}

				makeFloatBuckets := func(in []int64) []float64 {
					out := make([]float64, len(in))
					absolute = float64(0)
					for i, v := range in {
						absolute += float64(v)
						out[i] = absolute
					}
					return out
				}

				bFloatBuckets := makeFloatBuckets(tc.bucketsB)

				fInserts, bInserts, ok := expandFloatSpansAndBuckets(tc.spansA, tc.spansB, aXorValues, bFloatBuckets)
				if tc.expectReset {
					require.False(t, ok)
					return
				}
				require.Equal(t, tc.expectForwardInserts, fInserts, "forward inserts")
				require.Equal(t, tc.expectBackwardInserts, bInserts, "backward inserts")

				gotBspans := adjustForInserts(tc.spansB, bInserts)
				require.Equal(t, tc.expectMergedSpans, gotBspans)

				gotAbuckets := make([]float64, len(tc.expectBucketsA))
				insert(makeFloatBuckets(tc.bucketsA), gotAbuckets, fInserts, false)
				require.Equal(t, makeFloatBuckets(tc.expectBucketsA), gotAbuckets)

				gotBbuckets := make([]float64, len(tc.expectBucketsB))
				insert(makeFloatBuckets(tc.bucketsB), gotBbuckets, bInserts, false)
				require.Equal(t, makeFloatBuckets(tc.expectBucketsB), gotBbuckets)
			})
		})
	}
}
