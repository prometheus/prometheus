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

package promql

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestSortByLabelOrder(t *testing.T) {
	for _, tc := range []struct {
		name               string
		samples            Vector
		sortLabels         []string
		resultLabel        string
		expectedAscending  []string
		expectedDescending []string
	}{
		{
			name: "naturally equal values fall back to lexicographic order",
			samples: Vector{
				{Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "001", "shard", "a"), F: 1},
				{Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "01", "shard", "b"), F: 2},
				{Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "1", "shard", "c"), F: 3},
			},
			sortLabels:         []string{"cpu"},
			resultLabel:        "cpu",
			expectedAscending:  []string{"001", "01", "1"},
			expectedDescending: []string{"1", "01", "001"},
		},
		{
			name: "lexicographic fallback is used before the next sort label",
			samples: Vector{
				{Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "1", "shard", "a"), F: 1},
				{Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "01", "shard", "b"), F: 2},
			},
			sortLabels:         []string{"cpu", "shard"},
			resultLabel:        "cpu",
			expectedAscending:  []string{"01", "1"},
			expectedDescending: []string{"1", "01"},
		},
		{
			name: "missing and empty values sort before populated values",
			samples: Vector{
				{Metric: labels.FromStrings("__name__", "cpu_usage", "id", "missing"), F: 1},
				{Metric: labels.FromStrings("__name__", "cpu_usage", "cpu", "1", "id", "present"), F: 2},
				{Metric: labels.FromStrings("__name__", "cpu_usage", "cpu", "", "id", "empty"), F: 3},
			},
			sortLabels:         []string{"cpu"},
			resultLabel:        "id",
			expectedAscending:  []string{"empty", "missing", "present"},
			expectedDescending: []string{"present", "missing", "empty"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := parser.Expressions{nil}
			for _, l := range tc.sortLabels {
				args = append(args, &parser.StringLiteral{Val: l})
			}

			for _, vec := range samplePermutations(tc.samples) {
				ascending, _ := funcSortByLabel([]Vector{slices.Clone(vec)}, nil, args, nil)
				require.Equal(t, tc.expectedAscending, labelValues(ascending, tc.resultLabel))

				descending, _ := funcSortByLabelDesc([]Vector{slices.Clone(vec)}, nil, args, nil)
				require.Equal(t, tc.expectedDescending, labelValues(descending, tc.resultLabel))
			}
		})
	}
}

func samplePermutations(samples Vector) []Vector {
	var result []Vector
	var permute func(Vector, int)
	permute = func(vec Vector, index int) {
		if index == len(vec) {
			result = append(result, slices.Clone(vec))
			return
		}
		for i := index; i < len(vec); i++ {
			vec[index], vec[i] = vec[i], vec[index]
			permute(vec, index+1)
			vec[index], vec[i] = vec[i], vec[index]
		}
	}
	permute(slices.Clone(samples), 0)
	return result
}

func labelValues(vec Vector, name string) []string {
	values := make([]string, 0, len(vec))
	for _, sample := range vec {
		values = append(values, sample.Metric.Get(name))
	}
	return values
}
