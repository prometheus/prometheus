// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// DefaultOptions used for the DB. They are sane for setups using
// millisecond precision timestamps.
var DefaultOptions = &Options{
	Retention:  15 * 24 * 3600 * 1000, // 15 days
	DisableWAL: false,
}

// Options of the DB storage.
type Options struct {
	Retention  int64
	DisableWAL bool
}

// DB is a time series storage.
type DB struct {
	logger log.Logger
	opts   *Options
	path   string

	partitions []*Partition
}

// TODO(fabxc): make configurable
const (
	partitionShift = 0
	numPartitions  = 1 << partitionShift
	maxChunkSize   = 1024
)

// Open or create a new DB.
func Open(path string, l log.Logger, opts *Options) (*DB, error) {
	if opts == nil {
		opts = DefaultOptions
	}
	if err := os.MkdirAll(path, 0777); err != nil {
		return nil, err
	}
	if l == nil {
		l = log.NewLogfmtLogger(os.Stdout)
		l = log.NewContext(l).With("ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)
	}

	c := &DB{
		logger: l,
		opts:   opts,
		path:   path,
	}

	// Initialize vertical partitions.
	// TODO(fabxc): validate partition number to be power of 2, which is required
	// for the bitshift-modulo when finding the right partition.
	for i := 0; i < numPartitions; i++ {
		l := log.NewContext(l).With("partition", i)
		d := partitionDir(path, i)

		s, err := OpenPartition(d, i, l)
		if err != nil {
			return nil, fmt.Errorf("initializing partition %q failed: %s", d, err)
		}

		c.partitions = append(c.partitions, s)
	}

	return c, nil
}

func partitionDir(base string, i int) string {
	return filepath.Join(base, strconv.Itoa(i))
}

// Close the database.
func (db *DB) Close() error {
	var g errgroup.Group

	for _, partition := range db.partitions {
		g.Go(partition.Close)
	}

	return g.Wait()
}

// Appender allows committing batches of samples to a database.
// The data held by the appender is reset after Commit returns.
type Appender interface {
	// AddSeries registers a new known series label set with the appender
	// and returns a reference number used to add samples to it over the
	// life time of the Appender.
	// AddSeries(Labels) uint64

	// Add adds a sample pair for the referenced series.
	Add(lset labels.Labels, t int64, v float64) error

	// Commit submits the collected samples and purges the batch.
	Commit() error
}

// Appender returns a new appender against the database.
func (db *DB) Appender() Appender {
	return &bucketAppender{
		db:      db,
		buckets: make([][]hashedSample, numPartitions),
	}
}

type bucketAppender struct {
	db      *DB
	buckets [][]hashedSample
}

func (ba *bucketAppender) Add(lset labels.Labels, t int64, v float64) error {
	h := lset.Hash()
	s := h >> (64 - partitionShift)

	ba.buckets[s] = append(ba.buckets[s], hashedSample{
		hash:   h,
		labels: lset,
		t:      t,
		v:      v,
	})

	return nil
}

func (ba *bucketAppender) reset() {
	for i := range ba.buckets {
		ba.buckets[i] = ba.buckets[i][:0]
	}
}

func (ba *bucketAppender) Commit() error {
	defer ba.reset()

	var merr MultiError

	// Spill buckets into partitions.
	for s, b := range ba.buckets {
		merr.Add(ba.db.partitions[s].appendBatch(b))
	}
	return merr.Err()
}

type hashedSample struct {
	hash   uint64
	labels labels.Labels
	ref    uint32

	t int64
	v float64
}

const sep = '\xff'

// Partition handles reads and writes of time series falling into
// a hashed partition of a series.
type Partition struct {
	path    string
	logger  log.Logger
	metrics *partitionMetrics

	mtx       sync.RWMutex
	persisted []*persistedBlock
	heads     []*HeadBlock
	compactor *compactor

	donec chan struct{}
	cutc  chan struct{}
}

type partitionMetrics struct {
	persistences        prometheus.Counter
	persistenceDuration prometheus.Histogram
	samplesAppended     prometheus.Counter
}

func newPartitionMetrics(r prometheus.Registerer, i int) *partitionMetrics {
	partitionLabel := prometheus.Labels{
		"partition": fmt.Sprintf("%d", i),
	}

	m := &partitionMetrics{}

	m.persistences = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "tsdb_partition_persistences_total",
		Help:        "Total number of head persistances that ran so far.",
		ConstLabels: partitionLabel,
	})
	m.persistenceDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "tsdb_partition_persistence_duration_seconds",
		Help:        "Duration of persistences in seconds.",
		ConstLabels: partitionLabel,
		Buckets:     prometheus.ExponentialBuckets(0.25, 2, 5),
	})
	m.samplesAppended = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "tsdb_partition_samples_appended_total",
		Help:        "Total number of appended samples for the partition.",
		ConstLabels: partitionLabel,
	})

	if r != nil {
		r.MustRegister(
			m.persistences,
			m.persistenceDuration,
			m.samplesAppended,
		)
	}
	return m
}

