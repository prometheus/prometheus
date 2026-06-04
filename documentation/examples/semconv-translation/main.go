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
// It simulates a producer that evolves over three hours along two independent
// axes: its semantic-conventions version (1.0.0 → 1.1.0, which renames
// test.counter → test) and its OTLP translation strategy
// (UnderscoreEscapingWithSuffixes → NoTranslation, i.e. native OTel names).
//
// The demo shows:
//   - How queries break across these migrations (each raw metric name covers only its own era)
//   - How __semconv_url__ + __otlp_strategy__ restore continuity across an OTLP-strategy migration
//   - How adding __schema_url__ additionally recovers earlier semconv versions
//   - How aggregations and rate() work seamlessly across the naming boundaries
//
// Run with: go run ./documentation/examples/semconv-translation
//
// For browser demo with Prometheus UI:
//
//	go run ./documentation/examples/semconv-translation --data-dir=/tmp/demo-data --populate-only
//	./prometheus --storage.tsdb.path=/tmp/demo-data --config.file=/dev/null --enable-feature=semconv-versioned-read
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
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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
			return fmt.Errorf("create data dir: %w", err)
		}
	} else {
		// Temp directory - delete on exit.
		tmpDir, err := os.MkdirTemp("", "semconv-demo-")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
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
		return fmt.Errorf("open TSDB: %w", err)
	}
	defer db.Close()

	// Simulate a two-step migration over three hours:
	// - Phase 1 (3h ago to 2h ago): Producer used semconv 1.0.0 with
	//   UnderscoreEscapingWithSuffixes. Metric was `test.counter` then,
	//   stored as `test_counter_bytes_total`.
	// - Phase 2 (2h ago to 1h ago): Producer upgraded to semconv 1.1.0
	//   which renamed `test.counter` to `test`, still escaping via
	//   UnderscoreEscapingWithSuffixes. Stored as `test_bytes_total`.
	// - Phase 3 (1h ago to now): Same semconv 1.1.0, but the producer
	//   switched OTLP strategy to NoTranslation. Stored as `test`.
	// This represents a producer that evolved both its semconv version
	// (schema migration) and its OTLP translation strategy over time.
	now := time.Now()
	interval := 15 * time.Second
	value := 100.0

	// ===== Phase 1: semconv 1.0.0 era — pre-historical (3h-2h ago) =====
	printPhase(1, "Semconv 1.0.0 era: test.counter / UnderscoreEscapingWithSuffixes")

	fmt.Printf("Earliest era: the producer used semconv 1.0.0 where the metric was 'test.counter'.\n")
	fmt.Printf("With UnderscoreEscapingWithSuffixes, 'test.counter' (counter, unit By) became\n")
	fmt.Printf("'test_counter_bytes_total'. Writing samples from 3 hours ago to 2 hours ago...\n\n")

	app := db.Appender(context.Background())
	startTime := now.Add(-3 * time.Hour)
	endTime := now.Add(-2 * time.Hour)

	for t := startTime; t.Before(endTime); t = t.Add(interval) {
		value += 10
		lbls := labels.FromStrings(
			"__name__", "test_counter_bytes_total",
			"http_response_status_code", "200",
			"instance", "myapp:8080",
		)
		_, err = app.Append(0, lbls, t.UnixMilli(), value)
		if err != nil {
			return fmt.Errorf("append sample: %w", err)
		}

		lbls404 := labels.FromStrings(
			"__name__", "test_counter_bytes_total",
			"http_response_status_code", "404",
			"instance", "myapp:8080",
		)
		_, err = app.Append(0, lbls404, t.UnixMilli(), value*0.1)
		if err != nil {
			return fmt.Errorf("append sample: %w", err)
		}
	}

	if err := app.Commit(); err != nil {
		return fmt.Errorf("commit samples: %w", err)
	}

	fmt.Printf("  %s[Written]%s %d samples for test_counter_bytes_total{http_response_status_code=\"200\", instance=\"myapp:8080\"}\n", colorGreen, colorReset, int(time.Hour/interval))
	fmt.Printf("  %s[Written]%s %d samples for test_counter_bytes_total{http_response_status_code=\"404\", instance=\"myapp:8080\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// ===== Phase 2: semconv 1.1.0 era, still escaped (2h-1h ago) =====
	printPhase(2, "Semconv 1.1.0 era: test / UnderscoreEscapingWithSuffixes")

	fmt.Printf("The producer upgraded semconv to 1.1.0, which renamed 'test.counter' → 'test'.\n")
	fmt.Printf("OTLP strategy was still UnderscoreEscapingWithSuffixes, so the metric became 'test_bytes_total'.\n")
	fmt.Printf("The attribute 'http.response.status_code' was still escaped to 'http_response_status_code'.\n")
	fmt.Printf("Writing samples from 2 hours ago to 1 hour ago...\n\n")

	// Write samples with escaped naming (UnderscoreEscapingWithSuffixes style).
	// Spread across time for graphing.
	app = db.Appender(context.Background())
	startTime = now.Add(-2 * time.Hour)
	endTime = now.Add(-1 * time.Hour)

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
			return fmt.Errorf("append sample: %w", err)
		}

		// Also write a 404 series with lower values.
		lbls404 := labels.FromStrings(
			"__name__", "test_bytes_total",
			"http_response_status_code", "404",
			"instance", "myapp:8080",
		)
		_, err = app.Append(0, lbls404, t.UnixMilli(), value*0.1)
		if err != nil {
			return fmt.Errorf("append sample: %w", err)
		}
	}

	if err := app.Commit(); err != nil {
		return fmt.Errorf("commit samples: %w", err)
	}

	fmt.Printf("  %s[Written]%s %d samples for test_bytes_total{http_response_status_code=\"200\", instance=\"myapp:8080\"}\n", colorGreen, colorReset, int(time.Hour/interval))
	fmt.Printf("  %s[Written]%s %d samples for test_bytes_total{http_response_status_code=\"404\", instance=\"myapp:8080\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// ===== Phase 3: semconv 1.1.0 era, native OTel naming (1h ago to now) =====
	printPhase(3, "Semconv 1.1.0 era: test / NoTranslation (native OTel naming)")

	fmt.Printf("Simulating the SAME producer AFTER migration to native OTel naming.\n")
	fmt.Printf("The metric 'test' stays as 'test' (no unit/type suffix).\n")
	fmt.Printf("The attribute 'http.response.status_code' preserves its dots.\n")
	fmt.Printf("Writing samples from 1 hour ago to now...\n\n")

	// Write samples with native OTel naming (NoTranslation style).
	app = db.Appender(context.Background())
	startTime = now.Add(-1 * time.Hour)
	endTime = now
	// Continue counter from where Phase 2 left off.

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
			return fmt.Errorf("append sample: %w", err)
		}

		// Same 404 series, continuing after migration.
		lbls404 := labels.FromStrings(
			"__name__", "test",
			"http.response.status_code", "404",
			"instance", "myapp:8080",
		)
		_, err = app.Append(0, lbls404, t.UnixMilli(), value*0.1)
		if err != nil {
			return fmt.Errorf("append sample: %w", err)
		}
	}

	if err := app.Commit(); err != nil {
		return fmt.Errorf("commit samples: %w", err)
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
		fmt.Printf("  %sThe Problem - Queries break across migrations:%s\n", colorYellow, colorReset)
		fmt.Printf("    %stest_counter_bytes_total%s                    # Only the semconv 1.0.0 era (3h-2h ago)\n", colorMagenta, colorReset)
		fmt.Printf("    %stest_bytes_total%s                            # Only the escaped 1.1.0 era (2h-1h ago)\n", colorMagenta, colorReset)
		fmt.Printf("    %stest%s                                        # Only the native 1.1.0 era (1h-now)\n\n", colorMagenta, colorReset)
		fmt.Printf("  %sOTLP-strategy fan-out (__otlp_strategy__ selects the dialect):%s\n", colorGreen, colorReset)
		fmt.Printf("    %stest{__semconv_url__=\"registry/1.1.0\", __otlp_strategy__=\"NoTranslation\"}%s                  # Both 1.1.0 eras, native names\n", colorMagenta, colorReset)
		fmt.Printf("    %stest_bytes_total{__semconv_url__=\"registry/1.1.0\", __otlp_strategy__=\"UnderscoreEscapingWithSuffixes\"}%s # Same data, escaped names\n", colorMagenta, colorReset)
		fmt.Printf("    %ssum(test{__semconv_url__=\"registry/1.1.0\", __otlp_strategy__=\"NoTranslation\"})%s             # Aggregation works\n", colorMagenta, colorReset)
		fmt.Printf("    %srate(test{__semconv_url__=\"registry/1.1.0\", __otlp_strategy__=\"NoTranslation\"}[5m])%s        # Rate works too\n\n", colorMagenta, colorReset)
		fmt.Printf("  %sOTLP + schema-version fan-out (add __schema_url__):%s\n", colorGreen, colorReset)
		fmt.Printf("    %stest{__semconv_url__=\"registry/1.1.0\", __schema_url__=\"registry/registry.yaml\", __otlp_strategy__=\"NoTranslation\"}%s # ALL three eras!\n\n", colorMagenta, colorReset)
		return nil
	}

	// ===== Phase 4: Demonstrate the problem - broken queries after migration =====
	printPhase(4, "The Problem: Queries break after migration")

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
	runRangeQueryWithDetails(ctx, engine, db, now, "test_bytes_total",
		"Old metric name - only has data from BEFORE migration")

	// Query 2: New metric name - only returns post-migration data
	runRangeQueryWithDetails(ctx, engine, db, now, "test",
		"New metric name - only has data from AFTER migration")

	fmt.Printf("  %s=> Neither query alone shows the complete picture!%s\n\n", colorYellow, colorReset)

	// ===== Phase 5: The OTLP-strategy solution - __otlp_strategy__ =====
	printPhase(5, "The OTLP-strategy Solution: __otlp_strategy__")

	fmt.Printf("__semconv_url__ names the registry; __otlp_strategy__ turns on OTLP-strategy\n")
	fmt.Printf("fan-out and picks the dialect for results. Together they tell Prometheus to:\n")
	fmt.Printf("  1. Load semantic conventions from the embedded registry\n")
	fmt.Printf("  2. Generate query variants for all OTLP naming strategies\n")
	fmt.Printf("  3. Merge results, rendered in the requested dialect (here: native OTel names)\n\n")

	// Query 3: Semconv-aware query - returns data for the OTLP-strategy migration.
	runRangeQueryWithDetails(ctx, engine, semconvStorage, now, `test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"}`,
		"Semconv query - covers the OTLP-strategy migration with unified OTel naming!")

	fmt.Printf("  %s=> Complete coverage across the strategy migration boundary at %s%s\n", colorGreen, now.Add(-1*time.Hour).Format("15:04"), colorReset)
	fmt.Printf("  %s=> For escaped-name dashboards, query test_bytes_total{..., __otlp_strategy__=%s\n", colorCyan, colorReset)
	fmt.Printf("  %s   \"UnderscoreEscapingWithSuffixes\"} to get the identical data under escaped names.%s\n", colorCyan, colorReset)
	fmt.Printf("  %s=> But the semconv 1.0.0 era (3h-2h ago) is still missing — see Phase 8.%s\n\n", colorYellow, colorReset)

	// ===== Phase 6: Aggregation across the migration boundary =====
	printPhase(6, "Aggregation works across the migration boundary")

	fmt.Printf("Aggregations like sum() work seamlessly across different naming conventions:\n\n")

	runRangeQuery(ctx, engine, semconvStorage, now, `sum(test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"})`,
		"sum() aggregates data from both naming conventions into one continuous series")

	// ===== Phase 7: Rate calculation across the migration boundary =====
	printPhase(7, "Counter rates work across the migration boundary")

	fmt.Printf("Even rate() calculations work across the naming change:\n\n")

	runRangeQuery(ctx, engine, semconvStorage, now, `sum(rate(test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"}[5m]))`,
		"rate() computes correctly across the migration point")

	// ===== Phase 8: The schema-version solution - __schema_url__ =====
	printPhase(8, "The Schema-version Solution: __schema_url__")

	fmt.Printf("__otlp_strategy__ covers the OTLP-strategy axis but not schema-version renames.\n")
	fmt.Printf("Adding __schema_url__ tells Prometheus to also walk the OTel schema's `versions`\n")
	fmt.Printf("section, recovering historical metric names (e.g. test.counter from semconv 1.0.0)\n")
	fmt.Printf("and applying the OTLP-strategy fan-out to each.\n\n")

	fmt.Printf("Compare the time ranges of the two queries below — the second covers an extra\n")
	fmt.Printf("hour at the start because schema-walking surfaces the semconv 1.0.0 era:\n\n")

	runRangeQueryShowRange(ctx, engine, semconvStorage, now,
		`test{__semconv_url__="registry/1.1.0", __otlp_strategy__="NoTranslation"}`,
		"OTLP-strategy fan-out only")

	runRangeQueryShowRange(ctx, engine, semconvStorage, now,
		`test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml", __otlp_strategy__="NoTranslation"}`,
		"OTLP-strategy + schema-version fan-out")

	fmt.Printf("  %s=> All three eras (3h of data) accessible under the canonical OTel name!%s\n\n", colorGreen, colorReset)

	// ===== Summary =====
	fmt.Printf("\n%s%s--- Summary ---%s\n\n", colorBold, colorGreen, colorReset)
	fmt.Printf("This demo simulated a producer (myapp:8080) that evolved over three hours along\n")
	fmt.Printf("two independent axes: semconv version and OTLP translation strategy.\n\n")
	fmt.Printf("  %s*%s Earliest (3h-2h ago): semconv 1.0.0 + UnderscoreEscapingWithSuffixes\n", colorCyan, colorReset)
	fmt.Printf("      test_counter_bytes_total{http_response_status_code=\"200\", instance=\"myapp:8080\"}\n\n")
	fmt.Printf("  %s*%s Middle   (2h-1h ago): semconv 1.1.0 + UnderscoreEscapingWithSuffixes\n", colorCyan, colorReset)
	fmt.Printf("      test_bytes_total{http_response_status_code=\"200\", instance=\"myapp:8080\"}\n\n")
	fmt.Printf("  %s*%s Latest   (1h ago-now): semconv 1.1.0 + NoTranslation (native OTel)\n", colorCyan, colorReset)
	fmt.Printf("      test{http.response.status_code=\"200\", instance=\"myapp:8080\"}\n\n")
	fmt.Printf("  %s*%s Without the matchers: queries break, dashboards show gaps\n", colorYellow, colorReset)
	fmt.Printf("  %s*%s With __semconv_url__ + __otlp_strategy__: covers OTLP-strategy migrations\n", colorGreen, colorReset)
	fmt.Printf("  %s*%s Add __schema_url__ as well: covers both axes, full history\n\n", colorGreen, colorReset)
	fmt.Printf("Key benefits:\n")
	fmt.Printf("  - Existing dashboards keep working after migration\n")
	fmt.Printf("  - Aggregations (sum, avg, etc.) work across naming boundaries\n")
	fmt.Printf("  - Rate calculations produce continuous results\n")
	fmt.Printf("  - Results use canonical OTel semantic convention names\n\n")

	return nil
}

