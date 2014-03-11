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

// Vector is basically only an alias for clientmodel.Samples, but the
// contract is that in a Vector, all Samples have the same timestamp.
type Vector clientmodel.Samples

// Matrix is a slice of SampleSets that implements sort.Interface and
// has a String method.
// BUG(julius): Pointerize this.
type Matrix []metric.SampleSet

type groupedAggregation struct {
	labels     clientmodel.Metric
	value      clientmodel.SampleValue
	groupCount int
}

// ----------------------------------------------------------------------------
// Enums.

// ExprType is an enum for the rule language expression types.
type ExprType int

// Possible language expression types.
const (
	SCALAR ExprType = iota
	VECTOR
	MATRIX
	STRING
)

// BinOpType is an enum for binary operator types.
type BinOpType int

// Possible binary operator types.
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

// AggrType is an enum for aggregation types.
type AggrType int

// Possible aggregation types.
const (
	SUM AggrType = iota
	AVG
	MIN
	MAX
	COUNT
)

// ----------------------------------------------------------------------------
// Interfaces.

// Nodes is a slice of any mix of node types as all node types
// implement the Node interface.
type Nodes []Node

// Node is the top-level interface for any kind of nodes. Each node
// type implements one of the ...Node interfaces, each of which embeds
// this Node interface.
type Node interface {
	Type() ExprType
	Children() Nodes
	NodeTreeToDotGraph() string
	String() string
}

// ScalarNode is a Node for scalar values.
type ScalarNode interface {
	Node
	// Eval evaluates and returns the value of the scalar represented by this node.
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) clientmodel.SampleValue
}

// VectorNode is a Node for vector values.
type VectorNode interface {
	Node
	// Eval evaluates the node recursively and returns the result
	// as a Vector (i.e. a slice of Samples all at the given
	// Timestamp).
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Vector
}

// MatrixNode is a Node for matrix values.
type MatrixNode interface {
	Node
	// Eval evaluates the node recursively and returns the result as a Matrix.
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix
	// Eval evaluates the node recursively and returns the result
	// as a Matrix that only contains the boundary values.
	EvalBoundaries(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix
}

// StringNode is a Node for string values.
type StringNode interface {
	Node
	// Eval evaluates and returns the value of the string
	// represented by this node.
	Eval(timestamp clientmodel.Timestamp, view *viewAdapter) string
}

// ----------------------------------------------------------------------------
// ScalarNode types.

type (
	// ScalarLiteral represents a numeric selector.
	ScalarLiteral struct {
		value clientmodel.SampleValue
	}

	// ScalarFunctionCall represents a function with a numeric
	// return type.
	ScalarFunctionCall struct {
		function *Function
		args     Nodes
	}

	// ScalarArithExpr represents an arithmetic expression of
	// numeric type.
	ScalarArithExpr struct {
		opType BinOpType
		lhs    ScalarNode
		rhs    ScalarNode
	}
)

// ----------------------------------------------------------------------------
// VectorNode types.

type (
	// A VectorSelector represents a metric name plus labelset.
	VectorSelector struct {
		labels clientmodel.LabelSet
		// Fingerprints are populated from labels at query analysis time.
		fingerprints clientmodel.Fingerprints
	}

	// VectorFunctionCall represents a function with vector return
	// type.
	VectorFunctionCall struct {
		function *Function
		args     Nodes
	}

	// A VectorAggregation with vector return type.
	VectorAggregation struct {
		aggrType        AggrType
		groupBy         clientmodel.LabelNames
		keepExtraLabels bool
		vector          VectorNode
	}

	// VectorArithExpr represents an arithmetic expression of
	// vector type.
	VectorArithExpr struct {
		opType BinOpType
		lhs    VectorNode
		rhs    Node
	}
)

// ----------------------------------------------------------------------------
// MatrixNode types.

type (
	// A MatrixSelector represents a metric name plus labelset and
	// timerange.
	MatrixSelector struct {
		labels clientmodel.LabelSet
		// Fingerprints are populated from labels at query
		// analysis time.
		fingerprints clientmodel.Fingerprints
		interval     time.Duration
	}
)

// ----------------------------------------------------------------------------
// StringNode types.

type (
	// A StringLiteral is what you think it is.
	StringLiteral struct {
		str string
	}

	// StringFunctionCall represents a function with string return
	// type.
	StringFunctionCall struct {
		function *Function
		args     Nodes
	}
)

// ----------------------------------------------------------------------------
// Implementations.

// Type implements the Node interface.
func (node ScalarLiteral) Type() ExprType { return SCALAR }

// Type implements the Node interface.
func (node ScalarFunctionCall) Type() ExprType { return SCALAR }

// Type implements the Node interface.
func (node ScalarArithExpr) Type() ExprType { return SCALAR }

// Type implements the Node interface.
func (node VectorSelector) Type() ExprType { return VECTOR }

// Type implements the Node interface.
func (node VectorFunctionCall) Type() ExprType { return VECTOR }

// Type implements the Node interface.
func (node VectorAggregation) Type() ExprType { return VECTOR }

// Type implements the Node interface.
func (node VectorArithExpr) Type() ExprType { return VECTOR }

// Type implements the Node interface.
func (node MatrixSelector) Type() ExprType { return MATRIX }

// Type implements the Node interface.
func (node StringLiteral) Type() ExprType { return STRING }

// Type implements the Node interface.
func (node StringFunctionCall) Type() ExprType { return STRING }

// Children implements the Node interface and returns an empty slice.
func (node ScalarLiteral) Children() Nodes { return Nodes{} }

// Children implements the Node interface and returns the args of the
// function call.
func (node ScalarFunctionCall) Children() Nodes { return node.args }

// Children implements the Node interface and returns the LHS and the RHS
// of the expression.
func (node ScalarArithExpr) Children() Nodes { return Nodes{node.lhs, node.rhs} }

// Children implements the Node interface and returns an empty slice.
func (node VectorSelector) Children() Nodes { return Nodes{} }

// Children implements the Node interface and returns the args of the
// function call.
func (node VectorFunctionCall) Children() Nodes { return node.args }

// Children implements the Node interface and returns the vector to be
// aggregated.
func (node VectorAggregation) Children() Nodes { return Nodes{node.vector} }

// Children implements the Node interface and returns the LHS and the RHS
// of the expression.
func (node VectorArithExpr) Children() Nodes { return Nodes{node.lhs, node.rhs} }

// Children implements the Node interface and returns an empty slice.
func (node MatrixSelector) Children() Nodes { return Nodes{} }

// Children implements the Node interface and returns an empty slice.
func (node StringLiteral) Children() Nodes { return Nodes{} }

// Children implements the Node interface and returns the args of the
// function call.
func (node StringFunctionCall) Children() Nodes { return node.args }

// Eval implements the ScalarNode interface and returns the selector
// value.
func (node *ScalarLiteral) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) clientmodel.SampleValue {
	return node.value
}