// OpenPartition returns a new Partition.
func OpenPartition(path string, i int, logger log.Logger) (*Partition, error) {
	// Create directory if partition is new.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0777); err != nil {
			return nil, err
		}
	}

	// Initialize previously persisted blocks.
	persisted, heads, err := findBlocks(path)
	if err != nil {
		return nil, err
	}

	// TODO(fabxc): get time from client-defined `now` function.
	baset := time.Unix(0, 0).UnixNano() / int64(time.Millisecond)
	if len(persisted) > 0 {
		baset = persisted[len(persisted)-1].bstats.MaxTime
	}
	if len(heads) == 0 {
		head, err := OpenHeadBlock(filepath.Join(path, fmt.Sprintf("%d", baset)), baset)
		if err != nil {
			return nil, err
		}
		heads = []*HeadBlock{head}
	}

	s := &Partition{
		path:      path,
		logger:    logger,
		metrics:   newPartitionMetrics(nil, i),
		heads:     heads,
		persisted: persisted,
		cutc:      make(chan struct{}, 1),
		donec:     make(chan struct{}),
	}
	if s.compactor, err = newCompactor(i, s, logger); err != nil {
		return nil, err
	}
	go s.run()

	return s, nil
}

func (s *Partition) run() {
	for range s.cutc {
		// if err := s.cut(); err != nil {
		// 	s.logger.Log("msg", "cut error", "err", err)
		// }
		// select {
		// case <-s.cutc:
		// default:
		// }
		// start := time.Now()

		// if err := s.persist(); err != nil {
		// 	s.logger.Log("msg", "persistence error", "err", err)
		// }

		// s.metrics.persistenceDuration.Observe(time.Since(start).Seconds())
		// s.metrics.persistences.Inc()
	}
	close(s.donec)
}

// Close the partition.
func (s *Partition) Close() error {
	close(s.cutc)
	<-s.donec

	var merr MultiError
	merr.Add(s.compactor.Close())

	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, pb := range s.persisted {
		merr.Add(pb.Close())
	}
	for _, hb := range s.heads {
		merr.Add(hb.Close())
	}

	return merr.Err()
}

