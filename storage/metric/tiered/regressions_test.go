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
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

func GetFingerprintsForLabelSetUsesAndForLabelMatchingTests(p metric.Persistence, t testing.TB) {
	metrics := []clientmodel.LabelSet{
		{clientmodel.MetricNameLabel: "request_metrics_latency_equal_tallying_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{clientmodel.MetricNameLabel: "requests_metrics_latency_equal_accumulating_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{clientmodel.MetricNameLabel: "requests_metrics_latency_logarithmic_accumulating_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{clientmodel.MetricNameLabel: "requests_metrics_latency_logarithmic_tallying_microseconds", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
		{clientmodel.MetricNameLabel: "targets_healthy_scrape_latency_ms", "instance": "http://localhost:9090/metrics.json", "percentile": "0.010000"},
	}

	for _, metric := range metrics {
		m := clientmodel.Metric{}

		for k, v := range metric {
			m[clientmodel.LabelName(k)] = clientmodel.LabelValue(v)
		}

		testAppendSamples(p, &clientmodel.Sample{
			Value:     clientmodel.SampleValue(0.0),
			Timestamp: clientmodel.Now(),
			Metric:    m,
		}, t)
	}

	labelSet := clientmodel.LabelSet{
		clientmodel.MetricNameLabel: "targets_healthy_scrape_latency_ms",
		"percentile":                "0.010000",
	}

	fingerprints, err := p.GetFingerprintsForLabelMatchers(labelMatchersFromLabelSet(labelSet))
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
