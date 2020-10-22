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

package wal

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
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
				assert.NoError(t, err)
				_, err = f.Write([]byte{byte(recFirst)})
				assert.NoError(t, err)
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
				assert.NoError(t, err)
				_, err = f.Write([]byte{byte(recPageTerm)})
				assert.NoError(t, err)
			},
			4,
		},
		"bad_fragment_sequence": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				assert.NoError(t, err)
				_, err = f.Write([]byte{byte(recLast)})
				assert.NoError(t, err)
			},
			4,
		},
		"bad_fragment_flag": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				assert.NoError(t, err)
				_, err = f.Write([]byte{123})
				assert.NoError(t, err)
			},
			4,
		},
		"bad_checksum": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+4, 0)
				assert.NoError(t, err)
				_, err = f.Write([]byte{0})
				assert.NoError(t, err)
			},
			4,
		},
		"bad_length": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+2, 0)
				assert.NoError(t, err)
				_, err = f.Write([]byte{0})
				assert.NoError(t, err)
			},
			4,
		},
		"bad_content": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+100, 0)
				assert.NoError(t, err)
				_, err = f.Write([]byte("beef"))
				assert.NoError(t, err)
			},
			4,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "wal_repair")
			assert.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(dir))
			}()

			// We create 3 segments with 3 records each and
			// then corrupt a given record in a given segment.
			// As a result we want a repaired WAL with given intact records.
			segSize := 3 * pageSize
			w, err := NewSize(nil, nil, dir, segSize, false)
			assert.NoError(t, err)

			var records [][]byte

			for i := 1; i <= 9; i++ {
				b := make([]byte, pageSize-recordHeaderSize)
				b[0] = byte(i)
				records = append(records, b)
				assert.NoError(t, w.Log(b))
			}
			first, last, err := Segments(w.Dir())
			assert.NoError(t, err)
			assert.Equal(t, 3, 1+last-first, "wal creation didn't result in expected number of segments")

			assert.NoError(t, w.Close())

			f, err := os.OpenFile(SegmentName(dir, test.corrSgm), os.O_RDWR, 0666)
			assert.NoError(t, err)

			// Apply corruption function.
			test.corrFunc(f)

			assert.NoError(t, f.Close())

			w, err = NewSize(nil, nil, dir, segSize, false)
			assert.NoError(t, err)
			defer w.Close()

			first, last, err = Segments(w.Dir())
			assert.NoError(t, err)

			// Backfill segments from the most recent checkpoint onwards.
			for i := first; i <= last; i++ {
				s, err := OpenReadSegment(SegmentName(w.Dir(), i))
				assert.NoError(t, err)

				sr := NewSegmentBufReader(s)
				assert.NoError(t, err)
				r := NewReader(sr)
				for r.Next() {
				}

				//Close the segment so we don't break things on Windows.
				s.Close()

				// No corruption in this segment.
				if r.Err() == nil {
					continue
				}
				assert.NoError(t, w.Repair(r.Err()))
				break
			}

			sr, err := NewSegmentsReader(dir)
			assert.NoError(t, err)
			defer sr.Close()
			r := NewReader(sr)

			var result [][]byte
			for r.Next() {
				var b []byte
				result = append(result, append(b, r.Record()...))
			}
			assert.NoError(t, r.Err())
			assert.Equal(t, test.intactRecs, len(result), "Wrong number of intact records")

			for i, r := range result {
				if !bytes.Equal(records[i], r) {
					t.Fatalf("record %d diverges: want %x, got %x", i, records[i][:10], r[:10])
				}
			}

			// Make sure there is a new 0 size Segment after the corrupted Segment.
			_, last, err = Segments(w.Dir())
			assert.NoError(t, err)
			assert.Equal(t, test.corrSgm+1, last)
			fi, err := os.Stat(SegmentName(dir, last))
			assert.NoError(t, err)
			assert.Equal(t, int64(0), fi.Size())
		})
	}
}

