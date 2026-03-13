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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
)

func TestSubstringFilter(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		caseSensitive bool
		value         string
		wantAccepted  bool
		wantScore     float64
	}{
		{
			name:          "exact match case insensitive",
			query:         "prometheus",
			caseSensitive: false,
			value:         "Prometheus",
			wantAccepted:  true,
			wantScore:     1.0,
		},
		{
			name:          "exact match case sensitive",
			query:         "prometheus",
			caseSensitive: true,
			value:         "prometheus",
			wantAccepted:  true,
			wantScore:     1.0,
		},
		{
			name:          "case sensitive mismatch",
			query:         "prometheus",
			caseSensitive: true,
			value:         "Prometheus",
			wantAccepted:  false,
			wantScore:     0.0,
		},
		{
			name:          "prefix match",
			query:         "prom",
			caseSensitive: false,
			value:         "prometheus",
			wantAccepted:  true,
			wantScore:     1.0,
		},
		{
			name:          "substring match",
			query:         "meth",
			caseSensitive: false,
			value:         "prometheus",
			wantAccepted:  true,
			wantScore:     0.9,
		},
		{
			name:          "no match",
			query:         "grafana",
			caseSensitive: false,
			value:         "prometheus",
			wantAccepted:  false,
			wantScore:     0.0,
		},
		{
			name:          "empty query accepts all",
			query:         "",
			caseSensitive: false,
			value:         "anything",
			wantAccepted:  true,
			wantScore:     1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewSubstringFilter(tt.query, tt.caseSensitive)
			accepted, score := filter.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			require.Equal(t, tt.wantScore, score)
		})
	}
}

func TestFuzzyFilter(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		threshold     float64
		caseSensitive bool
		value         string
		wantAccepted  bool
		minScore      float64 // Minimum expected score.
	}{
		{
			name:          "exact match",
			query:         "prometheus",
			threshold:     0.8,
			caseSensitive: false,
			value:         "prometheus",
			wantAccepted:  true,
			minScore:      1.0,
		},
		{
			name:          "close match above threshold",
			query:         "prometheus",
			threshold:     0.8,
			caseSensitive: false,
			value:         "promethus", // Typo: one char different.
			wantAccepted:  true,
			minScore:      0.8,
		},
		{
			name:          "distant match below threshold",
			query:         "prometheus",
			threshold:     0.8,
			caseSensitive: false,
			value:         "grafana",
			wantAccepted:  false,
			minScore:      0.0,
		},
		{
			name:          "empty query accepts all",
			query:         "",
			threshold:     0.8,
			caseSensitive: false,
			value:         "anything",
			wantAccepted:  true,
			minScore:      1.0,
		},
		{
			name:          "case insensitive match",
			query:         "prometheus",
			threshold:     0.8,
			caseSensitive: false,
			value:         "PROMETHEUS",
			wantAccepted:  true,
			minScore:      1.0,
		},
		{
			name:          "case sensitive mismatch",
			query:         "prometheus",
			threshold:     0.8,
			caseSensitive: true,
			value:         "PROMETHEUS",
			wantAccepted:  false,
			minScore:      0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewFuzzyFilter(tt.query, tt.threshold, tt.caseSensitive)
			accepted, score := filter.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			if tt.wantAccepted {
				require.GreaterOrEqual(t, score, tt.minScore)
			}
		})
	}
}

func TestFuzzyFilterCaching(t *testing.T) {
	filter := NewFuzzyFilter("prometheus", 0.8, false)

	// First call: cache miss.
	accepted1, score1 := filter.Accept("promethus")
	require.True(t, accepted1)
	require.Greater(t, score1, 0.8)

	// Second call: cache hit (should return same score).
	accepted2, score2 := filter.Accept("promethus")
	require.True(t, accepted2)
	require.Equal(t, score1, score2)

	// Verify cache contains the value.
	filter.mu.RLock()
	cachedScore, found := filter.cache["promethus"]
	filter.mu.RUnlock()
	require.True(t, found)
	require.Equal(t, score1, cachedScore)
}

func TestFuzzyFilterConcurrency(t *testing.T) {
	filter := NewFuzzyFilter("prometheus", 0.8, false)
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

	// Verify cache contains all values.
	filter.mu.RLock()
	defer filter.mu.RUnlock()
	require.Len(t, filter.cache, len(values))
	for _, value := range values {
		_, found := filter.cache[value]
		require.True(t, found, "value %s not in cache", value)
	}
}

