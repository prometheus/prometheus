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

// +build !windows

package tsdb

import (
	"encoding/binary"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/tsdb/fileutil"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/testutil"
	"github.com/prometheus/tsdb/wal"
)

func TestSegmentWAL_cut(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_wal_cut")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(tmpdir))
	}()

	// This calls cut() implicitly the first time without a previous tail.
	w, err := OpenSegmentWAL(tmpdir, nil, 0, nil)
	testutil.Ok(t, err)

	testutil.Ok(t, w.write(WALEntrySeries, 1, []byte("Hello World!!")))

	testutil.Ok(t, w.cut())

	// Cutting creates a new file.
	testutil.Equals(t, 2, len(w.files))

	testutil.Ok(t, w.write(WALEntrySeries, 1, []byte("Hello World!!")))

	testutil.Ok(t, w.Close())

	for _, of := range w.files {
		f, err := os.Open(of.Name())
		testutil.Ok(t, err)

		// Verify header data.
		metab := make([]byte, 8)
		_, err = f.Read(metab)
		testutil.Ok(t, err)
		testutil.Equals(t, WALMagic, binary.BigEndian.Uint32(metab[:4]))
		testutil.Equals(t, WALFormatDefault, metab[4])

		// We cannot actually check for correct pre-allocation as it is
		// optional per filesystem and handled transparently.
		et, flag, b, err := newWALReader(nil, nil).entry(f)
		testutil.Ok(t, err)
		testutil.Equals(t, WALEntrySeries, et)
		testutil.Equals(t, byte(walSeriesSimple), flag)
		testutil.Equals(t, []byte("Hello World!!"), b)
	}
}

func TestSegmentWAL_Truncate(t *testing.T) {
	const (
		numMetrics = 20000
		batch      = 100
	)
	series, err := labels.ReadLabels(filepath.Join("testdata", "20kseries.json"), numMetrics)
	testutil.Ok(t, err)

	dir, err := ioutil.TempDir("", "test_wal_log_truncate")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	w, err := OpenSegmentWAL(dir, nil, 0, nil)
	testutil.Ok(t, err)
	w.segmentSize = 10000

	for i := 0; i < numMetrics; i += batch {
		var rs []RefSeries

		for j, s := range series[i : i+batch] {
			rs = append(rs, RefSeries{Labels: s, Ref: uint64(i+j) + 1})
		}
		err := w.LogSeries(rs)
		testutil.Ok(t, err)
	}

	// We mark the 2nd half of the files with a min timestamp that should discard
	// them from the selection of compactable files.
	for i, f := range w.files[len(w.files)/2:] {
		f.maxTime = int64(1000 + i)
	}
	// All series in those files must be preserved regarding of the provided postings list.
	boundarySeries := w.files[len(w.files)/2].minSeries

	// We truncate while keeping every 2nd series.
	keep := map[uint64]struct{}{}
	for i := 1; i <= numMetrics; i += 2 {
		keep[uint64(i)] = struct{}{}
	}
	keepf := func(id uint64) bool {
		_, ok := keep[id]
		return ok
	}

	err = w.Truncate(1000, keepf)
	testutil.Ok(t, err)

	var expected []RefSeries

	for i := 1; i <= numMetrics; i++ {
		if i%2 == 1 || uint64(i) >= boundarySeries {
			expected = append(expected, RefSeries{Ref: uint64(i), Labels: series[i-1]})
		}
	}

	// Call Truncate once again to see whether we can read the written file without
	// creating a new WAL.
	err = w.Truncate(1000, keepf)
	testutil.Ok(t, err)
	testutil.Ok(t, w.Close())

	// The same again with a new WAL.
	w, err = OpenSegmentWAL(dir, nil, 0, nil)
	testutil.Ok(t, err)

	var readSeries []RefSeries
	r := w.Reader()

	testutil.Ok(t, r.Read(func(s []RefSeries) {
		readSeries = append(readSeries, s...)
	}, nil, nil))

	testutil.Equals(t, expected, readSeries)
}

