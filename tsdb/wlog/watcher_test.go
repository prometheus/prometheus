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
package wlog

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/testrecord"
)

var (
	testTickerPeriod     = 500 * time.Millisecond
	defaultRetryInterval = 100 * time.Millisecond
	defaultRetries       = 100
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

func (wtm *writeToMock) StoreMetadata(_ []record.RefMetadata) { /* no-op */ }

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

func (wtm *writeToMock) seriesStored() int {
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

func newTestWatcher(dir string, to WriteTo) *Watcher {
	return NewWatcher(nil, nil, nil, "test", to, dir, true, true, true)
}

func newTestFastWatcher(dir string, to WriteTo) *Watcher {
	w := NewWatcher(nil, nil, nil, "test", to, dir, true, true, true)
	// Fasten our tests.
	w.checkpointPeriod = testTickerPeriod
	w.readPeriod = testTickerPeriod
	return w
}

func startWatching(t *testing.T, w *Watcher, startTimeFn func() time.Time) (isRunning func() bool, stop func()) {
	// It's like watcher.Start() but allows setting startTime and report errors to testutil.
	go func() {
		defer close(w.done)
		require.NoError(t, w.Watch(timestamp.FromTime(startTimeFn())))
	}()
	return func() bool {
		select {
		case <-w.done:
			return false
		default:
			return true
		}
	}, w.Stop
}

func logTestWALRecords(t *testing.T, ts time.Time, w *WL, seriesRefOffset, seriesCount, samplesCount, histogramsCount, exemplarsCount int) {
	t.Helper()

	enc := record.Encoder{}

	for i := 0; i < seriesCount; i++ {
		ref := (seriesRefOffset * seriesCount) + i + 100
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
					T:   ts.UnixNano() + 1,
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
					T:      ts.UnixNano() + 1,
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

			histograms, _ := enc.HistogramSamples([]record.RefHistogramSample{{
				Ref: chunks.HeadSeriesRef(inner),
				T:   ts.UnixNano() + 1,
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
				T:   ts.UnixNano() + 1,
				H:   customBucketHist,
			}}, nil)
			require.NoError(t, w.Log(customBucketHistograms))

			floatHistograms, _ := enc.FloatHistogramSamples([]record.RefFloatHistogramSample{{
				Ref: chunks.HeadSeriesRef(inner),
				T:   ts.UnixNano() + 1,
				FH:  hist.ToFloat(nil),
			}}, nil)
			require.NoError(t, w.Log(floatHistograms))

			customBucketFloatHistograms := enc.CustomBucketsFloatHistogramSamples([]record.RefFloatHistogramSample{{
				Ref: chunks.HeadSeriesRef(inner),
				T:   ts.UnixNano() + 1,
				FH:  customBucketHist.ToFloat(nil),
			}}, nil)
			require.NoError(t, w.Log(customBucketFloatHistograms))
		}
	}
	// Add in an unknown record type, which should be ignored.
	require.NoError(t, w.Log([]byte{255}))
}

func expectSegments(t *testing.T, wdir string, expected int) {
	t.Helper()

	first, last, err := Segments(wdir)
	require.NoError(t, err)
	if first == -1 && last == -1 {
		require.Equal(t, expected, 0, "expected different number of segments, got 0")
		return
	}
	require.Equal(t, expected, (last-first)+1, "expected different number of segments, got %v to %v", first, last)
}

func TestWatch_RetroAndFresh(t *testing.T) {
	const (
		seriesCount     = 10
		samplesCount    = 250
		exemplarsCount  = 25
		histogramsCount = 50
	)
	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		for _, segments := range []int{1, 4} {
			t.Run(fmt.Sprintf("compress=%s/segments=%v", compress, segments), func(t *testing.T) {
				now := time.Now()
				dir := t.TempDir()
				wdir := path.Join(dir, "wal")
				require.NoError(t, os.Mkdir(wdir, 0o777))

				// We will have 2 watchers. One starting now, one after data is logged to check
				// both retrospective watching and fresh data.
				wt1 := newWriteToMock(0)
				isRunning1, close1 := startWatching(t, newTestFastWatcher(dir, wt1), func() time.Time { return now })

				w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
				require.NoError(t, err)
				defer func() {
					require.NoError(t, w.Close())
				}()

				// Write to WAL.
				logTestWALRecords(t, now, w, 0, seriesCount, samplesCount, histogramsCount, exemplarsCount)

				// Create multiple segments if needed.
				for i := range segments - 1 {
					_, err := w.NextSegment()
					require.NoError(t, err)
					logTestWALRecords(t, now.Add(time.Duration(i+1)*time.Minute), w, i+1, seriesCount, samplesCount, histogramsCount, exemplarsCount)
				}
				expectSegments(t, wdir, segments)

				wt2 := newWriteToMock(0)
				isRunning2, close2 := startWatching(t, newTestFastWatcher(dir, wt2), func() time.Time { return now })

				expectedSeries := seriesCount * segments
				retry(t, defaultRetryInterval, defaultRetries, func() bool {
					if !isRunning1() {
						t.Fatal("watcher1 prematurely finished")
					}
					if !isRunning2() {
						t.Fatal("watcher2 prematurely finished")
					}
					if wt1.seriesStored() != expectedSeries || wt2.seriesStored() != expectedSeries {
						return false
					}
					return true
				})
				close1()
				require.Equal(t, expectedSeries, wt1.seriesStored(), "did not receive the expected number of series")
				close2()
				require.Equal(t, expectedSeries, wt2.seriesStored(), "did not receive the expected number of series")

				// For wt1 we watched from the beginning so expect all data.
				expectedSamples := seriesCount * samplesCount * segments
				expectedExemplars := seriesCount * exemplarsCount * segments
				expectedHistograms := seriesCount * histogramsCount * 2 * segments
				require.Equal(t, expectedSamples, wt1.samplesAppended, "did not receive the expected number of samples")
				require.Equal(t, expectedExemplars, wt1.exemplarsAppended, "did not receive the expected number of exemplars")
				require.Equal(t, expectedHistograms, wt1.histogramsAppended, "did not receive the expected number of histograms")
				require.Equal(t, expectedHistograms, wt1.floatHistogramsAppended, "did not receive the expected number of float histograms")

				// For wt2 we watched after all was written so expect last segment data.
				expectedSamples = seriesCount * samplesCount
				expectedExemplars = seriesCount * exemplarsCount
				expectedHistograms = seriesCount * histogramsCount * 2
				require.Equal(t, expectedSamples, wt2.samplesAppended, "did not receive the expected number of samples")
				require.Equal(t, expectedExemplars, wt2.exemplarsAppended, "did not receive the expected number of exemplars")
				require.Equal(t, expectedHistograms, wt2.histogramsAppended, "did not receive the expected number of histograms")
				require.Equal(t, expectedHistograms, wt2.floatHistogramsAppended, "did not receive the expected number of float histograms")
			})
		}
	}
}

