// Copyright 2013 The Prometheus Authors
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
	"errors"
	"fmt"
	"math/rand"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/runutil"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/prometheus/prometheus/util/testutil/synctest"
)

const defaultFlushDeadline = 1 * time.Minute

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

			var (
				series   []record.RefSeries
				metadata []record.RefMetadata
				samples  []record.RefSample
			)

			// Generates same series in both cases.
			samples, series = createTimeseries(1, 1)
			metadata = createSeriesMetadata(series)

			// Apply new config.
			queueConfig.Capacity = len(samples)
			queueConfig.MaxSamplesPerSend = len(samples)
			// For now we only ever have a single rw config in this test.
			conf.RemoteWriteConfigs[0].ProtobufMessage = tc.senderProtoMsg
			require.NoError(t, s.ApplyConfig(conf))
			hash, err := toHash(writeConfig)
			require.NoError(t, err)
			qm := s.rws.queues[hash]

			c := NewTestWriteClient(tc.receiverProtoMsg)
			c.injectErrors(tc.injectErrs)
			qm.SetClient(c)

			qm.StoreSeries(series, 0)
			qm.StoreMetadata(metadata)

			// Do we expect some data back?
			if !tc.expectFail {
				c.expectSamples(samples, series)
			} else {
				c.expectSamples(nil, nil)
			}

			// Schedule send.
			qm.Append(samples)

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
	// Let's create an even number of send batches, so we don't run into the
	// batch timeout case.
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
	for _, tc := range []struct {
		protoMsg remoteapi.WriteMessageType

		name            string
		samples         bool
		exemplars       bool
		histograms      bool
		floatHistograms bool
	}{
		{protoMsg: remoteapi.WriteV1MessageType, samples: true, exemplars: false, histograms: false, floatHistograms: false, name: "samples only"},
		{protoMsg: remoteapi.WriteV1MessageType, samples: true, exemplars: true, histograms: true, floatHistograms: true, name: "samples, exemplars, and histograms"},
		{protoMsg: remoteapi.WriteV1MessageType, samples: false, exemplars: true, histograms: false, floatHistograms: false, name: "exemplars only"},
		{protoMsg: remoteapi.WriteV1MessageType, samples: false, exemplars: false, histograms: true, floatHistograms: false, name: "histograms only"},
		{protoMsg: remoteapi.WriteV1MessageType, samples: false, exemplars: false, histograms: false, floatHistograms: true, name: "float histograms only"},

		{protoMsg: remoteapi.WriteV2MessageType, samples: true, exemplars: false, histograms: false, floatHistograms: false, name: "samples only"},
		{protoMsg: remoteapi.WriteV2MessageType, samples: true, exemplars: true, histograms: true, floatHistograms: true, name: "samples, exemplars, and histograms"},
		{protoMsg: remoteapi.WriteV2MessageType, samples: false, exemplars: true, histograms: false, floatHistograms: false, name: "exemplars only"},
		{protoMsg: remoteapi.WriteV2MessageType, samples: false, exemplars: false, histograms: true, floatHistograms: false, name: "histograms only"},
		{protoMsg: remoteapi.WriteV2MessageType, samples: false, exemplars: false, histograms: false, floatHistograms: true, name: "float histograms only"},
	} {
		t.Run(fmt.Sprintf("%s-%s", tc.protoMsg, tc.name), func(t *testing.T) {
			dir := t.TempDir()
			s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
			defer s.Close()

			var (
				series          []record.RefSeries
				metadata        []record.RefMetadata
				samples         []record.RefSample
				exemplars       []record.RefExemplar
				histograms      []record.RefHistogramSample
				floatHistograms []record.RefFloatHistogramSample
			)

			// Generates same series in both cases.
			if tc.samples {
				samples, series = createTimeseries(n, n)
			}
			if tc.exemplars {
				exemplars, series = createExemplars(n, n)
			}
			if tc.histograms {
				histograms, _, series = createHistograms(n, n, false)
			}
			if tc.floatHistograms {
				_, floatHistograms, series = createHistograms(n, n, true)
			}
			metadata = createSeriesMetadata(series)

			// Apply new config.
			queueConfig.Capacity = len(samples)
			queueConfig.MaxSamplesPerSend = len(samples) / 2
			// For now we only ever have a single rw config in this test.
			conf.RemoteWriteConfigs[0].ProtobufMessage = tc.protoMsg
			require.NoError(t, s.ApplyConfig(conf))
			hash, err := toHash(writeConfig)
			require.NoError(t, err)
			qm := s.rws.queues[hash]

			c := NewTestWriteClient(tc.protoMsg)
			qm.SetClient(c)

			qm.StoreSeries(series, 0)
			qm.StoreMetadata(metadata)

			// Send first half of data.
			c.expectSamples(samples[:len(samples)/2], series)
			c.expectExemplars(exemplars[:len(exemplars)/2], series)
			c.expectHistograms(histograms[:len(histograms)/2], series)
			c.expectFloatHistograms(floatHistograms[:len(floatHistograms)/2], series)
			if tc.protoMsg == remoteapi.WriteV2MessageType && len(metadata) > 0 {
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
			if tc.protoMsg == remoteapi.WriteV2MessageType && len(metadata) > 0 {
				c.expectMetadataForBatch(metadata, series, samples[len(samples)/2:], exemplars[len(exemplars)/2:], histograms[len(histograms)/2:], floatHistograms[len(floatHistograms)/2:])
			}
			qm.Append(samples[len(samples)/2:])
			qm.AppendExemplars(exemplars[len(exemplars)/2:])
			qm.AppendHistograms(histograms[len(histograms)/2:])
			qm.AppendFloatHistograms(floatHistograms[len(floatHistograms)/2:])
			c.waitForExpectedData(t, 30*time.Second)
		})
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
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, deadline, newPool(), newHighestTimestampMetric(), nil, false, false, false, protoMsg)

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

	num := 3
	_, series := createTimeseries(0, num)
	metadata := createSeriesMetadata(series)

	require.NoError(t, s.ApplyConfig(conf))
	hash, err := toHash(writeConfig)
	require.NoError(t, err)
	qm := s.rws.queues[hash]

	c := NewTestWriteClient(remoteapi.WriteV1MessageType)
	qm.SetClient(c)

	qm.StoreSeries(series, 0)
	qm.StoreMetadata(metadata)

	require.Len(t, qm.seriesLabels, num)
	require.Len(t, qm.seriesMetadata, num)

	c.waitForExpectedData(t, 30*time.Second)
}

