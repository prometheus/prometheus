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
	"math"
	"math/rand"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
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
	chkReader := mockChunkReader(make(map[chunks.ChunkRef]chunkenc.Chunk))
	lblIdx := make(map[string]map[string]struct{})
	mi := newMockIndex()
	blockMint := int64(math.MaxInt64)
	blockMaxt := int64(math.MinInt64)

	var chunkRef chunks.ChunkRef
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
		require.NoError(t, mi.AddSeries(storage.SeriesRef(i), ls, metas...))

		postings.Add(storage.SeriesRef(i), ls)

		ls.Range(func(l labels.Label) {
			vs, present := lblIdx[l.Name]
			if !present {
				vs = map[string]struct{}{}
				lblIdx[l.Name] = vs
			}
			vs[l.Value] = struct{}{}
		})
	}

	require.NoError(t, postings.Iter(func(l labels.Label, p index.Postings) error {
		return mi.WritePostings(l.Name, l.Value, p)
	}))
	return mi, chkReader, blockMint, blockMaxt
}

type blockQuerierTestCase struct {
	mint, maxt int64
	ms         []*labels.Matcher
	hints      *storage.SelectHints
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

		res := q.Select(false, c.hints, c.ms...)
		defer func() { require.NoError(t, q.Close()) }()

		for {
			eok, rok := c.exp.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				require.Equal(t, 0, len(res.Warnings()))
				break
			}
			sexp := c.exp.At()
			sres := res.At()
			require.Equal(t, sexp.Labels(), sres.Labels())

			smplExp, errExp := storage.ExpandSamples(sexp.Iterator(nil), nil)
			smplRes, errRes := storage.ExpandSamples(sres.Iterator(nil), nil)

			require.Equal(t, errExp, errRes)
			require.Equal(t, smplExp, smplRes)
		}
		require.NoError(t, res.Err())
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
		res := q.Select(false, c.hints, c.ms...)
		defer func() { require.NoError(t, q.Close()) }()

		for {
			eok, rok := c.expChks.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				require.Equal(t, 0, len(res.Warnings()))
				break
			}
			sexpChks := c.expChks.At()
			sres := res.At()

			require.Equal(t, sexpChks.Labels(), sres.Labels())

			chksExp, errExp := storage.ExpandChunks(sexpChks.Iterator(nil))
			rmChunkRefs(chksExp)
			chksRes, errRes := storage.ExpandChunks(sres.Iterator(nil))
			rmChunkRefs(chksRes)
			require.Equal(t, errExp, errRes)

			require.Equal(t, len(chksExp), len(chksRes))
			var exp, act [][]tsdbutil.Sample
			for i := range chksExp {
				samples, err := storage.ExpandSamples(chksExp[i].Chunk.Iterator(nil), nil)
				require.NoError(t, err)
				exp = append(exp, samples)
				samples, err = storage.ExpandSamples(chksRes[i].Chunk.Iterator(nil), nil)
				require.NoError(t, err)
				act = append(act, samples)
			}

			require.Equal(t, exp, act)
		}
		require.NoError(t, res.Err())
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
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}}, []tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}}, []tsdbutil.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}}, []tsdbutil.Sample{sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}}, []tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}}, []tsdbutil.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
		},
		{
			// This test runs a query disabling trimming. All chunks containing at least 1 sample within the queried
			// time range will be returned.
			mint:  2,
			maxt:  6,
			hints: &storage.SelectHints{Start: 2, End: 6, DisableTrimming: true},
			ms:    []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}},
					[]tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}},
					[]tsdbutil.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
		},
		{
			// This test runs a query disabling trimming. All chunks containing at least 1 sample within the queried
			// time range will be returned.
			mint:  5,
			maxt:  6,
			hints: &storage.SelectHints{Start: 5, End: 6, DisableTrimming: true},
			ms:    []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
		},
		{
			// This tests query sharding. The label sets involved both hash into this test's result set. The test
			// following this is companion to this test (same test but with a different ShardIndex) and should find that
			// the label sets involved do not hash to that test's result set.
			mint:  math.MinInt64,
			maxt:  math.MaxInt64,
			hints: &storage.SelectHints{Start: math.MinInt64, End: math.MaxInt64, ShardIndex: 0, ShardCount: 2},
			ms:    []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".*")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}}, []tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}}, []tsdbutil.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}}, []tsdbutil.Sample{sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
		},
		{
			// This is a companion to the test above.
			mint:    math.MinInt64,
			maxt:    math.MaxInt64,
			hints:   &storage.SelectHints{Start: math.MinInt64, End: math.MaxInt64, ShardIndex: 1, ShardCount: 2},
			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".*")},
			exp:     newMockSeriesSet([]storage.Series{}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
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
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
		},
	} {
		t.Run("", func(t *testing.T) {
			opts := DefaultHeadOptions()
			opts.ChunkRange = 2 * time.Hour.Milliseconds()
			h, err := NewHead(nil, nil, nil, nil, opts, nil)
			require.NoError(t, err)
			defer h.Close()

			app := h.Appender(context.Background())
			for _, s := range testData {
				for _, chk := range s.chunks {
					for _, sample := range chk {
						_, err = app.Append(0, labels.FromMap(s.lset), sample.t, sample.v)
						require.NoError(t, err)
					}
				}
			}
			require.NoError(t, app.Commit())

			hr := NewRangeHead(h, c.mint, c.maxt)
			ir, err := hr.Index()
			require.NoError(t, err)
			defer ir.Close()

			cr, err := hr.Chunks()
			require.NoError(t, err)
			defer cr.Close()

			testBlockQuerier(t, c, ir, cr, tombstones.NewMemTombstones())
		})
	}
}

