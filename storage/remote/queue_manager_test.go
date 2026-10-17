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
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gogo/protobuf/proto"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/runutil"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/prometheus/prometheus/util/testwal"
)

const defaultFlushDeadline = 1 * time.Minute

type recCase = testwal.RecordsCase

func newHighestTimestampMetric() *maxTimestamp {
	return &maxTimestamp{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "highest_timestamp_in_seconds",
			Help:      "Highest timestamp that has come into the remote storage via the Appender interface, in seconds since epoch. Initialized to 0 when no data has been received yet",
		}),
	}
}

func TestBasicContentNegotiation(t *testing.T) {
	t.Parallel()

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	queueConfig.MaxShards = 1

	// We need to set URL's so that metric creation doesn't panic.
	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}

	for _, tc := range []struct {
		name             string
		senderProtoMsg   remoteapi.WriteMessageType
		receiverProtoMsg remoteapi.WriteMessageType
		injectErrs       []error
		expectFail       bool
	}{
		{
			name:           "v2 happy path",
			senderProtoMsg: remoteapi.WriteV2MessageType, receiverProtoMsg: remoteapi.WriteV2MessageType,
			injectErrs: []error{nil},
		},
		{
			name:           "v1 happy path",
			senderProtoMsg: remoteapi.WriteV1MessageType, receiverProtoMsg: remoteapi.WriteV1MessageType,
			injectErrs: []error{nil},
		},
		// Test a case where the v1 request has a temporary delay but goes through on retry.
		{
			name:           "v1 happy path with one 5xx retry",
			senderProtoMsg: remoteapi.WriteV1MessageType, receiverProtoMsg: remoteapi.WriteV1MessageType,
			injectErrs: []error{RecoverableError{errors.New("pretend 500"), 1}, nil},
		},
		// Repeat the above test but with v2. The request has a temporary delay but goes through on retry.
		{
			name:           "v2 happy path with one 5xx retry",
			senderProtoMsg: remoteapi.WriteV2MessageType, receiverProtoMsg: remoteapi.WriteV2MessageType,
			injectErrs: []error{RecoverableError{errors.New("pretend 500"), 1}, nil},
		},
		// A few error cases of v2 talking to v1.
		{
			name:           "v2 talks to v1 that gives 400 or 415",
			senderProtoMsg: remoteapi.WriteV2MessageType, receiverProtoMsg: remoteapi.WriteV1MessageType,
			injectErrs: []error{errors.New("pretend unrecoverable err")},
			expectFail: true,
		},
		{
			name:           "v2 talks to (broken) v1 that tries to unmarshal v2 payload with v1 proto",
			senderProtoMsg: remoteapi.WriteV2MessageType, receiverProtoMsg: remoteapi.WriteV1MessageType,
			injectErrs: []error{nil},
			expectFail: true, // We detect this thanks to https://github.com/prometheus/prometheus/issues/14359
		},
		// Opposite, v1 talking to v2 only server.
		{
			name:           "v1 talks to v2 that gives 400 or 415",
			senderProtoMsg: remoteapi.WriteV1MessageType, receiverProtoMsg: remoteapi.WriteV2MessageType,
			injectErrs: []error{errors.New("pretend unrecoverable err")},
			expectFail: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
			defer s.Close()

			recs := testwal.GenerateRecords(recCase{
				NoST:   tc.senderProtoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
				Series: 1, SamplesPerSeries: 1,
			})

			conf.RemoteWriteConfigs[0].ProtobufMessage = tc.senderProtoMsg
			require.NoError(t, s.ApplyConfig(conf))
			hash, err := toHash(writeConfig)
			require.NoError(t, err)
			qm := s.rws.queues[hash]

			c := NewTestWriteClient(tc.receiverProtoMsg)
			c.injectErrors(tc.injectErrs)
			qm.SetClient(c)

			qm.StoreSeries(recs.Series, 0)
			qm.StoreMetadata(recs.Metadata)

			// Do we expect some data back?
			if !tc.expectFail {
				c.expectSamples(recs.Samples, recs.Series)
			} else {
				c.expectSamples(nil, nil)
			}

			// Schedule send.
			qm.Append(recs.Samples)

			if !tc.expectFail {
				// No error expected, so wait for data.
				c.waitForExpectedData(t, 5*time.Second)
				require.Equal(t, 0.0, client_testutil.ToFloat64(qm.metrics.failedSamplesTotal))
			} else {
				// Wait for failure to be recorded in metrics.
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, runutil.Retry(500*time.Millisecond, ctx.Done(), func() error {
					if client_testutil.ToFloat64(qm.metrics.failedSamplesTotal) != 1.0 {
						return fmt.Errorf("expected one sample failed in qm metrics; got %v", client_testutil.ToFloat64(qm.metrics.failedSamplesTotal))
					}
					return nil
				}))
			}

			// samplesTotal means attempts.
			require.Equal(t, float64(len(tc.injectErrs)), client_testutil.ToFloat64(qm.metrics.samplesTotal))
			require.Equal(t, float64(len(tc.injectErrs)-1), client_testutil.ToFloat64(qm.metrics.retriedSamplesTotal))
		})
	}
}

func TestSampleDelivery(t *testing.T) {
	t.Parallel()
	n := 3

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	queueConfig.MaxShards = 1

	// We need to set URL's so that metric creation doesn't panic.
	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = true
	writeConfig.SendNativeHistograms = true

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}

	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		for _, rc := range []recCase{
			{Series: n, SamplesPerSeries: n, HistogramsPerSeries: 0, FloatHistogramsPerSeries: 0, ExemplarsPerSeries: 0, Name: "samples only"},
			{Series: n, SamplesPerSeries: 0, HistogramsPerSeries: n, FloatHistogramsPerSeries: 0, ExemplarsPerSeries: 0, Name: "histograms only"},
			{Series: n, SamplesPerSeries: 0, HistogramsPerSeries: 0, FloatHistogramsPerSeries: n, ExemplarsPerSeries: 0, Name: "float histograms only"},
			{Series: n, SamplesPerSeries: 0, HistogramsPerSeries: 0, FloatHistogramsPerSeries: 0, ExemplarsPerSeries: n, Name: "exemplars only"},
			{Series: n, SamplesPerSeries: n, HistogramsPerSeries: n, FloatHistogramsPerSeries: n, ExemplarsPerSeries: n, Name: "all"},
		} {
			t.Run(fmt.Sprintf("proto=%s/case=%s", protoMsg, rc.Name), func(t *testing.T) {
				dir := t.TempDir()
				s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
				defer s.Close()

				rc.NoST = protoMsg == remoteapi.WriteV1MessageType // RW1 does not support ST.
				recs := testwal.GenerateRecords(rc)

				var (
					series          = recs.Series
					metadata        = recs.Metadata
					samples         = recs.Samples
					exemplars       = recs.Exemplars
					histograms      = recs.Histograms
					floatHistograms = recs.FloatHistograms
				)

				// Apply new config.
				queueConfig.Capacity = n
				queueConfig.MaxSamplesPerSend = n / 2
				conf.RemoteWriteConfigs[0].ProtobufMessage = protoMsg
				require.NoError(t, s.ApplyConfig(conf))
				hash, err := toHash(writeConfig)
				require.NoError(t, err)
				qm := s.rws.queues[hash]

				c := NewTestWriteClient(protoMsg)
				c.SetStoreWait(10 * time.Millisecond)
				qm.SetClient(c)

				qm.StoreSeries(series, 0)
				qm.StoreMetadata(metadata)

				// Send first half of data.
				c.expectSamples(samples[:len(samples)/2], series)
				c.expectExemplars(exemplars[:len(exemplars)/2], series)
				c.expectHistograms(histograms[:len(histograms)/2], series)
				c.expectFloatHistograms(floatHistograms[:len(floatHistograms)/2], series)
				if protoMsg == remoteapi.WriteV2MessageType && len(metadata) > 0 {
					c.expectMetadataForBatch(metadata, series, samples[:len(samples)/2], exemplars[:len(exemplars)/2], histograms[:len(histograms)/2], floatHistograms[:len(floatHistograms)/2])
				}
				qm.Append(samples[:len(samples)/2])
				qm.AppendExemplars(exemplars[:len(exemplars)/2])
				qm.AppendHistograms(histograms[:len(histograms)/2])
				qm.AppendFloatHistograms(floatHistograms[:len(floatHistograms)/2])
				c.waitForExpectedData(t, 30*time.Second)

				// Send second half of data.
				c.expectSamples(samples[len(samples)/2:], series)
				c.expectExemplars(exemplars[len(exemplars)/2:], series)
				c.expectHistograms(histograms[len(histograms)/2:], series)
				c.expectFloatHistograms(floatHistograms[len(floatHistograms)/2:], series)
				if protoMsg == remoteapi.WriteV2MessageType && len(metadata) > 0 {
					c.expectMetadataForBatch(metadata, series, samples[len(samples)/2:], exemplars[len(exemplars)/2:], histograms[len(histograms)/2:], floatHistograms[len(floatHistograms)/2:])
				}
				qm.Append(samples[len(samples)/2:])
				qm.AppendExemplars(exemplars[len(exemplars)/2:])
				qm.AppendHistograms(histograms[len(histograms)/2:])
				qm.AppendFloatHistograms(floatHistograms[len(floatHistograms)/2:])
				c.waitForExpectedData(t, 30*time.Second)

				// Ensure the queue metrics match what was sent
				require.Equal(t, float64(len(samples)), client_testutil.ToFloat64(qm.metrics.samplesTotal))
				require.Equal(t, float64(len(exemplars)), client_testutil.ToFloat64(qm.metrics.exemplarsTotal))
				require.Equal(t, float64(len(histograms)+len(floatHistograms)), client_testutil.ToFloat64(qm.metrics.histogramsTotal))
				var durationMetric dto.Metric
				require.NoError(t, qm.metrics.sentBatchDuration.Write(&durationMetric))
				require.GreaterOrEqual(t, *durationMetric.Histogram.SampleSum, (10 * time.Millisecond).Seconds())
			})
		}
	}
}

func newTestClientAndQueueManager(t testing.TB, flushDeadline time.Duration, protoMsg remoteapi.WriteMessageType) (*TestWriteClient, *QueueManager) {
	c := NewTestWriteClient(protoMsg)
	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	return c, newTestQueueManager(t, cfg, mcfg, flushDeadline, c, protoMsg)
}

func newTestQueueManager(t testing.TB, cfg config.QueueConfig, mcfg config.MetadataConfig, deadline time.Duration, c WriteClient, protoMsg remoteapi.WriteMessageType) *QueueManager {
	dir := t.TempDir()
	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, deadline, newPool(), newHighestTimestampMetric(), nil, false, false, false, protoMsg, record.NewBuffersPool(), false, false)

	return m
}

func testDefaultQueueConfig() config.QueueConfig {
	cfg := config.DefaultQueueConfig
	// For faster unit tests we don't wait default 5 seconds.
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	return cfg
}

func TestMetadataDelivery(t *testing.T) {
	c, m := newTestClientAndQueueManager(t, defaultFlushDeadline, remoteapi.WriteV1MessageType)
	m.Start()
	defer m.Stop()

	metadata := []scrape.MetricMetadata{}
	numMetadata := 1532
	for i := range numMetadata {
		metadata = append(metadata, scrape.MetricMetadata{
			MetricFamily: "prometheus_remote_storage_sent_metadata_bytes_" + strconv.Itoa(i),
			Type:         model.MetricTypeCounter,
			Help:         "a nice help text",
			Unit:         "",
		})
	}

	m.AppendWatcherMetadata(context.Background(), metadata)

	require.Equal(t, 0.0, client_testutil.ToFloat64(m.metrics.failedMetadataTotal))
	require.Len(t, c.receivedMetadata, numMetadata)
	// One more write than the rounded quotient should be performed in order to get samples that didn't
	// fit into MaxSamplesPerSend.
	require.Equal(t, numMetadata/config.DefaultMetadataConfig.MaxSamplesPerSend+1, c.writesReceived)
	// Make sure the last samples were sent.
	require.Equal(t, c.receivedMetadata[metadata[len(metadata)-1].MetricFamily][0].MetricFamilyName, metadata[len(metadata)-1].MetricFamily)
}

func TestWALMetadataDelivery(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
	defer s.Close()

	cfg := config.DefaultQueueConfig
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	cfg.MaxShards = 1

	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = cfg
	writeConfig.ProtobufMessage = remoteapi.WriteV2MessageType

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}

	n := 3
	recs := testwal.GenerateRecords(recCase{Series: n, SamplesPerSeries: n})

	require.NoError(t, s.ApplyConfig(conf))
	hash, err := toHash(writeConfig)
	require.NoError(t, err)
	qm := s.rws.queues[hash]

	c := NewTestWriteClient(remoteapi.WriteV2MessageType)
	qm.SetClient(c)

	qm.StoreSeries(recs.Series, 0)
	qm.StoreMetadata(recs.Metadata)

	require.Len(t, qm.seriesLabels, n)
	require.Len(t, qm.seriesMetadata, n)

	c.expectSamples(recs.Samples, recs.Series)
	c.expectMetadataForBatch(recs.Metadata, recs.Series, recs.Samples, nil, nil, nil)
	qm.Append(recs.Samples)
	c.waitForExpectedData(t, 30*time.Second)

	// Metadata is cached state, not a queue item used for shard scaling.
	require.Equal(t, int64(len(recs.Samples)), qm.dataOut.newEvents.Load())
}

func TestSampleDeliveryTimeout(t *testing.T) {
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			recs := testwal.GenerateRecords(recCase{
				NoST:   protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
				Series: 10, SamplesPerSeries: 10,
			})
			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1

			c := NewTestWriteClient(protoMsg)
			m := newTestQueueManager(t, cfg, mcfg, defaultFlushDeadline, c, protoMsg)
			m.StoreSeries(recs.Series, 0)
			m.Start()
			defer m.Stop()

			// Send the samples twice, waiting for the samples in the meantime.
			c.expectSamples(recs.Samples, recs.Series)
			m.Append(recs.Samples)
			c.waitForExpectedData(t, 30*time.Second)

			c.expectSamples(recs.Samples, recs.Series)
			m.Append(recs.Samples)
			c.waitForExpectedData(t, 30*time.Second)
		})
	}
}

func TestSampleDeliveryOrder(t *testing.T) {
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			ts := 10
			n := config.DefaultQueueConfig.MaxSamplesPerSend * ts
			recs := testwal.GenerateRecords(recCase{
				NoST:   protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
				Series: n, SamplesPerSeries: 1,
			})

			c, m := newTestClientAndQueueManager(t, defaultFlushDeadline, protoMsg)
			c.expectSamples(recs.Samples, recs.Series)
			m.StoreSeries(recs.Series, 0)

			m.Start()
			defer m.Stop()
			// These should be received by the client.
			m.Append(recs.Samples)
			c.waitForExpectedData(t, 30*time.Second)
		})
	}
}

func TestShutdown(t *testing.T) {
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				deadline := 15 * time.Second
				c := NewTestBlockedWriteClient()

				cfg := config.DefaultQueueConfig
				mcfg := config.DefaultMetadataConfig

				m := newTestQueueManager(t, cfg, mcfg, deadline, c, protoMsg)
				// Send 2x batch size, so we know it will need at least two sends.
				n := 2 * config.DefaultQueueConfig.MaxSamplesPerSend
				recs := testwal.GenerateRecords(recCase{
					NoST:   protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
					Series: n / 1000, SamplesPerSeries: 1000,
				})
				m.StoreSeries(recs.Series, 0)
				m.Start()

				// Append blocks to guarantee delivery, so we do it in the background.
				go func() {
					m.Append(recs.Samples)
				}()
				synctest.Wait()

				// Test to ensure that Stop doesn't block.
				start := time.Now()
				m.Stop()
				// The samples will never be delivered, so duration should
				// be at least equal to deadline, otherwise the flush deadline
				// was not respected.
				require.Equal(t, time.Since(start), deadline)
			})
		})
	}
}

func TestSeriesReset(t *testing.T) {
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			c := NewTestBlockedWriteClient()
			deadline := 5 * time.Second
			numSegments := 4
			numSeries := 25

			cfg := config.DefaultQueueConfig
			mcfg := config.DefaultMetadataConfig
			m := newTestQueueManager(t, cfg, mcfg, deadline, c, protoMsg)
			for i := range numSegments {
				series := []record.RefSeries{}
				metadata := []record.RefMetadata{}
				for j := range numSeries {
					ref := chunks.HeadSeriesRef((i * 100) + j)
					series = append(series, record.RefSeries{Ref: ref, Labels: labels.FromStrings("a", "a")})
					metadata = append(metadata, record.RefMetadata{Ref: ref, Type: 1, Unit: "", Help: "test"})
				}
				m.StoreSeries(series, i)
				m.StoreMetadata(metadata)
			}
			require.Len(t, m.seriesLabels, numSegments*numSeries)
			// V2 stores metadata in seriesMetadata map for inline sending.
			// V1 sends metadata separately via MetadataWatcher, so seriesMetadata is not populated.
			if protoMsg == remoteapi.WriteV2MessageType {
				require.Len(t, m.seriesMetadata, numSegments*numSeries)
			}

			m.SeriesReset(2)
			require.Len(t, m.seriesLabels, numSegments*numSeries/2)
			// Verify metadata is also reset for V2
			if protoMsg == remoteapi.WriteV2MessageType {
				require.Len(t, m.seriesMetadata, numSegments*numSeries/2)
			}
		})
	}
}

