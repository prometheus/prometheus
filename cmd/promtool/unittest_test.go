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
	"fmt"
	"github.com/prometheus/prometheus/promql"
	"regexp"
	"testing"
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

	tapFiles := []string{}
	tapCount := [2]int{}
	for _, tt := range tests {
		// Reuse these tests for the tap output testing, but only ones with default opts.
		if (tt.queryOpts == promql.LazyLoaderOpts{}) {
			tapFiles = append(tapFiles, tt.args.files...)
			tapCount[tt.want] += len(tt.args.files)
		}
		t.Run(tt.name, func(t *testing.T) {
			if got := RulesUnitTest(tt.queryOpts, tt.args.files...); got != tt.want {
				t.Errorf("RulesUnitTest() = %v, want %v", got, tt.want)
			}
		})
	}

	rePlan := regexp.MustCompile(fmt.Sprintf("^1..%d", len(tapFiles)))
	rePass := regexp.MustCompile(`(?m)^ok \d+ -`)
	reFail := regexp.MustCompile(`(?m)^not ok \d+ -`)

	t.Run("TAP output", func(t *testing.T) {
		var tapout bytes.Buffer
		if got := RulesUnitTestTap(&tapout, promql.LazyLoaderOpts{}, tapFiles...); got != 1 {
			t.Errorf("RulesUnitTestTap() = %v, want 1", got)
		}

		text := tapout.Bytes()
		if !rePlan.Match(text) {
			t.Errorf("TAP output has no plan\n")
		}
		passes := rePass.FindAll(text, -1)
		if len(passes) != tapCount[0] {
			t.Errorf("TAP output had %d ok; expected %d\n", len(passes), tapCount[0])
		}
		fails := reFail.FindAll(text, -1)
		if len(fails) != tapCount[1] {
			t.Errorf("TAP output had %d not ok; expected %d\n", len(fails), tapCount[1])
		}
	})
}
