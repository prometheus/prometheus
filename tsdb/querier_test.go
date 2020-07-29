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
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/testutil"
)

type mockSeriesSet struct {
	next   func() bool
	series func() storage.Series
	ws     func() storage.Warnings
	err    func() error
}

func (m *mockSeriesSet) Next() bool                 { return m.next() }
func (m *mockSeriesSet) At() storage.Series         { return m.series() }
func (m *mockSeriesSet) Err() error                 { return m.err() }
func (m *mockSeriesSet) Warnings() storage.Warnings { return m.ws() }

func newMockSeriesSet(list []storage.Series) *mockSeriesSet {
	i := -1
	return &mockSeriesSet{
		next: func() bool {
			i++
			return i < len(list)
		},
		series: func() storage.Series {
			return list[i]
		},
		err: func() error { return nil },
		ws:  func() storage.Warnings { return nil },
	}
}

func TestMergedSeriesSet(t *testing.T) {
	cases := []struct {
		// The input sets in order (samples in series in b are strictly
		// after those in a).
		a, b storage.SeriesSet
		// The composition of a and b in the partition series set must yield
		// results equivalent to the result series set.
		exp storage.SeriesSet
	}{
		{
			a: newMockSeriesSet([]storage.Series{
				newSeries(map[string]string{
					"a": "a",
				}, []tsdbutil.Sample{
					sample{t: 1, v: 1},
				}),
			}),
			b: newMockSeriesSet([]storage.Series{
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
			exp: newMockSeriesSet([]storage.Series{
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
			a: newMockSeriesSet([]storage.Series{
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
			b: newMockSeriesSet([]storage.Series{
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
			exp: newMockSeriesSet([]storage.Series{
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
		res := NewMergedSeriesSet([]storage.SeriesSet{c.a, c.b})

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

func expandSeriesIterator(it chunkenc.Iterator) (r []tsdbutil.Sample, err error) {
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
func createIdxChkReaders(t *testing.T, tc []seriesSamples) (IndexReader, ChunkReader, int64, int64) {
	sort.Slice(tc, func(i, j int) bool {
		return labels.Compare(labels.FromMap(tc[i].lset), labels.FromMap(tc[i].lset)) < 0
	})

	postings := index.NewMemPostings()
	chkReader := mockChunkReader(make(map[uint64]chunkenc.Chunk))
	lblIdx := make(map[string]stringset)
	mi := newMockIndex()
	blockMint := int64(math.MaxInt64)
	blockMaxt := int64(math.MinInt64)

	var chunkRef uint64
	for i, s := range tc {
		i = i + 1 // 0 is not a valid posting.
		metas := make([]chunks.Meta, 0, len(s.chunks))
		for _, chk := range s.chunks {
			if chk[0].t < blockMint {
				blockMint = chk[0].t
			}
			if chk[len(chk)-1].t > blockMaxt {
				blockMaxt = chk[len(chk)-1].t
			}

			metas = append(metas, chunks.Meta{
				MinTime: chk[0].t,
				MaxTime: chk[len(chk)-1].t,
				Ref:     chunkRef,
			})

			chunk := chunkenc.NewXORChunk()
			app, _ := chunk.Appender()
			for _, smpl := range chk {
				app.Append(smpl.t, smpl.v)
			}
			chkReader[chunkRef] = chunk
			chunkRef++
		}

		ls := labels.FromMap(s.lset)
		testutil.Ok(t, mi.AddSeries(uint64(i), ls, metas...))

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

	testutil.Ok(t, postings.Iter(func(l labels.Label, p index.Postings) error {
		return mi.WritePostings(l.Name, l.Value, p)
	}))

	return mi, chkReader, blockMint, blockMaxt
}

func TestBlockQuerier(t *testing.T) {
	newSeries := func(l map[string]string, s []tsdbutil.Sample) storage.Series {
		return &mockSeries{
			labels:   func() labels.Labels { return labels.FromMap(l) },
			iterator: func() chunkenc.Iterator { return newListSeriesIterator(s) },
		}
	}

	type query struct {
		mint, maxt int64
		ms         []*labels.Matcher
		exp        storage.SeriesSet
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
				ms:   []*labels.Matcher{},
				exp:  newMockSeriesSet([]storage.Series{}),
			},
			{
				mint: 0,
				maxt: 0,
				ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
				exp:  newMockSeriesSet([]storage.Series{}),
			},
			{
				mint: 1,
				maxt: 0,
				ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
				exp:  newMockSeriesSet([]storage.Series{}),
			},
			{
				mint: 2,
				maxt: 6,
				ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
				exp: newMockSeriesSet([]storage.Series{
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
		ir, cr, _, _ := createIdxChkReaders(t, cases.data)
		querier := &blockQuerier{
			index:      ir,
			chunks:     cr,
			tombstones: tombstones.NewMemTombstones(),

			mint: c.mint,
			maxt: c.maxt,
		}

		res := querier.Select(false, nil, c.ms...)

		for {
			eok, rok := c.exp.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				testutil.Equals(t, 0, len(res.Warnings()))
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
	newSeries := func(l map[string]string, s []tsdbutil.Sample) storage.Series {
		return &mockSeries{
			labels:   func() labels.Labels { return labels.FromMap(l) },
			iterator: func() chunkenc.Iterator { return newListSeriesIterator(s) },
		}
	}

	type query struct {
		mint, maxt int64
		ms         []*labels.Matcher
		exp        storage.SeriesSet
	}

	cases := struct {
		data []seriesSamples

		tombstones tombstones.Reader
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
		tombstones: tombstones.NewTestMemTombstones([]tombstones.Intervals{
			{{Mint: 1, Maxt: 3}},
			{{Mint: 1, Maxt: 3}, {Mint: 6, Maxt: 10}},
			{{Mint: 6, Maxt: 10}},
		}),
		queries: []query{
			{
				mint: 2,
				maxt: 7,
				ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
				exp: newMockSeriesSet([]storage.Series{
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
				ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "b", "b")},
				exp: newMockSeriesSet([]storage.Series{
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
				ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
				exp: newMockSeriesSet([]storage.Series{
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
				ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
				exp:  newMockSeriesSet([]storage.Series{}),
			},
		},
	}

Outer:
	for _, c := range cases.queries {
		ir, cr, _, _ := createIdxChkReaders(t, cases.data)
		querier := &blockQuerier{
			index:      ir,
			chunks:     cr,
			tombstones: cases.tombstones,

			mint: c.mint,
			maxt: c.maxt,
		}

		res := querier.Select(false, nil, c.ms...)

		for {
			eok, rok := c.exp.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				testutil.Equals(t, 0, len(res.Warnings()))
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
		// Postings should be in the sorted order of the series
		postings []uint64

		expIdxs []int
	}{
		{
			series: []refdSeries{
				{
					lset: labels.New([]labels.Label{{Name: "a", Value: "a"}}...),
					chunks: []chunks.Meta{
						{Ref: 29}, {Ref: 45}, {Ref: 245}, {Ref: 123}, {Ref: 4232}, {Ref: 5344},
						{Ref: 121},
					},
					ref: 12,
				},
				{
					lset: labels.New([]labels.Label{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}}...),
					chunks: []chunks.Meta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.New([]labels.Label{{Name: "b", Value: "c"}}...),
					chunks: []chunks.Meta{{Ref: 8282}},
					ref:    1,
				},
				{
					lset: labels.New([]labels.Label{{Name: "b", Value: "b"}}...),
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
					lset: labels.New([]labels.Label{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}}...),
					chunks: []chunks.Meta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.New([]labels.Label{{Name: "b", Value: "c"}}...),
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
			testutil.Ok(t, mi.AddSeries(s.ref, s.lset, s.chunks...))
		}

		bcs := &baseChunkSeries{
			p:          index.NewListPostings(tc.postings),
			index:      mi,
			tombstones: tombstones.NewMemTombstones(),
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

type itSeries struct {
	si chunkenc.Iterator
}

func (s itSeries) Iterator() chunkenc.Iterator { return s.si }
func (s itSeries) Labels() labels.Labels       { return labels.Labels{} }

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
				ress := []chunkenc.Iterator{
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
	lbls := []labels.Labels{labels.New(labels.Label{Name: "a", Value: "b"})}
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

func (m *mockChunkSeriesSet) At() (labels.Labels, []chunks.Meta, tombstones.Intervals) {
	return m.l[m.i], m.cm[m.i], nil
}

func (m *mockChunkSeriesSet) Err() error {
	return nil
}

// Test the cost of merging series sets for different number of merged sets and their size.
// The subset are all equivalent so this does not capture merging of partial or non-overlapping sets well.
// TODO(bwplotka): Merge with storage merged series set benchmark.
func BenchmarkMergedSeriesSet(b *testing.B) {
	var sel = func(sets []storage.SeriesSet) storage.SeriesSet {
		return NewMergedSeriesSet(sets)
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

				in := make([][]storage.Series, j)

				for _, l := range lbls {
					l2 := l
					for j := range in {
						in[j] = append(in[j], &mockSeries{labels: func() labels.Labels { return l2 }})
					}
				}

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					var sets []storage.SeriesSet
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
		r tombstones.Intervals
	}{
		{r: tombstones.Intervals{{Mint: 1, Maxt: 20}}},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 10}, {Mint: 12, Maxt: 20}, {Mint: 21, Maxt: 23}, {Mint: 25, Maxt: 30}}},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 10}, {Mint: 12, Maxt: 20}, {Mint: 20, Maxt: 30}}},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 10}, {Mint: 12, Maxt: 23}, {Mint: 25, Maxt: 30}}},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 23}, {Mint: 12, Maxt: 20}, {Mint: 25, Maxt: 30}}},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 23}, {Mint: 12, Maxt: 20}, {Mint: 25, Maxt: 3000}}},
		{r: tombstones.Intervals{{Mint: 0, Maxt: 2000}}},
		{r: tombstones.Intervals{{Mint: 500, Maxt: 2000}}},
		{r: tombstones.Intervals{{Mint: 0, Maxt: 200}}},
		{r: tombstones.Intervals{{Mint: 1000, Maxt: 20000}}},
	}

	for _, c := range cases {
		i := int64(-1)
		it := &deletedIterator{it: chk.Iterator(nil), intervals: c.r[:]}
		ranges := c.r[:]
		for it.Next() {
			i++
			for _, tr := range ranges {
				if tr.InBounds(i) {
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
			if tr.InBounds(i) {
				i = tr.Maxt + 1
				ranges = ranges[1:]
			}
		}

		testutil.Assert(t, i >= 1000, "")
		testutil.Ok(t, it.Err())
	}
}

func TestDeletedIterator_WithSeek(t *testing.T) {
	chk := chunkenc.NewXORChunk()
	app, err := chk.Appender()
	testutil.Ok(t, err)
	// Insert random stuff from (0, 1000).
	act := make([]sample, 1000)
	for i := 0; i < 1000; i++ {
		act[i].t = int64(i)
		act[i].v = float64(i)
		app.Append(act[i].t, act[i].v)
	}

	cases := []struct {
		r        tombstones.Intervals
		seek     int64
		ok       bool
		seekedTs int64
	}{
		{r: tombstones.Intervals{{Mint: 1, Maxt: 20}}, seek: 1, ok: true, seekedTs: 21},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 20}}, seek: 20, ok: true, seekedTs: 21},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 20}}, seek: 10, ok: true, seekedTs: 21},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 20}}, seek: 999, ok: true, seekedTs: 999},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 20}}, seek: 1000, ok: false},
		{r: tombstones.Intervals{{Mint: 1, Maxt: 23}, {Mint: 24, Maxt: 40}, {Mint: 45, Maxt: 3000}}, seek: 1, ok: true, seekedTs: 41},
		{r: tombstones.Intervals{{Mint: 5, Maxt: 23}, {Mint: 24, Maxt: 40}, {Mint: 41, Maxt: 3000}}, seek: 5, ok: false},
		{r: tombstones.Intervals{{Mint: 0, Maxt: 2000}}, seek: 10, ok: false},
		{r: tombstones.Intervals{{Mint: 500, Maxt: 2000}}, seek: 10, ok: true, seekedTs: 10},
		{r: tombstones.Intervals{{Mint: 500, Maxt: 2000}}, seek: 501, ok: false},
	}

	for _, c := range cases {
		it := &deletedIterator{it: chk.Iterator(nil), intervals: c.r[:]}

		testutil.Equals(t, c.ok, it.Seek(c.seek))
		if c.ok {
			ts, _ := it.At()
			testutil.Equals(t, c.seekedTs, ts)
		}
	}
}

type series struct {
	l      labels.Labels
	chunks []chunks.Meta
}

type mockIndex struct {
	series   map[uint64]series
	postings map[labels.Label][]uint64
	symbols  map[string]struct{}
}

func newMockIndex() mockIndex {
	ix := mockIndex{
		series:   make(map[uint64]series),
		postings: make(map[labels.Label][]uint64),
		symbols:  make(map[string]struct{}),
	}
	return ix
}

func (m mockIndex) Symbols() index.StringIter {
	l := []string{}
	for s := range m.symbols {
		l = append(l, s)
	}
	sort.Strings(l)
	return index.NewStringListIter(l)
}

func (m *mockIndex) AddSeries(ref uint64, l labels.Labels, chunks ...chunks.Meta) error {
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

func (m mockIndex) SortedLabelValues(name string) ([]string, error) {
	values, _ := m.LabelValues(name)
	sort.Strings(values)
	return values, nil
}

func (m mockIndex) LabelValues(name string) ([]string, error) {
	values := []string{}
	for l := range m.postings {
		if l.Name == name {
			values = append(values, l.Value)
		}
	}
	return values, nil
}

func (m mockIndex) Postings(name string, values ...string) (index.Postings, error) {
	res := make([]index.Postings, 0, len(values))
	for _, value := range values {
		l := labels.Label{Name: name, Value: value}
		res = append(res, index.NewListPostings(m.postings[l]))
	}
	return index.Merge(res...), nil
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
		return storage.ErrNotFound
	}
	*lset = append((*lset)[:0], s.l...)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

func (m mockIndex) LabelNames() ([]string, error) {
	names := map[string]struct{}{}
	for l := range m.postings {
		names[l.Name] = struct{}{}
	}
	l := make([]string, 0, len(names))
	for name := range names {
		l = append(l, name)
	}
	sort.Strings(l)
	return l, nil
}

type mockSeries struct {
	labels   func() labels.Labels
	iterator func() chunkenc.Iterator
}

func newSeries(l map[string]string, s []tsdbutil.Sample) storage.Series {
	return &mockSeries{
		labels:   func() labels.Labels { return labels.FromMap(l) },
		iterator: func() chunkenc.Iterator { return newListSeriesIterator(s) },
	}
}
func (m *mockSeries) Labels() labels.Labels       { return m.labels() }
func (m *mockSeries) Iterator() chunkenc.Iterator { return m.iterator() }

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
					generatedSeries []storage.Series
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
					blocks: make([]storage.Querier, 0, len(blocks)),
				}
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					testutil.Ok(b, err)
					que.blocks = append(que.blocks, q)
				}

				var sq storage.Querier = que
				if overlapPercentage > 0 {
					sq = &verticalQuerier{
						querier: *que,
					}
				}
				defer sq.Close()

				benchQuery(b, c.numSeries, sq, labels.Selector{labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*")})
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
					generatedSeries []storage.Series
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
					blocks: make([]storage.Querier, 0, len(blocks)),
				}
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					testutil.Ok(b, err)
					que.blocks = append(que.blocks, q)
				}

				var sq storage.Querier = que
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

				ss := sq.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
				for ss.Next() {
					it := ss.At().Iterator()
					for t := mint; t <= maxt; t++ {
						it.Seek(t)
					}
					testutil.Ok(b, it.Err())
				}
				testutil.Ok(b, ss.Err())
				testutil.Ok(b, err)
				testutil.Equals(b, 0, len(ss.Warnings()))
			})
		}
	}
}

