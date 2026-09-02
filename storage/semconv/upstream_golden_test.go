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

// The files under testdata/upstream are real artefacts from
// open-telemetry/semantic-conventions v1.44.0 (commit e10a930), kept unmodified.
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
)

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
}
