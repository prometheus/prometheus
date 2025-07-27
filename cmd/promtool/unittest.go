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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/grafana/regexp"
	"github.com/nsf/jsondiff"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/junitxml"
)

type coverageTracker struct {
	ruleFiles []string

	testedRules map[string]map[string]bool

	allRules map[string]map[string]*ruleInfo

	dependencies map[string][]string

	warnings []string

	testCaseMapping map[string][]string

	complexity *complexityMetrics

	testCases map[string]bool
}

type ruleInfo struct {
	Name string

	Type string

	Expression string

	File string

	Group string
}

type coverageReport struct {
	TotalRules int `json:"total_rules"`

	TestedRules int `json:"tested_rules"`

	Coverage float64 `json:"coverage_percentage"`

	RecordingRules *ruleCoverageSummary `json:"recording_rules"`

	AlertingRules *ruleCoverageSummary `json:"alerting_rules"`

	Files map[string]*fileCoverageInfo `json:"files"`

	Warnings []string `json:"warnings,omitempty"`

	TestCaseCount int `json:"test_case_count"`

	ComplexityMetrics *complexityMetrics `json:"complexity_metrics,omitempty"`
}

type ruleCoverageSummary struct {
	Total int `json:"total"`

	Tested int `json:"tested"`

	Coverage float64 `json:"coverage_percentage"`
}

type complexityMetrics struct {
	FunctionsUsed []string `json:"functions_used,omitempty"`

	ComplexExpressions int `json:"complex_expressions"`

	SubqueriesUsed int `json:"subqueries_used"`

	RegexMatchers int `json:"regex_matchers"`
}

type fileCoverageInfo struct {
	File string `json:"file"`

	TotalRules int `json:"total_rules"`

	TestedRules int `json:"tested_rules"`

	Coverage float64 `json:"coverage_percentage"`

	Rules map[string]*ruleCoverageInfo `json:"rules"`
}

type ruleCoverageInfo struct {
	Name string `json:"name"`

	Type string `json:"type"`

	Tested bool `json:"tested"`

	TestCases []string `json:"test_cases,omitempty"`
}

// RulesUnitTest does unit testing of rules based on the unit testing files provided.

// More info about the file format can be found in the docs.

func RulesUnitTest(queryOpts promqltest.LazyLoaderOpts, runStrings []string, diffFlag, debug, ignoreUnknownFields bool, files ...string) int {
	return RulesUnitTestResult(io.Discard, queryOpts, runStrings, diffFlag, debug, ignoreUnknownFields, false, "text", "", 0.0, files...)
}

func RulesUnitTestResult(results io.Writer, queryOpts promqltest.LazyLoaderOpts, runStrings []string, diffFlag, debug, ignoreUnknownFields, coverage bool, outputFormat, coverageOutput string, coverageThreshold float64, files ...string) int {
	failed := false

	junit := &junitxml.JUnitXML{}

	var run *regexp.Regexp
	if runStrings != nil {
		run = regexp.MustCompile(strings.Join(runStrings, "|"))
	}

	var tracker *coverageTracker
	if coverage {
		tracker = newCoverageTracker()
	}
	for _, f := range files {
		if errs := ruleUnitTest(f, queryOpts, run, diffFlag, debug, ignoreUnknownFields, tracker, junit.Suite(f)); errs != nil {

			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {

				fmt.Fprintln(os.Stderr, e.Error())

				fmt.Println()
			}

			failed = true
		} else {
			fmt.Println("  SUCCESS")
		}

		fmt.Println()
	}

	err := junit.WriteXML(results)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write JUnit XML: %s\n", err)
	}
	if coverage && tracker != nil {

		report := tracker.generateReport()
		if err := outputCoverageReport(report, outputFormat, coverageOutput); err != nil {
			fmt.Fprintf(os.Stderr, "failed to output coverage report: %s\n", err)
		}

		// Check coverage threshold
		if coverageThreshold > 0 && report.Coverage < coverageThreshold {

			fmt.Fprintf(os.Stderr, "Coverage %.2f%% is below threshold %.2f%%\n", report.Coverage, coverageThreshold)

			return coverageExitCode
		}
	}
	if failed {
		return 1 // failureExitCode
	}

	return 0 // successExitCode
}