var testData = []seriesSamples{
	{
		lset: map[string]string{"a": "a"},
		chunks: [][]sample{
			{{1, 2, nil, nil}, {2, 3, nil, nil}, {3, 4, nil, nil}},
			{{5, 2, nil, nil}, {6, 3, nil, nil}, {7, 4, nil, nil}},
		},
	},
	{
		lset: map[string]string{"a": "a", "b": "b"},
		chunks: [][]sample{
			{{1, 1, nil, nil}, {2, 2, nil, nil}, {3, 3, nil, nil}},
			{{5, 3, nil, nil}, {6, 6, nil, nil}},
		},
	},
	{
		lset: map[string]string{"b": "b"},
		chunks: [][]sample{
			{{1, 3, nil, nil}, {2, 2, nil, nil}, {3, 6, nil, nil}},
			{{5, 1, nil, nil}, {6, 7, nil, nil}, {7, 2, nil, nil}},
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
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{5, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{5, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("b", "b"),
					[]tsdbutil.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}}, []tsdbutil.Sample{sample{5, 1, nil, nil}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{5, 3, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]tsdbutil.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]tsdbutil.Sample{sample{5, 3, nil, nil}},
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
	chks map[chunks.ChunkRef]chunkenc.Chunk
}

func createFakeReaderAndNotPopulatedChunks(s ...[]tsdbutil.Sample) (*fakeChunksReader, []chunks.Meta) {
	f := &fakeChunksReader{
		chks: map[chunks.ChunkRef]chunkenc.Chunk{},
	}
	chks := make([]chunks.Meta, 0, len(s))

	for ref, samples := range s {
		chk := tsdbutil.ChunkFromSamples(samples)
		f.chks[chunks.ChunkRef(ref)] = chk.Chunk

		chks = append(chks, chunks.Meta{
			Ref:     chunks.ChunkRef(ref),
			MinTime: chk.MinTime,
			MaxTime: chk.MaxTime,
		})
	}
	return f, chks
}

func (r *fakeChunksReader) Chunk(meta chunks.Meta) (chunkenc.Chunk, error) {
	chk, ok := r.chks[meta.Ref]
	if !ok {
		return nil, errors.Errorf("chunk not found at ref %v", meta.Ref)
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
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
			},

			expected: []tsdbutil.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
			},
		},
		{
			name: "two full chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},

			expected: []tsdbutil.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
				}),
			},
		},
		{
			name: "three full chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
				{sample{10, 22, nil, nil}, sample{203, 3493, nil, nil}},
			},

			expected: []tsdbutil.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil}, sample{10, 22, nil, nil}, sample{203, 3493, nil, nil},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{10, 22, nil, nil}, sample{203, 3493, nil, nil},
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
				{sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			seek: 10,

			seekSuccess: false,
		},
		{
			name: "two chunks and seek on middle of first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			seek: 2,

			seekSuccess: true,
			expected: []tsdbutil.Sample{
				sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
		},
		{
			name: "two chunks and seek before first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			seek: -32,

			seekSuccess: true,
			expected: []tsdbutil.Sample{
				sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
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
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 9, Maxt: math.MaxInt64}),

			expected: []tsdbutil.Sample{
				sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89, nil, nil},
				}),
			},
		},
		{
			name: "two chunks with trimmed middle sample of first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: 2, Maxt: 3}},

			expected: []tsdbutil.Sample{
				sample{1, 2, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2, nil, nil}, sample{6, 1, nil, nil},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
				}),
			},
		},
		{
			name: "two chunks with deletion across two chunks",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: 6, Maxt: 7}},

			expected: []tsdbutil.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{9, 8, nil, nil},
			},
			expectedChks: []chunks.Meta{
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil},
				}),
				tsdbutil.ChunkFromSamples([]tsdbutil.Sample{
					sample{9, 8, nil, nil},
				}),
			},
		},
		// Deletion with seek.
		{
			name: "two chunks with trimmed first and last samples from edge chunks, seek from middle of first chunk",
			chks: [][]tsdbutil.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 9, Maxt: math.MaxInt64}),

			seek:        3,
			seekSuccess: true,
			expected: []tsdbutil.Sample{
				sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("sample", func(t *testing.T) {
				f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
				it := &populateWithDelSeriesIterator{}
				it.reset(ulid.ULID{}, f, chkMetas, tc.intervals)

				var r []tsdbutil.Sample
				if tc.seek != 0 {
					require.Equal(t, tc.seekSuccess, it.Seek(tc.seek) == chunkenc.ValFloat)
					require.Equal(t, tc.seekSuccess, it.Seek(tc.seek) == chunkenc.ValFloat) // Next one should be noop.

					if tc.seekSuccess {
						// After successful seek iterator is ready. Grab the value.
						t, v := it.At()
						r = append(r, sample{t: t, v: v})
					}
				}
				expandedResult, err := storage.ExpandSamples(it, newSample)
				require.NoError(t, err)
				r = append(r, expandedResult...)
				require.Equal(t, tc.expected, r)
			})
			t.Run("chunk", func(t *testing.T) {
				f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
				it := &populateWithDelChunkSeriesIterator{}
				it.reset(ulid.ULID{}, f, chkMetas, tc.intervals)

				if tc.seek != 0 {
					// Chunk iterator does not have Seek method.
					return
				}
				expandedResult, err := storage.ExpandChunks(it)
				require.NoError(t, err)

				// We don't care about ref IDs for comparison, only chunk's samples matters.
				rmChunkRefs(expandedResult)
				rmChunkRefs(tc.expectedChks)
				require.Equal(t, tc.expectedChks, expandedResult)
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
		[]tsdbutil.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}},
		[]tsdbutil.Sample{sample{4, 4, nil, nil}, sample{5, 5, nil, nil}},
	)

	it := &populateWithDelSeriesIterator{}
	it.reset(ulid.ULID{}, f, chkMetas, nil)
	require.Equal(t, chunkenc.ValFloat, it.Seek(1))
	require.Equal(t, chunkenc.ValFloat, it.Seek(2))
	require.Equal(t, chunkenc.ValFloat, it.Seek(2))
	ts, v := it.At()
	require.Equal(t, int64(2), ts)
	require.Equal(t, float64(2), v)
}

