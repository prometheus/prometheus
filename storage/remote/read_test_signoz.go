package remote

import (
	"context"
	"fmt"

	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/base"
)

type mockStorage struct {
	base.Storage
	got   *prompb.Query
	store []*prompb.TimeSeries
}

func (m *mockStorage) Read(_ context.Context, query *prompb.Query) (*prompb.QueryResult, error) {
	if m.got != nil {
		return nil, fmt.Errorf("expected only one call to remote client got: %v", query)
	}
	m.got = query

	matchers, err := FromLabelMatchers(query.Matchers)
	if err != nil {
		return nil, err
	}

	q := &prompb.QueryResult{}
	for _, s := range m.store {
		l := labelProtosToLabels(s.Labels)
		var notMatch bool

		for _, m := range matchers {
			if v := l.Get(m.Name); v != "" {
				if !m.Matches(v) {
					notMatch = true
					break
				}
			}
		}

		if !notMatch {
			q.Timeseries = append(q.Timeseries, &prompb.TimeSeries{Labels: s.Labels})
		}
	}
	return q, nil
}

func (m *mockStorage) CH() base.Storage {
	return m
}

func (m *mockStorage) reset() {
	m.got = nil
}
