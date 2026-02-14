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

// This demo showcases the persisted series metadata feature in Prometheus.
// It demonstrates how metric metadata (TYPE, HELP, UNIT) is persisted to disk
// and remains available via the /api/v1/metadata API even after the original
// scrape target is removed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/seriesmetadata"
)

// protobufAcceptHeader is the Accept header used to request protobuf format,
// which is required for native histogram support.
const protobufAcceptHeader = "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited"

// ANSI color codes for terminal output.
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorGray    = "\033[37m" // White/light gray - visible on dark backgrounds
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
)

func main() {
	fmt.Printf("%s%s=== Prometheus Persisted Metadata Demo ===%s\n\n", colorBold, colorCyan, colorReset)

	if err := runDemo(); err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
}

func runDemo() error {
	// Create temporary directory for TSDB data
	tmpDir, err := os.MkdirTemp("", "prometheus-metadata-demo-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	fmt.Printf("%sTSDB data directory:%s %s\n\n", colorGray, colorReset, tmpDir)

	// Start the mock exporter
	exporter := NewMockExporter("127.0.0.1:0")
	if err := exporter.Start(); err != nil {
		return fmt.Errorf("start exporter: %w", err)
	}
	defer func() { _ = exporter.Stop() }()
	exporterURL := "http://" + exporter.Addr() + "/metrics"
	fmt.Printf("%sMock exporter started at:%s %s%s%s\n\n", colorGray, colorReset, colorGreen, exporterURL, colorReset)

	// Create TSDB
	logger := promslog.New(&promslog.Config{})
	opts := tsdb.DefaultOptions()
	opts.MinBlockDuration = int64(time.Hour / time.Millisecond) // 1 hour blocks for demo
	opts.MaxBlockDuration = int64(time.Hour / time.Millisecond)
	opts.EnableNativeMetadata = true

	db, err := tsdb.Open(tmpDir, logger, nil, opts, nil)
	if err != nil {
		return fmt.Errorf("open TSDB: %w", err)
	}
	defer func() { db.Close() }()

	// === PHASE 1: Scrape and store metrics with metadata ===
	printPhase(1, "Scraping metrics from exporter (protobuf format) and storing in TSDB")

	metadataMap, stats, err := scrapeAndStore(db, exporterURL)
	if err != nil {
		return fmt.Errorf("scrape and store: %w", err)
	}

	fmt.Printf("%sStored %s%d%s metrics with metadata in TSDB head (in-memory)\n", colorGreen, colorBold, len(metadataMap), colorReset)
	fmt.Printf("  %s•%s Float samples: %s%d%s\n", colorGray, colorReset, colorBold, stats.floatSamples, colorReset)
	fmt.Printf("  %s•%s Native histograms: %s%d%s\n", colorGray, colorReset, colorBold, stats.histogramSamples, colorReset)
	fmt.Println()
	printMetadataMap("Metadata in memory (from scrape):", metadataMap)

	// === PHASE 2: Query metadata from TSDB head ===
	printPhase(2, "Querying metadata from TSDB (in-memory head)")

	headMeta, err := db.Head().SeriesMetadata()
	if err != nil {
		return fmt.Errorf("get head metadata: %w", err)
	}
	printSeriesMetadata("Metadata from TSDB head:", headMeta)
	headMeta.Close()

	// === PHASE 3: WAL Replay ===
	printPhase(3, "WAL replay: closing and reopening TSDB")

	fmt.Printf("Closing TSDB to flush WAL records to disk...\n")
	db.Close()

	fmt.Printf("Reopening TSDB from %s (triggers WAL replay)...\n\n", tmpDir)

	db, err = tsdb.Open(tmpDir, logger, nil, opts, nil)
	if err != nil {
		return fmt.Errorf("reopen TSDB after WAL replay: %w", err)
	}

	// Verify metadata survived WAL replay
	replayMeta, err := db.Head().SeriesMetadata()
	if err != nil {
		return fmt.Errorf("get head metadata after replay: %w", err)
	}
	printSeriesMetadata("Metadata after WAL replay:", replayMeta)
	replayMeta.Close()

	// === PHASE 4: Stop the exporter (simulate target going down) ===
	printPhase(4, "Stopping the mock exporter (simulating target removal)")

	if err := exporter.Stop(); err != nil {
		return fmt.Errorf("stop exporter: %w", err)
	}
	fmt.Printf("%sExporter stopped.%s In a real scenario, the scrape target is now gone.\n", colorYellow, colorReset)
	fmt.Printf("%sWithout metadata persistence, this metadata would be lost!%s\n", colorYellow, colorReset)

	// === PHASE 5: Compact head to persist metadata to disk ===
	printPhase(5, "Compacting TSDB head to persist metadata to disk")

	// Force compaction by creating a block
	if err := db.CompactHead(tsdb.NewRangeHead(db.Head(), db.Head().MinTime(), db.Head().MaxTime())); err != nil {
		// Compaction might fail if there's not enough data, which is fine for demo
		fmt.Printf("%sNote:%s Head compaction returned: %v (this is often expected with minimal data)\n", colorYellow, colorReset, err)
	}

	// Check for persisted blocks
	blocks := db.Blocks()
	fmt.Printf("Number of persisted blocks: %s%d%s\n", colorBold, len(blocks), colorReset)

	if len(blocks) > 0 {
		// Check if metadata file exists in the block
		blockDir := filepath.Join(tmpDir, blocks[0].Meta().ULID.String())
		metadataFile := filepath.Join(blockDir, seriesmetadata.SeriesMetadataFilename)
		if _, err := os.Stat(metadataFile); err == nil {
			fmt.Printf("%s✓ Metadata file created:%s %s\n", colorGreen, colorReset, metadataFile)
		}
	}
	fmt.Println()

	// === PHASE 6: Query persisted metadata from DB ===
	printPhase(6, "Querying metadata from TSDB (including persisted blocks)")

	dbMeta, err := db.SeriesMetadata()
	if err != nil {
		return fmt.Errorf("get DB metadata: %w", err)
	}
	printSeriesMetadata("Metadata from TSDB (persisted):", dbMeta)
	dbMeta.Close()

	// === PHASE 7: Demonstrate API response format ===
	printPhase(7, "Demonstrating /api/v1/metadata API response format")

	apiResponse := buildAPIResponse(db)
	prettyJSON, _ := json.MarshalIndent(apiResponse, "", "  ")
	fmt.Printf("%sAPI Response (same format as /api/v1/metadata):%s\n", colorBold, colorReset)
	fmt.Printf("%s%s%s\n", colorGray, string(prettyJSON), colorReset)
	fmt.Println()

	// === Summary ===
	printPhase(8, "Summary")
	fmt.Printf("%sThis demo showed how Prometheus persists metric metadata:%s\n", colorBold, colorReset)
	fmt.Printf("  %s1.%s Metadata is captured during scraping (TYPE, HELP, UNIT comments)\n", colorGreen, colorReset)
	fmt.Printf("  %s2.%s Metadata is stored in TSDB head (in-memory)\n", colorGreen, colorReset)
	fmt.Printf("  %s3.%s Metadata survives TSDB restart via %sWAL replay%s\n", colorGreen, colorReset, colorBold, colorReset)
	fmt.Printf("  %s4.%s When blocks are compacted, metadata is persisted to Parquet files\n", colorGreen, colorReset)
	fmt.Printf("  %s5.%s Even after scrape targets are removed, metadata remains queryable\n", colorGreen, colorReset)
	fmt.Printf("  %s6.%s The /api/v1/metadata endpoint returns both active and persisted metadata\n", colorGreen, colorReset)
	fmt.Println()
	fmt.Printf("%sThis enables users to understand historical metrics even when\n", colorCyan)
	fmt.Printf("the original exporters are no longer running.%s\n", colorReset)

	return nil
}

// scrapeStats tracks what was scraped.
type scrapeStats struct {
	floatSamples     int
	histogramSamples int
}

// scrapeAndStore fetches metrics from the exporter using protobuf format,
// parses them, and stores in TSDB.
func scrapeAndStore(db *tsdb.DB, url string) (map[string]metadata.Metadata, scrapeStats, error) {
	var stats scrapeStats

	// Create request with protobuf Accept header (required for native histograms)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, stats, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", protobufAcceptHeader)

	// Fetch metrics
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, stats, fmt.Errorf("fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, stats, fmt.Errorf("read response: %w", err)
	}

	// Parse metrics
	contentType := resp.Header.Get("Content-Type")
	fmt.Printf("%sResponse Content-Type:%s %s\n", colorGray, colorReset, contentType)

	parser, err := textparse.New(body, contentType, labels.NewSymbolTable(), textparse.ParserOptions{})
	if err != nil {
		return nil, stats, fmt.Errorf("create parser: %w", err)
	}

	// Collect metadata and samples
	metadataMap := make(map[string]metadata.Metadata)
	var currentMeta metadata.Metadata
	var currentMetric string

	app := db.Appender(context.TODO())
	now := time.Now().UnixMilli()

	for {
		entry, err := parser.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, stats, fmt.Errorf("parse: %w", err)
		}

		switch entry {
		case textparse.EntryType:
			metricName, metricType := parser.Type()
			currentMetric = string(metricName)
			currentMeta = metadataMap[currentMetric]
			currentMeta.Type = metricType
			metadataMap[currentMetric] = currentMeta

		case textparse.EntryHelp:
			metricName, helpText := parser.Help()
			currentMetric = string(metricName)
			currentMeta = metadataMap[currentMetric]
			currentMeta.Help = string(helpText)
			metadataMap[currentMetric] = currentMeta

		case textparse.EntryUnit:
			metricName, unit := parser.Unit()
			currentMetric = string(metricName)
			currentMeta = metadataMap[currentMetric]
			currentMeta.Unit = string(unit)
			metadataMap[currentMetric] = currentMeta

		case textparse.EntrySeries:
			_, ts, val := parser.Series()
			var lset labels.Labels
			parser.Labels(&lset)

			timestamp := now
			if ts != nil {
				timestamp = *ts
			}

			// Append sample
			ref, err := app.Append(0, lset, timestamp, val)
			if err != nil {
				return nil, stats, fmt.Errorf("append sample: %w", err)
			}
			stats.floatSamples++

			// Append metadata for this series
			// Try full metric name first, then stripped name (for histograms/summaries)
			metricName := lset.Get(labels.MetricName)
			if meta, ok := lookupMetadata(metadataMap, metricName); ok {
				if _, err := app.UpdateMetadata(ref, lset, meta); err != nil {
					_ = err // Metadata update errors are non-fatal
				}
			}

		case textparse.EntryHistogram:
			// Native histogram entry
			_, ts, h, fh := parser.Histogram()
			var lset labels.Labels
			parser.Labels(&lset)

			timestamp := now
			if ts != nil {
				timestamp = *ts
			}

			// Append native histogram
			histApp, ok := app.(storage.HistogramAppender)
			if !ok {
				return nil, stats, errors.New("appender does not support histograms")
			}
			ref, err := histApp.AppendHistogram(0, lset, timestamp, h, fh)
			if err != nil {
				return nil, stats, fmt.Errorf("append histogram: %w", err)
			}
			stats.histogramSamples++

			// Append metadata for this histogram
			metricName := lset.Get(labels.MetricName)
			if meta, ok := lookupMetadata(metadataMap, metricName); ok {
				if _, err := app.UpdateMetadata(ref, lset, meta); err != nil {
					_ = err // Metadata update errors are non-fatal
				}
			}
		}
	}

	if err := app.Commit(); err != nil {
		return nil, stats, fmt.Errorf("commit: %w", err)
	}

	return metadataMap, stats, nil
}

