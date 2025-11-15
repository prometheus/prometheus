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
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var updateOpenAPIGolden = flag.Bool("openapi.update", false, "Update golden OpenAPI spec file")

// startPrometheusWithHuma starts a Prometheus server with Huma enabled and returns its base URL.
func startPrometheusWithHuma(t *testing.T) (string, *http.Client) {
	t.Helper()

	configFile := filepath.Join(t.TempDir(), "prometheus.yml")
	configText := `
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
`
	err := os.WriteFile(configFile, []byte(configText), 0o644)
	require.NoError(t, err)

	port := testutil.RandomUnprivilegedPort(t)
	storagePath := t.TempDir()

	prom := prometheusCommandWithLogging(t, configFile, port,
		"--enable-feature=openapi-huma",
		"--storage.tsdb.path="+storagePath,
		"--web.enable-lifecycle",
		"--web.external-url=http://localhost:9090",
	)

	err = prom.Start()
	require.NoError(t, err, "Failed to start Prometheus")

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: 5 * time.Second}

	require.Eventually(t, func() bool {
		resp, err := client.Get(baseURL + "/-/ready")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond, "Prometheus did not become ready")

	return baseURL, client
}

// loadOpenAPIValidator loads the OpenAPI spec from the golden file and creates a validator.
func loadOpenAPIValidator(t *testing.T) validator.Validator {
	t.Helper()

	goldenFile := filepath.Join("testdata", "openapi_golden.yaml")
	specBytes, err := os.ReadFile(goldenFile)
	require.NoError(t, err)

	doc, err := libopenapi.NewDocument(specBytes)
	require.NoError(t, err)

	_, errs := doc.BuildV3Model()
	require.Empty(t, errs)

	docValidator, validatorErrs := validator.NewValidator(doc)
	require.Empty(t, validatorErrs)

	return docValidator
}

// validateHTTPResponse validates an HTTP response against the OpenAPI spec.
func validateHTTPResponse(t *testing.T, docValidator validator.Validator, req *http.Request, resp *http.Response, body []byte) {
	t.Helper()

	// Restore the response body since it was already consumed by io.ReadAll.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	valid, validationErrs := docValidator.ValidateHttpResponse(req, resp)
	if !valid {
		for _, vErr := range validationErrs {
			t.Logf("Validation error: %v", vErr)
		}
		t.Logf("Response body: %s", string(body))
		require.Fail(t, "Response does not match OpenAPI spec", "Found %d validation errors", len(validationErrs))
	}
}

// buildRequest creates an HTTP request with query parameters.
// According to Prometheus API docs, both GET and POST use query parameters.
func buildRequest(method, url string, queryParams map[string]string) (*http.Request, error) {
	if len(queryParams) > 0 {
		var params []string
		for key, value := range queryParams {
			params = append(params, fmt.Sprintf("%s=%s", key, value))
		}
		url = url + "?" + strings.Join(params, "&")
	}
	return http.NewRequest(method, url, nil)
}

// TestOpenAPISpecRetrieval tests that the OpenAPI spec can be retrieved from the golden file.
func TestOpenAPISpecRetrieval(t *testing.T) {
	goldenFile := filepath.Join("testdata", "openapi_golden.yaml")

	specBytes, err := os.ReadFile(goldenFile)
	require.NoError(t, err)
	require.NotEmpty(t, specBytes)

	var spec map[string]interface{}
	err = yaml.Unmarshal(specBytes, &spec)
	require.NoError(t, err)
	require.Contains(t, spec, "openapi")
	require.Contains(t, spec, "paths")
}

// TestOpenAPIGoldenFile tests that the OpenAPI spec matches the golden file and is valid.
func TestOpenAPIGoldenFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURL, client := startPrometheusWithHuma(t)

	resp, err := client.Get(baseURL + "/api/v1/openapi.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	specBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	goldenFile := filepath.Join("testdata", "openapi_golden.yaml")

	if *updateOpenAPIGolden {
		err := os.MkdirAll(filepath.Dir(goldenFile), 0755)
		require.NoError(t, err)

		err = os.WriteFile(goldenFile, specBytes, 0644)
		require.NoError(t, err)
		t.Logf("Updated golden file: %s", goldenFile)
		return
	}

	goldenBytes, err := os.ReadFile(goldenFile)
	if os.IsNotExist(err) {
		t.Fatalf("Golden file does not exist: %s\nRun with -openapi.update to create it", goldenFile)
	}
	require.NoError(t, err)

	require.Equal(t, string(goldenBytes), string(specBytes),
		"Generated OpenAPI spec does not match golden file.\nRun with -openapi.update to update the golden file.")

	doc, err := libopenapi.NewDocument(goldenBytes)
	require.NoError(t, err, "OpenAPI document should parse without errors")

	model, errs := doc.BuildV3Model()
	require.Empty(t, errs, "OpenAPI document should build without errors: %v", errs)
	require.NotNil(t, model)

	require.NotNil(t, model.Model.Info)
	require.Equal(t, "Prometheus API", model.Model.Info.Title)
	require.NotNil(t, model.Model.Paths)
	require.NotNil(t, model.Model.Paths.PathItems)

	endpointTests := []struct {
		path   string
		method string
	}{
		{path: "/metadata", method: http.MethodGet},
		{path: "/query_range", method: http.MethodGet},
		{path: "/query_range", method: http.MethodPost},
	}

	for _, tc := range endpointTests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			pathItem, ok := model.Model.Paths.PathItems.Get(tc.path)
			require.True(t, ok, "Should contain %s endpoint", tc.path)
			require.NotNil(t, pathItem)

			var operation interface{}
			switch tc.method {
			case http.MethodGet:
				operation = pathItem.Get
			case http.MethodPost:
				operation = pathItem.Post
			case http.MethodPut:
				operation = pathItem.Put
			case http.MethodDelete:
				operation = pathItem.Delete
			case http.MethodPatch:
				operation = pathItem.Patch
			}
			require.NotNil(t, operation, "Should have %s method on %s", tc.method, tc.path)
		})
	}
}

