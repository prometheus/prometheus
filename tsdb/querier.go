// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tsdb

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

// querier aggregates querying results from time blocks within
// a single partition.
type querier struct {
	blocks []storage.Querier
}

func (q *querier) LabelValues(n string) ([]string, storage.Warnings, error) {
	return q.lvals(q.blocks, n)
}

// LabelNames returns all the unique label names present querier blocks.
func (q *querier) LabelNames() ([]string, storage.Warnings, error) {
	labelNamesMap := make(map[string]struct{})
	var ws storage.Warnings
	for _, b := range q.blocks {
		names, w, err := b.LabelNames()
		ws = append(ws, w...)
		if err != nil {
			return nil, ws, errors.Wrap(err, "LabelNames() from Querier")
		}
		for _, name := range names {
			labelNamesMap[name] = struct{}{}
		}
	}

	labelNames := make([]string, 0, len(labelNamesMap))
	for name := range labelNamesMap {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)

	return labelNames, ws, nil
}

func (q *querier) lvals(qs []storage.Querier, n string) ([]string, storage.Warnings, error) {
	if len(qs) == 0 {
		return nil, nil, nil
	}
	if len(qs) == 1 {
		return qs[0].LabelValues(n)
	}
	l := len(qs) / 2

	var ws storage.Warnings
	s1, w, err := q.lvals(qs[:l], n)
	ws = append(ws, w...)
	if err != nil {
		return nil, ws, err
	}
	s2, ws, err := q.lvals(qs[l:], n)
	ws = append(ws, w...)
	if err != nil {
		return nil, ws, err
	}
	return mergeStrings(s1, s2), ws, nil
}

func (q *querier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	if len(q.blocks) == 0 {
		return storage.EmptySeriesSet(), nil, nil
	}
	if len(q.blocks) == 1 {
		// Sorting Head series is slow, and unneeded when only the
		// Head is being queried.
		return q.blocks[0].Select(sortSeries, hints, ms...)
	}

	ss := make([]storage.SeriesSet, len(q.blocks))
	var ws storage.Warnings
	for i, b := range q.blocks {
		// We have to sort if blocks > 1 as MergedSeriesSet requires it.
		s, w, err := b.Select(true, hints, ms...)
		ws = append(ws, w...)
		if err != nil {
			return nil, ws, err
		}
		ss[i] = s
	}

	return NewMergedSeriesSet(ss), ws, nil
}

func (q *querier) Close() error {
	var merr tsdb_errors.MultiError

	for _, bq := range q.blocks {
		merr.Add(bq.Close())
	}
	return merr.Err()
}

// verticalQuerier aggregates querying results from time blocks within
// a single partition. The block time ranges can be overlapping.
// TODO(bwplotka): Remove this once we move to storage.MergeSeriesSet function as they handle overlaps.
type verticalQuerier struct {
	querier
}

func (q *verticalQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	return q.sel(sortSeries, hints, q.blocks, ms)
}

func (q *verticalQuerier) sel(sortSeries bool, hints *storage.SelectHints, qs []storage.Querier, ms []*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	if len(qs) == 0 {
		return storage.EmptySeriesSet(), nil, nil
	}
	if len(qs) == 1 {
		return qs[0].Select(sortSeries, hints, ms...)
	}
	l := len(qs) / 2

	var ws storage.Warnings
	a, w, err := q.sel(sortSeries, hints, qs[:l], ms)
	ws = append(ws, w...)
	if err != nil {
		return nil, ws, err
	}
	b, w, err := q.sel(sortSeries, hints, qs[l:], ms)
	ws = append(ws, w...)
	if err != nil {
		return nil, ws, err
	}
	return newMergedVerticalSeriesSet(a, b), ws, nil
}

