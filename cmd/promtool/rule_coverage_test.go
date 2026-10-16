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
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/util/junitxml"
)

// recordCoverage builds a ruleCoverage from the given unit test files, mirroring
// how ruleUnitTest feeds the coverage tracker, so coverage can be asserted in
// isolation from the test run.
func recordCoverage(t *testing.T, p parser.Parser, files ...string) *ruleCoverage {
	t.Helper()
	return recordCoverageWithRun(t, p, nil, files...)
}

// recordCoverageWithRun is recordCoverage restricted to the test groups selected
// by run, mirroring the --run flag.
func recordCoverageWithRun(t *testing.T, p parser.Parser, run *regexp.Regexp, files ...string) *ruleCoverage {
	t.Helper()
	cov := newRuleCoverage(p, false)
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		var utf unitTestFile
		require.NoError(t, yaml.UnmarshalStrict(b, &utf))
		require.NoError(t, resolveAndGlobFilepaths(filepath.Dir(f), &utf))
		if utf.EvaluationInterval == 0 {
			utf.EvaluationInterval = model.Duration(time.Minute)
		}
		cov.record(run, &utf)
	}
	return cov
}

// coverageCounts returns the number of covered and total rules tracked.
func coverageCounts(cov *ruleCoverage) (covered, total int) {
	total = len(cov.order)
	for _, k := range cov.order {
		if cov.entries[k].state == coverageCovered {
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
	require.Equal(t, coverageStats{Covered: 3, Total: 5}, cov.report(&buf))
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
	cov := recordCoverageWithRun(t, p, regexp.MustCompile("NoSuchGroup"), "./testdata/coverage_test.yml")

	covered, total := coverageCounts(cov)
	require.Equal(t, 5, total, "all rules are still counted in the denominator")
	require.Equal(t, 0, covered, "no test group ran, so nothing is covered")
}

// TestRuleCoverageMultiDocumentRuleFile checks that a rule file the loader warns
// about is still analysed. The coverage manager must supply its own logger, since
// the loader logs such a warning while loading.
func TestRuleCoverageMultiDocumentRuleFile(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	require.NotPanics(t, func() {
		cov := recordCoverage(t, p, "./testdata/coverage_multidoc_test.yml")
		_, total := coverageCounts(cov)
		require.Equal(t, 1, total, "the first document's rule is still counted")
		require.Empty(t, cov.loadFailures)
	})

	var buf bytes.Buffer
	require.Equal(t, 0, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, true, 0, "./testdata/coverage_multidoc_test.yml"))
}

// TestRuleCoverageFailsClosedOnLoadErrors covers the fail-open bug where an
// unparseable rule file was dropped from the denominator. With no test group to
// run, nothing else loaded it, so the report became 0/0 and passed a threshold of
// 100 without analysing the suite at all.
func TestRuleCoverageFailsClosedOnLoadErrors(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})

	for _, tc := range []struct {
		name string
		file string
		run  []string
	}{
		{name: "no test groups", file: "./testdata/coverage_broken_test.yml"},
		{name: "no test group selected by --run", file: "./testdata/coverage_broken_run_test.yml", run: []string{"NoSuchGroup"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, tc.run, false, false, false, false, 100, tc.file),
				"an unanalysable suite must not satisfy a coverage threshold")

			// Report-only mode surfaces incomplete coverage as a warning but keeps
			// the informational --coverage exit code. Only a positive threshold
			// turns the same condition into a failure.
			cov := recordCoverageWithRun(t, p, nil, tc.file)
			require.Len(t, cov.loadFailures, 1, "one rule file failed, however many errors it produced")
			var report bytes.Buffer
			require.Empty(t, cov.reportAndGate(&report, 0))
			require.Contains(t, report.String(), "excluded from coverage")
		})
	}
}

// TestRuleCoverageDenominatorIsWhatTheSuiteReferences pins the operational limit
// of the feature: coverage only sees rule files the given test files reach. A
// rule file nobody wrote a test file for is outside the suite entirely, so it
// cannot lower the percentage. The documentation says so, because a threshold
// meant to catch an untested rule would not catch an untested rule file.
func TestRuleCoverageDenominatorIsWhatTheSuiteReferences(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tested.yml"), []byte(
		"groups:\n  - name: g\n    rules:\n      - record: r\n        expr: up\n"), 0o600))
	// Present on disk, referenced by nothing.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untested.yml"), []byte(
		"groups:\n  - name: other\n    rules:\n      - alert: NeverSeen\n        expr: up == 0\n"), 0o600))

	testFile := filepath.Join(dir, "test.yml")
	require.NoError(t, os.WriteFile(testFile, []byte(
		"rule_files:\n  - tested.yml\n\nevaluation_interval: 1m\n\ntests:\n"+
			"  - input_series:\n      - series: 'up{job=\"a\"}'\n        values: '1'\n"+
			"    promql_expr_test:\n      - expr: r\n        eval_time: 0m\n"+
			"        exp_samples:\n          - labels: 'r{job=\"a\"}'\n            value: 1\n"), 0o600))

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, testFile)
	covered, total := coverageCounts(cov)
	require.Equal(t, 1, total, "the unreferenced rule file is not part of the suite")
	require.Equal(t, 1, covered)
	require.Empty(t, cov.loadFailures, "an unreferenced file is not a load failure either")

	require.Equal(t, 0, RulesUnitTestResult(io.Discard, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, testFile))
}

// TestRuleCoverageUnmatchedRuleFile covers the fail-open case where a rule_files
// entry matching nothing was warned about and then dropped. The rules the missing
// file would have contributed never reach the denominator, so a suite whose
// remaining files happen to be fully covered used to satisfy a threshold of 100.
func TestRuleCoverageUnmatchedRuleFile(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, "./testdata/coverage_missing_test.yml"),
		"a partially resolved rule_files list must not satisfy a coverage threshold")

	// The rules that were found are still counted, and still fully covered: the
	// denominator alone cannot reveal the gap.
	cov := recordCoverage(t, p, "./testdata/coverage_missing_test.yml")
	covered, total := coverageCounts(cov)
	require.Equal(t, 1, total)
	require.Equal(t, 1, covered)
	require.Len(t, cov.loadFailures, 1)
	require.Contains(t, cov.reportAndGate(io.Discard, 100), "were not analysed")

	// Report-only mode keeps its exit code.
	require.Equal(t, 0, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, true, 0, "./testdata/coverage_missing_test.yml"))
}