// TestOpenAPIQueryRangeValidation tests that actual query_range responses validate against the OpenAPI spec.
func TestOpenAPIQueryRangeValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURL, client := startPrometheusWithHuma(t)
	docValidator := loadOpenAPIValidator(t)

	// Test cases for query_range endpoint.
	testCases := []struct {
		name     string
		method   string
		query    string
		start    string
		end      string
		step     string
		wantErr  bool
		skipBody bool // Skip body validation for error responses.
	}{
		{
			name:   "GET: Valid query with Prometheus duration",
			method: http.MethodGet,
			query:  "vector(1)",
			start:  fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			end:    fmt.Sprintf("%d", time.Now().Unix()),
			step:   "1m",
		},
		{
			name:   "GET: Valid query with float step",
			method: http.MethodGet,
			query:  "vector(1)",
			start:  fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			end:    fmt.Sprintf("%d", time.Now().Unix()),
			step:   "60",
		},
		{
			name:     "GET: Invalid step (negative)",
			method:   http.MethodGet,
			query:    "up",
			start:    fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			end:      fmt.Sprintf("%d", time.Now().Unix()),
			step:     "-1m",
			wantErr:  true,
			skipBody: true,
		},
		{
			name:     "GET: Invalid time range (end before start)",
			method:   http.MethodGet,
			query:    "up",
			start:    fmt.Sprintf("%d", time.Now().Unix()),
			end:      fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			step:     "1m",
			wantErr:  true,
			skipBody: true,
		},
		{
			name:   "POST: Valid query with Prometheus duration",
			method: http.MethodPost,
			query:  "vector(1)",
			start:  fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			end:    fmt.Sprintf("%d", time.Now().Unix()),
			step:   "1m",
		},
		{
			name:   "POST: Valid query with float step",
			method: http.MethodPost,
			query:  "vector(1)",
			start:  fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			end:    fmt.Sprintf("%d", time.Now().Unix()),
			step:   "60",
		},
		{
			name:     "POST: Invalid step (negative)",
			method:   http.MethodPost,
			query:    "up",
			start:    fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			end:      fmt.Sprintf("%d", time.Now().Unix()),
			step:     "-1m",
			wantErr:  true,
			skipBody: true,
		},
		{
			name:     "POST: Invalid time range (end before start)",
			method:   http.MethodPost,
			query:    "up",
			start:    fmt.Sprintf("%d", time.Now().Unix()),
			end:      fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()),
			step:     "1m",
			wantErr:  true,
			skipBody: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build request.
			params := map[string]string{
				"query": tc.query,
				"start": tc.start,
				"end":   tc.end,
				"step":  tc.step,
			}
			req, err := buildRequest(tc.method, baseURL+"/api/v1/query_range", params)
			require.NoError(t, err)

			// Make request.
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			bodyBytes, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tc.wantErr {
				require.NotEqual(t, http.StatusOK, resp.StatusCode, "Expected error response")
				if tc.skipBody {
					return
				}
			} else {
				require.Equal(t, http.StatusOK, resp.StatusCode, "Response body: %s", string(bodyBytes))
			}

			// Validate response against OpenAPI spec.
			validateHTTPResponse(t, docValidator, req, resp, bodyBytes)
		})
	}
}

// TestOpenAPIMetadataValidation tests that metadata endpoint responses validate against the OpenAPI spec.
func TestOpenAPIMetadataValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURL, client := startPrometheusWithHuma(t)
	docValidator := loadOpenAPIValidator(t)

	// Test metadata endpoint.
	url := baseURL + "/api/v1/metadata"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Validate response.
	validateHTTPResponse(t, docValidator, req, resp, bodyBytes)
}
