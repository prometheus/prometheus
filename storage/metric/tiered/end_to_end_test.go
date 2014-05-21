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

package tiered

import (
	"sort"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

func GetFingerprintsForLabelSetTests(p metric.Persistence, t testing.TB) {
	metrics := []clientmodel.Metric{
		{
			clientmodel.MetricNameLabel: "test_metric",
			"method":                    "get",
			"result":                    "success",
		},
		{
			clientmodel.MetricNameLabel: "test_metric",
			"method":                    "get",
			"result":                    "failure",
		},
		{
			clientmodel.MetricNameLabel: "test_metric",
			"method":                    "post",
			"result":                    "success",
		},
		{
			clientmodel.MetricNameLabel: "test_metric",
			"method":                    "post",
			"result":                    "failure",
		},
	}

	newTestLabelMatcher := func(matchType metric.MatchType, name clientmodel.LabelName, value clientmodel.LabelValue) *metric.LabelMatcher {
		m, err := metric.NewLabelMatcher(matchType, name, value)
		if err != nil {
			t.Fatalf("Couldn't create label matcher: %v", err)
		}
		return m
	}

	scenarios := []struct {
		in         metric.LabelMatchers
		outIndexes []int
	}{
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "test_metric"),
			},
			outIndexes: []int{0, 1, 2, 3},
		},
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "non_existent_metric"),
			},
			outIndexes: []int{},
		},
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "non_existent_metric"),
				newTestLabelMatcher(metric.Equal, "result", "success"),
			},
			outIndexes: []int{},
		},
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "test_metric"),
				newTestLabelMatcher(metric.Equal, "result", "success"),
			},
			outIndexes: []int{0, 2},
		},
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "test_metric"),
				newTestLabelMatcher(metric.NotEqual, "result", "success"),
			},
			outIndexes: []int{1, 3},
		},
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "test_metric"),
				newTestLabelMatcher(metric.RegexMatch, "result", "foo|success|bar"),
			},
			outIndexes: []int{0, 2},
		},
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "test_metric"),
				newTestLabelMatcher(metric.RegexNoMatch, "result", "foo|success|bar"),
			},
			outIndexes: []int{1, 3},
		},
		{
			in: metric.LabelMatchers{
				newTestLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, "test_metric"),
				newTestLabelMatcher(metric.RegexNoMatch, "result", "foo|success|bar"),
				newTestLabelMatcher(metric.RegexMatch, "method", "os"),
			},
			outIndexes: []int{3},
		},
	}

	for _, m := range metrics {
		testAppendSamples(p, &clientmodel.Sample{
			Value:     0,
			Timestamp: 0,
			Metric:    m,
		}, t)
	}

	for i, s := range scenarios {
		actualFps, err := p.GetFingerprintsForLabelMatchers(s.in)
		if err != nil {
			t.Fatalf("%d. Couldn't get fingerprints for label matchers: %v", i, err)
		}

		expectedFps := clientmodel.Fingerprints{}
		for _, i := range s.outIndexes {
			fp := &clientmodel.Fingerprint{}
			fp.LoadFromMetric(metrics[i])
			expectedFps = append(expectedFps, fp)
		}

		sort.Sort(actualFps)
		sort.Sort(expectedFps)

		if len(actualFps) != len(expectedFps) {
			t.Fatalf("%d. Got %d fingerprints; want %d", i, len(actualFps), len(expectedFps))
		}

		for j, actualFp := range actualFps {
			if !actualFp.Equal(expectedFps[j]) {
				t.Fatalf("%d.%d. Got fingerprint %v; want %v", i, j, actualFp, expectedFps[j])
			}
		}
	}
}

func GetLabelValuesForLabelNameTests(p metric.Persistence, t testing.TB) {
	testAppendSamples(p, &clientmodel.Sample{
		Value:     0,
		Timestamp: 0,
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "my_metric",
			"request_type":              "create",
			"result":                    "success",
		},
	}, t)

	testAppendSamples(p, &clientmodel.Sample{
		Value:     0,
		Timestamp: 0,
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: "my_metric",
			"request_type":              "delete",
			"outcome":                   "failure",
		},
	}, t)

	expectedIndex := map[clientmodel.LabelName]clientmodel.LabelValues{
		clientmodel.MetricNameLabel: {"my_metric"},
		"request_type":              {"create", "delete"},
		"result":                    {"success"},
		"outcome":                   {"failure"},
	}

	for name, expected := range expectedIndex {
		actual, err := p.GetLabelValuesForLabelName(name)
		if err != nil {
			t.Fatalf("Error getting values for label %s: %v", name, err)
		}
		if len(actual) != len(expected) {
			t.Fatalf("Number of values don't match for label %s: got %d; want %d", name, len(actual), len(expected))
		}
		for i := range expected {
			if actual[i] != expected[i] {
				t.Fatalf("%d. Got %s; want %s", i, actual[i], expected[i])
			}
		}
	}
}

