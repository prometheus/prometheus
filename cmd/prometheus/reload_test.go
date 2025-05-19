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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

const configReloadMetric = "prometheus_config_last_reload_successful"

func TestAutoReloadConfig_ValidToValid(t *testing.T) {
	steps := []struct {
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

	runTestSteps(t, steps)
}

func TestAutoReloadConfig_ValidToInvalidToValid(t *testing.T) {
	steps := []struct {
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

	runTestSteps(t, steps)
}

func runTestSteps(t *testing.T, steps []struct {
	configText       string
	expectedInterval string
	expectedMetric   float64
},
) {
	configDir := t.TempDir()
	configFilePath := filepath.Join(configDir, "prometheus.yml")

	t.Logf("Config file path: %s", configFilePath)

	require.NoError(t, os.WriteFile(configFilePath, []byte(steps[0].configText), 0o644), "Failed to write initial config file")

	port := testutil.RandomUnprivilegedPort(t)
	prom := prometheusCommandWithLogging(t, configFilePath, port, "--enable-feature=auto-reload-config", "--config.auto-reload-interval=1s")
	require.NoError(t, prom.Start())

	baseURL := "http://localhost:" + strconv.Itoa(port)
	require.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/-/ready")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond, "Prometheus didn't become ready in time")

	for i, step := range steps {
		t.Logf("Step %d", i)
		require.NoError(t, os.WriteFile(configFilePath, []byte(step.configText), 0o644), "Failed to write config file for step")

		require.Eventually(t, func() bool {
			return verifyScrapeInterval(t, baseURL, step.expectedInterval) &&
				verifyConfigReloadMetric(t, baseURL, step.expectedMetric)
		}, 10*time.Second, 500*time.Millisecond, "Prometheus config reload didn't happen in time")
	}
}

func verifyScrapeInterval(t *testing.T, baseURL, expectedInterval string) bool {
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
	return strings.Contains(config.Data.YAML, "scrape_interval: "+expectedInterval)
}

func verifyConfigReloadMetric(t *testing.T, baseURL string, expectedValue float64) bool {
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

	return found && actualValue == expectedValue
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

func prometheusCommandWithLogging(t *testing.T, configFilePath string, port int, extraArgs ...string) *exec.Cmd {
	stdoutPipe, stdoutWriter := io.Pipe()
	stderrPipe, stderrWriter := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)

	args := []string{
		"-test.main",
		"--config.file=" + configFilePath,
		"--web.listen-address=0.0.0.0:" + strconv.Itoa(port),
	}
	args = append(args, extraArgs...)
	prom := exec.Command(promPath, args...)
	prom.Stdout = stdoutWriter
	prom.Stderr = stderrWriter

	go func() {
		defer wg.Done()
		captureLogsToTLog(t, stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		captureLogsToTLog(t, stderrPipe)
	}()

	t.Cleanup(func() {
		prom.Process.Kill()
		prom.Wait()
		stdoutWriter.Close()
		stderrWriter.Close()
		wg.Wait()
	})
	return prom
}
