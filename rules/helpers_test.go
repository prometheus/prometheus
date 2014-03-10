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
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules/ast"
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
	for _, sampleSet := range matrix {
		lastSample := sampleSet.Values[len(sampleSet.Values)-1]
		vector = append(vector, &clientmodel.Sample{
			Metric:    sampleSet.Metric,
			Value:     lastSample.Value,
			Timestamp: lastSample.Timestamp,
		})
	}
	return vector
}

func storeMatrix(storage metric.TieredStorage, matrix ast.Matrix) (err error) {
	pendingSamples := clientmodel.Samples{}
	for _, sampleSet := range matrix {
		for _, sample := range sampleSet.Values {
			pendingSamples = append(pendingSamples, &clientmodel.Sample{
				Metric:    sampleSet.Metric,
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			})
		}
	}
	err = storage.AppendSamples(pendingSamples)
	return
}

var testMatrix = ast.Matrix{
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "api-server",
			"instance":                  "0",
			"group":                     "production",
		},
		Values: getTestValueStream(0, 100, 10, testStartTime),
	},
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "api-server",
			"instance":                  "1",
			"group":                     "production",
		},
		Values: getTestValueStream(0, 200, 20, testStartTime),
	},
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "api-server",
			"instance":                  "0",
			"group":                     "canary",
		},
		Values: getTestValueStream(0, 300, 30, testStartTime),
	},
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "api-server",
			"instance":                  "1",
			"group":                     "canary",
		},
		Values: getTestValueStream(0, 400, 40, testStartTime),
	},
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "app-server",
			"instance":                  "0",
			"group":                     "production",
		},
		Values: getTestValueStream(0, 500, 50, testStartTime),
	},
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "app-server",
			"instance":                  "1",
			"group":                     "production",
		},
		Values: getTestValueStream(0, 600, 60, testStartTime),
	},
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "app-server",
			"instance":                  "0",
			"group":                     "canary",
		},
		Values: getTestValueStream(0, 700, 70, testStartTime),
	},
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "http_requests",
			clientmodel.JobLabel:        "app-server",
			"instance":                  "1",
			"group":                     "canary",
		},
		Values: getTestValueStream(0, 800, 80, testStartTime),
	},
	// Single-letter metric and label names.
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "x",
			"y": "testvalue",
		},
		Values: getTestValueStream(0, 100, 10, testStartTime),
	},
	// Counter reset in the middle of range.
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "testcounter_reset_middle",
		},
		Values: append(getTestValueStream(0, 40, 10, testStartTime), getTestValueStream(0, 50, 10, testStartTime.Add(testSampleInterval*5))...),
	},
	// Counter reset at the end of range.
	{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "testcounter_reset_end",
		},
		Values: append(getTestValueStream(0, 90, 10, testStartTime), getTestValueStream(0, 0, 10, testStartTime.Add(testSampleInterval*10))...),
	},
}

var testVector = getTestVectorFromTestMatrix(testMatrix)
