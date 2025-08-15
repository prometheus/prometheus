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

	w, err := NewWriter(dir, WithSegmentSize(1024)) // use a small segment size to avoid allocating 512MB on disk for this test
	require.NoError(t, err)

	err = w.WriteChunks(chunkMetaFloat, chunkMetaHistogram, chunkMetaFloatHistogram)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	segmentFile := dir + "/000001"
	segmentInfo, err := os.Stat(segmentFile)
	require.NoError(t, err)

	varintBuffer := make([]byte, MaxChunkLengthFieldSize)
	actualSegmentFileSize := segmentInfo.Size()
	chunkFloatSize := ChunkSizeInBytes(chunkMetaFloat.Chunk, varintBuffer)
	chunkHistogramSize := ChunkSizeInBytes(chunkMetaHistogram.Chunk, varintBuffer)
	chunkFloatHistogramSize := ChunkSizeInBytes(chunkMetaFloatHistogram.Chunk, varintBuffer)
	expectedSegmentFileSize := SegmentHeaderSize + chunkFloatSize + chunkHistogramSize + chunkFloatHistogramSize
	require.Equal(t, int64(expectedSegmentFileSize), actualSegmentFileSize, "segment file size in bytes does not add up based on the chunk sizes")
}

func TestWriterSegmentFileSize(t *testing.T) {
	chunkMeta1, err := ChunkFromSamples([]Sample{
		sample{t: 10, f: 11},
	})
	require.NoError(t, err)

	chunkMeta2, err := ChunkFromSamples([]Sample{
		sample{t: 20, f: 11},
	})
	require.NoError(t, err)

	varintBuffer := make([]byte, MaxChunkLengthFieldSize)
	chunkFloatSize1 := ChunkSizeInBytes(chunkMeta1.Chunk, varintBuffer)
	chunkFloatSize2 := ChunkSizeInBytes(chunkMeta2.Chunk, varintBuffer)

	dir := t.TempDir()

	// There is an estimation error in WriteChunks when the chunk size is calculated. The actual chunk length field
	// size is not considered but always assumed to be MaxChunkLengthFieldSize (5 bytes). In this test, chunkMeta1
	// and chunkMeta2 take 1 byte each for the chunk length field, so the estimation error is 8 bytes.
	estimationError := (MaxChunkLengthFieldSize - 1) + (MaxChunkLengthFieldSize - 1)
	w, err := NewWriter(dir, WithSegmentSize(int64(SegmentHeaderSize+chunkFloatSize1+chunkFloatSize2+estimationError)))
	require.NoError(t, err)

	err = w.WriteChunks(chunkMeta1, chunkMeta2)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1, "expected only one segment to be created to store both chunks")

	segmentInfo, err := os.Stat(dir + "/000001")
	require.NoError(t, err)

	actualSegmentFileSize := segmentInfo.Size()
	expectedSegmentFileSize := SegmentHeaderSize + chunkFloatSize1 + chunkFloatSize2
	require.Equal(t, int64(expectedSegmentFileSize), actualSegmentFileSize, "segment file size in bytes does not add up based on the chunk sizes")
}
