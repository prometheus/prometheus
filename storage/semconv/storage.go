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

package semconv

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	semconvURLLabel = "__semconv_url__"
	schemaURLLabel  = "__schema_url__"

	// maxStorageFanOut bounds variant reads before any result allocation.
	maxStorageFanOut = 32

	// maxCanonicalSeriesMaterialization bounds input series consumed by all
	// reorder-required variants in one selection.
	maxCanonicalSeriesMaterialization = 1 << 16
)

// ErrSchemaWarning is the sentinel chained into every warning emitted by the
// semconv-aware querier. It wraps annotations.PromQLWarning so warnings are
// surfaced as PromQL warnings by util/annotations.AsStrings, and so callers
// can recognise the warning class via errors.Is(err, ErrSchemaWarning).
var ErrSchemaWarning = fmt.Errorf("%w: semconv", annotations.PromQLWarning)

var (
	errCanonicalSeriesMaterialization = errors.New("semconv canonical series materialization limit exceeded")
	errSchemaAwareSearchUnsupported   = errors.New("schema-aware search does not support __semconv_url__ or __schema_url__ matchers")
)

// schemaWarning wraps msg in the ErrSchemaWarning sentinel so the resulting error
// is classified as a PromQL warning when surfaced through Annotations.
func schemaWarning(msg string) error {
	return fmt.Errorf("%w %s", ErrSchemaWarning, msg)
}

// AwareStorage wraps the given storage so that PromQL queries carrying a
// __semconv_url__ or __schema_url__ matcher are answered by fanning out
// across the historical metric and attribute names declared by the referenced
// semconv/OTel schema. Results are merged so callers observe a single
// canonical naming. Queries without those matchers are passed through
// unchanged.
func AwareStorage(s storage.Storage) storage.Storage {
	return newAwareStorage(s, newSchemaEngine(embeddedRegistry))
}

// AwareStorageWithRegistry behaves like AwareStorage but resolves __semconv_url__
// and __schema_url__ matchers against an operator-provided registry instead of
// the embedded one, which it fully replaces. files holds the registry-root files
// keyed by base name (e.g. "registry.yaml", "1.0.0"). It returns an error if
// files is not a valid registry (empty, or a file fails to parse as the semconv
// or OTel schema its name implies), so callers can fail fast at startup.
func AwareStorageWithRegistry(s storage.Storage, files map[string][]byte) (storage.Storage, error) {
	if err := validateRegistryFiles(files); err != nil {
		return nil, err
	}
	return newAwareStorage(s, newSchemaEngine(newRegistrySource(files))), nil
}

type awareStorage struct {
	storage.Storage

	engine               *schemaEngine
	canonicalSeriesLimit int
}

func newAwareStorage(s storage.Storage, engine *schemaEngine) *awareStorage {
	return &awareStorage{
		Storage:              s,
		engine:               engine,
		canonicalSeriesLimit: maxCanonicalSeriesMaterialization,
	}
}

func (s *awareStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	aware := &awareQuerier{Querier: q, engine: s.engine, canonicalSeriesLimit: s.canonicalSeriesLimit}
	if searcher, ok := q.(storage.Searcher); ok {
		return &awareSearchQuerier{awareQuerier: aware, searcher: searcher}, nil
	}
	return aware, nil
}

func (s *awareStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	aware := &awareChunkQuerier{ChunkQuerier: q, engine: s.engine, canonicalSeriesLimit: s.canonicalSeriesLimit}
	if searcher, ok := q.(storage.Searcher); ok {
		return &awareSearchChunkQuerier{awareChunkQuerier: aware, searcher: searcher}, nil
	}
	return aware, nil
}

type awareSearchQuerier struct {
	*awareQuerier
	searcher storage.Searcher
}

type awareSearchChunkQuerier struct {
	*awareChunkQuerier
	searcher storage.Searcher
}

func (q *awareSearchQuerier) SearchLabelNames(ctx context.Context, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return searchLabelNames(ctx, q.searcher, hints, matchers)
}

func (q *awareSearchQuerier) SearchLabelValues(ctx context.Context, name string, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return searchLabelValues(ctx, q.searcher, name, hints, matchers)
}

func (q *awareSearchChunkQuerier) SearchLabelNames(ctx context.Context, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return searchLabelNames(ctx, q.searcher, hints, matchers)
}

func (q *awareSearchChunkQuerier) SearchLabelValues(ctx context.Context, name string, hints *storage.SearchHints, matchers ...*labels.Matcher) storage.SearchResultSet {
	return searchLabelValues(ctx, q.searcher, name, hints, matchers)
}

func searchLabelNames(ctx context.Context, searcher storage.Searcher, hints *storage.SearchHints, matchers []*labels.Matcher) storage.SearchResultSet {
	if hasReservedMatcher(matchers) {
		return storage.ErrSearchResultSet(errSchemaAwareSearchUnsupported)
	}
	return searcher.SearchLabelNames(ctx, hints, matchers...)
}

func searchLabelValues(ctx context.Context, searcher storage.Searcher, name string, hints *storage.SearchHints, matchers []*labels.Matcher) storage.SearchResultSet {
	if hasReservedMatcher(matchers) {
		return storage.ErrSearchResultSet(errSchemaAwareSearchUnsupported)
	}
	return searcher.SearchLabelValues(ctx, name, hints, matchers...)
}

func hasReservedMatcher(matchers []*labels.Matcher) bool {
	for _, matcher := range matchers {
		if isReservedLabel(matcher.Name) {
			return true
		}
	}
	return false
}

