// Copyright 2025 The Prometheus Authors
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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestIsOTLPSchema(t *testing.T) {
	tests := []struct {
		schemaURL string
		expected  bool
	}{
		{"https://prometheus.io/schema/otlp/untranslated", true},
		{"https://prometheus.io/schema/otlp/1.0.0", true},
		{"https://prometheus.io/schema/otlp", false}, // No version
		{"https://bwplotka.dev/schema/my_app/1.1.0", false},
		{"https://opentelemetry.io/schemas/1.21.0", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.schemaURL, func(t *testing.T) {
			require.Equal(t, tc.expected, isOTLPSchema(tc.schemaURL))
		})
	}
}

func TestOTLPSchemaVariants(t *testing.T) {
	t.Run("histogram with unit", func(t *testing.T) {
		e := newSchemaEngine()

		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.duration"),
			labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "https://prometheus.io/schema/otlp/untranslated"),
			labels.MustNewMatcher(labels.MatchEqual, model.MetricTypeLabel, "histogram"),
			labels.MustNewMatcher(labels.MatchEqual, model.MetricUnitLabel, "s"),
		}

		variants, qCtx, err := e.FindMatcherVariants("https://prometheus.io/schema/otlp/untranslated", matchers)
		require.NoError(t, err)
		require.Len(t, variants, 3)       // Three translation strategies
		require.NotEmpty(t, qCtx.changes) // Sentinel for transformation needed

		// Collect all generated metric names.
		names := make([]string, 0, len(variants))
		for _, v := range variants {
			for _, m := range v {
				if m.Name == model.MetricNameLabel {
					names = append(names, m.Value)
				}
			}
		}

		// Check that we got expected metric names for each strategy.
		require.Contains(t, names, "http_server_duration_seconds") // UnderscoreEscapingWithSuffixes
		require.Contains(t, names, "http_server_duration")         // UnderscoreEscapingWithoutSuffixes
		require.Contains(t, names, "http.server.duration_seconds") // NoUTF8EscapingWithSuffixes
	})

	t.Run("counter", func(t *testing.T) {
		e := newSchemaEngine()

		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.request.count"),
			labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "https://prometheus.io/schema/otlp/untranslated"),
			labels.MustNewMatcher(labels.MatchEqual, model.MetricTypeLabel, "counter"),
		}

		variants, _, err := e.FindMatcherVariants("https://prometheus.io/schema/otlp/untranslated", matchers)
		require.NoError(t, err)
		require.Len(t, variants, 3)

		names := make([]string, 0, len(variants))
		for _, v := range variants {
			for _, m := range v {
				if m.Name == model.MetricNameLabel {
					names = append(names, m.Value)
				}
			}
		}

		// Counter gets _total suffix with WithSuffixes strategies.
		require.Contains(t, names, "http_server_request_count_total") // UnderscoreEscapingWithSuffixes
		require.Contains(t, names, "http_server_request_count")       // UnderscoreEscapingWithoutSuffixes
		require.Contains(t, names, "http.server.request.count_total") // NoUTF8EscapingWithSuffixes
	})

	t.Run("with labels", func(t *testing.T) {
		e := newSchemaEngine()

		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.duration"),
			labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "https://prometheus.io/schema/otlp/untranslated"),
			labels.MustNewMatcher(labels.MatchEqual, model.MetricTypeLabel, "gauge"),
			labels.MustNewMatcher(labels.MatchEqual, "http.method", "GET"),
		}

		variants, _, err := e.FindMatcherVariants("https://prometheus.io/schema/otlp/untranslated", matchers)
		require.NoError(t, err)
		require.Len(t, variants, 3)

		// Check label transformations.
		for _, v := range variants {
			hasMethodMatcher := false
			for _, m := range v {
				if m.Name == "http_method" || m.Name == "http.method" {
					hasMethodMatcher = true
					require.Equal(t, "GET", m.Value)
				}
				// __schema_url__ should be stripped.
				require.NotEqual(t, schemaURLLabel, m.Name)
			}
			require.True(t, hasMethodMatcher, "expected http.method or http_method matcher")
		}
	})

	t.Run("unknown type and unit generates suffix variants", func(t *testing.T) {
		e := newSchemaEngine()

		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.requests"),
			labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "https://prometheus.io/schema/otlp/untranslated"),
		}

		variants, _, err := e.FindMatcherVariants("https://prometheus.io/schema/otlp/untranslated", matchers)
		require.NoError(t, err)
		// When both type and unit are unknown:
		// - 2 suffix strategies × 3 types × 5 units = 30 variants
		// - 1 no-suffix strategy × 1 type × 1 unit = 1 variant
		// Total = 31 variants
		require.Len(t, variants, 31)

		names := make([]string, 0, len(variants))
		for _, v := range variants {
			for _, m := range v {
				if m.Name == model.MetricNameLabel {
					names = append(names, m.Value)
				}
			}
		}

		// Check that we got variants with different suffixes.
		require.Contains(t, names, "http_server_requests")              // UnderscoreEscapingWithoutSuffixes
		require.Contains(t, names, "http_server_requests_total")        // UnderscoreEscapingWithSuffixes + counter
		require.Contains(t, names, "http_server_requests_seconds")      // UnderscoreEscapingWithSuffixes + unit "s"
		require.Contains(t, names, "http_server_requests_bytes")        // UnderscoreEscapingWithSuffixes + unit "By"
		require.Contains(t, names, "http.server.requests_total")        // NoUTF8EscapingWithSuffixes + counter
		require.Contains(t, names, "http.server.requests_seconds")      // NoUTF8EscapingWithSuffixes + unit "s"
		require.Contains(t, names, "http.server.requests_seconds_total") // NoUTF8EscapingWithSuffixes + counter + unit "s"
	})
}

