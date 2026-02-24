package promql

import (
	"math"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/stretchr/testify/require"
)

var testExpr = []struct {
	input    string
	expected parser.Expr
}{
	{
		input: "1",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 1},
		},
	},
	{
		input: "+Inf",
		expected: &parser.NumberLiteral{
			Val:      math.Inf(1),
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "123.4567",
		expected: &parser.NumberLiteral{
			Val:      123.4567,
			PosRange: posrange.PositionRange{Start: 0, End: 8},
		},
	},
	{
		input: "5e-3",
		expected: &parser.NumberLiteral{
			Val:      0.005,
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "1 + 1",
		expected: &parser.NumberLiteral{
			Val:      2,
			PosRange: posrange.PositionRange{Start: 0, End: 5},
		},
	},
	{
		input: "pi()",
		expected: &parser.NumberLiteral{
			Val:      math.Pi,
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "1 - 1",
		expected: &parser.NumberLiteral{
			Val:      0,
			PosRange: posrange.PositionRange{Start: 0, End: 5},
		},
	},
	{
		input: "1 * 1",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 5},
		},
	},
	{
		input: "1 % 1",
		expected: &parser.NumberLiteral{
			Val:      0,
			PosRange: posrange.PositionRange{Start: 0, End: 5},
		},
	},
	{
		input: "1 / 1",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 5},
		},
	},
	{
		input: "1 == bool 1",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: "1 != bool 1",
		expected: &parser.NumberLiteral{
			Val:      0,
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: "1 > bool 1",
		expected: &parser.NumberLiteral{
			Val:      0,
			PosRange: posrange.PositionRange{Start: 0, End: 10},
		},
	},
	{
		input: "1 >= bool 1",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: "1 < bool 1",
		expected: &parser.NumberLiteral{
			Val:      0,
			PosRange: posrange.PositionRange{Start: 0, End: 10},
		},
	},
	{
		input: "1 <= bool 1",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: "(-1)^2",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 6},
		},
	},
	{
		input: "-1*2",
		expected: &parser.NumberLiteral{
			Val:      -2,
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "-1+2",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 4},
		},
	},
	{
		input: "(-1)^-2",
		expected: &parser.NumberLiteral{
			Val:      1,
			PosRange: posrange.PositionRange{Start: 0, End: 7},
		},
	},
	{
		input: "+1 + -2 * 1",
		expected: &parser.NumberLiteral{
			Val:      -1,
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: "1 + 2/(3*1)",
		expected: &parser.NumberLiteral{
			Val:      1.6666666666666665,
			PosRange: posrange.PositionRange{Start: 0, End: 11},
		},
	},
	{
		input: "1 < bool 2 - 1 * 2",
		expected: &parser.NumberLiteral{
			Val:      0,
			PosRange: posrange.PositionRange{Start: 0, End: 18},
		},
	},
	{
		input: "((1+2)*(3-1))/(2+2)",
		expected: &parser.NumberLiteral{
			Val:      1.5,
			PosRange: posrange.PositionRange{Start: 0, End: 19},
		},
	},
	{
		input: "(((-1)^2) + (2 % 3) * (4 - 1))",
		expected: &parser.ParenExpr{
			Expr: &parser.NumberLiteral{
				Val:      7,
				PosRange: posrange.PositionRange{Start: 1, End: 29},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 30},
		},
	},
	{
		input: "-(1 + 2) * +((3 - 5) ^ 2)",
		expected: &parser.NumberLiteral{
			Val:      -12,
			PosRange: posrange.PositionRange{Start: 0, End: 24},
		},
	},
	{
		input: "((1+1) == bool (2))",
		expected: &parser.ParenExpr{
			Expr: &parser.NumberLiteral{
				Val:      1,
				PosRange: posrange.PositionRange{Start: 1, End: 18},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 19},
		},
	},
	{
		input: "((1+2) <= bool (2-1))",
		expected: &parser.ParenExpr{
			Expr: &parser.NumberLiteral{
				Val:      0,
				PosRange: posrange.PositionRange{Start: 1, End: 20},
			},
			PosRange: posrange.PositionRange{Start: 0, End: 21},
		},
	},
	{
		input: "1 + 2/(3*1) + (4-2)*(7%5)",
		expected: &parser.NumberLiteral{
			Val:      5.666666666666666,
			PosRange: posrange.PositionRange{Start: 0, End: 25},
		},
	},
	{
		input: "pi() * (1 + 1) - (3 - 3)",
		expected: &parser.NumberLiteral{
			Val:      2 * math.Pi,
			PosRange: posrange.PositionRange{Start: 0, End: 24},
		},
	},
	{
		input: "((1.5 + 2.25) * 2) / (7 - 3)",
		expected: &parser.NumberLiteral{
			Val:      ((1.5 + 2.25) * 2) / 4,
			PosRange: posrange.PositionRange{Start: 0, End: 28},
		},
	},
	{
		input: "((1 < bool 2) + (3 == bool 3)) * (4 % 3)",
		expected: &parser.NumberLiteral{
			Val:      2,
			PosRange: posrange.PositionRange{Start: 0, End: 40},
		},
	},
}

func TestConstantFolding(t *testing.T) {
	for _, test := range testExpr {
		expr, err := parser.ParseExpr(test.input)
		require.NoError(t, err, "cannot parse original query")
		expr, err = ConstantFoldExpr(expr)
		require.NoError(t, err, "cannot fold query")
		require.Equal(t, test.expected, expr, "error on input '%s'", test.input)
	}
}
