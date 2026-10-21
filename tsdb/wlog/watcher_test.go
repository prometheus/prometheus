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
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/prometheus/prometheus/util/testwal"
)

var (
	defaultRetryInterval = 100 * time.Millisecond
	defaultRetries       = 100
	wMetrics             = NewWatcherMetrics(prometheus.DefaultRegisterer)
)

// retry executes f() n times at each interval until it returns true.
// TODO(bwplotka): Replace with require.Eventually.
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
	t.Log("function returned false")
}

// Overwrite readTimeout defined in watcher.go.
func overwriteReadTimeout(t *testing.T, val time.Duration) {
	initialVal := readTimeout
	readTimeout = val
	t.Cleanup(func() { readTimeout = initialVal })
}

type writeToMock struct {
	mu sync.Mutex

	seriesStored            []record.RefSeries
	metadataStored          []record.RefMetadata
	samplesAppended         []record.RefSample
	exemplarsAppended       []record.RefExemplar
	histogramsAppended      []record.RefHistogramSample
	floatHistogramsAppended []record.RefFloatHistogramSample

	seriesStores           int
	metadataStores         int
	sampleAppends          int
	exemplarAppends        int
	histogramAppends       int
	floatHistogramsAppends int

	seriesSegmentIndexes map[chunks.HeadSeriesRef]int

	// If nonzero, delay reads with a short sleep.
	delay time.Duration
}

func (wtm *writeToMock) Append(s []record.RefSample) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.sampleAppends++
	wtm.samplesAppended = append(wtm.samplesAppended, s...)
	time.Sleep(wtm.delay)
	return true
}

func (wtm *writeToMock) AppendExemplars(e []record.RefExemplar) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	time.Sleep(wtm.delay)
	wtm.exemplarAppends++
	wtm.exemplarsAppended = append(wtm.exemplarsAppended, e...)
	return true
}

func (wtm *writeToMock) AppendHistograms(h []record.RefHistogramSample) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	time.Sleep(wtm.delay)
	wtm.histogramAppends++
	wtm.histogramsAppended = append(wtm.histogramsAppended, h...)
	return true
}

func (wtm *writeToMock) AppendFloatHistograms(fh []record.RefFloatHistogramSample) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	time.Sleep(wtm.delay)
	wtm.floatHistogramsAppends++
	wtm.floatHistogramsAppended = append(wtm.floatHistogramsAppended, fh...)
	return true
}

func (wtm *writeToMock) StoreSeries(series []record.RefSeries, index int) {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.seriesStores++
	wtm.seriesStored = append(wtm.seriesStored, series...)
	for _, s := range series {
		wtm.seriesSegmentIndexes[s.Ref] = index
	}
	time.Sleep(wtm.delay)
}

func (wtm *writeToMock) StoreMetadata(meta []record.RefMetadata) {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.metadataStores++
	wtm.metadataStored = append(wtm.metadataStored, meta...)
	time.Sleep(wtm.delay)
}

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

func (wtm *writeToMock) checkNumSeries() int {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	return len(wtm.seriesSegmentIndexes)
}

func newWriteToMock(delay time.Duration) *writeToMock {
	return &writeToMock{
		seriesSegmentIndexes: make(map[chunks.HeadSeriesRef]int),
		delay:                delay,
	}
}

