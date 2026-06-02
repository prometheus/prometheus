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
	"slices"
	"strconv"
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

// defaultInfoExtractor is reused across requests; it carries only static config
// (identifying labels, default info metric) and has no per-request state.
//
// Snapshot semantics: infohelper.NewWithDefaults() captures the package vars
// infohelper.DefaultIdentifyingLabels and infohelper.DefaultInfoMetricName at
// init time. The captured slice header points at DefaultIdentifyingLabels'
// backing array, so per-element mutations of that slice would be visible
// here — but a full reassignment of the var (DefaultIdentifyingLabels =
// otherSlice) would not. The package convention is that those vars are not
// reassigned at runtime, so the singleton is safe to share.
var defaultInfoExtractor = infohelper.NewWithDefaults()

// infoLabelsResult is one NDJSON record emitted by /api/v1/info_labels. Score
// is included only when include_score=true.
type infoLabelsResult struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
	Score  *float64 `json:"score,omitempty"`
}

// infoLabels handles GET/POST /api/v1/info_labels. It mirrors the contract of
// the /api/v1/search/* endpoints (feature-gated, NDJSON streaming, search[] +
// fuzzy + sort/score options) and adds the /info_labels-specific parameters
// metric_match (info-metric matcher), expr (PromQL expression evaluated to
// extract identifying-label values), and values_limit (per-label values cap).
func (api *API) infoLabels(w http.ResponseWriter, r *http.Request) {
	// /info_labels is dual-gated: --enable-feature=search-api covers the
	// NDJSON + parsing infrastructure it reuses; --enable-feature=
	// promql-experimental-functions covers info() itself — without info(),
	// the endpoint has no consumer use case. Both flags are checked upfront
	// so the user sees a single error listing every missing flag, instead
	// of having to flip them one at a time.
	if !api.enableSearch || !api.enableExperimentalFunctions {
		httputil.SetCORS(w, api.CORSOrigin, r)
		missing := make([]string, 0, 2)
		if !api.enableSearch {
			missing = append(missing, "search-api")
		}
		if !api.enableExperimentalFunctions {
			missing = append(missing, "promql-experimental-functions")
		}
		api.respondError(w, &apiError{errorUnavailable, fmt.Errorf("info_labels requires --enable-feature=%s", strings.Join(missing, ","))}, nil)
		return
	}

	aReq := api.newAutocompleteRequest(w, r, "info_labels")
	if aReq == nil {
		return
	}
	defer aReq.q.Close()

	// /info_labels scopes its search via metric_match and expr; match[] is not
	// supported because it would mean two equivalent matcher mechanisms with
	// unclear precedence. Reject explicitly so callers can fix the call.
	if len(aReq.sp.matcherSets) > 0 {
		api.respondError(w, &apiError{errorBadData, errors.New("match[] is not supported by info_labels; use metric_match or expr")}, nil)
		return
	}

	infoMetricMatcher, err := parseInfoMetricMatch(r.FormValue("metric_match"), defaultInfoExtractor.DefaultInfoMetric())
	if err != nil {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid metric_match: %w", err)}, nil)
		return
	}

	valuesLimit, apiErr := parseInfoValuesLimit(r)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	ctx := r.Context()

	var (
		identifyingLabelValues map[string]map[string]struct{}
		exprWarnings           []string
	)
	if exprParam := r.FormValue("expr"); exprParam != "" {
		opts, optsErr := extractQueryOpts(r)
		if optsErr != nil {
			api.respondError(w, &apiError{errorBadData, optsErr}, nil)
			return
		}
		ids, warnings, exprErr := api.evaluateExprIdentifyingLabels(ctx, opts, exprParam, aReq.sp.end, defaultInfoExtractor.IdentifyingLabels())
		if exprErr != nil {
			api.respondError(w, exprErr, nil)
			return
		}
		exprWarnings = warnings
		if len(ids) == 0 {
			// No identifying-label values means no info metrics can match.
			// Emit an empty first batch (carrying expr warnings) and a
			// success trailer so clients see a well-formed stream.
			respondEmptyInfoLabelsStream(api, w, warnings)
			return
		}
		identifyingLabelValues = ids
	}

	// SelectHints.Limit is deliberately left unset: we need to traverse
	// every matching info-metric series to discover the universe of data
	// labels, and per-series cost is small (just label iteration).
	//
	// In-memory growth on pathological metric_match values (e.g. =~.*)
	// is bounded instead by the namesLimit passed to ExtractDataLabels
	// below. We source the cap from api.maxSearchLimit, i.e. the operator
	// flag --web.search.max-limit (default 10000). Setting that flag to 0
	// disables the cap and makes the extractor unbounded — same convention
	// as the /api/v1/search/* endpoints.
	selectHints := &storage.SelectHints{
		Start: timestamp.FromTime(aReq.sp.start),
		End:   timestamp.FromTime(aReq.sp.end),
		Func:  "info_labels",
	}

	records, warnings, err := defaultInfoExtractor.ExtractDataLabels(ctx, aReq.q, infoMetricMatcher, identifyingLabelValues, selectHints, aReq.hints.Filter, api.maxSearchLimit, valuesLimit)
	if err != nil {
		api.respondPreStreamSearchError(w, err)
		return
	}

	sortInfoLabelRecords(records, aReq.sp.sortBy, aReq.sp.sortDir)

	hasMore := false
	if aReq.sp.limit > 0 && len(records) > aReq.sp.limit {
		records = records[:aReq.sp.limit]
		hasMore = true
	}

	streamInfoLabelRecords(ctx, api, w, records, hasMore, exprWarnings, annotationsToStrings(warnings), aReq.sp)
}

