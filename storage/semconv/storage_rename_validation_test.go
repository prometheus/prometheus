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

package semconv_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/util/teststorage"
)

// renameSchema is a two-version schema renaming old→new at 1.1.0, the shape
// every case below varies the semconv files around.
const renameSchema = `file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            %s: %s
`

// metricSemconv renders a semconv file declaring a single metric group. The
// group id follows the "metric.<metric_name>" form that upstream semantic
// conventions lint for, so it necessarily changes whenever the metric name does.
func metricSemconv(metricName, unit, instrument string) []byte {
	return metricSemconvWithStability(metricName, unit, instrument, "stable")
}

func metricSemconvWithStability(metricName, unit, instrument, stability string) []byte {
	stabilityLine := ""
	if stability != "" {
		stabilityLine = fmt.Sprintf("    stability: %s\n", stability)
	}
	return fmt.Appendf(nil, `groups:
  - id: metric.%s
    type: metric
    metric_name: %s
    unit: %q
    instrument: %s
%s    attributes:
      - ref: http.response.status_code
`, metricName, metricName, unit, instrument, stabilityLine)
}

// selectRenamed queries metricName anchored at semconv 1.1.0 over the given
// registry and returns the series found plus any warnings raised.
func selectRenamed(t *testing.T, files map[string][]byte, appendUnder, metricName string) (map[string]float64, []string) {
	t.Helper()
	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, files)
	require.NoError(t, err)

	appendSeries(t, wrapped, appendUnder, 1, 7.0, "http.response.status_code", "200")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metricName),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	got := collectSeries(t, set)
	return got, warningStrings(set.Warnings())
}

func selectRenamedError(t *testing.T, files map[string][]byte, appendUnder, metricName string) error {
	t.Helper()
	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, files)
	require.NoError(t, err)

	appendSeries(t, wrapped, appendUnder, 1, 7.0, "http.response.status_code", "200")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metricName),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	require.False(t, set.Next())
	return set.Err()
}

func TestRenameEdgeValidation(t *testing.T) {
	// A legitimate rename: the metric is the same measurement under a new name,
	// so unit and instrument agree and the historical series must still merge.
	//
	// This is also the case that rules out using the semconv group id as a
	// cross-version identity: the ids here are metric.old.name and
	// metric.new.name, so requiring them to be equal across the edge would
	// reject this rename even though it is exactly the rename the schema exists
	// to describe.
	t.Run("merges a rename whose unit and instrument agree", func(t *testing.T) {
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
			"1.0.0":         metricSemconv("old.name", "s", "histogram"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "old.name", "new.name")

		require.Len(t, got, 1, "expected the pre-rename series under the queried name, got %v", got)
		for k := range got {
			require.Contains(t, k, `__name__="new.name"`)
		}
		require.Empty(t, warnings, "a corroborated rename must not warn")
	})

	// The case the schema format cannot express and name traversal cannot
	// detect: two unrelated metrics that happen to share a surface name at
	// different versions. Their units disagree, so merging them would average
	// seconds with a queue depth.
	t.Run("does not merge a rename whose unit disagrees", func(t *testing.T) {
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
			"1.0.0":         metricSemconv("old.name", "{item}", "updowncounter"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "old.name", "new.name")

		require.Empty(t, got, "series of a differently-united metric must not be merged in, got %v", got)
		requireWarningsContain(t, warnings, "treating them as different metrics")
		requireWarningsContain(t, warnings, `resolves it to "old.name"`)
	})

	// Same unit, different instrument: a histogram and a counter are not the
	// same metric even when they measure in the same unit.
	t.Run("does not merge a rename whose instrument disagrees", func(t *testing.T) {
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
			"1.0.0":         metricSemconv("old.name", "s", "counter"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "old.name", "new.name")

		require.Empty(t, got, "got %v", got)
		requireWarningsContain(t, warnings, "treating them as different metrics")
	})

	for _, tc := range []struct {
		name         string
		oldStability string
		newStability string
	}{
		{name: "development definitions", oldStability: "development", newStability: "development"},
		{name: "one stable definition", oldStability: "development", newStability: "stable"},
		{name: "unspecified definitions"},
	} {
		t.Run("follows a metadata-changing rename for "+tc.name, func(t *testing.T) {
			got, warnings := selectRenamed(t, map[string][]byte{
				"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
				"1.0.0":         metricSemconvWithStability("old.name", "{item}", "updowncounter", tc.oldStability),
				"1.1.0":         metricSemconvWithStability("new.name", "s", "histogram", tc.newStability),
			}, "old.name", "new.name")

			require.Len(t, got, 1, "an explicit non-stable rename must still surface its historical series, got %v", got)
			requireWarningsContain(t, warnings, "following the explicit schema rename")
		})
	}

	// A registry need not ship a semconv for every version its schema
	// references, so an absent file means "cannot verify" and must leave the
	// existing name-traversal behaviour untouched while warning the caller.
	t.Run("warns but traverses an unverifiable rename when a semconv is absent", func(t *testing.T) {
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "old.name", "new.name")

		require.Len(t, got, 1, "expected the pre-rename series to still merge, got %v", got)
		require.Len(t, warnings, 1)
		requireWarningsContain(t, warnings, "version 1.0.0 is unavailable")
		requireWarningsContain(t, warnings, "without corroboration")
	})

	// A name the semconv does not declare as a metric is a strong hint of a
	// mis-authored edge, but a trimmed registry is a legitimate shape, so this
	// is reported without dropping series.
	t.Run("warns but still merges when a name is not declared as a metric", func(t *testing.T) {
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "typo.name", "new.name"),
			"1.0.0":         metricSemconv("old.name", "s", "histogram"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "typo.name", "new.name")

		require.Len(t, got, 1, "expected the series to still merge, got %v", got)
		requireWarningsContain(t, warnings, "could not be corroborated")
	})
}

