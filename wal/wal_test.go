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

	"github.com/prometheus/tsdb/testutil"
)

func TestWAL_Repair(t *testing.T) {
	for name, test := range map[string]struct {
		corrSgm    int              // Which segment to corrupt.
		corrFunc   func(f *os.File) // Func that applies the corruption.
		intactRecs int              // Total expected records left after the repair.
	}{
		"torn_last_record": {
			2,
			func(f *os.File) {
				_, err := f.Seek(pageSize*2, 0)
				testutil.Ok(t, err)
				_, err = f.Write([]byte{byte(recFirst)})
				testutil.Ok(t, err)
			},
			8,
		},
		// Ensures that the page buffer is big enough to fit
		// an entire page size without panicing.
		// https://github.com/prometheus/tsdb/pull/414
		"bad_header": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				testutil.Ok(t, err)
				_, err = f.Write([]byte{byte(recPageTerm)})
				testutil.Ok(t, err)
			},
			4,
		},
		"bad_fragment_sequence": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				testutil.Ok(t, err)
				_, err = f.Write([]byte{byte(recLast)})
				testutil.Ok(t, err)
			},
			4,
		},
		"bad_fragment_flag": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize, 0)
				testutil.Ok(t, err)
				_, err = f.Write([]byte{123})
				testutil.Ok(t, err)
			},
			4,
		},
		"bad_checksum": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+4, 0)
				testutil.Ok(t, err)
				_, err = f.Write([]byte{0})
				testutil.Ok(t, err)
			},
			4,
		},
		"bad_length": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+2, 0)
				testutil.Ok(t, err)
				_, err = f.Write([]byte{0})
				testutil.Ok(t, err)
			},
			4,
		},
		"bad_content": {
			1,
			func(f *os.File) {
				_, err := f.Seek(pageSize+100, 0)
				testutil.Ok(t, err)
				_, err = f.Write([]byte("beef"))
				testutil.Ok(t, err)
			},
			4,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "wal_repair")
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			// We create 3 segments with 3 records each and
			// then corrupt a given record in a given segment.
			// As a result we want a repaired WAL with given intact records.
			segSize := 3 * pageSize
			w, err := NewSize(nil, nil, dir, segSize)
			testutil.Ok(t, err)

			var records [][]byte

			for i := 1; i <= 9; i++ {
				b := make([]byte, pageSize-recordHeaderSize)
				b[0] = byte(i)
				records = append(records, b)
				testutil.Ok(t, w.Log(b))
			}
			testutil.Ok(t, w.Close())

			f, err := os.OpenFile(SegmentName(dir, test.corrSgm), os.O_RDWR, 0666)
			testutil.Ok(t, err)

			// Apply corruption function.
			test.corrFunc(f)

			testutil.Ok(t, f.Close())

			w, err = NewSize(nil, nil, dir, segSize)
			testutil.Ok(t, err)

			sr, err := NewSegmentsReader(dir)
			testutil.Ok(t, err)
			r := NewReader(sr)

			for r.Next() {
			}
			testutil.NotOk(t, r.Err())
			testutil.Ok(t, sr.Close())

			testutil.Ok(t, w.Repair(r.Err()))
			sr, err = NewSegmentsReader(dir)
			testutil.Ok(t, err)
			r = NewReader(sr)

			var result [][]byte
			for r.Next() {
				var b []byte
				result = append(result, append(b, r.Record()...))
			}
			testutil.Ok(t, r.Err())
			testutil.Equals(t, test.intactRecs, len(result), "Wrong number of intact records")

			for i, r := range result {
				if !bytes.Equal(records[i], r) {
					t.Fatalf("record %d diverges: want %x, got %x", i, records[i][:10], r[:10])
				}
			}

			// Make sure the last segment is the corrupt segment.
			_, last, err := w.Segments()
			testutil.Ok(t, err)
			testutil.Equals(t, test.corrSgm, last)
		})
	}
}

