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

package infohelper_test

import (
	"context"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/infohelper"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
)

func TestExtractDataLabels(t *testing.T) {
	testStorage := promqltest.LoadedStorage(t, `
		load 1m
			target_info{job="prometheus", instance="localhost:9090", version="2.0", env="prod"} 1
			target_info{job="prometheus", instance="localhost:9091", version="2.1", env="staging"} 1
			target_info{job="node", instance="node1:9100", version="1.0", region="us-east"} 1
			http_requests_total{job="prometheus", instance="localhost:9090"} 100
			http_requests_total{job="prometheus", instance="localhost:9091"} 200
			http_requests_total{job="node", instance="node1:9100"} 50
			custom_info{job="app", instance="app1:8080", custom_label="custom_value"} 1
	`)
	t.Cleanup(func() { testStorage.Close() })

	tests := []struct {
		name                   string
		infoMetricMatcher      *labels.Matcher
		identifyingLabelValues map[string]map[string]struct{}
		search                 string
		expected               infohelper.InfoLabelsResult
	}{
		{
			name:              "all target_info labels without filter",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"version": {"1.0", "2.0", "2.1"},
					"env":     {"prod", "staging"},
					"region":  {"us-east"},
				},
				LabelOrder: []string{"env", "region", "version"},
			},
		},
		{
			name:              "filter by job=prometheus",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"job": {"prometheus": {}},
			},
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"version": {"2.0", "2.1"},
					"env":     {"prod", "staging"},
				},
				LabelOrder: []string{"env", "version"},
			},
		},
		{
			name:              "filter by specific instance",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"instance": {"localhost:9090": {}},
			},
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"version": {"2.0"},
					"env":     {"prod"},
				},
				LabelOrder: []string{"env", "version"},
			},
		},
		{
			name:              "filter with multiple identifying values",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"job":      {"prometheus": {}, "node": {}},
				"instance": {"localhost:9090": {}, "node1:9100": {}},
			},
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"version": {"1.0", "2.0"},
					"env":     {"prod"},
					"region":  {"us-east"},
				},
				LabelOrder: []string{"env", "region", "version"},
			},
		},
		{
			name:              "custom info metric",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "custom_info"),
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"custom_label": {"custom_value"},
				},
				LabelOrder: []string{"custom_label"},
			},
		},
		{
			name:              "non-existent info metric",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "nonexistent_info"),
			expected: infohelper.InfoLabelsResult{
				Labels:     map[string][]string{},
				LabelOrder: []string{},
			},
		},
		{
			name:              "no matching identifying labels",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"job": {"nonexistent": {}},
			},
			expected: infohelper.InfoLabelsResult{
				Labels:     map[string][]string{},
				LabelOrder: []string{},
			},
		},
		{
			name:              "regex match on info metric name",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".*_info"),
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"version":      {"1.0", "2.0", "2.1"},
					"env":          {"prod", "staging"},
					"region":       {"us-east"},
					"custom_label": {"custom_value"},
				},
				LabelOrder: []string{"custom_label", "env", "region", "version"},
			},
		},
		{
			name:              "search filters label names",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			search:            "ver",
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"version": {"1.0", "2.0", "2.1"},
				},
				LabelOrder: []string{"version"},
			},
		},
		{
			name:              "search exact match sorts first",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			search:            "env",
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"env": {"prod", "staging"},
				},
				LabelOrder: []string{"env"},
			},
		},
		{
			name:              "search substring position ordering",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			search:            "ion",
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"version": {"1.0", "2.0", "2.1"},
					"region":  {"us-east"},
				},
				LabelOrder: []string{"region", "version"},
			},
		},
		{
			name:              "search no matches",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			search:            "zzz",
			expected: infohelper.InfoLabelsResult{
				Labels:     map[string][]string{},
				LabelOrder: []string{},
			},
		},
		{
			name:              "search is case-insensitive",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			search:            "ENV",
			expected: infohelper.InfoLabelsResult{
				Labels: map[string][]string{
					"env": {"prod", "staging"},
				},
				LabelOrder: []string{"env"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			extractor := infohelper.NewWithDefaults()

			q, err := testStorage.Querier(0, 100000)
			require.NoError(t, err)
			defer q.Close()

			hints := &storage.SelectHints{
				Start: 0,
				End:   100000,
				Func:  "info_labels",
			}

			result, _, err := extractor.ExtractDataLabels(ctx, q, tc.infoMetricMatcher, tc.identifyingLabelValues, hints, tc.search)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := infohelper.DefaultConfig()
	require.Equal(t, []string{"instance", "job"}, config.IdentifyingLabels)
	require.Equal(t, "target_info", config.DefaultInfoMetric)
}

func TestInfoLabelExtractorMethods(t *testing.T) {
	extractor := infohelper.NewWithDefaults()
	require.Equal(t, []string{"instance", "job"}, extractor.IdentifyingLabels())
	require.Equal(t, "target_info", extractor.DefaultInfoMetric())

	customConfig := infohelper.Config{
		IdentifyingLabels: []string{"custom_id"},
		DefaultInfoMetric: "custom_info",
	}
	customExtractor := infohelper.New(customConfig)
	require.Equal(t, []string{"custom_id"}, customExtractor.IdentifyingLabels())
	require.Equal(t, "custom_info", customExtractor.DefaultInfoMetric())
}

func TestBuildRegexpAlternation(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]struct{}
	}{
		{
			name:   "empty",
			values: map[string]struct{}{},
		},
		{
			name:   "single value",
			values: map[string]struct{}{"foo": {}},
		},
		{
			name:   "multiple values",
			values: map[string]struct{}{"foo": {}, "bar": {}, "baz": {}},
		},
		{
			name:   "values with regex metacharacters",
			values: map[string]struct{}{"a.b": {}, "c*d": {}, "e+f": {}},
		},
		{
			name:   "values with colons (like host:port)",
			values: map[string]struct{}{"localhost:9090": {}, "localhost:9091": {}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := infohelper.BuildRegexpAlternation(tc.values)

			if len(tc.values) > 0 {
				re := regexp.MustCompile("^(" + result + ")$")
				for v := range tc.values {
					require.True(t, re.MatchString(v))
				}
			} else {
				require.Empty(t, result)
			}
		})
	}
}
