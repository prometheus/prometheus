// Copyright 2014 The Prometheus Authors
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

package template

import (
	"math"
	"testing"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
)

type testTemplatesScenario struct {
	text       string
	output     string
	input      interface{}
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
			text:   "{{ query \"1.5\" | first | value }}",
			output: "1.5",
		},
		{
			// Get value from scalar query.
			text:   "{{ query \"scalar(count(metric))\" | first | value }}",
			output: "2",
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
			// Missing label is empty when using label function.
			text:   "{{ query \"metric{instance='a'}\" | first | label \"foo\" }}",
			output: "",
		},
		{
			// Missing label is empty when not using label function.
			text:   "{{ $x := query \"metric\" | first }}{{ $x.Labels.foo }}",
			output: "",
		},
		{
			text:   "{{ $x := query \"metric\" | first }}{{ $x.Labels.foo }}",
			output: "",
			html:   true,
		},
		{
			// Range over query and sort by label.
			text:   "{{ range query \"metric\" | sortByLabel \"instance\" }}{{.Labels.instance}}:{{.Value}}: {{end}}",
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
			// Humanize.
			text:   "{{ range . }}{{ humanize . }}:{{ end }}",
			input:  []float64{0.0, 1.0, 1234567.0, .12},
			output: "0:1:1.235M:120m:",
		},
		{
			// Humanize1024.
			text:   "{{ range . }}{{ humanize1024 . }}:{{ end }}",
			input:  []float64{0.0, 1.0, 1048576.0, .12},
			output: "0:1:1Mi:0.12:",
		},
		{
			// HumanizeDuration - seconds.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []float64{0, 1, 60, 3600, 86400, 86400 + 3600, -(86400*2 + 3600*3 + 60*4 + 5), 899.99},
			output: "0s:1s:1m 0s:1h 0m 0s:1d 0h 0m 0s:1d 1h 0m 0s:-2d 3h 4m 5s:14m 59s:",
		},
		{
			// HumanizeDuration - subsecond and fractional seconds.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []float64{.1, .0001, .12345, 60.1, 60.5, 1.2345, 12.345},
			output: "100ms:100us:123.5ms:1m 0s:1m 0s:1.234s:12.35s:",
		},
		{
			// Humanize* Inf and NaN.
			text:   "{{ range . }}{{ humanize . }}:{{ humanize1024 . }}:{{ humanizeDuration . }}:{{humanizeTimestamp .}}:{{ end }}",
			input:  []float64{math.Inf(1), math.Inf(-1), math.NaN()},
			output: "+Inf:+Inf:+Inf:+Inf:-Inf:-Inf:-Inf:-Inf:NaN:NaN:NaN:NaN:",
		},
		{
			// HumanizeTimestamp - model.SampleValue input.
			text:   "{{ 1435065584.128 | humanizeTimestamp }}",
			output: "2015-06-23 13:19:44.128 +0000 UTC",
		},
		{
			// Title.
			text:   "{{ \"aa bb CC\" | title }}",
			output: "Aa Bb CC",
		},
		{
			// toUpper.
			text:   "{{ \"aa bb CC\" | toUpper }}",
			output: "AA BB CC",
		},
		{
			// toLower.
			text:   "{{ \"aA bB CC\" | toLower }}",
			output: "aa bb cc",
		},
		{
			// Match.
			text:   "{{ match \"a+\" \"aa\" }} {{ match \"a+\" \"b\" }}",
			output: "true false",
		},
		{
			// graphLink.
			text:   "{{ graphLink \"up\" }}",
			output: "/graph?g0.expr=up&g0.tab=0",
		},
		{
			// tableLink.
			text:   "{{ tableLink \"up\" }}",
			output: "/graph?g0.expr=up&g0.tab=1",
		},
		{
			// tmpl.
			text:   "{{ define \"a\" }}x{{ end }}{{ $name := \"a\"}}{{ tmpl $name . }}",
			output: "x",
			html:   true,
		},
	}

	time := model.Time(0)

	storage, closer := local.NewTestStorage(t, 2)
	defer closer.Close()
	storage.Append(&model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: "metric",
			"instance":            "a"},
		Value: 11,
	})
	storage.Append(&model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: "metric",
			"instance":            "b"},
		Value: 21,
	})
	storage.WaitForIndexing()

	engine := promql.NewEngine(storage, nil)

	for i, s := range scenarios {
		var result string
		var err error
		expander := NewTemplateExpander(context.Background(), s.text, "test", s.input, time, engine, "")
		if s.html {
			result, err = expander.ExpandHTML(nil)
		} else {
			result, err = expander.Expand()
		}
		if s.shouldFail {
			if err == nil {
				t.Fatalf("%d. Error not returned from %v", i, s.text)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%d. Error returned from %v: %v", i, s.text, err)
			continue
		}
		if result != s.output {
			t.Fatalf("%d. Error in result from %v: Expected '%v' Got '%v'", i, s.text, s.output, result)
			continue
		}
	}
}
