// Copyright 2017 The Prometheus Authors
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
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage"
)

func testRemoteWriteConfig() *config.RemoteWriteConfig {
	return &config.RemoteWriteConfig{
		Name: "dev",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}
}

func TestWriteStorageApplyConfig_NoDuplicateWriteConfigs(t *testing.T) {
	dir := t.TempDir()

	cfg1 := config.RemoteWriteConfig{
		Name: "write-1",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}
	cfg2 := config.RemoteWriteConfig{
		Name: "write-2",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}
	cfg3 := config.RemoteWriteConfig{
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}

	for _, tc := range []struct {
		cfgs        []*config.RemoteWriteConfig
		expectedErr error
	}{
		{ // Two duplicates, we should get an error.
			cfgs:        []*config.RemoteWriteConfig{&cfg1, &cfg1},
			expectedErr: errors.New("duplicate remote write configs are not allowed, found duplicate for URL: http://localhost"),
		},
		{ // Duplicates but with different names, we should not get an error.
			cfgs: []*config.RemoteWriteConfig{&cfg1, &cfg2},
		},
		{ // Duplicates but one with no name, we should not get an error.
			cfgs: []*config.RemoteWriteConfig{&cfg1, &cfg3},
		},
		{ // Duplicates both with no name, we should get an error.
			cfgs:        []*config.RemoteWriteConfig{&cfg3, &cfg3},
			expectedErr: errors.New("duplicate remote write configs are not allowed, found duplicate for URL: http://localhost"),
		},
	} {
		t.Run("", func(t *testing.T) {
			s := NewWriteStorage(nil, nil, dir, time.Millisecond, nil)
			conf := &config.Config{
				GlobalConfig:       config.DefaultGlobalConfig,
				RemoteWriteConfigs: tc.cfgs,
			}
			err := s.ApplyConfig(conf)
			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Equal(t, tc.expectedErr, err)
			}

			require.NoError(t, s.Close())
		})
	}
}

func TestWriteStorageApplyConfig_RestartOnNameChange(t *testing.T) {
	dir := t.TempDir()

	cfg := testRemoteWriteConfig()

	hash, err := toHash(cfg)
	require.NoError(t, err)

	s := NewWriteStorage(nil, nil, dir, time.Millisecond, nil)

	conf := &config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, s.queues[hash].client().Name(), cfg.Name)

	// Change the queues name, ensure the queue has been restarted.
	conf.RemoteWriteConfigs[0].Name = "dev-2"
	require.NoError(t, s.ApplyConfig(conf))
	hash, err = toHash(cfg)
	require.NoError(t, err)
	require.Equal(t, s.queues[hash].client().Name(), conf.RemoteWriteConfigs[0].Name)

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_UpdateWithRegisterer(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, prometheus.NewRegistry(), dir, time.Millisecond, nil)
	c1 := &config.RemoteWriteConfig{
		Name: "named",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}
	c2 := &config.RemoteWriteConfig{
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}
	conf := &config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c1, c2},
	}
	require.NoError(t, s.ApplyConfig(conf))

	c1.QueueConfig.MaxShards = 10
	c2.QueueConfig.MaxShards = 10
	require.NoError(t, s.ApplyConfig(conf))
	for _, queue := range s.queues {
		require.Equal(t, 10, queue.cfg.MaxShards)
	}

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_Lifecycle(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			baseRemoteWriteConfig("http://test-storage.com"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_UpdateExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, prometheus.NewRegistry(), dir, time.Second, nil)

	externalLabels := labels.FromStrings("external", "true")
	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			testRemoteWriteConfig(),
		},
	}
	hash, err := toHash(conf.RemoteWriteConfigs[0])
	require.NoError(t, err)
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)
	require.Empty(t, s.queues[hash].externalLabels)

	conf.GlobalConfig.ExternalLabels = externalLabels
	hash, err = toHash(conf.RemoteWriteConfigs[0])
	require.NoError(t, err)
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)
	require.Equal(t, []labels.Label{{Name: "external", Value: "true"}}, s.queues[hash].externalLabels)

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_Idempotent(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil)
	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			baseRemoteWriteConfig("http://test-storage.com"),
		},
	}
	hash, err := toHash(conf.RemoteWriteConfigs[0])
	require.NoError(t, err)

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 1)
	_, hashExists := s.queues[hash]
	require.True(t, hashExists, "Queue pointer should have remained the same")

	require.NoError(t, s.Close())
}

