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
	"errors"
	"log"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

type ExpectedBucket struct {
	le  string
	val float64
}

type ExpectedClassicHistogram struct {
	buckets []ExpectedBucket
	count   float64
	sum     float64
}

func TestConvertNHCBToClassicHistogram(t *testing.T) {
	tests := []struct {
		name      string
		nhcb      any
		labels    labels.Labels
		expectErr bool
		expected  ExpectedClassicHistogram
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
			expected: ExpectedClassicHistogram{
				buckets: []ExpectedBucket{
					{le: "1", val: 10},
					{le: "2", val: 30},
					{le: "3", val: 60},
					{le: "+Inf", val: 60},
				},
				count: 60,
				sum:   100,
			},
		},
		{
			name: "Valid FloatHistogram",
			nhcb: &FloatHistogram{
				CustomValues:    []float64{1, 2, 3},
				PositiveBuckets: []float64{20.0, 40.0, 60.0},
				Count:           60.0,
				Sum:             100.0,
			},
			labels: labels.FromStrings("__name__", "test_metric"),
			expected: ExpectedClassicHistogram{
				buckets: []ExpectedBucket{
					{le: "1", val: 20},
					{le: "2", val: 40},
					{le: "3", val: 60},
					{le: "+Inf", val: 60},
				},
				count: 60,
				sum:   100,
			},
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
			expected: ExpectedClassicHistogram{
				buckets: []ExpectedBucket{
					{le: "+Inf", val: 0},
				},
				count: 0,
				sum:   0,
			},
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
			expected: ExpectedClassicHistogram{
				buckets: []ExpectedBucket{
					{le: "1", val: 0},
					{le: "2", val: 10},
					{le: "3", val: 10},
					{le: "+Inf", val: 10},
				},
				count: 10,
				sum:   50,
			},
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
			expected: ExpectedClassicHistogram{
				buckets: []ExpectedBucket{
					{le: "1", val: 10},
					{le: "+Inf", val: 10},
				},
				count: 10,
				sum:   20,
			},
		},
		{
			name: "Duplicate le label",
			nhcb: &Histogram{
				CustomValues:    []float64{1},
				PositiveBuckets: []int64{10},
				Count:           10,
				Sum:             20.0,
			},
			labels: labels.FromStrings("__name__", "test_metric", "le", "1.5"),
			expected: ExpectedClassicHistogram{
				buckets: []ExpectedBucket{
					{le: "1", val: 10},
					{le: "+Inf", val: 10},
				},
				count: 10,
				sum:   20,
			},
		},
	}

	labelBuilder := labels.NewBuilder(labels.EmptyLabels())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ExpectedClassicHistogram
			err := ConvertNHCBToClassicHistogram(tt.nhcb, tt.labels, labelBuilder, func(lbls labels.Labels, val float64) error {
				log.Println(tt.labels)
				log.Println("----", lbls)
				switch lbls.Get("__name__") {
				case tt.labels.Get("__name__") + ConvertedLabelSuffix + "_bucket":
					got.buckets = append(got.buckets, ExpectedBucket{
						le:  lbls.Get("le"),
						val: val,
					})
				case tt.labels.Get("__name__") + ConvertedLabelSuffix + "_count":
					got.count = val
				case tt.labels.Get("__name__") + ConvertedLabelSuffix + "_sum":
					got.sum = val
				default:
					return errors.New("unexpected metric name")
				}
				return nil
			})
			require.Equal(t, tt.expectErr, err != nil, "unexpected error: %v", err)
			if !tt.expectErr {
				log.Println(got)

				require.Len(t, got.buckets, len(tt.expected.buckets))
				for i, expBucket := range tt.expected.buckets {
					require.Equal(t, expBucket, got.buckets[i])
				}
				require.Equal(t, tt.expected.count, got.count)
				require.Equal(t, tt.expected.sum, got.sum)
			}
		})
	}
}
