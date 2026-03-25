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
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/httputil"
)

// searchParams holds the common parsed parameters for all search endpoints.
type searchParams struct {
	matcherSets   [][]*labels.Matcher
	searches      []string
	fuzzThreshold int    // 0-100, default 0 (no fuzz matching).
	fuzzAlg       string // "jarowinkler" (default) or "subsequence".
	caseSensitive bool   // Default true.
	sortBy        string
	sortDir       string // "asc" (default) or "dsc".
	includeScore  bool   // Include relevance score in each result record.
	start, end    time.Time
	limit         int // Default 100.
	batchSize     int // Default 100.
}

// searchMetricNameResult is a single result record for the metric_names endpoint.
type searchMetricNameResult struct {
	Name  string   `json:"name"`
	Score *float64 `json:"score,omitempty"`
	Type  string   `json:"type,omitempty"`
	Help  string   `json:"help,omitempty"`
	Unit  string   `json:"unit,omitempty"`
}

// searchLabelNameResult is a single result record for the label_names endpoint.
type searchLabelNameResult struct {
	Name  string   `json:"name"`
	Score *float64 `json:"score,omitempty"`
}

// searchLabelValueResult is a single result record for the label_values endpoint.
type searchLabelValueResult struct {
	Name  string   `json:"name"`
	Score *float64 `json:"score,omitempty"`
}

// searchBatch is a single NDJSON batch line containing results and optional warnings.
type searchBatch[T any] struct {
	Results  []T      `json:"results"`
	Warnings []string `json:"warnings,omitempty"`
}

// searchTrailer is the final NDJSON line indicating completion status.
type searchTrailer struct {
	Status   string   `json:"status"`
	HasMore  bool     `json:"has_more"`
	Warnings []string `json:"warnings,omitempty"`
}

// searchErrorResponse is an NDJSON error line.
type searchErrorResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

// ndjsonWriter writes newline-delimited JSON lines with flushing.
type ndjsonWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	json    jsoniter.API
}

// newNDJSONWriter creates a new NDJSON writer, setting the appropriate content type.
func newNDJSONWriter(w http.ResponseWriter) (*ndjsonWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("response writer does not support flushing")
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	return &ndjsonWriter{
		w:       w,
		flusher: flusher,
		json:    jsoniter.ConfigCompatibleWithStandardLibrary,
	}, nil
}

// writeLine marshals v as JSON, writes it as a single line, and flushes.
func (nw *ndjsonWriter) writeLine(v any) error {
	b, err := nw.json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = nw.w.Write(b)
	if err != nil {
		return err
	}
	nw.flusher.Flush()
	return nil
}

