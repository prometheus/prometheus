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
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/chunkenc"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/index"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
	"github.com/prometheus/tsdb/tsdbutil"
)

type mockSeriesSet struct {
	next   func() bool
	series func() Series
	err    func() error
}

func (m *mockSeriesSet) Next() bool { return m.next() }
func (m *mockSeriesSet) At() Series { return m.series() }
func (m *mockSeriesSet) Err() error { return m.err() }

func newMockSeriesSet(list []Series) *mockSeriesSet {
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
			a: newMockSeriesSet([]Series{
				newSeries(map[string]string{
					"a": "a",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 1},
				}),
			}),
			b: newMockSeriesSet([]Series{
				newSeries(map[string]string{
					"a": "a",
				}, []tsdbutil.Sample{
					sample{t: 2, v: 2},
				}),
				newSeries(map[string]string{
					"b": "b",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 1},
				}),
			}),
			exp: newMockSeriesSet([]Series{
				newSeries(map[string]string{
					"a": "a",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 1},
					sample{t: 2, v: 2},
				}),
				newSeries(map[string]string{
					"b": "b",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 1},
				}),
			}),
		},
		{
			a: newMockSeriesSet([]Series{
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "127.0.0.1:9090",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 1},
				}),
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "localhost:9090",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 2},
				}),
			}),
			b: newMockSeriesSet([]Series{
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "127.0.0.1:9090",
				}, []tsdbutil.Sample{
					sample{t: 2, v: 1},
				}),
				newSeries(map[string]string{
					"handler":  "query",
					"instance": "localhost:9090",
				}, []tsdbutil.Sample{
					sample{t: 2, v: 2},
				}),
			}),
			exp: newMockSeriesSet([]Series{
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "127.0.0.1:9090",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 1},
					sample{t: 2, v: 1},
				}),
				newSeries(map[string]string{
					"handler":  "prometheus",
					"instance": "localhost:9090",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 2},
				}),
				newSeries(map[string]string{
					"handler":  "query",
					"instance": "localhost:9090",
				}, []tsdbutil.Sample{
					sample{t: 2, v: 2},
				}),
			}),
		},
	}

Outer:
	for _, c := range cases {
		res := newMergedSeriesSet(c.a, c.b)

		for {
			eok, rok := c.exp.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				continue Outer
			}
			sexp := c.exp.At()
			sres := res.At()

			testutil.Equals(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}
	}
}

func expandSeriesIterator(it SeriesIterator) (r []tsdbutil.Sample, err error) {
	for it.Next() {
		t, v := it.At()
		r = append(r, sample{t: t, v: v})
	}

	return r, it.Err()
}

type seriesSamples struct {
	lset   map[string]string
	chunks [][]sample
}

// Index: labels -> postings -> chunkMetas -> chunkRef
// ChunkReader: ref -> vals
func createIdxChkReaders(tc []seriesSamples) (IndexReader, ChunkReader, int64, int64) {
	sort.Slice(tc, func(i, j int) bool {
		return labels.Compare(labels.FromMap(tc[i].lset), labels.FromMap(tc[i].lset)) < 0
	})

	postings := index.NewMemPostings()
	chkReader := mockChunkReader(make(map[uint64]chunkenc.Chunk))
	lblIdx := make(map[string]stringset)
	mi := newMockIndex()
	blockMint := int64(math.MaxInt64)
	blockMaxt := int64(math.MinInt64)

	for i, s := range tc {
		i = i + 1 // 0 is not a valid posting.
		metas := make([]chunks.Meta, 0, len(s.chunks))
		for _, chk := range s.chunks {
			// Collisions can be there, but for tests, its fine.
			ref := rand.Uint64()

			if chk[0].t < blockMint {
				blockMint = chk[0].t
			}
			if chk[len(chk)-1].t > blockMaxt {
				blockMaxt = chk[len(chk)-1].t
			}

			metas = append(metas, chunks.Meta{
				MinTime: chk[0].t,
				MaxTime: chk[len(chk)-1].t,
				Ref:     ref,
			})

			chunk := chunkenc.NewXORChunk()
			app, _ := chunk.Appender()
			for _, smpl := range chk {
				app.Append(smpl.t, smpl.v)
			}
			chkReader[ref] = chunk
		}

		ls := labels.FromMap(s.lset)
		mi.AddSeries(uint64(i), ls, metas...)

		postings.Add(uint64(i), ls)

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

	postings.Iter(func(l labels.Label, p index.Postings) error {
		return mi.WritePostings(l.Name, l.Value, p)
	})

	return mi, chkReader, blockMint, blockMaxt
}

func TestBlockQuerier(t *testing.T) {
	newSeries := func(l map[string]string, s []tsdbutil.Sample) Series {
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
		data []seriesSamples

		queries []query
	}{
		data: []seriesSamples{
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
		},

		queries: []query{
			{
				mint: 0,
				maxt: 0,
				ms:   []labels.Matcher{},
				exp:  newMockSeriesSet([]Series{}),
			},
			{
				mint: 0,
				maxt: 0,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp:  newMockSeriesSet([]Series{}),
			},
			{
				mint: 1,
				maxt: 0,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp:  newMockSeriesSet([]Series{}),
			},
			{
				mint: 2,
				maxt: 6,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp: newMockSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
					},
						[]tsdbutil.Sample{sample{2, 3}, sample{3, 4}, sample{5, 2}, sample{6, 3}},
					),
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{5, 3}, sample{6, 6}},
					),
				}),
			},
		},
	}

