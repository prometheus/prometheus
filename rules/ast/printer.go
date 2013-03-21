// Copyright 2013 Prometheus Team
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

package ast

import (
	"encoding/json"
	"fmt"
	"github.com/prometheus/prometheus/utility"
	"sort"
	"strings"
	"time"
)

type OutputFormat int

const (
	TEXT OutputFormat = iota
	JSON
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

func exprTypeToString(exprType ExprType) string {
	exprTypeMap := map[ExprType]string{
		SCALAR: "scalar",
		VECTOR: "vector",
		MATRIX: "matrix",
		STRING: "string",
	}
	return exprTypeMap[exprType]
}

func (vector Vector) String() string {
	metricStrings := []string{}
	for _, sample := range vector {
		metricName, ok := sample.Metric["name"]
		if !ok {
			panic("Tried to print vector without metric name")
		}
		labelStrings := []string{}
		for label, value := range sample.Metric {
			if label != "name" {
				// TODO escape special chars in label values here and elsewhere.
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

func (matrix Matrix) String() string {
	metricStrings := []string{}
	for _, sampleSet := range matrix {
		metricName, ok := sampleSet.Metric["name"]
		if !ok {
			panic("Tried to print matrix without metric name")
		}
		labelStrings := []string{}
		for label, value := range sampleSet.Metric {
			if label != "name" {
				labelStrings = append(labelStrings, fmt.Sprintf("%v='%v'", label, value))
			}
		}
		sort.Strings(labelStrings)
		valueStrings := []string{}
		for _, value := range sampleSet.Values {
			valueStrings = append(valueStrings,
				fmt.Sprintf("\n%v @[%v]", value.Value, value.Timestamp))
		}
		metricStrings = append(metricStrings,
			fmt.Sprintf("%v{%v} => %v",
				metricName,
				strings.Join(labelStrings, ","),
				strings.Join(valueStrings, ", ")))
	}
	sort.Strings(metricStrings)
	return strings.Join(metricStrings, "\n")
}

func ErrorToJSON(err error) string {
	errorStruct := struct {
		Type  string
		Value string
	}{
		Type:  "error",
		Value: err.Error(),
	}

	errorJSON, err := json.MarshalIndent(errorStruct, "", "\t")
	if err != nil {
		return ""
	}
	return string(errorJSON)
}

func TypedValueToJSON(data interface{}, typeStr string) string {
	dataStruct := struct {
		Type  string
		Value interface{}
	}{
		Type:  typeStr,
		Value: data,
	}
	dataJSON, err := json.MarshalIndent(dataStruct, "", "\t")
	if err != nil {
		return ErrorToJSON(err)
	}
	return string(dataJSON)
}

func EvalToString(node Node, timestamp *time.Time, format OutputFormat) string {
	switch node.Type() {
	case SCALAR:
		scalar := node.(ScalarNode).Eval(timestamp)
		switch format {
		case TEXT:
			return fmt.Sprintf("scalar: %v", scalar)
		case JSON:
			return TypedValueToJSON(scalar, "scalar")
		}
	case VECTOR:
		vector := node.(VectorNode).Eval(timestamp)
		switch format {
		case TEXT:
			return vector.String()
		case JSON:
			return TypedValueToJSON(vector, "vector")
		}
	case MATRIX:
		matrix := node.(MatrixNode).Eval(timestamp)
		switch format {
		case TEXT:
			return matrix.String()
		case JSON:
			return TypedValueToJSON(matrix, "matrix")
		}
	case STRING:
		str := node.(StringNode).Eval(timestamp)
		switch format {
		case TEXT:
			return str
		case JSON:
			return TypedValueToJSON(str, "string")
		}
	}
	panic("Switch didn't cover all node types")
}

func (node *VectorLiteral) String() string {
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

func (node *MatrixLiteral) String() string {
	vectorString := (&VectorLiteral{labels: node.labels}).String()
	intervalString := fmt.Sprintf("['%v']", utility.DurationToString(node.interval))
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
	return fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.String())
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
	return fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.String())
}

func (node *StringLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"'%v'\"];\n", node, node.str)
}

func (node *StringFunctionCall) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%v\"];\n", node, node.function.name)
	graph += functionArgsToDotGraph(node, node.args)
	return graph
}
