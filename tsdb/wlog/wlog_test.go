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

package wlog

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestWALRepair_ReadingError ensures that a repair is run for an error
// when reading a record.
func TestWALRepair_ReadingError(t *testing.T) {
	for name, test := range map[string]struct {
		corrSgm    int              // Which segment to corrupt.
		corrFunc   func(f *os.File) // Func that applies the corruption.
		intactRecs int              // Total expected records left after the repair.
	}{
		"torn_last_record": {
			2,
			func(f *os.File) {
				_, err := f.Seek(pageSize*2, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte{byte(recFirst)})
				require.NoError(t, err)
			},
			8,
		},
		// Ensures that the page buffer is big enough to fit
		// an entire page size without panicking.
		// https://github.com/prometheus/tsdb/pull/414
		"bad_header": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte{byte(recPageTerm)})
				require.NoError(t, err)
			},
			4,
		},
		"bad_fragment_sequence": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte{byte(recLast)})
				require.NoError(t, err)
			},
			4,
		},
		"bad_fragment_flag": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte{123})
				require.NoError(t, err)
			},
			4,
		},
		"bad_checksum": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+4, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte{0})
				require.NoError(t, err)
			},
			4,
		},
		"bad_length": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+2, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte{0})
				require.NoError(t, err)
			},
			4,
		},
		"bad_content": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+100, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte("beef"))
				require.NoError(t, err)
			},
			4,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()

			// We create 3 segments with 3 records each and
			// then corrupt a given record in a given segment.
			// As a result we want a repaired WAL with given intact records.
			segSize := 3 * pageSize
			w, err := NewSize(nil, nil, dir, segSize, CompressionNone)
			require.NoError(t, err)

			var records [][]byte

			for i := 1; i <= 9; i++ {
				b := make([]byte, pageSize-recordHeaderSize)
				b[0] = byte(i)
				records = append(records, b)
				require.NoError(t, w.Log(b))
			}
			first, last, err := Segments(w.Dir())
			require.NoError(t, err)
			require.Equal(t, 3, 1+last-first, "wlog creation didn't result in expected number of segments")

			require.NoError(t, w.Close())

			f, err := os.OpenFile(SegmentName(dir, test.corrSgm), os.O_RDWR, 0o666)
			require.NoError(t, err)

			// Apply corruption function.
			test.corrFunc(f)

			require.NoError(t, f.Close())

			w, err = NewSize(nil, nil, dir, segSize, CompressionNone)
			require.NoError(t, err)
			defer w.Close()

			first, last, err = Segments(w.Dir())
			require.NoError(t, err)

			// Backfill segments from the most recent checkpoint onwards.
			for i := first; i <= last; i++ {
				s, err := OpenReadSegment(SegmentName(w.Dir(), i))
				require.NoError(t, err)

				sr := NewSegmentBufReader(s)
				require.NoError(t, err)
				r := NewReader(sr)
				for r.Next() {
				}

				// Close the segment so we don't break things on Windows.
				s.Close()

				// No corruption in this segment.
				if r.Err() == nil {
					continue
				}
				require.NoError(t, w.Repair(r.Err()))
				break
			}

			sr, err := NewSegmentsReader(dir)
			require.NoError(t, err)
			defer sr.Close()
			r := NewReader(sr)

			var result [][]byte
			for r.Next() {
				var b []byte
				result = append(result, append(b, r.Record()...))
			}
			require.NoError(t, r.Err())
			require.Len(t, result, test.intactRecs, "Wrong number of intact records")

			for i, r := range result {
				require.True(t, bytes.Equal(records[i], r), "record %d diverges: want %x, got %x", i, records[i][:10], r[:10])
			}

			// Make sure there is a new 0 size Segment after the corrupted Segment.
			_, last, err = Segments(w.Dir())
			require.NoError(t, err)
			require.Equal(t, test.corrSgm+1, last)
			fi, err := os.Stat(SegmentName(dir, last))
			require.NoError(t, err)
			require.Equal(t, int64(0), fi.Size())
		})
	}
}

