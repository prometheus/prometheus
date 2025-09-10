// Copyright 2024 The Prometheus Authors
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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestScrapeFailureLogFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// Tracks the number of requests made to the mock server.
	var requestCount atomic.Int32

	// Starts a server that always returns HTTP 500 errors.
	mockServerAddress := startGarbageServer(t, &requestCount)

	// Create a temporary directory for Prometheus configuration and logs.
	tempDir := t.TempDir()

	// Define file paths for the scrape failure log and Prometheus configuration.
	// Like other files, the scrape failure log file should be relative to the
	// config file. Therefore, we split the name we put in the file and the full
	// path used to check the content of the file.
	scrapeFailureLogFileName := "scrape_failure.log"
	scrapeFailureLogFile := filepath.Join(tempDir, scrapeFailureLogFileName)
	promConfigFile := filepath.Join(tempDir, "prometheus.yml")

	// Step 1: Set up an initial Prometheus configuration that globally
	// specifies a scrape failure log file.
	promConfig := fmt.Sprintf(`
global:
  scrape_interval: 500ms
  scrape_failure_log_file: %s

scrape_configs:
  - job_name: 'test_job'
    static_configs:
      - targets: ['%s']
`, scrapeFailureLogFileName, mockServerAddress)

	err := os.WriteFile(promConfigFile, []byte(promConfig), 0o644)
	require.NoError(t, err, "Failed to write Prometheus configuration file")

	// Start Prometheus with the generated configuration and a random port, enabling the lifecycle API.
	port := testutil.RandomUnprivilegedPort(t)
	params := []string{
		"-test.main",
		"--config.file=" + promConfigFile,
		"--storage.tsdb.path=" + filepath.Join(tempDir, "data"),
		fmt.Sprintf("--web.listen-address=127.0.0.1:%d", port),
		"--web.enable-lifecycle",
	}
	prometheusProcess := exec.Command(promPath, params...)
	prometheusProcess.Stdout = os.Stdout
	prometheusProcess.Stderr = os.Stderr

	err = prometheusProcess.Start()
	require.NoError(t, err, "Failed to start Prometheus")
	defer prometheusProcess.Process.Kill()

	// Wait until the mock server receives at least two requests from Prometheus.
	require.Eventually(t, func() bool {
		return requestCount.Load() >= 2
	}, 30*time.Second, 500*time.Millisecond, "Expected at least two requests to the mock server")

	// Verify that the scrape failures have been logged to the specified file.
	content, err := os.ReadFile(scrapeFailureLogFile)
	require.NoError(t, err, "Failed to read scrape failure log")
	require.Contains(t, string(content), "server returned HTTP status 500 Internal Server Error", "Expected scrape failure log entry not found")

	// Step 2: Update the Prometheus configuration to remove the scrape failure
	// log file setting.
	promConfig = fmt.Sprintf(`
global:
  scrape_interval: 1s

scrape_configs:
  - job_name: 'test_job'
    static_configs:
      - targets: ['%s']
`, mockServerAddress)

	err = os.WriteFile(promConfigFile, []byte(promConfig), 0o644)
	require.NoError(t, err, "Failed to update Prometheus configuration file")

	// Reload Prometheus with the updated configuration.
	reloadURL := fmt.Sprintf("http://127.0.0.1:%d/-/reload", port)
	reloadPrometheusConfig(t, reloadURL)

	// Count the number of lines in the scrape failure log file before any
	// further requests.
	preReloadLogLineCount := countLinesInFile(scrapeFailureLogFile)

	// Wait for at least two more requests to the mock server to ensure
	// Prometheus continues scraping.
	requestsBeforeReload := requestCount.Load()
	require.Eventually(t, func() bool {
		return requestCount.Load() >= requestsBeforeReload+2
	}, 30*time.Second, 500*time.Millisecond, "Expected two more requests to the mock server after configuration reload")

	// Ensure that no new lines were added to the scrape failure log file after
	// the configuration change.
	require.Equal(t, preReloadLogLineCount, countLinesInFile(scrapeFailureLogFile), "No new lines should be added to the scrape failure log file after removing the log setting")

	// Step 3: Re-add the scrape failure log file setting, but this time under
	// scrape_configs, and reload Prometheus.
	promConfig = fmt.Sprintf(`
global:
  scrape_interval: 1s

scrape_configs:
  - job_name: 'test_job'
    scrape_failure_log_file: %s
    static_configs:
      - targets: ['%s']
`, scrapeFailureLogFileName, mockServerAddress)

	err = os.WriteFile(promConfigFile, []byte(promConfig), 0o644)
	require.NoError(t, err, "Failed to update Prometheus configuration file")

	// Reload Prometheus with the updated configuration.
	reloadPrometheusConfig(t, reloadURL)

	// Wait for at least two more requests to the mock server and verify that
	// new log entries are created.
	postReloadLogLineCount := countLinesInFile(scrapeFailureLogFile)
	requestsBeforeReAddingLog := requestCount.Load()
	require.Eventually(t, func() bool {
		return requestCount.Load() >= requestsBeforeReAddingLog+2
	}, 30*time.Second, 500*time.Millisecond, "Expected two additional requests after re-adding the log setting")

	// Confirm that new lines were added to the scrape failure log file.
	require.Greater(t, countLinesInFile(scrapeFailureLogFile), postReloadLogLineCount, "New lines should be added to the scrape failure log file after re-adding the log setting")
}

// startGarbageServer sets up a mock server that returns a 500 Internal Server Error
// for all requests. It also increments the request count each time it's hit.
func startGarbageServer(t *testing.T, requestCount *atomic.Int32) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Inc()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	parsedURL, err := url.Parse(server.URL)
	require.NoError(t, err, "Failed to parse mock server URL")

	return parsedURL.Host
}

// countLinesInFile counts and returns the number of lines in the specified file.
func countLinesInFile(filePath string) int {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0 // Return 0 if the file doesn't exist or can't be read.
	}
	return bytes.Count(data, []byte{'\n'})
}
