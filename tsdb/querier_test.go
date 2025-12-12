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
	"errors"
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/testutil"
)

// TODO(bwplotka): Replace those mocks with remote.concreteSeriesSet.
type mockSeriesSet struct {
	next   func() bool
	series func() storage.Series
	ws     func() annotations.Annotations
	err    func() error
}

func (m *mockSeriesSet) Next() bool                        { return m.next() }
func (m *mockSeriesSet) At() storage.Series                { return m.series() }
func (m *mockSeriesSet) Err() error                        { return m.err() }
func (m *mockSeriesSet) Warnings() annotations.Annotations { return m.ws() }

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
		ws:  func() annotations.Annotations { return nil },
	}
}

type mockChunkSeriesSet struct {
	next   func() bool
	series func() storage.ChunkSeries
	ws     func() annotations.Annotations
	err    func() error
}

func (m *mockChunkSeriesSet) Next() bool                        { return m.next() }
func (m *mockChunkSeriesSet) At() storage.ChunkSeries           { return m.series() }
func (m *mockChunkSeriesSet) Err() error                        { return m.err() }
func (m *mockChunkSeriesSet) Warnings() annotations.Annotations { return m.ws() }

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
		ws:  func() annotations.Annotations { return nil },
	}
}

type seriesSamples struct {
	lset   map[string]string
	chunks [][]sample
}

// Index: labels -> postings -> chunkMetas -> chunkRef.
// ChunkReader: ref -> vals.
func createIdxChkReaders(t *testing.T, tc []seriesSamples) (IndexReader, ChunkReader, int64, int64) {
	sort.Slice(tc, func(i, _ int) bool {
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
		i++ // 0 is not a valid posting.
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

			switch {
			case chk[0].fh != nil:
				chunk := chunkenc.NewFloatHistogramChunk()
				app, _ := chunk.Appender()
				for _, smpl := range chk {
					require.NotNil(t, smpl.fh, "chunk can only contain one type of sample")
					_, _, _, err := app.AppendFloatHistogram(nil, smpl.t, smpl.fh, true)
					require.NoError(t, err, "chunk should be appendable")
				}
				chkReader[chunkRef] = chunk
			case chk[0].h != nil:
				chunk := chunkenc.NewHistogramChunk()
				app, _ := chunk.Appender()
				for _, smpl := range chk {
					require.NotNil(t, smpl.h, "chunk can only contain one type of sample")
					_, _, _, err := app.AppendHistogram(nil, smpl.t, smpl.h, true)
					require.NoError(t, err, "chunk should be appendable")
				}
				chkReader[chunkRef] = chunk
			default:
				chunk := chunkenc.NewXORChunk()
				app, _ := chunk.Appender()
				for _, smpl := range chk {
					require.Nil(t, smpl.h, "chunk can only contain one type of sample")
					require.Nil(t, smpl.fh, "chunk can only contain one type of sample")
					app.Append(smpl.t, smpl.f)
				}
				chkReader[chunkRef] = chunk
			}
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

		res := q.Select(context.Background(), false, c.hints, c.ms...)
		defer func() { require.NoError(t, q.Close()) }()

		for {
			eok, rok := c.exp.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				require.Empty(t, res.Warnings())
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
		res := q.Select(context.Background(), false, c.hints, c.ms...)
		defer func() { require.NoError(t, q.Close()) }()

		for {
			eok, rok := c.expChks.Next(), res.Next()
			require.Equal(t, eok, rok)

			if !eok {
				require.Empty(t, res.Warnings())
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

			require.Len(t, chksRes, len(chksExp))
			var exp, act [][]chunks.Sample
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
					[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("b", "b"),
					[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}}, []chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}}, []chunks.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("b", "b"),
					[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}}, []chunks.Sample{sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}}, []chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}}, []chunks.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
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
					[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}},
					[]chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}},
					[]chunks.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
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
					[]chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
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
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("b", "b"),
					[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("b", "b"),
					[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
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
						_, err = app.Append(0, labels.FromMap(s.lset), sample.t, sample.f)
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

func TestBlockQuerier_TrimmingDoesNotModifyOriginalTombstoneIntervals(t *testing.T) {
	ctx := context.Background()
	c := blockQuerierTestCase{
		mint: 2,
		maxt: 6,
		ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", "a")},
		exp: newMockSeriesSet([]storage.Series{
			storage.NewListSeries(labels.FromStrings("a", "a"),
				[]chunks.Sample{sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
			),
			storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
				[]chunks.Sample{sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
			),
		}),
		expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
			storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
				[]chunks.Sample{sample{3, 4, nil, nil}}, []chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
			),
			storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
				[]chunks.Sample{sample{3, 3, nil, nil}}, []chunks.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
			),
		}),
	}
	ir, cr, _, _ := createIdxChkReaders(t, testData)
	stones := tombstones.NewMemTombstones()
	p, err := ir.Postings(ctx, "a", "a")
	require.NoError(t, err)
	refs, err := index.ExpandPostings(p)
	require.NoError(t, err)
	for _, ref := range refs {
		stones.AddInterval(ref, tombstones.Interval{Mint: 1, Maxt: 2})
	}
	testBlockQuerier(t, c, ir, cr, stones)
	for _, ref := range refs {
		intervals, err := stones.Get(ref)
		require.NoError(t, err)
		// Without copy, the intervals could be [math.MinInt64, 2].
		require.Equal(t, tombstones.Intervals{{Mint: 1, Maxt: 2}}, intervals)
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
					[]chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{5, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("b", "b"),
					[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{5, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("b", "b"),
					[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}}, []chunks.Sample{sample{5, 1, nil, nil}},
				),
			}),
		},
		{
			mint: 2,
			maxt: 6,
			ms:   []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "a")},
			exp: newMockSeriesSet([]storage.Series{
				storage.NewListSeries(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListSeries(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{5, 3, nil, nil}},
				),
			}),
			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a"),
					[]chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}},
				),
				storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "b", "b"),
					[]chunks.Sample{sample{5, 3, nil, nil}},
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
	chks      map[chunks.ChunkRef]chunkenc.Chunk
	iterables map[chunks.ChunkRef]chunkenc.Iterable
}

func createFakeReaderAndNotPopulatedChunks(s ...[]chunks.Sample) (*fakeChunksReader, []chunks.Meta) {
	f := &fakeChunksReader{
		chks:      map[chunks.ChunkRef]chunkenc.Chunk{},
		iterables: map[chunks.ChunkRef]chunkenc.Iterable{},
	}
	chks := make([]chunks.Meta, 0, len(s))

	for ref, samples := range s {
		chk, _ := chunks.ChunkFromSamples(samples)
		f.chks[chunks.ChunkRef(ref)] = chk.Chunk

		chks = append(chks, chunks.Meta{
			Ref:     chunks.ChunkRef(ref),
			MinTime: chk.MinTime,
			MaxTime: chk.MaxTime,
		})
	}
	return f, chks
}

// Samples in each slice are assumed to be sorted.
func createFakeReaderAndIterables(s ...[]chunks.Sample) (*fakeChunksReader, []chunks.Meta) {
	f := &fakeChunksReader{
		chks:      map[chunks.ChunkRef]chunkenc.Chunk{},
		iterables: map[chunks.ChunkRef]chunkenc.Iterable{},
	}
	chks := make([]chunks.Meta, 0, len(s))

	for ref, samples := range s {
		f.iterables[chunks.ChunkRef(ref)] = &mockIterable{s: samples}

		var minTime, maxTime int64
		if len(samples) > 0 {
			minTime = samples[0].T()
			maxTime = samples[len(samples)-1].T()
		}
		chks = append(chks, chunks.Meta{
			Ref:     chunks.ChunkRef(ref),
			MinTime: minTime,
			MaxTime: maxTime,
		})
	}
	return f, chks
}

func (r *fakeChunksReader) ChunkOrIterable(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	if chk, ok := r.chks[meta.Ref]; ok {
		return chk, nil, nil
	}

	if it, ok := r.iterables[meta.Ref]; ok {
		return nil, it, nil
	}
	return nil, nil, fmt.Errorf("chunk or iterable not found at ref %v", meta.Ref)
}

