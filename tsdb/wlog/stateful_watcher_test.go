// Copyright 2024 The Prometheus Authors
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
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

// The following tests are copied and adapted from watcher_test.go

func TestTailSamples_Stateful(t *testing.T) {
	t.Parallel()

	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250
	const exemplarsCount = 25
	const histogramsCount = 50
	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			t.Parallel()

			now := time.Now()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

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
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
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

				for j := 0; j < exemplarsCount; j++ {
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

				for j := 0; j < histogramsCount; j++ {
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

					histogram := enc.HistogramSamples([]record.RefHistogramSample{{
						Ref: chunks.HeadSeriesRef(inner),
						T:   now.UnixNano() + 1,
						H:   hist,
					}}, nil)
					require.NoError(t, w.Log(histogram))

					floatHistogram := enc.FloatHistogramSamples([]record.RefFloatHistogramSample{{
						Ref: chunks.HeadSeriesRef(inner),
						T:   now.UnixNano() + 1,
						FH:  hist.ToFloat(nil),
					}}, nil)
					require.NoError(t, w.Log(floatHistogram))
				}
			}

			// Start read after checkpoint, no more data written.
			first, last, err := Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock(0)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "test", wt, dir, true, true, true)
			watcher.SetStartTime(now)

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.setMetrics()
			for i := first; i <= last; i++ {
				watcher.readSegmentStrict(wdir, i, (*StatefulWatcher).readRecords)
			}

			expectedSeries := seriesCount
			expectedSamples := seriesCount * samplesCount
			expectedExemplars := seriesCount * exemplarsCount
			expectedHistograms := seriesCount * histogramsCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.seriesCount() >= expectedSeries
			})
			require.Equal(t, expectedSeries, wt.seriesCount(), "did not receive the expected number of series")
			require.Equal(t, expectedSamples, wt.samplesCount(), "did not receive the expected number of samples")
			require.Equal(t, expectedExemplars, wt.examplarsCount(), "did not receive the expected number of exemplars")
			require.Equal(t, expectedHistograms, wt.histogramsCount(), "did not receive the expected number of histograms")
			require.Equal(t, expectedHistograms, wt.floatHistogramsCount(), "did not receive the expected number of float histograms")
		})
	}
}

func TestReadToEndNoCheckpoint_Stateful(t *testing.T) {
	t.Parallel()

	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

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
						Ref:    chunks.HeadSeriesRef(i),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				recs = append(recs, series)
				for j := 0; j < samplesCount; j++ {
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
			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock(0)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "test", wt, dir, false, false, false)
			watcher.readTimeout = time.Second
			go watcher.Start()

			expected := seriesCount
			require.Eventually(t, func() bool {
				return wt.seriesCount() == expected
			}, 20*time.Second, 1*time.Second)
			watcher.Stop()
		})
	}
}

func TestReadToEndWithCheckpoint_Stateful(t *testing.T) {
	t.Parallel()

	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

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
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))
				// Add in an unknown record type, which should be ignored.
				require.NoError(t, w.Log([]byte{255}))

				for j := 0; j < samplesCount; j++ {
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

			Checkpoint(log.NewNopLogger(), w, 0, 1, func(x chunks.HeadSeriesRef) bool { return true }, 0)
			w.Truncate(1)

			// Write more records after checkpointing.
			for i := 0; i < seriesCount; i++ {
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(i),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
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

			wt := newWriteToMock(0)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "bar", wt, dir, false, false, false)
			watcher.readTimeout = time.Second
			go watcher.Start()

			expected := seriesCount * 2

			require.Eventually(t, func() bool {
				return wt.seriesCount() == expected
			}, 10*time.Second, 1*time.Second)
			watcher.Stop()
		})
	}
}