// TestRuleCoverageThresholdRejectsUnreachableAlertState covers the fail-open case
// where every alerting rule was assumed to pass through pending. An alert with no
// "for" is promoted to firing before its first sample, so an assertion on its
// pending state passes while observing nothing, and used to satisfy a threshold.
func TestRuleCoverageThresholdRejectsUnreachableAlertState(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, "./testdata/coverage_nofor_test.yml"),
		"an assertion that can never observe the rule must not satisfy a threshold")

	cov := recordCoverage(t, p, "./testdata/coverage_nofor_test.yml")
	covered, total := coverageCounts(cov)
	require.Equal(t, 1, total)
	require.Equal(t, 0, covered)
}

// TestRuleCoverageThresholdRejectsEmptyStaticLabel covers the false green where
// rewriting a rule's labels to strip templates also dropped labels set to the
// empty string, since labels.Builder deletes those. The selector below can never
// observe the rule, but it looked compatible once severity had gone missing.
func TestRuleCoverageThresholdRejectsEmptyStaticLabel(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, "./testdata/coverage_emptylabel_test.yml"),
		"a selector that contradicts an empty static label must not satisfy a threshold")

	cov := recordCoverage(t, p, "./testdata/coverage_emptylabel_test.yml")
	covered, total := coverageCounts(cov)
	require.Equal(t, 1, total)
	require.Equal(t, 0, covered)
}

// TestRuleCoverageThresholdRejectsUnsatisfiableSelector is the same check for a
// selector whose matchers cannot hold at once, which likewise passes as a test
// while observing nothing.
func TestRuleCoverageThresholdRejectsUnsatisfiableSelector(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules.yml"), []byte(
		"groups:\n  - name: g\n    rules:\n      - record: r\n        expr: vector(1)\n"), 0o600))
	testFile := filepath.Join(dir, "test.yml")
	require.NoError(t, os.WriteFile(testFile, []byte(
		"rule_files:\n  - rules.yml\n\nevaluation_interval: 1m\n\ntests:\n"+
			"  - promql_expr_test:\n      - expr: 'r{job=~\"a.*\",job!~\"a.*\"}'\n"+
			"        eval_time: 0m\n        exp_samples: []\n"), 0o600))

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, testFile))
}

// TestRegexpCandidates checks that a satisfiable expression yields a value it
// accepts. Individual candidates may fail: a zero-width assertion can contradict
// the parts around it, which is why callers confirm them.
func TestRegexpCandidates(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		expr string
		// usable is false when nothing matches the expression at all, so no
		// candidate can be expected to either.
		usable bool
	}{
		{expr: "xyz.*", usable: true},
		{expr: ".*foo", usable: true},
		{expr: "(staging|prod)-.*", usable: true},
		{expr: "[a-z]+", usable: true},
		{expr: ".+", usable: true},
		{expr: "a|b", usable: true},
		{expr: ".*prod.*", usable: true},
		{expr: `job-\d+`, usable: true},
		{expr: "", usable: true},
		{expr: ".*", usable: true},
		{expr: "a.*", usable: true},
		{expr: "node_.*_total", usable: true},
		{expr: "(?i)Prod.*", usable: true},
		{expr: `\d{3}`, usable: true},
		// \B holds between two word characters, so this one is ordinary.
		{expr: `foo\Bbar`, usable: true},
		{expr: `\B`, usable: true},
		// \b cannot hold between "o" and "b", so nothing matches this.
		{expr: `foo\bbar`, usable: false},
		// A high-fanout element before the rest of the expression used to fill
		// the candidate budget and leave the remaining parts unapplied, so every
		// candidate came out too short to match.
		{expr: ".*[0-9][a-z]", usable: true},
		{expr: ".*.*prod", usable: true},
		{expr: ".*[a-z].*xyz", usable: true},
		{expr: ".*prod.*", usable: true},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()
			m, err := labels.NewMatcher(labels.MatchRegexp, "job", tc.expr)
			require.NoError(t, err)
			require.Equal(t, tc.usable, slices.ContainsFunc(testRegexpCandidates(tc.expr), m.Matches),
				"candidates for %q", tc.expr)
		})
	}
}

// TestRuleCoverageThresholdAcceptsAlternationSelector is the end-to-end guard for
// the other direction: a selector whose first alternation branch is excluded can
// still read the rule, and must not make a legitimate threshold fail.
func TestRuleCoverageThresholdAcceptsAlternationSelector(t *testing.T) {
	t.Parallel()

	requireCoveredEndToEnd(t, `r{job=~"(prod|foobar).*",job!~"prod.*"}`)
}

// TestRuleCoverageThresholdAcceptsNonWordBoundarySelector covers a zero-width
// assertion that was mistaken for an impossible expression. \B holds between two
// word characters, so foo\Bbar matches foobar and the assertion reads the rule.
func TestRuleCoverageThresholdAcceptsNonWordBoundarySelector(t *testing.T) {
	t.Parallel()

	requireCoveredEndToEnd(t, `r{job=~"foo\\Bbar"}`)
}

// requireCoveredEndToEnd runs a suite whose single assertion really reads the
// output of its single rule, and requires a threshold of 100 to pass. Writing it
// this way keeps the fixture honest: the selector has to observe the rule, not
// merely be attributable to it.
func requireCoveredEndToEnd(t *testing.T, selector string) {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules.yml"), []byte(
		"groups:\n  - name: g\n    rules:\n      - record: r\n        expr: up\n"), 0o600))

	testFile := filepath.Join(dir, "test.yml")
	require.NoError(t, os.WriteFile(testFile, []byte(
		"rule_files:\n  - rules.yml\n\nevaluation_interval: 1m\n\ntests:\n"+
			"  - input_series:\n"+
			"      - series: 'up{job=\"foobar\"}'\n"+
			"        values: '1'\n"+
			"    promql_expr_test:\n"+
			"      - expr: '"+selector+"'\n"+
			"        eval_time: 0m\n"+
			"        exp_samples:\n"+
			"          - labels: 'r{job=\"foobar\"}'\n"+
			"            value: 1\n"), 0o600))

	p := parser.NewParser(parser.Options{})
	require.Equal(t, 0, RulesUnitTestResult(io.Discard, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, testFile),
		"a selector that observes the rule must satisfy the threshold")
}