type mockIterable struct {
	s []chunks.Sample
}

func (it *mockIterable) Iterator(chunkenc.Iterator) chunkenc.Iterator {
	return &mockSampleIterator{
		s:   it.s,
		idx: -1,
	}
}

type mockSampleIterator struct {
	s   []chunks.Sample
	idx int
}

func (it *mockSampleIterator) Seek(t int64) chunkenc.ValueType {
	for ; it.idx < len(it.s); it.idx++ {
		if it.idx != -1 && it.s[it.idx].T() >= t {
			return it.s[it.idx].Type()
		}
	}

	return chunkenc.ValNone
}

func (it *mockSampleIterator) At() (int64, float64) {
	return it.s[it.idx].T(), it.s[it.idx].F()
}

func (it *mockSampleIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return it.s[it.idx].T(), it.s[it.idx].H()
}

func (it *mockSampleIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return it.s[it.idx].T(), it.s[it.idx].FH()
}

func (it *mockSampleIterator) AtT() int64 {
	return it.s[it.idx].T()
}

func (it *mockSampleIterator) Next() chunkenc.ValueType {
	if it.idx < len(it.s)-1 {
		it.idx++
		return it.s[it.idx].Type()
	}

	return chunkenc.ValNone
}

func (*mockSampleIterator) Err() error { return nil }

