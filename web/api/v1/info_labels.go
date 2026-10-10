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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/infohelper"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/httputil"
)

const (
	maxInfoMatchersPerRequest     = 32
	maxInfoIdentifyingValues      = 10_000
	maxInfoIdentifyingRegexpBytes = 1_048_576
)

// infoDataLabelFilter excludes labels that identify an info series before the
// storage search applies its limit. The wrapped filter retains the shared
// search API's matching and scoring semantics for data labels.
type infoDataLabelFilter struct {
	filter storage.Filter
}

func (f infoDataLabelFilter) Accept(value string) (bool, float64) {
	if value == labels.MetricName || infohelper.IsDefaultIdentifyingLabel(value) {
		return false, 0
	}
	if f.filter == nil {
		return true, 1
	}
	return f.filter.Accept(value)
}

// searchResultSetWithWarnings adds expression-evaluation warnings to the
// warnings produced by storage without changing iteration behavior.
type searchResultSetWithWarnings struct {
	storage.SearchResultSet
	warnings annotations.Annotations
}

type infoLabelSearchRequest struct {
	*preparedSearchRequest
	ctx    context.Context
	cancel context.CancelFunc
}

func (s searchResultSetWithWarnings) Warnings() annotations.Annotations {
	var warnings annotations.Annotations
	warnings.Merge(s.warnings)
	warnings.Merge(s.SearchResultSet.Warnings())
	return warnings
}

// newInfoLabelSearchRequest prepares the common search request and rejects a
// second, conflicting scoping mechanism.
func (api *API) newInfoLabelSearchRequest(w http.ResponseWriter, r *http.Request, endpoint string) *infoLabelSearchRequest {
	if !api.infoLabelFeaturesEnabled(w, r, endpoint) {
		return nil
	}

	req := api.prepareSearchRequest(w, r, endpoint, searchRequestOptions{exprControlsTimeRange: true})
	if req == nil {
		return nil
	}

	if len(req.sp.matcherSets) > 0 {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("match[] is not supported by %s; use data_match[] or expr", endpoint)}, nil)
		return nil
	}
	timeout := api.queryTimeout
	if value := r.FormValue("timeout"); value != "" {
		requested, err := parseDuration(value)
		if err != nil {
			api.respondError(w, invalidParamError(err, "timeout").err, nil)
			return nil
		}
		if requested < timeout {
			timeout = requested
		}
	}
	ctx, cancel := context.WithTimeout(httputil.ContextFromRequest(r.Context(), r), timeout)
	return &infoLabelSearchRequest{preparedSearchRequest: req, ctx: ctx, cancel: cancel}
}

func (api *API) infoLabelFeaturesEnabled(w http.ResponseWriter, r *http.Request, endpoint string) bool {
	if api.enableSearch && api.enableExperimentalFunctions && !api.isAgent {
		return true
	}

	httputil.SetCORS(w, api.CORSOrigin, r)
	if api.isAgent {
		api.respondError(w, &apiError{errorExec, errors.New("unavailable with Prometheus Agent")}, nil)
		return false
	}
	missing := make([]string, 0, 2)
	if !api.enableSearch {
		missing = append(missing, "search-api")
	}
	if !api.enableExperimentalFunctions {
		missing = append(missing, "promql-experimental-functions")
	}
	api.respondError(w, &apiError{errorUnavailable, fmt.Errorf("%s requires --enable-feature=%s", endpoint, strings.Join(missing, ","))}, nil)
	return false
}

// infoLabels streams data-label names from the scoped info metrics.
func (api *API) infoLabels(w http.ResponseWriter, r *http.Request) {
	api.infoLabelSearch(w, r, false)
}

// infoLabelValues streams values for one exact info data-label name.
func (api *API) infoLabelValues(w http.ResponseWriter, r *http.Request) {
	api.infoLabelSearch(w, r, true)
}

