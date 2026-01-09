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
// Run with: go run ./documentation/examples/semconv_translation
package main

import (
	"context"
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

func main() {
	fmt.Printf("\n%s%s=== OTel Semantic Conventions Translation Demo ===%s\n\n", colorBold, colorCyan, colorReset)

	// Create a temporary directory for the TSDB.
	tmpDir, err := os.MkdirTemp("", "semconv-demo-")
	if err != nil {
		fmt.Printf("Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("TSDB data directory: %s%s%s\n\n", colorYellow, tmpDir, colorReset)

	// Create logger.
	logger := promslog.New(&promslog.Config{})

	// Open TSDB.
	db, err := tsdb.Open(tmpDir, logger, nil, tsdb.DefaultOptions(), nil)
	if err != nil {
		fmt.Printf("Failed to open TSDB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Base timestamp for our samples.
	baseTime := time.Now().UnixMilli()

	// ===== Phase 1: Write metrics with UnderscoreEscapingWithSuffixes =====
	printPhase(1, "Writing metrics with UnderscoreEscapingWithSuffixes strategy")

	fmt.Printf("This simulates an OTLP producer using traditional Prometheus naming.\n")
	fmt.Printf("The metric 'test' (unit: By, type: counter) becomes 'test_bytes_total'.\n")
	fmt.Printf("The attribute 'http.response.status_code' becomes 'http_response_status_code'.\n\n")

	// With UnderscoreEscapingWithSuffixes, the semantic convention metric "test"
	// (counter, unit: By) gets translated to "test_bytes_total" with underscored labels.
	app := db.Appender(context.Background())

	// Write samples with escaped naming (UnderscoreEscapingWithSuffixes style).
	lbls1 := labels.FromStrings(
		"__name__", "test_bytes_total",
		"http_response_status_code", "200",
		"tenant", "alice",
		"instance", "producer-escaped",
	)
	_, err = app.Append(0, lbls1, baseTime, 1500)
	if err != nil {
		fmt.Printf("Failed to append: %v\n", err)
		os.Exit(1)
	}
	printWritten(lbls1, 1500)

	lbls2 := labels.FromStrings(
		"__name__", "test_bytes_total",
		"http_response_status_code", "404",
		"tenant", "bob",
		"instance", "producer-escaped",
	)
	_, err = app.Append(0, lbls2, baseTime, 250)
	if err != nil {
		fmt.Printf("Failed to append: %v\n", err)
		os.Exit(1)
	}
	printWritten(lbls2, 250)

	if err := app.Commit(); err != nil {
		fmt.Printf("Failed to commit: %v\n", err)
		os.Exit(1)
	}

	// ===== Phase 2: Write metrics with NoTranslation =====
	printPhase(2, "Writing metrics with NoTranslation strategy")

	fmt.Printf("This simulates an OTLP producer using native OTel UTF-8 naming.\n")
	fmt.Printf("The metric 'test' stays as 'test' (no unit suffix).\n")
	fmt.Printf("The attribute 'http.response.status_code' preserves its dots.\n\n")

	app = db.Appender(context.Background())

	// Write samples with native OTel naming (NoTranslation style).
	lbls3 := labels.FromStrings(
		"__name__", "test",
		"http.response.status_code", "200",
		"tenant", "alice",
		"instance", "producer-native",
	)
	_, err = app.Append(0, lbls3, baseTime, 2300)
	if err != nil {
		fmt.Printf("Failed to append: %v\n", err)
		os.Exit(1)
	}
	printWritten(lbls3, 2300)

	lbls4 := labels.FromStrings(
		"__name__", "test",
		"http.response.status_code", "500",
		"tenant", "charlie",
		"instance", "producer-native",
	)
	_, err = app.Append(0, lbls4, baseTime, 75)
	if err != nil {
		fmt.Printf("Failed to append: %v\n", err)
		os.Exit(1)
	}
	printWritten(lbls4, 75)

	if err := app.Commit(); err != nil {
		fmt.Printf("Failed to commit: %v\n", err)
		os.Exit(1)
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
	q, err := engine.NewInstantQuery(ctx, semconvStorage, nil, query, time.UnixMilli(baseTime))
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

func printWritten(lbls labels.Labels, value float64) {
	fmt.Printf("  %s[Written]%s %s %.0f\n", colorGreen, colorReset, lbls.String(), value)
}
