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

package scrape

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestManagerScrapeActiveTarget(t *testing.T) {
	type testCase struct {
		name         string
		handler      http.Handler
		cfgYAML      string
		maxBytes     int64
		timeoutCap   time.Duration
		wantErr      string
		wantContains string
	}

	setup := func(t *testing.T, tc testCase) (*Manager, string, func()) {
		t.Helper()

		listener := newPipeListener()
		srv := httptest.NewUnstartedServer(tc.handler)
		srv.Listener = listener
		srv.Start()

		opts := &Options{
			skipJitterOffsetting: true,
			HTTPClientOptions: []config_util.HTTPClientOption{
				config_util.WithDialContextFunc(func(ctx context.Context, _, _ string) (net.Conn, error) {
					srvConn, cliConn := net.Pipe()
					select {
					case listener.conns <- srvConn:
						return cliConn, nil
					case <-listener.closed:
						return nil, net.ErrClosed
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}),
			},
		}

		app := teststorage.NewAppendable()
		m, err := NewManager(opts, promslog.NewNopLogger(), nil, nil, app, prometheus.NewRegistry())
		require.NoError(t, err)

		var cfg *config.Config
		if tc.cfgYAML == "" {
			cfg = &config.Config{
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
		} else {
			var err error
			cfg, err = config.Load(tc.cfgYAML, promslog.NewNopLogger())
			require.NoError(t, err)
		}
		require.NoError(t, m.ApplyConfig(cfg))

		m.updateTsets(map[string][]*targetgroup.Group{
			"test": {{
				Targets: []model.LabelSet{{
					model.SchemeLabel:  "http",
					model.AddressLabel: "test.local",
				}},
			}},
		})
		m.reload()

		cleanup := func() {
			m.Stop()
			srv.Close()
			_ = listener.Close()
		}
		return m, "http://test.local/metrics", cleanup
	}

	tests := []testCase{
		{
			name: "success_basic_text",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/metrics", r.URL.Path)
				w.Header().Set("Content-Type", "text/plain; version=0.0.4")
				fmt.Fprintln(w, "up 1")
			}),
			maxBytes:   1024,
			timeoutCap: 2 * time.Second,
		},
		{
			name: "uses_httpclient_config_basic_auth",
			cfgYAML: `
global:
  scrape_interval: 10s
  scrape_timeout: 10s
scrape_configs:
- job_name: test
  basic_auth:
    username: user
    password: pass
  static_configs:
  - targets: ['test.local']
`,
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Basic dXNlcjpwYXNz", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "text/plain; version=0.0.4")
				fmt.Fprintln(w, "ok 1")
			}),
			maxBytes:   1024,
			timeoutCap: 2 * time.Second,
		},
		{
			name: "rejects_non_active_target",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "ignored 1")
			}),
			maxBytes:   1024,
			timeoutCap: 2 * time.Second,
			wantErr:    errTargetNotActive.Error(),
		},
		{
			name: "enforces_body_limit",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain; version=0.0.4")
				for i := 0; i < 1024; i++ {
					_, _ = w.Write([]byte("x"))
				}
			}),
			maxBytes:     16,
			timeoutCap:   2 * time.Second,
			wantContains: "body size limit exceeded",
		},
		{
			name: "enforces_timeout_cap",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-time.After(2 * time.Second):
					w.Header().Set("Content-Type", "text/plain; version=0.0.4")
					fmt.Fprintln(w, "late 1")
				case <-r.Context().Done():
					return
				}
			}),
			maxBytes:     1024,
			timeoutCap:   10 * time.Millisecond,
			wantContains: "deadline exceeded",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, scrapeURL, cleanup := setup(t, tc)
			defer cleanup()

			urlToScrape := scrapeURL
			if tc.wantErr == errTargetNotActive.Error() {
				u, err := url.Parse(scrapeURL)
				require.NoError(t, err)
				u.Path = "/not-metrics"
				urlToScrape = u.String()
			}

			ct, body, err := m.ScrapeActiveTarget(context.Background(), "test", urlToScrape, tc.maxBytes, tc.timeoutCap)
			if tc.wantErr != "" || tc.wantContains != "" {
				require.Error(t, err)
				if tc.wantErr != "" {
					require.EqualError(t, err, tc.wantErr)
				}
				if tc.wantContains != "" {
					require.Contains(t, err.Error(), tc.wantContains)
				}
				return
			}

			require.NoError(t, err)
			require.Contains(t, ct, "text/plain")
			require.NotEmpty(t, body)
		})
	}
}
