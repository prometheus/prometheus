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

func TestXorOptSTChunk(t *testing.T) {
	testChunkAppenderV2ST(
		t,
		func() Chunk {
			return NewXOROptSTChunk()
		},
		func(c Chunk) (AppenderV2, error) {
			s := c.(*xorOptSTChunk)
			return s.AppenderV2()
		},
	)
}

func TestXorOptSTChunk_MoreThan127Samples(t *testing.T) {
	const afterMax = maxFirstSTChangeOn + 3
	t.Run("zero ST", func(t *testing.T) {
		chunk := NewXOROptSTChunk()
		app, err := chunk.AppenderV2()
		require.NoError(t, err)
		for i := 0; i < afterMax; i++ {
			app.Append(0, int64(i*10+1), float64(i)*1.5)
		}

		it := chunk.Iterator(nil)
		for i := 0; i < afterMax; i++ {
			require.Equal(t, it.Next(), ValFloat)
			st := it.AtST()
			ts, v := it.At()
			require.Equal(t, int64(0), st)
			require.Equal(t, int64(i*10+1), ts)
			require.Equal(t, float64(i)*1.5, v)
		}

		require.Equal(t, ValNone, it.Next())
		require.NoError(t, it.Err())
	})

	t.Run("non-zero ST after 127", func(t *testing.T) {
		chunk := NewXOROptSTChunk()
		app, err := chunk.AppenderV2()
		require.NoError(t, err)
		for i := 0; i < afterMax; i++ {
			st := int64(0)
			if i == afterMax-1 {
				st = int64((afterMax - 1) * 10)
			}
			app.Append(st, int64(i*10+1), float64(i)*1.5)
		}

		it := chunk.Iterator(nil)
		for i := 0; i < afterMax; i++ {
			require.Equal(t, it.Next(), ValFloat)
			st := it.AtST()
			ts, v := it.At()
			if i == afterMax-1 {
				require.Equal(t, int64((afterMax-1)*10), st)
			} else {
				require.Equal(t, int64(0), st)
			}
			require.Equal(t, int64(i*10+1), ts)
			require.Equal(t, float64(i)*1.5, v)
		}

		require.Equal(t, ValNone, it.Next())
		require.NoError(t, it.Err())
	})
}

func TestXorOptSTChunk_STHeader(t *testing.T) {
	b := make([]byte, 1)
	writeHeaderFirstSTKnown(b)
	firstSTKnown, firstSTChangeOn := readSTHeader(b)
	require.True(t, firstSTKnown)
	require.Equal(t, uint16(0), firstSTChangeOn)

	b = make([]byte, 1)
	firstSTKnown, firstSTChangeOn = readSTHeader(b)
	require.False(t, firstSTKnown)
	require.Equal(t, uint16(0), firstSTChangeOn)

	b = make([]byte, 1)
	writeHeaderFirstSTChangeOn(b, 1)
	firstSTKnown, firstSTChangeOn = readSTHeader(b)
	require.False(t, firstSTKnown)
	require.Equal(t, uint16(1), firstSTChangeOn)

	b = make([]byte, 1)
	writeHeaderFirstSTKnown(b)
	writeHeaderFirstSTChangeOn(b, 119)
	firstSTKnown, firstSTChangeOn = readSTHeader(b)
	require.True(t, firstSTKnown)
	require.Equal(t, uint16(119), firstSTChangeOn)
}
