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

// This demo showcases OTel resource attributes persistence in Prometheus TSDB.
// It demonstrates how resource attributes from OTLP metrics are persisted to disk
// and remain available for querying even after the data source is removed.
//
// The demo:
// 1. Creates a TSDB with OTLP write handler
// 2. Sends OTLP metrics with resource attributes (service.name, service.namespace, etc.)
// 3. Queries resource attributes from TSDB head (in-memory)
// 4. Forces compaction to persist data to Parquet blocks
// 5. Queries resource attributes from compacted blocks
// 6. Demonstrates the API response format for /api/v1/resources
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/xpdata/entity"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/seriesmetadata"
)

// ANSI color codes for terminal output.
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorGray    = "\033[37m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
)

const (
	serverAddr = "127.0.0.1:19999"
	otlpPath   = "/api/v1/otlp/v1/metrics"
)

func main() {
	fmt.Printf("%s%s=== Prometheus OTel Resource Attributes Persistence Demo ===%s\n\n", colorBold, colorCyan, colorReset)

	if err := runDemo(); err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
}

func runDemo() error {
	// Create temporary directory for TSDB data
	tmpDir, err := os.MkdirTemp("", "prometheus-otlp-demo-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	fmt.Printf("%sTSDB data directory:%s %s\n\n", colorGray, colorReset, tmpDir)

	// Create TSDB
	logger := promslog.New(&promslog.Config{})
	opts := tsdb.DefaultOptions()
	opts.MinBlockDuration = int64(time.Hour / time.Millisecond) // 1 hour blocks for demo
	opts.MaxBlockDuration = int64(time.Hour / time.Millisecond)

	db, err := tsdb.Open(tmpDir, logger, nil, opts, nil)
	if err != nil {
		return fmt.Errorf("open TSDB: %w", err)
	}
	defer db.Close()

	// Create and start OTLP HTTP server
	reg := prometheus.NewRegistry()
	configFunc := func() config.Config { return config.Config{} }
	otlpOpts := remote.OTLPOptions{
		LookbackDelta:             5 * time.Minute,
		AppendMetadata:            true,
		PersistResourceAttributes: true, // Enable resource attributes persistence
	}
	otlpHandler := remote.NewOTLPWriteHandler(logger, reg, db, configFunc, otlpOpts)

	mux := http.NewServeMux()
	mux.Handle(otlpPath, otlpHandler)

	server := &http.Server{Addr: serverAddr, Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("%sHTTP server error: %v%s\n", colorRed, err, colorReset)
		}
	}()
	defer server.Shutdown(context.Background())
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("%sOTLP receiver started at:%s http://%s%s%s\n\n", colorGray, colorReset, colorGreen, serverAddr+otlpPath, colorReset)

	// Capture timestamp before sending original metrics (for later info() comparison)
	originalTimestamp := time.Now()

	// === PHASE 1: Send OTLP metrics with resource attributes ===
	printPhase(1, "Sending OTLP metrics with resource attributes")

	metrics := createOTLPMetrics()
	if err := sendOTLPMetrics(metrics); err != nil {
		return fmt.Errorf("send OTLP metrics: %w", err)
	}
	fmt.Printf("%sSent %s%d%s resource metrics via OTLP\n\n", colorGreen, colorBold, metrics.ResourceMetrics().Len(), colorReset)
	printSentResources(metrics)

	// === PHASE 2: Query resource attributes from TSDB head ===
	printPhase(2, "Querying resource attributes from TSDB head (in-memory)")

	headMeta, err := db.Head().SeriesMetadata()
	if err != nil {
		return fmt.Errorf("get head metadata: %w", err)
	}
	printResourceAttributes("Resource attributes in head:", headMeta, db)
	headMeta.Close()

	// === PHASE 3: Force compaction to persist to disk ===
	printPhase(3, "Compacting TSDB head to persist resource attributes to disk")

	if err := db.CompactHead(tsdb.NewRangeHead(db.Head(), db.Head().MinTime(), db.Head().MaxTime())); err != nil {
		fmt.Printf("%sNote:%s Head compaction returned: %v (this is often expected with minimal data)\n", colorYellow, colorReset, err)
	}

	blocks := db.Blocks()
	fmt.Printf("Number of persisted blocks: %s%d%s\n", colorBold, len(blocks), colorReset)

	if len(blocks) > 0 {
		blockDir := filepath.Join(tmpDir, blocks[0].Meta().ULID.String())
		metadataFile := filepath.Join(blockDir, seriesmetadata.SeriesMetadataFilename)
		if info, err := os.Stat(metadataFile); err == nil {
			fmt.Printf("%s[OK] Parquet metadata file created:%s %s (%d bytes)\n", colorGreen, colorReset, metadataFile, info.Size())
		}
	}
	fmt.Println()

	// === PHASE 4: Query resource attributes from persisted blocks ===
	printPhase(4, "Querying resource attributes from persisted blocks")

	dbMeta, err := db.SeriesMetadata()
	if err != nil {
		return fmt.Errorf("get DB metadata: %w", err)
	}
	printResourceAttributes("Resource attributes from blocks:", dbMeta, db)
	dbMeta.Close()

	// === PHASE 5: Demonstrate descriptive attributes changing over time ===
	printPhase(5, "Descriptive attributes changing over time")

	fmt.Printf("%sScenario:%s payment-service is migrated to a new host in a different region.\n", colorBold, colorReset)
	fmt.Printf("The %sidentifying%s attributes (service.name, service.namespace, service.instance.id) stay the same,\n", colorCyan, colorReset)
	fmt.Printf("but the %sdescriptive%s attributes (host.name, cloud.region) change.\n\n", colorYellow, colorReset)

	// Wait so timestamps visibly differ
	time.Sleep(1 * time.Second)

	// Send metrics from same service instance but with changed descriptive attributes
	migratedMetrics := createMigratedServiceMetrics()
	if err := sendOTLPMetrics(migratedMetrics); err != nil {
		return fmt.Errorf("send migrated service metrics: %w", err)
	}
	fmt.Printf("%sSent metrics from payment-service after migration%s\n", colorGreen, colorReset)

	// Capture timestamp after sending migrated metrics (for later info() comparison)
	migratedTimestamp := time.Now()
	printSentResources(migratedMetrics)

	// Show the version history (blocks + head combined)
	fmt.Printf("%sVersion history for production/payment-service:%s\n", colorBold, colorReset)
	fmt.Printf("%s(Original version in block, new version in head)%s\n\n", colorGray, colorReset)
	migratedMeta, err := db.SeriesMetadata() // Combined view shows both versions
	if err != nil {
		return fmt.Errorf("get migrated metadata: %w", err)
	}
	printResourceAttributesFiltered("production/payment-service attributes:", migratedMeta, "payment-service", "production", db)
	migratedMeta.Close()

	// === PHASE 6: Demonstrate info() function with time-varying attributes ===
	printPhase(6, "Querying with info() to include resource attributes")

	fmt.Printf("The %sinfo()%s function enriches metrics with resource attributes at query time.\n", colorBold, colorReset)
	fmt.Printf("When descriptive attributes change over time, info() returns the values\n")
	fmt.Printf("that were active at the requested timestamp.\n\n")

	// Enable experimental functions (required for info())
	parser.EnableExperimentalFunctions = true

	// Create PromQL engine
	ctx := context.Background()
	engineOpts := promql.EngineOpts{
		Logger:               logger,
		Reg:                  prometheus.NewRegistry(),
		MaxSamples:           50000,
		Timeout:              time.Minute,
		EnableNegativeOffset: true,
	}
	engine := promql.NewEngine(engineOpts)

	const queryStr = `sum by (method, status, "cloud.region", "host.name") (info(http_requests_total{method="GET",status="200"}))`
	fmt.Printf("%sQuery:%s %s\n\n", colorBold, colorReset, queryStr)

	// Query at original timestamp (before migration)
	fmt.Printf("%sAt timestamp BEFORE migration (%s):%s\n", colorBold, originalTimestamp.Format(time.RFC3339), colorReset)
	query1, err := engine.NewInstantQuery(ctx, db, nil, queryStr, originalTimestamp)
	if err != nil {
		return fmt.Errorf("create query for original timestamp: %w", err)
	}
	result1 := query1.Exec(ctx)
	if result1.Err != nil {
		fmt.Printf("  %sQuery error: %v%s\n", colorRed, result1.Err, colorReset)
	} else {
		printPromQLResult(result1)
	}
	query1.Close()

	// Query at migrated timestamp (after migration)
	fmt.Printf("%sAt timestamp AFTER migration (%s):%s\n", colorBold, migratedTimestamp.Format(time.RFC3339), colorReset)
	query2, err := engine.NewInstantQuery(ctx, db, nil, queryStr, migratedTimestamp)
	if err != nil {
		return fmt.Errorf("create query for migrated timestamp: %w", err)
	}
	result2 := query2.Exec(ctx)
	if result2.Err != nil {
		fmt.Printf("  %sQuery error: %v%s\n", colorRed, result2.Err, colorReset)
	} else {
		printPromQLResult(result2)
	}
	query2.Close()

	fmt.Printf("%sThis enables time-accurate correlation of metrics with OTel traces/logs,\n", colorCyan)
	fmt.Printf("even when infrastructure changes occur during the query time range.%s\n\n", colorReset)

	// === PHASE 7: Show API response format ===
	printPhase(7, "API response format for /api/v1/resources")

	apiResponse := buildResourceAttributesAPIResponse(db)
	prettyJSON, _ := json.MarshalIndent(apiResponse, "", "  ")
	fmt.Printf("%sAPI Response (/api/v1/resources):%s\n", colorBold, colorReset)
	fmt.Printf("%s%s%s\n\n", colorGray, string(prettyJSON), colorReset)

	// === Summary ===
	printPhase(8, "Summary")
	fmt.Printf("%sThis demo showed how Prometheus persists OTel resource attributes:%s\n", colorBold, colorReset)
	fmt.Printf("  %s1.%s Resource attributes arrive via OTLP metrics (service.name, etc.)\n", colorGreen, colorReset)
	fmt.Printf("  %s2.%s Attributes are stored per-series in TSDB head (in-memory)\n", colorGreen, colorReset)
	fmt.Printf("  %s3.%s When blocks compact, attributes are persisted to Parquet files\n", colorGreen, colorReset)
	fmt.Printf("  %s4.%s %sIdentifying%s attributes (service.name, etc.) remain constant for a series\n", colorGreen, colorReset, colorCyan, colorReset)
	fmt.Printf("  %s5.%s %sDescriptive%s attributes (host.name, cloud.region) can change over time\n", colorGreen, colorReset, colorYellow, colorReset)
	fmt.Printf("  %s6.%s %sVersioned storage%s preserves attribute history with time ranges\n", colorGreen, colorReset, colorMagenta, colorReset)
	fmt.Printf("  %s7.%s Each version tracks when specific attributes were active (MinTime/MaxTime)\n", colorGreen, colorReset)
	fmt.Printf("  %s8.%s The %sinfo()%s function enriches queries with time-appropriate attributes\n", colorGreen, colorReset, colorBold, colorReset)
	fmt.Println()
	fmt.Printf("%sThis enables correlation of Prometheus metrics with OTel traces/logs\n", colorCyan)
	fmt.Printf("using the identifying resource attributes (service.name, etc.).\n")
	fmt.Printf("The version history allows tracking infrastructure changes over time.%s\n", colorReset)

	return nil
}

