// Copyright 2017 The Prometheus Authors
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

// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/go-kit/kit/log"
	"github.com/nightlyone/lockfile"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
)

// DefaultOptions used for the DB. They are sane for setups using
// millisecond precision timestampdb.
var DefaultOptions = &Options{
	WALFlushInterval:  5 * time.Second,
	RetentionDuration: 15 * 24 * 60 * 60 * 1000, // 15 days in milliseconds
	BlockRanges:       ExponentialBlockRanges(int64(2*time.Hour)/1e6, 3, 5),
	NoLockfile:        false,
}

// Options of the DB storage.
type Options struct {
	// The interval at which the write ahead log is flushed to disc.
	WALFlushInterval time.Duration

	// Duration of persisted data to keep.
	RetentionDuration uint64

	// The sizes of the Blocks.
	BlockRanges []int64

	// NoLockfile disables creation and consideration of a lock file.
	NoLockfile bool
}

// Appender allows appending a batch of data. It must be completed with a
// call to Commit or Rollback and must not be reused afterwards.
//
// Operations on the Appender interface are not goroutine-safe.
type Appender interface {
	// Add adds a sample pair for the given series. A reference number is
	// returned which can be used to add further samples in the same or later
	// transactions.
	// Returned reference numbers are ephemeral and may be rejected in calls
	// to AddFast() at any point. Adding the sample via Add() returns a new
	// reference number.
	// If the reference is the empty string it must not be used for caching.
	Add(l labels.Labels, t int64, v float64) (string, error)

	// Add adds a sample pair for the referenced series. It is generally faster
	// than adding a sample by providing its full label set.
	AddFast(ref string, t int64, v float64) error

	// Commit submits the collected samples and purges the batch.
	Commit() error

	// Rollback rolls back all modifications made in the appender so far.
	Rollback() error
}

// DB handles reads and writes of time series falling into
// a hashed partition of a seriedb.
type DB struct {
	dir   string
	lockf *lockfile.Lockfile

	logger     log.Logger
	metrics    *dbMetrics
	opts       *Options
	chunkPool  chunks.Pool
	appendPool sync.Pool
	compactor  Compactor
	wal        WAL

	// Mutex for that must be held when modifying the general block layout.
	mtx    sync.RWMutex
	blocks []DiskBlock

	head *Head

	compactc chan struct{}
	donec    chan struct{}
	stopc    chan struct{}

	// cmtx is used to control compactions and deletions.
	cmtx               sync.Mutex
	compactionsEnabled bool
}

type dbMetrics struct {
	activeAppenders     prometheus.Gauge
	loadedBlocks        prometheus.GaugeFunc
	reloads             prometheus.Counter
	reloadsFailed       prometheus.Counter
	walTruncateDuration prometheus.Summary
	samplesAppended     prometheus.Counter

	headSeries        prometheus.Gauge
	headSeriesCreated prometheus.Counter
	headSeriesRemoved prometheus.Counter
	headChunks        prometheus.Gauge
	headChunksCreated prometheus.Gauge
	headChunksRemoved prometheus.Gauge
	headGCDuration    prometheus.Summary
	headMinTime       prometheus.GaugeFunc
	headMaxTime       prometheus.GaugeFunc

	compactionsTriggered prometheus.Counter
}

