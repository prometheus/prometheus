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
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
)

// RulesUnitTest does unit testing of rules based on the unit testing files provided.
// More info about the file format can be found in the docs.
func RulesUnitTest(files ...string) int {
	failed := false

	for _, f := range files {
		if errs := ruleUnitTest(f); errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			failed = true
		} else {
			fmt.Println("  SUCCESS")
		}
		fmt.Println()
	}
	if failed {
		return 1
	}
	return 0
}

func ruleUnitTest(filename string) []error {
	fmt.Println("Unit Testing: ", filename)

	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return []error{err}
	}

	var unitTestInp unitTestFile
	if err := yaml.UnmarshalStrict(b, &unitTestInp); err != nil {
		return []error{err}
	}

	if unitTestInp.EvaluationInterval == 0 {
		unitTestInp.EvaluationInterval = 1 * time.Minute
	}

	// Bounds for evaluating the rules.
	mint := time.Unix(0, 0)
	maxd := unitTestInp.maxEvalTime()
	maxt := mint.Add(maxd)
	// Rounding off to nearest Eval time (> maxt).
	maxt = maxt.Add(unitTestInp.EvaluationInterval / 2).Round(unitTestInp.EvaluationInterval)

	// Giving number for groups mentioned in the file for ordering.
	// Lower number group should be evaluated before higher number group.
	groupOrderMap := make(map[string]int)
	for i, gn := range unitTestInp.GroupEvalOrder {
		if _, ok := groupOrderMap[gn]; ok {
			return []error{fmt.Errorf("Group name repeated in evaluation order: %s", gn)}
		}
		groupOrderMap[gn] = i
	}

	// Testing.
	var errs []error
	for _, t := range unitTestInp.Tests {
		ers := t.test(mint, maxt, unitTestInp.EvaluationInterval, groupOrderMap,
			unitTestInp.RuleFiles...)
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
	RuleFiles          []string      `yaml:"rule_files"`
	EvaluationInterval time.Duration `yaml:"evaluation_interval,omitempty"`
	GroupEvalOrder     []string      `yaml:"group_eval_order"`
	Tests              []testGroup   `yaml:"tests"`
}

func (utf *unitTestFile) maxEvalTime() time.Duration {
	var maxd time.Duration
	for _, t := range utf.Tests {
		d := t.maxEvalTime()
		if d > maxd {
			maxd = d
		}
	}
	return maxd
}

// testGroup is a group of input series and tests associated with it.
type testGroup struct {
	Interval        time.Duration    `yaml:"interval"`
	InputSeries     []series         `yaml:"input_series"`
	AlertRuleTests  []alertTestCase  `yaml:"alert_rule_test,omitempty"`
	PromqlExprTests []promqlTestCase `yaml:"promql_expr_test,omitempty"`
}

// test performs the unit tests.
func (tg *testGroup) test(mint, maxt time.Time, evalInterval time.Duration, groupOrderMap map[string]int, ruleFiles ...string) []error {
	// Setup testing suite.
	suite, err := promql.NewLazyLoader(nil, tg.seriesLoadingString())
	if err != nil {
		return []error{err}
	}
	defer suite.Close()

	// Load the rule files.
	opts := &rules.ManagerOptions{
		QueryFunc:  rules.EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		Appendable: suite.Storage(),
		Context:    context.Background(),
		NotifyFunc: func(ctx context.Context, expr string, alerts ...*rules.Alert) {},
		Logger:     &dummyLogger{},
	}
	m := rules.NewManager(opts)
	groupsMap, ers := m.LoadGroups(tg.Interval, ruleFiles...)
	if ers != nil {
		return ers
	}
	groups := orderedGroups(groupsMap, groupOrderMap)

	// Pre-processing some data for testing alerts.
	// All this preparation is so that we can test alerts as we evaluate the rules.
	// This avoids storing them in memory, as the number of evals might be high.

	// All the `eval_time` for which we have unit tests for alerts.
	alertEvalTimesMap := map[time.Duration]struct{}{}
	// Map of all the eval_time+alertname combination present in the unit tests.
	alertsInTest := make(map[time.Duration]map[string]struct{})
	// Map of all the unit tests for given eval_time.
	alertTests := make(map[time.Duration][]alertTestCase)
	for _, alert := range tg.AlertRuleTests {
		alertEvalTimesMap[alert.EvalTime] = struct{}{}

		if _, ok := alertsInTest[alert.EvalTime]; !ok {
			alertsInTest[alert.EvalTime] = make(map[string]struct{})
		}
		alertsInTest[alert.EvalTime][alert.Alertname] = struct{}{}

		alertTests[alert.EvalTime] = append(alertTests[alert.EvalTime], alert)
	}
	alertEvalTimes := make([]time.Duration, 0, len(alertEvalTimesMap))
	for k := range alertEvalTimesMap {
		alertEvalTimes = append(alertEvalTimes, k)
	}
	sort.Slice(alertEvalTimes, func(i, j int) bool {
		return alertEvalTimes[i] < alertEvalTimes[j]
	})

	// Current index in alertEvalTimes what we are looking at.
	curr := 0

	var errs []error
	for ts := mint; ts.Before(maxt); ts = ts.Add(evalInterval) {
		// Collects the alerts asked for unit testing.
		suite.WithSamplesTill(ts, func(err error) {
			if err != nil {
				errs = append(errs, err)
				return
			}
			for _, g := range groups {
				g.Eval(suite.Context(), ts)
			}
		})
		if len(errs) > 0 {
			return errs
		}

		for {
			if !(curr < len(alertEvalTimes) && ts.Sub(mint) <= alertEvalTimes[curr] &&
				alertEvalTimes[curr] < ts.Add(evalInterval).Sub(mint)) {
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
					a.ExpLabels[labels.AlertName] = testcase.Alertname

					expAlerts = append(expAlerts, labelAndAnnotation{
						Labels:      labels.FromMap(a.ExpLabels),
						Annotations: labels.FromMap(a.ExpAnnotations),
					})
				}

				if gotAlerts.Len() != expAlerts.Len() {
					errs = append(errs, fmt.Errorf("    alertname:%s, time:%s, \n        exp:%#v, \n        got:%#v",
						testcase.Alertname, testcase.EvalTime.String(), expAlerts.String(), gotAlerts.String()))
				} else {
					sort.Sort(gotAlerts)
					sort.Sort(expAlerts)

					if !reflect.DeepEqual(expAlerts, gotAlerts) {
						errs = append(errs, fmt.Errorf("    alertname:%s, time:%s, \n        exp:%#v, \n        got:%#v",
							testcase.Alertname, testcase.EvalTime.String(), expAlerts.String(), gotAlerts.String()))
					}
				}
			}

			curr++
		}
	}

	// Checking promql expressions.
