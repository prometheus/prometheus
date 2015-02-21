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
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// Function represents a function of the expression language and is
// used by function nodes.
type Function struct {
	name         string
	argTypes     []ExprType
	optionalArgs int
	returnType   ExprType
	callFn       func(timestamp clientmodel.Timestamp, args []Node) interface{}
}

// CheckArgTypes returns a non-nil error if the number or types of
// passed in arg nodes do not match the function's expectations.
func (function *Function) CheckArgTypes(args []Node) error {
	if len(function.argTypes) < len(args) {
		return fmt.Errorf(
			"too many arguments to function %v(): %v expected at most, %v given",
			function.name, len(function.argTypes), len(args),
		)
	}
	if len(function.argTypes)-function.optionalArgs > len(args) {
		return fmt.Errorf(
			"too few arguments to function %v(): %v expected at least, %v given",
			function.name, len(function.argTypes)-function.optionalArgs, len(args),
		)
	}
	for idx, arg := range args {
		invalidType := false
		var expectedType string
		if _, ok := arg.(ScalarNode); function.argTypes[idx] == ScalarType && !ok {
			invalidType = true
			expectedType = "scalar"
		}
		if _, ok := arg.(VectorNode); function.argTypes[idx] == VectorType && !ok {
			invalidType = true
			expectedType = "vector"
		}
		if _, ok := arg.(MatrixNode); function.argTypes[idx] == MatrixType && !ok {
			invalidType = true
			expectedType = "matrix"
		}
		if _, ok := arg.(StringNode); function.argTypes[idx] == StringType && !ok {
			invalidType = true
			expectedType = "string"
		}

		if invalidType {
			return fmt.Errorf(
				"wrong type for argument %v in function %v(), expected %v",
				idx, function.name, expectedType,
			)
		}
	}
	return nil
}

// === time() clientmodel.SampleValue ===
func timeImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return clientmodel.SampleValue(timestamp.Unix())
}

// === delta(matrix MatrixNode, isCounter=0 ScalarNode) Vector ===
func deltaImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	matrixNode := args[0].(MatrixNode)
	isCounter := len(args) >= 2 && args[1].(ScalarNode).Eval(timestamp) > 0
	resultVector := Vector{}

	// If we treat these metrics as counters, we need to fetch all values
	// in the interval to find breaks in the timeseries' monotonicity.
	// I.e. if a counter resets, we want to ignore that reset.
	var matrixValue Matrix
	if isCounter {
		matrixValue = matrixNode.Eval(timestamp)
	} else {
		matrixValue = matrixNode.EvalBoundaries(timestamp)
	}
	for _, samples := range matrixValue {
		// No sense in trying to compute a delta without at least two points. Drop
		// this vector element.
		if len(samples.Values) < 2 {
			continue
		}

		counterCorrection := clientmodel.SampleValue(0)
		lastValue := clientmodel.SampleValue(0)
		for _, sample := range samples.Values {
			currentValue := sample.Value
			if isCounter && currentValue < lastValue {
				counterCorrection += lastValue - currentValue
			}
			lastValue = currentValue
		}
		resultValue := lastValue - samples.Values[0].Value + counterCorrection

		targetInterval := args[0].(*MatrixSelector).interval
		sampledInterval := samples.Values[len(samples.Values)-1].Timestamp.Sub(samples.Values[0].Timestamp)
		if sampledInterval == 0 {
			// Only found one sample. Cannot compute a rate from this.
			continue
		}
		// Correct for differences in target vs. actual delta interval.
		//
		// Above, we didn't actually calculate the delta for the specified target
		// interval, but for an interval between the first and last found samples
		// under the target interval, which will usually have less time between
		// them. Depending on how many samples are found under a target interval,
		// the delta results are distorted and temporal aliasing occurs (ugly
		// bumps). This effect is corrected for below.
		intervalCorrection := clientmodel.SampleValue(targetInterval) / clientmodel.SampleValue(sampledInterval)
		resultValue *= intervalCorrection

		resultSample := &Sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: timestamp,
		}
		resultSample.Metric.Delete(clientmodel.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === rate(node MatrixNode) Vector ===
func rateImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	args = append(args, &ScalarLiteral{value: 1})
	vector := deltaImpl(timestamp, args).(Vector)

	// TODO: could be other type of MatrixNode in the future (right now, only
	// MatrixSelector exists). Find a better way of getting the duration of a
	// matrix, such as looking at the samples themselves.
	interval := args[0].(*MatrixSelector).interval
	for i := range vector {
		vector[i].Value /= clientmodel.SampleValue(interval / time.Second)
	}
	return vector
}

