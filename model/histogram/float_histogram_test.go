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
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFloatHistogramMul(t *testing.T) {
	cases := []struct {
		name     string
		in       *FloatHistogram
		scale    float64
		expected *FloatHistogram
	}{
		{
			"zero value",
			&FloatHistogram{},
			3.1415,
			&FloatHistogram{},
		},
		{
			"zero multiplier",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			0,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       0,
				Count:           0,
				Sum:             0,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{0, 0, 0, 0},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{0, 0, 0, 0},
			},
		},
		{
			"no-op",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			1,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
		},
		{
			"double",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			2,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           3493.3 * 2,
				Sum:             2349209.324 * 2,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{2, 6.6, 8.4, 0.2},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{6.2, 6, 1.234e5 * 2, 2000},
			},
		},
		{
			"triple",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			3,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       33,
				Count:           90,
				Sum:             69,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{3, 0, 9, 12, 21},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{9, 3, 15, 18},
			},
		},
		{
			"negation",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			-1,
			&FloatHistogram{
				ZeroThreshold:    0.01,
				ZeroCount:        -11,
				Count:            -30,
				Sum:              -23,
				PositiveSpans:    []Span{{-2, 2}, {1, 3}},
				PositiveBuckets:  []float64{-1, 0, -3, -4, -7},
				NegativeSpans:    []Span{{3, 2}, {3, 2}},
				NegativeBuckets:  []float64{-3, -1, -5, -6},
				CounterResetHint: GaugeType,
			},
		},
		{
			"negative multiplier",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			-2,
			&FloatHistogram{
				ZeroThreshold:    0.01,
				ZeroCount:        -22,
				Count:            -60,
				Sum:              -46,
				PositiveSpans:    []Span{{-2, 2}, {1, 3}},
				PositiveBuckets:  []float64{-2, 0, -6, -8, -14},
				NegativeSpans:    []Span{{3, 2}, {3, 2}},
				NegativeBuckets:  []float64{-6, -2, -10, -12},
				CounterResetHint: GaugeType,
			},
		},
		{
			"no-op with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
			1,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
		},
		{
			"triple with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			3,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           90,
				Sum:             69,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{3, 0, 9, 12, 21},
				CustomValues:    []float64{1, 2, 3, 4},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.in.Mul(c.scale))
			// Has it also happened in-place?
			require.Equal(t, c.expected, c.in)
		})
	}
}

func TestFloatHistogramCopy(t *testing.T) {
	cases := []struct {
		name     string
		orig     *FloatHistogram
		expected *FloatHistogram
	}{
		{
			name:     "without buckets",
			orig:     &FloatHistogram{},
			expected: &FloatHistogram{},
		},
		{
			name: "with buckets",
			orig: &FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []float64{5, 3, 1.234e5, 1000},
			},
			expected: &FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []float64{5, 3, 1.234e5, 1000},
			},
		},
		{
			name: "with empty buckets and non empty capacity",
			orig: &FloatHistogram{
				PositiveSpans:   make([]Span, 0, 1),
				PositiveBuckets: make([]float64, 0, 1),
				NegativeSpans:   make([]Span, 0, 1),
				NegativeBuckets: make([]float64, 0, 1),
			},
			expected: &FloatHistogram{},
		},
		{
			name: "with custom buckets",
			orig: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				CustomValues:    []float64{1, 2, 3},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				CustomValues:    []float64{1, 2, 3},
			},
		},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			hCopy := tcase.orig.Copy()

			// Modify a primitive value in the original histogram.
			tcase.orig.Sum++
			require.Equal(t, tcase.expected, hCopy)
			assertDeepCopyFHSpans(t, tcase.orig, hCopy, tcase.expected)
		})
	}
}

func TestFloatHistogramCopyTo(t *testing.T) {
	cases := []struct {
		name     string
		orig     *FloatHistogram
		expected *FloatHistogram
	}{
		{
			name:     "without buckets",
			orig:     &FloatHistogram{},
			expected: &FloatHistogram{},
		},
		{
			name: "with buckets",
			orig: &FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []float64{5, 3, 1.234e5, 1000},
			},
			expected: &FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				NegativeSpans:   []Span{{3, 2}},
				NegativeBuckets: []float64{5, 3, 1.234e5, 1000},
			},
		},
		{
			name: "with empty buckets and non empty capacity",
			orig: &FloatHistogram{
				PositiveSpans:   make([]Span, 0, 1),
				PositiveBuckets: make([]float64, 0, 1),
				NegativeSpans:   make([]Span, 0, 1),
				NegativeBuckets: make([]float64, 0, 1),
			},
			expected: &FloatHistogram{},
		},
		{
			name: "with custom buckets",
			orig: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				CustomValues:    []float64{1, 2, 3},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 1}},
				PositiveBuckets: []float64{1, 3, -3, 42},
				CustomValues:    []float64{1, 2, 3},
			},
		},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			hCopy := &FloatHistogram{}
			tcase.orig.CopyTo(hCopy)

			// Modify a primitive value in the original histogram.
			tcase.orig.Sum++
			require.Equal(t, tcase.expected, hCopy)
			assertDeepCopyFHSpans(t, tcase.orig, hCopy, tcase.expected)
		})
	}
}

func assertDeepCopyFHSpans(t *testing.T, orig, hCopy, expected *FloatHistogram) {
	// Do an in-place expansion of an original spans slice.
	orig.PositiveSpans = expandSpans(orig.PositiveSpans)
	orig.PositiveSpans[len(orig.PositiveSpans)-1] = Span{1, 2}

	hCopy.PositiveSpans = expandSpans(hCopy.PositiveSpans)
	expected.PositiveSpans = expandSpans(expected.PositiveSpans)
	// Expand the copy spans and assert that modifying the original has not affected the copy.
	require.Equal(t, expected, hCopy)
}

func TestFloatHistogramDiv(t *testing.T) {
	cases := []struct {
		name     string
		fh       *FloatHistogram
		s        float64
		expected *FloatHistogram
	}{
		{
			"zero value",
			&FloatHistogram{},
			3.1415,
			&FloatHistogram{},
		},
		{
			"zero divisor",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			0,
			&FloatHistogram{
				ZeroThreshold: 0.01,
				Count:         math.Inf(1),
				Sum:           math.Inf(1),
				ZeroCount:     math.Inf(1),
			},
		},
		{
			"no-op",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			1,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
		},
		{
			"half",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			2,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           15,
				Sum:             11.5,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{0.5, 0, 1.5, 2, 3.5},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{1.5, 0.5, 2.5, 3},
			},
		},
		{
			"negation",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			-1,
			&FloatHistogram{
				ZeroThreshold:    0.01,
				ZeroCount:        -5.5,
				Count:            -3493.3,
				Sum:              -2349209.324,
				PositiveSpans:    []Span{{-2, 1}, {2, 3}},
				PositiveBuckets:  []float64{-1, -3.3, -4.2, -0.1},
				NegativeSpans:    []Span{{3, 2}, {3, 2}},
				NegativeBuckets:  []float64{-3.1, -3, -1.234e5, -1000},
				CounterResetHint: GaugeType,
			},
		},
		{
			"negative half",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			-2,
			&FloatHistogram{
				ZeroThreshold:    0.01,
				ZeroCount:        -5.5,
				Count:            -15,
				Sum:              -11.5,
				PositiveSpans:    []Span{{-2, 2}, {1, 3}},
				PositiveBuckets:  []float64{-0.5, 0, -1.5, -2, -3.5},
				NegativeSpans:    []Span{{3, 2}, {3, 2}},
				NegativeBuckets:  []float64{-1.5, -0.5, -2.5, -3},
				CounterResetHint: GaugeType,
			},
		},
		{
			"no-op with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
			1,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
		},
		{
			"half with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			2,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             11.5,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{0.5, 0, 1.5, 2, 3.5},
				CustomValues:    []float64{1, 2, 3, 4},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.fh.Div(c.s))
			// Has it also happened in-place?
			require.Equal(t, c.expected, c.fh)
		})
	}
}

