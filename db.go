// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
	dir     string
	lockf   lockfile.Lockfile
	logger  log.Logger
	metrics *dbMetrics
	opts    *Options

	mtx       sync.RWMutex
	persisted []*persistedBlock
	heads     []*headBlock
	headGen   uint8

	compactor *compactor

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
		l = log.NewContext(l).With("ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
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

	if err := db.initBlocks(); err != nil {
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

			var seqs []int
			var infos []compactionInfo
			for _, b := range db.compactable() {
				m := b.Meta()

				infos = append(infos, compactionInfo{
					generation: m.Compaction.Generation,
					mint:       m.MinTime,
					maxt:       m.MaxTime,
					seq:        m.Sequence,
				})
				seqs = append(seqs, m.Sequence)
			}

			i, j, ok := db.compactor.pick(infos)
			if !ok {
				continue
			}
			db.logger.Log("msg", "compact", "seqs", fmt.Sprintf("%v", seqs[i:j]))

			if err := db.compact(i, j); err != nil {
				db.logger.Log("msg", "compaction failed", "err", err)
				continue
			}
			db.logger.Log("msg", "compaction completed")
			// Trigger another compaction in case there's more work to do.
			select {
			case db.compactc <- struct{}{}:
			default:
			}

		case <-db.stopc:
			return
		}
	}
}

func (db *DB) getBlock(i int) Block {
	if i < len(db.persisted) {
		return db.persisted[i]
	}
	return db.heads[i-len(db.persisted)]
}

// removeBlocks removes the blocks in range [i, j) from the list of persisted
// and head blocks. The blocks are not closed and their files not deleted.
func (db *DB) removeBlocks(i, j int) {
	for k := i; k < j; k++ {
		if i < len(db.persisted) {
			db.persisted = append(db.persisted[:i], db.persisted[i+1:]...)
		} else {
			l := i - len(db.persisted)
			db.heads = append(db.heads[:l], db.heads[l+1:]...)
		}
	}
}

func (db *DB) blocks() (bs []Block) {
	for _, b := range db.persisted {
		bs = append(bs, b)
	}
	for _, b := range db.heads {
		bs = append(bs, b)
	}
	return bs
}

// compact block in range [i, j) into a temporary directory and atomically
// swap the blocks out on successful completion.
func (db *DB) compact(i, j int) error {
	if j <= i {
		return errors.New("invalid compaction block range")
	}
	var blocks []Block
	for k := i; k < j; k++ {
		blocks = append(blocks, db.getBlock(k))
	}
	var (
		dir    = blocks[0].Dir()
		tmpdir = dir + ".tmp"
	)

	if err := db.compactor.compact(tmpdir, blocks...); err != nil {
		return err
	}

	pb, err := newPersistedBlock(tmpdir)
	if err != nil {
		return err
	}

	db.mtx.Lock()
	defer db.mtx.Unlock()

	for _, b := range blocks {
		if err := b.Close(); err != nil {
			return errors.Wrapf(err, "close old block %s", b.Dir())
		}
	}

	if err := renameDir(tmpdir, dir); err != nil {
		return errors.Wrap(err, "rename dir")
	}
	pb.dir = dir

	db.removeBlocks(i, j)
	db.persisted = append(db.persisted, pb)

	for _, b := range blocks[1:] {
		db.logger.Log("msg", "remove old dir", "dir", b.Dir())
		if err := os.RemoveAll(b.Dir()); err != nil {
			return errors.Wrap(err, "removing old block")
		}
	}
	if err := db.retentionCutoff(); err != nil {
		return err
	}

	return nil
}

func (db *DB) retentionCutoff() error {
	if db.opts.RetentionDuration == 0 {
		return nil
	}
	h := db.heads[len(db.heads)-1]
	t := h.meta.MinTime - int64(db.opts.RetentionDuration)

	var (
		blocks = db.blocks()
		i      int
		b      Block
	)
	for i, b = range blocks {
		if b.Meta().MinTime >= t {
			break
		}
	}
	if i <= 1 {
		return nil
	}
	db.logger.Log("msg", "retention cutoff", "idx", i-1)
	db.removeBlocks(0, i)

	for _, b := range blocks[:i] {
		if err := os.RemoveAll(b.Dir()); err != nil {
			return errors.Wrap(err, "removing old block")
		}
	}
	return nil
}

func (db *DB) initBlocks() error {
	var (
		persisted []*persistedBlock
		heads     []*headBlock
	)

	dirs, err := blockDirs(db.dir)
	if err != nil {
		return err
	}

	for _, dir := range dirs {
		if fileutil.Exist(filepath.Join(dir, walDirName)) {
			h, err := openHeadBlock(dir, db.logger)
			if err != nil {
				return err
			}
			h.generation = db.headGen
			db.headGen++
			heads = append(heads, h)
			continue
		}
		b, err := newPersistedBlock(dir)
		if err != nil {
			return err
		}
		persisted = append(persisted, b)
	}

	db.persisted = persisted
	db.heads = heads

	return nil
}

// Close the partition.
func (db *DB) Close() error {
	close(db.stopc)
	<-db.donec

	var merr MultiError

	db.mtx.Lock()
	defer db.mtx.Unlock()

	for _, pb := range db.persisted {
		merr.Add(pb.Close())
	}
	for _, hb := range db.heads {
		merr.Add(hb.Close())
	}

	merr.Add(db.lockf.Unlock())

	return merr.Err()
}

