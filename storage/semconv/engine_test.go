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
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func metricChange(renames map[string]string) schemaChange {
	return schemaChange{metricRenames: newDirectedRenames(renames)}
}

func attributeChange(renames map[string]string, metrics ...string) schemaChange {
	step := &attributeRenameStep{renames: newDirectedRenames(renames)}
	if len(metrics) > 0 {
		step.scopeSpecified = true
		step.applyToMetrics = make(map[string]struct{}, len(metrics))
		for _, metric := range metrics {
			step.applyToMetrics[metric] = struct{}{}
		}
	}
	return schemaChange{attributeRenames: step}
}

func testSchema(revisions ...schemaRevision) *otelSchema {
	return &otelSchema{revisions: revisions}
}

func testSchemaWithVersions(allVersions []string, revisions ...schemaRevision) *otelSchema {
	return &otelSchema{allVersions: allVersions, revisions: revisions}
}

func equalMatchers(metric string, attrs ...string) []*labels.Matcher {
	matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, metric)}
	for _, attr := range attrs {
		matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, attr, "value"))
	}
	return matchers
}

func requireMatcherVariants(t *testing.T, version string, schema *otelSchema, matchers []*labels.Matcher, canonicalAttrs []string, rv *renameValidator) []matcherVariant {
	t.Helper()
	variants, err := generateMatcherVariantsWithBudget(version, schema, matchers, canonicalAttrs, rv, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
	require.NoError(t, err)
	return variants
}

func requireVariantAccumulator(t *testing.T, anchorMetric string, canonicalAttrs []string, rv *renameValidator) *variantAccumulator {
	t.Helper()
	acc, err := newVariantAccumulatorWithBudget(anchorMetric, canonicalAttrs, rv, true, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
	require.NoError(t, err)
	return acc
}

func requireLineageStateKey(t *testing.T, state lineageState) string {
	t.Helper()
	key, err := lineageStateKeyWithBudget(state, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
	require.NoError(t, err)
	return key
}

func variantNames(variants []matcherVariant) []string {
	names := make([]string, 0, len(variants))
	for _, variant := range variants {
		name, err := extractMetricName(variant.matchers)
		if err == nil {
			names = append(names, name)
		}
	}
	return names
}

func variantMetricAndAttribute(variant matcherVariant) string {
	metric, attr := "", ""
	for _, matcher := range variant.matchers {
		if matcher.Name == labels.MetricName {
			metric = matcher.Value
		} else {
			attr = matcher.Name
		}
	}
	return metric + "/" + attr
}

func TestFindMatcherVariants_RequiresSemconvURL(t *testing.T) {
	e := newSchemaEngine(embeddedRegistry)
	_, _, err := e.findMatcherVariants("", "", equalMatchers("http.server.duration"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "semconvURL is required")
}

func TestFindMatcherVariants_RequiresMetricNameAnchorBeforeRegistryLookup(t *testing.T) {
	e := newSchemaEngine(embeddedRegistry)
	_, _, err := e.findMatcherVariants("not-a-registry-path", "also-invalid", []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "job", "api"),
	})
	require.ErrorIs(t, err, errMetricNameAnchor)
}

func TestRevisionPartition(t *testing.T) {
	revisions := []schemaRevision{{version: "1.0.0"}, {version: "1.1.0"}, {version: "1.2.0"}}
	tests := []struct {
		version string
		want    int
	}{
		{version: "0.9.0", want: 0},
		{version: "1.0.0", want: 1},
		{version: "1.0.5", want: 1},
		{version: "v1.1.0", want: 2},
		{version: "2.0.0", want: 3},
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			got, err := revisionPartitionWithBudget(revisions, tc.version, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	got, err := revisionPartitionWithBudget(nil, "1.0.0", newSchemaExpansionBudget(productionSchemaExpansionLimits()))
	require.NoError(t, err)
	require.Zero(t, got)
}

func TestNormalizeMetricMatchers(t *testing.T) {
	t.Run("normalizes compatible constraints around an equality", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, `metric\.(current|old)`),
			labels.MustNewMatcher(labels.MatchEqual, "attribute", "value"),
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.current"),
			labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, "metric.old"),
		}

		name, got, satisfiable, err := normalizeMetricMatchers(matchers)
		require.NoError(t, err)
		require.True(t, satisfiable)
		require.Equal(t, "metric.current", name)
		require.Len(t, got, 2)
		require.Equal(t, labels.MatchEqual, got[0].Type)
		require.Equal(t, labels.MetricName, got[0].Name)
		require.Equal(t, "metric.current", got[0].Value)
		require.Same(t, matchers[1], got[1])
	})

	t.Run("keeps contradictory constraints for direct evaluation", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.current"),
			labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, "metric.old"),
		}

		name, got, satisfiable, err := normalizeMetricMatchers(matchers)
		require.NoError(t, err)
		require.False(t, satisfiable)
		require.Equal(t, "metric.current", name)
		require.Same(t, matchers[0], got[0])
		require.Same(t, matchers[1], got[1])
	})

	t.Run("finds a non-empty equality independent of matcher order", func(t *testing.T) {
		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, ""),
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.current"),
		}

		name, got, satisfiable, err := normalizeMetricMatchers(matchers)
		require.NoError(t, err)
		require.False(t, satisfiable)
		require.Equal(t, "metric.current", name)
		require.Same(t, matchers[0], got[0])
		require.Same(t, matchers[1], got[1])
	})

	t.Run("requires a non-empty exact anchor", func(t *testing.T) {
		for _, matcher := range []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, "metric.*"),
			labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, "metric.old"),
			labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, ""),
		} {
			_, _, _, err := normalizeMetricMatchers([]*labels.Matcher{matcher})
			require.ErrorIs(t, err, errMetricNameAnchor)
			require.ErrorContains(t, err, "non-empty equality matcher")
		}
	})
}