func TestPopulateWithTombSeriesIterators(t *testing.T) {
	type minMaxTimes struct {
		minTime, maxTime int64
	}
	cases := []struct {
		name    string
		samples [][]chunks.Sample

		expected            []chunks.Sample
		expectedChks        []chunks.Meta
		expectedMinMaxTimes []minMaxTimes

		intervals tombstones.Intervals

		// Seek being zero means do not test seek.
		seek        int64
		seekSuccess bool

		// Set this to true if a sample slice will form multiple chunks.
		skipChunkTest bool

		skipIterableTest bool
	}{
		{
			name:    "no chunk",
			samples: [][]chunks.Sample{},
		},
		{
			name:    "one empty chunk", // This should never happen.
			samples: [][]chunks.Sample{{}},

			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{}),
			},
			expectedMinMaxTimes: []minMaxTimes{{0, 0}},
			// iterables with no samples will return no chunks instead of empty chunks
			skipIterableTest: true,
		},
		{
			name:    "one empty iterable",
			samples: [][]chunks.Sample{{}},

			// iterables with no samples will return no chunks
			expectedChks:  nil,
			skipChunkTest: true,
		},
		{
			name:    "three empty chunks", // This should never happen.
			samples: [][]chunks.Sample{{}, {}, {}},

			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{}),
				assureChunkFromSamples(t, []chunks.Sample{}),
				assureChunkFromSamples(t, []chunks.Sample{}),
			},
			expectedMinMaxTimes: []minMaxTimes{{0, 0}, {0, 0}, {0, 0}},
			// iterables with no samples will return no chunks instead of empty chunks
			skipIterableTest: true,
		},
		{
			name:    "three empty iterables",
			samples: [][]chunks.Sample{{}, {}, {}},

			// iterables with no samples will return no chunks
			expectedChks:  nil,
			skipChunkTest: true,
		},
		{
			name: "one chunk",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
			},

			expected: []chunks.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}},
		},
		{
			name: "two full chunks",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},

			expected: []chunks.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}, {7, 9}},
		},
		{
			name: "three full chunks",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
				{sample{10, 22, nil, nil}, sample{203, 3493, nil, nil}},
			},

			expected: []chunks.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil}, sample{10, 22, nil, nil}, sample{203, 3493, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{10, 22, nil, nil}, sample{203, 3493, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}, {7, 9}, {10, 203}},
		},
		// Seek cases.
		{
			name:    "three empty chunks and seek", // This should never happen.
			samples: [][]chunks.Sample{{}, {}, {}},
			seek:    1,

			seekSuccess: false,
		},
		{
			name: "two chunks and seek beyond chunks",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			seek: 10,

			seekSuccess: false,
		},
		{
			name: "two chunks and seek on middle of first chunk",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			seek: 2,

			seekSuccess: true,
			expected: []chunks.Sample{
				sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
		},
		{
			name: "two chunks and seek before first chunk",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			seek: -32,

			seekSuccess: true,
			expected: []chunks.Sample{
				sample{1, 2, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
		},
		// Deletion / Trim cases.
		{
			name:      "no chunk with deletion interval",
			samples:   [][]chunks.Sample{},
			intervals: tombstones.Intervals{{Mint: 20, Maxt: 21}},
		},
		{
			name: "two chunks with trimmed first and last samples from edge chunks",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 9, Maxt: math.MaxInt64}),

			expected: []chunks.Sample{
				sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 89, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{3, 6}, {7, 7}},
		},
		{
			name: "two chunks with trimmed middle sample of first chunk",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: 2, Maxt: 3}},

			expected: []chunks.Sample{
				sample{1, 2, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 2, nil, nil}, sample{6, 1, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}, {7, 9}},
		},
		{
			name: "two chunks with deletion across two chunks",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: 6, Maxt: 7}},

			expected: []chunks.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{9, 8, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{9, 8, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 3}, {9, 9}},
		},
		{
			name: "two chunks with first chunk deleted",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 6}},

			expected: []chunks.Sample{
				sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 89, nil, nil}, sample{9, 8, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{7, 9}},
		},
		// Deletion with seek.
		{
			name: "two chunks with trimmed first and last samples from edge chunks, seek from middle of first chunk",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 9, Maxt: math.MaxInt64}),

			seek:        3,
			seekSuccess: true,
			expected: []chunks.Sample{
				sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 89, nil, nil},
			},
		},
		{
			name: "one chunk where all samples are trimmed",
			samples: [][]chunks.Sample{
				{sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{sample{7, 89, nil, nil}, sample{9, 8, nil, nil}},
			},
			intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: 3}}.Add(tombstones.Interval{Mint: 4, Maxt: math.MaxInt64}),

			expected:     nil,
			expectedChks: nil,
		},
		{
			name: "one histogram chunk",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestHistogram(6), nil},
				},
			},
			expected: []chunks.Sample{
				sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil},
				sample{2, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(2)), nil},
				sample{3, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(3)), nil},
				sample{6, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(6)), nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil},
					sample{2, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(2)), nil},
					sample{3, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(3)), nil},
					sample{6, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(6)), nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}},
		},
		{
			name: "one histogram chunk intersect with earlier deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestHistogram(6), nil},
				},
			},
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 2}},
			expected: []chunks.Sample{
				sample{3, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(3)), nil},
				sample{6, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(6)), nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{3, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(3)), nil},
					sample{6, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(6)), nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{3, 6}},
		},
		{
			name: "one histogram chunk intersect with later deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestHistogram(6), nil},
				},
			},
			intervals: tombstones.Intervals{{Mint: 5, Maxt: 20}},
			expected: []chunks.Sample{
				sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil},
				sample{2, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(2)), nil},
				sample{3, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(3)), nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil},
					sample{2, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(2)), nil},
					sample{3, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(3)), nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 3}},
		},
		{
			name: "one float histogram chunk",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
				},
			},
			expected: []chunks.Sample{
				sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)},
				sample{2, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(2))},
				sample{3, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(3))},
				sample{6, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(6))},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(2))},
					sample{3, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(3))},
					sample{6, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(6))},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}},
		},
		{
			name: "one float histogram chunk intersect with earlier deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
				},
			},
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 2}},
			expected: []chunks.Sample{
				sample{3, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(3))},
				sample{6, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(6))},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{3, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(3))},
					sample{6, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(6))},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{3, 6}},
		},
		{
			name: "one float histogram chunk intersect with later deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
				},
			},
			intervals: tombstones.Intervals{{Mint: 5, Maxt: 20}},
			expected: []chunks.Sample{
				sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)},
				sample{2, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(2))},
				sample{3, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(3))},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(2))},
					sample{3, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(3))},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 3}},
		},
		{
			name: "one gauge histogram chunk",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, tsdbutil.GenerateTestGaugeHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestGaugeHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestGaugeHistogram(6), nil},
				},
			},
			expected: []chunks.Sample{
				sample{1, 0, tsdbutil.GenerateTestGaugeHistogram(1), nil},
				sample{2, 0, tsdbutil.GenerateTestGaugeHistogram(2), nil},
				sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
				sample{6, 0, tsdbutil.GenerateTestGaugeHistogram(6), nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, tsdbutil.GenerateTestGaugeHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestGaugeHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestGaugeHistogram(6), nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}},
		},
		{
			name: "one gauge histogram chunk intersect with earlier deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, tsdbutil.GenerateTestGaugeHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestGaugeHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestGaugeHistogram(6), nil},
				},
			},
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 2}},
			expected: []chunks.Sample{
				sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
				sample{6, 0, tsdbutil.GenerateTestGaugeHistogram(6), nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestGaugeHistogram(6), nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{3, 6}},
		},
		{
			name: "one gauge histogram chunk intersect with later deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, tsdbutil.GenerateTestGaugeHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestGaugeHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
					sample{6, 0, tsdbutil.GenerateTestGaugeHistogram(6), nil},
				},
			},
			intervals: tombstones.Intervals{{Mint: 5, Maxt: 20}},
			expected: []chunks.Sample{
				sample{1, 0, tsdbutil.GenerateTestGaugeHistogram(1), nil},
				sample{2, 0, tsdbutil.GenerateTestGaugeHistogram(2), nil},
				sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, tsdbutil.GenerateTestGaugeHistogram(1), nil},
					sample{2, 0, tsdbutil.GenerateTestGaugeHistogram(2), nil},
					sample{3, 0, tsdbutil.GenerateTestGaugeHistogram(3), nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 3}},
		},
		{
			name: "one gauge float histogram",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(6)},
				},
			},
			expected: []chunks.Sample{
				sample{1, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(1)},
				sample{2, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(2)},
				sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
				sample{6, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(6)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(6)},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}},
		},
		{
			name: "one gauge float histogram chunk intersect with earlier deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(6)},
				},
			},
			intervals: tombstones.Intervals{{Mint: 1, Maxt: 2}},
			expected: []chunks.Sample{
				sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
				sample{6, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(6)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(6)},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{3, 6}},
		},
		{
			name: "one gauge float histogram chunk intersect with later deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{1, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
					sample{6, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(6)},
				},
			},
			intervals: tombstones.Intervals{{Mint: 5, Maxt: 20}},
			expected: []chunks.Sample{
				sample{1, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(1)},
				sample{2, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(2)},
				sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(1)},
					sample{2, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(2)},
					sample{3, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3)},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 3}},
		},
		{
			name: "three full mixed chunks",
			samples: [][]chunks.Sample{
				{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}},
				{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
					sample{9, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil},
				},
				{
					sample{10, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)},
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				},
			},

			expected: []chunks.Sample{
				sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil}, sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil}, sample{9, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil}, sample{10, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)}, sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 5, nil, nil}, sample{6, 1, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
					sample{9, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{10, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)},
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{1, 6}, {7, 9}, {10, 203}},
		},
		{
			name: "three full mixed chunks in different order",
			samples: [][]chunks.Sample{
				{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
					sample{9, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil},
				},
				{sample{11, 2, nil, nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{16, 1, nil, nil}},
				{
					sample{100, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)},
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				},
			},

			expected: []chunks.Sample{
				sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil}, sample{9, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil}, sample{11, 2, nil, nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{16, 1, nil, nil}, sample{100, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)}, sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
					sample{9, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{11, 2, nil, nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{16, 1, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{100, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)},
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{7, 9}, {11, 16}, {100, 203}},
		},
		{
			name: "three full mixed chunks in different order intersect with deletion interval",
			samples: [][]chunks.Sample{
				{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
					sample{9, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil},
				},
				{sample{11, 2, nil, nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{16, 1, nil, nil}},
				{
					sample{100, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)},
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				},
			},
			intervals: tombstones.Intervals{{Mint: 8, Maxt: 11}, {Mint: 15, Maxt: 150}},

			expected: []chunks.Sample{
				sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{12, 3, nil, nil}, sample{13, 5, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{7, 7}, {12, 13}, {203, 203}},
		},
		{
			name: "three full mixed chunks overlapping",
			samples: [][]chunks.Sample{
				{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
					sample{12, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil},
				},
				{sample{11, 2, nil, nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{16, 1, nil, nil}},
				{
					sample{10, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)},
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				},
			},

			expected: []chunks.Sample{
				sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil}, sample{12, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil}, sample{11, 2, nil, nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{16, 1, nil, nil}, sample{10, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)}, sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 0, tsdbutil.GenerateTestGaugeHistogram(89), nil},
					sample{12, 0, tsdbutil.GenerateTestGaugeHistogram(8), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{11, 2, nil, nil}, sample{12, 3, nil, nil}, sample{13, 5, nil, nil}, sample{16, 1, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{10, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(22)},
					sample{203, 0, nil, tsdbutil.GenerateTestGaugeFloatHistogram(3493)},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{{7, 12}, {11, 16}, {10, 203}},
		},
		{
			name: "int histogram iterables with counter resets",
			samples: [][]chunks.Sample{
				{
					sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil},
					sample{8, 0, tsdbutil.GenerateTestHistogram(9), nil},
					// Counter reset should be detected when chunks are created from the iterable.
					sample{12, 0, tsdbutil.GenerateTestHistogram(5), nil},
					sample{15, 0, tsdbutil.GenerateTestHistogram(6), nil},
					sample{16, 0, tsdbutil.GenerateTestHistogram(7), nil},
					// Counter reset should be detected when chunks are created from the iterable.
					sample{17, 0, tsdbutil.GenerateTestHistogram(5), nil},
				},
				{
					sample{18, 0, tsdbutil.GenerateTestHistogram(6), nil},
					sample{19, 0, tsdbutil.GenerateTestHistogram(7), nil},
					// Counter reset should be detected when chunks are created from the iterable.
					sample{20, 0, tsdbutil.GenerateTestHistogram(5), nil},
					sample{21, 0, tsdbutil.GenerateTestHistogram(6), nil},
				},
			},

			expected: []chunks.Sample{
				sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil},
				sample{8, 0, tsdbutil.GenerateTestHistogram(9), nil},
				sample{12, 0, tsdbutil.GenerateTestHistogram(5), nil},
				sample{15, 0, tsdbutil.GenerateTestHistogram(6), nil},
				sample{16, 0, tsdbutil.GenerateTestHistogram(7), nil},
				sample{17, 0, tsdbutil.GenerateTestHistogram(5), nil},
				sample{18, 0, tsdbutil.GenerateTestHistogram(6), nil},
				sample{19, 0, tsdbutil.GenerateTestHistogram(7), nil},
				sample{20, 0, tsdbutil.GenerateTestHistogram(5), nil},
				sample{21, 0, tsdbutil.GenerateTestHistogram(6), nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil},
					sample{8, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(9)), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{12, 0, tsdbutil.SetHistogramCounterReset(tsdbutil.GenerateTestHistogram(5)), nil},
					sample{15, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(6)), nil},
					sample{16, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(7)), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{17, 0, tsdbutil.SetHistogramCounterReset(tsdbutil.GenerateTestHistogram(5)), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{18, 0, tsdbutil.GenerateTestHistogram(6), nil},
					sample{19, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(7)), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{20, 0, tsdbutil.SetHistogramCounterReset(tsdbutil.GenerateTestHistogram(5)), nil},
					sample{21, 0, tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(6)), nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{
				{7, 8},
				{12, 16},
				{17, 17},
				{18, 19},
				{20, 21},
			},

			// Skipping chunk test - can't create a single chunk for each
			// sample slice since there are counter resets in the middle of
			// the slices.
			skipChunkTest: true,
		},
		{
			name: "float histogram iterables with counter resets",
			samples: [][]chunks.Sample{
				{
					sample{7, 0, nil, tsdbutil.GenerateTestFloatHistogram(8)},
					sample{8, 0, nil, tsdbutil.GenerateTestFloatHistogram(9)},
					// Counter reset should be detected when chunks are created from the iterable.
					sample{12, 0, nil, tsdbutil.GenerateTestFloatHistogram(5)},
					sample{15, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
					sample{16, 0, nil, tsdbutil.GenerateTestFloatHistogram(7)},
					// Counter reset should be detected when chunks are created from the iterable.
					sample{17, 0, nil, tsdbutil.GenerateTestFloatHistogram(5)},
				},
				{
					sample{18, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
					sample{19, 0, nil, tsdbutil.GenerateTestFloatHistogram(7)},
					// Counter reset should be detected when chunks are created from the iterable.
					sample{20, 0, nil, tsdbutil.GenerateTestFloatHistogram(5)},
					sample{21, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
				},
			},

			expected: []chunks.Sample{
				sample{7, 0, nil, tsdbutil.GenerateTestFloatHistogram(8)},
				sample{8, 0, nil, tsdbutil.GenerateTestFloatHistogram(9)},
				sample{12, 0, nil, tsdbutil.GenerateTestFloatHistogram(5)},
				sample{15, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
				sample{16, 0, nil, tsdbutil.GenerateTestFloatHistogram(7)},
				sample{17, 0, nil, tsdbutil.GenerateTestFloatHistogram(5)},
				sample{18, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
				sample{19, 0, nil, tsdbutil.GenerateTestFloatHistogram(7)},
				sample{20, 0, nil, tsdbutil.GenerateTestFloatHistogram(5)},
				sample{21, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 0, nil, tsdbutil.GenerateTestFloatHistogram(8)},
					sample{8, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(9))},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{12, 0, nil, tsdbutil.SetFloatHistogramCounterReset(tsdbutil.GenerateTestFloatHistogram(5))},
					sample{15, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(6))},
					sample{16, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(7))},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{17, 0, nil, tsdbutil.SetFloatHistogramCounterReset(tsdbutil.GenerateTestFloatHistogram(5))},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{18, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)},
					sample{19, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(7))},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{20, 0, nil, tsdbutil.SetFloatHistogramCounterReset(tsdbutil.GenerateTestFloatHistogram(5))},
					sample{21, 0, nil, tsdbutil.SetFloatHistogramNotCounterReset(tsdbutil.GenerateTestFloatHistogram(6))},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{
				{7, 8},
				{12, 16},
				{17, 17},
				{18, 19},
				{20, 21},
			},

			// Skipping chunk test - can't create a single chunk for each
			// sample slice since there are counter resets in the middle of
			// the slices.
			skipChunkTest: true,
		},
		{
			name: "iterables with mixed encodings and counter resets",
			samples: [][]chunks.Sample{
				{
					sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil},
					sample{8, 0, tsdbutil.GenerateTestHistogram(9), nil},
					sample{9, 0, nil, tsdbutil.GenerateTestFloatHistogram(10)},
					sample{10, 0, nil, tsdbutil.GenerateTestFloatHistogram(11)},
					sample{11, 0, nil, tsdbutil.GenerateTestFloatHistogram(12)},
					sample{12, 13, nil, nil},
					sample{13, 14, nil, nil},
					sample{14, 0, tsdbutil.GenerateTestHistogram(8), nil},
					// Counter reset should be detected when chunks are created from the iterable.
					sample{15, 0, tsdbutil.GenerateTestHistogram(7), nil},
				},
				{
					sample{18, 0, tsdbutil.GenerateTestHistogram(6), nil},
					sample{19, 45, nil, nil},
				},
			},

			expected: []chunks.Sample{
				sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil},
				sample{8, 0, tsdbutil.GenerateTestHistogram(9), nil},
				sample{9, 0, nil, tsdbutil.GenerateTestFloatHistogram(10)},
				sample{10, 0, nil, tsdbutil.GenerateTestFloatHistogram(11)},
				sample{11, 0, nil, tsdbutil.GenerateTestFloatHistogram(12)},
				sample{12, 13, nil, nil},
				sample{13, 14, nil, nil},
				sample{14, 0, tsdbutil.GenerateTestHistogram(8), nil},
				sample{15, 0, tsdbutil.GenerateTestHistogram(7), nil},
				sample{18, 0, tsdbutil.GenerateTestHistogram(6), nil},
				sample{19, 45, nil, nil},
			},
			expectedChks: []chunks.Meta{
				assureChunkFromSamples(t, []chunks.Sample{
					sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil},
					sample{8, 0, tsdbutil.GenerateTestHistogram(9), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{9, 0, nil, tsdbutil.GenerateTestFloatHistogram(10)},
					sample{10, 0, nil, tsdbutil.GenerateTestFloatHistogram(11)},
					sample{11, 0, nil, tsdbutil.GenerateTestFloatHistogram(12)},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{12, 13, nil, nil},
					sample{13, 14, nil, nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{14, 0, tsdbutil.GenerateTestHistogram(8), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{15, 0, tsdbutil.SetHistogramCounterReset(tsdbutil.GenerateTestHistogram(7)), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{18, 0, tsdbutil.GenerateTestHistogram(6), nil},
				}),
				assureChunkFromSamples(t, []chunks.Sample{
					sample{19, 45, nil, nil},
				}),
			},
			expectedMinMaxTimes: []minMaxTimes{
				{7, 8},
				{9, 11},
				{12, 13},
				{14, 14},
				{15, 15},
				{18, 18},
				{19, 19},
			},

			skipChunkTest: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("sample", func(t *testing.T) {
				var f *fakeChunksReader
				var chkMetas []chunks.Meta
				// If the test case wants to skip the chunks test, it probably
				// means you can't create valid chunks from sample slices,
				// therefore create iterables over the samples instead.
				if tc.skipChunkTest {
					f, chkMetas = createFakeReaderAndIterables(tc.samples...)
				} else {
					f, chkMetas = createFakeReaderAndNotPopulatedChunks(tc.samples...)
				}
				it := &populateWithDelSeriesIterator{}
				it.reset(ulid.ULID{}, f, chkMetas, tc.intervals)

				var r []chunks.Sample
				if tc.seek != 0 {
					require.Equal(t, tc.seekSuccess, it.Seek(tc.seek) == chunkenc.ValFloat)
					require.Equal(t, tc.seekSuccess, it.Seek(tc.seek) == chunkenc.ValFloat) // Next one should be noop.

					if tc.seekSuccess {
						// After successful seek iterator is ready. Grab the value.
						t, v := it.At()
						r = append(r, sample{t: t, f: v})
					}
				}
				expandedResult, err := storage.ExpandSamples(it, newSample)
				require.NoError(t, err)
				r = append(r, expandedResult...)
				require.Equal(t, tc.expected, r)
			})
			t.Run("chunk", func(t *testing.T) {
				if tc.skipChunkTest {
					t.Skip()
				}
				f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.samples...)
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

				for i, meta := range expandedResult {
					require.Equal(t, tc.expectedMinMaxTimes[i].minTime, meta.MinTime)
					require.Equal(t, tc.expectedMinMaxTimes[i].maxTime, meta.MaxTime)
				}
			})
			t.Run("iterables", func(t *testing.T) {
				if tc.skipIterableTest {
					t.Skip()
				}
				f, chkMetas := createFakeReaderAndIterables(tc.samples...)
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

				for i, meta := range expandedResult {
					require.Equal(t, tc.expectedMinMaxTimes[i].minTime, meta.MinTime)
					require.Equal(t, tc.expectedMinMaxTimes[i].maxTime, meta.MaxTime)
				}
			})
		})
	}
}

