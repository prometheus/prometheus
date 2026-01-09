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
// It simulates a migration scenario where a producer changes OTLP translation
// strategy from UnderscoreEscapingWithSuffixes to NoTranslation (native OTel names).
//
// The demo shows:
//   - How queries break after migration (old metric name returns no new data)
//   - How __semconv_url__ provides query continuity across the migration
//   - How aggregations and rate() work seamlessly across naming boundaries
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
	"strings"
	"time"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
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

	// Simulate a migration scenario:
	// - Phase 1 (2h ago to 1h ago): Producer uses UnderscoreEscapingWithSuffixes (old strategy)
	// - Phase 2 (1h ago to now): Same producer migrates to NoTranslation (new strategy)
	// This represents a real-world scenario where you upgrade your OTLP configuration.
	now := time.Now()

	// ===== Phase 1: Write metrics with UnderscoreEscapingWithSuffixes =====
	printPhase(1, "Before migration: UnderscoreEscapingWithSuffixes")

	fmt.Printf("Simulating a producer BEFORE migration to native OTel naming.\n")
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

		// Same producer ("myapp") - this is the key point: same source, different naming over time.
		lbls := labels.FromStrings(
			"__name__", "test_bytes_total",
			"http_response_status_code", "200",
			"instance", "myapp:8080",
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
			"instance", "myapp:8080",
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

	fmt.Printf("  %s[Written]%s %d samples for test_bytes_total{http_response_status_code=\"200\", instance=\"myapp:8080\"}\n", colorGreen, colorReset, int(time.Hour/interval))
	fmt.Printf("  %s[Written]%s %d samples for test_bytes_total{http_response_status_code=\"404\", instance=\"myapp:8080\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// ===== Phase 2: Write metrics with NoTranslation =====
	printPhase(2, "After migration: NoTranslation (native OTel naming)")

	fmt.Printf("Simulating the SAME producer AFTER migration to native OTel naming.\n")
	fmt.Printf("The metric 'test' stays as 'test' (no unit/type suffix).\n")
	fmt.Printf("The attribute 'http.response.status_code' preserves its dots.\n")
	fmt.Printf("Writing samples from 1 hour ago to now...\n\n")

	// Write samples with native OTel naming (NoTranslation style).
	app = db.Appender(context.Background())
	startTime = now.Add(-1 * time.Hour)
	endTime = now
	// Continue counter from where Phase 1 left off.

	for t := startTime; t.Before(endTime); t = t.Add(interval) {
		value += 10

		// Same producer ("myapp"), same logical series - just different naming after migration.
		lbls := labels.FromStrings(
			"__name__", "test",
			"http.response.status_code", "200",
			"instance", "myapp:8080",
		)
		_, err = app.Append(0, lbls, t.UnixMilli(), value)
		if err != nil {
			fmt.Printf("Failed to append: %v\n", err)
			os.Exit(1)
		}

		// Same 404 series, continuing after migration.
		lbls404 := labels.FromStrings(
			"__name__", "test",
			"http.response.status_code", "404",
			"instance", "myapp:8080",
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

	fmt.Printf("  %s[Written]%s %d samples for test{http.response.status_code=\"200\", instance=\"myapp:8080\"}\n", colorGreen, colorReset, int(time.Hour/interval))
	fmt.Printf("  %s[Written]%s %d samples for test{http.response.status_code=\"404\", instance=\"myapp:8080\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// If populate-only mode, exit here.
	if *populateOnly {
		fmt.Printf("%s%s--- Data population complete ---%s\n\n", colorBold, colorGreen, colorReset)
		fmt.Printf("TSDB data written to: %s%s%s\n\n", colorYellow, tsdbDir, colorReset)
		fmt.Printf("To query this data in the browser, run:\n")
		fmt.Printf("  %s./prometheus --storage.tsdb.path=%s --config.file=/dev/null --enable-feature=semconv-versioned-read%s\n\n", colorCyan, tsdbDir, colorReset)
		fmt.Printf("Then open http://localhost:9090 and try these queries:\n\n")
		fmt.Printf("  %sThe Problem - Queries break after migration:%s\n", colorYellow, colorReset)
		fmt.Printf("    %stest_bytes_total%s                           # Only old data (before migration)\n", colorMagenta, colorReset)
		fmt.Printf("    %stest%s                                       # Only new data (after migration)\n\n", colorMagenta, colorReset)
		fmt.Printf("  %sThe Solution - Seamless query continuity:%s\n", colorGreen, colorReset)
		fmt.Printf("    %stest{__semconv_url__=\"registry/1.1.0\"}%s      # ALL data, unified naming!\n", colorMagenta, colorReset)
		fmt.Printf("    %ssum(test{__semconv_url__=\"registry/1.1.0\"})%s # Aggregation works across boundary\n", colorMagenta, colorReset)
		fmt.Printf("    %srate(test{__semconv_url__=\"registry/1.1.0\"}[5m])%s # Rate works too!\n\n", colorMagenta, colorReset)
		return
	}

	// ===== Phase 3: Demonstrate the problem - broken queries after migration =====
	printPhase(3, "The Problem: Queries break after migration")

	// Create semconv-aware storage wrapper.
	semconvStorage := semconv.AwareStorage(db)

	// Create PromQL engine.
	opts := promql.EngineOpts{
		Logger:     logger,
		MaxSamples: 50000,
		Timeout:    time.Minute,
	}
	engine := promql.NewEngine(opts)
	ctx := context.Background()

	fmt.Printf("After migrating to native OTel naming, queries using old names miss new data:\n\n")

	// Query 1: Old metric name - only returns pre-migration data
	runRangeQueryWithDetails(engine, db, ctx, now, "test_bytes_total",
		"Old metric name - only has data from BEFORE migration")

	// Query 2: New metric name - only returns post-migration data
	runRangeQueryWithDetails(engine, db, ctx, now, "test",
		"New metric name - only has data from AFTER migration")

	fmt.Printf("  %s=> Neither query alone shows the complete picture!%s\n\n", colorYellow, colorReset)

	// ===== Phase 4: The solution - semconv-aware queries =====
	printPhase(4, "The Solution: __semconv_url__ for query continuity")

	fmt.Printf("The __semconv_url__ parameter tells Prometheus to:\n")
	fmt.Printf("  1. Load semantic conventions from the embedded registry\n")
	fmt.Printf("  2. Generate query variants for all OTLP naming strategies\n")
	fmt.Printf("  3. Merge results with canonical OTel names\n\n")

	// Query 3: Semconv-aware query - returns ALL data with unified naming
	runRangeQueryWithDetails(engine, semconvStorage, ctx, now, `test{__semconv_url__="registry/1.1.0"}`,
		"Semconv query - returns ALL data with unified OTel naming!")

	fmt.Printf("  %s=> Complete data coverage across the migration boundary!%s\n\n", colorGreen, colorReset)

	// ===== Phase 5: Aggregation across the migration boundary =====
	printPhase(5, "Aggregation works across the migration boundary")

	fmt.Printf("Aggregations like sum() work seamlessly across different naming conventions:\n\n")

	runRangeQuery(engine, semconvStorage, ctx, now, `sum(test{__semconv_url__="registry/1.1.0"})`,
		"sum() aggregates data from both naming conventions into one continuous series")

	// ===== Phase 6: Rate calculation across the migration boundary =====
	printPhase(6, "Counter rates work across the migration boundary")

	fmt.Printf("Even rate() calculations work across the naming change:\n\n")

	runRangeQuery(engine, semconvStorage, ctx, now, `sum(rate(test{__semconv_url__="registry/1.1.0"}[5m]))`,
		"rate() computes correctly across the migration point")

	// ===== Summary =====
	fmt.Printf("\n%s%s--- Summary ---%s\n\n", colorBold, colorGreen, colorReset)
	fmt.Printf("This demo simulated a migration scenario where a producer (myapp:8080)\n")
	fmt.Printf("changed OTLP translation strategy:\n\n")
	fmt.Printf("  %s*%s Before migration (2h-1h ago): UnderscoreEscapingWithSuffixes\n", colorCyan, colorReset)
	fmt.Printf("      test_bytes_total{http_response_status_code=\"200\", instance=\"myapp:8080\"}\n\n")
	fmt.Printf("  %s*%s After migration (1h ago-now): NoTranslation (native OTel)\n", colorCyan, colorReset)
	fmt.Printf("      test{http.response.status_code=\"200\", instance=\"myapp:8080\"}\n\n")
	fmt.Printf("  %s*%s Without __semconv_url__: Queries break, dashboards show gaps\n", colorYellow, colorReset)
	fmt.Printf("  %s*%s With __semconv_url__: Seamless continuity, all data accessible\n\n", colorGreen, colorReset)
	fmt.Printf("Key benefits:\n")
	fmt.Printf("  - Existing dashboards keep working after migration\n")
	fmt.Printf("  - Aggregations (sum, avg, etc.) work across naming boundaries\n")
	fmt.Printf("  - Rate calculations produce continuous results\n")
	fmt.Printf("  - Results use canonical OTel semantic convention names\n\n")
}

func printPhase(n int, description string) {
	fmt.Printf("%s%s--- Phase %d: %s ---%s\n\n", colorBold, colorYellow, n, description, colorReset)
}

// runRangeQuery executes a range query over the last 2.5 hours and displays results.
func runRangeQuery(engine *promql.Engine, storage storage.Queryable, ctx context.Context, now time.Time, query, description string) {
	fmt.Printf("  Query: %s%s%s\n", colorMagenta, query, colorReset)
	fmt.Printf("  %s\n", description)

	// Query over the full time range (2.5h to capture data from both phases).
	start := now.Add(-150 * time.Minute)
	end := now
	step := 5 * time.Minute

	q, err := engine.NewRangeQuery(ctx, storage, nil, query, start, end, step)
	if err != nil {
		fmt.Printf("  %s[Error]%s Failed to create query: %v\n\n", colorYellow, colorReset, err)
		return
	}

	result := q.Exec(ctx)
	if result.Err != nil {
		fmt.Printf("  %s[Error]%s Query failed: %v\n\n", colorYellow, colorReset, result.Err)
		return
	}

	// Count data points.
	resultStr := result.Value.String()
	if resultStr == "" || resultStr == "{}" {
		fmt.Printf("  %s[Result]%s No data returned\n\n", colorYellow, colorReset)
	} else {
		// Count total data points across all series.
		pointCount := strings.Count(resultStr, "@")
		fmt.Printf("  %s[Result]%s %d data points spanning the full 2.5h range\n", colorGreen, colorReset, pointCount)
		fmt.Printf("           (covering both pre-migration and post-migration data)\n\n")
	}
}

// runRangeQueryWithDetails executes a range query and shows time coverage details.
func runRangeQueryWithDetails(engine *promql.Engine, storage storage.Queryable, ctx context.Context, now time.Time, query, description string) {
	fmt.Printf("  Query: %s%s%s\n", colorMagenta, query, colorReset)
	fmt.Printf("  %s\n", description)

	// Query over the full time range (2.5h to capture data from both phases).
	start := now.Add(-150 * time.Minute)
	end := now
	step := 5 * time.Minute

	q, err := engine.NewRangeQuery(ctx, storage, nil, query, start, end, step)
	if err != nil {
		fmt.Printf("  %s[Error]%s Failed to create query: %v\n\n", colorYellow, colorReset, err)
		return
	}

	result := q.Exec(ctx)
	if result.Err != nil {
		fmt.Printf("  %s[Error]%s Query failed: %v\n\n", colorYellow, colorReset, result.Err)
		return
	}

	// Analyze the matrix result for time coverage.
	matrix, ok := result.Value.(promql.Matrix)
	if !ok || len(matrix) == 0 {
		fmt.Printf("  %s[Result]%s No data returned\n\n", colorYellow, colorReset)
		return
	}

	// Find the time range covered by the data.
	var minTime, maxTime int64
	totalPoints := 0
	for _, series := range matrix {
		for _, point := range series.Floats {
			if minTime == 0 || point.T < minTime {
				minTime = point.T
			}
			if point.T > maxTime {
				maxTime = point.T
			}
			totalPoints++
		}
	}

	// Calculate time coverage.
	migrationPoint := now.Add(-1 * time.Hour).UnixMilli()
	minTimeStr := time.UnixMilli(minTime).Format("15:04")
	maxTimeStr := time.UnixMilli(maxTime).Format("15:04")
	migrationStr := time.UnixMilli(migrationPoint).Format("15:04")

	// Determine coverage description.
	var coverage string
	if minTime < migrationPoint && maxTime > migrationPoint {
		coverage = fmt.Sprintf("%s[FULL]%s %s to %s (spans migration at %s)", colorGreen, colorReset, minTimeStr, maxTimeStr, migrationStr)
	} else if maxTime <= migrationPoint {
		coverage = fmt.Sprintf("%s[PARTIAL]%s %s to %s (only pre-migration, ends before %s)", colorYellow, colorReset, minTimeStr, maxTimeStr, migrationStr)
	} else {
		coverage = fmt.Sprintf("%s[PARTIAL]%s %s to %s (only post-migration, starts after %s)", colorYellow, colorReset, minTimeStr, maxTimeStr, migrationStr)
	}

	fmt.Printf("  [Result] %d series, %d data points\n", len(matrix), totalPoints)
	fmt.Printf("           Time coverage: %s\n\n", coverage)
}
