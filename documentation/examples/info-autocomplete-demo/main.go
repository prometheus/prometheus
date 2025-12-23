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

// This demo starts Prometheus with the web UI to test autocomplete for the info() function.
// It sends OTLP metrics with resource attributes, then lets you interactively test
// autocomplete in the browser at http://localhost:9090.
//
// Run with: go run ./documentation/examples/info-autocomplete-demo
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/xpdata/entity"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/web"
)

const (
	webAddr  = "127.0.0.1:9090"
	otlpAddr = "127.0.0.1:9091"
	otlpPath = "/api/v1/otlp/v1/metrics"
)

func main() {
	fmt.Println("=== info() Autocomplete Demo ===")
	fmt.Println()

	if err := runDemo(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runDemo() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create temporary directory for TSDB data
	tmpDir, err := os.MkdirTemp("", "prometheus-info-autocomplete-demo-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	fmt.Printf("TSDB data directory: %s\n\n", tmpDir)

	// Create TSDB
	logger := promslog.New(&promslog.Config{})
	opts := tsdb.DefaultOptions()
	db, err := tsdb.Open(tmpDir, logger, nil, opts, nil)
	if err != nil {
		return fmt.Errorf("open TSDB: %w", err)
	}
	defer db.Close()

	// Create OTLP handler with resource attributes persistence enabled
	reg := prometheus.NewRegistry()
	configFunc := func() config.Config { return config.Config{} }
	otlpOpts := remote.OTLPOptions{
		LookbackDelta: 5 * time.Minute,
	}
	otlpHandler := remote.NewOTLPWriteHandler(logger, reg, db, configFunc, otlpOpts)

	// Start OTLP server on separate port
	otlpMux := http.NewServeMux()
	otlpMux.Handle(otlpPath, otlpHandler)
	otlpServer := &http.Server{Addr: otlpAddr, Handler: otlpMux}
	go func() {
		if err := otlpServer.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("OTLP server error: %v\n", err)
		}
	}()
	defer otlpServer.Shutdown(context.Background())

	// Create PromQL engine with LabelNamerConfig for info() function support
	engineOpts := promql.EngineOpts{
		Logger:               logger,
		Reg:                  prometheus.NewRegistry(),
		MaxSamples:           50000000,
		Timeout:              2 * time.Minute,
		EnableNegativeOffset: true,
		Parser:               parser.NewParser(parser.Options{EnableExperimentalFunctions: true}),
		// LabelNamerConfig enables translation of OTel attribute names to Prometheus label names
		// in the info() function output. Without this, info() would return OTel names like
		// "service.name" instead of Prometheus-compatible names like "service_name".
		LabelNamerConfig: &promql.LabelNamerConfig{
			UTF8Allowed:                 false, // Use legacy name sanitization
			UnderscoreLabelSanitization: false,
			PreserveMultipleUnderscores: false,
		},
	}
	engine := promql.NewEngine(engineOpts)

	// Create web handler with full UI
	externalURL, _ := url.Parse("http://" + webAddr)
	webOpts := &web.Options{
		Context:         ctx,
		ListenAddresses: []string{webAddr},
		ExternalURL:     externalURL,
		RoutePrefix:     "/",
		ReadTimeout:     30 * time.Second,
		MaxConnections:  512,

		Storage:      db,
		LocalStorage: &dbAdapter{db},
		TSDBDir:      tmpDir,

		QueryEngine: engine,
		// Minimal managers - only needed to satisfy web.Options interface.
		// Scrape/rule/alert endpoints won't work properly in this demo.
		ScrapeManager: &scrape.Manager{},
		RuleManager:   &rules.Manager{},
		LookbackDelta: 5 * time.Minute,

		Version:    &web.PrometheusVersion{Version: "demo"},
		Flags:      map[string]string{},
		Gatherer:   prometheus.DefaultGatherer,
		Registerer: prometheus.DefaultRegisterer,

		PageTitle: "Prometheus - info() Autocomplete Demo",
	}

	webHandler := web.New(logger.With("component", "web"), webOpts)
	if err := webHandler.ApplyConfig(&config.Config{}); err != nil {
		return fmt.Errorf("apply config: %w", err)
	}
	webHandler.SetReady(web.Ready)

	// Start web server
	listeners, err := webHandler.Listeners()
	if err != nil {
		return fmt.Errorf("create web listeners: %w", err)
	}

	go func() {
		if err := webHandler.Run(ctx, listeners, ""); err != nil {
			fmt.Printf("Web server error: %v\n", err)
		}
	}()

	// Wait for servers to start
	time.Sleep(500 * time.Millisecond)

	// Send OTLP metrics with resource attributes
	fmt.Println("Sending OTLP metrics with resource attributes...")
	metrics := createOTLPMetrics()
	if err := sendOTLPMetrics(metrics); err != nil {
		return fmt.Errorf("send OTLP metrics: %w", err)
	}
	fmt.Printf("Sent metrics from %d services\n\n", metrics.ResourceMetrics().Len())

	// Print testing instructions
	printInstructions()

	// Wait for shutdown signal
	fmt.Println("Press Ctrl+C to stop the demo")
	<-sigCh
	fmt.Println("\nShutting down...")

	return nil
}

// createOTLPMetrics creates OTLP metrics with diverse resource attributes for autocomplete testing.
func createOTLPMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	now := time.Now()

	// Service 1: api-gateway
	rm1 := md.ResourceMetrics().AppendEmpty()
	res1 := rm1.Resource()
	res1.Attributes().PutStr("service.name", "api-gateway")
	res1.Attributes().PutStr("service.namespace", "production")
	res1.Attributes().PutStr("service.instance.id", "gateway-001")
	res1.Attributes().PutStr("host.name", "prod-gateway-1.example.com")
	res1.Attributes().PutStr("cloud.region", "us-west-2")
	res1.Attributes().PutStr("cloud.provider", "aws")

	// Add entity_refs for proper identifying/descriptive classification
	entityRefs1 := entity.ResourceEntityRefs(res1)
	serviceRef1 := entityRefs1.AppendEmpty()
	serviceRef1.SetType("service")
	serviceRef1.IdKeys().Append("service.name")
	serviceRef1.IdKeys().Append("service.namespace")
	serviceRef1.IdKeys().Append("service.instance.id")
	serviceRef1.DescriptionKeys().Append("host.name")
	serviceRef1.DescriptionKeys().Append("cloud.region")
	serviceRef1.DescriptionKeys().Append("cloud.provider")

	sm1 := rm1.ScopeMetrics().AppendEmpty()
	m1 := sm1.Metrics().AppendEmpty()
	m1.SetName("http_requests_total")
	m1.SetDescription("Total HTTP requests")
	sum1 := m1.SetEmptySum()
	sum1.SetIsMonotonic(true)
	sum1.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	// Multiple data points with different labels
	dp1a := sum1.DataPoints().AppendEmpty()
	dp1a.SetTimestamp(pcommon.NewTimestampFromTime(now))
	dp1a.SetDoubleValue(1500)
	dp1a.Attributes().PutStr("method", "GET")
	dp1a.Attributes().PutStr("status", "200")

	dp1b := sum1.DataPoints().AppendEmpty()
	dp1b.SetTimestamp(pcommon.NewTimestampFromTime(now))
	dp1b.SetDoubleValue(50)
	dp1b.Attributes().PutStr("method", "POST")
	dp1b.Attributes().PutStr("status", "201")

	// Service 2: database
	rm2 := md.ResourceMetrics().AppendEmpty()
	res2 := rm2.Resource()
	res2.Attributes().PutStr("service.name", "database")
	res2.Attributes().PutStr("service.namespace", "production")
	res2.Attributes().PutStr("service.instance.id", "db-primary-001")
	res2.Attributes().PutStr("host.name", "prod-db-1.example.com")
	res2.Attributes().PutStr("cloud.region", "us-west-2")
	res2.Attributes().PutStr("db.system", "postgresql")

	entityRefs2 := entity.ResourceEntityRefs(res2)
	serviceRef2 := entityRefs2.AppendEmpty()
	serviceRef2.SetType("service")
	serviceRef2.IdKeys().Append("service.name")
	serviceRef2.IdKeys().Append("service.namespace")
	serviceRef2.IdKeys().Append("service.instance.id")
	serviceRef2.DescriptionKeys().Append("host.name")
	serviceRef2.DescriptionKeys().Append("cloud.region")
	serviceRef2.DescriptionKeys().Append("db.system")

	sm2 := rm2.ScopeMetrics().AppendEmpty()
	m2 := sm2.Metrics().AppendEmpty()
	m2.SetName("db_queries_total")
	m2.SetDescription("Total database queries")
	sum2 := m2.SetEmptySum()
	sum2.SetIsMonotonic(true)
	sum2.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	dp2 := sum2.DataPoints().AppendEmpty()
	dp2.SetTimestamp(pcommon.NewTimestampFromTime(now))
	dp2.SetDoubleValue(5000)
	dp2.Attributes().PutStr("operation", "SELECT")

	// Service 3: api-gateway in staging (different namespace, same service name)
	rm3 := md.ResourceMetrics().AppendEmpty()
	res3 := rm3.Resource()
	res3.Attributes().PutStr("service.name", "api-gateway")
	res3.Attributes().PutStr("service.namespace", "staging")
	res3.Attributes().PutStr("service.instance.id", "gateway-staging-001")
	res3.Attributes().PutStr("host.name", "staging-gateway-1.example.com")
	res3.Attributes().PutStr("cloud.region", "us-east-1")
	res3.Attributes().PutStr("cloud.provider", "aws")

	entityRefs3 := entity.ResourceEntityRefs(res3)
	serviceRef3 := entityRefs3.AppendEmpty()
	serviceRef3.SetType("service")
	serviceRef3.IdKeys().Append("service.name")
	serviceRef3.IdKeys().Append("service.namespace")
	serviceRef3.IdKeys().Append("service.instance.id")
	serviceRef3.DescriptionKeys().Append("host.name")
	serviceRef3.DescriptionKeys().Append("cloud.region")
	serviceRef3.DescriptionKeys().Append("cloud.provider")

	sm3 := rm3.ScopeMetrics().AppendEmpty()
	m3 := sm3.Metrics().AppendEmpty()
	m3.SetName("http_requests_total")
	m3.SetDescription("Total HTTP requests")
	sum3 := m3.SetEmptySum()
	sum3.SetIsMonotonic(true)
	sum3.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	dp3 := sum3.DataPoints().AppendEmpty()
	dp3.SetTimestamp(pcommon.NewTimestampFromTime(now))
	dp3.SetDoubleValue(100)
	dp3.Attributes().PutStr("method", "GET")
	dp3.Attributes().PutStr("status", "200")

	return md
}

