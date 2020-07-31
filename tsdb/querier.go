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
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

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

type blockBaseQuerier struct {
	index      IndexReader
	chunks     ChunkReader
	tombstones tombstones.Reader

	closed bool

	mint, maxt int64
}

func newBlockBaseQuerier(b BlockReader, mint, maxt int64) (*blockBaseQuerier, error) {
	indexr, err := b.Index()
	if err != nil {
		return nil, errors.Wrap(err, "open index reader")
	}
	chunkr, err := b.Chunks()
	if err != nil {
		indexr.Close()
		return nil, errors.Wrap(err, "open chunk reader")
	}
	tombsr, err := b.Tombstones()
	if err != nil {
		indexr.Close()
		chunkr.Close()
		return nil, errors.Wrap(err, "open tombstone reader")
	}

	if tombsr == nil {
		tombsr = tombstones.NewMemTombstones()
	}
	return &blockBaseQuerier{
		mint:       mint,
		maxt:       maxt,
		index:      indexr,
		chunks:     chunkr,
		tombstones: tombsr,
	}, nil
}

func (q *blockBaseQuerier) LabelValues(name string) ([]string, storage.Warnings, error) {
	res, err := q.index.SortedLabelValues(name)
	return res, nil, err
}

func (q *blockBaseQuerier) LabelNames() ([]string, storage.Warnings, error) {
	res, err := q.index.LabelNames()
	return res, nil, err
}

func (q *blockBaseQuerier) Close() error {
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

type blockQuerier struct {
	*blockBaseQuerier
}

// NewBlockQuerier returns a querier against the block reader and requested min and max time range.
func NewBlockQuerier(b BlockReader, mint, maxt int64) (storage.Querier, error) {
	q, err := newBlockBaseQuerier(b, mint, maxt)
	if err != nil {
		return nil, err
	}
	return &blockQuerier{blockBaseQuerier: q}, nil
}

func (q *blockQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.SeriesSet {
	mint := q.mint
	maxt := q.maxt
	if hints != nil {
		mint = hints.Start
		maxt = hints.End
	}

	p, err := PostingsForMatchers(q.index, ms...)
	if err != nil {
		return storage.ErrSeriesSet(err)
	}
	if sortSeries {
		p = q.index.SortedPostings(p)
	}
	return newBlockSeriesSet(q.index, q.chunks, q.tombstones, p, mint, maxt)
}

// blockChunkQuerier provides chunk querying access to a single block database.
type blockChunkQuerier struct {
	*blockBaseQuerier
}

// NewBlockChunkQuerier returns a chunk querier against the block reader and requested min and max time range.
func NewBlockChunkQuerier(b BlockReader, mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := newBlockBaseQuerier(b, mint, maxt)
	if err != nil {
		return nil, err
	}
	return &blockChunkQuerier{blockBaseQuerier: q}, nil
}

func (q *blockChunkQuerier) Select(sortSeries bool, hints *storage.SelectHints, ms ...*labels.Matcher) storage.ChunkSeriesSet {
	mint := q.mint
	maxt := q.maxt
	if hints != nil {
		mint = hints.Start
		maxt = hints.End
	}
	p, err := PostingsForMatchers(q.index, ms...)
	if err != nil {
		return storage.ErrChunkSeriesSet(err)
	}
	if sortSeries {
		p = q.index.SortedPostings(p)
	}
	return newBlockChunkSeriesSet(q.index, q.chunks, q.tombstones, p, mint, maxt)
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
