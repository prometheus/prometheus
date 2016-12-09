// Package tsdb implements a time series storage for float64 sample data.
package tsdb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/cespare/xxhash"
	"github.com/fabxc/tsdb/chunks"
	"github.com/prometheus/common/log"
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

	shards []*SeriesShard
}

// TODO(fabxc): make configurable
const (
	seriesShardShift = 3
	numSeriesShards  = 1 << seriesShardShift
	maxChunkSize     = 1024
)

// Open or create a new DB.
func Open(path string, l log.Logger, opts *Options) (*DB, error) {
	if opts == nil {
		opts = DefaultOptions
	}
	if err := os.MkdirAll(path, 0777); err != nil {
		return nil, err
	}

	c := &DB{
		logger: l,
		opts:   opts,
		path:   path,
	}

	// Initialize vertical shards.
	// TODO(fabxc): validate shard number to be power of 2, which is required
	// for the bitshift-modulo when finding the right shard.
	for i := 0; i < numSeriesShards; i++ {
		path := filepath.Join(path, fmt.Sprintf("%d", i))
		c.shards = append(c.shards, NewSeriesShard(path))
	}

	// TODO(fabxc): run background compaction + GC.

	return c, nil
}

// Close the database.
func (db *DB) Close() error {
	var wg sync.WaitGroup

	start := time.Now()

	for i, shard := range db.shards {
		fmt.Println("shard", i)
		fmt.Println("	num chunks", len(shard.head.forward))
		fmt.Println("	num samples", shard.head.samples)

		wg.Add(1)
		go func(i int, shard *SeriesShard) {
			f, err := os.Create(filepath.Join(db.path, fmt.Sprintf("shard-%d-series", i)))
			if err != nil {
				panic(err)
			}
			bw := &blockWriter{block: shard.head}
			n, err := bw.writeSeries(f)
			if err != nil {
				panic(err)
			}
			fmt.Println("	wrote bytes", n)
			if err := f.Sync(); err != nil {
				panic(err)
			}

			if err := f.Close(); err != nil {
				panic(err)
			}
			wg.Done()
		}(i, shard)
	}
	wg.Wait()

	fmt.Println("final serialization took", time.Since(start))

	return nil
}

// Querier returns a new querier over the database.
func (db *DB) Querier(start, end int64) Querier {
	return nil
}

// Matcher matches a string.
type Matcher interface {
	// Match returns true if the matcher applies to the string value.
	Match(v string) bool
}

// Querier provides querying access over time series data of a fixed
// time range.
type Querier interface {
	// Range returns the timestamp range of the Querier.
	Range() (start, end int64)

	// Iterator returns an interator over the inverted index that
	// matches the key label by the constraints of Matcher.
	Iterator(key string, m Matcher) Iterator

	// Labels resolves a label reference into a set of labels.
	Labels(ref LabelRefs) (Labels, error)

	// Series returns series provided in the index iterator.
	Series(Iterator) []Series

	// LabelValues returns all potential values for a label name.
	LabelValues(string) []string
	// LabelValuesFor returns all potential values for a label name.
	// under the constraint of another label.
	LabelValuesFor(string, Label) []string

	// Close releases the resources of the Querier.
	Close() error
}

// Series represents a single time series.
type Series interface {
	// LabelsRef returns the label set reference
	LabelRefs() LabelRefs
	// Iterator returns a new iterator of the data of the series.
	Iterator() SeriesIterator
}

// SeriesIterator iterates over the data of a time series.
type SeriesIterator interface {
	// Seek advances the iterator forward to the given timestamp.
	// If there's no value exactly at ts, it advances to the last value
	// before ts.
	Seek(ts int64) bool
	// Values returns the current timestamp/value pair.
	Values() (int64, float64)
	// Next advances the iterator by one.
	Next() bool
	// Err returns the current error.
	Err() error
}

const sep = '\xff'

// SeriesShard handles reads and writes of time series falling into
// a hashed shard of a series.
type SeriesShard struct {
	path string

	mtx    sync.RWMutex
	blocks *Block
	head   *HeadBlock
}

// NewSeriesShard returns a new SeriesShard.
func NewSeriesShard(path string) *SeriesShard {
	return &SeriesShard{
		path: path,
		// TODO(fabxc): restore from checkpoint.
		head: &HeadBlock{
			ivIndex: newMemIndex(),
			descs:   map[uint64][]*chunkDesc{},
			values:  map[string]stringset{},
			forward: map[uint32]*chunkDesc{},
		},
		// TODO(fabxc): provide access to persisted blocks.
	}
}

// chunkDesc wraps a plain data chunk and provides cached meta data about it.
type chunkDesc struct {
	lset  Labels
	chunk chunks.Chunk

	// Caching fields.
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
	}
	if err := cd.app.Append(ts, v); err != nil {
		return err
	}

	cd.lastTimestamp = ts
	cd.lastValue = v

	return nil
}

// LabelRefs contains a reference to a label set that can be resolved
// against a Querier.
type LabelRefs struct {
	block   uint64
	offsets []uint32
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
	s := uint16(h >> (64 - seriesShardShift))

	v.Buckets[s] = append(v.Buckets[s], Sample{
		Hash:   h,
		Labels: lset,
		Value:  val,
	})
}

// AppendVector adds values for a list of label sets for the given timestamp
// in milliseconds.
func (db *DB) AppendVector(ts int64, v *Vector) error {
	// Sequentially add samples to shards.
	for s, bkt := range v.Buckets {
		shard := db.shards[s]

		// TODO(fabxc): benchmark whether grouping into shards and submitting to
		// shards in batches is more efficient.
		shard.head.mtx.Lock()

		for _, smpl := range bkt {
			if err := shard.head.append(smpl.Hash, smpl.Labels, ts, smpl.Value); err != nil {
				shard.head.mtx.Unlock()
				// TODO(fabxc): handle gracefully and collect multi-error.
				return err
			}
		}
		shard.head.mtx.Unlock()
	}

	return nil
}