func newDBMetrics(db *DB, r prometheus.Registerer) *dbMetrics {
	m := &dbMetrics{}

	m.activeAppenders = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_active_appenders",
		Help: "Number of currently active appender transactions",
	})
	m.loadedBlocks = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "tsdb_blocks_loaded",
		Help: "Number of currently loaded data blocks",
	}, func() float64 {
		db.mtx.RLock()
		defer db.mtx.RUnlock()
		return float64(len(db.blocks))
	})
	m.reloads = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsdb_reloads_total",
		Help: "Number of times the database reloaded block data from disk.",
	})
	m.reloadsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsdb_reloads_failures_total",
		Help: "Number of times the database failed to reload black data from disk.",
	})

	m.walTruncateDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "tsdb_wal_truncate_duration_seconds",
		Help: "Duration of WAL truncation.",
	})

	m.headSeries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_series",
		Help: "Total number of series in the head block.",
	})
	m.headSeriesCreated = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_series_created_total",
		Help: "Total number of series created in the head",
	})
	m.headSeriesRemoved = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_series_removed_total",
		Help: "Total number of series removed in the head",
	})
	m.headChunks = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_chunks",
		Help: "Total number of chunks in the head block.",
	})
	m.headChunksCreated = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_chunks_created_total",
		Help: "Total number of chunks created in the head",
	})
	m.headChunksRemoved = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tsdb_head_chunks_removed_total",
		Help: "Total number of chunks removed in the head",
	})
	m.headGCDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "tsdb_head_gc_duration_seconds",
		Help: "Runtime of garbage collection in the head block.",
	})
	m.headMinTime = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "tsdb_head_max_time",
		Help: "Maximum timestamp of the head block.",
	}, func() float64 {
		return float64(db.head.MaxTime())
	})
	m.headMaxTime = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "tsdb_head_min_time",
		Help: "Minimum time bound of the head block.",
	}, func() float64 {
		return float64(db.head.MinTime())
	})

	m.samplesAppended = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsdb_samples_appended_total",
		Help: "Total number of appended sampledb.",
	})
	m.compactionsTriggered = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsdb_compactions_triggered_total",
		Help: "Total number of triggered compactions for the partition.",
	})

	if r != nil {
		r.MustRegister(
			m.activeAppenders,
			m.loadedBlocks,
			m.reloads,
			m.reloadsFailed,
			m.walTruncateDuration,

			m.headChunks,
			m.headChunksCreated,
			m.headChunksRemoved,
			m.headSeries,
			m.headSeriesCreated,
			m.headSeriesRemoved,
			m.headMinTime,
			m.headMaxTime,
			m.headGCDuration,

			m.samplesAppended,
			m.compactionsTriggered,
		)
	}
	return m
}

