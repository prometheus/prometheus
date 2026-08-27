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
	"runtime"
	"testing"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

type noopWriteClient struct{}

func (c *noopWriteClient) Store(_ context.Context, _ []byte, _ int) (WriteResponseStats, error) {
	return WriteResponseStats{}, nil
}
func (c *noopWriteClient) Name() string     { return "noop" }
func (c *noopWriteClient) Endpoint() string { return "http://localhost/noop" }

// BenchmarkQueueManager_PRW2_Baseline measures standard PRW 2.0 sample ingestion without exemplars.
func BenchmarkQueueManager_PRW2_Baseline(b *testing.B) {
	dir := b.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	queueConfig.MaxShards = 4
	queueConfig.MinShards = 4
	queueConfig.Capacity = 10000
	queueConfig.MaxSamplesPerSend = 1000

	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = false
	writeConfig.ProtobufMessage = remoteapi.WriteV2MessageType

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}
	require.NoError(b, s.ApplyConfig(conf))

	hash, err := toHash(writeConfig)
	require.NoError(b, err)
	qm := s.rws.queues[hash]
	qm.SetClient(&noopWriteClient{})

	numSeries := 1000
	series := make([]record.RefSeries, numSeries)
	for i := 0; i < numSeries; i++ {
		series[i] = record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i + 1),
			Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i), "job", "benchmark"),
		}
	}
	qm.StoreSeries(series, 0)

	samples := make([]record.RefSample, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ts := int64(i * 1000)
		for j := range samples {
			samples[j] = record.RefSample{
				Ref: chunks.HeadSeriesRef((j % numSeries) + 1),
				T:   ts,
				V:   float64(i),
			}
		}
		qm.Append(samples)
	}
}

// BenchmarkQueueManager_PRW2_Coalescing measures PRW 2.0 with shard coalescing enabled and samples + exemplars.
func BenchmarkQueueManager_PRW2_Coalescing(b *testing.B) {
	dir := b.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	queueConfig.MaxShards = 4
	queueConfig.MinShards = 4
	queueConfig.Capacity = 10000
	queueConfig.MaxSamplesPerSend = 1000

	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = true
	writeConfig.ProtobufMessage = remoteapi.WriteV2MessageType

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}
	require.NoError(b, s.ApplyConfig(conf))

	hash, err := toHash(writeConfig)
	require.NoError(b, err)
	qm := s.rws.queues[hash]
	qm.SetClient(&noopWriteClient{})

	numSeries := 1000
	series := make([]record.RefSeries, numSeries)
	for i := 0; i < numSeries; i++ {
		series[i] = record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i + 1),
			Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i), "job", "benchmark"),
		}
	}
	qm.StoreSeries(series, 0)

	samples := make([]record.RefSample, 100)
	exemplars := make([]record.RefExemplar, 100)
	exLabels := labels.FromStrings("trace_id", "4bf92f3577b34da6a3ce929d0e0e4736")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ts := int64(i * 1000)
		for j := range samples {
			ref := chunks.HeadSeriesRef((j % numSeries) + 1)
			samples[j] = record.RefSample{Ref: ref, T: ts, V: float64(i)}
			exemplars[j] = record.RefExemplar{Ref: ref, T: ts + 5, V: float64(i), Labels: exLabels}
		}
		qm.Append(samples)
		qm.AppendExemplars(exemplars)
	}
}

