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
	"io"
	"time"
)

// Processor for telemetry schema version 0.0.2.
var Processor002 ProcessorFunc = func(stream io.ReadCloser, timestamp time.Time, baseLabels model.LabelSet, results chan Result) error {
	// container for telemetry data
	var entities []struct {
		BaseLabels map[string]string `json:"baseLabels"`
		Docstring  string            `json:"docstring"`
		Metric     struct {
			Type   string          `json:"type"`
			Values json.RawMessage `json:"value"`
		} `json:"metric"`
	}

	// concrete type for histogram values
	type histogram struct {
		Labels map[string]string            `json:"labels"`
		Values map[string]model.SampleValue `json:"value"`
	}

	// concrete type for counter and gauge values
	type counter struct {
		Labels map[string]string `json:"labels"`
		Value  model.SampleValue `json:"value"`
	}

	defer stream.Close()

	if err := json.NewDecoder(stream).Decode(&entities); err != nil {
		return err
	}

	pendingSamples := model.Samples{}
	for _, entity := range entities {
		entityLabels := baseLabels.Merge(LabelSet(entity.BaseLabels))

		switch entity.Metric.Type {
		case "counter", "gauge":
			var values []counter

			if err := json.Unmarshal(entity.Metric.Values, &values); err != nil {
				results <- Result{
					Err: fmt.Errorf("Could not extract %s value: %s", entity.Metric.Type, err),
				}
				continue
			}

			for _, counter := range values {
				labels := entityLabels.Merge(LabelSet(counter.Labels))

				pendingSamples = append(pendingSamples, model.Sample{
					Metric:    model.Metric(labels),
					Timestamp: timestamp,
					Value:     counter.Value,
				})
			}

		case "histogram":
			var values []histogram

			if err := json.Unmarshal(entity.Metric.Values, &values); err != nil {
				results <- Result{
					Err: fmt.Errorf("Could not extract %s value: %s", entity.Metric.Type, err),
				}
				continue
			}

			for _, histogram := range values {
				for percentile, value := range histogram.Values {
					labels := entityLabels.Merge(LabelSet(histogram.Labels))
					labels[model.LabelName("percentile")] = model.LabelValue(percentile)

					pendingSamples = append(pendingSamples, model.Sample{
						Metric:    model.Metric(labels),
						Timestamp: timestamp,
						Value:     value,
					})
				}
			}

		default:
			results <- Result{
				Err: fmt.Errorf("Unknown metric type %q", entity.Metric.Type),
			}
		}
	}

	if len(pendingSamples) > 0 {
		results <- Result{Samples: pendingSamples}
	}

	return nil
}
