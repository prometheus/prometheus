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

	reSample = regexp.MustCompile(`^(.*)(?: \=\>|:) (\-?\d+\.?\d*(?:e-?\d+)?|[+-]Inf|NaN) \@\[(\d+)\]$`)
	// minNormal = math.Float64frombits(0x0010000000000000) // The smallest positive normal value of type float64.
)

// const (
// 	epsilon = 0.000001 // Relative error allowed for sample values.
// )

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