func TestGenerateMatcherVariants(t *testing.T) {
	t.Run("walks chained revisions in both directions", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "1.1.0", changes: []schemaChange{
				metricChange(map[string]string{"metric.v1": "metric.v2"}),
				attributeChange(map[string]string{"attr.v1": "attr.v2"}),
			}},
			schemaRevision{version: "1.2.0", changes: []schemaChange{
				metricChange(map[string]string{"metric.v2": "metric.v3"}),
				attributeChange(map[string]string{"attr.v2": "attr.v3"}),
			}},
		)

		backward := requireMatcherVariants(t, "1.2.0", schema, equalMatchers("metric.v3", "attr.v3"), []string{"attr.v3"}, nil)
		require.Equal(t, []string{"metric.v3/attr.v3", "metric.v2/attr.v2", "metric.v1/attr.v1"}, []string{
			variantMetricAndAttribute(backward[0]),
			variantMetricAndAttribute(backward[1]),
			variantMetricAndAttribute(backward[2]),
		})

		forward := requireMatcherVariants(t, "1.0.0", schema, equalMatchers("metric.v1", "attr.v1"), []string{"attr.v1"}, nil)
		require.Equal(t, []string{"metric.v1/attr.v1", "metric.v2/attr.v2", "metric.v3/attr.v3"}, []string{
			variantMetricAndAttribute(forward[0]),
			variantMetricAndAttribute(forward[1]),
			variantMetricAndAttribute(forward[2]),
		})
	})

	t.Run("emits only revision boundaries", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				metricChange(map[string]string{"metric.a": "metric.b"}),
				attributeChange(map[string]string{"attr.a": "attr.b"}),
				metricChange(map[string]string{"metric.b": "metric.c"}),
				attributeChange(map[string]string{"attr.b": "attr.c"}),
			},
		})
		variants := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric.c", "attr.c"), []string{"attr.c"}, nil)
		require.Len(t, variants, 2)
		require.Equal(t, "metric.c/attr.c", variantMetricAndAttribute(variants[0]))
		require.Equal(t, "metric.a/attr.a", variantMetricAndAttribute(variants[1]))
	})

	t.Run("validates only the final name of an ordered revision", func(t *testing.T) {
		schema := testSchemaWithVersions([]string{"1.0.0", "1.1.0"}, schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				metricChange(map[string]string{"metric.a": "metric.intermediate"}),
				metricChange(map[string]string{"metric.intermediate": "metric.current"}),
			},
		})
		rv := &renameValidator{
			anchorName:    "metric.current",
			anchorVersion: "1.1.0",
			anchorDef:     metricDef{unit: "s", instrument: "histogram"},
			anchorKnown:   true,
			seen:          map[string]struct{}{},
			lookup: func(version, name string) (metricDef, metricLookupStatus) {
				if version == "1.0.0" && name == "metric.a" {
					return metricDef{unit: "s", instrument: "histogram"}, metricDeclared
				}
				return metricDef{}, metricUndeclared
			},
		}
		variants := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric.current"), nil, rv)
		require.Equal(t, []string{"metric.current", "metric.a"}, variantNames(variants))
		require.Empty(t, rv.warnings, "the intermediate name is not a semconv-version endpoint")
	})

	t.Run("branches many-to-one renames deterministically", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{metricChange(map[string]string{
				"metric.b": "metric.current",
				"metric.a": "metric.current",
			})},
		})
		variants := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric.current"), nil, nil)
		require.Equal(t, []string{"metric.current", "metric.a", "metric.b"}, variantNames(variants))
	})

	t.Run("preserves convergence across ordered metric changes", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				metricChange(map[string]string{"metric.a": "metric.current"}),
				metricChange(map[string]string{"metric.b": "metric.current"}),
			},
		})

		backward := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric.current"), nil, nil)
		require.ElementsMatch(t, []string{"metric.current", "metric.a", "metric.b"}, variantNames(backward))

		for _, predecessor := range []string{"metric.a", "metric.b"} {
			forward := requireMatcherVariants(t, "1.0.0", schema, equalMatchers(predecessor), nil, nil)
			require.ElementsMatch(t, []string{predecessor, "metric.current"}, variantNames(forward))
		}
	})

	t.Run("preserves convergence across ordered attribute changes", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				attributeChange(map[string]string{"attr.a": "attr.current"}),
				attributeChange(map[string]string{"attr.b": "attr.current"}),
			},
		})

		backward := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric", "attr.current"), []string{"attr.current"}, nil)
		require.ElementsMatch(t, []string{"metric/attr.current", "metric/attr.a", "metric/attr.b"}, []string{
			variantMetricAndAttribute(backward[0]),
			variantMetricAndAttribute(backward[1]),
			variantMetricAndAttribute(backward[2]),
		})
		for _, variant := range backward[1:] {
			require.Equal(t, map[string]string{
				"attr.a": "attr.current",
				"attr.b": "attr.current",
			}, variant.mapping.translatedLabels)
		}

		for _, predecessor := range []string{"attr.a", "attr.b"} {
			forward := requireMatcherVariants(t, "1.0.0", schema, equalMatchers("metric", predecessor), []string{predecessor}, nil)
			require.ElementsMatch(t, []string{"metric/" + predecessor, "metric/attr.current"}, []string{
				variantMetricAndAttribute(forward[0]),
				variantMetricAndAttribute(forward[1]),
			})
		}
	})

	t.Run("an anchor before every revision has no backward step", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.0.0",
			changes: []schemaChange{metricChange(map[string]string{"metric.old": "metric.new"})},
		})
		variants := requireMatcherVariants(t, "0.9.0", schema, equalMatchers("metric.old"), nil, nil)
		require.Equal(t, []string{"metric.old", "metric.new"}, variantNames(variants))
	})

	t.Run("no revisions returns the direct variant", func(t *testing.T) {
		variants := requireMatcherVariants(t, "1.0.0", &otelSchema{}, equalMatchers("metric"), nil, nil)
		require.Len(t, variants, 1)
		require.Equal(t, "metric", variantNames(variants)[0])
	})
}