Outer:
	for _, c := range cases.queries {
		ir, cr, _, _ := createIdxChkReaders(cases.data)
		querier := &blockQuerier{
			index:      ir,
			chunks:     cr,
			tombstones: newMemTombstones(),

			mint: c.mint,
			maxt: c.maxt,
		}

		res, err := querier.Select(c.ms...)
		testutil.Ok(t, err)

		for {
			eok, rok := c.exp.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				continue Outer
			}
			sexp := c.exp.At()
			sres := res.At()

			testutil.Equals(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}
	}
}

func TestBlockQuerierDelete(t *testing.T) {
	newSeries := func(l map[string]string, s []tsdbutil.Sample) Series {
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
		data []seriesSamples

		tombstones TombstoneReader
		queries    []query
	}{
		data: []seriesSamples{
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
		tombstones: &memTombstones{intvlGroups: map[uint64]Intervals{
			1: Intervals{{1, 3}},
			2: Intervals{{1, 3}, {6, 10}},
			3: Intervals{{6, 10}},
		}},
		queries: []query{
			{
				mint: 2,
				maxt: 7,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp: newMockSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
					},
						[]tsdbutil.Sample{sample{5, 2}, sample{6, 3}, sample{7, 4}},
					),
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]tsdbutil.Sample{sample{4, 15}, sample{5, 3}},
					),
				}),
			},
			{
				mint: 2,
				maxt: 7,
				ms:   []labels.Matcher{labels.NewEqualMatcher("b", "b")},
				exp: newMockSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]tsdbutil.Sample{sample{4, 15}, sample{5, 3}},
					),
					newSeries(map[string]string{
						"b": "b",
					},
						[]tsdbutil.Sample{sample{2, 2}, sample{3, 6}, sample{5, 1}},
					),
				}),
			},
			{
				mint: 1,
				maxt: 4,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp: newMockSeriesSet([]Series{
					newSeries(map[string]string{
						"a": "a",
						"b": "b",
					},
						[]tsdbutil.Sample{sample{4, 15}},
					),
				}),
			},
			{
				mint: 1,
				maxt: 3,
				ms:   []labels.Matcher{labels.NewEqualMatcher("a", "a")},
				exp:  newMockSeriesSet([]Series{}),
			},
		},
	}

Outer:
	for _, c := range cases.queries {
		ir, cr, _, _ := createIdxChkReaders(cases.data)
		querier := &blockQuerier{
			index:      ir,
			chunks:     cr,
			tombstones: cases.tombstones,

			mint: c.mint,
			maxt: c.maxt,
		}

		res, err := querier.Select(c.ms...)
		testutil.Ok(t, err)

		for {
			eok, rok := c.exp.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				continue Outer
			}
			sexp := c.exp.At()
			sres := res.At()

			testutil.Equals(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := expandSeriesIterator(sexp.Iterator())
			smplRes, errRes := expandSeriesIterator(sres.Iterator())

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}
	}
}

func TestBaseChunkSeries(t *testing.T) {
	type refdSeries struct {
		lset   labels.Labels
		chunks []chunks.Meta

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
					chunks: []chunks.Meta{
						{Ref: 29}, {Ref: 45}, {Ref: 245}, {Ref: 123}, {Ref: 4232}, {Ref: 5344},
						{Ref: 121},
					},
					ref: 12,
				},
				{
					lset: labels.New([]labels.Label{{"a", "a"}, {"b", "b"}}...),
					chunks: []chunks.Meta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.New([]labels.Label{{"b", "c"}}...),
					chunks: []chunks.Meta{{Ref: 8282}},
					ref:    1,
				},
				{
					lset: labels.New([]labels.Label{{"b", "b"}}...),
					chunks: []chunks.Meta{
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
					chunks: []chunks.Meta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.New([]labels.Label{{"b", "c"}}...),
					chunks: []chunks.Meta{{Ref: 8282}},
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
			p:          index.NewListPostings(tc.postings),
			index:      mi,
			tombstones: newMemTombstones(),
		}

		i := 0
		for bcs.Next() {
			lset, chks, _ := bcs.At()

			idx := tc.expIdxs[i]

			testutil.Equals(t, tc.series[idx].lset, lset)
			testutil.Equals(t, tc.series[idx].chunks, chks)

			i++
		}
		testutil.Equals(t, len(tc.expIdxs), i)
		testutil.Ok(t, bcs.Err())
	}
}

