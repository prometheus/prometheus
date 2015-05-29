// Copyright 2015 The Prometheus Authors
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
	"reflect"
	"sort"
	"strings"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/util/strutil"
)

func (matrix Matrix) String() string {
	metricStrings := make([]string, 0, len(matrix))
	for _, sampleStream := range matrix {
		metricName, hasName := sampleStream.Metric.Metric[clientmodel.MetricNameLabel]
		numLabels := len(sampleStream.Metric.Metric)
		if hasName {
			numLabels--
		}
		labelStrings := make([]string, 0, numLabels)
		for label, value := range sampleStream.Metric.Metric {
			if label != clientmodel.MetricNameLabel {
				labelStrings = append(labelStrings, fmt.Sprintf("%s=%q", label, value))
			}
		}
		sort.Strings(labelStrings)
		valueStrings := make([]string, 0, len(sampleStream.Values))
		for _, value := range sampleStream.Values {
			valueStrings = append(valueStrings,
				fmt.Sprintf("\n%v @[%v]", value.Value, value.Timestamp))
		}
		metricStrings = append(metricStrings,
			fmt.Sprintf("%s{%s} => %s",
				metricName,
				strings.Join(labelStrings, ", "),
				strings.Join(valueStrings, ", ")))
	}
	sort.Strings(metricStrings)
	return strings.Join(metricStrings, "\n")
}

func (vector Vector) String() string {
	metricStrings := make([]string, 0, len(vector))
	for _, sample := range vector {
		metricStrings = append(metricStrings,
			fmt.Sprintf("%s => %v @[%v]",
				sample.Metric,
				sample.Value, sample.Timestamp))
	}
	return strings.Join(metricStrings, "\n")
}

// Tree returns a string of the tree structure of the given node.
func Tree(node Node) string {
	return tree(node, "")
}

func tree(node Node, level string) string {
	if node == nil {
		return fmt.Sprintf("%s |---- %T\n", level, node)
	}
	typs := strings.Split(fmt.Sprintf("%T", node), ".")[1]

	var t string
	// Only print the number of statements for readability.
	if stmts, ok := node.(Statements); ok {
		t = fmt.Sprintf("%s |---- %s :: %d\n", level, typs, len(stmts))
	} else {
		t = fmt.Sprintf("%s |---- %s :: %s\n", level, typs, node)
	}

	level += " · · ·"

	switch n := node.(type) {
	case Statements:
		for _, s := range n {
			t += tree(s, level)
		}
	case *AlertStmt:
		t += tree(n.Expr, level)

	case *EvalStmt:
		t += tree(n.Expr, level)

	case *RecordStmt:
		t += tree(n.Expr, level)

	case Expressions:
		for _, e := range n {
			t += tree(e, level)
		}
	case *AggregateExpr:
		t += tree(n.Expr, level)

	case *BinaryExpr:
		t += tree(n.LHS, level)
		t += tree(n.RHS, level)

	case *Call:
		t += tree(n.Args, level)

	case *ParenExpr:
		t += tree(n.Expr, level)

	case *UnaryExpr:
		t += tree(n.Expr, level)

	case *MatrixSelector, *NumberLiteral, *StringLiteral, *VectorSelector:
		// nothing to do

	default:
		panic("promql.Tree: not all node types covered")
	}
	return t
}

func (stmts Statements) String() (s string) {
	if len(stmts) == 0 {
		return ""
	}
	for _, stmt := range stmts {
		s += stmt.String()
		s += "\n\n"
	}
	return s[:len(s)-2]
}

func (node *AlertStmt) String() string {
	s := fmt.Sprintf("ALERT %s", node.Name)
	s += fmt.Sprintf("\n\tIF %s", node.Expr)
	if node.Duration > 0 {
		s += fmt.Sprintf("\n\tFOR %s", strutil.DurationToString(node.Duration))
	}
	if len(node.Labels) > 0 {
		s += fmt.Sprintf("\n\tWITH %s", node.Labels)
	}
	s += fmt.Sprintf("\n\tSUMMARY %q", node.Summary)
	s += fmt.Sprintf("\n\tDESCRIPTION %q", node.Description)
	return s
}

