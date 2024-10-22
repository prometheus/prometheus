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
	"encoding/binary"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
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
# HELP foo Counter with and without labels to certify CT is parsed for both cases
# TYPE foo counter
foo_total 17.0 1520879607.789 # {id="counter-test"} 5
foo_created 1520872607.123
foo_total{a="b"} 17.0 1520879607.789 # {id="counter-test"} 5
foo_created{a="b"} 1520872607.123
# HELP bar Summary with CT at the end, making sure we find CT even if it's multiple lines a far
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
# HELP fizz_created Gauge which shouldn't be parsed as CT
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
			m: `hh{}`,
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
			m: `hhh{}`,
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
			help: "Counter with and without labels to certify CT is parsed for both cases",
		}, {
			m:   "foo",
			typ: model.MetricTypeCounter,
		}, {
			m:    "foo_total",
			v:    17,
			lset: labels.FromStrings("__name__", "foo_total"),
			t:    int64p(1520879607789),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "counter-test"), Value: 5}},
			// TODO(krajorama): ct: int64p(1520872607123),
		}, {
			m:    `foo_total{a="b"}`,
			v:    17.0,
			lset: labels.FromStrings("__name__", "foo_total", "a", "b"),
			t:    int64p(1520879607789),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "counter-test"), Value: 5}},
			// TODO(krajorama): ct: int64p(1520872607123),
		}, {
			m:    "bar",
			help: "Summary with CT at the end, making sure we find CT even if it's multiple lines a far",
		}, {
			m:   "bar",
			typ: model.MetricTypeSummary,
		}, {
			m:    "bar_count",
			v:    17.0,
			lset: labels.FromStrings("__name__", "bar_count"),
			// TODO(krajorama): ct:   int64p(1520872608124),
		}, {
			m:    "bar_sum",
			v:    324789.3,
			lset: labels.FromStrings("__name__", "bar_sum"),
			// TODO(krajorama): ct:   int64p(1520872608124),
		}, {
			m:    `bar{quantile="0.95"}`,
			v:    123.7,
			lset: labels.FromStrings("__name__", "bar", "quantile", "0.95"),
			// TODO(krajorama): ct:   int64p(1520872608124),
		}, {
			m:    `bar{quantile="0.99"}`,
			v:    150.0,
			lset: labels.FromStrings("__name__", "bar", "quantile", "0.99"),
			// TODO(krajorama): ct:   int64p(1520872608124),
		}, {
			m:    "baz",
			help: "Histogram with the same objective as above's summary",
		}, {
			m:   "baz",
			typ: model.MetricTypeHistogram,
		}, {
			m: `baz{}`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           17,
				Sum:             324789.3,
				PositiveSpans:   []histogram.Span{{Offset: 1, Length: 1}}, // The first bucket has 0 count so we don't store it and Offset is 1.
				PositiveBuckets: []int64{17},
				CustomValues:    []float64{0.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "baz"),
			// TODO(krajorama): ct:   int64p(1520872609125),
		}, {
			m:    "fizz_created",
			help: "Gauge which shouldn't be parsed as CT",
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
			m: `something{}`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           18,
				Sum:             324789.4,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
				PositiveBuckets: []int64{1, 16},
				CustomValues:    []float64{0.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "something"),
			// TODO(krajorama): ct:   int64p(1520430001000),
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
			// TODO(krajorama): ct:   int64p(1520430002000),
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
			// TODO(krajorama): ct:   int64p(1520430003000),
		}, {
			m:    `yum_sum`,
			v:    324789.5,
			lset: labels.FromStrings("__name__", "yum_sum"),
			// TODO(krajorama): ct:   int64p(1520430003000),
		}, {
			m:    `yum{quantile="0.95"}`,
			v:    123.7,
			lset: labels.FromStrings("__name__", "yum", "quantile", "0.95"),
			// TODO(krajorama): ct:   int64p(1520430003000),
		}, {
			m:    `yum{quantile="0.99"}`,
			v:    150.0,
			lset: labels.FromStrings("__name__", "yum", "quantile", "0.99"),
			// TODO(krajorama): ct:   int64p(1520430003000),
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
			// TODO(krajorama): ct:   int64p(1520430004000),
		}, {
			m:    `foobar_sum`,
			v:    324789.6,
			lset: labels.FromStrings("__name__", "foobar_sum"),
			// TODO(krajorama): ct:   int64p(1520430004000),
		}, {
			m:    `foobar{quantile="0.95"}`,
			v:    123.8,
			lset: labels.FromStrings("__name__", "foobar", "quantile", "0.95"),
			// TODO(krajorama): ct:   int64p(1520430004000),
		}, {
			m:    `foobar{quantile="0.99"}`,
			v:    150.1,
			lset: labels.FromStrings("__name__", "foobar", "quantile", "0.99"),
			// TODO(krajorama): ct:   int64p(1520430004000),
		}, {
			m:    "metric",
			help: "foo\x00bar",
		}, {
			m:    "null_byte_metric{a=\"abc\x00\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"),
		},
	}

	p := NewOpenMetricsParser([]byte(input), labels.NewSymbolTable(), WithOMParserCTSeriesSkipped())
	p = NewNHCBParser(p, labels.NewSymbolTable(), false)
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
			m: `something{}`,
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
			// TODO(krajorama): ct:   int64p(1520430001000),
		}, {
			m: `something{a="b"}`,
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           9,
				Sum:             42123.0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}, {Offset: 1, Length: 1}},
				PositiveBuckets: []int64{8, -7},
				CustomValues:    []float64{0.0, 1.0}, // We do not store the +Inf boundary.
			},
			lset: labels.FromStrings("__name__", "something", "a", "b"),
			es: []exemplar.Exemplar{
				{Labels: labels.FromStrings("id", "something-test"), Value: 0.0, HasTs: true, Ts: 123321},
				{Labels: labels.FromStrings("id", "something-test"), Value: 2e100, HasTs: true, Ts: 123000},
			},
			// TODO(krajorama): ct:   int64p(1520430002000),
		},
	}

	p := NewOpenMetricsParser([]byte(input), labels.NewSymbolTable(), WithOMParserCTSeriesSkipped())

	p = NewNHCBParser(p, labels.NewSymbolTable(), false)
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

