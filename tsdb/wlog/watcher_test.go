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
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/testrecord"
)

var (
	defaultRetryInterval = 100 * time.Millisecond
	defaultRetries       = 100
)

// retry executes f() n times at each interval until it returns true.
func retry(t testing.TB, interval time.Duration, n int, f func() bool) {
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
	mu sync.Mutex

	samplesAppended         int
	exemplarsAppended       int
	histogramsAppended      int
	floatHistogramsAppended int
	seriesSegmentIndexes    map[chunks.HeadSeriesRef]int

	// If nonzero, delay reads with a short sleep.
	delay time.Duration
}

func (wtm *writeToMock) Append(s []record.RefSample) bool {
	time.Sleep(wtm.delay)

	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	wtm.samplesAppended += len(s)
	return true
}

func (wtm *writeToMock) AppendExemplars(e []record.RefExemplar) bool {
	time.Sleep(wtm.delay)

	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	wtm.exemplarsAppended += len(e)
	return true
}

func (wtm *writeToMock) AppendHistograms(h []record.RefHistogramSample) bool {
	time.Sleep(wtm.delay)

	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	wtm.histogramsAppended += len(h)
	return true
}

func (wtm *writeToMock) AppendFloatHistograms(fh []record.RefFloatHistogramSample) bool {
	time.Sleep(wtm.delay)

	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	wtm.floatHistogramsAppended += len(fh)
	return true
}

func (wtm *writeToMock) StoreSeries(series []record.RefSeries, index int) {
	time.Sleep(wtm.delay)
	wtm.UpdateSeriesSegment(series, index)
}

func (wtm *writeToMock) StoreMetadata(_ []record.RefMetadata) { /* no-op */ }

func (wtm *writeToMock) UpdateSeriesSegment(series []record.RefSeries, index int) {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	for _, s := range series {
		wtm.seriesSegmentIndexes[s.Ref] = index
	}
}

func (wtm *writeToMock) SeriesReset(index int) {
	// Check for series that are in segments older than the checkpoint
	// that were not also present in the checkpoint.
	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	for k, v := range wtm.seriesSegmentIndexes {
		if v < index {
			delete(wtm.seriesSegmentIndexes, k)
		}
	}
}

func (wtm *writeToMock) seriesStored() int {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	return len(wtm.seriesSegmentIndexes)
}

func (wtm *writeToMock) expectEventually(
	t testing.TB, isRunningFn func() bool,
	series, samples, exemplars, histograms int,
) {
	t.Helper()

	retry(t, defaultRetryInterval, defaultRetries, func() bool {
		if !isRunningFn() {
			t.Fatal("watcher prematurely finished")
		}

		wtm.mu.Lock()
		defer wtm.mu.Unlock()
		if len(wtm.seriesSegmentIndexes) != series {
			return false
		}
		if wtm.samplesAppended != samples {
			return false
		}
		if wtm.exemplarsAppended != exemplars {
			return false
		}
		if wtm.histogramsAppended+wtm.floatHistogramsAppended != 2*histograms {
			return false
		}
		return true
	})

	wtm.mu.Lock()
	defer wtm.mu.Unlock()
	require.Len(t, wtm.seriesSegmentIndexes, series, "did not receive the expected number of series")
	require.Equal(t, samples, wtm.samplesAppended, "did not receive the expected number of samples")
	require.Equal(t, exemplars, wtm.exemplarsAppended, "did not receive the expected number of exemplars")
	require.Equal(t, histograms, wtm.histogramsAppended, "did not receive the expected number of histograms")
	require.Equal(t, histograms, wtm.floatHistogramsAppended, "did not receive the expected number of float histograms")
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

func startWatching(t *testing.T, w *Watcher, startTimeFn func() time.Time) (isRunning func() bool) {
	t.Helper()

	// It's like watcher.Start() but allows setting startTime and report errors to testutil.
	go func() {
		defer close(w.done)
		require.NoError(t, w.Watch(timestamp.FromTime(startTimeFn())))
	}()
	t.Cleanup(w.Stop)

	return func() bool {
		select {
		case <-w.done:
			return false
		default:
			return true
		}
	}
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
					T:   timestamp.FromTime(ts),
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
					T:      timestamp.FromTime(ts),
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
				T:   timestamp.FromTime(ts),
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
				T:   timestamp.FromTime(ts),
				H:   customBucketHist,
			}}, nil)
			require.NoError(t, w.Log(customBucketHistograms))

			floatHistograms, _ := enc.FloatHistogramSamples([]record.RefFloatHistogramSample{{
				Ref: chunks.HeadSeriesRef(inner),
				T:   timestamp.FromTime(ts),
				FH:  hist.ToFloat(nil),
			}}, nil)
			require.NoError(t, w.Log(floatHistograms))

			customBucketFloatHistograms := enc.CustomBucketsFloatHistogramSamples([]record.RefFloatHistogramSample{{
				Ref: chunks.HeadSeriesRef(inner),
				T:   timestamp.FromTime(ts),
				FH:  customBucketHist.ToFloat(nil),
			}}, nil)
			require.NoError(t, w.Log(customBucketFloatHistograms))
		}
	}
	// Add an unknown record type, which should be ignored.
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

