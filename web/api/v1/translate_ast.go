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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type typeExpr string

const (
	aggregationType   typeExpr = "aggregation"
	binaryType        typeExpr = "binaryExpr"
	callType          typeExpr = "call"
	matrixType        typeExpr = "matrixExpr"
	vectorType        typeExpr = "vectorSelector"
	subqueryType      typeExpr = "subquery"
	numberLiteralType typeExpr = "numberLiteral"
	stringLiteralType typeExpr = "stringLiteral"
	parentExprType    typeExpr = "parenExpr"
	unaryExprType     typeExpr = "unaryExpr"
)

type aggregateExpr struct {
	Type     typeExpr `json:"type"`
	Op       string   `json:"op"`
	Expr     any      `json:"expr"`
	Param    any      `json:"param"`
	Grouping []string `json:"grouping"`
	Without  bool     `json:"without"`
}

type matchingBinaryExpr struct {
	Card    string   `json:"card"`
	Labels  []string `json:"labels"`
	On      bool     `json:"on"`
	Include []string `json:"include"`
}

type binaryExpr struct {
	Type     typeExpr            `json:"type"`
	Op       string              `json:"op"`
	LHS      any                 `json:"lhs"`
	RHS      any                 `json:"rhs"`
	Matching *matchingBinaryExpr `json:"matching,omitempty"`
	Bool     bool                `json:"bool"`
}

type functionCall struct {
	Name       string
	ArgTypes   []parser.ValueType `json:"argTypes"`
	Variadic   int                `json:"variadic"`
	ReturnType parser.ValueType   `json:"returnType"`
}

type callExpr struct {
	Type typeExpr     `json:"type"`
	Func functionCall `json:"functionCall"`
	Args []any        `json:"args,omitempty"`
}

type matcher struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type matrixSelector struct {
	Type       typeExpr  `json:"type"`
	Name       string    `json:"name"`
	Range      int64     `json:"range"`
	Offset     int64     `json:"offset"`
	Matchers   []matcher `json:"matchers"`
	Timestamp  *int64    `json:"timestamp,omitempty"`
	StartOrEnd *string   `json:"startOrEnd,omitempty"`
}

type vectorSelector struct {
	Type       typeExpr  `json:"type"`
	Name       string    `json:"name"`
	Offset     int64     `json:"offset"`
	Matchers   []matcher `json:"matchers"`
	Timestamp  *int64    `json:"timestamp,omitempty"`
	StartOrEnd *string   `json:"startOrEnd,omitempty"`
}

type SubQueryExpr struct {
	Type       typeExpr `json:"type"`
	Expr       any      `json:"expr"`
	Range      int64    `json:"range"`
	Offset     int64    `json:"offset"`
	Step       int64    `json:"step"`
	Timestamp  *int64   `json:"timestamp,omitempty"`
	StartOrEnd *string  `json:"startOrEnd,omitempty"`
}

type numberLiteral struct {
	Type typeExpr `json:"type"`
	Val  float64  `json:"val"`
}

type stringLitteral struct {
	Type typeExpr `json:"type"`
	Val  string   `json:"val"`
}

type parentExpr struct {
	Type typeExpr `json:"type"`
	Expr any      `json:"expr"`
}

type unaryExpr struct {
	Type typeExpr `json:"type"`
	Op   string   `json:"op"`
	Expr any      `json:"expr"`
}

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
		return &aggregateExpr{
			Type:     aggregationType,
			Op:       n.Op.String(),
			Expr:     translateAST(n.Expr),
			Param:    translateAST(n.Param),
			Grouping: sanitizeList(n.Grouping),
			Without:  n.Without,
		}
	case *parser.BinaryExpr:
		var matching *matchingBinaryExpr
		if m := n.VectorMatching; m != nil {
			matching = &matchingBinaryExpr{
				Card:    m.Card.String(),
				Labels:  sanitizeList(m.MatchingLabels),
				On:      m.On,
				Include: sanitizeList(m.Include),
			}
		}

		return &binaryExpr{
			Type:     binaryType,
			Op:       n.Op.String(),
			LHS:      translateAST(n.LHS),
			RHS:      translateAST(n.RHS),
			Matching: matching,
			Bool:     n.ReturnBool,
		}
	case *parser.Call:
		var args []any
		for _, arg := range n.Args {
			args = append(args, translateAST(arg))
		}

		return &callExpr{
			Type: callType,
			Func: functionCall{
				Name:       n.Func.Name,
				ArgTypes:   n.Func.ArgTypes,
				Variadic:   n.Func.Variadic,
				ReturnType: n.Func.ReturnType,
			},
			Args: args,
		}
	case *parser.MatrixSelector:
		vs := n.VectorSelector.(*parser.VectorSelector)
		return &matrixSelector{
			Type:       matrixType,
			Name:       vs.Name,
			Range:      n.Range.Milliseconds(),
			Offset:     vs.OriginalOffset.Milliseconds(),
			Matchers:   translateMatchers(vs.LabelMatchers),
			Timestamp:  vs.Timestamp,
			StartOrEnd: getStartOrEnd(vs.StartOrEnd),
		}
	case *parser.SubqueryExpr:
		return &SubQueryExpr{
			Type:       subqueryType,
			Expr:       translateAST(n.Expr),
			Range:      n.Range.Milliseconds(),
			Offset:     n.OriginalOffset.Milliseconds(),
			Step:       n.Step.Milliseconds(),
			Timestamp:  n.Timestamp,
			StartOrEnd: getStartOrEnd(n.StartOrEnd),
		}
	case *parser.NumberLiteral:
		return &numberLiteral{
			Type: numberLiteralType,
			Val:  n.Val,
		}
	case *parser.ParenExpr:
		return &parentExpr{
			Type: parentExprType,
			Expr: translateAST(n.Expr),
		}
	case *parser.StringLiteral:
		return &stringLitteral{
			Type: stringLiteralType,
			Val:  n.Val,
		}
	case *parser.UnaryExpr:
		return &unaryExpr{
			Type: unaryExprType,
			Op:   n.Op.String(),
			Expr: translateAST(n.Expr),
		}
	case *parser.VectorSelector:
		return &vectorSelector{
			Type:       vectorType,
			Name:       n.Name,
			Offset:     n.OriginalOffset.Milliseconds(),
			Matchers:   translateMatchers(n.LabelMatchers),
			Timestamp:  n.Timestamp,
			StartOrEnd: getStartOrEnd(n.StartOrEnd),
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

func translateMatchers(in []*labels.Matcher) []matcher {
	var out []matcher
	for _, m := range in {
		out = append(out, matcher{
			Name:  m.Name,
			Value: m.Value,
			Type:  m.Type.String(),
		})
	}
	return out
}

func getStartOrEnd(startOrEnd parser.ItemType) *string {
	if startOrEnd == 0 {
		return nil
	}
	result := startOrEnd.String()
	return &result
}
