// Copyright 2024 The Prometheus Authors

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

package cursor_test

import (
	"math/rand/v2"
	"sort"
	"strconv"
	"testing"

	"github.com/gernest/roaring"
	"github.com/gernest/roaring/shardwidth"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdbv2/cursor"
	"github.com/prometheus/prometheus/tsdbv2/data"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
)

func TestCusrsor_FirstNext(t *testing.T) {
	db := data.Test(t)
	ra := roaring.NewBitmap(0x00000001, 0x00000002, 0x00010003, 0x00030004)
	w := new(bitmaps.Batch)
	ba := db.NewBatch()
	require.NoError(t, w.WriteData(ba, 0, 0, 0, ra))
	require.NoError(t, ba.Commit(nil))

	it, err := db.NewIter(nil)
	require.NoError(t, err)
	defer it.Close()

	cu := cursor.New(it, 0)
	require.True(t, cu.ResetData(0, 0))
	require.True(t, cu.First())
	key, co := cu.Value()
	require.Equal(t, uint64(0), key)
	require.Equal(t, []uint16{1, 2}, co.Slice())

	require.True(t, cu.Next())
	key, co = cu.Value()
	require.Equal(t, uint64(1), key)
	require.Equal(t, []uint16{3}, co.Slice())

	require.True(t, cu.Next())
	key, co = cu.Value()
	require.Equal(t, uint64(3), key)
	require.Equal(t, []uint16{4}, co.Slice())
}

func TestCursor_FirstNext_Quick(t *testing.T) {
	if testing.Short() {
		t.Skip("-short enabled, skipping")
	}
	const n = 10000

	quickCheck(t, func(t *testing.T, rand *rand.Rand) {
		t.Parallel()

		// Generate sorted list of values.
		values := make([]uint64, rand.IntN(n))
		for i := range values {
			values[i] = rand.Uint64N(shardwidth.ShardWidth)
		}
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })

		db := data.Test(t)

		ra := roaring.NewBitmap(values...)
		ba := db.NewBatch()
		w := new(bitmaps.Batch)
		require.NoError(t, w.WriteData(ba, 0, 0, 0, ra))
		require.NoError(t, ba.Commit(nil))
		// Generate unique bucketed values.
		type Item struct {
			values []uint16
			key    uint64
		}
		var items []Item
		m := make(map[uint64]struct{})
		for _, v := range values {
			if _, ok := m[v]; ok {
				continue
			}
			m[v] = struct{}{}

			hi, lo := highbits(v), lowbits(v)
			if len(items) == 0 || items[len(items)-1].key != hi {
				items = append(items, Item{key: hi})
			}

			item := &items[len(items)-1]
			item.values = append(item.values, lo)
		}
		it, err := db.NewIter(nil)
		require.NoError(t, err)
		defer it.Close()
		cu := cursor.New(it, 0)

		for i, item := range items {
			if i == 0 {
				require.True(t, cu.First())
			} else {
				require.True(t, cu.Next())
			}
			key, co := cu.Value()
			require.Equal(t, item.key, key)
			require.Equal(t, item.values, co.Slice())
		}
	})
}

func TestCursor_RLETesting(t *testing.T) {
	db := data.Test(t)
	bm := roaring.NewBitmap()
	bm.Put(0, roaring.NewContainerRun([]roaring.Interval16{{Start: 10, Last: 11}}))
	ba := db.NewBatch()
	w := new(bitmaps.Batch)
	require.NoError(t, w.WriteData(ba, 0, 0, 0, bm))
	require.NoError(t, ba.Commit(nil))

	type T struct {
		name        string
		args        []uint64
		want        []uint16
		wantChanged bool
		wantErr     bool
	}
	samples := []T{
		{
			name:        "update run at Last",
			args:        []uint64{0x0000000c},
			want:        []uint16{0x0000000a, 0x0000000b, 0x0000000c},
			wantChanged: true,
			wantErr:     false,
		},
		{
			name:        "update run at beginning",
			args:        []uint64{0x00000001, 0x00000002},
			want:        []uint16{0x00000001, 0x00000002, 0x0000000a, 0x0000000b, 0x0000000c},
			wantChanged: true,
			wantErr:     false,
		},
		{
			name:        "add run at end",
			args:        []uint64{0x0000000f},
			want:        []uint16{0x00000001, 0x00000002, 0x0000000a, 0x0000000b, 0x0000000c, 0x0000000f},
			wantChanged: true,
			wantErr:     false,
		},
		{
			name:        "no change",
			args:        []uint64{0x0000000b},
			want:        []uint16{0x00000001, 0x00000002, 0x0000000a, 0x0000000b, 0x0000000c, 0x0000000f},
			wantChanged: false,
			wantErr:     false,
		},
		{
			name:        "update start",
			args:        []uint64{0x0000000e},
			want:        []uint16{0x00000001, 0x00000002, 0x0000000a, 0x0000000b, 0x0000000c, 0x0000000e, 0x0000000f},
			wantChanged: true,
			wantErr:     false,
		},
		{
			name:        "combine",
			args:        []uint64{0x0000000d},
			want:        []uint16{0x00000001, 0x00000002, 0x0000000a, 0x0000000b, 0x0000000c, 0x0000000d, 0x0000000e, 0x0000000f},
			wantChanged: true,
			wantErr:     false,
		},
		{
			name:        "add end",
			args:        []uint64{0x0000ffff},
			want:        []uint16{0x00000001, 0x00000002, 0x0000000a, 0x0000000b, 0x0000000c, 0x0000000d, 0x0000000e, 0x0000000f, 0x0000ffff},
			wantChanged: true,
			wantErr:     false,
		},
		{
			name:        "overflow container",
			args:        []uint64{0x00010000},
			want:        []uint16{0x00000001, 0x00000002, 0x0000000a, 0x0000000b, 0x0000000c, 0x0000000d, 0x0000000e, 0x0000000f, 0x0000ffff},
			wantChanged: true,
			wantErr:     false,
		},
	}

	for _, s := range samples {
		if true {
			continue
		}
		t.Run(s.name, func(t *testing.T) {
			ba := db.NewBatch()
			require.NoError(t, w.WriteData(ba, 0, 0, 0, roaring.NewBitmap(s.args...)))
			require.NoError(t, ba.Commit(nil))

			it, err := db.NewIter(nil)
			require.NoError(t, err)
			defer it.Close()

			cu := cursor.New(it, 0)

			require.True(t, cu.ResetData(0, 0))
			require.True(t, cu.First())

			_, co := cu.Value()
			require.Equal(t, s.want, co.Slice())
		})
	}
}

func quickCheck(t *testing.T, fn func(t *testing.T, rand *rand.Rand)) {
	for i := range 10 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			fn(t, rand.New(src(i+1)))
		})
	}
}

type src uint64

func (s src) Uint64() uint64 {
	return uint64(s)
}

func highbits(v uint64) uint64 { return v >> 16 }
func lowbits(v uint64) uint16  { return uint16(v & 0xFFFF) }
