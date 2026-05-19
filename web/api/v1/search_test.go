// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/annotations"
)

// newSearchTestAPI creates a minimal API suitable for search endpoint testing.
func newSearchTestAPI(t *testing.T) *API {
	t.Helper()

	s := promqltest.LoadedStorage(t, `
		load 1m
			go_gc_duration_seconds{instance="localhost:9090", job="prometheus"} 0+1x100
			go_gc_heap_goal_bytes{instance="localhost:9090", job="prometheus"} 0+1x100
			go_goroutines{instance="localhost:9090", job="prometheus"} 0+1x100
			up{instance="localhost:9090", job="prometheus"} 1+0x100
			up{instance="localhost:9091", job="node"} 1+0x100
			process_cpu_seconds_total{instance="localhost:9090", job="prometheus"} 0+1x100
	`)

	tr := newTestTargetRetriever([]*testTargetParams{
		{
			Identifier: "test",
			Labels: labels.FromMap(map[string]string{
				model.SchemeLabel:      "http",
				model.AddressLabel:     "localhost:9090",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "prometheus",
			}),
			Params: url.Values{},
			Active: true,
		},
	})
	require.NoError(t, tr.SetMetadataStoreForTargets("test", &testMetaStore{
		Metadata: []scrape.MetricMetadata{
			{MetricFamily: "go_gc_duration_seconds", Type: model.MetricTypeGauge, Help: "GC duration.", Unit: "seconds"},
			{MetricFamily: "go_goroutines", Type: model.MetricTypeGauge, Help: "Number of goroutines."},
			{MetricFamily: "up", Type: model.MetricTypeGauge, Help: "Target is up."},
		},
	}))

	// The test data is loaded from t=0 to t=6000s (100 minutes at 1m intervals).
	// Use a fixed now at t=7200s so the default 1-hour lookback [3600s, 7200s] covers the data.
	fixedNow := time.Unix(7200, 0)
	return &API{
		Queryable:       s,
		targetRetriever: tr.toFactory(),
		now:             func() time.Time { return fixedNow },
		config:          func() config.Config { return samplePrometheusCfg },
		ready:           func(f http.HandlerFunc) http.HandlerFunc { return f },
		parser:          parser.NewParser(parser.Options{}),
		enableSearch:    true,
		metaCache:       &searchMetadataCache{},
	}
}

// minimalSearchAPI returns an API with only the fields a search-endpoint
// test needs when it doesn't want the full target-retriever/storage
// scaffolding from newSearchTestAPI. metaCache is initialized so the
// metadata-cache path matches production. Callers must set Queryable
// before use.
func minimalSearchAPI() *API {
	return &API{
		now:          func() time.Time { return time.Unix(7200, 0) },
		ready:        func(f http.HandlerFunc) http.HandlerFunc { return f },
		parser:       parser.NewParser(parser.Options{}),
		enableSearch: true,
		metaCache:    &searchMetadataCache{},
	}
}

// parseNDJSON parses NDJSON response body into individual JSON objects.
func parseNDJSON(t *testing.T, body string) []json.RawMessage {
	t.Helper()
	var lines []json.RawMessage
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, json.RawMessage(line))
	}
	require.NoError(t, scanner.Err())
	return lines
}

// doSearchRequest performs a GET request to the search endpoint.
func doSearchRequest(t *testing.T, api *API, path string, params url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return doSearchRequestCtx(t.Context(), t, api, path, params)
}

// doSearchRequestCtx performs a GET request to the search endpoint with a
// caller-supplied context. A nil ctx uses the default request context.
func doSearchRequestCtx(ctx context.Context, t *testing.T, api *API, path string, params url.Values) *httptest.ResponseRecorder {
	t.Helper()

	r := route.New()
	api.Register(r)

	req := httptest.NewRequest(http.MethodGet, path+"?"+params.Encode(), http.NoBody)
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	return rec
}

type errorSearchQuerier struct {
	errorTestQuerier
	err error
}

func (q errorSearchQuerier) SearchLabelNames(context.Context, *storage.SearchHints, ...*labels.Matcher) storage.SearchResultSet {
	return storage.ErrSearchResultSet(q.err)
}

func (q errorSearchQuerier) SearchLabelValues(context.Context, string, *storage.SearchHints, ...*labels.Matcher) storage.SearchResultSet {
	return storage.ErrSearchResultSet(q.err)
}

