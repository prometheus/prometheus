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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
)

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
hhh_bucket{le="+Inf"} 1 # {aa="bb"} 4
# TYPE ggh gaugehistogram
ggh_bucket{le="+Inf"} 1 # {cc="dd",xx="yy"} 4 123.123
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
# TYPE foo counter
foo_total 17.0 1520879607.789 # {xx="yy"} 5`

	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1"
	input += "\n# EOF\n"

	int64p := func(x int64) *int64 { return &x }

	exp := []struct {
		lset    labels.Labels
		m       string
		t       *int64
		v       float64
		typ     MetricType
		help    string
		unit    string
		comment string
		e       *exemplar.Exemplar
	}{
		{
			m:    "go_gc_duration_seconds",
			help: "A summary of the GC invocation durations.",
		}, {
			m:   "go_gc_duration_seconds",
			typ: MetricTypeSummary,
		}, {
			m:    "go_gc_duration_seconds",
			unit: "seconds",
		}, {
			m:    `go_gc_duration_seconds{quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0"),
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
			typ: MetricTypeGauge,
		}, {
			m:    `go_goroutines`,
			v:    33,
			t:    int64p(123123),
			lset: labels.FromStrings("__name__", "go_goroutines"),
		}, {
			m:   "hh",
			typ: MetricTypeHistogram,
		}, {
			m:    `hh_bucket{le="+Inf"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "hh_bucket", "le", "+Inf"),
		}, {
			m:   "gh",
			typ: MetricTypeGaugeHistogram,
		}, {
			m:    `gh_bucket{le="+Inf"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "gh_bucket", "le", "+Inf"),
		}, {
			m:   "hhh",
			typ: MetricTypeHistogram,
		}, {
			m:    `hhh_bucket{le="+Inf"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "hhh_bucket", "le", "+Inf"),
			e:    &exemplar.Exemplar{Labels: labels.FromStrings("aa", "bb"), Value: 4},
		}, {
			m:   "ggh",
			typ: MetricTypeGaugeHistogram,
		}, {
			m:    `ggh_bucket{le="+Inf"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "ggh_bucket", "le", "+Inf"),
			e:    &exemplar.Exemplar{Labels: labels.FromStrings("cc", "dd", "xx", "yy"), Value: 4, HasTs: true, Ts: 123123},
		}, {
			m:   "ii",
			typ: MetricTypeInfo,
		}, {
			m:    `ii{foo="bar"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "ii", "foo", "bar"),
		}, {
			m:   "ss",
			typ: MetricTypeStateset,
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
			typ: MetricTypeUnknown,
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
			m:   "foo",
			typ: MetricTypeCounter,
		}, {
			m:    "foo_total",
			v:    17,
			lset: labels.FromStrings("__name__", "foo_total"),
			t:    int64p(1520879607789),
			e:    &exemplar.Exemplar{Labels: labels.FromStrings("xx", "yy"), Value: 5},
		}, {
			m:    "metric",
			help: "foo\x00bar",
		}, {
			m:    "null_byte_metric{a=\"abc\x00\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"),
		},
	}

	p := NewOpenMetricsParser([]byte(input))
	i := 0

	var res labels.Labels

	for {
		et, err := p.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		switch et {
		case EntrySeries:
			m, ts, v := p.Series()

			var e exemplar.Exemplar
			p.Metric(&res)
			found := p.Exemplar(&e)
			require.Equal(t, exp[i].m, string(m))
			if e.HasTs {
				require.Equal(t, exp[i].t, ts)
			}
			require.Equal(t, exp[i].v, v)
			require.Equal(t, exp[i].lset, res)
			if exp[i].e == nil {
				require.Equal(t, false, found)
			} else {
				require.Equal(t, true, found)
				require.Equal(t, *exp[i].e, e)
			}
			res = res[:0]

		case EntryType:
			m, typ := p.Type()
			require.Equal(t, exp[i].m, string(m))
			require.Equal(t, exp[i].typ, typ)

		case EntryHelp:
			m, h := p.Help()
			require.Equal(t, exp[i].m, string(m))
			require.Equal(t, exp[i].help, string(h))

		case EntryUnit:
			m, u := p.Unit()
			require.Equal(t, exp[i].m, string(m))
			require.Equal(t, exp[i].unit, string(u))

		case EntryComment:
			require.Equal(t, exp[i].comment, string(p.Comment()))
		}

		i++
	}
	require.Equal(t, len(exp), i)
}

