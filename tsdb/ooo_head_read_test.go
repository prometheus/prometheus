// Copyright 2022 The Prometheus Authors
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
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/compression"
)

type chunkInterval struct {
	// because we permutate the order of chunks, we cannot determine at test declaration time which chunkRefs we expect in the Output.
	// This ID matches expected output chunks against test input chunks, the test runner will assert the chunkRef for the matching chunk
	ID   int
	mint int64
	maxt int64
}

type expChunk struct {
	c chunkInterval
	m []chunkInterval
}

// permutateChunkIntervals returns all possible orders of the given chunkIntervals.
func permutateChunkIntervals(in []chunkInterval, out [][]chunkInterval, left, right int) [][]chunkInterval {
	if left == right {
		inCopy := make([]chunkInterval, len(in))
		copy(inCopy, in)
		return append(out, inCopy)
	}
	for i := left; i <= right; i++ {
		in[left], in[i] = in[i], in[left]
		out = permutateChunkIntervals(in, out, left+1, right)
		in[left], in[i] = in[i], in[left]
	}
	return out
}

// TestOOOHeadIndexReader_Series tests that the Series method works as expected.
// However it does so by creating chunks and memory mapping them unlike other
// tests of the head where samples are appended and we let the head memory map.
// We do this because the ingestion path and the appender for out of order
// samples are not ready yet.
func TestOOOHeadIndexReader_Series(t *testing.T) {
	tests := []struct {
		name                string
		queryMinT           int64
		queryMaxT           int64
		inputChunkIntervals []chunkInterval
		expChunks           []expChunk
	}{
		{
			name:      "Empty result and no error when head is empty",
			queryMinT: 0,
			queryMaxT: 100,
			expChunks: nil,
		},
		{
			name:      "If query interval is bigger than the existing chunks nothing is returned",
			queryMinT: 500,
			queryMaxT: 700,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 400},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval                                                                                                  [---------------------------------------]
			// Chunk 0                         [-----------------------------------------------------------]
			expChunks: nil,
		},
		{
			name:      "If query interval is smaller than the existing chunks nothing is returned",
			queryMinT: 100,
			queryMaxT: 400,
			inputChunkIntervals: []chunkInterval{
				{0, 500, 700},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval                [-----------------------------------------------------------]
			// Chunk 0:                                                                                                        [---------------------------------------]
			expChunks: nil,
		},
		{
			name:      "If query interval exceeds the existing chunk, it is returned",
			queryMinT: 100,
			queryMaxT: 400,
			inputChunkIntervals: []chunkInterval{
				{0, 150, 350},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval                [-----------------------------------------------------------]
			// Chunk 0:                                 [---------------------------------------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 150, 350}},
			},
		},
		{
			name:      "If chunk exceeds the query interval, it is returned",
			queryMinT: 150,
			queryMaxT: 350,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 400},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval:                          [---------------------------------------]
			// Chunk 0:                       [-----------------------------------------------------------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 100, 400}},
			},
		},
		{
			name:      "Pairwise overlaps should return the references of the first of each pair",
			queryMinT: 0,
			queryMaxT: 700,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 200},
				{1, 500, 600},
				{2, 150, 250},
				{3, 550, 650},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval        [---------------------------------------------------------------------------------------------------------------------------------]
			// Chunk 0:                        [-------------------]
			// Chunk 1:                                                                                                        [-------------------]
			// Chunk 2:                                  [-------------------]
			// Chunk 3:                                                                                                                  [-------------------]
			// Output Graphically              [-----------------------------]                                                 [-----------------------------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 100, 250}, m: []chunkInterval{{0, 100, 200}, {2, 150, 250}}},
				{c: chunkInterval{1, 500, 650}, m: []chunkInterval{{1, 500, 600}, {3, 550, 650}}},
			},
		},
		{
			name:      "If all chunks overlap, single big chunk is returned",
			queryMinT: 0,
			queryMaxT: 700,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 200},
				{1, 200, 300},
				{2, 300, 400},
				{3, 400, 500},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval        [---------------------------------------------------------------------------------------------------------------------------------]
			// Chunk 0:                        [-------------------]
			// Chunk 1:                                            [-------------------]
			// Chunk 2:                                                                [-------------------]
			// Chunk 3:                                                                                    [------------------]
			// Output Graphically              [------------------------------------------------------------------------------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 100, 500}, m: []chunkInterval{{0, 100, 200}, {1, 200, 300}, {2, 300, 400}, {3, 400, 500}}},
			},
		},
		{
			name:      "If no chunks overlap, all chunks are returned",
			queryMinT: 0,
			queryMaxT: 700,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 199},
				{1, 200, 299},
				{2, 300, 399},
				{3, 400, 499},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval        [---------------------------------------------------------------------------------------------------------------------------------]
			// Chunk 0:                        [------------------]
			// Chunk 1:                                            [------------------]
			// Chunk 2:                                                                [------------------]
			// Chunk 3:                                                                                    [------------------]
			// Output Graphically              [------------------][------------------][------------------][------------------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 100, 199}},
				{c: chunkInterval{1, 200, 299}},
				{c: chunkInterval{2, 300, 399}},
				{c: chunkInterval{3, 400, 499}},
			},
		},
		{
			name:      "Triplet with pairwise overlaps, query range covers all, and distractor extra chunk",
			queryMinT: 0,
			queryMaxT: 400,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 200},
				{1, 150, 300},
				{2, 250, 350},
				{3, 450, 550},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval        [--------------------------------------------------------------------]
			// Chunk 0:                        [------------------]
			// Chunk 1:                                 [-----------------------------]
			// Chunk 2:                                                     [------------------]
			// Chunk 3:                                                                                             [------------------]
			// Output Graphically              [-----------------------------------------------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 100, 350}, m: []chunkInterval{{0, 100, 200}, {1, 150, 300}, {2, 250, 350}}},
			},
		},
		{
			name:      "Query interval partially overlaps some chunks",
			queryMinT: 100,
			queryMaxT: 400,
			inputChunkIntervals: []chunkInterval{
				{0, 250, 500},
				{1, 0, 200},
				{2, 150, 300},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700
			// Query Interval                [------------------------------------------------------------]
			// Chunk 0:                                                     [-------------------------------------------------]
			// Chunk 1:             [-----------------------------]
			// Chunk 2:                                [------------------------------]
			// Output Graphically   [-----------------------------------------------------------------------------------------]
			expChunks: []expChunk{
				{c: chunkInterval{1, 0, 500}, m: []chunkInterval{{1, 0, 200}, {2, 150, 300}, {0, 250, 500}}},
			},
		},
		{
			name:      "A full overlap pair and disjointed triplet",
			queryMinT: 0,
			queryMaxT: 900,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 300},
				{1, 770, 850},
				{2, 150, 250},
				{3, 650, 750},
				{4, 600, 800},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700       750       800       850
			// Query Interval        [---------------------------------------------------------------------------------------------------------------------------------------------------------------]
			// Chunk 0:                        [---------------------------------------]
			// Chunk 1:                                                                                                                                                               [--------------]
			// Chunk 2:                                  [-------------------]
			// Chunk 3:                                                                                                                                      [-------------------]
			// Chunk 4:                                                                                                                             [---------------------------------------]
			// Output Graphically              [---------------------------------------]                                                            [------------------------------------------------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 100, 300}, m: []chunkInterval{{0, 100, 300}, {2, 150, 250}}},
				{c: chunkInterval{4, 600, 850}, m: []chunkInterval{{4, 600, 800}, {3, 650, 750}, {1, 770, 850}}},
			},
		},
		{
			name:      "Query range covers 3 disjoint chunks",
			queryMinT: 0,
			queryMaxT: 650,
			inputChunkIntervals: []chunkInterval{
				{0, 100, 150},
				{1, 300, 350},
				{2, 200, 250},
			},
			// ts                    0       100       150       200       250       300       350       400       450       500       550       600       650       700       750       800       850
			// Query Interval        [----------------------------------------------------------------------------------------------------------------------]
			// Chunk 0:                        [-------]
			// Chunk 1:                                                              [----------]
			// Chunk 2:                                           [--------]
			// Output Graphically              [-------]          [--------]         [----------]
			expChunks: []expChunk{
				{c: chunkInterval{0, 100, 150}},
				{c: chunkInterval{2, 200, 250}},
				{c: chunkInterval{1, 300, 350}},
			},
		},
	}

	s1Lset := labels.FromStrings("foo", "bar")
	s1ID := uint64(1)

	for _, tc := range tests {
		var permutations [][]chunkInterval
		if len(tc.inputChunkIntervals) == 0 {
			// handle special case
			permutations = [][]chunkInterval{
				nil,
			}
		} else {
			permutations = permutateChunkIntervals(tc.inputChunkIntervals, nil, 0, len(tc.inputChunkIntervals)-1)
		}
		for perm, intervals := range permutations {
			for _, headChunk := range []bool{false, true} {
				t.Run(fmt.Sprintf("name=%s, permutation=%d, headChunk=%t", tc.name, perm, headChunk), func(t *testing.T) {
					h, _ := newTestHead(t, 1000, compression.None, true)
					defer func() {
						require.NoError(t, h.Close())
					}()
					require.NoError(t, h.Init(0))

					s1, _, _ := h.getOrCreate(s1ID, s1Lset, false)
					s1.ooo = &memSeriesOOOFields{}

					// define our expected chunks, by looking at the expected ChunkIntervals and setting...
					// Ref to whatever Ref the chunk has, that we refer to by ID
					findID := func(id int) chunks.ChunkRef {
						for ref, c := range intervals {
							if c.ID == id {
								return chunks.ChunkRef(chunks.NewHeadChunkRef(chunks.HeadSeriesRef(s1ID), s1.oooHeadChunkID(ref)))
							}
						}
						return 0
					}
					var expChunks []chunks.Meta
					for _, e := range tc.expChunks {
						var chunk chunkenc.Chunk
						if len(e.m) > 0 {
							mm := &multiMeta{}
							for _, x := range e.m {
								meta := chunks.Meta{
									MinTime: x.mint,
									MaxTime: x.maxt,
									Ref:     findID(x.ID),
								}
								mm.metas = append(mm.metas, meta)
							}
							chunk = mm
						}
						meta := chunks.Meta{
							Chunk:   chunk,
							MinTime: e.c.mint,
							MaxTime: e.c.maxt,
							Ref:     findID(e.c.ID),
						}
						expChunks = append(expChunks, meta)
					}

					if headChunk && len(intervals) > 0 {
						// Put the last interval in the head chunk
						s1.ooo.oooHeadChunk = &oooHeadChunk{
							chunk:   NewOOOChunk(),
							minTime: intervals[len(intervals)-1].mint,
							maxTime: intervals[len(intervals)-1].maxt,
						}
						intervals = intervals[:len(intervals)-1]
					}

					for _, ic := range intervals {
						s1.ooo.oooMmappedChunks = append(s1.ooo.oooMmappedChunks, &mmappedChunk{
							minTime: ic.mint,
							maxTime: ic.maxt,
						})
					}

					ir := NewHeadAndOOOIndexReader(h, tc.queryMinT, tc.queryMinT, tc.queryMaxT, 0)

					var chks []chunks.Meta
					var b labels.ScratchBuilder
					err := ir.Series(storage.SeriesRef(s1ID), &b, &chks)
					require.NoError(t, err)
					require.Equal(t, s1Lset, b.Labels())
					require.Equal(t, expChunks, chks)

					err = ir.Series(storage.SeriesRef(s1ID+1), &b, &chks)
					require.Equal(t, storage.ErrNotFound, err)
				})
			}
		}
	}
}