// TODO: Remove after simpleSeries is merged
type itSeries struct {
	si SeriesIterator
}

func (s itSeries) Iterator() SeriesIterator { return s.si }
func (s itSeries) Labels() labels.Labels    { return labels.Labels{} }

func TestSeriesIterator(t *testing.T) {
	itcases := []struct {
		a, b, c []tsdbutil.Sample
		exp     []tsdbutil.Sample

		mint, maxt int64
	}{
		{
			a: []tsdbutil.Sample{},
			b: []tsdbutil.Sample{},
			c: []tsdbutil.Sample{},

			exp: []tsdbutil.Sample{},

			mint: math.MinInt64,
			maxt: math.MaxInt64,
		},
		{
			a: []tsdbutil.Sample{
				sample{1, 2},
				sample{2, 3},
				sample{3, 5},
				sample{6, 1},
			},
			b: []tsdbutil.Sample{},
			c: []tsdbutil.Sample{
				sample{7, 89}, sample{9, 8},
			},

			exp: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8},
			},
			mint: math.MinInt64,
			maxt: math.MaxInt64,
		},
		{
			a: []tsdbutil.Sample{},
			b: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1},
			},
			c: []tsdbutil.Sample{
				sample{7, 89}, sample{9, 8},
			},

			exp: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8},
			},
			mint: 2,
			maxt: 8,
		},
		{
			a: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1},
			},
			b: []tsdbutil.Sample{
				sample{7, 89}, sample{9, 8},
			},
			c: []tsdbutil.Sample{
				sample{10, 22}, sample{203, 3493},
			},

			exp: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8}, sample{10, 22}, sample{203, 3493},
			},
			mint: 6,
			maxt: 10,
		},
	}

	seekcases := []struct {
		a, b, c []tsdbutil.Sample

		seek    int64
		success bool
		exp     []tsdbutil.Sample

		mint, maxt int64
	}{
		{
			a: []tsdbutil.Sample{},
			b: []tsdbutil.Sample{},
			c: []tsdbutil.Sample{},

			seek:    0,
			success: false,
			exp:     nil,
		},
		{
			a: []tsdbutil.Sample{
				sample{2, 3},
			},
			b: []tsdbutil.Sample{},
			c: []tsdbutil.Sample{
				sample{7, 89}, sample{9, 8},
			},

			seek:    10,
			success: false,
			exp:     nil,
			mint:    math.MinInt64,
			maxt:    math.MaxInt64,
		},
		{
			a: []tsdbutil.Sample{},
			b: []tsdbutil.Sample{
				sample{1, 2}, sample{3, 5}, sample{6, 1},
			},
			c: []tsdbutil.Sample{
				sample{7, 89}, sample{9, 8},
			},

			seek:    2,
			success: true,
			exp: []tsdbutil.Sample{
				sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8},
			},
			mint: 5,
			maxt: 8,
		},
		{
			a: []tsdbutil.Sample{
				sample{6, 1},
			},
			b: []tsdbutil.Sample{
				sample{9, 8},
			},
			c: []tsdbutil.Sample{
				sample{10, 22}, sample{203, 3493},
			},

			seek:    10,
			success: true,
			exp: []tsdbutil.Sample{
				sample{10, 22}, sample{203, 3493},
			},
			mint: 10,
			maxt: 203,
		},
		{
			a: []tsdbutil.Sample{
				sample{6, 1},
			},
			b: []tsdbutil.Sample{
				sample{9, 8},
			},
			c: []tsdbutil.Sample{
				sample{10, 22}, sample{203, 3493},
			},

			seek:    203,
			success: true,
			exp: []tsdbutil.Sample{
				sample{203, 3493},
			},
			mint: 7,
			maxt: 203,
		},
	}

	t.Run("Chunk", func(t *testing.T) {
		for _, tc := range itcases {
			chkMetas := []chunks.Meta{
				tsdbutil.ChunkFromSamples(tc.a),
				tsdbutil.ChunkFromSamples(tc.b),
				tsdbutil.ChunkFromSamples(tc.c),
			}
			res := newChunkSeriesIterator(chkMetas, nil, tc.mint, tc.maxt)

			smplValid := make([]tsdbutil.Sample, 0)
			for _, s := range tc.exp {
				if s.T() >= tc.mint && s.T() <= tc.maxt {
					smplValid = append(smplValid, tsdbutil.Sample(s))
				}
			}
			exp := newListSeriesIterator(smplValid)

			smplExp, errExp := expandSeriesIterator(exp)
			smplRes, errRes := expandSeriesIterator(res)

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}

		t.Run("Seek", func(t *testing.T) {
			extra := []struct {
				a, b, c []tsdbutil.Sample

				seek    int64
				success bool
				exp     []tsdbutil.Sample

				mint, maxt int64
			}{
				{
					a: []tsdbutil.Sample{
						sample{6, 1},
					},
					b: []tsdbutil.Sample{
						sample{9, 8},
					},
					c: []tsdbutil.Sample{
						sample{10, 22}, sample{203, 3493},
					},

					seek:    203,
					success: false,
					exp:     nil,
					mint:    2,
					maxt:    202,
				},
				{
					a: []tsdbutil.Sample{
						sample{6, 1},
					},
					b: []tsdbutil.Sample{
						sample{9, 8},
					},
					c: []tsdbutil.Sample{
						sample{10, 22}, sample{203, 3493},
					},

					seek:    5,
					success: true,
					exp:     []tsdbutil.Sample{sample{10, 22}},
					mint:    10,
					maxt:    202,
				},
			}

			seekcases2 := append(seekcases, extra...)

			for _, tc := range seekcases2 {
				chkMetas := []chunks.Meta{
					tsdbutil.ChunkFromSamples(tc.a),
					tsdbutil.ChunkFromSamples(tc.b),
					tsdbutil.ChunkFromSamples(tc.c),
				}
				res := newChunkSeriesIterator(chkMetas, nil, tc.mint, tc.maxt)

				smplValid := make([]tsdbutil.Sample, 0)
				for _, s := range tc.exp {
					if s.T() >= tc.mint && s.T() <= tc.maxt {
						smplValid = append(smplValid, tsdbutil.Sample(s))
					}
				}
				exp := newListSeriesIterator(smplValid)

				testutil.Equals(t, tc.success, res.Seek(tc.seek))

				if tc.success {
					// Init the list and then proceed to check.
					remaining := exp.Next()
					testutil.Assert(t, remaining == true, "")

					for remaining {
						sExp, eExp := exp.At()
						sRes, eRes := res.At()
						testutil.Equals(t, eExp, eRes)
						testutil.Equals(t, sExp, sRes)

						remaining = exp.Next()
						testutil.Equals(t, remaining, res.Next())
					}
				}
			}
		})
	})

	t.Run("Chain", func(t *testing.T) {
		// Extra cases for overlapping series.
		itcasesExtra := []struct {
			a, b, c    []tsdbutil.Sample
			exp        []tsdbutil.Sample
			mint, maxt int64
		}{
			{
				a: []tsdbutil.Sample{
					sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1},
				},
				b: []tsdbutil.Sample{
					sample{5, 49}, sample{7, 89}, sample{9, 8},
				},
				c: []tsdbutil.Sample{
					sample{2, 33}, sample{4, 44}, sample{10, 3},
				},

				exp: []tsdbutil.Sample{
					sample{1, 2}, sample{2, 33}, sample{3, 5}, sample{4, 44}, sample{5, 49}, sample{6, 1}, sample{7, 89}, sample{9, 8}, sample{10, 3},
				},
				mint: math.MinInt64,
				maxt: math.MaxInt64,
			},
			{
				a: []tsdbutil.Sample{
					sample{1, 2}, sample{2, 3}, sample{9, 5}, sample{13, 1},
				},
				b: []tsdbutil.Sample{},
				c: []tsdbutil.Sample{
					sample{1, 23}, sample{2, 342}, sample{3, 25}, sample{6, 11},
				},

				exp: []tsdbutil.Sample{
					sample{1, 23}, sample{2, 342}, sample{3, 25}, sample{6, 11}, sample{9, 5}, sample{13, 1},
				},
				mint: math.MinInt64,
				maxt: math.MaxInt64,
			},
		}

		for _, tc := range itcases {
			a, b, c := itSeries{newListSeriesIterator(tc.a)},
				itSeries{newListSeriesIterator(tc.b)},
				itSeries{newListSeriesIterator(tc.c)}

			res := newChainedSeriesIterator(a, b, c)
			exp := newListSeriesIterator([]tsdbutil.Sample(tc.exp))

			smplExp, errExp := expandSeriesIterator(exp)
			smplRes, errRes := expandSeriesIterator(res)

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}

		for _, tc := range append(itcases, itcasesExtra...) {
			a, b, c := itSeries{newListSeriesIterator(tc.a)},
				itSeries{newListSeriesIterator(tc.b)},
				itSeries{newListSeriesIterator(tc.c)}

			res := newVerticalMergeSeriesIterator(a, b, c)
			exp := newListSeriesIterator([]tsdbutil.Sample(tc.exp))

			smplExp, errExp := expandSeriesIterator(exp)
			smplRes, errRes := expandSeriesIterator(res)

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}

		t.Run("Seek", func(t *testing.T) {
			for _, tc := range seekcases {
				ress := []SeriesIterator{
					newChainedSeriesIterator(
						itSeries{newListSeriesIterator(tc.a)},
						itSeries{newListSeriesIterator(tc.b)},
						itSeries{newListSeriesIterator(tc.c)},
					),
					newVerticalMergeSeriesIterator(
						itSeries{newListSeriesIterator(tc.a)},
						itSeries{newListSeriesIterator(tc.b)},
						itSeries{newListSeriesIterator(tc.c)},
					),
				}

				for _, res := range ress {
					exp := newListSeriesIterator(tc.exp)

					testutil.Equals(t, tc.success, res.Seek(tc.seek))

					if tc.success {
						// Init the list and then proceed to check.
						remaining := exp.Next()
						testutil.Assert(t, remaining == true, "")

						for remaining {
							sExp, eExp := exp.At()
							sRes, eRes := res.At()
							testutil.Equals(t, eExp, eRes)
							testutil.Equals(t, sExp, sRes)

							remaining = exp.Next()
							testutil.Equals(t, remaining, res.Next())
						}
					}
				}
			}
		})
	})
}

