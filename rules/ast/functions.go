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
	"container/heap"
	"fmt"
	"math"
	"sort"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

// Function represents a function of the expression language and is
// used by function nodes.
type Function struct {
	name       string
	argTypes   []ExprType
	returnType ExprType
	callFn     func(timestamp clientmodel.Timestamp, args []Node) interface{}
}

// CheckArgTypes returns a non-nil error if the number or types of
// passed in arg nodes do not match the function's expectations.
func (function *Function) CheckArgTypes(args []Node) error {
	if len(function.argTypes) != len(args) {
		return fmt.Errorf(
			"wrong number of arguments to function %v(): %v expected, %v given",
			function.name, len(function.argTypes), len(args),
		)
	}
	for idx, argType := range function.argTypes {
		invalidType := false
		var expectedType string
		if _, ok := args[idx].(ScalarNode); argType == SCALAR && !ok {
			invalidType = true
			expectedType = "scalar"
		}
		if _, ok := args[idx].(VectorNode); argType == VECTOR && !ok {
			invalidType = true
			expectedType = "vector"
		}
		if _, ok := args[idx].(MatrixNode); argType == MATRIX && !ok {
			invalidType = true
			expectedType = "matrix"
		}
		if _, ok := args[idx].(StringNode); argType == STRING && !ok {
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

// === delta(matrix MatrixNode, isCounter ScalarNode) Vector ===
func deltaImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	matrixNode := args[0].(MatrixNode)
	isCounter := args[1].(ScalarNode).Eval(timestamp) > 0
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

		resultSample := &clientmodel.Sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: timestamp,
		}
		delete(resultSample.Metric, clientmodel.MetricNameLabel)
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
	*s = append(*s, x.(*clientmodel.Sample))
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
	for k, v := range vector[0].Metric {
		// TODO(julius): Revisit this when https://github.com/prometheus/prometheus/issues/380
		// is implemented.
		if k == clientmodel.MetricNameLabel {
			continue
		}
		common[k] = v
	}

	for _, el := range vector[1:] {
		for k, v := range common {
			if el.Metric[k] != v {
				// Deletion of map entries while iterating over them is safe.
				// From http://golang.org/ref/spec#For_statements:
				// "If map entries that have not yet been reached are deleted during
				// iteration, the corresponding iteration values will not be produced."
				delete(common, k)
			}
		}
	}

	for _, el := range vector {
		for k := range el.Metric {
			if _, ok := common[k]; ok {
				delete(el.Metric, k)
			}
		}
	}
	return vector
}

// === sampleVectorImpl() Vector ===
func sampleVectorImpl(timestamp clientmodel.Timestamp, args []Node) interface{} {
	return Vector{
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "0",
			},
			Value:     10,
			Timestamp: timestamp,
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "1",
			},
			Value:     20,
			Timestamp: timestamp,
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "2",
			},
			Value:     30,
			Timestamp: timestamp,
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "3",
				"group":                     "canary",
			},
			Value:     40,
			Timestamp: timestamp,
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "2",
				"group":                     "canary",
			},
			Value:     40,
			Timestamp: timestamp,
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "3",
				"group":                     "mytest",
			},
			Value:     40,
			Timestamp: timestamp,
		},
		&clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "3",
				"group":                     "mytest",
			},
			Value:     40,
			Timestamp: timestamp,
		},
	}
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

		delete(el.Metric, clientmodel.MetricNameLabel)
		resultVector = append(resultVector, &clientmodel.Sample{
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
		delete(el.Metric, clientmodel.MetricNameLabel)
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
		&clientmodel.Sample{
			Metric:    m,
			Value:     1,
			Timestamp: timestamp,
		},
	}
}

var functions = map[string]*Function{
	"abs": {
		name:       "abs",
		argTypes:   []ExprType{VECTOR},
		returnType: VECTOR,
		callFn:     absImpl,
	},
	"absent": {
		name:       "absent",
		argTypes:   []ExprType{VECTOR},
		returnType: VECTOR,
		callFn:     absentImpl,
	},
	"avg_over_time": {
		name:       "avg_over_time",
		argTypes:   []ExprType{MATRIX},
		returnType: VECTOR,
		callFn:     avgOverTimeImpl,
	},
	"bottomk": {
		name:       "bottomk",
		argTypes:   []ExprType{SCALAR, VECTOR},
		returnType: VECTOR,
		callFn:     bottomkImpl,
	},
	"count_over_time": {
		name:       "count_over_time",
		argTypes:   []ExprType{MATRIX},
		returnType: VECTOR,
		callFn:     countOverTimeImpl,
	},
	"count_scalar": {
		name:       "count_scalar",
		argTypes:   []ExprType{VECTOR},
		returnType: SCALAR,
		callFn:     countScalarImpl,
	},
	"delta": {
		name:       "delta",
		argTypes:   []ExprType{MATRIX, SCALAR},
		returnType: VECTOR,
		callFn:     deltaImpl,
	},
	"drop_common_labels": {
		name:       "drop_common_labels",
		argTypes:   []ExprType{VECTOR},
		returnType: VECTOR,
		callFn:     dropCommonLabelsImpl,
	},
	"max_over_time": {
		name:       "max_over_time",
		argTypes:   []ExprType{MATRIX},
		returnType: VECTOR,
		callFn:     maxOverTimeImpl,
	},
	"min_over_time": {
		name:       "min_over_time",
		argTypes:   []ExprType{MATRIX},
		returnType: VECTOR,
		callFn:     minOverTimeImpl,
	},
	"rate": {
		name:       "rate",
		argTypes:   []ExprType{MATRIX},
		returnType: VECTOR,
		callFn:     rateImpl,
	},
	"sampleVector": {
		name:       "sampleVector",
		argTypes:   []ExprType{},
		returnType: VECTOR,
		callFn:     sampleVectorImpl,
	},
	"scalar": {
		name:       "scalar",
		argTypes:   []ExprType{VECTOR},
		returnType: SCALAR,
		callFn:     scalarImpl,
	},
	"sort": {
		name:       "sort",
		argTypes:   []ExprType{VECTOR},
		returnType: VECTOR,
		callFn:     sortImpl,
	},
	"sort_desc": {
		name:       "sort_desc",
		argTypes:   []ExprType{VECTOR},
		returnType: VECTOR,
		callFn:     sortDescImpl,
	},
	"sum_over_time": {
		name:       "sum_over_time",
		argTypes:   []ExprType{MATRIX},
		returnType: VECTOR,
		callFn:     sumOverTimeImpl,
	},
	"time": {
		name:       "time",
		argTypes:   []ExprType{},
		returnType: SCALAR,
		callFn:     timeImpl,
	},
	"topk": {
		name:       "topk",
		argTypes:   []ExprType{SCALAR, VECTOR},
		returnType: VECTOR,
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