func rmChunkRefs(chks []chunks.Meta) {
	for i := range chks {
		chks[i].Ref = 0
	}
}

func checkCurrVal(t *testing.T, valType chunkenc.ValueType, it *populateWithDelSeriesIterator, expectedTs, expectedValue int) {
	switch valType {
	case chunkenc.ValFloat:
		ts, v := it.At()
		require.Equal(t, int64(expectedTs), ts)
		require.Equal(t, float64(expectedValue), v)
	case chunkenc.ValHistogram:
		ts, h := it.AtHistogram(nil)
		require.Equal(t, int64(expectedTs), ts)
		h.CounterResetHint = histogram.UnknownCounterReset
		require.Equal(t, tsdbutil.GenerateTestHistogram(int64(expectedValue)), h)
	case chunkenc.ValFloatHistogram:
		ts, h := it.AtFloatHistogram(nil)
		require.Equal(t, int64(expectedTs), ts)
		h.CounterResetHint = histogram.UnknownCounterReset
		require.Equal(t, tsdbutil.GenerateTestFloatHistogram(int64(expectedValue)), h)
	default:
		panic("unexpected value type")
	}
}

// Regression for: https://github.com/prometheus/tsdb/pull/97
func TestPopulateWithDelSeriesIterator_DoubleSeek(t *testing.T) {
	cases := []struct {
		name    string
		valType chunkenc.ValueType
		chks    [][]chunks.Sample
	}{
		{
			name:    "float",
			valType: chunkenc.ValFloat,
			chks: [][]chunks.Sample{
				{},
				{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}},
				{sample{4, 4, nil, nil}, sample{5, 5, nil, nil}},
			},
		},
		{
			name:    "histogram",
			valType: chunkenc.ValHistogram,
			chks: [][]chunks.Sample{
				{},
				{sample{1, 0, tsdbutil.GenerateTestHistogram(1), nil}, sample{2, 0, tsdbutil.GenerateTestHistogram(2), nil}, sample{3, 0, tsdbutil.GenerateTestHistogram(3), nil}},
				{sample{4, 0, tsdbutil.GenerateTestHistogram(4), nil}, sample{5, 0, tsdbutil.GenerateTestHistogram(5), nil}},
			},
		},
		{
			name:    "float histogram",
			valType: chunkenc.ValFloatHistogram,
			chks: [][]chunks.Sample{
				{},
				{sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(1)}, sample{2, 0, nil, tsdbutil.GenerateTestFloatHistogram(2)}, sample{3, 0, nil, tsdbutil.GenerateTestFloatHistogram(3)}},
				{sample{4, 0, nil, tsdbutil.GenerateTestFloatHistogram(4)}, sample{5, 0, nil, tsdbutil.GenerateTestFloatHistogram(5)}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
			it := &populateWithDelSeriesIterator{}
			it.reset(ulid.ULID{}, f, chkMetas, nil)
			require.Equal(t, tc.valType, it.Seek(1))
			require.Equal(t, tc.valType, it.Seek(2))
			require.Equal(t, tc.valType, it.Seek(2))
			checkCurrVal(t, tc.valType, it, 2, 2)
			require.Equal(t, int64(0), chkMetas[0].MinTime)
			require.Equal(t, int64(1), chkMetas[1].MinTime)
			require.Equal(t, int64(4), chkMetas[2].MinTime)
		})
	}
}