// parseSearchParams parses the common query parameters for search endpoints.
func (api *API) parseSearchParams(r *http.Request) (searchParams, *apiError) {
	var sp searchParams

	now := api.now()
	start, err := parseTimeParam(r, "start", now.Add(-time.Hour))
	if err != nil {
		return sp, &apiError{errorBadData, err}
	}
	sp.start = start

	end, err := parseTimeParam(r, "end", now)
	if err != nil {
		return sp, &apiError{errorBadData, err}
	}
	sp.end = end

	if matchers := r.Form["match[]"]; len(matchers) > 0 {
		matcherSets, err := api.parseMatchersParam(matchers)
		if err != nil {
			return sp, &apiError{errorBadData, err}
		}
		sp.matcherSets = matcherSets
	}

	sp.searches = r.Form["search[]"]

	sp.fuzzThreshold = 0
	if v := r.FormValue("fuzz_threshold"); v != "" {
		ft, err := strconv.Atoi(v)
		if err != nil || ft < 0 || ft > 100 {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid fuzz_threshold %q: must be 0-100", v)}
		}
		sp.fuzzThreshold = ft
	}

	// Validate fuzz_alg if provided; "jarowinkler" (default) and "subsequence" are supported.
	sp.fuzzAlg = "jarowinkler"
	if v := r.FormValue("fuzz_alg"); v != "" {
		if v != "subsequence" && v != "jarowinkler" {
			return sp, &apiError{errorBadData, fmt.Errorf("unsupported fuzz_alg %q: must be \"jarowinkler\" or \"subsequence\"", v)}
		}
		sp.fuzzAlg = v
	}

	sp.caseSensitive = true
	if v := r.FormValue("case_sensitive"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid case_sensitive %q: must be boolean", v)}
		}
		sp.caseSensitive = b
	}

	if v := r.FormValue("include_score"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid include_score %q: must be boolean", v)}
		}
		sp.includeScore = b
	}

	sp.sortBy = r.FormValue("sort_by")
	sp.sortDir = r.FormValue("sort_dir")
	if sp.sortDir != "" && sp.sortBy == "" {
		return sp, &apiError{errorBadData, errors.New("sort_dir is only valid when sort_by is set")}
	}
	if sp.sortDir == "" {
		sp.sortDir = "asc"
	}
	if sp.sortDir != "asc" && sp.sortDir != "dsc" {
		return sp, &apiError{errorBadData, fmt.Errorf("invalid sort_dir %q: must be \"asc\" or \"dsc\"", sp.sortDir)}
	}
	if sp.sortBy == "score" && len(sp.searches) == 0 {
		return sp, &apiError{errorBadData, errors.New("sort_by=score requires search[] to be set")}
	}

	sp.limit = 100
	if v := r.FormValue("limit"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil || l < 0 {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid limit %q: must be non-negative integer", v)}
		}
		if l > 0 {
			sp.limit = l
		}
	}

	sp.batchSize = 100
	if v := r.FormValue("batch_size"); v != "" {
		bs, err := strconv.Atoi(v)
		if err != nil || bs < 0 {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid batch_size %q: must be non-negative integer", v)}
		}
		if bs > 0 {
			sp.batchSize = bs
		}
		// batch_size=0 means server-determined; keep default.
	}

	return sp, nil
}

// getMetricMetadata retrieves type, help, and unit metadata for a metric name.
func (api *API) getMetricMetadata(ctx context.Context, metricName string) (typ, help, unit string, ok bool) {
	tr := api.targetRetriever(ctx)
	if tr == nil {
		return "", "", "", false
	}
	for _, targets := range tr.TargetsActive() {
		for _, t := range targets {
			if md, found := t.GetMetadata(metricName); found {
				return string(md.Type), md.Help, md.Unit, true
			}
		}
	}
	return "", "", "", false
}

// scoreDescComparator sorts search results by score descending,
// with alphabetical tie-breaking.
type scoreDescComparator struct{}

// Compare implements storage.Comparator.
func (scoreDescComparator) Compare(a, b storage.SearchResult) int {
	if c := cmp.Compare(b.Score, a.Score); c != 0 {
		return c // Descending by score.
	}
	return cmp.Compare(a.Value, b.Value) // Ascending alphabetical tie-breaker.
}

// alphaComparator sorts search results alphabetically by value.
type alphaComparator struct{ desc bool }

// Compare implements storage.Comparator.
func (c alphaComparator) Compare(a, b storage.SearchResult) int {
	if c.desc {
		return cmp.Compare(b.Value, a.Value)
	}
	return cmp.Compare(a.Value, b.Value)
}

// streamBatches writes results in batch_size chunks as NDJSON lines.
// Warnings are attached to the first batch only.
func streamBatches[T any](nw *ndjsonWriter, results []T, batchSize int, warnings []string) error {
	for i := 0; i < len(results); i += batchSize {
		end := min(i+batchSize, len(results))
		batch := searchBatch[T]{Results: results[i:end]}
		if i == 0 {
			batch.Warnings = warnings
		}
		if err := nw.writeLine(batch); err != nil {
			return err
		}
	}
	// If there are no results, still emit an empty batch to carry warnings.
	if len(results) == 0 {
		batch := searchBatch[T]{Results: []T{}, Warnings: warnings}
		if err := nw.writeLine(batch); err != nil {
			return err
		}
	}
	return nil
}

