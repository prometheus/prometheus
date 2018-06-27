// Copyright 2015 The Prometheus Authors
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
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/promlint"
)

func main() {
	app := kingpin.New(filepath.Base(os.Args[0]), "Tooling for the Prometheus monitoring system.")
	app.Version(version.Print("promtool"))
	app.HelpFlag.Short('h')

	checkCmd := app.Command("check", "Check the resources for validity.")
	testCmd := app.Command("test", "Unit testing.")

	checkConfigCmd := checkCmd.Command("config", "Check if the config files are valid or not.")
	configFiles := checkConfigCmd.Arg(
		"config-files",
		"The config files to check.",
	).Required().ExistingFiles()

	checkRulesCmd := checkCmd.Command("rules", "Check if the rule files are valid or not.")
	ruleFiles := checkRulesCmd.Arg(
		"rule-files",
		"The rule files to check.",
	).Required().ExistingFiles()

	ruleTestCmd := testCmd.Command("rules", "Unit tests for rules.")
	ruleTestFiles := ruleTestCmd.Arg(
		"test-rule-file",
		"The unit test file.",
	).Required().ExistingFiles()

	checkMetricsCmd := checkCmd.Command("metrics", checkMetricsUsage)

	updateCmd := app.Command("update", "Update the resources to newer formats.")
	updateRulesCmd := updateCmd.Command("rules", "Update rules from the 1.x to 2.x format.")
	ruleFilesUp := updateRulesCmd.Arg("rule-files", "The rule files to update.").Required().ExistingFiles()

	queryCmd := app.Command("query", "Run query against a Prometheus server.")
	queryInstantCmd := queryCmd.Command("instant", "Run instant query.")
	queryServer := queryInstantCmd.Arg("server", "Prometheus server to query.").Required().URL()
	queryExpr := queryInstantCmd.Arg("expr", "PromQL query expression.").Required().String()

	queryRangeCmd := queryCmd.Command("range", "Run range query.")
	queryRangeServer := queryRangeCmd.Arg("server", "Prometheus server to query.").Required().URL()
	queryRangeExpr := queryRangeCmd.Arg("expr", "PromQL query expression.").Required().String()
	queryRangeBegin := queryRangeCmd.Flag("start", "Query range start time (RFC3339 or Unix timestamp).").String()
	queryRangeEnd := queryRangeCmd.Flag("end", "Query range end time (RFC3339 or Unix timestamp).").String()

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case checkConfigCmd.FullCommand():
		os.Exit(CheckConfig(*configFiles...))

	case checkRulesCmd.FullCommand():
		os.Exit(CheckRules(*ruleFiles...))

	case checkMetricsCmd.FullCommand():
		os.Exit(CheckMetrics())

	case updateRulesCmd.FullCommand():
		os.Exit(UpdateRules(*ruleFilesUp...))

	case queryInstantCmd.FullCommand():
		os.Exit(QueryInstant(*queryServer, *queryExpr))

	case queryRangeCmd.FullCommand():
		os.Exit(QueryRange(*queryRangeServer, *queryRangeExpr, *queryRangeBegin, *queryRangeEnd))

	case ruleTestCmd.FullCommand():
		os.Exit(RulesUnitTest(*ruleTestFiles...))
	}

}

// CheckConfig validates configuration files.
func CheckConfig(files ...string) int {
	failed := false

	for _, f := range files {
		ruleFiles, err := checkConfig(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
		} else {
			fmt.Printf("  SUCCESS: %d rule files found\n", len(ruleFiles))
		}
		fmt.Println()

		for _, rf := range ruleFiles {
			if n, err := checkRules(rf); err != nil {
				fmt.Fprintln(os.Stderr, "  FAILED:", err)
				failed = true
			} else {
				fmt.Printf("  SUCCESS: %d rules found\n", n)
			}
			fmt.Println()
		}
	}
	if failed {
		return 1
	}
	return 0
}

