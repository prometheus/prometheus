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

package promql

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// CostEstimate approximates a query's storage input before execution.
// Both values may overestimate or underestimate the actual cost.
type CostEstimate struct {
	// SeriesTouched estimates series reads, summed across explicit selectors.
	// Series shared by selectors may be counted more than once. Runtime-dependent
	// selections such as info() are omitted and produce an incomplete-estimate warning.
	SeriesTouched int64
	// SamplesRead estimates input in the engine's sample units: one per float
	// and (FloatHistogram.Size()+8)/16 per native histogram. Histogram size and
	// sample density are measured from a bounded sample of the matching data.
	SamplesRead int64
}

// EstimateCost approximates storage input without evaluating expr. The parser
// and evaluation options must match the engine; non-positive lookbackDelta and
// subqueryDefaultStep use package defaults. Range queries require a scalar or vector.
//
// Series enumeration uses the "series" hint over preprocessed selector windows.
// It scales with cardinality and storage blocks and can retrieve remote sample data.
// Callers must bound the whole operation, including parsing, by deadline and concurrency.
//
// CostSamplingQueryable sources measure density and point size incrementally,
// examining at most 50 series and, for density, 50 chunks per selector. Unsupported
// sources or windows use scrapeInterval and float-sized points with a warning.
//
// Estimates follow the engine's nested evaluation grids and incremental window reads.
// Neither figure is a guaranteed bound. Sampling may not represent all matching data;
// shared selectors count repeated series, and implicit info() reads are omitted with
// a warning. Arithmetic saturates at math.MaxInt64. Errors return no partial estimate.
func EstimateCost(ctx context.Context, q storage.Queryable, p parser.Parser, expr string, start, end time.Time, step, lookbackDelta, subqueryDefaultStep, scrapeInterval time.Duration) (estimate CostEstimate, annos annotations.Annotations, err error) {
	defer func() {
		err = costEstimationError(ctx, err)
		if err != nil {
			estimate = CostEstimate{}
		}
	}()
	if err := ctx.Err(); err != nil {
		return CostEstimate{}, annos, err
	}

	if lookbackDelta <= 0 {
		lookbackDelta = defaultLookbackDelta
	}
	if subqueryDefaultStep <= 0 {
		subqueryDefaultStep = defaultSubqueryStep
	}

	parsed, err := p.ParseExpr(expr)
	if contextErr := ctx.Err(); contextErr != nil {
		return CostEstimate{}, annos, errors.Join(contextErr, err)
	}
	if err != nil {
		return CostEstimate{}, annos, err
	}
	if step > 0 && parsed.Type() != parser.ValueTypeVector && parsed.Type() != parser.ValueTypeScalar {
		return CostEstimate{}, annos, fmt.Errorf("invalid expression type %q for range query, must be Scalar or instant Vector", parser.DocumentedType(parsed.Type()))
	}

	// Run the same preprocessing the engine runs before evaluating, so the
	// estimator sees the expression the engine would actually execute: duration
	// expressions resolved to durations, @ start()/end() resolved to timestamps,
	// histogram-stats decoding detected, and step-invariant subtrees wrapped.
	// Without this a range or offset written as an expression is misread, and a
	// step-invariant selector is charged once per step instead of once.
	parsed, err = PreprocessExpr(parsed, start, end, step)
	if contextErr := ctx.Err(); contextErr != nil {
		return CostEstimate{}, annos, errors.Join(contextErr, err)
	}
	if err != nil {
		return CostEstimate{}, annos, err
	}

	stmt := &parser.EvalStmt{
		Expr:          parsed,
		Start:         start,
		End:           end,
		Interval:      step,
		LookbackDelta: lookbackDelta,
	}

	// Collect every selector together with the effective time window it reads.
	// We mirror the shape of the engine's getTimeRangesForSelector/populateSeries
	// logic so that the estimated window matches what evaluation would actually
	// query. Whenever a MatrixSelector is encountered, evalRange is set to the
	// corresponding range; the VectorSelector inside then consumes it and resets
	// it, just like the engine does.
	//
	// For each selector we record, besides the union window [mint,maxt] used for
	// the Select call:
	//
	//   - rangeMs: the storage read window, including lookback extensions for
	//     anchored and smoothed selectors (0 for a plain instant selector).
	//   - numSteps: how many evaluation steps read this selector. For a plain
	//     selector this is the outer query step count; for a selector inside a
	//     subquery it is the subquery's own step count.
	//   - isRange: whether the selector reads a window of samples.
	type selectorWindow struct {
		matchers []*labels.Matcher
		mint     int64
		maxt     int64
		rangeMs  int64
		// stepMs is the interval between consecutive evaluations of this
		// selector: the outer query step for a plain selector, or the subquery
		// resolution for a selector inside a subquery. It approximates the new
		// samples a range selector reads at each step after the first.
		stepMs               int64
		numSteps             int64
		isRange              bool
		skipHistogramBuckets bool
	}
	var (
		selectors []selectorWindow
		evalRange time.Duration
	)
	var incomplete bool
	parser.Inspect(stmt.Expr, func(node parser.Node, path []parser.Node) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch n := node.(type) {
		case *parser.Call:
			if n.Func.Name == "info" {
				incomplete = true
			}
		case *parser.VectorSelector:
			for _, parent := range path {
				if call, ok := parent.(*parser.Call); ok && call.Func.Name == "info" && len(call.Args) > 1 && call.Args[1] == n {
					return nil
				}
			}
			mint, maxt := getTimeRangesForSelector(stmt, n, path, evalRange)
			isRange := evalRange > 0
			numSteps, stepMs := selectorEvaluationSteps(stmt, path, subqueryDefaultStep)
			if n.Timestamp != nil && isRange {
				// Range functions reuse an @ selector's window after their first step.
				numSteps = min(numSteps, 1)
			}
			rangeMs := evalRange.Milliseconds()
			switch {
			case n.Anchored:
				rangeMs = addSaturatingInt64(rangeMs, lookbackDelta.Milliseconds())
			case n.Smoothed:
				// Smoothed vectors also consume a window, rather than one point.
				rangeMs = addSaturatingInt64(rangeMs, lookbackDelta.Milliseconds())
				rangeMs = addSaturatingInt64(rangeMs, lookbackDelta.Milliseconds())
				isRange = true
			}
			selectors = append(selectors, selectorWindow{
				matchers:             n.LabelMatchers,
				mint:                 mint,
				maxt:                 maxt,
				rangeMs:              rangeMs,
				stepMs:               stepMs,
				numSteps:             numSteps,
				isRange:              isRange,
				skipHistogramBuckets: n.SkipHistogramBuckets,
			})
			evalRange = 0
		case *parser.MatrixSelector:
			evalRange = n.Range
		}
		return nil
	})
	if err := ctx.Err(); err != nil {
		return CostEstimate{}, annos, err
	}

	if incomplete {
		annos.Add(errors.New("query cost estimate is incomplete: info() selects additional series at runtime"))
	}

	if len(selectors) == 0 {
		return CostEstimate{}, annos, nil
	}

	// Mirror the engine: open a single querier over the union of every
	// selector's window and reuse it for each selector's Select call. Opening
	// one querier over [unionMint,unionMaxt] guarantees all selectors observe
	// the same set of storage blocks; opening a separate narrow querier per
	// selector could yield different block sets and different series counts.
	unionMint, unionMaxt := selectors[0].mint, selectors[0].maxt
	for _, sel := range selectors[1:] {
		if sel.mint < unionMint {
			unionMint = sel.mint
		}
		if sel.maxt > unionMaxt {
			unionMaxt = sel.maxt
		}
	}

	var sampler storage.SampleAndChunkQueryable
	if provider, ok := q.(storage.CostSamplingQueryable); ok {
		if source, supported := provider.CostSamplingQueryable(); supported {
			sampler = source
		}
	}
	if err := ctx.Err(); err != nil {
		return CostEstimate{}, annos, err
	}

	// The series count uses Func:"series" so storage can skip chunk decoding.
	querier, err := q.Querier(unionMint, unionMaxt)
	if err != nil {
		return CostEstimate{}, annos, err
	}
	defer func() { err = errors.Join(err, querier.Close()) }()

	for _, sel := range selectors {
		series, sa, err := countSeries(ctx, querier, sel.mint, sel.maxt, sel.matchers)
		annos = annos.Merge(sa)
		if err != nil {
			return CostEstimate{}, annos, err
		}

		estimate.SeriesTouched = addSaturatingInt64(estimate.SeriesTouched, series)

		// Sample the real window within a fixed chunk budget. Completed series
		// constrain extrapolation across gaps; partial series still inform the
		// interval when the window is too large to sample a whole series.
		effInterval := scrapeInterval
		avgPointCost := fallbackAvgPointCost
		var density selectorDensity
		if sampler != nil && series > 0 {
			var sia annotations.Annotations
			density, sia, err = sampleSelectorDensity(ctx, sampler, sel.mint, sel.maxt, sel.matchers)
			annos = annos.Merge(sia)
			if err != nil && !samplingUnsupported(err) {
				return CostEstimate{}, annos, err
			}
			if err == nil {
				var sca annotations.Annotations
				avgPointCost, sca, err = sampleAvgPointCost(ctx, sampler, sel.mint, sel.maxt, series, scrapeInterval, sel.skipHistogramBuckets, sel.matchers)
				annos = annos.Merge(sca)
				if err != nil && !samplingUnsupported(err) {
					return CostEstimate{}, annos, err
				}
			}
			if err != nil {
				// A later opening can see a new unsupported layout. Discard both
				// measurements rather than keeping a partially sampled estimate.
				density = selectorDensity{}
				avgPointCost = fallbackAvgPointCost
				annos.Add(errCostSamplingFallback)
			} else if density.intervalMs > 0 {
				effInterval = time.Duration(density.intervalMs * float64(time.Millisecond))
			}
		} else if series > 0 {
			annos.Add(errCostSamplingFallback)
		}

		// Estimate samples per series following the engine's incremental reads,
		// then multiply by the series count and scale by the measured per-point
		// cost. All arithmetic saturates to avoid int64 overflow for huge windows
		// with tiny scrape intervals.
		selectorSamples := mulSaturatingInt64(series, samplesPerSeries(sel.isRange, sel.rangeMs, sel.stepMs, effInterval, sel.numSteps))
		if sel.isRange && density.completeSeries > 0 {
			// A range selector cannot consume more points than exist in its
			// storage window. Extrapolate the mean of fully sampled series, not
			// a partial series that happened to exhaust the sampling budget.
			windowSamples := scaleSaturatingInt64(series, float64(density.samples)/float64(density.completeSeries))
			selectorSamples = min(selectorSamples, windowSamples)
		}
		selectorSamples = scaleSaturatingInt64(selectorSamples, avgPointCost)
		estimate.SamplesRead = addSaturatingInt64(estimate.SamplesRead, selectorSamples)
	}

	return estimate, annos, nil
}

