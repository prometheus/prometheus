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
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/testutil"
)

// newTestHeadDefaultOptions returns the HeadOptions that should be used by default in unit tests.
func newTestHeadDefaultOptions(chunkRange int64, oooEnabled bool) *HeadOptions {
	opts := DefaultHeadOptions()
	opts.ChunkRange = chunkRange
	opts.EnableExemplarStorage = true
	opts.MaxExemplars.Store(config.DefaultExemplarsConfig.MaxExemplars)
	opts.EnableNativeHistograms.Store(true)
	if oooEnabled {
		opts.OutOfOrderTimeWindow.Store(10 * time.Minute.Milliseconds())
	}
	return opts
}

func newTestHead(t testing.TB, chunkRange int64, compressWAL wlog.CompressionType, oooEnabled bool) (*Head, *wlog.WL) {
	return newTestHeadWithOptions(t, compressWAL, newTestHeadDefaultOptions(chunkRange, oooEnabled))
}

func newTestHeadWithOptions(t testing.TB, compressWAL wlog.CompressionType, opts *HeadOptions) (*Head, *wlog.WL) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compressWAL)
	require.NoError(t, err)

	// Override the chunks dir with the testing one.
	opts.ChunkDirRoot = dir

	h, err := NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)

	require.NoError(t, h.chunkDiskMapper.IterateAllChunks(func(_ chunks.HeadSeriesRef, _ chunks.ChunkDiskMapperRef, _, _ int64, _ uint16, _ chunkenc.Encoding, _ bool) error {
		return nil
	}))

	return h, wal
}

func BenchmarkCreateSeries(b *testing.B) {
	series := genSeries(b.N, 10, 0, 0)
	h, _ := newTestHead(b, 10000, wlog.CompressionNone, false)
	b.Cleanup(func() {
		require.NoError(b, h.Close())
	})

	b.ReportAllocs()
	b.ResetTimer()

	for _, s := range series {
		h.getOrCreate(s.Labels().Hash(), s.Labels())
	}
}

func BenchmarkHeadAppender_Append_Commit_ExistingSeries(b *testing.B) {
	seriesCounts := []int{100, 1000, 10000}
	series := genSeries(10000, 10, 0, 0)

	for _, seriesCount := range seriesCounts {
		b.Run(fmt.Sprintf("%d series", seriesCount), func(b *testing.B) {
			for _, samplesPerAppend := range []int64{1, 2, 5, 100} {
				b.Run(fmt.Sprintf("%d samples per append", samplesPerAppend), func(b *testing.B) {
					h, _ := newTestHead(b, 10000, wlog.CompressionNone, false)
					b.Cleanup(func() { require.NoError(b, h.Close()) })

					ts := int64(1000)
					appendSamples := func() error {
						var err error
						app := h.Appender(context.Background())
						for _, s := range series[:seriesCount] {
							var ref storage.SeriesRef
							for sampleIndex := int64(0); sampleIndex < samplesPerAppend; sampleIndex++ {
								ref, err = app.Append(ref, s.Labels(), ts+sampleIndex, float64(ts+sampleIndex))
								if err != nil {
									return err
								}
							}
						}
						ts += 1000 // should increment more than highest samplesPerAppend
						return app.Commit()
					}

					// Init series, that's not what we're benchmarking here.
					require.NoError(b, appendSamples())

					b.ReportAllocs()
					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						require.NoError(b, appendSamples())
					}
				})
			}
		})
	}
}

func populateTestWL(t testing.TB, w *wlog.WL, recs []interface{}) {
	var enc record.Encoder
	for _, r := range recs {
		switch v := r.(type) {
		case []record.RefSeries:
			require.NoError(t, w.Log(enc.Series(v, nil)))
		case []record.RefSample:
			require.NoError(t, w.Log(enc.Samples(v, nil)))
		case []tombstones.Stone:
			require.NoError(t, w.Log(enc.Tombstones(v, nil)))
		case []record.RefExemplar:
			require.NoError(t, w.Log(enc.Exemplars(v, nil)))
		case []record.RefMmapMarker:
			require.NoError(t, w.Log(enc.MmapMarkers(v, nil)))
		}
	}
}

func readTestWAL(t testing.TB, dir string) (recs []interface{}) {
	sr, err := wlog.NewSegmentsReader(dir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sr.Close())
	}()

	dec := record.NewDecoder(labels.NewSymbolTable())
	r := wlog.NewReader(sr)

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			recs = append(recs, series)
		case record.Samples:
			samples, err := dec.Samples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.HistogramSamples:
			samples, err := dec.HistogramSamples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.FloatHistogramSamples:
			samples, err := dec.FloatHistogramSamples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.Tombstones:
			tstones, err := dec.Tombstones(rec, nil)
			require.NoError(t, err)
			recs = append(recs, tstones)
		case record.Metadata:
			meta, err := dec.Metadata(rec, nil)
			require.NoError(t, err)
			recs = append(recs, meta)
		case record.Exemplars:
			exemplars, err := dec.Exemplars(rec, nil)
			require.NoError(t, err)
			recs = append(recs, exemplars)
		default:
			require.Fail(t, "unknown record type")
		}
	}
	require.NoError(t, r.Err())
	return recs
}

func BenchmarkLoadWLs(b *testing.B) {
	cases := []struct {
		// Total series is (batches*seriesPerBatch).
		batches          int
		seriesPerBatch   int
		samplesPerSeries int
		mmappedChunkT    int64
		// The first oooSeriesPct*seriesPerBatch series in a batch are selected as "OOO" series.
		oooSeriesPct float64
		// The first oooSamplesPct*samplesPerSeries samples in an OOO series are written as OOO samples.
		oooSamplesPct float64
		oooCapMax     int64
	}{
		{ // Less series and more samples. 2 hour WAL with 1 second scrape interval.
			batches:          10,
			seriesPerBatch:   100,
			samplesPerSeries: 7200,
		},
		{ // More series and less samples.
			batches:          10,
			seriesPerBatch:   10000,
			samplesPerSeries: 50,
		},
		{ // In between.
			batches:          10,
			seriesPerBatch:   1000,
			samplesPerSeries: 480,
		},
		{ // 2 hour WAL with 15 second scrape interval, and mmapped chunks up to last 100 samples.
			batches:          100,
			seriesPerBatch:   1000,
			samplesPerSeries: 480,
			mmappedChunkT:    3800,
		},
		{ // A lot of OOO samples (50% series with 50% of samples being OOO).
			batches:          10,
			seriesPerBatch:   1000,
			samplesPerSeries: 480,
			oooSeriesPct:     0.5,
			oooSamplesPct:    0.5,
			oooCapMax:        DefaultOutOfOrderCapMax,
		},
		{ // Fewer OOO samples (10% of series with 10% of samples being OOO).
			batches:          10,
			seriesPerBatch:   1000,
			samplesPerSeries: 480,
			oooSeriesPct:     0.1,
			oooSamplesPct:    0.1,
		},
		{ // 2 hour WAL with 15 second scrape interval, and mmapped chunks up to last 100 samples.
			// Four mmap markers per OOO series: 480 * 0.3 = 144, 144 / 32 (DefaultOutOfOrderCapMax) = 4.
			batches:          100,
			seriesPerBatch:   1000,
			samplesPerSeries: 480,
			mmappedChunkT:    3800,
			oooSeriesPct:     0.2,
			oooSamplesPct:    0.3,
			oooCapMax:        DefaultOutOfOrderCapMax,
		},
	}

	labelsPerSeries := 5
	// Rough estimates of most common % of samples that have an exemplar for each scrape.
	exemplarsPercentages := []float64{0, 0.5, 1, 5}
	lastExemplarsPerSeries := -1
	for _, c := range cases {
		for _, p := range exemplarsPercentages {
			exemplarsPerSeries := int(math.RoundToEven(float64(c.samplesPerSeries) * p / 100))
			// For tests with low samplesPerSeries we could end up testing with 0 exemplarsPerSeries
			// multiple times without this check.
			if exemplarsPerSeries == lastExemplarsPerSeries {
				continue
			}
			lastExemplarsPerSeries = exemplarsPerSeries
			b.Run(fmt.Sprintf("batches=%d,seriesPerBatch=%d,samplesPerSeries=%d,exemplarsPerSeries=%d,mmappedChunkT=%d,oooSeriesPct=%.3f,oooSamplesPct=%.3f,oooCapMax=%d", c.batches, c.seriesPerBatch, c.samplesPerSeries, exemplarsPerSeries, c.mmappedChunkT, c.oooSeriesPct, c.oooSamplesPct, c.oooCapMax),
				func(b *testing.B) {
					dir := b.TempDir()

					wal, err := wlog.New(nil, nil, dir, wlog.CompressionNone)
					require.NoError(b, err)
					var wbl *wlog.WL
					if c.oooSeriesPct != 0 {
						wbl, err = wlog.New(nil, nil, dir, wlog.CompressionNone)
						require.NoError(b, err)
					}

					// Write series.
					refSeries := make([]record.RefSeries, 0, c.seriesPerBatch)
					for k := 0; k < c.batches; k++ {
						refSeries = refSeries[:0]
						for i := k * c.seriesPerBatch; i < (k+1)*c.seriesPerBatch; i++ {
							lbls := make(map[string]string, labelsPerSeries)
							lbls[defaultLabelName] = strconv.Itoa(i)
							for j := 1; len(lbls) < labelsPerSeries; j++ {
								lbls[defaultLabelName+strconv.Itoa(j)] = defaultLabelValue + strconv.Itoa(j)
							}
							refSeries = append(refSeries, record.RefSeries{Ref: chunks.HeadSeriesRef(i) * 101, Labels: labels.FromMap(lbls)})
						}
						populateTestWL(b, wal, []interface{}{refSeries})
					}

					// Write samples.
					refSamples := make([]record.RefSample, 0, c.seriesPerBatch)

					oooSeriesPerBatch := int(float64(c.seriesPerBatch) * c.oooSeriesPct)
					oooSamplesPerSeries := int(float64(c.samplesPerSeries) * c.oooSamplesPct)

					for i := 0; i < c.samplesPerSeries; i++ {
						for j := 0; j < c.batches; j++ {
							refSamples = refSamples[:0]

							k := j * c.seriesPerBatch
							// Skip appending the first oooSamplesPerSeries samples for the series in the batch that
							// should have OOO samples. OOO samples are appended after all the in-order samples.
							if i < oooSamplesPerSeries {
								k += oooSeriesPerBatch
							}
							for ; k < (j+1)*c.seriesPerBatch; k++ {
								refSamples = append(refSamples, record.RefSample{
									Ref: chunks.HeadSeriesRef(k) * 101,
									T:   int64(i) * 10,
									V:   float64(i) * 100,
								})
							}
							populateTestWL(b, wal, []interface{}{refSamples})
						}
					}

					// Write mmapped chunks.
					if c.mmappedChunkT != 0 {
						chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, mmappedChunksDir(dir), chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
						require.NoError(b, err)
						cOpts := chunkOpts{
							chunkDiskMapper: chunkDiskMapper,
							chunkRange:      c.mmappedChunkT,
							samplesPerChunk: DefaultSamplesPerChunk,
						}
						for k := 0; k < c.batches*c.seriesPerBatch; k++ {
							// Create one mmapped chunk per series, with one sample at the given time.
							s := newMemSeries(labels.Labels{}, chunks.HeadSeriesRef(k)*101, 0, defaultIsolationDisabled)
							s.append(c.mmappedChunkT, 42, 0, cOpts)
							// There's only one head chunk because only a single sample is appended. mmapChunks()
							// ignores the latest chunk, so we need to cut a new head chunk to guarantee the chunk with
							// the sample at c.mmappedChunkT is mmapped.
							s.cutNewHeadChunk(c.mmappedChunkT, chunkenc.EncXOR, c.mmappedChunkT)
							s.mmapChunks(chunkDiskMapper)
						}
						require.NoError(b, chunkDiskMapper.Close())
					}

					// Write exemplars.
					refExemplars := make([]record.RefExemplar, 0, c.seriesPerBatch)
					for i := 0; i < exemplarsPerSeries; i++ {
						for j := 0; j < c.batches; j++ {
							refExemplars = refExemplars[:0]
							for k := j * c.seriesPerBatch; k < (j+1)*c.seriesPerBatch; k++ {
								refExemplars = append(refExemplars, record.RefExemplar{
									Ref:    chunks.HeadSeriesRef(k) * 101,
									T:      int64(i) * 10,
									V:      float64(i) * 100,
									Labels: labels.FromStrings("trace_id", fmt.Sprintf("trace-%d", i)),
								})
							}
							populateTestWL(b, wal, []interface{}{refExemplars})
						}
					}

					// Write OOO samples and mmap markers.
					refMarkers := make([]record.RefMmapMarker, 0, oooSeriesPerBatch)
					refSamples = make([]record.RefSample, 0, oooSeriesPerBatch)
					for i := 0; i < oooSamplesPerSeries; i++ {
						shouldAddMarkers := c.oooCapMax != 0 && i != 0 && int64(i)%c.oooCapMax == 0

						for j := 0; j < c.batches; j++ {
							refSamples = refSamples[:0]
							if shouldAddMarkers {
								refMarkers = refMarkers[:0]
							}
							for k := j * c.seriesPerBatch; k < (j*c.seriesPerBatch)+oooSeriesPerBatch; k++ {
								ref := chunks.HeadSeriesRef(k) * 101
								if shouldAddMarkers {
									// loadWBL() checks that the marker's MmapRef is less than or equal to the ref
									// for the last mmap chunk. Setting MmapRef to 0 to always pass that check.
									refMarkers = append(refMarkers, record.RefMmapMarker{Ref: ref, MmapRef: 0})
								}
								refSamples = append(refSamples, record.RefSample{
									Ref: ref,
									T:   int64(i) * 10,
									V:   float64(i) * 100,
								})
							}
							if shouldAddMarkers {
								populateTestWL(b, wbl, []interface{}{refMarkers})
							}
							populateTestWL(b, wal, []interface{}{refSamples})
							populateTestWL(b, wbl, []interface{}{refSamples})
						}
					}

					b.ResetTimer()

					// Load the WAL.
					for i := 0; i < b.N; i++ {
						opts := DefaultHeadOptions()
						opts.ChunkRange = 1000
						opts.ChunkDirRoot = dir
						if c.oooCapMax > 0 {
							opts.OutOfOrderCapMax.Store(c.oooCapMax)
						}
						h, err := NewHead(nil, nil, wal, wbl, opts, nil)
						require.NoError(b, err)
						h.Init(0)
					}
					b.StopTimer()
					wal.Close()
					if wbl != nil {
						wbl.Close()
					}
				})
		}
	}
}

// TestHead_HighConcurrencyReadAndWrite generates 1000 series with a step of 15s and fills a whole block with samples,
// this means in total it generates 4000 chunks because with a step of 15s there are 4 chunks per block per series.
// While appending the samples to the head it concurrently queries them from multiple go routines and verifies that the
// returned results are correct.
func TestHead_HighConcurrencyReadAndWrite(t *testing.T) {
	head, _ := newTestHead(t, DefaultBlockDuration, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	seriesCnt := 1000
	readConcurrency := 2
	writeConcurrency := 10
	startTs := uint64(DefaultBlockDuration) // start at the second block relative to the unix epoch.
	qryRange := uint64(5 * time.Minute.Milliseconds())
	step := uint64(15 * time.Second / time.Millisecond)
	endTs := startTs + uint64(DefaultBlockDuration)

	labelSets := make([]labels.Labels, seriesCnt)
	for i := 0; i < seriesCnt; i++ {
		labelSets[i] = labels.FromStrings("seriesId", strconv.Itoa(i))
	}

	head.Init(0)

	g, ctx := errgroup.WithContext(context.Background())
	whileNotCanceled := func(f func() (bool, error)) error {
		for ctx.Err() == nil {
			cont, err := f()
			if err != nil {
				return err
			}
			if !cont {
				return nil
			}
		}
		return nil
	}

	// Create one channel for each write worker, the channels will be used by the coordinator
	// go routine to coordinate which timestamps each write worker has to write.
	writerTsCh := make([]chan uint64, writeConcurrency)
	for writerTsChIdx := range writerTsCh {
		writerTsCh[writerTsChIdx] = make(chan uint64)
	}

	// workerReadyWg is used to synchronize the start of the test,
	// we only start the test once all workers signal that they're ready.
	var workerReadyWg sync.WaitGroup
	workerReadyWg.Add(writeConcurrency + readConcurrency)

	// Start the write workers.
	for wid := 0; wid < writeConcurrency; wid++ {
		// Create copy of workerID to be used by worker routine.
		workerID := wid

		g.Go(func() error {
			// The label sets which this worker will write.
			workerLabelSets := labelSets[(seriesCnt/writeConcurrency)*workerID : (seriesCnt/writeConcurrency)*(workerID+1)]

			// Signal that this worker is ready.
			workerReadyWg.Done()

			return whileNotCanceled(func() (bool, error) {
				ts, ok := <-writerTsCh[workerID]
				if !ok {
					return false, nil
				}

				app := head.Appender(ctx)
				for i := 0; i < len(workerLabelSets); i++ {
					// We also use the timestamp as the sample value.
					_, err := app.Append(0, workerLabelSets[i], int64(ts), float64(ts))
					if err != nil {
						return false, fmt.Errorf("Error when appending to head: %w", err)
					}
				}

				return true, app.Commit()
			})
		})
	}

	// queryHead is a helper to query the head for a given time range and labelset.
	queryHead := func(mint, maxt uint64, label labels.Label) (map[string][]chunks.Sample, error) {
		q, err := NewBlockQuerier(head, int64(mint), int64(maxt))
		if err != nil {
			return nil, err
		}
		return query(t, q, labels.MustNewMatcher(labels.MatchEqual, label.Name, label.Value)), nil
	}

	// readerTsCh will be used by the coordinator go routine to coordinate which timestamps the reader should read.
	readerTsCh := make(chan uint64)

	// Start the read workers.
	for wid := 0; wid < readConcurrency; wid++ {
		// Create copy of threadID to be used by worker routine.
		workerID := wid

		g.Go(func() error {
			querySeriesRef := (seriesCnt / readConcurrency) * workerID

			// Signal that this worker is ready.
			workerReadyWg.Done()

			return whileNotCanceled(func() (bool, error) {
				ts, ok := <-readerTsCh
				if !ok {
					return false, nil
				}

				querySeriesRef = (querySeriesRef + 1) % seriesCnt
				lbls := labelSets[querySeriesRef]
				// lbls has a single entry; extract it so we can run a query.
				var lbl labels.Label
				lbls.Range(func(l labels.Label) {
					lbl = l
				})
				samples, err := queryHead(ts-qryRange, ts, lbl)
				if err != nil {
					return false, err
				}

				if len(samples) != 1 {
					return false, fmt.Errorf("expected 1 series, got %d", len(samples))
				}

				series := lbls.String()
				expectSampleCnt := qryRange/step + 1
				if expectSampleCnt != uint64(len(samples[series])) {
					return false, fmt.Errorf("expected %d samples, got %d", expectSampleCnt, len(samples[series]))
				}

				for sampleIdx, sample := range samples[series] {
					expectedValue := ts - qryRange + (uint64(sampleIdx) * step)
					if sample.T() != int64(expectedValue) {
						return false, fmt.Errorf("expected sample %d to have ts %d, got %d", sampleIdx, expectedValue, sample.T())
					}
					if sample.F() != float64(expectedValue) {
						return false, fmt.Errorf("expected sample %d to have value %d, got %f", sampleIdx, expectedValue, sample.F())
					}
				}

				return true, nil
			})
		})
	}

	// Start the coordinator go routine.
	g.Go(func() error {
		currTs := startTs

		defer func() {
			// End of the test, close all channels to stop the workers.
			for _, ch := range writerTsCh {
				close(ch)
			}
			close(readerTsCh)
		}()

		// Wait until all workers are ready to start the test.
		workerReadyWg.Wait()
		return whileNotCanceled(func() (bool, error) {
			// Send the current timestamp to each of the writers.
			for _, ch := range writerTsCh {
				select {
				case ch <- currTs:
				case <-ctx.Done():
					return false, nil
				}
			}

			// Once data for at least <qryRange> has been ingested, send the current timestamp to the readers.
			if currTs > startTs+qryRange {
				select {
				case readerTsCh <- currTs - step:
				case <-ctx.Done():
					return false, nil
				}
			}

			currTs += step
			if currTs > endTs {
				return false, nil
			}

			return true, nil
		})
	})

	require.NoError(t, g.Wait())
}

func TestHead_ReadWAL(t *testing.T) {
	for _, compress := range []wlog.CompressionType{wlog.CompressionNone, wlog.CompressionSnappy, wlog.CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			entries := []interface{}{
				[]record.RefSeries{
					{Ref: 10, Labels: labels.FromStrings("a", "1")},
					{Ref: 11, Labels: labels.FromStrings("a", "2")},
					{Ref: 100, Labels: labels.FromStrings("a", "3")},
				},
				[]record.RefSample{
					{Ref: 0, T: 99, V: 1},
					{Ref: 10, T: 100, V: 2},
					{Ref: 100, T: 100, V: 3},
				},
				[]record.RefSeries{
					{Ref: 50, Labels: labels.FromStrings("a", "4")},
					// This series has two refs pointing to it.
					{Ref: 101, Labels: labels.FromStrings("a", "3")},
				},
				[]record.RefSample{
					{Ref: 10, T: 101, V: 5},
					{Ref: 50, T: 101, V: 6},
					{Ref: 101, T: 101, V: 7},
				},
				[]tombstones.Stone{
					{Ref: 0, Intervals: []tombstones.Interval{{Mint: 99, Maxt: 101}}},
				},
				[]record.RefExemplar{
					{Ref: 10, T: 100, V: 1, Labels: labels.FromStrings("trace_id", "asdf")},
				},
			}

			head, w := newTestHead(t, 1000, compress, false)
			defer func() {
				require.NoError(t, head.Close())
			}()

			populateTestWL(t, w, entries)

			require.NoError(t, head.Init(math.MinInt64))
			require.Equal(t, uint64(101), head.lastSeriesID.Load())

			s10 := head.series.getByID(10)
			s11 := head.series.getByID(11)
			s50 := head.series.getByID(50)
			s100 := head.series.getByID(100)

			testutil.RequireEqual(t, labels.FromStrings("a", "1"), s10.lset)
			require.Nil(t, s11) // Series without samples should be garbage collected at head.Init().
			testutil.RequireEqual(t, labels.FromStrings("a", "4"), s50.lset)
			testutil.RequireEqual(t, labels.FromStrings("a", "3"), s100.lset)

			expandChunk := func(c chunkenc.Iterator) (x []sample) {
				for c.Next() == chunkenc.ValFloat {
					t, v := c.At()
					x = append(x, sample{t: t, f: v})
				}
				require.NoError(t, c.Err())
				return x
			}

			c, _, _, err := s10.chunk(0, head.chunkDiskMapper, &head.memChunkPool)
			require.NoError(t, err)
			require.Equal(t, []sample{{100, 2, nil, nil}, {101, 5, nil, nil}}, expandChunk(c.chunk.Iterator(nil)))
			c, _, _, err = s50.chunk(0, head.chunkDiskMapper, &head.memChunkPool)
			require.NoError(t, err)
			require.Equal(t, []sample{{101, 6, nil, nil}}, expandChunk(c.chunk.Iterator(nil)))
			// The samples before the new series record should be discarded since a duplicate record
			// is only possible when old samples were compacted.
			c, _, _, err = s100.chunk(0, head.chunkDiskMapper, &head.memChunkPool)
			require.NoError(t, err)
			require.Equal(t, []sample{{101, 7, nil, nil}}, expandChunk(c.chunk.Iterator(nil)))

			q, err := head.ExemplarQuerier(context.Background())
			require.NoError(t, err)
			e, err := q.Select(0, 1000, []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "1")})
			require.NoError(t, err)
			require.True(t, exemplar.Exemplar{Ts: 100, Value: 1, Labels: labels.FromStrings("trace_id", "asdf")}.Equals(e[0].Exemplars[0]))
		})
	}
}

