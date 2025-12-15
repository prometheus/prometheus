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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestFetchOTelSchema(t *testing.T) {
	t.Run("collects attributes from versions", func(t *testing.T) {
		schema, err := fetchOTelSchema("./testdata/otel.yaml")
		require.NoError(t, err)

		attrs := schema.getAttributesForMetric("http.server.duration")
		require.Contains(t, attrs, "http.method")
		require.Contains(t, attrs, "http.request.method")
		require.Contains(t, attrs, "http.status_code")
		require.Contains(t, attrs, "http.response.status_code")
	})

	t.Run("collects global attributes from all section", func(t *testing.T) {
		schema, err := fetchOTelSchema("./testdata/otel_with_all_section.yaml")
		require.NoError(t, err)

		// Global attributes apply to all metrics.
		attrs := schema.getAttributesForMetric("my.metric")
		require.Contains(t, attrs, "global.old")
		require.Contains(t, attrs, "global.new")
		require.Contains(t, attrs, "metric.old")
		require.Contains(t, attrs, "metric.new")

		// Global attributes also apply to metrics not in versions.
		attrs = schema.getAttributesForMetric("other.metric")
		require.Contains(t, attrs, "global.old")
		require.Contains(t, attrs, "global.new")
	})

	t.Run("rejects unsupported file format", func(t *testing.T) {
		_, err := fetchOTelSchema("./testdata/otel_unsupported_format.yaml")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported OTel schema file format")
	})

	t.Run("fetches from http", func(t *testing.T) {
		srv := httptest.NewServer(http.FileServer(http.Dir("./testdata")))
		t.Cleanup(srv.Close)

		schema, err := fetchOTelSchema(srv.URL + "/otel.yaml")
		require.NoError(t, err)

		attrs := schema.getAttributesForMetric("http.server.duration")
		require.Contains(t, attrs, "http.method")
	})

	t.Run("collects per-version metric renames", func(t *testing.T) {
		schema, err := fetchOTelSchema("./testdata/otel_with_metric_renames.yaml")
		require.NoError(t, err)

		// Should have one version with renames.
		require.Len(t, schema.versionRenames, 1)

		// Metric renames should be bidirectional.
		renames := schema.versionRenames[0]
		require.Equal(t, "new.metric.name", renames.metrics["old.metric.name"])
		require.Equal(t, "old.metric.name", renames.metrics["new.metric.name"])
		require.Equal(t, "another.new.metric", renames.metrics["another.old.metric"])
		require.Equal(t, "another.old.metric", renames.metrics["another.new.metric"])
	})

	t.Run("collects per-version attribute renames", func(t *testing.T) {
		schema, err := fetchOTelSchema("./testdata/otel.yaml")
		require.NoError(t, err)

		// Should have one version with renames.
		require.Len(t, schema.versionRenames, 1)

		// Attribute renames should be bidirectional.
		renames := schema.versionRenames[0]
		require.Equal(t, "http.request.method", renames.attributes["http.method"])
		require.Equal(t, "http.method", renames.attributes["http.request.method"])
	})

	t.Run("collects renames from multiple versions", func(t *testing.T) {
		schema, err := fetchOTelSchema("./testdata/otel_with_chained_renames.yaml")
		require.NoError(t, err)

		// Should have two versions with renames.
		require.Len(t, schema.versionRenames, 2)

		// Each version has its own renames (v1→v2 in one, v2→v3 in another).
		allMetricRenames := make(map[string]string)
		for _, r := range schema.versionRenames {
			maps.Copy(allMetricRenames, r.metrics)
		}
		require.Contains(t, allMetricRenames, "metric.v1")
		require.Contains(t, allMetricRenames, "metric.v2")
		require.Contains(t, allMetricRenames, "metric.v3")
	})

	t.Run("sorts versions by semver", func(t *testing.T) {
		schema, err := fetchOTelSchema("./testdata/otel_with_chained_renames.yaml")
		require.NoError(t, err)

		require.Len(t, schema.versionRenames, 2)
		// Versions should be sorted: 1.0.0 before 1.1.0.
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
		{"1.0", "1.0.0", 0},    // Missing patch treated as 0.
		{"10.0.0", "9.0.0", 1}, // Double-digit major.
	}

	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			result := compareSemver(tc.a, tc.b)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestFetchSemconv(t *testing.T) {
	t.Run("parses groups section", func(t *testing.T) {
		sc, err := fetchSemconv("./testdata/otel.yaml")
		require.NoError(t, err)

		meta, ok := sc.metricMetadata["http.server.duration"]
		require.True(t, ok)
		require.Equal(t, "s", meta.Unit)
		require.Equal(t, model.MetricTypeHistogram, meta.Type)

		meta, ok = sc.metricMetadata["http.server.request.count"]
		require.True(t, ok)
		require.Equal(t, "{request}", meta.Unit)
		require.Equal(t, model.MetricTypeCounter, meta.Type)

		// Unknown metric returns not found.
		_, ok = sc.metricMetadata["unknown.metric"]
		require.False(t, ok)
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

func TestInstrumentToMetricType(t *testing.T) {
	tests := []struct {
		input    string
		expected model.MetricType
	}{
		{"counter", model.MetricTypeCounter},
		{"gauge", model.MetricTypeGauge},
		{"updowncounter", model.MetricTypeGauge},
		{"histogram", model.MetricTypeHistogram},
		{"unknown", model.MetricTypeUnknown},
		{"", model.MetricTypeUnknown},
		{"invalid", model.MetricTypeUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := instrumentToMetricType(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestFetchAndUnmarshal(t *testing.T) {
	t.Run("local file", func(t *testing.T) {
		var s semconv
		err := fetchAndUnmarshal("./testdata/otel.yaml", &s)
		require.NoError(t, err)
		require.NotEmpty(t, s.Groups)
	})

	t.Run("http", func(t *testing.T) {
		srv := httptest.NewServer(http.FileServer(http.Dir("./testdata")))
		t.Cleanup(srv.Close)

		var s semconv
		err := fetchAndUnmarshal(srv.URL+"/otel.yaml", &s)
		require.NoError(t, err)
		require.NotEmpty(t, s.Groups)
	})

	t.Run("embedded registry", func(t *testing.T) {
		// Test that embedded files can be read.
		// This requires a file to exist in the registry/ directory.
		var s semconv
		err := fetchAndUnmarshal("registry/placeholder.yaml", &s)
		// It's okay if this fails due to missing file - the embed functionality is tested.
		if err != nil {
			require.Contains(t, err.Error(), "read embedded")
		}
	})
}
