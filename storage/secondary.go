package storage

import "github.com/prometheus/prometheus/pkg/labels"

// secondaryQuerier is a wrapper that allows a querier to be treated in best effort manner.
// This means that a potential error on any method except Close will be passed as warning, and the result will be empty.
// NOTE: Prometheus treats all remote storages as secondary / best effort ones.
type secondaryQuerier struct {
	genericQuerier
}

func newSecondaryQuerierFrom(q Querier) genericQuerier {
	return &secondaryQuerier{newGenericQuerierFrom(q)}
}

func newSecondaryQuerierFromChunk(cq ChunkQuerier) genericQuerier {
	return &secondaryQuerier{newGenericQuerierFromChunk(cq)}
}

func (s *secondaryQuerier) Select(sortSeries bool, hints *SelectHints, matchers ...*labels.Matcher) genericSeriesSet {
	return &secondarySeriesSet{genericSeriesSet: s.genericQuerier.Select(sortSeries, hints, matchers...)}
}

func (s *secondaryQuerier) LabelValues(name string) ([]string, Warnings, error) {
	vals, w, err := s.genericQuerier.LabelValues(name)
	if err != nil {
		return nil, append([]error{err}, w...), nil
	}
	return vals, w, nil
}

func (s *secondaryQuerier)  LabelNames() ([]string, Warnings, error) {
	names, w, err := s.genericQuerier.LabelNames()
	if err != nil {
		return nil, append([]error{err}, w...), nil
	}
	return names, w, nil
}

type secondarySeriesSet struct {
	genericSeriesSet
}

func (c *secondarySeriesSet) Err() error {
	// Mask all errors, this series set is secondary.
	return nil
}

func (c *secondarySeriesSet) Warnings() Warnings {
	if err := c.genericSeriesSet.Err(); err != nil {
		return append([]error{err}, c.genericSeriesSet.Warnings()...)
	}
	return c.genericSeriesSet.Warnings()
}