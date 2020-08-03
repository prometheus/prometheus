// Copyright 2016 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestAlertingRuleHTMLSnippet(t *testing.T) {
	expr, err := parser.ParseExpr(`foo{html="<b>BOLD<b>"}`)
	testutil.Ok(t, err)
	rule := NewAlertingRule("testrule", expr, 0, labels.FromStrings("html", "<b>BOLD</b>"), labels.FromStrings("html", "<b>BOLD</b>"), nil, false, nil)

	const want = `alert: <a href="/test/prefix/graph?g0.expr=ALERTS%7Balertname%3D%22testrule%22%7D&g0.tab=1">testrule</a>
expr: <a href="/test/prefix/graph?g0.expr=foo%7Bhtml%3D%22%3Cb%3EBOLD%3Cb%3E%22%7D&g0.tab=1">foo{html=&#34;&lt;b&gt;BOLD&lt;b&gt;&#34;}</a>
labels:
  html: '&lt;b&gt;BOLD&lt;/b&gt;'
annotations:
  html: '&lt;b&gt;BOLD&lt;/b&gt;'
`

	got := rule.HTMLSnippet("/test/prefix")
	testutil.Assert(t, want == got, "incorrect HTML snippet; want:\n\n|%v|\n\ngot:\n\n|%v|", want, got)
}

func TestAlertingRuleState(t *testing.T) {
	tests := []struct {
		name   string
		active map[uint64]*Alert
		want   AlertState
	}{
		{
			name: "MaxStateFiring",
			active: map[uint64]*Alert{
				0: {State: StatePending},
				1: {State: StateFiring},
			},
			want: StateFiring,
		},
		{
			name: "MaxStatePending",
			active: map[uint64]*Alert{
				0: {State: StateInactive},
				1: {State: StatePending},
			},
			want: StatePending,
		},
		{
			name: "MaxStateInactive",
			active: map[uint64]*Alert{
				0: {State: StateInactive},
				1: {State: StateInactive},
			},
			want: StateInactive,
		},
	}

	for i, test := range tests {
		rule := NewAlertingRule(test.name, nil, 0, nil, nil, nil, true, nil)
		rule.active = test.active
		got := rule.State()
		testutil.Assert(t, test.want == got, "test case %d unexpected AlertState, want:%d got:%d", i, test.want, got)
	}
}

func TestAlertingRuleLabelsUpdate(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	testutil.Ok(t, err)
	defer suite.Close()

	testutil.Ok(t, suite.Run())

	expr, err := parser.ParseExpr(`http_requests < 100`)
	testutil.Ok(t, err)

	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		time.Minute,
		// Basing alerting rule labels off of a value that can change is a very bad idea.
		// If an alert is going back and forth between two label values it will never fire.
		// Instead, you should write two alerts with constant labels.
		labels.FromStrings("severity", "{{ if lt $value 80.0 }}critical{{ else }}warning{{ end }}"),
		nil, nil, true, nil,
	)

	results := []promql.Vector{
		{
			{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "pending",
					"instance", "0",
					"job", "app-server",
					"severity", "critical",
				),
				Point: promql.Point{V: 1},
			},
		},
		{
			{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "pending",
					"instance", "0",
					"job", "app-server",
					"severity", "warning",
				),
				Point: promql.Point{V: 1},
			},
		},
		{
			{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "pending",
					"instance", "0",
					"job", "app-server",
					"severity", "critical",
				),
				Point: promql.Point{V: 1},
			},
		},
		{
			{
				Metric: labels.FromStrings(
					"__name__", "ALERTS",
					"alertname", "HTTPRequestRateLow",
					"alertstate", "firing",
					"instance", "0",
					"job", "app-server",
					"severity", "critical",
				),
				Point: promql.Point{V: 1},
			},
		},
	}

	baseTime := time.Unix(0, 0)
	for i, result := range results {
		t.Logf("case %d", i)
		evalTime := baseTime.Add(time.Duration(i) * time.Minute)
		result[0].Point.T = timestamp.FromTime(evalTime)
		res, err := rule.Eval(suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil)
		testutil.Ok(t, err)

		var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
		for _, smpl := range res {
			smplName := smpl.Metric.Get("__name__")
			if smplName == "ALERTS" {
				filteredRes = append(filteredRes, smpl)
			} else {
				// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
				testutil.Equals(t, "ALERTS_FOR_STATE", smplName)
			}
		}

		testutil.Equals(t, result, filteredRes)
	}
}

