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
	"strconv"
	"sync"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

func schemaReadMatchers() []*labels.Matcher {
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
}

func TestSchemaAwareSelectSchedulesBeforeIteration(t *testing.T) {
	primary := teststorage.New(t)
	secondary := teststorage.New(t)
	appendSeries(t, secondary, "test.counter", 1, 1, "user", "acme")
	appendSeries(t, secondary, "up", 1, 2, "instance", "secondary")
	wrapper := semconv.AwareStorage(storage.NewFanout(nil, primary, secondary))

	t.Run("series", func(t *testing.T) {
		q, err := wrapper.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		schemaSet := q.Select(t.Context(), true, nil, schemaReadMatchers()...)
		secondSchemaSet := q.Select(t.Context(), true, nil, schemaReadMatchers()...)
		plainSet := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "up"))

		schemaSeries := collectSeries(t, schemaSet)
		require.Len(t, schemaSeries, 1)
		for got := range schemaSeries {
			require.Contains(t, got, `__name__="test"`)
			require.Contains(t, got, `tenant="acme"`)
		}
		require.Len(t, collectSeries(t, secondSchemaSet), 1)
		require.Len(t, collectSeries(t, plainSet), 1)
	})

	t.Run("chunk series", func(t *testing.T) {
		q, err := wrapper.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		schemaSet := q.Select(t.Context(), true, nil, schemaReadMatchers()...)
		secondSchemaSet := q.Select(t.Context(), true, nil, schemaReadMatchers()...)
		plainSet := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "up"))

		schemaSeries := collectSeries(t, storage.NewSeriesSetFromChunkSeriesSet(schemaSet))
		require.Len(t, schemaSeries, 1)
		for got := range schemaSeries {
			require.Contains(t, got, `__name__="test"`)
			require.Contains(t, got, `tenant="acme"`)
		}
		require.Len(t, collectSeries(t, storage.NewSeriesSetFromChunkSeriesSet(secondSchemaSet)), 1)
		require.Len(t, collectSeries(t, storage.NewSeriesSetFromChunkSeriesSet(plainSet)), 1)
	})
}

type contractHintStorage struct {
	storage.Storage

	mu               sync.Mutex
	seriesHints      []*storage.SelectHints
	chunkHints       []*storage.SelectHints
	seriesSort       []bool
	chunkSort        []bool
	labelNamesHints  []*storage.LabelHints
	labelValuesHints []*storage.LabelHints
	seriesNextCalls  int
	chunkNextCalls   int
	labelNamesCalls  int
	labelValuesCalls int
	labelNamesErr    error
}

type contractStorageCalls struct {
	seriesSelects int
	chunkSelects  int
	seriesNext    int
	chunkNext     int
	labelNames    int
	labelValues   int
}

func copyRecordedHints(hints *storage.SelectHints) *storage.SelectHints {
	if hints == nil {
		return nil
	}
	cloned := *hints
	cloned.Grouping = slices.Clone(hints.Grouping)
	cloned.ProjectionLabels = slices.Clone(hints.ProjectionLabels)
	return &cloned
}

func copyRecordedLabelHints(hints *storage.LabelHints) *storage.LabelHints {
	if hints == nil {
		return nil
	}
	cloned := *hints
	return &cloned
}

func (s *contractHintStorage) recordSeries(sortSeries bool, hints *storage.SelectHints) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seriesSort = append(s.seriesSort, sortSeries)
	s.seriesHints = append(s.seriesHints, copyRecordedHints(hints))
}

func (s *contractHintStorage) recordChunk(sortSeries bool, hints *storage.SelectHints) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunkSort = append(s.chunkSort, sortSeries)
	s.chunkHints = append(s.chunkHints, copyRecordedHints(hints))
}

func (s *contractHintStorage) recordedHints() (series, chunks []*storage.SelectHints) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seriesHints), slices.Clone(s.chunkHints)
}

func (s *contractHintStorage) recordedSort() (series, chunks []bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seriesSort), slices.Clone(s.chunkSort)
}

func (s *contractHintStorage) recordedLabelHints() (names, values []*storage.LabelHints) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.labelNamesHints), slices.Clone(s.labelValuesHints)
}

