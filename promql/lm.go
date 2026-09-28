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
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/schema"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

// lm_over_time method names and flags.
const (
	lmMethodOLS   = "lm"
	lmMethodRidge = "ridge"
	// lmFlagDiff fits on the first differences (Δ-on-Δ) of the response and
	// predictors rather than their levels, removing a shared time trend so the
	// coefficients and r² reflect co-movement instead of a common drift.
	lmFlagDiff = "diff"
	// lmFlagWLS applies exponential time-decay weights (weighted least squares),
	// so recent samples count more than old ones — a better fit for monitoring,
	// where the current relationship matters most. It reads an optional half-life
	// scalar (in seconds) as the last argument.
	lmFlagWLS = "wls"
)

// parseLMMethod splits the method argument into its base method ("lm" or
// "ridge") and optional comma-separated flags. Recognised flags are "diff"
// (first-difference the data) and "wls" (exponential time-decay weighting). It
// returns ok=false for an unknown base method or flag.
func parseLMMethod(s string) (base string, difference, weighted, ok bool) {
	parts := strings.Split(s, ",")
	base = parts[0]
	for _, f := range parts[1:] {
		switch f {
		case lmFlagDiff:
			difference = true
		case lmFlagWLS:
			weighted = true
		default:
			return base, false, false, false
		}
	}
	return base, difference, weighted, base == lmMethodOLS || base == lmMethodRidge
}

// firstDifference replaces the design/response with consecutive first
// differences (row i minus row i−1), leaving the intercept column (0) at 1 so
// its coefficient estimates the per-step drift. It assumes rows are in
// ascending timestamp order and removes a shared trend/level, so the slope
// reflects co-movement rather than a common time trend. The result has one
// fewer row; fewer than two input rows yield an empty result.
func firstDifference(design [][]float64, resp []float64) ([][]float64, []float64) {
	n := len(design)
	if n < 2 {
		return nil, nil
	}
	p := len(design[0])
	dDesign := make([][]float64, n-1)
	dResp := make([]float64, n-1)
	for i := 1; i < n; i++ {
		row := make([]float64, p)
		row[0] = 1
		for j := 1; j < p; j++ {
			row[j] = design[i][j] - design[i-1][j]
		}
		dDesign[i-1] = row
		dResp[i-1] = resp[i] - resp[i-1]
	}
	return dDesign, dResp
}

// firstDifferenceSlice returns the consecutive first differences of v (v[i] −
// v[i−1]), assuming v is in ascending timestamp order. The result has one fewer
// element; fewer than two inputs yield nil.
func firstDifferenceSlice(v []float64) []float64 {
	if len(v) < 2 {
		return nil
	}
	d := make([]float64, len(v)-1)
	for i := 1; i < len(v); i++ {
		d[i-1] = v[i] - v[i-1]
	}
	return d
}

// timeDecayWeightedCopy returns copies of design and resp with each row i scaled
// by sqrt(wᵢ), where wᵢ = 0.5^(ageᵢ/halflife) decays exponentially with the
// sample's age measured from the most recent row. times[i] is row i's timestamp
// (ms, ascending). halflifeMs must be > 0. Scaling the rows by sqrt(wᵢ) turns
// the ordinary least-squares solve into weighted least squares, so recent
// samples influence the fit more than old ones. The originals are not mutated.
func timeDecayWeightedCopy(design [][]float64, resp []float64, times []int64, halflifeMs float64) ([][]float64, []float64) {
	n := len(design)
	if n == 0 {
		return design, resp
	}
	maxT := times[n-1] // Ascending order, so the last row is the most recent.
	wDesign := make([][]float64, n)
	wResp := make([]float64, n)
	for i := range design {
		age := float64(maxT - times[i])
		// sqrt(0.5^(age/halflife)) = 2^(-age/(2·halflife)).
		s := math.Exp2(-age / (2 * halflifeMs))
		row := make([]float64, len(design[i]))
		for j := range design[i] {
			row[j] = design[i][j] * s
		}
		wDesign[i] = row
		wResp[i] = resp[i] * s
	}
	return wDesign, wResp
}

// lmInterceptLabelValue is the reserved value placed on the pivot label of the
// emitted intercept coefficient series. A predictor whose pivot value equals
// this string would be indistinguishable from the intercept, so such a group
// is skipped with a warning.
const lmInterceptLabelValue = "(intercept)"

