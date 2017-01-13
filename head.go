package tsdb

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/bradfitz/slice"
	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
)

// headBlock handles reads and writes of time series data within a time window.
type headBlock struct {
	mtx sync.RWMutex
	dir string

	// descs holds all chunk descs for the head block. Each chunk implicitly
	// is assigned the index as its ID.
	series []*memSeries
	// mapping maps a series ID to its position in an ordered list
	// of all series. The orderDirty flag indicates that it has gone stale.
	mapper *positionMapper
	// hashes contains a collision map of label set hashes of chunks
	// to their chunk descs.
	hashes map[uint64][]*memSeries

	values   map[string]stringset // label names to possible values
	postings *memPostings         // postings lists for terms

	wal *WAL

	stats *BlockStats
}

// openHeadBlock creates a new empty head block.
func openHeadBlock(dir string, l log.Logger) (*headBlock, error) {
	wal, err := OpenWAL(dir, log.NewContext(l).With("component", "wal"), 15*time.Second)
	if err != nil {
		return nil, err
	}

	b := &headBlock{
		dir:      dir,
		series:   []*memSeries{},
		hashes:   map[uint64][]*memSeries{},
		values:   map[string]stringset{},
		postings: &memPostings{m: make(map[term][]uint32)},
		wal:      wal,
		mapper:   newPositionMapper(nil),
	}
	b.stats = &BlockStats{
		MinTime: math.MinInt64,
		MaxTime: math.MaxInt64,
	}

	err = wal.ReadAll(&walHandler{
		series: func(lset labels.Labels) {
			b.create(lset.Hash(), lset)
			b.stats.SeriesCount++
		},
		sample: func(s refdSample) {
			si := s.ref

			cd := b.series[si]
			cd.append(s.t, s.v)

			if s.t > b.stats.MaxTime {
				b.stats.MaxTime = s.t
			}
			b.stats.SampleCount++
		},
	})
	if err != nil {
		return nil, err
	}

	b.updateMapping()

	return b, nil
}

// Close syncs all data and closes underlying resources of the head block.
func (h *headBlock) Close() error {
	return h.wal.Close()
}

func (h *headBlock) Dir() string          { return h.dir }
func (h *headBlock) Persisted() bool      { return false }
func (h *headBlock) Index() IndexReader   { return &headIndexReader{h} }
func (h *headBlock) Series() SeriesReader { return &headSeriesReader{h} }

// Stats returns statisitics about the indexed data.
func (h *headBlock) Stats() BlockStats {
	h.stats.mtx.RLock()
	defer h.stats.mtx.RUnlock()

	return *h.stats
}

func (h *headBlock) Appender() Appender {
	h.mtx.RLock()
	return &headAppender{headBlock: h, samples: getHeadAppendBuffer()}
}

var headPool = sync.Pool{}

func getHeadAppendBuffer() []refdSample {
	b := headPool.Get()
	if b == nil {
		return make([]refdSample, 0, 512)
	}
	return b.([]refdSample)
}

func putHeadAppendBuffer(b []refdSample) {
	headPool.Put(b[:0])
}

type headAppender struct {
	*headBlock

	newSeries map[uint64]hashedLabels
	newHashes map[uint64]uint64
	refmap    map[uint64]uint64
	newLabels []labels.Labels

	samples []refdSample
}

type hashedLabels struct {
	hash   uint64
	labels labels.Labels
}

type refdSample struct {
	ref uint64
	t   int64
	v   float64
}

func (a *headAppender) SetSeries(lset labels.Labels) (uint64, error) {
	return a.setSeries(lset.Hash(), lset)
}

func (a *headAppender) setSeries(hash uint64, lset labels.Labels) (uint64, error) {
	if ms := a.get(hash, lset); ms != nil {
		return uint64(ms.ref), nil
	}
	if ref, ok := a.newHashes[hash]; ok {
		return uint64(ref), nil
	}

	// We only know the actual reference after committing. We generate an
	// intermediate reference only valid for this batch.
	// It is indicated by the the LSB of the 4th byte being set to 1.
	// We use a random ID to avoid collisions when new series are created
	// in two subsequent batches. (TODO(fabxc): safe enough?)
	ref := uint64(rand.Int31()) | (1 << 32)

	if a.newSeries == nil {
		a.newSeries = map[uint64]hashedLabels{}
		a.newHashes = map[uint64]uint64{}
		a.refmap = map[uint64]uint64{}
	}
	a.newSeries[ref] = hashedLabels{hash: hash, labels: lset}
	a.newHashes[hash] = ref

	return ref, nil
}

