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
	"fmt"
	"math"
	"sort"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

// Function represents a function of the expression language and is
// used by function nodes.
type Function struct {
	name       string
	argTypes   []ExprType
	returnType ExprType
	callFn     func(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{}
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
func timeImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
	return clientmodel.SampleValue(time.Now().Unix())
}

// === delta(matrix MatrixNode, isCounter ScalarNode) Vector ===
func deltaImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
	matrixNode := args[0].(MatrixNode)
	isCounter := args[1].(ScalarNode).Eval(timestamp, view) > 0
	resultVector := Vector{}

	// If we treat these metrics as counters, we need to fetch all values
	// in the interval to find breaks in the timeseries' monotonicity.
	// I.e. if a counter resets, we want to ignore that reset.
	var matrixValue Matrix
	if isCounter {
		matrixValue = matrixNode.Eval(timestamp, view)
	} else {
		matrixValue = matrixNode.EvalBoundaries(timestamp, view)
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

		targetInterval := args[0].(*MatrixLiteral).interval
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
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === rate(node *MatrixNode) Vector ===
func rateImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
	args = append(args, &ScalarLiteral{value: 1})
	vector := deltaImpl(timestamp, view, args).(Vector)

	// TODO: could be other type of MatrixNode in the future (right now, only
	// MatrixLiteral exists). Find a better way of getting the duration of a
	// matrix, such as looking at the samples themselves.
	interval := args[0].(*MatrixLiteral).interval
	for i := range vector {
		vector[i].Value /= clientmodel.SampleValue(interval / time.Second)
	}
	return vector
}

type vectorByValueSorter struct {
	vector Vector
}

func (sorter vectorByValueSorter) Len() int {
	return len(sorter.vector)
}

func (sorter vectorByValueSorter) Less(i, j int) bool {
	return sorter.vector[i].Value < sorter.vector[j].Value
}

func (sorter vectorByValueSorter) Swap(i, j int) {
	sorter.vector[i], sorter.vector[j] = sorter.vector[j], sorter.vector[i]
}

// === sort(node *VectorNode) Vector ===
func sortImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
	byValueSorter := vectorByValueSorter{
		vector: args[0].(VectorNode).Eval(timestamp, view),
	}
	sort.Sort(byValueSorter)
	return byValueSorter.vector
}

// === sortDesc(node *VectorNode) Vector ===
func sortDescImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
	descByValueSorter := utility.ReverseSorter{
		Interface: vectorByValueSorter{
			vector: args[0].(VectorNode).Eval(timestamp, view),
		},
	}
	sort.Sort(descByValueSorter)
	return descByValueSorter.Interface.(vectorByValueSorter).vector
}

// === sampleVectorImpl() Vector ===
func sampleVectorImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
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

// === scalar(node *VectorNode) Scalar ===
func scalarImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
	v := args[0].(VectorNode).Eval(timestamp, view)
	if len(v) != 1 {
		return clientmodel.SampleValue(math.NaN())
	}
	return clientmodel.SampleValue(v[0].Value)
}

// === count_scalar(vector VectorNode) model.SampleValue ===
func countScalarImpl(timestamp clientmodel.Timestamp, view *viewAdapter, args []Node) interface{} {
	return clientmodel.SampleValue(len(args[0].(VectorNode).Eval(timestamp, view)))
}

var functions = map[string]*Function{
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
	"time": {
		name:       "time",
		argTypes:   []ExprType{},
		returnType: SCALAR,
		callFn:     timeImpl,
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
