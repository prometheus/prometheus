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
	"github.com/stretchr/testify/assert"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wal"
)

func TestSegmentWAL_cut(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_wal_cut")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpdir))
	}()

	// This calls cut() implicitly the first time without a previous tail.
	w, err := OpenSegmentWAL(tmpdir, nil, 0, nil)
	assert.NoError(t, err)

	assert.NoError(t, w.write(WALEntrySeries, 1, []byte("Hello World!!")))

	assert.NoError(t, w.cut())

	// Cutting creates a new file.
	assert.Equal(t, 2, len(w.files))

	assert.NoError(t, w.write(WALEntrySeries, 1, []byte("Hello World!!")))

	assert.NoError(t, w.Close())

	for _, of := range w.files {
		f, err := os.Open(of.Name())
		assert.NoError(t, err)

		// Verify header data.
		metab := make([]byte, 8)
		_, err = f.Read(metab)
		assert.NoError(t, err)
		assert.Equal(t, WALMagic, binary.BigEndian.Uint32(metab[:4]))
		assert.Equal(t, WALFormatDefault, metab[4])

		// We cannot actually check for correct pre-allocation as it is
		// optional per filesystem and handled transparently.
		et, flag, b, err := newWALReader(nil, nil).entry(f)
		assert.NoError(t, err)
		assert.Equal(t, WALEntrySeries, et)
		assert.Equal(t, byte(walSeriesSimple), flag)
		assert.Equal(t, []byte("Hello World!!"), b)
	}
}