// TestCorruptAndCarryOn writes a multi-segment WAL; corrupts the first segment and
// ensures that an error during reading that segment are correctly repaired before
// moving to write more records to the WAL.
func TestCorruptAndCarryOn(t *testing.T) {
	dir := t.TempDir()

	var (
		logger      = testutil.NewLogger(t)
		segmentSize = pageSize * 3
		recordSize  = (pageSize / 3) - recordHeaderSize
	)

	// Produce a WAL with a two segments of 3 pages with 3 records each,
	// so when we truncate the file we're guaranteed to split a record.
	{
		w, err := NewSize(logger, nil, dir, segmentSize, CompressionNone)
		require.NoError(t, err)

		for i := 0; i < 18; i++ {
			buf := make([]byte, recordSize)
			_, err := rand.Read(buf)
			require.NoError(t, err)

			err = w.Log(buf)
			require.NoError(t, err)
		}

		err = w.Close()
		require.NoError(t, err)
	}

	// Check all the segments are the correct size.
	{
		segments, err := listSegments(dir)
		require.NoError(t, err)
		for _, segment := range segments {
			f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%08d", segment.index)), os.O_RDONLY, 0o666)
			require.NoError(t, err)

			fi, err := f.Stat()
			require.NoError(t, err)

			t.Log("segment", segment.index, "size", fi.Size())
			require.Equal(t, int64(segmentSize), fi.Size())

			err = f.Close()
			require.NoError(t, err)
		}
	}

	// Truncate the first file, splitting the middle record in the second
	// page in half, leaving 4 valid records.
	{
		f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%08d", 0)), os.O_RDWR, 0o666)
		require.NoError(t, err)

		fi, err := f.Stat()
		require.NoError(t, err)
		require.Equal(t, int64(segmentSize), fi.Size())

		err = f.Truncate(int64(segmentSize / 2))
		require.NoError(t, err)

		err = f.Close()
		require.NoError(t, err)
	}

	// Now try and repair this WAL, and write 5 more records to it.
	{
		sr, err := NewSegmentsReader(dir)
		require.NoError(t, err)

		reader := NewReader(sr)
		i := 0
		for ; i < 4 && reader.Next(); i++ {
			require.Len(t, reader.Record(), recordSize)
		}
		require.Equal(t, 4, i, "not enough records")
		require.False(t, reader.Next(), "unexpected record")

		corruptionErr := reader.Err()
		require.Error(t, corruptionErr)

		err = sr.Close()
		require.NoError(t, err)

		w, err := NewSize(logger, nil, dir, segmentSize, CompressionNone)
		require.NoError(t, err)

		err = w.Repair(corruptionErr)
		require.NoError(t, err)

		// Ensure that we have a completely clean slate after repairing.
		require.Equal(t, 1, w.segment.Index()) // We corrupted segment 0.
		require.Equal(t, 0, w.donePages)

		for i := 0; i < 5; i++ {
			buf := make([]byte, recordSize)
			_, err := rand.Read(buf)
			require.NoError(t, err)

			err = w.Log(buf)
			require.NoError(t, err)
		}

		err = w.Close()
		require.NoError(t, err)
	}

	// Replay the WAL. Should get 9 records.
	{
		sr, err := NewSegmentsReader(dir)
		require.NoError(t, err)

		reader := NewReader(sr)
		i := 0
		for ; i < 9 && reader.Next(); i++ {
			require.Len(t, reader.Record(), recordSize)
		}
		require.Equal(t, 9, i, "wrong number of records")
		require.False(t, reader.Next(), "unexpected record")
		require.NoError(t, reader.Err())
		sr.Close()
	}
}

// TestClose ensures that calling Close more than once doesn't panic and doesn't block.
func TestClose(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSize(nil, nil, dir, pageSize, CompressionNone)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.Error(t, w.Close())
}

func TestSegmentMetric(t *testing.T) {
	var (
		segmentSize = pageSize
		recordSize  = (pageSize / 2) - recordHeaderSize
	)

	dir := t.TempDir()
	w, err := NewSize(nil, nil, dir, segmentSize, CompressionNone)
	require.NoError(t, err)

	initialSegment := client_testutil.ToFloat64(w.metrics.currentSegment)

	// Write 3 records, each of which is half the segment size, meaning we should rotate to the next segment.
	for i := 0; i < 3; i++ {
		buf := make([]byte, recordSize)
		_, err := rand.Read(buf)
		require.NoError(t, err)

		err = w.Log(buf)
		require.NoError(t, err)
	}
	require.Equal(t, initialSegment+1, client_testutil.ToFloat64(w.metrics.currentSegment), "segment metric did not increment after segment rotation")
	require.NoError(t, w.Close())
}

func TestCompression(t *testing.T) {
	bootstrap := func(compressed CompressionType) string {
		const (
			segmentSize = pageSize
			recordSize  = (pageSize / 2) - recordHeaderSize
			records     = 100
		)

		dirPath := t.TempDir()

		w, err := NewSize(nil, nil, dirPath, segmentSize, compressed)
		require.NoError(t, err)

		buf := make([]byte, recordSize)
		for i := 0; i < records; i++ {
			require.NoError(t, w.Log(buf))
		}
		require.NoError(t, w.Close())

		return dirPath
	}

	tmpDirs := make([]string, 0, 3)
	defer func() {
		for _, dir := range tmpDirs {
			require.NoError(t, os.RemoveAll(dir))
		}
	}()

	dirUnCompressed := bootstrap(CompressionNone)
	tmpDirs = append(tmpDirs, dirUnCompressed)

	for _, compressionType := range []CompressionType{CompressionSnappy, CompressionZstd} {
		dirCompressed := bootstrap(compressionType)
		tmpDirs = append(tmpDirs, dirCompressed)

		uncompressedSize, err := fileutil.DirSize(dirUnCompressed)
		require.NoError(t, err)
		compressedSize, err := fileutil.DirSize(dirCompressed)
		require.NoError(t, err)

		require.Greater(t, float64(uncompressedSize)*0.75, float64(compressedSize), "Compressing zeroes should save at least 25%% space - uncompressedSize: %d, compressedSize: %d", uncompressedSize, compressedSize)
	}
}

