package tsdb

import (
	"unsafe"

	"github.com/fabxc/tsdb"
	tsdbLabels "github.com/fabxc/tsdb/labels"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
)

type storage struct {
	db *tsdb.DB
}

// Open returns a new storage backed by a tsdb database.
func Open(path string) (storage.Storage, error) {
	db, err := tsdb.Open(path, nil, nil)
	if err != nil {
		return nil, err
	}
	return &storage{db: db}
}

func (db *storage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := db.db.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return querier{q: q}, nil
}

// Appender returns a new appender against the storage.
func (db *storage) Appender() (Appender, error) {
	a, err := db.db.Appender()
	if err != nil {
		return nil, err
	}
	return appender{a: a}, nil
}

// Close closes the storage and all its underlying resources.
func (db *storage) Close() error {
	return db.Close()
}

type querier struct {
	q tsdb.Querier
}

func (q *querier) Select(oms ...*promql.LabelMatcher) (storage.SeriesSet, error) {
	ms := make([]tsdbLabels.Matcher, 0, len(oms))

	for _, om := range oms {
		ms = append(ms, convertMatcher(om))
	}

	set := q.q.Select(ms...)

	return seriesSet{set: set}
}

func (q *querier) LabelValues(name string) ([]string, error) { return q.q.LabelValues(name) }
func (q *querier) Close() error                              { return q.q.Close() }

type seriesSet struct {
	set tsdb.SeriesSet
}

func (s *seriesSet) Next() bool             { return s.set.Next() }
func (s *seriesSet) Err() error             { return s.set.Err() }
func (s *seriesSet) Series() storage.Series { return series{s: s.set.Series()} }

type series struct {
	s tsdb.Series
}

func (s *series) Labels() labels.Labels            { return toLabels(s.s.Labels()) }
func (s *series) Iterator() storage.SeriesIterator { return storage.SeriesIterator(s.s.Iterator()) }

type appender struct {
	a tsdb.Appender
}

func (a *appender) Add(lset labels.Labels, t int64, v float64) { a.Add(toTSDBLabels(lset), t, v) }
func (a *appender) Commit() error                              { a.a.Commit() }

func convertMatcher(m *promql.LabelMatcher) tsdbLabels.Matcher {
	switch m.Type {
	case MatchEqual:
		return tsdbLabels.NewEqualMatcher(m.Name, m.Value)

	case MatchNotEqual:
		return tsdbLabels.Not(tsdbLabels.NewEqualMatcher(m.Name, m.Value))

	case MatchRegexp:
		res, err := tsdbLabels.NewRegexpMatcher(m.Name, m.Value)
		if err != nil {
			panic(err)
		}
		return res

	case MatchNotRegexp:
		res, err := tsdbLabels.NewRegexpMatcher(m.Name, m.Value)
		if err != nil {
			panic(err)
		}
		return tsdbLabels.Not(res)
	}
	panic("promql.LabelMatcher.matcher: invalid matcher type")
}

func toTSDBLabels(l labels.Labels) tsdbLabels.Labels {
	return *(*tsdbLabels.Labels)(unsafe.Pointer(&l))
}

func toLabels(l tsdbLabels.Labels) labels.Labels {
	return *(*labels.Labels)(unsafe.Pointer(&l))
}
