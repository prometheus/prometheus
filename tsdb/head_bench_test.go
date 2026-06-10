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
	"errors"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/util/compression"
)

type benchAppendFunc func(b *testing.B, h *Head, ts int64, series []storage.Series, samplesPerAppend int64) storage.AppenderTransaction

func appendV1Float(b *testing.B, h *Head, ts int64, series []storage.Series, samplesPerAppend int64) storage.AppenderTransaction {
	var err error
	app := h.Appender(b.Context())
	for _, s := range series {
		var ref storage.SeriesRef
		for sampleIndex := range samplesPerAppend {
			ref, err = app.Append(ref, s.Labels(), ts+sampleIndex, float64(ts+sampleIndex))
			require.NoError(b, err)
		}
	}
	return app
}

func appendV2Float(b *testing.B, h *Head, ts int64, series []storage.Series, samplesPerAppend int64) storage.AppenderTransaction {
	var err error
	app := h.AppenderV2(b.Context())
	for _, s := range series {
		var ref storage.SeriesRef
		for sampleIndex := range samplesPerAppend {
			ref, err = app.Append(ref, s.Labels(), 0, ts+sampleIndex, float64(ts+sampleIndex), nil, nil, storage.AOptions{})
			require.NoError(b, err)
		}
	}
	return app
}

func appendV1FloatOrHistogramWithExemplars(b *testing.B, h *Head, ts int64, series []storage.Series, samplesPerAppend int64) storage.AppenderTransaction {
	var err error
	app := h.Appender(b.Context())
	for i, s := range series {
		var ref storage.SeriesRef
		for sampleIndex := range samplesPerAppend {
			// if i is even, append a sample, else append a histogram.
			if i%2 == 0 {
				ref, err = app.Append(ref, s.Labels(), ts+sampleIndex, float64(ts+sampleIndex))
				require.NoError(b, err)
				// Every sample also has an exemplar attached.
				_, err = app.AppendExemplar(ref, s.Labels(), exemplar.Exemplar{
					Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
					Value:  rand.Float64(),
					Ts:     ts + sampleIndex,
				})
				require.NoError(b, err)
				continue
			}

			h := &histogram.Histogram{
				Count:         7 + uint64(ts*5),
				ZeroCount:     2 + uint64(ts),
				ZeroThreshold: 0.001,
				Sum:           18.4 * rand.Float64(),
				Schema:        1,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{ts + 1, 1, -1, 0},
			}
			ref, err = app.AppendHistogram(ref, s.Labels(), ts, h, nil)
			require.NoError(b, err)
			// Every histogram sample also has 3 exemplars attached.
			_, err = app.AppendExemplar(ref, s.Labels(), exemplar.Exemplar{
				Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
				Value:  rand.Float64(),
				Ts:     ts + sampleIndex,
			})
			require.NoError(b, err)
			_, err = app.AppendExemplar(ref, s.Labels(), exemplar.Exemplar{
				Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
				Value:  rand.Float64(),
				Ts:     ts + sampleIndex,
			})
			require.NoError(b, err)
			_, err = app.AppendExemplar(ref, s.Labels(), exemplar.Exemplar{
				Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
				Value:  rand.Float64(),
				Ts:     ts + sampleIndex,
			})
			require.NoError(b, err)
		}
	}
	return app
}

func appendV2FloatOrHistogramWithExemplars(b *testing.B, h *Head, ts int64, series []storage.Series, samplesPerAppend int64) storage.AppenderTransaction {
	var (
		err error
		ex  = make([]exemplar.Exemplar, 3)
	)

	app := h.AppenderV2(b.Context())
	for i, s := range series {
		var ref storage.SeriesRef
		for sampleIndex := range samplesPerAppend {
			aOpts := storage.AOptions{Exemplars: ex[:0]}

			// if i is even, append a sample, else append a histogram.
			if i%2 == 0 {
				// Every sample also has an exemplar attached.
				aOpts.Exemplars = append(aOpts.Exemplars, exemplar.Exemplar{
					Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
					Value:  rand.Float64(),
					Ts:     ts + sampleIndex,
				})
				ref, err = app.Append(ref, s.Labels(), 0, ts, float64(ts), nil, nil, aOpts)
				require.NoError(b, err)
				continue
			}
			h := &histogram.Histogram{
				Count:         7 + uint64(ts*5),
				ZeroCount:     2 + uint64(ts),
				ZeroThreshold: 0.001,
				Sum:           18.4 * rand.Float64(),
				Schema:        1,
				PositiveSpans: []histogram.Span{
					{Offset: 0, Length: 2},
					{Offset: 1, Length: 2},
				},
				PositiveBuckets: []int64{ts + 1, 1, -1, 0},
			}

			// Every histogram sample also has 3 exemplars attached.
			aOpts.Exemplars = append(aOpts.Exemplars,
				exemplar.Exemplar{
					Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
					Value:  rand.Float64(),
					Ts:     ts + sampleIndex,
				},
				exemplar.Exemplar{
					Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
					Value:  rand.Float64(),
					Ts:     ts + sampleIndex,
				},
				exemplar.Exemplar{
					Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
					Value:  rand.Float64(),
					Ts:     ts + sampleIndex,
				},
			)
			ref, err = app.Append(ref, s.Labels(), 0, ts, 0, h, nil, aOpts)
			require.NoError(b, err)
		}
	}
	return app
}

