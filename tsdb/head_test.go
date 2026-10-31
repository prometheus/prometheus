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

package tsdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/testutil"
)

const EncUnsupportedXOR = chunkenc.Encoding(math.MaxUint8)

// newTestHeadDefaultOptions returns the HeadOptions that should be used by default in unit tests.
func newTestHeadDefaultOptions(chunkRange int64, oooEnabled bool) *HeadOptions {
	opts := DefaultHeadOptions()
	opts.ChunkRange = chunkRange
	opts.EnableExemplarStorage = true
	opts.MaxExemplars.Store(config.DefaultExemplarsConfig.MaxExemplars)
	if oooEnabled {
		opts.OutOfOrderTimeWindow.Store(10 * time.Minute.Milliseconds())
	}
	return opts
}

func newTestHead(t testing.TB, chunkRange int64, compressWAL compression.Type, oooEnabled bool) (*Head, *wlog.WL) {
	return newTestHeadWithOptions(t, compressWAL, newTestHeadDefaultOptions(chunkRange, oooEnabled))
}

func newTestHeadWithOptions(t testing.TB, compressWAL compression.Type, opts *HeadOptions) (*Head, *wlog.WL) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compressWAL)
	require.NoError(t, err)

	// Override the chunks dir with the testing one.
	opts.ChunkDirRoot = dir

	h, err := NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Use _ = h.Close() instead of require.NoError because some tests
		// explicitly close the head as part of their test logic (e.g., to
		// restart/reopen the head), and we don't want to fail on double-close.
		_ = h.Close()
		// Clear the captured reference so parallel subtests retained in
		// testing.T.sub do not pin closed Heads via the t.cleanups backing array.
		h = nil
	})

	require.NoError(t, h.chunkDiskMapper.IterateAllChunks(func(chunks.HeadSeriesRef, chunks.ChunkDiskMapperRef, int64, int64, uint16, chunkenc.Encoding, bool) error {
		return nil
	}))

	return h, wal
}

func BenchmarkCreateSeries(b *testing.B) {
	series := genSeries(b.N, 10, 0, 0)
	h, _ := newTestHead(b, 10000, compression.None, false)

	b.ReportAllocs()
	b.ResetTimer()

	for _, s := range series {
		h.getOrCreate(s.Labels().Hash(), s.Labels(), false)
	}
}

func populateTestWL(t testing.TB, w *wlog.WL, recs []any, buf []byte, enableSTStorage bool) []byte {
	enc := record.Encoder{EnableSTStorage: enableSTStorage}
	for _, r := range recs {
		buf = buf[:0]
		switch v := r.(type) {
		case []record.RefSeries:
			buf = enc.Series(v, buf)
		case []record.RefSample:
			buf = enc.Samples(v, buf)
		case []tombstones.Stone:
			buf = enc.Tombstones(v, buf)
		case []record.RefExemplar:
			buf = enc.Exemplars(v, buf)
		case []record.RefHistogramSample:
			buf, _ = enc.HistogramSamples(v, buf)
		case []record.RefFloatHistogramSample:
			buf, _ = enc.FloatHistogramSamples(v, buf)
		case []record.RefMmapMarker:
			buf = enc.MmapMarkers(v, buf)
		case []record.RefMetadata:
			buf = enc.Metadata(v, buf)
		default:
			continue
		}
		require.NoError(t, w.Log(buf))
	}
	return buf
}

func readTestWAL(t testing.TB, dir string) (recs []any) {
	sr, err := wlog.NewSegmentsReader(dir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sr.Close())
	}()

	dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
	r := wlog.NewReader(sr)

	for r.Next() {
		rec := r.Record()

		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			recs = append(recs, series)
		case record.Samples, record.SamplesV2:
			samples, err := dec.Samples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
			samples, err := dec.HistogramSamples(rec, nil)
			require.NoError(t, err)
			recs = append(recs, samples)
		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
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
		case record.MinValidTime:
			// Internal checkpoint bookkeeping, not WAL content callers of readTestWAL care
			// about; see TestReadMinValidTime_* in tsdb/wlog for coverage of this record.
			_, err := dec.MinValidTime(rec)
			require.NoError(t, err)
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
		// histogramSeriesPct is the fraction of series that emit native
		// histogram samples instead of float samples. 0 means all float
		// (the default for existing cases), 1 means all histograms.
		// Histogram series use the last histogramSeriesPct*seriesPerBatch
		// refs in each batch so existing float-only shapes are unaffected.
		histogramSeriesPct float64
		// bucketsPerHistogram is the number of positive buckets written
		// per native histogram sample. Each bucket adds one span entry
		// and one bucket delta to the encoded histogram.
		bucketsPerHistogram int
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
		{ // All-histogram WAL, matching the "In between" float shape.
			// Exercises the native-histogram decode hot path (DecodeHistogram
			// + histogramSamplesV1/V2) which is shared by WAL replay,
			// WAL watcher (remote write), and checkpoint creation.
			// bucketsPerHistogram=8 is representative of a moderately
			// complex exponential histogram seen in practice.
			batches:             10,
			seriesPerBatch:      1000,
			samplesPerSeries:    480,
			histogramSeriesPct:  1.0,
			bucketsPerHistogram: 8,
		},
		{ // Mixed WAL: 50% float series, 50% native histogram series.
			// Models a deployment that is partway through migrating metrics
			// to native histograms.
			batches:             10,
			seriesPerBatch:      1000,
			samplesPerSeries:    480,
			histogramSeriesPct:  0.5,
			bucketsPerHistogram: 8,
		},
	}

	labelsPerSeries := 5
	// Rough estimates of most common % of samples that have an exemplar for each scrape.
	exemplarsPercentages := []float64{0, 0.5, 1, 5}
	lastExemplarsPerSeries := -1
	for _, enableSTStorage := range []bool{false, true} {
		for _, c := range cases {
			missingSeriesPercentages := []float64{0, 0.1}
			for _, missingSeriesPct := range missingSeriesPercentages {
				for _, p := range exemplarsPercentages {
					exemplarsPerSeries := int(math.RoundToEven(float64(c.samplesPerSeries) * p / 100))
					// For tests with low samplesPerSeries we could end up testing with 0 exemplarsPerSeries
					// multiple times without this check.
					if exemplarsPerSeries == lastExemplarsPerSeries {
						continue
					}
					lastExemplarsPerSeries = exemplarsPerSeries
					name := fmt.Sprintf("batches=%d,seriesPerBatch=%d,samplesPerSeries=%d,exemplarsPerSeries=%d,mmappedChunkT=%d,oooSeriesPct=%.3f,oooSamplesPct=%.3f,oooCapMax=%d,missingSeriesPct=%.3f,stStorage=%v", c.batches, c.seriesPerBatch, c.samplesPerSeries, exemplarsPerSeries, c.mmappedChunkT, c.oooSeriesPct, c.oooSamplesPct, c.oooCapMax, missingSeriesPct, enableSTStorage)
					if c.histogramSeriesPct > 0 {
						name += fmt.Sprintf(",histogramSeriesPct=%.3f,bucketsPerHistogram=%d", c.histogramSeriesPct, c.bucketsPerHistogram)
					}
					b.Run(name,
						func(b *testing.B) {
							dir := b.TempDir()

							wal, err := wlog.New(nil, nil, dir, compression.None)
							require.NoError(b, err)
							var wbl *wlog.WL
							if c.oooSeriesPct != 0 {
								wbl, err = wlog.New(nil, nil, dir, compression.None)
								require.NoError(b, err)
							}

							// Write series.
							refSeries := make([]record.RefSeries, 0, c.seriesPerBatch)
							var buf []byte
							builder := labels.NewBuilder(labels.EmptyLabels())
							for j := 1; j < labelsPerSeries; j++ {
								builder.Set(defaultLabelName+strconv.Itoa(j), defaultLabelValue+strconv.Itoa(j))
							}
							for k := 0; k < c.batches; k++ {
								refSeries = refSeries[:0]
								for i := k * c.seriesPerBatch; i < (k+1)*c.seriesPerBatch; i++ {
									builder.Set(defaultLabelName, strconv.Itoa(i))
									refSeries = append(refSeries, record.RefSeries{Ref: chunks.HeadSeriesRef(i) * 101, Labels: builder.Labels()})
								}

								writeSeries := refSeries
								if missingSeriesPct > 0 {
									newWriteSeries := make([]record.RefSeries, 0, int(float64(len(refSeries))*(1.0-missingSeriesPct)))
									keepRatio := 1.0 - missingSeriesPct
									// Keep approximately every 1/keepRatio series.
									for i, s := range refSeries {
										if int(float64(i)*keepRatio) != int(float64(i+1)*keepRatio) {
											newWriteSeries = append(newWriteSeries, s)
										}
									}
									writeSeries = newWriteSeries
								}

								buf = populateTestWL(b, wal, []any{writeSeries}, buf, enableSTStorage)
							}

							// Write samples. Series are split into float and
							// histogram series: the last histogramSeriesPerBatch
							// refs in each batch emit RefHistogramSample records;
							// the rest emit RefSample records. This mirrors how
							// real Prometheus deployments work — a given series is
							// committed to one type.
							histogramSeriesPerBatch := int(float64(c.seriesPerBatch) * c.histogramSeriesPct)
							floatSeriesPerBatch := c.seriesPerBatch - histogramSeriesPerBatch

							refSamples := make([]record.RefSample, 0, floatSeriesPerBatch)
							refHistSamples := make([]record.RefHistogramSample, 0, histogramSeriesPerBatch)

							oooSeriesPerBatch := int(float64(c.seriesPerBatch) * c.oooSeriesPct)
							oooSamplesPerSeries := int(float64(c.samplesPerSeries) * c.oooSamplesPct)

							// Build a reusable histogram template with the configured
							// bucket count. All histogram series share the same shape;
							// only the value (Sum/Count) changes per sample.
							var histTemplate *histogram.Histogram
							if histogramSeriesPerBatch > 0 {
								spans := make([]histogram.Span, c.bucketsPerHistogram)
								for idx := range spans {
									spans[idx] = histogram.Span{Offset: int32(idx), Length: 1}
								}
								buckets := make([]int64, c.bucketsPerHistogram)
								for idx := range buckets {
									buckets[idx] = int64(idx + 1)
								}
								histTemplate = &histogram.Histogram{
									Schema:          1,
									PositiveSpans:   spans,
									PositiveBuckets: buckets,
								}
							}

							for i := 0; i < c.samplesPerSeries; i++ {
								for j := 0; j < c.batches; j++ {
									refSamples = refSamples[:0]
									refHistSamples = refHistSamples[:0]

									// Float series occupy refs [j*seriesPerBatch, j*seriesPerBatch+floatSeriesPerBatch).
									k := j * c.seriesPerBatch
									if i < oooSamplesPerSeries {
										k += oooSeriesPerBatch
									}
									floatEnd := j*c.seriesPerBatch + floatSeriesPerBatch
									for ; k < floatEnd; k++ {
										refSamples = append(refSamples, record.RefSample{
											Ref: chunks.HeadSeriesRef(k) * 101,
											T:   int64(i) * 10,
											V:   float64(i) * 100,
										})
									}
									if len(refSamples) > 0 {
										buf = populateTestWL(b, wal, []any{refSamples}, buf, enableSTStorage)
									}

									// Histogram series occupy refs [j*seriesPerBatch+floatSeriesPerBatch, (j+1)*seriesPerBatch).
									for k = floatEnd; k < (j+1)*c.seriesPerBatch; k++ {
										h := *histTemplate
										h.Count = uint64(i + 1)
										h.Sum = float64(i) * 100
										refHistSamples = append(refHistSamples, record.RefHistogramSample{
											Ref: chunks.HeadSeriesRef(k) * 101,
											T:   int64(i) * 10,
											H:   &h,
										})
									}
									if len(refHistSamples) > 0 {
										buf = populateTestWL(b, wal, []any{refHistSamples}, buf, enableSTStorage)
									}
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
									s := newMemSeries(labels.Labels{}, chunks.HeadSeriesRef(k)*101, 0, defaultIsolationDisabled, false)
									s.append(0, c.mmappedChunkT, 42, 0, cOpts)
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
							for i := range exemplarsPerSeries {
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
									buf = populateTestWL(b, wal, []any{refExemplars}, buf, enableSTStorage)
								}
							}

							// Write OOO samples and mmap markers.
							refMarkers := make([]record.RefMmapMarker, 0, oooSeriesPerBatch)
							refSamples = make([]record.RefSample, 0, oooSeriesPerBatch)
							for i := range oooSamplesPerSeries {
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
										populateTestWL(b, wbl, []any{refMarkers}, buf, enableSTStorage)
									}
									buf = populateTestWL(b, wal, []any{refSamples}, buf, enableSTStorage)
									buf = populateTestWL(b, wbl, []any{refSamples}, buf, enableSTStorage)
								}
							}

							b.ResetTimer()

							// Load the WAL.
							for b.Loop() {
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
	}
}

// Recommended CLI invocation:
//
//	export bench=commitMetadata && go test ./tsdb \
//		-run '^$' -bench '^BenchmarkCommitMetadata' \
//		-benchtime 1s -count 6 -cpu 2 -timeout 999m -benchmem \
//		| tee ${bench}.txt
func BenchmarkCommitMetadata(b *testing.B) {
	const numSeries = 1000
	for _, numFamilies := range []int{10, 100} {
		b.Run(fmt.Sprintf("series=%d/families=%d", numSeries, numFamilies), func(b *testing.B) {
			helps := make([]string, numFamilies)
			units := make([]string, numFamilies)
			for f := range numFamilies {
				helps[f] = fmt.Sprintf("Total time spent in subsystem %d across all worker goroutines in seconds.", f)
				if f%2 == 0 {
					units[f] = fmt.Sprintf("seconds_%d", f)
				}
			}

			batch := &appendBatch{
				metadata:       make([]record.RefMetadata, numSeries),
				metadataSeries: make([]*memSeries, numSeries),
			}
			for i := range numSeries {
				fam := i % numFamilies
				batch.metadataSeries[i] = &memSeries{ref: chunks.HeadSeriesRef(i + 1)}
				batch.metadata[i] = record.RefMetadata{
					Ref:  chunks.HeadSeriesRef(i + 1),
					Type: uint8(record.Counter),
					Unit: units[fam],
					Help: helps[fam],
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				commitMetadata(batch)
			}
		})
	}
}

// BenchmarkLoadRealWLs will be skipped unless the BENCHMARK_LOAD_REAL_WLS_DIR environment variable is set.
// BENCHMARK_LOAD_REAL_WLS_DIR should be the folder where `wal` and `chunks_head` are located.
//
// Using an absolute path for BENCHMARK_LOAD_REAL_WLS_DIR is recommended.
//
// Because WLs loading may alter BENCHMARK_LOAD_REAL_WLS_DIR which can affect benchmark results and to ensure consistency,
// a copy of BENCHMARK_LOAD_REAL_WLS_DIR is made for each iteration and deleted at the end.
// Make sure there is sufficient disk space for that.
func BenchmarkLoadRealWLs(b *testing.B) {
	srcDir := os.Getenv("BENCHMARK_LOAD_REAL_WLS_DIR")
	if srcDir == "" {
		b.SkipNow()
	}

	// Load the WAL.
	for b.Loop() {
		b.StopTimer()
		dir := b.TempDir()
		require.NoError(b, fileutil.CopyDirs(srcDir, dir))

		wal, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(b, err)
		b.Cleanup(func() { wal.Close() })

		wbl, err := wlog.New(nil, nil, filepath.Join(dir, "wbl"), compression.None)
		require.NoError(b, err)
		b.Cleanup(func() { wbl.Close() })
		b.StartTimer()

		opts := DefaultHeadOptions()
		opts.ChunkDirRoot = dir
		h, err := NewHead(nil, nil, wal, wbl, opts, nil)
		require.NoError(b, err)
		require.NoError(b, h.Init(0))

		b.StopTimer()
		require.NoError(b, os.RemoveAll(dir))
	}
}

// TestHead_InitAppenderRace_ErrOutOfBounds tests against init races with maxTime vs minTime on empty head concurrent appends.
// See: https://github.com/prometheus/prometheus/pull/17963
func TestHead_InitAppenderRace_ErrOutOfBounds(t *testing.T) {
	head, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
	require.NoError(t, head.Init(0))

	ts := timestamp.FromTime(time.Now())
	appendCycles := 100

	g, ctx := errgroup.WithContext(t.Context())
	var wg sync.WaitGroup
	wg.Add(1)

	for i := range 100 {
		g.Go(func() error {
			appends := 0
			wg.Wait()
			for ctx.Err() == nil && appends < appendCycles {
				appends++
				app := head.AppenderV2(t.Context())
				if _, err := app.Append(0, labels.FromStrings("__name__", strconv.Itoa(i)), 0, ts, float64(ts), nil, nil, storage.AOptions{}); err != nil {
					return fmt.Errorf("error when appending to head: %w", err)
				}
				if err := app.Rollback(); err != nil {
					return err
				}
			}
			return nil
		})
	}
	wg.Done()
	require.NoError(t, g.Wait())
}

// TestHead_HighConcurrencyReadAndWrite generates 1000 series with a step of 15s and fills a whole block with samples,
// this means in total it generates 4000 chunks because with a step of 15s there are 4 chunks per block per series.
// While appending the samples to the head it concurrently queries them from multiple go routines and verifies that the
// returned results are correct.
func TestHead_HighConcurrencyReadAndWrite(t *testing.T) {
	t.Parallel()
	head, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)

	seriesCnt := 1000
	readConcurrency := 2
	writeConcurrency := 10
	startTs := uint64(DefaultBlockDuration) // Start at the second block relative to the unix epoch.
	qryRange := uint64(5 * time.Minute.Milliseconds())
	step := uint64(15 * time.Second / time.Millisecond)
	endTs := startTs + uint64(DefaultBlockDuration)

	labelSets := make([]labels.Labels, seriesCnt)
	for i := range seriesCnt {
		labelSets[i] = labels.FromStrings("seriesId", strconv.Itoa(i))
	}
	require.NoError(t, head.Init(0))

	g, ctx := errgroup.WithContext(t.Context())
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
	for wid := range writeConcurrency {
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

				app := head.AppenderV2(ctx)
				for i := range workerLabelSets {
					// We also use the timestamp as the sample value.
					if _, err := app.Append(0, workerLabelSets[i], 0, int64(ts), float64(ts), nil, nil, storage.AOptions{}); err != nil {
						return false, fmt.Errorf("error when appending (V2) to head: %w", err)
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
	for wid := range readConcurrency {
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
	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range []compression.Type{compression.None, compression.Snappy, compression.Zstd} {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				entries := []any{
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
						// Sample for duplicate series record.
						{Ref: 101, T: 101, V: 7},
					},
					[]tombstones.Stone{
						{Ref: 0, Intervals: []tombstones.Interval{{Mint: 99, Maxt: 101}}},
						// Tombstone for duplicate series record.
						{Ref: 101, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 100}}},
					},
					[]record.RefExemplar{
						{Ref: 10, T: 100, V: 1, Labels: labels.FromStrings("trace_id", "asdf")},
						// Exemplar for duplicate series record.
						{Ref: 101, T: 101, V: 7, Labels: labels.FromStrings("trace_id", "zxcv")},
					},
					[]record.RefMetadata{
						// Metadata for duplicate series record.
						{Ref: 101, Type: uint8(record.Counter), Unit: "foo", Help: "total foo"},
					},
				}

				head, w := newTestHead(t, 1000, compress, false)

				populateTestWL(t, w, entries, nil, enableSTStorage)

				require.NoError(t, head.Init(math.MinInt64))
				require.Equal(t, uint64(101), head.lastSeriesID.Load())

				s10 := head.series.getByID(10)
				s11 := head.series.getByID(11)
				s50 := head.series.getByID(50)
				s100 := head.series.getByID(100)
				s101 := head.series.getByID(101)

				testutil.RequireEqual(t, labels.FromStrings("a", "1"), s10.lset)
				require.Nil(t, s11) // Series without samples should be garbage collected at head.Init().
				testutil.RequireEqual(t, labels.FromStrings("a", "4"), s50.lset)
				testutil.RequireEqual(t, labels.FromStrings("a", "3"), s100.lset)

				// Duplicate series record should not be written to the head.
				require.Nil(t, s101)
				// But it should have a WAL expiry set.
				keepUntil, ok := head.getWALExpiry(101)
				require.True(t, ok)
				require.Equal(t, int64(101), keepUntil)
				// Only the duplicate series record should have a WAL expiry set.
				_, ok = head.getWALExpiry(50)
				require.False(t, ok)

				expandChunk := func(c chunkenc.Iterator) (x []sample) {
					for c.Next() == chunkenc.ValFloat {
						t, v := c.At()
						x = append(x, sample{t: t, f: v})
					}
					require.NoError(t, c.Err())
					return x
				}

				// Verify samples and exemplar for series 10.
				c, _, _, err := s10.chunk(0, head.chunkDiskMapper, &head.memChunkPool, nil)
				require.NoError(t, err)
				require.Equal(t, []sample{{0, 100, 2, nil, nil}, {0, 101, 5, nil, nil}}, expandChunk(c.chunk.Iterator(nil)))

				q, err := head.ExemplarQuerier(context.Background())
				require.NoError(t, err)
				e, err := q.Select(0, 1000, []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "1")})
				require.NoError(t, err)
				require.NotEmpty(t, e)
				require.NotEmpty(t, e[0].Exemplars)
				require.True(t, exemplar.Exemplar{Ts: 100, Value: 1, Labels: labels.FromStrings("trace_id", "asdf")}.Equals(e[0].Exemplars[0]))

				// Verify samples for series 50
				c, _, _, err = s50.chunk(0, head.chunkDiskMapper, &head.memChunkPool, nil)
				require.NoError(t, err)
				require.Equal(t, []sample{{0, 101, 6, nil, nil}}, expandChunk(c.chunk.Iterator(nil)))

				// Verify records for series 100 and its duplicate, series 101.
				// The samples before the new series record should be discarded since a duplicate record
				// is only possible when old samples were compacted.
				c, _, _, err = s100.chunk(0, head.chunkDiskMapper, &head.memChunkPool, nil)
				require.NoError(t, err)
				require.Equal(t, []sample{{0, 101, 7, nil, nil}}, expandChunk(c.chunk.Iterator(nil)))

				q, err = head.ExemplarQuerier(context.Background())
				require.NoError(t, err)
				e, err = q.Select(0, 1000, []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "3")})
				require.NoError(t, err)
				require.NotEmpty(t, e)
				require.NotEmpty(t, e[0].Exemplars)
				require.True(t, exemplar.Exemplar{Ts: 101, Value: 7, Labels: labels.FromStrings("trace_id", "zxcv")}.Equals(e[0].Exemplars[0]))

				require.False(t, metadata.IsZeroHandle(s100.meta))
				require.Equal(t, "foo", s100.meta.Value().Unit)
				require.Equal(t, "total foo", s100.meta.Value().Help)

				intervals, err := head.tombstones.Get(storage.SeriesRef(s100.ref))
				require.NoError(t, err)
				require.Equal(t, tombstones.Intervals{{Mint: 0, Maxt: 100}}, intervals)
			})
		}
	}
}

// TestHead_WALMultiRef_StaleDeletion_ChunkGaugeNotNegative is a regression test
// for https://github.com/prometheus/prometheus/issues/10884.
//
// When a stale series is deleted via truncateStaleSeries (which writes a
// [MinInt64, MaxInt64] tombstone to the WAL) and then re-created under the same
// labels (producing a new WAL ref), the WAL replay processes for the shared
// processor shard look like:
//
//	reset(mSeries, oldRef_chunks, oldRef)
//	deleteSeriesByID(oldRef)           ← removes chunks from gauge
//	reset(mSeries, newRef_chunks, newRef) ← was double-subtracting old chunks
//
// Without the fix, deleteSeriesByID removes M chunks from the gauge but leaves
// series.mmappedChunks non-nil. The subsequent reset then subtracts those M
// chunks a second time, driving prometheus_tsdb_head_chunks negative when M
// exceeds the new series' chunk count.
func TestHead_WALMultiRef_StaleDeletion_ChunkGaugeNotNegative(t *testing.T) {
	head, w := newTestHead(t, 1000, compression.None, false)
	require.NoError(t, head.Init(0))

	lset := labels.FromStrings("foo", "bar")
	appendSample := func(ts int64, v float64) storage.SeriesRef {
		app := head.AppenderV2(context.Background())
		ref, err := app.Append(0, lset, 0, ts, v, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		return ref
	}

	// Append samples across chunk boundaries to give ref1 3 m-mapped chunks + 1 active head chunk.
	ref1 := appendSample(100, 1) // nextAt=1000
	appendSample(1200, 2)        // ≥1000 → chunk 2, nextAt=2000
	appendSample(2300, 3)        // ≥2000 → chunk 3, nextAt=3000
	appendSample(3400, 4)        // ≥3000 → chunk 4 (active head)

	// Mark ref1 as stale.
	appendSample(3500, math.Float64frombits(value.StaleNaN))

	// Truncate stale series: removes ref1 from the head and writes a
	// [MinInt64, MaxInt64] tombstone record to the WAL.
	staleRefs := []storage.SeriesRef{ref1}
	require.NoError(t, head.truncateStaleSeries(staleRefs, 3500, head.snapshotFingerprints(staleRefs, math.MaxUint64)))

	// Append a single sample with the same labels to create ref2.
	// Ref2 has 0 m-mapped chunks, fewer than ref1's 3.
	ref2 := appendSample(5000, 5)
	require.NotEqual(t, ref1, ref2, "refs must differ after stale truncation and recreation")

	require.NoError(t, head.Close())

	// Reopen the head to trigger WAL replay. The WAL contains (in order):
	//   series(ref1) → tombstone(ref1, [MinInt64,MaxInt64]) → series(ref2) → sample(ref2)
	// Without the fix, replay drives prometheus_tsdb_head_chunks negative.
	w, err := wlog.New(nil, nil, w.Dir(), compression.None)
	require.NoError(t, err)
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = head.opts.ChunkDirRoot
	head, err = NewHead(nil, nil, w, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))
	defer func() {
		require.NoError(t, head.Close())
	}()

	chunksGauge := prom_testutil.ToFloat64(head.metrics.chunks)
	require.GreaterOrEqual(t, chunksGauge, 0.0, "prometheus_tsdb_head_chunks gauge must not be negative after WAL replay")
}