func TestReshard(t *testing.T) {
	t.Parallel()

	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			size := 10 // Make bigger to find more races.
			nSeries := 6
			samplesPerSeries := config.DefaultQueueConfig.Capacity * size
			recs := testwal.GenerateRecords(recCase{
				NoST:   protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
				Series: nSeries, SamplesPerSeries: samplesPerSeries,
			})
			t.Logf("about to send %v samples", len(recs.Samples))

			cfg := config.DefaultQueueConfig
			cfg.MaxShards = 1

			c := NewTestWriteClient(protoMsg)
			m := newTestQueueManager(t, cfg, config.DefaultMetadataConfig, defaultFlushDeadline, c, protoMsg)
			c.expectSamples(recs.Samples, recs.Series)
			m.StoreSeries(recs.Series, 0)

			m.Start()
			defer m.Stop()

			go func() {
				for i := 0; i < len(recs.Samples); i += config.DefaultQueueConfig.Capacity {
					sent := m.Append(recs.Samples[i : i+config.DefaultQueueConfig.Capacity])
					require.True(t, sent, "samples not sent")
					time.Sleep(100 * time.Millisecond)
				}
			}()

			for i := 1; i < len(recs.Samples)/config.DefaultQueueConfig.Capacity; i++ {
				m.shards.stop()
				m.shards.start(i)
				time.Sleep(100 * time.Millisecond)
			}

			c.waitForExpectedData(t, 30*time.Second)
		})
	}
}

func TestReshardRaceWithStop(t *testing.T) {
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			c := NewTestWriteClient(protoMsg)
			var m *QueueManager
			h := sync.Mutex{}
			h.Lock()

			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			exitCh := make(chan struct{})
			go func() {
				for {
					m = newTestQueueManager(t, cfg, mcfg, defaultFlushDeadline, c, protoMsg)

					m.Start()
					h.Unlock()
					h.Lock()
					m.Stop()

					select {
					case exitCh <- struct{}{}:
						return
					default:
					}
				}
			}()

			for i := 1; i < 100; i++ {
				h.Lock()
				m.reshardChan <- i
				h.Unlock()
			}
			<-exitCh
		})
	}
}

// TestSetClientRace runs SetClient concurrently with sample and metadata sends.
// It exists to fail under the race detector if send paths read storeClient without clientMtx.
// MaxShards, Series, SamplesPerSeries, and the Append count loop are set to large values to
// increase the likelihood of a race condition.  These numbers were chosen on an Apple m5 Pro machine.
func TestSetClientRace(t *testing.T) {
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			c := NewTestWriteClient(protoMsg)
			cfg := testDefaultQueueConfig()
			// More than one shard keeps multiple send goroutines in flight.
			cfg.MaxShards = 4
			m := newTestQueueManager(t, cfg, config.DefaultMetadataConfig, defaultFlushDeadline, c, protoMsg)

			// Series and samples only need to be large enough to fill a batch and hit Store.
			recs := testwal.GenerateRecords(recCase{
				NoST:             protoMsg == remoteapi.WriteV1MessageType,
				Series:           8,
				SamplesPerSeries: 20,
			})
			m.StoreSeries(recs.Series, 0)
			m.StoreMetadata(recs.Metadata)
			m.Start()
			defer m.Stop()

			ctx, cancel := context.WithCancel(t.Context())
			g, ctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				for {
					select {
					case <-ctx.Done():
						return nil
					default:
						m.SetClient(c)
					}
				}
			})
			g.Go(func() error {
				defer cancel()
				// Repeated appends keep shards sending; the exact count is not load-bearing.
				for range 50 {
					m.Append(recs.Samples)
					if protoMsg == remoteapi.WriteV1MessageType {
						m.AppendWatcherMetadata(context.Background(), []scrape.MetricMetadata{{
							MetricFamily: "test_metric",
							Type:         model.MetricTypeCounter,
							Help:         "help",
						}})
					}
				}
				return nil
			})
			require.NoError(t, g.Wait())
		})
	}
}

func TestReshardPartialBatch(t *testing.T) {
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			recs := testwal.GenerateRecords(recCase{
				NoST:   protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
				Series: 1, SamplesPerSeries: 10,
			})

			c := NewTestBlockedWriteClient()

			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1
			batchSendDeadline := time.Millisecond
			flushDeadline := 10 * time.Millisecond
			cfg.BatchSendDeadline = model.Duration(batchSendDeadline)

			m := newTestQueueManager(t, cfg, mcfg, flushDeadline, c, protoMsg)
			m.StoreSeries(recs.Series, 0)

			m.Start()

			for range 100 {
				done := make(chan struct{})
				go func() {
					m.Append(recs.Samples)
					time.Sleep(batchSendDeadline)
					m.shards.stop()
					m.shards.start(1)
					done <- struct{}{}
				}()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("Deadlock between sending and stopping detected")
					pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
					t.FailNow()
				}
			}
			// We can only call stop if there was not a deadlock.
			m.Stop()
		})
	}
}

// TestQueueFilledDeadlock makes sure the code does not deadlock in the case
// where a large scrape (> capacity + max samples per send) is appended at the
// same time as a batch times out according to the batch send deadline.
func TestQueueFilledDeadlock(t *testing.T) {
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			recs := testwal.GenerateRecords(recCase{
				NoST:   protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
				Series: 50, SamplesPerSeries: 1,
			})

			c := NewNopWriteClient()

			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1
			cfg.MaxSamplesPerSend = 10
			cfg.Capacity = 20
			flushDeadline := time.Second
			batchSendDeadline := time.Millisecond
			cfg.BatchSendDeadline = model.Duration(batchSendDeadline)

			m := newTestQueueManager(t, cfg, mcfg, flushDeadline, c, protoMsg)
			m.StoreSeries(recs.Series, 0)
			m.Start()
			defer m.Stop()

			for range 100 {
				done := make(chan struct{})
				go func() {
					time.Sleep(batchSendDeadline)
					m.Append(recs.Samples)
					done <- struct{}{}
				}()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("Deadlock between sending and appending detected")
					pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
					t.FailNow()
				}
			}
		})
	}
}

func TestReleaseNoninternedString(t *testing.T) {
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			_, m := newTestClientAndQueueManager(t, defaultFlushDeadline, protoMsg)
			m.Start()
			defer m.Stop()
			for i := 1; i < 1000; i++ {
				m.StoreSeries([]record.RefSeries{
					{
						Ref:    chunks.HeadSeriesRef(i),
						Labels: labels.FromStrings("asdf", strconv.Itoa(i)),
					},
				}, 0)
				m.SeriesReset(1)
			}

			metric := client_testutil.ToFloat64(noReferenceReleases)
			require.Equal(t, 0.0, metric, "expected there to be no calls to release for strings that were not already interned: %d", int(metric))
		})
	}
}

func TestShouldReshard(t *testing.T) {
	type testcase struct {
		startingShards                           int
		samplesIn, samplesOut, lastSendTimestamp int64
		expectedToReshard                        bool
		sendDeadline                             model.Duration
	}
	cases := []testcase{
		{
			// resharding shouldn't take place if we haven't successfully sent
			// since the last shardUpdateDuration, even if the send deadline is very low
			startingShards:    10,
			samplesIn:         1000,
			samplesOut:        10,
			lastSendTimestamp: time.Now().Unix() - int64(shardUpdateDuration),
			expectedToReshard: false,
			sendDeadline:      model.Duration(100 * time.Millisecond),
		},
		{
			startingShards:    10,
			samplesIn:         1000,
			samplesOut:        10,
			lastSendTimestamp: time.Now().Unix(),
			expectedToReshard: true,
			sendDeadline:      config.DefaultQueueConfig.BatchSendDeadline,
		},
	}

	for _, c := range cases {
		_, m := newTestClientAndQueueManager(t, time.Duration(c.sendDeadline), remoteapi.WriteV1MessageType)
		m.numShards = c.startingShards
		m.dataIn.incr(c.samplesIn)
		m.dataOut.incr(c.samplesOut)
		m.lastSendTimestamp.Store(c.lastSendTimestamp)
		m.Start()

		desiredShards := m.calculateDesiredShards()
		shouldReshard := m.shouldReshard(desiredShards)

		m.Stop()

		require.Equal(t, c.expectedToReshard, shouldReshard)
	}
}

// TestDisableReshardOnRetry asserts that resharding should be disabled when a
// recoverable error is returned from remote_write.
func TestDisableReshardOnRetry(t *testing.T) {
	t.Parallel()
	onStoredContext, onStoreCalled := context.WithCancel(context.Background())
	defer onStoreCalled()

	var (
		recs = testwal.GenerateRecords(recCase{Series: 100, SamplesPerSeries: 100})

		cfg        = config.DefaultQueueConfig
		mcfg       = config.DefaultMetadataConfig
		retryAfter = time.Second

		metrics = newQueueManagerMetrics(nil, "", "")

		client = &MockWriteClient{
			StoreFunc: func(context.Context, []byte, int) (WriteResponseStats, error) {
				onStoreCalled()

				return WriteResponseStats{}, RecoverableError{
					error:      errors.New("fake error"),
					retryAfter: model.Duration(retryAfter),
				}
			},
			NameFunc:     func() string { return "mock" },
			EndpointFunc: func() string { return "http://fake:9090/api/v1/write" },
		}
	)

	m := NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, client, 0, newPool(), newHighestTimestampMetric(), nil, false, false, false, remoteapi.WriteV1MessageType, nil, false, false)
	m.StoreSeries(recs.Series, 0)

	// Attempt to samples while the manager is running. We immediately stop the
	// manager after the recoverable error is generated to prevent the manager
	// from resharding itself.
	m.Start()
	{
		m.Append(recs.Samples)

		select {
		case <-onStoredContext.Done():
		case <-time.After(time.Minute):
			require.FailNow(t, "timed out waiting for client to be sent metrics")
		}
	}
	m.Stop()

	require.Eventually(t, func() bool {
		// Force m.lastSendTimestamp to be current so the last send timestamp isn't
		// the reason resharding is disabled.
		m.lastSendTimestamp.Store(time.Now().Unix())
		return m.shouldReshard(m.numShards+1) == false
	}, time.Minute, 10*time.Millisecond, "shouldReshard was never disabled")

	// After 2x retryAfter, resharding should be enabled again.
	require.Eventually(t, func() bool {
		// Force m.lastSendTimestamp to be current so the last send timestamp isn't
		// the reason resharding is disabled.
		m.lastSendTimestamp.Store(time.Now().Unix())
		return m.shouldReshard(m.numShards+1) == true
	}, time.Minute, retryAfter, "shouldReshard should have been re-enabled")
}

func createProtoTimeseriesWithOld(numSamples, baseTs int64) []prompb.TimeSeries {
	samples := make([]prompb.TimeSeries, numSamples)
	// use a fixed rand source so tests are consistent
	r := rand.New(rand.NewSource(99))
	for j := range numSamples {
		name := fmt.Sprintf("test_metric_%d", j)

		samples[j] = prompb.TimeSeries{
			Labels: []prompb.Label{{Name: "__name__", Value: name}},
			Samples: []prompb.Sample{
				{
					Timestamp: baseTs + j,
					Value:     float64(j),
				},
			},
		}
		// 10% of the time use a ts that is too old
		if r.Intn(10) == 0 {
			samples[j].Samples[0].Timestamp = baseTs - 5
		}
	}
	return samples
}

func getSeriesIDFromRef(r record.RefSeries) string {
	return r.Labels.String()
}

// TestWriteClient represents write client which does not call remote storage,
// but instead re-implements fake WriteHandler for test purposes.
type TestWriteClient struct {
	receivedSamples         map[string][]writev2.Sample
	expectedSamples         map[string][]writev2.Sample
	receivedExemplars       map[string][]prompb.Exemplar
	expectedExemplars       map[string][]prompb.Exemplar
	receivedHistograms      map[string][]writev2.Histogram
	receivedFloatHistograms map[string][]writev2.Histogram
	expectedHistograms      map[string][]writev2.Histogram
	expectedFloatHistograms map[string][]writev2.Histogram
	receivedMetadata        map[string][]prompb.MetricMetadata
	expectedMetadata        map[string][]prompb.MetricMetadata
	writesReceived          int
	mtx                     sync.Mutex
	protoMsg                remoteapi.WriteMessageType
	injectedErrs            []error
	currErr                 int
	retry                   bool

	storeWait time.Duration
	// TODO(npazosmendez): maybe replaceable with injectedErrs?
	returnError error
}

// NewTestWriteClient creates a new testing write client.
func NewTestWriteClient(protoMsg remoteapi.WriteMessageType) *TestWriteClient {
	return &TestWriteClient{
		receivedSamples:         map[string][]writev2.Sample{},
		expectedSamples:         map[string][]writev2.Sample{},
		receivedExemplars:       map[string][]prompb.Exemplar{},
		expectedExemplars:       map[string][]prompb.Exemplar{},
		receivedHistograms:      map[string][]writev2.Histogram{},
		expectedHistograms:      map[string][]writev2.Histogram{},
		receivedFloatHistograms: map[string][]writev2.Histogram{},
		expectedFloatHistograms: map[string][]writev2.Histogram{},
		receivedMetadata:        map[string][]prompb.MetricMetadata{},
		expectedMetadata:        map[string][]prompb.MetricMetadata{},
		protoMsg:                protoMsg,
		storeWait:               0,
		returnError:             nil,
	}
}

func (c *TestWriteClient) injectErrors(injectedErrs []error) {
	c.injectedErrs = injectedErrs
	c.currErr = -1
	c.retry = false
}

// expectSamples injects samples that will be expected on waitForExpectedData.
func (c *TestWriteClient) expectSamples(ss []record.RefSample, series []record.RefSeries) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedSamples = map[string][]writev2.Sample{}
	c.receivedSamples = map[string][]writev2.Sample{}

	for _, s := range ss {
		tsID := getSeriesIDFromRef(series[s.Ref])
		c.expectedSamples[tsID] = append(c.expectedSamples[tsID], writev2.Sample{
			StartTimestamp: s.ST,
			Timestamp:      s.T,
			Value:          s.V,
		})
	}
}

func (c *TestWriteClient) expectExemplars(ss []record.RefExemplar, series []record.RefSeries) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedExemplars = map[string][]prompb.Exemplar{}
	c.receivedExemplars = map[string][]prompb.Exemplar{}

	for _, s := range ss {
		tsID := getSeriesIDFromRef(series[s.Ref])
		e := prompb.Exemplar{
			Labels:    prompb.FromLabels(s.Labels, nil),
			Timestamp: s.T,
			Value:     s.V,
		}
		c.expectedExemplars[tsID] = append(c.expectedExemplars[tsID], e)
	}
}

func (c *TestWriteClient) expectHistograms(hh []record.RefHistogramSample, series []record.RefSeries) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedHistograms = map[string][]writev2.Histogram{}
	c.receivedHistograms = map[string][]writev2.Histogram{}

	for _, h := range hh {
		tsID := getSeriesIDFromRef(series[h.Ref])
		c.expectedHistograms[tsID] = append(c.expectedHistograms[tsID], writev2.FromIntHistogram(h.ST, h.T, h.H))
	}
}

func (c *TestWriteClient) expectFloatHistograms(fhs []record.RefFloatHistogramSample, series []record.RefSeries) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedFloatHistograms = map[string][]writev2.Histogram{}
	c.receivedFloatHistograms = map[string][]writev2.Histogram{}

	for _, fh := range fhs {
		tsID := getSeriesIDFromRef(series[fh.Ref])
		c.expectedFloatHistograms[tsID] = append(c.expectedFloatHistograms[tsID], writev2.FromFloatHistogram(fh.ST, fh.T, fh.FH))
	}
}

func (c *TestWriteClient) expectMetadataForBatch(metadata []record.RefMetadata, series []record.RefSeries, samples []record.RefSample, exemplars []record.RefExemplar, histograms []record.RefHistogramSample, floatHistograms []record.RefFloatHistogramSample) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedMetadata = map[string][]prompb.MetricMetadata{}
	c.receivedMetadata = map[string][]prompb.MetricMetadata{}

	// Collect refs that have data in this batch.
	refsWithData := make(map[chunks.HeadSeriesRef]struct{})
	for _, s := range samples {
		refsWithData[s.Ref] = struct{}{}
	}
	for _, e := range exemplars {
		refsWithData[e.Ref] = struct{}{}
	}
	for _, h := range histograms {
		refsWithData[h.Ref] = struct{}{}
	}
	for _, fh := range floatHistograms {
		refsWithData[fh.Ref] = struct{}{}
	}

	// Only expect metadata for series that have data in this batch.
	for _, m := range metadata {
		if _, ok := refsWithData[m.Ref]; !ok {
			continue
		}
		tsID := getSeriesIDFromRef(series[m.Ref])
		c.expectedMetadata[tsID] = append(c.expectedMetadata[tsID], prompb.MetricMetadata{
			MetricFamilyName: tsID,
			Type:             prompb.FromMetadataType(record.ToMetricType(m.Type)),
			Help:             m.Help,
			Unit:             m.Unit,
		})
	}
}

