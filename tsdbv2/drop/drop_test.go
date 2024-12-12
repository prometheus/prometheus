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

package drop_test

import (
	"testing"

	"github.com/gernest/roaring"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdbv2/cursor"
	"github.com/prometheus/prometheus/tsdbv2/data"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
)

func TestDrop_equality(t *testing.T) {
	db := data.Test(t)
	ra := roaring.NewBitmap()

	for n := range uint64(6) {
		bitmaps.Equality(ra, n, n)
	}
	bitmaps.Equality(ra, 4, 3)
	bitmaps.Equality(ra, 5, 3)
	bitmaps.Equality(ra, 56, 3)
	w := new(bitmaps.Batch)
	ba := db.NewBatch()
	require.NoError(t, w.WriteData(ba, 0, 0, 0, ra))
	require.NoError(t, ba.Commit(nil))

	it, err := db.NewIter(nil)
	require.NoError(t, err)
	defer it.Close()

	cu := cursor.New(it, 0)
	require.True(t, cu.ResetData(0, 0))
	u := new(cursor.Update)
	cu.ClearRecords(roaring.NewBitmap(2, 3), u)
	var key encoding.Key
	var keys []uint64
	var cos [][]uint16
	u.IterKey(&key, func(key []byte, co *roaring.Container) error {
		ck := encoding.ContainerKey(key)
		keys = append(keys, ck>>4)
		cos = append(cos, co.Slice())
		return nil
	})
	require.Equal(t, []uint64{2, 3}, keys)
	require.Equal(t, [][]uint16{{}, {4, 5, 56}}, cos)
}

func TestDrop_bsi(t *testing.T) {
	db := data.Test(t)
	ra := roaring.NewBitmap()

	for n := range uint64(6) {
		bitmaps.BitSliced(ra, n, int64(n))
	}
	w := new(bitmaps.Batch)
	ba := db.NewBatch()
	require.NoError(t, w.WriteData(ba, 0, 0, 0, ra))
	require.NoError(t, ba.Commit(nil))

	it, err := db.NewIter(nil)
	require.NoError(t, err)

	cu := cursor.New(it, 0)
	require.True(t, cu.ResetData(0, 0))
	u := new(cursor.Update)
	cu.ClearRecords(roaring.NewBitmap(2, 3), u)
	it.Close()
	var key encoding.Key
	ba = db.NewBatch()
	err = u.IterKey(&key, func(key []byte, co *roaring.Container) error {
		if co.N() == 0 {
			return ba.Delete(key, nil)
		}
		return ba.Set(key, co.Encode(), nil)
	})
	require.NoError(t, err)
	require.NoError(t, ba.Commit(nil))

	it, err = db.NewIter(nil)
	require.NoError(t, err)
	defer it.Close()

	cu = cursor.New(it, 0)
	require.True(t, cu.ResetData(0, 0))
	result := cu.ReadBSI(0, 0, roaring.NewBitmap(1, 2, 3, 4, 5))
	require.Equal(t, []uint64{1, 0, 0, 4, 5}, result)
}