func TestSearchEndpointsMapTSDBNotReadyToUnavailable(t *testing.T) {
	testCases := []struct {
		name      string
		queryable errorTestQueryable
	}{
		{
			name:      "querier error",
			queryable: errorTestQueryable{err: tsdb.ErrNotReady},
		},
		{
			name: "search result error",
			queryable: errorTestQueryable{
				q: errorSearchQuerier{err: tsdb.ErrNotReady},
			},
		},
	}

	endpoints := []struct {
		name   string
		path   string
		params url.Values
	}{
		{name: "metric names", path: "/search/metric_names"},
		{name: "label names", path: "/search/label_names"},
		{name: "label values", path: "/search/label_values", params: url.Values{"label": []string{"job"}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := minimalSearchAPI()
			api.Queryable = tc.queryable

			for _, endpoint := range endpoints {
				t.Run(endpoint.name, func(t *testing.T) {
					rec := doSearchRequest(t, api, endpoint.path, endpoint.params)
					require.Equal(t, http.StatusInternalServerError, rec.Code)

					var response Response
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
					require.Equal(t, statusError, response.Status)
					require.Equal(t, errorUnavailable.str, response.ErrorType)
					require.Contains(t, response.Error, tsdb.ErrNotReady.Error())
				})
			}
		})
	}
}