func deepLen[M any](ms ...map[string][]M) int {
	l := 0
	for _, m := range ms {
		for _, v := range m {
			l += len(v)
		}
	}
	return l
}

func (c *TestWriteClient) waitForExpectedData(tb testing.TB, timeout time.Duration) {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := runutil.Retry(500*time.Millisecond, ctx.Done(), func() error {
		c.mtx.Lock()
		exp := deepLen(c.expectedSamples) + deepLen(c.expectedExemplars) + deepLen(c.expectedHistograms, c.expectedFloatHistograms) + len(c.expectedMetadata)
		got := deepLen(c.receivedSamples) + deepLen(c.receivedExemplars) + deepLen(c.receivedHistograms, c.receivedFloatHistograms) + func() int {
			if len(c.receivedMetadata) == 0 {
				return 0
			}
			return len(c.expectedMetadata) // Count unique series that have metadata.
		}()
		c.mtx.Unlock()

		if got < exp {
			return fmt.Errorf("expected %v samples/exemplars/histograms/floathistograms/metadata, got %v", exp, got)
		}
		return nil
	}); err != nil {
		tb.Error(err)
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	for ts, expectedSamples := range c.expectedSamples {
		require.Equal(tb, expectedSamples, c.receivedSamples[ts], ts)
	}
	for ts, expectedExemplar := range c.expectedExemplars {
		require.Equal(tb, expectedExemplar, c.receivedExemplars[ts], ts)
	}
	for ts, expectedHistogram := range c.expectedHistograms {
		require.Equal(tb, expectedHistogram, c.receivedHistograms[ts], ts)
	}
	for ts, expectedFloatHistogram := range c.expectedFloatHistograms {
		require.Equal(tb, expectedFloatHistogram, c.receivedFloatHistograms[ts], ts)
	}
	for ts, expectedMetadata := range c.expectedMetadata {
		require.NotEmpty(tb, c.receivedMetadata[ts], "No metadata received for series %s", ts)
		// For metadata, we only check that we got at least one entry with the right content
		// since v2 protocol sends metadata with each data point
		require.Equal(tb, expectedMetadata[0], c.receivedMetadata[ts][0], ts)
	}
}

func (c *TestWriteClient) SetStoreWait(w time.Duration) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.storeWait = w
}

func (c *TestWriteClient) SetReturnError(err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.returnError = err
}

func (c *TestWriteClient) Store(_ context.Context, req []byte, _ int) (WriteResponseStats, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.storeWait > 0 {
		time.Sleep(c.storeWait)
	}
	if c.returnError != nil {
		return WriteResponseStats{}, c.returnError
	}

	reqBuf, err := compression.Decode(compression.Snappy, req, nil)
	if err != nil {
		return WriteResponseStats{}, err
	}

	// Check if we've been told to inject err for this call.
	if len(c.injectedErrs) > 0 {
		c.currErr++
		if err = c.injectedErrs[c.currErr]; err != nil {
			return WriteResponseStats{}, err
		}
	}

	var (
		reqProto   *prompb.WriteRequest
		reqProtoV2 *writev2.Request
	)
	switch c.protoMsg {
	case remoteapi.WriteV1MessageType:
		reqProto = &prompb.WriteRequest{}
		err = proto.Unmarshal(reqBuf, reqProto)
	case remoteapi.WriteV2MessageType:
		// NOTE(bwplotka): v1 msg can be unmarshaled to v2 sometimes, without
		// errors.
		reqProtoV2 = &writev2.Request{}
		err = proto.Unmarshal(reqBuf, reqProtoV2)
		if err == nil {
			reqProto, err = v2RequestToWriteRequest(reqProtoV2)
		}
	}
	if err != nil {
		return WriteResponseStats{}, err
	}

	rs := WriteResponseStats{}
	b := labels.NewScratchBuilder(0)
	for i, ts := range reqProto.Timeseries {
		labels := ts.ToLabels(&b, nil)
		tsID := labels.String()
		for j, s := range ts.Samples {
			st := int64(0)
			if reqProtoV2 != nil {
				// TODO(bwplotka): Refactor queue manager TestWriteClient for tighter validation
				// and native support for new RW2 features. For now we inject STs in RW2 case to the existing test suite.
				st = reqProtoV2.Timeseries[i].Samples[j].StartTimestamp
			}
			c.receivedSamples[tsID] = append(c.receivedSamples[tsID], writev2.Sample{
				StartTimestamp: st,
				Timestamp:      s.Timestamp,
				Value:          s.Value,
			})
		}
		rs.Samples += len(ts.Samples)

		if len(ts.Exemplars) > 0 {
			c.receivedExemplars[tsID] = append(c.receivedExemplars[tsID], ts.Exemplars...)
		}
		rs.Exemplars += len(ts.Exemplars)

		for j, h := range ts.Histograms {
			// Mirror the float sample pattern (see Samples loop above): v1 has
			// no ST on the wire so we record 0; v2 carries StartTimestamp on
			// each histogram, which we pull from reqProtoV2 to preserve it.
			st := int64(0)
			if reqProtoV2 != nil {
				st = reqProtoV2.Timeseries[i].Histograms[j].StartTimestamp
			}
			if h.IsFloatHistogram() {
				c.receivedFloatHistograms[tsID] = append(c.receivedFloatHistograms[tsID], writev2.FromFloatHistogram(st, h.Timestamp, h.ToFloatHistogram()))
			} else {
				c.receivedHistograms[tsID] = append(c.receivedHistograms[tsID], writev2.FromIntHistogram(st, h.Timestamp, h.ToIntHistogram()))
			}
		}
		rs.Histograms += len(ts.Histograms)
	}
	for _, m := range reqProto.Metadata {
		c.receivedMetadata[m.MetricFamilyName] = append(c.receivedMetadata[m.MetricFamilyName], m)
	}

	c.writesReceived++
	return rs, nil
}

func (*TestWriteClient) Name() string {
	return "testwriteclient"
}

func (*TestWriteClient) Endpoint() string {
	return "http://test-remote.com/1234"
}

func v2RequestToWriteRequest(v2Req *writev2.Request) (*prompb.WriteRequest, error) {
	req := &prompb.WriteRequest{
		Timeseries: make([]prompb.TimeSeries, len(v2Req.Timeseries)),
		Metadata:   []prompb.MetricMetadata{},
	}
	b := labels.NewScratchBuilder(0)
	for i, rts := range v2Req.Timeseries {
		lbls, err := rts.ToLabels(&b, v2Req.Symbols)
		if err != nil {
			return nil, fmt.Errorf("failed to convert labels: %w", err)
		}
		lbls.Range(func(l labels.Label) {
			req.Timeseries[i].Labels = append(req.Timeseries[i].Labels, prompb.Label{
				Name:  l.Name,
				Value: l.Value,
			})
		})

		exemplars := make([]prompb.Exemplar, len(rts.Exemplars))
		for j, e := range rts.Exemplars {
			exemplars[j].Value = e.Value
			exemplars[j].Timestamp = e.Timestamp
			ex, err := e.ToExemplar(&b, v2Req.Symbols)
			if err != nil {
				return nil, fmt.Errorf("failed to convert exemplar: %w", err)
			}
			ex.Labels.Range(func(l labels.Label) {
				exemplars[j].Labels = append(exemplars[j].Labels, prompb.Label{
					Name:  l.Name,
					Value: l.Value,
				})
			})
		}
		req.Timeseries[i].Exemplars = exemplars

		req.Timeseries[i].Samples = make([]prompb.Sample, len(rts.Samples))
		for j, s := range rts.Samples {
			req.Timeseries[i].Samples[j].Timestamp = s.Timestamp
			req.Timeseries[i].Samples[j].Value = s.Value
		}

		req.Timeseries[i].Histograms = make([]prompb.Histogram, len(rts.Histograms))
		for j, h := range rts.Histograms {
			if h.IsFloatHistogram() {
				req.Timeseries[i].Histograms[j] = prompb.FromFloatHistogram(h.Timestamp, h.ToFloatHistogram())
				continue
			}
			req.Timeseries[i].Histograms[j] = prompb.FromIntHistogram(h.Timestamp, h.ToIntHistogram())
		}

		// Convert v2 metadata to v1 format.
		if rts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
			labels, err := rts.ToLabels(&b, v2Req.Symbols)
			if err != nil {
				return nil, fmt.Errorf("failed to convert metadata labels: %w", err)
			}
			metadata, err := rts.ToMetadata(v2Req.Symbols)
			if err != nil {
				return nil, fmt.Errorf("failed to convert metadata: %w", err)
			}

			metricFamilyName := labels.String()

			req.Metadata = append(req.Metadata, prompb.MetricMetadata{
				MetricFamilyName: metricFamilyName,
				Type:             prompb.FromMetadataType(metadata.Type),
				Help:             metadata.Help,
				Unit:             metadata.Unit,
			})
		}
	}
	return req, nil
}

// TestBlockingWriteClient is a queue_manager WriteClient which will block
// on any calls to Store(), until the request's Context is cancelled, at which
// point the `numCalls` property will contain a count of how many times Store()
// was called.
type TestBlockingWriteClient struct {
	numCalls atomic.Uint64
}

func NewTestBlockedWriteClient() *TestBlockingWriteClient {
	return &TestBlockingWriteClient{}
}

func (c *TestBlockingWriteClient) Store(ctx context.Context, _ []byte, _ int) (WriteResponseStats, error) {
	c.numCalls.Inc()
	<-ctx.Done()
	return WriteResponseStats{}, nil
}

func (c *TestBlockingWriteClient) NumCalls() uint64 {
	return c.numCalls.Load()
}

func (*TestBlockingWriteClient) Name() string {
	return "testblockingwriteclient"
}

func (*TestBlockingWriteClient) Endpoint() string {
	return "http://test-remote-blocking.com/1234"
}

// For benchmarking the send and not the receive side.
type NopWriteClient struct{}

func NewNopWriteClient() *NopWriteClient { return &NopWriteClient{} }
func (*NopWriteClient) Store(context.Context, []byte, int) (WriteResponseStats, error) {
	return WriteResponseStats{}, nil
}
func (*NopWriteClient) Name() string     { return "nopwriteclient" }
func (*NopWriteClient) Endpoint() string { return "http://test-remote.com/1234" }

type MockWriteClient struct {
	StoreFunc    func(context.Context, []byte, int) (WriteResponseStats, error)
	NameFunc     func() string
	EndpointFunc func() string
}

func (c *MockWriteClient) Store(ctx context.Context, bb []byte, n int) (WriteResponseStats, error) {
	return c.StoreFunc(ctx, bb, n)
}
func (c *MockWriteClient) Name() string     { return c.NameFunc() }
func (c *MockWriteClient) Endpoint() string { return c.EndpointFunc() }

// Extra labels to make a more realistic workload - taken from Kubernetes' embedded cAdvisor metrics.
var extraLabels []labels.Label = []labels.Label{
	{Name: "kubernetes_io_arch", Value: "amd64"},
	{Name: "kubernetes_io_instance_type", Value: "c3.somesize"},
	{Name: "kubernetes_io_os", Value: "linux"},
	{Name: "container_name", Value: "some-name"},
	{Name: "failure_domain_kubernetes_io_region", Value: "somewhere-1"},
	{Name: "failure_domain_kubernetes_io_zone", Value: "somewhere-1b"},
	{Name: "id", Value: "/kubepods/burstable/pod6e91c467-e4c5-11e7-ace3-0a97ed59c75e/a3c8498918bd6866349fed5a6f8c643b77c91836427fb6327913276ebc6bde28"},
	{Name: "image", Value: "registry/organisation/name@sha256:dca3d877a80008b45d71d7edc4fd2e44c0c8c8e7102ba5cbabec63a374d1d506"},
	{Name: "instance", Value: "ip-111-11-1-11.ec2.internal"},
	{Name: "job", Value: "kubernetes-cadvisor"},
	{Name: "kubernetes_io_hostname", Value: "ip-111-11-1-11"},
	{Name: "monitor", Value: "prod"},
	{Name: "name", Value: "k8s_some-name_some-other-name-5j8s8_kube-system_6e91c467-e4c5-11e7-ace3-0a97ed59c75e_0"},
	{Name: "namespace", Value: "kube-system"},
	{Name: "pod_name", Value: "some-other-name-5j8s8"},
}

// Recommended CLI invocation(s):
/*
	export bench=sampleSend && go test ./storage/remote/... \
		-run '^$' -bench '^BenchmarkSampleSend' \
		-benchtime 1s -count 6 -cpu 2 -timeout 999m -benchmem \
		| tee ${bench}.txt
*/
func BenchmarkSampleSend(b *testing.B) {
	// Send one sample per series, which is the typical remote_write case
	const numSamples = 1
	const numSeries = 10000

	recs := testwal.GenerateRecords(recCase{Series: numSeries, SamplesPerSeries: numSamples, ExtraLabels: extraLabels})

	c := NewNopWriteClient()

	cfg := testDefaultQueueConfig()
	mcfg := config.DefaultMetadataConfig
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	cfg.MinShards = 20
	cfg.MaxShards = 20

	// todo: test with new proto type(s)
	for _, format := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		b.Run(string(format), func(b *testing.B) {
			m := newTestQueueManager(b, cfg, mcfg, defaultFlushDeadline, c, format)
			m.StoreSeries(recs.Series, 0)

			// These should be received by the client.
			m.Start()
			defer m.Stop()

			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				m.Append(recs.Samples)
				m.UpdateSeriesSegment(recs.Series, i+1) // simulate what wlog.Watcher.garbageCollectSeries does
				m.SeriesReset(i + 1)
			}
			// Do not include shutdown
			b.StopTimer()
		})
	}
}

// BenchmarkNHCBRepeatedBounds converts samples whose bucket layout never changes, the common case.
func BenchmarkNHCBRepeatedBounds(b *testing.B) {
	for _, buckets := range []int{10, 100, 1000, 5000} {
		b.Run(strconv.Itoa(buckets), func(b *testing.B) {
			m, stop := newTestNHCBQueueManager(b, NewNopWriteClient(), nhcbQueueTestOptions{protoMsg: remoteapi.WriteV1MessageType})
			defer stop()
			m.StoreSeries([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings("__name__", "latency")}}, 0)
			h := &histogram.Histogram{
				Schema: histogram.CustomBucketsSchema, CustomValues: make([]float64, buckets),
				PositiveSpans:   []histogram.Span{{Length: uint32(buckets + 1)}},
				PositiveBuckets: make([]int64, buckets+1), Count: uint64(buckets + 1),
				Sum: float64((buckets+1)*(buckets+1)) / 2,
			}
			for i := range h.CustomValues {
				h.CustomValues[i] = float64(i + 1)
			}
			h.PositiveBuckets[0] = 1
			require.NoError(b, h.Validate())
			samples := []record.RefHistogramSample{{Ref: 1, T: 1, H: h}}
			require.True(b, m.AppendHistograms(samples))
			b.ReportAllocs()
			for b.Loop() {
				samples[0].T++
				if !m.AppendHistograms(samples) {
					b.Fatal("append stopped")
				}
			}
		})
	}
}

// BenchmarkNHCBSeriesMemory reports the conversion state retained per series when every series shares one layout.
func BenchmarkNHCBSeriesMemory(b *testing.B) {
	const seriesCount = 10000
	h := &histogram.Histogram{
		Schema:        histogram.CustomBucketsSchema,
		CustomValues:  []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25},
		PositiveSpans: []histogram.Span{{Length: 13}}, PositiveBuckets: make([]int64, 13), Count: 13, Sum: 42,
	}
	h.PositiveBuckets[0] = 1
	require.NoError(b, h.Validate())
	series := make([]record.RefSeries, 0, seriesCount)
	samples := make([]record.RefHistogramSample, 0, seriesCount)
	for i := range seriesCount {
		ref := chunks.HeadSeriesRef(i + 1)
		series = append(series, record.RefSeries{Ref: ref, Labels: labels.FromStrings("__name__", "latency", "series", strconv.Itoa(i))})
		samples = append(samples, record.RefHistogramSample{Ref: ref, T: 1, H: h})
	}
	liveHeap := func() int64 {
		runtime.GC()
		runtime.GC() // The second cycle also empties sync.Pool victim caches.
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return int64(ms.HeapAlloc)
	}
	var retained int64
	for b.Loop() {
		b.StopTimer()
		m, stop := newTestNHCBQueueManager(b, NewNopWriteClient(), nhcbQueueTestOptions{protoMsg: remoteapi.WriteV1MessageType})
		m.StoreSeries(series, 0)
		before := liveHeap()
		b.StartTimer()
		if !m.AppendHistograms(samples) {
			b.Fatal("append stopped")
		}
		b.StopTimer()
		// Flush the shards so that only conversion state remains.
		stop()
		retained = liveHeap() - before
		runtime.KeepAlive(m)
		b.StartTimer()
	}
	b.ReportMetric(float64(retained)/seriesCount, "retained-B/series")
}