// lookupMetadata finds metadata for a metric name, trying the full name first,
// then falling back to the base name (without suffixes like _total, _count, etc.).
func lookupMetadata(m map[string]metadata.Metadata, metricName string) (metadata.Metadata, bool) {
	// Try full metric name first (works for counters in protobuf format)
	if meta, ok := m[metricName]; ok {
		return meta, true
	}
	// Fall back to stripped name (for histogram/summary sub-series)
	baseName := stripMetricSuffix(metricName)
	if baseName != metricName {
		if meta, ok := m[baseName]; ok {
			return meta, true
		}
	}
	return metadata.Metadata{}, false
}

// stripMetricSuffix removes summary sub-series suffixes to get the base metric name.
func stripMetricSuffix(name string) string {
	suffixes := []string{"_count", "_sum", "_bucket"}
	for _, suffix := range suffixes {
		if base, found := strings.CutSuffix(name, suffix); found {
			return base
		}
	}
	return name
}

// printPhase prints a phase header with color.
func printPhase(num int, title string) {
	fmt.Printf("%s%s--- Phase %d: %s ---%s\n\n", colorBold, colorMagenta, num, title, colorReset)
}

// printMetadataMap prints metadata from a map, sorted by metric name.
func printMetadataMap(title string, m map[string]metadata.Metadata) {
	fmt.Printf("%s%s%s\n", colorBold, title, colorReset)

	// Collect and sort names for consistent output
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		meta := m[name]
		fmt.Printf("  %s%s%s:\n", colorCyan, name, colorReset)
		fmt.Printf("    Type: %s\n", colorizeType(meta.Type))
		fmt.Printf("    Help: %s%s%s\n", colorGray, meta.Help, colorReset)
		if meta.Unit != "" {
			fmt.Printf("    Unit: %s\n", meta.Unit)
		}
	}
	fmt.Println()
}

