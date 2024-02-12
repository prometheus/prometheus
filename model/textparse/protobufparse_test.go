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
	"errors"
	"io"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"

	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
)

func createTestProtoBuf(t *testing.T) *bytes.Buffer {
	testMetricFamilies := []string{
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
unit: "bytes"
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
		`name: "test_gauge_histogram"
help: "Like test_histogram but as gauge histogram."
type: GAUGE_HISTOGRAM
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
		`name: "test_float_histogram"
help: "Test float histogram with many buckets removed to keep it manageable in size."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count_float: 175.0
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count_float: 2.0
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count_float: 4.0
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
      cumulative_count_float: 16
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
    zero_count_float: 2.0
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_count: 1.0
    negative_count: 3.0
    negative_count: -2.0
    negative_count: -1.0
    negative_count: 1.0
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_count: 1.0
    positive_count: 2.0
    positive_count: -1.0
    positive_count: -1.0
  >
  timestamp_ms: 1234568
>

`,
		`name: "test_gauge_float_histogram"
help: "Like test_float_histogram but as gauge histogram."
type: GAUGE_HISTOGRAM
metric: <
  histogram: <
    sample_count_float: 175.0
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count_float: 2.0
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count_float: 4.0
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
      cumulative_count_float: 16
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
    zero_count_float: 2.0
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_count: 1.0
    negative_count: 3.0
    negative_count: -2.0
    negative_count: -1.0
    negative_count: 1.0
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_count: 1.0
    positive_count: 2.0
    positive_count: -1.0
    positive_count: -1.0
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
		`name: "test_histogram_family"
help: "Test histogram metric family with two very simple histograms."
type: HISTOGRAM
metric: <
  label: <
    name: "foo"
    value: "bar"
  >
  histogram: <
    sample_count: 5
    sample_sum: 12.1
    bucket: <
      cumulative_count: 2
      upper_bound: 1.1
    >
    bucket: <
      cumulative_count: 3
      upper_bound: 2.2
    >
    schema: 3
    positive_span: <
      offset: 8
      length: 2
    >
    positive_delta: 2
    positive_delta: 1
  >
>
metric: <
  label: <
    name: "foo"
    value: "baz"
  >
  histogram: <
    sample_count: 6
    sample_sum: 13.1
    bucket: <
      cumulative_count: 1
      upper_bound: 1.1
    >
    bucket: <
      cumulative_count: 5
      upper_bound: 2.2
    >
    schema: 3
    positive_span: <
      offset: 8
      length: 2
    >
    positive_delta: 1
    positive_delta: 4
  >
>

`,
		`name: "test_float_histogram_with_zerothreshold_zero"
help: "Test float histogram with a zero threshold of zero."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count_float: 5.0
    sample_sum: 12.1
    schema: 3
    positive_span: <
      offset: 8
      length: 2
    >
    positive_count: 2.0
    positive_count: 3.0
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
		`name: "empty_histogram"
help: "A histogram without observations and with a zero threshold of zero but with a no-op span to identify it as a native histogram."
type: HISTOGRAM
metric: <
  histogram: <
    positive_span: <
      offset: 0
      length: 0
    >
  >
>

`,
		`name: "test_counter_with_createdtimestamp"
help: "A counter with a created timestamp."
type: COUNTER
metric: <
  counter: <
    value: 42
    created_timestamp: <
      seconds: 1
      nanos: 1
    >
  >
>

`,
		`name: "test_summary_with_createdtimestamp"
help: "A summary with a created timestamp."
type: SUMMARY
metric: <
  summary: <
    sample_count: 42
    sample_sum: 1.234
    created_timestamp: <
      seconds: 1
      nanos: 1
    >
  >
>

`,
		`name: "test_histogram_with_createdtimestamp"
help: "A histogram with a created timestamp."
type: HISTOGRAM
metric: <
  histogram: <
    created_timestamp: <
      seconds: 1
      nanos: 1
    >
    positive_span: <
      offset: 0
      length: 0
    >
  >
>

`,
		`name: "test_gaugehistogram_with_createdtimestamp"
help: "A gauge histogram with a created timestamp."
type: GAUGE_HISTOGRAM
metric: <
  histogram: <
    created_timestamp: <
      seconds: 1
      nanos: 1
    >
    positive_span: <
      offset: 0
      length: 0
    >
  >
>

`,
	}

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

func TestProtobufParse(t *testing.T) {
	type parseResult struct {
		lset    labels.Labels
		m       string
		t       int64
		v       float64
		typ     model.MetricType
		help    string
		unit    string
		comment string
		shs     *histogram.Histogram
		fhs     *histogram.FloatHistogram
		e       []exemplar.Exemplar
		ct      int64
	}

	inputBuf := createTestProtoBuf(t)

	scenarios := []struct {
		name     string
		parser   Parser
		expected []parseResult
	}{
		{
			name:   "ignore classic buckets of native histograms",
			parser: NewProtobufParser(inputBuf.Bytes(), false),
			expected: []parseResult{
				{
					m:    "go_build_info",
					help: "Build information about the main Go module.",
				},
				{
					m:   "go_build_info",
					typ: model.MetricTypeGauge,
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
					unit: "bytes",
				},
				{
					m:   "go_memstats_alloc_bytes_total",
					typ: model.MetricTypeCounter,
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
					typ: model.MetricTypeUnknown,
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
					typ: model.MetricTypeHistogram,
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
					},
				},
				{
					m:    "test_gauge_histogram",
					help: "Like test_histogram but as gauge histogram.",
				},
				{
					m:   "test_gauge_histogram",
					typ: model.MetricTypeGaugeHistogram,
				},
				{
					m: "test_gauge_histogram",
					t: 1234568,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.GaugeType,
						Count:            175,
						ZeroCount:        2,
						Sum:              0.0008280461746287094,
						ZeroThreshold:    2.938735877055719e-39,
						Schema:           3,
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
						"__name__", "test_gauge_histogram",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m:    "test_float_histogram",
					help: "Test float histogram with many buckets removed to keep it manageable in size.",
				},
				{
					m:   "test_float_histogram",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_float_histogram",
					t: 1234568,
					fhs: &histogram.FloatHistogram{
						Count:         175.0,
						ZeroCount:     2.0,
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
						PositiveBuckets: []float64{1.0, 2.0, -1.0, -1.0},
						NegativeBuckets: []float64{1.0, 3.0, -2.0, -1.0, 1.0},
					},
					lset: labels.FromStrings(
						"__name__", "test_float_histogram",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m:    "test_gauge_float_histogram",
					help: "Like test_float_histogram but as gauge histogram.",
				},
				{
					m:   "test_gauge_float_histogram",
					typ: model.MetricTypeGaugeHistogram,
				},
				{
					m: "test_gauge_float_histogram",
					t: 1234568,
					fhs: &histogram.FloatHistogram{
						CounterResetHint: histogram.GaugeType,
						Count:            175.0,
						ZeroCount:        2.0,
						Sum:              0.0008280461746287094,
						ZeroThreshold:    2.938735877055719e-39,
						Schema:           3,
						PositiveSpans: []histogram.Span{
							{Offset: -161, Length: 1},
							{Offset: 8, Length: 3},
						},
						NegativeSpans: []histogram.Span{
							{Offset: -162, Length: 1},
							{Offset: 23, Length: 4},
						},
						PositiveBuckets: []float64{1.0, 2.0, -1.0, -1.0},
						NegativeBuckets: []float64{1.0, 3.0, -2.0, -1.0, 1.0},
					},
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{
					m:    "test_histogram2",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{
					m:   "test_histogram2",
					typ: model.MetricTypeHistogram,
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
					m:    "test_histogram_family",
					help: "Test histogram metric family with two very simple histograms.",
				},
				{
					m:   "test_histogram_family",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_histogram_family\xfffoo\xffbar",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            5,
						Sum:              12.1,
						Schema:           3,
						PositiveSpans: []histogram.Span{
							{Offset: 8, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "bar",
					),
				},
				{
					m: "test_histogram_family\xfffoo\xffbaz",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              13.1,
						Schema:           3,
						PositiveSpans: []histogram.Span{
							{Offset: 8, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{1, 4},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "baz",
					),
				},
				{
					m:    "test_float_histogram_with_zerothreshold_zero",
					help: "Test float histogram with a zero threshold of zero.",
				},
				{
					m:   "test_float_histogram_with_zerothreshold_zero",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "test_float_histogram_with_zerothreshold_zero",
					fhs: &histogram.FloatHistogram{
						Count:  5.0,
						Sum:    12.1,
						Schema: 3,
						PositiveSpans: []histogram.Span{
							{Offset: 8, Length: 2},
						},
						PositiveBuckets: []float64{2.0, 3.0},
						NegativeSpans:   []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_with_zerothreshold_zero",
					),
				},
				{
					m:    "rpc_durations_seconds",
					help: "RPC latency distributions.",
				},
				{
					m:   "rpc_durations_seconds",
					typ: model.MetricTypeSummary,
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
					typ: model.MetricTypeSummary,
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
				{
					m:    "empty_histogram",
					help: "A histogram without observations and with a zero threshold of zero but with a no-op span to identify it as a native histogram.",
				},
				{
					m:   "empty_histogram",
					typ: model.MetricTypeHistogram,
				},
				{
					m: "empty_histogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "empty_histogram",
					),
				},
				{
					m:    "test_counter_with_createdtimestamp",
					help: "A counter with a created timestamp.",
				},
				{
					m:   "test_counter_with_createdtimestamp",
					typ: model.MetricTypeCounter,
				},
				{
					m:  "test_counter_with_createdtimestamp",
					v:  42,
					ct: 1000,
					lset: labels.FromStrings(
						"__name__", "test_counter_with_createdtimestamp",
					),
				},
				{
					m:    "test_summary_with_createdtimestamp",
					help: "A summary with a created timestamp.",
				},
				{
					m:   "test_summary_with_createdtimestamp",
					typ: model.MetricTypeSummary,
				},
				{
					m:  "test_summary_with_createdtimestamp_count",
					v:  42,
					ct: 1000,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_count",
					),
				},
				{
					m:  "test_summary_with_createdtimestamp_sum",
					v:  1.234,
					ct: 1000,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_sum",
					),
				},
				{
					m:    "test_histogram_with_createdtimestamp",
					help: "A histogram with a created timestamp.",
				},
				{
					m:   "test_histogram_with_createdtimestamp",
					typ: model.MetricTypeHistogram,
				},
				{
					m:  "test_histogram_with_createdtimestamp",
					ct: 1000,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_createdtimestamp",
					),
				},
				{
					m:    "test_gaugehistogram_with_createdtimestamp",
					help: "A gauge histogram with a created timestamp.",
				},
				{
					m:   "test_gaugehistogram_with_createdtimestamp",
					typ: model.MetricTypeGaugeHistogram,
				},
				{
					m:  "test_gaugehistogram_with_createdtimestamp",
					ct: 1000,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.GaugeType,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp",
					),
				},
			},
		},
		{
			name:   "parse classic and native buckets",
			parser: NewProtobufParser(inputBuf.Bytes(), true),
			expected: []parseResult{
				{ // 0
					m:    "go_build_info",
					help: "Build information about the main Go module.",
				},
				{ // 1
					m:   "go_build_info",
					typ: model.MetricTypeGauge,
				},
				{ // 2
					m: "go_build_info\xFFchecksum\xFF\xFFpath\xFFgithub.com/prometheus/client_golang\xFFversion\xFF(devel)",
					v: 1,
					lset: labels.FromStrings(
						"__name__", "go_build_info",
						"checksum", "",
						"path", "github.com/prometheus/client_golang",
						"version", "(devel)",
					),
				},
				{ // 3
					m:    "go_memstats_alloc_bytes_total",
					help: "Total number of bytes allocated, even if freed.",
				},
				{ // 4
					m:   "go_memstats_alloc_bytes_total",
					typ: model.MetricTypeCounter,
				},
				{ // 5
					m: "go_memstats_alloc_bytes_total",
					v: 1.546544e+06,
					lset: labels.FromStrings(
						"__name__", "go_memstats_alloc_bytes_total",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "42"), Value: 12, HasTs: true, Ts: 1625851151233},
					},
				},
				{ // 6
					m:    "something_untyped",
					help: "Just to test the untyped type.",
				},
				{ // 7
					m:   "something_untyped",
					typ: model.MetricTypeUnknown,
				},
				{ // 8
					m: "something_untyped",
					t: 1234567,
					v: 42,
					lset: labels.FromStrings(
						"__name__", "something_untyped",
					),
				},
				{ // 9
					m:    "test_histogram",
					help: "Test histogram with many buckets removed to keep it manageable in size.",
				},
				{ // 10
					m:   "test_histogram",
					typ: model.MetricTypeHistogram,
				},
				{ // 11
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
					},
				},
				{ // 12
					m: "test_histogram_count",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_count",
					),
				},
				{ // 13
					m: "test_histogram_sum",
					t: 1234568,
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_histogram_sum",
					),
				},
				{ // 14
					m: "test_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: 1234568,
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{ // 15
					m: "test_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: 1234568,
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{ // 16
					m: "test_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: 1234568,
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{ // 17
					m: "test_histogram_bucket\xffle\xff+Inf",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram_bucket",
						"le", "+Inf",
					),
				},
				{ // 18
					m:    "test_gauge_histogram",
					help: "Like test_histogram but as gauge histogram.",
				},
				{ // 19
					m:   "test_gauge_histogram",
					typ: model.MetricTypeGaugeHistogram,
				},
				{ // 20
					m: "test_gauge_histogram",
					t: 1234568,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.GaugeType,
						Count:            175,
						ZeroCount:        2,
						Sum:              0.0008280461746287094,
						ZeroThreshold:    2.938735877055719e-39,
						Schema:           3,
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
						"__name__", "test_gauge_histogram",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{ // 21
					m: "test_gauge_histogram_count",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_count",
					),
				},
				{ // 22
					m: "test_gauge_histogram_sum",
					t: 1234568,
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_sum",
					),
				},
				{ // 23
					m: "test_gauge_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: 1234568,
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{ // 24
					m: "test_gauge_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: 1234568,
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{ // 25
					m: "test_gauge_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: 1234568,
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{ // 26
					m: "test_gauge_histogram_bucket\xffle\xff+Inf",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_histogram_bucket",
						"le", "+Inf",
					),
				},
				{ // 27
					m:    "test_float_histogram",
					help: "Test float histogram with many buckets removed to keep it manageable in size.",
				},
				{ // 28
					m:   "test_float_histogram",
					typ: model.MetricTypeHistogram,
				},
				{ // 29
					m: "test_float_histogram",
					t: 1234568,
					fhs: &histogram.FloatHistogram{
						Count:         175.0,
						ZeroCount:     2.0,
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
						PositiveBuckets: []float64{1.0, 2.0, -1.0, -1.0},
						NegativeBuckets: []float64{1.0, 3.0, -2.0, -1.0, 1.0},
					},
					lset: labels.FromStrings(
						"__name__", "test_float_histogram",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{ // 30
					m: "test_float_histogram_count",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_count",
					),
				},
				{ // 31
					m: "test_float_histogram_sum",
					t: 1234568,
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_sum",
					),
				},
				{ // 32
					m: "test_float_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: 1234568,
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{ // 33
					m: "test_float_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: 1234568,
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{ // 34
					m: "test_float_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: 1234568,
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{ // 35
					m: "test_float_histogram_bucket\xffle\xff+Inf",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_bucket",
						"le", "+Inf",
					),
				},
				{ // 36
					m:    "test_gauge_float_histogram",
					help: "Like test_float_histogram but as gauge histogram.",
				},
				{ // 37
					m:   "test_gauge_float_histogram",
					typ: model.MetricTypeGaugeHistogram,
				},
				{ // 38
					m: "test_gauge_float_histogram",
					t: 1234568,
					fhs: &histogram.FloatHistogram{
						CounterResetHint: histogram.GaugeType,
						Count:            175.0,
						ZeroCount:        2.0,
						Sum:              0.0008280461746287094,
						ZeroThreshold:    2.938735877055719e-39,
						Schema:           3,
						PositiveSpans: []histogram.Span{
							{Offset: -161, Length: 1},
							{Offset: 8, Length: 3},
						},
						NegativeSpans: []histogram.Span{
							{Offset: -162, Length: 1},
							{Offset: 23, Length: 4},
						},
						PositiveBuckets: []float64{1.0, 2.0, -1.0, -1.0},
						NegativeBuckets: []float64{1.0, 3.0, -2.0, -1.0, 1.0},
					},
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{ // 39
					m: "test_gauge_float_histogram_count",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_count",
					),
				},
				{ // 40
					m: "test_gauge_float_histogram_sum",
					t: 1234568,
					v: 0.0008280461746287094,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_sum",
					),
				},
				{ // 41
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0004899999999999998",
					t: 1234568,
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0004899999999999998",
					),
				},
				{ // 42
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0003899999999999998",
					t: 1234568,
					v: 4,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0003899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, HasTs: true, Ts: 1625851155146},
					},
				},
				{ // 43
					m: "test_gauge_float_histogram_bucket\xffle\xff-0.0002899999999999998",
					t: 1234568,
					v: 16,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "-0.0002899999999999998",
					),
					e: []exemplar.Exemplar{
						{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, HasTs: false},
					},
				},
				{ // 44
					m: "test_gauge_float_histogram_bucket\xffle\xff+Inf",
					t: 1234568,
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_gauge_float_histogram_bucket",
						"le", "+Inf",
					),
				},
				{ // 45
					m:    "test_histogram2",
					help: "Similar histogram as before but now without sparse buckets.",
				},
				{ // 46
					m:   "test_histogram2",
					typ: model.MetricTypeHistogram,
				},
				{ // 47
					m: "test_histogram2_count",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_count",
					),
				},
				{ // 48
					m: "test_histogram2_sum",
					v: 0.000828,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_sum",
					),
				},
				{ // 49
					m: "test_histogram2_bucket\xffle\xff-0.00048",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_bucket",
						"le", "-0.00048",
					),
				},
				{ // 50
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
				{ // 51
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
				{ // 52
					m: "test_histogram2_bucket\xffle\xff+Inf",
					v: 175,
					lset: labels.FromStrings(
						"__name__", "test_histogram2_bucket",
						"le", "+Inf",
					),
				},
				{ // 53
					m:    "test_histogram_family",
					help: "Test histogram metric family with two very simple histograms.",
				},
				{ // 54
					m:   "test_histogram_family",
					typ: model.MetricTypeHistogram,
				},
				{ // 55
					m: "test_histogram_family\xfffoo\xffbar",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            5,
						Sum:              12.1,
						Schema:           3,
						PositiveSpans: []histogram.Span{
							{Offset: 8, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{2, 1},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "bar",
					),
				},
				{ // 56
					m: "test_histogram_family_count\xfffoo\xffbar",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "bar",
					),
				},
				{ // 57
					m: "test_histogram_family_sum\xfffoo\xffbar",
					v: 12.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "bar",
					),
				},
				{ // 58
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff1.1",
					v: 2,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "1.1",
					),
				},
				{ // 59
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff2.2",
					v: 3,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "2.2",
					),
				},
				{ // 60
					m: "test_histogram_family_bucket\xfffoo\xffbar\xffle\xff+Inf",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "bar",
						"le", "+Inf",
					),
				},
				{ // 61
					m: "test_histogram_family\xfffoo\xffbaz",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						Count:            6,
						Sum:              13.1,
						Schema:           3,
						PositiveSpans: []histogram.Span{
							{Offset: 8, Length: 2},
						},
						NegativeSpans:   []histogram.Span{},
						PositiveBuckets: []int64{1, 4},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_family",
						"foo", "baz",
					),
				},
				{ // 62
					m: "test_histogram_family_count\xfffoo\xffbaz",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_count",
						"foo", "baz",
					),
				},
				{ // 63
					m: "test_histogram_family_sum\xfffoo\xffbaz",
					v: 13.1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_sum",
						"foo", "baz",
					),
				},
				{ // 64
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff1.1",
					v: 1,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "1.1",
					),
				},
				{ // 65
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff2.2",
					v: 5,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "2.2",
					),
				},
				{ // 66
					m: "test_histogram_family_bucket\xfffoo\xffbaz\xffle\xff+Inf",
					v: 6,
					lset: labels.FromStrings(
						"__name__", "test_histogram_family_bucket",
						"foo", "baz",
						"le", "+Inf",
					),
				},
				{ // 67
					m:    "test_float_histogram_with_zerothreshold_zero",
					help: "Test float histogram with a zero threshold of zero.",
				},
				{ // 68
					m:   "test_float_histogram_with_zerothreshold_zero",
					typ: model.MetricTypeHistogram,
				},
				{ // 69
					m: "test_float_histogram_with_zerothreshold_zero",
					fhs: &histogram.FloatHistogram{
						Count:  5.0,
						Sum:    12.1,
						Schema: 3,
						PositiveSpans: []histogram.Span{
							{Offset: 8, Length: 2},
						},
						PositiveBuckets: []float64{2.0, 3.0},
						NegativeSpans:   []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_float_histogram_with_zerothreshold_zero",
					),
				},
				{ // 70
					m:    "rpc_durations_seconds",
					help: "RPC latency distributions.",
				},
				{ // 71
					m:   "rpc_durations_seconds",
					typ: model.MetricTypeSummary,
				},
				{ // 72
					m: "rpc_durations_seconds_count\xffservice\xffexponential",
					v: 262,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds_count",
						"service", "exponential",
					),
				},
				{ // 73
					m: "rpc_durations_seconds_sum\xffservice\xffexponential",
					v: 0.00025551262820703587,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds_sum",
						"service", "exponential",
					),
				},
				{ // 74
					m: "rpc_durations_seconds\xffservice\xffexponential\xffquantile\xff0.5",
					v: 6.442786329648548e-07,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.5",
						"service", "exponential",
					),
				},
				{ // 75
					m: "rpc_durations_seconds\xffservice\xffexponential\xffquantile\xff0.9",
					v: 1.9435742936658396e-06,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.9",
						"service", "exponential",
					),
				},
				{ // 76
					m: "rpc_durations_seconds\xffservice\xffexponential\xffquantile\xff0.99",
					v: 4.0471608667037015e-06,
					lset: labels.FromStrings(
						"__name__", "rpc_durations_seconds",
						"quantile", "0.99",
						"service", "exponential",
					),
				},
				{ // 77
					m:    "without_quantiles",
					help: "A summary without quantiles.",
				},
				{ // 78
					m:   "without_quantiles",
					typ: model.MetricTypeSummary,
				},
				{ // 79
					m: "without_quantiles_count",
					v: 42,
					lset: labels.FromStrings(
						"__name__", "without_quantiles_count",
					),
				},
				{ // 80
					m: "without_quantiles_sum",
					v: 1.234,
					lset: labels.FromStrings(
						"__name__", "without_quantiles_sum",
					),
				},
				{ // 78
					m:    "empty_histogram",
					help: "A histogram without observations and with a zero threshold of zero but with a no-op span to identify it as a native histogram.",
				},
				{ // 79
					m:   "empty_histogram",
					typ: model.MetricTypeHistogram,
				},
				{ // 80
					m: "empty_histogram",
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "empty_histogram",
					),
				},
				{ // 81
					m:    "test_counter_with_createdtimestamp",
					help: "A counter with a created timestamp.",
				},
				{ // 82
					m:   "test_counter_with_createdtimestamp",
					typ: model.MetricTypeCounter,
				},
				{ // 83
					m:  "test_counter_with_createdtimestamp",
					v:  42,
					ct: 1000,
					lset: labels.FromStrings(
						"__name__", "test_counter_with_createdtimestamp",
					),
				},
				{ // 84
					m:    "test_summary_with_createdtimestamp",
					help: "A summary with a created timestamp.",
				},
				{ // 85
					m:   "test_summary_with_createdtimestamp",
					typ: model.MetricTypeSummary,
				},
				{ // 86
					m:  "test_summary_with_createdtimestamp_count",
					v:  42,
					ct: 1000,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_count",
					),
				},
				{ // 87
					m:  "test_summary_with_createdtimestamp_sum",
					v:  1.234,
					ct: 1000,
					lset: labels.FromStrings(
						"__name__", "test_summary_with_createdtimestamp_sum",
					),
				},
				{ // 88
					m:    "test_histogram_with_createdtimestamp",
					help: "A histogram with a created timestamp.",
				},
				{ // 89
					m:   "test_histogram_with_createdtimestamp",
					typ: model.MetricTypeHistogram,
				},
				{ // 90
					m:  "test_histogram_with_createdtimestamp",
					ct: 1000,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.UnknownCounterReset,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_histogram_with_createdtimestamp",
					),
				},
				{ // 91
					m:    "test_gaugehistogram_with_createdtimestamp",
					help: "A gauge histogram with a created timestamp.",
				},
				{ // 92
					m:   "test_gaugehistogram_with_createdtimestamp",
					typ: model.MetricTypeGaugeHistogram,
				},
				{ // 93
					m:  "test_gaugehistogram_with_createdtimestamp",
					ct: 1000,
					shs: &histogram.Histogram{
						CounterResetHint: histogram.GaugeType,
						PositiveSpans:    []histogram.Span{},
						NegativeSpans:    []histogram.Span{},
					},
					lset: labels.FromStrings(
						"__name__", "test_gaugehistogram_with_createdtimestamp",
					),
				},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			var (
				i   int
				res labels.Labels
				p   = scenario.parser
				exp = scenario.expected
			)

			for {
				et, err := p.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)

				switch et {
				case EntrySeries:
					m, ts, v := p.Series()

					var e exemplar.Exemplar
					p.Metric(&res)
					eFound := p.Exemplar(&e)
					ct := p.CreatedTimestamp()
					require.Equal(t, exp[i].m, string(m), "i: %d", i)
					if ts != nil {
						require.Equal(t, exp[i].t, *ts, "i: %d", i)
					} else {
						require.Equal(t, int64(0), exp[i].t, "i: %d", i)
					}
					require.Equal(t, exp[i].v, v, "i: %d", i)
					testutil.RequireEqual(t, exp[i].lset, res, "i: %d", i)
					if len(exp[i].e) == 0 {
						require.False(t, eFound, "i: %d", i)
					} else {
						require.True(t, eFound, "i: %d", i)
						testutil.RequireEqual(t, exp[i].e[0], e, "i: %d", i)
						require.False(t, p.Exemplar(&e), "too many exemplars returned, i: %d", i)
					}
					if exp[i].ct != 0 {
						require.NotNilf(t, ct, "i: %d", i)
						require.Equal(t, exp[i].ct, *ct, "i: %d", i)
					} else {
						require.Nilf(t, ct, "i: %d", i)
					}

				case EntryHistogram:
					m, ts, shs, fhs := p.Histogram()
					p.Metric(&res)
					require.Equal(t, exp[i].m, string(m), "i: %d", i)
					if ts != nil {
						require.Equal(t, exp[i].t, *ts, "i: %d", i)
					} else {
						require.Equal(t, int64(0), exp[i].t, "i: %d", i)
					}
					testutil.RequireEqual(t, exp[i].lset, res, "i: %d", i)
					require.Equal(t, exp[i].m, string(m), "i: %d", i)
					if shs != nil {
						require.Equal(t, exp[i].shs, shs, "i: %d", i)
					} else {
						require.Equal(t, exp[i].fhs, fhs, "i: %d", i)
					}
					j := 0
					for e := (exemplar.Exemplar{}); p.Exemplar(&e); j++ {
						testutil.RequireEqual(t, exp[i].e[j], e, "i: %d", i)
						e = exemplar.Exemplar{}
					}
					require.Len(t, exp[i].e, j, "not enough exemplars found, i: %d", i)

				case EntryType:
					m, typ := p.Type()
					require.Equal(t, exp[i].m, string(m), "i: %d", i)
					require.Equal(t, exp[i].typ, typ, "i: %d", i)

				case EntryHelp:
					m, h := p.Help()
					require.Equal(t, exp[i].m, string(m), "i: %d", i)
					require.Equal(t, exp[i].help, string(h), "i: %d", i)

				case EntryUnit:
					m, u := p.Unit()
					require.Equal(t, exp[i].m, string(m), "i: %d", i)
					require.Equal(t, exp[i].unit, string(u), "i: %d", i)

				case EntryComment:
					require.Equal(t, exp[i].comment, string(p.Comment()), "i: %d", i)
				}

				i++
			}
			require.Len(t, exp, i)
		})
	}
}
