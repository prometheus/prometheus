// Copyright The Prometheus Authors
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
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/timestamp"
)

func TestVarbitInt(t *testing.T) {
	numbers := []int64{
		math.MinInt64,
		-36028797018963968, -36028797018963967,
		-16777216, -16777215,
		-131072, -131071,
		-2048, -2047,
		-256, -255,
		-32, -31,
		-4, -3,
		-1, 0, 1,
		4, 5,
		32, 33,
		256, 257,
		2048, 2049,
		131072, 131073,
		16777216, 16777217,
		36028797018963968, 36028797018963969,
		math.MaxInt64,
	}

	bs := bstream{}

	for _, n := range numbers {
		putVarbitInt(&bs, n)
	}

	bsr := newBReader(bs.bytes())

	for _, want := range numbers {
		got, err := readVarbitInt(&bsr)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestVarbitUint(t *testing.T) {
	numbers := []uint64{
		0, 1,
		7, 8,
		63, 64,
		511, 512,
		4095, 4096,
		262143, 262144,
		33554431, 33554432,
		72057594037927935, 72057594037927936,
		math.MaxUint64,
	}

	bs := bstream{}

	for _, n := range numbers {
		putVarbitUint(&bs, n)
	}

	bsr := newBReader(bs.bytes())

	for _, want := range numbers {
		got, err := readVarbitUint(&bsr)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestVarbitTimestamp(t *testing.T) {
	bs := bstream{}

	{
		ts := timestamp.FromTime(time.Now())
		curr := bs.lenBits()

		putVarbitInt(&bs, ts)
		require.Equal(t, 8*8, bs.lenBits()-curr)

		curr = bs.lenBits()
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, ts)] {
			bs.writeByte(b)
		}
		require.Equal(t, 8*6, bs.lenBits()-curr)

		i := uint8(0)
		for ; ; i++ {
			if bitRange(ts, i) {
				break
			}
		}
		fmt.Println("typical timestamp fits", i) // 42
	}
	{
		ts := int64(15000)
		curr := bs.lenBits()

		putVarbitInt(&bs, ts)
		require.Equal(t, 8*3, bs.lenBits()-curr)

		curr = bs.lenBits()
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, ts)] {
			bs.writeByte(b)
		}
		require.Equal(t, 8*3, bs.lenBits()-curr)

		i := uint8(0)
		for ; ; i++ {
			if bitRange(ts, i) {
				break
			}
		}
		fmt.Println("typical timestamp fits", i) // 15
	}
}

func TestYolo(t *testing.T) {
	curr := -1
	for n := range 150 {
		i := uint8(0)
		for ; ; i++ {
			if bitRange(int64(n), i) {
				break
			}
		}
		if int(i) > curr {
			curr = int(i)
			fmt.Println("Next bit size is for", n, "with ", i, "bits")
		}
	}
	curr = math.MaxInt64
	for n := range 150 {
		n = n - 150
		i := uint8(0)
		for ; ; i++ {
			if bitRange(int64(n), i) {
				break
			}
		}
		if int(i) < curr {
			curr = int(i)
			fmt.Println("Next bit size is for", n, "with ", i, "bits")
		}
	}

	// How far TS is 42 bits?
	ts := time.Now()
	ts.Add(100 * 365 * 24 * time.Hour)
	for range 100 {
		i := uint8(0)
		for ; ; i++ {
			if bitRange(timestamp.FromTime(ts), i) {
				break
			}
		}
		fmt.Println(ts, "timestamp fits", i)
		ts = ts.Add(1 * 365 * 24 * time.Hour)
	}
}

func TestSTDiff(t *testing.T) {
	// Just a basic test to see if encoding/decoding works as intended.
	values := []int64{-1000000, -100000, -1000, -10, 0, 10, 1000, 100000, 1000000}

	for _, v := range values {
		for bitOffset := 0; bitOffset < 8; bitOffset++ {
			// Create bstream with bitoffset.
			bs := bstream{}
			for i := 0; i < bitOffset; i++ {
				bs.writeBit(zero)
			}
			t.Run(fmt.Sprintf("val %d bit offset %d", v, bitOffset), func(t *testing.T) {
				putSTDiff(&bs, false, v)

				bsr := newBReader(bs.bytes())
				// Advance to bitoffset.
				for i := 0; i < bitOffset; i++ {
					_, err := bsr.readBit()
					require.NoError(t, err)
				}
				noChange, stDiff, err := readSTDiff(&bsr)
				require.NoError(t, err)
				require.False(t, noChange)
				require.Equal(t, v, stDiff)
			})
		}
	}
}
