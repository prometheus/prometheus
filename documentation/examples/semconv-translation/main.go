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

// This demo showcases versioned OTel semantic-conventions read in Prometheus,
// covering both legacy OTel schema 1.1.0 version renames and OTel Telemetry
// Policies (file_format: transform/2.0, open-telemetry/weaver#1850):
//   - Schema 1.1.0 renames: test.counter{user} → test{tenant}
//   - Policy transform/2.0 HTTP 1.20.0 → 1.23.1: metric & attribute renames,
//     enum value mapping ("2.0" → "2"), and conditional 5xx error.type derivation
//   - Policy transform/2.0 K8s 1.18.0 → 1.44.0: metric splits on attribute
//     predicates (k8s.resource_quota.hard_limit{resource="limits.memory"} →
//     k8s.resourcequota.memory.limit.hard) and regex split+discard extraction
//     (resource="requests.hugepages-2Mi" → k8s.hugepage.size="2Mi")
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

// writeLabelSets appends a sample for each label set over [start, end) at interval.
func writeLabelSets(db *tsdb.DB, start, end time.Time, interval time.Duration, value *float64, sets ...labels.Labels) error {
	app := db.Appender(context.Background())
	for t := start; t.Before(end); t = t.Add(interval) {
		*value += 5
		for i, lbls := range sets {
			if _, err := app.Append(0, lbls, t.UnixMilli(), *value+float64(i)); err != nil {
				return fmt.Errorf("append sample: %w", err)
			}
		}
	}
	return app.Commit()
}