func TestHead_WALCheckpointMultiRef(t *testing.T) {
	cases := []struct {
		name               string
		walEntries         []any
		expectedWalExpiry  int64
		walTruncateMinT    int64
		expectedWalEntries []any
	}{
		{
			name: "Samples only; keep needed duplicate series record",
			walEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{
					{Ref: 1, T: 100, V: 1},
					{Ref: 2, T: 200, V: 2},
					{Ref: 2, T: 500, V: 3},
				},
			},
			expectedWalExpiry: 500,
			walTruncateMinT:   500,
			expectedWalEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{
					{Ref: 2, T: 500, V: 3},
				},
			},
		},
		{
			name: "Tombstones only; keep needed duplicate series record",
			walEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]tombstones.Stone{
					{Ref: 1, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 100}}},
					{Ref: 2, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 200}}},
					{Ref: 2, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 500}}},
				},
			},
			expectedWalExpiry: 500,
			walTruncateMinT:   500,
			expectedWalEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]tombstones.Stone{
					{Ref: 2, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 500}}},
				},
			},
		},
		{
			name: "Exemplars only; keep needed duplicate series record",
			walEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefExemplar{
					{Ref: 1, T: 100, V: 1, Labels: labels.FromStrings("trace_id", "asdf")},
					{Ref: 2, T: 200, V: 2, Labels: labels.FromStrings("trace_id", "asdf")},
					{Ref: 2, T: 500, V: 3, Labels: labels.FromStrings("trace_id", "asdf")},
				},
			},
			expectedWalExpiry: 500,
			walTruncateMinT:   500,
			expectedWalEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefExemplar{
					{Ref: 2, T: 500, V: 3, Labels: labels.FromStrings("trace_id", "asdf")},
				},
			},
		},
		{
			name: "Histograms only; keep needed duplicate series record",
			walEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefHistogramSample{
					{Ref: 1, T: 100, H: &histogram.Histogram{}},
					{Ref: 2, T: 200, H: &histogram.Histogram{}},
					{Ref: 2, T: 500, H: &histogram.Histogram{}},
				},
			},
			expectedWalExpiry: 500,
			walTruncateMinT:   500,
			expectedWalEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: 500, H: &histogram.Histogram{}},
				},
			},
		},
		{
			name: "Float histograms only; keep needed duplicate series record",
			walEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 1, T: 100, FH: &histogram.FloatHistogram{}},
					{Ref: 2, T: 200, FH: &histogram.FloatHistogram{}},
					{Ref: 2, T: 500, FH: &histogram.FloatHistogram{}},
				},
			},
			expectedWalExpiry: 500,
			walTruncateMinT:   500,
			expectedWalEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: 500, FH: &histogram.FloatHistogram{}},
				},
			},
		},
		{
			name: "All record types; keep needed duplicate series record until last record",
			// Series with 2 refs and samples for both
			walEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{
					{Ref: 2, T: 500, V: 3},
				},
				[]tombstones.Stone{
					{Ref: 2, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 500}}},
				},
				[]record.RefExemplar{
					{Ref: 2, T: 800, V: 2, Labels: labels.FromStrings("trace_id", "asdf")},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: 500, H: &histogram.Histogram{}},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: 500, FH: &histogram.FloatHistogram{}},
				},
			},
			expectedWalExpiry: 800,
			walTruncateMinT:   700,
			expectedWalEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefExemplar{
					{Ref: 2, T: 800, V: 2, Labels: labels.FromStrings("trace_id", "asdf")},
				},
			},
		},
		{
			name: "All record types; drop expired duplicate series record",
			// Series with 2 refs and samples for both
			walEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
					{Ref: 2, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{
					{Ref: 2, T: 500, V: 2},
					{Ref: 1, T: 900, V: 3},
				},
				[]tombstones.Stone{
					{Ref: 2, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 750}}},
				},
				[]record.RefExemplar{
					{Ref: 2, T: 800, V: 2, Labels: labels.FromStrings("trace_id", "asdf")},
				},
				[]record.RefHistogramSample{
					{Ref: 2, T: 600, H: &histogram.Histogram{}},
				},
				[]record.RefFloatHistogramSample{
					{Ref: 2, T: 700, FH: &histogram.FloatHistogram{}},
				},
			},
			expectedWalExpiry: 800,
			walTruncateMinT:   900,
			expectedWalEntries: []any{
				[]record.RefSeries{
					{Ref: 1, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{
					{Ref: 1, T: 900, V: 3},
				},
			},
		},
	}

	for _, enableSTStorage := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.name+",stStorage="+strconv.FormatBool(enableSTStorage), func(t *testing.T) {
				h, w := newTestHead(t, 1000, compression.None, false)
				populateTestWL(t, w, tc.walEntries, nil, enableSTStorage)
				first, _, err := wlog.Segments(w.Dir())
				require.NoError(t, err)

				require.NoError(t, h.Init(0))

				keepUntil, ok := h.getWALExpiry(2)
				require.True(t, ok)
				require.Equal(t, tc.expectedWalExpiry, keepUntil)

				// Each truncation creates a new segment, so attempt truncations until a checkpoint is created
				for {
					h.lastWALTruncationTime.Store(0) // Reset so that it's always time to truncate the WAL
					err := h.truncateWAL(tc.walTruncateMinT)
					require.NoError(t, err)
					f, _, err := wlog.Segments(w.Dir())
					require.NoError(t, err)
					if f > first {
						break
					}
				}

				// Read test WAL , checkpoint first
				checkpointDir, _, err := wlog.LastCheckpoint(w.Dir())
				require.NoError(t, err)
				cprecs := readTestWAL(t, checkpointDir)
				recs := readTestWAL(t, w.Dir())
				recs = append(cprecs, recs...)

				// Use testutil.RequireEqual which handles labels properly with dedupelabels
				testutil.RequireEqual(t, tc.expectedWalEntries, recs)
			})
		}
	}
}

func TestHead_KeepSeriesInWALCheckpoint(t *testing.T) {
	existingRef := 1
	existingLbls := labels.FromStrings("foo", "bar")
	keepUntil := int64(10)

	cases := []struct {
		name     string
		prepare  func(t *testing.T, h *Head)
		mint     int64
		expected bool
	}{
		{
			name: "keep series still in the head",
			prepare: func(t *testing.T, h *Head) {
				_, _, err := h.getOrCreateWithOptionalID(chunks.HeadSeriesRef(existingRef), existingLbls.Hash(), existingLbls, false)
				require.NoError(t, err)
			},
			expected: true,
		},
		{
			name:     "keep series with keepUntil > mint",
			mint:     keepUntil - 1,
			expected: true,
		},
		{
			name:     "keep series with keepUntil = mint",
			mint:     keepUntil,
			expected: true,
		},
		{
			name:     "drop series with keepUntil < mint",
			mint:     keepUntil + 1,
			expected: false,
		},
		{
			// The WBL addresses this series through a reference that WAL replay
			// mapped onto another one, so mint cannot express how long the
			// record is needed.
			name: "keep series that the WBL references, past mint",
			prepare: func(_ *testing.T, h *Head) {
				h.pinWBLSeriesRefs(map[chunks.HeadSeriesRef]struct{}{chunks.HeadSeriesRef(existingRef): {}})
			},
			mint:     keepUntil + 1,
			expected: true,
		},
		{
			name: "drop series once the WBL reference is released",
			prepare: func(_ *testing.T, h *Head) {
				h.pinWBLSeriesRefs(map[chunks.HeadSeriesRef]struct{}{chunks.HeadSeriesRef(existingRef): {}})
				h.releaseWBLPinnedSeriesRefs()
			},
			mint:     keepUntil + 1,
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHead(t, 1000, compression.None, false)

			if tc.prepare != nil {
				tc.prepare(t, h)
			} else {
				h.updateWALExpiry(chunks.HeadSeriesRef(existingRef), keepUntil)
			}

			keep := h.keepSeriesInWALCheckpointFn(tc.mint)
			require.Equal(t, tc.expected, keep(chunks.HeadSeriesRef(existingRef)))
		})
	}
}

func TestHead_RaceBetweenExistingSeriesAppendAndGC(t *testing.T) {
	t.Parallel()
	// Unlike TestHead_RaceBetweenSeriesCreationAndGC above, this covers appends to a
	// series that already exists in the head: Append() looks the series up, releases the
	// stripe lock and only afterwards locks the series to mark it as pending commit. A
	// gc() in between removes the series from the head, so the samples appended to it
	// afterwards are lost even though Append() and Commit() report success.
	// See https://github.com/prometheus/prometheus/issues/8365.
	const (
		totalSeries = 10_000
		rounds      = 40
		appenders   = 16
		firstTs     = int64(1_000_000)
		tsStep      = int64(10_000)
	)

	head, _ := newTestHead(t, 1000, compression.None, false)
	require.NoError(t, head.Init(0))

	ctx := context.Background()
	series := make([]labels.Labels, totalSeries)
	for i := range series {
		series[i] = labels.FromStrings("foo", strconv.Itoa(i))
	}
	refs := make([]storage.SeriesRef, totalSeries)

	app := head.AppenderV2(ctx)
	for i := range series {
		ref, err := app.Append(0, series[i], 0, firstTs, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		refs[i] = ref
	}
	require.NoError(t, app.Commit())
	require.Equal(t, totalSeries, int(head.NumSeries()))

	for round := int64(1); round <= rounds; round++ {
		ts := firstTs + round*tsStep

		// Emulate the head truncation that makes existing series collectable in
		// production (see Head.truncateMemory): every series only holds samples from
		// the previous round, so gc() drops their chunks and then the series
		// themselves - unless an appender marked them as pending commit in time.
		head.minTime.Store(ts)
		head.minValidTime.Store(ts)

		done := atomic.NewBool(false)
		var wg sync.WaitGroup
		for a := range appenders {
			wg.Add(1)
			go func(a int) {
				defer wg.Done()
				app := head.AppenderV2(ctx)
				defer func() {
					if err := app.Commit(); err != nil {
						t.Errorf("Failed to commit: %v", err)
					}
				}()
				for i := a; i < totalSeries; i += appenders {
					// Append by reference, like the scrape loop does for series it has seen before.
					ref, err := app.Append(refs[i], series[i], 0, ts, 1, nil, nil, storage.AOptions{})
					if err != nil {
						t.Errorf("Failed to append: %v", err)
						return
					}
					refs[i] = ref
				}
			}(a)
		}
		go func() {
			wg.Wait()
			done.Store(true)
		}()

		// Don't check the atomic.Bool on all iterations in order to perform more gc iterations and make the race condition more likely.
		for i := 1; i%4 != 0 || !done.Load(); i++ {
			head.gc()
		}

		// Every Append() reported success, so no sample may have been dropped: each
		// series is either the one that was looked up or the one that replaced it after
		// being collected, but it has to be in the head, holding the sample.
		require.Equal(t, totalSeries, int(head.NumSeries()), "round %d", round)
	}

	for _, lset := range series {
		s := head.series.getByHash(lset.Hash(), lset)
		require.NotNil(t, s, "series %s is not in the head", lset)
		require.NotNil(t, s.headChunks, "series %s holds no sample", lset)
	}
}

func TestHead_UnknownWALRecord(t *testing.T) {
	head, w := newTestHead(t, 1000, compression.None, false)
	w.Log([]byte{255, 42})
	require.NoError(t, head.Init(0))
}

// BenchmarkHead_Truncate is quite heavy, so consider running it with
// -benchtime=10x or similar to get more stable and comparable results.
func BenchmarkHead_Truncate(b *testing.B) {
	const total = 1e6

	prepare := func(b *testing.B, churn int) *Head {
		h, _ := newTestHead(b, 1000, compression.None, false)

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
		for i := range int(total) {
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
			s, _, _ := h.getOrCreate(allSeries[i].Hash(), allSeries[i], false)
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

			for i := 0; b.Loop(); i++ {
				require.NoError(b, h.Truncate(1000*int64(i)))
				// Make sure the benchmark is meaningful and it's actually truncating the expected amount of series.
				require.Equal(b, total-churn*i, int(h.NumSeries()))
			}
		})
	}
}

func TestHead_Truncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)

	h.initTime(0)

	ctx := context.Background()

	s1, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1", "b", "1"), false)
	s2, _, _ := h.getOrCreate(2, labels.FromStrings("a", "2", "b", "1"), false)
	s3, _, _ := h.getOrCreate(3, labels.FromStrings("a", "1", "b", "2"), false)
	s4, _, _ := h.getOrCreate(4, labels.FromStrings("a", "2", "b", "2", "c", "1"), false)

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

	postingsA1, _ := index.ExpandPostings(h.postings.Postings(ctx, "a", "1"))
	postingsA2, _ := index.ExpandPostings(h.postings.Postings(ctx, "a", "2"))
	postingsB1, _ := index.ExpandPostings(h.postings.Postings(ctx, "b", "1"))
	postingsB2, _ := index.ExpandPostings(h.postings.Postings(ctx, "b", "2"))
	postingsC1, _ := index.ExpandPostings(h.postings.Postings(ctx, "c", "1"))
	postingsAll, _ := index.ExpandPostings(h.postings.Postings(ctx, "", ""))

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
		for _, value := range h.postings.LabelValues(ctx, name, nil) {
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
		New: func() any {
			return &memChunk{}
		},
	}

	s := newMemSeries(labels.FromStrings("a", "b"), 1, 0, defaultIsolationDisabled, false)

	for i := 0; i < 4000; i += 5 {
		ok, _ := s.append(0, int64(i), float64(i), 0, cOpts)
		require.True(t, ok, "sample append failed")
	}
	s.mmapChunks(chunkDiskMapper)

	// Check that truncate removes half of the chunks and afterwards
	// that the ID of the last chunk still gives us the same chunk afterwards.
	countBefore := len(s.mmappedChunks) + 1 // +1 for the head chunk.
	lastID := s.headChunkID(countBefore - 1)
	lastChunk, _, _, err := s.chunk(lastID, chunkDiskMapper, &memChunkPool, nil)
	require.NoError(t, err)
	require.NotNil(t, lastChunk)

	chk, _, _, err := s.chunk(0, chunkDiskMapper, &memChunkPool, nil)
	require.NotNil(t, chk)
	require.NoError(t, err)

	s.truncateChunksBefore(2000, 0)

	require.Equal(t, int64(2000), s.mmappedChunks[0].minTime)
	_, _, _, err = s.chunk(0, chunkDiskMapper, &memChunkPool, nil)
	require.Equal(t, storage.ErrNotFound, err, "first chunks not gone")
	require.Equal(t, countBefore/2, len(s.mmappedChunks)+1) // +1 for the head chunk.
	chk, _, _, err = s.chunk(lastID, chunkDiskMapper, &memChunkPool, nil)
	require.NoError(t, err)
	require.Equal(t, lastChunk, chk)
}

func TestMemSeries_truncateChunks_scenarios(t *testing.T) {
	const chunkRange = 100
	const chunkStep = 5

	tests := []struct {
		name                 string
		headChunks           int                // the number of head chunks to create on memSeries by appending enough samples
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

			series := newMemSeries(labels.EmptyLabels(), 1, 0, true, false)

			cOpts := chunkOpts{
				chunkDiskMapper: chunkDiskMapper,
				chunkRange:      chunkRange,
				samplesPerChunk: DefaultSamplesPerChunk,
			}

			var headStart int
			if tc.mmappedChunks > 0 {
				headStart = (tc.mmappedChunks + 1) * chunkRange
				for i := 0; i < (tc.mmappedChunks+1)*chunkRange; i += chunkStep {
					ok, _ := series.append(0, int64(i), float64(i), 0, cOpts)
					require.True(t, ok, "sample append failed")
				}
				series.mmapChunks(chunkDiskMapper)
			}

			if tc.headChunks == 0 {
				series.setHeadChunks(nil, 0)
			} else {
				for i := headStart; i < chunkRange*(tc.mmappedChunks+tc.headChunks); i += chunkStep {
					ok, _ := series.append(0, int64(i), float64(i), 0, cOpts)
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

			// Set headChunkCount before truncation (series.append bypasses observeChunkCreated).
			series.headChunkCount.Store(uint32(tc.headChunks))

			truncated := series.truncateChunksBefore(tc.truncateBefore, 0)
			require.Equal(t, tc.expectedTruncated, truncated, "wrong number of truncated chunks returned")
			require.Equal(t, uint32(tc.expectedHead), series.headChunkCount.Load(), "wrong headChunkCount after truncation")

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

	t.Run("head chunk count mismatch", func(t *testing.T) {
		// truncateChunksBefore must advance firstChunkID by the actual number of
		// removed chunks, measured from the list itself, even if headChunkCount
		// drifted from the real list length.
		s := &memSeries{ref: 1}
		s.setHeadChunks(buildHeadChunksLight(5), 5)
		s.headChunkCount.Store(7) // Drifted: the list has 5 chunks.

		// Chunks span [0,999]..[4000,4999]; mint=2000 drops the two oldest.
		removed := s.truncateChunksBefore(2000, 0)
		require.Equal(t, 2, removed)
		require.Equal(t, chunks.HeadChunkID(2), s.firstChunkID)
		require.Equal(t, 3, s.headChunks.len())
		require.Equal(t, uint32(3), s.headChunkCount.Load(), "truncation re-syncs the counter from the walk")
	})
}

func TestOOOTruncateChunksBefore_Wrap(t *testing.T) {
	tests := []struct {
		name               string
		firstOOOChunkID    chunks.HeadChunkID
		numOOOChunks       int
		hasOOOHeadChunk    bool
		truncateN          int
		expFirstOOOChunkID chunks.HeadChunkID
		expOOONil          bool
		expOOOMmappedLen   int
	}{
		{
			name:               "wrap past zero with remaining chunks",
			firstOOOChunkID:    chunks.HeadChunkID(oooChunkIDMask - 3),
			numOOOChunks:       5,
			truncateN:          4,
			expFirstOOOChunkID: 1,
			expOOOMmappedLen:   1,
		},
		{
			name:            "truncate all OOO chunks, no head chunk, ooo becomes nil",
			firstOOOChunkID: chunks.HeadChunkID(oooChunkIDMask - 3),
			numOOOChunks:    5,
			truncateN:       5,
			expOOONil:       true,
		},
		{
			name:               "truncate all mmapped, head chunk keeps ooo alive",
			firstOOOChunkID:    chunks.HeadChunkID(oooChunkIDMask - 1),
			numOOOChunks:       3,
			hasOOOHeadChunk:    true,
			truncateN:          3,
			expFirstOOOChunkID: 2,
			expOOOMmappedLen:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			chunkDiskMapper, err := chunks.NewChunkDiskMapper(nil, dir, chunkenc.NewPool(), chunks.DefaultWriteBufferSize, chunks.DefaultWriteQueueSize)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, chunkDiskMapper.Close()) })

			series := newMemSeries(labels.EmptyLabels(), 1, 0, true, false)
			series.ooo = &memSeriesOOOFields{firstOOOChunkID: tc.firstOOOChunkID}

			refs := make([]chunks.ChunkDiskMapperRef, tc.numOOOChunks)
			for i := 0; i < tc.numOOOChunks; i++ {
				ref := chunkDiskMapper.WriteChunk(series.ref, int64(i*100), int64(i*100+99), chunkenc.NewXORChunk(), true, handleChunkWriteError)
				refs[i] = ref
				series.ooo.oooMmappedChunks = append(series.ooo.oooMmappedChunks, &mmappedChunk{
					ref: ref, minTime: int64(i * 100), maxTime: int64(i*100 + 99),
				})
			}

			if tc.hasOOOHeadChunk {
				series.ooo.oooHeadChunk = &oooHeadChunk{
					chunk:   NewOOOChunk(),
					minTime: int64(tc.numOOOChunks * 100),
					maxTime: int64(tc.numOOOChunks*100 + 99),
				}
			}

			minOOOMmapRef := refs[tc.truncateN-1]
			series.truncateChunksBefore(0, minOOOMmapRef)

			if tc.expOOONil {
				require.Nil(t, series.ooo, "ooo should be nil after truncating all chunks")
			} else {
				require.NotNil(t, series.ooo, "ooo should not be nil")
				require.Equal(t, tc.expFirstOOOChunkID, series.ooo.firstOOOChunkID, "firstOOOChunkID mismatch after wrap")
				require.Len(t, series.ooo.oooMmappedChunks, tc.expOOOMmappedLen, "oooMmappedChunks length mismatch")
			}
		})
	}
}

func TestPushHeadChunk_PanicsAtIDSpaceBound(t *testing.T) {
	s := newMemSeries(labels.FromStrings("a", "b"), 1, 0, true, false)
	s.headChunkCount.Store(oooChunkIDMask - 1)

	require.Panics(t, func() {
		s.pushHeadChunk(&memChunk{
			chunk:   chunkenc.NewXORChunk(),
			minTime: 0,
			maxTime: 0,
		})
	})
}

