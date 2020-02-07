package storage

import "github.com/prometheus/prometheus/pkg/labels"

// Boilerplate on purpose. Generics some day...

type genericQuerierAdapter struct {
	baseQuerier

	// Union.
	q  Querier
	cq ChunkedQuerier
}

func (q *genericQuerierAdapter) Select(params *SelectParams, matchers ...*labels.Matcher) (genericSeriesSet, Warnings, error) {
	if q.q != nil {
		s, w, err := q.q.Select(params, matchers...)
		return s.(genericSeriesSet), w, err
	}
	s, w, err := q.cq.Select(params, matchers...)
	return s.(genericSeriesSet), w, err
}
func (q *genericQuerierAdapter) SelectSorted(params *SelectParams, matchers ...*labels.Matcher) (genericSeriesSet, Warnings, error) {
	if q.q != nil {
		s, w, err := q.q.SelectSorted(params, matchers...)
		return s.(genericSeriesSet), w, err
	}
	s, w, err := q.cq.SelectSorted(params, matchers...)
	return s.(genericSeriesSet), w, err
}

func newGenericQuerierFrom(q Querier) genericQuerier {
	return &genericQuerierAdapter{baseQuerier: q, q: q}
}

func newGenericQuerierFromChunked(cq ChunkedQuerier) genericQuerier {
	return &genericQuerierAdapter{baseQuerier: cq, cq: cq}
}

type genericQuerier interface {
	baseQuerier
	Select(*SelectParams, ...*labels.Matcher) (genericSeriesSet, Warnings, error)
	SelectSorted(*SelectParams, ...*labels.Matcher) (genericSeriesSet, Warnings, error)
}

type genericSeriesSet interface {
	Next() bool
	At() Labeled
	Err() error
}

type querierAdapter struct {
	genericQuerier
}

func (q *querierAdapter) Select(params *SelectParams, matchers ...*labels.Matcher) (SeriesSet, Warnings, error) {
	s, w, err := q.Select(params, matchers...)
	return s.(SeriesSet), w, err
}
func (q *querierAdapter) SelectSorted(params *SelectParams, matchers ...*labels.Matcher) (SeriesSet, Warnings, error) {
	s, w, err := q.SelectSorted(params, matchers...)
	return s.(SeriesSet), w, err
}

type chunkedQuerierAdapter struct {
	genericQuerier
}

func (q *chunkedQuerierAdapter) Select(params *SelectParams, matchers ...*labels.Matcher) (ChunkedSeriesSet, Warnings, error) {
	s, w, err := q.Select(params, matchers...)
	return s.(ChunkedSeriesSet), w, err
}
func (q *chunkedQuerierAdapter) SelectSorted(params *SelectParams, matchers ...*labels.Matcher) (ChunkedSeriesSet, Warnings, error) {
	s, w, err := q.SelectSorted(params, matchers...)
	return s.(ChunkedSeriesSet), w, err
}

type genericSeriesMerger interface {
	NewMergedSeries(...Labeled) Labeled
}

func newGenericSeriesMergerFrom(q SeriesMerger) genericSeriesMerger {

}

func newGenericSeriesMergerFromChunked(q ChunkedSeriesMerger) genericSeriesMerger {

}
