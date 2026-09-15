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

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

type infoCancelTrace struct {
	stage      string
	cancel     context.CancelFunc
	once       sync.Once
	mu         sync.Mutex
	canceled   time.Time
	contexts   []context.Context
	opened     atomic.Int64
	closed     atomic.Int64
	sets       atomic.Int64
	setsClosed atomic.Int64
	iterations atomic.Int64
	filters    atomic.Int64
	afterFlush atomic.Int64
}

func (t *infoCancelTrace) trigger() {
	t.once.Do(func() {
		t.mu.Lock()
		t.canceled = time.Now()
		t.mu.Unlock()
		t.cancel()
	})
}

func (t *infoCancelTrace) recordContext(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.contexts = append(t.contexts, ctx)
}

type infoCancelQueryable struct {
	storage.SampleAndChunkQueryable
	trace *infoCancelTrace
}

func (q infoCancelQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	querier, err := q.SampleAndChunkQueryable.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	q.trace.opened.Add(1)
	return &infoCancelQuerier{Querier: querier, trace: q.trace}, nil
}

type infoCancelQuerier struct {
	storage.Querier
	trace *infoCancelTrace
}

type infoCancelFilter func(string) (bool, float64)

func (f infoCancelFilter) Accept(value string) (bool, float64) {
	return f(value)
}

func (q *infoCancelQuerier) Select(ctx context.Context, sorted bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	q.trace.recordContext(ctx)
	return infoCancelSeriesSet{SeriesSet: q.Querier.Select(ctx, sorted, hints, matchers...), trace: q.trace}
}

type infoCancelSeriesSet struct {
	storage.SeriesSet
	trace *infoCancelTrace
}

func (s infoCancelSeriesSet) Next() bool {
	next := s.SeriesSet.Next()
	if next && s.trace.iterations.Add(1) == 5 && s.trace.stage == "expression" {
		s.trace.trigger()
	}
	return next
}

func (q *infoCancelQuerier) Close() error {
	q.trace.closed.Add(1)
	return q.Querier.Close()
}

func (q *infoCancelQuerier) searchHints(ctx context.Context, hints *storage.SearchHints) *storage.SearchHints {
	q.trace.recordContext(ctx)
	if q.trace.stage != "filter" {
		return hints
	}
	copyHints := *hints
	copyHints.Filter = infoCancelFilter(func(value string) (bool, float64) {
		// Cancel inside real Searcher construction, before a result iterator
		// exists. The original filter and its request-local memo stay intact.
		accepted, score := hints.Filter.Accept(value)
		if q.trace.filters.Add(1) == 5 {
			q.trace.trigger()
		}
		return accepted, score
	})
	return &copyHints
}

func (q *infoCancelQuerier) SearchLabelNames(ctx context.Context, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	rs := q.Querier.(storage.Searcher).SearchLabelNames(ctx, q.searchHints(ctx, hints), matchers...)
	q.trace.sets.Add(1)
	return infoCancelResultSet{SearchResultSet: rs, trace: q.trace}
}

func (q *infoCancelQuerier) SearchLabelValues(ctx context.Context, name string, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	rs := q.Querier.(storage.Searcher).SearchLabelValues(ctx, name, q.searchHints(ctx, hints), matchers...)
	q.trace.sets.Add(1)
	return infoCancelResultSet{SearchResultSet: rs, trace: q.trace}
}

type infoCancelResultSet struct {
	storage.SearchResultSet
	trace *infoCancelTrace
}

func (s infoCancelResultSet) Next() bool {
	if s.trace.stage == "flush" {
		s.trace.mu.Lock()
		canceled := !s.trace.canceled.IsZero()
		s.trace.mu.Unlock()
		if canceled {
			s.trace.afterFlush.Add(1)
		}
	}
	return s.SearchResultSet.Next()
}

func (s infoCancelResultSet) Close() error {
	s.trace.setsClosed.Add(1)
	return s.SearchResultSet.Close()
}

type infoCancelRecorder struct {
	*httptest.ResponseRecorder
	trace *infoCancelTrace
}

func (r infoCancelRecorder) Flush() {
	r.ResponseRecorder.Flush()
	if r.trace.stage == "flush" {
		r.trace.trigger()
	}
}

