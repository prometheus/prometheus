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

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	promconfig "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"
)

// TestCheckServerStatusWithBasicAuth verifies health check works with basic auth.
func TestCheckServerStatusWithBasicAuth(t *testing.T) {
	// Create a test HTTP server that requires basic auth
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "alice" || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Create HTTP config file with basic auth
	configContent := `basic_auth:
  username: alice
  password: secret
`
	configFile := filepath.Join(t.TempDir(), "http-config.yml")
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

	// Load HTTP config
	httpConfig, _, err := promconfig.LoadHTTPConfigFile(configFile)
	require.NoError(t, err)

	// Create round tripper from config
	httpRoundTripper, err := promconfig.NewRoundTripperFromConfig(*httpConfig, "promtool", promconfig.WithUserAgent("test"))
	require.NoError(t, err)

	// Parse server URL
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Check server status with auth
	err = CheckServerStatus(serverURL, "/-/healthy", httpRoundTripper)
	require.NoError(t, err, "health check should succeed with correct basic auth")
}

// TestCheckServerStatusWithoutAuth verifies health check fails without auth.
func TestCheckServerStatusWithoutAuth(t *testing.T) {
	// Create a test HTTP server that requires basic auth
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "alice" || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Parse server URL
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Check server status without auth
	err = CheckServerStatus(serverURL, "/-/healthy", http.DefaultClient.Transport)
	require.Error(t, err, "health check should fail without auth")
	require.ErrorContains(t, err, "status=401")
}
