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
	"testing"

	"github.com/stretchr/testify/require"
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

	c, err := NewLeveledCompactor(nil, nil, []int64{50}, nil)
	require.NoError(t, err)

	c.plan(metas)
}

func TestLeveledCompactor_plan(t *testing.T) {
	compactor, err := NewLeveledCompactor(nil, nil, []int64{
		20,
		60,
		240,
		720,
		2160,
	}, nil)
	require.NoError(t, err)

	metaRange := func(name string, mint, maxt int64, stats *BlockStats) dirMeta {
		meta := &BlockMeta{MinTime: mint, MaxTime: maxt}
		if stats != nil {
			meta.Stats = *stats
		}
		return dirMeta{
			dir:  name,
			meta: meta,
		}
	}

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
		// We should wait for a third block of size 20 to appear before compacting
		// the existing ones.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
			},
			expected: nil,
		},
		// Block to fill the entire parent range appeared – should be compacted.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		// Block for the next parent range appeared. Nothing will happen in the first one
		// anymore and we should compact it.
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
			},
			expected: []string{"1", "2"},
		},
		{
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
			metas: []dirMeta{
				metaRange("2", 20, 40, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 120, 180, nil),
				metaRange("6", 720, 960, nil),
			},
			expected: []string{"2", "4", "5"},
		},
		{
			metas: []dirMeta{
				metaRange("1", 0, 60, nil),
				metaRange("4", 60, 80, nil),
				metaRange("5", 80, 100, nil),
				metaRange("6", 100, 120, nil),
			},
			expected: []string{"4", "5", "6"},
		},
		// Select large blocks that have many tombstones.
		{
			metas: []dirMeta{
				metaRange("1", 0, 720, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
			},
			expected: []string{"1"},
		},
		// For small blocks, do not compact tombstones.
		{
			metas: []dirMeta{
				metaRange("1", 0, 30, &BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
			},
			expected: nil,
		},
		// Regression test: we were stuck in a compact loop where we always recompacted
		// the same block when tombstones and series counts were zero.
		{
			metas: []dirMeta{
				metaRange("1", 0, 720, &BlockStats{
					NumSeries:     0,
					NumTombstones: 0,
				}),
			},
			expected: nil,
		},
	}

	for i, c := range cases {
		res, err := compactor.plan(c.metas)
		require.NoError(t, err)

		require.Equal(t, c.expected, res, "test case %d", i)
	}
}