func TestSegmentWAL_Truncate(t *testing.T) {
	const (
		numMetrics = 20000
		batch      = 100
	)
	series, err := labels.ReadLabels(filepath.Join("testdata", "20kseries.json"), numMetrics)
	assert.NoError(t, err)

	dir, err := ioutil.TempDir("", "test_wal_log_truncate")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()

	w, err := OpenSegmentWAL(dir, nil, 0, nil)
	assert.NoError(t, err)
	defer func(wal *SegmentWAL) { assert.NoError(t, wal.Close()) }(w)
	w.segmentSize = 10000

	for i := 0; i < numMetrics; i += batch {
		var rs []record.RefSeries

		for j, s := range series[i : i+batch] {
			rs = append(rs, record.RefSeries{Labels: s, Ref: uint64(i+j) + 1})
		}
		err := w.LogSeries(rs)
		assert.NoError(t, err)
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
	assert.NoError(t, err)

	var expected []record.RefSeries

	for i := 1; i <= numMetrics; i++ {
		if i%2 == 1 || uint64(i) >= boundarySeries {
			expected = append(expected, record.RefSeries{Ref: uint64(i), Labels: series[i-1]})
		}
	}

	// Call Truncate once again to see whether we can read the written file without
	// creating a new WAL.
	err = w.Truncate(1000, keepf)
	assert.NoError(t, err)
	assert.NoError(t, w.Close())

	// The same again with a new WAL.
	w, err = OpenSegmentWAL(dir, nil, 0, nil)
	assert.NoError(t, err)
	defer func(wal *SegmentWAL) { assert.NoError(t, wal.Close()) }(w)

	var readSeries []record.RefSeries
	r := w.Reader()

	assert.NoError(t, r.Read(func(s []record.RefSeries) {
		readSeries = append(readSeries, s...)
	}, nil, nil))

	assert.Equal(t, expected, readSeries)
}

// Symmetrical test of reading and writing to the WAL via its main interface.
func TestSegmentWAL_Log_Restore(t *testing.T) {
	const (
		numMetrics = 50
		iterations = 5
		stepSize   = 5
	)
	// Generate testing data. It does not make semantic sense but
	// for the purpose of this test.
	series, err := labels.ReadLabels(filepath.Join("testdata", "20kseries.json"), numMetrics)
	assert.NoError(t, err)

	dir, err := ioutil.TempDir("", "test_wal_log_restore")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()

	var (
		recordedSeries  [][]record.RefSeries
		recordedSamples [][]record.RefSample
		recordedDeletes [][]tombstones.Stone
	)
	var totalSamples int

	// Open WAL a bunch of times, validate all previous data can be read,
	// write more data to it, close it.
	for k := 0; k < numMetrics; k += numMetrics / iterations {
		w, err := OpenSegmentWAL(dir, nil, 0, nil)
		assert.NoError(t, err)

		// Set smaller segment size so we can actually write several files.
		w.segmentSize = 1000 * 1000

		r := w.Reader()

		var (
			resultSeries  [][]record.RefSeries
			resultSamples [][]record.RefSample
			resultDeletes [][]tombstones.Stone
		)

		serf := func(series []record.RefSeries) {
			if len(series) > 0 {
				clsets := make([]record.RefSeries, len(series))
				copy(clsets, series)
				resultSeries = append(resultSeries, clsets)
			}
		}
		smplf := func(smpls []record.RefSample) {
			if len(smpls) > 0 {
				csmpls := make([]record.RefSample, len(smpls))
				copy(csmpls, smpls)
				resultSamples = append(resultSamples, csmpls)
			}
		}

		delf := func(stones []tombstones.Stone) {
			if len(stones) > 0 {
				cst := make([]tombstones.Stone, len(stones))
				copy(cst, stones)
				resultDeletes = append(resultDeletes, cst)
			}
		}

		assert.NoError(t, r.Read(serf, smplf, delf))

		assert.Equal(t, recordedSamples, resultSamples)
		assert.Equal(t, recordedSeries, resultSeries)
		assert.Equal(t, recordedDeletes, resultDeletes)

		series := series[k : k+(numMetrics/iterations)]

		// Insert in batches and generate different amounts of samples for each.
		for i := 0; i < len(series); i += stepSize {
			var samples []record.RefSample
			var stones []tombstones.Stone

			for j := 0; j < i*10; j++ {
				samples = append(samples, record.RefSample{
					Ref: uint64(j % 10000),
					T:   int64(j * 2),
					V:   rand.Float64(),
				})
			}

			for j := 0; j < i*20; j++ {
				ts := rand.Int63()
				stones = append(stones, tombstones.Stone{Ref: rand.Uint64(), Intervals: tombstones.Intervals{{Mint: ts, Maxt: ts + rand.Int63n(10000)}}})
			}

			lbls := series[i : i+stepSize]
			series := make([]record.RefSeries, 0, len(series))
			for j, l := range lbls {
				series = append(series, record.RefSeries{
					Ref:    uint64(i + j),
					Labels: l,
				})
			}

			assert.NoError(t, w.LogSeries(series))
			assert.NoError(t, w.LogSamples(samples))
			assert.NoError(t, w.LogDeletes(stones))

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

		assert.NoError(t, w.Close())
	}
}

func TestWALRestoreCorrupted_invalidSegment(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_wal_log_restore")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()

	wal, err := OpenSegmentWAL(dir, nil, 0, nil)
	assert.NoError(t, err)
	defer func(wal *SegmentWAL) { assert.NoError(t, wal.Close()) }(wal)

	_, err = wal.createSegmentFile(filepath.Join(dir, "000000"))
	assert.NoError(t, err)
	f, err := wal.createSegmentFile(filepath.Join(dir, "000001"))
	assert.NoError(t, err)
	f2, err := wal.createSegmentFile(filepath.Join(dir, "000002"))
	assert.NoError(t, err)
	assert.NoError(t, f2.Close())

	// Make header of second segment invalid.
	_, err = f.WriteAt([]byte{1, 2, 3, 4}, 0)
	assert.NoError(t, err)
	assert.NoError(t, f.Close())

	assert.NoError(t, wal.Close())

	wal, err = OpenSegmentWAL(dir, log.NewLogfmtLogger(os.Stderr), 0, nil)
	assert.NoError(t, err)
	defer func(wal *SegmentWAL) { assert.NoError(t, wal.Close()) }(wal)

	files, err := ioutil.ReadDir(dir)
	assert.NoError(t, err)
	fns := []string{}
	for _, f := range files {
		fns = append(fns, f.Name())
	}
	assert.Equal(t, []string{"000000"}, fns)
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
				assert.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				assert.NoError(t, err)

				assert.NoError(t, f.Truncate(off-1))
			},
		},
		{
			name: "truncate_body",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				assert.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				assert.NoError(t, err)

				assert.NoError(t, f.Truncate(off-8))
			},
		},
		{
			name: "body_content",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				assert.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				assert.NoError(t, err)

				// Write junk before checksum starts.
				_, err = f.WriteAt([]byte{1, 2, 3, 4}, off-8)
				assert.NoError(t, err)
			},
		},
		{
			name: "checksum",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				assert.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, io.SeekEnd)
				assert.NoError(t, err)

				// Write junk into checksum
				_, err = f.WriteAt([]byte{1, 2, 3, 4}, off-4)
				assert.NoError(t, err)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Generate testing data. It does not make semantic sense but
			// for the purpose of this test.
			dir, err := ioutil.TempDir("", "test_corrupted")
			assert.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(dir))
			}()

			w, err := OpenSegmentWAL(dir, nil, 0, nil)
			assert.NoError(t, err)
			defer func(wal *SegmentWAL) { assert.NoError(t, wal.Close()) }(w)

			assert.NoError(t, w.LogSamples([]record.RefSample{{T: 1, V: 2}}))
			assert.NoError(t, w.LogSamples([]record.RefSample{{T: 2, V: 3}}))

			assert.NoError(t, w.cut())

			// Sleep 2 seconds to avoid error where cut and test "cases" function may write or
			// truncate the file out of orders as "cases" are not synchronized with cut.
			// Hopefully cut will complete by 2 seconds.
			time.Sleep(2 * time.Second)

			assert.NoError(t, w.LogSamples([]record.RefSample{{T: 3, V: 4}}))
			assert.NoError(t, w.LogSamples([]record.RefSample{{T: 5, V: 6}}))

			assert.NoError(t, w.Close())

			// cut() truncates and fsyncs the first segment async. If it happens after
			// the corruption we apply below, the corruption will be overwritten again.
			// Fire and forget a sync to avoid flakiness.
			w.files[0].Sync()
			// Corrupt the second entry in the first file.
			// After re-opening we must be able to read the first entry
			// and the rest, including the second file, must be truncated for clean further
			// writes.
			c.f(t, w)

			logger := log.NewLogfmtLogger(os.Stderr)

			w2, err := OpenSegmentWAL(dir, logger, 0, nil)
			assert.NoError(t, err)
			defer func(wal *SegmentWAL) { assert.NoError(t, wal.Close()) }(w2)

			r := w2.Reader()

			serf := func(l []record.RefSeries) {
				assert.Equal(t, 0, len(l))
			}

			// Weird hack to check order of reads.
			i := 0
			samplef := func(s []record.RefSample) {
				if i == 0 {
					assert.Equal(t, []record.RefSample{{T: 1, V: 2}}, s)
					i++
				} else {
					assert.Equal(t, []record.RefSample{{T: 99, V: 100}}, s)
				}
			}

			assert.NoError(t, r.Read(serf, samplef, nil))

			assert.NoError(t, w2.LogSamples([]record.RefSample{{T: 99, V: 100}}))
			assert.NoError(t, w2.Close())

			// We should see the first valid entry and the new one, everything after
			// is truncated.
			w3, err := OpenSegmentWAL(dir, logger, 0, nil)
			assert.NoError(t, err)
			defer func(wal *SegmentWAL) { assert.NoError(t, wal.Close()) }(w3)

			r = w3.Reader()

			i = 0
			assert.NoError(t, r.Read(serf, samplef, nil))
		})
	}
}

