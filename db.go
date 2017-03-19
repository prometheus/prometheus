// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
	"github.com/nightlyone/lockfile"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// DefaultOptions used for the DB. They are sane for setups using
// millisecond precision timestampdb.
var DefaultOptions = &Options{
	WALFlushInterval:  5 * time.Second,
	RetentionDuration: 15 * 24 * 60 * 60 * 1000, // 15 days in milliseconds
	MinBlockDuration:  3 * 60 * 60 * 1000,       // 2 hours in milliseconds
	MaxBlockDuration:  24 * 60 * 60 * 1000,      // 1 days in milliseconds
	AppendableBlocks:  2,
}

// Options of the DB storage.
type Options struct {
	// The interval at which the write ahead log is flushed to disc.
	WALFlushInterval time.Duration

	// Duration of persisted data to keep.
	RetentionDuration uint64

	// The timestamp range of head blocks after which they get persisted.
	// It's the minimum duration of any persisted block.
	MinBlockDuration uint64

	// The maximum timestamp range of compacted blocks.
	MaxBlockDuration uint64

	// Number of head blocks that can be appended to.
	// Should be two or higher to prevent write errors in general scenarios.
	//
	// After a new block is started for timestamp t0 or higher, appends with
	// timestamps as early as t0 - (n-1) * MinBlockDuration are valid.
	AppendableBlocks int
}

// Appender allows appending a batch of data. It must be completed with a
// call to Commit or Rollback and must not be reused afterwards.
type Appender interface {
	// Add adds a sample pair for the given series. A reference number is
	// returned which can be used to add further samples in the same or later
	// transactions.
	// Returned reference numbers are ephemeral and may be rejected in calls
	// to AddFast() at any point. Adding the sample via Add() returns a new
	// reference number.
	Add(l labels.Labels, t int64, v float64) (uint64, error)

	// Add adds a sample pair for the referenced series. It is generally faster
	// than adding a sample by providing its full label set.
	AddFast(ref uint64, t int64, v float64) error

	// Commit submits the collected samples and purges the batch.
	Commit() error

	// Rollback rolls back all modifications made in the appender so far.
	Rollback() error
}

const sep = '\xff'

// DB handles reads and writes of time series falling into
// a hashed partition of a seriedb.
type DB struct {
	dir   string
	lockf lockfile.Lockfile

	logger  log.Logger
	metrics *dbMetrics
	opts    *Options

	// Mutex for that must be held when modifying the general
	// block layout.
	mtx       sync.RWMutex
	persisted []*persistedBlock
	seqBlocks map[int]Block

	// Mutex that must be held when modifying just the head blocks
	// or the general layout.
	headmtx sync.RWMutex
	heads   []*headBlock
	headGen uint8

	compactor Compactor

	compactc chan struct{}
	donec    chan struct{}
	stopc    chan struct{}
}

type dbMetrics struct {
	samplesAppended      prometheus.Counter
	compactionsTriggered prometheus.Counter
}