func TestSampleDeliveryTimeout(t *testing.T) {
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			// Let's send one less sample than batch size, and wait the timeout duration
			n := 9
			samples, series := createTimeseries(n, n)
			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1

			c := NewTestWriteClient(protoMsg)
			m := newTestQueueManager(t, cfg, mcfg, defaultFlushDeadline, c, protoMsg)
			m.StoreSeries(series, 0)
			m.Start()
			defer m.Stop()

			// Send the samples twice, waiting for the samples in the meantime.
			c.expectSamples(samples, series)
			m.Append(samples)
			c.waitForExpectedData(t, 30*time.Second)

			c.expectSamples(samples, series)
			m.Append(samples)
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
			samples := make([]record.RefSample, 0, n)
			series := make([]record.RefSeries, 0, n)
			for i := range n {
				name := fmt.Sprintf("test_metric_%d", i%ts)
				samples = append(samples, record.RefSample{
					Ref: chunks.HeadSeriesRef(i),
					T:   int64(i),
					V:   float64(i),
				})
				series = append(series, record.RefSeries{
					Ref:    chunks.HeadSeriesRef(i),
					Labels: labels.FromStrings("__name__", name),
				})
			}

			c, m := newTestClientAndQueueManager(t, defaultFlushDeadline, protoMsg)
			c.expectSamples(samples, series)
			m.StoreSeries(series, 0)

			m.Start()
			defer m.Stop()
			// These should be received by the client.
			m.Append(samples)
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
				n := 2 * config.DefaultQueueConfig.MaxSamplesPerSend
				samples, series := createTimeseries(n, n)
				m.StoreSeries(series, 0)
				m.Start()

				// Append blocks to guarantee delivery, so we do it in the background.
				go func() {
					m.Append(samples)
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
			nSamples := config.DefaultQueueConfig.Capacity * size
			samples, series := createTimeseries(nSamples, nSeries)

			cfg := config.DefaultQueueConfig
			cfg.MaxShards = 1

			c := NewTestWriteClient(protoMsg)
			m := newTestQueueManager(t, cfg, config.DefaultMetadataConfig, defaultFlushDeadline, c, protoMsg)
			c.expectSamples(samples, series)
			m.StoreSeries(series, 0)

			m.Start()
			defer m.Stop()

			go func() {
				for i := 0; i < len(samples); i += config.DefaultQueueConfig.Capacity {
					sent := m.Append(samples[i : i+config.DefaultQueueConfig.Capacity])
					require.True(t, sent, "samples not sent")
					time.Sleep(100 * time.Millisecond)
				}
			}()

			for i := 1; i < len(samples)/config.DefaultQueueConfig.Capacity; i++ {
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

func TestReshardPartialBatch(t *testing.T) {
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			samples, series := createTimeseries(1, 10)

			c := NewTestBlockedWriteClient()

			cfg := testDefaultQueueConfig()
			mcfg := config.DefaultMetadataConfig
			cfg.MaxShards = 1
			batchSendDeadline := time.Millisecond
			flushDeadline := 10 * time.Millisecond
			cfg.BatchSendDeadline = model.Duration(batchSendDeadline)

			m := newTestQueueManager(t, cfg, mcfg, flushDeadline, c, protoMsg)
			m.StoreSeries(series, 0)

			m.Start()

			for range 100 {
				done := make(chan struct{})
				go func() {
					m.Append(samples)
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
			samples, series := createTimeseries(50, 1)

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
			m.StoreSeries(series, 0)
			m.Start()
			defer m.Stop()

			for range 100 {
				done := make(chan struct{})
				go func() {
					time.Sleep(batchSendDeadline)
					m.Append(samples)
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
		fakeSamples, fakeSeries = createTimeseries(100, 100)

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

	m := NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, client, 0, newPool(), newHighestTimestampMetric(), nil, false, false, false, remoteapi.WriteV1MessageType)
	m.StoreSeries(fakeSeries, 0)

	// Attempt to samples while the manager is running. We immediately stop the
	// manager after the recoverable error is generated to prevent the manager
	// from resharding itself.
	m.Start()
	{
		m.Append(fakeSamples)

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

func createTimeseries(numSamples, numSeries int, extraLabels ...labels.Label) ([]record.RefSample, []record.RefSeries) {
	samples := make([]record.RefSample, 0, numSamples)
	series := make([]record.RefSeries, 0, numSeries)
	lb := labels.NewScratchBuilder(1 + len(extraLabels))
	for i := range numSeries {
		name := fmt.Sprintf("test_metric_%d", i)
		for j := range numSamples {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(i),
				T:   int64(j),
				V:   float64(i),
			})
		}
		// Create Labels that is name of series plus any extra labels supplied.
		lb.Reset()
		lb.Add(labels.MetricName, name)
		rand.Shuffle(len(extraLabels), func(i, j int) {
			extraLabels[i], extraLabels[j] = extraLabels[j], extraLabels[i]
		})
		for _, l := range extraLabels {
			lb.Add(l.Name, l.Value)
		}
		lb.Sort()
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: lb.Labels(),
		})
	}
	return samples, series
}

func createProtoTimeseriesWithOld(numSamples, baseTs int64, _ ...labels.Label) []prompb.TimeSeries {
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

func createExemplars(numExemplars, numSeries int) ([]record.RefExemplar, []record.RefSeries) {
	exemplars := make([]record.RefExemplar, 0, numExemplars)
	series := make([]record.RefSeries, 0, numSeries)
	for i := range numSeries {
		name := fmt.Sprintf("test_metric_%d", i)
		for j := range numExemplars {
			e := record.RefExemplar{
				Ref:    chunks.HeadSeriesRef(i),
				T:      int64(j),
				V:      float64(i),
				Labels: labels.FromStrings("trace_id", fmt.Sprintf("trace-%d", i)),
			}
			exemplars = append(exemplars, e)
		}
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.FromStrings("__name__", name),
		})
	}
	return exemplars, series
}

func createHistograms(numSamples, numSeries int, floatHistogram bool) ([]record.RefHistogramSample, []record.RefFloatHistogramSample, []record.RefSeries) {
	histograms := make([]record.RefHistogramSample, 0, numSamples)
	floatHistograms := make([]record.RefFloatHistogramSample, 0, numSamples)
	series := make([]record.RefSeries, 0, numSeries)
	for i := range numSeries {
		name := fmt.Sprintf("test_metric_%d", i)
		for j := range numSamples {
			hist := &histogram.Histogram{
				Schema:          2,
				ZeroThreshold:   1e-128,
				ZeroCount:       0,
				Count:           2,
				Sum:             0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{int64(i) + 1},
				NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				NegativeBuckets: []int64{int64(-i) - 1},
			}

			if floatHistogram {
				fh := record.RefFloatHistogramSample{
					Ref: chunks.HeadSeriesRef(i),
					T:   int64(j),
					FH:  hist.ToFloat(nil),
				}
				floatHistograms = append(floatHistograms, fh)
			} else {
				h := record.RefHistogramSample{
					Ref: chunks.HeadSeriesRef(i),
					T:   int64(j),
					H:   hist,
				}
				histograms = append(histograms, h)
			}
		}
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.FromStrings("__name__", name),
		})
	}
	if floatHistogram {
		return nil, floatHistograms, series
	}
	return histograms, nil, series
}

func createSeriesMetadata(series []record.RefSeries) []record.RefMetadata {
	metas := make([]record.RefMetadata, 0, len(series))

	for _, s := range series {
		metas = append(metas, record.RefMetadata{
			Ref:  s.Ref,
			Type: uint8(record.Counter),
			Unit: "unit text",
			Help: "help text",
		})
	}
	return metas
}

func getSeriesIDFromRef(r record.RefSeries) string {
	return r.Labels.String()
}

// TestWriteClient represents write client which does not call remote storage,
// but instead re-implements fake WriteHandler for test purposes.
type TestWriteClient struct {
	receivedSamples         map[string][]prompb.Sample
	expectedSamples         map[string][]prompb.Sample
	receivedExemplars       map[string][]prompb.Exemplar
	expectedExemplars       map[string][]prompb.Exemplar
	receivedHistograms      map[string][]prompb.Histogram
	receivedFloatHistograms map[string][]prompb.Histogram
	expectedHistograms      map[string][]prompb.Histogram
	expectedFloatHistograms map[string][]prompb.Histogram
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
		receivedSamples:  map[string][]prompb.Sample{},
		expectedSamples:  map[string][]prompb.Sample{},
		receivedMetadata: map[string][]prompb.MetricMetadata{},
		expectedMetadata: map[string][]prompb.MetricMetadata{},
		protoMsg:         protoMsg,
		storeWait:        0,
		returnError:      nil,
	}
}

