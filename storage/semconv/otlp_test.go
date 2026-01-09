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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestGenerateOTLPVariants(t *testing.T) {
	t.Run("generates variants for each strategy", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "http.server.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "http.method", "GET"),
		}
		meta := &metricMeta{Unit: "s", Type: model.MetricTypeHistogram}

		variants, err := generateOTLPVariants(matchers, meta)
		require.NoError(t, err)

		// Should have identity + 4 strategy variants.
		require.Len(t, variants, 5)

		// First variant is identity (original matchers).
		require.Equal(t, matchers, variants[0])

		// Extract metric names from all variants.
		names := make([]string, 0, len(variants))
		for _, v := range variants {
			for _, m := range v {
				if m.Name == labels.MetricName {
					names = append(names, m.Value)
				}
			}
		}

		// Check expected translations based on strategies.
		require.Contains(t, names, "http.server.duration")            // Identity
		require.Contains(t, names, "http_server_duration_seconds")    // UnderscoreEscaping + Suffixes
		require.Contains(t, names, "http_server_duration")            // UnderscoreEscaping, no Suffixes
		require.Contains(t, names, "http.server.duration_seconds")    // NoUTF8Escaping + Suffixes
	})

	t.Run("translates label names", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my.metric"),
			labels.MustNewMatcher(labels.MatchEqual, "http.request.method", "POST"),
		}

		variants, err := generateOTLPVariants(matchers, nil)
		require.NoError(t, err)

		// Collect all label names from variants.
		labelNames := make(map[string]bool)
		for _, v := range variants {
			for _, m := range v {
				if m.Name != labels.MetricName {
					labelNames[m.Name] = true
				}
			}
		}

		// Should have original and underscore-escaped version.
		require.True(t, labelNames["http.request.method"])
		require.True(t, labelNames["http_request_method"])
	})

	t.Run("handles nil metadata", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "custom.metric"),
		}

		variants, err := generateOTLPVariants(matchers, nil)
		require.NoError(t, err)
		require.Len(t, variants, 5)
	})

	t.Run("skips semconv and schema labels in translated variants", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my.metric"),
			labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "http://example.com/semconv"),
			labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "http://example.com/schema"),
			labels.MustNewMatcher(labels.MatchEqual, "my.label", "value"),
		}

		variants, err := generateOTLPVariants(matchers, nil)
		require.NoError(t, err)

		// First variant is identity (keeps all matchers).
		require.Equal(t, matchers, variants[0])

		// Translated variants (index 1+) should skip URL labels.
		for i, v := range variants {
			if i == 0 {
				continue // Skip identity variant.
			}
			require.Len(t, v, 2, "variant %d should only have metric name and my.label", i)
			for _, m := range v {
				require.NotEqual(t, semconvURLLabel, m.Name)
				require.NotEqual(t, schemaURLLabel, m.Name)
			}
		}
	})
}

