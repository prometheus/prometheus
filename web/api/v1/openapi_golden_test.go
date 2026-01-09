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

	"github.com/prometheus/prometheus/web/api/testhelpers"
)

var updateOpenAPISpec = flag.Bool("update-openapi-spec", false, "update openapi_golden.yaml with the current spec")

// TestOpenAPIGolden verifies that the OpenAPI spec matches the golden file.
func TestOpenAPIGolden(t *testing.T) {
	// Create an API instance to serve the OpenAPI spec.
	api := newTestAPI(t, testhelpers.APIConfig{})

	// Fetch the OpenAPI spec from the API.
	resp := testhelpers.GET(t, api, "/api/v1/openapi.yaml")
	require.Equal(t, 200, resp.StatusCode, "expected HTTP 200 for OpenAPI spec endpoint")
	require.NotEmpty(t, resp.Body, "OpenAPI spec should not be empty")

	goldenPath := filepath.Join("testdata", "openapi_golden.yaml")

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
		"OpenAPI spec does not match golden file. Run 'go test -update-openapi-spec' to update.")
}
