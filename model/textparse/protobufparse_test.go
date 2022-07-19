// Copyright 2021 The Prometheus Authors
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
	"io"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"

	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
)

func TestProtobufParse(t *testing.T) {
	textMetricFamilies := []string{
		`name: "go_build_info"
help: "Build information about the main Go module."
type: GAUGE
metric: <
  label: <
    name: "checksum"
    value: ""
  >
  label: <
    name: "path"
    value: "github.com/prometheus/client_golang"
  >
  label: <
    name: "version"
    value: "(devel)"
  >
  gauge: <
    value: 1
  >
>

`,
		`name: "go_memstats_alloc_bytes_total"
help: "Total number of bytes allocated, even if freed."
type: COUNTER
metric: <
  counter: <
    value: 1.546544e+06
    exemplar: <
      label: <
        name: "dummyID"
        value: "42"
      >
      value: 12
      timestamp: <
        seconds: 1625851151
        nanos: 233181499
      >
    >
  >
>

`,
		`name: "something_untyped"
help: "Just to test the untyped type."
type: UNTYPED
metric: <
  untyped: <
    value: 42
  >
  timestamp_ms: 1234567
>

`,
		`name: "test_histogram"
help: "Test histogram with many buckets removed to keep it manageable in size."
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
      exemplar: <
        label: <
          name: "dummyID"
          value: "59727"
        >
        value: -0.00039
        timestamp: <
          seconds: 1625851155
          nanos: 146848499
        >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "5617"
        >
        value: -0.00029
      >
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

`,
		`name: "test_histogram2"
help: "Similar histogram as before but now without sparse buckets."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.000828
    bucket: <
      cumulative_count: 2
      upper_bound: -0.00048
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.00038
      exemplar: <
        label: <
          name: "dummyID"
          value: "59727"
        >
        value: -0.00038
        timestamp: <
          seconds: 1625851153
          nanos: 146848499
        >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: 1
      exemplar: <
        label: <
          name: "dummyID"
          value: "5617"
        >
        value: -0.000295
      >
    >
    schema: 0
    zero_threshold: 0
  >
>

`,
		`name: "rpc_durations_seconds"
help: "RPC latency distributions."
type: SUMMARY
metric: <
  label: <
    name: "service"
    value: "exponential"
  >
  summary: <
    sample_count: 262
    sample_sum: 0.00025551262820703587
    quantile: <
      quantile: 0.5
      value: 6.442786329648548e-07
    >
    quantile: <
      quantile: 0.9
      value: 1.9435742936658396e-06
    >
    quantile: <
      quantile: 0.99
      value: 4.0471608667037015e-06
    >
  >
>
`,
		`name: "without_quantiles"
help: "A summary without quantiles."
type: SUMMARY
metric: <
  summary: <
    sample_count: 42
    sample_sum: 1.234
  >
>
`,
	}

	varintBuf := make([]byte, binary.MaxVarintLen32)
	inputBuf := &bytes.Buffer{}

	for _, tmf := range textMetricFamilies {
		pb := &dto.MetricFamily{}
		// From text to proto message.
		require.NoError(t, proto.UnmarshalText(tmf, pb))
		// From proto message to binary protobuf.
		protoBuf, err := proto.Marshal(pb)
		require.NoError(t, err)

		// Write first length, then binary protobuf.
		varintLength := binary.PutUvarint(varintBuf, uint64(len(protoBuf)))
		inputBuf.Write(varintBuf[:varintLength])
		inputBuf.Write(protoBuf)
	}

	exp := []struct {
		lset    labels.Labels
		m       string
		t       int64
		v       float64
		typ     MetricType
		help    string
		unit    string
		comment string
		shs     *histogram.Histogram
		e       []exemplar.Exemplar
	}{
		{
			m:    "go_build_info",
			help: "Build information about the main Go module.",
		},
		{
			m:   "go_build_info",
			typ: MetricTypeGauge,
		},
		{
			m: "go_build_info\xFFchecksum\xFF\xFFpath\xFFgithub.com/prometheus/client_golang\xFFversion\xFF(devel)",
			v: 1,
			lset: labels.FromStrings(
				"__name__", "go_build_info",
				"checksum", "",
				"path", "github.com/prometheus/client_golang",
				"version", "(devel)",
			),
		},
		{
			m:    "go_memstats_alloc_bytes_total",
			help: "Total number of bytes allocated, even if freed.",
		},
		{
			m:   "go_memstats_alloc_bytes_total",
			typ: MetricTypeCounter,
		},
		{
			m: "go_memstats_alloc_bytes_total",
			v: 1.546544e+06,
			lset: labels.FromStrings(
				"__name__", "go_memstats_alloc_bytes_total",
			),
			e: []exemplar.Exemplar{
				{Labels: labels.FromStrings("dummyID", "42"), Value: 12, HasTs: true, Ts: 1625851151233},
			},
		},
		{
			m:    "something_untyped",
			help: "Just to test the untyped type.",
		},
		{
			m:   "something_untyped",
			typ: MetricTypeUnknown,
		},
		{
			m: "something_untyped",
			t: 1234567,
			v: 42,
			lset: labels.FromStrings(
				"__name__", "something_untyped",
			),
		},
		{
			m:    "test_histogram",
			help: "Test histogram with many buckets removed to keep it manageable in size.",
		},
		{
			m:   "test_histogram",
			typ: MetricTypeHistogram,
		},
		{
			m: "test_histogram",
			t: 1234568,
			shs: &histogram.Histogram{
				Count:         175,
				ZeroCount:     2,
				Sum:           0.0008280461746287094,
				ZeroThreshold: 2.938735877055719e-39,
				Schema:        3,
				PositiveSpans: []histogram.Span{
					{Offset: -161, Length: 1},
					{Offset: 8, Length: 3},
				},
				NegativeSpans: []histogram.Span{
					{Offset: -162, Length: 1},
					{Offset: 23, Length: 4},
				},
				PositiveBuckets: []int64{1, 2, -1, -1},
				NegativeBuckets: []int64{1, 3, -2, -1, 1},
			},
			lset: labels.FromStrings(
				"__name__", "test_histogram",
			),
			e: []exemplar.Exemplar{
				{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
				{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
			},
		},
		{
			m:    "test_histogram2",
			help: "Similar histogram as before but now without sparse buckets.",
		},
		{
			m:   "test_histogram2",
			typ: MetricTypeHistogram,
		},
		{
			m: "test_histogram2_count",
			v: 175,
			lset: labels.FromStrings(
				"__name__", "test_histogram2_count",
			),
		},
		{
			m: "test_histogram2_sum",
			v: 0.000828,
			lset: labels.FromStrings(
				"__name__", "test_histogram2_sum",
			),
		},
		{
			m: "test_histogram2_bucket\xffle\xff-0.00048",
			v: 2,
			lset: labels.FromStrings(
				"__name__", "test_histogram2_bucket",
				"le", "-0.00048",
			),
		},
		{
			m: "test_histogram2_bucket\xffle\xff-0.00038",
			v: 4,
			lset: labels.FromStrings(
				"__name__", "test_histogram2_bucket",
				"le", "-0.00038",
			),
			e: []exemplar.Exemplar{
				{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00038, HasTs: true, Ts: 1625851153146},
			},
		},
		{
			m: "test_histogram2_bucket\xffle\xff1.0",
			v: 16,
			lset: labels.FromStrings(
				"__name__", "test_histogram2_bucket",
				"le", "1.0",
			),
			e: []exemplar.Exemplar{
				{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.000295, HasTs: false},
			},
		},
		{
			m: "test_histogram2_bucket\xffle\xff+Inf",
			v: 175,
			lset: labels.FromStrings(
				"__name__", "test_histogram2_bucket",
				"le", "+Inf",
			),
		},
		{
			m:    "rpc_durations_seconds",
			help: "RPC latency distributions.",
		},
		{
			m:   "rpc_durations_seconds",
			typ: MetricTypeSummary,
		},
		{
			m: "rpc_durations_seconds_count\xffservice\xffexponential",
			v: 262,
			lset: labels.FromStrings(
				"__name__", "rpc_durations_seconds_count",
				"service", "exponential",
			),
		},
		{
			m: "rpc_durations_seconds_sum\xffservice\xffexponential",
			v: 0.00025551262820703587,
			lset: labels.FromStrings(
				"__name__", "rpc_durations_seconds_sum",
				"service", "exponential",
			),
		},
		{
			m: "rpc_durations_seconds\xffservice\xffexponential\xffquantile\xff0.5",
			v: 6.442786329648548e-07,
			lset: labels.FromStrings(
				"__name__", "rpc_durations_seconds",
				"quantile", "0.5",
				"service", "exponential",
			),
		},
		{
			m: "rpc_durations_seconds\xffservice\xffexponential\xffquantile\xff0.9",
			v: 1.9435742936658396e-06,
			lset: labels.FromStrings(
				"__name__", "rpc_durations_seconds",
				"quantile", "0.9",
				"service", "exponential",
			),
		},
		{
			m: "rpc_durations_seconds\xffservice\xffexponential\xffquantile\xff0.99",
			v: 4.0471608667037015e-06,
			lset: labels.FromStrings(
				"__name__", "rpc_durations_seconds",
				"quantile", "0.99",
				"service", "exponential",
			),
		},
		{
			m:    "without_quantiles",
			help: "A summary without quantiles.",
		},
		{
			m:   "without_quantiles",
			typ: MetricTypeSummary,
		},
		{
			m: "without_quantiles_count",
			v: 42,
			lset: labels.FromStrings(
				"__name__", "without_quantiles_count",
			),
		},
		{
			m: "without_quantiles_sum",
			v: 1.234,
			lset: labels.FromStrings(
				"__name__", "without_quantiles_sum",
			),
		},
	}

	p := NewProtobufParser(inputBuf.Bytes())
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
			if ts != nil {
				require.Equal(t, exp[i].t, *ts)
			} else {
				require.Equal(t, exp[i].t, int64(0))
			}
			require.Equal(t, exp[i].v, v)
			require.Equal(t, exp[i].lset, res)
			if len(exp[i].e) == 0 {
				require.Equal(t, false, found)
			} else {
				require.Equal(t, true, found)
				require.Equal(t, exp[i].e[0], e)
			}
			res = res[:0]

		case EntryHistogram:
			m, ts, shs := p.Histogram()

			p.Metric(&res)
			require.Equal(t, exp[i].m, string(m))
			if ts != nil {
				require.Equal(t, exp[i].t, *ts)
			} else {
				require.Equal(t, exp[i].t, int64(0))
			}
			require.Equal(t, exp[i].lset, res)
			res = res[:0]
			require.Equal(t, exp[i].m, string(m))
			require.Equal(t, exp[i].shs, shs)
			j := 0
			for e := (exemplar.Exemplar{}); p.Exemplar(&e); j++ {
				require.Equal(t, exp[i].e[j], e)
				e = exemplar.Exemplar{}
			}
			require.Equal(t, len(exp[i].e), j, "not enough exemplars found")

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
	// TODO(beorn7): Once supported by the parser, test exemplars for
	// counters, exemplars for sparse histograms, legacy histograms including exemplars,
	// summaries.
	require.Equal(t, len(exp), i)
}
