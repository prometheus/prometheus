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
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/semconv"
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

// warningStrings flattens annotations into their string forms for assertion.
func warningStrings(a map[string]error) []string {
	out := make([]string, 0, len(a))
	for k := range a {
		out = append(out, k)
	}
	return out
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
