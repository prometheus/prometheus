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

// CostEstimate is the estimated resource cost of a query before execution.
//
// The values are derived from the storage index and a per-selector sample
// estimate. SeriesTouched is a cheap upper bound on the distinct series read.
// SamplesScanned models the engine's incremental per-step reads; see
// EstimateCost for the model and its accuracy limitations.
type CostEstimate struct {
	// SeriesTouched is the estimated number of series the query would read. It
	// is the sum of the series count across every selector in the query and may
	// double-count series that are shared between selectors. It is therefore an
	// upper bound on the distinct series read.
	SeriesTouched int64
	// SamplesScanned is the estimated number of samples the query would scan,
	// expressed in the engine's sample-unit accounting (a float point counts as
	// one unit; a native-histogram point counts as (FloatHistogram.Size()+8)/16
	// units, i.e. roughly half a unit per bucket). It is the sum, across every
	// selector, of that selector's per-series sample estimate multiplied by its
	// series count and scaled by the selector's measured average per-point cost.
	//
	// The per-point cost is measured by sampling a bounded number of the
	// selector's series (see EstimateCost): float selectors keep a cost of one
	// unit per point, while native-histogram selectors are scaled up to match the
	// engine's per-bucket sample accounting. This assumes the sampled series are
	// representative of all the selector's series; a selector mixing wide and
	// narrow histograms, or histograms and floats, is only approximated.
	//
	// The per-series estimate mirrors how the engine actually reads samples
	// rather than re-reading a full window at every step:
	//
	//   - A range (matrix) selector reads its full range window once at the
	//     first step and then only the samples that advance past the previous
	//     step's cutoff at each subsequent step, so its per-series estimate is
	//     samplesPerWindow(range) + (numSteps-1)*samplesAdvancedPerStep(step),
	//     with the per-step term capped at one full window because windows stop
	//     overlapping once the step is wider than the range.
	//   - An instant (vector) selector reads roughly one sample per evaluation
	//     step, so its per-series estimate is numSteps (and 1 for a pure instant
	//     query).
	//   - A selector inside a subquery is evaluated on the subquery's own, finer
	//     step grid spanning the query range plus the subquery range, so its
	//     numSteps is computed from that grid instead of the outer step count.
	//
	// Because a series shared between selectors is counted once per selector the
	// estimate can still over-count. It saturates at math.MaxInt64 instead of
	// overflowing.
	SamplesScanned int64
}

