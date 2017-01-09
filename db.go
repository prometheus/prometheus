// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// DefaultOptions used for the DB. They are sane for setups using
// millisecond precision timestampdb.
var DefaultOptions = &Options{
	Retention:  15 * 24 * 3600 * 1000, // 15 days
	DisableWAL: false,
}

// Options of the DB storage.
type Options struct {
	Retention        int64
	DisableWAL       bool
	WALFlushInterval time.Duration
}

// Appender allows committing batches of samples to a database.
// The data held by the appender is reset after Commit returndb.
type Appender interface {
	// AddSeries registers a new known series label set with the appender
	// and returns a reference number used to add samples to it over the
	// life time of the Appender.
	// AddSeries(Labels) uint64

	// Add adds a sample pair for the referenced seriedb.
	Add(lset labels.Labels, t int64, v float64) error

	// Commit submits the collected samples and purges the batch.
	Commit() error
}

type hashedSample struct {
	hash   uint64
	labels labels.Labels
	ref    uint32

	t int64
	v float64
}

const sep = '\xff'

// DB handles reads and writes of time series falling into
// a hashed partition of a seriedb.
type DB struct {
	dir     string
	logger  log.Logger
	metrics *dbMetrics

	mtx       sync.RWMutex
	persisted []*persistedBlock
	heads     []*HeadBlock
	compactor *compactor

	compactc chan struct{}
	cutc     chan struct{}
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
func Open(dir string, logger log.Logger) (db *DB, err error) {
	// Create directory if partition is new.
	if !fileutil.Exist(dir) {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return nil, err
		}
	}
	r := prometheus.DefaultRegisterer

	db = &DB{
		dir:      dir,
		logger:   logger,
		metrics:  newDBMetrics(r),
		compactc: make(chan struct{}, 1),
		cutc:     make(chan struct{}, 1),
		donec:    make(chan struct{}),
		stopc:    make(chan struct{}),
	}

	if err := db.initBlocks(); err != nil {
		return nil, err
	}
	if db.compactor, err = newCompactor(db, r); err != nil {
		return nil, err
	}

	go db.run()

	return db, nil
}

func (db *DB) run() {
	defer close(db.donec)

	for {
		select {
		case <-db.cutc:
			db.mtx.Lock()
			err := db.cut()
			db.mtx.Unlock()

			if err != nil {
				db.logger.Log("msg", "cut failed", "err", err)
			} else {
				select {
				case db.compactc <- struct{}{}:
				default:
				}
			}
			// Drain cut channel so we don't trigger immediately again.
			select {
			case <-db.cutc:
			default:
			}

		case <-db.compactc:
			db.metrics.compactionsTriggered.Inc()

			for {
				blocks := db.compactor.pick()
				if len(blocks) == 0 {
					break
				}
				// TODO(fabxc): pick emits blocks in order. compact acts on
				// inverted order. Put inversion into compactor?
				var bs []block
				for _, b := range blocks {
					bs = append([]block{b}, bs...)
				}

				select {
				case <-db.stopc:
					return
				default:
				}
				if err := db.compact(bs); err != nil {
					db.logger.Log("msg", "compaction failed", "err", err)
				}
			}

		case <-db.stopc:
			return
		}
	}
}

