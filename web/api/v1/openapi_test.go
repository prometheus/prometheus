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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

// TestOpenAPIHTTPHandler verifies that the OpenAPI endpoint serves a valid specification
// with correct headers, structure conforming to OpenAPI 3.1 standards, and consistent responses.
func TestOpenAPIHTTPHandler(t *testing.T) {
	builder := NewOpenAPIBuilder(OpenAPIOptions{}, promslog.NewNopLogger())

	// First request.
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec1 := httptest.NewRecorder()
	builder.ServeOpenAPI(rec1, req1)

	// Verify status code and headers.
	require.Equal(t, http.StatusOK, rec1.Code)
	require.True(t, strings.HasPrefix(rec1.Header().Get("Content-Type"), "application/yaml"), "Content-Type should start with application/yaml")
	require.Equal(t, "no-cache, no-store, must-revalidate", rec1.Header().Get("Cache-Control"))

	// Verify it is valid YAML.
	var spec map[string]any
	err := yaml.Unmarshal(rec1.Body.Bytes(), &spec)
	require.NoError(t, err)

	// Verify structure.
	require.Contains(t, spec, "openapi")
	require.Contains(t, spec, "info")
	require.Contains(t, spec, "paths")
	require.Contains(t, spec, "components")

	// Verify OpenAPI version (default is 3.1.0).
	require.Equal(t, "3.1.0", spec["openapi"])

	// Verify info section.
	info, ok := spec["info"].(map[any]any)
	require.True(t, ok, "info should be a map")
	require.Equal(t, "Prometheus API", info["title"])

	// Verify paths exist.
	paths, ok := spec["paths"].(map[any]any)
	require.True(t, ok, "paths should be a map")
	require.NotEmpty(t, paths, "paths should not be empty")

	// Second request to verify response consistency.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec2 := httptest.NewRecorder()
	builder.ServeOpenAPI(rec2, req2)

	// Both responses should be identical.
	require.Equal(t, rec1.Body.String(), rec2.Body.String())
}

// TestOpenAPIPathFiltering verifies that the IncludePaths option correctly filters
// which API paths are included in the generated specification.
func TestOpenAPIPathFiltering(t *testing.T) {
	tests := []struct {
		name         string
		includePaths []string
		wantPaths    []string
		excludePaths []string
	}{
		{
			name:         "no filter includes all",
			includePaths: nil,
			wantPaths:    []string{"/query", "/labels", "/alerts", "/targets"},
		},
		{
			name:         "filter query paths",
			includePaths: []string{"/query"},
			wantPaths:    []string{"/query", "/query_range", "/query_exemplars"},
			excludePaths: []string{"/labels", "/alerts", "/targets"},
		},
		{
			name:         "filter status paths",
			includePaths: []string{"/status"},
			wantPaths:    []string{"/status/config", "/status/flags", "/status/runtimeinfo"},
			excludePaths: []string{"/query", "/alerts", "/targets"},
		},
		{
			name:         "filter multiple prefixes",
			includePaths: []string{"/label", "/series"},
			wantPaths:    []string{"/labels", "/label/{name}/values", "/series"},
			excludePaths: []string{"/query", "/alerts", "/targets"},
		},
		{
			name:         "exact path match",
			includePaths: []string{"/alerts"},
			wantPaths:    []string{"/alerts"},
			excludePaths: []string{"/alertmanagers", "/query"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := NewOpenAPIBuilder(OpenAPIOptions{
				IncludePaths: tc.includePaths,
			}, promslog.NewNopLogger())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
			rec := httptest.NewRecorder()
			builder.ServeOpenAPI(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)

			var spec map[string]any
			err := yaml.Unmarshal(rec.Body.Bytes(), &spec)
			require.NoError(t, err)

			paths, ok := spec["paths"].(map[any]any)
			require.True(t, ok, "paths should be a map")

			for _, want := range tc.wantPaths {
				require.Contains(t, paths, want)
			}

			for _, exclude := range tc.excludePaths {
				require.NotContains(t, paths, exclude)
			}
		})
	}
}