type vectorByValueHeap Vector

func (s vectorByValueHeap) Len() int {
	return len(s)
}

func (s vectorByValueHeap) Less(i, j int) bool {
	return s[i].Value < s[j].Value
}

func (s vectorByValueHeap) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *vectorByValueHeap) Push(x interface{}) {
	*s = append(*s, x.(*Sample))
}

func (s *vectorByValueHeap) Pop() interface{} {
	old := *s
	n := len(old)
	el := old[n-1]
	*s = old[0 : n-1]
	return el
}

type reverseHeap struct {
	heap.Interface
}

func (s reverseHeap) Less(i, j int) bool {
	return s.Interface.Less(j, i)
}

// === sort(node VectorNode) Vector ===
func sortImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	byValueSorter := vectorByValueHeap(args[0].(VectorNode).Eval(timestamp))
	sort.Sort(byValueSorter)
	return Vector(byValueSorter)
}

// === sortDesc(node VectorNode) Vector ===
func sortDescImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	byValueSorter := vectorByValueHeap(args[0].(VectorNode).Eval(timestamp))
	sort.Sort(sort.Reverse(byValueSorter))
	return Vector(byValueSorter)
}

// === topk(k ScalarNode, node VectorNode) Vector ===
func topkImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	k := int(args[0].(ScalarNode).Eval(timestamp))
	if k < 1 {
		return Vector{}
	}

	topk := make(vectorByValueHeap, 0, k)
	vector := args[1].(VectorNode).Eval(timestamp)

	for _, el := range vector {
		if len(topk) < k || topk[0].Value < el.Value {
			if len(topk) == k {
				heap.Pop(&topk)
			}
			heap.Push(&topk, el)
		}
	}
	sort.Sort(sort.Reverse(topk))
	return Vector(topk)
}

// === bottomk(k ScalarNode, node VectorNode) Vector ===
func bottomkImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	k := int(args[0].(ScalarNode).Eval(timestamp))
	if k < 1 {
		return Vector{}
	}

	bottomk := make(vectorByValueHeap, 0, k)
	bkHeap := reverseHeap{Interface: &bottomk}
	vector := args[1].(VectorNode).Eval(timestamp)

	for _, el := range vector {
		if len(bottomk) < k || bottomk[0].Value > el.Value {
			if len(bottomk) == k {
				heap.Pop(&bkHeap)
			}
			heap.Push(&bkHeap, el)
		}
	}
	sort.Sort(bottomk)
	return Vector(bottomk)
}

// === drop_common_labels(node VectorNode) Vector ===
func dropCommonLabelsImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	vector := args[0].(VectorNode).Eval(timestamp)
	if len(vector) < 1 {
		return Vector{}
	}
	common := clientmodel.LabelSet{}
	for k, v := range vector[0].Metric.Metric {
		// TODO(julius): Should we also drop common metric names?
		if k == clientmodel.MetricNameLabel {
			continue
		}
		common[k] = v
	}

	for _, el := range vector[1:] {
		for k, v := range common {
			if el.Metric.Metric[k] != v {
				// Deletion of map entries while iterating over them is safe.
				// From http://golang.org/ref/spec#For_statements:
				// "If map entries that have not yet been reached are deleted during
				// iteration, the corresponding iteration values will not be produced."
				delete(common, k)
			}
		}
	}

	for _, el := range vector {
		for k := range el.Metric.Metric {
			if _, ok := common[k]; ok {
				el.Metric.Delete(k)
			}
		}
	}
	return vector
}

