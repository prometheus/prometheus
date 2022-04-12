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
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestChunkDiskMapper_WriteChunk_Chunk_IterateChunks(t *testing.T) {
	hrw := createChunkDiskMapper(t, "")
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
	}
	expectedData := []expectedDataType{}

	var buf [MaxHeadChunkMetaSize]byte
	totalChunks := 0
	var firstFileName string
	for hrw.curFileSequence < 3 || hrw.chkWriter.Buffered() == 0 {
		addChunks := func(numChunks int) {
			for i := 0; i < numChunks; i++ {
				seriesRef, chkRef, mint, maxt, chunk := createChunk(t, totalChunks, hrw)
				totalChunks++
				expectedData = append(expectedData, expectedDataType{
					seriesRef:  seriesRef,
					mint:       mint,
					maxt:       maxt,
					chunkRef:   chkRef,
					chunk:      chunk,
					numSamples: uint16(chunk.NumSamples()),
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
		require.NoError(t, hrw.CutNewFile())
		addChunks(10) // For chunks in in-memory buffer.
	}

	// Checking on-disk bytes for the first file.
	require.Equal(t, 3, len(hrw.mmappedChunkFiles), "expected 3 mmapped files, got %d", len(hrw.mmappedChunkFiles))
	require.Equal(t, len(hrw.mmappedChunkFiles), len(hrw.closers))

	actualBytes, err := ioutil.ReadFile(firstFileName)
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
	hrw = createChunkDiskMapper(t, dir)

	idx := 0
	require.NoError(t, hrw.IterateAllChunks(func(seriesRef HeadSeriesRef, chunkRef ChunkDiskMapperRef, mint, maxt int64, numSamples uint16, encoding chunkenc.Encoding) error {
		t.Helper()

		expData := expectedData[idx]
		require.Equal(t, expData.seriesRef, seriesRef)
		require.Equal(t, expData.chunkRef, chunkRef)
		require.Equal(t, expData.mint, mint)
		require.Equal(t, expData.maxt, maxt)
		require.Equal(t, expData.numSamples, numSamples)

		actChunk, err := hrw.Chunk(expData.chunkRef)
		require.NoError(t, err)
		require.Equal(t, expData.chunk.Bytes(), actChunk.Bytes())

		idx++
		return nil
	}))
	require.Equal(t, len(expectedData), idx)
}

func TestChunkDiskMapper_WriteUnsupportedChunk_Chunk_IterateChunks(t *testing.T) {
	hrw := createChunkDiskMapper(t, "")
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	ucSeriesRef, ucChkRef, ucMint, ucMaxt, uchunk := writeUnsupportedChunk(t, 0, hrw, nil)

	// Checking on-disk bytes for the first file.
	require.Equal(t, 1, len(hrw.mmappedChunkFiles), "expected 1 mmapped file, got %d", len(hrw.mmappedChunkFiles))
	require.Equal(t, len(hrw.mmappedChunkFiles), len(hrw.closers))

	// Testing IterateAllChunks method.
	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())
	hrw = createChunkDiskMapper(t, dir)

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

// TestChunkDiskMapper_Truncate tests
// * If truncation is happening properly based on the time passed.
// * The active file is not deleted even if the passed time makes it eligible to be deleted.
// * Non-empty current file leads to creation of another file after truncation.
func TestChunkDiskMapper_Truncate(t *testing.T) {
	hrw := createChunkDiskMapper(t, "")
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	timeRange := 0
	fileTimeStep := 100
	var thirdFileMinT, sixthFileMinT int64
	addChunk := func() int {
		t.Helper()

		step := 100
		mint, maxt := timeRange+1, timeRange+step-1
		var err error
		awaitCb := make(chan struct{})
		hrw.WriteChunk(1, int64(mint), int64(maxt), randomChunk(t), func(cbErr error) {
			err = cbErr
			close(awaitCb)
		})
		<-awaitCb
		require.NoError(t, err)
		timeRange += step

		return mint
	}

	verifyFiles := func(remainingFiles []int) {
		t.Helper()

		files, err := ioutil.ReadDir(hrw.dir.Name())
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
		mint := int64(addChunk())
		if i == 3 {
			thirdFileMinT = mint
		} else if i == 6 {
			sixthFileMinT = mint
		}
	}
	verifyFiles([]int{1, 2, 3, 4, 5, 6, 7})

	// Truncating files.
	require.NoError(t, hrw.Truncate(thirdFileMinT))

	// Add a chunk to trigger cutting of new file.
	addChunk()

	verifyFiles([]int{3, 4, 5, 6, 7, 8})

	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())

	// Restarted.
	hrw = createChunkDiskMapper(t, dir)

	verifyFiles([]int{3, 4, 5, 6, 7, 8})
	// New file is created after restart even if last file was empty.
	addChunk()
	verifyFiles([]int{3, 4, 5, 6, 7, 8, 9})

	// Truncating files after restart.
	require.NoError(t, hrw.Truncate(sixthFileMinT))
	verifyFiles([]int{6, 7, 8, 9})

	// Truncating a second time without adding a chunk shouldn't create a new file.
	require.NoError(t, hrw.Truncate(sixthFileMinT+1))
	verifyFiles([]int{6, 7, 8, 9})

	// Add a chunk to trigger cutting of new file.
	addChunk()

	verifyFiles([]int{6, 7, 8, 9, 10})

	// Truncating till current time should not delete the current active file.
	require.NoError(t, hrw.Truncate(int64(timeRange+(2*fileTimeStep))))

	// Add a chunk to trigger cutting of new file.
	addChunk()

	verifyFiles([]int{10, 11}) // One file is the previously active file and one currently created.
}

// TestChunkDiskMapper_Truncate_PreservesFileSequence tests that truncation doesn't poke
// holes into the file sequence, even if there are empty files in between non-empty files.
// This test exposes https://github.com/prometheus/prometheus/issues/7412 where the truncation
// simply deleted all empty files instead of stopping once it encountered a non-empty file.
func TestChunkDiskMapper_Truncate_PreservesFileSequence(t *testing.T) {
	hrw := createChunkDiskMapper(t, "")
	defer func() {
		require.NoError(t, hrw.Close())
	}()
	timeRange := 0

	addChunk := func() {
		t.Helper()

		awaitCb := make(chan struct{})

		step := 100
		mint, maxt := timeRange+1, timeRange+step-1
		hrw.WriteChunk(1, int64(mint), int64(maxt), randomChunk(t), func(err error) {
			close(awaitCb)
			require.NoError(t, err)
		})
		<-awaitCb
		timeRange += step
	}

	emptyFile := func() {
		t.Helper()

		_, _, err := hrw.cut()
		require.NoError(t, err)
		hrw.evtlPosMtx.Lock()
		hrw.evtlPos.toNewFile()
		hrw.evtlPosMtx.Unlock()
	}

	nonEmptyFile := func() {
		t.Helper()

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

		files, err := ioutil.ReadDir(hrw.dir.Name())
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
	file2Maxt := hrw.mmappedChunkFiles[2].maxt
	require.NoError(t, hrw.Truncate(file2Maxt+1))
	verifyFiles([]int{3, 4, 5, 6})

	// Add chunk, so file 6 is not empty anymore.
	addChunk()
	verifyFiles([]int{3, 4, 5, 6})

	// Truncating till file 3 should also delete file 4, because it is empty.
	file3Maxt := hrw.mmappedChunkFiles[3].maxt
	require.NoError(t, hrw.Truncate(file3Maxt+1))
	addChunk()
	verifyFiles([]int{5, 6, 7})

	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())
	// Restarting checks for unsequential files.
	hrw = createChunkDiskMapper(t, dir)
	verifyFiles([]int{5, 6, 7})
}

// TestHeadReadWriter_TruncateAfterIterateChunksError tests for
// https://github.com/prometheus/prometheus/issues/7753
func TestHeadReadWriter_TruncateAfterFailedIterateChunks(t *testing.T) {
	hrw := createChunkDiskMapper(t, "")
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	// Write a chunks to iterate on it later.
	var err error
	awaitCb := make(chan struct{})
	hrw.WriteChunk(1, 0, 1000, randomChunk(t), func(cbErr error) {
		err = cbErr
		close(awaitCb)
	})
	<-awaitCb
	require.NoError(t, err)

	dir := hrw.dir.Name()
	require.NoError(t, hrw.Close())

	// Restarting to recreate https://github.com/prometheus/prometheus/issues/7753.
	hrw = createChunkDiskMapper(t, dir)

	// Forcefully failing IterateAllChunks.
	require.Error(t, hrw.IterateAllChunks(func(_ HeadSeriesRef, _ ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding) error {
		return errors.New("random error")
	}))

	// Truncation call should not return error after IterateAllChunks fails.
	require.NoError(t, hrw.Truncate(2000))
}

func TestHeadReadWriter_ReadRepairOnEmptyLastFile(t *testing.T) {
	hrw := createChunkDiskMapper(t, "")
	defer func() {
		require.NoError(t, hrw.Close())
	}()

	timeRange := 0
	addChunk := func() {
		t.Helper()

		step := 100
		mint, maxt := timeRange+1, timeRange+step-1
		var err error
		awaitCb := make(chan struct{})
		hrw.WriteChunk(1, int64(mint), int64(maxt), randomChunk(t), func(cbErr error) {
			err = cbErr
			close(awaitCb)
		})
		<-awaitCb
		require.NoError(t, err)
		timeRange += step
	}
	nonEmptyFile := func() {
		t.Helper()

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
	hrw = createChunkDiskMapper(t, dir)

	// Removed from memory.
	require.Equal(t, 3, len(hrw.mmappedChunkFiles))
	for idx := range hrw.mmappedChunkFiles {
		require.LessOrEqual(t, idx, lastFile, "file index is bigger than previous last file")
	}

	// Removed even from disk.
	files, err := ioutil.ReadDir(dir)
	require.NoError(t, err)
	require.Equal(t, 3, len(files))
	for _, fi := range files {
		seq, err := strconv.ParseUint(fi.Name(), 10, 64)
		require.NoError(t, err)
		require.LessOrEqual(t, seq, uint64(lastFile), "file index on disk is bigger than previous last file")
	}
}

func createChunkDiskMapper(t *testing.T, dir string) *ChunkDiskMapper {
	if dir == "" {
		var err error
		dir, err = ioutil.TempDir("", "data")
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, os.RemoveAll(dir))
		})
	}

	hrw, err := NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), DefaultWriteBufferSize, DefaultWriteQueueSize)
	require.NoError(t, err)
	require.False(t, hrw.fileMaxtSet)
	require.NoError(t, hrw.IterateAllChunks(func(_ HeadSeriesRef, _ ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding) error {
		return nil
	}))
	require.True(t, hrw.fileMaxtSet)

	return hrw
}