func TestHead_WALMultiRef(t *testing.T) {
	head, w := newTestHead(t, 1000, wlog.CompressionNone, false)

	require.NoError(t, head.Init(0))

	app := head.Appender(context.Background())
	ref1, err := app.Append(0, labels.FromStrings("foo", "bar"), 100, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 1500, 2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	require.NoError(t, head.Truncate(1600))

	app = head.Appender(context.Background())
	ref2, err := app.Append(0, labels.FromStrings("foo", "bar"), 1700, 3)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 3.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 2000, 4)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 4.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	require.NotEqual(t, ref1, ref2, "Refs are the same")
	require.NoError(t, head.Close())

	w, err = wlog.New(nil, nil, w.Dir(), wlog.CompressionNone)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = w.Dir()
	head, err = NewHead(nil, nil, w, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))
	defer func() {
		require.NoError(t, head.Close())
	}()

	q, err := NewBlockQuerier(head, 0, 2100)
	require.NoError(t, err)
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	// The samples before the new ref should be discarded since Head truncation
	// happens only after compacting the Head.
	require.Equal(t, map[string][]chunks.Sample{`{foo="bar"}`: {
		sample{1700, 3, nil, nil},
		sample{2000, 4, nil, nil},
	}}, series)
}

func TestHead_ActiveAppenders(t *testing.T) {
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer head.Close()

	require.NoError(t, head.Init(0))

	// First rollback with no samples.
	app := head.Appender(context.Background())
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))
	require.NoError(t, app.Rollback())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))

	// Then commit with no samples.
	app = head.Appender(context.Background())
	require.NoError(t, app.Commit())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))

	// Now rollback with one sample.
	app = head.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 100, 1)
	require.NoError(t, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))
	require.NoError(t, app.Rollback())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))

	// Now commit with one sample.
	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 100, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))
}

func TestHead_UnknownWALRecord(t *testing.T) {
	head, w := newTestHead(t, 1000, wlog.CompressionNone, false)
	w.Log([]byte{255, 42})
	require.NoError(t, head.Init(0))
	require.NoError(t, head.Close())
}

// BenchmarkHead_Truncate is quite heavy, so consider running it with
// -benchtime=10x or similar to get more stable and comparable results.
func BenchmarkHead_Truncate(b *testing.B) {
	const total = 1e6

	prepare := func(b *testing.B, churn int) *Head {
		h, _ := newTestHead(b, 1000, wlog.CompressionNone, false)
		b.Cleanup(func() {
			require.NoError(b, h.Close())
		})

		h.initTime(0)

		internedItoa := map[int]string{}
		var mtx sync.RWMutex
		itoa := func(i int) string {
			mtx.RLock()
			s, ok := internedItoa[i]
			mtx.RUnlock()
			if ok {
				return s
			}
			mtx.Lock()
			s = strconv.Itoa(i)
			internedItoa[i] = s
			mtx.Unlock()
			return s
		}

		allSeries := [total]labels.Labels{}
		nameValues := make([]string, 0, 100)
		for i := 0; i < total; i++ {
			nameValues = nameValues[:0]

			// A thousand labels like lbl_x_of_1000, each with total/1000 values
			thousand := "lbl_" + itoa(i%1000) + "_of_1000"
			nameValues = append(nameValues, thousand, itoa(i/1000))
			// A hundred labels like lbl_x_of_100, each with total/100 values.
			hundred := "lbl_" + itoa(i%100) + "_of_100"
			nameValues = append(nameValues, hundred, itoa(i/100))

			if i%13 == 0 {
				ten := "lbl_" + itoa(i%10) + "_of_10"
				nameValues = append(nameValues, ten, itoa(i%10))
			}

			allSeries[i] = labels.FromStrings(append(nameValues, "first", "a", "second", "a", "third", "a")...)
			s, _, _ := h.getOrCreate(allSeries[i].Hash(), allSeries[i])
			s.mmappedChunks = []*mmappedChunk{
				{minTime: 1000 * int64(i/churn), maxTime: 999 + 1000*int64(i/churn)},
			}
		}

		return h
	}

	for _, churn := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("churn=%d", churn), func(b *testing.B) {
			if b.N > total/churn {
				// Just to make sure that benchmark still makes sense.
				panic("benchmark not prepared")
			}
			h := prepare(b, churn)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				require.NoError(b, h.Truncate(1000*int64(i)))
				// Make sure the benchmark is meaningful and it's actually truncating the expected amount of series.
				require.Equal(b, total-churn*i, int(h.NumSeries()))
			}
		})
	}
}

func TestHead_Truncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	ctx := context.Background()

	s1, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1", "b", "1"))
	s2, _, _ := h.getOrCreate(2, labels.FromStrings("a", "2", "b", "1"))
	s3, _, _ := h.getOrCreate(3, labels.FromStrings("a", "1", "b", "2"))
	s4, _, _ := h.getOrCreate(4, labels.FromStrings("a", "2", "b", "2", "c", "1"))

	s1.mmappedChunks = []*mmappedChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
		{minTime: 2000, maxTime: 2999},
	}
	s2.mmappedChunks = []*mmappedChunk{
		{minTime: 1000, maxTime: 1999},
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
	}
	s3.mmappedChunks = []*mmappedChunk{
		{minTime: 0, maxTime: 999},
		{minTime: 1000, maxTime: 1999},
	}
	s4.mmappedChunks = []*mmappedChunk{}

	// Truncation need not be aligned.
	require.NoError(t, h.Truncate(1))

	require.NoError(t, h.Truncate(2000))

	require.Equal(t, []*mmappedChunk{
		{minTime: 2000, maxTime: 2999},
	}, h.series.getByID(s1.ref).mmappedChunks)

	require.Equal(t, []*mmappedChunk{
		{minTime: 2000, maxTime: 2999},
		{minTime: 3000, maxTime: 3999},
	}, h.series.getByID(s2.ref).mmappedChunks)

	require.Nil(t, h.series.getByID(s3.ref))
	require.Nil(t, h.series.getByID(s4.ref))

	postingsA1, _ := index.ExpandPostings(h.postings.Get("a", "1"))
	postingsA2, _ := index.ExpandPostings(h.postings.Get("a", "2"))
	postingsB1, _ := index.ExpandPostings(h.postings.Get("b", "1"))
	postingsB2, _ := index.ExpandPostings(h.postings.Get("b", "2"))
	postingsC1, _ := index.ExpandPostings(h.postings.Get("c", "1"))
	postingsAll, _ := index.ExpandPostings(h.postings.Get("", ""))

	require.Equal(t, []storage.SeriesRef{storage.SeriesRef(s1.ref)}, postingsA1)
	require.Equal(t, []storage.SeriesRef{storage.SeriesRef(s2.ref)}, postingsA2)
	require.Equal(t, []storage.SeriesRef{storage.SeriesRef(s1.ref), storage.SeriesRef(s2.ref)}, postingsB1)
	require.Equal(t, []storage.SeriesRef{storage.SeriesRef(s1.ref), storage.SeriesRef(s2.ref)}, postingsAll)
	require.Nil(t, postingsB2)
	require.Nil(t, postingsC1)

	iter := h.postings.Symbols()
	symbols := []string{}
	for iter.Next() {
		symbols = append(symbols, iter.At())
	}
	require.Equal(t,
		[]string{"" /* from 'all' postings list */, "1", "2", "a", "b"},
		symbols)

	values := map[string]map[string]struct{}{}
	for _, name := range h.postings.LabelNames() {
		ss, ok := values[name]
		if !ok {
			ss = map[string]struct{}{}
			values[name] = ss
		}
		for _, value := range h.postings.LabelValues(ctx, name) {
			ss[value] = struct{}{}
		}
	}
	require.Equal(t, map[string]map[string]struct{}{
		"a": {"1": struct{}{}, "2": struct{}{}},
		"b": {"1": struct{}{}},
	}, values)
}

// Validate various behaviors brought on by firstChunkID accounting for
// garbage collected chunks.
func TestMemSeries_truncateChunks(t *testing.T) {
	dir := t.TempDir()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()
	cOpts := chunkOpts{
		chunkDiskMapper: chunkDiskMapper,
		chunkRange:      2000,
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	memChunkPool := sync.Pool{
		New: func() interface{} {
			return &memChunk{}
		},
	}

	s := newMemSeries(labels.FromStrings("a", "b"), 1, 0, defaultIsolationDisabled)

	for i := 0; i < 4000; i += 5 {
		ok, _ := s.append(int64(i), float64(i), 0, cOpts)
		require.True(t, ok, "sample append failed")
	}
	s.mmapChunks(chunkDiskMapper)

	// Check that truncate removes half of the chunks and afterwards
	// that the ID of the last chunk still gives us the same chunk afterwards.
	countBefore := len(s.mmappedChunks) + 1 // +1 for the head chunk.
	lastID := s.headChunkID(countBefore - 1)
	lastChunk, _, _, err := s.chunk(lastID, chunkDiskMapper, &memChunkPool)
	require.NoError(t, err)
	require.NotNil(t, lastChunk)

	chk, _, _, err := s.chunk(0, chunkDiskMapper, &memChunkPool)
	require.NotNil(t, chk)
	require.NoError(t, err)

	s.truncateChunksBefore(2000, 0)

	require.Equal(t, int64(2000), s.mmappedChunks[0].minTime)
	_, _, _, err = s.chunk(0, chunkDiskMapper, &memChunkPool)
	require.Equal(t, storage.ErrNotFound, err, "first chunks not gone")
	require.Equal(t, countBefore/2, len(s.mmappedChunks)+1) // +1 for the head chunk.
	chk, _, _, err = s.chunk(lastID, chunkDiskMapper, &memChunkPool)
	require.NoError(t, err)
	require.Equal(t, lastChunk, chk)
}

func TestMemSeries_truncateChunks_scenarios(t *testing.T) {
	const chunkRange = 100
	const chunkStep = 5

	tests := []struct {
		name                 string
		headChunks           int                // the number of head chubks to create on memSeries by appending enough samples
		mmappedChunks        int                // the number of mmapped chunks to create on memSeries by appending enough samples
		truncateBefore       int64              // the mint to pass to truncateChunksBefore()
		expectedTruncated    int                // the number of chunks that we're expecting be truncated and returned by truncateChunksBefore()
		expectedHead         int                // the expected number of head chunks after truncation
		expectedMmap         int                // the expected number of mmapped chunks after truncation
		expectedFirstChunkID chunks.HeadChunkID // the expected series.firstChunkID after truncation
	}{
		{
			name:           "empty memSeries",
			truncateBefore: chunkRange * 10,
		},
		{
			name:         "single head chunk, not truncated",
			headChunks:   1,
			expectedHead: 1,
		},
		{
			name:                 "single head chunk, truncated",
			headChunks:           1,
			truncateBefore:       chunkRange,
			expectedTruncated:    1,
			expectedHead:         0,
			expectedFirstChunkID: 1,
		},
		{
			name:         "2 head chunks, not truncated",
			headChunks:   2,
			expectedHead: 2,
		},
		{
			name:                 "2 head chunks, first truncated",
			headChunks:           2,
			truncateBefore:       chunkRange,
			expectedTruncated:    1,
			expectedHead:         1,
			expectedFirstChunkID: 1,
		},
		{
			name:                 "2 head chunks, everything truncated",
			headChunks:           2,
			truncateBefore:       chunkRange * 2,
			expectedTruncated:    2,
			expectedHead:         0,
			expectedFirstChunkID: 2,
		},
		{
			name:                 "no head chunks, 3 mmap chunks, second mmap truncated",
			headChunks:           0,
			mmappedChunks:        3,
			truncateBefore:       chunkRange * 2,
			expectedTruncated:    2,
			expectedHead:         0,
			expectedMmap:         1,
			expectedFirstChunkID: 2,
		},
		{
			name:          "single head chunk, single mmap chunk, not truncated",
			headChunks:    1,
			mmappedChunks: 1,
			expectedHead:  1,
			expectedMmap:  1,
		},
		{
			name:                 "single head chunk, single mmap chunk, mmap truncated",
			headChunks:           1,
			mmappedChunks:        1,
			truncateBefore:       chunkRange,
			expectedTruncated:    1,
			expectedHead:         1,
			expectedMmap:         0,
			expectedFirstChunkID: 1,
		},
		{
			name:                 "5 head chunk, 5 mmap chunk, third head truncated",
			headChunks:           5,
			mmappedChunks:        5,
			truncateBefore:       chunkRange * 7,
			expectedTruncated:    7,
			expectedHead:         3,
			expectedMmap:         0,
			expectedFirstChunkID: 7,
		},
		{
			name:                 "2 head chunks, 3 mmap chunks, second mmap truncated",
			headChunks:           2,
			mmappedChunks:        3,
			truncateBefore:       chunkRange * 2,
			expectedTruncated:    2,
			expectedHead:         2,
			expectedMmap:         1,
			expectedFirstChunkID: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, chunkDiskMapper.Close())
			}()

			series := newMemSeries(labels.EmptyLabels(), 1, 0, true)

			cOpts := chunkOpts{
				chunkDiskMapper: chunkDiskMapper,
				chunkRange:      chunkRange,
				samplesPerChunk: DefaultSamplesPerChunk,
			}

			var headStart int
			if tc.mmappedChunks > 0 {
				headStart = (tc.mmappedChunks + 1) * chunkRange
				for i := 0; i < (tc.mmappedChunks+1)*chunkRange; i += chunkStep {
					ok, _ := series.append(int64(i), float64(i), 0, cOpts)
					require.True(t, ok, "sample append failed")
				}
				series.mmapChunks(chunkDiskMapper)
			}

			if tc.headChunks == 0 {
				series.headChunks = nil
			} else {
				for i := headStart; i < chunkRange*(tc.mmappedChunks+tc.headChunks); i += chunkStep {
					ok, _ := series.append(int64(i), float64(i), 0, cOpts)
					require.True(t, ok, "sample append failed: %d", i)
				}
			}

			if tc.headChunks > 0 {
				require.NotNil(t, series.headChunks, "head chunk is missing")
				require.Equal(t, tc.headChunks, series.headChunks.len(), "wrong number of head chunks")
			} else {
				require.Nil(t, series.headChunks, "head chunk is present")
			}
			require.Len(t, series.mmappedChunks, tc.mmappedChunks, "wrong number of mmapped chunks")

			truncated := series.truncateChunksBefore(tc.truncateBefore, 0)
			require.Equal(t, tc.expectedTruncated, truncated, "wrong number of truncated chunks returned")

			require.Len(t, series.mmappedChunks, tc.expectedMmap, "wrong number of mmappedChunks after truncation")

			if tc.expectedHead > 0 {
				require.NotNil(t, series.headChunks, "headChunks should is nil after truncation")
				require.Equal(t, tc.expectedHead, series.headChunks.len(), "wrong number of head chunks after truncation")
				require.Nil(t, series.headChunks.oldest().prev, "last head chunk cannot have any next chunk set")
			} else {
				require.Nil(t, series.headChunks, "headChunks should is non-nil after truncation")
			}

			if series.headChunks != nil || len(series.mmappedChunks) > 0 {
				require.GreaterOrEqual(t, series.maxTime(), tc.truncateBefore, "wrong value of series.maxTime() after truncation")
			} else {
				require.Equal(t, int64(math.MinInt64), series.maxTime(), "wrong value of series.maxTime() after truncation")
			}

			require.Equal(t, tc.expectedFirstChunkID, series.firstChunkID, "wrong firstChunkID after truncation")
		})
	}
}

func TestHeadDeleteSeriesWithoutSamples(t *testing.T) {
	for _, compress := range []wlog.CompressionType{wlog.CompressionNone, wlog.CompressionSnappy, wlog.CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			entries := []interface{}{
				[]record.RefSeries{
					{Ref: 10, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{},
				[]record.RefSeries{
					{Ref: 50, Labels: labels.FromStrings("a", "2")},
				},
				[]record.RefSample{
					{Ref: 50, T: 80, V: 1},
					{Ref: 50, T: 90, V: 1},
				},
			}
			head, w := newTestHead(t, 1000, compress, false)
			defer func() {
				require.NoError(t, head.Close())
			}()

			populateTestWL(t, w, entries)

			require.NoError(t, head.Init(math.MinInt64))

			require.NoError(t, head.Delete(context.Background(), 0, 100, labels.MustNewMatcher(labels.MatchEqual, "a", "1")))
		})
	}
}

func TestHeadDeleteSimple(t *testing.T) {
	buildSmpls := func(s []int64) []sample {
		ss := make([]sample, 0, len(s))
		for _, t := range s {
			ss = append(ss, sample{t: t, f: float64(t)})
		}
		return ss
	}
	smplsAll := buildSmpls([]int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	lblDefault := labels.Label{Name: "a", Value: "b"}
	lblsDefault := labels.FromStrings("a", "b")

	cases := []struct {
		dranges    tombstones.Intervals
		addSamples []sample // Samples to add after delete.
		smplsExp   []sample
	}{
		{
			dranges:  tombstones.Intervals{{Mint: 0, Maxt: 3}},
			smplsExp: buildSmpls([]int64{4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}},
			smplsExp: buildSmpls([]int64{0, 4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			smplsExp: buildSmpls([]int64{0, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 700}},
			smplsExp: buildSmpls([]int64{0}),
		},
		{ // This case is to ensure that labels and symbols are deleted.
			dranges:  tombstones.Intervals{{Mint: 0, Maxt: 9}},
			smplsExp: buildSmpls([]int64{}),
		},
		{
			dranges:    tombstones.Intervals{{Mint: 1, Maxt: 3}},
			addSamples: buildSmpls([]int64{11, 13, 15}),
			smplsExp:   buildSmpls([]int64{0, 4, 5, 6, 7, 8, 9, 11, 13, 15}),
		},
		{
			// After delete, the appended samples in the deleted range should be visible
			// as the tombstones are clamped to head min/max time.
			dranges:    tombstones.Intervals{{Mint: 7, Maxt: 20}},
			addSamples: buildSmpls([]int64{11, 13, 15}),
			smplsExp:   buildSmpls([]int64{0, 1, 2, 3, 4, 5, 6, 11, 13, 15}),
		},
	}

	for _, compress := range []wlog.CompressionType{wlog.CompressionNone, wlog.CompressionSnappy, wlog.CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			for _, c := range cases {
				head, w := newTestHead(t, 1000, compress, false)
				require.NoError(t, head.Init(0))

				app := head.Appender(context.Background())
				for _, smpl := range smplsAll {
					_, err := app.Append(0, lblsDefault, smpl.t, smpl.f)
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())

				// Delete the ranges.
				for _, r := range c.dranges {
					require.NoError(t, head.Delete(context.Background(), r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value)))
				}

				// Add more samples.
				app = head.Appender(context.Background())
				for _, smpl := range c.addSamples {
					_, err := app.Append(0, lblsDefault, smpl.t, smpl.f)
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())

				// Compare the samples for both heads - before and after the reloadBlocks.
				reloadedW, err := wlog.New(nil, nil, w.Dir(), compress) // Use a new wal to ensure deleted samples are gone even after a reloadBlocks.
				require.NoError(t, err)
				opts := DefaultHeadOptions()
				opts.ChunkRange = 1000
				opts.ChunkDirRoot = reloadedW.Dir()
				reloadedHead, err := NewHead(nil, nil, reloadedW, nil, opts, nil)
				require.NoError(t, err)
				require.NoError(t, reloadedHead.Init(0))

				// Compare the query results for both heads - before and after the reloadBlocks.
			Outer:
				for _, h := range []*Head{head, reloadedHead} {
					q, err := NewBlockQuerier(h, h.MinTime(), h.MaxTime())
					require.NoError(t, err)
					actSeriesSet := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value))
					require.NoError(t, q.Close())
					expSeriesSet := newMockSeriesSet([]storage.Series{
						storage.NewListSeries(lblsDefault, func() []chunks.Sample {
							ss := make([]chunks.Sample, 0, len(c.smplsExp))
							for _, s := range c.smplsExp {
								ss = append(ss, s)
							}
							return ss
						}(),
						),
					})

					for {
						eok, rok := expSeriesSet.Next(), actSeriesSet.Next()
						require.Equal(t, eok, rok)

						if !eok {
							require.NoError(t, h.Close())
							require.NoError(t, actSeriesSet.Err())
							require.Empty(t, actSeriesSet.Warnings())
							continue Outer
						}
						expSeries := expSeriesSet.At()
						actSeries := actSeriesSet.At()

						require.Equal(t, expSeries.Labels(), actSeries.Labels())

						smplExp, errExp := storage.ExpandSamples(expSeries.Iterator(nil), nil)
						smplRes, errRes := storage.ExpandSamples(actSeries.Iterator(nil), nil)

						require.Equal(t, errExp, errRes)
						require.Equal(t, smplExp, smplRes)
					}
				}
			}
		})
	}
}

