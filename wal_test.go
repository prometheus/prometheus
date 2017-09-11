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

package tsdb

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func TestSegmentWAL_Open(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_wal_open")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	// Create segment files with an appropriate header.
	for i := 1; i <= 5; i++ {
		metab := make([]byte, 8)
		binary.BigEndian.PutUint32(metab[:4], WALMagic)
		metab[4] = WALFormatDefault

		f, err := os.Create(fmt.Sprintf("%s/000%d", tmpdir, i))
		require.NoError(t, err)
		_, err = f.Write(metab)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}

	// Initialize 5 correct segment files.
	w, err := OpenSegmentWAL(tmpdir, nil, 0)
	require.NoError(t, err)

	require.Equal(t, 5, len(w.files), "unexpected number of segments loaded")

	// Validate that files are locked properly.
	for _, of := range w.files {
		f, err := os.Open(of.Name())
		require.NoError(t, err, "open locked segment %s", f.Name())

		_, err = f.Read([]byte{0})
		require.NoError(t, err, "read locked segment %s", f.Name())

		_, err = f.Write([]byte{0})
		require.Error(t, err, "write to tail segment file %s", f.Name())

		require.NoError(t, f.Close())
	}

	for _, f := range w.files {
		require.NoError(t, f.Close())
	}

	// Make initialization fail by corrupting the header of one file.
	f, err := os.OpenFile(w.files[3].Name(), os.O_WRONLY, 0666)
	require.NoError(t, err)

	_, err = f.WriteAt([]byte{0}, 4)
	require.NoError(t, err)

	w, err = OpenSegmentWAL(tmpdir, nil, 0)
	require.Error(t, err, "open with corrupted segments")
}

func TestSegmentWAL_cut(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_wal_cut")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	// This calls cut() implicitly the first time without a previous tail.
	w, err := OpenSegmentWAL(tmpdir, nil, 0)
	require.NoError(t, err)

	require.NoError(t, w.write(WALEntrySeries, 1, []byte("Hello World!!")))

	require.NoError(t, w.cut(), "cut failed")

	// Cutting creates a new file.
	require.Equal(t, 2, len(w.files))

	require.NoError(t, w.write(WALEntrySeries, 1, []byte("Hello World!!")))

	require.NoError(t, w.Close())

	for _, of := range w.files {
		f, err := os.Open(of.Name())
		require.NoError(t, err)

		// Verify header data.
		metab := make([]byte, 8)
		_, err = f.Read(metab)
		require.NoError(t, err, "read meta data %s", f.Name())
		require.Equal(t, WALMagic, binary.BigEndian.Uint32(metab[:4]), "verify magic")
		require.Equal(t, WALFormatDefault, metab[4], "verify format flag")

		// We cannot actually check for correct pre-allocation as it is
		// optional per filesystem and handled transparently.
		et, flag, b, err := newWALReader(nil, nil).entry(f)
		require.NoError(t, err)
		require.Equal(t, WALEntrySeries, et)
		require.Equal(t, flag, byte(walSeriesSimple))
		require.Equal(t, []byte("Hello World!!"), b)
	}
}