// Open returns a new DB in the given directory.
func Open(dir string, l log.Logger, r prometheus.Registerer, opts *Options) (db *DB, err error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	if l == nil {
		l = log.NewLogfmtLogger(os.Stdout)
		l = log.With(l, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
	}
	if opts == nil {
		opts = DefaultOptions
	}

	wal, err := OpenSegmentWAL(filepath.Join(dir, "wal"), l, 10*time.Second)
	if err != nil {
		return nil, err
	}

	db = &DB{
		dir:                dir,
		logger:             l,
		opts:               opts,
		wal:                wal,
		compactc:           make(chan struct{}, 1),
		donec:              make(chan struct{}),
		stopc:              make(chan struct{}),
		compactionsEnabled: true,
		chunkPool:          chunks.NewPool(),
	}
	db.metrics = newDBMetrics(db, r)

	if !opts.NoLockfile {
		absdir, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		lockf, err := lockfile.New(filepath.Join(absdir, "lock"))
		if err != nil {
			return nil, err
		}
		if err := lockf.TryLock(); err != nil {
			return nil, errors.Wrapf(err, "open DB in %s", dir)
		}
		db.lockf = &lockf
	}

	copts := &LeveledCompactorOptions{
		blockRanges: opts.BlockRanges,
		chunkPool:   db.chunkPool,
	}

	if len(copts.blockRanges) == 0 {
		return nil, errors.New("at least one block-range must exist")
	}

	for float64(copts.blockRanges[len(copts.blockRanges)-1])/float64(opts.RetentionDuration) > 0.2 {
		if len(copts.blockRanges) == 1 {
			break
		}
		// Max overflow is restricted to 20%.
		copts.blockRanges = copts.blockRanges[:len(copts.blockRanges)-1]
	}

	db.compactor = NewLeveledCompactor(r, l, copts)

	db.head, err = NewHead(l, copts.blockRanges[0])
	if err != nil {
		return nil, err
	}
	if err := db.readWAL(db.wal.Reader()); err != nil {
		return nil, err
	}
	if err := db.reloadBlocks(); err != nil {
		return nil, err
	}

	go db.run()

	return db, nil
}

// Dir returns the directory of the database.
func (db *DB) Dir() string {
	return db.dir
}

func (db *DB) run() {
	defer close(db.donec)

	backoff := time.Duration(0)

	for {
		select {
		case <-db.stopc:
		case <-time.After(backoff):
		}

		select {
		case <-time.After(1 * time.Minute):
			select {
			case db.compactc <- struct{}{}:
			default:
			}
		case <-db.compactc:
			db.metrics.compactionsTriggered.Inc()

			_, err1 := db.retentionCutoff()
			if err1 != nil {
				db.logger.Log("msg", "retention cutoff failed", "err", err1)
			}

			_, err2 := db.compact()
			if err2 != nil {
				db.logger.Log("msg", "compaction failed", "err", err2)
			}

			if err1 != nil || err2 != nil {
				exponential(backoff, 1*time.Second, 1*time.Minute)
			}

		case <-db.stopc:
			return
		}
	}
}

func (db *DB) retentionCutoff() (bool, error) {
	if db.opts.RetentionDuration == 0 {
		return false, nil
	}

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	if len(db.blocks) == 0 {
		return false, nil
	}

	last := db.blocks[len(db.blocks)-1]
	mint := last.Meta().MaxTime - int64(db.opts.RetentionDuration)

	return retentionCutoff(db.dir, mint)
}

func (db *DB) compact() (changes bool, err error) {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	if !db.compactionsEnabled {
		return false, nil
	}

	// Check whether we have pending head blocks that are ready to be persisted.
	// They have the highest priority.
	for {
		select {
		case <-db.stopc:
			return changes, nil
		default:
		}
		// The head has a compactable range if 1.5 level 0 ranges are between the oldest
		// and newest timestamp. The 0.5 acts as a buffer of the appendable window.
		if db.head.MaxTime()-db.head.MinTime() <= db.opts.BlockRanges[0]/2*3 {
			break
		}
		mint, maxt := rangeForTimestamp(db.head.MinTime(), db.opts.BlockRanges[0])

		// Wrap head into a range that bounds all reads to it.
		head := &rangeHead{
			head: db.head,
			mint: mint,
			maxt: maxt,
		}
		if err = db.compactor.Write(db.dir, head, mint, maxt); err != nil {
			return changes, errors.Wrap(err, "persist head block")
		}
		changes = true

		if err := db.reloadBlocks(); err != nil {
			return changes, errors.Wrap(err, "reload blocks")
		}
		runtime.GC()
	}

	// Check for compactions of multiple blocks.
	for {
		plan, err := db.compactor.Plan(db.dir)
		if err != nil {
			return changes, errors.Wrap(err, "plan compaction")
		}
		if len(plan) == 0 {
			break
		}

		select {
		case <-db.stopc:
			return changes, nil
		default:
		}

		if err := db.compactor.Compact(db.dir, plan...); err != nil {
			return changes, errors.Wrapf(err, "compact %s", plan)
		}
		changes = true

		for _, pd := range plan {
			if err := os.RemoveAll(pd); err != nil {
				return changes, errors.Wrap(err, "delete compacted block")
			}
		}

		if err := db.reloadBlocks(); err != nil {
			return changes, errors.Wrap(err, "reload blocks")
		}
		runtime.GC()
	}

	return changes, nil
}

// retentionCutoff deletes all directories of blocks in dir that are strictly
// before mint.
func retentionCutoff(dir string, mint int64) (bool, error) {
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return false, errors.Wrapf(err, "open directory")
	}
	defer df.Close()

	dirs, err := blockDirs(dir)
	if err != nil {
		return false, errors.Wrapf(err, "list block dirs %s", dir)
	}

	changes := false

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return changes, errors.Wrapf(err, "read block meta %s", dir)
		}
		// The first block we encounter marks that we crossed the boundary
		// of deletable blocks.
		if meta.MaxTime >= mint {
			break
		}
		changes = true

		if err := os.RemoveAll(dir); err != nil {
			return changes, err
		}
	}

	return changes, fileutil.Fsync(df)
}

