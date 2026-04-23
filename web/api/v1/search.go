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
	"container/heap"
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
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/httputil"
)

// FuzzAlgorithms is the canonical list of supported fuzzy matching algorithms.
// It is the single source of truth used for validation, feature registration, and API documentation.
var FuzzAlgorithms = []string{"subsequence", "jarowinkler"}

// searchParams holds the common parsed parameters for all search endpoints.
type searchParams struct {
	matcherSets   [][]*labels.Matcher
	searches      []string
	fuzzThreshold int    // 0-100, default 0 (lowest fuzzy threshold).
	fuzzAlg       string // "subsequence" (default) or "jarowinkler".
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

	// Validate fuzz_alg if provided; FuzzAlgorithms lists all supported values.
	sp.fuzzAlg = FuzzAlgorithms[0]
	if v := r.FormValue("fuzz_alg"); v != "" {
		if !slices.Contains(FuzzAlgorithms, v) {
			return sp, &apiError{errorBadData, fmt.Errorf("unsupported fuzz_alg %q: must be one of %v", v, FuzzAlgorithms)}
		}
		sp.fuzzAlg = v
	}

	caseSensitive, apiErr := parseSearchBoolParam(r, "case_sensitive", true)
	if apiErr != nil {
		return sp, apiErr
	}
	sp.caseSensitive = caseSensitive

	includeScore, apiErr := parseSearchBoolParam(r, "include_score", false)
	if apiErr != nil {
		return sp, apiErr
	}
	sp.includeScore = includeScore

	sp.sortBy = r.FormValue("sort_by")
	sp.sortDir = r.FormValue("sort_dir")
	if sp.sortDir != "" && sp.sortBy == "" {
		return sp, &apiError{errorBadData, errors.New("sort_dir is only valid when sort_by is set")}
	}
	if sp.sortDir != "" && sp.sortBy == "score" {
		return sp, &apiError{errorBadData, errors.New("sort_dir is not supported for sort_by=score")}
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

func parseSearchBoolParam(r *http.Request, name string, defaultValue bool) (bool, *apiError) {
	v := r.FormValue(name)
	if v == "" {
		return defaultValue, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, &apiError{errorBadData, fmt.Errorf("invalid %s %q: must be boolean", name, v)}
	}
	return b, nil
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

// sortOrdering maps sort_by and sort_dir parameters to a storage.Ordering.
func sortOrdering(sortBy, sortDir string) storage.Ordering {
	switch sortBy {
	case "score":
		return storage.OrderByScoreDesc
	case "alpha":
		if sortDir == "dsc" {
			return storage.OrderByValueDesc
		}
	}
	return storage.OrderByValueAsc
}

// compareByOrdering returns a comparison function for the given ordering.
func compareByOrdering(o storage.Ordering) func(a, b storage.SearchResult) int {
	switch o {
	case storage.OrderByScoreDesc:
		return func(a, b storage.SearchResult) int {
			if c := cmp.Compare(b.Score, a.Score); c != 0 {
				return c
			}
			return cmp.Compare(a.Value, b.Value)
		}
	case storage.OrderByValueDesc:
		return func(a, b storage.SearchResult) int { return cmp.Compare(b.Value, a.Value) }
	default:
		return func(a, b storage.SearchResult) int { return cmp.Compare(a.Value, b.Value) }
	}
}

func searchAPIError(err error) *apiError {
	result := setUnavailStatusOnTSDBNotReady(apiFuncResult{err: returnAPIError(err)})
	return result.err
}

func (api *API) respondPreStreamSearchError(w http.ResponseWriter, err error) {
	api.respondError(w, searchAPIError(err), nil)
}

func writeStreamSearchError(nw *ndjsonWriter, err error) {
	apiErr := searchAPIError(err)
	_ = nw.writeLine(searchErrorResponse{Status: "error", ErrorType: apiErr.typ.str, Error: apiErr.err.Error()})
}

func writeStreamInternalError(nw *ndjsonWriter, err error) {
	_ = nw.writeLine(searchErrorResponse{Status: "error", ErrorType: errorInternal.str, Error: err.Error()})
}

func searchWarnings(rs storage.SearchResultSet) []string {
	var warnings []string
	for _, w := range rs.Warnings() {
		warnings = append(warnings, w.Error())
	}
	return warnings
}

// searchResultStreamer is generic because each endpoint streams a distinct result record type.
// The batching and has_more logic is shared.
type searchResultStreamer[T any] struct {
	rs        storage.SearchResultSet
	limit     int
	batchSize int
	emitted   int
	hasMore   bool
	toResult  func(storage.SearchResult) T
}

func (s *searchResultStreamer[T]) nextBatch() ([]T, error) {
	if s.hasMore {
		return nil, nil
	}
	batch := make([]T, 0, s.batchSize)
	for len(batch) < s.batchSize {
		if s.limit > 0 && s.emitted >= s.limit {
			if s.rs.Next() {
				s.hasMore = true
			}
			return batch, s.rs.Err()
		}
		if !s.rs.Next() {
			return batch, s.rs.Err()
		}
		s.emitted++
		batch = append(batch, s.toResult(s.rs.At()))
	}
	return batch, nil
}

func streamSearchResults[T any](api *API, w http.ResponseWriter, rs storage.SearchResultSet, sp searchParams, toResult func(storage.SearchResult) T) {
	defer func() { _ = rs.Close() }()

	streamer := &searchResultStreamer[T]{
		rs:        rs,
		limit:     sp.limit,
		batchSize: sp.batchSize,
		toResult:  toResult,
	}

	firstBatch, err := streamer.nextBatch()
	if err != nil {
		api.respondPreStreamSearchError(w, err)
		return
	}
	firstWarnings := searchWarnings(rs)

	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	if len(firstBatch) == 0 {
		if err := nw.writeLine(searchBatch[T]{Results: []T{}, Warnings: firstWarnings}); err != nil {
			writeStreamInternalError(nw, err)
			return
		}
		_ = nw.writeLine(searchTrailer{Status: "success", HasMore: streamer.hasMore})
		return
	}

	if err := nw.writeLine(searchBatch[T]{Results: firstBatch, Warnings: firstWarnings}); err != nil {
		writeStreamInternalError(nw, err)
		return
	}

	for {
		batch, err := streamer.nextBatch()
		if len(batch) > 0 {
			if writeErr := nw.writeLine(searchBatch[T]{Results: batch}); writeErr != nil {
				writeStreamInternalError(nw, writeErr)
				return
			}
		}
		if err != nil {
			writeStreamSearchError(nw, err)
			return
		}
		if len(batch) == 0 {
			break
		}
	}

	trailerWarnings := searchWarnings(rs)
	if slices.Equal(trailerWarnings, firstWarnings) {
		trailerWarnings = nil
	}
	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: streamer.hasMore, Warnings: trailerWarnings})
}

type searchResultSetItem struct {
	rs    storage.SearchResultSet
	value storage.SearchResult
}

type searchResultSetHeap struct {
	items []*searchResultSetItem
	cmp   func(a, b storage.SearchResult) int
}

func (h searchResultSetHeap) Len() int { return len(h.items) }

func (h searchResultSetHeap) Less(i, j int) bool {
	return h.cmp(h.items[i].value, h.items[j].value) < 0
}

func (h searchResultSetHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *searchResultSetHeap) Push(x any) {
	h.items = append(h.items, x.(*searchResultSetItem))
}

func (h *searchResultSetHeap) Pop() any {
	old := h.items
	n := len(old)
	item := old[n-1]
	h.items = old[:n-1]
	return item
}

func (h searchResultSetHeap) peek() *searchResultSetItem {
	if len(h.items) == 0 {
		return nil
	}
	return h.items[0]
}

type mergedSearchResultSet struct {
	sets        []storage.SearchResultSet
	heap        searchResultSetHeap
	order       storage.Ordering
	limit       int
	emitted     int
	initialized bool
	done        bool
	err         error
	curr        storage.SearchResult
}

func newMergedSearchResultSet(sets []storage.SearchResultSet, hints *storage.SearchHints) storage.SearchResultSet {
	if len(sets) == 0 {
		return storage.EmptySearchResultSet()
	}
	var (
		order storage.Ordering
		limit int
	)
	if hints != nil {
		order = hints.OrderBy
		limit = hints.Limit
	}
	return &mergedSearchResultSet{
		sets:  sets,
		heap:  searchResultSetHeap{cmp: compareByOrdering(order)},
		order: order,
		limit: limit,
	}
}

func (s *mergedSearchResultSet) initialize() {
	if s.initialized {
		return
	}
	s.initialized = true
	for _, rs := range s.sets {
		if rs.Next() {
			heap.Push(&s.heap, &searchResultSetItem{rs: rs, value: rs.At()})
			continue
		}
		if err := rs.Err(); err != nil {
			s.err = err
			s.done = true
			return
		}
	}
}

func (s *mergedSearchResultSet) advance(item *searchResultSetItem) {
	if s.done {
		return
	}
	if item.rs.Next() {
		item.value = item.rs.At()
		heap.Push(&s.heap, item)
		return
	}
	if err := item.rs.Err(); err != nil {
		s.err = err
	}
}

func (s *mergedSearchResultSet) sameResult(a, b storage.SearchResult) bool {
	if s.order == storage.OrderByValueAsc || s.order == storage.OrderByValueDesc {
		return a.Value == b.Value
	}
	return compareByOrdering(s.order)(a, b) == 0
}

func (s *mergedSearchResultSet) Next() bool {
	if s.done {
		return false
	}
	if s.err != nil {
		s.done = true
		return false
	}
	if s.limit > 0 && s.emitted >= s.limit {
		s.done = true
		return false
	}

	s.initialize()
	if s.done || s.heap.Len() == 0 {
		s.done = true
		return false
	}

	item := heap.Pop(&s.heap).(*searchResultSetItem)
	curr := item.value
	s.advance(item)

	for {
		next := s.heap.peek()
		if next == nil || !s.sameResult(curr, next.value) {
			break
		}
		dup := heap.Pop(&s.heap).(*searchResultSetItem)
		if dup.value.Score > curr.Score {
			curr.Score = dup.value.Score
		}
		s.advance(dup)
	}

	s.curr = curr
	s.emitted++
	return true
}

func (s *mergedSearchResultSet) At() storage.SearchResult {
	return s.curr
}

func (s *mergedSearchResultSet) Warnings() annotations.Annotations {
	var warnings annotations.Annotations
	for _, rs := range s.sets {
		warnings.Merge(rs.Warnings())
	}
	return warnings
}

func (s *mergedSearchResultSet) Err() error {
	if s.err != nil {
		return s.err
	}
	for _, rs := range s.sets {
		if err := rs.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *mergedSearchResultSet) Close() error {
	errs := make([]error, 0, len(s.sets))
	for _, rs := range s.sets {
		errs = append(errs, rs.Close())
	}
	return errors.Join(errs...)
}

// searchLabelValues retrieves label values using the Searcher interface.
// For multiple matcher sets it unions results from each, taking the max score per value.
func searchLabelValues(ctx context.Context, searcher storage.Searcher, name string, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) storage.SearchResultSet {
	if len(matcherSets) > 1 {
		sets := make([]storage.SearchResultSet, 0, len(matcherSets))
		for _, matchers := range matcherSets {
			sets = append(sets, searcher.SearchLabelValues(ctx, name, hints, matchers...))
		}
		return newMergedSearchResultSet(sets, hints)
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return searcher.SearchLabelValues(ctx, name, hints, matchers...)
}

// searchLabelNames retrieves label names using the Searcher interface.
// For multiple matcher sets it unions results from each, taking the max score per name.
func searchLabelNames(ctx context.Context, searcher storage.Searcher, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) storage.SearchResultSet {
	if len(matcherSets) > 1 {
		sets := make([]storage.SearchResultSet, 0, len(matcherSets))
		for _, matchers := range matcherSets {
			sets = append(sets, searcher.SearchLabelNames(ctx, hints, matchers...))
		}
		return newMergedSearchResultSet(sets, hints)
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return searcher.SearchLabelNames(ctx, hints, matchers...)
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
			// Jaro-Winkler: substring OR Jaro-Winkler fuzzy.
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

	if !api.enableSearch {
		api.respondError(w, &apiError{errorUnavailable, errors.New("search API disabled")}, nil)
		return
	}

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

	includeMetadata, apiErr := parseSearchBoolParam(r, "include_metadata", false)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	// Validate sort_by.
	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for metric_names: must be \"alpha\" or \"score\"", sp.sortBy)}, nil)
		return
	}

	ctx := r.Context()
	q, err := api.Queryable.Querier(timestamp.FromTime(sp.start), timestamp.FromTime(sp.end))
	if err != nil {
		api.respondPreStreamSearchError(w, err)
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
	searchHints.OrderBy = sortOrdering(sp.sortBy, sp.sortDir)

	searchResults := searchLabelValues(ctx, searcher, labels.MetricName, sp.matcherSets, searchHints)
	streamSearchResults(api, w, searchResults, sp, func(sr storage.SearchResult) searchMetricNameResult {
		result := searchMetricNameResult{Name: sr.Value}
		if sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		if includeMetadata {
			if typ, help, unit, ok := api.getMetricMetadata(ctx, sr.Value); ok {
				result.Type = typ
				result.Help = help
				result.Unit = unit
			}
		}
		return result
	})
}

// searchLabelNames handles GET/POST /api/v1/search/label_names.
func (api *API) searchLabelNames(w http.ResponseWriter, r *http.Request) {
	httputil.SetCORS(w, api.CORSOrigin, r)

	if !api.enableSearch {
		api.respondError(w, &apiError{errorUnavailable, errors.New("search API disabled")}, nil)
		return
	}

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
		api.respondPreStreamSearchError(w, err)
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
	searchHints.OrderBy = sortOrdering(sp.sortBy, sp.sortDir)

	searchResults := searchLabelNames(ctx, searcher, sp.matcherSets, searchHints)
	streamSearchResults(api, w, searchResults, sp, func(sr storage.SearchResult) searchLabelNameResult {
		result := searchLabelNameResult{Name: sr.Value}
		if sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		return result
	})
}

// searchLabelValues handles GET/POST /api/v1/search/label_values.
func (api *API) searchLabelValues(w http.ResponseWriter, r *http.Request) {
	httputil.SetCORS(w, api.CORSOrigin, r)

	if !api.enableSearch {
		api.respondError(w, &apiError{errorUnavailable, errors.New("search API disabled")}, nil)
		return
	}

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
		api.respondPreStreamSearchError(w, err)
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
	searchHints.OrderBy = sortOrdering(sp.sortBy, sp.sortDir)

	searchResults := searchLabelValues(ctx, searcher, labelName, sp.matcherSets, searchHints)
	streamSearchResults(api, w, searchResults, sp, func(sr storage.SearchResult) searchLabelValueResult {
		result := searchLabelValueResult{Name: sr.Value}
		if sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		return result
	})
}

// Ensure scrape.Target implements the metadata methods we need.
var _ interface {
	GetMetadata(string) (scrape.MetricMetadata, bool)
} = (*scrape.Target)(nil)