// classifyMatchers inspects matchers for the reserved __semconv_url__ and
// __schema_url__ labels and decides how the query is handled. A non-empty
// warning means pass through and annotate the result. fanout=true means the
// caller should fan out via findMatcherVariants; fanout=false with an empty
// warning means a plain passthrough (no schematization was requested).
//
// __schema_url__ triggers schema-version rename fan-out and requires
// __semconv_url__ (the registry source); __semconv_url__ on its own has no
// effect and is reported as such, rather than silently doing nothing.
func classifyMatchers(matchers []*labels.Matcher) (semconvURL, schemaURL, warning string, fanout bool) {
	dup := func(label string) string {
		return fmt.Sprintf("%s matcher was used more than once, schematization logic is skipped for %v", label, matchers)
	}
	ambiguous := func(label string) string {
		return fmt.Sprintf("%s matcher is ambiguous (not equal type), schematization logic is skipped for %v", label, matchers)
	}
	for _, m := range matchers {
		switch m.Name {
		case semconvURLLabel:
			if semconvURL != "" {
				return "", "", dup(semconvURLLabel), false
			}
			if m.Type != labels.MatchEqual {
				return "", "", ambiguous(semconvURLLabel), false
			}
			semconvURL = m.Value
		case schemaURLLabel:
			if schemaURL != "" {
				return "", "", dup(schemaURLLabel), false
			}
			if m.Type != labels.MatchEqual {
				return "", "", ambiguous(schemaURLLabel), false
			}
			schemaURL = m.Value
		}
	}

	if semconvURL == "" {
		if schemaURL != "" {
			return "", "", fmt.Sprintf("__schema_url__ requires __semconv_url__, schematization logic is skipped for %v", matchers), false
		}
		return "", "", "", false // Nothing requested.
	}

	if schemaURL == "" {
		return "", "", fmt.Sprintf("__semconv_url__ alone has no effect; add __schema_url__ to fan out, schematization logic is skipped for %v", matchers), false
	}

	return semconvURL, schemaURL, "", true
}

// variantErrorWarning formats the passthrough warning for a findMatcherVariants failure.
func variantErrorWarning(matchers []*labels.Matcher, err error) string {
	return fmt.Sprintf("failed to find variants, schematization logic is skipped for %v: %v", matchers, err)
}

func isHardVariantError(err error) bool {
	return errors.Is(err, errMetricNameAnchor) || errors.Is(err, errSchemaExpansion) || errors.Is(err, errAmbiguousSchemaRename) || errors.Is(err, errUnsafeSchemaMatcher)
}

func storageFanOutError(kind string, jobs int) error {
	return fmt.Errorf("%w: %s requires at least %d storage jobs, limit is %d", errSchemaExpansion, kind, jobs, maxStorageFanOut)
}

// isReservedLabel reports whether name is one of the wrapper's reserved matcher
// labels.
func isReservedLabel(name string) bool {
	return name == semconvURLLabel || name == schemaURLLabel
}

// stripReservedLabels returns matchers without the wrapper's reserved labels so
// a passthrough query behaves as if the wrapper were absent (rather than
// matching the never-present reserved labels and returning nothing). It returns
// the input unchanged when no reserved label is present, so the common path
// allocates nothing.
func stripReservedLabels(matchers []*labels.Matcher) []*labels.Matcher {
	hasReserved := false
	for _, m := range matchers {
		if isReservedLabel(m.Name) {
			hasReserved = true
			break
		}
	}
	if !hasReserved {
		return matchers
	}
	out := make([]*labels.Matcher, 0, len(matchers))
	for _, m := range matchers {
		if !isReservedLabel(m.Name) {
			out = append(out, m)
		}
	}
	return out
}

func storedReservedLabel(lbls labels.Labels) (string, bool) {
	if lbls.Has(schemaURLLabel) {
		return schemaURLLabel, true
	}
	if lbls.Has(semconvURLLabel) {
		return semconvURLLabel, true
	}
	return "", false
}

type canonicalSeriesBudget struct {
	kind      string
	limit     int
	remaining int
}

func newCanonicalSeriesBudget(kind string, limit int) *canonicalSeriesBudget {
	return &canonicalSeriesBudget{kind: kind, limit: limit, remaining: limit}
}

func (b *canonicalSeriesBudget) take() error {
	if b.remaining == 0 {
		return fmt.Errorf("%w: %s requires more than %d input series", errCanonicalSeriesMaterialization, b.kind, b.limit)
	}
	b.remaining--
	return nil
}

// reverseLabelName returns the canonical label name for n, looked up in the
// resolved variant's mapping. If no mapping applies n is returned unchanged.
// Note: the metric name (model.MetricNameLabel) is not reverse-mapped here —
// it is correctly reported as a label name by underlying storage. Value-level
// canonicalisation for __name__ is handled in queryLabelValues.
func reverseLabelName(mapping *labelMapping, n string) string {
	if mapping == nil {
		return n
	}
	if canon, ok := mapping.translatedLabels[n]; ok {
		return canon
	}
	return n
}

func cloneSelectHints(hints *storage.SelectHints) *storage.SelectHints {
	if hints == nil {
		return nil
	}
	cloned := *hints
	cloned.Grouping = slices.Clone(hints.Grouping)
	cloned.ProjectionLabels = slices.Clone(hints.ProjectionLabels)
	return &cloned
}

func variantSelectHints(hints *storage.SelectHints, resort, sharded, postFilter bool) *storage.SelectHints {
	cloned := cloneSelectHints(hints)
	if cloned == nil {
		return nil
	}

	// A physical projection cannot preserve canonical label aliases, a
	// canonical __series_hash__, or reserved-label validation, so schema-aware
	// reads that inspect labels must fetch the full label set.
	cloned.ProjectionLabels = nil
	cloned.ProjectionInclude = false
	// Aggregation hints refer to canonical label names and cannot be applied to
	// physical variants without changing query semantics. The API's metadata-only
	// series token is independent of label names and avoids loading samples.
	if cloned.Func != "series" {
		cloned.Func = ""
	}
	cloned.Grouping = nil
	cloned.By = false
	if resort || sharded || postFilter {
		cloned.Limit = 0
	}
	if sharded {
		cloned.ShardCount = 0
		cloned.ShardIndex = 0
	}
	return cloned
}

