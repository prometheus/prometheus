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
	"fmt"
	"io"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
)

func int64p(x int64) *int64 { return &x }

func TestOpenMetricsParse(t *testing.T) {
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
foo_total{le="c"} 21.0
foo_created{le="c"} 1520872621.123
foo_total{le="1"} 10.0
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
something_bucket{le="1"} 2
something_bucket{le="+Inf"} 18
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

	for _, typeAndUnitEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("type-and-unit=%v", typeAndUnitEnabled), func(t *testing.T) {
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
					m: `go_gc_duration_seconds{quantile="0"}`,
					v: 4.9351e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "__unit__", "seconds", "quantile", "0.0"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.0"),
					),
				}, {
					m: `go_gc_duration_seconds{quantile="0.25"}`,
					v: 7.424100000000001e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "__unit__", "seconds", "quantile", "0.25"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.25"),
					),
				}, {
					m: `go_gc_duration_seconds{quantile="0.5",a="b"}`,
					v: 8.3835e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "__unit__", "seconds", "quantile", "0.5", "a", "b"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.5", "a", "b"),
					),
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
					m: `go_gc_duration_seconds{quantile="1.0",a="b"}`,
					v: 8.3835e-05,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds", "__type__", string(model.MetricTypeSummary), "quantile", "1.0", "a", "b"),
						labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
					),
				}, {
					m: `go_gc_duration_seconds_count`,
					v: 99,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_gc_duration_seconds_count", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "go_gc_duration_seconds_count"),
					),
				}, {
					m:    `some:aggregate:rate5m{a_b="c"}`,
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "some:aggregate:rate5m", "a_b", "c"), model.MetricTypeSummary),
				}, {
					m:    "go_goroutines",
					help: "Number of goroutines that currently exist.",
				}, {
					m:   "go_goroutines",
					typ: model.MetricTypeGauge,
				}, {
					m: `go_goroutines`,
					v: 33,
					t: int64p(123123),
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "go_goroutines", "__type__", string(model.MetricTypeGauge)),
						labels.FromStrings("__name__", "go_goroutines"),
					),
				}, {
					m:   "hh",
					typ: model.MetricTypeHistogram,
				}, {
					m: `hh_bucket{le="+Inf"}`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "hh_bucket", "__type__", string(model.MetricTypeHistogram), "le", "+Inf"),
						labels.FromStrings("__name__", "hh_bucket", "le", "+Inf"),
					),
				}, {
					m:   "gh",
					typ: model.MetricTypeGaugeHistogram,
				}, {
					m: `gh_bucket{le="+Inf"}`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "gh_bucket", "__type__", string(model.MetricTypeGaugeHistogram), "le", "+Inf"),
						labels.FromStrings("__name__", "gh_bucket", "le", "+Inf"),
					),
				}, {
					m:   "hhh",
					typ: model.MetricTypeHistogram,
				}, {
					m: `hhh_bucket{le="+Inf"}`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "hhh_bucket", "__type__", string(model.MetricTypeHistogram), "le", "+Inf"),
						labels.FromStrings("__name__", "hhh_bucket", "le", "+Inf"),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "histogram-bucket-test"), Value: 4},
					},
				}, {
					m: `hhh_count`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "hhh_count", "__type__", string(model.MetricTypeHistogram)),
						labels.FromStrings("__name__", "hhh_count"),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "histogram-count-test"), Value: 4},
					},
				}, {
					m:   "ggh",
					typ: model.MetricTypeGaugeHistogram,
				}, {
					m: `ggh_bucket{le="+Inf"}`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "ggh_bucket", "__type__", string(model.MetricTypeGaugeHistogram), "le", "+Inf"),
						labels.FromStrings("__name__", "ggh_bucket", "le", "+Inf"),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "gaugehistogram-bucket-test", "xx", "yy"), Value: 4, HasTs: true, Ts: 123123},
					},
				}, {
					m: `ggh_count`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "ggh_count", "__type__", string(model.MetricTypeGaugeHistogram)),
						labels.FromStrings("__name__", "ggh_count"),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "gaugehistogram-count-test", "xx", "yy"), Value: 4, HasTs: true, Ts: 123123},
					},
				}, {
					m:   "smr_seconds",
					typ: model.MetricTypeSummary,
				}, {
					m: `smr_seconds_count`,
					v: 2,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "smr_seconds_count", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "smr_seconds_count"),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "summary-count-test"), Value: 1, HasTs: true, Ts: 123321},
					},
				}, {
					m: `smr_seconds_sum`,
					v: 42,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "smr_seconds_sum", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "smr_seconds_sum"),
					),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "summary-sum-test"), Value: 1, HasTs: true, Ts: 123321},
					},
				}, {
					m:   "ii",
					typ: model.MetricTypeInfo,
				}, {
					m: `ii{foo="bar"}`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "ii", "__type__", string(model.MetricTypeInfo), "foo", "bar"),
						labels.FromStrings("__name__", "ii", "foo", "bar"),
					),
				}, {
					m:   "ss",
					typ: model.MetricTypeStateset,
				}, {
					m: `ss{ss="foo"}`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "ss", "__type__", string(model.MetricTypeStateset), "ss", "foo"),
						labels.FromStrings("__name__", "ss", "ss", "foo"),
					),
				}, {
					m: `ss{ss="bar"}`,
					v: 0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "ss", "__type__", string(model.MetricTypeStateset), "ss", "bar"),
						labels.FromStrings("__name__", "ss", "ss", "bar"),
					),
				}, {
					m: `ss{A="a"}`,
					v: 0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "ss", "__type__", string(model.MetricTypeStateset), "A", "a"),
						labels.FromStrings("__name__", "ss", "A", "a"),
					),
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
					m: "foo_total",
					v: 17,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foo_total", "__type__", string(model.MetricTypeCounter)),
						labels.FromStrings("__name__", "foo_total"),
					),
					t: int64p(1520879607789),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "counter-test"), Value: 5},
					},
					ct: 1520872607123,
				}, {
					m: `foo_total{a="b"}`,
					v: 17.0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foo_total", "__type__", string(model.MetricTypeCounter), "a", "b"),
						labels.FromStrings("__name__", "foo_total", "a", "b"),
					),
					t: int64p(1520879607789),
					es: []exemplar.Exemplar{
						{Labels: labels.FromStrings("id", "counter-test"), Value: 5},
					},
					ct: 1520872607123,
				}, {
					m: `foo_total{le="c"}`,
					v: 21.0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foo_total", "__type__", string(model.MetricTypeCounter), "le", "c"),
						labels.FromStrings("__name__", "foo_total", "le", "c"),
					),
					ct: 1520872621123,
				}, {
					m: `foo_total{le="1"}`,
					v: 10.0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foo_total", "__type__", string(model.MetricTypeCounter), "le", "1"),
						labels.FromStrings("__name__", "foo_total", "le", "1"),
					),
				}, {
					m:    "bar",
					help: "Summary with CT at the end, making sure we find CT even if it's multiple lines a far",
				}, {
					m:   "bar",
					typ: model.MetricTypeSummary,
				}, {
					m: "bar_count",
					v: 17.0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "bar_count", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "bar_count"),
					),
					ct: 1520872608124,
				}, {
					m: "bar_sum",
					v: 324789.3,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "bar_sum", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "bar_sum"),
					),
					ct: 1520872608124,
				}, {
					m: `bar{quantile="0.95"}`,
					v: 123.7,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "bar", "__type__", string(model.MetricTypeSummary), "quantile", "0.95"),
						labels.FromStrings("__name__", "bar", "quantile", "0.95"),
					),
					ct: 1520872608124,
				}, {
					m: `bar{quantile="0.99"}`,
					v: 150.0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "bar", "__type__", string(model.MetricTypeSummary), "quantile", "0.99"),
						labels.FromStrings("__name__", "bar", "quantile", "0.99"),
					),
					ct: 1520872608124,
				}, {
					m:    "baz",
					help: "Histogram with the same objective as above's summary",
				}, {
					m:   "baz",
					typ: model.MetricTypeHistogram,
				}, {
					m: `baz_bucket{le="0.0"}`,
					v: 0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "baz_bucket", "__type__", string(model.MetricTypeHistogram), "le", "0.0"),
						labels.FromStrings("__name__", "baz_bucket", "le", "0.0"),
					),
					ct: 1520872609125,
				}, {
					m: `baz_bucket{le="+Inf"}`,
					v: 17,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "baz_bucket", "__type__", string(model.MetricTypeHistogram), "le", "+Inf"),
						labels.FromStrings("__name__", "baz_bucket", "le", "+Inf"),
					),
					ct: 1520872609125,
				}, {
					m: `baz_count`,
					v: 17,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "baz_count", "__type__", string(model.MetricTypeHistogram)),
						labels.FromStrings("__name__", "baz_count"),
					),
					ct: 1520872609125,
				}, {
					m: `baz_sum`,
					v: 324789.3,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "baz_sum", "__type__", string(model.MetricTypeHistogram)),
						labels.FromStrings("__name__", "baz_sum"),
					),
					ct: 1520872609125,
				}, {
					m:    "fizz_created",
					help: "Gauge which shouldn't be parsed as CT",
				}, {
					m:   "fizz_created",
					typ: model.MetricTypeGauge,
				}, {
					m: `fizz_created`,
					v: 17,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "fizz_created", "__type__", string(model.MetricTypeGauge)),
						labels.FromStrings("__name__", "fizz_created"),
					),
				}, {
					m:    "something",
					help: "Histogram with _created between buckets and summary",
				}, {
					m:   "something",
					typ: model.MetricTypeHistogram,
				}, {
					m: `something_count`,
					v: 18,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "something_count", "__type__", string(model.MetricTypeHistogram)),
						labels.FromStrings("__name__", "something_count"),
					),
					ct: 1520430001000,
				}, {
					m: `something_sum`,
					v: 324789.4,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "something_sum", "__type__", string(model.MetricTypeHistogram)),
						labels.FromStrings("__name__", "something_sum"),
					),
					ct: 1520430001000,
				}, {
					m: `something_bucket{le="0.0"}`,
					v: 1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "something_bucket", "__type__", string(model.MetricTypeHistogram), "le", "0.0"),
						labels.FromStrings("__name__", "something_bucket", "le", "0.0"),
					),
					ct: 1520430001000,
				}, {
					m: `something_bucket{le="1"}`,
					v: 2,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "something_bucket", "__type__", string(model.MetricTypeHistogram), "le", "1.0"),
						labels.FromStrings("__name__", "something_bucket", "le", "1.0"),
					),
					ct: 1520430001000,
				}, {
					m: `something_bucket{le="+Inf"}`,
					v: 18,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "something_bucket", "__type__", string(model.MetricTypeHistogram), "le", "+Inf"),
						labels.FromStrings("__name__", "something_bucket", "le", "+Inf"),
					),
					ct: 1520430001000,
				}, {
					m:    "yum",
					help: "Summary with _created between sum and quantiles",
				}, {
					m:   "yum",
					typ: model.MetricTypeSummary,
				}, {
					m: `yum_count`,
					v: 20,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "yum_count", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "yum_count"),
					),
					ct: 1520430003000,
				}, {
					m: `yum_sum`,
					v: 324789.5,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "yum_sum", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "yum_sum"),
					),
					ct: 1520430003000,
				}, {
					m: `yum{quantile="0.95"}`,
					v: 123.7,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "yum", "__type__", string(model.MetricTypeSummary), "quantile", "0.95"),
						labels.FromStrings("__name__", "yum", "quantile", "0.95"),
					),
					ct: 1520430003000,
				}, {
					m: `yum{quantile="0.99"}`,
					v: 150.0,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "yum", "__type__", string(model.MetricTypeSummary), "quantile", "0.99"),
						labels.FromStrings("__name__", "yum", "quantile", "0.99"),
					),
					ct: 1520430003000,
				}, {
					m:    "foobar",
					help: "Summary with _created as the first line",
				}, {
					m:   "foobar",
					typ: model.MetricTypeSummary,
				}, {
					m: `foobar_count`,
					v: 21,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foobar_count", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "foobar_count"),
					),
					ct: 1520430004000,
				}, {
					m: `foobar_sum`,
					v: 324789.6,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foobar_sum", "__type__", string(model.MetricTypeSummary)),
						labels.FromStrings("__name__", "foobar_sum"),
					),
					ct: 1520430004000,
				}, {
					m: `foobar{quantile="0.95"}`,
					v: 123.8,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foobar", "__type__", string(model.MetricTypeSummary), "quantile", "0.95"),
						labels.FromStrings("__name__", "foobar", "quantile", "0.95"),
					),
					ct: 1520430004000,
				}, {
					m: `foobar{quantile="0.99"}`,
					v: 150.1,
					lset: typeAndUnitLabels(
						typeAndUnitEnabled,
						labels.FromStrings("__name__", "foobar", "__type__", string(model.MetricTypeSummary), "quantile", "0.99"),
						labels.FromStrings("__name__", "foobar", "quantile", "0.99"),
					),
					ct: 1520430004000,
				}, {
					m:    "metric",
					help: "foo\x00bar",
				}, {
					m:    "null_byte_metric{a=\"abc\x00\"}",
					v:    1,
					lset: todoDetectFamilySwitch(typeAndUnitEnabled, labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"), model.MetricTypeSummary),
				},
			}
			opts := []OpenMetricsOption{WithOMParserCTSeriesSkipped()}
			if typeAndUnitEnabled {
				opts = append(opts, WithOMParserTypeAndUnitLabels())
			}
			p := NewOpenMetricsParser([]byte(input), labels.NewSymbolTable(), opts...)
			got := testParse(t, p)
			requireEntries(t, exp, got)
		})
	}
}