func (db *DB) getBlock(id ulid.ULID) (DiskBlock, bool) {
	for _, b := range db.blocks {
		if b.Meta().ULID == id {
			return b, true
		}
	}
	return nil, false
}

func (db *DB) readWAL(r WALReader) error {

	seriesFunc := func(series []labels.Labels) error {
		for _, lset := range series {
			db.head.create(lset.Hash(), lset)
			db.metrics.headSeries.Inc()
			db.metrics.headSeriesCreated.Inc()
		}
		return nil
	}
	samplesFunc := func(samples []RefSample) error {
		for _, s := range samples {
			ms, ok := db.head.series[uint32(s.Ref)]
			if !ok {
				return errors.Errorf("unknown series reference %d; abort WAL restore", s.Ref)
			}
			_, chunkCreated := ms.append(s.T, s.V)
			if chunkCreated {
				db.metrics.headChunksCreated.Inc()
				db.metrics.headChunks.Inc()
			}
		}

		return nil
	}
	deletesFunc := func(stones []Stone) error {
		for _, s := range stones {
			for _, itv := range s.intervals {
				db.head.tombstones.add(s.ref, itv)
			}
		}

		return nil
	}

	if err := r.Read(seriesFunc, samplesFunc, deletesFunc); err != nil {
		return errors.Wrap(err, "consume WAL")
	}

	return nil

}

func (db *DB) reloadBlocks() (err error) {
	defer func() {
		if err != nil {
			db.metrics.reloadsFailed.Inc()
		}
		db.metrics.reloads.Inc()
	}()

	var cs []io.Closer
	defer func() { closeAll(cs...) }()

	dirs, err := blockDirs(db.dir)
	if err != nil {
		return errors.Wrap(err, "find blocks")
	}
	var (
		blocks []DiskBlock
		exist  = map[ulid.ULID]struct{}{}
	)

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return errors.Wrapf(err, "read meta information %s", dir)
		}

		b, ok := db.getBlock(meta.ULID)
		if !ok {
			b, err = newPersistedBlock(dir, db.chunkPool)
			if err != nil {
				return errors.Wrapf(err, "open block %s", dir)
			}
		}

		blocks = append(blocks, b)
		exist[meta.ULID] = struct{}{}
	}

	if err := validateBlockSequence(blocks); err != nil {
		return errors.Wrap(err, "invalid block sequence")
	}

	// Close all opened blocks that no longer exist after we returned all locks.
	// TODO(fabxc: probably races with querier still reading from them. Can
	// we just abandon them and have the open FDs be GC'd automatically eventually?
	for _, b := range db.blocks {
		if _, ok := exist[b.Meta().ULID]; !ok {
			cs = append(cs, b)
		}
	}

	db.mtx.Lock()
	db.blocks = blocks
	db.mtx.Unlock()

	// Garbage collect data in the head if the most recent persisted block
	// covers data of its current time range.
	if len(blocks) == 0 {
		return
	}
	maxt := blocks[len(db.blocks)-1].Meta().MaxTime
	if maxt <= db.head.MinTime() {
		return
	}
	start := time.Now()
	atomic.StoreInt64(&db.head.minTime, maxt)

	series, chunks := db.head.gc()
	db.metrics.headSeriesRemoved.Add(float64(series))
	db.metrics.headSeries.Sub(float64(series))
	db.metrics.headChunksRemoved.Add(float64(chunks))
	db.metrics.headChunks.Sub(float64(chunks))

	db.logger.Log("msg", "head GC completed", "duration", time.Since(start))

	start = time.Now()

	if err := db.wal.Truncate(maxt); err != nil {
		return errors.Wrapf(err, "truncate WAL at %d", maxt)
	}
	db.metrics.walTruncateDuration.Observe(time.Since(start).Seconds())

	return nil
}