func TestOOOHeadChunkReader_LabelValues(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOHeadChunkReader_LabelValues(t, scenario)
		})
	}
}

//nolint:revive // unexported-return
func testOOOHeadChunkReader_LabelValues(t *testing.T, scenario sampleTypeScenario) {
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, compression.None, true)
	t.Cleanup(func() { require.NoError(t, head.Close()) })

	ctx := context.Background()

	app := head.Appender(context.Background())

	// Add in-order samples
	_, _, err := scenario.appendFunc(app, labels.FromStrings("foo", "bar1"), 100, int64(1))
	require.NoError(t, err)
	_, _, err = scenario.appendFunc(app, labels.FromStrings("foo", "bar2"), 100, int64(2))
	require.NoError(t, err)

	// Add ooo samples for those series
	_, _, err = scenario.appendFunc(app, labels.FromStrings("foo", "bar1"), 90, int64(1))
	require.NoError(t, err)
	_, _, err = scenario.appendFunc(app, labels.FromStrings("foo", "bar2"), 90, int64(2))
	require.NoError(t, err)

	require.NoError(t, app.Commit())

	cases := []struct {
		name       string
		queryMinT  int64
		queryMaxT  int64
		expValues1 []string
		expValues2 []string
		expValues3 []string
		expValues4 []string
	}{
		{
			name:       "LabelValues calls when ooo head has max query range",
			queryMinT:  math.MinInt64,
			queryMaxT:  math.MaxInt64,
			expValues1: []string{"bar1"},
			expValues2: nil,
			expValues3: []string{"bar1", "bar2"},
			expValues4: []string{"bar1", "bar2"},
		},
		{
			name:       "LabelValues calls with ooo head query range not overlapping in-order data",
			queryMinT:  90,
			queryMaxT:  90,
			expValues1: []string{"bar1"},
			expValues2: nil,
			expValues3: []string{"bar1", "bar2"},
			expValues4: []string{"bar1", "bar2"},
		},
		{
			name:       "LabelValues calls with ooo head query range not overlapping out-of-order data",
			queryMinT:  100,
			queryMaxT:  100,
			expValues1: []string{"bar1"},
			expValues2: nil,
			expValues3: []string{"bar1", "bar2"},
			expValues4: []string{"bar1", "bar2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// We first want to test using a head index reader that covers the biggest query interval
			oh := NewHeadAndOOOIndexReader(head, tc.queryMinT, tc.queryMinT, tc.queryMaxT, 0)
			matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1")}
			values, err := oh.LabelValues(ctx, "foo", nil, matchers...)
			sort.Strings(values)
			require.NoError(t, err)
			require.Equal(t, tc.expValues1, values)

			matchers = []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "foo", "^bar.")}
			values, err = oh.LabelValues(ctx, "foo", nil, matchers...)
			sort.Strings(values)
			require.NoError(t, err)
			require.Equal(t, tc.expValues2, values)

			matchers = []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.")}
			values, err = oh.LabelValues(ctx, "foo", nil, matchers...)
			sort.Strings(values)
			require.NoError(t, err)
			require.Equal(t, tc.expValues3, values)

			values, err = oh.LabelValues(ctx, "foo", nil)
			sort.Strings(values)
			require.NoError(t, err)
			require.Equal(t, tc.expValues4, values)
		})
	}
}