// lmR2LabelValue is the reserved pivot-label value for the emitted
// coefficient-of-determination (r²) series, which reports the fit quality
// (fraction of the response variance explained) alongside the coefficients.
// It lets callers tell a meaningful fit from a spurious one, which the
// coefficients alone cannot.
const lmR2LabelValue = "(r2)"

// maxLMPredictors caps the number of distinct pivot-label values (design-matrix
// columns) per group. The per-step solve is O(n·p² + p³), so this is set far
// below timeseries_gen's series cap to keep worst-case latency bounded.
const maxLMPredictors = 256

// qrRankTolerance is the absolute threshold below which a reflected column norm
// is treated as zero (rank-deficient design).
const qrRankTolerance = 1e-9

// householderLeastSquares solves min ||A·x - b|| for the least-squares
// coefficient vector x using Householder QR. A is n×p row-major (n ≥ p); b has
// length n. It returns the length-p solution and ok=true, or ok=false when the
// design is rank deficient (a reflected column norm falls below
// qrRankTolerance, i.e. collinear columns or too few rows). A and b are copied,
// not mutated.
func householderLeastSquares(a [][]float64, b []float64) ([]float64, bool) {
	n := len(a)
	if n == 0 {
		return nil, false
	}
	p := len(a[0])
	if n < p || p == 0 {
		return nil, false
	}

	// Work on copies so callers can reuse their buffers.
	r := make([][]float64, n)
	for i := range a {
		r[i] = slices.Clone(a[i])
	}
	y := slices.Clone(b)

	for k := range p {
		// Norm of the sub-column r[k:n][k].
		var sigma float64
		for i := k; i < n; i++ {
			sigma += r[i][k] * r[i][k]
		}
		sigma = math.Sqrt(sigma)
		if sigma < qrRankTolerance {
			return nil, false
		}
		// alpha = -sign(x_k)·||x|| chosen to avoid cancellation in v_k.
		if r[k][k] > 0 {
			sigma = -sigma
		}
		// Householder vector v lives in r[k:n][k]; v_k = x_k - alpha.
		r[k][k] -= sigma
		var vtv float64
		for i := k; i < n; i++ {
			vtv += r[i][k] * r[i][k]
		}
		if vtv < qrRankTolerance {
			return nil, false
		}
		// Reflect the trailing columns: w -= (2·vᵀw/vᵀv)·v.
		for j := k + 1; j < p; j++ {
			var s float64
			for i := k; i < n; i++ {
				s += r[i][k] * r[i][j]
			}
			s = 2 * s / vtv
			for i := k; i < n; i++ {
				r[i][j] -= s * r[i][k]
			}
		}
		// Reflect b.
		var s float64
		for i := k; i < n; i++ {
			s += r[i][k] * y[i]
		}
		s = 2 * s / vtv
		for i := k; i < n; i++ {
			y[i] -= s * r[i][k]
		}
		// The R diagonal is alpha; the reflector below it is no longer needed.
		r[k][k] = sigma
	}

	// Back-substitution on the upper-triangular R (p×p) against y[0:p].
	x := make([]float64, p)
	for i := p - 1; i >= 0; i-- {
		sum := y[i]
		for j := i + 1; j < p; j++ {
			sum -= r[i][j] * x[j]
		}
		if math.Abs(r[i][i]) < qrRankTolerance {
			return nil, false
		}
		x[i] = sum / r[i][i]
	}
	return x, true
}

