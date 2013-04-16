// Copyright 2013 Prometheus Team
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
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
	"strings"
	"testing"
	"time"
)

var testEvalTime = testStartTime.Add(testDuration5m * 10)

// Labels in expected output need to be alphabetically sorted.
var expressionTests = []struct {
	expr           string
	output         []string
	shouldFail     bool
	checkOrder     bool
	fullRanges     int
	intervalRanges int
}{
	{
		expr:           "SUM(http_requests)",
		output:         []string{"http_requests{} => 3600 @[%v]"},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job, group)",
		output: []string{
			"http_requests{group='canary',job='api-server'} => 700 @[%v]",
			"http_requests{group='canary',job='app-server'} => 1500 @[%v]",
			"http_requests{group='production',job='api-server'} => 300 @[%v]",
			"http_requests{group='production',job='app-server'} => 1100 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "AVG(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 250 @[%v]",
			"http_requests{job='app-server'} => 650 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "MIN(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 100 @[%v]",
			"http_requests{job='app-server'} => 500 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "MAX(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 400 @[%v]",
			"http_requests{job='app-server'} => 800 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) - count(http_requests)",
		output: []string{
			"http_requests{job='api-server'} => 992 @[%v]",
			"http_requests{job='app-server'} => 2592 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) - 2",
		output: []string{
			"http_requests{job='api-server'} => 998 @[%v]",
			"http_requests{job='app-server'} => 2598 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) % 3",
		output: []string{
			"http_requests{job='api-server'} => 1 @[%v]",
			"http_requests{job='app-server'} => 2 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) / 0",
		output: []string{
			"http_requests{job='api-server'} => +Inf @[%v]",
			"http_requests{job='app-server'} => +Inf @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) > 1000",
		output: []string{
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) <= 1000",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) != 1000",
		output: []string{
			"http_requests{job='app-server'} => 2600 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) == 1000",
		output: []string{
			"http_requests{job='api-server'} => 1000 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "SUM(http_requests) BY (job) + SUM(http_requests) BY (job)",
		output: []string{
			"http_requests{job='api-server'} => 2000 @[%v]",
			"http_requests{job='app-server'} => 5200 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "http_requests{job='api-server', group='canary'}",
		output: []string{
			"http_requests{group='canary',instance='0',job='api-server'} => 300 @[%v]",
			"http_requests{group='canary',instance='1',job='api-server'} => 400 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 2,
	}, {
		expr: "http_requests{job='api-server', group='canary'} + delta(http_requests{job='api-server'}[5m], 1)",
		output: []string{
			"http_requests{group='canary',instance='0',job='api-server'} => 330 @[%v]",
			"http_requests{group='canary',instance='1',job='api-server'} => 440 @[%v]",
		},
		fullRanges:     4,
		intervalRanges: 0,
	}, {
		expr: "delta(http_requests[25m], 1)",
		output: []string{
			"http_requests{group='canary',instance='0',job='api-server'} => 150 @[%v]",
			"http_requests{group='canary',instance='0',job='app-server'} => 350 @[%v]",
			"http_requests{group='canary',instance='1',job='api-server'} => 200 @[%v]",
			"http_requests{group='canary',instance='1',job='app-server'} => 400 @[%v]",
			"http_requests{group='production',instance='0',job='api-server'} => 50 @[%v]",
			"http_requests{group='production',instance='0',job='app-server'} => 250 @[%v]",
			"http_requests{group='production',instance='1',job='api-server'} => 100 @[%v]",
			"http_requests{group='production',instance='1',job='app-server'} => 300 @[%v]",
		},
		fullRanges:     8,
		intervalRanges: 0,
	}, {
		expr: "sort(http_requests)",
		output: []string{
			"http_requests{group='production',instance='0',job='api-server'} => 100 @[%v]",
			"http_requests{group='production',instance='1',job='api-server'} => 200 @[%v]",
			"http_requests{group='canary',instance='0',job='api-server'} => 300 @[%v]",
			"http_requests{group='canary',instance='1',job='api-server'} => 400 @[%v]",
			"http_requests{group='production',instance='0',job='app-server'} => 500 @[%v]",
			"http_requests{group='production',instance='1',job='app-server'} => 600 @[%v]",
			"http_requests{group='canary',instance='0',job='app-server'} => 700 @[%v]",
			"http_requests{group='canary',instance='1',job='app-server'} => 800 @[%v]",
		},
		checkOrder:     true,
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		expr: "sort_desc(http_requests)",
		output: []string{
			"http_requests{group='canary',instance='1',job='app-server'} => 800 @[%v]",
			"http_requests{group='canary',instance='0',job='app-server'} => 700 @[%v]",
			"http_requests{group='production',instance='1',job='app-server'} => 600 @[%v]",
			"http_requests{group='production',instance='0',job='app-server'} => 500 @[%v]",
			"http_requests{group='canary',instance='1',job='api-server'} => 400 @[%v]",
			"http_requests{group='canary',instance='0',job='api-server'} => 300 @[%v]",
			"http_requests{group='production',instance='1',job='api-server'} => 200 @[%v]",
			"http_requests{group='production',instance='0',job='api-server'} => 100 @[%v]",
		},
		checkOrder:     true,
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		// Single-letter label names and values.
		expr: "x{y='testvalue'}",
		output: []string{
			"x{y='testvalue'} => 100 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 1,
	}, {
		// Lower-cased aggregation operators should work too.
		expr: "sum(http_requests) by (job) + min(http_requests) by (job) + max(http_requests) by (job) + avg(http_requests) by (job)",
		output: []string{
			"http_requests{job='app-server'} => 4550 @[%v]",
			"http_requests{job='api-server'} => 1750 @[%v]",
		},
		fullRanges:     0,
		intervalRanges: 8,
	}, {
		// Deltas should be adjusted for target interval vs. samples under target interval.
		expr:           "delta(http_requests{group='canary',instance='1',job='app-server'}[18m], 1)",
		output:         []string{"http_requests{group='canary',instance='1',job='app-server'} => 288 @[%v]"},
		fullRanges:     1,
		intervalRanges: 0,
	}, {
		// Rates should transform per-interval deltas to per-second rates.
		expr:           "rate(http_requests{group='canary',instance='1',job='app-server'}[10m])",
		output:         []string{"http_requests{group='canary',instance='1',job='app-server'} => 0.26666668 @[%v]"},
		fullRanges:     1,
		intervalRanges: 0,
	}, {
		// Empty expressions shouldn't parse.
		expr:       "",
		shouldFail: true,
	}, {
		// Subtracting a vector from a scalar is not supported.
		expr:       "1 - http_requests",
		shouldFail: true,
	}, {
		// Interval durations can't be in quotes.
		expr:       "http_requests['1m']",
		shouldFail: true,
	},
}

func annotateWithTime(lines []string) []string {
	annotatedLines := []string{}
	for _, line := range lines {
		annotatedLines = append(annotatedLines, fmt.Sprintf(line, testEvalTime))
	}
	return annotatedLines
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

func TestExpressions(t *testing.T) {
	temporaryDirectory := test.NewTemporaryDirectory("rule_expression_tests", t)
	defer temporaryDirectory.Close()
	tieredStorage, err := metric.NewTieredStorage(5000, 5000, 100, time.Second*30, time.Second*1, time.Second*20, temporaryDirectory.Path())
	if err != nil {
		t.Fatalf("Error opening storage: %s", err)
	}
	go tieredStorage.Serve()

	ast.SetStorage(tieredStorage)

	storeMatrix(tieredStorage, testMatrix)
	tieredStorage.Flush()

	for i, exprTest := range expressionTests {
		expectedLines := annotateWithTime(exprTest.output)

		testExpr, err := LoadExprFromString(exprTest.expr)

		if err != nil {
			if exprTest.shouldFail {
				continue
			}
			t.Errorf("%d Error during parsing: %v", i, err)
			t.Errorf("%d Expression: %v", i, exprTest.expr)
		} else {
			if exprTest.shouldFail {
				t.Errorf("%d Test should fail, but didn't", i)
			}
			failed := false
			resultStr := ast.EvalToString(testExpr, testEvalTime, ast.TEXT)
			resultLines := strings.Split(resultStr, "\n")

			if len(exprTest.output) != len(resultLines) {
				t.Errorf("%d Number of samples in expected and actual output don't match", i)
				failed = true
			}

			if exprTest.checkOrder {
				for j, expectedSample := range expectedLines {
					if resultLines[j] != expectedSample {
						t.Errorf("%d.%d Expected sample '%v', got '%v'", i, j, resultLines[j], expectedSample)
						failed = true
					}
				}
			} else {
				for j, expectedSample := range expectedLines {
					found := false
					for _, actualSample := range resultLines {
						if actualSample == expectedSample {
							found = true
						}
					}
					if !found {
						t.Errorf("%d.%d Couldn't find expected sample in output: '%v'", i, j, expectedSample)
						failed = true
					}
				}
			}

			analyzer := ast.NewQueryAnalyzer()
			analyzer.AnalyzeQueries(testExpr)
			if exprTest.fullRanges != len(analyzer.FullRanges) {
				t.Errorf("%d Count of full ranges didn't match: %v vs %v", i, exprTest.fullRanges, len(analyzer.FullRanges))
				failed = true
			}
			if exprTest.intervalRanges != len(analyzer.IntervalRanges) {
				t.Errorf("%d Count of interval ranges didn't match: %v vs %v", i, exprTest.intervalRanges, len(analyzer.IntervalRanges))
				failed = true
			}

			if failed {
				t.Errorf("%d Expression: %v\n%v", i, exprTest.expr, vectorComparisonString(expectedLines, resultLines))
			}
		}
	}
}
