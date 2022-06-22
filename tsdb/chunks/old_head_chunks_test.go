// Copyright 2020 The Prometheus Authors
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
	"encoding/binary"
	"errors"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestOldChunkDiskMapper_WriteChunk_Chunk_IterateChunks(t *testing.T) {
	hrw := testOldChunkDiskMapper(t)
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	expectedBytes := []byte{}
	nextChunkOffset := uint64(HeadChunkFileHeaderSize)
	chkCRC32 := newCRC32()

	type expectedDataType struct {
		seriesRef  HeadSeriesRef
		chunkRef   ChunkDiskMapperRef
		mint, maxt int64
		numSamples uint16
		chunk      chunkenc.Chunk
		isOOO      bool
	}
	expectedData := []expectedDataType{}

	var buf [MaxHeadChunkMetaSize]byte
	totalChunks := 0
	var firstFileName string
	for hrw.curFileSequence < 3 || hrw.chkWriter.Buffered() == 0 {
		addChunks := func(numChunks int) {
			for i := 0; i < numChunks; i++ {
				seriesRef, chkRef, mint, maxt, chunk, isOOO := createChunkForOld(t, totalChunks, hrw)
				totalChunks++
				expectedData = append(expectedData, expectedDataType{
					seriesRef:  seriesRef,
					mint:       mint,
					maxt:       maxt,
					chunkRef:   chkRef,
					chunk:      chunk,
					numSamples: uint16(chunk.NumSamples()),
					isOOO:      isOOO,
				})

				if hrw.curFileSequence != 1 {
					// We are checking for bytes written only for the first file.
					continue
				}

				// Calculating expected bytes written on disk for first file.
				firstFileName = hrw.curFile.Name()
				require.Equal(t, newChunkDiskMapperRef(1, nextChunkOffset), chkRef)

				bytesWritten := 0
				chkCRC32.Reset()

				binary.BigEndian.PutUint64(buf[bytesWritten:], uint64(seriesRef))
				bytesWritten += SeriesRefSize
				binary.BigEndian.PutUint64(buf[bytesWritten:], uint64(mint))
				bytesWritten += MintMaxtSize
				binary.BigEndian.PutUint64(buf[bytesWritten:], uint64(maxt))
				bytesWritten += MintMaxtSize
				buf[bytesWritten] = byte(chunk.Encoding())
				bytesWritten += ChunkEncodingSize
				n := binary.PutUvarint(buf[bytesWritten:], uint64(len(chunk.Bytes())))
				bytesWritten += n

				expectedBytes = append(expectedBytes, buf[:bytesWritten]...)
				_, err := chkCRC32.Write(buf[:bytesWritten])
				require.NoError(t, err)
				expectedBytes = append(expectedBytes, chunk.Bytes()...)
				_, err = chkCRC32.Write(chunk.Bytes())
				require.NoError(t, err)

				expectedBytes = append(expectedBytes, chkCRC32.Sum(nil)...)

				// += seriesRef, mint, maxt, encoding, chunk data len, chunk data, CRC.
				nextChunkOffset += SeriesRefSize + 2*MintMaxtSize + ChunkEncodingSize + uint64(n) + uint64(len(chunk.Bytes())) + CRCSize
			}
		}
		addChunks(100)
		hrw.CutNewFile()
		addChunks(10) // For chunks in in-memory buffer.
	}

	// Checking on-disk bytes for the first file.
	require.Equal(t, 3, len(hrw.mmappedChunkFiles), "expected 3 mmapped files, got %d", len(hrw.mmappedChunkFiles))
	require.Equal(t, len(hrw.mmappedChunkFiles), len(hrw.closers))

	actualBytes, err := os.ReadFile(firstFileName)
	require.NoError(t, err)

	// Check header of the segment file.
	require.Equal(t, MagicHeadChunks, int(binary.BigEndian.Uint32(actualBytes[0:MagicChunksSize])))
	require.Equal(t, chunksFormatV1, int(actualBytes[MagicChunksSize]))

	// Remaining chunk data.
	fileEnd := HeadChunkFileHeaderSize + len(expectedBytes)
	require.Equal(t, expectedBytes, actualBytes[HeadChunkFileHeaderSize:fileEnd])

	// Testing reading of chunks.
	for _, exp := range expectedData {
		actChunk, err := hrw.Chunk(exp.chunkRef)
		require.NoError(t, err)
		require.Equal(t, exp.chunk.Bytes(), actChunk.Bytes())
	}

	// Testing IterateAllChunks method.
	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())
	hrw, err = NewOldChunkDiskMapper(dir, chunkenc.NewPool(), DefaultWriteBufferSize)
	require.NoError(t, err)

	idx := 0
	require.NoError(t, hrw.IterateAllChunks(func(seriesRef HeadSeriesRef, chunkRef ChunkDiskMapperRef, mint, maxt int64, numSamples uint16, encoding chunkenc.Encoding) error {
		t.Helper()

		expData := expectedData[idx]
		require.Equal(t, expData.seriesRef, seriesRef)
		require.Equal(t, expData.chunkRef, chunkRef)
		require.Equal(t, expData.maxt, maxt)
		require.Equal(t, expData.maxt, maxt)
		require.Equal(t, expData.numSamples, numSamples)
		require.Equal(t, expData.isOOO, chunkenc.IsOutOfOrderChunk(encoding))

		actChunk, err := hrw.Chunk(expData.chunkRef)
		require.NoError(t, err)
		require.Equal(t, expData.chunk.Bytes(), actChunk.Bytes())

		idx++
		return nil
	}))
	require.Equal(t, len(expectedData), idx)
}

