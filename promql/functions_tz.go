// Copyright 2022 The Prometheus Authors
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

//go:build !windows
// +build !windows

package promql

import (
	"time"

	"github.com/prometheus/prometheus/promql/parser"
)

// === time_tz(tz string) float64 ===
func funcTimeTZ(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) Vector {
	tutc := time.Unix(enh.Ts/1000, 0)
	tz, err := time.LoadLocation(stringFromArg(args[0]))
	if err != nil {
		panic(err)
	}

	ttz := tutc.In(tz)
	_, offset := ttz.Zone()

	return Vector{Sample{Point: Point{
		V: float64(enh.Ts+int64(offset*1000)) / 1000,
	}}}
}
