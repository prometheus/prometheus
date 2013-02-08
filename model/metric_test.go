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

package model

import (
	"github.com/prometheus/prometheus/utility/test"
	"testing"
)

func testMetric(t test.Tester) {
	var scenarios = []struct {
		input  map[string]string
		output Fingerprint
	}{
		{
			input:  map[string]string{},
			output: "d41d8cd98f00b204e9800998ecf8427e",
		},
		{
			input: map[string]string{
				"first_name":   "electro",
				"occupation":   "robot",
				"manufacturer": "westinghouse",
			},
			output: "18596f03fce001153495d903b8b577c0",
		},
	}

	for i, scenario := range scenarios {
		metric := Metric{}
		for key, value := range scenario.input {
			metric[LabelName(key)] = LabelValue(value)
		}

		expected := scenario.output
		actual := metric.Fingerprint()

		if expected != actual {
			t.Errorf("%d. expected %s, got %s", i, expected, actual)
		}
	}
}

func TestMetric(t *testing.T) {
	testMetric(t)
}

func BenchmarkMetric(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testMetric(b)
	}
}