// Symmetrical test of reading and writing to the WAL via its main interface.
func TestSegmentWAL_Log_Restore(t *testing.T) {
	const (
		numMetrics = 50
		iterations = 5
		stepSize   = 5
	)
	// Generate testing data. It does not make semantical sense but
	// for the purpose of this test.
	series, err := labels.ReadLabels(filepath.Join("testdata", "20kseries.json"), numMetrics)
	testutil.Ok(t, err)

	dir, err := ioutil.TempDir("", "test_wal_log_restore")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	var (
		recordedSeries  [][]RefSeries
		recordedSamples [][]RefSample
		recordedDeletes [][]Stone
	)
	var totalSamples int

	// Open WAL a bunch of times, validate all previous data can be read,
	// write more data to it, close it.
	for k := 0; k < numMetrics; k += numMetrics / iterations {
		w, err := OpenSegmentWAL(dir, nil, 0, nil)
		testutil.Ok(t, err)

		// Set smaller segment size so we can actually write several files.
		w.segmentSize = 1000 * 1000

		r := w.Reader()

		var (
			resultSeries  [][]RefSeries
			resultSamples [][]RefSample
			resultDeletes [][]Stone
		)

		serf := func(series []RefSeries) {
			if len(series) > 0 {
				clsets := make([]RefSeries, len(series))
				copy(clsets, series)
				resultSeries = append(resultSeries, clsets)
			}
		}
		smplf := func(smpls []RefSample) {
			if len(smpls) > 0 {
				csmpls := make([]RefSample, len(smpls))
				copy(csmpls, smpls)
				resultSamples = append(resultSamples, csmpls)
			}
		}

		delf := func(stones []Stone) {
			if len(stones) > 0 {
				cst := make([]Stone, len(stones))
				copy(cst, stones)
				resultDeletes = append(resultDeletes, cst)
			}
		}

		testutil.Ok(t, r.Read(serf, smplf, delf))

		testutil.Equals(t, recordedSamples, resultSamples)
		testutil.Equals(t, recordedSeries, resultSeries)
		testutil.Equals(t, recordedDeletes, resultDeletes)

		series := series[k : k+(numMetrics/iterations)]

		// Insert in batches and generate different amounts of samples for each.
		for i := 0; i < len(series); i += stepSize {
			var samples []RefSample
			var stones []Stone

			for j := 0; j < i*10; j++ {
				samples = append(samples, RefSample{
					Ref: uint64(j % 10000),
					T:   int64(j * 2),
					V:   rand.Float64(),
				})
			}

			for j := 0; j < i*20; j++ {
				ts := rand.Int63()
				stones = append(stones, Stone{rand.Uint64(), Intervals{{ts, ts + rand.Int63n(10000)}}})
			}

			lbls := series[i : i+stepSize]
			series := make([]RefSeries, 0, len(series))
			for j, l := range lbls {
				series = append(series, RefSeries{
					Ref:    uint64(i + j),
					Labels: l,
				})
			}

			testutil.Ok(t, w.LogSeries(series))
			testutil.Ok(t, w.LogSamples(samples))
			testutil.Ok(t, w.LogDeletes(stones))

			if len(lbls) > 0 {
				recordedSeries = append(recordedSeries, series)
			}
			if len(samples) > 0 {
				recordedSamples = append(recordedSamples, samples)
				totalSamples += len(samples)
			}
			if len(stones) > 0 {
				recordedDeletes = append(recordedDeletes, stones)
			}
		}

		testutil.Ok(t, w.Close())
	}
}

func TestWALRestoreCorrupted_invalidSegment(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_wal_log_restore")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	wal, err := OpenSegmentWAL(dir, nil, 0, nil)
	testutil.Ok(t, err)

	_, err = wal.createSegmentFile(filepath.Join(dir, "000000"))
	testutil.Ok(t, err)
	f, err := wal.createSegmentFile(filepath.Join(dir, "000001"))
	testutil.Ok(t, err)
	f2, err := wal.createSegmentFile(filepath.Join(dir, "000002"))
	testutil.Ok(t, err)
	testutil.Ok(t, f2.Close())

	// Make header of second segment invalid.
	_, err = f.WriteAt([]byte{1, 2, 3, 4}, 0)
	testutil.Ok(t, err)
	testutil.Ok(t, f.Close())

	testutil.Ok(t, wal.Close())

	_, err = OpenSegmentWAL(dir, log.NewLogfmtLogger(os.Stderr), 0, nil)
	testutil.Ok(t, err)

	fns, err := fileutil.ReadDir(dir)
	testutil.Ok(t, err)
	testutil.Equals(t, []string{"000000"}, fns)
}

