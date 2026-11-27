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
	"fmt"
	"math"
	"slices"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

// correlation_over_time method names.
const (
	correlationMethodPearson  = "pearson"
	correlationMethodSpearman = "spearman"
	correlationMethodKendall  = "kendall"
)

// correlationMethods lists the valid method names; the first is the default.
var correlationMethods = []string{correlationMethodPearson, correlationMethodSpearman, correlationMethodKendall}

// maxKendallCorrelationPairs is the soft warning threshold for Kendall's
// naive O(n²) implementation.
const maxKendallCorrelationPairs = 50_000_000

// pearsonOnSlices computes Pearson's product-moment correlation coefficient
// over two equal-length float slices using a numerically stable two-pass
// algorithm (mean-centering avoids the catastrophic cancellation of the
// naive nΣxy - ΣxΣy / sqrt(...) formula for near-constant inputs),
// see https://en.wikipedia.org/wiki/Algorithms_for_calculating_variance#Two-pass_algorithm
// Returns NaN if n < 2 or either input has zero variance.
func pearsonOnSlices(x, y []float64) float64 {
	n := len(x)
	if n < 2 {
		return math.NaN()
	}

	// First pass: means.
	var sumX, sumY float64
	for i := range n {
		sumX += x[i]
		sumY += y[i]
	}
	nf := float64(n)
	meanX := sumX / nf
	meanY := sumY / nf

	// Second pass: deviations from the mean.
	var sxy, sxx, syy float64
	for i := range n {
		dx := x[i] - meanX
		dy := y[i] - meanY
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}

	den := math.Sqrt(sxx * syy)
	if den == 0 {
		return math.NaN()
	}
	return sxy / den
}

// spearmanCorrelation computes Spearman's rank correlation coefficient by
// ranking both series and computing Pearson's r on the ranks.
func spearmanCorrelation(xFloats, yFloats []FPoint) float64 {
	x, y := alignByTimestamp(xFloats, yFloats)
	if len(x) < 2 {
		return math.NaN()
	}
	return pearsonOnSlices(fractionalRanks(x), fractionalRanks(y))
}

// kendallCorrelation computes Kendall's tau-b coefficient using the naive
// O(n^2) approach. For the typical range-vector sizes in Prometheus this is
// fast enough.
func kendallCorrelation(xFloats, yFloats []FPoint) float64 {
	x, y := alignByTimestamp(xFloats, yFloats)
	return kendallOnSlices(x, y)
}

// kendallOnSlices computes Kendall's tau-b coefficient for already paired
// equal-length slices.
func kendallOnSlices(x, y []float64) float64 {
	n := len(x)
	if n < 2 {
		return math.NaN()
	}

	var concordant, discordant, tiedX, tiedY float64
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			dx := x[i] - x[j]
			dy := y[i] - y[j]
			switch {
			case dx == 0 && dy == 0:
				tiedX++
				tiedY++
			case dx == 0:
				tiedX++
			case dy == 0:
				tiedY++
			case (dx > 0 && dy > 0) || (dx < 0 && dy < 0):
				concordant++
			default:
				discordant++
			}
		}
	}
	nPairs := float64(n) * float64(n-1) / 2
	// Tau-b accounts for ties.
	den := math.Sqrt((nPairs - tiedX) * (nPairs - tiedY))
	if den == 0 {
		return math.NaN()
	}
	return (concordant - discordant) / den
}

// alignByTimestamp returns two float64 slices containing the values of x and y
// at timestamps present in both inputs. Both inputs must be sorted by time.
func alignByTimestamp(xFloats, yFloats []FPoint) ([]float64, []float64) {
	c := min(len(xFloats), len(yFloats))
	x := make([]float64, 0, c)
	y := make([]float64, 0, c)
	i, j := 0, 0
	for i < len(xFloats) && j < len(yFloats) {
		switch {
		case xFloats[i].T < yFloats[j].T:
			i++
		case xFloats[i].T > yFloats[j].T:
			j++
		default:
			x = append(x, xFloats[i].F)
			y = append(y, yFloats[j].F)
			i++
			j++
		}
	}
	return x, y
}

// indexedValue is a helper for fractionalRanks.
type indexedValue struct {
	val float64
	idx int
}

// fractionalRanks returns the fractional ranks of values, handling ties with
// average ranks (the standard method for Spearman).
func fractionalRanks(vals []float64) []float64 {
	n := len(vals)
	iv := make([]indexedValue, n)
	for i, v := range vals {
		iv[i] = indexedValue{val: v, idx: i}
	}
	slices.SortFunc(iv, func(a, b indexedValue) int {
		switch {
		case a.val < b.val:
			return -1
		case a.val > b.val:
			return +1
		default:
			return 0
		}
	})

	ranks := make([]float64, n)
	for i := 0; i < n; {
		j := i + 1
		for j < n && iv[j].val == iv[i].val {
			j++
		}
		// Average rank for ties: ranks are 1-based.
		avg := float64(i+j+1) / 2.0
		for k := i; k < j; k++ {
			ranks[iv[k].idx] = avg
		}
		i = j
	}
	return ranks
}