type appendCase struct {
	name       string
	appendFunc benchAppendFunc
}

func appendCases() []appendCase {
	return []appendCase{
		{
			name:       "appender=v1/case=floats",
			appendFunc: appendV1Float,
		},
		{
			name:       "appender=v2/case=floats",
			appendFunc: appendV2Float,
		},
		{
			name:       "appender=v1/case=floatsHistogramsExemplars",
			appendFunc: appendV1FloatOrHistogramWithExemplars,
		},
		{
			name:       "appender=v2/case=floatsHistogramsExemplars",
			appendFunc: appendV2FloatOrHistogramWithExemplars,
		},
	}
}

/*
	export bench=append && go test \
	  -run '^$' -bench '^BenchmarkHeadAppender_AppendCommit$' \
	  -benchtime 5s -count 6 -cpu 2 -timeout 999m \
	  | tee ${bench}.txt
*/
func BenchmarkHeadAppender_AppendCommit(b *testing.B) {
	// NOTE(bwplotka): Previously we also had 1k and 10k series case. There is nothing
	// special happening in 100 vs 1k vs 10k, so let's save considerable amount of benchmark time
	// for quicker feedback. In return, we add more sample type cases.
	// Similarly, we removed the 2 sample in append case.
	//
	// TODO(bwplotka): This still takes ~6500s (~2h) for -benchtime 5s -count 6 to complete.
	// We might want to reduce the time bit more. 5s is really important as the slowest
	// case (appender=v1/case=floatsHistogramsExemplars/series=100/samples_per_append=100-2)
	// in 5s yields only 255 iters 23184892 ns/op. Perhaps -benchtime=300x would be better?
	seriesCounts := []int{10, 100}
	series := genSeries(100, 10, 0, 0) // Only using the generated labels.
	for _, appendCase := range appendCases() {
		for _, seriesCount := range seriesCounts {
			for _, samplesPerAppend := range []int64{1, 5, 100} {
				b.Run(fmt.Sprintf("%s/series=%d/samples_per_append=%d", appendCase.name, seriesCount, samplesPerAppend), func(b *testing.B) {
					opts := newTestHeadDefaultOptions(10000, false)
					opts.EnableExemplarStorage = true // We benchmark with exemplars, benchmark with them.
					h, _ := newTestHeadWithOptions(b, compression.None, opts)

					ts := int64(1000)

					// Init series, that's not what we're benchmarking here.
					app := appendCase.appendFunc(b, h, ts, series[:seriesCount], samplesPerAppend)
					require.NoError(b, app.Commit())
					ts += 1000 // should increment more than highest samplesPerAppend

					b.ReportAllocs()
					b.ResetTimer()

					for b.Loop() {
						app := appendCase.appendFunc(b, h, ts, series[:seriesCount], samplesPerAppend)
						require.NoError(b, app.Commit())
						ts += 1000 // should increment more than highest samplesPerAppend
					}
				})
			}
		}
	}
}

func BenchmarkHeadStripeSeriesCreate(b *testing.B) {
	newHead := func(b *testing.B, enableSharding bool) *Head {
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1000
		opts.ChunkDirRoot = b.TempDir()
		opts.EnableSharding = enableSharding
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(b, err)
		b.Cleanup(func() { require.NoError(b, h.Close()) })
		return h
	}

	// With sharding enabled, series creation additionally maintains shard hashes.
	for _, sharding := range []bool{false, true} {
		b.Run(fmt.Sprintf("sharding=%t", sharding), func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {
				h := newHead(b, sharding)
				for i := 0; b.Loop(); i++ {
					h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)), false)
				}
			})

			b.Run("parallel", func(b *testing.B) {
				h := newHead(b, sharding)
				var count atomic.Int64
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						i := count.Inc()
						h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(int(i))), false)
					}
				})
			})
		})
	}

	// The PreCreation() callback rejects every series.
	b.Run("preCreationFailure", func(b *testing.B) {
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1000
		opts.ChunkDirRoot = b.TempDir()

		// Mock the PreCreation() callback to fail on each series.
		opts.SeriesCallback = failingSeriesLifecycleCallback{}
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(b, err)
		b.Cleanup(func() { require.NoError(b, h.Close()) })

		for i := 0; b.Loop(); i++ {
			h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)), false)
		}
	})
}