func (api *API) infoLabelSearch(w http.ResponseWriter, r *http.Request, values bool) {
	endpoint := "info_labels"
	if values {
		endpoint = "info_label_values"
	}
	req := api.newInfoLabelSearchRequest(w, r, endpoint)
	if req == nil {
		return
	}
	defer req.cancel()

	labelName := r.FormValue("label")
	if values {
		if labelName == "" {
			api.respondError(w, &apiError{errorBadData, errors.New("missing required parameter \"label\"")}, nil)
			return
		}
		if labelName == labels.MetricName || infohelper.IsDefaultIdentifyingLabel(labelName) {
			api.respondError(w, &apiError{errorBadData, fmt.Errorf("label %q is not an info data label", labelName)}, nil)
			return
		}
	} else if _, ok := r.Form["label"]; ok {
		api.respondError(w, &apiError{errorBadData, errors.New("label is not supported by info_labels; use info_label_values for value discovery")}, nil)
		return
	}

	scope, apiErr := api.infoMetricMatcherSets(req.ctx, r, req.sp)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	if !values {
		req.hints.Filter = infoDataLabelFilter{filter: req.hints.Filter}
	}
	results := storage.EmptySearchResultSet()
	// No matcher sets means an empty expression scope, never an unscoped search.
	if len(scope.matcherSets) > 0 {
		searchReq := api.openInfoSearchRequest(req.ctx, w, req.preparedSearchRequest, scope.mint, scope.maxt)
		if searchReq == nil {
			return
		}
		defer searchReq.q.Close()
		if values {
			results = searchLabelValues(req.ctx, searchReq.searcher, labelName, scope.matcherSets, req.hints)
		} else {
			results = searchLabelNames(req.ctx, searchReq.searcher, scope.matcherSets, req.hints)
		}
	}
	results = searchResultSetWithWarnings{SearchResultSet: results, warnings: scope.warnings}

	if values {
		streamSearchResults(req.ctx, api, w, results, req.sp, func(sr storage.SearchResult) searchLabelValueResult {
			result := searchLabelValueResult{Value: sr.Value}
			if req.sp.includeScore {
				score := sr.Score
				result.Score = &score
			}
			return result
		})
	} else {
		streamSearchResults(req.ctx, api, w, results, req.sp, func(sr storage.SearchResult) searchLabelNameResult {
			result := searchLabelNameResult{Name: sr.Value}
			if req.sp.includeScore {
				score := sr.Score
				result.Score = &score
			}
			return result
		})
	}
}

type infoLabelScope struct {
	matcherSets [][]*labels.Matcher
	warnings    annotations.Annotations
	mint, maxt  int64
}

// infoMetricMatcherSets builds the common storage scope for both info-label
// discovery operations. An expression with no identifying-label values is a
// successful empty result and avoids an unscoped storage search.
func (api *API) infoMetricMatcherSets(ctx context.Context, r *http.Request, sp searchParams) (infoLabelScope, *apiError) {
	nameMatchers, dataMatchers, err := parseInfoMatchers(api.parser, r.Form)
	if err != nil {
		return infoLabelScope{}, &apiError{errorBadData, err}
	}
	evalTime, err := parseTimeParam(r, "time", sp.end)
	if err != nil {
		return infoLabelScope{}, &apiError{errorBadData, err}
	}

	effectiveNameMatchers := infohelper.EffectiveNameMatchers(nameMatchers)
	if exprParam := r.FormValue("expr"); exprParam != "" {
		opts, optsErr := extractQueryOpts(r)
		if optsErr != nil {
			return infoLabelScope{}, &apiError{errorBadData, optsErr}
		}
		vector, warnings, selectHints, exprErr := api.evaluateExprSeries(ctx, opts, exprParam, evalTime)
		if exprErr != nil {
			return infoLabelScope{warnings: warnings}, exprErr
		}
		matcherSetBuilder := infohelper.NewDefaultIdentifyingMatcherSetBuilder(infohelper.MatcherSetLimits{
			MaxValues:      maxInfoIdentifyingValues,
			MaxRegexpBytes: maxInfoIdentifyingRegexpBytes,
		})
		for _, sample := range vector {
			if infohelper.MatchesAll(sample.Metric.Get(labels.MetricName), effectiveNameMatchers) {
				continue
			}
			if err := matcherSetBuilder.Add(sample.Metric); err != nil {
				return infoLabelScope{warnings: warnings}, &apiError{errorBadData, fmt.Errorf("expr scope is too broad: %w; narrow expr", err)}
			}
		}
		matcherSets := matcherSetBuilder.MatcherSets()
		if len(matcherSets) == 0 {
			return infoLabelScope{warnings: warnings, mint: selectHints.Start, maxt: selectHints.End}, nil
		}
		return infoLabelScope{matcherSets: appendInfoScopeMatchers(matcherSets, dataMatchers, effectiveNameMatchers), warnings: warnings, mint: selectHints.Start, maxt: selectHints.End}, nil
	}

	return infoLabelScope{matcherSets: appendInfoScopeMatchers([][]*labels.Matcher{{}}, dataMatchers, effectiveNameMatchers), mint: timestamp.FromTime(sp.start), maxt: timestamp.FromTime(sp.end)}, nil
}