func TestWriteStorageApplyConfig_PartialUpdate(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil)

	c0 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(10 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
		WriteRelabelConfigs: []*relabel.Config{
			{
				Regex: relabel.MustNewRegexp(".+"),
			},
		},
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}
	c1 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(20 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
		HTTPClientConfig: common_config.HTTPClientConfig{
			BearerToken: "foo",
		},
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}
	c2 := &config.RemoteWriteConfig{
		RemoteTimeout:   model.Duration(30 * time.Second),
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: config.RemoteWriteProtoMsgV1,
	}

	conf := &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c0, c1, c2},
	}
	// We need to set URL's so that metric creation doesn't panic.
	for i := range conf.RemoteWriteConfigs {
		conf.RemoteWriteConfigs[i].URL = &common_config.URL{
			URL: &url.URL{
				Host: "http://test-storage.com",
			},
		}
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 3)

	hashes := make([]string, len(conf.RemoteWriteConfigs))
	queues := make([]*QueueManager, len(conf.RemoteWriteConfigs))
	storeHashes := func() {
		for i := range conf.RemoteWriteConfigs {
			hash, err := toHash(conf.RemoteWriteConfigs[i])
			require.NoError(t, err)
			hashes[i] = hash
			queues[i] = s.queues[hash]
		}
	}

	storeHashes()
	// Update c0 and c2.
	c0.WriteRelabelConfigs[0] = &relabel.Config{Regex: relabel.MustNewRegexp("foo")}
	c2.RemoteTimeout = model.Duration(50 * time.Second)
	conf = &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c0, c1, c2},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 3)

	_, hashExists := s.queues[hashes[0]]
	require.False(t, hashExists, "The queue for the first remote write configuration should have been restarted because the relabel configuration has changed.")
	q, hashExists := s.queues[hashes[1]]
	require.True(t, hashExists, "Hash of unchanged queue should have remained the same")
	require.Equal(t, q, queues[1], "Pointer of unchanged queue should have remained the same")
	_, hashExists = s.queues[hashes[2]]
	require.False(t, hashExists, "The queue for the third remote write configuration should have been restarted because the timeout has changed.")

	storeHashes()
	secondClient := s.queues[hashes[1]].client()
	// Update c1.
	c1.HTTPClientConfig.BearerToken = "bar"
	err := s.ApplyConfig(conf)
	require.NoError(t, err)
	require.Len(t, s.queues, 3)

	_, hashExists = s.queues[hashes[0]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")
	q, hashExists = s.queues[hashes[1]]
	require.True(t, hashExists, "Hash of queue with secret change should have remained the same")
	require.NotEqual(t, secondClient, q.client(), "Pointer of a client with a secret change should not be the same")
	_, hashExists = s.queues[hashes[2]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")

	storeHashes()
	// Delete c0.
	conf = &config.Config{
		GlobalConfig:       config.GlobalConfig{},
		RemoteWriteConfigs: []*config.RemoteWriteConfig{c1, c2},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queues, 2)

	_, hashExists = s.queues[hashes[0]]
	require.False(t, hashExists, "If a config is removed, the queue should be stopped and recreated.")
	_, hashExists = s.queues[hashes[1]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")
	_, hashExists = s.queues[hashes[2]]
	require.True(t, hashExists, "Pointer of unchanged queue should have remained the same")

	require.NoError(t, s.Close())
}

func TestOTLPWriteHandler(t *testing.T) {
	exportRequest := generateOTLPWriteRequest()

	buf, err := exportRequest.MarshalProto()
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")

	appendable := &mockAppendable{}
	handler := NewOTLPWriteHandler(nil, nil, appendable, func() config.Config {
		return config.Config{
			OTLPConfig: config.DefaultOTLPConfig,
		}
	}, OTLPOptions{})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Len(t, appendable.samples, 12)   // 1 (counter) + 1 (gauge) + 1 (target_info) + 7 (hist_bucket) + 2 (hist_sum, hist_count)
	require.Len(t, appendable.histograms, 1) // 1 (exponential histogram)
	require.Len(t, appendable.exemplars, 1)  // 1 (exemplar)
}

func generateOTLPWriteRequest() pmetricotlp.ExportRequest {
	d := pmetric.NewMetrics()

	// Generate One Counter, One Gauge, One Histogram, One Exponential-Histogram
	// with resource attributes: service.name="test-service", service.instance.id="test-instance", host.name="test-host"
	// with metric attribute: foo.bar="baz"

	timestamp := time.Now()

	resourceMetric := d.ResourceMetrics().AppendEmpty()
	resourceMetric.Resource().Attributes().PutStr("service.name", "test-service")
	resourceMetric.Resource().Attributes().PutStr("service.instance.id", "test-instance")
	resourceMetric.Resource().Attributes().PutStr("host.name", "test-host")

	scopeMetric := resourceMetric.ScopeMetrics().AppendEmpty()

	// Generate One Counter
	counterMetric := scopeMetric.Metrics().AppendEmpty()
	counterMetric.SetName("test-counter")
	counterMetric.SetDescription("test-counter-description")
	counterMetric.SetEmptySum()
	counterMetric.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	counterMetric.Sum().SetIsMonotonic(true)

	counterDataPoint := counterMetric.Sum().DataPoints().AppendEmpty()
	counterDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	counterDataPoint.SetDoubleValue(10.0)
	counterDataPoint.Attributes().PutStr("foo.bar", "baz")

	counterExemplar := counterDataPoint.Exemplars().AppendEmpty()

	counterExemplar.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	counterExemplar.SetDoubleValue(10.0)
	counterExemplar.SetSpanID(pcommon.SpanID{0, 1, 2, 3, 4, 5, 6, 7})
	counterExemplar.SetTraceID(pcommon.TraceID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})

	// Generate One Gauge
	gaugeMetric := scopeMetric.Metrics().AppendEmpty()
	gaugeMetric.SetName("test-gauge")
	gaugeMetric.SetDescription("test-gauge-description")
	gaugeMetric.SetEmptyGauge()

	gaugeDataPoint := gaugeMetric.Gauge().DataPoints().AppendEmpty()
	gaugeDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	gaugeDataPoint.SetDoubleValue(10.0)
	gaugeDataPoint.Attributes().PutStr("foo.bar", "baz")

	// Generate One Histogram
	histogramMetric := scopeMetric.Metrics().AppendEmpty()
	histogramMetric.SetName("test-histogram")
	histogramMetric.SetDescription("test-histogram-description")
	histogramMetric.SetEmptyHistogram()
	histogramMetric.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	histogramDataPoint := histogramMetric.Histogram().DataPoints().AppendEmpty()
	histogramDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	histogramDataPoint.ExplicitBounds().FromRaw([]float64{0.0, 1.0, 2.0, 3.0, 4.0, 5.0})
	histogramDataPoint.BucketCounts().FromRaw([]uint64{2, 2, 2, 2, 2, 2})
	histogramDataPoint.SetCount(10)
	histogramDataPoint.SetSum(30.0)
	histogramDataPoint.Attributes().PutStr("foo.bar", "baz")

	// Generate One Exponential-Histogram
	exponentialHistogramMetric := scopeMetric.Metrics().AppendEmpty()
	exponentialHistogramMetric.SetName("test-exponential-histogram")
	exponentialHistogramMetric.SetDescription("test-exponential-histogram-description")
	exponentialHistogramMetric.SetEmptyExponentialHistogram()
	exponentialHistogramMetric.ExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	exponentialHistogramDataPoint := exponentialHistogramMetric.ExponentialHistogram().DataPoints().AppendEmpty()
	exponentialHistogramDataPoint.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
	exponentialHistogramDataPoint.SetScale(2.0)
	exponentialHistogramDataPoint.Positive().BucketCounts().FromRaw([]uint64{2, 2, 2, 2, 2})
	exponentialHistogramDataPoint.SetZeroCount(2)
	exponentialHistogramDataPoint.SetCount(10)
	exponentialHistogramDataPoint.SetSum(30.0)
	exponentialHistogramDataPoint.Attributes().PutStr("foo.bar", "baz")

	return pmetricotlp.NewExportRequestFromMetrics(d)
}

func TestOTLPDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	appendable := &mockAppendable{}
	cfg := func() config.Config {
		return config.Config{OTLPConfig: config.DefaultOTLPConfig}
	}
	handler := NewOTLPWriteHandler(log, nil, appendable, cfg, OTLPOptions{ConvertDelta: true})

	md := pmetric.NewMetrics()
	ms := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()

	m := ms.AppendEmpty()
	m.SetName("some.delta.total")

	sum := m.SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	ts := time.Date(2000, 1, 2, 3, 4, 0, 0, time.UTC)
	for i := range 3 {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetIntValue(int64(i))
		dp.SetTimestamp(pcommon.NewTimestampFromTime(ts.Add(time.Duration(i) * time.Second)))
	}

	proto, err := pmetricotlp.NewExportRequestFromMetrics(md).MarshalProto()
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(proto))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Result().StatusCode)

	ls := labels.FromStrings("__name__", "some_delta_total")
	milli := func(sec int) int64 {
		return time.Date(2000, 1, 2, 3, 4, sec, 0, time.UTC).UnixMilli()
	}

	want := []mockSample{
		{t: milli(0), l: ls, v: 0}, // +0
		{t: milli(1), l: ls, v: 1}, // +1
		{t: milli(2), l: ls, v: 3}, // +2
	}
	if diff := cmp.Diff(want, appendable.samples, cmp.Exporter(func(_ reflect.Type) bool { return true })); diff != "" {
		t.Fatal(diff)
	}
}

