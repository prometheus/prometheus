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
	"sync"
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

// TestFanOutResultIsSorted checks that a variant whose labels were rewritten is
// re-sorted before it reaches the merge.
//
// Each variant is queried sorted by the names stored for its own era, and rewriting
// an attribute to its anchor-version name reorders it: here "ttt" sorts after "user"
// but before "tenant". storage.NewMergeSeriesSet assumes every input reports
// strictly increasing labels, so feeding it the rewritten set unsorted let it emit
// series out of order and, where two eras rewrote to the same labels, twice — a
// duplicate labelset PromQL rejects.
//
// The samples are asserted, not just the labels. A merged series computes its labels
// eagerly and its samples only when iterated, so a chain built over a slice the
// merge function still holds looks perfectly well-formed until something reads it.
func TestFanOutResultIsSorted(t *testing.T) {
	// Stored under the 1.0.0 name, so all of these come back in one variant.
	// "user" is rewritten to "tenant" at the 1.1.0 anchor, which moves it before
	// "ttt"; the two "1" series collide on the rewritten labels.
	appendAll := func(t *testing.T, s storage.Storage) {
		t.Helper()
		appendSeries(t, s, "test.counter", 1, 1.0, "ttt", "2")
		appendSeries(t, s, "test.counter", 1, 2.0, "user", "1")
		appendSeries(t, s, "test.counter", 2, 3.0, "tenant", "1")
	}
	// The two colliding series chain into one, so it carries both their samples.
	want := []seriesWithSamples{
		{
			lset:    labels.FromStrings(model.MetricNameLabel, "test", "tenant", "1"),
			samples: []sample{{t: 1, v: 2.0}, {t: 2, v: 3.0}},
		},
		{
			lset:    labels.FromStrings(model.MetricNameLabel, "test", "ttt", "2"),
			samples: []sample{{t: 1, v: 1.0}},
		},
	}

	t.Run("querier", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendAll(t, wrapped)

		set := selectAt(t, wrapped, "1.1.0", "test")

		got := collectWithSamples(t, set)
		require.Equal(t, want, got, "expected strictly increasing labels with no duplicates")
	})

	t.Run("chunk querier", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendAll(t, wrapped)

		cq, err := wrapped.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = cq.Close() })

		set := cq.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)

		got := collectWithSamples(t, storage.NewSeriesSetFromChunkSeriesSet(set))
		require.Equal(t, want, got, "expected strictly increasing labels with no duplicates")
	})
}

func TestFanOutCanonicalLabelsRemainValid(t *testing.T) {
	for _, tc := range []struct {
		name    string
		labels  []string
		want    labels.Labels
		wantErr string
	}{
		{
			name:   "renamed label is reordered",
			labels: []string{"service.name", "api", "thread.daemon", "true"},
			want: labels.FromStrings(
				model.MetricNameLabel, "jvm.thread.count",
				"jvm.thread.daemon", "true",
				"service.name", "api",
			),
		},
		{
			name:   "equal aliases collapse",
			labels: []string{"jvm.thread.daemon", "true", "thread.daemon", "true"},
			want: labels.FromStrings(
				model.MetricNameLabel, "jvm.thread.count",
				"jvm.thread.daemon", "true",
			),
		},
		{
			name:    "conflicting aliases fail",
			labels:  []string{"jvm.thread.daemon", "false", "thread.daemon", "true"},
			wantErr: `maps "jvm.thread.daemon" and "thread.daemon" to "jvm.thread.daemon" with conflicting values`,
		},
	} {
		for _, query := range []string{"series", "chunks"} {
			t.Run(tc.name+"/"+query, func(t *testing.T) {
				wrapped := newCanonicalLabelStorage(t, tc.labels...)
				if query == "series" {
					q, err := wrapped.Querier(0, 10)
					require.NoError(t, err)
					t.Cleanup(func() { _ = q.Close() })

					set := q.Select(context.Background(), false, nil, canonicalLabelMatchers()...)
					if tc.wantErr != "" {
						require.False(t, set.Next())
						require.ErrorContains(t, set.Err(), tc.wantErr)
						return
					}
					require.True(t, set.Next())
					require.Equal(t, tc.want, set.At().Labels())
					require.Equal(t, tc.want.Get("jvm.thread.daemon"), set.At().Labels().Get("jvm.thread.daemon"))
					require.False(t, set.Next())
					require.NoError(t, set.Err())
					return
				}

				q, err := wrapped.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })

				set := q.Select(context.Background(), false, nil, canonicalLabelMatchers()...)
				if tc.wantErr != "" {
					require.False(t, set.Next())
					require.ErrorContains(t, set.Err(), tc.wantErr)
					return
				}
				require.True(t, set.Next())
				require.Equal(t, tc.want, set.At().Labels())
				require.Equal(t, tc.want.Get("jvm.thread.daemon"), set.At().Labels().Get("jvm.thread.daemon"))
				require.False(t, set.Next())
				require.NoError(t, set.Err())
			})
		}
	}
}

