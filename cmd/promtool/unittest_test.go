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
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
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
		name      string
		args      args
		queryOpts promqltest.LazyLoaderOpts
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
		if got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, nil, false, false, false, false, "junit-xml", "", 0, reuseFiles...); got != 1 {
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
		run   []string
		files []string
	}
	tests := []struct {
		name                string
		args                args
		queryOpts           promqltest.LazyLoaderOpts
		want                int
		ignoreUnknownFields bool
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
		{
			name: "Test all with extra fields",
			args: args{
				files: []string{"./testdata/rules_run_extrafields.yml"},
			},
			ignoreUnknownFields: true,
			want:                0,
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
				run:   []string{"correct"},
				files: []string{"./testdata/rules_run_fuzzy.yml"},
			},
			want: 0,
		},
		{
			name: "Test fuzzy floating point comparison wrong match",
			args: args{
				run:   []string{"wrong"},
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

func TestCoverageReporting(t *testing.T) {
	t.Parallel()

	t.Run("Coverage Text Output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, nil, false, false, false, true, "text", "", 0, "./testdata/unittest.yml")
		require.Equal(t, 0, got)

		output := buf.String()
		require.Contains(t, output, "Overall coverage:")
		require.Contains(t, output, "Rule file coverage:")
		require.Contains(t, output, "Untested rules:")
		// Note: SUCCESS is printed to stderr, not captured in our buffer
	})

	t.Run("Coverage JSON Output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, nil, false, false, false, true, "json", "", 0, "./testdata/unittest.yml")
		require.Equal(t, 0, got)

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		// Find the JSON coverage report (look for the opening brace)
		var coverageJSON []string
		jsonStarted := false
		for _, line := range lines {
			if strings.HasPrefix(line, "{") {
				jsonStarted = true
				coverageJSON = []string{line}
			} else if jsonStarted && (strings.HasPrefix(line, "}") || strings.Contains(line, "]")) {
				coverageJSON = append(coverageJSON, line)
				if strings.HasPrefix(line, "}") {
					break
				}
			} else if jsonStarted {
				coverageJSON = append(coverageJSON, line)
			}
		}
		jsonStr := strings.Join(coverageJSON, "\n")
		require.NotEmpty(t, coverageJSON, "Coverage JSON report not found")

		var report struct {
			OverallCoverage float64                    `json:"overall_coverage"`
			TotalRules      int                        `json:"total_rules"`
			TestedRules     int                        `json:"tested_rules"`
			Files           map[string]interface{}     `json:"files"`
			TestResults     []TestResult               `json:"test_results"`
		}
		err := json.Unmarshal([]byte(jsonStr), &report)
		require.NoError(t, err)

		// Verify coverage report structure
		require.Equal(t, 5, report.TotalRules)
		require.Equal(t, 1, report.TestedRules)
		require.Equal(t, float64(20), report.OverallCoverage)
		require.Len(t, report.Files, 1) // Should have one rule file
		require.Len(t, report.TestResults, 5) // 5 test groups in unittest.yml

		// Verify test results structure
		for _, testResult := range report.TestResults {
			require.NotEmpty(t, testResult.Name)
			require.True(t, testResult.Passed) // All tests should pass
			require.Greater(t, testResult.Duration, int64(0))
			require.Empty(t, testResult.Error) // No errors expected
		}
	})

	t.Run("Coverage JUnit XML Output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, nil, false, false, false, true, "junit-xml", "", 0, "./testdata/unittest.yml")
		require.Equal(t, 0, got)

		output := buf.Bytes()
		var test junitxml.JUnitXML
		err := xml.Unmarshal(output, &test)
		require.NoError(t, err)

		// Should have 2 test suites: one for the test file and one for coverage
		require.Len(t, test.Suites, 2)

		// Find coverage suite
		var coverageSuite *junitxml.TestSuite
		for _, suite := range test.Suites {
			if suite.Name == "coverage" {
				coverageSuite = suite
				break
			}
		}
		require.NotNil(t, coverageSuite, "Coverage test suite not found")

		// Verify coverage property exists
		require.NotEmpty(t, coverageSuite.Properties)
		found := false
		for _, prop := range coverageSuite.Properties {
			if prop.Name == "coverage" {
				found = true
				require.Contains(t, prop.Value, "%")
				break
			}
		}
		require.True(t, found, "Coverage property not found in JUnit XML")
	})

	t.Run("Coverage With Failing Tests", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, nil, false, false, false, true, "json", "", 0, "./testdata/failing.yml")
		require.Equal(t, 1, got) // Should fail

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		// Find the JSON coverage report
		var coverageJSON []string
		jsonStarted := false
		for _, line := range lines {
			if strings.HasPrefix(line, "{") {
				jsonStarted = true
				coverageJSON = []string{line}
			} else if jsonStarted && (strings.HasPrefix(line, "}") || strings.Contains(line, "]")) {
				coverageJSON = append(coverageJSON, line)
				if strings.HasPrefix(line, "}") {
					break
				}
			} else if jsonStarted {
				coverageJSON = append(coverageJSON, line)
			}
		}
		jsonStr := strings.Join(coverageJSON, "\n")
		require.NotEmpty(t, coverageJSON, "Coverage JSON report not found")

		var report struct {
			TestResults []TestResult `json:"test_results"`
		}
		err := json.Unmarshal([]byte(jsonStr), &report)
		require.NoError(t, err)

		// Verify that failing tests are recorded correctly
		failingTests := 0
		for _, testResult := range report.TestResults {
			if !testResult.Passed {
				failingTests++
				require.NotEmpty(t, testResult.Error)
			}
		}
		require.Greater(t, failingTests, 0, "Expected at least one failing test")
	})
}

