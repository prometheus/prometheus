package retrieval

import (
	"encoding/json"
	"github.com/matttproud/prometheus/model"
	"io/ioutil"
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

type Target struct {
	State     TargetState
	Address   string
	Staleness time.Duration
	Frequency time.Duration
}

func (t *Target) Scrape() (samples []model.Sample, err error) {
	defer func() {
		if err != nil {
			t.State = ALIVE
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
