package tsdb

import (
	"fmt"
	"sort"
	"strings"

	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
)

// Querier provides querying access over time series data of a fixed
// time range.
type Querier interface {
	// Select returns a set of series that matches the given label matchers.
	Select(...labels.Matcher) SeriesSet

	// LabelValues returns all potential values for a label name.
	LabelValues(string) ([]string, error)
	// LabelValuesFor returns all potential values for a label name.
	// under the constraint of another label.
	LabelValuesFor(string, labels.Label) ([]string, error)

	// Close releases the resources of the Querier.
	Close() error
}

// Series represents a single time series.
type Series interface {
	// Labels returns the complete set of labels identifying the series.
	Labels() labels.Labels

	// Iterator returns a new iterator of the data of the series.
	Iterator() SeriesIterator
}

// querier aggregates querying results from time blocks within
// a single partition.
type querier struct {
	db     *DB
	blocks []Querier
}

// Querier returns a new querier over the data partition for the given
// time range.
func (s *DB) Querier(mint, maxt int64) Querier {
	s.mtx.RLock()

	s.headmtx.RLock()
	blocks := s.blocksForInterval(mint, maxt)
	s.headmtx.RUnlock()

	sq := &querier{
		blocks: make([]Querier, 0, len(blocks)),
		db:     s,
	}
	for _, b := range blocks {
		sq.blocks = append(sq.blocks, b.Querier(mint, maxt))
	}

	return sq
}

func (q *querier) LabelValues(n string) ([]string, error) {
	if len(q.blocks) == 0 {
		return nil, nil
	}
	res, err := q.blocks[0].LabelValues(n)
	if err != nil {
		return nil, err
	}
	for _, bq := range q.blocks[1:] {
		pr, err := bq.LabelValues(n)
		if err != nil {
			return nil, err
		}
		// Merge new values into deduplicated result.
		res = mergeStrings(res, pr)
	}
	return res, nil
}

func (q *querier) LabelValuesFor(string, labels.Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *querier) Select(ms ...labels.Matcher) SeriesSet {
	// Sets from different blocks have no time overlap. The reference numbers
	// they emit point to series sorted in lexicographic order.
	// We can fully connect partial series by simply comparing with the previous
	// label set.
	if len(q.blocks) == 0 {
		return nopSeriesSet{}
	}
	r := q.blocks[0].Select(ms...)

	for _, s := range q.blocks[1:] {
		r = newMergedSeriesSet(r, s.Select(ms...))
	}
	return r
}

func (q *querier) Close() error {
	var merr MultiError

	for _, bq := range q.blocks {
		merr.Add(bq.Close())
	}
	q.db.mtx.RUnlock()

	return merr.Err()
}

// blockQuerier provides querying access to a single block database.
type blockQuerier struct {
	index  IndexReader
	chunks ChunkReader

	postingsMapper func(Postings) Postings

	mint, maxt int64
}

func (q *blockQuerier) Select(ms ...labels.Matcher) SeriesSet {
	var (
		its    []Postings
		absent []string
	)
	for _, m := range ms {
		// If the matcher checks absence of a label, don't select them
		// but propagate the check into the series set.
		if _, ok := m.(*labels.EqualMatcher); ok && m.Matches("") {
			absent = append(absent, m.Name())
			continue
		}
		its = append(its, q.selectSingle(m))
	}

	p := Intersect(its...)

	if q.postingsMapper != nil {
		p = q.postingsMapper(p)
	}

	return &blockSeriesSet{
		set: &populatedChunkSeries{
			set: &baseChunkSeries{
				p:      p,
				index:  q.index,
				absent: absent,
			},
			chunks: q.chunks,
			mint:   q.mint,
			maxt:   q.maxt,
		},
	}
}

func (q *blockQuerier) selectSingle(m labels.Matcher) Postings {
	// Fast-path for equal matching.
	if em, ok := m.(*labels.EqualMatcher); ok {
		it, err := q.index.Postings(em.Name(), em.Value())
		if err != nil {
			return errPostings{err: err}
		}
		return it
	}

	tpls, err := q.index.LabelValues(m.Name())
	if err != nil {
		return errPostings{err: err}
	}
	// TODO(fabxc): use interface upgrading to provide fast solution
	// for equality and prefix matches. Tuples are lexicographically sorted.
	var res []string

	for i := 0; i < tpls.Len(); i++ {
		vals, err := tpls.At(i)
		if err != nil {
			return errPostings{err: err}
		}
		if m.Matches(vals[0]) {
			res = append(res, vals[0])
		}
	}
	if len(res) == 0 {
		return emptyPostings
	}

	var rit []Postings

	for _, v := range res {
		it, err := q.index.Postings(m.Name(), v)
		if err != nil {
			return errPostings{err: err}
		}
		rit = append(rit, it)
	}

	return Merge(rit...)
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
	return res, nil
}

func (q *blockQuerier) LabelValuesFor(string, labels.Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *blockQuerier) Close() error {
	return nil
}