var errCostSamplingFallback = errors.New("query cost estimate uses the configured scrape interval and float-sized points because bounded storage sampling is unavailable")

func costEstimationError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil && !errors.Is(err, contextErr) {
		return errors.Join(contextErr, err)
	}
	return err
}

func samplingUnsupported(err error) bool {
	return errors.Is(err, storage.ErrCostSamplingUnsupported) &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// mulSaturatingInt64 multiplies two non-negative int64 values, saturating at
// math.MaxInt64 instead of overflowing. It mirrors the saturation discipline
// used by uint64ToInt64Limit in engine.go.
func mulSaturatingInt64(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > math.MaxInt64/b {
		return math.MaxInt64
	}
	return a * b
}

// addSaturatingInt64 adds two non-negative int64 values, saturating at
// math.MaxInt64 instead of overflowing.
func addSaturatingInt64(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// scaleSaturatingInt64 multiplies a non-negative int64 by a non-negative float
// factor, rounding the result to the nearest int64 and saturating at
// math.MaxInt64 instead of overflowing. A factor <= 0 leaves the value unchanged
// so a degenerate measurement cannot zero out the estimate.
func scaleSaturatingInt64(a int64, factor float64) int64 {
	if a == 0 {
		return 0
	}
	if factor <= 0 {
		return a
	}
	scaled := math.Round(float64(a) * factor)
	if scaled >= math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(scaled)
}

// histogramSampleLimit is the maximum number of a selector's series the estimator
// decodes when measuring its average per-point cost. It bounds the extra work in
// two ways: it is the threshold below which a selector's real series count
// qualifies for exact sampling from its real window, and it is also the cap on
// series decoded from the fallback window near the selector's end (see
// sampleAvgPointCost). Either way at most this many sample decodes happen per
// selector.
const histogramSampleLimit = 50

// fallbackAvgPointCost is the per-point cost the estimator assumes when sampling
// finds no in-window points for a selector. Defaulting to one unit
// treats those series as floats, which neither inflates nor changes the existing
// float-only estimate. A future config knob could override this for deployments
// known to be histogram-heavy, but the primary path is the measured average from
// sampleAvgPointCost.
const fallbackAvgPointCost = 1.0

// histogramSampleWindowMin and histogramSampleWindowMax bound the narrow window
// near the selector's end that sampleAvgPointCost samples when the selector has
// too many series to sample its real window directly. The window is chosen large
// enough to contain a recent point (a few scrape intervals) yet small enough to
// limit chunk decoding.
const (
	histogramSampleWindowMin = 5 * time.Minute
	histogramSampleWindowMax = 30 * time.Minute
)

// fallbackSampleWindow returns the narrow window the samplers measure over when a
// selector's real window is too expensive to sample directly. It ends at the
// selector's own maxt, so a selector carrying an offset or an @ modifier is
// measured near the data it actually reads rather than near the query's end. The
// window spans a few scrape intervals so a point is very likely present, clamped
// between histogramSampleWindowMin and histogramSampleWindowMax so decoding stays
// bounded regardless of the scrape interval.
func fallbackSampleWindow(endMs int64, scrapeInterval time.Duration) (mint, maxt int64) {
	// Clamp before multiplying so a very large configured interval cannot overflow.
	window := max(histogramSampleWindowMin, max(0, min(scrapeInterval, histogramSampleWindowMax/8))*8)
	if endMs < math.MinInt64+window.Milliseconds() {
		return math.MinInt64, endMs
	}
	return endMs - window.Milliseconds(), endMs
}

// chunkSampleLimit bounds the chunks decoded per selector to measure density.
// The sampler stops before advancing the iterator beyond this budget, including
// when chunks contain only one sample or the budget ends partway through a series.
const chunkSampleLimit = 50

// Density sampling must also bound series whose iterators contain no chunks.
const densitySeriesSampleLimit = 50

// sampleAvgPointCost sizes the first usable point of at most histogramSampleLimit
// series through an approved sampling source. Small populations use the selector's
// real window; larger populations use a narrow window near its end.
// Bucket decoding honors skipHistogramBuckets and sizing matches HPoint.size().
// Missing usable points return fallbackAvgPointCost; failures remain errors.
func sampleAvgPointCost(ctx context.Context, q storage.Queryable, mint, maxt, series int64, scrapeInterval time.Duration, skipHistogramBuckets bool, matchers []*labels.Matcher) (avgPointCost float64, annos annotations.Annotations, err error) {
	defer func() { err = costEstimationError(ctx, err) }()
	if err := ctx.Err(); err != nil {
		return fallbackAvgPointCost, annos, err
	}
	qMint, qMaxt := mint, maxt
	if series <= 0 || series > histogramSampleLimit {
		// Too many series (or the count is not yet known) to sample the real window
		// affordably: fall back to a narrow window near the selector's end.
		qMint, qMaxt = fallbackSampleWindow(maxt, scrapeInterval)
	} else if qMint > qMaxt {
		// Guard against a real window with an inverted (overflowed) mint.
		qMint = qMaxt
	}

	querier, err := q.Querier(qMint, qMaxt)
	if err != nil {
		return fallbackAvgPointCost, annos, err
	}
	defer func() { err = errors.Join(err, querier.Close()) }()
	if err := ctx.Err(); err != nil {
		return fallbackAvgPointCost, annos, err
	}

	// Select WITHOUT Func:"series": the estimator's main querier asks for
	// label-only series, but here we must decode actual samples to size points.
	hints := &storage.SelectHints{Start: qMint, End: qMaxt}
	set := querier.Select(ctx, false, hints, matchers...)

	var (
		sum     float64
		n       int
		sampled int
	)
	for sampled < histogramSampleLimit && set.Next() {
		if err := ctx.Err(); err != nil {
			return fallbackAvgPointCost, annos, err
		}
		sampled++

		s := set.At()
		if skipHistogramBuckets {
			s = newHistogramStatsSeries(s)
		}
		it := s.Iterator(nil)
		cost, ok := firstPointCost(it)
		if err := it.Err(); err != nil {
			return fallbackAvgPointCost, set.Warnings(), err
		}
		if !ok {
			continue
		}
		sum += cost
		n++
	}

	annos = annos.Merge(set.Warnings())
	if err := set.Err(); err != nil {
		return fallbackAvgPointCost, annos, err
	}
	if n == 0 {
		// No in-window points were sampled: fall back to the documented default
		// rather than guessing a histogram cost.
		return fallbackAvgPointCost, annos, nil
	}
	return sum / float64(n), annos, nil
}

// firstPointCost advances the iterator to its first point and returns
// that point's cost in the engine's sample-unit accounting: one unit for a float
// point and (FloatHistogram.Size()+8)/16 units for a native-histogram point,
// mirroring HPoint.size(). It returns ok=false when the series has no usable
// first point (empty or a leading stale marker).
func firstPointCost(it chunkenc.Iterator) (cost float64, ok bool) {
	switch it.Next() {
	case chunkenc.ValFloat:
		_, v := it.At()
		if value.IsStaleNaN(v) {
			return 0, false
		}
		return 1, true
	case chunkenc.ValHistogram:
		_, h := it.AtHistogram(nil)
		fh := h.ToFloat(nil)
		if value.IsStaleNaN(fh.Sum) {
			return 0, false
		}
		return float64((fh.Size() + 8) / 16), true
	case chunkenc.ValFloatHistogram:
		_, fh := it.AtFloatHistogram(nil)
		if value.IsStaleNaN(fh.Sum) {
			return 0, false
		}
		return float64((fh.Size() + 8) / 16), true
	default:
		// chunkenc.ValNone or anything else: no usable point.
		return 0, false
	}
}

// selectorDensity summarizes a bounded sample of a selector's storage window.
// Only completely sampled series contribute to samples and completeSeries.
// Partial series can contribute to intervalMs, including gaps between chunks.
type selectorDensity struct {
	intervalMs     float64
	samples        int64
	completeSeries int64
}

// sampleSelectorDensity examines at most chunkSampleLimit chunks from the real
// selector window. It counts all points in completed series, including the
// effect of leading, trailing and internal gaps. When the budget ends partway
// through a series, that series only contributes its observed sample interval.
// Chunk boundaries are included in that interval so single-point chunks can
// still provide a measurement. No additional chunk is read to check whether a
// series that exhausts the budget is complete.
func sampleSelectorDensity(ctx context.Context, cq storage.ChunkQueryable, mint, maxt int64, matchers []*labels.Matcher) (density selectorDensity, annos annotations.Annotations, err error) {
	defer func() { err = costEstimationError(ctx, err) }()
	if err := ctx.Err(); err != nil {
		return density, annos, err
	}
	querier, err := cq.ChunkQuerier(mint, maxt)
	if err != nil {
		return density, annos, err
	}
	defer func() { err = errors.Join(err, querier.Close()) }()
	if err := ctx.Err(); err != nil {
		return density, annos, err
	}

	set := querier.Select(ctx, false, &storage.SelectHints{Start: mint, End: maxt}, matchers...)
	var (
		totalSpanMs float64
		totalGaps   int64
		read        int
		seriesRead  int
		it          chunks.Iterator
	)
	for read < chunkSampleLimit && seriesRead < densitySeriesSampleLimit && set.Next() {
		if err := ctx.Err(); err != nil {
			return density, set.Warnings(), err
		}
		seriesRead++
		var (
			points  int64
			lastMax int64
			haveMax bool
		)
		it = set.At().Iterator(it)
		for read < chunkSampleLimit && it.Next() {
			if err := ctx.Err(); err != nil {
				return density, set.Warnings(), err
			}
			read++
			meta := it.At()
			if meta.Chunk == nil || meta.Chunk.NumSamples() == 0 {
				continue
			}
			n := int64(meta.Chunk.NumSamples())
			points += n
			if haveMax && meta.MinTime > lastMax {
				totalGaps++
				totalSpanMs += float64(meta.MinTime) - float64(lastMax)
			}
			if n > 1 && meta.MaxTime > meta.MinTime {
				totalGaps += n - 1
				totalSpanMs += float64(meta.MaxTime) - float64(meta.MinTime)
			}
			lastMax, haveMax = meta.MaxTime, true
		}
		if err := it.Err(); err != nil {
			return density, set.Warnings(), err
		}
		if read < chunkSampleLimit && points > 0 {
			density.samples += points
			density.completeSeries++
		}
	}
	annos = set.Warnings()
	if err := set.Err(); err != nil {
		return density, annos, err
	}
	if totalGaps > 0 && totalSpanMs > 0 {
		density.intervalMs = totalSpanMs / float64(totalGaps)
	}
	return density, annos, nil
}

// countSeries enumerates matching series through the shared querier using the
// "series" hint. Backends may still retrieve sample data to enumerate their series.
// Enumeration is not capped; cancellation returns an error rather than a partial count.
func countSeries(ctx context.Context, querier storage.Querier, mint, maxt int64, matchers []*labels.Matcher) (count int64, annos annotations.Annotations, err error) {
	defer func() { err = costEstimationError(ctx, err) }()
	if err := ctx.Err(); err != nil {
		return 0, annos, err
	}
	// Func "series" lets supporting storage skip sample retrieval.
	hints := &storage.SelectHints{
		Start: mint,
		End:   maxt,
		Func:  "series",
	}
	set := querier.Select(ctx, false, hints, matchers...)

	for set.Next() {
		if err := ctx.Err(); err != nil {
			return 0, set.Warnings(), err
		}
		count++
	}

	annos = set.Warnings()
	if err := set.Err(); err != nil {
		return 0, annos, err
	}
	return count, annos, nil
}

// Follow the evaluator boundaries in path, preserving the order of subqueries
// and step-invariant expressions. This accounts for nested grids, offsets, @,
// and a fixed selector evaluated once inside a non-invariant subquery.
func selectorEvaluationSteps(stmt *parser.EvalStmt, path []parser.Node, defaultStep time.Duration) (numSteps, interval int64) {
	start, end := stmt.Start.UnixMilli(), stmt.End.UnixMilli()
	interval = max(stmt.Interval.Milliseconds(), 1)
	if stmt.Interval <= 0 {
		end = start
	}
	for _, node := range path {
		switch n := node.(type) {
		case *parser.StepInvariantExpr:
			end = start
		case *parser.SubqueryExpr:
			offset := n.OriginalOffset.Milliseconds()
			if n.Timestamp != nil {
				offset += start - *n.Timestamp
			}
			step := n.Step
			if step <= 0 {
				step = defaultStep
			}
			subqInterval := max(step.Milliseconds(), 1)
			start, end = subqueryEvaluationTimes(start, end, interval, offset, n.Range.Milliseconds(), subqInterval, n.Timestamp != nil)
			interval = subqInterval
		}
	}
	if end < start {
		return 0, interval
	}
	return (end-start)/interval + 1, interval
}

// defaultSubqueryStep is the subquery resolution EstimateCost falls back to when
// the caller passes a non-positive subqueryDefaultStep. It matches the engine's
// own default subquery evaluation interval
// (config.DefaultGlobalConfig.EvaluationInterval, 1m); callers should pass the
// engine's configured value instead of relying on it.
const defaultSubqueryStep = time.Minute

// samplesPerSeries models one full range window followed by incremental reads,
// or one sample per step for a vector selector. Fractional samples per step are
// accumulated before rounding so fine steps and non-integral scrape ratios do
// not discard reads. Nonoverlapping windows cost at most one window per step.
func samplesPerSeries(isRange bool, rangeMs, stepMs int64, scrapeInterval time.Duration, numSteps int64) int64 {
	if numSteps <= 0 {
		return 0
	}
	if !isRange {
		return numSteps
	}
	perWindow := samplesPerWindow(rangeMs, scrapeInterval)
	if numSteps == 1 || stepMs <= 0 || scrapeInterval.Milliseconds() <= 0 {
		return perWindow
	}
	perStep := min(float64(stepMs)/float64(scrapeInterval.Milliseconds()), float64(perWindow))
	return addSaturatingInt64(perWindow, scaleSaturatingInt64(numSteps-1, perStep))
}

// samplesPerWindow estimates how many samples a single series contributes over
// a window of windowMs milliseconds given the scrape interval. It assumes one
// sample every scrapeInterval across the inclusive window, i.e.
// floor(window/scrapeInterval)+1. If scrapeInterval is non-positive it returns 1
// so the estimate degrades to the series count rather than dividing by zero.
func samplesPerWindow(windowMs int64, scrapeInterval time.Duration) int64 {
	if scrapeInterval <= 0 {
		return 1
	}
	if windowMs < 0 {
		windowMs = 0
	}
	intervalMs := scrapeInterval.Milliseconds()
	if intervalMs <= 0 {
		return 1
	}
	return windowMs/intervalMs + 1
}