// Regression when seeked chunks were still found via binary search and we always
// skipped to the end when seeking a value in the current chunk.
func TestPopulateWithDelSeriesIterator_SeekInCurrentChunk(t *testing.T) {
	cases := []struct {
		name    string
		valType chunkenc.ValueType
		chks    [][]chunks.Sample
	}{
		{
			name:    "float",
			valType: chunkenc.ValFloat,
			chks: [][]chunks.Sample{
				{},
				{sample{1, 2, nil, nil}, sample{3, 4, nil, nil}, sample{5, 6, nil, nil}, sample{7, 8, nil, nil}},
				{},
			},
		},
		{
			name:    "histogram",
			valType: chunkenc.ValHistogram,
			chks: [][]chunks.Sample{
				{},
				{sample{1, 0, tsdbutil.GenerateTestHistogram(2), nil}, sample{3, 0, tsdbutil.GenerateTestHistogram(4), nil}, sample{5, 0, tsdbutil.GenerateTestHistogram(6), nil}, sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil}},
				{},
			},
		},
		{
			name:    "float histogram",
			valType: chunkenc.ValFloatHistogram,
			chks: [][]chunks.Sample{
				{},
				{sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(2)}, sample{3, 0, nil, tsdbutil.GenerateTestFloatHistogram(4)}, sample{5, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)}, sample{7, 0, nil, tsdbutil.GenerateTestFloatHistogram(8)}},
				{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
			it := &populateWithDelSeriesIterator{}
			it.reset(ulid.ULID{}, f, chkMetas, nil)
			require.Equal(t, tc.valType, it.Next())
			checkCurrVal(t, tc.valType, it, 1, 2)
			require.Equal(t, tc.valType, it.Seek(4))
			checkCurrVal(t, tc.valType, it, 5, 6)
			require.Equal(t, int64(0), chkMetas[0].MinTime)
			require.Equal(t, int64(1), chkMetas[1].MinTime)
			require.Equal(t, int64(0), chkMetas[2].MinTime)
		})
	}
}

func TestPopulateWithDelSeriesIterator_SeekWithMinTime(t *testing.T) {
	cases := []struct {
		name    string
		valType chunkenc.ValueType
		chks    [][]chunks.Sample
	}{
		{
			name:    "float",
			valType: chunkenc.ValFloat,
			chks: [][]chunks.Sample{
				{sample{1, 6, nil, nil}, sample{5, 6, nil, nil}, sample{6, 8, nil, nil}},
			},
		},
		{
			name:    "histogram",
			valType: chunkenc.ValHistogram,
			chks: [][]chunks.Sample{
				{sample{1, 0, tsdbutil.GenerateTestHistogram(6), nil}, sample{5, 0, tsdbutil.GenerateTestHistogram(6), nil}, sample{6, 0, tsdbutil.GenerateTestHistogram(8), nil}},
			},
		},
		{
			name:    "float histogram",
			valType: chunkenc.ValFloatHistogram,
			chks: [][]chunks.Sample{
				{sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)}, sample{5, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)}, sample{6, 0, nil, tsdbutil.GenerateTestFloatHistogram(8)}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
			it := &populateWithDelSeriesIterator{}
			it.reset(ulid.ULID{}, f, chkMetas, nil)
			require.Equal(t, chunkenc.ValNone, it.Seek(7))
			require.Equal(t, tc.valType, it.Seek(3))
			require.Equal(t, int64(1), chkMetas[0].MinTime)
		})
	}
}

// Regression when calling Next() with a time bounded to fit within two samples.
// Seek gets called and advances beyond the max time, which was just accepted as a valid sample.
func TestPopulateWithDelSeriesIterator_NextWithMinTime(t *testing.T) {
	cases := []struct {
		name    string
		valType chunkenc.ValueType
		chks    [][]chunks.Sample
	}{
		{
			name:    "float",
			valType: chunkenc.ValFloat,
			chks: [][]chunks.Sample{
				{sample{1, 6, nil, nil}, sample{5, 6, nil, nil}, sample{7, 8, nil, nil}},
			},
		},
		{
			name:    "histogram",
			valType: chunkenc.ValHistogram,
			chks: [][]chunks.Sample{
				{sample{1, 0, tsdbutil.GenerateTestHistogram(6), nil}, sample{5, 0, tsdbutil.GenerateTestHistogram(6), nil}, sample{7, 0, tsdbutil.GenerateTestHistogram(8), nil}},
			},
		},
		{
			name:    "float histogram",
			valType: chunkenc.ValFloatHistogram,
			chks: [][]chunks.Sample{
				{sample{1, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)}, sample{5, 0, nil, tsdbutil.GenerateTestFloatHistogram(6)}, sample{7, 0, nil, tsdbutil.GenerateTestFloatHistogram(8)}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, chkMetas := createFakeReaderAndNotPopulatedChunks(tc.chks...)
			it := &populateWithDelSeriesIterator{}
			it.reset(ulid.ULID{}, f, chkMetas, tombstones.Intervals{{Mint: math.MinInt64, Maxt: 2}}.Add(tombstones.Interval{Mint: 4, Maxt: math.MaxInt64}))
			require.Equal(t, chunkenc.ValNone, it.Next())
			require.Equal(t, int64(1), chkMetas[0].MinTime)
		})
	}
}