func checkFileExists(fn string) error {
	// Nothing set, nothing to error on.
	if fn == "" {
		return nil
	}
	_, err := os.Stat(fn)
	return err
}

func checkConfig(filename string) ([]string, error) {
	fmt.Println("Checking", filename)

	cfg, err := config.LoadFile(filename)
	if err != nil {
		return nil, err
	}

	var ruleFiles []string
	for _, rf := range cfg.RuleFiles {
		rfs, err := filepath.Glob(rf)
		if err != nil {
			return nil, err
		}
		// If an explicit file was given, error if it is not accessible.
		if !strings.Contains(rf, "*") {
			if len(rfs) == 0 {
				return nil, fmt.Errorf("%q does not point to an existing file", rf)
			}
			if err := checkFileExists(rfs[0]); err != nil {
				return nil, fmt.Errorf("error checking rule file %q: %s", rfs[0], err)
			}
		}
		ruleFiles = append(ruleFiles, rfs...)
	}

	for _, scfg := range cfg.ScrapeConfigs {
		if err := checkFileExists(scfg.HTTPClientConfig.BearerTokenFile); err != nil {
			return nil, fmt.Errorf("error checking bearer token file %q: %s", scfg.HTTPClientConfig.BearerTokenFile, err)
		}

		if err := checkTLSConfig(scfg.HTTPClientConfig.TLSConfig); err != nil {
			return nil, err
		}

		for _, kd := range scfg.ServiceDiscoveryConfig.KubernetesSDConfigs {
			if err := checkTLSConfig(kd.TLSConfig); err != nil {
				return nil, err
			}
		}

		for _, filesd := range scfg.ServiceDiscoveryConfig.FileSDConfigs {
			for _, file := range filesd.Files {
				files, err := filepath.Glob(file)
				if err != nil {
					return nil, err
				}
				if len(files) != 0 {
					// There was at least one match for the glob and we can assume checkFileExists
					// for all matches would pass, we can continue the loop.
					continue
				}
				fmt.Printf("  WARNING: file %q for file_sd in scrape job %q does not exist\n", file, scfg.JobName)
			}
		}
	}

	return ruleFiles, nil
}

func checkTLSConfig(tlsConfig config_util.TLSConfig) error {
	if err := checkFileExists(tlsConfig.CertFile); err != nil {
		return fmt.Errorf("error checking client cert file %q: %s", tlsConfig.CertFile, err)
	}
	if err := checkFileExists(tlsConfig.KeyFile); err != nil {
		return fmt.Errorf("error checking client key file %q: %s", tlsConfig.KeyFile, err)
	}

	if len(tlsConfig.CertFile) > 0 && len(tlsConfig.KeyFile) == 0 {
		return fmt.Errorf("client cert file %q specified without client key file", tlsConfig.CertFile)
	}
	if len(tlsConfig.KeyFile) > 0 && len(tlsConfig.CertFile) == 0 {
		return fmt.Errorf("client key file %q specified without client cert file", tlsConfig.KeyFile)
	}

	return nil
}

// CheckRules validates rule files.
func CheckRules(files ...string) int {
	failed := false

	for _, f := range files {
		if n, errs := checkRules(f); errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			failed = true
		} else {
			fmt.Printf("  SUCCESS: %d rules found\n", n)
		}
		fmt.Println()
	}
	if failed {
		return 1
	}
	return 0
}

func checkRules(filename string) (int, []error) {
	fmt.Println("Checking", filename)

	rgs, errs := rulefmt.ParseFile(filename)
	if errs != nil {
		return 0, errs
	}

	numRules := 0
	for _, rg := range rgs.Groups {
		numRules += len(rg.Rules)
	}

	return numRules, nil
}

// UpdateRules updates the rule files.
func UpdateRules(files ...string) int {
	failed := false

	for _, f := range files {
		if err := updateRules(f); err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
		}
	}

	if failed {
		return 1
	}
	return 0
}

