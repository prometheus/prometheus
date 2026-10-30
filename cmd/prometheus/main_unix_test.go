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
	"fmt"
	"net/http"
	"os"
	"os/exec"
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

	url := "http://localhost" + port + "/graph"

	require.Eventually(t, func() bool {
		r, err := http.Get(url)
		if err != nil {
			return false
		}
		r.Body.Close()
		return true
	}, startupTime, 500*time.Millisecond, "prometheus didn't start in the specified timeout")

	prom.Process.Signal(os.Interrupt)
	var stoppedErr error
	select {
	case stoppedErr = <-done:
	case <-time.After(10 * time.Second):
	}

	err = prom.Process.Kill()
	require.Error(t, err, "prometheus didn't shutdown gracefully after sending the Interrupt signal")
	// TODO - find a better way to detect when the process didn't exit as expected!
	if stoppedErr != nil {
		require.EqualError(t, stoppedErr, "signal: interrupt", "prometheus exit")
	}
}
