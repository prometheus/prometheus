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
	"math/bits"

	"github.com/gernest/roaring"
	"github.com/gernest/roaring/shardwidth"
)

const (
	falseRowOffset = 0 * shardwidth.ShardWidth
	trueRowOffset  = 1 * shardwidth.ShardWidth
)

func Equality(ra *roaring.Bitmap, id, value uint64) {
	ra.DirectAdd(
		value*shardwidth.ShardWidth +
			(id % shardwidth.ShardWidth),
	)
}

func Bool(ra *roaring.Bitmap, id uint64, value bool) {
	fragmentColumn := id % shardwidth.ShardWidth
	if value {
		ra.DirectAdd(trueRowOffset + fragmentColumn)
	} else {
		ra.DirectAdd(falseRowOffset + fragmentColumn)
	}
}

func BitSliced(ra *roaring.Bitmap, id uint64, svalue int64) {
	fragmentColumn := id % shardwidth.ShardWidth
	ra.DirectAdd(fragmentColumn)
	negative := svalue < 0
	var value uint64
	if negative {
		ra.DirectAdd(shardwidth.ShardWidth + fragmentColumn) // set sign bit
		value = uint64(svalue * -1)
	} else {
		value = uint64(svalue)
	}
	lz := bits.LeadingZeros64(value)
	row := uint64(2)
	for mask := uint64(0x1); mask <= 1<<(64-lz) && mask != 0; mask <<= 1 {
		if value&mask > 0 {
			ra.DirectAdd(row*shardwidth.ShardWidth + fragmentColumn)
		}
		row++
	}
}

func SetExistenceBit(ra *roaring.Bitmap, id uint64) {
	ra.DirectAdd(id % shardwidth.ShardWidth)
}