// Regression when seeked chunks were still found via binary search and we always
// skipped to the end when seeking a value in the current chunk.
func TestPopulateWithDelSeriesIterator_SeekInCurrentChunk(t *testing.T) {
	f, chkMetas := createFakeReaderAndNotPopulatedChunks(
		[]tsdbutil.Sample{},
		[]tsdbutil.Sample{sample{1, 2, nil, nil}, sample{3, 4, nil, nil}, sample{5, 6, nil, nil}, sample{7, 8, nil, nil}},
		[]tsdbutil.Sample{},
	)

	it := &populateWithDelSeriesIterator{}
	it.reset(ulid.ULID{}, f, chkMetas, nil)
	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, v := it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, float64(2), v)

	require.Equal(t, chunkenc.ValFloat, it.Seek(4))
	ts, v = it.At()
	require.Equal(t, int64(5), ts)
	require.Equal(t, float64(6), v)
}

func TestPopulateWithDelSeriesIterator_SeekWithMinTime(t *testing.T) {
	f, chkMetas := createFakeReaderAndNotPopulatedChunks(
		[]tsdbutil.Sample{sample{1, 6, nil, nil}, sample{5, 6, nil, nil}, sample{6, 8, nil, nil}},
	)

	it := &populateWithDelSeriesIterator{}
	it.reset(ulid.ULID{}, f, chkMetas, nil)
	require.Equal(t, chunkenc.ValNone, it.Seek(7))
	require.Equal(t, chunkenc.ValFloat, it.Seek(3))
}

