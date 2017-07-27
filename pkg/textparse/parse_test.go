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
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25",} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds{quantile="0.8", a="b"} 8.3835e-05
go_gc_duration_seconds{ quantile="0.9", a="b"} 8.3835e-05
go_gc_duration_seconds{ quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile= "1.0", a= "b" } 8.3835e-05
go_gc_duration_seconds { quantile = "1.0", a = "b" } 8.3835e-05
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"}	1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33  	123123
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1`
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1"

	int64p := func(x int64) *int64 { return &x }

	exp := []struct {
		lset labels.Labels
		m    string
		t    *int64
		v    float64
	}{
		{
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
			m:    `go_gc_duration_seconds{ quantile="1.0", a="b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds { quantile="1.0", a="b" }`,
			v:    8.3835e-05,
			lset: labels.FromStrings("__name__", "go_gc_duration_seconds", "quantile", "1.0", "a", "b"),
		}, {
			m:    `go_gc_duration_seconds { quantile= "1.0", a= "b" }`,
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
			m:    "null_byte_metric{a=\"abc\x00\"}",
			v:    1,
			lset: labels.FromStrings("__name__", "null_byte_metric", "a", "abc\x00"),
		},
	}

	p := New([]byte(input))
	i := 0

	var res labels.Labels

	for p.Next() {
		m, ts, v := p.At()

		p.Metric(&res)

		require.Equal(t, exp[i].m, string(m))
		require.Equal(t, exp[i].t, ts)
		require.Equal(t, exp[i].v, v)
		require.Equal(t, exp[i].lset, res)

		i++
		res = res[:0]
	}

	require.NoError(t, p.Err())
	require.Equal(t, len(exp), i)

}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		input string
		err   string
	}{
		{
			input: "a",
			err:   "no token found",
		},
		{
			input: "a{b='c'} 1\n",
			err:   "no token found",
		},
		{
			input: "a{b=\n",
			err:   "no token found",
		},
		{
			input: "a{\xff=\"foo\"} 1\n",
			err:   "no token found",
		},
		{
			input: "a{b=\"\xff\"} 1\n",
			err:   "Invalid UTF-8 label value.",
		},
		{
			input: "a true\n",
			err:   "strconv.ParseFloat: parsing \"true\": invalid syntax",
		},
		{
			input: "something_weird{problem=\"",
			err:   "no token found",
		},
	}

	for _, c := range cases {
		p := New([]byte(c.input))
		for p.Next() {
		}
		require.NotNil(t, p.Err())
		require.Equal(t, c.err, p.Err().Error())
	}
}

func TestNullByteHandling(t *testing.T) {
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
			err:   "no token found",
		},
		{
			input: "a{b=\"\x00",
			err:   "no token found",
		},
		{
			input: "a{b\x00=\"hiih\"}	1",
			err: "no token found",
		},
		{
			input: "a\x00{b=\"ddd\"} 1",
			err:   "no token found",
		},
	}

	for _, c := range cases {
		p := New([]byte(c.input))
		for p.Next() {
		}

		if c.err == "" {
			require.NoError(t, p.Err())
			continue
		}

		require.Error(t, p.Err())
		require.Equal(t, c.err, p.Err().Error())
	}
}

const (
	testdataSampleCount = 410
)

func BenchmarkParse(b *testing.B) {
	for _, fn := range []string{"testdata.txt", "testdata.nometa.txt"} {
		f, err := os.Open(fn)
		require.NoError(b, err)
		defer f.Close()

		buf, err := ioutil.ReadAll(f)
		require.NoError(b, err)

		b.Run("no-decode-metric/"+fn, func(b *testing.B) {
			total := 0

			b.SetBytes(int64(len(buf) * (b.N / testdataSampleCount)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i += testdataSampleCount {
				p := New(buf)

				for p.Next() && i < b.N {
					m, _, _ := p.At()

					total += len(m)
					i++
				}
				require.NoError(b, p.Err())
			}
			_ = total
		})
		b.Run("decode-metric/"+fn, func(b *testing.B) {
			total := 0

			b.SetBytes(int64(len(buf) * (b.N / testdataSampleCount)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i += testdataSampleCount {
				p := New(buf)

				for p.Next() && i < b.N {
					m, _, _ := p.At()

					res := make(labels.Labels, 0, 5)
					p.Metric(&res)

					total += len(m)
					i++
				}
				require.NoError(b, p.Err())
			}
			_ = total
		})
		b.Run("decode-metric-reuse/"+fn, func(b *testing.B) {
			total := 0
			res := make(labels.Labels, 0, 5)

			b.SetBytes(int64(len(buf) * (b.N / testdataSampleCount)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i += testdataSampleCount {
				p := New(buf)

				for p.Next() && i < b.N {
					m, _, _ := p.At()

					p.Metric(&res)

					total += len(m)
					i++
					res = res[:0]
				}
				require.NoError(b, p.Err())
			}
			_ = total
		})
		b.Run("expfmt-text/"+fn, func(b *testing.B) {
			b.SetBytes(int64(len(buf) * (b.N / testdataSampleCount)))
			b.ReportAllocs()
			b.ResetTimer()

			total := 0

			for i := 0; i < b.N; i += testdataSampleCount {
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

func BenchmarkGzip(b *testing.B) {
	for _, fn := range []string{"testdata.txt", "testdata.nometa.txt"} {
		b.Run(fn, func(b *testing.B) {
			f, err := os.Open(fn)
			require.NoError(b, err)
			defer f.Close()

			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)

			n, err := io.Copy(gw, f)
			require.NoError(b, err)
			require.NoError(b, gw.Close())

			gbuf, err := ioutil.ReadAll(&buf)
			require.NoError(b, err)

			k := b.N / testdataSampleCount

			b.ReportAllocs()
			b.SetBytes(int64(k) * int64(n))
			b.ResetTimer()

			total := 0

			for i := 0; i < k; i++ {
				gr, err := gzip.NewReader(bytes.NewReader(gbuf))
				require.NoError(b, err)

				d, err := ioutil.ReadAll(gr)
				require.NoError(b, err)
				require.NoError(b, gr.Close())

				total += len(d)
			}
			_ = total
		})
	}
}
