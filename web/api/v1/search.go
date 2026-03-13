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
	search        string
	fuzzThreshold int    // 0-100, default 0 (accepts any subsequence match).
	fuzzAlg       string // "subsequence" (default) or "jarowinkler".
	caseSensitive bool   // Default true.
	sortBy        string
	sortDir       string // "asc" (default) or "dsc".
	start, end    time.Time
	limit         int // Default 100.
	batchSize     int // Default 100.
}

// searchMetricNameResult is a single result record for the metric_names endpoint.
type searchMetricNameResult struct {
	Name        string `json:"name"`
	Cardinality *int   `json:"cardinality,omitempty"`
	Type        string `json:"type,omitempty"`
	Help        string `json:"help,omitempty"`
	Unit        string `json:"unit,omitempty"`
}

// searchLabelNameResult is a single result record for the label_names endpoint.
type searchLabelNameResult struct {
	Name        string `json:"name"`
	Frequency   *int   `json:"frequency,omitempty"`
	Cardinality *int   `json:"cardinality,omitempty"`
}

// searchLabelValueResult is a single result record for the label_values endpoint.
type searchLabelValueResult struct {
	Name      string `json:"name"`
	Frequency *int   `json:"frequency,omitempty"`
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

	start, err := parseTimeParam(r, "start", MinTime)
	if err != nil {
		return sp, &apiError{errorBadData, err}
	}
	sp.start = start

	end, err := parseTimeParam(r, "end", MaxTime)
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

	sp.search = r.FormValue("search")

	sp.fuzzThreshold = 0
	if v := r.FormValue("fuzz_threshold"); v != "" {
		ft, err := strconv.Atoi(v)
		if err != nil || ft < 0 || ft > 100 {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid fuzz_threshold %q: must be 0-100", v)}
		}
		sp.fuzzThreshold = ft
	}

	// Validate fuzz_alg if provided; "subsequence" and "jarowinkler" are supported.
	if v := r.FormValue("fuzz_alg"); v != "" {
		if v != "subsequence" && v != "jarowinkler" {
			return sp, &apiError{errorBadData, fmt.Errorf("unsupported fuzz_alg %q: must be \"subsequence\" or \"jarowinkler\"", v)}
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

// countSeries counts series matching the given matchers.
func countSeries(ctx context.Context, q storage.Querier, matchers ...*labels.Matcher) (int, error) {
	ss := q.Select(ctx, false, nil, matchers...)
	count := 0
	for ss.Next() {
		count++
	}
	if err := ss.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// countSeriesMultiMatch counts series matching any of the given matcher sets (union).
func countSeriesMultiMatch(ctx context.Context, q storage.Querier, matcherSets [][]*labels.Matcher, extraMatchers []*labels.Matcher) (int, error) {
	if len(matcherSets) <= 1 {
		// Single or no matcher set: combine and count directly.
		matchers := combineMatchers(extraMatchers, matcherSets)
		return countSeries(ctx, q, matchers...)
	}

	// Multiple matcher sets: collect series labels to deduplicate.
	seriesSet := make(map[string]struct{})
	for _, matchers := range matcherSets {
		combined := append(slices.Clone(matchers), extraMatchers...)
		ss := q.Select(ctx, false, nil, combined...)
		for ss.Next() {
			seriesSet[ss.At().Labels().String()] = struct{}{}
		}
		if err := ss.Err(); err != nil {
			return 0, err
		}
	}
	return len(seriesSet), nil
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

// scored pairs a result with its search relevance score.
type scored[T any] struct {
	result T
	score  float64
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

// getLabelValues retrieves label values across multiple matcher sets, deduplicating results.
func getLabelValues(ctx context.Context, q storage.Querier, name string, matcherSets [][]*labels.Matcher, hints *storage.LabelHints) ([]string, []string, error) {
	if len(matcherSets) > 1 {
		valuesSet := make(map[string]struct{})
		var warnings []string
		for _, matchers := range matcherSets {
			vals, w, err := q.LabelValues(ctx, name, hints, matchers...)
			if err != nil {
				return nil, nil, err
			}
			for _, s := range w {
				warnings = append(warnings, s.Error())
			}
			for _, v := range vals {
				valuesSet[v] = struct{}{}
			}
		}
		result := make([]string, 0, len(valuesSet))
		for v := range valuesSet {
			result = append(result, v)
		}
		// Sort for deterministic ordering.
		slices.Sort(result)
		return result, warnings, nil
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	vals, w, err := q.LabelValues(ctx, name, hints, matchers...)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	for _, s := range w {
		warnings = append(warnings, s.Error())
	}
	return vals, warnings, nil
}

// searchLabelValuesWithScores retrieves label values with scores using the Searcher interface.
// It handles multiple matcher sets by merging results and taking the maximum score for duplicates.
func searchLabelValuesWithScores(ctx context.Context, searcher storage.Searcher, name string, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) ([]storage.SearchResult, error) {
	if len(matcherSets) > 1 {
		valuesMap := make(map[string]float64) // Track max score for each value.
		for _, matchers := range matcherSets {
			results, err := searcher.SearchLabelValues(ctx, name, hints, matchers...)
			if err != nil {
				return nil, err
			}
			for _, result := range results {
				if existing, ok := valuesMap[result.Value]; !ok || result.Score > existing {
					valuesMap[result.Value] = result.Score
				}
			}
		}
		results := make([]storage.SearchResult, 0, len(valuesMap))
		for value, score := range valuesMap {
			results = append(results, storage.SearchResult{Value: value, Score: score})
		}
		sortSearchResults(results, hints)
		return results, nil
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return searcher.SearchLabelValues(ctx, name, hints, matchers...)
}

// searchLabelNamesWithScores retrieves label names with scores using the Searcher interface.
// It handles multiple matcher sets by merging results and taking the maximum score for duplicates.
func searchLabelNamesWithScores(ctx context.Context, searcher storage.Searcher, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) ([]storage.SearchResult, error) {
	if len(matcherSets) > 1 {
		namesMap := make(map[string]float64) // Track max score for each name.
		for _, matchers := range matcherSets {
			results, err := searcher.SearchLabelNames(ctx, hints, matchers...)
			if err != nil {
				return nil, err
			}
			for _, result := range results {
				if existing, ok := namesMap[result.Value]; !ok || result.Score > existing {
					namesMap[result.Value] = result.Score
				}
			}
		}
		results := make([]storage.SearchResult, 0, len(namesMap))
		for name, score := range namesMap {
			results = append(results, storage.SearchResult{Value: name, Score: score})
		}
		sortSearchResults(results, hints)
		return results, nil
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return searcher.SearchLabelNames(ctx, hints, matchers...)
}

// sortSearchResults sorts results using the comparator from hints,
// or alphabetically by value if no comparator is set.
func sortSearchResults(results []storage.SearchResult, hints *storage.SearchHints) {
	if hints != nil && hints.CompareFunc != nil {
		slices.SortFunc(results, hints.CompareFunc.Compare)
	} else {
		slices.SortFunc(results, func(a, b storage.SearchResult) int {
			return cmp.Compare(a.Value, b.Value)
		})
	}
}

// combineMatchers returns a flat matcher slice that combines the given matchers
// with those from all matcherSets (intersected). If matcherSets is empty, only
// the given matchers are returned.
// For multi-matcher sets, this returns only the extra matchers since enrichment
// queries should count series across the union of all matcher sets.
func combineMatchers(extra []*labels.Matcher, matcherSets [][]*labels.Matcher) []*labels.Matcher {
	if len(matcherSets) == 0 {
		return extra
	}
	// For single matcher set, combine with extra matchers.
	if len(matcherSets) == 1 {
		return append(slices.Clone(matcherSets[0]), extra...)
	}
	// For multiple matcher sets, return only extra matchers.
	// The caller should query across all matcher sets separately and union results.
	return extra
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

	includeCardinality := r.FormValue("include_cardinality") == "true"
	includeMetadata := r.FormValue("include_metadata") == "true"

	// Validate sort_by.
	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "cardinality" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for metric_names: must be alpha, cardinality, or score", sp.sortBy)}, nil)
		return
	}

	// Sorting by cardinality implicitly requires cardinality data.
	if sp.sortBy == "cardinality" {
		includeCardinality = true
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

	// Build filters for search.
	var filter storage.Filter
	if sp.search != "" {
		threshold := float64(sp.fuzzThreshold) / 100.0
		if sp.fuzzAlg == "jarowinkler" {
			// Jarowinkler: substring and Jaro-Winkler fuzzy filters OR'd together.
			substringFilter := NewSubstringFilter(sp.search, sp.caseSensitive)
			var fuzzyFilter *FuzzyFilter
			if sp.fuzzThreshold < 100 {
				fuzzyFilter = NewFuzzyFilter(sp.search, threshold, sp.caseSensitive)
			}
			filter = &orFilter{
				substringFilter: substringFilter,
				fuzzyFilter:     fuzzyFilter,
			}
		} else {
			// Default: subsequence algorithm handles all match types natively.
			filter = NewSubsequenceFilter(sp.search, threshold, sp.caseSensitive)
		}
	}

	// Create search hints.
	searchHints := &storage.SearchHints{
		Filter: filter,
		Limit:  0, // Don't limit yet if we need to enrich with cardinality/metadata.
	}
	if sp.sortBy == "score" {
		searchHints.CompareFunc = scoreDescComparator{}
	}

	// Get metric names with scores from storage.
	searchResults, err := searchLabelValuesWithScores(ctx, searcher, labels.MetricName, sp.matcherSets, searchHints)
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}

	// Build enriched results.
	type scoredResult = scored[searchMetricNameResult]
	var scoredResults []scoredResult

	for _, searchResult := range searchResults {
		result := searchMetricNameResult{Name: searchResult.Value}

		if includeCardinality {
			matcher, err := labels.NewMatcher(labels.MatchEqual, labels.MetricName, searchResult.Value)
			if err == nil {
				// Count series across all match[] sets (union).
				c, err := countSeriesMultiMatch(ctx, q, sp.matcherSets, []*labels.Matcher{matcher})
				if err == nil {
					result.Cardinality = &c
				}
			}
		}

		if includeMetadata {
			if typ, help, unit, ok := api.getMetricMetadata(ctx, searchResult.Value); ok {
				result.Type = typ
				result.Help = help
				result.Unit = unit
			}
		}

		scoredResults = append(scoredResults, scoredResult{result: result, score: searchResult.Score})
	}

	// Sort if needed (if not already sorted by score).
	if sp.sortBy != "" && sp.sortBy != "score" {
		sortScoredResults(scoredResults, sp.sortBy, sp.sortDir, func(r *searchMetricNameResult) (string, int, int) {
			card := 0
			if r.Cardinality != nil {
				card = *r.Cardinality
			}
			return r.Name, card, 0
		})
	}

	// Apply limit.
	hasMore := false
	if sp.limit > 0 && len(scoredResults) > sp.limit {
		scoredResults = scoredResults[:sp.limit]
		hasMore = true
	}

	// Extract results.
	results := make([]searchMetricNameResult, len(scoredResults))
	for i, sr := range scoredResults {
		results[i] = sr.result
	}

	// Stream NDJSON.
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	if err := streamBatches(nw, results, sp.batchSize, nil); err != nil {
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

	includeFrequency := r.FormValue("include_frequency") == "true"
	includeCardinality := r.FormValue("include_cardinality") == "true"

	// Validate sort_by.
	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "cardinality" && sp.sortBy != "frequency" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for label_names: must be alpha, cardinality, frequency, or score", sp.sortBy)}, nil)
		return
	}

	// Sorting by cardinality/frequency implicitly requires the corresponding data.
	if sp.sortBy == "cardinality" {
		includeCardinality = true
	}
	if sp.sortBy == "frequency" {
		includeFrequency = true
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

	// Build filters for search.
	var filter storage.Filter
	if sp.search != "" {
		threshold := float64(sp.fuzzThreshold) / 100.0
		if sp.fuzzAlg == "jarowinkler" {
			// Jarowinkler: substring and Jaro-Winkler fuzzy filters OR'd together.
			substringFilter := NewSubstringFilter(sp.search, sp.caseSensitive)
			var fuzzyFilter *FuzzyFilter
			if sp.fuzzThreshold < 100 {
				fuzzyFilter = NewFuzzyFilter(sp.search, threshold, sp.caseSensitive)
			}
			filter = &orFilter{
				substringFilter: substringFilter,
				fuzzyFilter:     fuzzyFilter,
			}
		} else {
			// Default: subsequence algorithm handles all match types natively.
			filter = NewSubsequenceFilter(sp.search, threshold, sp.caseSensitive)
		}
	}

	// Create search hints.
	searchHints := &storage.SearchHints{
		Filter: filter,
		Limit:  0, // Don't limit yet if we need to enrich with frequency/cardinality.
	}
	if sp.sortBy == "score" {
		searchHints.CompareFunc = scoreDescComparator{}
	}

	// Get label names with scores from storage.
	searchResults, err := searchLabelNamesWithScores(ctx, searcher, sp.matcherSets, searchHints)
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}

	// Build enriched results.
	type scoredResult = scored[searchLabelNameResult]
	var scoredResults []scoredResult

	for _, searchResult := range searchResults {
		result := searchLabelNameResult{Name: searchResult.Value}

		if includeFrequency {
			matcher, err := labels.NewMatcher(labels.MatchRegexp, searchResult.Value, ".+")
			if err == nil {
				// Count series across all match[] sets (union).
				freq, err := countSeriesMultiMatch(ctx, q, sp.matcherSets, []*labels.Matcher{matcher})
				if err == nil {
					result.Frequency = &freq
				}
			}
		}

		if includeCardinality {
			// Get label values across all match[] sets (union).
			vals, _, err := getLabelValues(ctx, q, searchResult.Value, sp.matcherSets, nil)
			if err == nil {
				card := len(vals)
				result.Cardinality = &card
			}
		}

		scoredResults = append(scoredResults, scoredResult{result: result, score: searchResult.Score})
	}

	// Sort if needed (if not already sorted by score).
	if sp.sortBy != "" && sp.sortBy != "score" {
		sortScoredResults(scoredResults, sp.sortBy, sp.sortDir, func(r *searchLabelNameResult) (string, int, int) {
			card := 0
			if r.Cardinality != nil {
				card = *r.Cardinality
			}
			freq := 0
			if r.Frequency != nil {
				freq = *r.Frequency
			}
			return r.Name, card, freq
		})
	}

	// Apply limit.
	hasMore := false
	if sp.limit > 0 && len(scoredResults) > sp.limit {
		scoredResults = scoredResults[:sp.limit]
		hasMore = true
	}

	// Extract results.
	results := make([]searchLabelNameResult, len(scoredResults))
	for i, sr := range scoredResults {
		results[i] = sr.result
	}

	// Stream NDJSON.
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	if err := streamBatches(nw, results, sp.batchSize, nil); err != nil {
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

	includeFrequency := r.FormValue("include_frequency") == "true"

	// Validate sort_by.
	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "frequency" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for label_values: must be alpha, frequency, or score", sp.sortBy)}, nil)
		return
	}

	// Sorting by frequency implicitly requires frequency data.
	if sp.sortBy == "frequency" {
		includeFrequency = true
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

	// Build filters for search.
	var filter storage.Filter
	if sp.search != "" {
		threshold := float64(sp.fuzzThreshold) / 100.0
		if sp.fuzzAlg == "jarowinkler" {
			// Jarowinkler: substring and Jaro-Winkler fuzzy filters OR'd together.
			substringFilter := NewSubstringFilter(sp.search, sp.caseSensitive)
			var fuzzyFilter *FuzzyFilter
			if sp.fuzzThreshold < 100 {
				fuzzyFilter = NewFuzzyFilter(sp.search, threshold, sp.caseSensitive)
			}
			filter = &orFilter{
				substringFilter: substringFilter,
				fuzzyFilter:     fuzzyFilter,
			}
		} else {
			// Default: subsequence algorithm handles all match types natively.
			filter = NewSubsequenceFilter(sp.search, threshold, sp.caseSensitive)
		}
	}

	// Create search hints.
	searchHints := &storage.SearchHints{
		Filter: filter,
		Limit:  0, // Don't limit yet if we need to enrich with frequency.
	}
	if sp.sortBy == "score" {
		searchHints.CompareFunc = scoreDescComparator{}
	}

	// Get label values with scores from storage.
	searchResults, err := searchLabelValuesWithScores(ctx, searcher, labelName, sp.matcherSets, searchHints)
	if err != nil {
		api.respondError(w, &apiError{errorExec, err}, nil)
		return
	}

	// Build enriched results.
	type scoredResult = scored[searchLabelValueResult]
	var scoredResults []scoredResult

	for _, searchResult := range searchResults {
		result := searchLabelValueResult{Name: searchResult.Value}

		if includeFrequency {
			matcher, err := labels.NewMatcher(labels.MatchEqual, labelName, searchResult.Value)
			if err == nil {
				// Count series across all match[] sets (union).
				freq, err := countSeriesMultiMatch(ctx, q, sp.matcherSets, []*labels.Matcher{matcher})
				if err == nil {
					result.Frequency = &freq
				}
			}
		}

		scoredResults = append(scoredResults, scoredResult{result: result, score: searchResult.Score})
	}

	// Sort if needed (if not already sorted by score).
	if sp.sortBy != "" && sp.sortBy != "score" {
		sortScoredResults(scoredResults, sp.sortBy, sp.sortDir, func(r *searchLabelValueResult) (string, int, int) {
			freq := 0
			if r.Frequency != nil {
				freq = *r.Frequency
			}
			return r.Name, 0, freq
		})
	}

	// Apply limit.
	hasMore := false
	if sp.limit > 0 && len(scoredResults) > sp.limit {
		scoredResults = scoredResults[:sp.limit]
		hasMore = true
	}

	// Extract results.
	results := make([]searchLabelValueResult, len(scoredResults))
	for i, sr := range scoredResults {
		results[i] = sr.result
	}

	// Stream NDJSON.
	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	if err := streamBatches(nw, results, sp.batchSize, nil); err != nil {
		_ = nw.writeLine(searchErrorResponse{Status: "error", ErrorType: "internal", Error: err.Error()})
		return
	}

	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: hasMore})
}

// sortScoredResults sorts scored results by the given sort_by and sort_dir.
// The accessor function extracts (name, cardinality, frequency) from a result.
func sortScoredResults[T any](results []scored[T], sortBy, sortDir string, accessor func(*T) (name string, cardinality, frequency int)) {
	desc := sortDir == "dsc"
	slices.SortStableFunc(results, func(a, b scored[T]) int {
		var c int
		switch sortBy {
		case "alpha":
			nameA, _, _ := accessor(&a.result)
			nameB, _, _ := accessor(&b.result)
			c = cmp.Compare(nameA, nameB)
		case "cardinality":
			_, cardA, _ := accessor(&a.result)
			_, cardB, _ := accessor(&b.result)
			c = cmp.Compare(cardA, cardB)
		case "frequency":
			_, _, freqA := accessor(&a.result)
			_, _, freqB := accessor(&b.result)
			c = cmp.Compare(freqA, freqB)
		case "score":
			c = cmp.Compare(a.score, b.score)
		}
		if desc {
			return -c
		}
		return c
	})
}

// Ensure scrape.Target implements the metadata methods we need.
var _ interface {
	GetMetadata(string) (scrape.MetricMetadata, bool)
} = (*scrape.Target)(nil)
