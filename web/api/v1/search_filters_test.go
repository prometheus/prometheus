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

package v1

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
)

func TestSubstringFilter(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		value        string
		wantAccepted bool
		wantScore    float64
	}{
		{
			name:         "exact match",
			query:        "prometheus",
			value:        "prometheus",
			wantAccepted: true,
			wantScore:    1.0,
		},
		{
			name:         "prefix match",
			query:        "prom",
			value:        "prometheus",
			wantAccepted: true,
			wantScore:    1.0,
		},
		{
			name:         "substring match in middle",
			query:        "meth",
			value:        "prometheus",
			wantAccepted: true,
			// idx=3, maxIdx=10-4=6 -> 1.0 - 0.9*3/6 = 0.55.
			wantScore: 0.55,
		},
		{
			name:         "substring match at end",
			query:        "heus",
			value:        "prometheus",
			wantAccepted: true,
			// idx=6, maxIdx=10-4=6 -> 1.0 - 0.9 = 0.1.
			wantScore: 0.1,
		},
		{
			name:         "case mismatch is not accepted",
			query:        "prometheus",
			value:        "Prometheus",
			wantAccepted: false,
			wantScore:    0.0,
		},
		{
			name:         "no match",
			query:        "grafana",
			value:        "prometheus",
			wantAccepted: false,
			wantScore:    0.0,
		},
		{
			name:         "empty query accepts all",
			query:        "",
			value:        "anything",
			wantAccepted: true,
			wantScore:    1.0,
		},
		{
			// "本" appears at byte offset 6 (after "東京日") but at rune
			// offset 3. The pre-rune scoring would return ~0.55; rune
			// scoring returns 0.55 for offset 3 of 6 since maxPos=6.
			// "東京日本京都" has 6 runes; query "本" has 1; maxPos=5;
			// pos=3 -> 1.0 - 0.9*3/5 = 0.46.
			name:         "multi-byte rune position",
			query:        "本",
			value:        "東京日本京都",
			wantAccepted: true,
			wantScore:    0.46,
		},
		{
			// ASCII fast path remains correct: identical scoring to
			// the original implementation for the common case.
			name:         "ascii substring stays on fast path",
			query:        "fo",
			value:        "infor",
			wantAccepted: true,
			// idx=2, maxIdx=5-2=3 -> 1.0 - 0.9*2/3 = 0.4.
			wantScore: 0.4,
		},
		{
			// ASCII query, ASCII match prefix, multi-byte suffix: the
			// fast path must NOT trigger because len(value) overcounts
			// runes and would inflate the score. Expected rune-based:
			// runes("abc日本")=5, runes("b")=1 -> maxPos=4, pos=1 ->
			// 1.0 - 0.9*1/4 = 0.775.
			name:         "ascii match with multi-byte suffix uses rune positions",
			query:        "b",
			value:        "abc日本",
			wantAccepted: true,
			wantScore:    0.775,
		},
		{
			// Symmetric case: multi-byte prefix forces the rune path
			// for the position computation.
			name:         "multi-byte prefix uses rune positions",
			query:        "c",
			value:        "日本cabc",
			wantAccepted: true,
			// runes("日本cabc")=6, query=1 -> maxPos=5, pos=2 (after
			// "日本") -> 1.0 - 0.9*2/5 = 0.64.
			wantScore: 0.64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewSubstringFilter(tt.query)
			accepted, score := filter.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			require.InDelta(t, tt.wantScore, score, 0.01)
		})
	}
}

