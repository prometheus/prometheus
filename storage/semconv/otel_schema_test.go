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
	"maps"
	"os"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

// loadOTelSchemaFile is a test helper that reads a YAML fixture from disk and
// parses it via the same code path fetchOTelSchema uses. Tests use this rather
// than fetchOTelSchema directly because fetch* is restricted to the embedded
// registry.
func loadOTelSchemaFile(t *testing.T, path string) otelSchema {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	s, err := loadOTelSchema(b)
	require.NoError(t, err)
	return s
}

func TestLoadOTelSchema(t *testing.T) {
	t.Run("rejects unsupported file format", func(t *testing.T) {
		b, err := os.ReadFile("./testdata/otel_unsupported_format.yaml")
		require.NoError(t, err)
		_, err = loadOTelSchema(b)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported OTel schema file format")
	})

	t.Run("collects renames from the all section", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel_with_all_section.yaml")
		require.Len(t, schema.versionRenames, 1)
		attrs := schema.versionRenames[0].attributes
		// Global ("all" section) renames are collected bidirectionally...
		require.Equal(t, "global.new", attrs["global.old"])
		require.Equal(t, "global.old", attrs["global.new"])
		// ...alongside the metric-section renames.
		require.Equal(t, "metric.new", attrs["metric.old"])
		require.Equal(t, "metric.old", attrs["metric.new"])
	})

	t.Run("collects per-version metric renames", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel_with_metric_renames.yaml")
		require.Len(t, schema.versionRenames, 1)
		// Metric renames should be bidirectional.
		renames := schema.versionRenames[0]
		require.Equal(t, "new.metric.name", renames.metrics["old.metric.name"])
		require.Equal(t, "old.metric.name", renames.metrics["new.metric.name"])
		require.Equal(t, "another.new.metric", renames.metrics["another.old.metric"])
		require.Equal(t, "another.old.metric", renames.metrics["another.new.metric"])
	})

	t.Run("collects per-version attribute renames", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel.yaml")
		require.Len(t, schema.versionRenames, 1)
		renames := schema.versionRenames[0]
		require.Equal(t, "http.request.method", renames.attributes["http.method"])
		require.Equal(t, "http.method", renames.attributes["http.request.method"])
	})

	t.Run("collects renames from multiple versions", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel_with_chained_renames.yaml")
		require.Len(t, schema.versionRenames, 2)
		allMetricRenames := make(map[string]string)
		for _, r := range schema.versionRenames {
			maps.Copy(allMetricRenames, r.metrics)
		}
		require.Contains(t, allMetricRenames, "metric.v1")
		require.Contains(t, allMetricRenames, "metric.v2")
		require.Contains(t, allMetricRenames, "metric.v3")
	})

	t.Run("sorts versions by semver", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel_with_chained_renames.yaml")
		require.Len(t, schema.versionRenames, 2)
		require.Equal(t, "1.0.0", schema.versionRenames[0].version)
		require.Equal(t, "1.1.0", schema.versionRenames[1].version)
	})
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.10.0", "1.9.0", 1}, // Numeric comparison, not string.
		{"10.0.0", "9.0.0", 1}, // Double-digit major.
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			result := compareSemver(tc.a, tc.b)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestValidateSemver(t *testing.T) {
	for _, v := range []string{"1.0.0", "10.20.30", "0.0.0"} {
		require.NoErrorf(t, validateSemver(v), "expected %q to be accepted", v)
	}
	for _, v := range []string{"", "1", "1.0", "1.0.0.0", "1.0.x", "1.0.0-rc1", "v1.0.0"} {
		require.Errorf(t, validateSemver(v), "expected %q to be rejected", v)
	}
}

func TestFetchSemconv(t *testing.T) {
	t.Run("registry: loads embedded version file", func(t *testing.T) {
		sc, err := fetchSemconv("registry/1.0.0")
		require.NoError(t, err)
		require.Equal(t, "1.0.0", sc.version)
	})

	t.Run("registry: rejects path traversal", func(t *testing.T) {
		for _, url := range []string{
			"registry/../etc/passwd",
			"registry/..",
			"../etc/passwd",
			"/etc/passwd",
			"http://example.com/x.yaml",
			"https://example.com/x.yaml",
			"./testdata/otel.yaml",
			"registry/",
			"",
		} {
			_, err := fetchSemconv(url)
			require.Errorf(t, err, "expected %q to be rejected", url)
		}
	})

	t.Run("registry: rejects non-semver version segment", func(t *testing.T) {
		// registry.yaml passes the URL regex but is not a semver-named file,
		// so version derivation fails.
		_, err := fetchSemconv("registry/registry.yaml")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid semver")
	})
}

func TestTransformOTelSchemaLabels(t *testing.T) {
	t.Run("transforms metric and label names", func(t *testing.T) {
		lbls := labels.FromStrings(
			model.MetricNameLabel, "http_server_duration_seconds",
			"http_method", "GET",
			"http_status_code", "200",
			"instance", "localhost:8080",
		)

		mapping := &labelMapping{
			translatedMetric: "http.server.duration",
			translatedLabels: map[string]string{
				"http_method":      "http.method",
				"http_status_code": "http.status_code",
			},
		}

		result := transformOTelSchemaLabels(lbls, mapping)

		require.Equal(t, "http.server.duration", result.Get(model.MetricNameLabel))
		require.Equal(t, "GET", result.Get("http.method"))
		require.Equal(t, "200", result.Get("http.status_code"))
		require.Equal(t, "localhost:8080", result.Get("instance"))
		require.Empty(t, result.Get("http_method"))
		require.Empty(t, result.Get("http_status_code"))
	})

	t.Run("removes __schema_url__", func(t *testing.T) {
		lbls := labels.FromStrings(
			model.MetricNameLabel, "http_server_duration_seconds",
			schemaURLLabel, "https://example.com/otel.yaml",
			"http_method", "GET",
		)

		mapping := &labelMapping{
			translatedMetric: "http.server.duration",
			translatedLabels: map[string]string{},
		}

		result := transformOTelSchemaLabels(lbls, mapping)

		require.Empty(t, result.Get(schemaURLLabel))
		require.Equal(t, "http.server.duration", result.Get(model.MetricNameLabel))
	})
}

func TestReadRegistryFile(t *testing.T) {
	t.Run("loads embedded registry entry", func(t *testing.T) {
		b, err := readRegistryFile("registry/1.0.0")
		require.NoError(t, err)
		require.NotEmpty(t, b)
	})

	t.Run("rejects HTTP, absolute paths, traversal, and non-registry paths", func(t *testing.T) {
		for _, url := range []string{
			"http://example.com/x.yaml",
			"/etc/passwd",
			"registry/../etc/passwd",
			"registry/..",
			"./testdata/otel.yaml",
			"registry/",
		} {
			_, err := readRegistryFile(url)
			require.Errorf(t, err, "expected %q to be rejected", url)
		}
	})
}
