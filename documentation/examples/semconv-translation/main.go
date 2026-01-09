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

// This demo showcases OTel semantic conventions translation in Prometheus.
// It writes metrics using different OTLP translation strategies (naming conventions)
// and demonstrates how __semconv_url__ queries can merge series with different names.
//
// Run with: go run ./documentation/examples/semconv-translation
//
// For browser demo with Prometheus UI:
//
//	go run ./documentation/examples/semconv-translation --data-dir=./semconv-demo-data --populate-only
//	./prometheus --storage.tsdb.path=./semconv-demo-data --config.file=/dev/null --enable-feature=semconv-versioned-read
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/tsdb"
)

// ANSI color codes for terminal output.
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorCyan    = "\033[36m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorMagenta = "\033[35m"
)

var (
	dataDir      = flag.String("data-dir", "", "TSDB data directory (default: temp directory, deleted on exit)")
	populateOnly = flag.Bool("populate-only", false, "Only populate data, skip query phase (for use with demo.sh)")
)

func main() {
	flag.Parse()

	fmt.Printf("\n%s%s=== OTel Semantic Conventions Translation Demo ===%s\n\n", colorBold, colorCyan, colorReset)

	// Determine TSDB directory.
	var tsdbDir string
	var cleanup func()

	if *dataDir != "" {
		// User-specified directory - don't delete on exit.
		tsdbDir = *dataDir
		cleanup = func() {}
		// Create directory if it doesn't exist.
		if err := os.MkdirAll(tsdbDir, 0o755); err != nil {
			fmt.Printf("Failed to create data dir: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Temp directory - delete on exit.
		tmpDir, err := os.MkdirTemp("", "semconv-demo-")
		if err != nil {
			fmt.Printf("Failed to create temp dir: %v\n", err)
			os.Exit(1)
		}
		tsdbDir = tmpDir
		cleanup = func() { os.RemoveAll(tmpDir) }
	}
	defer cleanup()

	fmt.Printf("TSDB data directory: %s%s%s\n\n", colorYellow, tsdbDir, colorReset)

	// Create logger.
	logger := promslog.New(&promslog.Config{})

	// Open TSDB.
	db, err := tsdb.Open(tsdbDir, logger, nil, tsdb.DefaultOptions(), nil)
	if err != nil {
		fmt.Printf("Failed to open TSDB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// For browser demo, spread data over time so graphs look interesting.
	// Phase 1 data: 2 hours ago to 1 hour ago (simulating "old" producer).
	// Phase 2 data: 1 hour ago to now (simulating "new" producer after migration).
	now := time.Now()

	// ===== Phase 1: Write metrics with UnderscoreEscapingWithSuffixes =====
	printPhase(1, "Writing metrics with UnderscoreEscapingWithSuffixes strategy")

	fmt.Printf("This simulates an OTLP producer using traditional Prometheus naming.\n")
	fmt.Printf("The metric 'test' (unit: By, type: counter) becomes 'test_bytes_total'.\n")
	fmt.Printf("The attribute 'http.response.status_code' becomes 'http_response_status_code'.\n")
	fmt.Printf("Writing samples from 2 hours ago to 1 hour ago...\n\n")

	// Write samples with escaped naming (UnderscoreEscapingWithSuffixes style).
	// Spread across time for graphing.
	app := db.Appender(context.Background())
	startTime := now.Add(-2 * time.Hour)
	endTime := now.Add(-1 * time.Hour)
	interval := 15 * time.Second
	value := 1000.0

	for t := startTime; t.Before(endTime); t = t.Add(interval) {
		value += 10 // Counter increases over time.

		lbls := labels.FromStrings(
			"__name__", "test_bytes_total",
			"http_response_status_code", "200",
			"tenant", "alice",
			"instance", "producer-escaped",
		)
		_, err = app.Append(0, lbls, t.UnixMilli(), value)
		if err != nil {
			fmt.Printf("Failed to append: %v\n", err)
			os.Exit(1)
		}

		// Also write a 404 series with lower values.
		lbls404 := labels.FromStrings(
			"__name__", "test_bytes_total",
			"http_response_status_code", "404",
			"tenant", "bob",
			"instance", "producer-escaped",
		)
		_, err = app.Append(0, lbls404, t.UnixMilli(), value*0.1)
		if err != nil {
			fmt.Printf("Failed to append: %v\n", err)
			os.Exit(1)
		}
	}

	if err := app.Commit(); err != nil {
		fmt.Printf("Failed to commit: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  %s[Written]%s %d samples for test_bytes_total{http_response_status_code=\"200\"}\n", colorGreen, colorReset, int(time.Hour/interval))
	fmt.Printf("  %s[Written]%s %d samples for test_bytes_total{http_response_status_code=\"404\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// ===== Phase 2: Write metrics with NoTranslation =====
	printPhase(2, "Writing metrics with NoTranslation strategy")

	fmt.Printf("This simulates an OTLP producer using native OTel UTF-8 naming.\n")
	fmt.Printf("The metric 'test' stays as 'test' (no unit suffix).\n")
	fmt.Printf("The attribute 'http.response.status_code' preserves its dots.\n")
	fmt.Printf("Writing samples from 1 hour ago to now...\n\n")

	// Write samples with native OTel naming (NoTranslation style).
	app = db.Appender(context.Background())
	startTime = now.Add(-1 * time.Hour)
	endTime = now
	// Continue counter from where Phase 1 left off.

	for t := startTime; t.Before(endTime); t = t.Add(interval) {
		value += 10

		lbls := labels.FromStrings(
			"__name__", "test",
			"http.response.status_code", "200",
			"tenant", "alice",
			"instance", "producer-native",
		)
		_, err = app.Append(0, lbls, t.UnixMilli(), value)
		if err != nil {
			fmt.Printf("Failed to append: %v\n", err)
			os.Exit(1)
		}

		// Also write a 500 series.
		lbls500 := labels.FromStrings(
			"__name__", "test",
			"http.response.status_code", "500",
			"tenant", "charlie",
			"instance", "producer-native",
		)
		_, err = app.Append(0, lbls500, t.UnixMilli(), value*0.05)
		if err != nil {
			fmt.Printf("Failed to append: %v\n", err)
			os.Exit(1)
		}
	}

	if err := app.Commit(); err != nil {
		fmt.Printf("Failed to commit: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  %s[Written]%s %d samples for test{http.response.status_code=\"200\"}\n", colorGreen, colorReset, int(time.Hour/interval))
	fmt.Printf("  %s[Written]%s %d samples for test{http.response.status_code=\"500\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// If populate-only mode, exit here.
	if *populateOnly {
		fmt.Printf("%s%s--- Data population complete ---%s\n\n", colorBold, colorGreen, colorReset)
		fmt.Printf("TSDB data written to: %s%s%s\n\n", colorYellow, tsdbDir, colorReset)
		fmt.Printf("To query this data in the browser, run:\n")
		fmt.Printf("  %s./prometheus --storage.tsdb.path=%s --config.file=/dev/null%s\n\n", colorCyan, tsdbDir, colorReset)
		fmt.Printf("Then open http://localhost:9090 and try these queries:\n")
		fmt.Printf("  1. %stest_bytes_total%s                         # Shows only escaped metrics\n", colorMagenta, colorReset)
		fmt.Printf("  2. %stest%s                                     # Shows only native metrics\n", colorMagenta, colorReset)
		fmt.Printf("  3. %stest{__semconv_url__=\"registry/1.1.0\"}%s    # Shows BOTH merged!\n\n", colorMagenta, colorReset)
		return
	}

	// ===== Phase 3: Query with __semconv_url__ =====
	printPhase(3, "Querying with __semconv_url__ for unified view")

	fmt.Printf("The __semconv_url__ parameter tells Prometheus to:\n")
	fmt.Printf("  1. Load the semantic conventions from the embedded registry\n")
	fmt.Printf("  2. Generate variant matchers for different naming conventions\n")
	fmt.Printf("  3. Merge results into canonical OTel names\n\n")

	// Create semconv-aware storage wrapper.
	semconvStorage := semconv.AwareStorage(db)

	// Create PromQL engine.
	opts := promql.EngineOpts{
		Logger:     logger,
		MaxSamples: 50000,
		Timeout:    time.Minute,
	}
	engine := promql.NewEngine(opts)

	// Query using __semconv_url__ to merge both naming conventions.
	// The registry/1.1.0 file defines the "test" metric with its attributes.
	query := `test{__semconv_url__="registry/1.1.0"}`
	fmt.Printf("Query: %s%s%s\n\n", colorMagenta, query, colorReset)

	ctx := context.Background()
	q, err := engine.NewInstantQuery(ctx, semconvStorage, nil, query, now)
	if err != nil {
		fmt.Printf("Failed to create query: %v\n", err)
		os.Exit(1)
	}

	result := q.Exec(ctx)
	if result.Err != nil {
		fmt.Printf("Query failed: %v\n", result.Err)
		os.Exit(1)
	}

	fmt.Printf("%sResults:%s\n", colorBold, colorReset)
	fmt.Printf("%s\n", result.Value.String())

	// ===== Summary =====
	fmt.Printf("\n%s%s--- Summary ---%s\n\n", colorBold, colorGreen, colorReset)
	fmt.Printf("The demo showed how Prometheus can unify metrics from different OTLP producers:\n\n")
	fmt.Printf("  %s*%s Producer A used UnderscoreEscapingWithSuffixes:\n", colorCyan, colorReset)
	fmt.Printf("      test_bytes_total{http_response_status_code=\"200\"}\n\n")
	fmt.Printf("  %s*%s Producer B used NoTranslation:\n", colorCyan, colorReset)
	fmt.Printf("      test{http.response.status_code=\"200\"}\n\n")
	fmt.Printf("  %s*%s Query with __semconv_url__ merged both into canonical OTel names:\n", colorCyan, colorReset)
	fmt.Printf("      test{http.response.status_code=\"200\"}\n\n")
	fmt.Printf("This enables querying across heterogeneous OTLP producers using\n")
	fmt.Printf("a single semantic conventions-aware query.\n\n")
}

func printPhase(n int, description string) {
	fmt.Printf("%s%s--- Phase %d: %s ---%s\n\n", colorBold, colorYellow, n, description, colorReset)
}