func TestOldChunkDiskMapper_WriteUnsupportedChunk_Chunk_IterateChunks(t *testing.T) {
	hrw := testOldChunkDiskMapper(t)
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	ucSeriesRef, ucChkRef, ucMint, ucMaxt, uchunk := writeUnsupportedChunk(t, 0, nil, hrw)

	// Checking on-disk bytes for the first file.
	require.Equal(t, 1, len(hrw.mmappedChunkFiles), "expected 1 mmapped file, got %d", len(hrw.mmappedChunkFiles))
	require.Equal(t, len(hrw.mmappedChunkFiles), len(hrw.closers))

	// Testing IterateAllChunks method.
	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())
	hrw, err := NewOldChunkDiskMapper(dir, chunkenc.NewPool(), DefaultWriteBufferSize)
	require.NoError(t, err)

	require.NoError(t, hrw.IterateAllChunks(func(seriesRef HeadSeriesRef, chunkRef ChunkDiskMapperRef, mint, maxt int64, numSamples uint16, encoding chunkenc.Encoding) error {
		t.Helper()

		require.Equal(t, ucSeriesRef, seriesRef)
		require.Equal(t, ucChkRef, chunkRef)
		require.Equal(t, ucMint, mint)
		require.Equal(t, ucMaxt, maxt)
		require.Equal(t, uchunk.Encoding(), encoding) // Asserts that the encoding is EncUnsupportedXOR

		actChunk, err := hrw.Chunk(chunkRef)
		// The chunk encoding is unknown so Chunk() should fail but us the caller
		// are ok with that. Above we asserted that the encoding we expected was
		// EncUnsupportedXOR
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "invalid chunk encoding \"<unknown>\"")
		require.Nil(t, actChunk)

		return nil
	}))
}

// TestOldChunkDiskMapper_Truncate tests
// * If truncation is happening properly based on the time passed.
// * The active file is not deleted even if the passed time makes it eligible to be deleted.
// * Empty current file does not lead to creation of another file after truncation.
// * Non-empty current file leads to creation of another file after truncation.
func TestOldChunkDiskMapper_Truncate(t *testing.T) {
	hrw := testOldChunkDiskMapper(t)
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	timeRange := 0
	fileTimeStep := 100

	addChunk := func() {
		mint := timeRange + 1                // Just after the new file cut.
		maxt := timeRange + fileTimeStep - 1 // Just before the next file.

		// Write a chunks to set maxt for the segment.
		_ = hrw.WriteChunk(1, int64(mint), int64(maxt), randomChunk(t), func(err error) {
			require.NoError(t, err)
		})

		timeRange += fileTimeStep
	}

	verifyFiles := func(remainingFiles []int) {
		t.Helper()

		files, err := os.ReadDir(hrw.dir.Name())
		require.NoError(t, err)
		require.Equal(t, len(remainingFiles), len(files), "files on disk")
		require.Equal(t, len(remainingFiles), len(hrw.mmappedChunkFiles), "hrw.mmappedChunkFiles")
		require.Equal(t, len(remainingFiles), len(hrw.closers), "closers")

		for _, i := range remainingFiles {
			_, ok := hrw.mmappedChunkFiles[i]
			require.Equal(t, true, ok)
		}
	}

	// Create segments 1 to 7.
	for i := 1; i <= 7; i++ {
		require.NoError(t, hrw.CutNewFile())
		addChunk()
	}
	verifyFiles([]int{1, 2, 3, 4, 5, 6, 7})

	// Truncating files.
	require.NoError(t, hrw.Truncate(3))
	verifyFiles([]int{3, 4, 5, 6, 7, 8})

	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())

	// Restarted.
	var err error
	hrw, err = NewOldChunkDiskMapper(dir, chunkenc.NewPool(), DefaultWriteBufferSize)
	require.NoError(t, err)

	require.False(t, hrw.fileMaxtSet)
	require.NoError(t, hrw.IterateAllChunks(func(_ HeadSeriesRef, _ ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding) error {
		return nil
	}))
	require.True(t, hrw.fileMaxtSet)

	verifyFiles([]int{3, 4, 5, 6, 7, 8})
	// New file is created after restart even if last file was empty.
	addChunk()
	verifyFiles([]int{3, 4, 5, 6, 7, 8, 9})

	// Truncating files after restart.
	require.NoError(t, hrw.Truncate(6))
	verifyFiles([]int{6, 7, 8, 9, 10})

	// As the last file was empty, this creates no new files.
	require.NoError(t, hrw.Truncate(6))
	verifyFiles([]int{6, 7, 8, 9, 10})

	require.NoError(t, hrw.Truncate(8))
	verifyFiles([]int{8, 9, 10})

	addChunk()

	// Truncating till current time should not delete the current active file.
	require.NoError(t, hrw.Truncate(10))
	verifyFiles([]int{10, 11}) // One file is the previously active file and one currently created.
}

