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
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
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

		testutil.Equals(t, exp, splitByRange(blocks, c.trange))
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

	c, err := NewLeveledCompactor(nil, nil, []int64{50}, nil)
	testutil.Ok(t, err)

	c.plan(metas)
}

func TestLeveledCompactor_plan(t *testing.T) {
	// This mimicks our default ExponentialBlockRanges with min block size equals to 20.
	compactor, err := NewLeveledCompactor(nil, nil, []int64{
		20,
		60,
		180,
		540,
		1620,
	}, nil)
	testutil.Ok(t, err)

	cases := []struct {
		metas    []dirMeta
		expected []string
	}{
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
			},
			expected: nil,
		},
		// We should wait for four blocks of size 20 to appear before compacting.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
			},
			expected: nil,
		},
		// We should wait for a next block of size 20 to appear before compacting
		// the existing ones. We have three, but we ignore the fresh one from WAl.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
			},
			expected: nil,
		},
		// Block to fill the entire parent range appeared – should be compacted.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 80, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		// Block for the next parent range appeared with gap with size 20. Nothing will happen in the first one
		// anymore but we ignore fresh one still, so no compaction.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
			},
			expected: nil,
		},
		// Block for the next parent range appeared, and we have a gap with size 20 between second and third block.
		// We will not get this missed gap anymore and we should compact just these two.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
				metaRange("4", 80, 100, nil),
			},
			expected: []string{"1", "2"},
		},
		{
			// We have 20, 20, 20, 60, 60 range blocks. "5" is marked as fresh one.
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 120, 180, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		{
			// We have 20, 60, 20, 60, 240 range blocks. We can compact 20 + 60 + 60.
			metas: []dirMeta{
				metaRange("2", 20, 40, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 960, 980, nil), // Fresh one.
				metaRange("6", 120, 180, nil),
				metaRange("7", 720, 960, nil),
			},
			expected: []string{"2", "4", "6"},
		},
		// Do not select large blocks that have many tombstones when there is no fresh block.
		{
			metas: []dirMeta{
				metaRange("1", 0, 540, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
			},
			expected: nil,
		},
		// Select large blocks that have many tombstones when fresh appears.
		{
			metas: []dirMeta{
				metaRange("1", 0, 540, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
				metaRange("2", 540, 560, nil),
			},
			expected: []string{"1"},
		},
		// For small blocks, do not compact tombstones, even when fresh appears.
		{
			metas: []dirMeta{
				metaRange("1", 0, 60, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
				metaRange("2", 60, 80, nil),
			},
			expected: nil,
		},
		// Regression test: we were stuck in a compact loop where we always recompacted
		// the same block when tombstones and series counts were zero.
		{
			metas: []dirMeta{
				metaRange("1", 0, 540, &BlockStats{
					NumSeries:     0,
					NumTombstones: 0,
				}),
				metaRange("2", 540, 560, nil),
			},
			expected: nil,
		},
		// Regression test: we were wrongly assuming that new block is fresh from WAL when its ULID is newest.
		// We need to actually look on max time instead.
		//
		// With previous, wrong approach "8" block was ignored, so we were wrongly compacting 5 and 7 and introducing
		// block overlaps.
		{
			metas: []dirMeta{
				metaRange("5", 0, 360, nil),
				metaRange("6", 540, 560, nil), // Fresh one.
				metaRange("7", 360, 420, nil),
				metaRange("8", 420, 540, nil),
			},
			expected: []string{"7", "8"},
		},
	}

	for _, c := range cases {
		if !t.Run("", func(t *testing.T) {
			res, err := compactor.plan(c.metas)
			testutil.Ok(t, err)

			testutil.Equals(t, c.expected, res)
		}) {
			return
		}
	}
}

func TestRangeWithFailedCompactionWontGetSelected(t *testing.T) {
	compactor, err := NewLeveledCompactor(nil, nil, []int64{
		20,
		60,
		240,
		720,
		2160,
	}, nil)
	testutil.Ok(t, err)

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
		testutil.Ok(t, err)

		testutil.Equals(t, []string(nil), res)
	}
}

func TestCompactionFailWillCleanUpTempDir(t *testing.T) {
	compactor, err := NewLeveledCompactor(nil, log.NewNopLogger(), []int64{
		20,
		60,
		240,
		720,
		2160,
	}, nil)
	testutil.Ok(t, err)

	tmpdir, err := ioutil.TempDir("", "test")
	testutil.Ok(t, err)
	defer os.RemoveAll(tmpdir)

	testutil.NotOk(t, compactor.write(tmpdir, &BlockMeta{}, erringBReader{}))
	_, err = os.Stat(filepath.Join(tmpdir, BlockMeta{}.ULID.String()) + ".tmp")
	testutil.Assert(t, os.IsNotExist(err), "directory is not cleaned up")
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

func (erringBReader) Index() (IndexReader, error)          { return nil, errors.New("index") }
func (erringBReader) Chunks() (ChunkReader, error)         { return nil, errors.New("chunks") }
func (erringBReader) Tombstones() (TombstoneReader, error) { return nil, errors.New("tombstones") }

type nopChunkWriter struct{}

func (nopChunkWriter) WriteChunks(chunks ...chunks.Meta) error { return nil }
func (nopChunkWriter) Close() error                            { return nil }

func TestCompaction_populateBlock(t *testing.T) {
	var populateBlocksCases = []struct {
		title              string
		inputSeriesSamples [][]seriesSamples
		compactMinTime     int64
		compactMaxTime     int64 // When not defined the test runner sets a default of math.MaxInt64.

		expSeriesSamples []seriesSamples
		expErr           error
	}{
		{
			title:              "Populate block from empty input should return error.",
			inputSeriesSamples: [][]seriesSamples{},
			expErr:             errors.New("cannot populate block from no readers"),
		},
		{
			// Populate from single block without chunks. We expect these kind of series being ignored.
			inputSeriesSamples: [][]seriesSamples{
				{
					{
						lset: map[string]string{"a": "b"},
					},
				},
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
			title: "Populate from two blocks showing that order is maintained.",
			inputSeriesSamples: [][]seriesSamples{
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
			},
			expSeriesSamples: []seriesSamples{
				{
					lset:   map[string]string{"a": "b"},
					chunks: [][]sample{{{t: 21}, {t: 30}}, {{t: 0}, {t: 10}}, {{t: 11}, {t: 20}}},
				},
				{
					lset:   map[string]string{"a": "c"},
					chunks: [][]sample{{{t: 40}, {t: 45}}, {{t: 1}, {t: 9}}, {{t: 10}, {t: 19}}},
				},
			},
		},
		{
			title: "Populate from two blocks showing that order or series is sorted.",
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
			// This should not happened because head block is making sure the chunks are not crossing block boundaries.
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
			expErr:         errors.New("found chunk with minTime: 10 maxTime: 30 outside of compacted minTime: 0 maxTime: 20"),
		},
		{
			// Introduced by https://github.com/prometheus/tsdb/issues/347.
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
			expErr:         errors.New("found chunk with minTime: 10 maxTime: 20 outside of compacted minTime: 0 maxTime: 10"),
		},
		{
			// No special deduplication expected.
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
					chunks: [][]sample{{{t: 1}, {t: 2}}, {{t: 10}, {t: 20}}, {{t: 10}, {t: 20}}},
				},
			},
		},
	}

	for _, tc := range populateBlocksCases {
		if ok := t.Run(tc.title, func(t *testing.T) {
			blocks := make([]BlockReader, 0, len(tc.inputSeriesSamples))
			for _, b := range tc.inputSeriesSamples {
				ir, cr := createIdxChkReaders(b)
				blocks = append(blocks, &mockBReader{ir: ir, cr: cr})
			}

			c, err := NewLeveledCompactor(nil, nil, []int64{0}, nil)
			testutil.Ok(t, err)

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
				testutil.NotOk(t, err)
				testutil.Equals(t, tc.expErr.Error(), err.Error())
				return
			}
			testutil.Ok(t, err)

			testutil.Equals(t, tc.expSeriesSamples, iw.series)

			// Check if stats are calculated properly.
			s := BlockStats{
				NumSeries: uint64(len(tc.expSeriesSamples)),
			}
			for _, series := range tc.expSeriesSamples {
				s.NumChunks += uint64(len(series.chunks))
				for _, chk := range series.chunks {
					s.NumSamples += uint64(len(chk))
				}
			}
			testutil.Equals(t, s, meta.Stats)
		}); !ok {
			return
		}
	}
}