func TestOTLPSchemaUnsupportedVersion(t *testing.T) {
	e := newSchemaEngine()

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.duration"),
		labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "https://prometheus.io/schema/otlp/translated"),
	}

	_, _, err := e.FindMatcherVariants("https://prometheus.io/schema/otlp/translated", matchers)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported OTLP schema version")
}

func TestOTLPTransformSeries(t *testing.T) {
	e := newSchemaEngine()

	t.Run("series without __schema_url__ (translated OTLP)", func(t *testing.T) {
		// Translated OTLP metrics don't have __schema_url__ stored.
		lbls := labels.FromStrings(
			model.MetricNameLabel, "http_server_duration_seconds",
			"http_method", "GET",
		)

		qCtx := queryContext{isOTLP: true}

		result, vt, err := e.TransformSeries(qCtx, lbls)
		require.NoError(t, err)

		// Labels should remain unchanged.
		require.Equal(t, "http_server_duration_seconds", result.Get(model.MetricNameLabel))
		require.Equal(t, "GET", result.Get("http_method"))

		// No value transformation for OTLP schema.
		require.Empty(t, vt.expr)
	})

	t.Run("series with __schema_url__ (OTLP query context)", func(t *testing.T) {
		// If series has __schema_url__ but query is OTLP, remove it.
		lbls := labels.FromStrings(
			model.MetricNameLabel, "http_server_duration_seconds",
			schemaURLLabel, "https://prometheus.io/schema/otlp/untranslated",
			"http_method", "GET",
		)

		qCtx := queryContext{isOTLP: true}

		result, vt, err := e.TransformSeries(qCtx, lbls)
		require.NoError(t, err)

		// __schema_url__ should be removed.
		require.Empty(t, result.Get(schemaURLLabel))

		// Other labels should remain.
		require.Equal(t, "http_server_duration_seconds", result.Get(model.MetricNameLabel))
		require.Equal(t, "GET", result.Get("http_method"))

		// No value transformation for OTLP schema.
		require.Empty(t, vt.expr)
	})

	t.Run("legacy test with __schema_url__", func(t *testing.T) {
		// Create labels as if returned from storage.
		lbls := labels.FromStrings(
			model.MetricNameLabel, "http_server_duration_seconds",
			schemaURLLabel, "https://prometheus.io/schema/otlp/untranslated",
			"http_method", "GET",
		)

		qCtx := queryContext{changes: []change{{}}} // Sentinel (non-OTLP context)

		result, vt, err := e.TransformSeries(qCtx, lbls)
		require.NoError(t, err)

		// __schema_url__ should be removed.
		require.Empty(t, result.Get(schemaURLLabel))

		// Other labels should remain.
		require.Equal(t, "http_server_duration_seconds", result.Get(model.MetricNameLabel))
		require.Equal(t, "GET", result.Get("http_method"))

		// No value transformation for OTLP schema.
		require.Empty(t, vt.expr)
	})
}

func TestPromTypeToOTelType(t *testing.T) {
	// OTel metric type constants:
	// Unknown=0, NonMonotonicCounter=1, MonotonicCounter=2, Gauge=3,
	// Histogram=4, ExponentialHistogram=5, Summary=6
	tests := []struct {
		promType model.MetricType
		expected int // Using int to avoid import dependency
	}{
		{model.MetricTypeCounter, 2},   // MetricTypeMonotonicCounter
		{model.MetricTypeGauge, 3},     // MetricTypeGauge
		{model.MetricTypeHistogram, 4}, // MetricTypeHistogram
		{model.MetricTypeSummary, 6},   // MetricTypeSummary
		{model.MetricTypeUnknown, 0},   // MetricTypeUnknown
	}

	for _, tc := range tests {
		t.Run(string(tc.promType), func(t *testing.T) {
			result := promTypeToOTelType(tc.promType)
			require.Equal(t, tc.expected, int(result))
		})
	}
}

