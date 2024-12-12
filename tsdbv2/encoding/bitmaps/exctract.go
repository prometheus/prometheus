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

package bitmaps

import (
	"github.com/gernest/roaring"
	"github.com/gernest/roaring/shardwidth"
)

func ReadDistinctMutex(ra Reader, shard uint64, filterBitmap *Bitmap, match func(row uint64, ra *Bitmap)) {
	filter := make([]*roaring.Container, 1<<ShardVsContainerExponent)
	filterIterator, _ := filterBitmap.Containers.Iterator(0)
	// So let's get these all with a nice convenient 0 offset...
	for filterIterator.Next() {
		k, c := filterIterator.Value()
		if c.N() == 0 {
			continue
		}
		filter[k%(1<<ShardVsContainerExponent)] = c
	}

	prevRow := ^uint64(0)
	seenThisRow := false
	resultContainerKey := shard * RowWidth
	for ra.Seek(0); ra.Valid(); ra.Next() {
		k, c := ra.Value()
		row := k >> ShardVsContainerExponent
		if row == prevRow {
			if seenThisRow {
				continue
			}
		} else {
			seenThisRow = false
			prevRow = row
		}
		if roaring.IntersectionAny(c, filter[k%(1<<ShardVsContainerExponent)]) {
			ro := roaring.NewBitmap()
			ro.Containers.Put(resultContainerKey, roaring.Intersect(c, filter[k%(1<<ShardVsContainerExponent)]))
			match(row, ro)
			seenThisRow = true
		}
	}
}

func ExtractEquality(ra Reader, shard uint64, match *roaring.Bitmap) (result []uint64) {
	result = make([]uint64, match.Count())

	filter := make([]*roaring.Container, 1<<ShardVsContainerExponent)
	filterIterator, _ := match.Containers.Iterator(0)
	for filterIterator.Next() {
		k, c := filterIterator.Value()
		if c.N() == 0 {
			continue
		}
		filter[k%(1<<ShardVsContainerExponent)] = c
	}

	prevRow := ^uint64(0)
	seenThisRow := false
	for ra.Seek(0); ra.Valid(); ra.Next() {
		k, c := ra.Value()
		row := k >> ShardVsContainerExponent
		if row == prevRow {
			if seenThisRow {
				continue
			}
		} else {
			seenThisRow = false
			prevRow = row
		}
		if roaring.IntersectionAny(c, filter[k%(1<<ShardVsContainerExponent)]) {
			indexIter(match, func(i int, matchRow uint64) {
				if c.Contains(uint16(matchRow)) {
					result[i] = row
				}
			})
			seenThisRow = true
		}
	}
	return
}

func ExtractBSI(r Reader, shard uint64, filter *roaring.Bitmap) (result []uint64) {
	result = make([]uint64, filter.Count())

	exists := Row(r, shard, bsiExistsBit)
	exists = exists.Intersect(filter)
	if !exists.Any() {
		return
	}

	data := make(map[uint64]uint64)
	mergeBits(exists, 0, data)

	sign := Row(r, shard, bsiSignBit)
	mergeBits(sign, 1<<63, data)

	bitDepth := r.Max() / shardwidth.ShardWidth

	for i := uint64(0); i < bitDepth; i++ {
		bits := Row(r, shard, bsiOffsetBit+i)

		bits = bits.Intersect(exists)
		mergeBits(bits, 1<<i, data)
	}
	indexIter(filter, func(i int, row uint64) {
		val, ok := data[row]
		if ok {
			result[i] = uint64((2*(int64(val)>>63) + 1) * int64(val&^(1<<63)))
		}
	})
	return
}

func indexIter(ra *roaring.Bitmap, f func(i int, row uint64)) {
	itr := ra.Iterator()
	itr.Seek(0)
	var i int
	for v, eof := itr.Next(); !eof; v, eof = itr.Next() {
		f(i, v)
		i++
	}
}

func mergeBits(ra *roaring.Bitmap, mask uint64, out map[uint64]uint64) {
	itr := ra.Iterator()
	itr.Seek(0)
	for v, eof := itr.Next(); !eof; v, eof = itr.Next() {
		out[v] |= mask
	}
}