// Refer to https://github.com/prometheus/prometheus/issues/2651.
func BenchmarkSetMatcher(b *testing.B) {
	cases := []struct {
		numBlocks                   int
		numSeries                   int
		numSamplesPerSeriesPerBlock int
		cardinality                 int
		pattern                     string
	}{
		// The first three cases are to find out whether the set
		// matcher is always faster than regex matcher.
		{
			numBlocks:                   1,
			numSeries:                   1,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 100,
			pattern:                     "1|2|3|4|5|6|7|8|9|10",
		},
		{
			numBlocks:                   1,
			numSeries:                   15,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 100,
			pattern:                     "1|2|3|4|5|6|7|8|9|10",
		},
		{
			numBlocks:                   1,
			numSeries:                   15,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 100,
			pattern:                     "1|2|3",
		},
		// Big data sizes benchmarks.
		{
			numBlocks:                   20,
			numSeries:                   1000,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 100,
			pattern:                     "1|2|3",
		},
		{
			numBlocks:                   20,
			numSeries:                   1000,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 100,
			pattern:                     "1|2|3|4|5|6|7|8|9|10",
		},
		// Increase cardinality.
		{
			numBlocks:                   1,
			numSeries:                   100000,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 100000,
			pattern:                     "1|2|3|4|5|6|7|8|9|10",
		},
		{
			numBlocks:                   1,
			numSeries:                   500000,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 500000,
			pattern:                     "1|2|3|4|5|6|7|8|9|10",
		},
		{
			numBlocks:                   10,
			numSeries:                   500000,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 500000,
			pattern:                     "1|2|3|4|5|6|7|8|9|10",
		},
		{
			numBlocks:                   1,
			numSeries:                   1000000,
			numSamplesPerSeriesPerBlock: 10,
			cardinality:                 1000000,
			pattern:                     "1|2|3|4|5|6|7|8|9|10",
		},
	}

	for _, c := range cases {
		dir, err := ioutil.TempDir("", "bench_postings_for_matchers")
		testutil.Ok(b, err)
		defer func() {
			testutil.Ok(b, os.RemoveAll(dir))
		}()

		var (
			blocks          []*Block
			prefilledLabels []map[string]string
			generatedSeries []storage.Series
		)
		for i := int64(0); i < int64(c.numBlocks); i++ {
			mint := i * int64(c.numSamplesPerSeriesPerBlock)
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
			blocks: make([]storage.Querier, 0, len(blocks)),
		}
		for _, blk := range blocks {
			q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
			testutil.Ok(b, err)
			que.blocks = append(que.blocks, q)
		}
		defer que.Close()

		benchMsg := fmt.Sprintf("nSeries=%d,nBlocks=%d,cardinality=%d,pattern=\"%s\"", c.numSeries, c.numBlocks, c.cardinality, c.pattern)
		b.Run(benchMsg, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				ss := que.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "test", c.pattern))
				for ss.Next() {
				}
				testutil.Ok(b, ss.Err())
				testutil.Equals(b, 0, len(ss.Warnings()))
			}
		})
	}
}

