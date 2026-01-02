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

// This file provides HTTP request builders for testing API endpoints.
package testhelpers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Response wraps an HTTP response with parsed JSON data.
// It supports method chaining for assertions.
//
// Example usage:
//
//	testhelpers.GET(t, api, "/api/v1/query", "query", "up").
//		ValidateOpenAPI().
//		RequireSuccess().
//		RequireEquals("$.data.resultType", "vector").
//		RequireLenAtLeast("$.data.result", 1)
//
//	testhelpers.POST(t, api, "/api/v1/query", "query", "up").
//		ValidateOpenAPI().
//		RequireSuccess().
//		RequireArrayContains("$.data.result", expectedValue)
type Response struct {
	StatusCode     int
	Body           string
	JSON           map[string]any
	t              *testing.T
	request        *http.Request
	requestBody    []byte
	responseHeader http.Header
}

// GET sends a GET request to the API and returns a Response with parsed JSON.
// queryParams should be pairs of key-value strings.
func GET(t *testing.T, api *APIWrapper, path string, queryParams ...string) *Response {
	t.Helper()

	if len(queryParams)%2 != 0 {
		t.Fatal("queryParams must be key-value pairs")
	}

	// Build query string.
	values := url.Values{}
	for i := 0; i < len(queryParams); i += 2 {
		values.Add(queryParams[i], queryParams[i+1])
	}

	fullPath := path
	if len(values) > 0 {
		fullPath = path + "?" + values.Encode()
	}

	req := httptest.NewRequest(http.MethodGet, fullPath, nil)
	return executeRequest(t, api, req)
}

// POST sends a POST request to the API with the given body and returns a Response with parsed JSON.
// bodyParams should be pairs of key-value strings for form data.
func POST(t *testing.T, api *APIWrapper, path string, bodyParams ...string) *Response {
	t.Helper()

	if len(bodyParams)%2 != 0 {
		t.Fatal("bodyParams must be key-value pairs")
	}

	// Build form data.
	values := url.Values{}
	for i := 0; i < len(bodyParams); i += 2 {
		values.Add(bodyParams[i], bodyParams[i+1])
	}

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return executeRequest(t, api, req)
}

// executeRequest executes an HTTP request and parses the response as JSON.
func executeRequest(t *testing.T, api *APIWrapper, req *http.Request) *Response {
	t.Helper()

	// Capture the request body for validation.
	var requestBody []byte
	if req.Body != nil {
		var err error
		requestBody, err = io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		// Restore the body for the actual request.
		req.Body = io.NopCloser(strings.NewReader(string(requestBody)))
	}

	recorder := httptest.NewRecorder()
	api.Handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	bodyBytes, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	resp := &Response{
		StatusCode:     result.StatusCode,
		Body:           string(bodyBytes),
		t:              t,
		request:        req,
		requestBody:    requestBody,
		responseHeader: result.Header,
	}

	// Try to parse as JSON.
	if result.Header.Get("Content-Type") == "application/json" || strings.Contains(result.Header.Get("Content-Type"), "application/json") {
		var jsonData map[string]any
		if err := json.Unmarshal(bodyBytes, &jsonData); err != nil {
			// If JSON parsing fails, leave JSON as nil.
			// This allows tests to handle non-JSON responses.
			resp.JSON = nil
		} else {
			resp.JSON = jsonData
		}
	}

	return resp
}
