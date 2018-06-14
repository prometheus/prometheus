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
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	sd_config "github.com/prometheus/prometheus/discovery/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/prometheus/prometheus/web"
	k8s_runtime "k8s.io/apimachinery/pkg/util/runtime"
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

// As soon as prometheus starts responding to http request should be able to accept Interrupt signals for a graceful shutdown.
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

// Let's provide an invalid configuration file and verify the exit status indicates the error.
func TestFailedStartupExitCode(t *testing.T) {
	fakeInputFile := "fake-input-file"
	expectedExitStatus := 1

	prom := exec.Command(promPath, "--config.file="+fakeInputFile)
	err := prom.Run()
	testutil.NotOk(t, err, "")

	if exitError, ok := err.(*exec.ExitError); ok {
		status := exitError.Sys().(syscall.WaitStatus)
		testutil.Equals(t, expectedExitStatus, status.ExitStatus())
	} else {
		t.Errorf("unable to retrieve the exit status for prometheus: %v", err)
	}
}

// TestApplyConfig ensures that no component is modifying the global config.
// The config includes many pointers so modifying it has global side effects.
func TestApplyConfig(t *testing.T) {
	// Disable k8s logs.
	k8s_runtime.ErrorHandlers = []func(error){}

	var (
		cfgOrig          *config.Config
		ctx, cancel      = context.WithCancel(context.Background())
		logger           = log.NewNopLogger()
		notifyManager    = notifier.NewManager(&notifier.Options{}, logger)
		discoveryManager = discovery.NewManager(ctx, logger)
		scrapeManager    = scrape.NewManager(logger, nil)
		webHandler       = web.New(logger, &web.Options{RoutePrefix: "/"})
		remoteStorage    = remote.NewStorage(logger, nil, time.Duration(1*time.Millisecond))
	)
	defer cancel() // Just in case to avoid goroutine leaks.

	// Load the config in all components.
	reloaders := []func(cfg *config.Config) error{
		// Save the config so we can compare it with a new untouched copy.
		// Since this is a pointer any modification will be reflected in the saved copy.
		func(cfg *config.Config) error {
			cfgOrig = cfg
			return nil
		},
		remoteStorage.ApplyConfig,
		webHandler.ApplyConfig,
		notifyManager.ApplyConfig,
		scrapeManager.ApplyConfig,
		func(cfg *config.Config) error {
			c := make(map[string]sd_config.ServiceDiscoveryConfig)
			for _, v := range cfg.ScrapeConfigs {
				c[v.JobName] = v.ServiceDiscoveryConfig
			}
			return discoveryManager.ApplyConfig(c)
		},
	}
	testutil.Ok(t, reloadConfig("../../config/testdata/conf.good.yml", logger, reloaders...))

	// Need to reload the file again so we have a completely new untouched copy.
	reloaders = []func(cfg *config.Config) error{
		func(cfg *config.Config) error {
			testutil.Equals(t, cfgOrig, cfg)
			return nil
		},
	}
	testutil.Ok(t, reloadConfig("../../config/testdata/conf.good.yml", logger, reloaders...))
}