// Refer to https://github.com/prometheus/prometheus/issues/2651.
func TestFindSetMatches(t *testing.T) {
	cases := []struct {
		pattern string
		exp     []string
	}{
		// Simple sets.
		{
			pattern: "^(?:foo|bar|baz)$",
			exp: []string{
				"foo",
				"bar",
				"baz",
			},
		},
		// Simple sets containing escaped characters.
		{
			pattern: "^(?:fo\\.o|bar\\?|\\^baz)$",
			exp: []string{
				"fo.o",
				"bar?",
				"^baz",
			},
		},
		// Simple sets containing special characters without escaping.
		{
			pattern: "^(?:fo.o|bar?|^baz)$",
			exp:     nil,
		},
		// Missing wrapper.
		{
			pattern: "foo|bar|baz",
			exp:     nil,
		},
	}

	for _, c := range cases {
		matches := findSetMatches(c.pattern)
		if len(c.exp) == 0 {
			if len(matches) != 0 {
				t.Errorf("Evaluating %s, unexpected result %v", c.pattern, matches)
			}
		} else {
			if len(matches) != len(c.exp) {
				t.Errorf("Evaluating %s, length of result not equal to exp", c.pattern)
			} else {
				for i := 0; i < len(c.exp); i++ {
					if c.exp[i] != matches[i] {
						t.Errorf("Evaluating %s, unexpected result %s", c.pattern, matches[i])
					}
				}
			}
		}
	}
}

