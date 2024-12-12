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
)

const (
	bsiExistsBit = 0
	bsiSignBit   = 1
	bsiOffsetBit = 2
)

type OP byte

const (
	EQ  OP = 1 + iota // ==
	NEQ               // !=
	LT                // <
	LTE               // <=
	GT                // >
	GTE               // >=

	BETWEEN // ><  (this is like a <= x <= b)
)

// Range returns bitmaps with a bsiGroup value encoding matching the predicate.
func Range(tx OffsetRanger, op OP, shard, bitDepth uint64, predicate, end int64) *roaring.Bitmap {
	switch op {
	case EQ:
		return rangeEQ(tx, shard, bitDepth, predicate)
	case NEQ:
		return rangeNEQ(tx, shard, bitDepth, predicate)
	case LT, LTE:
		return rangeLT(tx, shard, bitDepth, predicate, op == LTE)
	case GT, GTE:
		return rangeGT(tx, shard, bitDepth, predicate, op == GTE)
	case BETWEEN:
		return rangeBetween(tx, shard, bitDepth, predicate, end)
	default:
		return roaring.NewBitmap()
	}
}

func rangeEQ(tx OffsetRanger, shard, bitDepth uint64, predicate int64) *roaring.Bitmap {
	b := Row(tx, shard, bsiExistsBit)
	if !b.Any() {
		return b
	}
	upredicate := absInt64(predicate)
	if uint64(bits.Len64(upredicate)) > bitDepth {
		// Predicate is out of range.
		return roaring.NewBitmap()
	}
	r := Row(tx, shard, bsiSignBit)
	if predicate < 0 {
		b = b.Intersect(r) // only negatives
	} else {
		b = b.Difference(r) // only positives
	}
	for i := int(bitDepth - 1); i >= 0; i-- {
		row := Row(tx, shard, uint64(bsiOffsetBit+i))

		bit := (upredicate >> uint(i)) & 1

		if bit == 1 {
			b = b.Intersect(row)
		} else {
			b = b.Difference(row)
		}
	}
	return b
}

func rangeNEQ(tx OffsetRanger, shard, bitDepth uint64, predicate int64) *roaring.Bitmap {
	// Start with set of columns with values set.
	b := Row(tx, shard, bsiExistsBit)

	// Get the equal bitmap.
	eq := rangeEQ(tx, shard, bitDepth, predicate)

	// Not-null minus the equal bitmap.
	b = b.Difference(eq)
	return b
}

func rangeLT(tx OffsetRanger, shard, bitDepth uint64, predicate int64, allowEquality bool) *roaring.Bitmap {
	if predicate == 1 && !allowEquality {
		predicate, allowEquality = 0, true
	}

	// Start with set of columns with values set.
	b := Row(tx, shard, bsiExistsBit)

	// Get the sign bit row.
	sign := Row(tx, shard, bsiSignBit)

	// Create predicate without sign bit.
	upredicate := absInt64(predicate)

	switch {
	case predicate == 0 && !allowEquality:
		// Match all negative integers.
		return b.Intersect(sign)
	case predicate == 0 && allowEquality:
		// Match all integers that are either negative or 0.
		zeroes := rangeEQ(tx, shard, bitDepth, 0)
		return b.Intersect(sign).Union(zeroes)
	case predicate < 0:
		// Match all every negative number beyond the predicate.
		return rangeGTUnsigned(tx, b.Intersect(sign), shard, bitDepth, upredicate, allowEquality)
	default:
		// Match positive numbers less than the predicate, and all negatives.
		pos := rangeLTUnsigned(tx, b.Difference(sign), shard, bitDepth, upredicate, allowEquality)
		neg := b.Intersect(sign)
		return pos.Union(neg)
	}
}

func rangeLTUnsigned(tx OffsetRanger, filter *roaring.Bitmap, shard, bitDepth, predicate uint64, allowEquality bool) *roaring.Bitmap {
	switch {
	case uint64(bits.Len64(predicate)) > bitDepth:
		fallthrough
	case predicate == (1<<bitDepth)-1 && allowEquality:
		// This query matches all possible values.
		return filter
	case predicate == (1<<bitDepth)-1 && !allowEquality:
		// This query matches everything that is not (1<<bitDepth)-1.
		matches := roaring.NewBitmap()
		for i := uint64(0); i < bitDepth; i++ {
			row := Row(tx, shard, bsiOffsetBit+i)
			matches = matches.Union(filter.Difference(row))
		}
		return matches
	case allowEquality:
		predicate++
	}

	// Compare intermediate bits.
	matched := roaring.NewBitmap()
	remaining := filter
	for i := int(bitDepth - 1); i >= 0 && predicate > 0 && remaining.Any(); i-- {
		row := Row(tx, shard, uint64(bsiOffsetBit+i))
		zeroes := remaining.Difference(row)
		switch (predicate >> uint(i)) & 1 {
		case 1:
			// Match everything with a zero bit here.
			matched = matched.Union(zeroes)
			predicate &^= 1 << uint(i)
		case 0:
			// Discard everything with a one bit here.
			remaining = zeroes
		}
	}

	return matched
}