// TestRecycledMetricName covers the case the schema format cannot express: a
// metric name that was renamed away and later reused by an unrelated metric.
//
//	1.0.0  foo exists, counting bytes
//	2.0.0  schema renames foo to bar
//	5.0.0  a new, unrelated metric claims the name foo, measuring seconds
//
// Querying foo at 5.0.0 means the new metric. The 2.0.0 rename edge concerns the
// old foo and must not drag bar's series in. Walking backwards encounters the
// queried name only on the old side of that edge, which marks the lifecycle
// boundary even when the old and new metrics have identical signatures.
func TestRecycledMetricName(t *testing.T) {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/5.0.0
versions:
  1.0.0:
  2.0.0:
    metrics:
      changes:
        - rename_metrics:
            foo: bar
  5.0.0:
`)

	for _, tc := range []struct {
		name       string
		unit       string
		instrument string
	}{
		{name: "different signature", unit: "s", instrument: "histogram"},
		{name: "same signature", unit: "By", instrument: "counter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 5.0.0 declares both the reused name and the metric the old foo became.
			currentSemconv := fmt.Appendf(nil, `groups:
  - id: metric.foo
    type: metric
    metric_name: foo
    unit: %q
    instrument: %s
    attributes:
      - ref: http.response.status_code
  - id: metric.bar
    type: metric
    metric_name: bar
    unit: "By"
    instrument: counter
    attributes:
      - ref: http.response.status_code
`, tc.unit, tc.instrument)

			underlying := teststorage.New(t)
			wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
				"registry.yaml": schema,
				"1.0.0":         metricSemconv("foo", "By", "counter"),
				"2.0.0":         metricSemconv("bar", "By", "counter"),
				"5.0.0":         currentSemconv,
			})
			require.NoError(t, err)

			// Series of the metric the old foo became. Querying today's foo must
			// not pick these up, even when identity fields happen to match.
			appendSeries(t, wrapped, "bar", 1, 7.0, "http.response.status_code", "200")

			q, err := wrapped.Querier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })

			set := q.Select(context.Background(), false, nil,
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
				labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/5.0.0"),
				labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
			)
			got := collectSeries(t, set)
			require.Empty(t, got, "an unrelated metric that once held this name must not be merged in, got %v", got)
			requireWarningsContain(t, warningStrings(set.Warnings()), "metric lifecycle boundary")
		})
	}

	t.Run("trimmed anchor omits the reused name", func(t *testing.T) {
		underlying := teststorage.New(t)
		wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
			"registry.yaml": schema,
			"1.0.0":         metricSemconv("foo", "By", "counter"),
			"2.0.0":         metricSemconv("bar", "By", "counter"),
			"5.0.0":         metricSemconv("unrelated", "s", "histogram"),
		})
		require.NoError(t, err)

		appendSeries(t, wrapped, "bar", 1, 7.0, "http.response.status_code", "200")
		appendSeries(t, wrapped, "foo", 1, 9.0, "http.response.status_code", "500")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/5.0.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)
		got := collectSeries(t, set)
		require.Equal(t, map[string]float64{
			labels.FromStrings(model.MetricNameLabel, "foo", "http.response.status_code", "500").String(): 9,
		}, got)
		requireWarningsContain(t, warningStrings(set.Warnings()), "metric lifecycle boundary")
	})
}

// TestAttributeRenameScope checks that an attribute rename restricted by
// apply_to_metrics does not rewrite the attributes of other metrics. The schema
// scopes user→tenant to scoped.metric only, so a series of other.metric that
// carries user must keep it.
func TestAttributeRenameScope(t *testing.T) {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              user: tenant
            apply_to_metrics:
              - scoped.metric
`)
	semconv110 := []byte(`groups:
  - id: metric.scoped.metric
    type: metric
    metric_name: scoped.metric
    unit: "s"
    instrument: histogram
    attributes:
      - ref: tenant
  - id: metric.other.metric
    type: metric
    metric_name: other.metric
    unit: "s"
    instrument: histogram
    attributes:
      - ref: tenant
`)

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": schema,
		"1.1.0":         semconv110,
	})
	require.NoError(t, err)

	appendSeries(t, wrapped, "scoped.metric", 1, 1.0, "user", "alice")
	appendSeries(t, wrapped, "other.metric", 1, 2.0, "user", "bob")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	selectMetric := func(name string) map[string]float64 {
		return collectSeries(t, q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, name),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		))
	}

	// The metric the rename is scoped to still has its historical attribute
	// normalised to the anchor version's name.
	got := selectMetric("scoped.metric")
	require.Len(t, got, 1, "got %v", got)
	for k := range got {
		require.Contains(t, k, `tenant="alice"`, "the scoped metric must be normalised")
	}

	// Any other metric must be left alone. Before apply_to_metrics was honoured,
	// this attribute was rewritten to tenant as well.
	got = selectMetric("other.metric")
	require.Len(t, got, 1, "got %v", got)
	for k := range got {
		require.Contains(t, k, `user="bob"`, "a rename scoped to another metric must not apply here")
		require.NotContains(t, k, "tenant=")
	}
}

