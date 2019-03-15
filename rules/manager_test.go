// Copyright 2013 The Prometheus Authors
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

package rules

import (
	"context"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestAlertingRule(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 5m
			http_requests{job="app-server", instance="0", group="canary", severity="overwrite-me"}	75 85  95 105 105  95  85
			http_requests{job="app-server", instance="1", group="canary", severity="overwrite-me"}	80 90 100 110 120 130 140
	`)
	testutil.Ok(t, err)
	defer suite.Close()

	err = suite.Run()
	testutil.Ok(t, err)

	expr, err := promql.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	testutil.Ok(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		time.Minute,
		labels.FromStrings("severity", "{{\"c\"}}ritical"),
		nil, true, nil,
	)
	result := promql.Vector{
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "pending",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "pending",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "firing",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "firing",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
	}

	baseTime := time.Unix(0, 0)

	var tests = []struct {
		time   time.Duration
		result promql.Vector
	}{
		{
			time:   0,
			result: result[:2],
		}, {
			time:   5 * time.Minute,
			result: result[2:],
		}, {
			time:   10 * time.Minute,
			result: result[2:3],
		},
		{
			time:   15 * time.Minute,
			result: nil,
		},
		{
			time:   20 * time.Minute,
			result: nil,
		},
		{
			time:   25 * time.Minute,
			result: result[:1],
		},
		{
			time:   30 * time.Minute,
			result: result[2:3],
		},
	}

	for i, test := range tests {
		t.Logf("case %d", i)

		evalTime := baseTime.Add(test.time)

		res, err := rule.Eval(suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil)
		testutil.Ok(t, err)

		var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
		for _, smpl := range res {
			smplName := smpl.Metric.Get("__name__")
			if smplName == "ALERTS" {
				filteredRes = append(filteredRes, smpl)
			} else {
				// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
				testutil.Equals(t, smplName, "ALERTS_FOR_STATE")
			}
		}
		for i := range test.result {
			test.result[i].T = timestamp.FromTime(evalTime)
		}
		testutil.Assert(t, len(test.result) == len(filteredRes), "%d. Number of samples in expected and actual output don't match (%d vs. %d)", i, len(test.result), len(res))

		sort.Slice(filteredRes, func(i, j int) bool {
			return labels.Compare(filteredRes[i].Metric, filteredRes[j].Metric) < 0
		})
		testutil.Equals(t, test.result, filteredRes)

		for _, aa := range rule.ActiveAlerts() {
			testutil.Assert(t, aa.Labels.Get(model.MetricNameLabel) == "", "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
		}
	}
}

func TestForStateAddSamples(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 5m
			http_requests{job="app-server", instance="0", group="canary", severity="overwrite-me"}	75 85  95 105 105  95  85
			http_requests{job="app-server", instance="1", group="canary", severity="overwrite-me"}	80 90 100 110 120 130 140
	`)
	testutil.Ok(t, err)
	defer suite.Close()

	err = suite.Run()
	testutil.Ok(t, err)

	expr, err := promql.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	testutil.Ok(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		time.Minute,
		labels.FromStrings("severity", "{{\"c\"}}ritical"),
		nil, true, nil,
	)
	result := promql.Vector{
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			Point: promql.Point{V: 1},
		},
	}

	baseTime := time.Unix(0, 0)

	var tests = []struct {
		time            time.Duration
		result          promql.Vector
		persistThisTime bool // If true, it means this 'time' is persisted for 'for'.
	}{
		{
			time:            0,
			result:          append(promql.Vector{}, result[:2]...),
			persistThisTime: true,
		},
		{
			time:   5 * time.Minute,
			result: append(promql.Vector{}, result[2:]...),
		},
		{
			time:   10 * time.Minute,
			result: append(promql.Vector{}, result[2:3]...),
		},
		{
			time:   15 * time.Minute,
			result: nil,
		},
		{
			time:   20 * time.Minute,
			result: nil,
		},
		{
			time:            25 * time.Minute,
			result:          append(promql.Vector{}, result[:1]...),
			persistThisTime: true,
		},
		{
			time:   30 * time.Minute,
			result: append(promql.Vector{}, result[2:3]...),
		},
	}

	var forState float64
	for i, test := range tests {
		t.Logf("case %d", i)
		evalTime := baseTime.Add(test.time)

		if test.persistThisTime {
			forState = float64(evalTime.Unix())
		}
		if test.result == nil {
			forState = float64(value.StaleNaN)
		}

		res, err := rule.Eval(suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil)
		testutil.Ok(t, err)

		var filteredRes promql.Vector // After removing 'ALERTS' samples.
		for _, smpl := range res {
			smplName := smpl.Metric.Get("__name__")
			if smplName == "ALERTS_FOR_STATE" {
				filteredRes = append(filteredRes, smpl)
			} else {
				// If not 'ALERTS_FOR_STATE', it has to be 'ALERTS'.
				testutil.Equals(t, smplName, "ALERTS")
			}
		}
		for i := range test.result {
			test.result[i].T = timestamp.FromTime(evalTime)
			// Updating the expected 'for' state.
			if test.result[i].V >= 0 {
				test.result[i].V = forState
			}
		}
		testutil.Assert(t, len(test.result) == len(filteredRes), "%d. Number of samples in expected and actual output don't match (%d vs. %d)", i, len(test.result), len(res))

		sort.Slice(filteredRes, func(i, j int) bool {
			return labels.Compare(filteredRes[i].Metric, filteredRes[j].Metric) < 0
		})
		testutil.Equals(t, test.result, filteredRes)

		for _, aa := range rule.ActiveAlerts() {
			testutil.Assert(t, aa.Labels.Get(model.MetricNameLabel) == "", "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
		}

	}
}

