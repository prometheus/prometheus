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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestStorageFanOutContracts(t *testing.T) {
	t.Run("canonical labels are valid", func(t *testing.T) {
		mapping := buildLabelMapping("metric.new", map[string]string{"old": "new"})
		got, err := transformOTelSchemaLabels(labels.FromStrings(
			model.MetricNameLabel, "metric.old",
			semconvURLLabel, "registry/1.0.0",
			"old", "same",
			"new", "same",
		), mapping)
		require.NoError(t, err)
		require.Equal(t, labels.FromStrings(model.MetricNameLabel, "metric.new", "new", "same"), got)
	})

	t.Run("conflicting canonical labels fail", func(t *testing.T) {
		mapping := buildLabelMapping("metric.new", map[string]string{"old": "new"})
		_, err := transformOTelSchemaLabels(labels.FromStrings("old", "first", "new", "second"), mapping)
		require.ErrorContains(t, err, "conflicting values")
	})

	t.Run("only attribute mappings require resorting", func(t *testing.T) {
		require.False(t, mappingNeedsResort(buildLabelMapping("metric.new", nil)))
		require.True(t, mappingNeedsResort(buildLabelMapping("metric.new", map[string]string{"old": "new"})))
	})

	t.Run("identity mappings preserve underlying objects", func(t *testing.T) {
		mapping := buildLabelMapping("metric", nil)
		series := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "metric", "instance", "a"), nil)
		seriesSet := &awareSeriesSet{
			SeriesSet: &singleSeriesSet{series: series},
			mapping:   mapping,
		}
		require.True(t, seriesSet.Next())
		require.Same(t, series, seriesSet.At())

		chunkSeries := storage.NewSeriesToChunkEncoder(series)
		chunkSet := &awareChunkSeriesSet{
			ChunkSeriesSet: &singleChunkSeriesSet{series: chunkSeries},
			mapping:        mapping,
		}
		require.True(t, chunkSet.Next())
		require.Same(t, chunkSeries, chunkSet.At())
	})

	t.Run("select hints are cloned", func(t *testing.T) {
		original := &storage.SelectHints{
			Start:            1,
			End:              2,
			Limit:            3,
			Grouping:         []string{"group"},
			ProjectionLabels: []string{"project"},
		}
		cloned := cloneSelectHints(original)
		cloned.Limit = 0
		cloned.Grouping[0] = "mutated group"
		cloned.ProjectionLabels[0] = "mutated projection"
		require.Equal(t, 3, original.Limit)
		require.Equal(t, []string{"group"}, original.Grouping)
		require.Equal(t, []string{"project"}, original.ProjectionLabels)
	})

	t.Run("canonical materialization is bounded", func(t *testing.T) {
		budget := canonicalSeriesBudget{kind: "series", limit: 2, remaining: 2}
		require.NoError(t, budget.take())
		require.NoError(t, budget.take())
		require.ErrorIs(t, budget.take(), errCanonicalSeriesMaterialization)
	})

	t.Run("canonical post-filter skips nonmatches and charges input", func(t *testing.T) {
		matcher := labels.MustNewMatcher(labels.MatchEqual, "job", "keep")
		filtered := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "metric", "job", "drop"), nil)
		matched := storage.NewListSeries(labels.FromStrings(model.MetricNameLabel, "metric", "job", "keep"), nil)

		seriesBudget := newCanonicalSeriesBudget("series", 2)
		seriesSet := &awareSeriesSet{
			SeriesSet:         &sliceSeriesSet{series: []storage.Series{filtered, matched}, index: -1},
			mapping:           buildLabelMapping("metric", nil),
			canonicalMatchers: []*labels.Matcher{matcher},
			budget:            seriesBudget,
		}
		require.True(t, seriesSet.Next())
		require.Same(t, matched, seriesSet.At())
		require.False(t, seriesSet.Next())
		require.NoError(t, seriesSet.Err())
		require.Zero(t, seriesBudget.remaining)

		chunkBudget := newCanonicalSeriesBudget("chunks", 2)
		matchedChunk := storage.NewSeriesToChunkEncoder(matched)
		chunkSet := &awareChunkSeriesSet{
			ChunkSeriesSet: &sliceChunkSeriesSet{
				series: []storage.ChunkSeries{storage.NewSeriesToChunkEncoder(filtered), matchedChunk},
				index:  -1,
			},
			mapping:           buildLabelMapping("metric", nil),
			canonicalMatchers: []*labels.Matcher{matcher},
			budget:            chunkBudget,
		}
		require.True(t, chunkSet.Next())
		require.Same(t, matchedChunk, chunkSet.At())
		require.False(t, chunkSet.Next())
		require.NoError(t, chunkSet.Err())
		require.Zero(t, chunkBudget.remaining)
	})

	t.Run("label value fan-out is bounded", func(t *testing.T) {
		variants := []matcherVariant{
			{mapping: buildLabelMapping("metric", map[string]string{"old": "new"})},
			{mapping: buildLabelMapping("metric", nil)},
		}
		jobs, err := buildLabelValueJobs(variants, "new")
		require.NoError(t, err)
		require.Len(t, jobs, 3)

		_, err = buildLabelValueJobsUpTo(variants, "new", 2)
		require.ErrorIs(t, err, errSchemaExpansion)
	})
}