func TestRenameValidatorLookupFailures(t *testing.T) {
	t.Run("deduplicates an unavailable semconv at one boundary", func(t *testing.T) {
		rv := &renameValidator{
			anchorKnown: true,
			seen:        map[string]struct{}{},
			lookup: func(string, string) (metricDef, metricLookupStatus) {
				return metricDef{}, metricFileMissing
			},
		}

		require.True(t, rv.allowRevision("1.1.0", "1.0.0", "metric.current", "metric.a"))
		require.True(t, rv.allowRevision("1.1.0", "1.0.0", "metric.current", "metric.b"))
		require.Len(t, rv.warnings, 1)
		require.Contains(t, rv.warnings[0], "version 1.0.0 is unavailable")
	})

	t.Run("reports an unavailable semconv before an unknown anchor", func(t *testing.T) {
		rv := &renameValidator{
			anchorName:    "metric.current",
			anchorVersion: "1.1.0",
			seen:          map[string]struct{}{},
			lookup: func(string, string) (metricDef, metricLookupStatus) {
				return metricDef{}, metricFileMissing
			},
		}

		require.True(t, rv.allowRevision("1.1.0", "1.0.0", "metric.current", "metric.old"))
		require.Len(t, rv.warnings, 1)
		require.Contains(t, rv.warnings[0], "version 1.0.0 is unavailable")
		require.NotContains(t, rv.warnings[0], "no other version")
	})

	t.Run("fails open for an unknown lookup status", func(t *testing.T) {
		rv := &renameValidator{
			anchorKnown: true,
			seen:        map[string]struct{}{},
			lookup: func(string, string) (metricDef, metricLookupStatus) {
				return metricDef{}, metricLookupStatus(255)
			},
		}

		require.True(t, rv.allowRevision("1.1.0", "1.0.0", "metric.current", "metric.old"))
		require.Len(t, rv.warnings, 1)
		require.Contains(t, rv.warnings[0], "unknown metric lookup status 255")
	})
}

func TestMetricLifecycleBoundaries(t *testing.T) {
	t.Run("stops a corroborated retired name from a later anchor", func(t *testing.T) {
		schema := testSchemaWithVersions([]string{"1.0.0", "1.1.0", "1.2.0"}, schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{metricChange(map[string]string{"metric.old": "metric.current"})},
		})
		rv := &renameValidator{anchorKnown: true, seen: map[string]struct{}{}}

		variants := requireMatcherVariants(t, "1.2.0", schema, equalMatchers("metric.old"), nil, rv)
		require.Equal(t, []string{"metric.old"}, variantNames(variants))
		require.Len(t, rv.warnings, 1)
		require.Contains(t, rv.warnings[0], "metric lifecycle boundary")
	})

	t.Run("stops a reused old-side name even without identity evidence", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "2.0.0",
			changes: []schemaChange{metricChange(map[string]string{"foo": "bar"})},
		})
		rv := &renameValidator{seen: map[string]struct{}{}}
		variants := requireMatcherVariants(t, "5.0.0", schema, equalMatchers("foo"), nil, rv)
		require.Equal(t, []string{"foo"}, variantNames(variants))
		require.Len(t, rv.warnings, 1)
		require.Contains(t, rv.warnings[0], "metric lifecycle boundary")
	})

	t.Run("rejects a name reused within one revision", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				metricChange(map[string]string{"metric.a": "metric.b"}),
				metricChange(map[string]string{"metric.c": "metric.a"}),
			},
		})

		rv := &renameValidator{anchorDeclared: true, anchorKnown: true, seen: map[string]struct{}{}}
		_, err := generateMatcherVariantsWithBudget("1.1.0", schema, equalMatchers("metric.a"), nil, rv, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
		require.ErrorIs(t, err, errAmbiguousSchemaRename)
		require.ErrorContains(t, err, `metric name "metric.a"`)
	})

	t.Run("rejects a later identity claiming a retired name", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "1.1.0", changes: []schemaChange{metricChange(map[string]string{"foo": "bar"})}},
			schemaRevision{version: "1.2.0", changes: []schemaChange{metricChange(map[string]string{"baz": "foo"})}},
		)

		for _, metric := range []string{"bar", "foo"} {
			t.Run(metric, func(t *testing.T) {
				_, err := generateMatcherVariantsWithBudget("1.2.0", schema, equalMatchers(metric), nil, nil, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
				require.ErrorIs(t, err, errAmbiguousSchemaRename)
				require.ErrorContains(t, err, `metric name "foo"`)
			})
		}
	})

	t.Run("later convergence does not erase earlier reuse", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "1.1.0", changes: []schemaChange{metricChange(map[string]string{"foo": "bar"})}},
			schemaRevision{version: "1.2.0", changes: []schemaChange{metricChange(map[string]string{"baz": "foo"})}},
			schemaRevision{version: "1.3.0", changes: []schemaChange{metricChange(map[string]string{"foo": "bar"})}},
		)

		_, err := generateMatcherVariantsWithBudget("1.3.0", schema, equalMatchers("bar"), nil, nil, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
		require.ErrorIs(t, err, errAmbiguousSchemaRename)
		require.ErrorContains(t, err, `metric name "foo"`)
	})

	t.Run("allows a repeated historical source to converge", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "1.1.0", changes: []schemaChange{metricChange(map[string]string{"metric.long": "metric.short"})}},
			schemaRevision{version: "1.2.0", changes: []schemaChange{metricChange(map[string]string{
				"metric.long":  "metric.current",
				"metric.short": "metric.current",
			})}},
		)

		variants := requireMatcherVariants(t, "1.2.0", schema, equalMatchers("metric.current"), nil, nil)
		require.ElementsMatch(t, []string{"metric.current", "metric.long", "metric.short"}, variantNames(variants))
	})

	t.Run("applies one rename map atomically", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{metricChange(map[string]string{"metric.a": "metric.b", "metric.b": "metric.a"})},
		})

		_, err := generateMatcherVariantsWithBudget("1.1.0", schema, equalMatchers("metric.a"), nil, nil, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
		require.ErrorIs(t, err, errAmbiguousSchemaRename)
	})

	t.Run("does not turn a transient name into a version era", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				metricChange(map[string]string{"metric.c": "metric.transient"}),
				metricChange(map[string]string{"metric.transient": "metric.d"}),
			},
		})
		rv := &renameValidator{anchorKnown: true, seen: map[string]struct{}{}}

		variants := requireMatcherVariants(t, "1.2.0", schema, equalMatchers("metric.transient"), nil, rv)
		require.Equal(t, []string{"metric.transient"}, variantNames(variants))
	})

	t.Run("allows a legitimate rename back", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "2.0.0", changes: []schemaChange{metricChange(map[string]string{"foo": "bar"})}},
			schemaRevision{version: "3.0.0", changes: []schemaChange{metricChange(map[string]string{"bar": "foo"})}},
		)
		rv := &renameValidator{seen: map[string]struct{}{}}
		variants := requireMatcherVariants(t, "3.0.0", schema, equalMatchers("foo"), nil, rv)
		require.Equal(t, []string{"foo", "bar"}, variantNames(variants))
		require.Empty(t, rv.warnings)
	})
}