func TestReadCheckpoint_Stateful(t *testing.T) {
	t.Parallel()

	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

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
			for i := 0; i < seriesCount; i++ {
				ref := i + 100
				series := enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
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
			_, err = Checkpoint(log.NewNopLogger(), w, 30, 31, func(x chunks.HeadSeriesRef) bool { return true }, 0)
			require.NoError(t, err)
			require.NoError(t, w.Truncate(32))

			// Start read after checkpoint, no more data written.
			_, _, err = Segments(w.Dir())
			require.NoError(t, err)

			wt := newWriteToMock(0)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "test", wt, dir, false, false, false)
			go watcher.Start()

			expectedSeries := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.seriesCount() >= expectedSeries
			})
			watcher.Stop()
			require.Equal(t, expectedSeries, wt.seriesCount())
		})
	}
}

func TestReadCheckpointMultipleSegments_Stateful(t *testing.T) {
	t.Parallel()

	pageSize := 32 * 1024

	const segments = 1
	const seriesCount = 20
	const samplesCount = 300

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, pageSize, compress)
			require.NoError(t, err)

			// Write a bunch of data.
			for i := 0; i < segments; i++ {
				for j := 0; j < seriesCount; j++ {
					ref := j + (i * 100)
					series := enc.Series([]record.RefSeries{
						{
							Ref:    chunks.HeadSeriesRef(ref),
							Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
						},
					}, nil)
					require.NoError(t, w.Log(series))

					for k := 0; k < samplesCount; k++ {
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
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "test", wt, dir, false, false, false)
			watcher.MaxSegment = -1

			// Set the Watcher's metrics so they're not nil pointers.
			watcher.setMetrics()

			lastCheckpoint, checkpointIndex, err := LastCheckpoint(watcher.walDir)
			require.NoError(t, err)

			err = watcher.readCheckpoint(lastCheckpoint, checkpointIndex, (*StatefulWatcher).readRecords)
			require.NoError(t, err)
		})
	}
}

func TestCheckpointSeriesReset_Stateful(t *testing.T) {
	t.Parallel()

	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 20
	const samplesCount = 350
	testCases := []struct {
		compress CompressionType
		segments int
	}{
		{compress: CompressionNone, segments: 14},
		{compress: CompressionSnappy, segments: 13},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("compress=%s", tc.compress), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

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
						Ref:    chunks.HeadSeriesRef(ref),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
					},
				}, nil)
				require.NoError(t, w.Log(series))

				for j := 0; j < samplesCount; j++ {
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

			wt := newWriteToMock(0)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "foo", wt, dir, false, false, false)
			watcher.readTimeout = time.Second
			watcher.MaxSegment = -1
			go watcher.Start()

			expected := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.seriesCount() >= expected
			})
			require.Eventually(t, func() bool {
				return wt.seriesCount() == seriesCount
			}, 10*time.Second, 1*time.Second)

			_, err = Checkpoint(log.NewNopLogger(), w, 2, 4, func(x chunks.HeadSeriesRef) bool { return true }, 0)
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
				return wt.seriesCount() == tc.segments
			}, 20*time.Second, 1*time.Second)
		})
	}
}

func TestRun_StartupTime_Stateful(t *testing.T) {
	t.Parallel()

	const pageSize = 32 * 1024
	const segments = 10
	const seriesCount = 20
	const samplesCount = 300

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(string(compress), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			enc := record.Encoder{}
			w, err := NewSize(nil, nil, wdir, pageSize, compress)
			require.NoError(t, err)

			for i := 0; i < segments; i++ {
				for j := 0; j < seriesCount; j++ {
					ref := j + (i * 100)
					series := enc.Series([]record.RefSeries{
						{
							Ref:    chunks.HeadSeriesRef(ref),
							Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i)),
						},
					}, nil)
					require.NoError(t, w.Log(series))

					for k := 0; k < samplesCount; k++ {
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
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "bar", wt, dir, false, false, false)
			watcher.MaxSegment = segments
			watcher.setMetrics()

			startTime := time.Now()
			err = watcher.Run()
			require.Less(t, time.Since(startTime), watcher.readTimeout)
			require.NoError(t, err)
		})
	}
}

