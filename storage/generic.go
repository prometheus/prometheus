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

// ApplySearchHints filters, sorts, and limits a slice of string values according to hints,
// returning scored SearchResult entries. A nil hints value is treated as the zero value.
// The input values slice is assumed to be ordered ascending by value; the function only
// performs extra work for orderings that differ from this.
func ApplySearchHints(values []string, hints *SearchHints) []SearchResult {
	if hints == nil {
		hints = &SearchHints{}
	}
	results := make([]SearchResult, 0, len(values))
	for _, v := range values {
		if hints.Filter != nil {
			accepted, score := hints.Filter.Accept(v)
			if accepted {
				results = append(results, SearchResult{Value: v, Score: score})
			}
		} else {
			results = append(results, SearchResult{Value: v, Score: 1.0})
		}
	}
	switch hints.OrderBy {
	case OrderByValueAsc:
		// Input is already ascending; nothing to do.
	case OrderByValueDesc:
		slices.Reverse(results)
	case OrderByScoreDesc:
		slices.SortFunc(results, compareSearchResults(OrderByScoreDesc))
	}
	if hints.Limit > 0 && len(results) > hints.Limit {
		results = results[:hints.Limit]
	}
	return results
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

	// Prime both sides on first call.
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

	// Check for errors from either side after priming or after the
	// previous advance. An error means we should stop iteration.
	if s.a.Err() != nil || s.b.Err() != nil {
		s.done = true
		return false
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