func TestPostingsForMatchers(t *testing.T) {
	chunkDir, err := ioutil.TempDir("", "chunk_dir")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(chunkDir))
	}()
	h, err := NewHead(nil, nil, nil, 1000, chunkDir, nil, DefaultStripeSize, nil)
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, h.Close())
	}()

	app := h.Appender()
	app.Add(labels.FromStrings("n", "1"), 0, 0)
	app.Add(labels.FromStrings("n", "1", "i", "a"), 0, 0)
	app.Add(labels.FromStrings("n", "1", "i", "b"), 0, 0)
	app.Add(labels.FromStrings("n", "2"), 0, 0)
	app.Add(labels.FromStrings("n", "2.5"), 0, 0)
	testutil.Ok(t, app.Commit())

	cases := []struct {
		matchers []*labels.Matcher
		exp      []labels.Labels
	}{
		// Simple equals.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchEqual, "i", "a")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchEqual, "i", "missing")},
			exp:      []labels.Labels{},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "missing", "")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		// Not equals.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "n", "1")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "i", "")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "missing", "")},
			exp:      []labels.Labels{},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotEqual, "i", "a")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotEqual, "i", "")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		// Regex.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", "^1$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^a$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^a?$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "i", "^$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^.*$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^.+$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		// Not regex.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "n", "^1$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^a$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^a?$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^.*$")},
			exp:      []labels.Labels{},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^.+$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
			},
		},
		// Combinations.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotEqual, "i", ""), labels.MustNewMatcher(labels.MatchEqual, "i", "a")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotEqual, "i", "b"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^(b|a).*$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
			},
		},
		// Set optimization for Regex.
		// Refer to https://github.com/prometheus/prometheus/issues/2651.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", "1|2")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "2"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "i", "a|b")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", "x1|2")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", "2|2\\.5")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		// Empty value.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "i", "c||d")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
	}

	ir, err := h.Index()
	testutil.Ok(t, err)

	for _, c := range cases {
		exp := map[string]struct{}{}
		for _, l := range c.exp {
			exp[l.String()] = struct{}{}
		}
		p, err := PostingsForMatchers(ir, c.matchers...)
		testutil.Ok(t, err)

		for p.Next() {
			lbls := labels.Labels{}
			testutil.Ok(t, ir.Series(p.At(), &lbls, &[]chunks.Meta{}))
			if _, ok := exp[lbls.String()]; !ok {
				t.Errorf("Evaluating %v, unexpected result %s", c.matchers, lbls.String())
			} else {
				delete(exp, lbls.String())
			}
		}
		testutil.Ok(t, p.Err())
		if len(exp) != 0 {
			t.Errorf("Evaluating %v, missing results %+v", c.matchers, exp)
		}
	}

}