func TestRun_AvoidNotifyWhenBehind_Stateful(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" { // Takes a really long time, perhaps because min sleep time is 15ms.
		t.SkipNow()
	}
	const segmentsCount = 100
	const seriesCount = 1
	const samplesCount = 1

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(string(compress), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			wt := newWriteToMock(time.Millisecond)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "test", wt, dir, false, false, false)
			watcher.readTimeout = 15 * time.Second
			// The period is chosen in such a way as to not slow down the tests, but also to ensure
			// the duration comparison make sense.
			watcher.segmentCheckPeriod = 100 * time.Millisecond
			watcher.setMetrics()

			w, err := NewSize(nil, nil, wdir, pageSize, CompressionNone)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()
			// Write to the first segment.
			require.NoError(t, generateWALRecords(w, 0, seriesCount, samplesCount))

			// Start the watcher and ensure the first segment is read and tailed.
			go watcher.Start()
			require.Eventually(t, func() bool {
				return wt.seriesCount() == seriesCount && wt.samplesCount() == seriesCount*samplesCount
			}, 3*time.Second, 100*time.Millisecond)

			// Add the rest of the segments.
			// Start measuring the watcher reaction duration, we suppose segments generation should have little influence on that.
			startTime := time.Now()
			for i := 1; i < segmentsCount; i++ {
				w.NextSegment()
				require.NoError(t, generateWALRecords(w, i, seriesCount, samplesCount))
			}

			// If the watcher was to wait for readTimeout or segmentCheckPeriod to read every new segment, it would need min * segmentsCount.
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.seriesCount() == segmentsCount*seriesCount && wt.samplesCount() == seriesCount*samplesCount*segmentsCount
			})
			watcher.Stop()

			require.Less(t, time.Since(startTime), min(watcher.readTimeout, watcher.segmentCheckPeriod)*(segmentsCount/2))
		})
	}
}

// The following tests are specific to the stateful watcher.
// TODO: should they run on all WAL compress methods?

// This should only be called once as it may mess with existing WAL segments.
func generateSegments(t *testing.T, wdir string, wantedSegments []int, seriesCount, samplesCount int) *WL {
	t.Helper()

	if len(wantedSegments) == 0 {
		return nil
	}

	w, err := NewSize(nil, nil, wdir, pageSize, CompressionNone)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, w.Close())
	})
	// Generate the segments.
	lastSegment := slices.Max(wantedSegments)
	require.NoError(t, generateWALRecords(w, 0, seriesCount, samplesCount))
	for j := 1; j <= lastSegment; j++ {
		w.NextSegment()
		require.NoError(t, generateWALRecords(w, j, seriesCount, samplesCount))
	}
	// Delete the unwanted ones.
	segs, err := listSegments(wdir)
	require.NoError(t, err)
	for _, seg := range segs {
		if !slices.Contains(wantedSegments, seg.index) {
			require.NoError(t, os.Remove(SegmentName(wdir, seg.index)))
		}
	}
	// Sanity check
	var existing []int
	segs, err = listSegments(wdir)
	require.NoError(t, err)
	for _, seg := range segs {
		existing = append(existing, seg.index)
	}
	require.ElementsMatch(t, wantedSegments, existing)
	return w
}

// TestStateful_ProgressMarkerIsPersisted ensures that the right progress marker is persisted
// when the watcher is stopped.
func TestStateful_ProgressMarkerIsPersisted(t *testing.T) {
	t.Parallel()

	const seriesCount = 10
	const samplesCount = 50

	dir := t.TempDir()

	wdir := path.Join(dir, "wal")
	require.NoError(t, os.Mkdir(wdir, 0o777))

	watcherName := "bar"
	progressFilePath := path.Join(wdir, "remote_write", fmt.Sprintf("%s.json", watcherName))
	require.NoFileExists(t, progressFilePath)

	w, err := NewSize(nil, nil, wdir, pageSize, CompressionNone)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, w.Close())
	}()
	require.NoError(t, generateWALRecords(w, 0, seriesCount, samplesCount))

	wt := newWriteToMock(0)
	watcher := NewStatefulWatcher(wMetrics, nil, nil, watcherName, wt, dir, false, false, false)
	watcher.setMetrics()
	go watcher.Start()
	require.Eventually(t, func() bool {
		return wt.samplesCount() == seriesCount*samplesCount
	}, 5*time.Second, 100*time.Millisecond)
	watcher.Stop()

	progressMarker, err := os.ReadFile(progressFilePath)
	require.NoError(t, err)
	// Adjust offset if generateWALRecords changes.
	require.JSONEq(t, `{"segmentIndex": 0,"offset": 17350}`, string(progressMarker))
}

