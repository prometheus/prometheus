// Copyright 2024 The Prometheus Authors
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
	"bytes"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

func TestNHCBParserOnOMParser(t *testing.T) {
	// The input is taken originally from TestOpenMetricsParse, with additional tests for the NHCBParser.

	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# TYPE go_gc_duration_seconds summary
# UNIT go_gc_duration_seconds seconds
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25"} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
# HELP nohelp1 
# HELP help2 escape \ \n \\ \" \x chars
# UNIT nounit 
go_gc_duration_seconds{quantile="1.0",a="b"} 8.3835e-05
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"} 1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33 123.123
# TYPE hh histogram
hh_bucket{le="+Inf"} 1
# TYPE gh gaugehistogram
gh_bucket{le="+Inf"} 1
# TYPE hhh histogram
hhh_bucket{le="+Inf"} 1 # {id="histogram-bucket-test"} 4
hhh_count 1 # {id="histogram-count-test"} 4
# TYPE ggh gaugehistogram
ggh_bucket{le="+Inf"} 1 # {id="gaugehistogram-bucket-test",xx="yy"} 4 123.123
ggh_count 1 # {id="gaugehistogram-count-test",xx="yy"} 4 123.123
# TYPE smr_seconds summary
smr_seconds_count 2.0 # {id="summary-count-test"} 1 123.321
smr_seconds_sum 42.0 # {id="summary-sum-test"} 1 123.321
# TYPE ii info
ii{foo="bar"} 1
# TYPE ss stateset
ss{ss="foo"} 1
ss{ss="bar"} 0
ss{A="a"} 0
# TYPE un unknown
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1
# HELP foo Counter with and without labels to certify ST is parsed for both cases
# TYPE foo counter
foo_total 17.0 1520879607.789 # {id="counter-test"} 5
foo_created 1520872607.123
foo_total{a="b"} 17.0 1520879607.789 # {id="counter-test"} 5
foo_created{a="b"} 1520872607.123
# HELP bar Summary with ST at the end, making sure we find ST even if it's multiple lines a far
# TYPE bar summary
bar_count 17.0
bar_sum 324789.3
bar{quantile="0.95"} 123.7
bar{quantile="0.99"} 150.0
bar_created 1520872608.124
# HELP baz Histogram with the same objective as above's summary
# TYPE baz histogram
baz_bucket{le="0.0"} 0
baz_bucket{le="+Inf"} 17
baz_count 17
baz_sum 324789.3
baz_created 1520872609.125
# HELP fizz_created Gauge which shouldn't be parsed as ST
# TYPE fizz_created gauge
fizz_created 17.0
# HELP something Histogram with _created between buckets and summary
# TYPE something histogram
something_count 18
something_sum 324789.4
something_created 1520430001
something_bucket{le="0.0"} 1
something_bucket{le="+Inf"} 18
something_count{a="b"} 9
something_sum{a="b"} 42123.0
something_bucket{a="b",le="0.0"} 8
something_bucket{a="b",le="+Inf"} 9
something_created{a="b"} 1520430002
# HELP yum Summary with _created between sum and quantiles
# TYPE yum summary
yum_count 20
yum_sum 324789.5
yum_created 1520430003
yum{quantile="0.95"} 123.7
yum{quantile="0.99"} 150.0
# HELP foobar Summary with _created as the first line
# TYPE foobar summary
foobar_count 21
foobar_created 1520430004
foobar_sum 324789.6
foobar{quantile="0.95"} 123.8
foobar{quantile="0.99"} 150.1`

	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1"
	input += "\n# EOF\n"

	exp := []parsedEntry{
		{
			m:    "go_gc_duration_seconds",
			help: "A summary of the GC invocation durations.",
		}, {
			m:   "go_gc_duration_seconds",
			typ: model.MetricTypeSummary,
		}, {
			m:    "go_gc_duration_seconds",
			unit: "seconds",
		}, {
			m:    `go_gc_duration_seconds{quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.0"),
		}, {
			m:    `go_gc_duration_seconds{quantile="0.25"}`,
			v:    7.424100000000001e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.25"),
		}, {
			m:    `go_gc_duration_seconds{quantile="0.5",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.5", "a", "b"),
		}, {
			m:    "nohelp1",
			help: "",
		}, {
			m:    "help2",
			help: "escape \\ \n \\ \" \\x chars",
		}, {
			m:    "nounit",
			unit: "",
		}, {
			m:    `go_gc_duration_seconds{quantile="1.0",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
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
			m:   "hh",
			typ: model.MetricTypeHistogram,
		}, {
			m: `hh`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           1,
				Sum:             0.0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{1},
				// Custom values are empty as we do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "hh"),
		}, {
			m:   "gh",
			typ: model.MetricTypeGaugeHistogram,
		}, {
			m:    `gh_bucket{le="+Inf"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "gh_bucket", "le", "+Inf"),
		}, {
			m:   "hhh",
			typ: model.MetricTypeHistogram,
		}, {
			m: `hhh`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           1,
				Sum:             0.0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{1},
				// Custom values are empty as we do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "hhh"),
			es: []exemplar.Exemplar{
				{Labels: labels.FromStrings("id", "histogram-bucket-test"), Value: 4},
				{Labels: labels.FromStrings("id", "histogram-count-test"), Value: 4},
			},
		}, {
			m:   "ggh",
			typ: model.MetricTypeGaugeHistogram,
		}, {
			m:    `ggh_bucket{le="+Inf"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "ggh_bucket", "le", "+Inf"),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "gaugehistogram-bucket-test", "xx", "yy"), Value: 4, HasTs: true, Ts: 123123}},
		}, {
			m:    `ggh_count`,
			v:    1,
			lset: labels.FromStrings("__name__", "ggh_count"),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "gaugehistogram-count-test", "xx", "yy"), Value: 4, HasTs: true, Ts: 123123}},
		}, {
			m:   "smr_seconds",
			typ: model.MetricTypeSummary,
		}, {
			m:    `smr_seconds_count`,
			v:    2,
			lset: labels.FromStrings("__name__", "smr_seconds_count"),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "summary-count-test"), Value: 1, HasTs: true, Ts: 123321}},
		}, {
			m:    `smr_seconds_sum`,
			v:    42,
			lset: labels.FromStrings("__name__", "smr_seconds_sum"),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "summary-sum-test"), Value: 1, HasTs: true, Ts: 123321}},
		}, {
			m:   "ii",
			typ: model.MetricTypeInfo,
		}, {
			m:    `ii{foo="bar"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "ii", "foo", "bar"),
		}, {
			m:   "ss",
			typ: model.MetricTypeStateset,
		}, {
			m:    `ss{ss="foo"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "ss", "ss", "foo"),
		}, {
			m:    `ss{ss="bar"}`,
			v:    0,
			lset: labels.FromStrings("__name__", "ss", "ss", "bar"),
		}, {
			m:    `ss{A="a"}`,
			v:    0,
			lset: labels.FromStrings("A", "a", "__name__", "ss"),
		}, {
			m:   "un",
			typ: model.MetricTypeUnknown,
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
			m:    "foo",
			help: "Counter with and without labels to certify ST is parsed for both cases",
		}, {
			m:   "foo",
			typ: model.MetricTypeCounter,
		}, {
			m:    "foo_total",
			v:    17,
			lset: labels.FromStrings("__name__", "foo_total"),
			t:    int64p(1520879607789),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "counter-test"), Value: 5}},
			st:   1520872607123,
		}, {
			m:    `foo_total{a="b"}`,
			v:    17.0,
			lset: labels.FromStrings("__name__", "foo_total", "a", "b"),
			t:    int64p(1520879607789),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "counter-test"), Value: 5}},
			st:   1520872607123,
		}, {
			m:    "bar",
			help: "Summary with ST at the end, making sure we find ST even if it's multiple lines a far",
		}, {
			m:   "bar",
			typ: model.MetricTypeSummary,
		}, {
			m:    "bar_count",
			v:    17.0,
			lset: labels.FromStrings("__name__", "bar_count"),
			st:   1520872608124,
		}, {
			m:    "bar_sum",
			v:    324789.3,
			lset: labels.FromStrings("__name__", "bar_sum"),
			st:   1520872608124,
		}, {
			m:    `bar{quantile="0.95"}`,
			v:    123.7,
			lset: labels.FromStrings("__name__", "bar", "quantile", "0.95"),
			st:   1520872608124,
		}, {
			m:    `bar{quantile="0.99"}`,
			v:    150.0,
			lset: labels.FromStrings("__name__", "bar", "quantile", "0.99"),
			st:   1520872608124,
		}, {
			m:    "baz",
			help: "Histogram with the same objective as above's summary",
		}, {
			m:   "baz",
			typ: model.MetricTypeHistogram,
		}, {
			m: `baz`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           17,
				Sum:             324789.3,
				PositiveSpans:   []histogram.Span{{Offset: 1, Length: 1}}, // The first bucket has 0 count so we don't store it and Offset is 1.
				PositiveBuckets: []int64{17},
				CustomValues:    []float64{0.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "baz"),
			st:   1520872609125,
		}, {
			m:    "fizz_created",
			help: "Gauge which shouldn't be parsed as ST",
		}, {
			m:   "fizz_created",
			typ: model.MetricTypeGauge,
		}, {
			m:    `fizz_created`,
			v:    17,
			lset: labels.FromStrings("__name__", "fizz_created"),
		}, {
			m:    "something",
			help: "Histogram with _created between buckets and summary",
		}, {
			m:   "something",
			typ: model.MetricTypeHistogram,
		}, {
			m: `something`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           18,
				Sum:             324789.4,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
				PositiveBuckets: []int64{1, 16},
				CustomValues:    []float64{0.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "something"),
			st:   1520430001000,
		}, {
			m: `something{a="b"}`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           9,
				Sum:             42123.0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
				PositiveBuckets: []int64{8, -7},
				CustomValues:    []float64{0.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "something", "a", "b"),
			st:   1520430002000,
		}, {
			m:    "yum",
			help: "Summary with _created between sum and quantiles",
		}, {
			m:   "yum",
			typ: model.MetricTypeSummary,
		}, {
			m:    `yum_count`,
			v:    20,
			lset: labels.FromStrings("__name__", "yum_count"),
			st:   1520430003000,
		}, {
			m:    `yum_sum`,
			v:    324789.5,
			lset: labels.FromStrings("__name__", "yum_sum"),
			st:   1520430003000,
		}, {
			m:    `yum{quantile="0.95"}`,
			v:    123.7,
			lset: labels.FromStrings("__name__", "yum", "quantile", "0.95"),
			st:   1520430003000,
		}, {
			m:    `yum{quantile="0.99"}`,
			v:    150.0,
			lset: labels.FromStrings("__name__", "yum", "quantile", "0.99"),
			st:   1520430003000,
		}, {
			m:    "foobar",
			help: "Summary with _created as the first line",
		}, {
			m:   "foobar",
			typ: model.MetricTypeSummary,
		}, {
			m:    `foobar_count`,
			v:    21,
			lset: labels.FromStrings("__name__", "foobar_count"),
			st:   1520430004000,
		}, {
			m:    `foobar_sum`,
			v:    324789.6,
			lset: labels.FromStrings("__name__", "foobar_sum"),
			st:   1520430004000,
		}, {
			m:    `foobar{quantile="0.95"}`,
			v:    123.8,
			lset: labels.FromStrings("__name__", "foobar", "quantile", "0.95"),
			st:   1520430004000,
		}, {
			m:    `foobar{quantile="0.99"}`,
			v:    150.1,
			lset: labels.FromStrings("__name__", "foobar", "quantile", "0.99"),
			st:   1520430004000,
		}, {
			m:    "metric",
			help: "foo\x00bar",
		}, {
			m:    "null_byte_metric{a=\"abc\x00\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"),
		},
	}

	p, err := New([]byte(input), "application/openmetrics-text", labels.NewSymbolTable(), ParserOptions{ConvertClassicHistogramsToNHCB: true})
	require.NoError(t, err)
	require.NotNil(t, p)
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestNHCBParserOMParser_MultipleHistograms(t *testing.T) {
	// The input is taken originally from TestOpenMetricsParse, with additional tests for the NHCBParser.

	input := `# HELP something Histogram with _created between buckets and summary
# TYPE something histogram
something_count 18
something_sum 324789.4
something_bucket{le="0.0"} 1 # {id="something-test"} -2.0
something_bucket{le="1.0"} 16 # {id="something-test"} 0.5
something_bucket{le="+Inf"} 18 # {id="something-test"} 8
something_count{a="b"} 9
something_sum{a="b"} 42123.0
something_bucket{a="b",le="0.0"} 8 # {id="something-test"} 0.0 123.321
something_bucket{a="b",le="1.0"} 8
something_bucket{a="b",le="+Inf"} 9 # {id="something-test"} 2e100 123.000
# EOF
`

	exp := []parsedEntry{
		{
			m:    "something",
			help: "Histogram with _created between buckets and summary",
		}, {
			m:   "something",
			typ: model.MetricTypeHistogram,
		}, {
			m: `something`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           18,
				Sum:             324789.4,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{1, 14, -13},
				CustomValues:    []float64{0.0, 1.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "something"),
			es: []exemplar.Exemplar{
				{Labels: labels.FromStrings("id", "something-test"), Value: -2.0},
				{Labels: labels.FromStrings("id", "something-test"), Value: 0.5},
				{Labels: labels.FromStrings("id", "something-test"), Value: 8.0},
			},
		}, {
			m: `something{a="b"}`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           9,
				Sum:             42123.0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{8, -8, 1},
				CustomValues:    []float64{0.0, 1.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "something", "a", "b"),
			es: []exemplar.Exemplar{
				{Labels: labels.FromStrings("id", "something-test"), Value: 0.0, HasTs: true, Ts: 123321},
				{Labels: labels.FromStrings("id", "something-test"), Value: 2e100, HasTs: true, Ts: 123000},
			},
		},
	}

	p, err := New([]byte(input), "application/openmetrics-text", labels.NewSymbolTable(), ParserOptions{ConvertClassicHistogramsToNHCB: true})
	require.NoError(t, err)
	require.NotNil(t, p)
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

// Verify the requirement tables from
// https://github.com/prometheus/prometheus/issues/13532 .
// "classic" means the option "always_scrape_classic_histograms".
// "nhcb" means the option "convert_classic_histograms_to_nhcb".
//
// Case 1. Only classic histogram is exposed.
//
// | Scrape Config             | Expect classic | Expect exponential | Expect NHCB |.
// | classic=false, nhcb=false | YES            | NO                 | NO          |.
// | classic=true,  nhcb=false | YES            | NO                 | NO          |.
// | classic=false, nhcb=true  | NO             | NO                 | YES         |.
// | classic=true,  nhcb=true  | YES            | NO                 | YES         |.
//
// Case 2. Both classic and exponential histograms are exposed.
//
// | Scrape Config             | Expect classic | Expect exponential | Expect NHCB |.
// | classic=false, nhcb=false | NO             | YES                | NO          |.
// | classic=true,  nhcb=false | YES            | YES                | NO          |.
// | classic=false, nhcb=true  | NO             | YES                | NO          |.
// | classic=true,  nhcb=true  | YES            | YES                | NO          |.
//
// Case 3. Only exponential histogram is exposed.
//
// | Scrape Config             | Expect classic | Expect exponential | Expect NHCB |.
// | classic=false, nhcb=false | NO             | YES                | NO          |.
// | classic=true,  nhcb=false | NO             | YES                | NO          |.
// | classic=false, nhcb=true  | NO             | YES                | NO          |.
// | classic=true,  nhcb=true  | NO             | YES                | NO          |.
func TestNHCBParser_NoNHCBWhenExponential(t *testing.T) {
	type requirement struct {
		expectClassic     bool
		expectExponential bool
		expectNHCB        bool
	}

	cases := []map[string]requirement{
		// Case 1.
		{
			"classic=false, nhcb=false": {expectClassic: true, expectExponential: false, expectNHCB: false},
			"classic=true, nhcb=false":  {expectClassic: true, expectExponential: false, expectNHCB: false},
			"classic=false, nhcb=true":  {expectClassic: false, expectExponential: false, expectNHCB: true},
			"classic=true, nhcb=true":   {expectClassic: true, expectExponential: false, expectNHCB: true},
		},
		// Case 2.
		{
			"classic=false, nhcb=false": {expectClassic: false, expectExponential: true, expectNHCB: false},
			"classic=true, nhcb=false":  {expectClassic: true, expectExponential: true, expectNHCB: false},
			"classic=false, nhcb=true":  {expectClassic: false, expectExponential: true, expectNHCB: false},
			"classic=true, nhcb=true":   {expectClassic: true, expectExponential: true, expectNHCB: false},
		},
		// Case 3.
		{
			"classic=false, nhcb=false": {expectClassic: false, expectExponential: true, expectNHCB: false},
			"classic=true, nhcb=false":  {expectClassic: false, expectExponential: true, expectNHCB: false},
			"classic=false, nhcb=true":  {expectClassic: false, expectExponential: true, expectNHCB: false},
			"classic=true, nhcb=true":   {expectClassic: false, expectExponential: true, expectNHCB: false},
		},
	}

	// Create parser from keep classic option.
	type parserFactory func(keepClassic, nhcb bool) (Parser, error)

	type testCase struct {
		name    string
		parser  parserFactory
		classic bool
		nhcb    bool
		exp     []parsedEntry
	}

	type parserOptions struct {
		useUTF8sep        bool
		hasStartTimestamp bool
	}
	// Defines the parser name, the Parser factory and the test cases
	// supported by the parser and parser options.
	parsers := []func() (string, parserFactory, []int, parserOptions){
		func() (string, parserFactory, []int, parserOptions) {
			factory := func(keepClassic, nhcb bool) (Parser, error) {
				inputBuf := createTestProtoBufHistogram(t)
				return New(inputBuf.Bytes(), "application/vnd.google.protobuf", labels.NewSymbolTable(), ParserOptions{KeepClassicOnClassicAndNativeHistograms: keepClassic, ConvertClassicHistogramsToNHCB: nhcb})
			}
			return "ProtoBuf", factory, []int{1, 2, 3}, parserOptions{useUTF8sep: true, hasStartTimestamp: true}
		},
		func() (string, parserFactory, []int, parserOptions) {
			factory := func(keepClassic, nhcb bool) (Parser, error) {
				input := createTestOpenMetricsHistogram()
				return New([]byte(input), "application/openmetrics-text", labels.NewSymbolTable(), ParserOptions{KeepClassicOnClassicAndNativeHistograms: keepClassic, ConvertClassicHistogramsToNHCB: nhcb})
			}
			return "OpenMetrics", factory, []int{1}, parserOptions{hasStartTimestamp: true}
		},
		func() (string, parserFactory, []int, parserOptions) {
			factory := func(keepClassic, nhcb bool) (Parser, error) {
				input := createTestPromHistogram()
				return New([]byte(input), "text/plain", labels.NewSymbolTable(), ParserOptions{KeepClassicOnClassicAndNativeHistograms: keepClassic, ConvertClassicHistogramsToNHCB: nhcb})
			}
			return "Prometheus", factory, []int{1}, parserOptions{}
		},
	}

	testCases := []testCase{}
	for _, parser := range parsers {
		for _, classic := range []bool{false, true} {
			for _, nhcb := range []bool{false, true} {
				parserName, parser, supportedCases, options := parser()
				requirementName := "classic=" + strconv.FormatBool(classic) + ", nhcb=" + strconv.FormatBool(nhcb)
				tc := testCase{
					name:    "parser=" + parserName + ", " + requirementName,
					parser:  parser,
					classic: classic,
					nhcb:    nhcb,
					exp:     []parsedEntry{},
				}
				for _, caseNumber := range supportedCases {
					caseI := cases[caseNumber-1]
					req, ok := caseI[requirementName]
					require.True(t, ok, "Case %d does not have requirement %s", caseNumber, requirementName)
					metric := "test_histogram" + strconv.Itoa(caseNumber)
					tc.exp = append(tc.exp, parsedEntry{
						m:    metric,
						help: "Test histogram " + strconv.Itoa(caseNumber),
					})
					tc.exp = append(tc.exp, parsedEntry{
						m:   metric,
						typ: model.MetricTypeHistogram,
					})

					var st int64
					if options.hasStartTimestamp {
						st = 1000
					}

					var bucketForMetric func(string) string
					if options.useUTF8sep {
						bucketForMetric = func(s string) string {
							return "_bucket\xffle\xff" + s
						}
					} else {
						bucketForMetric = func(s string) string {
							return "_bucket{le=\"" + s + "\"}"
						}
					}

					if req.expectExponential {
						// Always expect exponential histogram first.
						exponentialSeries := []parsedEntry{
							{
								m: metric,
								shs: &histogram.Histogram{
									Schema:          3,
									Count:           175,
									Sum:             0.0008280461746287094,
									ZeroThreshold:   2.938735877055719e-39,
									ZeroCount:       2,
									PositiveSpans:   []histogram.Span{{Offset: -161, Length: 1}, {Offset: 8, Length: 3}},
									NegativeSpans:   []histogram.Span{{Offset: -162, Length: 1}, {Offset: 23, Length: 4}},
									PositiveBuckets: []int64{1, 2, -1, -1},
									NegativeBuckets: []int64{1, 3, -2, -1, 1},
								},
								lset: labels.FromStrings("__name__", metric),
								t:    int64p(1234568),
								st:   st,
							},
						}
						tc.exp = append(tc.exp, exponentialSeries...)
					}
					if req.expectClassic {
						// Always expect classic histogram series after exponential.
						classicSeries := []parsedEntry{
							{
								m:    metric + "_count",
								v:    175,
								lset: labels.FromStrings("__name__", metric+"_count"),
								t:    int64p(1234568),
								st:   st,
							},
							{
								m:    metric + "_sum",
								v:    0.0008280461746287094,
								lset: labels.FromStrings("__name__", metric+"_sum"),
								t:    int64p(1234568),
								st:   st,
							},
							{
								m:    metric + bucketForMetric("-0.0004899999999999998"),
								v:    2,
								lset: labels.FromStrings("__name__", metric+"_bucket", "le", "-0.0004899999999999998"),
								t:    int64p(1234568),
								st:   st,
							},
							{
								m:    metric + bucketForMetric("-0.0003899999999999998"),
								v:    4,
								lset: labels.FromStrings("__name__", metric+"_bucket", "le", "-0.0003899999999999998"),
								t:    int64p(1234568),
								st:   st,
							},
							{
								m:    metric + bucketForMetric("-0.0002899999999999998"),
								v:    16,
								lset: labels.FromStrings("__name__", metric+"_bucket", "le", "-0.0002899999999999998"),
								t:    int64p(1234568),
								st:   st,
							},
							{
								m:    metric + bucketForMetric("+Inf"),
								v:    175,
								lset: labels.FromStrings("__name__", metric+"_bucket", "le", "+Inf"),
								t:    int64p(1234568),
								st:   st,
							},
						}
						tc.exp = append(tc.exp, classicSeries...)
					}
					if req.expectNHCB {
						// Always expect NHCB series after classic.
						nhcbSeries := []parsedEntry{
							{
								m: metric,
								shs: &histogram.Histogram{
									Schema:          histogram.CustomBucketsSchema,
									Count:           175,
									Sum:             0.0008280461746287094,
									PositiveSpans:   []histogram.Span{{Length: 4}},
									PositiveBuckets: []int64{2, 0, 10, 147},
									CustomValues:    []float64{-0.0004899999999999998, -0.0003899999999999998, -0.0002899999999999998},
								},
								lset: labels.FromStrings("__name__", metric),
								t:    int64p(1234568),
								st:   st,
							},
						}
						tc.exp = append(tc.exp, nhcbSeries...)
					}
				}
				testCases = append(testCases, tc)
			}
		}
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.parser(tc.classic, tc.nhcb)
			require.NoError(t, err)
			require.NotNil(t, p)
			got := testParse(t, p)
			requireEntries(t, tc.exp, got)
		})
	}
}