func rangeGT(tx OffsetRanger, shard, bitDepth uint64, predicate int64, allowEquality bool) *roaring.Bitmap {
	if predicate == -1 && !allowEquality {
		predicate, allowEquality = 0, true
	}

	b := Row(tx, shard, bsiExistsBit)

	// Create predicate without sign bit.
	upredicate := absInt64(predicate)

	sign := Row(tx, shard, bsiSignBit)
	switch {
	case predicate == 0 && !allowEquality:
		// Match all positive numbers except zero.
		nonzero := rangeNEQ(tx, shard, bitDepth, 0)
		b = nonzero
		fallthrough
	case predicate == 0 && allowEquality:
		// Match all positive numbers.
		return b.Difference(sign)
	case predicate >= 0:
		// Match all positive numbers greater than the predicate.
		return rangeGTUnsigned(tx, b.Difference(sign), shard, bitDepth, upredicate, allowEquality)
	default:
		// Match all positives and greater negatives.
		neg := rangeLTUnsigned(tx, b.Intersect(sign), shard, bitDepth, upredicate, allowEquality)
		pos := b.Difference(sign)
		return pos.Union(neg)
	}
}

func rangeGTUnsigned(tx OffsetRanger, filter *roaring.Bitmap, shard, bitDepth, predicate uint64, allowEquality bool) *roaring.Bitmap {
prep:
	switch {
	case predicate == 0 && allowEquality:
		// This query matches all possible values.
		return filter
	case predicate == 0 && !allowEquality:
		// This query matches everything that is not 0.
		matches := roaring.NewBitmap()
		for i := uint64(0); i < bitDepth; i++ {
			row := Row(tx, shard, bsiOffsetBit+i)
			matches = matches.Union(filter.Intersect(row))
		}
		return matches
	case !allowEquality && uint64(bits.Len64(predicate)) > bitDepth:
		// The predicate is bigger than the BSI width, so nothing can be bigger.
		return roaring.NewBitmap()
	case allowEquality:
		predicate--
		allowEquality = false
		goto prep
	}

	// Compare intermediate bits.
	matched := roaring.NewBitmap()
	remaining := filter
	predicate |= (^uint64(0)) << bitDepth
	for i := int(bitDepth - 1); i >= 0 && predicate < ^uint64(0) && remaining.Any(); i-- {
		row := Row(tx, shard, uint64(bsiOffsetBit+i))
		ones := remaining.Intersect(row)
		switch (predicate >> uint(i)) & 1 {
		case 1:
			// Discard everything with a zero bit here.
			remaining = ones
		case 0:
			// Match everything with a one bit here.
			matched = matched.Union(ones)
			predicate |= 1 << uint(i)
		}
	}

	return matched
}

func rangeBetween(tx OffsetRanger, shard, bitDepth uint64, predicateMin, predicateMax int64) *roaring.Bitmap {
	b := Row(tx, shard, bsiExistsBit)

	// Convert predicates to unsigned values.
	upredicateMin, upredicateMax := absInt64(predicateMin), absInt64(predicateMax)

	switch {
	case predicateMin == predicateMax:
		return rangeEQ(tx, shard, bitDepth, predicateMin)
	case predicateMin >= 0:
		// Handle positive-only values.
		r := Row(tx, shard, bsiSignBit)
		return rangeBetweenUnsigned(tx, b.Difference(r), shard, bitDepth, upredicateMin, upredicateMax)
	case predicateMax < 0:
		// Handle negative-only values. Swap unsigned min/max predicates.
		r := Row(tx, shard, bsiSignBit)
		return rangeBetweenUnsigned(tx, b.Intersect(r), shard, bitDepth, upredicateMax, upredicateMin)
	default:
		// If predicate crosses positive/negative boundary then handle separately and union.
		r0 := Row(tx, shard, bsiSignBit)
		pos := rangeLTUnsigned(tx, b.Difference(r0), shard, bitDepth, upredicateMax, true)
		r1 := Row(tx, shard, bsiSignBit)
		neg := rangeLTUnsigned(tx, b.Intersect(r1), shard, bitDepth, upredicateMin, true)
		return pos.Union(neg)
	}
}

func rangeBetweenUnsigned(tx OffsetRanger, filter *roaring.Bitmap, shard, bitDepth, predicateMin, predicateMax uint64) *roaring.Bitmap {
	switch {
	case predicateMax > (1<<bitDepth)-1:
		// The upper bound cannot be violated.
		return rangeGTUnsigned(tx, filter, shard, bitDepth, predicateMin, true)
	case predicateMin == 0:
		// The lower bound cannot be violated.
		return rangeLTUnsigned(tx, filter, shard, bitDepth, predicateMax, true)
	}

	// Compare any upper bits which are equal.
	diffLen := bits.Len64(predicateMax ^ predicateMin)
	remaining := filter
	for i := int(bitDepth - 1); i >= diffLen; i-- {
		row := Row(tx, shard, uint64(bsiOffsetBit+i))
		switch (predicateMin >> uint(i)) & 1 {
		case 1:
			remaining = remaining.Intersect(row)
		case 0:
			remaining = remaining.Difference(row)
		}
	}

	// Clear the bits we just compared.
	equalMask := (^uint64(0)) << diffLen
	predicateMin &^= equalMask
	predicateMax &^= equalMask

	remaining = rangeGTUnsigned(tx, remaining, shard, uint64(diffLen), predicateMin, true)
	remaining = rangeLTUnsigned(tx, remaining, shard, uint64(diffLen), predicateMax, true)
	return remaining
}

func absInt64(v int64) uint64 {
	switch {
	case v > 0:
		return uint64(v)
	case v == -9223372036854775808:
		return 9223372036854775808
	default:
		return uint64(-v)
	}
}