func TestFuzzyFilter(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		threshold    float64
		value        string
		wantAccepted bool
		minScore     float64 // Minimum expected score.
	}{
		{
			name:         "exact match",
			query:        "prometheus",
			threshold:    0.8,
			value:        "prometheus",
			wantAccepted: true,
			minScore:     1.0,
		},
		{
			name:         "close match above threshold",
			query:        "prometheus",
			threshold:    0.8,
			value:        "promethus", // Typo: one char different.
			wantAccepted: true,
			minScore:     0.8,
		},
		{
			name:         "distant match below threshold",
			query:        "prometheus",
			threshold:    0.8,
			value:        "grafana",
			wantAccepted: false,
			minScore:     0.0,
		},
		{
			name:         "case mismatch is not accepted",
			query:        "prometheus",
			threshold:    0.8,
			value:        "PROMETHEUS",
			wantAccepted: false,
			minScore:     0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewFuzzyFilter(tt.query, tt.threshold)
			accepted, score := filter.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			if tt.wantAccepted {
				require.GreaterOrEqual(t, score, tt.minScore)
			}
		})
	}
}

func TestFuzzyFilterConcurrency(_ *testing.T) {
	filter := NewFuzzyFilter("prometheus", 0.8)
	values := []string{"prometheus", "promethus", "promethius", "prmetheus", "prometeus"} //nolint:misspell

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for _, value := range values {
				_, _ = filter.Accept(value)
			}
		})
	}

	wg.Wait()
}

func TestSubsequenceFilter(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		threshold    float64
		value        string
		wantAccepted bool
		wantScore    float64 // -1 means "any positive value".
	}{
		{
			name:         "exact match",
			query:        "prometheus",
			threshold:    1.0,
			value:        "prometheus",
			wantAccepted: true,
			wantScore:    1.0,
		},
		{
			name:         "prefix match scores 1.0",
			query:        "prom",
			threshold:    1.0,
			value:        "prometheus",
			wantAccepted: true,
			wantScore:    1.0,
		},
		{
			name:         "subsequence match above zero threshold",
			query:        "pms",
			threshold:    0.0,
			value:        "prometheus",
			wantAccepted: true,
			wantScore:    -1,
		},
		{
			name:         "non-subsequence rejected",
			query:        "xyz",
			threshold:    0.0,
			value:        "prometheus",
			wantAccepted: false,
			wantScore:    0.0,
		},
		{
			name:         "case mismatch rejected",
			query:        "prom",
			threshold:    0.0,
			value:        "PROMETHEUS",
			wantAccepted: false,
			wantScore:    0.0,
		},
		{
			name:         "below threshold rejected",
			query:        "pms",
			threshold:    0.99,
			value:        "prometheus",
			wantAccepted: false,
			wantScore:    -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewSubsequenceFilter(tt.query, tt.threshold)
			accepted, score := filter.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			if tt.wantScore >= 0 {
				require.InDelta(t, tt.wantScore, score, 1e-9)
			}
		})
	}
}

// countingFilter records how many times Accept is called per value.
type countingFilter struct {
	mu     sync.Mutex
	calls  map[string]int
	result map[string]memoEntry
}

func (f *countingFilter) Accept(value string) (bool, float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[value]++
	r := f.result[value]
	return r.accepted, r.score
}

func TestMemoizingFilter(t *testing.T) {
	inner := &countingFilter{
		calls: map[string]int{},
		result: map[string]memoEntry{
			"prometheus": {accepted: true, score: 1.0},
			"grafana":    {accepted: false, score: 0.0},
		},
	}
	memo := newMemoizingFilter(inner)

	// First call computes.
	accepted, score := memo.Accept("prometheus")
	require.True(t, accepted)
	require.Equal(t, 1.0, score)

	// Repeat calls hit the cache.
	for range 5 {
		accepted, score = memo.Accept("prometheus")
		require.True(t, accepted)
		require.Equal(t, 1.0, score)
	}

	// Distinct values are computed once each.
	accepted, score = memo.Accept("grafana")
	require.False(t, accepted)
	require.Equal(t, 0.0, score)
	memo.Accept("grafana")

	require.Equal(t, 1, inner.calls["prometheus"])
	require.Equal(t, 1, inner.calls["grafana"])
}

