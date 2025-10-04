// Copyright 2025 The Prometheus Authors
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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPushMetrics(t *testing.T) {
	tests := []struct {
		name           string
		metricsData    string
		expectedStatus int
		serverHandler  http.HandlerFunc
	}{
		{
			name: "successful push with v1",
			metricsData: `# HELP test_metric A test metric
# TYPE test_metric gauge
test_metric{label="value1"} 42.0
test_metric{label="value2"} 43.0
`,
			expectedStatus: successExitCode,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				// Verify request headers
				require.Equal(t, "snappy", r.Header.Get("Content-Encoding"))
				require.Equal(t, "application/x-protobuf", r.Header.Get("Content-Type"))
				require.Equal(t, "0.1.0", r.Header.Get("X-Prometheus-Remote-Write-Version"))

				// Read the body to ensure it's not empty
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NotEmpty(t, body, "Request body should not be empty")

				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name: "successful push with v2 and stats",
			metricsData: `# HELP test_counter A test counter
# TYPE test_counter counter
test_counter 100
`,
			expectedStatus: successExitCode,
			serverHandler: func(w http.ResponseWriter, _ *http.Request) {
				// Set v2 response headers with statistics
				w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", "1")
				w.Header().Set("X-Prometheus-Remote-Write-Histograms-Written", "0")
				w.Header().Set("X-Prometheus-Remote-Write-Exemplars-Written", "0")
				w.WriteHeader(http.StatusNoContent)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(tc.serverHandler)
			defer server.Close()

			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			// Create a temp file with metrics data
			tmpFile := t.TempDir() + "/metrics.txt"
			err = writeFile(tmpFile, tc.metricsData)
			require.NoError(t, err)

			// Call PushMetrics
			status := PushMetrics(
				serverURL,
				http.DefaultTransport,
				map[string]string{},
				30*time.Second,
				map[string]string{"job": "test"},
				tmpFile,
			)

			require.Equal(t, tc.expectedStatus, status)
		})
	}
}

func TestParseAndPushMetrics(t *testing.T) {
	// This test verifies that parseAndPushMetrics correctly marshals and compresses data
	var requestReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		// Verify the request is properly formatted
		require.Equal(t, "snappy", r.Header.Get("Content-Encoding"))
		require.Equal(t, "application/x-protobuf", r.Header.Get("Content-Type"))

		// Read and verify body is not empty
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NotEmpty(t, body, "Marshaled and compressed data should not be empty")

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Create test metrics
	metricsData := `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1027
http_requests_total{method="POST",code="200"} 3
`

	tmpFile := t.TempDir() + "/metrics.txt"
	err = writeFile(tmpFile, metricsData)
	require.NoError(t, err)

	// Push metrics
	status := PushMetrics(
		serverURL,
		http.DefaultTransport,
		map[string]string{},
		30*time.Second,
		map[string]string{"job": "test", "instance": "localhost:9090"},
		tmpFile,
	)

	require.Equal(t, successExitCode, status)
	require.True(t, requestReceived, "Server should have received the request")
}

// Helper function to write files.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
