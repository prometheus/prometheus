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
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/stretchr/testify/require"
)

func TestHumaCompression(t *testing.T) {
	// Create test storage with some data.
	data := `
		load 1m
			test_metric{foo="bar"} 1+0x100
	`
	storage := promqltest.LoadedStorage(t, data)
	defer storage.Close()

	engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
		MaxSamples:               50000000,
		Timeout:                  100 * time.Second,
		NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
	})

	api := &API{
		Queryable:   storage,
		QueryEngine: engine,
		ready:       func(f http.HandlerFunc) http.HandlerFunc { return f },
	}
	api.InstallCodec(JSONCodec{})

	externalURL, _ := url.Parse("http://localhost:9090")

	// Create test server with Huma (initiateHuma returns the handler).
	handler := api.initiateHuma(&HumaOptions{Enabled: true, ExternalURL: externalURL})(nil)
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	// Test WITH gzip compression.
	t.Run("with_gzip", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/query_range?query=test_metric&start=0&end=100&step=1", nil)
		require.NoError(t, err)
		req.Header.Set("Accept-Encoding", "gzip")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Check that Content-Encoding header is set.
		encoding := resp.Header.Get("Content-Encoding")
		require.Equal(t, "gzip", encoding, "Response should be gzip compressed")

		// Decompress and verify we can read it.
		reader, err := gzip.NewReader(resp.Body)
		require.NoError(t, err)
		defer reader.Close()

		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NotEmpty(t, body)
		require.Contains(t, string(body), `"status":"success"`)
	})

	// Test WITHOUT compression.
	t.Run("without_compression", func(t *testing.T) {
		req, err := http.NewRequest("GET", server.URL+"/query_range?query=test_metric&start=0&end=100&step=1", nil)
		require.NoError(t, err)
		// Don't set Accept-Encoding header.

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Check that Content-Encoding header is NOT set.
		encoding := resp.Header.Get("Content-Encoding")
		require.Empty(t, encoding, "Response should NOT be compressed")

		// Read body directly.
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NotEmpty(t, body)
		require.Contains(t, string(body), `"status":"success"`)
	})
}