// TestOpenAPISchemaCompleteness verifies that all referenced schemas in paths
// are defined in the components/schemas section of the specification.
func TestOpenAPISchemaCompleteness(t *testing.T) {
	builder := NewOpenAPIBuilder(OpenAPIOptions{}, promslog.NewNopLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	builder.ServeOpenAPI(rec, req)

	var spec map[string]any
	err := yaml.Unmarshal(rec.Body.Bytes(), &spec)
	require.NoError(t, err)

	components, ok := spec["components"].(map[any]any)
	require.True(t, ok, "components should be a map")

	schemas, ok := components["schemas"].(map[any]any)
	require.True(t, ok, "schemas should be a map")

	// Verify essential schemas are present.
	essentialSchemas := []string{
		"Error",
		"Labels",
		"QueryOutputBody",
		"LabelsOutputBody",
		"SeriesOutputBody",
		"TargetsOutputBody",
		"AlertsOutputBody",
		"RulesOutputBody",
		"StatusConfigOutputBody",
		"StatusFlagsOutputBody",
		"PrometheusVersion",
	}

	for _, schema := range essentialSchemas {
		require.Contains(t, schemas, schema)
	}
}

// TODO: Add test to verify all routes from api.go Register() are covered in OpenAPI spec.
// Consider wrapping Router to track registered paths and cross-check with OpenAPI paths.

// TestOpenAPIShouldIncludePath verifies the shouldIncludePath method correctly
// matches paths against the IncludePaths filter configuration.
func TestOpenAPIShouldIncludePath(t *testing.T) {
	tests := []struct {
		name         string
		includePaths []string
		path         string
		expected     bool
	}{
		{
			name:         "empty filter includes all",
			includePaths: nil,
			path:         "/query",
			expected:     true,
		},
		{
			name:         "exact match",
			includePaths: []string{"/query"},
			path:         "/query",
			expected:     true,
		},
		{
			name:         "prefix match",
			includePaths: []string{"/query"},
			path:         "/query_range",
			expected:     true,
		},
		{
			name:         "no match",
			includePaths: []string{"/query"},
			path:         "/labels",
			expected:     false,
		},
		{
			name:         "multiple filters with match",
			includePaths: []string{"/labels", "/series"},
			path:         "/series",
			expected:     true,
		},
		{
			name:         "multiple filters without match",
			includePaths: []string{"/labels", "/series"},
			path:         "/query",
			expected:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := &OpenAPIBuilder{
				options: OpenAPIOptions{
					IncludePaths: tc.includePaths,
				},
			}

			result := builder.shouldIncludePath(tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestOpenAPIVersionConsistency verifies that both OpenAPI versions are properly generated
// and that 3.2 has exactly one more path than 3.1 (/notifications/live).
func TestOpenAPIVersionConsistency(t *testing.T) {
	builder := NewOpenAPIBuilder(OpenAPIOptions{}, promslog.NewNopLogger())

	// Fetch OpenAPI 3.1 spec (default).
	req31 := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec31 := httptest.NewRecorder()
	builder.ServeOpenAPI(rec31, req31)

	require.Equal(t, http.StatusOK, rec31.Code)

	// Fetch OpenAPI 3.2 spec.
	req32 := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml?openapi_version=3.2", nil)
	rec32 := httptest.NewRecorder()
	builder.ServeOpenAPI(rec32, req32)

	require.Equal(t, http.StatusOK, rec32.Code)

	// Parse both specs.
	var spec31, spec32 map[string]any
	err := yaml.Unmarshal(rec31.Body.Bytes(), &spec31)
	require.NoError(t, err)
	err = yaml.Unmarshal(rec32.Body.Bytes(), &spec32)
	require.NoError(t, err)

	// Verify versions are different.
	require.Equal(t, "3.1.0", spec31["openapi"])
	require.Equal(t, "3.2.0", spec32["openapi"])

	// Verify /notifications/live is only in 3.2.
	paths31 := spec31["paths"].(map[any]any)
	paths32 := spec32["paths"].(map[any]any)

	require.NotContains(t, paths31, "/notifications/live")

	require.Contains(t, paths32, "/notifications/live")

	// Verify 3.2 has exactly one more path than 3.1.
	require.Len(t, paths32, len(paths31)+1,
		"OpenAPI 3.2 should have exactly one more path than 3.1")
}
