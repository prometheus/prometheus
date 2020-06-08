// Copyright 2020 The Prometheus Authors
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

import "github.com/prometheus/prometheus/pkg/labels"

type genericQuerier interface {
	baseQuerier
	Select(bool, *SelectHints, ...*labels.Matcher) (genericSeriesSet, Warnings, error)
}

type genericSeriesSet interface {
	Next() bool
	At() Labels
	Err() error
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
	baseQuerier

	// One-of. If both are set, Querier will be used.
	q  Querier
	cq ChunkQuerier
}

func (q *genericQuerierAdapter) Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) (genericSeriesSet, Warnings, error) {
	if q.q != nil {
		s, w, err := q.q.Select(sortSeries, hints, matchers...)
		return &genericSeriesSetAdapter{s}, w, err
	}
	s, w, err := q.cq.Select(sortSeries, hints, matchers...)
	return &genericChunkSeriesSetAdapter{s}, w, err
}

func newGenericQuerierFrom(q Querier) genericQuerier {
	return &genericQuerierAdapter{baseQuerier: q, q: q}
}

func newGenericQuerierFromChunk(cq ChunkQuerier) genericQuerier {
	return &genericQuerierAdapter{baseQuerier: cq, cq: cq}
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

func (q *querierAdapter) Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) (SeriesSet, Warnings, error) {
	s, w, err := q.genericQuerier.Select(sortSeries, hints, matchers...)
	return &seriesSetAdapter{s}, w, err
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

func (q *chunkQuerierAdapter) Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) (ChunkSeriesSet, Warnings, error) {
	s, w, err := q.genericQuerier.Select(sortSeries, hints, matchers...)
	return &chunkSeriesSetAdapter{s}, w, err
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
	VerticalChunkSeriesMergerFunc
}

func (a *chunkSeriesMergerAdapter) Merge(s ...Labels) Labels {
	buf := make([]ChunkSeries, 0, len(s))
	for _, ser := range s {
		buf = append(buf, ser.(ChunkSeries))
	}
	return a.VerticalChunkSeriesMergerFunc(buf...)
}
