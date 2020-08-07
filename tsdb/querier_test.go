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
	"time"

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

// TODO(bwplotka): Replace those mocks with remote.concreteSeriesSet.
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

type mockChunkSeriesSet struct {
	next   func() bool
	series func() storage.ChunkSeries
	ws     func() storage.Warnings
	err    func() error
}

func (m *mockChunkSeriesSet) Next() bool                 { return m.next() }
func (m *mockChunkSeriesSet) At() storage.ChunkSeries    { return m.series() }
func (m *mockChunkSeriesSet) Err() error                 { return m.err() }
func (m *mockChunkSeriesSet) Warnings() storage.Warnings { return m.ws() }

func newMockChunkSeriesSet(list []storage.ChunkSeries) *mockChunkSeriesSet {
	i := -1
	return &mockChunkSeriesSet{
		next: func() bool {
			i++
			return i < len(list)
		},
		series: func() storage.ChunkSeries {
			return list[i]
		},
		err: func() error { return nil },
		ws:  func() storage.Warnings { return nil },
	}
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

type blockQuerierTestCase struct {
	mint, maxt int64
	ms         []*labels.Matcher
	exp        storage.SeriesSet
	expChks    storage.ChunkSeriesSet
}

func testBlockQuerier(t *testing.T, c blockQuerierTestCase, ir IndexReader, cr ChunkReader, stones *tombstones.MemTombstones) {
	t.Run("sample", func(t *testing.T) {
		q := blockQuerier{
			blockBaseQuerier: &blockBaseQuerier{
				index:      ir,
				chunks:     cr,
				tombstones: stones,

				mint: c.mint,
				maxt: c.maxt,
			},
		}

		res := q.Select(false, nil, c.ms...)
		defer func() { testutil.Ok(t, q.Close()) }()

		for {
			eok, rok := c.exp.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				testutil.Equals(t, 0, len(res.Warnings()))
				break
			}
			sexp := c.exp.At()
			sres := res.At()
			testutil.Equals(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(), nil)

			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, smplExp, smplRes)
		}
		testutil.Ok(t, res.Err())
	})

	t.Run("chunk", func(t *testing.T) {
		q := blockChunkQuerier{
			blockBaseQuerier: &blockBaseQuerier{
				index:      ir,
				chunks:     cr,
				tombstones: stones,

				mint: c.mint,
				maxt: c.maxt,
			},
		}
		res := q.Select(false, nil, c.ms...)
		defer func() { testutil.Ok(t, q.Close()) }()

		for {
			eok, rok := c.expChks.Next(), res.Next()
			testutil.Equals(t, eok, rok)

			if !eok {
				testutil.Equals(t, 0, len(res.Warnings()))
				break
			}
			sexpChks := c.expChks.At()
			sres := res.At()

			testutil.Equals(t, sexpChks.Labels(), sres.Labels())

			chksExp, errExp := storage.ExpandChunks(sexpChks.Iterator())
			rmChunkRefs(chksExp)
			chksRes, errRes := storage.ExpandChunks(sres.Iterator())
			rmChunkRefs(chksRes)
			testutil.Equals(t, errExp, errRes)
			testutil.Equals(t, chksExp, chksRes)
		}
		testutil.Ok(t, res.Err())
	})
}

func TestBlockQuerier(t *testing.T) {
	for _, c := range []blockQuerierTestCase{
		{
			mint:    0,
			maxt:    0,
			ms:      []*labels.Matcher{},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    0,
			maxt:    0,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    1,
			maxt:    0,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    math.MinInt64,
			maxt:    math.MaxInt64,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "x")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint: math.MinInt64,
			maxt: math.MaxInt64,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".*")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{1, 2}, sample{2, 3}, sample{3, 4}, sample{5, 2}, sample{6, 3}, sample{7, 4}},
				),
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 3}, sample{6, 6}},
				),
				storage.NewListSeries(labels.Labels{{Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 3}, sample{2, 2}, sample{3, 6}, sample{5, 1}, sample{6, 7}, sample{7, 2}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{1, 2}, sample{2, 3}, sample{3, 4}}, []tsdbutil.Sample{sample{5, 2}, sample{6, 3}, sample{7, 4}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}}, []tsdbutil.Sample{sample{5, 3}, sample{6, 6}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 3}, sample{2, 2}, sample{3, 6}}, []tsdbutil.Sample{sample{5, 1}, sample{6, 7}, sample{7, 2}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{2, 3}, sample{3, 4}, sample{5, 2}, sample{6, 3}},
				),
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{5, 3}, sample{6, 6}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{2, 3}, sample{3, 4}}, []tsdbutil.Sample{sample{5, 2}, sample{6, 3}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}}, []tsdbutil.Sample{sample{5, 3}, sample{6, 6}},
				),
			}),
		},
	} {
		t.Run("", func(t *testing.T) {
			ir, cr, _, _ := createIdxChkReaders(t, testData)
			testBlockQuerier(t, c, ir, cr, tombstones.NewMemTombstones())
		})
	}
}