// TestClose ensures that calling Close more than once doesn't block and doesn't panic.
func TestClose(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_storage")
	if err != nil {
		t.Fatalf("Opening test dir failed: %s", err)
	}
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	createBlock(t, dir, genSeries(1, 1, 0, 10))
	createBlock(t, dir, genSeries(1, 1, 10, 20))

	db, err := Open(dir, nil, nil, DefaultOptions())
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	defer func() {
		testutil.Ok(t, db.Close())
	}()

	q, err := db.Querier(context.TODO(), 0, 20)
	testutil.Ok(t, err)
	testutil.Ok(t, q.Close())
	testutil.NotOk(t, q.Close())
}

func BenchmarkQueries(b *testing.B) {
	cases := map[string]labels.Selector{
		"Eq Matcher: Expansion - 1": {
			labels.MustNewMatcher(labels.MatchEqual, "la", "va"),
		},
		"Eq Matcher: Expansion - 2": {
			labels.MustNewMatcher(labels.MatchEqual, "la", "va"),
			labels.MustNewMatcher(labels.MatchEqual, "lb", "vb"),
		},

		"Eq Matcher: Expansion - 3": {
			labels.MustNewMatcher(labels.MatchEqual, "la", "va"),
			labels.MustNewMatcher(labels.MatchEqual, "lb", "vb"),
			labels.MustNewMatcher(labels.MatchEqual, "lc", "vc"),
		},
		"Regex Matcher: Expansion - 1": {
			labels.MustNewMatcher(labels.MatchRegexp, "la", ".*va"),
		},
		"Regex Matcher: Expansion - 2": {
			labels.MustNewMatcher(labels.MatchRegexp, "la", ".*va"),
			labels.MustNewMatcher(labels.MatchRegexp, "lb", ".*vb"),
		},
		"Regex Matcher: Expansion - 3": {
			labels.MustNewMatcher(labels.MatchRegexp, "la", ".*va"),
			labels.MustNewMatcher(labels.MatchRegexp, "lb", ".*vb"),
			labels.MustNewMatcher(labels.MatchRegexp, "lc", ".*vc"),
		},
	}

	queryTypes := make(map[string]storage.Querier)
	defer func() {
		for _, q := range queryTypes {
			// Can't run a check for error here as some of these will fail as
			// queryTypes is using the same slice for the different block queriers
			// and would have been closed in the previous iteration.
			q.Close()
		}
	}()

	for title, selectors := range cases {
		for _, nSeries := range []int{10} {
			for _, nSamples := range []int64{1000, 10000, 100000} {
				dir, err := ioutil.TempDir("", "test_persisted_query")
				testutil.Ok(b, err)
				defer func() {
					testutil.Ok(b, os.RemoveAll(dir))
				}()

				series := genSeries(nSeries, 5, 1, int64(nSamples))

				// Add some common labels to make the matchers select these series.
				{
					var commonLbls labels.Labels
					for _, selector := range selectors {
						switch selector.Type {
						case labels.MatchEqual:
							commonLbls = append(commonLbls, labels.Label{Name: selector.Name, Value: selector.Value})
						case labels.MatchRegexp:
							commonLbls = append(commonLbls, labels.Label{Name: selector.Name, Value: selector.Value})
						}
					}
					for i := range commonLbls {
						s := series[i].(*mockSeries)
						allLabels := append(commonLbls, s.Labels()...)
						s = &mockSeries{
							labels:   func() labels.Labels { return allLabels },
							iterator: s.iterator,
						}
						series[i] = s
					}
				}

				qs := make([]storage.Querier, 0, 10)
				for x := 0; x <= 10; x++ {
					block, err := OpenBlock(nil, createBlock(b, dir, series), nil)
					testutil.Ok(b, err)
					q, err := NewBlockQuerier(block, 1, int64(nSamples))
					testutil.Ok(b, err)
					qs = append(qs, q)
				}
				queryTypes["_1-Block"] = &querier{blocks: qs[:1]}
				queryTypes["_3-Blocks"] = &querier{blocks: qs[0:3]}
				queryTypes["_10-Blocks"] = &querier{blocks: qs}

				chunkDir, err := ioutil.TempDir("", "chunk_dir")
				testutil.Ok(b, err)
				defer func() {
					testutil.Ok(b, os.RemoveAll(chunkDir))
				}()
				head := createHead(b, series, chunkDir)
				qHead, err := NewBlockQuerier(head, 1, int64(nSamples))
				testutil.Ok(b, err)
				queryTypes["_Head"] = qHead

				for qtype, querier := range queryTypes {
					b.Run(title+qtype+"_nSeries:"+strconv.Itoa(nSeries)+"_nSamples:"+strconv.Itoa(int(nSamples)), func(b *testing.B) {
						expExpansions, err := strconv.Atoi(string(title[len(title)-1]))
						testutil.Ok(b, err)
						benchQuery(b, expExpansions, querier, selectors)
					})
				}
				testutil.Ok(b, head.Close())
			}
		}
	}
}

