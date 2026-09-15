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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

const infoBenchEnd int64 = 600_000

type infoBenchCase struct {
	targets   int
	mixed     bool
	expr      string
	selective bool
	search    string
	limit     int
}

func (c infoBenchCase) name() string {
	layout, scope := "head", "broad"
	if c.mixed {
		layout = "blocks_head"
	}
	if c.selective {
		scope = "one_job"
	}
	search := c.search
	if search == "" {
		search = "unfiltered"
	}
	return fmt.Sprintf("targets=%d/%s/%s/%s/%s/limit=%d", c.targets, layout, c.expr, scope, search, c.limit)
}

func infoBenchCases() []infoBenchCase {
	var cases []infoBenchCase
	for _, targets := range []int{100, 5000} {
		for _, expr := range []string{"none", "selector"} {
			for _, selective := range []bool{false, true} {
				cases = append(cases, infoBenchCase{targets: targets, expr: expr, selective: selective, limit: 100})
			}
		}
	}
	for _, expr := range []string{"none", "selector"} {
		cases = append(cases, infoBenchCase{targets: 5000, mixed: true, expr: expr, limit: 100})
	}
	for _, mixed := range []bool{false, true} {
		cases = append(cases, infoBenchCase{targets: 5000, mixed: mixed, expr: "rate", limit: 100})
	}
	for _, search := range []string{"substring", "subsequence"} {
		cases = append(cases, infoBenchCase{targets: 5000, mixed: true, expr: "selector", search: search, limit: 100})
	}
	return append(cases, infoBenchCase{targets: 5000, mixed: true, expr: "selector", limit: 6000})
}

func (c infoBenchCase) params(values bool) url.Values {
	p := url.Values{
		"start": {"0"}, "end": {"600"}, "time": {"600"}, "lookback_delta": {"10m"},
		"limit": {strconv.Itoa(c.limit)}, "batch_size": {"25"},
		"fuzz_alg": {"subsequence"}, "fuzz_threshold": {"0"}, "sort_by": {"alpha"},
	}
	selector := "http_requests_total"
	if c.selective {
		p.Set("data_match[]", `job="job_0"`)
		selector += `{job="job_0"}`
	}
	switch c.expr {
	case "selector":
		p.Set("expr", selector)
	case "rate":
		p.Set("expr", "rate("+selector+"[2m])")
	}
	if values {
		p.Set("label", "build_id")
	}
	switch c.search {
	case "substring":
		p.Set("fuzz_alg", "jarowinkler")
		p.Set("search[]", "build")
		if values {
			p.Set("search[]", "value_")
		}
	case "subsequence":
		p.Set("sort_by", "score")
		p.Set("include_score", "true")
		p.Set("search[]", "bid")
		if values {
			p.Set("search[]", "vle")
		}
	}
	return p
}

func infoBenchLabelCount(targets int) int {
	if targets == 100 {
		return 32
	}
	return 256
}

type infoBenchFixture struct {
	dir     string
	targets int
	mixed   bool
}