func GetMetricForFingerprintTests(p metric.Persistence, t testing.TB) {
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

	result, err := p.GetFingerprintsForLabelMatchers(metric.LabelMatchers{{
		Type:  metric.Equal,
		Name:  "request_type",
		Value: "your_mom",
	}})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	m, err := p.GetMetricForFingerprint(result[0])
	if err != nil {
		t.Error(err)
	}

	if m == nil {
		t.Fatal("Did not expect nil.")
	}

	if len(m) != 1 {
		t.Errorf("Expected one-dimensional metric.")
	}

	if m["request_type"] != "your_mom" {
		t.Errorf("Expected metric to match.")
	}

	result, err = p.GetFingerprintsForLabelMatchers(metric.LabelMatchers{{
		Type:  metric.Equal,
		Name:  "request_type",
		Value: "your_dad",
	}})

	if err != nil {
		t.Error(err)
	}

	if len(result) != 1 {
		t.Errorf("Expected one element.")
	}

	m, err = p.GetMetricForFingerprint(result[0])

	if m == nil {
		t.Fatal("Did not expect nil.")
	}

	if err != nil {
		t.Error(err)
	}

	if len(m) != 2 {
		t.Errorf("Expected two-dimensional metric.")
	}

	if m["request_type"] != "your_dad" {
		t.Errorf("Expected metric to match.")
	}

	if m["one-off"] != "value" {
		t.Errorf("Expected metric to match.")
	}

	// Verify that mutating a returned metric does not result in the mutated
	// metric to be returned at the next GetMetricForFingerprint() call.
	m["one-off"] = "new value"
	m, err = p.GetMetricForFingerprint(result[0])

	if m == nil {
		t.Fatal("Did not expect nil.")
	}

	if err != nil {
		t.Error(err)
	}

	if len(m) != 2 {
		t.Errorf("Expected two-dimensional metric.")
	}

	if m["request_type"] != "your_dad" {
		t.Errorf("Expected metric to match.")
	}

	if m["one-off"] != "value" {
		t.Errorf("Expected metric to match.")
	}
}

func AppendRepeatingValuesTests(p metric.Persistence, t testing.TB) {
	m := clientmodel.Metric{
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
				Metric:    m,
			}, t)
		}
	}

	v, ok := p.(metric.View)
	if !ok {
		// It's purely a benchmark for a Persistence that is not viewable.
		return
	}

	matchers := labelMatchersFromLabelSet(clientmodel.LabelSet{
		clientmodel.MetricNameLabel: "errors_total",
		"controller":                "foo",
		"operation":                 "bar",
	})

	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			fingerprints, err := p.GetFingerprintsForLabelMatchers(matchers)
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

func AppendsRepeatingValuesTests(p metric.Persistence, t testing.TB) {
	m := clientmodel.Metric{
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
				Metric:    m,
			})
		}
	}

	p.AppendSamples(s)

	v, ok := p.(metric.View)
	if !ok {
		// It's purely a benchmark for a MetricPersistance that is not viewable.
		return
	}

	matchers := labelMatchersFromLabelSet(clientmodel.LabelSet{
		clientmodel.MetricNameLabel: "errors_total",
		"controller":                "foo",
		"operation":                 "bar",
	})

	for i := 0; i < increments; i++ {
		for j := 0; j < repetitions; j++ {
			fingerprints, err := p.GetFingerprintsForLabelMatchers(matchers)
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

var testLevelDBGetLabelValuesForLabelName = buildLevelDBTestPersistence("get_label_values_for_labelname", GetLabelValuesForLabelNameTests)

func TestLevelDBGetFingerprintsForLabelName(t *testing.T) {
	testLevelDBGetLabelValuesForLabelName(t)
}

func BenchmarkLevelDBGetLabelValuesForLabelName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetLabelValuesForLabelName(b)
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

var testMemoryGetLabelValuesForLabelName = buildMemoryTestPersistence(GetLabelValuesForLabelNameTests)

func TestMemoryGetLabelValuesForLabelName(t *testing.T) {
	testMemoryGetLabelValuesForLabelName(t)
}

func BenchmarkMemoryGetLabelValuesForLabelName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetLabelValuesForLabelName(b)
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