// householderLeastSquaresPivoted solves min ||A·x − b|| like
// householderLeastSquares but is rank-revealing: instead of failing on a
// rank-deficient design, it runs column-pivoted Householder QR (Businger–Golub)
// to find the maximal set of independent columns, solves for their
// coefficients, and returns NaN for every dependent or (near-)constant column
// that had to be dropped. A is n×p row-major; b has length n. It returns the
// length-p coefficient vector (NaN in dropped positions), the numerical rank r
// (count of solved columns), and ok=false only when r==0 or there are fewer
// rows than the rank (underdetermined, no honest fit). A and b are copied, not
// mutated. A dropped column contributes 0 to the fitted model.
func householderLeastSquaresPivoted(a [][]float64, b []float64) ([]float64, int, bool) {
	n := len(a)
	if n == 0 {
		return nil, 0, false
	}
	p := len(a[0])
	if p == 0 {
		return nil, 0, false
	}

	r := make([][]float64, n)
	for i := range a {
		r[i] = slices.Clone(a[i])
	}
	y := slices.Clone(b)
	perm := make([]int, p) // perm[k] = original index of the column now at k.
	for j := range perm {
		perm[j] = j
	}

	// subColNorm returns ||r[k:n][j]||, the residual norm used for pivoting.
	subColNorm := func(k, j int) float64 {
		var s float64
		for i := k; i < n; i++ {
			s += r[i][j] * r[i][j]
		}
		return math.Sqrt(s)
	}

	rank := 0
	kMax := min(n, p)
	for k := range kMax {
		// Pivot: move the largest-residual-norm column into position k.
		best, bestNorm := k, subColNorm(k, k)
		for j := k + 1; j < p; j++ {
			if nj := subColNorm(k, j); nj > bestNorm {
				best, bestNorm = j, nj
			}
		}
		if bestNorm < qrRankTolerance {
			break // Remaining columns are all (near-)dependent; rank = k.
		}
		if best != k {
			for i := range r {
				r[i][k], r[i][best] = r[i][best], r[i][k]
			}
			perm[k], perm[best] = perm[best], perm[k]
		}

		// Householder reflector for the sub-column r[k:n][k].
		var sigma float64
		for i := k; i < n; i++ {
			sigma += r[i][k] * r[i][k]
		}
		sigma = math.Sqrt(sigma)
		if r[k][k] > 0 {
			sigma = -sigma
		}
		r[k][k] -= sigma
		var vtv float64
		for i := k; i < n; i++ {
			vtv += r[i][k] * r[i][k]
		}
		if vtv < qrRankTolerance {
			break
		}
		for j := k + 1; j < p; j++ {
			var s float64
			for i := k; i < n; i++ {
				s += r[i][k] * r[i][j]
			}
			s = 2 * s / vtv
			for i := k; i < n; i++ {
				r[i][j] -= s * r[i][k]
			}
		}
		var s float64
		for i := k; i < n; i++ {
			s += r[i][k] * y[i]
		}
		s = 2 * s / vtv
		for i := k; i < n; i++ {
			y[i] -= s * r[i][k]
		}
		r[k][k] = sigma
		rank++
	}

	if rank == 0 || n < rank {
		return nil, rank, false
	}

	// Back-substitute over the rank×rank upper-triangular block against y[0:rank].
	solved := make([]float64, rank)
	for i := rank - 1; i >= 0; i-- {
		sum := y[i]
		for j := i + 1; j < rank; j++ {
			sum -= r[i][j] * solved[j]
		}
		if math.Abs(r[i][i]) < qrRankTolerance {
			return nil, rank, false
		}
		solved[i] = sum / r[i][i]
	}

	// Scatter back to original column order; dropped columns get NaN.
	coeffs := make([]float64, p)
	for j := range coeffs {
		coeffs[j] = math.NaN()
	}
	for i := range rank {
		coeffs[perm[i]] = solved[i]
	}
	return coeffs, rank, true
}

// ridgeAugment appends p-1 penalty rows to the design matrix and zero entries to
// the response so that householderLeastSquares yields the ridge estimator
// β = (XᵀX + λI)⁻¹Xᵀy, with the intercept (column 0) left unpenalized. lambda
// must be > 0; the design matrix columns are [intercept, predictor₁, …].
func ridgeAugment(design [][]float64, resp []float64, lambda float64) ([][]float64, []float64) {
	p := len(design[0])
	sqrtLambda := math.Sqrt(lambda)
	aug := make([][]float64, len(design), len(design)+p-1)
	copy(aug, design)
	augResp := make([]float64, len(resp), len(resp)+p-1)
	copy(augResp, resp)
	for j := 1; j < p; j++ { // Skip column 0 (intercept).
		row := make([]float64, p)
		row[j] = sqrtLambda
		aug = append(aug, row)
		augResp = append(augResp, 0)
	}
	return aug, augResp
}

// lmR2 returns the coefficient of determination (r² = 1 − SS_res/SS_tot) of the
// fit with the given coefficients over the (unaugmented) design rows. It is NaN
// when there are no rows or the response has zero variance.
func lmR2(design [][]float64, resp, coeffs []float64) float64 {
	n := len(resp)
	if n == 0 {
		return math.NaN()
	}
	var mean float64
	for _, y := range resp {
		mean += y
	}
	mean /= float64(n)
	var ssRes, ssTot float64
	for i := range resp {
		var pred float64
		for j := range coeffs {
			if math.IsNaN(coeffs[j]) {
				continue // Dropped (rank-deficient) column contributes 0.
			}
			pred += coeffs[j] * design[i][j]
		}
		d := resp[i] - pred
		ssRes += d * d
		dt := resp[i] - mean
		ssTot += dt * dt
	}
	if ssTot == 0 {
		return math.NaN()
	}
	return 1 - ssRes/ssTot
}