func (api *API) openInfoSearchRequest(ctx context.Context, w http.ResponseWriter, prepared *preparedSearchRequest, mint, maxt int64) *searchRequest {
	if api.infoRequestContextDone(ctx, w) {
		return nil
	}

	q, err := api.Queryable.Querier(mint, maxt)
	if api.infoRequestContextDone(ctx, w) {
		if q != nil {
			_ = q.Close()
		}
		return nil
	}
	if err != nil {
		api.respondPreStreamSearchError(w, err)
		return nil
	}
	return api.searchRequestFromQuerier(w, prepared, q)
}

func (api *API) infoRequestContextDone(ctx context.Context, w http.ResponseWriter) bool {
	cause := context.Cause(ctx)
	if cause == nil {
		return false
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		api.respondPreStreamSearchError(w, context.DeadlineExceeded)
	}
	return true
}

func appendInfoScopeMatchers(matcherSets [][]*labels.Matcher, dataMatchers, nameMatchers []*labels.Matcher) [][]*labels.Matcher {
	for i, identifyingMatchers := range matcherSets {
		matchers := make([]*labels.Matcher, 0, len(identifyingMatchers)+len(dataMatchers)+len(nameMatchers))
		matchers = append(matchers, identifyingMatchers...)
		matchers = append(matchers, dataMatchers...)
		matchers = append(matchers, nameMatchers...)
		matcherSets[i] = matchers
	}
	return matcherSets
}

// evaluateExprSeries evaluates an instant-vector expression and returns the
// storage range that the corresponding info() call would use.
func (api *API) evaluateExprSeries(ctx context.Context, opts promql.QueryOpts, exprParam string, end time.Time) (promql.Vector, annotations.Annotations, storage.SelectHints, *apiError) {
	qry, err := api.QueryEngine.NewInstantQuery(ctx, api.Queryable, opts, exprParam, end)
	if err != nil {
		return nil, nil, storage.SelectHints{}, &apiError{errorBadData, fmt.Errorf("invalid expr: %w", err)}
	}
	defer qry.Close()

	stmt, ok := qry.Statement().(*parser.EvalStmt)
	if !ok {
		return nil, nil, storage.SelectHints{}, &apiError{errorInternal, errors.New("instant query returned an unexpected statement type")}
	}
	if stmt.Expr.Type() != parser.ValueTypeVector {
		return nil, nil, storage.SelectHints{}, &apiError{errorBadData, fmt.Errorf("expr must be an instant vector, got %s", parser.DocumentedType(stmt.Expr.Type()))}
	}
	selectPlan := infohelper.BuildSelectPlan(stmt.Expr, timestamp.FromTime(stmt.Start), timestamp.FromTime(stmt.End), stmt.Interval.Milliseconds(), stmt.LookbackDelta)

	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, res.Warnings, storage.SelectHints{}, returnAPIError(res.Err)
	}
	vector, ok := res.Value.(promql.Vector)
	if !ok {
		return nil, res.Warnings, storage.SelectHints{}, &apiError{errorInternal, fmt.Errorf("instant-vector expression returned %s", res.Value.Type())}
	}
	return vector, res.Warnings, selectPlan.Hints, nil
}

func parseInfoMatchers(p parser.Parser, form map[string][]string) ([]*labels.Matcher, []*labels.Matcher, error) {
	values := form["data_match[]"]
	if len(values) > maxInfoMatchersPerRequest {
		return nil, nil, fmt.Errorf("too many info matchers: maximum is %d", maxInfoMatchersPerRequest)
	}

	nameMatchers := make([]*labels.Matcher, 0, len(values))
	dataMatchers := make([]*labels.Matcher, 0, len(values))
	for _, value := range values {
		matcher, err := parseSingleInfoMatcher(p, value)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid data_match[] %q: %w", value, err)
		}
		if matcher.Name == labels.MetricName {
			nameMatchers = append(nameMatchers, matcher)
			continue
		}
		dataMatchers = append(dataMatchers, matcher)
	}
	return nameMatchers, dataMatchers, nil
}

func parseSingleInfoMatcher(p parser.Parser, value string) (*labels.Matcher, error) {
	matchers, err := p.ParseMetricSelector("{" + value + "}")
	if err != nil {
		return nil, err
	}
	if len(matchers) != 1 {
		return nil, fmt.Errorf("expected exactly one matcher, got %d", len(matchers))
	}
	return matchers[0], nil
}
