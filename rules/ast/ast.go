package ast

import (
	"errors"
	"fmt"
	"github.com/matttproud/prometheus/model"
	"log"
	"math"
	"sort"
	"strings"
	"time"
)

// ----------------------------------------------------------------------------
// Raw data value types.

type Vector []*model.Sample
type Matrix []*model.SampleSet

type groupedAggregation struct {
	labels     model.Metric
	value      model.SampleValue
	groupCount int
}

type labelValuePair struct {
	label model.LabelName
	value model.LabelValue
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
)

// ----------------------------------------------------------------------------
// Interfaces.

// All node interfaces include the Node interface.
type Node interface {
	Type() ExprType
	NodeTreeToDotGraph() string
}

// All node types implement one of the following interfaces. The name of the
// interface represents the type returned to the parent node.
type ScalarNode interface {
	Node
	Eval(timestamp *time.Time) model.SampleValue
}

type VectorNode interface {
	Node
	Eval(timestamp *time.Time) Vector
}

type MatrixNode interface {
	Node
	Eval(timestamp *time.Time) Matrix
	EvalBoundaries(timestamp *time.Time) Matrix
}

type StringNode interface {
	Node
	Eval(timestamp *time.Time) string
}

// ----------------------------------------------------------------------------
// ScalarNode types.

type (
	// A numeric literal.
	ScalarLiteral struct {
		value model.SampleValue
	}

	// A function of numeric return type.
	ScalarFunctionCall struct {
		function *Function
		args     []Node
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
		labels model.LabelSet
	}

	// A function of vector return type.
	VectorFunctionCall struct {
		function *Function
		args     []Node
	}

	// A vector aggregation with vector return type.
	VectorAggregation struct {
		aggrType AggrType
		groupBy  []model.LabelName
		vector   VectorNode
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
		labels   model.LabelSet
		interval time.Duration
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
		args     []Node
	}
)

// ----------------------------------------------------------------------------
// Implementations.

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

func (node *ScalarLiteral) Eval(timestamp *time.Time) model.SampleValue {
	return node.value
}

func (node *ScalarArithExpr) Eval(timestamp *time.Time) model.SampleValue {
	lhs := node.lhs.Eval(timestamp)
	rhs := node.rhs.Eval(timestamp)
	return evalScalarBinop(node.opType, lhs, rhs)
}

func (node *ScalarFunctionCall) Eval(timestamp *time.Time) model.SampleValue {
	return node.function.callFn(timestamp, node.args).(model.SampleValue)
}

func (node *VectorAggregation) labelsToGroupingKey(labels model.Metric) string {
	keyParts := []string{}
	for _, keyLabel := range node.groupBy {
		keyParts = append(keyParts, string(labels[keyLabel]))
	}
	return strings.Join(keyParts, ",") // TODO not safe when label value contains comma.
}

func labelsToKey(labels model.Metric) string {
	keyParts := []string{}
	for label, value := range labels {
		keyParts = append(keyParts, fmt.Sprintf("%v='%v'", label, value))
	}
	sort.Strings(keyParts)
	return strings.Join(keyParts, ",") // TODO not safe when label value contains comma.
}

func EvalVectorRange(node VectorNode, start time.Time, end time.Time, step time.Duration) Matrix {
	// TODO implement watchdog timer for long-running queries.
	sampleSets := map[string]*model.SampleSet{}
	for t := start; t.Before(end); t = t.Add(step) {
		vector := node.Eval(&t)
		for _, sample := range vector {
			samplePair := model.SamplePair{
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			}
			groupingKey := labelsToKey(sample.Metric)
			if sampleSets[groupingKey] == nil {
				sampleSets[groupingKey] = &model.SampleSet{
					Metric: sample.Metric,
					Values: []model.SamplePair{samplePair},
				}
			} else {
				sampleSets[groupingKey].Values = append(sampleSets[groupingKey].Values, samplePair)
			}
		}
	}

	matrix := Matrix{}
	for _, sampleSet := range sampleSets {
		matrix = append(matrix, sampleSet)
	}
	return matrix
}

func labelIntersection(metric1, metric2 model.Metric) model.Metric {
	intersection := model.Metric{}
	for label, value := range metric1 {
		if metric2[label] == value {
			intersection[label] = value
		}
	}
	return intersection
}

func (node *VectorAggregation) groupedAggregationsToVector(aggregations map[string]*groupedAggregation, timestamp *time.Time) Vector {
	vector := Vector{}
	for _, aggregation := range aggregations {
		if node.aggrType == AVG {
			aggregation.value = aggregation.value / model.SampleValue(aggregation.groupCount)
		}
		sample := &model.Sample{
			Metric:    aggregation.labels,
			Value:     aggregation.value,
			Timestamp: *timestamp,
		}
		vector = append(vector, sample)
	}
	return vector
}