func TestOpenMetricsParse_UTF8(t *testing.T) {
	input := `# HELP "go.gc_duration_seconds" A summary of the GC invocation durations.
# TYPE "go.gc_duration_seconds" summary
# UNIT "go.gc_duration_seconds" seconds
{"go.gc_duration_seconds",quantile="0"} 4.9351e-05
{"go.gc_duration_seconds",quantile="0.25"} 7.424100000000001e-05
{"go.gc_duration_seconds_created"} 1520872607.123
{"go.gc_duration_seconds",quantile="0.5",a="b"} 8.3835e-05
{"http.status",q="0.9",a="b"} 8.3835e-05
{"http.status",q="0.9",a="b"} 8.3835e-05
{q="0.9","http.status",a="b"} 8.3835e-05
{"go.gc_duration_seconds_sum"} 0.004304266
{"Heizölrückstoßabdämpfung 10€ metric with \"interesting\" {character\nchoices}","strange©™\n'quoted' \"name\""="6"} 10.0
quotedexemplar_count 1 # {"id.thing"="histogram-count-test"} 4
quotedexemplar2_count 1 # {"id.thing"="histogram-count-test",other="hello"} 4
`

	input += "# EOF\n"

	exp := []parsedEntry{
		{
			m:    "go.gc_duration_seconds",
			help: "A summary of the GC invocation durations.",
		}, {
			m:   "go.gc_duration_seconds",
			typ: model.MetricTypeSummary,
		}, {
			m:    "go.gc_duration_seconds",
			unit: "seconds",
		}, {
			m:    `{"go.gc_duration_seconds",quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.0"),
			ct:   1520872607123,
		}, {
			m:    `{"go.gc_duration_seconds",quantile="0.25"}`,
			v:    7.424100000000001e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.25"),
			ct:   1520872607123,
		}, {
			m:    `{"go.gc_duration_seconds",quantile="0.5",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds", "quantile", "0.5", "a", "b"),
		}, {
			m:    `{"http.status",q="0.9",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "http.status", "q", "0.9", "a", "b"),
		}, {
			m:    `{"http.status",q="0.9",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "http.status", "q", "0.9", "a", "b"),
		}, {
			m:    `{q="0.9","http.status",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "http.status", "q", "0.9", "a", "b"),
		}, {
			m:    `{"go.gc_duration_seconds_sum"}`,
			v:    0.004304266,
			lset: labels.FromStrings("__name__", "go.gc_duration_seconds_sum"),
		}, {
			m: `{"Heizölrückstoßabdämpfung 10€ metric with \"interesting\" {character\nchoices}","strange©™\n'quoted' \"name\""="6"}`,
			v: 10.0,
			lset: labels.FromStrings("__name__", `Heizölrückstoßabdämpfung 10€ metric with "interesting" {character
choices}`, "strange©™\n'quoted' \"name\"", "6"),
		}, {
			m:    `quotedexemplar_count`,
			v:    1,
			lset: labels.FromStrings("__name__", "quotedexemplar_count"),
			es: []exemplar.Exemplar{
				{Labels: labels.FromStrings("id.thing", "histogram-count-test"), Value: 4},
			},
		}, {
			m:    `quotedexemplar2_count`,
			v:    1,
			lset: labels.FromStrings("__name__", "quotedexemplar2_count"),
			es: []exemplar.Exemplar{
				{Labels: labels.FromStrings("id.thing", "histogram-count-test", "other", "hello"), Value: 4},
			},
		},
	}

	p := NewOpenMetricsParser([]byte(input), labels.NewSymbolTable(), WithOMParserCTSeriesSkipped())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetricsParseErrors(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		// Happy cases. EOF is returned by the parser at the end of valid
		// data.
		{
			input: "# EOF",
			err:   "EOF",
		},
		{
			input: "# EOF\n",
			err:   "EOF",
		},
		// Unhappy cases.
		{
			input: "",
			err:   "data does not end with # EOF",
		},
		{
			input: "\n",
			err:   "expected a valid start token, got \"\\n\" (\"INVALID\") while parsing: \"\\n\"",
		},
		{
			input: "metric",
			err:   "expected value after metric, got \"metric\" (\"EOF\") while parsing: \"metric\"",
		},
		{
			input: "metric 1",
			err:   "data does not end with # EOF",
		},
		{
			input: "metric 1\n",
			err:   "data does not end with # EOF",
		},
		{
			input: "metric_total 1 # {aa=\"bb\"} 4",
			err:   "data does not end with # EOF",
		},
		{
			input: "a\n#EOF\n",
			err:   "expected value after metric, got \"\\n\" (\"INVALID\") while parsing: \"a\\n\"",
		},
		{
			input: "\n\n#EOF\n",
			err:   "expected a valid start token, got \"\\n\" (\"INVALID\") while parsing: \"\\n\"",
		},
		{
			input: " a 1\n#EOF\n",
			err:   "expected a valid start token, got \" \" (\"INVALID\") while parsing: \" \"",
		},
		{
			input: "9\n#EOF\n",
			err:   "expected a valid start token, got \"9\" (\"INVALID\") while parsing: \"9\"",
		},
		{
			input: "# TYPE u untyped\n#EOF\n",
			err:   "invalid metric type \"untyped\"",
		},
		{
			input: "# TYPE c counter \n#EOF\n",
			err:   "invalid metric type \"counter \"",
		},
		{
			input: "#  TYPE c counter\n#EOF\n",
			err:   "expected a valid start token, got \"#  \" (\"INVALID\") while parsing: \"#  \"",
		},
		{
			input: "# TYPE \n#EOF\n",
			err:   "expected metric name after TYPE, got \"\\n\" (\"INVALID\") while parsing: \"# TYPE \\n\"",
		},
		{
			input: "# TYPE m\n#EOF\n",
			err:   "expected text in TYPE",
		},
		{
			input: "# UNIT metric suffix\n#EOF\n",
			err:   "unit \"suffix\" not a suffix of metric \"metric\"",
		},
		{
			input: "# UNIT metricsuffix suffix\n#EOF\n",
			err:   "unit \"suffix\" not a suffix of metric \"metricsuffix\"",
		},
		{
			input: "# UNIT m suffix\n#EOF\n",
			err:   "unit \"suffix\" not a suffix of metric \"m\"",
		},
		{
			input: "# UNIT \n#EOF\n",
			err:   "expected metric name after UNIT, got \"\\n\" (\"INVALID\") while parsing: \"# UNIT \\n\"",
		},
		{
			input: "# UNIT m\n#EOF\n",
			err:   "expected text in UNIT",
		},
		{
			input: "# HELP \n#EOF\n",
			err:   "expected metric name after HELP, got \"\\n\" (\"INVALID\") while parsing: \"# HELP \\n\"",
		},
		{
			input: "# HELP m\n#EOF\n",
			err:   "expected text in HELP",
		},
		{
			input: "a\t1\n#EOF\n",
			err:   "expected value after metric, got \"\\t\" (\"INVALID\") while parsing: \"a\\t\"",
		},
		{
			input: "a 1\t2\n#EOF\n",
			err:   "strconv.ParseFloat: parsing \"1\\t2\": invalid syntax while parsing: \"a 1\\t2\"",
		},
		{
			input: "a 1 2 \n#EOF\n",
			err:   "expected next entry after timestamp, got \" \\n\" (\"INVALID\") while parsing: \"a 1 2 \\n\"",
		},
		{
			input: "a 1 2 #\n#EOF\n",
			err:   "expected next entry after timestamp, got \" #\\n\" (\"TIMESTAMP\") while parsing: \"a 1 2 #\\n\"",
		},
		{
			input: "a 1 1z\n#EOF\n",
			err:   "strconv.ParseFloat: parsing \"1z\": invalid syntax while parsing: \"a 1 1z\"",
		},
		{
			input: " # EOF\n",
			err:   "expected a valid start token, got \" \" (\"INVALID\") while parsing: \" \"",
		},
		{
			input: "# EOF\na 1",
			err:   "unexpected data after # EOF",
		},
		{
			input: "# EOF\n\n",
			err:   "unexpected data after # EOF",
		},
		{
			input: "# EOFa 1",
			err:   "unexpected data after # EOF",
		},
		{
			input: "#\tTYPE c counter\n",
			err:   "expected a valid start token, got \"#\\t\" (\"INVALID\") while parsing: \"#\\t\"",
		},
		{
			input: "# TYPE c  counter\n",
			err:   "invalid metric type \" counter\"",
		},
		{
			input: "a 1 1 1\n# EOF\n",
			err:   "expected next entry after timestamp, got \" 1\\n\" (\"TIMESTAMP\") while parsing: \"a 1 1 1\\n\"",
		},
		{
			input: "a{b='c'} 1\n# EOF\n",
			err:   "expected label value, got \"'\" (\"INVALID\") while parsing: \"a{b='\"",
		},
		{
			input: "a{,b=\"c\"} 1\n# EOF\n",
			err:   "expected label name, got \",b\" (\"COMMA\") while parsing: \"a{,b\"",
		},
		{
			input: "a{b=\"c\"d=\"e\"} 1\n# EOF\n",
			err:   "expected comma or brace close, got \"d=\" (\"LNAME\") while parsing: \"a{b=\\\"c\\\"d=\"",
		},
		{
			input: "a{b=\"c\",,d=\"e\"} 1\n# EOF\n",
			err:   "expected label name, got \",d\" (\"COMMA\") while parsing: \"a{b=\\\"c\\\",,d\"",
		},
		{
			input: "a{b=\n# EOF\n",
			err:   "expected label value, got \"\\n\" (\"INVALID\") while parsing: \"a{b=\\n\"",
		},
		{
			input: "a{\xff=\"foo\"} 1\n# EOF\n",
			err:   "expected label name, got \"\\xff\" (\"INVALID\") while parsing: \"a{\\xff\"",
		},
		{
			input: "a{b=\"\xff\"} 1\n# EOF\n",
			err:   "invalid UTF-8 label value: \"\\\"\\xff\\\"\"",
		},
		{
			input: `{"a","b = "c"}
# EOF
`,
			err: "expected equal, got \"c\\\"\" (\"LNAME\") while parsing: \"{\\\"a\\\",\\\"b = \\\"c\\\"\"",
		},
		{
			input: `{"a",b\nc="d"} 1
# EOF
`,
			err: "expected equal, got \"\\\\\" (\"INVALID\") while parsing: \"{\\\"a\\\",b\\\\\"",
		},
		{
			input: "a true\n",
			err:   "strconv.ParseFloat: parsing \"true\": invalid syntax while parsing: \"a true\"",
		},
		{
			input: "something_weird{problem=\"\n# EOF\n",
			err:   "expected label value, got \"\\\"\\n\" (\"INVALID\") while parsing: \"something_weird{problem=\\\"\\n\"",
		},
		{
			input: "empty_label_name{=\"\"} 0\n# EOF\n",
			err:   "expected label name, got \"=\\\"\" (\"EQUAL\") while parsing: \"empty_label_name{=\\\"\"",
		},
		{
			input: "foo 1_2\n\n# EOF\n",
			err:   "unsupported character in float while parsing: \"foo 1_2\"",
		},
		{
			input: "foo 0x1p-3\n\n# EOF\n",
			err:   "unsupported character in float while parsing: \"foo 0x1p-3\"",
		},
		{
			input: "foo 0x1P-3\n\n# EOF\n",
			err:   "unsupported character in float while parsing: \"foo 0x1P-3\"",
		},
		{
			input: "foo 0 1_2\n\n# EOF\n",
			err:   "unsupported character in float while parsing: \"foo 0 1_2\"",
		},
		{
			input: "custom_metric_total 1 # {aa=bb}\n# EOF\n",
			err:   "expected label value, got \"b\" (\"INVALID\") while parsing: \"custom_metric_total 1 # {aa=b\"",
		},
		{
			input: "custom_metric_total 1 # {aa=\"bb\"}\n# EOF\n",
			err:   "expected value after exemplar labels, got \"\\n\" (\"INVALID\") while parsing: \"custom_metric_total 1 # {aa=\\\"bb\\\"}\\n\"",
		},
		{
			input: `custom_metric_total 1 # {aa="bb"}`,
			err:   "expected value after exemplar labels, got \"}\" (\"EOF\") while parsing: \"custom_metric_total 1 # {aa=\\\"bb\\\"}\"",
		},
		{
			input: `custom_metric_total 1 # {bb}`,
			err:   "expected label name, got \"}\" (\"BCLOSE\") while parsing: \"custom_metric_total 1 # {bb}\"",
		},
		{
			input: `custom_metric_total 1 # {bb, a="dd"}`,
			err:   "expected label name, got \", \" (\"COMMA\") while parsing: \"custom_metric_total 1 # {bb, \"",
		},
		{
			input: `custom_metric_total 1 # {aa="bb",,cc="dd"} 1`,
			err:   "expected label name, got \",c\" (\"COMMA\") while parsing: \"custom_metric_total 1 # {aa=\\\"bb\\\",,c\"",
		},
		{
			input: `custom_metric_total 1 # {aa="bb"} 1_2`,
			err:   "unsupported character in float while parsing: \"custom_metric_total 1 # {aa=\\\"bb\\\"} 1_2\"",
		},
		{
			input: `custom_metric_total 1 # {aa="bb"} 0x1p-3`,
			err:   "unsupported character in float while parsing: \"custom_metric_total 1 # {aa=\\\"bb\\\"} 0x1p-3\"",
		},
		{
			input: `custom_metric_total 1 # {aa="bb"} true`,
			err:   "strconv.ParseFloat: parsing \"true\": invalid syntax while parsing: \"custom_metric_total 1 # {aa=\\\"bb\\\"} true\"",
		},
		{
			input: `custom_metric_total 1 # {aa="bb",cc=}`,
			err:   "expected label value, got \"}\" (\"INVALID\") while parsing: \"custom_metric_total 1 # {aa=\\\"bb\\\",cc=}\"",
		},
		{
			input: `custom_metric_total 1 # {aa=\"\xff\"} 9.0`,
			err:   "expected label value, got \"\\\\\" (\"INVALID\") while parsing: \"custom_metric_total 1 # {aa=\\\\\"",
		},
		{
			input: `{b="c",} 1`,
			err:   "metric name not set while parsing: \"{b=\\\"c\\\",} 1\"",
		},
		{
			input: `a 1 NaN`,
			err:   `invalid timestamp NaN`,
		},
		{
			input: `a 1 -Inf`,
			err:   `invalid timestamp -Inf`,
		},
		{
			input: `a 1 Inf`,
			err:   `invalid timestamp +Inf`,
		},
		{
			input: "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 NaN",
			err:   `invalid exemplar timestamp NaN`,
		},
		{
			input: "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 -Inf",
			err:   `invalid exemplar timestamp -Inf`,
		},
	}

	for i, c := range cases {
		p := NewOpenMetricsParser([]byte(c.input), labels.NewSymbolTable(), WithOMParserCTSeriesSkipped())
		var err error
		for err == nil {
			_, err = p.Next()
		}
		require.Equal(t, c.err, err.Error(), "test %d: %s", i, c.input)
	}
}

func TestOMNullByteHandling(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		{
			input: "null_byte_metric{a=\"abc\x00\"} 1\n# EOF\n",
			err:   "",
		},
		{
			input: "a{b=\"\x00ss\"} 1\n# EOF\n",
			err:   "",
		},
		{
			input: "a{b=\"\x00\"} 1\n# EOF\n",
			err:   "",
		},
		{
			input: "a{b=\"\x00\"} 1\n# EOF",
			err:   "",
		},
		{
			input: "a{b=\x00\"ssss\"} 1\n# EOF\n",
			err:   "expected label value, got \"\\x00\" (\"INVALID\") while parsing: \"a{b=\\x00\"",
		},
		{
			input: "a{b=\"\x00",
			err:   "expected label value, got \"\\\"\\x00\" (\"INVALID\") while parsing: \"a{b=\\\"\\x00\"",
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
			input: "#",
			err:   "expected a valid start token, got \"#\" (\"INVALID\") while parsing: \"#\"",
		},
		{
			input: "# H",
			err:   "expected a valid start token, got \"# H\" (\"INVALID\") while parsing: \"# H\"",
		},
		{
			input: "custom_metric_total 1 # {b=\x00\"ssss\"} 1\n",
			err:   "expected label value, got \"\\x00\" (\"INVALID\") while parsing: \"custom_metric_total 1 # {b=\\x00\"",
		},
		{
			input: "custom_metric_total 1 # {b=\"\x00ss\"} 1\n",
			err:   "expected label value, got \"\\\"\\x00\" (\"INVALID\") while parsing: \"custom_metric_total 1 # {b=\\\"\\x00\"",
		},
	}

	for i, c := range cases {
		p := NewOpenMetricsParser([]byte(c.input), labels.NewSymbolTable(), WithOMParserCTSeriesSkipped())
		var err error
		for err == nil {
			_, err = p.Next()
		}

		if c.err == "" {
			require.ErrorIs(t, err, io.EOF, "test %d", i)
			continue
		}

		require.EqualError(t, err, c.err, "test %d", i)
	}
}

// TestCTParseFailures tests known failure edge cases, we know does not work due
// current OM spec limitations or clients with broken OM format.
// TODO(maniktherana): Make sure OM 1.1/2.0 pass CT via metadata or exemplar-like to avoid this.
func TestCTParseFailures(t *testing.T) {
	for _, tcase := range []struct {
		name     string
		input    string
		expected []parsedEntry
	}{
		{
			name: "_created line is a first one",
			input: `# HELP thing histogram with _created as first line
# TYPE thing histogram
thing_created 1520872607.123
thing_count 17
thing_sum 324789.3
thing_bucket{le="0.0"} 0
thing_bucket{le="+Inf"} 17
# HELP thing_c counter with _created as first line
# TYPE thing_c counter
thing_c_created 1520872607.123
thing_c_total 14123.232
# EOF
`,
			expected: []parsedEntry{
				{
					m:    "thing",
					help: "histogram with _created as first line",
				},
				{
					m:   "thing",
					typ: model.MetricTypeHistogram,
				},
				{
					m:  `thing_count`,
					ct: 0, // Should be int64p(1520872607123).
				},
				{
					m:  `thing_sum`,
					ct: 0, // Should be int64p(1520872607123).
				},
				{
					m:  `thing_bucket{le="0.0"}`,
					ct: 0, // Should be int64p(1520872607123).
				},
				{
					m:  `thing_bucket{le="+Inf"}`,
					ct: 0, // Should be int64p(1520872607123),
				},
				{
					m:    "thing_c",
					help: "counter with _created as first line",
				},
				{
					m:   "thing_c",
					typ: model.MetricTypeCounter,
				},
				{
					m:  `thing_c_total`,
					ct: 0, // Should be int64p(1520872607123).
				},
			},
		},
		{
			// TODO(bwplotka): Kind of correct bevaviour? If yes, let's move to the OK tests above.
			name: "maybe counter with no meta",
			input: `foo_total 17.0
foo_created 1520872607.123
foo_total{a="b"} 17.0
foo_created{a="b"} 1520872608.123
# EOF
`,
			expected: []parsedEntry{
				{
					m: `foo_total`,
				},
				{
					m: `foo_created`,
				},
				{
					m: `foo_total{a="b"}`,
				},
				{
					m: `foo_created{a="b"}`,
				},
			},
		},
	} {
		t.Run(fmt.Sprintf("case=%v", tcase.name), func(t *testing.T) {
			p := NewOpenMetricsParser([]byte(tcase.input), labels.NewSymbolTable(), WithOMParserCTSeriesSkipped())
			got := testParse(t, p)
			resetValAndLset(got) // Keep this test focused on metric, basic entries and CT only.
			requireEntries(t, tcase.expected, got)
		})
	}
}

func resetValAndLset(e []parsedEntry) {
	for i := range e {
		e[i].v = 0
		e[i].lset = labels.EmptyLabels()
	}
}
