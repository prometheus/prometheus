package tsdb

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/index"
	"github.com/prometheus/common/model"
)

// SeriesIterator provides iteration over a time series associated with a metric.
type SeriesIterator interface {
	Metric() map[string]string
	Seek(model.Time) (model.SamplePair, bool)
	Next() (model.SamplePair, bool)
	Err() error
}

type chunkSeriesIterator struct {
	m      map[string]string
	chunks []chunks.Chunk

	err    error
	cur    chunks.Iterator
	curPos int
}

func newChunkSeriesIterator(m map[string]string, chunks []chunks.Chunk) *chunkSeriesIterator {
	return &chunkSeriesIterator{
		m:      m,
		chunks: chunks,
	}
}

func (it *chunkSeriesIterator) Metric() map[string]string {
	return it.m
}

func (it *chunkSeriesIterator) Seek(ts model.Time) (model.SamplePair, bool) {
	// Naively go through all chunk's first timestamps and pick the chunk
	// containing the seeked timestamp.
	// TODO(fabxc): this can be made smarter if it's a bottleneck.
	for i, chk := range it.chunks {
		cit := chk.Iterator()
		first, ok := cit.First()
		if !ok {
			it.err = cit.Err()
			return model.SamplePair{}, false
		}
		if first.Timestamp > ts {
			break
		}
		it.cur = cit
		it.curPos = i
	}
	return it.cur.Seek(ts)
}

func (it *chunkSeriesIterator) Next() (model.SamplePair, bool) {
	sp, ok := it.cur.Next()
	if ok {
		return sp, true
	}
	if it.cur.Err() != io.EOF {
		it.err = it.cur.Err()
		return model.SamplePair{}, false
	}
	if len(it.chunks) == it.curPos+1 {
		it.err = io.EOF
		return model.SamplePair{}, false
	}
	it.curPos++
	it.cur = it.chunks[it.curPos].Iterator()

	// Return first sample of the new chunks.
	return it.cur.Seek(0)
}

func (it *chunkSeriesIterator) Err() error {
	return it.err
}

// Querier allows several queries over the storage with a consistent view if the data.
type Querier struct {
	db *DB
	iq *index.Querier
}

// Querier returns a new Querier on the index at the current point in time.
func (db *DB) Querier() (*Querier, error) {
	iq, err := db.indexer.Querier()
	if err != nil {
		return nil, err
	}
	return &Querier{db: db, iq: iq}, nil
}

// Close the querier. This invalidates all previously retrieved iterators.
func (q *Querier) Close() error {
	return q.iq.Close()
}

// Iterator returns an iterator over all chunks that match all given
// label matchers. The iterator is only valid until the Querier is closed.
func (q *Querier) Iterator(key string, matcher index.Matcher) (index.Iterator, error) {
	return q.iq.Search(key, matcher)
}

// RangeIterator returns an iterator over chunks that are present in the given time range.
// The returned iterator is only valid until the querier is closed.
func (q *Querier) RangeIterator(start, end model.Time) (index.Iterator, error) {
	return nil, nil
}

// InstantIterator returns an iterator over chunks possibly containing values for
// the given timestamp. The returned iterator is only valid until the querier is closed.
func (q *Querier) InstantIterator(at model.Time) (index.Iterator, error) {
	return nil, nil
}

func hash(m map[string]string) uint64 {
	return model.LabelsToSignature(m)
}

// Series returns a list of series iterators over all chunks in the given iterator.
// The returned series iterators are only valid until the querier is closed.
func (q *Querier) Series(it index.Iterator) ([]SeriesIterator, error) {
	mets := map[uint64]map[string]string{}
	its := map[uint64][]chunks.Chunk{}

	id, err := it.Seek(0)
	for ; err == nil; id, err = it.Next() {
		terms, err := q.iq.Doc(id)
		if err != nil {
			return nil, err
		}
		met := make(map[string]string, len(terms))
		for _, t := range terms {
			met[t.Field] = t.Val
		}
		fp := hash(met)

		chunk, err := q.chunk(ChunkID(id))
		if err != nil {
			return nil, err
		}

		its[fp] = append(its[fp], chunk)
		if _, ok := mets[fp]; ok {
			continue
		}
		mets[fp] = met
	}
	if err != io.EOF {
		return nil, err
	}

	res := make([]SeriesIterator, 0, len(its))
	for fp, chks := range its {
		res = append(res, newChunkSeriesIterator(mets[fp], chks))
	}
	return res, nil
}

func (q *Querier) chunk(id ChunkID) (chunks.Chunk, error) {
	q.db.memChunks.mtx.RLock()
	cd, ok := q.db.memChunks.chunks[id]
	q.db.memChunks.mtx.RUnlock()
	if ok {
		return cd.chunk, nil
	}

	var chk chunks.Chunk
	// TODO(fabxc): this starts a new read transaction for every
	// chunk we have to load from persistence.
	// Figure out what's best tradeoff between lock contention and
	// data consistency: start transaction when instantiating the querier
	// or lazily start transaction on first try. (Not all query operations
	// need access to persisted chunks.)
	err := q.db.persistence.view(func(tx *persistenceTx) error {
		chks := tx.ix.Bucket(bktChunks)
		ptr := chks.Get(id.bytes())
		if ptr == nil {
			return fmt.Errorf("chunk pointer for ID %d not found", id)
		}
		cdata, err := tx.chunks.Get(binary.BigEndian.Uint64(ptr))
		if err != nil {
			return fmt.Errorf("get chunk data for ID %d: %s", id, err)
		}
		chk, err = chunks.FromData(cdata)
		return err
	})
	return chk, err
}

// Metrics returns the unique metrics found across all chunks in the provided iterator.
func (q *Querier) Metrics(it index.Iterator) ([]map[string]string, error) {
	m := []map[string]string{}
	fps := map[uint64]struct{}{}

	id, err := it.Seek(0)
	for ; err == nil; id, err = it.Next() {
		terms, err := q.iq.Doc(id)
		if err != nil {
			return nil, err
		}
		met := make(map[string]string, len(terms))
		for _, t := range terms {
			met[t.Field] = t.Val
		}
		fp := hash(met)
		if _, ok := fps[fp]; ok {
			continue
		}
		fps[fp] = struct{}{}
		m = append(m, met)
	}
	if err != io.EOF {
		return nil, err
	}
	return m, nil
}
