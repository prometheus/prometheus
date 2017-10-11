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
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
	"github.com/stretchr/testify/require"
)

type mockSeriesIterator struct {
	seek func(int64) bool
	at   func() (int64, float64)
	next func() bool
	err  func() error
}

func (m *mockSeriesIterator) Seek(t int64) bool    { return m.seek(t) }
func (m *mockSeriesIterator) At() (int64, float64) { return m.at() }
func (m *mockSeriesIterator) Next() bool           { return m.next() }
func (m *mockSeriesIterator) Err() error           { return m.err() }

type mockSeries struct {
	labels   func() labels.Labels
	iterator func() SeriesIterator
}

func newSeries(l map[string]string, s []sample) Series {
	return &mockSeries{
		labels:   func() labels.Labels { return labels.FromMap(l) },
		iterator: func() SeriesIterator { return newListSeriesIterator(s) },
	}
}
func (m *mockSeries) Labels() labels.Labels    { return m.labels() }
func (m *mockSeries) Iterator() SeriesIterator { return m.iterator() }

type listSeriesIterator struct {
	list []sample
	idx  int
}

func newListSeriesIterator(list []sample) *listSeriesIterator {
	return &listSeriesIterator{list: list, idx: -1}
}

func (it *listSeriesIterator) At() (int64, float64) {
	s := it.list[it.idx]
	return s.t, s.v
}

func (it *listSeriesIterator) Next() bool {
	it.idx++
	return it.idx < len(it.list)
}

func (it *listSeriesIterator) Seek(t int64) bool {
	if it.idx == -1 {
		it.idx = 0
	}
	// Do binary search between current position and end.
	it.idx = sort.Search(len(it.list)-it.idx, func(i int) bool {
		s := it.list[i+it.idx]
		return s.t >= t
	})

	return it.idx < len(it.list)
}

func (it *listSeriesIterator) Err() error {
	return nil
}

func TestMergedSeriesSet(t *testing.T) {

	cases := []struct {
		// The input sets in order (samples in series in b are strictly
		// after those in a).
		a, b SeriesSet
		// The composition of a and b in the partition series set must yield
		// results equivalent to the result series set.
		exp SeriesSet
	}{
		{
			a: newListSeriesSet([]Series{
				newSeries(map[string]string{
					"a": "a",
				}, []sample{
					{t: 1, v: 1},
				}),
			}),
			b: newListSeriesSet([]Series{
				newSeries(map[string]string{
					"a": "a",
				}, []sample{
					{t: 2, v: 2},
				}),
				newSeries(map[string]string{
					"b": "b",
				}, []sample{
					{t: 1, v: 1},
				}),
			}),
			exp: newListSeriesSet([]Series{
				newSeries(map[string]string{
					"a": "a",
				}, []sample{
					{t: 1, v: 1},
					{t: 2, v: 2},
				}),
				newSeries(map[string]string{
					"b": "b",
				}, []sample{
					{t: 1, v: 1},
				}),
			}),
		},
		{
			a: newListSeriesSet([]Series{
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "127.0.0.1:9090",
				}, []sample{
					{t: 1, v: 1},
				}),
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "localhost:9090",
				}, []sample{
					{t: 1, v: 2},
				}),
			}),
			b: newListSeriesSet([]Series{
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "127.0.0.1:9090",
				}, []sample{
					{t: 2, v: 1},
				}),
				newSeries(map[string]string{
					"handler":  "query",
					"instance": "localhost:9090",
				}, []sample{
					{t: 2, v: 2},
				}),
			}),
			exp: newListSeriesSet([]Series{
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "127.0.0.1:9090",
				}, []sample{
					{t: 1, v: 1},
					{t: 2, v: 1},
				}),
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "localhost:9090",
				}, []sample{
					{t: 1, v: 2},
				}),
				newSeries(map[string]string{
					"handler":  "query",
					"instance": "localhost:9090",
				}, []sample{
					{t: 2, v: 2},
				}),
			}),
		},
	}

Outer:
	for _, c := range cases {
		res := newMergedSeriesSet(c.a, c.b)

		for {
			eok, rok := c.exp.Next(), res.Next()
			require.Equal(t, eok, rok, "next")

			if !eok {
				continue Outer
			}
			sexp := c.exp.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels(), "labels")

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			require.Equal(t, errExp, errRes, "samples error")
			require.Equal(t, smplExp, smplRes, "samples")
		}
	}
}