func enforceRecordedLabelLimit(values []string, hints *storage.LabelHints) []string {
	if hints != nil && hints.Limit > 0 && len(values) > hints.Limit {
		return values[:hints.Limit]
	}
	return values
}

func (s *contractHintStorage) calls() contractStorageCalls {
	s.mu.Lock()
	defer s.mu.Unlock()
	return contractStorageCalls{
		seriesSelects: len(s.seriesHints),
		chunkSelects:  len(s.chunkHints),
		seriesNext:    s.seriesNextCalls,
		chunkNext:     s.chunkNextCalls,
		labelNames:    s.labelNamesCalls,
		labelValues:   s.labelValuesCalls,
	}
}

func (s *contractHintStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &contractHintQuerier{Querier: q, storage: s}, nil
}

func (s *contractHintStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &contractHintChunkQuerier{ChunkQuerier: q, storage: s}, nil
}

type contractHintQuerier struct {
	storage.Querier
	storage *contractHintStorage
}

func (q *contractHintQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	q.storage.recordSeries(sortSeries, hints)
	return &contractHintSeriesSet{
		SeriesSet: q.Querier.Select(ctx, sortSeries, hints, matchers...),
		hints:     copyRecordedHints(hints),
		storage:   q.storage,
	}
}

func (q *contractHintQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.storage.mu.Lock()
	q.storage.labelNamesCalls++
	q.storage.labelNamesHints = append(q.storage.labelNamesHints, copyRecordedLabelHints(hints))
	err := q.storage.labelNamesErr
	q.storage.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	names, anns, err := q.Querier.LabelNames(ctx, hints, matchers...)
	return enforceRecordedLabelLimit(names, hints), anns, err
}

func (q *contractHintQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.storage.mu.Lock()
	q.storage.labelValuesCalls++
	q.storage.labelValuesHints = append(q.storage.labelValuesHints, copyRecordedLabelHints(hints))
	q.storage.mu.Unlock()
	values, anns, err := q.Querier.LabelValues(ctx, name, hints, matchers...)
	return enforceRecordedLabelLimit(values, hints), anns, err
}

type contractHintChunkQuerier struct {
	storage.ChunkQuerier
	storage *contractHintStorage
}

func (q *contractHintChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	q.storage.recordChunk(sortSeries, hints)
	return &contractHintChunkSeriesSet{
		ChunkSeriesSet: q.ChunkQuerier.Select(ctx, sortSeries, hints, matchers...),
		hints:          copyRecordedHints(hints),
		storage:        q.storage,
	}
}

func (q *contractHintChunkQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.storage.mu.Lock()
	q.storage.labelNamesCalls++
	q.storage.labelNamesHints = append(q.storage.labelNamesHints, copyRecordedLabelHints(hints))
	err := q.storage.labelNamesErr
	q.storage.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	names, anns, err := q.ChunkQuerier.LabelNames(ctx, hints, matchers...)
	return enforceRecordedLabelLimit(names, hints), anns, err
}

func (q *contractHintChunkQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.storage.mu.Lock()
	q.storage.labelValuesCalls++
	q.storage.labelValuesHints = append(q.storage.labelValuesHints, copyRecordedLabelHints(hints))
	q.storage.mu.Unlock()
	values, anns, err := q.ChunkQuerier.LabelValues(ctx, name, hints, matchers...)
	return enforceRecordedLabelLimit(values, hints), anns, err
}

type contractHintSeriesSet struct {
	storage.SeriesSet
	hints   *storage.SelectHints
	storage *contractHintStorage
	seen    int
	at      storage.Series
}

func (s *contractHintSeriesSet) Next() bool {
	s.storage.mu.Lock()
	s.storage.seriesNextCalls++
	s.storage.mu.Unlock()
	if s.hints != nil && s.hints.Limit > 0 && s.seen >= s.hints.Limit {
		return false
	}
	if !s.SeriesSet.Next() {
		return false
	}
	s.seen++
	series := s.SeriesSet.At()
	s.at = &contractHintSeries{Series: series, labels: projectLabels(series.Labels(), s.hints)}
	return true
}

func (s *contractHintSeriesSet) At() storage.Series {
	return s.at
}

type contractHintSeries struct {
	storage.Series
	labels labels.Labels
}

func (s *contractHintSeries) Labels() labels.Labels {
	return s.labels
}

