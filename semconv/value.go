package semconv

import (
	"fmt"

	"github.com/prometheus/prometheus/promql/parser"
)

type valueTransformer struct {
	expr parser.Expr
}

func newValueTransformer(toPromQL string) (*valueTransformer, error) {
	p := parser.NewParser(toPromQL)
	expr, err := p.ParseExpr()
	if err != nil {
		return nil, fmt.Errorf("can't parse %v: %w", toPromQL, err)
	}
	if _, err = transform(expr, 0); err != nil {
		return nil, err
	}

	return &valueTransformer{expr: expr}, nil
}

func (t *valueTransformer) Transform(v float64) float64 {
	if t == nil {
		return v // Noop.
	}
	// We did what we could and tested transform in constructor, skipping here.
	v, _ = transform(t.expr, v)
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