// TestMatchersSatisfiableAgainstOracle sweeps pairs of regexp matchers and checks
// the candidate search against brute force over a small alphabet. Whenever some
// value in the alphabet satisfies a pair, the search must find one too. This is
// the completeness half of the property; TestRegexpCandidates covers the rest.
func TestMatchersSatisfiableAgainstOracle(t *testing.T) {
	t.Parallel()

	alphabet := []string{
		"", "a", "b", "c", "A", "B", "0", "1", "-", "_", "\n", "\t", " ",
		"aa", "ab", "ba", "bb", "abc", "prod", "prod-", "staging", "staging-",
		"foo", "x", "y", "xy", "yx", "a-b", "z",
	}
	exprs := []string{
		"a.*", "b.*", ".*", ".", "[ab]+", "[a-z]+", "(staging|prod)-.*",
		"x?y?", "a|b", "xyz.*", "|a|A|0|-|aa", "[^a]+", "a+", "staging-.*",
		"[^\n]", "(a|b|c)", "a{1,2}", ".+", "prod|staging",
		`foo\\Bbar`, `\\B`, `\\Bfoo`, `foo\\bbar`,
	}
	types := []labels.MatchType{labels.MatchRegexp, labels.MatchNotRegexp}

	// Intersecting two unbounded expressions needs a value neither describes on
	// its own, which per-expression enumeration cannot reach.
	knownGaps := map[string]bool{
		`job=~".*foo",job=~"xyz.*"`: true,
		`job=~"xyz.*",job=~".*foo"`: true,
	}
	exprs = append(exprs, ".*foo")

	for _, first := range exprs {
		for _, second := range exprs {
			for _, firstType := range types {
				for _, secondType := range types {
					a, err := labels.NewMatcher(firstType, "job", first)
					require.NoError(t, err)
					b, err := labels.NewMatcher(secondType, "job", second)
					require.NoError(t, err)
					ms := []*labels.Matcher{a, b}

					witness, found := "", false
					for _, v := range alphabet {
						if allMatch(ms, v) {
							witness, found = v, true
							break
						}
					}
					if !found || knownGaps[a.String()+","+b.String()] {
						continue
					}
					require.True(t, (matcherSetSatisfiability(ms) == satisfiable),
						"%s and %s are satisfied by %q but the search found nothing", a, b, witness)
				}
			}
		}
	}
}

// TestMatcherSetSatisfiabilityStates checks the distinction the whole report
// rests on: a matcher set the search cannot decide must come back unknown, not
// unsatisfiable. Only a finite domain that was enumerated in full proves the
// negative.
func TestMatcherSetSatisfiabilityStates(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	for _, tc := range []struct {
		expr string
		want satisfiability
	}{
		{expr: `r{job=~"prom.*"}`, want: satisfiable},
		{expr: `r{job="a",job=~"a|b"}`, want: satisfiable},
		// Finite domains, enumerated in full, so the negative is proven.
		{expr: `r{job="a",job="b"}`, want: unsatisfiable},
		{expr: `r{job=~"a|b",job!~"a|b"}`, want: unsatisfiable},
		{expr: `r{job=~"a",job!~"a"}`, want: unsatisfiable},
		// Satisfiable, but the value is outside what the search builds.
		{expr: `r{job=~"a+",job!~"a|aa|aaa"}`, want: satisfiabilityUnknown},
		{expr: `r{job=~"[a-z]{2}",job!~"aa|az|za|zz"}`, want: satisfiabilityUnknown},
		// Genuinely impossible, but nothing pins the label, so it is not proven.
		{expr: `r{job!~".*"}`, want: satisfiabilityUnknown},
		// The budget runs out; that is a limit of the search, not a verdict.
		{expr: `r{job=~"[a-z]{1000}"}`, want: satisfiabilityUnknown},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, matcherSetSatisfiability(matchersFor(onlySelector(t, p, tc.expr), "job")))
		})
	}
}

// TestRuleCoverageReportsUndecidedSeparately checks that a rule the analysis
// could not decide is not presented as untested, and that a threshold still
// fails closed over it.
func TestRuleCoverageReportsUndecidedSeparately(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules.yml"), []byte(
		"groups:\n  - name: g\n    rules:\n      - record: r\n        expr: up\n"), 0o600))
	testFile := filepath.Join(dir, "test.yml")
	require.NoError(t, os.WriteFile(testFile, []byte(
		"rule_files:\n  - rules.yml\n\nevaluation_interval: 1m\n\ntests:\n"+
			"  - input_series:\n      - series: 'up{job=\"aaaa\"}'\n        values: '1'\n"+
			"    promql_expr_test:\n      - expr: 'r{job=~\"a+\",job!~\"a|aa|aaa\"}'\n"+
			"        eval_time: 0m\n        exp_samples:\n"+
			"          - labels: 'r{job=\"aaaa\"}'\n            value: 1\n"), 0o600))

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, testFile)

	var buf bytes.Buffer
	stats := cov.report(&buf)
	require.Equal(t, 1, stats.Total)
	require.Equal(t, 0, stats.Covered)
	require.Equal(t, 1, stats.Indeterminate, "the assertion reads the rule, but the search cannot show it")

	out := buf.String()
	require.Contains(t, out, "could not decide")
	require.NotContains(t, out, "Uncovered rules", "an undecided rule must not be called untested")

	// The gate still fails, but says why.
	reason := cov.reportAndGate(io.Discard, 100)
	require.Contains(t, reason, "could not be attributed either way")
	require.Equal(t, 1, RulesUnitTestResult(io.Discard, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, testFile))
}

// BenchmarkMatcherSetSatisfiability covers the shapes that used to be expensive:
// a counted repetition expands into a long concatenation, and the same selector
// is compared against every rule whose name matches.
func BenchmarkMatcherSetSatisfiability(b *testing.B) {
	p := parser.NewParser(parser.Options{})
	sel := func(expr string) []*labels.Matcher {
		parsed, err := p.ParseExpr(expr)
		require.NoError(b, err)
		return matchersFor(parser.ExtractSelectors(parsed)[0], "job")
	}
	for name, ms := range map[string][]*labels.Matcher{
		"equality":       sel(`r{job="a"}`),
		"regexp":         sel(`r{job=~"prom.*"}`),
		"countedRepeat":  sel(`r{job=~"[a-z]{1000}"}`),
		"largeExclusion": sel(`r{job!~"a|b|c|d|e|f|g|h|i|j|k|l|m|n|o|p|q|r|s|t|u|v|w|x|y|z"}`),
	} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				matcherSetSatisfiability(ms)
			}
		})
	}
}

