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

//go:build dedupelabels

package labels

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var expectedSizeOfLabels = []uint64{ // Values must line up with testCaseLabels.
	16,
	0,
	41,
	270,
	271,
	325,
}

var expectedByteSize = []uint64{ // Values must line up with testCaseLabels.
	8,
	0,
	8,
	8,
	8,
	32,
}

func TestVarint(t *testing.T) {
	cases := []struct {
		v        int
		expected []byte
	}{
		{0, []byte{0, 0}},
		{1, []byte{1, 0}},
		{2, []byte{2, 0}},
		{0x7FFF, []byte{0xFF, 0x7F}},
		{0x8000, []byte{0x00, 0x80, 0x01}},
		{0x8001, []byte{0x01, 0x80, 0x01}},
		{0x3FFFFF, []byte{0xFF, 0xFF, 0x7F}},
		{0x400000, []byte{0x00, 0x80, 0x80, 0x01}},
		{0x400001, []byte{0x01, 0x80, 0x80, 0x01}},
		{0x1FFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0x7F}},
	}
	var buf [16]byte
	for _, c := range cases {
		n := encodeVarint(buf[:], len(buf), c.v)
		require.Equal(t, len(c.expected), len(buf)-n)
		require.Equal(t, c.expected, buf[n:])
		got, m := decodeVarint(string(buf[:]), n)
		require.Equal(t, c.v, got)
		require.Equal(t, len(buf), m)
	}
	require.Panics(t, func() { encodeVarint(buf[:], len(buf), 1<<29) })
}