// NewBlockQuerier returns a querier against the reader.
func NewBlockQuerier(b BlockReader, mint, maxt int64) (storage.Querier, error) {
	indexr, err := b.Index(mint, maxt)
	if err != nil {
		return nil, errors.Wrapf(err, "open index reader")
	}
	chunkr, err := b.Chunks()
	if err != nil {
		indexr.Close()
		return nil, errors.Wrapf(err, "open chunk reader")
	}
	tombsr, err := b.Tombstones()
	if err != nil {
		indexr.Close()
		chunkr.Close()
		return nil, errors.Wrapf(err, "open tombstone reader")
	}
	return &blockQuerier{
		mint:       mint,
		maxt:       maxt,
		index:      indexr,
		chunks:     chunkr,
		tombstones: tombsr,
	}, nil
}

// blockQuerier provides querying access to a single block database.
type blockQuerier struct {
	index      IndexReader
	chunks     ChunkReader
	tombstones tombstones.Reader

	closed bool

	mint, maxt int64
}

func (q *blockQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	mint := q.mint
	maxt := q.maxt
	if hints != nil {
		mint = hints.Start
		maxt = hints.End
	}

	base, err := LookupChunkSeries(sortSeries, q.index, q.tombstones, q.chunks, mint, maxt, ms...)
	if err != nil {
		return nil, nil, err
	}

	return storage.NewSeriesSetFromChunkSeriesSet(base), nil, nil
}

func (q *blockQuerier) LabelValues(name string) ([]string, storage.Warnings, error) {
	res, err := q.index.LabelValues(name)
	return res, nil, err
}

func (q *blockQuerier) LabelNames() ([]string, storage.Warnings, error) {
	res, err := q.index.LabelNames()
	return res, nil, err
}

func (q *blockQuerier) Close() error {
	if q.closed {
		return errors.New("block querier already closed")
	}

	var merr tsdb_errors.MultiError
	merr.Add(q.index.Close())
	merr.Add(q.chunks.Close())
	merr.Add(q.tombstones.Close())
	q.closed = true
	return merr.Err()
}

// Bitmap used by func isRegexMetaCharacter to check whether a character needs to be escaped.
var regexMetaCharacterBytes [16]byte

// isRegexMetaCharacter reports whether byte b needs to be escaped.
func isRegexMetaCharacter(b byte) bool {
	return b < utf8.RuneSelf && regexMetaCharacterBytes[b%16]&(1<<(b/16)) != 0
}

func init() {
	for _, b := range []byte(`.+*?()|[]{}^$`) {
		regexMetaCharacterBytes[b%16] |= 1 << (b / 16)
	}
}

func findSetMatches(pattern string) []string {
	// Return empty matches if the wrapper from Prometheus is missing.
	if len(pattern) < 6 || pattern[:4] != "^(?:" || pattern[len(pattern)-2:] != ")$" {
		return nil
	}
	escaped := false
	sets := []*strings.Builder{{}}
	for i := 4; i < len(pattern)-2; i++ {
		if escaped {
			switch {
			case isRegexMetaCharacter(pattern[i]):
				sets[len(sets)-1].WriteByte(pattern[i])
			case pattern[i] == '\\':
				sets[len(sets)-1].WriteByte('\\')
			default:
				return nil
			}
			escaped = false
		} else {
			switch {
			case isRegexMetaCharacter(pattern[i]):
				if pattern[i] == '|' {
					sets = append(sets, &strings.Builder{})
				} else {
					return nil
				}
			case pattern[i] == '\\':
				escaped = true
			default:
				sets[len(sets)-1].WriteByte(pattern[i])
			}
		}
	}
	matches := make([]string, 0, len(sets))
	for _, s := range sets {
		if s.Len() > 0 {
			matches = append(matches, s.String())
		}
	}
	return matches
}

