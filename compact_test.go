package tsdb

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactionSelect(t *testing.T) {
	opts := &compactorOptions{
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
				{
					dir: "6",
					tr:  []int64{720, 960},
				},
				{
					dir: "7",
					tr:  []int64{1200, 1440},
				},
			},
			planned: [][]string{{"6", "7"}},
		},
		{
			blocks: []dirMetaSimple{
				{
					dir: "1",
					tr:  []int64{0, 20},
				},
				{
					dir: "2",
					tr:  []int64{60, 80},
				},
				{
					dir: "3",
					tr:  []int64{80, 100},
				},
			},
			planned: [][]string{{"2", "3"}},
		},
	}

	c := &compactor{
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
	splitterFunc := func(ds []dirMeta, tr int64) [][]dirMeta {
		rMap := make(map[int64][]dirMeta)
		for _, dir := range ds {
			t0 := dir.meta.MinTime - dir.meta.MinTime%tr
			if intervalContains(t0, t0+tr, dir.meta.MinTime) && intervalContains(t0, t0+tr, dir.meta.MaxTime) {
				rMap[t0] = append(rMap[t0], dir)
			}
		}
		res := make([][]dirMeta, 0, len(rMap))
		for _, v := range rMap {
			res = append(res, v)
		}

		sort.Slice(res, func(i, j int) bool {
			return res[i][0].meta.MinTime < res[j][0].meta.MinTime
		})

		return res
	}

	cases := []struct {
		trange int64
		ranges [][]int64
		output [][][]int64
	}{
		{
			trange: 60,
			ranges: [][]int64{{0, 10}},
		},
		{
			trange: 60,
			ranges: [][]int64{{0, 60}},
		},
		{
			trange: 60,
			ranges: [][]int64{{0, 10}, {30, 60}},
		},
		{
			trange: 60,
			ranges: [][]int64{{0, 10}, {60, 90}},
		},
		{
			trange: 60,
			ranges: [][]int64{{0, 10}, {20, 30}, {90, 120}},
		},
		{
			trange: 60,
			ranges: [][]int64{{0, 10}, {59, 60}, {60, 120}, {120, 180}, {190, 200}, {200, 210}, {220, 239}},
		},
	}

	for _, c := range cases {
		blocks := make([]dirMeta, 0, len(c.ranges))
		for _, r := range c.ranges {
			blocks = append(blocks, dirMeta{
				meta: &BlockMeta{
					MinTime: r[0],
					MaxTime: r[1],
				},
			})
		}

		require.Equal(t, splitterFunc(blocks, c.trange), splitByRange(blocks, c.trange))
	}
}
