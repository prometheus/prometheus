// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"strconv"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// Take a Go PromQL AST and translate it to an object that's nicely JSON-serializable
// for the tree view in the UI.
// TODO: Could it make sense to do this via the normal JSON marshalling methods? Maybe
// too UI-specific though.
func translateAST(node parser.Expr) any {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *parser.AggregateExpr:
		return map[string]any{
			"type":     "aggregation",
			"op":       n.Op.String(),
			"expr":     translateAST(n.Expr),
			"param":    translateAST(n.Param),
			"grouping": sanitizeList(n.Grouping),
			"without":  n.Without,
		}
	case *parser.BinaryExpr:
		var matching any
		if m := n.VectorMatching; m != nil {
			matching = map[string]any{
				"card":    m.Card.String(),
				"labels":  sanitizeList(m.MatchingLabels),
				"on":      m.On,
				"include": sanitizeList(m.Include),
			}
		}

		return map[string]any{
			"type":     "binaryExpr",
			"op":       n.Op.String(),
			"lhs":      translateAST(n.LHS),
			"rhs":      translateAST(n.RHS),
			"matching": matching,
			"bool":     n.ReturnBool,
		}
	case *parser.Call:
		args := []any{}
		for _, arg := range n.Args {
			args = append(args, translateAST(arg))
		}

		return map[string]any{
			"type": "call",
			"func": map[string]any{
				"name":       n.Func.Name,
				"argTypes":   n.Func.ArgTypes,
				"variadic":   n.Func.Variadic,
				"returnType": n.Func.ReturnType,
			},
			"args": args,
		}
	case *parser.MatrixSelector:
		vs := n.VectorSelector.(*parser.VectorSelector)
		return map[string]any{
			"type":       "matrixSelector",
			"name":       vs.Name,
			"range":      n.Range.Milliseconds(),
			"offset":     vs.OriginalOffset.Milliseconds(),
			"matchers":   translateMatchers(vs.LabelMatchers),
			"timestamp":  vs.Timestamp,
			"startOrEnd": getStartOrEnd(vs.StartOrEnd),
		}
	case *parser.SubqueryExpr:
		return map[string]any{
			"type":       "subquery",
			"expr":       translateAST(n.Expr),
			"range":      n.Range.Milliseconds(),
			"offset":     n.OriginalOffset.Milliseconds(),
			"step":       n.Step.Milliseconds(),
			"timestamp":  n.Timestamp,
			"startOrEnd": getStartOrEnd(n.StartOrEnd),
		}
	case *parser.NumberLiteral:
		return map[string]string{
			"type": "numberLiteral",
			"val":  strconv.FormatFloat(n.Val, 'f', -1, 64),
		}
	case *parser.ParenExpr:
		return map[string]any{
			"type": "parenExpr",
			"expr": translateAST(n.Expr),
		}
	case *parser.StringLiteral:
		return map[string]any{
			"type": "stringLiteral",
			"val":  n.Val,
		}
	case *parser.UnaryExpr:
		return map[string]any{
			"type": "unaryExpr",
			"op":   n.Op.String(),
			"expr": translateAST(n.Expr),
		}
	case *parser.VectorSelector:
		return map[string]any{
			"type":       "vectorSelector",
			"name":       n.Name,
			"offset":     n.OriginalOffset.Milliseconds(),
			"matchers":   translateMatchers(n.LabelMatchers),
			"timestamp":  n.Timestamp,
			"startOrEnd": getStartOrEnd(n.StartOrEnd),
		}
	}
	panic("unsupported node type")
}

func sanitizeList(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func translateMatchers(in []*labels.Matcher) any {
	out := []map[string]any{}
	for _, m := range in {
		out = append(out, map[string]any{
			"name":  m.Name,
			"value": m.Value,
			"type":  m.Type.String(),
		})
	}
	return out
}

func getStartOrEnd(startOrEnd parser.ItemType) any {
	if startOrEnd == 0 {
		return nil
	}

	return startOrEnd.String()
}
