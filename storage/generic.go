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

// This file holds boilerplate adapters for generic MergeSeriesSet and MergeQuerier functions, so we can have one optimized
// solution that works for both ChunkSeriesSet as well as SeriesSet.

package storage

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/annotations"
)

// querierAdapter must implement the Searcher interface.
var _ Searcher = &querierAdapter{}

type genericQuerier interface {
	LabelQuerier
	Select(context.Context, bool, *SelectHints, ...*labels.Matcher) genericSeriesSet
}

type genericSeriesSet interface {
	Next() bool
	At() Labels
	Err() error
	Warnings() annotations.Annotations
}

type genericSeriesMergeFunc func(...Labels) Labels

type genericSeriesSetAdapter struct {
	SeriesSet
}

func (a *genericSeriesSetAdapter) At() Labels {
	return a.SeriesSet.At()
}

type genericChunkSeriesSetAdapter struct {
	ChunkSeriesSet
}

func (a *genericChunkSeriesSetAdapter) At() Labels {
	return a.ChunkSeriesSet.At()
}

type genericQuerierAdapter struct {
	LabelQuerier

	// One-of. If both are set, Querier will be used.
	q  Querier
	cq ChunkQuerier
}

func (q *genericQuerierAdapter) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) genericSeriesSet {
	if q.q != nil {
		return &genericSeriesSetAdapter{q.q.Select(ctx, sortSeries, hints, matchers...)}
	}
	return &genericChunkSeriesSetAdapter{q.cq.Select(ctx, sortSeries, hints, matchers...)}
}

func newGenericQuerierFrom(q Querier) genericQuerier {
	return &genericQuerierAdapter{LabelQuerier: q, q: q}
}

func newGenericQuerierFromChunk(cq ChunkQuerier) genericQuerier {
	return &genericQuerierAdapter{LabelQuerier: cq, cq: cq}
}

type querierAdapter struct {
	genericQuerier
}

type seriesSetAdapter struct {
	genericSeriesSet
}

func (a *seriesSetAdapter) At() Series {
	return a.genericSeriesSet.At().(Series)
}

func (q *querierAdapter) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) SeriesSet {
	return &seriesSetAdapter{q.genericQuerier.Select(ctx, sortSeries, hints, matchers...)}
}

type chunkQuerierAdapter struct {
	genericQuerier
}

type chunkSeriesSetAdapter struct {
	genericSeriesSet
}

func (a *chunkSeriesSetAdapter) At() ChunkSeries {
	return a.genericSeriesSet.At().(ChunkSeries)
}

func (q *chunkQuerierAdapter) Select(ctx context.Context, sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) ChunkSeriesSet {
	return &chunkSeriesSetAdapter{q.genericQuerier.Select(ctx, sortSeries, hints, matchers...)}
}

type seriesMergerAdapter struct {
	VerticalSeriesMergeFunc
}

func (a *seriesMergerAdapter) Merge(s ...Labels) Labels {
	buf := make([]Series, 0, len(s))
	for _, ser := range s {
		buf = append(buf, ser.(Series))
	}
	return a.VerticalSeriesMergeFunc(buf...)
}

type chunkSeriesMergerAdapter struct {
	VerticalChunkSeriesMergeFunc
}

func (a *chunkSeriesMergerAdapter) Merge(s ...Labels) Labels {
	buf := make([]ChunkSeries, 0, len(s))
	for _, ser := range s {
		buf = append(buf, ser.(ChunkSeries))
	}
	return a.VerticalChunkSeriesMergeFunc(buf...)
}

// searcherFromGenericQuerier extracts a Searcher from a genericQuerierAdapter.
// Falls back to a direct Searcher assertion if the querier is not a
// genericQuerierAdapter.
func searcherFromGenericQuerier(gq genericQuerier) (Searcher, bool) {
	if a, ok := gq.(*genericQuerierAdapter); ok {
		s, ok := a.q.(Searcher)
		return s, ok
	}
	s, ok := gq.(Searcher)
	return s, ok
}

