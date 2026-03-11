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
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/web/api/testhelpers"
)

// TODO: Generate automated tests from OpenAPI spec to validate API responses.

// TestAPIEmpty tests the API with no metrics and no rules.
func TestAPIEmpty(t *testing.T) {
	// Create an API with empty defaults (no series, no rules).
	api := newTestAPI(t, testhelpers.APIConfig{})

	t.Run("GET /api/v1/labels returns success with empty array", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/labels").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONArray("$.data")
	})

	t.Run("GET /api/v1/query?query=up returns success (empty result ok)", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query", "query", "up").
			ValidateOpenAPI().
			RequireSuccess().
			RequireEquals("$.data.resultType", "vector")
	})

	t.Run("GET /api/v1/query_range?query=up returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query_range",
			"query", "up",
			"start", "0",
			"end", "100",
			"step", "10").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "matrix")
	})

	t.Run("GET /api/v1/series returns success with empty result", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/series",
			"match[]", "up",
			"start", "0",
			"end", "100").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONArray("$.data")
	})

	t.Run("GET /api/v1/label/__name__/values returns success with empty array", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/label/__name__/values").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONArray("$.data")
	})

	t.Run("GET /api/v1/targets returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/targets").
			RequireSuccess().
			RequireJSONPathExists("$.data.activeTargets")
	})

	t.Run("GET /api/v1/rules returns success with empty groups", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/rules").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data.groups")
	})

	t.Run("GET /api/v1/alerts returns success with empty alerts", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/alerts").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data.alerts")
	})

	t.Run("GET /api/v1/alertmanagers returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/alertmanagers").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data.activeAlertmanagers")
	})

	t.Run("GET /api/v1/metadata returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/metadata").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data")
	})

	t.Run("GET /api/v1/status/config returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/status/config").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data.yaml")
	})

	t.Run("GET /api/v1/status/flags returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/status/flags").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data")
	})

	t.Run("GET /api/v1/status/runtimeinfo returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/status/runtimeinfo").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data")
	})

	t.Run("GET /api/v1/status/buildinfo returns success", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/status/buildinfo").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data")
	})

	t.Run("POST /api/v1/query with form data returns success", func(t *testing.T) {
		testhelpers.POST(t, api, "/api/v1/query", "query", "up").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector")
	})
}

// TestAPIWithSeries tests the API with metrics/series data.
func TestAPIWithSeries(t *testing.T) {
	// Create an API with sample series data.
	api := newTestAPI(t, testhelpers.APIConfig{
		Queryable: testhelpers.NewLazyLoader(func() storage.SampleAndChunkQueryable {
			return testhelpers.NewQueryableWithSeries(testhelpers.FixtureMultipleSeries())
		}),
	})

	t.Run("GET /api/v1/query returns vector with >= 1 sample", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query", "query", "up").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector").
			RequireLenAtLeast("$.data.result", 1)
	})

	t.Run("GET /api/v1/query_range returns matrix result type", func(t *testing.T) {
		// Use relative timestamps to match our fixtures.
		now := time.Now().Unix()
		testhelpers.GET(t, api, "/api/v1/query_range",
			"query", "up",
			"start", strconv.FormatInt(now-120, 10),
			"end", strconv.FormatInt(now, 10),
			"step", "60").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "matrix")
		// Note: Result may be empty if timestamps don't align perfectly with samples.
	})

	t.Run("GET /api/v1/labels returns non-empty array", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/labels").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONArray("$.data").
			RequireLenAtLeast("$.data", 1)
	})

	t.Run("GET /api/v1/label/__name__/values contains expected metric names", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/label/__name__/values").
			RequireSuccess().
			ValidateOpenAPI().
			RequireArrayContains("$.data", "up").
			RequireArrayContains("$.data", "http_requests_total")
	})

	t.Run("GET /api/v1/label/job/values contains expected jobs", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/label/job/values").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONArray("$.data").
			RequireArrayContains("$.data", "prometheus").
			RequireArrayContains("$.data", "node").
			RequireArrayContains("$.data", "api")
	})

	t.Run("GET /api/v1/series with match returns results", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/series",
			"match[]", "up",
			"start", "0",
			"end", "120").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONArray("$.data").
			RequireLenAtLeast("$.data", 1)
	})

	t.Run("GET /api/v1/query with specific job returns filtered results", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query", "query", `up{job="prometheus"}`).
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector").
			RequireLenAtLeast("$.data.result", 1)
	})

	t.Run("GET /api/v1/query with aggregation returns result", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query", "query", "sum(up)").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector")
	})

	t.Run("POST /api/v1/query returns vector with data", func(t *testing.T) {
		testhelpers.POST(t, api, "/api/v1/query", "query", "up").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector").
			RequireLenAtLeast("$.data.result", 1)
	})
}

