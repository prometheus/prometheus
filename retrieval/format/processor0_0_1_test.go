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

package format

import (
	"container/list"
	"fmt"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility/test"
	"os"
	"path"
	"testing"
	"time"
)

func testProcessor001Process(t test.Tester) {
	var scenarios = []struct {
		in         string
		baseLabels model.LabelSet
		out        model.Samples
		err        error
	}{
		{
			in:  "empty.json",
			err: fmt.Errorf("unexpected end of JSON input"),
		},
		{
			in: "test0_0_1-0_0_2.json",
			baseLabels: model.LabelSet{
				model.JobLabel: "batch_exporter",
			},
			out: model.Samples{
				model.Sample{
					Metric: model.Metric{"service": "zed", model.MetricNameLabel: "rpc_calls_total", "job": "batch_job", "exporter_job": "batch_exporter"},
					Value:  25,
				},
				model.Sample{
					Metric: model.Metric{"service": "bar", model.MetricNameLabel: "rpc_calls_total", "job": "batch_job", "exporter_job": "batch_exporter"},
					Value:  25,
				},
				model.Sample{
					Metric: model.Metric{"service": "foo", model.MetricNameLabel: "rpc_calls_total", "job": "batch_job", "exporter_job": "batch_exporter"},
					Value:  25,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.010000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "zed", "job": "batch_exporter"},
					Value:  0.0459814091918713,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.010000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "bar", "job": "batch_exporter"},
					Value:  78.48563317257356,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.010000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "foo", "job": "batch_exporter"},
					Value:  15.890724674774395,
				},
				model.Sample{

					Metric: model.Metric{"percentile": "0.050000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "zed", "job": "batch_exporter"},
					Value:  0.0459814091918713,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.050000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "bar", "job": "batch_exporter"},
					Value:  78.48563317257356,
				},
				model.Sample{

					Metric: model.Metric{"percentile": "0.050000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "foo", "job": "batch_exporter"},
					Value:  15.890724674774395,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.500000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "zed", "job": "batch_exporter"},
					Value:  0.6120456642749681,
				},
				model.Sample{

					Metric: model.Metric{"percentile": "0.500000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "bar", "job": "batch_exporter"},
					Value:  97.31798360385088,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.500000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "foo", "job": "batch_exporter"},
					Value:  84.63044031436561,
				},
				model.Sample{

					Metric: model.Metric{"percentile": "0.900000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "zed", "job": "batch_exporter"},
					Value:  1.355915069887731,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.900000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "bar", "job": "batch_exporter"},
					Value:  109.89202084295582,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.900000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "foo", "job": "batch_exporter"},
					Value:  160.21100853053224,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.990000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "zed", "job": "batch_exporter"},
					Value:  1.772733213161236,
				},
				model.Sample{

					Metric: model.Metric{"percentile": "0.990000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "bar", "job": "batch_exporter"},
					Value:  109.99626121011262,
				},
				model.Sample{
					Metric: model.Metric{"percentile": "0.990000", model.MetricNameLabel: "rpc_latency_microseconds", "service": "foo", "job": "batch_exporter"},
					Value:  172.49828748957728,
				},
			},
		},
	}

	for i, scenario := range scenarios {
		inputChannel := make(chan Result, 1024)

		defer func(c chan Result) {
			close(c)
		}(inputChannel)

		reader, err := os.Open(path.Join("fixtures", scenario.in))
		if err != nil {
			t.Fatalf("%d. couldn't open scenario input file %s: %s", i, scenario.in, err)
		}

		err = Processor001.Process(reader, time.Now(), scenario.baseLabels, inputChannel)
		if !test.ErrorEqual(scenario.err, err) {
			t.Errorf("%d. expected err of %s, got %s", i, scenario.err, err)
			continue
		}

		delivered := model.Samples{}

		for len(inputChannel) != 0 {
			result := <-inputChannel
			if result.Err != nil {
				t.Fatalf("%d. expected no error, got: %s", i, result.Err)
			}
			delivered = append(delivered, result.Samples...)
		}

		if len(delivered) != len(scenario.out) {
			t.Errorf("%d. expected output length of %d, got %d", i, len(scenario.out), len(delivered))

			continue
		}

		expectedElements := list.New()
		for _, j := range scenario.out {
			expectedElements.PushBack(j)
		}

		for j := 0; j < len(delivered); j++ {
			actual := delivered[j]

			found := false
			for element := expectedElements.Front(); element != nil && found == false; element = element.Next() {
				candidate := element.Value.(model.Sample)

				if candidate.Value != actual.Value {
					continue
				}

				if len(candidate.Metric) != len(actual.Metric) {
					continue
				}

				labelsMatch := false

				for key, value := range candidate.Metric {
					actualValue, ok := actual.Metric[key]
					if !ok {
						break
					}
					if actualValue == value {
						labelsMatch = true
						break
					}
				}

				if !labelsMatch {
					continue
				}

				// XXX: Test time.
				found = true
				expectedElements.Remove(element)
			}

			if !found {
				t.Errorf("%d.%d. expected to find %s among candidate, absent", i, j, actual)
			}
		}
	}
}

func TestProcessor001Process(t *testing.T) {
	testProcessor001Process(t)
}

func BenchmarkProcessor001Process(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testProcessor001Process(b)
	}
}
