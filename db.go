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
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/go-kit/kit/log"
	"github.com/nightlyone/lockfile"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/labels"
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
	mtx    sync.RWMutex
	blocks []Block

	// Mutex that must be held when modifying just the head blocks
	// or the general layout.
	// Must never be held when acquiring a blocks's mutex!
	headmtx sync.RWMutex
	heads   []HeadBlock

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
	db.compactor = newCompactor(r, l, &compactorOptions{
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

func (db *DB) compact() (changes bool, err error) {
	db.headmtx.RLock()

	// Check whether we have pending head blocks that are ready to be persisted.
	// They have the highest priority.
	var singles []Block

	// Collect head blocks that are ready for compaction. Write them after
	// returning the lock to not block Appenders.
	// Selected blocks are semantically ensured to not be written to afterwards
	// by appendable().
	if len(db.heads) > db.opts.AppendableBlocks {
		for _, h := range db.heads[:len(db.heads)-db.opts.AppendableBlocks] {
			// Blocks that won't be appendable when instantiating a new appender
			// might still have active appenders on them.
			// Abort at the first one we encounter.
			if h.Busy() {
				break
			}
			singles = append(singles, h)
		}
	}

	db.headmtx.RUnlock()

	for _, h := range singles {
		select {
		case <-db.stopc:
			return changes, nil
		default:
		}

		if err = db.compactor.Write(h.Dir(), h); err != nil {
			return changes, errors.Wrap(err, "persist head block")
		}
		changes = true
		runtime.GC()
	}

	// Check for compactions of multiple blocks.
	for {
		plans, err := db.compactor.Plan(db.dir)
		if err != nil {
			return changes, errors.Wrap(err, "plan compaction")
		}
		if len(plans) == 0 {
			break
		}

		select {
		case <-db.stopc:
			return changes, nil
		default:
		}

		// We just execute compactions sequentially to not cause too extreme
		// CPU and memory spikes.
		// TODO(fabxc): return more descriptive plans in the future that allow
		// estimation of resource usage and conditional parallelization?
		for _, p := range plans {
			if err := db.compactor.Compact(p...); err != nil {
				return changes, errors.Wrapf(err, "compact %s", p)
			}
			changes = true
			runtime.GC()
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

func (db *DB) seqBlock(i int) (Block, bool) {
	for _, b := range db.blocks {
		if b.Meta().Sequence == i {
			return b, true
		}
	}
	return nil, false
}

func (db *DB) reloadBlocks() error {
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
		metas     []*BlockMeta
		blocks    []Block
		heads     []HeadBlock
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
		b, ok := db.seqBlock(meta.Sequence)

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
			heads = append(heads, b.(HeadBlock))
		} else {
			if !ok || meta.ULID != b.Meta().ULID {
				b, err = newPersistedBlock(dirs[i])
				if err != nil {
					return errors.Wrapf(err, "open persisted block %s", dirs[i])
				}
			}
		}

		seqBlocks[meta.Sequence] = b
		blocks = append(blocks, b)
	}

	// Close all blocks that we no longer need. They are closed after returning all
	// locks to avoid questionable locking order.
	for _, b := range db.blocks {
		if nb, ok := seqBlocks[b.Meta().Sequence]; !ok || nb != b {
			cs = append(cs, b)
		}
	}

	db.blocks = blocks
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

	// blocks also contains all head blocks.
	for _, pb := range db.blocks {
		g.Go(pb.Close)
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

	// XXX(fabxc): turn off creating initial appender as it will happen on-demand
	// anyway. For now this, with combination of only having a single timestamp per batch,
	// prevents opening more than one appender and hitting an unresolved deadlock (#11).
	//

	// Only instantiate appender after returning the headmtx to avoid
	// questionable locking order.
	db.headmtx.RLock()
	app := db.appendable()
	db.headmtx.RUnlock()

	for _, b := range app {
		a.heads = append(a.heads, &metaAppender{
			meta: b.Meta(),
			app:  b.Appender(),
		})
	}

	return a
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

func (a *dbAppender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	h, err := a.appenderFor(t)
	if err != nil {
		return 0, err
	}
	ref, err := h.app.Add(lset, t, v)
	if err != nil {
		return 0, err
	}
	a.samples++
	// Store last byte of sequence number in 3rd byte of refernece.
	return ref | (uint64(h.meta.Sequence^0xff) << 40), nil
}

func (a *dbAppender) AddFast(ref uint64, t int64, v float64) error {
	// Load the head last byte of the head sequence from the 3rd byte of the
	// reference number.
	gen := (ref << 16) >> 56

	h, err := a.appenderFor(t)
	if err != nil {
		return err
	}
	// If the last byte of the sequence does not add up, the reference is not valid.
	if uint64(h.meta.Sequence^0xff) != gen {
		return ErrNotFound
	}
	if err := h.app.AddFast(ref, t, v); err != nil {
		return err
	}

	a.samples++
	return nil
}

// appenderFor gets the appender for the head containing timestamp t.
// If the head block doesn't exist yet, it gets created.
func (a *dbAppender) appenderFor(t int64) (*metaAppender, error) {
	// If there's no fitting head block for t, ensure it gets created.
	if len(a.heads) == 0 || t >= a.heads[len(a.heads)-1].meta.MaxTime {
		a.db.headmtx.Lock()

		var newHeads []HeadBlock

		if err := a.db.ensureHead(t); err != nil {
			a.db.headmtx.Unlock()
			return nil, err
		}
		if len(a.heads) == 0 {
			newHeads = append(newHeads, a.db.appendable()...)
		} else {
			maxSeq := a.heads[len(a.heads)-1].meta.Sequence
			for _, b := range a.db.appendable() {
				if b.Meta().Sequence > maxSeq {
					newHeads = append(newHeads, b)
				}
			}
		}

		a.db.headmtx.Unlock()

		// XXX(fabxc): temporary workaround. See comment on instantiating DB.Appender.
		// for _, b := range newHeads {
		// 	// Only get appender for the block with the specific timestamp.
		// 	if t >= b.Meta().MaxTime {
		// 		continue
		// 	}
		// 	a.heads = append(a.heads, &metaAppender{
		// 		app:  b.Appender(),
		// 		meta: b.Meta(),
		// 	})
		// 	break
		// }

		// Instantiate appenders after returning headmtx to avoid questionable
		// locking order.
		for _, b := range newHeads {
			a.heads = append(a.heads, &metaAppender{
				app:  b.Appender(),
				meta: b.Meta(),
			})
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
		m := h.Meta()
		// If t doesn't exceed the range of heads blocks, there's nothing to do.
		if t < m.MaxTime {
			return nil
		}
		if _, err := db.cut(m.MaxTime); err != nil {
			return err
		}
	}
}

func (a *dbAppender) Commit() error {
	defer a.db.mtx.RUnlock()

	// Commits to partial appenders must be concurrent as concurrent appenders
	// may have conflicting locks on head appenders.
	// XXX(fabxc): is this a leaky abstraction? Should make an effort to catch a multi-error?
	var g errgroup.Group

	for _, h := range a.heads {
		g.Go(h.app.Commit)
	}

	if err := g.Wait(); err != nil {
		return err
	}
	// XXX(fabxc): Push the metric down into head block to account properly
	// for partial appends?
	a.db.metrics.samplesAppended.Add(float64(a.samples))

	return nil
}

func (a *dbAppender) Rollback() error {
	defer a.db.mtx.RUnlock()

	var g errgroup.Group

	for _, h := range a.heads {
		g.Go(h.app.Commit)
	}

	return g.Wait()
}

// appendable returns a copy of a slice of HeadBlocks that can still be appended to.
func (db *DB) appendable() []HeadBlock {
	var i int
	app := make([]HeadBlock, 0, db.opts.AppendableBlocks)

	if len(db.heads) > db.opts.AppendableBlocks {
		i = len(db.heads) - db.opts.AppendableBlocks
	}
	return append(app, db.heads[i:]...)
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

	for _, b := range db.blocks {
		m := b.Meta()
		if intervalOverlap(mint, maxt, m.MinTime, m.MaxTime) {
			bs = append(bs, b)
		}
	}

	return bs
}

// cut starts a new head block to append to. The completed head block
// will still be appendable for the configured grace period.
func (db *DB) cut(mint int64) (HeadBlock, error) {
	maxt := mint + int64(db.opts.MinBlockDuration)

	dir, seq, err := nextSequenceFile(db.dir, "b-")
	if err != nil {
		return nil, err
	}
	newHead, err := createHeadBlock(dir, seq, db.logger, mint, maxt)
	if err != nil {
		return nil, err
	}

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