// TestAPIWithRules tests the API with rules configured.
func TestAPIWithRules(t *testing.T) {
	// Create an API with rule groups.
	api := newTestAPI(t, testhelpers.APIConfig{
		RulesRetriever: testhelpers.NewLazyLoader(func() testhelpers.RulesRetriever {
			return testhelpers.NewRulesRetrieverWithGroups(testhelpers.FixtureRuleGroups())
		}),
	})

	t.Run("GET /api/v1/rules returns groups with rules", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/rules").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data.groups").
			RequireLenAtLeast("$.data.groups", 1).
			RequireSome("$.data.groups", func(group any) bool {
				if g, ok := group.(map[string]any); ok {
					return g["name"] == "example"
				}
				return false
			}).
			RequireSome("$.data.groups", func(group any) bool {
				if g, ok := group.(map[string]any); ok {
					if g["name"] == "example" {
						// Check that the group has rules.
						if rules, ok := g["rules"].([]any); ok {
							return len(rules) > 0
						}
					}
				}
				return false
			})
	})

	t.Run("GET /api/v1/alerts returns alerts array", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/alerts").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data.alerts").
			RequireJSONArray("$.data.alerts")
	})

	t.Run("GET /api/v1/rules with rule_name filter", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/rules", "rule_name[]", "InstanceDown").
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONPathExists("$.data.groups")
	})
}

// TestAPITSDBNotReady tests the API when TSDB is not ready (e.g., during WAL replay).
// TSDB not ready errors are converted to errorUnavailable by setUnavailStatusOnTSDBNotReady,
// which returns HTTP 500 Internal Server Error (the default for errorUnavailable).
func TestAPITSDBNotReady(t *testing.T) {
	// Create an API with a queryable that returns tsdb.ErrNotReady.
	api := newTestAPI(t, testhelpers.APIConfig{
		Queryable: testhelpers.NewLazyLoader(testhelpers.NewTSDBNotReadyQueryable),
	})

	t.Run("GET /api/v1/query returns 500 when TSDB not ready", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query", "query", "up").
			RequireStatusCode(500).
			ValidateOpenAPI().
			RequireError()
	})

	t.Run("POST /api/v1/query returns 500 when TSDB not ready", func(t *testing.T) {
		testhelpers.POST(t, api, "/api/v1/query", "query", "up").
			RequireStatusCode(500).
			ValidateOpenAPI().
			RequireError()
	})

	t.Run("GET /api/v1/query_range returns 500 when TSDB not ready", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query_range",
			"query", "up",
			"start", "0",
			"end", "100",
			"step", "10").
			RequireStatusCode(500).
			ValidateOpenAPI().
			RequireError()
	})

	t.Run("GET /api/v1/series returns 500 when TSDB not ready", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/series",
			"match[]", "up",
			"start", "0",
			"end", "100").
			RequireStatusCode(500).
			ValidateOpenAPI().
			RequireError()
	})

	t.Run("GET /api/v1/labels returns 500 when TSDB not ready", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/labels").
			RequireStatusCode(500).
			ValidateOpenAPI().
			RequireError()
	})

	t.Run("GET /api/v1/label/{name}/values returns 500 when TSDB not ready", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/label/__name__/values").
			RequireStatusCode(500).
			ValidateOpenAPI().
			RequireError()
	})
}