func TestFloatHistogramDetectReset(t *testing.T) {
	cases := []struct {
		name              string
		previous, current *FloatHistogram
		resetExpected     bool
	}{
		{
			"zero values",
			&FloatHistogram{},
			&FloatHistogram{},
			false,
		},
		{
			"no buckets to some buckets",
			&FloatHistogram{},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"some buckets to no buckets",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{},
			true,
		},
		{
			"one bucket appears, nothing else changes",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"one bucket disappears, nothing else changes",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			true,
		},
		{
			"an unpopulated bucket disappears, nothing else changes",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"an unpopulated bucket at the end disappears, nothing else changes",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {1, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 1}, {1, 2}},
				PositiveBuckets: []float64{1, 3.3, 4.2},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"an unpopulated bucket disappears in a histogram with nothing else",
			&FloatHistogram{
				PositiveSpans:   []Span{{23, 1}},
				PositiveBuckets: []float64{0},
			},
			&FloatHistogram{},
			false,
		},
		{
			"zero count goes up",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.6,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"zero count goes down",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.4,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			true,
		},
		{
			"count goes up",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.4,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"count goes down",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.2,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			true,
		},
		{
			"sum goes up",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349210,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"sum goes down",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349200,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"one positive bucket goes up",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.3, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"one positive bucket goes down",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.1, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			true,
		},
		{
			"one negative bucket goes up",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3.1, 1.234e5, 1000},
			},
			false,
		},
		{
			"one negative bucket goes down",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 2.9, 1.234e5, 1000},
			},
			true,
		},
		{
			"zero threshold decreases",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.009,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			true,
		},
		{
			"zero threshold increases without touching any existing buckets",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.011,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"zero threshold increases enough to cover existing buckets",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   1,
				ZeroCount:       7.73,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{1, 3}},
				PositiveBuckets: []float64{3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"zero threshold increases into the middle of an existing buckets",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.3,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			true,
		},
		{
			"schema increases without any other changes",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          1,
				PositiveSpans:   []Span{{-5, 4}, {2, 6}},
				PositiveBuckets: []float64{0.4, 0.6, 1, 0.23, 2, 1.3, 1.2, 3, 0.05, 0.05},
				NegativeSpans:   []Span{{5, 4}, {6, 4}},
				NegativeBuckets: []float64{2, 1.1, 2, 1, 0.234e5, 1e5, 500, 500},
			},
			true,
		},
		{
			"schema decreases without any other changes",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          1,
				PositiveSpans:   []Span{{-5, 4}, {2, 6}},
				PositiveBuckets: []float64{0.4, 0.6, 1, 0.23, 2, 1.3, 1.2, 3, 0.05, 0.05},
				NegativeSpans:   []Span{{5, 4}, {6, 4}},
				NegativeBuckets: []float64{2, 1.1, 2, 1, 0.234e5, 1e5, 500, 500},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"schema decreases and a bucket goes up",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          1,
				PositiveSpans:   []Span{{-5, 4}, {2, 6}},
				PositiveBuckets: []float64{0.4, 0.6, 1, 0.23, 2, 1.3, 1.2, 3, 0.05, 0.05},
				NegativeSpans:   []Span{{5, 4}, {6, 4}},
				NegativeBuckets: []float64{2, 1.1, 2, 1, 0.234e5, 1e5, 500, 500},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 4.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			false,
		},
		{
			"schema decreases and a bucket goes down",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          1,
				PositiveSpans:   []Span{{-5, 4}, {2, 6}},
				PositiveBuckets: []float64{0.4, 0.6, 1, 0.23, 2, 1.3, 1.2, 3, 0.05, 0.05},
				NegativeSpans:   []Span{{5, 4}, {6, 4}},
				NegativeBuckets: []float64{2, 1.1, 2, 1, 0.234e5, 1e5, 500, 500},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       5.5,
				Count:           3493.3,
				Sum:             2349209.324,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 2.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			true,
		},
		{
			"no buckets to some buckets with custom bounds",
			&FloatHistogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, 2, 3},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
			false,
		},
		{
			"some buckets to no buckets with custom bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
			&FloatHistogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, 2, 3},
			},
			true,
		},
		{
			"one bucket appears, nothing else changes with custom bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			false,
		},
		{
			"one bucket disappears, nothing else changes with custom bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 1.23, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			true,
		},
		{
			"an unpopulated bucket disappears, nothing else changes with custom bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			false,
		},
		{
			"one positive bucket goes up with custom bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.3, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			false,
		},
		{
			"one positive bucket goes down with custom bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3493.3,
				Sum:             2349209.324,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3.3, 4.1, 0.1},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			true,
		},
		{
			"mismatched custom bounds - no reset when all buckets increase",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           200,
				Sum:             1000,
				PositiveSpans:   []Span{{0, 4}},
				PositiveBuckets: []float64{20, 35, 40, 50}, // Previous: buckets for: (-Inf,0], (0,1], (1,2], (2,3], then (3,+Inf]
				CustomValues:    []float64{0, 1, 2, 3},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           200,
				Sum:             1000,
				PositiveSpans:   []Span{{0, 5}},
				PositiveBuckets: []float64{25, 15, 40, 50, 70}, // Current: buckets for: (-Inf,0], (0,0.5], (0.5,1], (1,2], (2,3], then (3,+Inf]
				CustomValues:    []float64{0, 0.5, 1, 2, 3},
			},
			false, // No reset: (-Inf,0] increases from 20 to 25, (0,1] increases from 35 to 55, (1,2] increases from 40 to 50, (2,3] increases from 50 to 70
		},
		{
			"mismatched custom bounds - reset in middle bucket",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           100,
				Sum:             500,
				PositiveSpans:   []Span{{1, 4}},
				PositiveBuckets: []float64{10, 15, 20, 25}, // Buckets for: [0,1], [1,3], [3,5], [5,+Inf]
				CustomValues:    []float64{0, 1, 3, 5},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           100,
				Sum:             500,
				PositiveSpans:   []Span{{1, 5}},
				PositiveBuckets: []float64{10, 16, 10, 5, 25}, // Buckets for: [0,1], [1,2], [2,3], [3,5], [5,+Inf]
				CustomValues:    []float64{0, 1, 2, 3, 5},
			},
			true, // Reset detected: [1,3] bucket decreased from 20 to 15
		},
		{
			"mismatched custom bounds - reset in last bucket",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           100,
				Sum:             500,
				PositiveSpans:   []Span{{1, 3}},
				PositiveBuckets: []float64{10, 20, 20}, // Buckets for: [0,1], [1,2], [2,+Inf]
				CustomValues:    []float64{0, 1, 2},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           100,
				Sum:             500,
				PositiveSpans:   []Span{{1, 4}},
				PositiveBuckets: []float64{100, 200, 8, 7}, // Buckets for: [0,1], [1,2], [2,3], [3,+Inf]
				CustomValues:    []float64{0, 1, 2, 3},
			},
			true, // Reset detected: [2,+Inf] bucket decreased from 20 to 15
		},
		{
			"mismatched custom bounds - no common bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           100,
				Sum:             500,
				PositiveSpans:   []Span{{1, 3}},
				PositiveBuckets: []float64{10, 20, 30}, // Buckets for: [1,2], [2,3], [3,+Inf]
				CustomValues:    []float64{4, 5, 6},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           100,
				Sum:             500,
				PositiveSpans:   []Span{{1, 3}},
				PositiveBuckets: []float64{15, 25, 35}, // Buckets for: [4,5], [5,6], [6,+Inf]
				CustomValues:    []float64{1, 2, 3},
			},
			false, // no decrease in aggregated single +Inf bounded bucket
		},
		{
			"mismatched custom bounds - sparse common bounds",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           200,
				Sum:             1000,
				PositiveSpans:   []Span{{0, 4}},
				PositiveBuckets: []float64{10, 20, 30, 40}, // Previous: buckets for: (-Inf,0], (0,1], (1,3], (3,5], then (5,+Inf]
				CustomValues:    []float64{0, 1, 3, 5},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           200,
				Sum:             1000,
				PositiveSpans:   []Span{{0, 5}},
				PositiveBuckets: []float64{15, 25, 70, 50, 100}, // Current: buckets for: (-Inf,0], (0,2], (2,3], (3,4], (4,5], then (5,+Inf]
				CustomValues:    []float64{0, 2, 3, 4, 5},
			},
			false, // No reset: common bounds [0,3,5] all increase when mapped
		},
		{
			"reset detected with mismatched custom bounds and split bucket spans",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           200,
				Sum:             1000,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},        // Split spans: buckets at indices 0,1 and 3,4,5
				PositiveBuckets: []float64{10, 20, 30, 40, 50}, // Buckets for: (-Inf,0], (0,1], skip (1,2], then (2,3], (3,4], (4,5]
				CustomValues:    []float64{0, 1, 2, 3, 4, 5},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           200,
				Sum:             1000,
				PositiveSpans:   []Span{{0, 3}, {1, 2}},        // Split spans: buckets at indices 0,1,2 and 4,5
				PositiveBuckets: []float64{15, 25, 35, 25, 60}, // Buckets for: (-Inf,0], (0,1], (1,3], skip (3,4], then (4,5], (5,7]
				CustomValues:    []float64{0, 1, 3, 4, 5, 7},
			},
			true, // Reset detected: bucket (3,4] goes from 40 to 0 (missing in current histogram)
		},
		{
			"no reset with mismatched custom bounds and split bucket spans",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           300,
				Sum:             1500,
				PositiveSpans:   []Span{{0, 2}, {2, 3}},        // Split spans: buckets at indices 0,1 and 4,5,6
				PositiveBuckets: []float64{10, 20, 30, 40, 50}, // Buckets for: (-Inf,0], (0,1], skip (1,2], (2,3], then (3,4], (4,5], (5,6]
				CustomValues:    []float64{0, 1, 2, 3, 4, 5, 6},
			},
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           300,
				Sum:             1500,
				PositiveSpans:   []Span{{0, 3}, {1, 2}},        // Split spans: buckets at indices 0,1,2 and 4,5
				PositiveBuckets: []float64{12, 25, 45, 75, 95}, // Buckets for: (-Inf,0], (0,0.5], (0.5,1], skip (1,3], then (3,5], (5,7]
				CustomValues:    []float64{0, 0.5, 1, 3, 5, 7},
			},
			false, // No reset: all mapped buckets increase
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.resetExpected, c.current.DetectReset(c.previous))
		})
	}
}

