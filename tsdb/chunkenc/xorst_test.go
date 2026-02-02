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
	testChunkSTHandling(t, ValFloat, func() Chunk {
		return NewXORSTChunk()
	},
	)
}

func TestXorOptSTChunk_MoreThan127Samples(t *testing.T) {
	const afterMax = maxFirstSTChangeOn + 3
	t.Run("zero ST", func(t *testing.T) {
		chunk := NewXORSTChunk()
		app, err := chunk.Appender()
		require.NoError(t, err)
		for i := range afterMax {
			newChunk, _ := app.Append(0, int64(i*10+1), float64(i)*1.5)
			require.Nil(t, newChunk)
		}

		it := chunk.Iterator(nil)
		for i := range afterMax {
			require.Equal(t, ValFloat, it.Next())
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
		chunk := NewXORSTChunk()
		app, err := chunk.Appender()
		require.NoError(t, err)
		for i := range afterMax {
			st := int64(0)
			if i == afterMax-1 {
				st = int64((afterMax - 1) * 10)
			}
			newChunk, _ := app.Append(st, int64(i*10+1), float64(i)*1.5)
			require.Nil(t, newChunk)
		}

		it := chunk.Iterator(nil)
		for i := range afterMax {
			require.Equal(t, ValFloat, it.Next())
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
	firstSTChangeOn := readSTHeader(b)
	require.Equal(t, uint8(0), firstSTChangeOn)

	b = make([]byte, 1)
	writeHeaderFirstSTChangeOn(b, 1)
	firstSTChangeOn = readSTHeader(b)
	require.Equal(t, uint8(1), firstSTChangeOn)

	b = make([]byte, 1)
	writeHeaderFirstSTChangeOn(b, 119)
	firstSTChangeOn = readSTHeader(b)
	require.Equal(t, uint8(119), firstSTChangeOn)
}
