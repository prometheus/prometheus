package tsdb

import (
	"errors"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/bradfitz/slice"
	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
)

// HeadBlock handles reads and writes of time series data within a time window.
type HeadBlock struct {
	mtx sync.RWMutex
	d   string

	// descs holds all chunk descs for the head block. Each chunk implicitly
	// is assigned the index as its ID.
	descs []*chunkDesc
	// mapping maps a series ID to its position in an ordered list
	// of all series. The orderDirty flag indicates that it has gone stale.
	mapper *positionMapper
	// hashes contains a collision map of label set hashes of chunks
	// to their chunk descs.
	hashes map[uint64][]*chunkDesc

	values   map[string]stringset // label names to possible values
	postings *memPostings         // postings lists for terms

	wal *WAL

	bstats *BlockStats
}

// OpenHeadBlock creates a new empty head block.
func OpenHeadBlock(dir string, l log.Logger) (*HeadBlock, error) {
	wal, err := OpenWAL(dir, log.NewContext(l).With("component", "wal"), 15*time.Second)
	if err != nil {
		return nil, err
	}

	b := &HeadBlock{
		d:        dir,
		descs:    []*chunkDesc{},
		hashes:   map[uint64][]*chunkDesc{},
		values:   map[string]stringset{},
		postings: &memPostings{m: make(map[term][]uint32)},
		wal:      wal,
		mapper:   newPositionMapper(nil),
	}
	b.bstats = &BlockStats{
		MinTime: math.MaxInt64,
		MaxTime: math.MinInt64,
	}

	err = wal.ReadAll(&walHandler{
		series: func(lset labels.Labels) {
			b.create(lset.Hash(), lset)
		},
		sample: func(s hashedSample) {
			cd := b.descs[s.ref]

			// Duplicated from appendBatch – TODO(fabxc): deduplicate?
			if cd.lastTimestamp == s.t && cd.lastValue != s.v {
				return
			}
			cd.append(s.t, s.v)

			if s.t > b.bstats.MaxTime {
				b.bstats.MaxTime = s.t
			}
			if s.t < b.bstats.MinTime {
				b.bstats.MinTime = s.t
			}
			b.bstats.SampleCount++
		},
	})
	if err != nil {
		return nil, err
	}

	b.updateMapping()

	return b, nil
}

// Close syncs all data and closes underlying resources of the head block.
func (h *HeadBlock) Close() error {
	return h.wal.Close()
}

func (h *HeadBlock) dir() string          { return h.d }
func (h *HeadBlock) persisted() bool      { return false }
func (h *HeadBlock) index() IndexReader   { return h }
func (h *HeadBlock) series() SeriesReader { return h }

func (h *HeadBlock) stats() BlockStats {
	h.bstats.mtx.RLock()
	defer h.bstats.mtx.RUnlock()

	return *h.bstats
}

// Chunk returns the chunk for the reference number.
func (h *HeadBlock) Chunk(ref uint32) (chunks.Chunk, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	if int(ref) >= len(h.descs) {
		return nil, errNotFound
	}
	return h.descs[int(ref)].chunk, nil
}

type headSeriesReader struct {
	h *HeadBlock
}

func (h *headSeriesReader) Chunk(ref uint32) (chunks.Chunk, error) {
	h.h.mtx.RLock()
	defer h.h.mtx.RUnlock()

	if int(ref) >= len(h.h.descs) {
		return nil, errNotFound
	}
	return &safeChunk{
		cd: h.h.descs[int(ref)],
	}, nil
}

type safeChunk struct {
	cd *chunkDesc
}

func (c *safeChunk) Iterator() chunks.Iterator {
	c.cd.mtx.Lock()
	defer c.cd.mtx.Unlock()

	return c.cd.iterator()
}

func (c *safeChunk) Appender() (chunks.Appender, error) {
	panic("illegal")
}

func (c *safeChunk) Bytes() []byte {
	panic("illegal")
}

func (c *safeChunk) Encoding() chunks.Encoding {
	panic("illegal")
}

