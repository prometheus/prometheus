// Copyright 2018 The Prometheus Authors
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
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/record"
)

var defaultRetryInterval = 100 * time.Millisecond
var defaultRetries = 100
var wMetrics = NewWatcherMetrics(prometheus.DefaultRegisterer)

// retry executes f() n times at each interval until it returns true.
func retry(t *testing.T, interval time.Duration, n int, f func() bool) {
	t.Helper()
	ticker := time.NewTicker(interval)
	for i := 0; i <= n; i++ {
		if f() {
			return
		}
		<-ticker.C
	}
	ticker.Stop()
	t.Logf("function returned false")
}

type writeToMock struct {
	samplesAppended      int
	exemplarsAppended    int
	seriesLock           sync.Mutex
	seriesSegmentIndexes map[uint64]int
}

func (wtm *writeToMock) Append(s []record.RefSample) bool {
	wtm.samplesAppended += len(s)
	return true
}

func (wtm *writeToMock) AppendExemplars(e []record.RefExemplar) bool {
	wtm.exemplarsAppended += len(e)
	return true
}

func (wtm *writeToMock) StoreSeries(series []record.RefSeries, index int) {
	wtm.seriesLock.Lock()
	defer wtm.seriesLock.Unlock()
	for _, s := range series {
		wtm.seriesSegmentIndexes[s.Ref] = index
	}
}

func (wtm *writeToMock) SeriesReset(index int) {
	// Check for series that are in segments older than the checkpoint
	// that were not also present in the checkpoint.
	wtm.seriesLock.Lock()
	defer wtm.seriesLock.Unlock()
	for k, v := range wtm.seriesSegmentIndexes {
		if v < index {
			delete(wtm.seriesSegmentIndexes, k)
		}
	}
}

func (wtm *writeToMock) checkNumLabels() int {
	wtm.seriesLock.Lock()
	defer wtm.seriesLock.Unlock()
	return len(wtm.seriesSegmentIndexes)
}

func newWriteToMock() *writeToMock {
	return &writeToMock{
		seriesSegmentIndexes: make(map[uint64]int),
	}
}

func TestTailSamples(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250
	const exemplarsCount = 25
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			now := time.Now()

			dir, err := ioutil.TempDir("", "readCheckpoint")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(dir))
			}()

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to the initial segment then checkpoint.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   now.UnixNano() + 1,
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}

				for j := 0; j < exemplarsCount; j++ {
					inner := rand.Intn(ref + 1)
					exemplar := enc.Exemplars([]record.RefExemplar{
						{
							Ref:    uint64(inner),
							T:      now.UnixNano() + 1,
							V:      float64(i),
							Labels: labels.FromStrings("traceID", fmt.Sprintf("trace-%d", inner)),
						},
					}, nil)
					require.NoError(t, w.Log(exemplar))
				}
			}

			// Start read after checkpoint, no more data written.
			first, last, err := Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, true)
			watcher.SetStartTime(now)

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.setMetrics()
			for i := first; i <= last; i++ {
				segment, err := OpenReadSegment(SegmentName(watcher.walDir, i))
				require.NoError(t, err)
				defer segment.Close()

				reader := NewLiveReader(nil, NewLiveReaderMetrics(nil), segment)
				// Use tail true so we can ensure we got the right number of samples.
				watcher.readSegment(reader, i, true)
			}

			expectedSeries := seriesCount
			expectedSamples := seriesCount * samplesCount
			expectedExemplars := seriesCount * exemplarsCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expectedSeries
			})
			require.Equal(t, expectedSeries, wt.checkNumLabels(), "did not receive the expected number of series")
			require.Equal(t, expectedSamples, wt.samplesAppended, "did not receive the expected number of samples")
			require.Equal(t, expectedExemplars, wt.exemplarsAppended, "did not receive the expected number of exemplars")
		})
	}
}

func TestReadToEndNoCheckpoint(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			dir, err := ioutil.TempDir("", "readToEnd_noCheckpoint")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(dir))
			}()
			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			require.NoError(t, err)

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			var recs [][]byte

			enc := record.Encoder{}

			for i := 0; i < seriesCount; i++ {
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(i),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				recs = append(recs, series)
				for j := 0; j < samplesCount; j++ {
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(j),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)

					recs = append(recs, sample)

					// Randomly batch up records.
					if rand.Intn(4) < 3 {
						require.NoError(t, w.Log(recs...))
						recs = recs[:0]
					}
				}
			}
			require.NoError(t, w.Log(recs...))

			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false)
			go watcher.Start()

			expected := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expected
			})
			watcher.Stop()
			require.Equal(t, expected, wt.checkNumLabels())
		})
	}
}

