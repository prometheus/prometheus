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
	"github.com/matttproud/prometheus/model"
	"io"
	"io/ioutil"
	"time"
)

const (
	baseLabels001 = "baseLabels"
	counter001    = "counter"
	docstring001  = "docstring"
	gauge001      = "gauge"
	histogram001  = "histogram"
	labels001     = "labels"
	metric001     = "metric"
	type001       = "type"
	value001      = "value"
	percentile001 = "percentile"
)

var (
	Processor001 Processor = &processor001{}
)

// processor001 is responsible for handling API version 0.0.1.
type processor001 struct {
}

// entity001 represents a the JSON structure that 0.0.1 uses.
type entity001 []struct {
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

func (p *processor001) Process(stream io.ReadCloser, results chan Result) (err error) {
	// TODO(matt): Replace with plain-jane JSON unmarshalling.
	defer stream.Close()

	buffer, err := ioutil.ReadAll(stream)
	if err != nil {
		return
	}

	entities := entity001{}

	err = json.Unmarshal(buffer, &entities)
	if err != nil {
		return
	}

	// Swap this to the testable timer.
	now := time.Now()

	// TODO(matt): This outer loop is a great basis for parallelization.
	for _, entity := range entities {
		for _, value := range entity.Metric.Value {
			metric := model.Metric{}
			for label, labelValue := range entity.BaseLabels {
				metric[model.LabelName(label)] = model.LabelValue(labelValue)
			}

			for label, labelValue := range value.Labels {
				metric[model.LabelName(label)] = model.LabelValue(labelValue)
			}

			switch entity.Metric.MetricType {
			case gauge001, counter001:
				sampleValue, ok := value.Value.(float64)
				if !ok {
					err = fmt.Errorf("Could not convert value from %s %s to float64.", entity, value)
					continue
				}

				sample := model.Sample{
					Metric:    metric,
					Timestamp: now,
					Value:     model.SampleValue(sampleValue),
				}

				results <- Result{
					Err:    err,
					Sample: sample,
				}

				break

			case histogram001:
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
					metric[model.LabelName(percentile001)] = model.LabelValue(percentile)
					sample := model.Sample{
						Metric:    metric,
						Timestamp: now,
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
