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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/grafana/regexp"
	"github.com/nsf/jsondiff"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/junitxml"
)

// RulesUnitTest does unit testing of rules based on the unit testing files provided.
// More info about the file format can be found in the docs.
func RulesUnitTest(queryOpts promqltest.LazyLoaderOpts, p parser.Parser, runStrings []string, diffFlag, debug, ignoreUnknownFields bool, files ...string) int {
	return RulesUnitTestResult(io.Discard, queryOpts, p, runStrings, diffFlag, debug, ignoreUnknownFields, false, 0, files...)
}

// RulesUnitTestResult runs unit tests for rules and writes JUnit XML results to the
// provided writer. If coverage is true, it reports to stderr which rules are
// exercised by the test assertions. If coverageThreshold is greater than zero, it
// also enables coverage reporting and returns a non-zero exit code when the total
// coverage percentage is below the threshold.
func RulesUnitTestResult(results io.Writer, queryOpts promqltest.LazyLoaderOpts, p parser.Parser, runStrings []string, diffFlag, debug, ignoreUnknownFields, coverage bool, coverageThreshold float64, files ...string) int {
	if coverageThreshold < 0 || coverageThreshold > 100 {
		fmt.Fprintf(os.Stderr, "invalid --coverage-threshold %.1f: must be between 0 and 100\n", coverageThreshold)
		return failureExitCode
	}
	failed := false
	junit := &junitxml.JUnitXML{}

	var run *regexp.Regexp
	if runStrings != nil {
		run = regexp.MustCompile(strings.Join(runStrings, "|"))
	}

	var cov *ruleCoverage
	if coverage || coverageThreshold > 0 {
		cov = newRuleCoverage()
	}

	for _, f := range files {
		if errs := ruleUnitTest(f, queryOpts, p, run, diffFlag, debug, ignoreUnknownFields, junit.Suite(f), cov); errs != nil {
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
	if cov != nil {
		pct := cov.report(os.Stderr)
		if belowThreshold(pct, coverageThreshold) {
			fmt.Fprintf(os.Stderr, "Rule test coverage %.1f%% is below the threshold of %.1f%%.\n", pct, coverageThreshold)
			failed = true
		}
	}
	if failed {
		return failureExitCode
	}
	return successExitCode
}

func ruleUnitTest(filename string, queryOpts promqltest.LazyLoaderOpts, p parser.Parser, run *regexp.Regexp, diffFlag, debug, ignoreUnknownFields bool, ts *junitxml.TestSuite, cov *ruleCoverage) []error {
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

	// Record rule coverage for the suite before running the tests, so rules in
	// files whose tests fail (or that have no tests at all) are still counted.
	if cov != nil {
		cov.record(p, run, ignoreUnknownFields, &unitTestInp)
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
		t.parser = p
		ers := t.test(testname, evalInterval, groupOrderMap, queryOpts, diffFlag, debug, ignoreUnknownFields, unitTestInp.FuzzyCompare, unitTestInp.RuleFiles...)
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
	RuleFiles          []string       `yaml:"rule_files"`
	EvaluationInterval model.Duration `yaml:"evaluation_interval,omitempty"`
	GroupEvalOrder     []string       `yaml:"group_eval_order"`
	Tests              []testGroup    `yaml:"tests"`
	FuzzyCompare       bool           `yaml:"fuzzy_compare,omitempty"`
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

// testStartTimestamp wraps time.Time to support custom YAML unmarshaling.
// It can parse both RFC3339 timestamps and Unix timestamps.
type testStartTimestamp struct {
	time.Time
}

// UnmarshalYAML implements custom YAML unmarshaling for testStartTimestamp.
// It accepts both RFC3339 formatted strings and numeric Unix timestamps.
func (t *testStartTimestamp) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsed, err := parseTime(s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

// testGroup is a group of input series and tests associated with it.
type testGroup struct {
	Interval        model.Duration     `yaml:"interval"`
	InputSeries     []series           `yaml:"input_series"`
	AlertRuleTests  []alertTestCase    `yaml:"alert_rule_test,omitempty"`
	PromqlExprTests []promqlTestCase   `yaml:"promql_expr_test,omitempty"`
	ExternalLabels  labels.Labels      `yaml:"external_labels,omitempty"`
	ExternalURL     string             `yaml:"external_url,omitempty"`
	TestGroupName   string             `yaml:"name,omitempty"`
	StartTimestamp  testStartTimestamp `yaml:"start_timestamp,omitempty"`

	parser parser.Parser `yaml:"-"`
}

// test performs the unit tests.
func (tg *testGroup) test(testname string, evalInterval time.Duration, groupOrderMap map[string]int, queryOpts promqltest.LazyLoaderOpts, diffFlag, debug, ignoreUnknownFields, fuzzyCompare bool, ruleFiles ...string) (outErr []error) {
	if debug {
		testStart := time.Now()
		fmt.Fprintf(os.Stderr, "DEBUG: Starting test %s\n", testname)
		defer func() {
			fmt.Fprintf(os.Stderr, "DEBUG: Test %s finished, took %v\n", testname, time.Since(testStart))
		}()
	}
	// Setup testing suite.
	// Set the start time from the test group.
	queryOpts.StartTime = tg.StartTimestamp.Time
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
		QueryFunc:  rules.EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		Appendable: suite.Storage(),
		Context:    context.Background(),
		NotifyFunc: func(context.Context, string, ...*rules.Alert) {},
		Logger:     promslog.NewNopLogger(),
		Parser:     tg.parser,
	}
	m := rules.NewManager(opts)
	groupsMap, ers := m.LoadGroups(time.Duration(tg.Interval), tg.ExternalLabels, tg.ExternalURL, nil, ignoreUnknownFields, ruleFiles...)
	if ers != nil {
		return ers
	}
	groups := orderedGroups(groupsMap, groupOrderMap)

	// Bounds for evaluating the rules.
	var mint time.Time
	if tg.StartTimestamp.IsZero() {
		mint = time.Unix(0, 0).UTC()
	} else {
		mint = tg.StartTimestamp.Time
	}
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
	slices.Sort(alertEvalTimes)

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
								Labels:      a.Labels.Copy(),
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

				var expAlerts labelsAndAnnotations
				for _, a := range testcase.ExpAlerts {
					// User gives only the labels from alerting rule, which doesn't
					// include this label (added by Prometheus during Eval).
					if a.ExpLabels == nil {
						a.ExpLabels = make(map[string]string)
					}
					a.ExpLabels[labels.AlertName] = testcase.Alertname

					expAlerts = append(expAlerts, labelAndAnnotation{
						Labels:      labels.FromMap(a.ExpLabels),
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
								Labels:      labels.Labels{},
								Annotations: labels.Labels{},
							})
						}
						// If empty, populates an empty value
						if expAlerts.Len() == 0 {
							expAlerts = append(expAlerts, labelAndAnnotation{
								Labels:      labels.Labels{},
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

		var gotSamples []parsedSample
		for _, s := range got {
			gotSamples = append(gotSamples, parsedSample{
				Labels:    s.Metric.Copy(),
				Value:     s.F,
				Histogram: promqltest.HistogramTestExpression(s.H),
			})
		}

		var expSamples []parsedSample
		for _, s := range testCase.ExpSamples {
			lb, err := tg.parser.ParseMetric(s.Labels)
			var hist *histogram.FloatHistogram
			if err == nil && s.Histogram != "" {
				_, values, parseErr := tg.parser.ParseSeriesDesc("{} " + s.Histogram)
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
				Labels:    lb,
				Value:     s.Value,
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
			fmt.Fprintf(os.Stderr, "DEBUG: Failed querying, expr: %q, err: %v\n", expr, err)
			return errs
		}
		res := q.Exec(suite.Context())
		if res.Err != nil {
			fmt.Fprintf(os.Stderr, "DEBUG: Failed query exec, expr: %q, err: %v\n", expr, res.Err)
			return errs
		}
		switch v := res.Value.(type) {
		case promql.Matrix:
			fmt.Fprintf(os.Stderr, "DEBUG: Dump of all data (input_series and rules) at %v:\n", ts)
			fmt.Fprintln(os.Stderr, v.String())
		default:
			fmt.Fprintf(os.Stderr, "DEBUG: Got unexpected type %T\n", v)
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
	var result strings.Builder
	fmt.Fprintf(&result, "load %v\n", shortDuration(tg.Interval))
	for _, is := range tg.InputSeries {
		fmt.Fprintf(&result, "  %v %v\n", is.Series, is.Values)
	}
	return result.String()
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
			T:      v.T,
			F:      v.V,
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

func (la labelsAndAnnotations) Len() int      { return len(la) }
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
	var s strings.Builder
	s.WriteString("[\n0:" + indentLines("\n"+la[0].String(), "  "))
	for i, l := range la[1:] {
		s.WriteString(",\n" + strconv.Itoa(i+1) + ":" + indentLines("\n"+l.String(), "  "))
	}
	s.WriteString("\n]")

	return s.String()
}

type labelAndAnnotation struct {
	Labels      labels.Labels
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
	EvalTime  model.Duration `yaml:"eval_time"`
	Alertname string         `yaml:"alertname"`
	ExpAlerts []alert        `yaml:"exp_alerts"`
}

type alert struct {
	ExpLabels      map[string]string `yaml:"exp_labels"`
	ExpAnnotations map[string]string `yaml:"exp_annotations"`
}

type promqlTestCase struct {
	Expr       string         `yaml:"expr"`
	EvalTime   model.Duration `yaml:"eval_time"`
	ExpSamples []sample       `yaml:"exp_samples"`
}

type sample struct {
	Labels    string  `yaml:"labels"`
	Value     float64 `yaml:"value"`
	Histogram string  `yaml:"histogram"` // A non-empty string means Value is ignored.
}

// parsedSample is a sample with parsed Labels.
type parsedSample struct {
	Labels    labels.Labels
	Value     float64
	Histogram string // TestExpression() of histogram.FloatHistogram
}

func parsedSamplesString(pss []parsedSample) string {
	if len(pss) == 0 {
		return "nil"
	}
	var s strings.Builder
	s.WriteString(pss[0].String())
	for _, ps := range pss[1:] {
		s.WriteString(", " + ps.String())
	}
	return s.String()
}

func (ps *parsedSample) String() string {
	if ps.Histogram != "" {
		return ps.Labels.String() + " " + ps.Histogram
	}
	return ps.Labels.String() + " " + strconv.FormatFloat(ps.Value, 'E', -1, 64)
}

// ruleKind identifies whether a rule is an alerting or a recording rule, used
// for coverage reporting.
type ruleKind uint8

const (
	alertingRule ruleKind = iota
	recordingRule
)

// String returns the rule kind as it appears in rule files, "alert" or "record".
func (k ruleKind) String() string {
	if k == alertingRule {
		return "alert"
	}
	return "record"
}

// Meta-metric names emitted by alerting rules. A promql_expr_test selecting one
// of these with an "alertname" matcher exercises the corresponding alerting rule.
const (
	alertsMetricName         = "ALERTS"
	alertsForStateMetricName = "ALERTS_FOR_STATE"
)

// ruleCoverageKey uniquely identifies a rule within a test suite. Keying on
// file, group, name, kind, labels and query ensures that the same physical rule
// referenced by multiple test files is counted exactly once, while distinct
// rules that merely share a name (in different groups, or with different labels
// or expressions) are counted separately.
type ruleCoverageKey struct {
	file   string
	group  string
	name   string
	labels string
	query  string
	kind   ruleKind
}

// ruleCoverage tracks which rules of a test suite are exercised by unit test
// assertions. It is populated from the output of rules.Manager.LoadGroups so it
// stays consistent with how Prometheus itself loads rules.
type ruleCoverage struct {
	order   []ruleCoverageKey
	covered map[ruleCoverageKey]bool
}

func newRuleCoverage() *ruleCoverage {
	return &ruleCoverage{covered: map[ruleCoverageKey]bool{}}
}

// register records a rule so that it is counted towards the coverage total. It
// is idempotent: registering the same rule again returns the existing key and
// preserves its covered state.
func (c *ruleCoverage) register(g *rules.Group, r rules.Rule, kind ruleKind) ruleCoverageKey {
	// Canonicalize the file path so the same physical rule file reached through
	// different relative paths from different test files is counted once.
	file := g.File()
	if abs, err := filepath.Abs(file); err == nil {
		file = abs
	}
	k := ruleCoverageKey{
		file:   file,
		group:  g.Name(),
		name:   r.Name(),
		labels: r.Labels().String(),
		query:  r.Query().String(),
		kind:   kind,
	}
	if _, ok := c.covered[k]; !ok {
		c.order = append(c.order, k)
		c.covered[k] = false
	}
	return k
}

// mark flags the rule identified by k as covered by a test assertion.
func (c *ruleCoverage) mark(k ruleCoverageKey) {
	c.covered[k] = true
}

// record loads the rule files referenced by a single unit test file through
// rules.Manager.LoadGroups, registers every alerting and recording rule, and
// marks those exercised by the file's test assertions as covered. Because rules
// are attributed via their loaded group rather than by bare name, coverage is
// counted precisely across files and groups. Only test groups selected by run
// contribute coverage, so the report reflects what this invocation actually ran.
func (c *ruleCoverage) record(p parser.Parser, run *regexp.Regexp, ignoreUnknownFields bool, utf *unitTestFile) {
	// Collect the alert names and PromQL selectors asserted by the test groups
	// that will actually run.
	testedAlertnames := map[string]struct{}{}
	var exprSelectors [][]*labels.Matcher
	for i := range utf.Tests {
		tg := &utf.Tests[i]
		if !matchesRun(tg.TestGroupName, run) {
			continue
		}
		for _, a := range tg.AlertRuleTests {
			testedAlertnames[a.Alertname] = struct{}{}
		}
		for _, tc := range tg.PromqlExprTests {
			expr, err := p.ParseExpr(tc.Expr)
			if err != nil {
				continue
			}
			exprSelectors = append(exprSelectors, parser.ExtractSelectors(expr)...)
		}
	}

	// Load each rule file independently so that one unparseable file does not
	// drop the rules of its valid siblings from the coverage total. Files that
	// fail to load are reported by the test run itself.
	m := rules.NewManager(&rules.ManagerOptions{Parser: p})
	for _, rf := range utf.RuleFiles {
		groupsMap, errs := m.LoadGroups(time.Duration(utf.EvaluationInterval), labels.EmptyLabels(), "", nil, ignoreUnknownFields, rf)
		if len(errs) > 0 {
			continue
		}
		for _, g := range groupsMap {
			for _, r := range g.Rules() {
				switch rule := r.(type) {
				case *rules.AlertingRule:
					k := c.register(g, rule, alertingRule)
					// An alerting rule is covered by an alert_rule_test naming its
					// alertname, or by a promql_expr_test selecting its ALERTS series.
					if _, ok := testedAlertnames[rule.Name()]; ok {
						c.mark(k)
						continue
					}
					for _, matchers := range exprSelectors {
						if selectorCoversAlertingRule(rule.Name(), rule.Labels(), matchers) {
							c.mark(k)
							break
						}
					}
				case *rules.RecordingRule:
					k := c.register(g, rule, recordingRule)
					for _, matchers := range exprSelectors {
						if selectorCoversRecordingRule(rule, matchers) {
							c.mark(k)
							break
						}
					}
				}
			}
		}
	}
}

// metricNameMatcher returns the matcher on the __name__ label from a selector,
// or nil for a wildcard selector that has no __name__ matcher.
func metricNameMatcher(matchers []*labels.Matcher) *labels.Matcher {
	for _, m := range matchers {
		if m.Name == labels.MetricName {
			return m
		}
	}
	return nil
}

// selectorCoversRecordingRule reports whether a promql_expr_test selector reads
// the output of the given recording rule: its __name__ must match the rule's
// output metric and any other matchers must be compatible with the rule's
// (merged group and rule) static labels. It is inspired by, but stricter than,
// the selector matching in buildDependencyMap in rules/group.go, which matches on
// name only.
func selectorCoversRecordingRule(rule *rules.RecordingRule, matchers []*labels.Matcher) bool {
	nameMatcher := metricNameMatcher(matchers)
	if nameMatcher == nil {
		// Wildcard selector: it cannot be attributed to a specific rule.
		return false
	}
	if !nameMatcher.Matches(rule.Name()) {
		return false
	}
	return labelsCompatible(rule.Labels(), matchers)
}

// selectorCoversAlertingRule reports whether a promql_expr_test selector asserts
// on the alerting rule with the given name and static labels via its ALERTS or
// ALERTS_FOR_STATE meta-metric. A selector without an "alertname" matcher is
// treated as indeterminate and does not count as coverage. Any remaining
// matchers must be compatible with the rule's static labels.
func selectorCoversAlertingRule(name string, ruleLabels labels.Labels, matchers []*labels.Matcher) bool {
	nameMatcher := metricNameMatcher(matchers)
	if nameMatcher == nil {
		return false
	}
	if !nameMatcher.Matches(alertsMetricName) && !nameMatcher.Matches(alertsForStateMetricName) {
		return false
	}
	alertnameMatched := false
	for _, m := range matchers {
		if m.Name == labels.AlertName {
			if !m.Matches(name) {
				return false
			}
			alertnameMatched = true
			break
		}
	}
	if !alertnameMatched {
		return false
	}
	return labelsCompatible(ruleLabels, matchers)
}

// labelsCompatible reports whether the non-name matchers of a selector are
// compatible with the rule's known static labels. The __name__ and alertname
// matchers are handled by the callers and skipped here. A matcher on a label the
// rule does not statically set is treated as compatible, because the value may be
// produced from the input series at evaluation time.
func labelsCompatible(ruleLabels labels.Labels, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if m.Name == labels.MetricName || m.Name == labels.AlertName {
			continue
		}
		if ruleLabels.Has(m.Name) && !m.Matches(ruleLabels.Get(m.Name)) {
			return false
		}
	}
	return true
}

// report writes the coverage summary to w and returns the overall coverage
// percentage in the range [0, 100]. Alerting and recording rules are reported
// separately. A suite with no rules is reported as fully covered.
func (c *ruleCoverage) report(w io.Writer) float64 {
	var alertTotal, alertCovered, recordTotal, recordCovered int
	for _, k := range c.order {
		switch k.kind {
		case alertingRule:
			alertTotal++
			if c.covered[k] {
				alertCovered++
			}
		case recordingRule:
			recordTotal++
			if c.covered[k] {
				recordCovered++
			}
		}
	}
	total := alertTotal + recordTotal
	covered := alertCovered + recordCovered

	fmt.Fprintln(w, "Rule test coverage:")
	fmt.Fprintf(w, "  Alerting rules:  %s\n", coverageFraction(alertCovered, alertTotal))
	fmt.Fprintf(w, "  Recording rules: %s\n", coverageFraction(recordCovered, recordTotal))
	fmt.Fprintf(w, "  Total:           %s\n", coverageFraction(covered, total))

	if covered < total {
		fmt.Fprintln(w, "  Untested rules:")
		uncovered := make([]ruleCoverageKey, 0, total-covered)
		for _, k := range c.order {
			if !c.covered[k] {
				uncovered = append(uncovered, k)
			}
		}
		slices.SortFunc(uncovered, func(a, b ruleCoverageKey) int {
			if n := strings.Compare(a.file, b.file); n != 0 {
				return n
			}
			if n := strings.Compare(a.group, b.group); n != 0 {
				return n
			}
			if a.kind != b.kind {
				return int(a.kind) - int(b.kind)
			}
			if n := strings.Compare(a.name, b.name); n != 0 {
				return n
			}
			return strings.Compare(a.labels, b.labels)
		})
		var lastFile, lastGroup string
		headerPrinted := false
		for _, k := range uncovered {
			if !headerPrinted || k.file != lastFile || k.group != lastGroup {
				fmt.Fprintf(w, "    group %q in %s:\n", k.group, filepath.Base(k.file))
				lastFile, lastGroup = k.file, k.group
				headerPrinted = true
			}
			fmt.Fprintf(w, "      - %s: %s\n", k.kind, k.name)
		}
	}

	return coveragePercentage(covered, total)
}

// coverageFraction formats a covered/total count together with its percentage.
// A count with no rules is reported without a percentage.
func coverageFraction(covered, total int) string {
	if total == 0 {
		return "0/0 covered"
	}
	return fmt.Sprintf("%d/%d covered (%.1f%%)", covered, total, coveragePercentage(covered, total))
}

// coveragePercentage returns covered/total as a percentage, treating an empty
// set as fully covered.
func coveragePercentage(covered, total int) float64 {
	if total == 0 {
		return 100
	}
	return float64(covered) / float64(total) * 100
}

// belowThreshold reports whether the coverage percentage pct is below threshold.
// A threshold of zero or less disables the check. The comparison uses pct exactly
// as it is displayed (one decimal place) so the exit code never contradicts the
// reported number, even at rounding ties.
func belowThreshold(pct, threshold float64) bool {
	if threshold <= 0 {
		return false
	}
	displayed, _ := strconv.ParseFloat(fmt.Sprintf("%.1f", pct), 64)
	return displayed < threshold
}
