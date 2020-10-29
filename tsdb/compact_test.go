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
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

func TestSplitByRange(t *testing.T) {
	cases := []struct {
		trange int64
		ranges [][2]int64
		output [][][2]int64
	}{
		{
			trange: 60,
			ranges: [][2]int64{{0, 10}},
			output: [][][2]int64{
				{{0, 10}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{0, 60}},
			output: [][][2]int64{
				{{0, 60}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{0, 10}, {9, 15}, {30, 60}},
			output: [][][2]int64{
				{{0, 10}, {9, 15}, {30, 60}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{70, 90}, {125, 130}, {130, 180}, {1000, 1001}},
			output: [][][2]int64{
				{{70, 90}},
				{{125, 130}, {130, 180}},
				{{1000, 1001}},
			},
		},
		// Mis-aligned or too-large blocks are ignored.
		{
			trange: 60,
			ranges: [][2]int64{{50, 70}, {70, 80}},
			output: [][][2]int64{
				{{70, 80}},
			},
		},
		{
			trange: 72,
			ranges: [][2]int64{{0, 144}, {144, 216}, {216, 288}},
			output: [][][2]int64{
				{{144, 216}},
				{{216, 288}},
			},
		},
		// Various awkward edge cases easy to hit with negative numbers.
		{
			trange: 60,
			ranges: [][2]int64{{-10, -5}},
			output: [][][2]int64{
				{{-10, -5}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{-60, -50}, {-10, -5}},
			output: [][][2]int64{
				{{-60, -50}, {-10, -5}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{-60, -50}, {-10, -5}, {0, 15}},
			output: [][][2]int64{
				{{-60, -50}, {-10, -5}},
				{{0, 15}},
			},
		},
	}

	for _, c := range cases {
		// Transform input range tuples into dirMetas.
		blocks := make([]dirMeta, 0, len(c.ranges))
		for _, r := range c.ranges {
			blocks = append(blocks, dirMeta{
				meta: &BlockMeta{
					MinTime: r[0],
					MaxTime: r[1],
				},
			})
		}

		// Transform output range tuples into dirMetas.
		exp := make([][]dirMeta, len(c.output))
		for i, group := range c.output {
			for _, r := range group {
				exp[i] = append(exp[i], dirMeta{
					meta: &BlockMeta{MinTime: r[0], MaxTime: r[1]},
				})
			}
		}

		require.Equal(t, exp, splitByRange(blocks, c.trange))
	}
}

// See https://github.com/prometheus/prometheus/issues/3064
func TestNoPanicFor0Tombstones(t *testing.T) {
	metas := []dirMeta{
		{
			dir: "1",
			meta: &BlockMeta{
				MinTime: 0,
				MaxTime: 100,
			},
		},
		{
			dir: "2",
			meta: &BlockMeta{
				MinTime: 101,
				MaxTime: 200,
			},
		},
	}

	c, err := NewLeveledCompactor(context.Background(), nil, nil, []int64{50}, nil)
	require.NoError(t, err)

	c.plan(metas)
}

func TestLeveledCompactor_plan(t *testing.T) {
	// This mimics our default ExponentialBlockRanges with min block size equals to 20.
	compactor, err := NewLeveledCompactor(context.Background(), nil, nil, []int64{
		20,
		60,
		180,
		540,
		1620,
	}, nil)
	require.NoError(t, err)

	cases := map[string]struct {
		metas    []dirMeta
		expected []string
	}{
		"Outside Range": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
			},
			expected: nil,
		},
		"We should wait for four blocks of size 20 to appear before compacting.": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
			},
			expected: nil,
		},
		`We should wait for a next block of size 20 to appear before compacting
		the existing ones. We have three, but we ignore the fresh one from WAl`: {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
			},
			expected: nil,
		},
		"Block to fill the entire parent range appeared – should be compacted": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 80, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		`Block for the next parent range appeared with gap with size 20. Nothing will happen in the first one
		anymore but we ignore fresh one still, so no compaction`: {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
			},
			expected: nil,
		},
		`Block for the next parent range appeared, and we have a gap with size 20 between second and third block.
		We will not get this missed gap anymore and we should compact just these two.`: {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
				metaRange("4", 80, 100, nil),
			},
			expected: []string{"1", "2"},
		},
		"We have 20, 20, 20, 60, 60 range blocks. '5' is marked as fresh one": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 120, 180, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		"We have 20, 60, 20, 60, 240 range blocks. We can compact 20 + 60 + 60": {
			metas: []dirMeta{
				metaRange("2", 20, 40, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 960, 980, nil), // Fresh one.
				metaRange("6", 120, 180, nil),
				metaRange("7", 720, 960, nil),
			},
			expected: []string{"2", "4", "6"},
		},
		"Do not select large blocks that have many tombstones when there is no fresh block": {
			metas: []dirMeta{
				metaRange("1", 0, 540, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
			},
			expected: nil,
		},
		"Select large blocks that have many tombstones when fresh appears": {
			metas: []dirMeta{
				metaRange("1", 0, 540, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
				metaRange("2", 540, 560, nil),
			},
			expected: []string{"1"},
		},
		"For small blocks, do not compact tombstones, even when fresh appears.": {
			metas: []dirMeta{
				metaRange("1", 0, 60, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
				metaRange("2", 60, 80, nil),
			},
			expected: nil,
		},
		`Regression test: we were stuck in a compact loop where we always recompacted
		the same block when tombstones and series counts were zero`: {
			metas: []dirMeta{
				metaRange("1", 0, 540, &BlockStats{
					NumSeries:     0,
					NumTombstones: 0,
				}),
				metaRange("2", 540, 560, nil),
			},
			expected: nil,
		},
		`Regression test: we were wrongly assuming that new block is fresh from WAL when its ULID is newest.
		We need to actually look on max time instead.

		With previous, wrong approach "8" block was ignored, so we were wrongly compacting 5 and 7 and introducing
		block overlaps`: {
			metas: []dirMeta{
				metaRange("5", 0, 360, nil),
				metaRange("6", 540, 560, nil), // Fresh one.
				metaRange("7", 360, 420, nil),
				metaRange("8", 420, 540, nil),
			},
			expected: []string{"7", "8"},
		},
		// |--------------|
		//               |----------------|
		//                                |--------------|
		"Overlapping blocks 1": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 19, 40, nil),
				metaRange("3", 40, 60, nil),
			},
			expected: []string{"1", "2"},
		},
		// |--------------|
		//                |--------------|
		//                        |--------------|
		"Overlapping blocks 2": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 30, 50, nil),
			},
			expected: []string{"2", "3"},
		},
		// |--------------|
		//         |---------------------|
		//                       |--------------|
		"Overlapping blocks 3": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 10, 40, nil),
				metaRange("3", 30, 50, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		// |--------------|
		//               |--------------------------------|
		//                |--------------|
		//                               |--------------|
		"Overlapping blocks 4": {
			metas: []dirMeta{
				metaRange("5", 0, 360, nil),
				metaRange("6", 340, 560, nil),
				metaRange("7", 360, 420, nil),
				metaRange("8", 420, 540, nil),
			},
			expected: []string{"5", "6", "7", "8"},
		},
		// |--------------|
		//               |--------------|
		//                                            |--------------|
		//                                                          |--------------|
		"Overlapping blocks 5": {
			metas: []dirMeta{
				metaRange("1", 0, 10, nil),
				metaRange("2", 9, 20, nil),
				metaRange("3", 30, 40, nil),
				metaRange("4", 39, 50, nil),
			},
			expected: []string{"1", "2"},
		},
	}

	for title, c := range cases {
		if !t.Run(title, func(t *testing.T) {
			res, err := compactor.plan(c.metas)
			require.NoError(t, err)
			require.Equal(t, c.expected, res)
		}) {
			return
		}
	}
}

