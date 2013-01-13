// Copyright 2012 Prometheus Team
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

package leveldb

import (
	"github.com/matttproud/prometheus/model"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestGetFingerprintsForLabelSetUsesAnd(t *testing.T) {
	temporaryDirectory, _ := ioutil.TempDir("", "test_get_fingerprints_for_label_set_uses_and")

	defer func() {
		err := os.RemoveAll(temporaryDirectory)
		if err != nil {
			t.Errorf("could not remove temporary directory: %f", err)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)
	defer persistence.Close()

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

		err := persistence.AppendSample(&model.Sample{
			Value:     model.SampleValue(0.0),
			Timestamp: time.Now(),
			Metric:    m,
		})

		if err != nil {
			t.Errorf("could not create base sample: %s", err)
		}
	}

	labelSet := model.LabelSet{
		"name":       "targets_healthy_scrape_latency_ms",
		"percentile": "0.010000",
	}

	fingerprints, err := persistence.GetFingerprintsForLabelSet(&labelSet)
	if err != nil {
		t.Errorf("could not get labels: %s", err)
	}

	if len(fingerprints) != 1 {
		t.Errorf("did not get a single metric as is expected, got %s", fingerprints)
	}
}