func cutNewSegment(tb testing.TB, w *WL) {
	tb.Helper()

	_, err := w.NextSegment()
	require.NoError(tb, err)
}

// TestWatch_Live starts a watcher on an empty WAL and expects it to follow all
// the incoming, live data written the multiple segments.
func TestWatch_Live(t *testing.T) {
	const (
		seriesCount     = 10
		samplesCount    = 250
		exemplarsCount  = 25
		histogramsCount = 50
		segments        = 4
	)
	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			now := time.Now()
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			wt := newWriteToMock(0)
			watcher := newTestWatcher(dir, wt)
			// Start time has to be before now to read all samples correctly.
			isRunningFn := startWatching(t, watcher, func() time.Time { return now.Add(-1 * time.Millisecond) })

			// Write a few segments.
			ts := now
			for i := range segments {
				logTestWALRecords(t, ts, w, i, seriesCount, samplesCount, histogramsCount, exemplarsCount)
				ts = ts.Add(1 * time.Minute)
				if i < segments-1 {
					cutNewSegment(t, w)
				}
			}
			expectSegments(t, wdir, segments)

			// Watcher watched from the beginning so expect all data.
			wt.expectEventually(t, isRunningFn,
				seriesCount*segments,
				// Last segment has 2x samples, exemplars and histograms, but due to start time logic
				// we expect only half.
				seriesCount*samplesCount*segments,
				seriesCount*exemplarsCount*segments,
				seriesCount*histogramsCount*2*segments,
			)

			// Whole test should not wait for emergency read timeout.
			require.Less(t, time.Since(now), watcher.readTimeout)
		})
	}
}

// TestWatch_ReplayStartTime starts a watcher on an already filled WAL. We expect it to
// replay series from all segments and replay data from the last segment, respecting
// start time.
//
// After the replay we also test some further live data read.
func TestWatch_ReplayStartTime(t *testing.T) {
	const (
		seriesCount     = 10
		samplesCount    = 250
		exemplarsCount  = 25
		histogramsCount = 50
		initialSegments = 4
	)
	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			now := time.Now()
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write a few segments.
			ts := now
			for i := range initialSegments {
				logTestWALRecords(t, ts, w, i, seriesCount, samplesCount, histogramsCount, exemplarsCount)
				ts = ts.Add(1 * time.Minute)
				if i < initialSegments-1 {
					cutNewSegment(t, w)
				}
			}
			expectSegments(t, wdir, initialSegments)

			// For the last segment, log more entries, with the new timestamp to test start time logic.
			logTestWALRecords(t, ts.Add(1*time.Minute), w, initialSegments+1, seriesCount, samplesCount, histogramsCount, exemplarsCount)
			expectSegments(t, wdir, initialSegments) // Still the same last segment.

			// Create a watcher that should replay series and the last segment data.
			// Set start time to ts, so we expect only half of the last segment to be replayed.
			wt := newWriteToMock(0)
			watcher := newTestWatcher(dir, wt)
			isRunningFn := startWatching(t, watcher, func() time.Time { return ts })

			wt.expectEventually(t, isRunningFn,
				seriesCount*(initialSegments+1),
				// Last segment has 2x samples, exemplars and histograms, but due to start time logic
				// we expect only half.
				seriesCount*samplesCount,
				// TODO(bwplotka): Start time does not apply on exemplars, should it?
				seriesCount*exemplarsCount*2,
				seriesCount*histogramsCount*2,
			)

			// Cut a new segment and log new data.
			cutNewSegment(t, w)
			logTestWALRecords(t, ts.Add(2*time.Minute), w, initialSegments+2, seriesCount, samplesCount, histogramsCount, exemplarsCount)
			watcher.Notify()

			wt.expectEventually(t, isRunningFn,
				seriesCount*(initialSegments+2),
				seriesCount*samplesCount*2,
				// TODO(bwplotka): Start time does not apply on exemplars, should it?
				seriesCount*exemplarsCount*3,
				seriesCount*histogramsCount*2*2,
			)

			// Whole test should not wait for emergency read timeout.
			require.Less(t, time.Since(now), watcher.readTimeout)
		})
	}
}