// PostingsForMatchers assembles a single postings iterator against the index reader
// based on the given matchers. The resulting postings are not ordered by series.
func PostingsForMatchers(ix IndexReader, ms ...*labels.Matcher) (index.Postings, error) {
	var its, notIts []index.Postings
	// See which label must be non-empty.
	// Optimization for case like {l=~".", l!="1"}.
	labelMustBeSet := make(map[string]bool, len(ms))
	for _, m := range ms {
		if !m.Matches("") {
			labelMustBeSet[m.Name] = true
		}
	}

	for _, m := range ms {
		if labelMustBeSet[m.Name] {
			// If this matcher must be non-empty, we can be smarter.
			matchesEmpty := m.Matches("")
			isNot := m.Type == labels.MatchNotEqual || m.Type == labels.MatchNotRegexp
			if isNot && matchesEmpty { // l!="foo"
				// If the label can't be empty and is a Not and the inner matcher
				// doesn't match empty, then subtract it out at the end.
				inverse, err := m.Inverse()
				if err != nil {
					return nil, err
				}

				it, err := postingsForMatcher(ix, inverse)
				if err != nil {
					return nil, err
				}
				notIts = append(notIts, it)
			} else if isNot && !matchesEmpty { // l!=""
				// If the label can't be empty and is a Not, but the inner matcher can
				// be empty we need to use inversePostingsForMatcher.
				inverse, err := m.Inverse()
				if err != nil {
					return nil, err
				}

				it, err := inversePostingsForMatcher(ix, inverse)
				if err != nil {
					return nil, err
				}
				its = append(its, it)
			} else { // l="a"
				// Non-Not matcher, use normal postingsForMatcher.
				it, err := postingsForMatcher(ix, m)
				if err != nil {
					return nil, err
				}
				its = append(its, it)
			}
		} else { // l=""
			// If the matchers for a labelname selects an empty value, it selects all
			// the series which don't have the label name set too. See:
			// https://github.com/prometheus/prometheus/issues/3575 and
			// https://github.com/prometheus/prometheus/pull/3578#issuecomment-351653555
			it, err := inversePostingsForMatcher(ix, m)
			if err != nil {
				return nil, err
			}
			notIts = append(notIts, it)
		}
	}

	// If there's nothing to subtract from, add in everything and remove the notIts later.
	if len(its) == 0 && len(notIts) != 0 {
		k, v := index.AllPostingsKey()
		allPostings, err := ix.Postings(k, v)
		if err != nil {
			return nil, err
		}
		its = append(its, allPostings)
	}

	it := index.Intersect(its...)

	for _, n := range notIts {
		it = index.Without(it, n)
	}

	return it, nil
}

func postingsForMatcher(ix IndexReader, m *labels.Matcher) (index.Postings, error) {
	// This method will not return postings for missing labels.

	// Fast-path for equal matching.
	if m.Type == labels.MatchEqual {
		return ix.Postings(m.Name, m.Value)
	}

	// Fast-path for set matching.
	if m.Type == labels.MatchRegexp {
		setMatches := findSetMatches(m.GetRegexString())
		if len(setMatches) > 0 {
			sort.Strings(setMatches)
			return ix.Postings(m.Name, setMatches...)
		}
	}

	vals, err := ix.LabelValues(m.Name)
	if err != nil {
		return nil, err
	}

	var res []string
	for _, val := range vals {
		if m.Matches(val) {
			res = append(res, val)
		}
	}

	if len(res) == 0 {
		return index.EmptyPostings(), nil
	}

	return ix.Postings(m.Name, res...)
}