func newDBMetrics(r prometheus.Registerer) *dbMetrics {
	m := &dbMetrics{}

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

	if l == nil {
		l = log.NewLogfmtLogger(os.Stdout)
		l = log.With(l, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
	}

	if opts == nil {
		opts = DefaultOptions
	}
	if opts.AppendableBlocks < 1 {
		return nil, errors.Errorf("AppendableBlocks must be greater than 0")
	}

	db = &DB{
		dir:      dir,
		lockf:    lockf,
		logger:   l,
		metrics:  newDBMetrics(r),
		opts:     opts,
		compactc: make(chan struct{}, 1),
		donec:    make(chan struct{}),
		stopc:    make(chan struct{}),
	}
	db.compactor = newCompactor(r, &compactorOptions{
		maxBlockRange: opts.MaxBlockDuration,
	})

	if err := db.reloadBlocks(); err != nil {
		return nil, err
	}
	go db.run()

	return db, nil
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

	// We don't count the span covered by head blocks towards the
	// retention time as it generally makes up a fraction of it.
	if len(db.persisted) == 0 {
		return false, nil
	}

	last := db.persisted[len(db.persisted)-1]
	mint := last.Meta().MaxTime - int64(db.opts.RetentionDuration)

	return retentionCutoff(db.dir, mint)
}

func (db *DB) compact() (changes bool, err error) {
	db.headmtx.RLock()

	// Check whether we have pending head blocks that are ready to be persisted.
	// They have the highest priority.
	var singles []*headBlock

	// Collect head blocks that are ready for compaction. Write them after
	// returning the lock to not block Appenders.
	// Selected blocks are semantically ensured to not be written to afterwards
	// by appendable().
	if len(db.heads) > db.opts.AppendableBlocks {
		for _, h := range db.heads[:len(db.heads)-db.opts.AppendableBlocks] {
			// Blocks that won't be appendable when instantiating a new appender
			// might still have active appenders on them.
			// Abort at the first one we encounter.
			if atomic.LoadUint64(&h.activeWriters) > 0 {
				break
			}
			singles = append(singles, h)
		}
	}

	db.headmtx.RUnlock()

Loop:
	for _, h := range singles {
		db.logger.Log("msg", "write head", "seq", h.Meta().Sequence)

		select {
		case <-db.stopc:
			break Loop
		default:
		}

		if err = db.compactor.Write(h.Dir(), h); err != nil {
			return changes, errors.Wrap(err, "persist head block")
		}
		changes = true
	}

	// Check for compactions of multiple blocks.
	for {
		plans, err := db.compactor.Plan(db.dir)
		if err != nil {
			return changes, errors.Wrap(err, "plan compaction")
		}

		select {
		case <-db.stopc:
			return false, nil
		default:
		}
		// We just execute compactions sequentially to not cause too extreme
		// CPU and memory spikes.
		// TODO(fabxc): return more descriptive plans in the future that allow
		// estimation of resource usage and conditional parallelization?
		for _, p := range plans {
			db.logger.Log("msg", "compact blocks", "seq", fmt.Sprintf("%v", p))

			if err := db.compactor.Compact(p...); err != nil {
				return changes, errors.Wrapf(err, "compact %s", p)
			}
			changes = true
		}
		// If we didn't compact anything, there's nothing left to do.
		if len(plans) == 0 {
			break
		}
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

func (db *DB) reloadBlocks() error {
	var cs []io.Closer
	defer closeAll(cs...)

	db.mtx.Lock()
	defer db.mtx.Unlock()

	db.headmtx.Lock()
	defer db.headmtx.Unlock()

	dirs, err := blockDirs(db.dir)
	if err != nil {
		return errors.Wrap(err, "find blocks")
	}
	var (
		metas     []*BlockMeta
		persisted []*persistedBlock
		heads     []*headBlock
		seqBlocks = make(map[int]Block, len(dirs))
	)

	for _, dir := range dirs {
		meta, err := readMetaFile(dir)
		if err != nil {
			return errors.Wrapf(err, "read meta information %s", dir)
		}
		metas = append(metas, meta)
	}

	for i, meta := range metas {
		b, ok := db.seqBlocks[meta.Sequence]

		if meta.Compaction.Generation == 0 {
			if !ok {
				b, err = openHeadBlock(dirs[i], db.logger)
				if err != nil {
					return errors.Wrapf(err, "load head at %s", dirs[i])
				}
			}
			if meta.ULID != b.Meta().ULID {
				return errors.Errorf("head block ULID changed unexpectedly")
			}
			heads = append(heads, b.(*headBlock))
		} else {
			if !ok || meta.ULID != b.Meta().ULID {
				b, err = newPersistedBlock(dirs[i])
				if err != nil {
					return errors.Wrapf(err, "open persisted block %s", dirs[i])
				}
			}
			persisted = append(persisted, b.(*persistedBlock))
		}

		seqBlocks[meta.Sequence] = b
	}

	// Close all blocks that we no longer need. They are closed after returning all
	// locks to avoid questionable locking order.
	for seq, b := range db.seqBlocks {
		if nb, ok := seqBlocks[seq]; !ok || nb != b {
			cs = append(cs, b)
		}
	}

	db.seqBlocks = seqBlocks
	db.persisted = persisted
	db.heads = heads

	return nil
}

// Close the partition.
func (db *DB) Close() error {
	close(db.stopc)
	<-db.donec

	db.mtx.Lock()
	defer db.mtx.Unlock()

	var g errgroup.Group

	for _, pb := range db.persisted {
		g.Go(pb.Close)
	}
	for _, hb := range db.heads {
		g.Go(hb.Close)
	}

	var merr MultiError

	merr.Add(g.Wait())
	merr.Add(db.lockf.Unlock())

	return merr.Err()
}

// Appender returns a new Appender on the database.
func (db *DB) Appender() Appender {
	db.mtx.RLock()
	a := &dbAppender{db: db}

	// Only instantiate appender after returning the headmtx to avoid
	// questionable locking order.
	db.headmtx.RLock()

	app := db.appendable()
	heads := make([]*headBlock, len(app))
	copy(heads, app)

	db.headmtx.RUnlock()

	for _, b := range heads {
		a.heads = append(a.heads, b.Appender().(*headAppender))
	}

	return a
}

type dbAppender struct {
	db      *DB
	heads   []*headAppender
	samples int
}

func (a *dbAppender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	h, err := a.appenderFor(t)
	if err != nil {
		return 0, err
	}
	ref, err := h.Add(lset, t, v)
	if err != nil {
		return 0, err
	}
	a.samples++
	return ref | (uint64(h.generation) << 40), nil
}

func (a *dbAppender) AddFast(ref uint64, t int64, v float64) error {
	// We store the head generation in the 4th byte and use it to reject
	// stale references.
	gen := uint8((ref << 16) >> 56)

	h, err := a.appenderFor(t)
	if err != nil {
		return err
	}
	// If the reference pointed into a previous block, we cannot
	// use it to append the sample.
	if h.generation != gen {
		return ErrNotFound
	}
	if err := h.AddFast(ref, t, v); err != nil {
		return err
	}

	a.samples++
	return nil
}

// appenderFor gets the appender for the head containing timestamp t.
// If the head block doesn't exist yet, it gets created.
func (a *dbAppender) appenderFor(t int64) (*headAppender, error) {
	// If there's no fitting head block for t, ensure it gets created.
	if len(a.heads) == 0 || t >= a.heads[len(a.heads)-1].meta.MaxTime {
		a.db.headmtx.Lock()

		var newHeads []*headBlock

		if err := a.db.ensureHead(t); err != nil {
			a.db.headmtx.Unlock()
			return nil, err
		}
		if len(a.heads) == 0 {
			newHeads = append(newHeads, a.db.appendable()...)
		} else {
			maxSeq := a.heads[len(a.heads)-1].meta.Sequence
			for _, b := range a.db.appendable() {
				if b.meta.Sequence > maxSeq {
					newHeads = append(newHeads, b)
				}
			}
		}

		a.db.headmtx.Unlock()

		// Instantiate appenders after returning headmtx to avoid questionable
		// locking order.
		for _, b := range newHeads {
			a.heads = append(a.heads, b.Appender().(*headAppender))
		}
	}
	for i := len(a.heads) - 1; i >= 0; i-- {
		if h := a.heads[i]; t >= h.meta.MinTime {
			return h, nil
		}
	}

	return nil, ErrNotFound
}

// ensureHead makes sure that there is a head block for the timestamp t if
// it is within or after the currently appendable window.
func (db *DB) ensureHead(t int64) error {
	// Initial case for a new database: we must create the first
	// AppendableBlocks-1 front padding heads.
	if len(db.heads) == 0 {
		for i := int64(db.opts.AppendableBlocks - 1); i >= 0; i-- {
			if _, err := db.cut(t - i*int64(db.opts.MinBlockDuration)); err != nil {
				return err
			}
		}
	}

	for {
		h := db.heads[len(db.heads)-1]
		// If t doesn't exceed the range of heads blocks, there's nothing to do.
		if t < h.meta.MaxTime {
			return nil
		}
		if _, err := db.cut(h.meta.MaxTime); err != nil {
			return err
		}
	}
}

func (a *dbAppender) Commit() error {
	var merr MultiError

	for _, h := range a.heads {
		merr.Add(h.Commit())
	}
	a.db.mtx.RUnlock()

	if merr.Err() == nil {
		a.db.metrics.samplesAppended.Add(float64(a.samples))
	}
	return merr.Err()
}

func (a *dbAppender) Rollback() error {
	var merr MultiError

	for _, h := range a.heads {
		merr.Add(h.Rollback())
	}
	a.db.mtx.RUnlock()

	return merr.Err()
}

func (db *DB) appendable() []*headBlock {
	if len(db.heads) <= db.opts.AppendableBlocks {
		return db.heads
	}
	return db.heads[len(db.heads)-db.opts.AppendableBlocks:]
}

func intervalOverlap(amin, amax, bmin, bmax int64) bool {
	if bmin >= amin && bmin <= amax {
		return true
	}
	if amin >= bmin && amin <= bmax {
		return true
	}
	return false
}

func intervalContains(min, max, t int64) bool {
	return t >= min && t <= max
}

// blocksForInterval returns all blocks within the partition that may contain
// data for the given time range.
func (db *DB) blocksForInterval(mint, maxt int64) []Block {
	var bs []Block

	for _, b := range db.persisted {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			bs = append(bs, b)
		}
	}
	for _, b := range db.heads {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			bs = append(bs, b)
		}
	}

	return bs
}

// cut starts a new head block to append to. The completed head block
// will still be appendable for the configured grace period.
func (db *DB) cut(mint int64) (*headBlock, error) {
	maxt := mint + int64(db.opts.MinBlockDuration)

	dir, seq, err := nextSequenceFile(db.dir, "b-")
	if err != nil {
		return nil, err
	}
	newHead, err := createHeadBlock(dir, seq, db.logger, mint, maxt)
	if err != nil {
		return nil, err
	}

	db.heads = append(db.heads, newHead)
	db.seqBlocks[seq] = newHead
	db.headGen++

	newHead.generation = db.headGen

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
	if !strings.HasPrefix(fi.Name(), "b-") {
		return false
	}
	if _, err := strconv.ParseUint(fi.Name()[2:], 10, 32); err != nil {
		return false
	}
	return true
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

func yoloString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}

func closeAll(cs ...io.Closer) error {
	var merr MultiError

	for _, c := range cs {
		merr.Add(c.Close())
	}
	return merr.Err()
}