func benchQuery(b *testing.B, expExpansions int, q storage.Querier, selectors labels.Selector) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ss := q.Select(false, nil, selectors...)
		var actualExpansions int
		for ss.Next() {
			s := ss.At()
			s.Labels()
			it := s.Iterator()
			for it.Next() {
			}
			actualExpansions++
		}
		testutil.Ok(b, ss.Err())
		testutil.Equals(b, 0, len(ss.Warnings()))
		testutil.Equals(b, expExpansions, actualExpansions)
		testutil.Ok(b, ss.Err())
	}
}

// mockMatcherIndex is used to check if the regex matcher works as expected.
type mockMatcherIndex struct{}

func (m mockMatcherIndex) Symbols() index.StringIter { return nil }

func (m mockMatcherIndex) Close() error { return nil }

// SortedLabelValues will return error if it is called.
func (m mockMatcherIndex) SortedLabelValues(name string) ([]string, error) {
	return []string{}, errors.New("sorted label values called")
}

// LabelValues will return error if it is called.
func (m mockMatcherIndex) LabelValues(name string) ([]string, error) {
	return []string{}, errors.New("label values called")
}

func (m mockMatcherIndex) Postings(name string, values ...string) (index.Postings, error) {
	return index.EmptyPostings(), nil
}

func (m mockMatcherIndex) SortedPostings(p index.Postings) index.Postings {
	return index.EmptyPostings()
}