// Regression when calling Next() with a time bounded to fit within two samples.
// Seek gets called and advances beyond the max time, which was just accepted as a valid sample.
func TestPopulateWithDelSeriesIterator_NextWithMinTime(t *testing.T) {
	f, chkMetas := createFakeReaderAndNotPopulatedChunks(
		[]tsdbutil.Sample{sample{1, 6, nil, nil}, sample{5, 6, nil, nil}, sample{7, 8, nil, nil}},
	)

	it := &populateWithDelSeriesIterator{}
	it.reset(ulid.ULID{}, f, chkMetas, tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 4, Maxt: math.MaxInt64}))
	require.Equal(t, chunkenc.ValNone, it.Next())
}

// Test the cost of merging series sets for different number of merged sets and their size.
// The subset are all equivalent so this does not capture merging of partial or non-overlapping sets well.
// TODO(bwplotka): Merge with storage merged series set benchmark.
func BenchmarkMergedSeriesSet(b *testing.B) {
	sel := func(sets []storage.SeriesSet) storage.SeriesSet {
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
				require.NoError(b, err)

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
					require.NoError(b, ms.Err())
					require.Equal(b, len(lbls), i)
				}
			})
		}
	}
}

type mockChunkReader map[chunks.ChunkRef]chunkenc.Chunk

func (cr mockChunkReader) Chunk(meta chunks.Meta) (chunkenc.Chunk, error) {
	chk, ok := cr[meta.Ref]
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
	require.NoError(t, err)
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
		it := &DeletedIterator{Iter: chk.Iterator(nil), Intervals: c.r[:]}
		ranges := c.r[:]
		for it.Next() == chunkenc.ValFloat {
			i++
			for _, tr := range ranges {
				if tr.InBounds(i) {
					i = tr.Maxt + 1
					ranges = ranges[1:]
				}
			}

			require.Less(t, i, int64(1000))

			ts, v := it.At()
			require.Equal(t, act[i].t, ts)
			require.Equal(t, act[i].v, v)
		}
		// There has been an extra call to Next().
		i++
		for _, tr := range ranges {
			if tr.InBounds(i) {
				i = tr.Maxt + 1
				ranges = ranges[1:]
			}
		}

		require.GreaterOrEqual(t, i, int64(1000))
		require.NoError(t, it.Err())
	}
}

func TestDeletedIterator_WithSeek(t *testing.T) {
	chk := chunkenc.NewXORChunk()
	app, err := chk.Appender()
	require.NoError(t, err)
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
		it := &DeletedIterator{Iter: chk.Iterator(nil), Intervals: c.r[:]}

		require.Equal(t, c.ok, it.Seek(c.seek) == chunkenc.ValFloat)
		if c.ok {
			ts := it.AtT()
			require.Equal(t, c.seekedTs, ts)
		}
	}
}

type series struct {
	l      labels.Labels
	chunks []chunks.Meta
}

type mockIndex struct {
	series   map[storage.SeriesRef]series
	postings map[labels.Label][]storage.SeriesRef
	symbols  map[string]struct{}
}