type contractHintChunkSeriesSet struct {
	storage.ChunkSeriesSet
	hints   *storage.SelectHints
	storage *contractHintStorage
	seen    int
	at      storage.ChunkSeries
}

func (s *contractHintChunkSeriesSet) Next() bool {
	s.storage.mu.Lock()
	s.storage.chunkNextCalls++
	s.storage.mu.Unlock()
	if s.hints != nil && s.hints.Limit > 0 && s.seen >= s.hints.Limit {
		return false
	}
	if !s.ChunkSeriesSet.Next() {
		return false
	}
	s.seen++
	series := s.ChunkSeriesSet.At()
	s.at = &contractHintChunkSeries{ChunkSeries: series, labels: projectLabels(series.Labels(), s.hints)}
	return true
}

func (s *contractHintChunkSeriesSet) At() storage.ChunkSeries {
	return s.at
}

type contractHintChunkSeries struct {
	storage.ChunkSeries
	labels labels.Labels
}

func (s *contractHintChunkSeries) Labels() labels.Labels {
	return s.labels
}

func projectLabels(full labels.Labels, hints *storage.SelectHints) labels.Labels {
	if hints == nil || len(hints.ProjectionLabels) == 0 {
		return full
	}
	projected := make(map[string]struct{}, len(hints.ProjectionLabels))
	for _, name := range hints.ProjectionLabels {
		projected[name] = struct{}{}
	}

	builder := labels.NewScratchBuilder(full.Len() + 1)
	full.Range(func(label labels.Label) {
		_, listed := projected[label.Name]
		if listed == hints.ProjectionInclude {
			builder.Add(label.Name, label.Value)
		}
	})
	builder.Add("__series_hash__", strconv.FormatUint(labels.StableHash(full), 10))
	builder.Sort()
	return builder.Labels()
}

func shardFixtureValues(t *testing.T) (excluded, included string) {
	t.Helper()
	for i := range 1000 {
		value := fmt.Sprintf("value-%03d", i)
		physical := labels.FromStrings(model.MetricNameLabel, "test.counter", "user", value)
		canonical := labels.FromStrings(model.MetricNameLabel, "test", "tenant", value)
		if labels.StableHash(physical)%2 != 0 {
			continue
		}
		switch labels.StableHash(canonical) % 2 {
		case 1:
			if excluded == "" {
				excluded = value
			}
		case 0:
			if excluded != "" {
				return excluded, value
			}
		}
	}
	t.Fatal("could not construct stable shard fixture")
	return "", ""
}

