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

package remote

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

type benchWriteClient struct {
	baseLatency time.Duration
	slowLatency time.Duration
	slowEvery   int64
	maxBatch    int
	batchesSent atomic.Int64
}

func (c *benchWriteClient) Store(ctx context.Context, _ []byte, _ int) (WriteResponseStats, error) {
	n := c.batchesSent.Add(1)
	d := c.baseLatency
	if c.slowEvery > 0 && n%c.slowEvery == 0 {
		d = c.slowLatency
	}
	if d > 0 {
		timer := time.NewTimer(d)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return WriteResponseStats{}, ctx.Err()
		}
	}
	return WriteResponseStats{
		Samples:   c.maxBatch,
		Confirmed: true,
	}, nil
}

func (*benchWriteClient) Name() string     { return "benchwriteclient" }
func (*benchWriteClient) Endpoint() string { return "http://bench-remote.example/api/v1/write" }

type highCardinalityBenchFixture struct {
	relabelConfigs []*relabel.Config
	series         []record.RefSeries
	keptSeries     []record.RefSeries
	meta           []record.RefMetadata
	sampleRing     []record.RefSample
}

func newHighCardinalityBenchFixture(b *testing.B, numSeries, samplesPerIter int) *highCardinalityBenchFixture {
	b.Helper()

	const numMetricFamilies = 1_000

	// Drop ~15% of series via write_relabel_configs, matching issue #17277.
	relabelConfigs := []*relabel.Config{{
		SourceLabels:         model.LabelNames{"__name__"},
		Separator:            ";",
		Regex:                relabel.MustNewRegexp("dropped_.*"),
		Action:               relabel.Drop,
		NameValidationScheme: model.UTF8Validation,
	}}

	series := make([]record.RefSeries, numSeries)
	keptSeries := make([]record.RefSeries, 0, numSeries)
	meta := make([]record.RefMetadata, numSeries)
	keptRefs := make([]chunks.HeadSeriesRef, 0, numSeries)
	droppedRefs := make([]chunks.HeadSeriesRef, 0, numSeries/5)

	baseSymbols := labels.NewSymbolTable()
	sb := labels.NewScratchBuilder(len(extraLabels) + 2)
	for i := range numSeries {
		ref := chunks.HeadSeriesRef(i + 1)
		fam := i % numMetricFamilies
		isDropped := i%20 < 3 // 15% of series dropped.
		name := fmt.Sprintf("metric_family_%d_total", fam)
		if isDropped {
			name = fmt.Sprintf("dropped_metric_%d_total", fam)
			droppedRefs = append(droppedRefs, ref)
		} else {
			keptRefs = append(keptRefs, ref)
		}

		sb.Reset()
		sb.SetSymbolTable(baseSymbols)
		sb.Add("__name__", name)
		sb.Add("series_id", strconv.Itoa(i))
		for _, el := range extraLabels[:6] {
			sb.Add(el.Name, el.Value)
		}
		sb.Sort()
		rs := record.RefSeries{
			Ref:    ref,
			Labels: sb.Labels(),
		}
		series[i] = rs
		if !isDropped {
			keptSeries = append(keptSeries, rs)
		}
		meta[i] = record.RefMetadata{
			Ref:  ref,
			Type: uint8(record.Counter),
			Unit: "seconds",
			Help: "Total time spent processing requests in seconds.",
		}
	}

	// Build a sample ring spanning all series so successive benchmark iterations sweep
	// across the full high-cardinality working set rather than staying in L3 cache.
	// ~30% of samples reference dropped series (shorter scrape interval), matching #17277.
	ringLen := max(numSeries, samplesPerIter)
	ringLen -= ringLen % samplesPerIter
	sampleRing := make([]record.RefSample, ringLen)
	for i := range ringLen {
		var ref chunks.HeadSeriesRef
		if i%10 < 3 {
			ref = droppedRefs[(uint64(i)*7919)%uint64(len(droppedRefs))]
		} else {
			ref = keptRefs[(uint64(i)*104729)%uint64(len(keptRefs))]
		}
		sampleRing[i] = record.RefSample{
			Ref: ref,
			T:   1_700_000_000_000 + int64(i),
			V:   float64(i),
		}
	}

	return &highCardinalityBenchFixture{
		relabelConfigs: relabelConfigs,
		series:         series,
		keptSeries:     keptSeries,
		meta:           meta,
		sampleRing:     sampleRing,
	}
}