// Check how long it takes to add N series, including external labels processing.
func BenchmarkStoreSeries(b *testing.B) {
	externalLabels := []labels.Label{
		{Name: "cluster", Value: "mycluster"},
		{Name: "replica", Value: "1"},
	}
	relabelConfigs := []*relabel.Config{{
		SourceLabels:         model.LabelNames{"namespace"},
		Separator:            ";",
		Regex:                relabel.MustNewRegexp("kube.*"),
		TargetLabel:          "job",
		Replacement:          "$1",
		Action:               relabel.Replace,
		NameValidationScheme: model.UTF8Validation,
	}}
	testCases := []struct {
		name           string
		externalLabels []labels.Label
		ts             []prompb.TimeSeries
		relabelConfigs []*relabel.Config
	}{
		{name: "plain"},
		{name: "externalLabels", externalLabels: externalLabels},
		{name: "relabel", relabelConfigs: relabelConfigs},
		{
			name:           "externalLabels+relabel",
			externalLabels: externalLabels,
			relabelConfigs: relabelConfigs,
		},
	}

	// numSeries chosen to be big enough that StoreSeries dominates creating a new queue manager.
	const numSeries = 1000
	recs := testwal.GenerateRecords(recCase{Series: numSeries, SamplesPerSeries: 0, ExtraLabels: extraLabels})

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				c := NewTestWriteClient(remoteapi.WriteV1MessageType)
				dir := b.TempDir()
				cfg := config.DefaultQueueConfig
				mcfg := config.DefaultMetadataConfig
				metrics := newQueueManagerMetrics(nil, "", "")

				m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false, false, remoteapi.WriteV1MessageType, record.NewBuffersPool(), false, false)
				m.externalLabels = tc.externalLabels
				m.relabelConfigs = tc.relabelConfigs

				m.StoreSeries(recs.Series, 0)
			}
		})
	}
}

func TestProcessExternalLabels(t *testing.T) {
	b := labels.NewBuilder(labels.EmptyLabels())
	for i, tc := range []struct {
		labels         labels.Labels
		externalLabels []labels.Label
		expected       labels.Labels
	}{
		// Test adding labels at the end.
		{
			labels:         labels.FromStrings("a", "b"),
			externalLabels: []labels.Label{{Name: "c", Value: "d"}},
			expected:       labels.FromStrings("a", "b", "c", "d"),
		},

		// Test adding labels at the beginning.
		{
			labels:         labels.FromStrings("c", "d"),
			externalLabels: []labels.Label{{Name: "a", Value: "b"}},
			expected:       labels.FromStrings("a", "b", "c", "d"),
		},

		// Test we don't override existing labels.
		{
			labels:         labels.FromStrings("a", "b"),
			externalLabels: []labels.Label{{Name: "a", Value: "c"}},
			expected:       labels.FromStrings("a", "b"),
		},

		// Test empty externalLabels.
		{
			labels:         labels.FromStrings("a", "b"),
			externalLabels: []labels.Label{},
			expected:       labels.FromStrings("a", "b"),
		},

		// Test empty labels.
		{
			labels:         labels.EmptyLabels(),
			externalLabels: []labels.Label{{Name: "a", Value: "b"}},
			expected:       labels.FromStrings("a", "b"),
		},

		// Test labels is longer than externalLabels.
		{
			labels:         labels.FromStrings("a", "b", "c", "d"),
			externalLabels: []labels.Label{{Name: "e", Value: "f"}},
			expected:       labels.FromStrings("a", "b", "c", "d", "e", "f"),
		},

		// Test externalLabels is longer than labels.
		{
			labels:         labels.FromStrings("c", "d"),
			externalLabels: []labels.Label{{Name: "a", Value: "b"}, {Name: "e", Value: "f"}},
			expected:       labels.FromStrings("a", "b", "c", "d", "e", "f"),
		},

		// Adding with and without clashing labels.
		{
			labels:         labels.FromStrings("a", "b", "c", "d"),
			externalLabels: []labels.Label{{Name: "a", Value: "xxx"}, {Name: "c", Value: "yyy"}, {Name: "e", Value: "f"}},
			expected:       labels.FromStrings("a", "b", "c", "d", "e", "f"),
		},
	} {
		b.Reset(tc.labels)
		processExternalLabels(b, tc.externalLabels)
		testutil.RequireEqual(t, tc.expected, b.Labels(), "test %d", i)
	}
}

func TestCalculateDesiredShards(t *testing.T) {
	cfg := config.DefaultQueueConfig
	_, m := newTestClientAndQueueManager(t, defaultFlushDeadline, remoteapi.WriteV1MessageType)
	samplesIn := m.dataIn

	// Need to start the queue manager so the proper metrics are initialized.
	// However we can stop it right away since we don't need to do any actual
	// processing.
	m.Start()
	m.Stop()

	inputRate := int64(50000)
	var pendingSamples int64

	// Two minute startup, no samples are sent.
	startedAt := time.Now().Add(-2 * time.Minute)

	// helper function for adding samples.
	addSamples := func(s int64, ts time.Duration) {
		pendingSamples += s
		samplesIn.incr(s)
		samplesIn.tick()

		m.highestRecvTimestamp.Set(float64(startedAt.Add(ts).Unix()))
	}

	// helper function for sending samples.
	sendSamples := func(s int64, ts time.Duration) {
		pendingSamples -= s
		m.dataOut.incr(s)
		m.dataOutDuration.incr(int64(m.numShards) * int64(shardUpdateDuration))

		// highest sent is how far back pending samples would be at our input rate.
		highestSent := startedAt.Add(ts - time.Duration(pendingSamples/inputRate)*time.Second)
		m.metrics.highestSentTimestamp.Set(float64(highestSent.Unix()))

		m.lastSendTimestamp.Store(time.Now().Unix())
	}

	ts := time.Duration(0)
	for ; ts < 120*time.Second; ts += shardUpdateDuration {
		addSamples(inputRate*int64(shardUpdateDuration/time.Second), ts)
		m.numShards = m.calculateDesiredShards()
		require.Equal(t, 1, m.numShards)
	}

	// Assume 100ms per request, or 10 requests per second per shard.
	// Shard calculation should never drop below barely keeping up.
	minShards := int(inputRate) / cfg.MaxSamplesPerSend / 10
	// This test should never go above 200 shards, that would be more resources than needed.
	maxShards := 200

	for ; ts < 15*time.Minute; ts += shardUpdateDuration {
		sin := inputRate * int64(shardUpdateDuration/time.Second)
		addSamples(sin, ts)

		sout := min(
			// You can't send samples that don't exist so cap at the number of pending samples.
			int64(m.numShards*cfg.MaxSamplesPerSend)*int64(shardUpdateDuration/(100*time.Millisecond)),
			pendingSamples,
		)
		sendSamples(sout, ts)

		t.Log("desiredShards", m.numShards, "pendingSamples", pendingSamples)
		m.numShards = m.calculateDesiredShards()
		require.GreaterOrEqual(t, m.numShards, minShards, "Shards are too low. desiredShards=%d, minShards=%d, t_seconds=%d", m.numShards, minShards, ts/time.Second)
		require.LessOrEqual(t, m.numShards, maxShards, "Shards are too high. desiredShards=%d, maxShards=%d, t_seconds=%d", m.numShards, maxShards, ts/time.Second)
	}
	require.Equal(t, int64(0), pendingSamples, "Remote write never caught up, there are still %d pending samples.", pendingSamples)
}

func TestCalculateDesiredShardsDetail(t *testing.T) {
	_, m := newTestClientAndQueueManager(t, defaultFlushDeadline, remoteapi.WriteV1MessageType)
	samplesIn := m.dataIn

	for _, tc := range []struct {
		name            string
		prevShards      int
		dataIn          int64 // Quantities normalised to seconds.
		dataOut         int64
		dataDropped     int64
		dataOutDuration float64
		backlog         float64
		expectedShards  int
	}{
		{
			name:           "nothing in or out 1",
			prevShards:     1,
			expectedShards: 1, // Shards stays the same.
		},
		{
			name:           "nothing in or out 10",
			prevShards:     10,
			expectedShards: 10, // Shards stays the same.
		},
		{
			name:            "steady throughput",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1,
			expectedShards:  1,
		},
		{
			name:            "sent timestamp in the future",
			prevShards:      40,
			dataIn:          1000,
			dataOut:         1000,
			dataOutDuration: 40,
			backlog:         -(5 * time.Hour).Seconds(),
			expectedShards:  40,
		},
		{
			name:            "scale down",
			prevShards:      10,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 5,
			expectedShards:  5,
		},
		{
			name:            "scale down constrained",
			prevShards:      7,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 5,
			expectedShards:  7,
		},
		{
			name:            "scale up",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 10,
			expectedShards:  10,
		},
		{
			name:            "scale up constrained",
			prevShards:      8,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 10,
			expectedShards:  8,
		},
		{
			name:            "backlogged 20s",
			prevShards:      2,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 2,
			backlog:         20,
			expectedShards:  4,
		},
		{
			name:            "backlogged 90s",
			prevShards:      4,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 4,
			backlog:         90,
			expectedShards:  22,
		},
		{
			name:            "backlog reduced",
			prevShards:      22,
			dataIn:          10,
			dataOut:         20,
			dataOutDuration: 4,
			backlog:         10,
			expectedShards:  3,
		},
		{
			name:            "backlog eliminated",
			prevShards:      3,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 2,
			backlog:         0,
			expectedShards:  2, // Shard back down.
		},
		{
			name:            "slight slowdown",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.2,
			expectedShards:  2, // 1.2 is rounded up to 2.
		},
		{
			name:            "bigger slowdown",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.4,
			expectedShards:  2,
		},
		{
			name:            "speed up",
			prevShards:      2,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.2,
			backlog:         0,
			expectedShards:  2, // No reaction - 1.2 is rounded up to 2.
		},
		{
			name:            "speed up more",
			prevShards:      2,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 0.9,
			backlog:         0,
			expectedShards:  1,
		},
		{
			name:            "marginal decision A",
			prevShards:      3,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 2.01,
			backlog:         0,
			expectedShards:  3, // 2.01 rounds up to 3.
		},
		{
			name:            "marginal decision B",
			prevShards:      3,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.99,
			backlog:         0,
			expectedShards:  2, // 1.99 rounds up to 2.
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.numShards = tc.prevShards
			forceEMWA(samplesIn, tc.dataIn*int64(shardUpdateDuration/time.Second))
			samplesIn.tick()
			forceEMWA(m.dataOut, tc.dataOut*int64(shardUpdateDuration/time.Second))
			forceEMWA(m.dataDropped, tc.dataDropped*int64(shardUpdateDuration/time.Second))
			forceEMWA(m.dataOutDuration, int64(tc.dataOutDuration*float64(shardUpdateDuration)))
			m.highestRecvTimestamp.value = tc.backlog // Not Set() because it can only increase value.

			require.Equal(t, tc.expectedShards, m.calculateDesiredShards())
		})
	}

	t.Run("NHCB conversion", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			histograms int
			samples    int
			asFloat    bool
		}{
			{name: "ordinary samples", samples: 10000},
			{name: "integer histograms", histograms: 1000},
			{name: "float histograms", histograms: 1000, asFloat: true},
			{name: "mixed samples", histograms: 500, samples: 5000},
		} {
			t.Run(tc.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					c := NewTestWriteClient(remoteapi.WriteV1MessageType)
					var inFlight, peakInFlight, calls atomic.Int64
					client := &MockWriteClient{
						NameFunc:     c.Name,
						EndpointFunc: c.Endpoint,
						StoreFunc: func(ctx context.Context, req []byte, attempt int) (WriteResponseStats, error) {
							active := inFlight.Inc()
							defer inFlight.Dec()
							for peak := peakInFlight.Load(); active > peak; peak = peakInFlight.Load() {
								if peakInFlight.CompareAndSwap(peak, active) {
									break
								}
							}
							calls.Inc()
							// Delay outside the recording client's mutex so all ten shards can send concurrently.
							timer := time.NewTimer(shardUpdateDuration)
							defer timer.Stop()
							select {
							case <-timer.C:
								return c.Store(ctx, req, attempt)
							case <-ctx.Done():
								return WriteResponseStats{}, ctx.Err()
							}
						},
					}
					rws := NewWriteStorage(nil, nil, t.TempDir(), defaultFlushDeadline, nil, false)
					defer func() { require.NoError(t, rws.Close()) }()
					m, stop := newTestNHCBQueueManager(t, client, nhcbQueueTestOptions{
						protoMsg:             remoteapi.WriteV1MessageType,
						numShards:            10,
						samplesIn:            rws.samplesIn,
						highestRecvTimestamp: rws.highestTimestamp,
					})
					defer stop()

					h := &histogram.Histogram{
						Schema:          histogram.CustomBucketsSchema,
						CustomValues:    []float64{1, 2, 3, 4, 5, 6, 7},
						PositiveSpans:   []histogram.Span{{Length: 8}},
						PositiveBuckets: []int64{1, 0, 0, 0, 0, 0, 0, 0},
						Count:           8,
						Sum:             30,
					}
					require.NoError(t, h.Validate())
					fh := h.ToFloat(nil)
					app := rws.AppenderV2(t.Context())
					var (
						series          []record.RefSeries
						histograms      []record.RefHistogramSample
						floatHistograms []record.RefFloatHistogramSample
						samples         []record.RefSample
					)
					const ts = int64(1000)
					for i := range tc.histograms + tc.samples {
						ref := chunks.HeadSeriesRef(i + 1)
						name := "requests_total"
						var intH *histogram.Histogram
						var floatH *histogram.FloatHistogram
						if i < tc.histograms {
							name = "latency"
							if tc.asFloat {
								floatH = fh
								floatHistograms = append(floatHistograms, record.RefFloatHistogramSample{Ref: ref, T: ts, FH: fh})
							} else {
								intH = h
								histograms = append(histograms, record.RefHistogramSample{Ref: ref, T: ts, H: h})
							}
						} else {
							samples = append(samples, record.RefSample{Ref: ref, T: ts, V: 1})
						}
						lbls := labels.FromStrings("__name__", name, "series", strconv.Itoa(i))
						series = append(series, record.RefSeries{Ref: ref, Labels: lbls})
						_, err := app.Append(storage.SeriesRef(ref), lbls, 0, ts, 1, intH, floatH, storage.AOptions{})
						require.NoError(t, err)
					}
					require.NoError(t, app.Commit())
					m.StoreSeries(series, 0)
					require.True(t, m.AppendHistograms(histograms))
					require.True(t, m.AppendFloatHistograms(floatHistograms))
					require.True(t, m.Append(samples))

					synctest.Wait()
					require.Equal(t, int64(10), inFlight.Load())
					time.Sleep(shardUpdateDuration)
					synctest.Wait()
					require.Zero(t, inFlight.Load())
					require.Equal(t, int64(10), peakInFlight.Load())
					require.Equal(t, int64(10), calls.Load())
					require.Equal(t, 10000, deepLen(c.receivedSamples))
					require.Zero(t, client_testutil.ToFloat64(m.metrics.failedSamplesTotal))
					require.Zero(t, client_testutil.ToFloat64(m.metrics.pendingSamples))
					require.Equal(t, m.highestRecvTimestamp.Get(), m.metrics.highestSentTimestamp.Get())
					require.Positive(t, m.dataIn.rate())
					// Each workload filled ten batches and kept ten shards busy for the whole interval.
					require.Equal(t, 10, m.calculateDesiredShards(), "NHCB expansion must not reduce the estimated shard demand")
				})
			})
		}
	})
}

func forceEMWA(r *ewmaRate, rate int64) {
	r.init = false
	r.newEvents.Store(rate)
}

func TestQueueManagerMetrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	metrics := newQueueManagerMetrics(reg, "name", "http://localhost:1234")

	// Make sure metrics pass linting.
	problems, err := client_testutil.GatherAndLint(reg)
	require.NoError(t, err)
	require.Empty(t, problems, "Metric linting problems detected: %v", problems)

	// Make sure all metrics were unregistered. A failure here means you need
	// unregister a metric in `queueManagerMetrics.unregister()`.
	metrics.unregister()
	err = client_testutil.GatherAndCompare(reg, strings.NewReader(""))
	require.NoError(t, err)
}

func TestQueue_FlushAndShutdownDoesNotDeadlock(t *testing.T) {
	capacity := 100
	batchSize := 10
	queue := newQueue(batchSize, capacity)
	for i := 0; i < capacity+batchSize; i++ {
		queue.Append(timeSeries{})
	}

	done := make(chan struct{})
	go queue.FlushAndShutdown(done)
	go func() {
		// Give enough time for FlushAndShutdown to acquire the lock. queue.Batch()
		// should not block forever even if the lock is acquired.
		time.Sleep(10 * time.Millisecond)
		queue.Batch()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Deadlock in FlushAndShutdown detected")
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		t.FailNow()
	}
}