func TestBlockQuerier_AgainstHeadWithOpenChunks(t *testing.T) {
	for _, c := range []blockQuerierTestCase{
		{
			mint:    0,
			maxt:    0,
			ms:      []*labels.Matcher{},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    0,
			maxt:    0,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    1,
			maxt:    0,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    math.MinInt64,
			maxt:    math.MaxInt64,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "x")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint: math.MinInt64,
			maxt: math.MaxInt64,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".*")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{1, 2}, sample{2, 3}, sample{3, 4}, sample{5, 2}, sample{6, 3}, sample{7, 4}},
				),
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 3}, sample{6, 6}},
				),
				storage.NewListSeries(labels.Labels{{Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 3}, sample{2, 2}, sample{3, 6}, sample{5, 1}, sample{6, 7}, sample{7, 2}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{1, 2}, sample{2, 3}, sample{3, 4}, sample{5, 2}, sample{6, 3}, sample{7, 4}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}, sample{5, 3}, sample{6, 6}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 3}, sample{2, 2}, sample{3, 6}, sample{5, 1}, sample{6, 7}, sample{7, 2}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{2, 3}, sample{3, 4}, sample{5, 2}, sample{6, 3}},
				),
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{5, 3}, sample{6, 6}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{2, 3}, sample{3, 4}, sample{5, 2}, sample{6, 3}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{2, 2}, sample{3, 3}, sample{5, 3}, sample{6, 6}},
				),
			}),
		},
	} {
		t.Run("", func(t *testing.T) {
			h, err := NewHead(nil, nil, nil, 2*time.Hour.Milliseconds(), "", nil, DefaultStripeSize, nil)
			testutil.Ok(t, err)
			defer h.Close()

			app := h.Appender(context.Background())
			for _, s := range testData {
				for _, chk := range s.chunks {
					for _, sample := range chk {
						_, err = app.Add(labels.FromMap(s.lset), sample.t, sample.v)
						testutil.Ok(t, err)
					}
				}
			}
			testutil.Ok(t, app.Commit())

			hr := NewRangeHead(h, c.mint, c.maxt)
			ir, err := hr.Index()
			testutil.Ok(t, err)
			defer ir.Close()

			cr, err := hr.Chunks()
			testutil.Ok(t, err)
			defer cr.Close()

			testBlockQuerier(t, c, ir, cr, tombstones.NewMemTombstones())
		})
	}
}

var testData = []seriesSamples{
	{
		lset: map[string]string{"a": "a"},
		chunks: [][]sample{
			{{1, 2}, {2, 3}, {3, 4}},
			{{5, 2}, {6, 3}, {7, 4}},
		},
	},
	{
		lset: map[string]string{"a": "a", "b": "b"},
		chunks: [][]sample{
			{{1, 1}, {2, 2}, {3, 3}},
			{{5, 3}, {6, 6}},
		},
	},
	{
		lset: map[string]string{"b": "b"},
		chunks: [][]sample{
			{{1, 3}, {2, 2}, {3, 6}},
			{{5, 1}, {6, 7}, {7, 2}},
		},
	},
}

func TestBlockQuerierDelete(t *testing.T) {
	stones := tombstones.NewTestMemTombstones([]tombstones.Intervals{
		{{Mint: 1, Maxt: 3}},
		{{Mint: 1, Maxt: 3}, {Mint: 6, Maxt: 10}},
		{{Mint: 6, Maxt: 10}},
	})

	for _, c := range []blockQuerierTestCase{
		{
			mint:    0,
			maxt:    0,
			ms:      []*labels.Matcher{},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    0,
			maxt:    0,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    1,
			maxt:    0,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint:    math.MinInt64,
			maxt:    math.MaxInt64,
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "x")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
		},
		{
			mint: math.MinInt64,
			maxt: math.MaxInt64,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".*")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{5, 2}, sample{6, 3}, sample{7, 4}},
				),
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{5, 3}},
				),
				storage.NewListSeries(labels.Labels{{Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 3}, sample{2, 2}, sample{3, 6}, sample{5, 1}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{5, 2}, sample{6, 3}, sample{7, 4}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{5, 3}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{1, 3}, sample{2, 2}, sample{3, 6}}, []tsdbutil.Sample{sample{5, 1}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{5, 2}, sample{6, 3}},
				),
				storage.NewListSeries(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{5, 3}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}},
					[]tsdbutil.Sample{sample{5, 2}, sample{6, 3}},
				),
				storage.NewListChunkSeriesFromSamples(labels.Labels{{Name: "a", Value: "a"}, {Name: "b", Value: "b"}},
					[]tsdbutil.Sample{sample{5, 3}},
				),
			}),
		},
	} {
		t.Run("", func(t *testing.T) {
			ir, cr, _, _ := createIdxChkReaders(t, testData)
			testBlockQuerier(t, c, ir, cr, stones)
		})
	}
}