func createAndWriteTo(t *testing.T, path, content string) {
	os.MkdirAll(filepath.Dir(path), 0o700)
	progressMarkerFile, err := os.Create(path)
	require.NoError(t, err)
	_, err = progressMarkerFile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, progressMarkerFile.Close())
}

func TestStateful_SamplesToReadOnStartup(t *testing.T) {
	t.Parallel()

	const seriesCount = 10
	const samplesCount = 50

	testCases := []struct {
		name             string
		segments         []int
		checkpointFromTo []int
		progressMarker   string

		expectedSamples        int
		expectedSeries         int
		expectedProgressMarker string
	}{
		{
			name:     "no progress marker",
			segments: []int{0, 1, 2},

			expectedSamples:        seriesCount * samplesCount,
			expectedSeries:         3 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 2,"offset": 17350}`,
		},
		// TODO: adjust, how to know samples to be readd?????
		/* 		{
			name:           "with progress marker",
			segments:       []int{10, 11, 12},
			progressMarker: `{"segmentIndex": 10,"offset": 320}`,

			expectedSamples:        seriesCount * samplesCount,
			expectedSeries:         3 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 2,"offset": 17350}`,
		}, */
		{
			name:           "progress marker segment > last segment",
			segments:       []int{0, 1, 2},
			progressMarker: `{"segmentIndex": 5,"offset": 320}`,

			expectedSamples:        seriesCount * samplesCount,
			expectedSeries:         3 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 2,"offset": 17350}`,
		},
		{
			name:             "progress marker segment > last segment with checkpoint",
			segments:         []int{3, 4, 5, 6},
			checkpointFromTo: []int{3, 5},
			progressMarker:   `{"segmentIndex": 7,"offset": 436}`,

			expectedSamples:        seriesCount * samplesCount,
			expectedSeries:         4 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 6,"offset": 17350}`,
		},
		{
			name:           "progress marker segment < first segment",
			segments:       []int{2, 3, 4},
			progressMarker: `{"segmentIndex": 0,"offset": 777}`,

			expectedSamples:        3 * seriesCount * samplesCount,
			expectedSeries:         3 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 4,"offset": 17350}`,
		},
		{
			name:             "progress marker segment < first segment with checkpoint",
			segments:         []int{6, 7, 8, 9},
			checkpointFromTo: []int{6, 8},
			progressMarker:   `{"segmentIndex": 2,"offset": 4356}`,

			expectedSamples:        seriesCount * samplesCount,
			expectedSeries:         4 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 9,"offset": 17350}`,
		},
		{
			// This simulates a scenario where the last segment was replaced by another one containing less data.
			// In such a case, the watcher will still use its previous marker.
			name:           "last segment replaced",
			segments:       []int{0, 1, 2},
			progressMarker: `{"segmentIndex": 2,"offset": 9999999999}`,

			expectedSeries:         3 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 2,"offset": 9999999999}`,
		},
		{
			name:           "corrupted progress marker",
			segments:       []int{0, 1, 2},
			progressMarker: `{"segmentInde`,

			expectedSamples:        seriesCount * samplesCount,
			expectedSeries:         3 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 2,"offset": 17350}`,
		},
		{
			name:             "checkpoint prior to first segment",
			segments:         []int{0, 1, 2},
			checkpointFromTo: []int{0, 1},

			expectedSamples:        seriesCount * samplesCount,
			expectedSeries:         3 * seriesCount,
			expectedProgressMarker: `{"segmentIndex": 2,"offset": 17350}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w := generateSegments(t, wdir, tc.segments, seriesCount, samplesCount)
			if tc.checkpointFromTo != nil {
				Checkpoint(log.NewNopLogger(), w, tc.checkpointFromTo[0], tc.checkpointFromTo[1], func(x chunks.HeadSeriesRef) bool { return true }, 0)
				w.Truncate(tc.checkpointFromTo[1] + 1)
			}

			watcherName := "bar"
			progressMarkerPath := path.Join(wdir, "remote_write", fmt.Sprintf("%s.json", watcherName))
			if tc.progressMarker != "" {
				createAndWriteTo(t, progressMarkerPath, tc.progressMarker)
			}

			// Start the watcher and make sure the appropriate data was forwarded.
			wt := newWriteToMock(0)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, watcherName, wt, dir, false, false, false)
			watcher.setMetrics()
			go watcher.Start()

			require.Eventually(t, func() bool {
				return wt.seriesCount() == tc.expectedSeries && wt.samplesCount() == tc.expectedSamples
			}, 5*time.Second, 100*time.Millisecond)
			watcher.Stop()

			// Check the final progress.
			b, err := os.ReadFile(progressMarkerPath)
			require.NoError(t, err)
			require.JSONEq(t, tc.expectedProgressMarker, string(b))
		})
	}
}

