package tsdb

import "github.com/fabxc/tsdb/chunks"

// Matcher matches a string.
type Matcher interface {
	// Match returns true if the matcher applies to the string value.
	Match(v string) bool
}

// Querier provides querying access over time series data of a fixed
// time range.
type Querier interface {
	// Range returns the timestamp range of the Querier.
	Range() (start, end int64)

	// Iterator returns an interator over the inverted index that
	// matches the key label by the constraints of Matcher.
	Iterator(key string, m Matcher) Iterator

	// Labels resolves a label reference into a set of labels.
	Labels(ref LabelRefs) (Labels, error)

	// Series returns series provided in the index iterator.
	Series(Iterator) []Series

	// LabelValues returns all potential values for a label name.
	LabelValues(string) []string
	// LabelValuesFor returns all potential values for a label name.
	// under the constraint of another label.
	LabelValuesFor(string, Label) []string

	// Close releases the resources of the Querier.
	Close() error
}

// Series represents a single time series.
type Series interface {
	Labels() (Labels, error)
	// Iterator returns a new iterator of the data of the series.
	Iterator() (SeriesIterator, error)
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
}
