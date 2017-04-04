package tsdb

import (
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/tsdb"
	tsdbLabels "github.com/prometheus/tsdb/labels"
)

// adapter implements a storage.Storage around TSDB.
type adapter struct {
	db *tsdb.DB
}

// Options of the DB storage.
type Options struct {
	// The interval at which the write ahead log is flushed to disc.
	WALFlushInterval time.Duration

	// The timestamp range of head blocks after which they get persisted.
	// It's the minimum duration of any persisted block.
	MinBlockDuration time.Duration

	// The maximum timestamp range of compacted blocks.
	MaxBlockDuration time.Duration

	// Number of head blocks that can be appended to.
	// Should be two or higher to prevent write errors in general scenarios.
	//
	// After a new block is started for timestamp t0 or higher, appends with
	// timestamps as early as t0 - (n-1) * MinBlockDuration are valid.
	AppendableBlocks int

	// Duration for how long to retain data.
	Retention time.Duration
}

// Open returns a new storage backed by a tsdb database.
func Open(path string, r prometheus.Registerer, opts *Options) (storage.Storage, error) {
	db, err := tsdb.Open(path, nil, r, &tsdb.Options{
		WALFlushInterval:  10 * time.Second,
		MinBlockDuration:  uint64(opts.MinBlockDuration.Seconds() * 1000),
		MaxBlockDuration:  uint64(opts.MaxBlockDuration.Seconds() * 1000),
		AppendableBlocks:  opts.AppendableBlocks,
		RetentionDuration: uint64(opts.Retention.Seconds() * 1000),
	})
	if err != nil {
		return nil, err
	}
	return adapter{db: db}, nil
}

func (a adapter) Querier(mint, maxt int64) (storage.Querier, error) {
	return querier{q: a.db.Querier(mint, maxt)}, nil
}

// Appender returns a new appender against the storage.
func (a adapter) Appender() (storage.Appender, error) {
	return appender{a: a.db.Appender()}, nil
}

// Close closes the storage and all its underlying resources.
func (a adapter) Close() error {
	return a.db.Close()
}

type querier struct {
	q tsdb.Querier
}

func (q querier) Select(oms ...*labels.Matcher) storage.SeriesSet {
	ms := make([]tsdbLabels.Matcher, 0, len(oms))

	for _, om := range oms {
		ms = append(ms, convertMatcher(om))
	}

	return seriesSet{set: q.q.Select(ms...)}
}

func (q querier) LabelValues(name string) ([]string, error) { return q.q.LabelValues(name) }
func (q querier) Close() error                              { return q.q.Close() }

type seriesSet struct {
	set tsdb.SeriesSet
}

func (s seriesSet) Next() bool         { return s.set.Next() }
func (s seriesSet) Err() error         { return s.set.Err() }
func (s seriesSet) At() storage.Series { return series{s: s.set.At()} }

type series struct {
	s tsdb.Series
}

func (s series) Labels() labels.Labels            { return toLabels(s.s.Labels()) }
func (s series) Iterator() storage.SeriesIterator { return storage.SeriesIterator(s.s.Iterator()) }

type appender struct {
	a tsdb.Appender
}

func (a appender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	ref, err := a.a.Add(toTSDBLabels(lset), t, v)

	switch err {
	case tsdb.ErrNotFound:
		return 0, storage.ErrNotFound
	case tsdb.ErrOutOfOrderSample:
		return 0, storage.ErrOutOfOrderSample
	case tsdb.ErrAmendSample:
		return 0, storage.ErrDuplicateSampleForTimestamp
	}
	return ref, err
}

func (a appender) AddFast(ref uint64, t int64, v float64) error {
	err := a.a.AddFast(ref, t, v)

	switch err {
	case tsdb.ErrNotFound:
		return storage.ErrNotFound
	case tsdb.ErrOutOfOrderSample:
		return storage.ErrOutOfOrderSample
	case tsdb.ErrAmendSample:
		return storage.ErrDuplicateSampleForTimestamp
	}
	return err
}

func (a appender) Commit() error   { return a.a.Commit() }
func (a appender) Rollback() error { return a.a.Rollback() }

func convertMatcher(m *labels.Matcher) tsdbLabels.Matcher {
	switch m.Type {
	case labels.MatchEqual:
		return tsdbLabels.NewEqualMatcher(m.Name, m.Value)

	case labels.MatchNotEqual:
		return tsdbLabels.Not(tsdbLabels.NewEqualMatcher(m.Name, m.Value))

	case labels.MatchRegexp:
		res, err := tsdbLabels.NewRegexpMatcher(m.Name, m.Value)
		if err != nil {
			panic(err)
		}
		return res

	case labels.MatchNotRegexp:
		res, err := tsdbLabels.NewRegexpMatcher(m.Name, m.Value)
		if err != nil {
			panic(err)
		}
		return tsdbLabels.Not(res)
	}
	panic("storage.convertMatcher: invalid matcher type")
}

func toTSDBLabels(l labels.Labels) tsdbLabels.Labels {
	return *(*tsdbLabels.Labels)(unsafe.Pointer(&l))
}

func toLabels(l tsdbLabels.Labels) labels.Labels {
	return *(*labels.Labels)(unsafe.Pointer(&l))
}