func run() error {
	flag.Parse()

	fmt.Printf("\n%s%s=== Versioned OTel Semantic Conventions & Telemetry Policies Demo ===%s\n\n", colorBold, colorCyan, colorReset)

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

	// Simulate native-OTel metrics evolving across semconv versions:
	// - Era 1 (2h-1h ago): semconv 1.0.0 ("test.counter"), HTTP 1.20.0 ("http.server.duration"),
	//   K8s 1.18.0 ("k8s.resource_quota.hard_limit").
	// - Era 2 (1h ago-now): semconv 1.1.0 ("test"), HTTP 1.23.1 ("http.server.request.duration"),
	//   K8s 1.44.0 ("k8s.resourcequota.memory.limit.hard" & "k8s.resourcequota.hugepage.request.hard").
	now := time.Now()
	era1Start, era1End := now.Add(-2*time.Hour), now.Add(-1*time.Hour)
	era2Start, era2End := now.Add(-1*time.Hour), now
	interval := 15 * time.Second
	value := 100.0
	httpValue := 42.0
	k8sValue := 1024.0

	// ===== Phase 1: Historical era (2h-1h ago) =====
	printPhase(1, "Historical era (2h-1h ago): semconv 1.0.0, HTTP 1.20.0, K8s 1.18.0")
	fmt.Print("Writing historical series before the schema/policy upgrades (2h ago to 1h ago)...\n\n")
	if err := writeEra(db, "test.counter", "user", era1Start, era1End, interval, &value); err != nil {
		return err
	}
	if err := writeLabelSets(db, era1Start, era1End, interval, &httpValue,
		labels.FromStrings(
			"__name__", "http.server.duration",
			"http.method", "POST",
			"http.status_code", "503",
			"net.protocol.version", "2.0",
			"http.scheme", "https",
			"instance", "myapp:8080",
		),
	); err != nil {
		return err
	}
	if err := writeLabelSets(db, era1Start, era1End, interval, &k8sValue,
		labels.FromStrings(
			"__name__", "k8s.resource_quota.hard_limit",
			"resource", "limits.memory",
			"k8s.resource_quota.name", "compute-quota",
		),
		labels.FromStrings(
			"__name__", "k8s.resource_quota.hard_limit",
			"resource", "requests.hugepages-2Mi",
			"k8s.resource_quota.name", "compute-quota",
		),
	); err != nil {
		return err
	}
	samplesPerEra := int(time.Hour / interval)
	fmt.Printf("  %s[Written]%s %d samples each for:\n", colorGreen, colorReset, samplesPerEra)
	fmt.Print("    - test.counter{user=\"acme\", http.response.status_code=\"200\"/\"404\"}\n")
	fmt.Print("    - http.server.duration{http.method=\"POST\", http.status_code=\"503\", net.protocol.version=\"2.0\", http.scheme=\"https\"}\n")
	fmt.Print("    - k8s.resource_quota.hard_limit{resource=\"limits.memory\" / \"requests.hugepages-2Mi\", k8s.resource_quota.name=\"compute-quota\"}\n\n")

	// ===== Phase 2: Upgraded era (1h ago-now) =====
	printPhase(2, "Upgraded era (1h ago-now): semconv 1.1.0, HTTP 1.23.1, K8s 1.44.0")
	fmt.Print("Producers upgrade to newer semantic conventions (1h ago to now)...\n\n")
	if err := writeEra(db, "test", "tenant", era2Start, era2End, interval, &value); err != nil {
		return err
	}
	if err := writeLabelSets(db, era2Start, era2End, interval, &httpValue,
		labels.FromStrings(
			"__name__", "http.server.request.duration",
			"http.request.method", "POST",
			"http.response.status_code", "503",
			"network.protocol.version", "2",
			"url.scheme", "https",
			"error.type", "503",
			"instance", "myapp:8080",
		),
	); err != nil {
		return err
	}
	if err := writeLabelSets(db, era2Start, era2End, interval, &k8sValue,
		labels.FromStrings(
			"__name__", "k8s.resourcequota.memory.limit.hard",
			"k8s.resourcequota.name", "compute-quota",
		),
		labels.FromStrings(
			"__name__", "k8s.resourcequota.hugepage_count.request.hard",
			"k8s.hugepage.size", "2Mi",
			"k8s.resourcequota.name", "compute-quota",
		),
	); err != nil {
		return err
	}
	fmt.Printf("  %s[Written]%s %d samples each for:\n", colorGreen, colorReset, samplesPerEra)
	fmt.Print("    - test{tenant=\"acme\", http.response.status_code=\"200\"/\"404\"}\n")
	fmt.Print("    - http.server.request.duration{http.request.method=\"POST\", http.response.status_code=\"503\", network.protocol.version=\"2\", url.scheme=\"https\", error.type=\"503\"}\n")
	fmt.Print("    - k8s.resourcequota.memory.limit.hard & k8s.resourcequota.hugepage_count.request.hard{k8s.hugepage.size=\"2Mi\", k8s.resourcequota.name=\"compute-quota\"}\n\n")

	// If populate-only mode, exit here.
	if *populateOnly {
		fmt.Printf("%s%s--- Data population complete ---%s\n\n", colorBold, colorGreen, colorReset)
		fmt.Printf("TSDB data written to: %s%s%s\n\n", colorYellow, tsdbDir, colorReset)
		fmt.Print("To query this data in the browser, run:\n")
		fmt.Printf("  %s./prometheus --storage.tsdb.path=%s --config.file=/dev/null --enable-feature=semconv-versioned-read%s\n\n", colorCyan, tsdbDir, colorReset)
		fmt.Print("Then open http://localhost:9090 and try these queries:\n\n")
		fmt.Printf("  %s1. Legacy OTel Schema 1.1.0 renames (test.counter{user} → test{tenant}):%s\n", colorGreen, colorReset)
		fmt.Printf("    %stest{__semconv_url__=\"registry/1.1.0\", __schema_url__=\"registry/registry.yaml\"}%s\n", colorMagenta, colorReset)
		fmt.Printf("    %ssum by (tenant) (test{__semconv_url__=\"registry/1.1.0\", __schema_url__=\"registry/registry.yaml\"})%s\n\n", colorMagenta, colorReset)
		fmt.Printf("  %s2. OTel Telemetry Policy transform/2.0 — HTTP 1.20.0 → 1.23.1 (rename + value map + 5xx error.type):%s\n", colorGreen, colorReset)
		fmt.Printf("    %s{\"http.server.request.duration\", __schema_url__=\"registry/http_1.20.0_to_1.23.1.yaml\", \"network.protocol.version\"=\"2\", \"error.type\"=\"503\"}%s\n\n", colorMagenta, colorReset)
		fmt.Printf("  %s3. OTel Telemetry Policy transform/2.0 — K8s 1.18.0 → 1.44.0 (metric splits & regex split+discard):%s\n", colorGreen, colorReset)
		fmt.Printf("    %s{\"k8s.resourcequota.memory.limit.hard\", __schema_url__=\"registry/k8s_1.18.0_to_1.44.0.yaml\"}%s\n", colorMagenta, colorReset)
		fmt.Printf("    %s{\"k8s.resourcequota.hugepage_count.request.hard\", __schema_url__=\"registry/k8s_1.18.0_to_1.44.0.yaml\", \"k8s.hugepage.size\"=\"2Mi\"}%s\n\n", colorMagenta, colorReset)
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
	printPhase(4, "Schema 1.1.0 Solution: __semconv_url__ + __schema_url__")

	fmt.Print("__semconv_url__ selects the registry version; __schema_url__ turns on\n")
	fmt.Print("schema-version fan-out, walking the schema's `versions` section to recover the\n")
	fmt.Print("metric's historical names and merging results under the requested version's name.\n\n")

	runRangeQueryWithDetails(ctx, engine, semconvStorage, now,
		`test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"}`,
		"Schema-aware query - spans the rename, unified under \"test\"")
	fmt.Printf("  %s=> Complete coverage across the rename boundary at %s%s\n\n", colorGreen, now.Add(-1*time.Hour).Format("15:04"), colorReset)

	// ===== Phase 5: attribute-rename continuity via sum by (tenant) =====
	printPhase(5, "Schema 1.1.0 Attribute Rename: user → tenant")

	fmt.Print("Semconv 1.1.0 also renamed the attribute 'user' → 'tenant'. __schema_url__\n")
	fmt.Print("normalises historical attribute names too, so aggregating by the new name folds\n")
	fmt.Print("the 1.0.0 era (labelled 'user') in rather than dropping it.\n\n")

	runRangeQueryWithDetails(ctx, engine, semconvStorage, now,
		`sum by (tenant) (test{__semconv_url__="registry/1.1.0", __schema_url__="registry/registry.yaml"})`,
		"sum by (tenant) - groups both eras under the canonical attribute name")
	fmt.Printf("  %s=> The 1.0.0 'user' series is grouped under 'tenant', spanning the rename%s\n\n", colorGreen, colorReset)

	// ===== Phase 6: OTel Telemetry Policy (transform/2.0) — HTTP 1.20.0 → 1.23.1 =====
	printPhase(6, "OTel Telemetry Policy (transform/2.0): HTTP 1.20.0 → 1.23.1")

	fmt.Print("With file_format: transform/2.0 (open-telemetry/weaver#1850), policies go beyond\n")
	fmt.Print("1:1 renames. http_1.20.0_to_1.23.1.yaml renames http.server.duration →\n")
	fmt.Print("http.server.request.duration, renames http.method/http.status_code/http.scheme,\n")
	fmt.Print("maps net.protocol.version \"2.0\" → network.protocol.version \"2\", and derives\n")
	fmt.Print("error.type=\"503\" when http.response.status_code >= 500.\n\n")

	runRangeQueryWithDetails(ctx, engine, semconvStorage, now,
		`{"http.server.request.duration", __schema_url__="registry/http_1.20.0_to_1.23.1.yaml", "http.request.method"="POST", "network.protocol.version"="2", "error.type"="503"}`,
		"Policy-aware HTTP query - bridges 1.20.0 and 1.23.1 into 1 unified series")
	fmt.Printf("  %s=> 1 series spanning both eras with mapped protocol version \"2\" and derived error.type=\"503\"%s\n\n", colorGreen, colorReset)

	// ===== Phase 7: OTel Telemetry Policy (transform/2.0) — K8s 1.18.0 → 1.44.0 splits =====
	printPhase(7, "OTel Telemetry Policy (transform/2.0): K8s 1.18.0 → 1.44.0 Metric Splits")

	fmt.Print("k8s_1.18.0_to_1.44.0.yaml splits multiplexed k8s.resource_quota.hard_limit series\n")
	fmt.Print("into dedicated metrics by predicate (resource=\"limits.memory\" →\n")
	fmt.Print("k8s.resourcequota.memory.limit.hard) and extracts hugepage sizes via regex\n")
	fmt.Print("split+discard (resource=\"requests.hugepages-2Mi\" → k8s.hugepage.size=\"2Mi\").\n\n")

	runRangeQueryWithDetails(ctx, engine, semconvStorage, now,
		`{"k8s.resourcequota.memory.limit.hard", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml"}`,
		"Split metric query (predicate equals: resource=\"limits.memory\")")
	runRangeQueryWithDetails(ctx, engine, semconvStorage, now,
		`{"k8s.resourcequota.hugepage_count.request.hard", __schema_url__="registry/k8s_1.18.0_to_1.44.0.yaml", "k8s.hugepage.size"="2Mi"}`,
		"Split metric query (regex split+discard: k8s.hugepage.size=\"2Mi\")")
	fmt.Printf("  %s=> Multiplexed 1.18.0 quota series seamlessly join 1.44.0 dedicated metrics!%s\n\n", colorGreen, colorReset)

	// ===== Phase 8: Authoring-Time PromQL AST Rewriter (semconv.RewritePromQLQuery) =====
	printPhase(8, "Authoring-Time PromQL AST Rewriter (semconv.RewritePromQLQuery)")

	fmt.Print("In addition to query-time storage translation (semconv.AwareStorage), queries\n")
	fmt.Print("can be rewritten at authoring time into standard PromQL (using `or`,\n")
	fmt.Print("`label_replace`, and `label_join`) to run against an unmodified TSDB:\n\n")

	origQuery := `sum by ("http.request.method", "network.protocol.version", "error.type") ({"http.server.request.duration", __schema_url__="registry/http_1.20.0_to_1.23.1.yaml", "http.request.method"="POST", "network.protocol.version"="2", "error.type"="503"})`
	rewrittenQuery, _, err := semconv.RewritePromQLQuery(origQuery, "")
	if err != nil {
		return fmt.Errorf("rewrite query: %w", err)
	}
	runRangeQueryWithDetails(ctx, engine, db, now, rewrittenQuery,
		"Rewritten standard PromQL executed directly against raw TSDB (no AwareStorage wrapper)")
	fmt.Printf("  %s=> Authoring-time PromQL AST rewrite produces identical results across both eras!%s\n\n", colorGreen, colorReset)

	// ===== Summary =====
	fmt.Printf("\n%s%s--- Summary ---%s\n\n", colorBold, colorGreen, colorReset)
	fmt.Print("This demo showcased two generations of OTel schema translation in Prometheus:\n\n")
	fmt.Printf("  %s1. Legacy OTel Schema (file_format: 1.1.0)%s\n", colorCyan, colorReset)
	fmt.Print("     - Metric & attribute renames: test.counter{user} → test{tenant}\n\n")
	fmt.Printf("  %s2. OTel Telemetry Policies (file_format: transform/2.0, weaver#1850)%s\n", colorCyan, colorReset)
	fmt.Print("     - HTTP 1.20.0 → 1.23.1: metric/attribute renames, enum value mapping (\"2.0\" → \"2\"),\n")
	fmt.Print("       and conditional attribute derivation (status >= 500 → error.type)\n")
	fmt.Print("     - K8s 1.18.0 → 1.44.0: predicate-based metric splits and regex split+discard\n")
	fmt.Print("       attribute extraction without requiring __semconv_url__\n\n")

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
