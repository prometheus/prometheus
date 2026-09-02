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
	"github.com/prometheus/prometheus/storage"
)

// selectAt runs a fanned-out Select against the embedded registry, anchored at
// semconv version anchor.
func selectAt(t *testing.T, s storage.Storage, anchor, metricName string) storage.SeriesSet {
	t.Helper()
	q, err := s.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	return q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metricName),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/"+anchor),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
}

// TestFanOutReachesEveryEraFromAnyAnchor walks the embedded registry's rename
// chain (test.counter → test at 1.1.0 → test.v2 at 1.2.0) from valid anchors.
//
// Anchoring at the newest version happens to work with a truncated walk, because
// then every rename is on the backward side; the oldest and middle anchors are what
// expose a walk that mis-slices the version history or fails to chain across the
// anchor. Each anchor must reach all three eras.
func TestFanOutReachesEveryEraFromAnyAnchor(t *testing.T) {
	// eras names the attribute each era's series is expected to carry in the
	// result. Ordered schema changes determine the anchor-era name even when the
	// anchor semconv does not declare attributes for the queried metric.
	eras := func(attrs ...string) []string { return attrs }

	for _, tc := range []struct {
		name        string
		anchor      string
		queried     string
		attrs       []string
		description string
	}{
		{
			name:        "oldest anchor",
			anchor:      "1.0.0",
			queried:     "test.counter",
			attrs:       eras("user", "user", "user"),
			description: "no version is at or before 1.0.0, so the whole chain has to be walked forward",
		},
		{
			name:        "middle anchor",
			anchor:      "1.1.0",
			queried:     "test",
			attrs:       eras("tenant", "tenant", "tenant"),
			description: "1.1.0 is walked backward and 1.2.0 forward",
		},
		{
			name:        "newest anchor",
			anchor:      "1.2.0",
			queried:     "test.v2",
			attrs:       eras("tenant", "tenant", "tenant"),
			description: "every rename is on the backward side",
		},
		{
			name:    "middle anchor with a name that version renamed away",
			anchor:  "1.1.0",
			queried: "test.counter",
			attrs:   eras("user", "user", "user"),
			description: "test.counter is the name 1.1.0 renamed, so the backward walk crosses " +
				"that rename and the forward walk has to continue from where it landed",
		},
		{
			name:        "newest anchor with the name that version renamed away",
			anchor:      "1.2.0",
			queried:     "test",
			attrs:       eras("tenant", "tenant", "tenant"),
			description: "the retirement revision is oriented forward when it is the exact anchor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapped, _ := newAwareStorage(t)
			// One series per naming era, distinguished by attribute value so the
			// three stay distinct series rather than merging into one.
			appendSeries(t, wrapped, "test.counter", 1, 1.0, "user", "a")
			appendSeries(t, wrapped, "test", 1, 2.0, "tenant", "b")
			appendSeries(t, wrapped, "test.v2", 1, 3.0, "tenant", "c")

			got := collectSeries(t, selectAt(t, wrapped, tc.anchor, tc.queried))

			require.Len(t, got, 3, "%s: got %v", tc.description, got)
			for i, value := range []string{"a", "b", "c"} {
				require.Contains(t, got,
					labels.FromStrings(model.MetricNameLabel, tc.queried, tc.attrs[i], value).String())
			}
		})
	}
}

func TestFanOutStopsAtRetirementBeforeLaterAnchor(t *testing.T) {
	wrapped, _ := newAwareStorage(t)
	appendSeries(t, wrapped, "test.counter", 1, 1.0, "user", "a")
	appendSeries(t, wrapped, "test", 1, 2.0, "tenant", "b")
	appendSeries(t, wrapped, "test.v2", 1, 3.0, "tenant", "c")

	set := selectAt(t, wrapped, "1.2.0", "test.counter")
	got := collectSeries(t, set)

	require.Equal(t, map[string]float64{
		labels.FromStrings(model.MetricNameLabel, "test.counter", "user", "a").String(): 1,
	}, got)
	requireWarningsContain(t, warningStrings(set.Warnings()), "metric lifecycle boundary")
}
