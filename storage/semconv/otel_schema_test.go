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
	"os"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

// testEngine returns a schemaEngine backed by the embedded registry, for tests
// that exercise the engine's fetch/read methods directly.
func testEngine() *schemaEngine {
	return newSchemaEngine(embeddedRegistry)
}

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
		require.Len(t, schema.revisions, 1)
		require.Len(t, schema.revisions[0].changes, 2)

		global := schema.revisions[0].changes[0].attributeRenames
		require.Equal(t, "global.new", global.renames.forward["global.old"])
		require.Equal(t, []string{"global.old"}, global.renames.reverse["global.new"])
		require.True(t, global.appliesTo("any.metric"))

		scoped := schema.revisions[0].changes[1].attributeRenames
		require.Equal(t, "metric.new", scoped.renames.forward["metric.old"])
		require.True(t, scoped.appliesTo("my.metric"))
		require.False(t, scoped.appliesTo("other.metric"))
	})

	t.Run("collects per-version metric renames", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel_with_metric_renames.yaml")
		require.Len(t, schema.revisions, 1)
		renames := schema.revisions[0].changes[0].metricRenames
		require.Equal(t, "new.metric.name", renames.forward["old.metric.name"])
		require.Equal(t, []string{"old.metric.name"}, renames.reverse["new.metric.name"])
		require.Equal(t, "another.new.metric", renames.forward["another.old.metric"])
		require.Equal(t, []string{"another.old.metric"}, renames.reverse["another.new.metric"])
	})

	t.Run("scopes per-version attribute renames to apply_to_metrics", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel.yaml")
		require.Len(t, schema.revisions, 1)
		require.Len(t, schema.revisions[0].changes, 2)
		http := schema.revisions[0].changes[0].attributeRenames
		for _, metric := range []string{"http.server.duration", "http.server.request.count"} {
			require.True(t, http.appliesTo(metric))
		}
		require.False(t, http.appliesTo("process.cpu.time"))
		require.Equal(t, "http.request.method", http.renames.forward["http.method"])

		cpu := schema.revisions[0].changes[1].attributeRenames
		require.True(t, cpu.appliesTo("process.cpu.time"))
		require.False(t, cpu.appliesTo("http.server.duration"))
		require.Equal(t, "cpu.mode", cpu.renames.forward["process.cpu.state"])
	})

	t.Run("preserves empty metric scope", func(t *testing.T) {
		schema, err := loadOTelSchema([]byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              omitted.old: omitted.new
        - rename_attributes:
            attribute_map:
              empty.old: empty.new
            apply_to_metrics: []
        - rename_attributes:
            attribute_map:
              scoped.old: scoped.new
            apply_to_metrics:
              - selected.metric
`))
		require.NoError(t, err)
		require.Len(t, schema.revisions, 1)
		changes := schema.revisions[0].changes
		require.Len(t, changes, 3)

		require.True(t, changes[0].attributeRenames.appliesTo("any.metric"))
		require.False(t, changes[1].attributeRenames.appliesTo("any.metric"))
		require.True(t, changes[2].attributeRenames.appliesTo("selected.metric"))
		require.False(t, changes[2].attributeRenames.appliesTo("other.metric"))
	})

	t.Run("collects renames from multiple versions", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel_with_chained_renames.yaml")
		require.Len(t, schema.revisions, 2)
		require.Equal(t, "metric.v2", schema.revisions[0].changes[0].metricRenames.forward["metric.v1"])
		require.Equal(t, "metric.v3", schema.revisions[1].changes[0].metricRenames.forward["metric.v2"])
	})

	t.Run("sorts versions by semver", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/otel_with_chained_renames.yaml")
		require.Len(t, schema.revisions, 2)
		require.Equal(t, "1.0.0", schema.revisions[0].version)
		require.Equal(t, "1.1.0", schema.revisions[1].version)
	})

	t.Run("keeps every predecessor of a real many-to-one rename", func(t *testing.T) {
		schema := loadOTelSchemaFile(t, "./testdata/upstream/schema-1.44.0.yaml")
		var renames *directedRenames
		for _, revision := range schema.revisions {
			if revision.version != "1.38.0" {
				continue
			}
			for _, change := range revision.changes {
				if change.metricRenames != nil {
					renames = change.metricRenames
					break
				}
			}
		}
		require.NotNil(t, renames)
		require.Equal(t, []string{
			"k8s.replication_controller.available_pods",
			"k8s.replicationcontroller.available_pods",
		}, renames.reverse["k8s.replicationcontroller.pod.available"])

		variants := requireMatcherVariants(t, "1.38.0", &schema,
			equalMatchers("k8s.replicationcontroller.pod.available"), nil, nil)
		require.ElementsMatch(t, []string{
			"k8s.replicationcontroller.pod.available",
			"k8s.replication_controller.available_pods",
			"k8s.replicationcontroller.available_pods",
		}, variantNames(variants))
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
		sc, err := testEngine().fetchSemconv("registry/1.0.0")
		require.NoError(t, err)
		require.Equal(t, "1.0.0", sc.version)
	})

	t.Run("registry: rejects path traversal", func(t *testing.T) {
		e := testEngine()
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
			_, err := e.fetchSemconv(url)
			require.Errorf(t, err, "expected %q to be rejected", url)
		}
	})

	t.Run("registry: rejects non-semver version segment", func(t *testing.T) {
		// registry.yaml passes the URL regex but is not a semver-named file,
		// so version derivation fails.
		_, err := testEngine().fetchSemconv("registry/registry.yaml")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid semver")
	})
}

func TestLoadSemconv(t *testing.T) {
	t.Run("indexes metric groups with attributes", func(t *testing.T) {
		sc, err := loadSemconv([]byte(`
groups:
  - id: metric.http.server.request.duration
    type: metric
    metric_name: http.server.request.duration
    stability: stable
    unit: s
    instrument: histogram
    attributes:
      - ref: http.request.method
`), "1.0.0")
		require.NoError(t, err)
		require.Equal(t, metricDef{attributes: []string{"http.request.method"}}, sc.metrics["http.server.request.duration"])
	})

	t.Run("indexes a metric group that declares no attributes", func(t *testing.T) {
		// Such a group is still a metric, so it must be visible to the
		// existence check that validates rename edges, even though it
		// contributes nothing to attribute-rename normalisation.
		sc, err := loadSemconv([]byte(`
groups:
  - id: metric.queue.depth
    type: metric
    metric_name: queue.depth
    unit: "{item}"
    instrument: updowncounter
`), "1.0.0")
		require.NoError(t, err)
		require.Contains(t, sc.metrics, "queue.depth")
		require.Empty(t, sc.attributesOf("queue.depth"))
	})

	t.Run("keeps the first declaration of a duplicate metric name", func(t *testing.T) {
		sc, err := loadSemconv([]byte(`
groups:
  - id: metric.shared.name
    type: metric
    metric_name: shared.name
    unit: s
    instrument: histogram
    attributes:
      - ref: http.request.method
  - id: metric.shared.name.other
    type: metric
    metric_name: shared.name
    unit: "{item}"
    instrument: updowncounter
    attributes:
      - ref: queue.name
`), "1.0.0")
		require.NoError(t, err)
		require.Equal(t, []string{"http.request.method"}, sc.attributesOf("shared.name"))
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

		result, err := transformOTelSchemaLabels(lbls, mapping)
		require.NoError(t, err)

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

		result, err := transformOTelSchemaLabels(lbls, mapping)
		require.NoError(t, err)

		require.Empty(t, result.Get(schemaURLLabel))
		require.Equal(t, "http.server.duration", result.Get(model.MetricNameLabel))
	})

	t.Run("sorts labels after a rename", func(t *testing.T) {
		lbls := labels.FromStrings(
			model.MetricNameLabel, "jvm.thread.count",
			"service.name", "api",
			"thread.daemon", "true",
		)
		mapping := &labelMapping{
			translatedMetric: "jvm.thread.count",
			translatedLabels: map[string]string{"thread.daemon": "jvm.thread.daemon"},
		}

		result, err := transformOTelSchemaLabels(lbls, mapping)
		require.NoError(t, err)
		require.Equal(t, labels.FromStrings(
			model.MetricNameLabel, "jvm.thread.count",
			"jvm.thread.daemon", "true",
			"service.name", "api",
		), result)
		require.Equal(t, "true", result.Get("jvm.thread.daemon"))
	})

	t.Run("collapses aliases with equal values", func(t *testing.T) {
		lbls := labels.FromStrings(
			model.MetricNameLabel, "jvm.thread.count",
			"jvm.thread.daemon", "true",
			"thread.daemon", "true",
		)
		mapping := &labelMapping{
			translatedMetric: "jvm.thread.count",
			translatedLabels: map[string]string{"thread.daemon": "jvm.thread.daemon"},
		}

		result, err := transformOTelSchemaLabels(lbls, mapping)
		require.NoError(t, err)
		require.Equal(t, labels.FromStrings(
			model.MetricNameLabel, "jvm.thread.count",
			"jvm.thread.daemon", "true",
		), result)
	})

	t.Run("rejects aliases with conflicting values", func(t *testing.T) {
		lbls := labels.FromStrings(
			model.MetricNameLabel, "jvm.thread.count",
			"jvm.thread.daemon", "false",
			"thread.daemon", "true",
		)
		mapping := &labelMapping{
			translatedMetric: "jvm.thread.count",
			translatedLabels: map[string]string{"thread.daemon": "jvm.thread.daemon"},
		}

		_, err := transformOTelSchemaLabels(lbls, mapping)
		require.ErrorContains(t, err, `maps "jvm.thread.daemon" and "thread.daemon" to "jvm.thread.daemon" with conflicting values`)
	})

	t.Run("handles many-to-one mappings", func(t *testing.T) {
		mapping := &labelMapping{
			translatedMetric: "metric.current",
			translatedLabels: map[string]string{
				"legacy.a": "current",
				"legacy.b": "current",
			},
		}

		result, err := transformOTelSchemaLabels(labels.FromStrings(
			model.MetricNameLabel, "metric.old",
			"legacy.a", "same",
			"legacy.b", "same",
		), mapping)
		require.NoError(t, err)
		require.Equal(t, labels.FromStrings(
			model.MetricNameLabel, "metric.current",
			"current", "same",
		), result)

		_, err = transformOTelSchemaLabels(labels.FromStrings(
			model.MetricNameLabel, "metric.old",
			"legacy.a", "one",
			"legacy.b", "two",
		), mapping)
		require.ErrorContains(t, err, `maps "legacy.a" and "legacy.b" to "current" with conflicting values`)
	})
}

func TestReadRegistryFile(t *testing.T) {
	t.Run("loads embedded registry entry", func(t *testing.T) {
		b, err := testEngine().readRegistryFile("registry/1.0.0")
		require.NoError(t, err)
		require.NotEmpty(t, b)
	})

	t.Run("rejects HTTP, absolute paths, traversal, and non-registry paths", func(t *testing.T) {
		e := testEngine()
		for _, url := range []string{
			"http://example.com/x.yaml",
			"/etc/passwd",
			"registry/../etc/passwd",
			"registry/..",
			"./testdata/otel.yaml",
			"registry/",
		} {
			_, err := e.readRegistryFile(url)
			require.Errorf(t, err, "expected %q to be rejected", url)
		}
	})
}

// TestUpstreamSemconvAttributes pins how the real semconv files' metric attributes
// parse, against the unmodified v1.44.0 artefact for semconv 1.22.0.
//
// It records a gap as much as a guarantee. Most real metric groups declare their
// attributes with extends, naming an attribute_group to inherit from, and
// semconvGroup has no such field: those groups parse with no attributes at all, so
// nothing canonicalises their attribute names across a rename and no
// apply_to_metrics scoping applies to them either. Only groups that list attributes
// inline are seen. If extends is resolved later, the second assertion here is the
// one that should change.
func TestUpstreamSemconvAttributes(t *testing.T) {
	b, err := os.ReadFile("./testdata/upstream/semconv-1.22.0.yaml")
	require.NoError(t, err)
	sc, err := loadSemconv(b, "1.22.0")
	require.NoError(t, err)

	// Declared inline, so they are parsed.
	require.Contains(t, sc.attributesOf("http.server.active_requests"), "http.request.method",
		"inline attributes of a real metric group must be parsed")

	// Declared via "extends: metric_attributes.http.server", so they are not.
	require.Empty(t, sc.attributesOf("http.server.request.duration"),
		"extends is not resolved, so this group has no attributes; if that changes, so must this")

	// Either way the group is indexed as a metric for lifecycle traversal.
	require.Contains(t, sc.metrics, "http.server.request.duration")
}