func expandSeriesIterator(it SeriesIterator) (r []sample, err error) {
	for it.Next() {
		t, v := it.At()
		r = append(r, sample{t: t, v: v})
	}

	return r, it.Err()
}

// Index: labels -> postings -> chunkMetas -> chunkRef
// ChunkReader: ref -> vals
func createIdxChkReaders(tc []struct {
	lset   map[string]string
	chunks [][]sample
}) (IndexReader, ChunkReader) {
	sort.Slice(tc, func(i, j int) bool {
		return labels.Compare(labels.FromMap(tc[i].lset), labels.FromMap(tc[i].lset)) < 0
	})

	postings := newMemPostings()
	chkReader := mockChunkReader(make(map[uint64]chunks.Chunk))
	lblIdx := make(map[string]stringset)
	mi := newMockIndex()

	for i, s := range tc {
		i = i + 1 // 0 is not a valid posting.
		metas := make([]ChunkMeta, 0, len(s.chunks))
		for _, chk := range s.chunks {
			// Collisions can be there, but for tests, its fine.
			ref := rand.Uint64()

			metas = append(metas, ChunkMeta{
				MinTime: chk[0].t,
				MaxTime: chk[len(chk)-1].t,
				Ref:     ref,
			})

			chunk := chunks.NewXORChunk()
			app, _ := chunk.Appender()
			for _, smpl := range chk {
				app.Append(smpl.t, smpl.v)
			}
			chkReader[ref] = chunk
		}

		ls := labels.FromMap(s.lset)
		mi.AddSeries(uint64(i), ls, metas...)

		postings.add(uint64(i), ls)

		for _, l := range ls {
			vs, present := lblIdx[l.Name]
			if !present {
				vs = stringset{}
				lblIdx[l.Name] = vs
			}
			vs.set(l.Value)
		}
	}

	for l, vs := range lblIdx {
		mi.WriteLabelIndex([]string{l}, vs.slice())
	}

	for l := range postings.m {
		mi.WritePostings(l.Name, l.Value, postings.get(l.Name, l.Value))
	}

	return mi, chkReader
}

func TestBlockQuerier(t *testing.T) {
	newSeries := func(l map[string]string, s []sample) Series {
		return &mockSeries{
			labels:   func() labels.Labels { return labels.FromMap(l) },
			iterator: func() SeriesIterator { return newListSeriesIterator(s) },
		}
	}

	type query struct {
		mint, maxt int64
		ms         []labels.Matcher
		exp        SeriesSet
	}

	cases := struct {
		data []struct {
			lset   map[string]string
			chunks [][]sample
		}

		queries []query
	}{
		data: []struct {
			lset   map[string]string
			chunks [][]sample
		}{
			{
				lset: map[string]string{
					"a": "a",
				},
				chunks: [][]sample{
					{
						{1, 2}, {2, 3}, {3, 4},
					},
					{
						{5, 2}, {6, 3}, {7, 4},
					},
				},
			},
			{
				lset: map[string]string{
					"a": "a",
					"b": "b",
				},
				chunks: [][]sample{
					{
						{1, 1}, {2, 2}, {3, 3},
					},
					{
						{5, 3}, {6, 6},
					},
				},
			},
			{
				lset: map[string]string{
					"b": "b",
				},
				chunks: [][]sample{
					{
						{1, 3}, {2, 2}, {3, 6},
					},
					{
						{5, 1}, {6, 7}, {7, 2},
					},
				},
			},
			{
				lset: map[string]string{
					"p": "abcd",
					"x": "xyz",
				},
				chunks: [][]sample{
					{
						{1, 2}, {2, 3}, {3, 4},
					},
					{
						{5, 2}, {6, 3}, {7, 4},
					},
				},
			},
			{
				lset: map[string]string{
					"a": "ab",
					"p": "abce",
				},
				chunks: [][]sample{
					{
						{1, 1}, {2, 2}, {3, 3},
					},
					{
						{5, 3}, {6, 6},
					},
				},
			},
			{
				lset: map[string]string{
					"p": "xyz",
				},
				chunks: [][]sample{
					{
						{1, 1}, {2, 2}, {3, 3},
					},
					{
						{4, 4}, {5, 5}, {6, 6},
					},
				},
			},
		},

		queries: []query{
			{
				mint: 0,
				maxt: 0,
				ms:   []labels.Matcher{},
				exp:  newListSeriesSet([]Series{}),
			},
			{
				mint: 0,
				maxt: 0,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp:  newListSeriesSet([]Series{}),
			},
			{
				mint: 1,
				maxt: 0,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp:  newListSeriesSet([]Series{}),
			},
			{
				mint: 2,
				maxt: 6,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp: newListSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
					},
						[]sample{{2, 3}, {3, 4}, {5, 2}, {6, 3}},
					),
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]sample{{2, 2}, {3, 3}, {5, 3}, {6, 6}},
					),
				}),
			},
			{
				mint: 2,
				maxt: 6,
				ms:   []labels.Matcher{labels.NewPrefixMatcher("p", "abc")},
				exp: newListSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "ab",
						"p": "abce",
					},
						[]sample{{2, 2}, {3, 3}, {5, 3}, {6, 6}},
					),
					newSeries(map[string]string{
						"p": "abcd",
						"x": "xyz",
					},
						[]sample{{2, 3}, {3, 4}, {5, 2}, {6, 3}},
					),
				}),
			},
		},
	}