func TestFloatHistogramCompact(t *testing.T) {
	cases := []struct {
		name            string
		in              *FloatHistogram
		maxEmptyBuckets int
		expected        *FloatHistogram
	}{
		{
			"empty histogram",
			&FloatHistogram{},
			0,
			&FloatHistogram{},
		},
		{
			"nothing should happen",
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
		},
		{
			"eliminate zero offsets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {0, 3}, {0, 1}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {0, 2}, {2, 1}, {0, 1}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 5}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 4}, {2, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"eliminate zero length",
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 2}, {2, 0}, {3, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {0, 0}, {2, 0}, {1, 4}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"eliminate multiple zero length spans",
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 2}, {2, 0}, {2, 0}, {2, 0}, {3, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 2}, {9, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
			},
		},
		{
			"cut empty buckets at start or end",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 4}, {5, 3}},
				PositiveBuckets: []float64{0, 0, 1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4, 0},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"cut empty buckets at start and end",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 4}, {5, 6}},
				PositiveBuckets: []float64{0, 0, 1, 3.3, 4.2, 0.1, 3.3, 0, 0, 0},
				NegativeSpans:   []Span{{-2, 4}, {3, 5}},
				NegativeBuckets: []float64{0, 0, 3.1, 3, 1.234e5, 1000, 3, 4, 0},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"cut empty buckets in the middle",
			&FloatHistogram{
				PositiveSpans:   []Span{{5, 4}},
				PositiveBuckets: []float64{1, 3, 0, 2},
			},
			0,
			&FloatHistogram{
				PositiveSpans: []Span{
					{Offset: 5, Length: 2},
					{Offset: 1, Length: 1},
				},
				PositiveBuckets: []float64{1, 3, 2},
			},
		},
		{
			"cut empty buckets at start or end of spans, even in the middle",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 6}},
				PositiveBuckets: []float64{0, 0, 1, 3.3, 0, 0, 4.2, 0.1, 3.3, 0, 0, 0},
				NegativeSpans:   []Span{{0, 2}, {2, 6}},
				NegativeBuckets: []float64{3.1, 3, 0, 1.234e5, 1000, 3, 4, 0},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 2}, {5, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 4}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"cut empty buckets at start and end - also merge spans due to maxEmptyBuckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 4}, {5, 3}},
				PositiveBuckets: []float64{0, 0, 1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4, 0},
			},
			10,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 10}},
				PositiveBuckets: []float64{1, 3.3, 0, 0, 0, 0, 0, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 9}},
				NegativeBuckets: []float64{3.1, 3, 0, 0, 0, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"cut empty buckets from the middle of a span",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []float64{0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 0, 3, 4},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 1}, {3, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 2}, {1, 2}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 3, 4},
			},
		},
		{
			"cut out a span containing only empty buckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 3}, {2, 2}, {3, 4}},
				PositiveBuckets: []float64{0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {7, 4}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
			},
		},
		{
			"cut empty buckets from the middle of a span, avoiding none due to maxEmptyBuckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 4}},
				PositiveBuckets: []float64{1, 0, 0, 3.3},
			},
			1,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 1}},
				PositiveBuckets: []float64{1, 3.3},
			},
		},
		{
			"cut empty buckets and merge spans due to maxEmptyBuckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 4}, {3, 1}},
				PositiveBuckets: []float64{1, 0, 0, 3.3, 4.2},
			},
			1,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 1}, {3, 1}},
				PositiveBuckets: []float64{1, 3.3, 4.2},
			},
		},
		{
			"cut empty buckets from the middle of a span, avoiding some due to maxEmptyBuckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}, {10, 2}},
				PositiveBuckets: []float64{0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3, 2, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 0, 3, 4},
			},
			1,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 1}, {2, 1}, {3, 3}, {10, 2}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3, 2, 3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 0, 3, 4},
			},
		},
		{
			"avoiding all cutting of empty buckets from the middle of a chunk due to maxEmptyBuckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []float64{0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 0, 3, 4},
			},
			2,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 4}, {3, 3}},
				PositiveBuckets: []float64{1, 0, 0, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 0, 3, 4},
			},
		},
		{
			"everything merged into one span due to maxEmptyBuckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []float64{0, 0, 1, 0, 0, 3.3, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 2}, {3, 5}},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000, 0, 3, 4},
			},
			3,
			&FloatHistogram{
				PositiveSpans:   []Span{{-2, 10}},
				PositiveBuckets: []float64{1, 0, 0, 3.3, 0, 0, 0, 4.2, 0.1, 3.3},
				NegativeSpans:   []Span{{0, 10}},
				NegativeBuckets: []float64{3.1, 3, 0, 0, 0, 1.234e5, 1000, 0, 3, 4},
			},
		},
		{
			"only empty buckets and maxEmptyBuckets greater zero",
			&FloatHistogram{
				PositiveSpans:   []Span{{-4, 6}, {3, 3}},
				PositiveBuckets: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0},
				NegativeSpans:   []Span{{0, 7}},
				NegativeBuckets: []float64{0, 0, 0, 0, 0, 0, 0},
			},
			3,
			&FloatHistogram{
				PositiveSpans:   []Span{},
				PositiveBuckets: []float64{},
				NegativeSpans:   []Span{},
				NegativeBuckets: []float64{},
			},
		},
		{
			"multiple spans of only empty buckets",
			&FloatHistogram{
				PositiveSpans:   []Span{{-10, 2}, {2, 1}, {3, 3}},
				PositiveBuckets: []float64{0, 0, 0, 0, 2, 3},
				NegativeSpans:   []Span{{-10, 2}, {2, 1}, {3, 3}},
				NegativeBuckets: []float64{2, 3, 0, 0, 0, 0},
			},
			0,
			&FloatHistogram{
				PositiveSpans:   []Span{{-1, 2}},
				PositiveBuckets: []float64{2, 3},
				NegativeSpans:   []Span{{-10, 2}},
				NegativeBuckets: []float64{2, 3},
			},
		},
		{
			"nothing should happen with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
			0,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 1}, {2, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				CustomValues:    []float64{1, 2, 3},
			},
		},
		{
			"eliminate zero offsets with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 1}, {0, 3}, {0, 1}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			0,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 5}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				CustomValues:    []float64{1, 2, 3, 4},
			},
		},
		{
			"eliminate multiple zero length spans with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 2}, {2, 0}, {2, 0}, {2, 0}, {3, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				CustomValues:    []float64{1, 2, 3, 4},
			},
			0,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 2}, {9, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				CustomValues:    []float64{1, 2, 3, 4},
			},
		},
		{
			"cut empty buckets at start and end with custom buckets",
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 4}, {5, 6}},
				PositiveBuckets: []float64{0, 0, 1, 3.3, 4.2, 0.1, 3.3, 0, 0, 0},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
			0,
			&FloatHistogram{
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{2, 2}, {5, 3}},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 3.3},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
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