type failingSeriesLifecycleCallback struct{}

func (failingSeriesLifecycleCallback) PreCreation(labels.Labels) error                     { return errors.New("failed") }
func (failingSeriesLifecycleCallback) PostCreation(labels.Labels)                          {}
func (failingSeriesLifecycleCallback) PostDeletion(map[chunks.HeadSeriesRef]labels.Labels) {}

func BenchmarkMmapHeadChunks(b *testing.B) {
	for _, seriesCount := range []int{1000, 10000, 100000} {
		for _, readyPct := range []float64{0.01, 0.1, 1.0} {
			readyCount := max(int(float64(seriesCount)*readyPct), 1)
			b.Run(fmt.Sprintf("series=%d/ready=%d", seriesCount, readyCount), func(b *testing.B) {
				db := newTestDB(b)
				db.DisableCompactions()
				chunkRange := DefaultBlockDuration

				// Create all series in batches.
				refs := make([]storage.SeriesRef, seriesCount)
				lblsPerSeries := make([]labels.Labels, seriesCount)
				ts := int64(0)
				const batchSize = 1000
				for batchStart := 0; batchStart < seriesCount; batchStart += batchSize {
					app := db.Appender(b.Context())
					batchEnd := min(batchStart+batchSize, seriesCount)
					for i := batchStart; i < batchEnd; i++ {
						lbls := labels.FromStrings("__name__", "bench", "i", strconv.Itoa(i))
						lblsPerSeries[i] = lbls
						ref, err := app.Append(0, lbls, ts, float64(i))
						require.NoError(b, err)
						refs[i] = ref
					}
					require.NoError(b, app.Commit())
				}

				// Pick a random, spread-out set of series to make ready each iteration.
				// Using a Fisher-Yates shuffle prefix ensures coverage across stripes.
				rng := rand.New(rand.NewSource(42))
				perm := rng.Perm(seriesCount)
				readyIndices := perm[:readyCount]

				// makeReady appends a sample past the chunk range boundary to
				// trigger a new head chunk on each ready series. Two appends
				// suffice: one near the current ts, one past nextAt.
				makeReady := func() {
					ts++ // small increment for first sample
					app := db.Appender(b.Context())
					for _, idx := range readyIndices {
						var err error
						refs[idx], err = app.Append(refs[idx], lblsPerSeries[idx], ts, float64(ts))
						require.NoError(b, err)
					}
					require.NoError(b, app.Commit())

					ts += chunkRange // jump past nextAt to force chunk cut
					app = db.Appender(b.Context())
					for _, idx := range readyIndices {
						var err error
						refs[idx], err = app.Append(refs[idx], lblsPerSeries[idx], ts, float64(ts))
						require.NoError(b, err)
					}
					require.NoError(b, app.Commit())
				}

				makeReady()

				b.ResetTimer()
				b.ReportAllocs()

				for range b.N {
					db.ForceHeadMMap()
					b.StopTimer()
					makeReady()
					b.StartTimer()
				}
			})
		}
	}
}

// setupHeadWithSeriesForSharding returns a Head with sharding enabled and
// numSeries series created in it, along with the series refs in creation order.
func setupHeadWithSeriesForSharding(b testing.TB, numSeries int) (*Head, []storage.SeriesRef) {
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = b.TempDir()
	opts.EnableSharding = true
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, h.Close()) })

	refs := make([]storage.SeriesRef, 0, numSeries)
	for i := range numSeries {
		lset := labels.FromStrings("const", "1", "unique", strconv.Itoa(i))
		s, _, err := h.getOrCreate(lset.Hash(), lset, false)
		require.NoError(b, err)
		refs = append(refs, storage.SeriesRef(s.ref))
	}
	return h, refs
}

func benchmarkHeadShardedPostings(b *testing.B, h *Head, refs []storage.SeriesRef, shardCount uint64) {
	ir := h.indexRange(math.MinInt64, math.MaxInt64)

	b.ReportAllocs()
	shard := uint64(0)
	for b.Loop() {
		sp := ir.ShardedPostings(index.NewListPostings(refs), shard%shardCount, shardCount)
		for sp.Next() {
		}
		require.NoError(b, sp.Err())
		shard++
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(len(refs)), "ns/series")
}

