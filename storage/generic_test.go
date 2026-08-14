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

package storage

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/annotations"
)

// prefixScoreFilter accepts values starting with prefix and returns a score
// inversely proportional to the value length (shorter = higher score). Useful
// for asserting top-K behaviour where the filter produces a non-trivial
// distribution of scores.
type prefixScoreFilter struct{ prefix string }

func (f prefixScoreFilter) Accept(v string) (bool, float64) {
	if !strings.HasPrefix(v, f.prefix) {
		return false, 0
	}
	// Score in (0, 1], higher for shorter values.
	return true, 1.0 / float64(1+len(v)-len(f.prefix))
}

// constScoreFilter accepts every value with the same score. It is used to
// exercise the OrderByScoreDesc tie-break on Value asc.
type constScoreFilter float64

func (f constScoreFilter) Accept(string) (bool, float64) { return true, float64(f) }

func TestApplySearchHints(t *testing.T) {
	values := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

	t.Run("nil hints returns all with score 1", func(t *testing.T) {
		got := ApplySearchHints(values, nil)
		require.Len(t, got, len(values))
		for i, r := range got {
			require.Equal(t, values[i], r.Value)
			require.Equal(t, 1.0, r.Score)
		}
	})

	t.Run("no filter ascending limit", func(t *testing.T) {
		got := ApplySearchHints(values, &SearchHints{Limit: 3})
		require.Equal(t, []SearchResult{
			{Value: "alpha", Score: 1.0},
			{Value: "beta", Score: 1.0},
			{Value: "gamma", Score: 1.0},
		}, got)
	})

	t.Run("no filter descending limit walks input in reverse", func(t *testing.T) {
		got := ApplySearchHints(values, &SearchHints{OrderBy: OrderByValueDesc, Limit: 2})
		require.Equal(t, []SearchResult{
			{Value: "epsilon", Score: 1.0},
			{Value: "delta", Score: 1.0},
		}, got)
	})

	t.Run("filter ascending early exit at limit", func(t *testing.T) {
		hits := 0
		filter := FilterFunc(func(_ string) (bool, float64) {
			hits++
			return true, 1.0
		})
		got := ApplySearchHints(values, &SearchHints{Filter: filter, Limit: 2})
		require.Len(t, got, 2)
		// Early exit: the filter should not have been called for values past
		// the second match.
		require.Equal(t, 2, hits)
	})

	t.Run("filter descending reverses then limits", func(t *testing.T) {
		got := ApplySearchHints(values, &SearchHints{
			Filter:  prefixScoreFilter{prefix: ""}, // accepts everything.
			OrderBy: OrderByValueDesc,
			Limit:   2,
		})
		require.Len(t, got, 2)
		// Input order is values[0..4]; reverse picks epsilon then delta.
		require.Equal(t, "epsilon", got[0].Value)
		require.Equal(t, "delta", got[1].Value)
	})

	t.Run("filter descending limit early exits without scanning all", func(t *testing.T) {
		// Counting filter that accepts every value. With early exit we
		// should call Accept at most Limit times for ValueDesc + Filter.
		hits := 0
		filter := FilterFunc(func(string) (bool, float64) {
			hits++
			return true, 1.0
		})
		got := ApplySearchHints(values, &SearchHints{
			Filter:  filter,
			OrderBy: OrderByValueDesc,
			Limit:   2,
		})
		require.Equal(t, []SearchResult{
			{Value: "epsilon", Score: 1.0},
			{Value: "delta", Score: 1.0},
		}, got)
		require.Equal(t, 2, hits, "must early-exit at limit for OrderByValueDesc")
	})

	t.Run("score desc top-K returns best K", func(t *testing.T) {
		// Build a wide value set where score is inversely tied to length.
		// Top-K of size 3 should pick the three shortest "x" values.
		var input []string
		for i := range 100 {
			input = append(input, fmt.Sprintf("x%0*d", i+1, i))
		}
		got := ApplySearchHints(input, &SearchHints{
			Filter:  prefixScoreFilter{prefix: "x"},
			OrderBy: OrderByScoreDesc,
			Limit:   3,
		})
		require.Len(t, got, 3)
		// Scores are non-increasing.
		for i := 1; i < len(got); i++ {
			require.GreaterOrEqual(t, got[i-1].Score, got[i].Score)
		}
		// Best score is the shortest value ("x0").
		require.Equal(t, "x0", got[0].Value)
	})

	t.Run("score desc tie-break on Value asc", func(t *testing.T) {
		// With a constant score, ordering tie-breaks on Value ascending.
		got := ApplySearchHints(values, &SearchHints{
			Filter:  constScoreFilter(0.5),
			OrderBy: OrderByScoreDesc,
			Limit:   3,
		})
		require.Equal(t, []SearchResult{
			{Value: "alpha", Score: 0.5},
			{Value: "beta", Score: 0.5},
			{Value: "delta", Score: 0.5},
		}, got)
	})

	t.Run("score desc unlimited keeps all matches sorted", func(t *testing.T) {
		got := ApplySearchHints(values, &SearchHints{
			Filter:  prefixScoreFilter{prefix: ""},
			OrderBy: OrderByScoreDesc,
		})
		require.Len(t, got, len(values))
		for i := 1; i < len(got); i++ {
			if got[i-1].Score == got[i].Score {
				require.Less(t, got[i-1].Value, got[i].Value)
				continue
			}
			require.Greater(t, got[i-1].Score, got[i].Score)
		}
	})

	t.Run("near-MaxInt limit does not panic top-K", func(t *testing.T) {
		// Reachable when --web.search.max-limit=0 disables the cap and a
		// client supplies a near-MaxInt limit. The pre-allocation must
		// clamp to len(values) rather than blowing up makeslice.
		got := ApplySearchHints([]string{"alpha", "beta", "gamma"}, &SearchHints{
			Filter:  FilterFunc(func(string) (bool, float64) { return true, 0.5 }),
			OrderBy: OrderByScoreDesc,
			Limit:   math.MaxInt,
		})
		require.Len(t, got, 3)
	})

	t.Run("near-MaxInt limit does not panic reverse early exit", func(t *testing.T) {
		got := ApplySearchHints([]string{"alpha", "beta", "gamma"}, &SearchHints{
			Filter:  FilterFunc(func(string) (bool, float64) { return true, 1.0 }),
			OrderBy: OrderByValueDesc,
			Limit:   math.MaxInt,
		})
		require.Equal(t, []SearchResult{
			{Value: "gamma", Score: 1.0},
			{Value: "beta", Score: 1.0},
			{Value: "alpha", Score: 1.0},
		}, got)
	})

	t.Run("near-MaxInt limit does not panic linear ascending path", func(t *testing.T) {
		// Same DoS-shape input but routed through applySearchHintsLinear
		// via OrderByValueAsc. linearResultCap must absorb the overflow
		// without makeslice panicking.
		got := ApplySearchHints([]string{"alpha", "beta", "gamma"}, &SearchHints{
			Filter:  FilterFunc(func(string) (bool, float64) { return true, 0.5 }),
			OrderBy: OrderByValueAsc,
			Limit:   math.MaxInt,
		})
		require.Len(t, got, 3)
		require.Equal(t, "alpha", got[0].Value)
		require.Equal(t, "gamma", got[2].Value)
	})

	t.Run("filter selectivity 1 percent caps allocation", func(t *testing.T) {
		// 10k values, 1% match. Without the alloc cap fix the result slice
		// would be sized to 10k. We don't directly assert capacity here (it
		// is implementation-defined), but exercising this path under -race
		// catches regressions in the linear capacity helper.
		var input []string
		for i := range 10000 {
			input = append(input, fmt.Sprintf("v%d", i))
		}
		got := ApplySearchHints(input, &SearchHints{
			Filter: FilterFunc(func(v string) (bool, float64) {
				// Match values whose decimal representation ends in "00".
				return strings.HasSuffix(v, "00"), 0.5
			}),
			Limit: 50,
		})
		require.LessOrEqual(t, len(got), 50)
	})
}

