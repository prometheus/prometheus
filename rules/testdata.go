package rules

import (
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/metric"
	"time"
)

var testDuration5m = time.Duration(5) * time.Minute
var testStartTime = time.Time{}

func getTestValueStream(startVal model.SampleValue,
	endVal model.SampleValue,
	stepVal model.SampleValue) (resultValues []model.SamplePair) {
	currentTime := testStartTime
	for currentVal := startVal; currentVal <= endVal; currentVal += stepVal {
		sample := model.SamplePair{
			Value:     currentVal,
			Timestamp: currentTime,
		}
		resultValues = append(resultValues, sample)
		currentTime = currentTime.Add(testDuration5m)
	}
	return resultValues
}

func getTestVectorFromTestMatrix(matrix ast.Matrix) ast.Vector {
	vector := ast.Vector{}
	for _, sampleSet := range matrix {
		lastSample := sampleSet.Values[len(sampleSet.Values)-1]
		vector = append(vector, &model.Sample{
			Metric:    sampleSet.Metric,
			Value:     lastSample.Value,
			Timestamp: lastSample.Timestamp,
		})
	}
	return vector
}

func storeMatrix(persistence metric.MetricPersistence, matrix ast.Matrix) error {
	for _, sampleSet := range matrix {
		for _, sample := range sampleSet.Values {
			err := persistence.AppendSample(&model.Sample{
				Metric:    sampleSet.Metric,
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

var testMatrix = ast.Matrix{
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "api-server",
			"instance": "0",
			"group":    "production",
		},
		Values: getTestValueStream(0, 100, 10),
	},
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "api-server",
			"instance": "1",
			"group":    "production",
		},
		Values: getTestValueStream(0, 200, 20),
	},
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "api-server",
			"instance": "0",
			"group":    "canary",
		},
		Values: getTestValueStream(0, 300, 30),
	},
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "api-server",
			"instance": "1",
			"group":    "canary",
		},
		Values: getTestValueStream(0, 400, 40),
	},
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "app-server",
			"instance": "0",
			"group":    "production",
		},
		Values: getTestValueStream(0, 500, 50),
	},
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "app-server",
			"instance": "1",
			"group":    "production",
		},
		Values: getTestValueStream(0, 600, 60),
	},
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "app-server",
			"instance": "0",
			"group":    "canary",
		},
		Values: getTestValueStream(0, 700, 70),
	},
	{
		Metric: model.Metric{
			"name":     "http_requests",
			"job":      "app-server",
			"instance": "1",
			"group":    "canary",
		},
		Values: getTestValueStream(0, 800, 80),
	},
}

var testVector = getTestVectorFromTestMatrix(testMatrix)
