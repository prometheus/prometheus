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

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

// Regression output selector constants. They choose which scalar the closed-form
// ordinary-least-squares fit of the dependent series y on the independent series
// x returns at each evaluation step.
const (
	regressionOutputSlope      = 0 // The regression coefficient β₁.
	regressionOutputIntercept  = 1 // The intercept β₀.
	regressionOutputPrediction = 2 // The fitted value ŷ at the most recent x in the window.
	regressionOutputR2         = 3 // The coefficient of determination r².
)

// Regression link function constants. The link relates the linear predictor to
// the dependent variable. The log link regresses ln(y) on x, yielding the
// multiplicative model y ≈ exp(β₀)·exp(β₁·x) that suits non-negative
// count-like series, mirroring the log link of count time-series GLMs.
const (
	regressionLinkIdentity = 0
	regressionLinkLog      = 1
)

// regressionFit holds the closed-form ordinary-least-squares fit of y on x.
type regressionFit struct {
	slope     float64
	intercept float64
	r2        float64
	xLatest   float64 // The independent value at the most recent paired timestamp.
	ok        bool    // False when the fit is undefined (n < 2 or zero variance in x).
}

// olsOnSlices computes the ordinary-least-squares fit of y on x for two
// equal-length, timestamp-aligned float slices using a numerically stable
// two-pass algorithm (mean-centering avoids catastrophic cancellation). The
// last element is assumed to be the most recent sample. The fit is marked not
// ok (and slope/intercept/r² are NaN) when there are fewer than two pairs or
// when x has zero variance.
func olsOnSlices(x, y []float64) regressionFit {
	n := len(x)
	if n < 2 {
		return regressionFit{slope: math.NaN(), intercept: math.NaN(), r2: math.NaN()}
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

	if sxx == 0 {
		// No variation in the independent variable: slope is undefined.
		return regressionFit{slope: math.NaN(), intercept: math.NaN(), r2: math.NaN(), xLatest: x[n-1]}
	}

	slope := sxy / sxx
	intercept := meanY - slope*meanX
	// r² equals Pearson's r squared; it is left undefined (NaN) when y is
	// constant, since there is no variance to explain.
	r2 := math.NaN()
	if syy != 0 {
		r2 = (sxy * sxy) / (sxx * syy)
	}
	return regressionFit{
		slope:     slope,
		intercept: intercept,
		r2:        r2,
		xLatest:   x[n-1],
		ok:        true,
	}
}

// logTransformPairs returns the x and y pairs with y replaced by ln(y), dropping
// pairs whose y is not strictly positive. The returned dropped count is the
// number of pairs removed because of a non-positive dependent value.
func logTransformPairs(x, y []float64) (outX, outY []float64, dropped int) {
	outX = make([]float64, 0, len(x))
	outY = make([]float64, 0, len(y))
	for i := range x {
		if y[i] <= 0 {
			dropped++
			continue
		}
		outX = append(outX, x[i])
		outY = append(outY, math.Log(y[i]))
	}
	return outX, outY, dropped
}

// selectRegressionOutput maps an output selector and the fit to the scalar the
// function returns. For the log link the prediction is back-transformed with
// exp; slope, intercept and r² stay on the linear (ln) scale, matching how a
// log-link GLM reports its coefficients.
func selectRegressionOutput(output, link int, fit regressionFit) float64 {
	if !fit.ok {
		return math.NaN()
	}
	switch output {
	case regressionOutputSlope:
		return fit.slope
	case regressionOutputIntercept:
		return fit.intercept
	case regressionOutputPrediction:
		pred := fit.intercept + fit.slope*fit.xLatest
		if link == regressionLinkLog {
			return math.Exp(pred)
		}
		return pred
	case regressionOutputR2:
		return fit.r2
	default:
		return math.NaN()
	}
}

// evalRegressionOverTime implements the regression_over_time PromQL function. It
// takes two range vectors — the dependent series y (first) and the independent
// series x (second) — and two optional scalar arguments: an output selector
// (0=slope, 1=intercept, 2=prediction, 3=r²; defaults to slope) and a link
// function (0=identity, 1=log; defaults to identity). Series are paired across
// the two selectors by exact match on all labels except __name__. At each
// evaluation step it fits an ordinary-least-squares regression of y on x over
// the float samples whose timestamps appear in both range windows (see
// alignByTimestamp) and returns the selected scalar.
//
// This generalises correlation_over_time: where correlation returns the unitless
// association coefficient, regression returns the predictive line itself
// (β₁ = r·σy/σx) and, optionally, a forecast. With the log link it fits ln(y) on
// x, the count-friendly multiplicative model used by count time-series GLMs.
//
// The result is NaN when a window has fewer than two paired samples, when x has
// zero variance, or when an output/link selector is invalid (a PromQL warning is
// then emitted). Histogram samples are skipped and do not contribute. Under the
// log link, samples with non-positive y are dropped (a PromQL info annotation is
// emitted). Series without a matching pair in the second selector are silently
// dropped. When multiple series in the second selector match the same label
// signature the first encountered series is used and a PromQL warning is emitted.
func (ev *evaluator) evalRegressionOverTime(ctx context.Context, e *parser.Call) (parser.Value, annotations.Annotations) {
	var warnings annotations.Annotations

	// Evaluate the optional output and link scalar arguments. Scalar
	// expressions produce one sample per query step, so per-step validation
	// happens inside the step loop.
	resolveScalarArg := func(idx int) (Matrix, bool) {
		if len(e.Args) <= idx {
			return nil, false
		}
		val, ws := ev.eval(ctx, e.Args[idx])
		warnings.Merge(ws)
		m, ok := val.(Matrix)
		if !ok {
			ev.error(errWithWarnings{
				fmt.Errorf("regression_over_time: expected scalar argument %d, got %T", idx+1, val),
				warnings,
			})
		}
		return m, true
	}
	outputMat, outputHasArg := resolveScalarArg(2)
	linkMat, linkHasArg := resolveScalarArg(3)

	// Each range-vector argument can be either a MatrixSelector (e.g.
	// `metric[5m]`) or a SubqueryExpr (e.g. `rate(metric[1m])[5m:]`).
	// Subqueries are materialised into a MatrixSelector, mirroring the
	// standard range-vector FunctionCall path.
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
				fmt.Errorf("regression_over_time: expected a range-vector argument, got %T", arg),
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

	// Build a map from label-signature (without __name__) to series index for
	// the second selector. Reuse a single byte buffer across hash calls to
	// avoid per-series allocations. Collisions are warned (non-deterministic
	// pairing) and the first series wins.
	sig1Map := make(map[uint64]int, len(vs1.Series))
	ambiguousReported := make(map[uint64]struct{})
	var hashBuf []byte
	for i, s := range vs1.Series {
		var sig uint64
		sig, hashBuf = s.Labels().HashWithoutLabels(hashBuf, model.MetricNameLabel)
		if _, dup := sig1Map[sig]; dup {
			if _, seen := ambiguousReported[sig]; !seen {
				ambiguousReported[sig] = struct{}{}
				warnings.Add(annotations.NewAmbiguousRegressionPairWarning(
					s.Labels().DropReserved(schema.IsMetadataLabel).String(),
					e.Args[1].PositionRange(),
				))
			}
			continue
		}
		sig1Map[sig] = i
	}

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
		sig, hashBuf = s0.Labels().HashWithoutLabels(hashBuf, model.MetricNameLabel)
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

		// Reused across steps by matrixIterSlice; do not retain references to
		// the returned slices beyond the current step.
		var floats0, floats1 []FPoint
		var hists0, hists1 []HPoint
		histogramsSeen := false
		nonPositiveLogSeen := false

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

			output := regressionOutputSlope
			if outputHasArg && len(outputMat) > 0 && len(outputMat[0].Floats) > step {
				output = int(outputMat[0].Floats[step].F)
			}
			link := regressionLinkIdentity
			if linkHasArg && len(linkMat) > 0 && len(linkMat[0].Floats) > step {
				link = int(linkMat[0].Floats[step].F)
			}
			validOutput := output >= regressionOutputSlope && output <= regressionOutputR2
			validLink := link == regressionLinkIdentity || link == regressionLinkLog
			if !validOutput && outputHasArg {
				warnings.Add(annotations.NewInvalidRegressionOutputWarning(outputMat[0].Floats[step].F, e.Args[2].PositionRange()))
			}
			if !validLink && linkHasArg {
				warnings.Add(annotations.NewInvalidRegressionLinkWarning(linkMat[0].Floats[step].F, e.Args[3].PositionRange()))
			}

			if len(floats0) == 0 || len(floats1) == 0 {
				continue
			}

			var r float64
			switch {
			case !validOutput || !validLink || len(floats0) < 2 || len(floats1) < 2:
				r = math.NaN()
			default:
				// y is the dependent (first) series, x the independent (second).
				y, x := alignByTimestamp(floats0, floats1)
				if link == regressionLinkLog {
					var dropped int
					x, y, dropped = logTransformPairs(x, y)
					if dropped > 0 && !nonPositiveLogSeen {
						nonPositiveLogSeen = true
						warnings.Add(annotations.NewNonPositiveRegressionLogValueInfo(e.Args[0].PositionRange()))
					}
				}
				r = selectRegressionOutput(output, link, olsOnSlices(x, y))
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