func TestRejectedLineageDoesNotLeakAttributeScope(t *testing.T) {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/5.0.0
versions:
  1.0.0:
  2.0.0:
    metrics:
      changes:
        - rename_metrics:
            foo: bar
        - rename_attributes:
            attribute_map:
              user: tenant
            apply_to_metrics:
              - bar
  5.0.0:
`)
	semconv500 := []byte(`groups:
  - id: metric.foo
    type: metric
    metric_name: foo
    unit: "s"
    instrument: histogram
    attributes:
      - ref: tenant
`)

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": schema,
		"5.0.0":         semconv500,
	})
	require.NoError(t, err)
	appendSeries(t, wrapped, "foo", 1, 1.0, "user", "alice")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "foo"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/5.0.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	got := collectSeries(t, set)
	require.Len(t, got, 1)
	for key := range got {
		require.Contains(t, key, `user="alice"`)
		require.NotContains(t, key, `tenant=`)
	}
	requireWarningsContain(t, warningStrings(set.Warnings()), "metric lifecycle boundary")
}

func TestManyToOneAttributeScopeStaysBranchLocal(t *testing.T) {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              user: tenant
            apply_to_metrics:
              - metric.a
        - rename_metrics:
            metric.a: metric.current
            metric.b: metric.current
`)
	semconv100 := []byte(`groups:
  - id: metric.metric.a
    type: metric
    metric_name: metric.a
    unit: "s"
    instrument: histogram
    attributes:
      - ref: user
  - id: metric.metric.b
    type: metric
    metric_name: metric.b
    unit: "s"
    instrument: histogram
    attributes:
      - ref: user
`)
	semconv110 := []byte(`groups:
  - id: metric.metric.current
    type: metric
    metric_name: metric.current
    unit: "s"
    instrument: histogram
    attributes:
      - ref: tenant
`)

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": schema,
		"1.0.0":         semconv100,
		"1.1.0":         semconv110,
	})
	require.NoError(t, err)
	appendSeries(t, wrapped, "metric.a", 1, 1.0, "user", "alice")
	appendSeries(t, wrapped, "metric.b", 1, 2.0, "user", "bob")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	baseMatchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}

	set := q.Select(context.Background(), false, nil, baseMatchers...)
	got := collectSeries(t, set)
	require.Len(t, got, 2, "got %v", got)
	var sawScoped, sawUnscoped bool
	for key := range got {
		sawScoped = sawScoped || strings.Contains(key, `tenant="alice"`)
		sawUnscoped = sawUnscoped || strings.Contains(key, `user="bob"`)
	}
	require.True(t, sawScoped, "metric.a's scoped alias was not canonicalised: %v", got)
	require.True(t, sawUnscoped, "metric.a's scope leaked into metric.b: %v", got)
	require.Empty(t, warningStrings(set.Warnings()))

	matched := q.Select(context.Background(), false, nil, append(baseMatchers,
		labels.MustNewMatcher(labels.MatchEqual, "tenant", "alice"),
	)...)
	require.Len(t, collectSeries(t, matched), 1)

	names, anns, err := q.LabelNames(context.Background(), nil, baseMatchers...)
	require.NoError(t, err)
	require.Empty(t, warningStrings(anns))
	require.Contains(t, names, "tenant")
	require.Contains(t, names, "user")

	values, anns, err := q.LabelValues(context.Background(), "tenant", nil, baseMatchers...)
	require.NoError(t, err)
	require.Empty(t, warningStrings(anns))
	require.Equal(t, []string{"alice"}, values)

	cq, err := wrapped.ChunkQuerier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cq.Close() })
	chunks := cq.Select(context.Background(), false, nil, baseMatchers...)
	var chunkLabels []string
	for chunks.Next() {
		chunkLabels = append(chunkLabels, chunks.At().Labels().String())
	}
	require.NoError(t, chunks.Err())
	require.Len(t, chunkLabels, 2)
	require.Condition(t, func() bool {
		return strings.Contains(strings.Join(chunkLabels, "\n"), `tenant="alice"`) &&
			strings.Contains(strings.Join(chunkLabels, "\n"), `user="bob"`)
	}, "chunk variants used the wrong mappings: %v", chunkLabels)
}

