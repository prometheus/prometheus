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
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/nightlyone/lockfile"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/chunkenc"
	"github.com/prometheus/tsdb/fileutil"
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
	// The interval at which the write ahead log is flushed to disk.
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
	Add(l labels.Labels, t int64, v float64) (uint64, error)

	// Add adds a sample pair for the referenced series. It is generally faster
	// than adding a sample by providing its full label set.
	AddFast(ref uint64, t int64, v float64) error

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
	chunkPool chunkenc.Pool
	compactor Compactor

	// Mutex for that must be held when modifying the general block layout.
	mtx    sync.RWMutex
	blocks []*Block

	head *Head

	compactc chan struct{}
	donec    chan struct{}
	stopc    chan struct{}

	// cmtx is used to control compactions and deletions.
	cmtx               sync.Mutex
	compactionsEnabled bool
}

type dbMetrics struct {
	loadedBlocks         prometheus.GaugeFunc
	reloads              prometheus.Counter
	reloadsFailed        prometheus.Counter
	compactionsTriggered prometheus.Counter
	tombCleanTimer       prometheus.Histogram
}

func newDBMetrics(db *DB, r prometheus.Registerer) *dbMetrics {
	m := &dbMetrics{}

	m.loadedBlocks = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "prometheus_tsdb_blocks_loaded",
		Help: "Number of currently loaded data blocks",
	}, func() float64 {
		db.mtx.RLock()
		defer db.mtx.RUnlock()
		return float64(len(db.blocks))
	})
	m.reloads = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_reloads_total",
		Help: "Number of times the database reloaded block data from disk.",
	})
	m.reloadsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_reloads_failures_total",
		Help: "Number of times the database failed to reload block data from disk.",
	})
	m.compactionsTriggered = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_compactions_triggered_total",
		Help: "Total number of triggered compactions for the partition.",
	})
	m.tombCleanTimer = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "prometheus_tsdb_tombstone_cleanup_seconds",
		Help: "The time taken to recompact blocks to remove tombstones.",
	})

	if r != nil {
		r.MustRegister(
			m.loadedBlocks,
			m.reloads,
			m.reloadsFailed,
			m.compactionsTriggered,
			m.tombCleanTimer,
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
		l = log.NewNopLogger()
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
		chunkPool:          chunkenc.NewPool(),
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

	db.compactor, err = NewLeveledCompactor(r, l, opts.BlockRanges, db.chunkPool)
	if err != nil {
		return nil, errors.Wrap(err, "create leveled compactor")
	}

	wal, err := OpenSegmentWAL(filepath.Join(dir, "wal"), l, opts.WALFlushInterval, r)
	if err != nil {
		return nil, err
	}
	db.head, err = NewHead(r, l, wal, opts.BlockRanges[0])
	if err != nil {
		return nil, err
	}
	if err := db.reload(); err != nil {
		return nil, err
	}
	if err := db.head.ReadWAL(); err != nil {
		return nil, errors.Wrap(err, "read WAL")
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
			return
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
				level.Error(db.logger).Log("msg", "retention cutoff failed", "err", err1)
			}

			_, err2 := db.compact()
			if err2 != nil {
				level.Error(db.logger).Log("msg", "compaction failed", "err", err2)
			}

			if err1 != nil || err2 != nil {
				backoff = exponential(backoff, 1*time.Second, 1*time.Minute)
			} else {
				backoff = 0
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
	blocks := db.blocks[:]
	db.mtx.RUnlock()

	if len(blocks) == 0 {
		return false, nil
	}

	last := blocks[len(db.blocks)-1]

	mint := last.Meta().MaxTime - int64(db.opts.RetentionDuration)
	dirs, err := retentionCutoffDirs(db.dir, mint)
	if err != nil {
		return false, err
	}

	// This will close the dirs and then delete the dirs.
	return len(dirs) > 0, db.reload(dirs...)
}

// Appender opens a new appender against the database.
func (db *DB) Appender() Appender {
	return dbAppender{db: db, Appender: db.head.Appender()}
}

// dbAppender wraps the DB's head appender and triggers compactions on commit
// if necessary.
type dbAppender struct {
	Appender
	db *DB
}

func (a dbAppender) Commit() error {
	err := a.Appender.Commit()

	// We could just run this check every few minutes practically. But for benchmarks
	// and high frequency use cases this is the safer way.
	if a.db.head.MaxTime()-a.db.head.MinTime() > a.db.head.chunkRange/2*3 {
		select {
		case a.db.compactc <- struct{}{}:
		default:
		}
	}
	return err
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
		if _, err = db.compactor.Write(db.dir, head, mint, maxt); err != nil {
			return changes, errors.Wrap(err, "persist head block")
		}
		changes = true

		runtime.GC()

		if err := db.reload(); err != nil {
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
		runtime.GC()

		if err := db.reload(plan...); err != nil {
			return changes, errors.Wrap(err, "reload blocks")
		}
		runtime.GC()
	}

	return changes, nil
}

// retentionCutoffDirs returns all directories of blocks in dir that are strictly
// before mint.
func retentionCutoffDirs(dir string, mint int64) ([]string, error) {
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "open directory")
	}
	defer df.Close()

	dirs, err := blockDirs(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "list block dirs %s", dir)
	}

	delDirs := []string{}

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return nil, errors.Wrapf(err, "read block meta %s", dir)
		}
		// The first block we encounter marks that we crossed the boundary
		// of deletable blocks.
		if meta.MaxTime >= mint {
			break
		}

		delDirs = append(delDirs, dir)
	}

	return delDirs, nil
}