// createOTLPMetrics creates OTLP metrics with diverse resource attributes.
func createOTLPMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	now := time.Now()

	// Resource 1: payment-service in production (with entity_refs)
	rm1 := md.ResourceMetrics().AppendEmpty()
	res1 := rm1.Resource()
	res1.Attributes().PutStr("service.name", "payment-service")
	res1.Attributes().PutStr("service.namespace", "production")
	res1.Attributes().PutStr("service.instance.id", "payment-001")
	res1.Attributes().PutStr("host.name", "prod-payment-1.example.com")
	res1.Attributes().PutStr("cloud.region", "us-west-2")
	res1.Attributes().PutStr("deployment.environment", "production")

	// Add entity_refs: service entity + host entity
	entityRefs1 := entity.ResourceEntityRefs(res1)
	// Service entity
	serviceRef := entityRefs1.AppendEmpty()
	serviceRef.SetType("service")
	serviceRef.IdKeys().Append("service.name")
	serviceRef.IdKeys().Append("service.namespace")
	serviceRef.IdKeys().Append("service.instance.id")
	serviceRef.DescriptionKeys().Append("deployment.environment")
	// Host entity
	hostRef := entityRefs1.AppendEmpty()
	hostRef.SetType("host")
	hostRef.IdKeys().Append("host.name")
	hostRef.DescriptionKeys().Append("cloud.region")

	sm1 := rm1.ScopeMetrics().AppendEmpty()
	m1 := sm1.Metrics().AppendEmpty()
	m1.SetName("http_requests_total")
	m1.SetDescription("Total HTTP requests")
	sum1 := m1.SetEmptySum()
	sum1.SetIsMonotonic(true)
	sum1.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp1 := sum1.DataPoints().AppendEmpty()
	dp1.SetTimestamp(pcommon.NewTimestampFromTime(now))
	dp1.SetDoubleValue(1500)
	dp1.Attributes().PutStr("method", "GET")
	dp1.Attributes().PutStr("status", "200")

	// Resource 2: order-service in production
	rm2 := md.ResourceMetrics().AppendEmpty()
	res2 := rm2.Resource()
	res2.Attributes().PutStr("service.name", "order-service")
	res2.Attributes().PutStr("service.namespace", "production")
	res2.Attributes().PutStr("service.instance.id", "order-001")
	res2.Attributes().PutStr("host.name", "prod-order-1.example.com")
	res2.Attributes().PutStr("cloud.region", "us-west-2")

	sm2 := rm2.ScopeMetrics().AppendEmpty()
	m2 := sm2.Metrics().AppendEmpty()
	m2.SetName("orders_processed_total")
	m2.SetDescription("Total orders processed")
	sum2 := m2.SetEmptySum()
	sum2.SetIsMonotonic(true)
	sum2.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp2 := sum2.DataPoints().AppendEmpty()
	dp2.SetTimestamp(pcommon.NewTimestampFromTime(now))
	dp2.SetDoubleValue(500)

	// Resource 3: payment-service in staging (different namespace)
	rm3 := md.ResourceMetrics().AppendEmpty()
	res3 := rm3.Resource()
	res3.Attributes().PutStr("service.name", "payment-service")
	res3.Attributes().PutStr("service.namespace", "staging")
	res3.Attributes().PutStr("service.instance.id", "payment-staging-001")
	res3.Attributes().PutStr("host.name", "staging-payment-1.example.com")
	res3.Attributes().PutStr("cloud.region", "us-east-1")

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
	dp3.Attributes().PutStr("method", "POST")
	dp3.Attributes().PutStr("status", "201")

	return md
}

