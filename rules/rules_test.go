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
	"path"
	"strings"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/utility/test"
)

var (
	testEvalTime = testStartTime.Add(testSampleInterval * 10)
	fixturesPath = "fixtures"
)

func annotateWithTime(lines []string, timestamp clientmodel.Timestamp) []string {
	annotatedLines := []string{}
	for _, line := range lines {
		annotatedLines = append(annotatedLines, fmt.Sprintf(line, timestamp))
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

func newTestStorage(t testing.TB) (storage local.Storage, closer test.Closer) {
	storage, closer = local.NewTestStorage(t)
	storeMatrix(storage, testMatrix)
	return storage, closer
}

func TestExpressions(t *testing.T) {
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
			expr:           `SUM(http_requests)`,
			output:         []string{`http_requests => 3600 @[%v]`},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests{instance="0"}) BY(job)`,
			output: []string{
				`http_requests{job="api-server"} => 400 @[%v]`,
				`http_requests{job="app-server"} => 1200 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 4,
		}, {
			expr: `SUM(http_requests{instance="0"}) BY(job) KEEPING_EXTRA`,
			output: []string{
				`http_requests{instance="0", job="api-server"} => 400 @[%v]`,
				`http_requests{instance="0", job="app-server"} => 1200 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 4,
		}, {
			expr: `SUM(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 1000 @[%v]`,
				`http_requests{job="app-server"} => 2600 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			// Non-existent labels mentioned in BY-clauses shouldn't propagate to output.
			expr: `SUM(http_requests) BY (job, nonexistent)`,
			output: []string{
				`http_requests{job="api-server"} => 1000 @[%v]`,
				`http_requests{job="app-server"} => 2600 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `
				// Test comment.
				SUM(http_requests) BY /* comments shouldn't
				have any effect */ (job) // another comment`,
			output: []string{
				`http_requests{job="api-server"} => 1000 @[%v]`,
				`http_requests{job="app-server"} => 2600 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `COUNT(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 4 @[%v]`,
				`http_requests{job="app-server"} => 4 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job, group)`,
			output: []string{
				`http_requests{group="canary", job="api-server"} => 700 @[%v]`,
				`http_requests{group="canary", job="app-server"} => 1500 @[%v]`,
				`http_requests{group="production", job="api-server"} => 300 @[%v]`,
				`http_requests{group="production", job="app-server"} => 1100 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `AVG(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 250 @[%v]`,
				`http_requests{job="app-server"} => 650 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `MIN(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 100 @[%v]`,
				`http_requests{job="app-server"} => 500 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `MAX(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 400 @[%v]`,
				`http_requests{job="app-server"} => 800 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) - COUNT(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 996 @[%v]`,
				`http_requests{job="app-server"} => 2596 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `2 - SUM(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => -998 @[%v]`,
				`http_requests{job="app-server"} => -2598 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `1000 / SUM(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 1 @[%v]`,
				`http_requests{job="app-server"} => 0.38461538461538464 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) - 2`,
			output: []string{
				`http_requests{job="api-server"} => 998 @[%v]`,
				`http_requests{job="app-server"} => 2598 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) % 3`,
			output: []string{
				`http_requests{job="api-server"} => 1 @[%v]`,
				`http_requests{job="app-server"} => 2 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) / 0`,
			output: []string{
				`http_requests{job="api-server"} => +Inf @[%v]`,
				`http_requests{job="app-server"} => +Inf @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) > 1000`,
			output: []string{
				`http_requests{job="app-server"} => 2600 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `1000 < SUM(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="app-server"} => 1000 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) <= 1000`,
			output: []string{
				`http_requests{job="api-server"} => 1000 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) != 1000`,
			output: []string{
				`http_requests{job="app-server"} => 2600 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) == 1000`,
			output: []string{
				`http_requests{job="api-server"} => 1000 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `SUM(http_requests) BY (job) + SUM(http_requests) BY (job)`,
			output: []string{
				`http_requests{job="api-server"} => 2000 @[%v]`,
				`http_requests{job="app-server"} => 5200 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `http_requests{job="api-server", group="canary"}`,
			output: []string{
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 2,
		}, {
			expr: `http_requests{job="api-server", group="canary"} + delta(http_requests{job="api-server"}[5m], 1)`,
			output: []string{
				`http_requests{group="canary", instance="0", job="api-server"} => 330 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 440 @[%v]`,
			},
			fullRanges:     4,
			intervalRanges: 0,
		}, {
			expr: `delta(http_requests[25m], 1)`,
			output: []string{
				`http_requests{group="canary", instance="0", job="api-server"} => 150 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 350 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="canary", instance="1", job="app-server"} => 400 @[%v]`,
				`http_requests{group="production", instance="0", job="api-server"} => 50 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 250 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 300 @[%v]`,
			},
			fullRanges:     8,
			intervalRanges: 0,
		}, {
			expr: `sort(http_requests)`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
			},
			checkOrder:     true,
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `sort_desc(http_requests)`,
			output: []string{
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
			},
			checkOrder:     true,
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `topk(3, http_requests)`,
			output: []string{
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
			},
			checkOrder:     true,
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `topk(5, http_requests{group="canary",job="app-server"})`,
			output: []string{
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
			},
			checkOrder:     true,
			fullRanges:     0,
			intervalRanges: 2,
		}, {
			expr: `bottomk(3, http_requests)`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
			},
			checkOrder:     true,
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			expr: `bottomk(5, http_requests{group="canary",job="app-server"})`,
			output: []string{
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
			},
			checkOrder:     true,
			fullRanges:     0,
			intervalRanges: 2,
		}, {
			// Single-letter label names and values.
			expr: `x{y="testvalue"}`,
			output: []string{
				`x{y="testvalue"} => 100 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 1,
		}, {
			// Lower-cased aggregation operators should work too.
			expr: `sum(http_requests) by (job) + min(http_requests) by (job) + max(http_requests) by (job) + avg(http_requests) by (job)`,
			output: []string{
				`http_requests{job="app-server"} => 4550 @[%v]`,
				`http_requests{job="api-server"} => 1750 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			// Deltas should be adjusted for target interval vs. samples under target interval.
			expr:           `delta(http_requests{group="canary", instance="1", job="app-server"}[18m], 1)`,
			output:         []string{`http_requests{group="canary", instance="1", job="app-server"} => 288 @[%v]`},
			fullRanges:     1,
			intervalRanges: 0,
		}, {
			// Rates should transform per-interval deltas to per-second rates.
			expr:           `rate(http_requests{group="canary", instance="1", job="app-server"}[10m])`,
			output:         []string{`http_requests{group="canary", instance="1", job="app-server"} => 0.26666666666666666 @[%v]`},
			fullRanges:     1,
			intervalRanges: 0,
		}, {
			// Counter resets in middle of range are ignored by delta() if counter == 1.
			expr:           `delta(testcounter_reset_middle[50m], 1)`,
			output:         []string{`testcounter_reset_middle => 90 @[%v]`},
			fullRanges:     1,
			intervalRanges: 0,
		}, {
			// Counter resets in middle of range are not ignored by delta() if counter == 0.
			expr:           `delta(testcounter_reset_middle[50m], 0)`,
			output:         []string{`testcounter_reset_middle => 50 @[%v]`},
			fullRanges:     1,
			intervalRanges: 0,
		}, {
			// Counter resets at end of range are ignored by delta() if counter == 1.
			expr:           `delta(testcounter_reset_end[5m], 1)`,
			output:         []string{`testcounter_reset_end => 0 @[%v]`},
			fullRanges:     1,
			intervalRanges: 0,
		}, {
			// Counter resets at end of range are not ignored by delta() if counter == 0.
			expr:           `delta(testcounter_reset_end[5m], 0)`,
			output:         []string{`testcounter_reset_end => -90 @[%v]`},
			fullRanges:     1,
			intervalRanges: 0,
		}, {
			// count_scalar for a non-empty vector should return scalar element count.
			expr:           `count_scalar(http_requests)`,
			output:         []string{`scalar: 8 @[%v]`},
			fullRanges:     0,
			intervalRanges: 8,
		}, {
			// count_scalar for an empty vector should return scalar 0.
			expr:           `count_scalar(nonexistent)`,
			output:         []string{`scalar: 0 @[%v]`},
			fullRanges:     0,
			intervalRanges: 0,
		}, {
			// Empty expressions shouldn't parse.
			expr:       ``,
			shouldFail: true,
		}, {
			// Interval durations can't be in quotes.
			expr:       `http_requests["1m"]`,
			shouldFail: true,
		}, {
			// Binop arguments need to be scalar or vector.
			expr:       `http_requests - http_requests[1m]`,
			shouldFail: true,
		}, {
			expr: `http_requests{group!="canary"}`,
			output: []string{
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 4,
		}, {
			expr: `http_requests{job=~"server",group!="canary"}`,
			output: []string{
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 4,
		}, {
			expr: `http_requests{job!~"api",group!="canary"}`,
			output: []string{
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 2,
		}, {
			expr:           `count_scalar(http_requests{job=~"^server$"})`,
			output:         []string{`scalar: 0 @[%v]`},
			fullRanges:     0,
			intervalRanges: 0,
		}, {
			expr: `http_requests{group="production",job=~"^api"}`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 2,
		},
		{
			expr: `abs(-1 * http_requests{group="production",job="api-server"})`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 2,
		},
		{
			expr: `avg_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 50 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 100 @[%v]`,
			},
			fullRanges:     2,
			intervalRanges: 0,
		},
		{
			expr: `count_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 11 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 11 @[%v]`,
			},
			fullRanges:     2,
			intervalRanges: 0,
		},
		{
			expr: `max_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
			},
			fullRanges:     2,
			intervalRanges: 0,
		},
		{
			expr: `min_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 0 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 0 @[%v]`,
			},
			fullRanges:     2,
			intervalRanges: 0,
		},
		{
			expr: `sum_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 550 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 1100 @[%v]`,
			},
			fullRanges:     2,
			intervalRanges: 0,
		},
		{
			expr:           `time()`,
			output:         []string{`scalar: 3000 @[%v]`},
			fullRanges:     0,
			intervalRanges: 0,
		},
		{
			expr: `drop_common_labels(http_requests{group="production",job="api-server"})`,
			output: []string{
				`http_requests{instance="0"} => 100 @[%v]`,
				`http_requests{instance="1"} => 200 @[%v]`,
			},
			fullRanges:     0,
			intervalRanges: 2,
		},
	}

	storage, closer := newTestStorage(t)
	defer closer.Close()

	for i, exprTest := range expressionTests {
		expectedLines := annotateWithTime(exprTest.output, testEvalTime)

		testExpr, err := LoadExprFromString(exprTest.expr)

		if err != nil {
			if exprTest.shouldFail {
				continue
			}
			t.Errorf("%d. Error during parsing: %v", i, err)
			t.Errorf("%d. Expression: %v", i, exprTest.expr)
		} else {
			if exprTest.shouldFail {
				t.Errorf("%d. Test should fail, but didn't", i)
			}
			failed := false
			resultStr := ast.EvalToString(testExpr, testEvalTime, ast.TEXT, storage, stats.NewTimerGroup())
			resultLines := strings.Split(resultStr, "\n")

			if len(exprTest.output) != len(resultLines) {
				t.Errorf("%d. Number of samples in expected and actual output don't match", i)
				failed = true
			}

			if exprTest.checkOrder {
				for j, expectedSample := range expectedLines {
					if resultLines[j] != expectedSample {
						t.Errorf("%d.%d. Expected sample '%v', got '%v'", i, j, resultLines[j], expectedSample)
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
						t.Errorf("%d.%d. Couldn't find expected sample in output: '%v'", i, j, expectedSample)
						failed = true
					}
				}
			}

			analyzer := ast.NewQueryAnalyzer(storage)
			ast.Walk(analyzer, testExpr)
			if exprTest.fullRanges != len(analyzer.FullRanges) {
				t.Errorf("%d. Count of full ranges didn't match: %v vs %v", i, exprTest.fullRanges, len(analyzer.FullRanges))
				failed = true
			}
			if exprTest.intervalRanges != len(analyzer.IntervalRanges) {
				t.Errorf("%d. Count of interval ranges didn't match: %v vs %v", i, exprTest.intervalRanges, len(analyzer.IntervalRanges))
				failed = true
			}

			if failed {
				t.Errorf("%d. Expression: %v\n%v", i, exprTest.expr, vectorComparisonString(expectedLines, resultLines))
			}
		}
	}
}

var ruleTests = []struct {
	inputFile         string
	shouldFail        bool
	errContains       string
	numRecordingRules int
	numAlertingRules  int
}{
	{
		inputFile:         "empty.rules",
		numRecordingRules: 0,
		numAlertingRules:  0,
	}, {
		inputFile:         "mixed.rules",
		numRecordingRules: 2,
		numAlertingRules:  2,
	},
	{
		inputFile:   "syntax_error.rules",
		shouldFail:  true,
		errContains: "Error parsing rules at line 5",
	},
	{
		inputFile:   "non_vector.rules",
		shouldFail:  true,
		errContains: "does not evaluate to vector type",
	},
}

func TestRules(t *testing.T) {
	for i, ruleTest := range ruleTests {
		testRules, err := LoadRulesFromFile(path.Join(fixturesPath, ruleTest.inputFile))

		if err != nil {
			if !ruleTest.shouldFail {
				t.Fatalf("%d. Error parsing rules file %v: %v", i, ruleTest.inputFile, err)
			} else {
				if !strings.Contains(err.Error(), ruleTest.errContains) {
					t.Fatalf("%d. Expected error containing '%v', got: %v", i, ruleTest.errContains, err)
				}
			}
		} else {
			numRecordingRules := 0
			numAlertingRules := 0

			for j, rule := range testRules {
				switch rule.(type) {
				case *RecordingRule:
					numRecordingRules++
				case *AlertingRule:
					numAlertingRules++
				default:
					t.Fatalf("%d.%d. Unknown rule type!", i, j)
				}
			}

			if numRecordingRules != ruleTest.numRecordingRules {
				t.Fatalf("%d. Expected %d recording rules, got %d", i, ruleTest.numRecordingRules, numRecordingRules)
			}
			if numAlertingRules != ruleTest.numAlertingRules {
				t.Fatalf("%d. Expected %d alerting rules, got %d", i, ruleTest.numAlertingRules, numAlertingRules)
			}

			// TODO(julius): add more complex checks on the parsed rules here.
		}
	}
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

	storage, closer := newTestStorage(t)
	defer closer.Close()

	alertExpr, err := LoadExprFromString(`http_requests{group="canary", job="app-server"} < 100`)
	if err != nil {
		t.Fatalf("Unable to parse alert expression: %s", err)
	}
	alertName := "HttpRequestRateLow"
	alertLabels := clientmodel.LabelSet{
		"severity": "critical",
	}
	rule := NewAlertingRule(alertName, alertExpr.(ast.VectorNode), time.Minute, alertLabels, "summary", "description")

	for i, expected := range evalOutputs {
		evalTime := testStartTime.Add(testSampleInterval * time.Duration(i))
		actual, err := rule.Eval(evalTime, storage)
		if err != nil {
			t.Fatalf("Error during alerting rule evaluation: %s", err)
		}
		actualLines := strings.Split(actual.String(), "\n")
		expectedLines := annotateWithTime(expected, evalTime)
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