func newInfoBenchFixture(tb testing.TB, dir string, targets int, mixed bool) infoBenchFixture {
	tb.Helper()
	require.NoError(tb, os.MkdirAll(dir, 0o755))
	f := infoBenchFixture{dir: dir, targets: targets, mixed: mixed}
	series := make([]labels.Labels, 0, targets*5)
	for i := range targets {
		job, instance := fmt.Sprintf("job_%d", i%10), fmt.Sprintf("instance_%05d", i)
		for _, metric := range []string{"http_requests_total", "rpc_requests_total", "db_requests_total", "queue_requests_total"} {
			series = append(series, labels.FromStrings("__name__", metric, "job", job, "instance", instance))
		}
		builder := labels.NewBuilder(labels.FromStrings(
			"__name__", "target_info", "job", job, "instance", instance,
			"build_id", fmt.Sprintf("value_%05d", i), "env", "prod", "region", fmt.Sprintf("region_%d", i%4),
		))
		for j := range 5 {
			builder.Set(fmt.Sprintf("meta_id_%03d", (i*5+j)%(infoBenchLabelCount(targets)-3)), "present")
		}
		series = append(series, builder.Labels())
	}
	appendSamples := func(appendable storage.Appendable, start, end int64) {
		app := appendable.Appender(tb.Context())
		refs := make([]storage.SeriesRef, len(series))
		// Commit in timestamp order: a block writer advances its appendable
		// window after commits, so later series must not restart at old times.
		for ts := start; ts < end; ts += 30_000 {
			for i, ls := range series {
				value := float64(ts/30_000 + 1)
				if ls.Get(labels.MetricName) == "target_info" {
					value = 1
				}
				var err error
				refs[i], err = app.Append(refs[i], ls, ts, value)
				require.NoError(tb, err)
				if i%1000 == 999 {
					require.NoError(tb, app.Commit())
					app = appendable.Appender(tb.Context())
				}
			}
		}
		require.NoError(tb, app.Commit())
	}
	var headStart int64
	if mixed {
		for block := range 3 {
			func() {
				writer, err := tsdb.NewBlockWriter(slog.New(slog.DiscardHandler), dir, 120_000)
				require.NoError(tb, err)
				defer func() { require.NoError(tb, writer.Close()) }()
				appendSamples(writer, int64(block)*120_000, int64(block+1)*120_000)
				_, err = writer.Flush(tb.Context())
				require.NoError(tb, err)
			}()
		}
		headStart = 360_000
	}
	db := f.open(tb)
	defer func() { require.NoError(tb, db.Close()) }()
	appendSamples(db, headStart, infoBenchEnd+1)
	require.Equal(tb, uint64(targets*5), db.Head().NumSeries())
	// A head following persisted blocks starts at the last block's exclusive
	// maximum, including the gap before its first appended sample.
	if mixed {
		require.Equal(tb, db.Blocks()[2].MaxTime(), db.Head().MinTime())
	} else {
		require.Equal(tb, headStart, db.Head().MinTime())
	}
	require.Equal(tb, infoBenchEnd, db.Head().MaxTime())
	return f
}

func (f infoBenchFixture) open(tb testing.TB) *tsdb.DB {
	tb.Helper()
	opts := tsdb.DefaultOptions()
	opts.RetentionDuration = 0
	db, err := tsdb.Open(f.dir, slog.New(slog.DiscardHandler), nil, opts, nil)
	require.NoError(tb, err)
	db.DisableCompactions()
	expectedBlocks := 0
	if f.mixed {
		expectedBlocks = 3
	}
	require.Len(tb, db.Blocks(), expectedBlocks)
	for i, block := range db.Blocks() {
		require.Equal(tb, int64(i)*120_000, block.MinTime())
		require.Equal(tb, int64(i+1)*120_000-30_000+1, block.MaxTime())
	}
	return db
}

func newInfoBenchAPI(queryable storage.SampleAndChunkQueryable) (*API, *promql.Engine, http.Handler) {
	engine := promql.NewEngine(promql.EngineOpts{
		Logger: slog.New(slog.DiscardHandler), MaxSamples: 1_000_000, Timeout: 2 * time.Minute,
		LookbackDelta: 10 * time.Minute, EnableAtModifier: true, EnableNegativeOffset: true,
		NoStepSubqueryIntervalFn: func(int64) int64 { return 60_000 },
	})
	api := minimalSearchAPI()
	api.Queryable = queryable
	api.QueryEngine = engine
	api.enableExperimentalFunctions = true
	api.queryTimeout = 2 * time.Minute
	api.now = func() time.Time { return time.UnixMilli(infoBenchEnd) }
	r := route.New().WithPrefix("/api/v1")
	api.Register(r)
	return api, engine, r
}

type infoBenchResponse struct {
	items   []string
	hasMore bool
	batches int
}

