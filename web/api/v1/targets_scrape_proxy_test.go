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

package v1

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/util/features"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestTargetsScrapeProxyEndpoint(t *testing.T) {
	var handlerDelayNs atomic.Int64
	metricsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d := handlerDelayNs.Load(); d > 0 {
			select {
			case <-time.After(time.Duration(d)):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintln(w, "probe 1")
	})

	srv := httptest.NewServer(metricsHandler)
	t.Cleanup(srv.Close)
	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)

	opts := &scrape.Options{DiscoveryReloadInterval: model.Duration(10 * time.Millisecond)}
	app := teststorage.NewAppendable()
	m, err := scrape.NewManager(opts, promslog.NewNopLogger(), nil, nil, app, prometheus.NewRegistry())
	require.NoError(t, err)
	t.Cleanup(m.Stop)

	cfg := &config.Config{
		GlobalConfig: config.GlobalConfig{
			ScrapeInterval:  model.Duration(10 * time.Second),
			ScrapeTimeout:   model.Duration(10 * time.Second),
			ScrapeProtocols: []config.ScrapeProtocol{config.PrometheusText1_0_0},
		},
		ScrapeConfigs: []*config.ScrapeConfig{{JobName: "test"}},
	}
	b, err := yaml.Marshal(*cfg)
	require.NoError(t, err)
	cfg, err = config.Load(string(b), promslog.NewNopLogger())
	require.NoError(t, err)
	require.NoError(t, m.ApplyConfig(cfg))

	tsetsCh := make(chan map[string][]*targetgroup.Group)
	go func() {
		_ = m.Run(tsetsCh)
	}()
	t.Cleanup(func() { close(tsetsCh) })

	tsetsCh <- map[string][]*targetgroup.Group{
		"test": {{
			Targets: []model.LabelSet{{
				model.SchemeLabel:  model.LabelValue(srvURL.Scheme),
				model.AddressLabel: model.LabelValue(srvURL.Host),
			}},
		}},
	}

	var activeScrapeURL string
	require.Eventually(t, func() bool {
		ta := m.TargetsActive()
		targets := ta["test"]
		if len(targets) == 0 {
			return false
		}
		activeScrapeURL = targets[0].URL().String()
		return activeScrapeURL != ""
	}, 2*time.Second, 10*time.Millisecond)

	api := &API{
		CORSOrigin:        nil,
		featureRegistry:   features.NewRegistry(),
		targetRetriever:   func(context.Context) TargetRetriever { return m },
		ready:             func(f http.HandlerFunc) http.HandlerFunc { return f },
		overrideErrorCode: nil,
	}

	makeReq := func(q url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/targets/scrape?"+q.Encode(), http.NoBody)
		w := httptest.NewRecorder()
		api.targetScrapeProxy(w, req)
		return w
	}

	t.Run("disabled_returns_404", func(t *testing.T) {
		w := makeReq(url.Values{
			"scrapePool": {"test"},
			"scrapeUrl":  {activeScrapeURL},
		})
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("enabled_missing_params_returns_400", func(t *testing.T) {
		api.featureRegistry.Set(features.API, "target_scrape_proxy", true)
		w := makeReq(url.Values{})
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("enabled_non_active_returns_422", func(t *testing.T) {
		api.featureRegistry.Set(features.API, "target_scrape_proxy", true)
		w := makeReq(url.Values{
			"scrapePool": {"test"},
			"scrapeUrl":  {srvURL.String() + "/not-metrics"},
		})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("enabled_success_returns_metrics", func(t *testing.T) {
		api.featureRegistry.Set(features.API, "target_scrape_proxy", true)
		w := makeReq(url.Values{
			"scrapePool": {"test"},
			"scrapeUrl":  {activeScrapeURL},
		})
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "probe 1")
	})

	t.Run("enabled_timeout_enforced", func(t *testing.T) {
		api.featureRegistry.Set(features.API, "target_scrape_proxy", true)
		handlerDelayNs.Store(int64(100 * time.Millisecond))
		t.Cleanup(func() { handlerDelayNs.Store(0) })
		w := makeReq(url.Values{
			"scrapePool": {"test"},
			"scrapeUrl":  {activeScrapeURL},
			"timeout":    {"1ms"},
		})
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, w.Body.String(), "deadline exceeded")
	})
}