func cloneLabelHints(hints *storage.LabelHints) *storage.LabelHints {
	if hints == nil {
		return nil
	}
	cloned := *hints
	return &cloned
}

func selectLimit(hints *storage.SelectHints) int {
	if hints == nil || hints.Limit <= 0 {
		return 0
	}
	return hints.Limit
}

func labelLimit(hints *storage.LabelHints) int {
	if hints == nil || hints.Limit <= 0 {
		return 0
	}
	return hints.Limit
}

func truncateStrings(values []string, limit int) []string {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

type awareQuerier struct {
	storage.Querier

	engine               *schemaEngine
	canonicalSeriesLimit int
}

func (q *awareQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	semconvURL, schemaURL, warning, fanout := classifyMatchers(matchers)
	passthrough := stripReservedLabels(matchers)
	if warning != "" {
		return annotateSeriesSet(q.Querier.Select(ctx, sortSeries, hints, passthrough...), warning)
	}
	if !fanout {
		return q.Querier.Select(ctx, sortSeries, hints, passthrough...)
	}

	variants, qCtx, err := q.engine.findMatcherVariants(semconvURL, schemaURL, matchers)
	if err != nil {
		if isHardVariantError(err) {
			return storage.ErrSeriesSet(err)
		}
		return annotateSeriesSet(
			q.Querier.Select(ctx, sortSeries, hints, passthrough...),
			variantErrorWarning(matchers, err),
		)
	}
	if len(variants) == 1 && variants[0].mapping == nil {
		// No transformation needed: passthrough.
		return q.Querier.Select(ctx, sortSeries, hints, passthrough...)
	}
	if err := ctx.Err(); err != nil {
		return annotateSeriesSet(storage.ErrSeriesSet(err), qCtx.warnings...)
	}
	if len(variants) == 1 && identityMatcherVariant(variants[0]) {
		variant := variants[0]
		variantHints := variantSelectHints(hints, false, false, len(variant.canonicalMatchers) > 0)
		set := storage.SeriesSet(&awareSeriesSet{
			SeriesSet:         q.Querier.Select(ctx, sortSeries, variantHints, slices.Clone(variant.matchers)...),
			mapping:           variant.mapping,
			canonicalMatchers: variant.canonicalMatchers,
		})
		if len(qCtx.warnings) > 0 {
			set = annotateSeriesSet(set, qCtx.warnings...)
		}
		return set
	}
	if len(variants) > maxStorageFanOut {
		err := storageFanOutError("series fan-out", len(variants))
		return annotateSeriesSet(storage.ErrSeriesSet(err), qCtx.warnings...)
	}
	limit := selectLimit(hints)
	var shardCount, shardIndex uint64
	if hints != nil {
		shardCount = hints.ShardCount
		shardIndex = hints.ShardIndex
	}
	budget := newCanonicalSeriesBudget("series reordering", q.canonicalSeriesLimit)

	seriesSets := make([]storage.SeriesSet, len(variants))
	var sortedSets []*sortedSeriesSet
	// All variants must share the querier's isolation snapshot. Querier methods
	// are not required to support concurrent calls, so issue Select calls serially.
	// Iteration is deferred until every selector on the querier can be scheduled.
	for i, variant := range variants {
		if err := ctx.Err(); err != nil {
			seriesSets[i] = storage.ErrSeriesSet(err)
			continue
		}
		matchersCopy := slices.Clone(variant.matchers)
		resort := mappingNeedsResort(variant.mapping)
		postFilter := len(variant.canonicalMatchers) > 0
		variantHints := variantSelectHints(hints, resort, shardCount > 0, postFilter)
		awareSet := &awareSeriesSet{
			SeriesSet:         q.Querier.Select(ctx, true, variantHints, matchersCopy...),
			mapping:           variant.mapping,
			canonicalMatchers: variant.canonicalMatchers,
			alwaysTransform:   metricMappingChanges(variant),
		}
		if postFilter {
			awareSet.budget = budget
		}
		if !resort {
			seriesSets[i] = awareSet
			continue
		}
		awareSet.budget = budget
		sortLimit := limit
		if shardCount > 0 {
			sortLimit = 0
		}
		sorted := newSortedSeriesSet(awareSet, sortLimit)
		sortedSets = append(sortedSets, sorted)
		seriesSets[i] = sorted
	}
	merged := &lazySeriesSet{init: func() storage.SeriesSet {
		if err := ctx.Err(); err != nil {
			return storage.ErrSeriesSet(err)
		}
		for _, sorted := range sortedSets {
			if err := ctx.Err(); err != nil {
				return storage.ErrSeriesSet(err)
			}
			sorted.load()
			if sorted.Err() != nil {
				return sorted
			}
		}
		set := storage.NewMergeSeriesSet(seriesSets, 0, storage.ChainedSeriesMerge)
		set = shardSeriesSet(set, shardCount, shardIndex)
		return limitSeriesSet(set, limit)
	}}
	if len(qCtx.warnings) > 0 {
		return annotateSeriesSet(merged, qCtx.warnings...)
	}
	return merged
}

func (q *awareQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return queryLabelNames(ctx, q.Querier, q.engine, hints, matchers)
}

func (q *awareQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return queryLabelValues(ctx, q.Querier, q.engine, name, hints, matchers)
}

type awareChunkQuerier struct {
	storage.ChunkQuerier

	engine               *schemaEngine
	canonicalSeriesLimit int
}

func (q *awareChunkQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.ChunkSeriesSet {
	semconvURL, schemaURL, warning, fanout := classifyMatchers(matchers)
	passthrough := stripReservedLabels(matchers)
	if warning != "" {
		return annotateChunkSeriesSet(q.ChunkQuerier.Select(ctx, sortSeries, hints, passthrough...), warning)
	}
	if !fanout {
		return q.ChunkQuerier.Select(ctx, sortSeries, hints, passthrough...)
	}

	variants, qCtx, err := q.engine.findMatcherVariants(semconvURL, schemaURL, matchers)
	if err != nil {
		if isHardVariantError(err) {
			return storage.ErrChunkSeriesSet(err)
		}
		return annotateChunkSeriesSet(
			q.ChunkQuerier.Select(ctx, sortSeries, hints, passthrough...),
			variantErrorWarning(matchers, err),
		)
	}
	if len(variants) == 1 && variants[0].mapping == nil {
		return q.ChunkQuerier.Select(ctx, sortSeries, hints, passthrough...)
	}
	if err := ctx.Err(); err != nil {
		return annotateChunkSeriesSet(storage.ErrChunkSeriesSet(err), qCtx.warnings...)
	}
	if len(variants) == 1 && identityMatcherVariant(variants[0]) {
		variant := variants[0]
		variantHints := variantSelectHints(hints, false, false, len(variant.canonicalMatchers) > 0)
		set := storage.ChunkSeriesSet(&awareChunkSeriesSet{
			ChunkSeriesSet:    q.ChunkQuerier.Select(ctx, sortSeries, variantHints, slices.Clone(variant.matchers)...),
			mapping:           variant.mapping,
			canonicalMatchers: variant.canonicalMatchers,
		})
		if len(qCtx.warnings) > 0 {
			set = annotateChunkSeriesSet(set, qCtx.warnings...)
		}
		return set
	}
	if len(variants) > maxStorageFanOut {
		err := storageFanOutError("chunk series fan-out", len(variants))
		return annotateChunkSeriesSet(storage.ErrChunkSeriesSet(err), qCtx.warnings...)
	}
	limit := selectLimit(hints)
	var shardCount, shardIndex uint64
	if hints != nil {
		shardCount = hints.ShardCount
		shardIndex = hints.ShardIndex
	}
	budget := newCanonicalSeriesBudget("chunk series reordering", q.canonicalSeriesLimit)

	chunkSeriesSets := make([]storage.ChunkSeriesSet, len(variants))
	var sortedSets []*sortedChunkSeriesSet
	// Keep all access to the shared underlying querier serial and defer iteration;
	// see Select above.
	for i, variant := range variants {
		if err := ctx.Err(); err != nil {
			chunkSeriesSets[i] = storage.ErrChunkSeriesSet(err)
			continue
		}
		matchersCopy := slices.Clone(variant.matchers)
		resort := mappingNeedsResort(variant.mapping)
		postFilter := len(variant.canonicalMatchers) > 0
		variantHints := variantSelectHints(hints, resort, shardCount > 0, postFilter)
		awareSet := &awareChunkSeriesSet{
			ChunkSeriesSet:    q.ChunkQuerier.Select(ctx, true, variantHints, matchersCopy...),
			mapping:           variant.mapping,
			canonicalMatchers: variant.canonicalMatchers,
			alwaysTransform:   metricMappingChanges(variant),
		}
		if postFilter {
			awareSet.budget = budget
		}
		if !resort {
			chunkSeriesSets[i] = awareSet
			continue
		}
		awareSet.budget = budget
		sortLimit := limit
		if shardCount > 0 {
			sortLimit = 0
		}
		sorted := newSortedChunkSeriesSet(awareSet, sortLimit)
		sortedSets = append(sortedSets, sorted)
		chunkSeriesSets[i] = sorted
	}
	merged := &lazyChunkSeriesSet{init: func() storage.ChunkSeriesSet {
		if err := ctx.Err(); err != nil {
			return storage.ErrChunkSeriesSet(err)
		}
		for _, sorted := range sortedSets {
			if err := ctx.Err(); err != nil {
				return storage.ErrChunkSeriesSet(err)
			}
			sorted.load()
			if sorted.Err() != nil {
				return sorted
			}
		}
		set := storage.NewMergeChunkSeriesSet(chunkSeriesSets, 0, storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge))
		set = shardChunkSeriesSet(set, shardCount, shardIndex)
		return limitChunkSeriesSet(set, limit)
	}}
	if len(qCtx.warnings) > 0 {
		return annotateChunkSeriesSet(merged, qCtx.warnings...)
	}
	return merged
}

func (q *awareChunkQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return queryLabelNames(ctx, q.ChunkQuerier, q.engine, hints, matchers)
}

func (q *awareChunkQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return queryLabelValues(ctx, q.ChunkQuerier, q.engine, name, hints, matchers)
}

// labelQuerier captures the label-query surface that both storage.Querier and
// storage.ChunkQuerier expose through the embedded storage.LabelQuerier.
type labelQuerier interface {
	LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error)
	LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error)
}