func ruleUnitTest(filename string, queryOpts promqltest.LazyLoaderOpts, run *regexp.Regexp, diffFlag, debug, ignoreUnknownFields bool, tracker *coverageTracker, ts *junitxml.TestSuite) []error {
	b, err := os.ReadFile(filename)
	if err != nil {

		ts.Abort(err)

		return []error{err}
	}

	var unitTestInp unitTestFile
	if err := yaml.UnmarshalStrict(b, &unitTestInp); err != nil {

		ts.Abort(err)

		return []error{err}
	}
	if err := resolveAndGlobFilepaths(filepath.Dir(filename), &unitTestInp); err != nil {

		ts.Abort(err)

		return []error{err}
	}
	if unitTestInp.EvaluationInterval == 0 {
		unitTestInp.EvaluationInterval = model.Duration(1 * time.Minute)
	}

	evalInterval := time.Duration(unitTestInp.EvaluationInterval)

	ts.Settime(time.Now().Format("2006-01-02T15:04:05"))

	// Giving number for groups mentioned in the file for ordering.

	// Lower number group should be evaluated before higher number group.

	groupOrderMap := make(map[string]int)
	for i, gn := range unitTestInp.GroupEvalOrder {
		if _, ok := groupOrderMap[gn]; ok {

			err := fmt.Errorf("group name repeated in evaluation order: %s", gn)

			ts.Abort(err)

			return []error{err}
		}

		groupOrderMap[gn] = i
	}

	// Testing.

	var errs []error
	for i, t := range unitTestInp.Tests {
		if !matchesRun(t.TestGroupName, run) {
			continue
		}

		testname := t.TestGroupName
		if testname == "" {
			testname = fmt.Sprintf("unnamed#%d", i)
		}

		tc := ts.Case(testname)
		if t.Interval == 0 {
			t.Interval = unitTestInp.EvaluationInterval
		}

		ers := t.test(testname, evalInterval, groupOrderMap, queryOpts, diffFlag, debug, ignoreUnknownFields, tracker, unitTestInp.FuzzyCompare, unitTestInp.RuleFiles...)
		if ers != nil {
			for _, e := range ers {
				tc.Fail(e.Error())
			}

			errs = append(errs, ers...)
		}
	}
	if len(errs) > 0 {
		return errs
	}

	return nil
}

func matchesRun(name string, run *regexp.Regexp) bool {
	if run == nil {
		return true
	}

	return run.MatchString(name)
}

// unitTestFile holds the contents of a single unit test file.

type unitTestFile struct {
	RuleFiles []string `yaml:"rule_files"`

	EvaluationInterval model.Duration `yaml:"evaluation_interval,omitempty"`

	GroupEvalOrder []string `yaml:"group_eval_order"`

	Tests []testGroup `yaml:"tests"`

	FuzzyCompare bool `yaml:"fuzzy_compare,omitempty"`
}

// resolveAndGlobFilepaths joins all relative paths in a configuration

// with a given base directory and replaces all globs with matching files.

func resolveAndGlobFilepaths(baseDir string, utf *unitTestFile) error {
	for i, rf := range utf.RuleFiles {
		if rf != "" && !filepath.IsAbs(rf) {
			utf.RuleFiles[i] = filepath.Join(baseDir, rf)
		}
	}

	var globbedFiles []string
	for _, rf := range utf.RuleFiles {

		m, err := filepath.Glob(rf)
		if err != nil {
			return err
		}
		if len(m) == 0 {
			fmt.Fprintln(os.Stderr, "  WARNING: no file match pattern", rf)
		}

		globbedFiles = append(globbedFiles, m...)
	}

	utf.RuleFiles = globbedFiles

	return nil
}

// testGroup is a group of input series and tests associated with it.

type testGroup struct {
	Interval model.Duration `yaml:"interval"`

	InputSeries []series `yaml:"input_series"`

	AlertRuleTests []alertTestCase `yaml:"alert_rule_test,omitempty"`

	PromqlExprTests []promqlTestCase `yaml:"promql_expr_test,omitempty"`

	ExternalLabels labels.Labels `yaml:"external_labels,omitempty"`

	ExternalURL string `yaml:"external_url,omitempty"`

	TestGroupName string `yaml:"name,omitempty"`
}

// test performs the unit tests.