// TestOOOHeadChunkReader_Chunk tests that the Chunk method works as expected.
// It does so by appending out of order samples to the db and then initializing
// an OOOHeadChunkReader to read chunks from it.
func TestOOOHeadChunkReader_Chunk(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOHeadChunkReader_Chunk(t, scenario)
		})
	}
}

//nolint:revive // unexported-return
func testOOOHeadChunkReader_Chunk(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 5
	opts.OutOfOrderTimeWindow = 120 * time.Minute.Milliseconds()

	s1 := labels.FromStrings("l", "v1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	t.Run("Getting a non existing chunk fails with not found error", func(t *testing.T) {
		db := newTestDBWithOpts(t, opts)

		cr := NewHeadAndOOOChunkReader(db.head, 0, 1000, nil, nil, 0)
		defer cr.Close()
		c, iterable, err := cr.ChunkOrIterable(chunks.Meta{
			Ref: 0x1800000, Chunk: chunkenc.Chunk(nil), MinTime: 100, MaxTime: 300,
		})
		require.Nil(t, iterable)
		require.EqualError(t, err, "not found")
		require.Nil(t, c)
	})

	tests := []struct {
		name                 string
		queryMinT            int64
		queryMaxT            int64
		firstInOrderSampleAt int64
		inputSamples         []testValue
		expSingleChunks      bool
		expChunkError        bool
		expChunksSamples     []chunks.SampleSlice
	}{
		{
			name:                 "Getting the head when there are no overlapping chunks returns just the samples in the head",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: []testValue{
				{Ts: minutes(30), V: 0},
				{Ts: minutes(40), V: 0},
			},
			expChunkError:   false,
			expSingleChunks: true,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0: Current Head                              [--------] (With 2 samples)
			// Output Graphically                                 [--------] (With 2 samples)
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(30), 0),
					scenario.sampleFunc(minutes(40), 0),
				},
			},
		},
		{
			name:                 "Getting the head chunk when there are overlapping chunks returns all combined",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples:         []testValue{{Ts: minutes(41), V: 0}, {Ts: minutes(42), V: 0}, {Ts: minutes(43), V: 0}, {Ts: minutes(44), V: 0}, {Ts: minutes(45), V: 0}, {Ts: minutes(30), V: 1}, {Ts: minutes(50), V: 1}},
			expChunkError:        false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0                                                     [---] (With 5 samples)
			// Chunk 1: Current Head                              [-----------------] (With 2 samples)
			// Output Graphically                                 [-----------------] (With 7 samples)
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(30), 1),
					scenario.sampleFunc(minutes(41), 0),
					scenario.sampleFunc(minutes(42), 0),
					scenario.sampleFunc(minutes(43), 0),
					scenario.sampleFunc(minutes(44), 0),
					scenario.sampleFunc(minutes(45), 0),
					scenario.sampleFunc(minutes(50), 1),
				},
			},
		},
		{
			name:                 "Two windows of overlapping chunks get properly converged",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: []testValue{
				// Chunk 0
				{Ts: minutes(10), V: 0},
				{Ts: minutes(12), V: 0},
				{Ts: minutes(14), V: 0},
				{Ts: minutes(16), V: 0},
				{Ts: minutes(20), V: 0},
				// Chunk 1
				{Ts: minutes(20), V: 1},
				{Ts: minutes(22), V: 1},
				{Ts: minutes(24), V: 1},
				{Ts: minutes(26), V: 1},
				{Ts: minutes(29), V: 1},
				// Chunk 3
				{Ts: minutes(30), V: 2},
				{Ts: minutes(32), V: 2},
				{Ts: minutes(34), V: 2},
				{Ts: minutes(36), V: 2},
				{Ts: minutes(40), V: 2},
				// Head
				{Ts: minutes(40), V: 3},
				{Ts: minutes(50), V: 3},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0                          [--------]
			// Chunk 1                                   [-------]
			// Chunk 2                                            [--------]
			// Chunk 3: Current Head                                       [--------]
			// Output Graphically               [----------------][-----------------]
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(10), 0),
					scenario.sampleFunc(minutes(12), 0),
					scenario.sampleFunc(minutes(14), 0),
					scenario.sampleFunc(minutes(16), 0),
					scenario.sampleFunc(minutes(20), 1),
					scenario.sampleFunc(minutes(22), 1),
					scenario.sampleFunc(minutes(24), 1),
					scenario.sampleFunc(minutes(26), 1),
					scenario.sampleFunc(minutes(29), 1),
				},
				{
					scenario.sampleFunc(minutes(30), 2),
					scenario.sampleFunc(minutes(32), 2),
					scenario.sampleFunc(minutes(34), 2),
					scenario.sampleFunc(minutes(36), 2),
					scenario.sampleFunc(minutes(40), 3),
					scenario.sampleFunc(minutes(50), 3),
				},
			},
		},
		{
			name:                 "Two windows of overlapping chunks in descending order get properly converged",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: []testValue{
				// Chunk 0
				{Ts: minutes(40), V: 0},
				{Ts: minutes(42), V: 0},
				{Ts: minutes(44), V: 0},
				{Ts: minutes(46), V: 0},
				{Ts: minutes(50), V: 0},
				// Chunk 1
				{Ts: minutes(30), V: 1},
				{Ts: minutes(32), V: 1},
				{Ts: minutes(34), V: 1},
				{Ts: minutes(36), V: 1},
				{Ts: minutes(40), V: 1},
				// Chunk 3
				{Ts: minutes(20), V: 2},
				{Ts: minutes(22), V: 2},
				{Ts: minutes(24), V: 2},
				{Ts: minutes(26), V: 2},
				{Ts: minutes(29), V: 2},
				// Head
				{Ts: minutes(10), V: 3},
				{Ts: minutes(20), V: 3},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0                                                     [--------]
			// Chunk 1                                            [--------]
			// Chunk 2                                   [-------]
			// Chunk 3: Current Head            [--------]
			// Output Graphically               [----------------][-----------------]
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(10), 3),
					scenario.sampleFunc(minutes(20), 2),
					scenario.sampleFunc(minutes(22), 2),
					scenario.sampleFunc(minutes(24), 2),
					scenario.sampleFunc(minutes(26), 2),
					scenario.sampleFunc(minutes(29), 2),
				},
				{
					scenario.sampleFunc(minutes(30), 1),
					scenario.sampleFunc(minutes(32), 1),
					scenario.sampleFunc(minutes(34), 1),
					scenario.sampleFunc(minutes(36), 1),
					scenario.sampleFunc(minutes(40), 0),
					scenario.sampleFunc(minutes(42), 0),
					scenario.sampleFunc(minutes(44), 0),
					scenario.sampleFunc(minutes(46), 0),
					scenario.sampleFunc(minutes(50), 0),
				},
			},
		},
		{
			name:                 "If chunks are not overlapped they are not converged",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: []testValue{
				// Chunk 0
				{Ts: minutes(10), V: 0},
				{Ts: minutes(12), V: 0},
				{Ts: minutes(14), V: 0},
				{Ts: minutes(16), V: 0},
				{Ts: minutes(18), V: 0},
				// Chunk 1
				{Ts: minutes(20), V: 1},
				{Ts: minutes(22), V: 1},
				{Ts: minutes(24), V: 1},
				{Ts: minutes(26), V: 1},
				{Ts: minutes(28), V: 1},
				// Chunk 3
				{Ts: minutes(30), V: 2},
				{Ts: minutes(32), V: 2},
				{Ts: minutes(34), V: 2},
				{Ts: minutes(36), V: 2},
				{Ts: minutes(38), V: 2},
				// Head
				{Ts: minutes(40), V: 3},
				{Ts: minutes(42), V: 3},
			},
			expChunkError:   false,
			expSingleChunks: true,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0                          [-------]
			// Chunk 1                                   [-------]
			// Chunk 2                                            [-------]
			// Chunk 3: Current Head                                       [-------]
			// Output Graphically               [-------][-------][-------][--------]
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(10), 0),
					scenario.sampleFunc(minutes(12), 0),
					scenario.sampleFunc(minutes(14), 0),
					scenario.sampleFunc(minutes(16), 0),
					scenario.sampleFunc(minutes(18), 0),
				},
				{
					scenario.sampleFunc(minutes(20), 1),
					scenario.sampleFunc(minutes(22), 1),
					scenario.sampleFunc(minutes(24), 1),
					scenario.sampleFunc(minutes(26), 1),
					scenario.sampleFunc(minutes(28), 1),
				},
				{
					scenario.sampleFunc(minutes(30), 2),
					scenario.sampleFunc(minutes(32), 2),
					scenario.sampleFunc(minutes(34), 2),
					scenario.sampleFunc(minutes(36), 2),
					scenario.sampleFunc(minutes(38), 2),
				},
				{
					scenario.sampleFunc(minutes(40), 3),
					scenario.sampleFunc(minutes(42), 3),
				},
			},
		},
		{
			name:                 "Triplet of chunks overlapping returns a single merged chunk",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: []testValue{
				// Chunk 0
				{Ts: minutes(10), V: 0},
				{Ts: minutes(15), V: 0},
				{Ts: minutes(20), V: 0},
				{Ts: minutes(25), V: 0},
				{Ts: minutes(30), V: 0},
				// Chunk 1
				{Ts: minutes(20), V: 1},
				{Ts: minutes(25), V: 1},
				{Ts: minutes(30), V: 1},
				{Ts: minutes(35), V: 1},
				{Ts: minutes(42), V: 1},
				// Chunk 2 Head
				{Ts: minutes(32), V: 2},
				{Ts: minutes(50), V: 2},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0                          [-----------------]
			// Chunk 1                                   [--------------------]
			// Chunk 2 Current Head                                  [--------------]
			// Output Graphically               [-----------------------------------]
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(10), 0),
					scenario.sampleFunc(minutes(15), 0),
					scenario.sampleFunc(minutes(20), 1),
					scenario.sampleFunc(minutes(25), 1),
					scenario.sampleFunc(minutes(30), 1),
					scenario.sampleFunc(minutes(32), 2),
					scenario.sampleFunc(minutes(35), 1),
					scenario.sampleFunc(minutes(42), 1),
					scenario.sampleFunc(minutes(50), 2),
				},
			},
		},
		{
			name:                 "Query interval partially overlaps with a triplet of chunks but still returns a single merged chunk",
			queryMinT:            minutes(12),
			queryMaxT:            minutes(33),
			firstInOrderSampleAt: minutes(120),
			inputSamples: []testValue{
				// Chunk 0
				{Ts: minutes(10), V: 0},
				{Ts: minutes(15), V: 0},
				{Ts: minutes(20), V: 0},
				{Ts: minutes(25), V: 0},
				{Ts: minutes(30), V: 0},
				// Chunk 1
				{Ts: minutes(20), V: 1},
				{Ts: minutes(25), V: 1},
				{Ts: minutes(30), V: 1},
				{Ts: minutes(35), V: 1},
				{Ts: minutes(42), V: 1},
				// Chunk 2 Head
				{Ts: minutes(32), V: 2},
				{Ts: minutes(50), V: 2},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval                     [------------------]
			// Chunk 0                          [-----------------]
			// Chunk 1                                   [--------------------]
			// Chunk 2 Current Head                                  [--------------]
			// Output Graphically               [-----------------------------------]
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(10), 0),
					scenario.sampleFunc(minutes(15), 0),
					scenario.sampleFunc(minutes(20), 1),
					scenario.sampleFunc(minutes(25), 1),
					scenario.sampleFunc(minutes(30), 1),
					scenario.sampleFunc(minutes(32), 2),
					scenario.sampleFunc(minutes(35), 1),
					scenario.sampleFunc(minutes(42), 1),
					scenario.sampleFunc(minutes(50), 2),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			db := newTestDBWithOpts(t, opts)

			app := db.Appender(context.Background())
			s1Ref, _, err := scenario.appendFunc(app, s1, tc.firstInOrderSampleAt, tc.firstInOrderSampleAt/1*time.Minute.Milliseconds())
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			// OOO few samples for s1.
			app = db.Appender(context.Background())
			for _, s := range tc.inputSamples {
				_, _, err := scenario.appendFunc(app, s1, s.Ts, s.V)
				require.NoError(t, err)
			}
			require.NoError(t, app.Commit())

			// The Series method populates the chunk metas, taking a copy of the
			// head OOO chunk if necessary. These are then used by the ChunkReader.
			ir := NewHeadAndOOOIndexReader(db.head, tc.queryMinT, tc.queryMinT, tc.queryMaxT, 0)
			var chks []chunks.Meta
			var b labels.ScratchBuilder
			err = ir.Series(s1Ref, &b, &chks)
			require.NoError(t, err)
			require.Len(t, chks, len(tc.expChunksSamples))

			cr := NewHeadAndOOOChunkReader(db.head, tc.queryMinT, tc.queryMaxT, nil, nil, 0)
			defer cr.Close()
			for i := 0; i < len(chks); i++ {
				c, iterable, err := cr.ChunkOrIterable(chks[i])
				require.NoError(t, err)
				var it chunkenc.Iterator
				if tc.expSingleChunks {
					it = c.Iterator(nil)
				} else {
					require.Nil(t, c)
					it = iterable.Iterator(nil)
				}
				resultSamples, err := storage.ExpandSamples(it, nil)
				require.NoError(t, err)
				requireEqualSamples(t, s1.String(), tc.expChunksSamples[i], resultSamples, requireEqualSamplesIgnoreCounterResets)
			}
		})
	}
}