func TestAlertingRuleExternalLabelsInTemplate(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	testutil.Ok(t, err)
	defer suite.Close()

	testutil.Ok(t, suite.Run())

	expr, err := parser.ParseExpr(`http_requests < 100`)
	testutil.Ok(t, err)

	ruleWithoutExternalLabels := NewAlertingRule(
		"ExternalLabelDoesNotExist",
		expr,
		time.Minute,
		labels.FromStrings("templated_label", "There are {{ len $externalLabels }} external Labels, of which foo is {{ $externalLabels.foo }}."),
		nil,
		nil,
		true, log.NewNopLogger(),
	)
	ruleWithExternalLabels := NewAlertingRule(
		"ExternalLabelExists",
		expr,
		time.Minute,
		labels.FromStrings("templated_label", "There are {{ len $externalLabels }} external Labels, of which foo is {{ $externalLabels.foo }}."),
		nil,
		labels.FromStrings("foo", "bar", "dings", "bums"),
		true, log.NewNopLogger(),
	)
	result := promql.Vector{
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "ExternalLabelDoesNotExist",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
				"templated_label", "There are 0 external Labels, of which foo is .",
			),
			Point: promql.Point{V: 1},
		},
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "ExternalLabelExists",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
				"templated_label", "There are 2 external Labels, of which foo is bar.",
			),
			Point: promql.Point{V: 1},
		},
	}

	evalTime := time.Unix(0, 0)
	result[0].Point.T = timestamp.FromTime(evalTime)
	result[1].Point.T = timestamp.FromTime(evalTime)

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := ruleWithoutExternalLabels.Eval(
		suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil,
	)
	testutil.Ok(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			testutil.Equals(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	res, err = ruleWithExternalLabels.Eval(
		suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil,
	)
	testutil.Ok(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			testutil.Equals(t, "ALERTS_FOR_STATE", smplName)
		}
	}

	testutil.Equals(t, result, filteredRes)
}

func TestAlertingRuleEmptyLabelFromTemplate(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			http_requests{job="app-server", instance="0"}	75 85 70 70
	`)
	testutil.Ok(t, err)
	defer suite.Close()

	testutil.Ok(t, suite.Run())

	expr, err := parser.ParseExpr(`http_requests < 100`)
	testutil.Ok(t, err)

	rule := NewAlertingRule(
		"EmptyLabel",
		expr,
		time.Minute,
		labels.FromStrings("empty_label", ""),
		nil,
		nil,
		true, log.NewNopLogger(),
	)
	result := promql.Vector{
		{
			Metric: labels.FromStrings(
				"__name__", "ALERTS",
				"alertname", "EmptyLabel",
				"alertstate", "pending",
				"instance", "0",
				"job", "app-server",
			),
			Point: promql.Point{V: 1},
		},
	}

	evalTime := time.Unix(0, 0)
	result[0].Point.T = timestamp.FromTime(evalTime)

	var filteredRes promql.Vector // After removing 'ALERTS_FOR_STATE' samples.
	res, err := rule.Eval(
		suite.Context(), evalTime, EngineQueryFunc(suite.QueryEngine(), suite.Storage()), nil,
	)
	testutil.Ok(t, err)
	for _, smpl := range res {
		smplName := smpl.Metric.Get("__name__")
		if smplName == "ALERTS" {
			filteredRes = append(filteredRes, smpl)
		} else {
			// If not 'ALERTS', it has to be 'ALERTS_FOR_STATE'.
			testutil.Equals(t, "ALERTS_FOR_STATE", smplName)
		}
	}
	testutil.Equals(t, result, filteredRes)
}

func TestAlertingRuleDuplicate(t *testing.T) {
	storage := teststorage.New(t)
	defer storage.Close()

	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}

	engine := promql.NewEngine(opts)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	now := time.Now()

	expr, _ := parser.ParseExpr(`vector(0) or label_replace(vector(0),"test","x","","")`)
	rule := NewAlertingRule(
		"foo",
		expr,
		time.Minute,
		labels.FromStrings("test", "test"),
		nil,
		nil,
		true, log.NewNopLogger(),
	)
	_, err := rule.Eval(ctx, now, EngineQueryFunc(engine, storage), nil)
	testutil.NotOk(t, err)
	e := fmt.Errorf("vector contains metrics with the same labelset after applying alert labels")
	testutil.ErrorEqual(t, e, err)
}