func validateBlockSequence(bs []DiskBlock) error {
	if len(bs) == 0 {
		return nil
	}
	sort.Slice(bs, func(i, j int) bool {
		return bs[i].Meta().MinTime < bs[j].Meta().MinTime
	})
	prev := bs[0]
	for _, b := range bs[1:] {
		if b.Meta().MinTime < prev.Meta().MaxTime {
			return errors.Errorf("block time ranges overlap (%d, %d)", b.Meta().MinTime, prev.Meta().MaxTime)
		}
	}
	return nil
}

// Close the partition.
func (db *DB) Close() error {
	close(db.stopc)
	<-db.donec

	db.mtx.Lock()
	defer db.mtx.Unlock()

	var g errgroup.Group

	// blocks also contains all head blocks.
	for _, pb := range db.blocks {
		g.Go(pb.Close)
	}

	var merr MultiError

	merr.Add(g.Wait())

	if db.lockf != nil {
		merr.Add(db.lockf.Unlock())
	}
	return merr.Err()
}

// DisableCompactions disables compactions.
func (db *DB) DisableCompactions() {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.compactionsEnabled = false
	db.logger.Log("msg", "compactions disabled")
}

// EnableCompactions enables compactions.
func (db *DB) EnableCompactions() {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.compactionsEnabled = true
	db.logger.Log("msg", "compactions enabled")
}

// Snapshot writes the current data to the directory.
func (db *DB) Snapshot(dir string) error {
	// if dir == db.dir {
	// 	return errors.Errorf("cannot snapshot into base directory")
	// }
	// db.cmtx.Lock()
	// defer db.cmtx.Unlock()

	// db.mtx.Lock() // To block any appenders.
	// defer db.mtx.Unlock()

	// blocks := db.blocks[:]
	// for _, b := range blocks {
	// 	db.logger.Log("msg", "snapshotting block", "block", b)
	// 	if err := b.Snapshot(dir); err != nil {
	// 		return errors.Wrap(err, "error snapshotting headblock")
	// 	}
	// }

	return nil
}

// Querier returns a new querier over the data partition for the given time range.
// A goroutine must not handle more than one open Querier.
func (db *DB) Querier(mint, maxt int64) Querier {
	db.mtx.RLock()

	blocks := db.blocksForInterval(mint, maxt)

	sq := &querier{
		blocks: make([]Querier, 0, len(blocks)),
		db:     db,
	}
	for _, b := range blocks {
		sq.blocks = append(sq.blocks, &blockQuerier{
			mint:       mint,
			maxt:       maxt,
			index:      b.Index(),
			chunks:     b.Chunks(),
			tombstones: b.Tombstones(),
		})
	}

	return sq
}

// initAppender is a helper to initialize the time bounds of a the head
// upon the first sample it receives.
type initAppender struct {
	app Appender
	db  *DB
}

func (a *initAppender) Add(lset labels.Labels, t int64, v float64) (string, error) {
	if a.app != nil {
		return a.app.Add(lset, t, v)
	}
	for {
		// In the init state, the head has a high timestamp of math.MinInt64.
		ht := a.db.head.MaxTime()
		if ht != math.MinInt64 {
			break
		}
		cr := a.db.opts.BlockRanges[0]
		mint, _ := rangeForTimestamp(t, cr)

		atomic.CompareAndSwapInt64(&a.db.head.maxTime, ht, t)
		atomic.StoreInt64(&a.db.head.minTime, mint-cr)
	}
	a.app = a.db.appender()

	return a.app.Add(lset, t, v)
}

func (a *initAppender) AddFast(ref string, t int64, v float64) error {
	if a.app == nil {
		return ErrNotFound
	}
	return a.app.AddFast(ref, t, v)
}

func (a *initAppender) Commit() error {
	if a.app == nil {
		return nil
	}
	return a.app.Commit()
}

func (a *initAppender) Rollback() error {
	if a.app == nil {
		return nil
	}
	return a.app.Rollback()
}