func (c *TestWriteClient) injectErrors(injectedErrs []error) {
	c.injectedErrs = injectedErrs
	c.currErr = -1
	c.retry = false
}

func (c *TestWriteClient) expectSamples(ss []record.RefSample, series []record.RefSeries) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedSamples = map[string][]prompb.Sample{}
	c.receivedSamples = map[string][]prompb.Sample{}

	for _, s := range ss {
		tsID := getSeriesIDFromRef(series[s.Ref])
		c.expectedSamples[tsID] = append(c.expectedSamples[tsID], prompb.Sample{
			Timestamp: s.T,
			Value:     s.V,
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

	c.expectedHistograms = map[string][]prompb.Histogram{}
	c.receivedHistograms = map[string][]prompb.Histogram{}

	for _, h := range hh {
		tsID := getSeriesIDFromRef(series[h.Ref])
		c.expectedHistograms[tsID] = append(c.expectedHistograms[tsID], prompb.FromIntHistogram(h.T, h.H))
	}
}

func (c *TestWriteClient) expectFloatHistograms(fhs []record.RefFloatHistogramSample, series []record.RefSeries) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedFloatHistograms = map[string][]prompb.Histogram{}
	c.receivedFloatHistograms = map[string][]prompb.Histogram{}

	for _, fh := range fhs {
		tsID := getSeriesIDFromRef(series[fh.Ref])
		c.expectedFloatHistograms[tsID] = append(c.expectedFloatHistograms[tsID], prompb.FromFloatHistogram(fh.T, fh.FH))
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

	var reqProto *prompb.WriteRequest
	switch c.protoMsg {
	case remoteapi.WriteV1MessageType:
		reqProto = &prompb.WriteRequest{}
		err = proto.Unmarshal(reqBuf, reqProto)
	case remoteapi.WriteV2MessageType:
		// NOTE(bwplotka): v1 msg can be unmarshaled to v2 sometimes, without
		// errors.
		var reqProtoV2 writev2.Request
		err = proto.Unmarshal(reqBuf, &reqProtoV2)
		if err == nil {
			reqProto, err = v2RequestToWriteRequest(&reqProtoV2)
		}
	}
	if err != nil {
		return WriteResponseStats{}, err
	}

	rs := WriteResponseStats{}
	b := labels.NewScratchBuilder(0)
	for _, ts := range reqProto.Timeseries {
		labels := ts.ToLabels(&b, nil)
		tsID := labels.String()
		if len(ts.Samples) > 0 {
			c.receivedSamples[tsID] = append(c.receivedSamples[tsID], ts.Samples...)
		}
		rs.Samples += len(ts.Samples)

		if len(ts.Exemplars) > 0 {
			c.receivedExemplars[tsID] = append(c.receivedExemplars[tsID], ts.Exemplars...)
		}
		rs.Exemplars += len(ts.Exemplars)

		for _, h := range ts.Histograms {
			if h.IsFloatHistogram() {
				c.receivedFloatHistograms[tsID] = append(c.receivedFloatHistograms[tsID], h)
			} else {
				c.receivedHistograms[tsID] = append(c.receivedHistograms[tsID], h)
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
			metadata := rts.ToMetadata(v2Req.Symbols)

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

func BenchmarkSampleSend(b *testing.B) {
	// Send one sample per series, which is the typical remote_write case
	const numSamples = 1
	const numSeries = 10000

	samples, series := createTimeseries(numSamples, numSeries, extraLabels...)

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
			m.StoreSeries(series, 0)

			// These should be received by the client.
			m.Start()
			defer m.Stop()

			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				m.Append(samples)
				m.UpdateSeriesSegment(series, i+1) // simulate what wlog.Watcher.garbageCollectSeries does
				m.SeriesReset(i + 1)
			}
			// Do not include shutdown
			b.StopTimer()
		})
	}
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
	_, series := createTimeseries(0, numSeries, extraLabels...)

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				c := NewTestWriteClient(remoteapi.WriteV1MessageType)
				dir := b.TempDir()
				cfg := config.DefaultQueueConfig
				mcfg := config.DefaultMetadataConfig
				metrics := newQueueManagerMetrics(nil, "", "")

				m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false, false, remoteapi.WriteV1MessageType)
				m.externalLabels = tc.externalLabels
				m.relabelConfigs = tc.relabelConfigs

				m.StoreSeries(series, 0)
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
		"namespace", "some-namespace")

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
			b.ReportMetric(float64(totalSize)/float64(b.N), "compressedSize/op")
		}
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
			req, _, _, err := buildV2WriteRequest(noopLogger, seriesBuff, symbolTable.Symbols(), &pBuf, nil, cEnc, "snappy")
			if err != nil {
				b.Fatal(err)
			}
			totalSize += len(req)
			b.ReportMetric(float64(totalSize)/float64(b.N), "compressedSize/op")
		}
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
			samples, newSamples, series := createTimeseriesWithOldSamples(nSamples, nSeries)

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
	t.Parallel()
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			maxSamplesPerSend := 10
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
			expectedSamples := map[string][]prompb.Sample{}

			appendData := func(numberOfSeries int, timeAdd time.Duration, shouldBeDropped bool) {
				t.Log(">>>>  Appending series ", numberOfSeries, " as batch ID ", batchID, " with timeAdd ", timeAdd, " and should be dropped ", shouldBeDropped)
				samples, series := createTimeseriesWithRandomLabelCount(strconv.Itoa(batchID), numberOfSeries, timeAdd, 9)
				m.StoreSeries(series, batchID)
				sent := m.Append(samples)
				require.True(t, sent, "samples not sent")
				if !shouldBeDropped {
					for _, s := range samples {
						tsID := getSeriesIDFromRef(series[s.Ref])
						expectedSamples[tsID] = append(c.expectedSamples[tsID], prompb.Sample{
							Timestamp: s.T,
							Value:     s.V,
						})
					}
				}
				batchID++
			}
			timeShift := -time.Millisecond * 5

			c.SetReturnError(RecoverableError{context.DeadlineExceeded, defaultBackoff})

			appendData(maxSamplesPerSend/2, timeShift, true)
			time.Sleep(sampleAgeLimit)
			appendData(maxSamplesPerSend/2, timeShift, true)
			time.Sleep(sampleAgeLimit / 10)
			appendData(maxSamplesPerSend/2, timeShift, true)
			time.Sleep(2 * sampleAgeLimit)
			appendData(2*maxSamplesPerSend, timeShift, false)
			time.Sleep(sampleAgeLimit / 2)
			c.SetReturnError(nil)
			appendData(5, timeShift, false)
			m.Stop()

			if diff := cmp.Diff(expectedSamples, c.receivedSamples); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func createTimeseriesWithRandomLabelCount(id string, seriesCount int, timeAdd time.Duration, maxLabels int) ([]record.RefSample, []record.RefSeries) {
	samples := []record.RefSample{}
	series := []record.RefSeries{}
	// use a fixed rand source so tests are consistent
	r := rand.New(rand.NewSource(99))
	for i := range seriesCount {
		s := record.RefSample{
			Ref: chunks.HeadSeriesRef(i),
			T:   time.Now().Add(timeAdd).UnixMilli(),
			V:   r.Float64(),
		}
		samples = append(samples, s)
		labelsCount := r.Intn(maxLabels)
		lb := labels.NewScratchBuilder(1 + labelsCount)
		lb.Add("__name__", "batch_"+id+"_id_"+strconv.Itoa(i))
		for j := 1; j < labelsCount+1; j++ {
			// same for both name and value
			label := "batch_" + id + "_label_" + strconv.Itoa(j)
			lb.Add(label, label)
		}
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: lb.Labels(),
		})
	}
	return samples, series
}