// drainSearchResultSet consumes a SearchResultSet into a slice, collecting warnings.
// Closes the set when done.
func drainSearchResultSet(rs storage.SearchResultSet) ([]storage.SearchResult, []string, error) {
	var results []storage.SearchResult
	var warnings []string
	for rs.Next() {
		results = append(results, rs.At())
	}
	for _, w := range rs.Warnings() {
		warnings = append(warnings, w.Error())
	}
	err := rs.Err()
	_ = rs.Close()
	return results, warnings, err
}

// searchLabelValues retrieves label values using the Searcher interface.
// For multiple matcher sets it unions results from each, taking the max score per value.
func searchLabelValues(ctx context.Context, searcher storage.Searcher, name string, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) ([]storage.SearchResult, []string, error) {
	if len(matcherSets) > 1 {
		valuesMap := make(map[string]float64)
		var warnings []string
		for _, matchers := range matcherSets {
			results, w, err := drainSearchResultSet(searcher.SearchLabelValues(ctx, name, hints, matchers...))
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, w...)
			for _, r := range results {
				if existing, ok := valuesMap[r.Value]; !ok || r.Score > existing {
					valuesMap[r.Value] = r.Score
				}
			}
		}
		merged := make([]storage.SearchResult, 0, len(valuesMap))
		for value, score := range valuesMap {
			merged = append(merged, storage.SearchResult{Value: value, Score: score})
		}
		if hints != nil && hints.CompareFunc != nil {
			slices.SortFunc(merged, hints.CompareFunc.Compare)
		}
		return merged, warnings, nil
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return drainSearchResultSet(searcher.SearchLabelValues(ctx, name, hints, matchers...))
}

// searchLabelNames retrieves label names using the Searcher interface.
// For multiple matcher sets it unions results from each, taking the max score per name.
func searchLabelNames(ctx context.Context, searcher storage.Searcher, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) ([]storage.SearchResult, []string, error) {
	if len(matcherSets) > 1 {
		namesMap := make(map[string]float64)
		var warnings []string
		for _, matchers := range matcherSets {
			results, w, err := drainSearchResultSet(searcher.SearchLabelNames(ctx, hints, matchers...))
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, w...)
			for _, r := range results {
				if existing, ok := namesMap[r.Value]; !ok || r.Score > existing {
					namesMap[r.Value] = r.Score
				}
			}
		}
		merged := make([]storage.SearchResult, 0, len(namesMap))
		for name, score := range namesMap {
			merged = append(merged, storage.SearchResult{Value: name, Score: score})
		}
		if hints != nil && hints.CompareFunc != nil {
			slices.SortFunc(merged, hints.CompareFunc.Compare)
		}
		return merged, warnings, nil
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return drainSearchResultSet(searcher.SearchLabelNames(ctx, hints, matchers...))
}

// buildSearchFilter builds a Filter for the given search terms and fuzzy settings.
// When multiple search terms are given, results matching any term are accepted (OR logic).
// Returns nil when no search terms are provided.
func buildSearchFilter(searches []string, fuzzThreshold int, fuzzAlg string, caseSensitive bool) storage.Filter {
	if len(searches) == 0 {
		return nil
	}
	threshold := float64(fuzzThreshold) / 100.0
	filters := make([]storage.Filter, 0, len(searches))
	for _, s := range searches {
		var f storage.Filter
		if fuzzAlg == "subsequence" {
			f = NewSubsequenceFilter(s, threshold, caseSensitive)
		} else {
			// jarowinkler: substring OR jaro-winkler fuzzy.
			substringFilter := NewSubstringFilter(s, caseSensitive)
			var fuzzyFilter *FuzzyFilter
			if fuzzThreshold > 0 {
				fuzzyFilter = NewFuzzyFilter(s, threshold, caseSensitive)
			}
			f = &orFilter{
				substringFilter: substringFilter,
				fuzzyFilter:     fuzzyFilter,
			}
		}
		filters = append(filters, f)
	}
	if len(filters) == 1 {
		return filters[0]
	}
	return newOrSearchesFilter(filters...)
}

