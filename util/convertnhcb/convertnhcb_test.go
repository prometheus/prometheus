// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package convertnhcb_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

func TestProcessUpperBoundsAndCreateBaseHistogram(t *testing.T) {
	tests := []struct {
		name        string
		upperBounds []float64
		needsDedup  bool
		expectedUB  []float64
		expectedHB  *histogram.Histogram
	}{
		{
			name:        "No deduplication needed",
			upperBounds: []float64{1, 2, 3, math.Inf(1)},
			needsDedup:  false,
			expectedUB:  []float64{1, 2, 3, math.Inf(1)},
			expectedHB: &histogram.Histogram{
				Count:  0,
				Sum:    0,
				Schema: histogram.CustomBucketsSchema,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
				},
				PositiveBuckets: make([]int64, 4),
				CustomValues:    []float64{1, 2, 3},
			},
		},
		{
			name:        "Deduplication needed",
			upperBounds: []float64{1, 2, 2, 3, math.Inf(1)},
			needsDedup:  true,
			expectedUB:  []float64{1, 2, 3, math.Inf(1)},
			expectedHB: &histogram.Histogram{
				Count:  0,
				Sum:    0,
				Schema: histogram.CustomBucketsSchema,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 4},
				},
				PositiveBuckets: make([]int64, 4),
				CustomValues:    []float64{1, 2, 3},
			},
		},
		{
			name:        "No infinite upper bound",
			upperBounds: []float64{1, 2, 3},
			needsDedup:  false,
			expectedUB:  []float64{1, 2, 3},
			expectedHB: &histogram.Histogram{
				Count:  0,
				Sum:    0,
				Schema: histogram.CustomBucketsSchema,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 3},
				},
				PositiveBuckets: make([]int64, 3),
				CustomValues:    []float64{1, 2, 3},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			upperBounds, hBase := convertnhcb.ProcessUpperBoundsAndCreateBaseHistogram(tt.upperBounds, tt.needsDedup)
			require.Equal(t, tt.expectedUB, upperBounds)
			require.Equal(t, tt.expectedHB, hBase)
		})
	}
}

func TestNewHistogram(t *testing.T) {
	tests := []struct {
		name        string
		histogram   convertnhcb.TempHistogram
		upperBounds []float64
		hBase       *histogram.Histogram
		fhBase      *histogram.FloatHistogram
		wantH       *histogram.Histogram
		wantFH      *histogram.FloatHistogram
	}{
		{
			name: "IntegerHistogramTest",
			histogram: convertnhcb.TempHistogram{
				BucketCounts: map[float64]float64{1: 10, 2: 15, 3: 25},
				Count:        25,
				Sum:          50,
				HasFloat:     false,
			},
			upperBounds: []float64{1, 2, 3},
			hBase: &histogram.Histogram{
				PositiveBuckets: make([]int64, 3),
				CustomValues:    []float64{1, 2, 3},
			},
			fhBase: nil,
			wantH: &histogram.Histogram{
				Count:           25,
				Sum:             50,
				PositiveBuckets: []int64{10, 5, 10},
				CustomValues:    []float64{1, 2, 3},
			},
			wantFH: nil,
		},
		{
			name: "FloatHistogramTest",
			histogram: convertnhcb.TempHistogram{
				BucketCounts: map[float64]float64{1: 10.5, 2: 14.5, 3: 24.0},
				Count:        25,
				Sum:          50,
				HasFloat:     true,
			},
			upperBounds: []float64{1, 2, 3},
			hBase:       nil,
			fhBase: &histogram.FloatHistogram{
				PositiveBuckets: make([]float64, 3),
				CustomValues:    []float64{1, 2, 3},
			},
			wantH: nil,
			wantFH: &histogram.FloatHistogram{
				Count:           25,
				Sum:             50,
				PositiveBuckets: []float64{10.5, 4.0, 9.5},
				CustomValues:    []float64{1, 2, 3},
			},
		},
		{
			name: "MissingBucketTestInt",
			histogram: convertnhcb.TempHistogram{
				BucketCounts: map[float64]float64{1: 10, 3: 25},
				Count:        25,
				Sum:          50,
				HasFloat:     false,
			},
			upperBounds: []float64{1, 2, 3},
			hBase: &histogram.Histogram{
				PositiveBuckets: make([]int64, 3),
				CustomValues:    []float64{1, 2, 3},
			},
			fhBase: nil,
			wantH: &histogram.Histogram{
				Count:           25,
				Sum:             50,
				PositiveBuckets: []int64{10, 0, 15},
				CustomValues:    []float64{1, 2, 3},
			},
			wantFH: nil,
		},
		{
			name: "MissingBucketTestFloat",
			histogram: convertnhcb.TempHistogram{
				BucketCounts: map[float64]float64{1: 10.5, 3: 24.0},
				Count:        25,
				Sum:          50,
				HasFloat:     true,
			},
			upperBounds: []float64{1, 2, 3},
			hBase:       nil,
			fhBase: &histogram.FloatHistogram{
				PositiveBuckets: make([]float64, 3),
				CustomValues:    []float64{1, 2, 3},
			},
			wantH: nil,
			wantFH: &histogram.FloatHistogram{
				Count:           25,
				Sum:             50,
				PositiveBuckets: []float64{10.5, 0, 9.5},
				CustomValues:    []float64{1, 2, 3},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotH, gotFH := convertnhcb.NewHistogram(tc.histogram, tc.upperBounds, tc.hBase, tc.fhBase)

			if tc.wantH != nil && gotH != nil {
				if gotH.Count != tc.wantH.Count || gotH.Sum != tc.wantH.Sum {
					t.Errorf("expected histogram %+v, got %+v", tc.wantH, gotH)
				}
			}

			if tc.wantFH != nil && gotFH != nil {
				if gotFH.Count != tc.wantFH.Count || gotFH.Sum != tc.wantFH.Sum {
					t.Errorf("expected float histogram %+v, got %+v", tc.wantFH, gotFH)
				}
			}
		})
	}
}