type singleSeriesSet struct {
	series storage.Series
	next   bool
}

func (s *singleSeriesSet) Next() bool {
	if s.next {
		return false
	}
	s.next = true
	return true
}

func (s *singleSeriesSet) At() storage.Series              { return s.series }
func (*singleSeriesSet) Err() error                        { return nil }
func (*singleSeriesSet) Warnings() annotations.Annotations { return nil }

type singleChunkSeriesSet struct {
	series storage.ChunkSeries
	next   bool
}

func (s *singleChunkSeriesSet) Next() bool {
	if s.next {
		return false
	}
	s.next = true
	return true
}

func (s *singleChunkSeriesSet) At() storage.ChunkSeries         { return s.series }
func (*singleChunkSeriesSet) Err() error                        { return nil }
func (*singleChunkSeriesSet) Warnings() annotations.Annotations { return nil }

type sliceSeriesSet struct {
	series []storage.Series
	index  int
}

func (s *sliceSeriesSet) Next() bool {
	s.index++
	return s.index < len(s.series)
}

func (s *sliceSeriesSet) At() storage.Series              { return s.series[s.index] }
func (*sliceSeriesSet) Err() error                        { return nil }
func (*sliceSeriesSet) Warnings() annotations.Annotations { return nil }

type sliceChunkSeriesSet struct {
	series []storage.ChunkSeries
	index  int
}

func (s *sliceChunkSeriesSet) Next() bool {
	s.index++
	return s.index < len(s.series)
}

func (s *sliceChunkSeriesSet) At() storage.ChunkSeries         { return s.series[s.index] }
func (*sliceChunkSeriesSet) Err() error                        { return nil }
func (*sliceChunkSeriesSet) Warnings() annotations.Annotations { return nil }

type resolverBudgetCallCounts struct {
	series      int
	chunk       int
	labelNames  int
	labelValues int
}

type optionalSearchStorage struct {
	storage.Storage

	querier      storage.Querier
	chunkQuerier storage.ChunkQuerier
}

func (s *optionalSearchStorage) Querier(int64, int64) (storage.Querier, error) {
	return s.querier, nil
}

func (s *optionalSearchStorage) ChunkQuerier(int64, int64) (storage.ChunkQuerier, error) {
	return s.chunkQuerier, nil
}

type nonSearchQuerier struct {
	storage.Querier
	closes *int
}

func (q *nonSearchQuerier) Close() error {
	*q.closes++
	return nil
}

type nonSearchChunkQuerier struct {
	storage.ChunkQuerier
	closes *int
}

func (q *nonSearchChunkQuerier) Close() error {
	*q.closes++
	return nil
}

type searchCall struct {
	kind     string
	ctx      context.Context
	name     string
	hints    *storage.SearchHints
	matchers []*labels.Matcher
}

type searchRecorder struct {
	calls  []searchCall
	result storage.SearchResultSet
}

func (r *searchRecorder) labelNames(ctx context.Context, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	r.calls = append(r.calls, searchCall{kind: "names", ctx: ctx, hints: hints, matchers: matchers})
	return r.result
}

func (r *searchRecorder) labelValues(ctx context.Context, name string, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	r.calls = append(r.calls, searchCall{kind: "values", ctx: ctx, name: name, hints: hints, matchers: matchers})
	return r.result
}

type searchableQuerier struct {
	*nonSearchQuerier
	recorder *searchRecorder
}

func (q *searchableQuerier) SearchLabelNames(ctx context.Context, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return q.recorder.labelNames(ctx, hints, matchers...)
}

