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
	"encoding/json"
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

	return &API{
		Queryable:       s,
		targetRetriever: tr.toFactory(),
		now:             time.Now,
		config:          func() config.Config { return samplePrometheusCfg },
		ready:           func(f http.HandlerFunc) http.HandlerFunc { return f },
		parser:          parser.NewParser(parser.Options{}),
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

	r := route.New()
	api.Register(r)

	req := httptest.NewRequest(http.MethodGet, path+"?"+params.Encode(), http.NoBody)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	return rec
}

func TestSearchMetricNames(t *testing.T) {
	api := newSearchTestAPI(t)

	t.Run("basic search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search": []string{"go_gc"},
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
			"search":           []string{"go_gc_duration"},
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

	t.Run("with cardinality", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search":              []string{"up"},
			"include_cardinality": []string{"true"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "up", batch.Results[0].Name)
		require.NotNil(t, batch.Results[0].Cardinality)
		require.Equal(t, 2, *batch.Results[0].Cardinality) // Two "up" series.
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

	t.Run("case insensitive search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search":         []string{"GO_GC"},
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
			"search":         []string{"go_goroutins"},
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
			"search":   []string{"go_gc"},
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

	t.Run("sort_by cardinality auto-enables include_cardinality", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/metric_names", url.Values{
			"search":   []string{"up"},
			"sort_by":  []string{"cardinality"},
			"sort_dir": []string{"dsc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		// Cardinality should be populated even without include_cardinality=true.
		require.NotNil(t, batch.Results[0].Cardinality)
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
		for range 2 {
			var batch searchBatch[searchMetricNameResult]
			require.NoError(t, json.Unmarshal(lines[i], &batch))
			require.Len(t, batch.Results, 2)
		}
		// Third batch should have 1 result (5 total).
		var batch searchBatch[searchMetricNameResult]
		require.NoError(t, json.Unmarshal(lines[2], &batch))
		require.Len(t, batch.Results, 1)
	})
}

func TestSearchLabelNames(t *testing.T) {
	api := newSearchTestAPI(t)

	t.Run("basic search", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{
			"search": []string{"inst"},
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

	t.Run("with frequency", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{
			"search":            []string{"job"},
			"include_frequency": []string{"true"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "job", batch.Results[0].Name)
		require.NotNil(t, batch.Results[0].Frequency)
		require.Equal(t, 6, *batch.Results[0].Frequency) // All 6 series have job.
	})

	t.Run("with cardinality", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_names", url.Values{
			"search":              []string{"job"},
			"include_cardinality": []string{"true"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelNameResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.NotNil(t, batch.Results[0].Cardinality)
		require.Equal(t, 2, *batch.Results[0].Cardinality) // "prometheus" and "node".
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
			"label":  []string{"job"},
			"search": []string{"prom"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.Equal(t, "prometheus", batch.Results[0].Name)
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

	t.Run("with frequency", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":             []string{"job"},
			"search":            []string{"prometheus"},
			"include_frequency": []string{"true"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 1)
		require.NotNil(t, batch.Results[0].Frequency)
		// 5 series have job="prometheus" (all except the node one).
		require.Equal(t, 5, *batch.Results[0].Frequency)
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
		require.Equal(t, "prometheus", batch.Results[0].Name)
		require.Equal(t, "node", batch.Results[1].Name)
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

	t.Run("sort_by frequency auto-enables include_frequency", func(t *testing.T) {
		rec := doSearchRequest(t, api, "/search/label_values", url.Values{
			"label":    []string{"job"},
			"sort_by":  []string{"frequency"},
			"sort_dir": []string{"dsc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		lines := parseNDJSON(t, rec.Body.String())
		var batch searchBatch[searchLabelValueResult]
		require.NoError(t, json.Unmarshal(lines[0], &batch))
		require.Len(t, batch.Results, 2)
		// Frequency should be populated even without include_frequency=true.
		require.NotNil(t, batch.Results[0].Frequency)
		// Sorted descending by frequency: prometheus (5 series) before node (1 series).
		require.Equal(t, "prometheus", batch.Results[0].Name)
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
			name: "go_goroutines", search: "", fuzzThreshold: 100, caseSensitive: true,
			wantMatched: true, wantScore: 1.0,
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
			// Substring but not prefix gets score 0.9.
			name: "go_goroutines", search: "goroutine", fuzzThreshold: 100, caseSensitive: true,
			wantMatched: true, wantScore: 0.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.search, func(t *testing.T) {
			// Build filters to match search logic (OR, not AND).
			var substringFilter *SubstringFilter
			var fuzzyFilter *FuzzyFilter
			if tt.search != "" {
				substringFilter = NewSubstringFilter(tt.search, tt.caseSensitive)
				if tt.fuzzThreshold < 100 {
					threshold := float64(tt.fuzzThreshold) / 100.0
					fuzzyFilter = NewFuzzyFilter(tt.search, threshold, tt.caseSensitive)
				}
			}
			filter := &orFilter{substringFilter: substringFilter, fuzzyFilter: fuzzyFilter}

			matched, score := filter.Accept(tt.name)
			require.Equal(t, tt.wantMatched, matched)
			if tt.wantMatched && tt.wantScore > 0 {
				require.InDelta(t, tt.wantScore, score, 0.01)
			}
		})
	}
}
