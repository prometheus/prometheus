package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/efficientgo/core/testutil"
	"github.com/efficientgo/e2e"
	e2einteractive "github.com/efficientgo/e2e/interactive"
	e2emon "github.com/efficientgo/e2e/monitoring"
)

const (
	promImage    = "quay.io/prometheus/prometheus:v3.0.0-rc.1"
	otelGenImage = "ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:v0.113.0"
)

func TestPrometheus_ReceivingOtel(t *testing.T) {
	e, err := e2e.New()
	t.Cleanup(e.Close)
	testutil.Ok(t, err)

	// Create self-scraping Prometheus that also receives OTLP.
	prom := newPrometheus(e, "prom-1", promImage, nil)

	// Create OpenTelemetry telemetrygen container.
	// https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/telemetrygen
	otelgen := newOtelGen(e, "otelgen-1", otelGenImage, prom.InternalEndpoint("http"), nil)
	testutil.Ok(t, e2e.StartAndWaitReady(prom, otelgen))

	//testutil.Ok(t, prom.WaitSumMetricsWithOptions(
	//	e2emon.Greater(expectSamples), []string{"prometheus_remote_storage_samples_total"},
	//	e2emon.WithLabelMatchers(&matchers.Matcher{Name: "remote_name", Value: "v2-to-sink", Type: matchers.MatchEqual}),
	//	e2emon.WithWaitBackoff(&backoff.Config{Min: 1 * time.Second, Max: 1 * time.Second, MaxRetries: 300}), // Wait 5m max.
	//))
	//testutil.Ok(t, prom.WaitSumMetricsWithOptions(
	//	e2emon.Greater(expectSamples), []string{"prometheus_remote_storage_samples_total"},
	//	e2emon.WithLabelMatchers(&matchers.Matcher{Name: "remote_name", Value: "v1-to-sink", Type: matchers.MatchEqual}),
	//))

	testutil.Ok(t, e2einteractive.OpenInBrowser("http://"+prom.Endpoint("http"))) // Open Prometheus UI.
	//testutil.Ok(t, e2einteractive.OpenInBrowser("http://"+otelgen.Endpoint("http")+"/metrics"))
	testutil.Ok(t, e2einteractive.RunUntilEndpointHit())
}

func newOtelGen(e e2e.Environment, name, image string, otlpEndpoint string, flagOverride map[string]string) e2e.Runnable {
	f := e.Runnable(name).Future()
	args := map[string]string{
		// https://opentelemetry.io/docs/specs/semconv/http/http-metrics/#metric-httpclientactive_requests
		"--metric-name":        "http.client.active_requests",
		"--rate":               "0.2",
		"--metrics":            "10000", // ~13h
		"--workers":            "1",
		"--otlp-insecure":      "",
		"--otlp-http":          "",
		"--otlp-endpoint":      otlpEndpoint,
		"--otlp-http-url-path": "/api/v1/otlp/v1/metrics",
		"--otlp-attributes":    "service.instance.id=\"my-laptop\"",
	}
	if flagOverride != nil {
		args = e2e.MergeFlagsWithoutRemovingEmpty(args, flagOverride)
	}

	return f.Init(e2e.StartOptions{
		Image: image,
		Command: e2e.NewCommand("metrics",
			append([]string{
				"--telemetry-attributes=server.address=\"top-secret🙈\"",
				"--telemetry-attributes=server.⚠️=\"🔥\"",
			}, e2e.BuildArgs(args)...)...),
		User: strconv.Itoa(os.Getuid()),
		//EnvVars: map[string]string{
		//	"OTEL_EXPORTER_OTLP_PROTOCOL":         "http/protobuf",
		//	"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "http://" + otlpEndpoint + "/api/v1/otlp/v1/metrics",
		//	"OTEL_SERVICE_NAME":                   "otelgen",
		//	"OTEL_RESOURCE_ATTRIBUTES":            "service.instance.id=my-laptop",
		//},
	})
}

func newPrometheus(env e2e.Environment, name, image string, flagOverride map[string]string) *e2emon.Prometheus {
	ports := map[string]int{"http": 9090}

	f := env.Runnable(name).WithPorts(ports).Future()
	config := fmt.Sprintf(`
global:
  external_labels:
    prometheus: %v
  metric_name_validation_scheme: utf8
otlp:
  promote_resource_attributes:
    - service.instance.id
  translation_strategy: "NoUTF8EscapingWithSuffixes"
scrape_configs:
- job_name: 'self'
  scrape_interval: 5s
  scrape_timeout: 5s
  static_configs:
  - targets: ['localhost:%v']

`, name, ports["http"])
	if err := os.WriteFile(filepath.Join(f.Dir(), "prometheus.yml"), []byte(config), 0o600); err != nil {
		return &e2emon.Prometheus{Runnable: e2e.NewFailedRunnable(name, fmt.Errorf("create prometheus config failed: %w", err))}
	}

	args := map[string]string{
		"--web.listen-address":                  fmt.Sprintf(":%d", ports["http"]),
		"--web.enable-otlp-receiver":            "",
		"--config.file":                         filepath.Join(f.Dir(), "prometheus.yml"),
		"--storage.tsdb.path":                   f.Dir(),
		"--enable-feature=exemplar-storage":     "",
		"--enable-feature=native-histograms":    "",
		"--enable-feature=metadata-wal-records": "",
		"--storage.tsdb.no-lockfile":            "",
		"--storage.tsdb.retention.time":         "1d",
		"--storage.tsdb.wal-compression":        "",
		"--storage.tsdb.min-block-duration":     "2h",
		"--storage.tsdb.max-block-duration":     "2h",
		"--web.enable-lifecycle":                "",
		"--log.format":                          "json",
		"--log.level":                           "info",
	}
	if flagOverride != nil {
		args = e2e.MergeFlagsWithoutRemovingEmpty(args, flagOverride)
	}

	p := e2emon.AsInstrumented(f.Init(e2e.StartOptions{
		Image:     image,
		Command:   e2e.NewCommandWithoutEntrypoint("prometheus", e2e.BuildArgs(args)...),
		Readiness: e2e.NewHTTPReadinessProbe("http", "/-/ready", 200, 200),
		User:      strconv.Itoa(os.Getuid()),
	}), "http")

	return &e2emon.Prometheus{
		Runnable:     p,
		Instrumented: p,
	}
}
