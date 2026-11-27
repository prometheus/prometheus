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
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// designFor builds the [1, x₁, x₂, …] design matrix from column-major predictors.
func designFor(cols ...[]float64) [][]float64 {
	n := len(cols[0])
	a := make([][]float64, n)
	for i := range n {
		row := make([]float64, len(cols)+1)
		row[0] = 1
		for j, c := range cols {
			row[j+1] = c[i]
		}
		a[i] = row
	}
	return a
}

func TestHouderholderLeastSquares_ExactFit(t *testing.T) {
	// y = 1 + 2·x1 + 3·x2, exactly recoverable.
	x1 := []float64{1, 2, 3, 4, 5}
	x2 := []float64{1, 0, 2, 1, 3}
	y := make([]float64, len(x1))
	for i := range y {
		y[i] = 1 + 2*x1[i] + 3*x2[i]
	}
	sol, ok := householderLeastSquares(designFor(x1, x2), y)
	require.True(t, ok)
	require.InDelta(t, 1.0, sol[0], 1e-9, "intercept")
	require.InDelta(t, 2.0, sol[1], 1e-9, "x1 coefficient")
	require.InDelta(t, 3.0, sol[2], 1e-9, "x2 coefficient")
}

func TestHouseholderLeastSquares_Overdetermined(t *testing.T) {
	// Simple bivariate OLS: y = 2x, slope 2, intercept 0.
	x := []float64{0, 1, 2, 3, 4}
	y := []float64{0, 2, 4, 6, 8}
	sol, ok := householderLeastSquares(designFor(x), y)
	require.True(t, ok)
	require.InDelta(t, 0.0, sol[0], 1e-9)
	require.InDelta(t, 2.0, sol[1], 1e-9)
}

func TestHouseholderLeastSquares_RankDeficient(t *testing.T) {
	// x2 = 2·x1: collinear predictors make the design rank deficient.
	x1 := []float64{1, 2, 3, 4, 5}
	x2 := []float64{2, 4, 6, 8, 10}
	y := []float64{1, 2, 3, 4, 5}
	_, ok := householderLeastSquares(designFor(x1, x2), y)
	require.False(t, ok)
}

func TestHouseholderLeastSquares_Underdetermined(t *testing.T) {
	// Fewer rows than columns (2 rows, 3 columns).
	x1 := []float64{1, 2}
	x2 := []float64{3, 4}
	y := []float64{1, 1}
	_, ok := householderLeastSquares(designFor(x1, x2), y)
	require.False(t, ok)
}

func TestHouseholderLeastSquaresPivoted_FullRank(t *testing.T) {
	// Full-rank design: same solution as the strict solver, no drops.
	x1 := []float64{1, 2, 3, 4, 5}
	x2 := []float64{1, 0, 2, 1, 3}
	y := make([]float64, len(x1))
	for i := range y {
		y[i] = 1 + 2*x1[i] + 3*x2[i]
	}
	coeffs, rank, ok := householderLeastSquaresPivoted(designFor(x1, x2), y)
	require.True(t, ok)
	require.Equal(t, 3, rank)
	require.InDelta(t, 1.0, coeffs[0], 1e-9)
	require.InDelta(t, 2.0, coeffs[1], 1e-9)
	require.InDelta(t, 3.0, coeffs[2], 1e-9)
}

func TestHouseholderLeastSquaresPivoted_DropsCollinear(t *testing.T) {
	// x2 = 2·x1: rank deficient. Instead of failing, the solver drops one of the
	// collinear pair (NaN coefficient) and solves the rest, still reproducing y.
	x1 := []float64{1, 2, 3, 4, 5}
	x2 := []float64{2, 4, 6, 8, 10}
	y := []float64{1, 2, 3, 4, 5} // y = x1 = 0.5·x2
	design := designFor(x1, x2)
	coeffs, rank, ok := householderLeastSquaresPivoted(design, y)
	require.True(t, ok)
	require.Equal(t, 2, rank, "intercept + one predictor")
	// Exactly one predictor coefficient is dropped (NaN); the intercept stays.
	require.False(t, math.IsNaN(coeffs[0]), "intercept retained")
	require.NotEqual(t, math.IsNaN(coeffs[1]), math.IsNaN(coeffs[2]), "exactly one predictor dropped")
	// The surviving fit reproduces y (dropped columns contribute 0).
	for i := range y {
		var pred float64
		for j := range coeffs {
			if math.IsNaN(coeffs[j]) {
				continue
			}
			pred += coeffs[j] * design[i][j]
		}
		require.InDelta(t, y[i], pred, 1e-9)
	}
}

func TestHouseholderLeastSquaresPivoted_DropsConstant(t *testing.T) {
	// A constant predictor is collinear with the intercept; the solver drops one
	// of them rather than NaN-ing the whole fit, and still recovers the x1 slope.
	x1 := []float64{1, 2, 3, 4, 5}
	xc := []float64{7, 7, 7, 7, 7}
	y := []float64{3, 5, 7, 9, 11} // y = 1 + 2·x1
	design := designFor(x1, xc)
	coeffs, rank, ok := householderLeastSquaresPivoted(design, y)
	require.True(t, ok)
	require.Equal(t, 2, rank)
	for i := range y {
		var pred float64
		for j := range coeffs {
			if math.IsNaN(coeffs[j]) {
				continue
			}
			pred += coeffs[j] * design[i][j]
		}
		require.InDelta(t, y[i], pred, 1e-9, "fit reproduces y despite the constant column")
	}
}