func (q *searchableQuerier) SearchLabelValues(ctx context.Context, name string, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return q.recorder.labelValues(ctx, name, hints, matchers...)
}

type searchableChunkQuerier struct {
	*nonSearchChunkQuerier
	recorder *searchRecorder
}

func (q *searchableChunkQuerier) SearchLabelNames(ctx context.Context, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return q.recorder.labelNames(ctx, hints, matchers...)
}

func (q *searchableChunkQuerier) SearchLabelValues(ctx context.Context, name string, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return q.recorder.labelValues(ctx, name, hints, matchers...)
}

type recordingSearchResultSet struct {
	closes int
}

func (*recordingSearchResultSet) Next() bool                        { return false }
func (*recordingSearchResultSet) At() storage.SearchResult          { return storage.SearchResult{} }
func (*recordingSearchResultSet) Warnings() annotations.Annotations { return nil }
func (*recordingSearchResultSet) Err() error                        { return nil }

func (s *recordingSearchResultSet) Close() error {
	s.closes++
	return nil
}

func TestAwareStoragePreservesOptionalSearcher(t *testing.T) {
	assertDelegates := func(t *testing.T, searcher storage.Searcher, recorder *searchRecorder) {
		t.Helper()
		type contextKey struct{}
		ctx := context.WithValue(t.Context(), contextKey{}, "sentinel")
		hints := &storage.SearchHints{Limit: 7}
		matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "job", "api")}

		result := searcher.SearchLabelNames(ctx, hints, matchers...)
		require.Same(t, recorder.result, result)
		require.NoError(t, result.Close())
		result = searcher.SearchLabelValues(ctx, "instance", hints, matchers...)
		require.Same(t, recorder.result, result)
		require.NoError(t, result.Close())

		require.Equal(t, []searchCall{
			{kind: "names", ctx: ctx, hints: hints, matchers: matchers},
			{kind: "values", ctx: ctx, name: "instance", hints: hints, matchers: matchers},
		}, recorder.calls)
	}
	assertRejectsReserved := func(t *testing.T, searcher storage.Searcher, recorder *searchRecorder) {
		t.Helper()
		calls := len(recorder.calls)
		for _, reserved := range []string{semconvURLLabel, schemaURLLabel} {
			matchers := []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "job", "api"),
				labels.MustNewMatcher(labels.MatchEqual, reserved, "registry/test"),
			}
			for _, result := range []storage.SearchResultSet{
				searcher.SearchLabelNames(t.Context(), nil, matchers...),
				searcher.SearchLabelValues(t.Context(), "instance", nil, matchers...),
			} {
				require.False(t, result.Next())
				require.ErrorIs(t, result.Err(), errSchemaAwareSearchUnsupported)
				require.NoError(t, result.Close())
			}
		}
		require.Len(t, recorder.calls, calls)
	}

	t.Run("does not advertise absent capability", func(t *testing.T) {
		querierCloses, chunkCloses := 0, 0
		wrapped := AwareStorage(&optionalSearchStorage{
			querier:      &nonSearchQuerier{Querier: storage.NoopQuerier(), closes: &querierCloses},
			chunkQuerier: &nonSearchChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), closes: &chunkCloses},
		})

		querier, err := wrapped.Querier(0, 1)
		require.NoError(t, err)
		_, ok := querier.(storage.Searcher)
		require.False(t, ok)
		require.NoError(t, querier.Close())

		chunkQuerier, err := wrapped.ChunkQuerier(0, 1)
		require.NoError(t, err)
		_, ok = chunkQuerier.(storage.Searcher)
		require.False(t, ok)
		require.NoError(t, chunkQuerier.Close())
		require.Equal(t, 1, querierCloses)
		require.Equal(t, 1, chunkCloses)
	})

	t.Run("delegates present capability", func(t *testing.T) {
		querierCloses, chunkCloses := 0, 0
		querierResult := &recordingSearchResultSet{}
		chunkResult := &recordingSearchResultSet{}
		querierRecorder := &searchRecorder{result: querierResult}
		chunkRecorder := &searchRecorder{result: chunkResult}
		wrapped := AwareStorage(&optionalSearchStorage{
			querier: &searchableQuerier{
				nonSearchQuerier: &nonSearchQuerier{Querier: storage.NoopQuerier(), closes: &querierCloses},
				recorder:         querierRecorder,
			},
			chunkQuerier: &searchableChunkQuerier{
				nonSearchChunkQuerier: &nonSearchChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), closes: &chunkCloses},
				recorder:              chunkRecorder,
			},
		})

		querier, err := wrapped.Querier(0, 1)
		require.NoError(t, err)
		searcher, ok := querier.(storage.Searcher)
		require.True(t, ok)
		assertDelegates(t, searcher, querierRecorder)
		assertRejectsReserved(t, searcher, querierRecorder)
		require.NoError(t, querier.Close())

		chunkQuerier, err := wrapped.ChunkQuerier(0, 1)
		require.NoError(t, err)
		searcher, ok = chunkQuerier.(storage.Searcher)
		require.True(t, ok)
		assertDelegates(t, searcher, chunkRecorder)
		assertRejectsReserved(t, searcher, chunkRecorder)
		require.NoError(t, chunkQuerier.Close())

		require.Equal(t, 2, querierResult.closes)
		require.Equal(t, 2, chunkResult.closes)
		require.Equal(t, 1, querierCloses)
		require.Equal(t, 1, chunkCloses)
	})
}

