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
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

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

// collectSeries drains a SeriesSet into a map of label-string -> first sample value.
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

func warningStrings(a map[string]error) []string {
	out := make([]string, 0, len(a))
	for k := range a {
		out = append(out, k)
	}
	return out
}

func TestAwareStoragePassthroughAndWarnings(t *testing.T) {
	wrapped, underlying := newTransformTestStorage(t)
	appendSeries(t, underlying, "test", 1, 1.0, "env", "prod")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, q.Close()) })

	t.Run("plain selector without __schema_url__ passes through without warnings", func(t *testing.T) {
		set := q.Select(t.Context(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
		)
		got := collectSeries(t, set)
		require.Empty(t, set.Warnings())
		require.Equal(t, map[string]float64{`{__name__="test", env="prod"}`: 1.0}, got)
	})

	t.Run("invalid __schema_url__ falls back to passthrough with ErrSchemaWarning", func(t *testing.T) {
		set := q.Select(t.Context(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "http://example.com/x.yaml"),
		)
		got := collectSeries(t, set)
		require.Equal(t, map[string]float64{`{__name__="test", env="prod"}`: 1.0}, got)
		warnings := set.Warnings()
		require.NotEmpty(t, warnings)
		for _, err := range warnings {
			require.ErrorIs(t, err, semconv.ErrSchemaWarning)
			require.ErrorIs(t, err, annotations.PromQLWarning)
		}
	})

	t.Run("missing __name__ equality matcher with __schema_url__ fails query", func(t *testing.T) {
		set := q.Select(t.Context(), false, nil,
			labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, "http.*"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/http_1.20.0_to_1.23.1.yaml"),
		)
		require.False(t, set.Next())
		require.ErrorContains(t, set.Err(), "requires a non-empty equality matcher on __name__")
	})
}

func TestAwareStorageWithRegistryValidationAndDefensiveCopy(t *testing.T) {
	t.Run("rejects empty or invalid registry", func(t *testing.T) {
		underlying := teststorage.New(t)
		t.Cleanup(func() { require.NoError(t, underlying.Close()) })
		_, err := semconv.AwareStorageWithRegistry(underlying, nil)
		require.Error(t, err)
		_, err = semconv.AwareStorageWithRegistry(underlying, map[string][]byte{
			"bad.yaml": []byte("file_format: 9.9.9\n"),
		})
		require.Error(t, err)
	})

	t.Run("defensively copies caller registry map and byte slices", func(t *testing.T) {
		files := testTransformRegistry()
		underlying := teststorage.New(t)
		t.Cleanup(func() { require.NoError(t, underlying.Close()) })
		wrapped, err := semconv.AwareStorageWithRegistry(underlying, files)
		require.NoError(t, err)

		for name, b := range files {
			clear(b)
			files[name] = []byte("mutated after construction")
		}

		appendSeries(t, underlying, "http.server.duration", 1, 7.0,
			"http.method", "GET",
			"http.status_code", "200",
		)
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, q.Close()) })

		set := q.Select(t.Context(), true, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "http.server.request.duration"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/http_1.20.0_to_1.23.1.yaml"),
		)
		got := collectSeries(t, set)
		require.Equal(t, map[string]float64{
			`{__name__="http.server.request.duration", "http.request.method"="GET", "http.response.status_code"="200"}`: 7.0,
		}, got)
	})
}
