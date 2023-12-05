// Copyright 2019 The Prometheus Authors
//
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
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/util/testutil"
)

type reader interface {
	Next() bool
	Err() error
	Record() []byte
	Offset() int64
}

type rec struct {
	t recType
	b []byte
}

var readerConstructors = map[string]func(io.Reader) reader{
	"Reader": func(r io.Reader) reader {
		return NewReader(r)
	},
	"LiveReader": func(r io.Reader) reader {
		lr := NewLiveReader(log.NewNopLogger(), NewLiveReaderMetrics(nil), r)
		lr.eofNonErr = true
		return lr
	},
}

var (
	data            = make([]byte, 100000)
	testReaderCases = []struct {
		t    []rec
		exp  [][]byte
		fail bool
	}{
		// Sequence of valid records.
		{
			t: []rec{
				{recFull, data[0:200]},
				{recFirst, data[200:300]},
				{recLast, data[300:400]},
				{recFirst, data[400:800]},
				{recMiddle, data[800:900]},
				{recPageTerm, make([]byte, pageSize-900-recordHeaderSize*5-1)}, // exactly lines up with page boundary.
				{recLast, data[900:900]},
				{recFirst, data[900:1000]},
				{recMiddle, data[1000:1200]},
				{recMiddle, data[1200:30000]},
				{recMiddle, data[30000:30001]},
				{recMiddle, data[30001:30001]},
				{recLast, data[30001:32000]},
			},
			exp: [][]byte{
				data[0:200],
				data[200:400],
				data[400:900],
				data[900:32000],
			},
		},
		// Exactly at the limit of one page minus the header size
		{
			t: []rec{
				{recFull, data[0 : pageSize-recordHeaderSize]},
			},
			exp: [][]byte{
				data[:pageSize-recordHeaderSize],
			},
		},
		// More than a full page, this exceeds our buffer and can never happen
		// when written by the WAL.
		{
			t: []rec{
				{recFull, data[0 : pageSize+1]},
			},
			fail: true,
		},
		// Two records the together are too big for a page.
		// NB currently the non-live reader succeeds on this. I think this is a bug.
		// but we've seen it in production.
		{
			t: []rec{
				{recFull, data[:pageSize/2]},
				{recFull, data[:pageSize/2]},
			},
			exp: [][]byte{
				data[:pageSize/2],
				data[:pageSize/2],
			},
		},
		// Invalid orders of record types.
		{
			t:    []rec{{recMiddle, data[:200]}},
			fail: true,
		},
		{
			t:    []rec{{recLast, data[:200]}},
			fail: true,
		},
		{
			t: []rec{
				{recFirst, data[:200]},
				{recFull, data[200:400]},
			},
			fail: true,
		},
		{
			t: []rec{
				{recFirst, data[:100]},
				{recMiddle, data[100:200]},
				{recFull, data[200:400]},
			},
			fail: true,
		},
		// Non-zero data after page termination.
		{
			t: []rec{
				{recFull, data[:100]},
				{recPageTerm, append(make([]byte, pageSize-recordHeaderSize-102), 1)},
			},
			exp:  [][]byte{data[:100]},
			fail: true,
		},
	}
)

func encodedRecord(t recType, b []byte) []byte {
	if t == recPageTerm {
		return append([]byte{0}, b...)
	}
	r := make([]byte, recordHeaderSize)
	r[0] = byte(t)
	binary.BigEndian.PutUint16(r[1:], uint16(len(b)))
	binary.BigEndian.PutUint32(r[3:], crc32.Checksum(b, castagnoliTable))
	return append(r, b...)
}

// TestReader feeds the reader a stream of encoded records with different types.
func TestReader(t *testing.T) {
	for name, fn := range readerConstructors {
		for i, c := range testReaderCases {
			t.Run(fmt.Sprintf("%s/%d", name, i), func(t *testing.T) {
				var buf []byte
				for _, r := range c.t {
					buf = append(buf, encodedRecord(r.t, r.b)...)
				}
				r := fn(bytes.NewReader(buf))

				for j := 0; r.Next(); j++ {
					t.Logf("record %d", j)
					rec := r.Record()

					if j >= len(c.exp) {
						t.Fatal("received more records than expected")
					}
					require.Equal(t, c.exp[j], rec, "Bytes within record did not match expected Bytes")
				}
				if !c.fail && r.Err() != nil {
					t.Fatalf("unexpected error: %s", r.Err())
				}
				if c.fail && r.Err() == nil {
					t.Fatalf("expected error but got none")
				}
			})
		}
	}
}