func BenchmarkOTLP(b *testing.B) {
	start := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)

	type Type struct {
		name string
		data func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric
	}
	types := []Type{{
		name: "sum",
		data: func() func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
			cumul := make(map[int]float64)
			return func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
				m := pmetric.NewMetric()
				sum := m.SetEmptySum()
				sum.SetAggregationTemporality(mode)
				dps := sum.DataPoints()
				for id := range dpc {
					dp := dps.AppendEmpty()
					dp.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
					dp.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(epoch) * time.Minute)))
					dp.Attributes().PutStr("id", strconv.Itoa(id))
					v := float64(rand.IntN(100)) / 10
					switch mode {
					case pmetric.AggregationTemporalityDelta:
						dp.SetDoubleValue(v)
					case pmetric.AggregationTemporalityCumulative:
						cumul[id] += v
						dp.SetDoubleValue(cumul[id])
					}
				}
				return []pmetric.Metric{m}
			}
		}(),
	}, {
		name: "histogram",
		data: func() func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
			bounds := [4]float64{1, 10, 100, 1000}
			type state struct {
				counts [4]uint64
				count  uint64
				sum    float64
			}
			var cumul []state
			return func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
				if cumul == nil {
					cumul = make([]state, dpc)
				}
				m := pmetric.NewMetric()
				hist := m.SetEmptyHistogram()
				hist.SetAggregationTemporality(mode)
				dps := hist.DataPoints()
				for id := range dpc {
					dp := dps.AppendEmpty()
					dp.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
					dp.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(epoch) * time.Minute)))
					dp.Attributes().PutStr("id", strconv.Itoa(id))
					dp.ExplicitBounds().FromRaw(bounds[:])

					var obs *state
					switch mode {
					case pmetric.AggregationTemporalityDelta:
						obs = new(state)
					case pmetric.AggregationTemporalityCumulative:
						obs = &cumul[id]
					}

					for i := range obs.counts {
						v := uint64(rand.IntN(10))
						obs.counts[i] += v
						obs.count++
						obs.sum += float64(v)
					}

					dp.SetCount(obs.count)
					dp.SetSum(obs.sum)
					dp.BucketCounts().FromRaw(obs.counts[:])
				}
				return []pmetric.Metric{m}
			}
		}(),
	}, {
		name: "exponential",
		data: func() func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
			type state struct {
				counts [4]uint64
				count  uint64
				sum    float64
			}
			var cumul []state
			return func(mode pmetric.AggregationTemporality, dpc, epoch int) []pmetric.Metric {
				if cumul == nil {
					cumul = make([]state, dpc)
				}
				m := pmetric.NewMetric()
				ex := m.SetEmptyExponentialHistogram()
				ex.SetAggregationTemporality(mode)
				dps := ex.DataPoints()
				for id := range dpc {
					dp := dps.AppendEmpty()
					dp.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
					dp.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(epoch) * time.Minute)))
					dp.Attributes().PutStr("id", strconv.Itoa(id))
					dp.SetScale(2)

					var obs *state
					switch mode {
					case pmetric.AggregationTemporalityDelta:
						obs = new(state)
					case pmetric.AggregationTemporalityCumulative:
						obs = &cumul[id]
					}

					for i := range obs.counts {
						v := uint64(rand.IntN(10))
						obs.counts[i] += v
						obs.count++
						obs.sum += float64(v)
					}

					dp.Positive().BucketCounts().FromRaw(obs.counts[:])
					dp.SetCount(obs.count)
					dp.SetSum(obs.sum)
				}

				return []pmetric.Metric{m}
			}
		}(),
	}}

	modes := []struct {
		name string
		data func(func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, int) []pmetric.Metric
	}{{
		name: "cumulative",
		data: func(data func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, epoch int) []pmetric.Metric {
			return data(pmetric.AggregationTemporalityCumulative, 10, epoch)
		},
	}, {
		name: "delta",
		data: func(data func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, epoch int) []pmetric.Metric {
			return data(pmetric.AggregationTemporalityDelta, 10, epoch)
		},
	}, {
		name: "mixed",
		data: func(data func(pmetric.AggregationTemporality, int, int) []pmetric.Metric, epoch int) []pmetric.Metric {
			cumul := data(pmetric.AggregationTemporalityCumulative, 5, epoch)
			delta := data(pmetric.AggregationTemporalityDelta, 5, epoch)
			out := append(cumul, delta...)
			rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
			return out
		},
	}}

	configs := []struct {
		name string
		opts OTLPOptions
	}{
		{name: "default"},
		{name: "convert", opts: OTLPOptions{ConvertDelta: true}},
	}

	Workers := runtime.GOMAXPROCS(0)
	for _, cs := range types {
		for _, mode := range modes {
			for _, cfg := range configs {
				b.Run(fmt.Sprintf("type=%s/temporality=%s/cfg=%s", cs.name, mode.name, cfg.name), func(b *testing.B) {
					if !cfg.opts.ConvertDelta && (mode.name == "delta" || mode.name == "mixed") {
						b.Skip("not possible")
					}

					var total int

					// reqs is a [b.N]*http.Request, divided across the workers.
					// deltatocumulative requires timestamps to be strictly in
					// order on a per-series basis. to ensure this, each reqs[k]
					// contains samples of differently named series, sorted
					// strictly in time order
					reqs := make([][]*http.Request, Workers)
					for n := range b.N {
						k := n % Workers

						md := pmetric.NewMetrics()
						ms := md.ResourceMetrics().AppendEmpty().
							ScopeMetrics().AppendEmpty().
							Metrics()

						for i, m := range mode.data(cs.data, n) {
							m.SetName(fmt.Sprintf("benchmark_%d_%d", k, i))
							m.MoveTo(ms.AppendEmpty())
						}

						total += sampleCount(md)

						ex := pmetricotlp.NewExportRequestFromMetrics(md)
						data, err := ex.MarshalProto()
						require.NoError(b, err)

						req, err := http.NewRequest("", "", bytes.NewReader(data))
						require.NoError(b, err)
						req.Header.Set("Content-Type", "application/x-protobuf")

						reqs[k] = append(reqs[k], req)
					}

					log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
					mock := new(mockAppendable)
					appendable := syncAppendable{Appendable: mock, lock: new(sync.Mutex)}
					cfgfn := func() config.Config {
						return config.Config{OTLPConfig: config.DefaultOTLPConfig}
					}
					handler := NewOTLPWriteHandler(log, nil, appendable, cfgfn, cfg.opts)

					fail := make(chan struct{})
					done := make(chan struct{})

					b.ResetTimer()
					b.ReportAllocs()

					// we use multiple workers to mimic a real-world scenario
					// where multiple OTel collectors are sending their
					// time-series in parallel.
					// this is necessary to exercise potential lock-contention
					// in this benchmark
					for k := range Workers {
						go func() {
							rec := httptest.NewRecorder()
							for _, req := range reqs[k] {
								handler.ServeHTTP(rec, req)
								if rec.Result().StatusCode != http.StatusOK {
									fail <- struct{}{}
									return
								}
							}
							done <- struct{}{}
						}()
					}

					for range Workers {
						select {
						case <-fail:
							b.FailNow()
						case <-done:
						}
					}

					require.Equal(b, total, len(mock.samples)+len(mock.histograms))
				})
			}
		}
	}
}