func mergeStrings(a, b []string) []string {
	maxl := len(a)
	if len(b) > len(a) {
		maxl = len(b)
	}
	res := make([]string, 0, maxl*10/9)

	for len(a) > 0 && len(b) > 0 {
		d := strings.Compare(a[0], b[0])

		if d == 0 {
			res = append(res, a[0])
			a, b = a[1:], b[1:]
		} else if d < 0 {
			res = append(res, a[0])
			a = a[1:]
		} else if d > 0 {
			res = append(res, b[0])
			b = b[1:]
		}
	}

	// Append all remaining elements.
	res = append(res, a...)
	res = append(res, b...)
	return res
}

// SeriesSet contains a set of series.
type SeriesSet interface {
	Next() bool
	At() Series
	Err() error
}

type nopSeriesSet struct{}

func (nopSeriesSet) Next() bool { return false }
func (nopSeriesSet) At() Series { return nil }
func (nopSeriesSet) Err() error { return nil }

// mergedSeriesSet takes two series sets as a single series set. The input series sets
// must be sorted and sequential in time, i.e. if they have the same label set,
// the datapoints of a must be before the datapoints of b.
type mergedSeriesSet struct {
	a, b SeriesSet

	cur          Series
	adone, bdone bool
}

func newMergedSeriesSet(a, b SeriesSet) *mergedSeriesSet {
	s := &mergedSeriesSet{a: a, b: b}
	// Initialize first elements of both sets as Next() needs
	// one element look-ahead.
	s.adone = !s.a.Next()
	s.bdone = !s.b.Next()

	return s
}

func (s *mergedSeriesSet) At() Series {
	return s.cur
}

func (s *mergedSeriesSet) Err() error {
	if s.a.Err() != nil {
		return s.a.Err()
	}
	return s.b.Err()
}

func (s *mergedSeriesSet) compare() int {
	if s.adone {
		return 1
	}
	if s.bdone {
		return -1
	}
	return labels.Compare(s.a.At().Labels(), s.b.At().Labels())
}

func (s *mergedSeriesSet) Next() bool {
	if s.adone && s.bdone || s.Err() != nil {
		return false
	}

	d := s.compare()

	// Both sets contain the current series. Chain them into a single one.
	if d > 0 {
		s.cur = s.b.At()
		s.bdone = !s.b.Next()
	} else if d < 0 {
		s.cur = s.a.At()
		s.adone = !s.a.Next()
	} else {
		s.cur = &chainedSeries{series: []Series{s.a.At(), s.b.At()}}
		s.adone = !s.a.Next()
		s.bdone = !s.b.Next()
	}
	return true
}

type chunkSeriesSet interface {
	Next() bool
	At() (labels.Labels, []*ChunkMeta)
	Err() error
}

// baseChunkSeries loads the label set and chunk references for a postings
// list from an index. It filters out series that have labels set that should be unset.
type baseChunkSeries struct {
	p      Postings
	index  IndexReader
	absent []string // labels that must be unset in results.

	lset labels.Labels
	chks []*ChunkMeta
	err  error
}

func (s *baseChunkSeries) At() (labels.Labels, []*ChunkMeta) { return s.lset, s.chks }
func (s *baseChunkSeries) Err() error                        { return s.err }

func (s *baseChunkSeries) Next() bool {
Outer:
	for s.p.Next() {
		lset, chunks, err := s.index.Series(s.p.At())
		if err != nil {
			s.err = err
			return false
		}

		// If a series contains a label that must be absent, it is skipped as well.
		for _, abs := range s.absent {
			if lset.Get(abs) != "" {
				continue Outer
			}
		}

		s.lset = lset
		s.chks = chunks

		return true
	}
	if err := s.p.Err(); err != nil {
		s.err = err
	}
	return false
}

// populatedChunkSeries loads chunk data from a store for a set of series
// with known chunk references. It filters out chunks that do not fit the
// given time range.
type populatedChunkSeries struct {
	set        chunkSeriesSet
	chunks     ChunkReader
	mint, maxt int64

	err  error
	chks []*ChunkMeta
	lset labels.Labels
}

func (s *populatedChunkSeries) At() (labels.Labels, []*ChunkMeta) { return s.lset, s.chks }
func (s *populatedChunkSeries) Err() error                        { return s.err }

func (s *populatedChunkSeries) Next() bool {
	for s.set.Next() {
		lset, chks := s.set.At()

		for i, c := range chks {
			if c.MaxTime < s.mint {
				chks = chks[1:]
				continue
			}
			if c.MinTime > s.maxt {
				chks = chks[:i]
				break
			}
			c.Chunk, s.err = s.chunks.Chunk(c.Ref)
			if s.err != nil {
				return false
			}
		}
		if len(chks) == 0 {
			continue
		}

		s.lset = lset
		s.chks = chks

		return true
	}
	if err := s.set.Err(); err != nil {
		s.err = err
	}
	return false
}

// blockSeriesSet is a set of series from an inverted index query.
type blockSeriesSet struct {
	set chunkSeriesSet
	err error
	cur Series
}

