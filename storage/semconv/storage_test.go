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
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

// requireWarningsContain asserts that at least one warning in the slice
// contains substr. Annotations are returned as a map, so iteration order is
// non-deterministic — checking warnings[0] would be flaky.
func requireWarningsContain(t *testing.T, warnings []string, substr string) {
	t.Helper()
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return
		}
	}
	t.Fatalf("expected a warning containing %q, got %v", substr, warnings)
}

// newAwareStorage builds a TestStorage and wraps it with AwareStorage. It
// returns both so tests can append directly into the underlying storage.
func newAwareStorage(t *testing.T) (storage.Storage, *teststorage.TestStorage) {
	t.Helper()
	underlying := teststorage.New(t)
	return semconv.AwareStorage(underlying), underlying
}

// erroringStorage wraps a storage.Storage and replaces LabelNames /
// LabelValues responses with a configured error whenever the supplied
// matchers carry an explicit __name__ value found in errsByMetric. All other
// methods delegate. It is a test-only fake used to exercise the wrapper's
// multi-variant error aggregation paths.
type erroringStorage struct {
	storage.Storage
	errsByMetric map[string]error
}

type queryCallCounts struct {
	mu               sync.Mutex
	selectSeries     int64
	selectChunks     int64
	labelNames       int64
	labelValues      int64
	chunkLabelNames  int64
	chunkLabelValues int64
}

func (c *queryCallCounts) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.selectSeries = 0
	c.selectChunks = 0
	c.labelNames = 0
	c.labelValues = 0
	c.chunkLabelNames = 0
	c.chunkLabelValues = 0
}

func (c *queryCallCounts) total() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.selectSeries + c.selectChunks + c.labelNames + c.labelValues + c.chunkLabelNames + c.chunkLabelValues
}

func (c *queryCallCounts) increment(counter *int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	(*counter)++
}

type countingStorage struct {
	storage.Storage
	counts *queryCallCounts
}

func (s *countingStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &countingQuerier{Querier: q, counts: s.counts}, nil
}

func (s *countingStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &countingChunkQuerier{ChunkQuerier: q, counts: s.counts}, nil
}

type countingQuerier struct {
	storage.Querier
	counts *queryCallCounts
}

func (q *countingQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	q.counts.increment(&q.counts.selectSeries)
	return q.Querier.Select(ctx, sortSeries, hints, matchers...)
}

func (q *countingQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.counts.increment(&q.counts.labelNames)
	return q.Querier.LabelNames(ctx, hints, matchers...)
}

func (q *countingQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.counts.increment(&q.counts.labelValues)
	return q.Querier.LabelValues(ctx, name, hints, matchers...)
}

type countingChunkQuerier struct {
	storage.ChunkQuerier
	counts *queryCallCounts
}

func (q *countingChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	q.counts.increment(&q.counts.selectChunks)
	return q.ChunkQuerier.Select(ctx, sortSeries, hints, matchers...)
}

func (q *countingChunkQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.counts.increment(&q.counts.chunkLabelNames)
	return q.ChunkQuerier.LabelNames(ctx, hints, matchers...)
}

func (q *countingChunkQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.counts.increment(&q.counts.chunkLabelValues)
	return q.ChunkQuerier.LabelValues(ctx, name, hints, matchers...)
}

func (s *erroringStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &erroringQuerier{Querier: q, errsByMetric: s.errsByMetric}, nil
}

func (s *erroringStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &erroringChunkQuerier{ChunkQuerier: q, errsByMetric: s.errsByMetric}, nil
}

// errsForMetric returns the configured error for the metric name carried in
// matchers, or nil if no matcher targets a metric in errsByMetric.
func errsForMetric(errsByMetric map[string]error, matchers []*labels.Matcher) error {
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel {
			if err, ok := errsByMetric[m.Value]; ok {
				return err
			}
		}
	}
	return nil
}

type erroringQuerier struct {
	storage.Querier
	errsByMetric map[string]error
}

func (q *erroringQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if err := errsForMetric(q.errsByMetric, matchers); err != nil {
		return nil, nil, err
	}
	return q.Querier.LabelNames(ctx, hints, matchers...)
}

