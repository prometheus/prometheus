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
	"unicode/utf8"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/strutil"
)

// Filters in this file expect the query passed to their constructor and the
// value passed to Accept to share the same case. To search case-insensitively,
// lowercase the query at construction time and wrap the resulting filter with
// caseFoldingFilter so incoming values are lowercased once for the whole chain.

// SubstringFilter implements case-sensitive substring matching with a
// position-based score. A prefix match scores 1.0; substring matches score in
// the range [0.1, 1.0) where earlier match positions score higher.
type SubstringFilter struct {
	query string
}

// NewSubstringFilter creates a new SubstringFilter. The query is matched
// against incoming values in their literal case.
func NewSubstringFilter(query string) *SubstringFilter {
	return &SubstringFilter{query: query}
}

// Accept returns true if the value contains the substring query, with a score
// that decreases as the match position moves toward the end of the value.
//
// The position component is computed in characters (runes), not bytes, so a
// match at character offset 1 in a multi-byte string scores the same as a
// match at byte offset 1 in an ASCII string. An ASCII fast path keeps the
// common case at byte arithmetic.
func (f *SubstringFilter) Accept(value string) (bool, float64) {
	if f.query == "" {
		return true, 1.0
	}
	idx := strings.Index(value, f.query)
	if idx < 0 {
		return false, 0.0
	}
	if idx == 0 {
		return true, 1.0
	}
	var pos, maxPos int
	// The fast path requires the entire value to be ASCII: if any byte
	// after the match is multi-byte, len(value) overcounts characters and
	// inflates the score. Checking the whole string is O(len(value)) but
	// short-circuits on the first non-ASCII byte, so the typical metric
	// name pays only a tight byte-loop.
	if isASCII(f.query) && isASCII(value) {
		pos = idx
		maxPos = len(value) - len(f.query)
	} else {
		pos = utf8.RuneCountInString(value[:idx])
		maxPos = utf8.RuneCountInString(value) - utf8.RuneCountInString(f.query)
	}
	if maxPos <= 0 {
		// Defensive: maxPos==0 would imply value and query have equal
		// rune counts, which forces idx==0 and is handled above. Treat
		// any unreachable case as a perfect match rather than dividing
		// by zero.
		return true, 1.0
	}
	// Scale to [0.1, 1.0). Earlier positions score closer to 1.0.
	score := 1.0 - 0.9*float64(pos)/float64(maxPos)
	return true, score
}

// isASCII reports whether s contains only ASCII bytes (< 0x80).
func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// FuzzyFilter implements Jaro-Winkler fuzzy matching against a query.
type FuzzyFilter struct {
	query     string
	matcher   *strutil.JaroWinklerMatcher
	threshold float64
}

// NewFuzzyFilter creates a new FuzzyFilter.
// threshold should be in range [0.0, 1.0] where 1.0 requires perfect match.
func NewFuzzyFilter(query string, threshold float64) *FuzzyFilter {
	return &FuzzyFilter{
		query:     query,
		matcher:   strutil.NewJaroWinklerMatcher(query),
		threshold: threshold,
	}
}

// Accept returns true if the value matches the fuzzy query above the threshold.
func (f *FuzzyFilter) Accept(value string) (bool, float64) {
	score := f.matcher.Score(value)
	return score >= f.threshold, score
}

// SubsequenceFilter implements fuzzy matching using a sequential character
// matching algorithm. It requires all pattern characters to appear in the value
// in order (subsequence matching), and scores matches by rewarding consecutive
// character runs and penalizing gaps.
// The score is normalized to [0.0, 1.0] and compared against a threshold.
type SubsequenceFilter struct {
	query     string
	matcher   *strutil.SubsequenceMatcher
	threshold float64
}

// NewSubsequenceFilter creates a new SubsequenceFilter.
// threshold should be in range [0.0, 1.0] where 0.0 accepts any subsequence match.
func NewSubsequenceFilter(query string, threshold float64) *SubsequenceFilter {
	return &SubsequenceFilter{
		query:     query,
		matcher:   strutil.NewSubsequenceMatcher(query),
		threshold: threshold,
	}
}

// Accept returns true if the value matches the subsequence query above the threshold.
// Prefix matches always score 1.0 for consistency with SubstringFilter.
func (f *SubsequenceFilter) Accept(value string) (bool, float64) {
	if strings.HasPrefix(value, f.query) {
		return true, 1.0
	}
	score := f.matcher.Score(value)
	// score == 0 means no subsequence match; always reject regardless of threshold.
	return score > 0 && score >= f.threshold, score
}

