package tsdb

import (
	"math"
	"sync"

	"github.com/fabxc/tsdb/chunks"
)

// HeadBlock handles reads and writes of time series data within a time window.
type HeadBlock struct {
	mtx     sync.RWMutex
	descs   map[uint64][]*chunkDesc // labels hash to possible chunks descs
	forward map[uint32]*chunkDesc   // chunk ID to chunk desc
	values  map[string][]string     // label names to possible values
	index   *memIndex               // inverted index for label pairs

	samples uint64
}

// get retrieves the chunk with the hash and label set and creates
// a new one if it doesn't exist yet.
func (h *HeadBlock) get(hash uint64, lset Labels) (*chunkDesc, bool) {
	cds := h.descs[hash]
	for _, cd := range cds {
		if cd.lset.Equals(lset) {
			return cd, false
		}
	}
	// None of the given chunks was for the series, create a new one.
	cd := &chunkDesc{
		lset:  lset,
		chunk: chunks.NewXORChunk(int(math.MaxInt64)),
	}

	h.descs[hash] = append(cds, cd)
	return cd, true
}

// append adds the sample to the headblock. If the series is seen
// for the first time it creates a chunk and index entries for it.
//
// TODO(fabxc): switch to single writer and append queue with optimistic concurrency?
func (h *HeadBlock) append(hash uint64, lset Labels, ts int64, v float64) error {
	chkd, created := h.get(hash, lset)
	if created {
		// Add each label pair as a term to the inverted index.
		terms := make([]string, 0, len(lset))
		b := make([]byte, 0, 64)

		for _, l := range lset {
			b = append(b, l.Name...)
			b = append(b, sep)
			b = append(b, l.Value...)

			terms = append(terms, string(b))
			b = b[:0]
		}
		id := h.index.add(terms...)

		// Store forward index for the returned ID.
		h.forward[id] = chkd
	}
	if err := chkd.append(ts, v); err != nil {
		return err
	}

	h.samples++
	return nil
}

func (h *HeadBlock) stats() *seriesStats {
	return &seriesStats{}
}

func (h *HeadBlock) seriesData() seriesDataIterator {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	it := &chunkDescsIterator{
		descs: make([]*chunkDesc, 0, len(h.forward)),
		i:     -1,
	}

	for _, cd := range h.forward {
		it.descs = append(it.descs, cd)
	}
	return it
}

type chunkDescsIterator struct {
	descs []*chunkDesc
	i     int
}

func (it *chunkDescsIterator) next() bool {
	it.i++
	return it.i < len(it.descs)
}

func (it *chunkDescsIterator) values() (skiplist, []chunks.Chunk) {
	return &simpleSkiplist{}, []chunks.Chunk{it.descs[it.i].chunk}
}

func (it *chunkDescsIterator) err() error {
	return nil
}