// Verify that the NHCBParser does not parse the NHCB when the exponential is present.
func TestNHCBParserProtoBufParser_NoNHCBWhenExponential(t *testing.T) {
	inputBuf := createTestProtoBufHistogram(t)
	// Initialize the protobuf parser so that it returns classic histograms as
	// well when there's both classic and exponential histograms.
	p := NewProtobufParser(inputBuf.Bytes(), true, labels.NewSymbolTable())

	// Initialize the NHCBParser so that it returns classic histograms as well
	// when there's both classic and exponential histograms.
	p = NewNHCBParser(p, labels.NewSymbolTable(), true)

	exp := []parsedEntry{
		{
			m:    "test_histogram",
			help: "Test histogram with classic and exponential buckets.",
		},
		{
			m:   "test_histogram",
			typ: model.MetricTypeHistogram,
		},
		{
			m: "test_histogram",
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
			lset: labels.FromStrings("__name__", "test_histogram"),
			t:    int64p(1234568),
		},
		{
			m:    "test_histogram_count",
			v:    175,
			lset: labels.FromStrings("__name__", "test_histogram_count"),
			t:    int64p(1234568),
		},
		{
			m:    "test_histogram_sum",
			v:    0.0008280461746287094,
			lset: labels.FromStrings("__name__", "test_histogram_sum"),
			t:    int64p(1234568),
		},
		{
			m:    "test_histogram_bucket\xffle\xff-0.0004899999999999998",
			v:    2,
			lset: labels.FromStrings("__name__", "test_histogram_bucket", "le", "-0.0004899999999999998"),
			t:    int64p(1234568),
		},
		{
			m:    "test_histogram_bucket\xffle\xff-0.0003899999999999998",
			v:    4,
			lset: labels.FromStrings("__name__", "test_histogram_bucket", "le", "-0.0003899999999999998"),
			t:    int64p(1234568),
		},
		{
			m:    "test_histogram_bucket\xffle\xff-0.0002899999999999998",
			v:    16,
			lset: labels.FromStrings("__name__", "test_histogram_bucket", "le", "-0.0002899999999999998"),
			t:    int64p(1234568),
		},
		{
			m:    "test_histogram_bucket\xffle\xff+Inf",
			v:    175,
			lset: labels.FromStrings("__name__", "test_histogram_bucket", "le", "+Inf"),
			t:    int64p(1234568),
		},
		{
			// TODO(krajorama): optimize: this should not be here. In case there's
			// an exponential histogram we should not convert the classic histogram
			// to NHCB. In the end TSDB will throw this away with
			// storage.errDuplicateSampleForTimestamp error at Commit(), but it
			// is better to avoid this conversion in the first place.
			m: "test_histogram{}",
			shs: &histogram.Histogram{
				Schema:          histogram.CustomBucketsSchema,
				Count:           175,
				Sum:             0.0008280461746287094,
				PositiveSpans:   []histogram.Span{{Length: 4}},
				PositiveBuckets: []int64{2, 0, 10, 147},
				CustomValues:    []float64{-0.0004899999999999998, -0.0003899999999999998, -0.0002899999999999998},
			},
			lset: labels.FromStrings("__name__", "test_histogram"),
			t:    int64p(1234568),
		},
	}
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func createTestProtoBufHistogram(t *testing.T) *bytes.Buffer {
	testMetricFamilies := []string{`name: "test_histogram"
help: "Test histogram with classic and exponential buckets."
type: HISTOGRAM
metric: <
  histogram: <
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
>
`}

	varintBuf := make([]byte, binary.MaxVarintLen32)
	buf := &bytes.Buffer{}

	for _, tmf := range testMetricFamilies {
		pb := &dto.MetricFamily{}
		// From text to proto message.
		require.NoError(t, proto.UnmarshalText(tmf, pb))
		// From proto message to binary protobuf.
		protoBuf, err := proto.Marshal(pb)
		require.NoError(t, err)

		// Write first length, then binary protobuf.
		varintLength := binary.PutUvarint(varintBuf, uint64(len(protoBuf)))
		buf.Write(varintBuf[:varintLength])
		buf.Write(protoBuf)
	}

	return buf
}