func createDummyTimeSeries(instances int) []timeSeries {
	metrics := []labels.Labels{
		labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0"),
		labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.25"),
		labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.5"),
		labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.75"),
		labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1"),
		labels.FromStrings("__name__", "go_gc_duration_seconds_sum"),
		labels.FromStrings("__name__", "go_gc_duration_seconds_count"),
		labels.FromStrings("__name__", "go_memstats_alloc_bytes_total"),
		labels.FromStrings("__name__", "go_memstats_frees_total"),
		labels.FromStrings("__name__", "go_memstats_lookups_total"),
		labels.FromStrings("__name__", "go_memstats_mallocs_total"),
		labels.FromStrings("__name__", "go_goroutines"),
		labels.FromStrings("__name__", "go_info", "version", "go1.19.3"),
		labels.FromStrings("__name__", "go_memstats_alloc_bytes"),
		labels.FromStrings("__name__", "go_memstats_buck_hash_sys_bytes"),
		labels.FromStrings("__name__", "go_memstats_gc_sys_bytes"),
		labels.FromStrings("__name__", "go_memstats_heap_alloc_bytes"),
		labels.FromStrings("__name__", "go_memstats_heap_idle_bytes"),
		labels.FromStrings("__name__", "go_memstats_heap_inuse_bytes"),
		labels.FromStrings("__name__", "go_memstats_heap_objects"),
		labels.FromStrings("__name__", "go_memstats_heap_released_bytes"),
		labels.FromStrings("__name__", "go_memstats_heap_sys_bytes"),
		labels.FromStrings("__name__", "go_memstats_last_gc_time_seconds"),
		labels.FromStrings("__name__", "go_memstats_mcache_inuse_bytes"),
		labels.FromStrings("__name__", "go_memstats_mcache_sys_bytes"),
		labels.FromStrings("__name__", "go_memstats_mspan_inuse_bytes"),
		labels.FromStrings("__name__", "go_memstats_mspan_sys_bytes"),
		labels.FromStrings("__name__", "go_memstats_next_gc_bytes"),
		labels.FromStrings("__name__", "go_memstats_other_sys_bytes"),
		labels.FromStrings("__name__", "go_memstats_stack_inuse_bytes"),
		labels.FromStrings("__name__", "go_memstats_stack_sys_bytes"),
		labels.FromStrings("__name__", "go_memstats_sys_bytes"),
		labels.FromStrings("__name__", "go_threads"),
	}

	commonLabels := labels.FromStrings(
		"cluster", "some-cluster-0",
		"container", "prometheus",
		"job", "some-namespace/prometheus",
		"namespace", "some-namespace",
	)

	var result []timeSeries
	r := rand.New(rand.NewSource(0))
	for i := range instances {
		b := labels.NewBuilder(commonLabels)
		b.Set("pod", "prometheus-"+strconv.Itoa(i))
		for _, lbls := range metrics {
			lbls.Range(func(l labels.Label) {
				b.Set(l.Name, l.Value)
			})
			result = append(result, timeSeries{
				seriesLabels: b.Labels(),
				value:        r.Float64(),
			})
		}
	}
	return result
}

// Recommended CLI invocation(s):
/*
	export bench=buildWriteRequest && go test ./storage/remote/... \
		-run '^$' -bench '^BenchmarkBuildWriteRequest' \
		-benchtime 1s -count 6 -cpu 2 -timeout 999m -benchmem \
		| tee ${bench}.txt
*/
func BenchmarkBuildWriteRequest(b *testing.B) {
	noopLogger := promslog.NewNopLogger()
	bench := func(b *testing.B, batch []timeSeries) {
		cEnc := compression.NewSyncEncodeBuffer()
		seriesBuff := make([]prompb.TimeSeries, len(batch))
		for i := range seriesBuff {
			seriesBuff[i].Samples = []prompb.Sample{{}}
			seriesBuff[i].Exemplars = []prompb.Exemplar{{}}
		}
		pBuf := proto.NewBuffer(nil)

		totalSize := 0
		for b.Loop() {
			populateTimeSeries(batch, seriesBuff, true, true)
			req, _, _, err := buildWriteRequest(noopLogger, seriesBuff, nil, pBuf, nil, cEnc, compression.Snappy)
			if err != nil {
				b.Fatal(err)
			}
			totalSize += len(req)
		}
		b.ReportMetric(float64(totalSize)/float64(b.N), "compressedSize/op")
	}

	twoBatch := createDummyTimeSeries(2)
	tenBatch := createDummyTimeSeries(10)
	hundredBatch := createDummyTimeSeries(100)

	b.Run("2 instances", func(b *testing.B) {
		bench(b, twoBatch)
	})

	b.Run("10 instances", func(b *testing.B) {
		bench(b, tenBatch)
	})

	b.Run("1k instances", func(b *testing.B) {
		bench(b, hundredBatch)
	})
}

// Recommended CLI invocation(s):
/*
	export bench=buildV2WriteRequest && go test ./storage/remote/... \
		-run '^$' -bench '^BenchmarkBuildV2WriteRequest' \
		-benchtime 1s -count 6 -cpu 2 -timeout 999m -benchmem \
		| tee ${bench}.txt
*/
func BenchmarkBuildV2WriteRequest(b *testing.B) {
	noopLogger := promslog.NewNopLogger()
	bench := func(b *testing.B, batch []timeSeries) {
		symbolTable := writev2.NewSymbolTable()
		cEnc := compression.NewSyncEncodeBuffer()
		seriesBuff := make([]writev2.TimeSeries, len(batch))
		for i := range seriesBuff {
			seriesBuff[i].Samples = []writev2.Sample{{}}
			seriesBuff[i].Exemplars = []writev2.Exemplar{{}}
		}
		pBuf := []byte{}

		totalSize := 0
		for b.Loop() {
			populateV2TimeSeries(&symbolTable, batch, seriesBuff, true, true, false)
			req, _, _, _, err := buildV2WriteRequest(noopLogger, seriesBuff, symbolTable.Symbols(), &pBuf, nil, cEnc, "snappy")
			if err != nil {
				b.Fatal(err)
			}
			totalSize += len(req)
		}
		b.ReportMetric(float64(totalSize)/float64(b.N), "compressedSize/op")
	}

	twoBatch := createDummyTimeSeries(2)
	tenBatch := createDummyTimeSeries(10)
	hundredBatch := createDummyTimeSeries(100)

	b.Run("2 instances", func(b *testing.B) {
		bench(b, twoBatch)
	})

	b.Run("10 instances", func(b *testing.B) {
		bench(b, tenBatch)
	})

	b.Run("1k instances", func(b *testing.B) {
		bench(b, hundredBatch)
	})
}

func TestDropOldTimeSeries(t *testing.T) {
	t.Parallel()
	// Test both v1 and v2 remote write protocols
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			size := 10
			nSeries := 6
			nSamples := config.DefaultQueueConfig.Capacity * size
			noST := protoMsg == remoteapi.WriteV1MessageType // RW1
			pastRecs := testwal.GenerateRecords(recCase{
				NoST:             noST,
				Series:           nSeries,
				SamplesPerSeries: (nSamples / nSeries) / 2, // Half data is past.
				TsFn: func(_, j int) int64 {
					past := timestamp.FromTime(time.Now().Add(-5 * time.Minute))
					return past + int64(j)
				},
			})
			newRecs := testwal.GenerateRecords(recCase{
				NoST:             noST,
				Series:           nSeries,
				SamplesPerSeries: (nSamples / nSeries) / 2, // Half data is past.
				TsFn: func(_, j int) int64 {
					return time.Now().UnixMilli() + int64(j)
				},
			})

			series := pastRecs.Series // Series is the same for both old and new.
			newSamples := newRecs.Samples
			samples := append(pastRecs.Samples, newRecs.Samples...)

			c := NewTestWriteClient(protoMsg)
			c.expectSamples(newSamples, series)

			cfg := config.DefaultQueueConfig
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1
			cfg.SampleAgeLimit = model.Duration(60 * time.Second)
			m := newTestQueueManager(t, cfg, mcfg, defaultFlushDeadline, c, protoMsg)
			m.StoreSeries(series, 0)

			m.Start()
			defer m.Stop()

			m.Append(samples)
			c.waitForExpectedData(t, 30*time.Second)
		})
	}
}

func TestIsSampleOld(t *testing.T) {
	currentTime := time.Now()
	require.True(t, isSampleOld(currentTime, 60*time.Second, timestamp.FromTime(currentTime.Add(-61*time.Second))))
	require.False(t, isSampleOld(currentTime, 60*time.Second, timestamp.FromTime(currentTime.Add(-59*time.Second))))
}

// Simulates scenario in which remote write endpoint is down and a subset of samples is dropped due to age limit while backoffing.
func TestSendSamplesWithBackoffWithSampleAgeLimit(t *testing.T) {
	const (
		maxSamplesPerSend = 10
		maxLabels         = 9
	)

	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			sampleAgeLimit := time.Second * 2

			cfg := config.DefaultQueueConfig
			cfg.MaxShards = 1
			cfg.SampleAgeLimit = model.Duration(sampleAgeLimit)
			// Set the batch send deadline to 5 minutes to effectively disable it.
			cfg.BatchSendDeadline = model.Duration(time.Minute * 5)
			cfg.Capacity = 10 * maxSamplesPerSend // more than the amount of data we append in the test
			cfg.MaxBackoff = model.Duration(time.Millisecond * 100)
			cfg.MinBackoff = model.Duration(time.Millisecond * 100)
			cfg.MaxSamplesPerSend = maxSamplesPerSend
			metadataCfg := config.DefaultMetadataConfig
			metadataCfg.Send = true
			metadataCfg.SendInterval = model.Duration(time.Second * 60)
			metadataCfg.MaxSamplesPerSend = maxSamplesPerSend
			c := NewTestWriteClient(protoMsg)
			m := newTestQueueManager(t, cfg, metadataCfg, time.Second, c, protoMsg)

			m.Start()

			batchID := 0
			appendData := func(numberOfSeries int, timeAdd time.Duration, shouldBeDropped bool) {
				t.Log(">>>>  Appending series ", numberOfSeries, " as batch ID ", batchID, " with timeAdd ", timeAdd, " and should be dropped ", shouldBeDropped)

				// Use a fixed rand source so tests are consistent.
				r := rand.New(rand.NewSource(99))

				recs := testwal.GenerateRecords(recCase{
					NoST:             protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
					Series:           numberOfSeries,
					SamplesPerSeries: 1,
					TsFn: func(_, _ int) int64 {
						return time.Now().Add(timeAdd).UnixMilli()
					},
					LabelsFn: func(lb *labels.ScratchBuilder, i int) labels.Labels {
						lb.Reset()
						labelsCount := r.Intn(maxLabels)
						lb.Add("__name__", "batch_"+strconv.Itoa(batchID)+"_id_"+strconv.Itoa(i))
						for j := 1; j < labelsCount+1; j++ {
							// same for both name and value
							label := "batch_" + strconv.Itoa(batchID) + "_label_" + strconv.Itoa(j)
							lb.Add(label, label)
						}
						return lb.Labels()
					},
				})

				m.StoreSeries(recs.Series, batchID)
				sent := m.Append(recs.Samples)
				require.True(t, sent, "samples not sent")
				if !shouldBeDropped {
					for _, s := range recs.Samples {
						tsID := getSeriesIDFromRef(recs.Series[s.Ref])
						c.expectedSamples[tsID] = append(c.expectedSamples[tsID], writev2.Sample{
							StartTimestamp: s.ST,
							Timestamp:      s.T,
							Value:          s.V,
						})
					}
				}
				batchID++
			}
			timeShift := -time.Millisecond * 5

			// Inject RW error.
			c.SetReturnError(RecoverableError{context.DeadlineExceeded, defaultBackoff})
			// Send current samples in various intervals.
			appendData(maxSamplesPerSend/2, timeShift, true)
			time.Sleep(sampleAgeLimit)
			appendData(maxSamplesPerSend/2, timeShift, true)
			time.Sleep(sampleAgeLimit / 10)
			appendData(maxSamplesPerSend/2, timeShift, true)
			time.Sleep(2 * sampleAgeLimit)
			// Eventually all the above data must be ignored as 2x sampleAgeLimit passed.

			// Now send, quickly re-enable RW target and send another batch.
			// We should expect all the data from those two below batches.
			appendData(2*maxSamplesPerSend, timeShift, false)
			time.Sleep(sampleAgeLimit / 2)
			c.SetReturnError(nil)
			appendData(5, timeShift, false)
			m.Stop()

			require.Equal(t, c.expectedSamples, c.receivedSamples)
		})
	}
}

func filterTsLimit(limit int64, ts prompb.TimeSeries) bool {
	return limit > ts.Samples[0].Timestamp
}

func TestBuildTimeSeries(t *testing.T) {
	testCases := []struct {
		name           string
		ts             []prompb.TimeSeries
		filter         func(ts prompb.TimeSeries) bool
		lowestTs       int64
		highestTs      int64
		droppedSamples int
		responseLen    int
	}{
		{
			name: "No filter applied",
			ts: []prompb.TimeSeries{
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567890,
							Value:     1.23,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567891,
							Value:     2.34,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567892,
							Value:     3.34,
						},
					},
				},
			},
			filter:      nil,
			responseLen: 3,
			lowestTs:    1234567890,
			highestTs:   1234567892,
		},
		{
			name: "Filter applied, samples in order",
			ts: []prompb.TimeSeries{
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567890,
							Value:     1.23,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567891,
							Value:     2.34,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567892,
							Value:     3.45,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567893,
							Value:     3.45,
						},
					},
				},
			},
			filter:         func(ts prompb.TimeSeries) bool { return filterTsLimit(1234567892, ts) },
			responseLen:    2,
			lowestTs:       1234567892,
			highestTs:      1234567893,
			droppedSamples: 2,
		},
		{
			name: "Filter applied, samples out of order",
			ts: []prompb.TimeSeries{
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567892,
							Value:     3.45,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567890,
							Value:     1.23,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567893,
							Value:     3.45,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567891,
							Value:     2.34,
						},
					},
				},
			},
			filter:         func(ts prompb.TimeSeries) bool { return filterTsLimit(1234567892, ts) },
			responseLen:    2,
			lowestTs:       1234567892,
			highestTs:      1234567893,
			droppedSamples: 2,
		},
		{
			name: "Filter applied, samples not consecutive",
			ts: []prompb.TimeSeries{
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567890,
							Value:     1.23,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567892,
							Value:     3.45,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567895,
							Value:     6.78,
						},
					},
				},
				{
					Samples: []prompb.Sample{
						{
							Timestamp: 1234567897,
							Value:     6.78,
						},
					},
				},
			},
			filter:         func(ts prompb.TimeSeries) bool { return filterTsLimit(1234567895, ts) },
			responseLen:    2,
			lowestTs:       1234567895,
			highestTs:      1234567897,
			droppedSamples: 2,
		},
	}

	// Run the test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, stats := buildTimeSeries(tc.ts, tc.filter)
			require.NotNil(t, result)
			require.Len(t, result, tc.responseLen)
			require.Equal(t, tc.highestTs, stats.highest)
			require.Equal(t, tc.lowestTs, stats.lowest)
			require.Equal(t, tc.droppedSamples, stats.droppedSamples)
		})
	}
}

func BenchmarkBuildTimeSeries(b *testing.B) {
	// Send one sample per series, which is the typical remote_write case
	const numSamples = 10000
	filter := func(ts prompb.TimeSeries) bool { return filterTsLimit(99, ts) }
	originalSamples := createProtoTimeseriesWithOld(numSamples, 100)

	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		samples := make([]prompb.TimeSeries, len(originalSamples))
		copy(samples, originalSamples)
		b.StartTimer()
		result, _ := buildTimeSeries(samples, filter)
		require.NotNil(b, result)
	}
}

func TestPopulateV2TimeSeries_UnexpectedMetadata(t *testing.T) {
	symbolTable := writev2.NewSymbolTable()
	pendingData := make([]writev2.TimeSeries, 4)

	batch := []timeSeries{
		{sType: tSample, seriesLabels: labels.FromStrings("__name__", "metric1")},
		{sType: tMetadata, seriesLabels: labels.FromStrings("__name__", "metric2")},
		{sType: tSample, seriesLabels: labels.FromStrings("__name__", "metric3")},
		{sType: tMetadata, seriesLabels: labels.FromStrings("__name__", "metric4")},
	}

	nSamples, nExemplars, nHistograms, nMetadata, nUnexpected := populateV2TimeSeries(
		&symbolTable, batch, pendingData, false, false, false,
	)

	require.Equal(t, 2, nSamples, "Should count 2 samples")
	require.Equal(t, 0, nExemplars, "Should count 0 exemplars")
	require.Equal(t, 0, nHistograms, "Should count 0 histograms")
	require.Equal(t, 0, nMetadata, "Should count 0 processed metadata")
	require.Equal(t, 2, nUnexpected, "Should count 2 unexpected metadata")
}

