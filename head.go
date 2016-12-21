package tsdb

import (
	"math"
	"os"
	"sort"
	"sync"

	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
)

// HeadBlock handles reads and writes of time series data within a time window.
type HeadBlock struct {
	mtx   sync.RWMutex
	descs map[uint64][]*chunkDesc // labels hash to possible chunks descs
	index *memIndex

	stats BlockStats
}

// NewHeadBlock creates a new empty head block.
func NewHeadBlock(baseTime int64) *HeadBlock {
	b := &HeadBlock{
		descs: make(map[uint64][]*chunkDesc, 2048),
		index: newMemIndex(),
	}
	b.stats.MinTime = baseTime

	return b
}

// Querier returns a new querier over the head block.
func (h *HeadBlock) Querier(mint, maxt int64) Querier {
	return newBlockQuerier(h, h, mint, maxt)
}

// Chunk returns the chunk for the reference number.
func (h *HeadBlock) Chunk(ref uint32) (chunks.Chunk, error) {
	c, ok := h.index.forward[ref]
	if !ok {
		return nil, errNotFound
	}
	return c.chunk, nil
}

func (h *HeadBlock) interval() (int64, int64) {
	return h.stats.MinTime, h.stats.MaxTime
}

// Stats returns statisitics about the indexed data.
func (h *HeadBlock) Stats() (BlockStats, error) {
	return h.stats, nil
}

// LabelValues returns the possible label values
func (h *HeadBlock) LabelValues(names ...string) (StringTuples, error) {
	if len(names) != 1 {
		return nil, errInvalidSize
	}
	var sl []string

	for s := range h.index.values[names[0]] {
		sl = append(sl, s)
	}
	sort.Strings(sl)

	t := &stringTuples{
		l: len(names),
		s: sl,
	}
	return t, nil
}

// Postings returns the postings list iterator for the label pair.
func (h *HeadBlock) Postings(name, value string) (Postings, error) {
	return h.index.Postings(term{name, value}), nil
}

// Series returns the series for the given reference.
func (h *HeadBlock) Series(ref uint32, mint, maxt int64) (Series, error) {
	cd, ok := h.index.forward[ref]
	if !ok {
		return nil, errNotFound
	}
	if !intervalOverlap(cd.firsTimestamp, cd.lastTimestamp, mint, maxt) {
		return nil, nil
	}
	s := &chunkSeries{
		labels: cd.lset,
		chunks: []ChunkMeta{
			{MinTime: h.stats.MinTime, Ref: 0},
		},
		chunk: func(ref uint32) (chunks.Chunk, error) {
			return cd.chunk, nil
		},
	}
	return s, nil
}

// get retrieves the chunk with the hash and label set and creates
// a new one if it doesn't exist yet.
func (h *HeadBlock) get(hash uint64, lset labels.Labels) *chunkDesc {
	cds := h.descs[hash]
	for _, cd := range cds {
		if cd.lset.Equals(lset) {
			return cd
		}
	}
	// None of the given chunks was for the series, create a new one.
	cd := &chunkDesc{
		lset:  lset,
		chunk: chunks.NewXORChunk(int(math.MaxInt64)),
	}
	h.index.add(cd)

	// For the head block there's exactly one chunk per series.
	h.stats.ChunkCount++
	h.stats.SeriesCount++

	h.descs[hash] = append(cds, cd)
	return cd
}

// append adds the sample to the headblock.
func (h *HeadBlock) append(hash uint64, lset labels.Labels, ts int64, v float64) error {
	if err := h.get(hash, lset).append(ts, v); err != nil {
		return err
	}

	h.stats.SampleCount++

	if ts > h.stats.MaxTime {
		h.stats.MaxTime = ts
	}

	return nil
}

func (h *HeadBlock) persist(p string) (int64, error) {
	sf, err := os.Create(chunksFileName(p))
	if err != nil {
		return 0, err
	}
	xf, err := os.Create(indexFileName(p))
	if err != nil {
		return 0, err
	}

	iw := newIndexWriter(xf)
	sw := newSeriesWriter(sf, iw, h.stats.MinTime)

	defer sw.Close()
	defer iw.Close()

	for ref, cd := range h.index.forward {
		if err := sw.WriteSeries(ref, cd.lset, []*chunkDesc{cd}); err != nil {
			return 0, err
		}
	}

	if err := iw.WriteStats(h.stats); err != nil {
		return 0, err
	}
	for n, v := range h.index.values {
		s := make([]string, 0, len(v))
		for x := range v {
			s = append(s, x)
		}

		if err := iw.WriteLabelIndex([]string{n}, s); err != nil {
			return 0, err
		}
	}

	for t := range h.index.postings.m {
		if err := iw.WritePostings(t.name, t.value, h.index.postings.get(t)); err != nil {
			return 0, err
		}
	}

	return iw.Size() + sw.Size(), nil
}
