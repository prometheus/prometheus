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
	"math"

	"github.com/prometheus/prometheus/promql/parser"
)

func ConstantFoldExpr(expr parser.Expr) (parser.Expr, error) {
	var err error
	switch n := expr.(type) {
	case *parser.BinaryExpr:
		lhs, err := ConstantFoldExpr(n.LHS)
		if err != nil {
			return expr, err
		}
		rhs, err := ConstantFoldExpr(n.RHS)
		if err != nil {
			return expr, err
		}
		n.LHS, n.RHS = lhs, rhs

		unwrapParenExpr(&lhs)
		unwrapParenExpr(&rhs)

		if lhs.Type() == parser.ValueTypeScalar && rhs.Type() == parser.ValueTypeScalar {
			lhsVal, okLHS := lhs.(*parser.NumberLiteral)
			rhsVal, okRHS := rhs.(*parser.NumberLiteral)
			if okLHS && okRHS {
				val := scalarBinop(n.Op, lhsVal.Val, rhsVal.Val)
				return &parser.NumberLiteral{
					Val:      val,
					PosRange: n.PositionRange(),
				}, nil
			}
		}
		return n, nil

	case *parser.Call:
		if n.Func.Name == "pi" {
			return &parser.NumberLiteral{
				Val:      math.Pi,
				PosRange: n.PositionRange(),
			}, nil
		}
		for i := range n.Args {
			n.Args[i], err = ConstantFoldExpr(n.Args[i])
			if err != nil {
				return expr, err
			}
		}
		return n, nil

	case *parser.MatrixSelector:
		n.VectorSelector, err = ConstantFoldExpr(n.VectorSelector)
		if err != nil {
			return expr, err
		}
		return n, nil

	case *parser.AggregateExpr:
		n.Expr, err = ConstantFoldExpr(n.Expr)
		if err != nil {
			return expr, err
		}
		return n, nil

	case *parser.SubqueryExpr:
		n.Expr, err = ConstantFoldExpr(n.Expr)
		if err != nil {
			return expr, err
		}
		return n, nil

	case *parser.ParenExpr:
		n.Expr, err = ConstantFoldExpr(n.Expr)
		if err != nil {
			return expr, err
		}
		return n, nil

	case *parser.UnaryExpr:
		n.Expr, err = ConstantFoldExpr(n.Expr)
		if err != nil {
			return expr, err
		}

		unwrapParenExpr(&n.Expr)
		if n.Expr.Type() == parser.ValueTypeScalar {
			if val, ok := n.Expr.(*parser.NumberLiteral); ok {
				if n.Op == parser.SUB {
					val.Val = -val.Val
				}
				return &parser.NumberLiteral{
					Val:      val.Val,
					PosRange: n.PositionRange(),
				}, nil
			}
		}
		return n, nil

	default:
		return n, nil
	}
}