func (h *HeadBlock) interval() (int64, int64) {
	h.bstats.mtx.RLock()
	defer h.bstats.mtx.RUnlock()
	return h.bstats.MinTime, h.bstats.MaxTime
}

// Stats returns statisitics about the indexed data.
func (h *HeadBlock) Stats() (BlockStats, error) {
	h.bstats.mtx.RLock()
	defer h.bstats.mtx.RUnlock()
	return *h.bstats, nil
}

// LabelValues returns the possible label values
func (h *HeadBlock) LabelValues(names ...string) (StringTuples, error) {
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
func (h *HeadBlock) Postings(name, value string) (Postings, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	return h.postings.get(term{name: name, value: value}), nil
}

// Series returns the series for the given reference.
func (h *HeadBlock) Series(ref uint32) (labels.Labels, []ChunkMeta, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	if int(ref) >= len(h.descs) {
		return nil, nil, errNotFound
	}
	cd := h.descs[ref]

	cd.mtx.RLock()
	meta := ChunkMeta{
		MinTime: cd.firstTimestamp,
		MaxTime: cd.lastTimestamp,
		Ref:     ref,
	}
	cd.mtx.RUnlock()
	return cd.lset, []ChunkMeta{meta}, nil
}

func (h *HeadBlock) LabelIndices() ([][]string, error) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	res := [][]string{}

	for s := range h.values {
		res = append(res, []string{s})
	}
	return res, nil
}

// get retrieves the chunk with the hash and label set and creates
// a new one if it doesn't exist yet.
func (h *HeadBlock) get(hash uint64, lset labels.Labels) *chunkDesc {
	cds := h.hashes[hash]

	for _, cd := range cds {
		if cd.lset.Equals(lset) {
			return cd
		}
	}
	return nil
}

func (h *HeadBlock) create(hash uint64, lset labels.Labels) *chunkDesc {
	cd := &chunkDesc{
		lset:          lset,
		chunk:         chunks.NewXORChunk(),
		lastTimestamp: math.MinInt64,
	}

	var err error
	cd.app, err = cd.chunk.Appender()
	if err != nil {
		// Getting an Appender for a new chunk must not panic.
		panic(err)
	}
	// Index the new chunk.
	cd.ref = uint32(len(h.descs))

	h.descs = append(h.descs, cd)
	h.hashes[hash] = append(h.hashes[hash], cd)

	for _, l := range lset {
		valset, ok := h.values[l.Name]
		if !ok {
			valset = stringset{}
			h.values[l.Name] = valset
		}
		valset.set(l.Value)

		h.postings.add(cd.ref, term{name: l.Name, value: l.Value})
	}

	h.postings.add(cd.ref, term{})

	return cd
}

var (
	// ErrOutOfOrderSample is returned if an appended sample has a
	// timestamp larger than the most recent sample.
	ErrOutOfOrderSample = errors.New("out of order sample")

	// ErrAmendSample is returned if an appended sample has the same timestamp
	// as the most recent sample but a different value.
	ErrAmendSample = errors.New("amending sample")
)

