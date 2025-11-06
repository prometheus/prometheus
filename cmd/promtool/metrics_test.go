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

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

// setupTestServer creates a test server with a mock storage for remote write testing.
func setupTestServer(t *testing.T, supportedTypes remoteapi.MessageTypes) (*httptest.Server, *mockStorage, *url.URL) {
	t.Helper()

	store := &mockStorage{}
	handler := remoteapi.NewWriteHandler(store, supportedTypes)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	return server, store, serverURL
}

// createMetricsFile creates a temporary file with the given metrics data.
func createMetricsFile(t *testing.T, metricsData string) string {
	t.Helper()

	tmpFile := t.TempDir() + "/metrics.txt"
	err := os.WriteFile(tmpFile, []byte(metricsData), 0o644)
	require.NoError(t, err)

	return tmpFile
}

func TestPushMetrics(t *testing.T) {
	tests := []struct {
		name        string
		metricsData string
		protoMsg    string
		serverTypes remoteapi.MessageTypes
	}{
		{
			name: "successful push with gauge metrics (V1)",
			metricsData: `# HELP test_metric A test metric
# TYPE test_metric gauge
test_metric{label="value1"} 42.0
test_metric{label="value2"} 43.0
`,
			protoMsg:    "prometheus.WriteRequest",
			serverTypes: remoteapi.MessageTypes{remoteapi.WriteV1MessageType},
		},
		{
			name: "successful push with counter metrics (V1)",
			metricsData: `# HELP test_counter A test counter
# TYPE test_counter counter
test_counter 100
`,
			protoMsg:    "prometheus.WriteRequest",
			serverTypes: remoteapi.MessageTypes{remoteapi.WriteV1MessageType},
		},
		{
			name: "successful push with gauge metrics (V2)",
			metricsData: `# HELP test_metric A test metric
# TYPE test_metric gauge
test_metric{label="value1"} 42.0
test_metric{label="value2"} 43.0
`,
			protoMsg:    "io.prometheus.write.v2.Request",
			serverTypes: remoteapi.MessageTypes{remoteapi.WriteV2MessageType},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, store, serverURL := setupTestServer(t, tc.serverTypes)
			tmpFile := createMetricsFile(t, tc.metricsData)

			status := PushMetrics(
				serverURL,
				http.DefaultTransport,
				map[string]string{},
				30*time.Second,
				tc.protoMsg,
				map[string]string{"job": "test"},
				tmpFile,
			)

			require.Equal(t, successExitCode, status)
			require.True(t, store.called, "Handler should have been called")
			require.NoError(t, store.lastErr, "Handler should not have returned an error")
			require.NotEmpty(t, store.receivedData, "Request should contain data (compression and decompression successful)")
			require.Contains(t, store.receivedContentType, "application/x-protobuf", "Content-Type should be protobuf")
		})
	}
}

// mockStorage is a simple mock for testing the remote write handler.
type mockStorage struct {
	called              bool
	lastErr             error
	receivedData        []byte
	receivedContentType string
}

func (m *mockStorage) Store(req *http.Request, _ remoteapi.WriteMessageType) (*remoteapi.WriteResponse, error) {
	m.called = true

	// Capture content-type header.
	m.receivedContentType = req.Header.Get("Content-Type")

	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		if err == nil {
			m.receivedData = data
		}
	}

	if m.lastErr != nil {
		return nil, m.lastErr
	}
	resp := remoteapi.NewWriteResponse()
	resp.SetStatusCode(http.StatusNoContent)
	return resp, nil
}

func TestPushMetricsInvalidProtoMsg(t *testing.T) {
	_, store, serverURL := setupTestServer(t, remoteapi.MessageTypes{remoteapi.WriteV1MessageType})
	tmpFile := createMetricsFile(t, "test_metric 1\n")

	status := PushMetrics(
		serverURL,
		http.DefaultTransport,
		map[string]string{},
		30*time.Second,
		"blablalba", // Invalid protoMsg.
		map[string]string{"job": "test"},
		tmpFile,
	)

	require.Equal(t, failureExitCode, status)
	require.False(t, store.called, "Handler should not have been called for invalid protoMsg")
}
