// Copyright The Prometheus Authors
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
	"fmt"
	"math/rand"
	"os"
	"path"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/compression"
)

var (
	defaultRetryInterval = 100 * time.Millisecond
	defaultRetries       = 100
	wMetrics             = NewWatcherMetrics(prometheus.DefaultRegisterer)
)

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

// Overwrite readTimeout defined in watcher.go.
func overwriteReadTimeout(t *testing.T, val time.Duration) {
	initialVal := readTimeout
	readTimeout = val
	t.Cleanup(func() { readTimeout = initialVal })
}

type writeToMock struct {
	samplesAppended         int
	exemplarsAppended       int
	histogramsAppended      int
	floatHistogramsAppended int
	seriesLock              sync.Mutex
	seriesSegmentIndexes    map[chunks.HeadSeriesRef]int

	// If nonzero, delay reads with a short sleep.
	delay time.Duration
}

func (wtm *writeToMock) Append(s []record.RefSample) bool {
	time.Sleep(wtm.delay)
	wtm.samplesAppended += len(s)
	return true
}

func (wtm *writeToMock) AppendExemplars(e []record.RefExemplar) bool {
	time.Sleep(wtm.delay)
	wtm.exemplarsAppended += len(e)
	return true
}

func (wtm *writeToMock) AppendHistograms(h []record.RefHistogramSample) bool {
	time.Sleep(wtm.delay)
	wtm.histogramsAppended += len(h)
	return true
}

func (wtm *writeToMock) AppendFloatHistograms(fh []record.RefFloatHistogramSample) bool {
	time.Sleep(wtm.delay)
	wtm.floatHistogramsAppended += len(fh)
	return true
}

func (wtm *writeToMock) StoreSeries(series []record.RefSeries, index int) {
	time.Sleep(wtm.delay)
	wtm.UpdateSeriesSegment(series, index)
}

func (*writeToMock) StoreMetadata([]record.RefMetadata) { /* no-op */ }

