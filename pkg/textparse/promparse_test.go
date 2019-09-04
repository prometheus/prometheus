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
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestPromParse(t *testing.T) {
	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# 	TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25",} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds{quantile="0.8", a="b"} 8.3835e-05
go_gc_duration_seconds{ quantile="0.9", a="b"} 8.3835e-05
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
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"}	1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33  	123123
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1`
	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1"

	int64p := func(x int64) *int64 { return &x }

	exp := []struct {
		lset    labels.Labels
		m       string
		t       *int64
		v       float64
		typ     MetricType
		help    string
		comment string
	}{
		{
			m:    "go_gc_duration_seconds",
			help: "A summary of the GC invocation durations.",
		}, {
			m:   "go_gc_duration_seconds",
			typ: MetricTypeSummary,
		}, {
			m:    `go_gc_duration_seconds{quantile="0"}`,
			v:    4.9351e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "0"),
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
			m:    "metric",
			help: "foo\x00bar",
		}, {
			m:    "null_byte_metric{a=\"abc\x00\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"),
		},
	}

	p := NewPromParser([]byte(input))
	i := 0

	var res labels.Labels

	for {
		et, err := p.Next()
		if err == io.EOF {
			break
		}
		testutil.Ok(t, err)

		switch et {
		case EntrySeries:
			m, ts, v := p.Series()

			p.Metric(&res)

			testutil.Equals(t, exp[i].m, string(m))
			testutil.Equals(t, exp[i].t, ts)
			testutil.Equals(t, exp[i].v, v)
			testutil.Equals(t, exp[i].lset, res)
			res = res[:0]

		case EntryType:
			m, typ := p.Type()
			testutil.Equals(t, exp[i].m, string(m))
			testutil.Equals(t, exp[i].typ, typ)

		case EntryHelp:
			m, h := p.Help()
			testutil.Equals(t, exp[i].m, string(m))
			testutil.Equals(t, exp[i].help, string(h))

		case EntryComment:
			testutil.Equals(t, exp[i].comment, string(p.Comment()))
		}

		i++
	}
	testutil.Equals(t, len(exp), i)
}

func TestPromParseErrors(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		{
			input: "a",
			err:   "expected value after metric, got \"MNAME\"",
		},
		{
			input: "a{b='c'} 1\n",
			err:   "expected label value, got \"INVALID\"",
		},
		{
			input: "a{b=\n",
			err:   "expected label value, got \"INVALID\"",
		},
		{
			input: "a{\xff=\"foo\"} 1\n",
			err:   "expected label name, got \"INVALID\"",
		},
		{
			input: "a{b=\"\xff\"} 1\n",
			err:   "invalid UTF-8 label value",
		},
		{
			input: "a true\n",
			err:   "strconv.ParseFloat: parsing \"true\": invalid syntax",
		},
		{
			input: "something_weird{problem=\"",
			err:   "expected label value, got \"INVALID\"",
		},
		{
			input: "empty_label_name{=\"\"} 0",
			err:   "expected label name, got \"EQUAL\"",
		},
		{
			input: "foo 1_2\n",
			err:   "unsupported character in float",
		},
		{
			input: "foo 0x1p-3\n",
			err:   "unsupported character in float",
		},
		{
			input: "foo 0x1P-3\n",
			err:   "unsupported character in float",
		},
		{
			input: "foo 0 1_2\n",
			err:   "expected next entry after timestamp, got \"MNAME\"",
		},
	}

	for i, c := range cases {
		p := NewPromParser([]byte(c.input))
		var err error
		for err == nil {
			_, err = p.Next()
		}
		testutil.NotOk(t, err)
		testutil.Equals(t, c.err, err.Error(), "test %d", i)
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
			err:   "expected label value, got \"INVALID\"",
		},
		{
			input: "a{b=\"\x00",
			err:   "expected label value, got \"INVALID\"",
		},
		{
			input: "a{b\x00=\"hiih\"}	1",
			err: "expected equal, got \"INVALID\"",
		},
		{
			input: "a\x00{b=\"ddd\"} 1",
			err:   "expected value after metric, got \"MNAME\"",
		},
	}

	for i, c := range cases {
		p := NewPromParser([]byte(c.input))
		var err error
		for err == nil {
			_, err = p.Next()
		}

		if c.err == "" {
			testutil.Equals(t, io.EOF, err, "test %d", i)
			continue
		}

		testutil.NotOk(t, err)
		testutil.Equals(t, c.err, err.Error(), "test %d", i)
	}
}

const (
	promtestdataSampleCount = 410
)

func BenchmarkParse(b *testing.B) {
	for parserName, parser := range map[string]func([]byte) Parser{
		"prometheus":  NewPromParser,
		"openmetrics": NewOpenMetricsParser,
	} {
		for _, fn := range []string{"promtestdata.txt", "promtestdata.nometa.txt"} {
			f, err := os.Open(fn)
			testutil.Ok(b, err)
			defer f.Close()

			buf, err := ioutil.ReadAll(f)
			testutil.Ok(b, err)

			b.Run(parserName+"/no-decode-metric/"+fn, func(b *testing.B) {
				total := 0

				b.SetBytes(int64(len(buf) * (b.N / promtestdataSampleCount)))
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i += promtestdataSampleCount {
					p := parser(buf)

				Outer:
					for i < b.N {
						t, err := p.Next()
						switch t {
						case EntryInvalid:
							if err == io.EOF {
								break Outer
							}
							b.Fatal(err)
						case EntrySeries:
							m, _, _ := p.Series()
							total += len(m)
							i++
						}
					}
				}
				_ = total
			})
			b.Run(parserName+"/decode-metric/"+fn, func(b *testing.B) {
				total := 0

				b.SetBytes(int64(len(buf) * (b.N / promtestdataSampleCount)))
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i += promtestdataSampleCount {
					p := parser(buf)

				Outer:
					for i < b.N {
						t, err := p.Next()
						switch t {
						case EntryInvalid:
							if err == io.EOF {
								break Outer
							}
							b.Fatal(err)
						case EntrySeries:
							m, _, _ := p.Series()

							res := make(labels.Labels, 0, 5)
							p.Metric(&res)

							total += len(m)
							i++
						}
					}
				}
				_ = total
			})
			b.Run(parserName+"/decode-metric-reuse/"+fn, func(b *testing.B) {
				total := 0
				res := make(labels.Labels, 0, 5)

				b.SetBytes(int64(len(buf) * (b.N / promtestdataSampleCount)))
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i += promtestdataSampleCount {
					p := parser(buf)

				Outer:
					for i < b.N {
						t, err := p.Next()
						switch t {
						case EntryInvalid:
							if err == io.EOF {
								break Outer
							}
							b.Fatal(err)
						case EntrySeries:
							m, _, _ := p.Series()

							p.Metric(&res)

							total += len(m)
							i++
							res = res[:0]
						}
					}
				}
				_ = total
			})
			b.Run("expfmt-text/"+fn, func(b *testing.B) {
				b.SetBytes(int64(len(buf) * (b.N / promtestdataSampleCount)))
				b.ReportAllocs()
				b.ResetTimer()

				total := 0

				for i := 0; i < b.N; i += promtestdataSampleCount {
					var (
						decSamples = make(model.Vector, 0, 50)
					)
					sdec := expfmt.SampleDecoder{
						Dec: expfmt.NewDecoder(bytes.NewReader(buf), expfmt.FmtText),
						Opts: &expfmt.DecodeOptions{
							Timestamp: model.TimeFromUnixNano(0),
						},
					}

					for {
						if err = sdec.Decode(&decSamples); err != nil {
							break
						}
						total += len(decSamples)
						decSamples = decSamples[:0]
					}
				}
				_ = total
			})
		}
	}
}
func BenchmarkGzip(b *testing.B) {
	for _, fn := range []string{"promtestdata.txt", "promtestdata.nometa.txt"} {
		b.Run(fn, func(b *testing.B) {
			f, err := os.Open(fn)
			testutil.Ok(b, err)
			defer f.Close()

			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)

			n, err := io.Copy(gw, f)
			testutil.Ok(b, err)
			testutil.Ok(b, gw.Close())

			gbuf, err := ioutil.ReadAll(&buf)
			testutil.Ok(b, err)

			k := b.N / promtestdataSampleCount

			b.ReportAllocs()
			b.SetBytes(int64(k) * int64(n))
			b.ResetTimer()

			total := 0

			for i := 0; i < k; i++ {
				gr, err := gzip.NewReader(bytes.NewReader(gbuf))
				testutil.Ok(b, err)

				d, err := ioutil.ReadAll(gr)
				testutil.Ok(b, err)
				testutil.Ok(b, gr.Close())

				total += len(d)
			}
			_ = total
		})
	}
}