func TestWatcher_Tail(t *testing.T) {
	const (
		pageSize           = 32 * 1024
		batches            = 3
		seriesPerBatch     = 100
		exemplarsPerSeries = 2
	)
	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s/stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				var (
					now  = time.Now()
					dir  = t.TempDir()
					wdir = path.Join(dir, "wal")
					enc  = record.Encoder{EnableSTStorage: enableSTStorage}
				)
				require.NoError(t, os.Mkdir(wdir, 0o777))

				// Generate test records that represents batches of records data.
				// "batch" simulates a single scrape or RW/OTLP receive message.
				// Watcher does not inspect the data other than watching start timestamp, so records
				// does not need any certain shape.
				records := make([]testwal.Records, batches)
				cbHistogramRecords := make([]testwal.Records, batches)
				for i := range records {
					tsFn := func(_, _ int) int64 {
						return timestamp.FromTime(now.Add(1 * time.Second))
					}
					records[i] = testwal.GenerateRecords(testwal.RecordsCase{
						NoST:       !enableSTStorage,
						RefPadding: i * seriesPerBatch,
						TsFn:       tsFn,

						Series:                   seriesPerBatch,
						SamplesPerSeries:         10,
						HistogramsPerSeries:      5,
						FloatHistogramsPerSeries: 5,
						ExemplarsPerSeries:       exemplarsPerSeries,
					})
					cbHistogramRecords[i] = testwal.GenerateRecords(testwal.RecordsCase{
						NoST:       !enableSTStorage,
						RefPadding: i * seriesPerBatch,
						TsFn:       tsFn,

						Series:                   seriesPerBatch,
						HistogramsPerSeries:      5,
						FloatHistogramsPerSeries: 5,
						HistogramFn: func(ref int) *histogram.Histogram {
							return &histogram.Histogram{
								Schema:        -53,
								ZeroThreshold: 1e-128,
								ZeroCount:     0,
								Count:         2,
								Sum:           0,
								PositiveSpans: []histogram.Span{{Offset: 0, Length: 1}},
								CustomValues:  []float64{float64(ref) + 2},
							}
						},
					})
				}

				// Create WAL for writing.
				w, err := NewSize(nil, nil, wdir, 128*pageSize, compress)
				require.NoError(t, err)
				t.Cleanup(func() {
					require.NoError(t, w.Close())
				})

				// Start watcher to that reads into a mock.
				wt := newWriteToMock(0)
				watcher := NewWatcher(wMetrics, nil, nil, "test", wt, dir, true, true, true, nil)
				// Update the time because we just created samples around "now" time and watcher
				// only starts watching after that time.
				watcher.SetStartTime(now)
				// Start spins up watcher loop in a go-routine.
				watcher.Start()
				t.Cleanup(watcher.Stop)

				// Write to WAL like append commit would do, while watcher is tailing.

				// Write first a few samples before the start time, we don't expect those to be appended.
				require.NoError(t, w.Log(enc.Samples([]record.RefSample{
					{Ref: 1, T: timestamp.FromTime(now), V: 123},
					{Ref: 2, T: timestamp.FromTime(now), V: 123.1},
				}, nil)))

				for i := range records {
					// Similar order as tsdb/head_appender.go.headAppenderBase.log
					// https://github.com/prometheus/prometheus/blob/1751685dd4f6430757ba3078a96cffeffcb2bb47/tsdb/head_append.go#L1053
					require.NoError(t, w.Log(enc.Series(records[i].Series, nil)))
					require.NoError(t, w.Log(enc.Metadata(records[i].Metadata, nil)))
					require.NoError(t, w.Log(enc.Samples(records[i].Samples, nil)))

					hs, cbHs := enc.HistogramSamples(records[i].Histograms, nil)
					require.Empty(t, cbHs)
					require.NoError(t, w.Log(hs))
					fhs, cbFhs := enc.FloatHistogramSamples(records[i].FloatHistograms, nil)
					require.Empty(t, cbFhs)
					require.NoError(t, w.Log(fhs))
					require.NoError(t, w.Log(enc.CustomBucketsHistogramSamples(cbHistogramRecords[i].Histograms, nil)))
					require.NoError(t, w.Log(enc.CustomBucketsFloatHistogramSamples(cbHistogramRecords[i].FloatHistograms, nil)))

					require.NoError(t, w.Log(enc.Exemplars(records[i].Exemplars, nil)))

					// Ping watcher for faster test. Watcher is checking for segment changes or 15s timeout.
					watcher.Notify()
				}

				// Wait for watcher to lead all.
				require.Eventually(t, func() bool {
					wt.mu.Lock()
					defer wt.mu.Unlock()

					// Exemplars are logged as the last one, so assert on those.
					return wt.exemplarAppends >= batches
				}, 2*time.Minute, 1*time.Second)

				wt.mu.Lock()
				defer wt.mu.Unlock()

				require.Equal(t, batches, wt.seriesStores)
				require.Equal(t, batches, wt.metadataStores)
				require.Equal(t, batches, wt.sampleAppends)
				require.Equal(t, 2*batches, wt.histogramAppends)
				require.Equal(t, 2*batches, wt.floatHistogramsAppends)
				require.Equal(t, batches, wt.exemplarAppends)

				for i := range batches {
					sector := len(records[i].Series)
					testutil.RequireEqual(t, records[i].Series, wt.seriesStored[i*sector:(i+1)*sector], i)
					sector = len(records[i].Metadata)
					require.Equal(t, records[i].Metadata, wt.metadataStored[i*sector:(i+1)*sector], i)
					sector = len(records[i].Samples)
					require.Equal(t, records[i].Samples, wt.samplesAppended[i*sector:(i+1)*sector], i)

					sector = len(records[i].Histograms) + len(cbHistogramRecords[i].Histograms)
					require.Equal(t, records[i].Histograms, wt.histogramsAppended[i*sector:i*sector+len(records[i].Histograms)], i)
					require.Equal(t, cbHistogramRecords[i].Histograms, wt.histogramsAppended[i*sector+len(records[i].Histograms):(i+1)*sector])
					sector = len(records[i].FloatHistograms) + len(cbHistogramRecords[i].FloatHistograms)
					require.Equal(t, records[i].FloatHistograms, wt.floatHistogramsAppended[i*sector:i*sector+len(records[i].FloatHistograms)])
					require.Equal(t, cbHistogramRecords[i].FloatHistograms, wt.floatHistogramsAppended[i*sector+len(records[i].FloatHistograms):(i+1)*sector])

					sector = len(records[i].Exemplars)
					testutil.RequireEqual(t, records[i].Exemplars, wt.exemplarsAppended[i*sector:(i+1)*sector])
				}
			})
		}
	}
}