// Appender returns a new Appender on the database.
func (db *DB) Appender() Appender {
	db.metrics.activeAppenders.Inc()

	// The head cache might not have a starting point yet. The init appender
	// picks up the first appended timestamp as the base.
	if db.head.MaxTime() == math.MinInt64 {
		return &initAppender{db: db}
	}
	return db.appender()
}

func (db *DB) appender() *dbAppender {
	db.head.mtx.RLock()

	return &dbAppender{
		db:            db,
		head:          db.head,
		wal:           db.wal,
		mint:          db.head.MaxTime() - db.opts.BlockRanges[0]/2,
		samples:       db.getAppendBuffer(),
		highTimestamp: math.MinInt64,
		lowTimestamp:  math.MaxInt64,
	}
}

func (db *DB) getAppendBuffer() []RefSample {
	b := db.appendPool.Get()
	if b == nil {
		return make([]RefSample, 0, 512)
	}
	return b.([]RefSample)
}

func (db *DB) putAppendBuffer(b []RefSample) {
	db.appendPool.Put(b[:0])
}

type dbAppender struct {
	db   *DB
	head *Head
	wal  WAL
	mint int64

	newSeries []*hashedLabels
	newLabels []labels.Labels
	newHashes map[uint64]uint64

	samples       []RefSample
	highTimestamp int64
	lowTimestamp  int64
}

type hashedLabels struct {
	ref    uint64
	hash   uint64
	labels labels.Labels
}

func (a *dbAppender) Add(lset labels.Labels, t int64, v float64) (string, error) {
	if t < a.mint {
		return "", ErrOutOfBounds
	}

	hash := lset.Hash()
	refb := make([]byte, 8)

	// Series exists already in the block.
	if ms := a.head.get(hash, lset); ms != nil {
		binary.BigEndian.PutUint64(refb, uint64(ms.ref))
		return string(refb), a.AddFast(string(refb), t, v)
	}
	// Series was added in this transaction previously.
	if ref, ok := a.newHashes[hash]; ok {
		binary.BigEndian.PutUint64(refb, ref)
		// XXX(fabxc): there's no fast path for multiple samples for the same new series
		// in the same transaction. We always return the invalid empty ref. It's has not
		// been a relevant use case so far and is not worth the trouble.
		return "", a.AddFast(string(refb), t, v)
	}

	// The series is completely new.
	if a.newSeries == nil {
		a.newHashes = map[uint64]uint64{}
	}
	// First sample for new series.
	ref := uint64(len(a.newSeries))

	a.newSeries = append(a.newSeries, &hashedLabels{
		ref:    ref,
		hash:   hash,
		labels: lset,
	})
	// First bit indicates its a series created in this transaction.
	ref |= (1 << 63)

	a.newHashes[hash] = ref
	binary.BigEndian.PutUint64(refb, ref)

	return "", a.AddFast(string(refb), t, v)
}

func (a *dbAppender) AddFast(ref string, t int64, v float64) error {
	if len(ref) != 8 {
		return errors.Wrap(ErrNotFound, "invalid ref length")
	}
	var (
		refn = binary.BigEndian.Uint64(yoloBytes(ref))
		id   = uint32(refn)
		inTx = refn&(1<<63) != 0
	)
	// Distinguish between existing series and series created in
	// this transaction.
	if inTx {
		if id > uint32(len(a.newSeries)-1) {
			return errors.Wrap(ErrNotFound, "transaction series ID too high")
		}
		// TODO(fabxc): we also have to validate here that the
		// sample sequence is valid.
		// We also have to revalidate it as we switch locks and create
		// the new series.
	} else {
		ms, ok := a.head.series[id]
		if !ok {
			return errors.Wrap(ErrNotFound, "unknown series")
		}
		if err := ms.appendable(t, v); err != nil {
			return err
		}
	}
	if t < a.mint {
		return ErrOutOfBounds
	}

	if t > a.highTimestamp {
		a.highTimestamp = t
	}
	// if t < a.lowTimestamp {
	// 	a.lowTimestamp = t
	// }

	a.samples = append(a.samples, RefSample{
		Ref: refn,
		T:   t,
		V:   v,
	})
	return nil
}

