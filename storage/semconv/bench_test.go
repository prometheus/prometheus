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
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

// benchSemconv declares the metric under test with one attribute, plus bench.solo,
// which no schema below renames. A query for bench.solo therefore fans out to a
// single variant with nothing rewritten but __name__, which is the common shape.
const benchSemconv = `
groups:
  - id: registry.attrs
    type: attribute_group
    brief: "Attributes"
    attributes:
      - id: %[2]s
        type: string
        stability: stable
        brief: "An attribute"
        examples: ["a"]
  - id: metric.%[1]s
    type: metric
    brief: "A metric"
    stability: stable
    metric_name: %[1]s
    instrument: counter
    unit: By
    attributes:
      - ref: %[2]s
  - id: metric.bench.solo
    type: metric
    brief: "A metric no schema renames"
    stability: stable
    metric_name: bench.solo
    instrument: counter
    unit: By
    attributes:
      - ref: attr.solo
`

// benchMetricRenameSchema renames only the metric, so nothing a returned series
// carries is rewritten apart from __name__.
const benchMetricRenameSchema = `
file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            bench.old: bench.new
`

// benchAttributeRenameSchema renames the metric and one of its attributes, which
// is what forces a variant to be buffered and re-sorted.
const benchAttributeRenameSchema = `
file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
            bench.old: bench.new
        - rename_attributes:
            attribute_map:
              attr.old: attr.new
            apply_to_metrics:
              - bench.new
`

// benchAttrOf gives each benchmark metric the attribute name its era declares.
var benchAttrOf = map[string]string{
	"bench.old":  "attr.old",
	"bench.new":  "attr.new",
	"bench.solo": "attr.solo",
}

// benchStorage builds a storage holding seriesCount series, spread evenly over the
// given metric names.
func benchStorage(b *testing.B, schema string, seriesCount int, metrics ...string) storage.Storage {
	b.Helper()
	wrapped, err := semconv.AwareStorageWithRegistry(teststorage.New(b), map[string][]byte{
		"registry.yaml": []byte(schema),
		"1.0.0":         fmt.Appendf(nil, benchSemconv, "bench.old", "attr.old"),
		"1.1.0":         fmt.Appendf(nil, benchSemconv, "bench.new", "attr.new"),
	})
	require.NoError(b, err)

	app := wrapped.Appender(context.Background())
	for i := range seriesCount {
		metric := metrics[i%len(metrics)]
		attr := benchAttrOf[metric]
		// Zero-padded so the stored order is stable and the series are spread
		// across both eras rather than clustered.
		lset := labels.FromStrings(model.MetricNameLabel, metric, attr, fmt.Sprintf("v%05d", i))
		_, err := app.Append(0, lset, 1, float64(i))
		require.NoError(b, err)
	}
	require.NoError(b, app.Commit())
	return wrapped
}

// BenchmarkSelectFanOut measures a fanned-out Select, drained the way PromQL
// drains one. Metric-only variants stream without querying label metadata;
// attribute renames still require buffering and sorting.
func BenchmarkSelectFanOut(b *testing.B) {
	for _, sc := range []struct {
		name    string
		schema  string
		query   string
		metrics []string
	}{
		// One variant, with only the canonical __name__ rewrite.
		{"no rename", benchMetricRenameSchema, "bench.solo", []string{"bench.solo"}},
		// Two variants, still with only __name__ rewritten.
		{"metric rename only", benchMetricRenameSchema, "bench.new", []string{"bench.old", "bench.new"}},
		// Two variants with an attribute rewritten as well.
		{"attribute rename", benchAttributeRenameSchema, "bench.new", []string{"bench.old", "bench.new"}},
	} {
		for _, seriesCount := range []int{100, 10000} {
			b.Run(fmt.Sprintf("%s/%d series", sc.name, seriesCount), func(b *testing.B) {
				s := benchStorage(b, sc.schema, seriesCount, sc.metrics...)
				matchers := []*labels.Matcher{
					labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, sc.query),
					labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
					labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
				}

				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					q, err := s.Querier(0, 10)
					require.NoError(b, err)

					var series, samples int
					set := q.Select(context.Background(), false, nil, matchers...)
					for set.Next() {
						series++
						it := set.At().Iterator(nil)
						for it.Next() == chunkenc.ValFloat {
							samples++
						}
					}
					require.NoError(b, set.Err())
					require.NoError(b, q.Close())
					if series != seriesCount {
						b.Fatalf("got %d series, want %d", series, seriesCount)
					}
				}
			})
		}
	}
}

