// Copyright 2021 The Prometheus Authors
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

package tsdb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestBoundedChunk(t *testing.T) {
	tests := []struct {
		name           string
		inputChunk     chunkenc.Chunk
		inputMinT      int64
		inputMaxT      int64
		initialSeek    int64
		seekIsASuccess bool
		expSamples     []sample
	}{
		{
			name:       "if there are no samples it returns nothing",
			inputChunk: newTestChunk(0),
			expSamples: nil,
		},
		{
			name:       "bounds represent a single sample",
			inputChunk: newTestChunk(10),
			expSamples: []sample{
				{0, 0, nil, nil},
			},
		},
		{
			name:       "if there are bounds set only samples within them are returned",
			inputChunk: newTestChunk(10),
			inputMinT:  1,
			inputMaxT:  8,
			expSamples: []sample{
				{1, 1, nil, nil},
				{2, 2, nil, nil},
				{3, 3, nil, nil},
				{4, 4, nil, nil},
				{5, 5, nil, nil},
				{6, 6, nil, nil},
				{7, 7, nil, nil},
				{8, 8, nil, nil},
			},
		},
		{
			name:       "if bounds set and only maxt is less than actual maxt",
			inputChunk: newTestChunk(10),
			inputMinT:  0,
			inputMaxT:  5,
			expSamples: []sample{
				{0, 0, nil, nil},
				{1, 1, nil, nil},
				{2, 2, nil, nil},
				{3, 3, nil, nil},
				{4, 4, nil, nil},
				{5, 5, nil, nil},
			},
		},
		{
			name:       "if bounds set and only mint is more than actual mint",
			inputChunk: newTestChunk(10),
			inputMinT:  5,
			inputMaxT:  9,
			expSamples: []sample{
				{5, 5, nil, nil},
				{6, 6, nil, nil},
				{7, 7, nil, nil},
				{8, 8, nil, nil},
				{9, 9, nil, nil},
			},
		},
		{
			name:           "if there are bounds set with seek before mint",
			inputChunk:     newTestChunk(10),
			inputMinT:      3,
			inputMaxT:      7,
			initialSeek:    1,
			seekIsASuccess: true,
			expSamples: []sample{
				{3, 3, nil, nil},
				{4, 4, nil, nil},
				{5, 5, nil, nil},
				{6, 6, nil, nil},
				{7, 7, nil, nil},
			},
		},
		{
			name:           "if there are bounds set with seek between mint and maxt",
			inputChunk:     newTestChunk(10),
			inputMinT:      3,
			inputMaxT:      7,
			initialSeek:    5,
			seekIsASuccess: true,
			expSamples: []sample{
				{5, 5, nil, nil},
				{6, 6, nil, nil},
				{7, 7, nil, nil},
			},
		},
		{
			name:           "if there are bounds set with seek after maxt",
			inputChunk:     newTestChunk(10),
			inputMinT:      3,
			inputMaxT:      7,
			initialSeek:    8,
			seekIsASuccess: false,
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("name=%s", tc.name), func(t *testing.T) {
			chunk := boundedChunk{tc.inputChunk, tc.inputMinT, tc.inputMaxT}

			// Testing Bytes()
			expChunk := chunkenc.NewXORChunk()
			if tc.inputChunk.NumSamples() > 0 {
				app, err := expChunk.Appender()
				require.NoError(t, err)
				for ts := tc.inputMinT; ts <= tc.inputMaxT; ts++ {
					app.Append(ts, float64(ts))
				}
			}
			require.Equal(t, expChunk.Bytes(), chunk.Bytes())

			var samples []sample
			it := chunk.Iterator(nil)

			if tc.initialSeek != 0 {
				// Testing Seek()
				val := it.Seek(tc.initialSeek)
				require.Equal(t, tc.seekIsASuccess, val == chunkenc.ValFloat)
				if val == chunkenc.ValFloat {
					t, v := it.At()
					samples = append(samples, sample{t, v, nil, nil})
				}
			}

			// Testing Next()
			for it.Next() == chunkenc.ValFloat {
				t, v := it.At()
				samples = append(samples, sample{t, v, nil, nil})
			}

			// it.Next() should keep returning no  value.
			for i := 0; i < 10; i++ {
				require.True(t, it.Next() == chunkenc.ValNone)
			}

			require.Equal(t, tc.expSamples, samples)
		})
	}
}

func newTestChunk(numSamples int) chunkenc.Chunk {
	xor := chunkenc.NewXORChunk()
	a, _ := xor.Appender()
	for i := 0; i < numSamples; i++ {
		a.Append(int64(i), float64(i))
	}
	return xor
}
