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

package chunks

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestReaderWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})
	r := &Reader{bs: []ByteSlice{b}}

	_, _, err := r.ChunkOrIterable(Meta{Ref: 0})
	require.Error(t, err)
}

func TestWriterWithDefaultSegmentSize(t *testing.T) {
	chk1, err := ChunkFromSamples([]Sample{
		sample{t: 10, f: 11},
		sample{t: 20, f: 12},
		sample{t: 30, f: 13},
	})
	require.NoError(t, err)

	chk2, err := ChunkFromSamples([]Sample{
		sample{t: 40, h: tsdbutil.GenerateTestHistogram(1)},
		sample{t: 50, h: tsdbutil.GenerateTestHistogram(2)},
		sample{t: 60, h: tsdbutil.GenerateTestHistogram(3)},
	})
	require.NoError(t, err)

	dir := t.TempDir()
	w, err := NewWriter(dir)
	require.NoError(t, err)

	err = w.WriteChunks(chk1, chk2)
	require.NoError(t, err)

	require.NoError(t, w.Close())

	d, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, d, 1, "expected only one segment to be created to hold both chunks")
}

func TestChunkFromSamplesGenericWithST(t *testing.T) {
	testCases := []struct {
		name    string
		samples []Sample
	}{
		{
			name: "all samples have ST==0",
			samples: []Sample{
				sample{t: 10, f: 1.0},
				sample{t: 20, f: 2.0},
				sample{t: 30, f: 3.0},
				sample{t: 40, f: 4.0},
			},
		},
		{
			name: "all samples have ST!=0",
			samples: []Sample{
				sample{st: 5, t: 10, f: 1.0},
				sample{st: 15, t: 20, f: 2.0},
				sample{st: 25, t: 30, f: 3.0},
				sample{st: 35, t: 40, f: 4.0},
			},
		},
		{
			name: "half samples have ST and half not",
			samples: []Sample{
				sample{t: 10, f: 1.0},
				sample{st: 15, t: 20, f: 2.0},
				sample{t: 30, f: 3.0},
				sample{st: 35, t: 40, f: 4.0},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := ChunkFromSamplesGeneric(SampleSlice(tc.samples))
			require.NoError(t, err)
			require.NotNil(t, meta.Chunk, "expected a single chunk to be returned")

			// Verify MinTime and MaxTime
			require.Equal(t, tc.samples[0].T(), meta.MinTime)
			require.Equal(t, tc.samples[len(tc.samples)-1].T(), meta.MaxTime)

			// Iterate through the chunk and verify values
			it := meta.Chunk.Iterator(nil)
			idx := 0
			for it.Next() != chunkenc.ValNone {
				ts, val := it.At()
				st := it.AtST()
				require.Equal(t, tc.samples[idx].ST(), st, "ST mismatch at index %d", idx)
				require.Equal(t, tc.samples[idx].T(), ts, "T mismatch at index %d", idx)
				require.Equal(t, tc.samples[idx].F(), val, "F mismatch at index %d", idx)
				idx++
			}
			require.NoError(t, it.Err())
			require.Equal(t, len(tc.samples), idx, "number of samples mismatch")
		})
	}
}
