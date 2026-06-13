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
	"sort"
	"strings"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/infohelper"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
)

// substringFilter is a minimal storage.Filter used to exercise the filter path
// without pulling in web/api/v1's search filter machinery. Score is 1.0 for
// exact match, 0.9 for prefix, 0.5 otherwise.
type substringFilter struct{ needle string }

func (f substringFilter) Accept(value string) (bool, float64) {
	lower := strings.ToLower(value)
	if !strings.Contains(lower, f.needle) {
		return false, 0
	}
	switch {
	case lower == f.needle:
		return true, 1.0
	case strings.HasPrefix(lower, f.needle):
		return true, 0.9
	default:
		return true, 0.5
	}
}

// sortByName sorts records in-place by Name. ExtractDataLabels does not promise
// an order, so tests assert on a name-sorted slice.
func sortByName(rs []infohelper.InfoLabelRecord) []infohelper.InfoLabelRecord {
	sort.Slice(rs, func(i, j int) bool { return rs[i].Name < rs[j].Name })
	return rs
}

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
		filter                 storage.Filter
		namesLimit             int
		valuesLimit            int
		expected               []infohelper.InfoLabelRecord
		expectTruncationWarn   bool
	}{
		{
			name:              "all target_info labels without filter",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			expected: []infohelper.InfoLabelRecord{
				{Name: "env", Values: []string{"prod", "staging"}, Score: 1.0},
				{Name: "region", Values: []string{"us-east"}, Score: 1.0},
				{Name: "version", Values: []string{"1.0", "2.0", "2.1"}, Score: 1.0},
			},
		},
		{
			name:              "filter by job=prometheus",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"job": {"prometheus": {}},
			},
			expected: []infohelper.InfoLabelRecord{
				{Name: "env", Values: []string{"prod", "staging"}, Score: 1.0},
				{Name: "version", Values: []string{"2.0", "2.1"}, Score: 1.0},
			},
		},
		{
			name:              "filter by specific instance",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"instance": {"localhost:9090": {}},
			},
			expected: []infohelper.InfoLabelRecord{
				{Name: "env", Values: []string{"prod"}, Score: 1.0},
				{Name: "version", Values: []string{"2.0"}, Score: 1.0},
			},
		},
		{
			name:              "filter with multiple identifying values",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"job":      {"prometheus": {}, "node": {}},
				"instance": {"localhost:9090": {}, "node1:9100": {}},
			},
			expected: []infohelper.InfoLabelRecord{
				{Name: "env", Values: []string{"prod"}, Score: 1.0},
				{Name: "region", Values: []string{"us-east"}, Score: 1.0},
				{Name: "version", Values: []string{"1.0", "2.0"}, Score: 1.0},
			},
		},
		{
			name:              "custom info metric",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "custom_info"),
			expected: []infohelper.InfoLabelRecord{
				{Name: "custom_label", Values: []string{"custom_value"}, Score: 1.0},
			},
		},
		{
			name:              "non-existent info metric",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "nonexistent_info"),
			expected:          []infohelper.InfoLabelRecord{},
		},
		{
			name:              "no matching identifying labels",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			identifyingLabelValues: map[string]map[string]struct{}{
				"job": {"nonexistent": {}},
			},
			expected: []infohelper.InfoLabelRecord{},
		},
		{
			name:              "regex match on info metric name",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".*_info"),
			expected: []infohelper.InfoLabelRecord{
				{Name: "custom_label", Values: []string{"custom_value"}, Score: 1.0},
				{Name: "env", Values: []string{"prod", "staging"}, Score: 1.0},
				{Name: "region", Values: []string{"us-east"}, Score: 1.0},
				{Name: "version", Values: []string{"1.0", "2.0", "2.1"}, Score: 1.0},
			},
		},
		{
			name:              "filter narrows label names and carries score",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			filter:            substringFilter{needle: "ver"},
			expected: []infohelper.InfoLabelRecord{
				{Name: "version", Values: []string{"1.0", "2.0", "2.1"}, Score: 0.9},
			},
		},
		{
			name:              "filter exact match scores 1.0",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			filter:            substringFilter{needle: "env"},
			expected: []infohelper.InfoLabelRecord{
				{Name: "env", Values: []string{"prod", "staging"}, Score: 1.0},
			},
		},
		{
			name:              "filter substring match scores 0.5",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			filter:            substringFilter{needle: "ion"},
			expected: []infohelper.InfoLabelRecord{
				{Name: "region", Values: []string{"us-east"}, Score: 0.5},
				{Name: "version", Values: []string{"1.0", "2.0", "2.1"}, Score: 0.5},
			},
		},
		{
			name:              "filter rejects every name",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			filter:            substringFilter{needle: "zzz"},
			expected:          []infohelper.InfoLabelRecord{},
		},
		{
			name:              "values_limit truncates after alpha sort",
			infoMetricMatcher: labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			valuesLimit:       2,
			expected: []infohelper.InfoLabelRecord{
				{Name: "env", Values: []string{"prod", "staging"}, Score: 1.0},
				{Name: "region", Values: []string{"us-east"}, Score: 1.0},
				{Name: "version", Values: []string{"1.0", "2.0"}, Score: 1.0},
			},
		},
		{
			// names_limit=2 caps the distinct names collected. We can't
			// assert which two names were chosen (the storage iteration
			// order is implementation-defined), only that the count is
			// right, every returned name has full values, and a
			// truncation warning is surfaced.
			name:                 "names_limit caps distinct names and warns",
			infoMetricMatcher:    labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
			namesLimit:           2,
			expectTruncationWarn: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			extractor := infohelper.NewWithDefaults()

			q, err := testStorage.Querier(0, 100000)
			require.NoError(t, err)
			defer q.Close()

			hints := &storage.SelectHints{Start: 0, End: 100000, Func: "info_labels"}

			records, warnings, err := extractor.ExtractDataLabels(ctx, q, tc.infoMetricMatcher, tc.identifyingLabelValues, hints, tc.filter, tc.namesLimit, tc.valuesLimit)
			require.NoError(t, err)

			if tc.namesLimit > 0 {
				require.LessOrEqual(t, len(records), tc.namesLimit)
			}
			if tc.expectTruncationWarn {
				warningTexts := make([]string, 0, len(warnings))
				for _, w := range warnings.AsErrors() {
					warningTexts = append(warningTexts, w.Error())
				}
				found := false
				for _, w := range warningTexts {
					if strings.Contains(w, "info-labels names truncated") {
						found = true
						break
					}
				}
				require.True(t, found, "expected truncation warning, got: %v", warningTexts)
			}
			if tc.expected != nil {
				require.Equal(t, tc.expected, sortByName(records))
			}
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
		{name: "empty", values: map[string]struct{}{}},
		{name: "single value", values: map[string]struct{}{"foo": {}}},
		{name: "multiple values", values: map[string]struct{}{"foo": {}, "bar": {}, "baz": {}}},
		{name: "values with regex metacharacters", values: map[string]struct{}{"a.b": {}, "c*d": {}, "e+f": {}}},
		{name: "values with colons (like host:port)", values: map[string]struct{}{"localhost:9090": {}, "localhost:9091": {}}},
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
