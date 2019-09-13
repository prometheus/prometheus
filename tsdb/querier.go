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
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/labels"
)

// Querier provides querying access over time series data of a fixed
// time range.
type Querier interface {
	// Select returns a set of series that matches the given label matchers.
	Select(...labels.Matcher) (SeriesSet, error)

	// LabelValues returns all potential values for a label name.
	LabelValues(string) ([]string, error)

	// LabelValuesFor returns all potential values for a label name.
	// under the constraint of another label.
	LabelValuesFor(string, labels.Label) ([]string, error)

	// LabelNames returns all the unique label names present in the block in sorted order.
	LabelNames() ([]string, error)

	// Close releases the resources of the Querier.
	Close() error
}

// Series exposes a single time series.
type Series interface {
	// Labels returns the complete set of labels identifying the series.
	Labels() labels.Labels

	// Iterator returns a new iterator of the data of the series.
	Iterator() SeriesIterator
}

// querier aggregates querying results from time blocks within
// a single partition.
type querier struct {
	blocks []Querier
}

func (q *querier) LabelValues(n string) ([]string, error) {
	return q.lvals(q.blocks, n)
}

// LabelNames returns all the unique label names present querier blocks.
func (q *querier) LabelNames() ([]string, error) {
	labelNamesMap := make(map[string]struct{})
	for _, b := range q.blocks {
		names, err := b.LabelNames()
		if err != nil {
			return nil, errors.Wrap(err, "LabelNames() from Querier")
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

	return labelNames, nil
}

func (q *querier) lvals(qs []Querier, n string) ([]string, error) {
	if len(qs) == 0 {
		return nil, nil
	}
	if len(qs) == 1 {
		return qs[0].LabelValues(n)
	}
	l := len(qs) / 2
	s1, err := q.lvals(qs[:l], n)
	if err != nil {
		return nil, err
	}
	s2, err := q.lvals(qs[l:], n)
	if err != nil {
		return nil, err
	}
	return mergeStrings(s1, s2), nil
}

func (q *querier) LabelValuesFor(string, labels.Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (q *querier) Select(ms ...labels.Matcher) (SeriesSet, error) {
	return q.sel(q.blocks, ms)
}

func (q *querier) sel(qs []Querier, ms []labels.Matcher) (SeriesSet, error) {
	if len(qs) == 0 {
		return EmptySeriesSet(), nil
	}
	if len(qs) == 1 {
		return qs[0].Select(ms...)
	}
	l := len(qs) / 2

	a, err := q.sel(qs[:l], ms)
	if err != nil {
		return nil, err
	}
	b, err := q.sel(qs[l:], ms)
	if err != nil {
		return nil, err
	}
	return newMergedSeriesSet(a, b), nil
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

func (q *verticalQuerier) Select(ms ...labels.Matcher) (SeriesSet, error) {
	return q.sel(q.blocks, ms)
}

func (q *verticalQuerier) sel(qs []Querier, ms []labels.Matcher) (SeriesSet, error) {
	if len(qs) == 0 {
		return EmptySeriesSet(), nil
	}
	if len(qs) == 1 {
		return qs[0].Select(ms...)
	}
	l := len(qs) / 2

	a, err := q.sel(qs[:l], ms)
	if err != nil {
		return nil, err
	}
	b, err := q.sel(qs[l:], ms)
	if err != nil {
		return nil, err
	}
	return newMergedVerticalSeriesSet(a, b), nil
}

// NewBlockQuerier returns a querier against the reader.
func NewBlockQuerier(b BlockReader, mint, maxt int64) (Querier, error) {
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
	tombstones TombstoneReader

	closed bool

	mint, maxt int64
}

func (q *blockQuerier) Select(ms ...labels.Matcher) (SeriesSet, error) {
	base, err := LookupChunkSeries(q.index, q.tombstones, ms...)
	if err != nil {
		return nil, err
	}
	return &blockSeriesSet{
		set: &populatedChunkSeries{
			set:    base,
			chunks: q.chunks,
			mint:   q.mint,
			maxt:   q.maxt,
		},

		mint: q.mint,
		maxt: q.maxt,
	}, nil
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

func (q *blockQuerier) LabelNames() ([]string, error) {
	return q.index.LabelNames()
}

func (q *blockQuerier) LabelValuesFor(string, labels.Label) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
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
// based on the given matchers.
func PostingsForMatchers(ix IndexReader, ms ...labels.Matcher) (index.Postings, error) {
	var its, notIts []index.Postings
	// See which label must be non-empty.
	// Optimization for case like {l=~".", l!="1"}.
	labelMustBeSet := make(map[string]bool, len(ms))
	for _, m := range ms {
		if !m.Matches("") {
			labelMustBeSet[m.Name()] = true
		}
	}

	for _, m := range ms {
		if labelMustBeSet[m.Name()] {
			// If this matcher must be non-empty, we can be smarter.
			matchesEmpty := m.Matches("")
			nm, isNot := m.(*labels.NotMatcher)
			if isNot && matchesEmpty { // l!="foo"
				// If the label can't be empty and is a Not and the inner matcher
				// doesn't match empty, then subtract it out at the end.
				it, err := postingsForMatcher(ix, nm.Matcher)
				if err != nil {
					return nil, err
				}
				notIts = append(notIts, it)
			} else if isNot && !matchesEmpty { // l!=""
				// If the label can't be empty and is a Not, but the inner matcher can
				// be empty we need to use inversePostingsForMatcher.
				it, err := inversePostingsForMatcher(ix, nm.Matcher)
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
		allPostings, err := ix.Postings(index.AllPostingsKey())
		if err != nil {
			return nil, err
		}
		its = append(its, allPostings)
	}

	it := index.Intersect(its...)

	for _, n := range notIts {
		it = index.Without(it, n)
	}

	return ix.SortedPostings(it), nil
}

func postingsForMatcher(ix IndexReader, m labels.Matcher) (index.Postings, error) {
	// This method will not return postings for missing labels.

	// Fast-path for equal matching.
	if em, ok := m.(*labels.EqualMatcher); ok {
		return ix.Postings(em.Name(), em.Value())
	}

	// Fast-path for set matching.
	if em, ok := m.(*labels.RegexpMatcher); ok {
		setMatches := findSetMatches(em.Value())
		if len(setMatches) > 0 {
			return postingsForSetMatcher(ix, em.Name(), setMatches)
		}
	}

	tpls, err := ix.LabelValues(m.Name())
	if err != nil {
		return nil, err
	}

	var res []string
	for i := 0; i < tpls.Len(); i++ {
		vals, err := tpls.At(i)
		if err != nil {
			return nil, err
		}
		if m.Matches(vals[0]) {
			res = append(res, vals[0])
		}
	}

	if len(res) == 0 {
		return index.EmptyPostings(), nil
	}

	var rit []index.Postings

	for _, v := range res {
		it, err := ix.Postings(m.Name(), v)
		if err != nil {
			return nil, err
		}
		rit = append(rit, it)
	}

	return index.Merge(rit...), nil
}

// inversePostingsForMatcher returns the postings for the series with the label name set but not matching the matcher.
func inversePostingsForMatcher(ix IndexReader, m labels.Matcher) (index.Postings, error) {
	tpls, err := ix.LabelValues(m.Name())
	if err != nil {
		return nil, err
	}

	var res []string
	for i := 0; i < tpls.Len(); i++ {
		vals, err := tpls.At(i)
		if err != nil {
			return nil, err
		}

		if !m.Matches(vals[0]) {
			res = append(res, vals[0])
		}
	}

	var rit []index.Postings
	for _, v := range res {
		it, err := ix.Postings(m.Name(), v)
		if err != nil {
			return nil, err
		}

		rit = append(rit, it)
	}

	return index.Merge(rit...), nil
}

func postingsForSetMatcher(ix IndexReader, name string, matches []string) (index.Postings, error) {
	var its []index.Postings
	for _, match := range matches {
		if it, err := ix.Postings(name, match); err == nil {
			its = append(its, it)
		} else {
			return nil, err
		}
	}
	return index.Merge(its...), nil
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

var emptySeriesSet = errSeriesSet{}

// EmptySeriesSet returns a series set that's always empty.
func EmptySeriesSet() SeriesSet {
	return emptySeriesSet
}

// mergedSeriesSet takes two series sets as a single series set. The input series sets
// must be sorted and sequential in time, i.e. if they have the same label set,
// the datapoints of a must be before the datapoints of b.
type mergedSeriesSet struct {
	a, b SeriesSet

	cur          Series
	adone, bdone bool
}

// NewMergedSeriesSet takes two series sets as a single series set. The input series sets
// must be sorted and sequential in time, i.e. if they have the same label set,
// the datapoints of a must be before the datapoints of b.
func NewMergedSeriesSet(a, b SeriesSet) SeriesSet {
	return newMergedSeriesSet(a, b)
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

type mergedVerticalSeriesSet struct {
	a, b         SeriesSet
	cur          Series
	adone, bdone bool
}

// NewMergedVerticalSeriesSet takes two series sets as a single series set.
// The input series sets must be sorted and
// the time ranges of the series can be overlapping.
func NewMergedVerticalSeriesSet(a, b SeriesSet) SeriesSet {
	return newMergedVerticalSeriesSet(a, b)
}

func newMergedVerticalSeriesSet(a, b SeriesSet) *mergedVerticalSeriesSet {
	s := &mergedVerticalSeriesSet{a: a, b: b}
	// Initialize first elements of both sets as Next() needs
	// one element look-ahead.
	s.adone = !s.a.Next()
	s.bdone = !s.b.Next()

	return s
}

func (s *mergedVerticalSeriesSet) At() Series {
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
		s.cur = &verticalChainedSeries{series: []Series{s.a.At(), s.b.At()}}
		s.adone = !s.a.Next()
		s.bdone = !s.b.Next()
	}
	return true
}

// ChunkSeriesSet exposes the chunks and intervals of a series instead of the
// actual series itself.
type ChunkSeriesSet interface {
	Next() bool
	At() (labels.Labels, []chunks.Meta, Intervals)
	Err() error
}

// baseChunkSeries loads the label set and chunk references for a postings
// list from an index. It filters out series that have labels set that should be unset.
type baseChunkSeries struct {
	p          index.Postings
	index      IndexReader
	tombstones TombstoneReader

	lset      labels.Labels
	chks      []chunks.Meta
	intervals Intervals
	err       error
}

// LookupChunkSeries retrieves all series for the given matchers and returns a ChunkSeriesSet
// over them. It drops chunks based on tombstones in the given reader.
func LookupChunkSeries(ir IndexReader, tr TombstoneReader, ms ...labels.Matcher) (ChunkSeriesSet, error) {
	if tr == nil {
		tr = newMemTombstones()
	}
	p, err := PostingsForMatchers(ir, ms...)
	if err != nil {
		return nil, err
	}
	return &baseChunkSeries{
		p:          p,
		index:      ir,
		tombstones: tr,
	}, nil
}

func (s *baseChunkSeries) At() (labels.Labels, []chunks.Meta, Intervals) {
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
			if errors.Cause(err) == ErrNotFound {
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
				if !(Interval{chk.MinTime, chk.MaxTime}.isSubrange(s.intervals)) {
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
	set        ChunkSeriesSet
	chunks     ChunkReader
	mint, maxt int64

	err       error
	chks      []chunks.Meta
	lset      labels.Labels
	intervals Intervals
}

func (s *populatedChunkSeries) At() (labels.Labels, []chunks.Meta, Intervals) {
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
				if s.err == ErrNotFound {
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
	set ChunkSeriesSet
	err error
	cur Series

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

func (s *blockSeriesSet) At() Series { return s.cur }
func (s *blockSeriesSet) Err() error { return s.err }

// chunkSeries is a series that is backed by a sequence of chunks holding
// time series data.
type chunkSeries struct {
	labels labels.Labels
	chunks []chunks.Meta // in-order chunk refs

	mint, maxt int64

	intervals Intervals
}

func (s *chunkSeries) Labels() labels.Labels {
	return s.labels
}

func (s *chunkSeries) Iterator() SeriesIterator {
	return newChunkSeriesIterator(s.chunks, s.intervals, s.mint, s.maxt)
}

// SeriesIterator iterates over the data of a time series.
type SeriesIterator interface {
	// Seek advances the iterator forward to the given timestamp.
	// If there's no value exactly at t, it advances to the first value
	// after t.
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
	return newChainedSeriesIterator(s.series...)
}

// chainedSeriesIterator implements a series iterater over a list
// of time-sorted, non-overlapping iterators.
type chainedSeriesIterator struct {
	series []Series // series in time order

	i   int
	cur SeriesIterator
}

func newChainedSeriesIterator(s ...Series) *chainedSeriesIterator {
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
	series []Series
}

func (s *verticalChainedSeries) Labels() labels.Labels {
	return s.series[0].Labels()
}

func (s *verticalChainedSeries) Iterator() SeriesIterator {
	return newVerticalMergeSeriesIterator(s.series...)
}

// verticalMergeSeriesIterator implements a series iterater over a list
// of time-sorted, time-overlapping iterators.
type verticalMergeSeriesIterator struct {
	a, b                  SeriesIterator
	aok, bok, initialized bool

	curT int64
	curV float64
}

func newVerticalMergeSeriesIterator(s ...Series) SeriesIterator {
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

	intervals Intervals
}

func newChunkSeriesIterator(cs []chunks.Meta, dranges Intervals, mint, maxt int64) *chunkSeriesIterator {
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

	intervals Intervals
}

func (it *deletedIterator) At() (int64, float64) {
	return it.it.At()
}

func (it *deletedIterator) Next() bool {
Outer:
	for it.it.Next() {
		ts, _ := it.it.At()

		for _, tr := range it.intervals {
			if tr.inBounds(ts) {
				continue Outer
			}

			if ts > tr.Maxt {
				it.intervals = it.intervals[1:]
				continue
			}

			return true
		}

		return true
	}

	return false
}

func (it *deletedIterator) Err() error {
	return it.it.Err()
}

type errSeriesSet struct {
	err error
}

func (s errSeriesSet) Next() bool { return false }
func (s errSeriesSet) At() Series { return nil }
func (s errSeriesSet) Err() error { return s.err }