// parseInfoValuesLimit parses the optional values_limit query parameter, which
// caps the number of values returned per label. 0 (or unset) means no cap.
func parseInfoValuesLimit(r *http.Request) (int, *apiError) {
	v := r.FormValue("values_limit")
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, &apiError{errorBadData, fmt.Errorf("invalid values_limit %q: must be a non-negative integer", v)}
	}
	return n, nil
}

// evaluateExprIdentifyingLabels evaluates a PromQL expression and extracts the
// identifying-label values from the result. Those values are used to restrict
// the info-metric query so callers can ask "what info labels are relevant to
// this expression's series" (e.g. rate(http_requests_total[5m])).
func (api *API) evaluateExprIdentifyingLabels(ctx context.Context, opts promql.QueryOpts, exprParam string, end time.Time, identifyingLabels []string) (map[string]map[string]struct{}, []string, *apiError) {
	qry, err := api.QueryEngine.NewInstantQuery(ctx, api.Queryable, opts, exprParam, end)
	if err != nil {
		return nil, nil, &apiError{errorBadData, fmt.Errorf("invalid expr: %w", err)}
	}
	defer qry.Close()

	res := qry.Exec(ctx)
	warnings := annotationsToStrings(res.Warnings)
	if res.Err != nil {
		return nil, warnings, returnAPIError(res.Err)
	}
	switch res.Value.Type() {
	case parser.ValueTypeVector, parser.ValueTypeMatrix:
		// OK.
	default:
		return nil, warnings, &apiError{errorBadData, fmt.Errorf("expr must return series (vector or matrix), got %s", res.Value.Type())}
	}
	return extractIdentifyingLabels(res.Value, identifyingLabels), warnings, nil
}

// sortInfoLabelRecords sorts the records in-place by the given sort criteria.
// sort_by=score sorts descending by Score with name as tie-breaker.
// Default (sort_by=alpha or unset) sorts ascending by Name; sort_dir=dsc
// reverses to descending. Mirrors sortOrdering for the search endpoints.
func sortInfoLabelRecords(records []infohelper.InfoLabelRecord, sortBy, sortDir string) {
	switch sortBy {
	case "score":
		slices.SortFunc(records, func(a, b infohelper.InfoLabelRecord) int {
			switch {
			case a.Score > b.Score:
				return -1
			case a.Score < b.Score:
				return 1
			default:
				return strings.Compare(a.Name, b.Name)
			}
		})
	default:
		if sortDir == "dsc" {
			slices.SortFunc(records, func(a, b infohelper.InfoLabelRecord) int {
				return strings.Compare(b.Name, a.Name)
			})
		} else {
			slices.SortFunc(records, func(a, b infohelper.InfoLabelRecord) int {
				return strings.Compare(a.Name, b.Name)
			})
		}
	}
}

