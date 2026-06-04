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
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	semconvURLLabel = "__semconv_url__"
	schemaURLLabel  = "__schema_url__"

	// fanOutLimit caps the number of concurrent queries issued to the
	// underlying storage when a Select / LabelNames / LabelValues call fans
	// out across semconv variants. The schema-version fan-out can produce
	// several variants per call; without a cap, concurrent PromQL evaluation
	// would spawn unbounded goroutines. The chosen value follows the same
	// errgroup.SetLimit convention used elsewhere in the codebase (see
	// discovery/aws/rds.go).
	fanOutLimit = 16
)

// ErrSchemaWarning is the sentinel chained into every warning emitted by the
// semconv-aware querier. It wraps annotations.PromQLWarning so warnings are
// surfaced as PromQL warnings by util/annotations.AsStrings, and so callers
// can recognise the warning class via errors.Is(err, ErrSchemaWarning).
var ErrSchemaWarning = fmt.Errorf("%w: semconv", annotations.PromQLWarning)

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
	return &awareStorage{Storage: s, engine: newSchemaEngine()}
}

type awareStorage struct {
	storage.Storage

	engine *schemaEngine
}

func (s *awareStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &awareQuerier{Querier: q, engine: s.engine}, nil
}

func (s *awareStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	q, err := s.Storage.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	return &awareChunkQuerier{ChunkQuerier: q, engine: s.engine}, nil
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

// reverseLabelName returns the canonical label name for n, looked up in the
// query's labelMapping. If no mapping applies n is returned unchanged.
// Note: the metric name (model.MetricNameLabel) is not reverse-mapped here —
// it is correctly reported as a label name by underlying storage. Value-level
// canonicalisation for __name__ is handled in queryLabelValues.
func reverseLabelName(q queryContext, n string) string {
	if q.labelMapping == nil {
		return n
	}
	if canon, ok := q.labelMapping.translatedLabels[n]; ok {
		return canon
	}
	return n
}

type awareQuerier struct {
	storage.Querier

	engine *schemaEngine
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
		return annotateSeriesSet(
			q.Querier.Select(ctx, sortSeries, hints, passthrough...),
			variantErrorWarning(matchers, err),
		)
	}
	if qCtx.labelMapping == nil {
		// No transformation needed: passthrough.
		return q.Querier.Select(ctx, sortSeries, hints, passthrough...)
	}

	seriesSets := make([]storage.SeriesSet, len(variants))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOutLimit)
	for i, ms := range variants {
		g.Go(func() error {
			// We must sort for NewMergeSeriesSet to work.
			seriesSets[i] = &awareSeriesSet{
				SeriesSet: q.Querier.Select(gctx, true, hints, ms...),
				engine:    q.engine,
				qCtx:      qCtx,
			}
			return nil
		})
	}
	_ = g.Wait()
	return storage.NewMergeSeriesSet(seriesSets, 0, storage.ChainedSeriesMerge)
}

func (q *awareQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return queryLabelNames(ctx, q.Querier, q.engine, hints, matchers)
}

func (q *awareQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return queryLabelValues(ctx, q.Querier, q.engine, name, hints, matchers)
}

