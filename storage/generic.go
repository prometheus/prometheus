package storage

import "github.com/prometheus/prometheus/pkg/labels"

// Boilerplate on purpose. Generics some day...
type genericQuerierAdapter struct {
	baseQuerier

	// One-of. If both are set, Querier will be used.
	q  Querier
	cq ChunkQuerier
}

type genericSeriesSetAdapter struct {
	SeriesSet
}

func (a *genericSeriesSetAdapter) At() Labeled {
	return a.SeriesSet.At().(Labeled)
}

type genericChunkSeriesSetAdapter struct {
	ChunkSeriesSet
}

func (a *genericChunkSeriesSetAdapter) At() Labeled {
	return a.ChunkSeriesSet.At().(Labeled)
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
	VerticalSeriesMerger
	buf []Series
}

func (a *seriesMergerAdapter) Merge(s ...Labeled) Labeled {
	a.buf = a.buf[:0]
	for _, ser := range s {
		a.buf = append(a.buf, ser.(Series))
	}
	return a.VerticalSeriesMerger.Merge(a.buf...)
}

type chunkSeriesMergerAdapter struct {
	VerticalChunkSeriesMerger
	buf []ChunkSeries
}

func (a *chunkSeriesMergerAdapter) Merge(s ...Labeled) Labeled {
	a.buf = a.buf[:0]
	for _, ser := range s {
		a.buf = append(a.buf, ser.(ChunkSeries))
	}
	return a.VerticalChunkSeriesMerger.Merge(a.buf...)
}
