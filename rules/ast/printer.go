package ast

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func binOpTypeToString(opType BinOpType) string {
	opTypeMap := map[BinOpType]string{
		ADD: "+",
		SUB: "-",
		MUL: "*",
		DIV: "/",
		MOD: "%",
		GT:  ">",
		LT:  "<",
		EQ:  "==",
		NE:  "!=",
		GE:  ">=",
		LE:  "<=",
	}
	return opTypeMap[opType]
}

func aggrTypeToString(aggrType AggrType) string {
	aggrTypeMap := map[AggrType]string{
		SUM: "SUM",
		AVG: "AVG",
		MIN: "MIN",
		MAX: "MAX",
	}
	return aggrTypeMap[aggrType]
}

func durationToString(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	factors := map[string]int64{
		"y": 60 * 60 * 24 * 365,
		"d": 60 * 60 * 24,
		"h": 60 * 60,
		"m": 60,
		"s": 1,
	}
	unit := "s"
	switch int64(0) {
	case seconds % factors["y"]:
		unit = "y"
	case seconds % factors["d"]:
		unit = "d"
	case seconds % factors["h"]:
		unit = "h"
	case seconds % factors["m"]:
		unit = "m"
	}
	return fmt.Sprintf("%v%v", seconds/factors[unit], unit)
}

func (vector Vector) ToString() string {
	metricStrings := []string{}
	for _, sample := range vector {
		metricName, ok := sample.Metric["name"]
		if !ok {
			panic("Tried to print vector without metric name")
		}
		labelStrings := []string{}
		for label, value := range sample.Metric {
			if label != "name" {
				labelStrings = append(labelStrings, fmt.Sprintf("%v='%v'", label, value))
			}
		}
		sort.Strings(labelStrings)
		metricStrings = append(metricStrings,
			fmt.Sprintf("%v{%v} => %v @[%v]",
				metricName,
				strings.Join(labelStrings, ","),
				sample.Value, sample.Timestamp))
	}
	sort.Strings(metricStrings)
	return strings.Join(metricStrings, "\n")
}

func (node *VectorLiteral) ToString() string {
	metricName, ok := node.labels["name"]
	if !ok {
		panic("Tried to print vector without metric name")
	}
	labelStrings := []string{}
	for label, value := range node.labels {
		if label != "name" {
			labelStrings = append(labelStrings, fmt.Sprintf("%v='%v'", label, value))
		}
	}
	sort.Strings(labelStrings)
	return fmt.Sprintf("%v{%v}", metricName, strings.Join(labelStrings, ","))
}

func (node *MatrixLiteral) ToString() string {
	vectorString := (&VectorLiteral{labels: node.labels}).ToString()
	intervalString := fmt.Sprintf("['%v']", durationToString(node.interval))
	return vectorString + intervalString
}

func (node *ScalarLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.value)
}

func functionArgsToDotGraph(node Node, args []Node) string {
	graph := ""
	for _, arg := range args {
		graph += fmt.Sprintf("%#p -> %#p;\n", node, arg)
	}
	for _, arg := range args {
		graph += arg.NodeTreeToDotGraph()
	}
	return graph
}

func (node *ScalarFunctionCall) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.function.name)
	graph += functionArgsToDotGraph(node, node.args)
	return graph
}

func (node *ScalarArithExpr) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%v\"];\n", node, binOpTypeToString(node.opType))
	graph += fmt.Sprintf("%#p -> %#p;\n", node, node.lhs)
	graph += fmt.Sprintf("%#p -> %#p;\n", node, node.rhs)
	graph += node.lhs.NodeTreeToDotGraph()
	graph += node.rhs.NodeTreeToDotGraph()
	return graph
}

func (node *VectorLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.ToString())
}

func (node *VectorFunctionCall) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.function.name)
	graph += functionArgsToDotGraph(node, node.args)
	return graph
}

func (node *VectorAggregation) NodeTreeToDotGraph() string {
	groupByStrings := []string{}
	for _, label := range node.groupBy {
		groupByStrings = append(groupByStrings, string(label))
	}

	graph := fmt.Sprintf("%#p[label=\"%v BY (%v)\"]\n",
		node,
		aggrTypeToString(node.aggrType),
		strings.Join(groupByStrings, ", "))
	graph += fmt.Sprintf("%#p -> %#p;\n", node, node.vector)
	graph += node.vector.NodeTreeToDotGraph()
	return graph
}

func (node *VectorArithExpr) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%v\"];\n", node, binOpTypeToString(node.opType))
	graph += fmt.Sprintf("%#p -> %#p;\n", node, node.lhs)
	graph += fmt.Sprintf("%#p -> %#p;\n", node, node.rhs)
	graph += node.lhs.NodeTreeToDotGraph()
	graph += node.rhs.NodeTreeToDotGraph()
	return graph
}

func (node *MatrixLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.ToString())
}

func (node *StringLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"'%v'\"];\n", node.str)
}

func (node *StringFunctionCall) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.function.name)
	graph += functionArgsToDotGraph(node, node.args)
	return graph
}