// Test reading from a WAL that has been corrupted through various means.
func TestWALRestoreCorrupted(t *testing.T) {
	cases := []struct {
		name string
		f    func(*testing.T, *SegmentWAL)
	}{
		{
			name: "truncate_checksum",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				testutil.Ok(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				testutil.Ok(t, err)

				testutil.Ok(t, f.Truncate(off-1))
			},
		},
		{
			name: "truncate_body",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				testutil.Ok(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				testutil.Ok(t, err)

				testutil.Ok(t, f.Truncate(off-8))
			},
		},
		{
			name: "body_content",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				testutil.Ok(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				testutil.Ok(t, err)

				// Write junk before checksum starts.
				_, err = f.WriteAt([]byte{1, 2, 3, 4}, off-8)
				testutil.Ok(t, err)
			},
		},
		{
			name: "checksum",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				testutil.Ok(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				testutil.Ok(t, err)

				// Write junk into checksum
				_, err = f.WriteAt([]byte{1, 2, 3, 4}, off-4)
				testutil.Ok(t, err)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Generate testing data. It does not make semantical sense but
			// for the purpose of this test.
			dir, err := ioutil.TempDir("", "test_corrupted")
			testutil.Ok(t, err)
			defer func() {
				testutil.Ok(t, os.RemoveAll(dir))
			}()

			w, err := OpenSegmentWAL(dir, nil, 0, nil)
			testutil.Ok(t, err)

			testutil.Ok(t, w.LogSamples([]RefSample{{T: 1, V: 2}}))
			testutil.Ok(t, w.LogSamples([]RefSample{{T: 2, V: 3}}))

			testutil.Ok(t, w.cut())

			// Sleep 2 seconds to avoid error where cut and test "cases" function may write or
			// truncate the file out of orders as "cases" are not synchronized with cut.
			// Hopefully cut will complete by 2 seconds.
			time.Sleep(2 * time.Second)

			testutil.Ok(t, w.LogSamples([]RefSample{{T: 3, V: 4}}))
			testutil.Ok(t, w.LogSamples([]RefSample{{T: 5, V: 6}}))

			testutil.Ok(t, w.Close())

			// cut() truncates and fsyncs the first segment async. If it happens after
			// the corruption we apply below, the corruption will be overwritten again.
			// Fire and forget a sync to avoid flakyness.
			w.files[0].Sync()
			// Corrupt the second entry in the first file.
			// After re-opening we must be able to read the first entry
			// and the rest, including the second file, must be truncated for clean further
			// writes.
			c.f(t, w)

			logger := log.NewLogfmtLogger(os.Stderr)

			w2, err := OpenSegmentWAL(dir, logger, 0, nil)
			testutil.Ok(t, err)

			r := w2.Reader()

			serf := func(l []RefSeries) {
				testutil.Equals(t, 0, len(l))
			}

			// Weird hack to check order of reads.
			i := 0
			samplf := func(s []RefSample) {
				if i == 0 {
					testutil.Equals(t, []RefSample{{T: 1, V: 2}}, s)
					i++
				} else {
					testutil.Equals(t, []RefSample{{T: 99, V: 100}}, s)
				}
			}

			testutil.Ok(t, r.Read(serf, samplf, nil))

			testutil.Ok(t, w2.LogSamples([]RefSample{{T: 99, V: 100}}))
			testutil.Ok(t, w2.Close())

			// We should see the first valid entry and the new one, everything after
			// is truncated.
			w3, err := OpenSegmentWAL(dir, logger, 0, nil)
			testutil.Ok(t, err)

			r = w3.Reader()

			i = 0
			testutil.Ok(t, r.Read(serf, samplf, nil))
		})
	}
}

