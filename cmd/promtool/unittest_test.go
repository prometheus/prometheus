// Copyright The Prometheus Authors
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

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
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
		{
			name: "Start time tests",
			args: args{
				files: []string{"./testdata/start-time-test.yml"},
			},
			queryOpts: promqltest.LazyLoaderOpts{
				EnableAtModifier: true,
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
			if got := RulesUnitTest(tt.queryOpts, parser.NewParser(parser.Options{}), nil, false, false, false, tt.args.files...); got != tt.want {
				t.Errorf("RulesUnitTest() = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("Junit xml output ", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		if got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, parser.NewParser(parser.Options{}), nil, false, false, false, false, reuseFiles...); got != 1 {
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

func TestRulesUnitTestCoverage(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})

	// End-to-end: run RulesUnitTestResult with coverage=true through the full test runner.
	// Coverage summary is printed to stderr; we verify the return code here and the
	// coverage logic through loadRuleEntries + ruleMatchesSelector tests below.
	var buf bytes.Buffer
	got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, true, "./testdata/coverage_test.yml")
	require.Equal(t, 0, got, "expected tests to pass with coverage enabled")

	// Verify loadRuleEntries correctly discovers all rules from the test fixture.
	entries := loadRuleEntries("./testdata/coverage_test.yml", p, false)
	require.Len(t, entries, 5, "coverage_rules.yml has 3 alerts + 2 recording rules")

	// Verify rule types.
	alertCount := 0
	recordCount := 0
	for _, e := range entries {
		switch e.rtype {
		case "alert":
			alertCount++
		case "record":
			recordCount++
		}
	}
	require.Equal(t, 3, alertCount)
	require.Equal(t, 2, recordCount)
}

func TestRuleMatchesSelector(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})

	tests := []struct {
		name    string
		entry   ruleEntry
		expr    string
		matches bool
	}{
		{
			name:    "direct recording rule match",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record"},
			expr:    "job:up:sum",
			matches: true,
		},
		{
			name:    "recording rule with aggregation",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record"},
			expr:    `sum by (job)(job:up:sum)`,
			matches: true,
		},
		{
			name:    "recording rule no match",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record"},
			expr:    "other_metric",
			matches: false,
		},
		{
			name:    "ALERTS meta-metric with alertname",
			entry:   ruleEntry{name: "InstanceDown", rtype: "alert"},
			expr:    `count(ALERTS{alertname="InstanceDown"})`,
			matches: true,
		},
		{
			name:    "ALERTS meta-metric wrong alertname",
			entry:   ruleEntry{name: "InstanceDown", rtype: "alert"},
			expr:    `count(ALERTS{alertname="OtherAlert"})`,
			matches: false,
		},
		{
			name:    "ALERTS without alertname is indeterminate",
			entry:   ruleEntry{name: "InstanceDown", rtype: "alert"},
			expr:    `count(ALERTS)`,
			matches: false,
		},
		{
			name:    "wildcard selector without __name__ is indeterminate",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record"},
			expr:    `{job="prometheus"}`,
			matches: false,
		},
		{
			name:    "label match with compatible static labels",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record", labels: labels.FromStrings("team", "infra")},
			expr:    `job:up:sum{team="infra"}`,
			matches: true,
		},
		{
			name:    "label mismatch with incompatible static labels",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record", labels: labels.FromStrings("team", "infra")},
			expr:    `job:up:sum{team="backend"}`,
			matches: false,
		},
		{
			name:    "label selector on label not in static labels is conservatively matched",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record"},
			expr:    `job:up:sum{job="prometheus"}`,
			matches: true,
		},
		{
			name:    "ALERTS with compatible static labels",
			entry:   ruleEntry{name: "InstanceDown", rtype: "alert", labels: labels.FromStrings("severity", "page")},
			expr:    `count(ALERTS{alertname="InstanceDown", severity="page"})`,
			matches: true,
		},
		{
			name:    "ALERTS with incompatible static labels",
			entry:   ruleEntry{name: "InstanceDown", rtype: "alert", labels: labels.FromStrings("severity", "page")},
			expr:    `count(ALERTS{alertname="InstanceDown", severity="critical"})`,
			matches: false,
		},
		{
			name:    "alert rule not matched by direct __name__",
			entry:   ruleEntry{name: "InstanceDown", rtype: "alert"},
			expr:    `InstanceDown`,
			matches: false,
		},
		{
			name:    "scalar expression has no selectors",
			entry:   ruleEntry{name: "job:up:sum", rtype: "record"},
			expr:    "vector(1)",
			matches: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsedExpr, err := p.ParseExpr(tc.expr)
			require.NoError(t, err)
			selectors := parser.ExtractSelectors(parsedExpr)
			matched := false
			for _, matchers := range selectors {
				if ruleMatchesSelector(&tc.entry, matchers) {
					matched = true
					break
				}
			}
			require.Equal(t, tc.matches, matched)
		})
	}
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
			got := RulesUnitTest(tt.queryOpts, parser.NewParser(parser.Options{}), tt.args.run, false, false, tt.ignoreUnknownFields, tt.args.files...)
			require.Equal(t, tt.want, got)
		})
	}
}
