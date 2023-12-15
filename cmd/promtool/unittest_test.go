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
	"testing"

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
		{
			name: "No test group interval",
			args: args{
				files: []string{"./testdata/no-test-group-interval.yml"},
			},
			queryOpts: promql.LazyLoaderOpts{
				EnableNegativeOffset: true,
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RulesUnitTest(tt.queryOpts, nil, tt.args.files...); got != tt.want {
				t.Errorf("RulesUnitTest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRulesUnitTestRun(t *testing.T) {
	type args struct {
		run   []string
		files []string
	}
	tests := []struct {
		name      string
		args      args
		queryOpts promql.LazyLoaderOpts
		want      int
	}{
		{
			name: "Test all without run arg",
			args: args{
				run:   nil,
				files: []string{"./testdata/rules_run.yml"},
			},
			want: 1,
		},
		{
			name: "Test all with run arg",
			args: args{
				run:   []string{"correct", "wrong"},
				files: []string{"./testdata/rules_run.yml"},
			},
			want: 1,
		},
		{
			name: "Test correct",
			args: args{
				run:   []string{"correct"},
				files: []string{"./testdata/rules_run.yml"},
			},
			want: 0,
		},
		{
			name: "Test wrong",
			args: args{
				run:   []string{"wrong"},
				files: []string{"./testdata/rules_run.yml"},
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RulesUnitTest(tt.queryOpts, tt.args.run, tt.args.files...); got != tt.want {
				t.Errorf("RulesUnitTest() = %v, want %v", got, tt.want)
			}
		})
	}
}
