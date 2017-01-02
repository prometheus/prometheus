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

	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
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

	shards []*Shard
}

// TODO(fabxc): make configurable
const (
	shardShift   = 0
	numShards    = 1 << shardShift
	maxChunkSize = 1024
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

	// Initialize vertical shards.
	// TODO(fabxc): validate shard number to be power of 2, which is required
	// for the bitshift-modulo when finding the right shard.
	for i := 0; i < numShards; i++ {
		l := log.NewContext(l).With("shard", i)
		d := shardDir(path, i)

		s, err := OpenShard(d, i, l)
		if err != nil {
			return nil, fmt.Errorf("initializing shard %q failed: %s", d, err)
		}

		c.shards = append(c.shards, s)
	}

	return c, nil
}

func shardDir(base string, i int) string {
	return filepath.Join(base, strconv.Itoa(i))
}

// Close the database.
func (db *DB) Close() error {
	var g errgroup.Group

	for _, shard := range db.shards {
		// Fix closure argument to goroutine.
		shard := shard
		g.Go(shard.Close)
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
		buckets: make([][]hashedSample, numShards),
	}
}

type bucketAppender struct {
	db      *DB
	buckets [][]hashedSample
}

func (ba *bucketAppender) Add(lset labels.Labels, t int64, v float64) error {
	h := lset.Hash()
	s := h >> (64 - shardShift)

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

	// Spill buckets into shards.
	for s, b := range ba.buckets {
		merr.Add(ba.db.shards[s].appendBatch(b))
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

// Shard handles reads and writes of time series falling into
// a hashed shard of a series.
type Shard struct {
	path      string
	persistCh chan struct{}
	logger    log.Logger
	metrics   *shardMetrics

	mtx       sync.RWMutex
	persisted persistedBlocks
	head      *HeadBlock
	compactor *compactor
}

type shardMetrics struct {
	persistences        prometheus.Counter
	persistenceDuration prometheus.Histogram
	samplesAppended     prometheus.Counter
}

func newShardMetrics(r prometheus.Registerer, i int) *shardMetrics {
	shardLabel := prometheus.Labels{
		"shard": fmt.Sprintf("%d", i),
	}

	m := &shardMetrics{
		persistences: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "tsdb_shard_persistences_total",
			Help:        "Total number of head persistances that ran so far.",
			ConstLabels: shardLabel,
		}),
		persistenceDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        "tsdb_shard_persistence_duration_seconds",
			Help:        "Duration of persistences in seconds.",
			ConstLabels: shardLabel,
			Buckets:     prometheus.ExponentialBuckets(0.25, 2, 5),
		}),
		samplesAppended: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "tsdb_shard_samples_appended_total",
			Help:        "Total number of appended samples for the shard.",
			ConstLabels: shardLabel,
		}),
	}

	if r != nil {
		r.MustRegister(
			m.persistences,
			m.persistenceDuration,
			m.samplesAppended,
		)
	}
	return m
}

// OpenShard returns a new Shard.
func OpenShard(path string, i int, logger log.Logger) (*Shard, error) {
	// Create directory if shard is new.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0777); err != nil {
			return nil, err
		}
	}

	// Initialize previously persisted blocks.
	pbs, head, err := findBlocks(path)
	if err != nil {
		return nil, err
	}

	// TODO(fabxc): get time from client-defined `now` function.
	// baset := time.Now().UnixNano() / int64(time.Millisecond)
	baset := time.Unix(0, 0).UnixNano() / int64(time.Millisecond)
	if len(pbs) > 0 {
		baset = pbs[len(pbs)-1].stats.MaxTime
	}
	if head == nil {
		head, err = OpenHeadBlock(filepath.Join(path, fmt.Sprintf("%d", baset)), baset)
		if err != nil {
			return nil, err
		}
	}

	s := &Shard{
		path:      path,
		persistCh: make(chan struct{}, 1),
		logger:    logger,
		metrics:   newShardMetrics(prometheus.DefaultRegisterer, i),
		head:      head,
		persisted: pbs,
	}
	s.compactor, err = newCompactor(s, logger)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// Close the shard.
