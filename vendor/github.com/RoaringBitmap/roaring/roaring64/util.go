package roaring64

import "github.com/RoaringBitmap/roaring"

func highbits(x uint64) uint32 {
	return uint32(x >> 32)
}

func lowbits(x uint64) uint32 {
	return uint32(x & maxLowBit)
}

const maxLowBit = roaring.MaxUint32
const maxUint32 = roaring.MaxUint32

func minOfInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxOfInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxOfUint32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func minOfUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
