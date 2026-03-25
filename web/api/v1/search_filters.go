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
	"strings"
	"sync"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/strutil"
)

// SubstringFilter implements fast case-insensitive substring matching.
// Returns true if the query is a substring of the value, with score based on match quality:
// - Exact match: score 1.0
// - Prefix match: score 0.8
// - Substring match: score 0.6
// - No match: score 0.0.
type SubstringFilter struct {
	query         string
	caseSensitive bool
}

// NewSubstringFilter creates a new SubstringFilter.
func NewSubstringFilter(query string, caseSensitive bool) *SubstringFilter {
	if !caseSensitive {
		query = strings.ToLower(query)
	}
	return &SubstringFilter{
		query:         query,
		caseSensitive: caseSensitive,
	}
}

// Accept returns true if the value matches the substring query.
func (f *SubstringFilter) Accept(value string) (bool, float64) {
	if f.query == "" {
		return true, 1.0
	}

	compareValue := value
	if !f.caseSensitive {
		compareValue = strings.ToLower(value)
	}

	// Substring match - check contains first.
	if strings.Contains(compareValue, f.query) {
		// Prefix match scores highest for autocomplete use cases.
		if strings.HasPrefix(compareValue, f.query) {
			return true, 1.0
		}
		return true, 0.9
	}

	return false, 0.0
}

// FuzzyFilter implements Jaro-Winkler fuzzy matching with score caching.
// The cache is scoped to a single filter instance (per-query lifecycle) and uses
// sync.RWMutex for concurrent access safety.
type FuzzyFilter struct {
	query         string
	threshold     float64
	caseSensitive bool
	mu            sync.RWMutex
	cache         map[string]float64
}

// NewFuzzyFilter creates a new FuzzyFilter.
// threshold should be in range [0.0, 1.0] where 1.0 requires perfect match.
func NewFuzzyFilter(query string, threshold float64, caseSensitive bool) *FuzzyFilter {
	if !caseSensitive {
		query = strings.ToLower(query)
	}
	return &FuzzyFilter{
		query:         query,
		threshold:     threshold,
		caseSensitive: caseSensitive,
		cache:         make(map[string]float64),
	}
}

// Accept returns true if the value matches the fuzzy query above the threshold.
// The Jaro-Winkler score is cached to avoid recomputation.
func (f *FuzzyFilter) Accept(value string) (bool, float64) {
	if f.query == "" {
		return true, 1.0
	}

	compareValue := value
	if !f.caseSensitive {
		compareValue = strings.ToLower(value)
	}

	// Check cache first (read lock).
	f.mu.RLock()
	score, cached := f.cache[compareValue]
	f.mu.RUnlock()

	if cached {
		return score >= f.threshold, score
	}

	// Compute Jaro-Winkler score.
	score = strutil.JaroWinkler(f.query, compareValue)

	// Store in cache (write lock).
	f.mu.Lock()
	f.cache[compareValue] = score
	f.mu.Unlock()

	return score >= f.threshold, score
}

// SubsequenceFilter implements fuzzy matching using a sequential character
// matching algorithm. It requires all pattern characters to appear in the text
// in order (subsequence matching), and scores matches by rewarding consecutive
// character runs and penalizing gaps.
// The score is normalized to [0.0, 1.0] and compared against a threshold.
type SubsequenceFilter struct {
	query         string
	threshold     float64
	caseSensitive bool
}

// NewSubsequenceFilter creates a new SubsequenceFilter.
// threshold should be in range [0.0, 1.0] where 0.0 accepts any subsequence match.
func NewSubsequenceFilter(query string, threshold float64, caseSensitive bool) *SubsequenceFilter {
	if !caseSensitive {
		query = strings.ToLower(query)
	}
	return &SubsequenceFilter{
		query:         query,
		threshold:     threshold,
		caseSensitive: caseSensitive,
	}
}

// Accept returns true if the value matches the subsequence query above the threshold.
// Prefix matches always score 1.0 for consistency with SubstringFilter.
func (f *SubsequenceFilter) Accept(value string) (bool, float64) {
	if f.query == "" {
		return true, 1.0
	}

	compareValue := value
	if !f.caseSensitive {
		compareValue = strings.ToLower(value)
	}

	// Prefix match always scores 1.0.
	if strings.HasPrefix(compareValue, f.query) {
		return true, 1.0
	}

	score := strutil.SubsequenceScore(f.query, compareValue)
	// score == 0 means no subsequence match; always reject regardless of threshold.
	return score > 0 && score >= f.threshold, score
}

// ChainFilter combines multiple filters with AND logic.
// Returns true only if all filters accept the value.
// The returned score is the minimum score from all filters.
type ChainFilter struct {
	filters []storage.Filter
}

// NewChainFilter creates a new ChainFilter.
func NewChainFilter(filters ...storage.Filter) *ChainFilter {
	return &ChainFilter{
		filters: filters,
	}
}

// Accept returns true if all filters accept the value.
// Returns the minimum score from all filters.
func (f *ChainFilter) Accept(value string) (bool, float64) {
	if len(f.filters) == 0 {
		return true, 1.0
	}

	minScore := 1.0
	for _, filter := range f.filters {
		accepted, score := filter.Accept(value)
		if !accepted {
			return false, 0.0
		}
		if score < minScore {
			minScore = score
		}
	}

	return true, minScore
}

// orSearchesFilter combines multiple per-term filters with OR logic.
// Returns true if any term filter accepts the value.
// The returned score is the maximum score from all accepting filters.
type orSearchesFilter struct {
	filters []storage.Filter
}

func newOrSearchesFilter(filters ...storage.Filter) *orSearchesFilter {
	return &orSearchesFilter{filters: filters}
}

// Accept returns true if any of the per-term filters accepts the value.
func (f *orSearchesFilter) Accept(value string) (bool, float64) {
	var maxScore float64
	accepted := false
	for _, filter := range f.filters {
		if ok, score := filter.Accept(value); ok {
			accepted = true
			if score > maxScore {
				maxScore = score
			}
		}
	}
	return accepted, maxScore
}

// orFilter combines substring and fuzzy filters with OR logic.
// Tries substring first, then fuzzy if substring doesn't match.
// This matches the original matchName behavior.
type orFilter struct {
	substringFilter *SubstringFilter
	fuzzyFilter     *FuzzyFilter
}

// Accept returns true if either substring or fuzzy filter accepts.
func (f *orFilter) Accept(value string) (bool, float64) {
	// If no filters, accept all.
	if f.substringFilter == nil && f.fuzzyFilter == nil {
		return true, 1.0
	}

	// Try substring first.
	if f.substringFilter != nil {
		accepted, score := f.substringFilter.Accept(value)
		if accepted {
			return true, score
		}
	}

	// Fall back to fuzzy if available.
	if f.fuzzyFilter != nil {
		return f.fuzzyFilter.Accept(value)
	}

	// No filter accepted.
	return false, 0.0
}