func TestSegmentWAL_Truncate(t *testing.T) {
	const (
		numMetrics = 20000
		batch      = 100
	)
	series, err := readPrometheusLabels("testdata/20k.series", numMetrics)
	require.NoError(t, err)

	dir, err := ioutil.TempDir("", "test_wal_log_truncate")
	require.NoError(t, err)
	// defer os.RemoveAll(dir)

	w, err := OpenSegmentWAL(dir, nil, 0)
	require.NoError(t, err)
	w.segmentSize = 10000

	for i := 0; i < numMetrics; i += batch {
		var rs []RefSeries

		for j, s := range series[i : i+batch] {
			rs = append(rs, RefSeries{Labels: s, Ref: uint64(i+j) + 1})
		}
		err := w.LogSeries(rs)
		require.NoError(t, err)
	}

	// We mark the 2nd half of the files with a min timestamp that should discard
	// them from the selection of compactable files.
	for i, f := range w.files[len(w.files)/2:] {
		f.maxTime = int64(1000 + i)
	}
	// All series in those files must be preserved regarding of the provided postings list.
	boundarySeries := w.files[len(w.files)/2].minSeries

	// We truncate while keeping every 2nd series.
	keep := []uint64{}
	for i := 1; i <= numMetrics; i += 2 {
		keep = append(keep, uint64(i))
	}

	err = w.Truncate(1000, newListPostings(keep))
	require.NoError(t, err)

	var expected []RefSeries

	for i := 1; i <= numMetrics; i++ {
		if i%2 == 1 || uint64(i) >= boundarySeries {
			expected = append(expected, RefSeries{Ref: uint64(i), Labels: series[i-1]})
		}
	}

	// Call Truncate once again to see whether we can read the written file without
	// creating a new WAL.
	err = w.Truncate(1000, newListPostings(keep))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// The same again with a new WAL.
	w, err = OpenSegmentWAL(dir, nil, 0)
	require.NoError(t, err)

	var readSeries []RefSeries
	r := w.Reader()

	r.Read(func(s []RefSeries) error {
		readSeries = append(readSeries, s...)
		return nil
	}, nil, nil)

	require.Equal(t, expected, readSeries)
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
	series, err := readPrometheusLabels("testdata/20k.series", numMetrics)
	require.NoError(t, err)

	dir, err := ioutil.TempDir("", "test_wal_log_restore")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	var (
		recordedSeries  [][]RefSeries
		recordedSamples [][]RefSample
		recordedDeletes [][]Stone
	)
	var totalSamples int

	// Open WAL a bunch of times, validate all previous data can be read,
	// write more data to it, close it.
	for k := 0; k < numMetrics; k += numMetrics / iterations {
		w, err := OpenSegmentWAL(dir, nil, 0)
		require.NoError(t, err)

		// Set smaller segment size so we can actually write several files.
		w.segmentSize = 1000 * 1000

		r := w.Reader()

		var (
			resultSeries  [][]RefSeries
			resultSamples [][]RefSample
			resultDeletes [][]Stone
		)

		serf := func(series []RefSeries) error {
			if len(series) > 0 {
				clsets := make([]RefSeries, len(series))
				copy(clsets, series)
				resultSeries = append(resultSeries, clsets)
			}

			return nil
		}
		smplf := func(smpls []RefSample) error {
			if len(smpls) > 0 {
				csmpls := make([]RefSample, len(smpls))
				copy(csmpls, smpls)
				resultSamples = append(resultSamples, csmpls)
			}

			return nil
		}

		delf := func(stones []Stone) error {
			if len(stones) > 0 {
				cst := make([]Stone, len(stones))
				copy(cst, stones)
				resultDeletes = append(resultDeletes, cst)
			}

			return nil
		}

		require.NoError(t, r.Read(serf, smplf, delf))

		require.Equal(t, recordedSamples, resultSamples)
		require.Equal(t, recordedSeries, resultSeries)
		require.Equal(t, recordedDeletes, resultDeletes)

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

			require.NoError(t, w.LogSeries(series))
			require.NoError(t, w.LogSamples(samples))
			require.NoError(t, w.LogDeletes(stones))

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

		require.NoError(t, w.Close())
	}
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
				require.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, os.SEEK_END)
				require.NoError(t, err)

				require.NoError(t, f.Truncate(off-1))
			},
		},
		{
			name: "truncate_body",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				require.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, os.SEEK_END)
				require.NoError(t, err)

				require.NoError(t, f.Truncate(off-8))
			},
		},
		{
			name: "body_content",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				require.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, os.SEEK_END)
				require.NoError(t, err)

				// Write junk before checksum starts.
				_, err = f.WriteAt([]byte{1, 2, 3, 4}, off-8)
				require.NoError(t, err)
			},
		},
		{
			name: "checksum",
			f: func(t *testing.T, w *SegmentWAL) {
				f, err := os.OpenFile(w.files[0].Name(), os.O_WRONLY, 0666)
				require.NoError(t, err)
				defer f.Close()

				off, err := f.Seek(0, os.SEEK_END)
				require.NoError(t, err)

				// Write junk into checksum
				_, err = f.WriteAt([]byte{1, 2, 3, 4}, off-4)
				require.NoError(t, err)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Generate testing data. It does not make semantical sense but
			// for the purpose of this test.
			dir, err := ioutil.TempDir("", "test_corrupted")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			w, err := OpenSegmentWAL(dir, nil, 0)
			require.NoError(t, err)

			require.NoError(t, w.LogSamples([]RefSample{{T: 1, V: 2}}))
			require.NoError(t, w.LogSamples([]RefSample{{T: 2, V: 3}}))

			require.NoError(t, w.cut())

			require.NoError(t, w.LogSamples([]RefSample{{T: 3, V: 4}}))
			require.NoError(t, w.LogSamples([]RefSample{{T: 5, V: 6}}))

			require.NoError(t, w.Close())

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

			w2, err := OpenSegmentWAL(dir, logger, 0)
			require.NoError(t, err)

			r := w2.Reader()

			serf := func(l []RefSeries) error {
				require.Equal(t, 0, len(l))
				return nil
			}
			delf := func([]Stone) error { return nil }

			// Weird hack to check order of reads.
			i := 0
			samplf := func(s []RefSample) error {
				if i == 0 {
					require.Equal(t, []RefSample{{T: 1, V: 2}}, s)
					i++
				} else {
					require.Equal(t, []RefSample{{T: 99, V: 100}}, s)
				}

				return nil
			}

			require.NoError(t, r.Read(serf, samplf, delf))

			require.NoError(t, w2.LogSamples([]RefSample{{T: 99, V: 100}}))
			require.NoError(t, w2.Close())

			// We should see the first valid entry and the new one, everything after
			// is truncated.
			w3, err := OpenSegmentWAL(dir, logger, 0)
			require.NoError(t, err)

			r = w3.Reader()

			i = 0
			require.NoError(t, r.Read(serf, samplf, delf))
		})
	}
}