type awareChunkQuerier struct {
	storage.ChunkQuerier

	engine *schemaEngine
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
		return annotateChunkSeriesSet(
			q.ChunkQuerier.Select(ctx, sortSeries, hints, passthrough...),
			variantErrorWarning(matchers, err),
		)
	}
	if qCtx.labelMapping == nil {
		return q.ChunkQuerier.Select(ctx, sortSeries, hints, passthrough...)
	}

	chunkSeriesSets := make([]storage.ChunkSeriesSet, len(variants))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOutLimit)
	for i, ms := range variants {
		g.Go(func() error {
			chunkSeriesSets[i] = &awareChunkSeriesSet{
				ChunkSeriesSet: q.ChunkQuerier.Select(gctx, true, hints, ms...),
				engine:         q.engine,
				qCtx:           qCtx,
			}
			return nil
		})
	}
	_ = g.Wait()
	return storage.NewMergeChunkSeriesSet(chunkSeriesSets, 0, storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge))
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
		names, anns, lerr := q.LabelNames(ctx, hints, passthrough...)
		return names, anns.Add(schemaWarning(variantErrorWarning(matchers, err))), lerr
	}
	if qCtx.labelMapping == nil {
		return q.LabelNames(ctx, hints, passthrough...)
	}

	type partial struct {
		names []string
		anns  annotations.Annotations
		err   error
	}
	results := make([]partial, len(variants))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOutLimit)
	for i, ms := range variants {
		g.Go(func() error {
			n, a, err := q.LabelNames(gctx, hints, ms...)
			results[i] = partial{names: n, anns: a, err: err}
			return nil
		})
	}
	_ = g.Wait()

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
			canonical := reverseLabelName(qCtx, n)
			if _, ok := seen[canonical]; ok {
				continue
			}
			seen[canonical] = struct{}{}
			combined = append(combined, canonical)
		}
	}
	slices.Sort(combined)
	return combined, combinedAnns, errors.Join(errs...)
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
		values, anns, lerr := q.LabelValues(ctx, name, hints, passthrough...)
		return values, anns.Add(schemaWarning(variantErrorWarning(matchers, err))), lerr
	}
	if qCtx.labelMapping == nil {
		return q.LabelValues(ctx, name, hints, passthrough...)
	}

	// LabelValues is queried with the user-supplied name as-is for each
	// variant: the engine's labelMapping only exposes historical→canonical for
	// label names, not the forward direction, so values stored under a
	// historical alias of `name` are not surfaced for arbitrary labels here.
	// The metric name (__name__) is the exception — every variant resolves to
	// the same canonical metric, recorded in labelMapping.translatedMetric.
	type partial struct {
		values []string
		anns   annotations.Annotations
		err    error
	}
	results := make([]partial, len(variants))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOutLimit)
	for i, ms := range variants {
		g.Go(func() error {
			v, a, err := q.LabelValues(gctx, name, hints, ms...)
			results[i] = partial{values: v, anns: a, err: err}
			return nil
		})
	}
	_ = g.Wait()

	// When the caller asked for values of __name__, every variant's results
	// are different escapings of the same canonical metric; collapse them.
	// qCtx.labelMapping is non-nil here (early return above) and
	// buildLabelMapping unconditionally sets translatedMetric, so the
	// canonical name is always available when name == __name__.
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
			if metricNameQuery {
				v = qCtx.labelMapping.translatedMetric
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			combined = append(combined, v)
		}
	}
	slices.Sort(combined)
	return combined, combinedAnns, errors.Join(errs...)
}

type annotatedSeriesSet struct {
	storage.SeriesSet

	warning string
}

func annotateSeriesSet(s storage.SeriesSet, warning string) storage.SeriesSet {
	return &annotatedSeriesSet{warning: warning, SeriesSet: s}
}

func (s *annotatedSeriesSet) Warnings() annotations.Annotations {
	got := s.SeriesSet.Warnings()
	return got.Add(schemaWarning(s.warning))
}

type annotatedChunkSeriesSet struct {
	storage.ChunkSeriesSet

	warning string
}

func annotateChunkSeriesSet(s storage.ChunkSeriesSet, warning string) storage.ChunkSeriesSet {
	return &annotatedChunkSeriesSet{warning: warning, ChunkSeriesSet: s}
}

func (s *annotatedChunkSeriesSet) Warnings() annotations.Annotations {
	got := s.ChunkSeriesSet.Warnings()
	return got.Add(schemaWarning(s.warning))
}

type awareSeriesSet struct {
	storage.SeriesSet

	qCtx   queryContext
	engine *schemaEngine

	at storage.Series
}

func (s *awareSeriesSet) At() storage.Series {
	return s.at
}

func (s *awareSeriesSet) Next() bool {
	if s.Err() != nil {
		return false
	}
	if !s.SeriesSet.Next() {
		return false
	}
	at := s.SeriesSet.At()
	s.at = &awareSeries{Series: at, lbls: s.engine.transformSeries(s.qCtx, at.Labels())}
	return true
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

	qCtx   queryContext
	engine *schemaEngine

	at storage.ChunkSeries
}

func (s *awareChunkSeriesSet) At() storage.ChunkSeries {
	return s.at
}

func (s *awareChunkSeriesSet) Next() bool {
	if s.Err() != nil {
		return false
	}
	if !s.ChunkSeriesSet.Next() {
		return false
	}
	at := s.ChunkSeriesSet.At()
	s.at = &awareChunkSeries{ChunkSeries: at, lbls: s.engine.transformSeries(s.qCtx, at.Labels())}
	return true
}

type awareChunkSeries struct {
	storage.ChunkSeries

	lbls labels.Labels
}

func (s *awareChunkSeries) Labels() labels.Labels {
	return s.lbls
}