func (tg *testGroup) test(testname string, evalInterval time.Duration, groupOrderMap map[string]int, queryOpts promqltest.LazyLoaderOpts, diffFlag, debug, ignoreUnknownFields bool, tracker *coverageTracker, fuzzyCompare bool, ruleFiles ...string) (outErr []error) {
	if debug {

		testStart := time.Now()

		fmt.Printf("DEBUG: Starting test %s\n", testname)

		defer func() {
			fmt.Printf("DEBUG: Test %s finished, took %v\n", testname, time.Since(testStart))
		}()
	}

	// Setup testing suite.

	suite, err := promqltest.NewLazyLoader(tg.seriesLoadingString(), queryOpts)
	if err != nil {
		return []error{err}
	}

	defer func() {
		err := suite.Close()
		if err != nil {
			outErr = append(outErr, err)
		}
	}()

	suite.SubqueryInterval = evalInterval

	// Load the rule files.

	opts := &rules.ManagerOptions{
		QueryFunc: rules.EngineQueryFunc(suite.QueryEngine(), suite.Storage()),

		Appendable: suite.Storage(),

		Context: context.Background(),

		NotifyFunc: func(_ context.Context, _ string, _ ...*rules.Alert) {},

		Logger: promslog.NewNopLogger(),
	}

	m := rules.NewManager(opts)

	groupsMap, ers := m.LoadGroups(time.Duration(tg.Interval), tg.ExternalLabels, tg.ExternalURL, nil, ignoreUnknownFields, ruleFiles...)
	if ers != nil {
		return ers
	}

	groups := orderedGroups(groupsMap, groupOrderMap)
	if tracker != nil {
		for _, ruleFile := range ruleFiles {
			if err := tracker.addRuleFile(ruleFile, groupsMap); err != nil {
				return []error{err}
			}
		}
	}

	// Bounds for evaluating the rules.

	mint := time.Unix(0, 0).UTC()

	maxt := mint.Add(tg.maxEvalTime())

	// Optional floating point compare fuzzing.

	var compareFloat64 cmp.Option = cmp.Options{}
	if fuzzyCompare {
		compareFloat64 = cmp.Comparer(func(x, y float64) bool {
			return x == y || math.Nextafter(x, math.Inf(-1)) == y || math.Nextafter(x, math.Inf(1)) == y
		})
	}

	// Pre-processing some data for testing alerts.

	// All this preparation is so that we can test alerts as we evaluate the rules.

	// This avoids storing them in memory, as the number of evals might be high.

	// All the `eval_time` for which we have unit tests for alerts.

	alertEvalTimesMap := map[model.Duration]struct{}{}

	// Map of all the eval_time+alertname combination present in the unit tests.

	alertsInTest := make(map[model.Duration]map[string]struct{})

	// Map of all the unit tests for given eval_time.

	alertTests := make(map[model.Duration][]alertTestCase)
	for _, alert := range tg.AlertRuleTests {
		if alert.Alertname == "" {

			var testGroupLog string
			if tg.TestGroupName != "" {
				testGroupLog = fmt.Sprintf(" (in TestGroup %s)", tg.TestGroupName)
			}

			return []error{fmt.Errorf("an item under alert_rule_test misses required attribute alertname at eval_time %v%s", alert.EvalTime, testGroupLog)}
		}

		alertEvalTimesMap[alert.EvalTime] = struct{}{}
		if _, ok := alertsInTest[alert.EvalTime]; !ok {
			alertsInTest[alert.EvalTime] = make(map[string]struct{})
		}

		alertsInTest[alert.EvalTime][alert.Alertname] = struct{}{}

		alertTests[alert.EvalTime] = append(alertTests[alert.EvalTime], alert)
	}

	alertEvalTimes := make([]model.Duration, 0, len(alertEvalTimesMap))
	for k := range alertEvalTimesMap {
		alertEvalTimes = append(alertEvalTimes, k)
	}

	sort.Slice(alertEvalTimes, func(i, j int) bool {
		return alertEvalTimes[i] < alertEvalTimes[j]
	})

	// Current index in alertEvalTimes what we are looking at.

	curr := 0
	for _, g := range groups {
		for _, r := range g.Rules() {
			if alertRule, ok := r.(*rules.AlertingRule); ok {
				// Mark alerting rules as restored, to ensure the ALERTS timeseries is

				// created when they run.

				alertRule.SetRestored(true)
			}
		}
	}

	var errs []error
	for ts := mint; ts.Before(maxt) || ts.Equal(maxt); ts = ts.Add(evalInterval) {

		// Collects the alerts asked for unit testing.

		var evalErrs []error

		suite.WithSamplesTill(ts, func(err error) {
			if err != nil {

				errs = append(errs, err)

				return
			}
			for _, g := range groups {

				g.Eval(suite.Context(), ts)
				for _, r := range g.Rules() {
					if r.LastError() != nil {
						evalErrs = append(evalErrs, fmt.Errorf("    rule: %s, time: %s, err: %w",

							r.Name(), ts.Sub(time.Unix(0, 0).UTC()), r.LastError()))
					}
				}
			}
		})

		errs = append(errs, evalErrs...)

		// Only end testing at this point if errors occurred evaluating above,

		// rather than any test failures already collected in errs.
		if len(evalErrs) > 0 {
			return errs
		}
		for curr < len(alertEvalTimes) && ts.Sub(mint) <= time.Duration(alertEvalTimes[curr]) &&

			time.Duration(alertEvalTimes[curr]) < ts.Add(evalInterval).Sub(mint) {

			// We need to check alerts for this time.

			// If 'ts <= `eval_time=alertEvalTimes[curr]` < ts+evalInterval'

			// then we compare alerts with the Eval at `ts`.

			t := alertEvalTimes[curr]

			presentAlerts := alertsInTest[t]

			got := make(map[string]labelsAndAnnotations)

			// Same Alert name can be present in multiple groups.

			// Hence we collect them all to check against expected alerts.
			for _, g := range groups {

				grules := g.Rules()
				for _, r := range grules {

					ar, ok := r.(*rules.AlertingRule)
					if !ok {
						continue
					}
					if _, ok := presentAlerts[ar.Name()]; !ok {
						continue
					}

					var alerts labelsAndAnnotations
					for _, a := range ar.ActiveAlerts() {
						if a.State == rules.StateFiring {
							alerts = append(alerts, labelAndAnnotation{
								Labels: a.Labels.Copy(),

								Annotations: a.Annotations.Copy(),
							})
						}
					}

					got[ar.Name()] = append(got[ar.Name()], alerts...)
				}
			}
			for _, testcase := range alertTests[t] {

				// Checking alerts.

				gotAlerts := got[testcase.Alertname]
				if tracker != nil {
					tracker.markRuleTested(testcase.Alertname, testname)
				}

				var expAlerts labelsAndAnnotations
				for _, a := range testcase.ExpAlerts {

					// User gives only the labels from alerting rule, which doesn't

					// include this label (added by Prometheus during Eval).
					if a.ExpLabels == nil {
						a.ExpLabels = make(map[string]string)
					}

					a.ExpLabels[labels.AlertName] = testcase.Alertname

					expAlerts = append(expAlerts, labelAndAnnotation{
						Labels: labels.FromMap(a.ExpLabels),

						Annotations: labels.FromMap(a.ExpAnnotations),
					})
				}

				sort.Sort(gotAlerts)

				sort.Sort(expAlerts)
				if !cmp.Equal(expAlerts, gotAlerts, cmp.Comparer(labels.Equal), compareFloat64) {

					var testName string
					if tg.TestGroupName != "" {
						testName = fmt.Sprintf("    name: %s,\n", tg.TestGroupName)
					}

					expString := indentLines(expAlerts.String(), "            ")

					gotString := indentLines(gotAlerts.String(), "            ")
					if diffFlag {

						// If empty, populates an empty value
						if gotAlerts.Len() == 0 {
							gotAlerts = append(gotAlerts, labelAndAnnotation{
								Labels: labels.Labels{},

								Annotations: labels.Labels{},
							})
						}

						// If empty, populates an empty value
						if expAlerts.Len() == 0 {
							expAlerts = append(expAlerts, labelAndAnnotation{
								Labels: labels.Labels{},

								Annotations: labels.Labels{},
							})
						}

						diffOpts := jsondiff.DefaultConsoleOptions()

						expAlertsJSON, err := json.Marshal(expAlerts)
						if err != nil {

							errs = append(errs, fmt.Errorf("error marshaling expected %s alert: [%s]", tg.TestGroupName, err.Error()))

							continue
						}

						gotAlertsJSON, err := json.Marshal(gotAlerts)
						if err != nil {

							errs = append(errs, fmt.Errorf("error marshaling received %s alert: [%s]", tg.TestGroupName, err.Error()))

							continue
						}

						res, diff := jsondiff.Compare(expAlertsJSON, gotAlertsJSON, &diffOpts)
						if res != jsondiff.FullMatch {
							errs = append(errs, fmt.Errorf("%s    alertname: %s, time: %s, \n        diff: %v",

								testName, testcase.Alertname, testcase.EvalTime.String(), indentLines(diff, "            ")))
						}
					} else {
						errs = append(errs, fmt.Errorf("%s    alertname: %s, time: %s, \n        exp:%v, \n        got:%v",

							testName, testcase.Alertname, testcase.EvalTime.String(), expString, gotString))
					}
				}
			}

			curr++
		}
	}

	// Checking promql expressions.

Outer:
	for _, testCase := range tg.PromqlExprTests {

		got, err := query(suite.Context(), testCase.Expr, mint.Add(time.Duration(testCase.EvalTime)),

			suite.QueryEngine(), suite.Queryable())
		if err != nil {

			errs = append(errs, fmt.Errorf("    expr: %q, time: %s, err: %s", testCase.Expr,

				testCase.EvalTime.String(), err.Error()))

			continue
		}
		if tracker != nil {

			expr, parseErr := parser.ParseExpr(testCase.Expr)
			if parseErr == nil {
				tracker.markExpressionTested(expr, testname)
			} else {
				tracker.warnings = append(tracker.warnings, fmt.Sprintf("Failed to parse expression in test %s: %s", testname, parseErr.Error()))
			}
		}

		var gotSamples []parsedSample
		for _, s := range got {
			gotSamples = append(gotSamples, parsedSample{
				Labels: s.Metric.Copy(),

				Value: s.F,

				Histogram: promqltest.HistogramTestExpression(s.H),
			})
		}

		var expSamples []parsedSample
		for _, s := range testCase.ExpSamples {

			lb, err := parser.ParseMetric(s.Labels)

			var hist *histogram.FloatHistogram
			if err == nil && s.Histogram != "" {

				_, values, parseErr := parser.ParseSeriesDesc("{} " + s.Histogram)
				switch {
				case parseErr != nil:

					err = parseErr
				case len(values) != 1:

					err = fmt.Errorf("expected 1 value, got %d", len(values))
				case values[0].Histogram == nil:

					err = fmt.Errorf("expected histogram, got %v", values[0])
				default:

					hist = values[0].Histogram
				}
			}
			if err != nil {

				err = fmt.Errorf("labels %q: %w", s.Labels, err)

				errs = append(errs, fmt.Errorf("    expr: %q, time: %s, err: %w", testCase.Expr,

					testCase.EvalTime.String(), err))

				continue Outer
			}

			expSamples = append(expSamples, parsedSample{
				Labels: lb,

				Value: s.Value,

				Histogram: promqltest.HistogramTestExpression(hist),
			})
		}

		sort.Slice(expSamples, func(i, j int) bool {
			return labels.Compare(expSamples[i].Labels, expSamples[j].Labels) <= 0
		})

		sort.Slice(gotSamples, func(i, j int) bool {
			return labels.Compare(gotSamples[i].Labels, gotSamples[j].Labels) <= 0
		})
		if !cmp.Equal(expSamples, gotSamples, cmp.Comparer(labels.Equal), compareFloat64) {
			errs = append(errs, fmt.Errorf("    expr: %q, time: %s,\n        exp: %v\n        got: %v", testCase.Expr,

				testCase.EvalTime.String(), parsedSamplesString(expSamples), parsedSamplesString(gotSamples)))
		}
	}
	if debug {

		ts := tg.maxEvalTime()

		// Potentially a test can be specified at a time with fractional seconds,

		// which PromQL cannot represent, so round up to the next whole second.

		ts = (ts + time.Second).Truncate(time.Second)

		expr := fmt.Sprintf(`{__name__=~".+"}[%v]`, ts)

		q, err := suite.QueryEngine().NewInstantQuery(context.Background(), suite.Queryable(), nil, expr, mint.Add(ts))
		if err != nil {

			fmt.Printf("DEBUG: Failed querying, expr: %q, err: %v\n", expr, err)

			return errs
		}

		res := q.Exec(suite.Context())
		if res.Err != nil {

			fmt.Printf("DEBUG: Failed query exec, expr: %q, err: %v\n", expr, res.Err)

			return errs
		}
		switch v := res.Value.(type) {
		case promql.Matrix:

			fmt.Printf("DEBUG: Dump of all data (input_series and rules) at %v:\n", ts)

			fmt.Println(v.String())
		default:

			fmt.Printf("DEBUG: Got unexpected type %T\n", v)

			return errs
		}
	}
	if len(errs) > 0 {
		return errs
	}

	return nil
}