func TestReadToEndNoCheckpoint(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
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

				enc := record.Encoder{EnableSTStorage: enableSTStorage}

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
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
				go watcher.Start()

				expected := seriesCount
				require.Eventually(t, func() bool {
					return wt.checkNumSeries() == expected
				}, 20*time.Second, 1*time.Second)
				watcher.Stop()
			})
		}
	}
}

func TestReadToEndWithCheckpoint(t *testing.T) {
	segmentSize := 32 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 10
	const samplesCount = 250

	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				dir := t.TempDir()

				wdir := path.Join(dir, "wal")
				err := os.Mkdir(wdir, 0o777)
				require.NoError(t, err)

				enc := record.Encoder{EnableSTStorage: enableSTStorage}
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

				Checkpoint(promslog.NewNopLogger(), w, 0, 1, func(chunks.HeadSeriesRef) bool { return true }, 0, enableSTStorage, false)
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
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
				go watcher.Start()

				expected := seriesCount * 2

				require.Eventually(t, func() bool {
					return wt.checkNumSeries() == expected
				}, 10*time.Second, 1*time.Second)
				watcher.Stop()
			})
		}
	}
}

func TestReadCheckpoint(t *testing.T) {
	pageSize := 32 * 1024
	const seriesCount = 10
	const samplesCount = 250

	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				dir := t.TempDir()

				wdir := path.Join(dir, "wal")
				err := os.Mkdir(wdir, 0o777)
				require.NoError(t, err)

				f, err := os.Create(SegmentName(wdir, 30))
				require.NoError(t, err)
				require.NoError(t, f.Close())

				enc := record.Encoder{EnableSTStorage: enableSTStorage}
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
				_, err = Checkpoint(promslog.NewNopLogger(), w, 30, 31, func(chunks.HeadSeriesRef) bool { return true }, 0, enableSTStorage, false)
				require.NoError(t, err)
				require.NoError(t, w.Truncate(32))

				// Start read after checkpoint, no more data written.
				_, _, err = Segments(w.Dir())
				require.NoError(t, err)

				wt := newWriteToMock(0)
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
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
}

func TestReadCheckpointMultipleSegments(t *testing.T) {
	pageSize := 32 * 1024

	const segments = 1
	const seriesCount = 40
	const samplesCount = 500

	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				dir := t.TempDir()

				wdir := path.Join(dir, "wal")
				err := os.Mkdir(wdir, 0o777)
				require.NoError(t, err)

				enc := record.Encoder{EnableSTStorage: enableSTStorage}
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
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
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
}