// inversePostingsForMatcher returns the postings for the series with the label name set but not matching the matcher.
func inversePostingsForMatcher(ix IndexReader, m *labels.Matcher) (index.Postings, error) {
	vals, err := ix.LabelValues(m.Name)
	if err != nil {
		return nil, err
	}

	var res []string
	for _, val := range vals {
		if !m.Matches(val) {
			res = append(res, val)
		}
	}

	return ix.Postings(m.Name, res...)
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

// mergedSeriesSet returns a series sets slice as a single series set. The input series sets
// must be sorted and sequential in time.
// TODO(bwplotka): Merge this with merge SeriesSet available in storage package (to limit size of PR).
type mergedSeriesSet struct {
	all  []storage.SeriesSet
	buf  []storage.SeriesSet // A buffer for keeping the order of SeriesSet slice during forwarding the SeriesSet.
	ids  []int               // The indices of chosen SeriesSet for the current run.
	done bool
	err  error
	cur  storage.Series
}

// TODO(bwplotka): Merge this with merge SeriesSet available in storage package (to limit size of PR).
func NewMergedSeriesSet(all []storage.SeriesSet) storage.SeriesSet {
	if len(all) == 1 {
		return all[0]
	}
	s := &mergedSeriesSet{all: all}
	// Initialize first elements of all sets as Next() needs
	// one element look-ahead.
	s.nextAll()
	if len(s.all) == 0 {
		s.done = true
	}

	return s
}

func (s *mergedSeriesSet) At() storage.Series {
	return s.cur
}

func (s *mergedSeriesSet) Err() error {
	return s.err
}

// nextAll is to call Next() for all SeriesSet.
// Because the order of the SeriesSet slice will affect the results,
// we need to use an buffer slice to hold the order.
func (s *mergedSeriesSet) nextAll() {
	s.buf = s.buf[:0]
	for _, ss := range s.all {
		if ss.Next() {
			s.buf = append(s.buf, ss)
		} else if ss.Err() != nil {
			s.done = true
			s.err = ss.Err()
			break
		}
	}
	s.all, s.buf = s.buf, s.all
}

// nextWithID is to call Next() for the SeriesSet with the indices of s.ids.
// Because the order of the SeriesSet slice will affect the results,
// we need to use an buffer slice to hold the order.
func (s *mergedSeriesSet) nextWithID() {
	if len(s.ids) == 0 {
		return
	}

	s.buf = s.buf[:0]
	i1 := 0
	i2 := 0
	for i1 < len(s.all) {
		if i2 < len(s.ids) && i1 == s.ids[i2] {
			if !s.all[s.ids[i2]].Next() {
				if s.all[s.ids[i2]].Err() != nil {
					s.done = true
					s.err = s.all[s.ids[i2]].Err()
					break
				}
				i2++
				i1++
				continue
			}
			i2++
		}
		s.buf = append(s.buf, s.all[i1])
		i1++
	}
	s.all, s.buf = s.buf, s.all
}

func (s *mergedSeriesSet) Next() bool {
	if s.done {
		return false
	}

	s.nextWithID()
	if s.done {
		return false
	}
	s.ids = s.ids[:0]
	if len(s.all) == 0 {
		s.done = true
		return false
	}

	// Here we are looking for a set of series sets with the lowest labels,
	// and we will cache their indexes in s.ids.
	s.ids = append(s.ids, 0)
	for i := 1; i < len(s.all); i++ {
		cmp := labels.Compare(s.all[s.ids[0]].At().Labels(), s.all[i].At().Labels())
		if cmp > 0 {
			s.ids = s.ids[:1]
			s.ids[0] = i
		} else if cmp == 0 {
			s.ids = append(s.ids, i)
		}
	}

	if len(s.ids) > 1 {
		series := make([]storage.Series, len(s.ids))
		for i, idx := range s.ids {
			series[i] = s.all[idx].At()
		}
		s.cur = storage.ChainedSeriesMerge(series...)
	} else {
		s.cur = s.all[s.ids[0]].At()
	}
	return true
}

type mergedVerticalSeriesSet struct {
	a, b         storage.SeriesSet
	cur          storage.Series
	adone, bdone bool
}

// NewMergedVerticalSeriesSet takes two series sets as a single series set.
// The input series sets must be sorted and
// the time ranges of the series can be overlapping.
func NewMergedVerticalSeriesSet(a, b storage.SeriesSet) storage.SeriesSet {
	return newMergedVerticalSeriesSet(a, b)
}

func newMergedVerticalSeriesSet(a, b storage.SeriesSet) *mergedVerticalSeriesSet {
	s := &mergedVerticalSeriesSet{a: a, b: b}
	// Initialize first elements of both sets as Next() needs
	// one element look-ahead.
	s.adone = !s.a.Next()
	s.bdone = !s.b.Next()

	return s
}

func (s *mergedVerticalSeriesSet) At() storage.Series {
	return s.cur
}

func (s *mergedVerticalSeriesSet) Err() error {
	if s.a.Err() != nil {
		return s.a.Err()
	}
	return s.b.Err()
}

func (s *mergedVerticalSeriesSet) compare() int {
	if s.adone {
		return 1
	}
	if s.bdone {
		return -1
	}
	return labels.Compare(s.a.At().Labels(), s.b.At().Labels())
}

func (s *mergedVerticalSeriesSet) Next() bool {
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
		s.cur = storage.ChainedSeriesMerge([]storage.Series{s.a.At(), s.b.At()}...)
		s.adone = !s.a.Next()
		s.bdone = !s.b.Next()
	}
	return true
}