func randomChunk(t *testing.T) chunkenc.Chunk {
	chunk := chunkenc.NewXORChunk()
	len := rand.Int() % 120
	app, err := chunk.Appender()
	require.NoError(t, err)
	for i := 0; i < len; i++ {
		app.Append(rand.Int63(), rand.Float64())
	}
	return chunk
}

func randomUnsupportedChunk(t *testing.T) chunkenc.Chunk {
	chunk := newUnsupportedChunk()
	len := rand.Int() % 120
	app, err := chunk.Appender()
	require.NoError(t, err)
	for i := 0; i < len; i++ {
		app.Append(rand.Int63(), rand.Float64())
	}
	return chunk
}

func createChunk(t *testing.T, idx int, hrw *ChunkDiskMapper) (seriesRef HeadSeriesRef, chunkRef ChunkDiskMapperRef, mint, maxt int64, chunk chunkenc.Chunk) {
	var err error
	seriesRef = HeadSeriesRef(rand.Int63())
	mint = int64((idx)*1000 + 1)
	maxt = int64((idx + 1) * 1000)
	chunk = randomChunk(t)
	awaitCb := make(chan struct{})
	chunkRef = hrw.WriteChunk(seriesRef, mint, maxt, chunk, func(cbErr error) {
		require.NoError(t, err)
		close(awaitCb)
	})
	<-awaitCb
	return
}