func TestMigrateWAL_Empty(t *testing.T) {
	// The migration proecedure must properly deal with a zero-length segment,
	// which is valid in the new format.
	dir, err := ioutil.TempDir("", "walmigrate")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	wdir := path.Join(dir, "wal")

	// Initialize empty WAL.
	w, err := wal.New(nil, nil, wdir, false)
	testutil.Ok(t, err)
	testutil.Ok(t, w.Close())

	testutil.Ok(t, MigrateWAL(nil, wdir))
}

func TestMigrateWAL_Fuzz(t *testing.T) {
	dir, err := ioutil.TempDir("", "walmigrate")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	wdir := path.Join(dir, "wal")

	// Should pass if no WAL exists yet.
	testutil.Ok(t, MigrateWAL(nil, wdir))

	oldWAL, err := OpenSegmentWAL(wdir, nil, time.Minute, nil)
	testutil.Ok(t, err)

	// Write some data.
	testutil.Ok(t, oldWAL.LogSeries([]RefSeries{
		{Ref: 100, Labels: labels.FromStrings("abc", "def", "123", "456")},
		{Ref: 1, Labels: labels.FromStrings("abc", "def2", "1234", "4567")},
	}))
	testutil.Ok(t, oldWAL.LogSamples([]RefSample{
		{Ref: 1, T: 100, V: 200},
		{Ref: 2, T: 300, V: 400},
	}))
	testutil.Ok(t, oldWAL.LogSeries([]RefSeries{
		{Ref: 200, Labels: labels.FromStrings("xyz", "def", "foo", "bar")},
	}))
	testutil.Ok(t, oldWAL.LogSamples([]RefSample{
		{Ref: 3, T: 100, V: 200},
		{Ref: 4, T: 300, V: 400},
	}))
	testutil.Ok(t, oldWAL.LogDeletes([]Stone{
		{ref: 1, intervals: []Interval{{100, 200}}},
	}))

	testutil.Ok(t, oldWAL.Close())

	// Perform migration.
	testutil.Ok(t, MigrateWAL(nil, wdir))

	w, err := wal.New(nil, nil, wdir, false)
	testutil.Ok(t, err)

	// We can properly write some new data after migration.
	var enc RecordEncoder
	testutil.Ok(t, w.Log(enc.Samples([]RefSample{
		{Ref: 500, T: 1, V: 1},
	}, nil)))

	testutil.Ok(t, w.Close())

	// Read back all data.
	sr, err := wal.NewSegmentsReader(wdir)
	testutil.Ok(t, err)

	r := wal.NewReader(sr)
	var res []interface{}
	var dec RecordDecoder

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case RecordSeries:
			s, err := dec.Series(rec, nil)
			testutil.Ok(t, err)
			res = append(res, s)
		case RecordSamples:
			s, err := dec.Samples(rec, nil)
			testutil.Ok(t, err)
			res = append(res, s)
		case RecordTombstones:
			s, err := dec.Tombstones(rec, nil)
			testutil.Ok(t, err)
			res = append(res, s)
		default:
			t.Fatalf("unknown record type %d", dec.Type(rec))
		}
	}
	testutil.Ok(t, r.Err())

	testutil.Equals(t, []interface{}{
		[]RefSeries{
			{Ref: 100, Labels: labels.FromStrings("abc", "def", "123", "456")},
			{Ref: 1, Labels: labels.FromStrings("abc", "def2", "1234", "4567")},
		},
		[]RefSample{{Ref: 1, T: 100, V: 200}, {Ref: 2, T: 300, V: 400}},
		[]RefSeries{
			{Ref: 200, Labels: labels.FromStrings("xyz", "def", "foo", "bar")},
		},
		[]RefSample{{Ref: 3, T: 100, V: 200}, {Ref: 4, T: 300, V: 400}},
		[]Stone{{ref: 1, intervals: []Interval{{100, 200}}}},
		[]RefSample{{Ref: 500, T: 1, V: 1}},
	}, res)

	// Migrating an already migrated WAL shouldn't do anything.
	testutil.Ok(t, MigrateWAL(nil, wdir))
}