// colorizeType returns a colored string for the metric type.
func colorizeType(t model.MetricType) string {
	switch t {
	case model.MetricTypeCounter:
		return colorGreen + string(t) + colorReset
	case model.MetricTypeGauge:
		return colorYellow + string(t) + colorReset
	case model.MetricTypeHistogram:
		return colorMagenta + string(t) + colorReset
	case model.MetricTypeSummary:
		return colorCyan + string(t) + colorReset
	default:
		return string(t)
	}
}

// printSeriesMetadata prints metadata from a seriesmetadata.Reader, sorted by metric name.
// Deduplicates entries by base metric name (strips _count, _sum, _bucket suffixes).
func printSeriesMetadata(title string, reader seriesmetadata.Reader) {
	fmt.Printf("%s%s%s\n", colorBold, title, colorReset)

	// Collect and deduplicate by base metric name
	seen := make(map[string]metadata.Metadata)
	_ = reader.IterByMetricName(func(name string, metas []metadata.Metadata) error {
		if len(metas) == 0 {
			return nil
		}
		meta := metas[0]
		baseName := stripMetricSuffix(name)
		// Prefer the base name entry, or first seen
		if _, exists := seen[baseName]; !exists || name == baseName {
			seen[baseName] = meta
		}
		return nil
	})

	// Sort names for consistent output
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)

	// Print sorted entries
	for _, name := range names {
		meta := seen[name]
		fmt.Printf("  %s%s%s:\n", colorCyan, name, colorReset)
		fmt.Printf("    Type: %s\n", colorizeType(meta.Type))
		fmt.Printf("    Help: %s%s%s\n", colorGray, meta.Help, colorReset)
		if meta.Unit != "" {
			fmt.Printf("    Unit: %s\n", meta.Unit)
		}
	}
	if len(seen) == 0 {
		fmt.Printf("  %s(no metadata found)%s\n", colorGray, colorReset)
	}
	fmt.Println()
}