Outer:
	for _, testCase := range tg.PromqlExprTests {
		got, err := query(suite.Context(), testCase.Expr, mint.Add(testCase.EvalTime),
			suite.QueryEngine(), suite.Queryable())
		if err != nil {
			errs = append(errs, fmt.Errorf("    expr:'%s', time:%s, err:%s", testCase.Expr,
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
			lb, err := promql.ParseMetric(s.Labels)
			if err != nil {
				errs = append(errs, fmt.Errorf("    expr:'%s', time:%s, err:%s", testCase.Expr,
					testCase.EvalTime.String(), err.Error()))
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
			errs = append(errs, fmt.Errorf("    expr:'%s', time:%s, \n        exp:%#v, \n        got:%#v", testCase.Expr,
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
	result := ""
	result += "load " + shortDuration(tg.Interval) + "\n"
	for _, is := range tg.InputSeries {
		result += "  " + is.Series + " " + is.Values + "\n"
	}
	return result
}

func shortDuration(d time.Duration) string {
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
	var maxd time.Duration
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
	return maxd
}

func query(ctx context.Context, qs string, t time.Time, engine *promql.Engine, qu storage.Queryable) (promql.Vector, error) {
	q, err := engine.NewInstantQuery(qu, qs, t)
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
		return nil, fmt.Errorf("rule result is not a vector or scalar")
	}
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
	s := "[" + la[0].String()
	for _, l := range la[1:] {
		s += ", " + l.String()
	}
	s += "]"

	return s
}

type labelAndAnnotation struct {
	Labels      labels.Labels
	Annotations labels.Labels
}

func (la *labelAndAnnotation) String() string {
	return "Labels:" + la.Labels.String() + " Annotations:" + la.Annotations.String()
}

type series struct {
	Series string `yaml:"series"`
	Values string `yaml:"values"`
}

type alertTestCase struct {
	EvalTime  time.Duration `yaml:"eval_time"`
	Alertname string        `yaml:"alertname"`
	ExpAlerts []alert       `yaml:"exp_alerts"`
}

type alert struct {
	ExpLabels      map[string]string `yaml:"exp_labels"`
	ExpAnnotations map[string]string `yaml:"exp_annotations"`
}

type promqlTestCase struct {
	Expr       string        `yaml:"expr"`
	EvalTime   time.Duration `yaml:"eval_time"`
	ExpSamples []sample      `yaml:"exp_samples"`
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
	for _, ps := range pss[0:] {
		s += ", " + ps.String()
	}
	return s
}

func (ps *parsedSample) String() string {
	return ps.Labels.String() + " " + strconv.FormatFloat(ps.Value, 'E', -1, 64)
}

type dummyLogger struct{}

func (l *dummyLogger) Log(keyvals ...interface{}) error {
	return nil
}
