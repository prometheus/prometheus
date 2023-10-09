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
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

func TestTemplateExpansion(t *testing.T) {
	testTemplateExpansion(t, []scenario{
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
			errorMsg:   "error parsing template test: template: test:1: unrecognized character in action: U+00A0",
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
			queryResult: promql.Vector{{T: 0, F: 1.5}},
		},
		{
			// Get value from query.
			text: "{{ query \"metric{instance='a'}\" | first | value }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					T:      0,
					F:      11,
				},
			},
			output: "11",
		},
		{
			// Get label from query.
			text: "{{ query \"metric{instance='a'}\" | first | label \"instance\" }}",

			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					T:      0,
					F:      11,
				},
			},
			output: "a",
		},
		{
			// Get label "__value__" from query.
			text: "{{ query \"metric{__value__='a'}\" | first | strvalue }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "__value__", "a"),
					T:      0,
					F:      11,
				},
			},
			output: "a",
		},
		{
			// Missing label is empty when using label function.
			text: "{{ query \"metric{instance='a'}\" | first | label \"foo\" }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					T:      0,
					F:      11,
				},
			},
			output: "",
		},
		{
			// Missing label is empty when not using label function.
			text: "{{ $x := query \"metric\" | first }}{{ $x.Labels.foo }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					T:      0,
					F:      11,
				},
			},
			output: "",
		},
		{
			text: "{{ $x := query \"metric\" | first }}{{ $x.Labels.foo }}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					T:      0,
					F:      11,
				},
			},
			output: "",
			html:   true,
		},
		{
			// Range over query and sort by label.
			text: "{{ range query \"metric\" | sortByLabel \"instance\" }}{{.Labels.instance}}:{{.Value}}: {{end}}",
			queryResult: promql.Vector{
				{
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "b"),
					T:      0,
					F:      21,
				}, {
					Metric: labels.FromStrings(labels.MetricName, "metric", "instance", "a"),
					T:      0,
					F:      11,
				},
			},
			output: "a:11: b:21: ",
		},
		{
			// Simple hostname.
			text:   "{{ \"foo.example.com\" | stripPort }}",
			output: "foo.example.com",
		},
		{
			// Hostname with port.
			text:   "{{ \"foo.example.com:12345\" | stripPort }}",
			output: "foo.example.com",
		},
		{
			// Simple IPv4 address.
			text:   "{{ \"192.0.2.1\" | stripPort }}",
			output: "192.0.2.1",
		},
		{
			// IPv4 address with port.
			text:   "{{ \"192.0.2.1:12345\" | stripPort }}",
			output: "192.0.2.1",
		},
		{
			// Simple IPv6 address.
			text:   "{{ \"2001:0DB8::1\" | stripPort }}",
			output: "2001:0DB8::1",
		},
		{
			// IPv6 address with port.
			text:   "{{ \"[2001:0DB8::1]:12345\" | stripPort }}",
			output: "2001:0DB8::1",
		},
		{
			// Value can't be split into host and port.
			text:   "{{ \"[2001:0DB8::1]::12345\" | stripPort }}",
			output: "[2001:0DB8::1]::12345",
		},
		{
			// Missing value is no value for nil options.
			text:   "{{ .Foo }}",
			output: "<no value>",
		},
		{
			// Missing value is no value for no options.
			text:    "{{ .Foo }}",
			options: make([]string, 0),
			output:  "<no value>",
		},
		{
			// Assert that missing value returns error with missingkey=error.
			text:       "{{ .Foo }}",
			options:    []string{"missingkey=error"},
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:3: executing "test" at <.Foo>: nil data; no entry for key "Foo"`,
		},
		{
			// Missing value is "" for nil options in ExpandHTML.
			text:   "{{ .Foo }}",
			output: "",
			html:   true,
		},
		{
			// Missing value is "" for no options in ExpandHTML.
			text:    "{{ .Foo }}",
			options: make([]string, 0),
			output:  "",
			html:    true,
		},
		{
			// Assert that missing value returns error with missingkey=error in ExpandHTML.
			text:       "{{ .Foo }}",
			options:    []string{"missingkey=error"},
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:3: executing "test" at <.Foo>: nil data; no entry for key "Foo"`,
			html:       true,
		},
		{
			// Unparsable template.
			text:       "{{",
			shouldFail: true,
			errorMsg:   "error parsing template test: template: test:1: unclosed action",
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
			// Humanize - float64.
			text:   "{{ range . }}{{ humanize . }}:{{ end }}",
			input:  []float64{0.0, 1.0, 1234567.0, .12},
			output: "0:1:1.235M:120m:",
		},
		{
			// Humanize - string.
			text:   "{{ range . }}{{ humanize . }}:{{ end }}",
			input:  []string{"0.0", "1.0", "1234567.0", ".12"},
			output: "0:1:1.235M:120m:",
		},
		{
			// Humanize - string with error.
			text:       `{{ humanize "one" }}`,
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:3: executing "test" at <humanize "one">: error calling humanize: strconv.ParseFloat: parsing "one": invalid syntax`,
		},
		{
			// Humanize - int.
			text:   "{{ range . }}{{ humanize . }}:{{ end }}",
			input:  []int64{0, -1, 1, 1234567, math.MaxInt64},
			output: "0:-1:1:1.235M:9.223E:",
		},
		{
			// Humanize - uint.
			text:   "{{ range . }}{{ humanize . }}:{{ end }}",
			input:  []uint64{0, 1, 1234567, math.MaxUint64},
			output: "0:1:1.235M:18.45E:",
		},
		{
			// Humanize1024 - float64.
			text:   "{{ range . }}{{ humanize1024 . }}:{{ end }}",
			input:  []float64{0.0, 1.0, 1048576.0, .12},
			output: "0:1:1Mi:0.12:",
		},
		{
			// Humanize1024 - string.
			text:   "{{ range . }}{{ humanize1024 . }}:{{ end }}",
			input:  []string{"0.0", "1.0", "1048576.0", ".12"},
			output: "0:1:1Mi:0.12:",
		},
		{
			// Humanize1024 - string with error.
			text:       `{{ humanize1024 "one" }}`,
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:3: executing "test" at <humanize1024 "one">: error calling humanize1024: strconv.ParseFloat: parsing "one": invalid syntax`,
		},
		{
			// Humanize1024 - int.
			text:   "{{ range . }}{{ humanize1024 . }}:{{ end }}",
			input:  []int64{0, -1, 1, 1234567, math.MaxInt64},
			output: "0:-1:1:1.177Mi:8Ei:",
		},
		{
			// Humanize1024 - uint.
			text:   "{{ range . }}{{ humanize1024 . }}:{{ end }}",
			input:  []uint64{0, 1, 1234567, math.MaxUint64},
			output: "0:1:1.177Mi:16Ei:",
		},
		{
			// HumanizeDuration - seconds - float64.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []float64{0, 1, 60, 3600, 86400, 86400 + 3600, -(86400*2 + 3600*3 + 60*4 + 5), 899.99},
			output: "0s:1s:1m 0s:1h 0m 0s:1d 0h 0m 0s:1d 1h 0m 0s:-2d 3h 4m 5s:14m 59s:",
		},
		{
			// HumanizeDuration - seconds - string.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []string{"0", "1", "60", "3600", "86400"},
			output: "0s:1s:1m 0s:1h 0m 0s:1d 0h 0m 0s:",
		},
		{
			// HumanizeDuration - subsecond and fractional seconds - float64.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []float64{.1, .0001, .12345, 60.1, 60.5, 1.2345, 12.345},
			output: "100ms:100us:123.5ms:1m 0s:1m 0s:1.234s:12.35s:",
		},
		{
			// HumanizeDuration - subsecond and fractional seconds - string.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []string{".1", ".0001", ".12345", "60.1", "60.5", "1.2345", "12.345"},
			output: "100ms:100us:123.5ms:1m 0s:1m 0s:1.234s:12.35s:",
		},
		{
			// HumanizeDuration - string with error.
			text:       `{{ humanizeDuration "one" }}`,
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:3: executing "test" at <humanizeDuration "one">: error calling humanizeDuration: strconv.ParseFloat: parsing "one": invalid syntax`,
		},
		{
			// HumanizeDuration - int.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []int{0, -1, 1, 1234567},
			output: "0s:-1s:1s:14d 6h 56m 7s:",
		},
		{
			// HumanizeDuration - uint.
			text:   "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
			input:  []uint{0, 1, 1234567},
			output: "0s:1s:14d 6h 56m 7s:",
		},
		{
			// Humanize* Inf and NaN - float64.
			text:   "{{ range . }}{{ humanize . }}:{{ humanize1024 . }}:{{ humanizeDuration . }}:{{humanizeTimestamp .}}:{{ end }}",
			input:  []float64{math.Inf(1), math.Inf(-1), math.NaN()},
			output: "+Inf:+Inf:+Inf:+Inf:-Inf:-Inf:-Inf:-Inf:NaN:NaN:NaN:NaN:",
		},
		{
			// Humanize* Inf and NaN - string.
			text:   "{{ range . }}{{ humanize . }}:{{ humanize1024 . }}:{{ humanizeDuration . }}:{{humanizeTimestamp .}}:{{ end }}",
			input:  []string{"+Inf", "-Inf", "NaN"},
			output: "+Inf:+Inf:+Inf:+Inf:-Inf:-Inf:-Inf:-Inf:NaN:NaN:NaN:NaN:",
		},
		{
			// HumanizePercentage - model.SampleValue input - float64.
			text:   "{{ -0.22222 | humanizePercentage }}:{{ 0.0 | humanizePercentage }}:{{ 0.1234567 | humanizePercentage }}:{{ 1.23456 | humanizePercentage }}",
			output: "-22.22%:0%:12.35%:123.5%",
		},
		{
			// HumanizePercentage - int.
			text:   "{{ range . }}{{ humanizePercentage . }}:{{ end }}",
			input:  []int64{0, -1, 1, 1234567, math.MaxInt64},
			output: "0%:-100%:100%:1.235e+08%:9.223e+20%:",
		},
		{
			// HumanizePercentage - uint.
			text:   "{{ range . }}{{ humanizePercentage . }}:{{ end }}",
			input:  []uint64{0, 1, 1234567, math.MaxUint64},
			output: "0%:100%:1.235e+08%:1.845e+21%:",
		},
		{
			// HumanizePercentage - model.SampleValue input - string.
			text:   `{{ "-0.22222" | humanizePercentage }}:{{ "0.0" | humanizePercentage }}:{{ "0.1234567" | humanizePercentage }}:{{ "1.23456" | humanizePercentage }}`,
			output: "-22.22%:0%:12.35%:123.5%",
		},
		{
			// HumanizePercentage - model.SampleValue input - string with error.
			text:       `{{ "one" | humanizePercentage }}`,
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:11: executing "test" at <humanizePercentage>: error calling humanizePercentage: strconv.ParseFloat: parsing "one": invalid syntax`,
		},
		{
			// HumanizeTimestamp - int.
			text:   "{{ range . }}{{ humanizeTimestamp . }}:{{ end }}",
			input:  []int64{0, -1, 1, 1234567, 9223372036},
			output: "1970-01-01 00:00:00 +0000 UTC:1969-12-31 23:59:59 +0000 UTC:1970-01-01 00:00:01 +0000 UTC:1970-01-15 06:56:07 +0000 UTC:2262-04-11 23:47:16 +0000 UTC:",
		},
		{
			// HumanizeTimestamp - uint.
			text:   "{{ range . }}{{ humanizeTimestamp . }}:{{ end }}",
			input:  []uint64{0, 1, 1234567, 9223372036},
			output: "1970-01-01 00:00:00 +0000 UTC:1970-01-01 00:00:01 +0000 UTC:1970-01-15 06:56:07 +0000 UTC:2262-04-11 23:47:16 +0000 UTC:",
		},
		{
			// HumanizeTimestamp - int with error.
			text:       "{{ range . }}{{ humanizeTimestamp . }}:{{ end }}",
			input:      []int64{math.MinInt64, math.MaxInt64},
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:16: executing "test" at <humanizeTimestamp .>: error calling humanizeTimestamp: -9.223372036854776e+18 cannot be represented as a nanoseconds timestamp since it overflows int64`,
		},
		{
			// HumanizeTimestamp - uint with error.
			text:       "{{ range . }}{{ humanizeTimestamp . }}:{{ end }}",
			input:      []uint64{math.MaxUint64},
			shouldFail: true,
			errorMsg:   `error executing template test: template: test:1:16: executing "test" at <humanizeTimestamp .>: error calling humanizeTimestamp: 1.8446744073709552e+19 cannot be represented as a nanoseconds timestamp since it overflows int64`,
		},
		{
			// HumanizeTimestamp - model.SampleValue input - float64.
			text:   "{{ 1435065584.128 | humanizeTimestamp }}",
			output: "2015-06-23 13:19:44.128 +0000 UTC",
		},
		{
			// HumanizeTimestamp - model.SampleValue input - string.
			text:   `{{ "1435065584.128" | humanizeTimestamp }}`,
			output: "2015-06-23 13:19:44.128 +0000 UTC",
		},
		{
			// ToTime - model.SampleValue input - float64.
			text:   `{{ (1435065584.128 | toTime).Format "2006" }}`,
			output: "2015",
		},
		{
			// ToTime - model.SampleValue input - string.
			text:   `{{ ("1435065584.128" | toTime).Format "2006" }}`,
			output: "2015",
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
		{
			// parseDuration (using printf to ensure the return is a string).
			text:   "{{ printf \"%0.2f\" (parseDuration \"1h2m10ms\") }}",
			output: "3720.01",
		},
		{
			// Simple hostname.
			text:   "{{ \"foo.example.com\" | stripDomain }}",
			output: "foo",
		},
		{
			// Hostname with port.
			text:   "{{ \"foo.example.com:12345\" | stripDomain }}",
			output: "foo:12345",
		},
		{
			// Simple IPv4 address.
			text:   "{{ \"192.0.2.1\" | stripDomain }}",
			output: "192.0.2.1",
		},
		{
			// IPv4 address with port.
			text:   "{{ \"192.0.2.1:12345\" | stripDomain }}",
			output: "192.0.2.1:12345",
		},
		{
			// Simple IPv6 address.
			text:   "{{ \"2001:0DB8::1\" | stripDomain }}",
			output: "2001:0DB8::1",
		},
		{
			// IPv6 address with port.
			text:   "{{ \"[2001:0DB8::1]:12345\" | stripDomain }}",
			output: "[2001:0DB8::1]:12345",
		},
		{
			// Value can't be split into host and port.
			text:   "{{ \"[2001:0DB8::1]::12345\" | stripDomain }}",
			output: "[2001:0DB8::1]::12345",
		},
	})
}

type scenario struct {
	text        string
	output      string
	input       interface{}
	options     []string
	queryResult promql.Vector
	shouldFail  bool
	html        bool
	errorMsg    string
}

func testTemplateExpansion(t *testing.T, scenarios []scenario) {
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
		expander := NewTemplateExpander(context.Background(), s.text, "test", s.input, 0, queryFunc, extURL, s.options)
		if s.html {
			result, err = expander.ExpandHTML(nil)
		} else {
			result, err = expander.Expand()
		}
		if s.shouldFail {
			require.Error(t, err, "%v", s.text)
			require.EqualError(t, err, s.errorMsg)
			continue
		}

		require.NoError(t, err)

		if err == nil {
			require.Equal(t, s.output, result)
		}
	}
}

func Test_floatToTime(t *testing.T) {
	type args struct {
		v float64
	}
	tests := []struct {
		name    string
		args    args
		want    *time.Time
		wantErr bool
	}{
		{
			"happy path",
			args{
				v: 1657155181,
			},
			func() *time.Time {
				tm := time.Date(2022, 7, 7, 0, 53, 1, 0, time.UTC)
				return &tm
			}(),
			false,
		},
		{
			"more than math.MaxInt64",
			args{
				v: 1.79769313486231570814527423731704356798070e+300,
			},
			nil,
			true,
		},
		{
			"less than math.MinInt64",
			args{
				v: -1.79769313486231570814527423731704356798070e+300,
			},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := floatToTime(tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("floatToTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("floatToTime() got = %v, want %v", got, tt.want)
			}
		})
	}
}