func TestAppendHistogramLayoutChange_PanicsAtIDSpaceBound(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	l := labels.FromStrings("l", "v1")

	app := head.AppenderV2(context.Background())
	h := tsdbutil.GenerateTestHistogram(0)
	_, err := app.Append(0, l, 0, 0, 0, h, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	ms, _, err := head.getOrCreate(l.Hash(), l, false)
	require.NoError(t, err)
	ms.Lock()
	ms.headChunkCount.Store(oooChunkIDMask - 1)
	ms.Unlock()

	h2 := h.Copy()
	h2.Schema++
	require.Panics(t, func() {
		app := head.AppenderV2(context.Background())
		_, _ = app.Append(0, l, 0, 1, 0, h2, nil, storage.AOptions{})
		_ = app.Commit()
	})
}

func TestHeadDeleteSeriesWithoutSamples(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range []compression.Type{compression.None, compression.Snappy, compression.Zstd} {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				entries := []any{
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

				populateTestWL(t, w, entries, nil, enableSTStorage)

				require.NoError(t, head.Init(math.MinInt64))

				require.NoError(t, head.Delete(context.Background(), 0, 100, labels.MustNewMatcher(labels.MatchEqual, "a", "1")))
			})
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

// TestMemSeries_append tests float appending with various useXOR2/st combinations.
func TestMemSeries_append(t *testing.T) {
	scenarios := []struct {
		name    string
		useXOR2 bool
		stFunc  func(ts int64) int64 // Function to compute st from ts
	}{
		{
			name:    "useXOR2=false st=0",
			useXOR2: false,
			stFunc:  func(_ int64) int64 { return 0 },
		},
		{
			name:    "useXOR2=true st=0",
			useXOR2: true,
			stFunc:  func(_ int64) int64 { return 0 },
		},
		{
			name:    "useXOR2=true st=ts",
			useXOR2: true,
			stFunc:  func(ts int64) int64 { return ts },
		},
		{
			name:    "useXOR2=true st=ts-100",
			useXOR2: true,
			stFunc:  func(ts int64) int64 { return ts - 100 },
		},
		{
			name:    "useXOR2=false st=ts (st ignored)",
			useXOR2: false,
			stFunc:  func(ts int64) int64 { return ts },
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			testMemSeriesAppend(t, scenario.useXOR2, scenario.stFunc)
		})
	}
}

func testMemSeriesAppend(t *testing.T, useXOR2 bool, stFunc func(ts int64) int64) {
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
		useXOR2:         useXOR2,
	}

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled, false)

	// Add first two samples at the very end of a chunk range and the next two
	// on and after it.
	// New chunk must correctly be cut at 1000.
	ok, chunkCreated := s.append(stFunc(998), 998, 1, 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.append(stFunc(999), 999, 2, 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")
	s.mmapChunks(chunkDiskMapper)

	ok, chunkCreated = s.append(stFunc(1000), 1000, 3, 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.append(stFunc(1001), 1001, 4, 0, cOpts)
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
		ts := 1001 + int64(i)
		ok, _ := s.append(stFunc(ts), ts, float64(i), 0, cOpts)
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

// TestMemSeries_appendHistogram tests histogram appending with various useXOR2/st combinations.
func TestMemSeries_appendHistogram(t *testing.T) {
	scenarios := []struct {
		name    string
		useXOR2 bool
		stFunc  func(ts int64) int64 // Function to compute st from ts
	}{
		{
			name:    "useXOR2=false st=0",
			useXOR2: false,
			stFunc:  func(_ int64) int64 { return 0 },
		},
		{
			name:    "useXOR2=true st=0",
			useXOR2: true,
			stFunc:  func(_ int64) int64 { return 0 },
		},
		{
			name:    "useXOR2=true st=ts",
			useXOR2: true,
			stFunc:  func(ts int64) int64 { return ts },
		},
		{
			name:    "useXOR2=true st=ts-100",
			useXOR2: true,
			stFunc:  func(ts int64) int64 { return ts - 100 },
		},
		{
			name:    "useXOR2=false st=ts (st ignored)",
			useXOR2: false,
			stFunc:  func(ts int64) int64 { return ts },
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			testMemSeriesAppendHistogram(t, scenario.useXOR2, scenario.stFunc)
		})
	}
}

func testMemSeriesAppendHistogram(t *testing.T, useXOR2 bool, stFunc func(ts int64) int64) {
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
		useXOR2:         useXOR2,
	}

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled, false)

	histograms := tsdbutil.GenerateTestHistograms(4)
	histogramWithOneMoreBucket := histograms[3].Copy()
	histogramWithOneMoreBucket.Count++
	histogramWithOneMoreBucket.Sum += 1.23
	histogramWithOneMoreBucket.PositiveSpans[1].Length = 3
	histogramWithOneMoreBucket.PositiveBuckets = append(histogramWithOneMoreBucket.PositiveBuckets, 1)

	// Add first two samples at the very end of a chunk range and the next two
	// on and after it.
	// New chunk must correctly be cut at 1000.
	ok, chunkCreated := s.appendHistogram(stFunc(998), 998, histograms[0], 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "first sample created chunk")

	ok, chunkCreated = s.appendHistogram(stFunc(999), 999, histograms[1], 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")

	ok, chunkCreated = s.appendHistogram(stFunc(1000), 1000, histograms[2], 0, cOpts)
	require.True(t, ok, "append failed")
	require.True(t, chunkCreated, "expected new chunk on boundary")

	ok, chunkCreated = s.appendHistogram(stFunc(1001), 1001, histograms[3], 0, cOpts)
	require.True(t, ok, "append failed")
	require.False(t, chunkCreated, "second sample should use same chunk")

	s.mmapChunks(chunkDiskMapper)
	require.Len(t, s.mmappedChunks, 1, "there should be only 1 mmapped chunk")
	require.Equal(t, int64(998), s.mmappedChunks[0].minTime, "wrong chunk range")
	require.Equal(t, int64(999), s.mmappedChunks[0].maxTime, "wrong chunk range")
	require.Equal(t, int64(1000), s.headChunks.minTime, "wrong chunk range")
	require.Equal(t, int64(1001), s.headChunks.maxTime, "wrong chunk range")

	ok, chunkCreated = s.appendHistogram(stFunc(1002), 1002, histogramWithOneMoreBucket, 0, cOpts)
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

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled, false)

	// At this slow rate, we will fill the chunk in two block durations.
	slowRate := (DefaultBlockDuration * 2) / samplesPerChunk

	var nextTs int64
	var totalAppendedSamples int
	for i := range samplesPerChunk / 4 {
		ok, _ := s.append(0, nextTs, float64(i), 0, cOpts)
		require.Truef(t, ok, "slow sample %d was not appended", i)
		nextTs += slowRate
		totalAppendedSamples++
	}
	require.Equal(t, DefaultBlockDuration, s.nextAt, "after appending a samplesPerChunk/4 samples at a slow rate, we should aim to cut a new block at the default block duration %d, but it's set to %d", DefaultBlockDuration, s.nextAt)

	// Suddenly, the rate increases and we receive a sample every millisecond.
	for i := range math.MaxUint16 {
		ok, _ := s.append(0, nextTs, float64(i), 0, cOpts)
		require.Truef(t, ok, "quick sample %d was not appended", i)
		nextTs++
		totalAppendedSamples++
	}
	ok, chunkCreated := s.append(0, DefaultBlockDuration, float64(0), 0, cOpts)
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
	h, _ := newTestHead(t, chunkRange, compression.None, false)

	cOpts := chunkOpts{
		chunkDiskMapper: h.chunkDiskMapper,
		chunkRange:      chunkRange,
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"), false)

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(0, 999, 999, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(0, 1000, 1000, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(0, 1999, 1999, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	idx := h.indexRange(0, 1500)
	var (
		chnks   []chunks.Meta
		builder labels.ScratchBuilder
	)
	require.NoError(t, idx.Series(1, &builder, &chnks))

	require.Equal(t, labels.FromStrings("a", "1"), builder.Labels())
	require.Len(t, chnks, 2)

	cr, err := h.chunksRange(0, 1500, nil)
	require.NoError(t, err)
	_, _, err = cr.ChunkOrIterable(chnks[0])
	require.NoError(t, err)
	_, _, err = cr.ChunkOrIterable(chnks[1])
	require.NoError(t, err)

	require.NoError(t, h.Truncate(1500)) // Remove a chunk.

	_, _, err = cr.ChunkOrIterable(chnks[0])
	require.Equal(t, storage.ErrNotFound, err)
	_, _, err = cr.ChunkOrIterable(chnks[1])
	require.NoError(t, err)
}

func TestGCSeriesAccess(t *testing.T) {
	// Put a series, select it. GC it and then access it.
	const chunkRange = 1000
	h, _ := newTestHead(t, chunkRange, compression.None, false)

	cOpts := chunkOpts{
		chunkDiskMapper: h.chunkDiskMapper,
		chunkRange:      chunkRange,
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	h.initTime(0)

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"), false)

	// Appending 2 samples for the first chunk.
	ok, chunkCreated := s.append(0, 0, 0, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(0, 999, 999, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.False(t, chunkCreated, "chunks was created")

	// A new chunks should be created here as it's beyond the chunk range.
	ok, chunkCreated = s.append(0, 1000, 1000, 0, cOpts)
	require.True(t, ok, "series append failed")
	require.True(t, chunkCreated, "chunks was not created")
	ok, chunkCreated = s.append(0, 1999, 1999, 0, cOpts)
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

// TestWalRepair_DecodingError ensures that a repair is run for an error
// when decoding a record.
func TestWalRepair_DecodingError(t *testing.T) {
	t.Parallel()
	for _, enableSTStorage := range []bool{false, true} {
		enc := record.Encoder{EnableSTStorage: enableSTStorage}
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
			for _, compress := range []compression.Type{compression.None, compression.Snappy, compression.Zstd} {
				t.Run(fmt.Sprintf("%s,compress=%s,stStorage=%v", name, compress, enableSTStorage), func(t *testing.T) {
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
}

// TestWblRepair_DecodingError ensures that a repair is run for an error
// when decoding a record.
func TestWblRepair_DecodingError(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		t.Run("enableSTStorage="+strconv.FormatBool(enableSTStorage), func(t *testing.T) {
			enc := record.Encoder{EnableSTStorage: enableSTStorage}
			corrFunc := func(rec []byte) []byte {
				return rec[:3]
			}
			rec := enc.Samples([]record.RefSample{{Ref: 0, T: 99, V: 1}}, []byte{})
			totalRecs := 9
			expRecs := 5
			dir := t.TempDir()

			// Fill the wbl and corrupt it.
			{
				wal, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
				require.NoError(t, err)
				wbl, err := wlog.New(nil, nil, filepath.Join(dir, "wbl"), compression.None)
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
		})
	}
}

// TestHead_TruncateWAL_IncrementsCorruptionMetricOnCorruptedSegment ensures that
// creating a checkpoint over a corrupted WAL segment increments
// prometheus_tsdb_wal_corruptions_total.
func TestHead_TruncateWAL_IncrementsCorruptionMetricOnCorruptedSegment(t *testing.T) {
	h, w := newTestHead(t, 1000, compression.None, false)

	enc := record.Encoder{}
	seriesRec := enc.Series([]record.RefSeries{
		{Ref: 1, Labels: labels.FromStrings("a", "b")},
	}, nil)

	// Create several WAL segments so that truncateWAL has a range to checkpoint.
	const numSegments = 4
	for range numSegments {
		require.NoError(t, w.Log(seriesRec))
		_, err := w.NextSegmentSync()
		require.NoError(t, err)
	}

	// Replay the intact WAL.
	require.NoError(t, h.Init(0))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))

	// Corrupt segment 0 on disk by flipping a byte inside the first record's data.
	segFile := wlog.SegmentName(w.Dir(), 0)
	f, err := os.OpenFile(segFile, os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0xff}, 7)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Trigger a checkpoint via WAL truncation.
	h.lastWALTruncationTime.Store(0)
	err = h.truncateWAL(100)
	require.Error(t, err)
	var cerr *wlog.CorruptionErr
	require.ErrorAs(t, err, &cerr, "checkpoint creation should report a WAL corruption error")
	require.Equal(t, 1.0, prom_testutil.ToFloat64(h.metrics.walCorruptionsTotal))
}

func TestHeadReadWriterRepair(t *testing.T) {
	dir := t.TempDir()

	const chunkRange = 1000

	walDir := filepath.Join(dir, "wal")
	// Fill the chunk segments and corrupt it.
	{
		w, err := wlog.New(nil, nil, walDir, compression.None)
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

		s, created, _ := h.getOrCreate(1, labels.FromStrings("a", "1"), false)
		require.True(t, created, "series was not created")

		for i := range 7 {
			ok, chunkCreated := s.append(0, int64(i*chunkRange), float64(i*chunkRange), 0, cOpts)
			require.True(t, ok, "series append failed")
			require.True(t, chunkCreated, "chunk was not created")
			ok, chunkCreated = s.append(0, int64(i*chunkRange)+chunkRange-1, float64(i*chunkRange), 0, cOpts)
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

func TestIsolationAppendIDZeroIsNoop(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	h, _ := newTestHead(t, 1000, compression.None, false)

	h.initTime(0)

	cOpts := chunkOpts{
		chunkDiskMapper: h.chunkDiskMapper,
		chunkRange:      h.chunkRange.Load(),
		samplesPerChunk: DefaultSamplesPerChunk,
	}

	s, _, _ := h.getOrCreate(1, labels.FromStrings("a", "1"), false)

	ok, _ := s.append(0, 0, 0, 0, cOpts)
	require.True(t, ok, "Series append failed.")
	require.Equal(t, 0, int(s.txs.txIDCount), "Series should not have an appendID after append with appendID=0.")
}

func TestHeadSeriesChunkRace(t *testing.T) {
	t.Parallel()
	for range 100 {
		testHeadSeriesChunkRace(t)
	}
}

// TestIsolationSeek checks that seeking does not expose head samples committed
// after the querier was created.
func TestIsolationSeek(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	db := newTestDB(t)
	ctx := t.Context()
	lbls := labels.FromStrings("foo", "bar")
	appendRange := func(from, to int64) {
		app := db.AppenderV2(ctx)
		for ts := from; ts <= to; ts++ {
			_, err := app.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}
	timestamps := func(from, to int64) []int64 {
		var res []int64
		for ts := from; ts <= to; ts++ {
			res = append(res, ts)
		}
		return res
	}

	appendRange(1, 10)
	// A querier starting after the chunk's first sample trims the chunk.
	queriers := []struct {
		name string
		mint int64
		q    storage.Querier
	}{
		{name: "untrimmed", mint: 0},
		{name: "trimmed", mint: 3},
	}
	for i := range queriers {
		q, err := db.Querier(queriers[i].mint, 100)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })
		queriers[i].q = q
	}
	// The queriers must not see samples committed after they were created,
	// although these share a head chunk with the ones they do see.
	appendRange(11, 20)

	for _, qc := range queriers {
		for _, tc := range []struct {
			name string
			seek int64 // Zero means advancing with Next only.
			want []int64
		}{
			{name: "next", want: timestamps(max(1, qc.mint), 10)},
			{name: "seek to a visible sample", seek: 5, want: timestamps(5, 10)},
			{name: "seek past the visible samples", seek: 15},
		} {
			t.Run(qc.name+"/"+tc.name, func(t *testing.T) {
				ss := qc.q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
				require.True(t, ss.Next())
				it := ss.At().Iterator(nil)
				vt := it.Next()
				if tc.seek != 0 {
					vt = it.Seek(tc.seek)
				}
				var got []int64
				for ; vt == chunkenc.ValFloat; vt = it.Next() {
					got = append(got, it.AtT())
				}
				require.NoError(t, it.Err())
				require.Equal(t, tc.want, got)
				require.False(t, ss.Next())
				require.NoError(t, ss.Err())
			})
		}
	}
}

func testHeadSeriesChunkRace(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)
	require.NoError(t, h.Init(0))
	app := h.AppenderV2(context.Background())

	s2, err := app.Append(0, labels.FromStrings("foo2", "bar"), 0, 5, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	for ts := int64(6); ts < 11; ts++ {
		_, err = app.Append(s2, labels.EmptyLabels(), 0, ts, 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	matcher := labels.MustNewMatcher(labels.MatchEqual, "", "")
	q, err := NewBlockQuerier(h, 18, 22)
	require.NoError(t, err)
	defer q.Close()

	var wg sync.WaitGroup
	wg.Go(func() {
		h.updateMinMaxTime(20, 25)
		h.gc()
	})
	ss := q.Select(context.Background(), false, nil, matcher)
	for ss.Next() {
	}
	require.NoError(t, ss.Err())
	wg.Wait()
}

func TestHeadLabelValuesWithMatchers(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)

	ctx := context.Background()

	app := head.AppenderV2(context.Background())
	for i := range 100 {
		_, err := app.Append(0, labels.FromStrings(
			"tens", fmt.Sprintf("value%d", i/10),
			"unique", fmt.Sprintf("value%d", i),
		), 0, 100, 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	var uniqueWithout30s []string
	for i := range 100 {
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

			actualValues, err := headIdxReader.SortedLabelValues(ctx, tt.labelName, nil, tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)

			actualValues, err = headIdxReader.LabelValues(ctx, tt.labelName, nil, tt.matchers...)
			sort.Strings(actualValues)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)
		})
	}
}

func TestHeadLabelNamesWithMatchers(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)

	app := head.AppenderV2(context.Background())
	for i := range 100 {
		_, err := app.Append(0, labels.FromStrings(
			"unique", fmt.Sprintf("value%d", i),
		), 0, 100, 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)

		if i%10 == 0 {
			_, err := app.Append(0, labels.FromStrings(
				"tens", fmt.Sprintf("value%d", i/10),
				"unique", fmt.Sprintf("value%d", i),
			), 0, 100, 0, nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}

		if i%20 == 0 {
			_, err := app.Append(0, labels.FromStrings(
				"tens", fmt.Sprintf("value%d", i/10),
				"twenties", fmt.Sprintf("value%d", i/20),
				"unique", fmt.Sprintf("value%d", i),
			), 0, 100, 0, nil, nil, storage.AOptions{})
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
	head, _ := newTestHeadWithOptions(t, compression.None, headOpts)

	ctx := context.Background()

	// Append some series.
	app := head.AppenderV2(ctx)
	for i := range 100 {
		_, err := app.Append(0, labels.FromStrings("unique", fmt.Sprintf("value%d", i), "const", "1"), 0, 100, 0, nil, nil, storage.AOptions{})
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

	for shardIndex := range shardCount {
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

	errExpected := errors.New("postings error")
	for _, tc := range []struct {
		name string
		refs []storage.SeriesRef
	}{
		{name: "empty"},
		{name: "non-empty", refs: expected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := index.Merge(ctx, index.NewListPostings(tc.refs), index.ErrPostings(errExpected))
			p = ir.ShardedPostings(p, 0, shardCount)
			require.False(t, p.Next())
			require.ErrorIs(t, p.Err(), errExpected)
		})
	}
}

func BenchmarkHeadLabelValuesWithMatchers(b *testing.B) {
	chunkRange := int64(2000)
	head, _ := newTestHead(b, chunkRange, compression.None, false)

	ctx := context.Background()

	app := head.AppenderV2(context.Background())

	metricCount := 1000000
	for i := range metricCount {
		_, err := app.Append(0, labels.FromStrings(
			"a_unique", fmt.Sprintf("value%d", i),
			"b_tens", fmt.Sprintf("value%d", i/(metricCount/10)),
			"c_ninety", fmt.Sprintf("value%d", i/(metricCount/10)/9), // "0" for the first 90%, then "1"
		), 0, 100, 0, nil, nil, storage.AOptions{})
		require.NoError(b, err)
	}
	require.NoError(b, app.Commit())

	headIdxReader := head.indexRange(0, 200)
	matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "c_ninety", "value0")}

	b.ReportAllocs()

	for b.Loop() {
		actualValues, err := headIdxReader.LabelValues(ctx, "b_tens", nil, matchers...)
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

	s := newMemSeries(labels.Labels{}, 1, 0, defaultIsolationDisabled, false)

	for i := range 7 {
		ok, _ := s.append(0, int64(i), float64(i), 0, cOpts)
		require.True(t, ok, "sample append failed")
	}

	c, _, _, err := s.chunk(0, chunkDiskMapper, &sync.Pool{
		New: func() any {
			return &memChunk{}
		},
	}, nil)
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
	t.Parallel()
	db := newTestDB(t)
	db.DisableCompactions()
	ctx := context.Background()

	var (
		app        = db.AppenderV2(context.Background())
		ref        = storage.SeriesRef(0)
		mint, maxt = int64(0), int64(0)
		err        error
	)

	// Appends samples to span over 1.5 block ranges.
	// 7 chunks with 15s scrape interval.
	for i := int64(0); i <= 120*7; i++ {
		ts := i * DefaultBlockDuration / (4 * 120)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), 0, ts, float64(i), nil, nil, storage.AOptions{})
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
	wg.Go(func() {
		// Compacting head while the querier spans the compaction time.
		require.NoError(t, db.Compact(ctx))
		require.NotEmpty(t, db.Blocks())
	})

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

// TestQuerierDoesNotBlockHeadTruncation verifies that a querier opened during an in-progress
// head truncation does not hold up the truncation for the range below the truncation point.
// The collision guard (IsQuerierCollidingWithTruncation) decides that in-order data below the
// truncation point now lives in a block, so the querier's in-order isolation read must be
// clamped to the truncation point. This is checked for both query paths:
//   - the plain in-order querier, clamped directly in db.Querier (close / reopen at newMint);
//   - the head+OOO querier, whose wrapper must take its in-order isolation read at the same
//     clamped point rather than the raw mint.
func TestQuerierDoesNotBlockHeadTruncation(t *testing.T) {
	t.Parallel()

	const (
		maxT      int64 = 3000
		truncTime int64 = 2000
	)

	// newDB builds a head with in-order samples spanning [0,maxT). When withOOO is set it also
	// appends an OOO range over the same period, which forces db.Querier down the head+OOO path
	// (overlapsOOO is decided from the head's global OOO min/max, not per series).
	newDB := func(t *testing.T, withOOO bool) *DB {
		dir := t.TempDir()
		opts := DefaultOptions()
		opts.OutOfOrderTimeWindow = maxT

		db, err := Open(dir, nil, nil, opts, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		db.DisableCompactions()

		app := db.AppenderV2(context.Background())
		ref := storage.SeriesRef(0)
		for i := int64(0); i < maxT; i += 100 {
			ref, err = app.Append(ref, labels.FromStrings("a", "b"), 0, i, float64(i), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		if withOOO {
			for i := int64(50); i < maxT; i += 100 {
				_, err = app.Append(ref, labels.FromStrings("a", "b"), 0, i, float64(i), nil, nil, storage.AOptions{})
				require.NoError(t, err)
			}
		}
		require.NoError(t, app.Commit())

		// Mock a head memory truncation in progress up to truncTime. At this point the block
		// for [headMin, truncTime) has already been written and reloaded, so any querier whose
		// range falls below truncTime must read from the block and must NOT pin the head.
		db.head.lastMemoryTruncationTime.Store(truncTime)
		db.head.memTruncationInProcess.Store(true)
		t.Cleanup(func() { db.head.memTruncationInProcess.Store(false) })
		return db
	}

	assertDoesNotBlock := func(t *testing.T, db *DB, cl io.Closer) {
		synctest.Test(t, func(t *testing.T) {
			var waitOver atomic.Bool
			go func() {
				db.head.WaitForPendingReadersInTimeRange(db.head.MinTime(), truncTime)
				waitOver.Store(true)
			}()

			// Let the goroutine run until it either finishes (no overlap) or blocks on Sleep.
			synctest.Wait()
			blocked := !waitOver.Load()

			// Always release the querier and drain so the goroutine never leaks, regardless
			// of outcome. Once the only overlapping read is gone the wait must complete.
			require.NoError(t, cl.Close())
			time.Sleep(time.Second)
			synctest.Wait()
			require.True(t, waitOver.Load(), "wait must complete once the querier is closed")

			require.False(t, blocked,
				"a querier whose range is below the truncation point must not block head truncation: "+
					"its in-order isolation read must be clamped to the truncation point")
		})
	}

	// Both ranges read in-order data below the truncation point: one straddles the truncation
	// point (collision guard reopens the head querier at the truncation point), the other is
	// entirely below it (guard closes the head querier without reopening). Neither must pin the head.
	ranges := []struct {
		name       string
		mint, maxt int64
	}{
		{"straddling truncation point", 1000, 2500},
		{"entirely below truncation point", 500, 1500},
	}
	for _, withOOO := range []bool{false, true} {
		name := "in-order"
		if withOOO {
			name = "in-order+OOO"
		}
		t.Run(name, func(t *testing.T) {
			db := newDB(t, withOOO)
			for _, r := range ranges {
				t.Run(r.name, func(t *testing.T) {
					q, err := db.Querier(r.mint, r.maxt)
					require.NoError(t, err)
					assertDoesNotBlock(t, db, q)

					cq, err := db.ChunkQuerier(r.mint, r.maxt)
					require.NoError(t, err)
					assertDoesNotBlock(t, db, cq)
				})
			}
		})
	}
}

func TestQueryOOOHeadDuringTruncate(t *testing.T) {
	testQueryOOOHeadDuringTruncateAppenderV2(t,
		func(db *DB, minT, maxT int64) (storage.LabelQuerier, error) {
			return db.Querier(minT, maxT)
		},
		func(t *testing.T, lq storage.LabelQuerier, minT, _ int64) {
			// Samples
			q, ok := lq.(storage.Querier)
			require.True(t, ok)
			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
			require.True(t, ss.Next())
			s := ss.At()
			require.False(t, ss.Next()) // One series.
			it := s.Iterator(nil)
			require.NotEqual(t, chunkenc.ValNone, it.Next()) // Has some data.
			require.Equal(t, minT, it.AtT())                 // It is an in-order sample.
			require.NotEqual(t, chunkenc.ValNone, it.Next()) // Has some data.
			require.Equal(t, minT+50, it.AtT())              // it is an out-of-order sample.
			require.NoError(t, it.Err())
		},
	)
}

// TestSnapshotUnknownEncodingFallsBackToWAL verifies that a snapshot containing
// an unknown chunk encoding causes the entire snapshot load to fail and fall back
// to full WAL replay, recovering all series without data loss.
func TestSnapshotUnknownEncodingFallsBackToWAL(t *testing.T) {
	head, _ := newTestHead(t, 120*4, compression.None, false)
	defer func() {
		head.opts.EnableMemorySnapshotOnShutdown = false
		require.NoError(t, head.Close())
	}()

	floatHist := tsdbutil.GenerateTestGaugeFloatHistograms(1)[0]
	lblsFloatHist := labels.FromStrings("floathist", "bar")
	lblsFloat := labels.FromStrings("foo", "bar")

	app := head.AppenderV2(context.Background())
	_, err := app.Append(0, lblsFloatHist, 0, 99, 0, nil, floatHist, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, lblsFloat, 0, 99, 99.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	head.opts.EnableMemorySnapshotOnShutdown = true
	require.NoError(t, head.Close())

	// Find the snapshot and corrupt the encoding byte of the float histogram series.
	snapDir, _, _, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.NoError(t, err)

	sr, err := wlog.NewSegmentsReader(snapDir)
	require.NoError(t, err)
	r := wlog.NewReader(sr)
	syms := labels.NewSymbolTable()
	rdec := record.NewDecoder(syms, promslog.NewNopLogger())
	var (
		records [][]byte
		mutated bool
	)
	for r.Next() {
		rec := append([]byte(nil), r.Record()...)
		if rec[0] == chunkSnapshotRecordTypeSeries {
			buf := encoding.Decbuf{B: rec}
			_ = buf.Byte() // flag
			_ = buf.Be64() // ref
			lset := rdec.DecodeLabels(&buf)
			_ = buf.Be64int64() // chunkRange
			if buf.Uvarint() == 1 && lset.Get("floathist") == "bar" {
				_ = buf.Be64int64() // minTime
				_ = buf.Be64int64() // maxTime
				encPos := len(rec) - buf.Len()
				require.Equal(t, byte(chunkenc.EncFloatHistogram), rec[encPos],
					"expected float histogram encoding at computed offset")
				rec[encPos] = 0xFF
				mutated = true
			}
		}
		records = append(records, rec)
	}
	require.NoError(t, r.Err())
	require.NoError(t, sr.Close())
	require.True(t, mutated, "expected to find and corrupt the float histogram series record")

	// Rewrite the snapshot with the mutated records.
	files, err := os.ReadDir(snapDir)
	require.NoError(t, err)
	for _, f := range files {
		require.NoError(t, os.Remove(filepath.Join(snapDir, f.Name())))
	}
	cp, err := wlog.New(nil, nil, snapDir, compression.None)
	require.NoError(t, err)
	require.NoError(t, cp.Log(records...))
	require.NoError(t, cp.Close())

	// Reload the head; snapshot should fail due to unknown encoding and fall back to WAL.
	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	require.NoError(t, err)
	head, err = NewHead(prometheus.NewRegistry(), nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))
	require.Equal(t, uint64(2), head.NumSeries(), "both series must be recovered from WAL")
	require.NotNil(t, head.series.getByHash(lblsFloat.Hash(), lblsFloat))
	require.NotNil(t, head.series.getByHash(lblsFloatHist.Hash(), lblsFloatHist))
}

func TestHistogramSTCounterResetHeaderOnChunkCut(t *testing.T) {
	for _, floatHisto := range []bool{false, true} {
		t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
			l := labels.FromStrings("a", "b")
			opts := newTestHeadDefaultOptions(2, false)
			opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
			opts.EnableHistogramSTEncoding.Store(true)
			head, _ := newTestHeadWithOptions(t, compression.None, opts)
			t.Cleanup(func() {
				require.NoError(t, head.Close())
			})
			require.NoError(t, head.Init(0))

			appendHistogram := func(ts int64, h *histogram.Histogram) {
				app := head.AppenderV2(context.Background())
				var err error
				if floatHisto {
					_, err = app.Append(0, l, 0, ts, 0, nil, h.ToFloat(nil), storage.AOptions{})
				} else {
					_, err = app.Append(0, l, 0, ts, 0, h.Copy(), nil, storage.AOptions{})
				}
				require.NoError(t, err)
				require.NoError(t, app.Commit())
			}

			h1 := tsdbutil.GenerateTestHistogram(0)
			h2 := tsdbutil.GenerateTestHistogram(1)

			// Chunk range is 2, so appending at t=1 and t=2 cuts a new chunk on the second append.
			appendHistogram(1, h1)
			appendHistogram(2, h2)

			ms, _, err := head.getOrCreate(l.Hash(), l, false)
			require.NoError(t, err)
			require.NotNil(t, ms.headChunks)
			require.NotNil(t, ms.headChunks.prev)

			if floatHisto {
				prevChunk, ok := ms.headChunks.prev.chunk.(*chunkenc.FloatHistogramSTChunk)
				require.True(t, ok)
				require.Equal(t, chunkenc.UnknownCounterReset, prevChunk.GetCounterResetHeader())

				headChunk, ok := ms.headChunks.chunk.(*chunkenc.FloatHistogramSTChunk)
				require.True(t, ok)
				require.Equal(t, chunkenc.NotCounterReset, headChunk.GetCounterResetHeader())
			} else {
				prevChunk, ok := ms.headChunks.prev.chunk.(*chunkenc.HistogramSTChunk)
				require.True(t, ok)
				require.Equal(t, chunkenc.UnknownCounterReset, prevChunk.GetCounterResetHeader())

				headChunk, ok := ms.headChunks.chunk.(*chunkenc.HistogramSTChunk)
				require.True(t, ok)
				require.Equal(t, chunkenc.NotCounterReset, headChunk.GetCounterResetHeader())
			}
		})
	}
}

// Tests https://github.com/prometheus/prometheus/issues/9725.
func TestChunkSnapshotReplayBug(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		t.Run("enableSTStorage="+strconv.FormatBool(enableSTStorage), func(t *testing.T) {
			dir := t.TempDir()
			wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
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
				enc := record.Encoder{EnableSTStorage: enableSTStorage}

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
		})
	}
}

// TestWALReplayMmapsChunks is a regression test ensuring that
// chunks are mmapped during WAL replay, not accumulated in the
// in-memory linked list.
func TestWALReplayMmapsChunks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		append func(app storage.AppenderV2, l labels.Labels, ts int64) error
	}{
		{
			name: "floats",
			append: func(app storage.AppenderV2, l labels.Labels, ts int64) error {
				_, err := app.Append(0, l, 0, ts, float64(ts), nil, nil, storage.AOptions{})
				return err
			},
		},
		{
			name: "histograms",
			append: func(app storage.AppenderV2, l labels.Labels, ts int64) error {
				_, err := app.Append(0, l, 0, ts, 0, tsdbutil.GenerateTestHistogram(ts), nil, storage.AOptions{})
				return err
			},
		},
		{
			name: "float histograms",
			append: func(app storage.AppenderV2, l labels.Labels, ts int64) error {
				_, err := app.Append(0, l, 0, ts, 0, nil, tsdbutil.GenerateTestFloatHistogram(ts), storage.AOptions{})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
			require.NoError(t, err)

			opts := DefaultHeadOptions()
			opts.ChunkRange = 1000
			opts.ChunkDirRoot = dir

			h, err := NewHead(nil, nil, wal, nil, opts, nil)
			require.NoError(t, err)
			require.NoError(t, h.Init(0))

			l := labels.FromStrings("foo", "bar")

			// Append 250 samples at 1-minute intervals. With ChunkRange=1000 and
			// DefaultSamplesPerChunk=120, this creates multiple chunks that should
			// be mmapped during WAL replay.
			app := h.AppenderV2(context.Background())
			for i := range 250 {
				require.NoError(t, tc.append(app, l, int64(i)*time.Minute.Milliseconds()))
			}
			require.NoError(t, app.Commit())

			require.NoError(t, h.Close())

			// Remove mmapped chunk files so WAL replay must recreate all chunks.
			require.NoError(t, os.RemoveAll(filepath.Join(dir, "chunks_head")))

			// Reopen Head — WAL replay happens in Init.
			wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
			require.NoError(t, err)
			h, err = NewHead(nil, nil, wal, nil, opts, nil)
			require.NoError(t, err)
			require.NoError(t, h.Init(0))

			ms, ok, err := h.getOrCreate(l.Hash(), l, false)
			require.NoError(t, err)
			require.False(t, ok)
			require.NotNil(t, ms)

			// Chunks must be mmapped during replay, not left in the head linked list.
			require.NotEmpty(t, ms.mmappedChunks, "expected chunks to be mmapped during WAL replay")
			require.Equal(t, 1, ms.headChunks.len(), "expected only one head chunk after replay")

			// Verify each mmapped chunk is readable from disk.
			for _, m := range ms.mmappedChunks {
				chk, err := h.chunkDiskMapper.Chunk(m.ref)
				require.NoError(t, err)
				require.Equal(t, int(m.numSamples), chk.NumSamples())
			}

			require.NoError(t, h.Close())
		})
	}
}

// unsupportedChunk holds a XORChunk and overrides the Encoding() method.
type unsupportedChunk struct {
	*chunkenc.XORChunk
}

func newUnsupportedChunk() *unsupportedChunk {
	return &unsupportedChunk{chunkenc.NewXORChunk()}
}

func (*unsupportedChunk) Encoding() chunkenc.Encoding {
	return EncUnsupportedXOR
}

// TODO(bwplotka): Bad benchmark (no b.Loop/b.N), fix or remove.
func BenchmarkCuttingHeadHistogramChunks(b *testing.B) {
	const (
		numSamples = 50000
		numBuckets = 100
	)
	samples := histogram.GenerateBigTestHistograms(numSamples, numBuckets)

	h, _ := newTestHead(b, DefaultBlockDuration, compression.None, false)
	defer func() {
		require.NoError(b, h.Close())
	}()

	a := h.AppenderV2(context.Background())
	ts := time.Now().UnixMilli()
	lbls := labels.FromStrings("foo", "bar")

	b.ResetTimer()

	for _, s := range samples {
		_, err := a.Append(0, lbls, 0, ts, 0, s, nil, storage.AOptions{})
		require.NoError(b, err)
	}
}

func labelsWithHashCollision() (labels.Labels, labels.Labels) {
	// These two series have the same XXHash; thanks to https://github.com/pstibrany/labels_hash_collisions
	ls1 := labels.FromStrings("__name__", "metric", "lbl", "HFnEaGl")
	ls2 := labels.FromStrings("__name__", "metric", "lbl", "RqcXatm")

	if ls1.Hash() != ls2.Hash() {
		// These ones are the same when using -tags slicelabels
		ls1 = labels.FromStrings("__name__", "metric", "lbl1", "value", "lbl2", "l6CQ5y")
		ls2 = labels.FromStrings("__name__", "metric", "lbl1", "value", "lbl2", "v7uDlF")
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
	ms1 := newMemSeries(lbls1, 1, 0, true, false)
	ms2 := newMemSeries(lbls2, 2, 0, true, false)
	hash := lbls1.Hash()
	s := newStripeSeries(1, noopSeriesLifecycleCallback{})

	got, created := s.setUnlessAlreadySet(hash, lbls1, ms1)
	require.True(t, created)
	require.Same(t, ms1, got)

	// Add a conflicting series
	got, created = s.setUnlessAlreadySet(hash, lbls2, ms2)
	require.True(t, created)
	require.Same(t, ms2, got)

	return s, ms1, ms2
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
	t.Run("retains the oldest referenced file regardless of timestamp order", func(t *testing.T) {
		lset := labels.FromStrings("a", "1")
		series := newMemSeries(lset, 1, 0, defaultIsolationDisabled, false)
		// File numbers occupy the upper 32 bits of the disk reference.
		series.mmappedChunks = []*mmappedChunk{
			{ref: chunks.ChunkDiskMapperRef(3 << 32), minTime: 0, maxTime: 50},
			{ref: chunks.ChunkDiskMapperRef(1 << 32), minTime: 100, maxTime: 150},
			{ref: chunks.ChunkDiskMapperRef(2 << 32), minTime: 200, maxTime: 250},
		}
		s := newStripeSeries(1, noopSeriesLifecycleCallback{})
		_, created := s.setUnlessAlreadySet(lset.Hash(), lset, series)
		require.True(t, created)

		for _, tc := range []struct {
			mint               int64
			minFile, remaining int
		}{
			{mint: 0, minFile: 1, remaining: 3},
			{mint: 51, minFile: 1, remaining: 2},
			{mint: 151, minFile: 2, remaining: 1},
			{mint: 251, minFile: math.MaxInt32, remaining: 0},
		} {
			_, _, _, _, _, _, _, _, minFile := s.gc(tc.mint, 0)
			require.Equal(t, tc.minFile, minFile, "mint=%d", tc.mint)
			require.Len(t, series.mmappedChunks, tc.remaining)
		}
	})

	t.Run("marks collected series", func(t *testing.T) {
		s, ms1, ms2 := stripeSeriesWithCollidingSeries(t)
		hash := ms1.lset.Hash()

		s.gc(0, 0)

		// Verify that we can get neither ms1 nor ms2 after garbage collection.
		require.Nil(t, s.getByHash(hash, ms1.lset))
		require.Nil(t, s.getByHash(hash, ms2.lset))
		ms1.Lock()
		require.True(t, ms1.isGCed())
		ms1.Unlock()
		ms2.Lock()
		require.True(t, ms2.isGCed())
		ms2.Unlock()
	})

	t.Run("collects a directly constructed series without samples or exemplars at mint<=0", func(t *testing.T) {
		// Construct the series directly to verify that zero-valued exemplar state
		// does not imply an exemplar at timestamp zero. With no chunks or exemplars,
		// the series must be collected at mint<=0 without constructor initialization.
		for _, mint := range []int64{-1, 0} {
			lset := labels.FromStrings("a", "1")
			series := &memSeries{lset: lset, ref: 1}
			s := newStripeSeries(1, noopSeriesLifecycleCallback{})
			_, created := s.setUnlessAlreadySet(lset.Hash(), lset, series)
			require.True(t, created)

			s.gc(mint, 0)

			require.Nil(t, s.getByHash(lset.Hash(), lset),
				"a series with no chunks and no exemplar must be collected at mint=%d, regardless of how it was constructed", mint)
		}
	})

	t.Run("keeps series with zero or negative exemplar timestamps", func(t *testing.T) {
		for _, ts := range []int64{-1, 0} {
			lset := labels.FromStrings("a", "1")
			series := &memSeries{lset: lset, ref: 1}
			s := newStripeSeries(1, noopSeriesLifecycleCallback{})
			_, created := s.setUnlessAlreadySet(lset.Hash(), lset, series)
			require.True(t, created)

			series.updateExemplarTimestamp(ts)
			// An older exemplar must not shorten the lifetime of the series.
			series.updateExemplarTimestamp(ts - 1)
			s.gc(ts, 0)
			require.Same(t, series, s.getByID(series.ref), "exemplar timestamp=%d", ts)

			s.gc(ts+1, 0)
			require.Nil(t, s.getByID(series.ref), "exemplar timestamp=%d", ts)
		}
	})

	t.Run("keeps series until all reservations are released", func(t *testing.T) {
		lset := labels.FromStrings("a", "1")
		series := newMemSeries(lset, 1, 0, defaultIsolationDisabled, false)
		series.Lock()
		series.markPendingCommit()
		series.markPendingCommit()
		require.True(t, series.unmarkPendingCommit())
		require.Equal(t, uint32(1), series.pendingCommitCount())
		series.Unlock()

		s := newStripeSeries(1, noopSeriesLifecycleCallback{})
		got, created := s.setUnlessAlreadySet(lset.Hash(), lset, series)
		require.True(t, created)
		require.Same(t, series, got)

		deleted, _, _, _, _, _, _, _, _ := s.gc(0, 0)
		require.Empty(t, deleted)
		require.Same(t, series, s.getByID(series.ref))

		series.Lock()
		require.True(t, series.unmarkPendingCommit())
		series.Unlock()
		deleted, _, _, _, _, _, _, _, _ = s.gc(0, 0)
		require.Contains(t, deleted, storage.SeriesRef(series.ref))
		require.Nil(t, s.getByID(series.ref))
	})

	t.Run("underflow preserves garbage-collected flag", func(t *testing.T) {
		series := newMemSeries(labels.FromStrings("a", "1"), 1, 0, defaultIsolationDisabled, false)
		series.Lock()
		t.Cleanup(series.Unlock)
		series.setGCed()
		require.False(t, series.unmarkPendingCommit())
		require.Zero(t, series.pendingCommitCount())
		require.True(t, series.isGCed())
	})
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

// TestHeadAppender_PendingCommitUnderflowIsReportedNotFatal verifies that releasing
// a pending-sample reservation that is not held is reported and survivable. The
// commit must still finish: panicking here would leave the series lock held, and
// wrapping the count would corrupt the packed flag bits.
func TestHeadAppender_PendingCommitUnderflowIsReportedNotFatal(t *testing.T) {
	h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
	lset := labels.FromStrings("a", "1")

	app := h.AppenderV2(t.Context())
	ref, err := app.Append(0, lset, 0, 100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)

	// Drop both reservations the append took, so every release during Commit
	// underflows: one for the queued sample, one for the created series.
	series := h.series.getByID(chunks.HeadSeriesRef(ref))
	require.NotNil(t, series)
	series.Lock()
	require.Equal(t, uint32(2), series.pendingCommitCount())
	require.True(t, series.unmarkPendingCommit())
	require.True(t, series.unmarkPendingCommit())
	series.Unlock()

	require.NotPanics(t, func() { require.NoError(t, app.Commit()) })
	require.Equal(t, 2.0, prom_testutil.ToFloat64(h.metrics.pendingCommitUnderflow))

	series.Lock()
	require.Zero(t, series.pendingCommitCount(), "the count must clamp rather than wrap")
	require.False(t, series.isGCed(), "underflow must not set packed flag bits")
	series.Unlock()

	// The sample is still committed; the accounting bug must not lose data on its own.
	q, err := NewBlockQuerier(h, math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	require.Equal(t,
		map[string][]chunks.Sample{`{a="1"}`: {sample{t: 100, f: 1}}},
		query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "1")))
}

func TestHeadCompactableDoesNotCompactEmptyHead(t *testing.T) {
	// Use a chunk range of 1 here so that if we attempted to determine if the head
	// was compactable using default values for min and max times, `Head.compactable()`
	// would return true which is incorrect. This test verifies that we short-circuit
	// the check when the head has not yet had any samples added.
	head, _ := newTestHead(t, 1, compression.None, false)

	require.False(t, head.compactable())
}

type countSeriesLifecycleCallback struct {
	created atomic.Int64
	deleted atomic.Int64
}

func (*countSeriesLifecycleCallback) PreCreation(labels.Labels) error { return nil }
func (c *countSeriesLifecycleCallback) PostCreation(labels.Labels)    { c.created.Inc() }
func (c *countSeriesLifecycleCallback) PostDeletion(s map[chunks.HeadSeriesRef]labels.Labels) {
	c.deleted.Add(int64(len(s)))
}

// staleSampleKind selects the sample type appended by appendStaleTestSample.
type staleSampleKind int

const (
	staleKindFloat staleSampleKind = iota
	staleKindHistogram
	staleKindFloatHistogram
)

func (k staleSampleKind) String() string {
	switch k {
	case staleKindFloat:
		return "float"
	case staleKindHistogram:
		return "histogram"
	default:
		return "float_histogram"
	}
}

// appendStaleTestSample appends a single sample of the given type, either a
// normal value or a staleness marker, and commits it.
func appendStaleTestSample(t testing.TB, head *Head, kind staleSampleKind, lbls labels.Labels, ts int64, stale bool) {
	t.Helper()
	staleNaN := math.Float64frombits(value.StaleNaN)
	app := head.AppenderV2(context.Background())
	var err error
	switch kind {
	case staleKindFloat:
		v := float64(ts)
		if stale {
			v = staleNaN
		}
		_, err = app.Append(0, lbls, 0, ts, v, nil, nil, storage.AOptions{})
	case staleKindHistogram:
		h := tsdbutil.GenerateTestHistogram(1)
		if stale {
			h.Sum = staleNaN
		}
		_, err = app.Append(0, lbls, 0, ts, 0, h, nil, storage.AOptions{})
	case staleKindFloatHistogram:
		fh := tsdbutil.GenerateTestFloatHistogram(1)
		if stale {
			fh.Sum = staleNaN
		}
		_, err = app.Append(0, lbls, 0, ts, 0, nil, fh, storage.AOptions{})
	}
	require.NoError(t, err)
	require.NoError(t, app.Commit())
}

// TestHead_NumStaleSeries_MixedType verifies that the stale-series gauge is
// maintained when a series goes stale as one sample type and then becomes
// non-stale as a different type (float, integer histogram, or float histogram).
// Before the fix, such a series stayed counted as stale because staleness was
// derived from a type-specific last value. The scenarios are exercised during
// live ingestion and after reconstruction from the WAL and from a memory
// snapshot.
func TestHead_NumStaleSeries_MixedType(t *testing.T) {
	transitions := []struct{ stale, resolve staleSampleKind }{
		{staleKindFloat, staleKindHistogram},
		{staleKindFloat, staleKindFloatHistogram},
		{staleKindHistogram, staleKindFloat},
		{staleKindHistogram, staleKindFloatHistogram},
		{staleKindFloatHistogram, staleKindFloat},
		{staleKindFloatHistogram, staleKindHistogram},
	}
	for _, mode := range []string{"live", "wal", "snapshot"} {
		for _, tr := range transitions {
			t.Run(fmt.Sprintf("%s/%s_to_%s", mode, tr.stale, tr.resolve), func(t *testing.T) {
				head, _ := newTestHead(t, 1000, compression.None, false)
				head.opts.EnableMemorySnapshotOnShutdown = mode == "snapshot"
				t.Cleanup(func() {
					// Captures head by reference, so it closes the final head after a restart.
					_ = head.Close()
				})
				require.NoError(t, head.Init(0))

				lbls := labels.FromStrings("name", t.Name())
				// Going stale counts the series, resolving with a different type must uncount it.
				appendStaleTestSample(t, head, tr.stale, lbls, 100, true)
				require.Equal(t, uint64(1), head.NumStaleSeries())
				appendStaleTestSample(t, head, tr.resolve, lbls, 200, false)
				require.Equal(t, uint64(0), head.NumStaleSeries())

				if mode != "live" {
					require.NoError(t, head.Close())
					wal, err := wlog.NewSize(nil, nil, filepath.Join(head.opts.ChunkDirRoot, "wal"), 32768, compression.None)
					require.NoError(t, err)
					head, err = NewHead(nil, nil, wal, nil, head.opts, nil)
					require.NoError(t, err)
					require.NoError(t, head.Init(0))
				}

				require.Equal(t, uint64(1), head.NumSeries())
				require.Equal(t, uint64(0), head.NumStaleSeries())
			})
		}
	}
}

// TestHead_NumStaleSeries_MixedTypeRemoval verifies that the removal paths that
// consult isStaleSeries treat a series that resolved to non-stale via a
// different sample type correctly: garbage collection must not decrement the
// gauge for it (which would underflow), and the early stale-series eviction
// (truncateStaleSeries, used during block writes) must not drop it as if it
// were stale.
func TestHead_NumStaleSeries_MixedTypeRemoval(t *testing.T) {
	ref := func(head *Head, lbls labels.Labels) storage.SeriesRef {
		ms := head.series.getByHash(lbls.Hash(), lbls)
		require.NotNil(t, ms)
		return storage.SeriesRef(ms.ref)
	}
	for _, tc := range []struct {
		name              string
		remove            func(t *testing.T, head *Head, refs []storage.SeriesRef)
		wantSeries        uint64
		crossTypeSurvives bool
	}{
		{
			name: "gc",
			remove: func(t *testing.T, head *Head, _ []storage.SeriesRef) {
				require.NoError(t, head.Truncate(1000))
				head.gc()
			},
			wantSeries:        1, // Only the keeper, which has a recent sample.
			crossTypeSurvives: false,
		},
		{
			name: "truncate_stale_series",
			remove: func(t *testing.T, head *Head, refs []storage.SeriesRef) {
				require.NoError(t, head.truncateStaleSeries(refs, 1000, head.snapshotFingerprints(refs, math.MaxUint64)))
			},
			wantSeries:        2, // Only the genuinely stale series is evicted.
			crossTypeSurvives: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head, _ := newTestHead(t, 1000, compression.None, false)
			t.Cleanup(func() { _ = head.Close() })
			require.NoError(t, head.Init(0))

			// A series that went stale as a float and became non-stale as a histogram.
			crossType := labels.FromStrings("name", "cross_type")
			appendStaleTestSample(t, head, staleKindFloat, crossType, 100, true)
			appendStaleTestSample(t, head, staleKindHistogram, crossType, 200, false)
			// A genuinely stale series.
			staleOnly := labels.FromStrings("name", "stale_only")
			appendStaleTestSample(t, head, staleKindHistogram, staleOnly, 100, true)
			// A recent non-stale series, kept alive past the truncation point.
			keeper := labels.FromStrings("name", "keeper")
			appendStaleTestSample(t, head, staleKindFloat, keeper, 5000, false)
			require.Equal(t, uint64(1), head.NumStaleSeries())

			crossTypeRef := ref(head, crossType)
			staleOnlyRef := ref(head, staleOnly)
			keeperRef := ref(head, keeper)
			refs := []storage.SeriesRef{crossTypeRef, staleOnlyRef, keeperRef}
			staleRefs, err := head.staleSeriesRefsNoOOOData(context.Background())
			require.NoError(t, err)
			require.Equal(t, []storage.SeriesRef{staleOnlyRef}, staleRefs.sortedByRef)
			tc.remove(t, head, refs)

			require.Equal(t, tc.wantSeries, head.NumSeries())
			require.Equal(t, uint64(0), head.NumStaleSeries())
			if tc.crossTypeSurvives {
				require.NotNil(t, head.series.getByHash(crossType.Hash(), crossType), "cross-type non-stale series must survive")
			} else {
				require.Nil(t, head.series.getByHash(crossType.Hash(), crossType))
			}
		})
	}
}

// TestHead_NumStaleSeries_WALReplayIgnoresRejectedCrossTypeSamples verifies
// that an older sample appearing later in the WAL does not change stale-series
// accounting when replay rejects it. This ordering is possible when concurrent
// appenders log in one order but apply their samples to the series in another.
func TestHead_NumStaleSeries_WALReplayIgnoresRejectedCrossTypeSamples(t *testing.T) {
	stale := math.Float64frombits(value.StaleNaN)
	hist := tsdbutil.GenerateTestHistogram(1)
	staleHist := hist.Copy()
	staleHist.Sum = stale
	floatHist := tsdbutil.GenerateTestFloatHistogram(1)

	tests := []struct {
		name  string
		newer any
		older any
	}{
		{
			name: "histogram_then_older_float",
			newer: []record.RefHistogramSample{
				{Ref: 1, T: 200, H: staleHist},
			},
			older: []record.RefSample{
				{Ref: 1, T: 100, V: 1},
			},
		},
		{
			name: "float_then_older_histogram",
			newer: []record.RefSample{
				{Ref: 1, T: 200, V: stale},
			},
			older: []record.RefHistogramSample{
				{Ref: 1, T: 100, H: hist},
			},
		},
		{
			name: "float_then_older_float_histogram",
			newer: []record.RefSample{
				{Ref: 1, T: 200, V: stale},
			},
			older: []record.RefFloatHistogramSample{
				{Ref: 1, T: 100, FH: floatHist},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			head, wal := newTestHead(t, 1000, compression.None, false)
			t.Cleanup(func() { require.NoError(t, head.Close()) })

			populateTestWL(t, wal, []any{
				[]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("name", tc.name)}},
				[]record.RefSample{{Ref: 1, T: 0, V: 1}},
				tc.newer,
				tc.older,
			}, nil, false)

			require.NoError(t, head.Init(0))

			series := head.series.getByID(1)
			require.NotNil(t, series)
			require.Equal(t, int64(200), series.maxTime())
			require.True(t, isStaleSeries(series))
			require.Equal(t, uint64(1), head.NumSeries())
			require.Equal(t, uint64(1), head.NumStaleSeries())
		})
	}
}

// TestHead_WALReplayStaleMarkerTypeConsistency verifies that a float staleness
// marker appended to a native histogram series is replayed as a histogram (or
// float-histogram) staleness marker — matching the live commitFloats conversion
// — rather than as a float sample landing on a histogram series. This must hold
// even when the preceding histogram is only in an m-mapped chunk (whose samples
// WAL replay skips), so the marker type is recovered from the chunk encoding.
func TestHead_WALReplayStaleMarkerTypeConsistency(t *testing.T) {
	for _, floatHistogram := range []bool{false, true} {
		for _, mmapped := range []bool{false, true} {
			for _, stStorage := range []bool{false, true} {
				name := fmt.Sprintf("floatHistogram=%t/mmapped=%t/stStorage=%t", floatHistogram, mmapped, stStorage)
				t.Run(name, func(t *testing.T) {
					opts := newTestHeadDefaultOptions(1000, false)
					if stStorage {
						opts.EnableSTStorage.Store(true)
						opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
						opts.EnableHistogramSTEncoding.Store(true)
					}
					head, _ := newTestHeadWithOptions(t, compression.None, opts)
					t.Cleanup(func() {
						// Captures head by reference, so it closes the reopened head.
						_ = head.Close()
					})
					require.NoError(t, head.Init(0))

					lbls := labels.FromStrings("foo", "bar")
					wantType := chunkenc.ValHistogram
					if floatHistogram {
						wantType = chunkenc.ValFloatHistogram
					}

					// Establish the series as a native histogram.
					app := head.AppenderV2(context.Background())
					var err error
					if floatHistogram {
						_, err = app.Append(0, lbls, 0, 100, 0, nil, tsdbutil.GenerateTestFloatHistogram(1), storage.AOptions{})
					} else {
						_, err = app.Append(0, lbls, 0, 100, 0, tsdbutil.GenerateTestHistogram(1), nil, storage.AOptions{})
					}
					require.NoError(t, err)
					require.NoError(t, app.Commit())

					// For the m-mapped case the marker crosses the chunk-range boundary
					// so the histogram chunk becomes a completed predecessor that gets
					// m-mapped; on replay those histogram samples are skipped.
					markerT := int64(200)
					if mmapped {
						markerT = 1000
					}

					// Append the staleness marker in a separate appender so it is logged
					// as a float record (converted to a histogram marker for the chunk).
					app = head.AppenderV2(context.Background())
					_, err = app.Append(0, lbls, 0, markerT, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{})
					require.NoError(t, err)
					require.NoError(t, app.Commit())

					if mmapped {
						head.mmapHeadChunks()
						s := head.series.getByHash(lbls.Hash(), lbls)
						require.NotNil(t, s)
						s.Lock()
						nMmapped := len(s.mmappedChunks)
						s.Unlock()
						require.NotZero(t, nMmapped, "histogram chunk must be m-mapped for this case")
					}

					// Reopen, forcing WAL replay of the float staleness record.
					require.NoError(t, head.Close())
					wal, err := wlog.NewSize(nil, nil, filepath.Join(head.opts.ChunkDirRoot, "wal"), 32768, compression.None)
					require.NoError(t, err)
					head, err = NewHead(nil, nil, wal, nil, head.opts, nil)
					require.NoError(t, err)
					require.NoError(t, head.Init(0))

					q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, q.Close()) })

					ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
					require.True(t, ss.Next())
					it := ss.At().Iterator(nil)

					var gotType chunkenc.ValueType
					var found bool
					for vt := it.Next(); vt != chunkenc.ValNone; vt = it.Next() {
						if it.AtT() == markerT {
							gotType = vt
							found = true
						}
					}
					require.NoError(t, it.Err())
					require.False(t, ss.Next())
					require.NoError(t, ss.Err())
					require.Empty(t, ss.Warnings())
					require.True(t, found, "staleness marker not found")
					require.Equal(t, wantType, gotType, "replayed staleness marker must keep the series' native histogram type")
				})
			}
		}
	}
}

func TestHead_NumNativeHistogramSeriesAndBuckets(t *testing.T) {
	t.Parallel()
	head, _ := newTestHead(t, 1000, compression.None, false)
	t.Cleanup(func() {
		// Captures head by reference, so it closes the final head after restarts.
		_ = head.Close()
		// Release the captured reference so completed parallel tests do not retain the head.
		head = nil
	})
	require.NoError(t, head.Init(0))

	// Initially, there are no native histogram series or buckets.
	require.Equal(t, uint64(0), head.NumNativeHistogramSeries())
	require.Equal(t, uint64(0), head.NumNativeHistogramBuckets())

	appendSample := func(lbls labels.Labels, ts int64, val float64) {
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, lbls, 0, ts, val, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	appendHistogram := func(lbls labels.Labels, ts int64, val *histogram.Histogram) {
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, lbls, 0, ts, 0, val, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	appendFloatHistogram := func(lbls labels.Labels, ts int64, val *histogram.FloatHistogram) {
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, lbls, 0, ts, 0, nil, val, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	verifyCounts := func(numHistogramSeries, numHistogramBuckets, numSeries int) {
		t.Helper()
		require.Equal(t, uint64(numHistogramSeries), head.NumNativeHistogramSeries())
		require.Equal(t, uint64(numHistogramBuckets), head.NumNativeHistogramBuckets())
		require.Equal(t, uint64(numSeries), head.NumSeries())
	}

	restartHeadAndVerifyCounts := func(numHistogramSeries, numHistogramBuckets, numSeries int) {
		t.Helper()
		verifyCounts(numHistogramSeries, numHistogramBuckets, numSeries)

		require.NoError(t, head.Close())

		wal, err := wlog.NewSize(nil, nil, filepath.Join(head.opts.ChunkDirRoot, "wal"), 32768, compression.None)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, wal, nil, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(0))

		verifyCounts(numHistogramSeries, numHistogramBuckets, numSeries)
	}

	// The standard test histograms carry 4 positive and 4 negative buckets each.
	stdHist := tsdbutil.GenerateTestHistograms(3)
	require.Equal(t, 8, len(stdHist[0].PositiveBuckets)+len(stdHist[0].NegativeBuckets))
	stdFloatHist := tsdbutil.GenerateTestFloatHistograms(3)
	require.Equal(t, 8, len(stdFloatHist[0].PositiveBuckets)+len(stdFloatHist[0].NegativeBuckets))
	// A histogram with a different (3) bucket count, to exercise bucket deltas.
	threeBucketHist := &histogram.Histogram{
		// Buckets accumulate to 2, 3, 4, so the observation count is 9.
		Count:           9,
		ZeroThreshold:   0.001,
		Schema:          1,
		Sum:             10,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
		PositiveBuckets: []int64{2, 1, 1},
	}

	series1 := labels.FromStrings("name", "series1")
	series2 := labels.FromStrings("name", "series2")

	// Plain float series never count as native histogram series.
	appendSample(series1, 100, 1)
	verifyCounts(0, 0, 1)

	// A float series turning into a native histogram series is counted, and its
	// buckets are added.
	appendHistogram(series1, 110, stdHist[0])
	verifyCounts(1, 8, 1)

	// Another native histogram sample with the same bucket count keeps the counts.
	appendHistogram(series1, 120, stdHist[1])
	verifyCounts(1, 8, 1)

	// A new series that starts directly as a float histogram is counted.
	appendFloatHistogram(series2, 100, stdFloatHist[0])
	verifyCounts(2, 16, 2)

	restartHeadAndVerifyCounts(2, 16, 2)

	// A native histogram sample with fewer buckets adjusts the bucket gauge by
	// the delta (8 -> 3).
	appendHistogram(series1, 130, threeBucketHist)
	verifyCounts(2, 11, 2)

	// A float sample on a native histogram series removes it from both gauges.
	appendSample(series1, 140, 5)
	verifyCounts(1, 8, 2)

	restartHeadAndVerifyCounts(1, 8, 2)

	// A proper staleness marker (an empty histogram) keeps the series counted as
	// a native histogram series but drops its buckets to zero.
	staleFloatHist := &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)}
	appendFloatHistogram(series2, 110, staleFloatHist)
	verifyCounts(1, 0, 2)

	// A subsequent non-stale float histogram restores the bucket count.
	appendFloatHistogram(series2, 120, stdFloatHist[1])
	verifyCounts(1, 8, 2)

	// This will test restarting with snapshot.
	head.opts.EnableMemorySnapshotOnShutdown = true
	restartHeadAndVerifyCounts(1, 8, 2)

	// Garbage collection: a removed native histogram series decrements both gauges.
	// Keep series2 alive with a fresh sample, then truncate away the older series1.
	appendFloatHistogram(series2, 400, stdFloatHist[2])
	require.NoError(t, head.Truncate(300))
	head.gc()
	// series1 (float, last sample at ts 140) is gone; series2 remains as a native
	// histogram series with 8 buckets.
	verifyCounts(1, 8, 1)

	// A brand new series starting directly as a native histogram is counted.
	series3 := labels.FromStrings("name", "series3")
	appendHistogram(series3, 400, stdHist[2])
	verifyCounts(2, 16, 2)

	// GC of that native histogram series decrements the gauges again.
	appendFloatHistogram(series2, 500, stdFloatHist[0])
	require.NoError(t, head.Truncate(450))
	head.gc()
	// series3 (last sample at ts 400) is removed, series2 remains.
	verifyCounts(1, 8, 1)

	t.Run("stale payload buckets are counted", func(t *testing.T) {
		variants := []struct {
			name      string
			newSample func() (*histogram.Histogram, *histogram.FloatHistogram)
		}{
			{
				name: "histogram",
				newSample: func() (*histogram.Histogram, *histogram.FloatHistogram) {
					return tsdbutil.GenerateTestHistograms(1)[0], nil
				},
			},
			{
				name: "float_histogram",
				newSample: func() (*histogram.Histogram, *histogram.FloatHistogram) {
					return nil, tsdbutil.GenerateTestFloatHistograms(1)[0]
				},
			},
			{
				name: "custom_bucket_histogram",
				newSample: func() (*histogram.Histogram, *histogram.FloatHistogram) {
					return tsdbutil.GenerateTestCustomBucketsHistogram(1), nil
				},
			},
			{
				name: "custom_bucket_float_histogram",
				newSample: func() (*histogram.Histogram, *histogram.FloatHistogram) {
					return nil, tsdbutil.GenerateTestCustomBucketsFloatHistogram(1)
				},
			},
		}

		for _, snapshot := range []bool{false, true} {
			for _, variant := range variants {
				name := fmt.Sprintf("snapshot=%t/%s", snapshot, variant.name)
				t.Run(name, func(t *testing.T) {
					testHead, _ := newTestHead(t, 1000, compression.None, false)
					testHead.opts.EnableMemorySnapshotOnShutdown = snapshot
					t.Cleanup(func() { _ = testHead.Close() })
					require.NoError(t, testHead.Init(0))

					appendSample := func(lbls labels.Labels, ts int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) {
						t.Helper()
						app := testHead.AppenderV2(t.Context())
						_, err := app.Append(0, lbls, 0, ts, v, h, fh, storage.AOptions{})
						require.NoError(t, err)
						require.NoError(t, app.Commit())
					}

					lbls := labels.FromStrings("name", t.Name())
					h, fh := variant.newSample()
					payloadBuckets := 0
					if h != nil {
						payloadBuckets = len(h.PositiveBuckets) + len(h.NegativeBuckets)
					} else {
						payloadBuckets = len(fh.PositiveBuckets) + len(fh.NegativeBuckets)
					}
					require.Positive(t, payloadBuckets)

					if h != nil {
						h.Sum = math.Float64frombits(value.StaleNaN)
					} else {
						fh.Sum = math.Float64frombits(value.StaleNaN)
					}
					appendSample(lbls, 100, 0, h, fh)
					require.Equal(t, uint64(1), testHead.NumStaleSeries())
					require.Equal(t, uint64(1), testHead.NumNativeHistogramSeries())
					require.Equal(t, uint64(payloadBuckets), testHead.NumNativeHistogramBuckets())

					opts := testHead.opts
					require.NoError(t, testHead.Close())
					wal, err := wlog.NewSize(nil, nil, filepath.Join(opts.ChunkDirRoot, "wal"), 32768, compression.None)
					require.NoError(t, err)
					testHead, err = NewHead(nil, nil, wal, nil, opts, nil)
					require.NoError(t, err)
					require.NoError(t, testHead.Init(0))

					require.Equal(t, uint64(1), testHead.NumStaleSeries())
					require.Equal(t, uint64(1), testHead.NumNativeHistogramSeries())
					require.Equal(t, uint64(payloadBuckets), testHead.NumNativeHistogramBuckets())

					series := testHead.series.getByHash(lbls.Hash(), lbls)
					require.NotNil(t, series)
					staleRefs := []storage.SeriesRef{storage.SeriesRef(series.ref)}
					require.NoError(t, testHead.truncateStaleSeries(staleRefs, 100, testHead.snapshotFingerprints(staleRefs, math.MaxUint64)))
					require.Zero(t, testHead.NumSeries())
					require.Zero(t, testHead.NumStaleSeries())
					require.Zero(t, testHead.NumNativeHistogramSeries())
					require.Zero(t, testHead.NumNativeHistogramBuckets())
				})
			}
		}
	})
}

// TestHead_FilterSelectedSeriesAndSortPostings exercises the helper directly, covering the
// three outcomes for a given ref: kept (clean series), skipped (series carries OOO data), and
// silently dropped (ref does not resolve to any series in the head).
func TestHead_FilterSelectedSeriesAndSortPostings(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	t.Cleanup(func() { _ = head.Close() })
	require.NoError(t, head.Init(0))

	// Enable OOO so we can push an OOO sample.
	head.opts.OutOfOrderTimeWindow.Store(1000)

	clean := labels.FromStrings("name", "clean")
	withOOO := labels.FromStrings("name", "with-ooo")

	app := head.AppenderV2(context.Background())
	cleanRef, err := app.Append(0, clean, 0, 200, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	withOOORef, err := app.Append(0, withOOO, 0, 200, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Push an OOO sample for the second series so its s.ooo becomes non-nil.
	app = head.AppenderV2(context.Background())
	_, err = app.Append(withOOORef, withOOO, 0, 100, 2.5, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// A ref that does not resolve to any series in the head must be silently dropped.
	bogusRef := storage.SeriesRef(math.MaxUint64)

	kept, err := head.filterSeriesAndSortPostings(index.NewListPostings([]storage.SeriesRef{cleanRef, withOOORef, bogusRef}), isSeriesWithoutOOO)
	require.NoError(t, err)
	require.Equal(t, seriesRefs{sortedByRef: []storage.SeriesRef{cleanRef}, sortedByLabels: []storage.SeriesRef{cleanRef}}, kept)
}

// TestHeadAppender_WALEncoder_EnableSTStorage verifies that when EnableSTStorage
// is true the WAL encoder writes SamplesV2 records, and when false it writes
// plain Samples (V1) records. The bug was that log() always created a zero-value
// record.Encoder (EnableSTStorage=false), ignoring the head option.
func TestHeadAppender_WALEncoder_EnableSTStorage(t *testing.T) {
	for _, enableST := range []bool{false, true} {
		t.Run(fmt.Sprintf("enableSTStorage=%v", enableST), func(t *testing.T) {
			opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
			opts.EnableSTStorage.Store(enableST)
			if enableST {
				opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
			} else {
				opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
			}
			opts.EnableHistogramSTEncoding.Store(enableST)
			h, w := newTestHeadWithOptions(t, compression.None, opts)

			lbls := labels.FromStrings("foo", "bar")
			app := h.AppenderV2(context.Background())
			for ts := int64(100); ts < 110; ts++ {
				_, err := app.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
				require.NoError(t, err)
			}
			require.NoError(t, app.Commit())
			require.NoError(t, h.Close())

			// Read WAL segments directly and check the sample record type.
			sr, err := wlog.NewSegmentsReader(w.Dir())
			require.NoError(t, err)
			defer func() { require.NoError(t, sr.Close()) }()

			dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
			r := wlog.NewReader(sr)

			var foundSampleRecord bool
			for r.Next() {
				rt := dec.Type(r.Record())
				switch rt {
				case record.Samples:
					require.False(t, enableST, "WAL contains Samples (V1) record but EnableSTStorage=true, expected SamplesV2")
					foundSampleRecord = true
				case record.SamplesV2:
					require.True(t, enableST, "WAL contains SamplesV2 record but EnableSTStorage=false, expected Samples (V1)")
					foundSampleRecord = true
				}
			}
			require.NoError(t, r.Err())
			require.True(t, foundSampleRecord, "no sample record found in WAL")
		})
	}
}

// TestHeadAppender_WBLEncoder_EnableSTStorage verifies that when EnableSTStorage
// is true the WBL encoder writes SamplesV2 records for out-of-order samples, and
// when false it writes plain Samples (V1) records. The bug was that collectOOORecords()
// always created record.Encoder{EnableSTStorage: false}, ignoring the head option.
func TestHeadAppender_WBLEncoder_EnableSTStorage(t *testing.T) {
	for _, enableST := range []bool{false, true} {
		t.Run(fmt.Sprintf("enableSTStorage=%v", enableST), func(t *testing.T) {
			dir := t.TempDir()
			wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.None)
			require.NoError(t, err)
			wbl, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.None)
			require.NoError(t, err)

			opts := DefaultHeadOptions()
			opts.ChunkRange = DefaultBlockDuration
			opts.ChunkDirRoot = dir
			opts.OutOfOrderTimeWindow.Store(60 * time.Minute.Milliseconds())
			opts.EnableSTStorage.Store(enableST)
			if enableST {
				opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
			} else {
				opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
			}
			opts.EnableHistogramSTEncoding.Store(enableST)

			h, err := NewHead(nil, nil, wal, wbl, opts, nil)
			require.NoError(t, err)
			require.NoError(t, h.Init(0))
			t.Cleanup(func() { _ = h.Close() })

			lbls := labels.FromStrings("foo", "bar")

			// Append an in-order sample to establish head maxt.
			app := h.AppenderV2(context.Background())
			_, err = app.Append(0, lbls, 0, 200, 200, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			// Append OOO samples; these are written to the WBL.
			app = h.AppenderV2(context.Background())
			for ts := int64(100); ts < 110; ts++ {
				_, err = app.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
				require.NoError(t, err)
			}
			require.NoError(t, app.Commit())

			require.NoError(t, h.Close())

			// Read WBL segments directly and check the sample record type.
			sr, err := wlog.NewSegmentsReader(filepath.Join(dir, wlog.WblDirName))
			require.NoError(t, err)
			defer func() { require.NoError(t, sr.Close()) }()

			dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
			r := wlog.NewReader(sr)

			var foundSampleRecord bool
			for r.Next() {
				rt := dec.Type(r.Record())
				switch rt {
				case record.Samples:
					require.False(t, enableST, "WBL contains Samples (V1) record but EnableSTStorage=true, expected SamplesV2")
					foundSampleRecord = true
				case record.SamplesV2:
					require.True(t, enableST, "WBL contains SamplesV2 record but EnableSTStorage=false, expected Samples (V1)")
					foundSampleRecord = true
				}
			}
			require.NoError(t, r.Err())
			require.True(t, foundSampleRecord, "no sample record found in WBL")
		})
	}
}

// TestHeadAppender_STStorage_Disabled verifies that when EnableSTStorage is false,
// start timestamps are NOT stored in chunks (AtST returns 0).
func TestHeadAppender_STStorage_Disabled(t *testing.T) {
	type sampleData struct {
		st      int64
		ts      int64
		fSample float64
	}

	samples := []sampleData{
		{st: 10, ts: 100, fSample: 1.0},
		{st: 20, ts: 200, fSample: 2.0},
		{st: 30, ts: 300, fSample: 3.0},
	}

	opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts.EnableSTStorage.Store(false) // Explicitly disable ST storage.
	h, _ := newTestHeadWithOptions(t, compression.None, opts)

	lbls := labels.FromStrings("foo", "bar")

	a := h.AppenderV2(context.Background())
	for _, s := range samples {
		_, err := a.Append(0, lbls, s.st, s.ts, s.fSample, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, a.Commit())

	ctx := context.Background()
	idxReader, err := h.Index()
	require.NoError(t, err)
	defer idxReader.Close()

	chkReader, err := h.Chunks()
	require.NoError(t, err)
	defer chkReader.Close()

	p, err := idxReader.Postings(ctx, "foo", "bar")
	require.NoError(t, err)

	var lblBuilder labels.ScratchBuilder
	require.True(t, p.Next())
	sRef := p.At()

	var chkMetas []chunks.Meta
	require.NoError(t, idxReader.Series(sRef, &lblBuilder, &chkMetas))

	for _, meta := range chkMetas {
		chk, iterable, err := chkReader.ChunkOrIterable(meta)
		require.NoError(t, err)
		require.Nil(t, iterable)

		it := chk.Iterator(nil)
		for it.Next() != chunkenc.ValNone {
			st := it.AtST()
			require.Equal(t, int64(0), st, "ST should be 0 when EnableSTStorage is false")
		}
		require.NoError(t, it.Err())
	}
}

// TestHeadAppender_STStorage_WALReplay verifies that ST values are preserved
// across a WAL replay for floats, histograms, and float histograms when
// EnableSTStorage and EnableHistogramSTEncoding are true. The original bug was
// that Commit() hardcoded EnableSTStorage=false in the WAL encoder, so ST
// values were written as V1 records (without ST) and lost on replay.
func TestHeadAppender_STStorage_WALReplay(t *testing.T) {
	type sampleCase struct {
		name   string
		append func(storage.AppenderV2, labels.Labels, int64, int64) (chunks.Sample, error)
	}

	const st = int64(50)
	for _, tc := range []sampleCase{
		{
			name: "float",
			append: func(app storage.AppenderV2, lbls labels.Labels, st, ts int64) (chunks.Sample, error) {
				_, err := app.Append(0, lbls, st, ts, float64(ts), nil, nil, storage.AOptions{})
				return sample{st: st, t: ts, f: float64(ts)}, err
			},
		},
		{
			name: "histogram",
			append: func(app storage.AppenderV2, lbls labels.Labels, st, ts int64) (chunks.Sample, error) {
				h := tsdbutil.GenerateTestHistogram(ts)
				h.CounterResetHint = histogram.NotCounterReset
				_, err := app.Append(0, lbls, st, ts, 0, h, nil, storage.AOptions{})
				return sample{st: st, t: ts, h: h}, err
			},
		},
		{
			name: "float histogram",
			append: func(app storage.AppenderV2, lbls labels.Labels, st, ts int64) (chunks.Sample, error) {
				fh := tsdbutil.GenerateTestFloatHistogram(ts)
				fh.CounterResetHint = histogram.NotCounterReset
				_, err := app.Append(0, lbls, st, ts, 0, nil, fh, storage.AOptions{})
				return sample{st: st, t: ts, fh: fh}, err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
			opts.EnableSTStorage.Store(true)
			opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
			opts.EnableHistogramSTEncoding.Store(true)
			h, w := newTestHeadWithOptions(t, compression.None, opts)

			lbls := labels.FromStrings("foo", "bar")

			a := h.AppenderV2(context.Background())
			var expected []chunks.Sample
			for ts := int64(100); ts < 200; ts++ {
				s, err := tc.append(a, lbls, st, ts)
				require.NoError(t, err)
				expected = append(expected, s)
			}
			require.NoError(t, a.Commit())
			require.NoError(t, h.Close())

			// Reopen the head, triggering WAL replay.
			w, err := wlog.New(nil, nil, w.Dir(), compression.None)
			require.NoError(t, err)
			opts.ChunkDirRoot = h.opts.ChunkDirRoot
			h2, err := NewHead(nil, nil, w, nil, opts, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = h2.Close() })
			require.NoError(t, h2.Init(0))

			q, err := NewBlockQuerier(h2, 100, 199)
			require.NoError(t, err)
			key := `{foo="bar"}`
			got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
			requireEqualSeries(t, map[string][]chunks.Sample{`{foo="bar"}`: expected}, got, true)
			for i, e := range expected {
				require.Equal(t, e.ST(), got[key][i].ST(), "ST mismatch at [%d]", i)
			}
		})
	}
}

// TestWALReplayStoreSTPassed is a regression test verifying that processWALSamples
// passes storeST to chunkOpts. Without storeST, a WAL written with EncXOR would be
// replayed into an EncXOR2+STStorage head without cutting the in-progress XOR chunk
// (CompatibleValues returns true for the XOR family), silently continuing a chunk that
// cannot hold start timestamps. With storeST set, the encoding mismatch forces an
// immediate chunk cut.
func TestWALReplayStoreSTPassed(t *testing.T) {
	// Phase 1: write a WAL with EncXOR and no ST storage.
	opts1 := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts1.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
	h, w := newTestHeadWithOptions(t, compression.None, opts1)

	lbls := labels.FromStrings("foo", "bar")

	app := h.AppenderV2(context.Background())
	for ts := int64(1); ts <= 5; ts++ {
		_, err := app.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.NoError(t, h.Close())

	// Phase 2: replay the WAL with EncXOR2 + ST storage enabled. If storeST were
	// not passed during WAL replay, the XOR chunk loaded from the chunk snapshot
	// would be continued with EncXOR2 appends (CompatibleValues = true, no cut), which
	// would produce a chunk that cannot store start timestamps.
	opts2 := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts2.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
	opts2.EnableSTStorage.Store(true)
	opts2.ChunkDirRoot = h.opts.ChunkDirRoot

	w2, err := wlog.New(nil, nil, w.Dir(), compression.None)
	require.NoError(t, err)
	h2, err := NewHead(nil, nil, w2, nil, opts2, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h2.Close() })
	require.NoError(t, h2.Init(0))

	// Append one post-replay sample with a start timestamp to confirm the active
	// chunk is EncXOR2 (which can store ST).
	app2 := h2.AppenderV2(context.Background())
	_, err = app2.Append(0, lbls, 0, 10, 10.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app2.Commit())

	ms, created, err := h2.getOrCreate(lbls.Hash(), lbls, false)
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, ms.headChunks)
	require.Equal(t, chunkenc.EncXOR2, ms.headChunks.chunk.Encoding(),
		"active chunk after WAL replay into EncXOR2+STStorage head must use EncXOR2")
}

// TestHeadAppender_STStorage_WBLReplay verifies that ST values are preserved
// across a WBL replay for out-of-order floats, histograms, and float
// histograms when EnableSTStorage and EnableHistogramSTEncoding are true. The
// original bug was that collectOOORecords() hardcoded EnableSTStorage=false in
// the WBL encoder (acc.enc), so OOO sample ST values were written as V1
// records (without ST) and lost on WBL replay.
func TestHeadAppender_STStorage_WBLReplay(t *testing.T) {
	type sampleCase struct {
		name      string
		append    func(storage.AppenderV2, labels.Labels, int64, int64) (chunks.Sample, error)
		expandOOO func(chunkenc.Iterator) ([]chunks.Sample, error)
	}

	const st = int64(50)
	for _, tc := range []sampleCase{
		{
			name: "float",
			append: func(app storage.AppenderV2, lbls labels.Labels, st, ts int64) (chunks.Sample, error) {
				_, err := app.Append(0, lbls, st, ts, float64(ts), nil, nil, storage.AOptions{})
				return sample{st: st, t: ts, f: float64(ts)}, err
			},
			expandOOO: func(it chunkenc.Iterator) ([]chunks.Sample, error) {
				var got []chunks.Sample
				for it.Next() != chunkenc.ValNone {
					t, v := it.At()
					got = append(got, sample{st: it.AtST(), t: t, f: v})
				}
				return got, it.Err()
			},
		},
		{
			name: "histogram",
			append: func(app storage.AppenderV2, lbls labels.Labels, st, ts int64) (chunks.Sample, error) {
				h := tsdbutil.GenerateTestHistogram(ts)
				h.CounterResetHint = histogram.NotCounterReset
				_, err := app.Append(0, lbls, st, ts, 0, h, nil, storage.AOptions{})
				return sample{st: st, t: ts, h: h}, err
			},
			expandOOO: func(it chunkenc.Iterator) ([]chunks.Sample, error) {
				var got []chunks.Sample
				for it.Next() != chunkenc.ValNone {
					t, h := it.AtHistogram(nil)
					got = append(got, sample{st: it.AtST(), t: t, h: h})
				}
				return got, it.Err()
			},
		},
		{
			name: "float histogram",
			append: func(app storage.AppenderV2, lbls labels.Labels, st, ts int64) (chunks.Sample, error) {
				fh := tsdbutil.GenerateTestFloatHistogram(ts)
				fh.CounterResetHint = histogram.NotCounterReset
				_, err := app.Append(0, lbls, st, ts, 0, nil, fh, storage.AOptions{})
				return sample{st: st, t: ts, fh: fh}, err
			},
			expandOOO: func(it chunkenc.Iterator) ([]chunks.Sample, error) {
				var got []chunks.Sample
				for it.Next() != chunkenc.ValNone {
					t, fh := it.AtFloatHistogram(nil)
					got = append(got, sample{st: it.AtST(), t: t, fh: fh})
				}
				return got, it.Err()
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.None)
			require.NoError(t, err)
			wbl, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.None)
			require.NoError(t, err)

			opts := DefaultHeadOptions()
			opts.ChunkRange = DefaultBlockDuration
			opts.ChunkDirRoot = dir
			opts.OutOfOrderTimeWindow.Store(60 * time.Minute.Milliseconds())
			opts.EnableSTStorage.Store(true)
			opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
			opts.EnableHistogramSTEncoding.Store(true)

			h, err := NewHead(nil, nil, wal, wbl, opts, nil)
			require.NoError(t, err)
			require.NoError(t, h.Init(0))

			lbls := labels.FromStrings("foo", "bar")

			// Append an in-order sample to establish the head's maxt.
			app := h.AppenderV2(context.Background())
			_, err = tc.append(app, lbls, st, 200)
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			// Append OOO samples with non-zero ST; these go to the WBL.
			// Use fewer than DefaultOutOfOrderCapMax (32) samples so they all stay in the
			// OOO head chunk (not mmap'd) and are exclusively recovered via WBL replay.
			app = h.AppenderV2(context.Background())
			var expected []chunks.Sample
			for ts := int64(100); ts < 120; ts++ {
				s, err := tc.append(app, lbls, st, ts)
				require.NoError(t, err)
				expected = append(expected, s)
			}
			require.NoError(t, app.Commit())
			require.NoError(t, h.Close())

			// Reopen the head, triggering WBL replay.
			wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.None)
			require.NoError(t, err)
			wbl, err = wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.None)
			require.NoError(t, err)
			h2, err := NewHead(nil, nil, wal, wbl, opts, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = h2.Close() })
			require.NoError(t, h2.Init(0))

			// Access the OOO head chunk directly and verify samples survived WBL replay.
			ms, created, err := h2.getOrCreate(lbls.Hash(), lbls, false)
			require.NoError(t, err)
			require.False(t, created)
			require.NotNil(t, ms.ooo)
			require.NotNil(t, ms.ooo.oooHeadChunk)

			chks, err := ms.ooo.oooHeadChunk.chunk.ToEncodedChunks(math.MinInt64, math.MaxInt64, true, true)
			require.NoError(t, err)
			require.Len(t, chks, 1)

			got, err := tc.expandOOO(chks[0].chunk.Iterator(nil))
			require.NoError(t, err)
			requireEqualSamples(t, "ooo", expected, got, requireEqualSamplesIgnoreCounterResets)
			for i, e := range expected {
				require.Equal(t, e.ST(), got[i].ST(), "ST mismatch at [%d]", i)
			}
		})
	}
}

// TestHeadAppender_STStorage_WALReplay_CrossEncoding verifies that WAL replay
// correctly handles a cross-encoding migration: samples written with EncXOR and
// no ST storage are replayed with EncXOR2+STStorage enabled. Since the original
// WAL records are V1 (no ST), the replayed samples have ST=0. The in-memory
// chunks must use EncXOR2 (the replay-time encoding).
func TestHeadAppender_STStorage_WALReplay_CrossEncoding(t *testing.T) {
	// Phase 1: write with EncXOR, no ST storage.
	opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts.EnableSTStorage.Store(false)
	opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
	h, w := newTestHeadWithOptions(t, compression.None, opts)

	lbls := labels.FromStrings("foo", "bar")

	a := h.AppenderV2(context.Background())
	for ts := int64(100); ts < 110; ts++ {
		_, err := a.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, a.Commit())
	require.NoError(t, h.Close())

	// Phase 2: reopen with EncXOR2+STStorage=true (migration to ST storage).
	w, err := wlog.New(nil, nil, w.Dir(), compression.None)
	require.NoError(t, err)
	opts.ChunkDirRoot = h.opts.ChunkDirRoot
	opts.EnableSTStorage.Store(true)
	opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
	h2, err := NewHead(nil, nil, w, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h2.Close() })
	require.NoError(t, h2.Init(0))

	// Data must survive the replay; ST values are 0 because the WAL records
	// were written without ST storage (V1 records carry no ST).
	q, err := NewBlockQuerier(h2, 100, 109)
	require.NoError(t, err)
	got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	var expected []chunks.Sample
	for ts := int64(100); ts < 110; ts++ {
		expected = append(expected, sample{0, ts, float64(ts), nil, nil})
	}
	require.Equal(t, map[string][]chunks.Sample{`{foo="bar"}`: expected}, got)

	// Verify the in-memory head chunk uses EncXOR2 (replay-time encoding).
	ms, created, err := h2.getOrCreate(lbls.Hash(), lbls, false)
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, ms.headChunks)
	require.Equal(t, chunkenc.EncXOR2, ms.headChunks.chunk.Encoding(),
		"chunks after replay into EncXOR2+STStorage head must use EncXOR2")
}