func TestDeleteUntilCurMax(t *testing.T) {
	hb, _ := newTestHead(t, 1000000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	numSamples := int64(10)
	app := hb.Appender(context.Background())
	smpls := make([]float64, numSamples)
	for i := int64(0); i < numSamples; i++ {
		smpls[i] = rand.Float64()
		_, err := app.Append(0, labels.FromStrings("a", "b"), i, smpls[i])
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.NoError(t, hb.Delete(context.Background(), 0, 10000, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))

	// Test the series returns no samples. The series is cleared only after compaction.
	q, err := NewBlockQuerier(hb, 0, 100000)
	require.NoError(t, err)
	res := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, res.Next(), "series is not present")
	s := res.At()
	it := s.Iterator(nil)
	require.Equal(t, chunkenc.ValNone, it.Next(), "expected no samples")
	for res.Next() {
	}
	require.NoError(t, res.Err())
	require.Empty(t, res.Warnings())

	// Add again and test for presence.
	app = hb.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("a", "b"), 11, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	q, err = NewBlockQuerier(hb, 0, 100000)
	require.NoError(t, err)
	res = q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, res.Next(), "series don't exist")
	exps := res.At()
	it = exps.Iterator(nil)
	resSamples, err := storage.ExpandSamples(it, newSample)
	require.NoError(t, err)
	require.Equal(t, []chunks.Sample{sample{11, 1, nil, nil}}, resSamples)
	for res.Next() {
	}
	require.NoError(t, res.Err())
	require.Empty(t, res.Warnings())
}

func TestDeletedSamplesAndSeriesStillInWALAfterCheckpoint(t *testing.T) {
	numSamples := 10000

	// Enough samples to cause a checkpoint.
	hb, w := newTestHead(t, int64(numSamples)*10, wlog.CompressionNone, false)

	for i := 0; i < numSamples; i++ {
		app := hb.Appender(context.Background())
		_, err := app.Append(0, labels.FromStrings("a", "b"), int64(i), 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	require.NoError(t, hb.Delete(context.Background(), 0, int64(numSamples), labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
	require.NoError(t, hb.Truncate(1))
	require.NoError(t, hb.Close())

	// Confirm there's been a checkpoint.
	cdir, _, err := wlog.LastCheckpoint(w.Dir())
	require.NoError(t, err)
	// Read in checkpoint and WAL.
	recs := readTestWAL(t, cdir)
	recs = append(recs, readTestWAL(t, w.Dir())...)

	var series, samples, stones, metadata int
	for _, rec := range recs {
		switch rec.(type) {
		case []record.RefSeries:
			series++
		case []record.RefSample:
			samples++
		case []tombstones.Stone:
			stones++
		case []record.RefMetadata:
			metadata++
		default:
			require.Fail(t, "unknown record type")
		}
	}
	require.Equal(t, 1, series)
	require.Equal(t, 9999, samples)
	require.Equal(t, 1, stones)
	require.Equal(t, 0, metadata)
}

func TestDelete_e2e(t *testing.T) {
	numDatapoints := 1000
	numRanges := 1000
	timeInterval := int64(2)
	// Create 8 series with 1000 data-points of different ranges, delete and run queries.
	lbls := [][]labels.Label{
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
	}
	seriesMap := map[string][]chunks.Sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []chunks.Sample{}
	}

	hb, _ := newTestHead(t, 100000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.Appender(context.Background())
	for _, l := range lbls {
		ls := labels.New(l...)
		series := []chunks.Sample{}
		ts := rand.Int63n(300)
		for i := 0; i < numDatapoints; i++ {
			v := rand.Float64()
			_, err := app.Append(0, ls, ts, v)
			require.NoError(t, err)
			series = append(series, sample{ts, v, nil, nil})
			ts += rand.Int63n(timeInterval) + 1
		}
		seriesMap[labels.New(l...).String()] = series
	}
	require.NoError(t, app.Commit())
	// Delete a time-range from each-selector.
	dels := []struct {
		ms     []*labels.Matcher
		drange tombstones.Intervals
	}{
		{
			ms:     []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "b")},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 500}, {Mint: 600, Maxt: 670}},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prom-k8s"),
			},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 500}, {Mint: 100, Maxt: 670}},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "c"),
				labels.MustNewMatcher(labels.MatchEqual, "instance", "localhost:9090"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 400}, {Mint: 100, Maxt: 6700}},
		},
		// TODO: Add Regexp Matchers.
	}
	for _, del := range dels {
		for _, r := range del.drange {
			require.NoError(t, hb.Delete(context.Background(), r.Mint, r.Maxt, del.ms...))
		}
		matched := labels.Slice{}
		for _, l := range lbls {
			s := labels.Selector(del.ms)
			ls := labels.New(l...)
			if s.Matches(ls) {
				matched = append(matched, ls)
			}
		}
		sort.Sort(matched)
		for i := 0; i < numRanges; i++ {
			q, err := NewBlockQuerier(hb, 0, 100000)
			require.NoError(t, err)
			defer q.Close()
			ss := q.Select(context.Background(), true, nil, del.ms...)
			// Build the mockSeriesSet.
			matchedSeries := make([]storage.Series, 0, len(matched))
			for _, m := range matched {
				smpls := seriesMap[m.String()]
				smpls = deletedSamples(smpls, del.drange)
				// Only append those series for which samples exist as mockSeriesSet
				// doesn't skip series with no samples.
				// TODO: But sometimes SeriesSet returns an empty chunkenc.Iterator
				if len(smpls) > 0 {
					matchedSeries = append(matchedSeries, storage.NewListSeries(m, smpls))
				}
			}
			expSs := newMockSeriesSet(matchedSeries)
			// Compare both SeriesSets.
			for {
				eok, rok := expSs.Next(), ss.Next()
				// Skip a series if iterator is empty.
				if rok {
					for ss.At().Iterator(nil).Next() == chunkenc.ValNone {
						rok = ss.Next()
						if !rok {
							break
						}
					}
				}
				require.Equal(t, eok, rok)
				if !eok {
					break
				}
				sexp := expSs.At()
				sres := ss.At()
				require.Equal(t, sexp.Labels(), sres.Labels())
				smplExp, errExp := storage.ExpandSamples(sexp.Iterator(nil), nil)
				smplRes, errRes := storage.ExpandSamples(sres.Iterator(nil), nil)
				require.Equal(t, errExp, errRes)
				require.Equal(t, smplExp, smplRes)
			}
			require.NoError(t, ss.Err())
			require.Empty(t, ss.Warnings())
		}
	}
}

func boundedSamples(full []chunks.Sample, mint, maxt int64) []chunks.Sample {
	for len(full) > 0 {
		if full[0].T() >= mint {
			break
		}
		full = full[1:]
	}
	for i, s := range full {
		// labels.Labelinate on the first sample larger than maxt.
		if s.T() > maxt {
			return full[:i]
		}
	}
	// maxt is after highest sample.
	return full
}

func deletedSamples(full []chunks.Sample, dranges tombstones.Intervals) []chunks.Sample {
	ds := make([]chunks.Sample, 0, len(full))
Outer:
	for _, s := range full {
		for _, r := range dranges {
			if r.InBounds(s.T()) {
				continue Outer
			}
		}
		ds = append(ds, s)
	}

	return ds
}

func TestComputeChunkEndTime(t *testing.T) {
	cases := map[string]struct {
		start, cur, max int64
		ratioToFull     float64
		res             int64
	}{
		"exactly 1/4 full, even increment": {
			start:       0,
			cur:         250,
			max:         1000,
			ratioToFull: 4,
			res:         1000,
		},
		"exactly 1/4 full, uneven increment": {
			start:       100,
			cur:         200,
			max:         1000,
			ratioToFull: 4,
			res:         550,
		},
		"decimal ratio to full": {
			start:       5000,
			cur:         5110,
			max:         10000,
			ratioToFull: 4.2,
			res:         5500,
		},
		// Case where we fit floored 0 chunks. Must catch division by 0
		// and default to maximum time.
		"fit floored 0 chunks": {
			start:       0,
			cur:         500,
			max:         1000,
			ratioToFull: 4,
			res:         1000,
		},
		// Catch division by zero for cur == start. Strictly not a possible case.
		"cur == start": {
			start:       100,
			cur:         100,
			max:         1000,
			ratioToFull: 4,
			res:         104,
		},
	}

	for testName, tc := range cases {
		t.Run(testName, func(t *testing.T) {
			got := computeChunkEndTime(tc.start, tc.cur, tc.max, tc.ratioToFull)
			require.Equal(t, tc.res, got, "(start: %d, cur: %d, max: %d)", tc.start, tc.cur, tc.max)
		})
	}
}

func TestMemSeries_append(t *testing.T) {
	dir := t.TempDir()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()
	cOpts := chunkOpts{
		chunkDiskMapper: chunkDiskMapper,
		chunkRange:      500,
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled)

	// Add first two samples at the very end of a chunk range and the next two
	// on and after it.
	// New chunk must correctly be cut at 1000.
	ok, chunkCreated := s.append(998, 1, 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.append(999, 2, 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")
	s.mmapChunks(chunkDiskMapper)

	ok, chunkCreated = s.append(1000, 3, 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.append(1001, 4, 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")

	s.mmapChunks(chunkDiskMapper)
	require.Len(t, s.mmappedChunks, 1, "there should be only 1 mmapped chunk")
	require.Equal(t, int64(998), s.mmappedChunks[0].minTime, "wrong chunk range")
	require.Equal(t, int64(999), s.mmappedChunks[0].maxTime, "wrong chunk range")
	require.Equal(t, int64(1000), s.headChunks.minTime, "wrong chunk range")
	require.Equal(t, int64(1001), s.headChunks.maxTime, "wrong chunk range")

	// Fill the range [1000,2000) with many samples. Intermediate chunks should be cut
	// at approximately 120 samples per chunk.
	for i := 1; i < 1000; i++ {
		ok, _ := s.append(1001+int64(i), float64(i), 0, cOpts)
		require.True(t, ok, "append failed")
	}
	s.mmapChunks(chunkDiskMapper)

	require.Greater(t, len(s.mmappedChunks)+1, 7, "expected intermediate chunks")

	// All chunks but the first and last should now be moderately full.
	for i, c := range s.mmappedChunks[1:] {
		chk, err := chunkDiskMapper.Chunk(c.ref)
		require.NoError(t, err)
		require.Greater(t, chk.NumSamples(), 100, "unexpected small chunk %d of length %d", i, chk.NumSamples())
	}
}

func TestMemSeries_appendHistogram(t *testing.T) {
	dir := t.TempDir()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()
	cOpts := chunkOpts{
		chunkDiskMapper: chunkDiskMapper,
		chunkRange:      int64(1000),
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled)

	histograms := tsdbutil.GenerateTestHistograms(4)
	histogramWithOneMoreBucket := histograms[3].Copy()
	histogramWithOneMoreBucket.Count++
	histogramWithOneMoreBucket.Sum += 1.23
	histogramWithOneMoreBucket.PositiveSpans[1].Length = 3
	histogramWithOneMoreBucket.PositiveBuckets = append(histogramWithOneMoreBucket.PositiveBuckets, 1)

	// Add first two samples at the very end of a chunk range and the next two
	// on and after it.
	// New chunk must correctly be cut at 1000.
	ok, chunkCreated := s.appendHistogram(998, histograms[0], 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.appendHistogram(999, histograms[1], 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")

	ok, chunkCreated = s.appendHistogram(1000, histograms[2], 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.appendHistogram(1001, histograms[3], 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")

	s.mmapChunks(chunkDiskMapper)
	require.Len(t, s.mmappedChunks, 1, "there should be only 1 mmapped chunk")
	require.Equal(t, int64(998), s.mmappedChunks[0].minTime, "wrong chunk range")
	require.Equal(t, int64(999), s.mmappedChunks[0].maxTime, "wrong chunk range")
	require.Equal(t, int64(1000), s.headChunks.minTime, "wrong chunk range")
	require.Equal(t, int64(1001), s.headChunks.maxTime, "wrong chunk range")

	ok, chunkCreated = s.appendHistogram(1002, histogramWithOneMoreBucket, 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "third sample should trigger a re-encoded chunk")

	s.mmapChunks(chunkDiskMapper)
	require.Len(t, s.mmappedChunks, 1, "there should be only 1 mmapped chunk")
	require.Equal(t, int64(998), s.mmappedChunks[0].minTime, "wrong chunk range")
	require.Equal(t, int64(999), s.mmappedChunks[0].maxTime, "wrong chunk range")
	require.Equal(t, int64(1000), s.headChunks.minTime, "wrong chunk range")
	require.Equal(t, int64(1002), s.headChunks.maxTime, "wrong chunk range")
}

func TestMemSeries_append_atVariableRate(t *testing.T) {
	const samplesPerChunk = 120
	dir := t.TempDir()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, chunkDiskMapper.Close())
	})
	cOpts := chunkOpts{
		chunkDiskMapper: chunkDiskMapper,
		chunkRange:      DefaultBlockDuration,
		samplesPerChunk: samplesPerChunk,
	}

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled)

	// At this slow rate, we will fill the chunk in two block durations.
	slowRate := (DefaultBlockDuration * 2) / samplesPerChunk

	var nextTs int64
	var totalAppendedSamples int
	for i := 0; i < samplesPerChunk/4; i++ {
		ok, _ := s.append(nextTs, float64(i), 0, cOpts)
		require.Truef(t, ok, "slow sample %d was not appended", i)
		nextTs += slowRate
		totalAppendedSamples++
	}
	require.Equal(t, DefaultBlockDuration, s.nextAt, "after appending a samplesPerChunk/4 samples at a slow rate, we should aim to cut a new block at the default block duration %d, but it's set to %d", DefaultBlockDuration, s.nextAt)

	// Suddenly, the rate increases and we receive a sample every millisecond.
	for i := 0; i < math.MaxUint16; i++ {
		ok, _ := s.append(nextTs, float64(i), 0, cOpts)
		require.Truef(t, ok, "quick sample %d was not appended", i)
		nextTs++
		totalAppendedSamples++
	}
	ok, chunkCreated := s.append(DefaultBlockDuration, float64(0), 0, cOpts)
	require.True(t, ok, "new chunk sample was not appended")
	require.True(t, chunkCreated, "sample at block duration timestamp should create a new chunk")

	s.mmapChunks(chunkDiskMapper)
	var totalSamplesInChunks int
	for i, c := range s.mmappedChunks {
		totalSamplesInChunks += int(c.numSamples)
		require.LessOrEqualf(t, c.numSamples, uint16(2*samplesPerChunk), "mmapped chunk %d has more than %d samples", i, 2*samplesPerChunk)
	}
	require.Equal(t, totalAppendedSamples, totalSamplesInChunks, "wrong number of samples in %d mmapped chunks", len(s.mmappedChunks))
}

func TestGCChunkAccess(t *testing.T) {
	// Put a chunk, select it. GC it and then access it.
	const chunkRange = 1000
	h, _ := newTestHead(t, chunkRange, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	cOpts := chunkOpts{
		chunkDiskMapper: h.chunkDiskMapper,
		chunkRange:      chunkRange,
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	idx := h.indexRange(0, 1500)
	var (
		chunks  []chunks.Meta
		builder labels.ScratchBuilder
	)
	require.NoError(t, idx.Series(1, &builder, &chunks))

	require.Equal(t, labels.FromStrings("a", "1"), builder.Labels())
	require.Len(t, chunks, 2)

	cr, err := h.chunksRange(0, 1500, nil)
	require.NoError(t, err)
	_, _, err = cr.ChunkOrIterable(chunks[0])
	require.NoError(t, err)
	_, _, err = cr.ChunkOrIterable(chunks[1])
	require.NoError(t, err)

	require.NoError(t, h.Truncate(1500)) // Remove a chunk.

	_, _, err = cr.ChunkOrIterable(chunks[0])
	require.Equal(t, storage.ErrNotFound, err)
	_, _, err = cr.ChunkOrIterable(chunks[1])
	require.NoError(t, err)
}

func TestGCSeriesAccess(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	const chunkRange = 1000
	h, _ := newTestHead(t, chunkRange, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	cOpts := chunkOpts{
		chunkDiskMapper: h.chunkDiskMapper,
		chunkRange:      chunkRange,
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(999, 999, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(1000, 1000, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(1999, 1999, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	idx := h.indexRange(0, 2000)
	var (
		chunks  []chunks.Meta
		builder labels.ScratchBuilder
	)
	require.NoError(t, idx.Series(1, &builder, &chunks))

	require.Equal(t, labels.FromStrings("a", "1"), builder.Labels())
	require.Len(t, chunks, 2)

	cr, err := h.chunksRange(0, 2000, nil)
	require.NoError(t, err)
	_, _, err = cr.ChunkOrIterable(chunks[0])
	require.NoError(t, err)
	_, _, err = cr.ChunkOrIterable(chunks[1])
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000)) // Remove the series.

	require.Equal(t, (*memSeries)(nil), h.series.getByID(1))

	_, _, err = cr.ChunkOrIterable(chunks[0])
	require.Equal(t, storage.ErrNotFound, err)
	_, _, err = cr.ChunkOrIterable(chunks[1])
	require.Equal(t, storage.ErrNotFound, err)
}

func TestUncommittedSamplesNotLostOnTruncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err := app.Append(0, lset, 2100, 1)
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000))
	require.NotNil(t, h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	require.NoError(t, app.Commit())

	q, err := NewBlockQuerier(h, 1500, 2500)
	require.NoError(t, err)
	defer q.Close()

	ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	require.True(t, ss.Next())
	for ss.Next() {
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())
}

func TestRemoveSeriesAfterRollbackAndTruncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	app := h.appender()
	lset := labels.FromStrings("a", "1")
	_, err := app.Append(0, lset, 2100, 1)
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000))
	require.NotNil(t, h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	require.NoError(t, app.Rollback())

	q, err := NewBlockQuerier(h, 1500, 2500)
	require.NoError(t, err)

	ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	require.False(t, ss.Next())
	require.Empty(t, ss.Warnings())
	require.NoError(t, q.Close())

	// Truncate again, this time the series should be deleted
	require.NoError(t, h.Truncate(2050))
	require.Equal(t, (*memSeries)(nil), h.series.getByHash(lset.Hash(), lset))
}

func TestHead_LogRollback(t *testing.T) {
	for _, compress := range []wlog.CompressionType{wlog.CompressionNone, wlog.CompressionSnappy, wlog.CompressionZstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			h, w := newTestHead(t, 1000, compress, false)
			defer func() {
				require.NoError(t, h.Close())
			}()

			app := h.Appender(context.Background())
			_, err := app.Append(0, labels.FromStrings("a", "b"), 1, 2)
			require.NoError(t, err)

			require.NoError(t, app.Rollback())
			recs := readTestWAL(t, w.Dir())

			require.Len(t, recs, 1)

			series, ok := recs[0].([]record.RefSeries)
			require.True(t, ok, "expected series record but got %+v", recs[0])
			require.Equal(t, []record.RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, series)
		})
	}
}

// TestWalRepair_DecodingError ensures that a repair is run for an error
// when decoding a record.
func TestWalRepair_DecodingError(t *testing.T) {
	var enc record.Encoder
	for name, test := range map[string]struct {
		corrFunc  func(rec []byte) []byte // Func that applies the corruption to a record.
		rec       []byte
		totalRecs int
		expRecs   int
	}{
		"decode_series": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Series([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, []byte{}),
			9,
			5,
		},
		"decode_samples": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Samples([]record.RefSample{{Ref: 0, T: 99, V: 1}}, []byte{}),
			9,
			5,
		},
		"decode_tombstone": {
			func(rec []byte) []byte {
				return rec[:3]
			},
			enc.Tombstones([]tombstones.Stone{{Ref: 1, Intervals: tombstones.Intervals{}}}, []byte{}),
			9,
			5,
		},
	} {
		for _, compress := range []wlog.CompressionType{wlog.CompressionNone, wlog.CompressionSnappy, wlog.CompressionZstd} {
			t.Run(fmt.Sprintf("%s,compress=%s", name, compress), func(t *testing.T) {
				dir := t.TempDir()

				// Fill the wal and corrupt it.
				{
					w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compress)
					require.NoError(t, err)

					for i := 1; i <= test.totalRecs; i++ {
						// At this point insert a corrupted record.
						if i-1 == test.expRecs {
							require.NoError(t, w.Log(test.corrFunc(test.rec)))
							continue
						}
						require.NoError(t, w.Log(test.rec))
					}

					opts := DefaultHeadOptions()
					opts.ChunkRange = 1
					opts.ChunkDirRoot = w.Dir()
					h, err := NewHead(nil, nil, w, nil, opts, nil)
					require.NoError(t, err)
					require.Equal(t, 0.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))
					initErr := h.Init(math.MinInt64)

					var cerr *wlog.CorruptionErr
					require.ErrorAs(t, initErr, &cerr, "reading the wal didn't return corruption error")
					require.NoError(t, h.Close()) // Head will close the wal as well.
				}

				// Open the db to trigger a repair.
				{
					db, err := Open(dir, nil, nil, DefaultOptions(), nil)
					require.NoError(t, err)
					defer func() {
						require.NoError(t, db.Close())
					}()
					require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
				}

				// Read the wal content after the repair.
				{
					sr, err := wlog.NewSegmentsReader(filepath.Join(dir, "wal"))
					require.NoError(t, err)
					defer sr.Close()
					r := wlog.NewReader(sr)

					var actRec int
					for r.Next() {
						actRec++
					}
					require.NoError(t, r.Err())
					require.Equal(t, test.expRecs, actRec, "Wrong number of intact records")
				}
			})
		}
	}
}