type labelValueJob struct {
	variant matcherVariant
	alias   string
}

func buildLabelValueJobs(variants []matcherVariant, name string) ([]labelValueJob, error) {
	return buildLabelValueJobsUpTo(variants, name, maxSchemaExpansion)
}

func buildLabelValueJobsUpTo(variants []matcherVariant, name string, maxJobs int) ([]labelValueJob, error) {
	canonicalNames := []string{name}
	seenCanonical := map[string]struct{}{name: {}}
	for _, variant := range variants {
		canonical := reverseLabelName(variant.mapping, name)
		if _, exists := seenCanonical[canonical]; exists {
			continue
		}
		seenCanonical[canonical] = struct{}{}
		canonicalNames = append(canonicalNames, canonical)
	}

	jobs := make([]labelValueJob, 0, min(len(variants), maxJobs))
	for _, variant := range variants {
		seenAliases := map[string]struct{}{}
		for _, canonical := range canonicalNames {
			aliases := []string{canonical}
			if variant.mapping != nil {
				aliases = variant.mapping.aliasesOf(canonical)
			}
			for _, alias := range aliases {
				if _, exists := seenAliases[alias]; exists {
					continue
				}
				if len(jobs) >= maxJobs {
					if maxJobs == maxSchemaExpansion {
						return nil, schemaExpansionError("label value jobs")
					}
					return nil, storageFanOutError("label value fan-out", len(jobs)+1)
				}
				seenAliases[alias] = struct{}{}
				jobs = append(jobs, labelValueJob{variant: variant, alias: alias})
			}
		}
	}
	return jobs, nil
}

