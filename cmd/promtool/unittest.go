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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
)

// RulesUnitTest does unit testing of rules based on the unit testing files provided.
// More info about the file format can be found in the docs.
func RulesUnitTest(queryOpts promql.LazyLoaderOpts, files ...string) int {
	failed := false

	for _, f := range files {
		if errs := ruleUnitTest(f, queryOpts); errs != nil {
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
	if failed {
		return failureExitCode
	}
	return successExitCode
}

func ruleUnitTest(filename string, queryOpts promql.LazyLoaderOpts) []error {
	fmt.Println("Unit Testing: ", filename)

	b, err := os.ReadFile(filename)
	if err != nil {
		return []error{err}
	}

	var unitTestInp unitTestFile
	if err := yaml.UnmarshalStrict(b, &unitTestInp); err != nil {
		return []error{err}
	}
	if err := resolveAndGlobFilepaths(filepath.Dir(filename), &unitTestInp); err != nil {
		return []error{err}
	}

	if unitTestInp.EvaluationInterval == 0 {
		unitTestInp.EvaluationInterval = model.Duration(1 * time.Minute)
	}

	evalInterval := time.Duration(unitTestInp.EvaluationInterval)

	// Giving number for groups mentioned in the file for ordering.
	// Lower number group should be evaluated before higher number group.
	groupOrderMap := make(map[string]int)
	for i, gn := range unitTestInp.GroupEvalOrder {
		if _, ok := groupOrderMap[gn]; ok {
			return []error{fmt.Errorf("group name repeated in evaluation order: %s", gn)}
		}
		groupOrderMap[gn] = i
	}

	// Testing.
	var errs []error
	for _, t := range unitTestInp.Tests {
		ers := t.test(evalInterval, groupOrderMap, queryOpts, unitTestInp.RuleFiles...)
		if ers != nil {
			errs = append(errs, ers...)
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// unitTestFile holds the contents of a single unit test file.
type unitTestFile struct {
	RuleFiles          []string       `yaml:"rule_files"`
	EvaluationInterval model.Duration `yaml:"evaluation_interval,omitempty"`
	GroupEvalOrder     []string       `yaml:"group_eval_order"`
	Tests              []testGroup    `yaml:"tests"`
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
		if len(m) <= 0 {
			fmt.Fprintln(os.Stderr, "  WARNING: no file match pattern", rf)
		}
		globbedFiles = append(globbedFiles, m...)
	}
	utf.RuleFiles = globbedFiles
	return nil
}

// testGroup is a group of input series and tests associated with it.
type testGroup struct {
	Interval        model.Duration   `yaml:"interval"`
	InputSeries     []series         `yaml:"input_series"`
	AlertRuleTests  []alertTestCase  `yaml:"alert_rule_test,omitempty"`
	PromqlExprTests []promqlTestCase `yaml:"promql_expr_test,omitempty"`
	ExternalLabels  labels.Labels    `yaml:"external_labels,omitempty"`
	ExternalURL     string           `yaml:"external_url,omitempty"`
	TestGroupName   string           `yaml:"name,omitempty"`
}

// test performs the unit tests.
func (tg *testGroup) test(evalInterval time.Duration, groupOrderMap map[string]int, queryOpts promql.LazyLoaderOpts, ruleFiles ...string) []error {
	// Setup testing suite.
	suite, err := promql.NewLazyLoader(nil, tg.seriesLoadingString(), queryOpts)
	if err != nil {
		return []error{err}
	}
	defer suite.Close()
	suite.SubqueryInterval = evalInterval

	// Load the rule files.
	opts := &rules.ManagerOptions{
		QueryFunc:  rules.EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		Appendable: suite.Storage(),
		Context:    context.Background(),
		NotifyFunc: func(ctx context.Context, expr string, alerts ...*rules.Alert) {},
		Logger:     log.NewNopLogger(),
	}
	m := rules.NewManager(opts)
	groupsMap, ers := m.LoadGroups(time.Duration(tg.Interval), tg.ExternalLabels, tg.ExternalURL, nil, ruleFiles...)
	if ers != nil {
		return ers
	}
	groups := orderedGroups(groupsMap, groupOrderMap)

	// Bounds for evaluating the rules.
	mint := time.Unix(0, 0).UTC()
	maxt := mint.Add(tg.maxEvalTime())

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
						evalErrs = append(evalErrs, fmt.Errorf("    rule: %s, time: %s, err: %v",
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

		for {
			if !(curr < len(alertEvalTimes) && ts.Sub(mint) <= time.Duration(alertEvalTimes[curr]) &&
				time.Duration(alertEvalTimes[curr]) < ts.Add(evalInterval).Sub(mint)) {
				break
			}

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
								Labels:      append(labels.Labels{}, a.Labels...),
								Annotations: append(labels.Labels{}, a.Annotations...),
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

				if !reflect.DeepEqual(expAlerts, gotAlerts) {
					var testName string
					if tg.TestGroupName != "" {
						testName = fmt.Sprintf("    name: %s,\n", tg.TestGroupName)
					}
					expString := indentLines(expAlerts.String(), "            ")
					gotString := indentLines(gotAlerts.String(), "            ")
					errs = append(errs, fmt.Errorf("%s    alertname: %s, time: %s, \n        exp:%v, \n        got:%v",
						testName, testcase.Alertname, testcase.EvalTime.String(), expString, gotString))
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
				Labels: s.Metric.Copy(),
				Value:  s.V,
			})
		}

		var expSamples []parsedSample
		for _, s := range testCase.ExpSamples {
			lb, err := parser.ParseMetric(s.Labels)
			if err != nil {
				err = fmt.Errorf("labels %q: %w", s.Labels, err)
				errs = append(errs, fmt.Errorf("    expr: %q, time: %s, err: %w", testCase.Expr,
					testCase.EvalTime.String(), err))
				continue Outer
			}
			expSamples = append(expSamples, parsedSample{
				Labels: lb,
				Value:  s.Value,
			})
		}

		sort.Slice(expSamples, func(i, j int) bool {
			return labels.Compare(expSamples[i].Labels, expSamples[j].Labels) <= 0
		})
		sort.Slice(gotSamples, func(i, j int) bool {
			return labels.Compare(gotSamples[i].Labels, gotSamples[j].Labels) <= 0
		})
		if !reflect.DeepEqual(expSamples, gotSamples) {
			errs = append(errs, fmt.Errorf("    expr: %q, time: %s,\n        exp: %v\n        got: %v", testCase.Expr,
				testCase.EvalTime.String(), parsedSamplesString(expSamples), parsedSamplesString(gotSamples)))
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
	q, err := engine.NewInstantQuery(qu, nil, qs, t)
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
			Point:  promql.Point(v),
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
	s := "[\n0:" + indentLines("\n"+la[0].String(), "  ")
	for i, l := range la[1:] {
		s += ",\n" + fmt.Sprintf("%d", i+1) + ":" + indentLines("\n"+l.String(), "  ")
	}
	s += "\n]"

	return s
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
	Labels string  `yaml:"labels"`
	Value  float64 `yaml:"value"`
}

// parsedSample is a sample with parsed Labels.
type parsedSample struct {
	Labels labels.Labels
	Value  float64
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
	return ps.Labels.String() + " " + strconv.FormatFloat(ps.Value, 'E', -1, 64)
}
