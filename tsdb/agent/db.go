// Copyright The Prometheus Authors
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
	"errors"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/zeropool"
)

const (
	sampleMetricTypeFloat     = "float"
	sampleMetricTypeHistogram = "histogram"
)

var ErrUnsupported = errors.New("unsupported operation with WAL-only storage")

// Default values for options.
var (
	DefaultTruncateFrequency = 2 * time.Hour
	DefaultMinWALTime        = int64(5 * time.Minute / time.Millisecond)
	DefaultMaxWALTime        = int64(4 * time.Hour / time.Millisecond)
)

// Options of the WAL storage.
type Options struct {
	// Segments (wal files) max size.
	// WALSegmentSize <= 0, segment size is default size.
	// WALSegmentSize > 0, segment size is WALSegmentSize.
	WALSegmentSize int

	// WALCompression configures the compression type to use on records in the WAL.
	WALCompression compression.Type

	// StripeSize is the size (power of 2) in entries of the series hash map. Reducing the size will save memory but impact performance.
	StripeSize int

	// TruncateFrequency determines how frequently to truncate data from the WAL.
	TruncateFrequency time.Duration

	// Shortest and longest amount of time data can exist in the WAL before being
	// deleted.
	MinWALTime, MaxWALTime int64

	// NoLockfile disables creation and consideration of a lock file.
	NoLockfile bool

	// OutOfOrderTimeWindow specifies how much out of order is allowed, if any.
	OutOfOrderTimeWindow int64

	// EnableSTAsZeroSample represents 'created-timestamp-zero-ingestion' feature flag.
	// If true, ST, if non-empty and earlier than sample timestamp, will be stored
	// as a zero sample before the actual sample.
	//
	// The zero sample is best-effort, only debug log on failure is emitted.
	// NOTE(bwplotka): This feature might be deprecated and removed once PROM-60
	// is implemented.
	EnableSTAsZeroSample bool
}

// DefaultOptions used for the WAL storage. They are reasonable for setups using
// millisecond-precision timestamps.
func DefaultOptions() *Options {
	return &Options{
		WALSegmentSize:       wlog.DefaultSegmentSize,
		WALCompression:       compression.None,
		StripeSize:           tsdb.DefaultStripeSize,
		TruncateFrequency:    DefaultTruncateFrequency,
		MinWALTime:           DefaultMinWALTime,
		MaxWALTime:           DefaultMaxWALTime,
		NoLockfile:           false,
		OutOfOrderTimeWindow: 0,
	}
}

type dbMetrics struct {
	r prometheus.Registerer

	numActiveSeries             prometheus.Gauge
	numWALSeriesPendingDeletion prometheus.Gauge
	totalAppendedSamples        *prometheus.CounterVec
	totalAppendedExemplars      prometheus.Counter
	totalOutOfOrderSamples      prometheus.Counter
	walTruncateDuration         prometheus.Summary
	walCorruptionsTotal         prometheus.Counter
	walTotalReplayDuration      prometheus.Gauge
	checkpointDeleteFail        prometheus.Counter
	checkpointDeleteTotal       prometheus.Counter
	checkpointCreationFail      prometheus.Counter
	checkpointCreationTotal     prometheus.Counter
}

func newDBMetrics(r prometheus.Registerer) *dbMetrics {
	m := dbMetrics{r: r}
	m.numActiveSeries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_agent_active_series",
		Help: "Number of active series being tracked by the WAL storage",
	})

	m.numWALSeriesPendingDeletion = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_agent_deleted_series",
		Help: "Number of series pending deletion from the WAL",
	})

	m.totalAppendedSamples = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "prometheus_agent_samples_appended_total",
		Help: "Total number of samples appended to the storage",
	}, []string{"type"})

	m.totalAppendedExemplars = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_exemplars_appended_total",
		Help: "Total number of exemplars appended to the storage",
	})

	m.totalOutOfOrderSamples = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_agent_out_of_order_samples_total",
		Help: "Total number of out of order samples ingestion failed attempts.",
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
			m.totalAppendedExemplars,
			m.totalOutOfOrderSamples,
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

func (m *dbMetrics) Unregister() {
	if m.r == nil {
		return
	}
	cs := []prometheus.Collector{
		m.numActiveSeries,
		m.numWALSeriesPendingDeletion,
		m.totalAppendedSamples,
		m.totalAppendedExemplars,
		m.totalOutOfOrderSamples,
		m.walTruncateDuration,
		m.walCorruptionsTotal,
		m.walTotalReplayDuration,
		m.checkpointDeleteFail,
		m.checkpointDeleteTotal,
		m.checkpointCreationFail,
		m.checkpointCreationTotal,
	}
	for _, c := range cs {
		m.r.Unregister(c)
	}
}