func TestGenerateOTLPVariants(t *testing.T) {
	t.Run("known type generates 3 variants", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.duration"),
		}
		metric := otlptranslator.Metric{
			Name: "http.server.duration",
			Unit: "s",
			Type: otlptranslator.MetricTypeHistogram,
		}

		variants, err := generateOTLPVariants(matchers, metric)
		require.NoError(t, err)
		require.Len(t, variants, 3)

		names := collectMetricNames(variants)
		require.Contains(t, names, "http_server_duration_seconds") // UnderscoreEscapingWithSuffixes
		require.Contains(t, names, "http_server_duration")         // UnderscoreEscapingWithoutSuffixes
		require.Contains(t, names, "http.server.duration_seconds") // NoUTF8EscapingWithSuffixes
	})

	t.Run("unknown type and unit generates suffix variants", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.requests"),
		}
		metric := otlptranslator.Metric{
			Name: "http.server.requests",
			Type: otlptranslator.MetricTypeUnknown,
			Unit: "", // Unknown unit
		}

		variants, err := generateOTLPVariants(matchers, metric)
		require.NoError(t, err)
		// When both type and unit are unknown:
		// - 2 suffix strategies × 3 types × 5 units = 30 variants
		// - 1 no-suffix strategy × 1 type × 1 unit = 1 variant
		// Total = 31 variants
		require.Len(t, variants, 31)

		names := collectMetricNames(variants)
		require.Contains(t, names, "http_server_requests")              // UnderscoreEscapingWithoutSuffixes
		require.Contains(t, names, "http_server_requests_total")        // UnderscoreEscapingWithSuffixes + counter
		require.Contains(t, names, "http_server_requests_seconds")      // UnderscoreEscapingWithSuffixes + unit "s"
		require.Contains(t, names, "http_server_requests_bytes")        // UnderscoreEscapingWithSuffixes + unit "By"
		require.Contains(t, names, "http.server.requests")              // NoUTF8EscapingWithSuffixes + unknown
		require.Contains(t, names, "http.server.requests_total")        // NoUTF8EscapingWithSuffixes + counter
		require.Contains(t, names, "http.server.requests_seconds")      // NoUTF8EscapingWithSuffixes + unit "s"
		require.Contains(t, names, "http.server.requests_seconds_total") // NoUTF8EscapingWithSuffixes + counter + unit "s"
	})

	t.Run("counter type with known unit generates _total suffix", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.requests"),
		}
		metric := otlptranslator.Metric{
			Name: "http.server.requests",
			Type: otlptranslator.MetricTypeMonotonicCounter,
			Unit: "1", // Known unit
		}

		variants, err := generateOTLPVariants(matchers, metric)
		require.NoError(t, err)
		require.Len(t, variants, 3)

		names := collectMetricNames(variants)
		require.Contains(t, names, "http_server_requests_total") // UnderscoreEscapingWithSuffixes
		require.Contains(t, names, "http_server_requests")       // UnderscoreEscapingWithoutSuffixes
		require.Contains(t, names, "http.server.requests_total") // NoUTF8EscapingWithSuffixes
	})

	t.Run("counter type with unknown unit generates unit variants", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.requests"),
		}
		metric := otlptranslator.Metric{
			Name: "http.server.requests",
			Type: otlptranslator.MetricTypeMonotonicCounter,
			Unit: "", // Unknown unit
		}

		variants, err := generateOTLPVariants(matchers, metric)
		require.NoError(t, err)
		// 2 suffix strategies × 1 type × 5 units + 1 no-suffix strategy = 11 variants
		require.Len(t, variants, 11)

		names := collectMetricNames(variants)
		require.Contains(t, names, "http_server_requests_total")         // UnderscoreEscapingWithSuffixes + no unit
		require.Contains(t, names, "http_server_requests_seconds_total") // UnderscoreEscapingWithSuffixes + unit "s"
		require.Contains(t, names, "http_server_requests_bytes_total")   // UnderscoreEscapingWithSuffixes + unit "By"
		require.Contains(t, names, "http_server_requests")               // UnderscoreEscapingWithoutSuffixes
		require.Contains(t, names, "http.server.requests_total")         // NoUTF8EscapingWithSuffixes + no unit
	})
}

func collectMetricNames(variants [][]*labels.Matcher) []string {
	names := make([]string, 0, len(variants))
	for _, v := range variants {
		for _, m := range v {
			if m.Name == model.MetricNameLabel {
				names = append(names, m.Value)
			}
		}
	}
	return names
}
