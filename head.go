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

	bstats BlockStats
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
	}

	b.bstats.MinTime = math.MaxInt64
	b.bstats.MaxTime = math.MinInt64

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

	b.rewriteMapping()

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
func (h *HeadBlock) stats() BlockStats    { return h.bstats }

// Chunk returns the chunk for the reference number.
func (h *HeadBlock) Chunk(ref uint32) (chunks.Chunk, error) {
	if int(ref) >= len(h.descs) {
		return nil, errNotFound
	}
	return h.descs[int(ref)].chunk, nil
}

func (h *HeadBlock) interval() (int64, int64) {
	return h.bstats.MinTime, h.bstats.MaxTime
}

// Stats returns statisitics about the indexed data.
func (h *HeadBlock) Stats() (BlockStats, error) {
	return h.bstats, nil
}

// LabelValues returns the possible label values
func (h *HeadBlock) LabelValues(names ...string) (StringTuples, error) {
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
	return h.postings.get(term{name: name, value: value}), nil
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

	slice.Sort(list, func(i, j int) bool {
		return h.mapper.fw[list[i]] < h.mapper.fw[list[j]]
	})

	return newListPostings(list)
}

// Series returns the series for the given reference.
func (h *HeadBlock) Series(ref uint32) (labels.Labels, []ChunkMeta, error) {
	if int(ref) >= len(h.descs) {
		return nil, nil, errNotFound
	}
	cd := h.descs[ref]

	meta := ChunkMeta{
		MinTime: cd.firstTimestamp,
		MaxTime: cd.lastTimestamp,
		Ref:     ref,
	}
	return cd.lset, []ChunkMeta{meta}, nil
}

func (h *HeadBlock) LabelIndices() ([][]string, error) {
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

	// For the head block there's exactly one chunk per series.
	h.bstats.ChunkCount++
	h.bstats.SeriesCount++

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
		newHashes    []uint64
		uniqueHashes = map[uint64]uint32{}
	)

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
			continue
		}
		s.ref = uint32(len(h.descs) + len(newSeries))
		uniqueHashes[s.hash] = s.ref

		newSeries = append(newSeries, s.labels)
		newHashes = append(newHashes, s.hash)
	}

	// Write all new series and samples to the WAL and add it to the
	// in-mem database on success.
	if err := h.wal.Log(newSeries, samples); err != nil {
		return err
	}

	// After the samples were successfully written to the WAL, there may
	// be no further failures.
	for i, s := range newSeries {
		h.create(newHashes[i], s)
	}
	// TODO(fabxc): just mark as dirty instead and trigger a remapping
	// periodically and upon querying.
	if len(newSeries) > 0 {
		h.rewriteMapping()
	}

	for _, s := range samples {
		cd := h.descs[s.ref]
		// Skip duplicate samples.
		if cd.lastTimestamp == s.t && cd.lastValue != s.v {
			continue
		}
		cd.append(s.t, s.v)

		if s.t > h.bstats.MaxTime {
			h.bstats.MaxTime = s.t
		}
		if s.t < h.bstats.MinTime {
			h.bstats.MinTime = s.t
		}
		h.bstats.SampleCount++
	}

	return nil
}

func (h *HeadBlock) rewriteMapping() {
	cds := make([]*chunkDesc, len(h.descs))
	copy(cds, h.descs)

	s := slice.SortInterface(cds, func(i, j int) bool {
		return labels.Compare(cds[i].lset, cds[j].lset) < 0
	})

	h.mapper = newPositionMapper(s)
}

// positionMapper stores a position mapping from unsorted to
// sorted indices of a sortable collection.
type positionMapper struct {
	sortable sort.Interface
	iv, fw   []int
}

func newPositionMapper(s sort.Interface) *positionMapper {
	m := &positionMapper{
		sortable: s,
		iv:       make([]int, s.Len()),
		fw:       make([]int, s.Len()),
	}
	for i := range m.iv {
		m.iv[i] = i
	}
	sort.Sort(m)

	for i, k := range m.iv {
		m.fw[k] = i
	}

	return m
}

func (m *positionMapper) Len() int           { return m.sortable.Len() }
func (m *positionMapper) Less(i, j int) bool { return m.sortable.Less(i, j) }

func (m *positionMapper) Swap(i, j int) {
	m.sortable.Swap(i, j)

	m.iv[i], m.iv[j] = m.iv[j], m.iv[i]
}
