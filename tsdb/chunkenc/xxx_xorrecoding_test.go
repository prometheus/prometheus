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

type sample struct {
	ts, st int64
	f      float64
}

func TestXorRecodingChunk(t *testing.T) {
	t.Run("manual for debugging", func(t *testing.T) {
		samples := []sample{
			{ts: 1000, st: 0, f: 1.5},
			{ts: 2000, st: 0, f: 2.5},
			{ts: 3000, st: 0, f: 3.5},
			{ts: 4000, st: 0, f: 4.5},
		}
		chunk := NewXorRecodingChunk()
		app, err := chunk.AppenderV2()
		require.NoError(t, err)
		for _, s := range samples {
			app.Append(s.st, s.ts, s.f)
		}

		it := chunk.Iterator(nil)
		for i, s := range samples {
			require.Equal(t, it.Next(), ValFloat)
			ts, f := it.At()
			st := it.AtST()
			require.Equal(t, s.ts, ts, fmt.Sprintf("%d: timestamp mismatch", i))
			require.Equal(t, s.st, st, fmt.Sprintf("%d: start time mismatch", i))
			require.InDelta(t, s.f, f, 1e-9, fmt.Sprintf("%d: value mismatch", i))
		}
		require.Equal(t, it.Next(), ValNone)
		require.NoError(t, it.Err())
	})

	stTimes := []int64{0, 500, 1500, 2500, 3500}
	for numberOfSamples := range 5 {
		samples := make([]sample, numberOfSamples)
		sampleSTidx := make([]int, numberOfSamples)
		for {
			for j := 0; j < numberOfSamples; j++ {
				samples[j] = sample{
					ts: int64(1000 * (j + 1)),
					st: stTimes[sampleSTidx[j]],
					f:  float64(j) + 0.5,
				}
			}

			t.Run(fmt.Sprintf("%v", samples), func(t *testing.T) {
				chunk := NewXorRecodingChunk()
				app, err := chunk.AppenderV2()
				require.NoError(t, err)
				for _, s := range samples {
					app.Append(s.st, s.ts, s.f)
				}
				it := chunk.Iterator(nil)
				for i, s := range samples {
					require.Equal(t, it.Next(), ValFloat)
					ts, f := it.At()
					st := it.AtST()
					require.Equal(t, s.ts, ts, fmt.Sprintf("%d: timestamp mismatch", i))
					require.Equal(t, s.st, st, fmt.Sprintf("%d: start time mismatch", i))
					require.InDelta(t, s.f, f, 1e-9, fmt.Sprintf("%d: value mismatch", i))
				}
				require.Equal(t, it.Next(), ValNone)
				require.NoError(t, it.Err())
			})

			exhausted := true
			for j := numberOfSamples - 1; j >= 0; j-- {
				if sampleSTidx[j] < j+1 {
					sampleSTidx[j]++
					exhausted = false
					break
				}
				sampleSTidx[j] = 0
			}
			if exhausted {
				break
			}
		}
	}
}
