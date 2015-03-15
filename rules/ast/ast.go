// Copyright 2013 The Prometheus Authors
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
	"flag"
	"fmt"
	"math"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
)

var (
	stalenessDelta = flag.Duration("query.staleness-delta", 300*time.Second, "Staleness delta allowance during expression evaluations.")
	queryTimeout   = flag.Duration("query.timeout", 2*time.Minute, "Maximum time a query may take before being aborted.")
)

type queryTimeoutError struct {
	timeoutAfter time.Duration
}

func (e queryTimeoutError) Error() string {
	return fmt.Sprintf("query timeout after %v", e.timeoutAfter)
}

// ----------------------------------------------------------------------------
// Raw data value types.

// SampleStream is a stream of Values belonging to an attached COWMetric.
type SampleStream struct {
	Metric clientmodel.COWMetric `json:"metric"`
	Values metric.Values         `json:"values"`
}

// Sample is a single sample belonging to a COWMetric.
type Sample struct {
	Metric    clientmodel.COWMetric   `json:"metric"`
	Value     clientmodel.SampleValue `json:"value"`
	Timestamp clientmodel.Timestamp   `json:"timestamp"`
}

// Vector is basically only an alias for clientmodel.Samples, but the
// contract is that in a Vector, all Samples have the same timestamp.
type Vector []*Sample

// Matrix is a slice of SampleStreams that implements sort.Interface and
// has a String method.
// BUG(julius): Pointerize this.
type Matrix []SampleStream

type groupedAggregation struct {
	labels     clientmodel.COWMetric
	value      clientmodel.SampleValue
	groupCount int
}

// ----------------------------------------------------------------------------
// Enums.

// ExprType is an enum for the rule language expression types.
type ExprType int

// Possible language expression types. We define these as integer constants
// because sometimes we need to pass around just the type without an object of
// that type.
const (
	ScalarType ExprType = iota
	VectorType
	MatrixType
	StringType
)

// BinOpType is an enum for binary operator types.
type BinOpType int

// Possible binary operator types.
const (
	Add BinOpType = iota
	Sub
	Mul
	Div
	Mod
	NE
	EQ
	GT
	LT
	GE
	LE
	And
	Or
)

// shouldDropMetric indicates whether the metric name should be dropped after
// applying this operator to a vector.
func (opType BinOpType) shouldDropMetric() bool {
	switch opType {
	case Add, Sub, Mul, Div, Mod:
		return true
	default:
		return false
	}
}

// AggrType is an enum for aggregation types.
type AggrType int

// Possible aggregation types.
const (
	Sum AggrType = iota
	Avg
	Min
	Max
	Count
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
	Eval(timestamp clientmodel.Timestamp) clientmodel.SampleValue
}

// VectorNode is a Node for vector values.
type VectorNode interface {
	Node
	// Eval evaluates the node recursively and returns the result
	// as a Vector (i.e. a slice of Samples all at the given
	// Timestamp).
	Eval(timestamp clientmodel.Timestamp) Vector
}

// MatrixNode is a Node for matrix values.
type MatrixNode interface {
	Node
	// Eval evaluates the node recursively and returns the result as a Matrix.
	Eval(timestamp clientmodel.Timestamp) Matrix
	// Eval evaluates the node recursively and returns the result
	// as a Matrix that only contains the boundary values.
	EvalBoundaries(timestamp clientmodel.Timestamp) Matrix
}

// StringNode is a Node for string values.
type StringNode interface {
	Node
	// Eval evaluates and returns the value of the string
	// represented by this node.
	Eval(timestamp clientmodel.Timestamp) string
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
		labelMatchers metric.LabelMatchers
		offset        time.Duration
		// The series iterators are populated at query analysis time.
		iterators map[clientmodel.Fingerprint]local.SeriesIterator
		metrics   map[clientmodel.Fingerprint]clientmodel.COWMetric
		// Fingerprints are populated from label matchers at query analysis time.
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

	// VectorArithExpr represents an arithmetic expression of vector type. At
	// least one of the two operand Nodes must be a VectorNode. The other may be
	// a VectorNode or ScalarNode. Both criteria are checked at runtime.
	VectorArithExpr struct {
		opType           BinOpType
		lhs              Node
		rhs              Node
		matchCardinality VectorMatchCardinality
		matchOn          clientmodel.LabelNames
		includeLabels    clientmodel.LabelNames
	}
)

