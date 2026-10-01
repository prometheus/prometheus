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
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/efficientgo/e2e"
	e2einteractive "github.com/efficientgo/e2e/interactive"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

const (
	defaultPromImage       = "prometheus:om2-bench"
	defaultJavaTargetImage = "europe-west3-docker.pkg.dev/macro-mile-203600/test/openmetrics2-java-demo:1.9.0"
)

// TestBenchmarkOpenMetricsScrape runs a side-by-side e2e benchmark comparing
// Prometheus scraping OpenMetrics 1.0 vs OpenMetrics 2.0 across a mix of
// Go (client_golang, 1,000 metric series/target) and Java (client_java Docker)
// targets, monitored via efficientgo/e2e/monitoring (e2emon).
//
// Recommended CLI invocation:
//
//	go test -v -timeout 30m -run '^TestBenchmarkOpenMetricsScrape$' .
//
// Optional environment variables:
//   - PROMETHEUS_IMAGE: Docker image with --enable-feature=openmetrics2 (default: "prometheus:om2-bench", built automatically from ../cmd/prometheus if missing).
//   - JAVA_TARGET_IMAGE: Docker image for client_java OM2 target (default: "europe-west3-docker.pkg.dev/macro-mile-203600/test/openmetrics2-java-demo:1.9.0").
//   - BENCH_TARGETS: Number of concurrent scrape targets per SDK job (default: 25 Go + 25 Java = 50 total).
//   - BENCH_DURATION: Steady-state measurement duration (default: "45s").
//   - BENCH_NHCB: Set to "true" to enable convert_classic_histograms_to_nhcb and scrape_native_histograms (default: "false").
//   - BENCH_INTERACTIVE: Set to "true" to open monitoring Prometheus UI and block until endpoint hit.
func TestBenchmarkOpenMetricsScrape(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e benchmark in short mode")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("skipping e2e benchmark because docker daemon is unavailable: %v", err)
	}

	promImage := envOrDefault("PROMETHEUS_IMAGE", defaultPromImage)
	ensurePrometheusBenchImage(t, promImage)

	javaImage := envOrDefault("JAVA_TARGET_IMAGE", defaultJavaTargetImage)
	numTargetsPerJob := envIntOrDefault(t, "BENCH_TARGETS", 25)
	benchDuration := envDurationOrDefault(t, "BENCH_DURATION", 45*time.Second)
	convertNHCB := envBoolOrDefault(t, "BENCH_NHCB", false)
	interactive := envBoolOrDefault(t, "BENCH_INTERACTIVE", false)
	const scrapeInterval = 1 * time.Second

	env, err := e2e.New()
	require.NoError(t, err)
	t.Cleanup(env.Close)

	// 1. Start the Go (client_golang) multi-target HTTP server on the host (1,000 series/target).
	goTargetSrv := startBenchTargetServer(t, env, numTargetsPerJob, scrapeInterval)

	// 2. Start isolated Java (client_java) OpenMetrics 2.0 target containers in Docker
	// for each scraper so OM1 and OM2 scrapes do not contend on the same JVM HTTP thread pool.
	javaTargetOM1 := env.Runnable("java-target-om1").
		WithPorts(map[string]int{"http": 9400}).
		Init(e2e.StartOptions{
			Image:     javaImage,
			Readiness: e2e.NewHTTPReadinessProbe("http", "/-/healthy", 200, 200),
		})
	javaTargetOM2 := env.Runnable("java-target-om2").
		WithPorts(map[string]int{"http": 9400}).
		Init(e2e.StartOptions{
			Image:     javaImage,
			Readiness: e2e.NewHTTPReadinessProbe("http", "/-/healthy", 200, 200),
		})
	require.NoError(t, e2e.StartAndWaitReady(javaTargetOM1, javaTargetOM2))

	// 3. Start e2emon monitoring service (scrapes all AsInstrumented runnables).
	mon, err := e2emon.Start(
		env,
		e2emon.WithScrapeInterval(2*time.Second),
		e2emon.WithPrometheusImage(promImage),
		e2emon.WithCadvisorDisabled(),
	)
	require.NoError(t, err)

	// 4. Start side-by-side Prometheus scrapers (prom-om1 and prom-om2) scraping both
	// the Go and Java target pools.
	promOM1 := newBenchPrometheus(
		t, env, "prom-om1", promImage,
		goTargetSrv.InternalTargets(env),
		javaTargetOM1.InternalEndpoint("http"),
		numTargetsPerJob,
		"OpenMetricsText1.0.0",
		scrapeInterval,
		convertNHCB,
	)
	promOM2 := newBenchPrometheus(
		t, env, "prom-om2", promImage,
		goTargetSrv.InternalTargets(env),
		javaTargetOM2.InternalEndpoint("http"),
		numTargetsPerJob,
		"OpenMetricsText2.0.0",
		scrapeInterval,
		convertNHCB,
	)
	require.NoError(t, e2e.StartAndWaitReady(promOM1, promOM2))

	monEndpoint := "http://" + mon.GetMonitoringRunnable().Endpoint("http")
	t.Logf("Monitoring Prometheus UI: %s", monEndpoint)
	t.Logf("prom-om1 UI: http://%s | prom-om2 UI: http://%s",
		promOM1.Endpoint("http"), promOM2.Endpoint("http"))
	t.Logf("Running steady-state scrape benchmark for %s (go_targets=%d, java_targets=%d, interval=%s, nhcb=%v)...",
		benchDuration, numTargetsPerJob, numTargetsPerJob, scrapeInterval, convertNHCB)

	// Wait 10 seconds for initial scrapes and e2emon self-scrapes to reach steady state,
	// then reset wire byte counters so the [benchDuration] window is 100% steady state.
	time.Sleep(10 * time.Second)
	goTargetSrv.ResetWireStats()

	time.Sleep(benchDuration)

	printComparisonReport(t, monEndpoint, promOM1, promOM2, goTargetSrv, numTargetsPerJob, benchDuration, convertNHCB)

	if interactive {
		require.NoError(t, mon.OpenUserInterfaceInBrowser())
		require.NoError(t, e2einteractive.RunUntilEndpointHit())
	}
}