func TestCheckpointSeriesReset(t *testing.T) {
	segmentSize := 64 * 1024
	// We need something similar to this # of series and samples
	// in order to get enough segments for us to checkpoint.
	const seriesCount = 30
	const samplesCount = 700
	testCases := []struct {
		compress        compression.Type
		enableSTStorage bool
		segments        int
	}{
		{compress: compression.None, enableSTStorage: false, segments: 24},
		{compress: compression.Snappy, enableSTStorage: false, segments: 23},
		{compress: compression.None, enableSTStorage: true, segments: 20},
		{compress: compression.Snappy, enableSTStorage: true, segments: 20},
	}

	dir := t.TempDir()
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("compress=%s,stStorage=%v", tc.compress, tc.enableSTStorage), func(t *testing.T) {
			subdir := filepath.Join(dir, fmt.Sprintf("%s-%v", tc.compress, tc.enableSTStorage))
			err := os.MkdirAll(subdir, 0o777)
			require.NoError(t, err)
			wdir := filepath.Join(subdir, "wal")
			err = os.MkdirAll(wdir, 0o777)
			require.NoError(t, err)

			enc := record.Encoder{EnableSTStorage: tc.enableSTStorage}
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
			watcher := NewWatcher(wMetrics, nil, nil, "", wt, subdir, false, false, false, nil)
			watcher.MaxSegment = -1
			go watcher.Start()

			expected := seriesCount
			retry(t, defaultRetryInterval, defaultRetries, func() bool {
				return wt.checkNumSeries() >= expected
			})
			require.Eventually(t, func() bool {
				return wt.checkNumSeries() == seriesCount
			}, 10*time.Second, 1*time.Second)

			// Simulate a second watcher lagging at segment 1 before checkpoint and truncate.
			restartWt := newWriteToMock(0)
			restartWatcher := NewWatcher(wMetrics, nil, nil, "", restartWt, subdir, false, false, false, nil)
			restartWatcher.SetMetrics()
			restartWatcher.MaxSegment = 1
			require.NoError(t, restartWatcher.Run())
			require.Equal(t, seriesCount-tc.segments, restartWt.checkNumSeries())

			_, err = Checkpoint(promslog.NewNopLogger(), w, 2, 4, func(chunks.HeadSeriesRef) bool { return true }, 0, true, false)
			require.NoError(t, err)

			err = w.Truncate(5)
			require.NoError(t, err)

			_, cpi, err := LastCheckpoint(wdir)
			require.NoError(t, err)
			// When current segment is at or behind the checkpoint, garbageCollectSeries
			// must skip the checkpoint without marking it as already processed.
			require.NoError(t, watcher.garbageCollectSeries(cpi))
			err = watcher.garbageCollectSeries(cpi + 1)
			require.NoError(t, err)

			watcher.Stop()
			// If you modify the checkpoint and truncate segment #'s run the test to see how
			// many series records you end up with and change the last Equals check accordingly
			// or modify the Equals to Assert(len(wt.seriesLabels) < seriesCount*10)
			require.Eventually(t, func() bool {
				return wt.checkNumSeries() == tc.segments
			}, 20*time.Second, 1*time.Second)

			// When Run restarts on a checkpoint that garbageCollectSeries never processed,
			// it must call SeriesReset to drop series from segments older than the checkpoint.
			restartWatcher.MaxSegment = cpi + 1
			require.NoError(t, restartWatcher.Run())
			for ref, seg := range restartWt.seriesSegmentIndexes {
				require.GreaterOrEqual(t, seg, cpi, "series %d from segment %d was not garbage collected on Run restart", ref, seg)
			}
		})
	}
}

func TestRun_StartupTime(t *testing.T) {
	const pageSize = 32 * 1024
	const segments = 20
	const seriesCount = 40
	const samplesCount = 500

	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				dir := t.TempDir()

				wdir := path.Join(dir, "wal")
				err := os.Mkdir(wdir, 0o777)
				require.NoError(t, err)

				enc := record.Encoder{EnableSTStorage: enableSTStorage}
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
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
				watcher.MaxSegment = segments

				watcher.SetMetrics()
				startTime := time.Now()

				err = watcher.Run()
				require.Less(t, time.Since(startTime), readTimeout)
				require.NoError(t, err)
			})
		}
	}
}

func generateWALRecords(w *WL, segment, seriesCount, samplesCount int, enableSTStorage bool) error {
	enc := record.Encoder{EnableSTStorage: enableSTStorage}
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

	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				dir := t.TempDir()

				wdir := path.Join(dir, "wal")
				err := os.Mkdir(wdir, 0o777)
				require.NoError(t, err)

				w, err := NewSize(nil, nil, wdir, segmentSize, compress)
				require.NoError(t, err)
				// Write to 00000000, the watcher will read series from it.
				require.NoError(t, generateWALRecords(w, 0, seriesCount, samplesCount, enableSTStorage))
				// Create 00000001, the watcher will tail it once started.
				w.NextSegment()

				// Set up the watcher and run it in the background.
				wt := newWriteToMock(time.Millisecond)
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
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
					require.NoError(t, generateWALRecords(w, i, seriesCount, samplesCount, enableSTStorage))
					w.NextSegment()
				}

				// Wait for the watcher.
				require.NoError(t, g.Wait())

				// All series and samples were read.
				require.Equal(t, (segmentsToRead+1)*seriesCount, wt.checkNumSeries()) // Series from 00000000 are also read.
				require.Len(t, wt.samplesAppended, segmentsToRead*seriesCount*samplesCount)
				require.NoError(t, w.Close())
			})
		}
	}
}

