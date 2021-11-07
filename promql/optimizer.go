// Copyright 2021 The Prometheus Authors
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
	"fmt"
	"time"

	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
)

// preprocessExprHelper wraps the child nodes of the expression
// with a StepInvariantExpr wherever it's step invariant. The returned boolean is true if the
// passed expression qualifies to be wrapped by StepInvariantExpr.
// It also resolves the preprocessors.
func preprocessExprHelper(expr parser.Expr, start, end time.Time) bool {
	switch n := expr.(type) {
	case *parser.VectorSelector:
		if n.StartOrEnd == parser.START {
			n.Timestamp = makeInt64Pointer(timestamp.FromTime(start))
		} else if n.StartOrEnd == parser.END {
			n.Timestamp = makeInt64Pointer(timestamp.FromTime(end))
		}
		return n.Timestamp != nil

	case *parser.AggregateExpr:
		return preprocessExprHelper(n.Expr, start, end)

	case *parser.BinaryExpr:
		isInvariant1, isInvariant2 := preprocessExprHelper(n.LHS, start, end), preprocessExprHelper(n.RHS, start, end)
		if isInvariant1 && isInvariant2 {
			return true
		}

		if isInvariant1 {
			n.LHS = newStepInvariantExpr(n.LHS)
		}
		if isInvariant2 {
			n.RHS = newStepInvariantExpr(n.RHS)
		}

		return false

	case *parser.Call:
		_, ok := AtModifierUnsafeFunctions[n.Func.Name]
		isStepInvariant := !ok
		isStepInvariantSlice := make([]bool, len(n.Args))
		for i := range n.Args {
			isStepInvariantSlice[i] = preprocessExprHelper(n.Args[i], start, end)
			isStepInvariant = isStepInvariant && isStepInvariantSlice[i]
		}

		if isStepInvariant {
			// The function and all arguments are step invariant.
			return true
		}

		for i, isi := range isStepInvariantSlice {
			if isi {
				n.Args[i] = newStepInvariantExpr(n.Args[i])
			}
		}
		return false

	case *parser.MatrixSelector:
		return preprocessExprHelper(n.VectorSelector, start, end)

	case *parser.SubqueryExpr:
		// Since we adjust offset for the @ modifier evaluation,
		// it gets tricky to adjust it for every subquery step.
		// Hence we wrap the inside of subquery irrespective of
		// @ on subquery (given it is also step invariant) so that
		// it is evaluated only once w.r.t. the start time of subquery.
		isInvariant := preprocessExprHelper(n.Expr, start, end)
		if isInvariant {
			n.Expr = newStepInvariantExpr(n.Expr)
		}
		if n.StartOrEnd == parser.START {
			n.Timestamp = makeInt64Pointer(timestamp.FromTime(start))
		} else if n.StartOrEnd == parser.END {
			n.Timestamp = makeInt64Pointer(timestamp.FromTime(end))
		}
		return n.Timestamp != nil

	case *parser.ParenExpr:
		return preprocessExprHelper(n.Expr, start, end)

	case *parser.UnaryExpr:
		return preprocessExprHelper(n.Expr, start, end)

	case *parser.StringLiteral, *parser.NumberLiteral:
		return true
	}

	panic(fmt.Sprintf("found unexpected node %#v", expr))
}

// WrapWithInvariantExpr wraps all possible step invariant parts of the given expression with
// StepInvariantExpr. It also resolves the preprocessors.
func WrapWithInvariantExpr(expr parser.Expr, start, end time.Time) parser.Expr {
	isStepInvariant := preprocessExprHelper(expr, start, end)
	if isStepInvariant {
		return newStepInvariantExpr(expr)
	}
	return expr
}

func newStepInvariantExpr(expr parser.Expr) parser.Expr {
	if e, ok := expr.(*parser.ParenExpr); ok {
		// Wrapping the inside of () makes it easy to unwrap the paren later.
		// But this effectively unwraps the paren.
		return newStepInvariantExpr(e.Expr)
	}
	return &parser.StepInvariantExpr{Expr: expr}
}

func FoldConstants(expr parser.Expr) parser.Expr {
	v := expr.(parser.Node)
	parser.InspectPostOrder(&v, func(node *parser.Node, path []parser.Node) error {
		if node == nil {
			return nil
		}
		switch n := (*node).(type) {
		case *parser.EvalStmt:
			n.Expr = resolveBinaryExpr(n.Expr)
		case *parser.Expressions:
			for i, e := range *n {
				(*n)[i] = resolveBinaryExpr(e)
			}
		case *parser.AggregateExpr:
			n.Param = resolveBinaryExpr(n.Param)
		case *parser.Call:
			for i, e := range (*n).Args {
				(*n).Args[i] = resolveBinaryExpr(e)
			}
		case *parser.SubqueryExpr:
			n.Expr = resolveBinaryExpr(n.Expr)
		case *parser.BinaryExpr:
			n.LHS = resolveBinaryExpr(n.LHS)
			n.RHS = resolveBinaryExpr(n.RHS)
			*node = resolveBinaryExpr(n)
		case *parser.ParenExpr:
			n.Expr = resolveBinaryExpr(n.Expr)
		}
		return nil
	})
	return v.(parser.Expr)
}

func resolveBinaryExpr(expr parser.Expr) parser.Expr {
	switch n := expr.(type) {
	case *parser.ParenExpr:
		unwrapParenExpr(&expr)
		return resolveBinaryExpr(expr)
	case *parser.BinaryExpr:
		lt, lOK := n.LHS.(*parser.NumberLiteral)
		rt, rOK := n.RHS.(*parser.NumberLiteral)
		if lOK && rOK {
			res := scalarBinop(n.Op, lt.Val, rt.Val)
			newNode := parser.NumberLiteral{
				Val: res,
			}
			return &newNode
		}
	}
	return expr
}

func OptimizeQuery(q Query) Query {
	s, ok := q.Statement().(*parser.EvalStmt)
	if !ok {
		return q
	}

	expr := FoldConstants(s.Expr)
	expr = WrapWithInvariantExpr(expr, s.Start, s.End)

	s.Expr = expr
	return q
}
