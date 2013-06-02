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
	time utility.Time
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

func (p *processor001) Process(stream io.ReadCloser, timestamp time.Time, baseLabels model.LabelSet, results chan Result) error {
	// TODO(matt): Replace with plain-jane JSON unmarshalling.
	defer stream.Close()

	buffer, err := ioutil.ReadAll(stream)
	if err != nil {
		return err
	}

	entities := entity001{}

	if err = json.Unmarshal(buffer, &entities); err != nil {
		return err
	}

	// TODO(matt): This outer loop is a great basis for parallelization.
	pendingSamples := model.Samples{}
	for _, entity := range entities {
		for _, value := range entity.Metric.Value {
			entityLabels := LabelSet(entity.BaseLabels).Merge(LabelSet(value.Labels))
			labels := mergeTargetLabels(entityLabels, baseLabels)

			switch entity.Metric.MetricType {
			case gauge001, counter001:
				sampleValue, ok := value.Value.(float64)
				if !ok {
					err = fmt.Errorf("Could not convert value from %s %s to float64.", entity, value)
					results <- Result{Err: err}
					continue
				}

				pendingSamples = append(pendingSamples, model.Sample{
					Metric:    model.Metric(labels),
					Timestamp: timestamp,
					Value:     model.SampleValue(sampleValue),
				})

				break

			case histogram001:
				sampleValue, ok := value.Value.(map[string]interface{})
				if !ok {
					err = fmt.Errorf("Could not convert value from %q to a map[string]interface{}.", value.Value)
					results <- Result{Err: err}
					continue
				}

				for percentile, percentileValue := range sampleValue {
					individualValue, ok := percentileValue.(float64)
					if !ok {
						err = fmt.Errorf("Could not convert value from %q to a float64.", percentileValue)
						results <- Result{Err: err}
						continue
					}

					childMetric := make(map[model.LabelName]model.LabelValue, len(labels)+1)

					for k, v := range labels {
						childMetric[k] = v
					}

					childMetric[model.LabelName(percentile001)] = model.LabelValue(percentile)

					pendingSamples = append(pendingSamples, model.Sample{
						Metric:    model.Metric(childMetric),
						Timestamp: timestamp,
						Value:     model.SampleValue(individualValue),
					})
				}

				break
			}
		}
	}
	if len(pendingSamples) > 0 {
		results <- Result{Samples: pendingSamples}
	}

	return nil
}
