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
	"iter"
	"net/http"
	"slices"
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
	if value == labels.MetricName || slices.Contains(infohelper.DefaultIdentifyingLabels, value) {
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

func (s searchResultSetWithWarnings) Warnings() annotations.Annotations {
	var warnings annotations.Annotations
	warnings.Merge(s.warnings)
	warnings.Merge(s.SearchResultSet.Warnings())
	return warnings
}

// newInfoLabelSearchRequest prepares the common search request and rejects
// parameters that would introduce a second, conflicting scoping mechanism or
// the removed combined-response value limit.
func (api *API) newInfoLabelSearchRequest(w http.ResponseWriter, r *http.Request, endpoint string) *preparedAutocompleteRequest {
	if !api.infoLabelFeaturesEnabled(w, r, endpoint) {
		return nil
	}

	req := api.prepareAutocompleteRequest(w, r, endpoint, autocompleteRequestOptions{exprControlsTimeRange: true})
	if req == nil {
		return nil
	}

	if len(req.sp.matcherSets) > 0 {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("match[] is not supported by %s; use metric_match[], data_match[], or expr", endpoint)}, nil)
		return nil
	}
	if _, ok := r.Form["values_limit"]; ok {
		api.respondError(w, &apiError{errorBadData, errors.New("values_limit is not supported; use limit on info_label_values")}, nil)
		return nil
	}
	for _, legacy := range []string{"metric_match", "data_match"} {
		if _, ok := r.Form[legacy]; ok {
			api.respondError(w, &apiError{errorBadData, fmt.Errorf("%s is not supported; use %s[]", legacy, legacy)}, nil)
			return nil
		}
	}
	return req
}

func (api *API) infoLabelFeaturesEnabled(w http.ResponseWriter, r *http.Request, endpoint string) bool {
	if api.enableSearch && api.enableExperimentalFunctions {
		return true
	}

	httputil.SetCORS(w, api.CORSOrigin, r)
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

// infoLabels handles GET/POST /api/v1/info_labels and streams data-label
// names from the scoped info metrics.
func (api *API) infoLabels(w http.ResponseWriter, r *http.Request) {
	req := api.newInfoLabelSearchRequest(w, r, "info_labels")
	if req == nil {
		return
	}

	if _, ok := r.Form["label"]; ok {
		api.respondError(w, &apiError{errorBadData, errors.New("label is not supported by info_labels; use info_label_values for value discovery")}, nil)
		return
	}

	matcherSets, exprWarnings, empty, mint, maxt, apiErr := api.infoMetricMatcherSets(r, req.sp)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	req.hints.Filter = infoDataLabelFilter{filter: req.hints.Filter}
	results := storage.EmptySearchResultSet()
	if !empty {
		searchReq := api.openSearchRequest(w, req, mint, maxt)
		if searchReq == nil {
			return
		}
		defer searchReq.q.Close()
		results = searchLabelNames(r.Context(), searchReq.searcher, matcherSets, req.hints)
	}
	results = searchResultSetWithWarnings{SearchResultSet: results, warnings: exprWarnings}

	streamSearchResults(r.Context(), api, w, results, req.sp, func(sr storage.SearchResult) searchLabelNameResult {
		result := searchLabelNameResult{Name: sr.Value}
		if req.sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		return result
	})
}

// infoLabelValues handles GET/POST /api/v1/info_label_values and streams
// values for one exact data-label name from the scoped info metrics.
func (api *API) infoLabelValues(w http.ResponseWriter, r *http.Request) {
	req := api.newInfoLabelSearchRequest(w, r, "info_label_values")
	if req == nil {
		return
	}

	labelName := r.FormValue("label")
	if labelName == "" {
		api.respondError(w, &apiError{errorBadData, errors.New("missing required parameter \"label\"")}, nil)
		return
	}
	if labelName == labels.MetricName || slices.Contains(infohelper.DefaultIdentifyingLabels, labelName) {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("label %q is not an info data label", labelName)}, nil)
		return
	}

	matcherSets, exprWarnings, empty, mint, maxt, apiErr := api.infoMetricMatcherSets(r, req.sp)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	results := storage.EmptySearchResultSet()
	if !empty {
		searchReq := api.openSearchRequest(w, req, mint, maxt)
		if searchReq == nil {
			return
		}
		defer searchReq.q.Close()
		results = searchLabelValues(r.Context(), searchReq.searcher, labelName, matcherSets, req.hints)
	}
	results = searchResultSetWithWarnings{SearchResultSet: results, warnings: exprWarnings}

	streamSearchResults(r.Context(), api, w, results, req.sp, func(sr storage.SearchResult) searchLabelValueResult {
		result := searchLabelValueResult{Value: sr.Value}
		if req.sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		return result
	})
}