// collectSearchers extracts Searcher implementations from a genericQuerier tree.
func collectSearchers(gq genericQuerier) []Searcher {
	if m, ok := gq.(*mergeGenericQuerier); ok {
		var searchers []Searcher
		for _, q := range m.queriers {
			searchers = append(searchers, collectSearchers(q)...)
		}
		return searchers
	}
	if s, ok := searcherFromGenericQuerier(gq); ok {
		return []Searcher{s}
	}
	return nil
}

// sliceSearchResultSet is a SearchResultSet backed by a pre-built slice.
type sliceSearchResultSet struct {
	results  []SearchResult
	warnings annotations.Annotations
	idx      int
}

func (s *sliceSearchResultSet) Next() bool {
	s.idx++
	return s.idx < len(s.results)
}

func (s *sliceSearchResultSet) At() SearchResult { return s.results[s.idx] }

func (s *sliceSearchResultSet) Warnings() annotations.Annotations { return s.warnings }

func (*sliceSearchResultSet) Err() error { return nil }

func (*sliceSearchResultSet) Close() error { return nil }

// NewSearchResultSetFromSlice returns a SearchResultSet that iterates over the given slice.
func NewSearchResultSetFromSlice(results []SearchResult, warns annotations.Annotations) SearchResultSet {
	return &sliceSearchResultSet{results: results, warnings: warns, idx: -1}
}

// errAfterSliceSet yields a fixed slice of results and then surfaces err once
// the slice is exhausted. It is the partial-results-then-error counterpart to
// sliceSearchResultSet.
type errAfterSliceSet struct {
	results  []SearchResult
	warnings annotations.Annotations
	idx      int
	err      error
}

func (s *errAfterSliceSet) Next() bool {
	s.idx++
	return s.idx < len(s.results)
}

func (s *errAfterSliceSet) At() SearchResult { return s.results[s.idx] }

func (s *errAfterSliceSet) Warnings() annotations.Annotations { return s.warnings }

func (s *errAfterSliceSet) Err() error {
	if s.idx >= len(s.results) {
		return s.err
	}
	return nil
}

func (*errAfterSliceSet) Close() error { return nil }

// NewSearchResultSetFromSliceAndError returns a SearchResultSet that iterates
// the given slice and, once exhausted, exposes err via Err(). Any warnings
// accumulated by the upstream are surfaced via Warnings(). It models a backend
// that produced partial output and warnings before its underlying iterator
// failed, which is common when a remote source returns a stream that aborts
// mid-flight. Callers should use this rather than ad-hoc test fakes so the
// error-surfacing behaviour stays consistent with the merge contract.
func NewSearchResultSetFromSliceAndError(results []SearchResult, warns annotations.Annotations, err error) SearchResultSet {
	return &errAfterSliceSet{results: results, warnings: warns, err: err, idx: -1}
}

// minLinearAllocCap is the floor for the linear-path result capacity hint when
// a Filter is active. It avoids degenerate growth for tiny limits while still
// keeping the upfront allocation small for sparse matches against large indices.
const minLinearAllocCap = 256