// seriesLoadingString returns the input series in PromQL notation.

func (tg *testGroup) seriesLoadingString() string {
	result := fmt.Sprintf("load %v\n", shortDuration(tg.Interval))
	for _, is := range tg.InputSeries {
		result += fmt.Sprintf("  %v %v\n", is.Series, is.Values)
	}

	return result
}

func shortDuration(d model.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}

	return s
}

// orderedGroups returns a slice of `*rules.Group` from `groupsMap` which follows the order

// mentioned by `groupOrderMap`. NOTE: This is partial ordering.

func orderedGroups(groupsMap map[string]*rules.Group, groupOrderMap map[string]int) []*rules.Group {
	groups := make([]*rules.Group, 0, len(groupsMap))
	for _, g := range groupsMap {
		groups = append(groups, g)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groupOrderMap[groups[i].Name()] < groupOrderMap[groups[j].Name()]
	})

	return groups
}

// maxEvalTime returns the max eval time among all alert and promql unit tests.

func (tg *testGroup) maxEvalTime() time.Duration {
	var maxd model.Duration
	for _, alert := range tg.AlertRuleTests {
		if alert.EvalTime > maxd {
			maxd = alert.EvalTime
		}
	}
	for _, pet := range tg.PromqlExprTests {
		if pet.EvalTime > maxd {
			maxd = pet.EvalTime
		}
	}

	return time.Duration(maxd)
}