func TestWatch_WithCheckpoint(t *testing.T) {
	const (
		seriesCount  = 10
		samplesCount = 250
	)

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write 4 full segments.
			for i := range 4 {
				logTestWALRecords(t, time.Now(), w, i, seriesCount, samplesCount, 0, 0)
				_, err = w.NextSegment()
				require.NoError(t, err)
			}
			// 5th open one.
			logTestWALRecords(t, time.Now(), w, 4, seriesCount, samplesCount, 0, 0)
			expectSegments(t, wdir, 5)

			_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1, func(_ chunks.HeadSeriesRef, _ int) bool { return true }, 0)
			require.NoError(t, err)
			require.NoError(t, w.Truncate(2))
			expectSegments(t, wdir, 3) // We should see 3 segment as the first 2 were truncated.

			// Start watching.
			wt := newWriteToMock(0)
			isRunningFn, closeFn := startWatching(t, newTestFastWatcher(dir, wt), func() time.Time { return time.Now() })

			expectedSeries := seriesCount * 5 // We should see series from all 5 segments.
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				if !isRunningFn() {
					t.Fatal("watcher1 prematurely finished")
				}
				return wt.seriesStored() == expectedSeries
			})
			require.Equal(t, expectedSeries, wt.seriesStored())

			// During watcher routine, do another checkpoint which deletes all previous series.
			// Then we truncate.
			// We expect watcher to reset the series eventually (GC routine).
			_, err = Checkpoint(promslog.NewNopLogger(), w, 2, 3, func(ref chunks.HeadSeriesRef, _ int) bool { return false }, 0)
			require.NoError(t, err)
			err = w.Truncate(4)
			require.NoError(t, err)
			expectSegments(t, wdir, 1)

			expectedSeries = seriesCount * 2 // Only series from the last 2 segments should be visible.
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				if !isRunningFn() {
					t.Fatal("watcher1 prematurely finished")
				}
				return wt.seriesStored() == expectedSeries
			})
			require.Equal(t, expectedSeries, wt.seriesStored())
			closeFn()
		})
	}
}