func TestManyToOneMetricRenames(t *testing.T) {
	t.Run("follows each predecessor's history", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "1.1.0", changes: []schemaChange{
				metricChange(map[string]string{"metric.b.older": "metric.b"}),
			}},
			schemaRevision{version: "1.2.0", changes: []schemaChange{
				metricChange(map[string]string{"metric.a": "metric.merged", "metric.b": "metric.merged"}),
			}},
		)

		variants := requireMatcherVariants(t, "1.2.0", schema, equalMatchers("metric.merged"), nil, nil)
		require.ElementsMatch(t,
			[]string{"metric.merged", "metric.a", "metric.b", "metric.b.older"},
			variantNames(variants))
	})

	t.Run("upstream schema collapses two spellings deterministically", func(t *testing.T) {
		b, err := os.ReadFile("./testdata/upstream/schema-1.44.0.yaml")
		require.NoError(t, err)
		schema, err := loadOTelSchema(b)
		require.NoError(t, err)

		variants := requireMatcherVariants(t, "1.44.0", &schema,
			equalMatchers("k8s.replicationcontroller.pod.available"), nil, nil)
		require.Equal(t, []string{
			"k8s.replicationcontroller.pod.available",
			"k8s.replication_controller.available_pods",
			"k8s.replicationcontroller.available_pods",
		}, variantNames(variants))
	})
}

func TestAttributeScopingFollowsMetricBranches(t *testing.T) {
	t.Run("scope is evaluated at its transformation position", func(t *testing.T) {
		beforeRename := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				attributeChange(map[string]string{"user": "tenant"}, "metric.old"),
				metricChange(map[string]string{"metric.old": "metric.new"}),
			},
		})
		variants := requireMatcherVariants(t, "1.1.0", beforeRename, equalMatchers("metric.new", "tenant"), []string{"tenant"}, nil)
		require.Equal(t, []string{"metric.new/tenant", "metric.old/user"}, []string{
			variantMetricAndAttribute(variants[0]), variantMetricAndAttribute(variants[1]),
		})

		afterRename := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				metricChange(map[string]string{"metric.old": "metric.new"}),
				attributeChange(map[string]string{"user": "tenant"}, "metric.old"),
			},
		})
		variants = requireMatcherVariants(t, "1.1.0", afterRename, equalMatchers("metric.new", "tenant"), []string{"tenant"}, nil)
		require.Equal(t, []string{"metric.new/tenant", "metric.old/tenant"}, []string{
			variantMetricAndAttribute(variants[0]), variantMetricAndAttribute(variants[1]),
		})
	})

	t.Run("a scope applies only to its many-to-one predecessor", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				attributeChange(map[string]string{"user": "tenant"}, "metric.a"),
				metricChange(map[string]string{"metric.a": "metric.current", "metric.b": "metric.current"}),
			},
		})
		variants := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric.current", "tenant"), []string{"tenant"}, nil)
		require.Len(t, variants, 3)
		require.Equal(t, "metric.current/tenant", variantMetricAndAttribute(variants[0]))
		require.Equal(t, "metric.a/user", variantMetricAndAttribute(variants[1]))
		require.Equal(t, map[string]string{"user": "tenant"}, variants[1].mapping.translatedLabels)
		require.Equal(t, "metric.b/tenant", variantMetricAndAttribute(variants[2]))
		require.Empty(t, variants[2].mapping.translatedLabels)
	})

	t.Run("an inapplicable earlier scope does not resolve a temporary origin", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{
				attributeChange(map[string]string{"attr.a": "attr.current"}, "metric.a"),
				attributeChange(map[string]string{"attr.b": "attr.current"}, "metric.b"),
			},
		})

		variants := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric.b", "attr.current"), []string{"attr.current"}, nil)
		require.Equal(t, []string{"metric.b/attr.current", "metric.b/attr.b"}, []string{
			variantMetricAndAttribute(variants[0]),
			variantMetricAndAttribute(variants[1]),
		})
		require.Equal(t, map[string]string{"attr.b": "attr.current"}, variants[1].mapping.translatedLabels)
	})
}