func newMockIndex() mockIndex {
	ix := mockIndex{
		series:   make(map[storage.SeriesRef]series),
		postings: make(map[labels.Label][]storage.SeriesRef),
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

func (m *mockIndex) AddSeries(ref storage.SeriesRef, l labels.Labels, chunks ...chunks.Meta) error {
	if _, ok := m.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	l.Range(func(lbl labels.Label) {
		m.symbols[lbl.Name] = struct{}{}
		m.symbols[lbl.Value] = struct{}{}
	})

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

func (m mockIndex) SortedLabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	values, _ := m.LabelValues(name, matchers...)
	sort.Strings(values)
	return values, nil
}

func (m mockIndex) LabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	var values []string

	if len(matchers) == 0 {
		for l := range m.postings {
			if l.Name == name {
				values = append(values, l.Value)
			}
		}
		return values, nil
	}

	for _, series := range m.series {
		for _, matcher := range matchers {
			if matcher.Matches(series.l.Get(matcher.Name)) {
				// TODO(colega): shouldn't we check all the matchers before adding this to the values?
				values = append(values, series.l.Get(name))
			}
		}
	}

	return values, nil
}

func (m mockIndex) LabelValueFor(id storage.SeriesRef, label string) (string, error) {
	return m.series[id].l.Get(label), nil
}

func (m mockIndex) LabelNamesFor(ids ...storage.SeriesRef) ([]string, error) {
	namesMap := make(map[string]bool)
	for _, id := range ids {
		m.series[id].l.Range(func(lbl labels.Label) {
			namesMap[lbl.Name] = true
		})
	}
	names := make([]string, 0, len(namesMap))
	for name := range namesMap {
		names = append(names, name)
	}
	return names, nil
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

func (m mockIndex) PostingsForMatchers(concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	var ps []storage.SeriesRef
	for p, s := range m.series {
		if matches(ms, s.l) {
			ps = append(ps, p)
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i] < ps[j] })
	return index.NewListPostings(ps), nil
}

func matches(ms []*labels.Matcher, lbls labels.Labels) bool {
	lm := lbls.Map()
	for _, m := range ms {
		if !m.Matches(lm[m.Name]) {
			return false
		}
	}
	return true
}

func (m mockIndex) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	out := make([]storage.SeriesRef, 0, 128)

	for p.Next() {
		ref := p.At()
		s, ok := m.series[ref]
		if !ok {
			continue
		}

		// Check if the series belong to the shard.
		if s.l.Hash()%shardCount != shardIndex {
			continue
		}

		out = append(out, ref)
	}

	return index.NewListPostings(out)
}

func (m mockIndex) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	s, ok := m.series[ref]
	if !ok {
		return storage.ErrNotFound
	}
	builder.Assign(s.l)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

func (m mockIndex) LabelNames(matchers ...*labels.Matcher) ([]string, error) {
	names := map[string]struct{}{}
	if len(matchers) == 0 {
		for l := range m.postings {
			names[l.Name] = struct{}{}
		}
	} else {
		for _, series := range m.series {
			matches := true
			for _, matcher := range matchers {
				matches = matches || matcher.Matches(series.l.Get(matcher.Name))
				if !matches {
					break
				}
			}
			if matches {
				series.l.Range(func(lbl labels.Label) {
					names[lbl.Name] = struct{}{}
				})
			}
		}
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
				dir := b.TempDir()

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
					require.NoError(b, err)
					blocks = append(blocks, block)
					defer block.Close()
				}

				qblocks := make([]storage.Querier, 0, len(blocks))
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					require.NoError(b, err)
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
				dir := b.TempDir()

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
					require.NoError(b, err)
					blocks = append(blocks, block)
					defer block.Close()
				}

				qblocks := make([]storage.Querier, 0, len(blocks))
				for _, blk := range blocks {
					q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
					require.NoError(b, err)
					qblocks = append(qblocks, q)
				}

				sq := storage.NewMergeQuerier(qblocks, nil, storage.ChainedSeriesMerge)
				defer sq.Close()

				mint := blocks[0].meta.MinTime
				maxt := blocks[len(blocks)-1].meta.MaxTime

				b.ResetTimer()
				b.ReportAllocs()

				var it chunkenc.Iterator
				ss := sq.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
				for ss.Next() {
					it = ss.At().Iterator(it)
					for t := mint; t <= maxt; t++ {
						it.Seek(t)
					}
					require.NoError(b, it.Err())
				}
				require.NoError(b, ss.Err())
				require.Equal(b, 0, len(ss.Warnings()))
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
		dir := b.TempDir()

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
			require.NoError(b, err)
			blocks = append(blocks, block)
			defer block.Close()
		}

		qblocks := make([]storage.Querier, 0, len(blocks))
		for _, blk := range blocks {
			q, err := NewBlockQuerier(blk, math.MinInt64, math.MaxInt64)
			require.NoError(b, err)
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
				require.NoError(b, ss.Err())
				require.Equal(b, 0, len(ss.Warnings()))
			}
		})
	}
}

