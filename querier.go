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

	// Ref() uint32
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

func (q *querier) Select(ms ...labels.Matcher) SeriesSet {
	// We gather the non-overlapping series from every shard and simply
	// return their union.
	r := &mergedSeriesSet{}

	for _, s := range q.shards {
		r.sets = append(r.sets, s.Select(ms...))
	}
	if len(r.sets) == 0 {
		return nopSeriesSet{}
	}
	return r
}

func (q *querier) LabelValues(n string) ([]string, error) {
	// TODO(fabxc): return returned merged result.
	res, err := q.shards[0].LabelValues(n)
	if err != nil {
		return nil, err
	}
	for _, sq := range q.shards[1:] {
		pr, err := sq.LabelValues(n)
		if err != nil {
			return nil, err
		}
		// Merge new values into deduplicated result.
		res = mergeStrings(res, pr)
	}
	return res, nil
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

func (q *querier) LabelValuesFor(string, labels.Label) ([]string, error) {
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
func (s *Shard) Querier(mint, maxt int64) Querier {
	blocks := s.blocksForInterval(mint, maxt)

	sq := &shardQuerier{
		blocks: make([]Querier, 0, len(blocks)),
	}
	for _, b := range blocks {
		sq.blocks = append(sq.blocks, b.Querier(mint, maxt))
	}

	return sq
}

func (q *shardQuerier) LabelValues(n string) ([]string, error) {
	// TODO(fabxc): return returned merged result.
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

func (q *shardQuerier) LabelValuesFor(string, labels.Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *shardQuerier) Select(ms ...labels.Matcher) SeriesSet {
	// Sets from different blocks have no time overlap. The reference numbers
	// they emit point to series sorted in lexicographic order.
	// We can fully connect partial series by simply comparing with the previous
	// label set.
	if len(q.blocks) == 0 {
		return nopSeriesSet{}
	}
	r := q.blocks[0].Select(ms...)

	for _, s := range q.blocks[1:] {
		r = newShardSeriesSet(r, s.Select(ms...))
	}
	return r
}

func (q *shardQuerier) Close() error {
	return nil
}

// blockQuerier provides querying access to a single block database.
type blockQuerier struct {
	index  IndexReader
	series SeriesReader

	mint, maxt int64
}

func newBlockQuerier(ix IndexReader, s SeriesReader, mint, maxt int64) *blockQuerier {
	return &blockQuerier{
		mint:   mint,
		maxt:   maxt,
		index:  ix,
		series: s,
	}
}

func (q *blockQuerier) Select(ms ...labels.Matcher) SeriesSet {
	var its []Postings
	for _, m := range ms {
		its = append(its, q.selectSingle(m))
	}

	return &blockSeriesSet{
		index: q.index,
		it:    Intersect(its...),
		mint:  q.mint,
		maxt:  q.maxt,
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
		return errPostings{err: nil}
	}

	var rit []Postings

	for _, v := range res {
		it, err := q.index.Postings(m.Name(), v)
		if err != nil {
			return errPostings{err: err}
		}
		rit = append(rit, it)
	}

	return Intersect(rit...)
}

func expandPostings(p Postings) (res []uint32, err error) {
	for p.Next() {
		res = append(res, p.Value())
	}
	return res, p.Err()
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

// SeriesSet contains a set of series.
type SeriesSet interface {
	Next() bool
	Series() Series
	Err() error
}

type nopSeriesSet struct{}

func (nopSeriesSet) Next() bool     { return false }
func (nopSeriesSet) Series() Series { return nil }
func (nopSeriesSet) Err() error     { return nil }

type mergedSeriesSet struct {
	sets []SeriesSet

	cur int
	err error
}

func (s *mergedSeriesSet) Series() Series { return s.sets[s.cur].Series() }
func (s *mergedSeriesSet) Err() error     { return s.sets[s.cur].Err() }

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

type shardSeriesSet struct {
	a, b SeriesSet

	cur    Series
	as, bs Series // peek ahead of each set
}

func newShardSeriesSet(a, b SeriesSet) *shardSeriesSet {
	s := &shardSeriesSet{a: a, b: b}
	// Initialize first elements of both sets as Next() needs
	// one element look-ahead.
	s.advanceA()
	s.advanceB()

	return s
}

// compareLabels compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func compareLabels(a, b labels.Labels) int {
	l := len(a)
	if len(b) < l {
		l = len(b)
	}

	for i := 0; i < l; i++ {
		if d := strings.Compare(a[i].Name, b[i].Name); d != 0 {
			return d
		}
		if d := strings.Compare(a[i].Value, b[i].Value); d != 0 {
			return d
		}
	}
	// If all labels so far were in common, the set with fewer labels comes first.
	return len(a) - len(b)
}

func (s *shardSeriesSet) Series() Series {
	return s.cur
}

func (s *shardSeriesSet) Err() error {
	if s.a.Err() != nil {
		return s.a.Err()
	}
	return s.b.Err()
}

func (s *shardSeriesSet) compare() int {
	if s.as == nil {
		return 1
	}
	if s.bs == nil {
		return -1
	}
	return compareLabels(s.as.Labels(), s.bs.Labels())
}

func (s *shardSeriesSet) advanceA() {
	if s.a.Next() {
		s.as = s.a.Series()
	} else {
		s.as = nil
	}
}

func (s *shardSeriesSet) advanceB() {
	if s.b.Next() {
		s.bs = s.b.Series()
	} else {
		s.bs = nil
	}
}

func (s *shardSeriesSet) Next() bool {
	if s.as == nil && s.bs == nil || s.Err() != nil {
		return false
	}

	d := s.compare()
	// Both sets contain the current series. Chain them into a single one.
	if d > 0 {
		s.cur = s.bs
		s.advanceB()

	} else if d < 0 {
		s.cur = s.as
		s.advanceA()

	} else {
		s.cur = &chainedSeries{series: []Series{s.as, s.bs}}
		s.advanceA()
		s.advanceB()
	}
	return true
}

// blockSeriesSet is a set of series from an inverted index query.
type blockSeriesSet struct {
	index      IndexReader
	it         Postings
	mint, maxt int64

	err error
	cur Series
}

func (s *blockSeriesSet) Next() bool {
	// Step through the postings iterator to find potential series.
	// Resolving series may return nil if no applicable data for the
	// time range exists and we can skip to the next series.
	for s.it.Next() {
		series, err := s.index.Series(s.it.Value(), s.mint, s.maxt)
		if err != nil {
			s.err = err
			return false
		}
		if series != nil {
			s.cur = series
			return true
		}
	}
	if s.it.Err() != nil {
		s.err = s.it.Err()
	}
	return false
}

func (s *blockSeriesSet) Series() Series { return s.cur }
func (s *blockSeriesSet) Err() error     { return s.err }

// chunkSeries is a series that is backed by a sequence of chunks holding
// time series data.
type chunkSeries struct {
	labels labels.Labels
	chunks []ChunkMeta // in-order chunk refs

	// chunk is a function that retrieves chunks based on a reference
	// number contained in the chunk meta information.
	chunk func(ref uint32) (chunks.Chunk, error)
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
	Values() (t int64, v float64)
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

func (it *chainedSeriesIterator) Values() (t int64, v float64) {
	return it.cur.Values()
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
	x := sort.Search(len(it.mints), func(i int) bool { return it.mints[i] >= t })

	if x == len(it.mints) {
		return false
	}
	if it.mints[x] == t {
		if x == 0 {
			return false
		}
		x--
	}

	it.i = x
	it.cur = it.chunks[x].Iterator()

	for it.cur.Next() {
		t0, _ := it.cur.Values()
		if t0 >= t {
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
	if t0 <= b.lastTime {
		for b.Next() {
			if tcur, _ := b.it.Values(); tcur >= t {
				return true
			}
		}
		return false
	}

	b.buf.reset()

	ok := b.it.Seek(t0)
	if !ok {
		return false
	}

	for b.Next() {
		if ts, _ := b.Values(); ts >= t {
			return true
		}
	}
	return false
}

// Next advances the iterator to the next element.
func (b *BufferedSeriesIterator) Next() bool {
	// Add current element to buffer before advancing.
	b.buf.add(b.it.Values())

	ok := b.it.Next()
	b.lastTime, _ = b.Values()
	return ok
}

// Values returns the current element of the iterator.
func (b *BufferedSeriesIterator) Values() (int64, float64) {
	return b.it.Values()
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

func (it *sampleRingIterator) Values() (int64, float64) {
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