// TestOOOHeadChunkReader_Chunk_ConsistentQueryResponseDespiteOfHeadExpanding tests
// that if a query comes and performs a Series() call followed by a Chunks() call
// the response is consistent with the data seen by Series() even if the OOO
// head receives more samples before Chunks() is called.
// An example:
//   - Response A comes from: Series() then Chunk()
//   - Response B comes from : Series(), in parallel new samples added to the head, then Chunk()
//   - A == B
func TestOOOHeadChunkReader_Chunk_ConsistentQueryResponseDespiteOfHeadExpanding(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOHeadChunkReader_Chunk_ConsistentQueryResponseDespiteOfHeadExpanding(t, scenario)
		})
	}
}

//nolint:revive // unexported-return
func testOOOHeadChunkReader_Chunk_ConsistentQueryResponseDespiteOfHeadExpanding(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 5
	opts.OutOfOrderTimeWindow = 120 * time.Minute.Milliseconds()

	s1 := labels.FromStrings("l", "v1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	tests := []struct {
		name                   string
		queryMinT              int64
		queryMaxT              int64
		firstInOrderSampleAt   int64
		initialSamples         []testValue
		samplesAfterSeriesCall []testValue
		expChunkError          bool
		expChunksSamples       []chunks.SampleSlice
	}{
		{
			name:                 "Current head gets old, new and in between sample after Series call, they all should be omitted from the result",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			initialSamples: []testValue{
				// Chunk 0
				{Ts: minutes(20), V: 0},
				{Ts: minutes(22), V: 0},
				{Ts: minutes(24), V: 0},
				{Ts: minutes(26), V: 0},
				{Ts: minutes(30), V: 0},
				// Chunk 1 Head
				{Ts: minutes(25), V: 1},
				{Ts: minutes(35), V: 1},
			},
			samplesAfterSeriesCall: []testValue{
				{Ts: minutes(10), V: 1},
				{Ts: minutes(32), V: 1},
				{Ts: minutes(50), V: 1},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [-----------------------------------]
			// Chunk 0:                                  [--------] (5 samples)
			// Chunk 1: Current Head                          [-------] (2 samples)
			// New samples added after Series()
			// Chunk 1: Current Head            [-----------------------------------] (5 samples)
			// Output Graphically                        [------------] (With 8 samples, samples newer than lastmint or older than lastmaxt are omitted but the ones in between are kept)
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(20), 0),
					scenario.sampleFunc(minutes(22), 0),
					scenario.sampleFunc(minutes(24), 0),
					scenario.sampleFunc(minutes(25), 1),
					scenario.sampleFunc(minutes(26), 0),
					scenario.sampleFunc(minutes(30), 0),
					scenario.sampleFunc(minutes(35), 1),
				},
			},
		},
		{
			name:                 "After Series() prev head mmapped after getting samples, new head gets new samples also overlapping, none should appear in response.",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			initialSamples: []testValue{
				// Chunk 0
				{Ts: minutes(20), V: 0},
				{Ts: minutes(22), V: 0},
				{Ts: minutes(24), V: 0},
				{Ts: minutes(26), V: 0},
				{Ts: minutes(30), V: 0},
				// Chunk 1 Head
				{Ts: minutes(25), V: 1},
				{Ts: minutes(35), V: 1},
			},
			samplesAfterSeriesCall: []testValue{
				{Ts: minutes(10), V: 1},
				{Ts: minutes(32), V: 1},
				{Ts: minutes(50), V: 1},
				// Chunk 1 gets mmapped and Chunk 2, the new head is born
				{Ts: minutes(25), V: 2},
				{Ts: minutes(31), V: 2},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [-----------------------------------]
			// Chunk 0:                                  [--------] (5 samples)
			// Chunk 1: Current Head                          [-------] (2 samples)
			// New samples added after Series()
			// Chunk 1 (mmapped)                     [-------------------------] (5 samples)
			// Chunk 2: Current Head                    [-----------] (2 samples)
			// Output Graphically                        [------------]  (8 samples) It has 5 from Chunk 0 and 3 from Chunk 1
			expChunksSamples: []chunks.SampleSlice{
				{
					scenario.sampleFunc(minutes(20), 0),
					scenario.sampleFunc(minutes(22), 0),
					scenario.sampleFunc(minutes(24), 0),
					scenario.sampleFunc(minutes(25), 1),
					scenario.sampleFunc(minutes(26), 0),
					scenario.sampleFunc(minutes(30), 0),
					scenario.sampleFunc(minutes(35), 1),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			db := newTestDBWithOpts(t, opts)

			app := db.Appender(context.Background())
			s1Ref, _, err := scenario.appendFunc(app, s1, tc.firstInOrderSampleAt, tc.firstInOrderSampleAt/1*time.Minute.Milliseconds())
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			// OOO few samples for s1.
			app = db.Appender(context.Background())
			for _, s := range tc.initialSamples {
				_, _, err := scenario.appendFunc(app, s1, s.Ts, s.V)
				require.NoError(t, err)
			}
			require.NoError(t, app.Commit())

			// The Series method populates the chunk metas, taking a copy of the
			// head OOO chunk if necessary. These are then used by the ChunkReader.
			ir := NewHeadAndOOOIndexReader(db.head, tc.queryMinT, tc.queryMinT, tc.queryMaxT, 0)
			var chks []chunks.Meta
			var b labels.ScratchBuilder
			err = ir.Series(s1Ref, &b, &chks)
			require.NoError(t, err)
			require.Len(t, chks, len(tc.expChunksSamples))

			// Now we keep receiving ooo samples
			// OOO few samples for s1.
			app = db.Appender(context.Background())
			for _, s := range tc.samplesAfterSeriesCall {
				_, _, err = scenario.appendFunc(app, s1, s.Ts, s.V)
				require.NoError(t, err)
			}
			require.NoError(t, app.Commit())

			cr := NewHeadAndOOOChunkReader(db.head, tc.queryMinT, tc.queryMaxT, nil, nil, 0)
			defer cr.Close()
			for i := 0; i < len(chks); i++ {
				c, iterable, err := cr.ChunkOrIterable(chks[i])
				require.NoError(t, err)
				require.Nil(t, c)

				it := iterable.Iterator(nil)
				resultSamples, err := storage.ExpandSamples(it, nil)
				require.NoError(t, err)
				requireEqualSamples(t, s1.String(), tc.expChunksSamples[i], resultSamples, requireEqualSamplesIgnoreCounterResets)
			}
		})
	}
}