// Regression for: https://github.com/prometheus/tsdb/pull/97
func TestChunkSeriesIterator_DoubleSeek(t *testing.T) {
	chkMetas := []chunks.Meta{
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{4, 4}, sample{5, 5}}),
	}

	res := newChunkSeriesIterator(chkMetas, nil, 2, 8)
	testutil.Assert(t, res.Seek(1) == true, "")
	testutil.Assert(t, res.Seek(2) == true, "")
	ts, v := res.At()
	testutil.Equals(t, int64(2), ts)
	testutil.Equals(t, float64(2), v)
}

// Regression when seeked chunks were still found via binary search and we always
// skipped to the end when seeking a value in the current chunk.
func TestChunkSeriesIterator_SeekInCurrentChunk(t *testing.T) {
	metas := []chunks.Meta{
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 2}, sample{3, 4}, sample{5, 6}, sample{7, 8}}),
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{}),
	}

	it := newChunkSeriesIterator(metas, nil, 1, 7)

	testutil.Assert(t, it.Next() == true, "")
	ts, v := it.At()
	testutil.Equals(t, int64(1), ts)
	testutil.Equals(t, float64(2), v)

	testutil.Assert(t, it.Seek(4) == true, "")
	ts, v = it.At()
	testutil.Equals(t, int64(5), ts)
	testutil.Equals(t, float64(6), v)
}