// TestWatch_ReplayWithCheckpoint starts a watcher on an already filled WAL with
// a checkpoint. We expect it to replay series from all checkpoints and segments
// and replay data from the last segment.
//
// After the replay we also test some further live checkpoint.
func TestWatch_ReplayWithCheckpoint(t *testing.T) {
	const (
		seriesCount     = 10
		samplesCount    = 250
		exemplarsCount  = 25
		histogramsCount = 50
		initialSegments = 5
	)
	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			now := time.Now()
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write a few segments.
			ts := now
			for i := range initialSegments {
				logTestWALRecords(t, ts, w, i, seriesCount, samplesCount, histogramsCount, exemplarsCount)
				ts = ts.Add(1 * time.Minute)
				if i < initialSegments-1 {
					cutNewSegment(t, w)
				}
			}
			expectSegments(t, wdir, initialSegments)

			_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1, func(_ chunks.HeadSeriesRef, _ int) bool { return true }, 0)
			require.NoError(t, err)
			require.NoError(t, w.Truncate(2))
			expectSegments(t, wdir, initialSegments-2) // We should see 3 segment as the first 2 were truncated.

			// Create a watcher that should replay series and the last segment data.
			wt := newWriteToMock(0)
			watcher := newTestWatcher(dir, wt)
			watcher.checkpointPeriod = 500 * time.Millisecond // Checkpoint period is long-ish (best effort), make our tests faster.
			isRunningFn := startWatching(t, watcher, func() time.Time { return now })

			wt.expectEventually(t, isRunningFn,
				seriesCount*initialSegments,
				seriesCount*samplesCount,
				// TODO(bwplotka): Start time does not apply on exemplars, should it?
				seriesCount*exemplarsCount,
				seriesCount*histogramsCount*2,
			)

			// During watcher routine, do another checkpoint which deletes all previous series.
			// Then we truncate. We expect watcher to reset the series eventually (GC routine).
			_, err = Checkpoint(promslog.NewNopLogger(), w, 2, 3, func(chunks.HeadSeriesRef, int) bool { return false }, 0)
			require.NoError(t, err)
			err = w.Truncate(4)
			require.NoError(t, err)
			expectSegments(t, wdir, 1)

			wt.expectEventually(t, isRunningFn,
				seriesCount*2,
				seriesCount*samplesCount,
				// TODO(bwplotka): Start time does not apply on exemplars, should it?
				seriesCount*exemplarsCount,
				seriesCount*histogramsCount*2,
			)

			// Whole test should not wait for emergency read timeout.
			require.Less(t, time.Since(now), watcher.readTimeout)
		})
	}
}

// TestWatch_EmergencyReadTimeout ensures we will read even if we miss notification.
func TestWatch_EmergencyReadTimeout(t *testing.T) {
	const (
		seriesCount     = 10
		samplesCount    = 250
		exemplarsCount  = 25
		histogramsCount = 50
	)
	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			now := time.Now()
			dir := t.TempDir()
			wdir := path.Join(dir, "wal")
			require.NoError(t, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, w.Close())
			}()

			// Write to a segment.
			ts := now
			logTestWALRecords(t, ts, w, 0, seriesCount, samplesCount, histogramsCount, exemplarsCount)
			ts = ts.Add(1 * time.Minute)

			// Start watcher.
			wt := newWriteToMock(0)
			watcher := newTestWatcher(dir, wt)
			watcher.readTimeout = 200 * time.Millisecond // Make our test faster for the expected case.

			// Start time has to be before now to read all samples correctly.
			isRunningFn := startWatching(t, watcher, func() time.Time { return now.Add(-1 * time.Millisecond) })

			// Write to the same segment, without notification. This will rely on readTimeout.
			logTestWALRecords(t, ts, w, 1, seriesCount, samplesCount, histogramsCount, exemplarsCount)
			expectSegments(t, wdir, 1)

			// We expect data from the last segment.
			wt.expectEventually(t, isRunningFn,
				seriesCount*2,
				seriesCount*samplesCount*2,
				seriesCount*exemplarsCount*2,
				seriesCount*histogramsCount*2*2,
			)
		})
	}
}

func logBenchWALRealisticRecords(tb *testing.B, w *WL, seriesRecords, seriesPerRecord, sampleRecords int, samplesCase testrecord.RefSamplesCase) {
	tb.Helper()

	enc := record.Encoder{}
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
	for _, compress := range compression.Types() {
		for _, data := range []testrecord.RefSamplesCase{
			testrecord.Realistic1000Samples,
			testrecord.WorstCase1000Samples,
		} {
			b.Run(fmt.Sprintf("compr=%v/data=%v", compress, data), func(b *testing.B) {
				dir := b.TempDir()
				wdir := path.Join(dir, "wal")
				require.NoError(b, os.Mkdir(wdir, 0o777))

				w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
				require.NoError(b, err)
				defer func() {
					require.NoError(b, w.Close())
				}()

				logBenchWALRealisticRecords(b, w, seriesRecords, seriesPerRecord, sampleRecords, data)
				// 	// Build segment.
				//	require.NoError(tb, w.flushPage(true))
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