Outer:
	for i, c := range cases.queries {
		ir, cr := createIdxChkReaders(cases.data)
		querier := &blockQuerier{
			index:      ir,
			chunks:     cr,
			tombstones: newEmptyTombstoneReader(),

			mint: c.mint,
			maxt: c.maxt,
		}

		res := querier.Select(c.ms...)

		for {
			eok, rok := c.exp.Next(), res.Next()
			require.Equal(t, eok, rok, "%d: next", i)

			if !eok {
				continue Outer
			}
			sexp := c.exp.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels(), "%d: labels", i)

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			require.Equal(t, errExp, errRes, "%d: samples error", i)
			require.Equal(t, smplExp, smplRes, "%d: samples", i)
		}
	}

	return
}

func TestBlockQuerierDelete(t *testing.T) {
	newSeries := func(l map[string]string, s []sample) Series {
		return &mockSeries{
			labels:   func() labels.Labels { return labels.FromMap(l) },
			iterator: func() SeriesIterator { return newListSeriesIterator(s) },
		}
	}

	type query struct {
		mint, maxt int64
		ms         []labels.Matcher
		exp        SeriesSet
	}

	cases := struct {
		data []struct {
			lset   map[string]string
			chunks [][]sample
		}

		tombstones tombstoneReader
		queries    []query
	}{
		data: []struct {
			lset   map[string]string
			chunks [][]sample
		}{
			{
				lset: map[string]string{
					"a": "a",
				},
				chunks: [][]sample{
					{
						{1, 2}, {2, 3}, {3, 4},
					},
					{
						{5, 2}, {6, 3}, {7, 4},
					},
				},
			},
			{
				lset: map[string]string{
					"a": "a",
					"b": "b",
				},
				chunks: [][]sample{
					{
						{1, 1}, {2, 2}, {3, 3},
					},
					{
						{4, 15}, {5, 3}, {6, 6},
					},
				},
			},
			{
				lset: map[string]string{
					"b": "b",
				},
				chunks: [][]sample{
					{
						{1, 3}, {2, 2}, {3, 6},
					},
					{
						{5, 1}, {6, 7}, {7, 2},
					},
				},
			},
		},
		tombstones: newTombstoneReader(
			map[uint64]Intervals{
				1: Intervals{{1, 3}},
				2: Intervals{{1, 3}, {6, 10}},
				3: Intervals{{6, 10}},
			},
		),

		queries: []query{
			{
				mint: 2,
				maxt: 7,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp: newListSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
					},
						[]sample{{5, 2}, {6, 3}, {7, 4}},
					),
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]sample{{4, 15}, {5, 3}},
					),
				}),
			},
			{
				mint: 2,
				maxt: 7,
				ms:   []labels.Matcher{labels.NewEqualMatcher("b", "b")},
				exp: newListSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]sample{{4, 15}, {5, 3}},
					),
					newSeries(map[string]string{
						"b": "b",
					},
						[]sample{{2, 2}, {3, 6}, {5, 1}},
					),
				}),
			},
			{
				mint: 1,
				maxt: 4,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp: newListSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]sample{{4, 15}},
					),
				}),
			},
			{
				mint: 1,
				maxt: 3,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp:  newListSeriesSet([]Series{}),
			},
		},
	}