func (node *EvalStmt) String() string {
	return "EVAL " + node.Expr.String()
}

func (node *RecordStmt) String() string {
	s := fmt.Sprintf("%s%s = %s", node.Name, node.Labels, node.Expr)
	return s
}

func (es Expressions) String() (s string) {
	if len(es) == 0 {
		return ""
	}
	for _, e := range es {
		s += e.String()
		s += ", "
	}
	return s[:len(s)-2]
}

func (node *AggregateExpr) String() string {
	aggrString := fmt.Sprintf("%s(%s)", node.Op, node.Expr)
	if len(node.Grouping) > 0 {
		return fmt.Sprintf("%s BY (%s)", aggrString, node.Grouping)
	}
	return aggrString
}

func (node *BinaryExpr) String() string {
	matching := ""
	vm := node.VectorMatching
	if vm != nil && len(vm.On) > 0 {
		matching = fmt.Sprintf(" ON(%s)", vm.On)
		if vm.Card == CardManyToOne {
			matching += fmt.Sprintf(" GROUP_LEFT(%s)", vm.Include)
		}
		if vm.Card == CardOneToMany {
			matching += fmt.Sprintf(" GROUP_RIGHT(%s)", vm.Include)
		}
	}
	return fmt.Sprintf("%s %s%s %s", node.LHS, node.Op, matching, node.RHS)
}

func (node *Call) String() string {
	return fmt.Sprintf("%s(%s)", node.Func.Name, node.Args)
}

func (node *MatrixSelector) String() string {
	vecSelector := &VectorSelector{
		Name:          node.Name,
		LabelMatchers: node.LabelMatchers,
	}
	return fmt.Sprintf("%s[%s]", vecSelector.String(), strutil.DurationToString(node.Range))
}

func (node *NumberLiteral) String() string {
	return fmt.Sprint(node.Val)
}

func (node *ParenExpr) String() string {
	return fmt.Sprintf("(%s)", node.Expr)
}

func (node *StringLiteral) String() string {
	return fmt.Sprintf("%q", node.Val)
}

func (node *UnaryExpr) String() string {
	return fmt.Sprintf("%s%s", node.Op, node.Expr)
}

func (node *VectorSelector) String() string {
	labelStrings := make([]string, 0, len(node.LabelMatchers)-1)
	for _, matcher := range node.LabelMatchers {
		// Only include the __name__ label if its no equality matching.
		if matcher.Name == clientmodel.MetricNameLabel && matcher.Type == metric.Equal {
			continue
		}
		labelStrings = append(labelStrings, matcher.String())
	}

	if len(labelStrings) == 0 {
		return node.Name
	}
	sort.Strings(labelStrings)
	return fmt.Sprintf("%s{%s}", node.Name, strings.Join(labelStrings, ","))
}

// DotGraph returns a DOT representation of a statement list.
func (ss Statements) DotGraph() string {
	graph := ""
	for _, stmt := range ss {
		graph += stmt.DotGraph()
	}
	return graph
}

// DotGraph returns a DOT representation of the alert statement.
func (node *AlertStmt) DotGraph() string {
	graph := fmt.Sprintf(
		`digraph "Alert Statement" {
	  %#p[shape="box",label="ALERT %s IF FOR %s"];
		%#p -> %x;
		%s
	}`,
		node, node.Name, strutil.DurationToString(node.Duration),
		node, reflect.ValueOf(node.Expr).Pointer(),
		node.Expr.DotGraph(),
	)
	return graph
}

// DotGraph returns a DOT representation of the eval statement.
func (node *EvalStmt) DotGraph() string {
	graph := fmt.Sprintf(
		`%#p[shape="box",label="[%d:%s:%d]";
		%#p -> %x;
		%s
	}`,
		node, node.Start, node.End, node.Interval,
		node, reflect.ValueOf(node.Expr).Pointer(),
		node.Expr.DotGraph(),
	)
	return graph
}