// nativeWriteToMock records what a watcher passes a native metadata writer
// since its last reset: entries as describeCompactNativeMetadata describes
// them, latest-value entries as "legacy ref help", the segment each record was
// read from, and the refs garbage collection kept.
type nativeWriteToMock struct {
	*writeToMock
	entries  []string
	segments []int
	kept     map[chunks.HeadSeriesRef]int
	// resets holds the checkpoint flag of each reset.
	resets []bool
}

func newNativeWriteToMock() *nativeWriteToMock {
	return &nativeWriteToMock{writeToMock: newWriteToMock(0)}
}

func (m *nativeWriteToMock) StoreNativeMetadataRecord(rec *record.CompactNativeMetadata, segment int) {
	m.segments = append(m.segments, segment)
	// The record's strings alias the watcher's buffer; describing copies them.
	for _, s := range describeCompactNativeMetadata(rec) {
		m.entries = append(m.entries, strings.Clone(s))
	}
}

func (m *nativeWriteToMock) StoreLegacyNativeMetadata(meta []record.RefMetadata, segment int) {
	m.segments = append(m.segments, segment)
	for _, e := range meta {
		m.entries = append(m.entries, fmt.Sprintf("legacy %d %s", e.Ref, e.Help))
	}
}

func (m *nativeWriteToMock) UpdateNativeMetadataSegment(refs []chunks.HeadSeriesRef, segment int) {
	if m.kept == nil {
		m.kept = map[chunks.HeadSeriesRef]int{}
	}
	for _, ref := range refs {
		m.kept[ref] = max(m.kept[ref], segment)
	}
}

func (m *nativeWriteToMock) ResetNativeMetadata(checkpoint bool) {
	m.entries, m.segments = nil, nil
	m.resets = append(m.resets, checkpoint)
}

