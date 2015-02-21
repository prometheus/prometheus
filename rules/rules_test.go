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
	"path"
	"strings"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
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
	expressionTests := []struct {
		expr       string
		output     []string
		shouldFail bool
		checkOrder bool
	}{
		{
			expr:   `SUM(http_requests)`,
			output: []string{`{} => 3600 @[%v]`},
		}, {
			expr: `SUM(http_requests{instance="0"}) BY(job)`,
			output: []string{
				`{job="api-server"} => 400 @[%v]`,
				`{job="app-server"} => 1200 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests{instance="0"}) BY(job) KEEPING_EXTRA`,
			output: []string{
				`{instance="0", job="api-server"} => 400 @[%v]`,
				`{instance="0", job="app-server"} => 1200 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 1000 @[%v]`,
				`{job="app-server"} => 2600 @[%v]`,
			},
		}, {
			// Non-existent labels mentioned in BY-clauses shouldn't propagate to output.
			expr: `SUM(http_requests) BY (job, nonexistent)`,
			output: []string{
				`{job="api-server"} => 1000 @[%v]`,
				`{job="app-server"} => 2600 @[%v]`,
			},
		}, {
			expr: `
				// Test comment.
				SUM(http_requests) BY /* comments shouldn't
				have any effect */ (job) // another comment`,
			output: []string{
				`{job="api-server"} => 1000 @[%v]`,
				`{job="app-server"} => 2600 @[%v]`,
			},
		}, {
			expr: `COUNT(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 4 @[%v]`,
				`{job="app-server"} => 4 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job, group)`,
			output: []string{
				`{group="canary", job="api-server"} => 700 @[%v]`,
				`{group="canary", job="app-server"} => 1500 @[%v]`,
				`{group="production", job="api-server"} => 300 @[%v]`,
				`{group="production", job="app-server"} => 1100 @[%v]`,
			},
		}, {
			expr: `AVG(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 250 @[%v]`,
				`{job="app-server"} => 650 @[%v]`,
			},
		}, {
			expr: `MIN(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 100 @[%v]`,
				`{job="app-server"} => 500 @[%v]`,
			},
		}, {
			expr: `MAX(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 400 @[%v]`,
				`{job="app-server"} => 800 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) - COUNT(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 996 @[%v]`,
				`{job="app-server"} => 2596 @[%v]`,
			},
		}, {
			expr: `2 - SUM(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => -998 @[%v]`,
				`{job="app-server"} => -2598 @[%v]`,
			},
		}, {
			expr: `1000 / SUM(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 1 @[%v]`,
				`{job="app-server"} => 0.38461538461538464 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) - 2`,
			output: []string{
				`{job="api-server"} => 998 @[%v]`,
				`{job="app-server"} => 2598 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) % 3`,
			output: []string{
				`{job="api-server"} => 1 @[%v]`,
				`{job="app-server"} => 2 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) / 0`,
			output: []string{
				`{job="api-server"} => +Inf @[%v]`,
				`{job="app-server"} => +Inf @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) > 1000`,
			output: []string{
				`{job="app-server"} => 2600 @[%v]`,
			},
		}, {
			expr: `1000 < SUM(http_requests) BY (job)`,
			output: []string{
				`{job="app-server"} => 1000 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) <= 1000`,
			output: []string{
				`{job="api-server"} => 1000 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) != 1000`,
			output: []string{
				`{job="app-server"} => 2600 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) == 1000`,
			output: []string{
				`{job="api-server"} => 1000 @[%v]`,
			},
		}, {
			expr: `SUM(http_requests) BY (job) + SUM(http_requests) BY (job)`,
			output: []string{
				`{job="api-server"} => 2000 @[%v]`,
				`{job="app-server"} => 5200 @[%v]`,
			},
		}, {
			expr: `http_requests{job="api-server", group="canary"}`,
			output: []string{
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
			},
		}, {
			expr: `http_requests{job="api-server", group="canary"} + rate(http_requests{job="api-server"}[5m]) * 5 * 60`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 330 @[%v]`,
				`{group="canary", instance="1", job="api-server"} => 440 @[%v]`,
			},
		}, {
			expr: `rate(http_requests[25m]) * 25 * 60`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 150 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 350 @[%v]`,
				`{group="canary", instance="1", job="api-server"} => 200 @[%v]`,
				`{group="canary", instance="1", job="app-server"} => 400 @[%v]`,
				`{group="production", instance="0", job="api-server"} => 50 @[%v]`,
				`{group="production", instance="0", job="app-server"} => 249.99999999999997 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 100 @[%v]`,
				`{group="production", instance="1", job="app-server"} => 300 @[%v]`,
			},
		}, {
			expr: `delta(http_requests[25m], 1)`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 150 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 350 @[%v]`,
				`{group="canary", instance="1", job="api-server"} => 200 @[%v]`,
				`{group="canary", instance="1", job="app-server"} => 400 @[%v]`,
				`{group="production", instance="0", job="api-server"} => 50 @[%v]`,
				`{group="production", instance="0", job="app-server"} => 250 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 100 @[%v]`,
				`{group="production", instance="1", job="app-server"} => 300 @[%v]`,
			},
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
			checkOrder: true,
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
			checkOrder: true,
		}, {
			expr: `topk(3, http_requests)`,
			output: []string{
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
			},
			checkOrder: true,
		}, {
			expr: `topk(5, http_requests{group="canary",job="app-server"})`,
			output: []string{
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
			},
			checkOrder: true,
		}, {
			expr: `bottomk(3, http_requests)`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
			},
			checkOrder: true,
		}, {
			expr: `bottomk(5, http_requests{group="canary",job="app-server"})`,
			output: []string{
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
			},
			checkOrder: true,
		}, {
			// Single-letter label names and values.
			expr: `x{y="testvalue"}`,
			output: []string{
				`x{y="testvalue"} => 100 @[%v]`,
			},
		}, {
			// Lower-cased aggregation operators should work too.
			expr: `sum(http_requests) by (job) + min(http_requests) by (job) + max(http_requests) by (job) + avg(http_requests) by (job)`,
			output: []string{
				`{job="app-server"} => 4550 @[%v]`,
				`{job="api-server"} => 1750 @[%v]`,
			},
		}, {
			// Deltas should be adjusted for target interval vs. samples under target interval.
			expr:   `delta(http_requests{group="canary", instance="1", job="app-server"}[18m])`,
			output: []string{`{group="canary", instance="1", job="app-server"} => 288 @[%v]`},
		}, {
			// Deltas should perform the same operation when 2nd argument is 0.
			expr:   `delta(http_requests{group="canary", instance="1", job="app-server"}[18m], 0)`,
			output: []string{`{group="canary", instance="1", job="app-server"} => 288 @[%v]`},
		}, {
			// Rates should calculate per-second rates.
			expr:   `rate(http_requests{group="canary", instance="1", job="app-server"}[60m])`,
			output: []string{`{group="canary", instance="1", job="app-server"} => 0.26666666666666666 @[%v]`},
		}, {
			// Deriv should return the same as rate in simple cases.
			expr:   `deriv(http_requests{group="canary", instance="1", job="app-server"}[60m])`,
			output: []string{`{group="canary", instance="1", job="app-server"} => 0.26666666666666666 @[%v]`},
		}, {
			// Counter resets at in the middle of range are handled correctly by rate().
			expr:   `rate(testcounter_reset_middle[60m])`,
			output: []string{`{} => 0.03 @[%v]`},
		}, {
			// Counter resets at end of range are ignored by rate().
			expr:   `rate(testcounter_reset_end[5m])`,
			output: []string{`{} => 0 @[%v]`},
		}, {
			// Deriv should return correct result.
			expr:   `deriv(testcounter_reset_middle[100m])`,
			output: []string{`{} => 0.010606060606060607 @[%v]`},
		}, {
			// count_scalar for a non-empty vector should return scalar element count.
			expr:   `count_scalar(http_requests)`,
			output: []string{`scalar: 8 @[%v]`},
		}, {
			// count_scalar for an empty vector should return scalar 0.
			expr:   `count_scalar(nonexistent)`,
			output: []string{`scalar: 0 @[%v]`},
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
		}, {
			expr: `http_requests{job=~"server",group!="canary"}`,
			output: []string{
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
			},
		}, {
			expr: `http_requests{job!~"api",group!="canary"}`,
			output: []string{
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
			},
		}, {
			expr:   `count_scalar(http_requests{job=~"^server$"})`,
			output: []string{`scalar: 0 @[%v]`},
		}, {
			expr: `http_requests{group="production",job=~"^api"}`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
			},
		},
		{
			expr: `abs(-1 * http_requests{group="production",job="api-server"})`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 200 @[%v]`,
			},
		},
		{
			expr: `floor(0.004 * http_requests{group="production",job="api-server"})`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 0 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 0 @[%v]`,
			},
		},
		{
			expr: `ceil(0.004 * http_requests{group="production",job="api-server"})`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 1 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 1 @[%v]`,
			},
		},
		{
			expr: `round(0.004 * http_requests{group="production",job="api-server"})`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 0 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 1 @[%v]`,
			},
		},
		{ // Round should correctly handle negative numbers.
			expr: `round(-1 * (0.004 * http_requests{group="production",job="api-server"}))`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 0 @[%v]`,
				`{group="production", instance="1", job="api-server"} => -1 @[%v]`,
			},
		},
		{ // Round should round half up.
			expr: `round(0.005 * http_requests{group="production",job="api-server"})`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 1 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 1 @[%v]`,
			},
		},
		{
			expr: `round(-1 * (0.005 * http_requests{group="production",job="api-server"}))`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 0 @[%v]`,
				`{group="production", instance="1", job="api-server"} => -1 @[%v]`,
			},
		},
		{
			expr: `round(1 + 0.005 * http_requests{group="production",job="api-server"})`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 2 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 2 @[%v]`,
			},
		},
		{
			expr: `round(-1 * (1 + 0.005 * http_requests{group="production",job="api-server"}))`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => -1 @[%v]`,
				`{group="production", instance="1", job="api-server"} => -2 @[%v]`,
			},
		},
		{ // Round should accept the number to round nearest to.
			expr: `round(0.0005 * http_requests{group="production",job="api-server"}, 0.1)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 0.1 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 0.1 @[%v]`,
			},
		},
		{
			expr: `round(2.1 + 0.0005 * http_requests{group="production",job="api-server"}, 0.1)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 2.2 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 2.2 @[%v]`,
			},
		},
		{
			expr: `round(5.2 + 0.0005 * http_requests{group="production",job="api-server"}, 0.1)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 5.3 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 5.3 @[%v]`,
			},
		},
		{ // Round should work correctly with negative numbers and multiple decimal places.
			expr: `round(-1 * (5.2 + 0.0005 * http_requests{group="production",job="api-server"}), 0.1)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => -5.2 @[%v]`,
				`{group="production", instance="1", job="api-server"} => -5.3 @[%v]`,
			},
		},
		{ // Round should work correctly with big toNearests.
			expr: `round(0.025 * http_requests{group="production",job="api-server"}, 5)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 5 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 5 @[%v]`,
			},
		},
		{
			expr: `round(0.045 * http_requests{group="production",job="api-server"}, 5)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 5 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 10 @[%v]`,
			},
		},
		{
			expr: `avg_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 50 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 100 @[%v]`,
			},
		},
		{
			expr: `count_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 11 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 11 @[%v]`,
			},
		},
		{
			expr: `max_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 200 @[%v]`,
			},
		},
		{
			expr: `min_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 0 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 0 @[%v]`,
			},
		},
		{
			expr: `sum_over_time(http_requests{group="production",job="api-server"}[1h])`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 550 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 1100 @[%v]`,
			},
		},
		{
			expr:   `time()`,
			output: []string{`scalar: 3000 @[%v]`},
		},
		{
			expr: `drop_common_labels(http_requests{group="production",job="api-server"})`,
			output: []string{
				`http_requests{instance="0"} => 100 @[%v]`,
				`http_requests{instance="1"} => 200 @[%v]`,
			},
		},
		{
			expr: `{` + string(clientmodel.MetricNameLabel) + `=~".*"}`,
			output: []string{
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`testcounter_reset_end => 0 @[%v]`,
				`testcounter_reset_middle => 50 @[%v]`,
				`x{y="testvalue"} => 100 @[%v]`,
				`label_grouping_test{a="a", b="abb"} => 200 @[%v]`,
				`label_grouping_test{a="aa", b="bb"} => 100 @[%v]`,
			},
		},
		{
			expr: `{job=~"server", job!~"api"}`,
			output: []string{
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
			},
		},
		{
			// Test alternative "by"-clause order.
			expr: `sum by (group) (http_requests{job="api-server"})`,
			output: []string{
				`{group="canary"} => 700 @[%v]`,
				`{group="production"} => 300 @[%v]`,
			},
		},
		{
			// Test alternative "by"-clause order with "keeping_extra".
			expr: `sum by (group) keeping_extra (http_requests{job="api-server"})`,
			output: []string{
				`{group="canary", job="api-server"} => 700 @[%v]`,
				`{group="production", job="api-server"} => 300 @[%v]`,
			},
		},
		{
			// Test both alternative "by"-clause orders in one expression.
			// Public health warning: stick to one form within an expression (or even
			// in an organization), or risk serious user confusion.
			expr: `sum(sum by (group) keeping_extra (http_requests{job="api-server"})) by (job)`,
			output: []string{
				`{job="api-server"} => 1000 @[%v]`,
			},
		},
		{
			expr: `absent(nonexistent)`,
			output: []string{
				`{} => 1 @[%v]`,
			},
		},
		{
			expr: `absent(nonexistent{job="testjob", instance="testinstance", method=~".*"})`,
			output: []string{
				`{instance="testinstance", job="testjob"} => 1 @[%v]`,
			},
		},
		{
			expr: `count_scalar(absent(http_requests))`,
			output: []string{
				`scalar: 0 @[%v]`,
			},
		},
		{
			expr: `count_scalar(absent(sum(http_requests)))`,
			output: []string{
				`scalar: 0 @[%v]`,
			},
		},
		{
			expr: `absent(sum(nonexistent{job="testjob", instance="testinstance"}))`,
			output: []string{
				`{} => 1 @[%v]`,
			},
		},
		{
			expr: `http_requests{group="production",job="api-server"} offset 5m`,
			output: []string{
				`http_requests{group="production", instance="0", job="api-server"} => 90 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 180 @[%v]`,
			},
		},
		{
			expr: `rate(http_requests{group="production",job="api-server"}[10m] offset 5m)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => 0.03333333333333333 @[%v]`,
				`{group="production", instance="1", job="api-server"} => 0.06666666666666667 @[%v]`,
			},
		},
		{
			expr:       `rate(http_requests[10m]) offset 5m`,
			shouldFail: true,
		},
		{
			expr:       `sum(http_requests) offset 5m`,
			shouldFail: true,
		},
		// Regression test for missing separator byte in labelsToGroupingKey.
		{
			expr: `sum(label_grouping_test) by (a, b)`,
			output: []string{
				`{a="a", b="abb"} => 200 @[%v]`,
				`{a="aa", b="bb"} => 100 @[%v]`,
			},
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
			resultStr := ast.EvalToString(testExpr, testEvalTime, ast.Text, storage, stats.NewTimerGroup())
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

			if failed {
				t.Errorf("%d. Expression: %v\n%v", i, exprTest.expr, vectorComparisonString(expectedLines, resultLines))
			}
		}
	}
}

func TestRangedEvaluationRegressions(t *testing.T) {
	scenarios := []struct {
		in   ast.Matrix
		out  ast.Matrix
		expr string
	}{
		{
			// Testing COWMetric behavior in drop_common_labels.
			in: ast.Matrix{
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"testlabel":                 "1",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     1,
						},
						{
							Timestamp: testStartTime.Add(time.Hour),
							Value:     1,
						},
					},
				},
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"testlabel":                 "2",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime.Add(time.Hour),
							Value:     2,
						},
					},
				},
			},
			out: ast.Matrix{
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     1,
						},
					},
				},
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"testlabel":                 "1",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime.Add(time.Hour),
							Value:     1,
						},
					},
				},
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"testlabel":                 "2",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime.Add(time.Hour),
							Value:     2,
						},
					},
				},
			},
			expr: "drop_common_labels(testmetric)",
		},
		{
			// Testing COWMetric behavior in vector aggregation.
			in: ast.Matrix{
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"testlabel":                 "1",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     1,
						},
						{
							Timestamp: testStartTime.Add(time.Hour),
							Value:     1,
						},
					},
				},
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"testlabel":                 "2",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     2,
						},
					},
				},
			},
			out: ast.Matrix{
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     3,
						},
					},
				},
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							"testlabel": "1",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime.Add(time.Hour),
							Value:     1,
						},
					},
				},
			},
			expr: "sum(testmetric) keeping_extra",
		},
		{
			// Testing metric fingerprint grouping behavior.
			in: ast.Matrix{
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"aa": "bb",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     1,
						},
					},
				},
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"a": "abb",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     2,
						},
					},
				},
			},
			out: ast.Matrix{
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"aa": "bb",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     1,
						},
					},
				},
				{
					Metric: clientmodel.COWMetric{
						Metric: clientmodel.Metric{
							clientmodel.MetricNameLabel: "testmetric",
							"a": "abb",
						},
					},
					Values: metric.Values{
						{
							Timestamp: testStartTime,
							Value:     2,
						},
					},
				},
			},
			expr: "testmetric",
		},
	}

	for i, s := range scenarios {
		storage, closer := local.NewTestStorage(t)
		storeMatrix(storage, s.in)

		expr, err := LoadExprFromString(s.expr)
		if err != nil {
			t.Fatalf("%d. Error parsing expression: %v", i, err)
		}

		got, err := ast.EvalVectorRange(
			expr.(ast.VectorNode),
			testStartTime,
			testStartTime.Add(time.Hour),
			time.Hour,
			storage,
			stats.NewTimerGroup(),
		)
		if err != nil {
			t.Fatalf("%d. Error evaluating expression: %v", i, err)
		}

		if got.String() != s.out.String() {
			t.Fatalf("%d. Expression: %s\n\ngot:\n=====\n%v\n====\n\nwant:\n=====\n%v\n=====\n", i, s.expr, got.String(), s.out.String())
		}

		closer.Close()
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
