// Copyright 2025 The Prometheus Authors
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

func TestSTHeader(t *testing.T) {
	b := make([]byte, 1)
	require.True(t, updateSTHeader(b, false, 120, 120))
	stZeroInitially, stSameUntil := readSTHeader(b, 120)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(120), stSameUntil)

	require.True(t, updateSTHeader(b, true, 120, 120))
	stZeroInitially, stSameUntil = readSTHeader(b, 120)
	require.True(t, stZeroInitially)
	require.Equal(t, uint16(120), stSameUntil)

	require.False(t, updateSTHeader(b, false, 20, 120))
	stZeroInitially, stSameUntil = readSTHeader(b, 120)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(20), stSameUntil)

	require.False(t, updateSTHeader(b, true, 20, 120))
	stZeroInitially, stSameUntil = readSTHeader(b, 120)
	require.True(t, stZeroInitially)
	require.Equal(t, uint16(20), stSameUntil)

	require.False(t, updateSTHeader(b, true, 21, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(21), stSameUntil)

	require.False(t, updateSTHeader(b, true, 19, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(19), stSameUntil)

	require.False(t, updateSTHeader(b, true, 1, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(1), stSameUntil)

	require.False(t, updateSTHeader(b, true, 2, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(2), stSameUntil)

	require.False(t, updateSTHeader(b, true, 119, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(119), stSameUntil)

	require.False(t, updateSTHeader(b, true, 127, 128))
	_, stSameUntil = readSTHeader(b, 128)
	require.Equal(t, uint16(127), stSameUntil)

	// Not full chunks works fine.
	require.True(t, updateSTHeader(b, false, 21, 21))
	stZeroInitially, stSameUntil = readSTHeader(b, 21)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(21), stSameUntil)

	require.False(t, updateSTHeader(b, false, 21, 90))
	stZeroInitially, stSameUntil = readSTHeader(b, 90)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(21), stSameUntil)

	require.False(t, updateSTHeader(b, true, 19, 90))
	stZeroInitially, stSameUntil = readSTHeader(b, 90)
	require.True(t, stZeroInitially)
	require.Equal(t, uint16(19), stSameUntil)

	require.False(t, updateSTHeader(b, false, 4, 127))
	stZeroInitially, stSameUntil = readSTHeader(b, 127)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(4), stSameUntil)

	require.False(t, updateSTHeader(b, false, 4, 129))
	stZeroInitially, stSameUntil = readSTHeader(b, 129)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(4), stSameUntil)

	require.False(t, updateSTHeader(b, false, 4, 130))
	stZeroInitially, stSameUntil = readSTHeader(b, 131)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(4), stSameUntil)

	require.False(t, updateSTHeader(b, false, 4, 130))
	stZeroInitially, stSameUntil = readSTHeader(b, 131)
	require.False(t, stZeroInitially)
	require.Equal(t, uint16(4), stSameUntil)

	require.False(t, updateSTHeader(b, true, 127, 9999))
	_, stSameUntil = readSTHeader(b, 9999)
	require.Equal(t, uint16(127), stSameUntil)

	// Wrong/odd inputs.
	require.False(t, updateSTHeader(b, true, 0, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(1), stSameUntil)

	require.False(t, updateSTHeader(b, true, 200, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(120), stSameUntil)

	require.False(t, updateSTHeader(b, true, math.MaxUint16, 120))
	_, stSameUntil = readSTHeader(b, 120)
	require.Equal(t, uint16(120), stSameUntil)

	require.False(t, updateSTHeader(b, true, 20, math.MaxUint16))
	_, stSameUntil = readSTHeader(b, math.MaxUint16)
	require.Equal(t, uint16(20), stSameUntil)
}