// ApplySearchHints filters, sorts, and limits a slice of string values according to hints,
// returning scored SearchResult entries. A nil hints value is treated as the zero value.
// The input values slice is assumed to be ordered ascending by value; the function only
// performs extra work for orderings that differ from this.
//
// Allocation and ordering are tuned to the (Filter, OrderBy, Limit) combination:
//   - Filter == nil: at most Limit entries are copied; OrderByValueDesc walks the input
//     in reverse so we never materialise the full slice when limited.
//   - OrderByValueAsc + Filter + Limit: stream-filter with early exit at Limit matches.
//   - OrderByValueDesc + Filter + Limit: reverse stream-filter with early exit at Limit
//     matches, taking advantage of the input being ascending so the tail is the largest.
//   - OrderByScoreDesc + Filter + Limit: top-K min-heap of size Limit, avoiding a full
//     sort over the matched set.
//   - Other combinations fall back to filter-then-reorder-then-slice, with the upfront
//     capacity capped by min(len(values), max(2*Limit, minLinearAllocCap)).
func ApplySearchHints(values []string, hints *SearchHints) []SearchResult {
	if hints == nil {
		hints = &SearchHints{}
	}
	if hints.Filter == nil {
		return applySearchHintsNoFilter(values, hints)
	}
	if hints.Limit > 0 {
		switch hints.OrderBy {
		case OrderByScoreDesc:
			return topKByScore(values, hints.Filter, hints.Limit)
		case OrderByValueDesc:
			return reverseFilterEarlyExit(values, hints.Filter, hints.Limit)
		}
	}
	return applySearchHintsLinear(values, hints)
}

// reverseFilterEarlyExit walks the input ascending-sorted slice from the tail,
// accepting up to limit matches. Because the input is ascending, the tail
// holds the lex-largest entries, so iterating in reverse yields results in
// descending order without an extra sort.
func reverseFilterEarlyExit(values []string, filter Filter, limit int) []SearchResult {
	results := make([]SearchResult, 0, min(limit, len(values)))
	for i := len(values) - 1; i >= 0 && len(results) < limit; i-- {
		accepted, score := filter.Accept(values[i])
		if !accepted {
			continue
		}
		results = append(results, SearchResult{Value: values[i], Score: score})
	}
	return results
}

// applySearchHintsNoFilter handles the unfiltered path: scores are uniformly 1.0
// and at most Limit entries are emitted in the requested order.
func applySearchHintsNoFilter(values []string, hints *SearchHints) []SearchResult {
	n := len(values)
	if hints.Limit > 0 && hints.Limit < n {
		n = hints.Limit
	}
	results := make([]SearchResult, 0, n)
	if hints.OrderBy == OrderByValueDesc {
		// Walk the input in reverse so we keep the largest-Value entries
		// without materialising the full slice when limited. The i >= 0
		// guard is defensive: n is clamped to len(values) above, so we
		// should always exit on len(results) == n first.
		for i := len(values) - 1; i >= 0 && len(results) < n; i-- {
			results = append(results, SearchResult{Value: values[i], Score: 1.0})
		}
		return results
	}
	// OrderByValueAsc and OrderByScoreDesc both reduce to value-ascending here:
	// uniform scores tie-break on Value asc under (Score desc, Value asc).
	for i := range n {
		results = append(results, SearchResult{Value: values[i], Score: 1.0})
	}
	return results
}

// applySearchHintsLinear handles the filtered path for orderings other than
// OrderByScoreDesc-with-limit (which uses top-K). It streams the filter and,
// for OrderByValueAsc with a limit, exits as soon as the limit is reached.
func applySearchHintsLinear(values []string, hints *SearchHints) []SearchResult {
	results := make([]SearchResult, 0, linearResultCap(len(values), hints.Limit))
	earlyExit := hints.OrderBy == OrderByValueAsc && hints.Limit > 0
	for _, v := range values {
		accepted, score := hints.Filter.Accept(v)
		if !accepted {
			continue
		}
		results = append(results, SearchResult{Value: v, Score: score})
		if earlyExit && len(results) >= hints.Limit {
			break
		}
	}
	switch hints.OrderBy {
	case OrderByValueDesc:
		slices.Reverse(results)
	case OrderByScoreDesc:
		// Reached only when Limit == 0; ApplySearchHints routes
		// OrderByScoreDesc + Limit > 0 to topKByScore instead.
		slices.SortFunc(results, compareSearchResults(OrderByScoreDesc))
	}
	if hints.Limit > 0 && len(results) > hints.Limit {
		results = results[:hints.Limit]
	}
	return results
}

