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

package semconv

import (
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"go.yaml.in/yaml/v3"
)

// TODO: Remove me after done testing.
//
//go:embed registry/*
var embeddedRegistry embed.FS

// semconv represents a semantic conventions file containing metric definitions.
// See: https://github.com/open-telemetry/semantic-conventions
//
// Example semconv YAML:
//
//	groups:
//	  - id: metric.http.server.duration
//	    type: metric
//	    metric_name: http.server.duration
//	    instrument: histogram
//	    unit: s
type semconv struct {
	Groups []semconvGroup `yaml:"groups"`

	version string

	// Pre-computed lookups (populated by init()).
	metricMetadata      map[string]metricMeta
	attributesPerMetric map[string][]string
}

// otelSchema represents an OpenTelemetry schema file.
// See: https://opentelemetry.io/docs/specs/otel/schemas/file_format_v1.1.0/
//
// Example schema YAML:
//
//	file_format: 1.1.0
//	schema_url: https://example.com/schemas/1.0.0
//
//	versions:
//	  1.0.0:
//	    metrics:
//	      changes:
//	        - rename_attributes:
//	            attribute_map:
//	              http.method: http.request.method
//	            apply_to_metrics:
//	              - http.server.duration
type otelSchema struct {
	FileFormat string                       `yaml:"file_format"`
	SchemaURL  string                       `yaml:"schema_url"`
	Versions   map[string]otelSchemaVersion `yaml:"versions"`

	// Pre-computed lookups (populated by fetchOTelSchema).
	attributesPerMetric map[string]map[string]struct{}
	versionRenames      []versionRenames // Per-version renames for generating matcher variants.
}

// versionRenames holds bidirectional rename mappings from a single schema version.
// When a version renames a→b, both directions are stored for lookup.
type versionRenames struct {
	version    string            // e.g., "1.1.0"
	metrics    map[string]string // metric name → its variant (bidirectional)
	attributes map[string]string // attribute name → its variant (bidirectional)
}

// collectVersionRenames extracts bidirectional rename mappings from a schema version.
func collectVersionRenames(versionStr string, version otelSchemaVersion) *versionRenames {
	renames := &versionRenames{
		version:    versionStr,
		metrics:    make(map[string]string),
		attributes: make(map[string]string),
	}

	// Collect attribute renames from "all" section.
	if version.All != nil {
		for _, change := range version.All.Changes {
			if change.RenameAttributes != nil {
				for oldName, newName := range change.RenameAttributes.AttributeMap {
					renames.attributes[oldName] = newName
					renames.attributes[newName] = oldName
				}
			}
		}
	}

	// Collect metric and attribute renames from "metrics" section.
	if version.Metrics != nil {
		for _, change := range version.Metrics.Changes {
			if change.RenameMetrics != nil {
				for oldName, newName := range change.RenameMetrics.NameMap {
					renames.metrics[oldName] = newName
					renames.metrics[newName] = oldName
				}
			}
			if change.RenameAttributes != nil {
				for oldName, newName := range change.RenameAttributes.AttributeMap {
					renames.attributes[oldName] = newName
					renames.attributes[newName] = oldName
				}
			}
		}
	}

	if len(renames.metrics) == 0 && len(renames.attributes) == 0 {
		return nil
	}
	return renames
}

// semconvGroup represents a semantic conventions group definition.
type semconvGroup struct {
	ID         string             `yaml:"id"`
	Type       string             `yaml:"type"`        // "metric", "attribute", "span", etc.
	MetricName string             `yaml:"metric_name"` // Only for type="metric"
	Instrument string             `yaml:"instrument"`  // counter, histogram, gauge, updowncounter
	Unit       string             `yaml:"unit"`
	Attributes []semconvAttribute `yaml:"attributes,omitempty"`
}

type semconvAttribute struct {
	// Ref to attribute ID.
	Ref string `yaml:"ref"`
}

// metricMeta contains unit and type information for a metric.
type metricMeta struct {
	Unit string
	Type model.MetricType
}

type otelSchemaVersion struct {
	All     *otelSchemaSection `yaml:"all,omitempty"`
	Metrics *otelSchemaSection `yaml:"metrics,omitempty"`
}

type otelSchemaSection struct {
	Changes []otelSchemaChange `yaml:"changes,omitempty"`
}

type otelSchemaChange struct {
	RenameAttributes *otelRenameAttributes `yaml:"rename_attributes,omitempty"`
	RenameMetrics    *otelRenameMetrics    `yaml:"rename_metrics,omitempty"`
}

type otelRenameAttributes struct {
	AttributeMap   map[string]string `yaml:"attribute_map,omitempty"`
	ApplyToMetrics []string          `yaml:"apply_to_metrics,omitempty"`
}

type otelRenameMetrics struct {
	NameMap map[string]string `yaml:"name_map,omitempty"`
}

// instrumentToMetricType converts OTel instrument to Prometheus metric type.
func instrumentToMetricType(instrument string) model.MetricType {
	switch instrument {
	case "counter":
		return model.MetricTypeCounter
	case "histogram":
		return model.MetricTypeHistogram
	case "gauge", "updowncounter":
		return model.MetricTypeGauge
	default:
		return model.MetricTypeUnknown
	}
}

