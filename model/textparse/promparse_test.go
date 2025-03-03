// Copyright 2017 The Prometheus Authors
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

package textparse

import (
	"io"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestPromParse(t *testing.T) {
	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# 	TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25",} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds{quantile="0.8", a="b"} 8.3835e-05
go_gc_duration_seconds{ quantile="0.9", a="b"} 8.3835e-05
# HELP prometheus_http_request_duration_seconds Histogram of latencies for HTTP requests.
# TYPE prometheus_http_request_duration_seconds histogram
prometheus_http_request_duration_seconds_bucket{handler="/",le="1"} 423
prometheus_http_request_duration_seconds_bucket{handler="/",le="2"} 1423
prometheus_http_request_duration_seconds_bucket{handler="/",le="+Inf"} 1423
prometheus_http_request_duration_seconds_sum{handler="/"} 2000
prometheus_http_request_duration_seconds_count{handler="/"} 1423
# Hrandom comment starting with prefix of HELP
#
wind_speed{A="2",c="3"} 12345
# comment with escaped \n newline
# comment with escaped \ escape character
# HELP nohelp1
# HELP nohelp2 
go_gc_duration_seconds{ quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile= "1.0", a= "b", } 8.3835e-05
go_gc_duration_seconds { quantile = "1.0", a = "b" } 8.3835e-05
go_gc_duration_seconds { quantile = "2.0" a = "b" } 8.3835e-05
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"}	1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33  	123123
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1
testmetric{le="10"} 1`
	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1"

	exp := []parsedEntry{
		{
			m:    "go_gc_duration_seconds",
			help: "A summary of the GC invocation durations.",
		}, {
			m:   "go_gc_duration_seconds",
			typ: model.MetricTypeSummary,
		}, {
			m:    `go_gc_duration_seconds{quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.0"),
		}, {
			m:    `go_gc_duration_seconds{quantile="0.25",}`,
			v:    7.424100000000001e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.25"),
		}, {
			m:    `go_gc_duration_seconds{quantile="0.5",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.5", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds{quantile="0.8", a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.8", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds{ quantile="0.9", a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.9", "a", "b"),
		}, {
			m:    "prometheus_http_request_duration_seconds",
			help: "Histogram of latencies for HTTP requests.",
		}, {
			m:   "prometheus_http_request_duration_seconds",
			typ: model.MetricTypeHistogram,
		}, {
			m:    `prometheus_http_request_duration_seconds_bucket{handler="/",le="1"}`,
			v:    423,
			lset: labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "handler", "/", "le", "1.0"),
		}, {
			m:    `prometheus_http_request_duration_seconds_bucket{handler="/",le="2"}`,
			v:    1423,
			lset: labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "handler", "/", "le", "2.0"),
		}, {
			m:    `prometheus_http_request_duration_seconds_bucket{handler="/",le="+Inf"}`,
			v:    1423,
			lset: labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "handler", "/", "le", "+Inf"),
		}, {
			m:    `prometheus_http_request_duration_seconds_sum{handler="/"}`,
			v:    2000,
			lset: labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_sum", "handler", "/"),
		}, {
			m:    `prometheus_http_request_duration_seconds_count{handler="/"}`,
			v:    1423,
			lset: labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_count", "handler", "/"),
		}, {
			comment: "# Hrandom comment starting with prefix of HELP",
		}, {
			comment: "#",
		}, {
			m:    `wind_speed{A="2",c="3"}`,
			v:    12345,
			lset: labels.FromStrings("A", "2", "__name__", "wind_speed", "c", "3"),
		}, {
			comment: "# comment with escaped \\n newline",
		}, {
			comment: "# comment with escaped \\ escape character",
		}, {
			m:    "nohelp1",
			help: "",
		}, {
			m:    "nohelp2",
			help: "",
		}, {
			m:    `go_gc_duration_seconds{ quantile="1.0", a="b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds { quantile="1.0", a="b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds { quantile= "1.0", a= "b", }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds { quantile = "1.0", a = "b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			// NOTE: Unlike OpenMetrics, PromParser allows spaces between label terms. This appears to be unintended and should probably be fixed.
			m:    `go_gc_duration_seconds { quantile = "2.0" a = "b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "2.0", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds_count`,
			v:    99,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds_count"),
		}, {
			m:    `some:aggregate:rate5m{a_b="c"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "some:aggregate:rate5m", "a_b", "c"),
		}, {
			m:    "go_goroutines",
			help: "Number of goroutines that currently exist.",
		}, {
			m:   "go_goroutines",
			typ: model.MetricTypeGauge,
		}, {
			m:    `go_goroutines`,
			v:    33,
			t:    int64p(123123),
			lset: labels.FromStrings("__name__", "go_goroutines"),
		}, {
			m:    "_metric_starting_with_underscore",
			v:    1,
			lset: labels.FromStrings("__name__", "_metric_starting_with_underscore"),
		}, {
			m:    "testmetric{_label_starting_with_underscore=\"foo\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "testmetric", "_label_starting_with_underscore", "foo"),
		}, {
			m:    "testmetric{label=\"\\\"bar\\\"\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "testmetric", "label", `"bar"`),
		}, {
			m:    `testmetric{le="10"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "testmetric", "le", "10"),
		}, {
			m:    "metric",
			help: "foo\x00bar",
		}, {
			m:    "null_byte_metric{a=\"abc\x00\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"),
		},
	}

	p := NewPromParser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestUTF8PromParse(t *testing.T) {
	input := `# HELP "go.gc_duration_seconds" A summary of the GC invocation durations.
# 	TYPE "go.gc_duration_seconds" summary
{"go.gc_duration_seconds",quantile="0"} 4.9351e-05
{"go.gc_duration_seconds",quantile="0.25",} 7.424100000000001e-05
{"go.gc_duration_seconds",quantile="0.5",a="b"} 8.3835e-05
{"go.gc_duration_seconds",quantile="0.8", a="b"} 8.3835e-05
{"go.gc_duration_seconds", quantile="0.9", a="b"} 8.3835e-05
{"go.gc_duration_seconds", quantile="1.0", a="b" } 8.3835e-05
{ "go.gc_duration_seconds", quantile="1.0", a="b" } 8.3835e-05
{ "go.gc_duration_seconds", quantile= "1.0", a= "b", } 8.3835e-05
{ "go.gc_duration_seconds", quantile = "1.0", a = "b" } 8.3835e-05
{"go.gc_duration_seconds_count"} 99
{"Heizölrückstoßabdämpfung 10€ metric with \"interesting\" {character\nchoices}","strange©™\n'quoted' \"name\""="6"} 10.0`

	exp := []parsedEntry{
		{
			m:    "go.gc_duration_seconds",
			help: "A summary of the GC invocation durations.",
		}, {
			m:   "go.gc_duration_seconds",
			typ: model.MetricTypeSummary,
		}, {
			m:    `{"go.gc_duration_seconds",quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.0"),
		}, {
			m:    `{"go.gc_duration_seconds",quantile="0.25",}`,
			v:    7.424100000000001e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.25"),
		}, {
			m:    `{"go.gc_duration_seconds",quantile="0.5",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.5", "a", "b"),
		}, {
			m:    `{"go.gc_duration_seconds",quantile="0.8", a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.8", "a", "b"),
		}, {
			m:    `{"go.gc_duration_seconds", quantile="0.9", a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.9", "a", "b"),
		}, {
			m:    `{"go.gc_duration_seconds", quantile="1.0", a="b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `{ "go.gc_duration_seconds", quantile="1.0", a="b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `{ "go.gc_duration_seconds", quantile= "1.0", a= "b", }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `{ "go.gc_duration_seconds", quantile = "1.0", a = "b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `{"go.gc_duration_seconds_count"}`,
			v:    99,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds_count"),
		}, {
			m: `{"Heizölrückstoßabdämpfung 10€ metric with \"interesting\" {character\nchoices}","strange©™\n'quoted' \"name\""="6"}`,
			v: 10.0,
			lset: labels.FromStrings("__name__", `Heizölrückstoßabdämpfung 10€ metric with "interesting" {character
choices}`, "strange©™\n'quoted' \"name\"", "6"),
		},
	}

	p := NewPromParser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestPromParseErrors(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		{
			input: "a",
			err:   "expected value after metric, got \"\\n\" (\"INVALID\") while parsing: \"a\\n\"",
		},
		{
			input: "a{b='c'} 1\n",
			err:   "expected label value, got \"'\" (\"INVALID\") while parsing: \"a{b='\"",
		},
		{
			input: "a{b=\n",
			err:   "expected label value, got \"\\n\" (\"INVALID\") while parsing: \"a{b=\\n\"",
		},
		{
			input: "a{\xff=\"foo\"} 1\n",
			err:   "expected label name, got \"\\xff\" (\"INVALID\") while parsing: \"a{\\xff\"",
		},
		{
			input: "a{b=\"\xff\"} 1\n",
			err:   "invalid UTF-8 label value: \"\\\"\\xff\\\"\"",
		},
		{
			input: `{"a", "b = "c"}`,
			err:   "expected equal, got \"c\\\"\" (\"LNAME\") while parsing: \"{\\\"a\\\", \\\"b = \\\"c\\\"\"",
		},
		{
			input: `{"a",b\nc="d"} 1`,
			err:   "expected equal, got \"\\\\\" (\"INVALID\") while parsing: \"{\\\"a\\\",b\\\\\"",
		},
		{
			input: "a true\n",
			err:   "strconv.ParseFloat: parsing \"true\": invalid syntax while parsing: \"a true\"",
		},
		{
			input: "something_weird{problem=\"",
			err:   "expected label value, got \"\\\"\\n\" (\"INVALID\") while parsing: \"something_weird{problem=\\\"\\n\"",
		},
		{
			input: "empty_label_name{=\"\"} 0",
			err:   "expected label name, got \"=\\\"\" (\"EQUAL\") while parsing: \"empty_label_name{=\\\"\"",
		},
		{
			input: "foo 1_2\n",
			err:   "unsupported character in float while parsing: \"foo 1_2\"",
		},
		{
			input: "foo 0x1p-3\n",
			err:   "unsupported character in float while parsing: \"foo 0x1p-3\"",
		},
		{
			input: "foo 0x1P-3\n",
			err:   "unsupported character in float while parsing: \"foo 0x1P-3\"",
		},
		{
			input: "foo 0 1_2\n",
			err:   "expected next entry after timestamp, got \"_\" (\"INVALID\") while parsing: \"foo 0 1_\"",
		},
		{
			input: `{a="ok"} 1`,
			err:   "metric name not set while parsing: \"{a=\\\"ok\\\"} 1\"",
		},
		{
			input: "# TYPE #\n#EOF\n",
			err:   "expected metric name after TYPE, got \"#\" (\"INVALID\") while parsing: \"# TYPE #\"",
		},
		{
			input: "# HELP #\n#EOF\n",
			err:   "expected metric name after HELP, got \"#\" (\"INVALID\") while parsing: \"# HELP #\"",
		},
	}

	for i, c := range cases {
		p := NewPromParser([]byte(c.input), labels.NewSymbolTable())
		var err error
		for err == nil {
			_, err = p.Next()
		}
		require.EqualError(t, err, c.err, "test %d", i)
	}
}

func TestPromNullByteHandling(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		{
			input: "null_byte_metric{a=\"abc\x00\"} 1",
			err:   "",
		},
		{
			input: "a{b=\"\x00ss\"} 1\n",
			err:   "",
		},
		{
			input: "a{b=\"\x00\"} 1\n",
			err:   "",
		},
		{
			input: "a{b=\"\x00\"} 1\n",
			err:   "",
		},
		{
			input: "a{b=\x00\"ssss\"} 1\n",
			err:   "expected label value, got \"\\x00\" (\"INVALID\") while parsing: \"a{b=\\x00\"",
		},
		{
			input: "a{b=\"\x00",
			err:   "expected label value, got \"\\\"\\x00\\n\" (\"INVALID\") while parsing: \"a{b=\\\"\\x00\\n\"",
		},
		{
			input: "a{b\x00=\"hiih\"}	1",
			err:   "expected equal, got \"\\x00\" (\"INVALID\") while parsing: \"a{b\\x00\"",
		},
		{
			input: "a\x00{b=\"ddd\"} 1",
			err:   "expected value after metric, got \"\\x00\" (\"INVALID\") while parsing: \"a\\x00\"",
		},
		{
			input: "a 0 1\x00",
			err:   "expected next entry after timestamp, got \"\\x00\" (\"INVALID\") while parsing: \"a 0 1\\x00\"",
		},
	}

	for i, c := range cases {
		p := NewPromParser([]byte(c.input), labels.NewSymbolTable())
		var err error
		for err == nil {
			_, err = p.Next()
		}

		if c.err == "" {
			require.Equal(t, io.EOF, err, "test %d", i)
			continue
		}

		require.EqualError(t, err, c.err, "test %d", i)
	}
}
