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

func TestChunkFromSamplesWithST(t *testing.T) {
	// Create samples with explicit ST (source timestamp) values.
	samples := []Sample{
		sample{t: 10, f: 11, st: 5},
		sample{t: 20, f: 12, st: 15},
		sample{t: 30, f: 13, st: 25},
	}

	chk, err := ChunkFromSamples(samples)
	require.NoError(t, err)
	require.NotNil(t, chk.Chunk)

	// Verify MinTime and MaxTime.
	require.Equal(t, int64(10), chk.MinTime)
	require.Equal(t, int64(30), chk.MaxTime)

	// Iterate over the chunk and verify ST values are preserved.
	it := chk.Chunk.Iterator(nil)
	idx := 0
	for vt := it.Next(); vt != chunkenc.ValNone; vt = it.Next() {
		require.Equal(t, chunkenc.ValFloat, vt)
		ts, v := it.At()
		st := it.AtST()
		require.Equal(t, samples[idx].ST(), st, "ST mismatch at index %d", idx)
		require.Equal(t, samples[idx].T(), ts, "T mismatch at index %d", idx)
		require.Equal(t, samples[idx].F(), v, "F mismatch at index %d", idx)
		idx++
	}
	require.NoError(t, it.Err())
	require.Equal(t, len(samples), idx, "expected all samples to be iterated")
}