// createMigratedServiceMetrics creates metrics from payment-service after migration.
// The identifying attributes stay the same, but descriptive attributes change.
func createMigratedServiceMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()

	// Same identifying attributes as before:
	//   service.name: payment-service
	//   service.namespace: production
	//   service.instance.id: payment-001
	// But different descriptive attributes:
	//   host.name: NEW-prod-payment-2.example.com (was prod-payment-1.example.com)
	//   cloud.region: eu-west-1 (was us-west-2)
	//   deployment.environment: production (unchanged)
	//   k8s.pod.name: NEW attribute added after migration

	rm := md.ResourceMetrics().AppendEmpty()
	res := rm.Resource()
	res.Attributes().PutStr("service.name", "payment-service")         // Same
	res.Attributes().PutStr("service.namespace", "production")         // Same
	res.Attributes().PutStr("service.instance.id", "payment-001")      // Same
	res.Attributes().PutStr("host.name", "prod-payment-2.example.com") // CHANGED
	res.Attributes().PutStr("cloud.region", "eu-west-1")               // CHANGED
	res.Attributes().PutStr("deployment.environment", "production")    // Same
	res.Attributes().PutStr("k8s.pod.name", "payment-7d4f8b9c5-xk2pq") // NEW

	// Add entity_refs: service entity + host entity (same structure, different descriptive values)
	entityRefs := entity.ResourceEntityRefs(res)
	// Service entity
	serviceRef := entityRefs.AppendEmpty()
	serviceRef.SetType("service")
	serviceRef.IdKeys().Append("service.name")
	serviceRef.IdKeys().Append("service.namespace")
	serviceRef.IdKeys().Append("service.instance.id")
	serviceRef.DescriptionKeys().Append("deployment.environment")
	serviceRef.DescriptionKeys().Append("k8s.pod.name") // NEW descriptive attribute
	// Host entity
	hostRef := entityRefs.AppendEmpty()
	hostRef.SetType("host")
	hostRef.IdKeys().Append("host.name")
	hostRef.DescriptionKeys().Append("cloud.region")

	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("http_requests_total")
	m.SetDescription("Total HTTP requests")
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp := sum.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	dp.SetDoubleValue(2500) // Counter increased
	dp.Attributes().PutStr("method", "GET")
	dp.Attributes().PutStr("status", "200")

	return md
}