// Appender returns a new Appender on the database.
func (db *DB) Appender() Appender {
	db.mtx.RLock()
	a := &dbAppender{db: db}

	for _, b := range db.appendable() {
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

func (a *dbAppender) hashedAdd(hash uint64, lset labels.Labels, t int64, v float64) (uint64, error) {
	h, err := a.appenderFor(t)
	if err != nil {
		return 0, err
	}
	ref, err := h.hashedAdd(hash, lset, t, v)
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
		a.db.mtx.RUnlock()

		if err := a.db.ensureHead(t); err != nil {
			a.db.mtx.RLock()
			return nil, err
		}

		a.db.mtx.RLock()

		if len(a.heads) == 0 {
			for _, b := range a.db.appendable() {
				a.heads = append(a.heads, b.Appender().(*headAppender))
			}
		} else {
			maxSeq := a.heads[len(a.heads)-1].meta.Sequence
			for _, b := range a.db.appendable() {
				if b.meta.Sequence > maxSeq {
					a.heads = append(a.heads, b.Appender().(*headAppender))
				}
			}
		}
	}
	for i := len(a.heads) - 1; i >= 0; i-- {
		if h := a.heads[i]; t >= h.meta.MinTime {
			return h, nil
		}
	}

	return nil, ErrNotFound
}

func (db *DB) ensureHead(t int64) error {
	db.mtx.Lock()
	defer db.mtx.Unlock()

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

func (db *DB) compactable() []Block {
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	var blocks []Block
	for _, pb := range db.persisted {
		blocks = append(blocks, pb)
	}

	if len(db.heads) <= db.opts.AppendableBlocks {
		return blocks
	}

	for _, h := range db.heads[:len(db.heads)-db.opts.AppendableBlocks] {
		// Blocks that won't be appendable when instantiating a new appender
		// might still have active appenders on them.
		// Abort at the first one we encounter.
		if atomic.LoadUint64(&h.activeWriters) > 0 {
			break
		}
		blocks = append(blocks, h)
	}
	return blocks
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

// PartitionedDB is a time series storage.
type PartitionedDB struct {
	logger log.Logger
	dir    string

	partitionPow uint
	Partitions   []*DB
}

func isPowTwo(x int) bool {
	return x > 0 && (x&(x-1)) == 0
}

// OpenPartitioned or create a new DB.
func OpenPartitioned(dir string, n int, l log.Logger, r prometheus.Registerer, opts *Options) (*PartitionedDB, error) {
	if !isPowTwo(n) {
		return nil, errors.Errorf("%d is not a power of two", n)
	}
	if opts == nil {
		opts = DefaultOptions
	}
	if l == nil {
		l = log.NewLogfmtLogger(os.Stdout)
		l = log.NewContext(l).With("ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
	}

	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	c := &PartitionedDB{
		logger:       l,
		dir:          dir,
		partitionPow: uint(math.Log2(float64(n))),
	}

	// Initialize vertical partitiondb.
	// TODO(fabxc): validate partition number to be power of 2, which is required
	// for the bitshift-modulo when finding the right partition.
	for i := 0; i < n; i++ {
		l := log.NewContext(l).With("partition", i)
		d := partitionDir(dir, i)

		s, err := Open(d, l, r, opts)
		if err != nil {
			return nil, fmt.Errorf("initializing partition %q failed: %s", d, err)
		}

		c.Partitions = append(c.Partitions, s)
	}

	return c, nil
}

func partitionDir(base string, i int) string {
	return filepath.Join(base, fmt.Sprintf("p-%0.4d", i))
}

// Close the database.
func (db *PartitionedDB) Close() error {
	var g errgroup.Group

	for _, partition := range db.Partitions {
		g.Go(partition.Close)
	}

	return g.Wait()
}

// Appender returns a new appender against the database.
func (db *PartitionedDB) Appender() Appender {
	app := &partitionedAppender{db: db}

	for _, p := range db.Partitions {
		app.partitions = append(app.partitions, p.Appender().(*dbAppender))
	}
	return app
}

type partitionedAppender struct {
	db         *PartitionedDB
	partitions []*dbAppender
}

func (a *partitionedAppender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	h := lset.Hash()
	p := h >> (64 - a.db.partitionPow)

	ref, err := a.partitions[p].hashedAdd(h, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref | (p << 48), nil
}

func (a *partitionedAppender) AddFast(ref uint64, t int64, v float64) error {
	p := uint8((ref << 8) >> 56)
	return a.partitions[p].AddFast(ref, t, v)
}

func (a *partitionedAppender) Commit() error {
	var merr MultiError

	for _, p := range a.partitions {
		merr.Add(p.Commit())
	}
	return merr.Err()
}

func (a *partitionedAppender) Rollback() error {
	var merr MultiError

	for _, p := range a.partitions {
		merr.Add(p.Rollback())
	}
	return merr.Err()
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
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&b))

	h := reflect.StringHeader{
		Data: sh.Data,
		Len:  sh.Len,
	}
	return *((*string)(unsafe.Pointer(&h)))
}

func closeAll(cs ...io.Closer) error {
	var merr MultiError

	for _, c := range cs {
		merr.Add(c.Close())
	}
	return merr.Err()
}
