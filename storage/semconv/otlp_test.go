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

	// Create labels as if returned from storage.
	lbls := labels.FromStrings(
		model.MetricNameLabel, "http_server_duration_seconds",
		schemaURLLabel, "https://prometheus.io/schema/otlp/untranslated",
		"http_method", "GET",
	)

	qCtx := queryContext{changes: []change{{}}} // Sentinel

	result, vt, err := e.TransformSeries(qCtx, lbls)
	require.NoError(t, err)

	// __schema_url__ should be removed.
	require.Empty(t, result.Get(schemaURLLabel))

	// Other labels should remain.
	require.Equal(t, "http_server_duration_seconds", result.Get(model.MetricNameLabel))
	require.Equal(t, "GET", result.Get("http_method"))

	// No value transformation for OTLP schema.
	require.Empty(t, vt.expr)
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