func newCanonicalLabelStorage(t *testing.T, kv ...string) storage.Storage {
	t.Helper()
	wrapped, err := semconv.AwareStorageWithRegistry(teststorage.New(t), canonicalLabelRegistry())
	require.NoError(t, err)
	appendSeries(t, wrapped, "jvm.thread.count", 1, 1, kv...)
	return wrapped
}

func canonicalLabelRegistry() map[string][]byte {
	return map[string][]byte{
		"registry.yaml": []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    all:
      changes:
        - rename_attributes:
            attribute_map:
              thread.daemon: jvm.thread.daemon
`),
		"1.1.0": []byte(`groups:
  - id: metric.jvm.thread.count
    type: metric
    metric_name: jvm.thread.count
    unit: "{thread}"
    instrument: updowncounter
    attributes:
      - ref: jvm.thread.daemon
`),
	}
}

func canonicalLabelMatchers() []*labels.Matcher {
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "jvm.thread.count"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
}

// TestFanOutWithoutAttributeRenameIsSorted guards the assumption that lets a
// variant stream instead of being buffered and sorted: when the schema renames only
// the metric, every series in a variant matches the same equality matcher on
// __name__, so they all get the same replacement and the order the underlying
// querier returned them in survives. Output must be sorted with no variant sorted
// after the fact.
func TestFanOutWithoutAttributeRenameIsSorted(t *testing.T) {
	wrapped, err := semconv.AwareStorageWithRegistry(teststorage.New(t), map[string][]byte{
		"registry.yaml": []byte(benchMetricRenameSchema),
		"1.0.0":         fmt.Appendf(nil, benchSemconv, "bench.old", "attr.old"),
		"1.1.0":         fmt.Appendf(nil, benchSemconv, "bench.new", "attr.new"),
	})
	require.NoError(t, err)

	// attr.old and attr.new are distinct labels that the schema does not rename, so
	// nothing rewrites them and the eras interleave in the merged order.
	appendSeries(t, wrapped, "bench.old", 1, 1.0, "attr.old", "zzz")
	appendSeries(t, wrapped, "bench.old", 1, 2.0, "attr.old", "aaa")
	appendSeries(t, wrapped, "bench.new", 1, 3.0, "attr.new", "mmm")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "bench.new"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	)

	got := collectWithSamples(t, set)
	require.Equal(t, []seriesWithSamples{
		{
			lset:    labels.FromStrings(model.MetricNameLabel, "bench.new", "attr.new", "mmm"),
			samples: []sample{{t: 1, v: 3.0}},
		},
		{
			lset:    labels.FromStrings(model.MetricNameLabel, "bench.new", "attr.old", "aaa"),
			samples: []sample{{t: 1, v: 2.0}},
		},
		{
			lset:    labels.FromStrings(model.MetricNameLabel, "bench.new", "attr.old", "zzz"),
			samples: []sample{{t: 1, v: 1.0}},
		},
	}, got)
}

type sample struct {
	t int64
	v float64
}

type seriesWithSamples struct {
	lset    labels.Labels
	samples []sample
}

// collectWithSamples drains a series set in order, reading each series' samples.
// Unlike collectSeries it keeps the order and every sample, which is what a set
// fed to a merge has to be checked on: the merge computes a series' labels
// eagerly and its samples lazily, so a malformed chain shows only when read.
func collectWithSamples(t *testing.T, set storage.SeriesSet) []seriesWithSamples {
	t.Helper()
	var out []seriesWithSamples
	for set.Next() {
		s := set.At()
		got := seriesWithSamples{lset: s.Labels()}
		it := s.Iterator(nil)
		for it.Next() == chunkenc.ValFloat {
			ts, v := it.At()
			got.samples = append(got.samples, sample{t: ts, v: v})
			require.LessOrEqual(t, len(got.samples), 16, "runaway iteration on %s", got.lset)
		}
		require.NoError(t, it.Err())
		out = append(out, got)
	}
	require.NoError(t, set.Err())
	return out
}

// ctxCheckingStorage returns series sets that fail if the context they were
// selected with has been cancelled by the time they are read, which is what an
// underlying querier streaming from a remote does implicitly.
type ctxCheckingStorage struct {
	storage.Storage
}

func (s ctxCheckingStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return ctxCheckingQuerier{Querier: q}, nil
}

func (s ctxCheckingStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return ctxCheckingChunkQuerier{ChunkQuerier: q}, nil
}

type ctxCheckingQuerier struct {
	storage.Querier
}

func (q ctxCheckingQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	return &ctxCheckingSeriesSet{
		SeriesSet: q.Querier.Select(ctx, sortSeries, hints, matchers...),
		ctx:       ctx,
	}
}

type ctxCheckingSeriesSet struct {
	storage.SeriesSet

	ctx context.Context
	err error
}

func (s *ctxCheckingSeriesSet) Next() bool {
	if err := s.ctx.Err(); err != nil {
		s.err = err
		return false
	}
	return s.SeriesSet.Next()
}

func (s *ctxCheckingSeriesSet) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.SeriesSet.Err()
}

type ctxCheckingChunkQuerier struct {
	storage.ChunkQuerier
}

func (q ctxCheckingChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	return &ctxCheckingChunkSeriesSet{
		ChunkSeriesSet: q.ChunkQuerier.Select(ctx, sortSeries, hints, matchers...),
		ctx:            ctx,
	}
}

type ctxCheckingChunkSeriesSet struct {
	storage.ChunkSeriesSet

	ctx context.Context
	err error
}

func (s *ctxCheckingChunkSeriesSet) Next() bool {
	if err := s.ctx.Err(); err != nil {
		s.err = err
		return false
	}
	return s.ChunkSeriesSet.Next()
}

func (s *ctxCheckingChunkSeriesSet) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.ChunkSeriesSet.Err()
}

// TestFanOutKeepsSelectContextAlive checks that the context handed to the
// underlying Select outlives fan-out scheduling.
//
// A Select is lazy, so its context has to stay valid until the result is read.
// A derived context canceled when workers finish would expire before anything
// reads a variant and abort a streaming querier mid-read. The in-memory storage
// rarely notices, so this asserts it through a querier that checks every Next.
func TestFanOutKeepsSelectContextAlive(t *testing.T) {
	wrapped, err := semconv.AwareStorageWithRegistry(
		ctxCheckingStorage{Storage: teststorage.New(t)},
		map[string][]byte{
			"registry.yaml": []byte(benchMetricRenameSchema),
			"1.0.0":         fmt.Appendf(nil, benchSemconv, "bench.old", "attr.old"),
			"1.1.0":         fmt.Appendf(nil, benchSemconv, "bench.new", "attr.new"),
		},
	)
	require.NoError(t, err)

	appendSeries(t, wrapped, "bench.old", 1, 1.0, "attr.old", "a")
	appendSeries(t, wrapped, "bench.new", 1, 2.0, "attr.new", "b")

	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "bench.new"),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}

	t.Run("series", func(t *testing.T) {
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil, matchers...)
		var got int
		for set.Next() {
			got++
		}
		require.NoError(t, set.Err(), "the variants were read with a cancelled context")
		require.Equal(t, 2, got, "both eras must be returned")
	})

	t.Run("chunks", func(t *testing.T) {
		q, err := wrapped.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil, matchers...)
		var got int
		for set.Next() {
			got++
		}
		require.NoError(t, set.Err(), "the variants were read with a cancelled context")
		require.Equal(t, 2, got, "both eras must be returned")
	})
}

const fanOutProbeVariants = 17

type fanOutSerialProbe struct {
	mu sync.Mutex

	totalCalls          int
	activeCalls         int
	maxActiveCalls      int
	activeByQuerier     map[int]int
	querierOverlap      bool
	matcherMutation     bool
	hintMutation        bool
	undrainedByQuerier  map[int]int
	selectedBeforeDrain bool
	replacement         *labels.Matcher
	nextQuerierID       int
	openedQueriers      int
	closedQueriers      map[int]int
	cancelOnFirstCall   context.CancelFunc
}

func newFanOutSerialProbe() *fanOutSerialProbe {
	return &fanOutSerialProbe{
		activeByQuerier:    map[int]int{},
		undrainedByQuerier: map[int]int{},
		replacement:        labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "mutated.by.storage"),
		closedQueriers:     map[int]int{},
	}
}

func (p *fanOutSerialProbe) openQuerier() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.nextQuerierID
	p.nextQuerierID++
	p.openedQueriers++
	return id
}

func (p *fanOutSerialProbe) closeQuerier(id int) {
	p.mu.Lock()
	p.closedQueriers[id]++
	p.mu.Unlock()
}

func (p *fanOutSerialProbe) mutateSelectHints(hints *storage.SelectHints) {
	if hints == nil {
		return
	}
	p.mu.Lock()
	if hints.Limit == -1 || len(hints.Grouping) > 0 && hints.Grouping[0] == "mutated.by.storage" {
		p.hintMutation = true
	}
	p.mu.Unlock()
	hints.Limit = -1
	if len(hints.Grouping) > 0 {
		hints.Grouping[0] = "mutated.by.storage"
	}
}

func (p *fanOutSerialProbe) mutateLabelHints(hints *storage.LabelHints) {
	if hints == nil {
		return
	}
	p.mu.Lock()
	if hints.Limit == -1 {
		p.hintMutation = true
	}
	p.mu.Unlock()
	hints.Limit = -1
}

func (p *fanOutSerialProbe) call(querierID int, matchers []*labels.Matcher, returnsSet bool) bool {
	p.mu.Lock()
	p.activeCalls++
	p.maxActiveCalls = max(p.maxActiveCalls, p.activeCalls)
	p.activeByQuerier[querierID]++
	if p.activeByQuerier[querierID] > 1 {
		p.querierOverlap = true
	}
	requiresDrain := returnsSet
	for _, matcher := range matchers {
		if matcher == p.replacement {
			p.matcherMutation = true
		}
		if matcher.Name == model.MetricNameLabel && matcher.Value == "metric.current" {
			requiresDrain = false
		}
	}
	p.totalCalls++
	var cancel context.CancelFunc
	if p.totalCalls == 1 {
		cancel = p.cancelOnFirstCall
	}
	if requiresDrain {
		if p.undrainedByQuerier[querierID] > 0 {
			p.selectedBeforeDrain = true
		}
		p.undrainedByQuerier[querierID]++
	}
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	if len(matchers) > 0 {
		// Storage adapters may mutate the variadic matcher slice.
		matchers[0] = p.replacement
	}
	time.Sleep(time.Millisecond)
	p.mu.Lock()
	p.activeCalls--
	p.activeByQuerier[querierID]--
	p.mu.Unlock()
	return requiresDrain
}

func (p *fanOutSerialProbe) drainedResortSet(querierID int) {
	p.mu.Lock()
	p.undrainedByQuerier[querierID]--
	p.mu.Unlock()
}

func (p *fanOutSerialProbe) snapshot() (total, maxActive int, querierOverlap, matcherMutation, selectedBeforeDrain bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.totalCalls, p.maxActiveCalls, p.querierOverlap, p.matcherMutation, p.selectedBeforeDrain
}

func (p *fanOutSerialProbe) closeSnapshot() (opened, closed int, duplicateClose bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, count := range p.closedQueriers {
		closed += count
		if count > 1 {
			duplicateClose = true
		}
	}
	return p.openedQueriers, closed, duplicateClose
}

func (p *fanOutSerialProbe) hintsWereShared() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hintMutation
}

type fanOutProbeStorage struct {
	storage.Storage
	probe *fanOutSerialProbe
}

func (s fanOutProbeStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &fanOutProbeQuerier{Querier: q, probe: s.probe, id: s.probe.openQuerier()}, nil
}

func (s fanOutProbeStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &fanOutProbeChunkQuerier{ChunkQuerier: q, probe: s.probe, id: s.probe.openQuerier()}, nil
}

type fanOutProbeQuerier struct {
	storage.Querier
	probe *fanOutSerialProbe
	id    int
}

func (q *fanOutProbeQuerier) Select(_ context.Context, _ bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	q.probe.mutateSelectHints(hints)
	requiresDrain := q.probe.call(q.id, matchers, true)
	return &fanOutProbeSeriesSet{SeriesSet: storage.NoopSeriesSet(), probe: q.probe, querierID: q.id, requiresDrain: requiresDrain}
}

func (q *fanOutProbeQuerier) LabelNames(_ context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.probe.mutateLabelHints(hints)
	q.probe.call(q.id, matchers, false)
	return nil, nil, nil
}

func (q *fanOutProbeQuerier) LabelValues(_ context.Context, _ string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.probe.mutateLabelHints(hints)
	q.probe.call(q.id, matchers, false)
	return nil, nil, nil
}

func (q *fanOutProbeQuerier) Close() error {
	q.probe.closeQuerier(q.id)
	return q.Querier.Close()
}

type fanOutProbeChunkQuerier struct {
	storage.ChunkQuerier
	probe *fanOutSerialProbe
	id    int
}

func (q *fanOutProbeChunkQuerier) Select(_ context.Context, _ bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	q.probe.mutateSelectHints(hints)
	requiresDrain := q.probe.call(q.id, matchers, true)
	return &fanOutProbeChunkSeriesSet{ChunkSeriesSet: storage.NoopChunkedSeriesSet(), probe: q.probe, querierID: q.id, requiresDrain: requiresDrain}
}

func (q *fanOutProbeChunkQuerier) LabelNames(_ context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.probe.mutateLabelHints(hints)
	q.probe.call(q.id, matchers, false)
	return nil, nil, nil
}

func (q *fanOutProbeChunkQuerier) LabelValues(_ context.Context, _ string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	q.probe.mutateLabelHints(hints)
	q.probe.call(q.id, matchers, false)
	return nil, nil, nil
}

func (q *fanOutProbeChunkQuerier) Close() error {
	q.probe.closeQuerier(q.id)
	return q.ChunkQuerier.Close()
}

type fanOutProbeSeriesSet struct {
	storage.SeriesSet
	probe         *fanOutSerialProbe
	querierID     int
	requiresDrain bool
}

func (s *fanOutProbeSeriesSet) Next() bool {
	if s.requiresDrain {
		s.probe.drainedResortSet(s.querierID)
		s.requiresDrain = false
	}
	return false
}

type fanOutProbeChunkSeriesSet struct {
	storage.ChunkSeriesSet
	probe         *fanOutSerialProbe
	querierID     int
	requiresDrain bool
}

func (s *fanOutProbeChunkSeriesSet) Next() bool {
	if s.requiresDrain {
		s.probe.drainedResortSet(s.querierID)
		s.requiresDrain = false
	}
	return false
}

func fanOutProbeRegistry() map[string][]byte {
	return fanOutRegistry(fanOutProbeVariants)
}

func fanOutRegistry(variants int) map[string][]byte {
	schema := []byte(`file_format: 1.1.0
schema_url: https://example.com/schemas/1.1.0
versions:
  1.0.0:
  1.1.0:
    all:
      changes:
        - rename_attributes:
            attribute_map:
              http.status_old: http.response.status_code
    metrics:
      changes:
        - rename_metrics:
`)
	for i := range variants - 1 {
		schema = fmt.Appendf(schema, "            metric.old.%02d: metric.current\n", i)
	}
	return map[string][]byte{
		"registry.yaml": schema,
		"1.1.0":         metricSemconv("metric.current", "s", "histogram"),
	}
}

func newFanOutProbeStorage(t *testing.T, probe *fanOutSerialProbe) storage.Storage {
	return newFanOutProbeStorageWithRegistry(t, probe, fanOutProbeRegistry())
}

func newFanOutProbeStorageWithRegistry(t *testing.T, probe *fanOutSerialProbe, registry map[string][]byte) storage.Storage {
	t.Helper()
	wrapped, err := semconv.AwareStorageWithRegistry(
		fanOutProbeStorage{Storage: teststorage.New(t), probe: probe},
		registry,
	)
	require.NoError(t, err)
	return wrapped
}

func fanOutProbeMatchers() []*labels.Matcher {
	return fanOutProbeMatchersFor("metric.current")
}

func requireSerialFanOut(t *testing.T, probe *fanOutSerialProbe, wantCalls int, checkScheduling bool) {
	t.Helper()
	total, maxActive, querierOverlap, matcherMutation, selectedBeforeDrain := probe.snapshot()
	require.Equal(t, wantCalls, total)
	require.Equal(t, 1, maxActive, "storage calls must be serial")
	require.False(t, querierOverlap, "calls on one underlying querier must not overlap")
	require.False(t, matcherMutation, "storage mutation must not escape one matcher slice")
	require.False(t, probe.hintsWereShared(), "storage mutation must not escape one hints value")
	opened, _, _ := probe.closeSnapshot()
	require.Equal(t, 1, opened, "fan-out must retain the outer query's storage snapshot")
	if checkScheduling {
		require.True(t, selectedBeforeDrain, "variant selects must be scheduled before iteration starts")
	}
}

func TestFanOutUsesOneQuerierAndIsolatesInputs(t *testing.T) {
	t.Run("series", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		hints := &storage.SelectHints{Limit: 1, Grouping: []string{"job"}}
		set := q.Select(context.Background(), false, hints, fanOutProbeMatchers()...)
		require.False(t, set.Next())
		require.NoError(t, set.Err())
		require.Equal(t, &storage.SelectHints{Limit: 1, Grouping: []string{"job"}}, hints)
		requireSerialFanOut(t, probe, fanOutProbeVariants, true)
	})

	t.Run("chunks", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		hints := &storage.SelectHints{Limit: 1, Grouping: []string{"job"}}
		set := q.Select(context.Background(), false, hints, fanOutProbeMatchers()...)
		require.False(t, set.Next())
		require.NoError(t, set.Err())
		require.Equal(t, &storage.SelectHints{Limit: 1, Grouping: []string{"job"}}, hints)
		requireSerialFanOut(t, probe, fanOutProbeVariants, true)
	})

	t.Run("label names", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		hints := &storage.LabelHints{Limit: 1}
		_, _, err = q.LabelNames(context.Background(), hints, fanOutProbeMatchers()...)
		require.NoError(t, err)
		require.Equal(t, &storage.LabelHints{Limit: 1}, hints)
		requireSerialFanOut(t, probe, fanOutProbeVariants, false)
	})

	t.Run("label values", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		hints := &storage.LabelHints{Limit: 1}
		_, _, err = q.LabelValues(context.Background(), model.MetricNameLabel, hints, fanOutProbeMatchers()...)
		require.NoError(t, err)
		require.Equal(t, &storage.LabelHints{Limit: 1}, hints)
		requireSerialFanOut(t, probe, fanOutProbeVariants, false)
	})
}

func TestStorageFanOutLimitFailsClosed(t *testing.T) {
	assertNoCalls := func(t *testing.T, probe *fanOutSerialProbe) {
		t.Helper()
		total, maxActive, querierOverlap, matcherMutation, _ := probe.snapshot()
		require.Zero(t, total)
		require.Zero(t, maxActive)
		require.False(t, querierOverlap)
		require.False(t, matcherMutation)
	}

	t.Run("allows 32 jobs", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorageWithRegistry(t, probe, fanOutRegistry(32))
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil, fanOutProbeMatchers()...)
		require.False(t, set.Next())
		require.NoError(t, set.Err())
		require.Empty(t, warningStrings(set.Warnings()))
		requireSerialFanOut(t, probe, 32, true)
	})

	t.Run("series", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorageWithRegistry(t, probe, fanOutRegistry(33))
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil, fanOutProbeMatchers()...)
		require.False(t, set.Next())
		require.ErrorContains(t, set.Err(), "schema expansion limit exceeded")
		require.Empty(t, warningStrings(set.Warnings()))
		assertNoCalls(t, probe)
	})

	t.Run("chunks", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorageWithRegistry(t, probe, fanOutRegistry(33))
		q, err := wrapped.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil, fanOutProbeMatchers()...)
		require.False(t, set.Next())
		require.ErrorContains(t, set.Err(), "schema expansion limit exceeded")
		require.Empty(t, warningStrings(set.Warnings()))
		assertNoCalls(t, probe)
	})

	t.Run("label names", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorageWithRegistry(t, probe, fanOutRegistry(33))
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		_, anns, err := q.LabelNames(context.Background(), nil, fanOutProbeMatchers()...)
		require.ErrorContains(t, err, "schema expansion limit exceeded")
		require.Empty(t, warningStrings(anns))
		assertNoCalls(t, probe)
	})

	t.Run("label values", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		_, anns, err := q.LabelValues(context.Background(), "http.response.status_code", nil, fanOutProbeMatchers()...)
		require.ErrorContains(t, err, "schema expansion limit exceeded")
		require.Empty(t, warningStrings(anns))
		assertNoCalls(t, probe)
	})
}

func TestFanOutOwnsOneQuerier(t *testing.T) {
	for _, query := range []string{"series", "chunks"} {
		t.Run(query, func(t *testing.T) {
			probe := newFanOutSerialProbe()
			wrapped := newFanOutProbeStorage(t, probe)
			if query == "series" {
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				set := q.Select(context.Background(), false, nil, fanOutProbeMatchers()...)
				require.False(t, set.Next())
				require.NoError(t, set.Err())
				require.NoError(t, q.Close())
			} else {
				q, err := wrapped.ChunkQuerier(0, 10)
				require.NoError(t, err)
				set := q.Select(context.Background(), false, nil, fanOutProbeMatchers()...)
				require.False(t, set.Next())
				require.NoError(t, set.Err())
				require.NoError(t, q.Close())
			}

			opened, closed, duplicateClose := probe.closeSnapshot()
			require.Equal(t, 1, opened)
			require.Equal(t, 1, closed)
			require.False(t, duplicateClose)
		})
	}
}

func TestCanceledFanOutDoesNotStartStorageCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("series", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(ctx, false, nil, fanOutProbeMatchers()...)
		require.False(t, set.Next())
		require.ErrorIs(t, set.Err(), context.Canceled)
		total, _, _, _, _ := probe.snapshot()
		require.Equal(t, 0, total)
		opened, _, _ := probe.closeSnapshot()
		require.Equal(t, 1, opened)
	})

	t.Run("chunks", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(ctx, false, nil, fanOutProbeMatchers()...)
		require.False(t, set.Next())
		require.ErrorIs(t, set.Err(), context.Canceled)
		total, _, _, _, _ := probe.snapshot()
		require.Equal(t, 0, total)
		opened, _, _ := probe.closeSnapshot()
		require.Equal(t, 1, opened)
	})

	t.Run("labels", func(t *testing.T) {
		probe := newFanOutSerialProbe()
		wrapped := newFanOutProbeStorage(t, probe)
		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		_, _, err = q.LabelNames(ctx, nil, fanOutProbeMatchers()...)
		require.ErrorIs(t, err, context.Canceled)
		total, _, _, _, _ := probe.snapshot()
		require.Equal(t, 0, total)
		opened, _, _ := probe.closeSnapshot()
		require.Equal(t, 1, opened)
	})
}

func TestCanceledFanOutStopsBeforeNextStorageCall(t *testing.T) {
	for _, operation := range []string{"series", "chunks", "label names", "label values"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			probe := newFanOutSerialProbe()
			probe.cancelOnFirstCall = cancel
			wrapped := newFanOutProbeStorage(t, probe)

			var queryErr error
			switch operation {
			case "series":
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })
				set := q.Select(ctx, false, nil, fanOutProbeMatchers()...)
				for set.Next() {
				}
				queryErr = set.Err()
			case "chunks":
				q, err := wrapped.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })
				set := q.Select(ctx, false, nil, fanOutProbeMatchers()...)
				for set.Next() {
				}
				queryErr = set.Err()
			case "label names":
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })
				_, _, queryErr = q.LabelNames(ctx, nil, fanOutProbeMatchers()...)
			case "label values":
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })
				_, _, queryErr = q.LabelValues(ctx, model.MetricNameLabel, nil, fanOutProbeMatchers()...)
			}

			require.ErrorIs(t, queryErr, context.Canceled)
			total, maxActive, querierOverlap, _, _ := probe.snapshot()
			require.Equal(t, 1, total)
			require.Equal(t, 1, maxActive)
			require.False(t, querierOverlap)
			opened, _, _ := probe.closeSnapshot()
			require.Equal(t, 1, opened)
		})
	}
}

// A TSDB querier captures append isolation when it is created. Schema fan-out
// must not open later queriers that can observe commits made after that point.
func TestFanOutPreservesTSDBSnapshot(t *testing.T) {
	want := []seriesWithSamples{
		{
			lset:    labels.FromStrings(model.MetricNameLabel, "test", "identity", "current"),
			samples: []sample{{t: 1, v: 1}},
		},
		{
			lset:    labels.FromStrings(model.MetricNameLabel, "test", "identity", "historical"),
			samples: []sample{{t: 1, v: 2}},
		},
	}

	for _, query := range []string{"series", "chunks"} {
		t.Run(query, func(t *testing.T) {
			wrapped, _ := newAwareStorage(t)
			appendSeries(t, wrapped, "test", 1, 1, "identity", "current")
			appendSeries(t, wrapped, "test.counter", 1, 2, "identity", "historical")

			var got []seriesWithSamples
			if query == "series" {
				q, err := wrapped.Querier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })

				appendSeries(t, wrapped, "test", 2, 11, "identity", "current")
				appendSeries(t, wrapped, "test.counter", 2, 12, "identity", "historical")
				got = collectWithSamples(t, q.Select(context.Background(), false, nil, fanOutProbeMatchersFor("test")...))
			} else {
				q, err := wrapped.ChunkQuerier(0, 10)
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })

				appendSeries(t, wrapped, "test", 2, 11, "identity", "current")
				appendSeries(t, wrapped, "test.counter", 2, 12, "identity", "historical")
				set := q.Select(context.Background(), false, nil, fanOutProbeMatchersFor("test")...)
				got = collectWithSamples(t, storage.NewSeriesSetFromChunkSeriesSet(set))
			}

			require.Equal(t, want, got)
		})
	}
}

// TestFanOutTSDBRangeSelect exercises the shared head chunk reader used when
// range-query hints enable its mutable chunk cache. The underlying querier does
// not permit concurrent Select calls.
func TestFanOutTSDBRangeSelect(t *testing.T) {
	wrapped, _ := newAwareStorage(t)
	appendSeries(t, wrapped, "test.counter", 1, 1, "user", "a")
	appendSeries(t, wrapped, "test", 1, 2, "tenant", "b")

	q, err := wrapped.Querier(0, 10)
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	set := q.Select(context.Background(), false, &storage.SelectHints{Start: 0, End: 10, Step: 1},
		fanOutProbeMatchersFor("test")...,
	)
	require.Len(t, collectSeries(t, set), 2)
}

func fanOutProbeMatchersFor(metricName string) []*labels.Matcher {
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, metricName),
		labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
	}
}
