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

// The code in this file was largely written by Damian Gryski as part of
// https://github.com/dgryski/go-tsz and published under the license below.
// It was modified to accommodate reading from byte slices without modifying
// the underlying bytes, which would panic when reading from mmap'd
// read-only byte slices.

// Copyright (c) 2015,2016 Damian Gryski <damian@gryski.com>
// All rights reserved.

// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:

// * Redistributions of source code must retain the above copyright notice,
// this list of conditions and the following disclaimer.
//
// * Redistributions in binary form must reproduce the above copyright notice,
// this list of conditions and the following disclaimer in the documentation
// and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package chunkenc

import (
	"math"
)

// putFloat64VBBucket writes a float64 using varbit optimized for SHS buckets.
// It does so by converting the underlying bits into an int64.
func putFloat64VBBucket(b *bstream, val float64) {
	// TODO: Since this is used for the zero threshold, this almost always goes into the default
	// bit range (i.e. using 5+64 bits). So we can consider skipping `putInt64VBBucket` and directly
	// write the float and save 5 bits here.
	putInt64VBBucket(b, int64(math.Float64bits(val)))
}

// readFloat64VBBucket reads a float64 using varbit optimized for SHS buckets
func readFloat64VBBucket(b *bstreamReader) (float64, error) {
	val, err := readInt64VBBucket(b)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(uint64(val)), nil
}

// putInt64VBBucket writes an int64 using varbit optimized for SHS buckets.
//
// TODO(Dieterbe): We could improve this further: Each branch doesn't need to
// support any values of any of the prior branches. So we can expand the range
// of each branch. Do more with fewer bits. It comes at the price of more
// expensive encoding and decoding (cutting out and later adding back that
// center-piece we skip).
func putInt64VBBucket(b *bstream, val int64) {
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

// readInt64VBBucket reads an int64 using varbit optimized for SHS buckets
func readInt64VBBucket(b *bstreamReader) (int64, error) {
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
			// or something
			bits = bits - (1 << sz)
		}
		val = int64(bits)
	}

	return val, nil
}