func TestAttributeMatcherExpansion(t *testing.T) {
	t.Run("moves duplicate matchers together", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{attributeChange(map[string]string{
				"attr.a": "attr.current",
				"attr.b": "attr.current",
				"attr.c": "attr.current",
			})},
		})
		matchers := equalMatchers("metric")
		for range 13 {
			matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, "attr.current", "value"))
		}

		variants := requireMatcherVariants(t, "1.1.0", schema, matchers, []string{"attr.current"}, nil)
		require.Len(t, variants, 4)
		for _, variant := range variants {
			var attributeName string
			for _, matcher := range variant.matchers[1:] {
				if attributeName == "" {
					attributeName = matcher.Name
				}
				require.Equal(t, attributeName, matcher.Name)
			}
		}
	})

	t.Run("caps products across distinct attributes", func(t *testing.T) {
		for _, tc := range []struct {
			groups  int
			wantLen int
			wantErr bool
		}{
			{groups: 8, wantLen: 256},
			{groups: 9, wantErr: true},
		} {
			t.Run(fmt.Sprintf("%d groups", tc.groups), func(t *testing.T) {
				renamed := map[string]string{}
				state := lineageState{metric: "metric", matchers: equalMatchers("metric")}
				for i := range tc.groups {
					current := fmt.Sprintf("attr.current.%d", i)
					renamed[fmt.Sprintf("attr.a.%d", i)] = current
					renamed[fmt.Sprintf("attr.b.%d", i)] = current
					state.matchers = append(state.matchers, labels.MustNewMatcher(labels.MatchEqual, current, "value"))
				}

				states, err := transformAttributeMatcherStatesWithBudget(state, newDirectedRenames(renamed), traverseBackward, 0, nil, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
				if tc.wantErr {
					require.ErrorIs(t, err, errSchemaExpansion)
					return
				}
				require.NoError(t, err)
				require.Len(t, states, tc.wantLen)
			})
		}
	})
}

func TestReplaceMetricMatchers(t *testing.T) {
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.current"),
		labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, "metric.current"),
		labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "metric.other"),
		labels.MustNewMatcher(labels.MatchEqual, "attribute", "value"),
	}
	got, err := replaceMetricMatchersWithBudget(matchers, "metric.current", "metric.old", newSchemaExpansionBudget(productionSchemaExpansionLimits()))
	require.NoError(t, err)

	require.Equal(t, labels.MatchEqual, got[0].Type)
	require.Equal(t, "metric.old", got[0].Value)
	require.Same(t, matchers[1], got[1], "non-equality constraints are normalized before traversal")
	require.Same(t, matchers[2], got[2], "non-equality constraints are normalized before traversal")
	require.Same(t, matchers[3], got[3], "a different metric constraint must remain untouched")
	require.Same(t, matchers[4], got[4], "a non-metric constraint must remain untouched")
}

func TestVariantAccumulatorMergesAttributeAliases(t *testing.T) {
	t.Run("compatible mappings", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{attributeChange(map[string]string{"user": "tenant"})},
		})
		variants := requireMatcherVariants(t, "1.1.0", schema, equalMatchers("metric"), []string{"tenant"}, nil)
		require.Len(t, variants, 1)
		require.Equal(t, map[string]string{"user": "tenant"}, variants[0].mapping.translatedLabels)
	})

	t.Run("conflicting mappings", func(t *testing.T) {
		rv := &renameValidator{seen: map[string]struct{}{}}
		acc := requireVariantAccumulator(t, "metric", []string{"tenant", "account"}, rv)
		matchers := equalMatchers("metric")
		require.NoError(t, acc.add(lineageState{matchers: matchers, metric: "metric", attrs: attributeLineage{
			"tenant": {"user": attributeOriginResolved},
		}}))
		require.NoError(t, acc.add(lineageState{matchers: matchers, metric: "metric", attrs: attributeLineage{
			"account": {"user": attributeOriginResolved},
		}}))

		require.Len(t, acc.variants, 1)
		require.NotContains(t, acc.variants[0].mapping.translatedLabels, "user")
		require.Len(t, rv.warnings, 1)
		require.Contains(t, rv.warnings[0], `attribute name "user" resolves to both "tenant" and "account"`)
	})

	t.Run("identity reuse across matcher variants", func(t *testing.T) {
		acc := requireVariantAccumulator(t, "metric", nil, nil)
		require.NoError(t, acc.add(lineageState{
			matchers: equalMatchers("metric", "first"),
			metric:   "metric",
			attrs: attributeLineage{
				"user": {"account": attributeOriginResolved},
			},
		}))
		err := acc.add(lineageState{
			matchers: equalMatchers("metric", "second"),
			metric:   "metric",
			attrs: attributeLineage{
				"tenant": {"user": attributeOriginResolved},
			},
		})
		require.ErrorIs(t, err, errAmbiguousSchemaRename)
		require.ErrorContains(t, err, `attribute name "user"`)
	})

	t.Run("identities stay scoped to a physical metric", func(t *testing.T) {
		acc := requireVariantAccumulator(t, "metric.current", nil, nil)
		require.NoError(t, acc.add(lineageState{
			matchers: equalMatchers("metric.old"),
			metric:   "metric.old",
			attrs: attributeLineage{
				"user": {"account": attributeOriginResolved},
			},
		}))
		require.NoError(t, acc.add(lineageState{
			matchers: equalMatchers("metric.new"),
			metric:   "metric.new",
			attrs: attributeLineage{
				"tenant": {"user": attributeOriginResolved},
			},
		}))
	})
}

