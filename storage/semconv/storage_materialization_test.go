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
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

var materializationTestRegistry = map[string][]byte{
	"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            name_map:
              budget.old: budget.current
        - rename_attributes:
            attribute_map:
              attr.old: attr.current
            apply_to_metrics:
              - budget.current
  1.2.0:
    metrics:
      changes:
        - rename_metrics:
            name_map:
              budget.current: budget.future
        - rename_attributes:
            attribute_map:
              attr.current: attr.future
            apply_to_metrics:
              - budget.future
`),
	"1.0.0": []byte(`groups:
  - id: registry.attr.old
    type: attribute_group
    brief: Old attribute.
    attributes:
      - id: attr.old
        type: string
        brief: Old attribute.
        examples: [value]
  - id: metric.budget.old
    type: metric
    brief: Old metric.
    metric_name: budget.old
    instrument: counter
    unit: "1"
    attributes:
      - ref: attr.old
`),
	"1.1.0": []byte(`groups:
  - id: registry.attr.current
    type: attribute_group
    brief: Current attribute.
    attributes:
      - id: attr.current
        type: string
        brief: Current attribute.
        examples: [value]
  - id: metric.budget.current
    type: metric
    brief: Current metric.
    metric_name: budget.current
    instrument: counter
    unit: "1"
    attributes:
      - ref: attr.current
`),
}

func newMaterializationTestStorage(t *testing.T, limit, oldSeries, futureSeries int) *awareStorage {
	t.Helper()
	underlying := teststorage.New(t)
	wrapped := newAwareStorage(underlying, newSchemaEngine(newRegistrySource(materializationTestRegistry)))
	wrapped.canonicalSeriesLimit = limit
	app := wrapped.Appender(context.Background())
	appendSeries := func(metric, attribute string, index int) {
		labelValues := []string{
			model.MetricNameLabel, metric,
			attribute, "value",
			"instance", fmt.Sprintf("%03d", index),
		}
		_, err := app.Append(0, labels.FromStrings(labelValues...), 1, float64(index))
		require.NoError(t, err)
	}
	for i := range oldSeries {
		appendSeries("budget.old", "attr.old", i)
	}
	for i := range futureSeries {
		appendSeries("budget.future", "attr.future", oldSeries+i)
	}
	require.NoError(t, app.Commit())
	variants, _, err := wrapped.engine.findMatcherVariants("registry/1.1.0", "registry/registry.yaml", materializationTestMatchers())
	require.NoError(t, err)
	require.Len(t, variants, 3)
	var names []string
	for _, variant := range variants {
		name, err := extractMetricName(variant.matchers)
		require.NoError(t, err)
		names = append(names, name)
	}
	require.ElementsMatch(t, []string{"budget.old", "budget.current", "budget.future"}, names)
	return wrapped
}

func materializationTestMatchers() []*labels.Matcher {
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "budget.current"),
		labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "registry/registry.yaml"),
	}
}

func TestCanonicalSeriesMaterializationBudget(t *testing.T) {
	for _, query := range []string{"series", "chunks"} {
		t.Run(query+" allows exact limit", func(t *testing.T) {
			wrapped := newMaterializationTestStorage(t, 2, 1, 1)
			if query == "series" {
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })
				set := q.Select(context.Background(), false, nil, materializationTestMatchers()...)
				for set.Next() {
				}
				require.NoError(t, set.Err())
				return
			}

			q, err := wrapped.ChunkQuerier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })
			set := q.Select(context.Background(), false, nil, materializationTestMatchers()...)
			for set.Next() {
			}
			require.NoError(t, set.Err())
		})

		t.Run(query+" rejects the next cross-variant input", func(t *testing.T) {
			wrapped := newMaterializationTestStorage(t, 2, 2, 1)
			hints := &storage.SelectHints{Start: 0, End: 10, Limit: 1}
			if query == "series" {
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })
				set := q.Select(context.Background(), false, hints, materializationTestMatchers()...)
				require.False(t, set.Next())
				require.ErrorIs(t, set.Err(), errCanonicalSeriesMaterialization)
				require.ErrorContains(t, set.Err(), "more than 2 input series")
				return
			}

			q, err := wrapped.ChunkQuerier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })
			set := q.Select(context.Background(), false, hints, materializationTestMatchers()...)
			require.False(t, set.Next())
			require.ErrorIs(t, set.Err(), errCanonicalSeriesMaterialization)
			require.ErrorContains(t, set.Err(), "more than 2 input series")
		})
	}
}