func sendOTLPMetrics(md pmetric.Metrics) error {
	marshaler := pmetric.JSONMarshaler{}
	data, err := marshaler.MarshalMetrics(md)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	url := fmt.Sprintf("http://%s%s", otlpAddr, otlpPath)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func printInstructions() {
	fmt.Println("========================================")
	fmt.Println("Prometheus UI is running at:")
	fmt.Printf("  http://%s\n", webAddr)
	fmt.Println()
	fmt.Println("Test autocomplete for info() function:")
	fmt.Println()
	fmt.Println("1. LABEL NAME autocomplete:")
	fmt.Println("   Type: info(http_requests_total, {")
	fmt.Println("   Expected suggestions: service_name, service_namespace,")
	fmt.Println("   host_name, cloud_region, cloud_provider, db_system")
	fmt.Println()
	fmt.Println("2. LABEL VALUE autocomplete:")
	fmt.Println("   Type: info(http_requests_total, {service_name=\"")
	fmt.Println("   Expected suggestions: api-gateway, database")
	fmt.Println()
	fmt.Println("3. Execute a full query:")
	fmt.Println("   info(http_requests_total)")
	fmt.Println("   This returns metrics enriched with resource attributes")
	fmt.Println()
	fmt.Println("Available metrics:")
	fmt.Println("  - http_requests_total (api-gateway in prod & staging)")
	fmt.Println("  - db_queries_total (database in prod)")
	fmt.Println()
	fmt.Println("Resource attributes available:")
	fmt.Println("  - service_name: api-gateway, database")
	fmt.Println("  - service_namespace: production, staging")
	fmt.Println("  - host_name: prod-gateway-1.example.com, etc.")
	fmt.Println("  - cloud_region: us-west-2, us-east-1")
	fmt.Println("  - cloud_provider: aws")
	fmt.Println("  - db_system: postgresql (database only)")
	fmt.Println("========================================")
	fmt.Println()
}

// dbAdapter wraps tsdb.DB to implement web.LocalStorage interface.
type dbAdapter struct {
	*tsdb.DB
}

func (a *dbAdapter) BlockMetas() ([]tsdb.BlockMeta, error) {
	return a.DB.BlockMetas(), nil
}

func (a *dbAdapter) Stats(statsByLabelName string, limit int) (*tsdb.Stats, error) {
	return a.Head().Stats(statsByLabelName, limit), nil
}

func (*dbAdapter) WALReplayStatus() (tsdb.WALReplayStatus, error) {
	return tsdb.WALReplayStatus{}, nil
}

var (
	_ storage.Storage  = (*dbAdapter)(nil)
	_ web.LocalStorage = (*dbAdapter)(nil)
)
var _ web.LocalStorage = (*dbAdapter)(nil)