func query(ctx context.Context, qs string, t time.Time, engine *promql.Engine, qu storage.Queryable) (promql.Vector, error) {
	q, err := engine.NewInstantQuery(ctx, qu, nil, qs, t)
	if err != nil {
		return nil, err
	}

	res := q.Exec(ctx)
	if res.Err != nil {
		return nil, res.Err
	}
	switch v := res.Value.(type) {
	case promql.Vector:

		return v, nil
	case promql.Scalar:

		return promql.Vector{promql.Sample{
			T: v.T,

			F: v.V,

			Metric: labels.Labels{},
		}}, nil
	default:

		return nil, errors.New("rule result is not a vector or scalar")
	}
}

// indentLines prefixes each line in the supplied string with the given "indent"

// string.

func indentLines(lines, indent string) string {
	sb := strings.Builder{}

	n := strings.Split(lines, "\n")
	for i, l := range n {
		if i > 0 {
			sb.WriteString(indent)
		}

		sb.WriteString(l)
		if i != len(n)-1 {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}

type labelsAndAnnotations []labelAndAnnotation

func (la labelsAndAnnotations) Len() int { return len(la) }

func (la labelsAndAnnotations) Swap(i, j int) { la[i], la[j] = la[j], la[i] }

func (la labelsAndAnnotations) Less(i, j int) bool {
	diff := labels.Compare(la[i].Labels, la[j].Labels)
	if diff != 0 {
		return diff < 0
	}

	return labels.Compare(la[i].Annotations, la[j].Annotations) < 0
}

func (la labelsAndAnnotations) String() string {
	if len(la) == 0 {
		return "[]"
	}

	s := "[\n0:" + indentLines("\n"+la[0].String(), "  ")
	for i, l := range la[1:] {
		s += ",\n" + strconv.Itoa(i+1) + ":" + indentLines("\n"+l.String(), "  ")
	}

	s += "\n]"

	return s
}

type labelAndAnnotation struct {
	Labels labels.Labels

	Annotations labels.Labels
}

func (la *labelAndAnnotation) String() string {
	return "Labels:" + la.Labels.String() + "\nAnnotations:" + la.Annotations.String()
}

type series struct {
	Series string `yaml:"series"`

	Values string `yaml:"values"`
}

type alertTestCase struct {
	EvalTime model.Duration `yaml:"eval_time"`

	Alertname string `yaml:"alertname"`

	ExpAlerts []alert `yaml:"exp_alerts"`
}

type alert struct {
	ExpLabels map[string]string `yaml:"exp_labels"`

	ExpAnnotations map[string]string `yaml:"exp_annotations"`
}

type promqlTestCase struct {
	Expr string `yaml:"expr"`

	EvalTime model.Duration `yaml:"eval_time"`

	ExpSamples []sample `yaml:"exp_samples"`
}

type sample struct {
	Labels string `yaml:"labels"`

	Value float64 `yaml:"value"`

	Histogram string `yaml:"histogram"` // A non-empty string means Value is ignored.
}

// parsedSample is a sample with parsed Labels.

type parsedSample struct {
	Labels labels.Labels

	Value float64

	Histogram string // TestExpression() of histogram.FloatHistogram
}

func parsedSamplesString(pss []parsedSample) string {
	if len(pss) == 0 {
		return "nil"
	}

	s := pss[0].String()
	for _, ps := range pss[1:] {
		s += ", " + ps.String()
	}

	return s
}

func (ps *parsedSample) String() string {
	if ps.Histogram != "" {
		return ps.Labels.String() + " " + ps.Histogram
	}

	return ps.Labels.String() + " " + strconv.FormatFloat(ps.Value, 'E', -1, 64)
}

func newCoverageTracker() *coverageTracker {
	return &coverageTracker{
		testedRules: make(map[string]map[string]bool),

		allRules: make(map[string]map[string]*ruleInfo),

		dependencies: make(map[string][]string),

		testCaseMapping: make(map[string][]string),

		testCases: make(map[string]bool),

		complexity: &complexityMetrics{
			FunctionsUsed: make([]string, 0),
		},
	}
}

func (ct *coverageTracker) addRuleFile(filename string, groups map[string]*rules.Group) error {
	ct.ruleFiles = append(ct.ruleFiles, filename)

	ct.allRules[filename] = make(map[string]*ruleInfo)

	ct.testedRules[filename] = make(map[string]bool)
	for _, group := range groups {
		for _, rule := range group.Rules() {

			ruleName := rule.Name()

			ruleType := "recording"
			if _, ok := rule.(*rules.AlertingRule); ok {
				ruleType = "alerting"
			}

			ct.allRules[filename][ruleName] = &ruleInfo{
				Name: ruleName,

				Type: ruleType,

				Expression: rule.Query().String(),

				File: filename,

				Group: group.Name(),
			}

			ct.extractDependencies(ruleName, rule.Query())
		}
	}

	return nil
}

func (ct *coverageTracker) extractDependencies(ruleName string, expr parser.Expr) {
	var dependencies []string

	seenMetrics := make(map[string]bool)

	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		switch n := node.(type) {
		case *parser.VectorSelector:

			metricName := n.Name
			if metricName != "" && metricName != ruleName && !seenMetrics[metricName] {

				dependencies = append(dependencies, metricName)

				seenMetrics[metricName] = true
			}
		case *parser.MatrixSelector:
			if vs, ok := n.VectorSelector.(*parser.VectorSelector); ok && vs.Name != "" && vs.Name != ruleName && !seenMetrics[vs.Name] {

				dependencies = append(dependencies, vs.Name)

				seenMetrics[vs.Name] = true
			}
		case *parser.Call:
			if n.Func != nil {

				// Track function usage

				funcName := n.Func.Name

				found := false
				for _, f := range ct.complexity.FunctionsUsed {
					if f == funcName {

						found = true

						break
					}
				}
				if !found {
					ct.complexity.FunctionsUsed = append(ct.complexity.FunctionsUsed, funcName)
				}
				switch funcName {
				case "label_replace", "label_join":

					ct.warnings = append(ct.warnings, fmt.Sprintf("Complex function %s in rule %s may affect coverage calculation", funcName, ruleName))

					ct.complexity.ComplexExpressions++
				case "absent", "absent_over_time":

					ct.warnings = append(ct.warnings, fmt.Sprintf("Function %s in rule %s may reference metrics that don't exist in test data", funcName, ruleName))
				case "group_left", "group_right":

					ct.warnings = append(ct.warnings, fmt.Sprintf("Complex aggregation %s in rule %s may affect dependency tracking", funcName, ruleName))

					ct.complexity.ComplexExpressions++
				}
			}
		case *parser.BinaryExpr:
			if n.VectorMatching != nil && (n.VectorMatching.On || len(n.VectorMatching.MatchingLabels) > 0) {

				ct.warnings = append(ct.warnings, fmt.Sprintf("Complex vector matching in rule %s may affect coverage calculation", ruleName))

				ct.complexity.ComplexExpressions++
			}
		case *parser.SubqueryExpr:

			ct.warnings = append(ct.warnings, fmt.Sprintf("Subquery in rule %s may reference additional time ranges", ruleName))

			ct.complexity.SubqueriesUsed++
		}

		return nil
	})
	if len(dependencies) > 0 {
		ct.dependencies[ruleName] = dependencies
	}
}