func (db *DB) getBlock(id ulid.ULID) (*Block, bool) {
	for _, b := range db.blocks {
		if b.Meta().ULID == id {
			return b, true
		}
	}
	return nil, false
}

func stringsContain(set []string, elem string) bool {
	for _, e := range set {
		if elem == e {
			return true
		}
	}
	return false
}

// reload on-disk blocks and trigger head truncation if new blocks appeared. It takes
// a list of block directories which should be deleted during reload.
func (db *DB) reload(deleteable ...string) (err error) {
	defer func() {
		if err != nil {
			db.metrics.reloadsFailed.Inc()
		}
		db.metrics.reloads.Inc()
	}()

	dirs, err := blockDirs(db.dir)
	if err != nil {
		return errors.Wrap(err, "find blocks")
	}
	var (
		blocks []*Block
		exist  = map[ulid.ULID]struct{}{}
	)

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return errors.Wrapf(err, "read meta information %s", dir)
		}
		// If the block is pending for deletion, don't add it to the new block set.
		if stringsContain(deleteable, dir) {
			continue
		}

		b, ok := db.getBlock(meta.ULID)
		if !ok {
			b, err = OpenBlock(dir, db.chunkPool)
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

	// Swap in new blocks first for subsequently created readers to be seen.
	// Then close previous blocks, which may block for pending readers to complete.
	db.mtx.Lock()
	oldBlocks := db.blocks
	db.blocks = blocks
	db.mtx.Unlock()

	for _, b := range oldBlocks {
		if _, ok := exist[b.Meta().ULID]; ok {
			continue
		}
		if err := b.Close(); err != nil {
			level.Warn(db.logger).Log("msg", "closing block failed", "err", err)
		}
		if err := os.RemoveAll(b.Dir()); err != nil {
			level.Warn(db.logger).Log("msg", "deleting block failed", "err", err)
		}
	}

	// Garbage collect data in the head if the most recent persisted block
	// covers data of its current time range.
	if len(blocks) == 0 {
		return nil
	}
	maxt := blocks[len(blocks)-1].Meta().MaxTime

	return errors.Wrap(db.head.Truncate(maxt), "head truncate failed")
}

