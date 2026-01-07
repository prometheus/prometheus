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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestDurationVisitor(t *testing.T) {
	// Enable experimental duration expression parsing.
	parser.ExperimentalDurationExpr = true
	t.Cleanup(func() {
		parser.ExperimentalDurationExpr = false
	})
	complexExpr := `sum_over_time(
		rate(metric[5m] offset 1h)[10m:30s] offset 2h
	) + 
	avg_over_time(
		metric[1h + 30m] offset -1h
	) * 
	count_over_time(
		metric[2h * 0.5]
	)`

	expr, err := parser.ParseExpr(complexExpr)
	require.NoError(t, err)

	err = parser.Walk(&durationVisitor{}, expr, nil)
	require.NoError(t, err)

	// Verify different parts of the expression have correct durations.
	// This is a binary expression at the top level.
	binExpr, ok := expr.(*parser.BinaryExpr)
	require.True(t, ok, "Expected binary expression at top level")

	// Left side should be sum_over_time with subquery.
	leftCall, ok := binExpr.LHS.(*parser.Call)
	require.True(t, ok, "Expected call expression on left side")
	require.Equal(t, "sum_over_time", leftCall.Func.Name)

	// Extract the subquery from sum_over_time.
	sumSubquery, ok := leftCall.Args[0].(*parser.SubqueryExpr)
	require.True(t, ok, "Expected subquery in sum_over_time")
	require.Equal(t, 10*time.Minute, sumSubquery.Range)
	require.Equal(t, 30*time.Second, sumSubquery.Step)
	require.Equal(t, 2*time.Hour, sumSubquery.OriginalOffset)

	// Extract the rate call inside the subquery.
	rateCall, ok := sumSubquery.Expr.(*parser.Call)
	require.True(t, ok, "Expected rate call in subquery")
	require.Equal(t, "rate", rateCall.Func.Name)

	// Extract the matrix selector from rate.
	rateMatrix, ok := rateCall.Args[0].(*parser.MatrixSelector)
	require.True(t, ok, "Expected matrix selector in rate")
	require.Equal(t, 5*time.Minute, rateMatrix.Range)
	require.Equal(t, 1*time.Hour, rateMatrix.VectorSelector.(*parser.VectorSelector).OriginalOffset)

	// Right side should be another binary expression (multiplication).
	rightBinExpr, ok := binExpr.RHS.(*parser.BinaryExpr)
	require.True(t, ok, "Expected binary expression on right side")

	// Left side of multiplication should be avg_over_time.
	avgCall, ok := rightBinExpr.LHS.(*parser.Call)
	require.True(t, ok, "Expected call expression on left side of multiplication")
	require.Equal(t, "avg_over_time", avgCall.Func.Name)

	// Extract the matrix selector from avg_over_time.
	avgMatrix, ok := avgCall.Args[0].(*parser.MatrixSelector)
	require.True(t, ok, "Expected matrix selector in avg_over_time")
	require.Equal(t, 90*time.Minute, avgMatrix.Range) // 1h + 30m
	require.Equal(t, -1*time.Hour, avgMatrix.VectorSelector.(*parser.VectorSelector).OriginalOffset)

	// Right side of multiplication should be count_over_time.
	countCall, ok := rightBinExpr.RHS.(*parser.Call)
	require.True(t, ok, "Expected call expression on right side of multiplication")
	require.Equal(t, "count_over_time", countCall.Func.Name)

	// Extract the matrix selector from count_over_time.
	countMatrix, ok := countCall.Args[0].(*parser.MatrixSelector)
	require.True(t, ok, "Expected matrix selector in count_over_time")
	require.Equal(t, 1*time.Hour, countMatrix.Range) // 2h * 0.5
}