// BenchmarkCompiledSelectorReuse covers the amplification the compiled selector
// removes: one expensive matcher set compared against many rules.
func BenchmarkCompiledSelectorReuse(b *testing.B) {
	p := parser.NewParser(parser.Options{})
	parsed, err := p.ParseExpr(`{__name__=~"r.*",job=~"[a-z]{1000}"}`)
	require.NoError(b, err)
	matchers := parser.ExtractSelectors(parsed)[0]

	for b.Loop() {
		sel := compileSelector(matchers)
		for i := range 200 {
			selectorCoversRecordingRule("r"+strconv.Itoa(i), labels.EmptyLabels(), sel)
		}
	}
}

// TestMatchersSatisfiableIsSound is the property the coverage gate rests on:
// matchersSatisfiable must never call a matcher set satisfiable without a value
// that actually satisfies it, because that is what would let an assertion which
// can never observe a rule count as covering it.
//
// Completeness is not asserted. The search is bounded by design, so the number of
// satisfiable sets it misses is reported rather than fixed, to keep the test from
// becoming a tripwire for every regexp the candidate builder cannot reach.
func TestMatchersSatisfiableIsSound(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewSource(20260820))
	types := []labels.MatchType{labels.MatchEqual, labels.MatchNotEqual, labels.MatchRegexp, labels.MatchNotRegexp}

	var checked, missed int
	for range 20000 {
		matchers := make([]*labels.Matcher, 0, 3)
		for range 1 + random.Intn(3) {
			matchType := types[random.Intn(len(types))]
			value := satisfiabilityFuzzValues[random.Intn(len(satisfiabilityFuzzValues))]
			if matchType == labels.MatchRegexp || matchType == labels.MatchNotRegexp {
				value = randomFuzzRegexp(random, 1+random.Intn(2))
			}
			// The generator can produce expressions the parser rejects, such as
			// nested repetition; those are not interesting here.
			m, err := labels.NewMatcher(matchType, "job", value)
			if err != nil {
				matchers = nil
				break
			}
			matchers = append(matchers, m)
		}
		if len(matchers) == 0 {
			continue
		}
		checked++

		result := matcherSetSatisfiability(matchers)
		if result == satisfiable {
			pinned, _ := pinnedValues(matchers)
			candidates := append(pinned, cheapWitnesses(matchers)...)
			candidates = append(candidates, regexpFuzzCandidates(matchers)...)
			require.True(t, slices.ContainsFunc(candidates, func(v string) bool { return allMatch(matchers, v) }),
				"%v was called satisfiable with no value that satisfies it", matchers)
			continue
		}
		if result == unsatisfiable {
			// Claiming impossible requires the enumeration to have been complete.
			_, exhaustive := pinnedValues(matchers)
			require.True(t, exhaustive,
				"%v was called unsatisfiable without a finite domain to enumerate", matchers)
		}
		if slices.ContainsFunc(satisfiabilityFuzzValues, func(v string) bool { return allMatch(matchers, v) }) {
			missed++
		}
	}
	t.Logf("%d matcher sets, no unsupported positives; the bounded search missed %d (%.3f%%)",
		checked, missed, 100*float64(missed)/float64(checked))
}

// satisfiabilityFuzzValues stand in for the label values a rule might produce.
var satisfiabilityFuzzValues = []string{
	"", "a", "b", "c", "A", "Z", "0", "9", "-", "_", ".", " ", "\n", "aa", "ab",
	"a-b", "a.b", "foo", "foobar", "prod", "prod-", "-prod", "staging-", "xyz",
	"xyzfoo", "x", "xy", "node_x_total", "測試", "--", "0x",
}

// randomFuzzRegexp builds an expression from the shapes that appear in rule test
// selectors: literals, classes, alternation, repetition and leading wildcards.
func randomFuzzRegexp(random *rand.Rand, depth int) string {
	atoms := []string{"a", "b", "foo", "prod", "xyz", "-", "_", "0", ".", "[ab]", "[a-z]", "[0-9]", "[^a]", `\d`, `\w`}
	if depth <= 0 {
		return atoms[random.Intn(len(atoms))]
	}
	switch random.Intn(7) {
	case 0:
		return randomFuzzRegexp(random, depth-1) + randomFuzzRegexp(random, depth-1)
	case 1:
		return "(" + randomFuzzRegexp(random, depth-1) + "|" + randomFuzzRegexp(random, depth-1) + ")"
	case 2:
		return randomFuzzRegexp(random, depth-1) + "*"
	case 3:
		return randomFuzzRegexp(random, depth-1) + "+"
	case 4:
		return randomFuzzRegexp(random, depth-1) + "?"
	case 5:
		return ".*" + randomFuzzRegexp(random, depth-1)
	default:
		return randomFuzzRegexp(random, depth-1) + ".*"
	}
}

// regexpFuzzCandidates returns the values the search would build from the
// required expressions of a matcher set.
func regexpFuzzCandidates(matchers []*labels.Matcher) []string {
	var out []string
	for _, m := range matchers {
		if m.Type == labels.MatchRegexp {
			out = append(out, testRegexpCandidates(m.GetRegexString())...)
		}
	}
	return out
}

// TestMatchersSatisfiableWitness checks the property the attribution relies on:
// whenever a matcher set is called satisfiable on the strength of a candidate
// value, some candidate really does satisfy every matcher.
func TestMatchersSatisfiableWitness(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	for _, expr := range []string{
		`r{job="a",job=~"a|b"}`,
		`r{job=~"a|b",job!~"b"}`,
		`r{job!="a"}`,
		`r{job!="",job!="a",job!="A",job!="0",job!="-"}`,
		`r{job!~"a"}`,
	} {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			ms := matchersFor(onlySelector(t, p, expr), "job")
			require.True(t, (matcherSetSatisfiability(ms) == satisfiable))

			candidates := append(exclusionWitnesses(ms), "a", "b", "aa")
			for _, m := range ms {
				if m.Type == labels.MatchEqual {
					candidates = append(candidates, m.Value)
				}
				candidates = append(candidates, m.SetMatches()...)
			}
			require.True(t, slices.ContainsFunc(candidates, func(v string) bool { return allMatch(ms, v) }),
				"a satisfiable matcher set must have a value that satisfies it")
		})
	}
}