// searchMetricNames handles GET/POST /api/v1/search/metric_names.
func (api *API) searchMetricNames(w http.ResponseWriter, r *http.Request) {
	httputil.SetCORS(w, api.CORSOrigin, r)

	if api.isAgent {
		api.respondError(w, &apiError{errorExec, errors.New("unavailable with Prometheus Agent")}, nil)
		return
	}

	if err := r.ParseForm(); err != nil {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("error parsing form values: %w", err)}, nil)
		return
	}

	sp, apiErr := api.parseSearchParams(r)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	includeMetadata := r.FormValue("include_metadata") == "true"

	// Validate sort_by.
	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for metric_names: must be \"alpha\" or \"score\"", sp.sortBy)}, nil)
		return
	}

	ctx := r.Context()
	q, err := api.Queryable.Querier(timestamp.FromTime(sp.start), timestamp.FromTime(sp.end))
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}
	defer q.Close()

	// Check if querier supports search interface.
	searcher, ok := q.(storage.Searcher)
	if !ok {
		api.respondError(w, &apiError{errorInternal, errors.New("search not supported by storage")}, nil)
		return
	}

	searchHints := &storage.SearchHints{
		Filter: buildSearchFilter(sp.searches, sp.fuzzThreshold, sp.fuzzAlg, sp.caseSensitive),
		Limit:  sp.limit + 1, // Fetch one extra to detect has_more.
	}
	switch sp.sortBy {
	case "score":
		searchHints.CompareFunc = scoreDescComparator{}
	case "alpha":
		searchHints.CompareFunc = alphaComparator{desc: sp.sortDir == "dsc"}
	}

	searchResults, warnings, err := searchLabelValues(ctx, searcher, labels.MetricName, sp.matcherSets, searchHints)
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}

	// Apply limit.
	hasMore := false
	if sp.limit > 0 && len(searchResults) > sp.limit {
		searchResults = searchResults[:sp.limit]
		hasMore = true
	}

	// Build results with optional score and metadata enrichment.
	results := make([]searchMetricNameResult, len(searchResults))
	for i, sr := range searchResults {
		results[i] = searchMetricNameResult{Name: sr.Value}
		if sp.includeScore {
			score := sr.Score
			results[i].Score = &score
		}
		if includeMetadata {
			if typ, help, unit, ok := api.getMetricMetadata(ctx, sr.Value); ok {
				results[i].Type = typ
				results[i].Help = help
				results[i].Unit = unit
			}
		}
	}

	// Stream NDJSON.
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	if err := streamBatches(nw, results, sp.batchSize, warnings); err != nil {
		// Cannot send error via respondError since headers are already sent.
		_ = nw.writeLine(searchErrorResponse{Status: "error", ErrorType: "internal", Error: err.Error()})
		return
	}

	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: hasMore})
}

