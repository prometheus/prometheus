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

		variants := generateOTLPVariants(matchers, meta)

		// Identity + 3 strategy variants.
		require.Len(t, variants, 4)

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
		require.Contains(t, names, "http.server.duration")         // Identity
		require.Contains(t, names, "http_server_duration_seconds") // UnderscoreEscaping + Suffixes
		require.Contains(t, names, "http_server_duration")         // UnderscoreEscaping, no Suffixes
		require.Contains(t, names, "http.server.duration_seconds") // NoUTF8Escaping + Suffixes
	})

	t.Run("translates label names", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my.metric"),
			labels.MustNewMatcher(labels.MatchEqual, "http.request.method", "POST"),
		}

		variants := generateOTLPVariants(matchers, nil)

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

		variants := generateOTLPVariants(matchers, nil)
		require.Len(t, variants, 4)
	})

	t.Run("skips semconv and schema labels in translated variants", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my.metric"),
			labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "http://example.com/semconv"),
			labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "http://example.com/schema"),
			labels.MustNewMatcher(labels.MatchEqual, "my.label", "value"),
		}

		variants := generateOTLPVariants(matchers, nil)

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

// generateCombinedVariants used to expose the schema × OTLP cross-product as
// a standalone helper. It is now inlined in findMatcherVariants so the dedup
// can use per-schema-variant metricMeta; its behaviour is exercised
// end-to-end via the AwareStorage tests in storage_test.go and
// otlp_storage_test.go.

func TestParseOTLPStrategy(t *testing.T) {
	for _, name := range []string{
		"UnderscoreEscapingWithSuffixes",
		"UnderscoreEscapingWithoutSuffixes",
		"NoUTF8EscapingWithSuffixes",
		"NoTranslation",
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := parseOTLPStrategy(name)
			require.True(t, ok)
		})
	}

	t.Run("rejects an unknown name", func(t *testing.T) {
		_, ok := parseOTLPStrategy("Bogus")
		require.False(t, ok)
	})
}

func TestResolveCanonicalMetric(t *testing.T) {
	sc := semconv{
		metricMetadata: map[string]metricMeta{
			"test": {Unit: "By", Type: model.MetricTypeCounter},
		},
		attributesPerMetric: map[string][]string{
			"test": {"http.response.status_code"},
		},
	}

	t.Run("canonical name under NoTranslation", func(t *testing.T) {
		got, ok := resolveCanonicalMetric(sc, "test", otlptranslator.NoTranslation)
		require.True(t, ok)
		require.Equal(t, "test", got)
	})

	t.Run("rejects a canonical name under a mismatched strategy", func(t *testing.T) {
		// "test" is not the UnderscoreEscapingWithSuffixes rendering of any metric
		// (that is "test_bytes_total"), so it must not resolve.
		_, ok := resolveCanonicalMetric(sc, "test", otlptranslator.UnderscoreEscapingWithSuffixes)
		require.False(t, ok)
	})

	t.Run("reverse-resolves a translated name", func(t *testing.T) {
		got, ok := resolveCanonicalMetric(sc, "test_bytes_total", otlptranslator.UnderscoreEscapingWithSuffixes)
		require.True(t, ok)
		require.Equal(t, "test", got)
	})

	t.Run("no match", func(t *testing.T) {
		_, ok := resolveCanonicalMetric(sc, "unrelated", otlptranslator.UnderscoreEscapingWithSuffixes)
		require.False(t, ok)
	})
}

func TestCanonicalizeMatchers(t *testing.T) {
	sc := semconv{
		metricMetadata:      map[string]metricMeta{"test": {Unit: "By", Type: model.MetricTypeCounter}},
		attributesPerMetric: map[string][]string{"test": {"http.response.status_code"}},
	}
	in := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test_bytes_total"),
		labels.MustNewMatcher(labels.MatchEqual, "http_response_status_code", "200"),
	}

	out := canonicalizeMatchers(sc, "test", otlptranslator.UnderscoreEscapingWithSuffixes, in)

	got := make(map[string]string, len(out))
	for _, m := range out {
		got[m.Name] = m.Value
	}
	require.Equal(t, "test", got[labels.MetricName])
	require.Equal(t, "200", got["http.response.status_code"])
	require.NotContains(t, got, "http_response_status_code")
}

func TestDialectMismatchWarning(t *testing.T) {
	sc := semconv{
		metricMetadata:      map[string]metricMeta{"test": {Unit: "By", Type: model.MetricTypeCounter}},
		attributesPerMetric: map[string][]string{"test": {"http.response.status_code"}},
	}
	warn := func(label, value string, strategy otlptranslator.TranslationStrategyOption) string {
		return dialectMismatchWarning(sc, "test", strategy, []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test"),
			labels.MustNewMatcher(labels.MatchEqual, label, value),
		})
	}

	t.Run("flags an attribute in the wrong escaping", func(t *testing.T) {
		// Escaped label under the native (NoTranslation) dialect.
		require.NotEmpty(t, warn("http_response_status_code", "200", otlptranslator.NoTranslation))
	})

	t.Run("no warning for an in-dialect label", func(t *testing.T) {
		require.Empty(t, warn("http.response.status_code", "200", otlptranslator.NoTranslation))
		require.Empty(t, warn("http_response_status_code", "200", otlptranslator.UnderscoreEscapingWithSuffixes))
	})

	t.Run("no warning for a non-attribute label", func(t *testing.T) {
		require.Empty(t, warn("instance", "a", otlptranslator.NoTranslation))
	})
}
