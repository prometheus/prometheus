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

// Search API stream contract:
//
//   - Successful responses use Content-Type "application/x-ndjson" and consist
//     of one JSON object per line.
//   - Zero or more searchBatch lines are emitted, each carrying a "results"
//     array and an optional "warnings" array. The first batch always emits
//     even when "results" is empty so clients can observe warnings reliably.
//   - The stream then terminates with EITHER a searchTrailer line (status
//     "success", optional "warnings" delta, "has_more" indicator) OR a
//     searchErrorResponse line (status "error", "errorType", "error") if the
//     storage backend errored mid-stream after the first batch was sent.
//   - Errors that occur before the first batch is written are reported as the
//     usual non-streaming JSON error object with a 4xx/5xx status code.
//   - Clients MUST tolerate an abrupt EOF without a trailer (e.g. transport
//     failures or server shutdown) and MUST ignore unknown fields in the
//     trailer for forward compatibility.
//
// Pagination scope: this version of the API has no cursor mechanism. The
// "has_more" flag in the trailer is informational only; clients that need
// more results should re-issue the request with a higher "limit"
// (subject to --web.search.max-limit) or narrow the "match[]" series selectors.
// A future version may introduce a cursor field in the trailer.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/httputil"
)

// defaultFuzzAlg is the algorithm assumed when fuzz_alg is not specified.
const defaultFuzzAlg = "subsequence"

// maxSearchTermsPerRequest caps the number of search[] query parameters
// accepted in one request. Per-value filter cost grows with the number of
// terms (each term adds at least one substring filter, optionally a fuzzy
// filter), so an unbounded count is a DoS surface. 32 is comfortably above
// realistic autocomplete usage (typically 1–3 terms) and tight enough that
// worst-case per-value evaluation stays bounded.
const maxSearchTermsPerRequest = 32

// fuzzAlgorithms is the canonical list of supported fuzzy matching algorithms,
// used by validation, feature registration, and API documentation. It is kept
// unexported so it cannot be mutated by external packages; use FuzzAlgorithms()
// to obtain a defensive copy.
var fuzzAlgorithms = []string{defaultFuzzAlg, "jarowinkler"}