func (a *dbAppender) createSeries() error {
	if len(a.newSeries) == 0 {
		return nil
	}
	a.newLabels = make([]labels.Labels, 0, len(a.newSeries))
	base0 := len(a.head.series)

	a.head.mtx.RUnlock()
	defer a.head.mtx.RLock()
	a.head.mtx.Lock()
	defer a.head.mtx.Unlock()

	base1 := len(a.head.series)

	for _, l := range a.newSeries {
		// We switched locks and have to re-validate that the series were not
		// created by another goroutine in the meantime.
		if base1 > base0 {
			if ms := a.head.get(l.hash, l.labels); ms != nil {
				l.ref = uint64(ms.ref)
				continue
			}
		}
		// Series is still new.
		a.newLabels = append(a.newLabels, l.labels)

		s := a.head.create(l.hash, l.labels)
		l.ref = uint64(s.ref)

		a.db.metrics.headSeriesCreated.Inc()
		a.db.metrics.headSeries.Inc()
	}

	// Write all new series to the WAL.
	if err := a.wal.LogSeries(a.newLabels); err != nil {
		return errors.Wrap(err, "WAL log series")
	}

	return nil
}

func (a *dbAppender) Commit() error {
	defer a.head.mtx.RUnlock()

	defer a.db.metrics.activeAppenders.Dec()
	defer a.db.putAppendBuffer(a.samples)

	if err := a.createSeries(); err != nil {
		return err
	}

	// We have to update the refs of samples for series we just created.
	for i := range a.samples {
		s := &a.samples[i]
		if s.Ref&(1<<63) != 0 {
			s.Ref = a.newSeries[(s.Ref<<1)>>1].ref
		}
	}

	// Write all new samples to the WAL and add them to the
	// in-mem database on success.
	if err := a.wal.LogSamples(a.samples); err != nil {
		return errors.Wrap(err, "WAL log samples")
	}

	total := uint64(len(a.samples))

	for _, s := range a.samples {
		series, ok := a.head.series[uint32(s.Ref)]
		if !ok {
			return errors.Errorf("series with ID %d not found", s.Ref)
		}
		ok, chunkCreated := series.append(s.T, s.V)
		if !ok {
			total--
		}
		if chunkCreated {
			a.db.metrics.headChunks.Inc()
			a.db.metrics.headChunksCreated.Inc()
		}
	}

	a.db.metrics.samplesAppended.Add(float64(total))

	for {
		ht := a.head.MaxTime()
		if a.highTimestamp <= ht {
			break
		}
		if a.highTimestamp-a.head.MinTime() > a.head.chunkRange/2*3 {
			select {
			case a.db.compactc <- struct{}{}:
			default:
			}
		}
		if atomic.CompareAndSwapInt64(&a.head.maxTime, ht, a.highTimestamp) {
			break
		}
	}

	return nil
}

func (a *dbAppender) Rollback() error {
	a.head.mtx.RUnlock()

	a.db.metrics.activeAppenders.Dec()
	a.db.putAppendBuffer(a.samples)

	return nil
}

func rangeForTimestamp(t int64, width int64) (mint, maxt int64) {
	mint = (t / width) * width
	return mint, mint + width
}

// Delete implements deletion of metrics. It only has atomicity guarantees on a per-block basis.
func (db *DB) Delete(mint, maxt int64, ms ...labels.Matcher) error {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.mtx.Lock()
	defer db.mtx.Unlock()

	var g errgroup.Group

	for _, b := range db.blocks {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			g.Go(func(b DiskBlock) func() error {
				return func() error { return b.Delete(mint, maxt, ms...) }
			}(b))
		}
	}
	if err := g.Wait(); err != nil {
		return err
	}

	ir := db.head.Index()

	pr := newPostingsReader(ir)
	p, absent := pr.Select(ms...)

	var stones []Stone

