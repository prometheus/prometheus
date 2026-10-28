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

package compliance

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/compliance/common"
	"github.com/stretchr/testify/require"
)

const (
	receiverPromConfig = `
storage:
  exemplars:
    max_exemplars: 100000
`
	// Skip receiver mode SHOULD rejections until implemented in Prometheus.
	receiverSkipTests = "TestModeValidation/.*_(classic_histogram_samples|untyped_metadata|without_start_timestamp)"
)

func startPrometheusReceiver(t *testing.T, bin string) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(receiverPromConfig), 0o600))

	runCtx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- common.RunCommand(runCtx, ".", nil,
			bin,
			fmt.Sprintf("--web.listen-address=%s", addr),
			"--web.enable-remote-write-receiver",
			fmt.Sprintf("--storage.tsdb.path=%v", dir),
			fmt.Sprintf("--config.file=%s", configFile),
			"--enable-feature=st-storage,exemplar-storage,metadata-wal-records",
		)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	readyURL := fmt.Sprintf("http://%s/-/ready", addr)
	require.Eventually(t, func() bool {
		select {
		case runErr := <-done:
			t.Fatalf("prometheus receiver exited prematurely: %v", runErr)
			return false
		default:
		}
		resp, reqErr := http.Get(readyURL)
		if reqErr != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 15*time.Second, 100*time.Millisecond, "timed out waiting for prometheus receiver at %s to become ready", readyURL)

	return fmt.Sprintf("http://%s/api/v1/write", addr)
}

// TestRemoteWriteReceiver runs tests from https://github.com/prometheus/compliance/tree/main/remotewrite/receiver
// against the Prometheus remote write receiver.
func TestRemoteWriteReceiver(t *testing.T) {
	bin := buildPrometheus(t)
	writeURL := startPrometheusReceiver(t, bin)

	rwConfigFile := filepath.Join(t.TempDir(), "rw-config.yml")
	rwConfig := fmt.Sprintf("remote_write:\n  - name: internal-prometheus\n    url: %s\n", writeURL)
	require.NoError(t, os.WriteFile(rwConfigFile, []byte(rwConfig), 0o600))

	cmd := exec.CommandContext(
		t.Context(),
		"go", "test", "-v",
		"-skip", receiverSkipTests,
		"github.com/prometheus/compliance/remotewrite/receiver",
	)
	cmd.Env = append(os.Environ(),
		"PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE="+rwConfigFile,
		"PROMETHEUS_RW2_COMPLIANCE_RECEIVERS=internal-prometheus",
	)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	require.NoError(t, err, "remote write receiver compliance tests failed")
}
