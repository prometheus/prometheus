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
				{0, 0},
			},
		},
		{
			name:       "if there are bounds set only samples within them are returned",
			inputChunk: newTestChunk(10),
			inputMinT:  1,
			inputMaxT:  8,
			expSamples: []sample{
				{1, 1},
				{2, 2},
				{3, 3},
				{4, 4},
				{5, 5},
				{6, 6},
				{7, 7},
				{8, 8},
			},
		},
		{
			name:       "if bounds set and only maxt is less than actual maxt",
			inputChunk: newTestChunk(10),
			inputMinT:  0,
			inputMaxT:  5,
			expSamples: []sample{
				{0, 0},
				{1, 1},
				{2, 2},
				{3, 3},
				{4, 4},
				{5, 5},
			},
		},
		{
			name:       "if bounds set and only mint is more than actual mint",
			inputChunk: newTestChunk(10),
			inputMinT:  5,
			inputMaxT:  9,
			expSamples: []sample{
				{5, 5},
				{6, 6},
				{7, 7},
				{8, 8},
				{9, 9},
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
				{3, 3},
				{4, 4},
				{5, 5},
				{6, 6},
				{7, 7},
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
				{5, 5},
				{6, 6},
				{7, 7},
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
				ok := it.Seek(tc.initialSeek)
				require.Equal(t, tc.seekIsASuccess, ok)
				if ok {
					t, v := it.At()
					samples = append(samples, sample{t, v})
				}
			}

			// Testing Next()
			for it.Next() {
				t, v := it.At()
				samples = append(samples, sample{t, v})
			}

			// it.Next() should keep returning false.
			for i := 0; i < 10; i++ {
				require.False(t, it.Next())
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
