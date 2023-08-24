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
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/teststorage"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestAlertingRule(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 5m
			http_requests{job="app-server", instance="0", group="canary", severity="overwrite-me"}	75 85  95 105 105  95  85
			http_requests{job="app-server", instance="1", group="canary", severity="overwrite-me"}	80 90 100 110 120 130 140
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	require.NoError(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		time.Minute,
		0,
		labels.FromStrings("severity", "{{\"c\"}}ritical"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)
	result := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "pending",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "pending",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "firing",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "HTTPRequestRateLow",
				"alertstate", "firing",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
	}

	baseTime := time.Unix(0, 0)

	tests := []struct {
		time   time.Duration
		result promql.Vector
	}{
		{
			time:   0,
			result: result[:2],
		},
		{
			time:   5 * time.Minute,
			result: result[2:],
		},
		{
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

		res, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0)
		require.NoError(t, err)

		var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
		for _, smpl := range res {
			smplName := smpl.Metric.Get("__name__")
			if smplName == "ALERTS" {
				filteredRes = append(filteredRes, smpl)
			} else {
				// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
				require.Equal(t, smplName, "ALERTS_FOR_STATE")
			}
		}
		for i := range test.result {
			test.result[i].T = timestamp.FromTime(evalTime)
		}
		require.Equal(t, len(test.result), len(filteredRes), "%d. Number of samples in expected and actual output don't match (%d vs. %d)", i, len(test.result), len(res))

		sort.Slice(filteredRes, func(i, j int) bool {
			return labels.Compare(filteredRes[i].Metric, filteredRes[j].Metric) < 0
		})
		require.Equal(t, test.result, filteredRes)

		for _, aa := range rule.ActiveAlerts() {
			require.Zero(t, aa.Labels.Get(model.MetricNameLabel), "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
		}
	}
}