func TestRangeWithFailedCompactionWontGetSelected(t *testing.T) {
	compactor, err := NewLeveledCompactor(context.Background(), nil, nil, []int64{
		20,
		60,
		240,
		720,
		2160,
	}, nil)
	require.NoError(t, err)

	cases := []struct {
		metas []dirMeta
	}{
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 80, nil),
			},
		},
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
				metaRange("4", 80, 100, nil),
			},
		},
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 120, 180, nil),
				metaRange("6", 180, 200, nil),
			},
		},
	}

	for _, c := range cases {
		c.metas[1].meta.Compaction.Failed = true
		res, err := compactor.plan(c.metas)
		require.NoError(t, err)

		require.Equal(t, []string(nil), res)
	}
}

func TestCompactionFailWillCleanUpTempDir(t *testing.T) {
	compactor, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{
		20,
		60,
		240,
		720,
		2160,
	}, nil)
	require.NoError(t, err)

	tmpdir, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()

	require.Error(t, compactor.write(tmpdir, &BlockMeta{}, erringBReader{}))
	_, err = os.Stat(filepath.Join(tmpdir, BlockMeta{}.ULID.String()) + tmpForCreationBlockDirSuffix)
	require.True(t, os.IsNotExist(err), "directory is not cleaned up")
}