// TestCorruptAndCarryOn writes a multi-segment WAL; corrupts the first segment and
// ensures that an error during reading that segment are correctly repaired before
// moving to write more records to the WAL.
func TestCorruptAndCarryOn(t *testing.T) {
	dir, err := ioutil.TempDir("", "wal_repair")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()

	var (
		logger      = testutil.NewLogger(t)
		segmentSize = pageSize * 3
		recordSize  = (pageSize / 3) - recordHeaderSize
	)

	// Produce a WAL with a two segments of 3 pages with 3 records each,
	// so when we truncate the file we're guaranteed to split a record.
	{
		w, err := NewSize(logger, nil, dir, segmentSize, false)
		assert.NoError(t, err)

		for i := 0; i < 18; i++ {
			buf := make([]byte, recordSize)
			_, err := rand.Read(buf)
			assert.NoError(t, err)

			err = w.Log(buf)
			assert.NoError(t, err)
		}

		err = w.Close()
		assert.NoError(t, err)
	}

	// Check all the segments are the correct size.
	{
		segments, err := listSegments(dir)
		assert.NoError(t, err)
		for _, segment := range segments {
			f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%08d", segment.index)), os.O_RDONLY, 0666)
			assert.NoError(t, err)

			fi, err := f.Stat()
			assert.NoError(t, err)

			t.Log("segment", segment.index, "size", fi.Size())
			assert.Equal(t, int64(segmentSize), fi.Size())

			err = f.Close()
			assert.NoError(t, err)
		}
	}

	// Truncate the first file, splitting the middle record in the second
	// page in half, leaving 4 valid records.
	{
		f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%08d", 0)), os.O_RDWR, 0666)
		assert.NoError(t, err)

		fi, err := f.Stat()
		assert.NoError(t, err)
		assert.Equal(t, int64(segmentSize), fi.Size())

		err = f.Truncate(int64(segmentSize / 2))
		assert.NoError(t, err)

		err = f.Close()
		assert.NoError(t, err)
	}

	// Now try and repair this WAL, and write 5 more records to it.
	{
		sr, err := NewSegmentsReader(dir)
		assert.NoError(t, err)

		reader := NewReader(sr)
		i := 0
		for ; i < 4 && reader.Next(); i++ {
			assert.Equal(t, recordSize, len(reader.Record()))
		}
		assert.Equal(t, 4, i, "not enough records")
		assert.True(t, !reader.Next(), "unexpected record")

		corruptionErr := reader.Err()
		assert.Error(t, corruptionErr)

		err = sr.Close()
		assert.NoError(t, err)

		w, err := NewSize(logger, nil, dir, segmentSize, false)
		assert.NoError(t, err)

		err = w.Repair(corruptionErr)
		assert.NoError(t, err)

		// Ensure that we have a completely clean slate after repairing.
		assert.Equal(t, w.segment.Index(), 1) // We corrupted segment 0.
		assert.Equal(t, w.donePages, 0)

		for i := 0; i < 5; i++ {
			buf := make([]byte, recordSize)
			_, err := rand.Read(buf)
			assert.NoError(t, err)

			err = w.Log(buf)
			assert.NoError(t, err)
		}

		err = w.Close()
		assert.NoError(t, err)
	}

	// Replay the WAL. Should get 9 records.
	{
		sr, err := NewSegmentsReader(dir)
		assert.NoError(t, err)

		reader := NewReader(sr)
		i := 0
		for ; i < 9 && reader.Next(); i++ {
			assert.Equal(t, recordSize, len(reader.Record()))
		}
		assert.Equal(t, 9, i, "wrong number of records")
		assert.True(t, !reader.Next(), "unexpected record")
		assert.Equal(t, nil, reader.Err())
		sr.Close()
	}
}