// linearResultCap returns the upfront capacity hint for the linear-path result
// slice. We cannot know the filter selectivity ahead of time, so we use 2*Limit
// as a heuristic for the expected match count and floor it at minLinearAllocCap;
// it is always bounded by len(values).
func linearResultCap(numValues, limit int) int {
	if limit <= 0 {
		return numValues
	}
	// Defensive overflow guard: 2*limit can wrap when an untrusted limit
	// value is in the upper int range (only reachable when the operator
	// disabled --web.search.max-limit). Fall back to the small-allocation
	// floor instead of numValues so a sparse filter does not cause a
	// multi-MB upfront allocation; append amortizes the growth from
	// there.
	allocCap := 2 * limit
	if allocCap < limit {
		return minLinearAllocCap
	}
	if allocCap < minLinearAllocCap {
		allocCap = minLinearAllocCap
	}
	if allocCap > numValues {
		allocCap = numValues
	}
	return allocCap
}

// topKByScore returns the top-K matches under the (Score desc, Value asc) total
// order, using a min-heap of size limit. This avoids sorting the full matched
// set when only the best Limit results are needed.
//
// The heap is a small typed structure (no container/heap interface) so each
// candidate replacement is a direct struct write rather than an interface
// box. This keeps the hot loop allocation-free past the initial fill.
func topKByScore(values []string, filter Filter, limit int) []SearchResult {
	h := make(searchTopKHeap, 0, min(limit, len(values)))
	for _, v := range values {
		accepted, score := filter.Accept(v)
		if !accepted {
			continue
		}
		if len(h) < limit {
			h = h.push(SearchResult{Value: v, Score: score})
			continue
		}
		// The heap minimum is the worst entry currently kept. Three cases:
		//   - Lower score: cannot improve, skip without a string compare.
		//   - Higher score: definitely better, replace.
		//   - Tied score:   keep the lex-smaller Value.
		worst := h[0]
		switch {
		case score < worst.Score:
			continue
		case score > worst.Score:
			h[0] = SearchResult{Value: v, Score: score}
			h.siftDown(0)
		case v < worst.Value:
			h[0] = SearchResult{Value: v, Score: score}
			h.siftDown(0)
		}
	}
	out := make([]SearchResult, len(h))
	// Pop returns worst-first under our heap order; place results from the tail
	// so the final slice is best-first (Score desc, Value asc).
	for i := len(out) - 1; i >= 0; i-- {
		var r SearchResult
		r, h = h.pop()
		out[i] = r
	}
	return out
}

// searchTopKHeap is a typed binary min-heap under the inverse of the (Score
// desc, Value asc) total order, so heap[0] is the worst entry currently kept.
// Replacing the minimum on better candidates keeps the K best entries without
// a full sort and without the per-operation interface boxing that
// container/heap would introduce.
type searchTopKHeap []SearchResult

// less reports whether index i should sift above index j in the heap. The
// "lighter" entry (lower score, or higher Value on ties) sits at the root.
func (h searchTopKHeap) less(i, j int) bool {
	if h[i].Score != h[j].Score {
		return h[i].Score < h[j].Score
	}
	return h[i].Value > h[j].Value
}

func (h searchTopKHeap) push(r SearchResult) searchTopKHeap {
	h = append(h, r)
	h.siftUp(len(h) - 1)
	return h
}

func (h searchTopKHeap) pop() (SearchResult, searchTopKHeap) {
	n := len(h) - 1
	out := h[0]
	h[0] = h[n]
	h = h[:n]
	if n > 0 {
		h.siftDown(0)
	}
	return out, h
}

func (h searchTopKHeap) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			return
		}
		h[i], h[parent] = h[parent], h[i]
		i = parent
	}
}

