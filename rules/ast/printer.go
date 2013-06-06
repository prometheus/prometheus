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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
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

func (opType BinOpType) String() string {
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
		AND: "AND",
		OR:  "OR",
	}
	return opTypeMap[opType]
}

func (aggrType AggrType) String() string {
	aggrTypeMap := map[AggrType]string{
		SUM:   "SUM",
		AVG:   "AVG",
		MIN:   "MIN",
		MAX:   "MAX",
		COUNT: "COUNT",
	}
	return aggrTypeMap[aggrType]
}

func (exprType ExprType) String() string {
	exprTypeMap := map[ExprType]string{
		SCALAR: "scalar",
		VECTOR: "vector",
		MATRIX: "matrix",
		STRING: "string",
	}
	return exprTypeMap[exprType]
}

func (vector Vector) String() string {
	metricStrings := make([]string, 0, len(vector))
	for _, sample := range vector {
		metricName, ok := sample.Metric[model.MetricNameLabel]
		if !ok {
			panic("Tried to print vector without metric name")
		}
		labelStrings := make([]string, 0, len(sample.Metric)-1)
		for label, value := range sample.Metric {
			if label != model.MetricNameLabel {
				// TODO escape special chars in label values here and elsewhere.
				labelStrings = append(labelStrings, fmt.Sprintf("%s='%s'", label, value))
			}
		}
		sort.Strings(labelStrings)
		metricStrings = append(metricStrings,
			fmt.Sprintf("%s{%s} => %v @[%v]",
				metricName,
				strings.Join(labelStrings, ","),
				sample.Value, sample.Timestamp))
	}
	return strings.Join(metricStrings, "\n")
}

func (matrix Matrix) String() string {
	metricStrings := make([]string, 0, len(matrix))
	for _, sampleSet := range matrix {
		metricName, ok := sampleSet.Metric[model.MetricNameLabel]
		if !ok {
			panic("Tried to print matrix without metric name")
		}
		labelStrings := make([]string, 0, len(sampleSet.Metric)-1)
		for label, value := range sampleSet.Metric {
			if label != model.MetricNameLabel {
				labelStrings = append(labelStrings, fmt.Sprintf("%s='%s'", label, value))
			}
		}
		sort.Strings(labelStrings)
		valueStrings := make([]string, 0, len(sampleSet.Values))
		for _, value := range sampleSet.Values {
			valueStrings = append(valueStrings,
				fmt.Sprintf("\n%v @[%v]", value.Value, value.Timestamp))
		}
		metricStrings = append(metricStrings,
			fmt.Sprintf("%s{%s} => %s",
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

func EvalToString(node Node, timestamp time.Time, format OutputFormat, storage *metric.TieredStorage, queryStats *stats.TimerGroup) string {
	viewTimer := queryStats.GetTimer(stats.TotalViewBuildingTime).Start()
	viewAdapter, err := viewAdapterForInstantQuery(node, timestamp, storage, queryStats)
	viewTimer.Stop()
	if err != nil {
		panic(err)
	}

	evalTimer := queryStats.GetTimer(stats.InnerEvalTime).Start()
	switch node.Type() {
	case SCALAR:
		scalar := node.(ScalarNode).Eval(timestamp, viewAdapter)
		evalTimer.Stop()
		switch format {
		case TEXT:
			return fmt.Sprintf("scalar: %v", scalar)
		case JSON:
			return TypedValueToJSON(scalar, "scalar")
		}
	case VECTOR:
		vector := node.(VectorNode).Eval(timestamp, viewAdapter)
		evalTimer.Stop()
		switch format {
		case TEXT:
			return vector.String()
		case JSON:
			return TypedValueToJSON(vector, "vector")
		}
	case MATRIX:
		matrix := node.(MatrixNode).Eval(timestamp, viewAdapter)
		evalTimer.Stop()
		switch format {
		case TEXT:
			return matrix.String()
		case JSON:
			return TypedValueToJSON(matrix, "matrix")
		}
	case STRING:
		str := node.(StringNode).Eval(timestamp, viewAdapter)
		evalTimer.Stop()
		switch format {
		case TEXT:
			return str
		case JSON:
			return TypedValueToJSON(str, "string")
		}
	}
	panic("Switch didn't cover all node types")
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
	graph := fmt.Sprintf("%#p[label=\"%s\"];\n", node, node.function.name)
	graph += functionArgsToDotGraph(node, node.args)
	return graph
}

func (node *ScalarArithExpr) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf(`
		%#p[label="%s"];
		%#p -> %#p;
		%#p -> %#p;
		%s
		%s
	}`, node, node.opType, node, node.lhs, node, node.rhs, node.lhs.NodeTreeToDotGraph(), node.rhs.NodeTreeToDotGraph())
	return graph
}

func (node *VectorLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"%s\"];\n", node, node)
}

func (node *VectorFunctionCall) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%s\"];\n", node, node.function.name)
	graph += functionArgsToDotGraph(node, node.args)
	return graph
}

