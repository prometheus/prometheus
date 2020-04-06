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

import (
	"context"
	"github.com/prometheus/prometheus/pkg/gate"
	"github.com/prometheus/prometheus/pkg/labels"
)

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
	remotely       bool
	remoteReadGate *gate.Gate
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

func (q *querierAdapter) Selects(ctx context.Context, selectParams []*SelectParam) []*SelectResult {
	var result []*SelectResult
	if q.remotely && len(selectParams) > 1 {
		if q.remoteReadGate != nil {
			if err := q.remoteReadGate.Start(ctx); err != nil {
				for _, param := range selectParams {
					result = append(result, &SelectResult{param, nil, nil, err})
				}
				return result
			}
			defer q.remoteReadGate.Done()
		}

		queryResultChan := make(chan *SelectResult)
		for _, param := range selectParams {
			go func(sp *SelectParam) {
				set, wrn, err := q.Select(sp.SortSeries, sp.Hints, sp.Matchers...)
				queryResultChan <- &SelectResult{sp, set, wrn, err}
			}(param)
		}
		for i := 0; i < len(selectParams); i++ {
			qryResult := <-queryResultChan
			result = append(result, qryResult)
		}
	} else {
		for _, sp := range selectParams {
			set, wrn, err := q.Select(sp.SortSeries, sp.Hints, sp.Matchers...)
			result = append(result, &SelectResult{sp, set, wrn, err})
		}
	}
	return result
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
	buf []Series
}

func (a *seriesMergerAdapter) Merge(s ...Labels) Labels {
	a.buf = a.buf[:0]
	for _, ser := range s {
		a.buf = append(a.buf, ser.(Series))
	}
	return a.VerticalSeriesMergeFunc(a.buf...)
}

type chunkSeriesMergerAdapter struct {
	VerticalChunkSeriesMergerFunc
	buf []ChunkSeries
}

func (a *chunkSeriesMergerAdapter) Merge(s ...Labels) Labels {
	a.buf = a.buf[:0]
	for _, ser := range s {
		a.buf = append(a.buf, ser.(ChunkSeries))
	}
	return a.VerticalChunkSeriesMergerFunc(a.buf...)
}