func queryLabelNames(ctx context.Context, q labelQuerier, e *schemaEngine, hints *storage.LabelHints, matchers []*labels.Matcher) ([]string, annotations.Annotations, error) {
	semconvURL, schemaURL, warning, fanout := classifyMatchers(matchers)
	passthrough := stripReservedLabels(matchers)
	if warning != "" {
		names, anns, err := q.LabelNames(ctx, hints, passthrough...)
		return names, anns.Add(schemaWarning(warning)), err
	}
	if !fanout {
		return q.LabelNames(ctx, hints, passthrough...)
	}

	variants, qCtx, err := e.findMatcherVariants(semconvURL, schemaURL, matchers)
	if err != nil {
		if isHardVariantError(err) {
			return nil, nil, err
		}
		names, anns, lerr := q.LabelNames(ctx, hints, passthrough...)
		return names, anns.Add(schemaWarning(variantErrorWarning(matchers, err))), lerr
	}
	if len(variants) == 1 && variants[0].mapping == nil {
		return q.LabelNames(ctx, hints, passthrough...)
	}
	if err := ctx.Err(); err != nil {
		return nil, addWarnings(nil, qCtx), err
	}
	if len(variants) > maxStorageFanOut {
		err := storageFanOutError("label name fan-out", len(variants))
		return nil, addWarnings(nil, qCtx), err
	}
	limit := labelLimit(hints)

	type partial struct {
		names   []string
		anns    annotations.Annotations
		err     error
		mapping *labelMapping
	}
	results := make([]partial, len(variants))
	for i, variant := range variants {
		if err := ctx.Err(); err != nil {
			results[i].err = err
			continue
		}
		matchersCopy := slices.Clone(variant.matchers)
		variantHints := cloneLabelHints(hints)
		if variantHints != nil {
			variantHints.Limit = 0
		}
		n, a, err := q.LabelNames(ctx, variantHints, matchersCopy...)
		results[i] = partial{names: n, anns: a, err: err, mapping: variant.mapping}
	}

	seen := make(map[string]struct{})
	var combined []string
	var combinedAnns annotations.Annotations
	var errs []error
	for _, p := range results {
		if p.err != nil {
			errs = append(errs, p.err)
		}
		combinedAnns.Merge(p.anns)
		for _, n := range p.names {
			if isReservedLabel(n) {
				// Select strips these from the series it returns, so reporting
				// them as label names would advertise labels no series carries.
				continue
			}
			canonical := reverseLabelName(p.mapping, n)
			if _, ok := seen[canonical]; ok {
				continue
			}
			seen[canonical] = struct{}{}
			combined = append(combined, canonical)
		}
	}
	slices.Sort(combined)
	return truncateStrings(combined, limit), addWarnings(combinedAnns, qCtx), errors.Join(errs...)
}