func TestReadToEndWithCheckpoint(t *testing.T) {
	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			dir, err := ioutil.TempDir("", "readToEnd_withCheckpoint")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(dir))
			}()

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, segmentSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to the initial segment then checkpoint.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				require.NoError(t, w.Log(series))
				// Add in an unknown record type, which should be ignored.
				require.NoError(t, w.Log([]byte{255}))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}

			Checkpoint(log.NewNopLogger(), w, 0, 1, func(x uint64) bool { return true }, 0)
			w.Truncate(1)

			// Write more records after checkpointing.
			for i := 0; i < seriesCount; i++ {
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(i),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(j),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}

			_, _, err = Segments(w.Dir())
			require.NoError(t, err)
			wt := newWriteToMock()
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false)
			go watcher.Start()

			expected := seriesCount * 2
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expected
			})
			watcher.Stop()
			require.Equal(t, expected, wt.checkNumLabels())
		})
	}
}

func TestReadCheckpoint(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			dir, err := ioutil.TempDir("", "readCheckpoint")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(dir))
			}()

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			require.NoError(t, err)

			os.Create(SegmentName(wdir, 30))

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to the initial segment then checkpoint.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}
			Checkpoint(log.NewNopLogger(), w, 30, 31, func(x uint64) bool { return true }, 0)
			w.Truncate(32)

			// Start read after checkpoint, no more data written.
			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false)
			go watcher.Start()

			expectedSeries := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expectedSeries
			})
			watcher.Stop()
			require.Equal(t, expectedSeries, wt.checkNumLabels())
		})
	}
}

func TestReadCheckpointMultipleSegments(t *testing.T) {
	pageSize := 32 * 1024

	const segments = 1
	const seriesCount = 20
	const samplesCount = 300

	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			dir, err := ioutil.TempDir("", "readCheckpoint")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(dir))
			}()

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, pageSize, compress)
			require.NoError(t, err)

			// Write a bunch of data.
			for i := 0; i < segments; i++ {
				for j := 0; j < seriesCount; j++ {
					ref := j + (i * 100)
					series := enc.Series([]record.RefSeries{
						{
							Ref:    uint64(ref),
							Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", j)}},
						},
					}, nil)
					require.NoError(t, w.Log(series))

					for k := 0; k < samplesCount; k++ {
						inner := rand.Intn(ref + 1)
						sample := enc.Samples([]record.RefSample{
							{
								Ref: uint64(inner),
								T:   int64(i),
								V:   float64(i),
							},
						}, nil)
						require.NoError(t, w.Log(sample))
					}
				}
			}
			require.NoError(t, w.Close())

			// At this point we should have at least 6 segments, lets create a checkpoint dir of the first 5.
			checkpointDir := dir + "/wal/checkpoint.000004"
			err = os.Mkdir(checkpointDir, 0777)
			require.NoError(t, err)
			for i := 0; i <= 4; i++ {
				err := os.Rename(SegmentName(dir+"/wal", i), SegmentName(checkpointDir, i))
				require.NoError(t, err)
			}

			wt := newWriteToMock()
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false)
			watcher.MaxSegment = -1

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.setMetrics()

			lastCheckpoint, _, err := LastCheckpoint(watcher.walDir)
			require.NoError(t, err)

			err = watcher.readCheckpoint(lastCheckpoint)
			require.NoError(t, err)
		})
	}
}

func TestCheckpointSeriesReset(t *testing.T) {
	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 20
	const samplesCount = 350
	testCases := []struct {
		compress bool
		segments int
	}{
		{compress: false, segments: 14},
		{compress: true, segments: 13},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("compress=%t", tc.compress), func(t *testing.T) {
			dir, err := ioutil.TempDir("", "seriesReset")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(dir))
			}()

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, segmentSize, tc.compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to the initial segment, then checkpoint later.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}

			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false)
			watcher.MaxSegment = -1
			go watcher.Start()

			expected := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expected
			})
			require.Equal(t, seriesCount, wt.checkNumLabels())

			_, err = Checkpoint(log.NewNopLogger(), w, 2, 4, func(x uint64) bool { return true }, 0)
			require.NoError(t, err)

			err = w.Truncate(5)
			require.NoError(t, err)

			_, cpi, err := LastCheckpoint(path.Join(dir, "wal"))
			require.NoError(t, err)
			err = watcher.garbageCollectSeries(cpi + 1)
			require.NoError(t, err)

			watcher.Stop()
			// If you modify the checkpoint and truncate segment #'s run the test to see how
			// many series records you end up with and change the last Equals check accordingly
			// or modify the Equals to Assert(len(wt.seriesLabels) < seriesCount*10)
			require.Equal(t, tc.segments, wt.checkNumLabels())
		})
	}
}