func TestOpenMetricsParseErrors(t *testing.T) {
	cases := []struct {
		input  string
		err    string
		line   int
		column int
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
			input:  "",
			err:    "data does not end with # EOF",
			line:   1,
			column: 1,
		},
		{
			input:  "\n",
			err:    "\"INVALID\" \"\\n\" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "metric",
			err:    "expected value after metric, got \"EOF\"",
			line:   1,
			column: 1,
		},
		{
			input:  "metric 1",
			err:    "data does not end with # EOF",
			line:   1,
			column: 7,
		},
		{
			input:  "metric 1\n",
			err:    "data does not end with # EOF",
			line:   2,
			column: 1,
		},
		{
			input:  "metric_total 1 # {aa=\"bb\"} 4",
			err:    "data does not end with # EOF",
			line:   1,
			column: 27,
		},
		{
			input:  "a\n#EOF\n",
			err:    "expected value after metric, got \"INVALID\"",
			line:   1,
			column: 2,
		},
		{
			input:  "\n\n#EOF\n",
			err:    "\"INVALID\" \"\\n\" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  " a 1\n#EOF\n",
			err:    "\"INVALID\" \" \" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "9\n#EOF\n",
			err:    "\"INVALID\" \"9\" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "# TYPE u untyped\n#EOF\n",
			err:    "invalid metric type \"untyped\"",
			line:   2,
			column: 1,
		},
		{
			input:  "# TYPE c counter \n#EOF\n",
			err:    "invalid metric type \"counter \"",
			line:   2,
			column: 1,
		},
		{
			input:  "#  TYPE c counter\n#EOF\n",
			err:    "\"INVALID\" \" \" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "# TYPE \n#EOF\n",
			err:    "expected metric name after TYPE, got \"INVALID\"",
			line:   1,
			column: 8,
		},
		{
			input:  "# TYPE m\n#EOF\n",
			err:    "expected text in TYPE",
			line:   1,
			column: 9,
		},
		{
			input:  "# UNIT metric suffix\n#EOF\n",
			err:    "unit not a suffix of metric \"metric\"",
			line:   2,
			column: 1,
		},
		{
			input:  "# UNIT metricsuffix suffix\n#EOF\n",
			err:    "unit not a suffix of metric \"metricsuffix\"",
			line:   2,
			column: 1,
		},
		{
			input:  "# UNIT m suffix\n#EOF\n",
			err:    "unit not a suffix of metric \"m\"",
			line:   2,
			column: 1,
		},
		{
			input:  "# UNIT \n#EOF\n",
			err:    "expected metric name after UNIT, got \"INVALID\"",
			line:   1,
			column: 8,
		},
		{
			input:  "# UNIT m\n#EOF\n",
			err:    "expected text in UNIT",
			line:   1,
			column: 9,
		},
		{
			input:  "# HELP \n#EOF\n",
			err:    "expected metric name after HELP, got \"INVALID\"",
			line:   1,
			column: 8,
		},
		{
			input:  "# HELP m\n#EOF\n",
			err:    "expected text in HELP",
			line:   1,
			column: 9,
		},
		{
			input:  "a\t1\n#EOF\n",
			err:    "expected value after metric, got \"INVALID\"",
			line:   1,
			column: 2,
		},
		{
			input:  "a 1\t2\n#EOF\n",
			err:    "strconv.ParseFloat: parsing \"1\\t2\": invalid syntax",
			line:   1,
			column: 2,
		},
		{
			input:  "a 1 2 \n#EOF\n",
			err:    "expected next entry after timestamp, got \"INVALID\"",
			line:   1,
			column: 6,
		},
		{
			input:  "a 1 2 #\n#EOF\n",
			err:    "expected next entry after timestamp, got \"TIMESTAMP\"",
			line:   1,
			column: 6,
		},
		{
			input:  "a 1 1z\n#EOF\n",
			err:    "strconv.ParseFloat: parsing \"1z\": invalid syntax",
			line:   1,
			column: 4,
		},
		{
			input:  " # EOF\n",
			err:    "\"INVALID\" \" \" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "# EOF\na 1",
			err:    "unexpected data after # EOF",
			line:   2,
			column: 1,
		},
		{
			input:  "# EOF\n\n",
			err:    "unexpected data after # EOF",
			line:   2,
			column: 1,
		},
		{
			input:  "# EOFa 1",
			err:    "unexpected data after # EOF",
			line:   1,
			column: 6,
		},
		{
			input:  "#\tTYPE c counter\n",
			err:    "\"INVALID\" \"\\t\" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "# TYPE c  counter\n",
			err:    "invalid metric type \" counter\"",
			line:   2,
			column: 1,
		},
		{
			input:  "a 1 1 1\n# EOF\n",
			err:    "expected next entry after timestamp, got \"TIMESTAMP\"",
			line:   1,
			column: 6,
		},
		{
			input:  "a{b='c'} 1\n# EOF\n",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 5,
		},
		{
			input:  "a{b=\"c\",} 1\n# EOF\n",
			err:    "expected label name, got \"BCLOSE\"",
			line:   1,
			column: 9,
		},
		{
			input:  "a{,b=\"c\"} 1\n# EOF\n",
			err:    "expected label name or left brace, got \"COMMA\"",
			line:   1,
			column: 3,
		},
		{
			input:  "a{b=\"c\"d=\"e\"} 1\n# EOF\n",
			err:    "expected comma, got \"LNAME\"",
			line:   1,
			column: 8,
		},
		{
			input:  "a{b=\"c\",,d=\"e\"} 1\n# EOF\n",
			err:    "expected label name, got \"COMMA\"",
			line:   1,
			column: 9,
		},
		{
			input:  "a{b=\n# EOF\n",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 5,
		},
		{
			input:  "a{\xff=\"foo\"} 1\n# EOF\n",
			err:    "expected label name or left brace, got \"INVALID\"",
			line:   1,
			column: 3,
		},
		{
			input:  "a{b=\"\xff\"} 1\n# EOF\n",
			err:    "invalid UTF-8 label value",
			line:   1,
			column: 5,
		},
		{
			input:  "a true\n",
			err:    "strconv.ParseFloat: parsing \"true\": invalid syntax",
			line:   1,
			column: 2,
		},
		{
			input:  "something_weird{problem=\"\n# EOF\n",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 25,
		},
		{
			input:  "empty_label_name{=\"\"} 0\n# EOF\n",
			err:    "expected label name or left brace, got \"EQUAL\"",
			line:   1,
			column: 18,
		},
		{
			input:  "foo 1_2\n\n# EOF\n",
			err:    "unsupported character in float",
			line:   1,
			column: 4,
		},
		{
			input:  "foo 0x1p-3\n\n# EOF\n",
			err:    "unsupported character in float",
			line:   1,
			column: 4,
		},
		{
			input:  "foo 0x1P-3\n\n# EOF\n",
			err:    "unsupported character in float",
			line:   1,
			column: 4,
		},
		{
			input:  "foo 0 1_2\n\n# EOF\n",
			err:    "unsupported character in float",
			line:   1,
			column: 6,
		},
		{
			input:  "custom_metric_total 1 # {aa=bb}\n# EOF\n",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 29,
		},
		{
			input:  "custom_metric_total 1 # {aa=\"bb\"}\n# EOF\n",
			err:    "expected value after exemplar labels, got \"INVALID\"",
			line:   1,
			column: 34,
		},
		{
			input:  `custom_metric_total 1 # {aa="bb"}`,
			err:    "expected value after exemplar labels, got \"EOF\"",
			line:   1,
			column: 33,
		},
		{
			input:  `custom_metric 1 # {aa="bb"}`,
			err:    "metric name custom_metric does not support exemplars",
			line:   1,
			column: 16,
		},
		{
			input:  `custom_metric_total 1 # {aa="bb",,cc="dd"} 1`,
			err:    "expected label name, got \"COMMA\"",
			line:   1,
			column: 34,
		},
		{
			input:  `custom_metric_total 1 # {aa="bb"} 1_2`,
			err:    "unsupported character in float",
			line:   1,
			column: 34,
		},
		{
			input:  `custom_metric_total 1 # {aa="bb"} 0x1p-3`,
			err:    "unsupported character in float",
			line:   1,
			column: 34,
		},
		{
			input:  `custom_metric_total 1 # {aa="bb"} true`,
			err:    "strconv.ParseFloat: parsing \"true\": invalid syntax",
			line:   1,
			column: 34,
		},
		{
			input:  `custom_metric_total 1 # {aa="bb",cc=}`,
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 37,
		},
		{
			input:  `custom_metric_total 1 # {aa=\"\xff\"} 9.0`,
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 29,
		},
		{
			input:  `{b="c",} 1`,
			err:    `"INVALID" "{" is not a valid start token`,
			line:   1,
			column: 1,
		},
		{
			input:  `a 1 NaN`,
			err:    `invalid timestamp`,
			line:   1,
			column: 4,
		},
		{
			input:  `a 1 -Inf`,
			err:    `invalid timestamp`,
			line:   1,
			column: 4,
		},
		{
			input:  `a 1 Inf`,
			err:    `invalid timestamp`,
			line:   1,
			column: 4,
		},
		{
			input:  "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 NaN",
			err:    `invalid exemplar timestamp`,
			line:   2,
			column: 38,
		},
		{
			input:  "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 -Inf",
			err:    `invalid exemplar timestamp`,
			line:   2,
			column: 38,
		},
		{
			input:  "# TYPE hhh histogram\nhhh_bucket{le=\"+Inf\"} 1 # {aa=\"bb\"} 4 Inf",
			err:    `invalid exemplar timestamp`,
			line:   2,
			column: 38,
		},
	}

	for i, c := range cases {
		p := NewOpenMetricsParser([]byte(c.input))
		var err error
		for err == nil {
			_, err = p.Next()
		}
		expectedErr := fmt.Sprintf(c.err+" (line %d, column %d)", c.line, c.column)
		if c.err != "EOF" {
			require.Equal(t, expectedErr, err.Error(), "test %d: %s", i, c.input)
		} else {
			require.Equal(t, c.err, err.Error(), "test %d: %s", i, c.input)
		}
	}
}

