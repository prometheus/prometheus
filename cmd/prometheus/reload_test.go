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
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

const configReloadMetric = "prometheus_config_last_reload_successful"

func TestAutoReloadConfig_ValidToValid(t *testing.T) {
	testCases := []struct {
		configText       string
		expectedInterval string
		expectedMetric   float64
	}{
		{
			configText: `
global:
  scrape_interval: 30s
`,
			expectedInterval: "30s",
			expectedMetric:   1,
		},
		{
			configText: `
global:
  scrape_interval: 15s
`,
			expectedInterval: "15s",
			expectedMetric:   1,
		},
		{
			configText: `
global:
  scrape_interval: 30s
`,
			expectedInterval: "30s",
			expectedMetric:   1,
		},
	}

	runTestCase(t, testCases)
}

func TestAutoReloadConfig_ValidToInvalidToValid(t *testing.T) {
	testCases := []struct {
		configText       string
		expectedInterval string
		expectedMetric   float64
	}{
		{
			configText: `
global:
  scrape_interval: 30s
`,
			expectedInterval: "30s",
			expectedMetric:   1,
		},
		{
			configText: `
global:
  scrape_interval: 15s
invalid_syntax
`,
			expectedInterval: "30s",
			expectedMetric:   0,
		},
		{
			configText: `
global:
  scrape_interval: 30s
`,
			expectedInterval: "30s",
			expectedMetric:   1,
		},
	}

	runTestCase(t, testCases)
}

func runTestCase(t *testing.T, testCases []struct {
	configText       string
	expectedInterval string
	expectedMetric   float64
},
) {
	configDir := t.TempDir()
	configFilePath := filepath.Join(configDir, "prometheus.yml")

	t.Logf("Config file path: %s", configFilePath)

	require.NoError(t, os.WriteFile(configFilePath, []byte(testCases[0].configText), 0o644), "Failed to write initial config file")

	port := testutil.RandomUnprivilegedPort(t)

	prom := runPrometheusWithLogging(t, configFilePath, port)
	defer prom.Process.Kill()

	time.Sleep(5 * time.Second)

	for _, tc := range testCases {
		require.NoError(t, os.WriteFile(configFilePath, []byte(tc.configText), 0o644), "Failed to write config file for test case")

		time.Sleep(2 * time.Second)

		verifyScrapeInterval(t, "http://localhost:"+strconv.Itoa(port), tc.expectedInterval)
		verifyConfigReloadMetric(t, "http://localhost:"+strconv.Itoa(port), tc.expectedMetric)
	}
}

func verifyScrapeInterval(t *testing.T, baseURL, expectedInterval string) {
	resp, err := http.Get(baseURL + "/api/v1/status/config")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	config := struct {
		Data struct {
			YAML string `json:"yaml"`
		} `json:"data"`
	}{}

	require.NoError(t, json.Unmarshal(body, &config))
	require.Contains(t, config.Data.YAML, "scrape_interval: "+expectedInterval)
}

func verifyConfigReloadMetric(t *testing.T, baseURL string, expectedValue float64) {
	resp, err := http.Get(baseURL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	lines := string(body)
	var actualValue float64
	found := false

	for _, line := range strings.Split(lines, "\n") {
		if strings.HasPrefix(line, configReloadMetric) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				actualValue, err = strconv.ParseFloat(parts[1], 64)
				require.NoError(t, err)
				found = true
				break
			}
		}
	}

	require.True(t, found, "Expected metric %s not found in Prometheus metrics", configReloadMetric)
	require.Equal(t, expectedValue, actualValue)
}

func captureLogsToTLog(t *testing.T, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		t.Log(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Logf("Error reading logs: %v", err)
	}
}

func runPrometheusWithLogging(t *testing.T, configFilePath string, port int) *exec.Cmd {
	stdoutPipe, stdoutWriter := io.Pipe()
	stderrPipe, stderrWriter := io.Pipe()

	prom := exec.Command(promPath, "-test.main", "--enable-feature=auto-reload-config", "--config.file="+configFilePath, "--config.auto-reload-interval=1s", "--web.listen-address=0.0.0.0:"+strconv.Itoa(port))
	prom.Stdout = stdoutWriter
	prom.Stderr = stderrWriter

	go func() {
		defer stdoutWriter.Close()
		captureLogsToTLog(t, stdoutPipe)
	}()

	go func() {
		defer stderrWriter.Close()
		captureLogsToTLog(t, stderrPipe)
	}()

	require.NoError(t, prom.Start())
	return prom
}