func TestMigrateWAL_Empty(t *testing.T) {
	// The migration procedure must properly deal with a zero-length segment,
	// which is valid in the new format.
	dir, err := ioutil.TempDir("", "walmigrate")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()

	wdir := path.Join(dir, "wal")

	// Initialize empty WAL.
	w, err := wal.New(nil, nil, wdir, false)
	assert.NoError(t, err)
	assert.NoError(t, w.Close())

	assert.NoError(t, MigrateWAL(nil, wdir))
}

func TestMigrateWAL_Fuzz(t *testing.T) {
	dir, err := ioutil.TempDir("", "walmigrate")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()

	wdir := path.Join(dir, "wal")

	// Should pass if no WAL exists yet.
	assert.NoError(t, MigrateWAL(nil, wdir))

	oldWAL, err := OpenSegmentWAL(wdir, nil, time.Minute, nil)
	assert.NoError(t, err)

	// Write some data.
	assert.NoError(t, oldWAL.LogSeries([]record.RefSeries{
		{Ref: 100, Labels: labels.FromStrings("abc", "def", "123", "456")},
		{Ref: 1, Labels: labels.FromStrings("abc", "def2", "1234", "4567")},
	}))
	assert.NoError(t, oldWAL.LogSamples([]record.RefSample{
		{Ref: 1, T: 100, V: 200},
		{Ref: 2, T: 300, V: 400},
	}))
	assert.NoError(t, oldWAL.LogSeries([]record.RefSeries{
		{Ref: 200, Labels: labels.FromStrings("xyz", "def", "foo", "bar")},
	}))
	assert.NoError(t, oldWAL.LogSamples([]record.RefSample{
		{Ref: 3, T: 100, V: 200},
		{Ref: 4, T: 300, V: 400},
	}))
	assert.NoError(t, oldWAL.LogDeletes([]tombstones.Stone{
		{Ref: 1, Intervals: []tombstones.Interval{{Mint: 100, Maxt: 200}}},
	}))

	assert.NoError(t, oldWAL.Close())

	// Perform migration.
	assert.NoError(t, MigrateWAL(nil, wdir))

	w, err := wal.New(nil, nil, wdir, false)
	assert.NoError(t, err)

	// We can properly write some new data after migration.
	var enc record.Encoder
	assert.NoError(t, w.Log(enc.Samples([]record.RefSample{
		{Ref: 500, T: 1, V: 1},
	}, nil)))

	assert.NoError(t, w.Close())

	// Read back all data.
	sr, err := wal.NewSegmentsReader(wdir)
	assert.NoError(t, err)

	r := wal.NewReader(sr)
	var res []interface{}
	var dec record.Decoder

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			s, err := dec.Series(rec, nil)
			assert.NoError(t, err)
			res = append(res, s)
		case record.Samples:
			s, err := dec.Samples(rec, nil)
			assert.NoError(t, err)
			res = append(res, s)
		case record.Tombstones:
			s, err := dec.Tombstones(rec, nil)
			assert.NoError(t, err)
			res = append(res, s)
		default:
			t.Fatalf("unknown record type %d", dec.Type(rec))
		}
	}
	assert.NoError(t, r.Err())

	assert.Equal(t, []interface{}{
		[]record.RefSeries{
			{Ref: 100, Labels: labels.FromStrings("abc", "def", "123", "456")},
			{Ref: 1, Labels: labels.FromStrings("abc", "def2", "1234", "4567")},
		},
		[]record.RefSample{{Ref: 1, T: 100, V: 200}, {Ref: 2, T: 300, V: 400}},
		[]record.RefSeries{
			{Ref: 200, Labels: labels.FromStrings("xyz", "def", "foo", "bar")},
		},
		[]record.RefSample{{Ref: 3, T: 100, V: 200}, {Ref: 4, T: 300, V: 400}},
		[]tombstones.Stone{{Ref: 1, Intervals: []tombstones.Interval{{Mint: 100, Maxt: 200}}}},
		[]record.RefSample{{Ref: 500, T: 1, V: 1}},
	}, res)

	// Migrating an already migrated WAL shouldn't do anything.
	assert.NoError(t, MigrateWAL(nil, wdir))
}