func queryLabelValues(ctx context.Context, q labelQuerier, e *schemaEngine, name string, hints *storage.LabelHints, matchers []*labels.Matcher) ([]string, annotations.Annotations, error) {
	semconvURL, schemaURL, warning, fanout := classifyMatchers(matchers)
	passthrough := stripReservedLabels(matchers)
	if warning != "" {
		values, anns, err := q.LabelValues(ctx, name, hints, passthrough...)
		return values, anns.Add(schemaWarning(warning)), err
	}
	if !fanout {
		return q.LabelValues(ctx, name, hints, passthrough...)
	}

	variants, qCtx, err := e.findMatcherVariants(semconvURL, schemaURL, matchers)
	if err != nil {
		if isHardVariantError(err) {
			return nil, nil, err
		}
		values, anns, lerr := q.LabelValues(ctx, name, hints, passthrough...)
		return values, anns.Add(schemaWarning(variantErrorWarning(matchers, err))), lerr
	}
	if isReservedLabel(name) {
		if err := ctx.Err(); err != nil {
			return nil, addWarnings(nil, qCtx), err
		}
		return []string{}, addWarnings(nil, qCtx), nil
	}
	if len(variants) == 1 && variants[0].mapping == nil {
		return q.LabelValues(ctx, name, hints, passthrough...)
	}
	if err := ctx.Err(); err != nil {
		return nil, addWarnings(nil, qCtx), err
	}

	// Each variant canonicalises the requested attribute through its own mapping,
	// then queries every alias resolved for that lineage. For __name__ there are
	// no attribute aliases, and its values are collapsed to the canonical metric.
	jobs, err := buildLabelValueJobsUpTo(variants, name, maxStorageFanOut)
	if err != nil {
		return nil, addWarnings(nil, qCtx), err
	}

	type partial struct {
		values  []string
		anns    annotations.Annotations
		err     error
		mapping *labelMapping
	}
	results := make([]partial, len(jobs))
	for i, job := range jobs {
		if err := ctx.Err(); err != nil {
			results[i].err = err
			continue
		}
		matchersCopy := slices.Clone(job.variant.matchers)
		v, a, err := q.LabelValues(ctx, job.alias, cloneLabelHints(hints), matchersCopy...)
		results[i] = partial{values: v, anns: a, err: err, mapping: job.variant.mapping}
	}

	// When the caller asked for values of __name__, every variant's results
	// are different escapings of the same canonical metric; collapse them.
	metricNameQuery := name == model.MetricNameLabel

	seen := make(map[string]struct{})
	var combined []string
	var combinedAnns annotations.Annotations
	var errs []error
	for _, p := range results {
		if p.err != nil {
			errs = append(errs, p.err)
		}
		combinedAnns.Merge(p.anns)
		for _, v := range p.values {
			if metricNameQuery && p.mapping != nil {
				v = p.mapping.translatedMetric
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			combined = append(combined, v)
		}
	}
	slices.Sort(combined)
	return truncateStrings(combined, labelLimit(hints)), addWarnings(combinedAnns, qCtx), errors.Join(errs...)
}

type annotatedSeriesSet struct {
	storage.SeriesSet

	warnings []string
}

func annotateSeriesSet(s storage.SeriesSet, warnings ...string) storage.SeriesSet {
	return &annotatedSeriesSet{warnings: warnings, SeriesSet: s}
}

func (s *annotatedSeriesSet) Warnings() annotations.Annotations {
	got := s.SeriesSet.Warnings()
	for _, w := range s.warnings {
		got = got.Add(schemaWarning(w))
	}
	return got
}

// addWarnings merges the query-resolution warnings collected in qCtx into anns.
func addWarnings(anns annotations.Annotations, qCtx queryContext) annotations.Annotations {
	for _, w := range qCtx.warnings {
		anns = anns.Add(schemaWarning(w))
	}
	return anns
}

type annotatedChunkSeriesSet struct {
	storage.ChunkSeriesSet

	warnings []string
}

func annotateChunkSeriesSet(s storage.ChunkSeriesSet, warnings ...string) storage.ChunkSeriesSet {
	return &annotatedChunkSeriesSet{warnings: warnings, ChunkSeriesSet: s}
}

func (s *annotatedChunkSeriesSet) Warnings() annotations.Annotations {
	got := s.ChunkSeriesSet.Warnings()
	for _, w := range s.warnings {
		got = got.Add(schemaWarning(w))
	}
	return got
}

type lazySeriesSet struct {
	init func() storage.SeriesSet
	set  storage.SeriesSet
}

func (s *lazySeriesSet) Next() bool {
	if s.set == nil {
		s.set = s.init()
		s.init = nil
	}
	return s.set.Next()
}

func (s *lazySeriesSet) At() storage.Series {
	if s.set == nil {
		return nil
	}
	return s.set.At()
}

func (s *lazySeriesSet) Err() error {
	if s.set == nil {
		return nil
	}
	return s.set.Err()
}

func (s *lazySeriesSet) Warnings() annotations.Annotations {
	if s.set == nil {
		return nil
	}
	return s.set.Warnings()
}

type lazyChunkSeriesSet struct {
	init func() storage.ChunkSeriesSet
	set  storage.ChunkSeriesSet
}

func (s *lazyChunkSeriesSet) Next() bool {
	if s.set == nil {
		s.set = s.init()
		s.init = nil
	}
	return s.set.Next()
}

func (s *lazyChunkSeriesSet) At() storage.ChunkSeries {
	if s.set == nil {
		return nil
	}
	return s.set.At()
}

func (s *lazyChunkSeriesSet) Err() error {
	if s.set == nil {
		return nil
	}
	return s.set.Err()
}

func (s *lazyChunkSeriesSet) Warnings() annotations.Annotations {
	if s.set == nil {
		return nil
	}
	return s.set.Warnings()
}

type shardedSeriesSet struct {
	storage.SeriesSet
	count uint64
	index uint64
}

func shardSeriesSet(set storage.SeriesSet, count, index uint64) storage.SeriesSet {
	if count == 0 {
		return set
	}
	return &shardedSeriesSet{SeriesSet: set, count: count, index: index}
}

func (s *shardedSeriesSet) Next() bool {
	for s.SeriesSet.Next() {
		if labels.StableHash(s.SeriesSet.At().Labels())%s.count == s.index {
			return true
		}
	}
	return false
}

type shardedChunkSeriesSet struct {
	storage.ChunkSeriesSet
	count uint64
	index uint64
}

func shardChunkSeriesSet(set storage.ChunkSeriesSet, count, index uint64) storage.ChunkSeriesSet {
	if count == 0 {
		return set
	}
	return &shardedChunkSeriesSet{ChunkSeriesSet: set, count: count, index: index}
}

func (s *shardedChunkSeriesSet) Next() bool {
	for s.ChunkSeriesSet.Next() {
		if labels.StableHash(s.ChunkSeriesSet.At().Labels())%s.count == s.index {
			return true
		}
	}
	return false
}

type limitedSeriesSet struct {
	storage.SeriesSet
	remaining int
}

func limitSeriesSet(set storage.SeriesSet, limit int) storage.SeriesSet {
	if limit <= 0 {
		return set
	}
	return &limitedSeriesSet{SeriesSet: set, remaining: limit}
}

func (s *limitedSeriesSet) Next() bool {
	if s.remaining == 0 || !s.SeriesSet.Next() {
		return false
	}
	s.remaining--
	return true
}

type limitedChunkSeriesSet struct {
	storage.ChunkSeriesSet
	remaining int
}

func limitChunkSeriesSet(set storage.ChunkSeriesSet, limit int) storage.ChunkSeriesSet {
	if limit <= 0 {
		return set
	}
	return &limitedChunkSeriesSet{ChunkSeriesSet: set, remaining: limit}
}

func (s *limitedChunkSeriesSet) Next() bool {
	if s.remaining == 0 || !s.ChunkSeriesSet.Next() {
		return false
	}
	s.remaining--
	return true
}

// mappingNeedsResort reports whether attribute rewriting can change a variant's
// series order or collapse distinct input labels.
func mappingNeedsResort(mapping *labelMapping) bool {
	return mapping != nil && len(mapping.translatedLabels) > 0
}

func identityMatcherVariant(variant matcherVariant) bool {
	if variant.mapping == nil || len(variant.mapping.translatedLabels) > 0 || len(variant.canonicalMatchers) > 0 {
		return false
	}
	return !metricMappingChanges(variant)
}

func metricMappingChanges(variant matcherVariant) bool {
	if variant.mapping == nil {
		return false
	}
	metricName, err := extractMetricName(variant.matchers)
	return err != nil || metricName != variant.mapping.translatedMetric
}

// sortAndChain returns in sorted by labels, with each run of series carrying
// identical labels collapsed into one via merge.
//
// Both are needed because rewriting or deleting labels can reorder a set and can
// make two series collide. A variant queries one naming era and its series come
// back sorted by that era's labels; rewriting an attribute or removing a stored
// query-control label can move it in the ordering. storage.NewMergeSeriesSet
// assumes each input reports strictly increasing labels, so it would otherwise
// emit reordered series out of order and collided ones twice.
func sortAndChain[T interface{ Labels() labels.Labels }](in []T, merge func(...T) T) []T {
	slices.SortStableFunc(in, func(a, b T) int {
		return labels.Compare(a.Labels(), b.Labels())
	})
	// A fresh slice, deliberately not in[:0]: the merge functions retain the slice
	// they are handed and read it only when the merged series is iterated, so
	// compacting in place would write a merged series back into the very range its
	// own chain still points at, and iterating it would recurse forever.
	out := make([]T, 0, len(in))
	for i := 0; i < len(in); {
		j := i + 1
		for j < len(in) && labels.Equal(in[j].Labels(), in[i].Labels()) {
			j++
		}
		if j-i == 1 {
			out = append(out, in[i])
		} else {
			out = append(out, merge(in[i:j]...))
		}
		i = j
	}
	return out
}

type topKEntry[T interface{ Labels() labels.Labels }] struct {
	labels labels.Labels
	series []T
	hash   uint64
}

// topKMaxHeap keeps the greatest retained label set at index zero.
type topKMaxHeap[T interface{ Labels() labels.Labels }] []*topKEntry[T]

func (h topKMaxHeap[T]) Len() int { return len(h) }

func (h topKMaxHeap[T]) Less(i, j int) bool {
	return labels.Compare(h[i].labels, h[j].labels) > 0
}

func (h topKMaxHeap[T]) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *topKMaxHeap[T]) Push(value any) {
	*h = append(*h, value.(*topKEntry[T]))
}