// TestDisableAutoCompactions checks that we can
// disable and enable the auto compaction.
// This is needed for unit tests that rely on
// checking state before and after a compaction.
func TestDisableAutoCompactions(t *testing.T) {
	db, close := openTestDB(t, nil)
	defer close()
	defer db.Close()

	blockRange := DefaultOptions.BlockRanges[0]
	label := labels.FromStrings("foo", "bar")

	// Trigger a compaction to check that it was skipped and
	// no new blocks were created when compaction is disabled.
	db.DisableCompactions()
	app := db.Appender()
	for i := int64(0); i < 3; i++ {
		_, err := app.Add(label, i*blockRange, 0)
		testutil.Ok(t, err)
		_, err = app.Add(label, i*blockRange+1000, 0)
		testutil.Ok(t, err)
	}
	testutil.Ok(t, app.Commit())

	select {
	case db.compactc <- struct{}{}:
	default:
	}

	m := &dto.Metric{}
	for x := 0; x < 10; x++ {
		db.metrics.compactionsSkipped.Write(m)
		if *m.Counter.Value > float64(0) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	testutil.Assert(t, *m.Counter.Value > float64(0), "No compaction was skipped after the set timeout.")
	testutil.Equals(t, 0, len(db.blocks))

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
	testutil.Assert(t, len(db.Blocks()) > 0, "No block was persisted after the set timeout.")
}