// EstimateCost estimates the cost of expr over [start,end] step without
// executing it.
//
// It parses expr, walks it to find every vector and matrix selector together
// with the effective time window each selector reads (taking range selectors,
// offsets, @ modifiers, subquery context and the lookback delta into account,
// mirroring the engine's own time-range logic), and then asks the storage for
// the number of series matching each selector. A single querier is opened over
// the union of every selector's window, mirroring the engine, so all selectors
// see the same set of storage blocks.
//
// When the storage exposes a storage.ChunkQueryable the series count also yields,
// as a cheap index-only byproduct, the number of chunk metas overlapping every
// selector's window (see countSeriesAndChunks). That count is not reported: it is
// used only to decide whether a selector's real window is cheap enough to sample
// its density directly (see sampleEffectiveInterval).
//
// SamplesScanned models the engine's incremental per-step reads rather than
// re-reading a full window at every step. For each selector the per-series
// sample estimate is:
//
//   - Range (matrix) selector: samplesPerWindow(range) at the first step plus
//     samplesAdvancedPerStep(step) at each of the (numSteps-1) subsequent steps.
//     The engine reads the full range window only at step 0 and then only the new
//     samples that advance past the previous step's cutoff, so one step's worth
//     of advanced samples approximates the new points read each subsequent step.
//     When the step is wider than the range the windows no longer overlap and the
//     engine re-reads a whole window each step, so the per-step term is capped at
//     samplesPerWindow(range).
//   - Instant (vector) selector: numSteps, because the engine reads roughly one
//     sample per evaluation step (the lookback delta only governs which sample
//     is selected, not how many samples are counted as read). A pure instant
//     query has numSteps==1, i.e. one sample per series.
//   - Selector inside a subquery: numSteps is taken from the subquery's own step
//     grid, which spans the query range plus the subquery range at the
//     subquery's resolution, rather than the outer step count, because the
//     engine evaluates the inner expression once on that finer grid.
//
// To size native-histogram points correctly the estimator additionally samples,
// whenever the storage exposes a storage.ChunkQueryable, a bounded number of each
// selector's series (at most histogramSampleLimit) and decodes their first
// in-window point to measure the average per-point cost in the engine's
// sample-unit accounting. When the selector's real series count fits within
// histogramSampleLimit the sample is taken directly from the selector's real
// window, so the measurement reflects the actual data the query would touch;
// otherwise it falls back to a bounded window near the selector's end (see
// sampleAvgPointCost). This makes the estimator no longer strictly index-only:
// each selector incurs a bounded (<= histogramSampleLimit) sample decode. The
// measured average is then used to scale that selector's per-series sample
// estimate. If a selector yields no sampled points (e.g. an empty window), or the
// storage exposes no chunk metadata at all, the estimator falls back to a
// documented default per-point cost (see fallbackAvgPointCost) and stays
// index-only.
//
// When the storage exposes chunk metadata the estimator also measures each
// selector's effective sample interval from a bounded sample of its chunks (at
// most chunkSampleLimit chunks, see sampleEffectiveInterval), summing their
// NumSamples header counts and time spans, and uses that measured interval to
// size the per-series sample estimate instead of the supplied scrapeInterval.
// When the selector's real chunk count fits within chunkSampleLimit the chunks
// are read directly from the selector's real window; otherwise the estimator
// falls back to a bounded window near the selector's end. This too is bounded work: the
// caller-supplied scrapeInterval is used only as the fallback when nothing is
// sampled or the storage exposes a plain Queryable.
//
// The estimate is intentionally cheap. SeriesTouched is an upper bound on the
// distinct series read. Its accuracy is limited in the following ways:
//
//   - Series are counted from the storage index regardless of whether they
//     actually have samples inside the selector's time window, so selectors that
//     match series with no in-window samples are over-counted.
//   - Series matched by more than one selector are counted once per selector,
//     so SeriesTouched is an upper bound on the distinct series read and
//     SamplesScanned double-counts the samples of shared series.
//   - When the time window spans multiple storage blocks, a series present in
//     several blocks may be counted multiple times.
//   - The per-series sample count assumes samples are present at exactly one
//     interval across the relevant window. When the storage exposes chunk
//     metadata that interval is measured from a bounded sample of the selector's
//     chunks (see sampleEffectiveInterval) rather than assumed; otherwise the
//     supplied scrapeInterval is used, and a selector whose real density differs
//     from it is mis-sized.
//   - Native-histogram points are sized by sampling at most histogramSampleLimit
//     of each selector's series and assuming the sampled series are
//     representative of all of them, so a selector mixing differently sized
//     histograms (or histograms and floats) is only approximated.
//   - Only a single level of subquery nesting is modelled exactly; deeper
//     nesting folds the innermost subquery's grid into numSteps but does not
//     account for the multiplicative effect of every nested level.
//   - All arithmetic saturates at math.MaxInt64 rather than overflowing to a
//     negative value.
//
// If scrapeInterval is non-positive, samplesPerWindow falls back to one sample
// per series so SamplesScanned degrades gracefully to the series count times
// the step count.
//
// expr is parsed with the supplied parser p so that experimental features the
// API enables (e.g. experimental functions or duration expressions) parse
// identically to the regular query path. Passing a parser configured differently
// from the engine's may cause valid queries to be rejected or vice versa.
//
// lookbackDelta and subqueryDefaultStep must be the values the engine that would
// run the query is configured with: the lookback delta it applies when a query
// omits an explicit one, and the interval it gives a subquery that omits an
// explicit step. Passing different values makes the estimated selector windows
// and subquery step grids diverge from what execution would use. Both fall back
// to the package defaults (5m and 1m) when non-positive, matching an engine left
// unconfigured; without that an instant selector would build a degenerate (~1ms
// or inverted) window and the estimate would collapse to roughly one sample per
// series.
func EstimateCost(ctx context.Context, q storage.Queryable, p parser.Parser, expr string, start, end time.Time, step, lookbackDelta, subqueryDefaultStep, scrapeInterval time.Duration) (CostEstimate, annotations.Annotations, error) {
	var annos annotations.Annotations

	if lookbackDelta <= 0 {
		lookbackDelta = defaultLookbackDelta
	}
	if subqueryDefaultStep <= 0 {
		subqueryDefaultStep = defaultSubqueryStep
	}

	parsed, err := p.ParseExpr(expr)
	if err != nil {
		return CostEstimate{}, annos, err
	}

	// Run the same preprocessing the engine runs before evaluating, so the
	// estimator sees the expression the engine would actually execute: duration
	// expressions resolved to durations, @ start()/end() resolved to timestamps,
	// histogram-stats decoding detected, and step-invariant subtrees wrapped.
	// Without this a range or offset written as an expression is misread, and a
	// step-invariant selector is charged once per step instead of once.
	parsed, err = PreprocessExpr(parsed, start, end, step)
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

	// Compute the number of evaluation steps. For an instant query (step == 0)
	// the engine evaluates a single step; for a range query it evaluates one
	// step per interval across [start,end] inclusive.
	queryNumSteps := int64(1)
	if step > 0 {
		queryNumSteps = int64(end.Sub(start)/step) + 1
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
	//   - rangeMs: the matrix range (0 for an instant selector), used to size the
	//     first-step full-window read of a range selector.
	//   - numSteps: how many evaluation steps read this selector. For a plain
	//     selector this is the outer query step count; for a selector inside a
	//     subquery it is the subquery's own step count.
	//   - isRange: whether the selector is a matrix selector.
	type selectorWindow struct {
		matchers []*labels.Matcher
		mint     int64
		maxt     int64
		rangeMs  int64
		// stepMs is the interval between consecutive evaluations of this
		// selector: the outer query step for a plain selector, or the subquery
		// resolution for a selector inside a subquery. It approximates the new
		// samples a range selector reads at each step after the first.
		stepMs   int64
		numSteps int64
		isRange  bool
	}
	var (
		selectors []selectorWindow
		evalRange time.Duration
	)
	parser.Inspect(stmt.Expr, func(node parser.Node, path []parser.Node) error {
		switch n := node.(type) {
		case *parser.VectorSelector:
			mint, maxt := getTimeRangesForSelector(stmt, n, path, evalRange)
			isRange := evalRange > 0
			// Determine how many steps read this selector. A selector inside a
			// subquery is evaluated on the subquery's finer grid, which spans the
			// query range plus the subquery range at the subquery's resolution,
			// so it is read many more times than the outer query step count
			// implies. This mirrors runSubquery building a child evaluator over
			// [start-range, end] stepping by the subquery interval.
			numSteps := queryNumSteps
			stepMs := step.Milliseconds()
			if subqStep, subqRange, ok := innermostSubquery(path, subqueryDefaultStep); ok {
				numSteps = subqueryNumSteps(start, end, subqStep, subqRange)
				stepMs = subqStep.Milliseconds()
			}
			if insideStepInvariant(path) {
				// The engine evaluates a step-invariant subtree once and copies the
				// result to every step, so its selectors are read once regardless of
				// the step count. A selector inside a subquery nested in a
				// step-invariant subtree still runs the subquery's own grid once.
				if _, _, ok := innermostSubquery(path, subqueryDefaultStep); !ok {
					numSteps = 1
				}
			}
			selectors = append(selectors, selectorWindow{
				matchers: n.LabelMatchers,
				mint:     mint,
				maxt:     maxt,
				rangeMs:  evalRange.Milliseconds(),
				stepMs:   stepMs,
				numSteps: numSteps,
				isRange:  isRange,
			})
			evalRange = 0
		case *parser.MatrixSelector:
			evalRange = n.Range
		}
		return nil
	})

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

	// Count series through a chunk querier when the storage exposes one: the
	// index-only chunk count comes for free with the series count and tells the
	// density sampler whether a selector's real window is cheap enough to measure
	// directly. Without chunk metadata the estimator counts series only and stays
	// strictly index-only, sizing samples from the supplied scrape interval.
	cq, useChunks := q.(storage.ChunkQueryable)

	var (
		querier      storage.Querier
		chunkQuerier storage.ChunkQuerier
	)
	// The series count always comes from the plain Querier: it carries the
	// Func:"series" hint that lets the storage skip chunks entirely, so it stays
	// index-only. The ChunkQuerier is only used for the bounded chunk-budget
	// probe, which decides whether the selector's real window is cheap enough to
	// measure directly.
	querier, err = q.Querier(unionMint, unionMaxt)
	if err != nil {
		return CostEstimate{}, annos, err
	}
	defer querier.Close()
	if useChunks {
		chunkQuerier, err = cq.ChunkQuerier(unionMint, unionMaxt)
		if err != nil {
			return CostEstimate{}, annos, err
		}
		defer chunkQuerier.Close()
	}

	var estimate CostEstimate
	for _, sel := range selectors {
		series, sa, err := countSeries(ctx, querier, sel.mint, sel.maxt, sel.matchers)
		annos = annos.Merge(sa)
		if err != nil {
			return CostEstimate{}, annos, err
		}

		// Does the selector's real window hold few enough chunks to measure its
		// density directly? The probe stops as soon as the budget is exceeded, so
		// it never walks a large selector's chunks.
		var realWindowFits bool
		if useChunks {
			var ca annotations.Annotations
			realWindowFits, ca, err = chunkBudgetFits(ctx, chunkQuerier, sel.mint, sel.maxt, sel.matchers)
			annos = annos.Merge(ca)
			if err != nil {
				return CostEstimate{}, annos, err
			}
		}
		estimate.SeriesTouched = addSaturatingInt64(estimate.SeriesTouched, series)

		// Size the per-series sample estimate from the data's real density when the
		// storage exposes chunk metadata: measure the selector's effective sample
		// interval from a bounded sample of its chunks (see sampleEffectiveInterval)
		// and use it in place of the caller-supplied scrape interval. Measuring
		// beats the supplied value because it reflects the actual fill (gaps,
		// staleness, a scrape interval that differs from the configured one) rather
		// than assuming a perfectly regular series. The supplied scrape interval
		// remains the fallback when no chunk is sampled or the storage exposes only
		// a plain Queryable. realWindowFits (probed above) tells
		// sampleEffectiveInterval whether the selector's real window is cheap enough
		// to decode directly instead of a narrow window near the selector's end.
		effInterval := scrapeInterval
		avgPointCost := fallbackAvgPointCost
		if useChunks {
			ivMs, ivOK, sia, ierr := sampleEffectiveInterval(ctx, cq, sel.mint, sel.maxt, realWindowFits, scrapeInterval, sel.matchers)
			annos = annos.Merge(sia)
			if ierr != nil {
				return CostEstimate{}, annos, ierr
			}
			if ivOK {
				effInterval = time.Duration(ivMs * float64(time.Millisecond))
			}

			// Measure the average per-point cost of this selector's series so that
			// native-histogram points, which the engine charges per bucket, are sized
			// correctly rather than counted as a single float unit each. This decodes
			// samples, so it only runs where chunk sampling runs at all: against a
			// plain storage.Queryable the estimator must stay index-only and keeps
			// the float-sized fallback cost. series (already counted above) tells
			// sampleAvgPointCost whether the selector's real window is cheap enough
			// (few enough series) to sample directly instead of a narrow window near
			// the selector's end.
			cost, sca, aerr := sampleAvgPointCost(ctx, q, sel.mint, sel.maxt, series, scrapeInterval, sel.matchers)
			annos = annos.Merge(sca)
			if aerr != nil {
				return CostEstimate{}, annos, aerr
			}
			avgPointCost = cost
		}

		// Estimate samples per series following the engine's incremental reads,
		// then multiply by the series count and scale by the measured per-point
		// cost. All arithmetic saturates to avoid int64 overflow for huge windows
		// with tiny scrape intervals.
		selectorSamples := mulSaturatingInt64(series, samplesPerSeries(sel.isRange, sel.rangeMs, sel.stepMs, effInterval, sel.numSteps))
		selectorSamples = scaleSaturatingInt64(selectorSamples, avgPointCost)
		estimate.SamplesScanned = addSaturatingInt64(estimate.SamplesScanned, selectorSamples)
	}

	return estimate, annos, nil
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
// near the selector's end that sampleAvgPointCost and sampleEffectiveInterval fall
// back to sampling over when the selector's real window is too expensive to
// sample directly (see their doc comments). The window is chosen large enough to
// contain a recent point (a few scrape intervals) yet small enough to limit chunk
// decoding.
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
	window := histogramSampleWindowMin
	if scrapeInterval > 0 {
		if w := scrapeInterval * 8; w > window {
			window = w
		}
	}
	if window > histogramSampleWindowMax {
		window = histogramSampleWindowMax
	}

	// Clamp against underflow when endMs is near math.MinInt64.
	mint = min(endMs-window.Milliseconds(), endMs)
	return mint, endMs
}

// chunkSampleLimit is the maximum number of chunks the estimator decodes when
// measuring a selector's effective sample interval from real chunk sample
// counts (see sampleEffectiveInterval). It bounds the extra work in two ways:
// it is the threshold below which a selector's real chunk count qualifies for
// exact sampling from its real window, and it is also the cap on chunks decoded
// from the fallback window near the query's end. Either way at most this many
// chunks are faulted in per selector, keeping the measurement far cheaper than
// executing the query even though reaching each chunk's NumSamples header
// requires paging the chunk in and CRC-checking it.
const chunkSampleLimit = 50

// sampleAvgPointCost measures the average per-point cost, in the engine's
// sample-unit accounting, of the series matching matchers. When the selector's
// real series count (series, already counted by the caller) is small enough to
// fit within histogramSampleLimit, it opens a querier over the selector's real
// window [mint,maxt] and decodes the first in-window point of every one of its
// series, so the measurement reflects the actual data the query would touch.
// Otherwise (series exceeds the limit) it falls back to sampling a narrow window
// ending at the selector's maxt (see fallbackSampleWindow): at most
// histogramSampleLimit series are decoded regardless of which window is used.
// Because it decodes samples, callers must only invoke it where chunk sampling is
// allowed to run at all, i.e. when the storage exposes a storage.ChunkQueryable;
// against a plain storage.Queryable the estimator stays index-only and uses
// fallbackAvgPointCost. It selects WITHOUT the
// "series" hint (which would return label-only series with no samples). A float
// point costs one unit; a native-histogram point costs
// (FloatHistogram.Size()+8)/16 units, mirroring HPoint.size().
//
// It returns the mean per-point cost over the sampled points. When no points are
// sampled (e.g. the sampled window holds nothing) it returns fallbackAvgPointCost
// so the estimate degrades to treating points as floats. Select
// warnings are merged into annos and the context deadline is honoured.
func sampleAvgPointCost(ctx context.Context, q storage.Queryable, mint, maxt, series int64, scrapeInterval time.Duration, matchers []*labels.Matcher) (avgPointCost float64, annos annotations.Annotations, err error) {
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
	defer querier.Close()

	// Select WITHOUT Func:"series": the estimator's main querier asks for
	// label-only series, but here we must decode actual samples to size points.
	hints := &storage.SelectHints{Start: qMint, End: qMaxt}
	set := querier.Select(ctx, false, hints, matchers...)

	var (
		sum     float64
		n       int
		sampled int
	)
	for set.Next() {
		if err := ctx.Err(); err != nil {
			return fallbackAvgPointCost, annos, err
		}
		if sampled >= histogramSampleLimit {
			break
		}
		sampled++

		s := set.At()
		it := s.Iterator(nil)
		cost, ok := firstPointCost(it)
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

// firstPointCost advances the iterator to its first non-stale point and returns
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

// sampleEffectiveInterval measures a selector's effective sample interval from
// the real sample counts of a bounded number of its chunks, so SamplesScanned
// reflects the data's actual density rather than the caller-supplied scrape
// interval. When the selector's real chunk count (numChunks, already counted by
// the caller via countSeriesAndChunks) is small enough to fit within
// chunkSampleLimit, it decodes directly from the selector's real window
// [mint,maxt]: since every one of those chunks fits the existing budget,
// decoding them all is no more expensive than the fallback-window sampling below,
// but it measures the density of the actual data the query would touch instead
// of an approximation. Otherwise (numChunks exceeds the limit, so the real
// window cannot be fully decoded affordably) it falls back to a narrow window
// ending at the selector's maxt (see fallbackSampleWindow). Either way it examines at
// most chunkSampleLimit in-window chunks and sums, for those that span a gap,
// each chunk's NumSamples (the two-byte per-chunk header count) and its trimmed
// time span. The effective interval is the mean gap between consecutive samples:
// totalSpan / totalGaps, where a chunk of k samples contributes k-1 gaps across
// its span. The budget counts every chunk examined, not just the usable ones, so
// a selector made of single-sample chunks cannot fault in more than
// chunkSampleLimit chunks.
//
// Reading NumSamples is O(1), but reaching it faults the chunk in and CRC-checks
// it, so the work is capped at chunkSampleLimit chunks to keep the measurement
// far cheaper than executing the query. It returns ok=false (the caller then
// falls back to the supplied scrape interval) when no gap can be measured, e.g.
// an empty window or only single-sample chunks. Select warnings are merged into
// annos and the context deadline is honoured.
//
// The chunks are trimmed to the query window by the chunk querier, so a chunk's
// span and NumSamples both reflect only the in-window samples; a leading or
// trailing chunk that straddles the window boundary contributes just its
// in-window portion.
func sampleEffectiveInterval(ctx context.Context, cq storage.ChunkQueryable, mint, maxt int64, realWindowFits bool, scrapeInterval time.Duration, matchers []*labels.Matcher) (intervalMs float64, ok bool, annos annotations.Annotations, err error) {
	qMint, qMaxt := mint, maxt
	if !realWindowFits {
		// The real window has no chunks to measure, or too many to decode
		// affordably: fall back to the same narrow window near the selector's end as
		// sampleAvgPointCost.
		qMint, qMaxt = fallbackSampleWindow(maxt, scrapeInterval)
	} else if qMint > qMaxt {
		// Guard against a real window with an inverted (overflowed) mint.
		qMint = qMaxt
	}

	querier, err := cq.ChunkQuerier(qMint, qMaxt)
	if err != nil {
		return 0, false, annos, err
	}
	defer querier.Close()

	set := querier.Select(ctx, false, &storage.SelectHints{Start: qMint, End: qMaxt}, matchers...)

	var (
		totalSpanMs int64
		totalGaps   int64
		// read counts every chunk the iterator hands us, whether or not it turns
		// out to be usable. The budget must bound the chunks faulted in, so a
		// selector made of single-sample or zero-span chunks cannot walk far past
		// chunkSampleLimit looking for a usable one.
		read int
		it   chunks.Iterator
	)
	for set.Next() {
		if err := ctx.Err(); err != nil {
			return 0, false, set.Warnings(), err
		}
		if read >= chunkSampleLimit {
			// Budget spent: stop before touching the next series' chunks at all.
			break
		}
		it = set.At().Iterator(it)
		for it.Next() {
			if read >= chunkSampleLimit {
				break
			}
			read++
			meta := it.At()
			if meta.Chunk == nil {
				continue
			}
			n := meta.Chunk.NumSamples()
			if n < 2 || meta.MaxTime <= meta.MinTime {
				// A chunk with fewer than two in-window samples spans no gap, so it
				// cannot inform the interval.
				continue
			}
			totalGaps += int64(n - 1)
			totalSpanMs += meta.MaxTime - meta.MinTime
		}
		if err := it.Err(); err != nil {
			return 0, false, set.Warnings(), err
		}
		if read >= chunkSampleLimit {
			break
		}
	}

	annos = set.Warnings()
	if err := set.Err(); err != nil {
		return 0, false, annos, err
	}
	if totalGaps == 0 || totalSpanMs <= 0 {
		// Nothing usable was sampled; let the caller fall back to the supplied
		// scrape interval.
		return 0, false, annos, nil
	}
	return float64(totalSpanMs) / float64(totalGaps), true, annos, nil
}

// countSeries counts the series matching matchers over [mint,maxt] without
// iterating their samples, using the supplied querier. The querier is opened by
// the caller over the union of every selector's window and reused for each
// selector, mirroring the engine; the per-selector window is applied through
// the SelectHints. It uses the portable storage.Querier Select path rather than
// reading postings directly, so it works against any storage.Queryable. The
// returned count reflects the storage index and may over-count: see EstimateCost
// for the accuracy limitations.
//
// The count is not capped: SeriesTouched is documented as an upper bound on the
// series the query would read, and stopping early would turn it into a silently
// truncated lower bound. Only context cancellation aborts the walk, and it does
// so with an error rather than a partial count.
func countSeries(ctx context.Context, querier storage.Querier, mint, maxt int64, matchers []*labels.Matcher) (count int64, annos annotations.Annotations, err error) {
	// Func "series" lets the storage know we only need the series labels and not
	// their samples, which keeps the count cheap.
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

// chunkBudgetFits reports whether the chunks of the series matching matchers
// over [mint,maxt] fit within chunkSampleLimit, so that sampleEffectiveInterval
// can measure the selector's density from its real window instead of
// extrapolating from a narrow proxy window.
//
// Reaching a chunk meta through the chunk iterator faults the chunk in and
// CRC-checks it, and a boundary chunk is decoded and re-encoded to trim it to
// the window, so this deliberately does NOT count the selector's chunks: it
// stops walking as soon as the budget is exceeded. At most chunkSampleLimit+1
// chunks are ever faulted in, whatever the selector's real size, which is what
// keeps the estimate cheap for a large selector. The exact count is not needed:
// the caller only compares it against the budget.
//
// Only context cancellation aborts the walk, and it does so with an error rather
// than a partial answer. Select warnings are merged into annos.
func chunkBudgetFits(ctx context.Context, cq storage.ChunkQuerier, mint, maxt int64, matchers []*labels.Matcher) (fits bool, annos annotations.Annotations, err error) {
	set := cq.Select(ctx, false, &storage.SelectHints{Start: mint, End: maxt}, matchers...)

	var (
		examined int64
		it       chunks.Iterator
	)
	for set.Next() {
		if err := ctx.Err(); err != nil {
			return false, set.Warnings(), err
		}
		it = set.At().Iterator(it)
		for it.Next() {
			examined++
			if examined > chunkSampleLimit {
				// Over budget: stop before faulting in any more chunks. The remaining
				// series are not walked at all.
				return false, set.Warnings(), nil
			}
		}
		if err := it.Err(); err != nil {
			return false, set.Warnings(), err
		}
	}

	annos = set.Warnings()
	if err := set.Err(); err != nil {
		return false, annos, err
	}
	// A window with no chunks has nothing to measure, so it does not "fit": the
	// caller falls back to the proxy window.
	return examined > 0, annos, nil
}

// insideStepInvariant reports whether the node at the end of path sits inside a
// StepInvariantExpr. The engine evaluates such a subtree once and copies the
// result to every step, so its selectors are read once rather than once per
// step.
func insideStepInvariant(path []parser.Node) bool {
	for _, node := range path {
		if _, ok := node.(*parser.StepInvariantExpr); ok {
			return true
		}
	}
	return false
}

// defaultSubqueryStep is the subquery resolution EstimateCost falls back to when
// the caller passes a non-positive subqueryDefaultStep. It matches the engine's
// own default subquery evaluation interval
// (config.DefaultGlobalConfig.EvaluationInterval, 1m); callers should pass the
// engine's configured value instead of relying on it.
const defaultSubqueryStep = time.Minute

// samplesPerSeries estimates how many samples a single series contributes to a
// query for one selector, modelling the engine's incremental per-step reads.
//
// For a range (matrix) selector the engine reads the full range window once at
// the first step and then only the samples that advance past the previous
// step's cutoff at each subsequent step, so the estimate is
// samplesPerWindow(range) + (numSteps-1)*samplesAdvancedPerStep(step).
//
// That incremental model only holds while the step is no wider than the range.
// Once the step exceeds the range, consecutive windows no longer overlap and the
// engine re-reads a whole window at every step, so the per-step term is capped
// at samplesPerWindow(range).
//
// For an instant (vector) selector the engine reads roughly one sample per
// evaluation step, so the estimate is numSteps (1 for a pure instant query).
//
// stepMs is the interval between consecutive evaluations of the selector (the
// outer query step, or the subquery resolution for a selector inside a
// subquery). All arithmetic saturates at math.MaxInt64.
func samplesPerSeries(isRange bool, rangeMs, stepMs int64, scrapeInterval time.Duration, numSteps int64) int64 {
	if numSteps < 1 {
		numSteps = 1
	}
	if !isRange {
		// One sample read per step.
		return numSteps
	}
	// Full range window read once at the first step.
	perSeries := samplesPerWindow(rangeMs, scrapeInterval)
	if numSteps > 1 {
		// One step's worth of new samples advanced at each subsequent step. The
		// engine only re-reads the points that move past the previous step's
		// cutoff, so this is the number of scrape intervals the window advances,
		// floor(step/scrapeInterval), without the inclusive boundary sample that
		// samplesPerWindow adds (that sample was already counted by the previous
		// step).
		perStep := samplesAdvancedPerStep(stepMs, scrapeInterval)
		// Windows stop overlapping once the step is wider than the range: the
		// engine then re-reads the whole window at each step rather than only the
		// advanced points, so a step's cost can never exceed one window.
		if perWindow := samplesPerWindow(rangeMs, scrapeInterval); perStep > perWindow {
			perStep = perWindow
		}
		perSeries = addSaturatingInt64(perSeries, mulSaturatingInt64(numSteps-1, perStep))
	}
	return perSeries
}

// innermostSubquery returns the step and range of the innermost subquery on the
// given path, and whether the path crosses any subquery at all. When a subquery
// omits an explicit step, defaultStep (the engine's configured default subquery
// interval) is returned. Only the innermost
// subquery is reported: deeper nesting is approximated by its grid alone and
// does not multiply across every nested level (see EstimateCost limitations).
func innermostSubquery(path []parser.Node, defaultStep time.Duration) (step, rangeDur time.Duration, ok bool) {
	for _, node := range path {
		if sq, isSubq := node.(*parser.SubqueryExpr); isSubq {
			ok = true
			rangeDur = sq.Range
			step = sq.Step
			if step <= 0 {
				step = defaultStep
			}
		}
	}
	return step, rangeDur, ok
}

// subqueryNumSteps returns the number of evaluation steps a selector inside a
// subquery is read across. The engine evaluates the subquery once on its own
// grid spanning [start-range, end] at the subquery's resolution, so the count
// is (end-start+range)/step + 1. It returns at least 1.
func subqueryNumSteps(start, end time.Time, step, rangeDur time.Duration) int64 {
	if step <= 0 {
		return 1
	}
	span := max(end.Sub(start)+rangeDur, 0)
	return int64(span/step) + 1
}

// samplesAdvancedPerStep estimates how many new samples a range selector reads
// when its window advances by stepMs milliseconds at a given scrape interval. It
// is floor(step/scrapeInterval): the number of whole scrape intervals the window
// moves forward, excluding the boundary sample that the previous step already
// read. It returns 0 for a non-positive scrape interval or step.
func samplesAdvancedPerStep(stepMs int64, scrapeInterval time.Duration) int64 {
	if scrapeInterval <= 0 || stepMs <= 0 {
		return 0
	}
	intervalMs := scrapeInterval.Milliseconds()
	if intervalMs <= 0 {
		return 0
	}
	return stepMs / intervalMs
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
