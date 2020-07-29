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
		return []string{}, nil, nil
	}
	if len(qs) == 1 {
		return qs[0].LabelValues(n)
	}
	l := len(qs) / 2

	var ws storage.Warnings
	s1, w, err := q.lvals(qs[:l], n)
	ws = append(ws, w...)
	if err != nil {
		return []string{}, ws, err
	}
	s2, ws, err := q.lvals(qs[l:], n)
	ws = append(ws, w...)
	if err != nil {
		return []string{}, ws, err
	}
	return mergeStrings(s1, s2), ws, nil
}

func (q *querier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	if len(q.blocks) == 0 {
		return storage.EmptySeriesSet()
	}
	if len(q.blocks) == 1 {
		// Sorting Head series is slow, and unneeded when only the
		// Head is being queried.
		return q.blocks[0].Select(sortSeries, hints, ms...)
	}

	ss := make([]storage.SeriesSet, len(q.blocks))
	for i, b := range q.blocks {
		// We have to sort if blocks > 1 as MergedSeriesSet requires it.
		ss[i] = b.Select(true, hints, ms...)
	}

	return NewMergedSeriesSet(ss)
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
type verticalQuerier struct {
	querier
}

func (q *verticalQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	return q.sel(sortSeries, hints, q.blocks, ms)
}

func (q *verticalQuerier) sel(sortSeries bool, hints *storage.SelectHints, qs []storage.Querier, ms []*labels.Matcher) storage.SeriesSet {
	if len(qs) == 0 {
		return storage.EmptySeriesSet()
	}
	if len(qs) == 1 {
		return qs[0].Select(sortSeries, hints, ms...)
	}
	l := len(qs) / 2

	return newMergedVerticalSeriesSet(
		q.sel(sortSeries, hints, qs[:l], ms),
		q.sel(sortSeries, hints, qs[l:], ms),
	)
}

// NewBlockQuerier returns a querier against the reader.
func NewBlockQuerier(b BlockReader, mint, maxt int64) (storage.Querier, error) {
	indexr, err := b.Index()
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

func (q *blockQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	var base storage.DeprecatedChunkSeriesSet
	var err error

	if sortSeries {
		base, err = LookupChunkSeriesSorted(q.index, q.tombstones, ms...)
	} else {
		base, err = LookupChunkSeries(q.index, q.tombstones, ms...)
	}
	if err != nil {
		return storage.ErrSeriesSet(err)
	}

	mint := q.mint
	maxt := q.maxt
	if hints != nil {
		mint = hints.Start
		maxt = hints.End
	}
	return &blockSeriesSet{
		set: &populatedChunkSeries{
			set:    base,
			chunks: q.chunks,
			mint:   mint,
			maxt:   maxt,
		},

		mint: mint,
		maxt: maxt,
	}
}

func (q *blockQuerier) LabelValues(name string) ([]string, storage.Warnings, error) {
	res, err := q.index.SortedLabelValues(name)
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
	lastVal, isSorted := "", true
	for _, val := range vals {
		if m.Matches(val) {
			res = append(res, val)
			if isSorted && val < lastVal {
				isSorted = false
			}
			lastVal = val
		}
	}

	if len(res) == 0 {
		return index.EmptyPostings(), nil
	}

	if !isSorted {
		sort.Strings(res)
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
	lastVal, isSorted := "", true
	for _, val := range vals {
		if !m.Matches(val) {
			res = append(res, val)
			if isSorted && val < lastVal {
				isSorted = false
			}
			lastVal = val
		}
	}

	if !isSorted {
		sort.Strings(res)
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
// TODO(bwplotka): Merge this with merge SeriesSet available in storage package.
type mergedSeriesSet struct {
	all  []storage.SeriesSet
	buf  []storage.SeriesSet // A buffer for keeping the order of SeriesSet slice during forwarding the SeriesSet.
	ids  []int               // The indices of chosen SeriesSet for the current run.
	done bool
	err  error
	cur  storage.Series
}

// TODO(bwplotka): Merge this with merge SeriesSet available in storage package.
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

func (s *mergedSeriesSet) Warnings() storage.Warnings {
	var ws storage.Warnings
	for _, ss := range s.all {
		ws = append(ws, ss.Warnings()...)
	}
	return ws
}

// nextAll is to call Next() for all SeriesSet.
// Because the order of the SeriesSet slice will affect the results,
// we need to use an buffer slice to hold the order.
func (s *mergedSeriesSet) nextAll() {
	s.buf = s.buf[:0]
	for _, ss := range s.all {
		if ss.Next() {
			s.buf = append(s.buf, ss)
			continue
		}

		if ss.Err() != nil {
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
		s.cur = &chainedSeries{series: series}
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

func (s *mergedVerticalSeriesSet) Warnings() storage.Warnings {
	var ws storage.Warnings
	ws = append(ws, s.a.Warnings()...)
	ws = append(ws, s.b.Warnings()...)
	return ws
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
		s.cur = &verticalChainedSeries{series: []storage.Series{s.a.At(), s.b.At()}}
		s.adone = !s.a.Next()
		s.bdone = !s.b.Next()
	}
	return true
}

// baseChunkSeries loads the label set and chunk references for a postings
// list from an index. It filters out series that have labels set that should be unset.
type baseChunkSeries struct {
	p          index.Postings
	index      IndexReader
	tombstones tombstones.Reader

	lset      labels.Labels
	chks      []chunks.Meta
	intervals tombstones.Intervals
	err       error
}

// LookupChunkSeries retrieves all series for the given matchers and returns a ChunkSeriesSet
// over them. It drops chunks based on tombstones in the given reader.
func LookupChunkSeries(ir IndexReader, tr tombstones.Reader, ms ...*labels.Matcher) (storage.DeprecatedChunkSeriesSet, error) {
	return lookupChunkSeries(false, ir, tr, ms...)
}

// LookupChunkSeries retrieves all series for the given matchers and returns a ChunkSeriesSet
// over them. It drops chunks based on tombstones in the given reader. Series will be in order.
func LookupChunkSeriesSorted(ir IndexReader, tr tombstones.Reader, ms ...*labels.Matcher) (storage.DeprecatedChunkSeriesSet, error) {
	return lookupChunkSeries(true, ir, tr, ms...)
}

func lookupChunkSeries(sorted bool, ir IndexReader, tr tombstones.Reader, ms ...*labels.Matcher) (storage.DeprecatedChunkSeriesSet, error) {
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
	return &baseChunkSeries{
		p:          p,
		index:      ir,
		tombstones: tr,
	}, nil
}

func (s *baseChunkSeries) At() (labels.Labels, []chunks.Meta, tombstones.Intervals) {
	return s.lset, s.chks, s.intervals
}

func (s *baseChunkSeries) Err() error { return s.err }

func (s *baseChunkSeries) Next() bool {
	var (
		lset     = make(labels.Labels, len(s.lset))
		chkMetas = make([]chunks.Meta, len(s.chks))
		err      error
	)

	for s.p.Next() {
		ref := s.p.At()
		if err := s.index.Series(ref, &lset, &chkMetas); err != nil {
			// Postings may be stale. Skip if no underlying series exists.
			if errors.Cause(err) == storage.ErrNotFound {
				continue
			}
			s.err = err
			return false
		}

		s.lset = lset
		s.chks = chkMetas
		s.intervals, err = s.tombstones.Get(s.p.At())
		if err != nil {
			s.err = errors.Wrap(err, "get tombstones")
			return false
		}

		if len(s.intervals) > 0 {
			// Only those chunks that are not entirely deleted.
			chks := make([]chunks.Meta, 0, len(s.chks))
			for _, chk := range s.chks {
				if !(tombstones.Interval{Mint: chk.MinTime, Maxt: chk.MaxTime}.IsSubrange(s.intervals)) {
					chks = append(chks, chk)
				}
			}

			s.chks = chks
		}

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
	set        storage.DeprecatedChunkSeriesSet
	chunks     ChunkReader
	mint, maxt int64

	err       error
	chks      []chunks.Meta
	lset      labels.Labels
	intervals tombstones.Intervals
}

func (s *populatedChunkSeries) At() (labels.Labels, []chunks.Meta, tombstones.Intervals) {
	return s.lset, s.chks, s.intervals
}

func (s *populatedChunkSeries) Err() error { return s.err }

func (s *populatedChunkSeries) Next() bool {
	for s.set.Next() {
		lset, chks, dranges := s.set.At()

		for len(chks) > 0 {
			if chks[0].MaxTime >= s.mint {
				break
			}
			chks = chks[1:]
		}

		// This is to delete in place while iterating.
		for i, rlen := 0, len(chks); i < rlen; i++ {
			j := i - (rlen - len(chks))
			c := &chks[j]

			// Break out at the first chunk that has no overlap with mint, maxt.
			if c.MinTime > s.maxt {
				chks = chks[:j]
				break
			}

			c.Chunk, s.err = s.chunks.Chunk(c.Ref)
			if s.err != nil {
				// This means that the chunk has be garbage collected. Remove it from the list.
				if s.err == storage.ErrNotFound {
					s.err = nil
					// Delete in-place.
					s.chks = append(chks[:j], chks[j+1:]...)
				}
				return false
			}
		}

		if len(chks) == 0 {
			continue
		}

		s.lset = lset
		s.chks = chks
		s.intervals = dranges

		return true
	}
	if err := s.set.Err(); err != nil {
		s.err = err
	}
	return false
}

// blockSeriesSet is a set of series from an inverted index query.
type blockSeriesSet struct {
	set storage.DeprecatedChunkSeriesSet
	err error
	cur storage.Series

	mint, maxt int64
}

func (s *blockSeriesSet) Next() bool {
	for s.set.Next() {
		lset, chunks, dranges := s.set.At()
		s.cur = &chunkSeries{
			labels: lset,
			chunks: chunks,
			mint:   s.mint,
			maxt:   s.maxt,

			intervals: dranges,
		}
		return true
	}
	if s.set.Err() != nil {
		s.err = s.set.Err()
	}
	return false
}

func (s *blockSeriesSet) At() storage.Series         { return s.cur }
func (s *blockSeriesSet) Err() error                 { return s.err }
func (s *blockSeriesSet) Warnings() storage.Warnings { return nil }

// chunkSeries is a series that is backed by a sequence of chunks holding
// time series data.
type chunkSeries struct {
	labels labels.Labels
	chunks []chunks.Meta // in-order chunk refs

	mint, maxt int64

	intervals tombstones.Intervals
}

func (s *chunkSeries) Labels() labels.Labels {
	return s.labels
}

func (s *chunkSeries) Iterator() chunkenc.Iterator {
	return newChunkSeriesIterator(s.chunks, s.intervals, s.mint, s.maxt)
}

// chainedSeries implements a series for a list of time-sorted series.
// They all must have the same labels.
type chainedSeries struct {
	series []storage.Series
}

func (s *chainedSeries) Labels() labels.Labels {
	return s.series[0].Labels()
}

func (s *chainedSeries) Iterator() chunkenc.Iterator {
	return newChainedSeriesIterator(s.series...)
}

// chainedSeriesIterator implements a series iterator over a list
// of time-sorted, non-overlapping iterators.
type chainedSeriesIterator struct {
	series []storage.Series // series in time order

	i   int
	cur chunkenc.Iterator
}

func newChainedSeriesIterator(s ...storage.Series) *chainedSeriesIterator {
	return &chainedSeriesIterator{
		series: s,
		i:      0,
		cur:    s[0].Iterator(),
	}
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

// verticalChainedSeries implements a series for a list of time-sorted, time-overlapping series.
// They all must have the same labels.
type verticalChainedSeries struct {
	series []storage.Series
}

func (s *verticalChainedSeries) Labels() labels.Labels {
	return s.series[0].Labels()
}

func (s *verticalChainedSeries) Iterator() chunkenc.Iterator {
	return newVerticalMergeSeriesIterator(s.series...)
}

// verticalMergeSeriesIterator implements a series iterator over a list
// of time-sorted, time-overlapping iterators.
type verticalMergeSeriesIterator struct {
	a, b                  chunkenc.Iterator
	aok, bok, initialized bool

	curT int64
	curV float64
}

func newVerticalMergeSeriesIterator(s ...storage.Series) chunkenc.Iterator {
	if len(s) == 1 {
		return s[0].Iterator()
	} else if len(s) == 2 {
		return &verticalMergeSeriesIterator{
			a: s[0].Iterator(),
			b: s[1].Iterator(),
		}
	}
	return &verticalMergeSeriesIterator{
		a: s[0].Iterator(),
		b: newVerticalMergeSeriesIterator(s[1:]...),
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
	if ok := it.it.Seek(t); !ok {
		return false
	}

	// Now double check if the entry falls into a deleted interval.
	ts, _ := it.At()
	for _, itv := range it.intervals {
		if ts < itv.Mint {
			return true
		}

		if ts > itv.Maxt {
			it.intervals = it.intervals[1:]
			continue
		}

		// We're in the middle of an interval, we can now call Next().
		return it.Next()
	}

	// The timestamp is greater than all the deleted intervals.
	return true
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
