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

func TestSortByLabelNaturallyEqualValuesUseFullLabelSetTieBreak(t *testing.T) {
	samples := Vector{
		{
			Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "001", "shard", "a"),
			F:      1,
		},
		{
			Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "01", "shard", "b"),
			F:      2,
		},
		{
			Metric: labels.FromStrings("__name__", "leading_zero_cpu", "cpu", "1", "shard", "c"),
			F:      3,
		},
	}
	expectedAscending := []string{"001", "01", "1"}
	expectedDescending := []string{"1", "01", "001"}

	for _, vec := range samplePermutations(samples) {
		args := parser.Expressions{nil, &parser.StringLiteral{Val: "cpu"}}

		ascending, _ := funcSortByLabel([]Vector{append(Vector(nil), vec...)}, nil, args, nil)
		require.Equal(t, expectedAscending, cpuLabelValues(ascending))

		descending, _ := funcSortByLabelDesc([]Vector{append(Vector(nil), vec...)}, nil, args, nil)
		require.Equal(t, expectedDescending, cpuLabelValues(descending))
	}
}

func TestSortByLabelMissingAndEmptyValuesSortBeforePopulatedValues(t *testing.T) {
	samples := Vector{
		{
			Metric: labels.FromStrings("__name__", "cpu_usage", "id", "missing"),
			F:      1,
		},
		{
			Metric: labels.FromStrings("__name__", "cpu_usage", "cpu", "1", "id", "present"),
			F:      2,
		},
		{
			Metric: labels.FromStrings("__name__", "cpu_usage", "cpu", "", "id", "empty"),
			F:      3,
		},
	}
	expectedAscending := []string{"empty", "missing", "present"}
	expectedDescending := []string{"present", "missing", "empty"}

	for _, vec := range samplePermutations(samples) {
		args := parser.Expressions{nil, &parser.StringLiteral{Val: "cpu"}}

		ascending, _ := funcSortByLabel([]Vector{append(Vector(nil), vec...)}, nil, args, nil)
		require.Equal(t, expectedAscending, idLabelValues(ascending))

		descending, _ := funcSortByLabelDesc([]Vector{append(Vector(nil), vec...)}, nil, args, nil)
		require.Equal(t, expectedDescending, idLabelValues(descending))
	}
}

func samplePermutations(samples Vector) []Vector {
	var result []Vector
	var permute func(Vector, int)
	permute = func(vec Vector, index int) {
		if index == len(vec) {
			result = append(result, append(Vector(nil), vec...))
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

func cpuLabelValues(vec Vector) []string {
	values := make([]string, 0, len(vec))
	for _, sample := range vec {
		values = append(values, sample.Metric.Get("cpu"))
	}
	return values
}

func idLabelValues(vec Vector) []string {
	values := make([]string, 0, len(vec))
	for _, sample := range vec {
		values = append(values, sample.Metric.Get("id"))
	}
	return values
}
