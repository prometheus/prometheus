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
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
)

// ----------------------------------------------------------------------------
// Raw data value types.

type Vector clientmodel.Samples

// BUG(julius): Pointerize this.
type Matrix []metric.SampleSet

type groupedAggregation struct {
	labels     clientmodel.Metric
	value      clientmodel.SampleValue
	groupCount int
}

// ----------------------------------------------------------------------------
// Enums.

// Rule language expression types.
type ExprType int

const (
	SCALAR ExprType = iota
	VECTOR
	MATRIX
	STRING
)

// Binary operator types.
type BinOpType int

const (
	ADD BinOpType = iota
	SUB
	MUL
	DIV
	MOD
	NE
	EQ
	GT
	LT
	GE
	LE
	AND
	OR
)

// Aggregation types.
type AggrType int

const (
	SUM AggrType = iota
	AVG
	MIN
	MAX
	COUNT
)

// ----------------------------------------------------------------------------
// Interfaces.

// All node interfaces include the Node interface.
type Nodes []Node

type Node interface {
	Type() ExprType
	Children() Nodes
	NodeTreeToDotGraph() string
	String() string
}

// All node types implement one of the following interfaces. The name of the
// interface represents the type returned to the parent node.
type ScalarNode interface {
	Node
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) clientmodel.SampleValue
}

type VectorNode interface {
	Node
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Vector
}

type MatrixNode interface {
	Node
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix
	EvalBoundaries(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix
}

type StringNode interface {
	Node
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) string
}

// ----------------------------------------------------------------------------
// ScalarNode types.

type (
	// A numeric literal.
	ScalarLiteral struct {
		value clientmodel.SampleValue
	}

	// A function of numeric return type.
	ScalarFunctionCall struct {
		function *Function
		args     Nodes
	}

	// An arithmetic expression of numeric type.
	ScalarArithExpr struct {
		opType BinOpType
		lhs    ScalarNode
		rhs    ScalarNode
	}
)

// ----------------------------------------------------------------------------
// VectorNode types.

type (
	// Vector literal, i.e. metric name plus labelset.
	VectorLiteral struct {
		labels clientmodel.LabelSet
		// Fingerprints are populated from labels at query analysis time.
		fingerprints clientmodel.Fingerprints
	}

	// A function of vector return type.
	VectorFunctionCall struct {
		function *Function
		args     Nodes
	}

	// A vector aggregation with vector return type.
	VectorAggregation struct {
		aggrType        AggrType
		groupBy         clientmodel.LabelNames
		keepExtraLabels bool
		vector          VectorNode
	}

	// An arithmetic expression of vector type.
	VectorArithExpr struct {
		opType BinOpType
		lhs    VectorNode
		rhs    Node
	}
)

// ----------------------------------------------------------------------------
// MatrixNode types.

type (
	// Matrix literal, i.e. metric name plus labelset and timerange.
	MatrixLiteral struct {
		labels clientmodel.LabelSet
		// Fingerprints are populated from labels at query analysis time.
		fingerprints clientmodel.Fingerprints
		interval     time.Duration
	}
)

// ----------------------------------------------------------------------------
// StringNode types.

type (
	// String literal.
	StringLiteral struct {
		str string
	}

	// A function of string return type.
	StringFunctionCall struct {
		function *Function
		args     Nodes
	}
)

// ----------------------------------------------------------------------------
// Implementations.

// Node.Type() methods.
func (node ScalarLiteral) Type() ExprType      { return SCALAR }
func (node ScalarFunctionCall) Type() ExprType { return SCALAR }
func (node ScalarArithExpr) Type() ExprType    { return SCALAR }
func (node VectorLiteral) Type() ExprType      { return VECTOR }
func (node VectorFunctionCall) Type() ExprType { return VECTOR }
func (node VectorAggregation) Type() ExprType  { return VECTOR }
func (node VectorArithExpr) Type() ExprType    { return VECTOR }
func (node MatrixLiteral) Type() ExprType      { return MATRIX }
func (node StringLiteral) Type() ExprType      { return STRING }
func (node StringFunctionCall) Type() ExprType { return STRING }

// Node.Children() methods.
func (node ScalarLiteral) Children() Nodes      { return Nodes{} }
func (node ScalarFunctionCall) Children() Nodes { return node.args }
func (node ScalarArithExpr) Children() Nodes    { return Nodes{node.lhs, node.rhs} }
func (node VectorLiteral) Children() Nodes      { return Nodes{} }
func (node VectorFunctionCall) Children() Nodes { return node.args }
func (node VectorAggregation) Children() Nodes  { return Nodes{node.vector} }
func (node VectorArithExpr) Children() Nodes    { return Nodes{node.lhs, node.rhs} }
func (node MatrixLiteral) Children() Nodes      { return Nodes{} }
func (node StringLiteral) Children() Nodes      { return Nodes{} }
func (node StringFunctionCall) Children() Nodes { return node.args }

