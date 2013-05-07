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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/metric"
	"time"
)

var testSampleInterval = time.Duration(5) * time.Minute
var testStartTime = time.Time{}

func getTestValueStream(startVal model.SampleValue,
	endVal model.SampleValue,
	stepVal model.SampleValue) (resultValues model.Values) {
	currentTime := testStartTime
	for currentVal := startVal; currentVal <= endVal; currentVal += stepVal {
		sample := model.SamplePair{
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
		vector = append(vector, model.Sample{
			Metric:    sampleSet.Metric,
			Value:     lastSample.Value,
			Timestamp: lastSample.Timestamp,
		})
	}
	return vector
}

func storeMatrix(storage metric.TieredStorage, matrix ast.Matrix) (err error) {
	pendingSamples := model.Samples{}
	for _, sampleSet := range matrix {
		for _, sample := range sampleSet.Values {
			pendingSamples = append(pendingSamples, model.Sample{
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
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "api-server",
			"instance":            "0",
			"group":               "production",
		},
		Values: getTestValueStream(0, 100, 10),
	},
	{
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "api-server",
			"instance":            "1",
			"group":               "production",
		},
		Values: getTestValueStream(0, 200, 20),
	},
	{
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "api-server",
			"instance":            "0",
			"group":               "canary",
		},
		Values: getTestValueStream(0, 300, 30),
	},
	{
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "api-server",
			"instance":            "1",
			"group":               "canary",
		},
		Values: getTestValueStream(0, 400, 40),
	},
	{
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "app-server",
			"instance":            "0",
			"group":               "production",
		},
		Values: getTestValueStream(0, 500, 50),
	},
	{
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "app-server",
			"instance":            "1",
			"group":               "production",
		},
		Values: getTestValueStream(0, 600, 60),
	},
	{
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "app-server",
			"instance":            "0",
			"group":               "canary",
		},
		Values: getTestValueStream(0, 700, 70),
	},
	{
		Metric: model.Metric{
			model.MetricNameLabel: "http_requests",
			model.JobLabel:        "app-server",
			"instance":            "1",
			"group":               "canary",
		},
		Values: getTestValueStream(0, 800, 80),
	},
	// Single-letter metric and label names.
	{
		Metric: model.Metric{
			model.MetricNameLabel: "x",
			"y": "testvalue",
		},
		Values: getTestValueStream(0, 100, 10),
	},
}

var testVector = getTestVectorFromTestMatrix(testMatrix)