func TestCalculateDuration(t *testing.T) {
	tests := []struct {
		name            string
		expr            parser.Expr
		expected        time.Duration
		errorMessage    string
		allowedNegative bool
	}{
		{
			name: "addition",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 5},
				RHS: &parser.NumberLiteral{Val: 10},
				Op:  parser.ADD,
			},
			expected: 15 * time.Second,
		},
		{
			name: "subtraction",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 15},
				RHS: &parser.NumberLiteral{Val: 5},
				Op:  parser.SUB,
			},
			expected: 10 * time.Second,
		},
		{
			name: "subtraction with negative",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 5},
				RHS: &parser.NumberLiteral{Val: 10},
				Op:  parser.SUB,
			},
			errorMessage: "duration must be greater than 0",
		},
		{
			name: "multiplication",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 5},
				RHS: &parser.NumberLiteral{Val: 3},
				Op:  parser.MUL,
			},
			expected: 15 * time.Second,
		},
		{
			name: "division",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 15},
				RHS: &parser.NumberLiteral{Val: 3},
				Op:  parser.DIV,
			},
			expected: 5 * time.Second,
		},
		{
			name: "modulo with numbers",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 17},
				RHS: &parser.NumberLiteral{Val: 5},
				Op:  parser.MOD,
			},
			expected: 2 * time.Second,
		},
		{
			name: "power",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 2},
				RHS: &parser.NumberLiteral{Val: 3},
				Op:  parser.POW,
			},
			expected: 8 * time.Second,
		},
		{
			name: "complex expression",
			expr: &parser.DurationExpr{
				LHS: &parser.DurationExpr{
					LHS: &parser.NumberLiteral{Val: 2},
					RHS: &parser.DurationExpr{
						LHS: &parser.NumberLiteral{Val: 3},
						RHS: &parser.NumberLiteral{Val: 4},
						Op:  parser.ADD,
					},
					Op: parser.MUL,
				},
				RHS: &parser.NumberLiteral{Val: 1},
				Op:  parser.SUB,
			},
			expected: 13 * time.Second,
		},
		{
			name: "unary negative",
			expr: &parser.DurationExpr{
				RHS: &parser.NumberLiteral{Val: 5},
				Op:  parser.SUB,
			},
			expected:        -5 * time.Second,
			allowedNegative: true,
		},
		{
			name: "step",
			expr: &parser.DurationExpr{
				Op: parser.STEP,
			},
			expected: 1 * time.Second,
		},
		{
			name: "step multiplication",
			expr: &parser.DurationExpr{
				LHS: &parser.DurationExpr{
					Op: parser.STEP,
				},
				RHS: &parser.NumberLiteral{Val: 3},
				Op:  parser.MUL,
			},
			expected: 3 * time.Second,
		},
		{
			name: "range",
			expr: &parser.DurationExpr{
				Op: parser.RANGE,
			},
			expected: 5 * time.Minute,
		},
		{
			name: "range division",
			expr: &parser.DurationExpr{
				LHS: &parser.DurationExpr{
					Op: parser.RANGE,
				},
				RHS: &parser.NumberLiteral{Val: 2},
				Op:  parser.DIV,
			},
			expected: 150 * time.Second,
		},
		{
			name: "max of step and range",
			expr: &parser.DurationExpr{
				LHS: &parser.DurationExpr{
					Op: parser.STEP,
				},
				RHS: &parser.DurationExpr{
					Op: parser.RANGE,
				},
				Op: parser.MAX,
			},
			expected: 5 * time.Minute,
		},
		{
			name: "division by zero",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 5},
				RHS: &parser.DurationExpr{
					LHS: &parser.NumberLiteral{Val: 5},
					RHS: &parser.NumberLiteral{Val: 5},
					Op:  parser.SUB,
				},
				Op: parser.DIV,
			},
			errorMessage: "division by zero",
		},
		{
			name: "modulo by zero",
			expr: &parser.DurationExpr{
				LHS: &parser.NumberLiteral{Val: 5},
				RHS: &parser.DurationExpr{
					LHS: &parser.NumberLiteral{Val: 5},
					RHS: &parser.NumberLiteral{Val: 5},
					Op:  parser.SUB,
				},
				Op: parser.MOD,
			},
			errorMessage: "modulo by zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &durationVisitor{step: 1 * time.Second, queryRange: 5 * time.Minute}
			result, err := v.calculateDuration(tt.expr, tt.allowedNegative)
			if tt.errorMessage != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMessage)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}
