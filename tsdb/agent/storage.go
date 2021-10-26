// Copyright 2021 The Prometheus Authors
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

package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wal"
	"go.uber.org/atomic"
)

var (
	ErrUnsupported = errors.New("unsupported operation with WAL-only storage")
)

// Default values for options.
var (
	DefaultTruncateFrequency = int64(2 * time.Hour / time.Millisecond)
	DefaultMinWALTime        = int64(5 * time.Minute / time.Millisecond)
	DefaultMaxWALTime        = int64(4 * time.Hour / time.Millisecond)
)

// Options of the WAL storage.
type Options struct {
	// Segments (wal files) max size.
	// WALSegmentSize <= 0, segment size is default size.
	// WALSegmentSize > 0, segment size is WALSegmentSize.
	WALSegmentSize int

	// WALCompression will turn on Snappy compression for records on the WAL.
	WALCompression bool

	// StripeSize is the size (power of 2) in entries of the series hash map. Reducing the size will save memory but impact performance.
	StripeSize int

	// TruncateFrequency determines how frequently to truncate data from the WAL.
	TruncateFrequency int64

	// Shortest and longest amount of time data can exist in the WAL before being
	// deleted.
	MinWALTime, MaxWALTime int64
}

// DefaultOptions used for the WAL storage. They are sane for setups using
// millisecond-precision timestamps.
func DefaultOptions() *Options {
	return &Options{
		WALSegmentSize:    wal.DefaultSegmentSize,
		WALCompression:    false,
		StripeSize:        tsdb.DefaultStripeSize,
		TruncateFrequency: DefaultTruncateFrequency,
		MinWALTime:        DefaultMinWALTime,
		MaxWALTime:        DefaultMaxWALTime,
	}
}

type storageMetrics struct {
	r prometheus.Registerer

	numActiveSeries             prometheus.Gauge
	numWALSeriesPendingDeletion prometheus.Gauge
	totalAppendedSamples        prometheus.Counter
	walTruncateDuration         prometheus.Summary
	walCorruptionsTotal         prometheus.Counter
	walTotalReplayDuration      prometheus.Gauge
	checkpointDeleteFail        prometheus.Counter
	checkpointDeleteTotal       prometheus.Counter
	checkpointCreationFail      prometheus.Counter
	checkpointCreationTotal     prometheus.Counter
}

func newStorageMetrics(r prometheus.Registerer) *storageMetrics {
	m := storageMetrics{r: r}
	m.numActiveSeries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_agent_active_series",
		Help: "Number of active series being tracked by the WAL storage",
	})

	m.numWALSeriesPendingDeletion = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_agent_deleted_series",
		Help: "Number of series pending deletion from the WAL",
	})

	m.totalAppendedSamples = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_samples_appended_total",
		Help: "Total number of samples appended to the storage",
	})

	m.walTruncateDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "prometheus_agent_truncate_duration_seconds",
		Help: "Duration of WAL truncation.",
	})

	m.walCorruptionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_corruptions_total",
		Help: "Total number of WAL corruptions.",
	})

	m.walTotalReplayDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_agent_data_replay_duration_seconds",
		Help: "Time taken to replay the data on disk.",
	})

	m.checkpointDeleteFail = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_checkpoint_deletions_failed_total",
		Help: "Total number of checkpoint deletions that failed.",
	})

	m.checkpointDeleteTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_checkpoint_deletions_total",
		Help: "Total number of checkpoint deletions attempted.",
	})

	m.checkpointCreationFail = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_checkpoint_creations_failed_total",
		Help: "Total number of checkpoint creations that failed.",
	})

	m.checkpointCreationTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_checkpoint_creations_total",
		Help: "Total number of checkpoint creations attempted.",
	})

	if r != nil {
		r.MustRegister(
			m.numActiveSeries,
			m.numWALSeriesPendingDeletion,
			m.totalAppendedSamples,
			m.walTruncateDuration,
			m.walCorruptionsTotal,
			m.walTotalReplayDuration,
			m.checkpointDeleteFail,
			m.checkpointDeleteTotal,
			m.checkpointCreationFail,
			m.checkpointCreationTotal,
		)
	}

	return &m
}

