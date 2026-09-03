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
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/util/teststorage"
)

// The files under testdata/upstream contain a real OTel schema and verbatim
// semantic-convention group excerpts pinned to their upstream commits.
// Hand-written fixtures are free to encode whatever shape the parser happens to
// expect, which is how rename_metrics came to be read from a name_map key that
// the file format does not have: every fixture agreed with the bug, so the tests
// passed while metric renames silently did nothing on real data. These tests
// exist so the real file format is what the parser is held to.
//
// Attribute-rename scoping is deliberately not covered from these artefacts. Real
// http metric groups declare their attributes with extends (see
// TestUpstreamSemconvAttributes), which the loader does not resolve, so such a
// metric has no attributes to rename and any assertion about scoping here would
// hold no matter what the scoping code did. The hand-written fixtures in
// storage_rename_validation_test.go and otel_schema_test.go cover it instead.
const (
	upstreamSchema        = "./testdata/upstream/schema-1.44.0.yaml"
	upstreamSemconv1_21_0 = "./testdata/upstream/semconv-1.21.0.yaml"
	upstreamSemconv1_22_0 = "./testdata/upstream/semconv-1.22.0.yaml"
	upstreamSemconv1_43_0 = "./testdata/upstream/semconv-1.43.0.yaml"
	upstreamSemconv1_44_0 = "./testdata/upstream/semconv-1.44.0.yaml"
)

// This edge is deliberately synthetic: its endpoints are unrelated stable
// metrics whose definitions below are verbatim upstream data.
const upstreamStableContradictionSchema = `file_format: 1.1.0
schema_url: https://example.com/schemas/1.44.0
versions:
  1.43.0:
  1.44.0:
    metrics:
      changes:
        - rename_metrics:
            jvm.class.loaded: jvm.cpu.count
`

// upstreamRegistry assembles the real artefacts into a registry, keyed the way
// AwareStorageWithRegistry expects: semver base names for semconv files, anything
// else for the schema.
func upstreamRegistry(t *testing.T) map[string][]byte {
	t.Helper()
	read := func(path string) []byte {
		b, err := os.ReadFile(filepath.Clean(path))
		require.NoError(t, err)
		return b
	}
	return map[string][]byte{
		"registry.yaml": read(upstreamSchema),
		"1.21.0":        read(upstreamSemconv1_21_0),
		"1.22.0":        read(upstreamSemconv1_22_0),
		"1.43.0":        read(upstreamSemconv1_43_0),
		"1.44.0":        read(upstreamSemconv1_44_0),
	}
}

// TestUpstreamSchemaMetricRenames guards the file format itself. Reading
// rename_metrics from a nested name_map key yielded zero metric renames from
// every real schema while all fixtures still passed, so assert against the real
// artefact that metric renames are found at all.
func TestUpstreamSchemaMetricRenames(t *testing.T) {
	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, upstreamRegistry(t))
	require.NoError(t, err)

	// http.server.duration was renamed to http.server.request.duration in
	// semconv 1.22.0, the canonical example of a metric rename. Write the series
	// under its pre-rename name and query it under the post-rename one.
	appendSeries(t, wrapped, "http.server.duration", 1, 7.0, "http.response.status_code", "200")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.request.duration"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.22.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	got := collectSeries(t, set)
	require.Len(t, got, 1,
		"expected the pre-rename series to surface under the queried name; zero means rename_metrics did not parse, got %v", got)
	for k := range got {
		require.Contains(t, k, `__name__="http.server.request.duration"`)
	}

	// Both versions declare the metric as s/histogram, so the metric rename is
	// corroborated. Later schema revisions may independently report ambiguous
	// attribute histories, but none of the warnings may concern metric lineage.
	for _, warning := range warningStrings(set.Warnings()) {
		require.Contains(t, warning, "attribute name",
			"a rename corroborated by the real semconv files must not raise a metric warning")
	}
}

func TestUpstreamExperimentalMetricRenameMayChangeUnit(t *testing.T) {
	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, upstreamRegistry(t))
	require.NoError(t, err)

	appendSeries(t, wrapped, "process.runtime.jvm.system.cpu.load_1m", 1, 7.0)

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "jvm.system.cpu.load_1m"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.22.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	got := collectSeries(t, set)
	require.Len(t, got, 1, "the upstream experimental rename must retain its historical series: %v", got)
	for key := range got {
		require.Contains(t, key, `__name__="jvm.system.cpu.load_1m"`)
	}
	requireWarningsContain(t, warningStrings(set.Warnings()), "following the explicit schema rename")
}

func TestUpstreamStableMetricContradiction(t *testing.T) {
	registry := upstreamRegistry(t)
	registry["stable-contradiction.yaml"] = []byte(upstreamStableContradictionSchema)

	underlying := teststorage.New(t)
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, registry)
	require.NoError(t, err)

	appendSeries(t, wrapped, "jvm.class.loaded", 1, 7.0)

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "jvm.cpu.count"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.44.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/stable-contradiction.yaml"),
	)
	got := collectSeries(t, set)
	require.Empty(t, got, "stable definitions with conflicting units and instruments must not merge: %v", got)
	requireWarningsContain(t, warningStrings(set.Warnings()), "treating them as different metrics")
	requireWarningsContain(t, warningStrings(set.Warnings()), `resolves it to "jvm.class.loaded"`)
}