// lmPredictor is one design-matrix column: the pivot-label value and the index
// of the contributing series in the second range vector.
type lmPredictor struct {
	value     string
	seriesIdx int
}

// evalLMOverTime implements the lm_over_time PromQL function. It fits a multiple
// linear regression of the response range vector y on the predictor range
// vector X at each evaluation step, pivoting X into a design matrix whose
// columns are the distinct values of labelName.
//
// Signature: lm_over_time(method string, y range-vector, X range-vector,
// labelName string, on="" string, lambda=0 scalar, halflife=0 scalar). method
// is "lm" (ordinary least squares) or "ridge" (L2-penalized, requires
// lambda > 0), optionally with the comma-separated ",diff" flag (e.g.
// "ridge,diff") to fit on first differences (Δ-on-Δ), which removes a shared
// time trend so the coefficients and r² reflect co-movement rather than
// common drift. Predictor series are grouped by all labels except __name__
// and labelName by default, or by exactly the `on` labels when given — the
// latter lets y and X (or X's own predictors) carry differing extra labels
// and still group together, as long as they agree on the `on` labels. Each
// group is one regression, and its distinct labelName values become the
// design-matrix columns. The response series is matched to a group by those
// same grouping labels. Each emitted series carries the group's labels plus
// labelName set to the predictor's value (or the reserved "(intercept)" for
// the intercept), and its value is the fitted coefficient.
//
// When labelName is empty the function degenerates to the bivariate case and
// returns the regression slope per matched pair, matching regression_over_time
// with the default (slope) output.
//
// For the OLS path the solve is rank-revealing (column-pivoted QR): when the
// design is rank deficient (a collinear or near-constant predictor) it drops
// only the offending columns — their coefficients are NaN — and still solves
// for the remaining predictors, emitting an info annotation naming what was
// dropped, so one degenerate predictor no longer NaNs the whole model. The
// ridge path is full rank by construction and always solves all columns. A step
// still yields all-NaN coefficients only when there are fewer samples than
// columns (nothing left to solve). Groups whose predictor cardinality exceeds
// maxLMPredictors, or that include a predictor whose pivot value collides with
// "(intercept)" or "(r2)", are skipped with a warning. Two ambiguity cases are
// also skipped rather than resolved arbitrarily: more than one response series
// matching the same group (which response should the fit use?), and — new
// once `on` can narrow the grouping key — two predictor series in the same
// group ending up with the same labelName pivot value (which would produce
// two design-matrix columns with the same header). Histogram samples are
// ignored.
func (ev *evaluator) evalLMOverTime(ctx context.Context, e *parser.Call) (parser.Value, annotations.Annotations) {
	var warnings annotations.Annotations

	methodArg := stringFromArg(e.Args[0])
	method, difference, weighted, ok := parseLMMethod(methodArg)
	if !ok {
		warnings.Add(annotations.NewInvalidLMMethodWarning(methodArg, e.Args[0].PositionRange()))
		return Matrix{}, warnings
	}
	labelName := stringFromArg(e.Args[3])

	// Optional `on` matching key; see parseOnMatching.
	on, ok := onArg(e, 4, &warnings)
	if !ok {
		return Matrix{}, warnings
	}

	// Optional lambda scalar, evaluated per step like correlation's method arg.
	var lambdaMat Matrix
	lambdaHasArg := len(e.Args) > 5
	if lambdaHasArg {
		val, ws := ev.eval(ctx, e.Args[5])
		warnings.Merge(ws)
		var ok bool
		lambdaMat, ok = val.(Matrix)
		if !ok {
			ev.error(errWithWarnings{
				fmt.Errorf("lm_over_time: expected scalar lambda argument, got %T", val),
				warnings,
			})
		}
	}

	// Optional half-life scalar (seconds) for the ,wls exponential time-decay
	// weights, evaluated per step like lambda.
	var halflifeMat Matrix
	halflifeHasArg := len(e.Args) > 6
	if halflifeHasArg {
		val, ws := ev.eval(ctx, e.Args[6])
		warnings.Merge(ws)
		var ok bool
		halflifeMat, ok = val.(Matrix)
		if !ok {
			ev.error(errWithWarnings{
				fmt.Errorf("lm_over_time: expected scalar half-life argument, got %T", val),
				warnings,
			})
		}
	}

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
				fmt.Errorf("lm_over_time: expected a range-vector argument, got %T", arg),
				warnings,
			})
			return nil
		}
	}
	selY := resolveMatrixArg(e.Args[1])
	selX := resolveMatrixArg(e.Args[2])

	ws, err := ev.checkAndExpandSeriesSet(ctx, selY)
	warnings.Merge(ws)
	if err != nil {
		ev.error(errWithWarnings{fmt.Errorf("expanding series: %w", err), warnings})
	}
	ws, err = ev.checkAndExpandSeriesSet(ctx, selX)
	warnings.Merge(ws)
	if err != nil {
		ev.error(errWithWarnings{fmt.Errorf("expanding series: %w", err), warnings})
	}

	vsY := selY.VectorSelector.(*parser.VectorSelector)
	vsX := selX.VectorSelector.(*parser.VectorSelector)

	rangeY := durationMilliseconds(selY.Range)
	rangeX := durationMilliseconds(selX.Range)
	offsetY := durationMilliseconds(vsY.Offset)
	offsetX := durationMilliseconds(vsX.Offset)
	numSteps := int(1 + (ev.endTimestamp-ev.startTimestamp)/ev.interval)

	lambdaAt := func(step int) float64 {
		if lambdaHasArg && len(lambdaMat) > 0 && len(lambdaMat[0].Floats) > step {
			return lambdaMat[0].Floats[step].F
		}
		return 0
	}
	halflifeAt := func(step int) float64 {
		if halflifeHasArg && len(halflifeMat) > 0 && len(halflifeMat[0].Floats) > step {
			return halflifeMat[0].Floats[step].F
		}
		return 0
	}

	// Group by all labels except __name__ and, defensively, labelName by
	// default, or by exactly the `on` labels. A group key matched by several
	// response series is ambiguous, so that group is skipped with a warning.
	groupDrop := []string{model.MetricNameLabel}
	if labelName != "" {
		groupDrop = append(groupDrop, labelName)
	}
	respBySig := uniqueSigIndex(vsY.Series, on, groupDrop, e.Args[1].PositionRange(),
		annotations.NewAmbiguousLMResponseWarning, &warnings)
	var hashBuf []byte

	if labelName == "" {
		if weighted {
			// Time-decay weighting is only wired into the pivot (multiple-
			// regression) path for now; warn rather than silently ignore.
			warnings.Add(annotations.NewWLSRequiresPivotWarning(e.Args[0].PositionRange()))
		}
		return ev.lmBivariate(ctx, e, selX, selY, vsX, vsY, respBySig, difference, on,
			rangeX, rangeY, offsetX, offsetY, numSteps, &warnings)
	}

	// Group predictor series by the grouping signature.
	type lmGroup struct {
		sig        uint64
		metric     labels.Labels // Group labels: the `on` labels when explicit.
		predictors []lmPredictor
	}
	groupBySig := make(map[uint64]*lmGroup)
	var groups []*lmGroup
	for i, s := range vsX.Series {
		var sig uint64
		sig, hashBuf = on.sig(s.Labels(), hashBuf, groupDrop...)
		g := groupBySig[sig]
		if g == nil {
			g = &lmGroup{sig: sig, metric: on.keep(s.Labels().DropReserved(schema.IsMetadataLabel))}
			groupBySig[sig] = g
			groups = append(groups, g)
		}
		g.predictors = append(g.predictors, lmPredictor{value: s.Labels().Get(labelName), seriesIdx: i})
	}

	var mat Matrix
	var chkIter chunkenc.Iterator

	for _, g := range groups {
		yIdx, ok := respBySig[g.sig]
		if !ok {
			continue // No response series for this predictor group.
		}
		// Deterministic column order by pivot value.
		slices.SortFunc(g.predictors, func(a, b lmPredictor) int {
			switch {
			case a.value < b.value:
				return -1
			case a.value > b.value:
				return +1
			default:
				return 0
			}
		})
		if len(g.predictors) > maxLMPredictors {
			warnings.Add(annotations.NewTooManyLMPredictorsWarning(len(g.predictors), maxLMPredictors, e.Args[2].PositionRange()))
			continue
		}
		reserved := false
		for _, pr := range g.predictors {
			if pr.value == lmInterceptLabelValue || pr.value == lmR2LabelValue {
				reserved = true
				break
			}
		}
		if reserved {
			warnings.Add(annotations.NewReservedLMInterceptLabelWarning(g.metric.String(), e.Args[2].PositionRange()))
			continue
		}

		// Two predictors sharing the same pivot value would produce two
		// design-matrix columns with the same header — typically because `on`
		// dropped the label that used to distinguish them. g.predictors is
		// sorted by value, so a collision is an adjacent duplicate. Skip the
		// whole group rather than guessing which one to keep.
		ambiguousPredictor := false
		for j := 1; j < len(g.predictors); j++ {
			if g.predictors[j].value == g.predictors[j-1].value {
				ambiguousPredictor = true
				warnings.Add(annotations.NewAmbiguousLMPredictorValueWarning(
					g.metric.String(), g.predictors[j].value, e.Args[2].PositionRange(),
				))
				break
			}
		}
		if ambiguousPredictor {
			continue
		}

		k := len(g.predictors)

		if err := contextDone(ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}

		// One buffered iterator for the response and each predictor.
		respIt := ev.newMatrixIterator(ctx, rangeY)
		chkIter = vsY.Series[yIdx].Iterator(chkIter)
		respIt.Reset(chkIter)

		predIts := make([]*matrixIterator, k)
		for j, pr := range g.predictors {
			it := ev.newMatrixIterator(ctx, rangeX)
			pit := vsX.Series[pr.seriesIdx].Iterator(nil)
			it.Reset(pit)
			predIts[j] = it
		}

		// Output series: one per coefficient (predictors + intercept) plus a
		// trailing r² fit-quality series.
		coeffSeries := make([]*Series, k+2)
		lb := labels.NewBuilder(g.metric)
		for j, pr := range g.predictors {
			lb.Reset(g.metric)
			lb.Set(labelName, pr.value)
			coeffSeries[j] = &Series{Metric: lb.Labels(), DropName: true}
		}
		lb.Reset(g.metric)
		lb.Set(labelName, lmInterceptLabelValue)
		coeffSeries[k] = &Series{Metric: lb.Labels(), DropName: true}
		lb.Reset(g.metric)
		lb.Set(labelName, lmR2LabelValue)
		coeffSeries[k+1] = &Series{Metric: lb.Labels(), DropName: true}

		var respFloats []FPoint
		var respHists []HPoint
		predFloats := make([][]FPoint, k)
		predHists := make([][]HPoint, k)
		rankDeficientSeen := false
		droppedSeen := false
		invalidHalflifeSeen := false

		step := -1
		for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
			step++
			maxtY := ts - offsetY
			mintY := maxtY - rangeY
			respFloats, respHists, _ = ev.matrixIterSlice(ctx, respIt, mintY, maxtY, respFloats, respHists, nil, readStep(e.Args[1], step))

			// rows keyed by timestamp; only timestamps present in y and all
			// predictors form a design-matrix row.
			type rowAcc struct {
				y      float64
				x      []float64
				filled int
			}
			rows := make(map[int64]*rowAcc, len(respFloats))
			for _, fp := range respFloats {
				rows[fp.T] = &rowAcc{y: fp.F, x: make([]float64, k)}
			}
			for j := range g.predictors {
				maxtX := ts - offsetX
				mintX := maxtX - rangeX
				predFloats[j], predHists[j], _ = ev.matrixIterSlice(ctx, predIts[j], mintX, maxtX, predFloats[j], predHists[j], nil, readStep(e.Args[2], step))
				for _, fp := range predFloats[j] {
					if ra, okRow := rows[fp.T]; okRow {
						ra.x[j] = fp.F
						ra.filled++
					}
				}
			}

			lambda := lambdaAt(step)
			useRidge := method == lmMethodRidge
			if useRidge && !(lambda > 0) {
				warnings.Add(annotations.NewInvalidRidgeLambdaWarning(lambda, e.Args[0].PositionRange()))
			}

			// Assemble the design matrix [1, x₁, …, x_k] over complete rows, in
			// ascending timestamp order so first-differencing (below) is well
			// defined.
			rowTimes := make([]int64, 0, len(rows))
			for t, ra := range rows {
				if ra.filled == k {
					rowTimes = append(rowTimes, t)
				}
			}
			slices.Sort(rowTimes)
			design := make([][]float64, 0, len(rowTimes))
			resp := make([]float64, 0, len(rowTimes))
			for _, t := range rowTimes {
				ra := rows[t]
				row := make([]float64, k+1)
				row[0] = 1
				copy(row[1:], ra.x)
				design = append(design, row)
				resp = append(resp, ra.y)
			}
			// usedTimes tracks the timestamp of each design row (for ,wls
			// weighting). First-differencing pairs consecutive rows, so a diff
			// row inherits the later timestamp of its pair.
			usedTimes := rowTimes
			if difference {
				design, resp = firstDifference(design, resp)
				if len(rowTimes) > 0 {
					usedTimes = rowTimes[1:]
				}
			}

			// solveDesign/solveResp feed the solver; for ,wls they are time-decay
			// weighted copies, leaving design/resp (unweighted) for an honest r².
			solveDesign, solveResp := design, resp
			if weighted {
				halflifeSec := halflifeAt(step)
				if halflifeSec > 0 {
					solveDesign, solveResp = timeDecayWeightedCopy(design, resp, usedTimes, halflifeSec*1000)
				} else if !invalidHalflifeSeen {
					invalidHalflifeSeen = true
					warnings.Add(annotations.NewInvalidWLSHalflifeWarning(halflifeSec, e.Args[0].PositionRange()))
				}
			}

			coeffs := make([]float64, k+1)
			var droppedCols []int
			solvable := len(design) >= k+1
			switch {
			case !solvable:
				// Handled below.
			case useRidge && lambda > 0:
				// Ridge is full rank by construction (λI), so the strict solver
				// suffices — no column can be rank deficient.
				if sol, ok := householderLeastSquares(ridgeAugment(solveDesign, solveResp, lambda)); ok {
					copy(coeffs, sol)
				} else {
					solvable = false
				}
			default:
				// OLS (and ridge with an invalid λ): rank-revealing solve that
				// drops only the degenerate columns and solves the rest, so one
				// constant or collinear predictor no longer NaNs the whole model.
				if sol, _, ok := householderLeastSquaresPivoted(solveDesign, solveResp); ok {
					copy(coeffs, sol)
					for j := range sol {
						if math.IsNaN(sol[j]) {
							droppedCols = append(droppedCols, j)
						}
					}
				} else {
					solvable = false
				}
			}
			if !solvable {
				if !rankDeficientSeen {
					rankDeficientSeen = true
					warnings.Add(annotations.NewRankDeficientLMDesignInfo(e.Args[2].PositionRange()))
				}
				for i := range coeffs {
					coeffs[i] = math.NaN()
				}
			} else if len(droppedCols) > 0 && !droppedSeen {
				droppedSeen = true
				names := make([]string, 0, len(droppedCols))
				for _, j := range droppedCols {
					if j == 0 {
						names = append(names, lmInterceptLabelValue)
						continue
					}
					names = append(names, g.predictors[j-1].value)
				}
				warnings.Add(annotations.NewDroppedLMPredictorsInfo(strings.Join(names, ", "), e.Args[2].PositionRange()))
			}

			// r² over the actual (unaugmented) rows; NaN when the fit failed.
			r2 := math.NaN()
			if solvable {
				r2 = lmR2(design, resp, coeffs)
			}

			// Emit: coeffs[0] is the intercept, coeffs[1..k] the predictors in
			// sorted column order, and a trailing r² fit-quality series.
			ev.currentSamples += k + 2
			ev.samplesStats.IncrementSamplesAtStep(step, int64(k+2))
			if ev.currentSamples > ev.maxSamples {
				ev.error(ErrTooManySamples(env))
			}
			for j := range k {
				if coeffSeries[j].Floats == nil {
					coeffSeries[j].Floats = getFPointSlice(numSteps)
				}
				coeffSeries[j].Floats = append(coeffSeries[j].Floats, FPoint{F: coeffs[j+1], T: ts})
			}
			if coeffSeries[k].Floats == nil {
				coeffSeries[k].Floats = getFPointSlice(numSteps)
			}
			coeffSeries[k].Floats = append(coeffSeries[k].Floats, FPoint{F: coeffs[0], T: ts})
			if coeffSeries[k+1].Floats == nil {
				coeffSeries[k+1].Floats = getFPointSlice(numSteps)
			}
			coeffSeries[k+1].Floats = append(coeffSeries[k+1].Floats, FPoint{F: r2, T: ts})

			stepRangeY := min(rangeY, ev.interval)
			respIt.ReduceDelta(stepRangeY)
			for j := range predIts {
				predIts[j].ReduceDelta(min(rangeX, ev.interval))
			}
		}

		ev.samplesStats.UpdatePeak(ev.currentSamples)
		ev.currentSamples -= len(respFloats) + totalHPointSize(respHists)
		putFPointSlice(respFloats)
		putMatrixSelectorHPointSlice(respHists)
		for j := range predFloats {
			ev.currentSamples -= len(predFloats[j]) + totalHPointSize(predHists[j])
			putFPointSlice(predFloats[j])
			putMatrixSelectorHPointSlice(predHists[j])
		}

		for _, s := range coeffSeries {
			if len(s.Floats) > 0 {
				mat = append(mat, *s)
			}
		}
	}

	if !ev.enableDelayedNameRemoval && mat.ContainsSameLabelset() {
		ev.errorf("vector cannot contain metrics with the same labelset")
	}
	return mat, warnings
}