// TestAPIWithNativeHistograms tests the API with native histogram data.
func TestAPIWithNativeHistograms(t *testing.T) {
	// Create an API with histogram series data.
	api := newTestAPI(t, testhelpers.APIConfig{
		Queryable: testhelpers.NewLazyLoader(func() storage.SampleAndChunkQueryable {
			return testhelpers.NewQueryableWithSeries(testhelpers.FixtureHistogramSeries())
		}),
	})

	t.Run("GET /api/v1/query returns vector with native histogram", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query", "query", "test_histogram").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector").
			RequireLenAtLeast("$.data.result", 1).
			RequireSome("$.data.result", func(item any) bool {
				sample, ok := item.(map[string]any)
				if !ok {
					return false
				}
				// Check that the sample has a histogram field (not a value field).
				_, hasHistogram := sample["histogram"]
				return hasHistogram
			})
	})

	t.Run("POST /api/v1/query returns vector with native histogram", func(t *testing.T) {
		testhelpers.POST(t, api, "/api/v1/query", "query", "test_histogram").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector").
			RequireLenAtLeast("$.data.result", 1).
			RequireSome("$.data.result", func(item any) bool {
				sample, ok := item.(map[string]any)
				if !ok {
					return false
				}
				// Check that the sample has a histogram field (not a value field).
				_, hasHistogram := sample["histogram"]
				return hasHistogram
			})
	})

	t.Run("GET /api/v1/query_range returns matrix with native histogram", func(t *testing.T) {
		// Use relative timestamps to match our fixtures.
		now := time.Now().Unix()
		testhelpers.GET(t, api, "/api/v1/query_range",
			"query", "test_histogram",
			"start", strconv.FormatInt(now-120, 10),
			"end", strconv.FormatInt(now, 10),
			"step", "60").
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "matrix")
	})

	t.Run("GET /api/v1/query with histogram selector", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/query", "query", `test_histogram{job="prometheus"}`).
			RequireSuccess().
			ValidateOpenAPI().
			RequireEquals("$.data.resultType", "vector").
			RequireLenAtLeast("$.data.result", 1)
	})

	t.Run("GET /api/v1/series returns histogram metric series", func(t *testing.T) {
		testhelpers.GET(t, api, "/api/v1/series",
			"match[]", "test_histogram",
			"start", "0",
			"end", strconv.FormatInt(time.Now().Unix(), 10)).
			RequireSuccess().
			ValidateOpenAPI().
			RequireJSONArray("$.data").
			RequireLenAtLeast("$.data", 1)
	})
}

// TestAPIWithStats tests the API with the stats query parameter.
func TestAPIWithStats(t *testing.T) {
	// Create an API with sample series data.
	api := newTestAPI(t, testhelpers.APIConfig{
		Queryable: testhelpers.NewLazyLoader(func() storage.SampleAndChunkQueryable {
			return testhelpers.NewQueryableWithSeries(testhelpers.FixtureMultipleSeries())
		}),
	})

	now := time.Now().Unix()

	// Test combinations of methods, endpoints, and stats values.
	methods := []string{"GET", "POST"}
	statsValues := []struct {
		value       string
		expectStats bool
	}{
		{"true", true},
		{"all", true},
		{"1", true},
		{"", false},
	}

	for _, method := range methods {
		for _, stats := range statsValues {
			t.Run(method+" /api/v1/query with stats="+stats.value, func(t *testing.T) {
				var params []string
				if stats.value != "" {
					params = []string{"query", "up", "stats", stats.value}
				} else {
					params = []string{"query", "up"}
				}

				var resp *testhelpers.Response
				if method == "GET" {
					resp = testhelpers.GET(t, api, "/api/v1/query", params...)
				} else {
					resp = testhelpers.POST(t, api, "/api/v1/query", params...)
				}

				resp.RequireSuccess().ValidateOpenAPI()

				if stats.expectStats {
					resp.RequireJSONPathExists("$.data.stats").
						RequireJSONPathExists("$.data.stats.timings").
						RequireJSONPathExists("$.data.stats.samples")
				} else {
					resp.RequireJSONPathNotExists("$.data.stats")
				}
			})

			t.Run(method+" /api/v1/query_range with stats="+stats.value, func(t *testing.T) {
				var params []string
				if stats.value != "" {
					params = []string{
						"query", "up",
						"start", strconv.FormatInt(now-120, 10),
						"end", strconv.FormatInt(now, 10),
						"step", "60",
						"stats", stats.value,
					}
				} else {
					params = []string{
						"query", "up",
						"start", strconv.FormatInt(now-120, 10),
						"end", strconv.FormatInt(now, 10),
						"step", "60",
					}
				}

				var resp *testhelpers.Response
				if method == "GET" {
					resp = testhelpers.GET(t, api, "/api/v1/query_range", params...)
				} else {
					resp = testhelpers.POST(t, api, "/api/v1/query_range", params...)
				}

				resp.RequireSuccess().ValidateOpenAPI()

				if stats.expectStats {
					resp.RequireJSONPathExists("$.data.stats").
						RequireJSONPathExists("$.data.stats.timings").
						RequireJSONPathExists("$.data.stats.samples")
				} else {
					resp.RequireJSONPathNotExists("$.data.stats")
				}
			})
		}
	}
}
