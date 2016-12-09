package tsdb

import (
	"math"
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

func (h *HeadBlock) stats() *blockStats {
	return &blockStats{
		chunks:  uint32(h.index.numSeries()),
		samples: h.samples,
	}
}
