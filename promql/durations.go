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

// durationVisitor is a visitor that visits a duration expression and calculates the duration.
type durationVisitor struct{}

func (v *durationVisitor) Visit(node parser.Node, _ []parser.Node) (parser.Visitor, error) {
	switch n := node.(type) {
	case *parser.VectorSelector:
		if n.OriginalOffsetExpr != nil {
			duration, err := calculateDuration(n.OriginalOffsetExpr, true)
			if err != nil {
				return nil, err
			}
			n.OriginalOffset = duration
		}
	case *parser.MatrixSelector:
		if n.RangeExpr != nil {
			duration, err := calculateDuration(n.RangeExpr, false)
			if err != nil {
				return nil, err
			}
			n.Range = duration
		}
	case *parser.SubqueryExpr:
		if n.OriginalOffsetExpr != nil {
			duration, err := calculateDuration(n.OriginalOffsetExpr, true)
			if err != nil {
				return nil, err
			}
			n.OriginalOffset = duration
		}
		if n.StepExpr != nil {
			duration, err := calculateDuration(n.StepExpr, false)
			if err != nil {
				return nil, err
			}
			n.Step = duration
		}
		if n.RangeExpr != nil {
			duration, err := calculateDuration(n.RangeExpr, false)
			if err != nil {
				return nil, err
			}
			n.Range = duration
		}
	}
	return v, nil
}

// calculateDuration computes the duration from a duration expression.
func calculateDuration(expr parser.Expr, allowedNegative bool) (time.Duration, error) {
	duration, err := evaluateDurationExpr(expr)
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
func evaluateDurationExpr(expr parser.Expr) (float64, error) {
	switch n := expr.(type) {
	case *parser.NumberLiteral:
		return n.Val, nil
	case *parser.DurationExpr:
		var lhs, rhs float64
		var err error

		if n.LHS != nil {
			lhs, err = evaluateDurationExpr(n.LHS)
			if err != nil {
				return 0, err
			}
		}

		rhs, err = evaluateDurationExpr(n.RHS)
		if err != nil {
			return 0, err
		}

		switch n.Op {
		case parser.ADD:
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