// searchLabelNames handles GET/POST /api/v1/search/label_names.
func (api *API) searchLabelNames(w http.ResponseWriter, r *http.Request) {
	httputil.SetCORS(w, api.CORSOrigin, r)

	if api.isAgent {
		api.respondError(w, &apiError{errorExec, errors.New("unavailable with Prometheus Agent")}, nil)
		return
	}

	if err := r.ParseForm(); err != nil {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("error parsing form values: %w", err)}, nil)
		return
	}

	sp, apiErr := api.parseSearchParams(r)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	// Validate sort_by.
	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for label_names: must be \"alpha\" or \"score\"", sp.sortBy)}, nil)
		return
	}

	ctx := r.Context()
	q, err := api.Queryable.Querier(timestamp.FromTime(sp.start), timestamp.FromTime(sp.end))
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}
	defer q.Close()

	// Check if querier supports search interface.
	searcher, ok := q.(storage.Searcher)
	if !ok {
		api.respondError(w, &apiError{errorInternal, errors.New("search not supported by storage")}, nil)
		return
	}

	searchHints := &storage.SearchHints{
		Filter: buildSearchFilter(sp.searches, sp.fuzzThreshold, sp.fuzzAlg, sp.caseSensitive),
		Limit:  sp.limit + 1, // Fetch one extra to detect has_more.
	}
	switch sp.sortBy {
	case "score":
		searchHints.CompareFunc = scoreDescComparator{}
	case "alpha":
		searchHints.CompareFunc = alphaComparator{desc: sp.sortDir == "dsc"}
	}

	searchResults, warnings, err := searchLabelNames(ctx, searcher, sp.matcherSets, searchHints)
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}

	// Apply limit.
	hasMore := false
	if sp.limit > 0 && len(searchResults) > sp.limit {
		searchResults = searchResults[:sp.limit]
		hasMore = true
	}

	results := make([]searchLabelNameResult, len(searchResults))
	for i, sr := range searchResults {
		results[i] = searchLabelNameResult{Name: sr.Value}
		if sp.includeScore {
			score := sr.Score
			results[i].Score = &score
		}
	}

	// Stream NDJSON.
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	if err := streamBatches(nw, results, sp.batchSize, warnings); err != nil {
		_ = nw.writeLine(searchErrorResponse{Status: "error", ErrorType: "internal", Error: err.Error()})
		return
	}

	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: hasMore})
}

// searchLabelValues handles GET/POST /api/v1/search/label_values.
func (api *API) searchLabelValues(w http.ResponseWriter, r *http.Request) {
	httputil.SetCORS(w, api.CORSOrigin, r)

	if api.isAgent {
		api.respondError(w, &apiError{errorExec, errors.New("unavailable with Prometheus Agent")}, nil)
		return
	}

	if err := r.ParseForm(); err != nil {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("error parsing form values: %w", err)}, nil)
		return
	}

	sp, apiErr := api.parseSearchParams(r)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	labelName := r.FormValue("label")
	if labelName == "" {
		api.respondError(w, &apiError{errorBadData, errors.New("missing required parameter \"label\"")}, nil)
		return
	}

	// Validate sort_by.
	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for label_values: must be \"alpha\" or \"score\"", sp.sortBy)}, nil)
		return
	}

	ctx := r.Context()
	q, err := api.Queryable.Querier(timestamp.FromTime(sp.start), timestamp.FromTime(sp.end))
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}
	defer q.Close()

	// Check if querier supports search interface.
	searcher, ok := q.(storage.Searcher)
	if !ok {
		api.respondError(w, &apiError{errorInternal, errors.New("search not supported by storage")}, nil)
		return
	}

	searchHints := &storage.SearchHints{
		Filter: buildSearchFilter(sp.searches, sp.fuzzThreshold, sp.fuzzAlg, sp.caseSensitive),
		Limit:  sp.limit + 1, // Fetch one extra to detect has_more.
	}
	switch sp.sortBy {
	case "score":
		searchHints.CompareFunc = scoreDescComparator{}
	case "alpha":
		searchHints.CompareFunc = alphaComparator{desc: sp.sortDir == "dsc"}
	}

	searchResults, warnings, err := searchLabelValues(ctx, searcher, labelName, sp.matcherSets, searchHints)
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}

	// Apply limit.
	hasMore := false
	if sp.limit > 0 && len(searchResults) > sp.limit {
		searchResults = searchResults[:sp.limit]
		hasMore = true
	}

	results := make([]searchLabelValueResult, len(searchResults))
	for i, sr := range searchResults {
		results[i] = searchLabelValueResult{Name: sr.Value}
		if sp.includeScore {
			score := sr.Score
			results[i].Score = &score
		}
	}

	// Stream NDJSON.
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	if err := streamBatches(nw, results, sp.batchSize, warnings); err != nil {
		_ = nw.writeLine(searchErrorResponse{Status: "error", ErrorType: "internal", Error: err.Error()})
		return
	}

	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: hasMore})
}

// Ensure scrape.Target implements the metadata methods we need.
var _ interface {
	GetMetadata(string) (scrape.MetricMetadata, bool)
} = (*scrape.Target)(nil)
