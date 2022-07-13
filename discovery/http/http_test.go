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

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

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

	d, err := NewDiscovery(&cfg, log.NewNopLogger(), nil)
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
	require.Equal(t, tgs, expectedTargets)
	require.Equal(t, 0.0, getFailureCount())
}

func TestHTTPInvalidCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, log.NewNopLogger(), nil)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.Refresh(ctx)
	require.EqualError(t, err, "server returned HTTP status 400 Bad Request")
	require.Equal(t, 1.0, getFailureCount())
}

func TestHTTPInvalidFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "{}")
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, log.NewNopLogger(), nil)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.Refresh(ctx)
	require.EqualError(t, err, `unsupported content type "text/plain; charset=utf-8"`)
	require.Equal(t, 1.0, getFailureCount())
}

var lastFailureCount float64

func getFailureCount() float64 {
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

	// account for failures in prior tests
	count := *counter.Counter.Value - lastFailureCount
	lastFailureCount = *counter.Counter.Value
	return count
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	d, err := NewDiscovery(&cfg, log.NewNopLogger(), nil)
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