func (s *Shard) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var e MultiError

	e.Add(s.compactor.close())

	for _, pb := range s.persisted {
		e.Add(pb.Close())
	}
	e.Add(s.head.Close())

	return e.Err()
}

func (s *Shard) appendBatch(samples []hashedSample) error {
	if len(samples) == 0 {
		return nil
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// TODO(fabxc): distinguish samples between concurrent heads for
	// different time blocks. Those may occurr during transition to still
	// allow late samples to arrive for a previous block.
	err := s.head.appendBatch(samples)
	if err == nil {
		s.metrics.samplesAppended.Add(float64(len(samples)))
	}

	// TODO(fabxc): randomize over time and use better scoring function.
	if s.head.stats.SampleCount/(uint64(s.head.stats.ChunkCount)+1) > 400 {
		select {
		case s.persistCh <- struct{}{}:
			go func() {
				start := time.Now()
				defer func() { s.metrics.persistenceDuration.Observe(time.Since(start).Seconds()) }()

				if err := s.persist(); err != nil {
					s.logger.Log("msg", "persistance error", "err", err)
				}
				s.metrics.persistences.Inc()
			}()
		default:
		}
	}

	return err
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

// blocksForRange returns all blocks within the shard that may contain
// data for the given time range.
func (s *Shard) blocksForInterval(mint, maxt int64) []block {
	var bs []block

	for _, b := range s.persisted {
		bmin, bmax := b.interval()

		if intervalOverlap(mint, maxt, bmin, bmax) {
			bs = append(bs, b)
		}
	}

	hmin, hmax := s.head.interval()

	if intervalOverlap(mint, maxt, hmin, hmax) {
		bs = append(bs, s.head)
	}

	return bs
}

// TODO(fabxc): make configurable.
const shardGracePeriod = 60 * 1000 // 60 seconds for millisecond scale

func (s *Shard) persist() error {
	s.mtx.Lock()

	// Set new head block.
	head := s.head
	newHead, err := OpenHeadBlock(filepath.Join(s.path, fmt.Sprintf("%d", head.stats.MaxTime)), head.stats.MaxTime)
	if err != nil {
		s.mtx.Unlock()
		return err
	}
	s.head = newHead

	s.mtx.Unlock()

	// Only allow another persistence to be triggered after the current one
	// has completed (successful or not.)
	defer func() {
		<-s.persistCh
	}()

	// TODO(fabxc): add grace period where we can still append to old head shard
	// before actually persisting it.
	p := filepath.Join(s.path, fmt.Sprintf("%d", head.stats.MinTime))

	if err := os.MkdirAll(p, 0777); err != nil {
		return err
	}

	n, err := head.persist(p)
	if err != nil {
		return err
	}
	sz := fmt.Sprintf("%.2fMiB", float64(n)/1024/1024)
	s.logger.Log("size", sz, "samples", head.stats.SampleCount, "chunks", head.stats.ChunkCount, "msg", "persisted head")

	// Reopen block as persisted block for querying.
	pb, err := newPersistedBlock(p)
	if err != nil {
		return err
	}

	s.mtx.Lock()
	s.persisted = append(s.persisted, pb)
	s.mtx.Unlock()

	s.compactor.trigger()

	return nil
}

// chunkDesc wraps a plain data chunk and provides cached meta data about it.
type chunkDesc struct {
	ref   uint32
	lset  labels.Labels
	chunk chunks.Chunk

	// Caching fields.
	firsTimestamp int64
	lastTimestamp int64
	lastValue     float64
	numSamples    int

	app chunks.Appender // Current appender for the chunks.
}

func (cd *chunkDesc) append(ts int64, v float64) {
	if cd.numSamples == 0 {
		cd.firsTimestamp = ts
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

	if len(es) > 0 {
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
