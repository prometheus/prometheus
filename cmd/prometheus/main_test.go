// Copyright 2017 The Prometheus Authors
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
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/prometheus/util/testutil"
)

var promPath string
var promConfig = filepath.Join("..", "..", "documentation", "examples", "prometheus.yml")
var promData = filepath.Join(os.TempDir(), "data")

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}
	// On linux with a global proxy the tests will fail as the go client(http,grpc) tries to connect through the proxy.
	os.Setenv("no_proxy", "localhost,127.0.0.1,0.0.0.0,:")

	var err error
	promPath, err = os.Getwd()
	if err != nil {
		fmt.Printf("can't get current dir :%s \n", err)
		os.Exit(1)
	}
	promPath = filepath.Join(promPath, "prometheus")

	build := exec.Command("go", "build", "-o", promPath)
	output, err := build.CombinedOutput()
	if err != nil {
		fmt.Printf("compilation error :%s \n", output)
		os.Exit(1)
	}

	exitCode := m.Run()
	os.Remove(promPath)
	os.RemoveAll(promData)
	os.Exit(exitCode)
}

// As soon as prometheus starts responding to http request should be able to accept Interrupt signals for a gracefull shutdown.
func TestStartupInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	prom := exec.Command(promPath, "--config.file="+promConfig, "--storage.tsdb.path="+promData)
	err := prom.Start()
	if err != nil {
		t.Errorf("execution error: %v", err)
		return
	}

	done := make(chan error)
	go func() {
		done <- prom.Wait()
	}()

	var startedOk bool
	var stoppedErr error

Loop:
	for x := 0; x < 10; x++ {
		// error=nil means prometheus has started so can send the interrupt signal and wait for the grace shutdown.
		if _, err := http.Get("http://localhost:9090/graph"); err == nil {
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
		t.Errorf("prometheus didn't start in the specified timeout")
		return
	}
	if err := prom.Process.Kill(); err == nil {
		t.Errorf("prometheus didn't shutdown gracefully after sending the Interrupt signal")
	} else if stoppedErr != nil && stoppedErr.Error() != "signal: interrupt" { // TODO - find a better way to detect when the process didn't exit as expected!
		t.Errorf("prometheus exited with an unexpected error:%v", stoppedErr)
	}
}

func TestComputeExternalURL(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{
			input: "",
			valid: true,
		},
		{
			input: "http://proxy.com/prometheus",
			valid: true,
		},
		{
			input: "'https://url/prometheus'",
			valid: false,
		},
		{
			input: "'relative/path/with/quotes'",
			valid: false,
		},
		{
			input: "http://alertmanager.company.com",
			valid: true,
		},
		{
			input: "https://double--dash.de",
			valid: true,
		},
		{
			input: "'http://starts/with/quote",
			valid: false,
		},
		{
			input: "ends/with/quote\"",
			valid: false,
		},
	}

	for _, test := range tests {
		_, err := computeExternalURL(test.input, "0.0.0.0:9090")
		if test.valid {
			testutil.Ok(t, err)
		} else {
			testutil.NotOk(t, err, "input=%q", test.input)
		}
	}
}