// evalCorrelationOverTime implements the correlation_over_time PromQL
// function. It takes two range vectors, an optional method string
// ("pearson" (the default), "spearman" or "kendall"), and an
// optional `on` comma-separated label list. Series across the two selectors
// are paired by exact match on all labels except __name__ by default, or by
// exactly the `on` labels when given — letting series with differing extra
// labels (different metrics, different label schemas) still pair, as long as
// they agree on the `on` labels. At each evaluation step it returns the
// correlation coefficient over the float samples whose timestamps appear in
// both range windows (see alignByTimestamp).
//
// The result is NaN when a window has fewer than 2 paired samples or
// when the variance is zero (constant series, single distinct value,
// or all-tied pairs for Kendall). Histogram samples are skipped and
// do not contribute to the correlation. Series without a matching pair
// in the second selector are silently dropped from the output. Multiple
// series in the FIRST selector matching the same second-selector series is
// normal fan-out (e.g. several first-selector series correlated against one
// shared second-selector series) and is not treated as ambiguous. But when
// multiple series in the SECOND selector match the same signature, the pair
// is ambiguous — which one should the first selector's series pair against?
// — so the whole pair is skipped (not picked arbitrarily) and a PromQL
// warning annotation is emitted once.
//
// An unknown method yields an empty result and a PromQL warning
// annotation, mirroring lm_over_time. Kendall is naive O(n^2); large range
// windows can be expensive.
func (ev *evaluator) evalCorrelationOverTime(ctx context.Context, e *parser.Call) (parser.Value, annotations.Annotations) {
	var warnings annotations.Annotations

	// Optional string options, parsed once up front.
	method, methodOK := enumArg(e, 2, correlationMethods, annotations.NewInvalidCorrelationMethodWarning, &warnings)
	on, onOK := onArg(e, 3, &warnings)
	if !methodOK || !onOK {
		return Matrix{}, warnings
	}
	// Kendall is only reachable with an explicit method argument.
	methodPos := e.PositionRange()
	if len(e.Args) > 2 {
		methodPos = e.Args[2].PositionRange()
	}

	// Each range-vector argument can be either a MatrixSelector (e.g.
	// `metric[5m]`) or a SubqueryExpr (e.g. `rate(metric[1m])[5m:]`).
	// Subqueries are materialised into a MatrixSelector with the resulting
	// series, mirroring what the engine does for the standard range-vector
	// FunctionCall path.
	resolveMatrixArg := func(arg parser.Expr) *parser.MatrixSelector {
		switch a := arg.(type) {
		case *parser.MatrixSelector:
			return a
		case *parser.SubqueryExpr:
			ms, _, ws := ev.evalSubquery(ctx, a, durationMilliseconds(a.OriginalOffset), durationMilliseconds(a.Range))
			warnings.Merge(ws)
			return ms
		default:
			ev.error(errWithWarnings{
				fmt.Errorf("correlation_over_time: expected a range-vector argument, got %T", arg),
				warnings,
			})
			return nil
		}
	}
	sel0 := resolveMatrixArg(e.Args[0])
	sel1 := resolveMatrixArg(e.Args[1])

	ws, err := ev.checkAndExpandSeriesSet(ctx, sel0)
	warnings.Merge(ws)
	if err != nil {
		ev.error(errWithWarnings{fmt.Errorf("expanding series: %w", err), warnings})
	}
	ws, err = ev.checkAndExpandSeriesSet(ctx, sel1)
	warnings.Merge(ws)
	if err != nil {
		ev.error(errWithWarnings{fmt.Errorf("expanding series: %w", err), warnings})
	}

	vs0 := sel0.VectorSelector.(*parser.VectorSelector)
	vs1 := sel1.VectorSelector.(*parser.VectorSelector)

	// Index the second selector by matching signature. Ambiguous signatures
	// (several series) are left out, so their pairs are skipped with a warning.
	dropName := []string{model.MetricNameLabel}
	sig1Map := uniqueSigIndex(vs1.Series, on, dropName, e.Args[1].PositionRange(),
		annotations.NewAmbiguousCorrelationPairWarning, &warnings)
	var hashBuf []byte

	selRange0 := durationMilliseconds(sel0.Range)
	selRange1 := durationMilliseconds(sel1.Range)
	offset0 := durationMilliseconds(vs0.Offset)
	offset1 := durationMilliseconds(vs1.Offset)

	numSteps := int(1 + (ev.endTimestamp-ev.startTimestamp)/ev.interval)

	it0 := ev.newMatrixIterator(ctx, selRange0)
	it1 := ev.newMatrixIterator(ctx, selRange1)

	var chkIter0, chkIter1 chunkenc.Iterator

	var mat Matrix

	// Process each series from the first selector and find its pair.
	for _, s0 := range vs0.Series {
		var sig uint64
		sig, hashBuf = on.sig(s0.Labels(), hashBuf, dropName...)
		j, ok := sig1Map[sig]
		if !ok {
			continue // No matching pair in the second selector.
		}
		s1 := vs1.Series[j]

		if err := contextDone(ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}

		chkIter0 = s0.Iterator(chkIter0)
		it0.Reset(chkIter0)
		chkIter1 = s1.Iterator(chkIter1)
		it1.Reset(chkIter1)

		// Output metric: drop __name__ from the first selector's labels.
		metric := s0.Labels()
		metricName := metric.Get(model.MetricNameLabel)
		if !ev.enableDelayedNameRemoval {
			metric = metric.DropReserved(schema.IsMetadataLabel)
		}

		ss := Series{
			Metric:   metric,
			DropName: true,
		}

		// Reused across steps by matrixIterSlice; do not retain references
		// to the returned slices beyond the current step.
		var floats0, floats1 []FPoint
		var hists0, hists1 []HPoint
		histogramsSeen := false
		largeKendallRangeSeen := false

		step := -1
		for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
			step++
			maxt0 := ts - offset0
			mint0 := maxt0 - selRange0
			maxt1 := ts - offset1
			mint1 := maxt1 - selRange1

			floats0, hists0, _ = ev.matrixIterSlice(ctx, it0, mint0, maxt0, floats0, hists0, nil, readStep(e.Args[0], step))
			floats1, hists1, _ = ev.matrixIterSlice(ctx, it1, mint1, maxt1, floats1, hists1, nil, readStep(e.Args[1], step))
			if !histogramsSeen && (len(hists0) > 0 || len(hists1) > 0) {
				histogramsSeen = true
				warnings.Add(annotations.NewHistogramIgnoredInMixedRangeInfo(metricName, e.Args[0].PositionRange()))
			}

			if len(floats0) == 0 || len(floats1) == 0 {
				continue
			}

			var r float64
			if len(floats0) < 2 || len(floats1) < 2 {
				r = math.NaN()
			} else {
				x, y := alignByTimestamp(floats0, floats1)
				switch method {
				case correlationMethodPearson:
					r = pearsonOnSlices(x, y)
				case correlationMethodSpearman:
					r = pearsonOnSlices(fractionalRanks(x), fractionalRanks(y))
				case correlationMethodKendall:
					nPairs := len(x) * (len(x) - 1) / 2
					if nPairs > maxKendallCorrelationPairs && !largeKendallRangeSeen {
						largeKendallRangeSeen = true
						warnings.Add(annotations.NewLargeKendallCorrelationRangeInfo(nPairs, methodPos))
					}
					r = kendallOnSlices(x, y)
				}
			}

			if ss.Floats == nil {
				ss.Floats = getFPointSlice(numSteps)
			}
			ev.currentSamples++
			ev.samplesStats.IncrementSamplesAtStep(step, 1)
			if ev.currentSamples > ev.maxSamples {
				ev.error(ErrTooManySamples(env))
			}
			ss.Floats = append(ss.Floats, FPoint{F: r, T: ts})
		}

		if len(ss.Floats) > 0 {
			mat = append(mat, ss)
		}
		ev.samplesStats.UpdatePeak(ev.currentSamples)
		ev.currentSamples -= len(floats0) + len(floats1) + totalHPointSize(hists0) + totalHPointSize(hists1)
		putFPointSlice(floats0)
		putFPointSlice(floats1)
		putMatrixSelectorHPointSlice(hists0)
		putMatrixSelectorHPointSlice(hists1)

		// Reduce buffer for next step optimization.
		stepRange0 := min(selRange0, ev.interval)
		stepRange1 := min(selRange1, ev.interval)
		it0.ReduceDelta(stepRange0)
		it1.ReduceDelta(stepRange1)
	}

	if !ev.enableDelayedNameRemoval && mat.ContainsSameLabelset() {
		ev.errorf("vector cannot contain metrics with the same labelset")
	}
	return mat, warnings
}

// readStep returns the evaluation step to which matrixIterSlice attributes
// newly read storage points for the range-vector argument arg. Subquery
// results are already charged during subquery evaluation, so they use -1.
func readStep(arg parser.Expr, step int) int {
	if _, ok := arg.(*parser.SubqueryExpr); ok {
		return -1
	}
	return step
}
