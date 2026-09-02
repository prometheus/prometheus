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
)

// TestLabelValuesCanonicalisesTheQueriedName asks for the values of a renamed
// attribute under its pre-rename name. The name is canonicalised first, so the
// fan-out covers the same aliases as asking under the anchor-version name;
// otherwise only the era that happens to store the attribute under the name as
// given contributes any values.
func TestLabelValuesCanonicalisesTheQueriedName(t *testing.T) {
	wrapped, _ := newAwareStorage(t)
	appendSeries(t, wrapped, "test.counter", 1, 1.0, "user", "a")
	appendSeries(t, wrapped, "test", 1, 2.0, "tenant", "b")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
	// "user" is the 1.0.0 name of the attribute the 1.1.0 anchor calls "tenant".
	values, _, err := q.LabelValues(context.Background(), "user", nil, matchers...)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, values)
}

// TestLabelNamesHidesReservedLabels checks that the reserved matcher labels are
// not reported as label names. Select strips them from the series it returns, so
// reporting them would advertise labels no series carries.
func TestLabelNamesHidesReservedLabels(t *testing.T) {
	wrapped, _ := newAwareStorage(t)
	appendSeries(t, wrapped, "test", 1, 1.0, "tenant", "a", "__schema_url__", "registry/registry.yaml")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	names, _, err := q.LabelNames(context.Background(), nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	require.NoError(t, err)
	require.Equal(t, []string{model.MetricNameLabel, "tenant"}, names)
}

func TestLabelValuesHidesReservedLabels(t *testing.T) {
	wrapped, counts := newCountingAwareStorage(t, canonicalLabelRegistry())
	appendSeries(t, wrapped, "jvm.thread.count", 1, 1.0,
		"__schema_url__", "stored-schema",
		"__semconv_url__", "stored-semconv",
	)

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	cq, err := wrapped.ChunkQuerier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cq.Close() })

	for _, tc := range []struct {
		name  string
		query func(string) ([]string, error)
	}{
		{
			name: "querier",
			query: func(name string) ([]string, error) {
				values, _, err := q.LabelValues(context.Background(), name, nil, canonicalLabelMatchers()...)
				return values, err
			},
		},
		{
			name: "chunk querier",
			query: func(name string) ([]string, error) {
				values, _, err := cq.LabelValues(context.Background(), name, nil, canonicalLabelMatchers()...)
				return values, err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range []string{"__schema_url__", "__semconv_url__"} {
				counts.reset()
				values, err := tc.query(name)
				require.NoError(t, err)
				require.NotNil(t, values)
				require.Empty(t, values)
				require.Zero(t, counts.total(), "reserved values must not schedule storage jobs")
			}
		})
	}

	counts.reset()
	values, _, err := q.LabelValues(context.Background(), "__schema_url__", nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "jvm.thread.count"),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"stored-schema"}, values, "ordinary queries must retain passthrough behavior")
	require.Equal(t, int64(1), counts.total())
}