// === round(vector VectorNode, toNearest=1 Scalar) Vector ===
func roundImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	// round returns a number rounded to toNearest.
	// Ties are solved by rounding up.
	toNearest := float64(1)
	if len(args) >= 2 {
		toNearest = float64(args[1].(ScalarNode).Eval(timestamp))
	}
	// Invert as it seems to cause fewer floating point accuracy issues.
	toNearestInverse := 1.0 / toNearest

	n := args[0].(VectorNode)
	vector := n.Eval(timestamp)
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Floor(float64(el.Value)*toNearestInverse+0.5) / toNearestInverse)
	}
	return vector
}

// === scalar(node VectorNode) Scalar ===
func scalarImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	v := args[0].(VectorNode).Eval(timestamp)
	if len(v) != 1 {
		return clientmodel.SampleValue(math.NaN())
	}
	return clientmodel.SampleValue(v[0].Value)
}

// === count_scalar(vector VectorNode) model.SampleValue ===
func countScalarImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return clientmodel.SampleValue(len(args[0].(VectorNode).Eval(timestamp)))
}

func aggrOverTime(timestamp clientmodel.Timestamp, args []Node, aggrFn func(metric.Values) clientmodel.SampleValue) interface{} {
	n := args[0].(MatrixNode)
	matrixVal := n.Eval(timestamp)
	resultVector := Vector{}

	for _, el := range matrixVal {
		if len(el.Values) == 0 {
			continue
		}

		el.Metric.Delete(clientmodel.MetricNameLabel)
		resultVector = append(resultVector, &Sample{
			Metric:    el.Metric,
			Value:     aggrFn(el.Values),
			Timestamp: timestamp,
		})
	}
	return resultVector
}

// === avg_over_time(matrix MatrixNode) Vector ===
func avgOverTimeImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return aggrOverTime(timestamp, args, func(values metric.Values) clientmodel.SampleValue {
		var sum clientmodel.SampleValue
		for _, v := range values {
			sum += v.Value
		}
		return sum / clientmodel.SampleValue(len(values))
	})
}

// === count_over_time(matrix MatrixNode) Vector ===
func countOverTimeImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return aggrOverTime(timestamp, args, func(values metric.Values) clientmodel.SampleValue {
		return clientmodel.SampleValue(len(values))
	})
}

// === floor(vector VectorNode) Vector ===
func floorImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	n := args[0].(VectorNode)
	vector := n.Eval(timestamp)
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Floor(float64(el.Value)))
	}
	return vector
}

// === max_over_time(matrix MatrixNode) Vector ===
func maxOverTimeImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return aggrOverTime(timestamp, args, func(values metric.Values) clientmodel.SampleValue {
		max := math.Inf(-1)
		for _, v := range values {
			max = math.Max(max, float64(v.Value))
		}
		return clientmodel.SampleValue(max)
	})
}

// === min_over_time(matrix MatrixNode) Vector ===
func minOverTimeImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return aggrOverTime(timestamp, args, func(values metric.Values) clientmodel.SampleValue {
		min := math.Inf(1)
		for _, v := range values {
			min = math.Min(min, float64(v.Value))
		}
		return clientmodel.SampleValue(min)
	})
}

// === sum_over_time(matrix MatrixNode) Vector ===
func sumOverTimeImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return aggrOverTime(timestamp, args, func(values metric.Values) clientmodel.SampleValue {
		var sum clientmodel.SampleValue
		for _, v := range values {
			sum += v.Value
		}
		return sum
	})
}

// === abs(vector VectorNode) Vector ===
func absImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	n := args[0].(VectorNode)
	vector := n.Eval(timestamp)
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Abs(float64(el.Value)))
	}
	return vector
}