func createTimeseriesWithOldSamples(numSamples, numSeries int, extraLabels ...labels.Label) ([]record.RefSample, []record.RefSample, []record.RefSeries) {
	newSamples := make([]record.RefSample, 0, numSamples)
	samples := make([]record.RefSample, 0, numSamples)
	series := make([]record.RefSeries, 0, numSeries)
	lb := labels.NewScratchBuilder(1 + len(extraLabels))
	for i := range numSeries {
		name := fmt.Sprintf("test_metric_%d", i)
		// We create half of the samples in the past.
		past := timestamp.FromTime(time.Now().Add(-5 * time.Minute))
		for j := 0; j < numSamples/2; j++ {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(i),
				T:   past + int64(j),
				V:   float64(i),
			})
		}
		for j := 0; j < numSamples/2; j++ {
			sample := record.RefSample{
				Ref: chunks.HeadSeriesRef(i),
				T:   time.Now().UnixMilli() + int64(j),
				V:   float64(i),
			}
			samples = append(samples, sample)
			newSamples = append(newSamples, sample)
		}
		// Create Labels that is name of series plus any extra labels supplied.
		lb.Reset()
		lb.Add(labels.MetricName, name)
		for _, l := range extraLabels {
			lb.Add(l.Name, l.Value)
		}
		lb.Sort()
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: lb.Labels(),
		})
	}
	return samples, newSamples, series
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
	for b.Loop() {
		samples := createProtoTimeseriesWithOld(numSamples, 100, extraLabels...)
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
		&symbolTable, batch, pendingData, false, false, false)

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
			samples, series := createTimeseries(nSamples, nSeries)

			_, m := newTestClientAndQueueManager(t, defaultFlushDeadline, protoMsg)
			m.Start()
			defer m.Stop()

			require.Equal(t, 0.0, m.metrics.highestTimestamp.Get())

			m.StoreSeries(series, 0)
			require.True(t, m.Append(samples))

			// Check that Append sets the highest timestamp correctly.
			highestTs := float64((nSamples - 1) / 1000)
			require.Greater(t, highestTs, 0.0)
			require.Equal(t, highestTs, m.metrics.highestTimestamp.Get())
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
