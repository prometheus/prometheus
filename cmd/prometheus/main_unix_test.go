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
//
//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/testutil"
)

// As soon as prometheus starts responding to http request it should be able to
// accept Interrupt signals for a graceful shutdown.
func TestStartupInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	port := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	prom := exec.Command(promPath, "-test.main", "--config.file="+promConfig, "--storage.tsdb.path="+t.TempDir(), "--web.listen-address=0.0.0.0"+port)
	err := prom.Start()
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- prom.Wait()
	}()

	var startedOk bool
	var stoppedErr error

	url := "http://localhost" + port + "/graph"

Loop:
	for range 10 {
		// error=nil means prometheus has started, so we can send the interrupt
		// signal and wait for the graceful shutdown.
		if _, err := http.Get(url); err == nil {
			startedOk = true
			prom.Process.Signal(os.Interrupt)
			select {
			case stoppedErr = <-done:
				break Loop
			case <-time.After(10 * time.Second):
			}
			break Loop
		}
		time.Sleep(500 * time.Millisecond)
	}

	require.True(t, startedOk, "prometheus didn't start in the specified timeout")
	err = prom.Process.Kill()
	require.Error(t, err, "prometheus didn't shutdown gracefully after sending the Interrupt signal")
	// TODO - find a better way to detect when the process didn't exit as expected!
	if stoppedErr != nil {
		require.EqualError(t, stoppedErr, "signal: interrupt", "prometheus exit")
	}
}

// TestUnixSocketListener checks that Prometheus can serve its UI, API and own
// telemetry exclusively on a Unix domain socket and scrape itself through it.
func TestUnixSocketListener(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	// t.TempDir can produce paths exceeding the macOS 104-char socket
	// path limit, so we create our own temp dir.
	dir, err := os.MkdirTemp("", "prom-uds")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "prometheus.sock")

	t.Run("requires external URL", func(t *testing.T) {
		prom := exec.Command(promPath, "-test.main",
			"--config.file="+promConfig,
			"--storage.tsdb.path="+t.TempDir(),
			"--web.listen-address=unix://"+socketPath,
		)
		var stderr bytes.Buffer
		prom.Stderr = &stderr
		err := prom.Run()
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		require.Equal(t, 2, exitErr.ExitCode())
		require.Contains(t, stderr.String(), "--web.external-url must be set")
	})

	t.Run("self-scrape over Unix socket", func(t *testing.T) {
		configFile := filepath.Join(t.TempDir(), "prometheus.yml")
		require.NoError(t, os.WriteFile(configFile, fmt.Appendf(nil, `
global:
  scrape_interval: 100ms
scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: [localhost]
        labels:
          __unix_socket__: %q
`, socketPath), 0o644))

		prom := commandWithLogging(t, nil, promPath, "-test.main",
			"--config.file="+configFile,
			"--storage.tsdb.path="+t.TempDir(),
			"--web.listen-address=unix://"+socketPath,
			"--web.external-url=http://localhost/",
		)
		require.NoError(t, prom.Start())

		client := &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		}}
		t.Cleanup(client.CloseIdleConnections)

		get := func(path string) (int, []byte, error) {
			resp, err := client.Get("http://localhost" + path)
			if err != nil {
				return 0, nil, err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			return resp.StatusCode, body, err
		}

		require.Eventually(t, func() bool {
			code, _, err := get("/-/ready")
			return err == nil && code == http.StatusOK
		}, readyTimeout, 100*time.Millisecond, "Prometheus didn't become ready on the Unix socket")

		code, body, err := get("/metrics")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code)
		require.Contains(t, string(body), "prometheus_build_info")

		// The self-scrape through the Unix socket succeeds.
		require.Eventually(t, func() bool {
			code, body, err := get("/api/v1/query?query=up")
			if err != nil || code != http.StatusOK {
				return false
			}
			var res struct {
				Data struct {
					Result []struct {
						Value [2]any `json:"value"`
					} `json:"result"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &res); err != nil {
				return false
			}
			return len(res.Data.Result) == 1 && res.Data.Result[0].Value[1] == "1"
		}, readyTimeout, 100*time.Millisecond, "Prometheus didn't scrape itself over the Unix socket")
	})
}
