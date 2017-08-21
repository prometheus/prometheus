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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	logger    log.Logger
	metrics   *dbMetrics
	opts      *Options
	chunkPool chunks.Pool

	// Mutex for that must be held when modifying the general block layout.
	mtx    sync.RWMutex
	blocks []Block

	// Mutex that must be held when modifying just the head blocks
	// or the general layout.
	// mtx must be held before acquiring.
	headmtx sync.RWMutex
	heads   []headBlock

	compactor Compactor

	compactc chan struct{}
	donec    chan struct{}
	stopc    chan struct{}

	// cmtx is used to control compactions and deletions.
	cmtx               sync.Mutex
	compactionsEnabled bool
}

type dbMetrics struct {
	activeAppenders      prometheus.Gauge
	loadedBlocks         prometheus.GaugeFunc
	reloads              prometheus.Counter
	reloadsFailed        prometheus.Counter
	reloadDuration       prometheus.Summary
	samplesAppended      prometheus.Counter
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
	m.reloadDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "tsdb_reload_duration_seconds",
		Help: "Duration of block reloads.",
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
			m.reloadDuration,
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

	db = &DB{
		dir:                dir,
		logger:             l,
		opts:               opts,
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

	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			select {
			case db.compactc <- struct{}{}:
			default:
			}
		case <-db.compactc:
			db.metrics.compactionsTriggered.Inc()

			changes1, err := db.retentionCutoff()
			if err != nil {
				db.logger.Log("msg", "retention cutoff failed", "err", err)
			}

			changes2, err := db.compact()
			if err != nil {
				db.logger.Log("msg", "compaction failed", "err", err)
			}

			if changes1 || changes2 {
				if err := db.reloadBlocks(); err != nil {
					db.logger.Log("msg", "reloading blocks failed", "err", err)
				}
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

	// We only consider the already persisted blocks. Head blocks generally
	// only account for a fraction of the total data.
	db.headmtx.RLock()
	lenp := len(db.blocks) - len(db.heads)
	db.headmtx.RUnlock()

	if lenp == 0 {
		return false, nil
	}

	last := db.blocks[lenp-1]
	mint := last.Meta().MaxTime - int64(db.opts.RetentionDuration)

	return retentionCutoff(db.dir, mint)
}

// headFullness returns up to which fraction of a blocks time range samples
// were already inserted.
func headFullness(h headBlock) float64 {
	m := h.Meta()
	a := float64(h.HighTimestamp() - m.MinTime)
	b := float64(m.MaxTime - m.MinTime)
	return a / b
}

// appendableHeads returns a copy of a slice of HeadBlocks that can still be appended to.
func (db *DB) appendableHeads() (r []headBlock) {
	switch l := len(db.heads); l {
	case 0:
	case 1:
		r = append(r, db.heads[0])
	default:
		if headFullness(db.heads[l-1]) < 0.5 {
			r = append(r, db.heads[l-2])
		}
		r = append(r, db.heads[l-1])
	}
	return r
}

func (db *DB) completedHeads() (r []headBlock) {
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	db.headmtx.RLock()
	defer db.headmtx.RUnlock()

	if len(db.heads) < 2 {
		return nil
	}

	// Select all old heads unless they still have pending appenders.
	for _, h := range db.heads[:len(db.heads)-2] {
		if h.ActiveWriters() > 0 {
			return r
		}
		r = append(r, h)
	}
	// Add the 2nd last head if the last head is more than 50% filled.
	// Compacting it early allows us to free its memory before allocating
	// more for the next block and thus reduces spikes.
	h0 := db.heads[len(db.heads)-1]
	h1 := db.heads[len(db.heads)-2]

	if headFullness(h0) >= 0.5 && h1.ActiveWriters() == 0 {
		r = append(r, h1)
	}
	return r
}

func (db *DB) compact() (changes bool, err error) {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	if !db.compactionsEnabled {
		return false, nil
	}

	// Check whether we have pending head blocks that are ready to be persisted.
	// They have the highest priority.
	for _, h := range db.completedHeads() {
		select {
		case <-db.stopc:
			return changes, nil
		default:
		}

		if err = db.compactor.Write(db.dir, h); err != nil {
			return changes, errors.Wrap(err, "persist head block")
		}
		changes = true

		if err := os.RemoveAll(h.Dir()); err != nil {
			return changes, errors.Wrap(err, "delete compacted head block")
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

func (db *DB) getBlock(id ulid.ULID) (Block, bool) {
	for _, b := range db.blocks {
		if b.Meta().ULID == id {
			return b, true
		}
	}
	return nil, false
}

func (db *DB) reloadBlocks() (err error) {
	defer func(t time.Time) {
		if err != nil {
			db.metrics.reloadsFailed.Inc()
		}
		db.metrics.reloads.Inc()
		db.metrics.reloadDuration.Observe(time.Since(t).Seconds())
	}(time.Now())

	var cs []io.Closer
	defer func() { closeAll(cs...) }()

	db.mtx.Lock()
	defer db.mtx.Unlock()

	db.headmtx.Lock()
	defer db.headmtx.Unlock()

	dirs, err := blockDirs(db.dir)
	if err != nil {
		return errors.Wrap(err, "find blocks")
	}
	var (
		blocks []Block
		exist  = map[ulid.ULID]struct{}{}
	)

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return errors.Wrapf(err, "read meta information %s", dir)
		}

		b, ok := db.getBlock(meta.ULID)
		if !ok {
			if meta.Compaction.Level == 0 {
				b, err = db.openHeadBlock(dir)
			} else {
				b, err = newPersistedBlock(dir, db.chunkPool)
			}
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
	for _, b := range db.blocks {
		if _, ok := exist[b.Meta().ULID]; !ok {
			cs = append(cs, b)
		}
	}

	db.blocks = blocks
	db.heads = nil

	for _, b := range blocks {
		if b.Meta().Compaction.Level == 0 {
			db.heads = append(db.heads, b.(*HeadBlock))
		}
	}

	return nil
}

func validateBlockSequence(bs []Block) error {
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
	if dir == db.dir {
		return errors.Errorf("cannot snapshot into base directory")
	}
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.mtx.Lock() // To block any appenders.
	defer db.mtx.Unlock()

	blocks := db.blocks[:]
	for _, b := range blocks {
		db.logger.Log("msg", "snapshotting block", "block", b)
		if err := b.Snapshot(dir); err != nil {
			return errors.Wrap(err, "error snapshotting headblock")
		}
	}

	return nil
}

// Appender returns a new Appender on the database.
func (db *DB) Appender() Appender {
	db.metrics.activeAppenders.Inc()

	db.mtx.RLock()
	return &dbAppender{db: db}
}

type dbAppender struct {
	db    *DB
	heads []*metaAppender

	samples int
}

type metaAppender struct {
	meta BlockMeta
	app  Appender
}

func (a *dbAppender) Add(lset labels.Labels, t int64, v float64) (string, error) {
	h, err := a.appenderAt(t)
	if err != nil {
		return "", err
	}
	ref, err := h.app.Add(lset, t, v)
	if err != nil {
		return "", err
	}
	a.samples++

	if ref == "" {
		return "", nil
	}
	return string(append(h.meta.ULID[:], ref...)), nil
}

func (a *dbAppender) AddFast(ref string, t int64, v float64) error {
	if len(ref) < 16 {
		return errors.Wrap(ErrNotFound, "invalid ref length")
	}
	// The first 16 bytes a ref hold the ULID of the head block.
	h, err := a.appenderAt(t)
	if err != nil {
		return err
	}
	// Validate the ref points to the same block we got for t.
	if string(h.meta.ULID[:]) != ref[:16] {
		return ErrNotFound
	}
	if err := h.app.AddFast(ref[16:], t, v); err != nil {
		// The block the ref points to might fit the given timestamp.
		// We mask the error to stick with our contract.
		if errors.Cause(err) == ErrOutOfBounds {
			err = ErrNotFound
		}
		return err
	}

	a.samples++
	return nil
}

// appenderFor gets the appender for the head containing timestamp t.
// If the head block doesn't exist yet, it gets created.
func (a *dbAppender) appenderAt(t int64) (*metaAppender, error) {
	for _, h := range a.heads {
		if intervalContains(h.meta.MinTime, h.meta.MaxTime-1, t) {
			return h, nil
		}
	}
	// Currently opened appenders do not cover t. Ensure the head block is
	// created and add missing appenders.
	a.db.headmtx.Lock()

	if err := a.db.ensureHead(t); err != nil {
		a.db.headmtx.Unlock()
		return nil, err
	}

	var hb headBlock
	for _, h := range a.db.appendableHeads() {
		m := h.Meta()

		if intervalContains(m.MinTime, m.MaxTime-1, t) {
			hb = h
			break
		}
	}
	a.db.headmtx.Unlock()

	if hb == nil {
		return nil, ErrOutOfBounds
	}
	// Instantiate appender after returning headmtx!
	app := &metaAppender{
		meta: hb.Meta(),
		app:  hb.Appender(),
	}
	a.heads = append(a.heads, app)

	return app, nil
}

func rangeForTimestamp(t int64, width int64) (mint, maxt int64) {
	mint = (t / width) * width
	return mint, mint + width
}

// ensureHead makes sure that there is a head block for the timestamp t if
// it is within or after the currently appendable window.
func (db *DB) ensureHead(t int64) error {
	var (
		mint, maxt = rangeForTimestamp(t, int64(db.opts.BlockRanges[0]))
		addBuffer  = len(db.blocks) == 0
		last       BlockMeta
	)

	if !addBuffer {
		last = db.blocks[len(db.blocks)-1].Meta()
		addBuffer = last.MaxTime <= mint-int64(db.opts.BlockRanges[0])
	}
	// Create another block of buffer in front if the DB is initialized or retrieving
	// new data after a long gap.
	// This ensures we always have a full block width of append window.
	if addBuffer {
		if _, err := db.createHeadBlock(mint-int64(db.opts.BlockRanges[0]), mint); err != nil {
			return err
		}
		// If the previous block reaches into our new window, make it smaller.
	} else if mt := last.MaxTime; mt > mint {
		mint = mt
	}
	if mint >= maxt {
		return nil
	}
	// Error if the requested time for a head is before the appendable window.
	if len(db.heads) > 0 && t < db.heads[0].Meta().MinTime {
		return ErrOutOfBounds
	}

	_, err := db.createHeadBlock(mint, maxt)
	return err
}

func (a *dbAppender) Commit() error {
	defer a.db.metrics.activeAppenders.Dec()
	defer a.db.mtx.RUnlock()

	// Commits to partial appenders must be concurrent as concurrent appenders
	// may have conflicting locks on head appenders.
	// For high-throughput use cases the errgroup causes significant blocking. Typically,
	// we just deal with a single appender and special case it.
	var err error

	switch len(a.heads) {
	case 1:
		err = a.heads[0].app.Commit()
	default:
		var g errgroup.Group
		for _, h := range a.heads {
			g.Go(h.app.Commit)
		}
		err = g.Wait()
	}

	if err != nil {
		return err
	}
	// XXX(fabxc): Push the metric down into head block to account properly
	// for partial appends?
	a.db.metrics.samplesAppended.Add(float64(a.samples))

	return nil
}

func (a *dbAppender) Rollback() error {
	defer a.db.metrics.activeAppenders.Dec()
	defer a.db.mtx.RUnlock()

	var g errgroup.Group

	for _, h := range a.heads {
		g.Go(h.app.Rollback)
	}

	return g.Wait()
}

// Delete implements deletion of metrics.
func (db *DB) Delete(mint, maxt int64, ms ...labels.Matcher) error {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.mtx.Lock()
	defer db.mtx.Unlock()

	blocks := db.blocksForInterval(mint, maxt)

	var g errgroup.Group

	for _, b := range blocks {
		g.Go(func(b Block) func() error {
			return func() error { return b.Delete(mint, maxt, ms...) }
		}(b))
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
func (db *DB) blocksForInterval(mint, maxt int64) []Block {
	var bs []Block

	for _, b := range db.blocks {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			bs = append(bs, b)
		}
	}

	return bs
}

// openHeadBlock opens the head block at dir.
func (db *DB) openHeadBlock(dir string) (*HeadBlock, error) {
	var (
		wdir = walDir(dir)
		l    = log.With(db.logger, "wal", wdir)
	)
	wal, err := OpenSegmentWAL(wdir, l, 5*time.Second)
	if err != nil {
		return nil, errors.Wrapf(err, "open WAL %s", dir)
	}

	h, err := OpenHeadBlock(dir, log.With(db.logger, "block", dir), wal, db.compactor)
	if err != nil {
		return nil, errors.Wrapf(err, "open head block %s", dir)
	}
	return h, nil
}

// createHeadBlock starts a new head block to append to.
func (db *DB) createHeadBlock(mint, maxt int64) (headBlock, error) {
	dir, err := TouchHeadBlock(db.dir, mint, maxt)
	if err != nil {
		return nil, errors.Wrapf(err, "touch head block %s", dir)
	}
	newHead, err := db.openHeadBlock(dir)
	if err != nil {
		return nil, err
	}

	db.logger.Log("msg", "created head block", "ulid", newHead.meta.ULID, "mint", mint, "maxt", maxt)

	db.blocks = append(db.blocks, newHead) // TODO(fabxc): this is a race!
	db.heads = append(db.heads, newHead)

	select {
	case db.compactc <- struct{}{}:
	default:
	}

	return newHead, nil
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