// onlySelector parses expr and returns its single selector.
func onlySelector(t *testing.T, p parser.Parser, expr string) []*labels.Matcher {
	t.Helper()
	parsed, err := p.ParseExpr(expr)
	require.NoError(t, err)
	selectors := parser.ExtractSelectors(parsed)
	require.Len(t, selectors, 1)
	return selectors[0]
}

// TestRuleCoverageThresholdFailureIsInJUnit checks that a run failing only on
// coverage is not reported as all green by whatever consumes the JUnit output.
func TestRuleCoverageThresholdFailureIsInJUnit(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, "./testdata/coverage_test.yml"))

	var report junitxml.JUnitXML
	require.NoError(t, xml.Unmarshal(buf.Bytes(), &report))
	var failures int
	for _, suite := range report.Suites {
		failures += suite.FailureCount
	}
	require.NotZero(t, failures, "the coverage failure must appear in the JUnit output")
}

// TestRuleCoverageThresholdRequiresRules checks that a suite loading no rules
// cannot satisfy a threshold: an empty set is unknown, not fully covered.
func TestRuleCoverageThresholdRequiresRules(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 100, "./testdata/coverage_norules_test.yml"))

	// Without a threshold the empty suite is merely reported.
	require.Equal(t, 0, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, true, 0, "./testdata/coverage_norules_test.yml"))

	// The gate explains why the threshold could not be evaluated.
	cov := recordCoverage(t, p, "./testdata/coverage_norules_test.yml")
	_, total := coverageCounts(cov)
	require.Equal(t, 0, total)
	require.Contains(t, cov.reportAndGate(io.Discard, 100), "no rules were loaded")
	require.Empty(t, cov.reportAndGate(io.Discard, 0))
}

// TestRuleCoverageDistinguishesRuleDeclarations covers the bug where rules were
// identified by content, so two declarations differing only in an untracked field
// such as "for" collapsed into one and the denominator lost a rule.
func TestRuleCoverageDistinguishesRuleDeclarations(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, "./testdata/coverage_ordinal_test.yml")

	covered, total := coverageCounts(cov)
	require.Equal(t, 2, total, "two declarations are two rules even when they look alike")
	require.Equal(t, 0, covered)

	var buf bytes.Buffer
	cov.report(&buf)
	out := buf.String()
	require.Equal(t, 2, strings.Count(out, "- alert: Twin"), "both declarations must be listed")
	// Their name, expression and labels are identical, so only the position tells
	// the reader which declaration each line is about.
	require.Contains(t, out, "(rule #1)")
	require.Contains(t, out, "(rule #2)")
}

// TestRuleCoverageReportIdentifiesPartiallyCoveredDeclarations checks the case the
// ordinal exists for: one of two lookalike declarations is covered, and the report
// has to say which one still needs a test.
func TestRuleCoverageReportIdentifiesPartiallyCoveredDeclarations(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, "./testdata/coverage_ordinal_partial_test.yml")

	covered, total := coverageCounts(cov)
	require.Equal(t, 2, total)
	require.Equal(t, 1, covered, "only the declaration with a hold duration reaches pending")

	var buf bytes.Buffer
	cov.report(&buf)
	out := buf.String()
	require.Equal(t, 1, strings.Count(out, "- alert: Twin"))
	require.Contains(t, out, "(rule #1)", "the declaration without a for is the untested one")
	require.NotContains(t, out, "(rule #2)")
}

// TestRuleCoverageReportsDistinguishablePaths checks that rule files sharing a
// base name stay distinguishable in the report, which listed only the base name.
func TestRuleCoverageReportsDistinguishablePaths(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, "./testdata/coverage_paths_test.yml")

	_, total := coverageCounts(cov)
	require.Equal(t, 2, total, "same-named rules in different files are distinct")

	var buf bytes.Buffer
	cov.report(&buf)
	out := buf.String()
	require.Contains(t, out, filepath.Join("testdata", "coverage_paths", "a", "rules.yml"))
	require.Contains(t, out, filepath.Join("testdata", "coverage_paths", "b", "rules.yml"))
}

// TestRuleCoverageDeduplicatesSymlinkedRuleFile checks that a rule file reached
// through a symlink is counted once. Making the path absolute is not enough,
// since both paths are then still distinct.
func TestRuleCoverageDeduplicatesSymlinkedRuleFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ruleFile := filepath.Join(dir, "rules.yml")
	require.NoError(t, os.WriteFile(ruleFile, []byte(
		"groups:\n  - name: g\n    rules:\n      - record: only:metric\n        expr: up\n"), 0o600))
	if err := os.Symlink(ruleFile, filepath.Join(dir, "link.yml")); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	testFile := filepath.Join(dir, "test.yml")
	require.NoError(t, os.WriteFile(testFile, []byte(
		"rule_files:\n  - rules.yml\n  - link.yml\n\ntests: []\n"), 0o600))

	cov := recordCoverage(t, parser.NewParser(parser.Options{}), testFile)
	_, total := coverageCounts(cov)
	require.Equal(t, 1, total, "the same rule reached through a symlink is one rule")
}

// TestRuleCoverageParsesSharedRuleFileOnce checks that a rule file referenced by
// several test files is loaded once rather than re-parsed per referencing file.
func TestRuleCoverageParsesSharedRuleFileOnce(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := recordCoverage(t, p, "./testdata/coverage_shared_a_test.yml", "./testdata/coverage_shared_b_test.yml")

	require.Len(t, cov.groups, 1, "the shared rule file must be parsed once for the suite")
	covered, total := coverageCounts(cov)
	require.Equal(t, 1, total)
	require.Equal(t, 1, covered, "coverage from every test file still applies")
}

// TestRuleCoverageThresholdValidation rejects thresholds outside [0, 100],
// including the non-finite values strconv.ParseFloat accepts. NaN compares false
// against every bound, so unvalidated it would pass the range check and disable
// the gate it was asked to enforce.
func TestRuleCoverageThresholdValidation(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	for _, threshold := range []float64{-1, 101, math.NaN(), math.Inf(1), math.Inf(-1)} {
		t.Run(formatThreshold(threshold), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, threshold, "./testdata/coverage_test.yml"))
		})
	}
}

