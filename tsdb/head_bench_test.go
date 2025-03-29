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

package tsdb

import (
	"context"
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

func BenchmarkHeadStripeSeriesCreate(b *testing.B) {
	chunkDir := b.TempDir()
	// Put a series, select it. GC it and then access it.
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = chunkDir
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(b, err)
	defer h.Close()

	for i := 0; i < b.N; i++ {
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

	for i := 0; i < b.N; i++ {
		h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)), false)
	}
}

func BenchmarkHead_WalCommit(b *testing.B) {
	seriesCounts := []int{100, 1000, 10000}
	series := genSeries(10000, 10, 0, 0) // Only using the generated labels.

	appendSamples := func(b *testing.B, app storage.Appender, seriesCount int, ts int64) {
		var err error
		for i, s := range series[:seriesCount] {
			var ref storage.SeriesRef
			// if i is even, append a sample, else append a histogram.
			if i%2 == 0 {
				ref, err = app.Append(ref, s.Labels(), ts, float64(ts))
			} else {
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
			}
			require.NoError(b, err)

			_, err = app.AppendExemplar(ref, s.Labels(), exemplar.Exemplar{
				Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
				Value:  rand.Float64(),
				Ts:     ts,
			})
			require.NoError(b, err)
		}
	}

	for _, seriesCount := range seriesCounts {
		b.Run(fmt.Sprintf("%d series", seriesCount), func(b *testing.B) {
			for _, commits := range []int64{1, 2} { // To test commits that create new series and when the series already exists.
				b.Run(fmt.Sprintf("%d commits", commits), func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						b.StopTimer()
						h, w := newTestHead(b, 10000, compression.None, false)
						b.Cleanup(func() {
							if h != nil {
								h.Close()
							}
							if w != nil {
								w.Close()
							}
						})
						app := h.Appender(context.Background())

						appendSamples(b, app, seriesCount, 0)

						b.StartTimer()
						require.NoError(b, app.Commit())
						if commits == 2 {
							b.StopTimer()
							app = h.Appender(context.Background())
							appendSamples(b, app, seriesCount, 1)
							b.StartTimer()
							require.NoError(b, app.Commit())
						}
						b.StopTimer()
						h.Close()
						h = nil
						w.Close()
						w = nil
					}
				})
			}
		})
	}
}

type failingSeriesLifecycleCallback struct{}

func (failingSeriesLifecycleCallback) PreCreation(labels.Labels) error                     { return errors.New("failed") }
func (failingSeriesLifecycleCallback) PostCreation(labels.Labels)                          {}
func (failingSeriesLifecycleCallback) PostDeletion(map[chunks.HeadSeriesRef]labels.Labels) {}
