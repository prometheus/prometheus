// Copyright 2022 The Prometheus Authors
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

package promql_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

func TestVector_ContainsSameLabelset(t *testing.T) {
	for name, tc := range map[string]struct {
		vector   promql.Vector
		expected bool
	}{
		"empty vector": {
			vector:   promql.Vector{},
			expected: false,
		},
		"vector with one series": {
			vector: promql.Vector{
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: false,
		},
		"vector with two different series": {
			vector: promql.Vector{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
			},
			expected: false,
		},
		"vector with two equal series": {
			vector: promql.Vector{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
		"vector with three series, two equal": {
			vector: promql.Vector{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.vector.ContainsSameLabelset())
		})
	}
}

func TestMatrix_ContainsSameLabelset(t *testing.T) {
	for name, tc := range map[string]struct {
		matrix   promql.Matrix
		expected bool
	}{
		"empty matrix": {
			matrix:   promql.Matrix{},
			expected: false,
		},
		"matrix with one series": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: false,
		},
		"matrix with two different series": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
			},
			expected: false,
		},
		"matrix with two equal series": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
		"matrix with three series, two equal": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a")},
				{Metric: labels.FromStrings("lbl", "b")},
				{Metric: labels.FromStrings("lbl", "a")},
			},
			expected: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.matrix.ContainsSameLabelset())
		})
	}
}

func TestMatrix_MergeSeriesWithSameLabelset(t *testing.T) {
	for name, tc := range map[string]struct {
		matrix         promql.Matrix
		expectedMatrix promql.Matrix
		expectError    bool
	}{
		"empty matrix": {
			matrix:         promql.Matrix{},
			expectedMatrix: promql.Matrix{},
			expectError:    false,
		},
		"matrix with one series": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
			},
			expectError: false,
		},
		"matrix with two different series": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "b"), Floats: []promql.FPoint{{T: 1, F: 2.0}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "b"), Floats: []promql.FPoint{{T: 1, F: 2.0}}},
			},
			expectError: false,
		},
		"matrix with two equal series, non-overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 2, F: 2.0}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}, {T: 2, F: 2.0}}},
			},
			expectError: false,
		},
		"matrix with two equal series, overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 2.0}}},
			},
			expectedMatrix: nil,
			expectError:    true,
		},
		"matrix with three series, two equal with non-overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "b"), Floats: []promql.FPoint{{T: 1, F: 3.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 2, F: 2.0}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}, {T: 2, F: 2.0}}},
				{Metric: labels.FromStrings("lbl", "b"), Floats: []promql.FPoint{{T: 1, F: 3.0}}},
			},
			expectError: false,
		},
		"matrix with three series with same labelset, non-overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 2, F: 2.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 3, F: 3.0}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}, {T: 2, F: 2.0}, {T: 3, F: 3.0}}},
			},
			expectError: false,
		},
		"matrix with unsorted timestamps after merge": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 3, F: 3.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}, {T: 3, F: 3.0}}},
			},
			expectError: false,
		},
		"matrix with multiple float samples per series": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}, {T: 2, F: 2.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 3, F: 3.0}, {T: 4, F: 4.0}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}, {T: 2, F: 2.0}, {T: 3, F: 3.0}, {T: 4, F: 4.0}}},
			},
			expectError: false,
		},
		"matrix with overlapping timestamps in multi-sample series": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}, {T: 2, F: 2.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 2, F: 3.0}, {T: 3, F: 4.0}}},
			},
			expectedMatrix: nil,
			expectError:    true,
		},
		"matrix with histogram samples, non-overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Histograms: []promql.HPoint{{T: 1, H: &histogram.FloatHistogram{Count: 1, Sum: 1}}}},
				{Metric: labels.FromStrings("lbl", "a"), Histograms: []promql.HPoint{{T: 2, H: &histogram.FloatHistogram{Count: 2, Sum: 2}}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Histograms: []promql.HPoint{{T: 1, H: &histogram.FloatHistogram{Count: 1, Sum: 1}}, {T: 2, H: &histogram.FloatHistogram{Count: 2, Sum: 2}}}},
			},
			expectError: false,
		},
		"matrix with histogram samples, overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Histograms: []promql.HPoint{{T: 1, H: &histogram.FloatHistogram{Count: 1, Sum: 1}}}},
				{Metric: labels.FromStrings("lbl", "a"), Histograms: []promql.HPoint{{T: 1, H: &histogram.FloatHistogram{Count: 2, Sum: 2}}}},
			},
			expectedMatrix: nil,
			expectError:    true,
		},
		"matrix with mixed float and histogram samples, non-overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Histograms: []promql.HPoint{{T: 2, H: &histogram.FloatHistogram{Count: 2, Sum: 2}}}},
			},
			expectedMatrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}, Histograms: []promql.HPoint{{T: 2, H: &histogram.FloatHistogram{Count: 2, Sum: 2}}}},
			},
			expectError: false,
		},
		"matrix with mixed float and histogram samples, overlapping timestamps": {
			matrix: promql.Matrix{
				{Metric: labels.FromStrings("lbl", "a"), Floats: []promql.FPoint{{T: 1, F: 1.0}}},
				{Metric: labels.FromStrings("lbl", "a"), Histograms: []promql.HPoint{{T: 1, H: &histogram.FloatHistogram{Count: 2, Sum: 2}}}},
			},
			expectedMatrix: nil,
			expectError:    true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := tc.matrix.MergeSeriesWithSameLabelset()
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			// Sort both matrices for comparison since map iteration order is not guaranteed
			require.ElementsMatch(t, tc.expectedMatrix, result)
		})
	}
}
