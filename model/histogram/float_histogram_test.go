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

func TestFloatHistogramScale(t *testing.T) {
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.in.Scale(c.scale))
			// Has it also happened in-place?
			require.Equal(t, c.expected, c.in)
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
		// TODO(beorn7): Add test cases.
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.in.Compact(c.maxEmptyBuckets))
			// Has it also happened in-place?
			require.Equal(t, c.expected, c.in)
		})
	}
}

func TestFloatHistogramAdd(t *testing.T) {
	cases := []struct {
		name               string
		in1, in2, expected *FloatHistogram
	}{
		{
			"same bucket layout",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			&FloatHistogram{
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
			"same bucket layout, defined differently",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {1, 1}, {0, 2}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-2, 2}, {1, 2}, {0, 1}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				NegativeSpans:   []Span{{3, 7}},
				NegativeBuckets: []float64{1, 1, 0, 0, 0, 4, 4},
			},
			&FloatHistogram{
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
			"non-overlapping spans",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{0, 2}, {3, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6},
				NegativeSpans:   []Span{{-9, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
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
			"non-overlapping inverted order",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{0, 2}, {3, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6},
				NegativeSpans:   []Span{{-9, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       19,
				Count:           51,
				Sum:             3.579,
				PositiveSpans:   []Span{{-2, 2}, {0, 5}, {0, 3}},
				PositiveBuckets: []float64{1, 0, 5, 4, 3, 4, 7, 2, 3, 6},
				NegativeSpans:   []Span{{-9, 2}, {3, 2}, {5, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4, 3, 1, 5, 6},
			},
		},
		{
			"overlapping spans",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			&FloatHistogram{
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
			"overlapping spans inverted order",
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             1.234,
				PositiveSpans:   []Span{{-1, 4}, {0, 3}},
				PositiveBuckets: []float64{5, 4, 2, 3, 6, 2, 5},
				NegativeSpans:   []Span{{4, 2}, {1, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       11,
				Count:           30,
				Sum:             2.345,
				PositiveSpans:   []Span{{-2, 2}, {2, 3}},
				PositiveBuckets: []float64{1, 0, 3, 4, 7},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{3, 1, 5, 6},
			},
			&FloatHistogram{
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.in1.Add(c.in2))
			// Has it also happened in-place?
			require.Equal(t, c.expected, c.in1)
		})
	}
}

func TestFloatHistogramSub(t *testing.T) {
	// This has fewer test cases than TestFloatHistogramAdd because Add and
	// Sub share most of the trickier code.
	cases := []struct {
		name               string
		in1, in2, expected *FloatHistogram
	}{
		{
			"same bucket layout",
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
			&FloatHistogram{
				ZeroThreshold:   0.01,
				ZeroCount:       8,
				Count:           21,
				Sum:             12,
				PositiveSpans:   []Span{{-2, 2}, {1, 3}},
				PositiveBuckets: []float64{0, 0, 2, 3, 6},
				NegativeSpans:   []Span{{3, 2}, {3, 2}},
				NegativeBuckets: []float64{1, 1, 4, 4},
			},
			&FloatHistogram{
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, c.in1.Sub(c.in2))
			// Has it also happened in-place?
			require.Equal(t, c.expected, c.in1)
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
