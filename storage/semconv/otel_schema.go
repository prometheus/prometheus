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
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/grafana/regexp"
	"go.yaml.in/yaml/v3"
)

// embeddedRegistry holds the bundled semconv and OTel schema files served as
// the default source from which __semconv_url__ and __schema_url__ values are
// resolved. Operators may replace it with their own registry via configuration
// (see AwareStorageWithRegistry); either way, queries can only ever name files
// inside the registry namespace. Accepting arbitrary HTTP URLs or local
// filesystem paths from the matchers themselves would expose a server-side
// fetch primitive to anyone able to issue a PromQL query, which is why the
// matcher values are gated by registryURLRe rather than treated as locations.
//
//go:embed registry/*
var embeddedRegistry embed.FS

// registryURLRe matches the only accepted shape of __semconv_url__ /
// __schema_url__ values: a single non-empty path segment under registry/.
// Path traversal (..) and absolute paths are rejected.
var registryURLRe = regexp.MustCompile(`^registry/[^/.][^/]*$`)

// semverRe matches plain MAJOR.MINOR.PATCH version strings. Pre-release and
// build metadata are intentionally not accepted; semconv versions in the
// registry are plain dotted integers.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// validateSemver returns an error if v is not a plain MAJOR.MINOR.PATCH string.
func validateSemver(v string) error {
	if !semverRe.MatchString(v) {
		return fmt.Errorf("invalid semver %q: expected MAJOR.MINOR.PATCH", v)
	}
	return nil
}

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

	// attributesPerMetric lists, per metric name, the attributes the metric
	// declares at this semconv version (populated by loadSemconv). It seeds
	// attribute-rename normalisation; see buildAttributeRenameMap.
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

	// versionRenames holds per-version renames for generating matcher variants
	// (populated by loadOTelSchema).
	versionRenames []versionRenames
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
	Attributes []semconvAttribute `yaml:"attributes,omitempty"`
}

type semconvAttribute struct {
	// Ref to attribute ID.
	Ref string `yaml:"ref"`
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

// staticCache is a generic, goroutine-safe cache keyed by URL for static
// (immutable, bounded) values. Entries never expire and are not evicted:
// the resolver only accepts paths inside the embedded registry, whose
// contents are bounded and immutable, so TTL- and size-based eviction would
// only add complexity for no benefit. Two callers racing on a cold key may
// both invoke the loader and Store the same value; the result is identical
// either way.
type staticCache[T any] struct {
	m sync.Map // url → T
}

func newStaticCache[T any]() *staticCache[T] {
	return &staticCache[T]{}
}

func (c *staticCache[T]) get(url string) (T, bool) {
	v, ok := c.m.Load(url)
	if !ok {
		var zero T
		return zero, false
	}
	return v.(T), true
}

func (c *staticCache[T]) set(url string, value T) {
	c.m.Store(url, value)
}

// fetchOTelSchema reads an OTel schema file from the engine's registry source
// and parses it. The URL must satisfy registryURLRe.
func (e *schemaEngine) fetchOTelSchema(url string) (otelSchema, error) {
	b, err := e.readRegistryFile(url)
	if err != nil {
		return otelSchema{}, fmt.Errorf("fetch OTel schema %q: %w", url, err)
	}
	s, err := loadOTelSchema(b)
	if err != nil {
		return otelSchema{}, fmt.Errorf("parse OTel schema %q: %w", url, err)
	}
	return s, nil
}

// loadOTelSchema parses raw OTel schema YAML bytes and post-processes them
// (validates file_format and per-version semver, collects attribute renames,
// sorts versions). It is the YAML-parsing core of fetchOTelSchema and is
// directly callable from tests that load fixtures off disk.
func loadOTelSchema(b []byte) (otelSchema, error) {
	var s otelSchema
	if err := yaml.Unmarshal(b, &s); err != nil {
		return otelSchema{}, fmt.Errorf("unmarshal: %w", err)
	}
	if s.FileFormat != "1.1.0" && s.FileFormat != "1.0.0" {
		return otelSchema{}, fmt.Errorf("unsupported OTel schema file format %q (expected 1.0.0 or 1.1.0)", s.FileFormat)
	}

	s.versionRenames = make([]versionRenames, 0, len(s.Versions))

	for versionStr, version := range s.Versions {
		if err := validateSemver(versionStr); err != nil {
			return otelSchema{}, err
		}
		if renames := collectVersionRenames(versionStr, version); renames != nil {
			s.versionRenames = append(s.versionRenames, *renames)
		}
	}

	slices.SortFunc(s.versionRenames, func(a, b versionRenames) int {
		return compareSemver(a.version, b.version)
	})

	return s, nil
}

// compareSemver compares two MAJOR.MINOR.PATCH version strings.
// Returns -1 if a < b, 0 if a == b, 1 if a > b. Both inputs must have
// already passed validateSemver; otherwise behaviour is undefined.
func compareSemver(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	for i := range partsA {
		numA, _ := strconv.Atoi(partsA[i])
		numB, _ := strconv.Atoi(partsB[i])
		if numA != numB {
			if numA < numB {
				return -1
			}
			return 1
		}
	}
	return 0
}

// fetchSemconv reads a semconv file from the engine's registry source and parses
// it. The version is derived from the last path segment of url; the URL must
// satisfy registryURLRe and the derived version must satisfy validateSemver.
func (e *schemaEngine) fetchSemconv(url string) (semconv, error) {
	b, err := e.readRegistryFile(url)
	if err != nil {
		return semconv{}, fmt.Errorf("fetch semconv %q: %w", url, err)
	}
	_, version := path.Split(url)
	s, err := loadSemconv(b, version)
	if err != nil {
		return semconv{}, fmt.Errorf("parse semconv %q: %w", url, err)
	}
	return s, nil
}

// loadSemconv parses raw semconv YAML bytes and post-processes them. The
// version is supplied by the caller (semconv files do not record their own
// version inside the YAML) and must satisfy validateSemver.
func loadSemconv(b []byte, version string) (semconv, error) {
	if err := validateSemver(version); err != nil {
		return semconv{}, err
	}
	var s semconv
	if err := yaml.Unmarshal(b, &s); err != nil {
		return semconv{}, fmt.Errorf("unmarshal: %w", err)
	}
	s.version = version
	s.attributesPerMetric = make(map[string][]string)
	for _, group := range s.Groups {
		if group.Type != "metric" || group.MetricName == "" || len(group.Attributes) == 0 {
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

// readRegistryFile reads a file from the engine's registry source. The path must
// satisfy registryURLRe — HTTP URLs and arbitrary local files are rejected to
// keep the __semconv_url__/__schema_url__ matchers from acting as a server-side
// fetch primitive. The gate applies to every source, embedded or operator-provided.
func (e *schemaEngine) readRegistryFile(url string) ([]byte, error) {
	if !registryURLRe.MatchString(url) {
		return nil, fmt.Errorf("invalid registry URL %q: only registry paths (registry/<name>) are accepted", url)
	}
	b, err := e.registry.ReadFile(url)
	if err != nil {
		return nil, fmt.Errorf("read registry file %s: %w", url, err)
	}
	return b, nil
}