func updateRules(filename string) error {
	fmt.Println("Updating", filename)

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	rules, err := promql.ParseStmts(string(content))
	if err != nil {
		return err
	}

	yamlRG := &rulefmt.RuleGroups{
		Groups: []rulefmt.RuleGroup{{
			Name: filename,
		}},
	}

	yamlRules := make([]rulefmt.Rule, 0, len(rules))

	for _, rule := range rules {
		switch r := rule.(type) {
		case *promql.AlertStmt:
			yamlRules = append(yamlRules, rulefmt.Rule{
				Alert:       r.Name,
				Expr:        r.Expr.String(),
				For:         model.Duration(r.Duration),
				Labels:      r.Labels.Map(),
				Annotations: r.Annotations.Map(),
			})
		case *promql.RecordStmt:
			yamlRules = append(yamlRules, rulefmt.Rule{
				Record: r.Name,
				Expr:   r.Expr.String(),
				Labels: r.Labels.Map(),
			})
		default:
			panic("unknown statement type")
		}
	}

	yamlRG.Groups[0].Rules = yamlRules
	y, err := yaml.Marshal(yamlRG)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename+".yml", y, 0666)
}

var checkMetricsUsage = strings.TrimSpace(`
Pass Prometheus metrics over stdin to lint them for consistency and correctness.

examples:

$ cat metrics.prom | promtool check metrics

$ curl -s http://localhost:9090/metrics | promtool check metrics
`)

// CheckMetrics performs a linting pass on input metrics.
func CheckMetrics() int {
	l := promlint.New(os.Stdin)
	problems, err := l.Lint()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error while linting:", err)
		return 1
	}

	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p.Metric, p.Text)
	}

	if len(problems) > 0 {
		return 3
	}

	return 0
}

// QueryInstant performs an instant query against a Prometheus server.
func QueryInstant(url *url.URL, query string) int {
	config := api.Config{
		Address: url.String(),
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	// Run query against client.
	api := v1.NewAPI(c)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, err := api.Query(ctx, query, time.Now())
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		return 1
	}

	fmt.Println(val.String())

	return 0
}

// QueryRange performs a range query against a Prometheus server.
func QueryRange(url *url.URL, query string, start string, end string) int {
	config := api.Config{
		Address: url.String(),
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	var stime, etime time.Time

	if end == "" {
		etime = time.Now()
	} else {
		etime, err = parseTime(end)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing end time:", err)
			return 1
		}
	}

	if start == "" {
		stime = etime.Add(-5 * time.Minute)
	} else {
		stime, err = parseTime(start)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing start time:", err)
		}
	}

	if !stime.Before(etime) {
		fmt.Fprintln(os.Stderr, "start time is not before end time")
	}

	resolution := math.Max(math.Floor(etime.Sub(stime).Seconds()/250), 1)
	// Convert seconds to nanoseconds such that time.Duration parses correctly.
	step := time.Duration(resolution * 1e9)

	// Run query against client.
	api := v1.NewAPI(c)
	r := v1.Range{Start: stime, End: etime, Step: step}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, err := api.QueryRange(ctx, query, r)
	cancel()

	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		return 1
	}

	fmt.Println(val.String())
	return 0
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		return time.Unix(int64(s), int64(ns*float64(time.Second))), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

