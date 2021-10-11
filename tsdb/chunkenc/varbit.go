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
// optimized for the dod's observed in histogram buckets.
//
// TODO(Dieterbe): We could improve this further: Each branch doesn't need to
// support any values of any of the prior branches. So we can expand the range
// of each branch. Do more with fewer bits. It comes at the price of more
// expensive encoding and decoding (cutting out and later adding back that
// center-piece we skip).
func putVarbitInt(b *bstream, val int64) {
	switch {
	case val == 0:
		b.writeBit(zero)
	case bitRange(val, 3): // -3 <= val <= 4
		b.writeBits(0b10, 2)
		b.writeBits(uint64(val), 3)
	case bitRange(val, 6): // -31 <= val <= 32
		b.writeBits(0b110, 3)
		b.writeBits(uint64(val), 6)
	case bitRange(val, 9): // -255 <= val <= 256
		b.writeBits(0b1110, 4)
		b.writeBits(uint64(val), 9)
	case bitRange(val, 12): // -2047 <= val <= 2048
		b.writeBits(0b11110, 5)
		b.writeBits(uint64(val), 12)
	default:
		b.writeBits(0b11111, 5)
		b.writeBits(uint64(val), 64)
	}
}

// readVarbitInt reads an int64 encoced with putVarbitInt.
func readVarbitInt(b *bstreamReader) (int64, error) {
	var d byte
	for i := 0; i < 5; i++ {
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
	case 0b11111:
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := b.readBits(64)
		if err != nil {
			return 0, err
		}

		val = int64(bits)
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