// === absent(vector VectorNode) Vector ===
func absentImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	n := args[0].(VectorNode)
	if len(n.Eval(timestamp)) > 0 {
		return Vector{}
	}
	m := clientmodel.Metric{}
	if vs, ok := n.(*VectorSelector); ok {
		for _, matcher := range vs.labelMatchers {
			if matcher.Type == metric.Equal && matcher.Name != clientmodel.MetricNameLabel {
				m[matcher.Name] = matcher.Value
			}
		}
	}
	return Vector{
		&Sample{
			Metric: clientmodel.COWMetric{
				Metric: m,
				Copied: true,
			},
			Value:     1,
			Timestamp: timestamp,
		},
	}
}

// === ceil(vector VectorNode) Vector ===
func ceilImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	n := args[0].(VectorNode)
	vector := n.Eval(timestamp)
	for _, el := range vector {
		el.Metric.Delete(clientmodel.MetricNameLabel)
		el.Value = clientmodel.SampleValue(math.Ceil(float64(el.Value)))
	}
	return vector
}

// === deriv(node MatrixNode) Vector ===
func derivImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	matrixNode := args[0].(MatrixNode)
	resultVector := Vector{}

	matrixValue := matrixNode.Eval(timestamp)
	for _, samples := range matrixValue {
		// No sense in trying to compute a derivative without at least two points.
		// Drop this vector element.
		if len(samples.Values) < 2 {
			continue
		}

		// Least squares.
		n := clientmodel.SampleValue(0)
		sumY := clientmodel.SampleValue(0)
		sumX := clientmodel.SampleValue(0)
		sumXY := clientmodel.SampleValue(0)
		sumX2 := clientmodel.SampleValue(0)
		for _, sample := range samples.Values {
			x := clientmodel.SampleValue(sample.Timestamp.UnixNano() / 1e9)
			n += 1.0
			sumY += sample.Value
			sumX += x
			sumXY += x * sample.Value
			sumX2 += x * x
		}
		numerator := sumXY - sumX*sumY/n
		denominator := sumX2 - (sumX*sumX)/n

		resultValue := numerator / denominator

		resultSample := &Sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: timestamp,
		}
		resultSample.Metric.Delete(clientmodel.MetricNameLabel)
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === histogram_quantile(k ScalarNode, vector VectorNode) Vector ===
func histogramQuantileImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	q := args[0].(ScalarNode).Eval(timestamp)
	inVec := args[1].(VectorNode).Eval(timestamp)
	outVec := Vector{}
	fpToMetricWithBuckets := map[clientmodel.Fingerprint]*metricWithBuckets{}
	for _, el := range inVec {
		upperBound, err := strconv.ParseFloat(
			string(el.Metric.Metric[clientmodel.BucketLabel]), 64,
		)
		if err != nil {
			// Oops, no bucket label or malformed label value. Skip.
			// TODO(beorn7): Issue a warning somehow.
			continue
		}
		fp := bucketFingerprint(el.Metric.Metric)
		mb, ok := fpToMetricWithBuckets[fp]
		if !ok {
			el.Metric.Delete(clientmodel.BucketLabel)
			el.Metric.Delete(clientmodel.MetricNameLabel)
			mb = &metricWithBuckets{el.Metric, nil}
			fpToMetricWithBuckets[fp] = mb
		}
		mb.buckets = append(mb.buckets, bucket{upperBound, el.Value})
	}

	for _, mb := range fpToMetricWithBuckets {
		outVec = append(outVec, &Sample{
			Metric:    mb.metric,
			Value:     clientmodel.SampleValue(quantile(q, mb.buckets)),
			Timestamp: timestamp,
		})
	}

	return outVec
}