// lmBivariate handles the no-pivot degenerate case: pair the response and
// predictor range vectors by label set (ignoring __name__ by default, or by
// exactly the `on` labels when given — see onMatching) and emit the OLS slope
// per matched pair, matching regression_over_time's default output. Multiple
// X series matching the same Y series is normal fan-out, not ambiguity;
// respBySig already excludes any Y-side signature matched by more than one
// series (see the caller).
func (ev *evaluator) lmBivariate(
	ctx context.Context, e *parser.Call,
	_, _ *parser.MatrixSelector,
	vsX, vsY *parser.VectorSelector,
	respBySig map[uint64]int,
	difference bool,
	on onMatching,
	rangeX, rangeY, offsetX, offsetY int64,
	numSteps int, warnings *annotations.Annotations,
) (parser.Value, annotations.Annotations) {
	var mat Matrix
	var chkIterX, chkIterY chunkenc.Iterator
	var hashBuf []byte

	itX := ev.newMatrixIterator(ctx, rangeX)
	itY := ev.newMatrixIterator(ctx, rangeY)

	for _, sx := range vsX.Series {
		var sig uint64
		sig, hashBuf = on.sig(sx.Labels(), hashBuf, model.MetricNameLabel)
		yIdx, ok := respBySig[sig]
		if !ok {
			continue
		}
		if err := contextDone(ctx, "expression evaluation"); err != nil {
			ev.error(err)
		}
		chkIterX = sx.Iterator(chkIterX)
		itX.Reset(chkIterX)
		chkIterY = vsY.Series[yIdx].Iterator(chkIterY)
		itY.Reset(chkIterY)

		metric := sx.Labels().DropReserved(schema.IsMetadataLabel)
		ss := Series{Metric: metric, DropName: true}

		var floatsX, floatsY []FPoint
		var histsX, histsY []HPoint

		step := -1
		for ts := ev.startTimestamp; ts <= ev.endTimestamp; ts += ev.interval {
			step++
			maxtX := ts - offsetX
			maxtY := ts - offsetY
			floatsX, histsX, _ = ev.matrixIterSlice(ctx, itX, maxtX-rangeX, maxtX, floatsX, histsX, nil, readStep(e.Args[2], step))
			floatsY, histsY, _ = ev.matrixIterSlice(ctx, itY, maxtY-rangeY, maxtY, floatsY, histsY, nil, readStep(e.Args[1], step))
			if len(floatsX) == 0 || len(floatsY) == 0 {
				continue
			}
			// y is the response (first arg), x the predictor (second arg).
			y, x := alignByTimestamp(floatsY, floatsX)
			if difference {
				x, y = firstDifferenceSlice(x), firstDifferenceSlice(y)
			}
			r := math.NaN()
			if fit := olsOnSlices(x, y); fit.ok {
				r = fit.slope
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
		ev.currentSamples -= len(floatsX) + len(floatsY) + totalHPointSize(histsX) + totalHPointSize(histsY)
		putFPointSlice(floatsX)
		putFPointSlice(floatsY)
		putMatrixSelectorHPointSlice(histsX)
		putMatrixSelectorHPointSlice(histsY)
		itX.ReduceDelta(min(rangeX, ev.interval))
		itY.ReduceDelta(min(rangeY, ev.interval))
	}

	if !ev.enableDelayedNameRemoval && mat.ContainsSameLabelset() {
		ev.errorf("vector cannot contain metrics with the same labelset")
	}
	return mat, *warnings
}