func TestNewTempHistogram(t *testing.T) {
	th := convertnhcb.NewTempHistogram()

	if th.BucketCounts == nil {
		t.Error("expected BucketCounts to be initialized")
	}
	if len(th.BucketCounts) != 0 {
		t.Errorf("expected BucketCounts to be empty, got length %d", len(th.BucketCounts))
	}
	if th.Count != 0 {
		t.Errorf("expected Count to be 0, got %f", th.Count)
	}
	if th.Sum != 0 {
		t.Errorf("expected Sum to be 0, got %f", th.Sum)
	}
	if th.HasFloat != false {
		t.Errorf("expected HasFloat to be false, got %v", th.HasFloat)
	}
}

func TestGetHistogramMetricBase(t *testing.T) {
	tests := []struct {
		name     string
		labels   labels.Labels
		suffix   string
		expected labels.Labels
	}{
		{
			name:   "Basic Case",
			labels: labels.FromMap(map[string]string{"__name__": "test_metric_bucket", "job": "test_job", "bucket": "0.5"}),
			suffix: "_bucket",
			expected: labels.FromMap(map[string]string{
				"__name__": "test_metric",
				"job":      "test_job",
				"bucket":   "0.5",
			}),
		},
		{
			name:   "No Bucket Label",
			labels: labels.FromMap(map[string]string{"__name__": "test_metric_sum", "job": "test_job"}),
			suffix: "_sum",
			expected: labels.FromMap(map[string]string{
				"__name__": "test_metric",
				"job":      "test_job",
			}),
		},
		{
			name:     "Empty Label Set",
			labels:   labels.Labels{},
			suffix:   "_count",
			expected: labels.Labels{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := convertnhcb.GetHistogramMetricBase(tc.labels, tc.suffix)

			if !labels.Equal(result, tc.expected) {
				t.Errorf("expected labels %+v, got %+v", tc.expected, result)
			}
		})
	}
}

func TestGetHistogramMetricBaseName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "Bucket Suffix", input: "metric_name_bucket", expected: "metric_name"},
		{name: "Sum Suffix", input: "metric_name_sum", expected: "metric_name"},
		{name: "Count Suffix", input: "metric_name_count", expected: "metric_name"},
		{name: "Created Suffix", input: "metric_name_created", expected: "metric_name_created"},
		{name: "Other Suffix", input: "metric_name_other", expected: "metric_name_other"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := convertnhcb.GetHistogramMetricBaseName(tc.input)

			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}