// TestHeadAppender_STStorage_WALReplay_CrossEncoding_Reverse verifies the reverse
// cross-encoding migration: samples written with EncXOR2+STStorage enabled (so the
// WAL records are V2 and carry start timestamps) are replayed into a head with
// EncXOR and ST storage disabled. The sample timestamps and values must survive,
// but the start timestamps are dropped (ST=0) because EncXOR chunks cannot store
// them and ST storage is disabled. The in-memory chunks must use EncXOR (the
// replay-time encoding).
func TestHeadAppender_STStorage_WALReplay_CrossEncoding_Reverse(t *testing.T) {
	// Phase 1: write with EncXOR2 and ST storage enabled.
	opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
	opts.EnableSTStorage.Store(true)
	opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
	h, w := newTestHeadWithOptions(t, compression.None, opts)

	lbls := labels.FromStrings("foo", "bar")

	a := h.AppenderV2(context.Background())
	for ts := int64(100); ts < 110; ts++ {
		_, err := a.Append(0, lbls, 50, ts, float64(ts), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, a.Commit())
	require.NoError(t, h.Close())

	// Phase 2: reopen with EncXOR and ST storage disabled (migration away from ST storage).
	w, err := wlog.New(nil, nil, w.Dir(), compression.None)
	require.NoError(t, err)
	opts.ChunkDirRoot = h.opts.ChunkDirRoot
	opts.EnableSTStorage.Store(false)
	opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
	h2, err := NewHead(nil, nil, w, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h2.Close() })
	require.NoError(t, h2.Init(0))

	// Data must survive the replay; ST values are dropped to 0 because the head
	// has ST storage disabled and EncXOR chunks do not store start timestamps.
	q, err := NewBlockQuerier(h2, 100, 109)
	require.NoError(t, err)
	got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))

	var expected []chunks.Sample
	for ts := int64(100); ts < 110; ts++ {
		expected = append(expected, sample{0, ts, float64(ts), nil, nil})
	}
	require.Equal(t, map[string][]chunks.Sample{`{foo="bar"}`: expected}, got)

	// Verify the in-memory head chunk uses EncXOR (replay-time encoding).
	ms, created, err := h2.getOrCreate(lbls.Hash(), lbls, false)
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, ms.headChunks)
	require.Equal(t, chunkenc.EncXOR, ms.headChunks.chunk.Encoding(),
		"chunks after replay into EncXOR+no-ST head must use EncXOR")
}

