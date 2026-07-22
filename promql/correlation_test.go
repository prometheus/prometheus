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

func TestPearsonOnSlices(t *testing.T) {
	cases := []struct {
		name string
		x, y []float64
		want float64 // NaN matched separately.
	}{
		{"perfect_positive", []float64{1, 2, 3, 4}, []float64{2, 4, 6, 8}, 1},
		{"perfect_negative", []float64{1, 2, 3, 4}, []float64{8, 6, 4, 2}, -1},
		{"too_few_pairs", []float64{1}, []float64{2}, math.NaN()},
		{"zero_variance_x", []float64{5, 5, 5}, []float64{1, 2, 3}, math.NaN()},
		{"zero_variance_y", []float64{1, 2, 3}, []float64{7, 7, 7}, math.NaN()},
		// Near-constant inputs must NOT produce NaN with the two-pass formula.
		// The naive nΣx² - (Σx)² formula loses all precision here.
		{"near_constant_stable", []float64{1e9, 1e9 + 1, 1e9 + 2, 1e9 + 3}, []float64{1, 2, 3, 4}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pearsonOnSlices(tc.x, tc.y)
			if math.IsNaN(tc.want) {
				require.True(t, math.IsNaN(got), "want NaN, got %v", got)
				return
			}
			require.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestFractionalRanks(t *testing.T) {
	// No ties: ranks are 1..n in order of value.
	require.Equal(t, []float64{1, 2, 3, 4}, fractionalRanks([]float64{10, 20, 30, 40}))
	// Reverse order: ranks reversed.
	require.Equal(t, []float64{4, 3, 2, 1}, fractionalRanks([]float64{40, 30, 20, 10}))
	// Ties: pair of tied values share average rank.
	// Values [10, 20, 20, 30] → sorted ranks 1, (2+3)/2, (2+3)/2, 4.
	require.Equal(t, []float64{1, 2.5, 2.5, 4}, fractionalRanks([]float64{10, 20, 20, 30}))
	// All tied: all get average rank (n+1)/2.
	require.Equal(t, []float64{2.5, 2.5, 2.5, 2.5}, fractionalRanks([]float64{7, 7, 7, 7}))
}

func TestAlignByTimestamp(t *testing.T) {
	x := []FPoint{{T: 1, F: 1}, {T: 2, F: 2}, {T: 4, F: 4}}
	y := []FPoint{{T: 2, F: 20}, {T: 3, F: 30}, {T: 4, F: 40}}
	gotX, gotY := alignByTimestamp(x, y)
	require.Equal(t, []float64{2, 4}, gotX)
	require.Equal(t, []float64{20, 40}, gotY)

	// Disjoint timestamps → empty result.
	x = []FPoint{{T: 1, F: 1}}
	y = []FPoint{{T: 2, F: 2}}
	gotX, gotY = alignByTimestamp(x, y)
	require.Empty(t, gotX)
	require.Empty(t, gotY)
}

func TestKendallCorrelation(t *testing.T) {
	// Perfectly concordant pairs → tau = 1.
	x := []FPoint{{T: 1, F: 1}, {T: 2, F: 2}, {T: 3, F: 3}, {T: 4, F: 4}}
	y := []FPoint{{T: 1, F: 10}, {T: 2, F: 20}, {T: 3, F: 30}, {T: 4, F: 40}}
	require.InDelta(t, 1.0, kendallCorrelation(x, y), 1e-9)

	// Perfectly discordant pairs → tau = -1.
	y = []FPoint{{T: 1, F: 40}, {T: 2, F: 30}, {T: 3, F: 20}, {T: 4, F: 10}}
	require.InDelta(t, -1.0, kendallCorrelation(x, y), 1e-9)

	// All tied on y → den = 0 → NaN.
	y = []FPoint{{T: 1, F: 5}, {T: 2, F: 5}, {T: 3, F: 5}, {T: 4, F: 5}}
	require.True(t, math.IsNaN(kendallCorrelation(x, y)))

	// Fewer than 2 pairs → NaN.
	require.True(t, math.IsNaN(kendallCorrelation(x[:1], y[:1])))
}

func TestSpearmanCorrelation(t *testing.T) {
	// Spearman is invariant under monotone transformation:
	// y = exp(x) is monotone increasing, so rho == 1 even though Pearson < 1.
	x := []FPoint{{T: 1, F: 1}, {T: 2, F: 2}, {T: 3, F: 3}, {T: 4, F: 4}, {T: 5, F: 5}}
	y := []FPoint{
		{T: 1, F: math.Exp(1)},
		{T: 2, F: math.Exp(2)},
		{T: 3, F: math.Exp(3)},
		{T: 4, F: math.Exp(4)},
		{T: 5, F: math.Exp(5)},
	}
	require.InDelta(t, 1.0, spearmanCorrelation(x, y), 1e-9)
}
