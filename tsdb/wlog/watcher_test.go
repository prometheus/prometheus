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
	"io"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
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

type writeToMock struct {
	mu        sync.Mutex
	benchMode bool

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

func (wtm *writeToMock) Reset() {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.seriesStored = nil
	wtm.metadataStored = nil
	wtm.samplesAppended = nil
	wtm.exemplarsAppended = nil
	wtm.histogramsAppended = nil
	wtm.floatHistogramsAppended = nil
	wtm.seriesStores = 0
	wtm.metadataStores = 0
	wtm.sampleAppends = 0
	wtm.exemplarAppends = 0
	wtm.histogramAppends = 0
	wtm.floatHistogramsAppends = 0
	clear(wtm.seriesSegmentIndexes)
}

func (wtm *writeToMock) Append(s []record.RefSample) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.sampleAppends++
	if wtm.benchMode {
		return true
	}
	wtm.samplesAppended = append(wtm.samplesAppended, s...)
	time.Sleep(wtm.delay)
	return true
}

func (wtm *writeToMock) AppendExemplars(e []record.RefExemplar) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.exemplarAppends++
	if wtm.benchMode {
		return true
	}
	time.Sleep(wtm.delay)
	wtm.exemplarsAppended = append(wtm.exemplarsAppended, e...)
	return true
}

func (wtm *writeToMock) AppendHistograms(h []record.RefHistogramSample) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.histogramAppends++
	if wtm.benchMode {
		return true
	}
	time.Sleep(wtm.delay)
	wtm.histogramsAppended = append(wtm.histogramsAppended, h...)
	return true
}

func (wtm *writeToMock) AppendFloatHistograms(fh []record.RefFloatHistogramSample) bool {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.floatHistogramsAppends++
	if wtm.benchMode {
		return true
	}
	time.Sleep(wtm.delay)
	wtm.floatHistogramsAppended = append(wtm.floatHistogramsAppended, fh...)
	return true
}