func (node *ScalarLiteral) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) clientmodel.SampleValue {
	return node.value
}

func (node *ScalarArithExpr) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) clientmodel.SampleValue {
	lhs := node.lhs.Eval(timestamp, view)
	rhs := node.rhs.Eval(timestamp, view)
	return evalScalarBinop(node.opType, lhs, rhs)
}

func (node *ScalarFunctionCall) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) clientmodel.SampleValue {
	return node.function.callFn(timestamp, view, node.args).(clientmodel.SampleValue)
}

func (node *VectorAggregation) labelsToGroupingKey(labels clientmodel.Metric) uint64 {
	summer := fnv.New64a()
	for _, label := range node.groupBy {
		fmt.Fprint(summer, labels[label])
	}

	return summer.Sum64()
}

func labelsToKey(labels clientmodel.Metric) uint64 {
	pairs := metric.LabelPairs{}

	for label, value := range labels {
		pairs = append(pairs, &metric.LabelPair{
			Name:  label,
			Value: value,
		})
	}

	sort.Sort(pairs)

	summer := fnv.New64a()

	for _, pair := range pairs {
		fmt.Fprint(summer, pair.Name, pair.Value)
	}

	return summer.Sum64()
}

func EvalVectorInstant(node VectorNode, timestamp clientmodel.Timestamp, storage *metric.TieredStorage, queryStats *stats.TimerGroup) (vector Vector, err error) {
	viewAdapter, err := viewAdapterForInstantQuery(node, timestamp, storage, queryStats)
	if err != nil {
		return
	}
	vector = node.Eval(timestamp, viewAdapter)
	return
}

func EvalVectorRange(node VectorNode, start clientmodel.Timestamp, end clientmodel.Timestamp, interval time.Duration, storage *metric.TieredStorage, queryStats *stats.TimerGroup) (Matrix, error) {
	// Explicitly initialize to an empty matrix since a nil Matrix encodes to
	// null in JSON.
	matrix := Matrix{}

	viewTimer := queryStats.GetTimer(stats.TotalViewBuildingTime).Start()
	viewAdapter, err := viewAdapterForRangeQuery(node, start, end, interval, storage, queryStats)
	viewTimer.Stop()
	if err != nil {
		return nil, err
	}

	// TODO implement watchdog timer for long-running queries.
	evalTimer := queryStats.GetTimer(stats.InnerEvalTime).Start()
	sampleSets := map[uint64]*metric.SampleSet{}
	for t := start; t.Before(end); t = t.Add(interval) {
		vector := node.Eval(t, viewAdapter)
		for _, sample := range vector {
			samplePair := &metric.SamplePair{
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			}
			groupingKey := labelsToKey(sample.Metric)
			if sampleSets[groupingKey] == nil {
				sampleSets[groupingKey] = &metric.SampleSet{
					Metric: sample.Metric,
					Values: metric.Values{samplePair},
				}
			} else {
				sampleSets[groupingKey].Values = append(sampleSets[groupingKey].Values, samplePair)
			}
		}
	}
	evalTimer.Stop()

	appendTimer := queryStats.GetTimer(stats.ResultAppendTime).Start()
	for _, sampleSet := range sampleSets {
		matrix = append(matrix, *sampleSet)
	}
	appendTimer.Stop()

	return matrix, nil
}

func labelIntersection(metric1, metric2 clientmodel.Metric) clientmodel.Metric {
	intersection := clientmodel.Metric{}
	for label, value := range metric1 {
		if metric2[label] == value {
			intersection[label] = value
		}
	}
	return intersection
}

func (node *VectorAggregation) groupedAggregationsToVector(aggregations map[uint64]*groupedAggregation, timestamp clientmodel.Timestamp) Vector {
	vector := Vector{}
	for _, aggregation := range aggregations {
		switch node.aggrType {
		case AVG:
			aggregation.value = aggregation.value / clientmodel.SampleValue(aggregation.groupCount)
		case COUNT:
			aggregation.value = clientmodel.SampleValue(aggregation.groupCount)
		default:
			// For other aggregations, we already have the right value.
		}
		sample := &clientmodel.Sample{
			Metric:    aggregation.labels,
			Value:     aggregation.value,
			Timestamp: timestamp,
		}
		vector = append(vector, sample)
	}
	return vector
}