func TestForStateAddSamples(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 5m
			http_requests{job="app-server", instance="0", group="canary", severity="overwrite-me"}	75 85  95 105 105  95  85
			http_requests{job="app-server", instance="1", group="canary", severity="overwrite-me"}	80 90 100 110 120 130 140
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	require.NoError(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		time.Minute,
		0,
		labels.FromStrings("severity", "{{\"c\"}}ritical"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)
	result := promql.Vector{
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "0",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
		promql.Sample{
			Metric: labels.FromStrings(
				"__name__", "ALERTS_FOR_STATE",
				"alertname", "HTTPRequestRateLow",
				"group", "canary",
				"instance", "1",
				"job", "app-server",
				"severity", "critical",
			),
			F: 1,
		},
	}

	baseTime := time.Unix(0, 0)

	tests := []struct {
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

		res, err := rule.Eval(context.TODO(), evalTime, EngineQueryFunc(testEngine, storage), nil, 0)
		require.NoError(t, err)

		var filteredRes promql.Vector // After removing 'ALERTS' samples.
		for _, smpl := range res {
			smplName := smpl.Metric.Get("__name__")
			if smplName == "ALERTS_FOR_STATE" {
				filteredRes = append(filteredRes, smpl)
			} else {
				// If not 'ALERTS_FOR_STATE', it has to be 'ALERTS'.
				require.Equal(t, smplName, "ALERTS")
			}
		}
		for i := range test.result {
			test.result[i].T = timestamp.FromTime(evalTime)
			// Updating the expected 'for' state.
			if test.result[i].F >= 0 {
				test.result[i].F = forState
			}
		}
		require.Equal(t, len(test.result), len(filteredRes), "%d. Number of samples in expected and actual output don't match (%d vs. %d)", i, len(test.result), len(res))

		sort.Slice(filteredRes, func(i, j int) bool {
			return labels.Compare(filteredRes[i].Metric, filteredRes[j].Metric) < 0
		})
		require.Equal(t, test.result, filteredRes)

		for _, aa := range rule.ActiveAlerts() {
			require.Zero(t, aa.Labels.Get(model.MetricNameLabel), "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
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
	storage := promql.LoadedStorage(t, `
		load 5m
		http_requests{job="app-server", instance="0", group="canary", severity="overwrite-me"}	75  85 50 0 0 25 0 0 40 0 120
		http_requests{job="app-server", instance="1", group="canary", severity="overwrite-me"}	125 90 60 0 0 25 0 0 40 0 130
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	require.NoError(t, err)

	opts := &ManagerOptions{
		QueryFunc:       EngineQueryFunc(testEngine, storage),
		Appendable:      storage,
		Queryable:       storage,
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
		0,
		labels.FromStrings("severity", "critical"),
		labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil,
	)

	group := NewGroup(GroupOptions{
		Name:          "default",
		Interval:      time.Second,
		Rules:         []Rule{rule},
		ShouldRestore: true,
		Opts:          opts,
	})
	groups := make(map[string]*Group)
	groups["default;"] = group

	initialRuns := []time.Duration{0, 5 * time.Minute}

	baseTime := time.Unix(0, 0)
	for _, duration := range initialRuns {
		evalTime := baseTime.Add(duration)
		group.Eval(context.TODO(), evalTime)
	}

	exp := rule.ActiveAlerts()
	for _, aa := range exp {
		require.Zero(t, aa.Labels.Get(model.MetricNameLabel), "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
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
			0,
			labels.FromStrings("severity", "critical"),
			labels.EmptyLabels(), labels.EmptyLabels(), "", false, nil,
		)
		newGroup := NewGroup(GroupOptions{
			Name:          "default",
			Interval:      time.Second,
			Rules:         []Rule{newRule},
			ShouldRestore: true,
			Opts:          opts,
		})

		newGroups := make(map[string]*Group)
		newGroups["default;"] = newGroup

		restoreTime := baseTime.Add(tst.restoreDuration)
		// First eval before restoration.
		newGroup.Eval(context.TODO(), restoreTime)
		// Restore happens here.
		newGroup.RestoreForState(restoreTime)

		got := newRule.ActiveAlerts()
		for _, aa := range got {
			require.Zero(t, aa.Labels.Get(model.MetricNameLabel), "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
		}
		sort.Slice(got, func(i, j int) bool {
			return labels.Compare(got[i].Labels, got[j].Labels) < 0
		})

		// Checking if we have restored it correctly.
		switch {
		case tst.noRestore:
			require.Equal(t, tst.num, len(got))
			for _, e := range got {
				require.Equal(t, e.ActiveAt, restoreTime)
			}
		case tst.gracePeriod:
			require.Equal(t, tst.num, len(got))
			for _, e := range got {
				require.Equal(t, opts.ForGracePeriod, e.ActiveAt.Add(alertForDuration).Sub(restoreTime))
			}
		default:
			exp := tst.alerts
			require.Equal(t, len(exp), len(got))
			sortAlerts(exp)
			sortAlerts(got)
			for i, e := range exp {
				require.Equal(t, e.Labels, got[i].Labels)

				// Difference in time should be within 1e6 ns, i.e. 1ms
				// (due to conversion between ns & ms, float64 & int64).
				activeAtDiff := float64(e.ActiveAt.Unix() + int64(tst.downDuration/time.Second) - got[i].ActiveAt.Unix())
				require.Equal(t, 0.0, math.Abs(activeAtDiff), "'for' state restored time is wrong")
			}
		}
	}

	for _, tst := range tests {
		testFunc(tst)
	}

	// Testing the grace period.
	for _, duration := range []time.Duration{10 * time.Minute, 15 * time.Minute, 20 * time.Minute} {
		evalTime := baseTime.Add(duration)
		group.Eval(context.TODO(), evalTime)
	}
	testFunc(testInput{
		restoreDuration: 25 * time.Minute,
		alerts:          []*Alert{},
		gracePeriod:     true,
		num:             2,
	})
}

func TestStaleness(t *testing.T) {
	st := teststorage.New(t)
	defer st.Close()
	engineOpts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(engineOpts)
	opts := &ManagerOptions{
		QueryFunc:  EngineQueryFunc(engine, st),
		Appendable: st,
		Queryable:  st,
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	}

	expr, err := parser.ParseExpr("a + 1")
	require.NoError(t, err)
	rule := NewRecordingRule("a_plus_one", expr, labels.Labels{})
	group := NewGroup(GroupOptions{
		Name:          "default",
		Interval:      time.Second,
		Rules:         []Rule{rule},
		ShouldRestore: true,
		Opts:          opts,
	})

	// A time series that has two samples and then goes stale.
	app := st.Appender(context.Background())
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 0, 1)
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 1000, 2)
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 2000, math.Float64frombits(value.StaleNaN))

	err = app.Commit()
	require.NoError(t, err)

	ctx := context.Background()

	// Execute 3 times, 1 second apart.
	group.Eval(ctx, time.Unix(0, 0))
	group.Eval(ctx, time.Unix(1, 0))
	group.Eval(ctx, time.Unix(2, 0))

	querier, err := st.Querier(context.Background(), 0, 2000)
	require.NoError(t, err)
	defer querier.Close()

	matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a_plus_one")
	require.NoError(t, err)

	set := querier.Select(false, nil, matcher)
	samples, err := readSeriesSet(set)
	require.NoError(t, err)

	metric := labels.FromStrings(model.MetricNameLabel, "a_plus_one").String()
	metricSample, ok := samples[metric]

	require.True(t, ok, "Series %s not returned.", metric)
	require.True(t, value.IsStaleNaN(metricSample[2].F), "Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(metricSample[2].F))
	metricSample[2].F = 42 // require.Equal cannot handle NaN.

	want := map[string][]promql.FPoint{
		metric: {{T: 0, F: 2}, {T: 1000, F: 3}, {T: 2000, F: 42}},
	}

	require.Equal(t, want, samples)
}

// Convert a SeriesSet into a form usable with require.Equal.
func readSeriesSet(ss storage.SeriesSet) (map[string][]promql.FPoint, error) {
	result := map[string][]promql.FPoint{}
	var it chunkenc.Iterator

	for ss.Next() {
		series := ss.At()

		points := []promql.FPoint{}
		it := series.Iterator(it)
		for it.Next() == chunkenc.ValFloat {
			t, v := it.At()
			points = append(points, promql.FPoint{T: t, F: v})
		}

		name := series.Labels().String()
		result[name] = points
	}
	return result, ss.Err()
}

func TestCopyState(t *testing.T) {
	oldGroup := &Group{
		rules: []Rule{
			NewAlertingRule("alert", nil, 0, 0, labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil),
			NewRecordingRule("rule1", nil, labels.EmptyLabels()),
			NewRecordingRule("rule2", nil, labels.EmptyLabels()),
			NewRecordingRule("rule3", nil, labels.FromStrings("l1", "v1")),
			NewRecordingRule("rule3", nil, labels.FromStrings("l1", "v2")),
			NewRecordingRule("rule3", nil, labels.FromStrings("l1", "v3")),
			NewAlertingRule("alert2", nil, 0, 0, labels.FromStrings("l2", "v1"), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil),
		},
		seriesInPreviousEval: []map[string]labels.Labels{
			{},
			{},
			{},
			{"r3a": labels.FromStrings("l1", "v1")},
			{"r3b": labels.FromStrings("l1", "v2")},
			{"r3c": labels.FromStrings("l1", "v3")},
			{"a2": labels.FromStrings("l2", "v1")},
		},
		evaluationTime: time.Second,
	}
	oldGroup.rules[0].(*AlertingRule).active[42] = nil
	newGroup := &Group{
		rules: []Rule{
			NewRecordingRule("rule3", nil, labels.FromStrings("l1", "v0")),
			NewRecordingRule("rule3", nil, labels.FromStrings("l1", "v1")),
			NewRecordingRule("rule3", nil, labels.FromStrings("l1", "v2")),
			NewAlertingRule("alert", nil, 0, 0, labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil),
			NewRecordingRule("rule1", nil, labels.EmptyLabels()),
			NewAlertingRule("alert2", nil, 0, 0, labels.FromStrings("l2", "v0"), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil),
			NewAlertingRule("alert2", nil, 0, 0, labels.FromStrings("l2", "v1"), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil),
			NewRecordingRule("rule4", nil, labels.EmptyLabels()),
		},
		seriesInPreviousEval: make([]map[string]labels.Labels, 8),
	}
	newGroup.CopyState(oldGroup)

	want := []map[string]labels.Labels{
		nil,
		{"r3a": labels.FromStrings("l1", "v1")},
		{"r3b": labels.FromStrings("l1", "v2")},
		{},
		{},
		nil,
		{"a2": labels.FromStrings("l2", "v1")},
		nil,
	}
	require.Equal(t, want, newGroup.seriesInPreviousEval)
	require.Equal(t, oldGroup.rules[0], newGroup.rules[3])
	require.Equal(t, oldGroup.evaluationTime, newGroup.evaluationTime)
	require.Equal(t, oldGroup.lastEvaluation, newGroup.lastEvaluation)
	require.Equal(t, []labels.Labels{labels.FromStrings("l1", "v3")}, newGroup.staleSeries)
}

func TestDeletedRuleMarkedStale(t *testing.T) {
	st := teststorage.New(t)
	defer st.Close()
	oldGroup := &Group{
		rules: []Rule{
			NewRecordingRule("rule1", nil, labels.FromStrings("l1", "v1")),
		},
		seriesInPreviousEval: []map[string]labels.Labels{
			{"r1": labels.FromStrings("l1", "v1")},
		},
	}
	newGroup := &Group{
		rules:                []Rule{},
		seriesInPreviousEval: []map[string]labels.Labels{},
		opts: &ManagerOptions{
			Appendable: st,
		},
	}
	newGroup.CopyState(oldGroup)

	newGroup.Eval(context.Background(), time.Unix(0, 0))

	querier, err := st.Querier(context.Background(), 0, 2000)
	require.NoError(t, err)
	defer querier.Close()

	matcher, err := labels.NewMatcher(labels.MatchEqual, "l1", "v1")
	require.NoError(t, err)

	set := querier.Select(false, nil, matcher)
	samples, err := readSeriesSet(set)
	require.NoError(t, err)

	metric := labels.FromStrings("l1", "v1").String()
	metricSample, ok := samples[metric]

	require.True(t, ok, "Series %s not returned.", metric)
	require.True(t, value.IsStaleNaN(metricSample[0].F), "Appended sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(metricSample[0].F))
}

func TestUpdate(t *testing.T) {
	files := []string{"fixtures/rules.yaml"}
	expected := map[string]labels.Labels{
		"test": labels.FromStrings("name", "value"),
	}
	st := teststorage.New(t)
	defer st.Close()
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(opts)
	ruleManager := NewManager(&ManagerOptions{
		Appendable: st,
		Queryable:  st,
		QueryFunc:  EngineQueryFunc(engine, st),
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	})
	ruleManager.start()
	defer ruleManager.Stop()

	err := ruleManager.Update(10*time.Second, files, labels.EmptyLabels(), "", nil)
	require.NoError(t, err)
	require.Greater(t, len(ruleManager.groups), 0, "expected non-empty rule groups")
	ogs := map[string]*Group{}
	for h, g := range ruleManager.groups {
		g.seriesInPreviousEval = []map[string]labels.Labels{
			expected,
		}
		ogs[h] = g
	}

	err = ruleManager.Update(10*time.Second, files, labels.EmptyLabels(), "", nil)
	require.NoError(t, err)
	for h, g := range ruleManager.groups {
		for _, actual := range g.seriesInPreviousEval {
			require.Equal(t, expected, actual)
		}
		// Groups are the same because of no updates.
		require.Equal(t, ogs[h], g)
	}

	// Groups will be recreated if updated.
	rgs, errs := rulefmt.ParseFile("fixtures/rules.yaml")
	require.Equal(t, 0, len(errs), "file parsing failures")

	tmpFile, err := os.CreateTemp("", "rules.test.*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	err = ruleManager.Update(10*time.Second, []string{tmpFile.Name()}, labels.EmptyLabels(), "", nil)
	require.NoError(t, err)

	for h, g := range ruleManager.groups {
		ogs[h] = g
	}

	// Update interval and reload.
	for i, g := range rgs.Groups {
		if g.Interval != 0 {
			rgs.Groups[i].Interval = g.Interval * 2
		} else {
			rgs.Groups[i].Interval = model.Duration(10)
		}
	}
	reloadAndValidate(rgs, t, tmpFile, ruleManager, ogs)

	// Update limit and reload.
	for i := range rgs.Groups {
		rgs.Groups[i].Limit = 1
	}
	reloadAndValidate(rgs, t, tmpFile, ruleManager, ogs)

	// Change group rules and reload.
	for i, g := range rgs.Groups {
		for j, r := range g.Rules {
			rgs.Groups[i].Rules[j].Expr.SetString(fmt.Sprintf("%s * 0", r.Expr.Value))
		}
	}
	reloadAndValidate(rgs, t, tmpFile, ruleManager, ogs)
}

// ruleGroupsTest for running tests over rules.
type ruleGroupsTest struct {
	Groups []ruleGroupTest `yaml:"groups"`
}

// ruleGroupTest forms a testing struct for running tests over rules.
type ruleGroupTest struct {
	Name     string         `yaml:"name"`
	Interval model.Duration `yaml:"interval,omitempty"`
	Limit    int            `yaml:"limit,omitempty"`
	Rules    []rulefmt.Rule `yaml:"rules"`
}

func formatRules(r *rulefmt.RuleGroups) ruleGroupsTest {
	grps := r.Groups
	tmp := []ruleGroupTest{}
	for _, g := range grps {
		rtmp := []rulefmt.Rule{}
		for _, r := range g.Rules {
			rtmp = append(rtmp, rulefmt.Rule{
				Record:      r.Record.Value,
				Alert:       r.Alert.Value,
				Expr:        r.Expr.Value,
				For:         r.For,
				Labels:      r.Labels,
				Annotations: r.Annotations,
			})
		}
		tmp = append(tmp, ruleGroupTest{
			Name:     g.Name,
			Interval: g.Interval,
			Limit:    g.Limit,
			Rules:    rtmp,
		})
	}
	return ruleGroupsTest{
		Groups: tmp,
	}
}

func reloadAndValidate(rgs *rulefmt.RuleGroups, t *testing.T, tmpFile *os.File, ruleManager *Manager, ogs map[string]*Group) {
	bs, err := yaml.Marshal(formatRules(rgs))
	require.NoError(t, err)
	tmpFile.Seek(0, 0)
	_, err = tmpFile.Write(bs)
	require.NoError(t, err)
	err = ruleManager.Update(10*time.Second, []string{tmpFile.Name()}, labels.EmptyLabels(), "", nil)
	require.NoError(t, err)
	for h, g := range ruleManager.groups {
		if ogs[h] == g {
			t.Fail()
		}
		ogs[h] = g
	}
}

func TestNotify(t *testing.T) {
	storage := teststorage.New(t)
	defer storage.Close()
	engineOpts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(engineOpts)
	var lastNotified []*Alert
	notifyFunc := func(ctx context.Context, expr string, alerts ...*Alert) {
		lastNotified = alerts
	}
	opts := &ManagerOptions{
		QueryFunc:   EngineQueryFunc(engine, storage),
		Appendable:  storage,
		Queryable:   storage,
		Context:     context.Background(),
		Logger:      log.NewNopLogger(),
		NotifyFunc:  notifyFunc,
		ResendDelay: 2 * time.Second,
	}

	expr, err := parser.ParseExpr("a > 1")
	require.NoError(t, err)
	rule := NewAlertingRule("aTooHigh", expr, 0, 0, labels.Labels{}, labels.Labels{}, labels.EmptyLabels(), "", true, log.NewNopLogger())
	group := NewGroup(GroupOptions{
		Name:          "alert",
		Interval:      time.Second,
		Rules:         []Rule{rule},
		ShouldRestore: true,
		Opts:          opts,
	})

	app := storage.Appender(context.Background())
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 1000, 2)
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 2000, 3)
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 5000, 3)
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 6000, 0)

	err = app.Commit()
	require.NoError(t, err)

	ctx := context.Background()

	// Alert sent right away
	group.Eval(ctx, time.Unix(1, 0))
	require.Equal(t, 1, len(lastNotified))
	require.NotZero(t, lastNotified[0].ValidUntil, "ValidUntil should not be zero")

	// Alert is not sent 1s later
	group.Eval(ctx, time.Unix(2, 0))
	require.Equal(t, 0, len(lastNotified))

	// Alert is resent at t=5s
	group.Eval(ctx, time.Unix(5, 0))
	require.Equal(t, 1, len(lastNotified))

	// Resolution alert sent right away
	group.Eval(ctx, time.Unix(6, 0))
	require.Equal(t, 1, len(lastNotified))
}

func TestMetricsUpdate(t *testing.T) {
	files := []string{"fixtures/rules.yaml", "fixtures/rules2.yaml"}
	metricNames := []string{
		"prometheus_rule_evaluations_total",
		"prometheus_rule_evaluation_failures_total",
		"prometheus_rule_group_interval_seconds",
		"prometheus_rule_group_last_duration_seconds",
		"prometheus_rule_group_last_evaluation_timestamp_seconds",
		"prometheus_rule_group_rules",
	}

	storage := teststorage.New(t)
	defer storage.Close()
	registry := prometheus.NewRegistry()
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(opts)
	ruleManager := NewManager(&ManagerOptions{
		Appendable: storage,
		Queryable:  storage,
		QueryFunc:  EngineQueryFunc(engine, storage),
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
		Registerer: registry,
	})
	ruleManager.start()
	defer ruleManager.Stop()

	countMetrics := func() int {
		ms, err := registry.Gather()
		require.NoError(t, err)
		var metrics int
		for _, m := range ms {
			s := m.GetName()
			for _, n := range metricNames {
				if s == n {
					metrics += len(m.Metric)
					break
				}
			}
		}
		return metrics
	}

	cases := []struct {
		files   []string
		metrics int
	}{
		{
			files:   files,
			metrics: 12,
		},
		{
			files:   files[:1],
			metrics: 6,
		},
		{
			files:   files[:0],
			metrics: 0,
		},
		{
			files:   files[1:],
			metrics: 6,
		},
	}

	for i, c := range cases {
		err := ruleManager.Update(time.Second, c.files, labels.EmptyLabels(), "", nil)
		require.NoError(t, err)
		time.Sleep(2 * time.Second)
		require.Equal(t, c.metrics, countMetrics(), "test %d: invalid count of metrics", i)
	}
}

func TestGroupStalenessOnRemoval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	files := []string{"fixtures/rules2.yaml"}
	sameFiles := []string{"fixtures/rules2_copy.yaml"}

	storage := teststorage.New(t)
	defer storage.Close()
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(opts)
	ruleManager := NewManager(&ManagerOptions{
		Appendable: storage,
		Queryable:  storage,
		QueryFunc:  EngineQueryFunc(engine, storage),
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	})
	var stopped bool
	ruleManager.start()
	defer func() {
		if !stopped {
			ruleManager.Stop()
		}
	}()

	cases := []struct {
		files    []string
		staleNaN int
	}{
		{
			files:    files,
			staleNaN: 0,
		},
		{
			// When we remove the files, it should produce a staleness marker.
			files:    files[:0],
			staleNaN: 1,
		},
		{
			// Rules that produce the same metrics but in a different file
			// should not produce staleness marker.
			files:    sameFiles,
			staleNaN: 0,
		},
		{
			// Staleness marker should be present as we don't have any rules
			// loaded anymore.
			files:    files[:0],
			staleNaN: 1,
		},
		{
			// Add rules back so we have rules loaded when we stop the manager
			// and check for the absence of staleness markers.
			files:    sameFiles,
			staleNaN: 0,
		},
	}

	var totalStaleNaN int
	for i, c := range cases {
		err := ruleManager.Update(time.Second, c.files, labels.EmptyLabels(), "", nil)
		require.NoError(t, err)
		time.Sleep(3 * time.Second)
		totalStaleNaN += c.staleNaN
		require.Equal(t, totalStaleNaN, countStaleNaN(t, storage), "test %d/%q: invalid count of staleness markers", i, c.files)
	}
	ruleManager.Stop()
	stopped = true
	require.Equal(t, totalStaleNaN, countStaleNaN(t, storage), "invalid count of staleness markers after stopping the engine")
}

func TestMetricsStalenessOnManagerShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	files := []string{"fixtures/rules2.yaml"}

	storage := teststorage.New(t)
	defer storage.Close()
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(opts)
	ruleManager := NewManager(&ManagerOptions{
		Appendable: storage,
		Queryable:  storage,
		QueryFunc:  EngineQueryFunc(engine, storage),
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	})
	var stopped bool
	ruleManager.start()
	defer func() {
		if !stopped {
			ruleManager.Stop()
		}
	}()

	err := ruleManager.Update(2*time.Second, files, labels.EmptyLabels(), "", nil)
	time.Sleep(4 * time.Second)
	require.NoError(t, err)
	start := time.Now()
	err = ruleManager.Update(3*time.Second, files[:0], labels.EmptyLabels(), "", nil)
	require.NoError(t, err)
	ruleManager.Stop()
	stopped = true
	require.True(t, time.Since(start) < 1*time.Second, "rule manager does not stop early")
	time.Sleep(5 * time.Second)
	require.Equal(t, 0, countStaleNaN(t, storage), "invalid count of staleness markers after stopping the engine")
}

func countStaleNaN(t *testing.T, st storage.Storage) int {
	var c int
	querier, err := st.Querier(context.Background(), 0, time.Now().Unix()*1000)
	require.NoError(t, err)
	defer querier.Close()

	matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_2")
	require.NoError(t, err)

	set := querier.Select(false, nil, matcher)
	samples, err := readSeriesSet(set)
	require.NoError(t, err)

	metric := labels.FromStrings(model.MetricNameLabel, "test_2").String()
	metricSample, ok := samples[metric]

	require.True(t, ok, "Series %s not returned.", metric)
	for _, s := range metricSample {
		if value.IsStaleNaN(s.F) {
			c++
		}
	}
	return c
}

func TestGroupHasAlertingRules(t *testing.T) {
	tests := []struct {
		group *Group
		want  bool
	}{
		{
			group: &Group{
				name: "HasAlertingRule",
				rules: []Rule{
					NewAlertingRule("alert", nil, 0, 0, labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "", true, nil),
					NewRecordingRule("record", nil, labels.EmptyLabels()),
				},
			},
			want: true,
		},
		{
			group: &Group{
				name:  "HasNoRule",
				rules: []Rule{},
			},
			want: false,
		},
		{
			group: &Group{
				name: "HasOnlyRecordingRule",
				rules: []Rule{
					NewRecordingRule("record", nil, labels.EmptyLabels()),
				},
			},
			want: false,
		},
	}

	for i, test := range tests {
		got := test.group.HasAlertingRules()
		require.Equal(t, test.want, got, "test case %d failed, expected:%t got:%t", i, test.want, got)
	}
}

func TestRuleHealthUpdates(t *testing.T) {
	st := teststorage.New(t)
	defer st.Close()
	engineOpts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := promql.NewEngine(engineOpts)
	opts := &ManagerOptions{
		QueryFunc:  EngineQueryFunc(engine, st),
		Appendable: st,
		Queryable:  st,
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	}

	expr, err := parser.ParseExpr("a + 1")
	require.NoError(t, err)
	rule := NewRecordingRule("a_plus_one", expr, labels.Labels{})
	group := NewGroup(GroupOptions{
		Name:          "default",
		Interval:      time.Second,
		Rules:         []Rule{rule},
		ShouldRestore: true,
		Opts:          opts,
	})

	// A time series that has two samples.
	app := st.Appender(context.Background())
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 0, 1)
	app.Append(0, labels.FromStrings(model.MetricNameLabel, "a"), 1000, 2)
	err = app.Commit()
	require.NoError(t, err)

	ctx := context.Background()

	rules := group.Rules()[0]
	require.NoError(t, rules.LastError())
	require.Equal(t, HealthUnknown, rules.Health())

	// Execute 2 times, it should be all green.
	group.Eval(ctx, time.Unix(0, 0))
	group.Eval(ctx, time.Unix(1, 0))

	rules = group.Rules()[0]
	require.NoError(t, rules.LastError())
	require.Equal(t, HealthGood, rules.Health())

	// Now execute the rule in the past again, this should cause append failures.
	group.Eval(ctx, time.Unix(0, 0))
	rules = group.Rules()[0]
	require.EqualError(t, rules.LastError(), storage.ErrOutOfOrderSample.Error())
	require.Equal(t, HealthBad, rules.Health())
}

func TestRuleGroupEvalIterationFunc(t *testing.T) {
	storage := promql.LoadedStorage(t, `
		load 5m
		http_requests{instance="0"}	75  85 50 0 0 25 0 0 40 0 120
	`)
	t.Cleanup(func() { storage.Close() })

	expr, err := parser.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	require.NoError(t, err)

	testValue := 1

	evalIterationFunc := func(ctx context.Context, g *Group, evalTimestamp time.Time) {
		testValue = 2
		DefaultEvalIterationFunc(ctx, g, evalTimestamp)
		testValue = 3
	}

	skipEvalIterationFunc := func(ctx context.Context, g *Group, evalTimestamp time.Time) {
		testValue = 4
	}

	type testInput struct {
		evalIterationFunc       GroupEvalIterationFunc
		expectedValue           int
		lastEvalTimestampIsZero bool
	}

	tests := []testInput{
		// testValue should still have value of 1 since the default iteration function will be called.
		{
			evalIterationFunc:       nil,
			expectedValue:           1,
			lastEvalTimestampIsZero: false,
		},
		// testValue should be incremented to 3 since evalIterationFunc is called.
		{
			evalIterationFunc:       evalIterationFunc,
			expectedValue:           3,
			lastEvalTimestampIsZero: false,
		},
		// testValue should be incremented to 4 since skipEvalIterationFunc is called.
		{
			evalIterationFunc:       skipEvalIterationFunc,
			expectedValue:           4,
			lastEvalTimestampIsZero: true,
		},
	}

	testFunc := func(tst testInput) {
		opts := &ManagerOptions{
			QueryFunc:       EngineQueryFunc(testEngine, storage),
			Appendable:      storage,
			Queryable:       storage,
			Context:         context.Background(),
			Logger:          log.NewNopLogger(),
			NotifyFunc:      func(ctx context.Context, expr string, alerts ...*Alert) {},
			OutageTolerance: 30 * time.Minute,
			ForGracePeriod:  10 * time.Minute,
		}

		activeAlert := &Alert{
			State:    StateFiring,
			ActiveAt: time.Now(),
		}

		m := map[uint64]*Alert{}
		m[1] = activeAlert

		rule := &AlertingRule{
			name:                "HTTPRequestRateLow",
			vector:              expr,
			holdDuration:        5 * time.Minute,
			labels:              labels.FromStrings("severity", "critical"),
			annotations:         labels.EmptyLabels(),
			externalLabels:      nil,
			externalURL:         "",
			active:              m,
			logger:              nil,
			restored:            atomic.NewBool(true),
			health:              atomic.NewString(string(HealthUnknown)),
			evaluationTimestamp: atomic.NewTime(time.Time{}),
			evaluationDuration:  atomic.NewDuration(0),
			lastError:           atomic.NewError(nil),
		}

		group := NewGroup(GroupOptions{
			Name:              "default",
			Interval:          time.Second,
			Rules:             []Rule{rule},
			ShouldRestore:     true,
			Opts:              opts,
			EvalIterationFunc: tst.evalIterationFunc,
		})

		go func() {
			group.run(opts.Context)
		}()

		time.Sleep(3 * time.Second)
		group.stop()

		require.Equal(t, tst.expectedValue, testValue)
		if tst.lastEvalTimestampIsZero {
			require.Zero(t, group.GetLastEvalTimestamp())
		} else {
			oneMinute, _ := time.ParseDuration("1m")
			require.WithinDuration(t, time.Now(), group.GetLastEvalTimestamp(), oneMinute)
		}
	}

	for i, tst := range tests {
		t.Logf("case %d", i)
		testFunc(tst)
	}
}

func TestNativeHistogramsInRecordingRules(t *testing.T) {
	storage := teststorage.New(t)
	t.Cleanup(func() { storage.Close() })

	// Add some histograms.
	db := storage.DB
	hists := tsdbutil.GenerateTestHistograms(5)
	ts := time.Now()
	app := db.Appender(context.Background())
	for i, h := range hists {
		l := labels.FromStrings("__name__", "histogram_metric", "idx", fmt.Sprintf("%d", i))
		_, err := app.AppendHistogram(0, l, ts.UnixMilli(), h.Copy(), nil)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	opts := &ManagerOptions{
		QueryFunc:  EngineQueryFunc(testEngine, storage),
		Appendable: storage,
		Queryable:  storage,
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	}

	expr, err := parser.ParseExpr("sum(histogram_metric)")
	require.NoError(t, err)
	rule := NewRecordingRule("sum:histogram_metric", expr, labels.Labels{})

	group := NewGroup(GroupOptions{
		Name:          "default",
		Interval:      time.Hour,
		Rules:         []Rule{rule},
		ShouldRestore: true,
		Opts:          opts,
	})

	group.Eval(context.Background(), ts.Add(10*time.Second))

	q, err := db.Querier(context.Background(), ts.UnixMilli(), ts.Add(20*time.Second).UnixMilli())
	require.NoError(t, err)
	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "sum:histogram_metric"))
	require.True(t, ss.Next())
	s := ss.At()
	require.False(t, ss.Next())

	require.Equal(t, labels.FromStrings("__name__", "sum:histogram_metric"), s.Labels())

	expHist := hists[0].ToFloat()
	for _, h := range hists[1:] {
		expHist = expHist.Add(h.ToFloat())
	}

	it := s.Iterator(nil)
	require.Equal(t, chunkenc.ValFloatHistogram, it.Next())
	tsp, fh := it.AtFloatHistogram()
	require.Equal(t, ts.Add(10*time.Second).UnixMilli(), tsp)
	require.Equal(t, expHist, fh)
	require.Equal(t, chunkenc.ValNone, it.Next())
}
