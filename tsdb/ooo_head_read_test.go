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
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

type chunkInterval struct {
	// because we permutate the order of chunks, we cannot determine at test declaration time which chunkRefs we expect in the Output.
	// This ID matches expected output chunks against test input chunks, the test runner will assert the chunkRef for the matching chunk
	ID   int
	mint int64
	maxt int64
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
		expChunks           []chunkInterval
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
			expChunks: []chunkInterval{
				{0, 150, 350},
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
			expChunks: []chunkInterval{
				{0, 100, 400},
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
			expChunks: []chunkInterval{
				{0, 100, 250},
				{1, 500, 650},
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
			expChunks: []chunkInterval{
				{0, 100, 500},
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
			expChunks: []chunkInterval{
				{0, 100, 199},
				{1, 200, 299},
				{2, 300, 399},
				{3, 400, 499},
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
			expChunks: []chunkInterval{
				{0, 100, 350},
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
			expChunks: []chunkInterval{
				{1, 0, 500},
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
			expChunks: []chunkInterval{
				{0, 100, 300},
				{4, 600, 850},
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
			expChunks: []chunkInterval{
				{0, 100, 150},
				{1, 300, 350},
				{2, 200, 250},
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
					h, _ := newTestHead(t, 1000, wlog.CompressionNone, true)
					defer func() {
						require.NoError(t, h.Close())
					}()
					require.NoError(t, h.Init(0))

					s1, _, _ := h.getOrCreate(s1ID, s1Lset)
					s1.ooo = &memSeriesOOOFields{}

					var lastChunk chunkInterval
					var lastChunkPos int

					// the marker should be set based on whichever is the last chunk/interval that overlaps with the query range
					for i, interv := range intervals {
						if overlapsClosedInterval(interv.mint, interv.maxt, tc.queryMinT, tc.queryMaxT) {
							lastChunk = interv
							lastChunkPos = i
						}
					}
					lastChunkRef := chunks.ChunkRef(chunks.NewHeadChunkRef(1, chunks.HeadChunkID(uint64(lastChunkPos))))

					// define our expected chunks, by looking at the expected ChunkIntervals and setting...
					var expChunks []chunks.Meta
					for _, e := range tc.expChunks {
						meta := chunks.Meta{
							Chunk:   chunkenc.Chunk(nil),
							MinTime: e.mint,
							MaxTime: e.maxt,
							// markers based on the last chunk we found above
							OOOLastMinTime: lastChunk.mint,
							OOOLastMaxTime: lastChunk.maxt,
							OOOLastRef:     lastChunkRef,
						}

						// Ref to whatever Ref the chunk has, that we refer to by ID
						for ref, c := range intervals {
							if c.ID == e.ID {
								meta.Ref = chunks.ChunkRef(chunks.NewHeadChunkRef(chunks.HeadSeriesRef(s1ID), chunks.HeadChunkID(ref)))
								break
							}
						}
						expChunks = append(expChunks, meta)
					}
					slices.SortFunc(expChunks, lessByMinTimeAndMinRef) // We always want the chunks to come back sorted by minTime asc.

					if headChunk && len(intervals) > 0 {
						// Put the last interval in the head chunk
						s1.ooo.oooHeadChunk = &oooHeadChunk{
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

					ir := NewOOOHeadIndexReader(h, tc.queryMinT, tc.queryMaxT, 0)

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
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, wlog.CompressionNone, true)
	t.Cleanup(func() { require.NoError(t, head.Close()) })

	ctx := context.Background()

	app := head.Appender(context.Background())

	// Add in-order samples
	_, err := app.Append(0, labels.FromStrings("foo", "bar1"), 100, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("foo", "bar2"), 100, 2)
	require.NoError(t, err)

	// Add ooo samples for those series
	_, err = app.Append(0, labels.FromStrings("foo", "bar1"), 90, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("foo", "bar2"), 90, 2)
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
			expValues2: []string{},
			expValues3: []string{"bar1", "bar2"},
			expValues4: []string{"bar1", "bar2"},
		},
		{
			name:       "LabelValues calls with ooo head query range not overlapping in-order data",
			queryMinT:  90,
			queryMaxT:  90,
			expValues1: []string{"bar1"},
			expValues2: []string{},
			expValues3: []string{"bar1", "bar2"},
			expValues4: []string{"bar1", "bar2"},
		},
		{
			name:       "LabelValues calls with ooo head query range not overlapping out-of-order data",
			queryMinT:  100,
			queryMaxT:  100,
			expValues1: []string{},
			expValues2: []string{},
			expValues3: []string{},
			expValues4: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// We first want to test using a head index reader that covers the biggest query interval
			oh := NewOOOHeadIndexReader(head, tc.queryMinT, tc.queryMaxT, 0)
			matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar1")}
			values, err := oh.LabelValues(ctx, "foo", matchers...)
			sort.Strings(values)
			require.NoError(t, err)
			require.Equal(t, tc.expValues1, values)

			matchers = []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotRegexp, "foo", "^bar.")}
			values, err = oh.LabelValues(ctx, "foo", matchers...)
			sort.Strings(values)
			require.NoError(t, err)
			require.Equal(t, tc.expValues2, values)

			matchers = []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.")}
			values, err = oh.LabelValues(ctx, "foo", matchers...)
			sort.Strings(values)
			require.NoError(t, err)
			require.Equal(t, tc.expValues3, values)

			values, err = oh.LabelValues(ctx, "foo")
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
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 5
	opts.OutOfOrderTimeWindow = 120 * time.Minute.Milliseconds()

	s1 := labels.FromStrings("l", "v1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	appendSample := func(app storage.Appender, l labels.Labels, timestamp int64, value float64) storage.SeriesRef {
		ref, err := app.Append(0, l, timestamp, value)
		require.NoError(t, err)
		return ref
	}

	t.Run("Getting a non existing chunk fails with not found error", func(t *testing.T) {
		db := newTestDBWithOpts(t, opts)

		cr := NewOOOHeadChunkReader(db.head, 0, 1000, nil)
		defer cr.Close()
		c, iterable, err := cr.ChunkOrIterable(chunks.Meta{
			Ref: 0x1000000, Chunk: chunkenc.Chunk(nil), MinTime: 100, MaxTime: 300,
		})
		require.Nil(t, iterable)
		require.Equal(t, err, fmt.Errorf("not found"))
		require.Nil(t, c)
	})

	tests := []struct {
		name                 string
		queryMinT            int64
		queryMaxT            int64
		firstInOrderSampleAt int64
		inputSamples         chunks.SampleSlice
		expChunkError        bool
		expChunksSamples     []chunks.SampleSlice
	}{
		{
			name:                 "Getting the head when there are no overlapping chunks returns just the samples in the head",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: chunks.SampleSlice{
				sample{t: minutes(30), f: float64(0)},
				sample{t: minutes(40), f: float64(0)},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0: Current Head                              [--------] (With 2 samples)
			// Output Graphically                                 [--------] (With 2 samples)
			expChunksSamples: []chunks.SampleSlice{
				{
					sample{t: minutes(30), f: float64(0)},
					sample{t: minutes(40), f: float64(0)},
				},
			},
		},
		{
			name:                 "Getting the head chunk when there are overlapping chunks returns all combined",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: chunks.SampleSlice{
				// opts.OOOCapMax is 5 so these will be mmapped to the first mmapped chunk
				sample{t: minutes(41), f: float64(0)},
				sample{t: minutes(42), f: float64(0)},
				sample{t: minutes(43), f: float64(0)},
				sample{t: minutes(44), f: float64(0)},
				sample{t: minutes(45), f: float64(0)},
				// The following samples will go to the head chunk, and we want it
				// to overlap with the previous chunk
				sample{t: minutes(30), f: float64(1)},
				sample{t: minutes(50), f: float64(1)},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0                                                     [---] (With 5 samples)
			// Chunk 1: Current Head                              [-----------------] (With 2 samples)
			// Output Graphically                                 [-----------------] (With 7 samples)
			expChunksSamples: []chunks.SampleSlice{
				{
					sample{t: minutes(30), f: float64(1)},
					sample{t: minutes(41), f: float64(0)},
					sample{t: minutes(42), f: float64(0)},
					sample{t: minutes(43), f: float64(0)},
					sample{t: minutes(44), f: float64(0)},
					sample{t: minutes(45), f: float64(0)},
					sample{t: minutes(50), f: float64(1)},
				},
			},
		},
		{
			name:                 "Two windows of overlapping chunks get properly converged",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: chunks.SampleSlice{
				// Chunk 0
				sample{t: minutes(10), f: float64(0)},
				sample{t: minutes(12), f: float64(0)},
				sample{t: minutes(14), f: float64(0)},
				sample{t: minutes(16), f: float64(0)},
				sample{t: minutes(20), f: float64(0)},
				// Chunk 1
				sample{t: minutes(20), f: float64(1)},
				sample{t: minutes(22), f: float64(1)},
				sample{t: minutes(24), f: float64(1)},
				sample{t: minutes(26), f: float64(1)},
				sample{t: minutes(29), f: float64(1)},
				// Chunk 2
				sample{t: minutes(30), f: float64(2)},
				sample{t: minutes(32), f: float64(2)},
				sample{t: minutes(34), f: float64(2)},
				sample{t: minutes(36), f: float64(2)},
				sample{t: minutes(40), f: float64(2)},
				// Head
				sample{t: minutes(40), f: float64(3)},
				sample{t: minutes(50), f: float64(3)},
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
					sample{t: minutes(10), f: float64(0)},
					sample{t: minutes(12), f: float64(0)},
					sample{t: minutes(14), f: float64(0)},
					sample{t: minutes(16), f: float64(0)},
					sample{t: minutes(20), f: float64(1)},
					sample{t: minutes(22), f: float64(1)},
					sample{t: minutes(24), f: float64(1)},
					sample{t: minutes(26), f: float64(1)},
					sample{t: minutes(29), f: float64(1)},
				},
				{
					sample{t: minutes(30), f: float64(2)},
					sample{t: minutes(32), f: float64(2)},
					sample{t: minutes(34), f: float64(2)},
					sample{t: minutes(36), f: float64(2)},
					sample{t: minutes(40), f: float64(3)},
					sample{t: minutes(50), f: float64(3)},
				},
			},
		},
		{
			name:                 "Two windows of overlapping chunks in descending order get properly converged",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: chunks.SampleSlice{
				// Chunk 0
				sample{t: minutes(40), f: float64(0)},
				sample{t: minutes(42), f: float64(0)},
				sample{t: minutes(44), f: float64(0)},
				sample{t: minutes(46), f: float64(0)},
				sample{t: minutes(50), f: float64(0)},
				// Chunk 1
				sample{t: minutes(30), f: float64(1)},
				sample{t: minutes(32), f: float64(1)},
				sample{t: minutes(34), f: float64(1)},
				sample{t: minutes(36), f: float64(1)},
				sample{t: minutes(40), f: float64(1)},
				// Chunk 2
				sample{t: minutes(20), f: float64(2)},
				sample{t: minutes(22), f: float64(2)},
				sample{t: minutes(24), f: float64(2)},
				sample{t: minutes(26), f: float64(2)},
				sample{t: minutes(29), f: float64(2)},
				// Head
				sample{t: minutes(10), f: float64(3)},
				sample{t: minutes(20), f: float64(3)},
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
					sample{t: minutes(10), f: float64(3)},
					sample{t: minutes(20), f: float64(2)},
					sample{t: minutes(22), f: float64(2)},
					sample{t: minutes(24), f: float64(2)},
					sample{t: minutes(26), f: float64(2)},
					sample{t: minutes(29), f: float64(2)},
				},
				{
					sample{t: minutes(30), f: float64(1)},
					sample{t: minutes(32), f: float64(1)},
					sample{t: minutes(34), f: float64(1)},
					sample{t: minutes(36), f: float64(1)},
					sample{t: minutes(40), f: float64(0)},
					sample{t: minutes(42), f: float64(0)},
					sample{t: minutes(44), f: float64(0)},
					sample{t: minutes(46), f: float64(0)},
					sample{t: minutes(50), f: float64(0)},
				},
			},
		},
		{
			name:                 "If chunks are not overlapped they are not converged",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: chunks.SampleSlice{
				// Chunk 0
				sample{t: minutes(10), f: float64(0)},
				sample{t: minutes(12), f: float64(0)},
				sample{t: minutes(14), f: float64(0)},
				sample{t: minutes(16), f: float64(0)},
				sample{t: minutes(18), f: float64(0)},
				// Chunk 1
				sample{t: minutes(20), f: float64(1)},
				sample{t: minutes(22), f: float64(1)},
				sample{t: minutes(24), f: float64(1)},
				sample{t: minutes(26), f: float64(1)},
				sample{t: minutes(28), f: float64(1)},
				// Chunk 2
				sample{t: minutes(30), f: float64(2)},
				sample{t: minutes(32), f: float64(2)},
				sample{t: minutes(34), f: float64(2)},
				sample{t: minutes(36), f: float64(2)},
				sample{t: minutes(38), f: float64(2)},
				// Head
				sample{t: minutes(40), f: float64(3)},
				sample{t: minutes(42), f: float64(3)},
			},
			expChunkError: false,
			// ts (in minutes)         0       10       20       30       40       50       60       70       80       90       100
			// Query Interval          [------------------------------------------------------------------------------------------]
			// Chunk 0                          [-------]
			// Chunk 1                                   [-------]
			// Chunk 2                                            [-------]
			// Chunk 3: Current Head                                       [-------]
			// Output Graphically               [-------][-------][-------][--------]
			expChunksSamples: []chunks.SampleSlice{
				{
					sample{t: minutes(10), f: float64(0)},
					sample{t: minutes(12), f: float64(0)},
					sample{t: minutes(14), f: float64(0)},
					sample{t: minutes(16), f: float64(0)},
					sample{t: minutes(18), f: float64(0)},
				},
				{
					sample{t: minutes(20), f: float64(1)},
					sample{t: minutes(22), f: float64(1)},
					sample{t: minutes(24), f: float64(1)},
					sample{t: minutes(26), f: float64(1)},
					sample{t: minutes(28), f: float64(1)},
				},
				{
					sample{t: minutes(30), f: float64(2)},
					sample{t: minutes(32), f: float64(2)},
					sample{t: minutes(34), f: float64(2)},
					sample{t: minutes(36), f: float64(2)},
					sample{t: minutes(38), f: float64(2)},
				},
				{
					sample{t: minutes(40), f: float64(3)},
					sample{t: minutes(42), f: float64(3)},
				},
			},
		},
		{
			name:                 "Triplet of chunks overlapping returns a single merged chunk",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			inputSamples: chunks.SampleSlice{
				// Chunk 0
				sample{t: minutes(10), f: float64(0)},
				sample{t: minutes(15), f: float64(0)},
				sample{t: minutes(20), f: float64(0)},
				sample{t: minutes(25), f: float64(0)},
				sample{t: minutes(30), f: float64(0)},
				// Chunk 1
				sample{t: minutes(20), f: float64(1)},
				sample{t: minutes(25), f: float64(1)},
				sample{t: minutes(30), f: float64(1)},
				sample{t: minutes(35), f: float64(1)},
				sample{t: minutes(42), f: float64(1)},
				// Chunk 2 Head
				sample{t: minutes(32), f: float64(2)},
				sample{t: minutes(50), f: float64(2)},
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
					sample{t: minutes(10), f: float64(0)},
					sample{t: minutes(15), f: float64(0)},
					sample{t: minutes(20), f: float64(1)},
					sample{t: minutes(25), f: float64(1)},
					sample{t: minutes(30), f: float64(1)},
					sample{t: minutes(32), f: float64(2)},
					sample{t: minutes(35), f: float64(1)},
					sample{t: minutes(42), f: float64(1)},
					sample{t: minutes(50), f: float64(2)},
				},
			},
		},
		{
			name:                 "Query interval partially overlaps with a triplet of chunks but still returns a single merged chunk",
			queryMinT:            minutes(12),
			queryMaxT:            minutes(33),
			firstInOrderSampleAt: minutes(120),
			inputSamples: chunks.SampleSlice{
				// Chunk 0
				sample{t: minutes(10), f: float64(0)},
				sample{t: minutes(15), f: float64(0)},
				sample{t: minutes(20), f: float64(0)},
				sample{t: minutes(25), f: float64(0)},
				sample{t: minutes(30), f: float64(0)},
				// Chunk 1
				sample{t: minutes(20), f: float64(1)},
				sample{t: minutes(25), f: float64(1)},
				sample{t: minutes(30), f: float64(1)},
				sample{t: minutes(35), f: float64(1)},
				sample{t: minutes(42), f: float64(1)},
				// Chunk 2 Head
				sample{t: minutes(32), f: float64(2)},
				sample{t: minutes(50), f: float64(2)},
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
					sample{t: minutes(10), f: float64(0)},
					sample{t: minutes(15), f: float64(0)},
					sample{t: minutes(20), f: float64(1)},
					sample{t: minutes(25), f: float64(1)},
					sample{t: minutes(30), f: float64(1)},
					sample{t: minutes(32), f: float64(2)},
					sample{t: minutes(35), f: float64(1)},
					sample{t: minutes(42), f: float64(1)},
					sample{t: minutes(50), f: float64(2)},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			db := newTestDBWithOpts(t, opts)

			app := db.Appender(context.Background())
			s1Ref := appendSample(app, s1, tc.firstInOrderSampleAt, float64(tc.firstInOrderSampleAt/1*time.Minute.Milliseconds()))
			require.NoError(t, app.Commit())

			// OOO few samples for s1.
			app = db.Appender(context.Background())
			for _, s := range tc.inputSamples {
				appendSample(app, s1, s.T(), s.F())
			}
			require.NoError(t, app.Commit())

			// The Series method is the one that populates the chunk meta OOO
			// markers like OOOLastRef. These are then used by the ChunkReader.
			ir := NewOOOHeadIndexReader(db.head, tc.queryMinT, tc.queryMaxT, 0)
			var chks []chunks.Meta
			var b labels.ScratchBuilder
			err := ir.Series(s1Ref, &b, &chks)
			require.NoError(t, err)
			require.Equal(t, len(tc.expChunksSamples), len(chks))

			cr := NewOOOHeadChunkReader(db.head, tc.queryMinT, tc.queryMaxT, nil)
			defer cr.Close()
			for i := 0; i < len(chks); i++ {
				c, iterable, err := cr.ChunkOrIterable(chks[i])
				require.NoError(t, err)
				require.Nil(t, c)

				var resultSamples chunks.SampleSlice
				it := iterable.Iterator(nil)
				for it.Next() == chunkenc.ValFloat {
					t, v := it.At()
					resultSamples = append(resultSamples, sample{t: t, f: v})
				}
				require.Equal(t, tc.expChunksSamples[i], resultSamples)
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
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 5
	opts.OutOfOrderTimeWindow = 120 * time.Minute.Milliseconds()

	s1 := labels.FromStrings("l", "v1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	appendSample := func(app storage.Appender, l labels.Labels, timestamp int64, value float64) storage.SeriesRef {
		ref, err := app.Append(0, l, timestamp, value)
		require.NoError(t, err)
		return ref
	}

	tests := []struct {
		name                   string
		queryMinT              int64
		queryMaxT              int64
		firstInOrderSampleAt   int64
		initialSamples         chunks.SampleSlice
		samplesAfterSeriesCall chunks.SampleSlice
		expChunkError          bool
		expChunksSamples       []chunks.SampleSlice
	}{
		{
			name:                 "Current head gets old, new and in between sample after Series call, they all should be omitted from the result",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			initialSamples: chunks.SampleSlice{
				// Chunk 0
				sample{t: minutes(20), f: float64(0)},
				sample{t: minutes(22), f: float64(0)},
				sample{t: minutes(24), f: float64(0)},
				sample{t: minutes(26), f: float64(0)},
				sample{t: minutes(30), f: float64(0)},
				// Chunk 1 Head
				sample{t: minutes(25), f: float64(1)},
				sample{t: minutes(35), f: float64(1)},
			},
			samplesAfterSeriesCall: chunks.SampleSlice{
				sample{t: minutes(10), f: float64(1)},
				sample{t: minutes(32), f: float64(1)},
				sample{t: minutes(50), f: float64(1)},
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
					sample{t: minutes(20), f: float64(0)},
					sample{t: minutes(22), f: float64(0)},
					sample{t: minutes(24), f: float64(0)},
					sample{t: minutes(25), f: float64(1)},
					sample{t: minutes(26), f: float64(0)},
					sample{t: minutes(30), f: float64(0)},
					sample{t: minutes(32), f: float64(1)}, // This sample was added after Series() but before Chunk() and its in between the lastmint and maxt so it should be kept
					sample{t: minutes(35), f: float64(1)},
				},
			},
		},
		{
			name:                 "After Series() previous head gets mmapped after getting samples, new head gets new samples also overlapping, none of these should appear in the response.",
			queryMinT:            minutes(0),
			queryMaxT:            minutes(100),
			firstInOrderSampleAt: minutes(120),
			initialSamples: chunks.SampleSlice{
				// Chunk 0
				sample{t: minutes(20), f: float64(0)},
				sample{t: minutes(22), f: float64(0)},
				sample{t: minutes(24), f: float64(0)},
				sample{t: minutes(26), f: float64(0)},
				sample{t: minutes(30), f: float64(0)},
				// Chunk 1 Head
				sample{t: minutes(25), f: float64(1)},
				sample{t: minutes(35), f: float64(1)},
			},
			samplesAfterSeriesCall: chunks.SampleSlice{
				sample{t: minutes(10), f: float64(1)},
				sample{t: minutes(32), f: float64(1)},
				sample{t: minutes(50), f: float64(1)},
				// Chunk 1 gets mmapped and Chunk 2, the new head is born
				sample{t: minutes(25), f: float64(2)},
				sample{t: minutes(31), f: float64(2)},
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
					sample{t: minutes(20), f: float64(0)},
					sample{t: minutes(22), f: float64(0)},
					sample{t: minutes(24), f: float64(0)},
					sample{t: minutes(25), f: float64(1)},
					sample{t: minutes(26), f: float64(0)},
					sample{t: minutes(30), f: float64(0)},
					sample{t: minutes(32), f: float64(1)}, // This sample was added after Series() but before Chunk() and its in between the lastmint and maxt so it should be kept
					sample{t: minutes(35), f: float64(1)},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			db := newTestDBWithOpts(t, opts)

			app := db.Appender(context.Background())
			s1Ref := appendSample(app, s1, tc.firstInOrderSampleAt, float64(tc.firstInOrderSampleAt/1*time.Minute.Milliseconds()))
			require.NoError(t, app.Commit())

			// OOO few samples for s1.
			app = db.Appender(context.Background())
			for _, s := range tc.initialSamples {
				appendSample(app, s1, s.T(), s.F())
			}
			require.NoError(t, app.Commit())

			// The Series method is the one that populates the chunk meta OOO
			// markers like OOOLastRef. These are then used by the ChunkReader.
			ir := NewOOOHeadIndexReader(db.head, tc.queryMinT, tc.queryMaxT, 0)
			var chks []chunks.Meta
			var b labels.ScratchBuilder
			err := ir.Series(s1Ref, &b, &chks)
			require.NoError(t, err)
			require.Equal(t, len(tc.expChunksSamples), len(chks))

			// Now we keep receiving ooo samples
			// OOO few samples for s1.
			app = db.Appender(context.Background())
			for _, s := range tc.samplesAfterSeriesCall {
				appendSample(app, s1, s.T(), s.F())
			}
			require.NoError(t, app.Commit())

			cr := NewOOOHeadChunkReader(db.head, tc.queryMinT, tc.queryMaxT, nil)
			defer cr.Close()
			for i := 0; i < len(chks); i++ {
				c, iterable, err := cr.ChunkOrIterable(chks[i])
				require.NoError(t, err)
				require.Nil(t, c)

				var resultSamples chunks.SampleSlice
				it := iterable.Iterator(nil)
				for it.Next() == chunkenc.ValFloat {
					ts, v := it.At()
					resultSamples = append(resultSamples, sample{t: ts, f: v})
				}
				require.Equal(t, tc.expChunksSamples[i], resultSamples)
			}
		})
	}
}

// TestSortByMinTimeAndMinRef tests that the sort function for chunk metas does sort
// by chunk meta MinTime and in case of same references by the lower reference.
func TestSortByMinTimeAndMinRef(t *testing.T) {
	tests := []struct {
		name  string
		input []chunkMetaAndChunkDiskMapperRef
		exp   []chunkMetaAndChunkDiskMapperRef
	}{
		{
			name: "chunks are ordered by min time",
			input: []chunkMetaAndChunkDiskMapperRef{
				{
					meta: chunks.Meta{
						Ref:     0,
						MinTime: 0,
					},
					ref: chunks.ChunkDiskMapperRef(0),
				},
				{
					meta: chunks.Meta{
						Ref:     1,
						MinTime: 1,
					},
					ref: chunks.ChunkDiskMapperRef(1),
				},
			},
			exp: []chunkMetaAndChunkDiskMapperRef{
				{
					meta: chunks.Meta{
						Ref:     0,
						MinTime: 0,
					},
					ref: chunks.ChunkDiskMapperRef(0),
				},
				{
					meta: chunks.Meta{
						Ref:     1,
						MinTime: 1,
					},
					ref: chunks.ChunkDiskMapperRef(1),
				},
			},
		},
		{
			name: "if same mintime, lower reference goes first",
			input: []chunkMetaAndChunkDiskMapperRef{
				{
					meta: chunks.Meta{
						Ref:     10,
						MinTime: 0,
					},
					ref: chunks.ChunkDiskMapperRef(0),
				},
				{
					meta: chunks.Meta{
						Ref:     5,
						MinTime: 0,
					},
					ref: chunks.ChunkDiskMapperRef(1),
				},
			},
			exp: []chunkMetaAndChunkDiskMapperRef{
				{
					meta: chunks.Meta{
						Ref:     5,
						MinTime: 0,
					},
					ref: chunks.ChunkDiskMapperRef(1),
				},
				{
					meta: chunks.Meta{
						Ref:     10,
						MinTime: 0,
					},
					ref: chunks.ChunkDiskMapperRef(0),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			slices.SortFunc(tc.input, refLessByMinTimeAndMinRef)
			require.Equal(t, tc.exp, tc.input)
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
