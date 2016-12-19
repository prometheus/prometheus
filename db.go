// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sync/errgroup"

	"github.com/cespare/xxhash"
	"github.com/fabxc/tsdb/chunks"
	"github.com/go-kit/kit/log"
)

// DefaultOptions used for the DB. They are sane for setups using
// millisecond precision timestamps.
var DefaultOptions = &Options{
	Retention: 15 * 24 * 3600 * 1000, // 15 days
}

// Options of the DB storage.
type Options struct {
	Retention int64
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
	shardShift   = 2
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

		s, err := OpenShard(d, l)
		if err != nil {
			return nil, fmt.Errorf("initializing shard %q failed: %s", d, err)
		}

		c.shards = append(c.shards, s)
	}

	// TODO(fabxc): run background compaction + GC.

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

// Appender adds a batch of samples.
type Appender interface {
	// Add adds a sample pair to the appended batch.
	Add(l Labels, t int64, v float64)

	// Commit submits the collected samples.
	Commit() error
}

// Vector is a set of LabelSet associated with one value each.
// Label sets and values must have equal length.
type Vector struct {
	Buckets map[uint16][]Sample
	reused  int
}

type Sample struct {
	Hash   uint64
	Labels Labels
	Value  float64
}

// Reset the vector but keep resources allocated.
func (v *Vector) Reset() {
	// Do a full reset every n-th reusage to avoid memory leaks.
	if v.Buckets == nil || v.reused > 100 {
		v.Buckets = make(map[uint16][]Sample, 0)
		return
	}
	for x, bkt := range v.Buckets {
		v.Buckets[x] = bkt[:0]
	}
	v.reused++
}

// Add a sample to the vector.
func (v *Vector) Add(lset Labels, val float64) {
	h := lset.Hash()
	s := uint16(h >> (64 - shardShift))

	v.Buckets[s] = append(v.Buckets[s], Sample{
		Hash:   h,
		Labels: lset,
		Value:  val,
	})
}

// func (db *DB) Appender() Appender {
// 	return &bucketAppender{
// 		samples: make([]Sample, 1024),
// 	}
// }

// type bucketAppender struct {
// 	db *DB
// 	// buckets []Sam
// }

// func (a *bucketAppender) Add(l Labels, t int64, v float64) {

// }

// func (a *bucketAppender) Commit() error {
// 	// f
// }

// AppendVector adds values for a list of label sets for the given timestamp
// in milliseconds.
func (db *DB) AppendVector(ts int64, v *Vector) error {
	// Sequentially add samples to shards.
	for s, bkt := range v.Buckets {
		shard := db.shards[s]
		if err := shard.appendBatch(ts, bkt); err != nil {
			// TODO(fabxc): handle gracefully and collect multi-error.
			return err
		}
	}

	return nil
}

func (db *DB) AppendSingle(lset Labels, ts int64, v float64) error {
	sort.Sort(lset)
	h := lset.Hash()
	s := uint16(h >> (64 - shardShift))

	return db.shards[s].appendBatch(ts, []Sample{
		{
			Hash:   h,
			Labels: lset,
			Value:  v,
		},
	})
}

const sep = '\xff'

// Shard handles reads and writes of time series falling into
// a hashed shard of a series.
type Shard struct {
	path      string
	persistCh chan struct{}
	logger    log.Logger

	mtx       sync.RWMutex
	persisted persistedBlocks
	head      *HeadBlock
}

// OpenShard returns a new Shard.
func OpenShard(path string, logger log.Logger) (*Shard, error) {
	// Create directory if shard is new.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0777); err != nil {
			return nil, err
		}
	}

	// Initialize previously persisted blocks.
	pbs, err := findPersistedBlocks(path)
	if err != nil {
		return nil, err
	}

	s := &Shard{
		path:      path,
		persistCh: make(chan struct{}, 1),
		logger:    logger,
		persisted: pbs,
		// TODO(fabxc): restore from checkpoint.
	}
	// TODO(fabxc): get base time from pre-existing blocks. Otherwise
	// it should come from a user defined start timestamp.
	// Use actual time for now.
	s.head = NewHeadBlock(time.Now().UnixNano() / int64(time.Millisecond))

	return s, nil
}