func TestTranslateMetricName(t *testing.T) {
	tests := []struct {
		name     string
		metric   string
		meta     *metricMeta
		strategy otlptranslator.TranslationStrategyOption
		expected string
	}{
		{
			name:     "histogram with seconds unit - underscore escaping with suffixes",
			metric:   "http.server.duration",
			meta:     &metricMeta{Unit: "s", Type: model.MetricTypeHistogram},
			strategy: otlptranslator.UnderscoreEscapingWithSuffixes,
			expected: "http_server_duration_seconds",
		},
		{
			name:     "histogram with seconds unit - underscore escaping without suffixes",
			metric:   "http.server.duration",
			meta:     &metricMeta{Unit: "s", Type: model.MetricTypeHistogram},
			strategy: otlptranslator.UnderscoreEscapingWithoutSuffixes,
			expected: "http_server_duration",
		},
		{
			name:     "histogram with seconds unit - no UTF8 escaping with suffixes",
			metric:   "http.server.duration",
			meta:     &metricMeta{Unit: "s", Type: model.MetricTypeHistogram},
			strategy: otlptranslator.NoUTF8EscapingWithSuffixes,
			expected: "http.server.duration_seconds",
		},
		{
			name:     "counter - adds total suffix",
			metric:   "http.server.requests",
			meta:     &metricMeta{Unit: "", Type: model.MetricTypeCounter},
			strategy: otlptranslator.UnderscoreEscapingWithSuffixes,
			expected: "http_server_requests_total",
		},
		{
			name:     "gauge - no suffix",
			metric:   "process.cpu.utilization",
			meta:     &metricMeta{Unit: "1", Type: model.MetricTypeGauge},
			strategy: otlptranslator.UnderscoreEscapingWithSuffixes,
			expected: "process_cpu_utilization_ratio",
		},
		{
			name:     "nil metadata",
			metric:   "custom.metric",
			meta:     nil,
			strategy: otlptranslator.UnderscoreEscapingWithSuffixes,
			expected: "custom_metric",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := translateMetricName(tc.metric, tc.meta, tc.strategy)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestTranslateLabelName(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		strategy otlptranslator.TranslationStrategyOption
		expected string
	}{
		{
			name:     "dot-separated - underscore escaping",
			label:    "http.request.method",
			strategy: otlptranslator.UnderscoreEscapingWithSuffixes,
			expected: "http_request_method",
		},
		{
			name:     "dot-separated - no escaping",
			label:    "http.request.method",
			strategy: otlptranslator.NoUTF8EscapingWithSuffixes,
			expected: "http.request.method",
		},
		{
			name:     "already valid prometheus name",
			label:    "instance",
			strategy: otlptranslator.UnderscoreEscapingWithSuffixes,
			expected: "instance",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := translateLabelName(tc.label, tc.strategy)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestPromTypeToOTelType(t *testing.T) {
	tests := []struct {
		promType model.MetricType
		otelType otlptranslator.MetricType
	}{
		{model.MetricTypeCounter, otlptranslator.MetricTypeMonotonicCounter},
		{model.MetricTypeGauge, otlptranslator.MetricTypeGauge},
		{model.MetricTypeHistogram, otlptranslator.MetricTypeHistogram},
		{model.MetricTypeSummary, otlptranslator.MetricTypeSummary},
		{model.MetricTypeUnknown, otlptranslator.MetricTypeUnknown},
	}

	for _, tc := range tests {
		t.Run(string(tc.promType), func(t *testing.T) {
			result := promTypeToOTelType(tc.promType)
			require.Equal(t, tc.otelType, result)
		})
	}
}

func TestBuildLabelMapping(t *testing.T) {
	t.Run("maps translated labels back to original", func(t *testing.T) {
		attributes := []string{"http.method", "http.status_code"}

		mapping := buildLabelMapping("http.server.duration", attributes)

		require.Equal(t, "http.server.duration", mapping.translatedMetric)

		// Check that underscore-escaped versions map back to original.
		require.Equal(t, "http.method", mapping.translatedLabels["http_method"])
		require.Equal(t, "http.status_code", mapping.translatedLabels["http_status_code"])

		// Original names should also be in the mapping (from NoUTF8Escaping strategy).
		require.Equal(t, "http.method", mapping.translatedLabels["http.method"])
	})

	t.Run("handles empty attributes", func(t *testing.T) {
		mapping := buildLabelMapping("my.metric", nil)

		require.Equal(t, "my.metric", mapping.translatedMetric)
		require.Empty(t, mapping.translatedLabels)
	})
}

func TestGenerateCombinedVariants(t *testing.T) {
	t.Run("cross-product of schema and OTLP variants", func(t *testing.T) {
		// Two schema variants (original and renamed).
		schemaVariants := [][]*labels.Matcher{
			{labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.v2")},
			{labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.v1")},
		}

		combined, err := generateCombinedVariants(schemaVariants, nil)
		require.NoError(t, err)

		// 2 schema variants × 5 OTLP variants (identity + 4 strategies) = 10 variants.
		// Some may be deduplicated if they produce the same result.
		require.GreaterOrEqual(t, len(combined), 2)
		require.LessOrEqual(t, len(combined), 10)

		// Verify both schema variants have OTLP translations.
		names := extractMetricNames(combined)
		require.Contains(t, names, "metric.v2")
		require.Contains(t, names, "metric.v1")
		require.Contains(t, names, "metric_v2")
		require.Contains(t, names, "metric_v1")
	})

	t.Run("deduplicates identical variants", func(t *testing.T) {
		// Same matchers twice - should deduplicate.
		schemaVariants := [][]*labels.Matcher{
			{labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my.metric")},
			{labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my.metric")},
		}

		combined, err := generateCombinedVariants(schemaVariants, nil)
		require.NoError(t, err)

		// For "my.metric":
		// - Identity: my.metric
		// - UnderscoreEscaping strategies: my_metric (both produce same)
		// - NoUTF8Escaping: my.metric (same as identity)
		// - NoTranslation: my.metric (same as identity)
		// So we get 2 unique variants, not 5 or 10.
		require.Len(t, combined, 2)

		names := extractMetricNames(combined)
		require.Contains(t, names, "my.metric")
		require.Contains(t, names, "my_metric")
	})
}
