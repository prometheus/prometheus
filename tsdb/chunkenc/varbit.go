// Copyright 2021 The Prometheus Authors
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

package chunkenc

import (
	"math"
	"math/bits"

	"github.com/pkg/errors"
)

// putVarbitFloat writes a float64 using varbit encoding.  It does so by
// converting the underlying bits into an int64.
func putVarbitFloat(b *bstream, val float64) {
	// TODO(beorn7): The resulting int64 here will almost never be a small
	// integer. Thus, the varbit encoding doesn't really make sense
	// here. This function is only used to encode the zero threshold in
	// histograms. Based on that, here is an idea to improve the encoding:
	//
	// It is recommended to use (usually negative) powers of two as
	// threshoulds. The default value for the zero threshald is in fact
	// 2^-128, or 0.5*2^-127, as it is represented by IEEE 754. It is
	// therefore worth a try to test if the threshold is a power of 2 and
	// then just store the exponent. 0 is also a commen threshold for those
	// use cases where only observations of precisely zero should go to the
	// zero bucket. This results in the following proposal:
	// - First we store 1 byte.
	// - Iff that byte is 255 (all bits set), it is followed by a direct
	//   8byte representation of the float.
	// - If the byte is 0, the threshold is 0.
	// - In all other cases, take the number represented by the byte,
	//   subtract 246, and that's the exponent (i.e. between -245 and
	//   +8, covering thresholds that are powers of 2 between 2^-246
	//   to 128).
	putVarbitInt(b, int64(math.Float64bits(val)))
}

// readVarbitFloat reads a float64 encoded with putVarbitFloat
func readVarbitFloat(b *bstreamReader) (float64, error) {
	val, err := readVarbitInt(b)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(uint64(val)), nil
}

// putVarbitInt writes an int64 using varbit encoding with a bit bucketing
// optimized for the dod's observed in histogram buckets, plus a few additional
// buckets for large numbers.
//
// TODO(Dieterbe): We could improve this further: Each branch doesn't need to
// support any values of any of the prior branches. So we can expand the range
// of each branch. Do more with fewer bits. It comes at the price of more
// expensive encoding and decoding (cutting out and later adding back that
// center-piece we skip).
func putVarbitInt(b *bstream, val int64) {
	switch {
	case val == 0: // Precisely 0, needs 1 bit.
		b.writeBit(zero)
	case bitRange(val, 3): // -3 <= val <= 4, needs 5 bits.
		b.writeBits(0b10, 2)
		b.writeBits(uint64(val), 3)
	case bitRange(val, 6): // -31 <= val <= 32, 9 bits.
		b.writeBits(0b110, 3)
		b.writeBits(uint64(val), 6)
	case bitRange(val, 9): // -255 <= val <= 256, 13 bits.
		b.writeBits(0b1110, 4)
		b.writeBits(uint64(val), 9)
	case bitRange(val, 12): // -2047 <= val <= 2048, 17 bits.
		b.writeBits(0b11110, 5)
		b.writeBits(uint64(val), 12)
	case bitRange(val, 18): // -131071 <= val <= 131072, 3 bytes.
		b.writeBits(0b111110, 6)
		b.writeBits(uint64(val), 18)
	case bitRange(val, 25): // -16777215 <= val <= 16777216, 4 bytes.
		b.writeBits(0b1111110, 7)
		b.writeBits(uint64(val), 25)
	case bitRange(val, 56): // -36028797018963967 <= val <= 36028797018963968, 8 bytes.
		b.writeBits(0b11111110, 8)
		b.writeBits(uint64(val), 56)
	default:
		b.writeBits(0b11111111, 8) // Worst case, needs 9 bytes.
		b.writeBits(uint64(val), 64)
	}
}