// TestRuleCoverageThresholdIsExact checks the gate compares raw counts, not the
// displayed percentage. Rounding first made it fail open: 1999 of 2000 rules is
// 99.95%, which displays as 100.0% and used to satisfy a threshold of 100.
func TestRuleCoverageThresholdIsExact(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	cov := newRuleCoverage(p, false)
	const total = 2000
	for i := range total {
		k := cov.register("/rules.yml", "g", i, recordingRule, fmt.Sprintf("m%d", i), labels.EmptyLabels())
		if i < total-1 {
			cov.observe(k, satisfiable)
		}
	}

	var buf bytes.Buffer
	reason := cov.reportAndGate(&buf, 100)
	require.Equal(t, "Rule test coverage 1999/2000 (99.95%) is below the threshold of 100%.", reason,
		"one uncovered rule must fail a threshold of 100")
	// The report must not claim 100% while a rule is uncovered either.
	require.Contains(t, buf.String(), "1999/2000 covered (99.95%)")

	require.Empty(t, cov.reportAndGate(io.Discard, 99.9), "99.95% clears a threshold of 99.9")
}

// TestRuleCoverageThresholdEndToEnd exercises the gate through the command entry
// point: 2/3 is 66.666...%, so it fails a threshold of 66.7 despite displaying
// as 66.7%.
func TestRuleCoverageThresholdEndToEnd(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	var buf bytes.Buffer
	require.Equal(t, 0, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 66.6, "./testdata/coverage_twothirds_test.yml"))
	require.Equal(t, 1, RulesUnitTestResult(&buf, promqltest.LazyLoaderOpts{}, p, nil, false, false, false, false, 66.7, "./testdata/coverage_twothirds_test.yml"))
}

// TestCoverageStatsBelowThreshold locks the gate to the exact counts.
func TestCoverageStatsBelowThreshold(t *testing.T) {
	t.Parallel()

	require.False(t, coverageStats{}.belowThreshold(0), "a zero threshold disables the check")
	require.False(t, coverageStats{Covered: 1, Total: 2}.belowThreshold(0))
	require.False(t, coverageStats{}.belowThreshold(100), "an empty set is gated by the caller, not here")

	require.True(t, coverageStats{Covered: 3, Total: 5}.belowThreshold(100))
	require.False(t, coverageStats{Covered: 3, Total: 5}.belowThreshold(60), "comparison is strictly less-than")
	require.True(t, coverageStats{Covered: 3, Total: 5}.belowThreshold(60.04), "a threshold need not be a round number")
	require.False(t, coverageStats{Covered: 3, Total: 5}.belowThreshold(50))

	// A single uncovered rule fails a threshold of 100 whatever the total.
	require.True(t, coverageStats{Covered: 1999, Total: 2000}.belowThreshold(100))
	require.True(t, coverageStats{Covered: 999999, Total: 1000000}.belowThreshold(100))
	require.False(t, coverageStats{Covered: 2000, Total: 2000}.belowThreshold(100))
}

