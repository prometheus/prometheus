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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestFindMatcherVariants_RequiresSemconvURL(t *testing.T) {
	e := newSchemaEngine()

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "http.server.duration"),
	}

	_, _, err := e.FindMatcherVariants("", "", matchers)
	require.Error(t, err)
	require.Contains(t, err.Error(), "semconvURL is required")
}

func TestGenerateMatcherVariants(t *testing.T) {
	t.Run("applies per-version renames", func(t *testing.T) {
		schema := &otelSchema{
			versionRenames: []versionRenames{
				{
					version:    "1.0.0",
					metrics:    map[string]string{"metric.v2": "metric.v1", "metric.v1": "metric.v2"},
					attributes: map[string]string{},
				},
			},
		}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.v2"),
		}

		result := generateMatcherVariants("1.0.0", schema, matchers)

		// Should have original + 1 version variant.
		require.Len(t, result, 2)
		metricNames := extractMetricNames(result)
		require.Contains(t, metricNames, "metric.v2")
		require.Contains(t, metricNames, "metric.v1")
	})

	t.Run("applies metric and attribute renames together", func(t *testing.T) {
		schema := &otelSchema{
			versionRenames: []versionRenames{
				{
					version:    "1.0.0",
					metrics:    map[string]string{"metric.new": "metric.old", "metric.old": "metric.new"},
					attributes: map[string]string{"attr.new": "attr.old", "attr.old": "attr.new"},
				},
			},
		}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.new"),
			labels.MustNewMatcher(labels.MatchEqual, "attr.new", "value"),
		}

		result := generateMatcherVariants("1.0.0", schema, matchers)

		// Should have original + 1 version variant (both metric AND attr renamed together).
		require.Len(t, result, 2)

		// Verify the combinations.
		found := make(map[string]bool)
		for _, m := range result {
			metricName := ""
			attrName := ""
			for _, matcher := range m {
				if matcher.Name == labels.MetricName {
					metricName = matcher.Value
				} else {
					attrName = matcher.Name
				}
			}
			found[metricName+"/"+attrName] = true
		}
		// Original: metric.new/attr.new
		// Version rename: metric.old/attr.old (BOTH renamed together)
		require.True(t, found["metric.new/attr.new"])
		require.True(t, found["metric.old/attr.old"])
		// Should NOT have mix-and-match combinations.
		require.False(t, found["metric.new/attr.old"])
		require.False(t, found["metric.old/attr.new"])
	})

	t.Run("resolves transitive rename chains", func(t *testing.T) {
		// Versions are sorted: 1.0.0 (v1↔v2), then 1.1.0 (v2↔v3).
		schema := &otelSchema{
			versionRenames: []versionRenames{
				{
					version:    "1.0.0",
					metrics:    map[string]string{"metric.v2": "metric.v1", "metric.v1": "metric.v2"},
					attributes: map[string]string{},
				},
				{
					version:    "1.1.0",
					metrics:    map[string]string{"metric.v3": "metric.v2", "metric.v2": "metric.v3"},
					attributes: map[string]string{},
				},
			},
		}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.v3"),
		}

		result := generateMatcherVariants("1.1.0", schema, matchers)

		// Anchored at 1.1.0, backward walk resolves: v3 → v2 (via 1.1.0) → v1 (via 1.0.0).
		require.Len(t, result, 3)
		metricNames := extractMetricNames(result)
		require.Contains(t, metricNames, "metric.v3")
		require.Contains(t, metricNames, "metric.v2")
		require.Contains(t, metricNames, "metric.v1")
	})

	t.Run("resolves transitive chains with paired attributes", func(t *testing.T) {
		schema := &otelSchema{
			versionRenames: []versionRenames{
				{
					version:    "1.0.0",
					metrics:    map[string]string{"metric.v2": "metric.v1", "metric.v1": "metric.v2"},
					attributes: map[string]string{"attr.v2": "attr.v1", "attr.v1": "attr.v2"},
				},
				{
					version:    "1.1.0",
					metrics:    map[string]string{"metric.v3": "metric.v2", "metric.v2": "metric.v3"},
					attributes: map[string]string{"attr.v3": "attr.v2", "attr.v2": "attr.v3"},
				},
			},
		}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.v3"),
			labels.MustNewMatcher(labels.MatchEqual, "attr.v3", "value"),
		}

		result := generateMatcherVariants("1.1.0", schema, matchers)

		// Anchored at 1.1.0, should have 3 variants with metric+attr paired correctly.
		require.Len(t, result, 3)

		found := make(map[string]bool)
		for _, m := range result {
			metricName := ""
			attrName := ""
			for _, matcher := range m {
				if matcher.Name == labels.MetricName {
					metricName = matcher.Value
				} else {
					attrName = matcher.Name
				}
			}
			found[metricName+"/"+attrName] = true
		}

		// Each version's renames are applied together via backward walk.
		require.True(t, found["metric.v3/attr.v3"]) // Original
		require.True(t, found["metric.v2/attr.v2"]) // v1.1.0 renames applied
		require.True(t, found["metric.v1/attr.v1"]) // v1.0.0 renames applied to v2

		// Should NOT have cross-version combinations.
		require.False(t, found["metric.v3/attr.v2"])
		require.False(t, found["metric.v3/attr.v1"])
		require.False(t, found["metric.v2/attr.v3"])
		require.False(t, found["metric.v2/attr.v1"])
		require.False(t, found["metric.v1/attr.v3"])
		require.False(t, found["metric.v1/attr.v2"])
	})

	t.Run("no renames returns original only", func(t *testing.T) {
		schema := &otelSchema{}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "my.metric"),
		}

		result := generateMatcherVariants("1.0.0", schema, matchers)

		require.Len(t, result, 1)
		require.Equal(t, matchers, result[0])
	})
}