// TestSortMetaByMinTimeAndMinRef tests that the sort function for chunk metas does sort
// by chunk meta MinTime and in case of same references by the lower reference.
func TestSortMetaByMinTimeAndMinRef(t *testing.T) {
	tests := []struct {
		name       string
		inputMetas []chunks.Meta
		expMetas   []chunks.Meta
	}{
		{
			name: "chunks are ordered by min time",
			inputMetas: []chunks.Meta{
				{
					Ref:     0,
					MinTime: 0,
				},
				{
					Ref:     1,
					MinTime: 1,
				},
			},
			expMetas: []chunks.Meta{
				{
					Ref:     0,
					MinTime: 0,
				},
				{
					Ref:     1,
					MinTime: 1,
				},
			},
		},
		{
			name: "if same mintime, lower reference goes first",
			inputMetas: []chunks.Meta{
				{
					Ref:     10,
					MinTime: 0,
				},
				{
					Ref:     5,
					MinTime: 0,
				},
			},
			expMetas: []chunks.Meta{
				{
					Ref:     5,
					MinTime: 0,
				},
				{
					Ref:     10,
					MinTime: 0,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			slices.SortFunc(tc.inputMetas, lessByMinTimeAndMinRef)
			require.Equal(t, tc.expMetas, tc.inputMetas)
		})
	}
}

func newTestDBWithOpts(t *testing.T, opts *Options) *DB {
	dir := t.TempDir()

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}