func TestPopulateV2TimeSeries_TypeAndUnitLabels(t *testing.T) {
	symbolTable := writev2.NewSymbolTable()

	testCases := []struct {
		name         string
		typeLabel    string
		unitLabel    string
		expectedType writev2.Metadata_MetricType
		description  string
	}{
		{
			name:         "counter_with_unit",
			typeLabel:    "counter",
			unitLabel:    "operations",
			expectedType: writev2.Metadata_METRIC_TYPE_COUNTER,
			description:  "Counter metric with operations unit",
		},
		{
			name:         "gauge_with_bytes",
			typeLabel:    "gauge",
			unitLabel:    "bytes",
			expectedType: writev2.Metadata_METRIC_TYPE_GAUGE,
			description:  "Gauge metric with bytes unit",
		},
		{
			name:         "histogram_with_seconds",
			typeLabel:    "histogram",
			unitLabel:    "seconds",
			expectedType: writev2.Metadata_METRIC_TYPE_HISTOGRAM,
			description:  "Histogram metric with seconds unit",
		},
		{
			name:         "summary_with_ratio",
			typeLabel:    "summary",
			unitLabel:    "ratio",
			expectedType: writev2.Metadata_METRIC_TYPE_SUMMARY,
			description:  "Summary metric with ratio unit",
		},
		{
			name:         "info_no_unit",
			typeLabel:    "info",
			unitLabel:    "",
			expectedType: writev2.Metadata_METRIC_TYPE_INFO,
			description:  "Info metric without unit",
		},
		{
			name:         "stateset_no_unit",
			typeLabel:    "stateset",
			unitLabel:    "",
			expectedType: writev2.Metadata_METRIC_TYPE_STATESET,
			description:  "Stateset metric without unit",
		},
		{
			name:         "unknown_type",
			typeLabel:    "unknown_type",
			unitLabel:    "meters",
			expectedType: writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
			description:  "Unknown type defaults to unspecified",
		},
		{
			name:         "empty_type_with_unit",
			typeLabel:    "",
			unitLabel:    "watts",
			expectedType: writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
			description:  "Empty type with unit",
		},
		{
			name:         "type_no_unit",
			typeLabel:    "gauge",
			unitLabel:    "",
			expectedType: writev2.Metadata_METRIC_TYPE_GAUGE,
			description:  "Type without unit",
		},
		{
			name:         "no_type_no_unit",
			typeLabel:    "",
			unitLabel:    "",
			expectedType: writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
			description:  "No type and no unit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			batch := make([]timeSeries, 1)
			builder := labels.NewScratchBuilder(2)
			metadata := schema.Metadata{
				Name: "test_metric_" + tc.name,
				Type: model.MetricType(tc.typeLabel),
				Unit: tc.unitLabel,
			}

			metadata.AddToLabels(&builder)

			batch[0] = timeSeries{
				seriesLabels: builder.Labels(),
				value:        123.45,
				timestamp:    time.Now().UnixMilli(),
				sType:        tSample,
			}

			pendingData := make([]writev2.TimeSeries, 1)

			symbolTable.Reset()
			nSamples, nExemplars, nHistograms, _, _ := populateV2TimeSeries(
				&symbolTable,
				batch,
				pendingData,
				false, // sendExemplars
				false, // sendNativeHistograms
				true,  // enableTypeAndUnitLabels
			)

			require.Equal(t, 1, nSamples, "Should have 1 sample")
			require.Equal(t, 0, nExemplars, "Should have 0 exemplars")
			require.Equal(t, 0, nHistograms, "Should have 0 histograms")

			require.Equal(t, tc.expectedType, pendingData[0].Metadata.Type,
				"Type should match expected for %s", tc.description)

			unitRef := pendingData[0].Metadata.UnitRef

			symbols := symbolTable.Symbols()
			require.Equal(t, tc.unitLabel, symbols[unitRef], "Unit should match")
		})
	}
}

// TestPopulateV2TimeSeries_MetadataAndTypeAndUnit verifies that type and unit labels are properly
// extracted from labels even when d.metadata is not nil (agent mode scenario).
// Regression test for https://github.com/prometheus/prometheus/issues/17381.
func TestPopulateV2TimeSeries_MetadataAndTypeAndUnit(t *testing.T) {
	symbolTable := writev2.NewSymbolTable()

	testCases := []struct {
		name              string
		typeLabel         string
		unitLabel         string
		metadata          *metadata.Metadata
		expectedType      writev2.Metadata_MetricType
		expectedUnit      string
		enableTypeAndUnit bool
	}{
		{
			name:              "type_and_unit_no_meta",
			typeLabel:         "gauge",
			unitLabel:         "bytes",
			metadata:          nil,
			expectedType:      writev2.Metadata_METRIC_TYPE_GAUGE,
			expectedUnit:      "bytes",
			enableTypeAndUnit: true,
		},
		{
			name:              "type_no_unit_no_meta",
			typeLabel:         "counter",
			unitLabel:         "",
			metadata:          nil,
			expectedType:      writev2.Metadata_METRIC_TYPE_COUNTER,
			expectedUnit:      "",
			enableTypeAndUnit: true,
		},
		{
			name:              "no_type_and_unit_no_meta",
			typeLabel:         "gauge",
			unitLabel:         "bytes",
			metadata:          nil,
			expectedType:      writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
			expectedUnit:      "",
			enableTypeAndUnit: false,
		},
		{
			name:      "type_and_unit_and_meta",
			typeLabel: "gauge",
			unitLabel: "bytes",
			metadata: &metadata.Metadata{
				Type: model.MetricTypeGauge,
				Unit: "bytes",
				Help: "Test metric",
			},
			expectedType:      writev2.Metadata_METRIC_TYPE_GAUGE,
			expectedUnit:      "bytes",
			enableTypeAndUnit: true,
		},
		{
			name:      "type-and-unit-overrides-meta",
			typeLabel: "counter",
			unitLabel: "requests",
			metadata: &metadata.Metadata{
				Type: model.MetricTypeGauge,
				Unit: "bytes",
				Help: "Test metric",
			},
			expectedType:      writev2.Metadata_METRIC_TYPE_COUNTER,
			expectedUnit:      "requests",
			enableTypeAndUnit: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			batch := make([]timeSeries, 1)
			builder := labels.NewScratchBuilder(3)

			// Simulate labels with __type__ and __unit__ as scraped with type-and-unit-labels feature.
			builder.Add(labels.MetricName, "test_metric")
			if tc.typeLabel != "" {
				builder.Add("__type__", tc.typeLabel)
			}
			if tc.unitLabel != "" {
				builder.Add("__unit__", tc.unitLabel)
			}
			builder.Sort()

			batch[0] = timeSeries{
				seriesLabels: builder.Labels(),
				value:        123.45,
				timestamp:    time.Now().UnixMilli(),
				sType:        tSample,
				metadata:     tc.metadata,
			}

			pendingData := make([]writev2.TimeSeries, 1)

			symbolTable.Reset()
			nSamples, nExemplars, nHistograms, nMetadata, _ := populateV2TimeSeries(
				&symbolTable,
				batch,
				pendingData,
				false,
				false,
				tc.enableTypeAndUnit,
			)

			require.Equal(t, 1, nSamples, "Should have 1 sample")
			require.Equal(t, 0, nExemplars, "Should have 0 exemplars")
			require.Equal(t, 0, nHistograms, "Should have 0 histograms")

			// Verify type is correctly extracted.
			require.Equal(t, tc.expectedType, pendingData[0].Metadata.Type,
				"Type should match expected for %s", tc.name)

			// Verify unit is correctly extracted.
			unitRef := pendingData[0].Metadata.UnitRef
			symbols := symbolTable.Symbols()
			var actualUnit string
			if unitRef > 0 && unitRef < uint32(len(symbols)) {
				actualUnit = symbols[unitRef]
			}
			require.Equal(t, tc.expectedUnit, actualUnit, "Unit should match for %s", tc.name)

			// Verify metadata count.
			if tc.metadata != nil && tc.enableTypeAndUnit {
				require.Equal(t, 1, nMetadata, "Should count metadata when d.metadata is provided")
			}
		})
	}
}

func TestHighestTimestampOnAppend(t *testing.T) {
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			nSamples := 11 * config.DefaultQueueConfig.Capacity
			nSeries := 3
			recs := testwal.GenerateRecords(recCase{
				NoST:   protoMsg == remoteapi.WriteV1MessageType, // RW1 does not support ST.
				Series: nSeries, SamplesPerSeries: nSamples / nSeries,
			})

			_, m := newTestClientAndQueueManager(t, defaultFlushDeadline, protoMsg)
			m.Start()
			defer m.Stop()

			require.Equal(t, 0.0, m.metrics.highestTimestamp.Get())

			m.StoreSeries(recs.Series, 0)
			require.True(t, m.Append(recs.Samples))

			// Check that Append sets the highest timestamp correctly.
			// NOTE: testwal.GenerateRecords yields nSamples/nSeries samples (36666), with <i for samplesPerSeries> timestamp.
			// This gives the highest timestamp of 36666/1000 (seconds).
			const expectedHighestTsSeconds = 36.0
			require.Equal(t, expectedHighestTsSeconds, m.metrics.highestTimestamp.Get())
		})
	}
}

func TestAppendHistogramSchemaValidation(t *testing.T) {
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(string(protoMsg), func(t *testing.T) {
			c := NewTestWriteClient(protoMsg)
			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1

			m := newTestQueueManager(t, cfg, mcfg, defaultFlushDeadline, c, protoMsg)
			m.sendNativeHistograms = true

			// Create series for the histograms
			series := []record.RefSeries{
				{
					Ref:    chunks.HeadSeriesRef(0),
					Labels: labels.FromStrings("__name__", "test_histogram"),
				},
			}
			m.StoreSeries(series, 0)

			// Create histograms with different schemas
			histograms := []record.RefHistogramSample{
				{
					Ref: chunks.HeadSeriesRef(0),
					T:   1234567890,
					H: &histogram.Histogram{
						Schema:          0, // Valid schema.
						ZeroThreshold:   1e-128,
						ZeroCount:       0,
						Count:           2,
						Sum:             5.0,
						PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						PositiveBuckets: []int64{2},
						NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						NegativeBuckets: []int64{1},
					},
				},
				{
					Ref: chunks.HeadSeriesRef(0),
					T:   1234567891,
					H: &histogram.Histogram{
						Schema:          histogram.CustomBucketsSchema, // Not valid for version 1.0.
						ZeroThreshold:   1e-128,
						ZeroCount:       0,
						Count:           1,
						Sum:             3.0,
						PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						PositiveBuckets: []int64{1},
						CustomValues:    []float64{2.0},
					},
				},
				{
					Ref: chunks.HeadSeriesRef(0),
					T:   1234567892,
					H: &histogram.Histogram{
						Schema:          0, // Valid schema.
						ZeroThreshold:   1e-128,
						ZeroCount:       0,
						Count:           2,
						Sum:             5.0,
						PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						PositiveBuckets: []int64{2},
						NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						NegativeBuckets: []int64{1},
					},
				},
			}

			floatHistograms := []record.RefFloatHistogramSample{
				{
					Ref: chunks.HeadSeriesRef(0),
					T:   1234567890,
					FH: &histogram.FloatHistogram{
						Schema:          0, // Valid schema.
						ZeroThreshold:   1e-128,
						ZeroCount:       0,
						Count:           2,
						Sum:             5.0,
						PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						PositiveBuckets: []float64{2.0},
						NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						NegativeBuckets: []float64{1.0},
					},
				},
				{
					Ref: chunks.HeadSeriesRef(0),
					T:   1234567891,
					FH: &histogram.FloatHistogram{
						Schema:          histogram.CustomBucketsSchema, // Not valid for version 1.0.
						ZeroThreshold:   1e-128,
						ZeroCount:       0,
						Count:           1,
						Sum:             3.0,
						PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						PositiveBuckets: []float64{1.0},
						CustomValues:    []float64{2.0},
					},
				},
				{
					Ref: chunks.HeadSeriesRef(0),
					T:   1234567892,
					FH: &histogram.FloatHistogram{
						Schema:          0, // Valid schema.
						ZeroThreshold:   1e-128,
						ZeroCount:       0,
						Count:           2,
						Sum:             5.0,
						PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						PositiveBuckets: []float64{2.0},
						NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
						NegativeBuckets: []float64{1.0},
					},
				},
			}

			if protoMsg == remoteapi.WriteV1MessageType {
				c.expectHistograms([]record.RefHistogramSample{histograms[0], histograms[2]}, series)
				c.expectFloatHistograms([]record.RefFloatHistogramSample{floatHistograms[0], floatHistograms[2]}, series)
			} else {
				c.expectHistograms(histograms, series)
				c.expectFloatHistograms(floatHistograms, series)
			}

			m.Start()
			defer m.Stop()

			// Get initial dropped histograms count
			initialDropped := client_testutil.ToFloat64(m.metrics.droppedHistogramsTotal.WithLabelValues(reasonNHCBNotSupported))

			require.True(t, m.AppendHistograms(histograms))
			require.True(t, m.AppendFloatHistograms(floatHistograms))

			// Wait for the valid histogram to be received
			c.waitForExpectedData(t, 30*time.Second)

			// Verify that one histogram was dropped due to invalid schema
			finalDropped := client_testutil.ToFloat64(m.metrics.droppedHistogramsTotal.WithLabelValues(reasonNHCBNotSupported))

			if protoMsg == remoteapi.WriteV1MessageType {
				require.Equal(t, initialDropped+2.0, finalDropped, "Expected exactly two histograms to be dropped due to invalid schema")
			} else {
				require.Equal(t, initialDropped, finalDropped, "No histograms should be dropped")
			}

			// Verify no failed histograms (this would indicate a different type of error)
			require.Equal(t, 0.0, client_testutil.ToFloat64(m.metrics.failedHistogramsTotal))
		})
	}
}

// TestAppendHistogramsWithStartTimestamp verifies that AppendHistograms and
// AppendFloatHistograms propagate the per-sample start timestamp end-to-end
// through the queue manager into a Remote Write 2.0 request. ST is only
// carried by RW2, so RW1 is not exercised here.
func TestAppendHistogramsWithStartTimestamp(t *testing.T) {
	protoMsg := remoteapi.WriteV2MessageType

	c := NewTestWriteClient(protoMsg)
	cfg := testDefaultQueueConfig()
	mcfg := config.DefaultMetadataConfig
	cfg.MaxShards = 1

	m := newTestQueueManager(t, cfg, mcfg, defaultFlushDeadline, c, protoMsg)
	m.sendNativeHistograms = true

	series := []record.RefSeries{
		{
			Ref:    chunks.HeadSeriesRef(0),
			Labels: labels.FromStrings("__name__", "test_histogram"),
		},
	}
	m.StoreSeries(series, 0)

	intHist := func(seed int64) *histogram.Histogram {
		h := &histogram.Histogram{
			Schema:          0,
			ZeroThreshold:   1e-128,
			Count:           uint64(seed + 1),
			Sum:             float64(seed),
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
			PositiveBuckets: []int64{seed + 1},
		}
		return h
	}
	floatHist := func(seed int64) *histogram.FloatHistogram {
		fh := &histogram.FloatHistogram{
			Schema:          0,
			ZeroThreshold:   1e-128,
			Count:           float64(seed + 1),
			Sum:             float64(seed),
			PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
			PositiveBuckets: []float64{float64(seed + 1)},
		}
		return fh
	}

	histograms := []record.RefHistogramSample{
		{Ref: chunks.HeadSeriesRef(0), ST: 100, T: 200, H: intHist(0)},
		{Ref: chunks.HeadSeriesRef(0), ST: 100, T: 300, H: intHist(1)},
		{Ref: chunks.HeadSeriesRef(0), ST: 250, T: 400, H: intHist(2)},
	}
	floatHistograms := []record.RefFloatHistogramSample{
		{Ref: chunks.HeadSeriesRef(0), ST: 101, T: 201, FH: floatHist(0)},
		{Ref: chunks.HeadSeriesRef(0), ST: 101, T: 301, FH: floatHist(1)},
		{Ref: chunks.HeadSeriesRef(0), ST: 251, T: 401, FH: floatHist(2)},
	}

	c.expectHistograms(histograms, series)
	c.expectFloatHistograms(floatHistograms, series)

	m.Start()
	defer m.Stop()

	require.True(t, m.AppendHistograms(histograms))
	require.True(t, m.AppendFloatHistograms(floatHistograms))

	c.waitForExpectedData(t, 30*time.Second)
}

type nhcbQueueTestOptions struct {
	protoMsg             remoteapi.WriteMessageType
	sendNativeHistograms bool
	sendExemplars        bool
	numShards            int
	samplesIn            *ewmaRate
	highestRecvTimestamp *maxTimestamp
	sampleAgeLimit       time.Duration
	batchSendDeadline    time.Duration
}

func newTestNHCBQueueManager(t testing.TB, c WriteClient, opts nhcbQueueTestOptions) (*QueueManager, func()) {
	t.Helper()
	cfg := config.DefaultQueueConfig
	cfg.MaxSamplesPerSend = 1000
	cfg.BatchSendDeadline = model.Duration(time.Minute)
	if opts.batchSendDeadline != 0 {
		cfg.BatchSendDeadline = model.Duration(opts.batchSendDeadline)
	}
	cfg.SampleAgeLimit = model.Duration(opts.sampleAgeLimit)
	mcfg := config.DefaultMetadataConfig
	mcfg.Send = false
	if opts.numShards == 0 {
		opts.numShards = 1
	}
	if opts.samplesIn == nil {
		opts.samplesIn = newEWMARate(ewmaWeight, shardUpdateDuration)
	}
	if opts.highestRecvTimestamp == nil {
		opts.highestRecvTimestamp = newHighestTimestampMetric()
	}
	m := NewQueueManager(newQueueManagerMetrics(nil, "", ""), nil, nil, nil, t.TempDir(), opts.samplesIn,
		cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), opts.highestRecvTimestamp,
		nil, opts.sendExemplars, opts.sendNativeHistograms, false, opts.protoMsg, record.NewBuffersPool(), false, true)
	// Start the production senders without the WAL watcher or automatic resharding loop.
	m.numShards = opts.numShards
	m.shards.start(opts.numShards)
	stop := sync.OnceFunc(m.shards.stop)
	t.Cleanup(stop)
	return m, stop
}

