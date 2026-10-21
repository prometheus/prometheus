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

package encoding

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecbufUvarintBytes(t *testing.T) {
	const payload = "abc"
	for _, tc := range []struct {
		name   string
		length uint64
	}{
		{name: "one byte beyond the buffer", length: uint64(len(payload)) + 1},
		// int(length) is negative on 32-bit platforms.
		{name: "above MaxInt32", length: math.MaxInt32 + 1},
		// int(length) wraps to a length within the buffer on 32-bit platforms.
		{name: "wraps to a valid length on 32-bit", length: math.MaxUint32 + 1 + uint64(len(payload)) - 1},
		// int(length) is negative on 64-bit platforms.
		{name: "above MaxInt64", length: math.MaxInt64 + 1},
		{name: "MaxUint64", length: math.MaxUint64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Encbuf{}
			e.PutUvarint64(tc.length)
			e.PutString(payload)
			d := Decbuf{B: e.Get()}
			require.Empty(t, d.UvarintBytes())
			require.ErrorIs(t, d.Err(), ErrInvalidSize)

			d = Decbuf{B: e.Get()}
			require.Empty(t, d.UvarintStr())
			require.ErrorIs(t, d.Err(), ErrInvalidSize)
		})
	}

	t.Run("exact length", func(t *testing.T) {
		e := Encbuf{}
		e.PutUvarintStr(payload)
		e.PutByte(1)
		d := Decbuf{B: e.Get()}
		require.Equal(t, payload, d.UvarintStr())
		require.NoError(t, d.Err())
		require.Equal(t, 1, d.Len())
	})
}
