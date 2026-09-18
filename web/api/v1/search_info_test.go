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
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/route"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/features"
)

type infoScopeQueryable struct {
	storage.SampleAndChunkQueryable
	mint, maxt int64
}

func (q *infoScopeQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	q.mint, q.maxt = mint, maxt
	return q.SampleAndChunkQueryable.Querier(mint, maxt)
}

func TestSearchInfoScope(t *testing.T) {
	api := minimalSearchAPI()
	q := &infoScopeQueryable{SampleAndChunkQueryable: promqltest.LoadedStorage(t, `
        load 1m
            requests_total{job="api",instance="a",base_label="base"} 0+1x20
            requests_total{job="worker",instance="b"} 0+2x20
            partial{job="api"} 1+0x20
            partial{instance="b"} 1+0x20
            target_info{job="api",instance="a",env="prod",version="v1"} 1+0x20
            target_info{job="worker",instance="b",env="dev",version="v2"} 1+0x20
            target_info{job="api",version="job-only"} 1+0x20
            target_info{instance="b",version="instance-only"} 1+0x20
            build_info{job="api",instance="a",revision="abc"} 1+0x20
    `)}
	api.Queryable = q
	api.QueryEngine = testEngine(t)
	api.now = func() time.Time { return time.Unix(600, 0) }
	api.featureRegistry = features.NewRegistry()
	api.featureRegistry.Enable(features.PromQLFunctions, "info")

	for _, tc := range []struct {
		name, endpoint string
		params         url.Values
		want           []string
		more           bool
	}{
		{name: "names exclude identifiers before limit", endpoint: "label_names", params: url.Values{
			"expr": {`rate(requests_total{job="api"}[5m])`}, "limit": {"1"},
		}, want: []string{"env"}, more: true},
		{name: "names are info labels only", endpoint: "label_names", params: url.Values{
			"expr": {`requests_total{job="api"}`},
		}, want: []string{"env", "version"}},
		{name: "values respect expression", endpoint: "label_values", params: url.Values{
			"expr": {`rate(requests_total{job="api"}[5m])`}, "label": {"version"},
		}, want: []string{"v1"}},
		{name: "completed matcher filters", endpoint: "label_values", params: url.Values{
			"expr": {"requests_total"}, "label": {"version"}, "data_match[]": {`env="dev"`},
		}, want: []string{"v2"}},
		{name: "custom info family", endpoint: "label_names", params: url.Values{
			"expr": {`requests_total{job="api"}`}, "data_match[]": {`__name__="build_info"`},
		}, want: []string{"revision"}},
		{name: "negative-only family", endpoint: "label_names", params: url.Values{
			"expr": {`requests_total{job="api"}`}, "data_match[]": {`__name__!="target_info"`},
		}, want: []string{"revision"}},
		{name: "missing identifying labels stay absent", endpoint: "label_values", params: url.Values{
			"expr": {"partial"}, "label": {"version"},
		}, want: []string{"instance-only", "job-only"}},
		{name: "empty expression result does not broaden scope", endpoint: "label_names", params: url.Values{
			"expr": {`requests_total{job="missing"}`},
		}},
		{name: "selector-free result without identifiers", endpoint: "label_names", params: url.Values{
			"expr": {"vector(1)"},
		}},
		{name: "info input is not enriched", endpoint: "label_names", params: url.Values{
			"expr": {"target_info"},
		}},
		{name: "no expression", endpoint: "label_values", params: url.Values{
			"label": {"version"}, "data_match[]": {`env="prod"`},
		}, want: []string{"v1"}},
		{name: "search and ordering", endpoint: "label_values", params: url.Values{
			"expr": {"requests_total"}, "label": {"version"}, "search[]": {"v"},
			"sort_by": {"alpha"}, "sort_dir": {"dsc"},
		}, want: []string{"v2", "v1"}},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				tc.params.Set("scope", "info")
				router := route.New()
				api.Register(router)
				path := "/search/" + tc.endpoint
				var request *http.Request
				if method == http.MethodPost {
					request = httptest.NewRequest(method, path, strings.NewReader(tc.params.Encode()))
					request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				} else {
					request = httptest.NewRequest(method, path+"?"+tc.params.Encode(), http.NoBody)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				lines := parseNDJSON(t, response.Body.String())
				require.NotEmpty(t, lines)
				var got []string
				for _, line := range lines[:len(lines)-1] {
					var batch struct {
						Results []struct{ Name, Value string }
					}
					require.NoError(t, json.Unmarshal(line, &batch))
					for _, result := range batch.Results {
						if tc.endpoint == "label_names" {
							got = append(got, result.Name)
						} else {
							got = append(got, result.Value)
						}
					}
				}
				require.Equal(t, tc.want, got)
				var trailer searchTrailer
				require.NoError(t, json.Unmarshal(lines[len(lines)-1], &trailer))
				require.Equal(t, "success", trailer.Status)
				require.Equal(t, tc.more, trailer.HasMore)
			})
		}
	}

	t.Run("temporal scope", func(t *testing.T) {
		for _, tc := range []struct {
			expr string
			end  int64
		}{
			{`requests_total offset 1m`, 540000},
			{`requests_total @ 300`, 300000},
			{`requests_total offset 1m or requests_total`, 600000},
			{`last_over_time(requests_total[2m:] offset 1m)`, 540000},
		} {
			response := doSearchRequest(t, api, "/search/label_names", url.Values{
				"scope": {"info"}, "expr": {tc.expr}, "end": {"600"},
				"start": {"999999"}, "lookback_delta": {"2m"},
			})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, tc.end, q.maxt, tc.expr)
			require.Equal(t, tc.end-120000+1, q.mint, tc.expr)
		}
	})

	t.Run("validation", func(t *testing.T) {
		for _, params := range []url.Values{
			{"scope": {"unknown"}},
			{"expr": {"requests_total"}},
			{"scope": {"info"}, "expr": {"1"}},
			{"scope": {"info"}, "expr": {"requests_total[5m]"}},
			{"scope": {"info"}, "match[]": {"target_info"}},
			{"scope": {"info"}, "data_match[]": {`env="prod",version="v1"`}},
			{"scope": {"info"}, "timeout": {"0"}},
		} {
			response := doSearchRequest(t, api, "/search/label_names", params)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		}
		for _, label := range []string{"", "__name__", "job", "instance"} {
			response := doSearchRequest(t, api, "/search/label_values", url.Values{"scope": {"info"}, "label": {label}})
			require.Equal(t, http.StatusBadRequest, response.Code)
		}
		response := doSearchRequest(t, api, "/search/metric_names", url.Values{"scope": {"info"}})
		require.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
		defer cancel()
		response := doSearchRequestCtx(ctx, t, api, "/search/label_names", url.Values{
			"scope": {"info"}, "timeout": {"1s"},
		})
		require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"errorType":"timeout"`)
	})

	t.Run("expression warnings", func(t *testing.T) {
		response := doSearchRequest(t, api, "/search/label_names", url.Values{
			"scope": {"info"}, "expr": {"rate(partial[5m])"},
		})
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "warnings")
	})

	t.Run("feature gate and advertisement", func(t *testing.T) {
		result := api.features(nil)
		require.True(t, result.data.(featuresData).data[features.API]["search_scope_info"])
		api.isAgent = true
		require.False(t, api.features(nil).data.(featuresData).data[features.API]["search_scope_info"])
		api.isAgent = false
		api.enableSearch = false
		require.False(t, api.features(nil).data.(featuresData).data[features.API]["search_scope_info"])
		api.enableSearch = true
		api.featureRegistry.Disable(features.PromQLFunctions, "info")
		result = api.features(nil)
		require.False(t, result.data.(featuresData).data[features.API]["search_scope_info"])
		response := doSearchRequest(t, api, "/search/label_names", url.Values{"scope": {"info"}})
		require.Equal(t, http.StatusInternalServerError, response.Code)
		require.Contains(t, response.Body.String(), `"errorType":"unavailable"`)
		response = doSearchRequest(t, api, "/search/label_names", url.Values{"match[]": {"target_info"}})
		require.Equal(t, http.StatusOK, response.Code)
	})
}