// FuzzAlgorithms returns the canonical list of supported fuzzy matching
// algorithms. The returned slice is a copy and may be modified safely.
func FuzzAlgorithms() []string {
	return slices.Clone(fuzzAlgorithms)
}

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
	// end == start is permitted: it represents a zero-duration "snapshot at
	// this instant" search. Only strictly inverted ranges are rejected, so
	// a client that accidentally sets end < start gets an immediate error
	// rather than empty (and possibly misleading) results.
	if sp.end.Before(sp.start) {
		return sp, &apiError{errorBadData, errors.New("end timestamp must not be before start timestamp")}
	}

	if matchers := r.Form["match[]"]; len(matchers) > 0 {
		matcherSets, err := api.parseMatchersParam(matchers)
		if err != nil {
			return sp, &apiError{errorBadData, err}
		}
		sp.matcherSets = matcherSets
	}

	sp.searches = r.Form["search[]"]
	if len(sp.searches) > maxSearchTermsPerRequest {
		return sp, &apiError{errorBadData, fmt.Errorf(
			"too many search[] terms: got %d, maximum is %d",
			len(sp.searches), maxSearchTermsPerRequest)}
	}

	sp.fuzzThreshold = 0
	if v := r.FormValue("fuzz_threshold"); v != "" {
		ft, err := strconv.Atoi(v)
		if err != nil || ft < 0 || ft > 100 {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid fuzz_threshold %q: must be 0-100", v)}
		}
		sp.fuzzThreshold = ft
	}

	// Validate fuzz_alg if provided; fuzzAlgorithms lists all supported values.
	sp.fuzzAlg = defaultFuzzAlg
	if v := r.FormValue("fuzz_alg"); v != "" {
		if !slices.Contains(fuzzAlgorithms, v) {
			return sp, &apiError{errorBadData, fmt.Errorf("unsupported fuzz_alg %q: must be one of %v", v, fuzzAlgorithms)}
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

	// Default limit is shrunk to maxSearchLimit when the operator configured
	// a smaller cap, so a request that omits "limit" still serves up to the
	// configured maximum rather than failing the cap check unconditionally.
	sp.limit = 100
	if api.maxSearchLimit > 0 && sp.limit > api.maxSearchLimit {
		sp.limit = api.maxSearchLimit
	}
	if v := r.FormValue("limit"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil || l < 0 {
			return sp, &apiError{errorBadData, fmt.Errorf("invalid limit %q: must be non-negative integer", v)}
		}
		if l > 0 {
			if api.maxSearchLimit > 0 && l > api.maxSearchLimit {
				return sp, &apiError{errorBadData, fmt.Errorf("limit %d exceeds the configured maximum (%d, see --web.search.max-limit)", l, api.maxSearchLimit)}
			}
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

// searchHintsLimit returns the storage-level Limit for a request: one above
// spLimit so the streamer can detect has_more by probing past the cap. The
// saturation guard avoids the int overflow that would be possible when the
// operator has disabled the cap with --web.search.max-limit=0 and a client
// supplies a near-MaxInt limit.
func searchHintsLimit(spLimit int) int {
	if spLimit >= math.MaxInt {
		return math.MaxInt
	}
	return spLimit + 1
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

// metricMetadataCacheTTL is the lifetime of a cached metric-metadata map.
// Autocomplete UIs hit /api/v1/search/metric_names with include_metadata=true
// on every keystroke; without a cache each request locks the scrape manager
// and walks every active target's metadata. The TTL is short so that operators
// who push new scrape targets see them in the search results within seconds.
const metricMetadataCacheTTL = 5 * time.Second

// metadataCacheEntry is the immutable payload stored in searchMetadataCache.
// Both fields are set together on each cache update so a reader observing a
// non-nil entry can read built and data without further synchronisation.
type metadataCacheEntry struct {
	built time.Time
	data  map[string]scrape.MetricMetadata
}

// searchMetadataCache caches the metric metadata map produced by
// buildMetricMetadataMap for a short TTL. It is shared across all in-flight
// /api/v1/search/metric_names requests on one API instance. Reads are
// lock-free via atomic.Pointer; concurrent writers race and the last Store
// wins, which is acceptable because the underlying scrape-manager snapshot is
// idempotent.
type searchMetadataCache struct {
	entry atomic.Pointer[metadataCacheEntry]
}

// buildMetricMetadataMap snapshots metric metadata across all active targets
// into a single map keyed by metric family name. It is intended to be called
// once per request when include_metadata=true so that per-result metadata
// lookups are O(1) and we acquire the scrape manager lock only once instead
// of once per emitted result.
//
// Results are cached on api.metaCache for metricMetadataCacheTTL so that a
// burst of autocomplete requests does not re-lock the scrape manager on every
// keystroke. Concurrent cache misses may rebuild redundantly — the alternative
// (sync/singleflight) is more complexity than the experimental endpoint
// warrants. Partial maps from a cancelled ctx are not stored in the cache.
//
// Callers must treat the returned map as read-only: on a cache hit the map
// is shared across concurrent requests, and any mutation would race with
// every other reader holding the same reference.
//
// Iteration order over active targets is non-deterministic; for a metric name
// that appears on multiple targets we keep the first metadata seen, matching
// the prior per-result fallthrough behaviour.
//
// The traversal aborts as soon as ctx is done so a request that the client
// has already abandoned (or one that has run past its deadline) does not
// keep accumulating per-target locks. Callers tolerate a partial map: a
// missing entry just means the result is emitted without metadata.
func (api *API) buildMetricMetadataMap(ctx context.Context) map[string]scrape.MetricMetadata {
	if c := api.metaCache; c != nil {
		if e := c.entry.Load(); e != nil && api.now().Sub(e.built) < metricMetadataCacheTTL {
			return e.data
		}
	}

	tr := api.targetRetriever(ctx)
	if tr == nil {
		return nil
	}
	out := map[string]scrape.MetricMetadata{}
	for _, targets := range tr.TargetsActive() {
		if ctx.Err() != nil {
			return out
		}
		for _, t := range targets {
			if ctx.Err() != nil {
				return out
			}
			for _, md := range t.ListMetadata() {
				if ctx.Err() != nil {
					return out
				}
				if _, exists := out[md.MetricFamily]; !exists {
					out[md.MetricFamily] = md
				}
			}
		}
	}

	if ctx.Err() == nil && api.metaCache != nil {
		api.metaCache.entry.Store(&metadataCacheEntry{built: api.now(), data: out})
	}
	return out
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

func streamSearchResults[T any](ctx context.Context, api *API, w http.ResponseWriter, rs storage.SearchResultSet, sp searchParams, toResult func(storage.SearchResult) T) {
	defer func() { _ = rs.Close() }()

	streamer := &searchResultStreamer[T]{
		rs:        rs,
		limit:     sp.limit,
		batchSize: sp.batchSize,
		toResult:  toResult,
	}

	firstBatch, firstErr := streamer.nextBatch()
	// A non-nil firstErr with zero results means the underlying iterator
	// could not produce anything; respond with the standard JSON error so
	// clients see a well-formed failure. When firstErr arrives alongside
	// partial results (e.g. one matcher-set succeeded and another failed,
	// fitting in the first batch), we open the stream so the partial data
	// is not lost — the in-band error line below signals the failure.
	if firstErr != nil && len(firstBatch) == 0 {
		api.respondPreStreamSearchError(w, firstErr)
		return
	}
	// Sort warnings so the order is deterministic on the wire and the
	// trailer dedup against trailerWarnings below is order-independent.
	// annotations.Annotations is map-backed.
	firstWarnings := searchWarnings(rs)
	slices.Sort(firstWarnings)

	nw, err := newNDJSONWriter(w)
	if err != nil {
		api.respondError(w, &apiError{errorInternal, err}, nil)
		return
	}

	// Always emit a first batch line so warnings are observable before any
	// trailer or error line, even when there are no results.
	if writeErr := nw.writeLine(searchBatch[T]{Results: firstBatch, Warnings: firstWarnings}); writeErr != nil {
		writeStreamInternalError(nw, writeErr)
		return
	}

	if firstErr != nil {
		writeStreamSearchError(nw, firstErr)
		return
	}

	if len(firstBatch) == 0 {
		_ = nw.writeLine(searchTrailer{Status: "success", HasMore: streamer.hasMore})
		return
	}

	for {
		// Stop pulling from storage as soon as the client goes away.
		// Without this check, an abandoned request keeps iterating the
		// underlying SearchResultSet (which may itself be doing real I/O).
		select {
		case <-ctx.Done():
			return
		default:
		}
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

	// Re-snapshot warnings after iteration: a Searcher may emit new warnings
	// while the merge tree is drained (e.g. a secondary querier whose error
	// becomes a warning at exhaustion). Dedup against the first batch so we
	// don't echo warnings the client has already received. Sort to match
	// firstWarnings' canonical order.
	trailerWarnings := searchWarnings(rs)
	slices.Sort(trailerWarnings)
	trailerWarnings := searchWarnings(rs)
	if slices.Equal(trailerWarnings, firstWarnings) {
		trailerWarnings = nil
	}
	_ = nw.writeLine(searchTrailer{Status: "success", HasMore: streamer.hasMore, Warnings: trailerWarnings})
}

// searchLabelValues retrieves label values using the Searcher interface.
// For multiple matcher sets the per-set results are merged via the storage
// helper, which handles deduplication, max-score collapse, and ordering.
// Each per-set sub-query is wrapped in a lazy SearchResultSet so the merge
// tree can short-circuit on limit without paying construction cost for
// branches it never pulls from.
func searchLabelValues(ctx context.Context, searcher storage.Searcher, name string, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) storage.SearchResultSet {
	if len(matcherSets) > 1 {
		sets := make([]storage.SearchResultSet, 0, len(matcherSets))
		for _, matchers := range matcherSets {
			sets = append(sets, storage.NewLazySearchResultSet(func() storage.SearchResultSet {
				return searcher.SearchLabelValues(ctx, name, hints, matchers...)
			}))
		}
		return storage.MergeSearchResultSets(sets, hints)
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return searcher.SearchLabelValues(ctx, name, hints, matchers...)
}

// searchLabelNames retrieves label names using the Searcher interface.
// For multiple matcher sets the per-set results are merged via the storage
// helper, which handles deduplication, max-score collapse, and ordering.
// Each per-set sub-query is wrapped in a lazy SearchResultSet so the merge
// tree can short-circuit on limit without paying construction cost for
// branches it never pulls from.
func searchLabelNames(ctx context.Context, searcher storage.Searcher, matcherSets [][]*labels.Matcher, hints *storage.SearchHints) storage.SearchResultSet {
	if len(matcherSets) > 1 {
		sets := make([]storage.SearchResultSet, 0, len(matcherSets))
		for _, matchers := range matcherSets {
			sets = append(sets, storage.NewLazySearchResultSet(func() storage.SearchResultSet {
				return searcher.SearchLabelNames(ctx, hints, matchers...)
			}))
		}
		return storage.MergeSearchResultSets(sets, hints)
	}

	var matchers []*labels.Matcher
	if len(matcherSets) == 1 {
		matchers = matcherSets[0]
	}
	return searcher.SearchLabelNames(ctx, hints, matchers...)
}

// buildSearchFilter builds a Filter for the given search terms and fuzzy settings.
// When multiple search terms are given, results matching any term are accepted (OR logic).
// Empty search terms are skipped. Returns nil when no usable search terms remain.
// For case-insensitive search, the query is lowercased here and the chain is wrapped
// with caseFoldingFilter so values are lowercased once at the top of the chain.
// When the chain contains an expensive matcher (subsequence, or Jaro-Winkler with a
// non-zero threshold) it is wrapped with memoizingFilter so values that reach the
// chain multiple times in one search (e.g. once per TSDB block) are scored once.
// Substring-only chains skip the memo: substring scoring is already O(L) and
// the cache lookup would only add overhead.
func buildSearchFilter(searches []string, fuzzThreshold int, fuzzAlg string, caseSensitive bool) storage.Filter {
	terms := make([]string, 0, len(searches))
	for _, s := range searches {
		if s == "" {
			continue
		}
		if !caseSensitive {
			s = strings.ToLower(s)
		}
		terms = append(terms, s)
	}
	if len(terms) == 0 {
		return nil
	}
	threshold := float64(fuzzThreshold) / 100.0
	filters := make([]storage.Filter, 0, len(terms))
	for _, s := range terms {
		var f storage.Filter
		if fuzzAlg == "subsequence" {
			f = NewSubsequenceFilter(s, threshold)
		} else {
			// Jaro-Winkler: substring OR Jaro-Winkler fuzzy.
			substringFilter := NewSubstringFilter(s)
			var fuzzyFilter *FuzzyFilter
			if fuzzThreshold > 0 {
				fuzzyFilter = NewFuzzyFilter(s, threshold)
			}
			f = &orFilter{
				substringFilter: substringFilter,
				fuzzyFilter:     fuzzyFilter,
			}
		}
		filters = append(filters, f)
	}
	var combined storage.Filter
	if len(filters) == 1 {
		combined = filters[0]
	} else {
		combined = newOrSearchesFilter(filters...)
	}
	if !caseSensitive {
		combined = newCaseFoldingFilter(combined)
	}
	if filterChainHasExpensiveScoring(fuzzThreshold, fuzzAlg) {
		combined = newMemoizingFilter(combined)
	}
	return combined
}

// filterChainHasExpensiveScoring reports whether the search filter chain built
// by buildSearchFilter will exercise a non-trivial scoring path that justifies
// memoization across blocks.
func filterChainHasExpensiveScoring(fuzzThreshold int, fuzzAlg string) bool {
	if fuzzAlg == "subsequence" {
		return true
	}
	// Jaro-Winkler is only constructed when fuzzThreshold > 0; below that the
	// chain is substring-only and memoization is not worth its overhead.
	return fuzzThreshold > 0
}

// searchRequest holds the common objects prepared by newSearchRequest for a search request.
type searchRequest struct {
	sp       searchParams
	hints    *storage.SearchHints
	searcher storage.Searcher
	q        storage.Querier
}

// newSearchRequest handles the setup shared by all search endpoints: CORS headers,
// feature-gate checks, form parsing, common parameter parsing, sort_by
// validation, querier acquisition, and search hint construction. On success a
// non-nil searchRequest is returned and the caller must defer req.q.Close(). On
// failure the error has already been written to w and nil is returned.
func (api *API) newSearchRequest(w http.ResponseWriter, r *http.Request, endpoint string) *searchRequest {
	httputil.SetCORS(w, api.CORSOrigin, r)

	if !api.enableSearch {
		api.respondError(w, &apiError{errorUnavailable, errors.New("search API disabled")}, nil)
		return nil
	}

	if api.isAgent {
		api.respondError(w, &apiError{errorExec, errors.New("unavailable with Prometheus Agent")}, nil)
		return nil
	}

	if err := r.ParseForm(); err != nil {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("error parsing form values: %w", err)}, nil)
		return nil
	}

	sp, apiErr := api.parseSearchParams(r)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return nil
	}

	if sp.sortBy != "" && sp.sortBy != "alpha" && sp.sortBy != "score" {
		api.respondError(w, &apiError{errorBadData, fmt.Errorf("invalid sort_by %q for %s: must be \"alpha\" or \"score\"", sp.sortBy, endpoint)}, nil)
		return nil
	}

	q, err := api.Queryable.Querier(timestamp.FromTime(sp.start), timestamp.FromTime(sp.end))
	if err != nil {
		api.respondPreStreamSearchError(w, err)
		return nil
	}

	searcher, ok := q.(storage.Searcher)
	if !ok {
		_ = q.Close()
		api.respondError(w, &apiError{errorInternal, errors.New("search not supported by storage")}, nil)
		return nil
	}

	hints := &storage.SearchHints{
		Filter: buildSearchFilter(sp.searches, sp.fuzzThreshold, sp.fuzzAlg, sp.caseSensitive),
		Limit:  searchHintsLimit(sp.limit), // Fetch one extra to detect has_more (with saturation guard).
	}
	hints.OrderBy = sortOrdering(sp.sortBy, sp.sortDir)

	return &searchRequest{sp: sp, hints: hints, searcher: searcher, q: q}
}

// searchMetricNames handles GET/POST /api/v1/search/metric_names.
func (api *API) searchMetricNames(w http.ResponseWriter, r *http.Request) {
	req := api.newSearchRequest(w, r, "metric_names")
	if req == nil {
		return
	}
	defer req.q.Close()

	includeMetadata, apiErr := parseSearchBoolParam(r, "include_metadata", false)
	if apiErr != nil {
		api.respondError(w, apiErr, nil)
		return
	}

	ctx := r.Context()

	// metaMap is built lazily on the first metadata lookup so a search that
	// returns zero results never pays the scrape-manager lock + map-build
	// cost. The streamer drives toResult from a single goroutine, so the
	// captured flag does not need a lock.
	var (
		metaMap     map[string]scrape.MetricMetadata
		metaMapDone bool
	)

	searchResults := searchLabelValues(ctx, req.searcher, labels.MetricName, req.sp.matcherSets, req.hints)
	streamSearchResults(ctx, api, w, searchResults, req.sp, func(sr storage.SearchResult) searchMetricNameResult {
		result := searchMetricNameResult{Name: sr.Value}
		if req.sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		if includeMetadata {
			if !metaMapDone {
				metaMap = api.buildMetricMetadataMap(ctx)
				metaMapDone = true
			}
			if md, ok := metaMap[sr.Value]; ok {
				result.Type = string(md.Type)
				result.Help = md.Help
				result.Unit = md.Unit
			}
		}
		return result
	})
}

// searchLabelNames handles GET/POST /api/v1/search/label_names.
func (api *API) searchLabelNames(w http.ResponseWriter, r *http.Request) {
	req := api.newSearchRequest(w, r, "label_names")
	if req == nil {
		return
	}
	defer req.q.Close()

	ctx := r.Context()
	searchResults := searchLabelNames(ctx, req.searcher, req.sp.matcherSets, req.hints)
	streamSearchResults(ctx, api, w, searchResults, req.sp, func(sr storage.SearchResult) searchLabelNameResult {
		result := searchLabelNameResult{Name: sr.Value}
		if req.sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		return result
	})
}

// searchLabelValues handles GET/POST /api/v1/search/label_values.
func (api *API) searchLabelValues(w http.ResponseWriter, r *http.Request) {
	req := api.newSearchRequest(w, r, "label_values")
	if req == nil {
		return
	}
	defer req.q.Close()

	labelName := r.FormValue("label")
	if labelName == "" {
		api.respondError(w, &apiError{errorBadData, errors.New("missing required parameter \"label\"")}, nil)
		return
	}

	ctx := r.Context()
	searchResults := searchLabelValues(ctx, req.searcher, labelName, req.sp.matcherSets, req.hints)
	streamSearchResults(ctx, api, w, searchResults, req.sp, func(sr storage.SearchResult) searchLabelValueResult {
		result := searchLabelValueResult{Name: sr.Value}
		if req.sp.includeScore {
			score := sr.Score
			result.Score = &score
		}
		return result
	})
}