func (m *storageMetrics) Unregister() {
	if m.r == nil {
		return
	}
	cs := []prometheus.Collector{
		m.numActiveSeries,
		m.numWALSeriesPendingDeletion,
		m.totalAppendedSamples,
	}
	for _, c := range cs {
		m.r.Unregister(c)
	}
}

// Storage represents a WAL-only storage. It implements storage.Storage.
type Storage struct {
	mtx    sync.RWMutex
	logger log.Logger
	opts   *Options
	rs     *remote.Storage

	wal *wal.WAL

	appenderPool sync.Pool
	bufPool      sync.Pool

	nextRef *atomic.Uint64
	series  *stripeSeries
	// deleted is a map of (ref IDs that should be deleted from WAL) to (the WAL segment they
	// must be kept around to).
	deleted map[uint64]int

	donec chan struct{}
	stopc chan struct{}

	metrics *storageMetrics
}

// NewStorage returns a agent.Storage.
func NewStorage(l log.Logger, reg prometheus.Registerer, rs *remote.Storage, dir string, opts *Options) (*Storage, error) {
	opts = validateOptions(opts)

	// remote_write expects WAL to be stored in a "wal" subdirectory of the main storage.
	dir = filepath.Join(dir, "wal")

	w, err := wal.NewSize(l, reg, dir, opts.WALSegmentSize, opts.WALCompression)
	if err != nil {
		return nil, errors.Wrap(err, "creating WAL")
	}

	s := &Storage{
		logger: l,
		opts:   opts,
		rs:     rs,

		wal: w,

		nextRef: atomic.NewUint64(0),
		series:  newStripeSeries(opts.StripeSize),
		deleted: make(map[uint64]int),

		donec: make(chan struct{}),
		stopc: make(chan struct{}),

		metrics: newStorageMetrics(reg),
	}

	s.bufPool.New = func() interface{} {
		return make([]byte, 0, 1024)
	}

	s.appenderPool.New = func() interface{} {
		return &appender{
			Storage:        s,
			pendingSeries:  make([]record.RefSeries, 0, 100),
			pendingSamples: make([]record.RefSample, 0, 100),
		}
	}

	if err := s.replayWAL(); err != nil {
		level.Warn(s.logger).Log("msg", "encountered WAL read error, attempting repair", "err", err)
		if err := w.Repair(err); err != nil {
			return nil, errors.Wrap(err, "repair corrupted WAL")
		}
	}

	go s.run()
	return s, nil
}

func validateOptions(opts *Options) *Options {
	if opts == nil {
		opts = DefaultOptions()
	}
	if opts.WALSegmentSize <= 0 {
		opts.WALSegmentSize = wal.DefaultSegmentSize
	}

	// Revert Stripesize to DefaultStripsize if Stripsize is either 0 or not a power of 2.
	if opts.StripeSize <= 0 || !((opts.StripeSize & (opts.StripeSize - 1)) == 0) {
		opts.StripeSize = tsdb.DefaultStripeSize
	}
	if opts.TruncateFrequency <= 0 {
		opts.TruncateFrequency = DefaultTruncateFrequency
	}
	if opts.MinWALTime <= 0 {
		opts.MinWALTime = 0
	}
	if opts.MaxWALTime <= 0 {
		opts.MaxWALTime = DefaultMaxWALTime
	}
	if opts.MaxWALTime < opts.TruncateFrequency {
		opts.MaxWALTime = opts.TruncateFrequency
	}
	return opts
}