func TestStateful_syncOnIteration(t *testing.T) {
	t.Parallel()

	const seriesCount = 10
	const samplesCount = 50

	testCases := []struct {
		name           string
		progressMarker *ProgressMarker
		segmentsState  *segmentsRange
		segments       []int

		expectedToFail         bool
		expectedProgressMarker *ProgressMarker
		expectedSegmentsState  *segmentsRange
	}{
		// First iteration
		{
			name:           "first iteration without progress marker without segments",
			expectedToFail: true,
		},
		{
			name:                   "first iteration without progress marker with one segment=00000000",
			segments:               []int{0},
			expectedProgressMarker: &ProgressMarker{},
			expectedSegmentsState:  &segmentsRange{},
		},
		{
			name:                   "first iteration without progress marker with one segment>00000000",
			segments:               []int{3},
			expectedProgressMarker: &ProgressMarker{SegmentIndex: 3},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   3,
				currentSegment: 3,
				lastSegment:    3,
			},
		},
		{
			name:                   "first iteration without progress marker with many segments>=00000000",
			segments:               []int{0, 1, 2},
			expectedProgressMarker: &ProgressMarker{SegmentIndex: 2},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   0,
				currentSegment: 0,
				lastSegment:    2,
			},
		},
		{
			name:                   "first iteration without progress marker with many segments>00000000",
			segments:               []int{7, 8, 9},
			expectedProgressMarker: &ProgressMarker{SegmentIndex: 9},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   7,
				currentSegment: 7,
				lastSegment:    9,
			},
		},
		{
			name: "first iteration with progress marker without segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 3,
				Offset:       123,
			},
			expectedToFail: true,
		},
		{
			name: "first iteration with progress marker at segment=00000000",
			progressMarker: &ProgressMarker{
				Offset: 123,
			},
			segments:               []int{0},
			expectedProgressMarker: &ProgressMarker{Offset: 123},
			expectedSegmentsState:  &segmentsRange{},
		},
		{
			name: "first iteration with progress marker > segment=00000000",
			progressMarker: &ProgressMarker{
				SegmentIndex: 10,
				Offset:       987,
			},
			segments:               []int{0},
			expectedProgressMarker: &ProgressMarker{},
			expectedSegmentsState:  &segmentsRange{},
		},
		{
			name: "first iteration with progress market at segment>00000000",
			progressMarker: &ProgressMarker{
				SegmentIndex: 4,
				Offset:       123,
			},
			segments: []int{4},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 4,
				Offset:       123,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   4,
				currentSegment: 4,
				lastSegment:    4,
			},
		},
		{
			name: "first iteration with progress marker at segment-1",
			progressMarker: &ProgressMarker{
				SegmentIndex: 3,
				Offset:       123,
			},
			segments: []int{4},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 4,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   4,
				currentSegment: 4,
				lastSegment:    4,
			},
		},
		{
			name: "first iteration with progress marker < segment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 2,
				Offset:       333,
			},
			segments: []int{4},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 4,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   4,
				currentSegment: 4,
				lastSegment:    4,
			},
		},
		{
			name: "first iteration with progress marker at segment+1",
			progressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       123,
			},
			segments: []int{7},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 7,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   7,
				currentSegment: 7,
				lastSegment:    7,
			},
		},
		{
			name: "first iteration with progress marker > segment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       123,
			},
			segments: []int{4},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 4,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   4,
				currentSegment: 4,
				lastSegment:    4,
			},
		},
		{
			name: "first iteration with progress marker < many segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 2,
				Offset:       123,
			},
			segments: []int{5, 6},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 5,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 5,
				lastSegment:    6,
			},
		},
		{
			name: "first iteration with progress marker <= many segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 5,
				Offset:       123,
			},
			segments: []int{5, 6},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 5,
				Offset:       123,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 5,
				lastSegment:    6,
			},
		},
		{
			name: "first iteration with progress marker between segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 5,
				Offset:       123,
			},
			segments: []int{4, 5, 6},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 5,
				Offset:       123,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   4,
				currentSegment: 4,
				lastSegment:    6,
			},
		},
		{
			name: "first iteration with progress marker > many segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       123,
			},
			segments: []int{1, 2, 3},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 3,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   1,
				currentSegment: 1,
				lastSegment:    3,
			},
		},
		{
			name: "first iteration with progress marker >= many segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       123,
			},
			segments: []int{6, 7, 8},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       123,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   6,
				currentSegment: 6,
				lastSegment:    8,
			},
		},
		// Intermediate iteration
		{
			name: "intermediate iteration without segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 2,
				Offset:       123,
			},
			segmentsState: &segmentsRange{
				firstSegment:   1,
				currentSegment: 3,
				lastSegment:    3,
			},
			expectedToFail: true,
		},
		{
			name: "intermediate iteration same segments",
			progressMarker: &ProgressMarker{
				SegmentIndex: 6,
				Offset:       951,
			},
			segmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 7,
				lastSegment:    7,
			},
			segments: []int{5, 6, 7},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 6,
				Offset:       951,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 7,
				lastSegment:    7,
			},
		},
		{
			name: "intermediate iteration firstSegment=prevFirstSegment and lastSegment>=prevLastSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 7,
				Offset:       123,
			},
			segmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 6,
				lastSegment:    7,
			},
			segments: []int{5, 6, 7, 8},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 7,
				Offset:       123,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 6,
				lastSegment:    8,
			},
		},
		{
			name: "intermediate iteration with firstSegment<prevFirstSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 7,
				Offset:       123,
			},
			segmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 7,
				lastSegment:    7,
			},
			segments:               []int{3, 4, 5, 6, 7},
			expectedProgressMarker: &ProgressMarker{SegmentIndex: 3},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   3,
				currentSegment: 3,
				lastSegment:    7,
			},
		},
		{
			name: "intermediate iteration with firstSegment and lastSegment < prevFirstSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 7,
				Offset:       123,
			},
			segmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 7,
				lastSegment:    10,
			},
			segments:               []int{1, 2, 3},
			expectedProgressMarker: &ProgressMarker{SegmentIndex: 1},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   1,
				currentSegment: 1,
				lastSegment:    3,
			},
		},
		{
			name: "intermediate iteration with progress marker segment >= firstSegment > prevFirstSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       963,
			},
			segmentsState: &segmentsRange{
				firstSegment:   4,
				currentSegment: 5,
				lastSegment:    9,
			},
			segments: []int{7, 8, 9},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       963,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   7,
				currentSegment: 7,
				lastSegment:    9,
			},
		},
		{
			name: "intermediate iteration with progress marker segment = firstSegment > prevFirstSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       558,
			},
			segmentsState: &segmentsRange{
				firstSegment:   6,
				currentSegment: 6,
				lastSegment:    9,
			},
			segments: []int{8, 9},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 8,
				Offset:       558,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   8,
				currentSegment: 8,
				lastSegment:    9,
			},
		},
		{
			name: "intermediate iteration with firstSegment > progress marker segment >= prevFirstSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 3,
				Offset:       558,
			},
			segmentsState: &segmentsRange{
				firstSegment:   0,
				currentSegment: 9,
				lastSegment:    10,
			},
			segments: []int{5, 6, 7},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 5,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 7,
				lastSegment:    7,
			},
		},
		{
			name: "intermediate iteration with lastSegment < progress marker segment <= prevLastSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 7,
				Offset:       558,
			},
			segmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 7,
				lastSegment:    8,
			},
			segments: []int{5, 6},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 6,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 6,
				lastSegment:    6,
			},
		},
		{
			name: "intermediate iteration with progress marker segment <= lastSegment < prevLastSegment",
			progressMarker: &ProgressMarker{
				SegmentIndex: 5,
				Offset:       558,
			},
			segmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 6,
				lastSegment:    8,
			},
			segments: []int{5, 6},
			expectedProgressMarker: &ProgressMarker{
				SegmentIndex: 5,
				Offset:       558,
			},
			expectedSegmentsState: &segmentsRange{
				firstSegment:   5,
				currentSegment: 6,
				lastSegment:    6,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			generateSegments(t, wdir, tc.segments, seriesCount, samplesCount)

			// Set up and run the watcher.
			wt := newWriteToMock(0)
			watcher := NewStatefulWatcher(wMetrics, nil, nil, "test", wt, dir, false, false, false)
			watcher.setMetrics()
			// not nil means we want to overwrite it.
			if tc.progressMarker != nil {
				watcher.progressMarker = tc.progressMarker
			}
			if tc.segmentsState != nil {
				watcher.segmentsState = tc.segmentsState
			}

			err := watcher.syncOnIteration()

			if tc.expectedToFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Equal(t, tc.expectedProgressMarker, watcher.progressMarker)
			require.Equal(t, tc.expectedSegmentsState, watcher.segmentsState)
			require.Equal(t, 0, wt.seriesCount())
			require.Equal(t, 0, wt.samplesAppended)
		})
	}
}

