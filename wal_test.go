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

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/tsdb/labels"
	"github.com/stretchr/testify/require"
)

func TestSegmentWAL_initSegments(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_wal_open")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	df, err := fileutil.OpenDir(tmpdir)
	require.NoError(t, err)

	w := &SegmentWAL{dirFile: df}

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
	require.NoError(t, w.initSegments())

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

	w = &SegmentWAL{dirFile: df}
	require.Error(t, w.initSegments(), "init corrupted segments")

	for _, f := range w.files {
		require.NoError(t, f.Close())
	}
}

func TestSegmentWAL_cut(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_wal_cut")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	// This calls cut() implicitly the first time without a previous tail.
	w, err := OpenSegmentWAL(tmpdir, nil, 0)
	require.NoError(t, err)

	require.NoError(t, w.entry(WALEntrySeries, 1, []byte("Hello World!!")))

	require.NoError(t, w.cut(), "cut failed")

	// Cutting creates a new file and close the previous tail file.
	require.Equal(t, 2, len(w.files))
	require.Equal(t, os.ErrInvalid.Error(), w.files[0].Close().Error())

	require.NoError(t, w.entry(WALEntrySeries, 1, []byte("Hello World!!")))

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

// Symmetrical test of reading and writing to the WAL via its main interface.
func TestSegmentWAL_Log_Restore(t *testing.T) {
	const (
		numMetrics = 5000
		iterations = 5
		stepSize   = 100
	)
	// Generate testing data. It does not make semantical sense but
	// for the purpose of this test.
	series, err := readPrometheusLabels("testdata/20k.series", numMetrics)
	require.NoError(t, err)

	dir, err := ioutil.TempDir("", "test_wal_log_restore")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	var (
		recordedSeries  [][]labels.Labels
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
			resultSeries  [][]labels.Labels
			resultSamples [][]RefSample
			resultDeletes [][]Stone
		)

		serf := func(lsets []labels.Labels) error {
			if len(lsets) > 0 {
				clsets := make([]labels.Labels, len(lsets))
				copy(clsets, lsets)
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
				resultDeletes = append(resultDeletes, stones)
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
				stones = append(stones, Stone{rand.Uint32(), intervals{{ts, ts + rand.Int63n(10000)}}})
			}

			lbls := series[i : i+stepSize]

			require.NoError(t, w.LogSeries(lbls))
			require.NoError(t, w.LogSamples(samples))
			require.NoError(t, w.LogDeletes(stones))

			if len(lbls) > 0 {
				recordedSeries = append(recordedSeries, lbls)
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
			dir, err := ioutil.TempDir("", "test_corrupted_checksum")
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

			// Corrupt the second entry in the first file.
			// After re-opening we must be able to read the first entry
			// and the rest, including the second file, must be truncated for clean further
			// writes.
			c.f(t, w)

			logger := log.NewLogfmtLogger(os.Stderr)

			w2, err := OpenSegmentWAL(dir, logger, 0)
			require.NoError(t, err)

			r := w2.Reader()
			serf := func(l []labels.Labels) error {
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