// VectorMatchCardinality is an enum describing vector matches (1:1, n:1, 1:n, n:m).
type VectorMatchCardinality int

// Constants for VectorMatchCardinality enum.
const (
	MatchOneToOne VectorMatchCardinality = iota
	MatchManyToOne
	MatchOneToMany
	MatchManyToMany
)

// ----------------------------------------------------------------------------
// MatrixNode types.

type (
	// A MatrixSelector represents a metric name plus labelset and
	// timerange.
	MatrixSelector struct {
		labelMatchers metric.LabelMatchers
		// The series iterators are populated at query analysis time.
		iterators map[clientmodel.Fingerprint]local.SeriesIterator
		metrics   map[clientmodel.Fingerprint]clientmodel.COWMetric
		// Fingerprints are populated from label matchers at query analysis time.
		fingerprints clientmodel.Fingerprints
		interval     time.Duration
		offset       time.Duration
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
func (node ScalarLiteral) Type() ExprType { return ScalarType }

// Type implements the Node interface.
func (node ScalarFunctionCall) Type() ExprType { return ScalarType }

// Type implements the Node interface.
func (node ScalarArithExpr) Type() ExprType { return ScalarType }

// Type implements the Node interface.
func (node VectorSelector) Type() ExprType { return VectorType }

// Type implements the Node interface.
func (node VectorFunctionCall) Type() ExprType { return VectorType }

// Type implements the Node interface.
func (node VectorAggregation) Type() ExprType { return VectorType }

// Type implements the Node interface.
func (node VectorArithExpr) Type() ExprType { return VectorType }

// Type implements the Node interface.
func (node MatrixSelector) Type() ExprType { return MatrixType }

// Type implements the Node interface.
func (node StringLiteral) Type() ExprType { return StringType }

// Type implements the Node interface.
func (node StringFunctionCall) Type() ExprType { return StringType }

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
func (node *ScalarLiteral) Eval(timestamp clientmodel.Timestamp) clientmodel.SampleValue {
	return node.value
}

// Eval implements the ScalarNode interface and returns the result of
// the expression.
func (node *ScalarArithExpr) Eval(timestamp clientmodel.Timestamp) clientmodel.SampleValue {
	lhs := node.lhs.Eval(timestamp)
	rhs := node.rhs.Eval(timestamp)
	return evalScalarBinop(node.opType, lhs, rhs)
}

// Eval implements the ScalarNode interface and returns the result of
// the function call.
func (node *ScalarFunctionCall) Eval(timestamp clientmodel.Timestamp) clientmodel.SampleValue {
	return node.function.callFn(timestamp, node.args).(clientmodel.SampleValue)
}

// EvalVectorInstant evaluates a VectorNode with an instant query.
func EvalVectorInstant(node VectorNode, timestamp clientmodel.Timestamp, storage local.Storage, queryStats *stats.TimerGroup) (Vector, error) {
	totalEvalTimer := queryStats.GetTimer(stats.TotalEvalTime).Start()
	defer totalEvalTimer.Stop()

	closer, err := prepareInstantQuery(node, timestamp, storage, queryStats)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	if et := totalEvalTimer.ElapsedTime(); et > *queryTimeout {
		return nil, queryTimeoutError{et}
	}
	return node.Eval(timestamp), nil
}

// EvalVectorRange evaluates a VectorNode with a range query.
func EvalVectorRange(node VectorNode, start clientmodel.Timestamp, end clientmodel.Timestamp, interval time.Duration, storage local.Storage, queryStats *stats.TimerGroup) (Matrix, error) {
	totalEvalTimer := queryStats.GetTimer(stats.TotalEvalTime).Start()
	defer totalEvalTimer.Stop()
	// Explicitly initialize to an empty matrix since a nil Matrix encodes to
	// null in JSON.
	matrix := Matrix{}

	prepareTimer := queryStats.GetTimer(stats.TotalQueryPreparationTime).Start()
	closer, err := prepareRangeQuery(node, start, end, interval, storage, queryStats)
	prepareTimer.Stop()
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	evalTimer := queryStats.GetTimer(stats.InnerEvalTime).Start()
	sampleStreams := map[clientmodel.Fingerprint]*SampleStream{}
	for t := start; !t.After(end); t = t.Add(interval) {
		if et := totalEvalTimer.ElapsedTime(); et > *queryTimeout {
			evalTimer.Stop()
			return nil, queryTimeoutError{et}
		}
		vector := node.Eval(t)
		for _, sample := range vector {
			samplePair := metric.SamplePair{
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			}
			fp := sample.Metric.Metric.Fingerprint()
			if sampleStreams[fp] == nil {
				sampleStreams[fp] = &SampleStream{
					Metric: sample.Metric,
					Values: metric.Values{samplePair},
				}
			} else {
				sampleStreams[fp].Values = append(sampleStreams[fp].Values, samplePair)
			}
		}
	}
	evalTimer.Stop()

	appendTimer := queryStats.GetTimer(stats.ResultAppendTime).Start()
	for _, sampleStream := range sampleStreams {
		matrix = append(matrix, *sampleStream)
	}
	appendTimer.Stop()

	return matrix, nil
}

func labelIntersection(metric1, metric2 clientmodel.COWMetric) clientmodel.COWMetric {
	for label, value := range metric1.Metric {
		if metric2.Metric[label] != value {
			metric1.Delete(label)
		}
	}
	return metric1
}

func (node *VectorAggregation) groupedAggregationsToVector(aggregations map[uint64]*groupedAggregation, timestamp clientmodel.Timestamp) Vector {
	vector := Vector{}
	for _, aggregation := range aggregations {
		switch node.aggrType {
		case Avg:
			aggregation.value = aggregation.value / clientmodel.SampleValue(aggregation.groupCount)
		case Count:
			aggregation.value = clientmodel.SampleValue(aggregation.groupCount)
		default:
			// For other aggregations, we already have the right value.
		}
		sample := &Sample{
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
func (node *VectorAggregation) Eval(timestamp clientmodel.Timestamp) Vector {
	vector := node.vector.Eval(timestamp)
	result := map[uint64]*groupedAggregation{}
	for _, sample := range vector {
		groupingKey := clientmodel.SignatureForLabels(sample.Metric.Metric, node.groupBy)
		if groupedResult, ok := result[groupingKey]; ok {
			if node.keepExtraLabels {
				groupedResult.labels = labelIntersection(groupedResult.labels, sample.Metric)
			}

			switch node.aggrType {
			case Sum:
				groupedResult.value += sample.Value
			case Avg:
				groupedResult.value += sample.Value
				groupedResult.groupCount++
			case Max:
				if groupedResult.value < sample.Value {
					groupedResult.value = sample.Value
				}
			case Min:
				if groupedResult.value > sample.Value {
					groupedResult.value = sample.Value
				}
			case Count:
				groupedResult.groupCount++
			default:
				panic("Unknown aggregation type")
			}
		} else {
			var m clientmodel.COWMetric
			if node.keepExtraLabels {
				m = sample.Metric
				m.Delete(clientmodel.MetricNameLabel)
			} else {
				m = clientmodel.COWMetric{
					Metric: clientmodel.Metric{},
					Copied: true,
				}
				for _, l := range node.groupBy {
					if v, ok := sample.Metric.Metric[l]; ok {
						m.Set(l, v)
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
func (node *VectorSelector) Eval(timestamp clientmodel.Timestamp) Vector {
	//// timer := v.stats.GetTimer(stats.GetValueAtTimeTime).Start()
	samples := Vector{}
	for fp, it := range node.iterators {
		sampleCandidates := it.GetValueAtTime(timestamp.Add(-node.offset))
		samplePair := chooseClosestSample(sampleCandidates, timestamp.Add(-node.offset))
		if samplePair != nil {
			samples = append(samples, &Sample{
				Metric:    node.metrics[fp],
				Value:     samplePair.Value,
				Timestamp: timestamp,
			})
		}
	}
	//// timer.Stop()
	return samples
}

// chooseClosestSample chooses the closest sample of a list of samples
// surrounding a given target time. If samples are found both before and after
// the target time, the sample value is interpolated between these. Otherwise,
// the single closest sample is returned verbatim.
func chooseClosestSample(samples metric.Values, timestamp clientmodel.Timestamp) *metric.SamplePair {
	var closestBefore *metric.SamplePair
	var closestAfter *metric.SamplePair
	for _, candidate := range samples {
		delta := candidate.Timestamp.Sub(timestamp)
		// Samples before target time.
		if delta < 0 {
			// Ignore samples outside of staleness policy window.
			if -delta > *stalenessDelta {
				continue
			}
			// Ignore samples that are farther away than what we've seen before.
			if closestBefore != nil && candidate.Timestamp.Before(closestBefore.Timestamp) {
				continue
			}
			sample := candidate
			closestBefore = &sample
		}

		// Samples after target time.
		if delta >= 0 {
			// Ignore samples outside of staleness policy window.
			if delta > *stalenessDelta {
				continue
			}
			// Ignore samples that are farther away than samples we've seen before.
			if closestAfter != nil && candidate.Timestamp.After(closestAfter.Timestamp) {
				continue
			}
			sample := candidate
			closestAfter = &sample
		}
	}

	switch {
	case closestBefore != nil && closestAfter != nil:
		return interpolateSamples(closestBefore, closestAfter, timestamp)
	case closestBefore != nil:
		return closestBefore
	default:
		return closestAfter
	}
}

// interpolateSamples interpolates a value at a target time between two
// provided sample pairs.
func interpolateSamples(first, second *metric.SamplePair, timestamp clientmodel.Timestamp) *metric.SamplePair {
	dv := second.Value - first.Value
	dt := second.Timestamp.Sub(first.Timestamp)

	dDt := dv / clientmodel.SampleValue(dt)
	offset := clientmodel.SampleValue(timestamp.Sub(first.Timestamp))

	return &metric.SamplePair{
		Value:     first.Value + (offset * dDt),
		Timestamp: timestamp,
	}
}

// Eval implements the VectorNode interface and returns the result of
// the function call.
func (node *VectorFunctionCall) Eval(timestamp clientmodel.Timestamp) Vector {
	return node.function.callFn(timestamp, node.args).(Vector)
}

func evalScalarBinop(opType BinOpType,
	lhs clientmodel.SampleValue,
	rhs clientmodel.SampleValue) clientmodel.SampleValue {
	switch opType {
	case Add:
		return lhs + rhs
	case Sub:
		return lhs - rhs
	case Mul:
		return lhs * rhs
	case Div:
		if rhs != 0 {
			return lhs / rhs
		}
		return clientmodel.SampleValue(math.Inf(int(rhs)))
	case Mod:
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
	case Add:
		return lhs + rhs, true
	case Sub:
		return lhs - rhs, true
	case Mul:
		return lhs * rhs, true
	case Div:
		if rhs != 0 {
			return lhs / rhs, true
		}
		return clientmodel.SampleValue(math.Inf(int(rhs))), true
	case Mod:
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
	}
	panic("Not all enum values enumerated in switch")
}

func labelsEqual(labels1, labels2 clientmodel.Metric) bool {
	for label, value := range labels1 {
		if labels2[label] != value && label != clientmodel.MetricNameLabel {
			return false
		}
	}
	return true
}

// Eval implements the VectorNode interface and returns the result of
// the expression.
func (node *VectorArithExpr) Eval(timestamp clientmodel.Timestamp) Vector {
	// Calculate vector-to-vector operation.
	if node.lhs.Type() == VectorType && node.rhs.Type() == VectorType {
		lhs := node.lhs.(VectorNode).Eval(timestamp)
		rhs := node.rhs.(VectorNode).Eval(timestamp)

		return node.evalVectors(timestamp, lhs, rhs)
	}

	// Calculate vector-to-scalar operation.
	var lhs Vector
	var rhs clientmodel.SampleValue
	swap := false

	if node.lhs.Type() == ScalarType && node.rhs.Type() == VectorType {
		lhs = node.rhs.(VectorNode).Eval(timestamp)
		rhs = node.lhs.(ScalarNode).Eval(timestamp)
		swap = true
	} else {
		lhs = node.lhs.(VectorNode).Eval(timestamp)
		rhs = node.rhs.(ScalarNode).Eval(timestamp)
	}

	result := make(Vector, 0, len(lhs))

	for _, lhsSample := range lhs {
		lv, rv := lhsSample.Value, rhs
		// lhs always contains the vector. If the original position was different
		// swap for calculating the value.
		if swap {
			lv, rv = rv, lv
		}
		value, keep := evalVectorBinop(node.opType, lv, rv)
		if keep {
			lhsSample.Value = value
			if node.opType.shouldDropMetric() {
				lhsSample.Metric.Delete(clientmodel.MetricNameLabel)
			}
			result = append(result, lhsSample)
		}
	}
	return result
}

// evalVectors evaluates the binary operation for the given vectors.
func (node *VectorArithExpr) evalVectors(timestamp clientmodel.Timestamp, lhs, rhs Vector) Vector {
	result := make(Vector, 0, len(rhs))
	// The control flow below handles one-to-one or many-to-one matching.
	// For one-to-many, swap sidedness and account for the swap when calculating
	// values.
	if node.matchCardinality == MatchOneToMany {
		lhs, rhs = rhs, lhs
	}
	// All samples from the rhs hashed by the matching label/values.
	rm := make(map[uint64]*Sample)
	// Maps the hash of the label values used for matching to the hashes of the label
	// values of the include labels (if any). It is used to keep track of already
	// inserted samples.
	added := make(map[uint64][]uint64)

	// Add all rhs samples to a map so we can easily find matches later.
	for _, rs := range rhs {
		hash := node.hashForMetric(rs.Metric.Metric)
		// The rhs is guaranteed to be the 'one' side. Having multiple samples
		// with the same hash means that the matching is many-to-many,
		// which is not supported.
		if _, found := rm[hash]; node.matchCardinality != MatchManyToMany && found {
			// Many-to-many matching not allowed.
			// TODO(fabxc): Return a query error here once AST nodes support that.
			return Vector{}
		}
		// In many-to-many matching the entry is simply overwritten. It can thus only
		// be used to check whether any matching rhs entry exists but not retrieve them all.
		rm[hash] = rs
	}

	// For all lhs samples find a respective rhs sample and perform
	// the binary operation.
	for _, ls := range lhs {
		hash := node.hashForMetric(ls.Metric.Metric)
		// Any lhs sample we encounter in an OR operation belongs to the result.
		if node.opType == Or {
			ls.Metric = node.resultMetric(ls, nil)
			result = append(result, ls)
			added[hash] = nil // Ensure matching rhs sample is not added later.
			continue
		}

		rs, found := rm[hash] // Look for a match in the rhs vector.
		if !found {
			continue
		}
		var value clientmodel.SampleValue
		var keep bool

		if node.opType == And {
			value = ls.Value
			keep = true
		} else {
			if _, exists := added[hash]; node.matchCardinality == MatchOneToOne && exists {
				// Many-to-one matching must be explicit.
				// TODO(fabxc): Return a query error here once AST nodes support that.
				return Vector{}
			}
			// Account for potentially swapped sidedness.
			vl, vr := ls.Value, rs.Value
			if node.matchCardinality == MatchOneToMany {
				vl, vr = vr, vl
			}
			value, keep = evalVectorBinop(node.opType, vl, vr)
		}

		if keep {
			metric := node.resultMetric(ls, rs)
			// Check if the same label set has been added for a many-to-one matching before.
			if node.matchCardinality == MatchManyToOne || node.matchCardinality == MatchOneToMany {
				insHash := clientmodel.SignatureForLabels(metric.Metric, node.includeLabels)
				if ihs, exists := added[hash]; exists {
					for _, ih := range ihs {
						if ih == insHash {
							// TODO(fabxc): Return a query error here once AST nodes support that.
							return Vector{}
						}
					}
					added[hash] = append(ihs, insHash)
				} else {
					added[hash] = []uint64{insHash}
				}
			}
			ns := &Sample{
				Metric:    metric,
				Value:     value,
				Timestamp: timestamp,
			}
			result = append(result, ns)
			if _, ok := added[hash]; !ok {
				added[hash] = nil // Set existence to true.
			}
		}
	}

	// Add all remaining samples in the rhs in an OR operation if they
	// have not been matched up with a lhs sample.
	if node.opType == Or {
		for hash, rs := range rm {
			if _, exists := added[hash]; !exists {
				rs.Metric = node.resultMetric(rs, nil)
				result = append(result, rs)
			}
		}
	}
	return result
}

// resultMetric returns the metric for the given sample(s) based on the vector
// binary operation and the matching options. If a label that has to be included is set on
// both sides an error is returned.
func (node *VectorArithExpr) resultMetric(ls, rs *Sample) clientmodel.COWMetric {
	if len(node.matchOn) == 0 || node.opType == Or || node.opType == And {
		if node.opType.shouldDropMetric() {
			ls.Metric.Delete(clientmodel.MetricNameLabel)
		}
		return ls.Metric
	}

	m := clientmodel.Metric{}
	for _, ln := range node.matchOn {
		m[ln] = ls.Metric.Metric[ln]
	}

	for _, ln := range node.includeLabels {
		// Included labels from the `group_x` modifier are taken from the "many"-side.
		v, ok := ls.Metric.Metric[ln]
		if ok {
			m[ln] = v
		}
	}
	return clientmodel.COWMetric{false, m}
}

// hashForMetric calculates a hash value for the given metric based on the matching
// options for the binary operation.
func (node *VectorArithExpr) hashForMetric(metric clientmodel.Metric) uint64 {
	var labels clientmodel.LabelNames

	if len(node.matchOn) > 0 {
		var match bool
		for _, ln := range node.matchOn {
			if _, match = metric[ln]; !match {
				break
			}
		}
		// If the metric does not contain the labels to match on, build the hash
		// over the whole metric to give it a unique hash.
		if !match {
			labels = make(clientmodel.LabelNames, 0, len(metric))
			for ln := range metric {
				labels = append(labels, ln)
			}
		} else {
			labels = node.matchOn
		}
	} else {
		labels = make(clientmodel.LabelNames, 0, len(metric))
		for ln := range metric {
			if ln != clientmodel.MetricNameLabel {
				labels = append(labels, ln)
			}
		}
	}
	return clientmodel.SignatureForLabels(metric, labels)
}

// Eval implements the MatrixNode interface and returns the value of
// the selector.
func (node *MatrixSelector) Eval(timestamp clientmodel.Timestamp) Matrix {
	interval := &metric.Interval{
		OldestInclusive: timestamp.Add(-node.interval - node.offset),
		NewestInclusive: timestamp.Add(-node.offset),
	}

	//// timer := v.stats.GetTimer(stats.GetRangeValuesTime).Start()
	sampleStreams := []SampleStream{}
	for fp, it := range node.iterators {
		samplePairs := it.GetRangeValues(*interval)
		if len(samplePairs) == 0 {
			continue
		}

		if node.offset != 0 {
			for _, sp := range samplePairs {
				sp.Timestamp = sp.Timestamp.Add(node.offset)
			}
		}

		sampleStream := SampleStream{
			Metric: node.metrics[fp],
			Values: samplePairs,
		}
		sampleStreams = append(sampleStreams, sampleStream)
	}
	//// timer.Stop()
	return sampleStreams
}

// EvalBoundaries implements the MatrixNode interface and returns the
// boundary values of the selector.
func (node *MatrixSelector) EvalBoundaries(timestamp clientmodel.Timestamp) Matrix {
	interval := &metric.Interval{
		OldestInclusive: timestamp.Add(-node.interval),
		NewestInclusive: timestamp,
	}

	//// timer := v.stats.GetTimer(stats.GetBoundaryValuesTime).Start()
	sampleStreams := []SampleStream{}
	for fp, it := range node.iterators {
		samplePairs := it.GetBoundaryValues(*interval)
		if len(samplePairs) == 0 {
			continue
		}

		sampleStream := SampleStream{
			Metric: node.metrics[fp],
			Values: samplePairs,
		}
		sampleStreams = append(sampleStreams, sampleStream)
	}
	//// timer.Stop()
	return sampleStreams
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
func (node *StringLiteral) Eval(timestamp clientmodel.Timestamp) string {
	return node.str
}

// Eval implements the StringNode interface and returns the result of
// the function call.
func (node *StringFunctionCall) Eval(timestamp clientmodel.Timestamp) string {
	return node.function.callFn(timestamp, node.args).(string)
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
func NewVectorSelector(m metric.LabelMatchers, offset time.Duration) *VectorSelector {
	return &VectorSelector{
		labelMatchers: m,
		offset:        offset,
		iterators:     map[clientmodel.Fingerprint]local.SeriesIterator{},
		metrics:       map[clientmodel.Fingerprint]clientmodel.COWMetric{},
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
	case ScalarType:
		return &ScalarFunctionCall{
			function: function,
			args:     args,
		}, nil
	case VectorType:
		return &VectorFunctionCall{
			function: function,
			args:     args,
		}, nil
	case StringType:
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
func NewArithExpr(opType BinOpType, lhs Node, rhs Node, matchCard VectorMatchCardinality, matchOn, include clientmodel.LabelNames) (Node, error) {
	if !nodesHaveTypes(Nodes{lhs, rhs}, []ExprType{ScalarType, VectorType}) {
		return nil, errors.New("binary operands must be of vector or scalar type")
	}

	if opType == And || opType == Or {
		if lhs.Type() == ScalarType || rhs.Type() == ScalarType {
			return nil, errors.New("AND and OR operators may only be used between vectors")
		}
		// Logical operations must never be used with group modifiers.
		if len(include) > 0 {
			return nil, errors.New("AND and OR operators must not have a group modifier")
		}
	}
	if lhs.Type() != VectorType || rhs.Type() != VectorType {
		if matchCard != MatchOneToOne || matchOn != nil || include != nil {
			return nil, errors.New("binary scalar expressions cannot have vector matching options")
		}
	}

	if lhs.Type() == VectorType || rhs.Type() == VectorType {
		return &VectorArithExpr{
			opType:           opType,
			lhs:              lhs,
			rhs:              rhs,
			matchCardinality: matchCard,
			matchOn:          matchOn,
			includeLabels:    include,
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
func NewMatrixSelector(vector *VectorSelector, interval time.Duration, offset time.Duration) *MatrixSelector {
	return &MatrixSelector{
		labelMatchers: vector.labelMatchers,
		interval:      interval,
		offset:        offset,
		iterators:     map[clientmodel.Fingerprint]local.SeriesIterator{},
		metrics:       map[clientmodel.Fingerprint]clientmodel.COWMetric{},
	}
}

// NewStringLiteral returns a StringLiteral with the given string as
// value.
func NewStringLiteral(str string) *StringLiteral {
	return &StringLiteral{
		str: str,
	}
}