func TestNewSearchResultSetFromSliceAndError(t *testing.T) {
	// Verify that the warns parameter round-trips through Warnings() and
	// that Err() is hidden until the slice is exhausted, so callers see
	// the partial-results-then-error shape the helper documents.
	var warns annotations.Annotations
	warns.Add(errors.New("upstream warning"))

	results := []SearchResult{
		{Value: "alpha", Score: 1.0},
		{Value: "beta", Score: 1.0},
	}
	rs := NewSearchResultSetFromSliceAndError(results, warns, errors.New("tail failure"))

	var got []SearchResult
	for rs.Next() {
		got = append(got, rs.At())
		// Warnings are observable from the start, before iteration ends.
		require.NotNil(t, rs.Warnings())
		// Err() must remain nil while the slice is still producing values.
		require.NoError(t, rs.Err())
	}
	require.Equal(t, results, got)
	require.Error(t, rs.Err())
	require.Contains(t, rs.Err().Error(), "tail failure")

	// Warnings still reflect the upstream payload after iteration ended.
	gotWarnings := rs.Warnings().AsErrors()
	require.Len(t, gotWarnings, 1)
	require.Contains(t, gotWarnings[0].Error(), "upstream warning")

	require.NoError(t, rs.Close())
}

func TestLinearResultCap(t *testing.T) {
	// Boundary: limit smaller than the floor.
	require.Equal(t, minLinearAllocCap, linearResultCap(10000, 10))
	// Limit zero means unlimited; cap grows to len.
	require.Equal(t, 10000, linearResultCap(10000, 0))
	// Cap is capped by len(values).
	require.Equal(t, 50, linearResultCap(50, 100))
	// 2*limit dominates when above the floor and below len.
	require.Equal(t, 1000, linearResultCap(10000, 500))
	// Overflow fallback returns the small-allocation floor so a near-MaxInt
	// limit does not pull a multi-MB upfront allocation when the filter
	// would reject most of the input anyway.
	require.Equal(t, minLinearAllocCap, linearResultCap(10000, math.MaxInt))
}

