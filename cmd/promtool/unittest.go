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
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"

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
		fmt.Println("Unit Testing:", f)
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
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return []error{err}
	}

	var unitTest unitTestFile
	if err := yaml.UnmarshalStrict(b, &unitTest); err != nil {
		return []error{err}
	}
	if err := resolveAndGlobFilepaths(filepath.Dir(filename), &unitTest); err != nil {
		return []error{err}
	}

	interval := unitTest.EvaluationInterval
	if interval == 0 {
		interval = 1 * time.Minute
	}

	// Bounds for evaluating the rules.
	mint := time.Unix(0, 0)
	maxd := unitTest.maxEvalTime()
	maxt := mint.Add(maxd)
	// Rounding off to nearest Eval time (> maxt).
	maxt = maxt.Add(interval / 2).Round(interval)

	// Testing.
	var errs []error
	for _, t := range unitTest.Tests {
		ers := t.test(mint, maxt, interval, unitTest.GroupEvalOrder,
			unitTest.RuleFiles...)
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
	EvaluationInterval time.Duration  `yaml:"evaluation_interval,omitempty"`
	GroupEvalOrder     groupEvalOrder `yaml:"group_eval_order"`
	Tests              []testGroup    `yaml:"tests"`
}

type groupEvalOrder map[string]int

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (geo *groupEvalOrder) UnmarshalYAML(unmarshal func(interface{}) error) error {
	plain := []string{}
	if err := unmarshal(&plain); err != nil {
		return err
	}
	// Giving number for groups mentioned in the file for ordering.
	// Lower number group should be evaluated before higher number group.
	*geo = make(map[string]int)
	for i, n := range plain {
		if _, ok := (*geo)[n]; ok {
			return errors.Errorf("group name repeated in evaluation order: %s", n)
		}
		(*geo)[n] = i
	}

	return nil
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
		globbedFiles = append(globbedFiles, m...)
	}
	utf.RuleFiles = globbedFiles
	return nil
}

// testGroup is a group of input series and tests associated with it.
type testGroup struct {
	Interval        time.Duration    `yaml:"interval"`
	InputSeries     []series         `yaml:"input_series"`
	AlertRuleTests  []alertTestCase  `yaml:"alert_rule_test,omitempty"`
	PromqlExprTests []promqlTestCase `yaml:"promql_expr_test,omitempty"`
	ExternalLabels  labels.Labels    `yaml:"external_labels,omitempty"`
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
		Logger:     log.NewNopLogger(),
	}
	m := rules.NewManager(opts)
	groupsMap, ers := m.LoadGroups(tg.Interval, tg.ExternalLabels, ruleFiles...)
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
		alertsInTest[alert.EvalTime][alert.Name] = struct{}{}

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
				for _, r := range g.Rules() {
					if r.LastError() != nil {
						errs = append(errs, errors.Errorf("    rule: %s, time: %s, err: %v",
							r.Name(), ts.Sub(time.Unix(0, 0)), r.LastError()))
					}
				}
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

			// The same alert name can be present in multiple groups.
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
				gotAlerts := got[testcase.Name]
				sort.Sort(gotAlerts)
				expAlerts := testcase.Alerts
				if !reflect.DeepEqual(expAlerts, gotAlerts) {
					errs = append(errs, errors.Errorf("    alertname: %q, time: %s\n        exp: %s, \n        got: %s",
						testcase.Name, testcase.EvalTime.String(), expAlerts.String(), gotAlerts.String()))
				}
			}

			curr++
		}
	}

	// Checking promql expressions.
	for _, testCase := range tg.PromqlExprTests {
		got, err := query(suite.Context(), testCase.Expr, mint.Add(testCase.EvalTime),
			suite.QueryEngine(), suite.Queryable())
		if err != nil {
			errs = append(errs, errors.Errorf("    expr: %q, time: %s, err: %s", testCase.Expr,
				testCase.EvalTime.String(), err.Error()))
			continue
		}

		var gotSamples samples
		for _, s := range got {
			gotSamples = append(gotSamples, sample{
				Labels: s.Metric.Copy(),
				Value:  s.V,
			})
		}
		sort.Sort(gotSamples)

		expSamples := testCase.ExpSamples
		sort.Sort(expSamples)
		if !reflect.DeepEqual(expSamples, gotSamples) {
			errs = append(errs, errors.Errorf("    expr: %q, time: %s\n        exp:%#v\n        got:%#v", testCase.Expr,
				testCase.EvalTime.String(), expSamples.String(), gotSamples.String()))
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
		return nil, errors.New("rule result is not a vector or scalar")
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
	Labels      labels.Labels `yaml:"exp_labels"`
	Annotations labels.Labels `yaml:"exp_annotations"`
}

func (la *labelAndAnnotation) String() string {
	return "Labels:" + la.Labels.String() + " Annotations:" + la.Annotations.String()
}

type series struct {
	Series string `yaml:"series"`
	Values string `yaml:"values"`
}

type alertTestCase struct {
	EvalTime time.Duration        `yaml:"eval_time"`
	Name     string               `yaml:"alertname"`
	Alerts   labelsAndAnnotations `yaml:"exp_alerts"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *alertTestCase) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain alertTestCase
	if err := unmarshal((*plain)(a)); err != nil {
		return err
	}

	if a.Name == "" {
		return errors.New("'alertname' can't be empty")
	}
	// User gives only the labels from alerting rule, which doesn't
	// include this label (added by Prometheus during Eval).
	for i := range a.Alerts {
		builder := labels.NewBuilder(a.Alerts[i].Labels)
		builder.Set(labels.AlertName, a.Name)
		a.Alerts[i].Labels = builder.Labels()
	}
	sort.Sort(a.Alerts)

	return nil
}

type promqlTestCase struct {
	Expr       string        `yaml:"expr"`
	EvalTime   time.Duration `yaml:"eval_time"`
	ExpSamples samples       `yaml:"exp_samples"`
}

type samples []sample

func (ss samples) String() string {
	if len(ss) == 0 {
		return "nil"
	}
	s := ss[0].String()
	for _, sample := range ss[1:] {
		s += ", " + sample.String()
	}
	return s
}

// Len implements the sort.Interface interface.
func (ss samples) Len() int { return len(ss) }

// Swap implements the sort.Interface interface.
func (ss samples) Swap(i, j int) { ss[i], ss[j] = ss[j], ss[i] }

// Less implements the sort.Interface interface.
func (ss samples) Less(i, j int) bool { return labels.Compare(ss[i].Labels, ss[j].Labels) <= 0 }

type sample struct {
	Labels labels.Labels
	Value  float64
}

func (s *sample) String() string {
	return s.Labels.String() + " " + strconv.FormatFloat(s.Value, 'E', -1, 64)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *sample) UnmarshalYAML(unmarshal func(interface{}) error) error {
	plain := struct {
		Labels string  `yaml:"labels"`
		Value  float64 `yaml:"value"`
	}{}
	if err := unmarshal(&plain); err != nil {
		return err
	}
	lbs, err := promql.ParseMetric(plain.Labels)
	if err != nil {
		err = errors.Wrapf(err, "labels %q", plain.Labels)
		return err
	}
	s.Labels = lbs
	s.Value = plain.Value
	sort.Sort(s.Labels)
	return nil
}