// DB represents a WAL-only storage. It implements storage.DB.
type DB struct {
	mtx    sync.RWMutex
	logger *slog.Logger
	opts   *Options
	rs     *remote.Storage

	wal    *wlog.WL
	locker *tsdbutil.DirLocker

	appenderPool   sync.Pool
	appenderV2Pool sync.Pool
	bufPool        sync.Pool

	// These pools are only used during WAL replay and are reset at the end.
	// NOTE: Adjust resetWALReplayResources() upon changes to the pools.
	walReplaySeriesPool          zeropool.Pool[[]record.RefSeries]
	walReplaySamplesPool         zeropool.Pool[[]record.RefSample]
	walReplayHistogramsPool      zeropool.Pool[[]record.RefHistogramSample]
	walReplayFloatHistogramsPool zeropool.Pool[[]record.RefFloatHistogramSample]

	nextRef *atomic.Uint64
	series  *stripeSeries
	// deleted is a map of (ref IDs that should be deleted from WAL) to (the WAL segment they
	// must be kept around to).
	deleted map[chunks.HeadSeriesRef]int

	donec chan struct{}
	stopc chan struct{}

	writeNotified wlog.WriteNotified

	metrics *dbMetrics
}

// Open returns a new agent.DB in the given directory.
func Open(l *slog.Logger, reg prometheus.Registerer, rs *remote.Storage, dir string, opts *Options) (*DB, error) {
	opts = validateOptions(opts)

	locker, err := tsdbutil.NewDirLocker(dir, "agent", l, reg)
	if err != nil {
		return nil, err
	}
	if !opts.NoLockfile {
		if err := locker.Lock(); err != nil {
			return nil, err
		}
	}

	// remote_write expects WAL to be stored in a "wal" subdirectory of the main storage.
	dir = filepath.Join(dir, "wal")

	w, err := wlog.NewSize(l, reg, dir, opts.WALSegmentSize, opts.WALCompression)
	if err != nil {
		return nil, fmt.Errorf("creating WAL: %w", err)
	}

	db := &DB{
		logger: l,
		opts:   opts,
		rs:     rs,

		wal:    w,
		locker: locker,

		nextRef: atomic.NewUint64(0),
		series:  newStripeSeries(opts.StripeSize),
		deleted: make(map[chunks.HeadSeriesRef]int),

		donec: make(chan struct{}),
		stopc: make(chan struct{}),

		metrics: newDBMetrics(reg),
	}

	db.bufPool.New = func() any {
		return make([]byte, 0, 1024)
	}

	db.appenderPool.New = func() any {
		return &appender{
			appenderBase: appenderBase{
				DB:                     db,
				pendingSeries:          make([]record.RefSeries, 0, 100),
				pendingSamples:         make([]record.RefSample, 0, 100),
				pendingHistograms:      make([]record.RefHistogramSample, 0, 100),
				pendingFloatHistograms: make([]record.RefFloatHistogramSample, 0, 100),
				pendingExamplars:       make([]record.RefExemplar, 0, 10),
			},
		}
	}
	db.appenderV2Pool.New = func() any {
		return &appenderV2{
			appenderBase: appenderBase{
				DB:                     db,
				pendingSeries:          make([]record.RefSeries, 0, 100),
				pendingSamples:         make([]record.RefSample, 0, 100),
				pendingHistograms:      make([]record.RefHistogramSample, 0, 100),
				pendingFloatHistograms: make([]record.RefFloatHistogramSample, 0, 100),
				pendingExamplars:       make([]record.RefExemplar, 0, 10),
			},
		}
	}

	if err := db.replayWAL(); err != nil {
		db.logger.Warn("encountered WAL read error, attempting repair", "err", err)
		if err := w.Repair(err); err != nil {
			return nil, fmt.Errorf("repair corrupted WAL: %w", err)
		}
		db.logger.Info("successfully repaired WAL")
	}

	go db.run()
	return db, nil
}

// SetWriteNotified allows to set an instance to notify when a write happens.
// It must be used during initialization. It is not safe to use it during execution.
func (db *DB) SetWriteNotified(wn wlog.WriteNotified) {
	db.writeNotified = wn
}