func TestPostingsForMatchers(t *testing.T) {
	chunkDir := t.TempDir()
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = chunkDir
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, h.Close())
	}()

	app := h.Appender(context.Background())
	app.Append(0, labels.FromStrings("n", "1"), 0, 0)
	app.Append(0, labels.FromStrings("n", "1", "i", "a"), 0, 0)
	app.Append(0, labels.FromStrings("n", "1", "i", "b"), 0, 0)
	app.Append(0, labels.FromStrings("n", "2"), 0, 0)
	app.Append(0, labels.FromStrings("n", "2.5"), 0, 0)
	require.NoError(t, app.Commit())

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
	require.NoError(t, err)

	for _, c := range cases {
		exp := map[string]struct{}{}
		for _, l := range c.exp {
			exp[l.String()] = struct{}{}
		}
		p, err := PostingsForMatchers(ir, c.matchers...)
		require.NoError(t, err)

		var builder labels.ScratchBuilder
		for p.Next() {
			require.NoError(t, ir.Series(p.At(), &builder, &[]chunks.Meta{}))
			lbls := builder.Labels()
			if _, ok := exp[lbls.String()]; !ok {
				t.Errorf("Evaluating %v, unexpected result %s", c.matchers, lbls.String())
			} else {
				delete(exp, lbls.String())
			}
		}
		require.NoError(t, p.Err())
		if len(exp) != 0 {
			t.Errorf("Evaluating %v, missing results %+v", c.matchers, exp)
		}
	}
}

// TestClose ensures that calling Close more than once doesn't block and doesn't panic.
func TestClose(t *testing.T) {
	dir := t.TempDir()

	createBlock(t, dir, genSeries(1, 1, 0, 10))
	createBlock(t, dir, genSeries(1, 1, 10, 20))

	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	if err != nil {
		t.Fatalf("Opening test storage failed: %s", err)
	}
	defer func() {
		require.NoError(t, db.Close())
	}()

	q, err := db.Querier(context.TODO(), 0, 20)
	require.NoError(t, err)
	require.NoError(t, q.Close())
	require.Error(t, q.Close())
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

	type qt struct {
		typ     string
		querier storage.Querier
	}
	var queryTypes []qt // We use a slice instead of map to keep the order of test cases consistent.
	defer func() {
		for _, q := range queryTypes {
			// Can't run a check for error here as some of these will fail as
			// queryTypes is using the same slice for the different block queriers
			// and would have been closed in the previous iteration.
			q.querier.Close()
		}
	}()

	for title, selectors := range cases {
		for _, nSeries := range []int{10} {
			for _, nSamples := range []int64{1000, 10000, 100000} {
				dir := b.TempDir()

				series := genSeries(nSeries, 5, 1, nSamples)

				// Add some common labels to make the matchers select these series.
				{
					var commonLbls []labels.Label
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
						allLabels := commonLbls
						s.Labels().Range(func(l labels.Label) {
							allLabels = append(allLabels, l)
						})
						newS := storage.NewListSeries(labels.New(allLabels...), nil)
						newS.SampleIteratorFn = s.SampleIteratorFn

						series[i] = newS
					}
				}

				qs := make([]storage.Querier, 0, 10)
				for x := 0; x <= 10; x++ {
					block, err := OpenBlock(nil, createBlock(b, dir, series), nil)
					require.NoError(b, err)
					q, err := NewBlockQuerier(block, 1, int64(nSamples))
					require.NoError(b, err)
					qs = append(qs, q)
				}

				queryTypes = append(queryTypes, qt{"_1-Block", storage.NewMergeQuerier(qs[:1], nil, storage.ChainedSeriesMerge)})
				queryTypes = append(queryTypes, qt{"_3-Blocks", storage.NewMergeQuerier(qs[0:3], nil, storage.ChainedSeriesMerge)})
				queryTypes = append(queryTypes, qt{"_10-Blocks", storage.NewMergeQuerier(qs, nil, storage.ChainedSeriesMerge)})

				chunkDir := b.TempDir()
				head := createHead(b, nil, series, chunkDir)
				qHead, err := NewBlockQuerier(NewRangeHead(head, 1, nSamples), 1, nSamples)
				require.NoError(b, err)
				queryTypes = append(queryTypes, qt{"_Head", qHead})

				for _, oooPercentage := range []int{1, 3, 5, 10} {
					chunkDir := b.TempDir()
					totalOOOSamples := oooPercentage * int(nSamples) / 100
					oooSampleFrequency := int(nSamples) / totalOOOSamples
					head := createHeadWithOOOSamples(b, nil, series, chunkDir, oooSampleFrequency)

					qHead, err := NewBlockQuerier(NewRangeHead(head, 1, nSamples), 1, nSamples)
					require.NoError(b, err)
					qOOOHead, err := NewBlockQuerier(NewOOORangeHead(head, 1, nSamples), 1, nSamples)
					require.NoError(b, err)

					queryTypes = append(queryTypes, qt{
						fmt.Sprintf("_Head_oooPercent:%d", oooPercentage),
						storage.NewMergeQuerier([]storage.Querier{qHead, qOOOHead}, nil, storage.ChainedSeriesMerge),
					})
				}

				for _, q := range queryTypes {
					b.Run(title+q.typ+"_nSeries:"+strconv.Itoa(nSeries)+"_nSamples:"+strconv.Itoa(int(nSamples)), func(b *testing.B) {
						expExpansions, err := strconv.Atoi(string(title[len(title)-1]))
						require.NoError(b, err)
						benchQuery(b, expExpansions, q.querier, selectors)
					})
				}
				require.NoError(b, head.Close())
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
		var it chunkenc.Iterator
		for ss.Next() {
			s := ss.At()
			s.Labels()
			it = s.Iterator(it)
			for it.Next() != chunkenc.ValNone {
				_, _ = it.At()
			}
			actualExpansions++
		}
		require.NoError(b, ss.Err())
		require.Equal(b, 0, len(ss.Warnings()))
		require.Equal(b, expExpansions, actualExpansions)
		require.NoError(b, ss.Err())
	}
}

// mockMatcherIndex is used to check if the regex matcher works as expected.
type mockMatcherIndex struct{}

func (m mockMatcherIndex) Symbols() index.StringIter { return nil }

func (m mockMatcherIndex) Close() error { return nil }

// SortedLabelValues will return error if it is called.
func (m mockMatcherIndex) SortedLabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	return []string{}, errors.New("sorted label values called")
}

