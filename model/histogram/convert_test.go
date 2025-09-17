// Copyright 2025 The Prometheus Authors
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

	"github.com/prometheus/prometheus/model/labels"
)

func TestConvertNHCBToClassicHistogram(t *testing.T) {
	tests := []struct {
		name           string
		nhcb           interface{}
		labels         labels.Labels
		expectErr      bool
		expectedLabels []labels.Labels
		expectedValues []float64
	}{
		{
			name: "Valid Histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{10, 20, 30},
				Count:           60,
				Sum:             100.0,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expectedLabels: []labels.Labels{
				labels.FromStrings("__name__", "test_metric_bucket", "le", "1"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "2"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "3"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"),
				labels.FromStrings("__name__", "test_metric_count"),
				labels.FromStrings("__name__", "test_metric_sum"),
			},
			expectedValues: []float64{10, 30, 60, 60, 60, 100},
		},
		{
			name: "Valid FloatHistogram",
			nhcb: &FloatHistogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{20.0, 40.0, 60.0},
				Count:           120.0,
				Sum:             100.0,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expectedLabels: []labels.Labels{
				labels.FromStrings("__name__", "test_metric_bucket", "le", "1"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "2"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "3"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"),
				labels.FromStrings("__name__", "test_metric_count"),
				labels.FromStrings("__name__", "test_metric_sum"),
			},
			expectedValues: []float64{20, 60, 120, 120, 120, 100},
		},
		{
			name: "Empty Histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{},
				PositiveBuckets: []int64{},
				Count:           0,
				Sum:             0.0,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expectedLabels: []labels.Labels{
				labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"),
				labels.FromStrings("__name__", "test_metric_count"),
				labels.FromStrings("__name__", "test_metric_sum"),
			},
			expectedValues: []float64{0, 0, 0},
		},
		{
			name: "Missing __name__ label",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{10, 20, 30},
				Count:           60,
				Sum:             100.0,
			},
			labels:    labels.FromStrings("job", "test_job"),
			expectErr: true,
		},
		{
			name:      "Unsupported histogram type",
			nhcb:      nil,
			labels:    labels.FromStrings("__name__", "test_metric"),
			expectErr: true,
		},
		{
			name: "Histogram with zero bucket counts",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []int64{0, 10, 0},
				Count:           10,
				Sum:             50.0,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expectedLabels: []labels.Labels{
				labels.FromStrings("__name__", "test_metric_bucket", "le", "1"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "2"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "3"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"),
				labels.FromStrings("__name__", "test_metric_count"),
				labels.FromStrings("__name__", "test_metric_sum"),
			},
			expectedValues: []float64{0, 10, 10, 10, 10, 50},
		},
		{
			name: "Mismatched bucket lengths",
			nhcb: &Histogram{
				CustomValues:    []float64{1, 2},
				PositiveBuckets: []int64{10, 20, 30},
				Count:           60,
				Sum:             100.0,
			},
			labels:    labels.FromStrings("__name__", "test_metric"),
			expectErr: true,
		},
		{
			name: "single series Histogram",
			nhcb: &Histogram{
				CustomValues:    []float64{1},
				PositiveBuckets: []int64{10},
				Count:           10,
				Sum:             20.0,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expectedLabels: []labels.Labels{
				labels.FromStrings("__name__", "test_metric_bucket", "le", "1"),
				labels.FromStrings("__name__", "test_metric_bucket", "le", "+Inf"),
				labels.FromStrings("__name__", "test_metric_count"),
				labels.FromStrings("__name__", "test_metric_sum"),
			},
			expectedValues: []float64{10, 10, 10, 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var actualLabels []labels.Labels
			var actualValues []float64

			err := ConvertNHCBToClassicHistogram(tt.nhcb, tt.labels, &labels.Builder{}, func(lbls labels.Labels, value float64) error {
				actualLabels = append(actualLabels, lbls)
				actualValues = append(actualValues, value)
				return nil
			})

			if (err != nil) != tt.expectErr {
				t.Errorf("ConvertNHCBToClassicHistogram() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if !tt.expectErr {
				if len(actualLabels) != len(tt.expectedLabels) {
					t.Errorf("Expected %d emissions, got %d", len(tt.expectedLabels), len(actualLabels))
					return
				}

				for i, expectedLabel := range tt.expectedLabels {
					if !labels.Equal(actualLabels[i], expectedLabel) {
						t.Errorf("Expected label[%d] = %v, got %v", i, expectedLabel, actualLabels[i])
					}
					if actualValues[i] != tt.expectedValues[i] {
						t.Errorf("Expected value[%d] = %f, got %f", i, tt.expectedValues[i], actualValues[i])
					}
				}
			}
		})
	}
}