// TestClose ensures that calling Close more than once doesn't panic and doesn't block.
func TestClose(t *testing.T) {
	dir, err := ioutil.TempDir("", "wal_repair")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()
	w, err := NewSize(nil, nil, dir, pageSize, false)
	assert.NoError(t, err)
	assert.NoError(t, w.Close())
	assert.Error(t, w.Close())
}

func TestSegmentMetric(t *testing.T) {
	var (
		segmentSize = pageSize
		recordSize  = (pageSize / 2) - recordHeaderSize
	)

	dir, err := ioutil.TempDir("", "segment_metric")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()
	w, err := NewSize(nil, nil, dir, segmentSize, false)
	assert.NoError(t, err)

	initialSegment := client_testutil.ToFloat64(w.metrics.currentSegment)

	// Write 3 records, each of which is half the segment size, meaning we should rotate to the next segment.
	for i := 0; i < 3; i++ {
		buf := make([]byte, recordSize)
		_, err := rand.Read(buf)
		assert.NoError(t, err)

		err = w.Log(buf)
		assert.NoError(t, err)
	}
	assert.True(t, client_testutil.ToFloat64(w.metrics.currentSegment) == initialSegment+1, "segment metric did not increment after segment rotation")
	assert.NoError(t, w.Close())
}

func TestCompression(t *testing.T) {
	bootstrap := func(compressed bool) string {
		const (
			segmentSize = pageSize
			recordSize  = (pageSize / 2) - recordHeaderSize
			records     = 100
		)

		dirPath, err := ioutil.TempDir("", fmt.Sprintf("TestCompression_%t", compressed))
		assert.NoError(t, err)

		w, err := NewSize(nil, nil, dirPath, segmentSize, compressed)
		assert.NoError(t, err)

		buf := make([]byte, recordSize)
		for i := 0; i < records; i++ {
			assert.NoError(t, w.Log(buf))
		}
		assert.NoError(t, w.Close())

		return dirPath
	}

	dirCompressed := bootstrap(true)
	defer func() {
		assert.NoError(t, os.RemoveAll(dirCompressed))
	}()
	dirUnCompressed := bootstrap(false)
	defer func() {
		assert.NoError(t, os.RemoveAll(dirUnCompressed))
	}()

	uncompressedSize, err := fileutil.DirSize(dirUnCompressed)
	assert.NoError(t, err)
	compressedSize, err := fileutil.DirSize(dirCompressed)
	assert.NoError(t, err)

	assert.True(t, float64(uncompressedSize)*0.75 > float64(compressedSize), "Compressing zeroes should save at least 25%% space - uncompressedSize: %d, compressedSize: %d", uncompressedSize, compressedSize)
}

func BenchmarkWAL_LogBatched(b *testing.B) {
	for _, compress := range []bool{true, false} {
		b.Run(fmt.Sprintf("compress=%t", compress), func(b *testing.B) {
			dir, err := ioutil.TempDir("", "bench_logbatch")
			assert.NoError(b, err)
			defer func() {
				assert.NoError(b, os.RemoveAll(dir))
			}()

			w, err := New(nil, nil, dir, compress)
			assert.NoError(b, err)
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
				assert.NoError(b, err)
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
	for _, compress := range []bool{true, false} {
		b.Run(fmt.Sprintf("compress=%t", compress), func(b *testing.B) {
			dir, err := ioutil.TempDir("", "bench_logsingle")
			assert.NoError(b, err)
			defer func() {
				assert.NoError(b, os.RemoveAll(dir))
			}()

			w, err := New(nil, nil, dir, compress)
			assert.NoError(b, err)
			defer w.Close()

			var buf [2048]byte
			b.SetBytes(2048)

			for i := 0; i < b.N; i++ {
				err := w.Log(buf[:])
				assert.NoError(b, err)
			}
			// Stop timer to not count fsync time on close.
			// If it's counted batched vs. single benchmarks are very similar but
			// do not show burst throughput well.
			b.StopTimer()
		})
	}
}
