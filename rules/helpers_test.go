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
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
)

var testSampleInterval = time.Duration(5) * time.Minute
var testStartTime = clientmodel.Timestamp(0)

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

func getTestVectorFromTestMatrix(matrix ast.Matrix) ast.Vector {
	vector := ast.Vector{}
	for _, sampleStream := range matrix {
		lastSample := sampleStream.Values[len(sampleStream.Values)-1]
		vector = append(vector, &ast.Sample{
			Metric:    sampleStream.Metric,
			Value:     lastSample.Value,
			Timestamp: lastSample.Timestamp,
		})
	}
	return vector
}

func storeMatrix(storage local.Storage, matrix ast.Matrix) {
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

var testMatrix = ast.Matrix{
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "0",
				"group":                     "production",
			},
		},
		Values: getTestValueStream(0, 100, 10, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "api-server",
				"instance":                  "1",
				"group":                     "production",
			},
		},
		Values: getTestValueStream(0, 200, 20, testStartTime),
	},
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
				"group":                     "production",
			},
		},
		Values: getTestValueStream(0, 500, 50, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "http_requests",
				clientmodel.JobLabel:        "app-server",
				"instance":                  "1",
				"group":                     "production",
			},
		},
		Values: getTestValueStream(0, 600, 60, testStartTime),
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
	// Single-letter metric and label names.
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "x",
				"y": "testvalue",
			},
		},
		Values: getTestValueStream(0, 100, 10, testStartTime),
	},
	// Counter reset in the middle of range.
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testcounter_reset_middle",
			},
		},
		Values: append(getTestValueStream(0, 40, 10, testStartTime), getTestValueStream(0, 50, 10, testStartTime.Add(testSampleInterval*5))...),
	},
	// Counter reset at the end of range.
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testcounter_reset_end",
			},
		},
		Values: append(getTestValueStream(0, 90, 10, testStartTime), getTestValueStream(0, 0, 10, testStartTime.Add(testSampleInterval*10))...),
	},
	// For label-key grouping regression test.
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "label_grouping_test",
				"a": "aa",
				"b": "bb",
			},
		},
		Values: getTestValueStream(0, 100, 10, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "label_grouping_test",
				"a": "a",
				"b": "abb",
			},
		},
		Values: getTestValueStream(0, 200, 20, testStartTime),
	},
	// Two histograms with 4 buckets each (*_sum and *_count not included,
	// only buckets). Lowest bucket for one histogram < 0, for the other >
	// 0. They have the same name, just separated by label. Not useful in
	// practice, but can happen (if clients change bucketing), and the
	// server has to cope with it.
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    "0.1",
				"start": "positive",
			},
		},
		Values: getTestValueStream(0, 50, 5, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    ".2",
				"start": "positive",
			},
		},
		Values: getTestValueStream(0, 70, 7, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    "1e0",
				"start": "positive",
			},
		},
		Values: getTestValueStream(0, 110, 11, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    "+Inf",
				"start": "positive",
			},
		},
		Values: getTestValueStream(0, 120, 12, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    "-.2",
				"start": "negative",
			},
		},
		Values: getTestValueStream(0, 10, 1, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    "-0.1",
				"start": "negative",
			},
		},
		Values: getTestValueStream(0, 20, 2, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    "0.3",
				"start": "negative",
			},
		},
		Values: getTestValueStream(0, 20, 2, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "testhistogram_bucket",
				"le":    "+Inf",
				"start": "negative",
			},
		},
		Values: getTestValueStream(0, 30, 3, testStartTime),
	},
	// Now a more realistic histogram per job and instance to test aggregation.
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job1",
				"instance":                  "ins1",
				"le":                        "0.1",
			},
		},
		Values: getTestValueStream(0, 10, 1, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job1",
				"instance":                  "ins1",
				"le":                        "0.2",
			},
		},
		Values: getTestValueStream(0, 30, 3, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job1",
				"instance":                  "ins1",
				"le":                        "+Inf",
			},
		},
		Values: getTestValueStream(0, 40, 4, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job1",
				"instance":                  "ins2",
				"le":                        "0.1",
			},
		},
		Values: getTestValueStream(0, 20, 2, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job1",
				"instance":                  "ins2",
				"le":                        "0.2",
			},
		},
		Values: getTestValueStream(0, 50, 5, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job1",
				"instance":                  "ins2",
				"le":                        "+Inf",
			},
		},
		Values: getTestValueStream(0, 60, 6, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job2",
				"instance":                  "ins1",
				"le":                        "0.1",
			},
		},
		Values: getTestValueStream(0, 30, 3, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job2",
				"instance":                  "ins1",
				"le":                        "0.2",
			},
		},
		Values: getTestValueStream(0, 40, 4, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job2",
				"instance":                  "ins1",
				"le":                        "+Inf",
			},
		},
		Values: getTestValueStream(0, 60, 6, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job2",
				"instance":                  "ins2",
				"le":                        "0.1",
			},
		},
		Values: getTestValueStream(0, 40, 4, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job2",
				"instance":                  "ins2",
				"le":                        "0.2",
			},
		},
		Values: getTestValueStream(0, 70, 7, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "request_duration_seconds_bucket",
				clientmodel.JobLabel:        "job2",
				"instance":                  "ins2",
				"le":                        "+Inf",
			},
		},
		Values: getTestValueStream(0, 90, 9, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "vector_matching_a",
				"l": "x",
			},
		},
		Values: getTestValueStream(0, 100, 1, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "vector_matching_a",
				"l": "y",
			},
		},
		Values: getTestValueStream(0, 100, 2, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "vector_matching_b",
				"l": "x",
			},
		},
		Values: getTestValueStream(0, 100, 4, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "cpu_count",
				"instance":                  "0",
				"type":                      "numa",
			},
		},
		Values: getTestValueStream(0, 500, 30, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "cpu_count",
				"instance":                  "0",
				"type":                      "smp",
			},
		},
		Values: getTestValueStream(0, 200, 10, testStartTime),
	},
	{
		Metric: clientmodel.COWMetric{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "cpu_count",
				"instance":                  "1",
				"type":                      "smp",
			},
		},
		Values: getTestValueStream(0, 200, 20, testStartTime),
	},
}

var testVector = getTestVectorFromTestMatrix(testMatrix)