func metaRange(name string, mint, maxt int64, stats *BlockStats) dirMeta {
	meta := &BlockMeta{MinTime: mint, MaxTime: maxt}
	if stats != nil {
		meta.Stats = *stats
	}
	return dirMeta{
		dir:  name,
		meta: meta,
	}
}

type erringBReader struct{}

func (erringBReader) Index() (IndexReader, error)            { return nil, errors.New("index") }
func (erringBReader) Chunks() (ChunkReader, error)           { return nil, errors.New("chunks") }
func (erringBReader) Tombstones() (tombstones.Reader, error) { return nil, errors.New("tombstones") }
func (erringBReader) Meta() BlockMeta                        { return BlockMeta{} }
func (erringBReader) Size() int64                            { return 0 }

type nopChunkWriter struct{}

func (nopChunkWriter) WriteChunks(chunks ...chunks.Meta) error { return nil }
func (nopChunkWriter) Close() error                            { return nil }

func samplesForRange(minTime, maxTime int64, maxSamplesPerChunk int) (ret [][]sample) {
	var curr []sample
	for i := minTime; i <= maxTime; i++ {
		curr = append(curr, sample{t: i})
		if len(curr) >= maxSamplesPerChunk {
			ret = append(ret, curr)
			curr = []sample{}
		}
	}
	if len(curr) > 0 {
		ret = append(ret, curr)
	}
	return ret
}