func TestOrderedConvergingMetricRenames(t *testing.T) {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            metric.a: metric.current
        - rename_metrics:
            metric.b: metric.current
`)
	semconv100 := []byte(`groups:
  - id: metric.metric.a
    type: metric
    metric_name: metric.a
    unit: s
    instrument: histogram
  - id: metric.metric.b
    type: metric
    metric_name: metric.b
    unit: s
    instrument: histogram
`)
	semconv110 := metricSemconv("metric.current", "s", "histogram")

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": schema,
		"1.0.0":         semconv100,
		"1.1.0":         semconv110,
	})
	require.NoError(t, err)
	appendSeries(t, wrapped, "metric.a", 1, 1.0, "source", "a")
	appendSeries(t, wrapped, "metric.b", 1, 2.0, "source", "b")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	got := collectSeries(t, set)
	require.Len(t, got, 2, "both ordered predecessors must resolve to the current metric: %v", got)
	require.Empty(t, warningStrings(set.Warnings()))
}

func TestOrderedConvergingAttributeRenames(t *testing.T) {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              attr.a: attr.current
        - rename_attributes:
            attribute_map:
              attr.b: attr.current
`)
	semconv110 := []byte(`groups:
  - id: metric.metric.current
    type: metric
    metric_name: metric.current
    unit: s
    instrument: histogram
    attributes:
      - ref: attr.current
`)

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": schema,
		"1.1.0":         semconv110,
	})
	require.NoError(t, err)
	appendSeries(t, wrapped, "metric.current", 1, 1.0, "attr.a", "match", "source", "a")
	appendSeries(t, wrapped, "metric.current", 1, 2.0, "attr.b", "match", "source", "b")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, "attr.current", "match"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
	set := q.Select(context.Background(), false, nil, matchers...)
	got := collectSeries(t, set)
	require.Len(t, got, 2, "both ordered attribute predecessors must be queried: %v", got)
	for key := range got {
		require.Contains(t, key, `"attr.current"="match"`)
		require.NotContains(t, key, `"attr.a"=`)
		require.NotContains(t, key, `"attr.b"=`)
	}
	require.Empty(t, warningStrings(set.Warnings()))
}