func TestTimeDecayWeightedCopy_Scaling(t *testing.T) {
	// Two rows one half-life apart: the newest keeps weight 1 (scale 1), the
	// oldest gets weight 0.5 (scale sqrt(0.5)).
	design := [][]float64{{1, 10}, {1, 20}}
	resp := []float64{3, 7}
	hlMs := 1000.0
	times := []int64{0, 1000} // oldest at 0 (age = hlMs), newest at 1000 (age 0).
	wd, wr := timeDecayWeightedCopy(design, resp, times, hlMs)
	sOld := math.Sqrt(0.5)
	require.InDelta(t, 1*sOld, wd[0][0], 1e-12)
	require.InDelta(t, 10*sOld, wd[0][1], 1e-12)
	require.InDelta(t, 3*sOld, wr[0], 1e-12)
	require.InDelta(t, 1.0, wd[1][0], 1e-12, "newest row unscaled")
	require.InDelta(t, 20.0, wd[1][1], 1e-12)
	require.InDelta(t, 7.0, wr[1], 1e-12)
	// Originals untouched.
	require.Equal(t, 1.0, design[0][0])
	require.Equal(t, 3.0, resp[0])
}

func TestTimeDecayWeighting_TiltsTowardRecent(t *testing.T) {
	// y is flat early then rises steeply at the recent end. Recency weighting
	// should pull the fitted slope up, toward the recent (steeper) trend.
	x := []float64{0, 1, 2, 3, 4}
	y := []float64{0, 0, 0, 5, 10}
	times := []int64{0, 1000, 2000, 3000, 4000}
	design := designFor(x)

	ols, ok := householderLeastSquares(design, y)
	require.True(t, ok)

	wd, wr := timeDecayWeightedCopy(design, y, times, 1000) // 1s half-life.
	wls, ok := householderLeastSquares(wd, wr)
	require.True(t, ok)

	require.Greater(t, wls[1], ols[1], "recency weighting steepens the slope toward the recent trend")
}

func TestParseLMMethod(t *testing.T) {
	cases := []struct {
		in         string
		base       string
		difference bool
		weighted   bool
		ok         bool
	}{
		{"lm", "lm", false, false, true},
		{"ridge", "ridge", false, false, true},
		{"lm,diff", "lm", true, false, true},
		{"ridge,diff", "ridge", true, false, true},
		{"lm,wls", "lm", false, true, true},
		{"ridge,wls", "ridge", false, true, true},
		{"ridge,diff,wls", "ridge", true, true, true},
		{"lm,bogus", "lm", false, false, false},
		{"nope", "nope", false, false, false},
		{"nope,diff", "nope", false, false, false},
	}
	for _, c := range cases {
		base, difference, weighted, ok := parseLMMethod(c.in)
		require.Equal(t, c.ok, ok, "ok for %q", c.in)
		require.Equal(t, c.base, base, "base for %q", c.in)
		if c.ok {
			require.Equal(t, c.difference, difference, "difference for %q", c.in)
			require.Equal(t, c.weighted, weighted, "weighted for %q", c.in)
		}
	}
}

func TestFirstDifference_RemovesTrend(t *testing.T) {
	// y = 100 + 5·t (pure trend) on x = t. On levels the slope is 5; on first
	// differences both Δy and Δx are constant, so the differenced predictor has
	// zero variance and the fit becomes rank deficient (no spurious slope).
	x := []float64{1, 2, 3, 4, 5}
	design := designFor(x)
	resp := []float64{105, 110, 115, 120, 125}

	dDesign, dResp := firstDifference(design, resp)
	require.Len(t, dDesign, len(design)-1)
	require.Len(t, dResp, len(resp)-1)
	for _, row := range dDesign {
		require.Equal(t, 1.0, row[0], "intercept column stays 1")
		require.InDelta(t, 1.0, row[1], 1e-9, "Δx is constant 1")
	}
	for _, d := range dResp {
		require.InDelta(t, 5.0, d, 1e-9, "Δy is constant 5")
	}
	_, ok := householderLeastSquares(dDesign, dResp)
	require.False(t, ok, "constant Δx column is rank deficient")
}

func TestFirstDifferenceSlice(t *testing.T) {
	require.Equal(t, []float64{1, 1, 2}, firstDifferenceSlice([]float64{1, 2, 3, 5}))
	require.Nil(t, firstDifferenceSlice([]float64{7}))
	require.Nil(t, firstDifferenceSlice(nil))
}

func TestRidgeAugment_ShrinksSlope(t *testing.T) {
	// With a large penalty the slope shrinks toward 0 and the (unpenalized)
	// intercept tends to the response mean.
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{6, 5, 13, 12, 20} // mean 11.2
	design := designFor(x)

	ols, ok := householderLeastSquares(design, y)
	require.True(t, ok)

	a, b := ridgeAugment(design, y, 1e6)
	ridge, ok := householderLeastSquares(a, b)
	require.True(t, ok)

	require.Less(t, math.Abs(ridge[1]), math.Abs(ols[1]), "ridge slope should be smaller in magnitude")
	require.InDelta(t, 0.0, ridge[1], 1e-3, "slope shrinks toward 0")
	require.InDelta(t, 11.2, ridge[0], 1e-3, "intercept tends to the mean")
}
