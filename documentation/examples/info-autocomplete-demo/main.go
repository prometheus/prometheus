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
// It appends metrics directly to the TSDB including target_info metrics, then lets you
// interactively test autocomplete in the browser at http://localhost:9090.
//
// Run with: go run ./documentation/examples/info-autocomplete-demo
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/web"
)

const webAddr = "127.0.0.1:9090"

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

	// Append test metrics directly to TSDB
	fmt.Println("Appending test metrics to TSDB...")
	if err := appendTestMetrics(db); err != nil {
		return fmt.Errorf("append test metrics: %w", err)
	}
	fmt.Println("Test metrics appended successfully")
	fmt.Println()

	// Continuously append samples to keep them fresh (every 15 seconds)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := appendTestMetrics(db); err != nil {
					fmt.Printf("Warning: failed to refresh metrics: %v\n", err)
				}
			}
		}
	}()

	// Create PromQL engine with experimental functions enabled (required for info())
	engineOpts := promql.EngineOpts{
		Logger:               logger,
		Reg:                  prometheus.NewRegistry(),
		MaxSamples:           50000000,
		Timeout:              2 * time.Minute,
		EnableNegativeOffset: true,
		Parser:               parser.NewParser(parser.Options{EnableExperimentalFunctions: true}),
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

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Print testing instructions
	printInstructions()

	// Wait for shutdown signal
	fmt.Println("Press Ctrl+C to stop the demo")
	<-sigCh
	fmt.Println("\nShutting down...")

	return nil
}

// appendTestMetrics appends test metrics to the TSDB head.
func appendTestMetrics(db *tsdb.DB) error {
	app := db.Appender(context.Background())
	now := time.Now().UnixMilli()

	// target_info for api-gateway in production
	_, err := app.Append(0, labels.FromStrings(
		"__name__", "target_info",
		"job", "api-gateway",
		"instance", "prod-gateway-1:8080",
		"version", "v1.0",
		"env", "prod",
		"cluster", "us-east",
	), now, 1)
	if err != nil {
		return fmt.Errorf("append target_info: %w", err)
	}

	// target_info for api-gateway in staging
	_, err = app.Append(0, labels.FromStrings(
		"__name__", "target_info",
		"job", "api-gateway",
		"instance", "staging-gateway-1:8080",
		"version", "v2.0",
		"env", "staging",
		"cluster", "us-west",
	), now, 1)
	if err != nil {
		return fmt.Errorf("append target_info: %w", err)
	}

	// target_info for database
	_, err = app.Append(0, labels.FromStrings(
		"__name__", "target_info",
		"job", "database",
		"instance", "prod-db-1:5432",
		"version", "v2.1",
		"env", "prod",
		"region", "us-east",
	), now, 1)
	if err != nil {
		return fmt.Errorf("append target_info: %w", err)
	}

	// build_info as an alternative info metric
	_, err = app.Append(0, labels.FromStrings(
		"__name__", "build_info",
		"job", "api-gateway",
		"instance", "prod-gateway-1:8080",
		"go_version", "1.21",
		"revision", "abc123",
	), now, 1)
	if err != nil {
		return fmt.Errorf("append build_info: %w", err)
	}

	// http_requests_total for api-gateway production
	// Add multiple data points so rate() works
	for _, status := range []string{"200", "404", "500"} {
		for _, method := range []string{"GET", "POST"} {
			lbls := labels.FromStrings(
				"__name__", "http_requests_total",
				"job", "api-gateway",
				"instance", "prod-gateway-1:8080",
				"method", method,
				"status", status,
			)
			// Add 6 data points, 1 minute apart, with increasing values
			for i := range 6 {
				ts := now - int64((5-i)*60*1000) // 5 min ago to now, 1 min intervals
				val := float64(100 + i*10)       // Increasing counter value
				_, err = app.Append(0, lbls, ts, val)
				if err != nil {
					return fmt.Errorf("append http_requests_total: %w", err)
				}
			}
		}
	}

	// http_requests_total for api-gateway staging
	lbls := labels.FromStrings(
		"__name__", "http_requests_total",
		"job", "api-gateway",
		"instance", "staging-gateway-1:8080",
		"method", "GET",
		"status", "200",
	)
	for i := range 6 {
		ts := now - int64((5-i)*60*1000)
		val := float64(50 + i*5)
		_, err = app.Append(0, lbls, ts, val)
		if err != nil {
			return fmt.Errorf("append http_requests_total: %w", err)
		}
	}

	// db_queries_total for database
	for _, op := range []string{"SELECT", "INSERT", "UPDATE"} {
		lbls := labels.FromStrings(
			"__name__", "db_queries_total",
			"job", "database",
			"instance", "prod-db-1:5432",
			"operation", op,
		)
		for i := range 6 {
			ts := now - int64((5-i)*60*1000)
			val := float64(1000 + i*100)
			_, err = app.Append(0, lbls, ts, val)
			if err != nil {
				return fmt.Errorf("append db_queries_total: %w", err)
			}
		}
	}

	return app.Commit()
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
	fmt.Println("   Expected suggestions: version, env, cluster")
	fmt.Println()
	fmt.Println("2. LABEL VALUE autocomplete:")
	fmt.Println("   Type: info(http_requests_total, {version=\"")
	fmt.Println("   Expected suggestions: v1.0, v2.0")
	fmt.Println()
	fmt.Println("3. Execute a full query:")
	fmt.Println("   info(http_requests_total)")
	fmt.Println("   This returns metrics enriched with target_info labels")
	fmt.Println()
	fmt.Println("4. Test the info_labels API endpoint:")
	fmt.Println("   curl 'http://localhost:9090/api/v1/info_labels'")
	fmt.Println("   curl 'http://localhost:9090/api/v1/info_labels?expr=http_requests_total{job=\"api-gateway\"}'")
	fmt.Println("   curl 'http://localhost:9090/api/v1/info_labels?expr=rate(http_requests_total[5m])'")
	fmt.Println()
	fmt.Println("5. Test search filtering (label name substring match):")
	fmt.Println("   curl 'http://localhost:9090/api/v1/info_labels?search=env'")
	fmt.Println("   curl 'http://localhost:9090/api/v1/info_labels?search=ion'")
	fmt.Println("   Response includes labelOrder sorted by relevance:")
	fmt.Println("   exact match > prefix > earlier substring position > alphabetical")
	fmt.Println()
	fmt.Println("Available metrics:")
	fmt.Println("  - http_requests_total (api-gateway in prod & staging)")
	fmt.Println("  - db_queries_total (database in prod)")
	fmt.Println("  - target_info (all services)")
	fmt.Println("  - build_info (api-gateway)")
	fmt.Println()
	fmt.Println("Data labels from target_info:")
	fmt.Println("  - version: v1.0, v2.0, v2.1")
	fmt.Println("  - env: prod, staging")
	fmt.Println("  - cluster: us-east, us-west (api-gateway)")
	fmt.Println("  - region: us-east (database)")
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