func sampleCount(md pmetric.Metrics) int {
	var total int
	rms := md.ResourceMetrics()
	for i := range rms.Len() {
		sms := rms.At(i).ScopeMetrics()
		for i := range sms.Len() {
			ms := sms.At(i).Metrics()
			for i := range ms.Len() {
				m := ms.At(i)
				switch m.Type() {
				case pmetric.MetricTypeSum:
					total += m.Sum().DataPoints().Len()
				case pmetric.MetricTypeGauge:
					total += m.Gauge().DataPoints().Len()
				case pmetric.MetricTypeHistogram:
					dps := m.Histogram().DataPoints()
					for i := range dps.Len() {
						total += dps.At(i).BucketCounts().Len()
						total++ // le=+Inf series
						total++ // _sum series
						total++ // _count series
					}
				case pmetric.MetricTypeExponentialHistogram:
					total += m.ExponentialHistogram().DataPoints().Len()
				case pmetric.MetricTypeSummary:
					total += m.Summary().DataPoints().Len()
				}
			}
		}
	}
	return total
}

type syncAppendable struct {
	lock sync.Locker
	storage.Appendable
}

type syncAppender struct {
	lock sync.Locker
	storage.Appender
}

func (s syncAppendable) Appender(ctx context.Context) storage.Appender {
	return syncAppender{Appender: s.Appendable.Appender(ctx), lock: s.lock}
}

func (s syncAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.Appender.Append(ref, l, t, v)
}

func (s syncAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, f *histogram.FloatHistogram) (storage.SeriesRef, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.Appender.AppendHistogram(ref, l, t, h, f)
}