func (wtm *writeToMock) UpdateSeriesSegment(series []record.RefSeries, index int) {
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

func (wtm *writeToMock) checkNumSeries() int {
	wtm.seriesLock.Lock()
	defer wtm.seriesLock.Unlock()
	return len(wtm.seriesSegmentIndexes)
}

func newWriteToMock(delay time.Duration) *writeToMock {
	return &writeToMock{
		seriesSegmentIndexes: make(map[chunks.HeadSeriesRef]int),
		delay:                delay,
	}
}

func TestTailSamples(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250
	const exemplarsCount = 25
	const histogramsCount = 50
	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			now := time.Now()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to the initial segment then checkpoint.
			for i := range seriesCount {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for range samplesCount {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: chunks.HeadSeriesRef(inner),
							T:   now.UnixNano() + 1,
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}

				for range exemplarsCount {
					inner := rand.Intn(ref + 1)
					exemplar := enc.Exemplars([]record.RefExemplar{
						{
							Ref:    chunks.HeadSeriesRef(inner),
							T:      now.UnixNano() + 1,
							V:      float64(i),
							Labels: labels.FromStrings("trace_id", fmt.Sprintf("trace-%d", inner)),
						},
					}, nil)
					require.NoError(t, w.Log(exemplar))
				}

				for range histogramsCount {
					inner := rand.Intn(ref + 1)
					hist := &histogram.Histogram{
						Schema:          2,
						ZeroThreshold:   1e-128,
						ZeroCount:       0,
						Count:           2,
						Sum:             0,
						PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						PositiveBuckets: []int64{int64(i) + 1},
						NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						NegativeBuckets: []int64{int64(-i) - 1},
					}

					histograms, _ := enc.HistogramSamples([]record.RefHistogramSample{{
						Ref: chunks.HeadSeriesRef(inner),
						T:   now.UnixNano() + 1,
						H:   hist,
					}}, nil)
					require.NoError(t, w.Log(histograms))

					customBucketHist := &histogram.Histogram{
						Schema:        -53,
						ZeroThreshold: 1e-128,
						ZeroCount:     0,
						Count:         2,
						Sum:           0,
						PositiveSpans: []histogram.Span{{Offset: 0, Length: 1}},
						CustomValues:  []float64{float64(i) + 2},
					}

					customBucketHistograms := enc.CustomBucketsHistogramSamples([]record.RefHistogramSample{{
						Ref: chunks.HeadSeriesRef(inner),
						T:   now.UnixNano() + 1,
						H:   customBucketHist,
					}}, nil)
					require.NoError(t, w.Log(customBucketHistograms))

					floatHistograms, _ := enc.FloatHistogramSamples([]record.RefFloatHistogramSample{{
						Ref: chunks.HeadSeriesRef(inner),
						T:   now.UnixNano() + 1,
						FH:  hist.ToFloat(nil),
					}}, nil)
					require.NoError(t, w.Log(floatHistograms))

					customBucketFloatHistograms := enc.CustomBucketsFloatHistogramSamples([]record.RefFloatHistogramSample{{
						Ref: chunks.HeadSeriesRef(inner),
						T:   now.UnixNano() + 1,
						FH:  customBucketHist.ToFloat(nil),
					}}, nil)
					require.NoError(t, w.Log(customBucketFloatHistograms))
				}
			}

			// Start read after checkpoint, no more data written.
			first, last, err := Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock(0)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, true, true, true)
			watcher.SetStartTime(now)

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.SetMetrics()
			for i := first; i <= last; i++ {
				segment, err := OpenReadSegment(SegmentName(watcher.walDir, i))
				require.NoError(t, err)

				reader := NewLiveReader(nil, NewLiveReaderMetrics(nil), segment)
				// Use tail true so we can ensure we got the right number of samples.
				watcher.readSegment(reader, i, true)
				require.NoError(t, segment.Close())
			}

			expectedSeries := seriesCount
			expectedSamples := seriesCount * samplesCount
			expectedExemplars := seriesCount * exemplarsCount
			expectedHistograms := seriesCount * histogramsCount * 2
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumSeries() >= expectedSeries
			})
			require.Equal(t, expectedSeries, wt.checkNumSeries(), "did not receive the expected number of series")
			require.Equal(t, expectedSamples, wt.samplesAppended, "did not receive the expected number of samples")
			require.Equal(t, expectedExemplars, wt.exemplarsAppended, "did not receive the expected number of exemplars")
			require.Equal(t, expectedHistograms, wt.histogramsAppended, "did not receive the expected number of histograms")
			require.Equal(t, expectedHistograms, wt.floatHistogramsAppended, "did not receive the expected number of float histograms")
		})
	}
}

func TestReadToEndNoCheckpoint(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			var recs [][]byte

			enc := record.Encoder{}

			for i := range seriesCount {
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(i),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				recs = append(recs, series)
				for j := range samplesCount {
					sample := enc.Samples([]record.RefSample{
						{
							Ref: chunks.HeadSeriesRef(j),
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
			overwriteReadTimeout(t, time.Second)
			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock(0)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false)
			go watcher.Start()

			expected := seriesCount
			require.Eventually(t, func() bool {
				return wt.checkNumSeries() == expected
			}, 20*time.Second, 1*time.Second)
			watcher.Stop()
		})
	}
}

func TestReadToEndWithCheckpoint(t *testing.T) {
	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, segmentSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to the initial segment then checkpoint.
			for i := range seriesCount {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))
				// Add in an unknown record type, which should be ignored.
				require.NoError(t, w.Log([]byte{255}))

				for range samplesCount {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: chunks.HeadSeriesRef(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}

			Checkpoint(promslog.NewNopLogger(), w, 0, 1, func(chunks.HeadSeriesRef) bool { return true }, 0)
			w.Truncate(1)

			// Write more records after checkpointing.
			for i := range seriesCount {
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(i),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := range samplesCount {
					sample := enc.Samples([]record.RefSample{
						{
							Ref: chunks.HeadSeriesRef(j),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}

			_, _, err = Segments(w.Dir())
			require.NoError(t, err)
			overwriteReadTimeout(t, time.Second)
			wt := newWriteToMock(0)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false)
			go watcher.Start()

			expected := seriesCount * 2

			require.Eventually(t, func() bool {
				return wt.checkNumSeries() == expected
			}, 10*time.Second, 1*time.Second)
			watcher.Stop()
		})
	}
}

func TestReadCheckpoint(t *testing.T) {
	t.Parallel()
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			f, err := os.Create(SegmentName(wdir, 30))
			require.NoError(t, err)
			require.NoError(t, f.Close())

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, w.Close())
			})

			// Write to the initial segment then checkpoint.
			for i := range seriesCount {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for range samplesCount {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: chunks.HeadSeriesRef(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}
			_, err = w.NextSegmentSync()
			require.NoError(t, err)
			_, err = Checkpoint(promslog.NewNopLogger(), w, 30, 31, func(chunks.HeadSeriesRef) bool { return true }, 0)
			require.NoError(t, err)
			require.NoError(t, w.Truncate(32))

			// Start read after checkpoint, no more data written.
			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock(0)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false)
			go watcher.Start()

			expectedSeries := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumSeries() >= expectedSeries
			})
			watcher.Stop()
			require.Equal(t, expectedSeries, wt.checkNumSeries())
		})
	}
}