func TestFloatHistogramAdd(t *testing.T) {
	cases := []struct {
		name                     string
		in1, in2, expected       *FloatHistogram
		expErrMsg                string
		expCounterResetCollision bool
		expNHCBBoundsReconciled  bool
	}{
		{
			name: "same bucket layout",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 5, 7, 13},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{4, 2, 9, 10},
			},
		},
		{
			name: "same bucket layout, defined differently",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {1, 1}, {0, 2}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-2, 2}, {1, 2}, {0, 1}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				NegativeSpans:   []Span{{3, 7}},
				NegativeBuckets: []float64{1, 1, 0, 0, 0, 4, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 2}, {1, 1}, {0, 2}},
				PositiveBuckets: []float64{1, 0, 5, 7, 13},
				NegativeSpans:   []Span{{3, 5}, {0, 2}},
				NegativeBuckets: []float64{4, 2, 0, 0, 0, 9, 10},
			},
		},
		{
			name: "non-overlapping spans",
			in1: &FloatHistogram{
				ZeroThreshold:   0.001,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.001,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{0, 2}, {3, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6},
				NegativeSpans:   []Span{{-9, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.001,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 4}, {0, 6}},
				PositiveBuckets: []float64{1, 0, 5, 4, 3, 4, 7, 2, 3, 6},
				NegativeSpans:   []Span{{-9, 2}, {3, 2}, {5, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4, 3, 1, 5, 6},
			},
		},
		{
			name: "non-overlapping spans inverted order",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{0, 2}, {3, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6},
				NegativeSpans:   []Span{{-6, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 2}, {0, 5}, {0, 3}},
				PositiveBuckets: []float64{1, 0, 5, 4, 3, 4, 7, 2, 3, 6},
				NegativeSpans:   []Span{{-6, 2}, {1, 2}, {4, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4, 3, 1, 5, 6},
			},
		},
		{
			name: "overlapping spans",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 4}, {0, 4}},
				PositiveBuckets: []float64{1, 5, 4, 2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "overlapping spans inverted order",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 5}, {0, 3}},
				PositiveBuckets: []float64{1, 5, 4, 2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "schema change",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				Schema:          0,
				PositiveSpans:   []Span{{-1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				Schema:          1,
				PositiveSpans:   []Span{{-4, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 3, 2, 2, 3, 4},
				NegativeSpans:   []Span{{6, 3}, {6, 4}},
				NegativeBuckets: []float64{3, 0.5, 0.5, 2, 3, 2, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 5}, {0, 3}},
				PositiveBuckets: []float64{1, 5, 4, 2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "larger zero bucket in first histogram",
			in1: &FloatHistogram{
				ZeroThreshold:   1,
				ZeroCount:       17,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{1, 2}, {0, 3}},
				PositiveBuckets: []float64{2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   1,
				ZeroCount:       29,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{1, 2}, {0, 3}},
				PositiveBuckets: []float64{2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "larger zero bucket in second histogram",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   1,
				ZeroCount:       17,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{1, 2}, {0, 3}},
				PositiveBuckets: []float64{2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   1,
				ZeroCount:       29,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{1, 5}},
				PositiveBuckets: []float64{2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "larger zero threshold in first histogram ends up inside a populated bucket of second histogram",
			in1: &FloatHistogram{
				ZeroThreshold:   0.2,
				ZeroCount:       17,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{1, 2}, {0, 3}},
				PositiveBuckets: []float64{2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.25,
				ZeroCount:       29,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-1, 1}, {1, 5}},
				PositiveBuckets: []float64{0, 2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "larger zero threshold in second histogram ends up inside a populated bucket of first histogram",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.2,
				ZeroCount:       17,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{1, 2}, {0, 3}},
				PositiveBuckets: []float64{2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.25,
				ZeroCount:       29,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{1, 5}},
				PositiveBuckets: []float64{2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "schema change combined with larger zero bucket in second histogram",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 5}, {0, 3}},
				PositiveBuckets: []float64{2, 5, 4, 2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.25,
				ZeroCount:       12,
				Count:           30,
				Sum:             2.345,
				Schema:          1,
				PositiveSpans:   []Span{{-3, 2}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 3, 2, 2, 3, 4},
				NegativeSpans:   []Span{{6, 3}, {6, 4}},
				NegativeBuckets: []float64{3, 0.5, 0.5, 2, 3, 2, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.25,
				ZeroCount:       22,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-1, 7}},
				PositiveBuckets: []float64{6, 4, 2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "schema change combined with larger zero bucket in first histogram",
			in1: &FloatHistogram{
				ZeroThreshold:   0.25,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				Schema:          0,
				PositiveSpans:   []Span{{-1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				Schema:          1,
				PositiveSpans:   []Span{{-4, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 3, 2, 2, 3, 4},
				NegativeSpans:   []Span{{6, 3}, {6, 4}},
				NegativeBuckets: []float64{3, 0.5, 0.5, 2, 3, 2, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.25,
				ZeroCount:       20,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 6, 10, 9, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{3, 2, 1, 4, 9, 6},
			},
		},
		{
			name: "same custom bucket layout",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             2.345,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           11,
				Sum:             1.234,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           26,
				Sum:             3.579,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 5, 7, 13},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
		},
		{
			name: "same custom bucket layout, defined differently",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             2.345,
				PositiveSpans:   []Span{{0, 2}, {1, 1}, {0, 2}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           11,
				Sum:             1.234,
				PositiveSpans:   []Span{{0, 2}, {1, 2}, {0, 1}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           26,
				Sum:             3.579,
				PositiveSpans:   []Span{{0, 2}, {1, 1}, {0, 2}},
				PositiveBuckets: []float64{1, 0, 5, 7, 13},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
		},
		{
			name: "non-overlapping spans with custom buckets",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             2.345,
				PositiveSpans:   []Span{{0, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           20,
				Sum:             1.234,
				PositiveSpans:   []Span{{2, 2}, {3, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           35,
				Sum:             3.579,
				PositiveSpans:   []Span{{0, 4}, {0, 6}},
				PositiveBuckets: []float64{1, 0, 5, 4, 3, 4, 7, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
		},
		{
			name: "non-overlapping spans inverted order with custom buckets",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           20,
				Sum:             1.234,
				PositiveSpans:   []Span{{2, 2}, {3, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             2.345,
				PositiveSpans:   []Span{{0, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           35,
				Sum:             3.579,
				PositiveSpans:   []Span{{0, 4}, {0, 6}},
				PositiveBuckets: []float64{1, 0, 5, 4, 3, 4, 7, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			},
		},
		{
			name: "overlapping spans with custom buckets",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             2.345,
				PositiveSpans:   []Span{{0, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           27,
				Sum:             1.234,
				PositiveSpans:   []Span{{1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           42,
				Sum:             3.579,
				PositiveSpans:   []Span{{0, 4}, {0, 4}},
				PositiveBuckets: []float64{1, 5, 4, 2, 6, 10, 9, 5},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
		},
		{
			name: "overlapping spans inverted order with custom buckets",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           27,
				Sum:             1.234,
				PositiveSpans:   []Span{{1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             2.345,
				PositiveSpans:   []Span{{0, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           42,
				Sum:             3.579,
				PositiveSpans:   []Span{{0, 4}, {0, 4}},
				PositiveBuckets: []float64{1, 5, 4, 2, 6, 10, 9, 5},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
		},
		{
			name: "custom buckets with partial intersection",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           10,
				Sum:             100,
				PositiveSpans:   []Span{{0, 3}},
				PositiveBuckets: []float64{2, 3, 5},
				CustomValues:    []float64{1, 2.5},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           8,
				Sum:             80,
				PositiveSpans:   []Span{{0, 4}},
				PositiveBuckets: []float64{1, 2, 3, 2},
				CustomValues:    []float64{1, 2, 3},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           18,
				Sum:             180,
				PositiveSpans:   []Span{{0, 2}},
				PositiveBuckets: []float64{3, 15},
				CustomValues:    []float64{1},
			},
			expNHCBBoundsReconciled: true,
		},
		{
			name: "different custom bucket layout - intersection and rollup",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           6,
				Sum:             2.345,
				PositiveSpans:   []Span{{0, 1}, {1, 1}},
				PositiveBuckets: []float64{1, 5},
				CustomValues:    []float64{2, 4},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           220,
				Sum:             1.234,
				PositiveSpans:   []Span{{0, 2}, {1, 1}, {0, 2}},
				PositiveBuckets: []float64{10, 20, 40, 50, 100},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           226,
				Sum:             3.579,
				PositiveSpans:   []Span{{0, 3}},
				PositiveBuckets: []float64{1 + 10 + 20, 40, 5 + 50 + 100},
				CustomValues:    []float64{2, 4},
			},
			expNHCBBoundsReconciled: true,
		},
		{
			name: "custom buckets with no common boundaries except +Inf",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           3,
				Sum:             50,
				PositiveSpans:   []Span{{0, 2}},
				PositiveBuckets: []float64{1, 2},
				CustomValues:    []float64{1.5},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           30,
				Sum:             40,
				PositiveSpans:   []Span{{0, 2}},
				PositiveBuckets: []float64{10, 20},
				CustomValues:    []float64{2.5},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           33,
				Sum:             90,
				PositiveSpans:   []Span{{0, 1}},
				PositiveBuckets: []float64{1 + 2 + 10 + 20},
				CustomValues:    nil,
			},
			expNHCBBoundsReconciled: true,
		},
		{
			name: "mix exponential and custom buckets histograms",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           59,
				Sum:             1.234,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 5}, {0, 3}},
				PositiveBuckets: []float64{2, 5, 4, 2, 3, 6, 7, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{4, 10, 1, 4, 14, 7},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           11,
				Sum:             12,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			expErrMsg: "cannot apply this operation on histograms with a mix of exponential and custom bucket schemas",
		},
		{
			name: "warn on counter reset hint collision",
			in1: &FloatHistogram{
				Schema:           CustomBucketsSchema,
				CounterResetHint: CounterReset,
			},
			in2: &FloatHistogram{
				Schema:           CustomBucketsSchema,
				CounterResetHint: NotCounterReset,
			},
			expErrMsg:                "",
			expCounterResetCollision: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testHistogramAdd(t, c.in1, c.in2, c.expected, c.expErrMsg, c.expCounterResetCollision, c.expNHCBBoundsReconciled)
			testHistogramAdd(t, c.in2, c.in1, c.expected, c.expErrMsg, c.expCounterResetCollision, c.expNHCBBoundsReconciled)
		})
	}
}

func testHistogramAdd(t *testing.T, a, b, expected *FloatHistogram, expErrMsg string, expCounterResetCollision, expNHCBBoundsReconciled bool) {
	require.NoError(t, a.Validate(), "a")
	require.NoError(t, b.Validate(), "b")

	var (
		aCopy        = a.Copy()
		bCopy        = b.Copy()
		expectedCopy *FloatHistogram
	)

	if expected != nil {
		expectedCopy = expected.Copy()
	}

	res, counterResetCollision, nhcbBoundsReconciled, err := aCopy.Add(bCopy)
	if expErrMsg != "" {
		require.EqualError(t, err, expErrMsg)
	} else {
		require.NoError(t, err)
	}

	// Check that the warnings are correct.
	require.Equal(t, expCounterResetCollision, counterResetCollision)
	require.Equal(t, expNHCBBoundsReconciled, nhcbBoundsReconciled)

	if expected != nil {
		res.Compact(0)
		expectedCopy.Compact(0)

		require.Equal(t, expectedCopy, res)

		// Has it also happened in-place?
		require.Equal(t, expectedCopy, aCopy)

		// Check that the argument was not mutated.
		require.Equal(t, b, bCopy)
	}
}

func TestFloatHistogramSub(t *testing.T) {
	// This has fewer test cases than TestFloatHistogramAdd because Add and
	// Sub share most of the trickier code.
	cases := []struct {
		name                     string
		in1, in2, expected       *FloatHistogram
		expErrMsg                string
		expCounterResetCollision bool
		expNHCBBoundsReconciled  bool
	}{
		{
			name: "same bucket layout",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             23,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             12,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       3,
				Count:           9,
				Sum:             11,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 1, 1, 1},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{2, 0, 1, 2},
			},
		},
		{
			name: "schema change",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           59,
				Sum:             1.234,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 5}, {0, 3}},
				PositiveBuckets: []float64{2, 5, 4, 2, 3, 6, 7, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{4, 10, 1, 4, 14, 7},
			},
			in2: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       2,
				Count:           19,
				Sum:             0.345,
				Schema:          1,
				PositiveSpans:   []Span{{-4, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 1, 2, 2, 3, 4},
				NegativeSpans:   []Span{{6, 3}, {6, 4}},
				NegativeBuckets: []float64{3, 0.5, 0.5, 2, 3, 2, 4},
			},
			expected: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       6,
				Count:           40,
				Sum:             0.889,
				PositiveSpans:   []Span{{-2, 5}, {0, 3}},
				PositiveBuckets: []float64{1, 5, 4, 2, 2, 2, 0, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{1, 9, 1, 4, 9, 1},
			},
		},
		{
			name: "same custom bucket layout",
			in1: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           15,
				Sum:             23,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           11,
				Sum:             12,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
			expected: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           4,
				Sum:             11,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 1, 1, 1},
				CustomValues:    []float64{1, 2, 3, 4, 5},
			},
		},
		{
			name: "different custom bucket layout - with intersection and rollup",
			in1: &FloatHistogram{
				Schema:           CustomBucketsSchema,
				Count:            220,
				Sum:              9.9,
				PositiveSpans:    []Span{{0, 2}, {1, 1}, {0, 2}},
				PositiveBuckets:  []float64{10, 20, 40, 50, 100},
				CustomValues:     []float64{1, 2, 3, 4, 5},
				CounterResetHint: GaugeType,
			},
			in2: &FloatHistogram{
				Schema:           CustomBucketsSchema,
				Count:            6,
				Sum:              4.4,
				PositiveSpans:    []Span{{0, 1}, {1, 1}},
				PositiveBuckets:  []float64{1, 5},
				CustomValues:     []float64{2, 4},
				CounterResetHint: GaugeType,
			},
			expected: &FloatHistogram{
				Schema:           CustomBucketsSchema,
				Count:            214,
				Sum:              5.5,
				PositiveSpans:    []Span{{0, 3}},
				PositiveBuckets:  []float64{10 + 20 - 1, 40, 50 + 100 - 5},
				CustomValues:     []float64{2, 4},
				CounterResetHint: GaugeType,
			},
			expNHCBBoundsReconciled: true,
		},
		{
			name: "mix exponential and custom buckets histograms",
			in1: &FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           59,
				Sum:             1.234,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 5}, {0, 3}},
				PositiveBuckets: []float64{2, 5, 4, 2, 3, 6, 7, 5},
				NegativeSpans:   []Span{{3, 3}, {1, 3}},
				NegativeBuckets: []float64{4, 10, 1, 4, 14, 7},
			},
			in2: &FloatHistogram{
				Schema:          CustomBucketsSchema,
				Count:           11,
				Sum:             12,
				PositiveSpans:   []Span{{0, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6},
			},
			expErrMsg: "cannot apply this operation on histograms with a mix of exponential and custom bucket schemas",
		},
		{
			name: "warn on counter reset hint collision",
			in1: &FloatHistogram{
				Schema:           CustomBucketsSchema,
				CounterResetHint: CounterReset,
			},
			in2: &FloatHistogram{
				Schema:           CustomBucketsSchema,
				CounterResetHint: NotCounterReset,
			},
			expErrMsg:                "",
			expCounterResetCollision: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testFloatHistogramSub(t, c.in1, c.in2, c.expected, c.expErrMsg, c.expCounterResetCollision, c.expNHCBBoundsReconciled)

			var expectedNegative *FloatHistogram
			if c.expected != nil {
				expectedNegative = c.expected.Copy().Mul(-1)
				// Mul(-1) sets the counter reset hint to
				// GaugeType, but we want to retain the original
				// counter reset hint for this test.
				expectedNegative.CounterResetHint = c.expected.CounterResetHint
			}
			testFloatHistogramSub(t, c.in2, c.in1, expectedNegative, c.expErrMsg, c.expCounterResetCollision, c.expNHCBBoundsReconciled)
		})
	}
}

func testFloatHistogramSub(t *testing.T, a, b, expected *FloatHistogram, expErrMsg string, expCounterResetCollision, expNHCBBoundsReconciled bool) {
	require.NoError(t, a.Validate(), "a")
	require.NoError(t, b.Validate(), "b")

	var (
		aCopy        = a.Copy()
		bCopy        = b.Copy()
		expectedCopy *FloatHistogram
	)

	if expected != nil {
		expectedCopy = expected.Copy()
	}

	res, counterResetCollision, nhcbBoundsReconciled, err := aCopy.Sub(bCopy)
	if expErrMsg != "" {
		require.EqualError(t, err, expErrMsg)
	} else {
		require.NoError(t, err)
	}

	if expected != nil {
		res.Compact(0)
		expectedCopy.Compact(0)

		require.Equal(t, expectedCopy, res)

		// Has it also happened in-place?
		require.Equal(t, expectedCopy, aCopy)

		// Check that the argument was not mutated.
		require.Equal(t, b, bCopy)

		// Check that the warnings are correct.
		require.Equal(t, expCounterResetCollision, counterResetCollision)
		require.Equal(t, expNHCBBoundsReconciled, nhcbBoundsReconciled)
	}
}

func TestFloatHistogramCopyToSchema(t *testing.T) {
	cases := []struct {
		name         string
		targetSchema int32
		in, expected *FloatHistogram
	}{
		{
			"no schema change",
			1,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				Schema:          1,
				PositiveSpans:   []Span{{-4, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 3, 2, 2, 3, 4},
				NegativeSpans:   []Span{{6, 3}, {6, 4}},
				NegativeBuckets: []float64{3, 0.5, 0.5, 2, 3, 2, 4},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				Schema:          1,
				PositiveSpans:   []Span{{-4, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 3, 2, 2, 3, 4},
				NegativeSpans:   []Span{{6, 3}, {6, 4}},
				NegativeBuckets: []float64{3, 0.5, 0.5, 2, 3, 2, 4},
			},
		},
		{
			"schema change",
			0,
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				Schema:          1,
				PositiveSpans:   []Span{{-4, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 3, 2, 2, 3, 4},
				NegativeSpans:   []Span{{6, 3}, {6, 4}},
				NegativeBuckets: []float64{3, 0.5, 0.5, 2, 3, 2, 4},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				Schema:          0,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
		},
		{
			"no schema change for custom buckets",
			CustomBucketsSchema,
			&FloatHistogram{
				Count:           30,
				Sum:             2.345,
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 3, 2, 2, 3, 4},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
			&FloatHistogram{
				Count:           30,
				Sum:             2.345,
				Schema:          CustomBucketsSchema,
				PositiveSpans:   []Span{{0, 3}, {5, 5}},
				PositiveBuckets: []float64{1, 0, 0, 3, 2, 2, 3, 4},
				CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inCopy := c.in.Copy()
			require.Equal(t, c.expected, c.in.CopyToSchema(c.targetSchema))
			// Check that the receiver histogram was not mutated:
			require.Equal(t, inCopy, c.in)
		})
	}
}

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
	var expBuckets, actBuckets []Bucket[float64]
	it := h.PositiveBucketIterator()
	for it.Next() {
		// Append in reverse to check reversed list.
		expBuckets = append([]Bucket[float64]{it.At()}, expBuckets...)
	}
	it = h.PositiveReverseBucketIterator()
	for it.Next() {
		actBuckets = append(actBuckets, it.At())
	}
	require.NotEmpty(t, expBuckets)
	require.NotEmpty(t, actBuckets)
	require.Equal(t, expBuckets, actBuckets)

	// Negative buckets.
	expBuckets = expBuckets[:0]
	actBuckets = actBuckets[:0]
	it = h.NegativeBucketIterator()
	for it.Next() {
		// Append in reverse to check reversed list.
		expBuckets = append([]Bucket[float64]{it.At()}, expBuckets...)
	}
	it = h.NegativeReverseBucketIterator()
	for it.Next() {
		actBuckets = append(actBuckets, it.At())
	}
	require.NotEmpty(t, expBuckets)
	require.NotEmpty(t, actBuckets)
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
		{
			h: FloatHistogram{
				Count:         447,
				ZeroCount:     42,
				ZeroThreshold: 0.5, // Coinciding with bucket boundary.
				Sum:           1008.4,
				Schema:        0,
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
				Count:         447,
				ZeroCount:     42,
				ZeroThreshold: 0.6, // Within the bucket closest to zero.
				Sum:           1008.4,
				Schema:        0,
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
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var expBuckets, actBuckets []Bucket[float64]

			if c.includeNeg {
				it := c.h.NegativeReverseBucketIterator()
				for it.Next() {
					b := it.At()
					if c.includeZero && b.Upper > -c.h.ZeroThreshold {
						b.Upper = -c.h.ZeroThreshold
					}
					expBuckets = append(expBuckets, b)
				}
			}
			if c.includeZero {
				expBuckets = append(expBuckets, Bucket[float64]{
					Lower:          -c.h.ZeroThreshold,
					Upper:          c.h.ZeroThreshold,
					LowerInclusive: true,
					UpperInclusive: true,
					Count:          c.h.ZeroCount,
				})
			}
			if c.includePos {
				it := c.h.PositiveBucketIterator()
				for it.Next() {
					b := it.At()
					if c.includeZero && b.Lower < c.h.ZeroThreshold {
						b.Lower = c.h.ZeroThreshold
					}
					expBuckets = append(expBuckets, b)
				}
			}

			it := c.h.AllBucketIterator()
			for it.Next() {
				actBuckets = append(actBuckets, it.At())
			}

			require.Equal(t, expBuckets, actBuckets)
		})
	}
}

func TestAllReverseFloatBucketIterator(t *testing.T) {
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
		{
			h: FloatHistogram{
				Count:         447,
				ZeroCount:     42,
				ZeroThreshold: 0.5, // Coinciding with bucket boundary.
				Sum:           1008.4,
				Schema:        0,
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
				Count:         447,
				ZeroCount:     42,
				ZeroThreshold: 0.6, // Within the bucket closest to zero.
				Sum:           1008.4,
				Schema:        0,
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
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var expBuckets, actBuckets []Bucket[float64]

			if c.includePos {
				it := c.h.PositiveReverseBucketIterator()
				for it.Next() {
					b := it.At()
					if c.includeZero && b.Lower < c.h.ZeroThreshold {
						b.Lower = c.h.ZeroThreshold
					}
					expBuckets = append(expBuckets, b)
				}
			}
			if c.includeZero {
				expBuckets = append(expBuckets, Bucket[float64]{
					Lower:          -c.h.ZeroThreshold,
					Upper:          c.h.ZeroThreshold,
					LowerInclusive: true,
					UpperInclusive: true,
					Count:          c.h.ZeroCount,
				})
			}
			if c.includeNeg {
				it := c.h.NegativeBucketIterator()
				for it.Next() {
					b := it.At()
					if c.includeZero && b.Upper > -c.h.ZeroThreshold {
						b.Upper = -c.h.ZeroThreshold
					}
					expBuckets = append(expBuckets, b)
				}
			}

			it := c.h.AllReverseBucketIterator()
			for it.Next() {
				actBuckets = append(actBuckets, it.At())
			}

			require.Equal(t, expBuckets, actBuckets)
		})
	}
}

func TestFloatBucketIteratorTargetSchema(t *testing.T) {
	h := FloatHistogram{
		Count:  405,
		Sum:    1008.4,
		Schema: 1,
		PositiveSpans: []Span{
			{Offset: 0, Length: 4},
			{Offset: 1, Length: 3},
			{Offset: 2, Length: 3},
		},
		PositiveBuckets: []float64{100, 344, 123, 55, 3, 63, 2, 54, 235, 33},
		NegativeSpans: []Span{
			{Offset: 0, Length: 3},
			{Offset: 7, Length: 4},
			{Offset: 1, Length: 3},
		},
		NegativeBuckets: []float64{10, 34, 1230, 54, 67, 63, 2, 554, 235, 33},
	}
	expPositiveBuckets := []Bucket[float64]{
		{Lower: 0.25, Upper: 1, LowerInclusive: false, UpperInclusive: true, Count: 100, Index: 0},
		{Lower: 1, Upper: 4, LowerInclusive: false, UpperInclusive: true, Count: 522, Index: 1},
		{Lower: 4, Upper: 16, LowerInclusive: false, UpperInclusive: true, Count: 68, Index: 2},
		{Lower: 16, Upper: 64, LowerInclusive: false, UpperInclusive: true, Count: 322, Index: 3},
	}
	expNegativeBuckets := []Bucket[float64]{
		{Lower: -1, Upper: -0.25, LowerInclusive: true, UpperInclusive: false, Count: 10, Index: 0},
		{Lower: -4, Upper: -1, LowerInclusive: true, UpperInclusive: false, Count: 1264, Index: 1},
		{Lower: -64, Upper: -16, LowerInclusive: true, UpperInclusive: false, Count: 184, Index: 3},
		{Lower: -256, Upper: -64, LowerInclusive: true, UpperInclusive: false, Count: 791, Index: 4},
		{Lower: -1024, Upper: -256, LowerInclusive: true, UpperInclusive: false, Count: 33, Index: 5},
	}

	it := h.floatBucketIterator(true, 0, -1)
	for i, b := range expPositiveBuckets {
		require.True(t, it.Next(), "positive iterator exhausted too early")
		require.Equal(t, b, it.At(), "bucket %d", i)
	}
	require.False(t, it.Next(), "positive iterator not exhausted")

	it = h.floatBucketIterator(false, 0, -1)
	for i, b := range expNegativeBuckets {
		require.True(t, it.Next(), "negative iterator exhausted too early")
		require.Equal(t, b, it.At(), "bucket %d", i)
	}
	require.False(t, it.Next(), "negative iterator not exhausted")
}

func TestFloatCustomBucketsIterators(t *testing.T) {
	cases := []struct {
		h                  FloatHistogram
		expPositiveBuckets []Bucket[float64]
	}{
		{
			h: FloatHistogram{
				Count:  622,
				Sum:    10008.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 1},
					{Offset: 1, Length: 1},
				},
				PositiveBuckets: []float64{100, 344, 123, 55},
				CustomValues:    []float64{10, 25, 50, 100, 500},
			},
			expPositiveBuckets: []Bucket[float64]{
				{Lower: math.Inf(-1), Upper: 10, LowerInclusive: true, UpperInclusive: true, Count: 100, Index: 0},
				{Lower: 10, Upper: 25, LowerInclusive: false, UpperInclusive: true, Count: 344, Index: 1},
				{Lower: 50, Upper: 100, LowerInclusive: false, UpperInclusive: true, Count: 123, Index: 3},
				{Lower: 500, Upper: math.Inf(1), LowerInclusive: false, UpperInclusive: true, Count: 55, Index: 5},
			},
		},
		{
			h: FloatHistogram{
				Count:  622,
				Sum:    10008.4,
				Schema: CustomBucketsSchema,
				PositiveSpans: []Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 1},
					{Offset: 1, Length: 1},
				},
				PositiveBuckets: []float64{100, 344, 123, 55},
				CustomValues:    []float64{-10, -5, 0, 10, 25},
			},
			expPositiveBuckets: []Bucket[float64]{
				{Lower: math.Inf(-1), Upper: -10, LowerInclusive: true, UpperInclusive: true, Count: 100, Index: 0},
				{Lower: -10, Upper: -5, LowerInclusive: false, UpperInclusive: true, Count: 344, Index: 1},
				{Lower: 0, Upper: 10, LowerInclusive: false, UpperInclusive: true, Count: 123, Index: 3},
				{Lower: 25, Upper: math.Inf(1), LowerInclusive: false, UpperInclusive: true, Count: 55, Index: 5},
			},
		},
	}

	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			{
				it := c.h.AllBucketIterator()
				for i, b := range c.expPositiveBuckets {
					require.True(t, it.Next(), "all bucket iterator exhausted too early")
					require.Equal(t, b, it.At(), "bucket %d", i)
				}
				require.False(t, it.Next(), "all bucket iterator not exhausted")

				it = c.h.AllReverseBucketIterator()
				length := len(c.expPositiveBuckets)
				for j := range length {
					i := length - j - 1
					b := c.expPositiveBuckets[i]
					require.True(t, it.Next(), "all reverse bucket iterator exhausted too early")
					require.Equal(t, b, it.At(), "bucket %d", i)
				}
				require.False(t, it.Next(), "all reverse bucket iterator not exhausted")

				it = c.h.PositiveBucketIterator()
				for i, b := range c.expPositiveBuckets {
					require.True(t, it.Next(), "positive bucket iterator exhausted too early")
					require.Equal(t, b, it.At(), "bucket %d", i)
				}
				require.False(t, it.Next(), "positive bucket iterator not exhausted")

				it = c.h.PositiveReverseBucketIterator()
				for j := range length {
					i := length - j - 1
					b := c.expPositiveBuckets[i]
					require.True(t, it.Next(), "positive reverse bucket iterator exhausted too early")
					require.Equal(t, b, it.At(), "bucket %d", i)
				}
				require.False(t, it.Next(), "positive reverse bucket iterator not exhausted")

				it = c.h.NegativeBucketIterator()
				require.False(t, it.Next(), "negative bucket iterator not exhausted")

				it = c.h.NegativeReverseBucketIterator()
				require.False(t, it.Next(), "negative reverse bucket iterator not exhausted")
			}
			{
				it := c.h.floatBucketIterator(true, 0, CustomBucketsSchema)
				for i, b := range c.expPositiveBuckets {
					require.True(t, it.Next(), "positive iterator exhausted too early")
					require.Equal(t, b, it.At(), "bucket %d", i)
				}
				require.False(t, it.Next(), "positive iterator not exhausted")

				it = c.h.floatBucketIterator(false, 0, CustomBucketsSchema)
				require.False(t, it.Next(), "negative iterator not exhausted")
			}
		})
	}
}

// TestFloatHistogramEquals tests FloatHistogram with float-specific cases that
// cannot be covered by TestHistogramEquals.
func TestFloatHistogramEquals(t *testing.T) {
	h1 := FloatHistogram{
		Schema:          3,
		Count:           2.2,
		Sum:             9.7,
		ZeroThreshold:   0.1,
		ZeroCount:       1.1,
		PositiveBuckets: []float64{3},
		NegativeBuckets: []float64{4},
	}

	equals := func(h1, h2 FloatHistogram) {
		require.True(t, h1.Equals(&h2))
		require.True(t, h2.Equals(&h1))
	}
	notEquals := func(h1, h2 FloatHistogram) {
		require.False(t, h1.Equals(&h2))
		require.False(t, h2.Equals(&h1))
	}

	h2 := h1.Copy()
	equals(h1, *h2)

	// Count is NaN (but not a StaleNaN).
	hCountNaN := h1.Copy()
	hCountNaN.Count = math.NaN()
	notEquals(h1, *hCountNaN)
	equals(*hCountNaN, *hCountNaN)

	// ZeroCount is NaN (but not a StaleNaN).
	hZeroCountNaN := h1.Copy()
	hZeroCountNaN.ZeroCount = math.NaN()
	notEquals(h1, *hZeroCountNaN)
	equals(*hZeroCountNaN, *hZeroCountNaN)

	// Positive bucket value is NaN.
	hPosBucketNaN := h1.Copy()
	hPosBucketNaN.PositiveBuckets[0] = math.NaN()
	notEquals(h1, *hPosBucketNaN)
	equals(*hPosBucketNaN, *hPosBucketNaN)

	// Negative bucket value is NaN.
	hNegBucketNaN := h1.Copy()
	hNegBucketNaN.NegativeBuckets[0] = math.NaN()
	notEquals(h1, *hNegBucketNaN)
	equals(*hNegBucketNaN, *hNegBucketNaN)

	// Custom bounds are defined for exponential schema.
	hCustom := h1.Copy()
	hCustom.CustomValues = []float64{1, 2, 3}
	equals(h1, *hCustom)

	cbh1 := FloatHistogram{
		Schema:          CustomBucketsSchema,
		Count:           2.2,
		Sum:             9.7,
		PositiveSpans:   []Span{{0, 1}},
		PositiveBuckets: []float64{3},
		CustomValues:    []float64{1, 2, 3},
	}

	require.NoError(t, cbh1.Validate())

	cbh2 := cbh1.Copy()
	equals(cbh1, *cbh2)

	// Has different custom bounds for custom buckets schema.
	cbh2 = cbh1.Copy()
	cbh2.CustomValues = []float64{1, 2, 3, 4}
	notEquals(cbh1, *cbh2)

	// Has non-empty negative spans and buckets for custom buckets schema.
	cbh2 = cbh1.Copy()
	cbh2.NegativeSpans = []Span{{Offset: 0, Length: 1}}
	cbh2.NegativeBuckets = []float64{1}
	notEquals(cbh1, *cbh2)

	// Has non-zero zero count and threshold for custom buckets schema.
	cbh2 = cbh1.Copy()
	cbh2.ZeroThreshold = 0.1
	cbh2.ZeroCount = 10
	notEquals(cbh1, *cbh2)
}

func TestFloatHistogramSize(t *testing.T) {
	cases := []struct {
		name     string
		fh       *FloatHistogram
		expected int
	}{
		{
			"without spans and buckets",
			&FloatHistogram{ // 8 bytes.
				CounterResetHint: 0,           // 1 byte.
				Schema:           1,           // 4 bytes.
				ZeroThreshold:    0.01,        // 8 bytes.
				ZeroCount:        5.5,         // 8 bytes.
				Count:            3493.3,      // 8 bytes.
				Sum:              2349209.324, // 8 bytes.
				PositiveSpans:    nil,         // 24 bytes.
				PositiveBuckets:  nil,         // 24 bytes.
				NegativeSpans:    nil,         // 24 bytes.
				NegativeBuckets:  nil,         // 24 bytes.
				CustomValues:     nil,         // 24 bytes.
			},
			8 + 4 + 4 + 8 + 8 + 8 + 8 + 24 + 24 + 24 + 24 + 24,
		},
		{
			"complete struct",
			&FloatHistogram{ // 8 bytes.
				CounterResetHint: 0,           // 1 byte.
				Schema:           1,           // 4 bytes.
				ZeroThreshold:    0.01,        // 8 bytes.
				ZeroCount:        5.5,         // 8 bytes.
				Count:            3493.3,      // 8 bytes.
				Sum:              2349209.324, // 8 bytes.
				PositiveSpans: []Span{ // 24 bytes.
					{-2, 1}, // 2 * 4 bytes.
					{2, 3},  //  2 * 4 bytes.
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1}, // 24 bytes + 4 * 8 bytes.
				NegativeSpans: []Span{ // 24 bytes.
					{3, 2}, // 2 * 4 bytes.
					{3, 2}, //  2 * 4 bytes.
				},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000}, // 24 bytes + 4 * 8 bytes.
				CustomValues:    nil,                              // 24 bytes.
			},
			8 + 4 + 4 + 8 + 8 + 8 + 8 + (24 + 2*4 + 2*4) + (24 + 2*4 + 2*4) + (24 + 4*8) + (24 + 4*8) + 24,
		},
		{
			"complete struct with custom buckets",
			&FloatHistogram{ // 8 bytes.
				CounterResetHint: 0,                   // 1 byte.
				Schema:           CustomBucketsSchema, // 4 bytes.
				ZeroThreshold:    0,                   // 8 bytes.
				ZeroCount:        0,                   // 8 bytes.
				Count:            3493.3,              // 8 bytes.
				Sum:              2349209.324,         // 8 bytes.
				PositiveSpans: []Span{ // 24 bytes.
					{0, 1}, // 2 * 4 bytes.
					{2, 3}, //  2 * 4 bytes.
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1}, // 24 bytes + 4 * 8 bytes.
				NegativeSpans:   nil,                         // 24 bytes.
				NegativeBuckets: nil,                         // 24 bytes.
				CustomValues:    []float64{1, 2, 3},          // 24 bytes + 3 * 8 bytes.
			},
			8 + 4 + 4 + 8 + 8 + 8 + 8 + (24 + 2*4 + 2*4) + (24 + 4*8) + 24 + 24 + (24 + 3*8),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.fh.Size())
		})
	}
}

