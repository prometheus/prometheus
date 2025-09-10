// Copyright 2024 The Prometheus Authors
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

package convertnhcb

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
)

func TestNHCBConvert(t *testing.T) {
	tests := map[string]struct {
		setup       func() *TempHistogram
		expectedErr error
		expectedH   *histogram.Histogram
		expectedFH  *histogram.FloatHistogram
	}{
		"empty": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				return &h
			},
			expectedH: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				PositiveSpans:   []histogram.Span{},
				PositiveBuckets: []int64{},
			},
		},
		"sum only": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetSum(1000.25)
				return &h
			},
			expectedH: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Sum:             1000.25,
				PositiveSpans:   []histogram.Span{},
				PositiveBuckets: []int64{},
			},
		},
		"single integer bucket": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetSum(1000.25)
				h.SetBucketCount(0.5, 1000)
				return &h
			},
			expectedH: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           1000,
				Sum:             1000.25,
				PositiveSpans:   []histogram.Span{{Length: 1}},
				PositiveBuckets: []int64{1000},
				CustomValues:    []float64{0.5},
			},
		},
		"single float bucket": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetSum(1000.25)
				h.SetBucketCount(0.5, 1337.42)
				return &h
			},
			expectedFH: &histogram.FloatHistogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           1337.42,
				Sum:             1000.25,
				PositiveSpans:   []histogram.Span{{Length: 1}},
				PositiveBuckets: []float64{1337.42},
				CustomValues:    []float64{0.5},
			},
		},
		"happy case integer bucket": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetCount(1000)
				h.SetSum(1000.25)
				h.SetBucketCount(0.5, 50)
				h.SetBucketCount(1.0, 950)
				h.SetBucketCount(math.Inf(1), 1000)
				return &h
			},
			expectedH: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           1000,
				Sum:             1000.25,
				PositiveSpans:   []histogram.Span{{Length: 3}},
				PositiveBuckets: []int64{50, 850, -850},
				CustomValues:    []float64{0.5, 1.0},
			},
		},
		"happy case float bucket": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetCount(1000)
				h.SetSum(1000.25)
				h.SetBucketCount(0.5, 50)
				h.SetBucketCount(1.0, 950.5)
				h.SetBucketCount(math.Inf(1), 1000)
				return &h
			},
			expectedFH: &histogram.FloatHistogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           1000,
				Sum:             1000.25,
				PositiveSpans:   []histogram.Span{{Length: 3}},
				PositiveBuckets: []float64{50, 900.5, 49.5},
				CustomValues:    []float64{0.5, 1.0},
			},
		},
		"non cumulative bucket": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetCount(1000)
				h.SetSum(1000.25)
				h.SetBucketCount(0.5, 50)
				h.SetBucketCount(1.0, 950)
				h.SetBucketCount(math.Inf(1), 900)
				return &h
			},
			expectedErr: errCountNotCumulative,
		},
		"negative count": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetCount(-1000)
				h.SetSum(1000.25)
				h.SetBucketCount(0.5, 50)
				h.SetBucketCount(1.0, 950)
				h.SetBucketCount(math.Inf(1), 900)
				return &h
			},
			expectedErr: errNegativeCount,
		},
		"mixed order": {
			setup: func() *TempHistogram {
				h := NewTempHistogram()
				h.SetBucketCount(0.5, 50)
				h.SetBucketCount(math.Inf(1), 1000)
				h.SetBucketCount(1.0, 950)
				h.SetCount(1000)
				h.SetSum(1000.25)
				return &h
			},
			expectedH: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           1000,
				Sum:             1000.25,
				PositiveSpans:   []histogram.Span{{Length: 3}},
				PositiveBuckets: []int64{50, 850, -850},
				CustomValues:    []float64{0.5, 1.0},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			th := test.setup()
			h, fh, err := th.Convert()
			if test.expectedErr != nil {
				require.ErrorIs(t, err, test.expectedErr)
				return
			}
			require.Equal(t, test.expectedH, h)
			if h != nil {
				require.NoError(t, h.Validate())
			}
			require.Equal(t, test.expectedFH, fh)
			if fh != nil {
				require.NoError(t, fh.Validate())
			}
		})
	}
}