// TestWatch_StartupTime ensures we don't need to wait for read ticker (15s default)
// to read the whole WAL on startup.
func TestWatch_StartupTime(t *testing.T) {
	const (
		seriesCount  = 20
		samplesCount = 300
	)

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(string(compress), func(t *testing.T) {
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write 5 segments.
			logTestWALRecords(t, time.Now(), w, 0, seriesCount, samplesCount, 0, 0)
			_, err = w.NextSegment()
			require.NoError(t, err)
			logTestWALRecords(t, time.Now(), w, 1, seriesCount, samplesCount, 0, 0)
			_, err = w.NextSegment()
			require.NoError(t, err)
			logTestWALRecords(t, time.Now(), w, 2, seriesCount, samplesCount, 0, 0)
			_, err = w.NextSegment()
			require.NoError(t, err)
			logTestWALRecords(t, time.Now(), w, 3, seriesCount, samplesCount, 0, 0)
			_, err = w.NextSegment()
			require.NoError(t, err)
			logTestWALRecords(t, time.Now(), w, 4, seriesCount, samplesCount, 0, 0)

			expectSegments(t, wdir, 5)

			wt := newWriteToMock(0)
			startTime := time.Now()
			watcher := newTestWatcher(dir, wt)
			isRunningFn, closeFn := startWatching(t, watcher, func() time.Time { return time.Now() })

			// TODO(bwplotka): During refactor I noticed we never read the last segment before
			// readTimeout/readPeriod. Is this expected?
			expectedSeries := seriesCount * 4
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				if !isRunningFn() {
					t.Fatal("watcher prematurely finished")
				}
				return wt.seriesStored() == expectedSeries
			})
			closeFn()
			require.Equal(t, expectedSeries, wt.seriesStored(), "did not receive the expected number of series")

			require.Less(t, time.Since(startTime), watcher.readPeriod)
			require.NoError(t, err)
		})
	}
}

func TestWatch_AvoidNotifyWhenBehind(t *testing.T) {
	const (
		segmentsToWrite = 5
		segmentsToRead  = segmentsToWrite - 1
		seriesCount     = 10
		samplesCount    = 50
	)

	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		t.Run(string(compress), func(t *testing.T) {
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to 00000000, the watcher will read series from it.
			logTestWALRecords(t, time.Now(), w, 0, seriesCount, samplesCount, 0, 0)
			// Create 00000001, the watcher will tail it once started.
			_, err = w.NextSegment()
			require.NoError(t, err)

			// Set up the watcher and run it in the background.
			wt := newWriteToMock(time.Millisecond)
			startTime := time.Now()
			watcher := newTestWatcher(dir, wt)
			isRunningFn, closeFn := startWatching(t, watcher, func() time.Time { return time.Now() })

			// Wait until the watcher goes through 00000000 and is tailing the next one.
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				if !isRunningFn() {
					t.Fatal("watcher prematurely finished")
				}
				return wt.seriesStored() == seriesCount
			})
			require.Equal(t, seriesCount, wt.seriesStored(), "did not receive the expected number of series")

			// In the meantime, add some new segments in bulk.
			// We should end up with segmentsToWrite + 1 segments now.
			for i := 1; i < segmentsToWrite; i++ {
				logTestWALRecords(t, time.Now(), w, i, seriesCount, samplesCount, 0, 0)
				_, err = w.NextSegment()
				require.NoError(t, err)
			}

			expectedSeries := (segmentsToRead + 1) * seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				if !isRunningFn() {
					t.Fatal("watcher prematurely finished")
				}
				return wt.seriesStored() == seriesCount
			})

			require.Equal(t, expectedSeries, wt.seriesStored()) // Series from 00000000 are also read.
			require.Equal(t, segmentsToRead*seriesCount*samplesCount, wt.samplesAppended)
			closeFn()

			// TODO(bwplotka): During refactor I noticed it still takes 10s (15s is read period).
			// Is this expected?
			require.Less(t, time.Since(startTime), watcher.readPeriod)
			require.NoError(t, err)
		})
	}
}

