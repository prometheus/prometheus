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

// The code in this file was largely written by Damian Gryski as part of
// https://github.com/dgryski/go-tsz and published under the license below.
// It was modified to accommodate reading from byte slices without modifying
// the underlying bytes, which would panic when reading from mmap'd
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
		// The below 2 sets ore the ones described in compareSpans's comments.
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

func TestInterjection(t *testing.T) {
	scenarios := []struct {
		description           string
		spansA, spansB        []histogram.Span
		valid                 bool
		interjections         []Interjection
		bucketsIn, bucketsOut []int64
	}{
		{
			description: "single prepend at the beginning",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -11, Length: 4},
			},
			valid: true,
			interjections: []Interjection{
				{
					pos: 0,
					num: 1,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{0, 6, -3, 0},
		},
		{
			description: "single append at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			valid: true,
			interjections: []Interjection{
				{
					pos: 3,
					num: 1,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{6, -3, 0, -3},
		},
		{
			description: "double prepend at the beginning",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -12, Length: 5},
			},
			valid: true,
			interjections: []Interjection{
				{
					pos: 0,
					num: 2,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{0, 0, 6, -3, 0},
		},
		{
			description: "double append at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -10, Length: 5},
			},
			valid: true,
			interjections: []Interjection{
				{
					pos: 3,
					num: 2,
				},
			},
			bucketsIn:  []int64{6, -3, 0},
			bucketsOut: []int64{6, -3, 0, -3, 0},
		},
		{
			description: "double prepond at the beginning and double append at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			spansB: []histogram.Span{
				{Offset: -12, Length: 7},
			},
			valid: true,
			interjections: []Interjection{
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
		},
		{
			description: "single removal of bucket at the start",
			spansA: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			spansB: []histogram.Span{
				{Offset: -9, Length: 3},
			},
			valid: false,
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
			valid: false,
		},
		{
			description: "single removal of bucket at the end",
			spansA: []histogram.Span{
				{Offset: -10, Length: 4},
			},
			spansB: []histogram.Span{
				{Offset: -10, Length: 3},
			},
			valid: false,
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
			valid: true,
			interjections: []Interjection{
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
		},
	}

	for _, s := range scenarios {
		t.Run(s.description, func(t *testing.T) {
			interjections, valid := compareSpans(s.spansA, s.spansB)
			if !s.valid {
				require.False(t, valid, "compareScan unexpectedly returned true")
				return
			}
			require.True(t, valid, "compareScan unexpectedly returned false")
			require.Equal(t, s.interjections, interjections)

			gotBuckets := make([]int64, len(s.bucketsOut))
			interject(s.bucketsIn, gotBuckets, interjections)
			require.Equal(t, s.bucketsOut, gotBuckets)
		})
	}
}

func TestWriteReadHistogramChunkLayout(t *testing.T) {
	layouts := []struct {
		schema                       int32
		zeroThreshold                float64
		positiveSpans, negativeSpans []histogram.Span
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
	}

	bs := bstream{}

	for _, l := range layouts {
		writeHistogramChunkLayout(&bs, l.schema, l.zeroThreshold, l.positiveSpans, l.negativeSpans)
	}

	bsr := newBReader(bs.bytes())

	for _, want := range layouts {
		gotSchema, gotZeroThreshold, gotPositiveSpans, gotNegativeSpans, err := readHistogramChunkLayout(&bsr)
		require.NoError(t, err)
		require.Equal(t, want.schema, gotSchema)
		require.Equal(t, want.zeroThreshold, gotZeroThreshold)
		require.Equal(t, want.positiveSpans, gotPositiveSpans)
		require.Equal(t, want.negativeSpans, gotNegativeSpans)
	}
}
