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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/util/teststorage"
)

func inheritedAttributeRegistry() map[string][]byte {
	return map[string][]byte{
		"registry.yaml": []byte(`
file_format: 1.1.0
schema_url: https://example.com/schemas/1.0.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              legacy.user: tenant.id
              legacy.partition: tenant.partition
            apply_to_metrics:
              - queue.depth
`),
		"1.0.0": []byte(`
groups:
  - id: attributes.identity
    type: attribute_group
    prefix: legacy
    attributes:
      - id: user
  - id: attributes.queue.base
    type: attribute_group
    extends: attributes.identity
  - id: attributes.queue
    type: attribute_group
    extends: attributes.queue.base
    prefix: legacy
    attributes:
      - id: partition
  - id: metric.queue.depth
    type: metric
    metric_name: queue.depth
    instrument: updowncounter
    unit: "{item}"
    extends: attributes.queue
`),
		"1.1.0": []byte(`
groups:
  - id: metric.queue.depth
    type: metric
    metric_name: queue.depth
    instrument: updowncounter
    unit: "{item}"
    extends: attributes.queue
  - id: attributes.queue
    type: attribute_group
    extends: attributes.queue.base
    prefix: tenant
    attributes:
      - id: partition
  - id: attributes.identity
    type: attribute_group
    prefix: tenant
    attributes:
      - id: id
  - id: attributes.queue.base
    type: attribute_group
    extends: attributes.identity
`),
	}
}

func TestInheritedMetricAttributesAreCanonicalized(t *testing.T) {
	wrapped, err := semconv.AwareStorageWithRegistry(teststorage.New(t), inheritedAttributeRegistry())
	require.NoError(t, err)
	appendSeries(t, wrapped, "queue.depth", 1, 7, "legacy.user", "alice", "legacy.partition", "primary")

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "queue.depth"),
		labels.MustNewMatcher(labels.MatchEqual, "tenant.id", "alice"),
		labels.MustNewMatcher(labels.MatchEqual, "tenant.partition", "primary"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil, matchers...)
	series := collectSeries(t, set)
	require.Len(t, series, 1)
	for key := range series {
		require.Contains(t, key, `"tenant.id"="alice"`)
		require.Contains(t, key, `"tenant.partition"="primary"`)
		require.NotContains(t, key, "legacy.user")
		require.NotContains(t, key, "legacy.partition")
	}
	require.Empty(t, warningStrings(set.Warnings()))

	names, warnings, err := q.LabelNames(context.Background(), nil, matchers...)
	require.NoError(t, err)
	require.Contains(t, names, "tenant.id")
	require.Contains(t, names, "tenant.partition")
	require.NotContains(t, names, "legacy.user")
	require.NotContains(t, names, "legacy.partition")
	require.Empty(t, warningStrings(warnings))

	values, warnings, err := q.LabelValues(context.Background(), "tenant.id", nil, matchers...)
	require.NoError(t, err)
	require.Equal(t, []string{"alice"}, values)
	require.Empty(t, warningStrings(warnings))

	values, warnings, err = q.LabelValues(context.Background(), "tenant.partition", nil, matchers...)
	require.NoError(t, err)
	require.Equal(t, []string{"primary"}, values)
	require.Empty(t, warningStrings(warnings))

	cq, err := wrapped.ChunkQuerier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cq.Close() })

	chunkSet := cq.Select(context.Background(), false, nil, matchers...)
	require.True(t, chunkSet.Next())
	require.Equal(t, "alice", chunkSet.At().Labels().Get("tenant.id"))
	require.Equal(t, "primary", chunkSet.At().Labels().Get("tenant.partition"))
	require.Empty(t, chunkSet.At().Labels().Get("legacy.user"))
	require.Empty(t, chunkSet.At().Labels().Get("legacy.partition"))
	require.False(t, chunkSet.Next())
	require.NoError(t, chunkSet.Err())
	require.Empty(t, warningStrings(chunkSet.Warnings()))

	names, warnings, err = cq.LabelNames(context.Background(), nil, matchers...)
	require.NoError(t, err)
	require.Contains(t, names, "tenant.id")
	require.Contains(t, names, "tenant.partition")
	require.NotContains(t, names, "legacy.user")
	require.NotContains(t, names, "legacy.partition")
	require.Empty(t, warningStrings(warnings))

	values, warnings, err = cq.LabelValues(context.Background(), "tenant.id", nil, matchers...)
	require.NoError(t, err)
	require.Equal(t, []string{"alice"}, values)
	require.Empty(t, warningStrings(warnings))

	values, warnings, err = cq.LabelValues(context.Background(), "tenant.partition", nil, matchers...)
	require.NoError(t, err)
	require.Equal(t, []string{"primary"}, values)
	require.Empty(t, warningStrings(warnings))
}
