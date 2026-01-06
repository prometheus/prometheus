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
	"math"
	"testing"

	"github.com/stretchr/testify/require"
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
