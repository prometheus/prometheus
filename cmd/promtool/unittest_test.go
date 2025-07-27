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
	"encoding/xml"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/util/junitxml"
)

func TestRulesUnitTest(t *testing.T) {
	t.Parallel()

	type args struct {
		files []string
	}

	tests := []struct {
		name string

		args args

		queryOpts promqltest.LazyLoaderOpts

		want int
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

			queryOpts: promqltest.LazyLoaderOpts{
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

			queryOpts: promqltest.LazyLoaderOpts{
				EnableNegativeOffset: true,
			},

			want: 0,
		},

		{
			name: "No test group interval",

			args: args{
				files: []string{"./testdata/no-test-group-interval.yml"},
			},

			queryOpts: promqltest.LazyLoaderOpts{
				EnableNegativeOffset: true,
			},

			want: 0,
		},
	}

	reuseFiles := []string{}

	reuseCount := [2]int{}

	for _, tt := range tests {

		if (tt.queryOpts == promqltest.LazyLoaderOpts{
			EnableNegativeOffset: true,
		} || tt.queryOpts == promqltest.LazyLoaderOpts{
			EnableAtModifier: true,
		}) {

			reuseFiles = append(reuseFiles, tt.args.files...)

			reuseCount[tt.want] += len(tt.args.files)

		}

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := RulesUnitTest(tt.queryOpts, nil, false, false, false, tt.args.files...); got != tt.want {
				t.Errorf("RulesUnitTest() = %v, want %v", got, tt.want)
			}
		})

	}

	t.Run("Junit xml output ", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		if got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, nil, false, false, false, false, "text", "", 0.0, reuseFiles...); got != 1 {
			t.Errorf("RulesUnitTestResults() = %v, want 1", got)
		}

		var test junitxml.JUnitXML

		output := buf.Bytes()

		err := xml.Unmarshal(output, &test)
		if err != nil {

			fmt.Println("error in decoding XML:", err)

			return

		}

		var total int

		var passes int

		var failures int

		var cases int

		total = len(test.Suites)

		if total != len(reuseFiles) {
			t.Errorf("JUnit output had %d testsuite elements; expected %d\n", total, len(reuseFiles))
		}

		for _, i := range test.Suites {

			if i.FailureCount == 0 {
				passes++
			} else {
				failures++
			}

			cases += len(i.Cases)

		}

		if total != passes+failures {
			t.Errorf("JUnit output mismatch: Total testsuites (%d) does not equal the sum of passes (%d) and failures (%d).", total, passes, failures)
		}

		if cases < total {
			t.Errorf("JUnit output had %d suites without test cases\n", total-cases)
		}
	})
}

func TestRulesUnitTestRun(t *testing.T) {
	t.Parallel()

	type args struct {
		run []string

		files []string
	}

	tests := []struct {
		name string

		args args

		queryOpts promqltest.LazyLoaderOpts

		want int

		ignoreUnknownFields bool
	}{
		{
			name: "Test all without run arg",

			args: args{
				run: nil,

				files: []string{"./testdata/rules_run.yml"},
			},

			want: 1,
		},

		{
			name: "Test all with run arg",

			args: args{
				run: []string{"correct", "wrong"},

				files: []string{"./testdata/rules_run.yml"},
			},

			want: 1,
		},

		{
			name: "Test correct",

			args: args{
				run: []string{"correct"},

				files: []string{"./testdata/rules_run.yml"},
			},

			want: 0,
		},

		{
			name: "Test wrong",

			args: args{
				run: []string{"wrong"},

				files: []string{"./testdata/rules_run.yml"},
			},

			want: 1,
		},

		{
			name: "Test all with extra fields",

			args: args{
				files: []string{"./testdata/rules_run_extrafields.yml"},
			},

			ignoreUnknownFields: true,

			want: 0,
		},

		{
			name: "Test precise floating point comparison expected failure",

			args: args{
				files: []string{"./testdata/rules_run_no_fuzzy.yml"},
			},

			want: 1,
		},

		{
			name: "Test fuzzy floating point comparison correct match",

			args: args{
				run: []string{"correct"},

				files: []string{"./testdata/rules_run_fuzzy.yml"},
			},

			want: 0,
		},

		{
			name: "Test fuzzy floating point comparison wrong match",

			args: args{
				run: []string{"wrong"},

				files: []string{"./testdata/rules_run_fuzzy.yml"},
			},

			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := RulesUnitTest(tt.queryOpts, tt.args.run, false, false, tt.ignoreUnknownFields, tt.args.files...)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestRulesUnitTestCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string

		files []string

		coverage bool

		want int
	}{
		{
			name: "Coverage enabled",

			files: []string{"./testdata/unittest.yml"},

			coverage: true,

			want: 0,
		},

		{
			name: "Coverage disabled",

			files: []string{"./testdata/unittest.yml"},

			coverage: false,

			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, nil, false, false, false, tt.coverage, "text", "", 0.0, tt.files...)

			require.Equal(t, tt.want, got)

			// The coverage output goes to stdout/stderr in the current implementation

			// We just verify that the function runs without error when coverage is enabled
		})
	}
}

func TestCoverageTracker(t *testing.T) {
	t.Parallel()

	tracker := newCoverageTracker()

	require.NotNil(t, tracker)

	require.Empty(t, tracker.ruleFiles)

	require.Empty(t, tracker.allRules)

	require.Empty(t, tracker.testedRules)

	require.Empty(t, tracker.warnings)

	// Add a rule manually to test markRuleTested

	tracker.allRules["test.yml"] = make(map[string]*ruleInfo)

	tracker.testedRules["test.yml"] = make(map[string]bool)

	tracker.allRules["test.yml"]["test_rule"] = &ruleInfo{
		Name: "test_rule",

		Type: "recording",
	}

	tracker.markRuleTested("test_rule", "test_case")

	require.Contains(t, tracker.testCaseMapping, "test_rule")

	require.Equal(t, []string{"test_case"}, tracker.testCaseMapping["test_rule"])

	require.True(t, tracker.testedRules["test.yml"]["test_rule"])
}

func TestCoverageOutputFormats(t *testing.T) {
	t.Parallel()

	report := &coverageReport{
		TotalRules: 5,

		TestedRules: 4,

		Coverage: 80.0,

		Files: map[string]*fileCoverageInfo{
			"test.yml": {
				File: "test.yml",

				TotalRules: 5,

				TestedRules: 4,

				Coverage: 80.0,

				Rules: map[string]*ruleCoverageInfo{
					"rule1": {
						Name: "rule1",

						Type: "recording",

						Tested: true,

						TestCases: []string{"test1"},
					},
				},
			},
		},
	}

	tests := []struct {
		name string

		format string
	}{
		{"Text format", "text"},

		{"JSON format", "json"},

		{"JUnit XML format", "junit-xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := outputCoverageReport(report, tt.format, "")

			require.NoError(t, err)
		})
	}
}
