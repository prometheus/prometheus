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
	"context"
	"math"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestTemplateExpansion(t *testing.T) {
	scenarios := []struct {
		text        string
		output      string
		input       interface{}
		queryResult promql.Vector
		shouldFail  bool
		html        bool
		errorMsg    string
	}{
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
			// Non-ASCII space (not allowed in text/template, see https://github.com/golang/go/blob/master/src/text/template/parse/lex.go#L98)
			text:       "{{ }}",
			shouldFail: true,
			errorMsg:   "error parsing template test: template: test:1: unexpected unrecognized character in action: U+00A0 in command",
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
			text:        "{{ query \"1.5\" | first | value }}",
			output:      "1.5",
			queryResult: promql.Vector{{Point: promql.Point{T: 0, V: 1.5}}},
		},
		{
			// Get value from query.
			text: "{{ query \"metric{instance='a'}\" | first | value }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					Point:  promql.Point{T: 0, V: 11},
				}},
			output: "11",
		},
		{
			// Get label from query.
			text: "{{ query \"metric{instance='a'}\" | first | label \"instance\" }}",

			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					Point:  promql.Point{T: 0, V: 11},
				}},
			output: "a",
		},
		{
			// Missing label is empty when using label function.
			text: "{{ query \"metric{instance='a'}\" | first | label \"foo\" }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					Point:  promql.Point{T: 0, V: 11},
				}},
			output: "",
		},
		{
			// Missing label is empty when not using label function.
			text: "{{ $x := query \"metric\" | first }}{{ $x.Labels.foo }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					Point:  promql.Point{T: 0, V: 11},
				}},
			output: "",
		},
		{
			text: "{{ $x := query \"metric\" | first }}{{ $x.Labels.foo }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					Point:  promql.Point{T: 0, V: 11},
				}},
			output: "",
			html:   true,
		},
		{
			// Range over query and sort by label.
			text: "{{ range query \"metric\" | sortByLabel \"instance\" }}{{.Labels.instance}}:{{.Value}}: {{end}}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					Point:  promql.Point{T: 0, V: 11},
				}, {
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "b"),
					Point:  promql.Point{T: 0, V: 21},
				}},
			output: "a:11: b:21: ",
		},
		{
			// Unparsable template.
			text:       "{{",
			shouldFail: true,
			errorMsg:   "error parsing template test: template: test:1: unexpected unclosed action in command",
		},
		{
			// Error in function.
			text:        "{{ query \"missing\" | first }}",
			queryResult: promql.Vector{},
			shouldFail:  true,
			errorMsg:    "error executing template test: template: test:1:21: executing \"test\" at <first>: error calling first: first() called on vector with no elements",
		},
		{
			// Panic.
			text:        "{{ (query \"missing\").banana }}",
			queryResult: promql.Vector{},
			shouldFail:  true,
			errorMsg:    "error executing template test: template: test:1:10: executing \"test\" at <\"missing\">: can't evaluate field banana in type template.queryResult",
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
			// HumanizePercentage - model.SampleValue input.
			text:   "{{ -0.22222 | humanizePercentage }}:{{ 0.0 | humanizePercentage }}:{{ 0.1234567 | humanizePercentage }}:{{ 1.23456 | humanizePercentage }}",
			output: "-22.22%:0%:12.35%:123.5%",
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
		{
			// pathPrefix.
			text:   "{{ pathPrefix }}",
			output: "/path/prefix",
		},
		{
			// externalURL.
			text:   "{{ externalURL }}",
			output: "http://testhost:9090/path/prefix",
		},
	}

	extURL, err := url.Parse("http://testhost:9090/path/prefix")
	if err != nil {
		panic(err)
	}

	for _, s := range scenarios {
		queryFunc := func(_ context.Context, _ string, _ time.Time) (promql.Vector, error) {
			return s.queryResult, nil
		}
		var result string
		var err error
		expander := NewTemplateExpander(context.Background(), s.text, "test", s.input, 0, queryFunc, extURL)
		if s.html {
			result, err = expander.ExpandHTML(nil)
		} else {
			result, err = expander.Expand()
		}
		if s.shouldFail {
			testutil.NotOk(t, err, "%v", s.text)
			continue
		}

		testutil.Ok(t, err)

		if err == nil {
			testutil.Equals(t, result, s.output)
		}
	}
}
