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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/tsdb/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/testutil"
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
	seriesLock           sync.Mutex
	seriesSegmentIndexes map[uint64]int
}

func (wtm *writeToMock) Append(s []record.RefSample) bool {
	wtm.samplesAppended += len(s)
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
	for _, compress := range []bool{false, true} {
		t.Run(fmt.Sprintf("compress=%t", compress), func(t *testing.T) {
			now := time.Now()

			dir, err := ioutil.TempDir("", "readCheckpoint")
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			testutil.Ok(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			testutil.Ok(t, err)

			// Write to the initial segment then checkpoint.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				testutil.Ok(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   int64(now.UnixNano()) + 1,
							V:   float64(i),
						},
					}, nil)
					testutil.Ok(t, w.Log(sample))
				}
			}

			// Start read after checkpoint, no more data written.
			first, last, err := w.Segments()
			testutil.Ok(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(nil, wMetrics, nil, "", wt, dir)
			watcher.StartTime = now.UnixNano()

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.setMetrics()
			for i := first; i <= last; i++ {
				segment, err := OpenReadSegment(SegmentName(watcher.walDir, i))
				testutil.Ok(t, err)
				defer segment.Close()

				reader := NewLiveReader(nil, NewLiveReaderMetrics(prometheus.DefaultRegisterer), segment)
				// Use tail true so we can ensure we got the right number of samples.
				watcher.readSegment(reader, i, true)
			}

			expectedSeries := seriesCount
			expectedSamples := seriesCount * samplesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expectedSeries
			})
			testutil.Equals(t, expectedSeries, wt.checkNumLabels())
			testutil.Equals(t, expectedSamples, wt.samplesAppended)
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
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)
			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			testutil.Ok(t, err)

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			testutil.Ok(t, err)

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
						testutil.Ok(t, w.Log(recs...))
						recs = recs[:0]
					}
				}
			}
			testutil.Ok(t, w.Log(recs...))

			_, _, err = w.Segments()
			testutil.Ok(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(nil, wMetrics, nil, "", wt, dir)
			go watcher.Start()

			expected := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expected
			})
			watcher.Stop()
			testutil.Equals(t, expected, wt.checkNumLabels())
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
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			testutil.Ok(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, segmentSize, compress)
			testutil.Ok(t, err)

			// Write to the initial segment then checkpoint.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				testutil.Ok(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					testutil.Ok(t, w.Log(sample))
				}
			}

			Checkpoint(w, 0, 1, func(x uint64) bool { return true }, 0)
			w.Truncate(1)

			// Write more records after checkpointing.
			for i := 0; i < seriesCount; i++ {
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(i),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				testutil.Ok(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(j),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					testutil.Ok(t, w.Log(sample))
				}
			}

			_, _, err = w.Segments()
			testutil.Ok(t, err)
			wt := newWriteToMock()
			watcher := NewWatcher(nil, wMetrics, nil, "", wt, dir)
			go watcher.Start()

			expected := seriesCount * 2
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expected
			})
			watcher.Stop()
			testutil.Equals(t, expected, wt.checkNumLabels())
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
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			testutil.Ok(t, err)

			os.Create(SegmentName(wdir, 30))

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			testutil.Ok(t, err)

			// Write to the initial segment then checkpoint.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				testutil.Ok(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					testutil.Ok(t, w.Log(sample))
				}
			}
			Checkpoint(w, 30, 31, func(x uint64) bool { return true }, 0)
			w.Truncate(32)

			// Start read after checkpoint, no more data written.
			_, _, err = w.Segments()
			testutil.Ok(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(nil, wMetrics, nil, "", wt, dir)
			go watcher.Start()

			expectedSeries := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expectedSeries
			})
			watcher.Stop()
			testutil.Equals(t, expectedSeries, wt.checkNumLabels())
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
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			testutil.Ok(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, pageSize, compress)
			testutil.Ok(t, err)

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
					testutil.Ok(t, w.Log(series))

					for k := 0; k < samplesCount; k++ {
						inner := rand.Intn(ref + 1)
						sample := enc.Samples([]record.RefSample{
							{
								Ref: uint64(inner),
								T:   int64(i),
								V:   float64(i),
							},
						}, nil)
						testutil.Ok(t, w.Log(sample))
					}
				}
			}

			// At this point we should have at least 6 segments, lets create a checkpoint dir of the first 5.
			checkpointDir := dir + "/wal/checkpoint.000004"
			err = os.Mkdir(checkpointDir, 0777)
			testutil.Ok(t, err)
			for i := 0; i <= 4; i++ {
				err := os.Rename(SegmentName(dir+"/wal", i), SegmentName(checkpointDir, i))
				testutil.Ok(t, err)
			}

			wt := newWriteToMock()
			watcher := NewWatcher(nil, wMetrics, nil, "", wt, dir)
			watcher.MaxSegment = -1

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.setMetrics()

			lastCheckpoint, _, err := LastCheckpoint(watcher.walDir)
			testutil.Ok(t, err)

			err = watcher.readCheckpoint(lastCheckpoint)
			testutil.Ok(t, err)
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
			testutil.Ok(t, err)
			defer os.RemoveAll(dir)

			wdir := path.Join(dir, "wal")
			err = os.Mkdir(wdir, 0777)
			testutil.Ok(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, segmentSize, tc.compress)
			testutil.Ok(t, err)

			// Write to the initial segment, then checkpoint later.
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    uint64(ref),
						Labels: labels.Labels{labels.Label{Name: "__name__", Value: fmt.Sprintf("metric_%d", i)}},
					},
				}, nil)
				testutil.Ok(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: uint64(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					testutil.Ok(t, w.Log(sample))
				}
			}

			_, _, err = w.Segments()
			testutil.Ok(t, err)

			wt := newWriteToMock()
			watcher := NewWatcher(nil, wMetrics, nil, "", wt, dir)
			watcher.MaxSegment = -1
			go watcher.Start()

			expected := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumLabels() >= expected
			})
			testutil.Equals(t, seriesCount, wt.checkNumLabels())

			_, err = Checkpoint(w, 2, 4, func(x uint64) bool { return true }, 0)
			testutil.Ok(t, err)

			err = w.Truncate(5)
			testutil.Ok(t, err)

			_, cpi, err := LastCheckpoint(path.Join(dir, "wal"))
			testutil.Ok(t, err)
			err = watcher.garbageCollectSeries(cpi + 1)
			testutil.Ok(t, err)

			watcher.Stop()
			// If you modify the checkpoint and truncate segment #'s run the test to see how
			// many series records you end up with and change the last Equals check accordingly
			// or modify the Equals to Assert(len(wt.seriesLabels) < seriesCount*10)
			testutil.Equals(t, tc.segments, wt.checkNumLabels())
		})
	}
}
