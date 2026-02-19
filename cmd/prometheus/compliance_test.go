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

//go:build compliance

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runCommand executes a command in a specific directory
func runCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	if len(args) == 0 {
		t.Fatal("no command to execute")
	}

	t.Log("Executing", args, "in dir", dir)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v failed: %s", args, err)
	}
}

func setupComplianceRepo(t *testing.T) (dir string) {
	repo := os.Getenv("COMPLIANCE_REPO")

	// Use sanitized repo URL as a directory, so it's clear where is the test source.
	repoSanitized := strings.TrimPrefix(repo, "https://")
	repoSanitized = strings.ReplaceAll(repoSanitized, ".", "_")
	repoSanitized = strings.ReplaceAll(repoSanitized, "/", "_")
	dir = filepath.Join(t.TempDir(), repoSanitized)
	dir, err := filepath.Abs(dir)
	require.NoError(t, err)

	runCommand(t, "", "git", "clone", repo, dir)
	runCommand(t, dir, "git", "checkout", os.Getenv("COMPLIANCE_VERSION"))
	return dir
}

// TestComplianceRWSender runs $COMPLIANCE_REPO@$COMPLIANCE_VERSION/remotewrite/sender tests against main().
//
// This tests assumes that:
// * It was executed from the Prometheus repo root, similar to other main tests.
// * TestMain has special case for -test.main that executes main().
func TestComplianceRWSender(t *testing.T) {
	os.Setenv("COMPLIANCE_REPO", "https://github.com/prometheus/compliance.git")
	os.Setenv("COMPLIANCE_VERSION", "0e0f72d49c46911df26c279f706949601ef3a44e")
	dir := setupComplianceRepo(t)
	dir = filepath.Join(dir, "remotewrite")

	// Prepare Prometheus config to use.
	// It has to be templated with the per test vars for scrape target and RW endpoint.
	// Environment variables should match what will be injected:
	// https://github.com/prometheus/compliance/blob/52166be6e0dab23c802fae66aa250eae18babb4a/remotewrite/sender/targets/process.go#L23
	const configTemplate = `
global:
  scrape_interval: 1s
remote_write:
  - url: "${PROMETHEUS_COMPLIANCE_RW_TARGET_REMOTE_WRITE_ENDPOINT}"
    protobuf_message: "${PROMETHEUS_COMPLIANCE_RW_TARGET_REMOTE_WRITE_MESSAGE}"
    send_exemplars: true
    queue_config:
      retry_on_http_429: true
    metadata_config:
      send: true
scrape_configs:
  - job_name: "${PROMETHEUS_COMPLIANCE_RW_TARGET_SCRAPE_JOB_NAME}"
    scrape_interval: 1s
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ["${PROMETHEUS_COMPLIANCE_RW_TARGET_SCRAPE_HOST_PORT}"]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.tmpl.yaml"), []byte(configTemplate), 0o644))

	// Prepare Prometheus init script. It runs the main() with the desired flags.
	// Configuration is env expanded for target variables mentioned above.
	startScriptCmd := filepath.Join(dir, "start.sh")
	startScript := fmt.Sprintf(`#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset

echo "Preparing configuration!"
envsubst < %v/config.tmpl.yaml > %v/config.yaml

echo "Starting Prometheus..."
%v -test.main --web.listen-address=0.0.0.0:0 --config.file=%v/config.yaml
`, dir, dir, promPath, dir)
	require.NoError(t, os.WriteFile(startScriptCmd, []byte(startScript), 0o644))
	require.NoError(t, os.Chmod(startScriptCmd, 0o755))

	// Run compliance tests using compliance test Makefile.
	// Use "process" based sender with our start script injected.
	opts := []string{
		"DEBUG=1", // Configure for debug output (e.g. Prometheus logs).
		// `PROMETHEUS_RW_COMPLIANCE_TEST_RE="regex"`, // Configure for the focus certain test for debugging.
		// `PROMETHEUS_RW_COMPLIANCE_SKIP_TEST_RE="regex"`, // Configure for tests to skip e.g. knowingly violating optional spec requirements.
		`PROMETHEUS_COMPLIANCE_RW_SENDERS="process"`,
		fmt.Sprintf(`PROMETHEUS_COMPLIANCE_RW_PROCESS_CMD="%s"`, startScriptCmd),
	}
	fmt.Println(dir)
	runCommand(t, dir, append([]string{"make", "sender"}, opts...)...)
}
