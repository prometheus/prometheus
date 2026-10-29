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

package storage_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestMockSeries(t *testing.T) {
	s := storage.MockSeries(nil, []int64{1, 2, 3}, []float64{1, 2, 3}, []string{"__name__", "foo"})
	it := s.Iterator(nil)
	ts := []int64{}
	vs := []float64{}
	for it.Next() == chunkenc.ValFloat {
		t, v := it.At()
		ts = append(ts, t)
		vs = append(vs, v)
	}
	require.Equal(t, []int64{1, 2, 3}, ts)
	require.Equal(t, []float64{1, 2, 3}, vs)
}

func TestMockSeriesWithST(t *testing.T) {
	s := storage.MockSeries([]int64{0, 1, 2}, []int64{1, 2, 3}, []float64{1, 2, 3}, []string{"__name__", "foo"})
	it := s.Iterator(nil)
	ts := []int64{}
	vs := []float64{}
	st := []int64{}
	for it.Next() == chunkenc.ValFloat {
		t, v := it.At()
		ts = append(ts, t)
		vs = append(vs, v)
		st = append(st, it.AtST())
	}
	require.Equal(t, []int64{1, 2, 3}, ts)
	require.Equal(t, []float64{1, 2, 3}, vs)
	require.Equal(t, []int64{0, 1, 2}, st)
}

func TestLabelHintsApplyLimit(t *testing.T) {
	// Unordered, so a leading limit and a smallest limit differ.
	input := []string{"d", "b", "e", "a", "c"}

	for _, tc := range []struct {
		name          string
		hints         *storage.LabelHints
		want          []string
		wantAllocated bool
	}{
		{"nil hints", nil, input, false},
		{"no limit", &storage.LabelHints{LimitSmallest: true}, input, false},
		{"limit at count", &storage.LabelHints{Limit: 5, LimitSmallest: true}, input, false},
		{"limit above count", &storage.LabelHints{Limit: 6, LimitSmallest: true}, input, false},
		{"negative limit", &storage.LabelHints{Limit: -1, LimitSmallest: true}, input, false},
		{"negative limit without flag", &storage.LabelHints{Limit: -1}, input, false},
		{"leading limit", &storage.LabelHints{Limit: 3}, []string{"d", "b", "e"}, false},
		{"smallest limit", &storage.LabelHints{Limit: 3, LimitSmallest: true}, []string{"a", "b", "c"}, true},
		{"smallest limit of one", &storage.LabelHints{Limit: 1, LimitSmallest: true}, []string{"a"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := slices.Clone(input)
			got, allocated := tc.hints.ApplyLimit(values)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantAllocated, allocated)
			require.Equal(t, input, values, "ApplyLimit must not modify its input")

			if allocated {
				// The caller may keep and modify an allocated result.
				got[0] = "zzz"
				require.Equal(t, input, values, "an allocated result must not alias the input")
			}
		})
	}
}

// TestLabelHintsApplyLimitSmallest compares the bounded selection against a
// full sort over randomly ordered values, at limits either side of a power of
// two so the heap is exercised at depth.
func TestLabelHintsApplyLimitSmallest(t *testing.T) {
	const count = 500
	values := make([]string, count)
	for i := range values {
		values[i] = fmt.Sprintf("value_%04d", i)
	}

	shuffled := slices.Clone(values)
	rand.New(rand.NewPCG(1, 2)).Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	for _, limit := range []int{1, 2, 3, 7, 63, 64, 65, count - 1} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			hints := &storage.LabelHints{Limit: limit, LimitSmallest: true}
			got, allocated := hints.ApplyLimit(shuffled)
			require.True(t, allocated)
			require.Equal(t, values[:limit], got)
		})
	}
}

func TestLabelHintsApplyLimitEdgeInputs(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		hints := &storage.LabelHints{Limit: 3, LimitSmallest: true}
		got, allocated := hints.ApplyLimit(nil)
		require.Nil(t, got)
		require.False(t, allocated)
	})

	t.Run("empty input", func(t *testing.T) {
		hints := &storage.LabelHints{Limit: 3, LimitSmallest: true}
		got, allocated := hints.ApplyLimit([]string{})
		require.Empty(t, got)
		require.False(t, allocated)
	})

	t.Run("duplicate values", func(t *testing.T) {
		// Duplicates must not be collapsed, and must not displace a smaller
		// value that appears later.
		hints := &storage.LabelHints{Limit: 3, LimitSmallest: true}
		got, allocated := hints.ApplyLimit([]string{"b", "b", "c", "a", "b"})
		require.Equal(t, []string{"a", "b", "b"}, got)
		require.True(t, allocated)
	})
}

func TestLabelHintsAllowsEarlyStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hints *storage.LabelHints
		want  bool
	}{
		{"nil hints", nil, false},
		{"no limit", &storage.LabelHints{}, false},
		{"negative limit", &storage.LabelHints{Limit: -1}, false},
		{"limit only", &storage.LabelHints{Limit: 1}, true},
		{"limit with smallest", &storage.LabelHints{Limit: 1, LimitSmallest: true}, false},
		{"smallest without limit", &storage.LabelHints{LimitSmallest: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.hints.AllowsEarlyStop())
		})
	}
}