type fakeChunksReader struct {
	ChunkReader
	chks map[uint64]chunkenc.Chunk
}

func createFakeReaderAndNotPopulatedChunks(s ...[]tsdbutil.Sample) (*fakeChunksReader, []chunks.Meta) {
	f := &fakeChunksReader{
		chks: map[uint64]chunkenc.Chunk{},
	}
	chks := make([]chunks.Meta, 0, len(s))

	for ref, samples := range s {
		chk := tsdbutil.ChunkFromSamples(samples)
		f.chks[uint64(ref)] = chk.Chunk

		chks = append(chks, chunks.Meta{
			Ref:     uint64(ref),
			MinTime: chk.MinTime,
			MaxTime: chk.MaxTime,
		})
	}
	return f, chks
}

func (r *fakeChunksReader) Chunk(ref uint64) (chunkenc.Chunk, error) {
	chk, ok := r.chks[ref]
	if !ok {
		return nil, errors.Errorf("chunk not found at ref %v", ref)
	}
	return chk, nil
}

func TestPopulateWithTombSeriesIterators(t *testing.T) {
	cases := []struct {
		name string
		chks [][]tsdbutil.Sample

		expected     []tsdbutil.Sample
		expectedChks []chunks.Meta

		intervals tombstones.Intervals

		// Seek being zero means do not test seek.
		seek        int64
		seekSuccess bool
	}{
		{
			name: "no chunk",
			chks: [][]tsdbutil.Sample{},
		},
		{
			name: "one empty chunk", // This should never happen.
			chks: [][]tsdbutil.Sample{{}},

			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{}),
			},
		},
		{
			name: "three empty chunks", // This should never happen.
			chks: [][]tsdbutil.Sample{{}, {}, {}},

			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{}),
			},
		},
		{
			name: "one chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}},
			},

			expected: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1},
				}),
			},
		},
		{
			name: "two full chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},

			expected: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89}, sample{9, 8},
				}),
			},
		},
		{
			name: "three full chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
				{sample{10, 22}, sample{203, 3493}},
			},

			expected: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8}, sample{10, 22}, sample{203, 3493},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89}, sample{9, 8},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{10, 22}, sample{203, 3493},
				}),
			},
		},
		// Seek cases.
		{
			name: "three empty chunks and seek", // This should never happen.
			chks: [][]tsdbutil.Sample{{}, {}, {}},
			seek: 1,

			seekSuccess: false,
		},
		{
			name: "two chunks and seek beyond chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},
			seek: 10,

			seekSuccess: false,
		},
		{
			name: "two chunks and seek on middle of first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},
			seek: 2,

			seekSuccess: true,
			expected: []tsdbutil.Sample{
				sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8},
			},
		},
		{
			name: "two chunks and seek before first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},
			seek: -32,

			seekSuccess: true,
			expected: []tsdbutil.Sample{
				sample{1, 2}, sample{3, 5}, sample{6, 1}, sample{7, 89}, sample{9, 8},
			},
		},
		// Deletion / Trim cases.
		{
			name:      "no chunk with deletion interval",
			chks:      [][]tsdbutil.Sample{},
			intervals: tombstones.Intervals{{Mint: 20, Maxt: 21}},
		},
		{
			name: "two chunks with trimmed first and last samples from edge chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},
			intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 9, Maxt: math.MaxInt64}),

			expected: []tsdbutil.Sample{
				sample{3, 5}, sample{6, 1}, sample{7, 89},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{3, 5}, sample{6, 1},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89},
				}),
			},
		},
		{
			name: "two chunks with trimmed middle sample of first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},
			intervals: tombstones.Intervals{{Mint: 2, Maxt: 3}},

			expected: []tsdbutil.Sample{
				sample{1, 2}, sample{6, 1}, sample{7, 89}, sample{9, 8},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2}, sample{6, 1},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89}, sample{9, 8},
				}),
			},
		},
		{
			name: "two chunks with deletion across two chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},
			intervals: tombstones.Intervals{{Mint: 6, Maxt: 7}},

			expected: []tsdbutil.Sample{
				sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{9, 8},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2}, sample{2, 3}, sample{3, 5},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{9, 8},
				}),
			},
		},
		// Deletion with seek.
		{
			name: "two chunks with trimmed first and last samples from edge chunks, seek from middle of first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2}, sample{2, 3}, sample{3, 5}, sample{6, 1}},
				{sample{7, 89}, sample{9, 8}},
			},
			intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 9, Maxt: math.MaxInt64}),

			seek:        3,
			seekSuccess: true,
			expected: []tsdbutil.Sample{
				sample{3, 5}, sample{6, 1}, sample{7, 89},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("sample", func(t *testing.T) {
				f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
				it := newPopulateWithDelGenericSeriesIterator(f, chkMetas, tc.intervals).toSeriesIterator()

				var r []tsdbutil.Sample
				if tc.seek != 0 {
					testutil.Equals(t, tc.seekSuccess, it.Seek(tc.seek))
					testutil.Equals(t, tc.seekSuccess, it.Seek(tc.seek)) // Next one should be noop.

					if tc.seekSuccess {
						// After successful seek iterator is ready. Grab the value.
						t, v := it.At()
						r = append(r, sample{t: t, v: v})
					}
				}
				expandedResult, err := storage.ExpandSamples(it, newSample)
				testutil.Ok(t, err)
				r = append(r, expandedResult...)
				testutil.Equals(t, tc.expected, r)
			})
			t.Run("chunk", func(t *testing.T) {
				f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
				it := newPopulateWithDelGenericSeriesIterator(f, chkMetas, tc.intervals).toChunkSeriesIterator()

				if tc.seek != 0 {
					// Chunk iterator does not have Seek method.
					return
				}
				expandedResult, err := storage.ExpandChunks(it)
				testutil.Ok(t, err)

				// We don't care about ref IDs for comparison, only chunk's samples matters.
				rmChunkRefs(expandedResult)
				rmChunkRefs(tc.expectedChks)
				testutil.Equals(t, tc.expectedChks, expandedResult)
			})
		})
	}
}