func (m mockMatcherIndex) Series(ref uint64, lset *labels.Labels, chks *[]chunks.Meta) error {
	return nil
}

func (m mockMatcherIndex) LabelNames() ([]string, error) { return []string{}, nil }

func TestPostingsForMatcher(t *testing.T) {
	cases := []struct {
		matcher  *labels.Matcher
		hasError bool
	}{
		{
			// Equal label matcher will just return.
			matcher:  labels.MustNewMatcher(labels.MatchEqual, "test", "test"),
			hasError: false,
		},
		{
			// Regex matcher which doesn't have '|' will call Labelvalues()
			matcher:  labels.MustNewMatcher(labels.MatchRegexp, "test", ".*"),
			hasError: true,
		},
		{
			matcher:  labels.MustNewMatcher(labels.MatchRegexp, "test", "a|b"),
			hasError: false,
		},
		{
			// Test case for double quoted regex matcher
			matcher:  labels.MustNewMatcher(labels.MatchRegexp, "test", "^(?:a|b)$"),
			hasError: true,
		},
	}

	for _, tc := range cases {
		ir := &mockMatcherIndex{}
		_, err := postingsForMatcher(ir, tc.matcher)
		if tc.hasError {
			testutil.NotOk(t, err)
		} else {
			testutil.Ok(t, err)
		}
	}
}
