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

func TestRetryOnServerError(t *testing.T) {
	attempt := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt++
		if attempt < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{"targets": ["127.0.0.1:9090"]}]`)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		RetryConfig: RetryConfig{
			MaxAttempts:     5,
			InitialInterval: model.Duration(10 * time.Millisecond),
			MaxInterval:     model.Duration(100 * time.Millisecond),
			Multiplier:      2.0,
		},
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
	require.Len(t, tgs, 1)
	require.Equal(t, 3, attempt)
	require.Equal(t, 0.0, getFailureCount(d.metrics.failuresCount))
}

func TestRetryExhausted(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		RetryConfig: RetryConfig{
			MaxAttempts:     3,
			InitialInterval: model.Duration(10 * time.Millisecond),
			MaxInterval:     model.Duration(100 * time.Millisecond),
			Multiplier:      2.0,
		},
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
	require.Contains(t, err.Error(), "all 3 attempts failed")
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

// TestNoRetryOn4xxErrors tests that the HTTP SD does not retry on 4xx client errors.
func TestNoRetryOn4xxErrors(t *testing.T) {
	attempt := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt++
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		RetryConfig: RetryConfig{
			MaxAttempts:     5,
			InitialInterval: model.Duration(10 * time.Millisecond),
			MaxInterval:     model.Duration(100 * time.Millisecond),
			Multiplier:      2.0,
		},
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
	require.Contains(t, err.Error(), "404 Not Found")
	// Should not retry on 4xx errors, so only one attempt
	require.Equal(t, 1, attempt)
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func TestRetryOn429RateLimit(t *testing.T) {
	attempt := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt++
		if attempt < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[{"targets": ["127.0.0.1:9090"]}]`)
	}))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
		RetryConfig: RetryConfig{
			MaxAttempts:     5,
			InitialInterval: model.Duration(10 * time.Millisecond),
			MaxInterval:     model.Duration(100 * time.Millisecond),
			Multiplier:      2.0,
		},
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
	require.Len(t, tgs, 1)
	require.Equal(t, 2, attempt)
}

func TestDefaultRetryConfig(t *testing.T) {
	ts := httptest.NewServer(http.FileServer(http.Dir("./fixtures")))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL + "/http_sd.good.json",
		RefreshInterval:  model.Duration(30 * time.Second),
		// RetryConfig not specified, should use defaults
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	defer refreshMetrics.Unregister()
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()

	d, err := NewDiscovery(&cfg, promslog.NewNopLogger(), nil, metrics)
	require.NoError(t, err)

	// Verify default retry config was applied
	require.Equal(t, 3, d.retryConfig.MaxAttempts)
	require.Equal(t, model.Duration(1*time.Second), d.retryConfig.InitialInterval)
	require.Equal(t, model.Duration(30*time.Second), d.retryConfig.MaxInterval)
	require.Equal(t, 2.0, d.retryConfig.Multiplier)

	ctx := context.Background()
	tgs, err := d.Refresh(ctx)
	require.NoError(t, err)
	require.Len(t, tgs, 1)
}

func TestIsRetryableStatusCode(t *testing.T) {
	cases := []struct {
		statusCode int
		retryable  bool
	}{
		{http.StatusOK, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("status_%d", tc.statusCode), func(t *testing.T) {
			require.Equal(t, tc.retryable, isRetryableStatusCode(tc.statusCode))
		})
	}
}