// ensurePrometheusBenchImage builds the local prometheus:om2-bench Docker image
// from ../cmd/prometheus if it does not already exist in the local Docker daemon.
func ensurePrometheusBenchImage(t *testing.T, image string) {
	t.Helper()
	if image != defaultPromImage {
		return
	}
	if err := exec.Command("docker", "image", "inspect", image).Run(); err == nil {
		return
	}

	t.Logf("Building %s from ../cmd/prometheus...", image)
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "prometheus")

	buildCmd := exec.Command("go", "build", "-trimpath", "-o", binPath, "../cmd/prometheus")
	buildCmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	require.NoError(t, buildCmd.Run(), "failed to build ../cmd/prometheus")

	// NOTE(bwplotka): e2emon sends `kill -SIGHUP 1` immediately upon container start
	// before WaitReady() completes. Wrap /bin/kill to wait until port 9090 is listening
	// (when main.go has registered its SIGHUP handler) and retry on transient permission errors.
	dockerfile := `FROM quay.io/prometheus/busybox-linux-amd64:latest
COPY prometheus /bin/prometheus
RUN rm -f /bin/kill && printf '#!/bin/sh\nfor i in $(seq 1 100); do\n  if nc -z 127.0.0.1 9090 >/dev/null 2>&1; then\n    break\n  fi\n  sleep 0.05\ndone\nfor i in $(seq 1 50); do\n  if /bin/busybox kill "$@" 2>/dev/null; then\n    exit 0\n  fi\n  sleep 0.05\ndone\nexec /bin/busybox kill "$@"\n' > /bin/kill && chmod +x /bin/kill
EXPOSE 9090
ENTRYPOINT ["/bin/prometheus"]
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0o600))

	dockerCmd := exec.Command("docker", "build", "-t", image, tmpDir)
	dockerCmd.Stdout = os.Stdout
	dockerCmd.Stderr = os.Stderr
	require.NoError(t, dockerCmd.Run(), "failed to build docker image %s", image)
}

type benchTargetServer struct {
	port       string
	numTargets int

	om1Scrapes   atomic.Int64
	om1WireBytes atomic.Int64
	om2Scrapes   atomic.Int64
	om2WireBytes atomic.Int64
}

type countingResponseWriter struct {
	http.ResponseWriter
	written int64
}

func (w *countingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)
	return n, err
}

func startBenchTargetServer(t *testing.T, env e2e.Environment, numTargets int, tickInterval time.Duration) *benchTargetServer {
	t.Helper()

	srv := &benchTargetServer{
		numTargets: numTargets,
	}

	mux := http.NewServeMux()

	type targetUpdater func(tick uint64)
	updaters := make([]targetUpdater, 0, numTargets)

	for targetIdx := range numTargets {
		reg := prometheus.NewRegistry()

		counters := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "bench_http_requests_total",
			Help: "Total number of simulated HTTP requests.",
		}, []string{"method", "status", "handler"})

		gauges := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "bench_memory_usage_bytes",
			Help: "Simulated memory usage in bytes.",
		}, []string{"region", "shard"})

		histograms := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "bench_http_request_duration_seconds",
			Help:    "Simulated HTTP request latency distribution.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		}, []string{"handler", "status"})

		summaries := prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name:       "bench_rpc_duration_seconds",
			Help:       "Simulated RPC latency summary.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, []string{"service", "rpc"})

		reg.MustRegister(counters, gauges, histograms, summaries)

		// Pre-bind 1,000 series handles per Go target so the background tick loop does not allocate label slices.
		const (
			numCounterSeries   = 400
			numGaugeSeries     = 400
			numHistogramSeries = 100
			numSummarySeries   = 100
		)
		counterHandles := make([]prometheus.Counter, numCounterSeries)
		for i := range numCounterSeries {
			counterHandles[i] = counters.WithLabelValues("GET", strconv.Itoa(200+(i%5)), fmt.Sprintf("/api/v1/item/%d", i))
		}
		gaugeHandles := make([]prometheus.Gauge, numGaugeSeries)
		for i := range numGaugeSeries {
			gaugeHandles[i] = gauges.WithLabelValues(fmt.Sprintf("region-%d", i%4), strconv.Itoa(i))
		}
		histHandles := make([]prometheus.Observer, numHistogramSeries)
		for i := range numHistogramSeries {
			histHandles[i] = histograms.WithLabelValues(fmt.Sprintf("handler_%d", i), "200")
		}
		summaryHandles := make([]prometheus.Observer, numSummarySeries)
		for i := range numSummarySeries {
			summaryHandles[i] = summaries.WithLabelValues(fmt.Sprintf("svc_%d", i%5), fmt.Sprintf("Method%d", i))
		}

		updateFn := func(tick uint64) {
			traceLabels := prometheus.Labels{"trace_id": fmt.Sprintf("trace-%d-%d", targetIdx, tick)}
			for i, c := range counterHandles {
				inc := float64((tick+uint64(i))%7 + 1)
				if adder, ok := c.(prometheus.ExemplarAdder); ok {
					adder.AddWithExemplar(inc, traceLabels)
				} else {
					c.Add(inc)
				}
			}
			for i, g := range gaugeHandles {
				g.Set(float64(1024*1024) + float64((tick*13+uint64(i)*7)%65536))
			}
			for i, h := range histHandles {
				obs := 0.005 * float64((tick+uint64(i))%200+1)
				if eo, ok := h.(prometheus.ExemplarObserver); ok {
					eo.ObserveWithExemplar(obs, traceLabels)
				} else {
					h.Observe(obs)
				}
			}
			for i, s := range summaryHandles {
				s.Observe(0.01 * float64((tick+uint64(i))%100+1))
			}
		}
		// Initialize series at tick 1 before any scrape arrives.
		updateFn(1)
		updaters = append(updaters, updateFn)

		h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
			EnableOpenMetrics:                   true,
			EnableOpenMetricsTextCreatedSamples: true,
			AcceptedFormats: []expfmt.Format{
				expfmt.FmtOpenMetrics_2_0_0,
				expfmt.FmtOpenMetrics_1_0_0,
				expfmt.FmtText,
			},
		})

		path := fmt.Sprintf("/targets/%d/metrics", targetIdx)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			cw := &countingResponseWriter{ResponseWriter: w}
			h.ServeHTTP(cw, r)
			ct := cw.Header().Get("Content-Type")
			if strings.Contains(ct, "version=2.0.0") {
				srv.om2Scrapes.Add(1)
				srv.om2WireBytes.Add(cw.written)
			} else if strings.Contains(ct, "version=1.0.0") {
				srv.om1Scrapes.Add(1)
				srv.om1WireBytes.Add(cw.written)
			}
		})
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		var tick uint64 = 2
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, u := range updaters {
					u(tick)
				}
				tick++
			}
		}
	}()

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	srv.port = port

	httpSrv := &http.Server{Handler: mux}
	go func() { _ = httpSrv.Serve(listener) }()
	env.AddCloser(func() { _ = httpSrv.Close() })

	return srv
}

func (s *benchTargetServer) InternalTargets(env e2e.Environment) []string {
	hostPort := net.JoinHostPort(env.HostAddr(), s.port)
	targets := make([]string, s.numTargets)
	for i := range s.numTargets {
		targets[i] = fmt.Sprintf("%s|%d", hostPort, i)
	}
	return targets
}

func (s *benchTargetServer) ResetWireStats() {
	s.om1Scrapes.Store(0)
	s.om1WireBytes.Store(0)
	s.om2Scrapes.Store(0)
	s.om2WireBytes.Store(0)
}

func newBenchPrometheus(
	t *testing.T,
	env e2e.Environment,
	name string,
	image string,
	goTargets []string,
	javaTargetInternalEndpoint string,
	numJavaVirtualTargets int,
	scrapeProtocol string,
	scrapeInterval time.Duration,
	convertNHCB bool,
) *e2emon.Prometheus {
	t.Helper()

	ports := map[string]int{"http": 9090}
	f := env.Runnable(name).WithPorts(ports).Future()

	var goStaticConfigs strings.Builder
	for _, tgt := range goTargets {
		parts := strings.SplitN(tgt, "|", 2)
		addr, idx := parts[0], parts[1]
		fmt.Fprintf(&goStaticConfigs, "  - targets: ['%s']\n    labels:\n      __metrics_path__: '/targets/%s/metrics'\n      target_idx: '%s'\n", addr, idx, idx)
	}

	var javaStaticConfigs strings.Builder
	for idx := range numJavaVirtualTargets {
		fmt.Fprintf(&javaStaticConfigs, "  - targets: ['%s']\n    labels:\n      __metrics_path__: '/metrics'\n      target_idx: '%d'\n", javaTargetInternalEndpoint, idx)
	}

	config := fmt.Sprintf(`