// RulesUnitTest does unit testing of rules based on the unit testing files provided.
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

	// Bounds for evaluating the rules.
	mint := time.Unix(0, 0)
	maxd := unitTestInp.maxEvalTime()
	maxt := mint.Add(maxd)
	// Rounding off to nearest Eval time (> maxt).
	maxt = maxt.Add(unitTestInp.EvaluationInterval / 2).Round(unitTestInp.EvaluationInterval)

	// Testing.
	var errs []error
	for _, t := range unitTestInp.Tests {
		ers := t.test(mint, maxt, unitTestInp.EvaluationInterval, unitTestInp.RuleFiles...)
		if ers != nil {
			errs = append(errs, ers...)
		}
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

type unitTestFile struct {
	RuleFiles          []string      `yaml:"rule_files"`
	EvaluationInterval time.Duration `yaml:"evaluation_interval,omitempty"`
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

type testGroup struct {
	Interval       time.Duration    `yaml:"interval"`
	InputSeries    []series         `yaml:"input_series"`
	AlertRuleTest  []alertTestCase  `yaml:"alert_rule_test,omitempty"`
	PromqlExprTest []promqlTestCase `yaml:"promql_expr_test,omitempty"`
}

func (tg *testGroup) test(mint, maxt time.Time, evalInterval time.Duration, ruleFiles ...string) []error {

	// Setup testing suite.
	suite, err := promql.NewTest(nil, tg.seriesLoadingString())
	if err != nil {
		return []error{err}
	}
	defer suite.Close()

	err = suite.Run()
	if err != nil {
		return []error{err}
	}

	// Load the rule files.
	opts := &rules.ManagerOptions{
		QueryFunc:  rules.EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		Appendable: suite.Storage(),
		Context:    context.Background(),
		NotifyFunc: func(ctx context.Context, expr string, alerts ...*rules.Alert) error { return nil },
	}
	m := rules.NewManager(opts)
	groupsMap, ers := m.LoadGroups(tg.Interval, ruleFiles...)
	if ers != nil {
		return ers
	}
	var groups []*rules.Group
	for _, g := range groupsMap {
		groups = append(groups, g)
	}

	// Pre-processing some data for testing alerts.
	// All this preparating so that we can test the alerts as and when we evaluate
	// the rules so that we can avoid storing them in memory, as number of eval might
	// get very big.

	// All the `eval_time` for which we have unit tests.
	var alertEvalTimes []time.Duration
	// Map of all the eval_time+alertname combination present in the unit tests.
	presentInTest := make(map[time.Duration]map[string]struct{})
	// Map of all the unit tests for given eval_time.
	alertTests := make(map[time.Duration][]alertTestCase)
	for _, art := range tg.AlertRuleTest {
		pos := func() int {
			for i, d := range alertEvalTimes {
				if d > art.EvalTime {
					return i
				}
			}
			return len(alertEvalTimes)
		}()
		alertEvalTimes = append(alertEvalTimes[:pos], append([]time.Duration{art.EvalTime}, alertEvalTimes[pos:]...)...)

		if _, ok := presentInTest[art.EvalTime]; !ok {
			presentInTest[art.EvalTime] = make(map[string]struct{})
		}
		presentInTest[art.EvalTime][art.Alertname] = struct{}{}

		if _, ok := alertTests[art.EvalTime]; ok {
			alertTests[art.EvalTime] = []alertTestCase{art}
		} else {
			alertTests[art.EvalTime] = append(alertTests[art.EvalTime], art)
		}
	}

	// Current index in alertEvalTimes what we are looking at.
	curr := 0

	var errs []error
	for ts := mint; maxt.Sub(ts) > 0; ts = ts.Add(evalInterval) {
		// Collects the alerts asked in unit testing.
		for _, g := range groups {
			g.Eval(suite.Context(), ts)
		}

		for curr < len(alertEvalTimes) && ts.Sub(mint) <= alertEvalTimes[curr] && alertEvalTimes[curr] < ts.Add(evalInterval).Sub(mint) {
			// If the eval_time=alertEvalTimes[curr] lies between `ts` and `ts+evalInterval`
			// (or equal to ts), then we compare alerts with the Eval at `ts`.
			t := alertEvalTimes[curr]

			presentNames := presentInTest[t]
			got := make(map[string]labelsAndAnnotations)

			for _, g := range groups {
				grules := g.Rules()

				for _, r := range grules {
					ar, ok := r.(*rules.AlertingRule)
					if !ok {
						continue
					}
					if _, ok := presentNames[ar.Name()]; !ok {
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

					if _, ok := got[ar.Name()]; ok {
						got[ar.Name()] = append(got[ar.Name()], alerts...)
					} else {
						got[ar.Name()] = alerts
					}

				}

			}

			for _, testCase := range alertTests[t] {
				// Checking alerts.
				gotAlerts, ok := got[testCase.Alertname]
				if !ok {
					gotAlerts = labelsAndAnnotations{}
				}

				var expAlerts labelsAndAnnotations
				for _, a := range testCase.ExpAlerts {
					// User gives only the labels from alerting rule, which doesn't
					// include this label (added by Prometheus during Eval).
					a.ExpLabels[labels.AlertName] = testCase.Alertname

					expAlerts = append(expAlerts, labelAndAnnotation{
						Labels:      labels.FromMap(a.ExpLabels),
						Annotations: labels.FromMap(a.ExpAnnotations),
					})
				}

				if gotAlerts.Len() != expAlerts.Len() {
					errs = append(errs, fmt.Errorf("    alertname:%s, time:%s, \n        exp:%#v, \n        got:%#v",
						testCase.Alertname, testCase.EvalTime.String(), expAlerts.String(), gotAlerts.String()))
				} else {
					sort.Sort(gotAlerts)
					sort.Sort(expAlerts)

					if !reflect.DeepEqual(expAlerts, gotAlerts) {
						errs = append(errs, fmt.Errorf("    alertname:%s, time:%s, \n        exp:%#v, \n        got:%#v",
							testCase.Alertname, testCase.EvalTime.String(), expAlerts.String(), gotAlerts.String()))
					}
				}

			}

			curr++
		}
	}

	// Checking promql expressions.
	for _, testCase := range tg.PromqlExprTest {
		got, err := query(suite.Context(), testCase.Expr, mint.Add(testCase.EvalTime),
			suite.QueryEngine(), suite.Queryable())
		if err != nil {
			errs = append(errs, fmt.Errorf("    expr:'%s', time:%s, err:%s", testCase.Expr,
				testCase.EvalTime.String(), err.Error()))
		}

		var gotSamples []parsedSample
		for _, s := range got {
			gotSamples = append(gotSamples, parsedSample{
				Labels: s.Metric.Copy(),
				Value:  s.V,
			})
		}

		var expSamples []parsedSample
		parseError := false
		for _, s := range testCase.ExpSamples {
			lb, err := promql.ParseMetric(s.Labels)
			if err != nil {
				errs = append(errs, fmt.Errorf("    expr:'%s', time:%s, err:%s", testCase.Expr,
					testCase.EvalTime.String(), err.Error()))
				parseError = true
				break
			}
			expSamples = append(expSamples, parsedSample{
				Labels: lb,
				Value:  s.Value,
			})
		}

		if !parseError && !reflect.DeepEqual(expSamples, gotSamples) {
			errs = append(errs, fmt.Errorf("    expr:'%s', time:%s, \n        exp:%#v, \n        got:%#v", testCase.Expr,
				testCase.EvalTime.String(), parsedSampleStringify(expSamples), parsedSampleStringify(gotSamples)))
		}

	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (tg *testGroup) seriesLoadingString() string {
	result := ""
	result += "load " + shortDur(tg.Interval) + "\n"
	for _, is := range tg.InputSeries {
		result += "  " + is.Series + " " + is.Values + "\n"
	}
	return result
}

// https://stackoverflow.com/a/41336257/6219247
func shortDur(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

func (tg *testGroup) maxEvalTime() time.Duration {
	var maxd time.Duration
	for _, art := range tg.AlertRuleTest {
		if art.EvalTime > maxd {
			maxd = art.EvalTime
		}
	}
	for _, pet := range tg.PromqlExprTest {
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
	l1h, l2h := la[i].Labels.Hash(), la[j].Labels.Hash()
	if l1h != l2h {
		return l1h < l2h
	}
	return la[i].Annotations.Hash() < la[j].Annotations.Hash()
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

type parsedSample struct {
	Labels labels.Labels
	Value  float64
}

func parsedSampleStringify(pss []parsedSample) string {
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