func TestLogPartialWrite(t *testing.T) {
	const segmentSize = pageSize * 2
	record := []byte{1, 2, 3, 4, 5}

	tests := map[string]struct {
		numRecords   int
		faultyRecord int
	}{
		"partial write when logging first record in a page": {
			numRecords:   10,
			faultyRecord: 1,
		},
		"partial write when logging record in the middle of a page": {
			numRecords:   10,
			faultyRecord: 3,
		},
		"partial write when logging last record of a page": {
			numRecords:   (pageSize / (recordHeaderSize + len(record))) + 10,
			faultyRecord: pageSize / (recordHeaderSize + len(record)),
		},
		// TODO the current implementation suffers this:
		// "partial write when logging a record overlapping two pages": {
		//	numRecords:   (pageSize / (recordHeaderSize + len(record))) + 10,
		//	faultyRecord: pageSize/(recordHeaderSize+len(record)) + 1,
		// },
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			dirPath := t.TempDir()

			w, err := NewSize(nil, nil, dirPath, segmentSize, CompressionNone)
			require.NoError(t, err)

			// Replace the underlying segment file with a mocked one that injects a failure.
			w.segment.SegmentFile = &faultySegmentFile{
				SegmentFile:       w.segment.SegmentFile,
				writeFailureAfter: ((recordHeaderSize + len(record)) * (testData.faultyRecord - 1)) + 2,
				writeFailureErr:   io.ErrShortWrite,
			}

			for i := 1; i <= testData.numRecords; i++ {
				if err := w.Log(record); i == testData.faultyRecord {
					require.ErrorIs(t, io.ErrShortWrite, err)
				} else {
					require.NoError(t, err)
				}
			}

			require.NoError(t, w.Close())

			// Read it back. We expect no corruption.
			s, err := OpenReadSegment(SegmentName(dirPath, 0))
			require.NoError(t, err)
			defer func() { require.NoError(t, s.Close()) }()

			r := NewReader(NewSegmentBufReader(s))
			for i := 0; i < testData.numRecords; i++ {
				require.True(t, r.Next())
				require.NoError(t, r.Err())
				require.Equal(t, record, r.Record())
			}
			require.False(t, r.Next())
			require.NoError(t, r.Err())
		})
	}
}

type faultySegmentFile struct {
	SegmentFile

	written           int
	writeFailureAfter int
	writeFailureErr   error
}

func (f *faultySegmentFile) Write(p []byte) (int, error) {
	if f.writeFailureAfter >= 0 && f.writeFailureAfter < f.written+len(p) {
		partialLen := f.writeFailureAfter - f.written
		if partialLen <= 0 || partialLen >= len(p) {
			partialLen = 1
		}

		// Inject failure.
		n, _ := f.SegmentFile.Write(p[:partialLen])
		f.written += n
		f.writeFailureAfter = -1

		return n, f.writeFailureErr
	}

	// Proxy the write to the underlying file.
	n, err := f.SegmentFile.Write(p)
	f.written += n
	return n, err
}

func BenchmarkWAL_LogBatched(b *testing.B) {
	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		b.Run(fmt.Sprintf("compress=%s", compress), func(b *testing.B) {
			dir := b.TempDir()

			w, err := New(nil, nil, dir, compress)
			require.NoError(b, err)
			defer w.Close()

			var buf [2048]byte
			var recs [][]byte
			b.SetBytes(2048)

			for i := 0; i < b.N; i++ {
				recs = append(recs, buf[:])
				if len(recs) < 1000 {
					continue
				}
				err := w.Log(recs...)
				require.NoError(b, err)
				recs = recs[:0]
			}
			// Stop timer to not count fsync time on close.
			// If it's counted batched vs. single benchmarks are very similar but
			// do not show burst throughput well.
			b.StopTimer()
		})
	}
}

func BenchmarkWAL_Log(b *testing.B) {
	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		b.Run(fmt.Sprintf("compress=%s", compress), func(b *testing.B) {
			dir := b.TempDir()

			w, err := New(nil, nil, dir, compress)
			require.NoError(b, err)
			defer w.Close()

			var buf [2048]byte
			b.SetBytes(2048)

			for i := 0; i < b.N; i++ {
				err := w.Log(buf[:])
				require.NoError(b, err)
			}
			// Stop timer to not count fsync time on close.
			// If it's counted batched vs. single benchmarks are very similar but
			// do not show burst throughput well.
			b.StopTimer()
		})
	}
}

func TestUnregisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()

	for i := 0; i < 2; i++ {
		wl, err := New(log.NewNopLogger(), reg, t.TempDir(), CompressionNone)
		require.NoError(t, err)
		require.NoError(t, wl.Close())
	}
}