// streamInfoLabelRecords writes the records as NDJSON batches followed by a
// success trailer. Any write failure during streaming surfaces as an in-band
// searchErrorResponse line — pre-stream errors are already handled by callers.
func streamInfoLabelRecords(ctx context.Context, api *API, w http.ResponseWriter, records []infohelper.InfoLabelRecord, hasMore bool, exprWarnings, extractWarnings []string, sp searchParams) {
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	warnings := mergeWarnings(exprWarnings, extractWarnings)

	includeScore := sp.includeScore
	batchSize := sp.batchSize
	if batchSize <= 0 {
		batchSize = defaultSearchBatchSize
	}

	emitBatch := func(batch []infoLabelsResult, warnings []string) bool {
		if writeErr := nw.writeLine(searchBatch[infoLabelsResult]{Results: batch, Warnings: warnings}); writeErr != nil {
			writeStreamInternalError(nw, writeErr)
			return false
		}
		return true
	}

	// Always emit a first batch line so warnings are observable even when
	// there are no records.
	first := make([]infoLabelsResult, 0, min(batchSize, len(records)))
	for i := 0; i < len(records) && i < batchSize; i++ {
		first = append(first, toInfoLabelsResult(records[i], includeScore))
	}
	if !emitBatch(first, warnings) {
		return
	}

	for offset := batchSize; offset < len(records); offset += batchSize {
		select {
		case <-ctx.Done():
			return
		default:
		}
		end := min(offset+batchSize, len(records))
		batch := make([]infoLabelsResult, 0, end-offset)
		for _, rec := range records[offset:end] {
			batch = append(batch, toInfoLabelsResult(rec, includeScore))
		}
		if !emitBatch(batch, nil) {
			return
		}
	}

	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: hasMore})
}

// respondEmptyInfoLabelsStream emits a well-formed empty NDJSON stream — a
// single empty first batch (carrying any warnings) followed by the success
// trailer. Used when the expr eval produced no identifying-label values.
func respondEmptyInfoLabelsStream(api *API, w http.ResponseWriter, warnings []string) {
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}
	_ = nw.writeLine(searchBatch[infoLabelsResult]{Results: []infoLabelsResult{}, Warnings: warnings})
	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: false})
}

func toInfoLabelsResult(r infohelper.InfoLabelRecord, includeScore bool) infoLabelsResult {
	out := infoLabelsResult{Name: r.Name, Values: r.Values}
	if includeScore {
		score := r.Score
		out.Score = &score
	}
	return out
}

// mergeWarnings returns a deduplicated, sorted slice combining the inputs.
// Stable order makes the NDJSON wire format assertable.
func mergeWarnings(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range b {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

// annotationsToStrings converts annotations.Annotations into a []string for
// use in NDJSON batch / trailer warnings.
func annotationsToStrings(a annotations.Annotations) []string {
	if len(a) == 0 {
		return nil
	}
	errs := a.AsErrors()
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Error())
	}
	return out
}

// parseInfoMetricMatch parses the metric_match query parameter for the
// /info_labels endpoint. Supported formats:
//
//   - "value"      -> MatchEqual
//   - "=~value"    -> MatchRegexp
//   - "!=value"    -> MatchNotEqual
//   - "!~value"    -> MatchNotRegexp
//
// An empty input returns MatchEqual for the configured default metric.
func parseInfoMetricMatch(s, defaultMetric string) (*labels.Matcher, error) {
	if s == "" {
		return labels.NewMatcher(labels.MatchEqual, labels.MetricName, defaultMetric)
	}

	var matchType labels.MatchType
	var value string

	switch {
	case strings.HasPrefix(s, "!~"):
		matchType = labels.MatchNotRegexp
		value = s[2:]
	case strings.HasPrefix(s, "!="):
		matchType = labels.MatchNotEqual
		value = s[2:]
	case strings.HasPrefix(s, "=~"):
		matchType = labels.MatchRegexp
		value = s[2:]
	default:
		matchType = labels.MatchEqual
		value = s
	}

	if value == "" {
		return nil, errors.New("metric_match value cannot be empty")
	}

	return labels.NewMatcher(matchType, labels.MetricName, value)
}

// extractIdentifyingLabels collects the identifying-label values present in a
// PromQL Vector or Matrix result, used to restrict /info_labels' info-metric
// query to series whose identifying labels match the expr eval.
func extractIdentifyingLabels(val parser.Value, identifyingLabels []string) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{})

	collect := func(metric labels.Labels) {
		for _, idLbl := range identifyingLabels {
			v := metric.Get(idLbl)
			if v == "" {
				continue
			}
			if result[idLbl] == nil {
				result[idLbl] = make(map[string]struct{})
			}
			result[idLbl][v] = struct{}{}
		}
	}

	switch v := val.(type) {
	case promql.Vector:
		for _, sample := range v {
			collect(sample.Metric)
		}
	case promql.Matrix:
		for _, series := range v {
			collect(series.Metric)
		}
	}
	return result
}
