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
package remote

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/util/testutil"
	"github.com/prometheus/tsdb"
	"github.com/prometheus/tsdb/labels"
	"github.com/prometheus/tsdb/wal"
)

var defaultRetryInterval = 100 * time.Millisecond
var defaultRetries = 100

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

func (wtm *writeToMock) Append(s []tsdb.RefSample) bool {
	wtm.samplesAppended += len(s)
	return true
}

func (wtm *writeToMock) StoreSeries(series []tsdb.RefSeries, index int) {
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
	now := time.Now()

	dir, err := ioutil.TempDir("", "readCheckpoint")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	wdir := path.Join(dir, "wal")
	err = os.Mkdir(wdir, 0777)
	testutil.Ok(t, err)

	// os.Create(wal.SegmentName(wdir, 30))

	enc := tsdb.RecordEncoder{}
	w, err := wal.NewSize(nil, nil, wdir, 128*pageSize)
	testutil.Ok(t, err)

	// Write to the initial segment then checkpoint.
	for i := 0; i < seriesCount; i++ {
		ref := i + 100
		series := enc.Series([]tsdb.RefSeries{
			tsdb.RefSeries{
				Ref:    uint64(ref),
				Labels: labels.Labels{labels.Label{"__name__", fmt.Sprintf("metric_%d", i)}},
			},
		}, nil)
		testutil.Ok(t, w.Log(series))

		for j := 0; j < samplesCount; j++ {
			inner := rand.Intn(ref + 1)
			sample := enc.Samples([]tsdb.RefSample{
				tsdb.RefSample{
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
	watcher := NewWALWatcher(nil, "", wt, dir)
	watcher.startTime = now.UnixNano()
	for i := first; i <= last; i++ {
		segment, err := wal.OpenReadSegment(wal.SegmentName(watcher.walDir, i))
		testutil.Ok(t, err)
		defer segment.Close()

		reader := wal.NewLiveReader(nil, segment)
		// Use tail true so we can ensure we got the right number of samples.
		watcher.readSegment(reader, i, true)
	}
	go watcher.Start()

	expectedSeries := seriesCount
	expectedSamples := seriesCount * samplesCount
	retry(t, defaultRetryInterval, defaultRetries, func() bool {
		return wt.checkNumLabels() >= expectedSeries
	})
	watcher.Stop()
	testutil.Equals(t, expectedSeries, wt.checkNumLabels())
	testutil.Equals(t, expectedSamples, wt.samplesAppended)
}

func TestReadToEndNoCheckpoint(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	dir, err := ioutil.TempDir("", "readToEnd_noCheckpoint")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)
	wdir := path.Join(dir, "wal")
	err = os.Mkdir(wdir, 0777)
	testutil.Ok(t, err)

	w, err := wal.NewSize(nil, nil, wdir, 128*pageSize)
	testutil.Ok(t, err)

	var recs [][]byte

	enc := tsdb.RecordEncoder{}

	for i := 0; i < seriesCount; i++ {
		series := enc.Series([]tsdb.RefSeries{
			tsdb.RefSeries{
				Ref:    uint64(i),
				Labels: labels.Labels{labels.Label{"__name__", fmt.Sprintf("metric_%d", i)}},
			},
		}, nil)
		recs = append(recs, series)
		for j := 0; j < samplesCount; j++ {
			sample := enc.Samples([]tsdb.RefSample{
				tsdb.RefSample{
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
	watcher := NewWALWatcher(nil, "", wt, dir)
	go watcher.Start()

	expected := seriesCount
	retry(t, defaultRetryInterval, defaultRetries, func() bool {
		return wt.checkNumLabels() >= expected
	})
	watcher.Stop()
	testutil.Equals(t, expected, wt.checkNumLabels())
}

func TestReadToEndWithCheckpoint(t *testing.T) {
	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 10
	const samplesCount = 250

	dir, err := ioutil.TempDir("", "readToEnd_withCheckpoint")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	wdir := path.Join(dir, "wal")
	err = os.Mkdir(wdir, 0777)
	testutil.Ok(t, err)

	enc := tsdb.RecordEncoder{}
	w, err := wal.NewSize(nil, nil, wdir, segmentSize)
	testutil.Ok(t, err)

	// Write to the initial segment then checkpoint.
	for i := 0; i < seriesCount; i++ {
		ref := i + 100
		series := enc.Series([]tsdb.RefSeries{
			tsdb.RefSeries{
				Ref:    uint64(ref),
				Labels: labels.Labels{labels.Label{"__name__", fmt.Sprintf("metric_%d", i)}},
			},
		}, nil)
		testutil.Ok(t, w.Log(series))

		for j := 0; j < samplesCount; j++ {
			inner := rand.Intn(ref + 1)
			sample := enc.Samples([]tsdb.RefSample{
				tsdb.RefSample{
					Ref: uint64(inner),
					T:   int64(i),
					V:   float64(i),
				},
			}, nil)
			testutil.Ok(t, w.Log(sample))
		}
	}

	tsdb.Checkpoint(w, 0, 1, func(x uint64) bool { return true }, 0)
	w.Truncate(1)

	// Write more records after checkpointing.
	for i := 0; i < seriesCount; i++ {
		series := enc.Series([]tsdb.RefSeries{
			tsdb.RefSeries{
				Ref:    uint64(i),
				Labels: labels.Labels{labels.Label{"__name__", fmt.Sprintf("metric_%d", i)}},
			},
		}, nil)
		testutil.Ok(t, w.Log(series))

		for j := 0; j < samplesCount; j++ {
			sample := enc.Samples([]tsdb.RefSample{
				tsdb.RefSample{
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
	watcher := NewWALWatcher(nil, "", wt, dir)
	go watcher.Start()

	expected := seriesCount * 2
	retry(t, defaultRetryInterval, defaultRetries, func() bool {
		return wt.checkNumLabels() >= expected
	})
	watcher.Stop()
	testutil.Equals(t, expected, wt.checkNumLabels())
}

func TestReadCheckpoint(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	dir, err := ioutil.TempDir("", "readCheckpoint")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	wdir := path.Join(dir, "wal")
	err = os.Mkdir(wdir, 0777)
	testutil.Ok(t, err)

	os.Create(wal.SegmentName(wdir, 30))

	enc := tsdb.RecordEncoder{}
	w, err := wal.NewSize(nil, nil, wdir, 128*pageSize)
	testutil.Ok(t, err)

	// Write to the initial segment then checkpoint.
	for i := 0; i < seriesCount; i++ {
		ref := i + 100
		series := enc.Series([]tsdb.RefSeries{
			tsdb.RefSeries{
				Ref:    uint64(ref),
				Labels: labels.Labels{labels.Label{"__name__", fmt.Sprintf("metric_%d", i)}},
			},
		}, nil)
		testutil.Ok(t, w.Log(series))

		for j := 0; j < samplesCount; j++ {
			inner := rand.Intn(ref + 1)
			sample := enc.Samples([]tsdb.RefSample{
				tsdb.RefSample{
					Ref: uint64(inner),
					T:   int64(i),
					V:   float64(i),
				},
			}, nil)
			testutil.Ok(t, w.Log(sample))
		}
	}
	tsdb.Checkpoint(w, 30, 31, func(x uint64) bool { return true }, 0)
	w.Truncate(32)

	// Start read after checkpoint, no more data written.
	_, _, err = w.Segments()
	testutil.Ok(t, err)

	wt := newWriteToMock()
	watcher := NewWALWatcher(nil, "", wt, dir)
	// watcher.
	go watcher.Start()

	expectedSeries := seriesCount
	retry(t, defaultRetryInterval, defaultRetries, func() bool {
		return wt.checkNumLabels() >= expectedSeries
	})
	watcher.Stop()
	testutil.Equals(t, expectedSeries, wt.checkNumLabels())
}

func TestReadCheckpointMultipleSegments(t *testing.T) {
	pageSize := 32 * 1024

	const segments = 1
	const seriesCount = 20
	const samplesCount = 300

	dir, err := ioutil.TempDir("", "readCheckpoint")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	wdir := path.Join(dir, "wal")
	err = os.Mkdir(wdir, 0777)
	testutil.Ok(t, err)

	enc := tsdb.RecordEncoder{}
	w, err := wal.NewSize(nil, nil, wdir, pageSize)
	testutil.Ok(t, err)

	// Write a bunch of data.
	for i := 0; i < segments; i++ {
		for j := 0; j < seriesCount; j++ {
			ref := j + (i * 100)
			series := enc.Series([]tsdb.RefSeries{
				tsdb.RefSeries{
					Ref:    uint64(ref),
					Labels: labels.Labels{labels.Label{"__name__", fmt.Sprintf("metric_%d", j)}},
				},
			}, nil)
			testutil.Ok(t, w.Log(series))

			for k := 0; k < samplesCount; k++ {
				inner := rand.Intn(ref + 1)
				sample := enc.Samples([]tsdb.RefSample{
					tsdb.RefSample{
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
		err := os.Rename(wal.SegmentName(dir+"/wal", i), wal.SegmentName(checkpointDir, i))
		testutil.Ok(t, err)
	}

	wt := newWriteToMock()
	watcher := NewWALWatcher(nil, "", wt, dir)
	watcher.maxSegment = -1

	lastCheckpoint, _, err := tsdb.LastCheckpoint(watcher.walDir)
	testutil.Ok(t, err)

	err = watcher.readCheckpoint(lastCheckpoint)
	testutil.Ok(t, err)
}

func TestCheckpointSeriesReset(t *testing.T) {
	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 20
	const samplesCount = 350

	dir, err := ioutil.TempDir("", "seriesReset")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	wdir := path.Join(dir, "wal")
	err = os.Mkdir(wdir, 0777)
	testutil.Ok(t, err)

	enc := tsdb.RecordEncoder{}
	w, err := wal.NewSize(nil, nil, wdir, segmentSize)
	testutil.Ok(t, err)

	// Write to the initial segment, then checkpoint later.
	for i := 0; i < seriesCount; i++ {
		ref := i + 100
		series := enc.Series([]tsdb.RefSeries{
			tsdb.RefSeries{
				Ref:    uint64(ref),
				Labels: labels.Labels{labels.Label{"__name__", fmt.Sprintf("metric_%d", i)}},
			},
		}, nil)
		testutil.Ok(t, w.Log(series))

		for j := 0; j < samplesCount; j++ {
			inner := rand.Intn(ref + 1)
			sample := enc.Samples([]tsdb.RefSample{
				tsdb.RefSample{
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
	watcher := NewWALWatcher(nil, "", wt, dir)
	watcher.maxSegment = -1
	go watcher.Start()

	expected := seriesCount
	retry(t, defaultRetryInterval, defaultRetries, func() bool {
		return wt.checkNumLabels() >= expected
	})
	testutil.Equals(t, seriesCount, wt.checkNumLabels())

	_, err = tsdb.Checkpoint(w, 2, 4, func(x uint64) bool { return true }, 0)
	testutil.Ok(t, err)

	err = w.Truncate(5)
	testutil.Ok(t, err)

	_, cpi, err := tsdb.LastCheckpoint(path.Join(dir, "wal"))
	testutil.Ok(t, err)
	err = watcher.garbageCollectSeries(cpi + 1)
	testutil.Ok(t, err)

	watcher.Stop()
	// If you modify the checkpoint and truncate segment #'s run the test to see how
	// many series records you end up with and change the last Equals check accordingly
	// or modify the Equals to Assert(len(wt.seriesLabels) < seriesCount*10)
	testutil.Equals(t, 14, wt.checkNumLabels())
}
