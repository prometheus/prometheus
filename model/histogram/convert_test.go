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

package histogram

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

type sample struct {
	lset labels.Labels
	val  float64
}

func TestConvertNHCBToClassicHistogram(t *testing.T) {
	tests := []struct {
		name      string
		nhcb      any
		labels    labels.Labels
		expectErr bool
		expected  []sample
	}{
		{
			name: "valid histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{10, 20, 30},
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
				},
				Count:  100,
				Sum:    100.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 40},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "valid floatHistogram",
			nhcb: &FloatHistogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{20.0, 40.0, 60.0}, // 20 -> 60 ->120
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
				},
				Count:  120.0,
				Sum:    100.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 120},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 120},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 120},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "empty histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{},
				PositiveBuckets: []int64{},
				PositiveSpans:   []Span{},
				Count:           0,
				Sum:             0.0,
				Schema:          CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 0},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 0},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 0},
			},
		},
		{
			name: "missing __name__ label",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{10, 20, 30},
				Count:           100,
				Sum:             100.0,
				Schema:          CustomBucketsSchema,
			},
			labels:    labels.FromStrings("job", "test_job"),
			expectErr: true,
		},
		{
			name:      "unsupported histogram type",
			nhcb:      nil,
			labels:    labels.FromStrings("__name__", "test_metric"),
			expectErr: true,
		},
		{
			name: "histogram with zero bucket counts",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{0, 10, 0},
				PositiveSpans: []Span{
					{Offset: 0, Length: 3},
				},
				Count:  20,
				Sum:    50.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 0},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 50},
			},
		},
		{
			name: "extra bucket counts than custom values",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2},
				PositiveBuckets: []int64{10, 20, 30},
				PositiveSpans:   []Span{{Offset: 0, Length: 3}},
				Count:           100,
				Sum:             100.0,
				Schema:          CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 40},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 100},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "mismatched bucket lengths with less filled bucket count",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2},
				PositiveBuckets: []int64{10},
				PositiveSpans:   []Span{{Offset: 0, Length: 2}},
				Count:           100,
				Sum:             100.0,
				Schema:          CustomBucketsSchema,
			},
			labels:    labels.FromStrings("__name__", "test_metric_bucket"),
			expectErr: true,
		},
		{
			name: "single series Histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1},
				PositiveBuckets: []int64{10},
				PositiveSpans: []Span{
					{Offset: 0, Length: 1},
				},
				Count:  10,
				Sum:    20.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 20},
			},
		},
		{
			name: "multiset label histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1},
				PositiveBuckets: []int64{10},
				PositiveSpans: []Span{
					{Offset: 0, Length: 1},
				},
				Count:  10,
				Sum:    20.0,
				Schema: CustomBucketsSchema,
			},
			labels: labels.FromStrings("__name__", "test_metric", "job", "test_job", "instance", "localhost:9090"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "job", "test_job", "instance", "localhost:9090", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "job", "test_job", "instance", "localhost:9090", "le", "+Inf"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_count", "job", "test_job", "instance", "localhost:9090"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_sum", "job", "test_job", "instance", "localhost:9090"), val: 20},
			},
		},
		{
			name: "exponential histogram",
			nhcb: &FloatHistogram{
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
			labels:    labels.FromStrings("__name__", "test_metric_bucket"),
			expectErr: true,
		},
		{
			name: "sparse histogram",
			nhcb: &Histogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
				PositiveSpans: []Span{
					{0, 2},
					{4, 1},
					{1, 2},
				},
				PositiveBuckets: []int64{1, 2, 3, 4, 5}, // 1 -> 3 -> 3 -> 3 -> 3 -> 3 -> 6 ->6 ->10 -> 15
				Count:           35,                     // 1 -> 4 -> 7 -> 10 -> 13 -> 16 -> 22 -> 28 -> 38 -> 53
				Sum:             123,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 1},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 4},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 7},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "4.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "5.0"), val: 13},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "6.0"), val: 16},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "7.0"), val: 22},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "8.0"), val: 28},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "9.0"), val: 38},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "10.0"), val: 53},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 53},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 35},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 123},
			},
		},
		{
			name: "sparse float histogram",
			nhcb: &FloatHistogram{
				Schema:       CustomBucketsSchema,
				CustomValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
				PositiveSpans: []Span{
					{0, 2},
					{4, 1},
					{1, 2},
				},
				PositiveBuckets: []float64{1, 2, 3, 4, 5}, // 1 -> 2 -> 0 -> 0 -> 0 -> 0 -> 3 -> 0 -> 4 -> 5
				Count:           15,                       // 1 -> 3 -> 3 -> 3 -> 3 -> 3 -> 6 -> 6 -> 10 -> 15
				Sum:             123,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 1},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "4.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "5.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "6.0"), val: 3},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "7.0"), val: 6},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "8.0"), val: 6},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "9.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "10.0"), val: 15},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 15},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 15},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 123},
			},
		},
	}

	labelBuilder := labels.NewBuilder(labels.EmptyLabels())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var emittedSamples []sample
			err := ConvertNHCBToClassic(tt.nhcb, tt.labels, labelBuilder, func(lbls labels.Labels, val float64) error {
				emittedSamples = append(emittedSamples, sample{lset: lbls, val: val})
				return nil
			})
			require.Equal(t, tt.expectErr, err != nil, "unexpected error: %v", err)
			if !tt.expectErr {
				require.Len(t, emittedSamples, len(tt.expected))
				for i, expSample := range tt.expected {
					require.True(t, labels.Equal(expSample.lset, emittedSamples[i].lset), "labels mismatch at index %d: expected %v, got %v", i, expSample.lset, emittedSamples[i].lset)
					require.Equal(t, expSample.val, emittedSamples[i].val, "value mismatch at index %d", i)
				}
			}
		})
	}
}