func runInfoCancellation(tb testing.TB, queryable storage.SampleAndChunkQueryable, c infoBenchCase, method string, values bool, stage string) time.Duration {
	tb.Helper()
	// This deadline is a hang guard, not a cancellation latency assertion.
	ctx, stop := context.WithTimeout(tb.Context(), 10*time.Second)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	trace := &infoCancelTrace{stage: stage, cancel: cancel}
	_, engine, handler := newInfoBenchAPI(infoCancelQueryable{SampleAndChunkQueryable: queryable, trace: trace})
	defer func() { require.NoError(tb, engine.Close()) }()
	path := "/api/v1/info_labels"
	if values {
		path = "/api/v1/info_label_values"
	}
	params := c.params(values)
	request := httptest.NewRequestWithContext(ctx, method, path+"?"+params.Encode(), http.NoBody)
	if method == http.MethodPost {
		request = httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(params.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	recorder := infoCancelRecorder{ResponseRecorder: httptest.NewRecorder(), trace: trace}
	handler.ServeHTTP(recorder, request)
	returned := time.Now()
	trace.mu.Lock()
	canceled := trace.canceled
	contexts := append([]context.Context(nil), trace.contexts...)
	trace.mu.Unlock()
	require.False(tb, canceled.IsZero(), "checkpoint was not reached")
	require.ErrorIs(tb, ctx.Err(), context.Canceled, "hang guard expired instead of checkpoint cancellation")
	require.NotEmpty(tb, contexts)
	for _, observed := range contexts {
		require.ErrorIs(tb, observed.Err(), context.Canceled)
	}
	require.Positive(tb, trace.opened.Load())
	require.Equal(tb, trace.opened.Load(), trace.closed.Load(), "querier leak or double close")
	require.Equal(tb, trace.sets.Load(), trace.setsClosed.Load(), "result-set leak or double close")
	if stage == "expression" {
		require.Zero(tb, trace.sets.Load(), "discovery must not start after expression cancellation")
	} else {
		require.Positive(tb, trace.sets.Load())
	}
	if stage == "flush" {
		require.Equal(tb, http.StatusOK, recorder.Code)
		require.Zero(tb, trace.afterFlush.Load(), "continued pulling results after cancellation")
		// A synthetic trailer lets the strict decoder validate the single
		// partial batch; any real trailer or extra record makes validation fail.
		body := append([]byte(nil), recorder.Body.Bytes()...)
		body = append(body, []byte("{\"status\":\"success\",\"has_more\":false}\n")...)
		got, err := decodeInfoBenchResponse(body, values, c.search == "subsequence")
		require.NoError(tb, err)
		want := c.expected(values)
		want.items, want.hasMore, want.batches = want.items[:min(25, len(want.items))], false, 1
		require.Equal(tb, want, got)
	} else {
		require.Equal(tb, statusClientClosedConnection, recorder.Code, recorder.Body.String())
		var response Response
		require.NoError(tb, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Equal(tb, statusError, response.Status)
		require.Equal(tb, errorCanceled.str, response.ErrorType)
	}
	return returned.Sub(canceled)
}

func TestInfoAutocompleteCancellation(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed=%t", mixed), func(t *testing.T) {
			fixture := newInfoBenchFixture(t, t.TempDir(), 100, mixed)
			db := fixture.open(t)
			defer func() { require.NoError(t, db.Close()) }()
			c := infoBenchCase{targets: 100, mixed: mixed, expr: "selector", search: "substring", limit: 100}
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				for _, values := range []bool{false, true} {
					for _, stage := range []string{"expression", "filter", "flush"} {
						t.Run(fmt.Sprintf("%s/values=%t/%s", method, values, stage), func(t *testing.T) {
							runInfoCancellation(t, db, c, method, values, stage)
						})
					}
				}
			}
		})
	}
}

// BenchmarkInfoAutocompleteCancellation measures checkpoint-to-handler-return
// latency on real query iteration, substring filtering, and batch flushing.
func BenchmarkInfoAutocompleteCancellation(b *testing.B) {
	// Use the fixed-iteration reproduction command in info_labels_bench_test.go.
	cases := []infoBenchCase{
		{targets: 5000, expr: "selector", search: "substring", limit: 100},
		{targets: 5000, mixed: true, expr: "selector", search: "substring", limit: 100},
	}
	forInfoBenchCases(b, cases, func(b *testing.B, fixture infoBenchFixture, c infoBenchCase) {
		db := fixture.open(b)
		defer func() { require.NoError(b, db.Close()) }()
		for _, values := range []bool{false, true} {
			for _, stage := range []string{"expression", "filter", "flush"} {
				b.Run(fmt.Sprintf("values=%t/%s", values, stage), func(b *testing.B) {
					b.StopTimer()
					samples := make([]time.Duration, 0, b.N)
					for range 5 {
						runInfoCancellation(b, db, c, http.MethodGet, values, stage)
					}
					// Keep the driver timed so Go can also calibrate a normal
					// duration-based run; only custom cancellation metrics are shown.
					b.StartTimer()
					for range b.N {
						elapsed := runInfoCancellation(b, db, c, http.MethodGet, values, stage)
						samples = append(samples, elapsed)
					}
					b.StopTimer()
					// Report only the measured cancellation interval, excluding
					// fixture access, API construction, and response assertions.
					b.ReportMetric(0, "ns/op")
					b.ReportMetric(float64(b.N), "samples")
					reportInfoBenchLatency(b, "cancel", samples)
				})
			}
		}
	})
}

func TestInfoAutocompleteBenchmarkResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		valid bool
	}{
		{name: "complete", body: "{\"results\":[{\"name\":\"build_id\"}]}\n{\"status\":\"success\",\"has_more\":false}\n", valid: true},
		{name: "empty", body: "{\"results\":[]}\n{\"status\":\"success\",\"has_more\":false}\n", valid: true},
		{name: "missing trailer", body: "{\"results\":[]}\n"},
		{name: "missing batch", body: "{\"status\":\"success\",\"has_more\":false}\n"},
		{name: "missing has_more", body: "{\"results\":[]}\n{\"status\":\"success\"}\n"},
		{name: "duplicate trailer", body: "{\"results\":[]}\n{\"status\":\"success\",\"has_more\":false}\n{\"status\":\"success\",\"has_more\":false}\n"},
		{name: "trailing content", body: "{\"results\":[]}\n{\"status\":\"success\",\"has_more\":false}\n{\"results\":[]}\n"},
		{name: "malformed", body: "{\"results\":[]}{}\n"},
		{name: "wrong record", body: "{\"results\":[{\"value\":\"build_id\"}]}\n{\"status\":\"success\",\"has_more\":false}\n"},
		{name: "warning", body: "{\"results\":[],\"warnings\":[\"partial\"]}\n{\"status\":\"success\",\"has_more\":false}\n"},
		{name: "error", body: "{\"results\":[]}\n{\"status\":\"error\",\"errorType\":\"timeout\",\"error\":\"timeout\"}\n"},
		{name: "null results", body: "{\"results\":null}\n{\"status\":\"success\",\"has_more\":false}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeInfoBenchResponse([]byte(tc.body), false, false)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
