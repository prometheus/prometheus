// Copyright 2014 Prometheus Team
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

package templates

import (
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric/tiered"
)

type testTemplatesScenario struct {
	text   string
	output string
}

func TestTemplateExpansion(t *testing.T) {
	scenarios := []testTemplatesScenario{
		{
			// No template.
			text:   "plain text",
			output: "plain text",
		},
		{
			// Simple value.
			text:   "{{ 1 }}",
			output: "1",
		},
		{
			// Simple value.
			text:   "{{ 1 }}",
			output: "1",
		},
		{
			// Get value from query.
			text:   "{{ query \"metric{instance='a'}\" | first | value }}",
			output: "11",
		},
		{
			// Get label from query.
			text:   "{{ query \"metric{instance='a'}\" | first | label \"instance\" }}",
			output: "a",
		},
		{
			// Range over query.
			text:   "{{ range query \"metric\" }}{{.Labels.instance}}:{{.Value}}: {{end}}",
			output: "a:11: b:21: ",
		},
	}

	time := clientmodel.Timestamp(0)

	ts, _ := tiered.NewTestTieredStorage(t)
	ts.AppendSamples(clientmodel.Samples{
		{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "metric",
				"instance":                  "a"},
			Value: 11,
		},
		{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "metric",
				"instance":                  "b"},
			Value: 21,
		},
	})

	for _, s := range scenarios {
		result, err := Expand(s.text, "test", nil, time, ts)
		if err != nil {
			t.Fatalf("Error returned from %v: %v", s.text, err)
			continue
		}
		if result != s.output {
			t.Fatalf("Error in result from %v: Expected '%v' Got '%v'", s.text, s.output, result)
			continue
		}
	}
}
