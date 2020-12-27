// Copyright 2020 The Prometheus Authors
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

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

func TestPromDumpParse(t *testing.T) {
	input := `{__name__="go_gc_duration_seconds", instance="localhost:9090", job="prometheus", quantile="0"} 4.7791e-05 1608996500647
{__name__="some:aggregate:rate5m", a_b="c"} 1 1608996530647
{__name__="_metric_starting_with_underscore", _label_starting_with_underscore="foo"} 1 1608996530647

  {__name__="whitespaced_metric"} 10 1608996530647
{__name__="metric", foo="bar"} 10 1608996530647
{__name__="metric", foo="bar", bar="foo"} 20 1608996545647
{__name__="metricWithNoLabels"} 1 1608996560647
{__name__="testmetric", label="\"bar\""} 1 1608996560647`
	input += "\n{__name__=\"null_byte_metric\", a=\"abc\x00\"} 1 1608996560647"

	int64p := func(x int64) *int64 { return &x }

	exp := []struct {
		lset      labels.Labels
		metric    string
		timestamp *int64
		value     float64
	}{
		{
			metric:    "go_gc_duration_seconds",
			timestamp: int64p(1608996500647),
			value:     4.7791e-05,
			lset: labels.FromStrings(
				"__name__", "go_gc_duration_seconds",
				"instance", "localhost:9090",
				"job", "prometheus",
				"quantile", "0",
			),
		},
		{
			metric:    "some:aggregate:rate5m",
			timestamp: int64p(1608996530647),
			value:     1,
			lset: labels.FromStrings(
				"__name__", "some:aggregate:rate5m",
				"a_b", "c",
			),
		},
		{
			metric:    "_metric_starting_with_underscore",
			timestamp: int64p(1608996530647),
			value:     1,
			lset: labels.FromStrings(
				"__name__", "_metric_starting_with_underscore",
				"_label_starting_with_underscore", "foo",
			),
		},
		{
			metric:    "whitespaced_metric",
			timestamp: int64p(1608996530647),
			value:     10,
			lset: labels.FromStrings(
				"__name__", "whitespaced_metric",
			),
		},
		{
			metric:    "metric",
			timestamp: int64p(1608996530647),
			value:     10,
			lset: labels.FromStrings(
				"__name__", "metric",
				"foo", "bar",
			),
		},
		{
			metric:    "metric",
			timestamp: int64p(1608996545647),
			value:     20,
			lset: labels.FromStrings(
				"__name__", "metric",
				"foo", "bar",
				"bar", "foo",
			),
		},
		{
			metric:    "metricWithNoLabels",
			timestamp: int64p(1608996560647),
			value:     1,
			lset: labels.FromStrings(
				"__name__", "metricWithNoLabels",
			),
		},
		{
			metric:    "testmetric",
			timestamp: int64p(1608996560647),
			value:     1,
			lset: labels.FromStrings(
				"__name__", "testmetric",
				"label", "\"bar\"",
			),
		},
		{
			metric:    "null_byte_metric",
			timestamp: int64p(1608996560647),
			value:     1,
			lset: labels.FromStrings(
				"__name__", "null_byte_metric",
				"a", "abc\x00",
			),
		},
	}

	p := NewPromDumpParser([]byte(input))
	i := 0

	var labelSet labels.Labels

	for {
		entry, err := p.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		switch entry {
		case EntrySeries:
			metric, timestamp, value := p.Series()

			p.Metric(&labelSet)

			require.Equal(t, exp[i].metric, string(metric))
			require.Equal(t, exp[i].timestamp, timestamp)
			require.Equal(t, exp[i].value, value)
			require.Equal(t, exp[i].lset, labelSet)
			labelSet = labelSet[:0]
		}

		i++
	}
	require.Equal(t, len(exp), i)
}

func TestPromDumpParseErrors(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		{
			input: "metric{foo=\"bar\"} 1 1608996560647",
			err:   "\"INVALID\" is not a valid start token",
		},
		{
			// __name__ is always the first label
			input: "{foo=\"bar\", __name__=\"metric\"} 1 1608996560647",
			err:   "expected __name__ label after open brace, got \"LNAME\"",
		},
		{
			input: "{__name__=\n",
			err:   "expected valid metric name after __name__ label, got \"INVALID\"",
		},
		{
			input: "{__name__=\"metric\", foo=\"\xff\"} 1 1608996560647",
			err:   "invalid UTF-8 label value",
		},
		{
			input: "{__name__=\"metric\", \xff=\"foo\"} 1 1608996560647",
			err:   "expected label name, got \"INVALID\"",
		},
		{
			input: "{__name__=\"boolean_metric\"} true\n",
			err:   "strconv.ParseFloat: parsing \"true\": invalid syntax",
		},
		{
			input: "{__name__=\"metric\"} 1_2 1608996560647",
			err:   "unsupported character in float",
		},
		{
			input: "{__name__=\"metric\"} 0x1p-3 1608996560647",
			err:   "unsupported character in float",
		},
		{
			input: "{__name__=\"metric\"} 0x1P-3 1608996560647",
			err:   "unsupported character in float",
		},
		{
			input: "{__name__=\"empty_label_name\", =\"\"} 1 1608996560647",
			err:   "expected label name, got \"EQUAL\"",
		},
		{
			input: "{__name__=\"metric_without_timestamp\"} 0",
			err:   "expected timestamp after metric value, got \"LINEBREAK\"",
		},
		{
			input: "{__name__=\"metric\"} 1 not_a_timestamp",
			err:   "expected timestamp after metric value, got \"INVALID\"",
		},
	}

	for i, c := range cases {
		p := NewPromDumpParser([]byte(c.input))
		var err error
		for err == nil {
			_, err = p.Next()
		}
		require.Error(t, err)
		require.Equal(t, c.err, err.Error(), "test %d", i)
	}
}

func TestPromDumpNullByteHandling(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		{
			input: "{__name__=\"null_byte_metric\", foo=\"bar\x00\"} 1 1608996560647",
			err:   "",
		},
		{
			input: "{__name__=\"null_byte_metric\", b=\"\x00ss\"} 1 1608996560647",
			err:   "",
		},
		{
			input: "{__name__=\"null_byte_metric\", b=\"\x00\"} 1 1608996560647",
			err:   "",
		},
		{
			input: "{__name__=\"null_byte_metric\", b=\x00\"foo\"} 1 1608996560647",
			err:   "expected label value, got \"INVALID\"",
		},
		{
			input: "{__name__=\"null_byte_metric\", b=\"\x00",
			err:   "expected label value, got \"INVALID\"",
		},
		{
			input: "{__name__=\"null_byte_metric\", b\x00=\"foo\"}	1 1608996560647",
			err: "expected equal, got \"INVALID\"",
		},
		{
			input: "{__name__=\"\x00\"} 1 1608996560647",
			err:   "expected valid metric name after __name__ label, got \"INVALID\"",
		},
	}

	for i, c := range cases {
		p := NewPromDumpParser([]byte(c.input))
		var err error
		for err == nil {
			_, err = p.Next()
		}

		if c.err == "" {
			require.Equal(t, io.EOF, err, "test %d", i)
			continue
		}

		require.Error(t, err)
		require.Equal(t, c.err, err.Error(), "test %d", i)
	}
}
