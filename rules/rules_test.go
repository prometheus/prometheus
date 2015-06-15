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

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
)

var (
	testSampleInterval = time.Duration(5) * time.Minute
	testStartTime      = clientmodel.Timestamp(0)
)

func getTestValueStream(startVal clientmodel.SampleValue, endVal clientmodel.SampleValue, stepVal clientmodel.SampleValue, startTime clientmodel.Timestamp) (resultValues metric.Values) {
	currentTime := startTime
	for currentVal := startVal; currentVal <= endVal; currentVal += stepVal {
		sample := metric.SamplePair{
			Value:     currentVal,
			Timestamp: currentTime,
		}
		resultValues = append(resultValues, sample)
		currentTime = currentTime.Add(testSampleInterval)
	}
	return resultValues
}

func getTestVectorFromTestMatrix(matrix promql.Matrix) promql.Vector {
	vector := promql.Vector{}
	for _, sampleStream := range matrix {
		lastSample := sampleStream.Values[len(sampleStream.Values)-1]
		vector = append(vector, &promql.Sample{
			Metric:    sampleStream.Metric,
			Value:     lastSample.Value,
			Timestamp: lastSample.Timestamp,
		})
	}
	return vector
}

func storeMatrix(storage local.Storage, matrix promql.Matrix) {
	pendingSamples := clientmodel.Samples{}
	for _, sampleStream := range matrix {
		for _, sample := range sampleStream.Values {
			pendingSamples = append(pendingSamples, &clientmodel.Sample{
				Metric:    sampleStream.Metric.Metric,
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			})
		}
	}
	for _, s := range pendingSamples {
		storage.Append(s)
	}
	storage.WaitForIndexing()
}

func vectorComparisonString(expected []string, actual []string) string {
	separator := "\n--------------\n"
	return fmt.Sprintf("Expected:%v%v%v\nActual:%v%v%v ",
		separator,
		strings.Join(expected, "\n"),
		separator,
		separator,
		strings.Join(actual, "\n"),
		separator)
}

func annotateWithTime(lines []string, timestamp clientmodel.Timestamp) []string {
	annotatedLines := []string{}
	for _, line := range lines {
		annotatedLines = append(annotatedLines, fmt.Sprintf(line, timestamp))
	}
	return annotatedLines
}

var testMatrix = promql.Matrix{
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "0",
				"group":                     "canary",
			},
		},
		Values: getTestValueStream(0, 300, 30, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "1",
				"group":                     "canary",
			},
		},
		Values: getTestValueStream(0, 400, 40, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "app-server",
				"instance":                  "0",
				"group":                     "canary",
			},
		},
		Values: getTestValueStream(0, 700, 70, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "app-server",
				"instance":                  "1",
				"group":                     "canary",
			},
		},
		Values: getTestValueStream(0, 800, 80, testStartTime),
	},
}

func TestAlertingRule(t *testing.T) {
	// Labels in expected output need to be alphabetically sorted.
	var evalOutputs = [][]string{
		{
			`ALERTS{alertname="HttpRequestRateLow", alertstate="pending", group="canary", instance="0", job="app-server", severity="critical"} => 1 @[%v]`,
			`ALERTS{alertname="HttpRequestRateLow", alertstate="pending", group="canary", instance="1", job="app-server", severity="critical"} => 1 @[%v]`,
		},
		{
			`ALERTS{alertname="HttpRequestRateLow", alertstate="pending", group="canary", instance="0", job="app-server", severity="critical"} => 0 @[%v]`,
			`ALERTS{alertname="HttpRequestRateLow", alertstate="firing", group="canary", instance="0", job="app-server", severity="critical"} => 1 @[%v]`,
			`ALERTS{alertname="HttpRequestRateLow", alertstate="pending", group="canary", instance="1", job="app-server", severity="critical"} => 0 @[%v]`,
			`ALERTS{alertname="HttpRequestRateLow", alertstate="firing", group="canary", instance="1", job="app-server", severity="critical"} => 1 @[%v]`,
		},
		{
			`ALERTS{alertname="HttpRequestRateLow", alertstate="firing", group="canary", instance="1", job="app-server", severity="critical"} => 0 @[%v]`,
			`ALERTS{alertname="HttpRequestRateLow", alertstate="firing", group="canary", instance="0", job="app-server", severity="critical"} => 0 @[%v]`,
		},
		{
		/* empty */
		},
		{
		/* empty */
		},
	}

	storage, closer := local.NewTestStorage(t, 1)
	defer closer.Close()

	storeMatrix(storage, testMatrix)

	engine := promql.NewEngine(storage, nil)
	defer engine.Stop()

	expr, err := promql.ParseExpr(`http_requests{group="canary", job="app-server"} < 100`)
	if err != nil {
		t.Fatalf("Unable to parse alert expression: %s", err)
	}

	alertLabels := clientmodel.LabelSet{
		"severity": "critical",
	}
	rule := NewAlertingRule("HttpRequestRateLow", expr, time.Minute, alertLabels, "summary", "description")

	for i, expectedLines := range evalOutputs {
		evalTime := testStartTime.Add(testSampleInterval * time.Duration(i))

		res, err := rule.eval(evalTime, engine)
		if err != nil {
			t.Fatalf("Error during alerting rule evaluation: %s", err)
		}

		actualLines := strings.Split(res.String(), "\n")
		expectedLines := annotateWithTime(expectedLines, evalTime)
		if actualLines[0] == "" {
			actualLines = []string{}
		}

		failed := false
		if len(actualLines) != len(expectedLines) {
			t.Errorf("%d. Number of samples in expected and actual output don't match (%d vs. %d)", i, len(expectedLines), len(actualLines))
			failed = true
		}

		for j, expectedSample := range expectedLines {
			found := false
			for _, actualSample := range actualLines {
				if actualSample == expectedSample {
					found = true
				}
			}
			if !found {
				t.Errorf("%d.%d. Couldn't find expected sample in output: '%v'", i, j, expectedSample)
				failed = true
			}
		}

		if failed {
			t.Fatalf("%d. Expected and actual outputs don't match:\n%v", i, vectorComparisonString(expectedLines, actualLines))
		}
	}
}