// Test the cost of merging series sets for different number of merged sets and their size.
// The subset are all equivalent so this does not capture merging of partial or non-overlapping sets well.
// TODO(bwplotka): Merge with storage merged series set benchmark.
func BenchmarkMergedSeriesSet(b *testing.B) {
	sel := func(sets []storage.SeriesSet) storage.SeriesSet {
		return storage.NewMergeSeriesSet(sets, 0, storage.ChainedSeriesMerge)
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

				for b.Loop() {
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
					require.Len(b, lbls, i)
				}
			})
		}
	}
}

type mockChunkReader map[chunks.ChunkRef]chunkenc.Chunk

func (cr mockChunkReader) ChunkOrIterable(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	chk, ok := cr[meta.Ref]
	if ok {
		return chk, nil, nil
	}

	return nil, nil, errors.New("Chunk with ref not found")
}

func (mockChunkReader) Close() error {
	return nil
}

func TestDeletedIterator(t *testing.T) {
	chk := chunkenc.NewXORChunk()
	app, err := chk.Appender()
	require.NoError(t, err)
	// Insert random stuff from (0, 1000).
	act := make([]sample, 1000)
	for i := range 1000 {
		act[i].t = int64(i)
		act[i].f = rand.Float64()
		app.Append(act[i].t, act[i].f)
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
			require.Equal(t, act[i].f, v)
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
	for i := range 1000 {
		act[i].t = int64(i)
		act[i].f = float64(i)
		app.Append(act[i].t, act[i].f)
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
		return fmt.Errorf("series with reference %d already added", ref)
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
		return fmt.Errorf("postings for %s already added", l)
	}
	ep, err := index.ExpandPostings(it)
	if err != nil {
		return err
	}
	m.postings[l] = ep
	return nil
}

func (mockIndex) Close() error {
	return nil
}

func (m mockIndex) SortedLabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error) {
	values, _ := m.LabelValues(ctx, name, hints, matchers...)
	sort.Strings(values)
	return values, nil
}

func (m mockIndex) LabelValues(_ context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, error) {
	var values []string

	if len(matchers) == 0 {
		for l := range m.postings {
			if l.Name == name {
				values = append(values, l.Value)
				if hints != nil && hints.Limit > 0 && len(values) >= hints.Limit {
					break
				}
			}
		}
		return values, nil
	}

	for _, series := range m.series {
		matches := true
		for _, matcher := range matchers {
			matches = matches && matcher.Matches(series.l.Get(matcher.Name))
			if !matches {
				break
			}
		}
		if matches && !slices.Contains(values, series.l.Get(name)) {
			values = append(values, series.l.Get(name))
		}
		if hints != nil && hints.Limit > 0 && len(values) >= hints.Limit {
			break
		}
	}

	return values, nil
}

func (m mockIndex) LabelNamesFor(_ context.Context, postings index.Postings) ([]string, error) {
	namesMap := make(map[string]bool)
	for postings.Next() {
		m.series[postings.At()].l.Range(func(lbl labels.Label) {
			namesMap[lbl.Name] = true
		})
	}
	if err := postings.Err(); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(namesMap))
	for name := range namesMap {
		names = append(names, name)
	}
	return names, nil
}

func (m mockIndex) Postings(ctx context.Context, name string, values ...string) (index.Postings, error) {
	res := make([]index.Postings, 0, len(values))
	for _, value := range values {
		l := labels.Label{Name: name, Value: value}
		res = append(res, index.NewListPostings(m.postings[l]))
	}
	return index.Merge(ctx, res...), nil
}

func (m mockIndex) SortedPostings(p index.Postings) index.Postings {
	ep, err := index.ExpandPostings(p)
	if err != nil {
		return index.ErrPostings(fmt.Errorf("expand postings: %w", err))
	}

	sort.Slice(ep, func(i, j int) bool {
		return labels.Compare(m.series[ep[i]].l, m.series[ep[j]].l) < 0
	})
	return index.NewListPostings(ep)
}

func (m mockIndex) PostingsForLabelMatching(ctx context.Context, name string, match func(string) bool) index.Postings {
	var res []index.Postings
	for l, srs := range m.postings {
		if l.Name == name && match(l.Value) {
			res = append(res, index.NewListPostings(srs))
		}
	}
	return index.Merge(ctx, res...)
}

func (m mockIndex) PostingsForAllLabelValues(ctx context.Context, name string) index.Postings {
	var res []index.Postings
	for l, srs := range m.postings {
		if l.Name == name {
			res = append(res, index.NewListPostings(srs))
		}
	}
	return index.Merge(ctx, res...)
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

func (m mockIndex) LabelNames(_ context.Context, matchers ...*labels.Matcher) ([]string, error) {
	names := map[string]struct{}{}
	if len(matchers) == 0 {
		for l := range m.postings {
			names[l.Name] = struct{}{}
		}
	} else {
		for _, series := range m.series {
			matches := true
			for _, matcher := range matchers {
				matches = matches && matcher.Matches(series.l.Get(matcher.Name))
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
					block, err := OpenBlock(nil, createBlock(b, dir, generatedSeries), nil, nil)
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
					block, err := OpenBlock(nil, createBlock(b, dir, generatedSeries), nil, nil)
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
				ss := sq.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
				for ss.Next() {
					it = ss.At().Iterator(it)
					for t := mint; t <= maxt; t++ {
						it.Seek(t)
					}
					require.NoError(b, it.Err())
				}
				require.NoError(b, ss.Err())
				require.Empty(b, ss.Warnings())
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
		benchMsg := fmt.Sprintf("nSeries=%d,nBlocks=%d,cardinality=%d,pattern=\"%s\"", c.numSeries, c.numBlocks, c.cardinality, c.pattern)
		b.Run(benchMsg, func(b *testing.B) {
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
				block, err := OpenBlock(nil, createBlock(b, dir, generatedSeries), nil, nil)
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

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				ss := sq.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, "test", c.pattern))
				for ss.Next() {
				}
				require.NoError(b, ss.Err())
				require.Empty(b, ss.Warnings())
			}
		})
	}
}

func TestPostingsForMatchers(t *testing.T) {
	ctx := context.Background()

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
	app.Append(0, labels.FromStrings("n", "1", "i", "\n"), 0, 0)
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
				labels.FromStrings("n", "1", "i", "\n"),
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
				labels.FromStrings("n", "1", "i", "\n"),
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
				labels.FromStrings("n", "1", "i", "\n"),
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
				labels.FromStrings("n", "1", "i", "\n"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotEqual, "i", "")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
			},
		},
		// Regex.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", "^1$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
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
				labels.FromStrings("n", "1", "i", "\n"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^.+$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
			},
		},
		// Not regex.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "i", "")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "n", "^1$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "n", "1")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "n", "1|2.5")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "n", "(1|2.5)")},
			exp: []labels.Labels{
				labels.FromStrings("n", "2"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^a$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^a?$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
			},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
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
				labels.FromStrings("n", "1", "i", "\n"),
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
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "i", "(a|b)")},
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
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "i", "(c||d)")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		// Test shortcut for i=~".*"
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "i", ".*")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		// Test shortcut for n=~".*" and i=~"^.*$"
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", ".*"), labels.MustNewMatcher(labels.MatchRegexp, "i", "^.*$")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1"),
				labels.FromStrings("n", "1", "i", "a"),
				labels.FromStrings("n", "1", "i", "b"),
				labels.FromStrings("n", "1", "i", "\n"),
				labels.FromStrings("n", "2"),
				labels.FromStrings("n", "2.5"),
			},
		},
		// Test shortcut for n=~"^.*$"
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", "^.*$"), labels.MustNewMatcher(labels.MatchEqual, "i", "a")},
			exp: []labels.Labels{
				labels.FromStrings("n", "1", "i", "a"),
			},
		},
		// Test shortcut for i!~".*"
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "i", ".*")},
			exp:      []labels.Labels{},
		},
		// Test shortcut for n!~"^.*$",  i!~".*". First one triggers empty result.
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "n", "^.*$"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", ".*")},
			exp:      []labels.Labels{},
		},
		// Test shortcut i!~".*"
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", ".*"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", ".*")},
			exp:      []labels.Labels{},
		},
		// Test shortcut i!~"^.*$"
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "n", "1"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", "^.*$")},
			exp:      []labels.Labels{},
		},
		// Test shortcut i!~".+"
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "n", ".*"), labels.MustNewMatcher(labels.MatchNotRegexp, "i", ".+")},
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
		var name strings.Builder
		for i, matcher := range c.matchers {
			if i > 0 {
				name.WriteString(",")
			}
			name.WriteString(matcher.String())
		}
		t.Run(name.String(), func(t *testing.T) {
			exp := map[string]struct{}{}
			for _, l := range c.exp {
				exp[l.String()] = struct{}{}
			}
			p, err := PostingsForMatchers(ctx, ir, c.matchers...)
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
			require.Empty(t, exp, "Evaluating %v", c.matchers)
		})
	}
}