func writeUnsupportedChunk(t *testing.T, idx int, hrw *ChunkDiskMapper, hrwOld *OldChunkDiskMapper) (seriesRef HeadSeriesRef, chunkRef ChunkDiskMapperRef, mint, maxt int64, chunk chunkenc.Chunk) {
	var err error
	seriesRef = HeadSeriesRef(rand.Int63())
	mint = int64((idx)*1000 + 1)
	maxt = int64((idx + 1) * 1000)
	chunk = randomUnsupportedChunk(t)
	awaitCb := make(chan struct{})
	if hrw != nil {
		chunkRef = hrw.WriteChunk(seriesRef, mint, maxt, chunk, func(cbErr error) {
			require.NoError(t, err)
			close(awaitCb)
		})
	} else {
		chunkRef = hrwOld.WriteChunk(seriesRef, mint, maxt, chunk, func(cbErr error) {
			require.NoError(t, err)
		})
		close(awaitCb)
	}
	<-awaitCb
	return
}

const (
	UnsupportedMask   = 0b10000000
	EncUnsupportedXOR = chunkenc.EncXOR | UnsupportedMask
)

// unsupportedChunk holds a XORChunk and overrides the Encoding() method.
type unsupportedChunk struct {
	*chunkenc.XORChunk
}

func newUnsupportedChunk() *unsupportedChunk {
	return &unsupportedChunk{chunkenc.NewXORChunk()}
}

func (c *unsupportedChunk) Encoding() chunkenc.Encoding {
	return EncUnsupportedXOR
}
