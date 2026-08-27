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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/compression"
)

// capturingV2WriteClient captures all deserialized writev2.Request payloads.
type capturingV2WriteClient struct {
	mtx      sync.Mutex
	requests []*writev2.Request
	compr    compression.Type
}

func newCapturingV2WriteClient(compr compression.Type) *capturingV2WriteClient {
	return &capturingV2WriteClient{
		compr: compr,
	}
}

func (c *capturingV2WriteClient) Store(_ context.Context, req []byte, _ int) (WriteResponseStats, error) {
	decomp, err := compression.Decode(c.compr, req, nil)
	if err != nil {
		return WriteResponseStats{}, err
	}
	var v2Req writev2.Request
	if err := v2Req.Unmarshal(decomp); err != nil {
		return WriteResponseStats{}, err
	}

	c.mtx.Lock()
	c.requests = append(c.requests, &v2Req)
	c.mtx.Unlock()

	var numSamples, numHistograms, numExemplars int
	for _, ts := range v2Req.Timeseries {
		numSamples += len(ts.Samples)
		numHistograms += len(ts.Histograms)
		numExemplars += len(ts.Exemplars)
	}

	return WriteResponseStats{
		Samples:    numSamples,
		Histograms: numHistograms,
		Exemplars:  numExemplars,
	}, nil
}

func (c *capturingV2WriteClient) Name() string     { return "capturing-v2-client" }
func (c *capturingV2WriteClient) Endpoint() string { return "http://localhost/write" }

func (c *capturingV2WriteClient) getRequests() []*writev2.Request {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	copied := make([]*writev2.Request, len(c.requests))
	copy(copied, c.requests)
	return copied
}

func TestQueueManager_PRW2_Coalescing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(50 * time.Millisecond)
	queueConfig.MaxShards = 1
	queueConfig.Capacity = 100
	queueConfig.MaxSamplesPerSend = 50

	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = true
	writeConfig.SendNativeHistograms = true
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

	client := newCapturingV2WriteClient(qm.compr)
	qm.SetClient(client)

	// Register series 1 to 6
	series := []record.RefSeries{
		{Ref: 1, Labels: labels.FromStrings("__name__", "metric_sample_first", "instance", "localhost")},
		{Ref: 2, Labels: labels.FromStrings("__name__", "metric_exemplar_first", "instance", "localhost")},
		{Ref: 3, Labels: labels.FromStrings("__name__", "metric_histogram", "instance", "localhost")},
		{Ref: 4, Labels: labels.FromStrings("__name__", "metric_float_histogram", "instance", "localhost")},
		{Ref: 5, Labels: labels.FromStrings("__name__", "metric_unmatched_exemplar", "instance", "localhost")},
		{Ref: 6, Labels: labels.FromStrings("__name__", "metric_cross_scrape", "instance", "localhost")},
	}
	qm.StoreSeries(series, 0)

	// 1. Sample-First: Enqueue sample at T=1000, then exemplar at T=1010
	qm.Append([]record.RefSample{{Ref: 1, T: 1000, V: 42.0}})
	qm.AppendExemplars([]record.RefExemplar{{Ref: 1, T: 1010, V: 42.0, Labels: labels.FromStrings("trace_id", "trace-sample-first")}})

	// 2. Exemplar-First: Enqueue exemplar at T=2000, then sample at T=2020
	qm.AppendExemplars([]record.RefExemplar{{Ref: 2, T: 2000, V: 84.0, Labels: labels.FromStrings("trace_id", "trace-exemplar-first")}})
	qm.Append([]record.RefSample{{Ref: 2, T: 2020, V: 84.0}})

	// 3. Int Histogram: Enqueue exemplar at T=3000, then histogram at T=3015
	h := &histogram.Histogram{Schema: 1, Count: 10, Sum: 25.0}
	qm.AppendExemplars([]record.RefExemplar{{Ref: 3, T: 3000, V: 5.0, Labels: labels.FromStrings("trace_id", "trace-histogram")}})
	qm.AppendHistograms([]record.RefHistogramSample{{Ref: 3, T: 3015, H: h}})

	// 4. Float Histogram: Enqueue exemplar at T=4000, then float histogram at T=4010
	fh := &histogram.FloatHistogram{Schema: 1, Count: 20, Sum: 50.0}
	qm.AppendExemplars([]record.RefExemplar{{Ref: 4, T: 4000, V: 7.5, Labels: labels.FromStrings("trace_id", "trace-float-histogram")}})
	qm.AppendFloatHistograms([]record.RefFloatHistogramSample{{Ref: 4, T: 4010, FH: fh}})

	// 5. Unmatched exemplar (no sample arrives for ref 5)
	qm.AppendExemplars([]record.RefExemplar{{Ref: 5, T: 5000, V: 99.0, Labels: labels.FromStrings("trace_id", "trace-unmatched")}})

	// 6. Cross-scrape rejection (>50ms delta): Sample at T=6000, Exemplar at T=6100 (100ms > 50ms)
	qm.Append([]record.RefSample{{Ref: 6, T: 6000, V: 111.0}})
	qm.AppendExemplars([]record.RefExemplar{{Ref: 6, T: 6100, V: 111.0, Labels: labels.FromStrings("trace_id", "trace-cross-scrape")}})

	// Wait for batches to be flushed by deadline
	require.Eventually(t, func() bool {
		reqs := client.getRequests()
		var totalSeries int
		for _, r := range reqs {
			totalSeries += len(r.Timeseries)
		}
		// Expect 5 sent series: refs 1, 2, 3, 4, 6 (ref 5 has no sample so must not be sent)
		return totalSeries >= 5
	}, 5*time.Second, 20*time.Millisecond)

	reqs := client.getRequests()
	require.NotEmpty(t, reqs)

	var totalSamples, totalHistograms, totalExemplars int
	for _, r := range reqs {
		for _, ts := range r.Timeseries {
			// PRW 2.0 Invariant: Every TimeSeries MUST contain at least one sample or histogram.
			// ZERO empty TimeSeries with only exemplars allowed!
			hasData := len(ts.Samples) > 0 || len(ts.Histograms) > 0
			require.True(t, hasData, "PRW 2.0 violation: TimeSeries must not be empty of samples/histograms")

			totalSamples += len(ts.Samples)
			totalHistograms += len(ts.Histograms)
			totalExemplars += len(ts.Exemplars)

			// If exemplars are present, verify they are attached to a valid sample/histogram
			if len(ts.Exemplars) > 0 {
				require.True(t, len(ts.Samples) > 0 || len(ts.Histograms) > 0)
			}
		}
	}

	// 3 float samples (refs 1, 2, 6) and 2 histograms (refs 3, 4)
	require.Equal(t, 3, totalSamples)
	require.Equal(t, 2, totalHistograms)
	// 4 matched exemplars (refs 1, 2, 3, 4)
	require.Equal(t, 4, totalExemplars)

	// Close storage to flush/drain coalescers
	s.Close()

	// Metrics validation:
	// 4 exemplars sent, 2 dropped (ref 5 unmatched, ref 6 cross-scrape >50ms)
	require.Equal(t, float64(4), client_testutil.ToFloat64(qm.metrics.exemplarsTotal))
	require.Equal(t, float64(2), client_testutil.ToFloat64(qm.metrics.unmatchedExemplarsDroppedTotal))
	require.Equal(t, float64(3), client_testutil.ToFloat64(qm.metrics.samplesTotal))
	require.Equal(t, float64(2), client_testutil.ToFloat64(qm.metrics.histogramsTotal))
}