func (s *Storage) replayWAL() error {
	level.Info(s.logger).Log("msg", "replaying WAL, this may take a while", "dir", s.wal.Dir())
	start := time.Now()

	dir, startFrom, err := wal.LastCheckpoint(s.wal.Dir())
	if err != nil && err != record.ErrNotFound {
		return errors.Wrap(err, "find last checkpoint")
	}

	multiRef := map[uint64]uint64{}

	if err == nil {
		sr, err := wal.NewSegmentsReader(dir)
		if err != nil {
			return errors.Wrap(err, "open checkpoint")
		}
		defer func() {
			if err := sr.Close(); err != nil {
				level.Warn(s.logger).Log("msg", "error while closing the wal segments reader", "err", err)
			}
		}()

		// A corrupted checkpoint is a hard error for now and requires user
		// intervention. There's likely little data that can be recovered anyway.
		if err := s.loadWAL(wal.NewReader(sr), multiRef); err != nil {
			return errors.Wrap(err, "backfill checkpoint")
		}
		startFrom++
		level.Info(s.logger).Log("msg", "WAL checkpoint loaded")
	}

	// Find the last segment.
	_, last, err := wal.Segments(s.wal.Dir())
	if err != nil {
		return errors.Wrap(err, "finding WAL segments")
	}

	// Backfil segments from the most recent checkpoint onwards.
	for i := startFrom; i <= last; i++ {
		seg, err := wal.OpenReadSegment(wal.SegmentName(s.wal.Dir(), i))
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("open WAL segment: %d", i))
		}

		sr := wal.NewSegmentBufReader(seg)
		err = s.loadWAL(wal.NewReader(sr), multiRef)
		if err := sr.Close(); err != nil {
			level.Warn(s.logger).Log("msg", "error while closing the wal segments reader", "err", err)
		}
		if err != nil {
			return err
		}
		level.Info(s.logger).Log("msg", "WAL segment loaded", "segment", i, "maxSegment", last)
	}

	walReplayDuration := time.Since(start)
	s.metrics.walTotalReplayDuration.Set(walReplayDuration.Seconds())

	return nil
}