// TestOldChunkDiskMapper_Truncate_PreservesFileSequence tests that truncation doesn't poke
// holes into the file sequence, even if there are empty files in between non-empty files.
// This test exposes https://github.com/prometheus/prometheus/issues/7412 where the truncation
// simply deleted all empty files instead of stopping once it encountered a non-empty file.
func TestOldChunkDiskMapper_Truncate_PreservesFileSequence(t *testing.T) {
	hrw := testOldChunkDiskMapper(t)
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	timeRange := 0
	addChunk := func() {
		step := 100
		mint, maxt := timeRange+1, timeRange+step-1
		_ = hrw.WriteChunk(1, int64(mint), int64(maxt), randomChunk(t), func(err error) {
			require.NoError(t, err)
		})
		timeRange += step
	}
	emptyFile := func() {
		require.NoError(t, hrw.CutNewFile())
	}
	nonEmptyFile := func() {
		emptyFile()
		addChunk()
	}

	addChunk()     // 1. Created with the first chunk.
	nonEmptyFile() // 2.
	nonEmptyFile() // 3.
	emptyFile()    // 4.
	nonEmptyFile() // 5.
	emptyFile()    // 6.

	verifyFiles := func(remainingFiles []int) {
		t.Helper()

		files, err := os.ReadDir(hrw.dir.Name())
		require.NoError(t, err)
		require.Equal(t, len(remainingFiles), len(files), "files on disk")
		require.Equal(t, len(remainingFiles), len(hrw.mmappedChunkFiles), "hrw.mmappedChunkFiles")
		require.Equal(t, len(remainingFiles), len(hrw.closers), "closers")

		for _, i := range remainingFiles {
			_, ok := hrw.mmappedChunkFiles[i]
			require.True(t, ok, "remaining file %d not in hrw.mmappedChunkFiles", i)
		}
	}

	verifyFiles([]int{1, 2, 3, 4, 5, 6})

	// Truncating files till 2. It should not delete anything after 3 (inclusive)
	// though files 4 and 6 are empty.
	require.NoError(t, hrw.Truncate(3))
	// As 6 was empty, it should not create another file.
	verifyFiles([]int{3, 4, 5, 6})

	addChunk()
	// Truncate creates another file as 6 is not empty now.
	require.NoError(t, hrw.Truncate(3))
	verifyFiles([]int{3, 4, 5, 6, 7})

	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())

	// Restarting checks for unsequential files.
	var err error
	hrw, err = NewOldChunkDiskMapper(dir, chunkenc.NewPool(), DefaultWriteBufferSize)
	require.NoError(t, err)
	verifyFiles([]int{3, 4, 5, 6, 7})
}

