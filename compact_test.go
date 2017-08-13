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

func TestLeveledCompactor_Select(t *testing.T) {
	opts := &LeveledCompactorOptions{
		blockRanges: []int64{
			20,
			60,
			240,
			720,
			2160,
		},
	}

	type dirMetaSimple struct {
		dir string
		tr  []int64
	}

	cases := []struct {
		blocks  []dirMetaSimple
		planned [][]string
	}{
		{
			blocks: []dirMetaSimple{
				{
					dir: "1",
					tr:  []int64{0, 20},
				},
			},
			planned: nil,
		},
		{
			// We should wait for a third block of size 20 to appear before compacting
			// the existing ones.
			blocks: []dirMetaSimple{
				{
					dir: "1",
					tr:  []int64{0, 20},
				},
				{
					dir: "2",
					tr:  []int64{20, 40},
				},
			},
			planned: nil,
		},
		{
			// Block to fill the entire parent range appeared – should be compacted.
			blocks: []dirMetaSimple{
				{
					dir: "1",
					tr:  []int64{0, 20},
				},
				{
					dir: "2",
					tr:  []int64{20, 40},
				},
				{
					dir: "3",
					tr:  []int64{40, 60},
				},
			},
			planned: [][]string{{"1", "2", "3"}},
		},
		{
			// Block for the next parent range appeared. Nothing will happen in the first one
			// anymore and we should compact it.
			blocks: []dirMetaSimple{
				{
					dir: "1",
					tr:  []int64{0, 20},
				},
				{
					dir: "2",
					tr:  []int64{20, 40},
				},
				{
					dir: "3",
					tr:  []int64{60, 80},
				},
			},
			planned: [][]string{{"1", "2"}},
		},
		{
			blocks: []dirMetaSimple{
				{
					dir: "1",
					tr:  []int64{0, 20},
				},
				{
					dir: "2",
					tr:  []int64{20, 40},
				},
				{
					dir: "3",
					tr:  []int64{40, 60},
				},
				{
					dir: "4",
					tr:  []int64{60, 120},
				},
				{
					dir: "5",
					tr:  []int64{120, 180},
				},
			},
			planned: [][]string{{"1", "2", "3"}}, // We still need 0-60 to compact 0-240
		},
		{
			blocks: []dirMetaSimple{
				{
					dir: "2",
					tr:  []int64{20, 40},
				},
				{
					dir: "4",
					tr:  []int64{60, 120},
				},
				{
					dir: "5",
					tr:  []int64{120, 180},
				},
				{
					dir: "6",
					tr:  []int64{720, 960},
				},
				{
					dir: "7",
					tr:  []int64{1200, 1440},
				},
			},
			planned: [][]string{{"2", "4", "5"}},
		},
		{
			blocks: []dirMetaSimple{
				{
					dir: "1",
					tr:  []int64{0, 60},
				},
				{
					dir: "4",
					tr:  []int64{60, 80},
				},
				{
					dir: "5",
					tr:  []int64{80, 100},
				},
				{
					dir: "6",
					tr:  []int64{100, 120},
				},
			},
			planned: [][]string{{"4", "5", "6"}},
		},
	}

	c := &LeveledCompactor{
		opts: opts,
	}
	sliceDirs := func(dms []dirMeta) [][]string {
		if len(dms) == 0 {
			return nil
		}
		var res []string
		for _, dm := range dms {
			res = append(res, dm.dir)
		}
		return [][]string{res}
	}

	dmFromSimple := func(dms []dirMetaSimple) []dirMeta {
		dirs := make([]dirMeta, 0, len(dms))
		for _, dir := range dms {
			dirs = append(dirs, dirMeta{
				dir: dir.dir,
				meta: &BlockMeta{
					MinTime: dir.tr[0],
					MaxTime: dir.tr[1],
				},
			})
		}

		return dirs
	}

	for _, tc := range cases {
		require.Equal(t, tc.planned, sliceDirs(c.selectDirs(dmFromSimple(tc.blocks))))
	}
}

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

	c := NewLeveledCompactor(nil, nil, &LeveledCompactorOptions{
		blockRanges: []int64{50},
	})
	c.plan(metas)
}
