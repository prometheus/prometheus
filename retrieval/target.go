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

package retrieval

import (
	"time"
)

type TargetState int

const (
	UNKNOWN TargetState = iota
	ALIVE
	UNREACHABLE
)

type Target struct {
	scheduledFor     time.Time
	unreachableCount int
	state            TargetState

	Address   string
	Frequency time.Duration
}

// KEPT FOR LEGACY COMPATIBILITY; PENDING REFACTOR

func (t *Target) Scrape() (samples []model.Sample, err error) {
	defer func() {
		if err != nil {
			t.state = ALIVE
		}
	}()

	ti := time.Now()
	resp, err := http.Get(t.Address)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	intermediate := make(map[string]interface{})
	err = json.Unmarshal(raw, &intermediate)
	if err != nil {
		return
	}

	baseLabels := map[string]string{"instance": t.Address}

	for name, v := range intermediate {
		asMap, ok := v.(map[string]interface{})

		if !ok {
			continue
		}

		switch asMap["type"] {
		case "counter":
			m := model.Metric{}
			m["name"] = model.LabelValue(name)
			asFloat, ok := asMap["value"].(float64)
			if !ok {
				continue
			}

			s := model.Sample{
				Metric:    m,
				Value:     model.SampleValue(asFloat),
				Timestamp: ti,
			}

			for baseK, baseV := range baseLabels {
				m[model.LabelName(baseK)] = model.LabelValue(baseV)
			}

			samples = append(samples, s)
		case "histogram":
			values, ok := asMap["value"].(map[string]interface{})
			if !ok {
				continue
			}

			for p, pValue := range values {
				asString, ok := pValue.(string)
				if !ok {
					continue
				}

				float, err := strconv.ParseFloat(asString, 64)
				if err != nil {
					continue
				}

				m := model.Metric{}
				m["name"] = model.LabelValue(name)
				m["percentile"] = model.LabelValue(p)

				s := model.Sample{
					Metric:    m,
					Value:     model.SampleValue(float),
					Timestamp: ti,
				}

				for baseK, baseV := range baseLabels {
					m[model.LabelName(baseK)] = model.LabelValue(baseV)
				}

				samples = append(samples, s)
			}
		}
	}

	return
}
