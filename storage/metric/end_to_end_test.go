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
	"testing"
	"time"
)

func GetFingerprintsForLabelSetTests(p MetricPersistence, t test.Tester) {
	testAppendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_mom",
		},
	}, t)

	testAppendSample(p, model.Sample{
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
	testAppendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"name":         "my_metric",
			"request_type": "your_mom",
			"language":     "english",
		},
	}, t)

	testAppendSample(p, model.Sample{
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
	testAppendSample(p, model.Sample{
		Value:     0,
		Timestamp: time.Time{},
		Metric: model.Metric{
			"request_type": "your_mom",
		},
	}, t)

	testAppendSample(p, model.Sample{
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

func AppendRepeatingValuesTests(p MetricPersistence, t test.Tester) {
	metric := model.Metric{
		"controller": "foo",
		"name":       "errors_total",
		"operation":  "bar",
	}

	increments := 10
	repetitions := 500

	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			time := time.Time{}.Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			testAppendSample(p, model.Sample{
				Value:     model.SampleValue(i),
				Timestamp: time,
				Metric:    metric,
			}, t)
		}
	}

	if true {
		// XXX: Purely a benchmark.
		return
	}

	labelSet := model.LabelSet{
		"controller": "foo",
		"name":       "errors_total",
		"operation":  "bar",
	}

	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			fingerprints, err := p.GetFingerprintsForLabelSet(labelSet)
			if err != nil {
				t.Fatal(err)
			}
			if len(fingerprints) != 1 {
				t.Fatalf("expected %d fingerprints, got %d", 1, len(fingerprints))
			}

			time := time.Time{}.Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			sample, err := p.GetValueAtTime(fingerprints[0], time, StalenessPolicy{})
			if err != nil {
				t.Fatal(err)
			}
			if sample == nil {
				t.Fatal("expected non-nil sample.")
			}

			expected := model.SampleValue(i)

			if sample.Value != expected {
				t.Fatalf("expected %d value, got %d", expected, sample.Value)
			}
		}
	}
}

func AppendsRepeatingValuesTests(p MetricPersistence, t test.Tester) {
	metric := model.Metric{
		"controller": "foo",
		"name":       "errors_total",
		"operation":  "bar",
	}

	increments := 10
	repetitions := 500

	s := model.Samples{}
	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			time := time.Time{}.Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			s = append(s, model.Sample{
				Value:     model.SampleValue(i),
				Timestamp: time,
				Metric:    metric,
			})
		}
	}

	p.AppendSamples(s)

	if true {
		// XXX: Purely a benchmark.
		return
	}

	labelSet := model.LabelSet{
		"controller": "foo",
		"name":       "errors_total",
		"operation":  "bar",
	}

	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			fingerprints, err := p.GetFingerprintsForLabelSet(labelSet)
			if err != nil {
				t.Fatal(err)
			}
			if len(fingerprints) != 1 {
				t.Fatalf("expected %d fingerprints, got %d", 1, len(fingerprints))
			}

			time := time.Time{}.Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			sample, err := p.GetValueAtTime(fingerprints[0], time, StalenessPolicy{})
			if err != nil {
				t.Fatal(err)
			}
			if sample == nil {
				t.Fatal("expected non-nil sample.")
			}

			expected := model.SampleValue(i)

			if sample.Value != expected {
				t.Fatalf("expected %d value, got %d", expected, sample.Value)
			}
		}
	}
}

// Test Definitions Below

var testLevelDBGetFingerprintsForLabelSet = buildLevelDBTestPersistence("get_fingerprints_for_labelset", GetFingerprintsForLabelSetTests)

func TestLevelDBGetFingerprintsForLabelSet(t *testing.T) {
	testLevelDBGetFingerprintsForLabelSet(t)
}

func BenchmarkLevelDBGetFingerprintsForLabelSet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetFingerprintsForLabelSet(b)
	}
}

var testLevelDBGetFingerprintsForLabelName = buildLevelDBTestPersistence("get_fingerprints_for_labelname", GetFingerprintsForLabelNameTests)

func TestLevelDBGetFingerprintsForLabelName(t *testing.T) {
	testLevelDBGetFingerprintsForLabelName(t)
}

func BenchmarkLevelDBGetFingerprintsForLabelName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetFingerprintsForLabelName(b)
	}
}

var testLevelDBGetMetricForFingerprint = buildLevelDBTestPersistence("get_metric_for_fingerprint", GetMetricForFingerprintTests)

func TestLevelDBGetMetricForFingerprint(t *testing.T) {
	testLevelDBGetMetricForFingerprint(t)
}

func BenchmarkLevelDBGetMetricForFingerprint(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetMetricForFingerprint(b)
	}
}

var testLevelDBAppendRepeatingValues = buildLevelDBTestPersistence("append_repeating_values", AppendRepeatingValuesTests)

func TestLevelDBAppendRepeatingValues(t *testing.T) {
	testLevelDBAppendRepeatingValues(t)
}

func BenchmarkLevelDBAppendRepeatingValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBAppendRepeatingValues(b)
	}
}

var testLevelDBAppendsRepeatingValues = buildLevelDBTestPersistence("appends_repeating_values", AppendsRepeatingValuesTests)

func TestLevelDBAppendsRepeatingValues(t *testing.T) {
	testLevelDBAppendsRepeatingValues(t)
}

func BenchmarkLevelDBAppendsRepeatingValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBAppendsRepeatingValues(b)
	}
}

var testMemoryGetFingerprintsForLabelSet = buildMemoryTestPersistence(GetFingerprintsForLabelSetTests)

func TestMemoryGetFingerprintsForLabelSet(t *testing.T) {
	testMemoryGetFingerprintsForLabelSet(t)
}

func BenchmarkMemoryGetFingerprintsForLabelSet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetFingerprintsForLabelSet(b)
	}
}

var testMemoryGetFingerprintsForLabelName = buildMemoryTestPersistence(GetFingerprintsForLabelNameTests)

func TestMemoryGetFingerprintsForLabelName(t *testing.T) {
	testMemoryGetFingerprintsForLabelName(t)
}

func BenchmarkMemoryGetFingerprintsForLabelName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetFingerprintsForLabelName(b)
	}
}

var testMemoryGetMetricForFingerprint = buildMemoryTestPersistence(GetMetricForFingerprintTests)

func TestMemoryGetMetricForFingerprint(t *testing.T) {
	testMemoryGetMetricForFingerprint(t)
}

func BenchmarkMemoryGetMetricForFingerprint(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetMetricForFingerprint(b)
	}
}

var testMemoryAppendRepeatingValues = buildMemoryTestPersistence(AppendRepeatingValuesTests)

func TestMemoryAppendRepeatingValues(t *testing.T) {
	testMemoryAppendRepeatingValues(t)
}

func BenchmarkMemoryAppendRepeatingValues(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryAppendRepeatingValues(b)
	}
}
