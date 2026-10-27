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
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/grafana/regexp"
	"go.yaml.in/yaml/v3"
)

// registryURLRe matches the only accepted shape of __schema_url__ values:
// a single non-empty path segment under registry/. Path traversal (..) and
// absolute paths are rejected.
var registryURLRe = regexp.MustCompile(`^registry/[^/.][^/]*$`)

// otelSchema represents an OpenTelemetry Telemetry Policy transformation schema
// file (file_format: transform/2.0, see open-telemetry/weaver#1850).
type otelSchema struct {
	FileFormat string `yaml:"file_format"`
	SchemaURL  string `yaml:"schema_url"`

	transform *transformFile
}

func decodeEffectiveYAMLMapping(value *yaml.Node) (map[string]yaml.Node, []string, error) {
	var fields map[string]yaml.Node
	if err := value.Decode(&fields); err != nil {
		return nil, nil, err
	}
	return fields, slices.Sorted(maps.Keys(fields)), nil
}

// staticCache is a generic, goroutine-safe cache keyed by URL for static
// (immutable, bounded) values.
type staticCache[T any] struct {
	m sync.Map // url -> T
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

// loadOTelSchema parses raw OTel transform/2.0 schema YAML bytes.
func loadOTelSchema(b []byte) (otelSchema, error) {
	var s otelSchema
	if err := yaml.Unmarshal(b, &s); err != nil {
		return otelSchema{}, fmt.Errorf("unmarshal: %w", err)
	}
	if s.FileFormat != transformFileFormat {
		return otelSchema{}, fmt.Errorf("unsupported OTel schema file format %q (expected %s)", s.FileFormat, transformFileFormat)
	}
	tf, err := loadTransformFile(b)
	if err != nil {
		return otelSchema{}, err
	}
	s.transform = tf
	if s.SchemaURL == "" {
		s.SchemaURL = tf.targetURL
	}
	return s, nil
}

// readRegistryFile reads a file from the engine's registry source. The path must
// satisfy registryURLRe — HTTP URLs and arbitrary local files are rejected to
// keep the __schema_url__ matcher from acting as a server-side fetch primitive.
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
