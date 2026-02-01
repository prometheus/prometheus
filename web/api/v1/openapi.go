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

// This file implements OpenAPI 3.2 specification generation for the Prometheus HTTP API.
// It provides dynamic spec building with optional path filtering.
package v1

import (
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

const (
	// OpenAPI 3.1.0 is the default version with broader compatibility.
	openAPIVersion31 = "3.1.0"
	// OpenAPI 3.2.0 supports advanced features like itemSchema for SSE streams.
	openAPIVersion32 = "3.2.0"
)

// OpenAPIOptions configures the OpenAPI spec builder.
type OpenAPIOptions struct {
	// IncludePaths filters which paths to include in the spec.
	// If empty, all paths are included.
	// Paths are matched by prefix (e.g., "/query" matches "/query" and "/query_range").
	IncludePaths []string

	// ExternalURL is the external URL of the Prometheus server (e.g., "http://prometheus.example.com:9090").
	ExternalURL string

	// Version is the API version to include in the OpenAPI spec.
	// If empty, defaults to "0.0.1-undefined".
	Version string
}

// OpenAPIBuilder builds and caches OpenAPI specifications.
type OpenAPIBuilder struct {
	mu           sync.RWMutex
	cachedYAML31 []byte // Cached OpenAPI 3.1 spec.
	cachedYAML32 []byte // Cached OpenAPI 3.2 spec.
	options      OpenAPIOptions
	logger       *slog.Logger
}

// NewOpenAPIBuilder creates a new OpenAPI builder with the given options.
func NewOpenAPIBuilder(opts OpenAPIOptions, logger *slog.Logger) *OpenAPIBuilder {
	b := &OpenAPIBuilder{
		options: opts,
		logger:  logger,
	}

	b.rebuild()
	return b
}

// rebuild constructs the OpenAPI specs for both 3.1 and 3.2 versions based on current options.
func (b *OpenAPIBuilder) rebuild() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Build OpenAPI 3.1 spec.
	doc31 := b.buildDocument(openAPIVersion31)
	yamlBytes31, err := doc31.Render()
	if err != nil {
		b.logger.Error("failed to render OpenAPI 3.1 spec - this is a bug, please report it", "err", err)
		return
	}
	b.cachedYAML31 = yamlBytes31

	// Build OpenAPI 3.2 spec.
	doc32 := b.buildDocument(openAPIVersion32)
	yamlBytes32, err := doc32.Render()
	if err != nil {
		b.logger.Error("failed to render OpenAPI 3.2 spec - this is a bug, please report it", "err", err)
		return
	}
	b.cachedYAML32 = yamlBytes32
}

// ServeOpenAPI returns the OpenAPI specification as YAML.
// By default, serves OpenAPI 3.1.0. Use ?openapi_version=3.2 for OpenAPI 3.2.0.
func (b *OpenAPIBuilder) ServeOpenAPI(w http.ResponseWriter, r *http.Request) {
	// Parse query parameter to determine which version to serve.
	requestedVersion := r.URL.Query().Get("openapi_version")

	b.mu.RLock()
	var yamlData []byte
	switch requestedVersion {
	case "3.2", "3.2.0":
		yamlData = b.cachedYAML32
	case "3.1", "3.1.0":
		yamlData = b.cachedYAML31
	default:
		// Default to OpenAPI 3.1.0 for broader compatibility.
		yamlData = b.cachedYAML31
	}
	b.mu.RUnlock()

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(yamlData)
}

// WrapHandler returns the handler unchanged (no validation).
func (*OpenAPIBuilder) WrapHandler(next http.HandlerFunc) http.HandlerFunc {
	return next
}

// shouldIncludePath checks if a path should be included based on options.
func (b *OpenAPIBuilder) shouldIncludePath(path string) bool {
	if len(b.options.IncludePaths) == 0 {
		return true
	}
	for _, include := range b.options.IncludePaths {
		if strings.HasPrefix(path, include) || path == include {
			return true
		}
	}
	return false
}

// shouldIncludePathForVersion checks if a path should be included for a specific OpenAPI version.
func (b *OpenAPIBuilder) shouldIncludePathForVersion(path, version string) bool {
	// First check IncludePaths filter.
	if !b.shouldIncludePath(path) {
		return false
	}

	// OpenAPI 3.1 excludes paths that require 3.2 features.
	// The /notifications/live endpoint uses itemSchema which is a 3.2-only feature.
	if version == openAPIVersion31 && path == "/notifications/live" {
		return false
	}

	return true
}

// buildDocument creates the OpenAPI document for the specified version using high-level structs.
func (b *OpenAPIBuilder) buildDocument(version string) *v3.Document {
	return &v3.Document{
		Version:    version,
		Info:       b.buildInfo(),
		Servers:    b.buildServers(),
		Tags:       b.buildTags(version),
		Paths:      b.buildPaths(version),
		Components: b.buildComponents(),
	}
}

// buildInfo constructs the info section.
func (b *OpenAPIBuilder) buildInfo() *base.Info {
	apiVersion := b.options.Version
	if apiVersion == "" {
		apiVersion = "0.0.1-undefined"
	}
	return &base.Info{
		Title:       "Prometheus API",
		Description: "Prometheus is an Open-Source monitoring system with a dimensional data model, flexible query language, efficient time series database and modern alerting approach.",
		Version:     apiVersion,
		Contact: &base.Contact{
			Name: "Prometheus Community",
			URL:  "https://prometheus.io/community/",
		},
	}
}