// infoMetricMatcherSets builds the common storage scope for both info-label
// discovery operations. An expression with no identifying-label values is a
// successful empty result and avoids an unscoped storage search.
func (api *API) infoMetricMatcherSets(r *http.Request, sp searchParams) ([][]*labels.Matcher, annotations.Annotations, bool, int64, int64, *apiError) {
	nameMatchers, dataMatchers, err := parseInfoMatchers(api.parser, r.Form)
	if err != nil {
		return nil, nil, false, 0, 0, &apiError{errorBadData, err}
	}
	evalTime, err := parseTimeParam(r, "time", sp.end)
	if err != nil {
		return nil, nil, false, 0, 0, &apiError{errorBadData, err}
	}

	effectiveNameMatchers := infohelper.EffectiveNameMatchers(nameMatchers)
	if exprParam := r.FormValue("expr"); exprParam != "" {
		opts, optsErr := extractQueryOpts(r)
		if optsErr != nil {
			return nil, nil, false, 0, 0, &apiError{errorBadData, optsErr}
		}
		vector, warnings, selectHints, exprErr := api.evaluateExprSeries(r.Context(), opts, exprParam, evalTime)
		if exprErr != nil {
			return nil, warnings, false, 0, 0, exprErr
		}
		eligibleMetrics := iter.Seq[labels.Labels](func(yield func(labels.Labels) bool) {
			for _, sample := range vector {
				if infohelper.MatchesAll(sample.Metric.Get(labels.MetricName), effectiveNameMatchers) {
					continue
				}
				if !yield(sample.Metric) {
					return
				}
			}
		})
		matcherSets, err := infohelper.IdentifyingMatcherSets(eligibleMetrics, infohelper.DefaultIdentifyingLabels, infohelper.MatcherSetLimits{
			MaxValues:      maxInfoIdentifyingValues,
			MaxRegexpBytes: maxInfoIdentifyingRegexpBytes,
		})
		if err != nil {
			return nil, warnings, false, 0, 0, &apiError{errorBadData, fmt.Errorf("expr scope is too broad: %w; narrow expr", err)}
		}
		if len(matcherSets) == 0 {
			return nil, warnings, true, selectHints.Start, selectHints.End, nil
		}
		return appendInfoScopeMatchers(matcherSets, dataMatchers, effectiveNameMatchers), warnings, false, selectHints.Start, selectHints.End, nil
	}

	return appendInfoScopeMatchers([][]*labels.Matcher{{}}, dataMatchers, effectiveNameMatchers), nil, false, timestamp.FromTime(sp.start), timestamp.FromTime(sp.end), nil
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
	selectHints := infohelper.SelectHints(stmt.Expr, timestamp.FromTime(stmt.Start), timestamp.FromTime(stmt.End), stmt.Interval.Milliseconds(), stmt.LookbackDelta)

	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, res.Warnings, storage.SelectHints{}, returnAPIError(res.Err)
	}
	vector, ok := res.Value.(promql.Vector)
	if !ok {
		return nil, res.Warnings, storage.SelectHints{}, &apiError{errorInternal, fmt.Errorf("instant-vector expression returned %s", res.Value.Type())}
	}
	return vector, res.Warnings, selectHints, nil
}

func parseInfoMatchers(p parser.Parser, form map[string][]string) ([]*labels.Matcher, []*labels.Matcher, error) {
	metricValues := form["metric_match[]"]
	dataValues := form["data_match[]"]
	if len(metricValues)+len(dataValues) > maxInfoMatchersPerRequest {
		return nil, nil, fmt.Errorf("too many info matchers: maximum is %d", maxInfoMatchersPerRequest)
	}

	nameMatchers := make([]*labels.Matcher, 0, len(metricValues))
	for _, value := range metricValues {
		matcher, err := parseSingleInfoMatcher(p, value)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid metric_match[] %q: %w", value, err)
		}
		if matcher.Name != labels.MetricName {
			return nil, nil, fmt.Errorf("metric_match[] must match %s, got %s", labels.MetricName, matcher.Name)
		}
		nameMatchers = append(nameMatchers, matcher)
	}

	dataMatchers := make([]*labels.Matcher, 0, len(dataValues))
	for _, value := range dataValues {
		matcher, err := parseSingleInfoMatcher(p, value)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid data_match[] %q: %w", value, err)
		}
		if matcher.Name == labels.MetricName {
			return nil, nil, fmt.Errorf("data_match[] cannot match %s; use metric_match[]", labels.MetricName)
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