func (h *HeadBlock) appendBatch(samples []hashedSample) error {
	// Find head chunks for all samples and allocate new IDs/refs for
	// ones we haven't seen before.
	var (
		newSeries    []labels.Labels
		newSamples   []*hashedSample
		newHashes    []uint64
		uniqueHashes = map[uint64]uint32{}
	)
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	for i := range samples {
		s := &samples[i]

		cd := h.get(s.hash, s.labels)
		if cd != nil {
			// Samples must only occur in order.
			if s.t < cd.lastTimestamp {
				return ErrOutOfOrderSample
			}
			if cd.lastTimestamp == s.t && cd.lastValue != s.v {
				return ErrAmendSample
			}
			// TODO(fabxc): sample refs are only scoped within a block for
			// now and we ignore any previously set value
			s.ref = cd.ref
			continue
		}

		// There may be several samples for a new series in a batch.
		// We don't want to reserve a new space for each.
		if ref, ok := uniqueHashes[s.hash]; ok {
			s.ref = ref
			newSamples = append(newSamples, s)
			continue
		}
		s.ref = uint32(len(newSeries))
		uniqueHashes[s.hash] = s.ref

		newSeries = append(newSeries, s.labels)
		newHashes = append(newHashes, s.hash)
		newSamples = append(newSamples, s)
	}

	// Write all new series and samples to the WAL and add it to the
	// in-mem database on success.
	if err := h.wal.Log(newSeries, samples); err != nil {
		return err
	}

	// After the samples were successfully written to the WAL, there may
	// be no further failures.
	if len(newSeries) > 0 {
		h.mtx.RUnlock()
		h.mtx.Lock()

		base := len(h.descs)

		for i, s := range newSeries {
			h.create(newHashes[i], s)
		}
		for _, s := range newSamples {
			s.ref = uint32(base) + s.ref
		}

		h.mtx.Unlock()
		h.mtx.RLock()
	}

	var (
		total = uint64(len(samples))
		mint  = int64(math.MaxInt64)
		maxt  = int64(math.MinInt64)
	)
	for _, s := range samples {
		cd := h.descs[s.ref]
		cd.mtx.Lock()
		// Skip duplicate samples.
		if cd.lastTimestamp == s.t && cd.lastValue != s.v {
			total--
			continue
		}
		cd.append(s.t, s.v)
		cd.mtx.Unlock()

		if mint > s.t {
			mint = s.t
		}
		if maxt < s.t {
			maxt = s.t
		}
	}

	h.bstats.mtx.Lock()
	defer h.bstats.mtx.Unlock()

	h.bstats.SampleCount += total
	h.bstats.SeriesCount += uint64(len(newSeries))
	h.bstats.ChunkCount += uint64(len(newSeries)) // head block has one chunk/series

	if mint < h.bstats.MinTime {
		h.bstats.MinTime = mint
	}
	if maxt > h.bstats.MaxTime {
		h.bstats.MaxTime = maxt
	}

	return nil
}

func (h *HeadBlock) fullness() float64 {
	h.bstats.mtx.RLock()
	defer h.bstats.mtx.RUnlock()

	return float64(h.bstats.SampleCount) / float64(h.bstats.SeriesCount+1) / 250
}

func (h *HeadBlock) updateMapping() {
	h.mtx.RLock()

	if h.mapper.sortable != nil && h.mapper.Len() == len(h.descs) {
		h.mtx.RUnlock()
		return
	}

	cds := make([]*chunkDesc, len(h.descs))
	copy(cds, h.descs)

	h.mtx.RUnlock()

	s := slice.SortInterface(cds, func(i, j int) bool {
		return labels.Compare(cds[i].lset, cds[j].lset) < 0
	})

	h.mapper.update(s)
}

// remapPostings changes the order of the postings from their ID to the ordering
// of the series they reference.
// Returned postings have no longer monotonic IDs and MUST NOT be used for regular
// postings set operations, i.e. intersect and merge.
func (h *HeadBlock) remapPostings(p Postings) Postings {
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

// chunkDesc wraps a plain data chunk and provides cached meta data about it.
type chunkDesc struct {
	mtx sync.RWMutex

	ref   uint32
	lset  labels.Labels
	chunk chunks.Chunk

	// Caching fielddb.
	firstTimestamp int64
	lastTimestamp  int64
	lastValue      float64
	numSamples     int

	sampleBuf [4]sample

	app chunks.Appender // Current appender for the chunkdb.
}

func (cd *chunkDesc) append(ts int64, v float64) {
	if cd.numSamples == 0 {
		cd.firstTimestamp = ts
	}
	cd.app.Append(ts, v)

	cd.lastTimestamp = ts
	cd.lastValue = v
	cd.numSamples++

	cd.sampleBuf[0] = cd.sampleBuf[1]
	cd.sampleBuf[1] = cd.sampleBuf[2]
	cd.sampleBuf[2] = cd.sampleBuf[3]
	cd.sampleBuf[3] = sample{t: ts, v: v}
}

func (cd *chunkDesc) iterator() chunks.Iterator {
	it := &memSafeIterator{
		Iterator: cd.chunk.Iterator(),
		i:        -1,
		total:    cd.numSamples,
		buf:      cd.sampleBuf,
	}
	return it
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