func createTestProtoBufHistogram(t *testing.T) *bytes.Buffer {
	testMetricFamilies := []string{`name: "test_histogram1"
help: "Test histogram 1"
type: HISTOGRAM
metric: <
  histogram: <
		created_timestamp: <
      seconds: 1
      nanos: 1
    >
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
    >
  >
  timestamp_ms: 1234568
>`, `name: "test_histogram2"
help: "Test histogram 2"
type: HISTOGRAM
metric: <
  histogram: <
		created_timestamp: <
      seconds: 1
      nanos: 1
    >
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>`, `name: "test_histogram3"
help: "Test histogram 3"
type: HISTOGRAM
metric: <
  histogram: <
		created_timestamp: <
      seconds: 1
      nanos: 1
    >
    sample_count: 175
    sample_sum: 0.0008280461746287094
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>
`}

	return metricFamiliesToProtobuf(t, testMetricFamilies)
}

func createTestOpenMetricsHistogram() string {
	return `# HELP test_histogram1 Test histogram 1
# TYPE test_histogram1 histogram
test_histogram1_count 175 1234.568
test_histogram1_sum 0.0008280461746287094 1234.568
test_histogram1_bucket{le="-0.0004899999999999998"} 2 1234.568
test_histogram1_bucket{le="-0.0003899999999999998"} 4 1234.568
test_histogram1_bucket{le="-0.0002899999999999998"} 16 1234.568
test_histogram1_bucket{le="+Inf"} 175 1234.568
test_histogram1_created 1
# EOF`
}