func (ct *coverageTracker) markRuleTested(ruleName, testCase string) {
	ct.testCases[testCase] = true
	for filename := range ct.allRules {
		if _, exists := ct.allRules[filename][ruleName]; exists {

			ct.testedRules[filename][ruleName] = true

			ct.testCaseMapping[ruleName] = append(ct.testCaseMapping[ruleName], testCase)
			if deps, hasDeps := ct.dependencies[ruleName]; hasDeps {
				for _, dep := range deps {
					ct.markRuleTested(dep, testCase)
				}
			}

			break
		}
	}
}

func (ct *coverageTracker) markExpressionTested(expr parser.Expr, testCase string) {
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		switch n := node.(type) {
		case *parser.VectorSelector:
			if n.Name != "" {
				ct.markRuleTested(n.Name, testCase)
			}

			// Also check for dynamic metric name references in label matchers

			ct.checkLabelMatchers(n.LabelMatchers, testCase)
		case *parser.MatrixSelector:
			if vs, ok := n.VectorSelector.(*parser.VectorSelector); ok && vs.Name != "" {

				ct.markRuleTested(vs.Name, testCase)

				ct.checkLabelMatchers(vs.LabelMatchers, testCase)
			}
		case *parser.Call:

			// Handle functions that might reference other metrics indirectly
			if n.Func != nil {
				switch n.Func.Name {
				case "label_replace", "label_join":

					// These functions might create references to other rules

					ct.extractFunctionDependencies(n, testCase)
				}
			}
		}

		return nil
	})
}

