// Copyright 2015 The Prometheus Authors
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

package promql

// Function represents a function of the expression language and is
// used by function nodes.
type Function struct {
	Name         string
	ArgTypes     []ExprType
	OptionalArgs int
	ReturnType   ExprType
	Call         func()
}

var functions = map[string]*Function{
	"abs": {
		Name:       "abs",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"absent": {
		Name:       "absent",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"avg_over_time": {
		Name:       "avg_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"bottomk": {
		Name:       "bottomk",
		ArgTypes:   []ExprType{ExprScalar, ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"ceil": {
		Name:       "ceil",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"count_over_time": {
		Name:       "count_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"count_scalar": {
		Name:       "count_scalar",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprScalar,
		Call:       func() {},
	},
	"delta": {
		Name:         "delta",
		ArgTypes:     []ExprType{ExprMatrix, ExprScalar},
		OptionalArgs: 1, // The 2nd argument is deprecated.
		ReturnType:   ExprVector,
		Call:         func() {},
	},
	"deriv": {
		Name:       "deriv",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"drop_common_labels": {
		Name:       "drop_common_labels",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"exp": {
		Name:       "exp",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"floor": {
		Name:       "floor",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"histogram_quantile": {
		Name:       "histogram_quantile",
		ArgTypes:   []ExprType{ExprScalar, ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"ln": {
		Name:       "ln",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"log10": {
		Name:       "log10",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"log2": {
		Name:       "log2",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"max_over_time": {
		Name:       "max_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"min_over_time": {
		Name:       "min_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"rate": {
		Name:       "rate",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"round": {
		Name:         "round",
		ArgTypes:     []ExprType{ExprVector, ExprScalar},
		OptionalArgs: 1,
		ReturnType:   ExprVector,
		Call:         func() {},
	},
	"scalar": {
		Name:       "scalar",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprScalar,
		Call:       func() {},
	},
	"sort": {
		Name:       "sort",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"sort_desc": {
		Name:       "sort_desc",
		ArgTypes:   []ExprType{ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"sum_over_time": {
		Name:       "sum_over_time",
		ArgTypes:   []ExprType{ExprMatrix},
		ReturnType: ExprVector,
		Call:       func() {},
	},
	"time": {
		Name:       "time",
		ArgTypes:   []ExprType{},
		ReturnType: ExprScalar,
		Call:       func() {},
	},
	"topk": {
		Name:       "topk",
		ArgTypes:   []ExprType{ExprScalar, ExprVector},
		ReturnType: ExprVector,
		Call:       func() {},
	},
}

// GetFunction returns a predefined Function object for the given name.
func GetFunction(name string) (*Function, bool) {
	function, ok := functions[name]
	return function, ok
}