Outer:
	for _, c := range cases.queries {
		ir, cr := createIdxChkReaders(cases.data)
		querier := &blockQuerier{
			index:      ir,
			chunks:     cr,
			tombstones: cases.tombstones,

			mint: c.mint,
			maxt: c.maxt,
		}

		res := querier.Select(c.ms...)

		for {
			eok, rok := c.exp.Next(), res.Next()
			require.Equal(t, eok, rok, "next")

			if !eok {
				continue Outer
			}
			sexp := c.exp.At()
			sres := res.At()

			require.Equal(t, sexp.Labels(), sres.Labels(), "labels")

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			require.Equal(t, errExp, errRes, "samples error")
			require.Equal(t, smplExp, smplRes, "samples")
		}
	}

	return
}

func TestBaseChunkSeries(t *testing.T) {
	type refdSeries struct {
		lset   labels.Labels
		chunks []ChunkMeta

		ref uint64
	}

	cases := []struct {
		series []refdSeries
		// Postings should be in the sorted order of the the series
		postings []uint64

		expIdxs []int
	}{
		{
			series: []refdSeries{
				{
					lset: labels.New([]labels.Label{{"a", "a"}}...),
					chunks: []ChunkMeta{
						{Ref: 29}, {Ref: 45}, {Ref: 245}, {Ref: 123}, {Ref: 4232}, {Ref: 5344},
						{Ref: 121},
					},
					ref: 12,
				},
				{
					lset: labels.New([]labels.Label{{"a", "a"}, {"b", "b"}}...),
					chunks: []ChunkMeta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.New([]labels.Label{{"b", "c"}}...),
					chunks: []ChunkMeta{{Ref: 8282}},
					ref:    1,
				},
				{
					lset: labels.New([]labels.Label{{"b", "b"}}...),
					chunks: []ChunkMeta{
						{Ref: 829}, {Ref: 239}, {Ref: 2349}, {Ref: 659}, {Ref: 269},
					},
					ref: 108,
				},
			},
			postings: []uint64{12, 13, 10, 108}, // 13 doesn't exist and should just be skipped over.
			expIdxs:  []int{0, 1, 3},
		},
		{
			series: []refdSeries{
				{
					lset: labels.New([]labels.Label{{"a", "a"}, {"b", "b"}}...),
					chunks: []ChunkMeta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.New([]labels.Label{{"b", "c"}}...),
					chunks: []ChunkMeta{{Ref: 8282}},
					ref:    3,
				},
			},
			postings: []uint64{},
			expIdxs:  []int{},
		},
	}

	for _, tc := range cases {
		mi := newMockIndex()
		for _, s := range tc.series {
			mi.AddSeries(s.ref, s.lset, s.chunks...)
		}

		bcs := &baseChunkSeries{
			p:          newListPostings(tc.postings),
			index:      mi,
			tombstones: newEmptyTombstoneReader(),
		}

		i := 0
		for bcs.Next() {
			lset, chks, _ := bcs.At()

			idx := tc.expIdxs[i]

			require.Equal(t, tc.series[idx].lset, lset)
			require.Equal(t, tc.series[idx].chunks, chks)

			i++
		}
		require.Equal(t, len(tc.expIdxs), i)
		require.NoError(t, bcs.Err())
	}

	return
}

// TODO: Remove after simpleSeries is merged
type itSeries struct {
	si SeriesIterator
}

func (s itSeries) Iterator() SeriesIterator { return s.si }
func (s itSeries) Labels() labels.Labels    { return labels.Labels{} }

func chunkFromSamples(s []sample) ChunkMeta {
	mint, maxt := int64(0), int64(0)

	if len(s) > 0 {
		mint, maxt = s[0].t, s[len(s)-1].t
	}

	c := chunks.NewXORChunk()
	ca, _ := c.Appender()

	for _, s := range s {
		ca.Append(s.t, s.v)
	}
	return ChunkMeta{
		MinTime: mint,
		MaxTime: maxt,
		Chunk:   c,
	}
}