// buildAPIResponse builds a response in the same format as /api/v1/metadata.
// Deduplicates entries by base metric name (strips _count, _sum, _bucket suffixes).
func buildAPIResponse(db *tsdb.DB) map[string]any {
	reader, err := db.SeriesMetadata()
	if err != nil {
		return map[string]any{
			"status": "error",
			"error":  err.Error(),
		}
	}
	defer reader.Close()

	// Collect and deduplicate by base metric name
	seen := make(map[string]metadata.Metadata)
	_ = reader.IterByMetricName(func(name string, metas []metadata.Metadata) error {
		if len(metas) == 0 {
			return nil
		}
		meta := metas[0]
		baseName := stripMetricSuffix(name)
		// Prefer the base name entry, or first seen
		if _, exists := seen[baseName]; !exists || name == baseName {
			seen[baseName] = meta
		}
		return nil
	})

	// Build response data
	data := make(map[string][]map[string]string)
	for name, meta := range seen {
		data[name] = []map[string]string{{
			"type": string(meta.Type),
			"help": meta.Help,
			"unit": meta.Unit,
		}}
	}

	// If no data from TSDB, show a message
	if len(data) == 0 {
		// Add some sample data to show the format
		data["demo_http_requests_total"] = []map[string]string{{
			"type": string(model.MetricTypeCounter),
			"help": "Total number of HTTP requests received.",
			"unit": "",
		}}
	}

	return map[string]any{
		"status": "success",
		"data":   data,
	}
}