// LookupChunkSeries retrieves all series for the given matchers and returns a ChunkSeriesSet
// over them. It drops chunks based on tombstones in the given reader.
func LookupChunkSeries(sorted bool, ir IndexReader, tr tombstones.Reader, c ChunkReader, minTime, maxTime int64, ms ...*labels.Matcher) (storage.ChunkSeriesSet, error) {
	if tr == nil {
		tr = tombstones.NewMemTombstones()
	}
	p, err := PostingsForMatchers(ir, ms...)
	if err != nil {
		return nil, err
	}
	if sorted {
		p = ir.SortedPostings(p)
	}
	return newBlockChunkSeriesSet(ir, c, tr, p, minTime, maxTime), nil
}

// MergeOverlappingChunksVertically merges multiple time overlapping chunks together into one.
// TODO(bwplotka): https://github.com/prometheus/tsdb/issues/670
func MergeOverlappingChunksVertically(chks ...chunks.Meta) chunks.Iterator {
	chk := chunkenc.NewXORChunk()
	app, err := chk.Appender()
	if err != nil {
		return errChunksIterator{err: err}
	}
	seriesIter := newVerticalMergeSeriesIterator(chks...)

	mint := int64(math.MaxInt64)
	maxt := int64(math.MinInt64)

	for seriesIter.Next() {
		t, v := seriesIter.At()
		app.Append(t, v)

		maxt = t
		if mint == math.MaxInt64 {
			mint = t
		}
	}
	if err := seriesIter.Err(); err != nil {
		return errChunksIterator{err: err}
	}

	return storage.NewListChunkSeriesIterator(chunks.Meta{
		MinTime: mint,
		MaxTime: maxt,
		Chunk:   chk,
	})
}

type errChunksIterator struct {
	err error
}

func (e errChunksIterator) At() chunks.Meta { return chunks.Meta{} }
func (e errChunksIterator) Next() bool      { return false }
func (e errChunksIterator) Err() error      { return e.err }

// verticalMergeSeriesIterator implements a series iterator over a list
// of time-sorted, time-overlapping iterators.
type verticalMergeSeriesIterator struct {
	a, b                  chunkenc.Iterator
	aok, bok, initialized bool

	curT int64
	curV float64
}

func newVerticalMergeSeriesIterator(s ...chunks.Meta) chunkenc.Iterator {
	if len(s) == 1 {
		return s[0].Chunk.Iterator(nil)
	}

	if len(s) == 2 {
		return &verticalMergeSeriesIterator{
			a:    s[0].Chunk.Iterator(nil),
			b:    s[1].Chunk.Iterator(nil),
			curT: math.MinInt64,
		}
	}

	return &verticalMergeSeriesIterator{
		a:    s[0].Chunk.Iterator(nil),
		b:    newVerticalMergeSeriesIterator(s[1:]...),
		curT: math.MinInt64,
	}
}

func (it *verticalMergeSeriesIterator) Seek(t int64) bool {
	it.aok, it.bok = it.a.Seek(t), it.b.Seek(t)
	it.initialized = true
	return it.Next()
}