func (ct *coverageTracker) checkLabelMatchers(matchers []*labels.Matcher, testCase string) {
	for _, matcher := range matchers {
		if matcher.Type == labels.MatchRegexp || matcher.Type == labels.MatchNotRegexp {

			ct.complexity.RegexMatchers++
			if matcher.Name == "__name__" {
				// Handle regex patterns on __name__ that might match multiple metrics

				ct.warnings = append(ct.warnings, fmt.Sprintf("Regex pattern on __name__ in test %s may match multiple metrics, coverage tracking may be incomplete", testCase))
			}
		}
	}
}

func (ct *coverageTracker) extractFunctionDependencies(call *parser.Call, testCase string) {
	// For complex functions, we can try to extract metric references from arguments
	for _, arg := range call.Args {
		if vs, ok := arg.(*parser.VectorSelector); ok && vs.Name != "" {
			ct.markRuleTested(vs.Name, testCase)
		}
	}
}

func (ct *coverageTracker) generateReport() *coverageReport {
	totalRules := 0

	testedRules := 0

	recordingTotal := 0

	recordingTested := 0

	alertingTotal := 0

	alertingTested := 0

	files := make(map[string]*fileCoverageInfo)
	for filename, rules := range ct.allRules {

		fileInfo := &fileCoverageInfo{
			File: filename,

			Rules: make(map[string]*ruleCoverageInfo),
		}

		fileTotalRules := 0

		fileTestedRules := 0
		for ruleName, ruleInfo := range rules {

			tested := ct.testedRules[filename][ruleName]

			testCases := ct.testCaseMapping[ruleName]
			if len(testCases) > 0 && !tested {

				tested = true

				ct.testedRules[filename][ruleName] = true
			}

			fileInfo.Rules[ruleName] = &ruleCoverageInfo{
				Name: ruleName,

				Type: ruleInfo.Type,

				Tested: tested,

				TestCases: testCases,
			}

			fileTotalRules++
			if tested {
				fileTestedRules++
			}

			// Track rule type metrics
			switch ruleInfo.Type {
			case "recording":

				recordingTotal++
				if tested {
					recordingTested++
				}
			case "alerting":

				alertingTotal++
				if tested {
					alertingTested++
				}
			}
		}

		fileInfo.TotalRules = fileTotalRules

		fileInfo.TestedRules = fileTestedRules
		if fileTotalRules > 0 {
			fileInfo.Coverage = float64(fileTestedRules) / float64(fileTotalRules) * 100
		}

		files[filename] = fileInfo

		totalRules += fileTotalRules

		testedRules += fileTestedRules
	}

	coverage := 0.0
	if totalRules > 0 {
		coverage = float64(testedRules) / float64(totalRules) * 100
	}

	recordingCoverage := 0.0
	if recordingTotal > 0 {
		recordingCoverage = float64(recordingTested) / float64(recordingTotal) * 100
	}

	alertingCoverage := 0.0
	if alertingTotal > 0 {
		alertingCoverage = float64(alertingTested) / float64(alertingTotal) * 100
	}

	return &coverageReport{
		TotalRules: totalRules,

		TestedRules: testedRules,

		Coverage: coverage,

		RecordingRules: &ruleCoverageSummary{
			Total: recordingTotal,

			Tested: recordingTested,

			Coverage: recordingCoverage,
		},

		AlertingRules: &ruleCoverageSummary{
			Total: alertingTotal,

			Tested: alertingTested,

			Coverage: alertingCoverage,
		},

		Files: files,

		Warnings: ct.warnings,

		TestCaseCount: len(ct.testCases),

		ComplexityMetrics: ct.complexity,
	}
}