func (node *VectorAggregation) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Vector {
	vector := node.vector.Eval(timestamp, view)
	result := map[uint64]*groupedAggregation{}
	for _, sample := range vector {
		groupingKey := node.labelsToGroupingKey(sample.Metric)
		if groupedResult, ok := result[groupingKey]; ok {
			if node.keepExtraLabels {
				groupedResult.labels = labelIntersection(groupedResult.labels, sample.Metric)
			}

			switch node.aggrType {
			case SUM:
				groupedResult.value += sample.Value
			case AVG:
				groupedResult.value += sample.Value
				groupedResult.groupCount++
			case MAX:
				if groupedResult.value < sample.Value {
					groupedResult.value = sample.Value
				}
			case MIN:
				if groupedResult.value > sample.Value {
					groupedResult.value = sample.Value
				}
			case COUNT:
				groupedResult.groupCount++
			default:
				panic("Unknown aggregation type")
			}
		} else {
			m := clientmodel.Metric{}
			if node.keepExtraLabels {
				m = sample.Metric
			} else {
				m[clientmodel.MetricNameLabel] = sample.Metric[clientmodel.MetricNameLabel]
				for _, l := range node.groupBy {
					m[l] = sample.Metric[l]
				}
			}
			result[groupingKey] = &groupedAggregation{
				labels:     m,
				value:      sample.Value,
				groupCount: 1,
			}
		}
	}

	return node.groupedAggregationsToVector(result, timestamp)
}

func (node *VectorLiteral) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Vector {
	values, err := view.GetValueAtTime(node.fingerprints, timestamp)
	if err != nil {
		glog.Error("Unable to get vector values: ", err)
		return Vector{}
	}
	return values
}

func (node *VectorFunctionCall) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Vector {
	return node.function.callFn(timestamp, view, node.args).(Vector)
}

func evalScalarBinop(opType BinOpType,
	lhs clientmodel.SampleValue,
	rhs clientmodel.SampleValue) clientmodel.SampleValue {
	switch opType {
	case ADD:
		return lhs + rhs
	case SUB:
		return lhs - rhs
	case MUL:
		return lhs * rhs
	case DIV:
		if rhs != 0 {
			return lhs / rhs
		} else {
			return clientmodel.SampleValue(math.Inf(int(rhs)))
		}
	case MOD:
		if rhs != 0 {
			return clientmodel.SampleValue(int(lhs) % int(rhs))
		} else {
			return clientmodel.SampleValue(math.Inf(int(rhs)))
		}
	case EQ:
		if lhs == rhs {
			return 1
		} else {
			return 0
		}
	case NE:
		if lhs != rhs {
			return 1
		} else {
			return 0
		}
	case GT:
		if lhs > rhs {
			return 1
		} else {
			return 0
		}
	case LT:
		if lhs < rhs {
			return 1
		} else {
			return 0
		}
	case GE:
		if lhs >= rhs {
			return 1
		} else {
			return 0
		}
	case LE:
		if lhs <= rhs {
			return 1
		} else {
			return 0
		}
	}
	panic("Not all enum values enumerated in switch")
}

func evalVectorBinop(opType BinOpType,
	lhs clientmodel.SampleValue,
	rhs clientmodel.SampleValue) (clientmodel.SampleValue, bool) {
	switch opType {
	case ADD:
		return lhs + rhs, true
	case SUB:
		return lhs - rhs, true
	case MUL:
		return lhs * rhs, true
	case DIV:
		if rhs != 0 {
			return lhs / rhs, true
		} else {
			return clientmodel.SampleValue(math.Inf(int(rhs))), true
		}
	case MOD:
		if rhs != 0 {
			return clientmodel.SampleValue(int(lhs) % int(rhs)), true
		} else {
			return clientmodel.SampleValue(math.Inf(int(rhs))), true
		}
	case EQ:
		if lhs == rhs {
			return lhs, true
		} else {
			return 0, false
		}
	case NE:
		if lhs != rhs {
			return lhs, true
		} else {
			return 0, false
		}
	case GT:
		if lhs > rhs {
			return lhs, true
		} else {
			return 0, false
		}
	case LT:
		if lhs < rhs {
			return lhs, true
		} else {
			return 0, false
		}
	case GE:
		if lhs >= rhs {
			return lhs, true
		} else {
			return 0, false
		}
	case LE:
		if lhs <= rhs {
			return lhs, true
		} else {
			return 0, false
		}
	case AND:
		return lhs, true
	case OR:
		return lhs, true // TODO: implement OR
	}
	panic("Not all enum values enumerated in switch")
}

func labelsEqual(labels1, labels2 clientmodel.Metric) bool {
	if len(labels1) != len(labels2) {
		return false
	}
	for label, value := range labels1 {
		if labels2[label] != value && label != clientmodel.MetricNameLabel {
			return false
		}
	}
	return true
}

