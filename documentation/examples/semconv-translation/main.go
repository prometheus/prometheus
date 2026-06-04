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

// This demo showcases versioned OTel semantic-conventions read in Prometheus.
// It simulates a native-OTel producer whose metric and one of its attributes
// are renamed across semantic-conventions versions: semconv 1.0.0 named the
// metric "test.counter" with attribute "user"; semconv 1.1.0 renamed them to
// "test" and "tenant".
//
// The demo shows:
//   - How a rename breaks queries (each name covers only its own era)
//   - How __schema_url__ walks the schema's version renames so a single query
//     surfaces both eras under the requested version's metric and attribute names
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

// writeEra writes the 200/404 series for one era under the given metric name
// over [start, end), advancing the shared counter value. tenantAttr is the
// era's name for the renamed attribute (user in 1.0.0, tenant in 1.1.0).
func writeEra(db *tsdb.DB, metric, tenantAttr string, start, end time.Time, interval time.Duration, value *float64) error {
	app := db.Appender(context.Background())
	for t := start; t.Before(end); t = t.Add(interval) {
		*value += 10
		for _, code := range []string{"200", "404"} {
			v := *value
			if code == "404" {
				v = *value * 0.1
			}
			lbls := labels.FromStrings(
				"__name__", metric,
				tenantAttr, "acme",
				"http.response.status_code", code,
				"instance", "myapp:8080",
			)
			if _, err := app.Append(0, lbls, t.UnixMilli(), v); err != nil {
				return fmt.Errorf("append sample: %w", err)
			}
		}
	}
	return app.Commit()
}