func rmChunkRefs(chks []chunks.Meta) {
	for i := range chks {
		chks[i].Ref = 0
	}
}

// Regression for: https://github.com/prometheus/tsdb/pull/97
func TestPopulateWithDelSeriesIterator_DoubleSeek(t *testing.T) {
	f, chkMetas := createFakeReaderAndNotPopulatedChunks(
		[]tsdbutil.Sample{},
		[]tsdbutil.Sample{sample{1, 1}, sample{2, 2}, sample{3, 3}},
		[]tsdbutil.Sample{sample{4, 4}, sample{5, 5}},
	)

	it := newPopulateWithDelGenericSeriesIterator(f, chkMetas, nil).toSeriesIterator()
	testutil.Assert(t, it.Seek(1), "")
	testutil.Assert(t, it.Seek(2), "")
	testutil.Assert(t, it.Seek(2), "")
	ts, v := it.At()
	testutil.Equals(t, int64(2), ts)
	testutil.Equals(t, float64(2), v)
}

// Regression when seeked chunks were still found via binary search and we always
// skipped to the end when seeking a value in the current chunk.
func TestPopulateWithDelSeriesIterator_SeekInCurrentChunk(t *testing.T) {
	f, chkMetas := createFakeReaderAndNotPopulatedChunks(
		[]tsdbutil.Sample{},
		[]tsdbutil.Sample{sample{1, 2}, sample{3, 4}, sample{5, 6}, sample{7, 8}},
		[]tsdbutil.Sample{},
	)

	it := newPopulateWithDelGenericSeriesIterator(f, chkMetas, nil).toSeriesIterator()
	testutil.Assert(t, it.Next(), "")
	ts, v := it.At()
	testutil.Equals(t, int64(1), ts)
	testutil.Equals(t, float64(2), v)

	testutil.Assert(t, it.Seek(4), "")
	ts, v = it.At()
	testutil.Equals(t, int64(5), ts)
	testutil.Equals(t, float64(6), v)
}