func (q *erroringQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if err := errsForMetric(q.errsByMetric, matchers); err != nil {
		return nil, nil, err
	}
	return q.Querier.LabelValues(ctx, name, hints, matchers...)
}

type erroringChunkQuerier struct {
	storage.ChunkQuerier
	errsByMetric map[string]error
}

func (q *erroringChunkQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if err := errsForMetric(q.errsByMetric, matchers); err != nil {
		return nil, nil, err
	}
	return q.ChunkQuerier.LabelNames(ctx, hints, matchers...)
}

func (q *erroringChunkQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if err := errsForMetric(q.errsByMetric, matchers); err != nil {
		return nil, nil, err
	}
	return q.ChunkQuerier.LabelValues(ctx, name, hints, matchers...)
}

// appendSeries writes a single (name=metric, labels..., t, v) sample into s.
// Labels alternate name/value pairs.
func appendSeries(t *testing.T, s storage.Storage, metric string, ts int64, v float64, kv ...string) {
	t.Helper()
	require.Equal(t, 0, len(kv)%2, "kv must be name/value pairs")
	app := s.Appender(context.Background())
	lblPairs := append([]string{model.MetricNameLabel, metric}, kv...)
	_, err := app.Append(0, labels.FromStrings(lblPairs...), ts, v)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
}

// collectSeries drains a SeriesSet into a slice of label-string -> values.
// Each value is the first sample's value (tests append a single sample each).
func collectSeries(t *testing.T, set storage.SeriesSet) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)
	for set.Next() {
		s := set.At()
		it := s.Iterator(nil)
		require.Positive(t, it.Next(), "expected at least one sample")
		_, v := it.At()
		out[s.Labels().String()] = v
	}
	require.NoError(t, set.Err())
	return out
}