// BenchmarkQueueManagerShardScaling reproduces the high-cardinality multi-shard contention
// and throughput ceiling reported in https://github.com/prometheus/prometheus/issues/17277.
//
// Recommended CLI invocation:
//
//	export bench=qm_scaling && go test ./storage/remote \
//		-run '^$' -bench '^BenchmarkQueueManagerShardScaling$' \
//		-benchtime 2s -count 6 -cpu 16 -timeout 999m -benchmem \
//		| tee ${bench}.txt
func BenchmarkQueueManagerShardScaling(b *testing.B) {
	numSeries := 1_000_000
	if testing.Short() {
		numSeries = 100_000
	}
	const (
		walBatchSize      = 1_000 // Typical scrape commit size read by wlog.Watcher.
		samplesPerIter    = 100_000
		maxSamplesPerSend = 2_000
	)

	fix := newHighCardinalityBenchFixture(b, numSeries, samplesPerIter)

	type scenario struct {
		name        string
		shards      []int
		baseLatency time.Duration
		slowLatency time.Duration
		slowEvery   int64
		capBatches  int
	}
	scenarios := []scenario{
		{
			name:       "fast",
			shards:     []int{1, 4, 50},
			capBatches: 10,
		},
		{
			// Simulates network RTT with occasional tail latency and tight per-shard capacity,
			// matching the queue backpressure conditions in issue #17277.
			name:        "jitter_rtt",
			shards:      []int{4, 50},
			baseLatency: 5 * time.Millisecond,
			slowLatency: 40 * time.Millisecond,
			slowEvery:   25,
			capBatches:  2,
		},
	}

	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		protoName := "rw1"
		if protoMsg == remoteapi.WriteV2MessageType {
			protoName = "rw2"
		}
		for _, sc := range scenarios {
			for _, numShards := range sc.shards {
				b.Run(fmt.Sprintf("proto=%s/mode=%s/shards=%d", protoName, sc.name, numShards), func(b *testing.B) {
					client := &benchWriteClient{
						baseLatency: sc.baseLatency,
						slowLatency: sc.slowLatency,
						slowEvery:   sc.slowEvery,
						maxBatch:    maxSamplesPerSend,
					}
					cfg := config.DefaultQueueConfig
					cfg.BatchSendDeadline = model.Duration(50 * time.Millisecond)
					cfg.MaxSamplesPerSend = maxSamplesPerSend
					cfg.Capacity = maxSamplesPerSend * sc.capBatches
					cfg.MinShards = numShards
					cfg.MaxShards = numShards

					metrics := newQueueManagerMetrics(nil, "", "")
					qm := NewQueueManager(
						metrics, nil, nil, nil, b.TempDir(),
						newEWMARate(ewmaWeight, shardUpdateDuration),
						cfg, config.DefaultMetadataConfig,
						labels.EmptyLabels(), fix.relabelConfigs, client,
						defaultFlushDeadline, newPool(), newHighestTimestampMetric(),
						nil, false, false, false, protoMsg, remoteapi.WriteModeNone,
						record.NewBuffersPool(), false, false,
					)
					qm.StoreSeries(fix.series, 0)
					qm.StoreMetadata(fix.meta)
					qm.Start()

					// Warm up shard goroutines and reusable batch/encoding buffers before timing.
					warmupSamples := min(len(fix.sampleRing), maxSamplesPerSend*numShards*2)
					for off := 0; off < warmupSamples; off += walBatchSize {
						end := min(off+walBatchSize, warmupSamples)
						require.True(b, qm.Append(fix.sampleRing[off:end]))
					}
					deadline := time.Now().Add(2 * time.Second)
					for client.batchesSent.Load() < int64(numShards) && time.Now().Before(deadline) {
						time.Sleep(time.Millisecond)
					}

					b.ReportAllocs()
					b.ResetTimer()
					var totalSamples int64
					ringPos := 0
					for b.Loop() {
						window := fix.sampleRing[ringPos : ringPos+samplesPerIter]
						ringPos += samplesPerIter
						if ringPos >= len(fix.sampleRing) {
							ringPos = 0
						}
						for off := 0; off < len(window); off += walBatchSize {
							require.True(b, qm.Append(window[off:off+walBatchSize]))
						}
						totalSamples += int64(len(window))
					}
					qm.Stop()
					b.StopTimer()

					elapsed := b.Elapsed().Seconds()
					if elapsed > 0 {
						b.ReportMetric(float64(totalSamples)/elapsed, "wal-samples/s")
						b.ReportMetric(float64(totalSamples)*0.7/elapsed, "kept-samples/s")
					}
				})
			}
		}
	}
}