// sortAlerts sorts `[]*Alert` w.r.t. the Labels.
func sortAlerts(items []*Alert) {
	sort.Slice(items, func(i, j int) bool {
		return labels.Compare(items[i].Labels, items[j].Labels) <= 0
	})
}

func TestForStateRestore(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 5m
		http_requests{job="app-server", instance="0", group="canary", severity="overwrite-me"}	75  85 50 0 0 25 0 0 40 0 120
		http_requests{job="app-server", instance="1", group="canary", severity="overwrite-me"}	125 90 60 0 0 25 0 0 40 0 130
	`)
	testutil.Ok(t, err)
	defer suite.Close()

	err = suite.Run()
	testutil.Ok(t, err)

	expr, err := promql.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	testutil.Ok(t, err)

	opts := &ManagerOptions{
		QueryFunc:       EngineQueryFunc(suite.QueryEngine(), suite.Storage()),
		Appendable:      suite.Storage(),
		TSDB:            suite.Storage(),
		Context:         context.Background(),
		Logger:          log.NewNopLogger(),
		NotifyFunc:      func(ctx context.Context, expr string, alerts ...*Alert) {},
		OutageTolerance: 30 * time.Minute,
		ForGracePeriod:  10 * time.Minute,
	}

	alertForDuration := 25 * time.Minute
	// Initial run before prometheus goes down.
	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		alertForDuration,
		labels.FromStrings("severity", "critical"),
		nil, true, nil,
	)

	group := NewGroup("default", "", time.Second, []Rule{rule}, true, opts)
	groups := make(map[string]*Group)
	groups["default;"] = group

	initialRuns := []time.Duration{0, 5 * time.Minute}

	baseTime := time.Unix(0, 0)
	for _, duration := range initialRuns {
		evalTime := baseTime.Add(duration)
		group.Eval(suite.Context(), evalTime)
	}

	exp := rule.ActiveAlerts()
	for _, aa := range exp {
		testutil.Assert(t, aa.Labels.Get(model.MetricNameLabel) == "", "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
	}
	sort.Slice(exp, func(i, j int) bool {
		return labels.Compare(exp[i].Labels, exp[j].Labels) < 0
	})

	// Prometheus goes down here. We create new rules and groups.
	type testInput struct {
		restoreDuration time.Duration
		alerts          []*Alert

		num          int
		noRestore    bool
		gracePeriod  bool
		downDuration time.Duration
	}

	tests := []testInput{
		{
			// Normal restore (alerts were not firing).
			restoreDuration: 15 * time.Minute,
			alerts:          rule.ActiveAlerts(),
			downDuration:    10 * time.Minute,
		},
		{
			// Testing Outage Tolerance.
			restoreDuration: 40 * time.Minute,
			noRestore:       true,
			num:             2,
		},
		{
			// No active alerts.
			restoreDuration: 50 * time.Minute,
			alerts:          []*Alert{},
		},
	}

	testFunc := func(tst testInput) {
		newRule := NewAlertingRule(
			"HTTPRequestRateLow",
			expr,
			alertForDuration,
			labels.FromStrings("severity", "critical"),
			nil, false, nil,
		)
		newGroup := NewGroup("default", "", time.Second, []Rule{newRule}, true, opts)

		newGroups := make(map[string]*Group)
		newGroups["default;"] = newGroup

		restoreTime := baseTime.Add(tst.restoreDuration)
		// First eval before restoration.
		newGroup.Eval(suite.Context(), restoreTime)
		// Restore happens here.
		newGroup.RestoreForState(restoreTime)

		got := newRule.ActiveAlerts()
		for _, aa := range got {
			testutil.Assert(t, aa.Labels.Get(model.MetricNameLabel) == "", "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
		}
		sort.Slice(got, func(i, j int) bool {
			return labels.Compare(got[i].Labels, got[j].Labels) < 0
		})

		// Checking if we have restored it correctly.
		if tst.noRestore {
			testutil.Equals(t, tst.num, len(got))
			for _, e := range got {
				testutil.Equals(t, e.ActiveAt, restoreTime)
			}
		} else if tst.gracePeriod {
			testutil.Equals(t, tst.num, len(got))
			for _, e := range got {
				testutil.Equals(t, opts.ForGracePeriod, e.ActiveAt.Add(alertForDuration).Sub(restoreTime))
			}
		} else {
			exp := tst.alerts
			testutil.Equals(t, len(exp), len(got))
			sortAlerts(exp)
			sortAlerts(got)
			for i, e := range exp {
				testutil.Equals(t, e.Labels, got[i].Labels)

				// Difference in time should be within 1e6 ns, i.e. 1ms
				// (due to conversion between ns & ms, float64 & int64).
				activeAtDiff := float64(e.ActiveAt.Unix() + int64(tst.downDuration/time.Second) - got[i].ActiveAt.Unix())
				testutil.Assert(t, math.Abs(activeAtDiff) == 0, "'for' state restored time is wrong")
			}
		}
	}

	for _, tst := range tests {
		testFunc(tst)
	}

	// Testing the grace period.
	for _, duration := range []time.Duration{10 * time.Minute, 15 * time.Minute, 20 * time.Minute} {
		evalTime := baseTime.Add(duration)
		group.Eval(suite.Context(), evalTime)
	}
	testFunc(testInput{
		restoreDuration: 25 * time.Minute,
		alerts:          []*Alert{},
		gracePeriod:     true,
		num:             2,
	})
}

func TestStaleness(t *testing.T) {
	storage := testutil.NewStorage(t)
	defer storage.Close()
	engineOpts := promql.EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
	}
	engine := promql.NewEngine(engineOpts)
	opts := &ManagerOptions{
		QueryFunc:  EngineQueryFunc(engine, storage),
		Appendable: storage,
		TSDB:       storage,
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	}

	expr, err := promql.ParseExpr("a + 1")
	testutil.Ok(t, err)
	rule := NewRecordingRule("a_plus_one", expr, labels.Labels{})
	group := NewGroup("default", "", time.Second, []Rule{rule}, true, opts)

	// A time series that has two samples and then goes stale.
	app, _ := storage.Appender()
	app.Add(labels.FromStrings(model.MetricNameLabel, "a"), 0, 1)
	app.Add(labels.FromStrings(model.MetricNameLabel, "a"), 1000, 2)
	app.Add(labels.FromStrings(model.MetricNameLabel, "a"), 2000, math.Float64frombits(value.StaleNaN))

	err = app.Commit()
	testutil.Ok(t, err)

	ctx := context.Background()

	// Execute 3 times, 1 second apart.
	group.Eval(ctx, time.Unix(0, 0))
	group.Eval(ctx, time.Unix(1, 0))
	group.Eval(ctx, time.Unix(2, 0))

	querier, err := storage.Querier(context.Background(), 0, 2000)
	testutil.Ok(t, err)
	defer querier.Close()

	matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a_plus_one")
	testutil.Ok(t, err)

	set, _, err := querier.Select(nil, matcher)
	testutil.Ok(t, err)

	samples, err := readSeriesSet(set)
	testutil.Ok(t, err)

	metric := labels.FromStrings(model.MetricNameLabel, "a_plus_one").String()
	metricSample, ok := samples[metric]

	testutil.Assert(t, ok, "Series %s not returned.", metric)
	testutil.Assert(t, value.IsStaleNaN(metricSample[2].V), "Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(metricSample[2].V))
	metricSample[2].V = 42 // reflect.DeepEqual cannot handle NaN.

	want := map[string][]promql.Point{
		metric: {{0, 2}, {1000, 3}, {2000, 42}},
	}

	testutil.Equals(t, want, samples)
}

// Convert a SeriesSet into a form usable with reflect.DeepEqual.
func readSeriesSet(ss storage.SeriesSet) (map[string][]promql.Point, error) {
	result := map[string][]promql.Point{}

	for ss.Next() {
		series := ss.At()

		points := []promql.Point{}
		it := series.Iterator()
		for it.Next() {
			t, v := it.At()
			points = append(points, promql.Point{T: t, V: v})
		}

		name := series.Labels().String()
		result[name] = points
	}
	return result, ss.Err()
}

func TestCopyState(t *testing.T) {
	oldGroup := &Group{
		rules: []Rule{
			NewAlertingRule("alert", nil, 0, nil, nil, true, nil),
			NewRecordingRule("rule1", nil, nil),
			NewRecordingRule("rule2", nil, nil),
			NewRecordingRule("rule3", nil, labels.Labels{{Name: "l1", Value: "v1"}}),
			NewRecordingRule("rule3", nil, labels.Labels{{Name: "l1", Value: "v2"}}),
			NewAlertingRule("alert2", nil, 0, labels.Labels{{Name: "l2", Value: "v1"}}, nil, true, nil),
		},
		seriesInPreviousEval: []map[string]labels.Labels{
			{"a": nil},
			{"r1": nil},
			{"r2": nil},
			{"r3a": labels.Labels{{Name: "l1", Value: "v1"}}},
			{"r3b": labels.Labels{{Name: "l1", Value: "v2"}}},
			{"a2": labels.Labels{{Name: "l2", Value: "v1"}}},
		},
		evaluationDuration: time.Second,
	}
	oldGroup.rules[0].(*AlertingRule).active[42] = nil
	newGroup := &Group{
		rules: []Rule{
			NewRecordingRule("rule3", nil, labels.Labels{{Name: "l1", Value: "v0"}}),
			NewRecordingRule("rule3", nil, labels.Labels{{Name: "l1", Value: "v1"}}),
			NewRecordingRule("rule3", nil, labels.Labels{{Name: "l1", Value: "v2"}}),
			NewAlertingRule("alert", nil, 0, nil, nil, true, nil),
			NewRecordingRule("rule1", nil, nil),
			NewAlertingRule("alert2", nil, 0, labels.Labels{{Name: "l2", Value: "v0"}}, nil, true, nil),
			NewAlertingRule("alert2", nil, 0, labels.Labels{{Name: "l2", Value: "v1"}}, nil, true, nil),
			NewRecordingRule("rule4", nil, nil),
		},
		seriesInPreviousEval: make([]map[string]labels.Labels, 8),
	}
	newGroup.CopyState(oldGroup)

	want := []map[string]labels.Labels{
		nil,
		{"r3a": labels.Labels{{Name: "l1", Value: "v1"}}},
		{"r3b": labels.Labels{{Name: "l1", Value: "v2"}}},
		{"a": nil},
		{"r1": nil},
		nil,
		{"a2": labels.Labels{{Name: "l2", Value: "v1"}}},
		nil,
	}
	testutil.Equals(t, want, newGroup.seriesInPreviousEval)
	testutil.Equals(t, oldGroup.rules[0], newGroup.rules[3])
	testutil.Equals(t, oldGroup.evaluationDuration, newGroup.evaluationDuration)
}

func TestUpdate(t *testing.T) {
	files := []string{"fixtures/rules.yaml"}
	expected := map[string]labels.Labels{
		"test": labels.FromStrings("name", "value"),
	}
	storage := testutil.NewStorage(t)
	defer storage.Close()
	opts := promql.EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
	}
	engine := promql.NewEngine(opts)
	ruleManager := NewManager(&ManagerOptions{
		Appendable: storage,
		TSDB:       storage,
		QueryFunc:  EngineQueryFunc(engine, storage),
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	})
	ruleManager.Run()
	defer ruleManager.Stop()

	err := ruleManager.Update(10*time.Second, files)
	testutil.Ok(t, err)
	testutil.Assert(t, len(ruleManager.groups) > 0, "expected non-empty rule groups")
	for _, g := range ruleManager.groups {
		g.seriesInPreviousEval = []map[string]labels.Labels{
			expected,
		}
	}

	err = ruleManager.Update(10*time.Second, files)
	testutil.Ok(t, err)
	for _, g := range ruleManager.groups {
		for _, actual := range g.seriesInPreviousEval {
			testutil.Equals(t, expected, actual)
		}
	}
}

func TestNotify(t *testing.T) {
	storage := testutil.NewStorage(t)
	defer storage.Close()
	engineOpts := promql.EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxConcurrent: 10,
		MaxSamples:    10,
		Timeout:       10 * time.Second,
	}
	engine := promql.NewEngine(engineOpts)
	var lastNotified []*Alert
	notifyFunc := func(ctx context.Context, expr string, alerts ...*Alert) {
		lastNotified = alerts
	}
	opts := &ManagerOptions{
		QueryFunc:   EngineQueryFunc(engine, storage),
		Appendable:  storage,
		TSDB:        storage,
		Context:     context.Background(),
		Logger:      log.NewNopLogger(),
		NotifyFunc:  notifyFunc,
		ResendDelay: 2 * time.Second,
	}

	expr, err := promql.ParseExpr("a > 1")
	testutil.Ok(t, err)
	rule := NewAlertingRule("aTooHigh", expr, 0, labels.Labels{}, labels.Labels{}, true, log.NewNopLogger())
	group := NewGroup("alert", "", time.Second, []Rule{rule}, true, opts)

	app, _ := storage.Appender()
	app.Add(labels.FromStrings(model.MetricNameLabel, "a"), 1000, 2)
	app.Add(labels.FromStrings(model.MetricNameLabel, "a"), 2000, 3)
	app.Add(labels.FromStrings(model.MetricNameLabel, "a"), 5000, 3)
	app.Add(labels.FromStrings(model.MetricNameLabel, "a"), 6000, 0)

	err = app.Commit()
	testutil.Ok(t, err)

	ctx := context.Background()

	// Alert sent right away
	group.Eval(ctx, time.Unix(1, 0))
	testutil.Equals(t, 1, len(lastNotified))
	testutil.Assert(t, !lastNotified[0].ValidUntil.IsZero(), "ValidUntil should not be zero")

	// Alert is not sent 1s later
	group.Eval(ctx, time.Unix(2, 0))
	testutil.Equals(t, 0, len(lastNotified))

	// Alert is resent at t=5s
	group.Eval(ctx, time.Unix(5, 0))
	testutil.Equals(t, 1, len(lastNotified))

	// Resolution alert sent right away
	group.Eval(ctx, time.Unix(6, 0))
	testutil.Equals(t, 1, len(lastNotified))
}