func (a *headAppender) Add(ref uint64, t int64, v float64) error {
	// We only own the first 5 bytes of the reference. Anything before is
	// used by higher-order appenders. We erase it to avoid issues.
	ref = (ref << 31) >> 31

	// Distinguish between existing series and series created in
	// this transaction.
	if ref&(1<<32) > 0 {
		if _, ok := a.newSeries[ref]; !ok {
			return errNotFound
		}
		// TODO(fabxc): we also have to validate here that the
		// sample sequence is valid.
		// We also have to revalidate it as we switch locks an create
		// the new series.
		a.samples = append(a.samples, refdSample{
			ref: ref,
			t:   t,
			v:   v,
		})
		return nil
	}

	ms := a.series[int(ref)]
	if ms == nil {
		return errNotFound
	}
	c := ms.head()

	// TODO(fabxc): memory series should be locked here already.
	// Only problem is release of locks in case of a rollback.
	if t < c.maxTime {
		return ErrOutOfOrderSample
	}
	if c.maxTime == t && ms.lastValue != v {
		return ErrAmendSample
	}

	a.samples = append(a.samples, refdSample{
		ref: ref,
		t:   t,
		v:   v,
	})
	return nil
}

func (a *headAppender) createSeries() {
	if len(a.newSeries) == 0 {
		return
	}
	a.newLabels = make([]labels.Labels, 0, len(a.newSeries))
	base0 := len(a.series)

	a.mtx.RUnlock()
	a.mtx.Lock()

	base1 := len(a.series)

	for ref, l := range a.newSeries {
		// We switched locks and have to re-validate that the series were not
		// created by another goroutine in the meantime.
		if base1 > base0 {
			if ms := a.get(l.hash, l.labels); ms != nil {
				a.refmap[ref] = uint64(ms.ref)
				continue
			}
		}
		// Series is still new.
		a.newLabels = append(a.newLabels, l.labels)
		a.refmap[ref] = uint64(len(a.series))

		a.create(l.hash, l.labels)
	}

	a.mtx.Unlock()
	a.mtx.RLock()
}

func (a *headAppender) Commit() error {
	defer putHeadAppendBuffer(a.samples)
	// Write all new series and samples to the WAL and add it to the
	// in-mem database on success.
	if err := a.wal.Log(a.newLabels, a.samples); err != nil {
		a.mtx.RUnlock()
		return err
	}

	a.createSeries()

	var (
		total = uint64(len(a.samples))
		mint  = int64(math.MaxInt64)
		maxt  = int64(math.MinInt64)
	)

	for i := range a.samples {
		s := &a.samples[i]

		if s.ref&(1<<32) > 0 {
			s.ref = a.refmap[s.ref]
		}
		if !a.series[s.ref].append(s.t, s.v) {
			total--
		}
	}
	a.mtx.RUnlock()

	a.stats.mtx.Lock()
	defer a.stats.mtx.Unlock()

	a.stats.SampleCount += total
	a.stats.SeriesCount += uint64(len(a.newSeries))

	if mint < a.stats.MinTime {
		a.stats.MinTime = mint
	}
	if maxt > a.stats.MaxTime {
		a.stats.MaxTime = maxt
	}

	return nil
}

func (a *headAppender) Rollback() error {
	putHeadAppendBuffer(a.samples)
	a.mtx.RUnlock()
	return nil
}

type headSeriesReader struct {
	*headBlock
}

// Chunk returns the chunk for the reference number.
func (h *headSeriesReader) Chunk(ref uint32) (chunks.Chunk, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	c := &safeChunk{
		Chunk: h.series[ref>>8].chunks[int((ref<<24)>>24)].chunk,
		s:     h.series[ref>>8],
		i:     int((ref << 24) >> 24),
	}
	return c, nil
}