func TestSeriesIterator(t *testing.T) {
	itcases := []struct {
		a, b, c []sample
		exp     []sample

		mint, maxt int64
	}{
		{
			a: []sample{},
			b: []sample{},
			c: []sample{},

			exp: []sample{},

			mint: math.MinInt64,
			maxt: math.MaxInt64,
		},
		{
			a: []sample{
				{1, 2}, {2, 3}, {3, 5}, {6, 1},
			},
			b: []sample{},
			c: []sample{
				{7, 89}, {9, 8},
			},

			exp: []sample{
				{1, 2}, {2, 3}, {3, 5}, {6, 1}, {7, 89}, {9, 8},
			},
			mint: math.MinInt64,
			maxt: math.MaxInt64,
		},
		{
			a: []sample{},
			b: []sample{
				{1, 2}, {2, 3}, {3, 5}, {6, 1},
			},
			c: []sample{
				{7, 89}, {9, 8},
			},

			exp: []sample{
				{1, 2}, {2, 3}, {3, 5}, {6, 1}, {7, 89}, {9, 8},
			},
			mint: 2,
			maxt: 8,
		},
		{
			a: []sample{
				{1, 2}, {2, 3}, {3, 5}, {6, 1},
			},
			b: []sample{
				{7, 89}, {9, 8},
			},
			c: []sample{
				{10, 22}, {203, 3493},
			},

			exp: []sample{
				{1, 2}, {2, 3}, {3, 5}, {6, 1}, {7, 89}, {9, 8}, {10, 22}, {203, 3493},
			},
			mint: 6,
			maxt: 10,
		},
	}

	seekcases := []struct {
		a, b, c []sample

		seek    int64
		success bool
		exp     []sample

		mint, maxt int64
	}{
		{
			a: []sample{},
			b: []sample{},
			c: []sample{},

			seek:    0,
			success: false,
			exp:     nil,
		},
		{
			a: []sample{
				{2, 3},
			},
			b: []sample{},
			c: []sample{
				{7, 89}, {9, 8},
			},

			seek:    10,
			success: false,
			exp:     nil,
			mint:    math.MinInt64,
			maxt:    math.MaxInt64,
		},
		{
			a: []sample{},
			b: []sample{
				{1, 2}, {3, 5}, {6, 1},
			},
			c: []sample{
				{7, 89}, {9, 8},
			},

			seek:    2,
			success: true,
			exp: []sample{
				{3, 5}, {6, 1}, {7, 89}, {9, 8},
			},
			mint: 5,
			maxt: 8,
		},
		{
			a: []sample{
				{6, 1},
			},
			b: []sample{
				{9, 8},
			},
			c: []sample{
				{10, 22}, {203, 3493},
			},

			seek:    10,
			success: true,
			exp: []sample{
				{10, 22}, {203, 3493},
			},
			mint: 10,
			maxt: 203,
		},
		{
			a: []sample{
				{6, 1},
			},
			b: []sample{
				{9, 8},
			},
			c: []sample{
				{10, 22}, {203, 3493},
			},

			seek:    203,
			success: true,
			exp: []sample{
				{203, 3493},
			},
			mint: 7,
			maxt: 203,
		},
	}

	t.Run("Chunk", func(t *testing.T) {
		for _, tc := range itcases {
			chkMetas := []ChunkMeta{
				chunkFromSamples(tc.a),
				chunkFromSamples(tc.b),
				chunkFromSamples(tc.c),
			}
			res := newChunkSeriesIterator(chkMetas, nil, tc.mint, tc.maxt)

			smplValid := make([]sample, 0)
			for _, s := range tc.exp {
				if s.t >= tc.mint && s.t <= tc.maxt {
					smplValid = append(smplValid, s)
				}
			}
			exp := newListSeriesIterator(smplValid)

			smplExp, errExp := expandSeriesIterator(exp)
			smplRes, errRes := expandSeriesIterator(res)

			require.Equal(t, errExp, errRes, "samples error")
			require.Equal(t, smplExp, smplRes, "samples")
		}

		t.Run("Seek", func(t *testing.T) {
			extra := []struct {
				a, b, c []sample

				seek    int64
				success bool
				exp     []sample

				mint, maxt int64
			}{
				{
					a: []sample{
						{6, 1},
					},
					b: []sample{
						{9, 8},
					},
					c: []sample{
						{10, 22}, {203, 3493},
					},

					seek:    203,
					success: false,
					exp:     nil,
					mint:    2,
					maxt:    202,
				},
				{
					a: []sample{
						{6, 1},
					},
					b: []sample{
						{9, 8},
					},
					c: []sample{
						{10, 22}, {203, 3493},
					},

					seek:    5,
					success: true,
					exp:     []sample{{10, 22}},
					mint:    10,
					maxt:    202,
				},
			}

			seekcases2 := append(seekcases, extra...)

			for _, tc := range seekcases2 {
				chkMetas := []ChunkMeta{
					chunkFromSamples(tc.a),
					chunkFromSamples(tc.b),
					chunkFromSamples(tc.c),
				}
				res := newChunkSeriesIterator(chkMetas, nil, tc.mint, tc.maxt)

				smplValid := make([]sample, 0)
				for _, s := range tc.exp {
					if s.t >= tc.mint && s.t <= tc.maxt {
						smplValid = append(smplValid, s)
					}
				}
				exp := newListSeriesIterator(smplValid)

				require.Equal(t, tc.success, res.Seek(tc.seek))

				if tc.success {
					// Init the list and then proceed to check.
					remaining := exp.Next()
					require.True(t, remaining)

					for remaining {
						sExp, eExp := exp.At()
						sRes, eRes := res.At()
						require.Equal(t, eExp, eRes, "samples error")
						require.Equal(t, sExp, sRes, "samples")

						remaining = exp.Next()
						require.Equal(t, remaining, res.Next())
					}
				}
			}
		})
	})

	t.Run("Chain", func(t *testing.T) {
		for _, tc := range itcases {
			a, b, c := itSeries{newListSeriesIterator(tc.a)},
				itSeries{newListSeriesIterator(tc.b)},
				itSeries{newListSeriesIterator(tc.c)}

			res := newChainedSeriesIterator(a, b, c)
			exp := newListSeriesIterator(tc.exp)

			smplExp, errExp := expandSeriesIterator(exp)
			smplRes, errRes := expandSeriesIterator(res)

			require.Equal(t, errExp, errRes, "samples error")
			require.Equal(t, smplExp, smplRes, "samples")
		}

		t.Run("Seek", func(t *testing.T) {
			for _, tc := range seekcases {
				a, b, c := itSeries{newListSeriesIterator(tc.a)},
					itSeries{newListSeriesIterator(tc.b)},
					itSeries{newListSeriesIterator(tc.c)}

				res := newChainedSeriesIterator(a, b, c)
				exp := newListSeriesIterator(tc.exp)

				require.Equal(t, tc.success, res.Seek(tc.seek))

				if tc.success {
					// Init the list and then proceed to check.
					remaining := exp.Next()
					require.True(t, remaining)

					for remaining {
						sExp, eExp := exp.At()
						sRes, eRes := res.At()
						require.Equal(t, eExp, eRes, "samples error")
						require.Equal(t, sExp, sRes, "samples")

						remaining = exp.Next()
						require.Equal(t, remaining, res.Next())
					}
				}
			}
		})
	})

	return
}