func (h *topKMaxHeap[T]) Pop() any {
	old := *h
	last := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	return last
}

func removeTopKEntry[T interface{ Labels() labels.Labels }](entries map[uint64][]*topKEntry[T], removed *topKEntry[T]) {
	bucket := entries[removed.hash]
	for i, entry := range bucket {
		if entry != removed {
			continue
		}
		bucket[i] = bucket[len(bucket)-1]
		bucket = bucket[:len(bucket)-1]
		if len(bucket) == 0 {
			delete(entries, removed.hash)
		} else {
			entries[removed.hash] = bucket
		}
		return
	}
}

// collectAndChainTopK drains input and returns the smallest limit distinct label
// sets. Capacity grows with observed groups rather than the caller's limit.
func collectAndChainTopK[T interface{ Labels() labels.Labels }](next func() bool, at func() T, limit int, merge func(...T) T) []T {
	retained := map[uint64][]*topKEntry[T]{}
	var h topKMaxHeap[T]
	for next() {
		series := at()
		seriesLabels := series.Labels()
		hash := seriesLabels.Hash()
		var existing *topKEntry[T]
		for _, entry := range retained[hash] {
			if labels.Equal(entry.labels, seriesLabels) {
				existing = entry
				break
			}
		}
		if existing != nil {
			existing.series = append(existing.series, series)
			continue
		}

		if len(h) == limit && labels.Compare(seriesLabels, h[0].labels) >= 0 {
			continue
		}
		if len(h) == limit {
			removeTopKEntry(retained, heap.Pop(&h).(*topKEntry[T]))
		}
		entry := &topKEntry[T]{labels: seriesLabels, series: []T{series}, hash: hash}
		retained[hash] = append(retained[hash], entry)
		heap.Push(&h, entry)
	}

	out := make([]T, 0, len(h))
	for _, entry := range h {
		if len(entry.series) == 1 {
			out = append(out, entry.series[0])
		} else {
			out = append(out, merge(entry.series...))
		}
	}
	slices.SortFunc(out, func(a, b T) int {
		return labels.Compare(a.Labels(), b.Labels())
	})
	return out
}

// sortedSeriesSet re-sorts a series set whose labels have been rewritten, so it
// can be fed to storage.NewMergeSeriesSet.
//
// It drains the whole set on first use because the rewritten order is not known
// sooner. With a positive limit it retains only the smallest canonical label
// groups; without one it buffers every series handle, but never their samples.
type sortedSeriesSet struct {
	storage.SeriesSet

	series []storage.Series
	idx    int
	limit  int
	loaded bool
}

func newSortedSeriesSet(s storage.SeriesSet, limit int) *sortedSeriesSet {
	return &sortedSeriesSet{SeriesSet: s, idx: -1, limit: limit}
}