// TestWblRepair_DecodingError ensures that a repair is run for an error
// when decoding a record.
func TestWblRepair_DecodingError(t *testing.T) {
	var enc record.Encoder
	corrFunc := func(rec []byte) []byte {
		return rec[:3]
	}
	rec := enc.Samples([]record.RefSample{{Ref: 0, T: 99, V: 1}}, []byte{})
	totalRecs := 9
	expRecs := 5
	dir := t.TempDir()

	// Fill the wbl and corrupt it.
	{
		wal, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), wlog.CompressionNone)
		require.NoError(t, err)
		wbl, err := wlog.New(nil, nil, filepath.Join(dir, "wbl"), wlog.CompressionNone)
		require.NoError(t, err)

		for i := 1; i <= totalRecs; i++ {
			// At this point insert a corrupted record.
			if i-1 == expRecs {
				require.NoError(t, wbl.Log(corrFunc(rec)))
				continue
			}
			require.NoError(t, wbl.Log(rec))
		}

		opts := DefaultHeadOptions()
		opts.ChunkRange = 1
		opts.ChunkDirRoot = wal.Dir()
		opts.OutOfOrderCapMax.Store(30)
		opts.OutOfOrderTimeWindow.Store(1000 * time.Minute.Milliseconds())
		h, err := NewHead(nil, nil, wal, wbl, opts, nil)
		require.NoError(t, err)
		require.Equal(t, 0.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))
		initErr := h.Init(math.MinInt64)

		var elb *errLoadWbl
		require.ErrorAs(t, initErr, &elb) // Wbl errors are wrapped into errLoadWbl, make sure we can unwrap it.

		var cerr *wlog.CorruptionErr
		require.ErrorAs(t, initErr, &cerr, "reading the wal didn't return corruption error")
		require.NoError(t, h.Close()) // Head will close the wal as well.
	}

	// Open the db to trigger a repair.
	{
		db, err := Open(dir, nil, nil, DefaultOptions(), nil)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, db.Close())
		}()
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.walCorruptionsTotal))
	}

	// Read the wbl content after the repair.
	{
		sr, err := wlog.NewSegmentsReader(filepath.Join(dir, "wbl"))
		require.NoError(t, err)
		defer sr.Close()
		r := wlog.NewReader(sr)

		var actRec int
		for r.Next() {
			actRec++
		}
		require.NoError(t, r.Err())
		require.Equal(t, expRecs, actRec, "Wrong number of intact records")
	}
}

func TestHeadReadWriterRepair(t *testing.T) {
	dir := t.TempDir()

	const chunkRange = 1000

	walDir := filepath.Join(dir, "wal")
	// Fill the chunk segments and corrupt it.
	{
		w, err := wlog.New(nil, nil, walDir, wlog.CompressionNone)
		require.NoError(t, err)

		opts := DefaultHeadOptions()
		opts.ChunkRange = chunkRange
		opts.ChunkDirRoot = dir
		opts.ChunkWriteQueueSize = 1 // We need to set this option so that we use the async queue. Upstream prometheus uses the queue directly.
		h, err := NewHead(nil, nil, w, nil, opts, nil)
		require.NoError(t, err)
		require.Equal(t, 0.0, prom_testutil.ToFloat64(h.metrics.mmapChunkCorruptionTotal))
		require.NoError(t, h.Init(math.MinInt64))

		cOpts := chunkOpts{
			chunkDiskMapper: h.chunkDiskMapper,
			chunkRange:      chunkRange,
			samplesPerChunk: DefaultSamplesPerChunk,
		}

		s, created, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))
		require.True(t, created, "series was not created")

		for i := 0; i < 7; i++ {
			ok, chunkCreated := s.append(int64(i*chunkRange), float64(i*chunkRange), 0, cOpts)
			require.True(t, ok, "series append failed")
			require.True(t, chunkCreated, "chunk was not created")
			ok, chunkCreated = s.append(int64(i*chunkRange)+chunkRange-1, float64(i*chunkRange), 0, cOpts)
			require.True(t, ok, "series append failed")
			require.False(t, chunkCreated, "chunk was created")
			h.chunkDiskMapper.CutNewFile()
			s.mmapChunks(h.chunkDiskMapper)
		}
		require.NoError(t, h.Close())

		// Verify that there are 6 segment files.
		// It should only be 6 because the last call to .CutNewFile() won't
		// take effect without another chunk being written.
		files, err := os.ReadDir(mmappedChunksDir(dir))
		require.NoError(t, err)
		require.Len(t, files, 6)

		// Corrupt the 4th file by writing a random byte to series ref.
		f, err := os.OpenFile(filepath.Join(mmappedChunksDir(dir), files[3].Name()), os.O_WRONLY, 0o666)
		require.NoError(t, err)
		n, err := f.WriteAt([]byte{67, 88}, chunks.HeadChunkFileHeaderSize+2)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.NoError(t, f.Close())
	}

	// Open the db to trigger a repair.
	{
		db, err := Open(dir, nil, nil, DefaultOptions(), nil)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, db.Close())
		}()
		require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.mmapChunkCorruptionTotal))
	}

	// Verify that there are 3 segment files after the repair.
	// The segments from the corrupt segment should be removed.
	{
		files, err := os.ReadDir(mmappedChunksDir(dir))
		require.NoError(t, err)
		require.Len(t, files, 3)
	}
}

func TestNewWalSegmentOnTruncate(t *testing.T) {
	h, wal := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()
	add := func(ts int64) {
		app := h.Appender(context.Background())
		_, err := app.Append(0, labels.FromStrings("a", "b"), ts, 0)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	add(0)
	_, last, err := wlog.Segments(wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 0, last)

	add(1)
	require.NoError(t, h.Truncate(1))
	_, last, err = wlog.Segments(wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 1, last)

	add(2)
	require.NoError(t, h.Truncate(2))
	_, last, err = wlog.Segments(wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 2, last)
}

func TestAddDuplicateLabelName(t *testing.T) {
	h, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	add := func(labels labels.Labels, labelName string) {
		app := h.Appender(context.Background())
		_, err := app.Append(0, labels, 0, 0)
		require.Error(t, err)
		require.Equal(t, fmt.Sprintf(`label name "%s" is not unique: invalid sample`, labelName), err.Error())
	}

	add(labels.FromStrings("a", "c", "a", "b"), "a")
	add(labels.FromStrings("a", "c", "a", "c"), "a")
	add(labels.FromStrings("__name__", "up", "job", "prometheus", "le", "500", "le", "400", "unit", "s"), "le")
}

func TestMemSeriesIsolation(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	// Put a series, select it. GC it and then access it.
	lastValue := func(h *Head, maxAppendID uint64) int {
		idx, err := h.Index()

		require.NoError(t, err)

		iso := h.iso.State(math.MinInt64, math.MaxInt64)
		iso.maxAppendID = maxAppendID

		chunks, err := h.chunksRange(math.MinInt64, math.MaxInt64, iso)
		require.NoError(t, err)
		// Hm.. here direct block chunk querier might be required?
		querier := blockQuerier{
			blockBaseQuerier: &blockBaseQuerier{
				index:      idx,
				chunks:     chunks,
				tombstones: tombstones.NewMemTombstones(),

				mint: 0,
				maxt: 10000,
			},
		}

		require.NoError(t, err)
		defer querier.Close()

		ss := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		_, seriesSet, ws, err := expandSeriesSet(ss)
		require.NoError(t, err)
		require.Empty(t, ws)

		for _, series := range seriesSet {
			return int(series[len(series)-1].f)
		}
		return -1
	}

	addSamples := func(h *Head) int {
		i := 1
		for ; i <= 1000; i++ {
			var app storage.Appender
			// To initialize bounds.
			if h.MinTime() == math.MaxInt64 {
				app = &initAppender{head: h}
			} else {
				a := h.appender()
				a.cleanupAppendIDsBelow = 0
				app = a
			}

			_, err := app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			h.mmapHeadChunks()
		}
		return i
	}

	testIsolation := func(h *Head, i int) {
	}

	// Test isolation without restart of Head.
	hb, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	i := addSamples(hb)
	testIsolation(hb, i)

	// Test simple cases in different chunks when no appendID cleanup has been performed.
	require.Equal(t, 10, lastValue(hb, 10))
	require.Equal(t, 130, lastValue(hb, 130))
	require.Equal(t, 160, lastValue(hb, 160))
	require.Equal(t, 240, lastValue(hb, 240))
	require.Equal(t, 500, lastValue(hb, 500))
	require.Equal(t, 750, lastValue(hb, 750))
	require.Equal(t, 995, lastValue(hb, 995))
	require.Equal(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 500.
	app := hb.appender()
	app.cleanupAppendIDsBelow = 500
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	i++

	// We should not get queries with a maxAppendID below 500 after the cleanup,
	// but they only take the remaining appendIDs into account.
	require.Equal(t, 499, lastValue(hb, 10))
	require.Equal(t, 499, lastValue(hb, 130))
	require.Equal(t, 499, lastValue(hb, 160))
	require.Equal(t, 499, lastValue(hb, 240))
	require.Equal(t, 500, lastValue(hb, 500))
	require.Equal(t, 995, lastValue(hb, 995))
	require.Equal(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appender()
	app.cleanupAppendIDsBelow = 1000
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 999, lastValue(hb, 998))
	require.Equal(t, 999, lastValue(hb, 999))
	require.Equal(t, 1000, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1002, lastValue(hb, 1002))
	require.Equal(t, 1002, lastValue(hb, 1003))

	i++
	// Cleanup appendIDs below 1001, but with a rollback.
	app = hb.appender()
	app.cleanupAppendIDsBelow = 1001
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, 1000, lastValue(hb, 999))
	require.Equal(t, 1000, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1002, lastValue(hb, 1002))
	require.Equal(t, 1002, lastValue(hb, 1003))

	require.NoError(t, hb.Close())

	// Test isolation with restart of Head. This is to verify the num samples of chunks after m-map chunk replay.
	hb, w := newTestHead(t, 1000, wlog.CompressionNone, false)
	i = addSamples(hb)
	require.NoError(t, hb.Close())

	wal, err := wlog.NewSize(nil, nil, w.Dir(), 32768, wlog.CompressionNone)
	require.NoError(t, err)
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = wal.Dir()
	hb, err = NewHead(nil, nil, wal, nil, opts, nil)
	defer func() { require.NoError(t, hb.Close()) }()
	require.NoError(t, err)
	require.NoError(t, hb.Init(0))

	// No appends after restarting. Hence all should return the last value.
	require.Equal(t, 1000, lastValue(hb, 10))
	require.Equal(t, 1000, lastValue(hb, 130))
	require.Equal(t, 1000, lastValue(hb, 160))
	require.Equal(t, 1000, lastValue(hb, 240))
	require.Equal(t, 1000, lastValue(hb, 500))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appender()
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	i++
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1001, lastValue(hb, 998))
	require.Equal(t, 1001, lastValue(hb, 999))
	require.Equal(t, 1001, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1001, lastValue(hb, 1002))
	require.Equal(t, 1001, lastValue(hb, 1003))

	// Cleanup appendIDs below 1002, but with a rollback.
	app = hb.appender()
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), int64(i), float64(i))
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, 1001, lastValue(hb, 999))
	require.Equal(t, 1001, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1001, lastValue(hb, 1002))
	require.Equal(t, 1001, lastValue(hb, 1003))
}

func TestIsolationRollback(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	// Rollback after a failed append and test if the low watermark has progressed anyway.
	hb, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(1), hb.iso.lowWatermark())

	app = hb.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 1, 1)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("foo", "bar", "foo", "baz"), 2, 2)
	require.Error(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, uint64(2), hb.iso.lowWatermark())

	app = hb.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 3, 3)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(3), hb.iso.lowWatermark(), "Low watermark should proceed to 3 even if append #2 was rolled back.")
}

func TestIsolationLowWatermarkMonotonous(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	hb, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app1 := hb.Appender(context.Background())
	_, err := app1.Append(0, labels.FromStrings("foo", "bar"), 0, 0)
	require.NoError(t, err)
	require.NoError(t, app1.Commit())
	require.Equal(t, uint64(1), hb.iso.lowWatermark(), "Low watermark should by 1 after 1st append.")

	app1 = hb.Appender(context.Background())
	_, err = app1.Append(0, labels.FromStrings("foo", "bar"), 1, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should be two, even if append is not committed yet.")

	app2 := hb.Appender(context.Background())
	_, err = app2.Append(0, labels.FromStrings("foo", "baz"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app2.Commit())
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should stay two because app1 is not committed yet.")

	is := hb.iso.State(math.MinInt64, math.MaxInt64)
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "After simulated read (iso state retrieved), low watermark should stay at 2.")

	require.NoError(t, app1.Commit())
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Even after app1 is committed, low watermark should stay at 2 because read is still ongoing.")

	is.Close()
	require.Equal(t, uint64(3), hb.iso.lowWatermark(), "After read has finished (iso state closed), low watermark should jump to three.")
}

func TestIsolationAppendIDZeroIsNoop(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	h, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	cOpts := chunkOpts{
		chunkDiskMapper: h.chunkDiskMapper,
		chunkRange:      h.chunkRange.Load(),
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"))

	ok, _ := s.append(0, 0, 0, cOpts)
	require.True(t, ok, "Series append failed.")
	require.Equal(t, 0, int(s.txs.txIDCount), "Series should not have an appendID after append with appendID=0.")
}

func TestHeadSeriesChunkRace(t *testing.T) {
	for i := 0; i < 1000; i++ {
		testHeadSeriesChunkRace(t)
	}
}

func TestIsolationWithoutAdd(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	hb, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.Appender(context.Background())
	require.NoError(t, app.Commit())

	app = hb.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "baz"), 1, 1)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, hb.iso.lastAppendID(), hb.iso.lowWatermark(), "High watermark should be equal to the low watermark")
}

func TestOutOfOrderSamplesMetric(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()
	db.DisableCompactions()

	ctx := context.Background()
	app := db.Appender(ctx)
	for i := 1; i <= 5; i++ {
		_, err = app.Append(0, labels.FromStrings("a", "b"), int64(i), 99)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Test out of order metric.
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat)))
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 2, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat)))

	_, err = app.Append(0, labels.FromStrings("a", "b"), 3, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat)))

	_, err = app.Append(0, labels.FromStrings("a", "b"), 4, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 3.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat)))
	require.NoError(t, app.Commit())

	// Compact Head to test out of bound metric.
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), DefaultBlockDuration*2, 99)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(math.MinInt64), db.head.minValidTime.Load())
	require.NoError(t, db.Compact(ctx))
	require.Greater(t, db.head.minValidTime.Load(), int64(0))

	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()-2, 99)
	require.Equal(t, storage.ErrOutOfBounds, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeFloat)))

	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()-1, 99)
	require.Equal(t, storage.ErrOutOfBounds, err)
	require.Equal(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples.WithLabelValues(sampleMetricTypeFloat)))
	require.NoError(t, app.Commit())

	// Some more valid samples for out of order.
	app = db.Appender(ctx)
	for i := 1; i <= 5; i++ {
		_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+int64(i), 99)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Test out of order metric.
	app = db.Appender(ctx)
	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+2, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 4.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat)))

	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+3, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 5.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat)))

	_, err = app.Append(0, labels.FromStrings("a", "b"), db.head.minValidTime.Load()+DefaultBlockDuration+4, 99)
	require.Equal(t, storage.ErrOutOfOrderSample, err)
	require.Equal(t, 6.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(sampleMetricTypeFloat)))
	require.NoError(t, app.Commit())
}

func testHeadSeriesChunkRace(t *testing.T) {
	h, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()
	require.NoError(t, h.Init(0))
	app := h.Appender(context.Background())

	s2, err := app.Append(0, labels.FromStrings("foo2", "bar"), 5, 0)
	require.NoError(t, err)
	for ts := int64(6); ts < 11; ts++ {
		_, err = app.Append(s2, labels.EmptyLabels(), ts, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	var wg sync.WaitGroup
	matcher := labels.MustNewMatcher(labels.MatchEqual, "", "")
	q, err := NewBlockQuerier(h, 18, 22)
	require.NoError(t, err)
	defer q.Close()

	wg.Add(1)
	go func() {
		h.updateMinMaxTime(20, 25)
		h.gc()
		wg.Done()
	}()
	ss := q.Select(context.Background(), false, nil, matcher)
	for ss.Next() {
	}
	require.NoError(t, ss.Err())
	wg.Wait()
}

func TestHeadLabelNamesValuesWithMinMaxRange(t *testing.T) {
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	const (
		firstSeriesTimestamp  int64 = 100
		secondSeriesTimestamp int64 = 200
		lastSeriesTimestamp   int64 = 300
	)
	var (
		seriesTimestamps = []int64{
			firstSeriesTimestamp,
			secondSeriesTimestamp,
			lastSeriesTimestamp,
		}
		expectedLabelNames  = []string{"a", "b", "c"}
		expectedLabelValues = []string{"d", "e", "f"}
		ctx                 = context.Background()
	)

	app := head.Appender(ctx)
	for i, name := range expectedLabelNames {
		_, err := app.Append(0, labels.FromStrings(name, expectedLabelValues[i]), seriesTimestamps[i], 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.Equal(t, firstSeriesTimestamp, head.MinTime())
	require.Equal(t, lastSeriesTimestamp, head.MaxTime())

	testCases := []struct {
		name           string
		mint           int64
		maxt           int64
		expectedNames  []string
		expectedValues []string
	}{
		{"maxt less than head min", head.MaxTime() - 10, head.MinTime() - 10, []string{}, []string{}},
		{"mint less than head max", head.MaxTime() + 10, head.MinTime() + 10, []string{}, []string{}},
		{"mint and maxt outside head", head.MaxTime() + 10, head.MinTime() - 10, []string{}, []string{}},
		{"mint and maxt within head", head.MaxTime() - 10, head.MinTime() + 10, expectedLabelNames, expectedLabelValues},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			headIdxReader := head.indexRange(tt.mint, tt.maxt)
			actualLabelNames, err := headIdxReader.LabelNames(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNames, actualLabelNames)
			if len(tt.expectedValues) > 0 {
				for i, name := range expectedLabelNames {
					actualLabelValue, err := headIdxReader.SortedLabelValues(ctx, name)
					require.NoError(t, err)
					require.Equal(t, []string{tt.expectedValues[i]}, actualLabelValue)
				}
			}
		})
	}
}

func TestHeadLabelValuesWithMatchers(t *testing.T) {
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	t.Cleanup(func() { require.NoError(t, head.Close()) })

	ctx := context.Background()

	app := head.Appender(context.Background())
	for i := 0; i < 100; i++ {
		_, err := app.Append(0, labels.FromStrings(
			"tens", fmt.Sprintf("value%d", i/10),
			"unique", fmt.Sprintf("value%d", i),
		), 100, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	var uniqueWithout30s []string
	for i := 0; i < 100; i++ {
		if i/10 != 3 {
			uniqueWithout30s = append(uniqueWithout30s, fmt.Sprintf("value%d", i))
		}
	}
	sort.Strings(uniqueWithout30s)
	testCases := []struct {
		name           string
		labelName      string
		matchers       []*labels.Matcher
		expectedValues []string
	}{
		{
			name:           "get tens based on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "unique", "value35")},
			expectedValues: []string{"value3"},
		}, {
			name:           "get unique ids based on a ten",
			labelName:      "unique",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "tens", "value1")},
			expectedValues: []string{"value10", "value11", "value12", "value13", "value14", "value15", "value16", "value17", "value18", "value19"},
		}, {
			name:           "get tens by pattern matching on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "unique", "value[5-7]5")},
			expectedValues: []string{"value5", "value6", "value7"},
		}, {
			name:           "get tens by matching for presence of unique label",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "unique", "")},
			expectedValues: []string{"value0", "value1", "value2", "value3", "value4", "value5", "value6", "value7", "value8", "value9"},
		}, {
			name:      "get unique IDs based on tens not being equal to a certain value, while not empty",
			labelName: "unique",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "tens", "value3"),
				labels.MustNewMatcher(labels.MatchNotEqual, "tens", ""),
			},
			expectedValues: uniqueWithout30s,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			headIdxReader := head.indexRange(0, 200)

			actualValues, err := headIdxReader.SortedLabelValues(ctx, tt.labelName, tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)

			actualValues, err = headIdxReader.LabelValues(ctx, tt.labelName, tt.matchers...)
			sort.Strings(actualValues)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)
		})
	}
}

func TestHeadLabelNamesWithMatchers(t *testing.T) {
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	app := head.Appender(context.Background())
	for i := 0; i < 100; i++ {
		_, err := app.Append(0, labels.FromStrings(
			"unique", fmt.Sprintf("value%d", i),
		), 100, 0)
		require.NoError(t, err)

		if i%10 == 0 {
			_, err := app.Append(0, labels.FromStrings(
				"tens", fmt.Sprintf("value%d", i/10),
				"unique", fmt.Sprintf("value%d", i),
			), 100, 0)
			require.NoError(t, err)
		}

		if i%20 == 0 {
			_, err := app.Append(0, labels.FromStrings(
				"tens", fmt.Sprintf("value%d", i/10),
				"twenties", fmt.Sprintf("value%d", i/20),
				"unique", fmt.Sprintf("value%d", i),
			), 100, 0)
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())

	testCases := []struct {
		name          string
		labelName     string
		matchers      []*labels.Matcher
		expectedNames []string
	}{
		{
			name:          "get with non-empty unique: all",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "unique", "")},
			expectedNames: []string{"tens", "twenties", "unique"},
		}, {
			name:          "get with unique ending in 1: only unique",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "unique", "value.*1")},
			expectedNames: []string{"unique"},
		}, {
			name:          "get with unique = value20: all",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "unique", "value20")},
			expectedNames: []string{"tens", "twenties", "unique"},
		}, {
			name:          "get tens = 1: unique & tens",
			matchers:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "tens", "value1")},
			expectedNames: []string{"tens", "unique"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			headIdxReader := head.indexRange(0, 200)

			actualNames, err := headIdxReader.LabelNames(context.Background(), tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNames, actualNames)
		})
	}
}

