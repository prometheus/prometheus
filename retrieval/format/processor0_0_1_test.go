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
	"fmt"
	"github.com/matttproud/prometheus/utility/test"
	"io/ioutil"
	"strings"
	"testing"
)

func testProcessor001Process(t test.Tester) {
	var scenarios = []struct {
		in  string
		out []Result
		err error
	}{
		{
			err: fmt.Errorf("unexpected end of JSON input"),
		},
		{
			in:  "[{\"baseLabels\":{\"name\":\"rpc_calls_total\"},\"docstring\":\"RPC calls.\",\"metric\":{\"type\":\"counter\",\"value\":[{\"labels\":{\"service\":\"zed\"},\"value\":25},{\"labels\":{\"service\":\"bar\"},\"value\":25},{\"labels\":{\"service\":\"foo\"},\"value\":25}]}},{\"baseLabels\":{\"name\":\"rpc_latency_microseconds\"},\"docstring\":\"RPC latency.\",\"metric\":{\"type\":\"histogram\",\"value\":[{\"labels\":{\"service\":\"foo\"},\"value\":{\"0.010000\":15.890724674774395,\"0.050000\":15.890724674774395,\"0.500000\":84.63044031436561,\"0.900000\":160.21100853053224,\"0.990000\":172.49828748957728}},{\"labels\":{\"service\":\"zed\"},\"value\":{\"0.010000\":0.0459814091918713,\"0.050000\":0.0459814091918713,\"0.500000\":0.6120456642749681,\"0.900000\":1.355915069887731,\"0.990000\":1.772733213161236}},{\"labels\":{\"service\":\"bar\"},\"value\":{\"0.010000\":78.48563317257356,\"0.050000\":78.48563317257356,\"0.500000\":97.31798360385088,\"0.900000\":109.89202084295582,\"0.990000\":109.99626121011262}}]}}]",
			out: []Result{},
		},
	}

	for i, scenario := range scenarios {
		inputChannel := make(chan Result, 1024)
		defer func(c chan Result) {
			close(c)
		}(inputChannel)
		reader := strings.NewReader(scenario.in)
		err := Processor001.Process(ioutil.NopCloser(reader), inputChannel)

		if scenario.err != nil && err != nil {
			if scenario.err.Error() != err.Error() {
				t.Errorf("%d. expected err of %s, got %s", i, scenario.err, err)
			}
		} else if scenario.err != err {
			t.Errorf("%d. expected err of %s, got %s", i, scenario.err, err)
		}

		delivered := make([]Result, 0)

		for len(inputChannel) != 0 {
			delivered = append(delivered, <-inputChannel)
		}

		if len(delivered) != len(scenario.out) {
			t.Errorf("%d. expected output length of %d, got %d", i, len(scenario.out), len(delivered))

			continue
		}

		for j := 0; j < len(delivered); j++ {
			actual := delivered[j]
			expected := scenario.out[j]

			if expected.Err != nil && actual.Err != nil {
				if expected.Err.Error() != actual.Err.Error() {
					t.Errorf("%d.%d. expected err of %s, got %s", i, j, expected.Err, actual.Err)
				}
			} else if expected.Err != actual.Err {
				t.Errorf("%d.%d. expected err of %s, got %s", i, j, expected.Err, actual.Err)
			}

			if len(expected.Sample.Metric) != len(actual.Sample.Metric) {
				t.Errorf("%d.%d. expected length of %s, got %s", i, j, len(expected.Sample.Metric), len(actual.Sample.Metric))

				continue
			}

			for key, value := range expected.Sample.Metric {
				actualValue, ok := actual.Sample.Metric[key]
				if !ok {
					t.Errorf("%d.%d. expected key of %s with %s value, got absent", i, j, key, value)
					continue
				}

				if value != actualValue {
					t.Errorf("%d.%d. expected key of %s with %s value, got %s", i, j, key, value, actualValue)
					continue
				}
			}

			if expected.Sample.Value != actual.Sample.Value {
				t.Errorf("%d.%d. expected value of %s, got %s", i, j, expected.Sample.Value, actual.Sample.Value)
				continue
			}
			// TODO(matt): Test time.
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