func validateOptions(opts *Options) *Options {
	if opts == nil {
		opts = DefaultOptions()
	}
	if opts.WALSegmentSize <= 0 {
		opts.WALSegmentSize = wlog.DefaultSegmentSize
	}

	if opts.WALCompression == "" {
		opts.WALCompression = compression.None
	}

	// Revert StripeSize to DefaultStripeSize if StripeSize is either 0 or not a power of 2.
	if opts.StripeSize <= 0 || ((opts.StripeSize & (opts.StripeSize - 1)) != 0) {
		opts.StripeSize = tsdb.DefaultStripeSize
	}
	if opts.TruncateFrequency <= 0 {
		opts.TruncateFrequency = DefaultTruncateFrequency
	}
	if opts.MinWALTime <= 0 {
		opts.MinWALTime = DefaultMinWALTime
	}
	if opts.MaxWALTime <= 0 {
		opts.MaxWALTime = DefaultMaxWALTime
	}
	if opts.MinWALTime > opts.MaxWALTime {
		opts.MaxWALTime = opts.MinWALTime
	}

	if t := int64(opts.TruncateFrequency / time.Millisecond); opts.MaxWALTime < t {
		opts.MaxWALTime = t
	}
	return opts
}

func (db *DB) replayWAL() error {
	db.logger.Info("replaying WAL, this may take a while", "dir", db.wal.Dir())
	defer db.resetWALReplayResources()
	start := time.Now()

	dir, startFrom, err := wlog.LastCheckpoint(db.wal.Dir())
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return fmt.Errorf("find last checkpoint: %w", err)
	}

	multiRef := map[chunks.HeadSeriesRef]chunks.HeadSeriesRef{}

	if err == nil {
		sr, err := wlog.NewSegmentsReader(dir)
		if err != nil {
			return fmt.Errorf("open checkpoint: %w", err)
		}
		defer func() {
			if err := sr.Close(); err != nil {
				db.logger.Warn("error while closing the wal segments reader", "err", err)
			}
		}()

		// A corrupted checkpoint is a hard error for now and requires user
		// intervention. There's likely little data that can be recovered anyway.
		if err := db.loadWAL(wlog.NewReader(sr), multiRef); err != nil {
			return fmt.Errorf("backfill checkpoint: %w", err)
		}
		startFrom++
		db.logger.Info("WAL checkpoint loaded")
	}

	// Find the last segment.
	_, last, err := wlog.Segments(db.wal.Dir())
	if err != nil {
		return fmt.Errorf("finding WAL segments: %w", err)
	}

	// Backfill segments from the most recent checkpoint onwards.
	for i := startFrom; i <= last; i++ {
		seg, err := wlog.OpenReadSegment(wlog.SegmentName(db.wal.Dir(), i))
		if err != nil {
			return fmt.Errorf("open WAL segment: %d: %w", i, err)
		}

		sr := wlog.NewSegmentBufReader(seg)
		err = db.loadWAL(wlog.NewReader(sr), multiRef)
		if err := sr.Close(); err != nil {
			db.logger.Warn("error while closing the wal segments reader", "err", err)
		}
		if err != nil {
			return err
		}
		db.logger.Info("WAL segment loaded", "segment", i, "maxSegment", last)
	}

	walReplayDuration := time.Since(start)
	db.metrics.walTotalReplayDuration.Set(walReplayDuration.Seconds())

	return nil
}

func (db *DB) resetWALReplayResources() {
	db.walReplaySeriesPool = zeropool.Pool[[]record.RefSeries]{}
	db.walReplaySamplesPool = zeropool.Pool[[]record.RefSample]{}
	db.walReplayHistogramsPool = zeropool.Pool[[]record.RefHistogramSample]{}
	db.walReplayFloatHistogramsPool = zeropool.Pool[[]record.RefFloatHistogramSample]{}
}

