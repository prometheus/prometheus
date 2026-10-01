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

func TestCheckServerStatusWithBasicAuth(t *testing.T) {
	for _, tc := range []struct {
		name      string
		config    string
		wantError bool
	}{
		{name: "with credentials", config: "basic_auth:\n  username: alice\n  password: secret\n"},
		{name: "without credentials", config: "{}", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()
				if !ok || username != "alice" || password != "secret" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			configFile := filepath.Join(t.TempDir(), "http-config.yml")
			require.NoError(t, os.WriteFile(configFile, []byte(tc.config), 0o600))
			httpConfig, _, err := promconfig.LoadHTTPConfigFile(configFile)
			require.NoError(t, err)
			httpRoundTripper, err := promconfig.NewRoundTripperFromConfig(*httpConfig, "promtool", promconfig.WithUserAgent("test"))
			require.NoError(t, err)
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			err = CheckServerStatus(serverURL, "/-/healthy", httpRoundTripper)
			if tc.wantError {
				require.ErrorContains(t, err, "status=401")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
