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

package config

import (
	"testing"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

// TestGlobalScrapeHTTPClientConfigInheritance verifies that scrape jobs inherit
// unset HTTP client settings from the global scrape_http_client_config.
func TestGlobalScrapeHTTPClientConfigInheritance(t *testing.T) {
	const in = `
global:
  scrape_http_client_config:
    tls_config:
      insecure_skip_verify: true
    basic_auth:
      username: globaluser
      password: globalpass
    http_headers:
      X-Global:
        values: [global-value]
scrape_configs:
  - job_name: inherits
    static_configs:
      - targets: ["localhost:9090"]
`
	cfg, err := Load(in, promslog.NewNopLogger())
	require.NoError(t, err)
	require.NotNil(t, cfg.GlobalConfig.ScrapeHTTPClientConfig)

	hc := cfg.ScrapeConfigs[0].HTTPClientConfig
	require.True(t, hc.TLSConfig.InsecureSkipVerify, "tls_config should be inherited")
	require.NotNil(t, hc.BasicAuth)
	require.Equal(t, "globaluser", hc.BasicAuth.Username)
	require.Equal(t, config.Secret("globalpass"), hc.BasicAuth.Password)
	require.NotNil(t, hc.HTTPHeaders)
	require.Equal(t, []string{"global-value"}, hc.HTTPHeaders.Headers["X-Global"].Values)
	// follow_redirects and enable_http2 are not inherited and keep their
	// per-scrape-config defaults (true).
	require.True(t, hc.FollowRedirects)
	require.True(t, hc.EnableHTTP2)
}

// TestGlobalScrapeHTTPClientConfigOverride verifies that per-job settings take
// precedence over the global scrape HTTP client config and that authentication
// is inherited as an atomic group.
func TestGlobalScrapeHTTPClientConfigOverride(t *testing.T) {
	const in = `
global:
  scrape_http_client_config:
    basic_auth:
      username: globaluser
      password: globalpass
scrape_configs:
  - job_name: overrides-auth
    authorization:
      credentials: jobtoken
    static_configs:
      - targets: ["localhost:9090"]
`
	cfg, err := Load(in, promslog.NewNopLogger())
	require.NoError(t, err)

	hc := cfg.ScrapeConfigs[0].HTTPClientConfig
	// The job configures its own authorization, so it must NOT inherit the
	// global basic_auth, which would be mutually exclusive.
	require.Nil(t, hc.BasicAuth)
	require.NotNil(t, hc.Authorization)
	require.Equal(t, config.Secret("jobtoken"), hc.Authorization.Credentials)
}

// TestGlobalScrapeHTTPClientConfigFollowRedirectsNotInherited verifies that
// follow_redirects and enable_http2 are not inherited from the global scrape
// HTTP client config and keep their per-scrape-config defaults.
func TestGlobalScrapeHTTPClientConfigFollowRedirectsNotInherited(t *testing.T) {
	const in = `
global:
  scrape_http_client_config:
    tls_config:
      insecure_skip_verify: true
scrape_configs:
  - job_name: default
    static_configs:
      - targets: ["localhost:9090"]
`
	cfg, err := Load(in, promslog.NewNopLogger())
	require.NoError(t, err)

	hc := cfg.ScrapeConfigs[0].HTTPClientConfig
	require.True(t, hc.TLSConfig.InsecureSkipVerify, "tls_config should still be inherited")
	// follow_redirects and enable_http2 keep their own defaults (true), not
	// inherited from the global block.
	require.Equal(t, config.DefaultHTTPClientConfig.FollowRedirects, hc.FollowRedirects)
	require.Equal(t, config.DefaultHTTPClientConfig.EnableHTTP2, hc.EnableHTTP2)
}

// TestGlobalScrapeHTTPClientConfigRejectsUnsupportedBooleans verifies that
// setting follow_redirects or enable_http2 to false in the global scrape HTTP
// client config is rejected at load time, since those cannot be inherited.
func TestGlobalScrapeHTTPClientConfigRejectsUnsupportedBooleans(t *testing.T) {
	for _, field := range []string{"follow_redirects", "enable_http2"} {
		in := `
global:
  scrape_http_client_config:
    ` + field + `: false
scrape_configs:
  - job_name: default
    static_configs:
      - targets: ["localhost:9090"]
`
		_, err := Load(in, promslog.NewNopLogger())
		require.Error(t, err, "expected error for global %s: false", field)
		require.Contains(t, err.Error(), field)
	}
}

// TestGlobalScrapeHTTPClientConfigProxyGroup verifies that proxy settings are
// inherited as an atomic group.
func TestGlobalScrapeHTTPClientConfigProxyGroup(t *testing.T) {
	const in = `
global:
  scrape_http_client_config:
    proxy_url: http://global.proxy:8080
scrape_configs:
  - job_name: inherits-proxy
    static_configs:
      - targets: ["localhost:9090"]
  - job_name: own-proxy
    proxy_from_environment: true
    static_configs:
      - targets: ["localhost:9091"]
`
	cfg, err := Load(in, promslog.NewNopLogger())
	require.NoError(t, err)

	inherit := cfg.ScrapeConfigs[0].HTTPClientConfig
	require.NotNil(t, inherit.ProxyURL.URL)
	require.Equal(t, "http://global.proxy:8080", inherit.ProxyURL.String())

	own := cfg.ScrapeConfigs[1].HTTPClientConfig
	require.True(t, own.ProxyFromEnvironment)
	require.Nil(t, own.ProxyURL.URL, "job with its own proxy setting should not inherit global proxy_url")
}

// TestGlobalScrapeHTTPClientConfigNil verifies that scrape jobs are unaffected
// when no global scrape HTTP client config is set.
func TestGlobalScrapeHTTPClientConfigNil(t *testing.T) {
	const in = `
scrape_configs:
  - job_name: default
    static_configs:
      - targets: ["localhost:9090"]
`
	cfg, err := Load(in, promslog.NewNopLogger())
	require.NoError(t, err)
	require.Nil(t, cfg.GlobalConfig.ScrapeHTTPClientConfig)

	hc := cfg.ScrapeConfigs[0].HTTPClientConfig
	require.Equal(t, config.DefaultHTTPClientConfig.FollowRedirects, hc.FollowRedirects)
	require.Equal(t, config.DefaultHTTPClientConfig.EnableHTTP2, hc.EnableHTTP2)
	require.Nil(t, hc.BasicAuth)
}

// TestGlobalScrapeHTTPClientConfigSetDirectory verifies that inherited file
// paths are resolved exactly once per scrape config, independently of the
// global template and of other scrape jobs that inherit the same settings.
func TestGlobalScrapeHTTPClientConfigSetDirectory(t *testing.T) {
	const in = `
global:
  scrape_http_client_config:
    basic_auth:
      username: u
      password_file: secrets/pw
    http_headers:
      X-Token:
        files: [secrets/token]
scrape_configs:
  - job_name: a
    static_configs:
      - targets: ["localhost:1"]
  - job_name: b
    static_configs:
      - targets: ["localhost:2"]
`
	cfg, err := Load(in, promslog.NewNopLogger())
	require.NoError(t, err)

	cfg.SetDirectory("conf")

	for _, sc := range cfg.ScrapeConfigs {
		hc := sc.HTTPClientConfig
		require.Equal(t, "conf/secrets/pw", hc.BasicAuth.PasswordFile, "job %q", sc.JobName)
		require.Equal(t, []string{"conf/secrets/token"}, hc.HTTPHeaders.Headers["X-Token"].Files, "job %q", sc.JobName)
	}

	// The global template is resolved once and independently.
	require.Equal(t, "conf/secrets/pw", cfg.GlobalConfig.ScrapeHTTPClientConfig.BasicAuth.PasswordFile)
}

// TestGlobalScrapeHTTPClientConfigInvalid verifies that an invalid global scrape
// HTTP client config is rejected at load time.
func TestGlobalScrapeHTTPClientConfigInvalid(t *testing.T) {
	const in = `
global:
  scrape_http_client_config:
    basic_auth:
      username: u
      password: p
    bearer_token: token
scrape_configs:
  - job_name: default
    static_configs:
      - targets: ["localhost:9090"]
`
	_, err := Load(in, promslog.NewNopLogger())
	require.Error(t, err)
}