func (it *verticalMergeSeriesIterator) Next() bool {
	if !it.initialized {
		it.aok = it.a.Next()
		it.bok = it.b.Next()
		it.initialized = true
	}

	if !it.aok && !it.bok {
		return false
	}

	if !it.aok {
		it.curT, it.curV = it.b.At()
		it.bok = it.b.Next()
		return true
	}
	if !it.bok {
		it.curT, it.curV = it.a.At()
		it.aok = it.a.Next()
		return true
	}

	acurT, acurV := it.a.At()
	bcurT, bcurV := it.b.At()
	if acurT < bcurT {
		it.curT, it.curV = acurT, acurV
		it.aok = it.a.Next()
	} else if acurT > bcurT {
		it.curT, it.curV = bcurT, bcurV
		it.bok = it.b.Next()
	} else {
		it.curT, it.curV = bcurT, bcurV
		it.aok = it.a.Next()
		it.bok = it.b.Next()
	}
	return true
}

func (it *verticalMergeSeriesIterator) At() (t int64, v float64) {
	return it.curT, it.curV
}

func (it *verticalMergeSeriesIterator) Err() error {
	if it.a.Err() != nil {
		return it.a.Err()
	}
	return it.b.Err()
}

// chunkSeriesIterator implements a series iterator on top
// of a list of time-sorted, non-overlapping chunks.
type chunkSeriesIterator struct {
	chunks []chunks.Meta

	i          int
	cur        chunkenc.Iterator
	bufDelIter *deletedIterator

	maxt, mint int64

	intervals tombstones.Intervals
}

func newChunkSeriesIterator(cs []chunks.Meta, dranges tombstones.Intervals, mint, maxt int64) *chunkSeriesIterator {
	csi := &chunkSeriesIterator{
		chunks: cs,
		i:      0,

		mint: mint,
		maxt: maxt,

		intervals: dranges,
	}
	csi.resetCurIterator()

	return csi
}

func (it *chunkSeriesIterator) resetCurIterator() {
	if len(it.intervals) == 0 {
		it.cur = it.chunks[it.i].Chunk.Iterator(it.cur)
		return
	}
	if it.bufDelIter == nil {
		it.bufDelIter = &deletedIterator{
			intervals: it.intervals,
		}
	}
	it.bufDelIter.it = it.chunks[it.i].Chunk.Iterator(it.bufDelIter.it)
	it.cur = it.bufDelIter
}

func (it *chunkSeriesIterator) Seek(t int64) (ok bool) {
	if t > it.maxt {
		return false
	}

	// Seek to the first valid value after t.
	if t < it.mint {
		t = it.mint
	}

	for ; it.chunks[it.i].MaxTime < t; it.i++ {
		if it.i == len(it.chunks)-1 {
			return false
		}
	}

	it.resetCurIterator()

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
		t, _ := it.cur.At()

		if t < it.mint {
			if !it.Seek(it.mint) {
				return false
			}
			t, _ = it.At()

			return t <= it.maxt
		}
		if t > it.maxt {
			return false
		}
		return true
	}
	if err := it.cur.Err(); err != nil {
		return false
	}
	if it.i == len(it.chunks)-1 {
		return false
	}

	it.i++
	it.resetCurIterator()

	return it.Next()
}

func (it *chunkSeriesIterator) Err() error {
	return it.cur.Err()
}

// deletedIterator wraps an Iterator and makes sure any deleted metrics are not
// returned.
type deletedIterator struct {
	it chunkenc.Iterator

	intervals tombstones.Intervals
}

func (it *deletedIterator) At() (int64, float64) {
	return it.it.At()
}

func (it *deletedIterator) Seek(t int64) bool {
	if it.it.Err() != nil {
		return false
	}
	return it.it.Seek(t)
}

func (it *deletedIterator) Next() bool {
Outer:
	for it.it.Next() {
		ts, _ := it.it.At()

		for _, tr := range it.intervals {
			if tr.InBounds(ts) {
				continue Outer
			}

			if ts <= tr.Maxt {
				return true

			}
			it.intervals = it.intervals[1:]
		}
		return true
	}
	return false
}

func (it *deletedIterator) Err() error { return it.it.Err() }