func TestQueueManager_Resharding(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(20 * time.Millisecond)
	queueConfig.MinShards = 1
	queueConfig.MaxShards = 16
	queueConfig.Capacity = 200
	queueConfig.MaxSamplesPerSend = 50

	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = true
	writeConfig.SendNativeHistograms = true
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

	client := newCapturingV2WriteClient(qm.compr)
	qm.SetClient(client)

	numSeries := 100
	series := make([]record.RefSeries, numSeries)
	for i := 0; i < numSeries; i++ {
		series[i] = record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i + 1),
			Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", i), "instance", "localhost"),
		}
	}
	qm.StoreSeries(series, 0)

	var (
		stopAppend atomic.Bool
		wg         sync.WaitGroup
		appendedSamples atomic.Int64
		appendedExemplars atomic.Int64
	)

	// Concurrently append paired samples and exemplars
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var seq int64
			for !stopAppend.Load() {
				seq++
				ref := chunks.HeadSeriesRef((seq % int64(numSeries)) + 1)
				ts := time.Now().UnixMilli()

				qm.Append([]record.RefSample{{Ref: ref, T: ts, V: float64(seq)}})
				appendedSamples.Add(1)

				qm.AppendExemplars([]record.RefExemplar{{
					Ref:    ref,
					T:      ts,
					V:      float64(seq),
					Labels: labels.FromStrings("trace_id", fmt.Sprintf("trace-%d-%d", w, seq)),
				}})
				appendedExemplars.Add(1)

				time.Sleep(500 * time.Microsecond)
			}
		}(worker)
	}

	// Concurrently trigger dynamic resharding back and forth
	shardTargets := []int{2, 4, 1, 8, 3, 6, 2, 4}
	for _, target := range shardTargets {
		time.Sleep(50 * time.Millisecond)
		qm.reshardChan <- target
	}

	time.Sleep(200 * time.Millisecond)
	stopAppend.Store(true)
	wg.Wait()

	// Wait for flush
	require.Eventually(t, func() bool {
		return client_testutil.ToFloat64(qm.metrics.pendingSamples) == 0
	}, 10*time.Second, 50*time.Millisecond)

	s.Close()

	reqs := client.getRequests()
	require.NotEmpty(t, reqs)

	// Verify all received TimeSeries are strictly valid PRW 2.0 (no empty series)
	for _, r := range reqs {
		for _, ts := range r.Timeseries {
			hasSampleOrHistogram := len(ts.Samples) > 0 || len(ts.Histograms) > 0
			require.True(t, hasSampleOrHistogram, "PRW 2.0 invariant: must contain at least 1 sample or histogram")
		}
	}
}
