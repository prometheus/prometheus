// Copyright The Prometheus Authors
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
	"fmt"
	"io"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

// lbls is a helper for the readability of the expectations.
func typeAndUnitLabels(typeAndUnitEnabled bool, enabled, disabled labels.Labels) labels.Labels {
	if typeAndUnitEnabled {
		return enabled
	}
	return disabled
}

// todoDetectFamilySwitch exists because there's a known TODO that require dedicated PR and benchmarks for PROM-39.
// OM and Prom text format do NOT require TYPE, HELP or UNIT lines. This means that metric families can switch without
// those metadata entries e.g.:
// ```
// TYPE go_goroutines gauge
// go_goroutines 33 # previous metric
// different_metric_total 12 # <--- different family!
// ```
// The expected type for "different_metric_total" is obviously unknown type and unit, but it's surprisingly expensive and complex
// to reliably write parser for those cases. Two main issues:
// a. TYPE and UNIT are associated with "metric family" which is different than resulting metric name (e.g. histograms).
// b. You have to alloc additional entries to pair TYPE and UNIT with metric families they refer to (nit)
//
// This problem is elevated for PROM-39 feature.
//
// Current metadata handling is semi broken here for this as the (a) is expensive and currently not fully accurate
// see: https://github.com/prometheus/prometheus/blob/dbf5d01a62249eddcd202303069f6cf7dd3c4a73/scrape/scrape.go#L1916
//
// To iterate, we keep it "knowingly" broken behind the feature flag.
// TODO(bwplotka): Remove this once we fix the problematic case e.g.
//   - introduce more accurate isSeriesPartOfFamily shared helper or even parser method that tells when new metric family starts
func todoDetectFamilySwitch(typeAndUnitEnabled bool, expected labels.Labels, brokenTypeInherited model.MetricType) labels.Labels {
	if typeAndUnitEnabled && brokenTypeInherited != model.MetricTypeUnknown {
		// Hack for now.
		b := labels.NewBuilder(expected)
		b.Set("__type__", string(brokenTypeInherited))
		return b.Labels()
	}
	return expected
}

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
# TYPE some_counter_total counter
# HELP some_counter_total Help after type.
some_counter_total 12
# HELP nohelp3
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1
testmetric{le="10"} 1
# HELP type_and_unit_test1 Type specified in metadata overrides.
# TYPE type_and_unit_test1 gauge
type_and_unit_test1{__type__="counter"} 123
# HELP type_and_unit_test2 Type specified in label.
type_and_unit_test2{__type__="counter"} 123`
	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1"

	for _, typeAndUnitEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("type-and-unit=%v", typeAndUnitEnabled), func(t *testing.T) {
			exp := []parsedEntry{
				{
					m:    "go_gc_duration_seconds",
					help: "A summary of the GC invocation durations.",
				},
				{
					m:   "go_gc_duration_seconds",
					typ: model.MetricTypeSummary,
				},
				{
					m: `go_gc_duration_seconds{quantile="0"}`,
					v: 4.9351e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "quantile", "0.0"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.0"),
					),
				},
				{
					m: `go_gc_duration_seconds{quantile="0.25",}`,
					v: 7.424100000000001e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "quantile", "0.25"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.25"),
					),
				},
				{
					m: `go_gc_duration_seconds{quantile="0.5",a="b"}`,
					v: 8.3835e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "quantile", "0.5", "a", "b"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.5", "a", "b"),
					),
				},
				{
					m: `go_gc_duration_seconds{quantile="0.8", a="b"}`,
					v: 8.3835e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "quantile", "0.8", "a", "b"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.8", "a", "b"),
					),
				},
				{
					m: `go_gc_duration_seconds{ quantile="0.9", a="b"}`,
					v: 8.3835e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "quantile", "0.9", "a", "b"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.9", "a", "b"),
					),
				},
				{
					m:    "prometheus_http_request_duration_seconds",
					help: "Histogram of latencies for HTTP requests.",
				},
				{
					m:   "prometheus_http_request_duration_seconds",
					typ: model.MetricTypeHistogram,
				},
				{
					m: `prometheus_http_request_duration_seconds_bucket{handler="/",le="1"}`,
					v: 423,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "__type__", string(model.MetricTypeHistogram), "handler", "/", "le", "1.0"),
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "handler", "/", "le", "1.0"),
					),
				},
				{
					m: `prometheus_http_request_duration_seconds_bucket{handler="/",le="2"}`,
					v: 1423,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "__type__", string(model.MetricTypeHistogram), "handler", "/", "le", "2.0"),
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "handler", "/", "le", "2.0"),
					),
				},
				{
					m: `prometheus_http_request_duration_seconds_bucket{handler="/",le="+Inf"}`,
					v: 1423,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "__type__", string(model.MetricTypeHistogram), "handler", "/", "le", "+Inf"),
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_bucket", "handler", "/", "le", "+Inf"),
					),
				},
				{
					m: `prometheus_http_request_duration_seconds_sum{handler="/"}`,
					v: 2000,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_sum", "__type__", string(model.MetricTypeHistogram), "handler", "/"),
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_sum", "handler", "/"),
					),
				},
				{
					m: `prometheus_http_request_duration_seconds_count{handler="/"}`,
					v: 1423,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_count", "__type__", string(model.MetricTypeHistogram), "handler", "/"),
						labels.FromStrings("__name__", "prometheus_http_request_duration_seconds_count", "handler", "/"),
					),
				},
				{
					comment: "# Hrandom comment starting with prefix of HELP",
				},
				{
					comment: "#",
				},
				{
					m: `wind_speed{A="2",c="3"}`,
					v: 12345,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						// NOTE(bwplotka): This is knowingly broken, inheriting old type when TYPE was not specified on a new metric.
						// This was broken forever on a case for a broken exposition. Don't fix for now (expensive).
						labels.FromStrings("A", "2", "__name__", "wind_speed", "__type__", string(model.MetricTypeHistogram), "c", "3"),
						labels.FromStrings("A", "2", "__name__", "wind_speed", "c", "3"),
					),
				},
				{
					comment: "# comment with escaped \\n newline",
				},
				{
					comment: "# comment with escaped \\ escape character",
				},
				{
					m:    "nohelp1",
					help: "",
				},
				{
					m:    "nohelp2",
					help: "",
				},
				{
					m:    `go_gc_duration_seconds{ quantile="1.0", a="b" }`,
					v:    8.3835e-05,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"), model.MetricTypeHistogram),
				},
				{
					m:    `go_gc_duration_seconds { quantile="1.0", a="b" }`,
					v:    8.3835e-05,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"), model.MetricTypeHistogram),
				},
				{
					m:    `go_gc_duration_seconds { quantile= "1.0", a= "b", }`,
					v:    8.3835e-05,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"), model.MetricTypeHistogram),
				},
				{
					m:    `go_gc_duration_seconds { quantile = "1.0", a = "b" }`,
					v:    8.3835e-05,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"), model.MetricTypeHistogram),
				},
				{
					// NOTE: Unlike OpenMetrics, PromParser allows spaces between label terms. This appears to be unintended and should probably be fixed.
					m:    `go_gc_duration_seconds { quantile = "2.0" a = "b" }`,
					v:    8.3835e-05,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "2.0", "a", "b"), model.MetricTypeHistogram),
				},
				{
					m:    `go_gc_duration_seconds_count`,
					v:    99,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "go_gc_duration_seconds_count"), model.MetricTypeHistogram),
				},
				{
					m:    `some:aggregate:rate5m{a_b="c"}`,
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "some:aggregate:rate5m", "a_b", "c"), model.MetricTypeHistogram),
				},
				{
					m:    "go_goroutines",
					help: "Number of goroutines that currently exist.",
				},
				{
					m:   "go_goroutines",
					typ: model.MetricTypeGauge,
				},
				{
					m: `go_goroutines`,
					v: 33,
					t: int64p(123123),
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_goroutines", "__type__", string(model.MetricTypeGauge)),
						labels.FromStrings("__name__", "go_goroutines"),
					),
				},
				{
					m:   "some_counter_total",
					typ: model.MetricTypeCounter,
				},
				{
					m:    "some_counter_total",
					help: "Help after type.",
				},
				{
					m: `some_counter_total`,
					v: 12,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "some_counter_total", "__type__", string(model.MetricTypeCounter)),
						labels.FromStrings("__name__", "some_counter_total"),
					),
				},
				{
					m:    "nohelp3",
					help: "",
				},
				{
					m:    "_metric_starting_with_underscore",
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "_metric_starting_with_underscore"), model.MetricTypeCounter),
				},
				{
					m:    "testmetric{_label_starting_with_underscore=\"foo\"}",
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "testmetric", "_label_starting_with_underscore", "foo"), model.MetricTypeCounter),
				},
				{
					m:    "testmetric{label=\"\\\"bar\\\"\"}",
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "testmetric", "label", `"bar"`), model.MetricTypeCounter),
				},
				{
					m:    `testmetric{le="10"}`,
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "testmetric", "le", "10"), model.MetricTypeCounter),
				},
				{
					m:    "type_and_unit_test1",
					help: "Type specified in metadata overrides.",
				},
				{
					m:   "type_and_unit_test1",
					typ: model.MetricTypeGauge,
				},
				{
					m: "type_and_unit_test1{__type__=\"counter\"}",
					v: 123,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "type_and_unit_test1", "__type__", string(model.MetricTypeGauge)),
						labels.FromStrings("__name__", "type_and_unit_test1", "__type__", string(model.MetricTypeCounter)),
					),
				},
				{
					m:    "type_and_unit_test2",
					help: "Type specified in label.",
				},
				{
					m:    "type_and_unit_test2{__type__=\"counter\"}",
					v:    123,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "type_and_unit_test2", "__type__", string(model.MetricTypeCounter)), model.MetricTypeGauge),
				},
				{
					m:    "metric",
					help: "foo\x00bar",
				},
				{
					m:    "null_byte_metric{a=\"abc\x00\"}",
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"), model.MetricTypeGauge),
				},
			}

			p := NewPromParser([]byte(input), labels.NewSymbolTable(), typeAndUnitEnabled)
			got := testParse(t, p)
			requireEntries(t, exp, got)
		})
	}
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

	p := NewPromParser([]byte(input), labels.NewSymbolTable(), false)
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
		p := NewPromParser([]byte(c.input), labels.NewSymbolTable(), false)
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
		p := NewPromParser([]byte(c.input), labels.NewSymbolTable(), false)
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
