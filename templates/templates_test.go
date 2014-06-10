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
	text       string
	output     string
	shouldFail bool
	html       bool
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
			// HTML escaping.
			text:   "{{ \"<b>\" }}",
			output: "&lt;b&gt;",
			html:   true,
		},
		{
			// Disabling HTML escaping.
			text:   "{{ \"<b>\" | safeHtml }}",
			output: "<b>",
			html:   true,
		},
		{
			// HTML escaping doesn't apply to non-html.
			text:   "{{ \"<b>\" }}",
			output: "<b>",
		},
		{
			// Pass multiple arguments to templates.
			text:   "{{define \"x\"}}{{.arg0}} {{.arg1}}{{end}}{{template \"x\" (args 1 \"2\")}}",
			output: "1 2",
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
		{
			// Unparsable template.
			text:       "{{",
			shouldFail: true,
		},
		{
			// Error in function.
			text:       "{{ query \"missing\" | first }}",
			shouldFail: true,
		},
		{
			// Panic.
			text:       "{{ (query \"missing\").banana }}",
			shouldFail: true,
		},
		{
			// Regex replacement.
			text:   "{{ reReplaceAll \"(a)b\" \"x$1\" \"ab\" }}",
			output: "xa",
		},
		{
			// Sorting.
			text:   "{{ range query \"metric\" | sortByLabel \"instance\" }}{{.Labels.instance}} {{end}}",
			output: "a b ",
		},
		{
			// Humanize.
			text:   "{{ 0.0 | humanize }}:{{ 1.0 | humanize }}:{{ 1234567.0 | humanize }}:{{ .12 | humanize }}",
			output: "0 :1 :1.235 M:120 m",
		},
		{
			// Humanize1024.
			text:   "{{ 0.0 | humanize1024 }}:{{ 1.0 | humanize1024 }}:{{ 1048576.0 | humanize1024 }}:{{ .12 | humanize1024}}",
			output: "0 :1 :1 Mi:0.12 ",
		},
		{
			// Title.
			text:   "{{ \"aa bb CC\" | title }}",
			output: "Aa Bb CC",
		},
		{
			// Match.
			text:   "{{ match \"a+\" \"aa\" }} {{ match \"a+\" \"b\" }}",
			output: "true false",
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
		var result string
		var err error
		expander := NewTemplateExpander(s.text, "test", nil, time, ts)
		if s.html {
			result, err = expander.ExpandHtml(nil)
		} else {
			result, err = expander.Expand()
		}
		if s.shouldFail {
			if err == nil {
				t.Fatalf("Error not returned from %v", s.text)
			}
			continue
		}
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