var functions = map[string]*Function{
	"abs": {
		name:       "abs",
		argTypes:   []ExprType{VectorType},
		returnType: VectorType,
		callFn:     absImpl,
	},
	"absent": {
		name:       "absent",
		argTypes:   []ExprType{VectorType},
		returnType: VectorType,
		callFn:     absentImpl,
	},
	"avg_over_time": {
		name:       "avg_over_time",
		argTypes:   []ExprType{MatrixType},
		returnType: VectorType,
		callFn:     avgOverTimeImpl,
	},
	"bottomk": {
		name:       "bottomk",
		argTypes:   []ExprType{ScalarType, VectorType},
		returnType: VectorType,
		callFn:     bottomkImpl,
	},
	"ceil": {
		name:       "ceil",
		argTypes:   []ExprType{VectorType},
		returnType: VectorType,
		callFn:     ceilImpl,
	},
	"count_over_time": {
		name:       "count_over_time",
		argTypes:   []ExprType{MatrixType},
		returnType: VectorType,
		callFn:     countOverTimeImpl,
	},
	"count_scalar": {
		name:       "count_scalar",
		argTypes:   []ExprType{VectorType},
		returnType: ScalarType,
		callFn:     countScalarImpl,
	},
	"delta": {
		name:         "delta",
		argTypes:     []ExprType{MatrixType, ScalarType},
		optionalArgs: 1, // The 2nd argument is deprecated.
		returnType:   VectorType,
		callFn:       deltaImpl,
	},
	"deriv": {
		name:       "deriv",
		argTypes:   []ExprType{MatrixType},
		returnType: VectorType,
		callFn:     derivImpl,
	},
	"drop_common_labels": {
		name:       "drop_common_labels",
		argTypes:   []ExprType{VectorType},
		returnType: VectorType,
		callFn:     dropCommonLabelsImpl,
	},
	"floor": {
		name:       "floor",
		argTypes:   []ExprType{VectorType},
		returnType: VectorType,
		callFn:     floorImpl,
	},
	"histogram_quantile": {
		name:       "histogram_quantile",
		argTypes:   []ExprType{ScalarType, VectorType},
		returnType: VectorType,
		callFn:     histogramQuantileImpl,
	},
	"max_over_time": {
		name:       "max_over_time",
		argTypes:   []ExprType{MatrixType},
		returnType: VectorType,
		callFn:     maxOverTimeImpl,
	},
	"min_over_time": {
		name:       "min_over_time",
		argTypes:   []ExprType{MatrixType},
		returnType: VectorType,
		callFn:     minOverTimeImpl,
	},
	"rate": {
		name:       "rate",
		argTypes:   []ExprType{MatrixType},
		returnType: VectorType,
		callFn:     rateImpl,
	},
	"round": {
		name:         "round",
		argTypes:     []ExprType{VectorType, ScalarType},
		optionalArgs: 1,
		returnType:   VectorType,
		callFn:       roundImpl,
	},
	"scalar": {
		name:       "scalar",
		argTypes:   []ExprType{VectorType},
		returnType: ScalarType,
		callFn:     scalarImpl,
	},
	"sort": {
		name:       "sort",
		argTypes:   []ExprType{VectorType},
		returnType: VectorType,
		callFn:     sortImpl,
	},
	"sort_desc": {
		name:       "sort_desc",
		argTypes:   []ExprType{VectorType},
		returnType: VectorType,
		callFn:     sortDescImpl,
	},
	"sum_over_time": {
		name:       "sum_over_time",
		argTypes:   []ExprType{MatrixType},
		returnType: VectorType,
		callFn:     sumOverTimeImpl,
	},
	"time": {
		name:       "time",
		argTypes:   []ExprType{},
		returnType: ScalarType,
		callFn:     timeImpl,
	},
	"topk": {
		name:       "topk",
		argTypes:   []ExprType{ScalarType, VectorType},
		returnType: VectorType,
		callFn:     topkImpl,
	},
}

// GetFunction returns a predefined Function object for the given
// name.
func GetFunction(name string) (*Function, error) {
	function, ok := functions[name]
	if !ok {
		return nil, fmt.Errorf("couldn't find function %v()", name)
	}
	return function, nil
}
