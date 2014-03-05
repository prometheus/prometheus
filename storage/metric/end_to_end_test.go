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
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility/test"
)

func GetFingerprintsForLabelSetTests(p MetricPersistence, t test.Tester) {
	testAppendSamples(p, &clientmodel.Sample{
		Value:     0,
		Timestamp: 0,
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "my_metric",
			"request_type":              "your_mom",
		},
	}, t)

	testAppendSamples(p, &clientmodel.Sample{
		Value:     0,
		Timestamp: 0,
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "my_metric",
			"request_type":              "your_dad",
		},
	}, t)

	result, err := p.GetFingerprintsForLabelSet(clientmodel.LabelSet{
		clientmodel.MetricNameLabel: clientmodel.LabelValue("my_metric"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 2 {
		t.Errorf("Expected two elements.")
	}

	result, err = p.GetFingerprintsForLabelSet(clientmodel.LabelSet{
		clientmodel.LabelName("request_type"): clientmodel.LabelValue("your_mom"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	result, err = p.GetFingerprintsForLabelSet(clientmodel.LabelSet{
		clientmodel.LabelName("request_type"): clientmodel.LabelValue("your_dad"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}
}

func GetMetricForFingerprintTests(p MetricPersistence, t test.Tester) {
	testAppendSamples(p, &clientmodel.Sample{
		Value:     0,
		Timestamp: 0,
		Metric: clientmodel.Metric{
			"request_type": "your_mom",
		},
	}, t)

	testAppendSamples(p, &clientmodel.Sample{
		Value:     0,
		Timestamp: 0,
		Metric: clientmodel.Metric{
			"request_type": "your_dad",
			"one-off":      "value",
		},
	}, t)

	result, err := p.GetFingerprintsForLabelSet(clientmodel.LabelSet{
		clientmodel.LabelName("request_type"): clientmodel.LabelValue("your_mom"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	metric, err := p.GetMetricForFingerprint(result[0])
	if err != nil {
		t.Error(err)
	}

	if metric == nil {
		t.Fatal("Did not expect nil.")
	}

	if len(metric) != 1 {
		t.Errorf("Expected one-dimensional metric.")
	}

	if metric["request_type"] != "your_mom" {
		t.Errorf("Expected metric to match.")
	}

	result, err = p.GetFingerprintsForLabelSet(clientmodel.LabelSet{
		clientmodel.LabelName("request_type"): clientmodel.LabelValue("your_dad"),
	})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	metric, err = p.GetMetricForFingerprint(result[0])

	if metric == nil {
		t.Fatal("Did not expect nil.")
	}

	if err != nil {
		t.Error(err)
	}

	if len(metric) != 2 {
		t.Errorf("Expected two-dimensional metric.")
	}

	if metric["request_type"] != "your_dad" {
		t.Errorf("Expected metric to match.")
	}

	if metric["one-off"] != "value" {
		t.Errorf("Expected metric to match.")
	}

	// Verify that mutating a returned metric does not result in the mutated
	// metric to be returned at the next GetMetricForFingerprint() call.
	metric["one-off"] = "new value"
	metric, err = p.GetMetricForFingerprint(result[0])

	if metric == nil {
		t.Fatal("Did not expect nil.")
	}

	if err != nil {
		t.Error(err)
	}

	if len(metric) != 2 {
		t.Errorf("Expected two-dimensional metric.")
	}

	if metric["request_type"] != "your_dad" {
		t.Errorf("Expected metric to match.")
	}

	if metric["one-off"] != "value" {
		t.Errorf("Expected metric to match.")
	}
}

func AppendRepeatingValuesTests(p MetricPersistence, t test.Tester) {
	metric := clientmodel.Metric{
		clientmodel.MetricNameLabel: "errors_total",
		"controller":                "foo",
		"operation":                 "bar",
	}

	increments := 10
	repetitions := 500

	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			time := clientmodel.Timestamp(0).Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			testAppendSamples(p, &clientmodel.Sample{
				Value:     clientmodel.SampleValue(i),
				Timestamp: time,
				Metric:    metric,
			}, t)
		}
	}

	v, ok := p.(View)
	if !ok {
		// It's purely a benchmark for a MetricPersistence that is not viewable.
		return
	}

	labelSet := clientmodel.LabelSet{
		clientmodel.MetricNameLabel: "errors_total",
		"controller":                "foo",
		"operation":                 "bar",
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

			time := clientmodel.Timestamp(0).Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			samples := v.GetValueAtTime(fingerprints[0], time)
			if len(samples) == 0 {
				t.Fatal("expected at least one sample.")
			}

			expected := clientmodel.SampleValue(i)

			for _, sample := range samples {
				if sample.Value != expected {
					t.Fatalf("expected %v value, got %v", expected, sample.Value)
				}
			}
		}
	}
}

func AppendsRepeatingValuesTests(p MetricPersistence, t test.Tester) {
	metric := clientmodel.Metric{
		clientmodel.MetricNameLabel: "errors_total",
		"controller":                "foo",
		"operation":                 "bar",
	}

	increments := 10
	repetitions := 500

	s := clientmodel.Samples{}
	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			time := clientmodel.Timestamp(0).Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			s = append(s, &clientmodel.Sample{
				Value:     clientmodel.SampleValue(i),
				Timestamp: time,
				Metric:    metric,
			})
		}
	}

	p.AppendSamples(s)

	v, ok := p.(View)
	if !ok {
		// It's purely a benchmark for a MetricPersistance that is not viewable.
		return
	}

	labelSet := clientmodel.LabelSet{
		clientmodel.MetricNameLabel: "errors_total",
		"controller":                "foo",
		"operation":                 "bar",
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

			time := clientmodel.Timestamp(0).Add(time.Duration(i) * time.Hour).Add(time.Duration(j) * time.Second)
			samples := v.GetValueAtTime(fingerprints[0], time)
			if len(samples) == 0 {
				t.Fatal("expected at least one sample.")
			}

			expected := clientmodel.SampleValue(i)

			for _, sample := range samples {
				if sample.Value != expected {
					t.Fatalf("expected %v value, got %v", expected, sample.Value)
				}
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