// Regression when calling Next() with a time bounded to fit within two samples.
// Seek gets called and advances beyond the max time, which was just accepted as a valid sample.
func TestChunkSeriesIterator_NextWithMinTime(t *testing.T) {
	metas := []chunks.Meta{
		tsdbutil.ChunkFromSamples([]tsdbutil.Sample{sample{1, 6}, sample{5, 6}, sample{7, 8}}),
	}

	it := newChunkSeriesIterator(metas, nil, 2, 4)
	testutil.Assert(t, it.Next() == false, "")
}

func TestPopulatedCSReturnsValidChunkSlice(t *testing.T) {
	lbls := []labels.Labels{labels.New(labels.Label{"a", "b"})}
	chunkMetas := [][]chunks.Meta{
		{
			{MinTime: 1, MaxTime: 2, Ref: 1},
			{MinTime: 3, MaxTime: 4, Ref: 2},
			{MinTime: 10, MaxTime: 12, Ref: 3},
		},
	}

	cr := mockChunkReader(
		map[uint64]chunkenc.Chunk{
			1: chunkenc.NewXORChunk(),
			2: chunkenc.NewXORChunk(),
			3: chunkenc.NewXORChunk(),
		},
	)

	m := &mockChunkSeriesSet{l: lbls, cm: chunkMetas, i: -1}
	p := &populatedChunkSeries{
		set:    m,
		chunks: cr,

		mint: 0,
		maxt: 0,
	}

	testutil.Assert(t, p.Next() == false, "")

	p.mint = 6
	p.maxt = 9
	testutil.Assert(t, p.Next() == false, "")

	// Test the case where 1 chunk could cause an unpopulated chunk to be returned.
	chunkMetas = [][]chunks.Meta{
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
	testutil.Assert(t, p.Next() == false, "")
}

type mockChunkSeriesSet struct {
	l  []labels.Labels
	cm [][]chunks.Meta

	i int
}

func (m *mockChunkSeriesSet) Next() bool {
	if len(m.l) != len(m.cm) {
		return false
	}
	m.i++
	return m.i < len(m.l)
}

func (m *mockChunkSeriesSet) At() (labels.Labels, []chunks.Meta, Intervals) {
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
			return EmptySeriesSet()
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
		20000,
	} {
		for _, j := range []int{1, 2, 4, 8, 16, 32} {
			b.Run(fmt.Sprintf("series=%d,blocks=%d", k, j), func(b *testing.B) {
				lbls, err := labels.ReadLabels(filepath.Join("testdata", "20kseries.json"), k)
				testutil.Ok(b, err)

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
						sets = append(sets, newMockSeriesSet(s))
					}
					ms := sel(sets)

					i := 0
					for ms.Next() {
						i++
					}
					testutil.Ok(b, ms.Err())
					testutil.Equals(b, len(lbls), i)
				}
			})
		}
	}
}