func TestReadCheckpointMultipleSegments(t *testing.T) {
	pageSize := 32 * 1024

	const segments = 1
	const seriesCount = 20
	const samplesCount = 300

	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, pageSize, compress)
			require.NoError(t, err)

			// Write a bunch of data.
			for i := range segments {
				for j := range seriesCount {
					ref := j + (i * 100)
					series := enc.Series([]record.RefSeries{
						{
							Ref:    chunks.HeadSeriesRef(ref),
							Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
						},
					}, nil)
					require.NoError(t, w.Log(series))

					for range samplesCount {
						inner := rand.Intn(ref + 1)
						sample := enc.Samples([]record.RefSample{
							{
								Ref: chunks.HeadSeriesRef(inner),
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
			err = os.Mkdir(checkpointDir, 0o777)
			require.NoError(t, err)
			for i := 0; i <= 4; i++ {
				err := os.Rename(SegmentName(dir+"/wal", i), SegmentName(checkpointDir, i))
				require.NoError(t, err)
			}

			wt := newWriteToMock(0)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false)
			watcher.MaxSegment = -1

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.SetMetrics()

			lastCheckpoint, _, err := LastCheckpoint(watcher.walDir)
			require.NoError(t, err)

			err = watcher.readCheckpoint(lastCheckpoint, (*Watcher).readSegment)
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
		compress compression.Type
		segments int
	}{
		{compress: compression.None, segments: 14},
		{compress: compression.Snappy, segments: 13},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("compress=%s", tc.compress), func(t *testing.T) {
			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, segmentSize, tc.compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to the initial segment, then checkpoint later.
			for i := range seriesCount {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for range samplesCount {
					inner := rand.Intn(ref + 1)
					sample := enc.Samples([]record.RefSample{
						{
							Ref: chunks.HeadSeriesRef(inner),
							T:   int64(i),
							V:   float64(i),
						},
					}, nil)
					require.NoError(t, w.Log(sample))
				}
			}

			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			overwriteReadTimeout(t, time.Second)
			wt := newWriteToMock(0)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false)
			watcher.MaxSegment = -1
			go watcher.Start()

			expected := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumSeries() >= expected
			})
			require.Eventually(t, func() bool {
				return wt.checkNumSeries() == seriesCount
			}, 10*time.Second, 1*time.Second)

			_, err = Checkpoint(promslog.NewNopLogger(), w, 2, 4, func(chunks.HeadSeriesRef) bool { return true }, 0)
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
			require.Eventually(t, func() bool {
				return wt.checkNumSeries() == tc.segments
			}, 20*time.Second, 1*time.Second)
		})
	}
}