// TestFormatCoveragePercentage checks the displayed percentage never contradicts
// the counts printed next to it by rounding to a misleading 100% or 0%.
func TestFormatCoveragePercentage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "100.0", formatCoveragePercentage(coverageStats{Covered: 5, Total: 5}))
	require.Equal(t, "0.0", formatCoveragePercentage(coverageStats{Covered: 0, Total: 5}))
	require.Equal(t, "66.7", formatCoveragePercentage(coverageStats{Covered: 2, Total: 3}))
	require.Equal(t, "99.95", formatCoveragePercentage(coverageStats{Covered: 1999, Total: 2000}))
	require.Equal(t, "0.001", formatCoveragePercentage(coverageStats{Covered: 1, Total: 100000}))
	require.Equal(t, "100.0", formatCoveragePercentage(coverageStats{}), "an empty set is fully covered")

	// A failure message must not read as a value being below itself: 2/3 is
	// 66.666...%, which is genuinely below a threshold of 66.7 even though both
	// render as "66.7" at one decimal place.
	require.Equal(t, "66.67", formatCoverageAgainstThreshold(coverageStats{Covered: 2, Total: 3}, 66.7))
	require.Equal(t, "60.0", formatCoverageAgainstThreshold(coverageStats{Covered: 3, Total: 5}, 60.04))
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
		name       string
		ruleName   string
		ruleLabels labels.Labels
		expr       string
		matches    bool
	}{
		{
			name:     "direct match",
			ruleName: "job:up:sum",
			expr:     "job:up:sum",
			matches:  true,
		},
		{
			name:     "aggregation wrapping",
			ruleName: "job:up:sum",
			expr:     `sum by (job)(job:up:sum)`,
			matches:  true,
		},
		{
			name:     "no match",
			ruleName: "job:up:sum",
			expr:     "other_metric",
			matches:  false,
		},
		{
			name:     "wildcard selector without __name__ is indeterminate",
			ruleName: "job:up:sum",
			expr:     `{job="prometheus"}`,
			matches:  false,
		},
		{
			name:       "compatible static label",
			ruleName:   "job:up:sum",
			ruleLabels: labels.FromStrings("team", "infra"),
			expr:       `job:up:sum{team="infra"}`,
			matches:    true,
		},
		{
			name:       "incompatible static label",
			ruleName:   "job:up:sum",
			ruleLabels: labels.FromStrings("team", "infra"),
			expr:       `job:up:sum{team="backend"}`,
			matches:    false,
		},
		{
			name:     "matcher on label absent from static labels is conservatively matched",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job="prometheus"}`,
			matches:  true,
		},
		{
			name:     "scalar expression has no selectors",
			ruleName: "job:up:sum",
			expr:     "vector(1)",
			matches:  false,
		},
		{
			name:     "ALERTS selector does not cover a recording rule",
			ruleName: "job:up:sum",
			expr:     `ALERTS{alertname="job:up:sum"}`,
			matches:  false,
		},
		{
			// PromQL combines repeated matchers on a label with AND, so the
			// negative matcher excludes the metric the regex would have selected.
			name:     "repeated __name__ matchers are combined with AND",
			ruleName: "job:up:sum",
			expr:     `{__name__=~"job:up:sum|other",__name__!="job:up:sum"}`,
			matches:  false,
		},
		{
			name:     "repeated __name__ matchers that all accept the rule match",
			ruleName: "job:up:sum",
			expr:     `{__name__=~"job:up:sum|other",__name__!="other"}`,
			matches:  true,
		},
		{
			// alertname is an ordinary label on a recording rule's output, not a
			// label the engine generates, so it must be compared like any other.
			name:       "incompatible static alertname label",
			ruleName:   "foo",
			ruleLabels: labels.FromStrings("alertname", "Bar"),
			expr:       `foo{alertname="Baz"}`,
			matches:    false,
		},
		{
			name:       "compatible static alertname label",
			ruleName:   "foo",
			ruleLabels: labels.FromStrings("alertname", "Bar"),
			expr:       `foo{alertname="Bar"}`,
			matches:    true,
		},
		{
			// The value of job comes from the input series and is unknown here,
			// but no series can carry two different values for it at once.
			name:     "self-contradictory matchers on a dynamic label",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job="a",job="b"}`,
			matches:  false,
		},
		{
			name:     "equality and negation of the same value on a dynamic label",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job="a",job!="a"}`,
			matches:  false,
		},
		{
			name:     "consistent repeated matchers on a dynamic label",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job="a",job=~"a|b"}`,
			matches:  true,
		},
		{
			// The regexps pin job to "a" and then exclude it, so no series can
			// satisfy both and the assertion can never observe the rule.
			name:     "contradictory regexp matchers on a dynamic label",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a",job!~"a"}`,
			matches:  false,
		},
		{
			name:     "disjoint regexp matchers on a dynamic label",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a",job=~"b"}`,
			matches:  false,
		},
		{
			// Prometheus regexps are fully anchored, so ".*" accepts every value
			// and its negation accepts none.
			name:     "negated total regexp on a dynamic label",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job!~".*"}`,
			matches:  false,
		},
		{
			name:     "regexp alternation with one surviving alternative",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a|b",job!~"b"}`,
			matches:  true,
		},
		{
			// An unbounded regexp accepts infinitely many values, so it stays
			// compatible rather than being rejected for want of a witness.
			name:     "unbounded regexp on a dynamic label",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"xyz.*"}`,
			matches:  true,
		},
		{
			name:     "negated equality on a dynamic label leaves other values",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job!="a"}`,
			matches:  true,
		},
		{
			// The same expression cannot be both required and excluded.
			name:     "identical unbounded regexp required and excluded",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a.*",job!~"a.*"}`,
			matches:  false,
		},
		{
			// No value can start with both "a" and "b".
			name:     "disjoint unbounded regexps",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a.*",job=~"b.*"}`,
			matches:  false,
		},
		{
			name:     "total regexp required and excluded",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~".*",job!~".*"}`,
			matches:  false,
		},
		{
			// One prefix continues the other, so "ab" satisfies both.
			name:     "nested unbounded regexp prefixes",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a.*",job=~"ab.*"}`,
			matches:  true,
		},
		{
			// A regexp with no literal prefix is still satisfiable, by "foo".
			name:     "unbounded regexp without a literal prefix",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~".*foo"}`,
			matches:  true,
		},
		{
			name:     "alternation followed by an unbounded regexp",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"(staging|prod)-.*"}`,
			matches:  true,
		},
		{
			name:     "character class regexp",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"[a-z]+"}`,
			matches:  true,
		},
		{
			// "a" satisfies both: it matches a.* and does not match ab.*.
			name:     "unbounded regexps that only look contradictory",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a.*",job!~"ab.*"}`,
			matches:  true,
		},
		{
			// The shortest value a.* allows is excluded, but "aa" is not.
			name:     "unbounded regexp with its shortest value excluded",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a.*",job!="a"}`,
			matches:  true,
		},
		{
			// The first alternation branch is excluded, the second is not.
			name:     "alternation branch excluded by a later matcher",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"(staging|prod)-.*",job!~"staging-.*"}`,
			matches:  true,
		},
		{
			// "b" satisfies both, but only if the class is not read as "a" alone.
			name:     "character class member excluded by a later matcher",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"[ab]+",job!~"a+"}`,
			matches:  true,
		},
		{
			// Needs one optional taken and the other left out.
			name:     "independent optional branches",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"x?y?",job!~"|xy"}`,
			matches:  true,
		},
		{
			// A negated alternation of literals rules out exactly those values,
			// which happen to be every fixed probe.
			name:     "negated alternation excluding every probe",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job!~"|a|A|0|-|aa"}`,
			matches:  true,
		},
		{
			// \B holds between two word characters, so this matches "foobar".
			// It used to be treated as an impossible expression.
			name:     "non-word-boundary assertion",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"foo\\Bbar"}`,
			matches:  true,
		},
		{
			// \b cannot hold between "o" and "b", so nothing matches this.
			name:     "word-boundary assertion that cannot hold",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"foo\\bbar"}`,
			matches:  false,
		},
		{
			// Label regexps are parsed dot-all, so "." accepts a newline and the
			// negated class does not.
			name:     "newline is a valid label value",
			ruleName: "job:up:sum",
			expr:     "job:up:sum{job=~\".\",job!~\"[^\\n]\"}",
			matches:  true,
		},
		{
			// Every fixed probe is excluded, but "aa" still satisfies the set.
			name:     "negative equalities excluding every probe",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job!="",job!="a",job!="A",job!="0",job!="-"}`,
			matches:  true,
		},
		{
			// One negative regexp excludes every probe and another the values
			// built to step outside it, but "b" satisfies both.
			name:     "negative regexps excluding the probes and one fresh family",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job!~"|a|A|0|-",job!~"coverage-witness-.*"}`,
			matches:  true,
		},
		{
			// The one- and two-fold values are excluded, so the search has to
			// build a longer one.
			name:     "repetition with its shortest values excluded",
			ruleName: "job:up:sum",
			expr:     `job:up:sum{job=~"a+",job!~"a|aa"}`,
			matches:  true,
		},
		{
			// A recording rule's labels are used verbatim, so template syntax in a
			// value is a literal string rather than something expanded later.
			name:       "template syntax in a recording rule label is literal",
			ruleName:   "foo",
			ruleLabels: labels.FromStrings("cluster", "{{ $externalLabels.cluster }}"),
			expr:       `foo{cluster="prod"}`,
			matches:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ruleLabels := tc.ruleLabels
			if ruleLabels.IsEmpty() {
				ruleLabels = labels.EmptyLabels()
			}
			require.Equal(t, tc.matches, anySelectorMatches(t, p, tc.expr, func(ms []*labels.Matcher) bool {
				return selectorCoversRecordingRule(tc.ruleName, ruleLabels, compileSelector(ms)) == satisfiable
			}))
		})
	}
}