type resolverBudgetQuerier struct {
	storage.Querier
	calls *resolverBudgetCallCounts
}

func (q *resolverBudgetQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	q.calls.series++
	return storage.NoopSeriesSet()
}

func (q *resolverBudgetQuerier) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.calls.labelNames++
	return nil, nil, nil
}

func (q *resolverBudgetQuerier) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.calls.labelValues++
	return nil, nil, nil
}

type resolverBudgetChunkQuerier struct {
	storage.ChunkQuerier
	calls *resolverBudgetCallCounts
}

func (q *resolverBudgetChunkQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.ChunkSeriesSet {
	q.calls.chunk++
	return storage.NoopChunkedSeriesSet()
}

func (q *resolverBudgetChunkQuerier) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.calls.labelNames++
	return nil, nil, nil
}

func (q *resolverBudgetChunkQuerier) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.calls.labelValues++
	return nil, nil, nil
}

func TestSchemaExpansionFailsBeforeStorage(t *testing.T) {
	engine := newSchemaEngine(embeddedRegistry)
	engine.limits = schemaExpansionLimits{work: 1, keyBytes: 1_000}
	calls := &resolverBudgetCallCounts{}
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
		labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "registry/registry.yaml"),
	}

	querier := &awareQuerier{
		Querier:              &resolverBudgetQuerier{Querier: storage.NoopQuerier(), calls: calls},
		engine:               engine,
		canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
	}
	series := querier.Select(t.Context(), true, nil, matchers...)
	require.False(t, series.Next())
	require.ErrorIs(t, series.Err(), errSchemaExpansion)

	chunkQuerier := &awareChunkQuerier{
		ChunkQuerier:         &resolverBudgetChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), calls: calls},
		engine:               engine,
		canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
	}
	chunks := chunkQuerier.Select(t.Context(), true, nil, matchers...)
	require.False(t, chunks.Next())
	require.ErrorIs(t, chunks.Err(), errSchemaExpansion)

	_, _, err := querier.LabelNames(t.Context(), nil, matchers...)
	require.ErrorIs(t, err, errSchemaExpansion)
	_, _, err = querier.LabelValues(t.Context(), "tenant", nil, matchers...)
	require.ErrorIs(t, err, errSchemaExpansion)

	require.Equal(t, &resolverBudgetCallCounts{}, calls)
}

