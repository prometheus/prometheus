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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
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
		QueueConfig: config.DefaultQueueConfig,
	}
}

func TestNoDuplicateWriteConfigs(t *testing.T) {
	dir := t.TempDir()

	cfg1 := config.RemoteWriteConfig{
		Name: "write-1",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig: config.DefaultQueueConfig,
	}
	cfg2 := config.RemoteWriteConfig{
		Name: "write-2",
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig: config.DefaultQueueConfig,
	}
	cfg3 := config.RemoteWriteConfig{
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig: config.DefaultQueueConfig,
	}

	type testcase struct {
		cfgs []*config.RemoteWriteConfig
		err  bool
	}

	cases := []testcase{
		{ // Two duplicates, we should get an error.
			cfgs: []*config.RemoteWriteConfig{
				&cfg1,
				&cfg1,
			},
			err: true,
		},
		{ // Duplicates but with different names, we should not get an error.
			cfgs: []*config.RemoteWriteConfig{
				&cfg1,
				&cfg2,
			},
			err: false,
		},
		{ // Duplicates but one with no name, we should not get an error.
			cfgs: []*config.RemoteWriteConfig{
				&cfg1,
				&cfg3,
			},
			err: false,
		},
		{ // Duplicates both with no name, we should get an error.
			cfgs: []*config.RemoteWriteConfig{
				&cfg3,
				&cfg3,
			},
			err: true,
		},
	}

	for _, tc := range cases {
		s := NewWriteStorage(nil, nil, dir, time.Millisecond, nil)
		conf := &config.Config{
			GlobalConfig:       config.DefaultGlobalConfig,
			RemoteWriteConfigs: tc.cfgs,
		}
		err := s.ApplyConfig(conf)
		gotError := err != nil
		require.Equal(t, tc.err, gotError)

		err = s.Close()
		require.NoError(t, err)
	}
}

func TestRestartOnNameChange(t *testing.T) {
	dir := t.TempDir()

	cfg := testRemoteWriteConfig()

	hash, err := toHash(cfg)
	require.NoError(t, err)

	s := NewWriteStorage(nil, nil, dir, time.Millisecond, nil)

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			cfg,
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Equal(t, s.queues[hash].client().Name(), cfg.Name)

	// Change the queues name, ensure the queue has been restarted.
	conf.RemoteWriteConfigs[0].Name = "dev-2"
	require.NoError(t, s.ApplyConfig(conf))
	hash, err = toHash(cfg)
	require.NoError(t, err)
	require.Equal(t, s.queues[hash].client().Name(), conf.RemoteWriteConfigs[0].Name)

	err = s.Close()
	require.NoError(t, err)
}

func TestUpdateWithRegisterer(t *testing.T) {
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
		QueueConfig: config.DefaultQueueConfig,
	}
	c2 := &config.RemoteWriteConfig{
		URL: &common_config.URL{
			URL: &url.URL{
				Scheme: "http",
				Host:   "localhost",
			},
		},
		QueueConfig: config.DefaultQueueConfig,
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

	err := s.Close()
	require.NoError(t, err)
}

func TestWriteStorageLifecycle(t *testing.T) {
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

	err := s.Close()
	require.NoError(t, err)
}

func TestUpdateExternalLabels(t *testing.T) {
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

	err = s.Close()
	require.NoError(t, err)
}

func TestWriteStorageApplyConfigsIdempotent(t *testing.T) {
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

	err = s.Close()
	require.NoError(t, err)
}

func TestWriteStorageApplyConfigsPartialUpdate(t *testing.T) {
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
	}
	c1 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(20 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
		HTTPClientConfig: common_config.HTTPClientConfig{
			BearerToken: "foo",
		},
	}
	c2 := &config.RemoteWriteConfig{
		RemoteTimeout: model.Duration(30 * time.Second),
		QueueConfig:   config.DefaultQueueConfig,
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

	err = s.Close()
	require.NoError(t, err)
}

func TestOTLPWriteHandler(t *testing.T) {
	exportRequest := generateOTLPWriteRequest(t)

	buf, err := exportRequest.MarshalProto()
	require.NoError(t, err)

	req, err := http.NewRequest("", "", bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")

	appendable := &mockAppendable{}
	handler := NewOTLPWriteHandler(nil, appendable)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.Len(t, appendable.samples, 12)   // 1 (counter) + 1 (gauge) + 1 (target_info) + 7 (hist_bucket) + 2 (hist_sum, hist_count)
	require.Len(t, appendable.histograms, 1) // 1 (exponential histogram)
	require.Len(t, appendable.exemplars, 1)  // 1 (exemplar)
}

func generateOTLPWriteRequest(t *testing.T) pmetricotlp.ExportRequest {
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
