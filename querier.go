package tsdb

import (
	"fmt"

	"github.com/fabxc/tsdb/chunks"
)

// Matcher matches a string.
type Matcher interface {
	// Match returns true if the matcher applies to the string value.
	Match(v string) bool
}

// Querier provides querying access over time series data of a fixed
// time range.
type Querier interface {
	// Iterator returns an interator over the inverted index that
	// matches the key label by the constraints of Matcher.
	Iterator(key string, m Matcher) Iterator

	// Series returns series provided in the index iterator.
	Series(Iterator) ([]Series, error)

	// LabelValues returns all potential values for a label name.
	LabelValues(string) ([]string, error)
	// LabelValuesFor returns all potential values for a label name.
	// under the constraint of another label.
	LabelValuesFor(string, Label) ([]string, error)

	// Close releases the resources of the Querier.
	Close() error
}

func example(db *DB) error {
	var m1, m2, m3, m4 Matcher

	q := db.Querier(0, 1000)

	series, err := q.Series(
		Merge(
			Intersect(q.Iterator("name", m1), q.Iterator("job", m2)),
			Intersect(q.Iterator("name", m3), q.Iterator("job", m4)),
		),
	)
	if err != nil {
		return err
	}
	for _, s := range series {
		s.Iterator()
	}
	return nil
}

// Series represents a single time series.
type Series interface {
	// Labels returns the complete set of labels identifying the series.
	Labels() Labels
	// Iterator returns a new iterator of the data of the series.
	Iterator() SeriesIterator
}

func inRange(x, mint, maxt int64) bool {
	return x >= mint && x <= maxt
}

// querier merges query results from a set of shard querieres.
type querier struct {
	mint, maxt int64
	shards     []Querier
}

// Querier returns a new querier over the database for the given
// time range.
func (db *DB) Querier(mint, maxt int64) Querier {
	q := &querier{
		mint: mint,
		maxt: maxt,
	}
	for _, s := range db.shards {
		q.shards = append(q.shards, s.Querier(mint, maxt))
	}

	return q
}

// SeriesSet contains a set of series.
type SeriesSet interface {
}

func (q *querier) Select(key string, m Matcher) SeriesSet {
	return nil
}

func (q *querier) Iterator(key string, m Matcher) Iterator {
	return nil
}

func (q *querier) Series(Iterator) ([]Series, error) {
	return nil, nil
}

func (q *querier) LabelValues(string) ([]string, error) {
	return nil, nil
}

func (q *querier) LabelValuesFor(string, Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *querier) Close() error {
	return nil
}

// shardQuerier aggregates querying results from time blocks within
// a single shard.
type shardQuerier struct {
	blocks []Querier
}

// Querier returns a new querier over the data shard for the given
// time range.
func (s *SeriesShard) Querier(mint, maxt int64) Querier {
	blocks := s.blocksForRange(mint, maxt)

	sq := &shardQuerier{
		blocks: make([]Querier, 0, len(blocks)),
	}
	for _, b := range blocks {
		sq.blocks = append(sq.blocks, b.Querier(mint, maxt))
	}

	return sq
}

func (q *shardQuerier) Iterator(name string, m Matcher) Iterator {
	// Iterators from different blocks have no time overlap. The reference numbers
	// they emit point to series sorted in lexicographic order.
	// If actually retrieving an iterator result via the Series method, we can fully
	// deduplicate series by simply comparing with the previous label set.
	var rit Iterator

	for _, s := range q.blocks {
		rit = Merge(rit, s.Iterator(name, m))
	}

	return rit
}

func (q *shardQuerier) Series(it Iterator) ([]Series, error) {
	// Dedulicate series as we stream through the iterator. See comment
	// on the Iterator method.

	var series []Series
	// var prev Labels

	// for it.Next() {
	// 	s, err := q.index.Series(it.Value())
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	series = append(series, s)
	// }
	// if it.Err() != nil {
	// 	return nil, it.Err()
	// }

	return series, nil
}

func (q *shardQuerier) LabelValues(string) ([]string, error) {
	return nil, nil
}

func (q *shardQuerier) LabelValuesFor(string, Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *shardQuerier) Close() error {
	return nil
}