// TestCorruptAndCarryOn writes a multi-segment WAL; corrupts the first segment and
// ensures that an error during reading that segment are correctly repaired before
// moving to write more records to the WAL.
func TestCorruptAndCarryOn(t *testing.T) {
	dir, err := ioutil.TempDir("", "wal_repair")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	var (
		logger      = testutil.NewLogger(t)
		segmentSize = pageSize * 3
		recordSize  = (pageSize / 3) - recordHeaderSize
	)

	// Produce a WAL with a two segments of 3 pages with 3 records each,
	// so when we truncate the file we're guaranteed to split a record.
	{
		w, err := NewSize(logger, nil, dir, segmentSize)
		testutil.Ok(t, err)

		for i := 0; i < 18; i++ {
			buf := make([]byte, recordSize)
			_, err := rand.Read(buf)
			testutil.Ok(t, err)

			err = w.Log(buf)
			testutil.Ok(t, err)
		}

		err = w.Close()
		testutil.Ok(t, err)
	}

	// Check all the segments are the correct size.
	{
		segments, err := listSegments(dir)
		testutil.Ok(t, err)
		for _, segment := range segments {
			f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%08d", segment.index)), os.O_RDONLY, 0666)
			testutil.Ok(t, err)

			fi, err := f.Stat()
			testutil.Ok(t, err)

			t.Log("segment", segment.index, "size", fi.Size())
			testutil.Equals(t, int64(segmentSize), fi.Size())

			err = f.Close()
			testutil.Ok(t, err)
		}
	}

	// Truncate the first file, splitting the middle record in the second
	// page in half, leaving 4 valid records.
	{
		f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%08d", 0)), os.O_RDWR, 0666)
		testutil.Ok(t, err)

		fi, err := f.Stat()
		testutil.Ok(t, err)
		testutil.Equals(t, int64(segmentSize), fi.Size())

		err = f.Truncate(int64(segmentSize / 2))
		testutil.Ok(t, err)

		err = f.Close()
		testutil.Ok(t, err)
	}

	// Now try and repair this WAL, and write 5 more records to it.
	{
		sr, err := NewSegmentsReader(dir)
		testutil.Ok(t, err)

		reader := NewReader(sr)
		i := 0
		for ; i < 4 && reader.Next(); i++ {
			testutil.Equals(t, recordSize, len(reader.Record()))
		}
		testutil.Equals(t, 4, i, "not enough records")
		testutil.Assert(t, !reader.Next(), "unexpected record")

		corruptionErr := reader.Err()
		testutil.Assert(t, corruptionErr != nil, "expected error")

		err = sr.Close()
		testutil.Ok(t, err)

		w, err := NewSize(logger, nil, dir, segmentSize)
		testutil.Ok(t, err)

		err = w.Repair(corruptionErr)
		testutil.Ok(t, err)

		for i := 0; i < 5; i++ {
			buf := make([]byte, recordSize)
			_, err := rand.Read(buf)
			testutil.Ok(t, err)

			err = w.Log(buf)
			testutil.Ok(t, err)
		}

		err = w.Close()
		testutil.Ok(t, err)
	}

	// Replay the WAL. Should get 9 records.
	{
		sr, err := NewSegmentsReader(dir)
		testutil.Ok(t, err)

		reader := NewReader(sr)
		i := 0
		for ; i < 9 && reader.Next(); i++ {
			testutil.Equals(t, recordSize, len(reader.Record()))
		}
		testutil.Equals(t, 9, i, "wrong number of records")
		testutil.Assert(t, !reader.Next(), "unexpected record")
		testutil.Equals(t, nil, reader.Err())
	}
}

func BenchmarkWAL_LogBatched(b *testing.B) {
	dir, err := ioutil.TempDir("", "bench_logbatch")
	testutil.Ok(b, err)
	defer os.RemoveAll(dir)

	w, err := New(nil, nil, "testdir")
	testutil.Ok(b, err)
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
		testutil.Ok(b, err)
		recs = recs[:0]
	}
	// Stop timer to not count fsync time on close.
	// If it's counted batched vs. single benchmarks are very similar but
	// do not show burst throughput well.
	b.StopTimer()
}

func BenchmarkWAL_Log(b *testing.B) {
	dir, err := ioutil.TempDir("", "bench_logsingle")
	testutil.Ok(b, err)
	defer os.RemoveAll(dir)

	w, err := New(nil, nil, "testdir")
	testutil.Ok(b, err)
	defer w.Close()

	var buf [2048]byte
	b.SetBytes(2048)

	for i := 0; i < b.N; i++ {
		err := w.Log(buf[:])
		testutil.Ok(b, err)
	}
	// Stop timer to not count fsync time on close.
	// If it's counted batched vs. single benchmarks are very similar but
	// do not show burst throughput well.
	b.StopTimer()
}