func run() error {
	flag.Parse()

	fmt.Printf("\n%s%s=== Versioned OTel Semantic Conventions Demo ===%s\n\n", colorBold, colorCyan, colorReset)

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

	// Simulate a native-OTel metric renamed across semconv versions:
	// - Phase 1 (2h-1h ago): semconv 1.0.0, metric named "test.counter".
	// - Phase 2 (1h ago-now): semconv 1.1.0 renamed it to "test".
	now := time.Now()
	interval := 15 * time.Second
	value := 100.0

	// ===== Phase 1: semconv 1.0.0 era — test.counter (2h-1h ago) =====
	printPhase(1, "Semconv 1.0.0 era: native metric test.counter")
	fmt.Print("The producer used semconv 1.0.0: metric 'test.counter' with attribute 'user'\n")
	fmt.Print("(native OTel names). Writing samples from 2 hours ago to 1 hour ago...\n\n")
	if err := writeEra(db, "test.counter", "user", now.Add(-2*time.Hour), now.Add(-1*time.Hour), interval, &value); err != nil {
		return err
	}
	fmt.Printf("  %s[Written]%s %d samples each for test.counter{user=\"acme\", http.response.status_code=\"200\"/\"404\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// ===== Phase 2: semconv 1.1.0 era — renamed to test (1h ago-now) =====
	printPhase(2, "Semconv 1.1.0 era: renamed to test")
	fmt.Print("Semconv 1.1.0 renamed 'test.counter' → 'test' and 'user' → 'tenant'. The same\n")
	fmt.Print("producer now writes 'test' with 'tenant'. Writing samples from 1 hour ago to now...\n\n")
	if err := writeEra(db, "test", "tenant", now.Add(-1*time.Hour), now, interval, &value); err != nil {
		return err
	}
	fmt.Printf("  %s[Written]%s %d samples each for test{tenant=\"acme\", http.response.status_code=\"200\"/\"404\"}\n\n", colorGreen, colorReset, int(time.Hour/interval))

	// If populate-only mode, exit here.
	if *populateOnly {
		fmt.Printf("%s%s--- Data population complete ---%s\n\n", colorBold, colorGreen, colorReset)
		fmt.Printf("TSDB data written to: %s%s%s\n\n", colorYellow, tsdbDir, colorReset)
		fmt.Print("To query this data in the browser, run:\n")
		fmt.Printf("  %s./prometheus --storage.tsdb.path=%s --config.file=/dev/null --enable-feature=semconv-versioned-read%s\n\n", colorCyan, tsdbDir, colorReset)
		fmt.Print("Then open http://localhost:9090 and try these queries:\n\n")
		fmt.Printf("  %sThe Problem - the rename splits the series across two names:%s\n", colorYellow, colorReset)
		fmt.Printf("    %s{\"test.counter\"}%s   # Only the semconv 1.0.0 era (2h-1h ago)\n", colorMagenta, colorReset)
		fmt.Printf("    %stest%s           # Only the semconv 1.1.0 era (1h-now)\n\n", colorMagenta, colorReset)
		fmt.Printf("  %sThe Solution - __schema_url__ walks the version renames:%s\n", colorGreen, colorReset)
		fmt.Printf("    %stest{__semconv_url__=\"registry/1.1.0\", __schema_url__=\"registry/registry.yaml\"}%s   # Both eras under \"test\"\n\n", colorMagenta, colorReset)
		fmt.Printf("  %sAttribute rename - __schema_url__ also normalises user → tenant:%s\n", colorGreen, colorReset)
		fmt.Printf("    %ssum by (tenant) (test{__semconv_url__=\"registry/1.1.0\", __schema_url__=\"registry/registry.yaml\"})%s   # 1.0.0 'user' folds into 'tenant'\n\n", colorMagenta, colorReset)
		return nil
	}

	// ===== Phase 3: Demonstrate the problem - the rename splits the series =====
	printPhase(3, "The Problem: a rename splits the series")

	semconvStorage := semconv.AwareStorage(db)
	opts := promql.EngineOpts{
		Logger:     logger,
		MaxSamples: 50000,
		Timeout:    time.Minute,
	}
	engine := promql.NewEngine(opts)
	ctx := context.Background()

	fmt.Print("After the rename, neither name alone covers the whole timeline:\n\n")
	runRangeQueryWithDetails(ctx, engine, db, now, `{"test.counter"}`,
		"Old (1.0.0) name - only data from BEFORE the rename")
	runRangeQueryWithDetails(ctx, engine, db, now, "test",
		"New (1.1.0) name - only data from AFTER the rename")
	fmt.Printf("  %s=> Neither query alone shows the complete picture!%s\n\n", colorYellow, colorReset)

	// ===== Phase 4: The schema-version solution - __schema_url__ =====
	printPhase(4, "The Solution: __schema_url__")

	fmt.Print("__semconv_url__ selects the registry version; __schema_url__ turns on\n")
	fmt.Print("schema-version fan-out, walking the schema's `versions` section to recover the\n")
	fmt.Print("metric's historical names and merging results under the requested version's name.\n\n")

	runRangeQueryWithDetails(ctx, engine, semconvStorage, now,
		`test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"}`,
		"Schema-aware query - spans the rename, unified under \"test\"")
	fmt.Printf("  %s=> Complete coverage across the rename boundary at %s%s\n\n", colorGreen, now.Add(-1*time.Hour).Format("15:04"), colorReset)

	// ===== Phase 5: attribute-rename continuity via sum by (tenant) =====
	printPhase(5, "Attribute rename: user → tenant")

	fmt.Print("Semconv 1.1.0 also renamed the attribute 'user' → 'tenant'. __schema_url__\n")
	fmt.Print("normalises historical attribute names too, so aggregating by the new name folds\n")
	fmt.Print("the 1.0.0 era (labelled 'user') in rather than dropping it.\n\n")

	runRangeQueryWithDetails(ctx, engine, semconvStorage, now,
		`sum by (tenant) (test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"})`,
		"sum by (tenant) - groups both eras under the canonical attribute name")
	fmt.Printf("  %s=> The 1.0.0 'user' series is grouped under 'tenant', spanning the rename%s\n\n", colorGreen, colorReset)

	// ===== Summary =====
	fmt.Printf("\n%s%s--- Summary ---%s\n\n", colorBold, colorGreen, colorReset)
	fmt.Print("This demo simulated a native-OTel producer (myapp:8080) whose metric was renamed\n")
	fmt.Print("across semantic-conventions versions:\n\n")
	fmt.Printf("  %s*%s semconv 1.0.0 (2h-1h ago): test.counter{user=\"acme\", http.response.status_code=\"200\", ...}\n", colorCyan, colorReset)
	fmt.Printf("  %s*%s semconv 1.1.0 (1h ago-now): test{tenant=\"acme\", http.response.status_code=\"200\", ...}\n\n", colorCyan, colorReset)
	fmt.Printf("  %s*%s Without __schema_url__: queries break at the rename, dashboards show gaps\n", colorYellow, colorReset)
	fmt.Printf("  %s*%s With __semconv_url__ + __schema_url__: one query spans the rename, unifying\n", colorGreen, colorReset)
	fmt.Print("    both the metric name (test) and the attribute name (tenant)\n\n")

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
// both eras) and returns summary stats. It prints the query and description
// lines and any error; ok is false on error or an empty result.
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

// runRangeQueryWithDetails runs query and classifies whether the result covers
// data before, after, or spanning the rename boundary.
func runRangeQueryWithDetails(ctx context.Context, engine *promql.Engine, storage storage.Queryable, now time.Time, query, description string) {
	stats, ok := execRange(ctx, engine, storage, now, query, description)
	if !ok {
		return
	}

	renamePoint := now.Add(-1 * time.Hour).UnixMilli()
	minTimeStr := time.UnixMilli(stats.minT).Format("15:04")
	maxTimeStr := time.UnixMilli(stats.maxT).Format("15:04")
	renameStr := time.UnixMilli(renamePoint).Format("15:04")

	var coverage string
	switch {
	case stats.minT < renamePoint && stats.maxT > renamePoint:
		coverage = fmt.Sprintf("%s[FULL]%s %s to %s (spans the rename at %s)", colorGreen, colorReset, minTimeStr, maxTimeStr, renameStr)
	case stats.maxT <= renamePoint:
		coverage = fmt.Sprintf("%s[PARTIAL]%s %s to %s (only before the rename, ends at or before %s)", colorYellow, colorReset, minTimeStr, maxTimeStr, renameStr)
	default:
		coverage = fmt.Sprintf("%s[PARTIAL]%s %s to %s (only after the rename, starts at or after %s)", colorYellow, colorReset, minTimeStr, maxTimeStr, renameStr)
	}

	fmt.Printf("  [Result] %d series, %d data points\n", stats.series, stats.points)
	fmt.Printf("           Time coverage: %s\n\n", coverage)
}