func collectSeriesSampleCounts(t *testing.T, set storage.SeriesSet) map[string]int {
	t.Helper()
	out := make(map[string]int)
	for set.Next() {
		series := set.At()
		it := series.Iterator(nil)
		for it.Next() != chunkenc.ValNone {
			out[series.Labels().String()]++
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, set.Err())
	return out
}

func undeclaredAttributeRegistry() map[string][]byte {
	return map[string][]byte{
		"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.0.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            m.old: m
        - rename_attributes:
            attribute_map:
              svc.env: svc.environment
            apply_to_metrics:
              - m
`),
		"1.0.0": []byte(`groups:
  - id: metric.m.old
    type: metric
    metric_name: m.old
    instrument: counter
    unit: "1"
`),
		"1.1.0": []byte(`groups:
  - id: metric.m
    type: metric
    metric_name: m
    instrument: counter
    unit: "1"
`),
	}
}

func undeclaredAttributeMatchers(version, metric, attribute string, withAttribute bool) []*labels.Matcher {
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metric),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/"+version),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
	if withAttribute {
		matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, attribute, "prod"))
	}
	return matchers
}

func TestSchemaAttributeRenamesWithoutSemconvDeclarations(t *testing.T) {
	underlying := teststorage.New(t)
	wrapper, err := semconv.AwareStorageWithRegistry(underlying, undeclaredAttributeRegistry())
	require.NoError(t, err)
	appendSeries(t, underlying, "m.old", 1, 1, "svc.env", "prod")
	appendSeries(t, underlying, "m", 2, 2, "svc.environment", "prod")

	for _, anchor := range []struct {
		name      string
		version   string
		metric    string
		attribute string
		alias     string
	}{
		{name: "backward", version: "1.1.0", metric: "m", attribute: "svc.environment", alias: "svc.env"},
		{name: "forward", version: "1.0.0", metric: "m.old", attribute: "svc.env", alias: "svc.environment"},
	} {
		t.Run(anchor.name, func(t *testing.T) {
			for _, withAttribute := range []bool{false, true} {
				t.Run(fmt.Sprintf("attribute matcher %t", withAttribute), func(t *testing.T) {
					matchers := undeclaredAttributeMatchers(anchor.version, anchor.metric, anchor.attribute, withAttribute)
					querier, err := wrapper.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, querier.Close()) })
					got := collectSeries(t, querier.Select(t.Context(), true, nil, matchers...))
					require.Len(t, got, 1)
					for labelSet := range got {
						require.Contains(t, labelSet, `__name__="`+anchor.metric+`"`)
						require.Contains(t, labelSet, `"`+anchor.attribute+`"="prod"`)
						require.NotContains(t, labelSet, `"`+anchor.alias+`"=`)
					}

					chunkQuerier, err := wrapper.ChunkQuerier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, chunkQuerier.Close()) })
					chunks := chunkQuerier.Select(t.Context(), true, nil, matchers...)
					require.Len(t, collectSeries(t, storage.NewSeriesSetFromChunkSeriesSet(chunks)), 1)
				})
			}

			matchers := undeclaredAttributeMatchers(anchor.version, anchor.metric, anchor.attribute, false)
			querier, err := wrapper.Querier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, querier.Close()) })
			names, _, err := querier.LabelNames(t.Context(), nil, matchers...)
			require.NoError(t, err)
			require.Contains(t, names, anchor.attribute)
			require.NotContains(t, names, anchor.alias)
			values, _, err := querier.LabelValues(t.Context(), anchor.attribute, nil, matchers...)
			require.NoError(t, err)
			require.Equal(t, []string{"prod"}, values)
		})
	}

	valueUnderlying := teststorage.New(t)
	valueWrapper, err := semconv.AwareStorageWithRegistry(valueUnderlying, undeclaredAttributeRegistry())
	require.NoError(t, err)
	appendSeries(t, valueUnderlying, "m.old", 1, 1, "svc.env", "legacy")
	appendSeries(t, valueUnderlying, "m", 2, 2, "svc.environment", "current")
	for _, anchor := range []struct {
		version   string
		metric    string
		attribute string
	}{
		{version: "1.1.0", metric: "m", attribute: "svc.environment"},
		{version: "1.0.0", metric: "m.old", attribute: "svc.env"},
	} {
		querier, err := valueWrapper.Querier(0, 10)
		require.NoError(t, err)
		values, _, err := querier.LabelValues(
			t.Context(),
			anchor.attribute,
			nil,
			undeclaredAttributeMatchers(anchor.version, anchor.metric, anchor.attribute, false)...,
		)
		require.NoError(t, err)
		require.NoError(t, querier.Close())
		require.Equal(t, []string{"current", "legacy"}, values)
	}
}

func TestSchemaMetricAndAttributeRevisionBoundaries(t *testing.T) {
	for _, anchor := range []struct {
		name      string
		version   string
		metric    string
		attribute string
		alias     string
	}{
		{name: "backward", version: "1.1.0", metric: "m", attribute: "svc.environment", alias: "svc.env"},
		{name: "forward", version: "1.0.0", metric: "m.old", attribute: "svc.env", alias: "svc.environment"},
	} {
		t.Run(anchor.name, func(t *testing.T) {
			underlying := teststorage.New(t)
			wrapper, err := semconv.AwareStorageWithRegistry(underlying, undeclaredAttributeRegistry())
			require.NoError(t, err)
			appendSeries(t, underlying, "m.old", 1, 1,
				"instance", "old-boundary", "svc.env", "prod")
			appendSeries(t, underlying, "m", 2, 2,
				"instance", "current-boundary", "svc.environment", "prod")
			appendSeries(t, underlying, "m", 10, 10,
				"instance", "dual", "svc.env", "prod", "svc.environment", "prod")

			matchers := undeclaredAttributeMatchers(anchor.version, anchor.metric, anchor.attribute, true)
			for _, query := range []string{"series", "chunks"} {
				t.Run(query, func(t *testing.T) {
					var set storage.SeriesSet
					if query == "series" {
						querier, err := wrapper.Querier(0, 20)
						require.NoError(t, err)
						t.Cleanup(func() { require.NoError(t, querier.Close()) })
						set = querier.Select(t.Context(), true, nil, matchers...)
					} else {
						querier, err := wrapper.ChunkQuerier(0, 20)
						require.NoError(t, err)
						t.Cleanup(func() { require.NoError(t, querier.Close()) })
						set = storage.NewSeriesSetFromChunkSeriesSet(querier.Select(t.Context(), true, nil, matchers...))
					}
					got := collectSeriesSampleCounts(t, set)
					require.Len(t, got, 3)
					for labelSet, sampleCount := range got {
						require.Contains(t, labelSet, `__name__="`+anchor.metric+`"`)
						require.Contains(t, labelSet, `"`+anchor.attribute+`"="prod"`)
						require.NotContains(t, labelSet, `"`+anchor.alias+`"=`)
						require.Equal(t, 1, sampleCount)
					}
				})
			}

			querier, err := wrapper.Querier(0, 20)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, querier.Close()) })
			baseMatchers := undeclaredAttributeMatchers(anchor.version, anchor.metric, anchor.attribute, false)
			names, _, err := querier.LabelNames(t.Context(), nil, baseMatchers...)
			require.NoError(t, err)
			require.Contains(t, names, anchor.attribute)
			require.NotContains(t, names, anchor.alias)
			values, _, err := querier.LabelValues(t.Context(), anchor.attribute, nil, baseMatchers...)
			require.NoError(t, err)
			require.Equal(t, []string{"prod"}, values)
		})
	}
}

func TestSchemaLabelMigrationConflictsFailClosed(t *testing.T) {
	for _, query := range []string{"series", "chunks"} {
		t.Run(query, func(t *testing.T) {
			t.Run("equal values coalesce on the anchor metric", func(t *testing.T) {
				wrapper, underlying := newAwareStorage(t)
				appendSeries(t, underlying, "test", 1, 1, "user", "acme", "tenant", "acme")

				if query == "series" {
					querier, err := wrapper.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, querier.Close()) })
					got := collectSeries(t, querier.Select(t.Context(), true, nil, schemaReadMatchers()...))
					require.Len(t, got, 1)
					for labelSet := range got {
						require.Contains(t, labelSet, `tenant="acme"`)
						require.NotContains(t, labelSet, "user=")
					}
					return
				}

				querier, err := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, querier.Close()) })
				set := storage.NewSeriesSetFromChunkSeriesSet(querier.Select(t.Context(), true, nil, schemaReadMatchers()...))
				require.Len(t, collectSeries(t, set), 1)
			})

			t.Run("different values on the anchor metric fail the query", func(t *testing.T) {
				wrapper, underlying := newAwareStorage(t)
				appendSeries(t, underlying, "test", 1, 1, "instance", "conflict", "user", "legacy", "tenant", "current")
				appendSeries(t, underlying, "test", 2, 2, "instance", "healthy", "user", "same")

				if query == "series" {
					querier, err := wrapper.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, querier.Close()) })
					set := querier.Select(t.Context(), true, nil, schemaReadMatchers()...)
					for set.Next() {
					}
					require.ErrorContains(t, set.Err(), "conflicting values")
					return
				}

				querier, err := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, querier.Close()) })
				set := querier.Select(t.Context(), true, nil, schemaReadMatchers()...)
				for set.Next() {
				}
				require.ErrorContains(t, set.Err(), "conflicting values")
			})
		})
	}
}

// warningStrings flattens annotations into their string forms for assertion.
func warningStrings(a map[string]error) []string {
	out := make([]string, 0, len(a))
	for k := range a {
		out = append(out, k)
	}
	return out
}

func metricVariantOverflowRegistry(sources int) map[string][]byte {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
`)
	for i := range sources {
		schema = fmt.Appendf(schema, "            metric.old.%03d: metric.current\n", i)
	}
	return map[string][]byte{
		"registry.yaml": schema,
		"1.1.0":         metricSemconv("metric.current", "s", "histogram"),
	}
}

func aggregateExpansionOverflowRegistry(attributes, sources int) map[string][]byte {
	files := metricVariantOverflowRegistry(sources)
	semconvFile := []byte(`groups:
  - id: metric.metric.current
    type: metric
    metric_name: metric.current
    unit: s
    instrument: histogram
    attributes:
`)
	for i := range attributes {
		name := "attr.current"
		if i > 0 {
			name = fmt.Sprintf("attr.current.%03d", i)
		}
		semconvFile = fmt.Appendf(semconvFile, "      - ref: %s\n", name)
	}
	files["1.1.0"] = semconvFile
	return files
}

func labelValueJobOverflowRegistry(aliases int) map[string][]byte {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
`)
	for i := range aliases {
		schema = fmt.Appendf(schema, "              attr.old.%03d: attr.current\n", i)
	}
	schema = append(schema, []byte(`        - rename_metrics:
            metric.a: metric.current
            metric.b: metric.current
`)...)
	return map[string][]byte{
		"registry.yaml": schema,
		"1.0.0": []byte(`groups:
  - id: metric.metric.a
    type: metric
    metric_name: metric.a
    unit: s
    instrument: histogram
    stability: stable
  - id: metric.metric.b
    type: metric
    metric_name: metric.b
    unit: s
    instrument: histogram
    stability: stable
`),
		"1.1.0": []byte(`groups:
  - id: metric.metric.current
    type: metric
    metric_name: metric.current
    unit: s
    instrument: histogram
    attributes:
      - ref: attr.current
`),
	}
}

func newCountingAwareStorage(t *testing.T, files map[string][]byte) (storage.Storage, *queryCallCounts) {
	t.Helper()
	counts := &queryCallCounts{}
	underlying := &countingStorage{Storage: teststorage.New(t), counts: counts}
	wrapped, err := semconv.AwareStorageWithRegistry(underlying, files)
	require.NoError(t, err)
	return wrapped, counts
}

func collectChunkSeriesCount(t *testing.T, set storage.ChunkSeriesSet) int {
	t.Helper()
	count := 0
	for set.Next() {
		count++
	}
	require.NoError(t, set.Err())
	return count
}

// TestAwareStorage tests AwareStorage.
func TestAwareStorage(t *testing.T) {
	t.Run("passes through without a special matcher", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "up", 1, 42, "instance", "a")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "up"))

		got := collectSeries(t, set)
		require.Len(t, got, 1)
		for k, v := range got {
			require.Contains(t, k, "instance=\"a\"")
			require.Equal(t, 42.0, v)
		}
	})

	t.Run("warns and passes through on a duplicate __semconv_url__", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.0.0"),
		)

		_ = collectSeries(t, set)
		warnings := warningStrings(set.Warnings())
		require.NotEmpty(t, warnings)
		requireWarningsContain(t, warnings, "used more than once")
	})

	t.Run("warns and passes through on a non-equal __semconv_url__", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchRegexp, "__semconv_url__", "registry/1\\..*"),
		)

		_ = collectSeries(t, set)
		warnings := warningStrings(set.Warnings())
		require.NotEmpty(t, warnings)
		requireWarningsContain(t, warnings, "ambiguous")
	})

	t.Run("rejects http and traversal URLs", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0)

		for _, bad := range []string{
			"http://169.254.169.254/latest/meta-data/iam/info",
			"https://example.com/etc/passwd",
			"/etc/passwd",
			"../../../etc/passwd",
			"./testdata/otel.yaml",
			"registry/../etc/passwd",
		} {
			t.Run(bad, func(t *testing.T) {
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })

				set := q.Select(context.Background(), false, nil,
					labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
					labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", bad),
					// A valid __schema_url__ triggers fan-out so the bad
					// __semconv_url__ is actually loaded and rejected.
					labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
				)
				// The set must complete cleanly (passthrough) and surface a warning.
				_ = collectSeries(t, set)
				warnings := warningStrings(set.Warnings())
				require.NotEmpty(t, warnings, "expected a warning for %q", bad)
				requireWarningsContain(t, warnings, "schematization logic is skipped")
			})
		}
	})

	t.Run("LabelValues passes through without a special matcher", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "up", 1, 1.0, "instance", "a")
		appendSeries(t, wrapped, "up", 1, 1.0, "instance", "b")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		vals, _, err := q.LabelValues(context.Background(), "instance", nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "up"))
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a", "b"}, vals)
	})

	// Schema-version rename fan-out: a native-OTel producer's historical metric
	// name surfaces under the requested version's canonical name via __schema_url__.
	t.Run("schema version rename", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		// The producer wrote the metric under its semconv 1.0.0 name "test.counter"
		// (native OTel names); semconv 1.1.0 renamed it to "test".
		appendSeries(t, wrapped, "test.counter", 1, 7.0, "http.response.status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the historical name to surface via __schema_url__")
		var found bool
		for k := range got {
			if strings.Contains(k, `__name__="test"`) {
				found = true
			}
		}
		require.True(t, found, "expected the renamed metric under its 1.1.0 name in: %v", got)
	})

	t.Run("rewrites duplicate metric name matchers", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test.counter", 1, 7.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)
		got := collectSeries(t, set)
		require.Len(t, got, 1, "both canonical metric matchers must follow the historical lineage: %v", got)
		for key := range got {
			require.Contains(t, key, `__name__="test"`)
		}
	})

	// When the fan-out probes multiple historical names, the wrapper surfaces
	// every underlying failure via errors.Is rather than only the first.
	t.Run("aggregates variant errors", func(t *testing.T) {
		// Anchored at semconv 1.1.0, "test" fans out to its historical names:
		// test.counter (1.0.0), test (1.1.0) and test.v2 (1.2.0). Each variant is
		// probed concurrently; a per-variant error must propagate through the join.
		errAnchor := errors.New("err-anchor")
		errOld := errors.New("err-old")
		errNew := errors.New("err-new")

		underlying := teststorage.New(t)
		wrapped := semconv.AwareStorage(&erroringStorage{
			Storage: underlying,
			errsByMetric: map[string]error{
				"test":         errAnchor,
				"test.counter": errOld,
				"test.v2":      errNew,
			},
		})

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		_, _, err = q.LabelNames(context.Background(), nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)
		require.Error(t, err)
		require.ErrorIs(t, err, errAnchor, "errors.Join should preserve every variant error")
		require.ErrorIs(t, err, errOld, "errors.Join should preserve every variant error")
		require.ErrorIs(t, err, errNew, "errors.Join should preserve every variant error")
	})

	// Attribute-rename normalisation: registry.yaml renames the attribute user
	// (semconv 1.0.0) -> tenant (1.1.0) on metric test, alongside the metric
	// rename test.counter -> test. A query at 1.1.0 must surface the 1.0.0-era
	// series under the canonical attribute name.
	t.Run("attribute rename", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		// semconv 1.0.0 era: native name test.counter with the old attribute user.
		appendSeries(t, wrapped, "test.counter", 1, 7.0, "user", "acme", "http.response.status_code", "200")
		// semconv 1.1.0 era: renamed to test with the new attribute tenant.
		appendSeries(t, wrapped, "test", 2, 8.0, "tenant", "acme", "http.response.status_code", "200")

		matchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		}

		t.Run("Select merges eras under the canonical attribute name", func(t *testing.T) {
			q, err := wrapped.Querier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })

			set := q.Select(context.Background(), true, nil, matchers...)
			got := collectSeries(t, set)
			require.Len(t, got, 1, "the two eras should merge into a single series under tenant: %v", got)
			for k := range got {
				require.Contains(t, k, `__name__="test"`)
				require.Contains(t, k, `tenant="acme"`)
				require.NotContains(t, k, "user=", "historical attribute name must be normalised: %v", got)
			}
		})

		t.Run("LabelNames reports the canonical attribute name", func(t *testing.T) {
			q, err := wrapped.Querier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })

			names, _, err := q.LabelNames(context.Background(), nil, matchers...)
			require.NoError(t, err)
			require.Contains(t, names, "tenant")
			require.Contains(t, names, "http.response.status_code")
			require.NotContains(t, names, "user", "historical attribute name must be normalised in LabelNames: %v", names)
		})

		t.Run("LabelValues surfaces values from both eras", func(t *testing.T) {
			// Distinct values per era: the assertion fails unless the 1.0.0 era,
			// stored under "user", is also consulted when querying values of "tenant".
			lvStore, _ := newAwareStorage(t)
			appendSeries(t, lvStore, "test.counter", 1, 7.0, "user", "legacy")
			appendSeries(t, lvStore, "test", 2, 8.0, "tenant", "current")

			q, err := lvStore.Querier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })

			values, _, err := q.LabelValues(context.Background(), "tenant", nil, matchers...)
			require.NoError(t, err)
			require.ElementsMatch(t, []string{"legacy", "current"}, values)
		})
	})
}

func TestMetricNameConstraintsKeepPromQLSemantics(t *testing.T) {
	wrapped, _ := newAwareStorage(t)
	appendSeries(t, wrapped, "test.counter", 1, 1.0, "era", "old")
	appendSeries(t, wrapped, "test", 1, 2.0, "era", "current")

	tests := []struct {
		name          string
		nameMatchers  []*labels.Matcher
		wantSeries    int
		wantLabelName bool
		wantValues    []string
	}{
		{
			name: "compatible constraints apply to the canonical name",
			nameMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, `test(?:\.counter)?`),
				labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "test.counter"),
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			},
			wantSeries:    2,
			wantLabelName: true,
			wantValues:    []string{"current", "old"},
		},
		{
			name: "contradictory constraints remain unsatisfiable",
			nameMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
				labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, `test\.counter`),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matchers := append(slices.Clone(tc.nameMatchers),
				labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
				labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
			)

			q, err := wrapped.Querier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })

			set := q.Select(context.Background(), false, nil, matchers...)
			got := collectSeries(t, set)
			require.Len(t, got, tc.wantSeries)
			for key := range got {
				require.Contains(t, key, `__name__="test"`)
			}

			names, _, err := q.LabelNames(context.Background(), nil, matchers...)
			require.NoError(t, err)
			require.Equal(t, tc.wantLabelName, slices.Contains(names, "era"))

			values, _, err := q.LabelValues(context.Background(), "era", nil, matchers...)
			require.NoError(t, err)
			if len(tc.wantValues) == 0 {
				require.Empty(t, values)
			} else {
				require.Equal(t, tc.wantValues, values)
			}

			cq, err := wrapped.ChunkQuerier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cq.Close() })
			require.Equal(t, tc.wantSeries, collectChunkSeriesCount(t,
				cq.Select(context.Background(), false, nil, matchers...)))
		})
	}
}

func TestSchemaAwareQueriesRequireExactMetricNameAnchor(t *testing.T) {
	controlMatchers := func(metricMatcher *labels.Matcher) []*labels.Matcher {
		matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "job", "api")}
		if metricMatcher != nil {
			matchers = append(matchers, metricMatcher)
		}
		return append(matchers,
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)
	}

	for _, tc := range []struct {
		name          string
		metricMatcher *labels.Matcher
	}{
		{name: "missing"},
		{name: "regexp", metricMatcher: labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, "metric.*")},
		{name: "negative", metricMatcher: labels.MustNewMatcher(labels.MatchNotEqual, model.MetricNameLabel, "metric.old")},
		{name: "empty equality", metricMatcher: labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapped, counts := newCountingAwareStorage(t, canonicalLabelRegistry())
			q, err := wrapped.Querier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = q.Close() })
			cq, err := wrapped.ChunkQuerier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { _ = cq.Close() })
			matchers := controlMatchers(tc.metricMatcher)

			counts.reset()
			set := q.Select(context.Background(), false, nil, matchers...)
			require.False(t, set.Next())
			require.ErrorContains(t, set.Err(), "requires a non-empty equality matcher on __name__")
			require.Zero(t, counts.total())

			counts.reset()
			chunkSet := cq.Select(context.Background(), false, nil, matchers...)
			require.False(t, chunkSet.Next())
			require.ErrorContains(t, chunkSet.Err(), "requires a non-empty equality matcher on __name__")
			require.Zero(t, counts.total())

			counts.reset()
			_, _, err = q.LabelNames(context.Background(), nil, matchers...)
			require.ErrorContains(t, err, "requires a non-empty equality matcher on __name__")
			require.Zero(t, counts.total())

			counts.reset()
			_, _, err = cq.LabelValues(context.Background(), "job", nil, matchers...)
			require.ErrorContains(t, err, "requires a non-empty equality matcher on __name__")
			require.Zero(t, counts.total())
		})
	}
}

func TestSchemaExpansionOverflowFailsBeforeStorage(t *testing.T) {
	for _, tc := range []struct {
		name        string
		files       map[string][]byte
		errorDetail string
	}{
		{
			name:        "collection cardinality",
			files:       metricVariantOverflowRegistry(256),
			errorDetail: "matcher variants would exceed 256",
		},
		{
			name:        "aggregate resolver work",
			files:       aggregateExpansionOverflowRegistry(255, 255),
			errorDetail: "resolver work would exceed 65536",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testSchemaExpansionOverflowFailsBeforeStorage(t, tc.files, tc.errorDetail)
		})
	}
}

func testSchemaExpansionOverflowFailsBeforeStorage(t *testing.T, files map[string][]byte, errorDetail string) {
	wrapped, counts := newCountingAwareStorage(t, files)
	appendSeries(t, wrapped, "metric.current", 1, 7.0, "attr.current", "value")
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	cq, err := wrapped.ChunkQuerier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cq.Close() })

	assertExpansionError := func(t *testing.T, err error) {
		t.Helper()
		require.ErrorContains(t, err, "schema expansion limit exceeded")
		require.ErrorContains(t, err, errorDetail)
		require.Zero(t, counts.total(), "overflow must fail before querying storage")
	}

	t.Run("Select", func(t *testing.T) {
		counts.reset()
		set := q.Select(context.Background(), false, nil, matchers...)
		require.False(t, set.Next())
		assertExpansionError(t, set.Err())
		require.Empty(t, set.Warnings())
	})

	t.Run("LabelNames", func(t *testing.T) {
		counts.reset()
		names, anns, err := q.LabelNames(context.Background(), nil, matchers...)
		assertExpansionError(t, err)
		require.Nil(t, names)
		require.Empty(t, anns)
	})

	t.Run("LabelValues", func(t *testing.T) {
		counts.reset()
		values, anns, err := q.LabelValues(context.Background(), "attr.current", nil, matchers...)
		assertExpansionError(t, err)
		require.Nil(t, values)
		require.Empty(t, anns)
	})

	t.Run("ChunkSelect", func(t *testing.T) {
		counts.reset()
		set := cq.Select(context.Background(), false, nil, matchers...)
		require.False(t, set.Next())
		assertExpansionError(t, set.Err())
		require.Empty(t, set.Warnings())
	})

	t.Run("ChunkLabelNames", func(t *testing.T) {
		counts.reset()
		names, anns, err := cq.LabelNames(context.Background(), nil, matchers...)
		assertExpansionError(t, err)
		require.Nil(t, names)
		require.Empty(t, anns)
	})

	t.Run("ChunkLabelValues", func(t *testing.T) {
		counts.reset()
		values, anns, err := cq.LabelValues(context.Background(), "attr.current", nil, matchers...)
		assertExpansionError(t, err)
		require.Nil(t, values)
		require.Empty(t, anns)
	})
}

func TestLabelValueJobOverflowFailsClosed(t *testing.T) {
	wrapped, counts := newCountingAwareStorage(t, labelValueJobOverflowRegistry(128))
	appendSeries(t, wrapped, "metric.current", 1, 7.0, "attr.current", "value")
	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	counts.reset()
	values, anns, err := q.LabelValues(context.Background(), "attr.current", nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)
	require.ErrorContains(t, err, "schema expansion limit exceeded")
	require.Empty(t, values)
	require.Empty(t, anns)
	require.Zero(t, counts.total(), "job overflow must not query storage")
}

func TestSchemaWarning_ClassifiedAsWarning(t *testing.T) {
	// ErrSchemaWarning chains through annotations.PromQLWarning, so warnings
	// emitted by the wrapper satisfy errors.Is for both sentinels and surface
	// as PromQL warnings via util/annotations.AsStrings.
	wrapped, _ := newAwareStorage(t)
	appendSeries(t, wrapped, "test", 1, 1.0)

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "http://example.com/x.yaml"),
	)
	_ = collectSeries(t, set)
	got := set.Warnings()
	require.NotEmpty(t, got, "expected at least one SchemaWarning to be emitted")
	for _, err := range got {
		require.ErrorIs(t, err, semconv.ErrSchemaWarning, "warning %v should be a SchemaWarning", err)
		require.ErrorIs(t, err, annotations.PromQLWarning, "warning %v should chain through PromQLWarning", err)
	}
}
