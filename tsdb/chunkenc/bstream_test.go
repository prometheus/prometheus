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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBstream_Reset(t *testing.T) {
	bs := bstream{
		stream: []byte("test"),
		count:  10,
	}
	bs.Reset([]byte("was reset"))

	require.Equal(t, bstream{
		stream: []byte("was reset"),
		count:  0,
	}, bs)
}

func TestBstreamReader(t *testing.T) {
	// Write to the bit stream.
	w := bstream{}
	for _, bit := range []bit{true, false} {
		w.writeBit(bit)
	}
	for nbits := 1; nbits <= 64; nbits++ {
		w.writeBits(uint64(nbits), nbits)
	}
	for v := 1; v < 10000; v += 123 {
		w.writeBits(uint64(v), 29)
	}

	// Read back.
	r := newBReader(w.bytes())
	for _, bit := range []bit{true, false} {
		v, err := r.readBitFast()
		if err != nil {
			v, err = r.readBit()
		}
		require.NoError(t, err)
		require.Equal(t, bit, v)
	}
	for nbits := uint8(1); nbits <= 64; nbits++ {
		v, err := r.readBitsFast(nbits)
		if err != nil {
			v, err = r.readBits(nbits)
		}
		require.NoError(t, err)
		require.Equal(t, uint64(nbits), v, "nbits=%d", nbits)
	}
	for v := 1; v < 10000; v += 123 {
		actual, err := r.readBitsFast(29)
		if err != nil {
			actual, err = r.readBits(29)
		}
		require.NoError(t, err)
		require.Equal(t, uint64(v), actual, "v=%d", v)
	}
}