func TestAmbiguousHistoricalBranchIsRejectedIndependently(t *testing.T) {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            old.bad: metric.current
            old.good: metric.current
`)
	semconv100 := []byte(`groups:
  - id: metric.old.bad.one
    type: metric
    metric_name: old.bad
    unit: "s"
    instrument: histogram
  - id: metric.old.bad.two
    type: metric
    metric_name: old.bad
    unit: "By"
    instrument: counter
  - id: metric.old.good
    type: metric
    metric_name: old.good
    unit: "s"
    instrument: histogram
`)

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": schema,
		"1.0.0":         semconv100,
		"1.1.0":         metricSemconv("metric.current", "s", "histogram"),
	})
	require.NoError(t, err)
	appendSeries(t, wrapped, "old.bad", 1, 1.0, "source", "bad")
	appendSeries(t, wrapped, "old.good", 1, 2.0, "source", "good")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	got := collectSeries(t, set)
	require.Len(t, got, 1, "the ambiguous sibling branch must be excluded: %v", got)
	for key := range got {
		require.Contains(t, key, `source="good"`)
	}
	requireWarningsContain(t, warningStrings(set.Warnings()), "ambiguous rename branch")
}

func TestAmbiguousMetricNameWarns(t *testing.T) {
	// Two groups declaring the same metric_name: the collision that previously
	// resolved to whichever group was parsed last, with no warning.
	ambiguous := []byte(`groups:
  - id: metric.shared.name
    type: metric
    metric_name: shared.name
    unit: "s"
    instrument: histogram
    attributes:
      - ref: http.response.status_code
  - id: metric.shared.name.other
    type: metric
    metric_name: shared.name
    unit: "{item}"
    instrument: updowncounter
    attributes:
      - ref: queue.name
`)

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "shared.name"),
		"1.1.0":         ambiguous,
	})
	require.NoError(t, err)

	appendSeries(t, wrapped, "shared.name", 1, 7.0)
	appendSeries(t, wrapped, "old.name", 1, 9.0)

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "shared.name"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}

	t.Run("Select", func(t *testing.T) {
		set := q.Select(context.Background(), false, nil, matchers...)
		got := collectSeries(t, set)
		require.Len(t, got, 1, "an ambiguous anchor must not traverse to old.name: %v", got)
		for _, value := range got {
			require.Equal(t, 7.0, value)
		}
		requireWarningsContain(t, warningStrings(set.Warnings()), "declared by more than one group")
	})

	t.Run("LabelNames", func(t *testing.T) {
		_, anns, err := q.LabelNames(context.Background(), nil, matchers...)
		require.NoError(t, err)
		requireWarningsContain(t, warningStrings(anns), "declared by more than one group")
	})

	t.Run("LabelValues", func(t *testing.T) {
		_, anns, err := q.LabelValues(context.Background(), "http.response.status_code", nil, matchers...)
		require.NoError(t, err)
		requireWarningsContain(t, warningStrings(anns), "declared by more than one group")
	})

	cq, err := wrapped.ChunkQuerier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cq.Close() })

	t.Run("ChunkSelect", func(t *testing.T) {
		set := cq.Select(context.Background(), false, nil, matchers...)
		var n int
		for set.Next() {
			n++
		}
		require.NoError(t, set.Err())
		require.Equal(t, 1, n, "an ambiguous anchor must use only the direct chunk variant")
		requireWarningsContain(t, warningStrings(set.Warnings()), "declared by more than one group")
	})

	t.Run("ChunkLabelNames", func(t *testing.T) {
		_, anns, err := cq.LabelNames(context.Background(), nil, matchers...)
		require.NoError(t, err)
		requireWarningsContain(t, warningStrings(anns), "declared by more than one group")
	})

	t.Run("ChunkLabelValues", func(t *testing.T) {
		_, anns, err := cq.LabelValues(context.Background(), "http.response.status_code", nil, matchers...)
		require.NoError(t, err)
		requireWarningsContain(t, warningStrings(anns), "declared by more than one group")
	})
}

// TestChunkQuerierSurfacesWarnings checks the ChunkQuerier path annotates too,
// since it fans out through the same resolver as Querier.
func TestChunkQuerierSurfacesWarnings(t *testing.T) {
	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
		"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
		"1.0.0":         metricSemconv("old.name", "{item}", "updowncounter"),
		"1.1.0":         metricSemconv("new.name", "s", "histogram"),
	})
	require.NoError(t, err)
	appendSeries(t, wrapped, "new.name", 1, 7.0)

	cq, err := wrapped.ChunkQuerier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cq.Close() })

	set := cq.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "new.name"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	var n int
	for set.Next() {
		n++
	}
	require.NoError(t, set.Err())
	require.Equal(t, 1, n, "the queried metric's own series must still be returned")

	var found bool
	for _, w := range warningStrings(set.Warnings()) {
		if strings.Contains(w, "treating them as different metrics") {
			found = true
		}
	}
	require.True(t, found, "expected the chunk querier to surface the mis-linked rename warning")
}

// recycledSchema retires old.name at 1.1.0 and hands the name to an unrelated
// metric at 1.2.0, so the name has two eras and they need not mean the same thing.
const recycledSchema = `file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            old.name: new.name
  1.2.0:
    metrics:
      changes:
        - rename_metrics:
            other.name: old.name
`

// TestRenameValidationWithoutAnchorDeclaration covers querying a name the anchor
// semconv does not declare, which is ordinary input here: the fan-out deliberately
// supports asking for a name the anchor version has already renamed away.
//
// Corroboration used to switch itself off silently for exactly that query, which is
// the case it exists for. The identity of the queried metric is instead taken from a
// version where the name is the current one, and only where no version settles it is
// the check skipped — and then said so.
func TestRenameValidationWithoutAnchorDeclaration(t *testing.T) {
	t.Run("corroborates against the era that declares the queried name", func(t *testing.T) {
		// Semconv 1.1.0 knows only new.name, so the queried old.name is resolved
		// against 1.0.0, where it is current. The two disagree on unit and
		// instrument, so the rename joins unrelated metrics and must not merge.
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
			"1.0.0":         metricSemconv("old.name", "{item}", "updowncounter"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "new.name", "old.name")

		require.Empty(t, got, "the contradicted rename must not pull in the other metric's series, got %v", got)
		requireWarningsContain(t, warnings, "treating them as different metrics")
	})

	t.Run("merges when the era that declares the queried name agrees", func(t *testing.T) {
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
			"1.0.0":         metricSemconv("old.name", "s", "histogram"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "new.name", "old.name")

		require.Len(t, got, 1, "expected the renamed series under the queried name, got %v", got)
		for k := range got {
			require.Contains(t, k, `__name__="old.name"`)
		}
		require.Empty(t, warnings, "a corroborated rename must not warn")
	})

	t.Run("warns when no version declares the queried name", func(t *testing.T) {
		// Nothing declares old.name anywhere, so there is no identity to check
		// hops against. The series still merge, but the caller is told the
		// corroboration did not run rather than left to assume it passed.
		got, warnings := selectRenamed(t, map[string][]byte{
			"registry.yaml": fmt.Appendf(nil, renameSchema, "old.name", "new.name"),
			"1.0.0":         metricSemconv("unrelated.name", "s", "histogram"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
		}, "new.name", "old.name")

		require.Len(t, got, 1, "an unverifiable rename is still followed, got %v", got)
		requireWarningsContain(t, warnings, "without corroboration")
	})

	t.Run("rejects when a disconnected identity reuses the queried name", func(t *testing.T) {
		// old.name is current at 1.0.0 and again at 1.2.0, as two different
		// metrics. Neither era can stand in for the other, so picking one would
		// check hops against an identity the caller never asked for.
		err := selectRenamedError(t, map[string][]byte{
			"registry.yaml": []byte(recycledSchema),
			"1.0.0":         metricSemconv("old.name", "{item}", "updowncounter"),
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
			"1.2.0":         metricSemconv("old.name", "s", "histogram"),
		}, "new.name", "old.name")

		require.ErrorContains(t, err, "semconv schema rename is ambiguous")
	})

	t.Run("rejects reuse before checking an ambiguous era declaration", func(t *testing.T) {
		ambiguous := []byte(`groups:
  - id: metric.old.name.one
    type: metric
    metric_name: old.name
    unit: s
    instrument: histogram
  - id: metric.old.name.two
    type: metric
    metric_name: old.name
    unit: By
    instrument: counter
`)
		err := selectRenamedError(t, map[string][]byte{
			"registry.yaml": []byte(recycledSchema),
			"1.0.0":         ambiguous,
			"1.1.0":         metricSemconv("new.name", "s", "histogram"),
			"1.2.0":         metricSemconv("old.name", "s", "histogram"),
		}, "new.name", "old.name")

		require.ErrorContains(t, err, "semconv schema rename is ambiguous")
	})
}