func TestSearchMetricNames(t *testing.T) {
	api := newSearchTestAPI(t)

	t.Run("basic search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]": []string{"go_gc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "application/x-ndjson; charset=utf-8", rec.Header().Get("Content-Type"))

		lines := parseNDJSON(t, rec.Body.String())
		require.GreaterOrEqual(t, len(lines), 2) // At least one batch + trailer.

		// Check trailer.
		var trailer searchTrailer
		require.NoError(t, json.Unmarshal(lines[len(lines)-1], &trailer))
		require.Equal(t, "success", trailer.Status)

		// Check first batch contains results.
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.NotEmpty(t, batch.Results)

		// All results should contain "go_gc".
		for _, r := range batch.Results {
			require.Contains(t, r.Name, "go_gc")
		}
	})

	t.Run("empty search returns all", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		// Should have all 5 distinct metric names.
		require.Len(t, batch.Results, 5)
	})

	t.Run("with limit", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"limit": []string{"2"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 2)

		var trailer searchTrailer
		require.NoError(t, json.Unmarshal(lines[len(lines)-1], &trailer))
		require.True(t, trailer.HasMore)
	})

	t.Run("with metadata", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]":         []string{"go_gc_duration"},
			"include_metadata": []string{"true"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "go_gc_duration_seconds", batch.Results[0].Name)
		require.Equal(t, "gauge", batch.Results[0].Type)
		require.Equal(t, "GC duration.", batch.Results[0].Help)
		require.Equal(t, "seconds", batch.Results[0].Unit)
	})

	t.Run("with include_score", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]":      []string{"go_gc"},
			"include_score": []string{"true"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.NotEmpty(t, batch.Results)
		for _, r := range batch.Results {
			require.NotNil(t, r.Score, "score should be set when include_score=true")
		}
	})

	t.Run("without include_score score is absent", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]": []string{"go_gc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.NotEmpty(t, batch.Results)
		for _, r := range batch.Results {
			require.Nil(t, r.Score, "score should be absent when include_score is not set")
		}
	})

	t.Run("unknown params are ignored", func(t *testing.T) {
		// include_cardinality is no longer supported; the endpoint should succeed and ignore it.
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]":            []string{"up"},
			"include_cardinality": []string{"true"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "up", batch.Results[0].Name)
	})

	t.Run("sort by alpha ascending", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"sort_by": []string{"alpha"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		for i := 1; i < len(batch.Results); i++ {
			require.LessOrEqual(t, batch.Results[i-1].Name, batch.Results[i].Name)
		}
	})

	t.Run("sort by alpha descending", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"sort_by":  []string{"alpha"},
			"sort_dir": []string{"dsc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		for i := 1; i < len(batch.Results); i++ {
			require.GreaterOrEqual(t, batch.Results[i-1].Name, batch.Results[i].Name)
		}
	})

	t.Run("sort_by=score without search[] is rejected", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"sort_by": []string{"score"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("case insensitive search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]":       []string{"GO_GC"},
			"case_sensitive": []string{"false"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.NotEmpty(t, batch.Results)
		for _, r := range batch.Results {
			require.Contains(t, strings.ToLower(r.Name), "go_gc")
		}
	})

	t.Run("fuzzy search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]":       []string{"go_goroutins"},
			"fuzz_threshold": []string{"80"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		// Should find go_goroutines via fuzzy match.
		found := false
		for _, r := range batch.Results {
			if r.Name == "go_goroutines" {
				found = true
			}
		}
		require.True(t, found, "expected to find go_goroutines via fuzzy match")
	})

	t.Run("invalid sort_by", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"sort_by": []string{"invalid"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid sort_dir", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"sort_dir": []string{"invalid"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid fuzz_threshold", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"fuzz_threshold": []string{"200"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid bool params", func(t *testing.T) {
		tests := []string{
			"case_sensitive",
			"include_score",
			"include_metadata",
		}

		for _, param := range tests {
			t.Run(param, func(t *testing.T) {
				rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
					param: []string{"maybe"},
				})
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), "bad_data")
				require.Contains(t, rec.Body.String(), "invalid "+param)
			})
		}
	})

	t.Run("sort_dir without sort_by", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"sort_dir": []string{"asc"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("unsupported fuzz_alg", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"fuzz_alg": []string{"levenshtein"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid fuzz_alg", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]": []string{"go_gc"},
			"fuzz_alg": []string{"jarowinkler"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("batch_size zero uses server default", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"batch_size": []string{"0"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		// With batch_size=0 (server default 100), all 5 results fit in one batch + trailer.
		require.Len(t, lines, 2)
	})

	t.Run("sort_by cardinality is invalid", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]": []string{"up"},
			"sort_by":  []string{"cardinality"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("sort_dir with sort_by=score is invalid", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search[]": []string{"up"},
			"sort_by":  []string{"score"},
			"sort_dir": []string{"asc"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("batch_size splitting", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"batch_size": []string{"2"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		// With 5 metrics and batch_size=2, should have 3 batch lines (2+2+1) + 1 trailer.
		require.Len(t, lines, 4)

		// First two batches should have 2 results each.
		for i := range 2 {
			var batch searchBatch[searchMetricNameResult]
			require.NoError(t, json.Unmarshal(lines[i], &batch))
			require.Len(t, batch.Results, 2)
		}
		// Third batch should have 1 result (5 total).
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[2], &batch))
		require.Len(t, batch.Results, 1)
	})

	t.Run("end before start is rejected", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"start": []string{"7200"},
			"end":   []string{"3600"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "end timestamp must not be before start timestamp")
	})

	t.Run("limit cap rejects excessive limits", func(t *testing.T) {
		capped := newSearchTestAPI(t)
		capped.maxSearchLimit = 50

		rec := doSearchRequest(t, capped, "/search/metric_names", url.Values{
			"limit": []string{"5000"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "exceeds the configured maximum")
	})

	t.Run("limit cap below default still serves request", func(t *testing.T) {
		// Operators with a small cap should still be able to serve requests
		// that omit "limit"; the implicit default must shrink to the cap,
		// and the trailer must still report has_more so the client knows
		// the cap truncated the result set.
		capped := newSearchTestAPI(t)
		capped.maxSearchLimit = 3

		rec := doSearchRequest(t, capped, "/search/metric_names", url.Values{})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		require.NotEmpty(t, lines)
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.LessOrEqual(t, len(batch.Results), 3,
			"default limit must be clamped to maxSearchLimit when smaller")

		var trailer searchTrailer
		require.NoError(t, json.Unmarshal(lines[len(lines)-1], &trailer))
		require.Equal(t, "success", trailer.Status)
		require.True(t, trailer.HasMore,
			"has_more must be true when the cap truncated a fixture with more values than the cap")
	})

	t.Run("limit cap allows in-range explicit limit", func(t *testing.T) {
		capped := newSearchTestAPI(t)
		capped.maxSearchLimit = 50

		rec := doSearchRequest(t, capped, "/search/metric_names", url.Values{
			"limit": []string{"2"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("limit cap zero disables the cap", func(t *testing.T) {
		// maxSearchLimit defaults to 0 in the test fixture; oversize limits
		// must therefore be accepted.
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"limit": []string{"100000"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("multiple match sets are merged and deduplicated", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"match[]": []string{"up", `{job="prometheus"}`},
			"sort_by": []string{"alpha"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 5)

		seen := map[string]struct{}{}
		for i, r := range batch.Results {
			_, ok := seen[r.Name]
			require.False(t, ok, "duplicate result %q", r.Name)
			seen[r.Name] = struct{}{}
			if i > 0 {
				require.LessOrEqual(t, batch.Results[i-1].Name, r.Name)
			}
		}
		require.Contains(t, seen, "up")
	})
}

func TestSearchLabelNames(t *testing.T) {
	api := newSearchTestAPI(t)

	t.Run("basic search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{
			"search[]": []string{"inst"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.NotEmpty(t, batch.Results)
		found := false
		for _, r := range batch.Results {
			if r.Name == "instance" {
				found = true
			}
		}
		require.True(t, found)
	})

	t.Run("empty search returns all", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		// Should have __name__, instance, job at minimum.
		require.GreaterOrEqual(t, len(batch.Results), 3)
	})

	t.Run("search by exact name", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{
			"search[]": []string{"job"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "job", batch.Results[0].Name)
	})

	t.Run("sort by alpha", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{
			"sort_by": []string{"alpha"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		for i := 1; i < len(batch.Results); i++ {
			require.LessOrEqual(t, batch.Results[i-1].Name, batch.Results[i].Name)
		}
	})

	t.Run("invalid sort_by", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{
			"sort_by": []string{"invalid"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestSearchLabelValues(t *testing.T) {
	api := newSearchTestAPI(t)

	t.Run("basic search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":    []string{"job"},
			"search[]": []string{"prom"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "prometheus", batch.Results[0].Value)
	})

	t.Run("missing label parameter", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("all values for label", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label": []string{"job"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 2) // "prometheus" and "node".
	})

	t.Run("search exact value", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":    []string{"job"},
			"search[]": []string{"prometheus"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "prometheus", batch.Results[0].Value)
	})

	t.Run("sort by alpha descending", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":    []string{"job"},
			"sort_by":  []string{"alpha"},
			"sort_dir": []string{"dsc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 2)
		require.Equal(t, "prometheus", batch.Results[0].Value)
		require.Equal(t, "node", batch.Results[1].Value)
	})

	t.Run("with limit and has_more", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label": []string{"instance"},
			"limit": []string{"1"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)

		var trailer searchTrailer
		require.NoError(t, json.Unmarshal(lines[len(lines)-1], &trailer))
		require.True(t, trailer.HasMore)
	})

	t.Run("invalid sort_by", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":   []string{"job"},
			"sort_by": []string{"cardinality"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("sort_by frequency is invalid", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":   []string{"job"},
			"sort_by": []string{"frequency"},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("multiple search terms OR logic", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":    []string{"job"},
			"search[]": []string{"prometheus", "node"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		// Both "prometheus" and "node" should be returned.
		require.Len(t, batch.Results, 2)
	})
}

func TestMatchName(t *testing.T) {
	tests := []struct {
		name          string
		search        string
		fuzzThreshold int
		caseSensitive bool
		wantMatched   bool
		wantScore     float64
	}{
		{
			// Empty searches must produce a nil filter; the subtest
			// returns early after asserting that, so the matched/score
			// fields below are unused for this row.
			name: "go_goroutines", search: "", fuzzThreshold: 100, caseSensitive: true,
		},
		{
			name: "go_goroutines", search: "go_gor", fuzzThreshold: 100, caseSensitive: true,
			wantMatched: true, wantScore: 1.0,
		},
		{
			name: "go_goroutines", search: "GO_GOR", fuzzThreshold: 100, caseSensitive: true,
			wantMatched: false,
		},
		{
			name: "go_goroutines", search: "GO_GOR", fuzzThreshold: 100, caseSensitive: false,
			wantMatched: true, wantScore: 1.0,
		},
		{
			name: "go_goroutines", search: "xyz_not_found", fuzzThreshold: 100, caseSensitive: true,
			wantMatched: false,
		},
		{
			name: "go_goroutines", search: "go_goroutins", fuzzThreshold: 80, caseSensitive: true,
			wantMatched: true,
		},
		{
			name: "go_goroutines", search: "go_goroutins", fuzzThreshold: 100, caseSensitive: true,
			wantMatched: false, // Fuzzy disabled with threshold=100.
		},
		{
			// Substring but not prefix gets a position-based score < 1.0.
			// idx=3, maxIdx=13-9=4 -> 1.0 - 0.9*3/4 = 0.325.
			name: "go_goroutines", search: "goroutine", fuzzThreshold: 100, caseSensitive: true,
			wantMatched: true, wantScore: 0.325,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.search, func(t *testing.T) {
			searches := []string{tt.search}
			if tt.search == "" {
				searches = nil
			}
			filter := buildSearchFilter(searches, tt.fuzzThreshold, "jarowinkler", tt.caseSensitive)

			if tt.search == "" {
				require.Nil(t, filter, "empty searches must yield a nil filter")
				return
			}
			matched, score := filter.Accept(tt.name)

			require.Equal(t, tt.wantMatched, matched)
			if tt.wantMatched && tt.wantScore > 0 {
				require.InDelta(t, tt.wantScore, score, 0.01)
			}
		})
	}
}

// countingTargetRetriever wraps a TargetRetriever and counts TargetsActive
// invocations, used to assert that buildMetricMetadataMap is called once per
// search request rather than once per result.
type countingTargetRetriever struct {
	inner            TargetRetriever
	targetsActiveCnt int
}

func (c *countingTargetRetriever) ScrapePoolConfig(pool string) (*config.ScrapeConfig, error) {
	return c.inner.ScrapePoolConfig(pool)
}

func (c *countingTargetRetriever) TargetsActive() map[string][]*scrape.Target {
	c.targetsActiveCnt++
	return c.inner.TargetsActive()
}

func (c *countingTargetRetriever) TargetsDropped() map[string][]*scrape.Target {
	return c.inner.TargetsDropped()
}

func (c *countingTargetRetriever) TargetsDroppedCounts() map[string]int {
	return c.inner.TargetsDroppedCounts()
}

func TestSearchMetricNamesMetadataMapBuiltOnce(t *testing.T) {
	api := newSearchTestAPI(t)
	counting := &countingTargetRetriever{inner: api.targetRetriever(t.Context())}
	api.targetRetriever = func(context.Context) TargetRetriever { return counting }

	rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
		"include_metadata": []string{"true"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	lines := parseNDJSON(t, rec.Body.String())
	var batch searchBatch[searchMetricNameResult]
	require.NoError(t, json.Unmarshal(lines[0], &batch))
	require.GreaterOrEqual(t, len(batch.Results), 5)

	require.Equal(t, 1, counting.targetsActiveCnt,
		"buildMetricMetadataMap must call TargetsActive exactly once per request")
}

func TestSearchMetricNamesMetadataMapCachedAcrossRequests(t *testing.T) {
	// The TTL cache must serve the second request without re-locking the
	// scrape manager. A regression that drops the cache check would make
	// every keystroke in an autocomplete UI walk every active target.
	api := newSearchTestAPI(t)
	counting := &countingTargetRetriever{inner: api.targetRetriever(t.Context())}
	api.targetRetriever = func(context.Context) TargetRetriever { return counting }

	rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
		"include_metadata": []string{"true"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, counting.targetsActiveCnt,
		"first request must build the metadata map exactly once")

	rec = doSearchRequest(t, api, "/search/metric_names", url.Values{
		"include_metadata": []string{"true"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, counting.targetsActiveCnt,
		"second request within TTL must hit the cache; targetsActiveCnt must not grow")
}

func TestSearchMetricNamesMetadataMapNotBuiltWhenEmpty(t *testing.T) {
	// With no matching results, the lazy metadata builder must not run at
	// all so that the request avoids the scrape-manager lock entirely.
	api := newSearchTestAPI(t)
	counting := &countingTargetRetriever{inner: api.targetRetriever(t.Context())}
	api.targetRetriever = func(context.Context) TargetRetriever { return counting }

	rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
		"include_metadata": []string{"true"},
		"search[]":         []string{"this_metric_definitely_does_not_exist_xyz"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 0, counting.targetsActiveCnt,
		"buildMetricMetadataMap must not be called when no results are emitted")
}

// sequentialSearchQuerier returns a different SearchResultSet for each
// successive call to SearchLabelNames or SearchLabelValues. It is used to test
// the multi-matcher-set merge behaviour where one set fails and the others
// must still surface their results.
type sequentialSearchQuerier struct {
	errorTestQuerier
	calls           []sequentialResp
	labelNameCount  int
	labelValueCount int
}

type sequentialResp struct {
	results []storage.SearchResult
	err     error
}

func (q *sequentialSearchQuerier) nextResp(idx int) sequentialResp {
	if idx >= len(q.calls) {
		return sequentialResp{}
	}
	return q.calls[idx]
}

func (q *sequentialSearchQuerier) SearchLabelNames(context.Context, *storage.SearchHints, ...*labels.Matcher) storage.SearchResultSet {
	r := q.nextResp(q.labelNameCount)
	q.labelNameCount++
	if r.err != nil {
		return storage.ErrSearchResultSet(r.err)
	}
	return storage.NewSearchResultSetFromSlice(r.results, nil)
}

func (q *sequentialSearchQuerier) SearchLabelValues(context.Context, string, *storage.SearchHints, ...*labels.Matcher) storage.SearchResultSet {
	r := q.nextResp(q.labelValueCount)
	q.labelValueCount++
	if r.err != nil {
		return storage.ErrSearchResultSet(r.err)
	}
	return storage.NewSearchResultSetFromSlice(r.results, nil)
}

func TestSearchEndpointsPartialMatcherSetError(t *testing.T) {
	// Two match[] sets: the first yields three values, the second errors.
	// The merge should emit the surviving set's values and the streamer
	// then writes a mid-stream error line.
	q := &sequentialSearchQuerier{
		calls: []sequentialResp{
			{results: []storage.SearchResult{
				{Value: "alpha", Score: 1.0},
				{Value: "beta", Score: 1.0},
				{Value: "gamma", Score: 1.0},
			}},
			{err: errors.New("simulated set failure")},
		},
	}
	queryable := errorTestQueryable{q: q}

	api := minimalSearchAPI()
	api.Queryable = queryable

	rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
		"match[]":    []string{"up", `{job="prometheus"}`},
		"batch_size": []string{"2"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	lines := parseNDJSON(t, rec.Body.String())
	require.NotEmpty(t, lines)

	// Concatenate batch values across all batch lines.
	var values []string
	var lastIsError bool
	for i, line := range lines {
		var trailer searchTrailer
		if err := json.Unmarshal(line, &trailer); err == nil && trailer.Status != "" {
			if i == len(lines)-1 && trailer.Status == "error" {
				lastIsError = true
				continue
			}
		}
		var batch searchBatch[searchMetricNameResult]
		if err := json.Unmarshal(line, &batch); err == nil && batch.Results != nil {
			for _, r := range batch.Results {
				values = append(values, r.Name)
			}
		}
	}
	require.Equal(t, []string{"alpha", "beta", "gamma"}, values, "values from the surviving match[] set must reach the client")
	require.True(t, lastIsError, "stream must terminate with an NDJSON error line, not a success trailer")
}

func TestSearchEndpointsCtxCancelled(t *testing.T) {
	// Use a controlled fake searcher so the test does not depend on the
	// shape of any storage fixture. With three explicit results and
	// batch_size=1, firstBatch is non-empty and the streaming loop must
	// observe the pre-cancelled context on its first iteration and return
	// before writing a success trailer.
	q := &sequentialSearchQuerier{
		calls: []sequentialResp{
			{results: []storage.SearchResult{
				{Value: "alpha", Score: 1.0},
				{Value: "beta", Score: 1.0},
				{Value: "gamma", Score: 1.0},
			}},
		},
	}
	queryable := errorTestQueryable{q: q}

	api := minimalSearchAPI()
	api.Queryable = queryable

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	rec := doSearchRequestCtx(ctx, t, api, "/search/metric_names", url.Values{
		"batch_size": []string{"1"},
	})

	require.Equal(t, http.StatusOK, rec.Code)
	lines := parseNDJSON(t, rec.Body.String())

	// The streaming loop only checks ctx.Done() AFTER writing the first
	// batch, so a successful run always emits at least one line before
	// returning. An empty body would otherwise let a regression slip
	// through this test silently.
	require.NotEmpty(t, lines, "expected at least the first batch line to be written before ctx-cancel takes effect")

	// No line should decode as a success trailer. This is robust to a
	// truncated body, an empty firstBatch path, or an earlier exit before
	// any batch is written.
	for i, line := range lines {
		var trailer searchTrailer
		if err := json.Unmarshal(line, &trailer); err == nil {
			require.NotEqual(t, "success", trailer.Status,
				"line %d is a success trailer but ctx was cancelled before iteration began: %s", i, line)
		}
	}
}

// fixedSearchQuerier returns a caller-provided SearchResultSet for both label
// search methods. It's used to construct controlled failure modes in tests.
type fixedSearchQuerier struct {
	errorTestQuerier
	rs storage.SearchResultSet
}

func (q fixedSearchQuerier) SearchLabelNames(context.Context, *storage.SearchHints, ...*labels.Matcher) storage.SearchResultSet {
	return q.rs
}

func (q fixedSearchQuerier) SearchLabelValues(context.Context, string, *storage.SearchHints, ...*labels.Matcher) storage.SearchResultSet {
	return q.rs
}

// TestSearchStreamFirstBatchError exercises the boundary where the first
// batch that streamSearchResults assembles already contains both partial
// results and a tail error. The handler must stream the partial results
// before terminating with an in-band NDJSON error line.
func TestSearchStreamFirstBatchError(t *testing.T) {
	rs := storage.NewSearchResultSetFromSliceAndError(
		[]storage.SearchResult{
			{Value: "alpha", Score: 1.0},
			{Value: "beta", Score: 1.0},
		},
		nil,
		errors.New("tail failure"),
	)
	queryable := errorTestQueryable{q: fixedSearchQuerier{rs: rs}}

	api := minimalSearchAPI()
	api.Queryable = queryable

	// batch_size=5 ensures both results land in firstBatch; the iterator
	// then surfaces the tail error in the same nextBatch call.
	rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
		"batch_size": []string{"5"},
	})
	require.Equal(t, http.StatusOK, rec.Code, "partial first-batch results must not be lost to a JSON error response")

	lines := parseNDJSON(t, rec.Body.String())
	require.GreaterOrEqual(t, len(lines), 2, "expected a batch line followed by an error line")

	var firstBatch searchBatch[searchMetricNameResult]
	require.NoError(t, json.Unmarshal(lines[0], &firstBatch))
	names := make([]string, 0, len(firstBatch.Results))
	for _, r := range firstBatch.Results {
		names = append(names, r.Name)
	}
	require.Equal(t, []string{"alpha", "beta"}, names)

	var lastTrailer searchTrailer
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &lastTrailer))
	require.Equal(t, "error", lastTrailer.Status, "stream must terminate with an error line, not a success trailer")
}

// TestStreamSearchResultsWarningsSorted verifies that warnings emitted in the
// first NDJSON batch are sorted (so the on-wire order is deterministic) and
// that the trailer's order-independent dedup correctly omits them when the
// warning set is unchanged at iteration end.
func TestStreamSearchResultsWarningsSorted(t *testing.T) {
	var warns annotations.Annotations
	// Add in non-alphabetical order to stress the map-iteration behaviour.
	warns.Add(errors.New("c-warn"))
	warns.Add(errors.New("a-warn"))
	warns.Add(errors.New("b-warn"))

	results := []storage.SearchResult{
		{Value: "alpha", Score: 1.0},
		{Value: "beta", Score: 0.9},
	}
	rs := storage.NewSearchResultSetFromSlice(results, warns)

	api := newSearchTestAPI(t)
	rec := httptest.NewRecorder()
	// httptest.NewRecorder implements http.Flusher; streamSearchResults
	// requires that.
	sp := searchParams{limit: 100, batchSize: 100}

	streamSearchResults(t.Context(), api, rec, rs, sp, func(sr storage.SearchResult) string {
		return sr.Value
	})

	lines := parseNDJSON(t, rec.Body.String())
	require.GreaterOrEqual(t, len(lines), 2, "expected at least a batch and a trailer line")

	var batch searchBatch[string]
	require.NoError(t, json.Unmarshal(lines[0], &batch))
	require.Equal(t, []string{"a-warn", "b-warn", "c-warn"}, batch.Warnings,
		"first batch must carry warnings in sorted order so the on-wire format is deterministic")

	var trailer searchTrailer
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &trailer))
	require.Equal(t, "success", trailer.Status)
	require.Empty(t, trailer.Warnings,
		"trailer must not re-emit warnings already carried by the first batch")
}

// warningsOnExhaustResultSet is a SearchResultSet that returns base warnings
// throughout iteration and additionally surfaces extra warnings once Next
// has returned false. It models a merge tree where a secondary querier's
// error becomes a warning only at exhaustion.
type warningsOnExhaustResultSet struct {
	inner    storage.SearchResultSet
	extra    annotations.Annotations
	finished bool
}

func (s *warningsOnExhaustResultSet) Next() bool {
	ok := s.inner.Next()
	if !ok {
		s.finished = true
	}
	return ok
}

func (s *warningsOnExhaustResultSet) At() storage.SearchResult { return s.inner.At() }
func (s *warningsOnExhaustResultSet) Err() error               { return s.inner.Err() }
func (s *warningsOnExhaustResultSet) Close() error             { return s.inner.Close() }

func (s *warningsOnExhaustResultSet) Warnings() annotations.Annotations {
	base := s.inner.Warnings()
	if !s.finished {
		return base
	}
	// Return a fresh map so the inner's warnings are not mutated.
	merged := annotations.Annotations{}
	merged.Merge(base)
	merged.Merge(s.extra)
	return merged
}

// TestStreamSearchResultsWarningsTrailerDiff covers the "warnings changed
// after iteration" path: warnings present at the first-batch snapshot must
// not duplicate, and warnings surfacing only at exhaustion must reach the
// trailer.
func TestStreamSearchResultsWarningsTrailerDiff(t *testing.T) {
	var base annotations.Annotations
	base.Add(errors.New("base-warn"))
	var extra annotations.Annotations
	extra.Add(errors.New("late-warn"))

	results := []storage.SearchResult{
		{Value: "alpha"},
		{Value: "beta"},
		{Value: "gamma"},
	}
	rs := &warningsOnExhaustResultSet{
		inner: storage.NewSearchResultSetFromSlice(results, base),
		extra: extra,
	}

	api := newSearchTestAPI(t)
	rec := httptest.NewRecorder()
	// batch_size=1 forces iteration past the first batch so the first-batch
	// warnings snapshot is taken before the inner is exhausted.
	sp := searchParams{limit: 100, batchSize: 1}

	streamSearchResults(t.Context(), api, rec, rs, sp, func(sr storage.SearchResult) string {
		return sr.Value
	})

	lines := parseNDJSON(t, rec.Body.String())
	// batch_size=1 with 3 results emits three batch lines, then the trailer.
	require.Len(t, lines, 4, "expected three batch lines plus a trailer")

	var firstBatch searchBatch[string]
	require.NoError(t, json.Unmarshal(lines[0], &firstBatch))
	require.Equal(t, []string{"base-warn"}, firstBatch.Warnings,
		"first batch must see only the warnings present before iteration exhausts")

	var trailer searchTrailer
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &trailer))
	require.Equal(t, "success", trailer.Status)
	require.Equal(t, []string{"base-warn", "late-warn"}, trailer.Warnings,
		"trailer must surface warnings that appeared only after iteration exhausted")
}

// TestSearchParamsRejectsExcessSearchTerms ensures that a request padded
// with more than maxSearchTermsPerRequest search[] params is rejected
// before any filter chain is built — preventing the quadratic per-value
// cost of an unbounded term count.
func TestSearchParamsRejectsExcessSearchTerms(t *testing.T) {
	api := newSearchTestAPI(t)

	tooMany := make([]string, maxSearchTermsPerRequest+1)
	for i := range tooMany {
		tooMany[i] = "t"
	}

	rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
		"search[]": tooMany,
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var response Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, statusError, response.Status)
	require.Equal(t, errorBadData.str, response.ErrorType)
	require.Contains(t, response.Error, "too many search[] terms")

	// At the cap the request must still succeed.
	atCap := make([]string, maxSearchTermsPerRequest)
	for i := range atCap {
		atCap[i] = "t"
	}
	rec = doSearchRequest(t, api, "/search/metric_names", url.Values{
		"search[]": atCap,
	})
	require.Equal(t, http.StatusOK, rec.Code,
		"a request with exactly maxSearchTermsPerRequest terms must be accepted")
}