// sendOTLPMetrics sends metrics to the OTLP receiver.
func sendOTLPMetrics(md pmetric.Metrics) error {
	marshaler := pmetric.JSONMarshaler{}
	data, err := marshaler.MarshalMetrics(md)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	url := fmt.Sprintf("http://%s%s", serverAddr, otlpPath)
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

// printSentResources prints a summary of sent resource metrics.
func printSentResources(md pmetric.Metrics) {
	fmt.Printf("%sSent resources:%s\n", colorBold, colorReset)
	for i := 0; i < md.ResourceMetrics().Len(); i++ {
		rm := md.ResourceMetrics().At(i)
		res := rm.Resource()

		serviceName := ""
		serviceNS := ""
		res.Attributes().Range(func(k string, v pcommon.Value) bool {
			if k == "service.name" {
				serviceName = v.AsString()
			}
			if k == "service.namespace" {
				serviceNS = v.AsString()
			}
			return true
		})

		fmt.Printf("  %s%d.%s %s%s%s/%s%s%s\n",
			colorGreen, i+1, colorReset,
			colorYellow, serviceNS, colorReset,
			colorCyan, serviceName, colorReset)

		// Print all attributes
		res.Attributes().Range(func(k string, v pcommon.Value) bool {
			fmt.Printf("      %s%s%s: %s\n", colorGray, k, colorReset, v.AsString())
			return true
		})
	}
	fmt.Println()
}

// printResourceAttributes prints resource attributes from a metadata reader.
// Shows all versions for each series when multiple versions exist.
func printResourceAttributes(title string, reader seriesmetadata.Reader, db *tsdb.DB) {
	fmt.Printf("%s%s%s\n", colorBold, title, colorReset)

	labelsMap := buildHashToLabelsMap(db)

	type entry struct {
		hash  uint64
		attrs *seriesmetadata.VersionedResource
	}
	var entries []entry

	_ = reader.IterVersionedResources(func(labelsHash uint64, attrs *seriesmetadata.VersionedResource) error {
		entries = append(entries, entry{hash: labelsHash, attrs: attrs})
		return nil
	})

	if len(entries) == 0 {
		fmt.Printf("  %s(no resource attributes found)%s\n\n", colorGray, colorReset)
		return
	}

	// Sort by labels string for consistent output (metric name is first label)
	slices.SortFunc(entries, func(a, b entry) int {
		aLabels := ""
		bLabels := ""
		if lbs, ok := labelsMap[a.hash]; ok {
			aLabels = lbs.String()
		}
		if lbs, ok := labelsMap[b.hash]; ok {
			bLabels = lbs.String()
		}
		if aLabels < bLabels {
			return -1
		}
		if aLabels > bLabels {
			return 1
		}
		return 0
	})

	for i, e := range entries {
		if lbs, ok := labelsMap[e.hash]; ok {
			fmt.Printf("  %s%d.%s Series: %s%s%s\n", colorGreen, i+1, colorReset, colorCyan, lbs.String(), colorReset)
		} else {
			fmt.Printf("  %s%d.%s Series (hash: %d)\n", colorGreen, i+1, colorReset, e.hash)
		}

		// Show number of versions
		numVersions := len(e.attrs.Versions)
		if numVersions > 1 {
			fmt.Printf("     %s(%d versions)%s\n", colorMagenta, numVersions, colorReset)
		}

		// Print each version
		for v, ver := range e.attrs.Versions {
			if numVersions > 1 {
				fmt.Printf("     %s--- Version %d ---%s\n", colorGray, v+1, colorReset)
			}

			fmt.Printf("     %sIdentifying Attributes:%s\n", colorBold, colorReset)
			// Sort and print identifying attributes
			idKeys := make([]string, 0, len(ver.Identifying))
			for k := range ver.Identifying {
				idKeys = append(idKeys, k)
			}
			slices.Sort(idKeys)
			for _, k := range idKeys {
				fmt.Printf("       %s%s%s: %s\n", colorCyan, k, colorReset, ver.Identifying[k])
			}

			fmt.Printf("     %sDescriptive Attributes:%s\n", colorBold, colorReset)
			// Sort and print descriptive attributes
			descKeys := make([]string, 0, len(ver.Descriptive))
			for k := range ver.Descriptive {
				descKeys = append(descKeys, k)
			}
			slices.Sort(descKeys)
			for _, k := range descKeys {
				fmt.Printf("       %s%s%s: %s\n", colorYellow, k, colorReset, ver.Descriptive[k])
			}

			// Print entities if present
			if len(ver.Entities) > 0 {
				fmt.Printf("     %sEntities:%s\n", colorBold, colorReset)
				for _, ent := range ver.Entities {
					fmt.Printf("       %s[%s]%s\n", colorMagenta, ent.Type, colorReset)
					if len(ent.ID) > 0 {
						fmt.Printf("         Identifying: ")
						entIDKeys := make([]string, 0, len(ent.ID))
						for k := range ent.ID {
							entIDKeys = append(entIDKeys, k)
						}
						slices.Sort(entIDKeys)
						for i, k := range entIDKeys {
							if i > 0 {
								fmt.Printf(", ")
							}
							fmt.Printf("%s%s%s=%s", colorCyan, k, colorReset, ent.ID[k])
						}
						fmt.Println()
					}
					if len(ent.Description) > 0 {
						fmt.Printf("         Descriptive: ")
						entDescKeys := make([]string, 0, len(ent.Description))
						for k := range ent.Description {
							entDescKeys = append(entDescKeys, k)
						}
						slices.Sort(entDescKeys)
						for i, k := range entDescKeys {
							if i > 0 {
								fmt.Printf(", ")
							}
							fmt.Printf("%s%s%s=%s", colorYellow, k, colorReset, ent.Description[k])
						}
						fmt.Println()
					}
				}
			}

			if ver.MinTime > 0 {
				fmt.Printf("     %sTime Range:%s %s - %s\n",
					colorBold, colorReset,
					time.UnixMilli(ver.MinTime).Format(time.RFC3339),
					time.UnixMilli(ver.MaxTime).Format(time.RFC3339))
			}
		}
	}
	fmt.Println()
}

// printResourceAttributesFiltered prints resource attributes filtered by service name and namespace.
// This highlights version history showing how descriptive attributes changed over time.
func printResourceAttributesFiltered(title string, reader seriesmetadata.Reader, serviceName, serviceNamespace string, db *tsdb.DB) {
	fmt.Printf("%s%s%s\n", colorBold, title, colorReset)

	labelsMap := buildHashToLabelsMap(db)

	type entry struct {
		hash  uint64
		attrs *seriesmetadata.VersionedResource
	}
	var entries []entry

	_ = reader.IterVersionedResources(func(labelsHash uint64, attrs *seriesmetadata.VersionedResource) error {
		// Filter by current version's service name/namespace from identifying attributes
		current := attrs.CurrentVersion()
		if current != nil && current.Identifying[seriesmetadata.AttrServiceName] == serviceName && current.Identifying[seriesmetadata.AttrServiceNamespace] == serviceNamespace {
			entries = append(entries, entry{hash: labelsHash, attrs: attrs})
		}
		return nil
	})

	if len(entries) == 0 {
		fmt.Printf("  %s(no matching resource attributes found)%s\n\n", colorGray, colorReset)
		return
	}

	for i, e := range entries {
		if lbs, ok := labelsMap[e.hash]; ok {
			fmt.Printf("  %s%d.%s Series: %s%s%s\n", colorGreen, i+1, colorReset, colorCyan, lbs.String(), colorReset)
		} else {
			fmt.Printf("  %s%d.%s Series (hash: %d)\n", colorGreen, i+1, colorReset, e.hash)
		}

		numVersions := len(e.attrs.Versions)
		fmt.Printf("     %s%d version(s) of resource attributes%s\n", colorMagenta, numVersions, colorReset)

		// Print each version showing how attributes changed
		for v, ver := range e.attrs.Versions {
			fmt.Printf("     %s--- Version %d ---%s\n", colorGray, v+1, colorReset)

			fmt.Printf("     %sIdentifying Attributes:%s\n", colorBold, colorReset)
			// Sort and print identifying attributes
			idKeys := make([]string, 0, len(ver.Identifying))
			for k := range ver.Identifying {
				idKeys = append(idKeys, k)
			}
			slices.Sort(idKeys)
			for _, k := range idKeys {
				fmt.Printf("       %s%s%s: %s\n", colorCyan, k, colorReset, ver.Identifying[k])
			}

			fmt.Printf("     %sDescriptive Attributes:%s\n", colorBold, colorReset)
			// Sort and print descriptive attributes
			descKeys := make([]string, 0, len(ver.Descriptive))
			for k := range ver.Descriptive {
				descKeys = append(descKeys, k)
			}
			slices.Sort(descKeys)
			for _, k := range descKeys {
				fmt.Printf("       %s%s%s: %s\n", colorYellow, k, colorReset, ver.Descriptive[k])
			}

			// Print entities if present
			if len(ver.Entities) > 0 {
				fmt.Printf("     %sEntities:%s\n", colorBold, colorReset)
				for _, ent := range ver.Entities {
					fmt.Printf("       %s[%s]%s\n", colorMagenta, ent.Type, colorReset)
					if len(ent.ID) > 0 {
						fmt.Printf("         Identifying: ")
						entIDKeys := make([]string, 0, len(ent.ID))
						for k := range ent.ID {
							entIDKeys = append(entIDKeys, k)
						}
						slices.Sort(entIDKeys)
						for i, k := range entIDKeys {
							if i > 0 {
								fmt.Printf(", ")
							}
							fmt.Printf("%s%s%s=%s", colorCyan, k, colorReset, ent.ID[k])
						}
						fmt.Println()
					}
					if len(ent.Description) > 0 {
						fmt.Printf("         Descriptive: ")
						entDescKeys := make([]string, 0, len(ent.Description))
						for k := range ent.Description {
							entDescKeys = append(entDescKeys, k)
						}
						slices.Sort(entDescKeys)
						for i, k := range entDescKeys {
							if i > 0 {
								fmt.Printf(", ")
							}
							fmt.Printf("%s%s%s=%s", colorYellow, k, colorReset, ent.Description[k])
						}
						fmt.Println()
					}
				}
			}

			if ver.MinTime > 0 {
				fmt.Printf("     %sTime Range:%s %s - %s\n",
					colorBold, colorReset,
					time.UnixMilli(ver.MinTime).Format(time.RFC3339),
					time.UnixMilli(ver.MaxTime).Format(time.RFC3339))
			}
		}
	}
	fmt.Println()
}

// printPhase prints a phase header.
func printPhase(num int, title string) {
	fmt.Printf("%s%s--- Phase %d: %s ---%s\n\n", colorBold, colorMagenta, num, title, colorReset)
}

// printPromQLResult prints the result of a PromQL query, showing series with their labels and values.
func printPromQLResult(result *promql.Result) {
	if result.Value == nil {
		fmt.Printf("  %s(no results)%s\n\n", colorGray, colorReset)
		return
	}

	switch v := result.Value.(type) {
	case promql.Vector:
		if len(v) == 0 {
			fmt.Printf("  %s(no results)%s\n\n", colorGray, colorReset)
			return
		}
		for _, sample := range v {
			// Print metric labels with color coding for resource attributes
			fmt.Printf("  %s%s%s ", colorCyan, sample.Metric.Get("__name__"), colorReset)
			fmt.Printf("{")
			first := true
			sample.Metric.Range(func(l labels.Label) {
				if l.Name == "__name__" {
					return
				}
				if !first {
					fmt.Printf(", ")
				}
				first = false
				// Color code identifying vs descriptive attributes
				switch l.Name {
				case "service.name", "service.namespace", "service.instance.id":
					fmt.Printf("%s%s%s=%q", colorCyan, l.Name, colorReset, l.Value)
				case "host.name", "cloud.region", "deployment.environment", "k8s.pod.name":
					fmt.Printf("%s%s%s=%q", colorYellow, l.Name, colorReset, l.Value)
				default:
					fmt.Printf("%s=%q", l.Name, l.Value)
				}
			})
			fmt.Printf("} %s%.0f%s\n", colorBold, sample.F, colorReset)
		}
		fmt.Println()
	case promql.Matrix:
		if len(v) == 0 {
			fmt.Printf("  %s(no results)%s\n\n", colorGray, colorReset)
			return
		}
		for _, series := range v {
			fmt.Printf("  %s%s%s ", colorCyan, series.Metric.Get("__name__"), colorReset)
			fmt.Printf("{")
			first := true
			series.Metric.Range(func(l labels.Label) {
				if l.Name == "__name__" {
					return
				}
				if !first {
					fmt.Printf(", ")
				}
				first = false
				switch l.Name {
				case "service.name", "service.namespace", "service.instance.id":
					fmt.Printf("%s%s%s=%q", colorCyan, l.Name, colorReset, l.Value)
				case "host.name", "cloud.region", "deployment.environment", "k8s.pod.name":
					fmt.Printf("%s%s%s=%q", colorYellow, l.Name, colorReset, l.Value)
				default:
					fmt.Printf("%s=%q", l.Name, l.Value)
				}
			})
			fmt.Printf("}")
			if len(series.Floats) > 0 {
				fmt.Printf(" %s%.0f%s", colorBold, series.Floats[len(series.Floats)-1].F, colorReset)
			}
			fmt.Println()
		}
		fmt.Println()
	default:
		fmt.Printf("  %sResult type: %T%s\n\n", colorGray, v, colorReset)
	}
}

// buildHashToLabelsMap builds a map from label hash to labels by querying all series from the DB.
// Note: Resource attributes use labels.StableHash(), not labels.Hash().
func buildHashToLabelsMap(db *tsdb.DB) map[uint64]labels.Labels {
	result := make(map[uint64]labels.Labels)

	// Get all series from the head index
	headIdx, err := db.Head().Index()
	if err == nil {
		defer headIdx.Close()
		// Use PostingsForLabelMatching to get all series with any __name__
		p := headIdx.PostingsForLabelMatching(context.Background(), "__name__", func(string) bool { return true })
		var builder labels.ScratchBuilder
		for p.Next() {
			ref := p.At()
			if err := headIdx.Series(ref, &builder, nil); err == nil {
				lbs := builder.Labels()
				// Resource attributes are keyed by StableHash, not Hash
				result[labels.StableHash(lbs)] = lbs.Copy()
			}
		}
	}

	// Also get series from blocks
	for _, b := range db.Blocks() {
		idx, err := b.Index()
		if err != nil {
			continue
		}
		p := idx.PostingsForLabelMatching(context.Background(), "__name__", func(string) bool { return true })
		var builder labels.ScratchBuilder
		for p.Next() {
			ref := p.At()
			if err := idx.Series(ref, &builder, nil); err == nil {
				lbs := builder.Labels()
				// Resource attributes are keyed by StableHash, not Hash
				result[labels.StableHash(lbs)] = lbs.Copy()
			}
		}
		idx.Close()
	}

	return result
}

// buildResourceAttributesAPIResponse builds a response in the /api/v1/resources format.
func buildResourceAttributesAPIResponse(db *tsdb.DB) map[string]any {
	reader, err := db.SeriesMetadata()
	if err != nil {
		return map[string]any{
			"status": "error",
			"error":  err.Error(),
		}
	}
	defer reader.Close()

	labelsMap := buildHashToLabelsMap(db)

	type entityEntry struct {
		Type        string            `json:"type"`
		Identifying map[string]string `json:"identifying"`
		Descriptive map[string]string `json:"descriptive"`
	}

	type resourceAttributeData struct {
		Identifying map[string]string `json:"identifying"`
		Descriptive map[string]string `json:"descriptive"`
	}

	type versionEntry struct {
		ResourceAttributes resourceAttributeData `json:"resource_attributes"`
		Entities           []entityEntry         `json:"entities,omitempty"`
		MinTimeMs          int64                 `json:"min_time_ms"`
		MaxTimeMs          int64                 `json:"max_time_ms"`
	}

	type responseEntry struct {
		Labels   map[string]string `json:"labels"`
		Versions []versionEntry    `json:"versions"`
	}

	var data []responseEntry
	_ = reader.IterVersionedResources(func(labelsHash uint64, attrs *seriesmetadata.VersionedResource) error {
		lbls := make(map[string]string)
		if lbs, ok := labelsMap[labelsHash]; ok {
			lbs.Range(func(l labels.Label) {
				lbls[l.Name] = l.Value
			})
		} else {
			lbls["__hash__"] = strconv.FormatUint(labelsHash, 10)
		}

		// Build versions with resource attributes and entities (if any)
		var versions []versionEntry
		for _, v := range attrs.Versions {
			ve := versionEntry{
				ResourceAttributes: resourceAttributeData{
					Identifying: v.Identifying,
					Descriptive: v.Descriptive,
				},
				MinTimeMs: v.MinTime,
				MaxTimeMs: v.MaxTime,
			}

			// Include entities if present (only when entity_refs were in OTLP data)
			for _, entity := range v.Entities {
				ve.Entities = append(ve.Entities, entityEntry{
					Type:        entity.Type,
					Identifying: entity.ID,
					Descriptive: entity.Description,
				})
			}

			versions = append(versions, ve)
		}

		data = append(data, responseEntry{
			Labels:   lbls,
			Versions: versions,
		})
		return nil
	})

	// Sort by metric name for consistent output
	slices.SortFunc(data, func(a, b responseEntry) int {
		aName := a.Labels["__name__"]
		bName := b.Labels["__name__"]
		if aName < bName {
			return -1
		}
		if aName > bName {
			return 1
		}
		return 0
	})

	// Limit to first 3 series for readability
	if len(data) > 3 {
		data = data[:3]
	}

	return map[string]any{
		"status": "success",
		"data":   data,
	}
}
