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
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
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
					b.Cleanup(func() { require.NoError(b, h.Close()) })

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
	chunkDir := b.TempDir()
	// Put a series, select it. GC it and then access it.
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = chunkDir
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(b, err)
	defer h.Close()

	for i := 0; b.Loop(); i++ {
		h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)), false)
	}
}

func BenchmarkHeadStripeSeriesCreateParallel(b *testing.B) {
	chunkDir := b.TempDir()
	// Put a series, select it. GC it and then access it.
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = chunkDir
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(b, err)
	defer h.Close()

	var count atomic.Int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := count.Inc()
			h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(int(i))), false)
		}
	})
}

func BenchmarkHeadStripeSeriesCreate_PreCreationFailure(b *testing.B) {
	chunkDir := b.TempDir()
	// Put a series, select it. GC it and then access it.
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = chunkDir

	// Mock the PreCreation() callback to fail on each series.
	opts.SeriesCallback = failingSeriesLifecycleCallback{}

	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(b, err)
	defer h.Close()

	for i := 0; b.Loop(); i++ {
		h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)), false)
	}
}

type failingSeriesLifecycleCallback struct{}

func (failingSeriesLifecycleCallback) PreCreation(labels.Labels) error                     { return errors.New("failed") }
func (failingSeriesLifecycleCallback) PostCreation(labels.Labels)                          {}
func (failingSeriesLifecycleCallback) PostDeletion(map[chunks.HeadSeriesRef]labels.Labels) {}