// buildServers constructs the servers section.
func (b *OpenAPIBuilder) buildServers() []*v3.Server {
	// ExternalURL is always set by computeExternalURL in main.go.
	// It includes scheme, host, port, and optional path prefix (without trailing slash).
	serverURL := "/api/v1"
	if b.options.ExternalURL != "" {
		baseURL, err := url.Parse(b.options.ExternalURL)
		if err == nil {
			// Use path.Join to properly append /api/v1 to the existing path.
			// Then use ResolveReference to construct the full URL.
			baseURL.Path = path.Join(baseURL.Path, "/api/v1")
			serverURL = baseURL.String()
		}
	}
	return []*v3.Server{
		{URL: serverURL},
	}
}

// buildTags constructs the global tags list.
// Tag summary is an OpenAPI 3.2 feature, excluded from 3.1.
// Tag description is supported in both 3.1 and 3.2.
func (*OpenAPIBuilder) buildTags(version string) []*base.Tag {
	// Define tags with all metadata.
	tagData := []struct {
		name        string
		summary     string
		description string
	}{
		{"query", "Query", "Query and evaluate PromQL expressions."},
		{"metadata", "Metadata", "Retrieve metric metadata such as type and unit."},
		{"labels", "Labels", "Query label names and values."},
		{"series", "Series", "Query and manage time series."},
		{"targets", "Targets", "Retrieve target and scrape pool information."},
		{"rules", "Rules", "Query recording and alerting rules."},
		{"alerts", "Alerts", "Query active alerts and alertmanager discovery."},
		{"status", "Status", "Retrieve server status and configuration."},
		{"admin", "Admin", "Administrative operations for TSDB management."},
		{"features", "Features", "Query enabled features."},
		{"remote", "Remote Storage", "Remote read and write endpoints."},
		{"otlp", "OTLP", "OpenTelemetry Protocol metrics ingestion."},
		{"notifications", "Notifications", "Server notifications and events."},
	}

	tags := make([]*base.Tag, 0, len(tagData))
	for _, td := range tagData {
		tag := &base.Tag{
			Name:        td.name,
			Description: td.description, // Description is supported in both 3.1 and 3.2.
		}

		// Summary is an OpenAPI 3.2 feature only.
		if version == openAPIVersion32 {
			tag.Summary = td.summary
		}

		tags = append(tags, tag)
	}

	return tags
}

// buildPaths constructs all API path definitions.
func (b *OpenAPIBuilder) buildPaths(version string) *v3.Paths {
	pathItems := orderedmap.New[string, *v3.PathItem]()

	allPaths := b.getAllPathDefinitions()
	for pair := allPaths.First(); pair != nil; pair = pair.Next() {
		if b.shouldIncludePathForVersion(pair.Key(), version) {
			pathItems.Set(pair.Key(), pair.Value())
		}
	}

	return &v3.Paths{PathItems: pathItems}
}

// getAllPathDefinitions returns all path definitions.
func (b *OpenAPIBuilder) getAllPathDefinitions() *orderedmap.Map[string, *v3.PathItem] {
	paths := orderedmap.New[string, *v3.PathItem]()

	// Query endpoints.
	paths.Set("/query", b.queryPath())
	paths.Set("/query_range", b.queryRangePath())
	paths.Set("/query_exemplars", b.queryExemplarsPath())
	paths.Set("/format_query", b.formatQueryPath())
	paths.Set("/parse_query", b.parseQueryPath())

	// Label endpoints.
	paths.Set("/labels", b.labelsPath())
	paths.Set("/label/{name}/values", b.labelValuesPath())

	// Series endpoints.
	paths.Set("/series", b.seriesPath())

	// Metadata endpoints.
	paths.Set("/metadata", b.metadataPath())

	// Target endpoints.
	paths.Set("/scrape_pools", b.scrapePoolsPath())
	paths.Set("/targets", b.targetsPath())
	paths.Set("/targets/metadata", b.targetsMetadataPath())
	paths.Set("/targets/relabel_steps", b.targetsRelabelStepsPath())

	// Rules and alerts endpoints.
	paths.Set("/rules", b.rulesPath())
	paths.Set("/alerts", b.alertsPath())
	paths.Set("/alertmanagers", b.alertmanagersPath())

	// Status endpoints.
	paths.Set("/status/config", b.statusConfigPath())
	paths.Set("/status/runtimeinfo", b.statusRuntimeInfoPath())
	paths.Set("/status/buildinfo", b.statusBuildInfoPath())
	paths.Set("/status/flags", b.statusFlagsPath())
	paths.Set("/status/tsdb", b.statusTSDBPath())
	paths.Set("/status/tsdb/blocks", b.statusTSDBBlocksPath())
	paths.Set("/status/walreplay", b.statusWALReplayPath())

	// Admin endpoints.
	paths.Set("/admin/tsdb/delete_series", b.adminDeleteSeriesPath())
	paths.Set("/admin/tsdb/clean_tombstones", b.adminCleanTombstonesPath())
	paths.Set("/admin/tsdb/snapshot", b.adminSnapshotPath())

	// Remote endpoints.
	paths.Set("/read", b.remoteReadPath())
	paths.Set("/write", b.remoteWritePath())
	paths.Set("/otlp/v1/metrics", b.otlpWritePath())

	// Notifications endpoints.
	paths.Set("/notifications", b.notificationsPath())
	paths.Set("/notifications/live", b.notificationsLivePath())

	// Features endpoint.
	paths.Set("/features", b.featuresPath())

	return paths
}