// Eval implements the ScalarNode interface and returns the result of
// the expression.
func (node *ScalarArithExpr) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) clientmodel.SampleValue {
	lhs := node.lhs.Eval(timestamp, view)
	rhs := node.rhs.Eval(timestamp, view)
	return evalScalarBinop(node.opType, lhs, rhs)
}

// Eval implements the ScalarNode interface and returns the result of
// the function call.
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

// EvalVectorInstant evaluates a VectorNode with an instant query.
func EvalVectorInstant(node VectorNode, timestamp clientmodel.Timestamp, storage *metric.TieredStorage, queryStats *stats.TimerGroup) (vector Vector, err error) {
	viewAdapter, err := viewAdapterForInstantQuery(node, timestamp, storage, queryStats)
	if err != nil {
		return
	}
	vector = node.Eval(timestamp, viewAdapter)
	return
}

// EvalVectorRange evaluates a VectorNode with a range query.
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
			samplePair := metric.SamplePair{
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

// Eval implements the VectorNode interface and returns the aggregated
// Vector.
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
					if v, ok := sample.Metric[l]; ok {
						m[l] = v
					}
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

// Eval implements the VectorNode interface and returns the value of
// the selector.
func (node *VectorSelector) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Vector {
	values, err := view.GetValueAtTime(node.fingerprints, timestamp)
	if err != nil {
		glog.Error("Unable to get vector values: ", err)
		return Vector{}
	}
	return values
}

// Eval implements the VectorNode interface and returns the result of
// the function call.
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
		}
		return clientmodel.SampleValue(math.Inf(int(rhs)))
	case MOD:
		if rhs != 0 {
			return clientmodel.SampleValue(int(lhs) % int(rhs))
		}
		return clientmodel.SampleValue(math.Inf(int(rhs)))
	case EQ:
		if lhs == rhs {
			return 1
		}
		return 0
	case NE:
		if lhs != rhs {
			return 1
		}
		return 0
	case GT:
		if lhs > rhs {
			return 1
		}
		return 0
	case LT:
		if lhs < rhs {
			return 1
		}
		return 0
	case GE:
		if lhs >= rhs {
			return 1
		}
		return 0
	case LE:
		if lhs <= rhs {
			return 1
		}
		return 0
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
		}
		return clientmodel.SampleValue(math.Inf(int(rhs))), true
	case MOD:
		if rhs != 0 {
			return clientmodel.SampleValue(int(lhs) % int(rhs)), true
		}
		return clientmodel.SampleValue(math.Inf(int(rhs))), true
	case EQ:
		if lhs == rhs {
			return lhs, true
		}
		return 0, false
	case NE:
		if lhs != rhs {
			return lhs, true
		}
		return 0, false
	case GT:
		if lhs > rhs {
			return lhs, true
		}
		return 0, false
	case LT:
		if lhs < rhs {
			return lhs, true
		}
		return 0, false
	case GE:
		if lhs >= rhs {
			return lhs, true
		}
		return 0, false
	case LE:
		if lhs <= rhs {
			return lhs, true
		}
		return 0, false
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