// decodeInfoBenchResponse rejects incomplete, malformed, and unexpected streams.
func decodeInfoBenchResponse(body []byte, values, scores bool) (infoBenchResponse, error) {
	var result infoBenchResponse
	if len(body) == 0 || body[len(body)-1] != '\n' {
		return result, errors.New("missing final newline")
	}
	lines := bytes.Split(body[:len(body)-1], []byte{'\n'})
	terminal := false
	var previousScore float64
	for i, line := range lines {
		var envelope struct {
			Results  json.RawMessage `json:"results"`
			Status   *string         `json:"status"`
			HasMore  *bool           `json:"has_more"`
			Warnings []string        `json:"warnings"`
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&envelope); err != nil || !json.Valid(line) {
			return result, fmt.Errorf("invalid record %d: %s", i, line)
		}
		if terminal || len(envelope.Warnings) != 0 {
			return result, fmt.Errorf("unexpected warning or content after trailer: %s", line)
		}
		if envelope.Status != nil {
			if *envelope.Status != "success" || envelope.HasMore == nil || envelope.Results != nil || i == 0 {
				return result, fmt.Errorf("invalid success trailer: %s", line)
			}
			terminal, result.hasMore = true, *envelope.HasMore
			continue
		}
		if envelope.HasMore != nil || len(envelope.Results) == 0 || bytes.Equal(envelope.Results, []byte("null")) {
			return result, fmt.Errorf("missing results: %s", line)
		}
		var records []struct {
			Name  *string  `json:"name"`
			Value *string  `json:"value"`
			Score *float64 `json:"score"`
		}
		dec = json.NewDecoder(bytes.NewReader(envelope.Results))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&records); err != nil {
			return result, err
		}
		for _, record := range records {
			item, other := record.Name, record.Value
			if values {
				item, other = record.Value, record.Name
			}
			if item == nil || other != nil || scores != (record.Score != nil) {
				return result, fmt.Errorf("unexpected result shape: %s", line)
			}
			if scores {
				score := *record.Score
				if math.IsNaN(score) || score <= 0 || score > 1 || (len(result.items) > 0 && score != previousScore) {
					return result, fmt.Errorf("expected equal positive scores for this fixture: %s", line)
				}
				previousScore = score
			}
			result.items = append(result.items, *item)
		}
		result.batches++
	}
	if !terminal {
		return result, errors.New("missing success trailer")
	}
	return result, nil
}

func (c infoBenchCase) expected(values bool) infoBenchResponse {
	names := map[string]struct{}{"build_id": {}, "env": {}, "region": {}}
	var items []string
	for i := range c.targets {
		if c.selective && i%10 != 0 {
			continue
		}
		if values {
			items = append(items, fmt.Sprintf("value_%05d", i))
		} else {
			for j := range 5 {
				names[fmt.Sprintf("meta_id_%03d", (i*5+j)%(infoBenchLabelCount(c.targets)-3))] = struct{}{}
			}
		}
	}
	if !values {
		for name := range names {
			if c.search == "" || name == "build_id" {
				items = append(items, name)
			}
		}
	}
	// Search variants match only build_id for names and every fixed-width
	// value for values. Their scores tie, so both orders are alphabetical.
	slices.Sort(items)
	hasMore := len(items) > c.limit
	items = items[:min(len(items), c.limit)]
	return infoBenchResponse{items: items, hasMore: hasMore, batches: max(1, (len(items)+24)/25)}
}

type infoBenchRecorder struct {
	*httptest.ResponseRecorder
	start, firstFlush time.Time
}

func (r *infoBenchRecorder) Flush() {
	r.ResponseRecorder.Flush()
	if !r.start.IsZero() && r.firstFlush.IsZero() {
		r.firstFlush = time.Now()
	}
}

type infoBenchObservation struct {
	recorder *infoBenchRecorder
	elapsed  time.Duration
	values   bool
}

type infoBenchOperation struct {
	namesURL, valuesURL string
	mode                string
	scores              bool
}

func newInfoBenchOperation(c infoBenchCase, mode string) infoBenchOperation {
	return infoBenchOperation{
		namesURL:  "/api/v1/info_labels?" + c.params(false).Encode(),
		valuesURL: "/api/v1/info_label_values?" + c.params(true).Encode(),
		mode:      mode, scores: c.search == "subsequence",
	}
}

func (op infoBenchOperation) run(tb testing.TB, handler http.Handler, timed bool) ([]infoBenchObservation, time.Duration) {
	tb.Helper()
	var observations []infoBenchObservation
	var start time.Time
	if timed {
		start = time.Now()
	}
	for _, values := range []bool{false, true} {
		if (values && op.mode == "names") || (!values && op.mode == "values") {
			continue
		}
		requestURL := op.namesURL
		if values {
			requestURL = op.valuesURL
		}
		req := httptest.NewRequestWithContext(tb.Context(), http.MethodGet, requestURL, http.NoBody)
		rec := &infoBenchRecorder{ResponseRecorder: httptest.NewRecorder()}
		if timed {
			rec.start = time.Now()
		}
		handler.ServeHTTP(rec, req)
		var elapsed time.Duration
		if timed {
			elapsed = time.Since(rec.start)
		}
		observations = append(observations, infoBenchObservation{recorder: rec, elapsed: elapsed, values: values})
		if !values && op.mode == "interaction" {
			response, err := decodeInfoBenchResponse(rec.Body.Bytes(), false, op.scores)
			require.NoError(tb, err)
			index := slices.Index(response.items, "build_id")
			require.NotEqual(tb, -1, index, "selected label must be returned by names discovery")
			parsed, err := url.Parse(op.valuesURL)
			require.NoError(tb, err)
			params := parsed.Query()
			params.Set("label", response.items[index])
			parsed.RawQuery = params.Encode()
			op.valuesURL = parsed.String()
		}
	}
	if timed {
		return observations, time.Since(start)
	}
	return observations, 0
}