// BenchmarkQueueManagerCheckpointGCContention measures Append throughput while background
// checkpoint series garbage collection (UpdateSeriesSegment + SeriesReset) runs concurrently,
// matching wlog.Watcher.garbageCollectSeries.
//
// Recommended CLI invocation:
//
//	export bench=qm_gc && go test ./storage/remote \
//		-run '^$' -bench '^BenchmarkQueueManagerCheckpointGCContention$' \
//		-benchtime 2s -count 6 -cpu 16 -timeout 999m -benchmem \
//		| tee ${bench}.txt
func BenchmarkQueueManagerCheckpointGCContention(b *testing.B) {
	numSeries := 500_000
	if testing.Short() {
		numSeries = 50_000
	}
	const (
		walBatchSize      = 1_000
		samplesPerIter    = 50_000
		maxSamplesPerSend = 2_000
		numShards         = 16
	)

	fix := newHighCardinalityBenchFixture(b, numSeries, samplesPerIter)

	client := &benchWriteClient{maxBatch: maxSamplesPerSend}
	cfg := config.DefaultQueueConfig
	cfg.BatchSendDeadline = model.Duration(50 * time.Millisecond)
	cfg.MaxSamplesPerSend = maxSamplesPerSend
	cfg.Capacity = maxSamplesPerSend * 10
	cfg.MinShards = numShards
	cfg.MaxShards = numShards

	metrics := newQueueManagerMetrics(nil, "", "")
	qm := NewQueueManager(
		metrics, nil, nil, nil, b.TempDir(),
		newEWMARate(ewmaWeight, shardUpdateDuration),
		cfg, config.DefaultMetadataConfig,
		labels.EmptyLabels(), fix.relabelConfigs, client,
		defaultFlushDeadline, newPool(), newHighestTimestampMetric(),
		nil, false, false, false, remoteapi.WriteV2MessageType, remoteapi.WriteModeNone,
		record.NewBuffersPool(), false, false,
	)
	qm.StoreSeries(fix.series, 0)
	qm.StoreMetadata(fix.meta)
	qm.Start()

	var wg sync.WaitGroup
	stopGC := make(chan struct{})
	wg.Go(func() {
		seg := 1
		for {
			select {
			case <-stopGC:
				return
			default:
			}
			qm.UpdateSeriesSegment(fix.series, seg)
			qm.SeriesReset(seg)
			seg++
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	var totalSamples int64
	ringPos := 0
	for b.Loop() {
		window := fix.sampleRing[ringPos : ringPos+samplesPerIter]
		ringPos += samplesPerIter
		if ringPos >= len(fix.sampleRing) {
			ringPos = 0
		}
		for off := 0; off < len(window); off += walBatchSize {
			require.True(b, qm.Append(window[off:off+walBatchSize]))
		}
		totalSamples += int64(len(window))
	}
	close(stopGC)
	wg.Wait()
	qm.Stop()
	b.StopTimer()

	elapsed := b.Elapsed().Seconds()
	if elapsed > 0 {
		b.ReportMetric(float64(totalSamples)/elapsed, "wal-samples/s")
		b.ReportMetric(float64(totalSamples)*0.7/elapsed, "kept-samples/s")
	}
}
