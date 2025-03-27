// Copyright 2025 The Prometheus Authors
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
	// Enable experimental duration expression parsing.
	parser.ExperimentalDurationExpr = true
	t.Cleanup(func() {
		parser.ExperimentalDurationExpr = false
	})
	tests := []struct {
		name        string
		expr        string
		expected    time.Duration
		expectError bool
	}{
		{
			name:     "number literal",
			expr:     "5",
			expected: 5 * time.Second,
		},
		{
			name:     "time unit literal",
			expr:     "1h",
			expected: time.Hour,
		},
		{
			name:     "addition with numbers",
			expr:     "5 + 10",
			expected: 15 * time.Second,
		},
		{
			name:     "addition with time units",
			expr:     "1h + 30m",
			expected: 90 * time.Minute,
		},
		{
			name:     "subtraction with numbers",
			expr:     "15 - 5",
			expected: 10 * time.Second,
		},
		{
			name:     "subtraction with time units",
			expr:     "2h - 30m",
			expected: 90 * time.Minute,
		},
		{
			name:     "multiplication with numbers",
			expr:     "5 * 3",
			expected: 15 * time.Second,
		},
		{
			name:     "multiplication with time unit and number",
			expr:     "2h * 1.5",
			expected: 3 * time.Hour,
		},
		{
			name:     "division with numbers",
			expr:     "15 / 3",
			expected: 5 * time.Second,
		},
		{
			name:     "division with time unit and number",
			expr:     "1h / 2",
			expected: 30 * time.Minute,
		},
		{
			name:     "modulo with numbers",
			expr:     "17 % 5",
			expected: 2 * time.Second,
		},
		{
			name:     "modulo with time unit and number",
			expr:     "70m % 60m",
			expected: 10 * time.Minute,
		},
		{
			name:     "power with numbers",
			expr:     "2 ^ 3",
			expected: 8 * time.Second,
		},
		{
			name:     "complex expression with numbers",
			expr:     "2 * (3 + 4) - 1",
			expected: 13 * time.Second,
		},
		{
			name:     "complex expression with time units",
			expr:     "2 * (1h + 30m) - 15m",
			expected: 165 * time.Minute,
		},
		{
			name:     "unary negative with number",
			expr:     "-5",
			expected: -5 * time.Second,
		},
		{
			name:     "unary negative with time unit",
			expr:     "-1h",
			expected: -time.Hour,
		},
		{
			name:        "division by zero with numbers",
			expr:        "5 / 0",
			expectError: true,
		},
		{
			name:        "division by zero with time unit",
			expr:        "1h / 0",
			expectError: true,
		},
		{
			name:        "modulo by zero with numbers",
			expr:        "5 % 0",
			expectError: true,
		},
		{
			name:        "modulo by zero with time unit",
			expr:        "1h % 0",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" offset", func(t *testing.T) {
			expr, err := parser.ParseExpr("foo offset (" + tt.expr + ")")
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Extract the duration expression from the vector selector
			vectorSelector, ok := expr.(*parser.VectorSelector)
			require.True(t, ok, "Expected vector selector, got %T", expr)

			result := vectorSelector.OriginalOffset
			if vectorSelector.OriginalOffsetExpr != nil {
				result, err = calculateDuration(vectorSelector.OriginalOffsetExpr, false)
				require.NoError(t, err)
			}
			require.Equal(t, tt.expected, result)
		})

		t.Run(tt.name+" subquery with fixed step", func(t *testing.T) {
			expr, err := parser.ParseExpr("foo[5m:(" + tt.expr + ")]")
			if tt.expectError || tt.expected < 0 {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Extract the duration expression from the subquery
			subquery, ok := expr.(*parser.SubqueryExpr)
			require.True(t, ok, "Expected subquery, got %T", expr)

			require.Equal(t, 5*time.Minute, subquery.Range)

			result := subquery.Step
			if subquery.StepExpr != nil {
				result, err = calculateDuration(subquery.StepExpr, false)
				require.NoError(t, err)
			}
			require.Equal(t, tt.expected, result)
		})

		t.Run(tt.name+" subquery with fixed range", func(t *testing.T) {
			expr, err := parser.ParseExpr("foo[(" + tt.expr + "):5m]")
			if tt.expectError || tt.expected < 0 {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Extract the duration expression from the subquery
			subquery, ok := expr.(*parser.SubqueryExpr)
			require.True(t, ok, "Expected subquery, got %T", expr)

			require.Equal(t, 5*time.Minute, subquery.Step)

			result := subquery.Range
			if subquery.RangeExpr != nil {
				result, err = calculateDuration(subquery.RangeExpr, false)
				require.NoError(t, err)
			}
			require.Equal(t, tt.expected, result)
		})

		t.Run(tt.name+" matrix selector", func(t *testing.T) {
			expr, err := parser.ParseExpr("foo[(" + tt.expr + ")]")
			if tt.expectError || tt.expected < 0 {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Extract the duration expression from the matrix selector
			matrixSelector, ok := expr.(*parser.MatrixSelector)
			require.True(t, ok, "Expected matrix selector, got %T", expr)

			result := matrixSelector.Range
			if matrixSelector.RangeExpr != nil {
				result, err = calculateDuration(matrixSelector.RangeExpr, false)
				require.NoError(t, err)
			}
			require.Equal(t, tt.expected, result)
		})
	}
}
