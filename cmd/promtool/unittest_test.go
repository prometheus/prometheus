// Copyright 2018 The Prometheus Authors
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
	"bytes"
	"testing"

	"github.com/grafana/regexp"

	"github.com/prometheus/prometheus/promql"
)

func TestRulesUnitTest(t *testing.T) {
	type args struct {
		files []string
	}
	tests := []struct {
		name      string
		args      args
		queryOpts promql.LazyLoaderOpts
		want      int
	}{
		{
			name: "Passing Unit Tests",
			args: args{
				files: []string{"./testdata/unittest.yml"},
			},
			want: 0,
		},
		{
			name: "Long evaluation interval",
			args: args{
				files: []string{"./testdata/long-period.yml"},
			},
			want: 0,
		},
		{
			name: "Bad input series",
			args: args{
				files: []string{"./testdata/bad-input-series.yml"},
			},
			want: 1,
		},
		{
			name: "Bad PromQL",
			args: args{
				files: []string{"./testdata/bad-promql.yml"},
			},
			want: 1,
		},
		{
			name: "Bad rules (syntax error)",
			args: args{
				files: []string{"./testdata/bad-rules-syntax-test.yml"},
			},
			want: 1,
		},
		{
			name: "Bad rules (error evaluating)",
			args: args{
				files: []string{"./testdata/bad-rules-error-test.yml"},
			},
			want: 1,
		},
		{
			name: "Simple failing test",
			args: args{
				files: []string{"./testdata/failing.yml"},
			},
			want: 1,
		},
		{
			name: "Disabled feature (@ modifier)",
			args: args{
				files: []string{"./testdata/at-modifier-test.yml"},
			},
			want: 1,
		},
		{
			name: "Enabled feature (@ modifier)",
			args: args{
				files: []string{"./testdata/at-modifier-test.yml"},
			},
			queryOpts: promql.LazyLoaderOpts{
				EnableAtModifier: true,
			},
			want: 0,
		},
		{
			name: "Disabled feature (negative offset)",
			args: args{
				files: []string{"./testdata/negative-offset-test.yml"},
			},
			want: 1,
		},
		{
			name: "Enabled feature (negative offset)",
			args: args{
				files: []string{"./testdata/negative-offset-test.yml"},
			},
			queryOpts: promql.LazyLoaderOpts{
				EnableNegativeOffset: true,
			},
			want: 0,
		},
	}
	reuseFiles := []string{}
	reuseCount := [2]int{} // count by exit code 0 or 1
	for _, tt := range tests {
		// Reuse some of these tests for the junit output testing, but only ones with default opts.
		if (tt.queryOpts == promql.LazyLoaderOpts{}) {
			reuseFiles = append(reuseFiles, tt.args.files...)
			reuseCount[tt.want] += len(tt.args.files)
		}
		t.Run(tt.name, func(t *testing.T) {
			if got := RulesUnitTest(tt.queryOpts, tt.args.files...); got != tt.want {
				t.Errorf("RulesUnitTest() = %v, want %v", got, tt.want)
			}
		})
	}

	reTop := regexp.MustCompile(`(?s)^<testsuites\W.*</testsuites>$`)
	reSuitesPass := regexp.MustCompile(`(?s)<testsuite [^>]*failures="0".*?</testsuite>`)
	reSuitesFail := regexp.MustCompile(`(?s)<testsuite [^>]*failures="[1-9].*?</testsuite>`)
	reCases := regexp.MustCompile(`(?s)<testcase .*?</testcase>`)

	t.Run("JUnit XML output", func(t *testing.T) {
		var buf bytes.Buffer
		if got := RulesUnitTestResults(&buf, promql.LazyLoaderOpts{}, reuseFiles...); got != 1 {
			t.Errorf("RulesUnitTestResults() = %v, want 1", got)
		}

		output := buf.Bytes()

		if !reTop.Match(output) {
			t.Errorf("JUnit output has no outer <testsuites>\n")
		}
		passes := len(reSuitesPass.FindAll(output, -1))
		failures := len(reSuitesFail.FindAll(output, -1))
		total := passes + failures
		if total != len(reuseFiles) {
			t.Errorf("JUnit output had %d testsuite elements; expected %d\n", total, len(reuseFiles))
		}
		if passes != reuseCount[0] {
			t.Errorf("JUnit output had %d passes; expected %d\n", passes, reuseCount[0])
		}
		if failures != reuseCount[1] {
			t.Errorf("JUnit output had %d failures; expected %d\n", failures, reuseCount[1])
		}
		cases := reCases.FindAll(output, -1)
		if len(cases) < total {
			t.Errorf("JUnit output had %d suites without test cases\n", total-len(cases))
		}
	})
}
