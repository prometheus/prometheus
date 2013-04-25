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

package format

import (
	"encoding/json"
	"fmt"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility"
	"io"
	"io/ioutil"
	"time"
)

const (
	baseLabels002 = "baseLabels"
	counter002    = "counter"
	docstring002  = "docstring"
	gauge002      = "gauge"
	histogram002  = "histogram"
	labels002     = "labels"
	metric002     = "metric"
	type002       = "type"
	value002      = "value"
	percentile002 = "percentile"
)

var (
	Processor002 Processor = &processor002{}
)

// processor002 is responsible for handling API version 0.0.2.
type processor002 struct {
	time utility.Time
}

// entity002 represents a the JSON structure that 0.0.2 uses.
type entity002 []struct {
	BaseLabels map[string]string `json:"baseLabels"`
	Docstring  string            `json:"docstring"`
	Metric     struct {
		MetricType string `json:"type"`
		Value      []struct {
			Labels map[string]string `json:"labels"`
			Value  interface{}       `json:"value"`
		} `json:"value"`
	} `json:"metric"`
}

func (p *processor002) Process(stream io.ReadCloser, timestamp time.Time, baseLabels model.LabelSet, results chan Result) (err error) {
	// TODO(matt): Replace with plain-jane JSON unmarshalling.
	defer stream.Close()

	buffer, err := ioutil.ReadAll(stream)
	if err != nil {
		return
	}

	entities := entity002{}

	err = json.Unmarshal(buffer, &entities)
	if err != nil {
		return
	}

	// TODO(matt): This outer loop is a great basis for parallelization.
	for _, entity := range entities {
		for _, value := range entity.Metric.Value {
			metric := model.Metric{}
			for label, labelValue := range baseLabels {
				metric[label] = labelValue
			}

			for label, labelValue := range entity.BaseLabels {
				metric[model.LabelName(label)] = model.LabelValue(labelValue)
			}

			for label, labelValue := range value.Labels {
				metric[model.LabelName(label)] = model.LabelValue(labelValue)
			}

			switch entity.Metric.MetricType {
			case gauge002, counter002:
				sampleValue, ok := value.Value.(float64)
				if !ok {
					err = fmt.Errorf("Could not convert value from %s %s to float64.", entity, value)
					continue
				}

				sample := model.Sample{
					Metric:    metric,
					Timestamp: timestamp,
					Value:     model.SampleValue(sampleValue),
				}

				results <- Result{
					Err:    err,
					Sample: sample,
				}

				break

			case histogram002:
				sampleValue, ok := value.Value.(map[string]interface{})
				if !ok {
					err = fmt.Errorf("Could not convert value from %q to a map[string]interface{}.", value.Value)
					continue
				}

				for percentile, percentileValue := range sampleValue {
					individualValue, ok := percentileValue.(float64)
					if !ok {
						err = fmt.Errorf("Could not convert value from %q to a float64.", percentileValue)
						continue
					}

					childMetric := make(map[model.LabelName]model.LabelValue, len(metric)+1)

					for k, v := range metric {
						childMetric[k] = v
					}

					childMetric[model.LabelName(percentile002)] = model.LabelValue(percentile)

					sample := model.Sample{
						Metric:    childMetric,
						Timestamp: timestamp,
						Value:     model.SampleValue(individualValue),
					}

					results <- Result{
						Err:    err,
						Sample: sample,
					}
				}

				break
			default:
			}
		}
	}

	return
}
