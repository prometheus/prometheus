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
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/stretchr/testify/require"
)

func TestHTTPValidRefresh(t *testing.T) {
	ts := httptest.NewServer(http.FileServer(http.Dir("./fixtures")))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL + "/http_sd.good.json",
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
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

	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.EqualError(t, err, "server returned HTTP status 400 Bad Request")
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

	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.EqualError(t, err, `unsupported content type "text/plain; charset=utf-8"`)
}