func TestWatcher_NativeMetadata(t *testing.T) {
	var enc record.Encoder
	// Each segment holds one series record and the listed records.
	segments := [][][]byte{
		{nativeMetadataGroupRecord(t, 1, "A@100"), nativeMetadataGroupRecord(t, 2, "A@10", "B@20", "C@30", "D@40", "E@50")},
		{nativeMetadataGroupRecord(t, 1, "B@150", "C@180"), nativeMetadataGroupRecord(t, 2, "F@60")},
		{enc.Metadata([]record.RefMetadata{{Ref: 3, Help: "L"}}, nil), nativeMetadataGroupRecord(t, 1, "D@200")},
		{nativeMetadataGroupRecord(t, 2, "D@50"), nativeMetadataGroupRecord(t, 3, "M@5")},
		{nativeMetadataOverrideRecord(t, 1, true, true, "A@120"), nativeMetadataGroupRecord(t, 2, "G@70")},
	}
	type setup struct {
		dir     string
		w       *WL
		entries []string
	}
	newWAL := func(t *testing.T) setup {
		dir := t.TempDir()
		w, err := NewSize(nil, nil, filepath.Join(dir, "wal"), 32*1024, compression.None)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, w.Close()) })
		var all []string
		for i, recs := range segments {
			require.NoError(t, w.Log(enc.Series([]record.RefSeries{{Ref: chunks.HeadSeriesRef(i + 1), Labels: labels.FromStrings("__name__", strconv.Itoa(i))}}, nil)))
			for _, rec := range recs {
				require.NoError(t, w.Log(rec))
				all = append(all, describeNativeMetadataRecord(t, rec)...)
			}
			// Leave an empty tail segment, so that replay reads every segment
			// to its end and returns.
			_, err := w.NextSegment()
			require.NoError(t, err)
		}
		return setup{dir: dir, w: w, entries: all}
	}
	newWatcher := func(s setup) (*Watcher, *nativeWriteToMock) {
		wt := newNativeWriteToMock()
		watcher := NewWatcher(wMetrics, nil, nil, "", wt, s.dir, false, false, true, nil)
		watcher.SetMetrics()
		watcher.MaxSegment = len(segments) - 1
		return watcher, wt
	}
	checkpoint := func(t *testing.T, s setup, from, to int) {
		_, err := Checkpoint(promslog.NewNopLogger(), s.w, from, to, func(chunks.HeadSeriesRef) bool { return true }, 0, false, true)
		require.NoError(t, err)
	}

	t.Run("records are forwarded in WAL order", func(t *testing.T) {
		s := newWAL(t)
		watcher, wt := newWatcher(s)
		require.NoError(t, watcher.Run())
		require.Equal(t, s.entries, wt.entries)
		require.Zero(t, wt.metadataStores, "latest-value entries reach the native writer")
		require.Equal(t, []bool{false}, wt.resets)
	})

	t.Run("each replay starts from reset state", func(t *testing.T) {
		for _, withCheckpoint := range []bool{false, true} {
			s := newWAL(t)
			if withCheckpoint {
				checkpoint(t, s, 0, 1)
				require.NoError(t, s.w.Truncate(2))
			}
			watcher, wt := newWatcher(s)
			require.NoError(t, watcher.Run())
			first := wt.entries
			require.NoError(t, watcher.Run())
			// The writer learns whether a checkpoint starts the replay.
			require.Equal(t, []bool{withCheckpoint, withCheckpoint}, wt.resets)
			require.Equal(t, first, wt.entries)
		}
	})

	t.Run("replay starts after the checkpoint's own segment", func(t *testing.T) {
		// A failed truncation leaves the checkpointed segments in place.
		s := newWAL(t)
		checkpoint(t, s, 0, 1)
		watcher, wt := newWatcher(s)
		require.NoError(t, watcher.Run())
		// The checkpoint's marker and reduced overrides, then segments 2 to 4.
		var want []string
		for _, recs := range segments[2:] {
			for _, rec := range recs {
				want = append(want, describeNativeMetadataRecord(t, rec)...)
			}
		}
		require.Equal(t, "provenance", wt.entries[0])
		require.Equal(t, want, wt.entries[len(wt.entries)-len(want):])
		require.Equal(t, sortedStrings([]string{"provenance", "override 1 A@100 B@150 C@180", "override 2 truncated B@20 C@30 D@40 E@50 F@60"}), sortedStrings(wt.entries[:len(wt.entries)-len(want)]))
	})

	t.Run("a newer checkpoint restarts replay", func(t *testing.T) {
		s := newWAL(t)
		checkpoint(t, s, 0, 1)
		require.NoError(t, s.w.Truncate(2))
		watcher, wt := newWatcher(s)
		raced := false
		watcher.testAfterCheckpoint = func() {
			if raced {
				return
			}
			raced = true
			// Checkpointing races the watcher, deleting the next segments.
			checkpoint(t, s, 2, 3)
			require.NoError(t, s.w.Truncate(4))
			require.NoError(t, DeleteCheckpoints(s.w.Dir(), 3))
		}
		require.NoError(t, watcher.Run())
		require.Equal(t, []bool{true, true}, wt.resets)
		_, index, err := LastCheckpoint(s.w.Dir())
		require.NoError(t, err)
		require.Equal(t, 3, index)
	})

	t.Run("missing segments without a newer checkpoint", func(t *testing.T) {
		s := newWAL(t)
		checkpoint(t, s, 0, 1)
		require.NoError(t, s.w.Truncate(2))
		require.NoError(t, os.Remove(SegmentName(s.w.Dir(), 2)))
		watcher, wt := newWatcher(s)
		require.NoError(t, watcher.Run())
		require.Len(t, wt.resets, 1)
		require.NotEmpty(t, wt.entries)
	})

	t.Run("records carry the segment they were read from", func(t *testing.T) {
		// A checkpoint's are read from its index: its marker, then its
		// reduced records.
		for _, c := range []struct {
			name       string
			checkpoint bool
			want       []int
		}{
			{name: "segments", want: []int{0, 0, 1, 1, 2, 2, 3, 3, 4, 4}},
			{name: "a checkpoint, then segments", checkpoint: true, want: []int{1, 1, 2, 2, 3, 3, 4, 4}},
		} {
			t.Run(c.name, func(t *testing.T) {
				s := newWAL(t)
				if c.checkpoint {
					checkpoint(t, s, 0, 1)
					require.NoError(t, s.w.Truncate(2))
				}
				watcher, wt := newWatcher(s)
				require.NoError(t, watcher.Run())
				require.Equal(t, c.want, wt.segments)
			})
		}
	})

	t.Run("garbage collection keeps the checkpoint's metadata refs", func(t *testing.T) {
		// Segments 0 to 2 hold metadata for refs 1, 2 and 3, including a
		// latest-value entry for ref 3; a checkpoint of them keeps all three.
		s := newWAL(t)
		watcher, wt := newWatcher(s)
		require.NoError(t, watcher.Run())
		checkpoint(t, s, 0, 2)
		require.NoError(t, watcher.garbageCollectSeries(4))
		require.Equal(t, map[chunks.HeadSeriesRef]int{1: 2, 2: 2, 3: 2}, wt.kept)
	})

	t.Run("writers without native metadata never see compact records", func(t *testing.T) {
		s := newWAL(t)
		wt := newWriteToMock(0)
		watcher := NewWatcher(wMetrics, nil, nil, "", wt, s.dir, false, false, true, nil)
		watcher.SetMetrics()
		watcher.MaxSegment = len(segments) - 1
		require.NoError(t, watcher.Run())
		require.Equal(t, []record.RefMetadata{{Ref: 3, Help: "L"}}, wt.metadataStored)
	})

	t.Run("strings outlive the reused record buffer", func(t *testing.T) {
		// One segment of many records, whose reader reuses its record buffer.
		dir := t.TempDir()
		w, err := NewSize(nil, nil, filepath.Join(dir, "wal"), 1<<20, compression.None)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, w.Close()) })
		require.NoError(t, w.Log(enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("__name__", "a")}}, nil)))
		var want []string
		for i := range 200 {
			help := strings.Repeat(strconv.Itoa(i), 1+i%7)
			rec := nativeMetadataGroupRecord(t, 1, fmt.Sprintf("%s@%d", help, 2*i), fmt.Sprintf("%s@%d", help, 2*i+1))
			require.NoError(t, w.Log(rec))
			want = append(want, describeNativeMetadataRecord(t, rec)...)
		}
		_, err = w.NextSegment()
		require.NoError(t, err)
		watcher, wt := newWatcher(setup{dir: dir})
		watcher.MaxSegment = 0
		require.NoError(t, watcher.Run())
		require.Equal(t, want, wt.entries)
	})

	t.Run("garbage collection keeps the refs of compact checkpoint records", func(t *testing.T) {
		// A checkpoint that keeps metadata only in compact records: without
		// their refs, the writer would drop the histories the checkpoint
		// keeps once it resets older series.
		dir := t.TempDir()
		walDir := filepath.Join(dir, "wal")
		w, err := NewSize(nil, nil, walDir, 32*1024, compression.None)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, w.Close()) })
		for range 4 {
			_, err := w.NextSegment()
			require.NoError(t, err)
		}
		cp, err := New(nil, nil, filepath.Join(walDir, checkpointPrefix+"00000002"), compression.None)
		require.NoError(t, err)
		require.NoError(t, cp.Log(nativeMetadataMarkerRecord(record.NativeMetadataProvenance)))
		require.NoError(t, cp.Log(nativeMetadataOverrideRecord(t, 1, false, false, "A@1")))
		require.NoError(t, cp.Log(nativeMetadataOverrideRecord(t, 2, false, true, "A@2")))
		require.NoError(t, cp.Log(nativeMetadataOverrideRecord(t, 3, true, false)))
		require.NoError(t, cp.Close())
		watcher, wt := newWatcher(setup{dir: dir})
		require.NoError(t, watcher.garbageCollectSeries(4))
		require.Equal(t, map[chunks.HeadSeriesRef]int{1: 2, 2: 2, 3: 2}, wt.kept)
	})
}