type safeChunk struct {
	chunks.Chunk
	s *memSeries
	i int
}

func (c *safeChunk) Iterator() chunks.Iterator {
	c.s.mtx.RLock()
	defer c.s.mtx.RUnlock()
	return c.s.iterator(c.i)
}

// func (c *safeChunk) Appender() (chunks.Appender, error) { panic("illegal") }
// func (c *safeChunk) Bytes() []byte                      { panic("illegal") }
// func (c *safeChunk) Encoding() chunks.Encoding          { panic("illegal") }

type headIndexReader struct {
	*headBlock
}

// LabelValues returns the possible label values
func (h *headIndexReader) LabelValues(names ...string) (StringTuples, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	if len(names) != 1 {
		return nil, errInvalidSize
	}
	var sl []string

	for s := range h.values[names[0]] {
		sl = append(sl, s)
	}
	sort.Strings(sl)

	return &stringTuples{l: len(names), s: sl}, nil
}

// Postings returns the postings list iterator for the label pair.
func (h *headIndexReader) Postings(name, value string) (Postings, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	return h.postings.get(term{name: name, value: value}), nil
}

// Series returns the series for the given reference.
func (h *headIndexReader) Series(ref uint32) (labels.Labels, []ChunkMeta, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	if int(ref) >= len(h.series) {
		return nil, nil, errNotFound
	}
	s := h.series[ref]
	metas := make([]ChunkMeta, 0, len(s.chunks))

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for i, c := range s.chunks {
		metas = append(metas, ChunkMeta{
			MinTime: c.minTime,
			MaxTime: c.maxTime,
			Ref:     (ref << 8) | uint32(i),
		})
	}

	return s.lset, metas, nil
}

func (h *headIndexReader) LabelIndices() ([][]string, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	res := [][]string{}

	for s := range h.values {
		res = append(res, []string{s})
	}
	return res, nil
}

func (h *headIndexReader) Stats() (BlockStats, error) {
	h.stats.mtx.RLock()
	defer h.stats.mtx.RUnlock()
	return *h.stats, nil
}

// get retrieves the chunk with the hash and label set and creates
// a new one if it doesn't exist yet.
func (h *headBlock) get(hash uint64, lset labels.Labels) *memSeries {
	series := h.hashes[hash]

	for _, s := range series {
		if s.lset.Equals(lset) {
			return s
		}
	}
	return nil
}

func (h *headBlock) create(hash uint64, lset labels.Labels) *memSeries {
	s := &memSeries{
		lset: lset,
		ref:  uint32(len(h.series)),
	}

	// Allocate empty space until we can insert at the given index.
	h.series = append(h.series, s)

	h.hashes[hash] = append(h.hashes[hash], s)

	for _, l := range lset {
		valset, ok := h.values[l.Name]
		if !ok {
			valset = stringset{}
			h.values[l.Name] = valset
		}
		valset.set(l.Value)

		h.postings.add(s.ref, term{name: l.Name, value: l.Value})
	}

	h.postings.add(s.ref, term{})

	return s
}

var (
	// ErrOutOfOrderSample is returned if an appended sample has a
	// timestamp larger than the most recent sample.
	ErrOutOfOrderSample = errors.New("out of order sample")

	// ErrAmendSample is returned if an appended sample has the same timestamp
	// as the most recent sample but a different value.
	ErrAmendSample = errors.New("amending sample")

	ErrOutOfBounds = errors.New("out of bounds")
)

func (h *headBlock) fullness() float64 {
	h.stats.mtx.RLock()
	defer h.stats.mtx.RUnlock()

	return float64(h.stats.SampleCount) / float64(h.stats.SeriesCount+1) / 250
}

func (h *headBlock) updateMapping() {
	h.mtx.RLock()

	if h.mapper.sortable != nil && h.mapper.Len() == len(h.series) {
		h.mtx.RUnlock()
		return
	}

	series := make([]*memSeries, len(h.series))
	copy(series, h.series)

	h.mtx.RUnlock()

	s := slice.SortInterface(series, func(i, j int) bool {
		return labels.Compare(series[i].lset, series[j].lset) < 0
	})

	h.mapper.update(s)
}

