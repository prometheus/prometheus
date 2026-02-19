package compliance

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender"
)

const (
	scrapeConfigTemplate = `
global:
  scrape_interval: 1s

remote_write:
  - url: "{{.RemoteWriteEndpointURL}}"
    protobuf_message: "{{.RemoteWriteMessage}}"
    send_exemplars: true
    queue_config:
      retry_on_http_429: true
    metadata_config:
      send: true

scrape_configs:
  - job_name: "{{.ScrapeTargetJobName}}"
    scrape_interval: 1s
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ["{{.ScrapeTargetHostPort}}"]
`
)

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

type internalPrometheus struct{}

func (p internalPrometheus) Name() string { return "internal-prometheus" }

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

	return sender.RunCommand(ctx, "../cmd/prometheus", nil,
		"go", "run", ".",
		"--web.listen-address=0.0.0.0:0",
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
		// Set important flags for the full remote write compliance:
		"--enable-feature=st-storage",
	)
}

var _ sender.Sender = internalPrometheus{}

// TestRemoteWriteSender runs remote write sender compliance tests defined in
// https://github.com/prometheus/compliance/tree/main/remotewrite/sender
func TestRemoteWriteSender(t *testing.T) {
	sender.RunTests(t, internalPrometheus{}, sender.ComplianceTests())
}
