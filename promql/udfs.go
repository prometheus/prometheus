// Copyright 2023 The Prometheus Authors
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

import (
	"fmt"

	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/stdlib"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

var (
	utilModules = []string{"rand"}
	utilSrc     = `
		rand := import("rand")

		_quicksort := func(arr, left, right, less) {
			if right <= left {
				return
			}
			i := left
		
			for j := left; j < right; j++ {
				if less(arr[j], arr[right]) {
					if i != j {
						tmp := arr[i]
						arr[i] = arr[j]
						arr[j] = tmp
					}
					i++
				}
			}
		
			if i != right {
				tmp := arr[i]
				arr[i] = arr[right]
				arr[right] = tmp
			}
		
			_quicksort(arr, left, i-1, less)
			_quicksort(arr, i+1, right, less)
		}
		
		// sorts in place. https://github.com/d5/tengo/pull/416
		quicksort := func(arr, less) {
			_quicksort(arr, 0, len(arr)-1, less)
		}

		median_simple := func(seq) {
			n := len(seq)
			if n == 0 {
				return error("cannot calculate median of empty array")
			}
			quicksort(seq, func(i, j) {
				return i < j
			})
			if n % 2 == 0 {
				return (seq[n/2-1] + seq[n/2]) / 2
			} else {
				return seq[n/2]
			}
		}

		_quickselect := func(l, k, pivot_fn) {
			if len(l) == 1 {
				if k != 0 {
					return error("invalid k for 1-element list")
				}
				return l[0]
			}

			pivot := pivot_fn(l)

			lows := []
			highs := []
			pivot_length := 0
			for x in l {
				if x < pivot {
					lows = append(lows, x)
				} else if x > pivot {
					highs = append(highs, x)
				} else {
					pivot_length++
				}
			}
			low_length := len(lows)

			if k < low_length {
				return _quickselect(lows, k, pivot_fn)
			} else if k < (low_length + pivot_length) {
				return pivot
			} else {
				return _quickselect(highs, k - low_length - pivot_length, pivot_fn)
			}
		}

		_quickselect_median := func(l, pivot_fn) {
			length := len(l)
			half_length := length / 2
			if length % 2 == 1 {
				return _quickselect(l, half_length, pivot_fn)
			} else {
				return 0.5 * (_quickselect(l, half_length - 1, pivot_fn) + _quickselect(l, half_length, pivot_fn))
			}
		}

		random_choice := func(seq) {
			return seq[rand.intn(len(seq))]
		}

		// https://rcoh.me/posts/linear-time-median-finding/
		median := func(seq) {
			return _quickselect_median(seq, random_choice)
		}

		export {
			quicksort: quicksort,
			median: median
		}
	`
)

func AddUDFfromConfig(cfg *config.UDFConfig) error {
	return addUDF("udf_"+cfg.Name, cfg.Src, cfg.Modules, cfg.UseUtil, cfg.InputTypes)
}

func addUDF(name, src string, modules []string, useUtil bool, inputTypes []parser.ValueType) error {
	c, err := compileSrc(src, modules, useUtil, inputTypes)
	if err != nil {
		return err
	}
	parser.AddFunction(name, inputTypes)
	FunctionCalls[name] = genFunctionCall(name, c, inputTypes)
	return nil
}

func makeInputName(i int) string {
	return fmt.Sprintf("input%d", i)
}

func compileSrc(src string, modules []string, useUtil bool, inputTypes []parser.ValueType) (*tengo.Compiled, error) {
	s := tengo.NewScript([]byte(src))
	if useUtil {
		modules = append(modules, utilModules...)
	}
	mods := stdlib.GetModuleMap(modules...)
	// mods := stdlib.GetModuleMap(stdlib.AllModuleNames()...)
	if useUtil {
		mods.AddSourceModule("util", []byte(utilSrc))
	}
	s.SetImports(mods)
	for i := range inputTypes {
		var interfaceType interface{}
		if err := s.Add(makeInputName(i), interfaceType); err != nil {
			return nil, err
		}
	}
	return s.Compile()
}

func runCustomFunc(funcName string, compiled *tengo.Compiled, input []interface{}) (float64, error) {
	c := compiled.Clone()
	for i, v := range input {
		if err := c.Set(makeInputName(i), v); err != nil {
			return 0, err
		}
	}
	if err := c.Run(); err != nil {
		return 0, err
	}
	output := c.Get("output")
	if err := output.Error(); err != nil {
		return 0, err
	}
	return output.Float(), nil
}

func genFunctionCall(funcName string, c *tengo.Compiled, inputTypes []parser.ValueType) FunctionCall {
	return func(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
		inputTail := make([]interface{}, 0, len(inputTypes)-1)
		for i, t := range inputTypes[1:] {
			switch t {
			case parser.ValueTypeScalar:
				inputTail = append(inputTail, vals[1+i].(Vector)[0].F)
			case parser.ValueTypeString:
				inputTail = append(inputTail, stringFromArg(args[1+i]))
			}
		}
		if inputTypes[0] == parser.ValueTypeMatrix {
			series := vals[0].(Matrix)[0]
			if len(series.Floats) == 0 { // Ignore histograms
				return enh.Out, nil
			}
			aggrFn := func(s []FPoint) float64 {
				arr := make([]interface{}, len(s))
				for i, f := range s {
					arr[i] = f.F
				}
				input := append([]interface{}{arr}, inputTail...)
				res, err := runCustomFunc(funcName, c, input)
				if err != nil {
					panic(err)
				}
				return res
			}
			return append(enh.Out, Sample{F: aggrFn(series.Floats)}), nil
		} else if inputTypes[0] == parser.ValueTypeVector {
			tfFn := func(f float64) float64 {
				input := append([]interface{}{f}, inputTail...)
				res, err := runCustomFunc(funcName, c, input)
				if err != nil {
					panic(err)
				}
				return res
			}
			return simpleFunc(vals, enh, tfFn), nil
		}
		panic(fmt.Sprintf("unhandled value type for genFunctionCall: %s", inputTypes[0]))
	}
}
