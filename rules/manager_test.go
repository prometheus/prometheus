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
		nil, nil,
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

		res, err := rule.Eval(suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine()), nil)
		testutil.Ok(t, err)

		for i := range test.result {
			test.result[i].T = timestamp.FromTime(evalTime)
		}
		testutil.Assert(t, len(test.result) == len(res), "%d. Number of samples in expected and actual output don't match (%d vs. %d)", i, len(test.result), len(res))

		sort.Slice(res, func(i, j int) bool {
			return labels.Compare(res[i].Metric, res[j].Metric) < 0
		})
		testutil.Equals(t, test.result, res)

		for _, aa := range rule.ActiveAlerts() {
			testutil.Assert(t, aa.Labels.Get(model.MetricNameLabel) == "", "%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
		}
	}
}

func annotateWithTime(lines []string, ts time.Time) []string {
	annotatedLines := []string{}
	for _, line := range lines {
		annotatedLines = append(annotatedLines, fmt.Sprintf(line, timestamp.FromTime(ts)))
	}
	return annotatedLines
}

func TestStaleness(t *testing.T) {
	storage := testutil.NewStorage(t)
	defer storage.Close()
	engine := promql.NewEngine(storage, nil)
	opts := &ManagerOptions{
		QueryFunc:  EngineQueryFunc(engine),
		Appendable: storage,
		Context:    context.Background(),
		Logger:     log.NewNopLogger(),
	}

	expr, err := promql.ParseExpr("a + 1")
	testutil.Ok(t, err)
	rule := NewRecordingRule("a_plus_one", expr, labels.Labels{})
	group := NewGroup("default", "", time.Second, []Rule{rule}, opts)

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

	set, err := querier.Select(matcher)
	testutil.Ok(t, err)

	samples, err := readSeriesSet(set)
	testutil.Ok(t, err)

	metric := labels.FromStrings(model.MetricNameLabel, "a_plus_one").String()
	metricSample, ok := samples[metric]

	testutil.Assert(t, ok, "Series %s not returned.", metric)
	testutil.Assert(t, value.IsStaleNaN(metricSample[2].V), "Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(metricSample[2].V))
	metricSample[2].V = 42 // reflect.DeepEqual cannot handle NaN.

	want := map[string][]promql.Point{
		metric: []promql.Point{{0, 2}, {1000, 3}, {2000, 42}},
	}

	testutil.Equals(t, want, samples)
}

// Convert a SeriesSet into a form useable with reflect.DeepEqual.
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
			NewAlertingRule("alert", nil, 0, nil, nil, nil),
			NewRecordingRule("rule1", nil, nil),
			NewRecordingRule("rule2", nil, nil),
			NewRecordingRule("rule3", nil, nil),
			NewRecordingRule("rule3", nil, nil),
		},
		seriesInPreviousEval: []map[string]labels.Labels{
			map[string]labels.Labels{"a": nil},
			map[string]labels.Labels{"r1": nil},
			map[string]labels.Labels{"r2": nil},
			map[string]labels.Labels{"r3a": nil},
			map[string]labels.Labels{"r3b": nil},
		},
		evaluationTime: time.Second,
	}
	oldGroup.rules[0].(*AlertingRule).active[42] = nil
	newGroup := &Group{
		rules: []Rule{
			NewRecordingRule("rule3", nil, nil),
			NewRecordingRule("rule3", nil, nil),
			NewRecordingRule("rule3", nil, nil),
			NewAlertingRule("alert", nil, 0, nil, nil, nil),
			NewRecordingRule("rule1", nil, nil),
			NewRecordingRule("rule4", nil, nil),
		},
		seriesInPreviousEval: make([]map[string]labels.Labels, 6),
	}
	newGroup.copyState(oldGroup)

	want := []map[string]labels.Labels{
		map[string]labels.Labels{"r3a": nil},
		map[string]labels.Labels{"r3b": nil},
		nil,
		map[string]labels.Labels{"a": nil},
		map[string]labels.Labels{"r1": nil},
		nil,
	}
	testutil.Equals(t, want, newGroup.seriesInPreviousEval)
	testutil.Equals(t, oldGroup.rules[0], newGroup.rules[3])
	testutil.Equals(t, oldGroup.evaluationTime, newGroup.evaluationTime)
}

func TestUpdate(t *testing.T) {
	expected := map[string]labels.Labels{
		"test": labels.FromStrings("name", "value"),
	}
	ruleManager := NewManager(&ManagerOptions{
		Context: context.Background(),
		Logger:  log.NewNopLogger(),
	})
	ruleManager.Run()

	err := ruleManager.Update(0, nil)
	testutil.Ok(t, err)
	for _, g := range ruleManager.groups {
		g.seriesInPreviousEval = []map[string]labels.Labels{
			expected,
		}
	}

	err = ruleManager.Update(0, nil)
	testutil.Ok(t, err)
	for _, g := range ruleManager.groups {
		for _, actual := range g.seriesInPreviousEval {
			testutil.Equals(t, expected, actual)
		}
	}
}