func (s *blockSeriesSet) Next() bool {
	for s.set.Next() {
		lset, chunks := s.set.At()
		s.cur = &chunkSeries{labels: lset, chunks: chunks}
		return true
	}
	if s.set.Err() != nil {
		s.err = s.set.Err()
	}
	return false
}

func (s *blockSeriesSet) At() Series { return s.cur }
func (s *blockSeriesSet) Err() error { return s.err }

// chunkSeries is a series that is backed by a sequence of chunks holding
// time series data.
type chunkSeries struct {
	labels labels.Labels
	chunks []*ChunkMeta // in-order chunk refs
}

func (s *chunkSeries) Labels() labels.Labels {
	return s.labels
}

func (s *chunkSeries) Iterator() SeriesIterator {
	return newChunkSeriesIterator(s.chunks)
}

// SeriesIterator iterates over the data of a time series.
type SeriesIterator interface {
	// Seek advances the iterator forward to the given timestamp.
	// If there's no value exactly at ts, it advances to the last value
	// before tt.
	Seek(t int64) bool
	// At returns the current timestamp/value pair.
	At() (t int64, v float64)
	// Next advances the iterator by one.
	Next() bool
	// Err returns the current error.
	Err() error
}

// chainedSeries implements a series for a list of time-sorted series.
// They all must have the same labels.
type chainedSeries struct {
	series []Series
}

func (s *chainedSeries) Labels() labels.Labels {
	return s.series[0].Labels()
}

func (s *chainedSeries) Iterator() SeriesIterator {
	return &chainedSeriesIterator{series: s.series}
}

// chainedSeriesIterator implements a series iterater over a list
// of time-sorted, non-overlapping iterators.
type chainedSeriesIterator struct {
	series []Series // series in time order

	i   int
	cur SeriesIterator
}

func (it *chainedSeriesIterator) Seek(t int64) bool {
	// We just scan the chained series sequentially as they are already
	// pre-selected by relevant time and should be accessed sequentially anyway.
	for i, s := range it.series[it.i:] {
		cur := s.Iterator()
		if !cur.Seek(t) {
			continue
		}
		it.cur = cur
		it.i += i
		return true
	}
	return false
}

func (it *chainedSeriesIterator) Next() bool {
	if it.cur == nil {
		it.cur = it.series[it.i].Iterator()
	}
	if it.cur.Next() {
		return true
	}
	if err := it.cur.Err(); err != nil {
		return false
	}
	if it.i == len(it.series)-1 {
		return false
	}

	it.i++
	it.cur = it.series[it.i].Iterator()

	return it.Next()
}

func (it *chainedSeriesIterator) At() (t int64, v float64) {
	return it.cur.At()
}

func (it *chainedSeriesIterator) Err() error {
	return it.cur.Err()
}

// chunkSeriesIterator implements a series iterator on top
// of a list of time-sorted, non-overlapping chunks.
type chunkSeriesIterator struct {
	chunks []*ChunkMeta

	i   int
	cur chunks.Iterator
}

func newChunkSeriesIterator(cs []*ChunkMeta) *chunkSeriesIterator {
	return &chunkSeriesIterator{
		chunks: cs,
		i:      0,
		cur:    cs[0].Chunk.Iterator(),
	}
}

func (it *chunkSeriesIterator) Seek(t int64) (ok bool) {
	// Only do binary search forward to stay in line with other iterators
	// that can only move forward.
	x := sort.Search(len(it.chunks[it.i:]), func(i int) bool { return it.chunks[i].MinTime >= t })
	x += it.i

	// If the timestamp was not found, it might be in the last chunk.
	if x == len(it.chunks) {
		x--
	}
	// Go to previous chunk if the chunk doesn't exactly start with t.
	// If we are already at the first chunk, we use it as it's the best we have.
	if x > 0 && it.chunks[x].MinTime > t {
		x--
	}

	it.i = x
	it.cur = it.chunks[x].Chunk.Iterator()

	for it.cur.Next() {
		t0, _ := it.cur.At()
		if t0 >= t {
			return true
		}
	}
	return false
}

func (it *chunkSeriesIterator) At() (t int64, v float64) {
	return it.cur.At()
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
	it.cur = it.chunks[it.i].Chunk.Iterator()

	return it.Next()
}

func (it *chunkSeriesIterator) Err() error {
	return it.cur.Err()
}

type mockSeriesSet struct {
	next   func() bool
	series func() Series
	err    func() error
}

func (m *mockSeriesSet) Next() bool { return m.next() }
func (m *mockSeriesSet) At() Series { return m.series() }
func (m *mockSeriesSet) Err() error { return m.err() }

func newListSeriesSet(list []Series) *mockSeriesSet {
	i := -1
	return &mockSeriesSet{
		next: func() bool {
			i++
			return i < len(list)
		},
		series: func() Series {
			return list[i]
		},
		err: func() error { return nil },
	}
}

type errSeriesSet struct {
	err error
}

func (s errSeriesSet) Next() bool { return false }
func (s errSeriesSet) At() Series { return nil }
func (s errSeriesSet) Err() error { return s.err }
