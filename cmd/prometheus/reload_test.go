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
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
		{
			configText: `
global:
  scrape_interval: 15s
  scrape_timeout: 30s
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
	prom := prometheusCommandWithLogging(t, configFilePath, port, "--config.auto-reload", "--config.auto-reload-interval=1s")
	require.NoError(t, prom.Start())

	baseURL := "http://localhost:" + strconv.Itoa(port)
	waitForPrometheusReady(t, port)

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

	for line := range strings.SplitSeq(lines, "\n") {
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

func captureLogsToTLog(t testing.TB, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		t.Log(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Logf("Error reading logs: %v", err)
	}
}

// gracefulShutdownTimeout bounds how long commandWithLogging waits for the
// process to exit after SIGTERM. It must exceed the default
// --storage.remote.flush-deadline (1m), which Prometheus may spend flushing
// remote write queues on shutdown.
const gracefulShutdownTimeout = 2 * time.Minute

func commandWithLogging(t testing.TB, logProcessor func(testing.TB, io.Reader), name string, args ...string) *exec.Cmd {
	if logProcessor == nil {
		logProcessor = captureLogsToTLog
	}

	stdoutPipe, stdoutWriter := io.Pipe()
	stderrPipe, stderrWriter := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)

	cmd := exec.Command(name, args...)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	go func() {
		defer wg.Done()
		defer func() { _, _ = io.Copy(io.Discard, stdoutPipe) }()
		logProcessor(t, stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		defer func() { _, _ = io.Copy(io.Discard, stderrPipe) }()
		logProcessor(t, stderrPipe)
	}()

	t.Cleanup(func() {
		// Close the writers and wait for the log processors on every path, so
		// that all output reaches the test log before the test completes.
		defer func() {
			stdoutWriter.Close()
			stderrWriter.Close()
			wg.Wait()
		}()

		// Terminate the process gracefully, as that's the expected shutdown path.
		// Prometheus must handle this signal cleanly.
		if runtime.GOOS != "windows" {
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				// The process may already be exiting, for example because the test
				// sent it os.Interrupt. Its exit status decides.
				t.Logf("sending SIGTERM: %v", err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("process did not exit cleanly after SIGTERM: %v", err)
				}
			case <-time.After(gracefulShutdownTimeout):
				// SIGQUIT makes the Go runtime write all goroutine stacks to stderr,
				// which ends up in the test log, and exit.
				t.Errorf("process did not exit within %v of SIGTERM, sending SIGQUIT", gracefulShutdownTimeout)
				cmd.Process.Signal(syscall.SIGQUIT)
				<-done
			}
		} else {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	return cmd
}

func prometheusCommandWithLogging(t testing.TB, configFilePath string, port int, extraArgs ...string) *exec.Cmd {
	args := []string{
		"-test.main",
		"--config.file=" + configFilePath,
		"--web.listen-address=0.0.0.0:" + strconv.Itoa(port),
		"--log.level=debug",
	}
	args = append(args, extraArgs...)
	return commandWithLogging(t, nil, promPath, args...)
}

// readyTimeout is how long we wait for a Prometheus process to become ready. It
// is generous on purpose: CI runs these tests with the race detector, which
// slows down both the test binary and the Prometheus processes it spawns in
// parallel.
const readyTimeout = 30 * time.Second

// waitForPrometheusReady waits until the Prometheus instance listening on port
// reports itself as ready, which means it has opened its storage and applied
// its initial configuration.
func waitForPrometheusReady(t testing.TB, port int) {
	t.Helper()

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/-/ready")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, readyTimeout, 100*time.Millisecond, "Prometheus didn't become ready in time")
}