// Close the shard.
func (s *Shard) Close() error {
	var e MultiError

	for _, pb := range s.persisted {
		e.Add(pb.Close())
	}

	return e.Err()
}

func (s *Shard) appendBatch(ts int64, samples []Sample) error {
	// TODO(fabxc): make configurable.
	const persistenceTimeThreshold = 1000 * 60 * 60 // 1 hour if timestamp in ms

	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, smpl := range samples {
		if err := s.head.append(smpl.Hash, smpl.Labels, ts, smpl.Value); err != nil {
			// TODO(fabxc): handle gracefully and collect multi-error.
			return err
		}
	}

	if ts > s.head.stats.MaxTime {
		s.head.stats.MaxTime = ts
	}

	// TODO(fabxc): randomize over time
	if s.head.stats.SampleCount/uint64(s.head.stats.ChunkCount) > 400 {
		select {
		case s.persistCh <- struct{}{}:
			go func() {
				if err := s.persist(); err != nil {
					s.logger.Log("msg", "persistance error", "err", err)
				}
			}()
		default:
		}
	}

	return nil
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

	fmt.Println("blocks for interval", bs)

	return bs
}

// TODO(fabxc): make configurable.
const shardGracePeriod = 60 * 1000 // 60 seconds for millisecond scale

func (s *Shard) persist() error {
	s.mtx.Lock()

	// Set new head block.
	head := s.head
	s.head = NewHeadBlock(head.stats.MaxTime)

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

	return nil
}

// chunkDesc wraps a plain data chunk and provides cached meta data about it.
type chunkDesc struct {
	lset  Labels
	chunk chunks.Chunk

	// Caching fields.
	firsTimestamp int64
	lastTimestamp int64
	lastValue     float64

	app chunks.Appender // Current appender for the chunks.
}

func (cd *chunkDesc) append(ts int64, v float64) (err error) {
	if cd.app == nil {
		cd.app, err = cd.chunk.Appender()
		if err != nil {
			return err
		}
		cd.firsTimestamp = ts
	}
	if err := cd.app.Append(ts, v); err != nil {
		return err
	}

	cd.lastTimestamp = ts
	cd.lastValue = v

	return nil
}

// Label is a key/value pair of strings.
type Label struct {
	Name, Value string
}

// Labels is a sorted set of labels. Order has to be guaranteed upon
// instantiation.
type Labels []Label

func (ls Labels) Len() int           { return len(ls) }
func (ls Labels) Swap(i, j int)      { ls[i], ls[j] = ls[j], ls[i] }
func (ls Labels) Less(i, j int) bool { return ls[i].Name < ls[j].Name }

// Hash returns a hash value for the label set.
func (ls Labels) Hash() uint64 {
	b := make([]byte, 0, 1024)

	for _, v := range ls {
		b = append(b, v.Name...)
		b = append(b, sep)
		b = append(b, v.Value...)
		b = append(b, sep)
	}
	return xxhash.Sum64(b)
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	for _, l := range ls {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// Equals returns whether the two label sets are equal.
func (ls Labels) Equals(o Labels) bool {
	if len(ls) != len(o) {
		return false
	}
	for i, l := range ls {
		if l.Name != o[i].Name || l.Value != o[i].Value {
			return false
		}
	}
	return true
}

// Map returns a string map of the labels.
func (ls Labels) Map() map[string]string {
	m := make(map[string]string, len(ls))
	for _, l := range ls {
		m[l.Name] = l.Value
	}
	return m
}

// NewLabels returns a sorted Labels from the given labels.
// The caller has to guarantee that all label names are unique.
func NewLabels(ls ...Label) Labels {
	set := make(Labels, 0, len(ls))
	for _, l := range ls {
		set = append(set, l)
	}
	sort.Sort(set)

	return set
}

// LabelsFromMap returns new sorted Labels from the given map.
func LabelsFromMap(m map[string]string) Labels {
	l := make([]Label, 0, len(m))
	for k, v := range m {
		l = append(l, Label{Name: k, Value: v})
	}
	return NewLabels(l...)
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
func (es MultiError) Add(err error) {
	if err != nil {
		es = append(es, err)
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
	h := reflect.StringHeader{
		Data: uintptr(unsafe.Pointer(&b[0])),
		Len:  len(b),
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