func TestUnsafeSchemaMatcherFailsBeforeStorage(t *testing.T) {
	registry := map[string][]byte{
		"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              attr.old: attr.current
`),
		"1.1.0": []byte(`groups:
  - id: metric.metric
    type: metric
    metric_name: metric
    instrument: counter
    unit: "1"
`),
	}
	engine := newSchemaEngine(newRegistrySource(registry))
	calls := &resolverBudgetCallCounts{}
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric"),
		labels.MustNewMatcher(labels.MatchNotEqual, "attr.current", "excluded"),
		labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "registry/registry.yaml"),
	}

	querier := &awareQuerier{
		Querier:              &resolverBudgetQuerier{Querier: storage.NoopQuerier(), calls: calls},
		engine:               engine,
		canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
	}
	series := querier.Select(t.Context(), true, nil, matchers...)
	require.False(t, series.Next())
	require.ErrorIs(t, series.Err(), errUnsafeSchemaMatcher)

	chunkQuerier := &awareChunkQuerier{
		ChunkQuerier:         &resolverBudgetChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), calls: calls},
		engine:               engine,
		canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
	}
	chunks := chunkQuerier.Select(t.Context(), true, nil, matchers...)
	require.False(t, chunks.Next())
	require.ErrorIs(t, chunks.Err(), errUnsafeSchemaMatcher)

	_, _, err := querier.LabelNames(t.Context(), nil, matchers...)
	require.ErrorIs(t, err, errUnsafeSchemaMatcher)
	_, _, err = querier.LabelValues(t.Context(), "attr.current", nil, matchers...)
	require.ErrorIs(t, err, errUnsafeSchemaMatcher)
	require.Equal(t, &resolverBudgetCallCounts{}, calls)
}

func TestAmbiguousSchemaReuseFailsBeforeStorage(t *testing.T) {
	metricReuseSchema := `file_format: 1.1.0
schema_url: https://example.com/schemas/1.2.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            foo: bar
  1.2.0:
    metrics:
      changes:
        - rename_metrics:
            baz: foo
`
	for _, tc := range []struct {
		name       string
		metric     string
		attribute  string
		labelValue string
		schema     string
	}{
		{
			name:       "historical metric alias",
			metric:     "bar",
			labelValue: model.MetricNameLabel,
			schema:     metricReuseSchema,
		},
		{
			name:       "reused metric anchor",
			metric:     "foo",
			labelValue: model.MetricNameLabel,
			schema:     metricReuseSchema,
		},
		{
			name:       "reused attribute alias",
			metric:     "metric",
			attribute:  "user",
			labelValue: "user",
			schema: `file_format: 1.1.0
schema_url: https://example.com/schemas/1.2.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              user: tenant
  1.2.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              account: user
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := map[string][]byte{
				"registry.yaml": []byte(tc.schema),
				"1.2.0": []byte(`groups:
  - id: metric.metric
    type: metric
    metric_name: metric
    instrument: counter
    unit: "1"
  - id: metric.foo
    type: metric
    metric_name: foo
    instrument: counter
    unit: "1"
  - id: metric.bar
    type: metric
    metric_name: bar
    instrument: counter
    unit: "1"
`),
			}
			engine := newSchemaEngine(newRegistrySource(registry))
			calls := &resolverBudgetCallCounts{}
			matchers := []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, tc.metric),
				labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "registry/1.2.0"),
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "registry/registry.yaml"),
			}
			if tc.attribute != "" {
				matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, tc.attribute, "value"))
			}

			querier := &awareQuerier{
				Querier:              &resolverBudgetQuerier{Querier: storage.NoopQuerier(), calls: calls},
				engine:               engine,
				canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
			}
			series := querier.Select(t.Context(), true, nil, matchers...)
			require.False(t, series.Next())
			require.ErrorIs(t, series.Err(), errAmbiguousSchemaRename)

			chunkQuerier := &awareChunkQuerier{
				ChunkQuerier:         &resolverBudgetChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), calls: calls},
				engine:               engine,
				canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
			}
			chunks := chunkQuerier.Select(t.Context(), true, nil, matchers...)
			require.False(t, chunks.Next())
			require.ErrorIs(t, chunks.Err(), errAmbiguousSchemaRename)

			_, _, err := querier.LabelNames(t.Context(), nil, matchers...)
			require.ErrorIs(t, err, errAmbiguousSchemaRename)
			_, _, err = querier.LabelValues(t.Context(), tc.labelValue, nil, matchers...)
			require.ErrorIs(t, err, errAmbiguousSchemaRename)
			require.Equal(t, &resolverBudgetCallCounts{}, calls)
		})
	}
}