func outputCoverageReport(report *coverageReport, format, outputFile string) error {
	var output io.Writer
	if outputFile != "" {

		file, err := os.Create(outputFile)
		if err != nil {
			return err
		}

		defer file.Close()

		output = file
	} else {
		output = os.Stdout
	}
	switch format {
	case "json":

		encoder := json.NewEncoder(output)

		encoder.SetIndent("", "  ")

		return encoder.Encode(report)
	case "junit-xml":

		return outputCoverageJUnit(report, output)
	default: // text

		return outputCoverageText(report, output)
	}
}

func outputCoverageText(report *coverageReport, output io.Writer) error {
	fmt.Fprintf(output, "\nCoverage Report:\n")

	fmt.Fprintf(output, "================\n")

	fmt.Fprintf(output, "Total Rules: %d\n", report.TotalRules)

	fmt.Fprintf(output, "Tested Rules: %d\n", report.TestedRules)

	fmt.Fprintf(output, "Overall Coverage: %.2f%%\n", report.Coverage)
	if report.RecordingRules != nil {
		fmt.Fprintf(output, "Recording Rules: %.2f%% (%d/%d)\n", report.RecordingRules.Coverage, report.RecordingRules.Tested, report.RecordingRules.Total)
	}
	if report.AlertingRules != nil {
		fmt.Fprintf(output, "Alerting Rules: %.2f%% (%d/%d)\n", report.AlertingRules.Coverage, report.AlertingRules.Tested, report.AlertingRules.Total)
	}

	fmt.Fprintf(output, "Test Cases: %d\n", report.TestCaseCount)
	if report.ComplexityMetrics != nil {

		fmt.Fprintf(output, "\nComplexity Metrics:\n")
		if len(report.ComplexityMetrics.FunctionsUsed) > 0 {
			fmt.Fprintf(output, "  Functions Used: %s\n", strings.Join(report.ComplexityMetrics.FunctionsUsed, ", "))
		}
		if report.ComplexityMetrics.ComplexExpressions > 0 {
			fmt.Fprintf(output, "  Complex Expressions: %d\n", report.ComplexityMetrics.ComplexExpressions)
		}
		if report.ComplexityMetrics.SubqueriesUsed > 0 {
			fmt.Fprintf(output, "  Subqueries Used: %d\n", report.ComplexityMetrics.SubqueriesUsed)
		}
		if report.ComplexityMetrics.RegexMatchers > 0 {
			fmt.Fprintf(output, "  Regex Matchers: %d\n", report.ComplexityMetrics.RegexMatchers)
		}
	}
	if len(report.Warnings) > 0 {

		fmt.Fprintf(output, "\nWarnings:\n")
		for _, warning := range report.Warnings {
			fmt.Fprintf(output, "  - %s\n", warning)
		}
	}

	fmt.Fprintf(output, "\nFile Coverage:\n")
	for filename, fileInfo := range report.Files {

		fmt.Fprintf(output, "  %s: %.2f%% (%d/%d)\n", filename, fileInfo.Coverage, fileInfo.TestedRules, fileInfo.TotalRules)
		for ruleName, ruleInfo := range fileInfo.Rules {

			status := "✗"
			if ruleInfo.Tested {
				status = "✓"
			}

			fmt.Fprintf(output, "    %s %s (%s)\n", status, ruleName, ruleInfo.Type)
			if len(ruleInfo.TestCases) > 0 {
				fmt.Fprintf(output, "      Tested by: %s\n", strings.Join(ruleInfo.TestCases, ", "))
			}
		}

		fmt.Fprintf(output, "\n")
	}

	return nil
}

func outputCoverageJUnit(report *coverageReport, output io.Writer) error {
	junit := &junitxml.JUnitXML{}

	suite := junit.Suite("coverage")

	suite.Case(fmt.Sprintf("Coverage: %.2f%% (%d/%d rules tested)",

		report.Coverage, report.TestedRules, report.TotalRules))
	for filename, fileInfo := range report.Files {
		for ruleName, ruleInfo := range fileInfo.Rules {

			suite.Case(fmt.Sprintf("%s:%s", filename, ruleName))
			if !ruleInfo.Tested {
				suite.Fail("Rule not covered by any test")
			}
		}
	}

	return junit.WriteXML(output)
}