func TestOMNullByteHandling(t *testing.T) {
	cases := []struct {
		input  string
		err    string
		line   int
		column int
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
			input:  "a{b=\x00\"ssss\"} 1\n# EOF\n",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 5,
		},
		{
			input:  "a{b=\"\x00",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 5,
		},
		{
			input: "a{b\x00=\"hiih\"}	1",
			err:    "expected equal, got \"INVALID\"",
			line:   1,
			column: 4,
		},
		{
			input:  "a\x00{b=\"ddd\"} 1",
			err:    "expected value after metric, got \"INVALID\"",
			line:   1,
			column: 2,
		},
		{
			input:  "#",
			err:    "\"INVALID\" \" \" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "# H",
			err:    "\"INVALID\" \" \" is not a valid start token",
			line:   1,
			column: 1,
		},
		{
			input:  "custom_metric_total 1 # {b=\x00\"ssss\"} 1\n",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 28,
		},
		{
			input:  "custom_metric_total 1 # {b=\"\x00ss\"} 1\n",
			err:    "expected label value, got \"INVALID\"",
			line:   1,
			column: 28,
		},
	}

	for i, c := range cases {
		p := NewOpenMetricsParser([]byte(c.input))
		var err error
		for err == nil {
			_, err = p.Next()
		}

		if c.err == "" {
			require.Equal(t, io.EOF, err, "test %d", i)
			continue
		}
		expectedErr := fmt.Sprintf(c.err+" (line %d, column %d)", c.line, c.column)
		require.Equal(t, expectedErr, err.Error(), "test %d: %s", i, c.input)
	}
}
