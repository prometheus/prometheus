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
	"math"

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
	srcMap = map[string]string{
		"udf_mad_over_time": `
			// fmt := import("fmt")
			math := import("math")
			util := import("util")

			mad := func(seq) {
				median := util.median(seq)
				if is_error(median) {
					return median
				}
				deltas := []
				for x in seq {
					deltas = append(deltas, math.abs(x-median))
				}
				// fmt.println(deltas)
				return util.median(deltas)
			}

			output := mad(input)
		`,
		"udf_std_dev_simple_over_time": `
			math := import("math")

			std := func(seq) {
				n := len(seq)
				if n == 0 {
					return error("cannot calculate mean of empty array")
				}
				sum := 0
				for x in seq {
					sum += x
				}
				mean := sum / n
				variance := 0
				for x in seq {
					variance += math.pow(x-mean, 2)
				}
				variance /= n
				return math.sqrt(variance)
			}

			output := std(input)
		`,
		"udf_std_dev_over_time": `
			math := import("math")

			std := func(seq) {
				count := 0
				mean := 0
				aux := 0
				for x in seq {
					count++
					delta := x - mean
					mean += delta/count
					aux += delta*(x-mean)
				}
				if count == 0 {
					return error("cannot calculate mean of empty array")
				}
				return math.sqrt(aux / count)
			}

			output := std(input)
		`,
	}
)

func init() {
	for name, src := range srcMap {
		err := addUDF(name, src, []string{"math", "rand"}, true)
		if err != nil {
			panic(err)
		}
	}
	for _, name := range []string{"mad_over_time", "std_dev_over_time"} {
		parser.AddFunction(name)
	}
	FunctionCalls["mad_over_time"] = funcMadOverTime
	FunctionCalls["std_dev_over_time"] = funcStdDevOverTime
}

func AddUDFfromConfig(cfg *config.UDFConfig) error {
	return addUDF("udf_"+cfg.Name, cfg.Src, cfg.Modules, cfg.UseUtil)
}

func addUDF(name, src string, modules []string, useUtil bool) error {
	c, err := compileSrc(src, modules, useUtil)
	if err != nil {
		return err
	}
	parser.AddFunction(name)
	FunctionCalls[name] = genFunctionCall(name, c)
	return nil
}

func compileSrc(src string, modules []string, useUtil bool) (*tengo.Compiled, error) {
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
	s.Add("input", []interface{}{})
	return s.Compile()
}

func runCustomFunc(funcName string, input []FPoint, compiled *tengo.Compiled) (float64, error) {
	inputInterface := make([]interface{}, len(input))
	for i, f := range input {
		inputInterface[i] = f.F
	}
	c := compiled.Clone()
	if err := c.Set("input", inputInterface); err != nil {
		return 0, err
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

func genFunctionCall(funcName string, c *tengo.Compiled) FunctionCall {
	return func(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
		if len(vals[0].(Matrix)[0].Floats) == 0 {
			// TODO(beorn7): The passed values only contain
			// histograms. This ignores histograms for now. If
			// there are only histograms, we have to return without adding
			// anything to enh.Out.
			return enh.Out, nil
		}
		return aggrOverTime(vals, enh, func(s Series) float64 {
			res, err := runCustomFunc(funcName, s.Floats, c)
			if err != nil {
				return 0
			}
			return res
		}), nil
	}
}

// below is just for benchmark comparison, should be removed before PR is merged

// duplicate of funcStddevOverTime that doesn't use kahan sum so it is comparable to funcUdfStdDevOverTime
// === std_dev_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcStdDevOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		// TODO(beorn7): The passed values only contain
		// histograms. std_dev_over_time ignores histograms for now. If
		// there are only histograms, we have to return without adding
		// anything to enh.Out.
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		var count float64
		var mean float64
		var aux float64
		for _, f := range s.Floats {
			count++
			delta := f.F - mean
			mean += delta / count
			aux += delta * (f.F - mean)
		}
		return math.Sqrt(aux / count)
	}), nil
}

// === mad_over_time(Matrix parser.ValueTypeMatrix) (Vector, Annotations) ===
func funcMadOverTime(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if len(vals[0].(Matrix)[0].Floats) == 0 {
		// TODO(beorn7): The passed values only contain
		// histograms. mad_over_time ignores histograms for now. If
		// there are only histograms, we have to return without adding
		// anything to enh.Out.
		return enh.Out, nil
	}
	return aggrOverTime(vals, enh, func(s Series) float64 {
		values := make(vectorByValueHeap, 0, len(s.Floats))
		for _, f := range s.Floats {
			values = append(values, Sample{F: f.F})
		}
		median := quantile(0.5, values)
		values = make(vectorByValueHeap, 0, len(s.Floats))
		for _, f := range s.Floats {
			values = append(values, Sample{F: math.Abs(f.F - median)})
		}
		return quantile(0.5, values)
	}), nil
}