func printPhase(n int, description string) {
	fmt.Printf("%s%s--- Phase %d: %s ---%s\n\n", colorBold, colorYellow, n, description, colorReset)
}

// queryStats summarizes a range-query result for the demo's display helpers.
type queryStats struct {
	series     int
	points     int
	minT, maxT int64
}

// execRange runs query over the demo's full ~3.5h window (wide enough to span
// all three eras) and returns summary stats. It prints the query and
// description lines and any error; ok is false on error or an empty result.
func execRange(ctx context.Context, engine *promql.Engine, storage storage.Queryable, now time.Time, query, description string) (queryStats, bool) {
	fmt.Printf("  Query: %s%s%s\n", colorMagenta, query, colorReset)
	fmt.Printf("  %s\n", description)

	start := now.Add(-210 * time.Minute)
	end := now
	step := 5 * time.Minute

	q, err := engine.NewRangeQuery(ctx, storage, nil, query, start, end, step)
	if err != nil {
		fmt.Printf("  %s[Error]%s Failed to create query: %v\n\n", colorYellow, colorReset, err)
		return queryStats{}, false
	}
	result := q.Exec(ctx)
	if result.Err != nil {
		fmt.Printf("  %s[Error]%s Query failed: %v\n\n", colorYellow, colorReset, result.Err)
		return queryStats{}, false
	}

	matrix, ok := result.Value.(promql.Matrix)
	if !ok || len(matrix) == 0 {
		fmt.Printf("  %s[Result]%s No data returned\n\n", colorYellow, colorReset)
		return queryStats{}, false
	}

	stats := queryStats{series: len(matrix)}
	for _, series := range matrix {
		for _, point := range series.Floats {
			if stats.minT == 0 || point.T < stats.minT {
				stats.minT = point.T
			}
			if point.T > stats.maxT {
				stats.maxT = point.T
			}
			stats.points++
		}
	}
	return stats, true
}