func (s *Storage) loadWAL(r *wal.Reader, multiRef map[uint64]uint64) (err error) {
	var (
		dec     record.Decoder
		lastRef uint64

		decoded    = make(chan interface{}, 10)
		errCh      = make(chan error, 1)
		seriesPool = sync.Pool{
			New: func() interface{} {
				return []record.RefSeries{}
			},
		}
		samplesPool = sync.Pool{
			New: func() interface{} {
				return []record.RefSample{}
			},
		}
	)

	go func() {
		defer close(decoded)
		for r.Next() {
			rec := r.Record()
			switch dec.Type(rec) {
			case record.Series:
				series := seriesPool.Get().([]record.RefSeries)[:0]
				series, err = dec.Series(rec, series)
				if err != nil {
					errCh <- &wal.CorruptionErr{
						Err:     errors.Wrap(err, "decode series"),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- series
			case record.Samples:
				samples := samplesPool.Get().([]record.RefSample)[:0]
				samples, err = dec.Samples(rec, samples)
				if err != nil {
					errCh <- &wal.CorruptionErr{
						Err:     errors.Wrap(err, "decode samples"),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- samples
			case record.Tombstones:
				// We don't care about tombstones
				continue
			case record.Exemplars:
				// We don't care about exemplars
				continue
			default:
				errCh <- &wal.CorruptionErr{
					Err:     errors.Errorf("invalid record type %v", dec.Type(rec)),
					Segment: r.Segment(),
					Offset:  r.Offset(),
				}
			}
		}
	}()

	var nonExistentSeriesRefs atomic.Uint64

	for d := range decoded {
		switch v := d.(type) {
		case []record.RefSeries:
			for _, entry := range v {
				// If this is a new series, create it in memory. If we never read in a
				// sample for this series, its timestamp will remain at 0 and it will
				// be deleted at the next GC.
				if s.series.GetByID(entry.Ref) == nil {
					series := &memSeries{ref: entry.Ref, lset: entry.Labels, lastTs: 0}
					s.series.Set(entry.Labels.Hash(), series)
					multiRef[entry.Ref] = series.ref
					s.metrics.numActiveSeries.Inc()
					if entry.Ref > lastRef {
						lastRef = entry.Ref
					}
				}
			}

			//nolint:staticcheck
			seriesPool.Put(v)
		case []record.RefSample:
			for _, entry := range v {
				// Update the lastTs for the series based
				ref, ok := multiRef[entry.Ref]
				if !ok {
					nonExistentSeriesRefs.Inc()
					continue
				}
				series := s.series.GetByID(ref)
				if entry.T > series.lastTs {
					series.lastTs = entry.T
				}
			}

			//nolint:staticcheck
			samplesPool.Put(v)
		default:
			panic(fmt.Errorf("unexpected decoded type: %T", d))
		}
	}

	if nonExistentSeriesRefs.Load() > 0 {
		level.Warn(s.logger).Log("msg", "found sample referencing non-existing series, skipping", nonExistentSeriesRefs.Load())
	}

	s.nextRef.Store(lastRef)

	select {
	case err := <-errCh:
		return err
	default:
		if r.Err() != nil {
			return errors.Wrap(r.Err(), "read records")
		}
		return nil
	}
}

func (s *Storage) run() {
	defer close(s.donec)

Loop:
	for {
		select {
		case <-s.stopc:
			break Loop
		case <-time.After(time.Duration(s.opts.TruncateFrequency) * time.Millisecond):
			ts := s.rs.LowestSentTimestamp() - s.opts.MinWALTime
			if ts < 0 {
				ts = 0
			}

			// Clamp timestamp to be the maximum lifetime of WAL data.
			if maxTS := timestamp.FromTime(time.Now()) - s.opts.MaxWALTime; ts < maxTS {
				ts = maxTS
			}

			level.Debug(s.logger).Log("msg", "truncating the WAL", "ts", ts)
			if err := s.truncate(ts); err != nil {
				level.Warn(s.logger).Log("msg", "failed to truncate WAL", "err", err)
			}
		}
	}
}

func (s *Storage) truncate(mint int64) error {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	start := time.Now()

	s.gc(mint)
	level.Info(s.logger).Log("msg", "series GC completed", "duration", time.Since(start))

	first, last, err := wal.Segments(s.wal.Dir())
	if err != nil {
		return errors.Wrap(err, "get segment range")
	}

	// Start a new segment so low ingestion volume instances don't have more WAL
	// than needed.
	err = s.wal.NextSegment()
	if err != nil {
		return errors.Wrap(err, "next segment")
	}

	last-- // Never consider most recent segment for checkpoint
	if last < 0 {
		return nil // no segments yet
	}

	// The lower two-thirds of segments should contain mostly obsolete samples.
	// If we have less than two segments, it's not worth checkpointing yet.
	last = first + (last-first)*2/3
	if last <= first {
		return nil
	}

	keep := func(id uint64) bool {
		if s.series.GetByID(id) != nil {
			return true
		}

		seg, ok := s.deleted[id]
		return ok && seg >= first
	}

	s.metrics.checkpointCreationTotal.Inc()

	if _, err = wal.Checkpoint(s.logger, s.wal, first, last, keep, mint); err != nil {
		s.metrics.checkpointCreationFail.Inc()
		if _, ok := errors.Cause(err).(*wal.CorruptionErr); ok {
			s.metrics.walCorruptionsTotal.Inc()
		}
		return errors.Wrap(err, "create checkpoint")
	}
	if err := s.wal.Truncate(last + 1); err != nil {
		// If truncating fails, we'll just try it again at the next checkpoint.
		// Leftover segments will still just be ignored in the future if there's a
		// checkpoint that supersedes them.
		level.Error(s.logger).Log("msg", "truncating segments failed", "err", err)
	}

	// The checkpoint is written and segments before it are truncated, so we
	// no longer need to track deleted series that were being kept around.
	for ref, segment := range s.deleted {
		if segment < first {
			delete(s.deleted, ref)
		}
	}
	s.metrics.checkpointDeleteTotal.Inc()
	s.metrics.numWALSeriesPendingDeletion.Set(float64(len(s.deleted)))

	if err := wal.DeleteCheckpoints(s.wal.Dir(), last); err != nil {
		// Leftover old checkpoints do not cause problems down the line beyond
		// occupying disk space. They will just be ignored since a newer checkpoint
		// exists.
		level.Error(s.logger).Log("msg", "delete old checkpoints", "err", err)
		s.metrics.checkpointDeleteFail.Inc()
	}

	s.metrics.walTruncateDuration.Observe(time.Since(start).Seconds())

	level.Info(s.logger).Log("msg", "WAL checkpoint complete", "first", first, "last", last, "duration", time.Since(start))
	return nil
}

// gc marks ref IDs that have not received a sample since mint as deleted in
// s.deleted, along with the segment where they originally got deleted.
func (s *Storage) gc(mint int64) {
	deleted := s.series.GC(mint)
	s.metrics.numActiveSeries.Sub(float64(len(deleted)))

	_, last, _ := wal.Segments(s.wal.Dir())

	// We want to keep series records for any newly deleted series
	// until we've passed the last recorded segment. This prevents
	// the WAL having samples for series records that no longer exist.
	for ref := range deleted {
		s.deleted[ref] = last
	}

	s.metrics.numWALSeriesPendingDeletion.Set(float64(len(s.deleted)))
}

// StartTime implements the Storage interface.
func (s *Storage) StartTime() (int64, error) {
	return int64(model.Latest), nil
}

// Querier implements the Storage interface.
func (s *Storage) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	return nil, ErrUnsupported
}

// ChunkQuerier implements the Storage interface.
func (s *Storage) ChunkQuerier(ctx context.Context, mint, maxt int64) (storage.ChunkQuerier, error) {
	return nil, ErrUnsupported
}

// ExemplarQuerier implements the Storage interface.
func (s *Storage) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	return nil, ErrUnsupported
}

// Appender implements storage.Storage.
func (s *Storage) Appender(_ context.Context) storage.Appender {
	return s.appenderPool.Get().(storage.Appender)
}

// Close implements the Storage interface.
func (s *Storage) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	close(s.stopc)
	<-s.donec

	s.metrics.Unregister()

	return s.wal.Close()
}

type appender struct {
	*Storage

	pendingSeries  []record.RefSeries
	pendingSamples []record.RefSample
}

func (a *appender) Append(ref uint64, l labels.Labels, t int64, v float64) (uint64, error) {
	if ref == 0 {
		return a.Add(l, t, v)
	}
	return ref, a.AddFast(ref, t, v)
}

func (a *appender) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	hash := l.Hash()
	series := a.series.GetByHash(hash, l)
	if series != nil {
		return series.ref, a.AddFast(series.ref, t, v)
	}

	// Ensure no empty or duplicate labels have gotten through. This mirrors the
	// equivalent validation code in the TSDB's headAppender.
	l = l.WithoutEmpty()
	if len(l) == 0 {
		return 0, errors.Wrap(tsdb.ErrInvalidSample, "empty labelset")
	}

	if lbl, dup := l.HasDuplicateLabelNames(); dup {
		return 0, errors.Wrap(tsdb.ErrInvalidSample, fmt.Sprintf(`label name "%s" is not unique`, lbl))
	}

	ref := a.nextRef.Inc()
	series = &memSeries{ref: ref, lset: l, lastTs: t}

	a.pendingSeries = append(a.pendingSeries, record.RefSeries{
		Ref:    ref,
		Labels: l,
	})
	a.pendingSamples = append(a.pendingSamples, record.RefSample{
		Ref: ref,
		T:   t,
		V:   v,
	})

	a.series.Set(hash, series)

	a.metrics.numActiveSeries.Inc()
	a.metrics.totalAppendedSamples.Inc()

	return series.ref, nil
}