// LabelValues will return error if it is called.
func (m mockMatcherIndex) LabelValues(name string, matchers ...*labels.Matcher) ([]string, error) {
	return []string{}, errors.New("label values called")
}

func (m mockMatcherIndex) LabelValueFor(id storage.SeriesRef, label string) (string, error) {
	return "", errors.New("label value for called")
}

func (m mockMatcherIndex) LabelNamesFor(ids ...storage.SeriesRef) ([]string, error) {
	return nil, errors.New("label names for for called")
}

func (m mockMatcherIndex) Postings(name string, values ...string) (index.Postings, error) {
	return index.EmptyPostings(), nil
}

func (m mockMatcherIndex) PostingsForMatchers(bool, ...*labels.Matcher) (index.Postings, error) {
	return index.EmptyPostings(), nil
}

func (m mockMatcherIndex) SortedPostings(p index.Postings) index.Postings {
	return index.EmptyPostings()
}

func (m mockMatcherIndex) ShardedPostings(ps index.Postings, shardIndex, shardCount uint64) index.Postings {
	return ps
}

func (m mockMatcherIndex) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	return nil
}

func (m mockMatcherIndex) LabelNames(...*labels.Matcher) ([]string, error) {
	return []string{}, nil
}

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
			hasError: false,
		},
	}

	for _, tc := range cases {
		ir := &mockMatcherIndex{}
		_, err := postingsForMatcher(ir, tc.matcher)
		if tc.hasError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestBlockBaseSeriesSet(t *testing.T) {
	type refdSeries struct {
		lset   labels.Labels
		chunks []chunks.Meta

		ref storage.SeriesRef
	}

	cases := []struct {
		series []refdSeries
		// Postings should be in the sorted order of the series
		postings []storage.SeriesRef

		expIdxs []int
	}{
		{
			series: []refdSeries{
				{
					lset: labels.FromStrings("a", "a"),
					chunks: []chunks.Meta{
						{Ref: 29},
						{Ref: 45},
						{Ref: 245},
						{Ref: 123},
						{Ref: 4232},
						{Ref: 5344},
						{Ref: 121},
					},
					ref: 12,
				},
				{
					lset: labels.FromStrings("a", "a", "b", "b"),
					chunks: []chunks.Meta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.FromStrings("b", "c"),
					chunks: []chunks.Meta{{Ref: 8282}},
					ref:    1,
				},
				{
					lset: labels.FromStrings("b", "b"),
					chunks: []chunks.Meta{
						{Ref: 829}, {Ref: 239}, {Ref: 2349}, {Ref: 659}, {Ref: 269},
					},
					ref: 108,
				},
			},
			postings: []storage.SeriesRef{12, 13, 10, 108}, // 13 doesn't exist and should just be skipped over.
			expIdxs:  []int{0, 1, 3},
		},
		{
			series: []refdSeries{
				{
					lset: labels.FromStrings("a", "a", "b", "b"),
					chunks: []chunks.Meta{
						{Ref: 82}, {Ref: 23}, {Ref: 234}, {Ref: 65}, {Ref: 26},
					},
					ref: 10,
				},
				{
					lset:   labels.FromStrings("b", "c"),
					chunks: []chunks.Meta{{Ref: 8282}},
					ref:    3,
				},
			},
			postings: []storage.SeriesRef{},
			expIdxs:  []int{},
		},
	}

	for _, tc := range cases {
		mi := newMockIndex()
		for _, s := range tc.series {
			require.NoError(t, mi.AddSeries(s.ref, s.lset, s.chunks...))
		}

		bcs := &blockBaseSeriesSet{
			p:          index.NewListPostings(tc.postings),
			index:      mi,
			tombstones: tombstones.NewMemTombstones(),
		}

		i := 0
		for bcs.Next() {
			si := populateWithDelGenericSeriesIterator{}
			si.reset(bcs.blockID, bcs.chunks, bcs.curr.chks, bcs.curr.intervals)
			idx := tc.expIdxs[i]

			require.Equal(t, tc.series[idx].lset, bcs.curr.labels)
			require.Equal(t, tc.series[idx].chunks, si.chks)

			i++
		}
		require.Equal(t, len(tc.expIdxs), i)
		require.NoError(t, bcs.Err())
	}
}

func BenchmarkHeadChunkQuerier(b *testing.B) {
	db := openTestDB(b, nil, nil)
	defer func() {
		require.NoError(b, db.Close())
	}()

	// 3h of data.
	numTimeseries := 100
	app := db.Appender(context.Background())
	for i := 0; i < 120*6; i++ {
		for j := 0; j < numTimeseries; j++ {
			lbls := labels.FromStrings("foo", fmt.Sprintf("bar%d", j))
			if i%10 == 0 {
				require.NoError(b, app.Commit())
				app = db.Appender(context.Background())
			}
			_, err := app.Append(0, lbls, int64(i*15)*time.Second.Milliseconds(), float64(i*100))
			require.NoError(b, err)
		}
	}
	require.NoError(b, app.Commit())

	querier, err := db.ChunkQuerier(context.Background(), math.MinInt64, math.MaxInt64)
	require.NoError(b, err)
	defer func(q storage.ChunkQuerier) {
		require.NoError(b, q.Close())
	}(querier)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		total := 0
		for ss.Next() {
			cs := ss.At()
			it := cs.Iterator(nil)
			for it.Next() {
				m := it.At()
				total += m.Chunk.NumSamples()
			}
		}
		_ = total
		require.NoError(b, ss.Err())
	}
}

func BenchmarkHeadQuerier(b *testing.B) {
	db := openTestDB(b, nil, nil)
	defer func() {
		require.NoError(b, db.Close())
	}()

	// 3h of data.
	numTimeseries := 100
	app := db.Appender(context.Background())
	for i := 0; i < 120*6; i++ {
		for j := 0; j < numTimeseries; j++ {
			lbls := labels.FromStrings("foo", fmt.Sprintf("bar%d", j))
			if i%10 == 0 {
				require.NoError(b, app.Commit())
				app = db.Appender(context.Background())
			}
			_, err := app.Append(0, lbls, int64(i*15)*time.Second.Milliseconds(), float64(i*100))
			require.NoError(b, err)
		}
	}
	require.NoError(b, app.Commit())

	querier, err := db.Querier(context.Background(), math.MinInt64, math.MaxInt64)
	require.NoError(b, err)
	defer func(q storage.Querier) {
		require.NoError(b, q.Close())
	}(querier)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ss := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		total := int64(0)
		for ss.Next() {
			cs := ss.At()
			it := cs.Iterator(nil)
			for it.Next() != chunkenc.ValNone {
				ts, _ := it.At()
				total += ts
			}
		}
		_ = total
		require.NoError(b, ss.Err())
	}
}