func TestMemoizingFilter_CacheCap(t *testing.T) {
	// Build a result map slightly larger than the memo cap so we can observe
	// the inner filter being recalled for values past the cap.
	results := make(map[string]memoEntry, memoizingFilterMaxEntries+10)
	for i := range memoizingFilterMaxEntries + 10 {
		results[strconv.Itoa(i)] = memoEntry{accepted: true, score: 1.0}
	}
	inner := &countingFilter{
		calls:  map[string]int{},
		result: results,
	}
	memo := newMemoizingFilter(inner)

	// Fill the cache to its cap.
	for i := range memoizingFilterMaxEntries {
		memo.Accept(strconv.Itoa(i))
	}
	require.Len(t, memo.cache, memoizingFilterMaxEntries)

	// A value past the cap is computed by inner but not stored.
	const overflow = "overflow-value"
	inner.result[overflow] = memoEntry{accepted: true, score: 0.5}
	memo.Accept(overflow)
	memo.Accept(overflow)
	require.Len(t, memo.cache, memoizingFilterMaxEntries)
	// Past the cap, every Accept reaches the inner filter.
	require.Equal(t, 2, inner.calls[overflow])

	// Cached values are still served from the cache.
	memo.Accept("0")
	require.Equal(t, 1, inner.calls["0"])
}

func TestMemoizingFilter_ByteCap(t *testing.T) {
	// Construct distinct values long enough that the byte cap is reached
	// well before the entry cap. Each key contributes len(value) bytes to
	// f.bytes.
	const valueLen = 1024
	entriesToOverflow := memoizingFilterMaxBytes/valueLen + 2
	require.Less(t, entriesToOverflow, memoizingFilterMaxEntries,
		"test setup: byte cap must be reached before entry cap")

	keys := make([]string, 0, entriesToOverflow)
	results := make(map[string]memoEntry, entriesToOverflow)
	for i := range entriesToOverflow {
		prefix := fmt.Sprintf("%010d:", i)
		key := prefix + strings.Repeat("x", valueLen-len(prefix))
		keys = append(keys, key)
		results[key] = memoEntry{accepted: true, score: 1.0}
	}
	require.Len(t, keys[0], valueLen)

	inner := &countingFilter{
		calls:  map[string]int{},
		result: results,
	}
	memo := newMemoizingFilter(inner)

	for _, k := range keys {
		memo.Accept(k)
	}

	// The byte cap must have stopped insertion before all entries fit.
	require.Less(t, len(memo.cache), entriesToOverflow,
		"byte cap must limit cached entries below the inserted count")
	require.Less(t, len(memo.cache), memoizingFilterMaxEntries,
		"byte cap must be reached before the entry cap")
	require.LessOrEqual(t, memo.bytes, memoizingFilterMaxBytes,
		"cached bytes must not exceed the configured budget")

	// A value past the cap is recomputed by the inner filter each call.
	overflowKey := keys[entriesToOverflow-1]
	callsBefore := inner.calls[overflowKey]
	require.NotContains(t, memo.cache, overflowKey,
		"the last inserted key must be past the byte cap and therefore not cached")
	memo.Accept(overflowKey)
	memo.Accept(overflowKey)
	require.Equal(t, callsBefore+2, inner.calls[overflowKey],
		"values past the byte cap must reach the inner filter on every Accept")
}

func TestBuildSearchFilter_MemoOnlyForExpensiveChain(t *testing.T) {
	// Substring-only chain: jaro-winkler with no fuzz threshold. Memo is
	// not added because substring scoring is already O(L).
	got := buildSearchFilter([]string{"prom"}, 0, "jarowinkler", true)
	_, isMemo := got.(*memoizingFilter)
	require.False(t, isMemo, "substring-only chain should not be wrapped with memoizingFilter")

	// Subsequence: always memoized.
	got = buildSearchFilter([]string{"prm"}, 0, "subsequence", true)
	_, isMemo = got.(*memoizingFilter)
	require.True(t, isMemo, "subsequence chain must be memoized")

	// Jaro-Winkler with a non-zero fuzz threshold: memoized.
	got = buildSearchFilter([]string{"prom"}, 80, "jarowinkler", true)
	_, isMemo = got.(*memoizingFilter)
	require.True(t, isMemo, "jaro-winkler with fuzz threshold must be memoized")
}