// remapPostings changes the order of the postings from their ID to the ordering
// of the series they reference.
// Returned postings have no longer monotonic IDs and MUST NOT be used for regular
// postings set operations, i.e. intersect and merge.
func (h *headBlock) remapPostings(p Postings) Postings {
	list, err := expandPostings(p)
	if err != nil {
		return errPostings{err: err}
	}

	h.mapper.mtx.Lock()
	defer h.mapper.mtx.Unlock()

	h.updateMapping()
	h.mapper.Sort(list)

	return newListPostings(list)
}

type memSeries struct {
	mtx sync.RWMutex

	ref    uint32
	lset   labels.Labels
	chunks []*memChunk

	lastValue float64
	sampleBuf [4]sample

	app chunks.Appender // Current appender for the chunkdb.
}

func (s *memSeries) cut() *memChunk {
	c := &memChunk{
		chunk:   chunks.NewXORChunk(),
		maxTime: math.MinInt64,
	}
	s.chunks = append(s.chunks, c)

	app, err := c.chunk.Appender()
	if err != nil {
		panic(err)
	}

	s.app = app
	return c
}

func (s *memSeries) append(t int64, v float64) bool {
	var c *memChunk

	if s.app == nil || s.head().samples > 10050 {
		c = s.cut()
		c.minTime = t
	} else {
		c = s.head()
		// Skip duplicate samples.
		if c.maxTime == t && s.lastValue != v {
			return false
		}
	}
	s.app.Append(t, v)

	c.maxTime = t
	c.samples++

	s.lastValue = v

	s.sampleBuf[0] = s.sampleBuf[1]
	s.sampleBuf[1] = s.sampleBuf[2]
	s.sampleBuf[2] = s.sampleBuf[3]
	s.sampleBuf[3] = sample{t: t, v: v}

	return true
}

func (s *memSeries) iterator(i int) chunks.Iterator {
	c := s.chunks[i]

	if i < len(s.chunks)-1 {
		return c.chunk.Iterator()
	}

	it := &memSafeIterator{
		Iterator: c.chunk.Iterator(),
		i:        -1,
		total:    c.samples,
		buf:      s.sampleBuf,
	}
	return it
}

func (s *memSeries) head() *memChunk {
	return s.chunks[len(s.chunks)-1]
}

type memChunk struct {
	chunk            chunks.Chunk
	minTime, maxTime int64
	samples          int
}

type memSafeIterator struct {
	chunks.Iterator

	i     int
	total int
	buf   [4]sample
}

func (it *memSafeIterator) Next() bool {
	if it.i+1 >= it.total {
		return false
	}
	it.i++
	if it.total-it.i > 4 {
		return it.Iterator.Next()
	}
	return true
}

func (it *memSafeIterator) At() (int64, float64) {
	if it.total-it.i > 4 {
		return it.Iterator.At()
	}
	s := it.buf[4-(it.total-it.i)]
	return s.t, s.v
}

// positionMapper stores a position mapping from unsorted to
// sorted indices of a sortable collection.
type positionMapper struct {
	mtx      sync.RWMutex
	sortable sort.Interface
	iv, fw   []int
}

func newPositionMapper(s sort.Interface) *positionMapper {
	m := &positionMapper{}
	if s != nil {
		m.update(s)
	}
	return m
}

func (m *positionMapper) Len() int           { return m.sortable.Len() }
func (m *positionMapper) Less(i, j int) bool { return m.sortable.Less(i, j) }

func (m *positionMapper) Swap(i, j int) {
	m.sortable.Swap(i, j)

	m.iv[i], m.iv[j] = m.iv[j], m.iv[i]
}

func (m *positionMapper) Sort(l []uint32) {
	slice.Sort(l, func(i, j int) bool {
		return m.fw[l[i]] < m.fw[l[j]]
	})
}

func (m *positionMapper) update(s sort.Interface) {
	m.sortable = s

	m.iv = make([]int, s.Len())
	m.fw = make([]int, s.Len())

	for i := range m.iv {
		m.iv[i] = i
	}
	sort.Sort(m)

	for i, k := range m.iv {
		m.fw[k] = i
	}
}
