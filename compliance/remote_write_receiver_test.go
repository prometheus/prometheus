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
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/compliance/remotewrite/receiver"
)

const receiverConfig = `
storage:
  exemplars:
    max_exemplars: 100000
`

type internalPrometheusReceiver struct {
	binary string
}

func (internalPrometheusReceiver) Name() string { return "internal-prometheus" }

func (p internalPrometheusReceiver) Run(ctx context.Context, opts receiver.Options) error {
	dir, err := os.MkdirTemp("", "test-receiver-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(receiverConfig), 0o600); err != nil {
		return err
	}

	return receiver.RunCommand(ctx, ".", nil,
		p.binary,
		fmt.Sprintf("--web.listen-address=%s", opts.ReceiveHostPort),
		"--web.enable-remote-write-receiver",
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
		"--enable-feature=st-storage,exemplar-storage,metadata-wal-records",
	)
}

var _ receiver.Receiver = internalPrometheusReceiver{}

// receiverSkip lists unimplemented SHOULD/MAY receiver compliance checks to skip in TestMain
// (writeHandler.Store currently ignores WriteMode validation).
const receiverSkip = "TestRemoteWriteReceiver/internal-prometheus/modes/validation/.*_(classic_histogram_samples|untyped_metadata|without_start_timestamp)"

// TestRemoteWriteReceiver runs tests from https://github.com/prometheus/compliance/tree/main/remotewrite/receiver
// against the Prometheus remote write receiver.
func TestRemoteWriteReceiver(t *testing.T) {
	receiver.RunTests(
		t,
		internalPrometheusReceiver{binary: buildPrometheus(t)},
		receiver.ComplianceTests(),
	)
}
