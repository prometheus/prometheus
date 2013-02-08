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

package metric

import (
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility/test"
	"time"
)

func GetFingerprintsForLabelSetTests(p MetricPersistence, t test.Tester) {
	appendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_mom",
		},
	}, t)

	appendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_dad",
		},
	}, t)

	result, err := p.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("name"): model.LabelValue("my_metric"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 2 {
		t.Errorf("Expected two elements.")
	}

	result, err = p.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_mom"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	result, err = p.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_dad"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}
}

func GetFingerprintsForLabelNameTests(p MetricPersistence, t test.Tester) {
	appendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_mom",
			"language":     "english",
		},
	}, t)

	appendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_dad",
			"sprache":      "deutsch",
		},
	}, t)

	b := model.LabelName("name")
	result, err := p.GetFingerprintsForLabelName(b)

	if err != nil {
		t.Error(err)
	}

	if len(result) != 2 {
		t.Errorf("Expected two elements.")
	}

	b = model.LabelName("request_type")
	result, err = p.GetFingerprintsForLabelName(b)

	if err != nil {
		t.Error(err)
	}

	if len(result) != 2 {
		t.Errorf("Expected two elements.")
	}

	b = model.LabelName("language")
	result, err = p.GetFingerprintsForLabelName(b)

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	b = model.LabelName("sprache")
	result, err = p.GetFingerprintsForLabelName(b)

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}
}

func GetMetricForFingerprintTests(p MetricPersistence, t test.Tester) {
	appendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"request_type": "your_mom",
		},
	}, t)

	appendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"request_type": "your_dad",
			"one-off":      "value",
		},
	}, t)

	result, err := p.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_mom"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	v, e := p.GetMetricForFingerprint(result[0])
	if e != nil {
		t.Error(e)
	}

	if v == nil {
		t.Fatal("Did not expect nil.")
	}

	metric := *v

	if len(metric) != 1 {
		t.Errorf("Expected one-dimensional metric.")
	}

	if metric["request_type"] != "your_mom" {
		t.Errorf("Expected metric to match.")
	}

	result, err = p.GetFingerprintsForLabelSet(model.LabelSet{
		model.LabelName("request_type"): model.LabelValue("your_dad"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	v, e = p.GetMetricForFingerprint(result[0])

	if v == nil {
		t.Fatal("Did not expect nil.")
	}

	metric = *v

	if e != nil {
		t.Error(e)
	}

	if len(metric) != 2 {
		t.Errorf("Expected one-dimensional metric.")
	}

	if metric["request_type"] != "your_dad" {
		t.Errorf("Expected metric to match.")
	}

	if metric["one-off"] != "value" {
		t.Errorf("Expected metric to match.")
	}
}