// TestHeadAppender_STStorage_ChunkEncoding verifies that the correct chunk encoding
// is used based on EnableSTStorage setting.
func TestHeadAppender_STStorage_ChunkEncoding(t *testing.T) {
	testHistogram := tsdbutil.GenerateTestHistogram(1)
	testFloatHistogram := tsdbutil.GenerateTestFloatHistogram(1)

	type appendableSample struct {
		st      int64
		ts      int64
		fSample float64
		h       *histogram.Histogram
		fh      *histogram.FloatHistogram
	}

	for _, tc := range []struct {
		name            string
		samples         []appendableSample
		stEncoding      chunkenc.Encoding
		regularEncoding chunkenc.Encoding
	}{
		{
			name: "float samples",
			samples: []appendableSample{
				{st: 10, ts: 100, fSample: 1.0},
				{st: 20, ts: 200, fSample: 2.0},
			},
			stEncoding:      chunkenc.EncXOR2,
			regularEncoding: chunkenc.EncXOR,
		},
		{
			name: "histogram samples",
			samples: []appendableSample{
				{st: 10, ts: 100, h: testHistogram},
				{st: 20, ts: 200, h: testHistogram},
			},
			stEncoding:      chunkenc.EncHistogramST,
			regularEncoding: chunkenc.EncHistogram,
		},
		{
			name: "float histogram samples",
			samples: []appendableSample{
				{st: 10, ts: 100, fh: testFloatHistogram},
				{st: 20, ts: 200, fh: testFloatHistogram},
			},
			stEncoding:      chunkenc.EncFloatHistogramST,
			regularEncoding: chunkenc.EncFloatHistogram,
		},
	} {
		for _, enableST := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/EnableSTStorage=%t", tc.name, enableST), func(t *testing.T) {
				opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
				opts.EnableSTStorage.Store(enableST)
				if enableST {
					opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
				} else {
					opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
				}
				opts.EnableHistogramSTEncoding.Store(enableST)
				h, _ := newTestHeadWithOptions(t, compression.None, opts)

				lbls := labels.FromStrings("foo", "bar")
				a := h.AppenderV2(context.Background())
				for _, s := range tc.samples {
					_, err := a.Append(0, lbls, s.st, s.ts, s.fSample, s.h, s.fh, storage.AOptions{})
					require.NoError(t, err)
				}
				require.NoError(t, a.Commit())

				ctx := context.Background()
				idxReader, err := h.Index()
				require.NoError(t, err)
				defer idxReader.Close()

				chkReader, err := h.Chunks()
				require.NoError(t, err)
				defer chkReader.Close()

				p, err := idxReader.Postings(ctx, "foo", "bar")
				require.NoError(t, err)

				var lblBuilder labels.ScratchBuilder
				require.True(t, p.Next())
				sRef := p.At()

				var chkMetas []chunks.Meta
				require.NoError(t, idxReader.Series(sRef, &lblBuilder, &chkMetas))
				require.NotEmpty(t, chkMetas)

				for _, meta := range chkMetas {
					chk, iterable, err := chkReader.ChunkOrIterable(meta)
					require.NoError(t, err)
					require.Nil(t, iterable)

					encoding := chk.Encoding()
					if enableST {
						require.Equal(t, tc.stEncoding, encoding,
							"Expected ST-capable encoding when EnableSTStorage is true")
					} else {
						require.Equal(t, tc.regularEncoding, encoding,
							"Expected regular encoding when EnableSTStorage is false")
					}
				}
			})
		}
	}
}

