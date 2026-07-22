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
// limitations under the License

package promql

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql/parser"
)

func addVar(vars map[string]string, value string) (varName string) {
	// TODO: Naive, fix.
	if strings.HasPrefix(value, "(") {
		value = strings.TrimPrefix(strings.TrimSuffix(value, ")"), "(")
	}

	// TODO: This is naive. We can do more with partial searches and "compacting" of variables
	// e.g. when adding looping_time{group_name="realtime",location="us-east1"}[2m]
	// to a vars with looping_time{group_name="realtime",location="us-east1"}[2m] | rate | sum | histogram_sum
	// we could have simpler variables.
	for k, v := range vars {
		if value == v {
			return k
		}
	}

	varName = fmt.Sprintf("x%d", len(vars)+1)
	vars[varName] = value
	return varName
}

// ToPiped transforms a standard PromQL query string into the piped syntax.
func ToPiped(query string) (string, error) {
	expr, err := parser.ParseExpr(query)
	if err != nil {
		return "", err
	}

	vars := map[string]string{}
	ret := bytes.NewBuffer(nil)
	printPipedWithVars(expr, ret, vars)

	if len(vars) == 0 {
		return ret.String(), nil
	}

	b := bytes.NewBuffer(nil)
	b.WriteString("let\n")
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		b.WriteString("  ")
		b.WriteString(k)
		b.WriteString(" = ")
		b.WriteString(vars[k])
		b.WriteString("\n")
	}
	b.WriteString("in ")
	b.WriteString(ret.String())
	return b.String(), nil
}

func stringPipedWithVars(node parser.Node, vars map[string]string) string {
	b := bytes.NewBuffer(nil)
	printPipedWithVars(node, b, vars)
	return b.String()
}

func writeLabels(b *bytes.Buffer, ss []string) {
	for i, s := range ss {
		if i > 0 {
			b.WriteString(", ")
		}
		if !model.LegacyValidation.IsValidMetricName(s) {
			b.Write(strconv.AppendQuote(b.AvailableBuffer(), s))
		} else {
			b.WriteString(s)
		}
	}
}

func printPipedWithVars(node parser.Node, b *bytes.Buffer, vars map[string]string) {
	switch n := node.(type) {
	case *parser.EvalStmt:
		printPipedWithVars(n.Expr, b, vars)
	case parser.Expressions:
		for _, e := range n {
			printPipedWithVars(e, b, vars)
		}
	case *parser.AggregateExpr:
		b.WriteString(stringPipedWithVars(n.Expr, vars))
		b.WriteString(" | ")
		b.WriteString(n.Op.String())
		if n.Op.IsAggregatorWithParam() {
			b.WriteString("(")
			b.WriteString(n.Param.String())
			b.WriteString(")")
		}
		switch {
		case n.Without:
			b.WriteString(" without (")
			writeLabels(b, n.Grouping)
			b.WriteString(") ")
		case len(n.Grouping) > 0:
			b.WriteString(" by (")
			writeLabels(b, n.Grouping)
			b.WriteString(") ")
		}
	case *parser.BinaryExpr:
		var lhs, rhs string
		switch {
		case n.LHS.Type() == parser.ValueTypeScalar && n.RHS.Type() == parser.ValueTypeScalar:
			// Two scalars.
			lhs = n.LHS.String()
			rhs = n.RHS.String()
		case n.LHS.Type() != parser.ValueTypeScalar && n.RHS.Type() != parser.ValueTypeScalar:
			pre := len(vars)
			lhs = stringPipedWithVars(n.LHS, vars)
			diff := len(vars) - pre

			// This is hacky, might be not very true for nested things.
			if diff == 0 {
				lhs = addVar(vars, lhs)
			}

			pre = len(vars)
			rhs = stringPipedWithVars(n.RHS, vars)
			diff = len(vars) - pre

			// This is hacky, might be not very true for nested things.
			if diff == 0 {
				rhs = addVar(vars, rhs)
			}

		case n.LHS.Type() == parser.ValueTypeScalar:
			// With pipe syntax we organize simpler form to the right.
			lhs = stringPipedWithVars(n.RHS, vars)
			rhs = n.LHS.String()
		case n.RHS.Type() == parser.ValueTypeScalar:
			// With pipe syntax we organize simpler form to the right.
			lhs = stringPipedWithVars(n.LHS, vars)
			rhs = n.RHS.String()
		}

		b.WriteString(lhs)
		b.WriteString(" ")
		b.WriteString(n.Op.String())
		if n.ReturnBool {
			b.WriteString(" bool")
		}
		b.WriteString(n.GetMatchingStr())
		b.WriteString(" ")
		b.WriteString(rhs)
	case *parser.Call:
		var (
			args = bytes.NewBuffer(nil)
			lhs  string
		)
		if len(n.Args) > 0 {
			for _, e := range n.Args {
				if e.Type() == parser.ValueTypeScalar || e.Type() == parser.ValueTypeString {
					if args.Len() > 0 {
						args.WriteString(", ")
					}
					args.WriteString(e.String())
					continue
				}
				if lhs != "" {
					// More than one complex arg (e.g. info function).
					// TODO: This is YOLO, one could think if there's a more readable way..
					if args.Len() > 0 {
						args.WriteString(", ")
					}
					args.WriteString(addVar(vars, lhs))
				}
				lhs = stringPipedWithVars(e, vars)
			}
		}
		if lhs != "" {
			b.WriteString(lhs)
			b.WriteString(" | ")
		}
		b.WriteString(n.Func.Name)
		if args.Len() > 0 {
			b.WriteString("(")
			b.WriteString(args.String())
			b.WriteString(")")
		}

	case *parser.SubqueryExpr:
		b.WriteString(stringPipedWithVars(n.Expr, vars))
	case *parser.ParenExpr:
		b.WriteString("(")
		b.WriteString(stringPipedWithVars(n.Expr, vars))
		b.WriteString(")")
	case *parser.UnaryExpr:
		b.WriteString(n.String())
	case *parser.MatrixSelector:
		b.WriteString(n.String())
	case *parser.StepInvariantExpr:
		b.WriteString(n.String())
	case *parser.NumberLiteral, *parser.StringLiteral, *parser.VectorSelector:
		b.WriteString(n.String())
	default:
		panic(fmt.Errorf("promql.printPiped: unhandled node type %T", node))
	}
}