func extractMetricNames(matcherSets [][]*labels.Matcher) []string {
	names := make([]string, 0, len(matcherSets))
	for _, set := range matcherSets {
		for _, m := range set {
			if m.Name == labels.MetricName {
				names = append(names, m.Value)
			}
		}
	}
	return names
}

func TestFindVersionAnchorIndex(t *testing.T) {
	versions := []versionRenames{
		{version: "1.0.0"},
		{version: "1.1.0"},
		{version: "1.2.0"},
	}

	tests := []struct {
		name          string
		targetVersion string
		expectedIndex int
	}{
		{"exact match first", "1.0.0", 0},
		{"exact match middle", "1.1.0", 1},
		{"exact match last", "1.2.0", 2},
		{"between versions", "1.0.5", 0},
		{"before all versions", "0.9.0", 0},
		{"after all versions", "2.0.0", 2},
		{"with v prefix", "v1.1.0", 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := findVersionAnchorIndex(versions, tc.targetVersion)
			require.Equal(t, tc.expectedIndex, idx)
		})
	}

	t.Run("empty versions", func(t *testing.T) {
		idx := findVersionAnchorIndex([]versionRenames{}, "1.0.0")
		require.Equal(t, 0, idx)
	})
}

func TestGenerateMatcherVariants_AnchoredTraversal(t *testing.T) {
	t.Run("anchored at middle version walks both directions", func(t *testing.T) {
		// Schema: [1.0.0: v1<->v2], [1.1.0: v2<->v3], [1.2.0: v3<->v4]
		schema := &otelSchema{
			versionRenames: []versionRenames{
				{version: "1.0.0", metrics: map[string]string{"metric.v2": "metric.v1", "metric.v1": "metric.v2"}},
				{version: "1.1.0", metrics: map[string]string{"metric.v3": "metric.v2", "metric.v2": "metric.v3"}},
				{version: "1.2.0", metrics: map[string]string{"metric.v4": "metric.v3", "metric.v3": "metric.v4"}},
			},
		}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.v3"),
		}

		// Anchored at 1.1.0: backward walks [1.0.0, 1.1.0], forward walks [1.1.0, 1.2.0].
		result := generateMatcherVariants("1.1.0", schema, matchers)

		require.Len(t, result, 4) // v1, v2, v3, v4
		names := extractMetricNames(result)
		require.Contains(t, names, "metric.v1")
		require.Contains(t, names, "metric.v2")
		require.Contains(t, names, "metric.v3")
		require.Contains(t, names, "metric.v4")
	})

	t.Run("anchored at first version only walks forward for newer", func(t *testing.T) {
		schema := &otelSchema{
			versionRenames: []versionRenames{
				{version: "1.0.0", metrics: map[string]string{"metric.v2": "metric.v1", "metric.v1": "metric.v2"}},
				{version: "1.1.0", metrics: map[string]string{"metric.v3": "metric.v2", "metric.v2": "metric.v3"}},
			},
		}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.v2"),
		}

		// Anchored at 1.0.0: backward walks [1.0.0], forward walks [1.0.0, 1.1.0].
		result := generateMatcherVariants("1.0.0", schema, matchers)

		names := extractMetricNames(result)
		require.Contains(t, names, "metric.v1") // backward from anchor
		require.Contains(t, names, "metric.v2") // original
		require.Contains(t, names, "metric.v3") // forward from anchor
	})

	t.Run("handles version with v prefix", func(t *testing.T) {
		schema := &otelSchema{
			versionRenames: []versionRenames{
				{version: "1.0.0", metrics: map[string]string{"metric.old": "metric.new", "metric.new": "metric.old"}},
			},
		}
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.new"),
		}

		result := generateMatcherVariants("v1.0.0", schema, matchers)

		require.Len(t, result, 2)
		names := extractMetricNames(result)
		require.Contains(t, names, "metric.new")
		require.Contains(t, names, "metric.old")
	})
}