// Regression for: https://github.com/prometheus/tsdb/pull/97
func TestChunkSeriesIterator_DoubleSeek(t *testing.T) {
	chkMetas := []ChunkMeta{
		chunkFromSamples([]sample{}),
		chunkFromSamples([]sample{{1, 1}, {2, 2}, {3, 3}}),
		chunkFromSamples([]sample{{4, 4}, {5, 5}}),
	}

	res := newChunkSeriesIterator(chkMetas, nil, 2, 8)
	require.True(t, res.Seek(1))
	require.True(t, res.Seek(2))
	ts, v := res.At()
	require.Equal(t, int64(2), ts)
	require.Equal(t, float64(2), v)
}

// Regression when seeked chunks were still found via binary search and we always
// skipped to the end when seeking a value in the current chunk.
func TestChunkSeriesIterator_SeekInCurrentChunk(t *testing.T) {
	metas := []ChunkMeta{
		chunkFromSamples([]sample{}),
		chunkFromSamples([]sample{{1, 2}, {3, 4}, {5, 6}, {7, 8}}),
		chunkFromSamples([]sample{}),
	}

	it := newChunkSeriesIterator(metas, nil, 1, 7)

	require.True(t, it.Next())
	ts, v := it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, float64(2), v)

	require.True(t, it.Seek(4))
	ts, v = it.At()
	require.Equal(t, int64(5), ts)
	require.Equal(t, float64(6), v)
}