func BenchmarkPersistedQueries(b *testing.B) {
	for _, nSeries := range []int{10, 100} {
		for _, nSamples := range []int64{1000, 10000, 100000} {
			b.Run(fmt.Sprintf("series=%d,samplesPerSeries=%d", nSeries, nSamples), func(b *testing.B) {
				dir, err := ioutil.TempDir("", "bench_persisted")
				testutil.Ok(b, err)
				defer os.RemoveAll(dir)

				block, err := OpenBlock(nil, createBlock(b, dir, genSeries(nSeries, 10, 1, int64(nSamples))), nil)
				testutil.Ok(b, err)
				defer block.Close()

				q, err := NewBlockQuerier(block, block.Meta().MinTime, block.Meta().MaxTime)
				testutil.Ok(b, err)
				defer q.Close()

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					ss, err := q.Select(labels.NewMustRegexpMatcher("__name__", ".+"))
					for ss.Next() {
						s := ss.At()
						s.Labels()
						it := s.Iterator()
						for it.Next() {
						}
						testutil.Ok(b, it.Err())
					}
					testutil.Ok(b, ss.Err())
					testutil.Ok(b, err)
				}
			})
		}
	}
}

type mockChunkReader map[uint64]chunkenc.Chunk

func (cr mockChunkReader) Chunk(id uint64) (chunkenc.Chunk, error) {
	chk, ok := cr[id]
	if ok {
		return chk, nil
	}

	return nil, errors.New("Chunk with ref not found")
}

func (cr mockChunkReader) Close() error {
	return nil
}

func TestDeletedIterator(t *testing.T) {
	chk := chunkenc.NewXORChunk()
	app, err := chk.Appender()
	testutil.Ok(t, err)
	// Insert random stuff from (0, 1000).
	act := make([]sample, 1000)
	for i := 0; i < 1000; i++ {
		act[i].t = int64(i)
		act[i].v = rand.Float64()
		app.Append(act[i].t, act[i].v)
	}

	cases := []struct {
		r Intervals
	}{
		{r: Intervals{{1, 20}}},
		{r: Intervals{{1, 10}, {12, 20}, {21, 23}, {25, 30}}},
		{r: Intervals{{1, 10}, {12, 20}, {20, 30}}},
		{r: Intervals{{1, 10}, {12, 23}, {25, 30}}},
		{r: Intervals{{1, 23}, {12, 20}, {25, 30}}},
		{r: Intervals{{1, 23}, {12, 20}, {25, 3000}}},
		{r: Intervals{{0, 2000}}},
		{r: Intervals{{500, 2000}}},
		{r: Intervals{{0, 200}}},
		{r: Intervals{{1000, 20000}}},
	}

	for _, c := range cases {
		i := int64(-1)
		it := &deletedIterator{it: chk.Iterator(), intervals: c.r[:]}
		ranges := c.r[:]
		for it.Next() {
			i++
			for _, tr := range ranges {
				if tr.inBounds(i) {
					i = tr.Maxt + 1
					ranges = ranges[1:]
				}
			}

			testutil.Assert(t, i < 1000, "")

			ts, v := it.At()
			testutil.Equals(t, act[i].t, ts)
			testutil.Equals(t, act[i].v, v)
		}
		// There has been an extra call to Next().
		i++
		for _, tr := range ranges {
			if tr.inBounds(i) {
				i = tr.Maxt + 1
				ranges = ranges[1:]
			}
		}

		testutil.Assert(t, i >= 1000, "")
		testutil.Ok(t, it.Err())
	}
}

type series struct {
	l      labels.Labels
	chunks []chunks.Meta
}

type mockIndex struct {
	series     map[uint64]series
	labelIndex map[string][]string
	postings   map[labels.Label][]uint64
	symbols    map[string]struct{}
}

func newMockIndex() mockIndex {
	ix := mockIndex{
		series:     make(map[uint64]series),
		labelIndex: make(map[string][]string),
		postings:   make(map[labels.Label][]uint64),
		symbols:    make(map[string]struct{}),
	}
	return ix
}

func (m mockIndex) Symbols() (map[string]struct{}, error) {
	return m.symbols, nil
}