func TestAttributeLifecycleReuse(t *testing.T) {
	t.Run("rejects a distinct identity claiming an alias", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "1.1.0", changes: []schemaChange{attributeChange(map[string]string{"user": "tenant"})}},
			schemaRevision{version: "1.2.0", changes: []schemaChange{attributeChange(map[string]string{"account": "user"})}},
		)

		_, err := generateMatcherVariantsWithBudget("1.2.0", schema, equalMatchers("metric", "user"), nil, nil, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
		require.ErrorIs(t, err, errAmbiguousSchemaRename)
		require.ErrorContains(t, err, `attribute name "user"`)
	})

	t.Run("allows a legitimate rename back", func(t *testing.T) {
		schema := testSchema(
			schemaRevision{version: "1.1.0", changes: []schemaChange{attributeChange(map[string]string{"user": "tenant"})}},
			schemaRevision{version: "1.2.0", changes: []schemaChange{attributeChange(map[string]string{"tenant": "user"})}},
		)

		variants := requireMatcherVariants(t, "1.2.0", schema, equalMatchers("metric", "user"), nil, nil)
		require.NotEmpty(t, variants)
	})

	t.Run("allows explicit convergence into an occupied identity", func(t *testing.T) {
		schema := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{attributeChange(map[string]string{
				"tls.client.server_name": "server.address",
			})},
		})

		variants := requireMatcherVariants(t, "1.0.0", schema, equalMatchers("metric"), []string{"server.address"}, nil)
		require.NotEmpty(t, variants)
	})
}