type infoBenchExpectation struct {
	names, values infoBenchResponse
	scores        bool
}

func newInfoBenchExpectation(c infoBenchCase) infoBenchExpectation {
	return infoBenchExpectation{names: c.expected(false), values: c.expected(true), scores: c.search == "subsequence"}
}

func validateInfoBenchObservations(tb testing.TB, expected infoBenchExpectation, observations []infoBenchObservation) int {
	tb.Helper()
	var bodyBytes int
	for _, observation := range observations {
		rec := observation.recorder
		require.Equal(tb, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(tb, "application/x-ndjson; charset=utf-8", rec.Header().Get("Content-Type"))
		response, err := decodeInfoBenchResponse(rec.Body.Bytes(), observation.values, expected.scores)
		require.NoError(tb, err)
		want := expected.names
		if observation.values {
			want = expected.values
		}
		require.Equal(tb, want, response)
		bodyBytes += rec.Body.Len()
	}
	return bodyBytes
}

func verifyInfoBenchScope(tb testing.TB, api *API, c infoBenchCase) {
	tb.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/info_labels?"+c.params(false).Encode(), http.NoBody)
	prepared := api.prepareAutocompleteRequest(httptest.NewRecorder(), req, "info_labels", autocompleteRequestOptions{exprControlsTimeRange: true})
	require.NotNil(tb, prepared)
	sets, warnings, empty, mint, maxt, apiErr := api.infoMetricMatcherSets(tb.Context(), req, prepared.sp)
	require.Nil(tb, apiErr)
	require.Empty(tb, warnings)
	require.False(tb, empty)
	require.Len(tb, sets, 1)
	// The expression lookback excludes its left boundary by one millisecond.
	expectedStart := int64(0)
	if c.expr != "none" {
		expectedStart = 1
	}
	require.Equal(tb, expectedStart, mint)
	require.Equal(tb, infoBenchEnd, maxt)
}

func forInfoBenchCases(b *testing.B, cases []infoBenchCase, run func(*testing.B, infoBenchFixture, infoBenchCase)) {
	b.Helper()
	root := b.TempDir()
	fixtures := map[string]infoBenchFixture{}
	for _, c := range cases {
		b.Run(c.name(), func(b *testing.B) {
			key := fmt.Sprintf("%d_%t", c.targets, c.mixed)
			fixture, ok := fixtures[key]
			if !ok {
				fixture = newInfoBenchFixture(b, filepath.Join(root, key), c.targets, c.mixed)
				fixtures[key] = fixture
			}
			run(b, fixture, c)
		})
	}
}

// Run these synthetic local TSDB benchmarks serially from the repository root.
// Capture output outside the checkout and record the commit, Go version, CPU,
// memory, OS, GOMAXPROCS, and system load with the results.
//
//	go test ./web/api/v1 -run '^$' -bench '^BenchmarkInfoAutocomplete(Phases)?$' -benchmem -count=6
//	go test ./web/api/v1 -run '^$' -bench '^BenchmarkInfoAutocomplete(Latency|ReopenedLatency|Cancellation)$' -benchtime=200x -count=6
//
// Use benchstat for throughput and phase results. Compare revisions with the
// same harness and environment.

// BenchmarkInfoAutocomplete measures the in-process request driver, routing,
// response buffering, and serialization, including driver allocations and
// selection from the names response during an interaction.
func BenchmarkInfoAutocomplete(b *testing.B) {
	forInfoBenchCases(b, infoBenchCases(), func(b *testing.B, fixture infoBenchFixture, c infoBenchCase) {
		db := fixture.open(b)
		defer func() { require.NoError(b, db.Close()) }()
		api, engine, handler := newInfoBenchAPI(db)
		defer func() { require.NoError(b, engine.Close()) }()
		verifyInfoBenchScope(b, api, c)
		expected := newInfoBenchExpectation(c)
		for _, mode := range []string{"names", "values", "interaction"} {
			b.Run(mode, func(b *testing.B) {
				b.StopTimer()
				op := newInfoBenchOperation(c, mode)
				var bodyBytes int
				var reference []infoBenchObservation
				for range 5 {
					reference, _ = op.run(b, handler, false)
					bodyBytes = validateInfoBenchObservations(b, expected, reference)
				}
				b.ReportAllocs()
				b.StartTimer()
				for b.Loop() {
					observations, _ := op.run(b, handler, false)
					// Byte equality with the strictly validated reference catches
					// failed/truncated responses without per-iteration decoding or
					// timer stops dominating the cheapest requests.
					for i, observation := range observations {
						got, want := observation.recorder, reference[i].recorder
						if got.Code != want.Code || got.Header().Get("Content-Type") != want.Header().Get("Content-Type") || !bytes.Equal(got.Body.Bytes(), want.Body.Bytes()) {
							b.Fatalf("response differs from validated reference: %s", got.Body.String())
						}
					}
				}
				b.ReportMetric(float64(bodyBytes), "body-bytes/op")
			})
		}
	})
}

// BenchmarkInfoAutocompleteLatency samples request-entry-to-return and first
// flush latency separately from the allocation benchmark.
func BenchmarkInfoAutocompleteLatency(b *testing.B) {
	benchmarkInfoAutocompleteLatency(b, false)
}

// BenchmarkInfoAutocompleteReopenedLatency recreates storage and engine before
// each sample. It does not evict the OS page cache or measure database startup.
func BenchmarkInfoAutocompleteReopenedLatency(b *testing.B) {
	benchmarkInfoAutocompleteLatency(b, true)
}

func benchmarkInfoAutocompleteLatency(b *testing.B, reopen bool) {
	b.Helper()
	cases := infoBenchCases()
	if reopen {
		cases = []infoBenchCase{
			{targets: 5000, mixed: true, expr: "none", limit: 100},
			{targets: 5000, mixed: true, expr: "selector", limit: 100},
		}
	}
	forInfoBenchCases(b, cases, func(b *testing.B, fixture infoBenchFixture, c infoBenchCase) {
		expected := newInfoBenchExpectation(c)
		for _, mode := range []string{"names", "values", "interaction"} {
			b.Run(mode, func(b *testing.B) {
				b.StopTimer()
				op := newInfoBenchOperation(c, mode)
				db := fixture.open(b)
				api, engine, handler := newInfoBenchAPI(db)
				closeInstance := func() {
					require.NoError(b, engine.Close())
					require.NoError(b, db.Close())
					if reopen {
						// Open creates a WAL segment even for read-only requests.
						// Remove only that empty segment after closing, so repeated
						// samples do not accumulate files or change replay work.
						walDir := filepath.Join(fixture.dir, "wal")
						_, last, err := wlog.Segments(walDir)
						require.NoError(b, err)
						segment := wlog.SegmentName(walDir, last)
						stat, err := os.Stat(segment)
						require.NoError(b, err)
						require.Zero(b, stat.Size(), "reopened benchmark unexpectedly wrote WAL data")
						require.NoError(b, os.Remove(segment))
					}
				}
				verifyInfoBenchScope(b, api, c)
				var bodyBytes int
				for range 5 {
					observations, _ := op.run(b, handler, false)
					bodyBytes = validateInfoBenchObservations(b, expected, observations)
				}
				if reopen {
					// Preflight must not warm an instance used for a fresh sample.
					closeInstance()
				} else {
					defer closeInstance()
				}
				latencies := map[string][]time.Duration{}
				for _, metric := range []string{"names", "values", "names-first-batch", "values-first-batch", "interaction"} {
					latencies[metric] = make([]time.Duration, 0, b.N)
				}
				// Opening, WAL replay, and closing stay outside request timing.
				// An interaction shares its instance, so names can warm values.
				for range b.N {
					if reopen {
						db = fixture.open(b)
						_, engine, handler = newInfoBenchAPI(db)
					}
					b.StartTimer()
					observations, elapsed := op.run(b, handler, true)
					b.StopTimer()
					if reopen {
						closeInstance()
					}
					require.Equal(b, bodyBytes, validateInfoBenchObservations(b, expected, observations))
					if mode == "interaction" {
						latencies["interaction"] = append(latencies["interaction"], elapsed)
					}
					for _, observation := range observations {
						metric := "names"
						if observation.values {
							metric = "values"
						}
						require.False(b, observation.recorder.firstFlush.IsZero())
						latencies[metric] = append(latencies[metric], observation.elapsed)
						latencies[metric+"-first-batch"] = append(latencies[metric+"-first-batch"], observation.recorder.firstFlush.Sub(observation.recorder.start))
					}
				}
				for metric, samples := range latencies {
					if len(samples) != 0 {
						reportInfoBenchLatency(b, metric, samples)
					}
				}
				b.ReportMetric(float64(b.N), "samples")
				b.ReportMetric(float64(bodyBytes), "body-bytes/op")
			})
		}
	})
}

func reportInfoBenchLatency(b *testing.B, metric string, samples []time.Duration) {
	b.Helper()
	slices.Sort(samples)
	// Percentiles use nearest rank within this repetition. Keep each run's
	// p50/p95 separate when summarizing repeated measurements.
	for _, percentile := range []int{50, 95} {
		index := (len(samples)*percentile+99)/100 - 1
		b.ReportMetric(float64(samples[index].Nanoseconds()), fmt.Sprintf("%s-p%d-ns", metric, percentile))
	}
}

// BenchmarkInfoAutocompletePhases excludes HTTP handling. Discovery includes
// fresh filter and querier construction, limit+1 consumption, and closure.
// Phase timings are not an additive breakdown of the request benchmark.
func BenchmarkInfoAutocompletePhases(b *testing.B) {
	var cases []infoBenchCase
	for _, mixed := range []bool{false, true} {
		for _, expr := range []string{"none", "selector", "rate"} {
			cases = append(cases, infoBenchCase{targets: 5000, mixed: mixed, expr: expr, limit: 100})
		}
	}
	forInfoBenchCases(b, cases, func(b *testing.B, fixture infoBenchFixture, c infoBenchCase) {
		db := fixture.open(b)
		defer func() { require.NoError(b, db.Close()) }()
		api, engine, _ := newInfoBenchAPI(db)
		defer func() { require.NoError(b, engine.Close()) }()
		verifyInfoBenchScope(b, api, c)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/info_labels?"+c.params(false).Encode(), http.NoBody)
		prepared := api.prepareAutocompleteRequest(httptest.NewRecorder(), req, "info_labels", autocompleteRequestOptions{exprControlsTimeRange: true})
		require.NotNil(b, prepared)
		sets, _, _, mint, maxt, apiErr := api.infoMetricMatcherSets(b.Context(), req, prepared.sp)
		require.Nil(b, apiErr)
		b.Run("expression_and_scope", func(b *testing.B) {
			b.ReportAllocs()
			var last [][]*labels.Matcher
			for b.Loop() {
				got, warnings, empty, start, end, err := api.infoMetricMatcherSets(b.Context(), req, prepared.sp)
				if err != nil || len(warnings) != 0 || empty || start != mint || end != maxt {
					b.Fatalf("invalid expression scope: error=%v warnings=%v empty=%t range=[%d,%d]", err, warnings, empty, start, end)
				}
				last = got
			}
			require.Len(b, last, len(sets))
			for i, set := range sets {
				require.Len(b, last[i], len(set))
				for j, matcher := range set {
					require.Equal(b, matcher.String(), last[i][j].String())
				}
			}
		})
		for _, values := range []bool{false, true} {
			name, total := "discover_names", infoBenchLabelCount(c.targets)
			if values {
				name, total = "discover_values", c.targets
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					q, err := db.Querier(mint, maxt)
					require.NoError(b, err)
					searcher := q.(storage.Searcher)
					// Only immutable scope is reused. Filters and their memo tables
					// have the same per-request lifetime as in the HTTP handlers.
					hints := &storage.SearchHints{
						OrderBy: sortOrdering(prepared.sp.sortBy, prepared.sp.sortDir),
						Limit:   searchHintsLimit(c.limit),
						Filter:  buildSearchFilter(prepared.sp.searches, prepared.sp.fuzzThreshold, prepared.sp.fuzzAlg, prepared.sp.caseSensitive),
					}
					var rs storage.SearchResultSet
					if values {
						rs = searchLabelValues(b.Context(), searcher, "build_id", sets, hints)
					} else {
						hints.Filter = infoDataLabelFilter{filter: hints.Filter}
						rs = searchLabelNames(b.Context(), searcher, sets, hints)
					}
					count := 0
					for rs.Next() {
						_ = rs.At()
						count++
					}
					resultErr, resultCloseErr, queryCloseErr := rs.Err(), rs.Close(), q.Close()
					if resultErr != nil || resultCloseErr != nil || queryCloseErr != nil || count != min(total, c.limit+1) {
						b.Fatalf("invalid discovery: error=%v result close=%v querier close=%v count=%d", resultErr, resultCloseErr, queryCloseErr, count)
					}
				}
			})
		}
	})
}