func TestCaseFoldingFilter(t *testing.T) {
	// Inner filter expects lowercased query and value.
	inner := NewSubstringFilter("prom")
	wrapped := newCaseFoldingFilter(inner)

	accepted, score := wrapped.Accept("Prometheus")
	require.True(t, accepted)
	require.Equal(t, 1.0, score)

	accepted, _ = wrapped.Accept("Grafana")
	require.False(t, accepted)
}

func TestOrFilter(t *testing.T) {
	tests := []struct {
		name           string
		substringQuery string
		fuzzyQuery     string
		fuzzyThreshold float64
		value          string
		wantAccepted   bool
		minScore       float64
	}{
		{
			name:           "substring match only",
			substringQuery: "prom",
			value:          "prometheus",
			wantAccepted:   true,
			minScore:       1.0, // Prefix match.
		},
		{
			name:           "substring rejects",
			substringQuery: "prom",
			value:          "node",
			wantAccepted:   false,
		},
		{
			name:           "fuzzy fallback",
			substringQuery: "go_gor",
			fuzzyQuery:     "go_gor",
			fuzzyThreshold: 0.8,
			value:          "go_goroutins", // Not substring, but fuzzy matches.
			wantAccepted:   true,
			minScore:       0.8,
		},
		{
			name:         "no filters accept all",
			value:        "anything",
			wantAccepted: true,
			minScore:     1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var substringFilter *SubstringFilter
			var fuzzyFilter *FuzzyFilter

			if tt.substringQuery != "" {
				substringFilter = NewSubstringFilter(tt.substringQuery)
			}
			if tt.fuzzyQuery != "" {
				fuzzyFilter = NewFuzzyFilter(tt.fuzzyQuery, tt.fuzzyThreshold)
			}

			filter := &orFilter{substringFilter: substringFilter, fuzzyFilter: fuzzyFilter}
			accepted, score := filter.Accept(tt.value)

			require.Equal(t, tt.wantAccepted, accepted)
			if tt.wantAccepted {
				require.GreaterOrEqual(t, score, tt.minScore)
			}
		})
	}
}

func TestChainFilter(t *testing.T) {
	type filterSpec struct {
		query       string
		threshold   float64
		isSubstring bool
	}
	tests := []struct {
		name         string
		filters      []filterSpec
		value        string
		wantAccepted bool
		wantScore    float64
	}{
		{
			name: "both filters accept",
			filters: []filterSpec{
				{query: "prom", isSubstring: true},
				{query: "prometheus", threshold: 0.8},
			},
			value:        "prometheus",
			wantAccepted: true,
			wantScore:    1.0,
		},
		{
			name: "first filter rejects",
			filters: []filterSpec{
				{query: "grafana", isSubstring: true},
				{query: "prometheus", threshold: 0.8},
			},
			value:        "prometheus",
			wantAccepted: false,
			wantScore:    0.0,
		},
		{
			name: "second filter rejects",
			filters: []filterSpec{
				{query: "prom", isSubstring: true},
				{query: "grafana", threshold: 0.9},
			},
			value:        "prometheus",
			wantAccepted: false,
			wantScore:    0.0,
		},
		{
			name:         "empty chain accepts all",
			filters:      nil,
			value:        "anything",
			wantAccepted: true,
			wantScore:    1.0,
		},
		{
			name: "max score wins for ranking",
			filters: []filterSpec{
				{query: "prom", isSubstring: true},    // Score: 1.0 (prefix).
				{query: "prometheus", threshold: 0.5}, // Score: 1.0 (exact).
			},
			value:        "prometheus",
			wantAccepted: true,
			wantScore:    1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filters []storage.Filter
			for _, f := range tt.filters {
				if f.isSubstring {
					filters = append(filters, NewSubstringFilter(f.query))
				} else {
					filters = append(filters, NewFuzzyFilter(f.query, f.threshold))
				}
			}

			chain := NewChainFilter(filters...)
			accepted, score := chain.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			require.InDelta(t, tt.wantScore, score, 1e-9)
		})
	}
}
