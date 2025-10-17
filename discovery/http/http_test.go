// Copyright 2021 The Prometheus Authors
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

package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestHTTPValidRefresh(t *testing.T) {
	ts := httptest.NewServer(http.FileServer(http.Dir("./fixtures")))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL + "/http_sd.good.json",
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.Refresh(ctx)
	require.NoError(t, err)

	expectedTargets := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel: model.LabelValue("127.0.0.1:9090"),
				},
			},
			Labels: model.LabelSet{
				model.LabelName("__meta_datacenter"): model.LabelValue("bru1"),
				model.LabelName("__meta_url"):        model.LabelValue(ts.URL + "/http_sd.good.json"),
			},
			Source: urlSource(ts.URL+"/http_sd.good.json", 0),
		},
	}
	require.Equal(t, expectedTargets, tgs)
	require.Equal(t, 0.0, getFailureCount(d.metrics.failuresCount))
}

func TestHTTPInvalidCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.Refresh(ctx)
	require.EqualError(t, err, "server returned HTTP status 400 Bad Request")
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func TestHTTPInvalidFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "{}")
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.Refresh(ctx)
	require.EqualError(t, err, `unsupported content type "text/plain; charset=utf-8"`)
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func getFailureCount(failuresCount prometheus.Counter) float64 {
	failureChan := make(chan prometheus.Metric)

	go func() {
		failuresCount.Collect(failureChan)
		close(failureChan)
	}()

	var counter dto.Metric
	for {
		metric, ok := <-failureChan
		if ok == false {
			break
		}
		metric.Write(&counter)
	}

	return *counter.Counter.Value
}

func TestContentTypeRegex(t *testing.T) {
	cases := []struct {
		header string
		match  bool
	}{
		{
			header: "application/json;charset=utf-8",
			match:  true,
		},
		{
			header: "application/json;charset=UTF-8",
			match:  true,
		},
		{
			header: "Application/JSON;Charset=\"utf-8\"",
			match:  true,
		},
		{
			header: "application/json; charset=\"utf-8\"",
			match:  true,
		},
		{
			header: "application/json",
			match:  true,
		},
		{
			header: "application/jsonl; charset=\"utf-8\"",
			match:  false,
		},
		{
			header: "application/json;charset=UTF-9",
			match:  false,
		},
		{
			header: "application /json;charset=UTF-8",
			match:  false,
		},
		{
			header: "application/ json;charset=UTF-8",
			match:  false,
		},
		{
			header: "application/json;",
			match:  false,
		},
		{
			header: "charset=UTF-8",
			match:  false,
		},
	}

	for _, test := range cases {
		t.Run(test.header, func(t *testing.T) {
			require.Equal(t, test.match, matchContentType.MatchString(test.header))
		})
	}
}

func TestRetryOnRetryableErrors(t *testing.T) {
	var attemptCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			// Return retryable error for first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{"labels": {"k": "v"}, "targets": ["127.0.0.1"]}]`)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		MinBackoff:       model.Duration(10 * time.Millisecond),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.Refresh(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, attemptCount)
	require.Len(t, tgs, 1)

	// Verify retry metrics were incremented
	httpMetrics := metrics.(*httpMetrics)
	retryMetric, err := httpMetrics.retriesCount.GetMetricWithLabelValues("1")
	require.NoError(t, err)
	var m dto.Metric
	require.NoError(t, retryMetric.Write(&m))
	require.Equal(t, 1.0, *m.Counter.Value)

	retryMetric2, err := httpMetrics.retriesCount.GetMetricWithLabelValues("2")
	require.NoError(t, err)
	require.NoError(t, retryMetric2.Write(&m))
	require.Equal(t, 1.0, *m.Counter.Value)
}

