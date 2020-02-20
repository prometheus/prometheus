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
	"testing"
	"time"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestHeadReadWriter_WriteChunk(t *testing.T) {
	hrw, close := testHeadReadWriter(t)
	defer func() {
		testutil.Ok(t, hrw.Close())
		close()
	}()

	expectedBytes := []byte{}
	numChunks := 2000
	nextChunkOffset := uint64(HeadChunkFileHeaderSize + SeriesRefSize + 2*MintMaxtSize)
	chkCRC32 := newCRC32()

	var buf [8]byte
	for i := 0; i < numChunks; i++ {
		seriesRef := uint64(rand.Int63())
		mint := int64(i*1000 + 1)
		maxt := int64((i + 1) * 1000)
		chunk := randomChunk(t)
		chkCRC32.Reset()

		chkRef, err := hrw.WriteChunk(seriesRef, mint, maxt, chunk)
		testutil.Ok(t, err)
		testutil.Equals(t, chunkRef(1, nextChunkOffset), chkRef)

		// Calculating expected bytes written on disk.
		binary.BigEndian.PutUint64(buf[:], seriesRef)
		expectedBytes = append(expectedBytes, buf[:SeriesRefSize]...)
		_, err = chkCRC32.Write(buf[:SeriesRefSize])
		testutil.Ok(t, err)

		binary.BigEndian.PutUint64(buf[:], uint64(mint))
		expectedBytes = append(expectedBytes, buf[:MintMaxtSize]...)
		_, err = chkCRC32.Write(buf[:MintMaxtSize])
		testutil.Ok(t, err)

		binary.BigEndian.PutUint64(buf[:], uint64(maxt))
		expectedBytes = append(expectedBytes, buf[:MintMaxtSize]...)
		_, err = chkCRC32.Write(buf[:MintMaxtSize])
		testutil.Ok(t, err)

		expectedBytes = append(expectedBytes, byte(chunk.Encoding()))
		_, err = chkCRC32.Write([]byte{byte(chunk.Encoding())})
		testutil.Ok(t, err)

		n := binary.PutUvarint(buf[:], uint64(len(chunk.Bytes())))
		expectedBytes = append(expectedBytes, buf[:n]...)
		_, err = chkCRC32.Write(buf[:n])
		testutil.Ok(t, err)

		expectedBytes = append(expectedBytes, chunk.Bytes()...)
		_, err = chkCRC32.Write(chunk.Bytes())
		testutil.Ok(t, err)

		expectedBytes = append(expectedBytes, chkCRC32.Sum(nil)...)

		// += encoding + chunk data len + chunk data + CRC + seriesRef,mint,maxt of next chunk.
		nextChunkOffset += 1 + uint64(n) + uint64(len(chunk.Bytes())) + CRCSize + SeriesRefSize + 2*MintMaxtSize
	}
	testutil.Ok(t, hrw.flushBuffer())

	testutil.Assert(t, len(hrw.mmappedChunkFiles) == 1 && len(hrw.closers) == 1, "expected only 1 mmapped file, got %d", len(hrw.mmappedChunkFiles))

	fileName := hrw.curFile.Name()
	actualBytes, err := ioutil.ReadFile(fileName)
	testutil.Ok(t, err)

	// Check header of the segment file.
	testutil.Equals(t, MagicHeadChunks, int(binary.BigEndian.Uint32(actualBytes[0:MagicChunksSize])))
	testutil.Equals(t, chunksFormatV1, int(actualBytes[MagicChunksSize]))

	mint := binary.BigEndian.Uint64(actualBytes[HeaderMintOffset : HeaderMintOffset+8])
	maxt := binary.BigEndian.Uint64(actualBytes[HeaderMaxtOffset : HeaderMaxtOffset+8])
	testutil.Assert(t, mint > 0, "expected mint >0")
	testutil.Assert(t, maxt == 0, "expected maxt = 0 for an active file")

	// Remaining chunk data.
	testutil.Equals(t, expectedBytes, actualBytes[HeadChunkFileHeaderSize:])
}

func TestHeadReadWriter_ReadChunk(t *testing.T) {
	hrw, close := testHeadReadWriter(t)
	defer func() {
		testutil.Ok(t, hrw.Close())
		close()
	}()

	type expectedDataType struct {
		chunkRef uint64
		chunk    chunkenc.Chunk
	}

	expectedData := []expectedDataType{}
	numChunks := 100

	timesRan := 0
	populateChunks := func() {
		for i := 0; i < numChunks; i++ {
			chunk := randomChunk(t)
			chunkRef, err := hrw.WriteChunk(
				uint64(rand.Int63()),
				int64(((timesRan*numChunks)+i)*1000+1),
				int64(((timesRan*numChunks)+i+1)*1000),
				chunk,
			)
			testutil.Ok(t, err)

			expectedData = append(expectedData, expectedDataType{
				chunkRef: chunkRef,
				chunk:    chunk,
			})
		}
	}

	// Add chunks till we fill at least 1 segment file
	// and also have something in the in-memory buffer.
	for hrw.curFileSequence < 2 || hrw.wbuf.Buffered() == 0 {
		populateChunks()
		timesRan++
	}

	// This is to test the access of buffered chunks.
	testutil.Assert(t, hrw.wbuf.Buffered() > 0, "there are no buffered chunks")

	for _, exp := range expectedData {
		actChunk, err := hrw.Chunk(exp.chunkRef)
		testutil.Ok(t, err)
		testutil.Equals(t, exp.chunk.Bytes(), actChunk.Bytes())
	}
}