func TestAppendToConvertNHCBToClassic(t *testing.T) {
	h := &histogram.Histogram{
		Schema:       histogram.CustomBucketsSchema,
		CustomValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 4, Length: 1},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []int64{1, 2, 3, 4, 5},
		Count:           35,
		Sum:             123,
	}

	fh := &histogram.FloatHistogram{
		Schema:       histogram.CustomBucketsSchema,
		CustomValues: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 2},
			{Offset: 4, Length: 1},
			{Offset: 1, Length: 2},
		},
		PositiveBuckets: []float64{1, 2, 3, 4, 5},
		Count:           15,
		Sum:             123,
	}

	expH := &histogram.Histogram{
		Schema:          0,
		ZeroThreshold:   1e-128,
		Count:           10,
		Sum:             20,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []int64{10},
	}

	expFH := &histogram.FloatHistogram{
		Schema:          0,
		ZeroThreshold:   1e-128,
		Count:           10,
		Sum:             20,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
		PositiveBuckets: []float64{10},
	}

	testCases := []struct {
		name                    string
		protoMsg                remoteapi.WriteMessageType
		isFloatHistogram        bool
		sendNativeHistograms    bool
		convertFlag             bool
		relabelConfigs          []*relabel.Config
		expectedMetricName      string
		expectedSamplesCount    float64
		expectedHistogramsCount float64
		expectedDroppedSeries   float64
	}{
		{
			name:                    "convert nhcb to classic v1",
			protoMsg:                remoteapi.WriteV1MessageType,
			isFloatHistogram:        false,
			sendNativeHistograms:    true,
			convertFlag:             true,
			expectedMetricName:      "test_histogram",
			expectedSamplesCount:    13,
			expectedHistogramsCount: 0,
		},
		{
			name:                    "convert float nhcb to classic v1",
			protoMsg:                remoteapi.WriteV1MessageType,
			isFloatHistogram:        true,
			sendNativeHistograms:    true,
			convertFlag:             true,
			expectedMetricName:      "test_histogram",
			expectedSamplesCount:    13,
			expectedHistogramsCount: 0,
		},
		{
			name:                    "convert nhcb to classic v2",
			protoMsg:                remoteapi.WriteV2MessageType,
			isFloatHistogram:        false,
			sendNativeHistograms:    true,
			convertFlag:             true,
			expectedMetricName:      "test_histogram",
			expectedSamplesCount:    13,
			expectedHistogramsCount: 0,
		},
		{
			name:                    "convert float nhcb to classic v2",
			protoMsg:                remoteapi.WriteV2MessageType,
			isFloatHistogram:        true,
			sendNativeHistograms:    true,
			convertFlag:             true,
			expectedMetricName:      "test_histogram",
			expectedSamplesCount:    13,
			expectedHistogramsCount: 0,
		},
		{
			name:                    "convert nhcb to classic with send_native_histograms=false",
			protoMsg:                remoteapi.WriteV1MessageType,
			isFloatHistogram:        false,
			sendNativeHistograms:    false,
			convertFlag:             true,
			expectedMetricName:      "test_histogram",
			expectedSamplesCount:    13,
			expectedHistogramsCount: 0,
		},
		{
			name:                    "convert float nhcb to classic with send_native_histograms=false",
			protoMsg:                remoteapi.WriteV1MessageType,
			isFloatHistogram:        true,
			sendNativeHistograms:    false,
			convertFlag:             true,
			expectedMetricName:      "test_histogram",
			expectedSamplesCount:    13,
			expectedHistogramsCount: 0,
		},
		{
			name:                 "convert nhcb to classic after write_relabel_configs rename",
			protoMsg:             remoteapi.WriteV2MessageType,
			isFloatHistogram:     false,
			sendNativeHistograms: true,
			convertFlag:          true,
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{model.MetricNameLabel},
					Regex:                relabel.MustNewRegexp("test_histogram"),
					TargetLabel:          model.MetricNameLabel,
					Replacement:          "relabeled_histogram",
					Action:               relabel.Replace,
					NameValidationScheme: model.UTF8Validation,
				},
			},
			expectedMetricName:      "relabeled_histogram",
			expectedSamplesCount:    13,
			expectedHistogramsCount: 0,
		},
		{
			name:                 "drop nhcb via write_relabel_configs before conversion",
			protoMsg:             remoteapi.WriteV1MessageType,
			isFloatHistogram:     false,
			sendNativeHistograms: false,
			convertFlag:          true,
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{model.MetricNameLabel},
					Regex:                relabel.MustNewRegexp("test_histogram"),
					Action:               relabel.Drop,
					NameValidationScheme: model.UTF8Validation,
				},
			},
			expectedSamplesCount:    0,
			expectedHistogramsCount: 0,
			expectedDroppedSeries:   1,
		},
		{
			name:                    "no nhcb to classic conversion v2",
			protoMsg:                remoteapi.WriteV2MessageType,
			sendNativeHistograms:    true,
			convertFlag:             false,
			isFloatHistogram:        false,
			expectedSamplesCount:    0,
			expectedHistogramsCount: 1,
		},
		{
			name:                    "no float nhcb to classic conversion v2",
			protoMsg:                remoteapi.WriteV2MessageType,
			sendNativeHistograms:    true,
			convertFlag:             false,
			isFloatHistogram:        true,
			expectedSamplesCount:    0,
			expectedHistogramsCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewTestWriteClient(tc.protoMsg)
			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1

			m := newTestQueueManager(t, cfg, mcfg, defaultFlushDeadline, c, tc.protoMsg)
			m.convertNHCBToClassic = tc.convertFlag
			m.sendNativeHistograms = tc.sendNativeHistograms
			m.relabelConfigs = tc.relabelConfigs

			series := []record.RefSeries{
				{
					Ref:    chunks.HeadSeriesRef(0),
					Labels: labels.FromStrings("__name__", "test_histogram"),
				},
				{
					Ref:    chunks.HeadSeriesRef(1),
					Labels: labels.FromStrings("__name__", "exp_histogram"),
				},
			}
			m.StoreSeries(series, 0)

			m.Start()
			defer m.Stop()

			initialDroppedConversionError := client_testutil.ToFloat64(m.metrics.droppedHistogramsTotal.WithLabelValues(reasonNHCBConversionError))

			const (
				st = int64(1234560000)
				ts = int64(1234567890)
			)
			if !tc.isFloatHistogram {
				samples := []record.RefHistogramSample{{Ref: chunks.HeadSeriesRef(0), ST: st, T: ts, H: h}}
				if !tc.sendNativeHistograms {
					// Non-NHCB histograms must be ignored when send_native_histograms is false.
					samples = append(samples, record.RefHistogramSample{Ref: chunks.HeadSeriesRef(1), ST: st, T: ts, H: expH})
				}
				require.True(t, m.AppendHistograms(samples))
			} else {
				samples := []record.RefFloatHistogramSample{{Ref: chunks.HeadSeriesRef(0), ST: st, T: ts, FH: fh}}
				if !tc.sendNativeHistograms {
					samples = append(samples, record.RefFloatHistogramSample{Ref: chunks.HeadSeriesRef(1), ST: st, T: ts, FH: expFH})
				}
				require.True(t, m.AppendFloatHistograms(samples))
			}

			require.Eventually(t, func() bool {
				return client_testutil.ToFloat64(m.metrics.samplesTotal) == tc.expectedSamplesCount &&
					client_testutil.ToFloat64(m.metrics.histogramsTotal) == tc.expectedHistogramsCount &&
					client_testutil.ToFloat64(m.metrics.droppedHistogramsTotal.WithLabelValues(reasonDroppedSeries)) == tc.expectedDroppedSeries
			}, 5*time.Second, 10*time.Millisecond)

			finalDroppedConversionError := client_testutil.ToFloat64(m.metrics.droppedHistogramsTotal.WithLabelValues(reasonNHCBConversionError))
			require.Equal(t, initialDroppedConversionError, finalDroppedConversionError, "No conversion errors should occur")
			require.Equal(t, 0.0, client_testutil.ToFloat64(m.metrics.failedHistogramsTotal))

			if tc.expectedSamplesCount > 0 {
				expectedST := int64(0)
				if tc.protoMsg == remoteapi.WriteV2MessageType {
					expectedST = st
				}
				expectedCount := h.Count
				if tc.isFloatHistogram {
					expectedCount = uint64(fh.Count)
				}
				c.mtx.Lock()
				require.Equal(t, []writev2.Sample{{StartTimestamp: expectedST, Timestamp: ts, Value: float64(expectedCount)}}, c.receivedSamples[fmt.Sprintf("{__name__=%q}", tc.expectedMetricName+"_count")])
				require.Equal(t, []writev2.Sample{{StartTimestamp: expectedST, Timestamp: ts, Value: 123}}, c.receivedSamples[fmt.Sprintf("{__name__=%q}", tc.expectedMetricName+"_sum")])
				require.Equal(t, []writev2.Sample{{StartTimestamp: expectedST, Timestamp: ts, Value: float64(expectedCount)}}, c.receivedSamples[fmt.Sprintf("{__name__=%q, le=\"+Inf\"}", tc.expectedMetricName+"_bucket")])
				c.mtx.Unlock()
			}
		})
	}

	baseSeries := record.RefSeries{Ref: 1, Labels: labels.FromStrings("__name__", "latency", "job", "api")}
	classicSeries := []record.RefSeries{
		{Ref: 1, Labels: labels.FromStrings("__name__", "latency_bucket", "job", "api", "le", "1.0")},
		{Ref: 2, Labels: labels.FromStrings("__name__", "latency_bucket", "job", "api", "le", "2.0")},
		{Ref: 3, Labels: labels.FromStrings("__name__", "latency_bucket", "job", "api", "le", "+Inf")},
		{Ref: 4, Labels: labels.FromStrings("__name__", "latency_count", "job", "api")},
		{Ref: 5, Labels: labels.FromStrings("__name__", "latency_sum", "job", "api")},
	}
	classicValues := []float64{2, 5, 9, 9, 13}
	newHistogram := func() *histogram.Histogram {
		return &histogram.Histogram{
			Schema:          histogram.CustomBucketsSchema,
			CustomValues:    []float64{1, 2},
			PositiveSpans:   []histogram.Span{{Length: 3}},
			PositiveBuckets: []int64{2, 1, 1},
			Count:           9,
			Sum:             13,
		}
	}
	appendHistogram := func(t *testing.T, m *QueueManager, h *histogram.Histogram, ts int64, asFloat bool) {
		t.Helper()
		require.NoError(t, h.Validate())
		if asFloat {
			require.True(t, m.AppendFloatHistograms([]record.RefFloatHistogramSample{{Ref: baseSeries.Ref, T: ts, FH: h.ToFloat(nil)}}))
		} else {
			require.True(t, m.AppendHistograms([]record.RefHistogramSample{{Ref: baseSeries.Ref, T: ts, H: h}}))
		}
	}
	requireInitialSamples := func(t *testing.T, c *TestWriteClient) {
		t.Helper()
		for i, series := range classicSeries {
			got := c.receivedSamples[series.Labels.String()]
			require.NotEmpty(t, got, "normal sample for %s", series.Labels)
			require.Equal(t, writev2.Sample{Timestamp: 1000, Value: classicValues[i]}, got[0], "normal sample for %s", series.Labels)
		}
	}

	t.Run("sample type transitions", func(t *testing.T) {
		for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
			for _, target := range []struct {
				name       string
				kind       string
				sendNative bool
			}{
				{name: "ordinary float", kind: "float", sendNative: true},
				{name: "integer exponential histogram", kind: "integer", sendNative: true},
				{name: "float exponential histogram", kind: "histogram", sendNative: true},
				{name: "integer exponential without forwarding", kind: "integer"},
				{name: "float exponential without forwarding", kind: "histogram"},
			} {
				if !target.sendNative && protoMsg == remoteapi.WriteV2MessageType {
					continue
				}
				for _, prior := range []string{"ordinary control", "after integer NHCB", "after float NHCB"} {
					t.Run(fmt.Sprintf("%s/%s/%s", protoMsg, target.name, prior), func(t *testing.T) {
						c := NewTestWriteClient(protoMsg)
						m, stop := newTestNHCBQueueManager(t, c, nhcbQueueTestOptions{
							protoMsg: protoMsg, sendNativeHistograms: target.sendNative, sendExemplars: target.sendNative,
						})
						m.StoreSeries([]record.RefSeries{baseSeries}, 0)
						converted := prior != "ordinary control"
						if converted {
							appendHistogram(t, m, newHistogram(), 1000, prior == "after float NHCB")
						}
						before := m.dataInExpanded.newEvents.Load()
						h := newHistogram()
						h.Schema = 0
						h.CustomValues = nil
						require.NoError(t, h.Validate())
						stale := math.Float64frombits(value.StaleNaN)
						switch target.kind {
						case "float":
							require.True(t, m.Append([]record.RefSample{{Ref: baseSeries.Ref, ST: 1500, T: 2000, V: 17}}))
						case "integer":
							require.True(t, m.AppendHistograms([]record.RefHistogramSample{{Ref: baseSeries.Ref, ST: 1500, T: 2000, H: h}}))
						case "histogram":
							require.True(t, m.AppendFloatHistograms([]record.RefFloatHistogramSample{{Ref: baseSeries.Ref, ST: 1500, T: 2000, FH: h.ToFloat(nil)}}))
						}
						extra := int64(0)
						if converted {
							extra = 5
							if !target.sendNative {
								extra--
							}
						}
						expanded := m.dataInExpanded.newEvents.Load() - before
						if target.sendNative {
							require.True(t, m.AppendExemplars([]record.RefExemplar{{
								Ref: baseSeries.Ref, T: 2500, V: 1.5, Labels: labels.FromStrings("trace_id", "abc"),
							}}))
						}
						beforeStale := m.dataInExpanded.newEvents.Load()
						switch target.kind {
						case "float":
							require.True(t, m.Append([]record.RefSample{{Ref: baseSeries.Ref, T: 3000, V: stale}}))
						case "integer":
							require.True(t, m.AppendHistograms([]record.RefHistogramSample{{Ref: baseSeries.Ref, T: 3000, H: &histogram.Histogram{Sum: stale}}}))
						case "histogram":
							require.True(t, m.AppendFloatHistograms([]record.RefFloatHistogramSample{{Ref: baseSeries.Ref, T: 3000, FH: &histogram.FloatHistogram{Sum: stale}}}))
						}
						afterStale := m.dataInExpanded.newEvents.Load()
						stop()
						t.Run("expansion accounting", func(t *testing.T) {
							require.Equal(t, extra, expanded, "retirement and replacement must be accounted together")
							require.Equal(t, beforeStale, afterStale, "the replacement's marker must not expand into old classic series")
						})
						if converted {
							t.Run("converted series retirement", func(t *testing.T) {
								requireInitialSamples(t, c)
								for _, series := range classicSeries {
									got := c.receivedSamples[series.Labels.String()]
									require.Len(t, got, 2, "retired series %s", series.Labels)
									require.Equal(t, int64(2000), got[1].Timestamp)
									require.Zero(t, got[1].StartTimestamp, "retirement must not borrow the replacement's start time")
									require.True(t, value.IsStaleNaN(got[1].Value))
								}
							})
						}
						if !target.sendNative {
							require.Empty(t, c.receivedHistograms)
							require.Empty(t, c.receivedFloatHistograms)
							return
						}
						expectedST := int64(0)
						if protoMsg == remoteapi.WriteV2MessageType {
							expectedST = 1500
						}
						key := baseSeries.Labels.String()
						t.Run("replacement identity", func(t *testing.T) {
							if target.kind == "float" {
								got := c.receivedSamples[key]
								require.Len(t, got, 2, "the replacement float must receive its own stale marker")
								require.Equal(t, writev2.Sample{StartTimestamp: expectedST, Timestamp: 2000, Value: 17}, got[0])
								require.Equal(t, int64(3000), got[1].Timestamp)
								require.True(t, value.IsStaleNaN(got[1].Value))
							} else {
								got := c.receivedHistograms[key]
								want := writev2.FromIntHistogram(expectedST, 2000, h)
								if target.kind == "histogram" {
									got = c.receivedFloatHistograms[key]
									want = writev2.FromFloatHistogram(expectedST, 2000, h.ToFloat(nil))
								}
								require.Len(t, got, 2, "the replacement histogram must receive its own stale marker")
								require.Equal(t, want, got[0])
								require.Equal(t, int64(3000), got[1].Timestamp)
								require.True(t, value.IsStaleNaN(got[1].Sum))
							}
						})
						t.Run("exemplar identity", func(t *testing.T) {
							require.Equal(t, map[string][]prompb.Exemplar{key: {{
								Timestamp: 2500, Value: 1.5, Labels: []prompb.Label{{Name: "trace_id", Value: "abc"}},
							}}}, c.receivedExemplars, "replacement exemplars must not retain bucket identity")
						})
					})
				}
			}
		}
	})

	t.Run("age filtered replacements preserve conversion", func(t *testing.T) {
		// A type change dropped by sample_age_limit must neither retire the classic series
		// nor stop their conversion; otherwise the series' later stale marker would miss them.
		// The fake clock makes the age limit pass instantly and deterministically.
		synctest.Test(t, func(t *testing.T) {
			c := NewTestWriteClient(remoteapi.WriteV1MessageType)
			// Native forwarding is on, so a wrongly accepted histogram replacement would be sent.
			m, stop := newTestNHCBQueueManager(t, c, nhcbQueueTestOptions{
				protoMsg: remoteapi.WriteV1MessageType, sendNativeHistograms: true, sampleAgeLimit: time.Second, batchSendDeadline: time.Millisecond,
			})
			m.StoreSeries([]record.RefSeries{baseSeries}, 0)
			firstT := time.Now().UnixMilli()
			appendHistogram(t, m, newHistogram(), firstT, false)
			time.Sleep(2 * time.Second)
			// Each sample type change is now too old to send.
			require.True(t, m.Append([]record.RefSample{{Ref: baseSeries.Ref, T: firstT + 1, V: 17}}))
			h := newHistogram()
			h.Schema = 0
			h.CustomValues = nil
			require.True(t, m.AppendHistograms([]record.RefHistogramSample{{Ref: baseSeries.Ref, T: firstT + 2, H: h}}))
			require.True(t, m.AppendFloatHistograms([]record.RefFloatHistogramSample{{Ref: baseSeries.Ref, T: firstT + 3, FH: h.ToFloat(nil)}}))
			// A fresh stale marker must still reach every classic series.
			staleT := time.Now().UnixMilli()
			require.True(t, m.Append([]record.RefSample{{Ref: baseSeries.Ref, T: staleT, V: math.Float64frombits(value.StaleNaN)}}))
			stop()
			for _, series := range classicSeries {
				got := c.receivedSamples[series.Labels.String()]
				require.Len(t, got, 2)
				require.Equal(t, staleT, got[1].Timestamp)
				require.True(t, value.IsStaleNaN(got[1].Value))
			}
			// The dropped samples must not have been forwarded.
			require.Len(t, c.receivedSamples, len(classicSeries))
			require.Empty(t, c.receivedHistograms)
			require.Empty(t, c.receivedFloatHistograms)
		})
	})

	t.Run("staleness", func(t *testing.T) {
		for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
			for _, asFloat := range []bool{false, true} {
				for _, sendNative := range []bool{false, true} {
					for _, marker := range []string{"float", "histogram without layout", "custom bucket histogram"} {
						t.Run(fmt.Sprintf("%s/float=%t/send native histograms=%t/%s", protoMsg, asFloat, sendNative, marker), func(t *testing.T) {
							c := NewTestWriteClient(protoMsg)
							m, stop := newTestNHCBQueueManager(t, c, nhcbQueueTestOptions{protoMsg: protoMsg, sendNativeHistograms: sendNative})
							m.StoreSeries([]record.RefSeries{baseSeries}, 0)
							h := newHistogram()
							appendHistogram(t, m, h, 1000, asFloat)
							stale := math.Float64frombits(value.StaleNaN)
							switch marker {
							case "float":
								// Separate scrape transactions produce a float WAL staleness marker.
								require.True(t, m.Append([]record.RefSample{{Ref: baseSeries.Ref, T: 2000, V: stale}}))
							case "histogram without layout":
								appendHistogram(t, m, &histogram.Histogram{Sum: stale}, 2000, asFloat)
							case "custom bucket histogram":
								h = h.Copy()
								h.Sum = stale
								appendHistogram(t, m, h, 2000, asFloat)
							}
							stop()
							requireInitialSamples(t, c)
							for _, series := range classicSeries {
								got := c.receivedSamples[series.Labels.String()]
								require.Len(t, got, 2, "converted series %s must receive a stale marker", series.Labels)
								require.Equal(t, int64(2000), got[1].Timestamp)
								require.True(t, value.IsStaleNaN(got[1].Value), "converted series %s must be stale", series.Labels)
							}
							require.Len(t, c.receivedSamples, len(classicSeries))
						})
					}
				}
			}
		}
		t.Run("ordinary float control", func(t *testing.T) {
			c := NewTestWriteClient(remoteapi.WriteV1MessageType)
			m, stop := newTestNHCBQueueManager(t, c, nhcbQueueTestOptions{protoMsg: remoteapi.WriteV1MessageType})
			m.StoreSeries([]record.RefSeries{baseSeries}, 0)
			require.True(t, m.Append([]record.RefSample{
				{Ref: baseSeries.Ref, T: 1000, V: 13},
				{Ref: baseSeries.Ref, T: 2000, V: math.Float64frombits(value.StaleNaN)},
			}))
			stop()
			got := c.receivedSamples[baseSeries.Labels.String()]
			require.Len(t, got, 2)
			require.Equal(t, writev2.Sample{Timestamp: 1000, Value: 13}, got[0])
			require.Equal(t, int64(2000), got[1].Timestamp)
			require.True(t, value.IsStaleNaN(got[1].Value))
		})
	})

	t.Run("removed boundaries", func(t *testing.T) {
		for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
			for _, asFloat := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/float=%t", protoMsg, asFloat), func(t *testing.T) {
					c := NewTestWriteClient(protoMsg)
					m, stop := newTestNHCBQueueManager(t, c, nhcbQueueTestOptions{protoMsg: protoMsg})
					m.StoreSeries([]record.RefSeries{baseSeries}, 0)
					// Start with finite boundaries 1 and 2, then remove 1 while the histogram stays active.
					appendHistogram(t, m, newHistogram(), 1000, asFloat)
					appendHistogram(t, m, &histogram.Histogram{
						Schema:          histogram.CustomBucketsSchema,
						CustomValues:    []float64{2},
						PositiveSpans:   []histogram.Span{{Length: 2}},
						PositiveBuckets: []int64{5, 0},
						Count:           10,
						Sum:             15,
					}, 2000, asFloat)
					stop()
					requireInitialSamples(t, c)
					for i, v := range []float64{5, 10, 10, 15} {
						series := classicSeries[i+1]
						require.Equal(t, []writev2.Sample{
							{Timestamp: 1000, Value: classicValues[i+1]},
							{Timestamp: 2000, Value: v},
						}, c.receivedSamples[series.Labels.String()], "retained series %s", series.Labels)
					}
					// The removed le="1.0" bucket must become stale while the retained series keep updating.
					got := c.receivedSamples[classicSeries[0].Labels.String()]
					require.Len(t, got, 2, "removed bucket must receive a stale marker")
					require.Equal(t, int64(2000), got[1].Timestamp)
					require.True(t, value.IsStaleNaN(got[1].Value))
				})
			}
		}
	})

	t.Run("boundary layouts", func(t *testing.T) {
		// When an NHCB series changes bucket layout, every bucket of the new layout is sent, and
		// each finite bucket it dropped gets one stale marker without a start time. +Inf, _count
		// and _sum exist in every layout, so they are never retired.
		for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
			for _, asFloat := range []bool{false, true} {
				for _, tc := range []struct {
					name          string
					before, after []float64
				}{
					{name: "unchanged", before: []float64{1, 2}, after: []float64{1, 2}},
					{name: "added boundary", before: []float64{1, 2}, after: []float64{0.5, 1, 2}},
					{name: "removed trailing boundary", before: []float64{1, 2}, after: []float64{1}},
					{name: "interleaved replacement", before: []float64{1, 2}, after: []float64{1.5, 2.5}},
					{name: "all finite boundaries removed", before: []float64{1, 2}},
					{name: "first finite boundary added", after: []float64{1}},
				} {
					t.Run(fmt.Sprintf("%s/float=%t/%s", protoMsg, asFloat, tc.name), func(t *testing.T) {
						c := NewTestWriteClient(protoMsg)
						m, stop := newTestNHCBQueueManager(t, c, nhcbQueueTestOptions{protoMsg: protoMsg})
						m.StoreSeries([]record.RefSeries{baseSeries}, 0)
						expected := map[string][]writev2.Sample{}
						for i, bounds := range [][]float64{tc.before, tc.after} {
							h := &histogram.Histogram{
								Schema: histogram.CustomBucketsSchema, CustomValues: bounds,
								PositiveSpans:   []histogram.Span{{Length: uint32(len(bounds) + 1)}},
								PositiveBuckets: make([]int64, len(bounds)+1), Count: uint64(len(bounds) + 1), Sum: 13,
							}
							// One observation per bucket gives cumulative counts 1, 2, ... including +Inf.
							h.PositiveBuckets[0] = 1
							require.NoError(t, h.Validate())
							ts := int64(1000 * (i + 1))
							st := ts - 500
							if asFloat {
								require.True(t, m.AppendFloatHistograms([]record.RefFloatHistogramSample{{Ref: baseSeries.Ref, ST: st, T: ts, FH: h.ToFloat(nil)}}))
							} else {
								require.True(t, m.AppendHistograms([]record.RefHistogramSample{{Ref: baseSeries.Ref, ST: st, T: ts, H: h}}))
							}
							// Only remote write v2 carries start times.
							if protoMsg == remoteapi.WriteV1MessageType {
								st = 0
							}
							for j, bound := range append(append([]float64(nil), bounds...), math.Inf(1)) {
								ls := labels.FromStrings("__name__", "latency_bucket", "job", "api", "le", labels.FormatOpenMetricsFloat(bound))
								expected[ls.String()] = append(expected[ls.String()], writev2.Sample{StartTimestamp: st, Timestamp: ts, Value: float64(j + 1)})
							}
							for _, s := range []struct {
								name  string
								value float64
							}{
								{name: "latency_count", value: float64(h.Count)},
								{name: "latency_sum", value: h.Sum},
							} {
								ls := labels.FromStrings("__name__", s.name, "job", "api")
								expected[ls.String()] = append(expected[ls.String()], writev2.Sample{StartTimestamp: st, Timestamp: ts, Value: s.value})
							}
						}
						// Buckets missing from the new layout are retired at its timestamp.
						for _, bound := range tc.before {
							if !slices.Contains(tc.after, bound) {
								ls := labels.FromStrings("__name__", "latency_bucket", "job", "api", "le", labels.FormatOpenMetricsFloat(bound))
								expected[ls.String()] = append(expected[ls.String()], writev2.Sample{Timestamp: 2000, Value: math.Float64frombits(value.StaleNaN)})
							}
						}
						stop()
						// Nothing beyond the expected series and samples may be sent.
						require.Len(t, c.receivedSamples, len(expected))
						for key, want := range expected {
							got := c.receivedSamples[key]
							require.Len(t, got, len(want), key)
							for i, sample := range want {
								require.Equal(t, sample.Timestamp, got[i].Timestamp)
								if value.IsStaleNaN(sample.Value) {
									require.True(t, value.IsStaleNaN(got[i].Value), key)
									require.Zero(t, got[i].StartTimestamp, "retired bucket %s must not borrow the new layout's start time", key)
								} else {
									require.Equal(t, sample, got[i], key)
								}
							}
						}
					})
				}
			}
		}
	})

	t.Run("shared bucket layouts", func(t *testing.T) {
		m, stop := newTestNHCBQueueManager(t, NewTestWriteClient(remoteapi.WriteV1MessageType), nhcbQueueTestOptions{protoMsg: remoteapi.WriteV1MessageType})
		defer stop()
		other := record.RefSeries{Ref: 2, Labels: labels.FromStrings("__name__", "latency", "job", "web")}
		m.StoreSeries([]record.RefSeries{baseSeries, other}, 0)
		appendTo := func(ref chunks.HeadSeriesRef, h *histogram.Histogram, ts int64) {
			t.Helper()
			require.True(t, m.AppendHistograms([]record.RefHistogramSample{{Ref: ref, T: ts, H: h}}))
		}
		appendTo(baseSeries.Ref, newHistogram(), 1000)
		appendTo(other.Ref, newHistogram(), 1000)
		require.Len(t, m.nhcbLayouts, 1)
		shared := m.nhcbSeriesBounds[baseSeries.Ref]
		require.Same(t, shared, m.nhcbSeriesBounds[other.Ref])
		require.Equal(t, 2, shared.refs)

		changed := newHistogram()
		changed.CustomValues = []float64{1, 3}
		require.NoError(t, changed.Validate())
		appendTo(other.Ref, changed, 2000)
		require.Len(t, m.nhcbLayouts, 2)
		require.Equal(t, 1, shared.refs)

		appendTo(other.Ref, &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}, 3000)
		require.Len(t, m.nhcbLayouts, 1, "a stale marker must release its layout")
		require.NotContains(t, m.nhcbSeriesBounds, other.Ref)

		m.SeriesReset(1)
		require.Empty(t, m.nhcbSeriesBounds)
		require.Empty(t, m.nhcbLayouts, "garbage collection must release the last reference")
	})

	t.Run("exemplars", func(t *testing.T) {
		for _, kind := range []string{"integer histogram", "float histogram", "classic control", "native control"} {
			t.Run(kind, func(t *testing.T) {
				// Use a fresh receiver so an existing base series cannot hide incorrect exemplar routing.
				opts := tsdb.DefaultHeadOptions()
				opts.ChunkDirRoot = t.TempDir()
				opts.EnableExemplarStorage = true
				opts.MaxExemplars.Store(10)
				head, err := tsdb.NewHead(nil, nil, nil, nil, opts, nil)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, head.Close()) })
				require.NoError(t, head.Init(math.MinInt64))
				handler := NewWriteHandler(promslog.NewNopLogger(), nil, head, []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType}, false, false)
				c := NewTestWriteClient(remoteapi.WriteV1MessageType)
				var statuses []int
				client := &MockWriteClient{
					NameFunc:     c.Name,
					EndpointFunc: c.Endpoint,
					StoreFunc: func(ctx context.Context, payload []byte, attempt int) (WriteResponseStats, error) {
						req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(payload))
						req.Header.Set("Content-Type", remoteWriteContentTypeHeaders[remoteapi.WriteV1MessageType])
						req.Header.Set("Content-Encoding", compression.Snappy)
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, req)
						statuses = append(statuses, response.Code)
						if response.Code != http.StatusNoContent {
							return WriteResponseStats{}, fmt.Errorf("receiver returned HTTP %d: %s", response.Code, response.Body.String())
						}
						return c.Store(ctx, payload, attempt)
					},
				}
				m, stop := newTestNHCBQueueManager(t, client, nhcbQueueTestOptions{
					protoMsg:             remoteapi.WriteV1MessageType,
					sendExemplars:        true,
					sendNativeHistograms: kind == "native control",
				})
				if kind == "classic control" {
					m.StoreSeries(classicSeries, 0)
					for i, series := range classicSeries {
						require.True(t, m.Append([]record.RefSample{{Ref: series.Ref, T: 1000, V: classicValues[i]}}))
					}
				} else {
					m.StoreSeries([]record.RefSeries{baseSeries}, 0)
					h := newHistogram()
					if kind == "native control" {
						// Use an exponential histogram to exercise native delivery without NHCB conversion.
						h.Schema = 0
						h.CustomValues = nil
					}
					appendHistogram(t, m, h, 1000, kind == "float histogram")
				}
				// Ref 1 identifies the le="1.0" bucket in the classic control and the original histogram otherwise.
				exemplarLabels := labels.FromStrings("trace_id", "abc")
				require.True(t, m.AppendExemplars([]record.RefExemplar{{Ref: 1, T: 1000, V: 0.5, Labels: exemplarLabels}}))
				stop()
				require.Equal(t, 1, deepLen(c.receivedExemplars), "sender must transmit exactly one exemplar")
				// RW1 may return HTTP 204 after dropping an exemplar for an unknown series, so query storage below.
				require.NotEmpty(t, statuses)
				for _, status := range statuses {
					require.Equal(t, http.StatusNoContent, status)
				}
				if kind == "native control" {
					require.Equal(t, uint64(1), head.NumSeries())
					require.Len(t, c.receivedHistograms[baseSeries.Labels.String()], 1)
				} else {
					require.Equal(t, uint64(len(classicSeries)), head.NumSeries())
					requireInitialSamples(t, c)
				}
				q, err := head.ExemplarQuerier(t.Context())
				require.NoError(t, err)
				got, err := q.Select(0, 2000, []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "__name__", "latency.*")})
				require.NoError(t, err)
				require.Len(t, got, 1, "receiver must preserve the histogram exemplar")
				require.Len(t, got[0].Exemplars, 1)
				testutil.RequireEqual(t, exemplarLabels, got[0].Exemplars[0].Labels)
				require.Equal(t, 0.5, got[0].Exemplars[0].Value)
				require.Equal(t, int64(1000), got[0].Exemplars[0].Ts)
				if kind == "native control" {
					testutil.RequireEqual(t, baseSeries.Labels, got[0].SeriesLabels)
				} else {
					require.Equal(t, "latency_bucket", got[0].SeriesLabels.Get("__name__"))
					bound, err := strconv.ParseFloat(got[0].SeriesLabels.Get("le"), 64)
					require.NoError(t, err)
					require.GreaterOrEqual(t, bound, 0.5)
					require.NotEmpty(t, c.receivedSamples[got[0].SeriesLabels.String()])
				}
			})
		}
	})
}