global:
  extra_scrape_metrics: true
  scrape_native_histograms: %v
  convert_classic_histograms_to_nhcb: %v
  external_labels:
    bench_instance: %s
scrape_configs:
- job_name: 'go-bench'
  scrape_interval: %s
  scrape_timeout: %s
  scrape_protocols:
  - %s
  static_configs:
%s
- job_name: 'java-bench'
  scrape_interval: %s
  scrape_timeout: %s
  scrape_protocols:
  - %s
  static_configs:
%s
`,
		convertNHCB, convertNHCB, name,
		scrapeInterval, scrapeInterval, scrapeProtocol, goStaticConfigs.String(),
		scrapeInterval, scrapeInterval, scrapeProtocol, javaStaticConfigs.String(),
	)

	require.NoError(t, os.WriteFile(filepath.Join(f.Dir(), "prometheus.yml"), []byte(config), 0o600))

	args := map[string]string{
		"--web.listen-address":              fmt.Sprintf(":%d", ports["http"]),
		"--config.file":                     filepath.Join(f.Dir(), "prometheus.yml"),
		"--storage.tsdb.path":               f.Dir(),
		"--enable-feature=openmetrics2":     "",
		"--enable-feature=st-storage":       "",
		"--enable-feature=exemplar-storage": "",
		"--storage.tsdb.no-lockfile":        "",
		"--storage.tsdb.retention.time":     "1d",
		"--storage.tsdb.wal-compression":    "",
		"--storage.tsdb.min-block-duration": "2h",
		"--storage.tsdb.max-block-duration": "2h",
		"--web.enable-lifecycle":            "",
		"--log.level":                       "warn",
	}

	p := e2emon.AsInstrumented(f.Init(e2e.StartOptions{
		Image:     image,
		Command:   e2e.NewCommandWithoutEntrypoint("prometheus", e2e.BuildArgs(args)...),
		Readiness: e2e.NewHTTPReadinessProbe("http", "/-/ready", 200, 200),
		User:      strconv.Itoa(os.Getuid()),
		EnvVars: map[string]string{
			"GOMAXPROCS": "2",
		},
	}), "http")

	return &e2emon.Prometheus{
		Runnable:     p,
		Instrumented: p,
	}
}

func printComparisonReport(
	t *testing.T,
	monEndpoint string,
	promOM1, promOM2 *e2emon.Prometheus,
	goTargetSrv *benchTargetServer,
	numTargetsPerJob int,
	window time.Duration,
	convertNHCB bool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	monAPI := newPromAPI(t, monEndpoint)
	om1API := newPromAPI(t, "http://"+promOM1.Endpoint("http"))
	om2API := newPromAPI(t, "http://"+promOM2.Endpoint("http"))

	// Sanity check that all Go and Java targets are up (1) in both Prometheus instances.
	require.Equal(t, float64(numTargetsPerJob), queryScalar(ctx, t, om1API, `sum(up{job="go-bench"})`), "prom-om1 go-bench targets not all up")
	require.Equal(t, float64(numTargetsPerJob), queryScalar(ctx, t, om2API, `sum(up{job="go-bench"})`), "prom-om2 go-bench targets not all up")
	require.Equal(t, float64(numTargetsPerJob), queryScalar(ctx, t, om1API, `sum(up{job="java-bench"})`), "prom-om1 java-bench targets not all up")
	require.Equal(t, float64(numTargetsPerJob), queryScalar(ctx, t, om2API, `sum(up{job="java-bench"})`), "prom-om2 java-bench targets not all up")

	winStr := fmt.Sprintf("%ds", int(window.Seconds()))

	type metricRow struct {
		label  string
		unit   string
		om1Val float64
		om2Val float64
	}

	var rows []metricRow
	addMonQuery := func(label, unit, exprTemplate string, scale float64) {
		q1 := fmt.Sprintf(exprTemplate, "prom-om1", winStr)
		q2 := fmt.Sprintf(exprTemplate, "prom-om2", winStr)
		v1 := queryScalar(ctx, t, monAPI, q1) * scale
		v2 := queryScalar(ctx, t, monAPI, q2) * scale
		rows = append(rows, metricRow{label: label, unit: unit, om1Val: v1, om2Val: v2})
	}
	addDirectQuery := func(label, unit, expr string, scale float64) {
		v1 := queryScalar(ctx, t, om1API, expr) * scale
		v2 := queryScalar(ctx, t, om2API, expr) * scale
		rows = append(rows, metricRow{label: label, unit: unit, om1Val: v1, om2Val: v2})
	}

	// Go target wire transfer stats from host HTTP server.
	om1Scrapes := max(goTargetSrv.om1Scrapes.Load(), 1)
	om2Scrapes := max(goTargetSrv.om2Scrapes.Load(), 1)
	rows = append(rows, metricRow{
		label:  "[Go SDK] Gzipped Wire Payload / Scrape",
		unit:   "KB",
		om1Val: float64(goTargetSrv.om1WireBytes.Load()) / float64(om1Scrapes) / 1024.0,
		om2Val: float64(goTargetSrv.om2WireBytes.Load()) / float64(om2Scrapes) / 1024.0,
	})

	// Per-job scrape metrics directly from prom-om1 and prom-om2.
	addDirectQuery("[Go SDK] Uncompressed Body Size / Scrape", "KB", `avg(scrape_body_size_bytes{job="go-bench"})`, 1.0/1024.0)
	addDirectQuery("[Go SDK] Samples Scraped / Target", "samples", `avg(scrape_samples_scraped{job="go-bench"})`, 1.0)
	addDirectQuery("[Go SDK] Avg Scrape Duration", "ms", fmt.Sprintf(`avg(avg_over_time(scrape_duration_seconds{job="go-bench"}[%s]))`, winStr), 1000.0)

	addDirectQuery("[Java SDK] Uncompressed Body Size / Scrape", "KB", `avg(scrape_body_size_bytes{job="java-bench"})`, 1.0/1024.0)
	addDirectQuery("[Java SDK] Samples Scraped / Target", "samples", `avg(scrape_samples_scraped{job="java-bench"})`, 1.0)
	addDirectQuery("[Java SDK] Avg Scrape Duration", "ms", fmt.Sprintf(`avg(avg_over_time(scrape_duration_seconds{job="java-bench"}[%s]))`, winStr), 1000.0)

	// Overall Process / Go runtime / TSDB metrics from e2emon monitoring Prometheus.
	addMonQuery("[Total] Process CPU Usage", "cores", `rate(process_cpu_seconds_total{job=%q}[%s])`, 1.0)
	addMonQuery("[Total] Go Heap Alloc Rate", "MB/s", `rate(go_memstats_alloc_bytes_total{job=%q}[%s])`, 1.0/(1024.0*1024.0))
	addMonQuery("[Total] Go Heap In-Use", "MB", `avg_over_time(go_memstats_heap_inuse_bytes{job=%q}[%s])`, 1.0/(1024.0*1024.0))
	addMonQuery("[Total] Process Resident Memory (RSS)", "MB", `avg_over_time(process_resident_memory_bytes{job=%q}[%s])`, 1.0/(1024.0*1024.0))
	addMonQuery("[Total] Go GC Cycles Rate", "gc/s", `rate(go_gc_duration_seconds_count{job=%q}[%s])`, 1.0)
	addMonQuery("[Total] TSDB Active Series (Head)", "series", `max_over_time(prometheus_tsdb_head_series{job=%q}[%s])`, 1.0)
	addMonQuery("[Total] TSDB Float Samples Appended", "samples/s", `rate(prometheus_tsdb_head_samples_appended_total{job=%q,type="float"}[%s])`, 1.0)
	addMonQuery("[Total] TSDB Histogram Samples Appended", "samples/s", `rate(prometheus_tsdb_head_samples_appended_total{job=%q,type="histogram"}[%s])`, 1.0)
	addMonQuery("[Total] TSDB Exemplars Appended", "exemplars/s", `rate(prometheus_tsdb_exemplar_exemplars_appended_total{job=%q}[%s])`, 1.0)

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n=== OpenMetrics 1.0 vs OpenMetrics 2.0 E2E Scrape Benchmark (window=%s, nhcb=%v) ===\n\n", window, convertNHCB)
	fmt.Fprintf(&sb, "| Metric | OM 1.0 (`prom-om1`) | OM 2.0 (`prom-om2`) | Diff (OM2 vs OM1) |\n")
	fmt.Fprintf(&sb, "| :--- | ---: | ---: | ---: |\n")
	for _, r := range rows {
		diffStr := "n/a"
		if r.om1Val != 0 && !math.IsNaN(r.om1Val) && !math.IsNaN(r.om2Val) {
			pct := ((r.om2Val - r.om1Val) / r.om1Val) * 100.0
			diffStr = fmt.Sprintf("%+.2f%%", pct)
		}
		fmt.Fprintf(&sb, "| %s | %.2f %s | %.2f %s | %s |\n", r.label, r.om1Val, r.unit, r.om2Val, r.unit, diffStr)
	}
	t.Log(sb.String())
}

func newPromAPI(t *testing.T, address string) v1.API {
	t.Helper()
	cl, err := api.NewClient(api.Config{Address: address})
	require.NoError(t, err)
	return v1.NewAPI(cl)
}

func queryScalar(ctx context.Context, t *testing.T, promAPI v1.API, query string) float64 {
	t.Helper()
	val, _, _, err := promAPI.Query(ctx, query, time.Now())
	require.NoError(t, err, "query failed: %s", query)
	vec, ok := val.(model.Vector)
	if !ok || len(vec) == 0 {
		return 0
	}
	return float64(vec[0].Value)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(t *testing.T, key string, def int) int {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	require.NoError(t, err)
	return n
}

func envDurationOrDefault(t *testing.T, key string, def time.Duration) time.Duration {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	require.NoError(t, err)
	return d
}

func envBoolOrDefault(t *testing.T, key string, def bool) bool {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	require.NoError(t, err)
	return b
}