func (m mockIndex) AddSeries(ref uint64, l labels.Labels, chunks ...chunks.Meta) error {
	if _, ok := m.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	for _, lbl := range l {
		m.symbols[lbl.Name] = struct{}{}
		m.symbols[lbl.Value] = struct{}{}
	}

	s := series{l: l}
	// Actual chunk data is not stored in the index.
	for _, c := range chunks {
		c.Chunk = nil
		s.chunks = append(s.chunks, c)
	}
	m.series[ref] = s

	return nil
}

func (m mockIndex) WriteLabelIndex(names []string, values []string) error {
	// TODO support composite indexes
	if len(names) != 1 {
		return errors.New("composite indexes not supported yet")
	}
	sort.Strings(values)
	m.labelIndex[names[0]] = values
	return nil
}

func (m mockIndex) WritePostings(name, value string, it index.Postings) error {
	l := labels.Label{Name: name, Value: value}
	if _, ok := m.postings[l]; ok {
		return errors.Errorf("postings for %s already added", l)
	}
	ep, err := index.ExpandPostings(it)
	if err != nil {
		return err
	}
	m.postings[l] = ep
	return nil
}

func (m mockIndex) Close() error {
	return nil
}

func (m mockIndex) LabelValues(names ...string) (index.StringTuples, error) {
	// TODO support composite indexes
	if len(names) != 1 {
		return nil, errors.New("composite indexes not supported yet")
	}

	return index.NewStringTuples(m.labelIndex[names[0]], 1)
}

func (m mockIndex) Postings(name, value string) (index.Postings, error) {
	l := labels.Label{Name: name, Value: value}
	return index.NewListPostings(m.postings[l]), nil
}

func (m mockIndex) SortedPostings(p index.Postings) index.Postings {
	ep, err := index.ExpandPostings(p)
	if err != nil {
		return index.ErrPostings(errors.Wrap(err, "expand postings"))
	}

	sort.Slice(ep, func(i, j int) bool {
		return labels.Compare(m.series[ep[i]].l, m.series[ep[j]].l) < 0
	})
	return index.NewListPostings(ep)
}

func (m mockIndex) Series(ref uint64, lset *labels.Labels, chks *[]chunks.Meta) error {
	s, ok := m.series[ref]
	if !ok {
		return ErrNotFound
	}
	*lset = append((*lset)[:0], s.l...)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

func (m mockIndex) LabelIndices() ([][]string, error) {
	res := make([][]string, 0, len(m.labelIndex))
	for k := range m.labelIndex {
		res = append(res, []string{k})
	}
	return res, nil
}

func (m mockIndex) LabelNames() ([]string, error) {
	labelNames := make([]string, 0, len(m.labelIndex))
	for name := range m.labelIndex {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)
	return labelNames, nil
}

type mockSeries struct {
	labels   func() labels.Labels
	iterator func() SeriesIterator
}

func newSeries(l map[string]string, s []tsdbutil.Sample) Series {
	return &mockSeries{
		labels:   func() labels.Labels { return labels.FromMap(l) },
		iterator: func() SeriesIterator { return newListSeriesIterator(s) },
	}
}
func (m *mockSeries) Labels() labels.Labels    { return m.labels() }
func (m *mockSeries) Iterator() SeriesIterator { return m.iterator() }

type listSeriesIterator struct {
	list []tsdbutil.Sample
	idx  int
}

func newListSeriesIterator(list []tsdbutil.Sample) *listSeriesIterator {
	return &listSeriesIterator{list: list, idx: -1}
}

func (it *listSeriesIterator) At() (int64, float64) {
	s := it.list[it.idx]
	return s.T(), s.V()
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
		return s.T() >= t
	})

	return it.idx < len(it.list)
}

func (it *listSeriesIterator) Err() error {
	return nil
}

