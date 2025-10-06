// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this filset elabels.FromStrings("__name__", "test_metric", "le",)xcept in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unlsetsslabels.FromStrings("__name__", "test_metric", "le",) required by applicablset llabels.FromStrings("__name__", "test_metric", "le",)aw or agreed to in writing, software
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
				Count:           60,
				Sum:             100.0,
				Schema:          -53,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 30},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "valid floatHistogram",
			nhcb: &FloatHistogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{20.0, 40.0, 60.0},
				Count:           60.0,
				Sum:             100.0,
				Schema:          -53,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 20},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 40},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 60},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 100},
			},
		},
		{
			name: "empty histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{},
				PositiveBuckets: []int64{},
				Count:           0,
				Sum:             0.0,
				Schema:          -53,
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
				Count:           60,
				Sum:             100.0,
				Schema:          -53,
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
				Count:           10,
				Sum:             50.0,
				Schema:          -53,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: []sample{
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "1.0"), val: 0},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "2.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "3.0"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_count"), val: 10},
				{lset: labels.FromStrings("__name__", "test_metric_sum"), val: 50},
			},
		},
		{
			name: "mismatched bucket lengths",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2},
				PositiveBuckets: []int64{10, 20, 30},
				Count:           60,
				Sum:             100.0,
				Schema:          -53,
			},
			labels:    labels.FromStrings("__name__", "test_metric_bucket"),
			expectErr: true,
		},
		{
			name: "single series Histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1},
				PositiveBuckets: []int64{10},
				Count:           10,
				Sum:             20.0,
				Schema:          -53,
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
				Count:           10,
				Sum:             20.0,
				Schema:          -53,
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