// TestFloatChunkEncodingSwitchWaitsForNextChunk validates the chunk-cut behaviour when
// the float chunk encoding changes between XOR and XOR2. When ST storage is disabled
// the two encodings are compatible and in-progress chunks are not cut; the new encoding
// takes effect only when the current chunk fills up naturally. When ST storage is enabled
// the encodings are not compatible (XOR cannot store start timestamps) and an encoding
// switch — in either direction (XOR2→XOR or XOR→XOR2) — must cut the current chunk
// immediately.
func TestFloatChunkEncodingSwitchWaitsForNextChunk(t *testing.T) {
	t.Run("without_st_storage", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
		opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
		h, _ := newTestHeadWithOptions(t, compression.None, opts)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("foo", "bar")

		// Phase 1: Start appending while XOR2 is enabled.
		app1 := h.AppenderV2(context.Background())
		for ts := int64(1); ts <= 5; ts++ {
			_, err := app1.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app1.Commit())

		// Simulate a config reload that switches encoding to XOR.
		h.opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))

		// A new appender picks up useXOR2=false, but the existing XOR2 chunk must
		// not be cut — XOR and XOR2 are compatible when ST is disabled.
		app2 := h.AppenderV2(context.Background())
		for ts := int64(6); ts <= 10; ts++ {
			_, err := app2.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app2.Commit())

		// The head chunk must still be XOR2: encoding change does not cut chunks.
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms.headChunks)
		require.Nil(t, ms.headChunks.prev, "chunk must not have been cut; only one chunk must exist after encoding switch without ST storage")
		require.Equal(t, chunkenc.EncXOR2, ms.headChunks.chunk.Encoding(),
			"in-progress chunk must keep XOR2 encoding after encoding switch")

		// Phase 2: Fill the current chunk past its natural size limit so that a new
		// chunk is started. The new chunk must use the updated XOR encoding.
		// We append enough samples to guarantee at least one natural chunk cut.
		app3 := h.AppenderV2(context.Background())
		for ts := int64(11); ts <= int64(DefaultSamplesPerChunk*3); ts++ {
			_, err := app3.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app3.Commit())

		ms, _, err = h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.NotNil(t, ms.headChunks)
		require.NotNil(t, ms.headChunks.prev, "at least one chunk cut must have occurred during phase 2")
		require.Equal(t, chunkenc.EncXOR, ms.headChunks.chunk.Encoding(),
			"new chunk after natural boundary must use XOR encoding")
	})

	t.Run("with_st_storage", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
		opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
		opts.EnableSTStorage.Store(true)
		h, _ := newTestHeadWithOptions(t, compression.None, opts)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("foo", "bar")

		// Phase 1: Start appending while XOR2 is enabled.
		app1 := h.AppenderV2(context.Background())
		for ts := int64(1); ts <= 5; ts++ {
			_, err := app1.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app1.Commit())

		// Verify the in-progress chunk is XOR2.
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, chunkenc.EncXOR2, ms.headChunks.chunk.Encoding(),
			"chunk must be XOR2 before encoding switch")

		// Simulate a config reload that switches encoding to XOR.
		h.opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))

		// With ST storage enabled, XOR and XOR2 are NOT compatible. The first
		// append after the encoding switch must cut the current XOR2 chunk and
		// start a new XOR chunk.
		app2 := h.AppenderV2(context.Background())
		_, err = app2.Append(0, lbls, 0, 6, 6, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app2.Commit())

		ms, _, err = h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.NotNil(t, ms.headChunks)
		require.Equal(t, chunkenc.EncXOR, ms.headChunks.chunk.Encoding(),
			"encoding switch with ST storage must cut chunk immediately")
		require.NotNil(t, ms.headChunks.prev,
			"the old XOR2 chunk must exist as a previous chunk after the cut")
		require.Equal(t, chunkenc.EncXOR2, ms.headChunks.prev.chunk.Encoding(),
			"the evicted chunk must retain its original XOR2 encoding")
	})

	t.Run("with_st_storage_xor_to_xor2", func(t *testing.T) {
		// Start with XOR encoding and then switch to XOR2 while ST storage is
		// enabled. The encoding switch must cut the current XOR chunk immediately
		// because XOR does not store start timestamps.
		opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
		opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
		opts.EnableSTStorage.Store(true)
		h, _ := newTestHeadWithOptions(t, compression.None, opts)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("foo", "bar")

		// Phase 1: Start appending while XOR is active.
		app1 := h.AppenderV2(context.Background())
		for ts := int64(1); ts <= 5; ts++ {
			_, err := app1.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app1.Commit())

		// Verify the in-progress chunk is XOR.
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, chunkenc.EncXOR, ms.headChunks.chunk.Encoding(),
			"chunk must be XOR before encoding switch")

		// Simulate a config reload that switches encoding to XOR2.
		h.opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))

		// With ST storage enabled, the first append must cut the XOR chunk and
		// start a new XOR2 chunk.
		app2 := h.AppenderV2(context.Background())
		_, err = app2.Append(0, lbls, 0, 6, 6, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app2.Commit())

		ms, _, err = h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.NotNil(t, ms.headChunks)
		require.Equal(t, chunkenc.EncXOR2, ms.headChunks.chunk.Encoding(),
			"encoding switch with ST storage must cut chunk immediately")
		require.NotNil(t, ms.headChunks.prev,
			"the old XOR chunk must exist as a previous chunk after the cut")
		require.Equal(t, chunkenc.EncXOR, ms.headChunks.prev.chunk.Encoding(),
			"the evicted chunk must retain its original XOR encoding")
	})

	t.Run("without_st_storage_xor_to_xor2", func(t *testing.T) {
		// Start with XOR encoding and switch to XOR2 while ST storage is disabled.
		// The existing XOR chunk must not be cut — XOR and XOR2 are compatible
		// when ST is disabled.
		opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
		opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
		opts.EnableSTStorage.Store(false)
		h, _ := newTestHeadWithOptions(t, compression.None, opts)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("foo", "bar")

		app1 := h.AppenderV2(context.Background())
		for ts := int64(1); ts <= 5; ts++ {
			_, err := app1.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app1.Commit())

		// Switch to XOR2.
		h.opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))

		app2 := h.AppenderV2(context.Background())
		for ts := int64(6); ts <= 10; ts++ {
			_, err := app2.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app2.Commit())

		// The head chunk must still be XOR: encoding change does not cut chunks
		// when ST storage is disabled.
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms.headChunks)
		require.Nil(t, ms.headChunks.prev, "chunk must not have been cut; only one chunk must exist after encoding switch without ST storage")
		require.Equal(t, chunkenc.EncXOR, ms.headChunks.chunk.Encoding(),
			"in-progress chunk must keep XOR encoding after encoding switch")
	})
}

// TestWALReplayRaceWithStaleSeriesCompaction verifies that deleteSeriesByID correctly locks the
// hash shard (not only the ref shard) when deleting from the hashes map.
// The race only occurs when Prometheus restarts after having done a stale series compaction because
// deleteSeriesByID is not used otherwise.
func TestWALReplayRaceWithStaleSeriesCompaction(t *testing.T) {
	opts := newTestHeadDefaultOptions(1000, false)
	// A small stripe size ensures many series share hash shards, increasing
	// the likelihood that deleteSeriesByID and getOrCreateWithOptionalID
	// contend on the same shard during WAL replay.
	opts.StripeSize = 32
	head, _ := newTestHeadWithOptions(t, compression.None, opts)
	require.NoError(t, head.Init(0))

	appendSample := func(lbls labels.Labels, ts int64, val float64) {
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, lbls, 0, ts, val, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	// Step 1: Create a batch of series and make them stale.
	const numStaleSeries = 500
	staleLbls := make([]labels.Labels, numStaleSeries)
	for i := range numStaleSeries {
		staleLbls[i] = labels.FromStrings("__name__", "stale_metric", "i", strconv.Itoa(i))
		appendSample(staleLbls[i], 100, float64(i))
	}
	for _, lbl := range staleLbls {
		appendSample(lbl, 200, math.Float64frombits(value.StaleNaN))
	}
	require.Equal(t, uint64(numStaleSeries), head.NumStaleSeries())

	// Step 2: Truncate stale series. This removes them from the Head and
	// writes tombstone records (with Mint=MinInt64, Maxt=MaxInt64) to the WAL.
	staleRefs := make([]storage.SeriesRef, 0, numStaleSeries)
	for i := range numStaleSeries {
		ms := head.series.getByHash(staleLbls[i].Hash(), staleLbls[i])
		require.NotNil(t, ms)
		staleRefs = append(staleRefs, storage.SeriesRef(ms.ref))
	}
	require.NoError(t, head.truncateStaleSeries(staleRefs, 300, head.snapshotFingerprints(staleRefs, math.MaxUint64)))
	require.Equal(t, uint64(0), head.NumStaleSeries())
	require.Equal(t, uint64(0), head.NumSeries())

	// Step 3: Add new series AFTER the truncation. In the WAL, these series
	// records appear after the tombstone records. During replay, the main
	// goroutine will create these series (via getOrCreateWithOptionalID, which
	// accesses hashes[hashShard] under locks[hashShard]) concurrently with
	// the walSubsetProcessor goroutines deleting the stale series (via
	// deleteSeriesByID, which must also lock the correct hashShard).
	const numNewSeries = 500
	for i := range numNewSeries {
		lbl := labels.FromStrings("__name__", "new_metric", "i", strconv.Itoa(i))
		appendSample(lbl, 300, float64(i))
	}
	require.Equal(t, uint64(numNewSeries), head.NumSeries())

	// Step 4: Close and re-open the Head to trigger WAL replay.
	// With the buggy locking, the race detector should catch the data race
	// between the main goroutine (creating series) and worker goroutines
	// (deleting stale series) during replay.
	require.NoError(t, head.Close())

	wal, err := wlog.NewSize(nil, nil, filepath.Join(head.opts.ChunkDirRoot, "wal"), 32768, compression.None)
	require.NoError(t, err)
	head, err = NewHead(nil, nil, wal, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0)) // Should not cause a race here.

	require.Equal(t, uint64(0), head.NumStaleSeries())
	require.Equal(t, uint64(numNewSeries), head.NumSeries())
	require.NoError(t, head.Close())
}