func BenchmarkQueryIterator(b *testing.B) {
	cases := []struct {
		numBlocks                   int
		numSeries                   int
		numSamplesPerSeriesPerBlock int
		overlapPercentages          []int // >=0, <=100, this is w.r.t. the previous block.
	}{
		{
			numBlocks:                   20,
			numSeries:                   1000,
			numSamplesPerSeriesPerBlock: 20000,
			overlapPercentages:          []int{0, 10, 30},
		},
	}

	for _, c := range cases {
		for _, overlapPercentage := range c.overlapPercentages {
			benchMsg := fmt.Sprintf("nBlocks=%d,nSeries=%d,numSamplesPerSeriesPerBlock=%d,overlap=%d%%",
				c.numBlocks, c.numSeries, c.numSamplesPerSeriesPerBlock, overlapPercentage)

			b.Run(benchMsg, func(b *testing.B) {
				dir, err := ioutil.TempDir("", "bench_query_iterator")
				testutil.Ok(b, err)
				defer func() {
					testutil.Ok(b, os.RemoveAll(dir))
				}()

				var (
					blocks          []*Block
					overlapDelta    = int64(overlapPercentage * c.numSamplesPerSeriesPerBlock / 100)
					prefilledLabels []map[string]string
					generatedSeries []Series
				)
				for i := int64(0); i < int64(c.numBlocks); i++ {
					offset := i * overlapDelta
					mint := i*int64(c.numSamplesPerSeriesPerBlock) - offset
					maxt := mint + int64(c.numSamplesPerSeriesPerBlock) - 1
					if len(prefilledLabels) == 0 {
						generatedSeries = genSeries(c.numSeries, 10, mint, maxt)
						for _, s := range generatedSeries {
							prefilledLabels = append(prefilledLabels, s.Labels().Map())
						}
					} else {
						generatedSeries = populateSeries(prefilledLabels, mint, maxt)
					}
					block, err := OpenBlock(nil, createBlock(b, dir, generatedSeries), nil)
					testutil.Ok(b, err)
					blocks = append(blocks, block)
					defer block.Close()
				}

				que := &querier{
					blocks: make([]Querier, 0, len(blocks)),
				}
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					testutil.Ok(b, err)
					que.blocks = append(que.blocks, q)
				}

				var sq Querier = que
				if overlapPercentage > 0 {
					sq = &verticalQuerier{
						querier: *que,
					}
				}
				defer sq.Close()

				b.ResetTimer()
				b.ReportAllocs()

				ss, err := sq.Select(labels.NewMustRegexpMatcher("__name__", ".*"))
				testutil.Ok(b, err)
				for ss.Next() {
					it := ss.At().Iterator()
					for it.Next() {
					}
					testutil.Ok(b, it.Err())
				}
				testutil.Ok(b, ss.Err())
				testutil.Ok(b, err)
			})
		}
	}
}

func BenchmarkQuerySeek(b *testing.B) {
	cases := []struct {
		numBlocks                   int
		numSeries                   int
		numSamplesPerSeriesPerBlock int
		overlapPercentages          []int // >=0, <=100, this is w.r.t. the previous block.
	}{
		{
			numBlocks:                   20,
			numSeries:                   100,
			numSamplesPerSeriesPerBlock: 2000,
			overlapPercentages:          []int{0, 10, 30, 50},
		},
	}

	for _, c := range cases {
		for _, overlapPercentage := range c.overlapPercentages {
			benchMsg := fmt.Sprintf("nBlocks=%d,nSeries=%d,numSamplesPerSeriesPerBlock=%d,overlap=%d%%",
				c.numBlocks, c.numSeries, c.numSamplesPerSeriesPerBlock, overlapPercentage)

			b.Run(benchMsg, func(b *testing.B) {
				dir, err := ioutil.TempDir("", "bench_query_iterator")
				testutil.Ok(b, err)
				defer func() {
					testutil.Ok(b, os.RemoveAll(dir))
				}()

				var (
					blocks          []*Block
					overlapDelta    = int64(overlapPercentage * c.numSamplesPerSeriesPerBlock / 100)
					prefilledLabels []map[string]string
					generatedSeries []Series
				)
				for i := int64(0); i < int64(c.numBlocks); i++ {
					offset := i * overlapDelta
					mint := i*int64(c.numSamplesPerSeriesPerBlock) - offset
					maxt := mint + int64(c.numSamplesPerSeriesPerBlock) - 1
					if len(prefilledLabels) == 0 {
						generatedSeries = genSeries(c.numSeries, 10, mint, maxt)
						for _, s := range generatedSeries {
							prefilledLabels = append(prefilledLabels, s.Labels().Map())
						}
					} else {
						generatedSeries = populateSeries(prefilledLabels, mint, maxt)
					}
					block, err := OpenBlock(nil, createBlock(b, dir, generatedSeries), nil)
					testutil.Ok(b, err)
					blocks = append(blocks, block)
					defer block.Close()
				}

				que := &querier{
					blocks: make([]Querier, 0, len(blocks)),
				}
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					testutil.Ok(b, err)
					que.blocks = append(que.blocks, q)
				}

				var sq Querier = que
				if overlapPercentage > 0 {
					sq = &verticalQuerier{
						querier: *que,
					}
				}
				defer sq.Close()

				mint := blocks[0].meta.MinTime
				maxt := blocks[len(blocks)-1].meta.MaxTime

				b.ResetTimer()
				b.ReportAllocs()

				ss, err := sq.Select(labels.NewMustRegexpMatcher("__name__", ".*"))
				for ss.Next() {
					it := ss.At().Iterator()
					for t := mint; t <= maxt; t++ {
						it.Seek(t)
					}
					testutil.Ok(b, it.Err())
				}
				testutil.Ok(b, ss.Err())
				testutil.Ok(b, err)
			})
		}
	}
}