// TestOldChunkDiskMapper_TruncateAfterFailedIterateChunks tests for
// https://github.com/prometheus/prometheus/issues/7753
func TestOldChunkDiskMapper_TruncateAfterFailedIterateChunks(t *testing.T) {
	hrw := testOldChunkDiskMapper(t)
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	// Write a chunks to iterate on it later.
	_ = hrw.WriteChunk(1, 0, 1000, randomChunk(t), func(err error) {
		require.NoError(t, err)
	})

	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())

	// Restarting to recreate https://github.com/prometheus/prometheus/issues/7753.
	hrw, err := NewOldChunkDiskMapper(dir, chunkenc.NewPool(), DefaultWriteBufferSize)
	require.NoError(t, err)

	// Forcefully failing IterateAllChunks.
	require.Error(t, hrw.IterateAllChunks(func(_ HeadSeriesRef, _ ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding) error {
		return errors.New("random error")
	}))

	// Truncation call should not return error after IterateAllChunks fails.
	require.NoError(t, hrw.Truncate(2000))
}

func TestOldChunkDiskMapper_ReadRepairOnEmptyLastFile(t *testing.T) {
	hrw := testOldChunkDiskMapper(t)
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	timeRange := 0
	addChunk := func() {
		step := 100
		mint, maxt := timeRange+1, timeRange+step-1
		_ = hrw.WriteChunk(1, int64(mint), int64(maxt), randomChunk(t), func(err error) {
			require.NoError(t, err)
		})
		timeRange += step
	}
	nonEmptyFile := func() {
		require.NoError(t, hrw.CutNewFile())
		addChunk()
	}

	addChunk()     // 1. Created with the first chunk.
	nonEmptyFile() // 2.
	nonEmptyFile() // 3.

	require.Equal(t, 3, len(hrw.mmappedChunkFiles))
	lastFile := 0
	for idx := range hrw.mmappedChunkFiles {
		if idx > lastFile {
			lastFile = idx
		}
	}
	require.Equal(t, 3, lastFile)
	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())

	// Write an empty last file mimicking an abrupt shutdown on file creation.
	emptyFileName := segmentFile(dir, lastFile+1)
	f, err := os.OpenFile(emptyFileName, os.O_WRONLY|os.O_CREATE, 0o666)
	require.NoError(t, err)
	require.NoError(t, f.Sync())
	stat, err := f.Stat()
	require.NoError(t, err)
	require.Equal(t, int64(0), stat.Size())
	require.NoError(t, f.Close())

	// Open chunk disk mapper again, corrupt file should be removed.
	hrw, err = NewOldChunkDiskMapper(dir, chunkenc.NewPool(), DefaultWriteBufferSize)
	require.NoError(t, err)
	require.False(t, hrw.fileMaxtSet)
	require.NoError(t, hrw.IterateAllChunks(func(_ HeadSeriesRef, _ ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding) error {
		return nil
	}))
	require.True(t, hrw.fileMaxtSet)

	// Removed from memory.
	require.Equal(t, 3, len(hrw.mmappedChunkFiles))
	for idx := range hrw.mmappedChunkFiles {
		require.LessOrEqual(t, idx, lastFile, "file index is bigger than previous last file")
	}

	// Removed even from disk.
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Equal(t, 3, len(files))
	for _, fi := range files {
		seq, err := strconv.ParseUint(fi.Name(), 10, 64)
		require.NoError(t, err)
		require.LessOrEqual(t, seq, uint64(lastFile), "file index on disk is bigger than previous last file")
	}
}

func testOldChunkDiskMapper(t *testing.T) *OldChunkDiskMapper {
	tmpdir := t.TempDir()

	hrw, err := NewOldChunkDiskMapper(tmpdir, chunkenc.NewPool(), DefaultWriteBufferSize)
	require.NoError(t, err)
	require.False(t, hrw.fileMaxtSet)
	require.NoError(t, hrw.IterateAllChunks(func(_ HeadSeriesRef, _ ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding) error {
		return nil
	}))
	require.True(t, hrw.fileMaxtSet)
	return hrw
}

func createChunkForOld(t *testing.T, idx int, hrw *OldChunkDiskMapper) (seriesRef HeadSeriesRef, chunkRef ChunkDiskMapperRef, mint, maxt int64, chunk chunkenc.Chunk, isOOO bool) {
	seriesRef = HeadSeriesRef(rand.Int63())
	mint = int64((idx)*1000 + 1)
	maxt = int64((idx + 1) * 1000)
	chunk = randomChunk(t)
	if rand.Intn(2) == 0 {
		isOOO = true
		chunk = &chunkenc.OOOXORChunk{XORChunk: chunk.(*chunkenc.XORChunk)}
	}
	chunkRef = hrw.WriteChunk(seriesRef, mint, maxt, chunk, func(err error) {
		require.NoError(t, err)
	})
	return
}