// TestHead_ReadWAL_TombstonesAdvanceLastSeriesID verifies that replay advances
// lastSeriesID past tombstone refs, regardless of whether the stone's series
// record exists.
func TestHead_ReadWAL_TombstonesAdvanceLastSeriesID(t *testing.T) {
	cases := []struct {
		name  string
		stone tombstones.Stone
	}{
		{
			name:  "interval tombstone",
			stone: tombstones.Stone{Ref: 42, Intervals: []tombstones.Interval{{Mint: 0, Maxt: 100}}},
		},
		{
			name:  "full-range tombstone",
			stone: tombstones.Stone{Ref: 42, Intervals: []tombstones.Interval{{Mint: math.MinInt64, Maxt: math.MaxInt64}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := []any{
				[]record.RefSeries{
					{Ref: 10, Labels: labels.FromStrings("a", "1")},
				},
				[]record.RefSample{
					{Ref: 10, T: 100, V: 1},
				},
				[]tombstones.Stone{tc.stone},
			}

			head, w := newTestHead(t, 1000, compression.None, false)
			defer func() {
				require.NoError(t, head.Close())
			}()

			populateTestWL(t, w, entries, nil, false)

			require.NoError(t, head.Init(math.MinInt64))

			require.Equal(t, uint64(42), head.lastSeriesID.Load())
		})
	}
}

// TestStripeSeries_UnlinkHash verifies that unlinkHash removes a series from the hash
// index while leaving it resolvable by ref. Lookups by labels no longer find it, and a
// series with the same labels is created fresh instead of aliasing the unlinked one.
func TestStripeSeries_UnlinkHash(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	lbls := labels.FromStrings("a", "1")
	s1, created, err := head.getOrCreateWithOptionalID(1, lbls.Hash(), lbls, false)
	require.NoError(t, err)
	require.True(t, created)

	head.series.unlinkHash(lbls.Hash(), s1.ref)

	// Gone from the hash index, still resolvable by ref.
	require.Nil(t, head.series.getByHash(lbls.Hash(), lbls))
	require.Same(t, s1, head.series.getByID(s1.ref))

	// A series with the same labels isn't aliased onto s1.
	s2, created, err := head.getOrCreateWithOptionalID(2, lbls.Hash(), lbls, false)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, chunks.HeadSeriesRef(2), s2.ref)
}

// TestHead_WALReplay_FullRangeTombstoneDeletesDuplicateRef verifies that a full-range
// tombstone whose ref was mapped onto another series during replay deletes the series it was mapped to.
func TestHead_WALReplay_FullRangeTombstoneDeletesDuplicateRef(t *testing.T) {
	lbls := labels.FromStrings("a", "1")
	entries := []any{
		[]record.RefSeries{
			{Ref: 1, Labels: lbls},
			// Duplicate record for the same series under a different ref; replay
			// maps ref 2 onto ref 1.
			{Ref: 2, Labels: lbls},
		},
		[]record.RefSample{
			{Ref: 2, T: 100, V: 1},
		},
		[]tombstones.Stone{
			{Ref: 2, Intervals: tombstones.Intervals{{Mint: math.MinInt64, Maxt: math.MaxInt64}}},
		},
	}

	head, w := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, head.Close())
	}()
	populateTestWL(t, w, entries, nil, false)

	require.NoError(t, head.Init(math.MinInt64))

	require.Equal(t, uint64(0), head.NumSeries())
}

// TestHead_WALCheckpoint_FullRangeTombstones verifies checkpointing of a series deleted
// during replay by a full-range tombstone. Replay sets a WAL expiry at the series' max
// sample time, so checkpoints keep its series record and tombstone while the WAL still
// holds samples for it and drop both once those samples age out.
func TestHead_WALCheckpoint_FullRangeTombstones(t *testing.T) {
	lbls1 := labels.FromStrings("a", "1")
	lbls2 := labels.FromStrings("a", "2")
	fullRange := tombstones.Intervals{{Mint: math.MinInt64, Maxt: math.MaxInt64}}
	walEntries := []any{
		[]record.RefSeries{{Ref: 1, Labels: lbls1}, {Ref: 2, Labels: lbls2}},
		[]record.RefSample{
			{Ref: 1, T: 100, V: 1},
			{Ref: 1, T: 500, V: 2},
			{Ref: 2, T: 100, V: 3},
			{Ref: 2, T: 200, V: 4},
		},
		[]tombstones.Stone{
			{Ref: 1, Intervals: fullRange},
			{Ref: 2, Intervals: fullRange},
		},
	}

	head, w := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, head.Close())
	}()
	populateTestWL(t, w, walEntries, nil, false)
	first, _, err := wlog.Segments(w.Dir())
	require.NoError(t, err)

	require.NoError(t, head.Init(math.MinInt64))

	// Replay applied the tombstones: both series are deleted, and each ref has a WAL
	// expiry at its max sample time.
	require.Equal(t, uint64(0), head.NumSeries())
	keepUntil, ok := head.getWALExpiry(1)
	require.True(t, ok)
	require.Equal(t, int64(500), keepUntil)
	keepUntil, ok = head.getWALExpiry(2)
	require.True(t, ok)
	require.Equal(t, int64(200), keepUntil)

	// Checkpoint with mint=250, between the two expiries. Each truncation creates a
	// new segment, so attempt truncations until a checkpoint is created.
	for {
		head.lastWALTruncationTime.Store(0) // Reset so that it's always time to truncate the WAL.
		require.NoError(t, head.truncateWAL(250))
		f, _, err := wlog.Segments(w.Dir())
		require.NoError(t, err)
		if f > first {
			break
		}
	}

	// The WAL still holds a sample for ref 1 (t=500 >= mint), so its series record
	// must be kept for the sample to resolve on replay, and its tombstone with it so
	// the replayed series is deleted again. Ref 2's samples all age out, and its
	// series record and tombstone are dropped with them.
	expected := []any{
		[]record.RefSeries{{Ref: 1, Labels: lbls1}},
		[]record.RefSample{{Ref: 1, T: 500, V: 2}},
		[]tombstones.Stone{{Ref: 1, Intervals: fullRange}},
	}
	cpDir, _, err := wlog.LastCheckpoint(w.Dir())
	require.NoError(t, err)
	recs := append(readTestWAL(t, cpDir), readTestWAL(t, w.Dir())...)
	testutil.RequireEqual(t, expected, recs)

	// WAL expiries are cleaned up alongside the records they retain.
	keepUntil, ok = head.getWALExpiry(1)
	require.True(t, ok, "ref 1 expiry must be retained (500 >= mint 250)")
	require.Equal(t, int64(500), keepUntil)
	_, ok = head.getWALExpiry(2)
	require.False(t, ok, "ref 2 expiry must be cleared (200 < mint 250)")
}

func TestHead_FastStartupStateFile(t *testing.T) {
	opts := newTestHeadDefaultOptions(1000, false)
	// Enable the fast startup feature.
	opts.EnableFastStartup = true

	head, w := newTestHeadWithOptions(t, compression.None, opts)
	require.NoError(t, head.Init(0))

	// Add a single sample to the Head.
	app := head.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("fast", "startup"), 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	stateFilePath := filepath.Join(w.Dir(), "series_state.json")

	// Poll for the ticker (1s interval) to write the series state file rather
	// than waiting a fixed amount of time: faster in the common case, and robust
	// if the first tick is momentarily delayed under load.
	var state SeriesLifecycleState
	var b []byte
	require.Eventually(t, func() bool {
		var err error
		b, err = os.ReadFile(stateFilePath)
		if err != nil {
			return false
		}
		return json.Unmarshal(b, &state) == nil && state.LastSeriesID == 1
	}, 10*time.Second, 20*time.Millisecond, "series_state.json should be written by the ticker")

	// The ticker should write an unclean state.
	require.False(t, state.CleanShutdown, "ticker should write CleanShutdown: false")
	require.Equal(t, uint64(1), state.LastSeriesID, "LastSeriesID should be 1 after adding our sample")
	require.Equal(t, 0, state.LastWALSegment, "LastWALSegment should be 0 on a fresh WAL")

	// Perform a clean shutdown.
	require.NoError(t, head.Close())

	b, err = os.ReadFile(stateFilePath)
	require.NoError(t, err, "series_state.json should still exist after Close()")
	require.NoError(t, json.Unmarshal(b, &state))

	// Calling head.Close() should put us in the clean state.
	require.True(t, state.CleanShutdown, "Close() should write CleanShutdown: true")
	require.Equal(t, uint64(1), state.LastSeriesID, "LastSeriesID should remain 1")
	require.Equal(t, 0, state.LastWALSegment, "LastWALSegment should remain 0")
}

func TestHead_ReadSeriesStateFile(t *testing.T) {
	opts := newTestHeadDefaultOptions(1000, false)
	head, w := newTestHeadWithOptions(t, compression.None, opts)

	// Fresh boot case.
	// Should return 0 valued state and os.ErrNotExist.
	state, err := head.readSeriesStateFile()
	require.Error(t, err, "reading non-existent state file should return an error")
	require.True(t, os.IsNotExist(err), "error should be of type os.ErrNotExist")
	require.Equal(t, SeriesLifecycleState{}, state, "state should be zero-valued when file does not exist")

	// Valid file case.
	expectedState := SeriesLifecycleState{
		LastSeriesID:   42000,
		LastWALSegment: 5,
		CleanShutdown:  true,
	}

	b, err := json.Marshal(expectedState)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(w.Dir(), "series_state.json"), b, 0o666)
	require.NoError(t, err)

	state, err = head.readSeriesStateFile()
	require.NoError(t, err, "reading valid state file should not error")
	require.Equal(t, expectedState, state, "read state should match written state")
}

func TestHead_FindLastSeriesID(t *testing.T) {
	opts := newTestHeadDefaultOptions(1000, false)
	head, w := newTestHeadWithOptions(t, compression.None, opts)

	// Write Series A to first segment.
	app := head.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("metric", "A"), 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Force the WAL to cut a new segment file (second segment).
	_, err = w.NextSegment()
	require.NoError(t, err)

	// Write new sample for series A to second segment.
	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("metric", "A"), 0, 200, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Get the current max segment number.
	first, last, err := wlog.Segments(w.Dir())
	require.NoError(t, err)

	mockState := SeriesLifecycleState{
		LastSeriesID:   1,
		LastWALSegment: first,
		CleanShutdown:  false,
	}

	// Should return 1 as there is only 1 series created so far.
	id, err := head.findLastSeriesID(mockState, last)
	require.NoError(t, err)
	require.Equal(t, uint64(1), id, "Should find ID 1 as no new series were created in segment 2")

	// Write Series B to the second segment
	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("metric", "B"), 0, 300, 3.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Scanning both files should now return 2
	id, err = head.findLastSeriesID(mockState, last)
	require.NoError(t, err)
	require.Equal(t, uint64(2), id, "Should find ID 2 after new series was created in segment 2")

	// Simulate state file knowing about latest segment.
	mockState.LastWALSegment = last

	// Should return 2 as it should scan the last file and find series B.
	id, err = head.findLastSeriesID(mockState, last)
	require.NoError(t, err)
	require.Equal(t, uint64(2), id, "Should find ID 2 even when state file's last segment is the newest segment")
}

