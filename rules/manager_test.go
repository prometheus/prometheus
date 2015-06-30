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
	"reflect"
	"strings"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/promql"
)

func TestAlertingRule(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 5m
			http_requests{job="api-server", instance="0", group="production"}	0+10x10
			http_requests{job="api-server", instance="1", group="production"}	0+20x10
			http_requests{job="api-server", instance="0", group="canary"}		0+30x10
			http_requests{job="api-server", instance="1", group="canary"}		0+40x10
			http_requests{job="app-server", instance="0", group="production"}	0+50x10
			http_requests{job="app-server", instance="1", group="production"}	0+60x10
			http_requests{job="app-server", instance="0", group="canary"}		0+70x10
			http_requests{job="app-server", instance="1", group="canary"}		0+80x10
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
		clientmodel.LabelSet{"severity": "critical"},
		"summary", "description", "runbook",
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
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="1", job="app-server", severity="critical"} => 0 @[%v]`,
				`ALERTS{alertname="HTTPRequestRateLow", alertstate="firing", group="canary", instance="0", job="app-server", severity="critical"} => 0 @[%v]`,
			},
		},
		{
			time:   15 * time.Minute,
			result: nil,
		},
		{
			time:   20 * time.Minute,
			result: nil,
		},
	}

	for i, test := range tests {
		evalTime := clientmodel.Timestamp(0).Add(test.time)

		res, err := rule.eval(evalTime, suite.QueryEngine())
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
	}
}

func annotateWithTime(lines []string, timestamp clientmodel.Timestamp) []string {
	annotatedLines := []string{}
	for _, line := range lines {
		annotatedLines = append(annotatedLines, fmt.Sprintf(line, timestamp))
	}
	return annotatedLines
}

func TestTransferAlertState(t *testing.T) {
	m := NewManager(&ManagerOptions{})

	alert := &Alert{
		Name:  "testalert",
		State: StateFiring,
	}

	arule := AlertingRule{
		name:         "test",
		activeAlerts: map[clientmodel.Fingerprint]*Alert{},
	}
	aruleCopy := arule

	m.rules = append(m.rules, &arule)

	// Set an alert.
	arule.activeAlerts[0] = alert

	// Save state and get the restore function.
	restore := m.transferAlertState()

	// Remove arule from the rule list and add an unrelated rule and the
	// stateless copy of arule.
	m.rules = []Rule{
		&AlertingRule{
			name:         "test_other",
			activeAlerts: map[clientmodel.Fingerprint]*Alert{},
		},
		&aruleCopy,
	}

	// Apply the restore function.
	restore()

	if ar := m.rules[0].(*AlertingRule); len(ar.activeAlerts) != 0 {
		t.Fatalf("unexpected alert for unrelated alerting rule")
	}
	if ar := m.rules[1].(*AlertingRule); !reflect.DeepEqual(ar.activeAlerts[0], alert) {
		t.Fatalf("alert state was not restored")
	}
}