func TestReader_Live(t *testing.T) {
	logger := testutil.NewLogger(t)

	for i := range testReaderCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			writeFd, err := os.CreateTemp("", "TestReader_Live")
			require.NoError(t, err)
			defer os.Remove(writeFd.Name())

			go func(i int) {
				for _, rec := range testReaderCases[i].t {
					rec := encodedRecord(rec.t, rec.b)
					_, err := writeFd.Write(rec)
					require.NoError(t, err)
					runtime.Gosched()
				}
				writeFd.Close()
			}(i)

			// Read from a second FD on the same file.
			readFd, err := os.Open(writeFd.Name())
			require.NoError(t, err)
			reader := NewLiveReader(logger, NewLiveReaderMetrics(nil), readFd)
			for _, exp := range testReaderCases[i].exp {
				for !reader.Next() {
					require.Equal(t, io.EOF, reader.Err(), "expect EOF, got: %v", reader.Err())
					runtime.Gosched()
				}

				actual := reader.Record()
				require.Equal(t, exp, actual, "read wrong record")
			}

			require.False(t, reader.Next(), "unexpected record")
			if testReaderCases[i].fail {
				require.Error(t, reader.Err())
			}
		})
	}
}

const fuzzLen = 500

func generateRandomEntries(w *WL, records chan []byte) error {
	var recs [][]byte
	for i := 0; i < fuzzLen; i++ {
		var sz int64
		switch i % 5 {
		case 0, 1:
			sz = 50
		case 2, 3:
			sz = pageSize
		default:
			sz = pageSize * 8
		}
		n, err := rand.Int(rand.Reader, big.NewInt(sz))
		if err != nil {
			return err
		}
		rec := make([]byte, n.Int64())
		if _, err := rand.Read(rec); err != nil {
			return err
		}

		records <- rec

		// Randomly batch up records.
		recs = append(recs, rec)
		n, err = rand.Int(rand.Reader, big.NewInt(int64(4)))
		if err != nil {
			return err
		}
		if int(n.Int64()) < 3 {
			if err := w.Log(recs...); err != nil {
				return err
			}
			recs = recs[:0]
		}
	}
	return w.Log(recs...)
}

type multiReadCloser struct {
	reader  io.Reader
	closers []io.Closer
}

func (m *multiReadCloser) Read(p []byte) (n int, err error) {
	return m.reader.Read(p)
}

func (m *multiReadCloser) Close() error {
	return tsdb_errors.NewMulti(tsdb_errors.CloseAll(m.closers)).Err()
}

func allSegments(dir string) (io.ReadCloser, error) {
	seg, err := listSegments(dir)
	if err != nil {
		return nil, err
	}

	var readers []io.Reader
	var closers []io.Closer
	for _, r := range seg {
		f, err := os.Open(filepath.Join(dir, r.name))
		if err != nil {
			return nil, err
		}
		readers = append(readers, f)
		closers = append(closers, f)
	}

	return &multiReadCloser{
		reader:  io.MultiReader(readers...),
		closers: closers,
	}, nil
}

func TestReaderFuzz(t *testing.T) {
	for name, fn := range readerConstructors {
		for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
			t.Run(fmt.Sprintf("%s,compress=%s", name, compress), func(t *testing.T) {
				dir := t.TempDir()

				w, err := NewSize(nil, nil, dir, 128*pageSize, compress)
				require.NoError(t, err)

				// Buffering required as we're not reading concurrently.
				input := make(chan []byte, fuzzLen)
				err = generateRandomEntries(w, input)
				require.NoError(t, err)
				close(input)

				err = w.Close()
				require.NoError(t, err)

				sr, err := allSegments(w.Dir())
				require.NoError(t, err)
				defer sr.Close()

				reader := fn(sr)
				for expected := range input {
					require.True(t, reader.Next(), "expected record: %v", reader.Err())
					r := reader.Record()
					// Expected value may come as nil or empty slice, so it requires special comparison.
					if len(expected) == 0 {
						require.Len(t, r, 0)
					} else {
						require.Equal(t, expected, r, "read wrong record")
					}
				}
				require.False(t, reader.Next(), "unexpected record")
			})
		}
	}
}

