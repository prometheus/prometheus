// Copyright 2024 The Prometheus Authors
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

package kahansum

import "math"

// Inc performs addition of two floating-point numbers using the Kahan summation algorithm.
func Inc(inc, sum, c float64) (newSum, newC float64) {
	// We've seen Kahan summation return less accurate results when Inc function is
	// allowed to be inlined (see https://github.com/prometheus/prometheus/pull/16895).
	// Go permits fusing float operations (e.g. using fused multiply-add, which allows
	// calculating a*b+c without rounding the result of a*b to precision available in float64),
	// and Kahan sum is sensitive to float rounding behavior. Instead of forbidding inlining
	// (which only disallows fusing operations outside of Inc with operations  happening inside)
	// and eating the performance cost of non-inlined function calls, we forbid just the fusing
	// across Inc call boundary. We can do that by explicitly requesting Inc arguments and results
	// to be rounded to float64 precision, as documented in go spec (https://go.dev/ref/spec#Floating_point_operators).
	// The following casts are not no-ops!
	inc = float64(inc)
	sum = float64(sum)
	c = float64(c)

	t := sum + inc
	switch {
	case math.IsInf(t, 0):
		c = 0

		// Using Neumaier improvement, swap if next term larger than sum.
	case math.Abs(sum) >= math.Abs(inc):
		c += (sum - t) + inc
	default:
		c += (inc - t) + sum
	}

	t = float64(t)
	c = float64(c)
	return t, c
}

func Dec(dec, sum, c float64) (newSum, newC float64) {
	return Inc(-dec, sum, c)
}