func (db *DB) loadWAL(r *wlog.Reader, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef) (err error) {
	var (
		syms    = labels.NewSymbolTable() // One table for the whole WAL.
		dec     = record.NewDecoder(syms, db.logger)
		lastRef = chunks.HeadSeriesRef(db.nextRef.Load())

		decoded = make(chan any, 10)
		errCh   = make(chan error, 1)
	)

	go func() {
		defer close(decoded)
		var err error
		for r.Next() {
			rec := r.Record()
			switch dec.Type(rec) {
			case record.Series:
				series := db.walReplaySeriesPool.Get()[:0]
				series, err = dec.Series(rec, series)
				if err != nil {
					errCh <- &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode series: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- series
			case record.Samples:
				samples := db.walReplaySamplesPool.Get()[:0]
				samples, err = dec.Samples(rec, samples)
				if err != nil {
					errCh <- &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode samples: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- samples
			case record.HistogramSamples, record.CustomBucketsHistogramSamples:
				histograms := db.walReplayHistogramsPool.Get()[:0]
				histograms, err = dec.HistogramSamples(rec, histograms)
				if err != nil {
					errCh <- &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode histogram samples: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- histograms
			case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
				floatHistograms := db.walReplayFloatHistogramsPool.Get()[:0]
				floatHistograms, err = dec.FloatHistogramSamples(rec, floatHistograms)
				if err != nil {
					errCh <- &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode float histogram samples: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- floatHistograms
			case record.Tombstones, record.Exemplars:
				// We don't care about tombstones or exemplars during replay.
				// TODO: If decide to decode exemplars, we should make sure to prepopulate
				// stripeSeries.exemplars in the next block by using setLatestExemplar.
				continue
			default:
				errCh <- &wlog.CorruptionErr{
					Err:     fmt.Errorf("invalid record type %v", dec.Type(rec)),
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
				if db.series.GetByID(entry.Ref) == nil {
					series := &memSeries{ref: entry.Ref, lset: entry.Labels, lastTs: 0}
					db.series.Set(entry.Labels.Hash(), series)
					multiRef[entry.Ref] = series.ref
					db.metrics.numActiveSeries.Inc()
					if entry.Ref > lastRef {
						lastRef = entry.Ref
					}
				}
			}
			db.walReplaySeriesPool.Put(v)
		case []record.RefSample:
			for _, entry := range v {
				// Update the lastTs for the series based
				ref, ok := multiRef[entry.Ref]
				if !ok {
					nonExistentSeriesRefs.Inc()
					continue
				}
				series := db.series.GetByID(ref)
				if entry.T > series.lastTs {
					series.lastTs = entry.T
				}
			}
			db.walReplaySamplesPool.Put(v)
		case []record.RefHistogramSample:
			for _, entry := range v {
				// Update the lastTs for the series based
				ref, ok := multiRef[entry.Ref]
				if !ok {
					nonExistentSeriesRefs.Inc()
					continue
				}
				series := db.series.GetByID(ref)
				if entry.T > series.lastTs {
					series.lastTs = entry.T
				}
			}
			db.walReplayHistogramsPool.Put(v)
		case []record.RefFloatHistogramSample:
			for _, entry := range v {
				// Update the lastTs for the series based
				ref, ok := multiRef[entry.Ref]
				if !ok {
					nonExistentSeriesRefs.Inc()
					continue
				}
				series := db.series.GetByID(ref)
				if entry.T > series.lastTs {
					series.lastTs = entry.T
				}
			}
			db.walReplayFloatHistogramsPool.Put(v)
		default:
			panic(fmt.Errorf("unexpected decoded type: %T", d))
		}
	}

	if v := nonExistentSeriesRefs.Load(); v > 0 {
		db.logger.Warn("found sample referencing non-existing series", "skipped_series", v)
	}

	db.nextRef.Store(uint64(lastRef))

	select {
	case err := <-errCh:
		return err
	default:
		if r.Err() != nil {
			return fmt.Errorf("read records: %w", r.Err())
		}
		return nil
	}
}

func (db *DB) run() {
	defer close(db.donec)

Loop:
	for {
		select {
		case <-db.stopc:
			break Loop
		case <-time.After(db.opts.TruncateFrequency):
			// The timestamp ts is used to determine which series are not receiving
			// samples and may be deleted from the WAL. Their most recent append
			// timestamp is compared to ts, and if that timestamp is older then ts,
			// they are considered inactive and may be deleted.
			//
			// Subtracting a duration from ts will add a buffer for when series are
			// considered inactive and safe for deletion.
			ts := max(db.rs.LowestSentTimestamp()-db.opts.MinWALTime, 0)

			// Network issues can prevent the result of getRemoteWriteTimestamp from
			// changing. We don't want data in the WAL to grow forever, so we set a cap
			// on the maximum age data can be. If our ts is older than this cutoff point,
			// we'll shift it forward to start deleting very stale data.
			if maxTS := timestamp.FromTime(time.Now()) - db.opts.MaxWALTime; ts < maxTS {
				ts = maxTS
			}

			db.logger.Debug("truncating the WAL", "ts", ts)
			if err := db.truncate(ts); err != nil {
				db.logger.Warn("failed to truncate WAL", "err", err)
			}
		}
	}
}

// keepSeriesInWALCheckpointFn returns a function that is used to determine whether a series record should be kept in the checkpoint.
// last is the last WAL segment that was considered for checkpointing.
// NOTE: the agent implementation here is different from the Prometheus implementation, in that it uses WAL segment numbers instead of timestamps.
func (db *DB) keepSeriesInWALCheckpointFn(last int) func(id chunks.HeadSeriesRef) bool {
	return func(id chunks.HeadSeriesRef) bool {
		// Keep the record if the series exists in the db.
		if db.series.GetByID(id) != nil {
			return true
		}

		// Keep the record if the series was recently deleted.
		seg, ok := db.deleted[id]
		return ok && seg > last
	}
}

func (db *DB) truncate(mint int64) error {
	db.logger.Info("series GC started")
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	start := time.Now()

	db.gc(mint)
	db.logger.Info("series GC completed", "duration", time.Since(start))

	first, last, err := wlog.Segments(db.wal.Dir())
	if err != nil {
		return fmt.Errorf("get segment range: %w", err)
	}

	// Start a new segment so low ingestion volume instances don't have more WAL
	// than needed.
	if _, err := db.wal.NextSegment(); err != nil {
		return fmt.Errorf("next segment: %w", err)
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

	db.metrics.checkpointCreationTotal.Inc()

	if _, err = wlog.Checkpoint(db.logger, db.wal, first, last, db.keepSeriesInWALCheckpointFn(last), mint); err != nil {
		db.metrics.checkpointCreationFail.Inc()
		var cerr *wlog.CorruptionErr
		if errors.As(err, &cerr) {
			db.metrics.walCorruptionsTotal.Inc()
		}
		return fmt.Errorf("create checkpoint: %w", err)
	}
	if err := db.wal.Truncate(last + 1); err != nil {
		// If truncating fails, we'll just try it again at the next checkpoint.
		// Leftover segments will still just be ignored in the future if there's a
		// checkpoint that supersedes them.
		db.logger.Error("truncating segments failed", "err", err)
	}

	// The checkpoint is written and segments before it are truncated, so we
	// no longer need to track deleted series that were being kept around.
	for ref, segment := range db.deleted {
		if segment <= last {
			delete(db.deleted, ref)
		}
	}
	db.metrics.checkpointDeleteTotal.Inc()
	db.metrics.numWALSeriesPendingDeletion.Set(float64(len(db.deleted)))

	if err := wlog.DeleteCheckpoints(db.wal.Dir(), last); err != nil {
		// Leftover old checkpoints do not cause problems down the line beyond
		// occupying disk space. They will just be ignored since a newer checkpoint
		// exists.
		db.logger.Error("delete old checkpoints", "err", err)
		db.metrics.checkpointDeleteFail.Inc()
	}

	db.metrics.walTruncateDuration.Observe(time.Since(start).Seconds())

	db.logger.Info("WAL checkpoint complete", "first", first, "last", last, "duration", time.Since(start))
	return nil
}

// gc marks ref IDs that have not received a sample since mint as deleted in
// s.deleted, along with the segment where they originally got deleted.
func (db *DB) gc(mint int64) {
	deleted := db.series.GC(mint)
	db.metrics.numActiveSeries.Sub(float64(len(deleted)))

	_, last, _ := wlog.Segments(db.wal.Dir())

	// We want to keep series records for any newly deleted series
	// until we've passed the last recorded segment. This prevents
	// the WAL having samples for series records that no longer exist.
	for ref := range deleted {
		db.deleted[ref] = last
	}

	db.metrics.numWALSeriesPendingDeletion.Set(float64(len(db.deleted)))
}

// StartTime implements the Storage interface.
func (*DB) StartTime() (int64, error) {
	return int64(model.Latest), nil
}

// Querier implements the Storage interface.
func (*DB) Querier(int64, int64) (storage.Querier, error) {
	return nil, ErrUnsupported
}

// ChunkQuerier implements the Storage interface.
func (*DB) ChunkQuerier(int64, int64) (storage.ChunkQuerier, error) {
	return nil, ErrUnsupported
}

// ExemplarQuerier implements the Storage interface.
func (*DB) ExemplarQuerier(context.Context) (storage.ExemplarQuerier, error) {
	return nil, ErrUnsupported
}

// Appender implements storage.Storage.
func (db *DB) Appender(context.Context) storage.Appender {
	return db.appenderPool.Get().(storage.Appender)
}

// Close implements the Storage interface.
func (db *DB) Close() error {
	db.mtx.Lock()
	defer db.mtx.Unlock()

	close(db.stopc)
	<-db.donec

	db.metrics.Unregister()

	return errors.Join(db.locker.Release(), db.wal.Close())
}

type appenderBase struct {
	*DB

	pendingSeries          []record.RefSeries
	pendingSamples         []record.RefSample
	pendingHistograms      []record.RefHistogramSample
	pendingFloatHistograms []record.RefFloatHistogramSample
	pendingExamplars       []record.RefExemplar

	// Pointers to the series referenced by each element of pendingSamples.
	// Series lock is not held on elements.
	sampleSeries []*memSeries

	// Pointers to the series referenced by each element of pendingHistograms.
	// Series lock is not held on elements.
	histogramSeries []*memSeries

	// Pointers to the series referenced by each element of pendingFloatHistograms.
	// Series lock is not held on elements.
	floatHistogramSeries []*memSeries
}

type appender struct {
	appenderBase

	hints *storage.AppendOptions
}

func (a *appender) SetOptions(opts *storage.AppendOptions) {
	a.hints = opts
}

func (a *appender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	// series references and chunk references are identical for agent mode.
	headRef := chunks.HeadSeriesRef(ref)

	series := a.series.GetByID(headRef)
	if series == nil {
		var err error
		series, err = a.getOrCreate(l)
		if err != nil {
			return 0, err
		}
	}

	series.Lock()
	defer series.Unlock()

	if t <= a.minValidTime(series.lastTs) {
		a.metrics.totalOutOfOrderSamples.Inc()
		return 0, storage.ErrOutOfOrderSample
	}

	// NOTE: always modify pendingSamples and sampleSeries together.
	a.pendingSamples = append(a.pendingSamples, record.RefSample{
		Ref: series.ref,
		T:   t,
		V:   v,
	})
	a.sampleSeries = append(a.sampleSeries, series)

	a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricTypeFloat).Inc()
	return storage.SeriesRef(series.ref), nil
}

func (a *appenderBase) getOrCreate(l labels.Labels) (series *memSeries, err error) {
	// Ensure no empty or duplicate labels have gotten through. This mirrors the
	// equivalent validation code in the TSDB's headAppender.
	l = l.WithoutEmpty()
	if l.IsEmpty() {
		return nil, fmt.Errorf("empty labelset: %w", tsdb.ErrInvalidSample)
	}

	if lbl, dup := l.HasDuplicateLabelNames(); dup {
		return nil, fmt.Errorf(`label name "%s" is not unique: %w`, lbl, tsdb.ErrInvalidSample)
	}

	hash := l.Hash()

	series = a.series.GetByHash(hash, l)
	if series != nil {
		return series, nil
	}

	ref := chunks.HeadSeriesRef(a.nextRef.Inc())
	series = &memSeries{ref: ref, lset: l, lastTs: math.MinInt64}
	a.series.Set(hash, series)

	a.pendingSeries = append(a.pendingSeries, record.RefSeries{
		Ref:    series.ref,
		Labels: l,
	})
	a.metrics.numActiveSeries.Inc()
	return series, nil
}

func (a *appender) AppendExemplar(ref storage.SeriesRef, _ labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	// Series references and chunk references are identical for agent mode.
	headRef := chunks.HeadSeriesRef(ref)

	s := a.series.GetByID(headRef)
	if s == nil {
		return 0, fmt.Errorf("unknown series ref when trying to add exemplar: %d", ref)
	}

	// Ensure no empty labels have gotten through.
	e.Labels = e.Labels.WithoutEmpty()

	if err := a.validateExemplar(s.ref, e); err != nil {
		if errors.Is(err, storage.ErrDuplicateExemplar) {
			// Duplicate, don't return an error but don't accept the exemplar.
			return 0, nil
		}
		return 0, err
	}

	a.series.SetLatestExemplar(s.ref, &e)
	a.pendingExamplars = append(a.pendingExamplars, record.RefExemplar{
		Ref:    s.ref,
		T:      e.Ts,
		V:      e.Value,
		Labels: e.Labels,
	})
	a.metrics.totalAppendedExemplars.Inc()
	return storage.SeriesRef(s.ref), nil
}

func (a *appenderBase) validateExemplar(ref chunks.HeadSeriesRef, e exemplar.Exemplar) error {
	if lbl, dup := e.Labels.HasDuplicateLabelNames(); dup {
		return fmt.Errorf(`label name "%s" is not unique: %w`, lbl, tsdb.ErrInvalidExemplar)
	}

	// Exemplar label length does not include chars involved in text rendering such as quotes
	// equals sign, or commas. See definition of const ExemplarMaxLabelLength.
	labelSetLen := 0
	if err := e.Labels.Validate(func(l labels.Label) error {
		labelSetLen += utf8.RuneCountInString(l.Name)
		labelSetLen += utf8.RuneCountInString(l.Value)
		if labelSetLen > exemplar.ExemplarMaxLabelSetLength {
			return storage.ErrExemplarLabelLength
		}
		return nil
	}); err != nil {
		return err
	}
	// Check for duplicate vs last stored exemplar for this series, and discard those.
	// Otherwise, record the current exemplar as the latest.
	// Prometheus' TSDB returns 0 when encountering duplicates, so we do the same here.
	prevExemplar := a.series.GetLatestExemplar(ref)
	if prevExemplar != nil && prevExemplar.Equals(e) {
		return storage.ErrDuplicateExemplar
	}
	return nil
}

func (a *appender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if h != nil {
		if err := h.Validate(); err != nil {
			return 0, err
		}
	}

	if fh != nil {
		if err := fh.Validate(); err != nil {
			return 0, err
		}
	}

	// series references and chunk references are identical for agent mode.
	headRef := chunks.HeadSeriesRef(ref)

	series := a.series.GetByID(headRef)
	if series == nil {
		var err error
		series, err = a.getOrCreate(l)
		if err != nil {
			return 0, err
		}
	}

	series.Lock()
	defer series.Unlock()

	if t <= a.minValidTime(series.lastTs) {
		a.metrics.totalOutOfOrderSamples.Inc()
		return 0, storage.ErrOutOfOrderSample
	}

	switch {
	case h != nil:
		// NOTE: always modify pendingHistograms and histogramSeries together
		a.pendingHistograms = append(a.pendingHistograms, record.RefHistogramSample{
			Ref: series.ref,
			T:   t,
			H:   h,
		})
		a.histogramSeries = append(a.histogramSeries, series)
	case fh != nil:
		// NOTE: always modify pendingFloatHistograms and floatHistogramSeries together
		a.pendingFloatHistograms = append(a.pendingFloatHistograms, record.RefFloatHistogramSample{
			Ref: series.ref,
			T:   t,
			FH:  fh,
		})
		a.floatHistogramSeries = append(a.floatHistogramSeries, series)
	}

	a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
	return storage.SeriesRef(series.ref), nil
}

func (*appender) UpdateMetadata(storage.SeriesRef, labels.Labels, metadata.Metadata) (storage.SeriesRef, error) {
	// TODO: Wire metadata in the Agent's appender.
	return 0, nil
}

func (a *appender) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if h != nil {
		if err := h.Validate(); err != nil {
			return 0, err
		}
	}
	if fh != nil {
		if err := fh.Validate(); err != nil {
			return 0, err
		}
	}
	if st >= t {
		return 0, storage.ErrSTNewerThanSample
	}

	series := a.series.GetByID(chunks.HeadSeriesRef(ref))
	if series == nil {
		var err error
		series, err = a.getOrCreate(l)
		if err != nil {
			return 0, err
		}
	}

	series.Lock()
	defer series.Unlock()

	if st <= a.minValidTime(series.lastTs) {
		return 0, storage.ErrOutOfOrderST
	}

	if st <= series.lastTs {
		// discard the sample if it's out of order.
		return 0, storage.ErrOutOfOrderST
	}
	// NOTE(bwplotka): This is a bug, as we "commit" pending sample TS as the WAL last TS. It was likely done
	// to satisfy incorrect TestDBStartTimestampSamplesIngestion test. We are leaving it as-is given the planned removal
	// of AppenderV1 as per https://github.com/prometheus/prometheus/issues/17632.
	series.lastTs = st

	switch {
	case h != nil:
		zeroHistogram := &histogram.Histogram{}
		a.pendingHistograms = append(a.pendingHistograms, record.RefHistogramSample{
			Ref: series.ref,
			T:   st,
			H:   zeroHistogram,
		})
		a.histogramSeries = append(a.histogramSeries, series)
	case fh != nil:
		a.pendingFloatHistograms = append(a.pendingFloatHistograms, record.RefFloatHistogramSample{
			Ref: series.ref,
			T:   st,
			FH:  &histogram.FloatHistogram{},
		})
		a.floatHistogramSeries = append(a.floatHistogramSeries, series)
	}

	a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricTypeHistogram).Inc()
	return storage.SeriesRef(series.ref), nil
}

func (a *appender) AppendSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64) (storage.SeriesRef, error) {
	if st >= t {
		return 0, storage.ErrSTNewerThanSample
	}

	series := a.series.GetByID(chunks.HeadSeriesRef(ref))
	if series == nil {
		var err error
		series, err = a.getOrCreate(l)
		if err != nil {
			return 0, err
		}
	}

	series.Lock()
	defer series.Unlock()

	if t <= a.minValidTime(series.lastTs) {
		a.metrics.totalOutOfOrderSamples.Inc()
		return 0, storage.ErrOutOfOrderSample
	}

	if st <= series.lastTs {
		// discard the sample if it's out of order.
		return 0, storage.ErrOutOfOrderST
	}
	// NOTE(bwplotka): This is a bug, as we "commit" pending sample TS as the WAL last TS. It was likely done
	// to satisfy incorrect TestDBStartTimestampSamplesIngestion test. We are leaving it as-is given the planned removal
	// of AppenderV1 as per https://github.com/prometheus/prometheus/issues/17632.
	series.lastTs = st

	// NOTE: always modify pendingSamples and sampleSeries together.
	a.pendingSamples = append(a.pendingSamples, record.RefSample{
		Ref: series.ref,
		T:   st,
		V:   0,
	})
	a.sampleSeries = append(a.sampleSeries, series)

	a.metrics.totalAppendedSamples.WithLabelValues(sampleMetricTypeFloat).Inc()

	return storage.SeriesRef(series.ref), nil
}

// Commit submits the collected samples and purges the batch.
func (a *appender) Commit() error {
	defer a.appenderPool.Put(a)
	return a.commit()
}

func (a *appender) Rollback() error {
	defer a.appenderPool.Put(a)
	return a.rollback()
}

func (a *appenderBase) commit() error {
	if err := a.log(); err != nil {
		return err
	}

	a.clearData()

	if a.writeNotified != nil {
		a.writeNotified.Notify()
	}
	return nil
}

// log logs all pending data to the WAL.
func (a *appenderBase) log() error {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	var encoder record.Encoder
	buf := a.bufPool.Get().([]byte)
	defer func() {
		a.bufPool.Put(buf) //nolint:staticcheck
	}()

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

	if len(a.pendingHistograms) > 0 {
		var customBucketsHistograms []record.RefHistogramSample
		buf, customBucketsHistograms = encoder.HistogramSamples(a.pendingHistograms, buf)
		if len(buf) > 0 {
			if err := a.wal.Log(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
		if len(customBucketsHistograms) > 0 {
			buf = encoder.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
			if err := a.wal.Log(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}

	if len(a.pendingFloatHistograms) > 0 {
		var customBucketsFloatHistograms []record.RefFloatHistogramSample
		buf, customBucketsFloatHistograms = encoder.FloatHistogramSamples(a.pendingFloatHistograms, buf)
		if len(buf) > 0 {
			if err := a.wal.Log(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
		if len(customBucketsFloatHistograms) > 0 {
			buf = encoder.CustomBucketsFloatHistogramSamples(customBucketsFloatHistograms, nil)
			if err := a.wal.Log(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}

	if len(a.pendingExamplars) > 0 {
		buf = encoder.Exemplars(a.pendingExamplars, buf)
		if err := a.wal.Log(buf); err != nil {
			return err
		}
		buf = buf[:0]
	}

	var series *memSeries
	for i, s := range a.pendingSamples {
		series = a.sampleSeries[i]
		if !series.updateTimestamp(s.T) {
			a.metrics.totalOutOfOrderSamples.Inc()
		}
	}
	for i, s := range a.pendingHistograms {
		series = a.histogramSeries[i]
		if !series.updateTimestamp(s.T) {
			a.metrics.totalOutOfOrderSamples.Inc()
		}
	}
	for i, s := range a.pendingFloatHistograms {
		series = a.floatHistogramSeries[i]
		if !series.updateTimestamp(s.T) {
			a.metrics.totalOutOfOrderSamples.Inc()
		}
	}

	return nil
}

// clearData clears all pending data.
func (a *appenderBase) clearData() {
	a.pendingSeries = a.pendingSeries[:0]
	a.pendingSamples = a.pendingSamples[:0]
	a.pendingHistograms = a.pendingHistograms[:0]
	a.pendingFloatHistograms = a.pendingFloatHistograms[:0]
	a.pendingExamplars = a.pendingExamplars[:0]
	a.sampleSeries = a.sampleSeries[:0]
	a.histogramSeries = a.histogramSeries[:0]
	a.floatHistogramSeries = a.floatHistogramSeries[:0]
}

func (a *appenderBase) rollback() error {
	// Series are created in-memory regardless of rollback. This means we must
	// log them to the WAL, otherwise subsequent commits may reference a series
	// which was never written to the WAL.
	if err := a.logSeries(); err != nil {
		return err
	}

	a.clearData()
	return nil
}

// logSeries logs only pending series records to the WAL.
func (a *appenderBase) logSeries() error {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	if len(a.pendingSeries) > 0 {
		buf := a.bufPool.Get().([]byte)
		defer func() {
			a.bufPool.Put(buf) //nolint:staticcheck
		}()

		var encoder record.Encoder
		buf = encoder.Series(a.pendingSeries, buf)
		if err := a.wal.Log(buf); err != nil {
			return err
		}
		buf = buf[:0]
	}

	return nil
}

// minValidTime returns the minimum timestamp that a sample can have
// and is needed for preventing underflow.
func (a *appenderBase) minValidTime(lastTs int64) int64 {
	if lastTs < math.MinInt64+a.opts.OutOfOrderTimeWindow {
		return math.MinInt64
	}

	return lastTs - a.opts.OutOfOrderTimeWindow
}