const benchLatencyVariants = 32

type latencyStorage struct {
	storage.Storage
	delay time.Duration
}

func (s latencyStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &latencyQuerier{Querier: q, delay: s.delay}, nil
}

func (s latencyStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &latencyChunkQuerier{ChunkQuerier: q, delay: s.delay}, nil
}

type latencyQuerier struct {
	storage.Querier
	delay time.Duration
}

func (q *latencyQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	time.Sleep(q.delay)
	return storage.NoopSeriesSet()
}

func (q *latencyQuerier) LabelNames(context.Context, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	time.Sleep(q.delay)
	return nil, nil, nil
}

func (q *latencyQuerier) LabelValues(context.Context, string, *storage.LabelHints, ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	time.Sleep(q.delay)
	return nil, nil, nil
}

type latencyChunkQuerier struct {
	storage.ChunkQuerier
	delay time.Duration
}

func (q *latencyChunkQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.ChunkSeriesSet {
	time.Sleep(q.delay)
	return storage.NoopChunkedSeriesSet()
}

func benchLatencyRegistry() map[string][]byte {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    metrics:
      changes:
        - rename_metrics:
`)
	for i := range benchLatencyVariants - 1 {
		schema = fmt.Appendf(schema, "            bench.old.%02d: bench.current\n", i)
	}
	return map[string][]byte{
		"registry.yaml": schema,
		"1.1.0":         fmt.Appendf(nil, benchSemconv, "bench.current", "attr.current"),
	}
}

func benchLatencyStorage(b *testing.B) storage.Storage {
	b.Helper()
	wrapped, err := semconv.AwareStorageWithRegistry(
		latencyStorage{Storage: teststorage.New(b), delay: time.Millisecond},
		benchLatencyRegistry(),
	)
	require.NoError(b, err)
	return wrapped
}

func benchLatencyMatchers() []*labels.Matcher {
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "bench.current"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
}

// BenchmarkFanOutCallLatency measures the storage-call portion of schema
// fan-out independently of series decoding and iteration.
func BenchmarkFanOutCallLatency(b *testing.B) {
	for _, operation := range []string{"select", "chunk select", "label names", "label values"} {
		b.Run(operation, func(b *testing.B) {
			s := benchLatencyStorage(b)
			matchers := benchLatencyMatchers()
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				switch operation {
				case "select":
					q, err := s.Querier(0, 10)
					require.NoError(b, err)
					set := q.Select(context.Background(), false, nil, matchers...)
					for set.Next() {
					}
					require.NoError(b, set.Err())
					require.NoError(b, q.Close())
				case "chunk select":
					q, err := s.ChunkQuerier(0, 10)
					require.NoError(b, err)
					set := q.Select(context.Background(), false, nil, matchers...)
					for set.Next() {
					}
					require.NoError(b, set.Err())
					require.NoError(b, q.Close())
				case "label names":
					q, err := s.Querier(0, 10)
					require.NoError(b, err)
					_, _, err = q.LabelNames(context.Background(), nil, matchers...)
					require.NoError(b, err)
					require.NoError(b, q.Close())
				case "label values":
					q, err := s.Querier(0, 10)
					require.NoError(b, err)
					_, _, err = q.LabelValues(context.Background(), "attr.current", nil, matchers...)
					require.NoError(b, err)
					require.NoError(b, q.Close())
				}
			}
		})
	}
}