func TestStateful_CatchAfterStop(t *testing.T) {
	t.Parallel()

	const seriesCount = 10
	const samplesCount = 50

	dir := t.TempDir()
	wdir := path.Join(dir, "wal")
	require.NoError(t, os.Mkdir(wdir, 0o777))

	w, err := NewSize(nil, nil, wdir, pageSize, CompressionNone)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, w.Close())
	}()
	require.NoError(t, generateWALRecords(w, 0, seriesCount, samplesCount))

	// Start the watcher and make sure the appropriate data was forwarded.
	watcherName := "foo"
	wt := newWriteToMock(0)
	watcher := NewStatefulWatcher(wMetrics, nil, nil, watcherName, wt, dir, false, false, false)
	watcher.setMetrics()
	go watcher.Start()

	require.Eventually(t, func() bool {
		return wt.seriesCount() == seriesCount && wt.samplesCount() == seriesCount*samplesCount
	}, 3*time.Second, 100*time.Millisecond)
	// Stop the watcher.
	watcher.Stop()

	// Add more records to the current segment and add a new one.
	require.NoError(t, generateWALRecords(w, 0, seriesCount, samplesCount))
	w.NextSegment()
	require.NoError(t, generateWALRecords(w, 1, seriesCount, samplesCount))

	// Re-start the watcher and ensures it catches up.
	wt = newWriteToMock(0)
	watcher = NewStatefulWatcher(wMetrics, nil, nil, watcherName, wt, dir, false, false, false)
	watcher.setMetrics()
	go watcher.Start()

	require.Eventually(t, func() bool {
		return wt.seriesCount() == 2*seriesCount && wt.samplesCount() == 2*seriesCount*samplesCount
	}, 3*time.Second, 100*time.Millisecond)
	watcher.Stop()
}