func (node *VectorAggregation) Eval(timestamp *time.Time) Vector {
	vector := node.vector.Eval(timestamp)
	result := map[string]*groupedAggregation{}
	for _, sample := range vector {
		groupingKey := node.labelsToGroupingKey(sample.Metric)
		if groupedResult, ok := result[groupingKey]; ok {
			groupedResult.labels = labelIntersection(groupedResult.labels, sample.Metric)
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
			}
		} else {
			result[groupingKey] = &groupedAggregation{
				labels:     sample.Metric,
				value:      sample.Value,
				groupCount: 1,
			}
		}
	}
	return node.groupedAggregationsToVector(result, timestamp)
}

func (node *VectorLiteral) Eval(timestamp *time.Time) Vector {
	values, err := persistence.GetValueAtTime(node.labels, timestamp, &stalenessPolicy)
	if err != nil {
		log.Printf("Unable to get vector values")
		return Vector{}
	}
	return values
}

func (node *VectorFunctionCall) Eval(timestamp *time.Time) Vector {
	return node.function.callFn(timestamp, node.args).(Vector)
}

func evalScalarBinop(opType BinOpType,
	lhs model.SampleValue,
	rhs model.SampleValue) model.SampleValue {
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
			return model.SampleValue(math.Inf(int(rhs)))
		}
	case MOD:
		if rhs != 0 {
			return model.SampleValue(int(lhs) % int(rhs))
		} else {
			return model.SampleValue(math.Inf(int(rhs)))
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
	lhs model.SampleValue,
	rhs model.SampleValue) (model.SampleValue, bool) {
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
			return model.SampleValue(math.Inf(int(rhs))), true
		}
	case MOD:
		if rhs != 0 {
			return model.SampleValue(int(lhs) % int(rhs)), true
		} else {
			return model.SampleValue(math.Inf(int(rhs))), true
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

func labelsEqual(labels1, labels2 model.Metric) bool {
	if len(labels1) != len(labels2) {
		return false
	}
	for label, value := range labels1 {
		if labels2[label] != value && label != "name" {
			return false
		}
	}
	return true
}

func (node *VectorArithExpr) Eval(timestamp *time.Time) Vector {
	lhs := node.lhs.Eval(timestamp)
	result := Vector{}
	if node.rhs.Type() == SCALAR {
		rhs := node.rhs.(ScalarNode).Eval(timestamp)
		for _, lhsSample := range lhs {
			value, keep := evalVectorBinop(node.opType, lhsSample.Value, rhs)
			if keep {
				lhsSample.Value = value
				result = append(result, lhsSample)
			}
		}
		return result
	} else if node.rhs.Type() == VECTOR {
		rhs := node.rhs.(VectorNode).Eval(timestamp)
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

func (node *MatrixLiteral) Eval(timestamp *time.Time) Matrix {
	interval := &model.Interval{
		OldestInclusive: timestamp.Add(-node.interval),
		NewestInclusive: *timestamp,
	}
	values, err := persistence.GetRangeValues(node.labels, interval, &stalenessPolicy)
	if err != nil {
		log.Printf("Unable to get values for vector interval")
		return Matrix{}
	}
	return values
}

func (node *MatrixLiteral) EvalBoundaries(timestamp *time.Time) Matrix {
	interval := &model.Interval{
		OldestInclusive: timestamp.Add(-node.interval),
		NewestInclusive: *timestamp,
	}
	values, err := persistence.GetBoundaryValues(node.labels, interval, &stalenessPolicy)
	if err != nil {
		log.Printf("Unable to get boundary values for vector interval")
		return Matrix{}
	}
	return values
}

func (matrix Matrix) Len() int {
	return len(matrix)
}

func (matrix Matrix) Less(i, j int) bool {
	return labelsToKey(matrix[i].Metric) < labelsToKey(matrix[j].Metric)
}

func (matrix Matrix) Swap(i, j int) {
	matrix[i], matrix[j] = matrix[j], matrix[i]
}

func (node *StringLiteral) Eval(timestamp *time.Time) string {
	return node.str
}

func (node *StringFunctionCall) Eval(timestamp *time.Time) string {
	return node.function.callFn(timestamp, node.args).(string)
}

// ----------------------------------------------------------------------------
// Constructors.

func NewScalarLiteral(value model.SampleValue) *ScalarLiteral {
	return &ScalarLiteral{
		value: value,
	}
}

func NewVectorLiteral(labels model.LabelSet) *VectorLiteral {
	return &VectorLiteral{
		labels: labels,
	}
}

func NewVectorAggregation(aggrType AggrType, vector VectorNode, groupBy []model.LabelName) *VectorAggregation {
	return &VectorAggregation{
		aggrType: aggrType,
		groupBy:  groupBy,
		vector:   vector,
	}
}

func NewFunctionCall(function *Function, args []Node) (Node, error) {
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

func nodesHaveTypes(nodes []Node, exprTypes []ExprType) bool {
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
	if !nodesHaveTypes([]Node{lhs, rhs}, []ExprType{SCALAR, VECTOR}) {
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
