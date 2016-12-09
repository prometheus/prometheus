package tsdb

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/fabxc/tsdb/chunks"
)

// HeadBlock handles reads and writes of time series data within a time window.
type HeadBlock struct {
	mtx     sync.RWMutex
	descs   map[uint64][]*chunkDesc // labels hash to possible chunks descs
	forward map[uint32]*chunkDesc   // chunk ID to chunk desc
	values  map[string]stringset    // label names to possible values
	ivIndex *memIndex               // inverted index for label pairs

	samples uint64 // total samples in the block.
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
	h.index(cd)

	h.descs[hash] = append(cds, cd)
	return cd
}

func (h *HeadBlock) index(chkd *chunkDesc) {
	// Add each label pair as a term to the inverted index.
	terms := make([]string, 0, len(chkd.lset))
	b := make([]byte, 0, 64)

	for _, l := range chkd.lset {
		b = append(b, l.Name...)
		b = append(b, sep)
		b = append(b, l.Value...)

		terms = append(terms, string(b))
		b = b[:0]

		// Add to label name to values index.
		valset, ok := h.values[l.Name]
		if !ok {
			valset = stringset{}
			h.values[l.Name] = valset
		}
		valset.set(l.Value)
	}
	id := h.ivIndex.add(terms...)

	// Store forward index for the returned ID.
	h.forward[id] = chkd
}

// append adds the sample to the headblock.
func (h *HeadBlock) append(hash uint64, lset Labels, ts int64, v float64) error {
	if err := h.get(hash, lset).append(ts, v); err != nil {
		return err
	}
	h.samples++
	return nil
}

func (h *HeadBlock) stats() *seriesStats {
	return &seriesStats{
		series:  uint32(len(h.forward)),
		samples: h.samples,
	}
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

type stringset map[string]struct{}

func (ss stringset) set(s string) {
	ss[s] = struct{}{}
}

func (ss stringset) has(s string) bool {
	_, ok := ss[s]
	return ok
}

func (ss stringset) String() string {
	return strings.Join(ss.slice(), ",")
}

func (ss stringset) slice() []string {
	slice := make([]string, 0, len(ss))
	for k := range ss {
		slice = append(slice, k)
	}
	sort.Strings(slice)
	return slice
}
