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
	"context"
	"slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/annotations"
)

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

// collectSearchers extracts Searcher implementations from a genericQuerier tree.
func collectSearchers(gq genericQuerier) []Searcher {
	switch v := gq.(type) {
	case *mergeGenericQuerier:
		var searchers []Searcher
		for _, q := range v.queriers {
			searchers = append(searchers, collectSearchers(q)...)
		}
		return searchers
	case *secondaryQuerier:
		return collectSearchers(v.genericQuerier)
	case *genericQuerierAdapter:
		if s, ok := v.q.(Searcher); ok {
			return []Searcher{s}
		}
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

func (s *sliceSearchResultSet) Err() error { return nil }

func (s *sliceSearchResultSet) Close() error { return nil }

// NewSearchResultSetFromSlice returns a SearchResultSet that iterates over the given slice.
func NewSearchResultSetFromSlice(results []SearchResult, warns annotations.Annotations) SearchResultSet {
	return &sliceSearchResultSet{results: results, warnings: warns, idx: -1}
}

// mergeSearchSets merges results from multiple Searcher calls, deduplicating by value
// and taking the maximum score for duplicates. Returns a SearchResultSet.
func mergeSearchSets(hints *SearchHints, fn func(Searcher) SearchResultSet, searchers []Searcher) SearchResultSet {
	if len(searchers) == 0 {
		return EmptySearchResultSet()
	}
	if len(searchers) == 1 {
		return fn(searchers[0])
	}

	scores := make(map[string]float64)
	var warns annotations.Annotations
	for _, s := range searchers {
		rs := fn(s)
		for rs.Next() {
			r := rs.At()
			if existing, ok := scores[r.Value]; !ok || r.Score > existing {
				scores[r.Value] = r.Score
			}
		}
		warns.Merge(rs.Warnings())
		if err := rs.Err(); err != nil {
			_ = rs.Close()
			return ErrSearchResultSet(err)
		}
		_ = rs.Close()
	}

	merged := make([]SearchResult, 0, len(scores))
	for value, score := range scores {
		merged = append(merged, SearchResult{Value: value, Score: score})
	}
	if hints != nil && hints.CompareFunc != nil {
		slices.SortFunc(merged, hints.CompareFunc.Compare)
	}
	if hints != nil && hints.Limit > 0 && len(merged) > hints.Limit {
		merged = merged[:hints.Limit]
	}
	return &sliceSearchResultSet{results: merged, warnings: warns, idx: -1}
}

// Compile-time assertion that querierAdapter implements Searcher.
var _ Searcher = &querierAdapter{}

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