// TestWatcher_NativeMetadataDecodeFailure checks that a watcher forwarding
// native metadata delivers nothing past a record it can't decode, wherever the
// record is, and however often it retries.
// recordingGauge records the values a gauge is set to.
type recordingGauge struct {
	prometheus.Gauge
	mtx sync.Mutex
	set []float64
}

func (g *recordingGauge) Set(v float64) {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	g.set = append(g.set, v)
	g.Gauge.Set(v)
}

func (g *recordingGauge) values() []float64 {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	return slices.Clone(g.set)
}

func TestWatcher_NativeMetadataDecodeFailure(t *testing.T) {
	var enc record.Encoder
	for name, bad := range undecodableNativeMetadataRecords(t) {
		t.Run(name, func(t *testing.T) {
			// newWAL writes segment 0, holding a series and a sample, and
			// then the bad record either in a checkpoint of segment 0 or in
			// segment 1, followed in segment 1 by a sample at 200.
			newWAL := func(t *testing.T, inCheckpoint bool) (string, *WL) {
				dir := t.TempDir()
				w, err := NewSize(nil, nil, filepath.Join(dir, "wal"), 32*1024, compression.None)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, w.Close()) })
				require.NoError(t, w.Log(enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("__name__", "a")}}, nil)))
				require.NoError(t, w.Log(enc.Samples([]record.RefSample{{Ref: 1, T: 100, V: 1}}, nil)))
				_, err = w.NextSegment()
				require.NoError(t, err)
				if inCheckpoint {
					cp, err := New(nil, nil, filepath.Join(dir, "wal", checkpointPrefix+"00000000"), compression.None)
					require.NoError(t, err)
					require.NoError(t, cp.Log(enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("__name__", "a")}}, nil)))
					require.NoError(t, cp.Log(bad))
					require.NoError(t, cp.Close())
				} else {
					require.NoError(t, w.Log(bad))
				}
				require.NoError(t, w.Log(enc.Samples([]record.RefSample{{Ref: 1, T: 200, V: 2}}, nil)))
				return dir, w
			}
			newWatcher := func(dir string) (*Watcher, *nativeWriteToMock) {
				wt := newNativeWriteToMock()
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, true, nil)
				watcher.SetMetrics()
				watcher.SetStartTime(time.UnixMilli(0))
				return watcher, wt
			}
			requireHeld := func(t *testing.T, err error, wt *nativeWriteToMock) {
				t.Helper()
				var decodeErr *NativeMetadataDecodeError
				require.ErrorAs(t, err, &decodeErr)
				for _, s := range wt.samplesAppended {
					require.Less(t, s.T, int64(200), "nothing past the record is delivered")
				}
			}
			// runTail runs the watcher on a WAL whose tail holds the bad
			// record until Run returns, notifying it of writes meanwhile.
			runTail := func(watcher *Watcher, poke func()) error {
				done := make(chan error, 1)
				go func() { done <- watcher.Run() }()
				for {
					select {
					case err := <-done:
						return err
					case <-time.After(20 * time.Millisecond):
						poke()
					}
				}
			}

			t.Run("in the checkpoint", func(t *testing.T) {
				dir, w := newWAL(t, true)
				_, err := w.NextSegment()
				require.NoError(t, err)
				watcher, wt := newWatcher(dir)
				watcher.MaxSegment = 2
				for range 2 {
					requireHeld(t, watcher.Run(), wt)
				}
			})

			t.Run("in a segment read before the tail", func(t *testing.T) {
				dir, w := newWAL(t, false)
				_, err := w.NextSegment()
				require.NoError(t, err)
				watcher, wt := newWatcher(dir)
				watcher.MaxSegment = 2
				for range 2 {
					requireHeld(t, watcher.Run(), wt)
				}
			})

			t.Run("in the active tail", func(t *testing.T) {
				dir, _ := newWAL(t, false)
				watcher, wt := newWatcher(dir)
				requireHeld(t, runTail(watcher, watcher.Notify), wt)
			})

			t.Run("in the tail, retried after the segment rotated", func(t *testing.T) {
				dir, w := newWAL(t, false)
				watcher, wt := newWatcher(dir)
				rotated := false
				requireHeld(t, runTail(watcher, func() {
					if !rotated {
						_, err := w.NextSegment()
						require.NoError(t, err)
						rotated = true
					}
				}), wt)
				// The retry reads the segment before its new tail.
				watcher.MaxSegment = 2
				requireHeld(t, watcher.Run(), wt)
			})

			t.Run("the held gauge", func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					dir, w := newWAL(t, false)
					watcher, _ := newWatcher(dir)
					gauge := &recordingGauge{Gauge: prometheus.NewGauge(prometheus.GaugeOpts{Name: "held"})}
					watcher.nativeMetadataHeld = gauge
					go watcher.loop()
					defer func() {
						close(watcher.quit)
						<-watcher.done
					}()
					// Every retry holds at the record, and the gauge never
					// reports otherwise in between.
					time.Sleep(time.Minute)
					synctest.Wait()
					require.Greater(t, len(gauge.values()), 1)
					require.NotContains(t, gauge.values(), 0.0)

					// A checkpoint of the record's segment, without it, lets
					// the next retry get past it.
					cp, err := New(nil, nil, filepath.Join(dir, "wal", checkpointPrefix+"00000001"), compression.None)
					require.NoError(t, err)
					require.NoError(t, cp.Log(enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("__name__", "a")}}, nil)))
					require.NoError(t, cp.Close())
					_, err = w.NextSegment()
					require.NoError(t, err)
					time.Sleep(time.Minute)
					synctest.Wait()
					values := gauge.values()
					require.Equal(t, 0.0, values[len(values)-1])
				})
			})

			t.Run("in a checkpoint that garbage collection scans", func(t *testing.T) {
				dir, w := newWAL(t, true)
				for range 2 {
					_, err := w.NextSegment()
					require.NoError(t, err)
				}
				watcher, wt := newWatcher(dir)
				wt.seriesSegmentIndexes[1] = 0
				err := watcher.garbageCollectSeries(2)
				var decodeErr *NativeMetadataDecodeError
				require.ErrorAs(t, err, &decodeErr)
				// The scan failed before any reset, so the series is kept.
				require.Equal(t, 1, wt.checkNumSeries())
			})
		})
	}
}