func TestHeadShardedPostings(t *testing.T) {
	headOpts := newTestHeadDefaultOptions(1000, false)
	headOpts.EnableSharding = true
	head, _ := newTestHeadWithOptions(t, wlog.CompressionNone, headOpts)
	defer func() {
		require.NoError(t, head.Close())
	}()

	ctx := context.Background()

	// Append some series.
	app := head.Appender(ctx)
	for i := 0; i < 100; i++ {
		_, err := app.Append(0, labels.FromStrings("unique", fmt.Sprintf("value%d", i), "const", "1"), 100, 0)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	ir := head.indexRange(0, 200)

	// List all postings for a given label value. This is what we expect to get
	// in output from all shards.
	p, err := ir.Postings(ctx, "const", "1")
	require.NoError(t, err)

	var expected []storage.SeriesRef
	for p.Next() {
		expected = append(expected, p.At())
	}
	require.NoError(t, p.Err())
	require.NotEmpty(t, expected)

	// Query the same postings for each shard.
	const shardCount = uint64(4)
	actualShards := make(map[uint64][]storage.SeriesRef)
	actualPostings := make([]storage.SeriesRef, 0, len(expected))

	for shardIndex := uint64(0); shardIndex < shardCount; shardIndex++ {
		p, err = ir.Postings(ctx, "const", "1")
		require.NoError(t, err)

		p = ir.ShardedPostings(p, shardIndex, shardCount)
		for p.Next() {
			ref := p.At()

			actualShards[shardIndex] = append(actualShards[shardIndex], ref)
			actualPostings = append(actualPostings, ref)
		}
		require.NoError(t, p.Err())
	}

	// We expect the postings merged out of shards is the exact same of the non sharded ones.
	require.ElementsMatch(t, expected, actualPostings)

	// We expect the series in each shard are the expected ones.
	for shardIndex, ids := range actualShards {
		for _, id := range ids {
			var lbls labels.ScratchBuilder

			require.NoError(t, ir.Series(id, &lbls, nil))
			require.Equal(t, shardIndex, labels.StableHash(lbls.Labels())%shardCount)
		}
	}
}

func TestErrReuseAppender(t *testing.T) {
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	app := head.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("test", "test"), 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())

	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("test", "test"), 1, 0)
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Error(t, app.Rollback())
	require.Error(t, app.Commit())

	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("test", "test"), 2, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Error(t, app.Rollback())
	require.Error(t, app.Commit())

	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("test", "test"), 3, 0)
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())
}

func TestHeadMintAfterTruncation(t *testing.T) {
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, wlog.CompressionNone, false)

	app := head.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("a", "b"), 100, 100)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 4000, 200)
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 8000, 300)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Truncating outside the appendable window and actual mint being outside
	// appendable window should leave mint at the actual mint.
	require.NoError(t, head.Truncate(3500))
	require.Equal(t, int64(4000), head.MinTime())
	require.Equal(t, int64(4000), head.minValidTime.Load())

	// After truncation outside the appendable window if the actual min time
	// is in the appendable window then we should leave mint at the start of appendable window.
	require.NoError(t, head.Truncate(5000))
	require.Equal(t, head.appendableMinValidTime(), head.MinTime())
	require.Equal(t, head.appendableMinValidTime(), head.minValidTime.Load())

	// If the truncation time is inside the appendable window, then the min time
	// should be the truncation time.
	require.NoError(t, head.Truncate(7500))
	require.Equal(t, int64(7500), head.MinTime())
	require.Equal(t, int64(7500), head.minValidTime.Load())

	require.NoError(t, head.Close())
}

func TestHeadExemplars(t *testing.T) {
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, wlog.CompressionNone, false)
	app := head.Appender(context.Background())

	l := labels.FromStrings("trace_id", "123")
	// It is perfectly valid to add Exemplars before the current start time -
	// histogram buckets that haven't been update in a while could still be
	// exported exemplars from an hour ago.
	ref, err := app.Append(0, labels.FromStrings("a", "b"), 100, 100)
	require.NoError(t, err)
	_, err = app.AppendExemplar(ref, l, exemplar.Exemplar{
		Labels: l,
		HasTs:  true,
		Ts:     -1000,
		Value:  1,
	})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.NoError(t, head.Close())
}

func BenchmarkHeadLabelValuesWithMatchers(b *testing.B) {
	chunkRange := int64(2000)
	head, _ := newTestHead(b, chunkRange, wlog.CompressionNone, false)
	b.Cleanup(func() { require.NoError(b, head.Close()) })

	ctx := context.Background()

	app := head.Appender(context.Background())

	metricCount := 1000000
	for i := 0; i < metricCount; i++ {
		_, err := app.Append(0, labels.FromStrings(
			"a_unique", fmt.Sprintf("value%d", i),
			"b_tens", fmt.Sprintf("value%d", i/(metricCount/10)),
			"c_ninety", fmt.Sprintf("value%d", i/(metricCount/10)/9), // "0" for the first 90%, then "1"
		), 100, 0)
		require.NoError(b, err)
	}
	require.NoError(b, app.Commit())

	headIdxReader := head.indexRange(0, 200)
	matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "c_ninety", "value0")}

	b.ResetTimer()
	b.ReportAllocs()

	for benchIdx := 0; benchIdx < b.N; benchIdx++ {
		actualValues, err := headIdxReader.LabelValues(ctx, "b_tens", matchers...)
		require.NoError(b, err)
		require.Len(b, actualValues, 9)
	}
}

func TestIteratorSeekIntoBuffer(t *testing.T) {
	dir := t.TempDir()
	// This is usually taken from the Head, but passing manually here.
	chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, chunkDiskMapper.Close())
	}()
	cOpts := chunkOpts{
		chunkDiskMapper: chunkDiskMapper,
		chunkRange:      500,
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled)

	for i := 0; i < 7; i++ {
		ok, _ := s.append(int64(i), float64(i), 0, cOpts)
		require.True(t, ok, "sample append failed")
	}

	c, _, _, err := s.chunk(0, chunkDiskMapper, &sync.Pool{
		New: func() interface{} {
			return &memChunk{}
		},
	})
	require.NoError(t, err)
	it := c.chunk.Iterator(nil)

	// First point.
	require.Equal(t, chunkenc.ValFloat, it.Seek(0))
	ts, val := it.At()
	require.Equal(t, int64(0), ts)
	require.Equal(t, float64(0), val)

	// Advance one point.
	require.Equal(t, chunkenc.ValFloat, it.Next())
	ts, val = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, float64(1), val)

	// Seeking an older timestamp shouldn't cause the iterator to go backwards.
	require.Equal(t, chunkenc.ValFloat, it.Seek(0))
	ts, val = it.At()
	require.Equal(t, int64(1), ts)
	require.Equal(t, float64(1), val)

	// Seek into the buffer.
	require.Equal(t, chunkenc.ValFloat, it.Seek(3))
	ts, val = it.At()
	require.Equal(t, int64(3), ts)
	require.Equal(t, float64(3), val)

	// Iterate through the rest of the buffer.
	for i := 4; i < 7; i++ {
		require.Equal(t, chunkenc.ValFloat, it.Next())
		ts, val = it.At()
		require.Equal(t, int64(i), ts)
		require.Equal(t, float64(i), val)
	}

	// Run out of elements in the iterator.
	require.Equal(t, chunkenc.ValNone, it.Next())
	require.Equal(t, chunkenc.ValNone, it.Seek(7))
}

// Tests https://github.com/prometheus/prometheus/issues/8221.
func TestChunkNotFoundHeadGCRace(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()
	ctx := context.Background()

	var (
		app        = db.Appender(context.Background())
		ref        = storage.SeriesRef(0)
		mint, maxt = int64(0), int64(0)
		err        error
	)

	// Appends samples to span over 1.5 block ranges.
	// 7 chunks with 15s scrape interval.
	for i := int64(0); i <= 120*7; i++ {
		ts := i * DefaultBlockDuration / (4 * 120)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), ts, float64(i))
		require.NoError(t, err)
		maxt = ts
	}
	require.NoError(t, app.Commit())

	// Get a querier before compaction (or when compaction is about to begin).
	q, err := db.Querier(mint, maxt)
	require.NoError(t, err)

	// Query the compacted range and get the first series before compaction.
	ss := q.Select(context.Background(), true, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, ss.Next())
	s := ss.At()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Compacting head while the querier spans the compaction time.
		require.NoError(t, db.Compact(ctx))
		require.NotEmpty(t, db.Blocks())
	}()

	// Give enough time for compaction to finish.
	// We expect it to be blocked until querier is closed.
	<-time.After(3 * time.Second)

	// Now consume after compaction when it's gone.
	it := s.Iterator(nil)
	for it.Next() == chunkenc.ValFloat {
		_, _ = it.At()
	}
	// It should error here without any fix for the mentioned issue.
	require.NoError(t, it.Err())
	for ss.Next() {
		s = ss.At()
		it = s.Iterator(it)
		for it.Next() == chunkenc.ValFloat {
			_, _ = it.At()
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, ss.Err())

	require.NoError(t, q.Close())
	wg.Wait()
}

// Tests https://github.com/prometheus/prometheus/issues/9079.
func TestDataMissingOnQueryDuringCompaction(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()
	ctx := context.Background()

	var (
		app        = db.Appender(context.Background())
		ref        = storage.SeriesRef(0)
		mint, maxt = int64(0), int64(0)
		err        error
	)

	// Appends samples to span over 1.5 block ranges.
	expSamples := make([]chunks.Sample, 0)
	// 7 chunks with 15s scrape interval.
	for i := int64(0); i <= 120*7; i++ {
		ts := i * DefaultBlockDuration / (4 * 120)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), ts, float64(i))
		require.NoError(t, err)
		maxt = ts
		expSamples = append(expSamples, sample{ts, float64(i), nil, nil})
	}
	require.NoError(t, app.Commit())

	// Get a querier before compaction (or when compaction is about to begin).
	q, err := db.Querier(mint, maxt)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Compacting head while the querier spans the compaction time.
		require.NoError(t, db.Compact(ctx))
		require.NotEmpty(t, db.Blocks())
	}()

	// Give enough time for compaction to finish.
	// We expect it to be blocked until querier is closed.
	<-time.After(3 * time.Second)

	// Querying the querier that was got before compaction.
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.Equal(t, map[string][]chunks.Sample{`{a="b"}`: expSamples}, series)

	wg.Wait()
}

func TestIsQuerierCollidingWithTruncation(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()

	var (
		app = db.Appender(context.Background())
		ref = storage.SeriesRef(0)
		err error
	)

	for i := int64(0); i <= 3000; i++ {
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), i, float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// This mocks truncation.
	db.head.memTruncationInProcess.Store(true)
	db.head.lastMemoryTruncationTime.Store(2000)

	// Test that IsQuerierValid suggests correct querier ranges.
	cases := []struct {
		mint, maxt                int64 // For the querier.
		expShouldClose, expGetNew bool
		expNewMint                int64
	}{
		{-200, -100, true, false, 0},
		{-200, 300, true, false, 0},
		{100, 1900, true, false, 0},
		{1900, 2200, true, true, 2000},
		{2000, 2500, false, false, 0},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("mint=%d,maxt=%d", c.mint, c.maxt), func(t *testing.T) {
			shouldClose, getNew, newMint := db.head.IsQuerierCollidingWithTruncation(c.mint, c.maxt)
			require.Equal(t, c.expShouldClose, shouldClose)
			require.Equal(t, c.expGetNew, getNew)
			if getNew {
				require.Equal(t, c.expNewMint, newMint)
			}
		})
	}
}

func TestWaitForPendingReadersInTimeRange(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()

	sampleTs := func(i int64) int64 { return i * DefaultBlockDuration / (4 * 120) }

	var (
		app = db.Appender(context.Background())
		ref = storage.SeriesRef(0)
		err error
	)

	for i := int64(0); i <= 3000; i++ {
		ts := sampleTs(i)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), ts, float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	truncMint, truncMaxt := int64(1000), int64(2000)
	cases := []struct {
		mint, maxt int64
		shouldWait bool
	}{
		{0, 500, false},     // Before truncation range.
		{500, 1500, true},   // Overlaps with truncation at the start.
		{1200, 1700, true},  // Within truncation range.
		{1800, 2500, true},  // Overlaps with truncation at the end.
		{2000, 2500, false}, // After truncation range.
		{2100, 2500, false}, // After truncation range.
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("mint=%d,maxt=%d,shouldWait=%t", c.mint, c.maxt, c.shouldWait), func(t *testing.T) {
			checkWaiting := func(cl io.Closer) {
				var waitOver atomic.Bool
				go func() {
					db.head.WaitForPendingReadersInTimeRange(truncMint, truncMaxt)
					waitOver.Store(true)
				}()
				<-time.After(550 * time.Millisecond)
				require.Equal(t, !c.shouldWait, waitOver.Load())
				require.NoError(t, cl.Close())
				<-time.After(550 * time.Millisecond)
				require.True(t, waitOver.Load())
			}

			q, err := db.Querier(c.mint, c.maxt)
			require.NoError(t, err)
			checkWaiting(q)

			cq, err := db.ChunkQuerier(c.mint, c.maxt)
			require.NoError(t, err)
			checkWaiting(cq)
		})
	}
}

func TestAppendHistogram(t *testing.T) {
	l := labels.FromStrings("a", "b")
	for _, numHistograms := range []int{1, 10, 150, 200, 250, 300} {
		t.Run(strconv.Itoa(numHistograms), func(t *testing.T) {
			head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
			t.Cleanup(func() {
				require.NoError(t, head.Close())
			})

			require.NoError(t, head.Init(0))
			ingestTs := int64(0)
			app := head.Appender(context.Background())

			expHistograms := make([]chunks.Sample, 0, 2*numHistograms)

			// Counter integer histograms.
			for _, h := range tsdbutil.GenerateTestHistograms(numHistograms) {
				_, err := app.AppendHistogram(0, l, ingestTs, h, nil)
				require.NoError(t, err)
				expHistograms = append(expHistograms, sample{t: ingestTs, h: h})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.Appender(context.Background())
				}
			}

			// Gauge integer histograms.
			for _, h := range tsdbutil.GenerateTestGaugeHistograms(numHistograms) {
				_, err := app.AppendHistogram(0, l, ingestTs, h, nil)
				require.NoError(t, err)
				expHistograms = append(expHistograms, sample{t: ingestTs, h: h})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.Appender(context.Background())
				}
			}

			expFloatHistograms := make([]chunks.Sample, 0, 2*numHistograms)

			// Counter float histograms.
			for _, fh := range tsdbutil.GenerateTestFloatHistograms(numHistograms) {
				_, err := app.AppendHistogram(0, l, ingestTs, nil, fh)
				require.NoError(t, err)
				expFloatHistograms = append(expFloatHistograms, sample{t: ingestTs, fh: fh})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.Appender(context.Background())
				}
			}

			// Gauge float histograms.
			for _, fh := range tsdbutil.GenerateTestGaugeFloatHistograms(numHistograms) {
				_, err := app.AppendHistogram(0, l, ingestTs, nil, fh)
				require.NoError(t, err)
				expFloatHistograms = append(expFloatHistograms, sample{t: ingestTs, fh: fh})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.Appender(context.Background())
				}
			}

			require.NoError(t, app.Commit())

			q, err := NewBlockQuerier(head, head.MinTime(), head.MaxTime())
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, q.Close())
			})

			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

			require.True(t, ss.Next())
			s := ss.At()
			require.False(t, ss.Next())

			it := s.Iterator(nil)
			actHistograms := make([]chunks.Sample, 0, len(expHistograms))
			actFloatHistograms := make([]chunks.Sample, 0, len(expFloatHistograms))
			for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
				switch typ {
				case chunkenc.ValHistogram:
					ts, h := it.AtHistogram(nil)
					actHistograms = append(actHistograms, sample{t: ts, h: h})
				case chunkenc.ValFloatHistogram:
					ts, fh := it.AtFloatHistogram(nil)
					actFloatHistograms = append(actFloatHistograms, sample{t: ts, fh: fh})
				}
			}

			compareSeries(
				t,
				map[string][]chunks.Sample{"dummy": expHistograms},
				map[string][]chunks.Sample{"dummy": actHistograms},
			)
			compareSeries(
				t,
				map[string][]chunks.Sample{"dummy": expFloatHistograms},
				map[string][]chunks.Sample{"dummy": actFloatHistograms},
			)
		})
	}
}