func TestSubsequenceFilter(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		threshold     float64
		caseSensitive bool
		value         string
		wantAccepted  bool
		wantScore     float64 // -1 means "any positive value".
	}{
		{
			name:          "empty query accepts all",
			query:         "",
			threshold:     0.0,
			caseSensitive: false,
			value:         "anything",
			wantAccepted:  true,
			wantScore:     1.0,
		},
		{
			name:          "exact match",
			query:         "prometheus",
			threshold:     1.0,
			caseSensitive: true,
			value:         "prometheus",
			wantAccepted:  true,
			wantScore:     1.0,
		},
		{
			name:          "prefix match scores 1.0",
			query:         "prom",
			threshold:     1.0,
			caseSensitive: true,
			value:         "prometheus",
			wantAccepted:  true,
			wantScore:     1.0,
		},
		{
			name:          "subsequence match above zero threshold",
			query:         "pms",
			threshold:     0.0,
			caseSensitive: true,
			value:         "prometheus",
			wantAccepted:  true,
			wantScore:     -1,
		},
		{
			name:          "non-subsequence rejected",
			query:         "xyz",
			threshold:     0.0,
			caseSensitive: true,
			value:         "prometheus",
			wantAccepted:  false,
			wantScore:     0.0,
		},
		{
			name:          "case insensitive match",
			query:         "prom",
			threshold:     1.0,
			caseSensitive: false,
			value:         "PROMETHEUS",
			wantAccepted:  true,
			wantScore:     1.0,
		},
		{
			name:          "case sensitive mismatch",
			query:         "prom",
			threshold:     0.0,
			caseSensitive: true,
			value:         "PROMETHEUS",
			wantAccepted:  false,
			wantScore:     0.0,
		},
		{
			name:          "below threshold rejected",
			query:         "pms",
			threshold:     0.99,
			caseSensitive: true,
			value:         "prometheus",
			wantAccepted:  false,
			wantScore:     -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewSubsequenceFilter(tt.query, tt.threshold, tt.caseSensitive)
			accepted, score := filter.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			if tt.wantScore >= 0 {
				require.InDelta(t, tt.wantScore, score, 1e-9)
			}
		})
	}
}

func TestOrFilter(t *testing.T) {
	tests := []struct {
		name           string
		substringQuery string
		fuzzyQuery     string
		fuzzyThreshold float64
		caseSensitive  bool
		value          string
		wantAccepted   bool
		minScore       float64
	}{
		{
			name:           "substring match only",
			substringQuery: "prom",
			caseSensitive:  true,
			value:          "prometheus",
			wantAccepted:   true,
			minScore:       1.0, // Prefix match.
		},
		{
			name:           "substring rejects",
			substringQuery: "prom",
			caseSensitive:  true,
			value:          "node",
			wantAccepted:   false,
		},
		{
			name:           "fuzzy fallback",
			substringQuery: "go_gor",
			fuzzyQuery:     "go_gor",
			fuzzyThreshold: 0.8,
			caseSensitive:  true,
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
				substringFilter = NewSubstringFilter(tt.substringQuery, tt.caseSensitive)
			}
			if tt.fuzzyQuery != "" {
				fuzzyFilter = NewFuzzyFilter(tt.fuzzyQuery, tt.fuzzyThreshold, tt.caseSensitive)
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
	tests := []struct {
		name    string
		filters []struct {
			query         string
			threshold     float64
			caseSensitive bool
			isSubstring   bool
		}
		value        string
		wantAccepted bool
		maxScore     float64 // Maximum expected score (minimum from filters).
	}{
		{
			name: "both filters accept",
			filters: []struct {
				query         string
				threshold     float64
				caseSensitive bool
				isSubstring   bool
			}{
				{query: "prom", isSubstring: true, caseSensitive: false},
				{query: "prometheus", threshold: 0.8, caseSensitive: false},
			},
			value:        "prometheus",
			wantAccepted: true,
			maxScore:     1.0,
		},
		{
			name: "first filter rejects",
			filters: []struct {
				query         string
				threshold     float64
				caseSensitive bool
				isSubstring   bool
			}{
				{query: "grafana", isSubstring: true, caseSensitive: false},
				{query: "prometheus", threshold: 0.8, caseSensitive: false},
			},
			value:        "prometheus",
			wantAccepted: false,
			maxScore:     0.0,
		},
		{
			name: "second filter rejects",
			filters: []struct {
				query         string
				threshold     float64
				caseSensitive bool
				isSubstring   bool
			}{
				{query: "prom", isSubstring: true, caseSensitive: false},
				{query: "grafana", threshold: 0.9, caseSensitive: false},
			},
			value:        "prometheus",
			wantAccepted: false,
			maxScore:     0.0,
		},
		{
			name: "empty chain accepts all",
			filters: []struct {
				query         string
				threshold     float64
				caseSensitive bool
				isSubstring   bool
			}{},
			value:        "anything",
			wantAccepted: true,
			maxScore:     1.0,
		},
		{
			name: "minimum score from multiple filters",
			filters: []struct {
				query         string
				threshold     float64
				caseSensitive bool
				isSubstring   bool
			}{
				{query: "prom", isSubstring: true, caseSensitive: false},    // Score: 1.0 (prefix).
				{query: "prometheus", threshold: 0.5, caseSensitive: false}, // Score: 1.0 (exact).
			},
			value:        "prometheus",
			wantAccepted: true,
			maxScore:     1.0, // Both return 1.0, minimum is 1.0.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filters []storage.Filter
			for _, f := range tt.filters {
				if f.isSubstring {
					filters = append(filters, NewSubstringFilter(f.query, f.caseSensitive))
				} else {
					filters = append(filters, NewFuzzyFilter(f.query, f.threshold, f.caseSensitive))
				}
			}

			chain := NewChainFilter(filters...)
			accepted, score := chain.Accept(tt.value)
			require.Equal(t, tt.wantAccepted, accepted)
			if tt.wantAccepted {
				require.LessOrEqual(t, score, tt.maxScore+0.01) // Allow small floating point error.
			} else {
				require.Equal(t, 0.0, score)
			}
		})
	}
}
