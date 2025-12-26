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
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/grafana/regexp"
	"github.com/russross/blackfriday/v2"
)

var funcDocsRe = regexp.MustCompile("^## `([^)]+)\\(\\)` and `([^)]+)\\(\\)`\n$|^## `(.+)\\(\\)`\n$|^## (Trigonometric Functions)\n$")

func main() {
	// Read from local file instead of fetching from upstream.
	if len(os.Args) < 2 {
		log.Fatalln("Usage: gen_functions_docs <path-to-functions.md>")
	}
	functionsPath := os.Args[1]
	file, err := os.Open(functionsPath)
	if err != nil {
		log.Fatalln("Failed to open function docs:", err)
	}
	defer file.Close()

	funcDocs := map[string]string{}

	r := bufio.NewReader(file)
	currentFunc := ""
	currentDocs := ""

	saveCurrent := func() {
		switch currentFunc {
		case "<aggregation>_over_time":
			for _, fn := range []string{
				"avg_over_time",
				"min_over_time",
				"max_over_time",
				"sum_over_time",
				"count_over_time",
				"quantile_over_time",
				"stddev_over_time",
				"stdvar_over_time",
				"last_over_time",
				"present_over_time",
				"mad_over_time",
				"first_over_time",
				"ts_of_first_over_time",
				"ts_of_last_over_time",
				"ts_of_max_over_time",
				"ts_of_min_over_time",
			} {
				funcDocs[fn] = currentDocs
			}
		case "Trigonometric Functions":
			for _, fn := range []string{
				"acos",
				"acosh",
				"asin",
				"asinh",
				"atan",
				"atanh",
				"cos",
				"cosh",
				"sin",
				"sinh",
				"tan",
				"tanh",
				"deg",
				"pi",
				"rad",
			} {
				funcDocs[fn] = currentDocs
			}
		case "histogram_count_and_histogram_sum":
			funcDocs["histogram_count"] = currentDocs
			funcDocs["histogram_sum"] = currentDocs
		case "histogram_stddev_and_histogram_stdvar":
			funcDocs["histogram_stddev"] = currentDocs
			funcDocs["histogram_stdvar"] = currentDocs
		default:
			funcDocs[currentFunc] = currentDocs
		}
	}

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				saveCurrent()
				break
			}
			log.Fatalln("Error reading response body:", err)
		}

		matches := funcDocsRe.FindStringSubmatch(line)
		if len(matches) > 0 {
			if currentFunc != "" {
				saveCurrent()
			}
			currentDocs = ""

			if matches[1] != "" && matches[2] != "" {
				// Combined functions: "## `function1()` and `function2()`"
				// Store as "function1_and_function2" and handle in saveCurrent.
				currentFunc = matches[1] + "_and_" + matches[2]
			} else if matches[3] != "" {
				// Single function: "## `function_name()`"
				currentFunc = string(matches[3])
			} else if matches[4] != "" {
				// Special section: "## Trigonometric Functions"
				currentFunc = matches[4]
			}
		} else {
			currentDocs += line
		}
	}

	fmt.Println("import React from 'react';")
	fmt.Println("")
	fmt.Println("const funcDocs: Record<string, React.ReactNode> = {")

	funcNames := make([]string, 0, len(funcDocs))
	for k := range funcDocs {
		funcNames = append(funcNames, k)
	}
	sort.Strings(funcNames)
	for _, fn := range funcNames {
		// Translate:
		//   {   ===>   {'{'}
		//   }   ===>   {'}'}
		//
		// TODO: Make this set of conflicting string replacements less hacky.
		jsxEscapedDocs := strings.ReplaceAll(funcDocs[fn], "{", `__LEFT_BRACE__'{'__RIGHT_BRACE__`)
		jsxEscapedDocs = strings.ReplaceAll(jsxEscapedDocs, "}", `__LEFT_BRACE__'}'__RIGHT_BRACE__`)
		jsxEscapedDocs = strings.ReplaceAll(jsxEscapedDocs, "__LEFT_BRACE__", "{")
		jsxEscapedDocs = strings.ReplaceAll(jsxEscapedDocs, "__RIGHT_BRACE__", "}")
		fmt.Printf("  '%s': <>%s</>,\n", fn, string(blackfriday.Run([]byte(jsxEscapedDocs))))
	}
	fmt.Println("};")
	fmt.Println("")
	fmt.Println("export default funcDocs;")
}
