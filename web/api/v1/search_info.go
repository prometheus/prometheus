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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/features"
	"github.com/prometheus/prometheus/util/httputil"
)

func (r *searchRequest) close() {
	if r.q != nil {
		_ = r.q.Close()
	}
	if r.cancel != nil {
		r.cancel()
	}
}

func (api *API) infoSearchEnabled() bool {
	return api.enableSearch && !api.isAgent && api.featureRegistry != nil &&
		api.featureRegistry.Get()[features.PromQLFunctions]["info"]
}

func (api *API) validateSearchScope(r *http.Request, endpoint string) *apiError {
	switch scope := r.FormValue("scope"); scope {
	case "":
		if _, ok := r.Form["expr"]; ok {
			return &apiError{errorBadData, errors.New("expr requires scope=info")}
		}
		if _, ok := r.Form["data_match[]"]; ok {
			return &apiError{errorBadData, errors.New("data_match[] requires scope=info")}
		}
		return nil
	case "info":
		if endpoint == "metric_names" {
			return &apiError{errorBadData, errors.New("scope=info requires label_names or label_values")}
		}
		if !api.infoSearchEnabled() {
			return &apiError{errorUnavailable, errors.New("scope=info requires promql-experimental-functions")}
		}
		if _, ok := r.Form["match[]"]; ok {
			return &apiError{errorBadData, errors.New("scope=info uses data_match[], not match[]")}
		}
		if endpoint == "label_values" && !isInfoDataLabel(r.FormValue("label")) {
			return &apiError{errorBadData, errors.New("label must be a non-identifying info data label")}
		}
		return nil
	default:
		return &apiError{errorBadData, fmt.Errorf("unknown scope %q", scope)}
	}
}

// prepareInfoScope adds a bounded query phase ahead of ordinary storage search.
// This PoC uses a fixed 30-second ceiling for the complete scoped request.
func (api *API) prepareInfoScope(r *http.Request, sp *searchParams, req *searchRequest) *apiError {
	timeout := 30 * time.Second
	if value := r.FormValue("timeout"); value != "" {
		requested, err := parseDuration(value)
		if err != nil || requested <= 0 {
			return &apiError{errorBadData, errors.New("timeout must be a positive duration")}
		}
		timeout = min(timeout, requested)
	}
	req.ctx, req.cancel = context.WithTimeout(httputil.ContextFromRequest(r.Context(), r), timeout)
	raw := r.Form["data_match[]"]
	if len(raw) > 32 {
		return &apiError{errorBadData, errors.New("at most 32 data_match[] values are supported")}
	}
	var matchers []*labels.Matcher
	for _, value := range raw {
		parsed, err := api.parser.ParseMetricSelector("{" + value + "}")
		if err != nil || len(parsed) != 1 {
			return &apiError{errorBadData, fmt.Errorf("invalid data_match[] %q: expected one full matcher", value)}
		}
		matchers = append(matchers, parsed[0])
	}
	expr := r.FormValue("expr")
	if expr == "" {
		sp.matcherSets = promql.InfoSearchMatchers(nil, matchers)
		return nil
	}
	at, err := parseTimeParam(r, "time", sp.end)
	if err != nil {
		return &apiError{errorBadData, err}
	}
	opts, err := extractQueryOpts(r)
	if err != nil {
		return &apiError{errorBadData, err}
	}
	query, err := api.QueryEngine.NewInstantQuery(req.ctx, api.Queryable, opts, expr, at)
	if err != nil {
		return &apiError{errorBadData, err}
	}
	defer query.Close()
	stmt, ok := query.Statement().(*parser.EvalStmt)
	if !ok || stmt.Expr.Type() != parser.ValueTypeVector {
		return &apiError{errorBadData, errors.New("expr must be an instant vector")}
	}
	hints := promql.InfoSearchHints(stmt)
	result := query.Exec(req.ctx)
	if result.Err != nil {
		return returnAPIError(result.Err)
	}
	vector := result.Value.(promql.Vector)
	// Bound preparation independently of the final search result limit.
	// The deliberately simple PoC bound counts samples, including duplicates.
	if len(vector) > 10000 {
		return &apiError{errorBadData, errors.New("expr scope exceeds 10000 samples; narrow expr")}
	}
	identifyingBytes := 0
	for _, sample := range vector {
		identifyingBytes += len(sample.Metric.Get("job")) + len(sample.Metric.Get("instance"))
		if identifyingBytes > 1<<20 {
			return &apiError{errorBadData, errors.New("expr identifying labels exceed 1 MiB; narrow expr")}
		}
	}
	// Keep an empty query result distinct from the nil, unscoped case.
	if vector == nil {
		vector = promql.Vector{}
	}
	sp.matcherSets = promql.InfoSearchMatchers(vector, matchers)
	req.empty = len(sp.matcherSets) == 0
	req.warnings = result.Warnings
	sp.start, sp.end = timestamp.Time(hints.Start), timestamp.Time(hints.End)
	return nil
}

func isInfoDataLabel(name string) bool {
	return name != "" && name != labels.MetricName && name != "job" && name != "instance"
}

type infoSearchFilter struct {
	storage.Filter
}

func (f infoSearchFilter) Accept(value string) (bool, float64) {
	if !isInfoDataLabel(value) {
		return false, 0
	}
	if f.Filter == nil {
		return true, 1
	}
	return f.Filter.Accept(value)
}

type infoSearchWarnings struct {
	storage.SearchResultSet
	warnings annotations.Annotations
}

func (s infoSearchWarnings) Warnings() annotations.Annotations {
	var warnings annotations.Annotations
	warnings.Merge(s.warnings)
	warnings.Merge(s.SearchResultSet.Warnings())
	return warnings
}