func TestPopulateWithDelSeriesIterator_SeekWithMinTime(t *testing.T) {
	f, chkMetas := createFakeReaderAndNotPopulatedChunks(
		[]tsdbutil.Sample{sample{1, 6}, sample{5, 6}, sample{6, 8}},
	)

	it := newPopulateWithDelGenericSeriesIterator(f, chkMetas, nil).toSeriesIterator()
	testutil.Equals(t, false, it.Seek(7))
	testutil.Equals(t, true, it.Seek(3))
}

// Regression when calling Next() with a time bounded to fit within two samples.
// Seek gets called and advances beyond the max time, which was just accepted as a valid sample.
func TestPopulateWithDelSeriesIterator_NextWithMinTime(t *testing.T) {
	f, chkMetas := createFakeReaderAndNotPopulatedChunks(
		[]tsdbutil.Sample{sample{1, 6}, sample{5, 6}, sample{7, 8}},
	)

	it := newPopulateWithDelGenericSeriesIterator(
		f, chkMetas, tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 4, Maxt: math.MaxInt64}),
	).toSeriesIterator()
	testutil.Equals(t, false, it.Next())
}

// Test the cost of merging series sets for different number of merged sets and their size.
// The subset are all equivalent so this does not capture merging of partial or non-overlapping sets well.
// TODO(bwplotka): Merge with storage merged series set benchmark.
func BenchmarkMergedSeriesSet(b *testing.B) {
	var sel = func(sets []storage.SeriesSet) storage.SeriesSet {
		return storage.NewMergeSeriesSet(sets, storage.ChainedSeriesMerge)
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
						in[j] = append(in[j], storage.NewListSeries(l2, nil))
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

				qblocks := make([]storage.Querier, 0, len(blocks))
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					testutil.Ok(b, err)
					qblocks = append(qblocks, q)
				}

				sq := storage.NewMergeQuerier(qblocks, nil, storage.ChainedSeriesMerge)
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

				qblocks := make([]storage.Querier, 0, len(blocks))
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					testutil.Ok(b, err)
					qblocks = append(qblocks, q)
				}

				sq := storage.NewMergeQuerier(qblocks, nil, storage.ChainedSeriesMerge)
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

		qblocks := make([]storage.Querier, 0, len(blocks))
		for _, blk := range blocks {
			q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
			testutil.Ok(b, err)
			qblocks = append(qblocks, q)
		}

		sq := storage.NewMergeQuerier(qblocks, nil, storage.ChainedSeriesMerge)
		defer sq.Close()

		benchMsg := fmt.Sprintf("nSeries=%d,nBlocks=%d,cardinality=%d,pattern=\"%s\"", c.numSeries, c.numBlocks, c.cardinality, c.pattern)
		b.Run(benchMsg, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				ss := sq.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "test", c.pattern))
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

	app := h.Appender(context.Background())
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

				series := genSeries(nSeries, 5, 1, nSamples)

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
						s := series[i].(*storage.SeriesEntry)
						allLabels := append(commonLbls, s.Labels()...)
						newS := storage.NewListSeries(allLabels, nil)
						newS.SampleIteratorFn = s.SampleIteratorFn

						series[i] = newS
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

				queryTypes["_1-Block"] = storage.NewMergeQuerier(qs[:1], nil, storage.ChainedSeriesMerge)
				queryTypes["_3-Blocks"] = storage.NewMergeQuerier(qs[0:3], nil, storage.ChainedSeriesMerge)
				queryTypes["_10-Blocks"] = storage.NewMergeQuerier(qs, nil, storage.ChainedSeriesMerge)

				chunkDir, err := ioutil.TempDir("", "chunk_dir")
				testutil.Ok(b, err)
				defer func() {
					testutil.Ok(b, os.RemoveAll(chunkDir))
				}()
				head := createHead(b, nil, series, chunkDir)
				qHead, err := NewBlockQuerier(head, 1, nSamples)
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

func TestBlockBaseSeriesSet(t *testing.T) {
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

		bcs := &blockBaseSeriesSet{
			p:          index.NewListPostings(tc.postings),
			index:      mi,
			tombstones: tombstones.NewMemTombstones(),
		}

		i := 0
		for bcs.Next() {
			chks := bcs.currIterFn().chks
			idx := tc.expIdxs[i]

			testutil.Equals(t, tc.series[idx].lset, bcs.currLabels)
			testutil.Equals(t, tc.series[idx].chunks, chks)

			i++
		}
		testutil.Equals(t, len(tc.expIdxs), i)
		testutil.Ok(t, bcs.Err())
	}
}