func TestSchemaAwareSelectAppliesHintsToCanonicalLabels(t *testing.T) {
	t.Run("clears unsafe physical hints", func(t *testing.T) {
		excluded, included := shardFixtureValues(t)
		underlying := teststorage.New(t)
		contractStorage := &contractHintStorage{Storage: underlying}
		wrapper := semconv.AwareStorage(contractStorage)
		appendSeries(t, underlying, "test.counter", 1, 1, "user", excluded)
		appendSeries(t, underlying, "test.counter", 1, 2, "user", included)

		hints := &storage.SelectHints{
			Start:             0,
			End:               10,
			Limit:             1,
			Func:              "sum",
			Grouping:          []string{"tenant"},
			By:                true,
			ShardCount:        2,
			ShardIndex:        0,
			ProjectionLabels:  []string{"tenant"},
			ProjectionInclude: true,
		}
		wantHints := copyRecordedHints(hints)

		q, err := wrapper.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })
		gotSeries := collectSeries(t, q.Select(t.Context(), true, hints, schemaReadMatchers()...))
		require.Len(t, gotSeries, 1)
		for got := range gotSeries {
			require.Contains(t, got, `__name__="test"`)
			require.Contains(t, got, `tenant="`+included+`"`)
			require.NotContains(t, got, excluded)
			require.NotContains(t, got, "__series_hash__")
		}

		cq, err := wrapper.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, cq.Close()) })
		gotChunks := collectSeries(t, storage.NewSeriesSetFromChunkSeriesSet(
			cq.Select(t.Context(), true, hints, schemaReadMatchers()...),
		))
		require.Len(t, gotChunks, 1)
		for got := range gotChunks {
			require.Contains(t, got, `tenant="`+included+`"`)
			require.NotContains(t, got, "__series_hash__")
		}

		require.Equal(t, wantHints, hints, "schema fan-out must not mutate caller-owned hints")
		seriesHints, chunkHints := contractStorage.recordedHints()
		for _, recorded := range append(seriesHints, chunkHints...) {
			require.NotNil(t, recorded)
			require.Zero(t, recorded.Limit)
			require.Zero(t, recorded.ShardCount)
			require.Zero(t, recorded.ShardIndex)
			require.Nil(t, recorded.ProjectionLabels)
			require.False(t, recorded.ProjectionInclude)
			require.Empty(t, recorded.Func)
			require.Nil(t, recorded.Grouping)
			require.False(t, recorded.By)
		}
	})

	t.Run("preserves metadata-only series token", func(t *testing.T) {
		underlying := teststorage.New(t)
		contractStorage := &contractHintStorage{Storage: underlying}
		wrapper := semconv.AwareStorage(contractStorage)
		appendSeries(t, underlying, "test.counter", 1, 1, "user", "acme")

		hints := &storage.SelectHints{
			Start:             0,
			End:               10,
			Func:              "series",
			Grouping:          []string{"tenant"},
			By:                true,
			ProjectionLabels:  []string{"tenant"},
			ProjectionInclude: true,
		}
		wantHints := copyRecordedHints(hints)

		q, err := wrapper.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })
		_ = q.Select(t.Context(), true, hints, schemaReadMatchers()...)

		cq, err := wrapper.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, cq.Close()) })
		_ = cq.Select(t.Context(), true, hints, schemaReadMatchers()...)

		require.Equal(t, wantHints, hints, "schema fan-out must not mutate caller-owned hints")
		seriesHints, chunkHints := contractStorage.recordedHints()
		require.Greater(t, len(seriesHints), 1, "series metadata lookup must fan out")
		require.Greater(t, len(chunkHints), 1, "chunk metadata lookup must fan out")
		for _, recorded := range append(seriesHints, chunkHints...) {
			require.NotNil(t, recorded)
			require.Equal(t, "series", recorded.Func)
			require.Nil(t, recorded.Grouping)
			require.False(t, recorded.By)
			require.Nil(t, recorded.ProjectionLabels)
			require.False(t, recorded.ProjectionInclude)
		}
	})
}

