// Copyright 2020 The Prometheus Authors
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

	"github.com/prometheus/prometheus/util/testutil"
)

// As soon as prometheus starts responding to http request it should be able to
// accept Interrupt signals for a graceful shutdown.
func TestStartupInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	port := fmt.Sprintf(":%d", testutil.RandomUnprivilegedPort(t))

	prom := exec.Command(promPath, "-test.main", "--config.file="+promConfig, "--storage.tsdb.path="+t.TempDir(), "--web.listen-address=0.0.0.0"+port)
	err := prom.Start()
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- prom.Wait()
	}()

	var startedOk bool
	var stoppedErr error

	url := "http://localhost" + port + "/graph"

Loop:
	for x := 0; x < 10; x++ {
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

	if !startedOk {
		t.Fatal("prometheus didn't start in the specified timeout")
	}
	switch err := prom.Process.Kill(); {
	case err == nil:
		t.Errorf("prometheus didn't shutdown gracefully after sending the Interrupt signal")
	case stoppedErr != nil && stoppedErr.Error() != "signal: interrupt":
		// TODO: find a better way to detect when the process didn't exit as expected!
		t.Errorf("prometheus exited with an unexpected error: %v", stoppedErr)
	}
}
