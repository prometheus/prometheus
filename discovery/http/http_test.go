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
	"go.uber.org/atomic"
	"go.yaml.in/yaml/v2"

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

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:            promslog.NewNopLogger(),
		HTTPClientOptions: nil,
		Metrics:           metrics,
		SetName:           "http",
	})
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

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:            promslog.NewNopLogger(),
		HTTPClientOptions: nil,
		Metrics:           metrics,
		SetName:           "http",
	})
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

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:            promslog.NewNopLogger(),
		HTTPClientOptions: nil,
		Metrics:           metrics,
		SetName:           "http",
	})
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

func TestMinBackoffConfig(t *testing.T) {
	tests := []struct {
		name            string
		config          string
		wantMinBackoff  model.Duration
		wantErrContains string
	}{
		{
			name: "disabled by default",
			config: `url: http://example.com
refresh_interval: 1m
`,
		},
		{
			name: "configured",
			config: `url: http://example.com
refresh_interval: 1m
min_backoff: 5s
`,
			wantMinBackoff: model.Duration(5 * time.Second),
		},
		{
			name: "greater than refresh interval",
			config: `url: http://example.com
refresh_interval: 5s
min_backoff: 10s
`,
			wantErrContains: "min_backoff must not be greater than refresh_interval",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg SDConfig
			err := yaml.Unmarshal([]byte(test.config), &cfg)
			if test.wantErrContains != "" {
				require.ErrorContains(t, err, test.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantMinBackoff, cfg.MinBackoff)
		})
	}
}

func TestHTTPRetry(t *testing.T) {
	tests := []struct {
		name            string
		minBackoff      time.Duration
		statusCodes     []int
		wantAttempts    int32
		wantTargets     int
		wantErrContains string
	}{
		{
			name:       "server errors then success",
			minBackoff: time.Millisecond,
			statusCodes: []int{
				http.StatusInternalServerError,
				http.StatusBadGateway,
				http.StatusOK,
			},
			wantAttempts: 3,
			wantTargets:  1,
		},
		{
			name:       "rate limit then success",
			minBackoff: time.Millisecond,
			statusCodes: []int{
				http.StatusTooManyRequests,
				http.StatusOK,
			},
			wantAttempts: 2,
			wantTargets:  1,
		},
		{
			name: "disabled",
			statusCodes: []int{
				http.StatusInternalServerError,
				http.StatusOK,
			},
			wantAttempts:    1,
			wantErrContains: "server returned HTTP status 500 Internal Server Error",
		},
		{
			name:       "client error",
			minBackoff: time.Millisecond,
			statusCodes: []int{
				http.StatusBadRequest,
				http.StatusOK,
			},
			wantAttempts:    1,
			wantErrContains: "server returned HTTP status 400 Bad Request",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var attempts atomic.Int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempt := int(attempts.Add(1)) - 1
				statusCode := test.statusCodes[min(attempt, len(test.statusCodes)-1)]
				if statusCode == http.StatusOK {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(statusCode)
				if statusCode == http.StatusOK {
					fmt.Fprint(w, `[{"targets":["127.0.0.1:9090"]}]`)
				}
			}))
			t.Cleanup(ts.Close)

			d := newTestDiscovery(t, SDConfig{
				HTTPClientConfig: config.DefaultHTTPClientConfig,
				URL:              ts.URL,
				RefreshInterval:  model.Duration(100 * time.Millisecond),
				MinBackoff:       model.Duration(test.minBackoff),
			})
			tgs, err := d.Refresh(context.Background())
			if test.wantErrContains != "" {
				require.ErrorContains(t, err, test.wantErrContains)
				require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
			} else {
				require.NoError(t, err)
				require.Len(t, tgs, test.wantTargets)
				require.Equal(t, 0.0, getFailureCount(d.metrics.failuresCount))
			}
			require.Equal(t, test.wantAttempts, attempts.Load())
		})
	}
}

func TestHTTPRetryBudget(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	d := newTestDiscovery(t, SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(100 * time.Millisecond),
		MinBackoff:       model.Duration(5 * time.Millisecond),
	})
	start := time.Now()
	_, err := d.Refresh(context.Background())
	require.EqualError(t, err, "server returned HTTP status 500 Internal Server Error")
	require.Greater(t, attempts.Load(), int32(1))
	require.Less(t, time.Since(start), 500*time.Millisecond)
	require.Equal(t, 1.0, getFailureCount(d.metrics.failuresCount))
}

func newTestDiscovery(t *testing.T, cfg SDConfig) *Discovery {
	t.Helper()
	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	t.Cleanup(refreshMetrics.Unregister)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	t.Cleanup(metrics.Unregister)

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "http",
	})
	require.NoError(t, err)
	return d
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

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:            promslog.NewNopLogger(),
		HTTPClientOptions: nil,
		Metrics:           metrics,
		SetName:           "http",
	})
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