func (s *Partition) appendBatch(samples []hashedSample) error {
	if len(samples) == 0 {
		return nil
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()

	head := s.heads[len(s.heads)-1]

	// TODO(fabxc): distinguish samples between concurrent heads for
	// different time blocks. Those may occurr during transition to still
	// allow late samples to arrive for a previous block.
	err := head.appendBatch(samples)
	if err == nil {
		s.metrics.samplesAppended.Add(float64(len(samples)))
	}

	// TODO(fabxc): randomize over time and use better scoring function.
	if head.bstats.SampleCount/(uint64(head.bstats.ChunkCount)+1) > 250 {
		if err := s.cut(); err != nil {
			s.logger.Log("msg", "cut failed", "err", err)
		}
	}

	return err
}

func (s *Partition) lock() sync.Locker {
	return &s.mtx
}

func (s *Partition) headForDir(dir string) (int, bool) {
	for i, b := range s.heads {
		if b.dir() == dir {
			return i, true
		}
	}
	return -1, false
}

func (s *Partition) persistedForDir(dir string) (int, bool) {
	for i, b := range s.persisted {
		if b.dir() == dir {
			return i, true
		}
	}
	return -1, false
}

func (s *Partition) reinit(dir string) error {
	if !fileutil.Exist(dir) {
		if i, ok := s.headForDir(dir); ok {
			if err := s.heads[i].Close(); err != nil {
				return err
			}
			s.heads = append(s.heads[:i], s.heads[i+1:]...)
		}
		if i, ok := s.persistedForDir(dir); ok {
			if err := s.persisted[i].Close(); err != nil {
				return err
			}
			s.persisted = append(s.persisted[:i], s.persisted[i+1:]...)
		}
		return nil
	}

	// Remove a previous head block.
	if i, ok := s.headForDir(dir); ok {
		if err := s.heads[i].Close(); err != nil {
			return err
		}
		s.heads = append(s.heads[:i], s.heads[i+1:]...)
	}
	// Close an old persisted block.
	i, ok := s.persistedForDir(dir)
	if ok {
		if err := s.persisted[i].Close(); err != nil {
			return err
		}
	}
	pb, err := newPersistedBlock(dir)
	if err != nil {
		return errors.Wrap(err, "open persisted block")
	}
	if i >= 0 {
		s.persisted[i] = pb
	} else {
		s.persisted = append(s.persisted, pb)
	}

	return nil
}

func (s *Partition) compactable() []block {
	var blocks []block
	for _, pb := range s.persisted {
		blocks = append([]block{pb}, blocks...)
	}

	// threshold := s.heads[len(s.heads)-1].bstats.MaxTime - headGracePeriod

	// for _, hb := range s.heads {
	// 	if hb.bstats.MaxTime < threshold {
	// 		blocks = append(blocks, hb)
	// 	}
	// }
	for _, hb := range s.heads[:len(s.heads)-1] {
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

// blocksForRange returns all blocks within the partition that may contain
// data for the given time range.
func (s *Partition) blocksForInterval(mint, maxt int64) []block {
	var bs []block

	for _, b := range s.persisted {
		bmin, bmax := b.interval()

		if intervalOverlap(mint, maxt, bmin, bmax) {
			bs = append(bs, b)
		}
	}
	for _, b := range s.heads {
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
func (s *Partition) cut() error {
	// Set new head block.
	head := s.heads[len(s.heads)-1]

	newHead, err := OpenHeadBlock(filepath.Join(s.path, fmt.Sprintf("%d", head.bstats.MaxTime)), head.bstats.MaxTime)
	if err != nil {
		return err
	}
	s.heads = append(s.heads, newHead)

	s.compactor.trigger()

	return nil
}

// func (s *Partition) persist() error {
// 	s.mtx.Lock()

// 	// Set new head block.
// 	head := s.head
// 	newHead, err := OpenHeadBlock(filepath.Join(s.path, fmt.Sprintf("%d", head.bstats.MaxTime)), head.bstats.MaxTime)
// 	if err != nil {
// 		s.mtx.Unlock()
// 		return err
// 	}
// 	s.head = newHead

// 	s.mtx.Unlock()

// 	// TODO(fabxc): add grace period where we can still append to old head partition
// 	// before actually persisting it.
// 	dir := filepath.Join(s.path, fmt.Sprintf("%d", head.stats.MinTime))

// 	if err := persist(dir, head.persist); err != nil {
// 		return err
// 	}
// 	s.logger.Log("samples", head.stats.SampleCount, "chunks", head.stats.ChunkCount, "msg", "persisted head")

// 	// Reopen block as persisted block for querying.
// 	pb, err := newPersistedBlock(dir)
// 	if err != nil {
// 		return err
// 	}

// 	s.mtx.Lock()
// 	s.persisted = append(s.persisted, pb)
// 	s.mtx.Unlock()

// 	s.compactor.trigger()

// 	return nil
// }

// chunkDesc wraps a plain data chunk and provides cached meta data about it.
type chunkDesc struct {
	ref   uint32
	lset  labels.Labels
	chunk chunks.Chunk

	// Caching fields.
	firstTimestamp int64
	lastTimestamp  int64
	lastValue      float64
	numSamples     int

	app chunks.Appender // Current appender for the chunks.
}

func (cd *chunkDesc) append(ts int64, v float64) {
	if cd.numSamples == 0 {
		cd.firstTimestamp = ts
	}
	cd.app.Append(ts, v)

	cd.lastTimestamp = ts
	cd.lastValue = v
	cd.numSamples++
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