// DotGraph returns a DOT representation of the record statement.
func (node *RecordStmt) DotGraph() string {
	graph := fmt.Sprintf(
		`%#p[shape="box",label="%s = "];
		%#p -> %x;
		%s
	}`,
		node, node.Name,
		node, reflect.ValueOf(node.Expr).Pointer(),
		node.Expr.DotGraph(),
	)
	return graph
}

// DotGraph returns a DOT representation of // DotGraph returns a DOT representation of the record statement.
// DotGraph returns a DOT representation of a statement list.
func (es Expressions) DotGraph() string {
	graph := ""
	for _, expr := range es {
		graph += expr.DotGraph()
	}
	return graph
}

// DotGraph returns a DOT representation of the vector aggregation.
func (node *AggregateExpr) DotGraph() string {
	groupByStrings := make([]string, 0, len(node.Grouping))
	for _, label := range node.Grouping {
		groupByStrings = append(groupByStrings, string(label))
	}

	graph := fmt.Sprintf("%#p[label=\"%s BY (%s)\"]\n",
		node,
		node.Op,
		strings.Join(groupByStrings, ", "))
	graph += fmt.Sprintf("%#p -> %x;\n", node, reflect.ValueOf(node.Expr).Pointer())
	graph += node.Expr.DotGraph()
	return graph
}

// DotGraph returns a DOT representation of the expression.
func (node *BinaryExpr) DotGraph() string {
	nodeAddr := reflect.ValueOf(node).Pointer()
	graph := fmt.Sprintf(
		`
		%x[label="%s"];
		%x -> %x;
		%x -> %x;
		%s
		%s
	}`,
		nodeAddr, node.Op,
		nodeAddr, reflect.ValueOf(node.LHS).Pointer(),
		nodeAddr, reflect.ValueOf(node.RHS).Pointer(),
		node.LHS.DotGraph(),
		node.RHS.DotGraph(),
	)
	return graph
}

// DotGraph returns a DOT representation of the function call.
func (node *Call) DotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%s\"];\n", node, node.Func.Name)
	graph += functionArgsToDotGraph(node, node.Args)
	return graph
}

// DotGraph returns a DOT representation of the number literal.
func (node *NumberLiteral) DotGraph() string {
	return fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.Val)
}

// DotGraph returns a DOT representation of the encapsulated expression.
func (node *ParenExpr) DotGraph() string {
	return node.Expr.DotGraph()
}

// DotGraph returns a DOT representation of the matrix selector.
func (node *MatrixSelector) DotGraph() string {
	return fmt.Sprintf("%#p[label=\"%s\"];\n", node, node)
}

// DotGraph returns a DOT representation of the string literal.
func (node *StringLiteral) DotGraph() string {
	return fmt.Sprintf("%#p[label=\"'%q'\"];\n", node, node.Val)
}

// DotGraph returns a DOT representation of the unary expression.
func (node *UnaryExpr) DotGraph() string {
	nodeAddr := reflect.ValueOf(node).Pointer()
	graph := fmt.Sprintf(
		`
		%x[label="%s"];
		%x -> %x;
		%s
		%s
	}`,
		nodeAddr, node.Op,
		nodeAddr, reflect.ValueOf(node.Expr).Pointer(),
		node.Expr.DotGraph(),
	)
	return graph
}

// DotGraph returns a DOT representation of the vector selector.
func (node *VectorSelector) DotGraph() string {
	return fmt.Sprintf("%#p[label=\"%s\"];\n", node, node)
}

func functionArgsToDotGraph(node Node, args Expressions) string {
	graph := args.DotGraph()
	for _, arg := range args {
		graph += fmt.Sprintf("%x -> %x;\n", reflect.ValueOf(node).Pointer(), reflect.ValueOf(arg).Pointer())
	}
	return graph
}