func (node *VectorAggregation) NodeTreeToDotGraph() string {
	groupByStrings := make([]string, 0, len(node.groupBy))
	for _, label := range node.groupBy {
		groupByStrings = append(groupByStrings, string(label))
	}

	graph := fmt.Sprintf("%#p[label=\"%s BY (%s)\"]\n",
		node,
		node.aggrType,
		strings.Join(groupByStrings, ", "))
	graph += fmt.Sprintf("%#p -> %#p;\n", node, node.vector)
	graph += node.vector.NodeTreeToDotGraph()
	return graph
}

func (node *VectorArithExpr) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf(`
		%#p[label="%s"];
		%#p -> %#p;
		%#p -> %#p;
		%s
		%s
	`, node, node.opType, node, node.lhs, node, node.rhs, node.lhs.NodeTreeToDotGraph(), node.rhs.NodeTreeToDotGraph())
	return graph
}

func (node *MatrixLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"%s\"];\n", node, node)
}

func (node *StringLiteral) NodeTreeToDotGraph() string {
	return fmt.Sprintf("%#p[label=\"'%s'\"];\n", node, node.str)
}

func (node *StringFunctionCall) NodeTreeToDotGraph() string {
	graph := fmt.Sprintf("%#p[label=\"%s\"];\n", node, node.function.name)
	graph += functionArgsToDotGraph(node, node.args)
	return graph
}

func (nodes Nodes) String() string {
	nodeStrings := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeStrings = append(nodeStrings, node.String())
	}
	return strings.Join(nodeStrings, ", ")
}

func (node *ScalarLiteral) String() string {
	return fmt.Sprint(node.value)
}

func (node *ScalarFunctionCall) String() string {
	return fmt.Sprintf("%s(%s)", node.function.name, node.args)
}

func (node *ScalarArithExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", node.lhs, node.opType, node.rhs)
}

func (node *VectorLiteral) String() string {
	metricName, ok := node.labels[model.MetricNameLabel]
	if !ok {
		panic("Tried to print vector without metric name")
	}
	labelStrings := make([]string, 0, len(node.labels)-1)
	for label, value := range node.labels {
		if label != model.MetricNameLabel {
			labelStrings = append(labelStrings, fmt.Sprintf("%s='%s'", label, value))
		}
	}
	sort.Strings(labelStrings)
	return fmt.Sprintf("%s{%s}", metricName, strings.Join(labelStrings, ","))
}

func (node *VectorFunctionCall) String() string {
	return fmt.Sprintf("%s(%s)", node.function.name, node.args)
}

func (node *VectorAggregation) String() string {
	return fmt.Sprintf("%s(%s) BY (%s)", node.aggrType, node.vector, node.groupBy)
}

func (node *VectorArithExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", node.lhs, node.opType, node.rhs)
}

func (node *MatrixLiteral) String() string {
	vectorString := (&VectorLiteral{labels: node.labels}).String()
	intervalString := fmt.Sprintf("[%s]", utility.DurationToString(node.interval))
	return vectorString + intervalString
}

func (node *StringLiteral) String() string {
	return fmt.Sprintf("'%s'", node.str)
}

func (node *StringFunctionCall) String() string {
	return fmt.Sprintf("%s(%s)", node.function.name, node.args)
}