Outer:
	for p.Next() {
		series := db.head.series[p.At()]

		for _, abs := range absent {
			if series.lset.Get(abs) != "" {
				continue Outer
			}
		}

		// Delete only until the current values and not beyond.
		t0, t1 := clampInterval(mint, maxt, series.minTime(), series.maxTime())
		stones = append(stones, Stone{p.At(), Intervals{{t0, t1}}})
	}

	if p.Err() != nil {
		return p.Err()
	}
	if err := db.wal.LogDeletes(stones); err != nil {
		return err
	}
	for _, s := range stones {
		db.head.tombstones.add(s.ref, s.intervals[0])
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func intervalOverlap(amin, amax, bmin, bmax int64) bool {
	// Checks Overlap: http://stackoverflow.com/questions/3269434/
	return amin <= bmax && bmin <= amax
}

func intervalContains(min, max, t int64) bool {
	return t >= min && t <= max
}

// blocksForInterval returns all blocks within the partition that may contain
// data for the given time range.
func (db *DB) blocksForInterval(mint, maxt int64) []BlockReader {
	var bs []BlockReader

	for _, b := range db.blocks {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			bs = append(bs, b)
		}
	}
	if maxt >= db.head.MinTime() {
		bs = append(bs, db.head)
	}

	return bs
}

func isBlockDir(fi os.FileInfo) bool {
	if !fi.IsDir() {
		return false
	}
	_, err := ulid.Parse(fi.Name())
	return err == nil
}

func blockDirs(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dirs []string

	for _, fi := range files {
		if isBlockDir(fi) {
			dirs = append(dirs, filepath.Join(dir, fi.Name()))
		}
	}
	return dirs, nil
}

func sequenceFiles(dir, prefix string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var res []string

	for _, fi := range files {
		if isSequenceFile(fi, prefix) {
			res = append(res, filepath.Join(dir, fi.Name()))
		}
	}
	return res, nil
}

func isSequenceFile(fi os.FileInfo, prefix string) bool {
	if !strings.HasPrefix(fi.Name(), prefix) {
		return false
	}
	if _, err := strconv.ParseUint(fi.Name()[len(prefix):], 10, 32); err != nil {
		return false
	}
	return true
}

func nextSequenceFile(dir, prefix string) (string, int, error) {
	names, err := fileutil.ReadDir(dir)
	if err != nil {
		return "", 0, err
	}

	i := uint64(0)
	for _, n := range names {
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		j, err := strconv.ParseUint(n[len(prefix):], 10, 32)
		if err != nil {
			continue
		}
		i = j
	}
	return filepath.Join(dir, fmt.Sprintf("%s%0.6d", prefix, i+1)), int(i + 1), nil
}

// The MultiError type implements the error interface, and contains the
// Errors used to construct it.
type MultiError []error

// Returns a concatenated string of the contained errors
func (es MultiError) Error() string {
	var buf bytes.Buffer

	if len(es) > 1 {
		fmt.Fprintf(&buf, "%d errors: ", len(es))
	}

	for i, err := range es {
		if i != 0 {
			buf.WriteString("; ")
		}
		buf.WriteString(err.Error())
	}

	return buf.String()
}

// Add adds the error to the error list if it is not nil.
func (es *MultiError) Add(err error) {
	if err == nil {
		return
	}
	if merr, ok := err.(MultiError); ok {
		*es = append(*es, merr...)
	} else {
		*es = append(*es, err)
	}
}

// Err returns the error list as an error or nil if it is empty.
func (es MultiError) Err() error {
	if len(es) == 0 {
		return nil
	}
	return es
}

func yoloString(b []byte) string { return *((*string)(unsafe.Pointer(&b))) }
func yoloBytes(s string) []byte  { return *((*[]byte)(unsafe.Pointer(&s))) }

func closeAll(cs ...io.Closer) error {
	var merr MultiError

	for _, c := range cs {
		merr.Add(c.Close())
	}
	return merr.Err()
}

func exponential(d, min, max time.Duration) time.Duration {
	d *= 2
	if d < min {
		d = min
	}
	if d > max {
		d = max
	}
	return d
}
