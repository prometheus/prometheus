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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"text/template"

	"github.com/prometheus/compliance/remotewrite/sender"
)

const (
	scrapeConfigTemplate = `
global:
  scrape_interval: 1s

remote_write:
  - url: "{{.RemoteWriteEndpointURL}}"
    protobuf_message: "{{.RemoteWriteMessage}}"
{{- if .RemoteWriteMode}}
    mode: "{{.RemoteWriteMode}}"
{{- end}}
    send_exemplars: true
    send_native_histograms: true
    queue_config:
      retry_on_http_429: true
    metadata_config:
      send: true

scrape_configs:
  - job_name: "{{.ScrapeTargetJobName}}"
    scrape_interval: 1s
    scrape_native_histograms: true
{{- if .RemoteWriteMode}}
    convert_classic_histograms_to_nhcb: true
{{- end}}
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ["{{.ScrapeTargetHostPort}}"]
`
)

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

type internalPrometheus struct {
	binary    string
	agentMode bool
}

func (internalPrometheus) Name() string { return "internal-prometheus" }

// Run runs a <REPO>cmd/prometheus main package as a test sender target, until ctx is done.
func (p internalPrometheus) Run(ctx context.Context, opts sender.Options) error {
	var buf bytes.Buffer
	if err := scrapeConfigTmpl.Execute(&buf, opts); err != nil {
		return fmt.Errorf("failed to execute config template: %w", err)
	}

	dir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		return err
	}
	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, buf.Bytes(), 0o600); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	args := []string{
		"--web.listen-address=0.0.0.0:0",
		fmt.Sprintf("--config.file=%s", configFile),
	}
	if p.agentMode {
		// Agent mode: st-storage and metadata-wal-records (agent WAL does not use XOR2 chunks).
		args = append(args, "--enable-feature=st-storage,metadata-wal-records")
		args = append(args, fmt.Sprintf("--storage.agent.path=%v", dir), "--agent")
	} else {
		// Server mode: st-storage enables the ST-capable chunk encodings.
		args = append(args, "--enable-feature=st-storage,exemplar-storage,metadata-wal-records")
		args = append(args, fmt.Sprintf("--storage.tsdb.path=%v", dir))
	}
	return sender.RunCommand(ctx, ".", nil, p.binary, args...)
}

var _ sender.Sender = internalPrometheus{}

// senderSkip lists unimplemented SHOULD/MAY sender compliance checks to skip in TestMain:
// - Stateless exemplar coalescing (https://github.com/prometheus/prometheus/pull/20070).
// - Untyped metadata inference (https://github.com/prometheus/proposals/pull/104).
const senderSkip = "TestRemoteWriteSender/.*/internal-prometheus/exemplars/rw2-stateless.*/(_present|exemplar_additional_labels)" +
	"|TestRemoteWriteSender/.*/internal-prometheus/metadata/rw2-stateless.*/untyped_metadata_inferred"

// TestRemoteWriteSender runs remote write sender compliance tests defined in
// https://github.com/prometheus/compliance/tree/main/remotewrite/sender against
// both agent and server modes.
func TestRemoteWriteSender(t *testing.T) {
	bin := buildPrometheus(t)
	// sender.RunTests disambiguates each suite by protocol and mode (/rw2, /rw2-stateless, /rw2-stateless-st).
	tests := slices.Concat(
		sender.ComplianceTests(),
		sender.StatelessComplianceTests(),
		sender.StatelessSTComplianceTests(),
	)

	t.Run("mode=server", func(t *testing.T) {
		t.Parallel()
		sender.RunTests(t, internalPrometheus{binary: bin}, tests)
	})
	t.Run("mode=agent", func(t *testing.T) {
		t.Parallel()
		sender.RunTests(t, internalPrometheus{binary: bin, agentMode: true}, tests)
	})
}
