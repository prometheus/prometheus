package tsdb

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
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

	blocks := s.blocksForInterval(mint, maxt)

	sq := &querier{
		blocks: make([]Querier, 0, len(blocks)),
		db:     s,
	}

	for _, b := range blocks {
		q := &blockQuerier{
			mint:   mint,
			maxt:   maxt,
			index:  b.Index(),
			chunks: b.Chunks(),
		}

		// TODO(fabxc): find nicer solution.
		if hb, ok := b.(*headBlock); ok {
			q.postingsMapper = hb.remapPostings
		}

		sq.blocks = append(sq.blocks, q)
	}

	return sq
}

func (q *querier) LabelValues(n string) ([]string, error) {
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
		r = newPartitionSeriesSet(r, s.Select(ms...))
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

func newBlockQuerier(ix IndexReader, c ChunkReader, mint, maxt int64) *blockQuerier {
	return &blockQuerier{
		mint:   mint,
		maxt:   maxt,
		index:  ix,
		chunks: c,
	}
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
		index:  q.index,
		chunks: q.chunks,
		it:     p,
		absent: absent,
		mint:   q.mint,
		maxt:   q.maxt,
	}
}

func (q *blockQuerier) selectSingle(m labels.Matcher) Postings {
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

// partitionedQuerier merges query results from a set of partition querieres.
type partitionedQuerier struct {
	mint, maxt int64
	partitions []Querier
}

// Querier returns a new querier over the database for the given
// time range.
func (db *PartitionedDB) Querier(mint, maxt int64) Querier {
	q := &partitionedQuerier{
		mint: mint,
		maxt: maxt,
	}
	for _, s := range db.Partitions {
		q.partitions = append(q.partitions, s.Querier(mint, maxt))
	}

	return q
}

func (q *partitionedQuerier) Select(ms ...labels.Matcher) SeriesSet {
	// We gather the non-overlapping series from every partition and simply
	// return their union.
	r := &mergedSeriesSet{}

	for _, s := range q.partitions {
		r.sets = append(r.sets, s.Select(ms...))
	}
	if len(r.sets) == 0 {
		return nopSeriesSet{}
	}
	return r
}

func (q *partitionedQuerier) LabelValues(n string) ([]string, error) {
	res, err := q.partitions[0].LabelValues(n)
	if err != nil {
		return nil, err
	}
	for _, sq := range q.partitions[1:] {
		pr, err := sq.LabelValues(n)
		if err != nil {
			return nil, err
		}
		// Merge new values into deduplicated result.
		res = mergeStrings(res, pr)
	}
	return res, nil
}

func (q *partitionedQuerier) LabelValuesFor(string, labels.Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *partitionedQuerier) Close() error {
	var merr MultiError

	for _, sq := range q.partitions {
		merr.Add(sq.Close())
	}
	return merr.Err()
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

type mergedSeriesSet struct {
	sets []SeriesSet

	cur int
	err error
}

func (s *mergedSeriesSet) At() Series { return s.sets[s.cur].At() }
func (s *mergedSeriesSet) Err() error { return s.sets[s.cur].Err() }

func (s *mergedSeriesSet) Next() bool {
	// TODO(fabxc): We just emit the sets one after one. They are each
	// lexicographically sorted. Should we emit their union sorted too?
	if s.sets[s.cur].Next() {
		return true
	}

	if s.cur == len(s.sets)-1 {
		return false
	}
	s.cur++

	return s.Next()
}

type partitionSeriesSet struct {
	a, b SeriesSet

	cur          Series
	adone, bdone bool
}

func newPartitionSeriesSet(a, b SeriesSet) *partitionSeriesSet {
	s := &partitionSeriesSet{a: a, b: b}
	// Initialize first elements of both sets as Next() needs
	// one element look-ahead.
	s.adone = !s.a.Next()
	s.bdone = !s.b.Next()

	return s
}

func (s *partitionSeriesSet) At() Series {
	return s.cur
}

func (s *partitionSeriesSet) Err() error {
	if s.a.Err() != nil {
		return s.a.Err()
	}
	return s.b.Err()
}

func (s *partitionSeriesSet) compare() int {
	if s.adone {
		return 1
	}
	if s.bdone {
		return -1
	}
	return labels.Compare(s.a.At().Labels(), s.b.At().Labels())
}

func (s *partitionSeriesSet) Next() bool {
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

// blockSeriesSet is a set of series from an inverted index query.
type blockSeriesSet struct {
	index      IndexReader
	chunks     ChunkReader
	it         Postings // postings list referencing series
	absent     []string // labels that must not be set for result series
	mint, maxt int64    // considered time range

	err error
	cur Series
}

func (s *blockSeriesSet) Next() bool {
	// Step through the postings iterator to find potential series.
outer:
	for s.it.Next() {
		lset, chunks, err := s.index.Series(s.it.At())
		if err != nil {
			s.err = err
			return false
		}

		// If a series contains a label that must be absent, it is skipped as well.
		for _, abs := range s.absent {
			if lset.Get(abs) != "" {
				continue outer
			}
		}

		ser := &chunkSeries{
			labels: lset,
			chunks: make([]ChunkMeta, 0, len(chunks)),
			chunk:  s.chunks.Chunk,
		}
		// Only use chunks that fit the time range.
		for _, c := range chunks {
			if c.MaxTime < s.mint {
				continue
			}
			if c.MinTime > s.maxt {
				break
			}
			ser.chunks = append(ser.chunks, c)
		}
		// If no chunks of the series apply to the time range, skip it.
		if len(ser.chunks) == 0 {
			continue
		}

		s.cur = ser
		return true
	}
	if s.it.Err() != nil {
		s.err = s.it.Err()
	}
	return false
}

func (s *blockSeriesSet) At() Series { return s.cur }
func (s *blockSeriesSet) Err() error { return s.err }

// chunkSeries is a series that is backed by a sequence of chunks holding
// time series data.
type chunkSeries struct {
	labels labels.Labels
	chunks []ChunkMeta // in-order chunk refs

	// chunk is a function that retrieves chunks based on a reference
	// number contained in the chunk meta information.
	chunk func(ref uint64) (chunks.Chunk, error)
}

func (s *chunkSeries) Labels() labels.Labels {
	return s.labels
}

func (s *chunkSeries) Iterator() SeriesIterator {
	var cs []chunks.Chunk
	var mints []int64

	for _, co := range s.chunks {
		c, err := s.chunk(co.Ref)
		if err != nil {
			panic(err) // TODO(fabxc): add error series iterator.
		}
		cs = append(cs, c)
		mints = append(mints, co.MinTime)
	}

	// TODO(fabxc): consider pushing chunk retrieval further down. In practice, we
	// probably have to touch all chunks anyway and it doesn't matter.
	return newChunkSeriesIterator(mints, cs)
}

// SeriesIterator iterates over the data of a time series.
type SeriesIterator interface {
	// Seek advances the iterator forward to the given timestamp.
	// If there's no value exactly at ts, it advances to the last value
	// before tt.
	Seek(t int64) bool
	// Values returns the current timestamp/value pair.
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
	mints  []int64 // minimum timestamps for each iterator
	chunks []chunks.Chunk

	i   int
	cur chunks.Iterator
}

func newChunkSeriesIterator(mints []int64, cs []chunks.Chunk) *chunkSeriesIterator {
	if len(mints) != len(cs) {
		panic("chunk references and chunks length don't match")
	}
	return &chunkSeriesIterator{
		mints:  mints,
		chunks: cs,
		i:      0,
		cur:    cs[0].Iterator(),
	}
}

func (it *chunkSeriesIterator) Seek(t int64) (ok bool) {
	// Only do binary search forward to stay in line with other iterators
	// that can only move forward.
	x := sort.Search(len(it.mints[it.i:]), func(i int) bool { return it.mints[i] >= t })
	x += it.i

	// If the timestamp was not found, it might be in the last chunk.
	if x == len(it.mints) {
		x--
	}
	// Go to previous chunk if the chunk doesn't exactly start with t.
	// If we are already at the first chunk, we use it as it's the best we have.
	if x > 0 && it.mints[x] > t {
		x--
	}

	it.i = x
	it.cur = it.chunks[x].Iterator()

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
	it.cur = it.chunks[it.i].Iterator()

	return it.Next()
}

func (it *chunkSeriesIterator) Err() error {
	return it.cur.Err()
}

// BufferedSeriesIterator wraps an iterator with a look-back buffer.
type BufferedSeriesIterator struct {
	it  SeriesIterator
	buf *sampleRing

	lastTime int64
}

// NewBuffer returns a new iterator that buffers the values within the time range
// of the current element and the duration of delta before.
func NewBuffer(it SeriesIterator, delta int64) *BufferedSeriesIterator {
	return &BufferedSeriesIterator{
		it:       it,
		buf:      newSampleRing(delta, 16),
		lastTime: math.MinInt64,
	}
}

// PeekBack returns the previous element of the iterator. If there is none buffered,
// ok is false.
func (b *BufferedSeriesIterator) PeekBack() (t int64, v float64, ok bool) {
	return b.buf.last()
}

// Buffer returns an iterator over the buffered data.
func (b *BufferedSeriesIterator) Buffer() SeriesIterator {
	return b.buf.iterator()
}

// Seek advances the iterator to the element at time t or greater.
func (b *BufferedSeriesIterator) Seek(t int64) bool {
	t0 := t - b.buf.delta

	// If the delta would cause us to seek backwards, preserve the buffer
	// and just continue regular advancment while filling the buffer on the way.
	if t0 > b.lastTime {
		b.buf.reset()

		ok := b.it.Seek(t0)
		if !ok {
			return false
		}
		b.lastTime, _ = b.At()
	}

	if b.lastTime >= t {
		return true
	}
	for b.Next() {
		if b.lastTime >= t {
			return true
		}
	}

	return false
}

// Next advances the iterator to the next element.
func (b *BufferedSeriesIterator) Next() bool {
	// Add current element to buffer before advancing.
	b.buf.add(b.it.At())

	ok := b.it.Next()
	if ok {
		b.lastTime, _ = b.At()
	}
	return ok
}

// Values returns the current element of the iterator.
func (b *BufferedSeriesIterator) At() (int64, float64) {
	return b.it.At()
}

// Err returns the last encountered error.
func (b *BufferedSeriesIterator) Err() error {
	return b.it.Err()
}

type sample struct {
	t int64
	v float64
}

type sampleRing struct {
	delta int64

	buf []sample // lookback buffer
	i   int      // position of most recent element in ring buffer
	f   int      // position of first element in ring buffer
	l   int      // number of elements in buffer
}

func newSampleRing(delta int64, sz int) *sampleRing {
	r := &sampleRing{delta: delta, buf: make([]sample, sz)}
	r.reset()

	return r
}

func (r *sampleRing) reset() {
	r.l = 0
	r.i = -1
	r.f = 0
}

func (r *sampleRing) iterator() SeriesIterator {
	return &sampleRingIterator{r: r, i: -1}
}

type sampleRingIterator struct {
	r *sampleRing
	i int
}

func (it *sampleRingIterator) Next() bool {
	it.i++
	return it.i < it.r.l
}

func (it *sampleRingIterator) Seek(int64) bool {
	return false
}

func (it *sampleRingIterator) Err() error {
	return nil
}

func (it *sampleRingIterator) At() (int64, float64) {
	return it.r.at(it.i)
}

func (r *sampleRing) at(i int) (int64, float64) {
	j := (r.f + i) % len(r.buf)
	s := r.buf[j]
	return s.t, s.v
}

// add adds a sample to the ring buffer and frees all samples that fall
// out of the delta range.
func (r *sampleRing) add(t int64, v float64) {
	l := len(r.buf)
	// Grow the ring buffer if it fits no more elements.
	if l == r.l {
		buf := make([]sample, 2*l)
		copy(buf[l+r.f:], r.buf[r.f:])
		copy(buf, r.buf[:r.f])

		r.buf = buf
		r.i = r.f
		r.f += l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	r.buf[r.i] = sample{t: t, v: v}
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	for r.buf[r.f].t < t-r.delta {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
}

// last returns the most recent element added to the ring.
func (r *sampleRing) last() (int64, float64, bool) {
	if r.l == 0 {
		return 0, 0, false
	}
	s := r.buf[r.i]
	return s.t, s.v, true
}

func (r *sampleRing) samples() []sample {
	res := make([]sample, r.l)

	var k = r.f + r.l
	var j int
	if k > len(r.buf) {
		k = len(r.buf)
		j = r.l - k + r.f
	}

	n := copy(res, r.buf[r.f:k])
	copy(res[n:], r.buf[:j])

	return res
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
