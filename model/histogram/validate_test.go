// Copyright 2023 The Prometheus Authors
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
	"testing"

	"github.com/stretchr/testify/require"
)

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
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			if err := ValidateHistogram(tc.h); tc.errMsg != "" {
				require.EqualError(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
			if tc.skipFloat {
				return
			}
			if err := ValidateFloatHistogram(tc.h.ToFloat()); tc.errMsg != "" {
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
		require.NoError(b, ValidateHistogram(h))
	}
}
