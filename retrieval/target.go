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
	"encoding/json"
	"fmt"
	"github.com/matttproud/golang_instrumentation/metrics"
	"github.com/matttproud/prometheus/model"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
)

type TargetState int

const (
	UNKNOWN TargetState = iota
	ALIVE
	UNREACHABLE
)

const (
	MAXIMUM_BACKOFF = time.Minute * 30
)

type Target struct {
	scheduledFor     time.Time
	unreachableCount int
	state            TargetState

	Address    string
	Deadline   time.Duration
	BaseLabels model.LabelSet

	// XXX: Move this to a field with the target manager initialization instead of here.
	Interval time.Duration
}

type Result struct {
	Err     error
	Samples []model.Sample
	Target  Target
}

func (t *Target) reschedule(s TargetState) {
	currentState := t.state

	switch currentState {
	case UNKNOWN, UNREACHABLE:
		switch s {
		case ALIVE:
			t.unreachableCount = 0
			targetsHealthy.Increment()
		case UNREACHABLE:
			backoff := MAXIMUM_BACKOFF
			exponential := time.Duration(math.Pow(2, float64(t.unreachableCount))) * time.Second
			if backoff > exponential {
				backoff = exponential
			}

			t.scheduledFor = time.Now().Add(backoff)
			t.unreachableCount++
			log.Printf("%s unavailable %s times deferred for %s.", t, t.unreachableCount, backoff)
		default:
		}
	case ALIVE:
		switch s {
		case UNREACHABLE:
			t.unreachableCount++
			targetsUnhealthy.Increment()
		}
	default:
	}

	if s != currentState {
		log.Printf("%s transitioning from %s to %s.", t, currentState, s)
	}

	t.state = s
}

func (t *Target) Scrape(results chan Result) (err error) {
	result := Result{}

	defer func() {
		futureState := t.state

		switch err {
		case nil:
			futureState = ALIVE
		default:
			futureState = UNREACHABLE
		}

		t.reschedule(futureState)

		result.Err = err
		results <- result
	}()

	done := make(chan bool)

	request := func() {
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

				result.Samples = append(result.Samples, s)
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

					result.Samples = append(result.Samples, s)
				}
			}
		}

		done <- true
	}

	accumulator := func(d time.Duration) {
		ms := float64(d) / float64(time.Millisecond)
		if err == nil {
			scrapeLatencyHealthy.Add(ms)
		} else {
			scrapeLatencyUnhealthy.Add(ms)
		}
	}

	go metrics.InstrumentCall(request, accumulator)

	select {
	case <-done:
		break
	case <-time.After(t.Deadline):
		err = fmt.Errorf("Target %s exceeded %s deadline.", t, t.Deadline)
	}

	return
}