// TestQuerierIndexQueriesRace tests the index queries with racing appends.
func TestQuerierIndexQueriesRace(t *testing.T) {
	const testRepeats = 1000

	testCases := []struct {
		matchers []*labels.Matcher
	}{
		{
			matchers: []*labels.Matcher{
				// This matcher should involve the AllPostings posting list in calculating the posting lists.
				labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, "metric"),
			},
		},
		{
			matchers: []*labels.Matcher{
				// The first matcher should be effectively the same as AllPostings, because all series have always_0=0
				// If it is evaluated first, then __name__=metric will contain more series than always_0=0.
				labels.MustNewMatcher(labels.MatchNotEqual, "always_0", "0"),
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric"),
			},
		},
	}

	for _, c := range testCases {
		t.Run(fmt.Sprintf("%v", c.matchers), func(t *testing.T) {
			t.Parallel()
			db := newTestDB(t)
			h := db.Head()
			ctx, cancel := context.WithCancel(context.Background())
			wg := &sync.WaitGroup{}
			wg.Add(1)
			go appendSeries(t, ctx, wg, h)
			t.Cleanup(wg.Wait)
			t.Cleanup(cancel)

			for range testRepeats {
				q, err := db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)

				values, _, err := q.LabelValues(ctx, "seq", nil, c.matchers...)
				require.NoError(t, err)
				require.Emptyf(t, values, `label values for label "seq" should be empty`)

				// Sleep to give the appends some change to run.
				time.Sleep(time.Millisecond)
			}
		})
	}
}

func appendSeries(t *testing.T, ctx context.Context, wg *sync.WaitGroup, h *Head) {
	defer wg.Done()

	for i := 0; ctx.Err() == nil; i++ {
		app := h.Appender(context.Background())
		_, err := app.Append(0, labels.FromStrings(labels.MetricName, "metric", "seq", strconv.Itoa(i), "always_0", "0"), 0, 0)
		require.NoError(t, err)
		err = app.Commit()
		require.NoError(t, err)

		// Throttle down the appends to keep the test somewhat nimble.
		// Otherwise, we end up appending thousands or millions of samples.
		time.Sleep(time.Millisecond)
	}
}

// TestClose ensures that calling Close more than once doesn't block and doesn't panic.
func TestClose(t *testing.T) {
	dir := t.TempDir()

	createBlock(t, dir, genSeries(1, 1, 0, 10))
	createBlock(t, dir, genSeries(1, 1, 10, 20))

	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err, "Opening test storage failed: %s")
	defer func() {
		require.NoError(t, db.Close())
	}()

	q, err := db.Querier(0, 20)
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
					block, err := OpenBlock(nil, createBlock(b, dir, series), nil, nil)
					require.NoError(b, err)
					q, err := NewBlockQuerier(block, 1, nSamples)
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
					isoState := head.oooIso.TrackReadAfter(0)
					qOOOHead := NewHeadAndOOOQuerier(1, 1, nSamples, head, isoState, qHead)

					queryTypes = append(queryTypes, qt{
						fmt.Sprintf("_Head_oooPercent:%d", oooPercentage), qOOOHead,
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
	b.ReportAllocs()
	for b.Loop() {
		ss := q.Select(context.Background(), false, nil, selectors...)
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
		require.Empty(b, ss.Warnings())
		require.Equal(b, expExpansions, actualExpansions)
		require.NoError(b, ss.Err())
	}
}

// mockMatcherIndex is used to check if the regex matcher works as expected.
type mockMatcherIndex struct{}

func (mockMatcherIndex) Symbols() index.StringIter { return nil }

func (mockMatcherIndex) Close() error { return nil }

// SortedLabelValues will return error if it is called.
func (mockMatcherIndex) SortedLabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, error) {
	return []string{}, errors.New("sorted label values called")
}

// LabelValues will return error if it is called.
func (mockMatcherIndex) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, error) {
	return []string{}, errors.New("label values called")
}

func (mockMatcherIndex) LabelNamesFor(context.Context, index.Postings) ([]string, error) {
	return nil, errors.New("label names for called")
}

func (mockMatcherIndex) Postings(context.Context, string, ...string) (index.Postings, error) {
	return index.EmptyPostings(), nil
}

func (mockMatcherIndex) SortedPostings(index.Postings) index.Postings {
	return index.EmptyPostings()
}

func (mockMatcherIndex) ShardedPostings(ps index.Postings, _, _ uint64) index.Postings {
	return ps
}

func (mockMatcherIndex) Series(storage.SeriesRef, *labels.ScratchBuilder, *[]chunks.Meta) error {
	return nil
}

func (mockMatcherIndex) LabelNames(context.Context, ...*labels.Matcher) ([]string, error) {
	return []string{}, nil
}

func (mockMatcherIndex) PostingsForLabelMatching(context.Context, string, func(string) bool) index.Postings {
	return index.ErrPostings(errors.New("PostingsForLabelMatching called"))
}

func (mockMatcherIndex) PostingsForAllLabelValues(context.Context, string) index.Postings {
	return index.ErrPostings(errors.New("PostingsForAllLabelValues called"))
}

func TestPostingsForMatcher(t *testing.T) {
	ctx := context.Background()

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
		t.Run(tc.matcher.String(), func(t *testing.T) {
			ir := &mockMatcherIndex{}
			_, err := postingsForMatcher(ctx, ir, tc.matcher)
			if tc.hasError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
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
			require.Equal(t, tc.series[idx].chunks, si.metas)

			i++
		}
		require.Len(t, tc.expIdxs, i)
		require.NoError(t, bcs.Err())
	}
}