func (h searchTopKHeap) siftDown(i int) {
	n := len(h)
	for {
		left := 2*i + 1
		if left >= n {
			return
		}
		smallest := left
		if right := left + 1; right < n && h.less(right, left) {
			smallest = right
		}
		if !h.less(smallest, i) {
			return
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}

// compareSearchResults returns the total-order comparison function for the
// given Ordering. For OrderByValueAsc and OrderByValueDesc the order is on
// Value alone. For OrderByScoreDesc the order is (Score desc, Value asc),
// which is a total order and defines the position at which a duplicate value
// is first emitted by the streaming merge.
func compareSearchResults(o Ordering) func(a, b SearchResult) int {
	switch o {
	case OrderByValueDesc:
		return func(a, b SearchResult) int { return cmp.Compare(b.Value, a.Value) }
	case OrderByScoreDesc:
		return func(a, b SearchResult) int {
			if c := cmp.Compare(b.Score, a.Score); c != 0 {
				return c
			}
			return cmp.Compare(a.Value, b.Value)
		}
	default:
		return func(a, b SearchResult) int { return cmp.Compare(a.Value, b.Value) }
	}
}

// mergeSearchSets merges results from multiple Searcher calls using a streaming
// pairwise k-way merge. Each searcher is required to emit results in the order
// requested by hints.OrderBy; the merge deduplicates by value so that a value
// appearing in several sources is emitted once, carrying its highest score.
func mergeSearchSets(hints *SearchHints, fn func(Searcher) SearchResultSet, searchers []Searcher) SearchResultSet {
	if len(searchers) == 0 {
		return EmptySearchResultSet()
	}

	sets := make([]SearchResultSet, len(searchers))
	for i, s := range searchers {
		sets[i] = &lazySearchResultSet{init: func() SearchResultSet { return fn(s) }}
	}
	var (
		order Ordering
		limit int
	)
	if hints != nil {
		order = hints.OrderBy
		limit = hints.Limit
	}

	// Duplicate Values collapse in place inside mergingSearchResultSet.
	// Under value-based orderings they are trivially adjacent. Under
	// OrderByScoreDesc the Searcher contract requires identical scores
	// for a given Value, so duplicates tie on (Score, Value) and are
	// adjacent there too.
	return pairwiseMergeSearchSets(sets, order, limit)
}

// MergeSearchResultSets merges pre-sorted SearchResultSets into a single set
// according to hints.OrderBy and hints.Limit. A nil hints value is treated as
// the zero value (OrderByValueAsc, no limit).
//
// All inputs must yield results in the requested order. Duplicates collapse in
// place: under value-based orderings the higher score wins; under
// OrderByScoreDesc the Searcher contract requires identical scores for a given
// Value, so duplicates tie on (Score, Value) and are adjacent.
//
// The returned set owns all inputs: the caller closes the returned set exactly
// once and must not close the inputs separately. If a single input errors, the
// merge keeps draining the surviving inputs and surfaces the joined error via
// Err() once iteration ends.
//
// MergeSearchResultSets does not lazily construct its inputs: each set in
// `sets` is taken as already opened, since SearchResultSet construction is
// the caller's responsibility. Callers that hold Searcher instances and want
// to defer SearchLabel* calls until the result is actually consumed should
// wrap each input in their own lazy SearchResultSet (the storage package
// uses an internal lazy wrapper for that path).
func MergeSearchResultSets(sets []SearchResultSet, hints *SearchHints) SearchResultSet {
	var (
		order Ordering
		limit int
	)
	if hints != nil {
		order = hints.OrderBy
		limit = hints.Limit
	}
	return pairwiseMergeSearchSets(sets, order, limit)
}

// pairwiseMergeSearchSets recursively merges SearchResultSets in a balanced
// binary tree. Each merge node respects the requested ordering and stops after
// emitting limit results, enabling early termination that avoids consuming the
// full input from child nodes.
func pairwiseMergeSearchSets(sets []SearchResultSet, order Ordering, limit int) SearchResultSet {
	switch len(sets) {
	case 0:
		return EmptySearchResultSet()
	case 1:
		if limit > 0 {
			return &limitSearchResultSet{rs: sets[0], limit: limit}
		}
		return sets[0]
	default:
		mid := len(sets) / 2
		left := pairwiseMergeSearchSets(sets[:mid], order, limit)
		right := pairwiseMergeSearchSets(sets[mid:], order, limit)
		return newMergingSearchResultSet(left, right, order, limit)
	}
}

// NewLazySearchResultSet returns a SearchResultSet that defers calling init
// until the first Next, At, Warnings, Err, or Close. It is intended for
// callers of MergeSearchResultSets that want to amortize sub-query
// construction cost across a merge tree with an early-terminating limit:
// branches that the merge never pulls from incur no construction cost.
func NewLazySearchResultSet(init func() SearchResultSet) SearchResultSet {
	return &lazySearchResultSet{init: init}
}

// lazySearchResultSet defers the creation of a SearchResultSet until the first
// call to Next. This avoids invoking searchers whose results are never consumed.
type lazySearchResultSet struct {
	init func() SearchResultSet
	rs   SearchResultSet
}

func (s *lazySearchResultSet) ensure() {
	if s.rs == nil {
		s.rs = s.init()
		s.init = nil
	}
}

func (s *lazySearchResultSet) Next() bool {
	s.ensure()
	return s.rs.Next()
}

func (s *lazySearchResultSet) At() SearchResult {
	if s.rs == nil {
		return SearchResult{}
	}
	return s.rs.At()
}

func (s *lazySearchResultSet) Warnings() annotations.Annotations {
	if s.rs == nil {
		return nil
	}
	return s.rs.Warnings()
}

func (s *lazySearchResultSet) Err() error {
	if s.rs == nil {
		return nil
	}
	return s.rs.Err()
}

func (s *lazySearchResultSet) Close() error {
	if s.rs == nil {
		return nil
	}
	return s.rs.Close()
}

// limitSearchResultSet wraps a SearchResultSet and stops after limit results.
type limitSearchResultSet struct {
	rs      SearchResultSet
	limit   int
	emitted int
}

func (s *limitSearchResultSet) Next() bool {
	if s.limit > 0 && s.emitted >= s.limit {
		return false
	}
	if s.rs.Next() {
		s.emitted++
		return true
	}
	return false
}

func (s *limitSearchResultSet) At() SearchResult                  { return s.rs.At() }
func (s *limitSearchResultSet) Warnings() annotations.Annotations { return s.rs.Warnings() }
func (s *limitSearchResultSet) Err() error                        { return s.rs.Err() }
func (s *limitSearchResultSet) Close() error                      { return s.rs.Close() }

// mergingSearchResultSet lazily merges two pre-sorted SearchResultSets using
// the comparison function defined by order. Both inputs must yield results in
// that order. Equal entries (same Value under value orderings, same
// (Score, Value) under OrderByScoreDesc) collapse in place; under value
// orderings the higher score wins.
//
// Partial-error semantics: if one side returns Next()==false with a non-nil
// Err(), iteration does not terminate. The other side keeps draining until
// it too is exhausted, after which Err() returns errors.Join of any errors
// recorded on either side. This trades a strict fail-fast contract for the
// preservation of buffered results from the surviving side, which is the
// behaviour expected by callers that fan out queries across heterogeneous
// backends.
type mergingSearchResultSet struct {
	a, b         SearchResultSet
	cmpFn        func(a, b SearchResult) int
	valueOrder   bool // true when order collapses adjacent duplicates by Value.
	limit        int
	emitted      int
	curr         SearchResult
	aVal, bVal   SearchResult
	aOk, bOk     bool // Whether aVal/bVal hold a buffered value.
	aInit, bInit bool // Whether a/b have been advanced at least once.
	done         bool
}

func newMergingSearchResultSet(a, b SearchResultSet, order Ordering, limit int) *mergingSearchResultSet {
	return &mergingSearchResultSet{
		a:          a,
		b:          b,
		cmpFn:      compareSearchResults(order),
		valueOrder: order == OrderByValueAsc || order == OrderByValueDesc,
		limit:      limit,
	}
}

func (s *mergingSearchResultSet) Next() bool {
	if s.done {
		return false
	}
	if s.limit > 0 && s.emitted >= s.limit {
		s.done = true
		return false
	}

	// Prime both sides on first call. A side returning Next()=false here
	// either ran clean out of values or surfaced an error; in either case
	// we stop pulling from it but keep draining the other side. The error
	// (if any) is reported via Err() once iteration ends, joined with the
	// other side's error if it also fails.
	if !s.aInit {
		s.aOk = s.a.Next()
		if s.aOk {
			s.aVal = s.a.At()
		}
		s.aInit = true
	}
	if !s.bInit {
		s.bOk = s.b.Next()
		if s.bOk {
			s.bVal = s.b.At()
		}
		s.bInit = true
	}

	switch {
	case !s.aOk && !s.bOk:
		s.done = true
		return false
	case !s.aOk:
		s.curr = s.bVal
		s.bOk = s.b.Next()
		if s.bOk {
			s.bVal = s.b.At()
		}
	case !s.bOk:
		s.curr = s.aVal
		s.aOk = s.a.Next()
		if s.aOk {
			s.aVal = s.a.At()
		}
	default:
		// Under value-based orderings, equal-Value entries collapse in
		// place and keep the higher score. Under OrderByScoreDesc the
		// comparator tie-breaks on Value, so equal cmp means equal
		// (Score, Value) — collapsing is safe there too.
		c := s.cmpFn(s.aVal, s.bVal)
		switch {
		case c < 0:
			s.curr = s.aVal
			s.aOk = s.a.Next()
			if s.aOk {
				s.aVal = s.a.At()
			}
		case c > 0:
			s.curr = s.bVal
			s.bOk = s.b.Next()
			if s.bOk {
				s.bVal = s.b.At()
			}
		default:
			if s.valueOrder && s.bVal.Score > s.aVal.Score {
				s.curr = s.bVal
			} else {
				s.curr = s.aVal
			}
			s.aOk = s.a.Next()
			if s.aOk {
				s.aVal = s.a.At()
			}
			s.bOk = s.b.Next()
			if s.bOk {
				s.bVal = s.b.At()
			}
		}
	}

	s.emitted++
	return true
}

func (s *mergingSearchResultSet) At() SearchResult { return s.curr }

func (s *mergingSearchResultSet) Warnings() annotations.Annotations {
	var ws annotations.Annotations
	ws.Merge(s.a.Warnings())
	ws.Merge(s.b.Warnings())
	return ws
}

func (s *mergingSearchResultSet) Err() error {
	return errors.Join(s.a.Err(), s.b.Err())
}

func (s *mergingSearchResultSet) Close() error {
	return errors.Join(s.a.Close(), s.b.Close())
}

// SearchLabelNames implements Searcher by merging results from all underlying queriers
// that support the Searcher interface.
func (q *querierAdapter) SearchLabelNames(ctx context.Context, hints *SearchHints, matchers ...*labels.Matcher) SearchResultSet {
	return mergeSearchSets(hints, func(s Searcher) SearchResultSet {
		return s.SearchLabelNames(ctx, hints, matchers...)
	}, collectSearchers(q.genericQuerier))
}

// SearchLabelValues implements Searcher by merging results from all underlying queriers
// that support the Searcher interface.
func (q *querierAdapter) SearchLabelValues(ctx context.Context, name string, hints *SearchHints, matchers ...*labels.Matcher) SearchResultSet {
	return mergeSearchSets(hints, func(s Searcher) SearchResultSet {
		return s.SearchLabelValues(ctx, name, hints, matchers...)
	}, collectSearchers(q.genericQuerier))
}

type noopGenericSeriesSet struct{}

func (noopGenericSeriesSet) Next() bool { return false }

func (noopGenericSeriesSet) At() Labels { return nil }

func (noopGenericSeriesSet) Err() error { return nil }

func (noopGenericSeriesSet) Warnings() annotations.Annotations { return nil }