func validateBlockSequence(bs []*Block) error {
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

func (db *DB) String() string {
	return "HEAD"
}

// Blocks returns the databases persisted blocks.
func (db *DB) Blocks() []*Block {
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	return db.blocks
}

// Head returns the databases's head.
func (db *DB) Head() *Head {
	return db.head
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
	merr.Add(db.head.Close())
	return merr.Err()
}

// DisableCompactions disables compactions.
func (db *DB) DisableCompactions() {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.compactionsEnabled = false
	level.Info(db.logger).Log("msg", "compactions disabled")
}

// EnableCompactions enables compactions.
func (db *DB) EnableCompactions() {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.compactionsEnabled = true
	level.Info(db.logger).Log("msg", "compactions enabled")
}

// Snapshot writes the current data to the directory.
func (db *DB) Snapshot(dir string) error {
	if dir == db.dir {
		return errors.Errorf("cannot snapshot into base directory")
	}
	if _, err := ulid.Parse(dir); err == nil {
		return errors.Errorf("dir must not be a valid ULID")
	}

	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	for _, b := range db.blocks {
		level.Info(db.logger).Log("msg", "snapshotting block", "block", b)

		if err := b.Snapshot(dir); err != nil {
			return errors.Wrapf(err, "error snapshotting block: %s", b.Dir())
		}
	}
	_, err := db.compactor.Write(dir, db.head, db.head.MinTime(), db.head.MaxTime())
	return errors.Wrap(err, "snapshot head block")
}

// Querier returns a new querier over the data partition for the given time range.
// A goroutine must not handle more than one open Querier.
func (db *DB) Querier(mint, maxt int64) (Querier, error) {
	var blocks []BlockReader

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	for _, b := range db.blocks {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			blocks = append(blocks, b)
		}
	}
	if maxt >= db.head.MinTime() {
		blocks = append(blocks, db.head)
	}

	sq := &querier{
		blocks: make([]Querier, 0, len(blocks)),
	}
	for _, b := range blocks {
		q, err := NewBlockQuerier(b, mint, maxt)
		if err == nil {
			sq.blocks = append(sq.blocks, q)
			continue
		}
		// If we fail, all previously opened queriers must be closed.
		for _, q := range sq.blocks {
			q.Close()
		}
		return nil, errors.Wrapf(err, "open querier for block %s", b)
	}
	return sq, nil
}

func rangeForTimestamp(t int64, width int64) (mint, maxt int64) {
	mint = (t / width) * width
	return mint, mint + width
}

// Delete implements deletion of metrics. It only has atomicity guarantees on a per-block basis.
func (db *DB) Delete(mint, maxt int64, ms ...labels.Matcher) error {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	var g errgroup.Group

	db.mtx.RLock()
	defer db.mtx.RUnlock()

	for _, b := range db.blocks {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			g.Go(func(b *Block) func() error {
				return func() error { return b.Delete(mint, maxt, ms...) }
			}(b))
		}
	}
	g.Go(func() error {
		return db.head.Delete(mint, maxt, ms...)
	})
	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

// CleanTombstones re-writes any blocks with tombstones.
func (db *DB) CleanTombstones() error {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	start := time.Now()
	defer db.metrics.tombCleanTimer.Observe(float64(time.Since(start).Seconds()))

	db.mtx.RLock()
	blocks := db.blocks[:]
	db.mtx.RUnlock()

	deleted := []string{}
	for _, b := range blocks {
		ok, err := b.CleanTombstones(db.Dir(), db.compactor)
		if err != nil {
			return errors.Wrapf(err, "clean tombstones: %s", b.Dir())
		}

		if ok {
			deleted = append(deleted, b.Dir())
		}
	}

	if len(deleted) == 0 {
		return nil
	}

	return errors.Wrap(db.reload(deleted...), "reload blocks")
}

func intervalOverlap(amin, amax, bmin, bmax int64) bool {
	// Checks Overlap: http://stackoverflow.com/questions/3269434/
	return amin <= bmax && bmin <= amax
}

func intervalContains(min, max, t int64) bool {
	return t >= min && t <= max
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

func sequenceFiles(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var res []string

	for _, fi := range files {
		if _, err := strconv.ParseUint(fi.Name(), 10, 64); err != nil {
			continue
		}
		res = append(res, filepath.Join(dir, fi.Name()))
	}
	return res, nil
}

func nextSequenceFile(dir string) (string, int, error) {
	names, err := fileutil.ReadDir(dir)
	if err != nil {
		return "", 0, err
	}

	i := uint64(0)
	for _, n := range names {
		j, err := strconv.ParseUint(n, 10, 64)
		if err != nil {
			continue
		}
		i = j
	}
	return filepath.Join(dir, fmt.Sprintf("%0.6d", i+1)), int(i + 1), nil
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
