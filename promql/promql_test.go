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

package promql

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
)

var (
	testEvalTime = testStartTime.Add(testSampleInterval * 10)
	fixturesPath = "fixtures"

	reSample  = regexp.MustCompile(`^(.*)(?: \=\>|:) (\-?\d+\.?\d*(?:e-?\d+)?|[+-]Inf|NaN) \@\[(\d+)\]$`)
	minNormal = math.Float64frombits(0x0010000000000000) // The smallest positive normal value of type float64.
)

const (
	epsilon = 0.000001 // Relative error allowed for sample values.
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

// samplesAlmostEqual returns true if the two sample lines only differ by a
// small relative error in their sample value.
func samplesAlmostEqual(a, b string) bool {
	if a == b {
		// Fast path if strings are equal.
		return true
	}
	aMatches := reSample.FindStringSubmatch(a)
	if aMatches == nil {
		panic(fmt.Errorf("sample %q did not match regular expression", a))
	}
	bMatches := reSample.FindStringSubmatch(b)
	if bMatches == nil {
		panic(fmt.Errorf("sample %q did not match regular expression", b))
	}
	if aMatches[1] != bMatches[1] {
		return false // Labels don't match.
	}
	if aMatches[3] != bMatches[3] {
		return false // Timestamps don't match.
	}
	// If we are here, we have the diff in the floats.
	// We have to check if they are almost equal.
	aVal, err := strconv.ParseFloat(aMatches[2], 64)
	if err != nil {
		panic(err)
	}
	bVal, err := strconv.ParseFloat(bMatches[2], 64)
	if err != nil {
		panic(err)
	}

	// Cf. http://floating-point-gui.de/errors/comparison/
	if aVal == bVal {
		return true
	}

	diff := math.Abs(aVal - bVal)

	if aVal == 0 || bVal == 0 || diff < minNormal {
		return diff < epsilon*minNormal
	}
	return diff/(math.Abs(aVal)+math.Abs(bVal)) < epsilon
}

func newTestStorage(t testing.TB) (storage local.Storage, closer test.Closer) {
	storage, closer = local.NewTestStorage(t, 1)
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
				# Test comment.
				SUM(http_requests) BY # comments shouldn't have any effect
				(job) # another comment`,
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
		},
		{
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
			expr: `sort(0 / round(http_requests, 400) + http_requests)`,
			output: []string{
				`{group="production", instance="0", job="api-server"} => NaN @[%v]`,
				`{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
				`{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`{group="production", instance="1", job="app-server"} => 600 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
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
		},
		{
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
		},
		{
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
		},
		{
			// Deriv should return the same as rate in simple cases.
			expr:   `deriv(http_requests{group="canary", instance="1", job="app-server"}[60m])`,
			output: []string{`{group="canary", instance="1", job="app-server"} => 0.26666666666666666 @[%v]`},
		},
		{
			// Counter resets at in the middle of range are handled correctly by rate().
			expr:   `rate(testcounter_reset_middle[60m])`,
			output: []string{`{} => 0.03 @[%v]`},
		}, {
			// Counter resets at end of range are ignored by rate().
			expr:   `rate(testcounter_reset_end[5m])`,
			output: []string{`{} => 0 @[%v]`},
		},
		{
			// Deriv should return correct result.
			expr:   `deriv(testcounter_reset_middle[100m])`,
			output: []string{`{} => 0.010606060606060607 @[%v]`},
		},
		{
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
				`testhistogram_bucket{le="0.1", start="positive"} => 50 @[%v]`,
				`testhistogram_bucket{le=".2", start="positive"} => 70 @[%v]`,
				`testhistogram_bucket{le="1e0", start="positive"} => 110 @[%v]`,
				`testhistogram_bucket{le="+Inf", start="positive"} => 120 @[%v]`,
				`testhistogram_bucket{le="-.2", start="negative"} => 10 @[%v]`,
				`testhistogram_bucket{le="-0.1", start="negative"} => 20 @[%v]`,
				`testhistogram_bucket{le="0.3", start="negative"} => 20 @[%v]`,
				`testhistogram_bucket{le="+Inf", start="negative"} => 30 @[%v]`,
				`request_duration_seconds_bucket{instance="ins1", job="job1", le="0.1"} => 10 @[%v]`,
				`request_duration_seconds_bucket{instance="ins1", job="job1", le="0.2"} => 30 @[%v]`,
				`request_duration_seconds_bucket{instance="ins1", job="job1", le="+Inf"} => 40 @[%v]`,
				`request_duration_seconds_bucket{instance="ins2", job="job1", le="0.1"} => 20 @[%v]`,
				`request_duration_seconds_bucket{instance="ins2", job="job1", le="0.2"} => 50 @[%v]`,
				`request_duration_seconds_bucket{instance="ins2", job="job1", le="+Inf"} => 60 @[%v]`,
				`request_duration_seconds_bucket{instance="ins1", job="job2", le="0.1"} => 30 @[%v]`,
				`request_duration_seconds_bucket{instance="ins1", job="job2", le="0.2"} => 40 @[%v]`,
				`request_duration_seconds_bucket{instance="ins1", job="job2", le="+Inf"} => 60 @[%v]`,
				`request_duration_seconds_bucket{instance="ins2", job="job2", le="0.1"} => 40 @[%v]`,
				`request_duration_seconds_bucket{instance="ins2", job="job2", le="0.2"} => 70 @[%v]`,
				`request_duration_seconds_bucket{instance="ins2", job="job2", le="+Inf"} => 90 @[%v]`,
				`vector_matching_a{l="x"} => 10 @[%v]`,
				`vector_matching_a{l="y"} => 20 @[%v]`,
				`vector_matching_b{l="x"} => 40 @[%v]`,
				`cpu_count{instance="1", type="smp"} => 200 @[%v]`,
				`cpu_count{instance="0", type="smp"} => 100 @[%v]`,
				`cpu_count{instance="0", type="numa"} => 300 @[%v]`,
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
			expr: `http_requests{group="canary"} and http_requests{instance="0"}`,
			output: []string{
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
			},
		},
		{
			expr: `(http_requests{group="canary"} + 1) and http_requests{instance="0"}`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 301 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 701 @[%v]`,
			},
		},
		{
			expr: `(http_requests{group="canary"} + 1) and on(instance, job) http_requests{instance="0", group="production"}`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 301 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 701 @[%v]`,
			},
		},
		{
			expr: `(http_requests{group="canary"} + 1) and on(instance) http_requests{instance="0", group="production"}`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 301 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 701 @[%v]`,
			},
		},
		{
			expr: `http_requests{group="canary"} or http_requests{group="production"}`,
			output: []string{
				`http_requests{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`http_requests{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`http_requests{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
				`http_requests{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`http_requests{group="production", instance="0", job="api-server"} => 100 @[%v]`,
				`http_requests{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
			},
		},
		{
			// On overlap the rhs samples must be dropped.
			expr: `(http_requests{group="canary"} + 1) or http_requests{instance="1"}`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 301 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 701 @[%v]`,
				`{group="canary", instance="1", job="api-server"} => 401 @[%v]`,
				`{group="canary", instance="1", job="app-server"} => 801 @[%v]`,
				`http_requests{group="production", instance="1", job="api-server"} => 200 @[%v]`,
				`http_requests{group="production", instance="1", job="app-server"} => 600 @[%v]`,
			},
		},
		{
			// Matching only on instance excludes everything that has instance=0/1 but includes
			// entries without the instance label.
			expr: `(http_requests{group="canary"} + 1) or on(instance) (http_requests or cpu_count or vector_matching_a)`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => 301 @[%v]`,
				`{group="canary", instance="0", job="app-server"} => 701 @[%v]`,
				`{group="canary", instance="1", job="api-server"} => 401 @[%v]`,
				`{group="canary", instance="1", job="app-server"} => 801 @[%v]`,
				`vector_matching_a{l="x"} => 10 @[%v]`,
				`vector_matching_a{l="y"} => 20 @[%v]`,
			},
		},
		{
			expr: `http_requests{group="canary"} / on(instance,job) http_requests{group="production"}`,
			output: []string{
				`{instance="0", job="api-server"} => 3 @[%v]`,
				`{instance="0", job="app-server"} => 1.4 @[%v]`,
				`{instance="1", job="api-server"} => 2 @[%v]`,
				`{instance="1", job="app-server"} => 1.3333333333333333 @[%v]`,
			},
		},
		{
			// Include labels must guarantee uniquely identifiable time series.
			expr:       `http_requests{group="production"} / on(instance) group_left(group) cpu_count{type="smp"}`,
			shouldFail: true,
		},
		{
			// Many-to-many matching is not allowed.
			expr:       `http_requests{group="production"} / on(instance) group_left(job,type) cpu_count`,
			shouldFail: true,
		},
		{
			// Many-to-one matching must be explicit.
			expr:       `http_requests{group="production"} / on(instance) cpu_count{type="smp"}`,
			shouldFail: true,
		},
		{
			expr: `http_requests{group="production"} / on(instance) group_left(job) cpu_count{type="smp"}`,
			output: []string{
				`{instance="1", job="api-server"} => 1 @[%v]`,
				`{instance="0", job="app-server"} => 5 @[%v]`,
				`{instance="1", job="app-server"} => 3 @[%v]`,
				`{instance="0", job="api-server"} => 1 @[%v]`,
			},
		},
		{
			// Ensure sidedness of grouping preserves operand sides.
			expr: `cpu_count{type="smp"} / on(instance) group_right(job) http_requests{group="production"}`,
			output: []string{
				`{instance="1", job="app-server"} => 0.3333333333333333 @[%v]`,
				`{instance="0", job="app-server"} => 0.2 @[%v]`,
				`{instance="1", job="api-server"} => 1 @[%v]`,
				`{instance="0", job="api-server"} => 1 @[%v]`,
			},
		},
		{
			// Include labels from both sides.
			expr: `http_requests{group="production"} / on(instance) group_left(job) cpu_count{type="smp"}`,
			output: []string{
				`{instance="1", job="api-server"} => 1 @[%v]`,
				`{instance="0", job="app-server"} => 5 @[%v]`,
				`{instance="1", job="app-server"} => 3 @[%v]`,
				`{instance="0", job="api-server"} => 1 @[%v]`,
			},
		},
		{
			expr: `http_requests{group="production"} < on(instance,job) http_requests{group="canary"}`,
			output: []string{
				`{instance="1", job="app-server"} => 600 @[%v]`,
				`{instance="0", job="app-server"} => 500 @[%v]`,
				`{instance="1", job="api-server"} => 200 @[%v]`,
				`{instance="0", job="api-server"} => 100 @[%v]`,
			},
		},
		{
			expr:   `http_requests{group="production"} > on(instance,job) http_requests{group="canary"}`,
			output: []string{},
		},
		{
			expr:   `http_requests{group="production"} == on(instance,job) http_requests{group="canary"}`,
			output: []string{},
		},
		{
			expr: `http_requests > on(instance) group_left(group,job) cpu_count{type="smp"}`,
			output: []string{
				`{group="canary", instance="0", job="app-server"} => 700 @[%v]`,
				`{group="canary", instance="1", job="app-server"} => 800 @[%v]`,
				`{group="canary", instance="0", job="api-server"} => 300 @[%v]`,
				`{group="canary", instance="1", job="api-server"} => 400 @[%v]`,
				`{group="production", instance="0", job="app-server"} => 500 @[%v]`,
				`{group="production", instance="1", job="app-server"} => 600 @[%v]`,
			},
		},
		{
			expr:       `http_requests / on(instance) 3`,
			shouldFail: true,
		},
		{
			expr:       `3 / on(instance) http_requests_total`,
			shouldFail: true,
		},
		{
			expr:       `3 / on(instance) 3`,
			shouldFail: true,
		},
		{
			// Missing label list for grouping mod.
			expr:       `http_requests{group="production"} / on(instance) group_left cpu_count{type="smp"}`,
			shouldFail: true,
		},
		{
			// No group mod allowed for logical operations.
			expr:       `http_requests{group="production"} or on(instance) group_left(type) cpu_count{type="smp"}`,
			shouldFail: true,
		},
		{
			// No group mod allowed for logical operations.
			expr:       `http_requests{group="production"} and on(instance) group_left(type) cpu_count{type="smp"}`,
			shouldFail: true,
		},
		{
			// No duplicate use of label.
			expr:       `http_requests{group="production"} + on(instance) group_left(job,instance) cpu_count{type="smp"}`,
			shouldFail: true,
		},
		{
			expr: `{l="x"} + on(__name__) {l="y"}`,
			output: []string{
				`vector_matching_a => 30 @[%v]`,
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
		// Quantile too low.
		{
			expr: `histogram_quantile(-0.1, testhistogram_bucket)`,
			output: []string{
				`{start="positive"} => -Inf @[%v]`,
				`{start="negative"} => -Inf @[%v]`,
			},
		},
		// Quantile too high.
		{
			expr: `histogram_quantile(1.01, testhistogram_bucket)`,
			output: []string{
				`{start="positive"} => +Inf @[%v]`,
				`{start="negative"} => +Inf @[%v]`,
			},
		},
		// Quantile value in lowest bucket, which is positive.
		{
			expr: `histogram_quantile(0, testhistogram_bucket{start="positive"})`,
			output: []string{
				`{start="positive"} => 0 @[%v]`,
			},
		},
		// Quantile value in lowest bucket, which is negative.
		{
			expr: `histogram_quantile(0, testhistogram_bucket{start="negative"})`,
			output: []string{
				`{start="negative"} => -0.2 @[%v]`,
			},
		},
		// Quantile value in highest bucket.
		{
			expr: `histogram_quantile(1, testhistogram_bucket)`,
			output: []string{
				`{start="positive"} => 1 @[%v]`,
				`{start="negative"} => 0.3 @[%v]`,
			},
		},
		// Finally some useful quantiles.
		{
			expr: `histogram_quantile(0.2, testhistogram_bucket)`,
			output: []string{
				`{start="positive"} => 0.048 @[%v]`,
				`{start="negative"} => -0.2 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, testhistogram_bucket)`,
			output: []string{
				`{start="positive"} => 0.15 @[%v]`,
				`{start="negative"} => -0.15 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.8, testhistogram_bucket)`,
			output: []string{
				`{start="positive"} => 0.72 @[%v]`,
				`{start="negative"} => 0.3 @[%v]`,
			},
		},
		// More realistic with rates.
		{
			expr: `histogram_quantile(0.2, rate(testhistogram_bucket[5m]))`,
			output: []string{
				`{start="positive"} => 0.048 @[%v]`,
				`{start="negative"} => -0.2 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, rate(testhistogram_bucket[5m]))`,
			output: []string{
				`{start="positive"} => 0.15 @[%v]`,
				`{start="negative"} => -0.15 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.8, rate(testhistogram_bucket[5m]))`,
			output: []string{
				`{start="positive"} => 0.72 @[%v]`,
				`{start="negative"} => 0.3 @[%v]`,
			},
		},
		// Aggregated histogram: Everything in one.
		{
			expr: `histogram_quantile(0.3, sum(rate(request_duration_seconds_bucket[5m])) by (le))`,
			output: []string{
				`{} => 0.075 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, sum(rate(request_duration_seconds_bucket[5m])) by (le))`,
			output: []string{
				`{} => 0.1277777777777778 @[%v]`,
			},
		},
		// Aggregated histogram: Everything in one. Now with avg, which does not change anything.
		{
			expr: `histogram_quantile(0.3, avg(rate(request_duration_seconds_bucket[5m])) by (le))`,
			output: []string{
				`{} => 0.075 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, avg(rate(request_duration_seconds_bucket[5m])) by (le))`,
			output: []string{
				`{} => 0.12777777777777778 @[%v]`,
			},
		},
		// Aggregated histogram: By job.
		{
			expr: `histogram_quantile(0.3, sum(rate(request_duration_seconds_bucket[5m])) by (le, instance))`,
			output: []string{
				`{instance="ins1"} => 0.075 @[%v]`,
				`{instance="ins2"} => 0.075 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, sum(rate(request_duration_seconds_bucket[5m])) by (le, instance))`,
			output: []string{
				`{instance="ins1"} => 0.1333333333 @[%v]`,
				`{instance="ins2"} => 0.125 @[%v]`,
			},
		},
		// Aggregated histogram: By instance.
		{
			expr: `histogram_quantile(0.3, sum(rate(request_duration_seconds_bucket[5m])) by (le, job))`,
			output: []string{
				`{job="job1"} => 0.1 @[%v]`,
				`{job="job2"} => 0.0642857142857143 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, sum(rate(request_duration_seconds_bucket[5m])) by (le, job))`,
			output: []string{
				`{job="job1"} => 0.14 @[%v]`,
				`{job="job2"} => 0.1125 @[%v]`,
			},
		},
		// Aggregated histogram: By job and instance.
		{
			expr: `histogram_quantile(0.3, sum(rate(request_duration_seconds_bucket[5m])) by (le, job, instance))`,
			output: []string{
				`{instance="ins1", job="job1"} => 0.11 @[%v]`,
				`{instance="ins2", job="job1"} => 0.09 @[%v]`,
				`{instance="ins1", job="job2"} => 0.06 @[%v]`,
				`{instance="ins2", job="job2"} => 0.0675 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, sum(rate(request_duration_seconds_bucket[5m])) by (le, job, instance))`,
			output: []string{
				`{instance="ins1", job="job1"} => 0.15 @[%v]`,
				`{instance="ins2", job="job1"} => 0.1333333333333333 @[%v]`,
				`{instance="ins1", job="job2"} => 0.1 @[%v]`,
				`{instance="ins2", job="job2"} => 0.1166666666666667 @[%v]`,
			},
		},
		// The unaggregated histogram for comparison. Same result as the previous one.
		{
			expr: `histogram_quantile(0.3, rate(request_duration_seconds_bucket[5m]))`,
			output: []string{
				`{instance="ins1", job="job1"} => 0.11 @[%v]`,
				`{instance="ins2", job="job1"} => 0.09 @[%v]`,
				`{instance="ins1", job="job2"} => 0.06 @[%v]`,
				`{instance="ins2", job="job2"} => 0.0675 @[%v]`,
			},
		},
		{
			expr: `histogram_quantile(0.5, rate(request_duration_seconds_bucket[5m]))`,
			output: []string{
				`{instance="ins1", job="job1"} => 0.15 @[%v]`,
				`{instance="ins2", job="job1"} => 0.13333333333333333 @[%v]`,
				`{instance="ins1", job="job2"} => 0.1 @[%v]`,
				`{instance="ins2", job="job2"} => 0.11666666666666667 @[%v]`,
			},
		},
		{
			expr:   `12.34e6`,
			output: []string{`scalar: 12340000 @[%v]`},
		},
		{
			expr:   `12.34e+6`,
			output: []string{`scalar: 12340000 @[%v]`},
		},
		{
			expr:   `12.34e-6`,
			output: []string{`scalar: 0.00001234 @[%v]`},
		},
		{
			expr:   `1+1`,
			output: []string{`scalar: 2 @[%v]`},
		},
		{
			expr:   `1-1`,
			output: []string{`scalar: 0 @[%v]`},
		},
		{
			expr:   `1 - -1`,
			output: []string{`scalar: 2 @[%v]`},
		},
		{
			expr:   `.2`,
			output: []string{`scalar: 0.2 @[%v]`},
		},
		{
			expr:   `+0.2`,
			output: []string{`scalar: 0.2 @[%v]`},
		},
		{
			expr:   `-0.2e-6`,
			output: []string{`scalar: -0.0000002 @[%v]`},
		},
		{
			expr:   `+Inf`,
			output: []string{`scalar: +Inf @[%v]`},
		},
		{
			expr:   `inF`,
			output: []string{`scalar: +Inf @[%v]`},
		},
		{
			expr:   `-inf`,
			output: []string{`scalar: -Inf @[%v]`},
		},
		{
			expr:   `NaN`,
			output: []string{`scalar: NaN @[%v]`},
		},
		{
			expr:   `nan`,
			output: []string{`scalar: NaN @[%v]`},
		},
		{
			expr:   `2.`,
			output: []string{`scalar: 2 @[%v]`},
		},
		{
			expr:       `999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999`,
			shouldFail: true,
		},
		{
			expr:   `1 / 0`,
			output: []string{`scalar: +Inf @[%v]`},
		},
		{
			expr:   `-1 / 0`,
			output: []string{`scalar: -Inf @[%v]`},
		},
		{
			expr:   `0 / 0`,
			output: []string{`scalar: NaN @[%v]`},
		},
		{
			expr:   `1 % 0`,
			output: []string{`scalar: NaN @[%v]`},
		},
		{
			expr: `http_requests{group="canary", instance="0", job="api-server"} / 0`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => +Inf @[%v]`,
			},
		},
		{
			expr: `-1 * http_requests{group="canary", instance="0", job="api-server"} / 0`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => -Inf @[%v]`,
			},
		},
		{
			expr: `0 * http_requests{group="canary", instance="0", job="api-server"} / 0`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => NaN @[%v]`,
			},
		},
		{
			expr: `0 * http_requests{group="canary", instance="0", job="api-server"} % 0`,
			output: []string{
				`{group="canary", instance="0", job="api-server"} => NaN @[%v]`,
			},
		},
		{
			expr: `exp(vector_matching_a)`,
			output: []string{
				`{l="x"} => 22026.465794806718 @[%v]`,
				`{l="y"} => 485165195.4097903 @[%v]`,
			},
		},
		{
			expr: `exp(vector_matching_a - 10)`,
			output: []string{
				`{l="y"} => 22026.465794806718 @[%v]`,
				`{l="x"} => 1 @[%v]`,
			},
		},
		{
			expr: `exp(vector_matching_a - 20)`,
			output: []string{
				`{l="x"} => 4.5399929762484854e-05 @[%v]`,
				`{l="y"} => 1 @[%v]`,
			},
		},
		{
			expr: `ln(vector_matching_a)`,
			output: []string{
				`{l="x"} => 2.302585092994046 @[%v]`,
				`{l="y"} => 2.995732273553991 @[%v]`,
			},
		},
		{
			expr: `ln(vector_matching_a - 10)`,
			output: []string{
				`{l="y"} => 2.302585092994046 @[%v]`,
				`{l="x"} => -Inf @[%v]`,
			},
		},
		{
			expr: `ln(vector_matching_a - 20)`,
			output: []string{
				`{l="y"} => -Inf @[%v]`,
				`{l="x"} => NaN @[%v]`,
			},
		},
		{
			expr: `exp(ln(vector_matching_a))`,
			output: []string{
				`{l="y"} => 20 @[%v]`,
				`{l="x"} => 10 @[%v]`,
			},
		},
		{
			expr: `sqrt(vector_matching_a)`,
			output: []string{
				`{l="x"} => 3.1622776601683795 @[%v]`,
				`{l="y"} => 4.47213595499958 @[%v]`,
			},
		},
		{
			expr: `log2(vector_matching_a)`,
			output: []string{
				`{l="x"} => 3.3219280948873626 @[%v]`,
				`{l="y"} => 4.321928094887363 @[%v]`,
			},
		},
		{
			expr: `log2(vector_matching_a - 10)`,
			output: []string{
				`{l="y"} => 3.3219280948873626 @[%v]`,
				`{l="x"} => -Inf @[%v]`,
			},
		},
		{
			expr: `log2(vector_matching_a - 20)`,
			output: []string{
				`{l="x"} => NaN @[%v]`,
				`{l="y"} => -Inf @[%v]`,
			},
		},
		{
			expr: `log10(vector_matching_a)`,
			output: []string{
				`{l="x"} => 1 @[%v]`,
				`{l="y"} => 1.301029995663981 @[%v]`,
			},
		},
		{
			expr: `log10(vector_matching_a - 10)`,
			output: []string{
				`{l="y"} => 1 @[%v]`,
				`{l="x"} => -Inf @[%v]`,
			},
		},
		{
			expr: `log10(vector_matching_a - 20)`,
			output: []string{
				`{l="x"} => NaN @[%v]`,
				`{l="y"} => -Inf @[%v]`,
			},
		},
		{
			expr: `stddev(http_requests)`,
			output: []string{
				`{} => 229.12878474779 @[%v]`,
			},
		},
		{
			expr: `stddev by (instance)(http_requests)`,
			output: []string{
				`{instance="0"} => 223.60679774998 @[%v]`,
				`{instance="1"} => 223.60679774998 @[%v]`,
			},
		},
		{
			expr: `stdvar(http_requests)`,
			output: []string{
				`{} => 52500 @[%v]`,
			},
		},
		{
			expr: `stdvar by (instance)(http_requests)`,
			output: []string{
				`{instance="0"} => 50000 @[%v]`,
				`{instance="1"} => 50000 @[%v]`,
			},
		},
	}

	storage, closer := newTestStorage(t)
	defer closer.Close()

	engine := NewEngine(storage)

	for i, exprTest := range expressionTests {
		expectedLines := annotateWithTime(exprTest.output, testEvalTime)

		query, err := engine.NewInstantQuery(exprTest.expr, testEvalTime)

		if err != nil {
			if !exprTest.shouldFail {
				t.Errorf("%d. Error during parsing: %v", i, err)
				t.Errorf("%d. Expression: %v", i, exprTest.expr)
			}
			continue
		}

		failed := false

		res := query.Exec()
		if res.Err != nil {
			if !exprTest.shouldFail {
				t.Errorf("%d. Error evaluating query: %s", res.Err)
				t.Errorf("%d. Expression: %v", i, exprTest.expr)
			}
			continue
		}
		if exprTest.shouldFail {
			t.Errorf("%d. Expression should fail but did not", i)
			continue
		}
		resultLines := strings.Split(res.String(), "\n")
		// resultStr := ast.EvalToString(testExpr, testEvalTime, ast.Text, storage, stats.NewTimerGroup())
		// resultLines := strings.Split(resultStr, "\n")

		if len(exprTest.output) == 0 && strings.Trim(res.String(), "\n") == "" {
			// expected and received empty vector, everything is fine
			continue
		} else if len(exprTest.output) != len(resultLines) {
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
					if samplesAlmostEqual(actualSample, expectedSample) {
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

func TestRangedEvaluationRegressions(t *testing.T) {
	scenarios := []struct {
		in   Matrix
		out  Matrix
		expr string
	}{
		{
			// Testing COWMetric behavior in drop_common_labels.
			in: Matrix{
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
			out: Matrix{
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
			in: Matrix{
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
			out: Matrix{
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
			in: Matrix{
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
			out: Matrix{
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
		storage, closer := local.NewTestStorage(t, 1)
		storeMatrix(storage, s.in)

		engine := NewEngine(storage)
		query, err := engine.NewRangeQuery(s.expr, testStartTime, testStartTime.Add(time.Hour), time.Hour)
		if err != nil {
			t.Errorf("%d. Error in expression %q", i, s.expr)
			t.Fatalf("%d. Error parsing expression: %v", i, err)
		}
		res := query.Exec()
		if res.Err != nil {
			t.Errorf("%d. Error in expression %q", i, s.expr)
			t.Fatalf("%d. Error evaluating expression: %v", i, err)
		}

		if res.String() != s.out.String() {
			t.Errorf("%d. Error in expression %q", i, s.expr)
			t.Fatalf("%d. Expression: %s\n\ngot:\n=====\n%v\n====\n\nwant:\n=====\n%v\n=====\n", i, s.expr, res.String(), s.out.String())
		}

		closer.Close()
	}
}
