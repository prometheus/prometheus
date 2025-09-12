// Copyright 2017 The Prometheus Authors
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

// TestWriterSegmentFileSize demonstrates that WriteChunks accurately estimates chunk sizes,
// and avoids unnecessary segment splits.
func TestWriterSegmentFileSize(t *testing.T) {
	chunkMeta1, err := ChunkFromSamples([]Sample{
		sample{t: 10, f: 11},
	})
	require.NoError(t, err)

	chunkMeta2, err := ChunkFromSamples([]Sample{
		sample{t: 20, f: 11},
	})
	require.NoError(t, err)

	// First, write both chunks to a segment with a large enough size.
	dir1 := t.TempDir()
	writer1, err := NewWriter(dir1, WithSegmentSize(100)) // Large enough to hold both chunks.
	require.NoError(t, err)
	require.NoError(t, writer1.WriteChunks(chunkMeta1, chunkMeta2))
	require.NoError(t, writer1.Close())

	files, err := os.ReadDir(dir1)
	require.NoError(t, err)
	require.Len(t, files, 1, "one segment file should contain both chunks")

	// Get the actual size of the created segment file.
	segmentInfo, err := os.Stat(dir1 + "/000001")
	require.NoError(t, err)
	actualSegmentFileSize := segmentInfo.Size()

	// Now, use the actual segment file size as the limit: both chunks still fit.
	dir2 := t.TempDir()
	writer2, err := NewWriter(dir2, WithSegmentSize(actualSegmentFileSize))
	require.NoError(t, err)
	require.NoError(t, writer2.WriteChunks(chunkMeta1, chunkMeta2))
	require.NoError(t, writer2.Close())

	files2, err := os.ReadDir(dir2)
	require.NoError(t, err)
	require.Len(t, files2, 1, "one segment file should contain both chunks")
}

func TestChunkSizeInBytes(t *testing.T) {
	chunkMetaFloat, err := ChunkFromSamples([]Sample{
		sample{t: 10, f: 11},
		sample{t: 20, f: 12},
		sample{t: 30, f: 13},
	})
	require.NoError(t, err)

	chunkMetaHistogram, err := ChunkFromSamples([]Sample{
		sample{t: 40, h: tsdbutil.GenerateTestHistogram(1)},
		sample{t: 50, h: tsdbutil.GenerateTestHistogram(2)},
		sample{t: 60, h: tsdbutil.GenerateTestHistogram(3)},
	})
	require.NoError(t, err)

	chunkMetaFloatHistogram, err := ChunkFromSamples([]Sample{
		sample{t: 70, fh: tsdbutil.GenerateTestFloatHistogram(1)},
		sample{t: 80, fh: tsdbutil.GenerateTestFloatHistogram(2)},
		sample{t: 90, fh: tsdbutil.GenerateTestFloatHistogram(3)},
	})
	require.NoError(t, err)

	dir := t.TempDir()

	w, err := NewWriter(dir, WithSegmentSize(1024)) // Use a small segment size to avoid allocating 512MB on disk for this test.
	require.NoError(t, err)

	err = w.WriteChunks(chunkMetaFloat, chunkMetaHistogram, chunkMetaFloatHistogram)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	segmentFile := dir + "/000001"
	segmentInfo, err := os.Stat(segmentFile)
	require.NoError(t, err)
	actualSegmentFileSize := segmentInfo.Size()

	varintBuffer := make([]byte, MaxChunkLengthFieldSize)
	chunkFloatSize := EncodedSizeInBytes(chunkMetaFloat.Chunk, varintBuffer)
	chunkHistogramSize := EncodedSizeInBytes(chunkMetaHistogram.Chunk, varintBuffer)
	chunkFloatHistogramSize := EncodedSizeInBytes(chunkMetaFloatHistogram.Chunk, varintBuffer)
	expectedSegmentFileSize := SegmentHeaderSize + chunkFloatSize + chunkHistogramSize + chunkFloatHistogramSize
	require.Equal(t, expectedSegmentFileSize, actualSegmentFileSize, "segment file size in bytes does not add up based on the chunk sizes")
}
