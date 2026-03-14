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

package promql_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/teststorage"
)

// newNativeMetadataStorage creates a TestStorage with native metadata enabled.
func newNativeMetadataStorage(t *testing.T) *teststorage.TestStorage {
	t.Helper()
	return teststorage.New(t, func(opts *tsdb.Options) {
		opts.EnableNativeMetadata = true
	})
}

// newNativeMetadataEngine creates a PromQL engine with native metadata enabled
// and the resource-attributes strategy.
func newNativeMetadataEngine(t *testing.T) *promql.Engine {
	t.Helper()
	return newEngineWithStrategy(t, promql.InfoResourceStrategyResourceAttributes)
}

// newEngineWithStrategy creates a PromQL engine with native metadata enabled and
// the given InfoResourceStrategy.
func newEngineWithStrategy(t *testing.T, strategy promql.InfoResourceStrategy) *promql.Engine {
	t.Helper()
	return promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
		MaxSamples:           50000,
		Timeout:              10 * time.Second,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		EnableNativeMetadata: true,
		InfoResourceStrategy: strategy,
		Parser:               parser.NewParser(promqltest.TestParserOpts),
	})
}

// appendWithResource appends float samples with a resource context via AppenderV2.
func appendWithResource(
	t *testing.T, stor *teststorage.TestStorage,
	ls labels.Labels, timestamps []int64, values []float64,
	rc *storage.ResourceContext,
) {
	t.Helper()
	ctx := context.Background()
	app := stor.AppenderV2(ctx)
	var ref storage.SeriesRef
	for i, ts := range timestamps {
		var err error
		ref, err = app.Append(ref, ls, 0, ts, values[i], nil, nil, storage.AOptions{
			Resource: rc,
		})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
}

// appendSamples appends float samples via the V1 Appender (no resource context).
func appendSamples(
	t *testing.T, stor *teststorage.TestStorage,
	ls labels.Labels, timestamps []int64, values []float64,
) {
	t.Helper()
	ctx := context.Background()
	app := stor.Appender(ctx)
	var ref storage.SeriesRef
	for i, ts := range timestamps {
		var err error
		ref, err = app.Append(ref, ls, ts, values[i])
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
}

// execRangeQuery executes a range query and returns the sorted result matrix.
func execRangeQuery(
	t *testing.T, engine *promql.Engine, store storage.Queryable,
	query string, start, end time.Time, step time.Duration,
) promql.Matrix {
	t.Helper()
	ctx := context.Background()
	qry, err := engine.NewRangeQuery(ctx, store, nil, query, start, end, step)
	require.NoError(t, err)
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	mat, err := res.Matrix()
	require.NoError(t, err)
	sort.Sort(mat)
	return mat
}

// execRangeQueryErr executes a range query and returns the query error (if any).
func execRangeQueryErr(
	t *testing.T, engine *promql.Engine, store storage.Queryable,
	query string, start, end time.Time, step time.Duration,
) error {
	t.Helper()
	ctx := context.Background()
	qry, err := engine.NewRangeQuery(ctx, store, nil, query, start, end, step)
	require.NoError(t, err)
	res := qry.Exec(ctx)
	return res.Err
}

// timestamps and values for the default 3-step range used by most subtests.
// Timestamps: 60s, 120s, 180s.
var (
	defaultTimestamps = []int64{60_000, 120_000, 180_000}
	defaultValues     = []float64{1, 2, 3}
	defaultStart      = time.Unix(60, 0)
	defaultEnd        = time.Unix(180, 0)
	defaultStep       = time.Minute
)

func TestInfoNativeMetadata(t *testing.T) {
	// ---------------------------------------------------------------
	// Native metadata path tests (resource attributes from TSDB)
	// ---------------------------------------------------------------

	t.Run("all resource attrs", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Identifying: map[string]string{"service.name": "myapp"},
			Descriptive: map[string]string{"host.name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Base labels + both resource attrs
		require.Equal(t, "host1", mat[0].Metric.Get("host.name"))
		require.Equal(t, "myapp", mat[0].Metric.Get("service.name"))
		require.Len(t, mat[0].Floats, 3)
	})

	t.Run("filtered by one attr", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Identifying: map[string]string{"service_name": "myapp"},
			Descriptive: map[string]string{"host_name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		mat := execRangeQuery(t, engine, stor, `info(metric, {host_name=~".+"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Only the filtered attr is added
		require.Equal(t, "host1", mat[0].Metric.Get("host_name"))
		// service_name is NOT added because it's not in the filter
		require.Empty(t, mat[0].Metric.Get("service_name"))
	})

	t.Run("filter requires non-empty attr missing", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Identifying: map[string]string{"service.name": "myapp"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// non_existent doesn't exist in resource attrs, filter requires non-empty → no results
		mat := execRangeQuery(t, engine, stor, `info(metric, {non_existent=~".+"})`, defaultStart, defaultEnd, defaultStep)
		require.Empty(t, mat)
	})

	t.Run("filter allows empty and non-empty combo", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host_name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// host_name=~".+" requires non-empty (matched), non_existent=~".*" allows empty
		mat := execRangeQuery(t, engine, stor, `info(metric, {host_name=~".+", non_existent=~".*"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "host1", mat[0].Metric.Get("host_name"))
	})

	t.Run("overlapping labels base wins", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// Resource has "job" which also exists as a base label → base wins
		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"job": "from_resource", "host.name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric_with_overlap", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		mat := execRangeQuery(t, engine, stor, `info(metric_with_overlap)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// "job" should keep the base value
		require.Equal(t, "j1", mat[0].Metric.Get("job"))
		// "host.name" is added from resource
		require.Equal(t, "host1", mat[0].Metric.Get("host.name"))
	})

	t.Run("overlapping label filtered", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"job": "from_resource", "host_name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric_with_overlap", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// Filter specifically for host_name, which exists in resource
		mat := execRangeQuery(t, engine, stor, `info(metric_with_overlap, {host_name=~".+"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "host1", mat[0].Metric.Get("host_name"))
		// "job" from resource is NOT added (not in the filter)
		require.Equal(t, "j1", mat[0].Metric.Get("job"))
	})

	t.Run("explicit __name__=target_info", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Identifying: map[string]string{"service.name": "myapp"},
			Descriptive: map[string]string{"host.name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// Explicit __name__="target_info" should still use native metadata path
		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__="target_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "host1", mat[0].Metric.Get("host.name"))
		require.Equal(t, "myapp", mat[0].Metric.Get("service.name"))
	})

	t.Run("no resource attributes", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// Append via V1 appender (no resource context)
		ls := labels.FromStrings("__name__", "metric_no_resource", "job", "j1", "instance", "i1")
		appendSamples(t, stor, ls, defaultTimestamps, defaultValues)

		mat := execRangeQuery(t, engine, stor, `info(metric_no_resource)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Series should be returned unchanged
		require.Equal(t, "j1", mat[0].Metric.Get("job"))
		require.Len(t, mat[0].Floats, 3)
	})

	t.Run("resource version change", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")

		// First version: host1
		rc1 := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host1"},
		}
		appendWithResource(t, stor, ls, []int64{60_000, 120_000}, []float64{1, 2}, rc1)

		// Second version: host2 (at a later timestamp)
		rc2 := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host2"},
		}
		appendWithResource(t, stor, ls, []int64{180_000}, []float64{3}, rc2)

		// Query across the version change
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		// Should produce 2 series due to different label sets
		require.Len(t, mat, 2)

		// Collect host.name values across all series
		hostNames := map[string]bool{}
		for _, s := range mat {
			hostNames[s.Metric.Get("host.name")] = true
		}
		require.True(t, hostNames["host1"])
		require.True(t, hostNames["host2"])
	})

	t.Run("lookback for resource", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")

		// Only samples at 60s and 180s → gap at 120s
		appendWithResource(t, stor, ls, []int64{60_000, 180_000}, []float64{1, 3}, rc)

		// Query at 120s — lookback should find the 60s sample
		mat := execRangeQuery(t, engine, stor, `info(metric)`,
			time.Unix(120, 0), time.Unix(120, 0), defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "host1", mat[0].Metric.Get("host.name"))
	})

	t.Run("@ modifier", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// Use @ modifier to pin the evaluation at 60s
		mat := execRangeQuery(t, engine, stor, `info(metric @ 60)`,
			time.Unix(60, 0), time.Unix(180, 0), defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "host1", mat[0].Metric.Get("host.name"))
		// All points should have value 1 (pinned at 60s)
		for _, p := range mat[0].Floats {
			require.Equal(t, float64(1), p.F)
		}
	})

	t.Run("offset modifier", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host1"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// offset 1m shifts the query window back
		mat := execRangeQuery(t, engine, stor, `info(metric offset 1m)`,
			time.Unix(120, 0), time.Unix(240, 0), defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "host1", mat[0].Metric.Get("host.name"))
	})

	t.Run("multiple series different resources", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc1 := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host1"},
		}
		rc2 := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host2"},
		}
		ls1 := labels.FromStrings("__name__", "data_metric", "job", "j1", "instance", "i1")
		ls2 := labels.FromStrings("__name__", "data_metric", "job", "j2", "instance", "i2")
		appendWithResource(t, stor, ls1, defaultTimestamps, defaultValues, rc1)
		appendWithResource(t, stor, ls2, defaultTimestamps, defaultValues, rc2)

		mat := execRangeQuery(t, engine, stor, `info(data_metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 2)

		hostNames := map[string]bool{}
		for _, s := range mat {
			hostNames[s.Metric.Get("host.name")] = true
		}
		require.True(t, hostNames["host1"])
		require.True(t, hostNames["host2"])
	})

	// ---------------------------------------------------------------
	// Fallback to metric join (new behavior: native metadata enabled
	// but __name__ routes through evalInfoTargetInfo)
	// ---------------------------------------------------------------

	t.Run("__name__=build_info falls back to metric join", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// Append resource attrs (these should NOT be used for build_info join)
		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// Append an actual build_info series
		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__="build_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Should get "version" from the build_info metric join, NOT "host.name" from resource
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
		require.Empty(t, mat[0].Metric.Get("host.name"))
	})

	t.Run("__name__ regex uses hybrid mode", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// Append build_info and target_info metrics
		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// Regex __name__=~".+_info" matches both target_info and build_info → hybrid mode.
		// Native metadata handles target_info portion (resource attrs), metric-join handles build_info.
		// The actual target_info metric in storage is excluded from metric-join.
		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__=~".+_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// "version" comes from build_info metric-join
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
		// "host.name" comes from native metadata (resource attrs), NOT "region" from target_info metric
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		require.Empty(t, mat[0].Metric.Get("region"))
	})

	t.Run("info metric excluded from enrichment", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// build_info is the base metric AND matches the info selector
		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// Query: info(build_info, {__name__=~".+_info"})
		// build_info matches the info metric name selector, so it should be
		// returned unchanged (not enriched with itself)
		mat := execRangeQuery(t, engine, stor, `info(build_info, {__name__=~".+_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
		require.Len(t, mat[0].Floats, 3)
	})

	t.Run("__name__=non_existent falls back to metric join", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host_name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// __name__="non_existent" → falls back to metric join, no such info series exists.
		// With no data-label matchers (only __name__), the base series passes through unchanged.
		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__="non_existent"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Series returned unchanged (no enrichment from resource or info metric)
		require.Empty(t, mat[0].Metric.Get("host_name"))
		require.Equal(t, "j1", mat[0].Metric.Get("job"))
	})

	// ---------------------------------------------------------------
	// Hybrid mode tests (native metadata + metric-join combined)
	// ---------------------------------------------------------------

	t.Run("hybrid explicit target_info|build_info", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// Explicit alternation: __name__=~"target_info|build_info" → hybrid
		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__=~"target_info|build_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Native metadata: host.name from resource attrs
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		// Metric-join: version from build_info
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
	})

	t.Run("hybrid no build_info metric exists", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// No build_info metric appended. Hybrid mode: native metadata enriches,
		// metric-join finds nothing → series has only resource attrs.
		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__=~"target_info|build_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		require.Empty(t, mat[0].Metric.Get("version"))
	})

	t.Run("hybrid no resource attrs", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// Append via V1 (no resource context)
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendSamples(t, stor, ls, defaultTimestamps, defaultValues)

		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// No resource attrs → native metadata does nothing, metric-join adds build_info labels
		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__=~"target_info|build_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
		require.Empty(t, mat[0].Metric.Get("host.name"))
	})

	t.Run("hybrid target_info metric excluded from join", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// Append a target_info metric with a label that differs from resource attrs.
		// In hybrid mode this metric should NOT be used — native metadata replaces it.
		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__=~"target_info|build_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Native metadata: host.name from resource attrs
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		// The target_info metric's "region" should NOT appear (excluded from metric-join)
		require.Empty(t, mat[0].Metric.Get("region"))
	})

	t.Run("hybrid multiple series different resources", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		rc1 := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host1"},
		}
		rc2 := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "host2"},
		}
		ls1 := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		ls2 := labels.FromStrings("__name__", "metric", "job", "j2", "instance", "i2")
		appendWithResource(t, stor, ls1, defaultTimestamps, defaultValues, rc1)
		appendWithResource(t, stor, ls2, defaultTimestamps, defaultValues, rc2)

		// build_info only for j1/i1
		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		mat := execRangeQuery(t, engine, stor, `info(metric, {__name__=~"target_info|build_info"})`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 2)

		// Both series get resource attrs from native metadata
		hostNames := map[string]bool{}
		var j1Version string
		var j2Version string
		for _, s := range mat {
			hostNames[s.Metric.Get("host.name")] = true
			if s.Metric.Get("job") == "j1" {
				j1Version = s.Metric.Get("version")
			} else {
				j2Version = s.Metric.Get("version")
			}
		}
		require.True(t, hostNames["host1"])
		require.True(t, hostNames["host2"])
		// j1 gets version from build_info metric-join
		require.Equal(t, "1.0", j1Version)
		// j2 has no matching build_info → no version
		require.Empty(t, j2Version)
	})

	t.Run("hybrid data label filter does not drop series", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// Resource attrs have host.name but NOT version
		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// The version=~".+" filter is meant for build_info, not for resource attrs.
		// The series should NOT be dropped just because the resource lacks "version".
		mat := execRangeQuery(t, engine, stor,
			`info(metric, {__name__=~"target_info|build_info", version=~".+"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		// Native metadata: host.name from resource attrs
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		// Metric-join: version from build_info (filtered by version=~".+")
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
	})

	t.Run("hybrid errors on conflicting labels", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// Resource attrs have "region" with one value
		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"region": "us-west"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// build_info has "region" with a conflicting value
		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// Hybrid mode: native metadata sets region=us-west, metric-join tries region=us-east → error
		err := execRangeQueryErr(t, engine, stor,
			`info(metric, {__name__=~"target_info|build_info"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Error(t, err)
		require.Contains(t, err.Error(), "conflicting label region")
	})

	t.Run("hybrid same label same value no conflict", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t)

		// Resource and build_info both have "region=us-east" — no conflict
		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"region": "us-east"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// Same value on both → no error, region appears on the output
		mat := execRangeQuery(t, engine, stor,
			`info(metric, {__name__=~"target_info|build_info"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
	})

	// ---------------------------------------------------------------
	// Paired resource-attributes vs hybrid strategy tests
	// Same storage setup, different engines, opposite assertions.
	// ---------------------------------------------------------------

	t.Run("resource-attributes: native-only ignores target_info metric", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t) // resource-attributes strategy

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// target_info metric in storage — should be ignored in native-only mode
		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		// target_info "region" is NOT added — resource-attributes mode doesn't do metric-join
		require.Empty(t, mat[0].Metric.Get("region"))
	})

	t.Run("hybrid: native-only also joins target_info metric", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newEngineWithStrategy(t, promql.InfoResourceStrategyHybrid) // hybrid strategy

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		// target_info "region" IS added — hybrid performs metric-join
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
	})

	t.Run("resource-attributes: hybrid conflict errors", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t) // resource-attributes strategy

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"region": "us-west"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// resource-attributes: hybrid mode errors on conflict between native metadata and metric-join
		err := execRangeQueryErr(t, engine, stor,
			`info(metric, {__name__=~"target_info|build_info"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Error(t, err)
		require.Contains(t, err.Error(), "conflicting label region")
	})

	t.Run("hybrid: hybrid conflict resolved silently", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newEngineWithStrategy(t, promql.InfoResourceStrategyHybrid) // hybrid strategy

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"region": "us-west"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		// hybrid: same scenario, but conflict is silently resolved (native metadata wins)
		mat := execRangeQuery(t, engine, stor,
			`info(metric, {__name__=~"target_info|build_info"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-west", mat[0].Metric.Get("region"))
	})

	t.Run("resource-attributes: hybrid excludes target_info from join", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newNativeMetadataEngine(t) // resource-attributes strategy

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		mat := execRangeQuery(t, engine, stor,
			`info(metric, {__name__=~"target_info|build_info"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
		// target_info "region" NOT added — excluded from metric-join
		require.Empty(t, mat[0].Metric.Get("region"))
	})

	t.Run("hybrid: hybrid includes target_info in join", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newEngineWithStrategy(t, promql.InfoResourceStrategyHybrid) // hybrid strategy

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		buildInfo := labels.FromStrings(
			"__name__", "build_info",
			"job", "j1", "instance", "i1",
			"version", "1.0",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})
		appendSamples(t, stor, buildInfo, defaultTimestamps, []float64{1, 1, 1})

		mat := execRangeQuery(t, engine, stor,
			`info(metric, {__name__=~"target_info|build_info"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		require.Equal(t, "1.0", mat[0].Metric.Get("version"))
		// target_info "region" IS added — included in metric-join with hybrid strategy
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
	})

	// ---------------------------------------------------------------
	// Additional hybrid strategy edge-case tests
	// ---------------------------------------------------------------

	t.Run("hybrid native metadata wins on conflict", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newEngineWithStrategy(t, promql.InfoResourceStrategyHybrid)

		// Resource has region=us-west
		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"region": "us-west"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// target_info has region=us-east (conflict)
		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// Hybrid mode: native metadata wins silently on conflict
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-west", mat[0].Metric.Get("region"))
	})

	t.Run("hybrid no resource attrs uses target_info only", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newEngineWithStrategy(t, promql.InfoResourceStrategyHybrid)

		// No resource context (V1 appender)
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendSamples(t, stor, ls, defaultTimestamps, defaultValues)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// No resource attrs → native metadata does nothing, target_info metric-join enriches
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
	})

	t.Run("hybrid no target_info uses resource attrs only", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newEngineWithStrategy(t, promql.InfoResourceStrategyHybrid)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		// No target_info metric → hybrid join finds nothing, resource attrs still applied
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
	})

	t.Run("hybrid with explicit __name__=target_info", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)
		engine := newEngineWithStrategy(t, promql.InfoResourceStrategyHybrid)

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// Explicit __name__=target_info → native-only mode, but with hybrid strategy
		mat := execRangeQuery(t, engine, stor,
			`info(metric, {__name__="target_info"})`,
			defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "from_resource", mat[0].Metric.Get("host.name"))
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
	})

	t.Run("gate: strategy ignored without native metadata", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)

		// Engine WITHOUT native metadata — strategy is forced to target-info
		engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
			MaxSamples:           50000,
			Timeout:              10 * time.Second,
			EnableAtModifier:     true,
			EnableNegativeOffset: true,
			EnableNativeMetadata: false,
			InfoResourceStrategy: promql.InfoResourceStrategyHybrid,
			Parser:               parser.NewParser(promqltest.TestParserOpts),
		})

		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendSamples(t, stor, ls, defaultTimestamps, defaultValues)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// Without native metadata, strategy is forced to target-info regardless.
		// Standard metric-join behavior.
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
	})

	t.Run("gate: resource-attributes forced to target-info without native metadata", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)

		// Engine WITHOUT native metadata but with resource-attributes strategy
		engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
			MaxSamples:           50000,
			Timeout:              10 * time.Second,
			EnableAtModifier:     true,
			EnableNegativeOffset: true,
			EnableNativeMetadata: false,
			InfoResourceStrategy: promql.InfoResourceStrategyResourceAttributes,
			Parser:               parser.NewParser(promqltest.TestParserOpts),
		})

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// Strategy forced to target-info: uses metric-join, gets region from target_info,
		// does NOT get host.name from resource attrs.
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
		require.Empty(t, mat[0].Metric.Get("host.name"))
	})

	t.Run("gate: empty strategy defaults to target-info with native metadata", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)

		// Engine with native metadata but empty strategy (zero value).
		// Should default to target-info for consistency with the CLI default.
		engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
			MaxSamples:           50000,
			Timeout:              10 * time.Second,
			EnableAtModifier:     true,
			EnableNegativeOffset: true,
			EnableNativeMetadata: true,
			Parser:               parser.NewParser(promqltest.TestParserOpts),
			// InfoResourceStrategy intentionally not set (zero value "").
		})

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// Empty strategy + native metadata → defaults to target-info (consistent with CLI).
		// Gets region from target_info metric-join, does NOT get host.name from resource attrs.
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
		require.Empty(t, mat[0].Metric.Get("host.name"))
	})

	t.Run("gate: invalid strategy defaults to target-info", func(t *testing.T) {
		stor := newNativeMetadataStorage(t)

		// Engine with an invalid strategy string — should warn and default to target-info.
		engine := promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
			MaxSamples:           50000,
			Timeout:              10 * time.Second,
			EnableAtModifier:     true,
			EnableNegativeOffset: true,
			EnableNativeMetadata: true,
			InfoResourceStrategy: promql.InfoResourceStrategy("bogus"),
			Parser:               parser.NewParser(promqltest.TestParserOpts),
		})

		rc := &storage.ResourceContext{
			Descriptive: map[string]string{"host.name": "from_resource"},
		}
		ls := labels.FromStrings("__name__", "metric", "job", "j1", "instance", "i1")
		appendWithResource(t, stor, ls, defaultTimestamps, defaultValues, rc)

		targetInfoMetric := labels.FromStrings(
			"__name__", "target_info",
			"job", "j1", "instance", "i1",
			"region", "us-east",
		)
		appendSamples(t, stor, targetInfoMetric, defaultTimestamps, []float64{1, 1, 1})

		// Invalid strategy → falls back to target-info (metric-join only).
		mat := execRangeQuery(t, engine, stor, `info(metric)`, defaultStart, defaultEnd, defaultStep)
		require.Len(t, mat, 1)
		require.Equal(t, "us-east", mat[0].Metric.Get("region"))
		require.Empty(t, mat[0].Metric.Get("host.name"))
	})
}
