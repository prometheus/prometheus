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
	"encoding/binary"
	"hash/crc32"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"github.com/prometheus/tsdb/testutil"
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
	data := make([]byte, 100000)
	_, err := rand.Read(data)
	testutil.Ok(t, err)

	type record struct {
		t recType
		b []byte
	}
	cases := []struct {
		t    []record
		exp  [][]byte
		fail bool
	}{
		// Sequence of valid records.
		{
			t: []record{
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
			t: []record{
				{recFull, data[0 : pageSize-recordHeaderSize]},
			},
			exp: [][]byte{
				data[:pageSize-recordHeaderSize],
			},
		},
		// More than a full page, this exceeds our buffer and can never happen
		// when written by the WAL.
		{
			t: []record{
				{recFull, data[0 : pageSize+1]},
			},
			fail: true,
		},
		// Invalid orders of record types.
		{
			t:    []record{{recMiddle, data[:200]}},
			fail: true,
		},
		{
			t:    []record{{recLast, data[:200]}},
			fail: true,
		},
		{
			t: []record{
				{recFirst, data[:200]},
				{recFull, data[200:400]},
			},
			fail: true,
		},
		{
			t: []record{
				{recFirst, data[:100]},
				{recMiddle, data[100:200]},
				{recFull, data[200:400]},
			},
			fail: true,
		},
		// Non-zero data after page termination.
		{
			t: []record{
				{recFull, data[:100]},
				{recPageTerm, append(make([]byte, 1000), 1)},
			},
			exp:  [][]byte{data[:100]},
			fail: true,
		},
	}
	for i, c := range cases {
		t.Logf("test %d", i)

		var buf []byte
		for _, r := range c.t {
			buf = append(buf, encodedRecord(r.t, r.b)...)
		}
		r := NewReader(bytes.NewReader(buf))

		for j := 0; r.Next(); j++ {
			t.Logf("record %d", j)
			rec := r.Record()

			if j >= len(c.exp) {
				t.Fatal("received more records than inserted")
			}
			testutil.Equals(t, c.exp[j], rec)
		}
		if !c.fail && r.Err() != nil {
			t.Fatalf("unexpected error: %s", r.Err())
		}
		if c.fail && r.Err() == nil {
			t.Fatalf("expected error but got none")
		}
	}
}

func TestWAL_FuzzWriteRead(t *testing.T) {
	const count = 25000

	dir, err := ioutil.TempDir("", "walfuzz")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	w, err := NewSize(nil, nil, dir, 128*pageSize)
	testutil.Ok(t, err)

	var input [][]byte
	var recs [][]byte

	for i := 0; i < count; i++ {
		var sz int
		switch i % 5 {
		case 0, 1:
			sz = 50
		case 2, 3:
			sz = pageSize
		default:
			sz = 8 * pageSize
		}
		rec := make([]byte, rand.Intn(sz))
		_, err := rand.Read(rec)
		testutil.Ok(t, err)

		input = append(input, rec)
		recs = append(recs, rec)

		// Randomly batch up records.
		if rand.Intn(4) < 3 {
			testutil.Ok(t, w.Log(recs...))
			recs = recs[:0]
		}
	}
	testutil.Ok(t, w.Log(recs...))

	m, n, err := w.Segments()
	testutil.Ok(t, err)

	rc, err := NewSegmentsRangeReader(dir, m, n)
	testutil.Ok(t, err)
	defer rc.Close()

	rdr := NewReader(rc)

	for i := 0; rdr.Next(); i++ {
		rec := rdr.Record()
		if i >= len(input) {
			t.Fatal("read too many records")
		}
		if !bytes.Equal(input[i], rec) {
			t.Fatalf("record %d (len %d) does not match (expected len %d)",
				i, len(rec), len(input[i]))
		}
	}
	testutil.Ok(t, rdr.Err())
}

func TestWAL_Repair(t *testing.T) {
	for name, cf := range map[string]func(f *os.File){
		"bad_fragment_sequence": func(f *os.File) {
			_, err := f.Seek(pageSize, 0)
			testutil.Ok(t, err)
			_, err = f.Write([]byte{byte(recLast)})
			testutil.Ok(t, err)
		},
		"bad_fragment_flag": func(f *os.File) {
			_, err := f.Seek(pageSize, 0)
			testutil.Ok(t, err)
			_, err = f.Write([]byte{123})
			testutil.Ok(t, err)
		},
		"bad_checksum": func(f *os.File) {
			_, err := f.Seek(pageSize+4, 0)
			testutil.Ok(t, err)
			_, err = f.Write([]byte{0})
			testutil.Ok(t, err)
		},
		"bad_length": func(f *os.File) {
			_, err := f.Seek(pageSize+2, 0)
			testutil.Ok(t, err)
			_, err = f.Write([]byte{0})
			testutil.Ok(t, err)
		},
		"bad_content": func(f *os.File) {
			_, err := f.Seek(pageSize+100, 0)
			testutil.Ok(t, err)
			_, err = f.Write([]byte("beef"))
			testutil.Ok(t, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "wal_repair")
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			// We create 3 segments with 3 records each and then corrupt the 2nd record
			// of the 2nd segment.
			// As a result we want a repaired WAL with the first 4 records intact.
			w, err := NewSize(nil, nil, dir, 3*pageSize)
			testutil.Ok(t, err)

			var records [][]byte

			for i := 1; i <= 9; i++ {
				b := make([]byte, pageSize-recordHeaderSize)
				b[0] = byte(i)
				records = append(records, b)
				testutil.Ok(t, w.Log(b))
			}
			testutil.Ok(t, w.Close())

			f, err := os.OpenFile(SegmentName(dir, 1), os.O_RDWR, 0666)
			testutil.Ok(t, err)

			// Apply corruption function.
			cf(f)

			testutil.Ok(t, f.Close())

			w, err = New(nil, nil, dir)
			testutil.Ok(t, err)

			sr, err := NewSegmentsReader(dir)
			testutil.Ok(t, err)
			r := NewReader(sr)

			for r.Next() {
			}
			testutil.NotOk(t, r.Err())

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
			testutil.Equals(t, 4, len(result))

			for i, r := range result {
				if !bytes.Equal(records[i], r) {
					t.Fatalf("record %d diverges: want %x, got %x", i, records[i][:10], r[:10])
				}
			}
		})
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