func (a *appender) AddFast(ref uint64, t int64, v float64) error {
	series := a.series.GetByID(ref)
	if series == nil {
		return storage.ErrNotFound
	}
	series.Lock()
	defer series.Unlock()

	// Update last recorded timestamp. Used by Storage.gc to determine if a
	// series is dead.
	series.lastTs = t

	a.pendingSamples = append(a.pendingSamples, record.RefSample{
		Ref: ref,
		T:   t,
		V:   v,
	})

	a.metrics.totalAppendedSamples.Inc()
	return nil
}

func (a *appender) AppendExemplar(ref uint64, l labels.Labels, e exemplar.Exemplar) (uint64, error) {
	// remote_write doesn't support exemplars yet, so do nothing here.
	return 0, nil
}

// Commit submits the collected samples and purges the batch.
func (a *appender) Commit() error {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	var encoder record.Encoder
	buf := a.bufPool.Get().([]byte)

	if len(a.pendingSeries) > 0 {
		buf = encoder.Series(a.pendingSeries, buf)
		if err := a.wal.Log(buf); err != nil {
			return err
		}
		buf = buf[:0]
	}

	if len(a.pendingSamples) > 0 {
		buf = encoder.Samples(a.pendingSamples, buf)
		if err := a.wal.Log(buf); err != nil {
			return err
		}
		buf = buf[:0]
	}

	//nolint:staticcheck
	a.bufPool.Put(buf)
	return a.Rollback()
}

func (a *appender) Rollback() error {
	a.pendingSeries = a.pendingSeries[:0]
	a.pendingSamples = a.pendingSamples[:0]
	a.appenderPool.Put(a)
	return nil
}
