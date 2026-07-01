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
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules"
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
		if got := RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, parser.NewParser(parser.Options{}), nil, false, false, false, false, 0, reuseFiles...); got != 1 {
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

// recordCoverage builds a ruleCoverage from the given unit test files, mirroring
// how ruleUnitTest feeds the coverage tracker, so coverage can be asserted in
// isolation from the test run.
func recordCoverage(t *testing.T, p parser.Parser, files ...string) *ruleCoverage {
	t.Helper()
	cov := newRuleCoverage()
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		var utf unitTestFile
		require.NoError(t, yaml.UnmarshalStrict(b, &utf))
		require.NoError(t, resolveAndGlobFilepaths(filepath.Dir(f), &utf))
		if utf.EvaluationInterval == 0 {
			utf.EvaluationInterval = model.Duration(time.Minute)
		}
		cov.record(p, nil, false, &utf)
	}
	return cov
}

// coverageCounts returns the number of covered and total rules tracked.
func coverageCounts(cov *ruleCoverage) (covered, total int) {
	total = len(cov.order)
	for _, k := range cov.order {
		if cov.covered[k] {
			covered++
		}
	}
	return covered, total
}

func TestRulesUnitTestCoverage(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})

	// Tests pass and coverage is informational only: exit code 0.
	var buf bytes.Buffer
	require.Equal(t, 0, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, true, 0, "./testdata/coverage_test.yml"))

	// coverage_test.yml exercises 3 of 5 rules (60%). A threshold above it fails,
	// a threshold at or below it passes. coverage=false with a positive threshold
	// still enables coverage and gates the exit code.
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, "./testdata/coverage_test.yml"))
	require.Equal(t, 0, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 60, "./testdata/coverage_test.yml"))
}

func TestRuleCoverageModel(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, "./testdata/coverage_test.yml")

	covered, total := coverageCounts(cov)
	require.Equal(t, 5, total, "coverage_rules.yml has 3 alerts + 2 recording rules")
	require.Equal(t, 3, covered)

	var buf bytes.Buffer
	require.InEpsilon(t, 60.0, cov.report(&buf), 0.0001)
	out := buf.String()
	require.Contains(t, out, "Alerting rules:  2/3 covered")
	require.Contains(t, out, "Recording rules: 1/2 covered")
	require.Contains(t, out, "Total:           3/5 covered")
	require.Contains(t, out, "- alert: NeverTested")
	require.Contains(t, out, "- record: job:up:avg")
}

// TestRuleCoverageDeduplicatesAcrossFiles covers the bug where a rule file shared
// by multiple test files was counted once per referencing file, inflating the
// denominator and listing the same rule as both covered and untested.
func TestRuleCoverageDeduplicatesAcrossFiles(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, "./testdata/coverage_shared_a_test.yml", "./testdata/coverage_shared_b_test.yml")

	covered, total := coverageCounts(cov)
	require.Equal(t, 1, total, "the shared rule must be counted exactly once")
	require.Equal(t, 1, covered, "it is covered because one test file exercises it")
}

// TestRuleCoverageDistinguishesDuplicateNames covers the bug where rules sharing a
// name in different groups were conflated. They must be counted as distinct rules
// and attributed individually: the two same-named alerts are both covered by
// promtool's union evaluation, while the label-scoped query covers only one of the
// two same-named recording rules.
func TestRuleCoverageDistinguishesDuplicateNames(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, "./testdata/coverage_dupname_test.yml")

	covered, total := coverageCounts(cov)
	require.Equal(t, 4, total, "two same-named alerts and two same-named records are four distinct rules")
	require.Equal(t, 3, covered, "both alerts plus only the tier=a recording rule")

	var buf bytes.Buffer
	cov.report(&buf)
	out := buf.String()
	require.Contains(t, out, `group "groupB"`)
	require.Contains(t, out, "- record: dup:metric")
	require.NotContains(t, out, `group "groupA"`)
	require.NotContains(t, out, "SameName")
}

// TestRuleCoverageHonorsRunFilter ensures coverage only counts assertions from
// test groups selected by --run, so a filtered run cannot report rules as covered
// that this invocation never exercised.
func TestRuleCoverageHonorsRunFilter(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	b, err := os.ReadFile("./testdata/coverage_test.yml")
	require.NoError(t, err)
	var utf unitTestFile
	require.NoError(t, yaml.UnmarshalStrict(b, &utf))
	require.NoError(t, resolveAndGlobFilepaths("./testdata", &utf))

	cov := newRuleCoverage()
	cov.record(p, regexp.MustCompile("NoSuchGroup"), false, &utf)

	covered, total := coverageCounts(cov)
	require.Equal(t, 5, total, "all rules are still counted in the denominator")
	require.Equal(t, 0, covered, "no test group ran, so nothing is covered")
}

// TestRuleCoverageThresholdValidation rejects out-of-range thresholds.
func TestRuleCoverageThresholdValidation(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, -1, "./testdata/coverage_test.yml"))
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 101, "./testdata/coverage_test.yml"))
}

