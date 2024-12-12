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

const (
	exponent                 = 20
	shardWidth               = 1 << exponent
	rowExponent              = (exponent - 16)  // for instance, 20-16 = 4
	RowWidth                 = 1 << rowExponent // containers per row, for instance 1<<4 = 16
	KeyMask                  = (RowWidth - 1)   // a mask for offset within the row
	ShardVsContainerExponent = 4
	RowMask                  = ^roaring.FilterKey(KeyMask) // a mask for the row bits, without converting them to a row ID
)

type Reader interface {
	ResetData(shard, field uint64) bool
	ResetExistence(shard, field uint64) bool
	Seek(key uint64) bool
	Valid() bool
	Next() bool
	Value() (uint64, *roaring.Container)
	Max() uint64
	OffsetRange(offset, start, end uint64) *roaring.Bitmap
}

const (
	// Row ids used for boolean fields.
	falseRowID = uint64(0)
	trueRowID  = uint64(1)
)

func Readtrue(ra Reader, shard uint64) *roaring.Bitmap {
	return Row(ra, shard, trueRowID)
}

func ReadFalse(ra Reader, shard uint64) *roaring.Bitmap {
	return Row(ra, shard, falseRowID)
}

func ReadEquality(ra Reader, shard uint64, filterBitmap *roaring.Bitmap, match func(row uint64, ra *roaring.Bitmap)) {
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

func MinBSI(r Reader, shard uint64) (result int64, count uint64) {
	depth := r.Max() / shardwidth.ShardWidth
	consider := Row(r, shard, bsiExistsBit)
	if !consider.Any() {
		return 0, 0
	}
	if row := Row(r, shard, bsiSignBit).Intersect(consider); row.Any() {
		result, count = maxUnsigned(r, row, depth, shard)
		return -result, count
	}
	return minUnsigned(r, consider, depth, shard)
}

func minUnsigned(r Reader, filter *roaring.Bitmap, depth, shard uint64) (result int64, count uint64) {
	count = filter.Count()
	for i := int(depth - 1); i >= 0; i-- {
		row := Row(r, shard, uint64(bsiOffsetBit+i))
		row = filter.Difference(row)
		count = row.Count()
		if count > 0 {
			filter = row
		} else {
			result += (1 << uint(i))
			if i == 0 {
				count = filter.Count()
			}
		}
	}
	return
}

func maxUnsigned(r Reader, filter *roaring.Bitmap, depth, shard uint64) (result int64, count uint64) {
	count = filter.Count()
	for i := int(depth - 1); i >= 0; i-- {
		row := Row(r, shard, uint64(bsiOffsetBit+i))
		row = row.Intersect(filter)

		count = row.Count()
		if count > 0 {
			result += (1 << uint(i))
			filter = row
		} else if i == 0 {
			count = filter.Count()
		}
	}
	return
}

type OffsetRanger interface {
	OffsetRange(offset, start, end uint64) *roaring.Bitmap
}

func Row(ra OffsetRanger, shard, rowID uint64) *roaring.Bitmap {
	return ra.OffsetRange(shardWidth*shard, shardWidth*rowID, shardWidth*(rowID+1))
}

func Existence(tx OffsetRanger, shard uint64) *roaring.Bitmap {
	return Row(tx, shard, bsiExistsBit)
}

func Slice(ra *roaring.Bitmap) []uint64 {
	if !ra.Any() {
		return nil
	}
	itr := ra.Iterator()
	itr.Seek(0)
	a := make([]uint64, 0, ra.Count())
	for v, eof := itr.Next(); !eof; v, eof = itr.Next() {
		a = append(a, v)
	}
	return a
}
