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

// putSTVarbitInt writes an int64 using varbit encoding with a bit bucketing
// optimized for ST.
func putSTVarbitInt(b *bstream, val int64) {
	switch {
	case val == 0:
		b.writeBit(zero)
	case bitRange(val, 6):
		b.writeByte(0b10<<6 | (uint8(val) & (1<<6 - 1))) // 0b10 size code combined with 6 bits of dod.
	case bitRange(val, 13):
		b.writeByte(0b110<<5 | (uint8(val>>8) & (1<<5 - 1))) // 0b110 size code combined with 5 bits of dod.
		b.writeByte(uint8(val))                              // Bottom 8 bits of dod.
	case bitRange(val, 20):
		b.writeBits(0b1110, 4)
		b.writeBits(uint64(val), 20)
	default:
		b.writeBits(0b1111, 4)
		b.writeBits(uint64(val), 64)
	}
}

// readSTVarbitInt reads an int64 encoded with putSTVarbitInt.
func readSTVarbitInt(b *bstreamReader) (int64, error) {
	var d byte
	// read delta-of-delta
	for range 4 {
		d <<= 1
		bit, err := b.readBitFast()
		if err != nil {
			bit, err = b.readBit()
			if err != nil {
				return 0, err
			}
		}
		if bit == zero {
			break
		}
		d |= 1
	}
	var sz uint8
	var val int64
	switch d {
	case 0b0:
		// dod == 0
	case 0b10:
		sz = 6
	case 0b110:
		sz = 13
	case 0b1110:
		sz = 20
	case 0b1111:
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
			if err != nil {
				return 0, err
			}
		}

		// Account for negative numbers, which come back as high unsigned numbers.
		// See docs/bstream.md.
		if bits > (1 << (sz - 1)) {
			bits -= 1 << sz
		}
		val = int64(bits)
	}
	return val, nil
}

func putSTDiff(b *bstream, noChange bool, val int64) {
	if noChange {
		b.writeBit(zero)
		return
	}
	switch {
	case bitRange(val, 6):
		b.writeByte(0b10<<6 | (uint8(val) & (1<<6 - 1))) // 0b10 size code combined with 6 bits of dod.
	case bitRange(val, 13):
		b.writeByte(0b110<<5 | (uint8(val>>8) & (1<<5 - 1))) // 0b110 size code combined with 5 bits of dod.
		b.writeByte(uint8(val))                              // Bottom 8 bits of dod.
	case bitRange(val, 20):
		b.writeBits(0b1110, 4)
		b.writeBits(uint64(val), 20)
	default:
		b.writeBits(0b1111, 4)
		b.writeBits(uint64(val), 64)
	}
}

func readSTDiff(b *bstreamReader) (bool, int64, error) {
	var d byte
	// read delta-of-delta
	for range 4 {
		d <<= 1
		bit, err := b.readBitFast()
		if err != nil {
			bit, err = b.readBit()
			if err != nil {
				return false, 0, err
			}
		}
		if bit == zero {
			break
		}
		d |= 1
	}
	var sz uint8
	var val int64
	switch d {
	case 0b0:
		return true, 0, nil
	case 0b10:
		sz = 6
	case 0b110:
		sz = 13
	case 0b1110:
		sz = 20
	case 0b1111:
		// Do not use fast because it's very unlikely it will succeed.
		bits, err := b.readBits(64)
		if err != nil {
			return false, 0, err
		}

		val = int64(bits)
	}

	if sz != 0 {
		bits, err := b.readBitsFast(sz)
		if err != nil {
			bits, err = b.readBits(sz)
			if err != nil {
				return false, 0, err
			}
		}

		// Account for negative numbers, which come back as high unsigned numbers.
		// See docs/bstream.md.
		if bits > (1 << (sz - 1)) {
			bits -= 1 << sz
		}
		val = int64(bits)
	}
	return false, val, nil
}