func contractFanOutRegistry(variants int) map[string][]byte {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/current
versions:
  1.0.0:
`)
	for i := 1; i < variants; i++ {
		oldName := fmt.Sprintf("metric.old.%02d", i-1)
		newName := fmt.Sprintf("metric.old.%02d", i)
		if i == variants-1 {
			newName = "metric.current"
		}
		schema = fmt.Appendf(schema, `  1.%d.0:
    metrics:
      changes:
        - rename_metrics:
            %s: %s
`, i, oldName, newName)
	}
	version := fmt.Sprintf("1.%d.0", variants-1)
	return map[string][]byte{
		"registry.yaml": schema,
		version: []byte(`groups:
  - id: metric.metric.current
    type: metric
    brief: "A metric"
    stability: stable
    metric_name: metric.current
    instrument: counter
    unit: "1"
`),
	}
}

func contractFanOutMatchers(variants int) []*labels.Matcher {
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric.current"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", fmt.Sprintf("registry/1.%d.0", variants-1)),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
}

func newContractFanOutStorage(t testing.TB, variants int) (storage.Storage, *contractHintStorage) {
	t.Helper()
	underlying := &contractHintStorage{Storage: teststorage.New(t)}
	wrapper, err := semconv.AwareStorageWithRegistry(underlying, contractFanOutRegistry(variants))
	require.NoError(t, err)
	return wrapper, underlying
}

func newCanonicalLimitStorage(t *testing.T, storedControlLabels bool) (storage.Storage, *contractHintStorage) {
	t.Helper()
	underlying := &contractHintStorage{Storage: teststorage.New(t)}
	appendSeries(t, underlying, "test.counter", 1, 1, "trace", "x")
	appendSeries(t, underlying, "test.counter", 1, 2, "user", "a")
	if storedControlLabels {
		appendSeries(t, underlying, "test.counter", 1, 3,
			"user", "c",
			"__schema_url__", "stored",
			"__semconv_url__", "stored",
		)
	} else {
		appendSeries(t, underlying, "test.counter", 1, 3, "user", "c")
	}
	appendSeries(t, underlying, "test", 1, 4, "tenant", "b")
	appendSeries(t, underlying, "test", 1, 5, "tenant", "z")
	return semconv.AwareStorage(underlying), underlying
}

func TestSchemaAwareLimitsApplyAfterCanonicalization(t *testing.T) {
	for _, query := range []string{"series", "chunks"} {
		t.Run(query, func(t *testing.T) {
			wrapper, _ := newCanonicalLimitStorage(t, false)
			hints := &storage.SelectHints{Start: 0, End: 10, Step: 5, Limit: 1}
			wantHints := copyRecordedHints(hints)
			wantLabels := labels.FromStrings(model.MetricNameLabel, "test", "tenant", "a")

			if query == "series" {
				q, err := wrapper.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				require.Equal(t, map[string]float64{wantLabels.String(): 2},
					collectSeries(t, q.Select(t.Context(), false, hints, schemaReadMatchers()...)))
			} else {
				q, err := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				set := q.Select(t.Context(), false, hints, schemaReadMatchers()...)
				require.True(t, set.Next())
				require.Equal(t, wantLabels, set.At().Labels())
				require.False(t, set.Next())
				require.NoError(t, set.Err())
			}

			require.Equal(t, wantHints, hints, "schema fan-out must not mutate caller-owned hints")
		})
	}

	for _, query := range []string{"querier", "chunk querier"} {
		t.Run("label names/"+query, func(t *testing.T) {
			wrapper, underlying := newCanonicalLimitStorage(t, true)
			hints := &storage.LabelHints{Limit: 2}

			var (
				names []string
				err   error
			)
			if query == "querier" {
				q, qerr := wrapper.Querier(0, 10)
				require.NoError(t, qerr)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				names, _, err = q.LabelNames(t.Context(), hints, schemaReadMatchers()...)
			} else {
				q, qerr := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, qerr)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				names, _, err = q.LabelNames(t.Context(), hints, schemaReadMatchers()...)
			}

			require.NoError(t, err)
			require.Equal(t, []string{model.MetricNameLabel, "tenant"}, names)
			require.Equal(t, 2, hints.Limit, "schema fan-out must not mutate caller-owned hints")
			nameHints, _ := underlying.recordedLabelHints()
			require.Greater(t, len(nameHints), 1)
			for _, recorded := range nameHints {
				require.Equal(t, &storage.LabelHints{}, recorded,
					"label-name limits must be applied after reserved-label filtering and canonicalization")
			}
		})

		t.Run("label values/"+query, func(t *testing.T) {
			wrapper, underlying := newCanonicalLimitStorage(t, false)
			hints := &storage.LabelHints{Limit: 1}

			var (
				values []string
				err    error
			)
			if query == "querier" {
				q, qerr := wrapper.Querier(0, 10)
				require.NoError(t, qerr)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				values, _, err = q.LabelValues(t.Context(), "tenant", hints, schemaReadMatchers()...)
			} else {
				q, qerr := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, qerr)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				values, _, err = q.LabelValues(t.Context(), "tenant", hints, schemaReadMatchers()...)
			}

			require.NoError(t, err)
			require.Equal(t, []string{"a"}, values)
			require.Equal(t, 1, hints.Limit, "schema fan-out must not mutate caller-owned hints")
			_, valueHints := underlying.recordedLabelHints()
			require.Greater(t, len(valueHints), 1)
			for _, recorded := range valueHints {
				require.Equal(t, hints, recorded, "safe per-alias limit pushdown should be retained")
			}
		})
	}

	t.Run("nil and zero limits stay disabled", func(t *testing.T) {
		wrapper, _ := newCanonicalLimitStorage(t, false)
		q, err := wrapper.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		require.Len(t, collectSeries(t, q.Select(t.Context(), false, nil, schemaReadMatchers()...)), 5)
		zeroSelectHints := &storage.SelectHints{Start: 0, End: 10}
		require.Len(t, collectSeries(t, q.Select(t.Context(), false, zeroSelectHints, schemaReadMatchers()...)), 5)
		require.Zero(t, zeroSelectHints.Limit)

		names, _, err := q.LabelNames(t.Context(), nil, schemaReadMatchers()...)
		require.NoError(t, err)
		require.Equal(t, []string{model.MetricNameLabel, "tenant", "trace"}, names)
		zeroLabelHints := &storage.LabelHints{}
		values, _, err := q.LabelValues(t.Context(), "tenant", zeroLabelHints, schemaReadMatchers()...)
		require.NoError(t, err)
		require.Equal(t, []string{"a", "b", "c", "z"}, values)
		require.Zero(t, zeroLabelHints.Limit)
	})
}

func TestSchemaAwareStorageFanOutLimitFailsClosed(t *testing.T) {
	for _, variants := range []int{32, 33} {
		wantErr := variants > 32
		t.Run(fmt.Sprintf("%d variants", variants), func(t *testing.T) {
			t.Run("series", func(t *testing.T) {
				wrapper, underlying := newContractFanOutStorage(t, variants)
				q, err := wrapper.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })

				set := q.Select(t.Context(), false, nil, contractFanOutMatchers(variants)...)
				require.False(t, set.Next())
				if wantErr {
					require.ErrorContains(t, set.Err(), "schema expansion limit exceeded")
					require.Zero(t, underlying.calls().seriesSelects)
				} else {
					require.NoError(t, set.Err())
					require.Equal(t, variants, underlying.calls().seriesSelects)
				}
				require.Zero(t, underlying.calls().labelNames)
			})

			t.Run("chunks", func(t *testing.T) {
				wrapper, underlying := newContractFanOutStorage(t, variants)
				q, err := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })

				set := q.Select(t.Context(), false, nil, contractFanOutMatchers(variants)...)
				require.False(t, set.Next())
				if wantErr {
					require.ErrorContains(t, set.Err(), "schema expansion limit exceeded")
					require.Zero(t, underlying.calls().chunkSelects)
				} else {
					require.NoError(t, set.Err())
					require.Equal(t, variants, underlying.calls().chunkSelects)
				}
				require.Zero(t, underlying.calls().labelNames)
			})

			t.Run("label names", func(t *testing.T) {
				wrapper, underlying := newContractFanOutStorage(t, variants)
				q, err := wrapper.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })

				_, _, err = q.LabelNames(t.Context(), nil, contractFanOutMatchers(variants)...)
				if wantErr {
					require.ErrorContains(t, err, "schema expansion limit exceeded")
					require.Zero(t, underlying.calls().labelNames)
				} else {
					require.NoError(t, err)
					require.Equal(t, variants, underlying.calls().labelNames)
				}
			})

			t.Run("label values", func(t *testing.T) {
				wrapper, underlying := newContractFanOutStorage(t, variants)
				q, err := wrapper.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })

				_, _, err = q.LabelValues(t.Context(), model.MetricNameLabel, nil, contractFanOutMatchers(variants)...)
				if wantErr {
					require.ErrorContains(t, err, "schema expansion limit exceeded")
					require.Zero(t, underlying.calls().labelValues)
				} else {
					require.NoError(t, err)
					require.Equal(t, variants, underlying.calls().labelValues)
				}
			})
		})
	}
}

func appendContractSeries(t testing.TB, s storage.Storage, count int) {
	appendContractSeriesForMetric(t, s, "metric.old.00", count)
}

func appendContractSeriesForMetric(t testing.TB, s storage.Storage, metric string, count int) {
	t.Helper()
	app := s.Appender(t.Context())
	for i := range count {
		_, err := app.Append(0, labels.FromStrings(
			model.MetricNameLabel, metric,
			"instance", fmt.Sprintf("instance-%05d", i),
		), 1, float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
}

func TestSchemaAwareMetricOnlySelectStreamsWithoutLabelMetadata(t *testing.T) {
	const seriesCount = 100
	for _, query := range []string{"series", "chunks"} {
		t.Run(query, func(t *testing.T) {
			underlying := &contractHintStorage{
				Storage:       teststorage.New(t),
				labelNamesErr: errors.New("label names unavailable"),
			}
			appendContractSeries(t, underlying, seriesCount)
			wrapper, err := semconv.AwareStorageWithRegistry(underlying, contractFanOutRegistry(2))
			require.NoError(t, err)
			hints := &storage.SelectHints{Start: 0, End: 10, Limit: 1}
			wantHints := copyRecordedHints(hints)

			if query == "series" {
				q, err := wrapper.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				set := q.Select(t.Context(), false, hints, contractFanOutMatchers(2)...)
				require.True(t, set.Next())
				require.False(t, set.Next())
				require.NoError(t, set.Err())
				calls := underlying.calls()
				require.Zero(t, calls.labelNames)
				require.Less(t, calls.seriesNext, seriesCount)
				seriesHints, _ := underlying.recordedHints()
				require.Len(t, seriesHints, 2)
				for _, recorded := range seriesHints {
					require.Equal(t, wantHints, recorded)
				}
				require.Equal(t, wantHints, hints, "schema fan-out must not mutate caller-owned hints")
				return
			}

			q, err := wrapper.ChunkQuerier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, q.Close()) })
			set := q.Select(t.Context(), false, hints, contractFanOutMatchers(2)...)
			require.True(t, set.Next())
			require.False(t, set.Next())
			require.NoError(t, set.Err())
			calls := underlying.calls()
			require.Zero(t, calls.labelNames)
			require.Less(t, calls.chunkNext, seriesCount)
			_, chunkHints := underlying.recordedHints()
			require.Len(t, chunkHints, 2)
			for _, recorded := range chunkHints {
				require.Equal(t, wantHints, recorded)
			}
			require.Equal(t, wantHints, hints, "schema fan-out must not mutate caller-owned hints")
		})
	}
}

func TestSchemaAwareSelectRejectsStoredControlLabels(t *testing.T) {
	for _, labelName := range []string{"__semconv_url__", "__schema_url__"} {
		for _, query := range []string{"series", "chunks"} {
			t.Run(labelName+"/"+query, func(t *testing.T) {
				underlying := teststorage.New(t)
				appendSeries(t, underlying, "test.counter", 1, 1, "user", "acme", labelName, "stored")
				wrapper := semconv.AwareStorage(underlying)

				if query == "series" {
					q, err := wrapper.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, q.Close()) })
					set := q.Select(t.Context(), false, nil, schemaReadMatchers()...)
					require.False(t, set.Next())
					require.ErrorContains(t, set.Err(), "encountered stored control label "+labelName)
					return
				}

				q, err := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, q.Close()) })
				set := q.Select(t.Context(), false, nil, schemaReadMatchers()...)
				require.False(t, set.Next())
				require.ErrorContains(t, set.Err(), "encountered stored control label "+labelName)
			})
		}
	}
}

func TestSchemaAwareIdentitySelectRejectsStoredControlLabels(t *testing.T) {
	for _, labelName := range []string{"__semconv_url__", "__schema_url__"} {
		for _, query := range []string{"series", "chunks"} {
			t.Run(labelName+"/"+query, func(t *testing.T) {
				underlying := &contractHintStorage{Storage: teststorage.New(t)}
				appendSeries(t, underlying, "metric.current", 1, 1, "instance", "a", labelName, "stored")
				wrapper, err := semconv.AwareStorageWithRegistry(underlying, contractFanOutRegistry(1))
				require.NoError(t, err)
				matchers := contractFanOutMatchers(1)
				hints := &storage.SelectHints{
					Start:             0,
					End:               10,
					ProjectionLabels:  []string{"instance"},
					ProjectionInclude: true,
				}
				wantHints := copyRecordedHints(hints)

				if query == "series" {
					querier, err := wrapper.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, querier.Close()) })
					set := querier.Select(t.Context(), false, hints, matchers...)
					require.False(t, set.Next())
					require.ErrorContains(t, set.Err(), "encountered stored control label "+labelName)
					seriesHints, _ := underlying.recordedHints()
					require.Len(t, seriesHints, 1)
					require.Nil(t, seriesHints[0].ProjectionLabels)
					require.False(t, seriesHints[0].ProjectionInclude)
					require.Equal(t, wantHints, hints)
					return
				}

				querier, err := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, querier.Close()) })
				set := querier.Select(t.Context(), false, hints, matchers...)
				require.False(t, set.Next())
				require.ErrorContains(t, set.Err(), "encountered stored control label "+labelName)
				_, chunkHints := underlying.recordedHints()
				require.Len(t, chunkHints, 1)
				require.Nil(t, chunkHints[0].ProjectionLabels)
				require.False(t, chunkHints[0].ProjectionInclude)
				require.Equal(t, wantHints, hints)
			})
		}
	}
}

func TestSchemaAwareIdentitySelectSanitizesUnsafeHints(t *testing.T) {
	for _, funcCase := range []struct {
		name     string
		function string
		wantFunc string
	}{
		{name: "aggregation", function: "sum"},
		{name: "metadata only", function: "series", wantFunc: "series"},
	} {
		for _, query := range []string{"series", "chunks"} {
			t.Run(funcCase.name+"/"+query, func(t *testing.T) {
				underlying := &contractHintStorage{Storage: teststorage.New(t, func(opts *tsdb.Options) {
					opts.EnableSharding = true
				})}
				appendSeries(t, underlying, "metric.current", 1, 1, "instance", "a")
				appendSeries(t, underlying, "metric.current", 2, 2, "instance", "b")
				wrapper, err := semconv.AwareStorageWithRegistry(underlying, contractFanOutRegistry(1))
				require.NoError(t, err)
				hints := &storage.SelectHints{
					Start:             1,
					End:               2,
					Limit:             1,
					Step:              3,
					Func:              funcCase.function,
					Grouping:          []string{"instance"},
					By:                true,
					Range:             4,
					ShardCount:        1,
					ShardIndex:        0,
					DisableTrimming:   true,
					ProjectionLabels:  []string{"instance"},
					ProjectionInclude: true,
				}
				originalHints := copyRecordedHints(hints)
				wantHints := copyRecordedHints(hints)
				wantHints.Func = funcCase.wantFunc
				wantHints.Grouping = nil
				wantHints.By = false
				wantHints.ProjectionLabels = nil
				wantHints.ProjectionInclude = false

				if query == "series" {
					querier, err := wrapper.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, querier.Close()) })
					set := querier.Select(t.Context(), false, hints, contractFanOutMatchers(1)...)
					require.True(t, set.Next())
					require.True(t, set.At().Labels().Has(model.MetricNameLabel))
					require.False(t, set.At().Labels().Has("__series_hash__"))
					require.False(t, set.Next())
					require.NoError(t, set.Err())
					seriesHints, _ := underlying.recordedHints()
					require.Equal(t, []*storage.SelectHints{wantHints}, seriesHints)
					seriesSort, _ := underlying.recordedSort()
					require.Equal(t, []bool{false}, seriesSort)
					require.Equal(t, originalHints, hints)
					return
				}

				querier, err := wrapper.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, querier.Close()) })
				set := querier.Select(t.Context(), false, hints, contractFanOutMatchers(1)...)
				require.True(t, set.Next())
				require.True(t, set.At().Labels().Has(model.MetricNameLabel))
				require.False(t, set.At().Labels().Has("__series_hash__"))
				require.False(t, set.Next())
				require.NoError(t, set.Err())
				_, chunkHints := underlying.recordedHints()
				require.Equal(t, []*storage.SelectHints{wantHints}, chunkHints)
				_, chunkSort := underlying.recordedSort()
				require.Equal(t, []bool{false}, chunkSort)
				require.Equal(t, originalHints, hints)
			})
		}
	}
}

func BenchmarkSchemaAwareMetricOnlySelect10000(b *testing.B) {
	for _, tc := range []struct {
		name     string
		variants int
		metric   string
	}{
		{name: "identity", variants: 1, metric: "metric.current"},
		{name: "rewrite", variants: 2, metric: "metric.old.00"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			underlying := teststorage.New(b)
			appendContractSeriesForMetric(b, underlying, tc.metric, 10000)
			wrapper, err := semconv.AwareStorageWithRegistry(underlying, contractFanOutRegistry(tc.variants))
			require.NoError(b, err)
			matchers := contractFanOutMatchers(tc.variants)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				q, err := wrapper.Querier(0, 10)
				if err != nil {
					b.Fatal(err)
				}
				set := q.Select(b.Context(), false, nil, matchers...)
				count := 0
				for set.Next() {
					count++
				}
				if err := set.Err(); err != nil {
					b.Fatal(err)
				}
				if err := q.Close(); err != nil {
					b.Fatal(err)
				}
				if count != 10000 {
					b.Fatalf("got %d series, want 10000", count)
				}
			}
		})
	}
}