func TestRun_StartupTime(t *testing.T) {
	t.Parallel()
	const pageSize = 32 * 1024
	const segments = 10
	const seriesCount = 20
	const samplesCount = 300

	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, pageSize, compress)
			require.NoError(t, err)

			for i := range segments {
				for j := range seriesCount {
					ref := j + (i * 100)
					series := enc.Series([]record.RefSeries{
						{
							Ref:    chunks.HeadSeriesRef(ref),
							Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
						},
					}, nil)
					require.NoError(t, w.Log(series))

					for range samplesCount {
						inner := rand.Intn(ref + 1)
						sample := enc.Samples([]record.RefSample{
							{
								Ref: chunks.HeadSeriesRef(inner),
								T:   int64(i),
								V:   float64(i),
							},
						}, nil)
						require.NoError(t, w.Log(sample))
					}
				}
			}
			require.NoError(t, w.Close())

			wt := newWriteToMock(0)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false)
			watcher.MaxSegment = segments

			watcher.SetMetrics()
			startTime := time.Now()

			err = watcher.Run()
			require.Less(t, time.Since(startTime), readTimeout)
			require.NoError(t, err)
		})
	}
}

func generateWALRecords(w *WL, segment, seriesCount, samplesCount int) error {
	enc := record.Encoder{}
	for j := range seriesCount {
		ref := j + (segment * 100)
		series := enc.Series([]record.RefSeries{
			{
				Ref:    chunks.HeadSeriesRef(ref),
				Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", segment)),
			},
		}, nil)
		if err := w.Log(series); err != nil {
			return err
		}

		for range samplesCount {
			inner := rand.Intn(ref + 1)
			sample := enc.Samples([]record.RefSample{
				{
					Ref: chunks.HeadSeriesRef(inner),
					T:   int64(segment),
					V:   float64(segment),
				},
			}, nil)
			if err := w.Log(sample); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestRun_AvoidNotifyWhenBehind(t *testing.T) {
	if runtime.GOOS == "windows" { // Takes a really long time, perhaps because min sleep time is 15ms.
		t.SkipNow()
	}
	const segmentSize = pageSize // Smallest allowed segment size.
	const segmentsToWrite = 5
	const segmentsToRead = segmentsToWrite - 1
	const seriesCount = 10
	const samplesCount = 50

	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			err := os.Mkdir(wdir, 0o777)
			require.NoError(t, err)

			w, err := NewSize(nil, nil, wdir, segmentSize, compress)
			require.NoError(t, err)
			// Write to 00000000, the watcher will read series from it.
			require.NoError(t, generateWALRecords(w, 0, seriesCount, samplesCount))
			// Create 00000001, the watcher will tail it once started.
			w.NextSegment()

			// Set up the watcher and run it in the background.
			wt := newWriteToMock(time.Millisecond)
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false)
			watcher.SetMetrics()
			watcher.MaxSegment = segmentsToRead

			var g errgroup.Group
			g.Go(func() error {
				startTime := time.Now()
				err = watcher.Run()
				if err != nil {
					return err
				}
				// If the watcher was to wait for readTicker to read every new segment, it would need readTimeout * segmentsToRead.
				d := time.Since(startTime)
				if d > readTimeout {
					return fmt.Errorf("watcher ran for %s, it shouldn't rely on readTicker=%s to read the new segments", d, readTimeout)
				}
				return nil
			})

			// The watcher went through 00000000 and is tailing the next one.
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumSeries() == seriesCount
			})

			// In the meantime, add some new segments in bulk.
			// We should end up with segmentsToWrite + 1 segments now.
			for i := 1; i < segmentsToWrite; i++ {
				require.NoError(t, generateWALRecords(w, i, seriesCount, samplesCount))
				w.NextSegment()
			}

			// Wait for the watcher.
			require.NoError(t, g.Wait())

			// All series and samples were read.
			require.Equal(t, (segmentsToRead+1)*seriesCount, wt.checkNumSeries()) // Series from 00000000 are also read.
			require.Equal(t, segmentsToRead*seriesCount*samplesCount, wt.samplesAppended)
			require.NoError(t, w.Close())
		})
	}
}