// blockQuerier provides querying access to a single block database.
type blockQuerier struct {
	mint, maxt int64

	index  IndexReader
	series SeriesReader
}

func newBlockQuerier(ix IndexReader, s SeriesReader, mint, maxt int64) *blockQuerier {
	return &blockQuerier{
		mint:   mint,
		maxt:   maxt,
		index:  ix,
		series: s,
	}
}

func (q *blockQuerier) Iterator(name string, m Matcher) Iterator {
	tpls, err := q.index.LabelValues(name)
	if err != nil {
		return errIterator{err: err}
	}
	// TODO(fabxc): use interface upgrading to provide fast solution
	// for equality and prefix matches. Tuples are lexicographically sorted.
	var res []string

	for i := 0; i < tpls.Len(); i++ {
		vals, err := tpls.At(i)
		if err != nil {
			return errIterator{err: err}
		}
		if m.Match(vals[0]) {
			res = append(res, vals[0])
		}
	}

	var rit Iterator

	for _, v := range res {
		it, err := q.index.Postings(name, v)
		if err != nil {
			return errIterator{err: err}
		}
		rit = Intersect(rit, it)
	}

	return rit
}

func (q *blockQuerier) Series(it Iterator) ([]Series, error) {
	var series []Series

	for it.Next() {
		s, err := q.index.Series(it.Value())
		if err != nil {
			return nil, err
		}
		series = append(series, s)
	}
	if it.Err() != nil {
		return nil, it.Err()
	}

	return series, nil
}

func (q *blockQuerier) LabelValues(name string) ([]string, error) {
	tpls, err := q.index.LabelValues(name)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0, tpls.Len())

	for i := 0; i < tpls.Len(); i++ {
		vals, err := tpls.At(i)
		if err != nil {
			return nil, err
		}
		res = append(res, vals[0])
	}
	return nil, nil
}

func (q *blockQuerier) LabelValuesFor(string, Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *blockQuerier) Close() error {
	return nil
}

// SeriesIterator iterates over the data of a time series.
type SeriesIterator interface {
	// Seek advances the iterator forward to the given timestamp.
	// If there's no value exactly at ts, it advances to the last value
	// before tt.
	Seek(t int64) bool
	// Values returns the current timestamp/value pair.
	Values() (t int64, v float64)
	// Next advances the iterator by one.
	Next() bool
	// Err returns the current error.
	Err() error
}

// chunkSeriesIterator implements a series iterator on top
// of a list of time-sorted, non-overlapping chunks.
type chunkSeriesIterator struct {
	// minTimes []int64
	chunks []chunks.Chunk

	i   int
	cur chunks.Iterator
	err error
}

func newChunkSeriesIterator(cs []chunks.Chunk) *chunkSeriesIterator {
	return &chunkSeriesIterator{
		chunks: cs,
		i:      0,
		cur:    cs[0].Iterator(),
	}
}

func (it *chunkSeriesIterator) Seek(t int64) (ok bool) {
	// TODO(fabxc): skip to relevant chunk.
	for it.Next() {
		if ts, _ := it.Values(); ts >= t {
			return true
		}
	}
	return false
}

func (it *chunkSeriesIterator) Values() (t int64, v float64) {
	return it.cur.Values()
}

func (it *chunkSeriesIterator) Next() bool {
	if it.cur.Next() {
		return true
	}
	if err := it.cur.Err(); err != nil {
		return false
	}
	if it.i == len(it.chunks)-1 {
		return false
	}

	it.i++
	it.cur = it.chunks[it.i].Iterator()

	return it.Next()
}

func (it *chunkSeriesIterator) Err() error {
	return it.cur.Err()
}

type bufferedSeriesIterator struct {
	// TODO(fabxc): time-based look back buffer for time-aggregating
	// queries such as rate. It should allow us to re-use an iterator
	// within a range query while calculating time-aggregates at any point.
	//
	// It also allows looking up/seeking at-or-before without modifying
	// the simpler interface.
	//
	// Consider making this the main external interface.
	SeriesIterator

	buf []sample // lookback buffer
	i   int      // current head
}

type sample struct {
	t int64
	v float64
}

func (b *bufferedSeriesIterator) PeekBack(i int) (t int64, v float64, ok bool) {
	return 0, 0, false
}