func TestHistogramInWALAndMmapChunk(t *testing.T) {
	head, _ := newTestHead(t, 3000, wlog.CompressionNone, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	// Series with only histograms.
	s1 := labels.FromStrings("a", "b1")
	k1 := s1.String()
	numHistograms := 300
	exp := map[string][]chunks.Sample{}
	ts := int64(0)
	var app storage.Appender
	for _, gauge := range []bool{true, false} {
		app = head.Appender(context.Background())
		var hists []*histogram.Histogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeHistograms(numHistograms)
		} else {
			hists = tsdbutil.GenerateTestHistograms(numHistograms)
		}
		for _, h := range hists {
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.AppendHistogram(0, s1, ts, h, nil)
			require.NoError(t, err)
			exp[k1] = append(exp[k1], sample{t: ts, h: h.Copy()})
			ts++
			if ts%5 == 0 {
				require.NoError(t, app.Commit())
				app = head.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}
	for _, gauge := range []bool{true, false} {
		app = head.Appender(context.Background())
		var hists []*histogram.FloatHistogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeFloatHistograms(numHistograms)
		} else {
			hists = tsdbutil.GenerateTestFloatHistograms(numHistograms)
		}
		for _, h := range hists {
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.AppendHistogram(0, s1, ts, nil, h)
			require.NoError(t, err)
			exp[k1] = append(exp[k1], sample{t: ts, fh: h.Copy()})
			ts++
			if ts%5 == 0 {
				require.NoError(t, app.Commit())
				app = head.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
		head.mmapHeadChunks()
	}

	// There should be 20 mmap chunks in s1.
	ms := head.series.getByHash(s1.Hash(), s1)
	require.Len(t, ms.mmappedChunks, 25)
	expMmapChunks := make([]*mmappedChunk, 0, 20)
	for _, mmap := range ms.mmappedChunks {
		require.Greater(t, mmap.numSamples, uint16(0))
		cpy := *mmap
		expMmapChunks = append(expMmapChunks, &cpy)
	}
	expHeadChunkSamples := ms.headChunks.chunk.NumSamples()
	require.Positive(t, expHeadChunkSamples)

	// Series with mix of histograms and float.
	s2 := labels.FromStrings("a", "b2")
	k2 := s2.String()
	ts = 0
	for _, gauge := range []bool{true, false} {
		app = head.Appender(context.Background())
		var hists []*histogram.Histogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeHistograms(100)
		} else {
			hists = tsdbutil.GenerateTestHistograms(100)
		}
		for _, h := range hists {
			ts++
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.AppendHistogram(0, s2, ts, h, nil)
			require.NoError(t, err)
			eh := h.Copy()
			if !gauge && ts > 30 && (ts-10)%20 == 1 {
				// Need "unknown" hint after float sample.
				eh.CounterResetHint = histogram.UnknownCounterReset
			}
			exp[k2] = append(exp[k2], sample{t: ts, h: eh})
			if ts%20 == 0 {
				require.NoError(t, app.Commit())
				app = head.Appender(context.Background())
				// Add some float.
				for i := 0; i < 10; i++ {
					ts++
					_, err := app.Append(0, s2, ts, float64(ts))
					require.NoError(t, err)
					exp[k2] = append(exp[k2], sample{t: ts, f: float64(ts)})
				}
				require.NoError(t, app.Commit())
				app = head.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}
	for _, gauge := range []bool{true, false} {
		app = head.Appender(context.Background())
		var hists []*histogram.FloatHistogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeFloatHistograms(100)
		} else {
			hists = tsdbutil.GenerateTestFloatHistograms(100)
		}
		for _, h := range hists {
			ts++
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.AppendHistogram(0, s2, ts, nil, h)
			require.NoError(t, err)
			eh := h.Copy()
			if !gauge && ts > 30 && (ts-10)%20 == 1 {
				// Need "unknown" hint after float sample.
				eh.CounterResetHint = histogram.UnknownCounterReset
			}
			exp[k2] = append(exp[k2], sample{t: ts, fh: eh})
			if ts%20 == 0 {
				require.NoError(t, app.Commit())
				app = head.Appender(context.Background())
				// Add some float.
				for i := 0; i < 10; i++ {
					ts++
					_, err := app.Append(0, s2, ts, float64(ts))
					require.NoError(t, err)
					exp[k2] = append(exp[k2], sample{t: ts, f: float64(ts)})
				}
				require.NoError(t, app.Commit())
				app = head.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	// Restart head.
	require.NoError(t, head.Close())
	startHead := func() {
		w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, w, nil, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(0))
	}
	startHead()

	// Checking contents of s1.
	ms = head.series.getByHash(s1.Hash(), s1)
	require.Equal(t, expMmapChunks, ms.mmappedChunks)
	require.Equal(t, expHeadChunkSamples, ms.headChunks.chunk.NumSamples())

	testQuery := func() {
		q, err := NewBlockQuerier(head, head.MinTime(), head.MaxTime())
		require.NoError(t, err)
		act := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "a", "b.*"))
		compareSeries(t, exp, act)
	}
	testQuery()

	// Restart with no mmap chunks to test WAL replay.
	require.NoError(t, head.Close())
	require.NoError(t, os.RemoveAll(mmappedChunksDir(head.opts.ChunkDirRoot)))
	startHead()
	testQuery()
}

func TestChunkSnapshot(t *testing.T) {
	head, _ := newTestHead(t, 120*4, wlog.CompressionNone, false)
	defer func() {
		head.opts.EnableMemorySnapshotOnShutdown = false
		require.NoError(t, head.Close())
	}()

	type ex struct {
		seriesLabels labels.Labels
		e            exemplar.Exemplar
	}

	numSeries := 10
	expSeries := make(map[string][]chunks.Sample)
	expHist := make(map[string][]chunks.Sample)
	expFloatHist := make(map[string][]chunks.Sample)
	expTombstones := make(map[storage.SeriesRef]tombstones.Intervals)
	expExemplars := make([]ex, 0)
	histograms := tsdbutil.GenerateTestGaugeHistograms(481)
	floatHistogram := tsdbutil.GenerateTestGaugeFloatHistograms(481)

	addExemplar := func(app storage.Appender, ref storage.SeriesRef, lbls labels.Labels, ts int64) {
		e := ex{
			seriesLabels: lbls,
			e: exemplar.Exemplar{
				Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
				Value:  rand.Float64(),
				Ts:     ts,
			},
		}
		expExemplars = append(expExemplars, e)
		_, err := app.AppendExemplar(ref, e.seriesLabels, e.e)
		require.NoError(t, err)
	}

	checkSamples := func() {
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expSeries, series)
	}
	checkHistograms := func() {
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "hist", "baz.*"))
		require.Equal(t, expHist, series)
	}
	checkFloatHistograms := func() {
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "floathist", "bat.*"))
		require.Equal(t, expFloatHist, series)
	}
	checkTombstones := func() {
		tr, err := head.Tombstones()
		require.NoError(t, err)
		actTombstones := make(map[storage.SeriesRef]tombstones.Intervals)
		require.NoError(t, tr.Iter(func(ref storage.SeriesRef, itvs tombstones.Intervals) error {
			for _, itv := range itvs {
				actTombstones[ref].Add(itv)
			}
			return nil
		}))
		require.Equal(t, expTombstones, actTombstones)
	}
	checkExemplars := func() {
		actExemplars := make([]ex, 0, len(expExemplars))
		err := head.exemplars.IterateExemplars(func(seriesLabels labels.Labels, e exemplar.Exemplar) error {
			actExemplars = append(actExemplars, ex{
				seriesLabels: seriesLabels,
				e:            e,
			})
			return nil
		})
		require.NoError(t, err)
		// Verifies both existence of right exemplars and order of exemplars in the buffer.
		testutil.RequireEqualWithOptions(t, expExemplars, actExemplars, []cmp.Option{cmp.AllowUnexported(ex{})})
	}

	var (
		wlast, woffset int
		err            error
	)

	closeHeadAndCheckSnapshot := func() {
		require.NoError(t, head.Close())

		_, sidx, soffset, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
		require.NoError(t, err)
		require.Equal(t, wlast, sidx)
		require.Equal(t, woffset, soffset)
	}

	openHeadAndCheckReplay := func() {
		w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, w, nil, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(math.MinInt64))

		checkSamples()
		checkHistograms()
		checkFloatHistograms()
		checkTombstones()
		checkExemplars()
	}

	{ // Initial data that goes into snapshot.
		// Add some initial samples with >=1 m-map chunk.
		app := head.Appender(context.Background())
		for i := 1; i <= numSeries; i++ {
			lbls := labels.FromStrings("foo", fmt.Sprintf("bar%d", i))
			lblStr := lbls.String()
			lblsHist := labels.FromStrings("hist", fmt.Sprintf("baz%d", i))
			lblsHistStr := lblsHist.String()
			lblsFloatHist := labels.FromStrings("floathist", fmt.Sprintf("bat%d", i))
			lblsFloatHistStr := lblsFloatHist.String()

			// 240 samples should m-map at least 1 chunk.
			for ts := int64(1); ts <= 240; ts++ {
				val := rand.Float64()
				expSeries[lblStr] = append(expSeries[lblStr], sample{ts, val, nil, nil})
				ref, err := app.Append(0, lbls, ts, val)
				require.NoError(t, err)

				hist := histograms[int(ts)]
				expHist[lblsHistStr] = append(expHist[lblsHistStr], sample{ts, 0, hist, nil})
				_, err = app.AppendHistogram(0, lblsHist, ts, hist, nil)
				require.NoError(t, err)

				floatHist := floatHistogram[int(ts)]
				expFloatHist[lblsFloatHistStr] = append(expFloatHist[lblsFloatHistStr], sample{ts, 0, nil, floatHist})
				_, err = app.AppendHistogram(0, lblsFloatHist, ts, nil, floatHist)
				require.NoError(t, err)

				// Add an exemplar and to create multiple WAL records.
				if ts%10 == 0 {
					addExemplar(app, ref, lbls, ts)
					require.NoError(t, app.Commit())
					app = head.Appender(context.Background())
				}
			}
		}
		require.NoError(t, app.Commit())

		// Add some tombstones.
		var enc record.Encoder
		for i := 1; i <= numSeries; i++ {
			ref := storage.SeriesRef(i)
			itvs := tombstones.Intervals{
				{Mint: 1234, Maxt: 2345},
				{Mint: 3456, Maxt: 4567},
			}
			for _, itv := range itvs {
				expTombstones[ref].Add(itv)
			}
			head.tombstones.AddInterval(ref, itvs...)
			err := head.wal.Log(enc.Tombstones([]tombstones.Stone{
				{Ref: ref, Intervals: itvs},
			}, nil))
			require.NoError(t, err)
		}
	}

	// These references should be the ones used for the snapshot.
	wlast, woffset, err = head.wal.LastSegmentAndOffset()
	require.NoError(t, err)
	if woffset != 0 && woffset < 32*1024 {
		// The page is always filled before taking the snapshot.
		woffset = 32 * 1024
	}

	{
		// Creating snapshot and verifying it.
		head.opts.EnableMemorySnapshotOnShutdown = true
		closeHeadAndCheckSnapshot() // This will create a snapshot.

		// Test the replay of snapshot.
		openHeadAndCheckReplay()
	}

	{ // Additional data to only include in WAL and m-mapped chunks and not snapshot. This mimics having an old snapshot on disk.
		// Add more samples.
		app := head.Appender(context.Background())
		for i := 1; i <= numSeries; i++ {
			lbls := labels.FromStrings("foo", fmt.Sprintf("bar%d", i))
			lblStr := lbls.String()
			lblsHist := labels.FromStrings("hist", fmt.Sprintf("baz%d", i))
			lblsHistStr := lblsHist.String()
			lblsFloatHist := labels.FromStrings("floathist", fmt.Sprintf("bat%d", i))
			lblsFloatHistStr := lblsFloatHist.String()

			// 240 samples should m-map at least 1 chunk.
			for ts := int64(241); ts <= 480; ts++ {
				val := rand.Float64()
				expSeries[lblStr] = append(expSeries[lblStr], sample{ts, val, nil, nil})
				ref, err := app.Append(0, lbls, ts, val)
				require.NoError(t, err)

				hist := histograms[int(ts)]
				expHist[lblsHistStr] = append(expHist[lblsHistStr], sample{ts, 0, hist, nil})
				_, err = app.AppendHistogram(0, lblsHist, ts, hist, nil)
				require.NoError(t, err)

				floatHist := floatHistogram[int(ts)]
				expFloatHist[lblsFloatHistStr] = append(expFloatHist[lblsFloatHistStr], sample{ts, 0, nil, floatHist})
				_, err = app.AppendHistogram(0, lblsFloatHist, ts, nil, floatHist)
				require.NoError(t, err)

				// Add an exemplar and to create multiple WAL records.
				if ts%10 == 0 {
					addExemplar(app, ref, lbls, ts)
					require.NoError(t, app.Commit())
					app = head.Appender(context.Background())
				}
			}
		}
		require.NoError(t, app.Commit())

		// Add more tombstones.
		var enc record.Encoder
		for i := 1; i <= numSeries; i++ {
			ref := storage.SeriesRef(i)
			itvs := tombstones.Intervals{
				{Mint: 12345, Maxt: 23456},
				{Mint: 34567, Maxt: 45678},
			}
			for _, itv := range itvs {
				expTombstones[ref].Add(itv)
			}
			head.tombstones.AddInterval(ref, itvs...)
			err := head.wal.Log(enc.Tombstones([]tombstones.Stone{
				{Ref: ref, Intervals: itvs},
			}, nil))
			require.NoError(t, err)
		}
	}
	{
		// Close Head and verify that new snapshot was not created.
		head.opts.EnableMemorySnapshotOnShutdown = false
		closeHeadAndCheckSnapshot() // This should not create a snapshot.

		// Test the replay of snapshot, m-map chunks, and WAL.
		head.opts.EnableMemorySnapshotOnShutdown = true // Enabled to read from snapshot.
		openHeadAndCheckReplay()
	}

	// Creating another snapshot should delete the older snapshot and replay still works fine.
	wlast, woffset, err = head.wal.LastSegmentAndOffset()
	require.NoError(t, err)
	if woffset != 0 && woffset < 32*1024 {
		// The page is always filled before taking the snapshot.
		woffset = 32 * 1024
	}

	{
		// Close Head and verify that new snapshot was created.
		closeHeadAndCheckSnapshot()

		// Verify that there is only 1 snapshot.
		files, err := os.ReadDir(head.opts.ChunkDirRoot)
		require.NoError(t, err)
		snapshots := 0
		for i := len(files) - 1; i >= 0; i-- {
			fi := files[i]
			if strings.HasPrefix(fi.Name(), chunkSnapshotPrefix) {
				snapshots++
				require.Equal(t, chunkSnapshotDir(wlast, woffset), fi.Name())
			}
		}
		require.Equal(t, 1, snapshots)

		// Test the replay of snapshot.
		head.opts.EnableMemorySnapshotOnShutdown = true // Enabled to read from snapshot.

		// Disabling exemplars to check that it does not hard fail replay
		// https://github.com/prometheus/prometheus/issues/9437#issuecomment-933285870.
		head.opts.EnableExemplarStorage = false
		head.opts.MaxExemplars.Store(0)
		expExemplars = expExemplars[:0]

		openHeadAndCheckReplay()

		require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))
	}
}

func TestSnapshotError(t *testing.T) {
	head, _ := newTestHead(t, 120*4, wlog.CompressionNone, false)
	defer func() {
		head.opts.EnableMemorySnapshotOnShutdown = false
		require.NoError(t, head.Close())
	}()

	// Add a sample.
	app := head.Appender(context.Background())
	lbls := labels.FromStrings("foo", "bar")
	_, err := app.Append(0, lbls, 99, 99)
	require.NoError(t, err)

	// Add histograms
	hist := tsdbutil.GenerateTestGaugeHistograms(1)[0]
	floatHist := tsdbutil.GenerateTestGaugeFloatHistograms(1)[0]
	lblsHist := labels.FromStrings("hist", "bar")
	lblsFloatHist := labels.FromStrings("floathist", "bar")

	_, err = app.AppendHistogram(0, lblsHist, 99, hist, nil)
	require.NoError(t, err)

	_, err = app.AppendHistogram(0, lblsFloatHist, 99, nil, floatHist)
	require.NoError(t, err)

	require.NoError(t, app.Commit())

	// Add some tombstones.
	itvs := tombstones.Intervals{
		{Mint: 1234, Maxt: 2345},
		{Mint: 3456, Maxt: 4567},
	}
	head.tombstones.AddInterval(1, itvs...)

	// Check existence of data.
	require.NotNil(t, head.series.getByHash(lbls.Hash(), lbls))
	tm, err := head.tombstones.Get(1)
	require.NoError(t, err)
	require.NotEmpty(t, tm)

	head.opts.EnableMemorySnapshotOnShutdown = true
	require.NoError(t, head.Close()) // This will create a snapshot.

	// Remove the WAL so that we don't load from it.
	require.NoError(t, os.RemoveAll(head.wal.Dir()))

	// Corrupt the snapshot.
	snapDir, _, _, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.NoError(t, err)
	files, err := os.ReadDir(snapDir)
	require.NoError(t, err)
	f, err := os.OpenFile(path.Join(snapDir, files[0].Name()), os.O_RDWR, 0)
	require.NoError(t, err)
	// Create snapshot backup to be restored on future test cases.
	snapshotBackup, err := io.ReadAll(f)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0b11111111}, 18)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Create new Head which should replay this snapshot.
	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
	require.NoError(t, err)
	// Testing https://github.com/prometheus/prometheus/issues/9437 with the registry.
	head, err = NewHead(prometheus.NewRegistry(), nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	// There should be no series in the memory after snapshot error since WAL was removed.
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))
	require.Equal(t, uint64(0), head.NumSeries())
	require.Nil(t, head.series.getByHash(lbls.Hash(), lbls))
	tm, err = head.tombstones.Get(1)
	require.NoError(t, err)
	require.Empty(t, tm)
	require.NoError(t, head.Close())

	// Test corruption in the middle of the snapshot.
	f, err = os.OpenFile(path.Join(snapDir, files[0].Name()), os.O_RDWR, 0)
	require.NoError(t, err)
	_, err = f.WriteAt(snapshotBackup, 0)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0b11111111}, 300)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	c := &countSeriesLifecycleCallback{}
	opts := head.opts
	opts.SeriesCallback = c

	w, err = wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
	require.NoError(t, err)
	head, err = NewHead(prometheus.NewRegistry(), nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	// There should be no series in the memory after snapshot error since WAL was removed.
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))
	require.Nil(t, head.series.getByHash(lbls.Hash(), lbls))
	require.Equal(t, uint64(0), head.NumSeries())

	// Since the snapshot could replay certain series, we continue invoking the create hooks.
	// In such instances, we need to ensure that we also trigger the delete hooks when resetting the memory.
	require.Equal(t, int64(2), c.created.Load())
	require.Equal(t, int64(2), c.deleted.Load())

	require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.seriesRemoved))
	require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.seriesCreated))
}

func TestHistogramMetrics(t *testing.T) {
	numHistograms := 10
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	expHSeries, expHSamples := 0, 0

	for x := 0; x < 5; x++ {
		expHSeries++
		l := labels.FromStrings("a", fmt.Sprintf("b%d", x))
		for i, h := range tsdbutil.GenerateTestHistograms(numHistograms) {
			app := head.Appender(context.Background())
			_, err := app.AppendHistogram(0, l, int64(i), h, nil)
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			expHSamples++
		}
		for i, fh := range tsdbutil.GenerateTestFloatHistograms(numHistograms) {
			app := head.Appender(context.Background())
			_, err := app.AppendHistogram(0, l, int64(numHistograms+i), nil, fh)
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			expHSamples++
		}
	}

	require.Equal(t, float64(expHSamples), prom_testutil.ToFloat64(head.metrics.samplesAppended.WithLabelValues(sampleMetricTypeHistogram)))

	require.NoError(t, head.Close())
	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
	require.NoError(t, err)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))

	require.Equal(t, float64(0), prom_testutil.ToFloat64(head.metrics.samplesAppended.WithLabelValues(sampleMetricTypeHistogram))) // Counter reset.
}

func TestHistogramStaleSample(t *testing.T) {
	t.Run("integer histogram", func(t *testing.T) {
		testHistogramStaleSampleHelper(t, false)
	})
	t.Run("float histogram", func(t *testing.T) {
		testHistogramStaleSampleHelper(t, true)
	})
}

func testHistogramStaleSampleHelper(t *testing.T, floatHistogram bool) {
	t.Helper()
	l := labels.FromStrings("a", "b")
	numHistograms := 20
	head, _ := newTestHead(t, 100000, wlog.CompressionNone, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	type timedHistogram struct {
		t  int64
		h  *histogram.Histogram
		fh *histogram.FloatHistogram
	}
	expHistograms := make([]timedHistogram, 0, numHistograms)

	testQuery := func(numStale int) {
		q, err := NewBlockQuerier(head, head.MinTime(), head.MaxTime())
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, q.Close())
		})

		ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

		require.True(t, ss.Next())
		s := ss.At()
		require.False(t, ss.Next())

		it := s.Iterator(nil)
		actHistograms := make([]timedHistogram, 0, len(expHistograms))
		for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
			switch typ {
			case chunkenc.ValHistogram:
				t, h := it.AtHistogram(nil)
				actHistograms = append(actHistograms, timedHistogram{t: t, h: h})
			case chunkenc.ValFloatHistogram:
				t, h := it.AtFloatHistogram(nil)
				actHistograms = append(actHistograms, timedHistogram{t: t, fh: h})
			}
		}

		// We cannot compare StaleNAN with require.Equal, hence checking each histogram manually.
		require.Equal(t, len(expHistograms), len(actHistograms))
		actNumStale := 0
		for i, eh := range expHistograms {
			ah := actHistograms[i]
			if floatHistogram {
				switch {
				case value.IsStaleNaN(eh.fh.Sum):
					actNumStale++
					require.True(t, value.IsStaleNaN(ah.fh.Sum))
					// To make require.Equal work.
					ah.fh.Sum = 0
					eh.fh = eh.fh.Copy()
					eh.fh.Sum = 0
				case i > 0:
					prev := expHistograms[i-1]
					if prev.fh == nil || value.IsStaleNaN(prev.fh.Sum) {
						eh.fh.CounterResetHint = histogram.UnknownCounterReset
					}
				}
				require.Equal(t, eh, ah)
			} else {
				switch {
				case value.IsStaleNaN(eh.h.Sum):
					actNumStale++
					require.True(t, value.IsStaleNaN(ah.h.Sum))
					// To make require.Equal work.
					ah.h.Sum = 0
					eh.h = eh.h.Copy()
					eh.h.Sum = 0
				case i > 0:
					prev := expHistograms[i-1]
					if prev.h == nil || value.IsStaleNaN(prev.h.Sum) {
						eh.h.CounterResetHint = histogram.UnknownCounterReset
					}
				}
				require.Equal(t, eh, ah)
			}
		}
		require.Equal(t, numStale, actNumStale)
	}

	// Adding stale in the same appender.
	app := head.Appender(context.Background())
	for _, h := range tsdbutil.GenerateTestHistograms(numHistograms) {
		var err error
		if floatHistogram {
			_, err = app.AppendHistogram(0, l, 100*int64(len(expHistograms)), nil, h.ToFloat(nil))
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), fh: h.ToFloat(nil)})
		} else {
			_, err = app.AppendHistogram(0, l, 100*int64(len(expHistograms)), h, nil)
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), h: h})
		}
		require.NoError(t, err)
	}
	// +1 so that delta-of-delta is not 0.
	_, err := app.Append(0, l, 100*int64(len(expHistograms))+1, math.Float64frombits(value.StaleNaN))
	require.NoError(t, err)
	if floatHistogram {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, fh: &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)}})
	} else {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, h: &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}})
	}
	require.NoError(t, app.Commit())

	// Only 1 chunk in the memory, no m-mapped chunk.
	s := head.series.getByHash(l.Hash(), l)
	require.NotNil(t, s)
	require.NotNil(t, s.headChunks)
	require.Equal(t, 1, s.headChunks.len())
	require.Empty(t, s.mmappedChunks)
	testQuery(1)

	// Adding stale in different appender and continuing series after a stale sample.
	app = head.Appender(context.Background())
	for _, h := range tsdbutil.GenerateTestHistograms(2 * numHistograms)[numHistograms:] {
		var err error
		if floatHistogram {
			_, err = app.AppendHistogram(0, l, 100*int64(len(expHistograms)), nil, h.ToFloat(nil))
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), fh: h.ToFloat(nil)})
		} else {
			_, err = app.AppendHistogram(0, l, 100*int64(len(expHistograms)), h, nil)
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), h: h})
		}
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	app = head.Appender(context.Background())
	// +1 so that delta-of-delta is not 0.
	_, err = app.Append(0, l, 100*int64(len(expHistograms))+1, math.Float64frombits(value.StaleNaN))
	require.NoError(t, err)
	if floatHistogram {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, fh: &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)}})
	} else {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, h: &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}})
	}
	require.NoError(t, app.Commit())
	head.mmapHeadChunks()

	// Total 2 chunks, 1 m-mapped.
	s = head.series.getByHash(l.Hash(), l)
	require.NotNil(t, s)
	require.NotNil(t, s.headChunks)
	require.Equal(t, 1, s.headChunks.len())
	require.Len(t, s.mmappedChunks, 1)
	testQuery(2)
}

func TestHistogramCounterResetHeader(t *testing.T) {
	for _, floatHisto := range []bool{true} { // FIXME
		t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
			l := labels.FromStrings("a", "b")
			head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
			t.Cleanup(func() {
				require.NoError(t, head.Close())
			})
			require.NoError(t, head.Init(0))

			ts := int64(0)
			appendHistogram := func(h *histogram.Histogram) {
				ts++
				app := head.Appender(context.Background())
				var err error
				if floatHisto {
					_, err = app.AppendHistogram(0, l, ts, nil, h.ToFloat(nil))
				} else {
					_, err = app.AppendHistogram(0, l, ts, h.Copy(), nil)
				}
				require.NoError(t, err)
				require.NoError(t, app.Commit())
			}

			var expHeaders []chunkenc.CounterResetHeader
			checkExpCounterResetHeader := func(newHeaders ...chunkenc.CounterResetHeader) {
				expHeaders = append(expHeaders, newHeaders...)

				ms, _, err := head.getOrCreate(l.Hash(), l)
				require.NoError(t, err)
				ms.mmapChunks(head.chunkDiskMapper)
				require.Len(t, ms.mmappedChunks, len(expHeaders)-1) // One is the head chunk.

				for i, mmapChunk := range ms.mmappedChunks {
					chk, err := head.chunkDiskMapper.Chunk(mmapChunk.ref)
					require.NoError(t, err)
					if floatHisto {
						require.Equal(t, expHeaders[i], chk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
					} else {
						require.Equal(t, expHeaders[i], chk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
					}
				}
				if floatHisto {
					require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
				} else {
					require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
				}
			}

			h := tsdbutil.GenerateTestHistograms(1)[0]
			h.PositiveBuckets = []int64{100, 1, 1, 1}
			h.NegativeBuckets = []int64{100, 1, 1, 1}
			h.Count = 1000

			// First histogram is UnknownCounterReset.
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.UnknownCounterReset)

			// Another normal histogram.
			h.Count++
			appendHistogram(h)
			checkExpCounterResetHeader()

			// Counter reset via Count.
			h.Count--
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Add 2 non-counter reset histogram chunks (each chunk targets 1024 bytes which contains ~500 int histogram
			// samples or ~1000 float histogram samples).
			numAppend := 2000
			if floatHisto {
				numAppend = 1000
			}
			for i := 0; i < numAppend; i++ {
				appendHistogram(h)
			}

			checkExpCounterResetHeader(chunkenc.NotCounterReset, chunkenc.NotCounterReset)

			// Changing schema will cut a new chunk with unknown counter reset.
			h.Schema++
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.UnknownCounterReset)

			// Changing schema will zero threshold a new chunk with unknown counter reset.
			h.ZeroThreshold += 0.01
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.UnknownCounterReset)

			// Counter reset by removing a positive bucket.
			h.PositiveSpans[1].Length--
			h.PositiveBuckets = h.PositiveBuckets[1:]
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Counter reset by removing a negative bucket.
			h.NegativeSpans[1].Length--
			h.NegativeBuckets = h.NegativeBuckets[1:]
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Add 2 non-counter reset histogram chunks. Just to have some non-counter reset chunks in between.
			for i := 0; i < 2000; i++ {
				appendHistogram(h)
			}
			checkExpCounterResetHeader(chunkenc.NotCounterReset, chunkenc.NotCounterReset)

			// Counter reset with counter reset in a positive bucket.
			h.PositiveBuckets[len(h.PositiveBuckets)-1]--
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Counter reset with counter reset in a negative bucket.
			h.NegativeBuckets[len(h.NegativeBuckets)-1]--
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)
		})
	}
}