// load drains and sorts the underlying set. It must not overlap access through
// another set from the same querier.
func (s *sortedSeriesSet) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	if s.limit > 0 {
		s.series = collectAndChainTopK(s.SeriesSet.Next, s.SeriesSet.At, s.limit, storage.ChainedSeriesMerge)
		if s.Err() != nil {
			s.series = nil
		}
		return
	}
	for s.SeriesSet.Next() {
		s.series = append(s.series, s.SeriesSet.At())
	}
	if s.Err() != nil {
		s.series = nil
		return
	}
	s.series = sortAndChain(s.series, storage.ChainedSeriesMerge)
}

func (s *sortedSeriesSet) Next() bool {
	s.load()
	if s.Err() != nil {
		return false
	}
	s.idx++
	return s.idx < len(s.series)
}

func (s *sortedSeriesSet) At() storage.Series {
	return s.series[s.idx]
}

// sortedChunkSeriesSet is sortedSeriesSet for chunk series; see there.
type sortedChunkSeriesSet struct {
	storage.ChunkSeriesSet

	series []storage.ChunkSeries
	idx    int
	limit  int
	loaded bool
}

func newSortedChunkSeriesSet(s storage.ChunkSeriesSet, limit int) *sortedChunkSeriesSet {
	return &sortedChunkSeriesSet{ChunkSeriesSet: s, idx: -1, limit: limit}
}

// load drains and sorts the underlying set; see sortedSeriesSet.load.
func (s *sortedChunkSeriesSet) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	if s.limit > 0 {
		s.series = collectAndChainTopK(s.ChunkSeriesSet.Next, s.ChunkSeriesSet.At, s.limit, storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge))
		if s.Err() != nil {
			s.series = nil
		}
		return
	}
	for s.ChunkSeriesSet.Next() {
		s.series = append(s.series, s.ChunkSeriesSet.At())
	}
	if s.Err() != nil {
		s.series = nil
		return
	}
	s.series = sortAndChain(s.series, storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge))
}

func (s *sortedChunkSeriesSet) Next() bool {
	s.load()
	if s.Err() != nil {
		return false
	}
	s.idx++
	return s.idx < len(s.series)
}

func (s *sortedChunkSeriesSet) At() storage.ChunkSeries {
	return s.series[s.idx]
}

func matchesCanonicalMatchers(lbls labels.Labels, matchers []*labels.Matcher) bool {
	for _, matcher := range matchers {
		if !matcher.Matches(lbls.Get(matcher.Name)) {
			return false
		}
	}
	return true
}

type awareSeriesSet struct {
	storage.SeriesSet

	mapping           *labelMapping
	canonicalMatchers []*labels.Matcher
	alwaysTransform   bool
	budget            *canonicalSeriesBudget

	at  storage.Series
	err error
}

func (s *awareSeriesSet) At() storage.Series {
	return s.at
}

func (s *awareSeriesSet) Next() bool {
	if s.Err() != nil {
		return false
	}
	for s.SeriesSet.Next() {
		if s.budget != nil {
			if err := s.budget.take(); err != nil {
				s.err = err
				return false
			}
		}
		at := s.SeriesSet.At()
		if name, ok := storedReservedLabel(at.Labels()); ok {
			s.err = fmt.Errorf("schema-aware query encountered stored control label %s", name)
			return false
		}
		result := at
		lbls := at.Labels()
		if s.alwaysTransform || labelMappingChangesLabels(lbls, s.mapping) {
			transformed, err := transformOTelSchemaLabels(lbls, s.mapping)
			if err != nil {
				s.err = err
				return false
			}
			lbls = transformed
			result = &awareSeries{Series: at, lbls: lbls}
		}
		if !matchesCanonicalMatchers(lbls, s.canonicalMatchers) {
			continue
		}
		s.at = result
		return true
	}
	return false
}

func (s *awareSeriesSet) Err() error {
	return errors.Join(s.err, s.SeriesSet.Err())
}

type awareSeries struct {
	storage.Series

	lbls labels.Labels
}

func (s *awareSeries) Labels() labels.Labels {
	return s.lbls
}

type awareChunkSeriesSet struct {
	storage.ChunkSeriesSet

	mapping           *labelMapping
	canonicalMatchers []*labels.Matcher
	alwaysTransform   bool
	budget            *canonicalSeriesBudget

	at  storage.ChunkSeries
	err error
}

func (s *awareChunkSeriesSet) At() storage.ChunkSeries {
	return s.at
}

func (s *awareChunkSeriesSet) Next() bool {
	if s.Err() != nil {
		return false
	}
	for s.ChunkSeriesSet.Next() {
		if s.budget != nil {
			if err := s.budget.take(); err != nil {
				s.err = err
				return false
			}
		}
		at := s.ChunkSeriesSet.At()
		if name, ok := storedReservedLabel(at.Labels()); ok {
			s.err = fmt.Errorf("schema-aware query encountered stored control label %s", name)
			return false
		}
		result := at
		lbls := at.Labels()
		if s.alwaysTransform || labelMappingChangesLabels(lbls, s.mapping) {
			transformed, err := transformOTelSchemaLabels(lbls, s.mapping)
			if err != nil {
				s.err = err
				return false
			}
			lbls = transformed
			result = &awareChunkSeries{ChunkSeries: at, lbls: lbls}
		}
		if !matchesCanonicalMatchers(lbls, s.canonicalMatchers) {
			continue
		}
		s.at = result
		return true
	}
	return false
}

func (s *awareChunkSeriesSet) Err() error {
	return errors.Join(s.err, s.ChunkSeriesSet.Err())
}

type awareChunkSeries struct {
	storage.ChunkSeries

	lbls labels.Labels
}

func (s *awareChunkSeries) Labels() labels.Labels {
	return s.lbls
}