// FilterFunc adapts a function to the Filter interface for testing.
type FilterFunc func(string) (bool, float64)

func (f FilterFunc) Accept(v string) (bool, float64) { return f(v) }

func BenchmarkApplySearchHints(b *testing.B) {
	const n = 100_000
	values := make([]string, n)
	for i := range values {
		values[i] = fmt.Sprintf("metric_%07d", i)
	}

	cases := []struct {
		name  string
		hints *SearchHints
	}{
		{"NoFilter_NoLimit_Asc", &SearchHints{OrderBy: OrderByValueAsc}},
		{"NoFilter_Limit100_Asc", &SearchHints{OrderBy: OrderByValueAsc, Limit: 100}},
		{"NoFilter_Limit100_Desc", &SearchHints{OrderBy: OrderByValueDesc, Limit: 100}},
		{"Filter1pct_NoLimit_Asc", &SearchHints{Filter: FilterFunc(func(v string) (bool, float64) {
			return strings.HasSuffix(v, "00"), 1.0
		})}},
		{"Filter1pct_Limit100_Asc", &SearchHints{Filter: FilterFunc(func(v string) (bool, float64) {
			return strings.HasSuffix(v, "00"), 1.0
		}), Limit: 100}},
		{"Filter50pct_TopK100_ScoreDesc", &SearchHints{Filter: FilterFunc(func(v string) (bool, float64) {
			// Diversify scores by hashing the trailing two characters
			// so the heap exercises real reorderings, not just the
			// tied-score replace path.
			b0 := v[len(v)-1]
			if b0&1 != 0 {
				return false, 0
			}
			b1 := v[len(v)-2]
			score := float64(int(b0)*31+int(b1)%53) / 1024.0
			if score > 1.0 {
				score = 1.0
			}
			return true, score
		}), OrderBy: OrderByScoreDesc, Limit: 100}},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = ApplySearchHints(values, c.hints)
			}
		})
	}
}