func BenchmarkHeadChunkQuerier(b *testing.B) {
	db := newTestDB(b)

	// 3h of data.
	numTimeseries := 100
	app := db.Appender(context.Background())
	for i := range 120 * 6 {
		for j := range numTimeseries {
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

	querier, err := db.ChunkQuerier(math.MinInt64, math.MaxInt64)
	require.NoError(b, err)
	defer func(q storage.ChunkQuerier) {
		require.NoError(b, q.Close())
	}(querier)
	b.ReportAllocs()

	for b.Loop() {
		ss := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
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
	db := newTestDB(b)

	// 3h of data.
	numTimeseries := 100
	app := db.Appender(context.Background())
	for i := range 120 * 6 {
		for j := range numTimeseries {
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

	querier, err := db.Querier(math.MinInt64, math.MaxInt64)
	require.NoError(b, err)
	defer func(q storage.Querier) {
		require.NoError(b, q.Close())
	}(querier)
	b.ReportAllocs()

	for b.Loop() {
		ss := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
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

// This is a regression test for the case where gauge histograms were not handled by
// populateWithDelChunkSeriesIterator correctly.
func TestQueryWithDeletedHistograms(t *testing.T) {
	ctx := context.Background()
	testcases := map[string]func(int) (*histogram.Histogram, *histogram.FloatHistogram){
		"intCounter": func(i int) (*histogram.Histogram, *histogram.FloatHistogram) {
			return tsdbutil.GenerateTestHistogram(int64(i)), nil
		},
		"intgauge": func(int) (*histogram.Histogram, *histogram.FloatHistogram) {
			return tsdbutil.GenerateTestGaugeHistogram(rand.Int63() % 1000), nil
		},
		"floatCounter": func(i int) (*histogram.Histogram, *histogram.FloatHistogram) {
			return nil, tsdbutil.GenerateTestFloatHistogram(int64(i))
		},
		"floatGauge": func(int) (*histogram.Histogram, *histogram.FloatHistogram) {
			return nil, tsdbutil.GenerateTestGaugeFloatHistogram(rand.Int63() % 1000)
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			db := newTestDB(t)
			app := db.Appender(context.Background())

			var (
				err       error
				seriesRef storage.SeriesRef
			)
			lbs := labels.FromStrings("__name__", "test", "type", name)

			for i := range 100 {
				h, fh := tc(i)
				seriesRef, err = app.AppendHistogram(seriesRef, lbs, int64(i), h, fh)
				require.NoError(t, err)
			}

			require.NoError(t, app.Commit())

			matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test")
			require.NoError(t, err)

			// Delete the last 20.
			err = db.Delete(ctx, 80, 100, matcher)
			require.NoError(t, err)

			chunkQuerier, err := db.ChunkQuerier(0, 100)
			require.NoError(t, err)

			css := chunkQuerier.Select(context.Background(), false, nil, matcher)

			seriesCount := 0
			for css.Next() {
				seriesCount++
				series := css.At()

				sampleCount := 0
				it := series.Iterator(nil)
				for it.Next() {
					chk := it.At()
					for cit := chk.Chunk.Iterator(nil); cit.Next() != chunkenc.ValNone; {
						sampleCount++
					}
				}
				require.NoError(t, it.Err())
				require.Equal(t, 80, sampleCount)
			}
			require.NoError(t, css.Err())
			require.Equal(t, 1, seriesCount)
		})
	}
}

func TestQueryWithOneChunkCompletelyDeleted(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	app := db.Appender(context.Background())

	var (
		err       error
		seriesRef storage.SeriesRef
	)
	lbs := labels.FromStrings("__name__", "test")

	// Create an int histogram chunk with samples between 0 - 20 and 30 - 40.
	for i := range 20 {
		h := tsdbutil.GenerateTestHistogram(1)
		seriesRef, err = app.AppendHistogram(seriesRef, lbs, int64(i), h, nil)
		require.NoError(t, err)
	}
	for i := 30; i < 40; i++ {
		h := tsdbutil.GenerateTestHistogram(1)
		seriesRef, err = app.AppendHistogram(seriesRef, lbs, int64(i), h, nil)
		require.NoError(t, err)
	}

	// Append some float histograms - float histograms are a different encoding
	// type from int histograms so a new chunk is created.
	for i := 60; i < 100; i++ {
		fh := tsdbutil.GenerateTestFloatHistogram(1)
		seriesRef, err = app.AppendHistogram(seriesRef, lbs, int64(i), nil, fh)
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())

	matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test")
	require.NoError(t, err)

	// Delete all samples from the int histogram chunk. The deletion intervals
	// doesn't cover the entire histogram chunk, but does cover all the samples
	// in the chunk. This case was previously not handled properly.
	err = db.Delete(ctx, 0, 20, matcher)
	require.NoError(t, err)
	err = db.Delete(ctx, 30, 40, matcher)
	require.NoError(t, err)

	chunkQuerier, err := db.ChunkQuerier(0, 100)
	require.NoError(t, err)

	css := chunkQuerier.Select(context.Background(), false, nil, matcher)

	seriesCount := 0
	for css.Next() {
		seriesCount++
		series := css.At()

		sampleCount := 0
		it := series.Iterator(nil)
		for it.Next() {
			chk := it.At()
			cit := chk.Chunk.Iterator(nil)
			for vt := cit.Next(); vt != chunkenc.ValNone; vt = cit.Next() {
				require.Equal(t, chunkenc.ValFloatHistogram, vt, "Only float histograms expected, other sample types should have been deleted.")
				sampleCount++
			}
		}
		require.NoError(t, it.Err())
		require.Equal(t, 40, sampleCount)
	}
	require.NoError(t, css.Err())
	require.Equal(t, 1, seriesCount)
}

func TestReader_PostingsForLabelMatchingHonorsContextCancel(t *testing.T) {
	ir := mockReaderOfLabels{}

	failAfter := uint64(mockReaderOfLabelsSeriesCount / 2 / checkContextEveryNIterations)
	ctx := &testutil.MockContextErrAfter{FailAfter: failAfter}
	_, err := labelValuesWithMatchers(ctx, ir, "__name__", nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".+"))

	require.Error(t, err)
	require.Equal(t, failAfter+1, ctx.Count()) // Plus one for the Err() call that puts the error in the result.
}

type mockReaderOfLabels struct{}

const mockReaderOfLabelsSeriesCount = checkContextEveryNIterations * 10

func (mockReaderOfLabels) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, error) {
	return make([]string, mockReaderOfLabelsSeriesCount), nil
}

func (mockReaderOfLabels) SortedLabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, error) {
	panic("SortedLabelValues called")
}

func (mockReaderOfLabels) Close() error {
	return nil
}

func (mockReaderOfLabels) LabelNames(context.Context, ...*labels.Matcher) ([]string, error) {
	panic("LabelNames called")
}

func (mockReaderOfLabels) LabelNamesFor(context.Context, index.Postings) ([]string, error) {
	panic("LabelNamesFor called")
}

func (mockReaderOfLabels) PostingsForLabelMatching(context.Context, string, func(string) bool) index.Postings {
	panic("PostingsForLabelMatching called")
}

func (mockReaderOfLabels) PostingsForAllLabelValues(context.Context, string) index.Postings {
	panic("PostingsForAllLabelValues called")
}

func (mockReaderOfLabels) Postings(context.Context, string, ...string) (index.Postings, error) {
	panic("Postings called")
}

func (mockReaderOfLabels) ShardedPostings(index.Postings, uint64, uint64) index.Postings {
	panic("Postings called")
}

func (mockReaderOfLabels) SortedPostings(index.Postings) index.Postings {
	panic("SortedPostings called")
}

func (mockReaderOfLabels) Series(storage.SeriesRef, *labels.ScratchBuilder, *[]chunks.Meta) error {
	panic("Series called")
}

func (mockReaderOfLabels) Symbols() index.StringIter {
	panic("Series called")
}

// TestMergeQuerierConcurrentSelectMatchers reproduces the data race bug from
// https://github.com/prometheus/prometheus/issues/14723, when one of the queriers (blockQuerier in this case)
// alters the passed matchers.
func TestMergeQuerierConcurrentSelectMatchers(t *testing.T) {
	block, err := OpenBlock(nil, createBlock(t, t.TempDir(), genSeries(1, 1, 0, 1)), nil, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, block.Close())
	}()
	p, err := NewBlockQuerier(block, 0, 1)
	require.NoError(t, err)

	// A secondary querier is required to enable concurrent select; a blockQuerier is used for simplicity.
	s, err := NewBlockQuerier(block, 0, 1)
	require.NoError(t, err)

	originalMatchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchRegexp, "baz", ".*"),
		labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
	}
	matchers := append([]*labels.Matcher{}, originalMatchers...)

	mergedQuerier := storage.NewMergeQuerier([]storage.Querier{p}, []storage.Querier{s}, storage.ChainedSeriesMerge)
	defer func() {
		require.NoError(t, mergedQuerier.Close())
	}()

	mergedQuerier.Select(context.Background(), false, nil, matchers...)

	require.Equal(t, originalMatchers, matchers)
}
