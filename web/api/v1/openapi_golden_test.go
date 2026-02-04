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
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/prometheus/prometheus/web/api/testhelpers"
)

var updateOpenAPISpec = flag.Bool("update-openapi-spec", false, "update openapi golden files with the current specs")

// TestOpenAPIGolden_3_1 verifies that the OpenAPI 3.1 spec matches the golden file.
func TestOpenAPIGolden_3_1(t *testing.T) {
	// Create an API instance to serve the OpenAPI spec.
	api := newTestAPI(t, testhelpers.APIConfig{})

	// Fetch the OpenAPI 3.1 spec from the API (default, no query param).
	resp := testhelpers.GET(t, api, "/api/v1/openapi.yaml")
	require.Equal(t, 200, resp.StatusCode, "expected HTTP 200 for OpenAPI spec endpoint")
	require.NotEmpty(t, resp.Body, "OpenAPI spec should not be empty")

	goldenPath := filepath.Join("testdata", "openapi_3.1_golden.yaml")

	if *updateOpenAPISpec {
		// Update mode: write the current spec to the golden file.
		t.Logf("Updating golden file: %s", goldenPath)

		// Ensure the testdata directory exists.
		err := os.MkdirAll(filepath.Dir(goldenPath), 0o755)
		require.NoError(t, err, "failed to create testdata directory")

		// Write the golden file.
		err = os.WriteFile(goldenPath, []byte(resp.Body), 0o644)
		require.NoError(t, err, "failed to write golden file")

		t.Logf("Golden file updated successfully")
		return
	}

	// Comparison mode: verify the spec matches the golden file.
	goldenData, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "failed to read golden file (run with -update-openapi-spec to generate it)")

	require.Equal(t, string(goldenData), resp.Body,
		"OpenAPI 3.1 spec does not match golden file. Run 'go test -update-openapi-spec' to update.")

	// Verify version field is 3.1.0.
	var spec map[string]any
	err = yaml.Unmarshal([]byte(resp.Body), &spec)
	require.NoError(t, err)
	require.Equal(t, "3.1.0", spec["openapi"], "OpenAPI version should be 3.1.0")

	// Verify /notifications/live is NOT present in 3.1 spec.
	paths := spec["paths"].(map[string]any)
	_, found := paths["/notifications/live"]
	require.False(t, found, "/notifications/live should not be in OpenAPI 3.1 spec")
}

// TestOpenAPIGolden_3_2 verifies that the OpenAPI 3.2 spec matches the golden file.
func TestOpenAPIGolden_3_2(t *testing.T) {
	// Create an API instance to serve the OpenAPI spec.
	api := newTestAPI(t, testhelpers.APIConfig{})

	// Fetch the OpenAPI 3.2 spec from the API with query parameter.
	resp := testhelpers.GET(t, api, "/api/v1/openapi.yaml?openapi_version=3.2")
	require.Equal(t, 200, resp.StatusCode, "expected HTTP 200 for OpenAPI spec endpoint")
	require.NotEmpty(t, resp.Body, "OpenAPI spec should not be empty")

	goldenPath := filepath.Join("testdata", "openapi_3.2_golden.yaml")

	if *updateOpenAPISpec {
		// Update mode: write the current spec to the golden file.
		t.Logf("Updating golden file: %s", goldenPath)

		// Ensure the testdata directory exists.
		err := os.MkdirAll(filepath.Dir(goldenPath), 0o755)
		require.NoError(t, err, "failed to create testdata directory")

		// Write the golden file.
		err = os.WriteFile(goldenPath, []byte(resp.Body), 0o644)
		require.NoError(t, err, "failed to write golden file")

		t.Logf("Golden file updated successfully")
		return
	}

	// Comparison mode: verify the spec matches the golden file.
	goldenData, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "failed to read golden file (run with -update-openapi-spec to generate it)")

	require.Equal(t, string(goldenData), resp.Body,
		"OpenAPI 3.2 spec does not match golden file. Run 'go test -update-openapi-spec' to update.")

	// Verify version field is 3.2.0.
	var spec map[string]any
	err = yaml.Unmarshal([]byte(resp.Body), &spec)
	require.NoError(t, err)
	require.Equal(t, "3.2.0", spec["openapi"], "OpenAPI version should be 3.2.0")

	// Verify /notifications/live IS present in 3.2 spec.
	paths := spec["paths"].(map[string]any)
	_, found := paths["/notifications/live"]
	require.True(t, found, "/notifications/live should be in OpenAPI 3.2 spec")
}

// TestOpenAPIVersionSelection verifies version query parameter handling.
func TestOpenAPIVersionSelection(t *testing.T) {
	api := newTestAPI(t, testhelpers.APIConfig{})

	tests := []struct {
		name            string
		url             string
		expectedVersion string
		expectLivePath  bool
	}{
		{
			name:            "default to 3.1.0",
			url:             "/api/v1/openapi.yaml",
			expectedVersion: "3.1.0",
			expectLivePath:  false,
		},
		{
			name:            "explicit 3.1",
			url:             "/api/v1/openapi.yaml?openapi_version=3.1",
			expectedVersion: "3.1.0",
			expectLivePath:  false,
		},
		{
			name:            "explicit 3.2",
			url:             "/api/v1/openapi.yaml?openapi_version=3.2",
			expectedVersion: "3.2.0",
			expectLivePath:  true,
		},
		{
			name:            "invalid version defaults to 3.1.0",
			url:             "/api/v1/openapi.yaml?openapi_version=4.0",
			expectedVersion: "3.1.0",
			expectLivePath:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := testhelpers.GET(t, api, tc.url)
			require.Equal(t, 200, resp.StatusCode)

			var spec map[string]any
			err := yaml.Unmarshal([]byte(resp.Body), &spec)
			require.NoError(t, err)

			require.Equal(t, tc.expectedVersion, spec["openapi"])

			paths := spec["paths"].(map[string]any)
			_, found := paths["/notifications/live"]
			require.Equal(t, tc.expectLivePath, found)
		})
	}
}