func TestHead_mmapHeadChunks(t *testing.T) {
	// Interval such that one chunk's worth of samples (DefaultSamplesPerChunk) spans DefaultBlockDuration/4.
	interval := DefaultBlockDuration / (4 * DefaultSamplesPerChunk)
	// Enough samples to cross the nextAt boundary multiple times, producing 3 head chunks per series.
	const chunkCutIterations = 2*DefaultSamplesPerChunk + 10

	t.Run("basic", func(t *testing.T) {
		h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
		require.NoError(t, h.Init(0))

		countReady := func() int {
			n := 0
			for i := range h.series.size {
				h.series.locks[i].RLock()
				for _, s := range h.series.series[i] {
					if s.headChunkCount.Load() >= 2 {
						n++
					}
				}
				h.series.locks[i].RUnlock()
			}
			return n
		}

		getCount := func(lbls labels.Labels) uint32 {
			s := h.series.getByHash(lbls.Hash(), lbls)
			require.NotNil(t, s, "series %s not found", lbls)
			return s.headChunkCount.Load()
		}

		lblsA := labels.FromStrings("__name__", "seriesA")
		lblsB := labels.FromStrings("__name__", "seriesB")
		lblsC := labels.FromStrings("__name__", "seriesC")

		ts := int64(0)

		// First chunk creation should set headChunkCount to 1.
		app := h.AppenderV2(t.Context())
		_, err := app.Append(0, lblsA, 0, ts, 1.0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		ts += interval

		require.Equal(t, uint32(1), getCount(lblsA), "first chunk should set headChunkCount to 1")
		require.Equal(t, 0, countReady(), "series with only a first chunk should not be ready")

		// Appending enough samples to trigger chunk cuts should update headChunkCount.
		var refB, refC storage.SeriesRef
		app = h.AppenderV2(t.Context())
		for range chunkCutIterations {
			var err error
			refB, err = app.Append(refB, lblsB, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			refC, err = app.Append(refC, lblsC, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())

		require.Equal(t, 2, countReady(), "expected both series to be marked ready")
		// With ~2*DefaultSamplesPerChunk samples, we expect 3 head chunks (the count tracks total head chunks).
		require.Equal(t, uint32(3), getCount(lblsB), "series B headChunkCount should reflect actual head chunk count")
		require.Equal(t, uint32(3), getCount(lblsC), "series C headChunkCount should reflect actual head chunk count")

		// mmapHeadChunks should reset headChunkCount to 1 (one head chunk remains).
		h.mmapHeadChunks()

		for _, lbls := range []labels.Labels{lblsB, lblsC} {
			s := h.series.getByHash(lbls.Hash(), lbls)
			require.NotNil(t, s, "series %s not found", lbls)
			s.Lock()
			require.NotNil(t, s.headChunks, "series %s should have head chunks", lbls)
			require.Nil(t, s.headChunks.prev, "series %s should not have prev mmapped", lbls)
			require.NotEmpty(t, s.mmappedChunks, "series %s should have mmapped chunks", lbls)
			s.Unlock()
		}

		require.Equal(t, uint32(1), getCount(lblsB), "headChunkCount should be 1 after mmap")
		require.Equal(t, uint32(1), getCount(lblsC), "headChunkCount should be 1 after mmap")
		require.Equal(t, 0, countReady(), "ready set should be empty after mmapHeadChunks")

		// A second call should be a no-op.
		beforeMetric := prom_testutil.ToFloat64(h.metrics.mmapChunksTotal)
		h.mmapHeadChunks()
		afterMetric := prom_testutil.ToFloat64(h.metrics.mmapChunksTotal)
		require.Equal(t, beforeMetric, afterMetric, "second call should mmap 0 chunks")

		// Only newly ready series should be processed.
		app = h.AppenderV2(t.Context())
		for range chunkCutIterations {
			var err error
			refB, err = app.Append(refB, lblsB, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())

		require.Equal(t, 1, countReady(), "only series B should be ready")
		require.Equal(t, uint32(3), getCount(lblsB), "series B headChunkCount should reflect new chunks")
		require.Equal(t, uint32(1), getCount(lblsC), "series C headChunkCount should be unchanged")

		beforeMetric = prom_testutil.ToFloat64(h.metrics.mmapChunksTotal)
		h.mmapHeadChunks()
		afterMetric = prom_testutil.ToFloat64(h.metrics.mmapChunksTotal)
		require.Greater(t, afterMetric, beforeMetric, "third call should mmap chunks from series B")
		require.Equal(t, uint32(1), getCount(lblsB), "series B headChunkCount should be 1 after mmap")
	})

	// Regression test for https://github.com/prometheus/prometheus/issues/17941:
	// if mmapChunks panics (via handleChunkWriteError) while mmapHeadChunks holds
	// the stripe and series locks, those locks must still be released. A leaked
	// lock previously self-deadlocked the subsequent Head.Close() -> mmapHeadChunks().
	t.Run("panic releases locks", func(t *testing.T) {
		h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("__name__", "series")
		ts := int64(0)
		var ref storage.SeriesRef
		app := h.AppenderV2(t.Context())
		for range chunkCutIterations {
			var err error
			ref, err = app.Append(ref, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())

		s := h.series.getByHash(lbls.Hash(), lbls)
		require.NotNil(t, s)
		require.GreaterOrEqual(t, s.headChunkCount.Load(), uint32(2), "need >=2 head chunks pending mmap")

		// Break the chunk write path in an OS agnostic way: dropping a segment
		// file with a far higher sequence number into the head chunks directory
		// makes the next cut file's sequence differ from the one the chunk
		// reference was handed out for, which fails the write.
		rogue := filepath.Join(mmappedChunksDir(h.opts.ChunkDirRoot), "000100")
		require.NoError(t, os.WriteFile(rogue, nil, 0o666))

		// newTestHead's cleanup calls Head.Close() -> mmapHeadChunks(), which would
		// otherwise re-trigger the panic on the now-missing directory. Closing the
		// mapper first makes WriteChunk return ErrChunkDiskMapperClosed, which
		// handleChunkWriteError does not panic on. Cleanups run LIFO, so this runs
		// before Head.Close().
		t.Cleanup(func() { _ = h.chunkDiskMapper.Close() })

		require.Panics(t, func() { h.mmapHeadChunks() })

		// The locks held while mmapChunks panicked must have been released.
		require.True(t, s.TryLock(), "series lock leaked after panic")
		s.Unlock()
		for i := range h.series.size {
			require.Truef(t, h.series.locks[i].TryLock(), "stripe lock %d leaked after panic", i)
			h.series.locks[i].Unlock()
		}
	})

	// Regression test for https://github.com/prometheus/prometheus/issues/19445:
	// mmapHeadChunksInStripe used to hold a stripe's RLock while locking each series
	// in it, while stripeSeries.gcSeries's check function, invoked under
	// iterForDeletion locks the series first and then (under some conditions) the
	// stripe's lock. This is a deadlock.
	//
	// Since a pure timing/scheduling based test is unlikely to fail, this test
	// mimics gcSeries locking behavior and drives the real mmapHeadChunksInStripe.
	// The test fails if it regresses to holding the stripe's RLock while locking a series.
	t.Run("mmapHeadChunksInStripe releases the stripe lock before locking a series", func(t *testing.T) {
		h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("__name__", "series")
		app := h.AppenderV2(t.Context())
		var (
			ref storage.SeriesRef
			ts  int64
		)
		for range DefaultSamplesPerChunk + 10 {
			var err error
			ref, err = app.Append(ref, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())

		series := h.series.getByID(chunks.HeadSeriesRef(ref))
		require.NotNil(t, series)
		require.GreaterOrEqual(t, series.headChunkCount.Load(), uint32(2), "series must be mmap-ready")
		stripe := h.series.refStripe(chunks.HeadSeriesRef(ref))

		// gcSeries: hold the series lock.
		series.Lock()
		mmapDone := make(chan struct{})
		go func() {
			defer close(mmapDone)
			var candidates []*memSeries
			h.mmapHeadChunksInStripe(stripe, &candidates)
		}()
		select {
		case <-mmapDone:
			t.Fatal("mmapHeadChunksInStripe returned without ever blocking on the series lock")
		case <-time.After(time.Second):
		}

		// If mmapHeadChunksInStripe still holds the stripe's RLock, this TryLock (write)
		// fails (it's blocked on the series lock while also holding the stripe lock).
		// This can only pass if the RLock has been released before ever locking the
		// series lock.
		gotLock := h.series.locks[stripe].TryLock()
		if gotLock {
			h.series.locks[stripe].Unlock()
		}

		// Unblock mmapHeadChunksInStripe and wait for it to finish before asserting, so
		// this test doesn't leak a goroutine regardless of the outcome.
		series.Unlock()
		select {
		case <-mmapDone:
		case <-time.After(5 * time.Second):
			t.Fatal("mmapHeadChunksInStripe never completed after the series lock was released")
		}

		require.True(t, gotLock, "mmapHeadChunksInStripe held the stripe's RLock while blocked on a series lock; "+
			"this deadlocks against a concurrent gcSeries")
	})

	t.Run("gcSeries preserves head chunks for active readers after WAL replay", func(t *testing.T) {
		if defaultIsolationDisabled {
			t.Skip("skipping test since tsdb isolation is disabled")
		}

		dir := t.TempDir()
		db, err := Open(dir, nil, nil, DefaultOptions(), nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			if db != nil {
				require.NoError(t, db.Close())
			}
		})
		db.DisableCompactions()

		app := db.AppenderV2(t.Context())
		ref, err := app.Append(0, labels.FromStrings("__name__", "series"), 0, 100, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		err = db.Close()
		db = nil
		require.NoError(t, err)

		db, err = Open(dir, nil, nil, DefaultOptions(), nil)
		require.NoError(t, err)
		db.DisableCompactions()
		h := db.Head()
		series := h.series.getByID(chunks.HeadSeriesRef(ref))
		require.NotNil(t, series)
		series.Lock()
		txIDCount, txIDsLen := series.txs.txIDCount, len(series.txs.txIDs)
		series.Unlock()
		require.Zero(t, txIDCount)
		require.Zero(t, txIDsLen, "WAL replay must leave the transaction ring empty")

		rh := NewRangeHead(h, 100, 100)
		ir, err := rh.Index()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, ir.Close()) })
		cr, err := rh.Chunks()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, cr.Close()) })
		builder := labels.NewScratchBuilder(0)
		var metas []chunks.Meta
		require.NoError(t, ir.Series(ref, &builder, &metas))
		require.Len(t, metas, 1)
		chk, _, err := cr.ChunkOrIterable(metas[0])
		require.NoError(t, err)
		require.NotNil(t, chk)

		// Retire the series directly to test the saved chunk independently of reader waiting.
		deleted := h.gcSeries([]storage.SeriesRef{ref}, 100, func(*memSeries) bool { return true })
		require.Contains(t, deleted, ref)
		require.Zero(t, h.NumSeries())
		require.Nil(t, h.series.getByID(chunks.HeadSeriesRef(ref)))

		var it chunkenc.Iterator
		require.NotPanics(t, func() { it = chk.Iterator(nil) })
		require.Equal(t, chunkenc.ValFloat, it.Next())
		ts, v := it.At()
		require.Equal(t, int64(100), ts)
		require.Equal(t, 1.0, v)
		require.Equal(t, chunkenc.ValNone, it.Next())
		require.NoError(t, it.Err())
	})

	// A stale mmap candidate must not write a retired series' retained chunks to disk.
	t.Run("gcSeries preserves head chunks and a stale mmap is a no-op", func(t *testing.T) {
		h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("__name__", "series")
		app := h.AppenderV2(t.Context())
		var (
			ref storage.SeriesRef
			ts  int64
		)
		for range DefaultSamplesPerChunk + 10 {
			var err error
			ref, err = app.Append(ref, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())

		series := h.series.getByID(chunks.HeadSeriesRef(ref))
		require.NotNil(t, series)
		headChunksBefore := series.headChunks
		headChunkCountBefore := series.headChunkCount.Load()
		mmappedChunksBefore := len(series.mmappedChunks)
		require.GreaterOrEqual(t, headChunkCountBefore, uint32(2), "series must be mmap-ready")
		stripe := h.series.refStripe(series.ref)
		require.Equal(t, int32(1), h.series.mmapReady[stripe].Load())

		deleted := h.gcSeries([]storage.SeriesRef{ref}, math.MaxInt64, func(*memSeries) bool { return true })
		require.Contains(t, deleted, ref)
		require.Nil(t, h.series.getByID(chunks.HeadSeriesRef(ref)))
		require.True(t, series.isGCed())
		require.Same(t, headChunksBefore, series.headChunks)
		require.Equal(t, headChunkCountBefore, series.headChunkCount.Load())
		require.Zero(t, h.series.mmapReady[stripe].Load())

		require.Equal(t, 0, h.mmapSeriesChunks(series), "mmapSeriesChunks must be a no-op on an evicted series")
		require.Same(t, headChunksBefore, series.headChunks)
		require.Equal(t, headChunkCountBefore, series.headChunkCount.Load())
		require.Len(t, series.mmappedChunks, mmappedChunksBefore)
		require.Zero(t, h.series.mmapReady[stripe].Load())
	})

	t.Run("ooo does not inflate count", func(t *testing.T) {
		h, _ := newTestHead(t, DefaultBlockDuration, compression.None, true /* oooEnabled */)
		require.NoError(t, h.Init(0))

		lbls := labels.FromStrings("__name__", "test")
		ts := int64(0)

		// Create enough in-order samples to get headChunkCount >= 2.
		var ref storage.SeriesRef
		app := h.AppenderV2(t.Context())
		for range chunkCutIterations {
			var err error
			ref, err = app.Append(ref, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())

		s := h.series.getByHash(lbls.Hash(), lbls)
		require.NotNil(t, s)
		countBefore := s.headChunkCount.Load()
		require.GreaterOrEqual(t, countBefore, uint32(2), "need multiple head chunks for this test")

		// Append an OOO sample that creates an OOO chunk.
		oooTs := ts - 5*time.Minute.Milliseconds() // Within the 10m OOO window.
		app = h.AppenderV2(t.Context())
		_, err := app.Append(0, lbls, 0, oooTs, 999.0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		// headChunkCount must not change from an OOO insert.
		require.Equal(t, countBefore, s.headChunkCount.Load(), "OOO insert should not inflate headChunkCount")
	})

	// Verify OOO chunk creation does not spuriously increment mmapReady when the
	// series happens to be at headChunkCount == 2. This simulates the natural
	// window where a series has just become mmap-ready and an OOO sample arrives.
	// Covers all three commit paths: floats, histograms, and float histograms.
	t.Run("ooo insert does not increment mmap ready", func(t *testing.T) {
		type testCase struct {
			appendSample func(app storage.AppenderV2, lbls labels.Labels, ts int64) error
		}
		testCases := map[string]testCase{
			"floats": {
				appendSample: func(app storage.AppenderV2, lbls labels.Labels, ts int64) error {
					_, err := app.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
					return err
				},
			},
			"histograms": {
				appendSample: func(app storage.AppenderV2, lbls labels.Labels, ts int64) error {
					_, err := app.Append(0, lbls, 0, ts, 0, tsdbutil.GenerateTestHistograms(1)[0], nil, storage.AOptions{})
					return err
				},
			},
			"float histograms": {
				appendSample: func(app storage.AppenderV2, lbls labels.Labels, ts int64) error {
					_, err := app.Append(0, lbls, 0, ts, 0, nil, tsdbutil.GenerateTestFloatHistograms(1)[0], storage.AOptions{})
					return err
				},
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				h, _ := newTestHead(t, DefaultBlockDuration, compression.None, true /* oooEnabled */)
				require.NoError(t, h.Init(0))

				mmapReadyTotal := func() int32 {
					var n int32
					for i := range h.series.size {
						n += h.series.mmapReady[i].Load()
					}
					return n
				}

				// Create a series with one in-order sample.
				lbls := labels.FromStrings("__name__", "test")
				ts := int64(0)
				app := h.AppenderV2(t.Context())
				require.NoError(t, tc.appendSample(app, lbls, ts))
				require.NoError(t, app.Commit())
				ts += interval

				s := h.series.getByHash(lbls.Hash(), lbls)
				require.NotNil(t, s)

				// Force the exact state required to trigger the bug: series at
				// headChunkCount == 2 (mmap-ready) with mmapReady already incremented.
				s.Lock()
				s.headChunkCount.Store(2)
				s.Unlock()
				h.series.incMmapReady(s.ref)
				require.Equal(t, int32(1), mmapReadyTotal())

				// Append an OOO sample. This creates an OOO chunk (not a regular
				// head chunk), so mmapReady must stay at 1.
				oooTs := ts - 5*time.Minute.Milliseconds()
				app = h.AppenderV2(t.Context())
				require.NoError(t, tc.appendSample(app, lbls, oooTs))
				require.NoError(t, app.Commit())

				require.Equal(t, int32(1), mmapReadyTotal(),
					"OOO chunk creation must not increment mmapReady")
			})
		}
	})

	// Verify the WAL-replay path's eager mmap (appendChunkAndMmap) keeps
	// mmapReady consistent when a series enters the path with
	// headChunkCount >= 2 (typical after loadChunkSnapshot restores a
	// series that had multiple head chunks at shutdown).
	t.Run("wal replay does not leak mmap ready", func(t *testing.T) {
		// Pre-generate varied histograms; reusing the same histogram every
		// iteration compresses too well to trigger chunk cuts.
		histograms := tsdbutil.GenerateTestHistograms(chunkCutIterations)
		floatHistograms := tsdbutil.GenerateTestFloatHistograms(chunkCutIterations)

		type testCase struct {
			appendOnce     func(app storage.AppenderV2, lbls labels.Labels, ts int64, i int) error
			appendInternal func(ms *memSeries, t int64, opts chunkOpts) (bool, bool)
		}
		testCases := map[string]testCase{
			"floats": {
				appendOnce: func(app storage.AppenderV2, lbls labels.Labels, ts int64, _ int) error {
					_, err := app.Append(0, lbls, 0, ts, float64(ts), nil, nil, storage.AOptions{})
					return err
				},
				appendInternal: func(ms *memSeries, t int64, opts chunkOpts) (bool, bool) {
					return ms.append(0, t, 0, 0, opts)
				},
			},
			"histograms": {
				appendOnce: func(app storage.AppenderV2, lbls labels.Labels, ts int64, i int) error {
					_, err := app.Append(0, lbls, 0, ts, 0, histograms[i], nil, storage.AOptions{})
					return err
				},
				appendInternal: func(ms *memSeries, t int64, opts chunkOpts) (bool, bool) {
					return ms.appendHistogram(0, t, histograms[0], 0, opts)
				},
			},
			"float histograms": {
				appendOnce: func(app storage.AppenderV2, lbls labels.Labels, ts int64, i int) error {
					_, err := app.Append(0, lbls, 0, ts, 0, nil, floatHistograms[i], storage.AOptions{})
					return err
				},
				appendInternal: func(ms *memSeries, t int64, opts chunkOpts) (bool, bool) {
					return ms.appendFloatHistogram(0, t, floatHistograms[0], 0, opts)
				},
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
				require.NoError(t, h.Init(0))

				mmapReadyTotal := func() int32 {
					var n int32
					for i := range h.series.size {
						n += h.series.mmapReady[i].Load()
					}
					return n
				}

				// Build a series naturally to headChunkCount >= 2 via the normal
				// appender path. This sets mmapReady to 1.
				lbls := labels.FromStrings("__name__", "test")
				ts := int64(0)
				app := h.AppenderV2(t.Context())
				for i := range chunkCutIterations {
					require.NoError(t, tc.appendOnce(app, lbls, ts, i))
					ts += interval
				}
				require.NoError(t, app.Commit())

				s := h.series.getByHash(lbls.Hash(), lbls)
				require.NotNil(t, s)
				require.GreaterOrEqual(t, s.headChunkCount.Load(), uint32(2))
				require.Equal(t, int32(1), mmapReadyTotal())

				// Invoke the WAL-replay-style helper with a sample timestamped
				// past the next chunk boundary to force a chunk cut.
				s.Lock()
				_, chunkCreated := h.appendChunkAndMmap(s, func() (bool, bool) {
					return tc.appendInternal(s, ts+h.chunkRange.Load(), chunkOpts{
						chunkDiskMapper: h.chunkDiskMapper,
						chunkRange:      h.chunkRange.Load(),
						samplesPerChunk: h.opts.SamplesPerChunk,
					})
				})
				s.Unlock()
				require.True(t, chunkCreated, "test needs a chunk cut to be meaningful")

				// After the chunk cut + mmap, headChunkCount == 1 (mmapChunks
				// always sets it to 1 when it does work). The series is no
				// longer mmap-ready, so mmapReady must be 0.
				require.Equal(t, int32(0), mmapReadyTotal(),
					"WAL-replay-style chunk cut must decrement mmapReady when prev >= 2")
			})
		}
	})

	// Verify the mmapReady per-stripe counter stays in sync with the actual
	// number of series that have headChunkCount >= 2, across append, mmap,
	// GC, and stale-series deletion.
	t.Run("counter consistency", func(t *testing.T) {
		h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
		require.NoError(t, h.Init(0))

		// mmapReadyCounter returns the sum of mmapReady[i] across all stripes.
		mmapReadyCounter := func() int32 {
			var n int32
			for i := range h.series.size {
				n += h.series.mmapReady[i].Load()
			}
			return n
		}
		requireCounterConsistent := func(msg string) {
			t.Helper()

			var actual int32
			for i := range h.series.size {
				h.series.locks[i].RLock()
				for _, s := range h.series.series[i] {
					if s.headChunkCount.Load() >= 2 {
						actual++
					}
				}
				h.series.locks[i].RUnlock()
			}
			counter := mmapReadyCounter()
			require.Equal(t, actual, counter, "%s: mmapReady counter (%d) != actual ready count (%d)", msg, counter, actual)
		}

		lblsA := labels.FromStrings("__name__", "seriesA")
		lblsB := labels.FromStrings("__name__", "seriesB")
		lblsC := labels.FromStrings("__name__", "seriesC")
		lblsHist := labels.FromStrings("__name__", "seriesHist")
		lblsFHist := labels.FromStrings("__name__", "seriesFHist")

		// Phase 1: Append enough samples to create multiple head chunks.
		// Include histogram and float-histogram series to cover commitHistograms
		// and commitFloatHistograms increment paths.
		ts := int64(0)
		var refA, refB, refC, refHist, refFHist storage.SeriesRef
		histograms := tsdbutil.GenerateTestHistograms(chunkCutIterations)
		floatHistograms := tsdbutil.GenerateTestFloatHistograms(chunkCutIterations)
		app := h.AppenderV2(t.Context())
		for i := range chunkCutIterations {
			var err error
			refA, err = app.Append(refA, lblsA, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			refB, err = app.Append(refB, lblsB, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			refC, err = app.Append(refC, lblsC, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			refHist, err = app.Append(refHist, lblsHist, 0, ts, 0, histograms[i], nil, storage.AOptions{})
			require.NoError(t, err)
			refFHist, err = app.Append(refFHist, lblsFHist, 0, ts, 0, nil, floatHistograms[i], storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())
		requireCounterConsistent("after initial appends with chunk cuts")
		require.Equal(t, int32(5), mmapReadyCounter(), "all five series should be ready")

		// Phase 2: mmapHeadChunks should bring counter to 0.
		h.mmapHeadChunks()
		requireCounterConsistent("after mmapHeadChunks")
		require.Equal(t, int32(0), mmapReadyCounter(), "counter should be 0 after mmap")

		// Phase 3: Make only seriesA ready again, then GC with a mint that
		// truncates its head chunks back to 1.
		app = h.AppenderV2(t.Context())
		for range chunkCutIterations {
			var err error
			refA, err = app.Append(refA, lblsA, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())
		requireCounterConsistent("after making seriesA ready again")
		require.Equal(t, int32(1), mmapReadyCounter(), "only seriesA should be ready")

		// Mmap to bring seriesA back to headChunkCount == 1, then append
		// again so the next GC can test the counter decrement path.
		h.mmapHeadChunks()
		requireCounterConsistent("after second mmap")
		require.Equal(t, int32(0), mmapReadyCounter())

		// Make seriesA ready a third time, then GC at a time that truncates
		// the mmapped chunks but not the new head chunks.
		app = h.AppenderV2(t.Context())
		for range chunkCutIterations {
			var err error
			refA, err = app.Append(refA, lblsA, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())
		requireCounterConsistent("after third round of appends")
		require.Equal(t, int32(1), mmapReadyCounter())

		// Set minTime to current ts so GC truncates old chunks. Since seriesA
		// still has recent head chunks (headChunkCount >= 2), GC's
		// truncateChunksBefore may or may not reduce headChunkCount depending
		// on timestamps. Regardless, the counter must stay consistent.
		h.minTime.Store(ts)
		h.gc()
		requireCounterConsistent("after GC truncation")

		// Phase 4: Stale-series deletion via truncateStaleSeries (gcStaleSeries path).
		// Build seriesB up to headChunkCount >= 2, then mark stale and truncate.
		app = h.AppenderV2(t.Context())
		for range chunkCutIterations {
			var err error
			refB, err = app.Append(refB, lblsB, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ts += interval
		}
		require.NoError(t, app.Commit())

		// Mark seriesB stale so truncateStaleSeries will remove it.
		app = h.AppenderV2(t.Context())
		_, err := app.Append(refB, lblsB, 0, ts, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		ts += interval

		sB := h.series.getByHash(lblsB.Hash(), lblsB)
		require.NotNil(t, sB)
		require.GreaterOrEqual(t, sB.headChunkCount.Load(), uint32(2),
			"seriesB must be mmap-ready at deletion time for this test to be meaningful")
		requireCounterConsistent("before stale series deletion")
		readyBefore := mmapReadyCounter()

		// Use truncateStaleSeries which calls gcStaleSeries internally.
		staleRefs := []storage.SeriesRef{storage.SeriesRef(sB.ref)}
		require.NoError(t, h.truncateStaleSeries(
			staleRefs, ts, h.snapshotFingerprints(staleRefs, math.MaxUint64),
		))
		requireCounterConsistent("after truncateStaleSeries")
		require.Less(t, mmapReadyCounter(), readyBefore,
			"counter should have decreased after deleting a ready series")

		// Phase 5: Verify mmapHeadChunks does not decrement when mmapChunks
		// returns 0. This simulates the race where a series passes the
		// headChunkCount >= 2 filter but by the time the series lock is
		// acquired, another goroutine (GC) has already reduced the count.
		//
		// We create a fresh series with 1 head chunk, then artificially set
		// headChunkCount to 2 and mmapReady to 1. mmapChunks will see
		// headChunks.prev == nil and return 0. Without the n > 0 guard,
		// the counter would be decremented to 0 even though the "real"
		// decrement never happened (simulating a double-decrement).
		lblsD := labels.FromStrings("__name__", "seriesD")
		app = h.AppenderV2(t.Context())
		_, err = app.Append(0, lblsD, 0, ts, float64(ts), nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		sD := h.series.getByHash(lblsD.Hash(), lblsD)
		require.NotNil(t, sD)
		require.Equal(t, uint32(1), sD.headChunkCount.Load())
		require.Nil(t, sD.headChunks.prev, "only one head chunk, prev must be nil")
		stripeD := h.series.refStripe(sD.ref)

		// Simulate the race: inflate headChunkCount so the series passes the
		// >= 2 filter, and set mmapReady to 1 (as the increment site would).
		sD.headChunkCount.Store(2)
		h.series.mmapReady[stripeD].Add(1)

		h.mmapHeadChunks()

		// mmapChunks returned 0 (headChunks.prev is nil). With the n > 0
		// guard, the counter stays at 1. Without the guard, it would drop
		// to 0 — an incorrect extra decrement.
		require.Equal(t, int32(1), h.series.mmapReady[stripeD].Load(),
			"mmapHeadChunks must not decrement when mmapChunks returns 0")

		// Clean up: restore the real state.
		sD.headChunkCount.Store(1)
		h.series.mmapReady[stripeD].Add(-1)
		requireCounterConsistent("final state")
	})
}

func TestQueryOOOHeadDuringTruncateAcrossWrap(t *testing.T) {
	const maxT int64 = 6000

	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = maxT
	opts.MinBlockDuration = maxT / 2

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	l := labels.FromStrings("a", "b")
	app := db.AppenderV2(context.Background())

	// In-order samples.
	ref, err := app.Append(0, l, 0, 0, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	for i := int64(100); i < maxT; i += 100 {
		_, err = app.Append(ref, l, 0, i, float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	// OOO samples interleaved between in-order ones.
	for i := int64(50); i < maxT; i += 100 {
		_, err = app.Append(ref, l, 0, i, float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Seed firstOOOChunkID near the wrap boundary.
	ms := db.head.series.getByID(chunks.HeadSeriesRef(ref))
	require.NotNil(t, ms)
	ms.Lock()
	require.NotNil(t, ms.ooo)
	ms.ooo.firstOOOChunkID = chunks.HeadChunkID(oooChunkIDMask - 1)
	ms.Unlock()

	// Pre-truncation query.
	q, err := db.Querier(0, maxT)
	require.NoError(t, err)
	ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, ss.Next())
	it := ss.At().Iterator(nil)
	var preCount int
	for it.Next() != chunkenc.ValNone {
		preCount++
	}
	require.NoError(t, it.Err())
	require.Positive(t, preCount, "pre-truncation query should return data")
	require.NoError(t, q.Close())

	// Compact, which triggers truncation and wraps firstOOOChunkID.
	require.NoError(t, db.Compact(context.Background()))

	// Post-truncation query.
	q, err = db.Querier(0, maxT)
	require.NoError(t, err)
	ss = q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, ss.Next())
	it = ss.At().Iterator(nil)
	var postCount int
	for it.Next() != chunkenc.ValNone {
		postCount++
	}
	require.NoError(t, it.Err())
	require.Positive(t, postCount, "post-truncation query should return data")
	require.NoError(t, q.Close())
}

func TestOOORestartResetsFirstOOOChunkID(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOORestartResetsFirstOOOChunkID(t, scenario)
		})
	}
}

func testOOORestartResetsFirstOOOChunkID(t *testing.T, scenario sampleTypeScenario) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
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
		app := h.AppenderV2(context.Background())
		_, _, err := scenario.appendFunc(app, l, mins*time.Minute.Milliseconds(), mins)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	// In-order sample.
	appendSample(200)

	// OOO samples: 92 samples to create 3 mmapped chunks (cap=30).
	for mins := int64(100); mins <= 191; mins++ {
		appendSample(mins)
	}

	ms, ok, err := h.getOrCreate(l.Hash(), l, false)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)
	require.NotNil(t, ms.ooo)
	require.NotEmpty(t, ms.ooo.oooMmappedChunks)

	expMmapCount := len(ms.ooo.oooMmappedChunks)

	// Seed firstOOOChunkID to a large value.
	ms.Lock()
	ms.ooo.firstOOOChunkID = chunks.HeadChunkID(oooChunkIDMask - 100)
	ms.Unlock()

	require.NoError(t, h.Close())

	// Reopen.
	wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err = wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	ms, ok, err = h.getOrCreate(l.Hash(), l, false)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)
	require.NotNil(t, ms.ooo)

	// firstOOOChunkID resets to 0 after replay.
	require.Equal(t, chunks.HeadChunkID(0), ms.ooo.firstOOOChunkID)

	// Verify mmapped chunks are still accessible.
	require.Len(t, ms.ooo.oooMmappedChunks, expMmapCount)
	for _, m := range ms.ooo.oooMmappedChunks {
		chk, err := h.chunkDiskMapper.Chunk(m.ref)
		require.NoError(t, err)
		require.Equal(t, int(m.numSamples), chk.NumSamples())
	}

	require.NoError(t, h.Close())
}

func TestHeadAppender_DuplicateSamplesDroppedMetric(t *testing.T) {
	newStatsHead := func(t *testing.T, oooEnabled bool) *Head {
		opts := newTestHeadDefaultOptions(DefaultBlockDuration, oooEnabled)
		opts.EnableCommitStats = true
		head, _ := newTestHeadWithOptions(t, compression.None, opts)
		return head
	}
	for _, useV2 := range []bool{false, true} {
		t.Run(fmt.Sprintf("v2=%t", useV2), func(t *testing.T) {
			ctx := t.Context()
			type reportingAppender struct {
				storage.LimitedAppenderV1
				CommitStatsReporter
			}
			newAppender := func(src interface {
				storage.Appendable
				storage.AppendableV2
			},
			) reportingAppender {
				if useV2 {
					app := src.AppenderV2(ctx)
					reporter, ok := app.(CommitStatsReporter)
					require.True(t, ok, "V2 appender must report commit stats")
					return reportingAppender{storage.AppenderV2AsLimitedV1(app), reporter}
				}
				app := src.Appender(ctx)
				reporter, ok := app.(CommitStatsReporter)
				require.True(t, ok, "V1 appender must report commit stats")
				return reportingAppender{app, reporter}
			}
			lbls := labels.FromStrings("foo", "bar")

			requireStats := func(t *testing.T, app CommitStatsReporter, wantSameValue, wantDifferentValue int) {
				t.Helper()
				stats := app.CommitStats().DiscardedSamples
				require.Equal(t, wantSameValue, stats.TotalSameValue(), "same-value dropped samples")
				require.Equal(t, wantDifferentValue, stats.TotalDifferentValue(), "different-value dropped samples")
			}

			for name, scenario := range sampleTypeScenarios {
				t.Run(name, func(t *testing.T) {
					head := newStatsHead(t, true)

					dupsDropped := func() float64 {
						return prom_testutil.ToFloat64(head.metrics.duplicateSamples.WithLabelValues(scenario.sampleType))
					}

					app := newAppender(head)
					_, _, err := scenario.appendFunc(app, lbls, 60_000, 1)
					require.NoError(t, err)
					_, _, err = scenario.appendFunc(app, lbls, 120_000, 2)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					requireStats(t, app, 0, 0)

					// Exact duplicate of the latest in-order sample: dropped at commit.
					app = newAppender(head)
					_, _, err = scenario.appendFunc(app, lbls, 120_000, 2)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					require.Equal(t, 1.0, dupsDropped())
					requireStats(t, app, 1, 0)

					// Different value against a committed sample: Append error. Counted in
					// the metric (all duplicate discards) but NOT in CommitStats, which only
					// reclassifies samples that Append accepted.
					app = newAppender(head)
					_, _, err = scenario.appendFunc(app, lbls, 120_000, 3)
					require.ErrorIs(t, err, storage.ErrDuplicateSampleForTimestamp)
					require.NoError(t, app.Commit())
					require.Equal(t, 2.0, dupsDropped())
					requireStats(t, app, 0, 0)
					// Must not be counted as an out-of-order rejection.
					require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))

					// Within-batch conflict: passes Append, dropped at commit, first sample wins.
					app = newAppender(head)
					_, _, err = scenario.appendFunc(app, lbls, 180_000, 6)
					require.NoError(t, err)
					_, _, err = scenario.appendFunc(app, lbls, 180_000, 7)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					require.Equal(t, 3.0, dupsDropped())
					requireStats(t, app, 0, 1)

					// Within-batch exact duplicate.
					app = newAppender(head)
					_, _, err = scenario.appendFunc(app, lbls, 240_000, 8)
					require.NoError(t, err)
					_, _, err = scenario.appendFunc(app, lbls, 240_000, 8)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					require.Equal(t, 4.0, dupsDropped())
					requireStats(t, app, 1, 0)

					// First out-of-order sample is accepted.
					app = newAppender(head)
					_, _, err = scenario.appendFunc(app, lbls, 30_000, 4)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					require.Equal(t, 4.0, dupsDropped())
					requireStats(t, app, 0, 0)

					// Exact duplicate of the out-of-order sample.
					app = newAppender(head)
					_, _, err = scenario.appendFunc(app, lbls, 30_000, 4)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					require.Equal(t, 5.0, dupsDropped())
					requireStats(t, app, 1, 0)

					// OOO timestamp clash is dropped even when the value differs.
					app = newAppender(head)
					_, _, err = scenario.appendFunc(app, lbls, 30_000, 5)
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					require.Equal(t, 6.0, dupsDropped())
					requireStats(t, app, 0, 1)

					// Dropped duplicates must not inflate the appended count (5 stored samples).
					require.Equal(t, 5.0, prom_testutil.ToFloat64(head.metrics.samplesAppended.WithLabelValues(scenario.sampleType)))
				})
			}

			t.Run("stats aggregate per series with counts", func(t *testing.T) {
				head := newStatsHead(t, false)
				lblsA := labels.FromStrings("foo", "bar")
				lblsB := labels.FromStrings("foo", "baz")

				app := newAppender(head)
				_, err := app.Append(0, lblsA, 100, 1)
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				app = newAppender(head)
				for range 3 {
					_, err = app.Append(0, lblsA, 100, 1)
					require.NoError(t, err)
				}
				_, err = app.Append(0, lblsB, 100, 10)
				require.NoError(t, err)
				_, err = app.Append(0, lblsB, 100, 11)
				require.NoError(t, err)
				_, err = app.Append(0, lblsB, 100, 12)
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				stats := app.CommitStats().DiscardedSamples
				require.Equal(t, []DiscardedSeriesSamples{{Labels: lblsA, Count: 3}}, stats.SameTimestampSameValue)
				require.Equal(t, []DiscardedSeriesSamples{{Labels: lblsB, Count: 2}}, stats.SameTimestampDifferentValue)
			})

			t.Run("rollback reports no stats", func(t *testing.T) {
				head := newStatsHead(t, false)

				app := newAppender(head)
				_, err := app.Append(0, lbls, 100, 1)
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				app = newAppender(head)
				_, err = app.Append(0, lbls, 100, 1)
				require.NoError(t, err)
				require.NoError(t, app.Rollback())
				requireStats(t, app, 0, 0)
			})

			t.Run("stats reachable through the DB appender chain on first push", func(t *testing.T) {
				opts := DefaultOptions()
				opts.EnableCommitStats = true
				db := newTestDB(t, withOpts(opts))

				// First push routes through the DB and init appender wrappers.
				app := newAppender(db)
				_, err := app.Append(0, lbls, 100, 1)
				require.NoError(t, err)
				_, err = app.Append(0, lbls, 100, 1)
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				requireStats(t, app, 1, 0)
			})

			t.Run("stats disabled by default, metric still counts", func(t *testing.T) {
				head, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)

				app := newAppender(head)
				_, err := app.Append(0, lbls, 100, 1)
				require.NoError(t, err)
				_, err = app.Append(0, lbls, 100, 1)
				require.NoError(t, err)
				_, err = app.Append(0, lbls, 200, 1)
				require.NoError(t, err)
				_, err = app.Append(0, lbls, 200, 2)
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				requireStats(t, app, 0, 0)
				require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.duplicateSamples.WithLabelValues(sampleMetricTypeFloat)))
				require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.samplesAppended.WithLabelValues(sampleMetricTypeFloat)))
			})
		})
	}
}