// caseFoldingFilter wraps another Filter and lowercases the value once before
// delegating, so a chain of case-insensitive matchers does not each repeat the
// case fold. The wrapped filter must have been constructed with a lowercased
// query.
type caseFoldingFilter struct {
	inner storage.Filter
}

func newCaseFoldingFilter(inner storage.Filter) *caseFoldingFilter {
	return &caseFoldingFilter{inner: inner}
}

// Accept lowercases the value and delegates to the inner filter.
func (f *caseFoldingFilter) Accept(value string) (bool, float64) {
	return f.inner.Accept(strings.ToLower(value))
}

// memoEntry stores a cached filter result.
type memoEntry struct {
	accepted bool
	score    float64
}

// memoizingFilterMaxEntries and memoizingFilterMaxBytes together cap the
// memoization cache. Past either cap, lookups fall through to the inner
// filter without populating the cache further. The entry cap protects
// against tiny-value blowup (millions of short distinct values); the byte
// cap protects against long-value blowup (a label of URLs or user agents
// where 100k entries would otherwise hold tens of megabytes). Bytes are
// counted as the sum of cached key lengths — a coarse but cheap approximation
// of map memory.
const (
	memoizingFilterMaxEntries = 100_000
	memoizingFilterMaxBytes   = 10 << 20 // 10 MiB.
)

// memoizingFilter caches the (accepted, score) returned by the inner filter
// for each distinct value. It is intended to be used as the outermost wrapper
// in buildSearchFilter so that values reaching the chain multiple times in a
// single search (e.g. once per TSDB block during a multi-block lookup) are
// scored only once.
//
// memoizingFilter is not safe for concurrent use: the search path drives
// SearchResultSet iteration from a single goroutine (the merge driver pulls
// child sets sequentially and ApplySearchHints runs synchronously inside one
// Searcher). Adding a cache lock would charge every Accept on the hot path
// without buying anything, so the lock is omitted.
type memoizingFilter struct {
	inner storage.Filter
	cache map[string]memoEntry
	bytes int // Sum of len(key) for currently cached entries.
}

func newMemoizingFilter(inner storage.Filter) *memoizingFilter {
	return &memoizingFilter{
		inner: inner,
		cache: make(map[string]memoEntry),
	}
}

// Accept returns the cached result for value, computing and caching it on miss.
// Once the cache reaches either memoizingFilterMaxEntries or
// memoizingFilterMaxBytes, the inner filter is called directly without
// populating the cache further; this preserves correctness while bounding
// memory.
func (f *memoizingFilter) Accept(value string) (bool, float64) {
	if e, ok := f.cache[value]; ok {
		return e.accepted, e.score
	}
	accepted, score := f.inner.Accept(value)
	if len(f.cache) < memoizingFilterMaxEntries && f.bytes+len(value) <= memoizingFilterMaxBytes {
		f.cache[value] = memoEntry{accepted: accepted, score: score}
		f.bytes += len(value)
	}
	return accepted, score
}

// ChainFilter combines multiple filters with AND logic.
// Returns true only if all filters accept the value.
// The returned score is the best (max) score across the filters, so that
// rankings reflect the strongest matching dimension.
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
// Returns the maximum score from all filters.
func (f *ChainFilter) Accept(value string) (bool, float64) {
	if len(f.filters) == 0 {
		return true, 1.0
	}

	var maxScore float64
	for _, filter := range f.filters {
		accepted, score := filter.Accept(value)
		if !accepted {
			return false, 0.0
		}
		if score > maxScore {
			maxScore = score
		}
	}

	return true, maxScore
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
// Stops iterating once a perfect (1.0) score is found.
func (f *orSearchesFilter) Accept(value string) (bool, float64) {
	var maxScore float64
	accepted := false
	for _, filter := range f.filters {
		ok, score := filter.Accept(value)
		if !ok {
			continue
		}
		accepted = true
		if score > maxScore {
			maxScore = score
		}
		if maxScore >= 1.0 {
			return true, maxScore
		}
	}
	return accepted, maxScore
}

// orFilter combines substring and fuzzy filters with OR logic.
// Tries substring first, then fuzzy if substring doesn't match.
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