func (db *DB) compact(blocks []block) error {
	if len(blocks) == 0 {
		return nil
	}
	tmpdir := blocks[0].dir() + ".tmp"

	// TODO(fabxc): find a better place to do this transparently.
	for _, b := range blocks {
		if h, ok := b.(*HeadBlock); ok {
			h.updateMapping()
		}
	}

	if err := db.compactor.compact(tmpdir, blocks...); err != nil {
		return err
	}

	db.mtx.Lock()
	defer db.mtx.Unlock()

	if err := renameDir(tmpdir, blocks[0].dir()); err != nil {
		return errors.Wrap(err, "rename dir")
	}
	for _, b := range blocks[1:] {
		if err := os.RemoveAll(b.dir()); err != nil {
			return errors.Wrap(err, "delete dir")
		}
	}

	var merr MultiError

	for _, b := range blocks {
		merr.Add(errors.Wrapf(db.reinit(b.dir()), "reinit block at %q", b.dir()))
	}
	return merr.Err()
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

func (db *DB) initBlocks() error {
	var (
		pbs   []*persistedBlock
		heads []*HeadBlock
	)

	files, err := ioutil.ReadDir(db.dir)
	if err != nil {
		return err
	}

	for _, fi := range files {
		if !isBlockDir(fi) {
			continue
		}
		dir := filepath.Join(db.dir, fi.Name())

		if fileutil.Exist(filepath.Join(dir, walFileName)) {
			h, err := OpenHeadBlock(dir, db.logger)
			if err != nil {
				return err
			}
			heads = append(heads, h)
			continue
		}

		b, err := newPersistedBlock(dir)
		if err != nil {
			return err
		}
		pbs = append(pbs, b)
	}

	// Validate that blocks are sequential in time.
	lastTime := int64(math.MinInt64)

	for _, b := range pbs {
		if b.stats().MinTime < lastTime {
			return errors.Errorf("illegal order for block at %q", b.dir())
		}
		lastTime = b.stats().MaxTime
	}
	for _, b := range heads {
		if b.stats().MinTime < lastTime {
			return errors.Errorf("illegal order for block at %q", b.dir())
		}
		lastTime = b.stats().MaxTime
	}

	db.persisted = pbs
	db.heads = heads

	if len(heads) == 0 {
		return db.cut()
	}
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

	return merr.Err()
}

func (db *DB) Appender() Appender {
	return &dbAppender{db: db}
}

type dbAppender struct {
	db  *DB
	buf []hashedSample
}

func (a *dbAppender) Add(lset labels.Labels, t int64, v float64) error {
	return a.add(hashedSample{
		hash:   lset.Hash(),
		labels: lset,
		t:      t,
		v:      v,
	})
}

func (a *dbAppender) add(s hashedSample) error {
	a.buf = append(a.buf, s)
	return nil
}

func (a *dbAppender) Commit() error {
	err := a.db.appendBatch(a.buf)
	a.buf = a.buf[:0]
	return err
}

func (db *DB) appendBatch(samples []hashedSample) error {
	if len(samples) == 0 {
		return nil
	}
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	head := db.heads[len(db.heads)-1]

	// TODO(fabxc): distinguish samples between concurrent heads for
	// different time blocks. Those may occurr during transition to still
	// allow late samples to arrive for a previous block.
	n, err := head.appendBatch(samples)
	if err == nil {
		db.metrics.samplesAppended.Add(float64(n))
	}

	if head.fullness() > 1.0 {
		select {
		case db.cutc <- struct{}{}:
		default:
		}
	}

	return err
}

func (db *DB) headForDir(dir string) (int, bool) {
	for i, b := range db.heads {
		if b.dir() == dir {
			return i, true
		}
	}
	return -1, false
}

func (db *DB) persistedForDir(dir string) (int, bool) {
	for i, b := range db.persisted {
		if b.dir() == dir {
			return i, true
		}
	}
	return -1, false
}

func (db *DB) reinit(dir string) error {
	if !fileutil.Exist(dir) {
		if i, ok := db.headForDir(dir); ok {
			if err := db.heads[i].Close(); err != nil {
				return err
			}
			db.heads = append(db.heads[:i], db.heads[i+1:]...)
		}
		if i, ok := db.persistedForDir(dir); ok {
			if err := db.persisted[i].Close(); err != nil {
				return err
			}
			db.persisted = append(db.persisted[:i], db.persisted[i+1:]...)
		}
		return nil
	}

	// Remove a previous head block.
	if i, ok := db.headForDir(dir); ok {
		if err := db.heads[i].Close(); err != nil {
			return err
		}
		db.heads = append(db.heads[:i], db.heads[i+1:]...)
	}
	// Close an old persisted block.
	i, ok := db.persistedForDir(dir)
	if ok {
		if err := db.persisted[i].Close(); err != nil {
			return err
		}
	}
	pb, err := newPersistedBlock(dir)
	if err != nil {
		return errors.Wrap(err, "open persisted block")
	}
	if i >= 0 {
		db.persisted[i] = pb
	} else {
		db.persisted = append(db.persisted, pb)
	}

	return nil
}

func (db *DB) compactable() []block {
	db.mtx.RLock()
	defer db.mtx.RUnlock()

	var blocks []block
	for _, pb := range db.persisted {
		blocks = append([]block{pb}, blocks...)
	}

	// threshold := db.heads[len(db.heads)-1].bstatdb.MaxTime - headGracePeriod

	// for _, hb := range db.heads {
	// 	if hb.bstatdb.MaxTime < threshold {
	// 		blocks = append(blocks, hb)
	// 	}
	// }
	for _, hb := range db.heads[:len(db.heads)-1] {
		blocks = append([]block{hb}, blocks...)
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
func (db *DB) blocksForInterval(mint, maxt int64) []block {
	var bs []block

	for _, b := range db.persisted {
		bmin, bmax := b.interval()

		if intervalOverlap(mint, maxt, bmin, bmax) {
			bs = append(bs, b)
		}
	}
	for _, b := range db.heads {
		bmin, bmax := b.interval()

		if intervalOverlap(mint, maxt, bmin, bmax) {
			bs = append(bs, b)
		}
	}

	return bs
}

// TODO(fabxc): make configurable.
const headGracePeriod = 60 * 1000 // 60 seconds for millisecond scale

// cut starts a new head block to append to. The completed head block
// will still be appendable for the configured grace period.
func (db *DB) cut() error {
	dir, err := db.nextBlockDir()
	if err != nil {
		return err
	}
	newHead, err := OpenHeadBlock(dir, db.logger)
	if err != nil {
		return err
	}
	db.heads = append(db.heads, newHead)

	return nil
}

func (db *DB) nextBlockDir() (string, error) {
	names, err := fileutil.ReadDir(db.dir)
	if err != nil {
		return "", err
	}

	i := uint64(0)
	for _, n := range names {
		if !strings.HasPrefix(n, "b-") {
			continue
		}
		j, err := strconv.ParseUint(n[2:], 10, 32)
		if err != nil {
			continue
		}
		i = j
	}
	return filepath.Join(db.dir, fmt.Sprintf("b-%0.6d", i+1)), nil
}

// PartitionedDB is a time series storage.
type PartitionedDB struct {
	logger log.Logger
	opts   *Options
	dir    string

	partitionPow uint
	Partitions   []*DB
}

func isPowTwo(x int) bool {
	return x > 0 && (x&(x-1)) == 0
}

// OpenPartitioned or create a new DB.
func OpenPartitioned(dir string, n int, l log.Logger, opts *Options) (*PartitionedDB, error) {
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
		opts:         opts,
		dir:          dir,
		partitionPow: uint(math.Log2(float64(n))),
	}

	// Initialize vertical partitiondb.
	// TODO(fabxc): validate partition number to be power of 2, which is required
	// for the bitshift-modulo when finding the right partition.
	for i := 0; i < n; i++ {
		l := log.NewContext(l).With("partition", i)
		d := partitionDir(dir, i)

		s, err := Open(d, l)
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
		app.buckets = append(app.buckets, p.Appender().(*dbAppender))
	}
	return app
}

type partitionedAppender struct {
	db      *PartitionedDB
	buckets []*dbAppender
}

func (ba *partitionedAppender) SetSeries(lset labels.Labels) (uint32, error) {

	return 0, nil
}

func (a *partitionedAppender) Add(lset labels.Labels, t int64, v float64) error {
	h := lset.Hash()
	s := h >> (64 - a.db.partitionPow)

	return a.buckets[s].add(hashedSample{
		hash:   h,
		labels: lset,
		t:      t,
		v:      v,
	})
}

func (ba *partitionedAppender) Commit() error {
	var merr MultiError

	// Spill buckets into partitiondb.
	for _, b := range ba.buckets {
		merr.Add(b.Commit())
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

func yoloBytes(s string) []byte {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))

	h := reflect.SliceHeader{
		Cap:  sh.Len,
		Len:  sh.Len,
		Data: sh.Data,
	}
	return *((*[]byte)(unsafe.Pointer(&h)))
}
