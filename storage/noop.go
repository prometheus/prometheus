package storage

import "github.com/prometheus/prometheus/pkg/labels"

type noopQuerier struct{}

// NoopQuerier is a Querier that does nothing.
var NoopQuerier Querier = noopQuerier{}

func (noopQuerier) Select(...*labels.Matcher) SeriesSet {
	return NoopSeriesSet
}

func (noopQuerier) LabelValues(name string) ([]string, error) {
	return nil, nil
}

func (noopQuerier) Close() error {
	return nil
}

type noopSeriesSet struct{}

// NoopSeriesSet is a SeriesSet that does nothing.
var NoopSeriesSet = noopSeriesSet{}

func (noopSeriesSet) Next() bool {
	return false
}

func (noopSeriesSet) At() Series {
	return nil
}

func (noopSeriesSet) Err() error {
	return nil
}