func (s *otelSchema) collectAttributesFromVersion(version otelSchemaVersion) {
	// Collect from "all" section (global attributes).
	if version.All != nil {
		for _, change := range version.All.Changes {
			if change.RenameAttributes == nil {
				continue
			}

			for oldName, newName := range change.RenameAttributes.AttributeMap {
				s.attributesPerMetric[""][oldName] = struct{}{}
				s.attributesPerMetric[""][newName] = struct{}{}
			}
		}
	}

	// Collect from metrics section.
	if version.Metrics != nil {
		for _, change := range version.Metrics.Changes {
			if change.RenameAttributes == nil {
				continue
			}

			attrs := change.RenameAttributes
			if len(attrs.ApplyToMetrics) == 0 {
				// Applies to all metrics - add to global.
				for oldName, newName := range attrs.AttributeMap {
					s.attributesPerMetric[""][oldName] = struct{}{}
					s.attributesPerMetric[""][newName] = struct{}{}
				}
			} else {
				// Applies to specific metrics.
				for _, metricName := range attrs.ApplyToMetrics {
					if s.attributesPerMetric[metricName] == nil {
						s.attributesPerMetric[metricName] = make(map[string]struct{})
					}
					for oldName, newName := range attrs.AttributeMap {
						s.attributesPerMetric[metricName][oldName] = struct{}{}
						s.attributesPerMetric[metricName][newName] = struct{}{}
					}
				}
			}
		}
	}
}

// getAttributesForMetric returns all attribute names that apply to a given metric,
// including global attributes from the "all" section.
func (s *otelSchema) getAttributesForMetric(metricName string) []string {
	global := s.attributesPerMetric[""]
	specific := s.attributesPerMetric[metricName]

	// Fast path: no metric-specific attributes.
	if len(specific) == 0 {
		result := make([]string, 0, len(global))
		for attr := range global {
			result = append(result, attr)
		}
		return result
	}

	// Merge global and metric-specific, using specific as the base since we need to check for dups.
	result := make([]string, 0, len(global)+len(specific))
	for attr := range specific {
		result = append(result, attr)
	}
	for attr := range global {
		if _, exists := specific[attr]; !exists {
			result = append(result, attr)
		}
	}
	return result
}

// cache is a generic TTL cache for fetched resources.
type cache[T any] struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry[T]
}

type cacheEntry[T any] struct {
	value     T
	fetchTime time.Time
}

func newCache[T any]() *cache[T] {
	return &cache[T]{entries: make(map[string]cacheEntry[T])}
}

func (c *cache[T]) get(url string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[url]
	if !ok || time.Since(entry.fetchTime) > cacheTTL {
		var zero T
		return zero, false
	}
	return entry.value, true
}

func (c *cache[T]) set(url string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[url] = cacheEntry[T]{value: value, fetchTime: time.Now()}
}

func fetchOTelSchema(url string) (otelSchema, error) {
	var s otelSchema
	if err := fetchAndUnmarshal(url, &s); err != nil {
		return otelSchema{}, fmt.Errorf("fetch OTel schema %q: %w", url, err)
	}
	if s.FileFormat != "1.1.0" && s.FileFormat != "1.0.0" {
		return otelSchema{}, fmt.Errorf("unsupported OTel schema file format %q (expected 1.0.0 or 1.1.0)", s.FileFormat)
	}

	s.attributesPerMetric = map[string]map[string]struct{}{
		"": {},
	}
	s.versionRenames = make([]versionRenames, 0, len(s.Versions))

	// Collect per-version renames.
	for versionStr, version := range s.Versions {
		s.collectAttributesFromVersion(version)
		if renames := collectVersionRenames(versionStr, version); renames != nil {
			s.versionRenames = append(s.versionRenames, *renames)
		}
	}

	// Sort versions for ordered traversal.
	slices.SortFunc(s.versionRenames, func(a, b versionRenames) int {
		return compareSemver(a.version, b.version)
	})

	return s, nil
}

// compareSemver compares two semver strings (e.g., "1.2.3").
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareSemver(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	for i := 0; i < max(len(partsA), len(partsB)); i++ {
		var numA, numB int
		if i < len(partsA) {
			numA, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			numB, _ = strconv.Atoi(partsB[i])
		}
		if numA < numB {
			return -1
		}
		if numA > numB {
			return 1
		}
	}
	return 0
}

func fetchSemconv(url string) (semconv, error) {
	var s semconv
	if err := fetchAndUnmarshal(url, &s); err != nil {
		return semconv{}, fmt.Errorf("fetch semconv %q: %w", url, err)
	}

	_, s.version = path.Split(url)
	// TODO: Verify that s.version is valid.
	s.metricMetadata = make(map[string]metricMeta)
	s.attributesPerMetric = make(map[string][]string)
	for _, group := range s.Groups {
		if group.Type != "metric" || group.MetricName == "" {
			continue
		}

		s.metricMetadata[group.MetricName] = metricMeta{
			Unit: group.Unit,
			Type: instrumentToMetricType(group.Instrument),
		}

		if len(group.Attributes) == 0 {
			continue
		}

		attrs := make([]string, 0, len(group.Attributes))
		for _, attr := range group.Attributes {
			attrs = append(attrs, attr.Ref)
		}
		s.attributesPerMetric[group.MetricName] = attrs
	}
	return s, nil
}

func fetchAndUnmarshal[T any](url string, out *T) error {
	var b []byte
	switch {
	case strings.HasPrefix(url, "http"):
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("http fetch %s: %w", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("http fetch %s, got non-200 status: %d", url, resp.StatusCode)
		}

		// TODO(bwplotka): Add limit.
		b, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read from http %s: %w", url, err)
		}
	// TODO: Remove me after testing.
	case strings.HasPrefix(url, "registry/"):
		var err error
		b, err = embeddedRegistry.ReadFile(url)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", url, err)
		}
	default:
		var err error
		b, err = os.ReadFile(url)
		if err != nil {
			return fmt.Errorf("read file %s: %w", url, err)
		}
	}
	if err := yaml.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unmarshal %q: %w", url, err)
	}
	return nil
}
