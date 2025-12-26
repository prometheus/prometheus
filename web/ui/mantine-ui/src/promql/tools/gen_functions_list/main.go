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

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/prometheus/prometheus/promql/parser"
)

func formatValueType(vt parser.ValueType) string {
	return "valueType." + string(vt)
}

func formatValueTypes(vts []parser.ValueType) string {
	fmtVts := make([]string, 0, len(vts))
	for _, vt := range vts {
		fmtVts = append(fmtVts, formatValueType(vt))
	}
	return strings.Join(fmtVts, ", ")
}

func main() {
	fnNames := make([]string, 0, len(parser.Functions))
	for name := range parser.Functions {
		fnNames = append(fnNames, name)
	}
	sort.Strings(fnNames)
	fmt.Println(`import { valueType, Func } from './ast';

export const functionSignatures: Record<string, Func> = {`)
	for _, fnName := range fnNames {
		fn := parser.Functions[fnName]
		fmt.Printf("  %s: { name: '%s', argTypes: [%s], variadic: %d, returnType: %s },\n", fn.Name, fn.Name, formatValueTypes(fn.ArgTypes), fn.Variadic, formatValueType(fn.ReturnType))
	}
	fmt.Println("};")
}