// TestRuleCoverageThresholdRounding checks the gate compares the displayed
// (one-decimal) percentage, so a threshold equal to the reported value passes.
func TestRuleCoverageThresholdRounding(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	// 2/3 displays as 66.7%; a threshold equal to it passes, just above it fails.
	require.Equal(t, 0, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 66.7, "./testdata/coverage_twothirds_test.yml"))
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 66.8, "./testdata/coverage_twothirds_test.yml"))
}

// TestBelowThreshold locks the gate to the displayed (one-decimal) percentage,
// including half-even rounding ties where naive rounding would disagree with the
// printed value. For example 6.25 prints as "6.2", so it is below 6.3.
func TestBelowThreshold(t *testing.T) {
	t.Parallel()

	require.False(t, belowThreshold(0, 0), "a zero threshold disables the check")
	require.False(t, belowThreshold(50, 0))
	require.True(t, belowThreshold(60, 100))
	require.False(t, belowThreshold(60, 60), "comparison is strictly less-than")
	require.False(t, belowThreshold(60, 50))

	// 6.25 displays as 6.2 (round half to even); the gate must match the display.
	require.True(t, belowThreshold(6.25, 6.3))
	require.False(t, belowThreshold(6.25, 6.2))
	// 31.25 displays as 31.2.
	require.True(t, belowThreshold(31.25, 31.3))
	require.False(t, belowThreshold(31.25, 31.2))
}

// anySelectorMatches parses expr, extracts its selectors, and reports whether any
// of them satisfies the given predicate.
func anySelectorMatches(t *testing.T, p parser.Parser, expr string, pred func([]*labels.Matcher) bool) bool {
	t.Helper()
	parsed, err := p.ParseExpr(expr)
	require.NoError(t, err)
	return slices.ContainsFunc(parser.ExtractSelectors(parsed), pred)
}

func TestSelectorCoversRecordingRule(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	tests := []struct {
		name    string
		rule    *rules.RecordingRule
		expr    string
		matches bool
	}{
		{
			name:    "direct match",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.EmptyLabels()),
			expr:    "job:up:sum",
			matches: true,
		},
		{
			name:    "aggregation wrapping",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.EmptyLabels()),
			expr:    `sum by (job)(job:up:sum)`,
			matches: true,
		},
		{
			name:    "no match",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.EmptyLabels()),
			expr:    "other_metric",
			matches: false,
		},
		{
			name:    "wildcard selector without __name__ is indeterminate",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.EmptyLabels()),
			expr:    `{job="prometheus"}`,
			matches: false,
		},
		{
			name:    "compatible static label",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.FromStrings("team", "infra")),
			expr:    `job:up:sum{team="infra"}`,
			matches: true,
		},
		{
			name:    "incompatible static label",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.FromStrings("team", "infra")),
			expr:    `job:up:sum{team="backend"}`,
			matches: false,
		},
		{
			name:    "matcher on label absent from static labels is conservatively matched",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.EmptyLabels()),
			expr:    `job:up:sum{job="prometheus"}`,
			matches: true,
		},
		{
			name:    "scalar expression has no selectors",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.EmptyLabels()),
			expr:    "vector(1)",
			matches: false,
		},
		{
			name:    "ALERTS selector does not cover a recording rule",
			rule:    rules.NewRecordingRule("job:up:sum", nil, labels.EmptyLabels()),
			expr:    `ALERTS{alertname="job:up:sum"}`,
			matches: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.matches, anySelectorMatches(t, p, tc.expr, func(ms []*labels.Matcher) bool {
				return selectorCoversRecordingRule(tc.rule, ms)
			}))
		})
	}
}

func TestSelectorCoversAlertingRule(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	tests := []struct {
		name       string
		alertName  string
		ruleLabels labels.Labels
		expr       string
		matches    bool
	}{
		{
			name:       "ALERTS with matching alertname",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `count(ALERTS{alertname="InstanceDown"})`,
			matches:    true,
		},
		{
			name:       "ALERTS_FOR_STATE with matching alertname",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS_FOR_STATE{alertname="InstanceDown"}`,
			matches:    true,
		},
		{
			name:       "ALERTS with wrong alertname",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `count(ALERTS{alertname="OtherAlert"})`,
			matches:    false,
		},
		{
			name:       "ALERTS without alertname is indeterminate",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `count(ALERTS)`,
			matches:    false,
		},
		{
			name:       "ALERTS with compatible static label",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("severity", "page"),
			expr:       `ALERTS{alertname="InstanceDown", severity="page"}`,
			matches:    true,
		},
		{
			name:       "ALERTS with incompatible static label",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("severity", "page"),
			expr:       `ALERTS{alertname="InstanceDown", severity="critical"}`,
			matches:    false,
		},
		{
			name:       "direct alert name does not cover an alerting rule",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `InstanceDown`,
			matches:    false,
		},
		{
			name:       "recording-style selector does not cover an alerting rule",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `job:up:sum`,
			matches:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.matches, anySelectorMatches(t, p, tc.expr, func(ms []*labels.Matcher) bool {
				return selectorCoversAlertingRule(tc.alertName, tc.ruleLabels, ms)
			}))
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