func (wtm *writeToMock) StoreSeries(series []record.RefSeries, index int) {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	wtm.seriesStores++
	if wtm.benchMode {
		return
	}
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
	if wtm.benchMode {
		return
	}
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

				// Wait for watcher to read all.
				require.Eventually(t, func() bool {
					wt.mu.Lock()
					defer wt.mu.Unlock()

					// Exemplars are logged as the last one, so assert on those.
					return wt.exemplarAppends >= batches
				}, 2*time.Minute, 100*time.Millisecond)

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
				_, _, err = Segments(w.Dir())
				require.NoError(t, err)

				wt := newWriteToMock(0)
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
				watcher.readTimeout = 100 * time.Millisecond
				go watcher.Start()

				expected := seriesCount
				require.Eventually(t, func() bool {
					return wt.checkNumSeries() == expected
				}, 20*time.Second, 10*time.Millisecond)

				// Write one more series without calling watcher.Notify() to verify readTimeout fallback.
				require.NoError(t, w.Log(enc.Series([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(seriesCount),
						Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", seriesCount)),
					},
				}, nil)))
				require.Eventually(t, func() bool {
					return wt.checkNumSeries() == expected+1
				}, 20*time.Second, 10*time.Millisecond)
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

				Checkpoint(promslog.NewNopLogger(), w, 0, 1, func(chunks.HeadSeriesRef) bool { return true }, 0, enableSTStorage, true)
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
				wt := newWriteToMock(0)
				watcher := NewWatcher(wMetrics, nil, nil, "", wt, dir, false, false, false, nil)
				go watcher.Start()

				expected := seriesCount * 2

				require.Eventually(t, func() bool {
					return wt.checkNumSeries() == expected
				}, 10*time.Second, 100*time.Millisecond)
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
				_, err = Checkpoint(promslog.NewNopLogger(), w, 30, 31, func(chunks.HeadSeriesRef) bool { return true }, 0, enableSTStorage, true)
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
			}, 10*time.Second, 100*time.Millisecond)

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
			}, 20*time.Second, 100*time.Millisecond)
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
				require.Less(t, time.Since(startTime), watcher.readTimeout)
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
					if d > watcher.readTimeout {
						return fmt.Errorf("watcher ran for %s, it shouldn't rely on readTicker=%s to read the new segments", d, watcher.readTimeout)
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

type offsetLimitReader struct {
	r     io.Reader
	pos   int64
	limit int64
}

func (l *offsetLimitReader) Read(p []byte) (int, error) {
	rem := l.limit - l.pos
	if rem <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > rem {
		p = p[:rem]
	}
	n, err := l.r.Read(p)
	l.pos += int64(n)
	return n, err
}

func logBenchRecords(tb testing.TB, w *WL, enc *record.Encoder, recs testwal.Records) {
	tb.Helper()
	if len(recs.Series) > 0 {
		require.NoError(tb, w.Log(enc.Series(recs.Series, nil)))
	}
	if len(recs.Metadata) > 0 {
		require.NoError(tb, w.Log(enc.Metadata(recs.Metadata, nil)))
	}
	if len(recs.Samples) > 0 {
		require.NoError(tb, w.Log(enc.Samples(recs.Samples, nil)))
	}
	if len(recs.Histograms) > 0 {
		hs, _ := enc.HistogramSamples(recs.Histograms, nil)
		require.NoError(tb, w.Log(hs))
	}
	if len(recs.FloatHistograms) > 0 {
		fhs, _ := enc.FloatHistogramSamples(recs.FloatHistograms, nil)
		require.NoError(tb, w.Log(fhs))
	}
	if len(recs.Exemplars) > 0 {
		require.NoError(tb, w.Log(enc.Exemplars(recs.Exemplars, nil)))
	}
}

// Recommended CLI invocation:
/*
	export bench=main && go test ./tsdb/wlog/... \
		-run '^$' -bench '^BenchmarkWatcher_ReadSegment' \
		-benchtime 10x -count 6 -cpu 2 -timeout 999m -benchmem \
		| tee ${bench}.txt
*/
func BenchmarkWatcher_ReadSegment(b *testing.B) {
	now := time.Now()
	tsFn := func(_, _ int) int64 {
		return timestamp.FromTime(now.Add(1 * time.Second))
	}

	initSamplesRecs := testwal.GenerateRecords(testwal.RecordsCase{
		Series:           1000,
		SamplesPerSeries: 1,
		TsFn:             tsFn,
	})
	cases := []struct {
		name      string
		appends   int
		initRecs  testwal.Records
		laterRecs testwal.Records
	}{
		{
			name:     "data=samples",
			appends:  500,
			initRecs: initSamplesRecs,
			laterRecs: testwal.Records{
				Samples: initSamplesRecs.Samples,
			},
		},
		{
			name:    "data=all",
			appends: 100,
			initRecs: testwal.GenerateRecords(testwal.RecordsCase{
				Series:                   100,
				SamplesPerSeries:         10,
				HistogramsPerSeries:      1,
				FloatHistogramsPerSeries: 1,
				ExemplarsPerSeries:       1,
				TsFn:                     tsFn,
			}),
			laterRecs: testwal.GenerateRecords(testwal.RecordsCase{
				Series:                   10,
				SamplesPerSeries:         100,
				HistogramsPerSeries:      10,
				FloatHistogramsPerSeries: 10,
				ExemplarsPerSeries:       10,
				TsFn:                     tsFn,
			}),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			var (
				dir  = b.TempDir()
				wdir = path.Join(dir, "wal")
				enc  = record.Encoder{EnableSTStorage: true}
			)
			require.NoError(b, os.Mkdir(wdir, 0o777))

			w, err := NewSize(nil, nil, wdir, DefaultSegmentSize, compression.Snappy)
			require.NoError(b, err)
			b.Cleanup(func() {
				_ = w.Close()
			})

			offsets := make([]int64, 0, tc.appends)
			logBenchRecords(b, w, &enc, tc.initRecs)
			fi, err := w.segment.Stat()
			require.NoError(b, err)
			offsets = append(offsets, fi.Size())

			for range tc.appends - 1 {
				logBenchRecords(b, w, &enc, tc.laterRecs)
				fi, err := w.segment.Stat()
				require.NoError(b, err)
				offsets = append(offsets, fi.Size())
			}
			require.NoError(b, w.Close())

			first, last, err := Segments(w.Dir())
			require.NoError(b, err)
			require.Equal(b, 0, last, "expected a single segment, got %v", last+1)
			segmentPath := SegmentName(w.Dir(), first)

			b.Run("mode=onlySeries", func(b *testing.B) {
				wt := newWriteToMock(0)
				wt.benchMode = true
				watcher := NewWatcher(wMetrics, nil, nil, "test", wt, b.TempDir(), true, true, true, record.NewBuffersPool())
				watcher.SetMetrics()
				watcher.SetStartTime(timestamp.Time(math.MinInt64))

				segment, err := OpenReadSegment(segmentPath)
				require.NoError(b, err)
				b.Cleanup(func() { _ = segment.Close() })

				decBuf := compression.NewSyncDecodeBuffer()
				segReader := NewLiveReader(watcher.logger, watcher.readerMetrics, segment)
				segReader.decBuf = decBuf
				require.ErrorIs(b, watcher.readSegment(segReader, 0, true), io.EOF)

				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					b.StopTimer()
					wt.Reset()
					_, err = segment.SegmentFile.(*os.File).Seek(0, io.SeekStart)
					require.NoError(b, err)
					segReader = NewLiveReader(watcher.logger, watcher.readerMetrics, segment)
					segReader.decBuf = decBuf
					b.StartTimer()

					require.ErrorIs(b, watcher.readSegment(segReader, 0, true), io.EOF)
				}
			})

			b.Run("mode=tail-one-go", func(b *testing.B) {
				wt := newWriteToMock(0)
				wt.benchMode = true
				watcher := NewWatcher(wMetrics, nil, nil, "test", wt, b.TempDir(), true, true, true, record.NewBuffersPool())
				watcher.SetMetrics()
				watcher.SetStartTime(timestamp.Time(math.MinInt64))

				segment, err := OpenReadSegment(segmentPath)
				require.NoError(b, err)
				b.Cleanup(func() { _ = segment.Close() })

				decBuf := compression.NewSyncDecodeBuffer()
				segReader := NewLiveReader(watcher.logger, watcher.readerMetrics, segment)
				segReader.decBuf = decBuf
				require.ErrorIs(b, watcher.readSegment(segReader, 0, false), io.EOF)

				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					b.StopTimer()
					wt.Reset()
					_, err = segment.SegmentFile.(*os.File).Seek(0, io.SeekStart)
					require.NoError(b, err)
					segReader = NewLiveReader(watcher.logger, watcher.readerMetrics, segment)
					segReader.decBuf = decBuf
					b.StartTimer()

					require.ErrorIs(b, watcher.readSegment(segReader, 0, false), io.EOF)
				}
			})

			b.Run("mode=tail-per-scrape", func(b *testing.B) {
				wt := newWriteToMock(0)
				wt.benchMode = true
				watcher := NewWatcher(wMetrics, nil, nil, "test", wt, b.TempDir(), true, true, true, record.NewBuffersPool())
				watcher.SetMetrics()
				watcher.SetStartTime(timestamp.Time(math.MinInt64))

				segment, err := OpenReadSegment(segmentPath)
				require.NoError(b, err)
				b.Cleanup(func() { _ = segment.Close() })

				decBuf := compression.NewSyncDecodeBuffer()
				segReader := NewLiveReader(watcher.logger, watcher.readerMetrics, segment)
				segReader.decBuf = decBuf
				require.ErrorIs(b, watcher.readSegment(segReader, 0, false), io.EOF)

				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					b.StopTimer()
					wt.Reset()
					_, err = segment.SegmentFile.(*os.File).Seek(0, io.SeekStart)
					require.NoError(b, err)
					limReader := &offsetLimitReader{r: segment}
					segReader = NewLiveReader(watcher.logger, watcher.readerMetrics, limReader)
					segReader.decBuf = decBuf
					b.StartTimer()

					for _, off := range offsets {
						limReader.limit = off
						require.ErrorIs(b, watcher.readSegment(segReader, 0, false), io.EOF)
					}
				}
			})
		})
	}
}