// Eval implements the VectorNode interface and returns the result of
// the expression.
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

// Eval implements the MatrixNode interface and returns the value of
// the selector.
func (node *MatrixSelector) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix {
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

// EvalBoundaries implements the MatrixNode interface and returns the
// boundary values of the selector.
func (node *MatrixSelector) EvalBoundaries(timestamp clientmodel.Timestamp, view *viewAdapter) Matrix {
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

// Len implements sort.Interface.
func (matrix Matrix) Len() int {
	return len(matrix)
}

// Less implements sort.Interface.
func (matrix Matrix) Less(i, j int) bool {
	return matrix[i].Metric.String() < matrix[j].Metric.String()
}

// Swap implements sort.Interface.
func (matrix Matrix) Swap(i, j int) {
	matrix[i], matrix[j] = matrix[j], matrix[i]
}

// Eval implements the StringNode interface and returns the value of
// the selector.
func (node *StringLiteral) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) string {
	return node.str
}

// Eval implements the StringNode interface and returns the result of
// the function call.
func (node *StringFunctionCall) Eval(timestamp clientmodel.Timestamp, view *viewAdapter) string {
	return node.function.callFn(timestamp, view, node.args).(string)
}

// ----------------------------------------------------------------------------
// Constructors.

// NewScalarLiteral returns a ScalarLiteral with the given value.
func NewScalarLiteral(value clientmodel.SampleValue) *ScalarLiteral {
	return &ScalarLiteral{
		value: value,
	}
}

// NewVectorSelector returns a (not yet evaluated) VectorSelector with
// the given LabelSet.
func NewVectorSelector(labels clientmodel.LabelSet) *VectorSelector {
	return &VectorSelector{
		labels: labels,
	}
}

// NewVectorAggregation returns a (not yet evaluated)
// VectorAggregation, aggregating the given VectorNode using the given
// AggrType, grouping by the given LabelNames.
func NewVectorAggregation(aggrType AggrType, vector VectorNode, groupBy clientmodel.LabelNames, keepExtraLabels bool) *VectorAggregation {
	return &VectorAggregation{
		aggrType:        aggrType,
		groupBy:         groupBy,
		keepExtraLabels: keepExtraLabels,
		vector:          vector,
	}
}

// NewFunctionCall returns a (not yet evaluated) function call node
// (of type ScalarFunctionCall, VectorFunctionCall, or
// StringFunctionCall).
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

// NewArithExpr returns a (not yet evaluated) expression node (of type
// VectorArithExpr or ScalarArithExpr).
func NewArithExpr(opType BinOpType, lhs Node, rhs Node) (Node, error) {
	if !nodesHaveTypes(Nodes{lhs, rhs}, []ExprType{SCALAR, VECTOR}) {
		return nil, errors.New("binary operands must be of vector or scalar type")
	}
	if lhs.Type() == SCALAR && rhs.Type() == VECTOR {
		return nil, errors.New("left side of vector binary operation must be of vector type")
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

// NewMatrixSelector returns a (not yet evaluated) MatrixSelector with
// the given VectorSelector and Duration.
func NewMatrixSelector(vector *VectorSelector, interval time.Duration) *MatrixSelector {
	return &MatrixSelector{
		labels:   vector.labels,
		interval: interval,
	}
}

// NewStringLiteral returns a StringLiteral with the given string as
// value.
func NewStringLiteral(str string) *StringLiteral {
	return &StringLiteral{
		str: str,
	}
}
