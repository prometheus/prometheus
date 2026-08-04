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
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/web/api/testhelpers"
)

func BenchmarkQueryScalarArguments(b *testing.B) {
	const (
		seriesCount = 100
		steps       = 1000
	)
	const interval = 10 * time.Second

	stor := teststorage.New(b)
	stor.DisableCompactions()

	ctx := context.Background()
	metrics := make([]labels.Labels, seriesCount)
	refs := make([]storage.SeriesRef, seriesCount)
	for i := range metrics {
		metrics[i] = labels.FromStrings(labels.MetricName, "a_hundred", "l", strconv.Itoa(i))
	}

	// A one-day matrix window plus the subquery's 1000 evaluation steps.
	numIntervals := int(24*time.Hour/interval) + steps
	for step := range numIntervals + 1 {
		app := stor.Appender(ctx)
		for i, metric := range metrics {
			ref, err := app.Append(refs[i], metric, int64(step)*interval.Milliseconds(), float64(step)+float64(i)/seriesCount)
			if err != nil {
				b.Fatal(err)
			}
			refs[i] = ref
		}
		if err := app.Commit(); err != nil {
			b.Fatal(err)
		}
	}
	stor.ForceHeadMMap()
	stor.Compact(ctx)

	api := newTestAPI(b, testhelpers.APIConfig{
		QueryEngine: testhelpers.NewLazyLoader(func() promql.QueryEngine {
			return promqltest.NewTestEngineWithOpts(b, promql.EngineOpts{
				MaxSamples: 50_000_000,
				Timeout:    100 * time.Second,
				Parser: parser.NewParser(parser.Options{
					EnableExperimentalFunctions: true,
				}),
			})
		}),
		Queryable: testhelpers.NewLazyLoader(func() storage.SampleAndChunkQueryable {
			return stor
		}),
	})

	params := make(url.Values)
	params.Set("query", "last_over_time(double_exponential_smoothing(a_hundred[1d], 0.3, 0.3)[10000s:10s])")
	params.Set("time", strconv.FormatInt(int64(numIntervals)*int64(interval/time.Second), 10))
	path := "/api/v1/query?" + params.Encode()

	var recorder *httptest.ResponseRecorder
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		recorder = httptest.NewRecorder()
		api.Handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			b.Fatalf("unexpected status code %d: %s", recorder.Code, recorder.Body.String())
		}
	}

	var response struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string            `json:"resultType"`
			Result     []json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		b.Fatal(err)
	}
	if response.Status != "success" {
		b.Fatalf("unexpected response status %q", response.Status)
	}
	if response.Data.ResultType != "vector" {
		b.Fatalf("unexpected result type %q", response.Data.ResultType)
	}
	if len(response.Data.Result) != seriesCount {
		b.Fatalf("unexpected result series count %d", len(response.Data.Result))
	}
}
