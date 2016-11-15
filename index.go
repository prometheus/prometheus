package tsdb

import (
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/fabxc/tsdb/index"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

const (
	defaultIndexerTimeout = 1 * time.Second
	defaultIndexerQsize   = 500000
)

// indexer asynchronously indexes chunks in batches. It indexes all labels
// of a chunk with a forward mapping and additionally indexes the chunk for
// the time slice of its first sample.
type indexer struct {
	*chunkBatchProcessor

	ix *index.Index
	mc *memChunks
}

// Create batch indexer that creates new index documents
// and indexes them by the metric fields.
// Its post-indexing hook populates the in-memory chunk forward index.
func newMetricIndexer(path string, qsz int, qto time.Duration) (*indexer, error) {
	ix, err := index.Open(path, nil)
	if err != nil {
		return nil, err
	}

	i := &indexer{
		ix:                  ix,
		chunkBatchProcessor: newChunkBatchProcessor(log.Base(), qsz, qto),
	}
	i.chunkBatchProcessor.processf = i.index

	return i, nil
}

func (ix *indexer) Querier() (*index.Querier, error) {
	return ix.ix.Querier()
}

const (
	timeSliceField = "__ts__"
	timeSliceSize  = 3 * time.Hour
)

func timeSlice(t model.Time) model.Time {
	return t - (t % model.Time(timeSliceSize/time.Millisecond))
}

func timeString(t model.Time) string {
	return strconv.FormatInt(int64(t), 16)
}

func (ix *indexer) close() error {
	return ix.ix.Close()
}

func (ix *indexer) index(cds ...*chunkDesc) error {
	b, err := ix.ix.Batch()
	if err != nil {
		return err
	}

	ids := make([]ChunkID, len(cds))
	for i, cd := range cds {
		terms := make(index.Terms, 0, len(cd.met))
		for k, v := range cd.met {
			t := index.Term{Field: string(k), Val: string(v)}
			terms = append(terms, t)
		}
		id := b.Add(terms)
		ts := timeSlice(cd.firstTime)

		// If the chunk has a higher time slice than the high one,
		// don't index. It will be indexed when the next time slice
		// is initiated over all memory chunks.
		if ts <= ix.mc.highTime {
			b.SecondaryIndex(id, index.Term{
				Field: timeSliceField,
				Val:   timeString(ts),
			})
		}

		ids[i] = ChunkID(id)
	}

	if err := b.Commit(); err != nil {
		return err
	}

	// We have to lock here already instead of post-commit as otherwise we might
	// generate new chunk IDs, skip their indexing, and have a reindexTime being
	// called with the chunk ID not being visible yet.
	// TODO(fabxc): move back up
	ix.mc.mtx.Lock()
	defer ix.mc.mtx.Unlock()

	// Make in-memory chunks visible for read.
	for i, cd := range cds {
		atomic.StoreUint64((*uint64)(&cd.id), uint64(ids[i]))
		ix.mc.chunks[cd.id] = cd
	}
	return nil
}

// reindexTime creates an initial time slice index over all chunk IDs.
// Any future chunks indexed for the same time slice must have higher IDs.
func (ix *indexer) reindexTime(ids ChunkIDs, ts model.Time) error {
	b, err := ix.ix.Batch()
	if err != nil {
		return err
	}
	sort.Sort(ids)
	t := index.Term{Field: timeSliceField, Val: timeString(ts)}

	for _, id := range ids {
		b.SecondaryIndex(index.DocID(id), t)
	}
	return b.Commit()
}