func TestCompaction_populateBlock(t *testing.T) {
	for _, tc := range []struct {
		title              string
		inputSeriesSamples [][]seriesSamples
		compactMinTime     int64
		compactMaxTime     int64 // When not defined the test runner sets a default of math.MaxInt64.
		expSeriesSamples   []seriesSamples
		expErr             error
	}{
		{
			title:              "Populate block from empty input should return error.",
			inputSeriesSamples: [][]seriesSamples{},
			expErr:             errors.New("cannot populate block from no readers"),
		},
		{
			// Populate from single block without chunks. We expect these kind of series being ignored.
			inputSeriesSamples: [][]seriesSamples{
				{{lset: map[string]string{"a": "b"}}},
			},
		},
		{
			title: "Populate from single block. We expect the same samples at the output.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}},
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "b"},
					chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}},
				},
			},
		},
		{
			title: "Populate from two blocks.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}},
					},
					{
						lset:   map[string]string{"a": "c"},
						chunks: [][]sample{{{t: 1}, {t: 9}}, {{t: 10}, {t: 19}}},
					},
					{
						// no-chunk series should be dropped.
						lset: map[string]string{"a": "empty"},
					},
				},
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 21}, {t: 30}}},
					},
					{
						lset:   map[string]string{"a": "c"},
						chunks: [][]sample{{{t: 40}, {t: 45}}},
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "b"},
					chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}, {{t: 21}, {t: 30}}},
				},
				{
					lset:   map[string]string{"a": "c"},
					chunks: [][]sample{{{t: 1}, {t: 9}}, {{t: 10}, {t: 19}}, {{t: 40}, {t: 45}}},
				},
			},
		},
		{
			title: "Populate from two blocks; chunks with negative time.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}},
					},
					{
						lset:   map[string]string{"a": "c"},
						chunks: [][]sample{{{t: -11}, {t: -9}}, {{t: 10}, {t: 19}}},
					},
					{
						// no-chunk series should be dropped.
						lset: map[string]string{"a": "empty"},
					},
				},
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 21}, {t: 30}}},
					},
					{
						lset:   map[string]string{"a": "c"},
						chunks: [][]sample{{{t: 40}, {t: 45}}},
					},
				},
			},
			compactMinTime: -11,
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "b"},
					chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}, {{t: 21}, {t: 30}}},
				},
				{
					lset:   map[string]string{"a": "c"},
					chunks: [][]sample{{{t: -11}, {t: -9}}, {{t: 10}, {t: 19}}, {{t: 40}, {t: 45}}},
				},
			},
		},
		{
			title: "Populate from two blocks showing that order is maintained.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}},
					},
					{
						lset:   map[string]string{"a": "c"},
						chunks: [][]sample{{{t: 1}, {t: 9}}, {{t: 10}, {t: 19}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 21}, {t: 30}}},
					},
					{
						lset:   map[string]string{"a": "c"},
						chunks: [][]sample{{{t: 40}, {t: 45}}},
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "b"},
					chunks: [][]sample{{{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}, {{t: 21}, {t: 30}}},
				},
				{
					lset:   map[string]string{"a": "c"},
					chunks: [][]sample{{{t: 1}, {t: 9}}, {{t: 10}, {t: 19}}, {{t: 40}, {t: 45}}},
				},
			},
		},
		{
			title: "Populate from two blocks showing that order of series is sorted.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "4"},
						chunks: [][]sample{{{t: 5}, {t: 7}}},
					},
					{
						lset:   map[string]string{"a": "3"},
						chunks: [][]sample{{{t: 5}, {t: 6}}},
					},
					{
						lset:   map[string]string{"a": "same"},
						chunks: [][]sample{{{t: 1}, {t: 4}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "2"},
						chunks: [][]sample{{{t: 1}, {t: 3}}},
					},
					{
						lset:   map[string]string{"a": "1"},
						chunks: [][]sample{{{t: 1}, {t: 2}}},
					},
					{
						lset:   map[string]string{"a": "same"},
						chunks: [][]sample{{{t: 5}, {t: 8}}},
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "1"},
					chunks: [][]sample{{{t: 1}, {t: 2}}},
				},
				{
					lset:   map[string]string{"a": "2"},
					chunks: [][]sample{{{t: 1}, {t: 3}}},
				},
				{
					lset:   map[string]string{"a": "3"},
					chunks: [][]sample{{{t: 5}, {t: 6}}},
				},
				{
					lset:   map[string]string{"a": "4"},
					chunks: [][]sample{{{t: 5}, {t: 7}}},
				},
				{
					lset:   map[string]string{"a": "same"},
					chunks: [][]sample{{{t: 1}, {t: 4}}, {{t: 5}, {t: 8}}},
				},
			},
		},
		{
			title: "Populate from two blocks 1:1 duplicated chunks; with negative timestamps.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "1"},
						chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 3}, {t: 4}}},
					},
					{
						lset:   map[string]string{"a": "2"},
						chunks: [][]sample{{{t: -3}, {t: -2}}, {{t: 1}, {t: 3}, {t: 4}}, {{t: 5}, {t: 6}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "1"},
						chunks: [][]sample{{{t: 3}, {t: 4}}},
					},
					{
						lset:   map[string]string{"a": "2"},
						chunks: [][]sample{{{t: 1}, {t: 3}, {t: 4}}, {{t: 7}, {t: 8}}},
					},
				},
			},
			compactMinTime: -3,
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "1"},
					chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 3}, {t: 4}}},
				},
				{
					lset:   map[string]string{"a": "2"},
					chunks: [][]sample{{{t: -3}, {t: -2}}, {{t: 1}, {t: 3}, {t: 4}}, {{t: 5}, {t: 6}}, {{t: 7}, {t: 8}}},
				},
			},
		},
		{
			// This should not happened because head block is making sure the chunks are not crossing block boundaries.
			// We used to return error, but now chunk is trimmed.
			title: "Populate from single block containing chunk outside of compact meta time range.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 10}, {t: 30}}},
					},
				},
			},
			compactMinTime: 0,
			compactMaxTime: 20,
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "b"},
					chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 10}}},
				},
			},
		},
		{
			// Introduced by https://github.com/prometheus/tsdb/issues/347. We used to return error, but now chunk is trimmed.
			title: "Populate from single block containing extra chunk",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "issue347"},
						chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 10}, {t: 20}}},
					},
				},
			},
			compactMinTime: 0,
			compactMaxTime: 10,
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "issue347"},
					chunks: [][]sample{{{t: 1}, {t: 2}}},
				},
			},
		},
		{
			// Deduplication expected.
			// Introduced by pull/370 and pull/539.
			title: "Populate from two blocks containing duplicated chunk.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 10}, {t: 20}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "b"},
						chunks: [][]sample{{{t: 10}, {t: 20}}},
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "b"},
					chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 10}, {t: 20}}},
				},
			},
		},
		{
			// Introduced by https://github.com/prometheus/tsdb/pull/539.
			title: "Populate from three overlapping blocks.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "overlap-all"},
						chunks: [][]sample{{{t: 19}, {t: 30}}},
					},
					{
						lset:   map[string]string{"a": "overlap-beginning"},
						chunks: [][]sample{{{t: 0}, {t: 5}}},
					},
					{
						lset:   map[string]string{"a": "overlap-ending"},
						chunks: [][]sample{{{t: 21}, {t: 30}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "overlap-all"},
						chunks: [][]sample{{{t: 0}, {t: 10}, {t: 11}, {t: 20}}},
					},
					{
						lset:   map[string]string{"a": "overlap-beginning"},
						chunks: [][]sample{{{t: 0}, {t: 10}, {t: 12}, {t: 20}}},
					},
					{
						lset:   map[string]string{"a": "overlap-ending"},
						chunks: [][]sample{{{t: 0}, {t: 10}, {t: 13}, {t: 20}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "overlap-all"},
						chunks: [][]sample{{{t: 27}, {t: 35}}},
					},
					{
						lset:   map[string]string{"a": "overlap-ending"},
						chunks: [][]sample{{{t: 27}, {t: 35}}},
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "overlap-all"},
					chunks: [][]sample{{{t: 0}, {t: 10}, {t: 11}, {t: 19}, {t: 20}, {t: 27}, {t: 30}, {t: 35}}},
				},
				{
					lset:   map[string]string{"a": "overlap-beginning"},
					chunks: [][]sample{{{t: 0}, {t: 5}, {t: 10}, {t: 12}, {t: 20}}},
				},
				{
					lset:   map[string]string{"a": "overlap-ending"},
					chunks: [][]sample{{{t: 0}, {t: 10}, {t: 13}, {t: 20}}, {{t: 21}, {t: 27}, {t: 30}, {t: 35}}},
				},
			},
		},
		{
			title: "Populate from three partially overlapping blocks with few full chunks.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "1", "b": "1"},
						chunks: samplesForRange(0, 659, 120), // 5 chunks and half.
					},
					{
						lset:   map[string]string{"a": "1", "b": "2"},
						chunks: samplesForRange(0, 659, 120),
					},
				},
				{
					{
						lset:   map[string]string{"a": "1", "b": "2"},
						chunks: samplesForRange(480, 1199, 120), // two chunks overlapping with previous, two non overlapping and two overlapping with next block.
					},
					{
						lset:   map[string]string{"a": "1", "b": "3"},
						chunks: samplesForRange(480, 1199, 120),
					},
				},
				{
					{
						lset:   map[string]string{"a": "1", "b": "2"},
						chunks: samplesForRange(960, 1499, 120), // 5 chunks and half.
					},
					{
						lset:   map[string]string{"a": "1", "b": "4"},
						chunks: samplesForRange(960, 1499, 120),
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "1", "b": "1"},
					chunks: samplesForRange(0, 659, 120),
				},
				{
					lset:   map[string]string{"a": "1", "b": "2"},
					chunks: samplesForRange(0, 1499, 120),
				},
				{
					lset:   map[string]string{"a": "1", "b": "3"},
					chunks: samplesForRange(480, 1199, 120),
				},
				{
					lset:   map[string]string{"a": "1", "b": "4"},
					chunks: samplesForRange(960, 1499, 120),
				},
			},
		},
		{
			title: "Populate from three partially overlapping blocks with chunks that are expected to merge into single big chunks.",
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset:   map[string]string{"a": "1", "b": "2"},
						chunks: [][]sample{{{t: 0}, {t: 6902464}}, {{t: 6961968}, {t: 7080976}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "1", "b": "2"},
						chunks: [][]sample{{{t: 3600000}, {t: 13953696}}, {{t: 14042952}, {t: 14221464}}},
					},
				},
				{
					{
						lset:   map[string]string{"a": "1", "b": "2"},
						chunks: [][]sample{{{t: 10800000}, {t: 14251232}}, {{t: 14280984}, {t: 14340488}}},
					},
				},
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "1", "b": "2"},
					chunks: [][]sample{{{t: 0}, {t: 3600000}, {t: 6902464}, {t: 6961968}, {t: 7080976}, {t: 10800000}, {t: 13953696}, {t: 14042952}, {t: 14221464}, {t: 14251232}}, {{t: 14280984}, {t: 14340488}}},
				},
			},
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			blocks := make([]BlockReader, 0, len(tc.inputSeriesSamples))
			for _, b := range tc.inputSeriesSamples {
				ir, cr, mint, maxt := createIdxChkReaders(t, b)
				blocks = append(blocks, &mockBReader{ir: ir, cr: cr, mint: mint, maxt: maxt})
			}

			c, err := NewLeveledCompactor(context.Background(), nil, nil, []int64{0}, nil)
			require.NoError(t, err)

			meta := &BlockMeta{
				MinTime: tc.compactMinTime,
				MaxTime: tc.compactMaxTime,
			}
			if meta.MaxTime == 0 {
				meta.MaxTime = math.MaxInt64
			}

			iw := &mockIndexWriter{}
			err = c.populateBlock(blocks, meta, iw, nopChunkWriter{})
			if tc.expErr != nil {
				require.Error(t, err)
				require.Equal(t, tc.expErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)

			// Check if response is expected and chunk is valid.
			var raw []seriesSamples
			for _, s := range iw.seriesChunks {
				ss := seriesSamples{lset: s.l.Map()}
				var iter chunkenc.Iterator
				for _, chk := range s.chunks {
					var (
						samples       = make([]sample, 0, chk.Chunk.NumSamples())
						iter          = chk.Chunk.Iterator(iter)
						firstTs int64 = math.MaxInt64
						s       sample
					)
					for iter.Next() {
						s.t, s.v = iter.At()
						if firstTs == math.MaxInt64 {
							firstTs = s.t
						}
						samples = append(samples, s)
					}

					// Check if chunk has correct min, max times.
					require.Equal(t, firstTs, chk.MinTime, "chunk Meta %v does not match the first encoded sample timestamp: %v", chk, firstTs)
					require.Equal(t, s.t, chk.MaxTime, "chunk Meta %v does not match the last encoded sample timestamp %v", chk, s.t)

					require.NoError(t, iter.Err())
					ss.chunks = append(ss.chunks, samples)
				}
				raw = append(raw, ss)
			}
			require.Equal(t, tc.expSeriesSamples, raw)

			// Check if stats are calculated properly.
			s := BlockStats{NumSeries: uint64(len(tc.expSeriesSamples))}
			for _, series := range tc.expSeriesSamples {
				s.NumChunks += uint64(len(series.chunks))
				for _, chk := range series.chunks {
					s.NumSamples += uint64(len(chk))
				}
			}
			require.Equal(t, s, meta.Stats)
		})
	}
}