// runRangeQuery runs query and reports the point count and the time span the
// result actually covers.
func runRangeQuery(ctx context.Context, engine *promql.Engine, storage storage.Queryable, now time.Time, query, description string) {
	stats, ok := execRange(ctx, engine, storage, now, query, description)
	if !ok {
		return
	}
	fmt.Printf("  %s[Result]%s %d series, %d data points across %s to %s\n\n",
		colorGreen, colorReset, stats.series, stats.points,
		time.UnixMilli(stats.minT).Format("15:04"), time.UnixMilli(stats.maxT).Format("15:04"))
}

// runRangeQueryWithDetails runs query and classifies whether the result covers
// data before, after, or spanning the OTLP-strategy migration point.
func runRangeQueryWithDetails(ctx context.Context, engine *promql.Engine, storage storage.Queryable, now time.Time, query, description string) {
	stats, ok := execRange(ctx, engine, storage, now, query, description)
	if !ok {
		return
	}

	migrationPoint := now.Add(-1 * time.Hour).UnixMilli()
	minTimeStr := time.UnixMilli(stats.minT).Format("15:04")
	maxTimeStr := time.UnixMilli(stats.maxT).Format("15:04")
	migrationStr := time.UnixMilli(migrationPoint).Format("15:04")

	var coverage string
	switch {
	case stats.minT < migrationPoint && stats.maxT > migrationPoint:
		coverage = fmt.Sprintf("%s[FULL]%s %s to %s (spans migration at %s)", colorGreen, colorReset, minTimeStr, maxTimeStr, migrationStr)
	case stats.maxT <= migrationPoint:
		coverage = fmt.Sprintf("%s[PARTIAL]%s %s to %s (only pre-migration, ends at or before %s)", colorYellow, colorReset, minTimeStr, maxTimeStr, migrationStr)
	default:
		coverage = fmt.Sprintf("%s[PARTIAL]%s %s to %s (only post-migration, starts at or after %s)", colorYellow, colorReset, minTimeStr, maxTimeStr, migrationStr)
	}

	fmt.Printf("  [Result] %d series, %d data points\n", stats.series, stats.points)
	fmt.Printf("           Time coverage: %s\n\n", coverage)
}

// runRangeQueryShowRange runs query and prints the earliest and latest sample
// timestamps in the result. Used in the cross-version phase to contrast two
// queries side-by-side: the one without __schema_url__ misses the oldest era,
// so its earliest timestamp is later than the one with __schema_url__.
func runRangeQueryShowRange(ctx context.Context, engine *promql.Engine, storage storage.Queryable, now time.Time, query, label string) {
	stats, ok := execRange(ctx, engine, storage, now, query, label)
	if !ok {
		return
	}
	fmt.Printf("  %s[Result]%s %d series, %d data points\n", colorGreen, colorReset, stats.series, stats.points)
	fmt.Printf("           Time range: %s%s%s to %s%s%s\n\n",
		colorCyan, time.UnixMilli(stats.minT).Format("15:04"), colorReset,
		colorCyan, time.UnixMilli(stats.maxT).Format("15:04"), colorReset)
}
