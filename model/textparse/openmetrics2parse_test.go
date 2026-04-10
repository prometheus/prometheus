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
	"io"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

func TestOpenMetrics2Parse(t *testing.T) {
	input := `# HELP go_gc_duration_seconds A summary of GC invocation durations.
# TYPE go_gc_duration_seconds summary
# UNIT go_gc_duration_seconds seconds
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds_count 99
go_gc_duration_seconds_sum 0.004
# TYPE go_goroutines gauge
go_goroutines 33 123456.789
# TYPE hh histogram
hh_bucket{le="+Inf"} 1
# TYPE ii info
ii{foo="bar"} 1
# TYPE ss stateset
ss{ss="foo"} 1
ss{ss="bar"} 0
# TYPE un unknown
un 1
# TYPE foo counter
foo_total 17.0 1520879607.789 # {id="counter-test"} 5
# EOF
`

	exp := []parsedEntry{
		{m: "go_gc_duration_seconds", help: "A summary of GC invocation durations."},
		{m: "go_gc_duration_seconds", typ: model.MetricTypeSummary},
		{m: "go_gc_duration_seconds", unit: "seconds"},
		{
			m:    `go_gc_duration_seconds{quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0.0"),
		},
		{
			m:    `go_gc_duration_seconds{quantile="0.5",a="b"}`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "a", "b", "quantile", "0.5"),
		},
		{
			m:    "go_gc_duration_seconds_count",
			v:    99,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds_count"),
		},
		{
			m:    "go_gc_duration_seconds_sum",
			v:    0.004,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds_sum"),
		},
		{m: "go_goroutines", typ: model.MetricTypeGauge},
		{
			m:    "go_goroutines",
			v:    33,
			t:    int64p(123456789),
			lset: labels.FromStrings("__name__", "go_goroutines"),
		},
		{m: "hh", typ: model.MetricTypeHistogram},
		{
			m:    `hh_bucket{le="+Inf"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "hh_bucket", "le", "+Inf"),
		},
		{m: "ii", typ: model.MetricTypeInfo},
		{
			m:    `ii{foo="bar"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "ii", "foo", "bar"),
		},
		{m: "ss", typ: model.MetricTypeStateset},
		{
			m:    `ss{ss="foo"}`,
			v:    1,
			lset: labels.FromStrings("__name__", "ss", "ss", "foo"),
		},
		{
			m:    `ss{ss="bar"}`,
			v:    0,
			lset: labels.FromStrings("__name__", "ss", "ss", "bar"),
		},
		{m: "un", typ: model.MetricTypeUnknown},
		{
			m:    "un",
			v:    1,
			lset: labels.FromStrings("__name__", "un"),
		},
		{m: "foo", typ: model.MetricTypeCounter},
		{
			m:    "foo_total",
			v:    17.0,
			t:    int64p(1520879607789),
			lset: labels.FromStrings("__name__", "foo_total"),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "counter-test"), Value: 5}},
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseStartTimestamp(t *testing.T) {
	input := `# TYPE requests counter
requests_total 42.0 1000000.0 st@500000.0
# EOF
`
	exp := []parsedEntry{
		{m: "requests", typ: model.MetricTypeCounter},
		{
			m:    "requests_total",
			v:    42.0,
			t:    int64p(1000000000),
			st:   500000000,
			lset: labels.FromStrings("__name__", "requests_total"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseStartTimestampNoMainTimestamp(t *testing.T) {
	input := `# TYPE requests counter
requests_total 42.0 st@500000.0
# EOF
`
	exp := []parsedEntry{
		{m: "requests", typ: model.MetricTypeCounter},
		{
			m:    "requests_total",
			v:    42.0,
			st:   500000000,
			lset: labels.FromStrings("__name__", "requests_total"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseMultipleExemplars(t *testing.T) {
	input := `# TYPE foo counter
foo_total 17.0 # {id="a"} 1.0 # {id="b"} 2.0 1234567.0
# EOF
`
	exp := []parsedEntry{
		{m: "foo", typ: model.MetricTypeCounter},
		{
			m:    "foo_total",
			v:    17.0,
			lset: labels.FromStrings("__name__", "foo_total"),
			es: []exemplar.Exemplar{
				{Labels: labels.FromStrings("id", "a"), Value: 1.0},
				{Labels: labels.FromStrings("id", "b"), Value: 2.0, HasTs: true, Ts: 1234567000},
			},
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseCompositeSummary(t *testing.T) {
	input := `# HELP rpc_duration_seconds RPC duration.
# TYPE rpc_duration_seconds summary
rpc_duration_seconds {count:100,sum:30000.0,quantile:[0.5:100.0,0.9:200.0,0.99:300.0]}
# EOF
`
	exp := []parsedEntry{
		{m: "rpc_duration_seconds", help: "RPC duration."},
		{m: "rpc_duration_seconds", typ: model.MetricTypeSummary},
		// Exploded series: _count, _sum, quantile entries.
		{
			m:    "rpc_duration_seconds_count",
			v:    100,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds_count"),
		},
		{
			m:    "rpc_duration_seconds_sum",
			v:    30000.0,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds_sum"),
		},
		{
			m:    "rpc_duration_seconds",
			v:    100.0,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds", "quantile", "0.5"),
		},
		{
			m:    "rpc_duration_seconds",
			v:    200.0,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds", "quantile", "0.9"),
		},
		{
			m:    "rpc_duration_seconds",
			v:    300.0,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds", "quantile", "0.99"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseCompositeClassicHistogram(t *testing.T) {
	input := `# HELP req_duration Request duration histogram.
# TYPE req_duration histogram
req_duration {count:3,sum:6.0,bucket:[+Inf:3,0.1:1,1.0:2]}
# EOF
`
	exp := []parsedEntry{
		{m: "req_duration", help: "Request duration histogram."},
		{m: "req_duration", typ: model.MetricTypeHistogram},
		{
			m:    "req_duration_count",
			v:    3,
			lset: labels.FromStrings("__name__", "req_duration_count"),
		},
		{
			m:    "req_duration_sum",
			v:    6.0,
			lset: labels.FromStrings("__name__", "req_duration_sum"),
		},
		{
			m:    "req_duration_bucket",
			v:    3,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "le", "+Inf"),
		},
		{
			m:    "req_duration_bucket",
			v:    1,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "le", "0.1"),
		},
		{
			m:    "req_duration_bucket",
			v:    2,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "le", "1.0"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseNativeHistogram(t *testing.T) {
	input := `# HELP test_histogram Native histogram.
# TYPE test_histogram histogram
test_histogram {count:5,sum:12.1,schema:0,zero_threshold:0.001,zero_count:2,positive_spans:[0:3],positive_deltas:[1,1,-1],negative_spans:[],negative_deltas:[]}
# EOF
`
	exp := []parsedEntry{
		{m: "test_histogram", help: "Native histogram."},
		{m: "test_histogram", typ: model.MetricTypeHistogram},
		{
			m:    "test_histogram",
			lset: labels.FromStrings("__name__", "test_histogram"),
			shs: &histogram.Histogram{
				Schema:          0,
				ZeroThreshold:   0.001,
				ZeroCount:       2,
				Count:           5,
				Sum:             12.1,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 3}},
				PositiveBuckets: []int64{1, 1, -1},
			},
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseUTF8MetricName(t *testing.T) {
	input := `# TYPE "metric.name" gauge
{"metric.name",label="value"} 1.0
# EOF
`
	exp := []parsedEntry{
		{m: "metric.name", typ: model.MetricTypeGauge},
		{
			m:    `{"metric.name",label="value"}`,
			v:    1.0,
			lset: labels.FromStrings("__name__", "metric.name", "label", "value"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseErrors(t *testing.T) {
	for _, tc := range []struct {
		input string
		err   string
	}{
		{
			input: "metric 1.0\n",
			err:   "does not end with # EOF",
		},
		{
			input: "# EOF\nextra\n",
			err:   "unexpected data after # EOF",
		},
		{
			// Composite value with unknown type (no # TYPE before).
			input: `foo {count:1,sum:1.0}
# EOF
`,
			err: "composite value not supported",
		},
	} {
		t.Run(tc.err, func(t *testing.T) {
			p := NewOpenMetrics2Parser([]byte(tc.input), labels.NewSymbolTable())
			var gotErr error
			for {
				_, err := p.Next()
				if err != nil {
					gotErr = err
					break
				}
			}
			require.ErrorContains(t, gotErr, tc.err)
		})
	}
}

// TestOpenMetrics2ParseEOFHandling verifies that the parser returns io.EOF
// correctly when the input ends with "# EOF\n".
func TestOpenMetrics2ParseEOFHandling(t *testing.T) {
	p := NewOpenMetrics2Parser([]byte("# EOF\n"), labels.NewSymbolTable())
	et, err := p.Next()
	require.Equal(t, EntryInvalid, et)
	require.ErrorIs(t, err, io.EOF)
}

func TestOpenMetrics2ParseCompositeExtraLabelsUTF8(t *testing.T) {
	input := `# TYPE "req.duration" histogram
{"req.duration",job="api"} {count:3,sum:6.0,bucket:[+Inf:3,0.1:1]}
# EOF
`
	exp := []parsedEntry{
		{m: "req.duration", typ: model.MetricTypeHistogram},
		{
			m:    "req.duration_count",
			v:    3,
			lset: labels.FromStrings("__name__", "req.duration_count", "job", "api"),
		},
		{
			m:    "req.duration_sum",
			v:    6.0,
			lset: labels.FromStrings("__name__", "req.duration_sum", "job", "api"),
		},
		{
			m:    "req.duration_bucket",
			v:    3,
			lset: labels.FromStrings("__name__", "req.duration_bucket", "job", "api", "le", "+Inf"),
		},
		{
			m:    "req.duration_bucket",
			v:    1,
			lset: labels.FromStrings("__name__", "req.duration_bucket", "job", "api", "le", "0.1"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseCompositeExtraLabels(t *testing.T) {
	input := `# TYPE req_duration histogram
req_duration{job="api",env="prod"} {count:3,sum:6.0,bucket:[+Inf:3,0.1:1,1.0:2]}
# EOF
`
	exp := []parsedEntry{
		{m: "req_duration", typ: model.MetricTypeHistogram},
		{
			m:    "req_duration_count",
			v:    3,
			lset: labels.FromStrings("__name__", "req_duration_count", "env", "prod", "job", "api"),
		},
		{
			m:    "req_duration_sum",
			v:    6.0,
			lset: labels.FromStrings("__name__", "req_duration_sum", "env", "prod", "job", "api"),
		},
		{
			m:    "req_duration_bucket",
			v:    3,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "env", "prod", "job", "api", "le", "+Inf"),
		},
		{
			m:    "req_duration_bucket",
			v:    1,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "env", "prod", "job", "api", "le", "0.1"),
		},
		{
			m:    "req_duration_bucket",
			v:    2,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "env", "prod", "job", "api", "le", "1.0"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseCompositeWithTimestamp(t *testing.T) {
	input := `# TYPE req_duration histogram
req_duration {count:2,sum:4.0} 1234567.0 st@1000.0
# EOF
`
	exp := []parsedEntry{
		{m: "req_duration", typ: model.MetricTypeHistogram},
		{
			m:    "req_duration_count",
			v:    2,
			t:    int64p(1234567000),
			st:   1000000,
			lset: labels.FromStrings("__name__", "req_duration_count"),
		},
		{
			m:    "req_duration_sum",
			v:    4.0,
			t:    int64p(1234567000),
			st:   1000000,
			lset: labels.FromStrings("__name__", "req_duration_sum"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseFloatHistogram(t *testing.T) {
	input := `# TYPE test_histogram histogram
test_histogram {count:5.5,sum:12.1,schema:0,zero_threshold:0.001,zero_count:2.5,positive_spans:[0:2],positive_deltas:[2.0,1.0],negative_spans:[],negative_deltas:[]}
# EOF
`
	exp := []parsedEntry{
		{m: "test_histogram", typ: model.MetricTypeHistogram},
		{
			m:    "test_histogram",
			lset: labels.FromStrings("__name__", "test_histogram"),
			fhs: &histogram.FloatHistogram{
				Schema:          0,
				ZeroThreshold:   0.001,
				ZeroCount:       2.5,
				Count:           5.5,
				Sum:             12.1,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}},
				PositiveBuckets: []float64{2.0, 1.0},
			},
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseCompositeGaugeHistogram(t *testing.T) {
	input := `# TYPE req_size gaugehistogram
req_size {count:10,sum:100.0,schema:0,zero_threshold:0.001,zero_count:1,positive_spans:[0:2],positive_deltas:[3,2],negative_spans:[],negative_deltas:[]}
# EOF
`
	exp := []parsedEntry{
		{m: "req_size", typ: model.MetricTypeGaugeHistogram},
		{
			m:    "req_size",
			lset: labels.FromStrings("__name__", "req_size"),
			shs: &histogram.Histogram{
				Schema:           0,
				ZeroThreshold:    0.001,
				ZeroCount:        1,
				Count:            10,
				Sum:              100.0,
				PositiveSpans:    []histogram.Span{{Offset: 0, Length: 2}},
				PositiveBuckets:  []int64{3, 2},
				CounterResetHint: histogram.GaugeType,
			},
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseTypeAndUnitLabels(t *testing.T) {
	input := `# TYPE process_open_fds gauge
# UNIT process_open_fds fds
process_open_fds 8.0
# TYPE rpc_duration_seconds summary
# UNIT rpc_duration_seconds seconds
rpc_duration_seconds {count:10,sum:5.0,quantile:[0.5:1.0]}
# EOF
`
	exp := []parsedEntry{
		{m: "process_open_fds", typ: model.MetricTypeGauge},
		{m: "process_open_fds", unit: "fds"},
		{
			m:    "process_open_fds",
			v:    8.0,
			lset: labels.FromStrings("__name__", "process_open_fds", "__type__", "gauge", "__unit__", "fds"),
		},
		{m: "rpc_duration_seconds", typ: model.MetricTypeSummary},
		{m: "rpc_duration_seconds", unit: "seconds"},
		// Composite (pending) entries do not currently receive __type__/__unit__ injection
		// because buildPendingLabels bypasses schema.Metadata — see review issue #3.
		{
			m:    "rpc_duration_seconds_count",
			v:    10,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds_count"),
		},
		{
			m:    "rpc_duration_seconds_sum",
			v:    5.0,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds_sum"),
		},
		{
			m:    "rpc_duration_seconds",
			v:    1.0,
			lset: labels.FromStrings("__name__", "rpc_duration_seconds", "quantile", "0.5"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable(), WithOM2TypeAndUnitLabels())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}

func TestOpenMetrics2ParseExemplarOnCompositeLine(t *testing.T) {
	input := `# TYPE req_duration histogram
req_duration {count:2,sum:4.0,bucket:[+Inf:2,1.0:1]} # {id="req-1"} 3.8
# EOF
`
	exp := []parsedEntry{
		{m: "req_duration", typ: model.MetricTypeHistogram},
		// Exemplar is accessible only on the first pending entry served.
		{
			m:    "req_duration_count",
			v:    2,
			lset: labels.FromStrings("__name__", "req_duration_count"),
			es:   []exemplar.Exemplar{{Labels: labels.FromStrings("id", "req-1"), Value: 3.8}},
		},
		{
			m:    "req_duration_sum",
			v:    4.0,
			lset: labels.FromStrings("__name__", "req_duration_sum"),
		},
		{
			m:    "req_duration_bucket",
			v:    2,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "le", "+Inf"),
		},
		{
			m:    "req_duration_bucket",
			v:    1,
			lset: labels.FromStrings("__name__", "req_duration_bucket", "le", "1.0"),
		},
	}

	p := NewOpenMetrics2Parser([]byte(input), labels.NewSymbolTable())
	got := testParse(t, p)
	requireEntries(t, exp, got)
}
