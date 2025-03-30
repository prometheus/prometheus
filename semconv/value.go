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

package semconv

import (
	"fmt"

	"github.com/prometheus/prometheus/promql/parser"
)

type valueTransformer struct {
	expr []parser.Expr
}

func (vt valueTransformer) AddPromQL(toPromQL string) (valueTransformer, error) {
	p := parser.NewParser(toPromQL)
	expr, err := p.ParseExpr()
	if err != nil {
		return vt, fmt.Errorf("can't parse %v: %w", toPromQL, err)
	}
	// Validate it.
	if _, err = transform(expr, 0); err != nil {
		return vt, err
	}
	vt.expr = append(vt.expr, expr)
	return vt, nil
}

func (vt valueTransformer) Transform(v float64) float64 {
	for _, e := range vt.expr {
		// We did what we could and tested transform in constructor, skipping error here.
		v, _ = transform(e, v)
	}
	return v
}

func transform(node parser.Expr, in float64) (float64, error) {
	switch e := node.(type) {
	case *parser.NumberLiteral:
		return e.Val, nil
	case *parser.VectorSelector:
		return in, nil
	case *parser.BinaryExpr:
		lhs, err := transform(e.LHS, in)
		if err != nil {
			return 0, err
		}
		rhs, err := transform(e.RHS, in)
		if err != nil {
			return 0, err
		}
		switch e.Op {
		case parser.ADD:
			return lhs + rhs, nil
		case parser.SUB:
			return lhs - rhs, nil
		case parser.MUL:
			return lhs * rhs, nil
		case parser.DIV:
			return lhs / rhs, nil
		default:
			return 0, fmt.Errorf("binary operator %v not allowed", parser.ItemTypeStr[e.Op])
		}
	default:
		return 0, fmt.Errorf("PromQL node %v not allowed", e.Type())
	}
}
