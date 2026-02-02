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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkXorRead(b *testing.B) {
	c := NewXORChunk()
	app, err := c.Appender()
	require.NoError(b, err)
	for i := int64(0); i < 120*1000; i += 1000 {
		newChunk, _ := app.Append(0, i, float64(i)+float64(i)/10+float64(i)/100+float64(i)/1000)
		require.Nil(b, newChunk)
	}

	b.ReportAllocs()

	var it Iterator
	for b.Loop() {
		var ts int64
		var v float64
		it = c.Iterator(it)
		for it.Next() != ValNone {
			ts, v = it.At()
		}
		_, _ = ts, v
	}
}

func TestXORChunk_AppendST(t *testing.T) {
	for stStartAt := range 5 {
		t.Run(fmt.Sprintf("start ST at sample %d", stStartAt), func(t *testing.T) {
			c := NewXORChunk()
			chunks := []Chunk{c}
			app, err := c.Appender()
			require.NoError(t, err)

			timestamp := func(i int) int64 { return int64((i + 1) * 5000) }
			st := func(i int) int64 {
				if i >= stStartAt {
					return 1000
				}
				return 0
			}
			for i := range 4 {
				newChunk, newApp := app.Append(st(i), timestamp(i), float64(i))
				if i == stStartAt {
					require.NotNil(t, newChunk, "expected new chunk allocation")
					require.NotEqual(t, app, newApp, "expected new app allocation")
					app = newApp
					chunks = append(chunks, newChunk)
				} else {
					require.Nil(t, newChunk, "unexpected new chunk allocation")
					require.Equal(t, app, newApp, "unexpected new app allocation")
				}
			}
			if stStartAt < 4 {
				require.Len(t, chunks, 2, "expected two chunks to be created")
			} else {
				require.Len(t, chunks, 1, "expected only one chunk to be created")
			}
			// Verify samples.
			count := 0
			for _, chk := range chunks {
				var it Iterator
				it = chk.Iterator(it)
				for it.Next() != ValNone {
					ts, v := it.At()
					require.Equal(t, float64(count), v, "value mismatch at timestamp %d", ts)
					require.Equal(t, timestamp(count), ts, "timestamp mismatch at count %d", count)
					require.Equal(t, st(count), it.AtST(), "ST mismatch at timestamp %d", ts)
					count++
				}
			}
		})
	}
}
