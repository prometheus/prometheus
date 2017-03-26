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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql"
)

func TestAlertingRule(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 5m
			http_requests{job="app-server", instance="0", group="canary", severity="overwrite-me"}	75 85  95 105 105  95  85
			http_requests{job="app-server", instance="1", group="canary", severity="overwrite-me"}	80 90 100 110 120 130 140
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer suite.Close()

	if err := suite.Run(); err != nil {
		t.Fatal(err)
	}

	expr, err := promql.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	if err != nil {
		t.Fatalf("Unable to parse alert expression: %s", err)
	}

	rule := NewAlertingRule(
		"HTTPRequestRateLow",
		expr,
		time.Minute,
		model.LabelSet{"severity": "{{\"c\"}}ritical"},
		model.LabelSet{},
	)

	var tests = []struct {
		time   time.Duration
		result []string
	}{
		{
			time: 0,
			result: []string{
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="pending", group="canary", instance="0", job="app-server", severity="critical"} => 1 @[%v]`,
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="pending", group="canary", instance="1", job="app-server", severity="critical"} => 1 @[%v]`,
			},
		}, {
			time: 5 * time.Minute,
			result: []string{
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="pending", group="canary", instance="0", job="app-server", severity="critical"} => 0 @[%v]`,
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="0", job="app-server", severity="critical"} => 1 @[%v]`,
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="pending", group="canary", instance="1", job="app-server", severity="critical"} => 0 @[%v]`,
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="1", job="app-server", severity="critical"} => 1 @[%v]`,
			},
		}, {
			time: 10 * time.Minute,
			result: []string{
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="0", job="app-server", severity="critical"} => 1 @[%v]`,
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="1", job="app-server", severity="critical"} => 0 @[%v]`,
			},
		},
		{
			time: 15 * time.Minute,
			result: []string{
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="0", job="app-server", severity="critical"} => 0 @[%v]`,
			},
		},
		{
			time:   20 * time.Minute,
			result: []string{},
		},
		{
			time: 25 * time.Minute,
			result: []string{
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="pending", group="canary", instance="0", job="app-server", severity="critical"} => 1 @[%v]`,
			},
		},
		{
			time: 30 * time.Minute,
			result: []string{
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="pending", group="canary", instance="0", job="app-server", severity="critical"} => 0 @[%v]`,
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="0", job="app-server", severity="critical"} => 1 @[%v]`,
			},
		},
	}

	for i, test := range tests {
		evalTime := model.Time(0).Add(test.time)

		res, err := rule.Eval(suite.Context(), evalTime, suite.QueryEngine(), "")
		if err != nil {
			t.Fatalf("Error during alerting rule evaluation: %s", err)
		}

		actual := strings.Split(res.String(), "\n")
		expected := annotateWithTime(test.result, evalTime)
		if actual[0] == "" {
			actual = []string{}
		}

		if len(actual) != len(expected) {
			t.Errorf("%d. Number of samples in expected and actual output don't match (%d vs. %d)", i, len(expected), len(actual))
		}

		for j, expectedSample := range expected {
			found := false
			for _, actualSample := range actual {
				if actualSample == expectedSample {
					found = true
				}
			}
			if !found {
				t.Errorf("%d.%d. Couldn't find expected sample in output: '%v'", i, j, expectedSample)
			}
		}

		if t.Failed() {
			t.Errorf("%d. Expected and actual outputs don't match:", i)
			t.Fatalf("Expected:\n%v\n----\nActual:\n%v", strings.Join(expected, "\n"), strings.Join(actual, "\n"))
		}

		for _, aa := range rule.ActiveAlerts() {
			if _, ok := aa.Labels[model.MetricNameLabel]; ok {
				t.Fatalf("%s label set on active alert: %s", model.MetricNameLabel, aa.Labels)
			}
		}
	}
}

func annotateWithTime(lines []string, timestamp model.Time) []string {
	annotatedLines := []string{}
	for _, line := range lines {
		annotatedLines = append(annotatedLines, fmt.Sprintf(line, timestamp))
	}
	return annotatedLines
}