func logBenchWALRealisticRecords(tb *testing.B, wdir string, seriesRecords, seriesPerRecord, sampleRecords int, compress CompressionType, samplesCase testrecord.RefSamplesCase) {
	tb.Helper()

	enc := record.Encoder{}
	w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
	require.NoError(tb, err)
	defer w.Close()

	for i := range seriesRecords {
		series := make([]record.RefSeries, seriesPerRecord)
		for j := range seriesPerRecord {
			series[j] = record.RefSeries{
				Ref:    chunks.HeadSeriesRef(i*seriesPerRecord + j),
				Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", 0), "foo", "bar", "foo1", "bar2", "sdfsasdgfadsfgaegga", "dgsfzdsf§sfawf2"),
			}
		}
		rec := enc.Series(series, nil)
		require.NoError(tb, w.Log(rec))
	}
	for i := 0; i < sampleRecords; i++ {
		rec := enc.Samples(testrecord.GenTestRefSamplesCase(tb, samplesCase), nil)
		require.NoError(tb, w.Log(rec))
	}
	// Build segment.
	require.NoError(tb, w.flushPage(true))
}

/*
	export bench=watcher-read-v1 && go test ./tsdb/wlog/... \
		-run '^$' -bench '^BenchmarkWatcherReadSegment' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkWatcherReadSegment(b *testing.B) {
	const (
		seriesRecords   = 100           //  Targets * Scrapes
		seriesPerRecord = 10            // New series per scrape.
		sampleRecords   = seriesRecords // Targets * Scrapes
	)
	for _, compress := range []CompressionType{CompressionNone, CompressionSnappy, CompressionZstd} {
		for _, data := range []testrecord.RefSamplesCase{
			testrecord.Realistic1000Samples,
			testrecord.WorstCase1000Samples,
		} {
			b.Run(fmt.Sprintf("compr=%v/data=%v", compress, data), func(b *testing.B) {
				dir := b.TempDir()
				wdir := path.Join(dir, "wal")
				require.NoError(b, os.Mkdir(wdir, 0o777))

				logBenchWALRealisticRecords(b, wdir, seriesRecords, seriesPerRecord, sampleRecords, compress, data)
				logger := promslog.NewNopLogger()

				b.Run("func=readSegmentSeries", func(b *testing.B) {
					benchmarkedReadFn := (*Watcher).readSegmentSeries

					wt := newWriteToMock(0)
					watcher := newTestWatcher(dir, wt)
					// Required as we don't use public method, but invoke readSegment* directly.
					watcher.initMetrics()

					// Validate our test data first.
					testReadFn(b, wdir, 0, logger, watcher, benchmarkedReadFn)
					require.Equal(b, seriesRecords*seriesPerRecord, wt.seriesStored())
					require.Equal(b, 0, wt.samplesAppended) // ReadSegmentSeries skips non-series.

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						testReadFn(b, wdir, 0, logger, watcher, benchmarkedReadFn)
					}
				})
				b.Run("func=readSegment", func(b *testing.B) {
					benchmarkedReadFn := func(w *Watcher, r *LiveReader, segmentNum int) error {
						// StartTime being ultra low is required as WorstCase1000Samples have
						// math.MinInt32 timestamps (for compression overhead).
						return w.readSegment(r, math.MinInt32-1, segmentNum)
					}

					wt := newWriteToMock(0)
					watcher := newTestWatcher(dir, wt)
					// Required as we don't use public method, but invoke readSegment* directly.
					watcher.initMetrics()

					// Validate our test data first.
					testReadFn(b, wdir, 0, logger, watcher, benchmarkedReadFn)
					require.Equal(b, seriesRecords*seriesPerRecord, wt.seriesStored())
					require.Equal(b, sampleRecords*1000, wt.samplesAppended)

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						testReadFn(b, wdir, 0, logger, watcher, benchmarkedReadFn)
					}
				})
			})
		}
	}
}

func testReadFn(tb testing.TB, wdir string, segNum int, logger *slog.Logger, watcher *Watcher, fn segmentReadFn) {
	tb.Helper()

	segment, err := OpenReadSegment(SegmentName(wdir, segNum))
	require.NoError(tb, err)

	r := NewLiveReader(logger, watcher.readerMetrics, segment)
	require.NoError(tb, fn(watcher, r, segNum))
}