func (node *VectorArithExpr) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Vector {
	lhs := node.lhs.Eval(timestamp, view)
	result := Vector{}
	if node.rhs.Type() == SCALAR {
		rhs := node.rhs.(ScalarNode).Eval(timestamp, view)
		for _, lhsSample := range lhs {
			value, keep := evalVectorBinop(node.opType, lhsSample.Value, rhs)
			if keep {
				lhsSample.Value = value
				result = append(result, lhsSample)
			}
		}
		return result
	} else if node.rhs.Type() == VECTOR {
		rhs := node.rhs.(VectorNode).Eval(timestamp, view)
		for _, lhsSample := range lhs {
			for _, rhsSample := range rhs {
				if labelsEqual(lhsSample.Metric, rhsSample.Metric) {
					value, keep := evalVectorBinop(node.opType, lhsSample.Value, rhsSample.Value)
					if keep {
						lhsSample.Value = value
						result = append(result, lhsSample)
					}
				}
			}
		}
		return result
	}
	panic("Invalid vector arithmetic expression operands")
}

func (node *MatrixLiteral) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix {
	interval := &metric.Interval{
		OldestInclusive: timestamp.Add(-node.interval),
		NewestInclusive: timestamp,
	}
	values, err := view.GetRangeValues(node.fingerprints, interval)
	if err != nil {
		glog.Error("Unable to get values for vector interval: ", err)
		return Matrix{}
	}
	return values
}

func (node *MatrixLiteral) EvalBoundaries(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix {
	interval := &metric.Interval{
		OldestInclusive: timestamp.Add(-node.interval),
		NewestInclusive: timestamp,
	}
	values, err := view.GetBoundaryValues(node.fingerprints, interval)
	if err != nil {
		glog.Error("Unable to get boundary values for vector interval: ", err)
		return Matrix{}
	}
	return values
}

func (matrix Matrix) Len() int {
	return len(matrix)
}

func (matrix Matrix) Less(i, j int) bool {
	return matrix[i].Metric.String() < matrix[j].Metric.String()
}

func (matrix Matrix) Swap(i, j int) {
	matrix[i], matrix[j] = matrix[j], matrix[i]
}

func (node *StringLiteral) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) string {
	return node.str
}

func (node *StringFunctionCall) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) string {
	return node.function.callFn(timestamp, view, node.args).(string)
}

// ----------------------------------------------------------------------------
// Constructors.

func NewScalarLiteral(value clientmodel.SampleValue) *ScalarLiteral {
	return &ScalarLiteral{
		value: value,
	}
}

func NewVectorLiteral(labels clientmodel.LabelSet) *VectorLiteral {
	return &VectorLiteral{
		labels: labels,
	}
}

func NewVectorAggregation(aggrType AggrType, vector VectorNode, groupBy clientmodel.LabelNames, keepExtraLabels bool) *VectorAggregation {
	return &VectorAggregation{
		aggrType:        aggrType,
		groupBy:         groupBy,
		keepExtraLabels: keepExtraLabels,
		vector:          vector,
	}
}

func NewFunctionCall(function *Function, args Nodes) (Node, error) {
	if err := function.CheckArgTypes(args); err != nil {
		return nil, err
	}
	switch function.returnType {
	case SCALAR:
		return &ScalarFunctionCall{
			function: function,
			args:     args,
		}, nil
	case VECTOR:
		return &VectorFunctionCall{
			function: function,
			args:     args,
		}, nil
	case STRING:
		return &StringFunctionCall{
			function: function,
			args:     args,
		}, nil
	}
	panic("Function with invalid return type")
}

func nodesHaveTypes(nodes Nodes, exprTypes []ExprType) bool {
	for _, node := range nodes {
		correctType := false
		for _, exprType := range exprTypes {
			if node.Type() == exprType {
				correctType = true
			}
		}
		if !correctType {
			return false
		}
	}
	return true
}

func NewArithExpr(opType BinOpType, lhs Node, rhs Node) (Node, error) {
	if !nodesHaveTypes(Nodes{lhs, rhs}, []ExprType{SCALAR, VECTOR}) {
		return nil, errors.New("Binary operands must be of vector or scalar type")
	}
	if lhs.Type() == SCALAR && rhs.Type() == VECTOR {
		return nil, errors.New("Left side of vector binary operation must be of vector type")
	}

	if opType == AND || opType == OR {
		if lhs.Type() == SCALAR || rhs.Type() == SCALAR {
			return nil, errors.New("AND and OR operators may only be used between vectors")
		}
	}

	if lhs.Type() == VECTOR || rhs.Type() == VECTOR {
		return &VectorArithExpr{
			opType: opType,
			lhs:    lhs.(VectorNode),
			rhs:    rhs,
		}, nil
	}

	return &ScalarArithExpr{
		opType: opType,
		lhs:    lhs.(ScalarNode),
		rhs:    rhs.(ScalarNode),
	}, nil
}

func NewMatrixLiteral(vector *VectorLiteral, interval time.Duration) *MatrixLiteral {
	return &MatrixLiteral{
		labels:   vector.labels,
		interval: interval,
	}
}

func NewStringLiteral(str string) *StringLiteral {
	return &StringLiteral{
		str: str,
	}
}
