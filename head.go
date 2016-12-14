package tsdb

import (
	"math"
	"sort"
	"sync"

	"github.com/fabxc/tsdb/chunks"
)

// HeadBlock handles reads and writes of time series data within a time window.
type HeadBlock struct {
	mtx   sync.RWMutex
	descs map[uint64][]*chunkDesc // labels hash to possible chunks descs
	index *memIndex

	samples       uint64 // total samples in the block
	highTimestamp int64  // highest timestamp of any sample
	baseTimestamp int64  // all samples are strictly later
}

// NewHeadBlock creates a new empty head block.
func NewHeadBlock(baseTime int64) *HeadBlock {
	return &HeadBlock{
		descs:         make(map[uint64][]*chunkDesc, 2048),
		index:         newMemIndex(),
		baseTimestamp: baseTime,
	}
}

// Querier returns a new querier over the head block.
func (h *HeadBlock) Querier(mint, maxt int64) Querier {
	return newBlockQuerier(h, h, mint, maxt)
}

func (h *HeadBlock) Chunk(ref uint32) (chunks.Chunk, error) {
	c, ok := h.index.forward[ref]
	if !ok {
		return nil, errNotFound
	}
	return c.chunk, nil
}

// Stats returns statisitics about the indexed data.
func (h *HeadBlock) Stats() (*BlockStats, error) {
	return nil, nil
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
func (h *HeadBlock) Postings(name, value string) (Iterator, error) {
	return h.index.Postings(term{name, value}), nil
}

// Series returns the series for the given reference.
func (h *HeadBlock) Series(ref uint32) (Series, error) {
	cd, ok := h.index.forward[ref]
	if !ok {
		return nil, errNotFound
	}
	s := &series{
		labels: cd.lset,
		offsets: []ChunkOffset{
			{Value: h.baseTimestamp, Offset: 0},
		},
		chunk: func(ref uint32) (chunks.Chunk, error) {
			return cd.chunk, nil
		},
	}
	return s, nil
}

// get retrieves the chunk with the hash and label set and creates
// a new one if it doesn't exist yet.
func (h *HeadBlock) get(hash uint64, lset Labels) *chunkDesc {
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

	h.descs[hash] = append(cds, cd)
	return cd
}

// append adds the sample to the headblock.
func (h *HeadBlock) append(hash uint64, lset Labels, ts int64, v float64) error {
	if err := h.get(hash, lset).append(ts, v); err != nil {
		return err
	}
	h.samples++
	return nil
}

type blockStats struct {
	chunks  uint32
	samples uint64
}

func (h *HeadBlock) stats() *blockStats {
	return &blockStats{
		chunks:  uint32(h.index.numSeries()),
		samples: h.samples,
	}
}