func TestSchemaExpansionLimits(t *testing.T) {
	t.Run("attribute lineage includes canonical definitions", func(t *testing.T) {
		for _, tc := range []struct {
			attributes int
			wantLen    int
			wantErr    bool
		}{
			{attributes: maxSchemaExpansion, wantLen: maxSchemaExpansion},
			{attributes: maxSchemaExpansion + 1, wantErr: true},
		} {
			t.Run(fmt.Sprintf("%d attributes", tc.attributes), func(t *testing.T) {
				attributes := make([]string, 0, tc.attributes)
				for i := range tc.attributes {
					attributes = append(attributes, fmt.Sprintf("attr.%03d", i))
				}
				lineage, err := newAttributeLineageWithBudget(attributes, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
				if tc.wantErr {
					require.ErrorIs(t, err, errSchemaExpansion)
					return
				}
				require.NoError(t, err)
				require.Len(t, lineage, tc.wantLen)
			})
		}
	})

	t.Run("matcher variants include the anchor", func(t *testing.T) {
		for _, tc := range []struct {
			sources int
			wantLen int
			wantErr bool
		}{
			{sources: maxSchemaExpansion - 1, wantLen: maxSchemaExpansion},
			{sources: maxSchemaExpansion, wantErr: true},
		} {
			t.Run(fmt.Sprintf("%d sources", tc.sources), func(t *testing.T) {
				renamed := make(map[string]string, tc.sources)
				for i := range tc.sources {
					renamed[fmt.Sprintf("metric.old.%03d", i)] = "metric.current"
				}
				schema := testSchema(schemaRevision{
					version: "1.1.0",
					changes: []schemaChange{metricChange(renamed)},
				})

				variants, err := generateMatcherVariantsWithBudget("1.1.0", schema, equalMatchers("metric.current"), nil, nil, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
				if tc.wantErr {
					require.ErrorIs(t, err, errSchemaExpansion)
					return
				}
				require.NoError(t, err)
				require.Len(t, variants, tc.wantLen)
			})
		}
	})

	t.Run("attribute mappings are bounded across states", func(t *testing.T) {
		acc := requireVariantAccumulator(t, "metric", []string{"attr.current"}, nil)
		matchers := equalMatchers("metric")
		for i := range maxSchemaExpansion {
			err := acc.add(lineageState{matchers: matchers, metric: "metric", attrs: attributeLineage{
				"attr.current": {fmt.Sprintf("attr.old.%03d", i): attributeOriginResolved},
			}})
			require.NoError(t, err)
		}
		err := acc.add(lineageState{matchers: matchers, metric: "metric", attrs: attributeLineage{
			"attr.current": {"attr.overflow": attributeOriginResolved},
		}})
		require.ErrorIs(t, err, errSchemaExpansion)
	})

	t.Run("label value jobs include the canonical name", func(t *testing.T) {
		for _, tc := range []struct {
			aliases int
			wantLen int
			wantErr bool
		}{
			{aliases: maxSchemaExpansion - 1, wantLen: maxSchemaExpansion},
			{aliases: maxSchemaExpansion, wantErr: true},
		} {
			t.Run(fmt.Sprintf("%d aliases", tc.aliases), func(t *testing.T) {
				mapping := make(map[string]string, tc.aliases)
				for i := range tc.aliases {
					mapping[fmt.Sprintf("attr.old.%03d", i)] = "attr.current"
				}
				jobs, err := buildLabelValueJobs([]matcherVariant{{
					matchers: equalMatchers("metric"),
					mapping:  buildLabelMapping("metric", mapping),
				}}, "attr.current")
				if tc.wantErr {
					require.ErrorIs(t, err, errSchemaExpansion)
					return
				}
				require.NoError(t, err)
				require.Len(t, jobs, tc.wantLen)
			})
		}
	})
}

func TestSchemaExpansionBudget(t *testing.T) {
	state := lineageState{
		matchers: equalMatchers("metric", "tenant"),
		metric:   "metric",
		attrs: attributeLineage{
			"tenant": {"tenant": attributeOriginResolved, "user": attributeOriginResolved},
		},
	}

	t.Run("find variants returns no partial result", func(t *testing.T) {
		files := map[string][]byte{
			"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.1.0:
`),
			"1.1.0": []byte(`groups:
  - id: metric.metric
    type: metric
    metric_name: metric
    unit: s
    instrument: histogram
`),
		}
		for name, tc := range map[string]struct {
			limits schemaExpansionLimits
			detail string
		}{
			"work": {
				limits: schemaExpansionLimits{work: 1, keyBytes: 1_000},
				detail: "resolver work",
			},
			"key bytes": {
				limits: schemaExpansionLimits{work: 1_000, keyBytes: 1},
				detail: "deduplication key bytes",
			},
		} {
			t.Run(name, func(t *testing.T) {
				engine := newSchemaEngine(newRegistrySource(files))
				engine.limits = tc.limits
				variants, _, err := engine.findMatcherVariants(
					"registry/1.1.0",
					"registry/registry.yaml",
					equalMatchers("metric"),
				)
				require.ErrorIs(t, err, errSchemaExpansion)
				require.ErrorContains(t, err, tc.detail)
				require.Nil(t, variants)
			})
		}
	})

	t.Run("metric ownership analysis returns no partial result", func(t *testing.T) {
		revisions := []schemaRevision{{
			version: "1.1.0",
			changes: []schemaChange{metricChange(map[string]string{"metric.old": "metric.current"})},
		}}
		reused, err := reusedMetricNamesWithBudget(revisions, newSchemaExpansionBudget(schemaExpansionLimits{work: 1, keyBytes: 1_000}))
		require.ErrorIs(t, err, errSchemaExpansion)
		require.Nil(t, reused)
	})

	t.Run("cached metric ownership work is charged to every query", func(t *testing.T) {
		files := map[string][]byte{
			"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            metric.old: metric.current
`),
			"1.1.0": []byte(`groups:
  - id: metric.metric.current
    type: metric
    metric_name: metric.current
    unit: s
    instrument: histogram
`),
		}
		engine := newSchemaEngine(newRegistrySource(files))
		schema, err := engine.getOTelSchema("registry/registry.yaml")
		require.NoError(t, err)
		analysis := engine.schemaSafetyAnalysis("registry/registry.yaml", &schema)
		require.NoError(t, analysis.err)
		require.Positive(t, analysis.work)

		engine.limits = schemaExpansionLimits{work: analysis.work - 1, keyBytes: 1_000_000}
		variants, _, err := engine.findMatcherVariants(
			"registry/1.1.0",
			"registry/registry.yaml",
			equalMatchers("metric.current"),
		)
		require.ErrorIs(t, err, errSchemaExpansion)
		require.Nil(t, variants)
	})

	t.Run("preflights keys before allocation and state mutation", func(t *testing.T) {
		keyBytes := uint64(len(requireLineageStateKey(t, state)))
		require.Positive(t, keyBytes)

		budget := newSchemaExpansionBudget(schemaExpansionLimits{work: 1_000, keyBytes: keyBytes - 1})
		states := newLineageStateSet(1, budget)
		err := states.add(state)
		require.ErrorIs(t, err, errSchemaExpansion)
		require.ErrorContains(t, err, "deduplication key bytes")
		require.Empty(t, states.states)
		require.Empty(t, states.seen)
		require.Zero(t, budget.keyBytes)

		budget = newSchemaExpansionBudget(schemaExpansionLimits{work: 1_000, keyBytes: keyBytes})
		states = newLineageStateSet(1, budget)
		require.NoError(t, states.add(state))
		require.Len(t, states.states, 1)
		require.Equal(t, keyBytes, budget.keyBytes)
	})

	t.Run("charges duplicate attempts", func(t *testing.T) {
		probe := newSchemaExpansionBudget(schemaExpansionLimits{work: 1_000, keyBytes: 1_000})
		require.NoError(t, newLineageStateSet(1, probe).add(state))
		require.Positive(t, probe.work)
		require.Positive(t, probe.keyBytes)

		budget := newSchemaExpansionBudget(schemaExpansionLimits{
			work:     probe.work*2 - 1,
			keyBytes: probe.keyBytes * 2,
		})
		states := newLineageStateSet(1, budget)
		require.NoError(t, states.add(state))
		err := states.add(state)
		require.ErrorIs(t, err, errSchemaExpansion)
		require.ErrorContains(t, err, "resolver work")
		require.Len(t, states.states, 1)
		require.Len(t, states.seen, 1)
		require.Equal(t, probe.keyBytes, budget.keyBytes)
	})

	t.Run("starts before identity recovery", func(t *testing.T) {
		schema := testSchemaWithVersions([]string{"1.0.0", "1.1.0"}, schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{metricChange(map[string]string{"metric.old": "metric.current"})},
		})
		anchor := semconv{version: "1.1.0", metrics: map[string]metricDef{}}
		lookup := func(string, string) (metricDef, metricLookupStatus) {
			return metricDef{unit: "s", instrument: "histogram"}, metricDeclared
		}
		budget := newSchemaExpansionBudget(schemaExpansionLimits{work: 1, keyBytes: 1_000})

		_, err := newRenameValidator(schema, lookup, anchor, "metric.old", budget)
		require.ErrorIs(t, err, errSchemaExpansion)
		require.ErrorContains(t, err, "resolver work")
	})

	t.Run("shares one budget across traversal directions", func(t *testing.T) {
		backward := testSchema(schemaRevision{
			version: "1.1.0",
			changes: []schemaChange{metricChange(map[string]string{"metric.old": "metric.current"})},
		})
		forward := testSchema(schemaRevision{
			version: "1.2.0",
			changes: []schemaChange{metricChange(map[string]string{"metric.current": "metric.new"})},
		})
		combined := testSchema(
			schemaRevision{
				version: "1.1.0",
				changes: []schemaChange{metricChange(map[string]string{"metric.old": "metric.current"})},
			},
			schemaRevision{
				version: "1.2.0",
				changes: []schemaChange{metricChange(map[string]string{"metric.current": "metric.new"})},
			},
		)

		measure := func(t *testing.T, schema *otelSchema, wantVariants int) uint64 {
			t.Helper()
			budget := newSchemaExpansionBudget(schemaExpansionLimits{work: 1_000, keyBytes: 1_000_000})
			variants, err := generateMatcherVariantsWithBudget("1.1.0", schema, equalMatchers("metric.current"), nil, nil, budget)
			require.NoError(t, err)
			require.Len(t, variants, wantVariants)
			return budget.work
		}

		backwardWork := measure(t, backward, 2)
		forwardWork := measure(t, forward, 2)
		combinedWork := measure(t, combined, 3)
		limit := max(backwardWork, forwardWork)
		require.Greater(t, combinedWork, limit)

		for name, schema := range map[string]*otelSchema{"backward": backward, "forward": forward} {
			t.Run(name+" fits independently", func(t *testing.T) {
				budget := newSchemaExpansionBudget(schemaExpansionLimits{work: limit, keyBytes: 1_000_000})
				_, err := generateMatcherVariantsWithBudget("1.1.0", schema, equalMatchers("metric.current"), nil, nil, budget)
				require.NoError(t, err)
			})
		}

		budget := newSchemaExpansionBudget(schemaExpansionLimits{work: limit, keyBytes: 1_000_000})
		variants, err := generateMatcherVariantsWithBudget("1.1.0", combined, equalMatchers("metric.current"), nil, nil, budget)
		require.ErrorIs(t, err, errSchemaExpansion)
		require.ErrorContains(t, err, "resolver work")
		require.Nil(t, variants, "overflow must not return partial variants")
	})
}