func TestAppendingDifferentEncodingToSameSeries(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.EnableNativeHistograms = true
	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	db.DisableCompactions()

	hists := tsdbutil.GenerateTestHistograms(10)
	floatHists := tsdbutil.GenerateTestFloatHistograms(10)
	lbls := labels.FromStrings("a", "b")

	var expResult []chunks.Sample
	checkExpChunks := func(count int) {
		ms, created, err := db.Head().getOrCreate(lbls.Hash(), lbls)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms)
		require.Equal(t, count, ms.headChunks.len())
	}

	appends := []struct {
		samples   []chunks.Sample
		expChunks int
		err       error
		// If this is empty, samples above will be taken instead of this.
		addToExp []chunks.Sample
	}{
		// Histograms that end up in the expected samples are copied here so that we
		// can independently set the CounterResetHint later.
		{
			samples:   []chunks.Sample{sample{t: 100, h: hists[0].Copy()}},
			expChunks: 1,
		},
		{
			samples:   []chunks.Sample{sample{t: 200, f: 2}},
			expChunks: 2,
		},
		{
			samples:   []chunks.Sample{sample{t: 210, fh: floatHists[0].Copy()}},
			expChunks: 3,
		},
		{
			samples:   []chunks.Sample{sample{t: 220, h: hists[1].Copy()}},
			expChunks: 4,
		},
		{
			samples:   []chunks.Sample{sample{t: 230, fh: floatHists[3].Copy()}},
			expChunks: 5,
		},
		{
			samples: []chunks.Sample{sample{t: 100, h: hists[2].Copy()}},
			err:     storage.ErrOutOfOrderSample,
		},
		{
			samples:   []chunks.Sample{sample{t: 300, h: hists[3].Copy()}},
			expChunks: 6,
		},
		{
			samples: []chunks.Sample{sample{t: 100, f: 2}},
			err:     storage.ErrOutOfOrderSample,
		},
		{
			samples: []chunks.Sample{sample{t: 100, fh: floatHists[4].Copy()}},
			err:     storage.ErrOutOfOrderSample,
		},
		{
			// Combination of histograms and float64 in the same commit. The behaviour is undefined, but we want to also
			// verify how TSDB would behave. Here the histogram is appended at the end, hence will be considered as out of order.
			samples: []chunks.Sample{
				sample{t: 400, f: 4},
				sample{t: 500, h: hists[5]}, // This won't be committed.
				sample{t: 600, f: 6},
			},
			addToExp: []chunks.Sample{
				sample{t: 400, f: 4},
				sample{t: 600, f: 6},
			},
			expChunks: 7, // Only 1 new chunk for float64.
		},
		{
			// Here the histogram is appended at the end, hence the first histogram is out of order.
			samples: []chunks.Sample{
				sample{t: 700, h: hists[7]}, // Out of order w.r.t. the next float64 sample that is appended first.
				sample{t: 800, f: 8},
				sample{t: 900, h: hists[9]},
			},
			addToExp: []chunks.Sample{
				sample{t: 800, f: 8},
				sample{t: 900, h: hists[9].Copy()},
			},
			expChunks: 8, // float64 added to old chunk, only 1 new for histograms.
		},
		{
			// Float histogram is appended at the end.
			samples: []chunks.Sample{
				sample{t: 1000, fh: floatHists[7]}, // Out of order w.r.t. the next histogram.
				sample{t: 1100, h: hists[9]},
			},
			addToExp: []chunks.Sample{
				sample{t: 1100, h: hists[9].Copy()},
			},
			expChunks: 8,
		},
	}

	for _, a := range appends {
		app := db.Appender(context.Background())
		for _, s := range a.samples {
			var err error
			if s.H() != nil || s.FH() != nil {
				_, err = app.AppendHistogram(0, lbls, s.T(), s.H(), s.FH())
			} else {
				_, err = app.Append(0, lbls, s.T(), s.F())
			}
			require.Equal(t, a.err, err)
		}

		if a.err == nil {
			require.NoError(t, app.Commit())
			if len(a.addToExp) > 0 {
				expResult = append(expResult, a.addToExp...)
			} else {
				expResult = append(expResult, a.samples...)
			}
			checkExpChunks(a.expChunks)
		} else {
			require.NoError(t, app.Rollback())
		}
	}
	for i, s := range expResult[1:] {
		switch {
		case s.H() != nil && expResult[i].H() == nil:
			s.(sample).h.CounterResetHint = histogram.UnknownCounterReset
		case s.FH() != nil && expResult[i].FH() == nil:
			s.(sample).fh.CounterResetHint = histogram.UnknownCounterReset
		}
	}

	// Query back and expect same order of samples.
	q, err := db.Querier(math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.Equal(t, map[string][]chunks.Sample{lbls.String(): expResult}, series)
}

// Tests https://github.com/prometheus/prometheus/issues/9725.
func TestChunkSnapshotReplayBug(t *testing.T) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)

	// Write few series records and samples such that the series references are not in order in the WAL
	// for status_code="200".
	var buf []byte
	for i := 1; i <= 1000; i++ {
		var ref chunks.HeadSeriesRef
		if i <= 500 {
			ref = chunks.HeadSeriesRef(i * 100)
		} else {
			ref = chunks.HeadSeriesRef((i - 500) * 50)
		}
		seriesRec := record.RefSeries{
			Ref: ref,
			Labels: labels.FromStrings(
				"__name__", "request_duration",
				"status_code", "200",
				"foo", fmt.Sprintf("baz%d", rand.Int()),
			),
		}
		// Add a sample so that the series is not garbage collected.
		samplesRec := record.RefSample{Ref: ref, T: 1000, V: 1000}
		var enc record.Encoder

		rec := enc.Series([]record.RefSeries{seriesRec}, buf)
		buf = rec[:0]
		require.NoError(t, wal.Log(rec))
		rec = enc.Samples([]record.RefSample{samplesRec}, buf)
		buf = rec[:0]
		require.NoError(t, wal.Log(rec))
	}

	// Write a corrupt snapshot to fail the replay on startup.
	snapshotName := chunkSnapshotDir(0, 100)
	cpdir := filepath.Join(dir, snapshotName)
	require.NoError(t, os.MkdirAll(cpdir, 0o777))

	err = os.WriteFile(filepath.Join(cpdir, "00000000"), []byte{1, 5, 3, 5, 6, 7, 4, 2, 2}, 0o777)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.EnableMemorySnapshotOnShutdown = true
	head, err := NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))
	defer func() {
		require.NoError(t, head.Close())
	}()

	// Snapshot replay should error out.
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))

	// Querying `request_duration{status_code!="200"}` should return no series since all of
	// them have status_code="200".
	q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	series := query(t, q,
		labels.MustNewMatcher(labels.MatchEqual, "__name__", "request_duration"),
		labels.MustNewMatcher(labels.MatchNotEqual, "status_code", "200"),
	)
	require.Empty(t, series, "there should be no series found")
}

func TestChunkSnapshotTakenAfterIncompleteSnapshot(t *testing.T) {
	dir := t.TempDir()
	wlTemp, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)

	// Write a snapshot with .tmp suffix. This used to fail taking any further snapshots or replay of snapshots.
	snapshotName := chunkSnapshotDir(0, 100) + ".tmp"
	cpdir := filepath.Join(dir, snapshotName)
	require.NoError(t, os.MkdirAll(cpdir, 0o777))

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.EnableMemorySnapshotOnShutdown = true
	head, err := NewHead(nil, nil, wlTemp, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))

	// Add some samples for the snapshot.
	app := head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 10, 10)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Should not return any error for a successful snapshot.
	require.NoError(t, head.Close())

	// Verify the snapshot.
	name, idx, offset, err := LastChunkSnapshot(dir)
	require.NoError(t, err)
	require.NotEqual(t, "", name)
	require.Equal(t, 0, idx)
	require.Positive(t, offset)
}

// TestWBLReplay checks the replay at a low level.
func TestWBLReplay(t *testing.T) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = dir
	opts.OutOfOrderTimeWindow.Store(30 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	var expOOOSamples []sample
	l := labels.FromStrings("foo", "bar")
	appendSample := func(mins int64, isOOO bool) {
		app := h.Appender(context.Background())
		ts, v := mins*time.Minute.Milliseconds(), float64(mins)
		_, err := app.Append(0, l, ts, v)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		if isOOO {
			expOOOSamples = append(expOOOSamples, sample{t: ts, f: v})
		}
	}

	// In-order sample.
	appendSample(60, false)

	// Out of order samples.
	appendSample(40, true)
	appendSample(35, true)
	appendSample(50, true)
	appendSample(55, true)
	appendSample(59, true)
	appendSample(31, true)

	// Check that Head's time ranges are set properly.
	require.Equal(t, 60*time.Minute.Milliseconds(), h.MinTime())
	require.Equal(t, 60*time.Minute.Milliseconds(), h.MaxTime())
	require.Equal(t, 31*time.Minute.Milliseconds(), h.MinOOOTime())
	require.Equal(t, 59*time.Minute.Milliseconds(), h.MaxOOOTime())

	// Restart head.
	require.NoError(t, h.Close())
	wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	oooWlog, err = wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0)) // Replay happens here.

	// Get the ooo samples from the Head.
	ms, ok, err := h.getOrCreate(l.Hash(), l)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)

	xor, err := ms.ooo.oooHeadChunk.chunk.ToXOR()
	require.NoError(t, err)

	it := xor.Iterator(nil)
	actOOOSamples := make([]sample, 0, len(expOOOSamples))
	for it.Next() == chunkenc.ValFloat {
		ts, v := it.At()
		actOOOSamples = append(actOOOSamples, sample{t: ts, f: v})
	}

	// OOO chunk will be sorted. Hence sort the expected samples.
	sort.Slice(expOOOSamples, func(i, j int) bool {
		return expOOOSamples[i].t < expOOOSamples[j].t
	})

	require.Equal(t, expOOOSamples, actOOOSamples)

	require.NoError(t, h.Close())
}

// TestOOOMmapReplay checks the replay at a low level.
func TestOOOMmapReplay(t *testing.T) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = dir
	opts.OutOfOrderCapMax.Store(30)
	opts.OutOfOrderTimeWindow.Store(1000 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	l := labels.FromStrings("foo", "bar")
	appendSample := func(mins int64) {
		app := h.Appender(context.Background())
		ts, v := mins*time.Minute.Milliseconds(), float64(mins)
		_, err := app.Append(0, l, ts, v)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	// In-order sample.
	appendSample(200)

	// Out of order samples. 92 samples to create 3 m-map chunks.
	for mins := int64(100); mins <= 191; mins++ {
		appendSample(mins)
	}

	ms, ok, err := h.getOrCreate(l.Hash(), l)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)

	require.Len(t, ms.ooo.oooMmappedChunks, 3)
	// Verify that we can access the chunks without error.
	for _, m := range ms.ooo.oooMmappedChunks {
		chk, err := h.chunkDiskMapper.Chunk(m.ref)
		require.NoError(t, err)
		require.Equal(t, int(m.numSamples), chk.NumSamples())
	}

	expMmapChunks := make([]*mmappedChunk, 3)
	copy(expMmapChunks, ms.ooo.oooMmappedChunks)

	// Restart head.
	require.NoError(t, h.Close())

	wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	oooWlog, err = wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0)) // Replay happens here.

	// Get the mmap chunks from the Head.
	ms, ok, err = h.getOrCreate(l.Hash(), l)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)

	require.Len(t, ms.ooo.oooMmappedChunks, len(expMmapChunks))
	// Verify that we can access the chunks without error.
	for _, m := range ms.ooo.oooMmappedChunks {
		chk, err := h.chunkDiskMapper.Chunk(m.ref)
		require.NoError(t, err)
		require.Equal(t, int(m.numSamples), chk.NumSamples())
	}

	actMmapChunks := make([]*mmappedChunk, len(expMmapChunks))
	copy(actMmapChunks, ms.ooo.oooMmappedChunks)

	require.Equal(t, expMmapChunks, actMmapChunks)

	require.NoError(t, h.Close())
}

func TestHeadInit_DiscardChunksWithUnsupportedEncoding(t *testing.T) {
	h, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	require.NoError(t, h.Init(0))

	ctx := context.Background()
	app := h.Appender(ctx)
	seriesLabels := labels.FromStrings("a", "1")
	var seriesRef storage.SeriesRef
	var err error
	for i := 0; i < 400; i++ {
		seriesRef, err = app.Append(0, seriesLabels, int64(i), float64(i))
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
	require.Greater(t, prom_testutil.ToFloat64(h.metrics.chunksCreated), 1.0)

	uc := newUnsupportedChunk()
	// Make this chunk not overlap with the previous and the next
	h.chunkDiskMapper.WriteChunk(chunks.HeadSeriesRef(seriesRef), 500, 600, uc, false, func(err error) { require.NoError(t, err) })

	app = h.Appender(ctx)
	for i := 700; i < 1200; i++ {
		_, err := app.Append(0, seriesLabels, int64(i), float64(i))
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
	require.Greater(t, prom_testutil.ToFloat64(h.metrics.chunksCreated), 4.0)

	series, created, err := h.getOrCreate(seriesLabels.Hash(), seriesLabels)
	require.NoError(t, err)
	require.False(t, created, "should already exist")
	require.NotNil(t, series, "should return the series we created above")

	series.mmapChunks(h.chunkDiskMapper)
	expChunks := make([]*mmappedChunk, len(series.mmappedChunks))
	copy(expChunks, series.mmappedChunks)

	require.NoError(t, h.Close())

	wal, err := wlog.NewSize(nil, nil, filepath.Join(h.opts.ChunkDirRoot, "wal"), 32768, wlog.CompressionNone)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, nil, h.opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	series, created, err = h.getOrCreate(seriesLabels.Hash(), seriesLabels)
	require.NoError(t, err)
	require.False(t, created, "should already exist")
	require.NotNil(t, series, "should return the series we created above")

	require.Equal(t, expChunks, series.mmappedChunks)
}

const (
	UnsupportedMask   = 0b10000000
	EncUnsupportedXOR = chunkenc.EncXOR | UnsupportedMask
)

// unsupportedChunk holds a XORChunk and overrides the Encoding() method.
type unsupportedChunk struct {
	*chunkenc.XORChunk
}

func newUnsupportedChunk() *unsupportedChunk {
	return &unsupportedChunk{chunkenc.NewXORChunk()}
}

func (c *unsupportedChunk) Encoding() chunkenc.Encoding {
	return EncUnsupportedXOR
}

// Tests https://github.com/prometheus/prometheus/issues/10277.
func TestMmapPanicAfterMmapReplayCorruption(t *testing.T) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionNone)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = DefaultBlockDuration
	opts.ChunkDirRoot = dir
	opts.EnableExemplarStorage = true
	opts.MaxExemplars.Store(config.DefaultExemplarsConfig.MaxExemplars)

	h, err := NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	lastTs := int64(0)
	var ref storage.SeriesRef
	lbls := labels.FromStrings("__name__", "testing", "foo", "bar")
	addChunks := func() {
		interval := DefaultBlockDuration / (4 * 120)
		app := h.Appender(context.Background())
		for i := 0; i < 250; i++ {
			ref, err = app.Append(ref, lbls, lastTs, float64(lastTs))
			lastTs += interval
			if i%10 == 0 {
				require.NoError(t, app.Commit())
				app = h.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	addChunks()

	require.NoError(t, h.Close())
	wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionNone)
	require.NoError(t, err)

	mmapFilePath := filepath.Join(dir, "chunks_head", "000001")
	f, err := os.OpenFile(mmapFilePath, os.O_WRONLY, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 17)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	h, err = NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	addChunks()

	require.NoError(t, h.Close())
}

// Tests https://github.com/prometheus/prometheus/issues/10277.
func TestReplayAfterMmapReplayError(t *testing.T) {
	dir := t.TempDir()
	var h *Head
	var err error

	openHead := func() {
		wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionNone)
		require.NoError(t, err)

		opts := DefaultHeadOptions()
		opts.ChunkRange = DefaultBlockDuration
		opts.ChunkDirRoot = dir
		opts.EnableMemorySnapshotOnShutdown = true
		opts.MaxExemplars.Store(config.DefaultExemplarsConfig.MaxExemplars)

		h, err = NewHead(nil, nil, wal, nil, opts, nil)
		require.NoError(t, err)
		require.NoError(t, h.Init(0))
	}

	openHead()

	itvl := int64(15 * time.Second / time.Millisecond)
	lastTs := int64(0)
	lbls := labels.FromStrings("__name__", "testing", "foo", "bar")
	var expSamples []chunks.Sample
	addSamples := func(numSamples int) {
		app := h.Appender(context.Background())
		var ref storage.SeriesRef
		for i := 0; i < numSamples; i++ {
			ref, err = app.Append(ref, lbls, lastTs, float64(lastTs))
			expSamples = append(expSamples, sample{t: lastTs, f: float64(lastTs)})
			require.NoError(t, err)
			lastTs += itvl
			if i%10 == 0 {
				require.NoError(t, app.Commit())
				app = h.Appender(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	// Creating multiple m-map files.
	for i := 0; i < 5; i++ {
		addSamples(250)
		require.NoError(t, h.Close())
		if i != 4 {
			// Don't open head for the last iteration.
			openHead()
		}
	}

	files, err := os.ReadDir(filepath.Join(dir, "chunks_head"))
	require.Len(t, files, 5)

	// Corrupt a m-map file.
	mmapFilePath := filepath.Join(dir, "chunks_head", "000002")
	f, err := os.OpenFile(mmapFilePath, os.O_WRONLY, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 17)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	openHead()
	h.mmapHeadChunks()

	// There should be less m-map files due to corruption.
	files, err = os.ReadDir(filepath.Join(dir, "chunks_head"))
	require.Len(t, files, 2)

	// Querying should not panic.
	q, err := NewBlockQuerier(h, 0, lastTs)
	require.NoError(t, err)
	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "__name__", "testing"))
	require.Equal(t, map[string][]chunks.Sample{lbls.String(): expSamples}, res)

	require.NoError(t, h.Close())
}

func TestOOOAppendWithNoSeries(t *testing.T) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.OutOfOrderCapMax.Store(30)
	opts.OutOfOrderTimeWindow.Store(120 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})
	require.NoError(t, h.Init(0))

	appendSample := func(lbls labels.Labels, ts int64) {
		app := h.Appender(context.Background())
		_, err := app.Append(0, lbls, ts*time.Minute.Milliseconds(), float64(ts))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	verifyOOOSamples := func(lbls labels.Labels, expSamples int) {
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms)

		require.Nil(t, ms.headChunks)
		require.NotNil(t, ms.ooo.oooHeadChunk)
		require.Equal(t, expSamples, ms.ooo.oooHeadChunk.chunk.NumSamples())
	}

	verifyInOrderSamples := func(lbls labels.Labels, expSamples int) {
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms)

		require.Nil(t, ms.ooo)
		require.NotNil(t, ms.headChunks)
		require.Equal(t, expSamples, ms.headChunks.chunk.NumSamples())
	}

	newLabels := func(idx int) labels.Labels { return labels.FromStrings("foo", strconv.Itoa(idx)) }

	s1 := newLabels(1)
	appendSample(s1, 300) // At 300m.
	verifyInOrderSamples(s1, 1)

	// At 239m, the sample cannot be appended to in-order chunk since it is
	// beyond the minValidTime. So it should go in OOO chunk.
	// Series does not exist for s2 yet.
	s2 := newLabels(2)
	appendSample(s2, 239) // OOO sample.
	verifyOOOSamples(s2, 1)

	// Similar for 180m.
	s3 := newLabels(3)
	appendSample(s3, 180) // OOO sample.
	verifyOOOSamples(s3, 1)

	// Now 179m is too old.
	s4 := newLabels(4)
	app := h.Appender(context.Background())
	_, err = app.Append(0, s4, 179*time.Minute.Milliseconds(), float64(179))
	require.Equal(t, storage.ErrTooOldSample, err)
	require.NoError(t, app.Rollback())
	verifyOOOSamples(s3, 1)

	// Samples still go into in-order chunk for samples within
	// appendable minValidTime.
	s5 := newLabels(5)
	appendSample(s5, 240)
	verifyInOrderSamples(s5, 1)
}

func TestHeadMinOOOTimeUpdate(t *testing.T) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, wlog.CompressionSnappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.OutOfOrderTimeWindow.Store(10 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})
	require.NoError(t, h.Init(0))

	appendSample := func(ts int64) {
		lbls := labels.FromStrings("foo", "bar")
		app := h.Appender(context.Background())
		_, err := app.Append(0, lbls, ts*time.Minute.Milliseconds(), float64(ts))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	appendSample(300) // In-order sample.

	require.Equal(t, int64(math.MaxInt64), h.MinOOOTime())

	appendSample(295) // OOO sample.
	require.Equal(t, 295*time.Minute.Milliseconds(), h.MinOOOTime())

	// Allowed window for OOO is >=290, which is before the earliest ooo sample 295, so it gets set to the lower value.
	require.NoError(t, h.truncateOOO(0, 1))
	require.Equal(t, 290*time.Minute.Milliseconds(), h.MinOOOTime())

	appendSample(310) // In-order sample.
	appendSample(305) // OOO sample.
	require.Equal(t, 290*time.Minute.Milliseconds(), h.MinOOOTime())

	// Now the OOO sample 295 was not gc'ed yet. And allowed window for OOO is now >=300.
	// So the lowest among them, 295, is set as minOOOTime.
	require.NoError(t, h.truncateOOO(0, 2))
	require.Equal(t, 295*time.Minute.Milliseconds(), h.MinOOOTime())
}

func TestGaugeHistogramWALAndChunkHeader(t *testing.T) {
	l := labels.FromStrings("a", "b")
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	ts := int64(0)
	appendHistogram := func(h *histogram.Histogram) {
		ts++
		app := head.Appender(context.Background())
		_, err := app.AppendHistogram(0, l, ts, h.Copy(), nil)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	hists := tsdbutil.GenerateTestGaugeHistograms(5)
	hists[0].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[0])
	appendHistogram(hists[1])
	appendHistogram(hists[2])
	hists[3].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[3])
	appendHistogram(hists[3])
	appendHistogram(hists[4])

	checkHeaders := func() {
		head.mmapHeadChunks()
		ms, _, err := head.getOrCreate(l.Hash(), l)
		require.NoError(t, err)
		require.Len(t, ms.mmappedChunks, 3)
		expHeaders := []chunkenc.CounterResetHeader{
			chunkenc.UnknownCounterReset,
			chunkenc.GaugeType,
			chunkenc.UnknownCounterReset,
			chunkenc.GaugeType,
		}
		for i, mmapChunk := range ms.mmappedChunks {
			chk, err := head.chunkDiskMapper.Chunk(mmapChunk.ref)
			require.NoError(t, err)
			require.Equal(t, expHeaders[i], chk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
		}
		require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
	}
	checkHeaders()

	recs := readTestWAL(t, head.wal.Dir())
	require.Equal(t, []interface{}{
		[]record.RefSeries{
			{
				Ref:    1,
				Labels: labels.FromStrings("a", "b"),
			},
		},
		[]record.RefHistogramSample{{Ref: 1, T: 1, H: hists[0]}},
		[]record.RefHistogramSample{{Ref: 1, T: 2, H: hists[1]}},
		[]record.RefHistogramSample{{Ref: 1, T: 3, H: hists[2]}},
		[]record.RefHistogramSample{{Ref: 1, T: 4, H: hists[3]}},
		[]record.RefHistogramSample{{Ref: 1, T: 5, H: hists[3]}},
		[]record.RefHistogramSample{{Ref: 1, T: 6, H: hists[4]}},
	}, recs)

	// Restart Head without mmap chunks to expect the WAL replay to recognize gauge histograms.
	require.NoError(t, head.Close())
	require.NoError(t, os.RemoveAll(mmappedChunksDir(head.opts.ChunkDirRoot)))

	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
	require.NoError(t, err)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))

	checkHeaders()
}