func BenchmarkCompaction(b *testing.B) {
	cases := []struct {
		ranges         [][2]int64
		compactionType string
	}{
		{
			ranges:         [][2]int64{{0, 100}, {200, 300}, {400, 500}, {600, 700}},
			compactionType: "normal",
		},
		{
			ranges:         [][2]int64{{0, 1000}, {2000, 3000}, {4000, 5000}, {6000, 7000}},
			compactionType: "normal",
		},
		{
			ranges:         [][2]int64{{0, 2000}, {3000, 5000}, {6000, 8000}, {9000, 11000}},
			compactionType: "normal",
		},
		{
			ranges:         [][2]int64{{0, 5000}, {6000, 11000}, {12000, 17000}, {18000, 23000}},
			compactionType: "normal",
		},
		// 40% overlaps.
		{
			ranges:         [][2]int64{{0, 100}, {60, 160}, {120, 220}, {180, 280}},
			compactionType: "vertical",
		},
		{
			ranges:         [][2]int64{{0, 1000}, {600, 1600}, {1200, 2200}, {1800, 2800}},
			compactionType: "vertical",
		},
		{
			ranges:         [][2]int64{{0, 2000}, {1200, 3200}, {2400, 4400}, {3600, 5600}},
			compactionType: "vertical",
		},
		{
			ranges:         [][2]int64{{0, 5000}, {3000, 8000}, {6000, 11000}, {9000, 14000}},
			compactionType: "vertical",
		},
	}

	nSeries := 10000
	for _, c := range cases {
		nBlocks := len(c.ranges)
		b.Run(fmt.Sprintf("type=%s,blocks=%d,series=%d,samplesPerSeriesPerBlock=%d", c.compactionType, nBlocks, nSeries, c.ranges[0][1]-c.ranges[0][0]+1), func(b *testing.B) {
			dir, err := ioutil.TempDir("", "bench_compaction")
			require.NoError(b, err)
			defer func() {
				require.NoError(b, os.RemoveAll(dir))
			}()
			blockDirs := make([]string, 0, len(c.ranges))
			var blocks []*Block
			for _, r := range c.ranges {
				block, err := OpenBlock(nil, createBlock(b, dir, genSeries(nSeries, 10, r[0], r[1])), nil)
				require.NoError(b, err)
				blocks = append(blocks, block)
				defer func() {
					require.NoError(b, block.Close())
				}()
				blockDirs = append(blockDirs, block.Dir())
			}

			c, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{0}, nil)
			require.NoError(b, err)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err = c.Compact(dir, blockDirs, blocks)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkCompactionFromHead(b *testing.B) {
	dir, err := ioutil.TempDir("", "bench_compaction_from_head")
	require.NoError(b, err)
	defer func() {
		require.NoError(b, os.RemoveAll(dir))
	}()
	totalSeries := 100000
	for labelNames := 1; labelNames < totalSeries; labelNames *= 10 {
		labelValues := totalSeries / labelNames
		b.Run(fmt.Sprintf("labelnames=%d,labelvalues=%d", labelNames, labelValues), func(b *testing.B) {
			chunkDir, err := ioutil.TempDir("", "chunk_dir")
			require.NoError(b, err)
			defer func() {
				require.NoError(b, os.RemoveAll(chunkDir))
			}()
			h, err := NewHead(nil, nil, nil, 1000, chunkDir, nil, DefaultStripeSize, nil)
			require.NoError(b, err)
			for ln := 0; ln < labelNames; ln++ {
				app := h.Appender(context.Background())
				for lv := 0; lv < labelValues; lv++ {
					app.Add(labels.FromStrings(fmt.Sprintf("%d", ln), fmt.Sprintf("%d%s%d", lv, postingsBenchSuffix, ln)), 0, 0)
				}
				require.NoError(b, app.Commit())
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				createBlockFromHead(b, filepath.Join(dir, fmt.Sprintf("%d-%d", i, labelNames)), h)
			}
			h.Close()
		})
	}
}

// TestDisableAutoCompactions checks that we can
// disable and enable the auto compaction.
// This is needed for unit tests that rely on
// checking state before and after a compaction.
func TestDisableAutoCompactions(t *testing.T) {
	db := openTestDB(t, nil, nil)
	defer func() {
		require.NoError(t, db.Close())
	}()

	blockRange := db.compactor.(*LeveledCompactor).ranges[0]
	label := labels.FromStrings("foo", "bar")

	// Trigger a compaction to check that it was skipped and
	// no new blocks were created when compaction is disabled.
	db.DisableCompactions()
	app := db.Appender(context.Background())
	for i := int64(0); i < 3; i++ {
		_, err := app.Add(label, i*blockRange, 0)
		require.NoError(t, err)
		_, err = app.Add(label, i*blockRange+1000, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	select {
	case db.compactc <- struct{}{}:
	default:
	}

	for x := 0; x < 10; x++ {
		if prom_testutil.ToFloat64(db.metrics.compactionsSkipped) > 0.0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	require.Greater(t, prom_testutil.ToFloat64(db.metrics.compactionsSkipped), 0.0, "No compaction was skipped after the set timeout.")
	require.Equal(t, 0, len(db.blocks))

	// Enable the compaction, trigger it and check that the block is persisted.
	db.EnableCompactions()
	select {
	case db.compactc <- struct{}{}:
	default:
	}
	for x := 0; x < 100; x++ {
		if len(db.Blocks()) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Greater(t, len(db.Blocks()), 0, "No block was persisted after the set timeout.")
}

// TestCancelCompactions ensures that when the db is closed
// any running compaction is cancelled to unblock closing the db.
func TestCancelCompactions(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "testCancelCompaction")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()

	// Create some blocks to fall within the compaction range.
	createBlock(t, tmpdir, genSeries(1, 10000, 0, 1000))
	createBlock(t, tmpdir, genSeries(1, 10000, 1000, 2000))
	createBlock(t, tmpdir, genSeries(1, 1, 2000, 2001)) // The most recent block is ignored so can be e small one.

	// Copy the db so we have an exact copy to compare compaction times.
	tmpdirCopy := tmpdir + "Copy"
	err = fileutil.CopyDirs(tmpdir, tmpdirCopy)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdirCopy))
	}()

	// Measure the compaction time without interrupting it.
	var timeCompactionUninterrupted time.Duration
	{
		db, err := open(tmpdir, log.NewNopLogger(), nil, DefaultOptions(), []int64{1, 2000})
		require.NoError(t, err)
		require.Equal(t, 3, len(db.Blocks()), "initial block count mismatch")
		require.Equal(t, 0.0, prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran), "initial compaction counter mismatch")
		db.compactc <- struct{}{} // Trigger a compaction.
		var start time.Time
		for prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.populatingBlocks) <= 0 {
			time.Sleep(3 * time.Millisecond)
		}
		start = time.Now()

		for prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran) != 1 {
			time.Sleep(3 * time.Millisecond)
		}
		timeCompactionUninterrupted = time.Since(start)

		require.NoError(t, db.Close())
	}
	// Measure the compaction time when closing the db in the middle of compaction.
	{
		db, err := open(tmpdirCopy, log.NewNopLogger(), nil, DefaultOptions(), []int64{1, 2000})
		require.NoError(t, err)
		require.Equal(t, 3, len(db.Blocks()), "initial block count mismatch")
		require.Equal(t, 0.0, prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran), "initial compaction counter mismatch")
		db.compactc <- struct{}{} // Trigger a compaction.
		dbClosed := make(chan struct{})

		for prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.populatingBlocks) <= 0 {
			time.Sleep(3 * time.Millisecond)
		}
		go func() {
			require.NoError(t, db.Close())
			close(dbClosed)
		}()

		start := time.Now()
		<-dbClosed
		actT := time.Since(start)
		expT := time.Duration(timeCompactionUninterrupted / 2) // Closing the db in the middle of compaction should less than half the time.
		require.True(t, actT < expT, "closing the db took more than expected. exp: <%v, act: %v", expT, actT)
	}
}