func TestFloatHistogramString(t *testing.T) {
	cases := []struct {
		name     string
		fh       *FloatHistogram
		expected string
	}{
		{
			"exponential histogram",
			&FloatHistogram{
				Schema:        1,
				ZeroThreshold: 0.01,
				ZeroCount:     5.5,
				Count:         3493.3,
				Sum:           2349209.324,
				PositiveSpans: []Span{
					{-2, 1},
					{2, 3},
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans: []Span{
					{3, 2},
					{3, 2},
				},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
			`{count:3493.3, sum:2.349209324e+06, [-22.62741699796952,-16):1000, [-16,-11.31370849898476):123400, [-4,-2.82842712474619):3, [-2.82842712474619,-2):3.1, [-0.01,0.01]:5.5, (0.35355339059327373,0.5]:1, (1,1.414213562373095]:3.3, (1.414213562373095,2]:4.2, (2,2.82842712474619]:0.1}`,
		},
		{
			"custom buckets histogram",
			&FloatHistogram{
				Schema: CustomBucketsSchema,
				Count:  3493.3,
				Sum:    2349209.324,
				PositiveSpans: []Span{
					{0, 1},
					{2, 4},
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1, 5},
				CustomValues:    []float64{1, 2, 5, 10, 15, 20},
			},
			`{count:3493.3, sum:2.349209324e+06, [-Inf,1]:1, (5,10]:3.3, (10,15]:4.2, (15,20]:0.1, (20,+Inf]:5}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, c.fh.Validate())
			require.Equal(t, c.expected, c.fh.String())
		})
	}
}

func TestFloatHistogramValidateNegativeHistogram(t *testing.T) {
	cases := []struct {
		name string
		fh   *FloatHistogram
	}{
		{
			"positive bucket with negative population",
			&FloatHistogram{
				Schema:        1,
				ZeroThreshold: 0.01,
				ZeroCount:     5.5,
				Count:         3493.3,
				Sum:           2349209.324,
				PositiveSpans: []Span{
					{-2, 1},
					{2, 3},
				},
				PositiveBuckets: []float64{1, 3.3, -4.2, 0.1},
				NegativeSpans: []Span{
					{3, 2},
					{3, 2},
				},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
		},
		{
			"negative bucket with negative population",
			&FloatHistogram{
				Schema:        1,
				ZeroThreshold: 0.01,
				ZeroCount:     5.5,
				Count:         3493.3,
				Sum:           2349209.324,
				PositiveSpans: []Span{
					{-2, 1},
					{2, 3},
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans: []Span{
					{3, 2},
					{3, 2},
				},
				NegativeBuckets: []float64{3.1, -3, 1.234e5, 1000},
			},
		},
		{
			"negative count",
			&FloatHistogram{
				Schema:        1,
				ZeroThreshold: 0.01,
				ZeroCount:     5.5,
				Count:         -3493.3,
				Sum:           2349209.324,
				PositiveSpans: []Span{
					{-2, 1},
					{2, 3},
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans: []Span{
					{3, 2},
					{3, 2},
				},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
		},
		{
			"zero bucket with negative population",
			&FloatHistogram{
				Schema:        1,
				ZeroThreshold: 0.01,
				ZeroCount:     -5.5,
				Count:         3493.3,
				Sum:           2349209.324,
				PositiveSpans: []Span{
					{-2, 1},
					{2, 3},
				},
				PositiveBuckets: []float64{1, 3.3, 4.2, 0.1},
				NegativeSpans: []Span{
					{3, 2},
					{3, 2},
				},
				NegativeBuckets: []float64{3.1, 3, 1.234e5, 1000},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Error(t, c.fh.Validate())
		})
	}
}

func BenchmarkFloatHistogramAllBucketIterator(b *testing.B) {
	rng := rand.New(rand.NewSource(0))

	fh := createRandomFloatHistogram(rng, 50)

	b.ReportAllocs() // the current implementation reports 1 alloc
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for it := fh.AllBucketIterator(); it.Next(); {
		}
	}
}

func BenchmarkFloatHistogramDetectReset(b *testing.B) {
	rng := rand.New(rand.NewSource(0))

	fh := createRandomFloatHistogram(rng, 50)

	b.ReportAllocs() // the current implementation reports 0 allocs
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		// Detect against the itself (no resets is the worst case input).
		fh.DetectReset(fh)
	}
}

func createRandomFloatHistogram(rng *rand.Rand, spanNum int32) *FloatHistogram {
	f := &FloatHistogram{}
	f.PositiveSpans, f.PositiveBuckets = createRandomSpans(rng, spanNum)
	f.NegativeSpans, f.NegativeBuckets = createRandomSpans(rng, spanNum)
	return f
}

func createRandomSpans(rng *rand.Rand, spanNum int32) ([]Span, []float64) {
	Spans := make([]Span, spanNum)
	Buckets := make([]float64, 0)
	for i := 0; i < int(spanNum); i++ {
		Spans[i].Offset = rng.Int31n(spanNum) + 1
		Spans[i].Length = uint32(rng.Int31n(spanNum) + 1)
		for j := 0; j < int(Spans[i].Length); j++ {
			Buckets = append(Buckets, float64(rng.Int31n(spanNum)+1))
		}
	}
	return Spans, Buckets
}

func TestFloatHistogramReduceResolution(t *testing.T) {
	tcs := map[string]struct {
		origin *FloatHistogram
		target *FloatHistogram
	}{
		"valid float histogram": {
			origin: &FloatHistogram{
				Schema: 0,
				PositiveSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 3, Length: 2},
				},
				PositiveBuckets: []float64{1, 3, 1, 2, 1, 1},
				NegativeSpans: []Span{
					{Offset: 0, Length: 4},
					{Offset: 0, Length: 0},
					{Offset: 3, Length: 2},
				},
				NegativeBuckets: []float64{1, 3, 1, 2, 1, 1},
			},
			target: &FloatHistogram{
				Schema: -1,
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
					{Offset: 1, Length: 1},
				},
				PositiveBuckets: []float64{1, 4, 2, 2},
				NegativeSpans: []Span{
					{Offset: 0, Length: 3},
					{Offset: 1, Length: 1},
				},
				NegativeBuckets: []float64{1, 4, 2, 2},
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