func TestRetryExhaustion(t *testing.T) {
	var attemptCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(200 * time.Millisecond),
		MinBackoff:       model.Duration(10 * time.Millisecond),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.Refresh(ctx)
	require.Error(t, err)
	// Should timeout after refresh_interval (200ms)
	require.Contains(t, err.Error(), "timeout")
	// Should have made multiple attempts within the timeout
	require.Greater(t, attemptCount, 1)
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func TestNoRetryOnNonRetryableErrors(t *testing.T) {
	var attemptCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusBadRequest) // 4xx is not retryable
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		MinBackoff:       model.Duration(10 * time.Millisecond),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.Refresh(ctx)
	require.Error(t, err)
	require.EqualError(t, err, "server returned HTTP status 400 Bad Request")
	require.Equal(t, 1, attemptCount) // Should only attempt once
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func TestRetryOn429RateLimit(t *testing.T) {
	var attemptCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{"labels": {"k": "v"}, "targets": ["127.0.0.1"]}]`)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		MinBackoff:       model.Duration(10 * time.Millisecond),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.Refresh(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, attemptCount)
	require.Len(t, tgs, 1)
}

func TestRetryContextCancellation(t *testing.T) {
	var attemptCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		MinBackoff:       model.Duration(100 * time.Millisecond),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel context after a short delay
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	_, err = d.Refresh(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	// Should have attempted at least once
	require.Positive(t, attemptCount)
}

func TestSlowHTTPRequestTimeout(t *testing.T) {
	requestStarted := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		// Simulate a slow/hanging request by waiting for context cancellation
		<-r.Context().Done()
		// Request was cancelled by timeout
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(300 * time.Millisecond),
		MinBackoff:       model.Duration(50 * time.Millisecond),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	start := time.Now()
	_, err = d.Refresh(ctx)
	elapsed := time.Since(start)

	require.Error(t, err)
	// Should timeout after refresh_interval (300ms)
	require.Greater(t, elapsed, 250*time.Millisecond, "should wait at least close to refresh_interval")
	require.Less(t, elapsed, 500*time.Millisecond, "should not wait much longer than refresh_interval")
	require.Contains(t, err.Error(), "timeout")
	// Verify request was started
	select {
	case <-requestStarted:
	default:
		t.Fatal("HTTP request was never started")
	}
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func TestDefaultMinBackoff(t *testing.T) {
	// Test that default min_backoff is 0 (retries disabled)
	require.Equal(t, model.Duration(0), DefaultSDConfig.MinBackoff)
}

func TestMinBackoffValidation(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectedErr string
	}{
		{
			name: "valid config with min_backoff",
			yaml: `
url: http://example.com
refresh_interval: 60s
min_backoff: 1s
`,
			expectedErr: "",
		},
		{
			name: "min_backoff zero disables retries",
			yaml: `
url: http://example.com
refresh_interval: 60s
min_backoff: 0
`,
			expectedErr: "",
		},
		{
			name: "negative min_backoff is invalid",
			yaml: `
url: http://example.com
refresh_interval: 60s
min_backoff: -1s
`,
			expectedErr: "not a valid duration string",
		},
		{
			name: "min_backoff greater than refresh_interval",
			yaml: `
url: http://example.com
refresh_interval: 30s
min_backoff: 60s
`,
			expectedErr: "min_backoff must not be greater than refresh_interval",
		},
		{
			name: "missing URL",
			yaml: `
refresh_interval: 60s
`,
			expectedErr: "URL is missing",
		},
		{
			name: "invalid URL scheme",
			yaml: `
url: ftp://example.com
refresh_interval: 60s
`,
			expectedErr: "URL scheme must be 'http' or 'https'",
		},
		{
			name: "missing host in URL",
			yaml: `
url: http://
refresh_interval: 60s
`,
			expectedErr: "host is missing in URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg SDConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)

			if tt.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNoRetryWhenMinBackoffIsZero(t *testing.T) {
	var attemptCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		MinBackoff:       model.Duration(0), // Retries disabled
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.Refresh(ctx)
	require.Error(t, err)
	require.Equal(t, 1, attemptCount) // Should only attempt once
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func TestUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		validate func(*testing.T, SDConfig, error)
	}{
		{
			name: "apply defaults when min_backoff not specified",
			yaml: `
url: http://example.com
refresh_interval: 60s
`,
			validate: func(t *testing.T, cfg SDConfig, err error) {
				require.NoError(t, err)
				require.Equal(t, model.Duration(0), cfg.MinBackoff)
			},
		},
		{
			name: "preserve user-specified min_backoff",
			yaml: `
url: http://example.com
refresh_interval: 60s
min_backoff: 5s
`,
			validate: func(t *testing.T, cfg SDConfig, err error) {
				require.NoError(t, err)
				require.Equal(t, model.Duration(5*time.Second), cfg.MinBackoff)
			},
		},
		{
			name: "https URL is valid",
			yaml: `
url: https://example.com
refresh_interval: 60s
`,
			validate: func(t *testing.T, cfg SDConfig, err error) {
				require.NoError(t, err)
				require.Equal(t, "https://example.com", cfg.URL)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg SDConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
			tt.validate(t, cfg, err)
		})
	}
}

func TestSourceDisappeared(t *testing.T) {
	var stubResponse string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, stubResponse)
	}))
	t.Cleanup(ts.Close)

	cases := []struct {
		responses       []string
		expectedTargets [][]*targetgroup.Group
	}{
		{
			responses: []string{
				`[]`,
				`[]`,
			},
			expectedTargets: [][]*targetgroup.Group{{}, {}},
		},
		{
			responses: []string{
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"]}]`,
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"]}, {"labels": {"k": "2"}, "targets": ["127.0.0.1"]}]`,
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("1"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
				},
				{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("1"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("2"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
				},
			},
		},
		{
			responses: []string{
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"]}, {"labels": {"k": "2"}, "targets": ["127.0.0.1"]}]`,
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"]}]`,
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("1"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("2"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
				},
				{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("1"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: nil,
						Labels:  nil,
						Source:  urlSource(ts.URL, 1),
					},
				},
			},
		},
		{
			responses: []string{
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"]}, {"labels": {"k": "2"}, "targets": ["127.0.0.1"]}, {"labels": {"k": "3"}, "targets": ["127.0.0.1"]}]`,
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"]}]`,
				`[{"labels": {"k": "v"}, "targets": ["127.0.0.2"]}, {"labels": {"k": "vv"}, "targets": ["127.0.0.3"]}]`,
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("1"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("2"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("3"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 2),
					},
				},
				{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.1"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("1"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: nil,
						Labels:  nil,
						Source:  urlSource(ts.URL, 1),
					},
					{
						Targets: nil,
						Labels:  nil,
						Source:  urlSource(ts.URL, 2),
					},
				},
				{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.2"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("v"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue("127.0.0.3"),
							},
						},
						Labels: model.LabelSet{
							model.LabelName("k"):          model.LabelValue("vv"),
							model.LabelName("__meta_url"): model.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
				},
			},
		},
	}

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(1 * time.Second),
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)
	for _, test := range cases {
		ctx := context.Background()
		for i, res := range test.responses {
			stubResponse = res
			tgs, err := d.Refresh(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expectedTargets[i], tgs)
		}
	}
}