// TestDeleteCompactionBlockAfterFailedReload ensures that a failed reloadBlocks immediately after a compaction
// deletes the resulting block to avoid creatings blocks with the same time range.
func TestDeleteCompactionBlockAfterFailedReload(t *testing.T) {
	tests := map[string]func(*DB) int{
		"Test Head Compaction": func(db *DB) int {
			rangeToTriggerCompaction := db.compactor.(*LeveledCompactor).ranges[0]/2*3 - 1
			defaultLabel := labels.FromStrings("foo", "bar")

			// Add some data to the head that is enough to trigger a compaction.
			app := db.Appender(context.Background())
			_, err := app.Add(defaultLabel, 1, 0)
			require.NoError(t, err)
			_, err = app.Add(defaultLabel, 2, 0)
			require.NoError(t, err)
			_, err = app.Add(defaultLabel, 3+rangeToTriggerCompaction, 0)
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			return 0
		},
		"Test Block Compaction": func(db *DB) int {
			blocks := []*BlockMeta{
				{MinTime: 0, MaxTime: 100},
				{MinTime: 100, MaxTime: 150},
				{MinTime: 150, MaxTime: 200},
			}
			for _, m := range blocks {
				createBlock(t, db.Dir(), genSeries(1, 1, m.MinTime, m.MaxTime))
			}
			require.NoError(t, db.reload())
			require.Equal(t, len(blocks), len(db.Blocks()), "unexpected block count after a reloadBlocks")

			return len(blocks)
		},
	}

	for title, bootStrap := range tests {
		t.Run(title, func(t *testing.T) {
			db := openTestDB(t, nil, []int64{1, 100})
			defer func() {
				require.NoError(t, db.Close())
			}()
			db.DisableCompactions()

			expBlocks := bootStrap(db)

			// Create a block that will trigger the reloadBlocks to fail.
			blockPath := createBlock(t, db.Dir(), genSeries(1, 1, 200, 300))
			lastBlockIndex := path.Join(blockPath, indexFilename)
			actBlocks, err := blockDirs(db.Dir())
			require.NoError(t, err)
			require.Equal(t, expBlocks, len(actBlocks)-1)    // -1 to exclude the corrupted block.
			require.NoError(t, os.RemoveAll(lastBlockIndex)) // Corrupt the block by removing the index file.

			require.Equal(t, 0.0, prom_testutil.ToFloat64(db.metrics.reloadsFailed), "initial 'failed db reloadBlocks' count metrics mismatch")
			require.Equal(t, 0.0, prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran), "initial `compactions` count metric mismatch")
			require.Equal(t, 0.0, prom_testutil.ToFloat64(db.metrics.compactionsFailed), "initial `compactions failed` count metric mismatch")

			// Do the compaction and check the metrics.
			// Compaction should succeed, but the reloadBlocks should fail and
			// the new block created from the compaction should be deleted.
			require.Error(t, db.Compact())
			require.Equal(t, 1.0, prom_testutil.ToFloat64(db.metrics.reloadsFailed), "'failed db reloadBlocks' count metrics mismatch")
			require.Equal(t, 1.0, prom_testutil.ToFloat64(db.compactor.(*LeveledCompactor).metrics.ran), "`compaction` count metric mismatch")
			require.Equal(t, 1.0, prom_testutil.ToFloat64(db.metrics.compactionsFailed), "`compactions failed` count metric mismatch")

			actBlocks, err = blockDirs(db.Dir())
			require.NoError(t, err)
			require.Equal(t, expBlocks, len(actBlocks)-1, "block count should be the same as before the compaction") // -1 to exclude the corrupted block.
		})
	}
}
