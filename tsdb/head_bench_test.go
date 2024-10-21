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
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/wlog"
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
		h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)))
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
			h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(int(i))))
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
		h.getOrCreate(uint64(i), labels.FromStrings("a", strconv.Itoa(i)))
	}
}

func BenchmarkHead_WalCommit(b *testing.B) {
	minute := func(m int) int64 { return int64(m) * time.Minute.Milliseconds() }
	seriesCounts := []int{100, 1000, 10000}
	series := genSeries(10000, 10, 0, 0)
	histograms := genHistogramSeries(10000, 10, minute(0), minute(119), minute(1), true)

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

						for _, s := range histograms[:seriesCount] {
							var ref storage.SeriesRef
							for sampleIndex := int64(0); sampleIndex < samplesPerAppend; sampleIndex++ {
								ref, err = app.Append(ref, s.Labels(), ts+sampleIndex, float64(ts+sampleIndex))
								if err != nil {
									return err
								}

								_, err = app.AppendExemplar(ref, s.Labels(), exemplar.Exemplar{
									Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
									Value:  rand.Float64(),
									Ts:     ts,
								})
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

type failingSeriesLifecycleCallback struct{}

func (failingSeriesLifecycleCallback) PreCreation(labels.Labels) error                     { return errors.New("failed") }
func (failingSeriesLifecycleCallback) PostCreation(labels.Labels)                          {}
func (failingSeriesLifecycleCallback) PostDeletion(map[chunks.HeadSeriesRef]labels.Labels) {}