func TestGaugeFloatHistogramWALAndChunkHeader(t *testing.T) {
	l := labels.FromStrings("a", "b")
	head, _ := newTestHead(t, 1000, wlog.CompressionNone, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	ts := int64(0)
	appendHistogram := func(h *histogram.FloatHistogram) {
		ts++
		app := head.Appender(context.Background())
		_, err := app.AppendHistogram(0, l, ts, nil, h.Copy())
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	hists := tsdbutil.GenerateTestGaugeFloatHistograms(5)
	hists[0].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[0])
	appendHistogram(hists[1])
	appendHistogram(hists[2])
	hists[3].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[3])
	appendHistogram(hists[3])
	appendHistogram(hists[4])

	checkHeaders := func() {
		ms, _, err := head.getOrCreate(l.Hash(), l)
		require.NoError(t, err)
		head.mmapHeadChunks()
		require.Len(t, ms.mmappedChunks, 3)
		expHeaders := []chunkenc.CounterResetHeader{
			chunkenc.UnknownCounterReset,
			chunkenc.GaugeType,
			chunkenc.UnknownCounterReset,
			chunkenc.GaugeType,
		}
		for i, mmapChunk := range ms.mmappedChunks {
			chk, err := head.chunkDiskMapper.Chunk(mmapChunk.ref)
			require.NoError(t, err)
			require.Equal(t, expHeaders[i], chk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
		}
		require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
	}
	checkHeaders()

	recs := readTestWAL(t, head.wal.Dir())
	require.Equal(t, []interface{}{
		[]record.RefSeries{
			{
				Ref:    1,
				Labels: labels.FromStrings("a", "b"),
			},
		},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 1, FH: hists[0]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 2, FH: hists[1]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 3, FH: hists[2]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 4, FH: hists[3]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 5, FH: hists[3]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 6, FH: hists[4]}},
	}, recs)

	// Restart Head without mmap chunks to expect the WAL replay to recognize gauge histograms.
	require.NoError(t, head.Close())
	require.NoError(t, os.RemoveAll(mmappedChunksDir(head.opts.ChunkDirRoot)))

	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
	require.NoError(t, err)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))

	checkHeaders()
}

func TestSnapshotAheadOfWALError(t *testing.T) {
	head, _ := newTestHead(t, 120*4, wlog.CompressionNone, false)
	head.opts.EnableMemorySnapshotOnShutdown = true
	// Add a sample to fill WAL.
	app := head.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 10, 10)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Increment snapshot index to create sufficiently large difference.
	for i := 0; i < 2; i++ {
		_, err = head.wal.NextSegment()
		require.NoError(t, err)
	}
	require.NoError(t, head.Close()) // This will create a snapshot.

	_, idx, _, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.NoError(t, err)
	require.Equal(t, 2, idx)

	// Restart the WAL while keeping the old snapshot. The new head is created manually in this case in order
	// to keep using the same snapshot directory instead of a random one.
	require.NoError(t, os.RemoveAll(head.wal.Dir()))
	head.opts.EnableMemorySnapshotOnShutdown = false
	w, _ := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	// Add a sample to fill WAL.
	app = head.Appender(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 10, 10)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	lastSegment, _, _ := w.LastSegmentAndOffset()
	require.Equal(t, 0, lastSegment)
	require.NoError(t, head.Close())

	// New WAL is saved, but old snapshot still exists.
	_, idx, _, err = LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.NoError(t, err)
	require.Equal(t, 2, idx)

	// Create new Head which should detect the incorrect index and delete the snapshot.
	head.opts.EnableMemorySnapshotOnShutdown = true
	w, _ = wlog.NewSize(nil, nil, head.wal.Dir(), 32768, wlog.CompressionNone)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	// Verify that snapshot directory does not exist anymore.
	_, _, _, err = LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.Equal(t, record.ErrNotFound, err)

	require.NoError(t, head.Close())
}

func BenchmarkCuttingHeadHistogramChunks(b *testing.B) {
	const (
		numSamples = 50000
		numBuckets = 100
	)
	samples := histogram.GenerateBigTestHistograms(numSamples, numBuckets)

	h, _ := newTestHead(b, DefaultBlockDuration, wlog.CompressionNone, false)
	defer func() {
		require.NoError(b, h.Close())
	}()

	a := h.Appender(context.Background())
	ts := time.Now().UnixMilli()
	lbls := labels.FromStrings("foo", "bar")

	b.ResetTimer()

	for _, s := range samples {
		_, err := a.AppendHistogram(0, lbls, ts, s, nil)
		require.NoError(b, err)
	}
}

func TestCuttingNewHeadChunks(t *testing.T) {
	ctx := context.Background()
	testCases := map[string]struct {
		numTotalSamples int
		timestampJitter bool
		floatValFunc    func(i int) float64
		histValFunc     func(i int) *histogram.Histogram
		expectedChks    []struct {
			numSamples int
			numBytes   int
		}
	}{
		"float samples": {
			numTotalSamples: 180,
			floatValFunc: func(i int) float64 {
				return 1.
			},
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{numSamples: 120, numBytes: 46},
				{numSamples: 60, numBytes: 32},
			},
		},
		"large float samples": {
			// Normally 120 samples would fit into a single chunk but these chunks violate the 1005 byte soft cap.
			numTotalSamples: 120,
			timestampJitter: true,
			floatValFunc: func(i int) float64 {
				// Flipping between these two make each sample val take at least 64 bits.
				vals := []float64{math.MaxFloat64, 0x00}
				return vals[i%len(vals)]
			},
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{99, 1008},
				{21, 219},
			},
		},
		"small histograms": {
			numTotalSamples: 240,
			histValFunc: func() func(i int) *histogram.Histogram {
				hists := histogram.GenerateBigTestHistograms(240, 10)
				return func(i int) *histogram.Histogram {
					return hists[i]
				}
			}(),
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{120, 1087},
				{120, 1039},
			},
		},
		"large histograms": {
			numTotalSamples: 240,
			histValFunc: func() func(i int) *histogram.Histogram {
				hists := histogram.GenerateBigTestHistograms(240, 100)
				return func(i int) *histogram.Histogram {
					return hists[i]
				}
			}(),
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{40, 896},
				{40, 899},
				{40, 896},
				{30, 690},
				{30, 691},
				{30, 694},
				{30, 693},
			},
		},
		"really large histograms": {
			// Really large histograms; each chunk can only contain a single histogram but we have a 10 sample minimum
			// per chunk.
			numTotalSamples: 11,
			histValFunc: func() func(i int) *histogram.Histogram {
				hists := histogram.GenerateBigTestHistograms(11, 100000)
				return func(i int) *histogram.Histogram {
					return hists[i]
				}
			}(),
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{10, 200103},
				{1, 87540},
			},
		},
	}
	for testName, tc := range testCases {
		t.Run(testName, func(t *testing.T) {
			h, _ := newTestHead(t, DefaultBlockDuration, wlog.CompressionNone, false)
			defer func() {
				require.NoError(t, h.Close())
			}()

			a := h.Appender(context.Background())

			ts := int64(10000)
			lbls := labels.FromStrings("foo", "bar")
			jitter := []int64{0, 1} // A bit of jitter to prevent dod=0.

			for i := 0; i < tc.numTotalSamples; i++ {
				if tc.floatValFunc != nil {
					_, err := a.Append(0, lbls, ts, tc.floatValFunc(i))
					require.NoError(t, err)
				} else if tc.histValFunc != nil {
					_, err := a.AppendHistogram(0, lbls, ts, tc.histValFunc(i), nil)
					require.NoError(t, err)
				}

				ts += int64(60 * time.Second / time.Millisecond)
				if tc.timestampJitter {
					ts += jitter[i%len(jitter)]
				}
			}

			require.NoError(t, a.Commit())

			idxReader, err := h.Index()
			require.NoError(t, err)

			chkReader, err := h.Chunks()
			require.NoError(t, err)

			p, err := idxReader.Postings(ctx, "foo", "bar")
			require.NoError(t, err)

			var lblBuilder labels.ScratchBuilder

			for p.Next() {
				sRef := p.At()

				chkMetas := make([]chunks.Meta, len(tc.expectedChks))
				require.NoError(t, idxReader.Series(sRef, &lblBuilder, &chkMetas))

				require.Len(t, chkMetas, len(tc.expectedChks))

				for i, expected := range tc.expectedChks {
					chk, iterable, err := chkReader.ChunkOrIterable(chkMetas[i])
					require.NoError(t, err)
					require.Nil(t, iterable)

					require.Equal(t, expected.numSamples, chk.NumSamples())
					require.Len(t, chk.Bytes(), expected.numBytes)
				}
			}
		})
	}
}

// TestHeadDetectsDuplcateSampleAtSizeLimit tests a regression where a duplicate sample
// is appended to the head, right when the head chunk is at the size limit.
// The test adds all samples as duplicate, thus expecting that the result has
// exactly half of the samples.
func TestHeadDetectsDuplicateSampleAtSizeLimit(t *testing.T) {
	numSamples := 1000
	baseTS := int64(1695209650)

	h, _ := newTestHead(t, DefaultBlockDuration, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	a := h.Appender(context.Background())
	var err error
	vals := []float64{math.MaxFloat64, 0x00} // Use the worst case scenario for the XOR encoding. Otherwise we hit the sample limit before the size limit.
	for i := 0; i < numSamples; i++ {
		ts := baseTS + int64(i/2)*10000
		a.Append(0, labels.FromStrings("foo", "bar"), ts, vals[(i/2)%len(vals)])
		err = a.Commit()
		require.NoError(t, err)
		a = h.Appender(context.Background())
	}

	indexReader, err := h.Index()
	require.NoError(t, err)

	var (
		chunks  []chunks.Meta
		builder labels.ScratchBuilder
	)
	require.NoError(t, indexReader.Series(1, &builder, &chunks))

	chunkReader, err := h.Chunks()
	require.NoError(t, err)

	storedSampleCount := 0
	for _, chunkMeta := range chunks {
		chunk, iterable, err := chunkReader.ChunkOrIterable(chunkMeta)
		require.NoError(t, err)
		require.Nil(t, iterable)
		storedSampleCount += chunk.NumSamples()
	}

	require.Equal(t, numSamples/2, storedSampleCount)
}

func TestWALSampleAndExemplarOrder(t *testing.T) {
	lbls := labels.FromStrings("foo", "bar")
	testcases := map[string]struct {
		appendF      func(app storage.Appender, ts int64) (storage.SeriesRef, error)
		expectedType reflect.Type
	}{
		"float sample": {
			appendF: func(app storage.Appender, ts int64) (storage.SeriesRef, error) {
				return app.Append(0, lbls, ts, 1.0)
			},
			expectedType: reflect.TypeOf([]record.RefSample{}),
		},
		"histogram sample": {
			appendF: func(app storage.Appender, ts int64) (storage.SeriesRef, error) {
				return app.AppendHistogram(0, lbls, ts, tsdbutil.GenerateTestHistogram(1), nil)
			},
			expectedType: reflect.TypeOf([]record.RefHistogramSample{}),
		},
		"float histogram sample": {
			appendF: func(app storage.Appender, ts int64) (storage.SeriesRef, error) {
				return app.AppendHistogram(0, lbls, ts, nil, tsdbutil.GenerateTestFloatHistogram(1))
			},
			expectedType: reflect.TypeOf([]record.RefFloatHistogramSample{}),
		},
	}

	for testName, tc := range testcases {
		t.Run(testName, func(t *testing.T) {
			h, w := newTestHead(t, 1000, wlog.CompressionNone, false)
			defer func() {
				require.NoError(t, h.Close())
			}()

			app := h.Appender(context.Background())
			ref, err := tc.appendF(app, 10)
			require.NoError(t, err)
			app.AppendExemplar(ref, lbls, exemplar.Exemplar{Value: 1.0, Ts: 5})

			app.Commit()

			recs := readTestWAL(t, w.Dir())
			require.Len(t, recs, 3)
			_, ok := recs[0].([]record.RefSeries)
			require.True(t, ok, "expected first record to be a RefSeries")
			actualType := reflect.TypeOf(recs[1])
			require.Equal(t, tc.expectedType, actualType, "expected second record to be a %s", tc.expectedType)
			_, ok = recs[2].([]record.RefExemplar)
			require.True(t, ok, "expected third record to be a RefExemplar")
		})
	}
}

// TestHeadCompactionWhileAppendAndCommitExemplar simulates a use case where
// a series is removed from the head while an exemplar is being appended to it.
// This can happen in theory by compacting the head at the right time due to
// a series being idle.
// The test cheats a little bit by not appending a sample with the exemplar.
// If you also add a sample and run Truncate in a concurrent goroutine and run
// the test around a million(!) times, you can get
// `unknown HeadSeriesRef when trying to add exemplar: 1` error on push.
// It is likely that running the test for much longer and with more time variations
// would trigger the
// `signal SIGSEGV: segmentation violation code=0x1 addr=0x20 pc=0xbb03d1`
// panic, that we have seen in the wild once.
func TestHeadCompactionWhileAppendAndCommitExemplar(t *testing.T) {
	h, _ := newTestHead(t, DefaultBlockDuration, wlog.CompressionNone, false)
	app := h.Appender(context.Background())
	lbls := labels.FromStrings("foo", "bar")
	ref, err := app.Append(0, lbls, 1, 1)
	require.NoError(t, err)
	app.Commit()
	// Not adding a sample here to trigger the fault.
	app = h.Appender(context.Background())
	_, err = app.AppendExemplar(ref, lbls, exemplar.Exemplar{Value: 1, Ts: 20})
	require.NoError(t, err)
	h.Truncate(10)
	app.Commit()
	h.Close()
}

func labelsWithHashCollision() (labels.Labels, labels.Labels) {
	// These two series have the same XXHash; thanks to https://github.com/pstibrany/labels_hash_collisions
	ls1 := labels.FromStrings("__name__", "metric", "lbl1", "value", "lbl2", "l6CQ5y")
	ls2 := labels.FromStrings("__name__", "metric", "lbl1", "value", "lbl2", "v7uDlF")

	if ls1.Hash() != ls2.Hash() {
		// These ones are the same when using -tags stringlabels
		ls1 = labels.FromStrings("__name__", "metric", "lbl", "HFnEaGl")
		ls2 = labels.FromStrings("__name__", "metric", "lbl", "RqcXatm")
	}

	if ls1.Hash() != ls2.Hash() {
		panic("This code needs to be updated: find new labels with colliding hash values.")
	}

	return ls1, ls2
}

// stripeSeriesWithCollidingSeries returns a stripeSeries with two memSeries having the same, colliding, hash.
func stripeSeriesWithCollidingSeries(t *testing.T) (*stripeSeries, *memSeries, *memSeries) {
	t.Helper()

	lbls1, lbls2 := labelsWithHashCollision()
	ms1 := memSeries{
		lset: lbls1,
	}
	ms2 := memSeries{
		lset: lbls2,
	}
	hash := lbls1.Hash()
	s := newStripeSeries(1, noopSeriesLifecycleCallback{})

	got, created, err := s.getOrSet(hash, lbls1, func() *memSeries {
		return &ms1
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Same(t, &ms1, got)

	// Add a conflicting series
	got, created, err = s.getOrSet(hash, lbls2, func() *memSeries {
		return &ms2
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Same(t, &ms2, got)

	return s, &ms1, &ms2
}

func TestStripeSeries_getOrSet(t *testing.T) {
	s, ms1, ms2 := stripeSeriesWithCollidingSeries(t)
	hash := ms1.lset.Hash()

	// Verify that we can get both of the series despite the hash collision
	got := s.getByHash(hash, ms1.lset)
	require.Same(t, ms1, got)
	got = s.getByHash(hash, ms2.lset)
	require.Same(t, ms2, got)
}

func TestStripeSeries_gc(t *testing.T) {
	s, ms1, ms2 := stripeSeriesWithCollidingSeries(t)
	hash := ms1.lset.Hash()

	s.gc(0, 0)

	// Verify that we can get neither ms1 nor ms2 after gc-ing corresponding series
	got := s.getByHash(hash, ms1.lset)
	require.Nil(t, got)
	got = s.getByHash(hash, ms2.lset)
	require.Nil(t, got)
}

func TestPostingsCardinalityStats(t *testing.T) {
	head := &Head{postings: index.NewMemPostings()}
	head.postings.Add(1, labels.FromStrings(labels.MetricName, "t", "n", "v1"))
	head.postings.Add(2, labels.FromStrings(labels.MetricName, "t", "n", "v2"))

	statsForMetricName := head.PostingsCardinalityStats(labels.MetricName, 10)
	head.postings.Add(3, labels.FromStrings(labels.MetricName, "t", "n", "v3"))
	// Using cache.
	require.Equal(t, statsForMetricName, head.PostingsCardinalityStats(labels.MetricName, 10))

	statsForSomeLabel := head.PostingsCardinalityStats("n", 10)
	// Cache should be evicted because of the change of label name.
	require.NotEqual(t, statsForMetricName, statsForSomeLabel)
	head.postings.Add(4, labels.FromStrings(labels.MetricName, "t", "n", "v4"))
	// Using cache.
	require.Equal(t, statsForSomeLabel, head.PostingsCardinalityStats("n", 10))
	// Cache should be evicted because of the change of limit parameter.
	statsForSomeLabel1 := head.PostingsCardinalityStats("n", 1)
	require.NotEqual(t, statsForSomeLabel1, statsForSomeLabel)
	// Using cache.
	require.Equal(t, statsForSomeLabel1, head.PostingsCardinalityStats("n", 1))
}

func TestHeadAppender_AppendCTZeroSample(t *testing.T) {
	type appendableSamples struct {
		ts  int64
		val float64
		ct  int64
	}
	for _, tc := range []struct {
		name              string
		appendableSamples []appendableSamples
		expectedSamples   []model.Sample
	}{
		{
			name: "In order ct+normal sample",
			appendableSamples: []appendableSamples{
				{ts: 100, val: 10, ct: 1},
			},
			expectedSamples: []model.Sample{
				{Timestamp: 1, Value: 0},
				{Timestamp: 100, Value: 10},
			},
		},
		{
			name: "Consecutive appends with same ct ignore ct",
			appendableSamples: []appendableSamples{
				{ts: 100, val: 10, ct: 1},
				{ts: 101, val: 10, ct: 1},
			},
			expectedSamples: []model.Sample{
				{Timestamp: 1, Value: 0},
				{Timestamp: 100, Value: 10},
				{Timestamp: 101, Value: 10},
			},
		},
		{
			name: "Consecutive appends with newer ct do not ignore ct",
			appendableSamples: []appendableSamples{
				{ts: 100, val: 10, ct: 1},
				{ts: 102, val: 10, ct: 101},
			},
			expectedSamples: []model.Sample{
				{Timestamp: 1, Value: 0},
				{Timestamp: 100, Value: 10},
				{Timestamp: 101, Value: 0},
				{Timestamp: 102, Value: 10},
			},
		},
		{
			name: "CT equals to previous sample timestamp is ignored",
			appendableSamples: []appendableSamples{
				{ts: 100, val: 10, ct: 1},
				{ts: 101, val: 10, ct: 100},
			},
			expectedSamples: []model.Sample{
				{Timestamp: 1, Value: 0},
				{Timestamp: 100, Value: 10},
				{Timestamp: 101, Value: 10},
			},
		},
	} {
		h, _ := newTestHead(t, DefaultBlockDuration, wlog.CompressionNone, false)
		defer func() {
			require.NoError(t, h.Close())
		}()
		a := h.Appender(context.Background())
		lbls := labels.FromStrings("foo", "bar")
		for _, sample := range tc.appendableSamples {
			_, err := a.AppendCTZeroSample(0, lbls, sample.ts, sample.ct)
			require.NoError(t, err)
			_, err = a.Append(0, lbls, sample.ts, sample.val)
			require.NoError(t, err)
		}
		require.NoError(t, a.Commit())

		q, err := NewBlockQuerier(h, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		require.True(t, ss.Next())
		s := ss.At()
		require.False(t, ss.Next())
		it := s.Iterator(nil)
		for _, sample := range tc.expectedSamples {
			require.Equal(t, chunkenc.ValFloat, it.Next())
			timestamp, value := it.At()
			require.Equal(t, sample.Timestamp, model.Time(timestamp))
			require.Equal(t, sample.Value, model.SampleValue(value))
		}
		require.Equal(t, chunkenc.ValNone, it.Next())
	}
}

func TestHeadCompactableDoesNotCompactEmptyHead(t *testing.T) {
	// Use a chunk range of 1 here so that if we attempted to determine if the head
	// was compactable using default values for min and max times, `Head.compactable()`
	// would return true which is incorrect. This test verifies that we short-circuit
	// the check when the head has not yet had any samples added.
	head, _ := newTestHead(t, 1, wlog.CompressionNone, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	require.False(t, head.compactable())
}

type countSeriesLifecycleCallback struct {
	created atomic.Int64
	deleted atomic.Int64
}

func (c *countSeriesLifecycleCallback) PreCreation(labels.Labels) error { return nil }
func (c *countSeriesLifecycleCallback) PostCreation(labels.Labels)      { c.created.Inc() }
func (c *countSeriesLifecycleCallback) PostDeletion(s map[chunks.HeadSeriesRef]labels.Labels) {
	c.deleted.Add(int64(len(s)))
}
