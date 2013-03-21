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

func GetFingerprintsForLabelSetUsesAndForLabelMatchingTests(p MetricPersistence, t test.Tester) {
	metrics := []map[string]string{
		{"name": "request_metrics_latency_equal_tallying_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{"name": "requests_metrics_latency_equal_accumulating_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{"name": "requests_metrics_latency_logarithmic_accumulating_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{"name": "requests_metrics_latency_logarithmic_tallying_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{"name": "targets_healthy_scrape_latency_ms", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
	}

	for _, metric := range metrics {
		m := model.Metric{}

		for k, v := range metric {
			m[model.LabelName(k)] = model.LabelValue(v)
		}

		testAppendSample(p, model.Sample{
			Value:     model.SampleValue(0.0),
			Timestamp: time.Now(),
			Metric:    m,
		}, t)
	}

	labelSet := model.LabelSet{
		"name":       "targets_healthy_scrape_latency_ms",
		"percentile": "0.010000",
	}

	fingerprints, err := p.GetFingerprintsForLabelSet(labelSet)
	if err != nil {
		t.Errorf("could not get labels: %s", err)
	}

	if len(fingerprints) != 1 {
		t.Errorf("did not get a single metric as is expected, got %s", fingerprints)
	}
}

// Test Definitions Below

var testLevelDBGetFingerprintsForLabelSetUsesAndForLabelMatching = buildLevelDBTestPersistence("get_fingerprints_for_labelset_uses_and_for_label_matching", GetFingerprintsForLabelSetUsesAndForLabelMatchingTests)

func TestLevelDBGetFingerprintsForLabelSetUsesAndForLabelMatching(t *testing.T) {
	testLevelDBGetFingerprintsForLabelSetUsesAndForLabelMatching(t)
}

func BenchmarkLevelDBGetFingerprintsForLabelSetUsesAndForLabelMatching(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testLevelDBGetFingerprintsForLabelSetUsesAndForLabelMatching(b)
	}
}

var testMemoryGetFingerprintsForLabelSetUsesAndForLabelMatching = buildMemoryTestPersistence(GetFingerprintsForLabelSetUsesAndForLabelMatchingTests)

func TestMemoryGetFingerprintsForLabelSetUsesAndForLabelMatching(t *testing.T) {
	testMemoryGetFingerprintsForLabelSetUsesAndForLabelMatching(t)
}

func BenchmarkMemoryGetFingerprintsForLabelSetUsesAndLabelMatching(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMemoryGetFingerprintsForLabelSetUsesAndForLabelMatching(b)
	}
}
