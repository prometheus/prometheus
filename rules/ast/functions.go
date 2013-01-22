package ast

import (
	"errors"
	"fmt"
	"github.com/matttproud/prometheus/model"
	"time"
)

type Function struct {
	name       string
	argTypes   []ExprType
	returnType ExprType
	callFn     func(timestamp *time.Time, args []Node) interface{}
}

func (function *Function) CheckArgTypes(args []Node) error {
	if len(function.argTypes) != len(args) {
		return errors.New(
			fmt.Sprintf("Wrong number of arguments to function %v(): %v expected, %v given",
				function.name, len(function.argTypes), len(args)))
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
			return errors.New(
				fmt.Sprintf("Wrong type for argument %v in function %v(), expected %v",
					idx, function.name, expectedType))
		}
	}
	return nil
}

// === time() ===
func timeImpl(timestamp *time.Time, args []Node) interface{} {
	return model.SampleValue(time.Now().Unix())
}

// === count(vector VectorNode) ===
func countImpl(timestamp *time.Time, args []Node) interface{} {
	return model.SampleValue(len(args[0].(VectorNode).Eval(timestamp)))
}

// === delta(matrix MatrixNode, isCounter ScalarNode) ===
func deltaImpl(timestamp *time.Time, args []Node) interface{} {
	matrixNode := args[0].(MatrixNode)
	isCounter := int(args[1].(ScalarNode).Eval(timestamp))
	resultVector := Vector{}

	// If we treat these metrics as counters, we need to fetch all values
	// in the interval to find breaks in the timeseries' monotonicity.
	// I.e. if a counter resets, we want to ignore that reset.
	var matrixValue Matrix
	if isCounter > 0 {
		matrixValue = matrixNode.Eval(timestamp)
	} else {
		matrixValue = matrixNode.EvalBoundaries(timestamp)
	}
	for _, samples := range matrixValue {
		counterCorrection := model.SampleValue(0)
		lastValue := model.SampleValue(0)
		for _, sample := range samples.Values {
			currentValue := sample.Value
			if currentValue < lastValue {
				counterCorrection += lastValue - currentValue
			}
			lastValue = currentValue
		}
		resultValue := lastValue - samples.Values[0].Value + counterCorrection
		resultSample := &model.Sample{
			Metric:    samples.Metric,
			Value:     resultValue,
			Timestamp: *timestamp,
		}
		resultVector = append(resultVector, resultSample)
	}
	return resultVector
}

// === rate(node *MatrixNode) ===
func rateImpl(timestamp *time.Time, args []Node) interface{} {
	args = append(args, &ScalarLiteral{value: 1})
	vector := deltaImpl(timestamp, args).(Vector)

	// TODO: could be other type of MatrixNode in the future (right now, only
	// MatrixLiteral exists). Find a better way of getting the duration of a
	// matrix, such as looking at the samples themselves.
	interval := args[0].(*MatrixLiteral).interval
	for _, sample := range vector {
		sample.Value /= model.SampleValue(interval / time.Second)
	}
	return vector
}

// === sampleVectorImpl() ===
func sampleVectorImpl(timestamp *time.Time, args []Node) interface{} {
	return Vector{
		&model.Sample{
			Metric: model.Metric{
				"name":     "http_requests",
				"job":      "api-server",
				"instance": "0",
			},
			Value:     10,
			Timestamp: *timestamp,
		},
		&model.Sample{
			Metric: model.Metric{
				"name":     "http_requests",
				"job":      "api-server",
				"instance": "1",
			},
			Value:     20,
			Timestamp: *timestamp,
		},
		&model.Sample{
			Metric: model.Metric{
				"name":     "http_requests",
				"job":      "api-server",
				"instance": "2",
			},
			Value:     30,
			Timestamp: *timestamp,
		},
		&model.Sample{
			Metric: model.Metric{
				"name":     "http_requests",
				"job":      "api-server",
				"instance": "3",
				"group":    "canary",
			},
			Value:     40,
			Timestamp: *timestamp,
		},
		&model.Sample{
			Metric: model.Metric{
				"name":     "http_requests",
				"job":      "api-server",
				"instance": "2",
				"group":    "canary",
			},
			Value:     40,
			Timestamp: *timestamp,
		},
		&model.Sample{
			Metric: model.Metric{
				"name":     "http_requests",
				"job":      "api-server",
				"instance": "3",
				"group":    "mytest",
			},
			Value:     40,
			Timestamp: *timestamp,
		},
		&model.Sample{
			Metric: model.Metric{
				"name":     "http_requests",
				"job":      "api-server",
				"instance": "3",
				"group":    "mytest",
			},
			Value:     40,
			Timestamp: *timestamp,
		},
	}
}

var functions = map[string]*Function{
	"time": {
		name:       "time",
		argTypes:   []ExprType{},
		returnType: SCALAR,
		callFn:     timeImpl,
	},
	"count": {
		name:       "count",
		argTypes:   []ExprType{VECTOR},
		returnType: SCALAR,
		callFn:     countImpl,
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
}

func GetFunction(name string) (*Function, error) {
	function, ok := functions[name]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Couldn't find function %v()", name))
	}
	return function, nil
}