func BenchmarkMetricOwnershipAnalysisUpstream(b *testing.B) {
	raw, err := os.ReadFile("./testdata/upstream/schema-1.44.0.yaml")
	require.NoError(b, err)
	schema, err := loadOTelSchema(raw)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reused, err := reusedMetricNamesWithBudget(schema.revisions, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
		if err != nil {
			b.Fatal(err)
		}
		if reused == nil {
			b.Fatal("ownership analysis returned a nil result")
		}
	}
}

func TestDeduplicateLineageStatesPreservesCanonicalIdentity(t *testing.T) {
	matchers := equalMatchers("metric")
	states := []lineageState{
		{matchers: matchers, metric: "metric", attrs: attributeLineage{"tenant": {"tenant": attributeOriginResolved, "user": attributeOriginResolved}}},
		{matchers: matchers, metric: "metric", attrs: attributeLineage{"tenant": {"user": attributeOriginResolved}}},
		{matchers: matchers, metric: "metric", attrs: attributeLineage{"tenant": {"user": attributeOriginResolved}, "user": {"tenant": attributeOriginResolved}}},
	}

	deduplicated, err := deduplicateLineageStatesWithBudget(states, newSchemaExpansionBudget(productionSchemaExpansionLimits()))
	require.NoError(t, err)
	require.Len(t, deduplicated, 3)
	require.NotEqual(t,
		requireLineageStateKey(t, lineageState{matchers: matchers, attrs: attributeLineage{"a": {"b": attributeOriginResolved}, "c": {"d": attributeOriginResolved}}}),
		requireLineageStateKey(t, lineageState{matchers: matchers, attrs: attributeLineage{"a": {"b": attributeOriginResolved, "c": attributeOriginResolved, "d": attributeOriginResolved}}}),
	)
	require.NotEqual(t,
		requireLineageStateKey(t, lineageState{matchers: matchers, metric: "metric", renamedFrom: "metric.old"}),
		requireLineageStateKey(t, lineageState{matchers: matchers, metric: "metric", renamedFrom: "metric.other"}),
	)
	require.NotEqual(t,
		requireLineageStateKey(t, lineageState{matchers: matchers, metric: "metric"}),
		requireLineageStateKey(t, lineageState{matchers: matchers, metric: "metric", metricOriginPending: true}),
	)
	require.NotEqual(t,
		requireLineageStateKey(t, lineageState{matchers: matchers, metric: "metric"}),
		requireLineageStateKey(t, lineageState{matchers: matchers, metric: "metric", pendingAttributeMatchers: map[int]struct{}{1: {}}}),
	)
	require.NotEqual(t,
		requireLineageStateKey(t, lineageState{matchers: matchers, attrs: attributeLineage{"a": {"b": attributeOriginResolved}}}),
		requireLineageStateKey(t, lineageState{matchers: matchers, attrs: attributeLineage{"a": {"b": attributeOriginPending}}}),
	)
}
