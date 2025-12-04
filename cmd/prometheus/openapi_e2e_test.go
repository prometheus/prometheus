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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/prometheus/util/testutil"
)

// OpenAPISpec represents a minimal structure of the OpenAPI spec.
type OpenAPISpec struct {
	Paths map[string]map[string]PathItem `yaml:"paths"`
}

// PathItem represents an operation in the OpenAPI spec.
type PathItem struct {
	OperationID string `yaml:"operationId"`
}

func TestOpenAPIE2E(t *testing.T) {
	// Skip if schemathesis is not available.
	if _, err := exec.LookPath("schemathesis"); err != nil {
		t.Skip("schemathesis not found in PATH, skipping E2E test")
	}

	// Parse OpenAPI spec to get operation IDs.
	openAPISpec := filepath.Join("testdata", "openapi_golden.yaml")
	specData, err := os.ReadFile(openAPISpec)
	require.NoError(t, err, "Failed to read OpenAPI spec")

	var spec OpenAPISpec
	require.NoError(t, yaml.Unmarshal(specData, &spec), "Failed to parse OpenAPI spec")

	// Collect all operation IDs except the excluded one.
	var operationIDs []string
	for path, methods := range spec.Paths {
		if path == "/notifications/live" {
			continue
		}
		for _, item := range methods {
			if item.OperationID != "" {
				operationIDs = append(operationIDs, item.OperationID)
			}
		}
	}

	// Create temporary directory for test data.
	configDir := t.TempDir()
	configFilePath := filepath.Join(configDir, "prometheus.yml")

	// Write minimal config.
	configText := `
global:
  scrape_interval: 30s
`
	require.NoError(t, os.WriteFile(configFilePath, []byte(configText), 0o644), "Failed to write config file")

	// Start Prometheus on a random port.
	port := testutil.RandomUnprivilegedPort(t)
	storageDir := filepath.Join(configDir, "data")
	prom := prometheusCommandWithLogging(t, configFilePath, port, "--storage.tsdb.path="+storageDir)
	require.NoError(t, prom.Start())
	defer func() {
		if prom.Process != nil {
			prom.Process.Kill()
		}
	}()

	baseURL := "http://localhost:" + strconv.Itoa(port)

	// Wait for Prometheus to become ready.
	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/-/ready")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond, "Prometheus didn't become ready in time")

	// Run schemathesis tests for each operation as a subtest.
	for _, opID := range operationIDs {
		opID := opID // Capture for closure.
		t.Run(opID, func(t *testing.T) {
			// Create temp file for JUnit XML report.
			junitFile := filepath.Join(t.TempDir(), "junit.xml")

			schemathesisCmd := exec.Command(
				"schemathesis",
				"--config-file", filepath.Join("testdata", "schemathesis.toml"),
				"run",
				"-u", baseURL+"/api/v1",
				"--include-operation-id", opID,
				"--report", "junit",
				"--report-junit-path", junitFile,
				openAPISpec,
			)
			output, err := schemathesisCmd.CombinedOutput()

			// Read JUnit report to check for failures.
			if junitData, readErr := os.ReadFile(junitFile); readErr == nil {
				// Parse JUnit XML to check for failures or errors.
				hasFailure := strings.Contains(string(junitData), "<failure") || strings.Contains(string(junitData), "<error")
				if hasFailure {
					t.Logf("Schemathesis output for %s:\n%s", opID, string(output))
					// Extract failure details from JUnit.
					if strings.Contains(string(junitData), "<failure") {
						t.Log("Found test failures")
					}
					if strings.Contains(string(junitData), "<error") {
						t.Log("Found test errors")
					}
					t.Errorf("Schemathesis test failed for operation %s", opID)
				}
			} else if err != nil {
				// Fallback if JUnit report couldn't be read.
				t.Logf("Schemathesis output for %s:\n%s", opID, string(output))
				t.Errorf("Schemathesis test failed for operation %s: %v", opID, err)
			}
		})
	}
}
