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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

var updateFeatures = flag.Bool("update-features", false, "update features.json golden file")

func TestFeaturesAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "prometheus.yml")
	require.NoError(t, os.WriteFile(configFile, []byte{}, 0o644))

	port := testutil.RandomUnprivilegedPort(t)
	prom := prometheusCommandWithLogging(
		t,
		configFile,
		port,
		fmt.Sprintf("--storage.tsdb.path=%s", tmpDir),
	)
	require.NoError(t, prom.Start())

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for Prometheus to be ready.
	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/-/ready")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond, "Prometheus didn't become ready in time")

	// Fetch features from the API.
	resp, err := http.Get(baseURL + "/api/v1/features")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Parse API response.
	var apiResponse struct {
		Status string                     `json:"status"`
		Data   map[string]map[string]bool `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &apiResponse))
	require.Equal(t, "success", apiResponse.Status)

	goldenPath := filepath.Join("testdata", "features.json")

	// If update flag is set, write the current features to the golden file.
	if *updateFeatures {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		require.NoError(t, encoder.Encode(apiResponse.Data))
		// Ensure testdata directory exists.
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, buf.Bytes(), 0o644))
		t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	// Load golden file.
	goldenData, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "Failed to read golden file %s. Run 'make update-features-testdata' to generate it.", goldenPath)

	var expectedFeatures map[string]map[string]bool
	require.NoError(t, json.Unmarshal(goldenData, &expectedFeatures))

	// The labels implementation depends on build tags (stringlabels, slicelabels, or dedupelabels).
	// We need to update the expected features to match the current build.
	if prometheusFeatures, ok := expectedFeatures["prometheus"]; ok {
		// Remove all label implementation features from expected.
		delete(prometheusFeatures, "stringlabels")
		delete(prometheusFeatures, "slicelabels")
		delete(prometheusFeatures, "dedupelabels")
		// Add the current implementation.
		if actualPrometheus, ok := apiResponse.Data["prometheus"]; ok {
			for _, impl := range []string{"stringlabels", "slicelabels", "dedupelabels"} {
				if actualPrometheus[impl] {
					prometheusFeatures[impl] = true
				}
			}
		}
	}

	// Compare the features data with the golden file.
	require.Equal(t, expectedFeatures, apiResponse.Data, "Features mismatch. Run 'make update-features-testdata' to update the golden file.")
}