func TestSelectorCoversAlertingRule(t *testing.T) {
	t.Parallel()

	p := parser.NewParser(parser.Options{})
	tests := []struct {
		name string
		// holdDuration is the rule's "for"; zero unless the case is about the
		// alert states a selector can observe.
		holdDuration time.Duration
		alertName    string
		ruleLabels   labels.Labels
		expr         string
		matches      bool
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
		{
			// PromQL combines repeated matchers on a label with AND, so this
			// selector can never select the InstanceDown alert.
			name:       "repeated alertname matchers are combined with AND",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS{alertname=~"InstanceDown|Other",alertname!="InstanceDown"}`,
			matches:    false,
		},
		{
			name:       "repeated alertname matchers that all accept the rule match",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS{alertname=~"InstanceDown|Other",alertname!="Other"}`,
			matches:    true,
		},
		{
			name:       "repeated __name__ matchers exclude both meta-metrics",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `{__name__=~"ALERTS.*",__name__!="ALERTS",__name__!="ALERTS_FOR_STATE",alertname="InstanceDown"}`,
			matches:    false,
		},
		{
			// The engine sets alertstate on every ALERTS series, and an alert is
			// only sampled while it is pending or firing.
			name:       "impossible alertstate value",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS{alertname="InstanceDown",alertstate="bogus"}`,
			matches:    false,
		},
		{
			name:       "firing alertstate value",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS{alertname="InstanceDown",alertstate="firing"}`,
			matches:    true,
		},
		{
			// An alert is only observable as pending while it is held down.
			name:         "pending alertstate value with a hold duration",
			holdDuration: 5 * time.Minute,
			alertName:    "InstanceDown",
			ruleLabels:   labels.EmptyLabels(),
			expr:         `ALERTS{alertname="InstanceDown",alertstate="pending"}`,
			matches:      true,
		},
		{
			// Without "for" the alert is promoted to firing before the first
			// sample, so it never appears as pending.
			name:       "pending alertstate value without a hold duration",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS{alertname="InstanceDown",alertstate="pending"}`,
			matches:    false,
		},
		{
			name:         "firing alertstate value with a hold duration",
			holdDuration: 5 * time.Minute,
			alertName:    "InstanceDown",
			ruleLabels:   labels.EmptyLabels(),
			expr:         `ALERTS{alertname="InstanceDown",alertstate="firing"}`,
			matches:      true,
		},
		{
			// ALERTS_FOR_STATE carries no generated alertstate, so the hold
			// duration does not constrain it.
			name:       "ALERTS_FOR_STATE is unaffected by the hold duration",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS_FOR_STATE{alertname="InstanceDown"}`,
			matches:    true,
		},
		{
			// A static alertstate label cannot survive on an ALERTS series because
			// the engine overwrites it, but ALERTS_FOR_STATE carries no generated
			// alertstate, so there it is an ordinary label.
			name:       "ALERTS_FOR_STATE treats alertstate as an ordinary label",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("alertstate", "custom"),
			expr:       `ALERTS_FOR_STATE{alertname="InstanceDown",alertstate="custom"}`,
			matches:    true,
		},
		{
			name:       "ALERTS ignores a static alertstate label the engine overwrites",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("alertstate", "custom"),
			expr:       `ALERTS{alertname="InstanceDown",alertstate="firing"}`,
			matches:    true,
		},
		{
			// The engine applies the rule's own labels last, so an empty value
			// deletes the label from the output and nothing can be non-empty.
			name:       "non-empty matcher against an empty static label",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("dynamic", "{{ $value }}", "fixed", ""),
			expr:       `ALERTS{alertname="InstanceDown",fixed!=""}`,
			matches:    false,
		},
		{
			// The same value the rule produces, expressed as a negated regexp.
			name:       "negated regexp that accepts the empty static label",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("dynamic", "{{ $value }}", "fixed", ""),
			expr:       `ALERTS{alertname="InstanceDown",fixed!~".+"}`,
			matches:    true,
		},
		{
			// A label the rule sets to the empty string still constrains the
			// selector, including when another label is templated. Rewriting the
			// label set to strip templates used to drop this one as well.
			name:       "empty static label alongside a templated one",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("severity", "", "cluster", "{{ $externalLabels.cluster }}"),
			expr:       `ALERTS{alertname="InstanceDown",severity="page"}`,
			matches:    false,
		},
		{
			name:       "empty static label on its own",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("severity", ""),
			expr:       `ALERTS{alertname="InstanceDown",severity="page"}`,
			matches:    false,
		},
		{
			// Matching the empty value is what the rule actually produces.
			name:       "selector asking for the empty value",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("severity", "", "cluster", "{{ $externalLabels.cluster }}"),
			expr:       `ALERTS{alertname="InstanceDown",severity=""}`,
			matches:    true,
		},
		{
			// Alert labels are expanded per alert instance, so the literal value
			// says nothing about what the series will carry.
			name:       "templated alert label is not compared literally",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("cluster", "{{ $externalLabels.cluster }}"),
			expr:       `ALERTS{alertname="InstanceDown",cluster="prod"}`,
			matches:    true,
		},
		{
			name:       "non-templated labels are still compared when another label is templated",
			alertName:  "InstanceDown",
			ruleLabels: labels.FromStrings("cluster", "{{ $externalLabels.cluster }}", "severity", "page"),
			expr:       `ALERTS{alertname="InstanceDown",cluster="prod",severity="critical"}`,
			matches:    false,
		},
		{
			name:       "self-contradictory matchers on a dynamic label",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS{alertname="InstanceDown",instance="a",instance="b"}`,
			matches:    false,
		},
		{
			name:       "contradictory regexp matchers on a dynamic label",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS{alertname="InstanceDown",instance=~"a",instance!~"a"}`,
			matches:    false,
		},
		{
			name:       "negated total regexp on a dynamic label",
			alertName:  "InstanceDown",
			ruleLabels: labels.EmptyLabels(),
			expr:       `ALERTS_FOR_STATE{alertname="InstanceDown",instance!~".*"}`,
			matches:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.matches, anySelectorMatches(t, p, tc.expr, func(ms []*labels.Matcher) bool {
				return selectorCoversAlertingRule(tc.alertName, tc.holdDuration, tc.ruleLabels, compileSelector(ms)) == satisfiable
			}))
		})
	}
}

// testRegexpCandidates runs the candidate walk with a generous budget, so tests
// exercise the search rather than the budget.
func testRegexpCandidates(expr string) []string {
	candidates, _ := regexpCandidates(expr, candidateByteBudget)
	return candidates
}