func TestReaderFuzz_Live(t *testing.T) {
	logger := testutil.NewLogger(t)
	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()

			w, err := NewSize(nil, nil, dir, 128*pageSize, compress)
			require.NoError(t, err)
			defer w.Close()

			// In the background, generate a stream of random records and write them
			// to the WAL.
			input := make(chan []byte, fuzzLen/10) // buffering required as we sometimes batch WAL writes.
			done := make(chan struct{})
			go func() {
				err := generateRandomEntries(w, input)
				require.NoError(t, err)
				time.Sleep(100 * time.Millisecond)
				close(done)
			}()

			// Tail the WAL and compare the results.
			m, _, err := Segments(w.Dir())
			require.NoError(t, err)

			seg, err := OpenReadSegment(SegmentName(dir, m))
			require.NoError(t, err)
			defer seg.Close()

			r := NewLiveReader(logger, nil, seg)
			segmentTicker := time.NewTicker(100 * time.Millisecond)
			readTicker := time.NewTicker(10 * time.Millisecond)

			readSegment := func(r *LiveReader) bool {
				for r.Next() {
					rec := r.Record()
					expected, ok := <-input
					require.True(t, ok, "unexpected record")
					// Expected value may come as nil or empty slice, so it requires special comparison.
					if len(expected) == 0 {
						require.Len(t, rec, 0)
					} else {
						require.Equal(t, expected, rec, "record does not match expected")
					}
				}
				require.Equal(t, io.EOF, r.Err(), "expected EOF, got: %v", r.Err())
				return true
			}

		outer:
			for {
				select {
				case <-segmentTicker.C:
					// check if new segments exist
					_, last, err := Segments(w.Dir())
					require.NoError(t, err)
					if last <= seg.i {
						continue
					}

					// read to end of segment.
					readSegment(r)

					fi, err := os.Stat(SegmentName(dir, seg.i))
					require.NoError(t, err)
					require.Equal(t, r.Offset(), fi.Size(), "expected to have read whole segment, but read %d of %d", r.Offset(), fi.Size())

					seg, err = OpenReadSegment(SegmentName(dir, seg.i+1))
					require.NoError(t, err)
					defer seg.Close()
					r = NewLiveReader(logger, nil, seg)

				case <-readTicker.C:
					readSegment(r)

				case <-done:
					readSegment(r)
					break outer
				}
			}

			require.Equal(t, io.EOF, r.Err(), "expected EOF")
		})
	}
}

func TestLiveReaderCorrupt_ShortFile(t *testing.T) {
	// Write a corrupt WAL segment, there is one record of pageSize in length,
	// but the segment is only half written.
	logger := testutil.NewLogger(t)
	dir := t.TempDir()

	w, err := NewSize(nil, nil, dir, pageSize, CompressionNone)
	require.NoError(t, err)

	rec := make([]byte, pageSize-recordHeaderSize)
	_, err = rand.Read(rec)
	require.NoError(t, err)

	err = w.Log(rec)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	segmentFile, err := os.OpenFile(filepath.Join(dir, "00000000"), os.O_RDWR, 0o666)
	require.NoError(t, err)

	err = segmentFile.Truncate(pageSize / 2)
	require.NoError(t, err)

	err = segmentFile.Close()
	require.NoError(t, err)

	// Try and LiveReader it.
	m, _, err := Segments(w.Dir())
	require.NoError(t, err)

	seg, err := OpenReadSegment(SegmentName(dir, m))
	require.NoError(t, err)
	defer seg.Close()

	r := NewLiveReader(logger, nil, seg)
	require.False(t, r.Next(), "expected no records")
	require.Equal(t, io.EOF, r.Err(), "expected error, got: %v", r.Err())
}

func TestLiveReaderCorrupt_RecordTooLongAndShort(t *testing.T) {
	// Write a corrupt WAL segment, when record len > page size.
	logger := testutil.NewLogger(t)
	dir := t.TempDir()

	w, err := NewSize(nil, nil, dir, pageSize*2, CompressionNone)
	require.NoError(t, err)

	rec := make([]byte, pageSize-recordHeaderSize)
	_, err = rand.Read(rec)
	require.NoError(t, err)

	err = w.Log(rec)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	segmentFile, err := os.OpenFile(filepath.Join(dir, "00000000"), os.O_RDWR, 0o666)
	require.NoError(t, err)

	// Override the record length
	buf := make([]byte, 3)
	buf[0] = byte(recFull)
	binary.BigEndian.PutUint16(buf[1:], 0xFFFF)
	_, err = segmentFile.WriteAt(buf, 0)
	require.NoError(t, err)

	err = segmentFile.Close()
	require.NoError(t, err)

	// Try and LiveReader it.
	m, _, err := Segments(w.Dir())
	require.NoError(t, err)

	seg, err := OpenReadSegment(SegmentName(dir, m))
	require.NoError(t, err)
	defer seg.Close()

	r := NewLiveReader(logger, NewLiveReaderMetrics(nil), seg)
	require.False(t, r.Next(), "expected no records")
	require.EqualError(t, r.Err(), "record length greater than a single page: 65542 > 32768", "expected error, got: %v", r.Err())
}

func TestReaderData(t *testing.T) {
	dir := os.Getenv("WALDIR")
	if dir == "" {
		return
	}

	for name, fn := range readerConstructors {
		t.Run(name, func(t *testing.T) {
			w, err := New(nil, nil, dir, CompressionSnappy)
			require.NoError(t, err)

			sr, err := allSegments(dir)
			require.NoError(t, err)

			reader := fn(sr)
			for reader.Next() {
			}
			require.NoError(t, reader.Err())

			err = w.Repair(reader.Err())
			require.NoError(t, err)
		})
	}
}