// Regression when calling Next() with a time bounded to fit within two samples.
// Seek gets called and advances beyond the max time, which was just accepted as a valid sample.
func TestChunkSeriesIterator_NextWithMinTime(t *testing.T) {
	metas := []ChunkMeta{
		chunkFromSamples([]sample{{1, 6}, {5, 6}, {7, 8}}),
	}

	it := newChunkSeriesIterator(metas, nil, 2, 4)
	require.False(t, it.Next())
}

func TestPopulatedCSReturnsValidChunkSlice(t *testing.T) {
	lbls := []labels.Labels{labels.New(labels.Label{"a", "b"})}
	chunkMetas := [][]ChunkMeta{
		{
			{MinTime: 1, MaxTime: 2, Ref: 1},
			{MinTime: 3, MaxTime: 4, Ref: 2},
			{MinTime: 10, MaxTime: 12, Ref: 3},
		},
	}

	cr := mockChunkReader(
		map[uint64]chunks.Chunk{
			1: chunks.NewXORChunk(),
			2: chunks.NewXORChunk(),
			3: chunks.NewXORChunk(),
		},
	)

	m := &mockChunkSeriesSet{l: lbls, cm: chunkMetas, i: -1}
	p := &populatedChunkSeries{
		set:    m,
		chunks: cr,

		mint: 0,
		maxt: 0,
	}

	require.False(t, p.Next())

	p.mint = 6
	p.maxt = 9
	require.False(t, p.Next())

	// Test the case where 1 chunk could cause an unpopulated chunk to be returned.
	chunkMetas = [][]ChunkMeta{
		{
			{MinTime: 1, MaxTime: 2, Ref: 1},
		},
	}

	m = &mockChunkSeriesSet{l: lbls, cm: chunkMetas, i: -1}
	p = &populatedChunkSeries{
		set:    m,
		chunks: cr,

		mint: 10,
		maxt: 15,
	}
	require.False(t, p.Next())
	return
}

type mockChunkSeriesSet struct {
	l  []labels.Labels
	cm [][]ChunkMeta

	i int
}

func (m *mockChunkSeriesSet) Next() bool {
	if len(m.l) != len(m.cm) {
		return false
	}
	m.i++
	return m.i < len(m.l)
}

func (m *mockChunkSeriesSet) At() (labels.Labels, []ChunkMeta, Intervals) {
	return m.l[m.i], m.cm[m.i], nil
}

func (m *mockChunkSeriesSet) Err() error {
	return nil
}

// Test the cost of merging series sets for different number of merged sets and their size.
// The subset are all equivalent so this does not capture merging of partial or non-overlapping sets well.
func BenchmarkMergedSeriesSet(b *testing.B) {
	var sel func(sets []SeriesSet) SeriesSet

	sel = func(sets []SeriesSet) SeriesSet {
		if len(sets) == 0 {
			return nopSeriesSet{}
		}
		if len(sets) == 1 {
			return sets[0]
		}
		l := len(sets) / 2
		return newMergedSeriesSet(sel(sets[:l]), sel(sets[l:]))
	}

	for _, k := range []int{
		100,
		1000,
		10000,
		100000,
	} {
		for _, j := range []int{1, 2, 4, 8, 16, 32} {
			b.Run(fmt.Sprintf("series=%d,blocks=%d", k, j), func(b *testing.B) {
				lbls, err := readPrometheusLabels("testdata/1m.series", k)
				require.NoError(b, err)

				sort.Sort(labels.Slice(lbls))

				in := make([][]Series, j)

				for _, l := range lbls {
					l2 := l
					for j := range in {
						in[j] = append(in[j], &mockSeries{labels: func() labels.Labels { return l2 }})
					}
				}

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					var sets []SeriesSet
					for _, s := range in {
						sets = append(sets, newListSeriesSet(s))
					}
					ms := sel(sets)

					i := 0
					for ms.Next() {
						i++
					}
					require.NoError(b, ms.Err())
					require.Equal(b, len(lbls), i)
				}
			})
		}
	}
}