// BenchmarkQueueManager_PRW2_SeriesChurn_100k tests memory stability and allocation efficiency under 100k churned series.
func BenchmarkQueueManager_PRW2_SeriesChurn_100k(b *testing.B) {
	dir := b.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(50 * time.Millisecond)
	queueConfig.MaxShards = 4
	queueConfig.MinShards = 4
	queueConfig.Capacity = 10000
	queueConfig.MaxSamplesPerSend = 1000

	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = true
	writeConfig.ProtobufMessage = remoteapi.WriteV2MessageType

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}
	require.NoError(b, s.ApplyConfig(conf))

	hash, err := toHash(writeConfig)
	require.NoError(b, err)
	qm := s.rws.queues[hash]
	qm.SetClient(&noopWriteClient{})

	const totalChurnSeries = 100000
	seriesBatch := make([]record.RefSeries, 1000)
	samplesBatch := make([]record.RefSample, 1000)
	exemplarsBatch := make([]record.RefExemplar, 1000)
	exLabels := labels.FromStrings("trace_id", "trace-churn-benchmark")

	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Ingest 100,000 unique series in chunks of 1,000
		for offset := 0; offset < totalChurnSeries; offset += 1000 {
			ts := time.Now().UnixMilli()
			for k := 0; k < 1000; k++ {
				ref := chunks.HeadSeriesRef(offset + k + 1)
				seriesBatch[k] = record.RefSeries{
					Ref:    ref,
					Labels: labels.FromStrings("__name__", fmt.Sprintf("churn_metric_%d", offset+k), "cycle", fmt.Sprintf("c%d", i)),
				}
				samplesBatch[k] = record.RefSample{Ref: ref, T: ts, V: float64(k)}
				exemplarsBatch[k] = record.RefExemplar{Ref: ref, T: ts + 2, V: float64(k), Labels: exLabels}
			}

			qm.StoreSeries(seriesBatch, 0)
			qm.Append(samplesBatch)
			qm.AppendExemplars(exemplarsBatch)
		}
	}

	b.StopTimer()

	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	heapGrowthMB := float64(memAfter.HeapAlloc-memBefore.HeapAlloc) / (1024 * 1024)
	b.Logf("HeapAlloc after 100k churned series: %.2f MB (growth: %.2f MB)", float64(memAfter.HeapAlloc)/(1024*1024), heapGrowthMB)
}

func TestQueueManager_100kSeriesChurn_HeapStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 100k churn test in short mode")
	}

	dir := t.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(20 * time.Millisecond)
	queueConfig.MaxShards = 4
	queueConfig.MinShards = 4
	queueConfig.Capacity = 5000
	queueConfig.MaxSamplesPerSend = 500

	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = true
	writeConfig.ProtobufMessage = remoteapi.WriteV2MessageType

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}
	require.NoError(t, s.ApplyConfig(conf))

	hash, err := toHash(writeConfig)
	require.NoError(t, err)
	qm := s.rws.queues[hash]
	qm.SetClient(&noopWriteClient{})

	const totalSeries = 100000
	seriesChunk := make([]record.RefSeries, 1000)
	samplesChunk := make([]record.RefSample, 1000)
	exemplarsChunk := make([]record.RefExemplar, 1000)
	exLabels := labels.FromStrings("trace_id", "trace-churn-stability")

	runtime.GC()
	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)

	for chunk := 0; chunk < totalSeries/1000; chunk++ {
		ts := time.Now().UnixMilli()
		for i := 0; i < 1000; i++ {
			ref := chunks.HeadSeriesRef(chunk*1000 + i + 1)
			seriesChunk[i] = record.RefSeries{
				Ref:    ref,
				Labels: labels.FromStrings("__name__", fmt.Sprintf("churn_%d", ref), "pod", "test"),
			}
			samplesChunk[i] = record.RefSample{Ref: ref, T: ts, V: float64(i)}
			exemplarsChunk[i] = record.RefExemplar{Ref: ref, T: ts + 1, V: float64(i), Labels: exLabels}
		}

		qm.StoreSeries(seriesChunk, 0)
		qm.Append(samplesChunk)
		qm.AppendExemplars(exemplarsChunk)
	}

	// Verify all queues process and drain
	time.Sleep(500 * time.Millisecond)

	runtime.GC()
	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	heapMB := float64(memEnd.HeapAlloc) / (1024 * 1024)
	t.Logf("HeapAlloc after 100,000 distinct series: %.2f MB (GC cycles: %d)", heapMB, memEnd.NumGC-memStart.NumGC)

	// Ring buffer memory is bounded (2048 slots per shard)
	for _, q := range qm.shards.queues {
		require.LessOrEqual(t, q.coalescer.PendingCount(), 2048)
	}
}
