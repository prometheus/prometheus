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

func verifyInfoBenchScope(tb testing.TB, api *API, c infoBenchCase) {
	tb.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/info_labels?"+c.params(false).Encode(), http.NoBody)
	prepared := api.prepareSearchRequest(httptest.NewRecorder(), req, "info_labels", searchRequestOptions{exprControlsTimeRange: true})
	require.NotNil(tb, prepared)
	scope, apiErr := api.infoMetricMatcherSets(tb.Context(), req, prepared.sp)
	require.Nil(tb, apiErr)
	require.Empty(tb, scope.warnings)
	require.Len(tb, scope.matcherSets, 1)
	// The expression lookback excludes its left boundary by one millisecond.
	expectedStart := int64(0)
	if c.expr != "none" {
		expectedStart = 1
	}
	require.Equal(tb, expectedStart, scope.mint)
	require.Equal(tb, infoBenchEnd, scope.maxt)
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
// Record the revision and environment, and compare identical harnesses with benchstat.
//
//	go test ./web/api/v1 -run '^$' -bench '^BenchmarkInfoAutocomplete$' -benchmem -count=6

// BenchmarkInfoAutocomplete measures the in-process HTTP driver, routing,
// response buffering, and serialization, including driver allocations.
func BenchmarkInfoAutocomplete(b *testing.B) {
	forInfoBenchCases(b, infoBenchCases(), func(b *testing.B, fixture infoBenchFixture, c infoBenchCase) {
		db := fixture.open(b)
		defer func() { require.NoError(b, db.Close()) }()
		api, engine, handler := newInfoBenchAPI(db)
		defer func() { require.NoError(b, engine.Close()) }()
		verifyInfoBenchScope(b, api, c)
		for _, values := range []bool{false, true} {
			mode, path := "names", "/api/v1/search/info_labels"
			if values {
				mode, path = "values", "/api/v1/search/info_label_values"
			}
			b.Run(mode, func(b *testing.B) {
				b.StopTimer()
				requestURL := path + "?" + c.params(values).Encode()
				run := func() *httptest.ResponseRecorder {
					req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, requestURL, http.NoBody)
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, req)
					return rec
				}
				var reference *httptest.ResponseRecorder
				for range 5 {
					reference = run()
					require.Equal(b, http.StatusOK, reference.Code, reference.Body.String())
					require.Equal(b, "application/x-ndjson; charset=utf-8", reference.Header().Get("Content-Type"))
					response, err := decodeInfoBenchResponse(reference.Body.Bytes(), values, c.search == "subsequence")
					require.NoError(b, err)
					require.Equal(b, c.expected(values), response)
				}
				b.ReportAllocs()
				b.StartTimer()
				for b.Loop() {
					got := run()
					// Compare against a complete, strictly validated response without
					// adding per-iteration decoding or timer stops to the measurement.
					if got.Code != reference.Code || got.Header().Get("Content-Type") != reference.Header().Get("Content-Type") || !bytes.Equal(got.Body.Bytes(), reference.Body.Bytes()) {
						b.Fatalf("response differs from validated reference: %s", got.Body.String())
					}
				}
				b.ReportMetric(float64(reference.Body.Len()), "body-bytes/op")
			})
		}
	})
}