// readVarbitInt reads an int64 encoced with putVarbitInt.
func readVarbitInt(b *bstreamReader) (int64, error) {
	var d byte
	for i := 0; i < 8; i++ {
		d <<= 1
		bit, err := b.readBitFast()
		if err != nil {
			bit, err = b.readBit()
		}
		if err != nil {
			return 0, err
		}
		if bit == zero {
			break
		}
		d |= 1
	}

	var val int64
	var sz uint8

	switch d {
	case 0b0:
		// val == 0
	case 0b10:
		sz = 3
	case 0b110:
		sz = 6
	case 0b1110:
		sz = 9
	case 0b11110:
		sz = 12
	case 0b111110:
		sz = 18
	case 0b1111110:
		sz = 25
	case 0b11111110:
		sz = 56
	case 0b11111111:
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := b.readBits(64)
		if err != nil {
			return 0, err
		}

		val = int64(bits)
	default:
		return 0, errors.Errorf("invalid bit pattern %b", d)
	}

	if sz != 0 {
		bits, err := b.readBitsFast(sz)
		if err != nil {
			bits, err = b.readBits(sz)
		}
		if err != nil {
			return 0, err
		}
		if bits > (1 << (sz - 1)) {
			// Or something.
			bits = bits - (1 << sz)
		}
		val = int64(bits)
	}

	return val, nil
}

func bitRangeUint(x uint64, nbits int) bool {
	return bits.LeadingZeros64(x) >= 64-nbits
}

// putVarbitUint writes a uint64 using varbit encoding. It uses the same bit
// buckets as putVarbitInt.
func putVarbitUint(b *bstream, val uint64) {
	switch {
	case val == 0: // Precisely 0, needs 1 bit.
		b.writeBit(zero)
	case bitRangeUint(val, 3): // val <= 7, needs 5 bits.
		b.writeBits(0b10, 2)
		b.writeBits(val, 3)
	case bitRangeUint(val, 6): // val <= 63, 9 bits.
		b.writeBits(0b110, 3)
		b.writeBits(val, 6)
	case bitRangeUint(val, 9): // val <= 511, 13 bits.
		b.writeBits(0b1110, 4)
		b.writeBits(val, 9)
	case bitRangeUint(val, 12): // val <= 4095, 17 bits.
		b.writeBits(0b11110, 5)
		b.writeBits(val, 12)
	case bitRangeUint(val, 18): // val <= 262143, 3 bytes.
		b.writeBits(0b111110, 6)
		b.writeBits(val, 18)
	case bitRangeUint(val, 25): // val <= 33554431, 4 bytes.
		b.writeBits(0b1111110, 7)
		b.writeBits(val, 25)
	case bitRangeUint(val, 56): // val <= 72057594037927935, 8 bytes.
		b.writeBits(0b11111110, 8)
		b.writeBits(val, 56)
	default:
		b.writeBits(0b11111111, 8) // Worst case, needs 9 bytes.
		b.writeBits(val, 64)
	}
}

// readVarbitUint reads a uint64 encoced with putVarbitUint.
func readVarbitUint(b *bstreamReader) (uint64, error) {
	var d byte
	for i := 0; i < 8; i++ {
		d <<= 1
		bit, err := b.readBitFast()
		if err != nil {
			bit, err = b.readBit()
		}
		if err != nil {
			return 0, err
		}
		if bit == zero {
			break
		}
		d |= 1
	}

	var (
		bits uint64
		sz   uint8
		err  error
	)

	switch d {
	case 0b0:
		// val == 0
	case 0b10:
		sz = 3
	case 0b110:
		sz = 6
	case 0b1110:
		sz = 9
	case 0b11110:
		sz = 12
	case 0b111110:
		sz = 18
	case 0b1111110:
		sz = 25
	case 0b11111110:
		sz = 56
	case 0b11111111:
		// Do not use fast because it's very unlikely it will succeed.
		bits, err = b.readBits(64)
		if err != nil {
			return 0, err
		}
	default:
		return 0, errors.Errorf("invalid bit pattern %b", d)
	}

	if sz != 0 {
		bits, err = b.readBitsFast(sz)
		if err != nil {
			bits, err = b.readBits(sz)
		}
		if err != nil {
			return 0, err
		}
	}

	return bits, nil
}