func BenchmarkHeadShardedPostings(b *testing.B) {
	for _, numSeries := range []int{100_000, 1_000_000} {
		for _, shardCount := range []uint64{16, 64} {
			b.Run(fmt.Sprintf("series=%d/shardCount=%d", numSeries, shardCount), func(b *testing.B) {
				h, refs := setupHeadWithSeriesForSharding(b, numSeries)
				benchmarkHeadShardedPostings(b, h, refs, shardCount)
			})
		}
	}

	// Half the series get garbage collected from the head, while their refs
	// remain in the benchmarked postings list: exercises the not-found path,
	// like querying postings that include recently deleted series.
	b.Run("series=100000/shardCount=16/withChurn", func(b *testing.B) {
		const numSeries = 100_000
		h, refs := setupHeadWithSeriesForSharding(b, numSeries)

		// Give even-numbered series a sample at t=3000 so they survive the
		// truncation below. One series gets a sample at t=0 so the head's min
		// time makes the truncation effective. Odd-numbered series stay
		// sample-less and get garbage collected.
		ctx := b.Context()
		app := h.Appender(ctx)
		_, err := app.Append(0, labels.FromStrings("const", "1", "unique", "1"), 0, 1.0)
		require.NoError(b, err)
		for i := 0; i < numSeries; i += 2 {
			lset := labels.FromStrings("const", "1", "unique", strconv.Itoa(i))
			_, err := app.Append(0, lset, 3000, 1.0)
			require.NoError(b, err)
			if i%20_000 == 0 {
				require.NoError(b, app.Commit())
				app = h.Appender(ctx)
			}
		}
		require.NoError(b, app.Commit())
		require.NoError(b, h.Truncate(2000))

		benchmarkHeadShardedPostings(b, h, refs, 16)
	})

	// Refs spaced 50 apart (~2% ref-space occupancy): production ingesters
	// accumulate refs monotonically across churn, so live refs are sparse in
	// ref space, unlike the dense refs the other variants create.
	b.Run("series=100000/shardCount=16/sparseRefs", func(b *testing.B) {
		const (
			numSeries = 100_000
			refStride = 50
		)
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1000
		opts.ChunkDirRoot = b.TempDir()
		opts.EnableSharding = true
		h, err := NewHead(nil, nil, nil, nil, opts, nil)
		require.NoError(b, err)
		b.Cleanup(func() { require.NoError(b, h.Close()) })

		refs := make([]storage.SeriesRef, 0, numSeries)
		for i := range numSeries {
			lset := labels.FromStrings("const", "1", "unique", strconv.Itoa(i))
			s, _, err := h.getOrCreateWithOptionalID(chunks.HeadSeriesRef((i+1)*refStride), lset.Hash(), lset, false)
			require.NoError(b, err)
			refs = append(refs, storage.SeriesRef(s.ref))
		}
		benchmarkHeadShardedPostings(b, h, refs, 16)
	})

	// The input postings is a merge tree over interleaved sub-lists: the
	// shape multi-matcher queries produce. Exercises implementations whose
	// cost depends on the input's Seek cost, unlike a flat list input.
	b.Run("series=100000/shardCount=16/treeInput", func(b *testing.B) {
		const (
			numSeries = 100_000
			subLists  = 64
		)
		h, refs := setupHeadWithSeriesForSharding(b, numSeries)

		// Interleave the refs across sub-lists; their merge enumerates
		// exactly the original refs.
		lists := make([][]storage.SeriesRef, subLists)
		for i, ref := range refs {
			lists[i%subLists] = append(lists[i%subLists], ref)
		}

		ir := h.indexRange(math.MinInt64, math.MaxInt64)
		b.ReportAllocs()
		shard := uint64(0)
		for b.Loop() {
			sub := make([]index.Postings, 0, subLists)
			for _, l := range lists {
				sub = append(sub, index.NewListPostings(l))
			}
			sp := ir.ShardedPostings(index.Merge(b.Context(), sub...), shard%16, 16)
			for sp.Next() {
			}
			require.NoError(b, sp.Err())
			shard++
		}
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(len(refs)), "ns/series")
	})
}

func BenchmarkStripeSeriesShardHashLookup(b *testing.B) {
	for _, numSeries := range []int{1_000_000, 3_000_000} {
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			h, refs := setupHeadWithSeriesForSharding(b, numSeries)

			b.Run("getByID", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; b.Loop(); i++ {
					if s := h.series.getByID(chunks.HeadSeriesRef(refs[i%len(refs)])); s == nil {
						b.Fatal("series not found")
					}
				}
			})

			b.Run("getByID-parallel", func(b *testing.B) {
				b.ReportAllocs()
				// Start workers at evenly spaced, deterministic offsets so
				// the access pattern is identical across runs.
				var worker atomic.Int64
				b.RunParallel(func(pb *testing.PB) {
					i := int(worker.Inc()-1) * len(refs) / runtime.GOMAXPROCS(0)
					for pb.Next() {
						if s := h.series.getByID(chunks.HeadSeriesRef(refs[i%len(refs)])); s == nil {
							panic("series not found")
						}
						i++
					}
				})
			})
		})
	}
}