func TestForwardMetricConvergenceFansOutToStorage(t *testing.T) {
	registry := map[string][]byte{
		"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            metric.old.one: metric.current
            metric.old.two: metric.current
`),
		"1.0.0": []byte(`groups:
  - id: metric.metric.old.one
    type: metric
    metric_name: metric.old.one
    instrument: counter
    unit: "1"
`),
	}
	engine := newSchemaEngine(newRegistrySource(registry))
	calls := &resolverBudgetCallCounts{}
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.old.one"),
		labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "registry/1.0.0"),
		labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "registry/registry.yaml"),
	}

	querier := &awareQuerier{
		Querier:              &resolverBudgetQuerier{Querier: storage.NoopQuerier(), calls: calls},
		engine:               engine,
		canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
	}
	series := querier.Select(t.Context(), true, nil, matchers...)
	require.False(t, series.Next())
	require.NoError(t, series.Err())

	chunkQuerier := &awareChunkQuerier{
		ChunkQuerier:         &resolverBudgetChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), calls: calls},
		engine:               engine,
		canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
	}
	chunks := chunkQuerier.Select(t.Context(), true, nil, matchers...)
	require.False(t, chunks.Next())
	require.NoError(t, chunks.Err())

	_, _, err := querier.LabelNames(t.Context(), nil, matchers...)
	require.NoError(t, err)
	_, _, err = querier.LabelValues(t.Context(), model.MetricNameLabel, nil, matchers...)
	require.NoError(t, err)
	require.Positive(t, calls.series)
	require.Positive(t, calls.chunk)
	require.Positive(t, calls.labelNames)
	require.Positive(t, calls.labelValues)
}

func TestSchemaConvergenceFansOutToStorage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		metric    string
		attribute string
		schema    string
	}{
		{
			name:      "within one change",
			metric:    "metric",
			attribute: "attr.current",
			schema: `file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              attr.old.one: attr.current
              attr.old.two: attr.current
`,
		},
		{
			name:      "across ordered changes",
			metric:    "metric",
			attribute: "attr.current",
			schema: `file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              attr.old.one: attr.current
        - rename_attributes:
            attribute_map:
              attr.old.two: attr.current
`,
		},
		{
			name:      "through an intermediate predecessor",
			metric:    "metric",
			attribute: "attr.current",
			schema: `file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              attr.old: attr.current
        - rename_attributes:
            attribute_map:
              attr.old: attr.middle
        - rename_attributes:
            attribute_map:
              attr.middle: attr.current
`,
		},
		{
			name:      "attribute convergence across revisions",
			metric:    "metric",
			attribute: "attr.current",
			schema: `file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              attr.old.one: attr.current
  1.1.0:
    metrics:
      changes:
        - rename_attributes:
            attribute_map:
              attr.old.two: attr.current
`,
		},
		{
			name:   "metric convergence across revisions",
			metric: "metric.current",
			schema: `file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
    metrics:
      changes:
        - rename_metrics:
            metric.old.one: metric.current
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            metric.old.two: metric.current
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := map[string][]byte{
				"registry.yaml": []byte(tc.schema),
				"1.1.0": []byte(`groups:
  - id: metric.metric
    type: metric
    metric_name: metric
    instrument: counter
    unit: "1"
  - id: metric.metric.current
    type: metric
    metric_name: metric.current
    instrument: counter
    unit: "1"
`),
			}
			engine := newSchemaEngine(newRegistrySource(registry))
			calls := &resolverBudgetCallCounts{}
			matchers := []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, tc.metric),
				labels.MustNewMatcher(labels.MatchEqual, semconvURLLabel, "registry/1.1.0"),
				labels.MustNewMatcher(labels.MatchEqual, schemaURLLabel, "registry/registry.yaml"),
			}
			if tc.attribute != "" {
				matchers = append(matchers, labels.MustNewMatcher(labels.MatchEqual, tc.attribute, "value"))
			}

			querier := &awareQuerier{
				Querier:              &resolverBudgetQuerier{Querier: storage.NoopQuerier(), calls: calls},
				engine:               engine,
				canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
			}
			series := querier.Select(t.Context(), true, nil, matchers...)
			require.False(t, series.Next())
			require.NoError(t, series.Err())

			chunkQuerier := &awareChunkQuerier{
				ChunkQuerier:         &resolverBudgetChunkQuerier{ChunkQuerier: storage.NoopChunkedQuerier(), calls: calls},
				engine:               engine,
				canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
			}
			chunks := chunkQuerier.Select(t.Context(), true, nil, matchers...)
			require.False(t, chunks.Next())
			require.NoError(t, chunks.Err())

			_, _, err := querier.LabelNames(t.Context(), nil, matchers...)
			require.NoError(t, err)
			labelName := tc.attribute
			if labelName == "" {
				labelName = model.MetricNameLabel
			}
			_, _, err = querier.LabelValues(t.Context(), labelName, nil, matchers...)
			require.NoError(t, err)
			require.Positive(t, calls.series)
			require.Positive(t, calls.chunk)
			require.Positive(t, calls.labelNames)
			require.Positive(t, calls.labelValues)
		})
	}
}
