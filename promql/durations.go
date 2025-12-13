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
	"fmt"
	"math"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
)

// durationVisitor is a visitor that calculates the actual value of
// duration expressions in AST nodes. For example the query
// "http_requests_total offset (1h / 2)" is represented in the AST
// as a VectorSelector with OriginalOffset 0 and the duration expression
// in OriginalOffsetExpr representing (1h / 2). This visitor evaluates
// such duration expression, setting OriginalOffset to 30m.
type durationVisitor struct {
	step       time.Duration
	queryRange time.Duration
}

// Visit finds any duration expressions in AST Nodes and modifies the Node to
// store the concrete value. Note that parser.Walk does NOT traverse the
// duration expressions such as OriginalOffsetExpr so we make our own recursive
// call on those to evaluate the result.
func (v *durationVisitor) Visit(node parser.Node, _ []parser.Node) (parser.Visitor, error) {
	switch n := node.(type) {
	case *parser.VectorSelector:
		if n.OriginalOffsetExpr != nil {
			duration, err := v.calculateDuration(n.OriginalOffsetExpr, true)
			if err != nil {
				return nil, err
			}
			n.OriginalOffset = duration
		}
	case *parser.MatrixSelector:
		if n.RangeExpr != nil {
			duration, err := v.calculateDuration(n.RangeExpr, false)
			if err != nil {
				return nil, err
			}
			n.Range = duration
		}
	case *parser.SubqueryExpr:
		if n.OriginalOffsetExpr != nil {
			duration, err := v.calculateDuration(n.OriginalOffsetExpr, true)
			if err != nil {
				return nil, err
			}
			n.OriginalOffset = duration
		}
		if n.StepExpr != nil {
			duration, err := v.calculateDuration(n.StepExpr, false)
			if err != nil {
				return nil, err
			}
			n.Step = duration
		}
		if n.RangeExpr != nil {
			duration, err := v.calculateDuration(n.RangeExpr, false)
			if err != nil {
				return nil, err
			}
			n.Range = duration
		}
	}
	return v, nil
}

// calculateDuration returns the float value of a duration expression as
// time.Duration or an error if the duration is invalid.
func (v *durationVisitor) calculateDuration(expr parser.Expr, allowedNegative bool) (time.Duration, error) {
	duration, err := v.evaluateDurationExpr(expr)
	if err != nil {
		return 0, err
	}
	if duration <= 0 && !allowedNegative {
		return 0, fmt.Errorf("%d:%d: duration must be greater than 0", expr.PositionRange().Start, expr.PositionRange().End)
	}
	if duration > 1<<63-1 || duration < -1<<63 {
		return 0, fmt.Errorf("%d:%d: duration is out of range", expr.PositionRange().Start, expr.PositionRange().End)
	}
	return time.Duration(duration*1000) * time.Millisecond, nil
}

// evaluateDurationExpr recursively evaluates a duration expression to a float64 value.
func (v *durationVisitor) evaluateDurationExpr(expr parser.Expr) (float64, error) {
	switch n := expr.(type) {
	case *parser.NumberLiteral:
		return n.Val, nil
	case *parser.DurationExpr:
		var lhs, rhs float64
		var err error

		if n.LHS != nil {
			lhs, err = v.evaluateDurationExpr(n.LHS)
			if err != nil {
				return 0, err
			}
		}

		if n.RHS != nil {
			rhs, err = v.evaluateDurationExpr(n.RHS)
			if err != nil {
				return 0, err
			}
		}

		switch n.Op {
		case parser.STEP:
			return float64(v.step.Seconds()), nil
		case parser.RANGE:
			return float64(v.queryRange.Seconds()), nil
		case parser.MIN:
			return math.Min(lhs, rhs), nil
		case parser.MAX:
			return math.Max(lhs, rhs), nil
		case parser.ADD:
			if n.LHS == nil {
				// Unary positive duration expression.
				return rhs, nil
			}
			return lhs + rhs, nil
		case parser.SUB:
			if n.LHS == nil {
				// Unary negative duration expression.
				return -rhs, nil
			}
			return lhs - rhs, nil
		case parser.MUL:
			return lhs * rhs, nil
		case parser.DIV:
			if rhs == 0 {
				return 0, fmt.Errorf("%d:%d: division by zero", expr.PositionRange().Start, expr.PositionRange().End)
			}
			return lhs / rhs, nil
		case parser.MOD:
			if rhs == 0 {
				return 0, fmt.Errorf("%d:%d: modulo by zero", expr.PositionRange().Start, expr.PositionRange().End)
			}
			return math.Mod(lhs, rhs), nil
		case parser.POW:
			return math.Pow(lhs, rhs), nil
		default:
			return 0, fmt.Errorf("unexpected duration expression operator %q", n.Op)
		}
	default:
		return 0, fmt.Errorf("unexpected duration expression type %T", n)
	}
}