func createTestPromHistogram() string {
	return `# HELP test_histogram1 Test histogram 1
# TYPE test_histogram1 histogram
test_histogram1_count 175 1234568
test_histogram1_sum 0.0008280461746287094 1234568
test_histogram1_bucket{le="-0.0004899999999999998"} 2 1234568
test_histogram1_bucket{le="-0.0003899999999999998"} 4 1234568
test_histogram1_bucket{le="-0.0002899999999999998"} 16 1234568
test_histogram1_bucket{le="+Inf"} 175 1234568`
}

func TestNHCBParserErrorHandling(t *testing.T) {
	input := `# HELP something Histogram with non cumulative buckets
# TYPE something histogram
something_count 18
something_sum 324789.4
something_created 1520430001
something_bucket{le="0.0"} 18
something_bucket{le="+Inf"} 1
something_count{a="b"} 9
something_sum{a="b"} 42123
something_created{a="b"} 1520430002
something_bucket{a="b",le="0.0"} 1
something_bucket{a="b",le="+Inf"} 9
# EOF`
	exp := []parsedEntry{
		{
			m:    "something",
			help: "Histogram with non cumulative buckets",
		},
		{
			m:   "something",
			typ: model.MetricTypeHistogram,
		},
		// The parser should skip the series with non-cumulative buckets.
		{
			m: `something{a="b"}`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           9,
				Sum:             42123.0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
				PositiveBuckets: []int64{1, 7},
				CustomValues:    []float64{0.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "something", "a", "b"),
			st:   1520430002000,
		},
	}

	p, err := New([]byte(input), "application/openmetrics-text", labels.NewSymbolTable(), ParserOptions{ConvertClassicHistogramsToNHCB: true})
	require.NoError(t, err)
	require.NotNil(t, p)
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestNHCBParserResetLastExponential(t *testing.T) {
	testMetricFamilies := []string{`name: "test_histogram1"
help: "Test histogram 1"
type: HISTOGRAM
metric: <
  histogram: <
		created_timestamp: <
      seconds: 1
      nanos: 1
    >
    sample_count: 175
    sample_sum: 0.0008280461746287094
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>
`, // Regression test to see that state resets after exponential native histogram.
		`name: "test_histogram2"
help: "Test histogram 2"
type: HISTOGRAM
metric: <
  histogram: <
		created_timestamp: <
      seconds: 1
      nanos: 1
    >
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
    >
  >
  timestamp_ms: 1234568
>`}

	buf := metricFamiliesToProtobuf(t, testMetricFamilies)

	exp := []parsedEntry{
		{
			m:    "test_histogram1",
			help: "Test histogram 1",
		},
		{
			m:   "test_histogram1",
			typ: model.MetricTypeHistogram,
		},
		// The parser should skip the series with non-cumulative buckets.
		{
			m: `test_histogram1`,
			shs: &histogram.Histogram{
				Schema:          3,
				Count:           175,
				Sum:             0.0008280461746287094,
				ZeroThreshold:   2.938735877055719e-39,
				ZeroCount:       2,
				PositiveSpans:   []histogram.Span{{Offset: -161, Length: 1}, {Offset: 8, Length: 3}},
				NegativeSpans:   []histogram.Span{{Offset: -162, Length: 1}, {Offset: 23, Length: 4}},
				PositiveBuckets: []int64{1, 2, -1, -1},
				NegativeBuckets: []int64{1, 3, -2, -1, 1},
			},
			lset: labels.FromStrings("__name__", "test_histogram1"),
			t:    int64p(1234568),
			st:   1000,
		},
		{
			m:    "test_histogram2",
			help: "Test histogram 2",
		},
		{
			m:   "test_histogram2",
			typ: model.MetricTypeHistogram,
		},
		{
			m: "test_histogram2",
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           175,
				Sum:             0.0008280461746287094,
				PositiveSpans:   []histogram.Span{{Length: 4}},
				PositiveBuckets: []int64{2, 0, 10, 147},
				CustomValues:    []float64{-0.0004899999999999998, -0.0003899999999999998, -0.0002899999999999998},
			},
			lset: labels.FromStrings("__name__", "test_histogram2"),
			t:    int64p(1234568),
			st:   1000,
		},
	}

	p, err := New(buf.Bytes(), "application/vnd.google.protobuf", labels.NewSymbolTable(), ParserOptions{ConvertClassicHistogramsToNHCB: true})
	require.NoError(t, err)
	require.NotNil(t, p)
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

// TestNHCBNotCorruptMetricNameAfterRead is a regression test for https://github.com/prometheus/prometheus/issues/17075.
func TestNHCBNotCorruptMetricNameAfterRead(t *testing.T) {
	inputOM := `# HELP test_histogram_seconds Just a test histogram
# TYPE test_histogram_seconds histogram
test_histogram_seconds_count 10
test_histogram_seconds_sum 100
test_histogram_seconds_bucket{le="10"} 10
test_histogram_seconds_bucket{le="+Inf"} 10
# HELP different_metric Just a different metric
# TYPE different_metric histogram
different_metric_count 5
different_metric_sum 50
different_metric_bucket{le="10"} 5
different_metric_bucket{le="+Inf"} 5
# EOF`

	testMetricFamilies := []string{`name: "test_histogram_seconds"
help: "Just a test histogram"
type: HISTOGRAM
metric: <
  histogram: <
	sample_count: 10
	sample_sum: 100
	bucket: <
	  cumulative_count: 10
	  upper_bound: 10
	>
  >
>`, `name: "different_metric"
help: "Just a different metric"
type: HISTOGRAM
metric: <
  histogram: <
	sample_count: 5
	sample_sum: 50
	bucket: <
	  cumulative_count: 5
	  upper_bound: 10
	>
  >
>`}

	buf := metricFamiliesToProtobuf(t, testMetricFamilies)

	testCases := []struct {
		input []byte
		typ   string
	}{
		{input: buf.Bytes(), typ: "application/vnd.google.protobuf"},
		{input: []byte(inputOM), typ: "text/plain"},
		{input: []byte(inputOM), typ: "application/openmetrics-text"},
	}

	for _, tc := range testCases {
		t.Run(tc.typ, func(t *testing.T) {
			p, err := New(tc.input, tc.typ, labels.NewSymbolTable(), ParserOptions{ConvertClassicHistogramsToNHCB: true})
			require.NoError(t, err)
			require.NotNil(t, p)

			getNext := func() Entry {
				e, err := p.Next()
				require.NoError(t, err)
				return e
			}

			require.Equal(t, EntryHelp, getNext())
			lastMFName, lastHelp := p.Help()
			require.Equal(t, "test_histogram_seconds", string(lastMFName))
			require.Equal(t, "Just a test histogram", string(lastHelp))

			require.Equal(t, EntryType, getNext())
			var lastType model.MetricType
			lastMFName, lastType = p.Type()
			require.Equal(t, "test_histogram_seconds", string(lastMFName))
			require.Equal(t, model.MetricTypeHistogram, lastType)

			require.Equal(t, EntryHistogram, getNext())
			_, _, h, _ := p.Histogram()
			require.NotNil(t, h)
			require.Equal(t, "test_histogram_seconds", string(lastMFName))
		})
	}
}