func TestHeadReadWriter_IterateChunks(t *testing.T) {
	hrw, close := testHeadReadWriter(t)
	defer func(h *ChunkDiskMapper) {
		testutil.Ok(t, h.Close())
		close()
	}(hrw)

	type expectedDataType struct {
		seriesRef, chunkRef uint64
		mint, maxt          int64
		chunk               chunkenc.Chunk
	}

	expectedData := []expectedDataType{}
	numChunks := 10000

	for i := 0; i < numChunks; i++ {
		seriesRef := uint64(rand.Int63())
		mint := int64(i*1000 + 1)
		maxt := int64((i + 1) * 1000)
		chunk := randomChunk(t)

		chunkRef, err := hrw.WriteChunk(seriesRef, mint, maxt, chunk)
		testutil.Ok(t, err)

		expectedData = append(expectedData, expectedDataType{
			seriesRef: seriesRef,
			mint:      mint,
			maxt:      maxt,
			chunkRef:  chunkRef,
			chunk:     chunk,
		})
	}

	dir := hrw.dir.Name()
	testutil.Ok(t, hrw.Close())

	hrw, err := NewChunkDiskMapper(dir, chunkenc.NewPool())
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, hrw.Close())
	}()

	expIdx := 0
	err = hrw.IterateAllChunks(func(seriesRef, chunkRef uint64, mint, maxt int64) error {
		t.Helper()

		expData := expectedData[expIdx]
		testutil.Equals(t, expData.seriesRef, seriesRef)
		testutil.Equals(t, expData.chunkRef, chunkRef)
		testutil.Equals(t, expData.maxt, maxt)
		testutil.Equals(t, expData.maxt, maxt)

		actChunk, err := hrw.Chunk(expData.chunkRef)
		testutil.Ok(t, err)
		testutil.Equals(t, expData.chunk.Bytes(), actChunk.Bytes())

		expIdx++
		return nil
	})
	testutil.Ok(t, err)
	testutil.Equals(t, expIdx, len(expectedData))
}

func TestHeadReadWriter_Truncate(t *testing.T) {
	hrw, close := testHeadReadWriter(t)
	defer func() {
		testutil.Ok(t, hrw.Close())
		close()
	}()

	var timeToTruncate int64

	// Cut 5 segments.
	for i := 1; i <= 5; i++ {
		testutil.Ok(t, hrw.cut(int64(i-1)*100))

		// Write a chunks to set maxt for the segment.
		_, err := hrw.WriteChunk(1, int64((i-1)*100)+1, int64(i*100)-1, randomChunk(t))
		testutil.Ok(t, err)

		time.Sleep(100 * time.Millisecond)
		if i == 3 {
			// Truncate the segment files before the 3rd segment.
			timeToTruncate = int64((i-1)*100) + 1
		}
	}

	// Verify the number of segments.
	files, err := ioutil.ReadDir(hrw.dir.Name())
	testutil.Ok(t, err)
	testutil.Equals(t, 5, len(files))
	testutil.Equals(t, 5, len(hrw.mmappedChunkFiles))
	testutil.Equals(t, 5, len(hrw.closers))

	// Truncating files.
	testutil.Ok(t, hrw.Truncate(timeToTruncate))

	// Verifying the truncated files.
	files, err = ioutil.ReadDir(hrw.dir.Name())
	testutil.Ok(t, err)
	testutil.Equals(t, 3, len(files))
	testutil.Equals(t, 3, len(hrw.mmappedChunkFiles))
	testutil.Equals(t, 3, len(hrw.closers))

	_, ok := hrw.mmappedChunkFiles[3]
	testutil.Equals(t, true, ok)
	_, ok = hrw.mmappedChunkFiles[4]
	testutil.Equals(t, true, ok)
	_, ok = hrw.mmappedChunkFiles[5]
	testutil.Equals(t, true, ok)

	// Truncating till current time should not delete the current active file.
	testutil.Ok(t, hrw.Truncate(time.Now().UnixNano()/1e6))

	files, err = ioutil.ReadDir(hrw.dir.Name())
	testutil.Ok(t, err)
	testutil.Equals(t, 1, len(files))
	testutil.Equals(t, 1, len(hrw.mmappedChunkFiles))
	testutil.Equals(t, 1, len(hrw.closers))
	_, ok = hrw.mmappedChunkFiles[5]
	testutil.Equals(t, true, ok)
}

func testHeadReadWriter(t *testing.T) (hrw *ChunkDiskMapper, close func()) {
	tmpdir, err := ioutil.TempDir("", "data")
	testutil.Ok(t, err)
	hrw, err = NewChunkDiskMapper(tmpdir, chunkenc.NewPool())
	testutil.Ok(t, err)
	return hrw, func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	}
}

func randomChunk(t *testing.T) chunkenc.Chunk {
	chunk := chunkenc.NewXORChunk()
	len := rand.Int() % 120
	app, err := chunk.Appender()
	testutil.Ok(t, err)
	for i := 0; i < len; i++ {
		app.Append(rand.Int63(), rand.Float64())
	}
	return chunk
}
